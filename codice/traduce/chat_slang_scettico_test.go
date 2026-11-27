package traduce

import "testing"

func TestAbbreviazioniContesto(t *testing.T) {
	for in, vuole := range map[string]string{
		"see u at 9 pm":   "see you at 9 pm",
		"pm me pls":       "message me please",
		"press R to drop": "press R to drop",
		"u r late":        "you are late",
	} {
		if got := espandiAbbreviazioni(in); got != vuole {
			t.Errorf("%q: vuole %q, ho %q", in, vuole, got)
		}
	}
}
