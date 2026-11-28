// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

// Metodi di compressione degli zip oltre a Store e Deflate (CTRL-ZP-01, ZP01-4). Prima uno zip
// sano con un metodo sconosciuto veniva detto "rovinato". Ora:
//   - BZip2 (12), LZMA (14) e Zstandard (93) si aprono, con le librerie già in third_party e
//     con i limiti di memoria degli altri formati;
//   - gli altri (Deflate64, XZ, PPMd...) danno ErrMetodo con il nome del metodo.

import (
	"archive/zip"
	"compress/bzip2"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz/lzma"
)

func init() {
	zip.RegisterDecompressor(12, func(r io.Reader) io.ReadCloser { return io.NopCloser(bzip2.NewReader(r)) })
	zip.RegisterDecompressor(93, func(r io.Reader) io.ReadCloser {
		zr, err := zstd.NewReader(r, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxWindow(uint64(limiteFinestra())))
		if err != nil {
			return io.NopCloser(erroreLettore{err})
		}
		return zr.IOReadCloser()
	})
}

// erroreMetodo: il metodo dello zip che Tasso Zip non sa aprire.
func erroreMetodo(m uint16) error {
	return fmt.Errorf("%w: %s", ErrMetodo, nomeMetodoZip(m))
}

// metodoSconosciuto: l'errore di archive/zip (o di sevenzip) per un metodo che non c'è.
func metodoSconosciuto(err error) bool {
	return err != nil && (errors.Is(err, zip.ErrAlgorithm) || strings.Contains(err.Error(), "unsupported compression algorithm"))
}

// apriVoceZip apre i dati di una voce dello zip. LZMA (14) lo apro io: il decompressore di
// archive/zip non conosce la dimensione del file, e senza il segno di fine (facoltativo nello
// zip) la libreria lzma non saprebbe dove fermarsi.
func apriVoceZip(f *zip.File) (io.ReadCloser, error) {
	if f.Method != 14 {
		return f.Open()
	}
	raw, err := f.OpenRaw()
	if err != nil {
		return nil, err
	}
	// intestazione LZMA dello zip: versione (2 byte), lunghezza delle proprietà (2), proprietà (5)
	var testa [4]byte
	if _, err := io.ReadFull(raw, testa[:]); err != nil {
		return nil, err
	}
	np := int(binary.LittleEndian.Uint16(testa[2:]))
	if np != 5 {
		return nil, fmt.Errorf("lzma: proprietà di %d byte", np)
	}
	var classica [lzma.HeaderLen]byte // proprietà (5) + dimensione (8): il formato .lzma classico
	if _, err := io.ReadFull(raw, classica[:5]); err != nil {
		return nil, err
	}
	binary.LittleEndian.PutUint64(classica[5:], f.UncompressedSize64)
	if f.UncompressedSize64 == 0 {
		return io.NopCloser(strings.NewReader("")), nil
	}
	lim := limiteFinestra()
	if lim > 1<<31-1 {
		lim = 1<<31 - 1
	}
	lr, err := lzma.ReaderConfig{DictCap: int(lim)}.NewReader(io.MultiReader(strings.NewReader(string(classica[:])), raw))
	if err != nil {
		if strings.Contains(err.Error(), "dictionary") {
			return nil, fmt.Errorf("%w (%v)", ErrMemoria, err)
		}
		return nil, err
	}
	return &lettoreCRC{r: lr, h: crc32.NewIEEE(), crc: f.CRC32, dim: f.UncompressedSize64}, nil
}

// lettoreCRC controlla somma e dimensione alla fine dei dati (come fa archive/zip).
type lettoreCRC struct {
	r   io.Reader
	h   hash.Hash32
	crc uint32
	dim uint64
	n   uint64
}

func (l *lettoreCRC) Read(p []byte) (int, error) {
	n, err := l.r.Read(p)
	l.h.Write(p[:n])
	l.n += uint64(n)
	if l.n > l.dim {
		return n, zip.ErrFormat
	}
	if err == io.EOF {
		if l.n != l.dim {
			return n, io.ErrUnexpectedEOF
		}
		if l.crc != 0 && l.h.Sum32() != l.crc {
			return n, zip.ErrChecksum
		}
	}
	return n, err
}

func (l *lettoreCRC) Close() error { return nil }
