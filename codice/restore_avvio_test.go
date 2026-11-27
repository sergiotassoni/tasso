package main

import (
	"errors"
	"strings"
	"testing"
)

func TestStartupRimessoOK(t *testing.T) {
	if !startupRimessoOK(psResult{JSON: "ok"}) {
		t.Fatal("riuscito ma visto come fallito")
	}
	for _, r := range []psResult{{}, {JSON: "ok", Err: errors.New("x")}, {JSON: "ok", TooSlow: true}} {
		if startupRimessoOK(r) {
			t.Fatalf("fallito ma visto come riuscito: %+v", r)
		}
	}
}

func TestNomeProgrammaAvvio(t *testing.T) {
	casi := map[string]string{"Discord": "Discord", `Pacchetto_abc\App`: "App", " ": "?", `fine\`: `fine\`}
	for in, want := range casi {
		if got := nomeProgrammaAvvio(in); got != want {
			t.Errorf("%q: %q, volevo %q", in, got, want)
		}
	}
}

func TestMsgAvvioFalliti(t *testing.T) {
	m := msgAvvioFalliti([]string{"Steam", "Discord"})
	if !strings.Contains(m, "Discord, Steam") || !strings.Contains(m, "Gestione attività") {
		t.Fatal(m)
	}
	tanti := make([]string, 12)
	for i := range tanti {
		tanti[i] = string(rune('a' + i))
	}
	if m := msgAvvioFalliti(tanti); !strings.Contains(m, "h, …") || strings.Contains(m, "i,") {
		t.Fatal(m)
	}
	for _, s := range []string{"hkcu", "appx"} {
		if !scopeAvvioNoto(s) {
			t.Fatal(s)
		}
	}
	if scopeAvvioNoto("hkcr") {
		t.Fatal("hkcr")
	}
}
