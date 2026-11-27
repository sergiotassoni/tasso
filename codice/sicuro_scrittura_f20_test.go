package main

import (
	"os"
	"path/filepath"
	"testing"
)

// CTRL-FIN-20 F20-6: boost.json, il portachiavi e gli altri file scritti da amministratore in
// %LOCALAPPDATA%\Tasso non seguono collegamenti e giunzioni (come settings.json e i registri).

func TestScriviBoostSicuroNonSegueLink(t *testing.T) {
	app, fuori := preparaApp(t)
	p := filepath.Join(app, "boost.json")
	// il file a parte (boost.json.tmp) è un collegamento a un file di sistema: si toglie, non si segue
	link(t, fuori, p+".tmp")
	if err := scriviBoostSicuro(p, []byte(`{"on":false}`)); err != nil {
		t.Fatal(err)
	}
	restaOriginale(t, fuori)
	if b, _ := os.ReadFile(p); string(b) != `{"on":false}` {
		t.Fatalf("boost.json non scritto: %q", b)
	}
	// boost.json stesso è un collegamento: non scrivo
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	link(t, fuori, p)
	if err := scriviBoostSicuro(p, []byte("x")); err == nil {
		t.Fatal("boost.json collegamento accettato")
	}
	restaOriginale(t, fuori)
}

func TestScriviBoostSicuroCartellaLink(t *testing.T) {
	app, fuori := preparaApp(t)
	altrove := filepath.Dir(fuori)
	link(t, altrove, filepath.Join(app, "giro"))
	if err := scriviBoostSicuro(filepath.Join(app, "giro", "boost.json"), []byte("x")); err == nil {
		t.Fatal("cartella collegamento accettata")
	}
	if _, err := os.Lstat(filepath.Join(altrove, "boost.json.tmp")); err == nil {
		t.Fatal("scritto un file dove porta il collegamento")
	}
	if _, err := os.Lstat(filepath.Join(altrove, "boost.json")); err == nil {
		t.Fatal("scritto boost.json dove porta il collegamento")
	}
}

func TestPortachiaviSenzaPinNonSegueLink(t *testing.T) {
	_, fuori := preparaApp(t)
	link(t, fuori, noPinPath())
	setNoPin(true)
	restaOriginale(t, fuori)
	saveGuard(vaultGuard{Fails: 1})
	restaOriginale(t, fuori)
}
