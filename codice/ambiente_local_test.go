package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// RECENSIONE-3b, R3b-3: AppData\Local va bene solo dentro il profilo e senza giunzioni dal profilo in giù.
func TestSenzaLinkDaProfilo(t *testing.T) {
	prof := t.TempDir()
	local := filepath.Join(prof, "AppData", "Local")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	if !senzaLinkDaProfilo(prof, local) {
		t.Fatal("cartella normale dentro il profilo rifiutata")
	}
	if senzaLinkDaProfilo(prof, filepath.Dir(prof)) {
		t.Fatal("cartella fuori dal profilo accettata")
	}
	fuori := t.TempDir()
	j := filepath.Join(prof, "AppData", "Finta")
	if out, err := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"), "/c", "mklink", "/J", j, fuori).CombinedOutput(); err != nil {
		t.Skip("giunzione non creata:", err, string(out))
	}
	if senzaLinkDaProfilo(prof, j) {
		t.Fatal("giunzione verso fuori dal profilo accettata")
	}
}

// localVera: il risultato sta sempre dentro il profilo vero (quando Windows lo dice).
func TestLocalVeraNelProfilo(t *testing.T) {
	prof := profiloVero()
	if prof == "" {
		t.Skip("profilo vero non trovato")
	}
	if l, _ := localVera(`C:\Windows\Temp`); !dentroDi(l, prof) {
		t.Fatalf("localVera fuori dal profilo: %q", l)
	}
	if l, nota := localVera(filepath.Join(prof, "AppData", "Local")); nota != "" && senzaLinkDaProfilo(prof, l) {
		t.Fatalf("AppData\\Local normale cambiata: %q (%s)", l, nota)
	}
}
