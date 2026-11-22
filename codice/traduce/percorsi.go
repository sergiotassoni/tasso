package traduce

import "path/filepath"

// dirApp: %LOCALAPPDATA%\TassoTraduce (impostata all'avvio)
var dirApp string

// fotoVisibili: solo nella modalità foto dell'exe di collaudo le finestre sopra il gioco si
// lasciano fotografare; di solito sono escluse dalle catture per non rileggere le proprie scritte.
var fotoVisibili bool

func affinita() uintptr {
	if fotoVisibili {
		return 0
	}
	return 0x11 // WDA_EXCLUDEFROMCAPTURE
}

var nomeCartellaMozilla = "mozilla-enit-2" // modello 2.1 di Firefox (il vecchio 1.0 stava in mozilla-enit)

func dirMozilla() string { return filepath.Join(dirApp, "modelli", nomeCartellaMozilla) }
func dirIA() string      { return filepath.Join(dirApp, "modelli", "ia") }
