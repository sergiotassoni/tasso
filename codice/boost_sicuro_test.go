package main

import (
	"os"
	"path/filepath"
	"testing"
)

// boost.json non deve mai restare a metà: c'è sempre un file intero da cui ripartire.
func TestBoostJsonSicuro(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "boost.json")
	if err := scriviBoostSicuro(p, []byte(`{"on":true,"prevMode":"normale"}`)); err != nil {
		t.Fatal(err)
	}
	if err := scriviBoostSicuro(p, []byte(`{"on":true,"prevMode":"silenzioso"}`)); err != nil {
		t.Fatal(err)
	}
	if s := leggiBoostGrezzo(p); !s.On || s.PrevMode != "silenzioso" {
		t.Fatalf("letto %+v", s)
	}
	// File troncato (PC spento mentre scriveva): si riparte dalla copia di prima, e quello rotto resta da parte.
	if err := os.WriteFile(p, []byte(`{"on":tr`), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := leggiBoostGrezzo(p); !s.On || s.PrevMode != "normale" {
		t.Fatalf("dopo il file rotto letto %+v, voglio la copia .bak", s)
	}
	if _, err := os.Stat(p + ".rotto"); err != nil {
		t.Fatal("il file rotto doveva restare come boost.json.rotto")
	}
	// Crash tra i due spostamenti: boost.json non c'è, la copia sì.
	_ = os.Remove(p)
	if s := leggiBoostGrezzo(p); !s.On {
		t.Fatal("senza boost.json doveva leggere boost.json.bak")
	}
	if _, err := os.Stat(p + ".tmp"); err == nil {
		t.Fatal("il file temporaneo non deve restare")
	}
}

func TestBoostDaRimettereAllUscita(t *testing.T) {
	if boostDaRimettereAllUscita(false, false, false, false) {
		t.Fatal("niente acceso: non c'è niente da rimettere")
	}
	for i, c := range [][4]bool{{true, false, false, false}, {false, true, false, false}, {false, false, true, false}, {false, false, false, true}} {
		if !boostDaRimettereAllUscita(c[0], c[1], c[2], c[3]) {
			t.Fatalf("caso %d: andava rimesso", i)
		}
	}
}

func TestCopreSchermo(t *testing.T) {
	scr := [4]int32{0, 0, 1920, 1080}
	if !copreSchermo([4]int32{0, 0, 1920, 1080}, scr) || !copreSchermo([4]int32{-8, -8, 1928, 1088}, scr) {
		t.Fatal("finestra grande quanto lo schermo: è a schermo intero")
	}
	if copreSchermo([4]int32{0, 0, 1920, 1040}, scr) || copreSchermo([4]int32{100, 100, 900, 700}, scr) {
		t.Fatal("finestra normale o massimizzata con la barra: non è a schermo intero")
	}
	if copreSchermo([4]int32{1920, 0, 3840, 1080}, scr) {
		t.Fatal("finestra sull'altro schermo")
	}
}
