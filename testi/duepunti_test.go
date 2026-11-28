package testi

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// RECENSIONE-4c: i due punti che la pagina mette tra due pezzi di testo passano da dp(), che in
// francese mette lo spazio non separabile davanti (come nei testi).
func TestDuePuntiComposti(t *testing.T) {
	b, err := os.ReadFile("../ui/index.html")
	if err != nil {
		t.Skip("index.html non trovato")
	}
	pagina := string(b)
	if !strings.Contains(pagina, `function dp() { return S.lingua === "fr" ? "\u00a0: " : ": "; }`) {
		t.Error("index.html: manca dp() con lo spazio non separabile per il francese")
	}
	composta := regexp.MustCompile(`[+?] ": " [+:)]`)
	for _, riga := range strings.Split(pagina, "\n") {
		if m := composta.FindStringIndex(riga); m != nil {
			t.Errorf("index.html: due punti composti senza dp(): …%s…", riga[max(0, m[0]-60):min(len(riga), m[1]+30)])
		}
	}
}

// Nei testi francesi di testi.json la punteggiatura segue la stessa regola di Tasso: spazio non
// separabile (U+00A0) prima di ":" e di "»" e dopo "«", spazio stretto (U+202F) prima di ";", "!" e "?".
func TestFrancesePunteggiatura(t *testing.T) {
	prima := regexp.MustCompile(`[ \t]+[;!?:»]([^0-9A-Za-zÀ-ÿ]|$)`)
	dopo := regexp.MustCompile(`«[ \t]`)
	stretto := regexp.MustCompile(`\x{00A0}[;!?]`)
	voci, ok := Tutti()["fr"]
	if !ok || len(voci) == 0 {
		t.Fatal("testi.json: manca il francese")
	}
	n := 0
	for k, v := range voci {
		if prima.MatchString(v) || dopo.MatchString(v) || stretto.MatchString(v) {
			n++
			if n <= 20 {
				t.Errorf("testi.json/fr: spazio sbagliato nella punteggiatura in %q = %q", k, v)
			}
		}
	}
	if n > 20 {
		t.Errorf("testi.json/fr: in tutto %d frasi da sistemare", n)
	}
}
