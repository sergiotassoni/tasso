package traduce

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// motore che tiene i segnaposti (come Bergamot con i nomi finti) e traduce qualche parola.
type motoreSegnaposti struct{ perde bool }

func (motoreSegnaposti) Nome() string { return "Mozilla" }
func (m motoreSegnaposti) Traduci(_ context.Context, r RichiestaTraduzione) (string, error) {
	s := r.Testo
	if m.perde {
		for _, p := range segnapostiTasti {
			s = strings.ReplaceAll(s, p, "")
		}
		// senza segnaposti il motore storpia il tasto
		s = strings.ReplaceAll(s, "CONTROL", "CONTROLLO")
	}
	rep := strings.NewReplacer("Hold", "Tieni premuto", "Press", "Premi", "and use", "e usa", "to sneak", "per muoverti furtivamente",
		"to crouch", "per accovacciarti", "Grab objects using", "Afferra gli oggetti usando")
	return rep.Replace(s), nil
}

func TestTraduciConMotoreTasti(t *testing.T) {
	ctx := context.Background()
	got, err := traduciConMotore(ctx, motoreSegnaposti{}, RichiestaTraduzione{Testo: "Hold CONTROL and use WASD to sneak"})
	if err != nil || got != "Tieni premuto CONTROL e usa WASD per muoverti furtivamente" {
		t.Fatalf("segnaposti: %q %v", got, err)
	}
	got, _ = traduciConMotore(ctx, motoreSegnaposti{}, RichiestaTraduzione{Testo: "Grab objects using left mouse."})
	if got != "Afferra gli oggetti usando il tasto sinistro del mouse." {
		t.Fatalf("left mouse: %q", got)
	}
	// il motore perde i segnaposti: rifà senza e rimette il tasto com'era
	got, _ = traduciConMotore(ctx, motoreSegnaposti{perde: true}, RichiestaTraduzione{Testo: "Press CONTROL to crouch"})
	if got != "Premi CONTROL per accovacciarti" {
		t.Fatalf("segnaposto perso: %q", got)
	}
}

// "Stato: Error": se un pezzo non si traduce, niente etichetta mezza inglese.
func TestEtichettaMaiMezza(t *testing.T) {
	tr := NuovoTraduttore(filepath.Join(t.TempDir(), "m.json"), glossarioBase(), &motoreFinto{nome: "Mozilla", err: errSenzaMotore{}})
	r := tr.TraduciScritta(context.Background(), "Mating Status: Unknown Error")
	if r.Tradotto != "" && rigaMezzaTradotta("Mating Status: Unknown Error", r.Tradotto) {
		t.Fatalf("etichetta mezza tradotta: %q", r.Tradotto)
	}
	// primo giro "veloce": niente motore, la scritta resta da tradurre
	r = tr.TraduciScritta(soloVeloce(context.Background()), "The bridge collapsed behind you")
	if r.Tradotto != "" || r.Errore != errSoloVeloce {
		t.Fatalf("solo veloce: %+v", r)
	}
	// le frasi pronte escono anche nel giro veloce
	if r = tr.TraduciScritta(soloVeloce(context.Background()), "Quit game"); r.Tradotto != "Esci dal gioco" {
		t.Fatalf("frase pronta nel giro veloce: %+v", r)
	}
}

type errSenzaMotore struct{}

func (errSenzaMotore) Error() string { return "non pronto" }
