package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// A1 + X3: la prima scelta della modalità di Tasso (anche dall'installatore) non mette mai il PC in
// Gaming o FPS per sempre: con Gaming resta in Normale (piano energetico e notifiche come prima), e chi
// ha già un'altra modalità del PC non viene toccato.
func TestPrimoAvvioMaiGaming(t *testing.T) {
	casi := []struct{ profilo, prima, vuoi string }{
		{"gioco", "", "normale"},
		{"gioco", "normale", "normale"},
		{"gioco", "gaming", ""},
		{"gioco", "fps", ""},
		{"gioco", "film", ""},
		{"gioco", "originale", ""},
		{"ufficio", "normale", "normale"},
		{"entrambi", "normale", "normale"},
		{"boh", "normale", ""},
	}
	for _, c := range casi {
		if g := modoPrimoAvvio(c.profilo, c.prima); g != c.vuoi {
			t.Errorf("modoPrimoAvvio(%q, %q) = %q, voglio %q", c.profilo, c.prima, g, c.vuoi)
		}
	}
	b, err := os.ReadFile("settings.go")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?s)func profiloPC\(p string\) \{.*?\n\}`).Find(b)
	if m == nil {
		t.Fatal("profiloPC non trovata")
	}
	if strings.Contains(string(m), `"gaming"`) || strings.Contains(string(m), `"fps"`) {
		t.Error("profiloPC non deve mettere Gaming o FPS")
	}
}
