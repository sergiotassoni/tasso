// Package sfondi: gli sfondi del desktop abbinati ai temi di Tasso (3.2).
//
// Qui c'è solo la logica pura (quale sfondo va con quale tema, quando salvare lo sfondo
// dell'utente, cosa rimettere), così i test girano anche su Linux: go test ./sfondi/
// Le miniature 480x270 sono dentro l'exe; gli sfondi 4K (cartella 4k, non incorporata) li
// mette l'installatore in Programmi\Tasso\sfondi\.
package sfondi

import (
	"embed"
	"encoding/base64"
	"path/filepath"
	"strings"
)

//go:embed mini/*.jpg
var miniFS embed.FS

// Nomi italiani degli sfondi (l'interfaccia li traduce).
var nomi = map[string]string{
	"gaming":   "Gaming",
	"neonrgb":  "Neon RGB",
	"oro":      "Oro",
	"argento":  "Argento",
	"nebbia":   "Nebbia",
	"blunotte": "Blu notte",
	"sabbia":   "Sabbia",
}

// Temi di Gaming con uno sfondo che gli somiglia davvero (gli altri: nessuna domanda).
var perGaming = map[string]string{
	"tasso":    "gaming",  // scuro, occhi verdi: lo sfondo Gaming
	"neonrgb":  "neonrgb", // luci RGB
	"neon":     "neonrgb", // fucsia su viola: lo stesso tramonto al neon
	"oro":      "oro",
	"argento":  "argento",
	"carbonio": "argento", // grigio scuro metallico
	"ghiaccio": "nebbia",  // chiaro e azzurro
	"sabbia":   "sabbia",
}

// Temi di Ufficio.
var perUfficio = map[string]string{
	"ufficio": "nebbia",   // Azzurro tenue: grigio-azzurro chiaro
	"perla":   "nebbia",   // grigio perla
	"ardesia": "blunotte", // blu calmo scuro
	"carta":   "sabbia",   // caldo e chiaro
}

// Tutti: gli ID di tutti gli sfondi, in ordine fisso.
func Tutti() []string {
	return []string{"gaming", "neonrgb", "oro", "argento", "nebbia", "blunotte", "sabbia"}
}

// PerTema: lo sfondo abbinato al tema ("" se non ce n'è uno che ha senso).
func PerTema(tema string, ufficio bool) string {
	if ufficio {
		return perUfficio[tema]
	}
	return perGaming[tema]
}

// Nome: il nome italiano dello sfondo ("" se l'ID non esiste).
func Nome(id string) string { return nomi[id] }

// Valido: l'ID è uno degli sfondi di Tasso.
func Valido(id string) bool { _, ok := nomi[id]; return ok }

// File: il nome del file 4K dello sfondo (lo stesso in Programmi\Tasso\sfondi e nella cartella dati).
func File(id string) string {
	if !Valido(id) {
		return ""
	}
	return "tasso-" + id + ".jpg"
}

// Mini: la miniatura JPEG 480x270 (nil se l'ID non esiste).
func Mini(id string) []byte {
	if !Valido(id) {
		return nil
	}
	b, err := miniFS.ReadFile("mini/" + File(id))
	if err != nil {
		return nil
	}
	return b
}

// MiniDataURL: la miniatura pronta per un <img src="..."> ("" se non c'è).
func MiniDataURL(id string) string {
	b := Mini(id)
	if b == nil {
		return ""
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b)
}

// norm: percorso Windows confrontabile (barre \, minuscole, senza \ in fondo).
func norm(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "/", `\`)
	p = strings.TrimPrefix(p, `\\?\`)
	return strings.TrimRight(strings.ToLower(p), `\`)
}

// RadiceOk: una cartella usabile come radice "sicura": percorso locale assoluto, non la radice
// di un disco (C:\ andrebbe bene per tutto) e senza "..".
func RadiceOk(base string) bool {
	nb := norm(base)
	return len(nb) > 3 && Locale(nb+`\`) && !strings.Contains(nb, `..`)
}

// Sotto: p è dentro la cartella base (o è base stessa). Confronto alla Windows, senza ".." .
// Una radice di disco (C:\) o non locale non vale mai come base.
func Sotto(p, base string) bool {
	np, nb := norm(p), norm(base)
	if np == "" || !RadiceOk(base) || strings.Contains(np, `\..`) || strings.Contains(np, `..\`) {
		return false
	}
	return np == nb || strings.HasPrefix(np, nb+`\`)
}

// DiTasso: lo sfondo attuale è uno di quelli messi da Tasso (nella cartella dati o in Programmi).
// Quelli non vanno mai salvati come "il tuo sfondo".
func DiTasso(attuale string, cartelle ...string) bool {
	if attuale == "" {
		return false
	}
	for _, c := range cartelle {
		if c != "" && Sotto(attuale, c) {
			n := strings.ToLower(filepath.Base(strings.ReplaceAll(attuale, `\`, "/")))
			if strings.HasPrefix(n, "tasso-") && strings.HasSuffix(n, ".jpg") {
				return true
			}
		}
	}
	return false
}

// DaSalvare: prima di mettere uno sfondo di Tasso, va salvato quello attuale? Sì, a meno che
// quello attuale sia già uno di Tasso (es. si passa da Oro ad Argento): in quel caso resta
// salvato il tuo di prima, che è quello da rimettere.
func DaSalvare(attuale string, cartelleTasso ...string) bool {
	return !DiTasso(attuale, cartelleTasso...)
}

// ImmagineOk: estensione di un'immagine che Windows sa mettere come sfondo.
func ImmagineOk(p string) bool {
	switch strings.ToLower(filepath.Ext(strings.ReplaceAll(p, `\`, "/"))) {
	case ".jpg", ".jpeg", ".jfif", ".jpe", ".png", ".bmp", ".dib", ".gif", ".tif", ".tiff", ".wdp", ".heic", ".webp":
		return true
	}
	return false
}

// Locale: percorso assoluto su un disco del PC (C:\...), non di rete.
func Locale(p string) bool {
	p = strings.TrimPrefix(p, `\\?\`)
	if len(p) < 4 || p[1] != ':' || (p[2] != '\\' && p[2] != '/') {
		return false
	}
	c := p[0] | 0x20
	return c >= 'a' && c <= 'z'
}

// Ripristino: cosa rimettere con "Rimetti il mio".
//
//	salvato: lo sfondo dell'utente com'era ("" = nessuna immagine, solo il colore);
//	copia:   la copia di quell'immagine tenuta da Tasso nella sua cartella dati;
//	usabile: dice se un file c'è ed è sicuro da usare.
//
// Restituisce il percorso da mettere ("" = nessuna immagine) e ok=false se non c'è niente di
// sicuro da rimettere (allora Tasso lo dice e non tocca niente).
func Ripristino(salvato, copia string, usabile func(string) bool) (string, bool) {
	if salvato == "" {
		return "", true
	}
	if Locale(salvato) && ImmagineOk(salvato) && usabile(salvato) {
		return salvato, true
	}
	if copia != "" && usabile(copia) {
		return copia, true
	}
	return "", false
}

// EstensioneDa: l'estensione giusta per un'immagine, guardando i primi byte (TranscodedWallpaper
// di Windows non ha estensione e può essere JPEG, PNG, BMP o GIF). "" se non la riconosco.
func EstensioneDa(b []byte) string {
	switch {
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return ".jpg"
	case len(b) >= 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n":
		return ".png"
	case len(b) >= 6 && (string(b[:6]) == "GIF87a" || string(b[:6]) == "GIF89a"):
		return ".gif"
	case len(b) >= 14 && b[0] == 'B' && b[1] == 'M':
		return ".bmp"
	}
	return ""
}

// EstensioniCopia: le estensioni possibili della copia dello sfondo dell'utente.
func EstensioniCopia() []string { return []string{".jpg", ".png", ".bmp", ".gif"} }

// Messo: lo sfondo di Tasso registrato (registrato) è davvero quello che Windows mostra adesso
// (attuale)? Se l'utente l'ha cambiato da Windows, no: "" (come se Tasso non l'avesse messo).
func Messo(registrato, attuale string, cartelle ...string) string {
	if registrato == "" || !Valido(registrato) || !DiTasso(attuale, cartelle...) {
		return ""
	}
	if !strings.EqualFold(filepath.Base(strings.ReplaceAll(attuale, `\`, "/")), File(registrato)) {
		return ""
	}
	return registrato
}

// DaChiedere: la finestrella "Vuoi anche lo sfondo abbinato?" si mostra solo se c'è uno
// sfondo per il tema, non è già quello messo, l'utente non ha detto "Non chiedermelo più" e
// per quello sfondo non gliel'ho già chiesto (una volta sola per sfondo).
func DaChiedere(id, giàMesso string, maiPiù bool, chiesti []string) bool {
	if id == "" || !Valido(id) || maiPiù || id == giàMesso {
		return false
	}
	for _, c := range chiesti {
		if c == id {
			return false
		}
	}
	return true
}
