// Package ui: la pagina dell'installatore (HTML, caratteri, icone animate).
package ui

import (
	"embed"
	"encoding/base64"
	"html"
	"regexp"
	"sort"
	"strings"
)

//go:embed index.html aggiorna.html caratteri.css icone/*.svg
var file embed.FS

// Icone animate (occhi che si muovono) di ogni componente.
var Icone = []string{"gaming", "ufficio", "zip", "pdf"}

// Pagina restituisce l'HTML completo, con caratteri e icone dentro (niente file esterni).
func Pagina() string {
	b, _ := file.ReadFile("index.html")
	s := string(b)
	css, _ := file.ReadFile("caratteri.css")
	s = strings.Replace(s, "/*CARATTERI*/", string(css), 1)
	for _, n := range Icone {
		svg, _ := file.ReadFile("icone/" + n + ".svg")
		s = strings.ReplaceAll(s, "{{ICONA:"+n+"}}", "data:image/svg+xml;base64,"+base64.StdEncoding.EncodeToString(svg))
	}
	return s
}

// PaginaAggiorna: la finestrella con la barra dell'aggiornamento silenzioso
// (TassoSetup.exe --aggiorna-silenzioso). Titolo e primo testo arrivano già tradotti.
func PaginaAggiorna(titolo, testo string) string {
	b, _ := file.ReadFile("aggiorna.html")
	s := string(b)
	css, _ := file.ReadFile("caratteri.css")
	s = strings.Replace(s, "/*CARATTERI*/", string(css), 1)
	s = strings.Replace(s, "{{TITOLO}}", html.EscapeString(titolo), 1)
	return strings.Replace(s, "{{TESTO}}", html.EscapeString(testo), 1)
}

var (
	reDataT = regexp.MustCompile(`data-t[pa]?="([a-z0-9_]+)"`) // data-tp: il segnaposto; data-ta: l'etichetta per chi non vede
	reT     = regexp.MustCompile(`\bT\('([a-z0-9_]+)'[,)]`)
	reNome  = regexp.MustCompile(`'(nome_[a-z_]+)'`)
	reComp  = regexp.MustCompile(`'comp_' \+ [a-z.]+ \+ '_([a-z]+)'`)
	reFase  = regexp.MustCompile(`'fase_' \+ a\.fase`)
)

// Chiavi elenca le chiavi dei testi usate dalla pagina (per il test del dizionario).
// Le chiavi composte (comp_<id>_nome, fase_<fase>) si espandono con i valori possibili.
func Chiavi() []string {
	b, _ := file.ReadFile("index.html")
	s := string(b)
	set := map[string]bool{}
	for _, m := range reDataT.FindAllStringSubmatch(s, -1) {
		set[m[1]] = true
	}
	for _, m := range reT.FindAllStringSubmatch(s, -1) {
		set[m[1]] = true
	}
	for _, m := range reNome.FindAllStringSubmatch(s, -1) { // "Come ti chiami?": NOME_ERR
		set[m[1]] = true
	}
	for _, m := range reComp.FindAllStringSubmatch(s, -1) {
		for _, id := range Icone {
			set["comp_"+id+"_"+m[1]] = true
		}
	}
	if reFase.MatchString(s) {
		for _, f := range []string{"prepara", "copia", "sposta", "registro", "collegamenti"} {
			set["fase_"+f] = true
		}
	}
	var out []string
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
