package nome

// Controlli sulla pagina: nomeControlla (JavaScript) dà gli stessi esiti di Controlla (con node, se
// c'è), il nome entra solo con textContent e ogni messaggio d'errore ha il suo testo in tutte le lingue.

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
	b, err := os.ReadFile(percorsoPagina)
	if err != nil {
		t.Skip("pagina non trovata")
	}
	return string(b)
}

func TestPaginaControllaComeGo(t *testing.T) {
	s := pagina(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node non c'è")
	}
	m := regexp.MustCompile(`(?s)function nomePulisci\(s\) \{.*?\n// Fine delle regole`).FindString(s)
	if m == "" {
		t.Fatal("nomeControlla non trovata nella pagina")
	}
	m = strings.TrimSuffix(m, "// Fine delle regole")
	casi := []string{"Sergio", "  Gian   Luca ", "S", "", "Sergio★", "<b>x</b>", "Niccolò", "José-María", "Zoë_92", "Анна", "李小龍",
		"R2-D2", "a.b", "..", "-_ .", "Ser\u200bgio", "Ser\u202egio", "e\u0301va", "\u0301ab", strings.Repeat("a", 20), strings.Repeat("è", 21),
		"Sergio😀", "a&b", "Ser'gio", "\tAnna\u00a0Maria\n"}
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

// Il nome non entra mai con innerHTML.
func TestPaginaNomeSoloTextContent(t *testing.T) {
	s := pagina(t)
	n := 0
	for i, r := range strings.Split(s, "\n") {
		if !strings.Contains(r, "S.nome") && !strings.Contains(r, "r.nome") {
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

// Ogni codice d'errore ha la sua frase in tutte le lingue.
func TestTestiDegliErrori(t *testing.T) {
	b, err := os.ReadFile(percorsoTesti)
	if err != nil {
		t.Skip("testi non trovati")
	}
	var d map[string]map[string]string
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	for l, m := range d {
		for _, c := range []string{Corto, Lungo, Simbolo, Invisibile, Senza, CodiceRegistro} {
			if strings.TrimSpace(m["nome_err_"+c]) == "" {
				t.Errorf("%s: manca nome_err_%s", l, c)
			}
		}
		for _, k := range []string{"domanda_nome", "nome_salvato"} {
			if !strings.Contains(m[k], "{n}") {
				t.Errorf("%s/%s: manca {n}", l, k)
			}
		}
		if !strings.Contains(m["nome_err_simbolo"], "{s}") {
			t.Errorf("%s/nome_err_simbolo: manca {s}", l)
		}
	}
}

const (
	percorsoPagina = "../ui/index.html"
	percorsoTesti  = "../testi/testi.json"
)
