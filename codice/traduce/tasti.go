package traduce

// Tasti rapidi scelti dall'utente. Codice = modificatori<<16 | tasto virtuale di Windows
// (modificatori: 1 Alt, 2 Ctrl, 4 Maiusc).

import (
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

// tasti che i giochi usano di continuo: da soli no (con Ctrl o Alt sì)
var tastiDaGioco = map[uint32]bool{
	0x20: true, 0x0D: true, 0x1B: true, 0x09: true, 0x08: true, // spazio, invio, esc, tab, backspace
	0x25: true, 0x26: true, 0x27: true, 0x28: true, // frecce
	'W': true, 'A': true, 'S': true, 'D': true, 'E': true, 'Q': true, 'R': true, 'F': true,
	'C': true, 'X': true, 'Z': true, 'G': true, 'V': true, 'B': true, 'I': true, 'M': true,
	'1': true, '2': true, '3': true, '4': true, '5': true,
}

func controllaTasto(nome string, codice uint32, attuali map[string]uint32) error {
	if _, ok := tastiDiBase[nome]; !ok {
		return fmt.Errorf("funzione sconosciuta")
	}
	vk, mod := codice&0xFFFF, codice>>16
	if vk == 0 || vk > 0xFE || mod > 7 {
		return fmt.Errorf("questo tasto non si può usare")
	}
	if mod&3 == 0 && tastiDaGioco[vk] {
		return fmt.Errorf("%s lo usano quasi tutti i giochi: scegline un altro, o usalo con Ctrl o Alt", etichettaTasto(codice))
	}
	for k, c := range attuali {
		if k != nome && c == codice {
			return fmt.Errorf("%s lo usa già un'altra funzione", etichettaTasto(codice))
		}
	}
	return nil
}

func etichettaTasto(codice uint32) string {
	vk, mod := codice&0xFFFF, codice>>16
	var p []string
	if mod&2 != 0 {
		p = append(p, "Ctrl")
	}
	if mod&1 != 0 {
		p = append(p, "Alt")
	}
	if mod&4 != 0 {
		p = append(p, "Maiusc")
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
		if s, ok := nomi[vk]; ok {
			n = s
		} else {
			n = fmt.Sprintf("tasto %d", vk)
		}
	}
	return strings.Join(append(p, n), "+")
}

// nomeTasto: il tasto scelto adesso per una funzione, da scrivere nei messaggi (es. "F7").
func nomeTasto(funzione string) string {
	return etichettaTasto(tastiCompleti(leggiImp().Tasti)[funzione])
}

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
