package traduce

import (
	"os"
	"path/filepath"
	"testing"
)

// L'elenco dei file aggiunti (manifesto.json) sta nella cartella dei dati dell'utente: un programma
// normale lo può cambiare. "Ferma traduzione" (e "Togli Tasso dal PC", da amministratore) non deve
// mai cancellare o sostituire file fuori da un gioco Unity, né i file del gioco stesso.
func TestTogliModGiocoElencoFinto(t *testing.T) {
	app := t.TempDir()

	// 1. cartella che non è un gioco Unity (come C:\Windows\System32): niente si tocca
	sistema := t.TempDir()
	os.MkdirAll(filepath.Join(sistema, "drivers"), 0o755)
	os.WriteFile(filepath.Join(sistema, "drivers", "disk.sys"), []byte("driver"), 0o644)
	os.WriteFile(filepath.Join(sistema, "utile.dll"), []byte("vero"), 0o644)
	copie := cartellaCopie(app, "finto")
	os.MkdirAll(filepath.Join(copie, "originali"), 0o755)
	os.WriteFile(filepath.Join(copie, "originali", "utile.dll"), []byte("cattivo"), 0o644)
	if err := scriviManifesto(app, "finto", ManifestoMod{Cartella: sistema, Tipo: "mono", Aggiunti: []string{"drivers"}, Sostituiti: []string{"utile.dll"}}); err != nil {
		t.Fatal(err)
	}
	if err := TogliModGioco(app, "finto"); err == nil {
		t.Fatal("elenco su una cartella che non è un gioco accettato")
	}
	if b, _ := os.ReadFile(filepath.Join(sistema, "drivers", "disk.sys")); string(b) != "driver" {
		t.Fatal("cancellato un file fuori da un gioco")
	}
	if b, _ := os.ReadFile(filepath.Join(sistema, "utile.dll")); string(b) != "vero" {
		t.Fatal("sostituito un file fuori da un gioco")
	}
	if _, ok := leggiManifesto(app, "finto"); !ok {
		t.Fatal("l'elenco rifiutato non deve sparire (va in Documenti con le copie)")
	}
	// percorso non completo o non pulito: rifiutato
	for _, c := range []string{"", "relativo", sistema + string(os.PathSeparator) + ".." + string(os.PathSeparator) + filepath.Base(sistema)} {
		if cartellaModValida(c) {
			t.Fatalf("cartella %q accettata", c)
		}
	}

	// 2. gioco Unity vero, ma l'elenco dice "aggiunti" i file del gioco: restano
	gioco := t.TempDir()
	os.MkdirAll(filepath.Join(gioco, "Gioco_Data", "Managed"), 0o755)
	os.WriteFile(filepath.Join(gioco, "Gioco_Data", "Managed", "Assembly-CSharp.dll"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(gioco, "Gioco.exe"), []byte("exe"), 0o644)
	os.WriteFile(filepath.Join(gioco, "UnityPlayer.dll"), []byte("up"), 0o644)
	os.WriteFile(filepath.Join(gioco, "winhttp.dll"), []byte("bepinex"), 0o644)
	os.MkdirAll(filepath.Join(cartellaCopie(app, "g"), "originali"), 0o755)
	os.WriteFile(filepath.Join(cartellaCopie(app, "g"), "originali", "Gioco.exe"), []byte("cattivo"), 0o644)
	if !cartellaModValida(gioco) {
		t.Fatal("gioco Unity non riconosciuto")
	}
	if err := scriviManifesto(app, "g", ManifestoMod{Cartella: gioco, Tipo: "mono",
		Aggiunti:   []string{"Gioco_Data", "Gioco.exe", "UnityPlayer.dll", "winhttp.dll", "x:y"},
		Sostituiti: []string{"Gioco.exe"}}); err != nil {
		t.Fatal(err)
	}
	if err := TogliModGioco(app, "g"); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"Gioco.exe", "UnityPlayer.dll", filepath.Join("Gioco_Data", "Managed", "Assembly-CSharp.dll")} {
		if _, err := os.Stat(filepath.Join(gioco, f)); err != nil {
			t.Fatalf("tolto un file del gioco: %s", f)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(gioco, "Gioco.exe")); string(b) != "exe" {
		t.Fatal("sovrascritto l'exe del gioco")
	}
	if _, err := os.Stat(filepath.Join(gioco, "winhttp.dll")); err == nil {
		t.Fatal("winhttp.dll (di BepInEx) doveva essere tolto")
	}
}
