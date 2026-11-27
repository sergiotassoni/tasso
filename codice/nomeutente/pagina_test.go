package nomeutente

// Controlli sulla finestra di Tasso (ui/index.html): il nome entra sempre con esc() o textContent,
// e la funzione nomeControlla della pagina dà gli stessi esiti di Controlla (con node, se c'è).

import (
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func pagina(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../ui/index.html")
	if err != nil {
		t.Skip("index.html non trovato")
	}
	return string(b)
}

// Ogni riga che mette il nome dentro dell'HTML lo passa da esc().
func TestPaginaNomeSempreConEsc(t *testing.T) {
	s := pagina(t)
	usi := 0
	for i, riga := range strings.Split(s, "\n") {
		if !strings.Contains(riga, "nomeOra()") && !strings.Contains(riga, "nomeSulBiglietto()") {
			continue
		}
		usi++
		html := strings.Contains(riga, "innerHTML") || strings.Contains(riga, "insertAdjacentHTML") || strings.Contains(riga, "modal(") || strings.Contains(riga, "'<")
		if html && !strings.Contains(riga, "esc(") {
			t.Errorf("riga %d: il nome entra nell'HTML senza esc(): %.160s", i+1, strings.TrimSpace(riga))
		}
	}
	if usi < 8 {
		t.Fatalf("il nome si usa in %d righe: manca qualcosa?", usi)
	}
	// i saluti con il nome vanno in textContent (o in toast, che fa esc da solo)
	for _, f := range []string{"if (e && n) e.textContent = (", `pill.textContent = n ||`} {
		if !strings.Contains(s, f) {
			t.Errorf("non trovo %q", f)
		}
	}
}

// La pagina e Go controllano il nome allo stesso modo.
func TestPaginaControllaComeGo(t *testing.T) {
	s := pagina(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node non c'è")
	}
	m := regexp.MustCompile(`(?s)function nomePulisci\(s\)\{.*?\n\}\n// La frase gentile`).FindString(s)
	if m == "" {
		t.Fatal("nomeControlla non trovata in index.html")
	}
	m = strings.TrimSuffix(m, "// La frase gentile")
	casi := []string{"Sergio", "  Gian   Luca ", "S", "", "Sergio★", "<b>x</b>", "Niccolò", "José-María", "Zoë_92", "Анна", "李小龍",
		"R2-D2", "a.b", "..", "-_ .", "Ser\u200bgio", "Ser\u202egio", "e\u0301va", "\u0301ab", strings.Repeat("a", 20), strings.Repeat("è", 21),
		"Sergio😀", "a&b", "Ser'gio", "\tAnna\u00a0Maria\n", "ÀÈÌÒÙ àèìòù"}
	in, _ := json.Marshal(casi)
	js := m + "\nconst C=" + string(in) + ";console.log(JSON.stringify(C.map(x=>{const p=nomePulisci(x);const e=nomeControlla(p);return [p,e.codice,e.simbolo||'']})));"
	out, err := exec.Command(node, "-e", js).Output()
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	var got [][3]string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("risposta di node: %v (%s)", err, out)
	}
	for i, c := range casi {
		p, e := DaSalvare(c)
		if got[i] != [3]string{p, e.Codice, e.Simbolo} {
			t.Errorf("%q: pagina %q, Go %q", c, got[i], [3]string{p, e.Codice, e.Simbolo})
		}
	}
}
