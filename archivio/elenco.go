// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/bodgit/sevenzip"
	"github.com/klauspost/compress/zstd"
	"github.com/nwaples/rardecode/v2"
	"github.com/ulikunitz/xz"
)

// VoceArchivio: una voce dell'archivio, come la mostra la vista "Apri con Tasso Zip".
type VoceArchivio struct {
	Nome      string    `json:"nome"` // con "/" (vedi NormalizzaNome)
	Cartella  bool      `json:"cartella"`
	Dim       int64     `json:"dim"`
	Compresso int64     `json:"compresso"` // -1 se non si sa (7z solido, tar compressi)
	Quando    time.Time `json:"quando"`
	Cifrato   bool      `json:"cifrato"`
}

// ElencoArchivio: cosa c'è dentro un archivio.
type ElencoArchivio struct {
	Tipo    Tipo           `json:"tipo"`
	Dim     int64          `json:"dim"` // dimensione dell'archivio (tutte le parti)
	Voci    []VoceArchivio `json:"voci"`
	Cifrato bool           `json:"cifrato"` // almeno una voce con password
	Saltate int            `json:"saltate"` // voci troppo numerose (oltre il limite) non elencate
}

// MaxVociElenco: oltre, l'elenco si ferma (la vista resta veloce; l'estrazione le prende tutte).
const MaxVociElenco = 200_000

// Elenca legge l'elenco delle voci senza estrarre niente. Per i tar compressi (.tar.gz,
// .tar.xz...) deve leggere tutto l'archivio (lo fa anche 7-Zip).
func Elenca(ctx context.Context, percorso, password string) (el ElencoArchivio, err error) {
	percorso, err = filepath.Abs(percorso)
	if err != nil {
		return el, err
	}
	fi, err := os.Stat(percorso)
	if err != nil {
		return el, err
	}
	if !fi.Mode().IsRegular() {
		return el, ErrFormato
	}
	l := &lavoroEstrai{ctx: ctx, opz: OpzioniEstrai{Password: password}}
	percorso, tipo, err := l.apri(percorso)
	if err != nil {
		return el, err
	}
	defer l.vol.Close()
	el.Tipo, el.Dim = tipo, l.vol.Size()
	aggiungi := func(v VoceArchivio) bool {
		if len(el.Voci) >= MaxVociElenco {
			el.Saltate++
			return ctx.Err() == nil
		}
		v.Nome = NormalizzaNome(v.Nome)
		if v.Nome == "" {
			return true
		}
		if v.Cifrato {
			el.Cifrato = true
		}
		el.Voci = append(el.Voci, v)
		return ctx.Err() == nil
	}

	switch tipo {
	case TipoZip:
		r, e := zip.NewReader(l.vol, l.vol.Size())
		if e != nil && !errors.Is(e, zip.ErrInsecurePath) {
			return el, l.erroreLettura(e)
		}
		for _, f := range r.File {
			m := f.Mode()
			v := VoceArchivio{Nome: decodificaNomeZip(f.Name), Cartella: m.IsDir() || hasSlash(f.Name),
				Dim: int64(f.UncompressedSize64), Compresso: int64(f.CompressedSize64), Quando: f.Modified, Cifrato: f.Flags&1 != 0}
			if !aggiungi(v) {
				return el, ErrAnnullato
			}
		}
	case Tipo7z:
		r, e := sevenzip.NewReaderWithPassword(l.vol, l.vol.Size(), password)
		if e != nil {
			return el, l.errore7z(e)
		}
		for _, f := range r.File {
			v := VoceArchivio{Nome: f.Name, Cartella: f.Mode().IsDir(), Dim: int64(f.UncompressedSize), Compresso: -1, Quando: f.Modified}
			if !aggiungi(v) {
				return el, ErrAnnullato
			}
		}
	case TipoRar:
		opz := []rardecode.Option{rardecode.MaxDictionarySize(1 << 30)}
		if password != "" {
			opz = append(opz, rardecode.Password(password))
		}
		lista, e := rardecode.List(percorso, opz...)
		if e != nil {
			return el, l.erroreRar(e, false)
		}
		for _, f := range lista {
			v := VoceArchivio{Nome: f.Name, Cartella: f.IsDir, Dim: f.UnPackedSize, Compresso: f.PackedSize, Quando: f.ModificationTime, Cifrato: f.Encrypted}
			if f.UnKnownSize {
				v.Dim = 0
			}
			if !aggiungi(v) {
				return el, ErrAnnullato
			}
		}
	case TipoTar, TipoGz, TipoBz2, TipoXz, TipoZst:
		if e := l.elencaFlusso(tipo, percorso, aggiungi); e != nil {
			return el, e
		}
	default:
		return el, ErrFormato
	}
	if ctx.Err() != nil {
		return el, ErrAnnullato
	}
	return el, nil
}

func hasSlash(n string) bool { return len(n) > 0 && (n[len(n)-1] == '/' || n[len(n)-1] == '\\') }

// elencaFlusso: tar, anche compressi (gz, bz2, xz, zst), e file singoli compressi.
func (l *lavoroEstrai) elencaFlusso(tipo Tipo, percorso string, aggiungi func(VoceArchivio) bool) error {
	var r io.Reader = bufio.NewReaderSize(l.vol.lettoreSequenziale(), 1<<20)
	if tipo != TipoTar {
		var err error
		switch tipo {
		case TipoGz:
			var gz *gzip.Reader
			gz, err = gzip.NewReader(r)
			if err == nil {
				r = gz
			}
		case TipoBz2:
			r = bzip2.NewReader(r)
		case TipoXz:
			r, err = xz.NewReader(r)
		case TipoZst:
			var zr *zstd.Decoder
			zr, err = zstd.NewReader(r, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxWindow(1<<30))
			if err == nil {
				defer zr.Close()
				r = zr
			}
		}
		if err != nil {
			return l.erroreLettura(err)
		}
	}
	br := bufio.NewReaderSize(r, 1<<20)
	testa, _ := br.Peek(512)
	if tipo != TipoTar && !sembraTar(testa) {
		// Un file solo compresso: la dimensione si sa solo leggendolo tutto.
		n, err := io.Copy(io.Discard, &lettoreAnnullabile{ctx: l.ctx, r: br})
		if err != nil {
			return l.erroreLettura(err)
		}
		aggiungi(VoceArchivio{Nome: nomeDentro(percorso), Dim: n, Compresso: l.vol.Size()})
		return nil
	}
	tr := tar.NewReader(br)
	for {
		if l.ctx.Err() != nil {
			return ErrAnnullato
		}
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return l.erroreLettura(err)
		}
		switch h.Typeflag {
		case tar.TypeReg, tar.TypeRegA, tar.TypeGNUSparse:
			if !aggiungi(VoceArchivio{Nome: h.Name, Dim: h.Size, Compresso: -1, Quando: h.ModTime}) {
				return ErrAnnullato
			}
		case tar.TypeDir:
			if !aggiungi(VoceArchivio{Nome: h.Name, Cartella: true, Compresso: -1, Quando: h.ModTime}) {
				return ErrAnnullato
			}
		}
	}
}

type lettoreAnnullabile struct {
	ctx context.Context
	r   io.Reader
}

func (a *lettoreAnnullabile) Read(p []byte) (int, error) {
	if a.ctx.Err() != nil {
		return 0, ErrAnnullato
	}
	return a.r.Read(p)
}
