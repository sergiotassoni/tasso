// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bodgit/sevenzip"
	"github.com/nwaples/rardecode/v2"
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
	Metodo  string         `json:"metodo"`  // metodi di compressione usati (es. "Deflate, Store")
	Solido  bool           `json:"solido"`  // 7z/rar "solido": i file sono compressi tutti insieme
}

// nomeMetodoZip: il nome del metodo di compressione di una voce zip.
func nomeMetodoZip(m uint16) string {
	switch m {
	case 0:
		return "Store"
	case 8:
		return "Deflate"
	case 9:
		return "Deflate64"
	case 12:
		return "BZip2"
	case 14:
		return "LZMA"
	case 93:
		return "Zstandard"
	case 95:
		return "XZ"
	case 98:
		return "PPMd"
	case 99:
		return "AES"
	}
	return fmt.Sprintf("#%d", m)
}

func uniscoMetodi(visti map[string]bool, ordine []string) string {
	var out []string
	for _, m := range ordine {
		if visti[m] {
			out = append(out, m)
		}
	}
	return strings.Join(out, ", ")
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
		visti := map[string]bool{}
		var ordine []string
		defer func() {
			el.Metodo = uniscoMetodi(visti, ordine)
		}()
		for _, f := range r.File {
			if n := nomeMetodoZip(f.Method); !visti[n] && !(f.Method == 0 && hasSlash(f.Name)) {
				visti[n] = true
				ordine = append(ordine, n)
			}
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
		el.Metodo = "7z"
		flussi := map[int]int{}
		for _, f := range r.File {
			if !f.Mode().IsDir() && f.UncompressedSize > 0 {
				flussi[f.Stream]++
				if flussi[f.Stream] > 1 {
					el.Solido = true
				}
			}
			v := VoceArchivio{Nome: f.Name, Cartella: f.Mode().IsDir(), Dim: int64(f.UncompressedSize), Compresso: -1, Quando: f.Modified}
			if !aggiungi(v) {
				return el, ErrAnnullato
			}
		}
	case TipoRar:
		opz := []rardecode.Option{rardecode.MaxDictionarySize(limiteFinestra())}
		if password != "" {
			opz = append(opz, rardecode.Password(password))
		}
		lista, e := rardecode.List(percorso, opz...)
		if e != nil {
			return el, l.erroreRar(e, false)
		}
		el.Metodo = "RAR"
		for _, f := range lista {
			if f.Solid {
				el.Solido = true
			}
			v := VoceArchivio{Nome: f.Name, Cartella: f.IsDir, Dim: f.UnPackedSize, Compresso: f.PackedSize, Quando: f.ModificationTime, Cifrato: f.Encrypted}
			if f.UnKnownSize {
				v.Dim = 0
			}
			if !aggiungi(v) {
				return el, ErrAnnullato
			}
		}
	case TipoTar, TipoGz, TipoBz2, TipoXz, TipoZst:
		el.Metodo = map[Tipo]string{TipoTar: "tar", TipoGz: "gzip", TipoBz2: "bzip2", TipoXz: "xz (LZMA2)", TipoZst: "Zstandard"}[tipo]
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
	r, chiudi, err := l.apriFlusso(tipo)
	if err != nil {
		return l.erroreLettura(err)
	}
	defer chiudi()
	br := bufio.NewReaderSize(r, 1<<20)
	testa, errP := br.Peek(512)
	if errP != nil && errP != io.EOF && errP != bufio.ErrBufferFull {
		// Errore già all'inizio (es. intestazione del blocco xz rovinata). Va detto adesso:
		// bufio lo "consuma" e la lettura dopo sembrerebbe un file vuoto e sano.
		return l.erroreLettura(errP)
	}
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
			// anche qui: la somma di controllo del compressore sta in fondo
			if err := l.fineFlusso(tipo, br); err != nil {
				return l.erroreLettura(err)
			}
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
