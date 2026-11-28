package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// preparaApp: una cartella dati finta e un file "di sistema" fuori da lei.
func preparaApp(t *testing.T) (app, fuori string) {
	t.Helper()
	radice := t.TempDir()
	app = filepath.Join(radice, "Tasso")
	if err := os.MkdirAll(filepath.Join(app, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	fuori = filepath.Join(radice, "sistema.txt")
	if err := os.WriteFile(fuori, []byte("ORIGINALE"), 0o644); err != nil {
		t.Fatal(err)
	}
	vecchio := appDir
	appDir = app
	t.Cleanup(func() { appDir = vecchio })
	return app, fuori
}

func link(t *testing.T, a, nome string) {
	t.Helper()
	if err := os.Symlink(a, nome); err != nil {
		t.Skip("collegamenti simbolici non permessi qui:", err)
	}
}

func restaOriginale(t *testing.T, fuori string) {
	t.Helper()
	b, _ := os.ReadFile(fuori)
	if string(b) != "ORIGINALE" {
		t.Fatalf("il file fuori dalla cartella dati è stato cambiato: %q", b)
	}
}

func TestScriviFileAppNormale(t *testing.T) {
	app, _ := preparaApp(t)
	p := filepath.Join(app, "nuova", "boost.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := scriviFileApp(p, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := scriviFileApp(filepath.Join(app, "settings.json"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := apriLogApp(filepath.Join(app, "logs", "avvio.txt"), os.O_WRONLY|os.O_CREATE|os.O_APPEND)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func TestScriviFileAppRifiutaFileLink(t *testing.T) {
	app, fuori := preparaApp(t)
	p := filepath.Join(app, "boost.json")
	link(t, fuori, p)
	if err := scriviFileApp(p, []byte("CATTIVO"), 0o644); err != errPercorsoLink {
		t.Fatalf("atteso errPercorsoLink, avuto %v", err)
	}
	restaOriginale(t, fuori)
}

func TestScriviFileAppRifiutaCartellaLink(t *testing.T) {
	app, fuori := preparaApp(t)
	altra := filepath.Join(filepath.Dir(fuori), "altrove")
	if err := os.Mkdir(altra, 0o755); err != nil {
		t.Fatal(err)
	}
	link(t, altra, filepath.Join(app, "stato"))
	if err := scriviFileApp(filepath.Join(app, "stato", "x.json"), []byte("CATTIVO"), 0o644); err != errPercorsoLink {
		t.Fatalf("atteso errPercorsoLink, avuto %v", err)
	}
	if _, err := os.Stat(filepath.Join(altra, "x.json")); err == nil {
		t.Fatal("ha scritto attraverso la cartella collegamento")
	}
	// logs collegamento: il registro non si apre proprio.
	if err := os.RemoveAll(filepath.Join(app, "logs")); err != nil {
		t.Fatal(err)
	}
	link(t, altra, filepath.Join(app, "logs"))
	if _, err := apriLogApp(filepath.Join(app, "logs", "crash.txt"), os.O_WRONLY|os.O_CREATE|os.O_APPEND); err != errPercorsoLink {
		t.Fatalf("atteso errPercorsoLink, avuto %v", err)
	}
}

func TestCartellaDatiLink(t *testing.T) {
	app, fuori := preparaApp(t)
	vera := app + "-vera"
	if err := os.Rename(app, vera); err != nil {
		t.Fatal(err)
	}
	link(t, vera, app)
	if err := scriviFileApp(filepath.Join(app, "settings.json"), []byte("CATTIVO"), 0o644); err != errPercorsoLink {
		t.Fatalf("atteso errPercorsoLink, avuto %v", err)
	}
	restaOriginale(t, fuori)
}

func TestApriLogAppSpostaIlLink(t *testing.T) {
	app, fuori := preparaApp(t)
	p := filepath.Join(app, "logs", "crash.txt")
	link(t, fuori, p)
	f, err := apriLogApp(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("riga")
	f.Close()
	restaOriginale(t, fuori)
	fi, err := os.Lstat(p)
	if err != nil || !fi.Mode().IsRegular() {
		t.Fatalf("crash.txt dovrebbe essere un file vero: %v %v", fi, err)
	}
}

func TestRinominaAppEFuoriCartella(t *testing.T) {
	app, fuori := preparaApp(t)
	a := filepath.Join(app, "a.tmp")
	if err := os.WriteFile(a, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// L'arrivo è un collegamento: lo scambio sostituisce il collegamento, non il file di sistema.
	b := filepath.Join(app, "settings.json")
	link(t, fuori, b)
	if err := rinominaApp(a, b); err != nil {
		t.Fatal(err)
	}
	restaOriginale(t, fuori)
	// Percorsi fuori dalla cartella dati: nessun controllo (non sono di questa funzione).
	if err := controllaPercorsoApp(fuori); err != nil {
		t.Fatal(err)
	}
	if _, ok := sottoApp(app, app+"-altro"); ok {
		t.Fatal("una cartella con nome simile non è dentro la cartella dati")
	}
}

func TestLeggiFileAppNonSegueLink(t *testing.T) {
	app, fuori := preparaApp(t)
	buono := filepath.Join(app, "boost.json")
	if err := os.WriteFile(buono, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, err := leggiFileApp(buono); err != nil || string(b) != "{}" {
		t.Fatalf("file normale: %q %v", b, err)
	}
	if _, err := leggiFileApp(filepath.Join(app, "manca.json")); !os.IsNotExist(err) {
		t.Fatalf("file che manca: %v", err)
	}
	cattivo := filepath.Join(app, "settings.json")
	link(t, fuori, cattivo)
	if b, err := leggiFileApp(cattivo); !errors.Is(err, errPercorsoLink) {
		t.Fatalf("collegamento: %q %v", b, err)
	}
	altrove := filepath.Join(filepath.Dir(app), "altrove")
	_ = os.MkdirAll(altrove, 0o755)
	_ = os.WriteFile(filepath.Join(altrove, "crash.txt"), []byte("x"), 0o644)
	_ = os.RemoveAll(filepath.Join(app, "logs"))
	link(t, altrove, filepath.Join(app, "logs"))
	if _, err := leggiFileApp(filepath.Join(app, "logs", "crash.txt")); err == nil {
		t.Fatal("ha seguito la cartella collegata")
	}
}

// N2: settings.json collegamento: messo da parte (rinominato), il file a cui porta resta com'è e le
// impostazioni non vanno in sola lettura.
func TestLoadSettingsCollegamentoMessoDaParte(t *testing.T) {
	app, fuori := preparaApp(t)
	link(t, fuori, filepath.Join(app, "settings.json"))
	vecchieRO := settingsReadOnly
	t.Cleanup(func() { settingsReadOnly = vecchieRO })
	settingsReadOnly = false
	loadSettings()
	restaOriginale(t, fuori)
	if settingsReadOnly {
		t.Fatal("collegamento vero: non serve la sola lettura")
	}
	l, _ := filepath.Glob(filepath.Join(app, "settings.json.collegamento-*"))
	if len(l) != 1 {
		t.Fatalf("collegamento non messo da parte: %v", l)
	}
	if _, err := os.Lstat(filepath.Join(app, "settings.json")); !os.IsNotExist(err) {
		t.Fatal("settings.json collegamento ancora al suo posto")
	}
}

// RECENSIONE-3c, R3c-2: le cartelle sotto appDir si creano dentro la radice (creaCartellaApp).
func TestCreaCartellaApp(t *testing.T) {
	radice := t.TempDir()
	vecchio := appDir
	appDir = filepath.Join(radice, "Tasso") // non c'è ancora: si crea dalla cartella sopra
	t.Cleanup(func() { appDir = vecchio })
	for _, d := range []string{filepath.Join(appDir, "logs"), filepath.Join(appDir, "backup-registro", "altro account"), appDir} {
		if err := creaCartellaApp(d); err != nil {
			t.Fatalf("creaCartellaApp(%q): %v", d, err)
		}
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			t.Fatalf("%q non creata: %v", d, err)
		}
	}
	// Fuori da appDir: come os.MkdirAll.
	fuori := filepath.Join(radice, "altro", "sotto")
	if err := creaCartellaApp(fuori); err != nil {
		t.Fatal(err)
	}
	// Una cartella del percorso è un collegamento: non si crea niente dall'altra parte.
	altrove := filepath.Join(radice, "altrove")
	if err := os.Mkdir(altrove, 0o755); err != nil {
		t.Fatal(err)
	}
	link(t, altrove, filepath.Join(appDir, "backup"))
	if err := creaCartellaApp(filepath.Join(appDir, "backup", "nuova")); err == nil {
		t.Fatal("cartella creata attraverso un collegamento")
	}
	if _, err := os.Stat(filepath.Join(altrove, "nuova")); err == nil {
		t.Fatal("cartella creata dall'altra parte del collegamento")
	}
}

// R3c-2: appDir stessa trasformata in giunzione (come farebbe un programma dell'utente).
func TestCreaCartellaAppGiunzione(t *testing.T) {
	radice := t.TempDir()
	altrove := filepath.Join(radice, "Windows-finta")
	if err := os.Mkdir(altrove, 0o755); err != nil {
		t.Fatal(err)
	}
	vecchio := appDir
	appDir = filepath.Join(radice, "Tasso")
	t.Cleanup(func() { appDir = vecchio })
	if out, err := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"), "/c", "mklink", "/J", appDir, altrove).CombinedOutput(); err != nil {
		t.Skip("giunzione non creata:", err, string(out))
	}
	if err := creaCartellaApp(filepath.Join(appDir, "logs")); err == nil {
		t.Fatal("cartella creata attraverso la giunzione")
	}
	if _, err := os.Stat(filepath.Join(altrove, "logs")); err == nil {
		t.Fatal("logs creata nella cartella di arrivo della giunzione")
	}
}
