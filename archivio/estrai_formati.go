// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
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

// dataZip: la data del file nello zip. Se c'è solo la data MS-DOS (zip di Esplora file, di
// Python, di Info-ZIP -X…), quella è in ora locale: archive/zip la legge come UTC e i file
// uscivano spostati di 1 o 2 ore (CTRL-ZP-01, ZP01-1). Con i campi della data estesa
// (0x5455, 0x000a NTFS, 0x5855 Unix) la data di archive/zip è già giusta.
func dataZip(f *zip.File) time.Time {
	if f.Modified.IsZero() || haDataEstesa(f.Extra) {
		return f.Modified
	}
	m := f.Modified
	return time.Date(m.Year(), m.Month(), m.Day(), m.Hour(), m.Minute(), m.Second(), 0, time.Local)
}

func haDataEstesa(extra []byte) bool {
	for len(extra) >= 4 {
		tag := uint16(extra[0]) | uint16(extra[1])<<8
		n := int(extra[2]) | int(extra[3])<<8
		if tag == 0x5455 || tag == 0x000a || tag == 0x5855 {
			return true
		}
		if 4+n > len(extra) {
			return false
		}
		extra = extra[4+n:]
	}
	return false
}

func (l *lavoroEstrai) zip() error {
	r, err := zip.NewReader(l.vol, l.vol.Size())
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return l.erroreLettura(err)
	}
	// Zip con password: Tasso Zip non li apre. Se solo alcuni file hanno la password, gli altri
	// escono e quelli con la password restano fra i "non presi"; se non esce niente, lo dico.
	// Nella Verifica no: un file che non posso controllare vuol dire che non posso dire "a posto".
	var totale int64
	cifrati, aes := 0, false
	for _, f := range r.File {
		nome := decodificaNomeZip(f.Name)
		if f.Flags&1 != 0 && l.d.vuole(nome) {
			cifrati++
			aes = aes || f.Method == 99
			if l.d.prova {
				break
			}
			continue
		}
		if f.UncompressedSize64 < 1<<62 && l.d.vuole(nome) {
			totale += int64(f.UncompressedSize64)
		}
	}
	var metodo error // il primo metodo sconosciuto trovato
	erroreCifrato := ErrZipCifrato
	if aes {
		erroreCifrato = ErrZipCifratoAES
	}
	if cifrati > 0 && l.d.prova {
		return erroreCifrato
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
		case f.Flags&1 != 0:
			if err := l.d.nuovaVoce(); err != nil {
				return err
			}
			l.d.salta(nome, "con_password")
		case m&fs.ModeSymlink != 0:
			if err := l.d.nuovaVoce(); err != nil {
				return err
			}
			l.d.salta(nome, "collegamento")
		case m.IsDir() || strings.HasSuffix(f.Name, "/"):
			if err := l.d.creaCartella(nome, dataZip(f)); err != nil {
				return l.erroreLettura(err)
			}
		case !m.IsRegular() && m&fs.ModeType != 0:
			if err := l.d.nuovaVoce(); err != nil {
				return err
			}
			l.d.salta(nome, "file_speciale")
		default:
			rc, err := apriVoceZip(f)
			if err == nil {
				err = l.d.creaFileAttr(nome, rc, dataZip(f), m.Perm()&0o200 == 0)
				rc.Close()
			}
			if metodoSconosciuto(err) {
				// metodo che Tasso Zip non sa aprire: l'archivio non è rovinato, lo dico
				if l.d.prova {
					return erroreMetodo(f.Method)
				}
				if err := l.d.nuovaVoce(); err != nil {
					return err
				}
				l.d.salta(nome, "metodo")
				if metodo == nil {
					metodo = erroreMetodo(f.Method)
				}
				continue
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
	if cifrati > 0 && l.d.file == 0 {
		return erroreCifrato // non è uscito nessun file: tutti quelli scelti hanno la password
	}
	if metodo != nil && l.d.file == 0 {
		return metodo // nessun file uscito: tutti con un metodo che non conosco
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
		if !l.d.vuole(f.Name) {
			continue
		}
		// Dati cifrati senza compressione (solo AES): senza password la libreria non dà errore
		// di cifratura, esce solo una somma sbagliata ("rovinato"). Chiedo subito la password.
		if l.opz.Password == "" && f.IsEncrypted() {
			return ErrPassword
		}
		if f.UncompressedSize < 1<<62 {
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
		case !m.IsRegular() && m&fs.ModeType != 0:
			if err := l.d.nuovaVoce(); err != nil {
				return err
			}
			l.d.salta(f.Name, "file_speciale")
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
			var dati io.Reader = io.TeeReader(rc, h)
			if f.Attributes&0x400 != 0 && f.UncompressedSize >= 8 && f.UncompressedSize <= 16<<10 {
				// FILE_ATTRIBUTE_REPARSE_POINT: può essere un collegamento o una giunzione salvati
				// da 7-Zip (-snl), con dentro i dati del collegamento. Come per zip, tar e rar non
				// esce niente. (Un file di OneDrive ha lo stesso attributo ma dati veri: esce.)
				buf, e := io.ReadAll(dati)
				if e == nil && datiCollegamento(buf) {
					rc.Close()
					if err := l.d.nuovaVoce(); err != nil {
						return err
					}
					l.d.salta(f.Name, "collegamento")
					continue
				}
				dati = io.MultiReader(bytes.NewReader(buf), erroreLettore{e})
			}
			prima := l.d.file
			err = l.d.creaFileAttr(f.Name, dati, f.Modified, f.Attributes&0x01 != 0 || m.Perm()&0o200 == 0 && f.Attributes&0xf0000000 != 0)
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

// datiCollegamento: i dati sono quelli di un collegamento di Windows (REPARSE_DATA_BUFFER con
// un'etichetta Microsoft "name surrogate": collegamento simbolico, giunzione, collegamento WSL).
func datiCollegamento(b []byte) bool {
	if len(b) < 8 {
		return false
	}
	tag := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
	lung := int(b[4]) | int(b[5])<<8
	return tag&0x80000000 != 0 && tag&0x20000000 != 0 && lung == len(b)-8
}

// erroreLettore: dopo i dati già letti, l'errore della lettura (nil = fine dei dati).
type erroreLettore struct{ err error }

func (e erroreLettore) Read([]byte) (int, error) {
	if e.err != nil {
		return 0, e.err
	}
	return 0, io.EOF
}

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
			if err := l.d.creaFileAttr(h.Name, r, h.ModificationTime, h.Mode().Perm()&0o200 == 0); err != nil {
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
	testa, errP := br.Peek(1024)
	if errP != nil && errP != io.EOF && errP != bufio.ErrBufferFull {
		// Errore già all'inizio (es. intestazione del blocco xz rovinata). Va detto adesso:
		// bufio lo "consuma" e la lettura dopo sembrerebbe un file vuoto e sano.
		return l.erroreLettura(errP)
	}
	if tipo != TipoTar && !sembraTar(testa) && !(tarVuoto(testa) && nomeDaTar(percorso)) {
		// Un file solo compresso (es. "relazione.txt.gz"). Se il .gz porta dentro il nome e la
		// data del file (gzip -N, molti siti), uso quelli (ZP01-10).
		quando := time.Time{}
		if g, ok := r.(*gzMembri); ok {
			if n := nomeGz(g.gz.Header.Name); n != "" {
				nomeSingolo = n
			}
			quando = g.gz.Header.ModTime
		}
		if err := l.d.creaFile(nomeSingolo, br, quando); err != nil {
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
		case tar.TypeReg, tar.TypeRegA, tar.TypeCont, tar.TypeGNUSparse: // TypeCont: "file contiguo", un file normale
			if err := l.d.creaFileAttr(h.Name, tr, h.ModTime, h.Mode&0o200 == 0 && h.Mode != 0); err != nil {
				if !l.soloQuesto(err) {
					return l.erroreLettura(err)
				}
				l.d.fallito(h.Name, err) // il flusso dopo un errore di solito non si legge più
			}
		case tar.TypeDir:
			if err := l.d.creaCartella(h.Name, h.ModTime); err != nil {
				return l.erroreLettura(err)
			}
		case tar.TypeLink:
			// hard link (GNU tar li fa da solo per i file con più nomi): esce come copia del
			// file già uscito dallo stesso archivio, che è dentro la cartella di lavoro (ZP01-3)
			fatto, err := l.d.creaCollegamentoFisso(h.Name, h.Linkname, h.ModTime)
			if err != nil {
				if !l.soloQuesto(err) {
					return l.erroreLettura(err)
				}
				l.d.fallito(h.Name, err)
			} else if !fatto {
				if err := l.d.nuovaVoce(); err != nil {
					return err
				}
				l.d.salta(h.Name, "collegamento")
			}
		case tar.TypeSymlink:
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

// nomeGz: il nome scritto dentro il .gz, solo l'ultimo pezzo e pulito ("" = nessuno).
func nomeGz(n string) string {
	n = strings.ReplaceAll(n, `\`, "/")
	if i := strings.LastIndex(n, "/"); i >= 0 {
		n = n[i+1:]
	}
	return pulisciPezzo(n)
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
