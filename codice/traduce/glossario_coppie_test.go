package traduce

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// F02-1: le parole fisse valgono per ogni coppia di lingue, non solo inglese → italiano.
type mozEco struct{ visto []string }

func (m *mozEco) Nome() string { return "Mozilla" }
func (m *mozEco) Traduci(ctx context.Context, r RichiestaTraduzione) (string, error) {
	return "IT:" + r.Testo, nil
}
func (m *mozEco) TraduciCoppia(ctx context.Context, testo, da, a string) (string, error) {
	m.visto = append(m.visto, testo)
	return "[" + a + "] " + testo, nil
}

func TestParoleFisseInOgniCoppia(t *testing.T) {
	m := &mozEco{}
	g := Glossario{NonTradurre: []string{"Geralt"}, Termini: map[string]string{"Health": "Salute"}}
	g = g.conTerminiPer("de", map[string]string{"Health": "Gesundheit"})
	tr := NuovoTraduttore("", g, m)
	tr.Coppia = func() (string, string) { return "en", "de" }
	ctx := context.Background()

	// scritta intera = parola fissa verso il tedesco (non quella verso l'italiano)
	if r := tr.TraduciScritta(ctx, "Health"); r.Tradotto != "Gesundheit" || r.Motore != "glossario" {
		t.Fatalf("Health → %q (%s)", r.Tradotto, r.Motore)
	}
	if r := tr.TraduciScritta(ctx, "HEALTH:"); r.Tradotto != "GESUNDHEIT:" {
		t.Fatalf("HEALTH: → %q", r.Tradotto)
	}
	// all'inizio di una scritta breve
	if r := tr.TraduciScritta(ctx, "Health potion"); !strings.Contains(r.Tradotto, "Gesundheit") {
		t.Fatalf("Health potion → %q", r.Tradotto)
	}
	// nome da non tradurre in mezzo alla frase: al motore arriva un nome finto, torna il nome
	r := tr.Traduci(ctx, "You must find Geralt before the night falls on the village.")
	if !strings.Contains(r.Tradotto, "Geralt") || strings.Contains(r.Tradotto, "Qzx") {
		t.Fatalf("nome da non tradurre: %q", r.Tradotto)
	}
	if ultimo := m.visto[len(m.visto)-1]; strings.Contains(ultimo, "Geralt") {
		t.Fatalf("il nome è arrivato al motore: %q", ultimo)
	}
	// verso il francese non ci sono parole fisse: Health si traduce col motore
	tr.Coppia = func() (string, string) { return "en", "fr" }
	if r := tr.TraduciScritta(ctx, "Health"); r.Motore == "glossario" {
		t.Fatalf("parola fissa del tedesco usata in francese: %q", r.Tradotto)
	}
	// cambio delle parole fisse: la memoria delle altre coppie si svuota
	tr.Coppia = func() (string, string) { return "en", "de" }
	tr.TraduciScritta(ctx, "Mana potion")
	tr.ImpostaGlossario(g.conTerminiPer("de", map[string]string{"Mana": "Zauberkraft"}))
	if r := tr.TraduciScritta(ctx, "Mana potion"); !strings.Contains(r.Tradotto, "Zauberkraft") {
		t.Fatalf("memoria vecchia dopo il cambio: %q", r.Tradotto)
	}
}

func TestGlossarioPerLinguaSalvato(t *testing.T) {
	g := Glossario{Termini: map[string]string{"HP": "PV"}}
	g = g.conTerminiPer("de", map[string]string{"HP": "LP"})
	b, _ := json.Marshal(g)
	var h Glossario
	if err := json.Unmarshal(b, &h); err != nil {
		t.Fatal(err)
	}
	if h.perArrivo("it").Termini["HP"] != "PV" || h.perArrivo("de").Termini["HP"] != "LP" || len(h.perArrivo("fr").Termini) != 0 {
		t.Fatalf("glossario per lingua: %s", b)
	}
	// il vecchio file (solo termini) resta valido per l'italiano
	var v Glossario
	_ = json.Unmarshal([]byte(`{"non_tradurre":["Geralt"],"termini":{"HP":"PV"}}`), &v)
	if v.perArrivo("it").Termini["HP"] != "PV" || v.perArrivo("de").NonTradurre[0] != "Geralt" {
		t.Fatal("file vecchio")
	}
}
