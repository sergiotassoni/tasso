package traduce

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestLeggiSenzaLinkErrori(t *testing.T) {
	base := t.TempDir()
	fuori := filepath.Join(t.TempDir(), "x")
	_ = os.WriteFile(fuori, []byte("X"), 0o644)
	if err := os.Symlink(fuori, filepath.Join(base, "imp.json")); err != nil {
		t.Skip(err)
	}
	if _, err := leggiSenzaLink(base, "imp.json"); !errors.Is(err, errLink) {
		t.Fatalf("collegamento vero: %v", err)
	}
	if _, err := leggiSenzaLink(base, "manca.json"); !os.IsNotExist(err) {
		t.Fatalf("file che manca: %v", err)
	}
	// radice che è un collegamento: rifiutata subito
	altra := filepath.Join(t.TempDir(), "radice")
	_ = os.Symlink(base, altra)
	if _, err := leggiSenzaLink(altra, "qualsiasi.json"); !errors.Is(err, errLink) {
		t.Fatalf("radice collegamento: %v", err)
	}
}

// N3: un gioco che c'è in tutti e due i posti resta solo nella cartella nuova.
func TestSpostaCopieUnaSola(t *testing.T) {
	radice := t.TempDir()
	vecchia, nuova := filepath.Join(radice, "app"), filepath.Join(radice, "lavoro")
	for _, d := range []string{filepath.Join(vecchia, "copie-giochi", "g"), filepath.Join(nuova, "copie-giochi", "g")} {
		_ = os.MkdirAll(d, 0o755)
		_ = os.WriteFile(filepath.Join(d, "manifesto.json"), []byte(d), 0o644)
	}
	if err := spostaCopieVecchie(vecchia, nuova); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(vecchia, "copie-giochi", "g")); !os.IsNotExist(err) {
		t.Fatal("doppione rimasto in AppData")
	}
}

func TestSegnoTassoNelGioco(t *testing.T) {
	g := t.TempDir()
	m := ManifestoMod{Cartella: g, Tipo: "mono", Token: "0123456789abcdef"}
	if segnoTassoNelGioco(m) {
		t.Fatal("senza file di XUnity")
	}
	cfg := filepath.Join(g, "BepInEx", "config")
	_ = os.MkdirAll(cfg, 0o755)
	_ = os.WriteFile(filepath.Join(cfg, "AutoTranslatorConfig.ini"), []byte("Url=http://127.0.0.1:1/gioco/k/altro"), 0o644)
	if segnoTassoNelGioco(m) {
		t.Fatal("segreto diverso accettato")
	}
	_ = os.WriteFile(filepath.Join(cfg, "AutoTranslatorConfig.ini"), []byte("Url=http://127.0.0.1:1/gioco/k/0123456789abcdef"), 0o644)
	if !segnoTassoNelGioco(m) {
		t.Fatal("segno vero non visto")
	}
	u := ManifestoMod{Cartella: g, Tipo: "unreal", Aggiunti: []string{filepath.Join("Paks", "~mods", nomeModUnreal)}}
	if segnoTassoNelGioco(u) {
		t.Fatal("unreal senza pak")
	}
	_ = os.MkdirAll(filepath.Join(g, "Paks", "~mods"), 0o755)
	_ = os.WriteFile(filepath.Join(g, "Paks", "~mods", nomeModUnreal), []byte("p"), 0o644)
	if !segnoTassoNelGioco(u) {
		t.Fatal("unreal con pak")
	}
}

// D8: dentro Tasso un elenco in AppData vale solo per un gioco con il segno di Tasso, mai per
// cartelle di sistema; quelli della cartella protetta valgono come prima.
func TestManifestoAffidabile(t *testing.T) {
	vecchiaF, vecchiaG := Ospite.CartellaProtetta, Ospite.GiocoDiTasso
	t.Cleanup(func() {
		Ospite.CartellaProtetta, Ospite.GiocoDiTasso = vecchiaF, vecchiaG
		copieMu.Lock()
		copieProtetta = ""
		copieMu.Unlock()
	})
	lavoro, app, g := t.TempDir(), t.TempDir(), t.TempDir()
	copieMu.Lock()
	copieProtetta = lavoro
	copieMu.Unlock()
	Ospite.CartellaProtetta = func() string { return lavoro }
	Ospite.GiocoDiTasso = nil
	m := ManifestoMod{Cartella: g, Tipo: "mono", Token: "0123456789abcdef", Exe: "Gioco.exe"}
	if !manifestoAffidabile(lavoro, m) {
		t.Fatal("cartella protetta: come prima")
	}
	if manifestoAffidabile(app, m) {
		t.Fatal("AppData senza segno accettato")
	}
	cfg := filepath.Join(g, "BepInEx", "config")
	_ = os.MkdirAll(cfg, 0o755)
	_ = os.WriteFile(filepath.Join(cfg, "AutoTranslatorConfig.ini"), []byte("/gioco/k/0123456789abcdef"), 0o644)
	if !manifestoAffidabile(app, m) {
		t.Fatal("AppData con segno rifiutato")
	}
	Ospite.GiocoDiTasso = func(string) bool { return false }
	if manifestoAffidabile(app, m) {
		t.Fatal("cartella che non è di giochi accettata")
	}
	Ospite.GiocoDiTasso = nil
	if manifestoAffidabile(app, ManifestoMod{Cartella: "/", Token: m.Token}) {
		t.Fatal("radice del disco accettata")
	}
	Ospite.CartellaProtetta = nil
	if !manifestoAffidabile(app, ManifestoMod{}) {
		t.Fatal("Tasso Traduce da solo: come prima")
	}
}

// Ritocco 5: un elenco in AppData che Tasso non usa viene riconosciuto, per dire cosa fare.
func TestElencoIgnorato(t *testing.T) {
	vecchiDir, vecchiaF := dirApp, Ospite.CartellaProtetta
	t.Cleanup(func() {
		dirApp, Ospite.CartellaProtetta = vecchiDir, vecchiaF
		copieMu.Lock()
		copieDeciso, copieProtetta = false, ""
		copieMu.Unlock()
	})
	radice := t.TempDir()
	dirApp = filepath.Join(radice, "app")
	lavoro := filepath.Join(radice, "lavoro")
	_ = os.MkdirAll(filepath.Join(lavoro, "copie-giochi"), 0o755)
	_ = os.WriteFile(filepath.Join(lavoro, "copie-giochi", segnoCopieSpostate), []byte("ok"), 0o644)
	copieMu.Lock()
	copieDeciso, copieProtetta = false, ""
	copieMu.Unlock()
	Ospite.CartellaProtetta = func() string { return lavoro }
	g := t.TempDir()
	d := filepath.Join(dirApp, "copie-giochi", "k")
	_ = os.MkdirAll(d, 0o755)
	// elenco senza firma (3.1.1): non è "ignorato", lo toglie togliModVecchia (3.1.2)
	_ = os.WriteFile(filepath.Join(d, "manifesto.json"), []byte(`{"gioco":"Gioco","cartella":`+strconvQuote(g)+`,"tipo":"mono","token":"0123456789abcdef"}`), 0o644)
	_ = baseCopie()
	if _, _, ok := elencoIgnorato("k"); ok {
		t.Fatal("elenco senza firma trattato come ignorato")
	}
	// elenco firmato da Tasso ma senza il segno di Tasso nel gioco (D8): ignorato
	m := ManifestoMod{Gioco: "Gioco", Cartella: g, Tipo: "mono", Token: "0123456789abcdef"}
	if err := firmaManifesto(dirApp, "k", &m); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(m)
	_ = os.WriteFile(filepath.Join(d, "manifesto.json"), b, 0o644)
	if !manifestoFirmato(dirApp, m) {
		t.Fatal("firma non valida")
	}
	if _, dove, ok := elencoIgnorato("k"); !ok || dove != filepath.Join(d, "originali") {
		t.Fatalf("non riconosciuto: %v %s", ok, dove)
	}
	// firma che non torna (elenco cambiato dopo): ignorato anche lui
	m.Gioco = "Gioco"
	m.Token = "fedcba9876543210"
	b, _ = json.Marshal(m)
	_ = os.WriteFile(filepath.Join(d, "manifesto.json"), b, 0o644)
	if _, _, ok := elencoIgnorato("k"); !ok {
		t.Fatal("elenco con la firma sbagliata non riconosciuto")
	}
	if l := GiochiDaSistemareAMano(); len(l) != 1 || l[0] != "Gioco" {
		t.Fatal(l)
	}
	if m := msgSistemaAMano("Gioco", "x"); !strings.Contains(m, "Verifica file") || strings.Contains(m, "Steam") {
		t.Fatal(m)
	}
}

func strconvQuote(s string) string { b, _ := json.Marshal(s); return string(b) }
