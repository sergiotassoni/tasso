// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

// strumenti.go: le funzioni della vista documento chiamate dalla pagina (organizza, testo,
// immagini, filigrana, intestazione, collegamenti, oscura, proteggi, comprimi, esporta).
// Ogni modifica crea un NUOVO file in memoria: si scrive su disco solo con "Salva una copia".

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io/fs"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

func daBase64(s string) ([]byte, error) {
	if i := strings.Index(s, ","); i >= 0 && strings.HasPrefix(s, "data:") {
		s = s[i+1:]
	}
	return base64.StdEncoding.DecodeString(s)
}

// FonteAltra: un altro PDF da cui prendere pagine (Inserisci pagine).
type FonteAltra struct {
	ID string `json:"id"`
	PW string `json:"pw"`
}

func goOrganizza(id, pw string, altri []FonteAltra, seq []PaginaSeq) (VoceInfo, error) {
	var fonti [][]byte
	for i, a := range append([]FonteAltra{{ID: id, PW: pw}}, altri...) {
		v, err := prendiVoce(a.ID)
		if err != nil {
			return VoceInfo{}, perPagina(err)
		}
		b, err := v.contenuto()
		if err == nil {
			b, err = inChiaro(b, a.PW)
		}
		if err != nil {
			if i > 0 {
				return VoceInfo{}, perPagina(&ErrPezzo{Indice: i - 1, Err: err})
			}
			return VoceInfo{}, perPagina(err)
		}
		fonti = append(fonti, b)
	}
	vi, err := esegui(id, pw, func([]byte, string) ([]byte, error) { return Organizza(fonti, seq) })
	if err == nil && len(altri) > 0 {
		// pagine prese anche da altri file: il marchio del web più severo fra tutti
		var zone [][]byte
		for _, a := range append([]FonteAltra{{ID: id}}, altri...) {
			if v, e := prendiVoce(a.ID); e == nil {
				zone = append(zone, v.Zona)
			}
		}
		if nv, e := prendiVoce(vi.ID); e == nil {
			archivio.Lock()
			nv.Zona = zonaPiuSevera(append(zone, nv.Zona)...)
			archivio.Unlock()
		}
	}
	return vi, err
}

// ElementoPagina: un Elemento come lo manda la pagina (l'immagine in base64).
type ElementoPagina struct {
	Elemento
	Img64 string `json:"img"`
}

func goDisegna(id, pw string, ee []ElementoPagina) (VoceInfo, error) {
	var el []Elemento
	for _, e := range ee {
		x := e.Elemento
		if e.Img64 != "" {
			b, err := daBase64(e.Img64)
			if err != nil {
				return VoceInfo{}, perPagina(errc("immagine", err))
			}
			x.Img = b
		}
		el = append(el, x)
	}
	return esegui(id, pw, func(b []byte, pw string) ([]byte, error) { return Disegna(b, pw, el) })
}

func goCollegamenti(id, pw string, cc []Collegamento) (VoceInfo, error) {
	return esegui(id, pw, func(b []byte, pw string) ([]byte, error) { return AggiungiCollegamenti(b, pw, cc) })
}

// PaginaOscurataPagina: come la manda la pagina (immagine JPEG o PNG in base64).
type PaginaOscurataPagina struct {
	Pagina int    `json:"pagina"`
	Img64  string `json:"img"`
}

func goOscura(id, pw string, pp []PaginaOscurataPagina) (VoceInfo, error) {
	var po []PaginaOscurata
	for _, p := range pp {
		b, err := daBase64(p.Img64)
		if err != nil {
			return VoceInfo{}, perPagina(errc("immagine", err))
		}
		po = append(po, PaginaOscurata{Pagina: p.Pagina, Img: b})
	}
	return esegui(id, pw, func(b []byte, pw string) ([]byte, error) { return Oscura(b, pw, po) })
}

func goProteggi(id, pw string, o OpzProteggi) (VoceInfo, error) {
	return esegui(id, pw, func(b []byte, pw string) ([]byte, error) { return Proteggi(b, pw, o) })
}

func goComprimi(id, pw string, immagini bool) (VoceInfo, error) {
	return esegui(id, pw, func(b []byte, pw string) ([]byte, error) { return Comprimi(b, pw, immagini) })
}

// goCaricaTesto: un testo (es. quello estratto dal PDF) come file da salvare.
func goCaricaTesto(nome, testo string) VoceInfo {
	b := []byte("\ufeff" + strings.ReplaceAll(testo, "\n", "\r\n"))
	return aggiungiVoce(&Voce{Nome: nome, Dati: b, Tipo: "txt"})
}

// ---------------------------------------------------------------- serie di immagini (Esporta)

// Una serie: l'utente sceglie una volta nome e cartella ("Contratto.png"); le pagine diventano
// "Contratto - 1.png", "Contratto - 2.png"... Se un file esiste già, si aggiunge " (2)".
type serie struct {
	base, ext string
	scritti   []string
	zona      []byte // marchio del web del PDF d'origine
}

var serieAperte = struct {
	sync.Mutex
	m map[string]*serie
	n int
}{m: map[string]*serie{}}

func goSerieInizio(idOrigine, nomeSuggerito, titolo, ext string) (string, error) {
	if ext != "png" && ext != "jpg" {
		return "", perPagina(errc("generico", nil))
	}
	cartella := ""
	var zona []byte
	if v, err := prendiVoce(idOrigine); err == nil {
		cartella = v.cartella()
		zona = v.Zona
	}
	p, err := chiediDove(nomeSuggerito, cartella, titolo, ext)
	if err != nil || p == "" {
		return "", perPagina(err)
	}
	serieAperte.Lock()
	defer serieAperte.Unlock()
	serieAperte.n++
	tok := "s" + strconv.Itoa(serieAperte.n)
	serieAperte.m[tok] = &serie{base: strings.TrimSuffix(p, filepath.Ext(p)), ext: ext, zona: zona}
	return tok, nil
}

// conDPI (ZP06-7): scrive nel PNG (blocco pHYs) o nel JPEG (JFIF) la risoluzione con cui la
// pagina è stata disegnata: in Word o in stampa l'immagine esce grande come la pagina vera.
// Cambia solo qualche byte dell'intestazione: l'immagine non viene ricodificata.
func conDPI(b []byte, dpi int) []byte {
	if dpi <= 0 || dpi > 10000 {
		return b
	}
	switch {
	case len(b) > 33 && bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")) && string(b[12:16]) == "IHDR":
		if bytes.Contains(b[:min(len(b), 4096)], []byte("pHYs")) {
			return b
		}
		ppm := uint32(math.Round(float64(dpi) / 0.0254))
		ch := make([]byte, 0, 21)
		ch = binary.BigEndian.AppendUint32(ch, 9)
		ch = append(ch, "pHYs"...)
		ch = binary.BigEndian.AppendUint32(ch, ppm)
		ch = binary.BigEndian.AppendUint32(ch, ppm)
		ch = append(ch, 1) // metro
		ch = binary.BigEndian.AppendUint32(ch, crc32.ChecksumIEEE(ch[4:]))
		out := append([]byte{}, b[:33]...)
		out = append(out, ch...)
		return append(out, b[33:]...)
	case len(b) > 4 && b[0] == 0xFF && b[1] == 0xD8:
		if len(b) > 18 && b[2] == 0xFF && b[3] == 0xE0 && string(b[6:11]) == "JFIF\x00" {
			out := append([]byte{}, b...)
			out[13] = 1 // punti per pollice
			binary.BigEndian.PutUint16(out[14:], uint16(dpi))
			binary.BigEndian.PutUint16(out[16:], uint16(dpi))
			return out
		}
		app0 := []byte{0xFF, 0xE0, 0, 16, 'J', 'F', 'I', 'F', 0, 1, 1, 1, 0, 0, 0, 0, 0, 0}
		binary.BigEndian.PutUint16(app0[12:], uint16(dpi))
		binary.BigEndian.PutUint16(app0[14:], uint16(dpi))
		out := append([]byte{0xFF, 0xD8}, app0...)
		return append(out, b[2:]...)
	}
	return b
}

// goSerieScrivi: dpi = la risoluzione delle immagini (0 = non scriverla).
func goSerieScrivi(tok string, n int, b64 string, dpi int) (string, error) {
	serieAperte.Lock()
	s, ok := serieAperte.m[tok]
	serieAperte.Unlock()
	if !ok || n < 1 {
		return "", perPagina(errc("perso", nil))
	}
	b, err := daBase64(b64)
	if err != nil {
		return "", perPagina(errc("immagine", err))
	}
	b = conDPI(b, dpi)
	p := ""
	for k := 1; ; k++ {
		if k > 99 {
			return "", perPagina(errc("scrittura", nil))
		}
		p = fmt.Sprintf("%s - %d.%s", s.base, n, s.ext)
		if k > 1 {
			p = fmt.Sprintf("%s - %d (%d).%s", s.base, n, k, s.ext)
		}
		if eUnOriginale(p) {
			continue
		}
		// Crea il file solo se non c'è ancora (O_EXCL): anche se un altro programma lo crea
		// proprio in questo momento, non viene mai sovrascritto.
		err = scriviNuovo(p, b)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", perPagina(errc("scrittura", err))
		}
	}
	mettiZona(p, s.zona)
	serieAperte.Lock()
	s.scritti = append(s.scritti, p)
	serieAperte.Unlock()
	return p, nil
}

func goSerieFine(tok string) {
	serieAperte.Lock()
	delete(serieAperte.m, tok)
	serieAperte.Unlock()
}
