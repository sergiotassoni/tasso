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
