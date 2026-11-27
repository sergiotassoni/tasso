// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package nome

// Controlli sulla pagina: il nome entra solo con textContent e ogni messaggio d'errore ha il suo
// testo in tutte e 7 le lingue.

import (
	"os"
	"strings"
	"testing"
)

const percorsoTesti = "../ui/testi.js"

// Il nome non entra mai con innerHTML.
func TestPaginaNomeSoloTextContent(t *testing.T) {
	s := pagina(t)
	n := 0
	for i, r := range strings.Split(s, "\n") {
		if !strings.Contains(r, "nomeUtente") && !strings.Contains(r, "r.nome") {
			continue
		}
		n++
		if strings.Contains(r, "innerHTML") || strings.Contains(r, "insertAdjacentHTML") {
			t.Errorf("riga %d: il nome entra con innerHTML: %.140s", i+1, strings.TrimSpace(r))
		}
	}
	if n < 4 {
		t.Fatalf("il nome si usa in %d righe: manca qualcosa?", n)
	}
}

// Ogni codice d'errore ha la sua frase in tutte e 7 le lingue.
func TestTestiDegliErrori(t *testing.T) {
	b, err := os.ReadFile(percorsoTesti)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, c := range []string{Corto, Lungo, Simbolo, Invisibile, Senza, Accenti, CodiceRegistro} {
		k := chiaveErrore(c)
		if n := strings.Count(s, k); n != 7 {
			t.Errorf("%s: in %d lingue su 7", k, n)
		}
	}
}

func chiaveErrore(c string) string {
	return "\n    nomeErr" + strings.ToUpper(c[:1]) + c[1:] + ": "
}
