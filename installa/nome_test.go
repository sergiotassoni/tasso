package installa

import (
	"context"
	"testing"
)

// Le regole stanno in nome/ (con i loro test); qui solo il collegamento.
func TestNomeRegole(t *testing.T) {
	if c, _ := ControllaNome("Niccolò"); c != "" || !NomeValido("Niccolò") {
		t.Fatal("nome buono rifiutato")
	}
	if c, s := ControllaNome("Sergio★"); c != "simbolo" || s != "★" {
		t.Fatalf("simbolo: %q %q", c, s)
	}
	if c, _ := ControllaNome("ㅤㅤ"); c != "invisibile" {
		t.Fatalf("riempitivo coreano: %q", c)
	}
	if PulisciNome("  Gian \t  Luca ") != "Gian Luca" || NomeValido(" Sergio") {
		t.Fatal("pulizia sbagliata")
	}
}

// Stesso utente: il nome va nel suo HKCU. Un aggiornamento senza nome non lo tocca, un nome
// sbagliato non si scrive, e va via con l'ultimo programma.
func TestNomeScrittoETolto(t *testing.T) {
	p := nuovaProva(t, "3.1")
	o := p.opzioni(Zip)
	o.Nome = "  Niccolò  "
	if _, err := p.in.Installa(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if v, _ := p.reg.LeggiTesto(HKCU, ChiaveTasso, ValoreNome); v != "Niccolò" {
		t.Fatalf("nome scritto: %q", v)
	}
	if p.in.LeggiNome() != "Niccolò" {
		t.Fatalf("LeggiNome = %q", p.in.LeggiNome())
	}
	if _, err := p.in.Installa(context.Background(), p.opzioni(Zip)); err != nil { // aggiornamento automatico
		t.Fatal(err)
	}
	o3 := p.opzioni(Zip)
	o3.Nome = "<script>" // pagina manomessa
	if _, err := p.in.Installa(context.Background(), o3); err != nil {
		t.Fatal(err)
	}
	if v, _ := p.reg.LeggiTesto(HKCU, ChiaveTasso, ValoreNome); v != "Niccolò" {
		t.Fatalf("nome cambiato: %q", v)
	}
	p.reg.ScriviNumero(HKCU, ChiaveTasso, valoreNomeBiglietto, 0)
	if _, err := p.in.Disinstalla(p.base, []Componente{Zip}); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.reg.LeggiTesto(HKCU, ChiaveTasso, ValoreNome); ok || !p.reg.Vuota(HKCU, ChiaveTasso) {
		t.Fatal("nome rimasto dopo l'ultimo programma")
	}
}

// UAC con un altro amministratore (P8): il nome va solo all'utente della sessione, mai nel profilo
// dell'amministratore; LeggiNome legge solo l'utente della sessione.
func TestNomeSoloAllUtenteVero(t *testing.T) {
	p := nuovaProva(t, "3.1")
	utente := NuovoRegistroMemoria()
	p.in.RegistroUtente = utente
	p.reg.ScriviTesto(HKCU, ChiaveTasso, ValoreNome, "Amministratore")
	o := p.opzioni(Zip)
	o.Nome = "Niccolò"
	if _, err := p.in.Installa(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if v, _ := utente.LeggiTesto(HKCU, ChiaveTasso, ValoreNome); v != "Niccolò" {
		t.Fatalf("utente della sessione: %q", v)
	}
	if v, _ := p.reg.LeggiTesto(HKCU, ChiaveTasso, ValoreNome); v != "Amministratore" {
		t.Fatalf("toccato il profilo dell'amministratore: %q", v)
	}
	if p.in.LeggiNome() != "Niccolò" {
		t.Fatalf("LeggiNome = %q", p.in.LeggiNome())
	}
	utente.TogliValore(HKCU, ChiaveTasso, ValoreNome)
	if p.in.LeggiNome() != "" {
		t.Fatal("LeggiNome ha preso il nome dell'amministratore")
	}
	utente.ScriviTesto(HKCU, ChiaveTasso, ValoreNome, "Niccolò")
	if _, err := p.in.Disinstalla(p.base, []Componente{Zip}); err != nil {
		t.Fatal(err)
	}
	if _, ok := utente.LeggiTesto(HKCU, ChiaveTasso, ValoreNome); ok {
		t.Fatal("nome dell'utente rimasto dopo l'ultimo programma")
	}
}
