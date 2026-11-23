package traduce

import "testing"

func TestRifinisciENome(t *testing.T) {
	if g := rifinisci("RETRY", "Riprova"); g != "RIPROVA" {
		t.Fatal(g)
	}
	if g := rifinisci("CHECKING AGAIN AUTOMATICALLY", "Controllo di nuovo automaticamente."); g != "CONTROLLO DI NUOVO AUTOMATICAMENTE" {
		t.Fatal(g)
	}
	if g := rifinisci("Hello.", "Ciao."); g != "Ciao." {
		t.Fatal(g)
	}
	for _, s := range []string{"Ilia (s Bar needs the Steam client", "Lia (s Bar needs", "Liar's Bar needs"} {
		if g := correggiNomeGioco(s, "Liar's Bar"); g[:10] != "Liar's Bar" {
			t.Fatal(s, "->", g)
		}
	}
	if g := correggiNomeGioco("owns Lia (s Bar, then", "Liar's Bar"); g != "owns Liar's Bar, then" {
		t.Fatal(g)
	}
	if g := correggiNomeGioco("Start the bar now", "Liar's Bar"); g != "Start the bar now" {
		t.Fatal(g)
	}
}
