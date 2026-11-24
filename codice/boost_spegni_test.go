package main

import (
	"testing"
	"time"
)

// Spegnimento automatico del Boost acceso a mano (boostDaSpegnere in boost.go).
func TestBoostDaSpegnere(t *testing.T) {
	t0 := time.Date(2026, 11, 24, 20, 0, 0, 0, time.UTC)
	min := func(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }
	var zero time.Time
	casi := []struct {
		nome                    string
		acceso, gioco, occupato bool
		accesoAlle, ultimoGioco time.Time
		adesso                  time.Time
		vuole                   bool
	}{
		{"spento: niente da fare", false, false, false, t0, zero, min(60), false},
		{"acceso da poco", true, false, false, t0, zero, min(14), false},
		{"15 minuti senza giochi dall'accensione", true, false, false, t0, zero, min(15), true},
		{"gioco aperto adesso: mai", true, true, false, t0, min(1), min(40), false},
		{"gioco chiuso da 10 minuti: aspetta", true, false, false, t0, min(30), min(40), false},
		{"gioco chiuso da 15 minuti: spegne", true, false, false, t0, min(30), min(45), true},
		{"pulizia o aggiornamento in corso: aspetta", true, false, true, t0, zero, min(90), false},
		{"finita la pulizia: spegne", true, false, false, t0, zero, min(90), true},
		{"ultimo gioco visto prima di questa accensione: conta l'accensione", true, false, false, min(20), min(5), min(30), false},
		{"senza ora di accensione: non spegne", true, false, false, zero, zero, min(90), false},
	}
	for _, c := range casi {
		if got := boostDaSpegnere(c.acceso, c.gioco, c.occupato, c.accesoAlle, c.ultimoGioco, c.adesso); got != c.vuole {
			t.Errorf("%s: %v, volevo %v", c.nome, got, c.vuole)
		}
	}
	if boostSpegniDopo != 15*time.Minute {
		t.Errorf("boostSpegniDopo = %v", boostSpegniDopo)
	}
	if s := boostSpentoTesto(); s != "Boost spento da solo: nessun gioco aperto da 15 minuti. Il PC è tornato com'era." {
		t.Errorf("testo: %q", s)
	}
}
