package installa

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// RECENSIONE-3c, R3c-3: UAC confermato dall'utente per sé. Le cartelle dati nella sua
// %LOCALAPPDATA% (Zip, PDF, Tasso, Tasso Traduce) si tolgono solo con TogliCartellaUtente (il suo
// token limitato), mai con os.RemoveAll da amministratore.
func TestCartelleDatiConTokenLimitato(t *testing.T) {
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(TuttiComponenti...)); err != nil {
		t.Fatal(err)
	}
	dati := t.TempDir()
	scrivi := func(parti ...string) {
		f := filepath.Join(append([]string{dati}, parti...)...)
		os.MkdirAll(filepath.Dir(f), 0o755)
		os.WriteFile(f, []byte("x"), 0o644)
	}
	scrivi("Tasso Zip", "impostazioni.json")
	scrivi("Tasso PDF", "impostazioni.json")
	scrivi("Tasso", "settings.json")
	scrivi("Tasso", "backup", "reg-1.reg") // da tenere
	scrivi("TassoTraduce", "memoria.db")
	p.in.CartellaDatiUtente = func() string { return dati }
	var conToken []string
	p.in.TogliCartellaUtente = func(d string) error { conToken = append(conToken, d); return os.RemoveAll(d) }
	r, err := p.in.DisinstallaCon(p.base, TuttiComponenti, OpzioniDisinstalla{TogliDati: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"Tasso Zip", "Tasso PDF", "TassoTraduce", filepath.Join("Tasso", "settings.json")} {
		if esiste(filepath.Join(dati, f)) {
			t.Errorf("resta %s", f)
		}
	}
	if !esiste(filepath.Join(dati, "Tasso", "backup", "reg-1.reg")) {
		t.Error("tolta una copia del registro da tenere")
	}
	sort.Strings(conToken)
	voglio := []string{filepath.Join(dati, "Tasso PDF"), filepath.Join(dati, "Tasso Zip"), filepath.Join(dati, "Tasso", "settings.json"), filepath.Join(dati, "TassoTraduce")}
	sort.Strings(voglio)
	if strings.Join(conToken, "|") != strings.Join(voglio, "|") {
		t.Fatalf("tolti con il token limitato: %q, attesi %q", conToken, voglio)
	}
	if len(r.NonTolti) != 0 {
		t.Fatalf("non tolti: %q", r.NonTolti)
	}
}

// R3c-3: se con il token limitato non si riesce (per esempio una giunzione che porta in una
// cartella di Windows), non si riprova da amministratore: la cartella resta e si dice.
func TestCartelleDatiTokenLimitatoNonRiesce(t *testing.T) {
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(Gaming, Zip)); err != nil {
		t.Fatal(err)
	}
	dati := t.TempDir()
	for _, d := range []string{"Tasso", "TassoTraduce", "Tasso Zip"} {
		os.MkdirAll(filepath.Join(dati, d), 0o755)
		os.WriteFile(filepath.Join(dati, d, "x"), []byte("x"), 0o644)
	}
	p.in.CartellaDatiUtente = func() string { return dati }
	p.in.TogliCartellaUtente = func(string) error { return os.ErrPermission }
	r, err := p.in.DisinstallaCon(p.base, []Componente{Gaming, Zip}, OpzioniDisinstalla{TogliDati: true})
	if r == nil {
		t.Fatalf("disinstallazione: %v", err)
	}
	detti := strings.Join(r.NonTolti, "|")
	for _, d := range []string{"Tasso", "TassoTraduce", "Tasso Zip"} {
		if !esiste(filepath.Join(dati, d, "x")) {
			t.Errorf("%s cancellata da amministratore", d)
		}
		if !strings.Contains(detti, filepath.Join(dati, d)) {
			t.Errorf("%s rimasta ma non detta: %q", d, r.NonTolti)
		}
	}
}
