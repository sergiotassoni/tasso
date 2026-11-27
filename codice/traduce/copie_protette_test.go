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

// C1: con --togli-silenzioso baseCopie viene chiamata prima che Tasso dica qual è la cartella
// protetta: non deve fissare AppData per sempre, e chi toglie le traduzioni guarda tutti e due i posti.
func TestBaseCopieAspettaOspite(t *testing.T) {
	vecchiDir, vecchiaF := dirApp, Ospite.CartellaProtetta
	t.Cleanup(func() {
		dirApp, Ospite.CartellaProtetta = vecchiDir, vecchiaF
		copieMu.Lock()
		copieDeciso, copieProtetta = false, ""
		copieMu.Unlock()
	})
	copieMu.Lock()
	copieDeciso, copieProtetta = false, ""
	copieMu.Unlock()
	radice := t.TempDir()
	dirApp = filepath.Join(radice, "app")
	lavoro := filepath.Join(radice, "lavoro")
	// copie già spostate da un avvio di prima: in AppData resta solo un gioco di Tasso Traduce da solo
	_ = os.MkdirAll(filepath.Join(lavoro, "copie-giochi", "spostato"), 0o755)
	_ = os.WriteFile(filepath.Join(lavoro, "copie-giochi", "spostato", "manifesto.json"), []byte("{}"), 0o644)
	_ = os.WriteFile(filepath.Join(lavoro, "copie-giochi", segnoCopieSpostate), []byte("ok"), 0o644)
	_ = os.MkdirAll(filepath.Join(dirApp, "copie-giochi", "dasolo"), 0o755)
	_ = os.WriteFile(filepath.Join(dirApp, "copie-giochi", "dasolo", "manifesto.json"), []byte("{}"), 0o644)

	Ospite.CartellaProtetta = nil
	if b := baseCopie(); b != dirApp {
		t.Fatalf("senza Ospite: %s", b)
	}
	Ospite.CartellaProtetta = func() string { return lavoro }
	if b := baseCopie(); b != lavoro {
		t.Fatalf("dopo Ospite deve usare lavoro, non %s", b)
	}
	if bb := basiCopie(); len(bb) != 2 || bb[0] != lavoro || bb[1] != dirApp {
		t.Fatalf("basiCopie: %v", bb)
	}
	if baseDi("spostato") != lavoro || baseDi("dasolo") != dirApp || baseDi("nuovo") != lavoro {
		t.Fatal("baseDi sbaglia posto")
	}
	if CartellaCopie() != filepath.Join(lavoro, "copie-giochi") {
		t.Fatal(CartellaCopie())
	}
	// il gioco di Tasso Traduce da solo resta in AppData (lo spostamento è già stato fatto)
	if _, err := os.Stat(filepath.Join(dirApp, "copie-giochi", "dasolo")); err != nil {
		t.Fatal("copia in AppData toccata")
	}
}

func TestDentroFinale(t *testing.T) {
	if !dentroFinale(`C:\Users\a\AppData\x\f.txt`, `c:\users\a\appdata`) || dentroFinale(`C:\Users\a\AppDataX\f`, `C:\Users\a\AppData`) || dentroFinale("", `C:\x`) {
		t.Fatal("dentroFinale")
	}
}
