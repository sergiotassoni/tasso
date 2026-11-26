package main

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// La domanda "Com'è andata la prima pulizia?" (una volta sola) non va fatta quando la pulizia finisce
// con Riavvia/Spegni: l'utente non c'è e il PC si riavvia, la domanda andrebbe persa per sempre.
func TestFeedbackNonSuRiavvio(t *testing.T) {
	b, err := os.ReadFile("ui/index.html")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`function fbAskOnce\(p\)\{[\s\S]*?S\.feedbackAsked = true`).Find(b)
	if m == nil {
		t.Fatal("fbAskOnce non trovata")
	}
	for _, c := range []string{"p.shutdown", "p.restart", "p.endErr", "p.cancelled"} {
		if !regexp.MustCompile(regexp.QuoteMeta(c)).Match(m) {
			t.Errorf("fbAskOnce non esclude %s", c)
		}
	}
}

// Il registro (.log) nei temi chiari era grigio scuro con testo grigio (contrasto 2,6): lo sfondo
// resta quasi quello della pagina, così il testo si legge in tutti i temi.
func TestRegistroLeggibile(t *testing.T) {
	b, err := os.ReadFile("ui/index.html")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`\.log\{[^}]*background:color-mix\(in srgb,var\(--bg\) (\d+)%,#000\)`).FindSubmatch(b)
	if m == nil {
		t.Fatal(".log non trovato")
	}
	if n, _ := strconv.Atoi(string(m[1])); n < 85 {
		t.Errorf("sfondo del registro troppo scuro nei temi chiari: bg %s%%", m[1])
	}
}
