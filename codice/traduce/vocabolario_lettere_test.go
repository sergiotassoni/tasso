package traduce

import "testing"

func TestLettereIniziali(t *testing.T) {
	if vocabolario() == nil || len(vocLiv) == 0 {
		t.Skip("vocabolario non presente")
	}
	if g := correggiParole("Oin Multiplayer Game (Invite Code)"); g != "Join Multiplayer Game (Invite Code)" {
		t.Fatalf("J persa: %q", g)
	}
	if g := correggiParole("Options"); g != "Options" {
		t.Fatalf("parola giusta toccata: %q", g)
	}
}
