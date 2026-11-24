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

func TestStaccaEAccenti(t *testing.T) {
	if vocabolario() == nil || len(vocLiv) == 0 {
		t.Skip("vocabolario non presente")
	}
	if g := correggiParole("Join MultiplayerGame (Invite Codë)"); g != "Join Multiplayer Game (Invite Code)" {
		t.Fatalf("%q", g)
	}
	if g := correggiParole("Open PlayStation Store"); g != "Open PlayStation Store" {
		t.Fatalf("marchio staccato: %q", g)
	}
	if g := correggiParole("Global Palbox"); g != "Global Palbox" {
		t.Fatalf("nome toccato: %q", g)
	}
}

func TestNomiIntatti(t *testing.T) {
	if vocabolario() == nil || len(vocLiv) == 0 {
		t.Skip("vocabolario non presente")
	}
	caricaCataloghi()
	if g := nomiIntatti("Global Palbox", "Palibox globale"); g != "Palbox globale" {
		t.Fatalf("%q", g)
	}
	if g := nomiIntatti("Options", "Opzioni"); g != "Opzioni" {
		t.Fatalf("%q", g)
	}
	if g := nomiIntatti("Talk to Geralt", "Parla con Geralt"); g != "Parla con Geralt" {
		t.Fatalf("%q", g)
	}
}

func TestJoinTagliatoENome(t *testing.T) {
	if g := correggiParole("in Multiplayer Game (Invite Code)"); g != "Join Multiplayer Game (Invite Code)" {
		t.Fatal(g)
	}
	if g := correggiParole("in the house"); g != "in the house" {
		t.Fatal(g)
	}
	if daTradurre("1LMESS1A (STEAM)") || daTradurre("XX4LEX") {
		t.Fatal("nome tradotto")
	}
	if !daTradurre("Start Game") {
		t.Fatal("menu non tradotto")
	}
}
