package traduce

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// RECENSIONE-3, R3-1: in %LOCALAPPDATA%\TassoTraduce (dirApp) Tasso, da amministratore, non scrive,
// non rinomina e non cancella attraverso collegamenti o giunzioni (come F20-6 per la cartella
// Tasso). Su Linux il collegamento simbolico fa la parte della giunzione (ModeSymlink).

// datiFinti: dirApp in una cartella temporanea e un file "di sistema" fuori, che non deve cambiare.
func datiFinti(t *testing.T) (app, fuori string) {
	t.Helper()
	radice := t.TempDir()
	app = filepath.Join(radice, "TassoTraduce")
	if err := os.MkdirAll(filepath.Join(app, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	sistema := filepath.Join(radice, "Windows")
	if err := os.MkdirAll(sistema, 0o755); err != nil {
		t.Fatal(err)
	}
	fuori = filepath.Join(sistema, "importante.dll")
	if err := os.WriteFile(fuori, []byte("originale"), 0o644); err != nil {
		t.Fatal(err)
	}
	vecchio := dirApp
	dirApp = app
	t.Cleanup(func() { dirApp = vecchio })
	return app, fuori
}

func collegaFinto(t *testing.T, verso, nome string) {
	t.Helper()
	if err := os.Symlink(verso, nome); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("collegamenti simbolici non permessi su questo Windows:", err)
		}
		t.Fatal(err)
	}
}

func intatto(t *testing.T, fuori string) {
	t.Helper()
	b, err := os.ReadFile(fuori)
	if err != nil || string(b) != "originale" {
		t.Fatalf("il file fuori da TassoTraduce è cambiato: %q %v", b, err)
	}
	voci, _ := os.ReadDir(filepath.Dir(fuori))
	if len(voci) != 1 {
		t.Fatalf("scritto qualcosa nella cartella di sistema: %v", voci)
	}
}

func TestScriviDatiNormale(t *testing.T) {
	app, _ := datiFinti(t)
	p := filepath.Join(app, "impostazioni.json")
	if err := scriviDati(p, []byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := scriviDati(p, []byte(`{"a":2}`)); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != `{"a":2}` {
		t.Fatalf("%q", b)
	}
	voci, _ := os.ReadDir(app)
	for _, v := range voci {
		if strings.Contains(v.Name(), ".tmp") {
			t.Fatalf("resta un file a parte: %s", v.Name())
		}
	}
}

// La cartella logs (o modelli\ia, lavoro...) è una giunzione verso Windows: niente scritte, rinomini
// né cancellazioni là dentro.
func TestDatiCartellaGiunzione(t *testing.T) {
	app, fuori := datiFinti(t)
	if err := os.RemoveAll(filepath.Join(app, "logs")); err != nil {
		t.Fatal(err)
	}
	collegaFinto(t, filepath.Dir(fuori), filepath.Join(app, "logs"))
	p := filepath.Join(app, "logs", "importante.dll")
	if err := scriviDati(p, []byte("cattivo")); !errors.Is(err, errDatiLink) {
		t.Fatalf("scrittura accettata: %v", err)
	}
	if f, err := apriLogDati(p); err == nil {
		f.Close()
		t.Fatal("registro aperto attraverso la giunzione")
	}
	if f, err := creaLogDati(p); err == nil {
		f.Close()
		t.Fatal("registro creato attraverso la giunzione")
	}
	if f, err := creaFileDati(filepath.Join(app, "logs", "modello.gguf.parziale")); err == nil {
		f.Close()
		t.Fatal("scaricamento accettato attraverso la giunzione")
	}
	if err := rimuoviDati(p); err == nil {
		t.Fatal("cancellazione accettata attraverso la giunzione")
	}
	if err := rinominaDati(p, filepath.Join(app, "via.dll")); err == nil {
		t.Fatal("rinomino accettato attraverso la giunzione")
	}
	if err := creaCartellaDati(filepath.Join(app, "logs", "sotto")); err == nil {
		t.Fatal("cartella creata attraverso la giunzione")
	}
	ruotaLogDati(p, 0)
	intatto(t, fuori)
}

// TassoTraduce stessa è una giunzione: non scrivo niente.
func TestDatiRadiceGiunzione(t *testing.T) {
	app, fuori := datiFinti(t)
	if err := os.RemoveAll(app); err != nil {
		t.Fatal(err)
	}
	collegaFinto(t, filepath.Dir(fuori), app)
	if err := scriviDati(filepath.Join(app, "importante.dll"), []byte("cattivo")); !errors.Is(err, errDatiLink) {
		t.Fatalf("scrittura accettata: %v", err)
	}
	if err := percorsoDatiSicuro(app); !errors.Is(err, errDatiLink) {
		t.Fatalf("radice giunzione non vista: %v", err)
	}
	intatto(t, fuori)
}

// Il file stesso è un collegamento a un file di sistema: si sostituisce o si toglie il collegamento,
// il file a cui porta non cambia.
func TestDatiFileCollegamento(t *testing.T) {
	app, fuori := datiFinti(t)
	p := filepath.Join(app, "logs", "crash.txt")
	collegaFinto(t, fuori, p)
	f, err := apriLogDati(p)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("riga\r\n")
	f.Close()
	intatto(t, fuori)
	if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
		t.Fatalf("crash.txt non è un file vero: %v", err)
	}
	q := filepath.Join(app, "impostazioni.json")
	collegaFinto(t, fuori, q)
	if err := scriviDati(q, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	intatto(t, fuori)
	if b, _ := os.ReadFile(q); string(b) != "{}" {
		t.Fatalf("%q", b)
	}
	r := filepath.Join(app, "logs", "voci.txt")
	collegaFinto(t, fuori, r)
	if f, err := creaLogDati(r); err != nil {
		t.Fatal(err)
	} else {
		f.Close()
	}
	intatto(t, fuori)
}

// Lo scaricamento di un modello: il .parziale con il nome di sempre, preparato come collegamento da
// un altro programma, non si segue (il file nuovo ha un nome a caso) e alla fine va al suo posto.
func TestScaricaParzialeCollegamento(t *testing.T) {
	app, fuori := datiFinti(t)
	dir := filepath.Join(app, "modelli", "ia")
	if err := creaCartellaDati(dir); err != nil {
		t.Fatal(err)
	}
	collegaFinto(t, fuori, filepath.Join(dir, "modello.gguf.parziale"))
	f, err := creaFileDati(filepath.Join(dir, "modello.gguf.parziale"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("modello")
	f.Close()
	if err := rinominaDati(f.Name(), filepath.Join(dir, "modello.gguf")); err != nil {
		t.Fatal(err)
	}
	intatto(t, fuori)
	if b, _ := os.ReadFile(filepath.Join(dir, "modello.gguf")); string(b) != "modello" {
		t.Fatalf("%q", b)
	}
	// e dopo: i pezzi rimasti si tolgono togliendo il collegamento, non il file di sistema
	if err := rimuoviDati(filepath.Join(dir, "modello.gguf.parziale")); err != nil {
		t.Fatal(err)
	}
	intatto(t, fuori)
}

// Fuori da TassoTraduce le funzioni non cambiano niente (cartelle dei giochi, Programmi\Tasso).
func TestDatiFuoriNonControlla(t *testing.T) {
	_, _ = datiFinti(t)
	altrove := filepath.Join(t.TempDir(), "gioco")
	if err := creaCartellaDati(altrove); err != nil {
		t.Fatal(err)
	}
	if err := scriviDati(filepath.Join(altrove, "x.txt"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := percorsoDatiSicuro(altrove); err != nil {
		t.Fatal(err)
	}
}

// La libreria delle traduzioni e la memoria (traduttore.go) passano dalle stesse regole.
func TestScriviLibreriaGiunzione(t *testing.T) {
	app, fuori := datiFinti(t)
	collegaFinto(t, filepath.Dir(fuori), filepath.Join(app, "librerie"))
	if scriviLibreria(filepath.Join(app, "librerie", "importante.dll"), map[string]string{"a": "b"}, nil) {
		t.Fatal("libreria scritta attraverso la giunzione")
	}
	intatto(t, fuori)
}
