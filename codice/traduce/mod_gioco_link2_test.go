package traduce

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// giocoFinto: gioco Unity creato da un programma normale.
func giocoFinto(t *testing.T) string {
	g := t.TempDir()
	os.MkdirAll(filepath.Join(g, "Gioco_Data", "Managed"), 0o755)
	os.WriteFile(filepath.Join(g, "Gioco_Data", "Managed", "Assembly-CSharp.dll"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(g, "Gioco.exe"), []byte("exe"), 0o644)
	return g
}

func link(t *testing.T, a, nome string) {
	if err := os.Symlink(a, nome); err != nil { // su Windows: una giunzione
		t.Skip(err)
	}
}

// plugin-utente: se la copia porta altrove (giunzione in %LOCALAPPDATA%), la DLL del gioco non deve finire lì
func TestFermaTraduzionePluginUtenteNonVaFuori(t *testing.T) {
	app, sistema, gioco := t.TempDir(), t.TempDir(), giocoFinto(t)
	os.MkdirAll(filepath.Join(gioco, "BepInEx", "plugins"), 0o755)
	os.WriteFile(filepath.Join(gioco, "BepInEx", "plugins", "winhttp.dll"), []byte("cattivo"), 0o644)
	copie := cartellaCopie(app, "x")
	os.MkdirAll(filepath.Join(copie, "plugin-utente"), 0o755)
	link(t, sistema, filepath.Join(copie, "plugin-utente", "plugins"))
	scriviManifesto(app, "x", ManifestoMod{Cartella: gioco, Tipo: "mono", Aggiunti: []string{"BepInEx"}})
	_ = TogliModGioco(app, "x")
	if _, err := os.Lstat(filepath.Join(sistema, "winhttp.dll")); err == nil {
		t.Fatal("un file del gioco è stato spostato fuori dalle copie attraverso un link")
	}
}

// originali: un link nelle copie farebbe leggere da amministratore un file qualsiasi e copiarlo nel gioco
func TestFermaTraduzioneOriginaleLinkNonLetto(t *testing.T) {
	app, segreto, gioco := t.TempDir(), t.TempDir(), giocoFinto(t)
	os.WriteFile(filepath.Join(segreto, "segreto.txt"), []byte("segreto"), 0o644)
	copie := cartellaCopie(app, "x")
	os.MkdirAll(filepath.Join(copie, "originali"), 0o755)
	link(t, filepath.Join(segreto, "segreto.txt"), filepath.Join(copie, "originali", "a.dll"))
	scriviManifesto(app, "x", ManifestoMod{Cartella: gioco, Tipo: "mono", Sostituiti: []string{"a.dll"}})
	if err := TogliModGioco(app, "x"); err == nil {
		t.Error("con un link fra gli originali Ferma traduzione deve dire che non è riuscito")
	}
	if b, _ := os.ReadFile(filepath.Join(gioco, "a.dll")); string(b) == "segreto" {
		t.Fatal("file fuori dalle copie letto attraverso un link")
	}
}

// Translation: i link dentro la cartella delle traduzioni non si seguono (né in lettura né in scrittura)
func TestFermaTraduzioneTranslationLink(t *testing.T) {
	app, segreto, gioco := t.TempDir(), t.TempDir(), giocoFinto(t)
	os.WriteFile(filepath.Join(segreto, "segreto.txt"), []byte("segreto"), 0o644)
	tr := filepath.Join(gioco, "BepInEx", "Translation")
	os.MkdirAll(tr, 0o755)
	os.WriteFile(filepath.Join(tr, "vera.txt"), []byte("ciao"), 0o644)
	link(t, filepath.Join(segreto, "segreto.txt"), filepath.Join(tr, "rubato.txt"))
	link(t, segreto, filepath.Join(tr, "cartella"))
	scriviManifesto(app, "x", ManifestoMod{Cartella: gioco, Tipo: "mono", Aggiunti: []string{"BepInEx"}})
	if err := TogliModGioco(app, "x"); err != nil {
		t.Fatal(err)
	}
	salvate := filepath.Join(cartellaCopie(app, "x"), "traduzioni")
	if b, _ := os.ReadFile(filepath.Join(salvate, "vera.txt")); string(b) != "ciao" {
		t.Error("la traduzione vera deve essere tenuta")
	}
	for _, n := range []string{"rubato.txt", "cartella"} {
		if _, err := os.Lstat(filepath.Join(salvate, n)); err == nil {
			t.Errorf("%s: seguito un link dentro Translation", n)
		}
	}
	if _, err := os.Stat(filepath.Join(segreto, "segreto.txt")); err != nil {
		t.Error("RemoveAll ha seguito un link e cancellato fuori dal gioco")
	}
}

// Aggiunti: se una voce aggiunta è un link, RemoveAll toglie solo il link, non quello a cui porta
func TestFermaTraduzioneAggiuntoLink(t *testing.T) {
	app, sistema, gioco := t.TempDir(), t.TempDir(), giocoFinto(t)
	os.WriteFile(filepath.Join(sistema, "utile.dll"), []byte("vero"), 0o644)
	link(t, sistema, filepath.Join(gioco, "BepInEx"))
	scriviManifesto(app, "x", ManifestoMod{Cartella: gioco, Tipo: "mono", Aggiunti: []string{"BepInEx"}})
	_ = TogliModGioco(app, "x")
	if b, _ := os.ReadFile(filepath.Join(sistema, "utile.dll")); string(b) != "vero" {
		t.Fatal("cancellato fuori dal gioco attraverso un link")
	}
}

// cartella delle copie che è un link: Ferma traduzione non tocca niente
func TestFermaTraduzioneCopieLink(t *testing.T) {
	app, altrove, gioco := t.TempDir(), t.TempDir(), giocoFinto(t)
	scriviManifesto(app, "x", ManifestoMod{Cartella: gioco, Tipo: "mono"})
	os.Rename(cartellaCopie(app, "x"), filepath.Join(altrove, "x"))
	link(t, filepath.Join(altrove, "x"), cartellaCopie(app, "x"))
	if err := TogliModGioco(app, "x"); err == nil {
		t.Error("con la cartella delle copie che porta altrove deve rifiutare")
	}
}

func zipCon(t *testing.T, file map[string]string) string {
	p := filepath.Join(t.TempDir(), "m.zip")
	f, _ := os.Create(p)
	w := zip.NewWriter(f)
	for n, c := range file {
		x, _ := w.Create(n)
		x.Write([]byte(c))
	}
	w.Close()
	f.Close()
	return p
}

// installazione: un file del gioco che è un link (o una cartella con una giunzione) non viene sovrascritto
func TestInstallaNonSegueLink(t *testing.T) {
	app, sistema, gioco := t.TempDir(), t.TempDir(), giocoFinto(t)
	os.WriteFile(filepath.Join(sistema, "utile.dll"), []byte("vero"), 0o644)
	link(t, filepath.Join(sistema, "utile.dll"), filepath.Join(gioco, ".doorstop_version"))
	z := zipCon(t, map[string]string{"BepInEx/core/BepInEx.dll": "b", ".doorstop_version": "mod"})
	if _, err := InstallaModGioco(app, "G", gioco, "x", []string{z}, "en", "it"); err == nil {
		t.Error("con un link nel gioco l'installazione deve fermarsi")
	}
	if b, _ := os.ReadFile(filepath.Join(sistema, "utile.dll")); string(b) != "vero" {
		t.Fatalf("file fuori dal gioco sovrascritto attraverso un link: %q", b)
	}
	if _, err := os.Lstat(filepath.Join(gioco, ".doorstop_version")); err != nil {
		t.Error("il link dell'utente non va cancellato")
	}
}

// installazione: la cartella degli originali che porta altrove fa fermare prima di toccare il gioco
func TestInstallaCopieLink(t *testing.T) {
	app, altrove, gioco := t.TempDir(), t.TempDir(), giocoFinto(t)
	os.MkdirAll(filepath.Join(app, "copie-giochi"), 0o755)
	link(t, altrove, cartellaCopie(app, "x"))
	z := zipCon(t, map[string]string{"winhttp.dll": "mod"})
	if _, err := InstallaModGioco(app, "G", gioco, "x", []string{z}, "en", "it"); err == nil {
		t.Error("con le copie che portano altrove l'installazione deve fermarsi")
	}
	if _, err := os.Stat(filepath.Join(gioco, "winhttp.dll")); err == nil {
		t.Error("il gioco non va toccato")
	}
	if e, _ := os.ReadDir(altrove); len(e) > 0 {
		t.Error("scritto fuori attraverso un link")
	}
}

// Unreal: ~mods che porta altrove: Tasso non scrive e non cancella lì
func TestUnrealNonSegueLink(t *testing.T) {
	gioco, altrove := t.TempDir(), t.TempDir()
	paks := filepath.Join(gioco, "G", "Content", "Paks")
	os.MkdirAll(paks, 0o755)
	link(t, altrove, filepath.Join(paks, "~mods"))
	if _, _, err := ScriviModUnreal(gioco, paks, nil); err == nil {
		t.Error("con ~mods che porta altrove non deve scrivere")
	}
	os.WriteFile(filepath.Join(altrove, nomeModUnreal), []byte("x"), 0o644)
	rel, _ := filepath.Rel(gioco, filepath.Join(paks, "~mods", nomeModUnreal))
	if err := togliModUnreal(ManifestoMod{Cartella: gioco, Tipo: "unreal", Aggiunti: []string{rel}}); err == nil {
		t.Error("con ~mods che porta altrove deve dire che non è riuscito")
	}
	if _, err := os.Stat(filepath.Join(altrove, nomeModUnreal)); err != nil {
		t.Error("cancellato fuori dal gioco attraverso un link")
	}
}
