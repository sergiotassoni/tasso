package installa

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Disinstallando Tasso: prima Tasso toglie le sue cose (se rifiuta non si tocca niente), poi
// vanno via le sue cartelle di lavoro, le attività \Tasso\, le sue chiavi e, se confermato, i dati.
func TestDisinstallaTassoTogliCoseSue(t *testing.T) {
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(Gaming, Ufficio, Zip)); err != nil {
		t.Fatal(err)
	}
	dati := t.TempDir()
	for _, d := range []string{"Tasso", "TassoTraduce", "Tasso Zip"} {
		os.MkdirAll(filepath.Join(dati, d), 0o755)
		os.WriteFile(filepath.Join(dati, d, "x"), []byte("x"), 0o644)
	}
	for _, d := range []string{"lavoro", "strumenti", "driver"} {
		os.MkdirAll(filepath.Join(p.base, d, "dentro"), 0o755)
		os.WriteFile(filepath.Join(p.base, d, "dentro", "f"), []byte("x"), 0o644)
	}
	p.reg.ScriviTesto(HKLM, ChiaveTasso+`\PuntoRipristino`, "x", "1")
	p.reg.ScriviTesto(HKCU, ChiaveTasso+`\TassoZip`, "y", "di Zip")
	p.in.CartellaDatiUtente = func() string { return dati }

	// Tasso rifiuta: niente tolto
	p.in.PulisciTasso = func(string) error { return errors.New("un gioco tradotto è aperto") }
	var eRif *ErroreTassoRifiuta
	if _, err := p.in.DisinstallaCon(p.base, []Componente{Gaming, Ufficio}, OpzioniDisinstalla{TogliDati: true}); !errors.As(err, &eRif) {
		t.Fatalf("atteso ErroreTassoRifiuta, ho %v", err)
	}
	if !esiste(filepath.Join(p.base, "Tasso.exe")) || !esiste(filepath.Join(p.base, "lavoro")) || !esiste(filepath.Join(p.menu, "Tasso Gaming.lnk")) {
		t.Fatal("Tasso ha rifiutato ma qualcosa è stato tolto")
	}

	// Tasso accetta
	pulito, attivita := 0, 0
	p.in.PulisciTasso = func(b string) error {
		if !esiste(filepath.Join(b, "Tasso.exe")) {
			t.Fatal("PulisciTasso chiamato dopo aver tolto Tasso.exe")
		}
		pulito++
		return nil
	}
	p.in.TogliAttivita = func() error { attivita++; return nil }
	r, err := p.in.DisinstallaCon(p.base, []Componente{Gaming, Ufficio}, OpzioniDisinstalla{TogliDati: true})
	if err != nil {
		t.Fatal(err)
	}
	if pulito != 1 || attivita != 1 {
		t.Fatalf("pulizia di Tasso %d volte, attività %d volte", pulito, attivita)
	}
	for _, d := range []string{"lavoro", "strumenti", "driver", "Tasso.exe"} {
		if esiste(filepath.Join(p.base, d)) {
			t.Errorf("resta %s", d)
		}
	}
	if esiste(filepath.Join(dati, "Tasso")) || esiste(filepath.Join(dati, "TassoTraduce")) || !esiste(filepath.Join(dati, "Tasso Zip")) {
		t.Fatal("dati utente tolti male")
	}
	if _, ok := p.reg.LeggiTesto(HKLM, ChiaveTasso+`\PuntoRipristino`, "x"); ok {
		t.Fatal("PuntoRipristino rimasto")
	}
	if v, _ := p.reg.LeggiTesto(HKCU, ChiaveTasso+`\TassoZip`, "y"); v != "di Zip" {
		t.Fatal("tolta una chiave di Zip")
	}
	if !esiste(filepath.Join(p.base, "Tasso Zip", "TassoZip.exe")) || r.TuttoTolto {
		t.Fatal("Zip doveva restare")
	}
}

// Senza conferma i dati restano; togliendo solo Zip, Tasso non viene chiamato.
func TestDisinstallaSenzaDatiESoloZip(t *testing.T) {
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(Gaming, Zip)); err != nil {
		t.Fatal(err)
	}
	dati := t.TempDir()
	os.MkdirAll(filepath.Join(dati, "TassoTraduce"), 0o755)
	p.in.CartellaDatiUtente = func() string { return dati }
	p.in.PulisciTasso = func(string) error { t.Fatal("Tasso chiamato togliendo solo Zip"); return nil }
	if _, err := p.in.Disinstalla(p.base, []Componente{Zip}); err != nil {
		t.Fatal(err)
	}
	p.in.PulisciTasso = nil
	if _, err := p.in.Disinstalla(p.base, []Componente{Gaming}); err != nil {
		t.Fatal(err)
	}
	if !esiste(filepath.Join(dati, "TassoTraduce")) {
		t.Fatal("dati tolti senza conferma")
	}
}

// File ancora occupato: si riprova, poi lo cancella Windows al riavvio (e si dice).
func TestDisinstallaFileOccupatoAlRiavvio(t *testing.T) {
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(TuttiComponenti...)); err != nil {
		t.Fatal(err)
	}
	occupato := errors.New("occupato")
	tentativi := 0
	p.in.TogliFile = func(f string) error {
		if strings.HasSuffix(f, "TassoPDF.exe") {
			tentativi++
			return occupato
		}
		return os.Remove(f)
	}
	p.in.Bloccato = func(err error) bool { return errors.Is(err, occupato) }
	p.in.Dormi = func(time.Duration) {}
	var alRiavvio []string
	p.in.CancellaAlRiavvio = func(f string) error { alRiavvio = append(alRiavvio, f); return nil }
	r, err := p.in.Disinstalla(p.base, TuttiComponenti)
	if err != nil {
		t.Fatal(err)
	}
	if tentativi != TentativiRiprova {
		t.Fatalf("tentativi %d", tentativi)
	}
	if len(r.AlRiavvio) != 1 || !strings.HasSuffix(r.AlRiavvio[0], "TassoPDF.exe") || len(alRiavvio) != 1 {
		t.Fatalf("al riavvio: %v / %v", r.AlRiavvio, alRiavvio)
	}
}
