package traduce

import "testing"

func TestNomiFissiIntatti(t *testing.T) {
	casi := []struct{ en, it, vuoi string }{
		{"Hello Games has been hard at work.", "Ciao Games è stato al lavoro.", "Hello Games è stato al lavoro."},
		{"construct an eerie corvette in No Man's Sky Forsaken.", "costruisci una corvetta inquietante Sky Forsaken di No Man.", "costruisci una corvetta inquietante No Man's Sky Forsaken."},
		{"expand No Man's Sky.", "espandere No Man's Sky.", "espandere No Man's Sky."},
		{"Nothing here.", "Ciao Games.", "Ciao Games."},
	}
	for _, c := range casi {
		if got := nomiFissiIntatti(c.en, c.it); got != c.vuoi {
			t.Errorf("%q -> %q, voglio %q", c.it, got, c.vuoi)
		}
	}
}

func TestNomeGiocoNonMangiaParole(t *testing.T) {
	casi := []struct{ in, vuoi string }{
		{"construct an eerie corvette in No Man's Sky Forsaken.", "construct an eerie corvette in No Man's Sky Forsaken."},
		{"expand No Man's Sky. We hope you enjoy.", "expand No Man's Sky. We hope you enjoy."},
		{"expand No Mans Sky. We hope", "expand No Man's Sky. We hope"},
	}
	for _, c := range casi {
		if got := correggiNomeGioco(c.in, "No Man's Sky"); got != c.vuoi {
			t.Errorf("%q -> %q, voglio %q", c.in, got, c.vuoi)
		}
	}
}
