package traduce

import (
	"os"
	"path/filepath"
)

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

// Ospite: funzioni di Tasso che Tasso Traduce usa quando è dentro Tasso. Tasso gira come
// amministratore: giochi e pagine vanno aperti come utente normale, e gli script PowerShell
// vanno scritti dove un programma normale non può cambiarli prima che partano.
var Ospite struct {
	ApriComeUtente func(percorso, argomenti string) bool // apre un programma/indirizzo come utente normale
	CartellaScript func() string                         // cartella protetta per gli script .ps1
	// CartellaTasso: la cartella di Tasso in Programmi (di base C:\Program Files\Tasso; un'altra
	// cartella dentro Programmi se l'installatore ha messo Tasso lì). nil = quella di base.
	CartellaTasso func() string
}

// cartellaScript: la cartella protetta di Tasso se c'è, altrimenti quella di Tasso Traduce.
func cartellaScript() string {
	if Ospite.CartellaScript != nil {
		if d := Ospite.CartellaScript(); d != "" {
			return d
		}
	}
	return filepath.Join(dirApp, "lavoro")
}

// scriviNuovo crea un file che non deve esistere già (niente file preparati da altri).
func scriviNuovo(p string, b []byte) error {
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}
