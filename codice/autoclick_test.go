package main

import (
	"strings"
	"testing"
)

// L'autoclick clicca solo nella finestra di partenza: con Alt+Tab o una finestra di Windows davanti si ferma.
func TestAcFinestraCambiata(t *testing.T) {
	if acFinestraCambiata(0x100, 0x100) {
		t.Fatal("stessa finestra: deve continuare")
	}
	if !acFinestraCambiata(0x100, 0x200) || !acFinestraCambiata(0x100, 0) {
		t.Fatal("finestra cambiata o nessuna finestra davanti: deve fermarsi")
	}
	if acFinestraCambiata(0, 0x200) {
		t.Fatal("senza finestra di partenza non si sa: niente blocco")
	}
}

// I giochi più noti con anti-cheat senza processo dell'anti-cheat visibile devono essere nell'elenco.
func TestAntiCheatElenco(t *testing.T) {
	have := map[string]bool{}
	for _, e := range antiCheatExes {
		have[strings.ToLower(e)] = true
	}
	for _, e := range []string{"vgc.exe", "cs2.exe", "faceit.exe", "overwatch.exe", "valorant-win64-shipping.exe"} {
		if !have[e] {
			t.Errorf("manca %s", e)
		}
	}
}
