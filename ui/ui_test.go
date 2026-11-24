package ui

import (
	"strings"
	"testing"

	"tassosetup/testi"
)

func TestChiaviDellaPagina(t *testing.T) {
	ch := Chiavi()
	if len(ch) < 40 {
		t.Fatalf("trovate solo %d chiavi: l'espressione regolare non funziona?", len(ch))
	}
	for _, k := range ch {
		for _, l := range testi.Lingue {
			if _, ok := testi.Dizionario[l.Codice][k]; !ok {
				t.Errorf("la pagina usa %q, che manca in %s", k, l.Codice)
			}
		}
	}
}

func TestPaginaCompleta(t *testing.T) {
	p := Pagina()
	if strings.Contains(p, "{{ICONA") || strings.Contains(p, "/*CARATTERI*/") {
		t.Fatal("segnaposto rimasti nella pagina")
	}
	if !strings.Contains(p, "TassoArchivo") || strings.Count(p, "data:image/svg+xml;base64,") < 7 {
		t.Fatal("caratteri o icone mancanti")
	}
	if strings.Contains(p, "http://") || strings.Contains(p, "https://") {
		// solo gli spazi dei nomi XML delle icone sono ammessi
		for _, r := range strings.Split(p, "\"") {
			if strings.HasPrefix(r, "http") && !strings.HasPrefix(r, "http://www.w3.org/") {
				t.Fatalf("la pagina carica qualcosa da internet: %s", r)
			}
		}
	}
	if len(p) > 1800000 {
		t.Fatalf("pagina troppo grande per NavigateToString: %d byte", len(p))
	}
}

func TestPaginaAggiorna(t *testing.T) {
	p := PaginaAggiorna("Aggiorno <Tasso>", "Chiudo & aggiorno")
	if strings.Contains(p, "{{") || strings.Contains(p, "/*CARATTERI*/") || !strings.Contains(p, "TassoArchivo") {
		t.Fatal("segnaposto rimasti o caratteri mancanti")
	}
	if !strings.Contains(p, "Aggiorno &lt;Tasso&gt;") || !strings.Contains(p, "Chiudo &amp; aggiorno") {
		t.Fatal("testi non messi o non protetti")
	}
	if strings.Contains(p, "https://") {
		t.Fatal("la finestrella carica qualcosa da internet")
	}
}
