package main

import (
	"testing"
	"time"
)

func TestSegnaNovita(t *testing.T) {
	oggi := time.Date(2026, 11, 28, 10, 0, 0, 0, time.Local)
	var s Settings
	if !segnaNovita(&s, "3.2", oggi) || s.NovitaDal != "2026-11-28" || s.NovitaVer != "3.2" {
		t.Fatalf("primo avvio: %q %q", s.NovitaDal, s.NovitaVer)
	}
	// riavvio, reinstallazione o aggiornamento alla stessa versione (anche 3.2.1): non cambia
	for _, v := range []string{"3.2", "3.2.1"} {
		if segnaNovita(&s, v, oggi.AddDate(0, 0, 20)) || s.NovitaDal != "2026-11-28" {
			t.Fatalf("%s: la data è cambiata in %q", v, s.NovitaDal)
		}
	}
	if got := novitaFino(&s); got != "2026-12-12" {
		t.Fatalf("fine: %q", got)
	}
	// versione nuova: data nuova
	if !segnaNovita(&s, "3.3", oggi.AddDate(0, 1, 0)) || s.NovitaDal != "2026-12-28" || s.NovitaVer != "3.3" {
		t.Fatalf("3.3: %q %q", s.NovitaDal, s.NovitaVer)
	}
	// data rovinata: si rimette
	s.NovitaDal = "boh"
	if !segnaNovita(&s, "3.3", oggi) || s.NovitaDal != "2026-11-28" {
		t.Fatalf("data rovinata: %q", s.NovitaDal)
	}
	if novitaFino(&Settings{}) != "" {
		t.Fatal("senza data deve essere vuoto")
	}
}
