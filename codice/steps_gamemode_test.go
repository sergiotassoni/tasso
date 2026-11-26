package main

import (
	"strings"
	"testing"
)

// La Modalità Gioco di Windows la accendono solo Gaming e FPS; "Normale" (e "Rimetti com'era")
// la rimette com'era prima, anche spenta o mancante.
func TestModalitaGiocoWindows(t *testing.T) {
	set := "'AutoGameModeEnabled' 1"
	for _, m := range []string{"gaming", "fps"} {
		if !strings.Contains(modeScripts[m], set) {
			t.Errorf("%s deve accendere la Modalità Gioco di Windows", m)
		}
	}
	for _, m := range []string{"normale", "film", "silenzioso"} {
		if strings.Contains(modeScripts[m], set) {
			t.Errorf("%s non deve forzare la Modalità Gioco di Windows", m)
		}
	}
	if !strings.Contains(modeBase, "-Name 'GameMode'") || !strings.Contains(modeBase, "[string]$p.GameMode -eq 'none'") {
		t.Error("Save-Prev/Restore-Prev devono salvare e rimettere la Modalità Gioco")
	}
}
