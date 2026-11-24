package traduce

import (
	"reflect"
	"testing"
)

func TestPulisci(t *testing.T) {
	casi := map[string]string{
		"You must find the\nancient sword before\nnightfall.": "You must find the ancient sword before nightfall.",
		"Hello there.\nWho are you?":                          "Hello there.\nWho are you?",
		"an incred-\nible journey.":                           "an incredible journey.",
		"| think this is g0od , friend.":                      "I think this is good, friend.",
		"@#%\n~~\nGo north.":                                  "Go north.",
		"It’s “fine”.":                                        `It's "fine".`,
		"":                                                    "",
	}
	for in, atteso := range casi {
		if got := pulisci(in); got != atteso {
			t.Errorf("pulisci(%q) = %q, atteso %q", in, got, atteso)
		}
	}
}

func TestSeparaParlante(t *testing.T) {
	casi := []struct{ in, nome, battuta string }{
		{"Captain Price: Move out!", "Captain Price", "Move out!"},
		{"Just text here.", "", "Just text here."},
		{"Old Keeper:\nThe bridge collapsed.", "", "Old Keeper:\nThe bridge collapsed."},
		{"Warning: hull integrity at 30 percent.", "", "Warning: hull integrity at 30 percent."},
		{"Quest updated: Find a way.", "", "Quest updated: Find a way."},
	}
	for _, c := range casi {
		n, b := separaParlante(c.in)
		if n != c.nome || b != c.battuta {
			t.Errorf("separaParlante(%q) = (%q,%q), atteso (%q,%q)", c.in, n, b, c.nome, c.battuta)
		}
	}
}

func TestGlossarioTrova(t *testing.T) {
	g := Glossario{NonTradurre: []string{"Geralt"}, Termini: map[string]string{"HP": "PV", "Side Quest": "Missione secondaria", "Quest": "Missione"}}
	got := g.trova("Geralt lost 20 HP during the side quest.")
	atteso := []voceTrovata{{"Side Quest", "Missione secondaria"}, {"Geralt", "Geralt"}, {"HP", "PV"}}
	if !reflect.DeepEqual(got, atteso) {
		t.Errorf("trova = %v, atteso %v", got, atteso)
	}
	if v := g.trova("HPotion"); len(v) != 0 {
		t.Errorf("non deve trovare parti di parola: %v", v)
	}
}
