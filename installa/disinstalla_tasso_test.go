package installa

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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

// M2 (verifica2): installazione di prima in una cartella condivisa (non "...\Tasso", con dentro
// anche cose di altri programmi): lavoro, strumenti e driver NON si cancellano.
func TestDisinstallaCartellaCondivisaTieneSottocartelle(t *testing.T) {
	p := nuovaProva(t, "3.1")
	// la cartella di prova si chiama Tasso: la sposto in una "condivisa" con un altro programma
	condivisa := filepath.Join(filepath.Dir(p.base), "Programmi miei")
	if err := os.MkdirAll(condivisa, 0o755); err != nil {
		t.Fatal(err)
	}
	p.base = condivisa
	o := p.opzioni(Gaming, Ufficio)
	o.Base = condivisa
	if _, err := p.in.Installa(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	altro := filepath.Join(condivisa, "AltroProgramma", "altro.exe")
	os.MkdirAll(filepath.Dir(altro), 0o755)
	os.WriteFile(altro, []byte("x"), 0o644)
	driver := filepath.Join(condivisa, "driver", "stampante.inf")
	os.MkdirAll(filepath.Dir(driver), 0o755)
	os.WriteFile(driver, []byte("di un altro programma"), 0o644)
	st, _ := LeggiStato(condivisa)
	if CartellaSoloTasso(condivisa, st) {
		t.Fatal("una cartella con altri programmi non è solo di Tasso")
	}
	if cr := CriteriPer(condivisa, []Componente{Gaming}, st, nil, nil, 1, ""); len(cr.Cartelle) != 0 {
		t.Fatalf("chiuderei tutto quello che parte da %v", cr.Cartelle)
	}
	if _, err := p.in.DisinstallaCon(condivisa, []Componente{Gaming, Ufficio}, OpzioniDisinstalla{}); err != nil {
		t.Fatal(err)
	}
	if !esiste(driver) || !esiste(altro) {
		t.Fatal("cancellata una cartella di un altro programma")
	}
}

// La cartella di Tasso (si chiama Tasso, o dentro ci sono solo cose di Tasso Setup): sì.
func TestCartellaSoloTasso(t *testing.T) {
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(Gaming, Zip)); err != nil {
		t.Fatal(err)
	}
	st, _ := LeggiStato(p.base)
	if !CartellaSoloTasso(p.base, st) {
		t.Fatal("la cartella di Tasso non riconosciuta")
	}
	if CartellaSoloTasso(p.base, nil) {
		t.Fatal("senza installazione: no")
	}
}

// TogliTasso: Tasso.exe va via solo togliendo tutti i suoi componenti installati.
func TestTogliTasso(t *testing.T) {
	st := &Stato{Componenti: []Componente{Gaming, Ufficio, Zip}}
	casi := []struct {
		togli []Componente
		via   bool
	}{
		{[]Componente{Gaming, Ufficio}, true},
		{[]Componente{Gaming}, false}, // resta Ufficio
		{[]Componente{Zip}, false},
		{[]Componente{Gaming, Ufficio, Zip}, true},
	}
	for i, c := range casi {
		if TogliTasso(st, c.togli) != c.via {
			t.Errorf("%d: %v", i, c.togli)
		}
	}
	if TogliTasso(nil, []Componente{Gaming}) || TogliTasso(&Stato{Componenti: []Componente{Zip}}, []Componente{Zip}) {
		t.Error("senza Tasso installato")
	}
}

// TassoGiaPulito: Tasso ha già tolto le sue cose (goPreparaDisinstalla): non lo si richiama.
func TestDisinstallaTassoGiaPulito(t *testing.T) {
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(Gaming, Ufficio)); err != nil {
		t.Fatal(err)
	}
	p.in.PulisciTasso = func(string) error { t.Fatal("PulisciTasso richiamato"); return nil }
	if _, err := p.in.DisinstallaCon(p.base, []Componente{Gaming, Ufficio}, OpzioniDisinstalla{TassoGiaPulito: true}); err != nil {
		t.Fatal(err)
	}
}

// "Togli anche i miei dati" con Tasso che non ha spostato in Documenti gli originali dei giochi
// (Tasso vecchio, bloccato o in errore): copie-giochi e le copie del registro restano, il resto dei
// dati va via. Le copie a metà degli aggiornamenti di Tasso (Tasso.exe.old...) vanno via con Tasso,
// così la cartella resta vuota e si toglie.
func TestDisinstallaDatiTieneOriginaliGiochi(t *testing.T) {
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
	scrivi("TassoTraduce", "copie-giochi", "gioco-1", "data.pak")
	scrivi("TassoTraduce", "memoria.db")
	scrivi("TassoTraduce", "modelli", "m.bin")
	scrivi("Tasso", "backup", "reg-1.reg")
	scrivi("Tasso", "settings.json")
	scrivi("Tasso", "portachiavi.dat")
	os.MkdirAll(filepath.Join(dati, "Tasso", "backup-registro"), 0o755) // vuota: va via
	for _, f := range ResiduiAggiornamentoTasso {
		os.WriteFile(filepath.Join(p.base, f), []byte("vecchio"), 0o755)
	}
	p.in.CartellaDatiUtente = func() string { return dati }
	r, err := p.in.DisinstallaCon(p.base, TuttiComponenti, OpzioniDisinstalla{TogliDati: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range [][]string{{"TassoTraduce", "copie-giochi", "gioco-1", "data.pak"}, {"Tasso", "backup", "reg-1.reg"}} {
		if !esiste(filepath.Join(append([]string{dati}, f...)...)) {
			t.Errorf("tolto %v: era un originale da tenere", f)
		}
	}
	for _, f := range [][]string{{"TassoTraduce", "memoria.db"}, {"TassoTraduce", "modelli"}, {"Tasso", "settings.json"},
		{"Tasso", "portachiavi.dat"}, {"Tasso", "backup-registro"}} {
		if esiste(filepath.Join(append([]string{dati}, f...)...)) {
			t.Errorf("resta %v", f)
		}
	}
	if !r.TuttoTolto || esiste(p.base) {
		t.Fatalf("la cartella di Tasso doveva sparire (tutto tolto %v, non tolti %v)", r.TuttoTolto, r.NonTolti)
	}
	tenuti := []string{filepath.Join(dati, "Tasso", "backup"), filepath.Join(dati, "TassoTraduce", "copie-giochi")}
	if !reflect.DeepEqual(r.DatiTenuti, tenuti) {
		t.Fatalf("dati tenuti %q, voglio %q (la pagina finale deve dirlo)", r.DatiTenuti, tenuti)
	}

	// Tasso ha già spostato tutto in Documenti (copie-giochi vuota): via la cartella intera
	p2 := nuovaProva(t, "3.1")
	if _, err := p2.in.Installa(context.Background(), p2.opzioni(Gaming)); err != nil {
		t.Fatal(err)
	}
	dati2 := t.TempDir()
	os.MkdirAll(filepath.Join(dati2, "TassoTraduce", "copie-giochi"), 0o755)
	os.WriteFile(filepath.Join(dati2, "TassoTraduce", "memoria.db"), []byte("x"), 0o644)
	p2.in.CartellaDatiUtente = func() string { return dati2 }
	r2, err := p2.in.DisinstallaCon(p2.base, []Componente{Gaming}, OpzioniDisinstalla{TogliDati: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.DatiTenuti) != 0 {
		t.Fatalf("niente da tenere, ma dice %q", r2.DatiTenuti)
	}
	if esiste(filepath.Join(dati2, "TassoTraduce")) {
		t.Fatal("TassoTraduce doveva sparire tutta")
	}
}

// Una cartella con un nome qualsiasi, con dentro solo cose di Tasso e un Tasso.exe.old, è ancora solo di Tasso.
func TestCartellaSoloTassoConResiduiAggiornamento(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "AppGiochi")
	os.MkdirAll(dir, 0o755)
	st := &Stato{Programmi: map[Programma]StatoProg{ProgTasso: {File: []FileElenco{{Nome: "Tasso.exe"}}}}}
	os.WriteFile(filepath.Join(dir, "Tasso.exe"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "Tasso.exe.old"), nil, 0o644)
	if !CartellaSoloTasso(dir, st) {
		t.Fatal("Tasso.exe.old è di Tasso")
	}
	os.WriteFile(filepath.Join(dir, "altro.exe"), nil, 0o644)
	if CartellaSoloTasso(dir, st) {
		t.Fatal("altro.exe non è di Tasso")
	}
}
