package installa

import (
	"context"
	"strings"
	"testing"
)

func TestNomeRegole(t *testing.T) {
	buoni := []string{"Sergio", "Jo", "Gian Luca", "Niccolò", "José-María", "Zoë_92", "Łukasz", "Анна", "李小龍", "R2-D2", "e\u0301va", strings.Repeat("a", 20)}
	for _, s := range buoni {
		if c := ControllaNome(s); c != "" || !NomeValido(s) {
			t.Errorf("%q: %q, doveva andare bene", s, c)
		}
	}
	cattivi := map[string]string{"": "corto", "S": "corto", strings.Repeat("è", 21): "lungo", "Sergio★": "simbolo", "<b>x": "simbolo",
		"a&b": "simbolo", "Sergio😀": "simbolo", "Ser\u200bgio": "invisibile", "Ser\u202egio": "invisibile", "\u0301ab": "simbolo", "..": "senza", "ab\xff": "invisibile"}
	for s, want := range cattivi {
		if c := ControllaNome(s); c != want || NomeValido(s) {
			t.Errorf("%q: %q, voglio %q", s, c, want)
		}
	}
	if PulisciNome("  Gian \t  Luca\u00a0") != "Gian Luca" || NomeValido(" Sergio") {
		t.Fatal("pulizia sbagliata")
	}
}

// Il nome va in HKCU di chi installa e dell'utente della sessione (come la lingua), un aggiornamento
// senza nome non lo tocca, un nome sbagliato non si scrive, e va via con l'ultimo programma.
func TestNomeScrittoETolto(t *testing.T) {
	p := nuovaProva(t, "3.1")
	utente := NuovoRegistroMemoria()
	p.in.RegistroUtente = utente
	o := p.opzioni(Zip)
	o.Nome = "  Niccolò  "
	if _, err := p.in.Installa(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	for _, r := range []*RegistroMemoria{p.reg, utente} {
		if v, _ := r.LeggiTesto(HKCU, ChiaveTasso, ValoreNome); v != "Niccolò" {
			t.Fatalf("nome scritto: %q", v)
		}
	}
	if p.in.LeggiNome() != "Niccolò" {
		t.Fatalf("LeggiNome = %q", p.in.LeggiNome())
	}
	// aggiornamento automatico (Nome vuoto): il nome resta
	o2 := p.opzioni(Zip)
	if _, err := p.in.Installa(context.Background(), o2); err != nil {
		t.Fatal(err)
	}
	if v, _ := utente.LeggiTesto(HKCU, ChiaveTasso, ValoreNome); v != "Niccolò" {
		t.Fatalf("aggiornamento senza nome l'ha cambiato: %q", v)
	}
	// nome sbagliato (pagina manomessa): non si scrive
	o3 := p.opzioni(Zip)
	o3.Nome = "<script>"
	if _, err := p.in.Installa(context.Background(), o3); err != nil {
		t.Fatal(err)
	}
	if v, _ := utente.LeggiTesto(HKCU, ChiaveTasso, ValoreNome); v != "Niccolò" {
		t.Fatalf("nome sbagliato scritto: %q", v)
	}
	// un valore strano nel registro non si propone
	utente.ScriviTesto(HKCU, ChiaveTasso, ValoreNome, "a<b")
	if p.in.LeggiNome() != "" {
		t.Fatal("valore strano proposto")
	}
	utente.ScriviTesto(HKCU, ChiaveTasso, ValoreNome, "Niccolò")
	utente.ScriviNumero(HKCU, ChiaveTasso, valoreNomeBiglietto, 0)
	if _, err := p.in.Disinstalla(p.base, []Componente{Zip}); err != nil {
		t.Fatal(err)
	}
	for _, r := range []*RegistroMemoria{p.reg, utente} {
		if _, ok := r.LeggiTesto(HKCU, ChiaveTasso, ValoreNome); ok {
			t.Fatal("nome rimasto dopo l'ultimo programma")
		}
		if !r.Vuota(HKCU, ChiaveTasso) {
			t.Fatal("HKCU\\Software\\Tasso rimasta con dei valori")
		}
	}
}
