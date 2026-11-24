package traduce

// Tasti rapidi scelti dall'utente. Codice = modificatori<<16 | tasto virtuale di Windows
// (modificatori: 1 Alt, 2 Ctrl, 4 Maiusc, 8 Win: gli stessi numeri di RegisterHotKey).

import (
	"errors"
	"fmt"
	"strings"
)

var tastiDiBase = map[string]uint32{
	// Dentro Tasso F6 è il tasto dell'autoclick: la traduzione dello schermo usa F7 e "mostra anche
	// l'inglese" passa a Ctrl+Alt+I (Ctrl+Alt+E no: sulla tastiera italiana è AltGr+E, cioè €).
	"pagina": 0x76, "zona": 0x78, "traduci": 0x79, "auto": 0x7A, "inglese": 3<<16 | 'I', "cronologia": 0x77,
	"chat": 3<<16 | 'T', // Ctrl+Alt+T: appunti in italiano -> appunti in inglese
	"voci": 3<<16 | 'V', // Ctrl+Alt+V (Ctrl+Maiusc+V e Ctrl+Maiusc+T servono già a browser e Discord): sottotitoli delle voci sì/no
}

// AltriTasti: i tasti rapidi di Tasso fuori dalla traduzione (autoclick, pannellino in gioco,
// scorciatoia del portachiavi), codice -> nome della funzione. Lo imposta Tasso (codice/): così
// due funzioni di Tasso non prendono mai la stessa combinazione.
var AltriTasti func() map[uint32]string

// TastoVietato: perché questa combinazione non si può usare ("" = si può). Codice come sopra,
// con 8 = tasto Windows. Vale per tutti i tasti rapidi di Tasso.
func TastoVietato(codice uint32) string {
	vk, mod := codice&0xFFFF, codice>>16
	switch {
	case vk == 0 || vk > 0xFE || mod > 15:
		return "Questo tasto non si può usare: scegline un altro."
	case vk >= 0x10 && vk <= 0x12, vk == 0x5B, vk == 0x5C, vk >= 0xA0 && vk <= 0xA5, vk == 0x14, vk == 0x90:
		return "Premi anche un tasto vero insieme a Ctrl, Alt, Maiusc o Win (per esempio Ctrl+Alt+K)."
	case vk == 0x1B:
		return "Esc serve ai giochi per aprire e chiudere i menu: scegline un altro."
	case mod&3 == 3 && (vk == 0x2E || vk == 0x6E):
		return "Ctrl+Alt+Canc è di Windows: scegline un'altra."
	case mod&8 != 0 && vk == 'L':
		return "Win+L blocca il PC: scegline un'altra."
	case mod&1 != 0 && (vk == 0x09 || vk == 0x73):
		return etichettaTasto(codice) + " è di Windows (cambia finestra o chiude il programma): scegline un'altra."
	case mod&3 == 3 && (vk == 'E' || vk == 0xBA || vk == 0xBB || vk == 0xC0 || vk == 0xDE):
		return etichettaTasto(codice) + " serve per scrivere € @ # [ ] sulla tastiera italiana: scegline un'altra."
	}
	if mod&0xB == 0 { // senza Ctrl, Alt o Win (al massimo con Maiusc)
		singolo := (vk >= 0x70 && vk <= 0x87) || vk == 0x13 || vk == 0x91 || vk == 0x2D // F1-F24, Pausa, Bloc Scorr, Ins
		if !singolo {
			return etichettaTasto(codice) + " ti serve per giocare e per scrivere: usa un tasto F (F1–F24) oppure aggiungi Ctrl, Alt o Win."
		}
	}
	return ""
}

// TastoGiaUsato: quale funzione di Tasso usa già questa combinazione, tranne "salvo"
// (il nome di una funzione della traduzione, o il nome mostrato di un altro tasto). "" = libera.
func TastoGiaUsato(codice uint32, attuali map[string]uint32, salvo string) string {
	for k, c := range attuali {
		if k != salvo && c == codice {
			return nomeFunzioneTasto(k)
		}
	}
	if AltriTasti != nil {
		for c, n := range AltriTasti() {
			if c == codice && n != salvo {
				return n
			}
		}
	}
	return ""
}

// nomeFunzioneTasto: il nome della funzione come lo vede l'utente nella pagina Giochi.
func nomeFunzioneTasto(k string) string {
	n := map[string]string{"pagina": "Mostra/nascondi le scritte in italiano", "cronologia": "Cronologia",
		"zona": "Traduci solo una zona", "traduci": "Ritraduci adesso", "auto": "Zona automatica",
		"inglese": "Mostra anche l'inglese", "voci": "Voci (sottotitoli)", "chat": "Chat tradotta"}[k]
	if n == "" {
		return k
	}
	return n
}

func controllaTasto(nome string, codice uint32, attuali map[string]uint32) error {
	if _, ok := tastiDiBase[nome]; !ok {
		return fmt.Errorf("funzione sconosciuta")
	}
	if m := TastoVietato(codice); m != "" {
		return errors.New(m)
	}
	if f := TastoGiaUsato(codice, attuali, nome); f != "" {
		return fmt.Errorf("%s lo usa già «%s»: scegline un altro", etichettaTasto(codice), f)
	}
	return nil
}

// EtichettaTasto: il nome della combinazione ("Ctrl+Alt+I"), per Tasso.
func EtichettaTasto(codice uint32) string { return etichettaTasto(codice) }

func etichettaTasto(codice uint32) string {
	vk, mod := codice&0xFFFF, codice>>16
	var p []string
	if mod&8 != 0 {
		p = append(p, "Win")
	}
	if mod&2 != 0 {
		p = append(p, "Ctrl")
	}
	if mod&1 != 0 {
		p = append(p, "Alt")
	}
	if mod&4 != 0 {
		p = append(p, testoTasso("Maiusc", "Shift"))
	}
	var n string
	switch {
	case vk >= 0x70 && vk <= 0x87:
		n = fmt.Sprintf("F%d", vk-0x6F)
	case (vk >= '0' && vk <= '9') || (vk >= 'A' && vk <= 'Z'):
		n = string(rune(vk))
	case vk >= 0x60 && vk <= 0x69:
		n = fmt.Sprintf("Num %d", vk-0x60)
	default:
		nomi := map[uint32]string{0x2D: "Ins", 0x2E: "Canc", 0x24: "Home", 0x23: "Fine", 0x21: "Pag su", 0x22: "Pag giù",
			0x13: "Pausa", 0x91: "Bloc Scorr", 0xC0: "ò", 0xDE: "à", 0xBA: "è", 0xBF: "ù", 0xDC: "\\", 0xBB: "+", 0xBD: "-",
			0xBC: ",", 0xBE: ".", 0x20: "Spazio", 0x0D: "Invio", 0x09: "Tab"}
		if s, ok := nomiTastiInglesi[vk]; ok && inIngleseTasso() {
			n = s // solo il nome mostrato: il tasto si salva sempre col suo codice
		} else if s, ok := nomi[vk]; ok {
			n = s
		} else {
			n = fmt.Sprintf(testoTasso("tasto %d", "key %d"), vk)
		}
	}
	return strings.Join(append(p, n), "+")
}

// nomiTastiInglesi: i nomi dei tasti che in inglese si chiamano diversamente.
var nomiTastiInglesi = map[uint32]string{0x2E: "Del", 0x23: "End", 0x21: "PgUp", 0x22: "PgDn",
	0x13: "Pause", 0x91: "Scroll Lock", 0x20: "Space", 0x0D: "Enter"}

func tastiCompleti(t map[string]uint32) map[string]uint32 {
	out := map[string]uint32{}
	for k, v := range tastiDiBase {
		out[k] = v
	}
	for k, v := range t {
		if _, ok := tastiDiBase[k]; ok && v != 0 {
			out[k] = v
		}
	}
	return out
}
