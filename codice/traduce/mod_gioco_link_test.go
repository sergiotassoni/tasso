package traduce

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFermaTraduzioneNonSegueLink(t *testing.T) {
	app := t.TempDir()
	sistema := t.TempDir() // come System32
	os.WriteFile(filepath.Join(sistema, "utile.dll"), []byte("vero"), 0o644)
	gioco := t.TempDir() // gioco Unity finto, creato da un programma normale
	os.MkdirAll(filepath.Join(gioco, "Gioco_Data", "Managed"), 0o755)
	os.WriteFile(filepath.Join(gioco, "Gioco_Data", "Managed", "Assembly-CSharp.dll"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(gioco, "Gioco.exe"), []byte("exe"), 0o644)
	if err := os.Symlink(sistema, filepath.Join(gioco, "sub")); err != nil { // su Windows: una giunzione
		t.Skip(err)
	}
	copie := cartellaCopie(app, "x")
	os.MkdirAll(filepath.Join(copie, "originali", "sub"), 0o755)
	os.WriteFile(filepath.Join(copie, "originali", "sub", "utile.dll"), []byte("cattivo"), 0o644)
	scriviManifesto(app, "x", ManifestoMod{Cartella: gioco, Tipo: "mono", Sostituiti: []string{filepath.Join("sub", "utile.dll")}})
	if err := TogliModGioco(app, "x"); err == nil {
		t.Error("con un link nel percorso Ferma traduzione deve dire che non è riuscito")
	}
	if b, _ := os.ReadFile(filepath.Join(sistema, "utile.dll")); string(b) != "vero" {
		t.Fatalf("file fuori dal gioco sovrascritto attraverso un collegamento: %q", b)
	}
}
