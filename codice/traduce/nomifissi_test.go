package traduce

import (
	"context"
	"strings"
	"testing"
)

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

func TestSchermataNelPrompt(t *testing.T) {
	ctx := ConSchermata(context.Background(), []string{"Sprint by Holding Shift", "Crouch with Ctrl", "Mouse"})
	r := RichiestaTraduzione{Testo: "Mouse", Schermata: schermataDa(ctx, false)}
	m := costruisciMessaggi(r)
	if !strings.Contains(m[1].Content, "Crouch with Ctrl") || !strings.HasSuffix(m[1].Content, "Testo da tradurre:\nMouse") {
		t.Fatalf("prompt senza schermata: %q", m[1].Content)
	}
	if schermataDa(ctx, true) != nil {
		t.Fatal("nei dialoghi niente schermata")
	}
}

func TestSchermataNellaCorrezione(t *testing.T) {
	m := costruisciMessaggi(RichiestaTraduzione{Testo: "Steam Discussions", Bozza: "Discussioni a vapore", Schermata: []string{"Bug Report", "Steam Discussions"}})
	if !strings.Contains(m[1].Content, "Bug Report") {
		t.Error("la correzione deve vedere la schermata")
	}
}

func TestEtichettaConMaiuscola(t *testing.T) {
	if comeMaiuscola("Mouse", "mouse") != "Mouse" || comeMaiuscola("mouse", "mouse") != "mouse" {
		t.Error("maiuscola")
	}
}

func TestTokenMassimi(t *testing.T) {
	if n := tokenMassimi(RichiestaTraduzione{Testo: "Apply", Bozza: "Applica"}); n != 24 {
		t.Fatalf("etichetta corta: %d", n)
	}
	lunga := strings.Repeat("parola ", 40) // 280 lettere
	n := tokenMassimi(RichiestaTraduzione{Testo: lunga, Bozza: lunga})
	if n < 200 || n > 400 {
		t.Fatalf("frase lunga: %d", n)
	}
}

func TestPulisciPrefissoBozza(t *testing.T) {
	if g := pulisciRisposta("Traduzione automatica: Lingua dell'interfaccia"); g != "Lingua dell'interfaccia" {
		t.Fatal(g)
	}
}
