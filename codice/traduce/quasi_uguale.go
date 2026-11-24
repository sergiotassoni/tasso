package traduce

// Soglia di somiglianza (idea presa da LunaTranslator): se l'OCR legge "Hel1o there, traveller"
// invece di "Hello there, traveller", la frase è la stessa di prima. Invece di ritradurla (e magari
// avere una traduzione diversa) uso quella già nella libreria del gioco. Solo per frasi lunghe,
// solo errori piccoli dentro le parole, mai se cambiano i numeri o una negazione.

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

var negazioni = map[string]bool{
	"not": true, "no": true, "never": true, "nor": true, "can't": true, "cannot": true, "won't": true,
	"don't": true, "doesn't": true, "didn't": true, "isn't": true, "aren't": true, "wasn't": true,
	"weren't": true, "shouldn't": true, "wouldn't": true, "couldn't": true, "haven't": true, "hasn't": true,
}

// cifre: i numeri veri della frase ("150", "3/10", "x2"); una cifra dentro una parola ("Hel1o") è
// un errore dell'OCR e non conta.
var pronomi = map[string]bool{
	"he": true, "she": true, "him": true, "her": true, "his": true, "hers": true, "it": true, "its": true,
	"they": true, "them": true, "their": true, "this": true, "these": true, "that": true, "those": true,
	"i": true, "me": true, "my": true, "we": true, "us": true, "our": true, "you": true, "your": true,
}

func cifre(s string) string {
	var out []string
	for _, w := range strings.Fields(s) {
		d, l := 0, 0
		for _, r := range w {
			if unicode.IsDigit(r) {
				d++
			} else if unicode.IsLetter(r) {
				l++
			}
		}
		if d > 0 && d >= l {
			out = append(out, w)
		}
	}
	return strings.Join(out, " ")
}

// distanzaTesti: distanza di Levenshtein tra a e b, fermandosi appena supera max (restituisce max+1).
func distanzaTesti(a, b []rune, max int) int {
	if d := len(a) - len(b); d > max || -d > max {
		return max + 1
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		minRiga := cur[0]
		for j := 1; j <= len(b); j++ {
			c := 1
			if a[i-1] == b[j-1] {
				c = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+c)
			minRiga = min(minRiga, cur[j])
		}
		if minRiga > max {
			return max + 1
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

func tolleranza(n int) int {
	switch {
	case n < 15:
		return 0
	case n < 30:
		return 1
	case n < 60:
		return 2
	}
	return 3
}

// quasiUguale: vero se b è la stessa frase di a letta con piccoli errori dell'OCR.
func quasiUguale(a, b string) bool {
	ra, rb := []rune(a), []rune(b)
	tol := tolleranza(min(len(ra), len(rb)))
	if tol == 0 || a == b || cifre(a) != cifre(b) {
		return false
	}
	wa, wb := strings.Fields(strings.ToLower(a)), strings.Fields(strings.ToLower(b))
	if len(wa) != len(wb) {
		return false
	}
	for i := range wa {
		if wa[i] == wb[i] {
			continue
		}
		pa, pb := strings.Trim(wa[i], ".,!?;:\"'"), strings.Trim(wb[i], ".,!?;:\"'")
		if pa == pb {
			// cambia solo la punteggiatura ("village." / "village,"): prima il controllo dei prefissi
			// la scartava sempre. Ma domanda e risposta non sono la stessa frase.
			if strings.Contains(wa[i], "?") != strings.Contains(wb[i], "?") {
				return false
			}
			continue
		}
		if negazioni[pa] || negazioni[pb] || pronomi[pa] || pronomi[pb] || distanzaTesti([]rune(pa), []rune(pb), 2) > 2 {
			return false
		}
		// "E" / "F", "on"/"off": parole cortissime (tasti, numeri) cambiano il senso
		if len([]rune(pa)) <= 2 || len([]rune(pb)) <= 2 {
			return false
		}
		// "locked"/"unlocked", "visible"/"invisible": un prefisso che nega non è un errore di lettura
		for _, pre := range []string{"un", "in", "im", "dis", "non", "ir", "il"} {
			if strings.TrimPrefix(pa, pre) == pb || strings.TrimPrefix(pb, pre) == pa {
				return false
			}
		}
	}
	return distanzaTesti(ra, rb, tol) <= tol
}

// chiaveLung: indice della libreria per il "quasi uguale": chi parla e lunghezza della frase (in
// byte). Così una frase nuova si confronta solo con quelle di lunghezza simile, non con tutte.
type chiaveLung struct {
	parlante string
	n        int
}

// indicizza mette una chiave della libreria ("parlante\x00frase") nell'indice. Le frasi corte non
// servono: per loro il "quasi uguale" non vale mai.
func indicizza(indice map[chiaveLung][]string, chiave string) {
	p, testo, ok := strings.Cut(chiave, "\x00")
	if !ok || tolleranza(utf8.RuneCountInString(testo)) == 0 {
		return
	}
	k := chiaveLung{p, len(testo)}
	indice[k] = append(indice[k], chiave)
}

type candidatoQuasi struct{ testo, tradotto string }

// candidatiQuasiUgualiLocked: le frasi della libreria di lunghezza simile (stesso parlante).
// Va chiamata con t.mu preso; il confronto vero (lento) si fa dopo, senza lucchetto.
func (t *Traduttore) candidatiQuasiUgualiLocked(parlante, battuta string) []candidatoQuasi {
	if t.libreria == nil || len(t.perLunghezza) == 0 || tolleranza(utf8.RuneCountInString(battuta)) == 0 {
		return nil
	}
	var out []candidatoQuasi
	n := len(battuta)
	for d := -6; d <= 6; d++ {
		for _, k := range t.perLunghezza[chiaveLung{parlante, n + d}] {
			out = append(out, candidatoQuasi{k[len(parlante)+1:], t.libreria[k]})
		}
	}
	return out
}

// sceltaQuasiUguale: tra i candidati, la frase quasi uguale più vicina (meno lettere diverse; a
// parità, la prima in ordine alfabetico): sempre la stessa scelta, qualunque sia l'ordine.
func sceltaQuasiUguale(battuta string, cand []candidatoQuasi) (string, bool) {
	rb := []rune(battuta)
	meglio, dMeglio, trovata := candidatoQuasi{}, 0, false
	for _, c := range cand {
		if !quasiUguale(c.testo, battuta) {
			continue
		}
		d := distanzaTesti([]rune(c.testo), rb, 3)
		if !trovata || d < dMeglio || (d == dMeglio && c.testo < meglio.testo) {
			meglio, dMeglio, trovata = c, d, true
		}
	}
	return meglio.tradotto, trovata
}
