package traduce

// Letture doppie della stessa riga. L'OCR (con le riletture delle righe piccole o incerte) a
// volte restituisce la stessa riga due volte, in pezzi che si sovrappongono: "Have you ever seen
// anything like this?" e "er seen anything like this? Looks like a talisman.", oppure "Jump
// Atta" dentro "Jump Attack". Tradotti tutti e due finivano uno sopra l'altro (e restavano in
// inglese tutti e due come "traduzioni sovrapposte"), o peggio uno tradotto e l'altro no: mezza
// riga italiana sopra l'inglese (Directive 8020, "If you leav— e le tue informazioni…").
//
// unisciLettureDoppie le riporta a UNA riga: i pezzi che si accavallano e dicono le stesse parole
// nella parte comune diventano la riga intera; il pezzo che sta tutto dentro un'altra lettura
// della stessa riga sparisce. Poi la riga intera si giudica una volta sola: tradotta tutta o
// lasciata in inglese tutta.

import (
	"strings"
	"unicode"
)

// unisciLettureDoppie: vedi sopra. Le scritte che non si accavallano restano come sono.
func unisciLettureDoppie(b []Blocco) []Blocco {
	out := append([]Blocco(nil), b...)
	for cambiato := true; cambiato; {
		cambiato = false
		for i := 0; i < len(out) && !cambiato; i++ {
			for j := 0; j < len(out) && !cambiato; j++ {
				if i == j {
					continue
				}
				a, c := out[i], out[j]
				if c.X < a.X || (c.X == a.X && j < i) || !lineaSimile(a, c) {
					continue
				}
				comune := min(a.X+a.W, c.X+c.W) - c.X
				if comune < max(4, min(a.H, c.H)/2) {
					continue // non si accavallano (i pezzi vicini li unisce unisciStessaRiga)
				}
				if n, ok := letturaDoppia(a, c); ok {
					out[i] = n
					out = append(out[:j], out[j+1:]...)
					cambiato = true
				}
			}
		}
	}
	return out
}

// letturaDoppia: a (a sinistra) e c si accavallano sulla stessa linea. Se sono due letture della
// stessa riga restituisce la riga sola che le sostituisce.
func letturaDoppia(a, c Blocco) (Blocco, bool) {
	wa, wc := strings.Fields(a.Testo), strings.Fields(c.Testo)
	if len(wa) == 0 || len(wc) == 0 {
		return Blocco{}, false
	}
	tol := max(3, min(a.H, c.H)/3)
	dentro := c.X+c.W <= a.X+a.W+tol // c sta tutta sotto a
	if dentro {
		if pezzoDi(wc, wa) {
			return a, true // "Jump Atta" dentro "Jump Attack": resta la lettura intera
		}
		if abs(a.X-c.X) <= tol && abs(a.W-c.W) <= 2*tol {
			// stesso riquadro, letture diverse ("Gamer's Litüe Playgrmjnd" / "Garner's Little
			// Playground"): resta la migliore
			return letturaMigliore(a, c), true
		}
		return Blocco{}, false
	}
	if a.X+tol >= c.X && pezzoDi(wa, wc) {
		return c, true // a sta tutta dentro c (stesso inizio)
	}
	// c continua a destra: la fine di a e l'inizio di c dicono le stesse parole?
	for s := 0; s < len(wa); s++ {
		n := len(wa) - s // parole in comune
		if n > len(wc)-1 {
			continue // c deve continuare oltre a
		}
		scelte, forti, parziali, ok := allinea(wa[s:], wc[:n])
		unaLunga := n == 1 && forti == 1 && len([]rune(normaleParola(wa[s]))) >= 4 // "This revolt" + "revolt brings…"
		if !ok || forti < 1 || (forti+parziali < 2 && !unaLunga) || forti*3 < (n-parziali)*2 {
			continue
		}
		testo := append(append(append([]string(nil), wa[:s]...), scelte...), wc[n:]...)
		m := unisciDue(a, c)
		m.Testo = strings.Join(testo, " ")
		m.Grezzo = m.Testo
		pa, pc := a.parole(), c.parole()
		if len(pa) == len(wa) && len(pc) == len(wc) {
			pos := append([]ParolaOCR(nil), pa[:s]...)
			for k, w := range pc {
				if k < n {
					w.T = scelte[k]
				}
				pos = append(pos, w)
			}
			m.Pos = &pos
		} else {
			m.Pos = nil
		}
		m.Affid = 0
		return m, true
	}
	return Blocco{}, false
}

// allinea: le parole x (fine del pezzo a sinistra) e y (inizio del pezzo a destra) sono le stesse
// lette due volte. La prima di y può essere la fine di una parola tagliata ("'e" di "leave"),
// l'ultima di x l'inizio di una parola tagliata ("Jerem" di "Jeremy's"). Restituisce le parole
// scelte (quelle vere, o le più lunghe), quante coincidono davvero (forti), quante sono pezzi
// tagliati (parziali) e se tutto torna.
func allinea(x, y []string) ([]string, int, int, bool) {
	scelte := make([]string, len(x))
	forti, parziali, sbagliate := 0, 0, 0
	for k := range x {
		nx, ny := normaleParola(x[k]), normaleParola(y[k])
		switch {
		case nx == ny && nx != "":
			forti++
			scelte[k] = migliore(x[k], y[k])
		case k == 0 && (ny == "" || strings.HasSuffix(nx, ny)):
			scelte[k] = x[k] // pezzetto di parola all'inizio della rilettura
			parziali++
		case k == len(x)-1 && nx != "" && strings.HasPrefix(ny, nx) && len(ny) > len(nx):
			scelte[k] = y[k] // parola tagliata alla fine della prima lettura
			parziali++
		case simili(nx, ny):
			forti++
			scelte[k] = migliore(x[k], y[k])
		default:
			sbagliate++
			scelte[k] = migliore(x[k], y[k])
		}
	}
	return scelte, forti, parziali, sbagliate*4 <= len(x) && sbagliate <= 1
}

// pezzoDi: le parole p sono un pezzo consecutivo di t (letto due volte), con la prima e
// l'ultima che possono essere tagliate.
func pezzoDi(p, t []string) bool {
	if len(p) > len(t) {
		return false
	}
	for s := 0; s+len(p) <= len(t); s++ {
		forti, sbagliate := 0, 0
		for k := range p {
			np, nt := normaleParola(p[k]), normaleParola(t[s+k])
			switch {
			case np == nt && np != "", simili(np, nt):
				forti++
			case k == 0 && (np == "" || strings.HasSuffix(nt, np)):
			case k == len(p)-1 && np != "" && strings.HasPrefix(nt, np):
			default:
				sbagliate++
			}
		}
		if sbagliate == 0 && (forti >= 1 && len(p) <= 2 || forti*3 >= len(p)*2) {
			return true
		}
	}
	return false
}

// normaleParola: la parola senza segni, minuscola, con le lettere che l'OCR scambia ridotte a una.
func normaleParola(w string) string { return lettereConfuse(w) }

// simili: due letture della stessa parola con una lettera diversa (parole di almeno 3 lettere:
// "{he" / "the", "ill" / "will"). Si usa solo dentro due pezzi già accavallati sulla stessa linea.
func simili(a, b string) bool {
	if a == "" || b == "" || max(len([]rune(a)), len([]rune(b))) < 3 {
		return false
	}
	return distanzaModifica(a, b) <= 1
}

// migliore: tra due letture della stessa parola quella vera (nel vocabolario); se lo sono tutte e
// due o nessuna, la più lunga ("will" invece di "ill").
func migliore(x, y string) string {
	px, py := strings.TrimFunc(x, nonLettera), strings.TrimFunc(y, nonLettera)
	ex, ey := px != "" && esiste(px), py != "" && esiste(py)
	switch {
	case ex && !ey:
		return x
	case ey && !ex:
		return y
	case len([]rune(py)) > len([]rune(px)):
		return y
	}
	return x
}

func nonLettera(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }

// letturaMigliore: tra due letture dello stesso riquadro quella con più parole vere (a parità,
// la prima).
func letturaMigliore(a, c Blocco) Blocco {
	punti := func(t string) int {
		_, f, r := valutaParole(t)
		if _, aff := correggiRigaOCR(t); aff == letturaCatalog {
			return 1000
		}
		return -2*f - r
	}
	if punti(c.Testo) > punti(a.Testo) {
		return c
	}
	return a
}
