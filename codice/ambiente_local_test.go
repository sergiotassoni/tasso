package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// RECENSIONE-3c, R3c-1: solo <profilo>\AppData\Local. Un'altra cartella dentro il profilo (che un
// programma dell'utente potrebbe poi scambiare con una giunzione) non va bene.
func TestLocalVeraSoloAppDataLocal(t *testing.T) {
	prof := t.TempDir()
	std := filepath.Join(prof, "AppData", "Local")
	altra := filepath.Join(prof, "L")
	for _, d := range []string{std, altra} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if l, nota := localVeraDa(std, prof); l != std || nota != "" {
		t.Fatalf("AppData\\Local normale cambiata: %q (%s)", l, nota)
	}
	if l, nota := localVeraDa(strings.ToUpper(std)+`\`, prof); !strings.EqualFold(filepath.Clean(l), std) || nota != "" {
		t.Fatalf("AppData\\Local scritta in maiuscolo rifiutata: %q (%s)", l, nota)
	}
	for _, local := range []string{altra, filepath.Join(prof, "AppData"), filepath.Join(std, "Sotto"), `C:\Windows\Temp`, ""} {
		if l, nota := localVeraDa(local, prof); l != std || nota == "" {
			t.Fatalf("localVeraDa(%q) = %q (%s): doveva essere %q con la nota", local, l, nota, std)
		}
	}
}
