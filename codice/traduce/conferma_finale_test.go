package traduce

import "testing"

func TestConfermaFinaleNumeriEChat(t *testing.T) {
	for in, want := range map[string]string{
		"Damage increased by 1096": "Damage increased by 10%",
		"Founded by 1996":          "Founded by 1996",
		"Increased 1096":           "Increased 10%",
	} {
		if got := correggiNumeriOCR(in); got != want {
			t.Errorf("%q: %q, volevo %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"R U OK":   "are you OK",
		"U THERE?": "you THERE?",
		"premi R":  "premi R",
		"PRESS R":  "PRESS R",
	} {
		if got := espandiAbbreviazioni(in); got != want {
			t.Errorf("%q: %q, volevo %q", in, got, want)
		}
	}
}
