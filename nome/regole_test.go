// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package nome

// Le regole del nome sono le stesse in 8 posti: Go e pagina di Tasso, installatore, Tasso Zip e
// Tasso PDF. Qui si controlla che il pezzo in Go e quello nella pagina abbiano l'impronta comune
// (le stesse costanti stanno nei test degli altri 3 programmi: se passano tutti, le 8 copie sono
// uguali) e, con node, che la pagina dia lo stesso esito di Go su tutti i caratteri di Unicode.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"unicode/utf8"
)

const (
	impronteRegoleGo = "fb61622c982843d42885b391d396c2d53dde50e42cc5b49086980b0e91721670"
	impronteRegoleJS = "1124eda3fe80099c7da69db9d7d595a0d67404e292f6aceaead7cfd0aa6e8033"
	percorsoPagina   = "../ui/index.html"
)

func pezzo(t *testing.T, s, inizio, fine string) string {
	t.Helper()
	a := strings.Index(s, inizio)
	b := strings.Index(s, fine)
	if a < 0 || b < a {
		t.Fatalf("pezzo %q non trovato", inizio)
	}
	return s[a : b+len(fine)+1]
}

func impronta(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func pagina(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(percorsoPagina)
	if err != nil {
		t.Fatalf("pagina: %v", err)
	}
	return string(b)
}

func TestRegoleUgualiNelleCopie(t *testing.T) {
	b, err := os.ReadFile("nome.go")
	if err != nil {
		t.Fatal(err)
	}
	if h := impronta(pezzo(t, string(b), "// ==== REGOLE DEL NOME: INIZIO ====", "// ==== REGOLE DEL NOME: FINE ====")); h != impronteRegoleGo {
		t.Errorf("le regole in Go non sono più quelle comuni (impronta %s): cambiale in tutti e 4 i programmi e aggiorna l'impronta", h)
	}
	if h := impronta(pezzo(t, pagina(t), "/* ==== REGOLE DEL NOME: INIZIO ====", "/* ==== REGOLE DEL NOME: FINE ==== */")); h != impronteRegoleJS {
		t.Errorf("le regole della pagina non sono più quelle comuni (impronta %s)", h)
	}
}

// Tutti i caratteri di Unicode (tranne i surrogati), uno per volta dentro "a?b", più dei casi
// scelti: la pagina e Go devono dare lo stesso codice (e lo stesso simbolo).
func TestPaginaControllaComeGo(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node non c'è")
	}
	js := pezzo(t, pagina(t), "/* ==== REGOLE DEL NOME: INIZIO ====", "/* ==== REGOLE DEL NOME: FINE ==== */")
	casi := []string{"Sergio", "  Gian   Luca ", "S", "", "Sergio★", "<b>x</b>", "Niccolò", "José-María", "Анна",
		"e\u0301va", "\u0301ab", "a\u0301\u0302\u0303\u0304", "a \u0301", "ㅤㅤ", "Sergio\ufe0f", "\tAnna\u00a0Maria\n", "\ufeffLuca", strings.Repeat("è", 21)}
	in, _ := json.Marshal(casi)
	prog := js + `
const C = ` + string(in) + `;
const out = C.map(x => { const p = nomePulisci(x), e = nomeControlla(p); return [p, e.codice, e.simbolo || ""]; });
const M = {"": "o", corto: "c", lungo: "l", simbolo: "s", invisibile: "i", senza: "n", accenti: "a"};
let tutti = "";
for (let k = 0; k <= 0x10FFFF; k++) { if (k >= 0xD800 && k <= 0xDFFF) { tutti += "-"; continue; } const e = nomeControlla("a" + String.fromCodePoint(k) + "b"); tutti += M[e.codice]; }
process.stdout.write(JSON.stringify({out, tutti}));`
	f := t.TempDir() + "/prova.js"
	if err := os.WriteFile(f, []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := exec.Command(node, f).Output()
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	var r struct {
		Out   [][3]string `json:"out"`
		Tutti string      `json:"tutti"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatalf("risposta di node: %v", err)
	}
	for i, c := range casi {
		p, e := DaSalvare(c)
		if r.Out[i] != [3]string{p, e.Codice, e.Simbolo} {
			t.Errorf("%q: pagina %q, Go %q", c, r.Out[i], [3]string{p, e.Codice, e.Simbolo})
		}
	}
	M := map[string]byte{OK: 'o', Corto: 'c', Lungo: 'l', Simbolo: 's', Invisibile: 'i', Senza: 'n', Accenti: 'a'}
	if utf8.RuneCountInString(r.Tutti) != 0x110000 {
		t.Fatalf("node ha controllato %d caratteri", len(r.Tutti))
	}
	diversi := 0
	for k := rune(0); k <= 0x10FFFF; k++ {
		if k >= 0xD800 && k <= 0xDFFF {
			continue
		}
		if g := M[Controlla("a"+string(k)+"b").Codice]; g != r.Tutti[k] {
			if diversi++; diversi <= 10 {
				t.Errorf("U+%04X: pagina %c, Go %c", k, r.Tutti[k], g)
			}
		}
	}
	if diversi > 0 {
		t.Errorf("%d caratteri con esito diverso", diversi)
	}
}
