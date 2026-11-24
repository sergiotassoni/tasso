// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

import (
	"strings"
	"testing"
)

// Gli aggiornamenti li gestisce Tasso: Tasso PDF apre solo Tasso o, dopo un sì, la pagina delle versioni.
func TestAggiornamenti(t *testing.T) {
	vecchio, vecchioLink := piattaforma.apriTassoAgg, piattaforma.apriLink
	defer func() { piattaforma.apriTassoAgg, piattaforma.apriLink = vecchio, vecchioLink }()
	piattaforma.apriTassoAgg = nil
	if r := goControllaAggiornamenti(); r != "nessuno" {
		t.Fatalf("senza Tasso: %q", r)
	}
	piattaforma.apriTassoAgg = func() string { return "tasso" }
	if r := goControllaAggiornamenti(); r != "tasso" {
		t.Fatalf("con Tasso: %q", r)
	}
	var aperto string
	piattaforma.apriLink = func(u string) error { aperto = u; return nil }
	if err := goPaginaVersioni(); err != nil || !strings.HasPrefix(aperto, "https://github.com/sergiotassoni/tasso/") {
		t.Fatalf("pagina: %q %v", aperto, err)
	}
	if _, ok := legami()["goControllaAggiornamenti"]; !ok {
		t.Fatal("goControllaAggiornamenti non collegata alla pagina")
	}
}
