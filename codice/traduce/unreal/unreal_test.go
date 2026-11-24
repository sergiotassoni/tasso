package unreal

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func locresFinto(frasi []string, ver byte) []byte {
	var w scrittore
	w.Write(magiaLocres)
	w.u8(ver)
	w.u64(0) // posizione dell'elenco, la scrivo dopo
	if ver >= 2 {
		w.u32(uint32(len(frasi)))
	}
	w.u32(1) // un namespace
	if ver >= 2 {
		w.u32(0xABCD)
	}
	w.fstring("")
	w.u32(uint32(len(frasi)))
	for i := range frasi {
		if ver >= 2 {
			w.u32(uint32(i * 7))
		}
		w.fstring("CHIAVE" + string(rune('A'+i)))
		w.u32(uint32(i * 13))
		w.u32(uint32(i))
	}
	b := w.Bytes()
	binary.LittleEndian.PutUint64(b[17:], uint64(len(b)))
	var c scrittore
	c.Write(b)
	c.u32(uint32(len(frasi)))
	for _, f := range frasi {
		c.fstring(f)
		if ver >= 2 {
			c.u32(1)
		}
	}
	return c.Bytes()
}

func TestLocresIdaEritorno(t *testing.T) {
	for _, v := range []byte{1, 2, 3} {
		orig := locresFinto([]string{"Start Game", "Options", "Quit Game"}, v)
		l, err := LeggiLocres(orig)
		if err != nil {
			t.Fatal(err)
		}
		if len(l.Frasi) != 3 || l.Frasi[1] != "Options" {
			t.Fatalf("frasi %v", l.Frasi)
		}
		if !bytes.Equal(l.Scrivi(), orig) {
			t.Fatalf("v%d: riscritto diverso", v)
		}
		l.Frasi = []string{"Inizia il gioco", "Opzioni", "Esci dal gioco è"}
		l2, err := LeggiLocres(l.Scrivi())
		if err != nil || l2.Frasi[2] != "Esci dal gioco è" || l2.Frasi[0] != "Inizia il gioco" {
			t.Fatalf("v%d tradotto: %v %v", v, err, l2)
		}
	}
}

func TestPakIdaEritorno(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "prova_P.pak")
	f, _ := os.Create(p)
	file := []FileNuovo{
		{"Pal/Content/Localization/Game/en/Game.locres", []byte("ciao locres")},
		{"Pal/Content/Localization/Game/de/Game.locres", bytes.Repeat([]byte("x"), 100000)},
	}
	if err := ScriviPak(f, file); err != nil {
		t.Fatal(err)
	}
	f.Close()
	pk, err := ApriPak(p)
	if err != nil {
		t.Fatal(err)
	}
	defer pk.Chiudi()
	if pk.Versione != 11 || pk.Montaggio != "../../../" || len(pk.Voci) != 2 {
		t.Fatalf("pak: %+v", pk)
	}
	v, ok := pk.Trova("pal/content/localization/game/en/game.locres")
	if !ok {
		t.Fatal("non trovato")
	}
	b, err := pk.Leggi(v)
	if err != nil || string(b) != "ciao locres" {
		t.Fatalf("letto %q %v", b, err)
	}
	v, _ = pk.Trova("Pal/Content/Localization/Game/de/Game.locres")
	b, _ = pk.Leggi(v)
	if len(b) != 100000 {
		t.Fatal("secondo file")
	}
}

func TestHashPercorso(t *testing.T) {
	// FNV-64 di "a" in UTF-16 (61 00) con seme 0
	h := uint64(0xcbf29ce484222325)
	for _, b := range []byte{0x61, 0x00} {
		h ^= uint64(b)
		h *= 0x00000100000001b3
	}
	if hashPercorso("A", 0) != h {
		t.Fatal("hash")
	}
}

func TestOodleSiCarica(t *testing.T) {
	if _, err := DecomprimiOodle([]byte{1, 2, 3, 4, 5, 6, 7, 8}, 100); err == nil {
		t.Fatal("dati finti accettati")
	}
}
