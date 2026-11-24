package traduce

import "testing"

func TestCorreggiNumeriOCR(t *testing.T) {
	casi := [][2]string{
		{"- Health Regen Increased 6.796", "- Health Regen Increased 6.7%"},
		{"- Locked Health Regen Increased 1096", "- Locked Health Regen Increased 10%"},
		{"- Stamina Regen Increased IS%", "- Stamina Regen Increased 15%"},
		{"LOW RULES - 1.2Sx - STORAGE", "LOW RULES - 1.25x - STORAGE"},
		{"Players 18S/42S", "Players 185/425"},
		{"You have 1996 gold", "You have 1996 gold"},
		{"Level IS", "Level IS"},
		{"Score 2096 points", "Score 2096 points"},
	}
	for _, c := range casi {
		if g := correggiNumeriOCR(c[0]); g != c[1] {
			t.Errorf("%q -> %q, voleva %q", c[0], g, c[1])
		}
	}
}
