package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUnaRiga(t *testing.T) {
	if g := unaRiga("Chiudi Gioco\r\ne riprova:\tniente  tolto. "); g != "Chiudi Gioco e riprova: niente tolto." {
		t.Fatalf("unaRiga = %q", g)
	}
}

func TestNomeCopiaLibero(t *testing.T) {
	ora := time.Date(2026, 11, 27, 15, 30, 0, 0, time.Local)
	c := map[string]bool{}
	es := func(p string) bool { return c[p] }
	a := nomeCopiaLibero(`D`, "g", ora, es)
	if a != filepath.Join("D", "g") {
		t.Fatal(a)
	}
	c[a] = true
	b := nomeCopiaLibero(`D`, "g", ora, es)
	if b != filepath.Join("D", "g (copia del 27-11-2026)") {
		t.Fatal(b)
	}
	c[b] = true
	d := nomeCopiaLibero(`D`, "g", ora, es)
	if d != filepath.Join("D", "g (copia del 27-11-2026 15.30)") {
		t.Fatal(d)
	}
	c[d] = true
	if e := nomeCopiaLibero(`D`, "g", ora, es); e != filepath.Join("D", "g (copia del 27-11-2026 15.30 2)") {
		t.Fatal(e)
	}
}

func TestMsgCopieNonSalvate(t *testing.T) {
	m := msgCopieNonSalvate([]string{`C:\x`})
	if !strings.Contains(m, `C:\x`) || !strings.Contains(m, "Tasso non è stato tolto") {
		t.Fatal(m)
	}
}

func TestMotivoTogliNoSenzaFinale(t *testing.T) {
	setMu.Lock()
	prima := settings.Lang
	settings.Lang = "it"
	setMu.Unlock()
	defer func() { setMu.Lock(); settings.Lang = prima; setMu.Unlock() }()
	m := motivoTogliNo(msgCopieNonSalvate([]string{`C:\x`}))
	if !strings.Contains(m, `C:\x`) || strings.Contains(m, "non è stato tolto") || !strings.HasSuffix(m, "riprova a togliere Tasso.") {
		t.Fatal(m)
	}
	if g := motivoTogliNo("Tasso non è stato tolto."); g != "Tasso non è stato tolto." {
		t.Fatal(g)
	}
	// in inglese: tradotto (anche la parte fissa attorno al percorso) e senza la frase finale
	setMu.Lock()
	settings.Lang = "en"
	setMu.Unlock()
	m = motivoTogliNo(msgCopieNonSalvate([]string{`C:\x`}))
	if !strings.Contains(m, `C:\x`) || strings.Contains(m, "not been removed") || strings.Contains(m, "was not removed") || strings.Contains(m, "Copiali") {
		t.Fatal(m)
	}
}
