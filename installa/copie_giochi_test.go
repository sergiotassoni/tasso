package installa

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// La risposta di "Tasso.exe --togli-silenzioso": solo TOGLI-OK è "fatto"; TOGLI-NO ferma; errore,
// blocco e tempo scaduto non fermano, ma non contano come fatto.
func TestEsitoTogliSilenzioso(t *testing.T) {
	casi := []struct {
		nome    string
		uscita  string
		err     error
		ok      bool
		rifiuto bool
	}{
		{"ok", "tasso-togli-silenzioso-v1\nTOGLI-OK\n", nil, true, false},
		{"rifiuta", "tasso-togli-silenzioso-v1\nTOGLI-NO Chiudi Gioco e riprova\n", &exec.ExitError{}, false, true},
		{"errore", "tasso-togli-silenzioso-v1\n", &exec.ExitError{}, false, false},
		{"blocco o crash", "", errors.New("exit status 0xc0000005"), false, false},
		{"tempo scaduto", "tasso-togli-silenzioso-v1\n", context.DeadlineExceeded, false, false},
		{"niente", "", nil, false, false},
		{"ok ma ucciso", "TOGLI-OK\n", context.DeadlineExceeded, false, false},
	}
	for _, c := range casi {
		ok, rif := EsitoTogliSilenzioso(c.uscita, c.err)
		if ok != c.ok || (rif != nil) != c.rifiuto {
			t.Errorf("%s: ok=%v rifiuto=%v", c.nome, ok, rif)
		}
	}
	if _, rif := EsitoTogliSilenzioso("TOGLI-NO Chiudi Gioco e riprova\n", nil); rif == nil || rif.Error() != "Chiudi Gioco e riprova" {
		t.Fatalf("motivo sbagliato: %v", rif)
	}
}

// Tasso va in errore, si blocca o va fuori tempo: gli originali dei giochi in lavoro\copie-giochi
// non si cancellano mai. Vanno in Documenti (o, se non si può, in ProgramData); se non si può da
// nessuna parte la disinstallazione si ferma e non tocca niente.
func TestDisinstallaSalvaCopieGiochiSeTassoNonFinisce(t *testing.T) {
	for _, caso := range []struct {
		nome   string
		uscita string
		err    error
	}{
		{"errore", "tasso-togli-silenzioso-v1\n", &exec.ExitError{}},
		{"blocco", "", errors.New("signal: killed")},
		{"tempo scaduto", "tasso-togli-silenzioso-v1\n", context.DeadlineExceeded},
	} {
		t.Run(caso.nome, func(t *testing.T) {
			p := nuovaProva(t, "3.2")
			if _, err := p.in.Installa(context.Background(), p.opzioni(TuttiComponenti...)); err != nil {
				t.Fatal(err)
			}
			orig := filepath.Join(CartellaCopieGiochi(p.base), "gioco-1", "originali", "x.dll")
			os.MkdirAll(filepath.Dir(orig), 0o755)
			os.WriteFile(orig, []byte("originale"), 0o644)
			os.WriteFile(filepath.Join(CartellaCopieGiochi(p.base), ".spostate-da-appdata"), []byte("ok"), 0o644)
			p.in.PulisciTasso = func(string) error { _, r := EsitoTogliSilenzioso(caso.uscita, caso.err); return r }
			sicuri := t.TempDir()
			doc := filepath.Join(sicuri, "Documenti", NomeCopieSicurezza)
			pd := filepath.Join(sicuri, "ProgramData", NomeCopieSicurezza)
			p.in.MeteCopieGiochi = func() []Meta { return []Meta{{Cartella: doc, Copia: true}, {Cartella: pd}} }

			r, err := p.in.DisinstallaCon(p.base, TuttiComponenti, OpzioniDisinstalla{})
			if err != nil {
				t.Fatal(err)
			}
			salvato := filepath.Join(doc, "giochi", "gioco-1", "originali", "x.dll")
			if b, err := os.ReadFile(salvato); err != nil || string(b) != "originale" {
				t.Fatalf("originale non messo al sicuro in Documenti: %v", err)
			}
			if len(r.CopieSalvate) != 1 || r.CopieSalvate[0] != filepath.Join(doc, "giochi", "gioco-1") {
				t.Fatalf("la pagina finale deve dirlo: %q", r.CopieSalvate)
			}
			if esiste(filepath.Join(p.base, "lavoro")) {
				t.Fatal("lavoro doveva sparire (le copie sono al sicuro)")
			}
		})
	}
}

func TestDisinstallaSiFermaSeLeCopieNonVannoDaNessunaParte(t *testing.T) {
	p := nuovaProva(t, "3.2")
	if _, err := p.in.Installa(context.Background(), p.opzioni(TuttiComponenti...)); err != nil {
		t.Fatal(err)
	}
	orig := filepath.Join(CartellaCopieGiochi(p.base), "gioco-1", "originali", "x.dll")
	os.MkdirAll(filepath.Dir(orig), 0o755)
	os.WriteFile(orig, []byte("originale"), 0o644)
	p.in.PulisciTasso = func(string) error { return nil } // Tasso in errore
	sicuri := t.TempDir()
	// le due mete sono file, non cartelle: non si può scrivere da nessuna parte
	doc, pd := filepath.Join(sicuri, "doc"), filepath.Join(sicuri, "pd")
	os.WriteFile(doc, []byte("x"), 0o644)
	os.WriteFile(pd, []byte("x"), 0o644)
	p.in.MeteCopieGiochi = func() []Meta { return []Meta{{Cartella: doc, Copia: true}, {Cartella: pd}} }
	_, err := p.in.DisinstallaCon(p.base, TuttiComponenti, OpzioniDisinstalla{})
	var e *ErroreCopieNonSalvate
	if !errors.As(err, &e) || len(e.Dove) != 1 {
		t.Fatalf("atteso ErroreCopieNonSalvate, ho %v", err)
	}
	if !esiste(orig) || !esiste(filepath.Join(p.base, "Tasso.exe")) || !esiste(filepath.Join(p.menu, "Tasso Gaming.lnk")) {
		t.Fatal("le copie non sono al sicuro ma qualcosa è stato tolto")
	}
	// senza mete: lo stesso
	p.in.MeteCopieGiochi = nil
	if _, err := p.in.DisinstallaCon(p.base, TuttiComponenti, OpzioniDisinstalla{}); !errors.As(err, &e) || !esiste(orig) {
		t.Fatalf("senza mete: %v", err)
	}
}

// Documenti come collegamento: non lo seguo, le copie vanno in ProgramData (rinominate).
// Una seconda copia dello stesso gioco non si mescola con la prima.
func TestSalvaCopieGiochiDocumentiCollegamento(t *testing.T) {
	base := filepath.Join(t.TempDir(), "Tasso")
	for _, g := range []string{"g1", "g2"} {
		os.MkdirAll(filepath.Join(CartellaCopieGiochi(base), g, "originali"), 0o755)
		os.WriteFile(filepath.Join(CartellaCopieGiochi(base), g, "originali", "f"), []byte(g), 0o644)
	}
	sicuri := t.TempDir()
	altrove := filepath.Join(sicuri, "altrove")
	os.MkdirAll(altrove, 0o755)
	docLink := filepath.Join(sicuri, "Documenti")
	if err := os.Symlink(altrove, docLink); err != nil {
		t.Skip("collegamenti non disponibili:", err)
	}
	pd := filepath.Join(sicuri, "ProgramData", NomeCopieSicurezza)
	os.MkdirAll(filepath.Join(pd, "giochi", "g1"), 0o755) // c'è già una copia di g1
	mete := []Meta{{Cartella: filepath.Join(docLink, NomeCopieSicurezza), Copia: true}, {Cartella: pd}}
	salvate, err := SalvaCopieGiochi(base, mete, nuovaProva(t, "3.2").in.adesso())
	if err != nil {
		t.Fatal(err)
	}
	if len(salvate) != 2 || salvate[0] != filepath.Join(pd, "giochi", "g1 (copia del 24-11-2026)") || salvate[1] != filepath.Join(pd, "giochi", "g2") {
		t.Fatalf("salvate %q", salvate)
	}
	if v, _ := os.ReadDir(altrove); len(v) != 0 {
		t.Fatal("scritto dove porta il collegamento di Documenti")
	}
	if len(giochiDaSalvare(base)) != 0 {
		t.Fatal("restano giochi in lavoro")
	}
	// niente da salvare (solo il segno): nessun errore anche senza mete
	os.WriteFile(filepath.Join(CartellaCopieGiochi(base), ".spostate-da-appdata"), []byte("ok"), 0o644)
	if s, err := SalvaCopieGiochi(base, nil, nuovaProva(t, "3.2").in.adesso()); err != nil || len(s) != 0 {
		t.Fatal(s, err)
	}
}
