package traduce

import (
	"strings"
	"unicode"
)

// sottotitoloPronto: con la zona dei sottotitoli il testo si legge ogni mezzo secondo.
//   - Nei video lo sfondo si muove e l'OCR sbaglia qualche lettera a ogni lettura ("Welhave",
//     "thestorm", prova del 24/11): se è lo stesso sottotitolo di prima non lo ritraduco.
//   - Un sottotitolo che sta comparendo (dissolvenza, parole una alla volta) si legge a metà:
//     traduco quando due letture di fila dicono (quasi) la stessa cosa.
//
// Restituisce se tradurre adesso e la lettura da tenere come candidata per il giro dopo.
func sottotitoloPronto(letto, ultimo, candidato string) (bool, string) {
	if ultimo != "" && stessoSottotitolo(letto, ultimo) {
		return false, candidato
	}
	if !stessoSottotitolo(letto, candidato) {
		return false, letto
	}
	return true, letto
}

// stessoSottotitolo: le due letture sono lo stesso sottotitolo letto con qualche errore.
// Conto solo lettere e cifre (spazi e punteggiatura l'OCR li sbaglia spesso); i numeri e le
// negazioni devono essere uguali; le frasi corte devono essere identiche (lì una lettera
// cambia il senso: "I'm here" / "I'm there").
func stessoSottotitolo(a, b string) bool {
	if a == b {
		return a != ""
	}
	if a == "" || b == "" || cifre(a) != cifre(b) || negazioniDi(a) != negazioniDi(b) {
		return false
	}
	na, nb := lettereCifre(a), lettereCifre(b)
	l := max(len(na), len(nb))
	if l < 12 {
		return string(na) == string(nb)
	}
	return distanzaTesti(na, nb, l/5) <= l/5
}

func lettereCifre(s string) []rune {
	var out []rune
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out = append(out, r)
		}
	}
	return out
}

func negazioniDi(s string) string {
	var out []string
	for _, w := range strings.Fields(strings.ToLower(s)) {
		if w = strings.Trim(w, ".,!?;:\"'"); negazioni[w] || strings.HasSuffix(w, "n't") {
			out = append(out, w)
		}
	}
	return strings.Join(out, " ")
}

// righeSottotitolo: nei video l'OCR legge a volte pezzetti dello sfondo ("o — c") sopra il
// sottotitolo, e ogni volta diversi. Se c'è almeno una riga vera (due parole di 3 lettere o più)
// tolgo le righe senza nessuna parola di 3 lettere; un sottotitolo corto da solo ("No.") resta.
func righeSottotitolo(t string) string {
	righe := strings.Split(t, "\n")
	parole := func(r string) int {
		n := 0
		for _, w := range strings.Fields(r) {
			l := 0
			for _, c := range w {
				if unicode.IsLetter(c) {
					l++
				}
			}
			if l >= 3 {
				n++
			}
		}
		return n
	}
	vera := false
	for _, r := range righe {
		vera = vera || parole(r) >= 2
	}
	if !vera {
		return t
	}
	var tenute []string
	for _, r := range righe {
		if parole(r) > 0 {
			tenute = append(tenute, r)
		}
	}
	return strings.Join(tenute, "\n")
}
