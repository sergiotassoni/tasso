package traduce

import (
	"strings"
	"unicode"
)

// sottotitoloPronto: con la zona dei sottotitoli il testo si legge ogni mezzo secondo.
//   - Nei video lo sfondo si muove e l'OCR sbaglia una lettera ogni tanto: se è lo stesso
//     sottotitolo di prima non lo ritraduco (la traduzione cambiava o lampeggiava sotto gli occhi).
//   - Un sottotitolo che sta comparendo (dissolvenza, parole una alla volta) si legge a metà:
//     traduco solo quando due letture di fila dicono la stessa cosa.
//
// Restituisce se tradurre adesso e la lettura da tenere come candidata per il giro dopo.
func sottotitoloPronto(letto, ultimo, candidato string) (bool, string) {
	if ultimo != "" && (letto == ultimo || quasiUguale(letto, ultimo)) {
		return false, candidato
	}
	if letto != candidato && !quasiUguale(letto, candidato) {
		return false, letto
	}
	return true, letto
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
