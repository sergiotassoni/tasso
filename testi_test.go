// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// chiaviLingue: le chiavi di ogni lingua in un file di testi (blocchi "  xx: {" ... "  },").
func chiaviLingue(t *testing.T, file string) map[string]map[string]bool {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]bool{}
	reBlocco := regexp.MustCompile(`(?m)^  (it|en|es|fr|de|pt|pl): \{`)
	reChiave := regexp.MustCompile(`(?m)(?:^\s+|["'\x60],\s*)([A-Za-z_][A-Za-z0-9_]*):\s*["'\x60]`)
	idx := reBlocco.FindAllStringSubmatchIndex(string(b), -1)
	for i, m := range idx {
		fine := len(b)
		if i+1 < len(idx) {
			fine = idx[i+1][0]
		}
		l := string(b[m[2]:m[3]])
		out[l] = map[string]bool{}
		for _, k := range reChiave.FindAllStringSubmatch(string(b[m[1]:fine]), -1) {
			out[l][k[1]] = true
		}
	}
	return out
}

// Tutte le lingue hanno tutte le chiavi, e ogni testo usato dall'interfaccia esiste.
func TestTestiCompleti(t *testing.T) {
	tutte := map[string]map[string]bool{}
	for _, f := range []string{"ui/testi.js", "ui/testi_doc.js"} {
		for l, kk := range chiaviLingue(t, f) {
			if tutte[l] == nil {
				tutte[l] = map[string]bool{}
			}
			for k := range kk {
				tutte[l][k] = true
			}
		}
	}
	if len(tutte) != 7 {
		t.Fatalf("lingue trovate: %d", len(tutte))
	}
	for l, kk := range tutte {
		var manca []string
		for k := range tutte["it"] {
			if !kk[k] {
				manca = append(manca, k)
			}
		}
		for k := range kk {
			if !tutte["it"][k] {
				manca = append(manca, "(in più) "+k)
			}
		}
		sort.Strings(manca)
		if len(manca) > 0 {
			t.Errorf("%s: %s", l, strings.Join(manca, ", "))
		}
	}
	// chiavi usate nella pagina
	var usate []string
	for _, f := range []string{"ui/app.js", "ui/documento.js", "ui/index.html"} {
		b, _ := os.ReadFile(f)
		for _, re := range []string{`\bt\("([A-Za-z_0-9]+)"`, `data-tt?="([A-Za-z_0-9]+)"`} {
			for _, m := range regexp.MustCompile(re).FindAllStringSubmatch(string(b), -1) {
				usate = append(usate, m[1])
			}
		}
	}
	b, _ := os.ReadFile("ui/documento.js")
	doc := string(b)
	for _, m := range regexp.MustCompile(`\{ id: "([a-zA-Z]+)"`).FindAllStringSubmatch(doc, -1) {
		usate = append(usate, "s_"+m[1], "d_"+m[1])
	}
	for _, re := range []string{`bottone\([^,]+, "(\w+)"`, `spunta\(\w+, "(\w+)"`, `testoP\(\w+, "(\w+)"`, `campoPagine\(\w+, "(\w+)"`, `g: "(\w+)"`, `\["(r_\w+)", "\w+"\]`} {
		for _, m := range regexp.MustCompile(re).FindAllStringSubmatch(doc, -1) {
			usate = append(usate, m[1])
		}
	}
	for _, m := range regexp.MustCompile(`\[\s*"(\w+)", "\w+"\s*\]`).FindAllStringSubmatch(doc[strings.Index(doc, "const RAPIDI"):], 9) {
		usate = append(usate, "r_"+m[1])
	}
	for _, k := range usate {
		if strings.HasSuffix(k, "_") || k == "text" {
			continue
		}
		if !tutte["it"][k] {
			t.Errorf("testo usato ma non scritto: %s", k)
		}
	}
	// i codici di errore del programma hanno il loro testo
	for _, f := range []string{"modifica.go", "strumenti.go", "app.go", "avvio.go", "pdfops.go"} {
		b, _ := os.ReadFile(f)
		for _, m := range regexp.MustCompile(`errc\("([a-z_]+)"`).FindAllStringSubmatch(string(b), -1) {
			if m[1] != "lingua" && !tutte["it"]["e_"+m[1]] {
				t.Errorf("errore %q senza testo (in %s)", m[1], f)
			}
		}
	}
}

func TestTestiMenu(t *testing.T) {
	for l := range lingueNote {
		for _, k := range []string{"apri", "unisci", "firma", "sblocca", "comprimi"} {
			if testiMenu[l][k] == "" || !strings.Contains(testiMenu[l][k], "Tasso PDF") {
				t.Errorf("menu %s/%s", l, k)
			}
		}
	}
}
