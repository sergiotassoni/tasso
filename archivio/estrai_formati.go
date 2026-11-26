// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bodgit/sevenzip"
	"github.com/nwaples/rardecode/v2"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
)

// ---------------- ZIP ----------------

// decodificaNomeZip: i vecchi zip (per es. quelli fatti da Windows) scrivono i nomi con la
// tabella di caratteri OEM (437, 850...), non in UTF-8.
func decodificaNomeZip(n string) string {
	if utf8.ValidString(n) {
		return n
	}
	var enc encoding.Encoding = charmap.CodePage437
	switch codePageOEM() {
	case 850:
		enc = charmap.CodePage850
	case 852:
		enc = charmap.CodePage852
	case 858:
		enc = charmap.CodePage858
	case 860:
		enc = charmap.CodePage860
	case 863:
		enc = charmap.CodePage863
	case 865:
		enc = charmap.CodePage865
	case 866:
		enc = charmap.CodePage866
	}
	if s, err := enc.NewDecoder().String(n); err == nil {
		return s
	}
	return strings.ToValidUTF8(n, "_")
}

func (l *lavoroEstrai) zip() error {
	r, err := zip.NewReader(l.vol, l.vol.Size())
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return l.erroreLettura(err)
	}
	var totale int64
	for _, f := range r.File {
		if f.Flags&1 != 0 {
			return ErrZipCifrato
		}
		if f.UncompressedSize64 < 1<<62 && l.d.vuole(decodificaNomeZip(f.Name)) {
			totale += int64(f.UncompressedSize64)
		}
	}
	if err := l.d.controllaDichiarato(totale, len(r.File)); err != nil {
		return err
	}
	l.totale = totale
	for _, f := range r.File {
		nome := decodificaNomeZip(f.Name)
		if !l.d.vuole(nome) {
			continue
		}
		m := f.Mode()
		switch {
		case m&fs.ModeSymlink != 0:
			if err := l.d.nuovaVoce(); err != nil {
				return err
			}
			l.d.salta(nome, "collegamento")
		case m.IsDir() || strings.HasSuffix(f.Name, "/"):
			if err := l.d.creaCartella(nome, f.Modified); err != nil {
				return l.erroreLettura(err)
			}
		case !m.IsRegular() && m&fs.ModeType != 0:
			if err := l.d.nuovaVoce(); err != nil {
				return err
			}
			l.d.salta(nome, "file_speciale")
		default:
			rc, err := f.Open()
			if err == nil {
				err = l.d.creaFile(nome, rc, f.Modified)
				rc.Close()
			}
			if l.soloQuesto(err) {
				l.d.fallito(nome, err) // dati rovinati: salto questo file e vado avanti
				continue
			}
			if err != nil {
				return l.erroreLettura(err)
			}
		}
	}
	return nil
}

// ---------------- 7z ----------------

func (l *lavoroEstrai) errore7z(err error) error {
	var re *sevenzip.ReadError
	if errors.As(err, &re) && re.Encrypted {
		if l.opz.Password == "" {
			return ErrPassword
		}
		return ErrPasswordSbagliata
	}
	if strings.Contains(err.Error(), "no password") {
		return ErrPassword
	}
	return l.erroreLettura(err)
}

func (l *lavoroEstrai) setteZip() error {
	r, err := sevenzip.NewReaderWithPassword(l.vol, l.vol.Size(), l.opz.Password)
	if err != nil {
		return l.errore7z(err)
	}
	var totale int64
	for _, f := range r.File {
		if f.UncompressedSize < 1<<62 && l.d.vuole(f.Name) {
			totale += int64(f.UncompressedSize)
		}
	}
	if err := l.d.controllaDichiarato(totale, len(r.File)); err != nil {
		return err
	}
	l.totale = totale
	for _, f := range r.File {
		if !l.d.vuole(f.Name) {
			continue
		}
		m := f.Mode()
		switch {
		case m&fs.ModeSymlink != 0:
			if err := l.d.nuovaVoce(); err != nil {
				return err
			}
			l.d.salta(f.Name, "collegamento")
		case m.IsDir():
			if err := l.d.creaCartella(f.Name, f.Modified); err != nil {
				return l.errore7z(err)
			}
		default:
			rc, err := f.Open()
			if err != nil {
				if e := l.errore7z(err); !l.soloQuesto(err) || l.cifratoSbagliato(e) {
					return l.comePassword(e)
				}
				l.d.fallito(f.Name, err)
				continue
			}
			h := crc32.NewIEEE()
			prima := l.d.file
			err = l.d.creaFile(f.Name, io.TeeReader(rc, h), f.Modified)
			rc.Close()
			if err == nil && f.CRC32 != 0 && f.UncompressedSize > 0 && h.Sum32() != f.CRC32 {
				if l.d.file > prima {
					l.d.togliUltimo()
				}
				err = fmt.Errorf("%s: %w", f.Name, errDatiNonCorretti)
			}
			if err != nil {
				e := l.errore7z(err)
				if l.cifratoSbagliato(e) || !l.soloQuesto(err) {
					return l.comePassword(e)
				}
				l.d.fallito(f.Name, err)
				continue
			}
			if l.opz.Password != "" {
				l.cifratiBuoni++
			}
		}
	}
	return nil
}

var errDatiNonCorretti = errors.New("dati non corretti")

// comePassword: un errore dei dati con la password e nessun file giusto = password sbagliata.
func (l *lavoroEstrai) comePassword(e error) error {
	if errors.Is(e, ErrPassword) || errors.Is(e, ErrPasswordSbagliata) || !l.soloQuesto(e) {
		return e
	}
	if l.opz.Password != "" && l.cifratiBuoni == 0 {
		return ErrPasswordSbagliata
	}
	return e
}

// cifratoSbagliato: con la password, se nessun file è ancora uscito giusto, un errore dei dati
// vuol dire quasi sempre password sbagliata (non "file rovinato").
func (l *lavoroEstrai) cifratoSbagliato(e error) bool {
	if errors.Is(e, ErrPassword) || errors.Is(e, ErrPasswordSbagliata) {
		return true
	}
	return l.opz.Password != "" && l.cifratiBuoni == 0 && l.soloQuesto(e)
}

// ---------------- RAR ----------------

func (l *lavoroEstrai) erroreRar(err error, cifrato bool) error {
	switch {
	case errors.Is(err, rardecode.ErrArchiveEncrypted), errors.Is(err, rardecode.ErrArchivedFileEncrypted):
		if l.opz.Password == "" {
			return ErrPassword
		}
		return ErrPasswordSbagliata
	case errors.Is(err, rardecode.ErrBadPassword):
		return ErrPasswordSbagliata
	case errors.Is(err, rardecode.ErrMultiVolume), errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("%w (%v)", ErrParteMancante, err)
	}
	if cifrato && l.opz.Password != "" {
		var pe *fs.PathError
		if !errors.As(err, &pe) && !errors.Is(err, ErrAnnullato) && !errors.Is(err, ErrBomba) && !errors.Is(err, ErrSpazio) {
			return ErrPasswordSbagliata // RAR vecchi: con la password sbagliata i dati escono rovinati
		}
	}
	return l.erroreLettura(err)
}

func (l *lavoroEstrai) rar(percorso string) error {
	opz := []rardecode.Option{rardecode.MaxDictionarySize(limiteFinestra())}
	if l.opz.Password != "" {
		opz = append(opz, rardecode.Password(l.opz.Password))
	}
	// Prima la lista (dimensioni per la barra, controllo password e parti).
	lista, err := rardecode.List(percorso, opz...)
	if err != nil {
		return l.erroreRar(err, false)
	}
	var totale int64
	for _, f := range lista {
		if f.Encrypted && l.opz.Password == "" {
			return ErrPassword
		}
		if !f.UnKnownSize && f.UnPackedSize > 0 && f.UnPackedSize < 1<<62 {
			totale += f.UnPackedSize
		}
	}
	if err := l.d.controllaDichiarato(totale, len(lista)); err != nil {
		return err
	}
	l.totale = totale

	r, err := rardecode.OpenReader(percorso, opz...)
	if err != nil {
		return l.erroreRar(err, false)
	}
	defer r.Close()
	for {
		h, err := r.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			if e := l.erroreRar(err, false); !l.soloQuesto(e) {
				return e
			}
			return l.resto(l.erroreRar(err, false))
		}
		switch {
		case h.LinkType != rardecode.LinkTypeNone || h.Mode()&fs.ModeSymlink != 0:
			if err := l.d.nuovaVoce(); err != nil {
				return err
			}
			l.d.salta(h.Name, "collegamento")
		case h.IsDir:
			if err := l.d.creaCartella(h.Name, h.ModificationTime); err != nil {
				return l.erroreRar(err, false)
			}
		default:
			if h.Encrypted && l.opz.Password == "" {
				return ErrPassword
			}
			if err := l.d.creaFile(h.Name, r, h.ModificationTime); err != nil {
				e := l.erroreRar(err, h.Encrypted && l.cifratiBuoni == 0)
				if !l.soloQuesto(e) {
					return e
				}
				l.d.fallito(h.Name, err)
				continue
			}
			if h.Encrypted {
				l.cifratiBuoni++
			}
		}
	}
}

// ---------------- TAR, GZ, BZ2, XZ ----------------

func (l *lavoroEstrai) flussoTar(tipo Tipo, percorso string) error {
	r, chiudi, err := l.apriFlusso(tipo)
	if err != nil {
		return l.erroreLettura(err)
	}
	defer chiudi()
	nomeSingolo := ""
	if tipo != TipoTar {
		nomeSingolo = nomeDentro(percorso)
	}
	br := bufio.NewReaderSize(r, 1<<20)
	testa, errP := br.Peek(512)
	if errP != nil && errP != io.EOF && errP != bufio.ErrBufferFull {
		// Errore già all'inizio (es. intestazione del blocco xz rovinata). Va detto adesso:
		// bufio lo "consuma" e la lettura dopo sembrerebbe un file vuoto e sano.
		return l.erroreLettura(errP)
	}
	if tipo != TipoTar && !sembraTar(testa) {
		// Un file solo compresso (es. "relazione.txt.gz").
		if err := l.d.creaFile(nomeSingolo, br, time.Time{}); err != nil {
			return l.erroreLettura(err)
		}
		return nil
	}
	tr := tar.NewReader(br)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			// Il tar è finito, ma la somma di controllo del compressore sta in fondo al flusso.
			if err := l.fineFlusso(tipo, br); err != nil {
				return l.sommaFinale(l.erroreLettura(err))
			}
			return nil
		}
		if err != nil {
			return l.resto(l.erroreLettura(err))
		}
		switch h.Typeflag {
		case tar.TypeReg, tar.TypeRegA, tar.TypeGNUSparse:
			if err := l.d.creaFile(h.Name, tr, h.ModTime); err != nil {
				if !l.soloQuesto(err) {
					return l.erroreLettura(err)
				}
				l.d.fallito(h.Name, err) // il flusso dopo un errore di solito non si legge più
			}
		case tar.TypeDir:
			if err := l.d.creaCartella(h.Name, h.ModTime); err != nil {
				return l.erroreLettura(err)
			}
		case tar.TypeSymlink, tar.TypeLink:
			if err := l.d.nuovaVoce(); err != nil {
				return err
			}
			l.d.salta(h.Name, "collegamento")
		case tar.TypeXGlobalHeader, tar.TypeXHeader, tar.TypeGNULongName, tar.TypeGNULongLink:
			// intestazioni di servizio: niente da estrarre
		default:
			if err := l.d.nuovaVoce(); err != nil {
				return err
			}
			l.d.salta(h.Name, "file_speciale")
		}
	}
}

// nomeDentro: il nome del file dentro "relazione.txt.gz" è "relazione.txt".
func nomeDentro(percorso string) string {
	n := filepath.Base(percorso)
	if m := reNumero3.FindStringSubmatch(n); m != nil {
		n = m[1]
	}
	l := strings.ToLower(n)
	for _, e := range []string{".gz", ".bz2", ".xz", ".zst"} {
		if strings.HasSuffix(l, e) && len(n) > len(e) {
			return n[:len(n)-len(e)]
		}
	}
	if b, _ := DividiNome(n, false); b != "" {
		return b
	}
	return "file"
}
