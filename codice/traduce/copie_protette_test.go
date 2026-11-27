package traduce

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSpostaCopieVecchie(t *testing.T) {
	radice := t.TempDir()
	vecchia, nuova := filepath.Join(radice, "app"), filepath.Join(radice, "lavoro")
	og := filepath.Join(vecchia, "copie-giochi", "gioco1", "originali")
	_ = os.MkdirAll(og, 0o755)
	_ = os.WriteFile(filepath.Join(og, "a.dll"), []byte("A"), 0o644)
	_ = os.WriteFile(filepath.Join(vecchia, "copie-giochi", "gioco1", "manifesto.json"), []byte(`{"tipo":"mono"}`), 0o644)
	// un gioco che c'è già nella nuova: non si tocca
	_ = os.MkdirAll(filepath.Join(vecchia, "copie-giochi", "gioco2"), 0o755)
	_ = os.WriteFile(filepath.Join(vecchia, "copie-giochi", "gioco2", "manifesto.json"), []byte("VECCHIO"), 0o644)
	_ = os.MkdirAll(filepath.Join(nuova, "copie-giochi", "gioco2"), 0o755)
	_ = os.WriteFile(filepath.Join(nuova, "copie-giochi", "gioco2", "manifesto.json"), []byte("NUOVO"), 0o644)
	if err := spostaCopieVecchie(vecchia, nuova); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(nuova, "copie-giochi", "gioco1", "originali", "a.dll")); string(b) != "A" {
		t.Fatalf("originale non copiato: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(nuova, "copie-giochi", "gioco2", "manifesto.json")); string(b) != "NUOVO" {
		t.Fatalf("gioco già spostato toccato: %q", b)
	}
	if _, err := os.Stat(filepath.Join(vecchia, "copie-giochi")); !os.IsNotExist(err) {
		t.Fatal("la cartella vecchia è rimasta")
	}
	// dopo il segno, una copia rimessa nel posto vecchio non viene più presa
	_ = os.MkdirAll(filepath.Join(vecchia, "copie-giochi", "cattivo"), 0o755)
	_ = os.WriteFile(filepath.Join(vecchia, "copie-giochi", "cattivo", "manifesto.json"), []byte("x"), 0o644)
	if err := spostaCopieVecchie(vecchia, nuova); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(nuova, "copie-giochi", "cattivo")); !os.IsNotExist(err) {
		t.Fatal("ripreso dal posto vecchio dopo lo spostamento")
	}
}

func TestSpostaCopieNonSegueLink(t *testing.T) {
	radice := t.TempDir()
	vecchia, nuova, fuori := filepath.Join(radice, "app"), filepath.Join(radice, "lavoro"), filepath.Join(radice, "fuori")
	_ = os.MkdirAll(filepath.Join(vecchia, "copie-giochi"), 0o755)
	_ = os.MkdirAll(filepath.Join(nuova, "copie-giochi"), 0o755)
	_ = os.MkdirAll(fuori, 0o755)
	_ = os.WriteFile(filepath.Join(fuori, "segreto.txt"), []byte("S"), 0o644)
	if err := os.Symlink(fuori, filepath.Join(vecchia, "copie-giochi", "link")); err != nil {
		t.Skip("niente collegamenti qui:", err)
	}
	if err := spostaCopieVecchie(vecchia, nuova); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(nuova, "copie-giochi", "link")); !os.IsNotExist(err) {
		t.Fatal("ha seguito il collegamento")
	}
	if _, err := os.Stat(filepath.Join(fuori, "segreto.txt")); err != nil {
		t.Fatal("ha cancellato dentro il collegamento")
	}
}

func TestLeggiSenzaLink(t *testing.T) {
	base := t.TempDir()
	_ = os.WriteFile(filepath.Join(base, "imp.json"), []byte("{}"), 0o644)
	if b, err := leggiSenzaLink(base, "imp.json"); err != nil || string(b) != "{}" {
		t.Fatal(b, err)
	}
	fuori := filepath.Join(t.TempDir(), "x")
	_ = os.WriteFile(fuori, []byte("X"), 0o644)
	if err := os.Symlink(fuori, filepath.Join(base, "l.json")); err != nil {
		t.Skip(err)
	}
	if _, err := leggiSenzaLink(base, "l.json"); err == nil {
		t.Fatal("ha seguito il collegamento")
	}
}
