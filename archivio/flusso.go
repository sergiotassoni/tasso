// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

// Archivi "a flusso": tar, anche compressi (gz, bz2, xz, zst), e file singoli compressi.
//
//   - La somma di controllo dei compressori (CRC32 di gzip, CRC di bzip2 e xz, checksum di zstd)
//     sta alla FINE del flusso compresso. Il tar finisce prima (due blocchi vuoti), quindi chi
//     si fermava lì non la controllava mai: un tar.gz con un byte cambiato usciva con i dati
//     sbagliati e la Verifica diceva "OK". Ora, finito il tar, leggo il resto fino in fondo.
//   - Un .xz minuscolo può dichiarare un dizionario di 4 GB e la libreria lo riserva tutto
//     subito: prima di aprirlo controllo i dizionari dichiarati (stessa soglia del 7z).

import (
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
	"github.com/ulikunitz/xz/lzma"
)

// apriFlusso: il lettore dei dati decompressi (per il tar semplice, i dati così come sono).
// chiudi va sempre chiamata alla fine.
func (l *lavoroEstrai) apriFlusso(tipo Tipo) (r io.Reader, chiudi func(), err error) {
	chiudi = func() {}
	src := bufio.NewReaderSize(l.vol.lettoreSequenziale(), 1<<20)
	switch tipo {
	case TipoTar:
		return src, chiudi, nil
	case TipoGz:
		g, err := nuovoGzMembri(src)
		if err != nil {
			return nil, chiudi, err
		}
		return g, chiudi, nil
	case TipoBz2:
		return bzip2.NewReader(src), chiudi, nil
	case TipoXz:
		if err := controllaDizionariXz(l.vol, l.vol.Size()); err != nil {
			return nil, chiudi, err
		}
		x, err := xz.NewReader(&fineXz{r: src})
		if err != nil {
			return nil, chiudi, err
		}
		return x, chiudi, nil
	case TipoZst:
		zr, err := zstd.NewReader(src, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxWindow(uint64(limiteFinestra())))
		if err != nil {
			return nil, chiudi, err
		}
		return zr, zr.Close, nil
	}
	return nil, chiudi, ErrFormato
}

// fineFlusso: finito il tar, legge il resto del flusso compresso fino in fondo, così il
// compressore controlla la sua somma finale. Per il tar semplice non c'è niente da fare.
func (l *lavoroEstrai) fineFlusso(tipo Tipo, r io.Reader) error {
	if tipo == TipoTar {
		return nil
	}
	_, err := io.Copy(io.Discard, &lettoreAnnullabile{ctx: l.ctx, r: r})
	if err == nil || codaInPiu(err) {
		return nil
	}
	return err
}

// codaInPiu: dopo un flusso finito e controllato ci sono byte in più che non sono un altro
// flusso (zeri di riempimento di certi programmi): non sono un danno dei file.
func codaInPiu(err error) bool {
	var se bzip2.StructuralError
	return errors.As(err, &se) && string(se) == "bad magic value in continuation file"
}

// gzMembri: un .gz può essere fatto di più "membri" uno dopo l'altro. Leggo un membro alla
// volta (il CRC32 di ognuno viene controllato alla sua fine) e, quando un membro è finito,
// guardo se dopo ne inizia un altro. Byte in più che non sono un membro (zeri di riempimento)
// chiudono il flusso senza errore: il CRC dei membri veri è già stato controllato.
type gzMembri struct {
	src  *bufio.Reader
	gz   *gzip.Reader
	fine bool
}

func nuovoGzMembri(src *bufio.Reader) (*gzMembri, error) {
	gz, err := gzip.NewReader(src)
	if err != nil {
		return nil, err
	}
	gz.Multistream(false)
	return &gzMembri{src: src, gz: gz}, nil
}

func (g *gzMembri) Read(p []byte) (int, error) {
	for {
		if g.fine {
			return 0, io.EOF
		}
		n, err := g.gz.Read(p)
		if err != io.EOF {
			return n, err
		}
		// membro finito (CRC controllato): ce n'è un altro?
		if _, e := g.src.Peek(1); e != nil {
			g.fine = true
		} else if e := g.gz.Reset(g.src); e != nil {
			if !errors.Is(e, gzip.ErrHeader) && !errors.Is(e, io.ErrUnexpectedEOF) && !errors.Is(e, io.EOF) {
				return n, e
			}
			g.fine = true // byte in più dopo l'ultimo membro
		} else {
			g.gz.Multistream(false)
		}
		if n > 0 {
			return n, nil
		}
	}
}

// fineXz: la libreria xz, se il file finisce dentro l'intestazione di un blocco (o subito
// prima dell'indice), lo prende per la fine normale del flusso: un .xz tagliato a 12-23 byte
// (o senza indice) usciva vuoto o incompleto e la Verifica diceva "OK". Un flusso xz vero
// finisce sempre con il piede "YZ", seguito al massimo da zeri di riempimento: se il file
// finisce in un altro modo, la fine diventa io.ErrUnexpectedEOF ("rovinato").
type fineXz struct {
	r      io.Reader
	ultimi [2]byte // gli ultimi due byte prima degli zeri finali
	zeri   int64   // zeri alla fine di quello letto finora
	letti  int64
}

func (f *fineXz) Read(p []byte) (int, error) {
	n, err := f.r.Read(p)
	f.guarda(p[:n])
	if err == io.EOF && !f.finePulita() {
		return n, io.ErrUnexpectedEOF
	}
	return n, err
}

func (f *fineXz) guarda(b []byte) {
	f.letti += int64(len(b))
	i := len(b) - 1
	for i >= 0 && b[i] == 0 {
		i--
	}
	if i < 0 {
		f.zeri += int64(len(b))
		return
	}
	prima := f.ultimi[1]
	if f.zeri > 0 {
		prima = 0
	}
	if i >= 1 {
		prima = b[i-1]
	}
	f.ultimi = [2]byte{prima, b[i]}
	f.zeri = int64(len(b) - 1 - i)
}

func (f *fineXz) finePulita() bool {
	return f.letti == 0 || (f.ultimi == [2]byte{'Y', 'Z'} && f.zeri%4 == 0)
}

// ---------------------------------------------------------------- dizionari degli .xz

// controllaDizionariXz: legge dall'indice in fondo al file dove stanno i blocchi e, dalle
// loro intestazioni, quanto dizionario LZMA2 dichiarano. Se uno serve più memoria di quella
// che si può usare (vedi limiteDizionario) restituisce ErrMemoria, prima di riservare niente.
// Se l'indice non si legge, controllo almeno il primo blocco (il decompressore poi dirà
// "rovinato").
func controllaDizionariXz(r io.ReaderAt, dim int64) error {
	blocchi, err := blocchiXz(r, dim)
	if err != nil || len(blocchi) == 0 {
		blocchi = []int64{12} // il primo blocco, subito dopo l'intestazione del flusso
	}
	for _, off := range blocchi {
		d, ok := dizionarioBloccoXz(r, off)
		if ok {
			if err := controllaDizionario(d); err != nil {
				return err
			}
		}
	}
	return nil
}

// dizionarioBloccoXz: il dizionario dichiarato dal filtro LZMA2 (id 0x21) nell'intestazione
// del blocco che inizia a off.
func dizionarioBloccoXz(r io.ReaderAt, off int64) (int64, bool) {
	var b [1024]byte
	if _, err := r.ReadAt(b[:1], off); err != nil || b[0] == 0 {
		return 0, false
	}
	n := (int(b[0]) + 1) * 4
	if _, err := r.ReadAt(b[:n], off); err != nil {
		return 0, false
	}
	flag := b[1]
	p := 2
	if flag&0x40 != 0 { // dimensione compressa presente
		if _, k := binary.Uvarint(b[p:n]); k > 0 {
			p += k
		} else {
			return 0, false
		}
	}
	if flag&0x80 != 0 { // dimensione decompressa presente
		if _, k := binary.Uvarint(b[p:n]); k > 0 {
			p += k
		} else {
			return 0, false
		}
	}
	filtri := int(flag&3) + 1
	var max int64
	trovato := false
	for i := 0; i < filtri && p < n-4; i++ {
		id, k := binary.Uvarint(b[p:n])
		if k <= 0 {
			return max, trovato
		}
		p += k
		sz, k := binary.Uvarint(b[p:n])
		if k <= 0 || p+k+int(sz) > n {
			return max, trovato
		}
		p += k
		if id == 0x21 && sz == 1 {
			if d, err := lzma.DecodeDictCap(b[p]); err == nil && d > max {
				max, trovato = d, true
			}
		}
		p += int(sz)
	}
	return max, trovato
}

// blocchiXz: le posizioni di inizio di tutti i blocchi di tutti i flussi del file, leggendo
// dalla fine (piede del flusso -> indice -> dimensioni dei blocchi).
func blocchiXz(r io.ReaderAt, dim int64) ([]int64, error) {
	var out []int64
	fine := dim
	for giri := 0; fine > 0 && giri < 10000; giri++ {
		// riempimento fra i flussi: zeri a gruppi di 4
		var q [4]byte
		for fine >= 4 {
			if _, err := r.ReadAt(q[:], fine-4); err != nil {
				return out, err
			}
			if q != [4]byte{} {
				break
			}
			fine -= 4
		}
		if fine < 24 {
			break
		}
		var piede [12]byte
		if _, err := r.ReadAt(piede[:], fine-12); err != nil {
			return out, err
		}
		if piede[10] != 'Y' || piede[11] != 'Z' {
			return out, errors.New("xz: piede del flusso non trovato")
		}
		dimIndice := (int64(binary.LittleEndian.Uint32(piede[4:8])) + 1) * 4
		inizioIndice := fine - 12 - dimIndice
		if inizioIndice < 12 || dimIndice > 64<<20 {
			return out, errors.New("xz: indice non valido")
		}
		indice := make([]byte, dimIndice)
		if _, err := r.ReadAt(indice, inizioIndice); err != nil {
			return out, err
		}
		if indice[0] != 0 {
			return out, errors.New("xz: indice non valido")
		}
		p := 1
		quanti, k := binary.Uvarint(indice[p:])
		if k <= 0 || quanti > 1<<20 {
			return out, errors.New("xz: indice non valido")
		}
		p += k
		var dimensioni []int64
		var totale int64
		for i := uint64(0); i < quanti; i++ {
			compr, k := binary.Uvarint(indice[p:])
			if k <= 0 {
				return out, errors.New("xz: indice non valido")
			}
			p += k
			_, k = binary.Uvarint(indice[p:])
			if k <= 0 {
				return out, errors.New("xz: indice non valido")
			}
			p += k
			pad := (int64(compr) + 3) &^ 3
			if pad <= 0 || pad > dim {
				return out, errors.New("xz: indice non valido")
			}
			dimensioni = append(dimensioni, pad)
			totale += pad
		}
		inizioFlusso := inizioIndice - totale - 12
		if inizioFlusso < 0 {
			return out, errors.New("xz: indice non valido")
		}
		off := inizioFlusso + 12
		var questi []int64
		for _, d := range dimensioni {
			questi = append(questi, off)
			off += d
		}
		out = append(questi, out...)
		fine = inizioFlusso
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("xz: nessun blocco")
	}
	return out, nil
}
