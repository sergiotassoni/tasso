// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

// Memoria per decomprimere. Un archivio di pochi byte può "dichiarare" un dizionario enorme
// (1, 2, 3 GB): la libreria lo riservava tutto subito, già solo per aprire l'archivio, e con
// poca RAM libera il programma si chiudeva di colpo (in Go "out of memory" non si ferma).
//
// Regole:
//   - il dizionario non serve mai più grande dei dati che escono (un rimando non può andare
//     prima dell'inizio): lo riduco a quella misura, come fa 7-Zip;
//   - se anche così serve più memoria di quella che il PC ha libera, mi fermo con un
//     messaggio chiaro (ErrMemoria) invece di provarci.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/bodgit/sevenzip"
	"github.com/klauspost/compress/zstd"
	"github.com/nwaples/rardecode/v2"
	"github.com/ulikunitz/xz/lzma"
)

// ErrMemoria: per aprire l'archivio servirebbe più memoria di quella libera.
var ErrMemoria = errors.New("serve troppa memoria per aprire questo archivio")

// sempreConsentito: fino a qui il dizionario va sempre bene (7-Zip "Ultra" usa 64 MB).
const sempreConsentito = 256 << 20

// memoriaPerProve: nei test si può fingere una RAM libera diversa (0 = quella vera).
var memoriaPerProve int64

// limiteDizionario: quanta memoria può usare un dizionario. Fino a 256 MB sempre; oltre, fino a
// metà della RAM libera (se non si sa quanta ce n'è: 1 GB).
func limiteDizionario() int64 {
	lib := memoriaLibera()
	if memoriaPerProve > 0 {
		lib = memoriaPerProve
	}
	if lib <= 0 {
		return 1 << 30
	}
	if lib/2 > sempreConsentito {
		return lib / 2
	}
	return sempreConsentito
}

// dizionarioServe: la misura del dizionario da riservare (dichiarato, ma mai più dei dati).
func dizionarioServe(dichiarato int64, dati uint64) int64 {
	d := dichiarato
	if dati > 0 && dati < 1<<62 && int64(dati) < d {
		d = int64(dati)
	}
	if d < 4096 {
		d = 4096
	}
	return d
}

func controllaDizionario(d int64) error {
	if d > limiteDizionario() {
		return fmt.Errorf("%w (%d MB)", ErrMemoria, d>>20)
	}
	return nil
}

func init() {
	sevenzip.RegisterDecompressor([]byte{0x21}, sevenzip.Decompressor(nuovoLZMA2))
	sevenzip.RegisterDecompressor([]byte{0x03, 0x01, 0x01}, sevenzip.Decompressor(nuovoLZMA))
}

type chiudiLettore struct {
	io.Reader
	c io.Closer
}

func (c *chiudiLettore) Close() error { return c.c.Close() }

// nuovoLZMA2: come quello della libreria, con il dizionario misurato sui dati.
func nuovoLZMA2(p []byte, dati uint64, readers []io.ReadCloser) (io.ReadCloser, error) {
	if len(readers) != 1 {
		return nil, errors.New("lzma2: serve un flusso solo")
	}
	if len(p) != 1 || p[0] > 40 {
		return nil, errors.New("lzma2: proprietà non valide")
	}
	dichiarato := int64(2|(int(p[0])&1)) << (p[0]/2 + 11)
	if p[0] == 40 {
		dichiarato = 1<<32 - 1
	}
	d := dizionarioServe(dichiarato, dati)
	if err := controllaDizionario(d); err != nil {
		return nil, err
	}
	cfg := lzma.Reader2Config{DictCap: int(d)}
	if err := cfg.Verify(); err != nil {
		return nil, fmt.Errorf("lzma2: %w", err)
	}
	lr, err := cfg.NewReader2(readers[0])
	if err != nil {
		return nil, fmt.Errorf("lzma2: %w", err)
	}
	return &chiudiLettore{Reader: lr, c: readers[0]}, nil
}

// nuovoLZMA: LZMA (proprietà: 1 byte lc/lp/pb + 4 byte di dizionario).
func nuovoLZMA(p []byte, dati uint64, readers []io.ReadCloser) (io.ReadCloser, error) {
	if len(readers) != 1 {
		return nil, errors.New("lzma: serve un flusso solo")
	}
	if len(p) != 5 {
		return nil, errors.New("lzma: proprietà non valide")
	}
	dichiarato := int64(binary.LittleEndian.Uint32(p[1:]))
	d := dizionarioServe(dichiarato, dati)
	if err := controllaDizionario(d); err != nil {
		return nil, err
	}
	q := append([]byte(nil), p...)
	if d < dichiarato { // dizionario ridotto ai dati: stessa decompressione, meno memoria
		binary.LittleEndian.PutUint32(q[1:], uint32(d))
	}
	h := bytes.NewBuffer(q)
	_ = binary.Write(h, binary.LittleEndian, dati)
	var r io.Reader = io.MultiReader(h, readers[0])
	if br, ok := readers[0].(io.ByteReader); ok {
		r = &lettoreMisto{b: h, br: br, mr: r}
	}
	lr, err := lzma.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("lzma: %w", err)
	}
	return &chiudiLettore{Reader: lr, c: readers[0]}, nil
}

// lettoreMisto: prima l'intestazione in memoria, poi il flusso (anche byte per byte, veloce).
type lettoreMisto struct {
	b  *bytes.Buffer
	br io.ByteReader
	mr io.Reader
}

func (m *lettoreMisto) ReadByte() (byte, error) {
	if m.b.Len() > 0 {
		return m.b.ReadByte()
	}
	return m.br.ReadByte()
}

func (m *lettoreMisto) Read(p []byte) (int, error) { return m.mr.Read(p) }

// limiteDizionarioRar e zstd: stessa regola (la libreria rar lo limita già ai dati del file).
func limiteFinestra() int64 {
	l := limiteDizionario()
	if l > 4<<30 {
		l = 4 << 30
	}
	return l
}

// erroreMemoria: errori delle librerie che vogliono dire "dizionario o finestra troppo grandi".
func erroreMemoria(err error) bool {
	return errors.Is(err, ErrMemoria) || errors.Is(err, rardecode.ErrDictionaryTooLarge) || errors.Is(err, zstd.ErrWindowSizeExceeded)
}
