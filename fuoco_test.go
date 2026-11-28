// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

import (
	"os"
	"strings"
	"testing"
)

// RECENSIONE-2c, B1: nelle domande il fuoco (e quindi Invio) va sul tasto principale, non sull'ultimo.
// In «Buttare via le modifiche?» da Annulla in alto l'ultimo tasto è «Butta via».
func TestFuocoSulTastoPrincipale(t *testing.T) {
	b, err := os.ReadFile("ui/app.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `box.querySelector(".prim") || box.lastChild`) {
		t.Error("chiedi: il fuoco non va sul tasto principale")
	}
	if strings.Contains(s, "box.lastChild && box.lastChild.focus()") {
		t.Error("chiedi: il fuoco va ancora sempre sull'ultimo tasto")
	}
	d, err := os.ReadFile("ui/documento.js")
	if err != nil {
		t.Fatal(err)
	}
	// senzaApplica: «Continua a modificare» è il tasto principale.
	if !strings.Contains(string(d), "else tasti[0].push(true);") {
		t.Error("chiudiStrumento: «Continua a modificare» non è più il tasto principale senza Applica")
	}
}
