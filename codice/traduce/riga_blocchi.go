package traduce

// Righe intere come unità, e riquadro che copre TUTTA la scritta originale (tutti i giochi).
//
// - unisciStessaRiga: l'OCR a volte divide una riga in due ("JOIN" e "FRIEND", "FOCUS > Move to
//   fill" e "the progress bar"). Due pezzi sulla stessa linea, della stessa grandezza e vicini
//   come due parole tornano una riga sola: si traduce tutta insieme, mai metà sì e metà no.
// - spezzaSeparatori: "FOCUS > Move to fill the progress bar" sono due scritte (etichetta e
//   frase): le divido al segno ">" o "|" e ognuna si traduce intera per conto suo.
// - adattaAInchiostro / scrittaContinua: il rettangolo dell'OCR è spesso più stretto delle
//   lettere vere (effetti di colore, ombre, lettere tagliate). Lo allargo fin dove arriva
//   l'inchiostro attaccato alla scritta. Se accanto c'è un'altra parola non letta ("FRIEND"
//   dopo "JOIN"), la riga è incompleta: non la traduco (resterebbe l'inglese a metà).
// - calcolaToppa: il riquadro che copre l'inglese, un po' più grande della scritta e mai sulla riga sotto.

import (
	"image"
	"image/color"
	"math"
	"sort"
	"strings"
	"unicode"
)

// stessaLinea: due scritte di una riga sola sulla stessa linea e della stessa grandezza.
func stessaLinea(a, b Blocco) bool {
	if max(1, a.Righe) != 1 || max(1, b.Righe) != 1 || a.H <= 0 || b.H <= 0 {
		return false
	}
	hMin, hMax := min(a.H, b.H), max(a.H, b.H)
	if float64(hMax) > 1.45*float64(hMin) {
		return false
	}
	sopra := min(a.Y+a.H, b.Y+b.H) - max(a.Y, b.Y)
	return sopra*10 >= hMin*6
}

// sogliaParole: lo spazio massimo tra due parole della stessa riga (oltre è un'altra colonna).
func sogliaParole(a, b Blocco) int {
	h := float64(min(a.H, b.H))
	em := math.Max(a.Em, b.Em)
	if em <= 0 {
		em = h * 1.2
	}
	return int(math.Max(1.1*h, 0.9*em) + 0.5)
}

// unisciStessaRiga: pezzi della stessa riga divisi dall'OCR tornano una scritta sola.
func unisciStessaRiga(b []Blocco) []Blocco {
	out := append([]Blocco(nil), b...)
	for cambiato := true; cambiato; {
		cambiato = false
		sort.SliceStable(out, func(i, j int) bool {
			if abs((out[i].Y+out[i].H/2)-(out[j].Y+out[j].H/2)) > min(out[i].H, out[j].H)/2 {
				return out[i].Y < out[j].Y
			}
			return out[i].X < out[j].X
		})
		for i := 0; i < len(out) && !cambiato; i++ {
			for j := 0; j < len(out); j++ {
				if i == j {
					continue
				}
				a, c := out[i], out[j]
				if c.X < a.X {
					continue
				}
				buco := c.X - (a.X + a.W)
				if !stessaLinea(a, c) || buco < -min(a.H, c.H)/2 || buco > sogliaParole(a, c) {
					// pezzi di UNA frase (il primo vuole il seguito o il secondo comincia
					// minuscolo): uniti anche un po' più lontani o sovrapposti (l'OCR ha perso
					// o letto due volte una parola in mezzo), e la riga intera si giudica insieme
					if !pezziDiUnaFrase(a, c) {
						continue
					}
					c = senzaLetteraDoppia(a, c)
				}
				out[i] = unisciDue(a, c)
				out = append(out[:j], out[j+1:]...)
				cambiato = true
				break
			}
		}
	}
	return out
}

// paroleSeguito: parole che a fine pezzo vogliono quasi sempre il seguito sulla stessa riga
// (oltre a paroleSospese): comparativi, ausiliari, congiunzioni. "less" e "more" chiudono anche
// pulsanti corti ("Show more"): contano solo nei pezzi di almeno tre parole.
var paroleSeguito = map[string]bool{"less": true, "more": true, "than": true, "very": true, "so": true,
	"too": true, "as": true, "not": true, "can": true, "will": true, "would": true, "could": true,
	"should": true, "must": true, "if": true, "when": true, "while": true, "because": true, "these": true,
	"those": true, "each": true, "every": true, "some": true, "has": true, "have": true, "had": true}

// chiedeSeguito: il pezzo finisce con una parola che vuole il seguito ("...makes less", "...of the").
func chiedeSeguito(t string) bool {
	w := strings.Fields(strings.TrimSpace(t))
	if len(w) == 0 {
		return false
	}
	if len(w) == 1 && verbiTasto[strings.ToLower(w[0])] {
		return true // "Press" | [icona del tasto] | "to reveal the HUD": il comando continua dopo l'icona
	}
	u := w[len(w)-1]
	if strings.HasSuffix(u, ",") {
		return len(w) >= 2
	}
	u = strings.ToLower(strings.Trim(u, "\"'"))
	if paroleSospese[u] {
		return true
	}
	return paroleSeguito[u] && len(w) >= 3
}

// iniziaMinuscola: il pezzo comincia con una parola di sole lettere minuscola ("s noise…",
// "noise when…"): è il seguito di una frase, non una scritta a sé ("x2", "4K" non contano).
func iniziaMinuscola(t string) bool {
	w := strings.Fields(strings.TrimSpace(t))
	if len(w) > 1 && segnoIcona(w[0]) {
		w = w[1:] // "• to reveal…": l'icona di un tasto letta come un segno, poi la frase continua
	}
	if len(w) == 0 {
		return false
	}
	p := strings.TrimRight(w[0], ".,;:!?")
	r := []rune(p)
	if len(r) == 0 || !unicode.IsLower(r[0]) {
		return false
	}
	for _, c := range r {
		if !unicode.IsLetter(c) && c != '\'' && c != '-' {
			return false
		}
	}
	return true
}

// pezziDiUnaFrase: a (a sinistra) e c sulla stessa linea sono due pezzi della stessa frase: a
// vuole il seguito o c comincia minuscolo, e tra i due c'è al massimo lo spazio di una parola
// persa (3 em) o si sovrappongono di poco.
func pezziDiUnaFrase(a, c Blocco) bool {
	if c.X < a.X || !lineaSimile(a, c) {
		return false
	}
	if !chiedeSeguito(a.Testo) && !iniziaMinuscola(c.Testo) && !pezzoTroncato(a.Testo, c.Testo) {
		return false
	}
	em := math.Max(a.Em, c.Em)
	if em <= 0 {
		em = float64(min(a.H, c.H)) * 1.1
	}
	buco := c.X - (a.X + a.W)
	return buco >= -max(a.H, c.H) && float64(buco) <= 3*em
}

// pezzoTroncato: l'OCR ha spezzato una parola tra i due pezzi ("...enemies ar" | "Ire nearby."):
// a finisce con un pezzo di parola che non esiste o c comincia con un pezzetto con la maiuscola
// che non è una parola comune, e tra i due non c'è la fine di una frase (. ! ? :).
func pezzoTroncato(a, c string) bool {
	w := strings.Fields(strings.TrimSpace(a))
	if len(w) == 0 || strings.TrimSpace(c) == "" {
		return false
	}
	u := strings.TrimRight(w[len(w)-1], "\"'»”’)")
	if fineFrase(u) || strings.HasSuffix(u, ":") || strings.HasSuffix(u, ";") {
		return false
	}
	return finisceConFrammento(a) || iniziaConFrammento(c)
}

// finisceConFrammento: l'ultima parola del pezzo è di sole lettere minuscole e non esiste ("ar").
func finisceConFrammento(t string) bool {
	w := strings.Fields(strings.TrimSpace(t))
	if len(w) == 0 {
		return false
	}
	u := []rune(strings.Trim(w[len(w)-1], "\"'"))
	if len(u) == 0 {
		return false
	}
	for _, c := range u {
		if !unicode.IsLetter(c) || !unicode.IsLower(c) {
			return false
		}
	}
	return !esiste(string(u))
}

// paroleCorteNote: parole corte con la maiuscola che stanno bene all'inizio di una scritta anche
// se nel vocabolario sono rare.
var paroleCorteNote = map[string]bool{"ok": true, "yes": true, "no": true, "new": true, "off": true, "on": true}

// iniziaConFrammento: il pezzo comincia con un pezzetto di 2-3 lettere con la maiuscola e il resto
// minuscolo ("Ire") che non è una parola comune né una sigla: la fine di una parola spezzata.
func iniziaConFrammento(t string) bool {
	w := strings.Fields(strings.TrimSpace(t))
	if len(w) == 0 {
		return false
	}
	r := []rune(strings.TrimRight(w[0], ".,;:!?"))
	if len(r) < 2 || len(r) > 3 || !unicode.IsUpper(r[0]) {
		return false
	}
	for _, c := range r[1:] {
		if !unicode.IsLetter(c) || !unicode.IsLower(c) {
			return false // "HP", "XP", "4K": sigle, non pezzi di parola
		}
	}
	l := strings.ToLower(string(r))
	if paroleCorteNote[l] {
		return false
	}
	if n, ok := vocabolario()[l]; ok && n <= 20 {
		return false // "The", "Are", "You": parole comuni
	}
	return true
}

// motivoFratello: la scritta b è un pezzo di una frase letta in più pezzi sulla stessa linea
// (fratelli: stessa altezza, stessa linea, vicini o sovrapposti) e un fratello resta in inglese
// perché letto male, o b comincia con un pezzetto di parola spezzata ("Ire nearby.") con un
// fratello a sinistra: mai tradurre un fratello da solo (si vedrebbe mezza frase sopra
// l'inglese). Le voci vere di una fila di pulsanti (buco largo, maiuscola) restano separate.
func motivoFratello(b Blocco, tutte []Blocco) string {
	for _, s := range tutte {
		if s.X == b.X && s.Y == b.Y && s.W == b.W && s.H == b.H {
			continue // b stessa
		}
		sx, dx := s, b
		if s.X > b.X {
			sx, dx = b, s
		}
		if sx.X+sx.W >= dx.X+dx.W {
			// uno contiene l'altro: la riga intera da cui vengono le voci va bene, ma se b sta
			// dentro un'altra lettura della stessa riga rimasta in inglese perché letta male, b è
			// un pezzo di quella riga: tradotto da solo sarebbe mezza riga italiana sopra
			// l'inglese ("'e your information, I will make sure to" dentro "No, I lc nit. Je
			// has gone missing… I ill make", Directive 8020)
			if s.X <= b.X && s.X+s.W >= b.X+b.W && !(s.X == b.X && s.W == b.W) && lineaSimile(s, b) &&
				motivoNonTradurre(s) == "lettura incerta" {
				return "pezzo di una riga rimasta in inglese"
			}
			continue
		}
		// accavallati: due letture della stessa riga che si sovrappongono (anche per molto più
		// delle due altezze che fratelli accetta): pezzi della stessa riga
		accavallati := dx.X < sx.X+sx.W && lineaSimile(sx, dx)
		if !fratelli(sx, dx) && !accavallati {
			continue // lontani
		}
		if dx.X == b.X && iniziaConFrammento(b.Testo) {
			return "pezzo di una parola spezzata"
		}
		if !accavallati && vociSeparate(sx, dx) {
			continue
		}
		if motivoNonTradurre(s) == "lettura incerta" {
			return "pezzo di una frase rimasta in inglese"
		}
	}
	return ""
}

// fratelli: sx (a sinistra) e dx sono sulla stessa linea con la stessa altezza (±25%), i centri
// entro 0,4 em, e vicini (al massimo 3 em di buco) o sovrapposti.
func fratelli(sx, dx Blocco) bool {
	if max(1, sx.Righe) != 1 || max(1, dx.Righe) != 1 || sx.H <= 0 || dx.H <= 0 {
		return false
	}
	hMin, hMax := min(sx.H, dx.H), max(sx.H, dx.H)
	if float64(hMax) > 1.25*float64(hMin) {
		return false
	}
	em := math.Max(sx.Em, dx.Em)
	if em <= 0 {
		em = float64(hMax) * 1.1
	}
	if math.Abs(float64((sx.Y+sx.H/2)-(dx.Y+dx.H/2))) > 0.4*em {
		return false
	}
	buco := dx.X - (sx.X + sx.W)
	return buco >= -2*hMax && float64(buco) <= 3*em
}

// vociSeparate: due voci di una fila di pulsanti, non pezzi di una frase: c'è un buco vero tra le
// due (almeno mezzo em), la seconda comincia con la maiuscola o un numero, e niente fa pensare a
// una frase spezzata (parola che vuole il seguito, minuscola, pezzo di parola).
func vociSeparate(sx, dx Blocco) bool {
	em := math.Max(sx.Em, dx.Em)
	if em <= 0 {
		em = float64(max(sx.H, dx.H)) * 1.1
	}
	if float64(dx.X-(sx.X+sx.W)) < 0.5*em {
		return false
	}
	w := strings.Fields(dx.Testo)
	if len(w) == 0 || !inizioVoce(w[0]) {
		return false
	}
	return !chiedeSeguito(sx.Testo) && !iniziaMinuscola(dx.Testo) && !pezzoTroncato(sx.Testo, dx.Testo)
}

// lineaSimile: come stessaLinea ma con l'altezza più libera (un pezzo riletto ingrandito o con
// lettere solo basse, "s noise", può avere il riquadro un po' più basso o più alto).
func lineaSimile(a, b Blocco) bool {
	if max(1, a.Righe) != 1 || max(1, b.Righe) != 1 || a.H <= 0 || b.H <= 0 {
		return false
	}
	hMin, hMax := min(a.H, b.H), max(a.H, b.H)
	if hMax > 2*hMin {
		return false
	}
	if abs((a.Y+a.H/2)-(b.Y+b.H/2))*2 > hMin {
		return false
	}
	sopra := min(a.Y+a.H, b.Y+b.H) - max(a.Y, b.Y)
	return sopra*10 >= hMin*6
}

// senzaLetteraDoppia: "less" + "s noise": la lettera sola minuscola all'inizio di c è la fine
// dell'ultima parola di a letta due volte (l'OCR ha ripreso la riga da metà parola): la tolgo.
func senzaLetteraDoppia(a, c Blocco) Blocco {
	pa, pc := a.parole(), c.parole()
	wc := strings.Fields(c.Testo)
	wa := strings.Fields(a.Testo)
	if len(pc) < 2 || len(wc) < 2 || len(wa) == 0 {
		return c
	}
	l := strings.TrimSpace(pc[0].T)
	ultima := strings.ToLower(strings.Trim(wa[len(wa)-1], ".,;:!?\"'"))
	if len([]rune(l)) != 1 || l == "a" || !unicode.IsLower([]rune(l)[0]) || !strings.HasSuffix(ultima, l) || !esiste(ultima) {
		return c
	}
	if len(pa) > 0 {
		u := pa[len(pa)-1]
		if pc[0].X > u.X+u.W+0.6*u.H { // lontana dalla parola: non è la stessa lettera
			return c
		}
	}
	n := bloccoDaPosizioni(pc[1:], c)
	n.Grezzo = n.Testo
	return n
}

// unisciEtichette: un'etichetta letta a parte ("TIP:", "Objective:", "MARCUS:") e la frase dopo
// sulla stessa linea, un po' più lontane di due parole (fino a 2,5 em), tornano una scritta sola
// se tra le due non c'è inchiostro (nessuna parola persa in mezzo): si traduce la frase intera.
// Senza immagine non unisco niente (non posso sapere se in mezzo manca una parola).
func unisciEtichette(img *image.RGBA, b []Blocco) []Blocco {
	if img == nil {
		return b
	}
	out := append([]Blocco(nil), b...)
	for cambiato := true; cambiato; {
		cambiato = false
		for i := 0; i < len(out) && !cambiato; i++ {
			a := out[i]
			t := strings.TrimSpace(a.Testo)
			if !strings.HasSuffix(t, ":") || len(strings.Fields(t)) > 2 || !inizioVoce(t) {
				continue
			}
			for j := range out {
				c := out[j]
				if j == i || c.X <= a.X || !stessaLinea(a, c) || !inizioVoce(c.Testo) {
					continue
				}
				em := math.Max(a.Em, c.Em)
				if em <= 0 {
					em = float64(min(a.H, c.H)) * 1.1
				}
				buco := c.X - (a.X + a.W)
				if buco <= sogliaParole(a, c) || float64(buco) > 2.5*em || !bucoVuoto(img, a, c) {
					continue
				}
				out[i] = unisciDue(a, c)
				out = append(out[:j], out[j+1:]...)
				cambiato = true
				break
			}
		}
	}
	return out
}

// bucoVuoto: tra la fine di a e l'inizio di c (stessa linea) nessuna colonna ha inchiostro di lettere.
func bucoVuoto(img *image.RGBA, a, c Blocco) bool {
	coloriBloccoSemplice(img, &a)
	a.HaContorno = false
	if !inchiostroCredibile(img, &a) {
		return false
	}
	em := a.Em
	if em <= 0 {
		em = float64(a.H) * 1.1
	}
	bordo := int(em*0.12 + 0.5) // le code delle lettere già lette
	x0, x1 := a.X+a.W+bordo, c.X-bordo
	if x1 <= x0 {
		return true
	}
	minCol := max(2, a.H*15/100)
	for _, n := range colonneInchiostro(img, &a, x0, x1) {
		if n >= minCol {
			return false
		}
	}
	return true
}

func unisciDue(a, c Blocco) Blocco {
	x0, y0 := min(a.X, c.X), min(a.Y, c.Y)
	x1, y1 := max(a.X+a.W, c.X+c.W), max(a.Y+a.H, c.Y+c.H)
	n := a
	n.Testo = strings.TrimSpace(a.Testo + " " + c.Testo)
	if a.Grezzo != "" || c.Grezzo != "" {
		n.Grezzo = strings.TrimSpace(a.Grezzo + " " + c.Grezzo)
	}
	n.X, n.Y, n.W, n.H = x0, y0, x1-x0, y1-y0
	unite := append(append([]ParolaOCR(nil), a.parole()...), c.parole()...)
	n.Pos = &unite
	if a.Em > 0 && c.Em > 0 {
		n.Em = (a.Em*float64(a.W) + c.Em*float64(c.W)) / float64(a.W+c.W)
	} else {
		n.Em = math.Max(a.Em, c.Em)
	}
	return n
}

// separatori: segni che dividono un'etichetta dalla sua frase ("FOCUS > Move…", "Map | Inventory").
var separatoriRiga = map[string]bool{">": true, ">>": true, "»": true, "›": true, "|": true, "•": true, "·": true, "→": true}

// spezzaSeparatori: divide le scritte al segno separatore. Il segno resta del gioco (non coperto).
func spezzaSeparatori(b []Blocco) []Blocco {
	var out []Blocco
	for _, bl := range b {
		if max(1, bl.Righe) != 1 || len(bl.parole()) < 3 {
			out = append(out, bl)
			continue
		}
		var gruppi [][]ParolaOCR
		cur := []ParolaOCR{}
		for _, w := range bl.parole() {
			t := strings.TrimSpace(w.T)
			if separatoriRiga[t] {
				if len(cur) > 0 {
					gruppi = append(gruppi, cur)
				}
				cur = []ParolaOCR{}
				continue
			}
			// segno attaccato alla parola: ">Move" → "Move" (il segno resta scoperto)
			for sep := range separatoriRiga {
				if strings.HasPrefix(t, sep) && len(t) > len(sep) && len(cur) > 0 {
					gruppi = append(gruppi, cur)
					cur = []ParolaOCR{}
					quota := float64(len(sep)) / float64(len(t))
					w.X += w.W * quota
					w.W -= w.W * quota
					w.T = strings.TrimPrefix(t, sep)
					break
				}
			}
			cur = append(cur, w)
		}
		if len(cur) > 0 {
			gruppi = append(gruppi, cur)
		}
		// "Press • to reveal the HUD": il segno è l'icona di un tasto dentro una frase (prima un
		// comando che vuole il seguito, dopo una parola minuscola): la frase resta intera
		if len(gruppi) < 2 {
			out = append(out, bl)
			continue
		}
		uniti := [][]ParolaOCR{gruppi[0]}
		for _, g := range gruppi[1:] {
			u := uniti[len(uniti)-1]
			if chiedeSeguito(testoParole(u)) || iniziaMinuscola(testoParole(g)) {
				uniti[len(uniti)-1] = append(append([]ParolaOCR(nil), u...), g...)
				continue
			}
			uniti = append(uniti, g)
		}
		if len(uniti) < 2 {
			out = append(out, bloccoDaPosizioni(uniti[0], bl)) // senza il segno dell'icona
			continue
		}
		for _, g := range uniti {
			out = append(out, bloccoDaPosizioni(g, bl))
		}
	}
	return out
}

// spezzaPulsanti: una fila di pulsanti, di schede o di comandi in fondo allo schermo letta come
// una riga sola ("Apply   Reset to Default   Back"): dove lo spazio tra due parole è molto più
// grande dello spazio normale della riga, e il pezzo dopo comincia con la maiuscola o un numero,
// sono voci diverse. Ognuna si traduce e si copre per conto suo (una frase tradotta tutta insieme
// mescolerebbe le voci, e con le voci che restano uguali verrebbe scartata come mezza inglese).
// Con un solo spazio (due parole) serve un buco più largo di un'altezza intera: i titoli con le
// lettere spaziate ("IRON   HARBOR") restano uniti.
func spezzaPulsanti(b []Blocco) []Blocco {
	var out []Blocco
	for _, bl := range b {
		p := append([]ParolaOCR(nil), bl.parole()...)
		if max(1, bl.Righe) != 1 || len(p) < 2 {
			out = append(out, bl)
			continue
		}
		sort.SliceStable(p, func(i, j int) bool { return p[i].X < p[j].X })
		hs := make([]float64, 0, len(p))
		for _, w := range p {
			hs = append(hs, w.H)
		}
		sort.Float64s(hs)
		hMed := hs[len(hs)/2]
		buchi := make([]float64, len(p)-1)
		for i := 1; i < len(p); i++ {
			buchi[i-1] = p[i].X - (p[i-1].X + p[i-1].W)
		}
		soglia := 1.2 * hMed // due parole sole: più di un'altezza intera
		sogliaFrase := soglia
		if len(buchi) >= 2 {
			ord := append([]float64(nil), buchi...)
			sort.Float64s(ord)
			normale := math.Max(ord[(len(ord)-1)/2], 0.15*hMed) // spazio normale tra due parole
			soglia = math.Max(2.5*normale, 0.6*hMed)
			sogliaFrase = soglia
			// buchi di due grandezze nettamente diverse (le parole di Windows sono strette sulle
			// lettere: "Apply" 23 px "Reset" 8 "to" 10 "Default" 23 "Back"): i buchi piccoli sono
			// gli spazi della riga, quelli grandi (almeno 1,8 volte il più grande dei piccoli e
			// più di metà altezza) dividono le voci, anche se con poche parole la mediana dei buchi
			// cade già su un buco grande.
			for k := 0; k+1 < len(ord); k++ {
				piccolo := math.Max(ord[k], 0.15*hMed)
				if ord[k+1] >= 1.8*piccolo && ord[k+1] >= 0.6*hMed {
					soglia = math.Min(soglia, ord[k+1])
					break
				}
			}
		}
		var gruppi [][]ParolaOCR
		cur := []ParolaOCR{p[0]}
		for i := 1; i < len(p); i++ {
			s := soglia
			if fineFrase(p[i-1].T) {
				s = sogliaFrase // dopo il punto lo spazio è spesso più largo: due frasi, non due voci
			}
			if buchi[i-1] >= s && inizioVoce(p[i].T) && fineVoce(p[i-1].T) && !chiedeSeguito(testoParole(cur)) {
				gruppi = append(gruppi, cur)
				cur = nil
			}
			cur = append(cur, p[i])
		}
		gruppi = append(gruppi, cur)
		if len(gruppi) < 2 {
			out = append(out, bl)
			continue
		}
		for _, g := range gruppi {
			n := bloccoDaPosizioni(g, bl)
			n.Grezzo = n.Testo
			out = append(out, n)
		}
	}
	return out
}

// inizioVoce: la parola può essere l'inizio di un pulsante o di una scheda (maiuscola o numero).
func inizioVoce(w string) bool {
	for _, c := range w {
		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			return unicode.IsUpper(c) || unicode.IsDigit(c)
		}
		if !strings.ContainsRune("[(<\"'«", c) {
			return false
		}
	}
	return false
}

// fineVoce: la parola può chiudere una voce (non "of", "the", "to" o una virgola: lì la frase continua).
func fineVoce(w string) bool {
	t := strings.TrimSpace(w)
	if strings.HasSuffix(t, ",") || strings.HasSuffix(t, "-") || strings.HasSuffix(t, ":") {
		return false // dopo un'etichetta ("TIP:") la frase continua
	}
	return !paroleSospese[strings.ToLower(strings.Trim(t, ".:;!?\"'"))]
}

// frecce dei selettori delle impostazioni ("< On >", "◀ English ▶", "‹ High ›").
var (
	frecceSinistra = "<‹◀◄«←⟨"
	frecceDestra   = ">›▶►»→⟩"
)

// togliFrecce: le frecce di un selettore ("< Off >", "◀ 1920 x 1080 ▶") non fanno parte della
// scritta: tolte, resta il valore da tradurre ("Off" -> "Disattivato"). Il riquadro è quello del
// valore: le frecce restano del gioco (se l'italiano è più lungo può coprirle, ma si legge tutto,
// invece di rimpicciolirsi fino ai puntini tra due frecce vicine).
func togliFrecce(b []Blocco) []Blocco {
	var out []Blocco
	for _, bl := range b {
		p := bl.parole()
		if max(1, bl.Righe) != 1 || len(p) == 0 {
			out = append(out, bl)
			continue
		}
		t := strings.TrimSpace(bl.Testo)
		r := []rune(t)
		if len(r) < 3 || !strings.ContainsRune(frecceSinistra, r[0]) || !strings.ContainsRune(frecceDestra, r[len(r)-1]) {
			out = append(out, bl)
			continue
		}
		parole := append([]ParolaOCR(nil), p...)
		tolte := 0
		// a sinistra: la parola "<" da sola, o il segno attaccato ("<No>")
		if w := []rune(strings.TrimSpace(parole[0].T)); len(w) > 0 && strings.ContainsRune(frecceSinistra, w[0]) {
			tolte++
			if len(w) == 1 {
				parole = parole[1:]
			} else {
				quota := 1 / float64(len(w))
				parole[0].X += parole[0].W * quota
				parole[0].W -= parole[0].W * quota
				parole[0].T = string(w[1:])
			}
		}
		if n := len(parole); n > 0 {
			if w := []rune(strings.TrimSpace(parole[n-1].T)); len(w) > 0 && strings.ContainsRune(frecceDestra, w[len(w)-1]) {
				tolte++
				if len(w) == 1 {
					parole = parole[:n-1]
				} else {
					quota := 1 / float64(len(w))
					parole[n-1].W -= parole[n-1].W * quota
					parole[n-1].T = string(w[:len(w)-1])
				}
			}
		}
		if len(parole) == 0 || tolte < 2 {
			out = append(out, bl)
			continue
		}
		out = append(out, bloccoDaPosizioni(parole, bl))
	}
	return out
}

// bloccoDaPosizioni: una scritta da parole già in pixel della cattura (copia grandezza e colori di base).
func bloccoDaPosizioni(p []ParolaOCR, base Blocco) Blocco {
	x0, y0, x1, y1 := 1e18, 1e18, -1e18, -1e18
	var parole []string
	for _, w := range p {
		x0, y0 = math.Min(x0, w.X), math.Min(y0, w.Y)
		x1, y1 = math.Max(x1, w.X+w.W), math.Max(y1, w.Y+w.H)
		parole = append(parole, w.T)
	}
	copia := append([]ParolaOCR(nil), p...)
	n := Blocco{Testo: strings.Join(parole, " "), Righe: 1, Em: base.Em, Pos: &copia,
		X: int(x0), Y: int(y0), W: int(x1 - x0 + 0.5), H: int(y1 - y0 + 0.5)}
	n.Grezzo = n.Testo
	if e := emParole(p); e > 0 {
		n.Em = e
	}
	return n
}

// eInchiostro: il pixel è del colore delle lettere (più vicino all'inchiostro che allo sfondo, e
// abbastanza vicino all'inchiostro). Con il contorno anche il colore del contorno conta.
func eInchiostro(c color.RGBA, b *Blocco) bool {
	di, ds := distanza(c, b.Inchiostro), distanza(c, b.Sfondo)
	if di < ds && di < 150 {
		return true
	}
	if b.HaContorno {
		dc := distanza(c, b.Contorno)
		return dc < ds && dc < 90
	}
	return false
}

// colonneInchiostro: per ogni colonna x della fascia [x0,x1) quante righe hanno inchiostro (solo
// nella parte centrale delle lettere, dove ci sono quasi tutte).
func colonneInchiostro(img *image.RGBA, b *Blocco, x0, x1 int) []int {
	bd := img.Bounds()
	ya, yb := b.Y+b.H*15/100, b.Y+b.H*85/100
	out := make([]int, max(0, x1-x0))
	for x := x0; x < x1; x++ {
		if x < bd.Min.X || x >= bd.Max.X {
			continue
		}
		for y := max(ya, bd.Min.Y); y < min(yb, bd.Max.Y); y++ {
			if eInchiostro(img.RGBAAt(x, y), b) {
				out[x-x0]++
			}
		}
	}
	return out
}

// scrittaContinua: accanto al riquadro (a sinistra o a destra) c'è un'altra parola che l'OCR non ha
// letto ("JOIN" letto, "FRIEND" no; "GAME" letto, "QUIT" no). Allora la riga è incompleta.
//
// Conta solo l'inchiostro che sembra lettere: non i bordi dei riquadri (una linea che continua
// sopra e sotto la scritta), non le zone piene dello stesso colore (cielo chiaro accanto a lettere
// bianche, il contorno scuro che somiglia allo sfondo): lì finisce la riga, non manca una parola.
func scrittaContinua(img *image.RGBA, b Blocco) bool {
	return scrittaContinuaTra(img, b, nil)
}

// scrittaContinuaTra: come scrittaContinua, ma l'inchiostro che sta dentro un'altra scritta già
// letta sulla stessa linea (altri) non è una parola "non letta": i pulsanti di una fila
// ("Apply", "Reset to Default", "Back"), "TIP:" letto a parte prima della frase. Per i lati si
// guarda la fascia della prima e dell'ultima parola (non di tutta la riga): una scritta un po'
// storta (cartello ruotato) ha le parole a altezze diverse, e la fascia di tutta la riga
// prenderebbe il bordo della targa sopra o sotto le lettere.
func scrittaContinuaTra(img *image.RGBA, b Blocco, altri []Blocco) bool {
	// colori semplici (sfondo del bordo, inchiostro più lontano): qui serve solo distinguere le
	// lettere dallo sfondo, anche quando il contorno o l'ombra confondono i colori del disegno
	coloriBloccoSemplice(img, &b)
	b.HaContorno = false
	if !inchiostroCredibile(img, &b) {
		return false // non so separare le lettere dallo sfondo: nessuna prova di una parola tagliata
	}
	bSx, bDx := fasceLati(b)
	return latoContinua(img, bDx, altri, b, 1) || latoContinua(img, bSx, altri, b, -1)
}

// fasceLati: il riquadro da usare per guardare a sinistra e a destra: la riga con l'altezza e la
// posizione verticale della prima (sinistra) e dell'ultima parola (destra), alte almeno quanto la
// mediana delle parole (le minuscole corte "us." non restringono la fascia).
func fasceLati(b Blocco) (sx, dx Blocco) {
	sx, dx = b, b
	p := b.parole()
	if len(p) < 2 || max(1, b.Righe) != 1 {
		return
	}
	hs := make([]float64, 0, len(p))
	for _, w := range p {
		hs = append(hs, w.H)
	}
	sort.Float64s(hs)
	hMed := hs[len(hs)/2]
	if hMed < 4 || hMed*1.15 >= float64(b.H) {
		return // parole alte come la riga: niente da guadagnare
	}
	primo, ultimo := p[0], p[0]
	for _, w := range p {
		if w.X < primo.X {
			primo = w
		}
		if w.X+w.W > ultimo.X+ultimo.W {
			ultimo = w
		}
	}
	fascia := func(w ParolaOCR, r *Blocco) {
		h := math.Max(w.H, hMed)
		cy := w.Y + w.H/2
		r.Y = int(cy - h/2 + 0.5)
		r.H = max(4, int(h+0.5))
	}
	fascia(primo, &sx)
	fascia(ultimo, &dx)
	return
}

// latoContinua: dal bordo della scritta verso destra (verso>0) o sinistra (<0), c'è una parola
// non letta? (b: la fascia del lato; tutta: la scritta intera, per riconoscerla tra gli altri)
func latoContinua(img *image.RGBA, b Blocco, altri []Blocco, tutta Blocco, verso int) bool {
	em := b.Em
	if em <= 0 {
		em = float64(b.H) * 1.1
	}
	largo := int(1.3*em + 0.5)
	minCol := max(2, b.H*15/100)   // una colonna "piena" ha inchiostro in almeno il 15% dell'altezza
	minRun := max(2, int(em*0.08)) // e una lettera è larga almeno un dodicesimo di em (anche i caratteri a pixel)
	banda := max(1, b.H*85/100-b.H*15/100)
	piena := max(minCol, banda*90/100)            // colonna piena da cima a fondo della fascia
	larghiPieni := max(minRun+1, int(em*0.6+0.5)) // nessuna lettera è piena per più di mezza em
	controlla := func(col []int, x0, verso int) bool {
		// verso>0: dalla scritta verso destra; <0: dalla scritta verso sinistra
		n := len(col)
		inizio := int(em*0.12 + 0.5) // i primi pixel possono essere coda delle lettere già lette
		piene, primo, tratti := 0, -1, 0
		for k := inizio; k < n; {
			i := k
			if verso < 0 {
				i = n - 1 - k
			}
			if col[i] < minCol {
				k++
				continue
			}
			// un tratto di colonne con inchiostro: lettere, o bordo/zona piena?
			run, intere, fuori := 0, 0, 0
			for ; k < n; k++ {
				i = k
				if verso < 0 {
					i = n - 1 - k
				}
				if col[i] < minCol {
					break
				}
				run++
				if col[i] >= piena {
					intere++
				}
				if colonnaFuoriRiga(img, &b, x0+i) {
					fuori++
				}
			}
			if fuori*2 >= run || intere >= larghiPieni {
				return false // bordo del riquadro o zona piena: la riga finisce qui
			}
			if primo < 0 {
				primo = k - run
			}
			piene += run
			// lettere: tratti fitti dalla prima colonna con inchiostro (lo spazio tra due parole
			// non conta)
			if run >= minRun && piene*100 >= (k-primo)*25 {
				return true
			}
			// caratteri sottili (tratti di un pixel): tre aste separate entro una em sono lettere
			if tratti++; tratti >= 3 && float64(k-primo) <= em {
				return true
			}
		}
		return false
	}
	x0 := b.X + b.W
	if verso < 0 {
		x0 = b.X - largo
	}
	col := colonneInchiostro(img, &b, x0, x0+largo)
	togliAltreScritte(col, x0, b, tutta, altri)
	return controlla(col, x0, verso)
}

// togliAltreScritte: le colonne che cadono dentro un'altra scritta letta sulla stessa linea non
// contano come inchiostro (è una scritta letta, non una parola persa).
func togliAltreScritte(col []int, x0 int, b, tutta Blocco, altri []Blocco) {
	for _, a := range altri {
		if a.X == tutta.X && a.Y == tutta.Y && a.W == tutta.W && a.H == tutta.H && a.Testo == tutta.Testo {
			continue
		}
		sopra := min(a.Y+a.H, b.Y+b.H) - max(a.Y, b.Y)
		if sopra*2 < min(a.H, b.H) {
			continue
		}
		for x := max(a.X-1, x0); x < min(a.X+a.W+1, x0+len(col)); x++ {
			col[x-x0] = 0
		}
	}
}

// inchiostroCredibile: dentro il riquadro le lettere coprono una parte della fascia centrale, non
// quasi tutto (se "inchiostro" è quasi ogni pixel, i colori sono sbagliati).
func inchiostroCredibile(img *image.RGBA, b *Blocco) bool {
	col := colonneInchiostro(img, b, b.X, b.X+b.W)
	banda := max(1, b.Y+b.H*85/100-(b.Y+b.H*15/100))
	tot := 0
	for _, c := range col {
		tot += c
	}
	return len(col) > 0 && tot*100 <= len(col)*banda*95
}

// colonnaFuoriRiga: nella colonna x l'inchiostro continua senza interruzioni sopra o sotto la
// riga (per un terzo della sua altezza): è il bordo di un riquadro o una zona piena, non una lettera.
func colonnaFuoriRiga(img *image.RGBA, b *Blocco, x int) bool {
	bd := img.Bounds()
	if x < bd.Min.X || x >= bd.Max.X {
		return false
	}
	ext := max(3, b.H/3)
	continua := func(y0, y1 int) bool {
		if y0 < bd.Min.Y || y1 > bd.Max.Y {
			return false
		}
		for y := y0; y < y1; y++ {
			if !eInchiostro(img.RGBAAt(x, y), b) {
				return false
			}
		}
		return true
	}
	return continua(b.Y-ext, b.Y) || continua(b.Y+b.H, b.Y+b.H+ext)
}

// adattaAInchiostro: allarga il riquadro fin dove arrivano le lettere attaccate (effetti di colore,
// ombre, la coda di una lettera tagliata dall'OCR). Al massimo un terzo di em per lato.
func adattaAInchiostro(img *image.RGBA, b *Blocco) {
	em := b.Em
	if em <= 0 {
		em = float64(b.H) * 1.1
	}
	lim := max(1, int(em/3+0.5))
	soglia := max(1, b.H*8/100)
	// a destra e a sinistra: colonne di inchiostro attaccate al bordo
	dx := colonneInchiostro(img, b, b.X+b.W, b.X+b.W+lim)
	k := 0
	for k < len(dx) && dx[k] >= soglia {
		k++
	}
	if k < len(dx) { // se l'inchiostro arriva fino al limite è un'altra parola: non allargo
		b.W += k
	}
	sx := colonneInchiostro(img, b, b.X-lim, b.X)
	k = 0
	for k < len(sx) && sx[len(sx)-1-k] >= soglia {
		k++
	}
	if k < len(sx) {
		b.X -= k
		b.W += k
	}
	// sopra e sotto: righe con inchiostro attaccate (accenti, gambe delle lettere)
	bd := img.Bounds()
	riga := func(y int) int {
		n := 0
		if y < bd.Min.Y || y >= bd.Max.Y {
			return 0
		}
		for x := max(b.X, bd.Min.X); x < min(b.X+b.W, bd.Max.X); x++ {
			if eInchiostro(img.RGBAAt(x, y), b) {
				n++
			}
		}
		return n
	}
	limV := max(1, int(em/4+0.5))
	sogliaR := max(1, b.W/40)
	for k = 0; k < limV && riga(b.Y-1) >= sogliaR; k++ {
		b.Y--
		b.H++
	}
	for k = 0; k < limV && riga(b.Y+b.H) >= sogliaR; k++ {
		b.H++
	}
}

// calcolaToppa: il riquadro che copre l'inglese e la scritta italiana. x0..x1 e y0..y1: dalla
// scritta originale (più il margine m) a tutta la traduzione; una riga sola non scende sotto le
// sue lettere (non copre la voce sotto). testo: il rettangolo della scritta italiana.
func calcolaToppa(b Blocco, testo image.Rectangle, larg, m int, unaRiga bool) image.Rectangle {
	x0 := min(b.X, testo.Min.X+2) - m
	x1 := max(b.X+larg, testo.Max.X-2) + m
	y0 := b.Y - m
	y1 := y0 + max(b.H, testo.Dy()-4) + 2*m
	if unaRiga {
		y1 = b.Y + b.H + m
	}
	r := image.Rect(x0, y0, x1, y1)
	if b.Pulita != nil {
		r = r.Union(testo)
	}
	return r
}

// fineFrase: la parola chiude una frase (".", "!", "?", anche dentro le virgolette).
func fineFrase(w string) bool {
	t := strings.TrimRight(strings.TrimSpace(w), "\"'»”’)")
	return strings.HasSuffix(t, ".") || strings.HasSuffix(t, "!") || strings.HasSuffix(t, "?") || strings.HasSuffix(t, "…")
}

// fraseCompleta: una frase intera (maiuscola all'inizio, punto, "!" o "?" alla fine, almeno tre parole).
func fraseCompleta(t string) bool {
	t = strings.TrimSpace(t)
	if len(strings.Fields(t)) < 3 || !fineFrase(t) {
		return false
	}
	for _, c := range t {
		if unicode.IsLetter(c) {
			return unicode.IsUpper(c)
		}
		if unicode.IsDigit(c) {
			return false
		}
	}
	return false
}

// fraseConPocoAccanto: la scritta è una frase intera e l'inchiostro non letto accanto è corto
// (al massimo ~4 lettere, poi niente): un'etichetta come "TIP:" su un fondo che l'OCR non ha letto,
// un'icona, un numero. Si traduce e si copre solo la frase letta (l'etichetta resta del gioco):
// meglio della frase intera lasciata in inglese. Se accanto continua una parola lunga o altre
// parole, la riga resta incompleta.
func fraseConPocoAccanto(img *image.RGBA, b Blocco, altri []Blocco) bool {
	if !fraseCompleta(b.Testo) {
		return false
	}
	coloriBloccoSemplice(img, &b)
	b.HaContorno = false
	sx, dx := fasceLati(b)
	for _, l := range []struct {
		f     Blocco
		verso int
	}{{sx, -1}, {dx, 1}} {
		if latoContinua(img, l.f, altri, b, l.verso) && !inchiostroCorto(img, l.f, b, altri, l.verso) {
			return false
		}
	}
	return true
}

// inchiostroCorto: dal bordo della scritta verso un lato, l'inchiostro attaccato è un gruppo corto
// (largo al massimo 2,6 em) seguito da almeno 0,9 em vuoti (o dal bordo dell'immagine).
func inchiostroCorto(img *image.RGBA, f, tutta Blocco, altri []Blocco, verso int) bool {
	em := f.Em
	if em <= 0 {
		em = float64(f.H) * 1.1
	}
	largo := int(5*em + 0.5)
	x0 := f.X + f.W
	if verso < 0 {
		x0 = f.X - largo
	}
	bd := img.Bounds()
	col := colonneInchiostro(img, &f, x0, x0+largo)
	togliAltreScritte(col, x0, f, tutta, altri)
	minCol := max(2, f.H*15/100)
	vuotoFine := int(0.9*em + 0.5)
	primo, ultimo, vuote := -1, -1, 0
	for k := 0; k < len(col); k++ {
		i := k
		if verso < 0 {
			i = len(col) - 1 - k
		}
		if x := x0 + i; x < bd.Min.X || x >= bd.Max.X {
			if primo < 0 {
				return false
			}
			break // bordo dell'immagine: il gruppo finisce qui
		}
		if col[i] >= minCol {
			if primo < 0 {
				primo = k
			}
			ultimo, vuote = k, 0
			continue
		}
		if primo >= 0 {
			if vuote++; vuote >= vuotoFine {
				break
			}
		}
	}
	if primo < 0 {
		return true
	}
	if vuote < vuotoFine && ultimo >= len(col)-1-vuotoFine {
		return false // l'inchiostro continua fino al limite: non è un gruppo corto
	}
	return float64(ultimo-primo+1) <= math.Min(2.6*em, 2.4*float64(f.H))
}
