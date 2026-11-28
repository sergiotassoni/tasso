package main

import (
	"os"
	"strings"
	"testing"
)

// P3 (prova sul PC, 28/11): dopo "Rimetti com'era tutto" di Ottimizza tutto in "Torna indietro"
// restava "Impostazioni di Windows": la foto fatta dal giro di Ottimizza non si toglieva mai.
func TestFotoSoloOttDaTogliere(t *testing.T) {
	casi := []struct {
		daOtt     bool
		ott, priv int
		vuoi      bool
	}{
		{true, 0, 0, true},   // tutto rimesso: la foto fatta per Ottimizza non serve più
		{true, 1, 0, false},  // una modifica di Ottimizza è ancora attiva
		{true, 0, 2, false},  // la Privacy ha ancora valori di Tasso
		{false, 0, 0, false}, // la foto l'ha usata anche un'altra parte di Tasso: resta
	}
	for _, c := range casi {
		if got := fotoSoloOttDaTogliere(c.daOtt, c.ott, c.priv); got != c.vuoi {
			t.Errorf("%+v: %v", c, got)
		}
	}
}

// Solo Ottimizza tutto fa la foto "sua"; tutte le altre parti di Tasso usano ensureSnapshot (che
// toglie il segno "solo Ottimizza", così la loro foto non si perde mai).
func TestFotoOttSoloDaOttimizza(t *testing.T) {
	voluti := map[string]int{"ottimizza_giro.go": 1, "ottimizza_win.go": 2}
	files, _ := os.ReadDir(".")
	for _, f := range files {
		n := f.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") || n == "restore.go" {
			continue
		}
		b, err := os.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Count(string(b), "ensureSnapshotOtt()"); got != voluti[n] {
			t.Errorf("%s: ensureSnapshotOtt() %d volte, volevo %d", n, got, voluti[n])
		}
	}
	b, _ := os.ReadFile("ottimizza_win.go")
	if !strings.Contains(string(b), "togliFotoSoloOtt() // P3: \"Torna indietro\"") {
		t.Error("goOttRimettiTutto non toglie la foto fatta da Ottimizza")
	}
}

// P5: la scritta sotto "Traduci da solo appena apro un gioco" segue l'interruttore (acceso e spento).
func TestTraduciDaSoloNota(t *testing.T) {
	b, err := os.ReadFile("ui/index.html")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, x := range []string{`id="trSwAutoNota"`, "trAutoNota(!!st.tutto_auto)", "trAutoNota(!!r)", "_t('Acceso: Tasso traduce da solo"} {
		if !strings.Contains(s, x) {
			t.Errorf("manca %q", x)
		}
	}
}
