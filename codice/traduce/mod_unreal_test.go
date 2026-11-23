package traduce

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tasso/traduce/unreal"
)

func locresV1(frasi []string) []byte {
	var b bytes.Buffer
	b.Write([]byte{0x0E, 0x14, 0x74, 0x75, 0x67, 0x4A, 0x03, 0xFC, 0x4A, 0x15, 0x90, 0x9D, 0xC3, 0x37, 0x7F, 0x1B})
	b.WriteByte(1)
	_ = binary.Write(&b, binary.LittleEndian, uint64(29))
	b.Write([]byte{0, 0, 0, 0})
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(frasi)))
	for _, f := range frasi {
		_ = binary.Write(&b, binary.LittleEndian, uint32(len(f)+1))
		b.WriteString(f)
		b.WriteByte(0)
	}
	return b.Bytes()
}

func TestLocresDellaLingua(t *testing.T) {
	si := []string{"Pal/Content/Localization/Game/en/Game.locres", "X/Content/Localization/Game/en-US/Game.locres", "/A/Content/Localization/Engine/zh-Hans/Engine.locres"}
	da := []string{"en", "en", "zh"}
	for i, n := range si {
		if !locresDellaLingua(n, da[i]) {
			t.Errorf("%s dovrebbe essere %s", n, da[i])
		}
	}
	for _, n := range []string{"Pal/Content/Localization/Game/it/Game.locres", "Pal/Content/Localization/Game/en/Game.locmeta", "Pal/Content/en/Game.locres"} {
		if locresDellaLingua(n, "en") {
			t.Errorf("%s non è inglese", n)
		}
	}
}

func TestPezziFissi(t *testing.T) {
	finto := func(s string) (string, bool) {
		r := strings.NewReplacer("Level", "Livello", "Hello", "Ciao", "Press", "Premi", "to start", "per iniziare", "Attack", "Attacco")
		return r.Replace(s), true
	}
	casi := map[string]string{
		"Level {0}":                   "Livello {0}",
		"<Bold>Hello</>":              "<Bold>Ciao</>",
		"Press {Key} to start":        "Premi {Key} per iniziare",
		"Hello\nPress {Key} to start": "Ciao\nPremi {Key} per iniziare",
	}
	for in, atteso := range casi {
		if got, _ := TraduciConPezziFissi(in, finto); got != atteso {
			t.Errorf("%q -> %q, atteso %q", in, got, atteso)
		}
	}
	// il traduttore perde il segnaposto: traduco a pezzi e lo tengo
	perde := func(s string) (string, bool) {
		return strings.NewReplacer("{0}", "", "Level", "Livello").Replace(s), true
	}
	if got, _ := TraduciConPezziFissi("Level {0}", perde); got != "Livello {0}" {
		t.Errorf("segnaposto perso: %q", got)
	}
	for _, s := range []string{"123", "BP_Item_01", "{0}", "", "+"} {
		if tradurreFrase(s) {
			t.Errorf("%q non va tradotta", s)
		}
	}
}

func TestModUnrealCompleta(t *testing.T) {
	gioco := t.TempDir()
	paks := filepath.Join(gioco, "Pal", "Content", "Paks")
	_ = os.MkdirAll(paks, 0o755)
	f, _ := os.Create(filepath.Join(paks, "Pal-Windows.pak"))
	err := unreal.ScriviPak(f, []unreal.FileNuovo{
		{Nome: "Pal/Content/Localization/Game/en/Game.locres", Dati: locresV1([]string{"Hello", "Level {0}", "100", "Hello"})},
		{Nome: "Pal/Content/Localization/Game/it/Game.locres", Dati: locresV1([]string{"Ciao"})},
		{Nome: "Pal/Content/Altro.uasset", Dati: []byte("x")},
	})
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if CartellaPaksUnreal(gioco) != paks {
		t.Fatal("cartella Paks non trovata")
	}
	if ok, m := UnrealPossibile(gioco); !ok {
		t.Fatal(m)
	}
	if f, sorg, err := LeggiLocresUnreal(paks, "it"); err != nil || sorg != "en" || f[0].Nome != "Pal/Content/Localization/Game/it/Game.locres" {
		t.Fatalf("lingua del gioco: %v %s %v", err, sorg, f)
	}
	files, _, err := LeggiLocresUnreal(paks, "en")
	if err != nil || len(files) != 1 {
		t.Fatalf("%v %d", err, len(files))
	}
	chiamate := 0
	n, err := TraduciLocres(files, func(s string) (string, bool) {
		chiamate++
		return strings.NewReplacer("Hello", "Ciao àè", "Level", "Livello").Replace(s), true
	}, nil, nil)
	if err != nil || n != 2 || chiamate != 2 {
		t.Fatalf("tradotte %d, chiamate %d, %v", n, chiamate, err)
	}
	rel, creata, err := ScriviModUnreal(gioco, paks, files)
	if err != nil || !creata {
		t.Fatal(err, creata)
	}
	p, err := unreal.ApriPak(filepath.Join(gioco, rel))
	if err != nil {
		t.Fatal(err)
	}
	v, ok := p.Trova("Pal/Content/Localization/Game/en/Game.locres")
	if !ok {
		t.Fatal("file non nel pak")
	}
	b, _ := p.Leggi(v)
	p.Chiudi()
	l, err := unreal.LeggiLocres(b)
	if err != nil || strings.Join(l.Frasi, "|") != "Ciao àè|Livello {0}|100|Ciao àè" {
		t.Fatalf("%v %q", err, l.Frasi)
	}
	m := ManifestoMod{Cartella: gioco, Tipo: "unreal", Aggiunti: []string{rel}, CreataMods: creata}
	if err := togliModUnreal(m); err != nil {
		t.Fatal(err)
	}
	if esisteVoce(filepath.Join(paks, "~mods")) {
		t.Fatal("~mods non tolta")
	}
	if !esisteVoce(filepath.Join(paks, "Pal-Windows.pak")) {
		t.Fatal("originale sparito")
	}
}
