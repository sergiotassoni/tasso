package main

import "testing"

// Privacy: senza il sì dell'utente nessun nome di gioco viene cercato su Steam.
func TestCoverDaCercare(t *testing.T) {
	if coverDaCercare(false, 0) {
		t.Fatal("senza permesso non si cerca niente")
	}
	if !coverDaCercare(true, 0) || coverDaCercare(true, 40) {
		t.Fatal("con permesso: al massimo 40 per volta")
	}
}
