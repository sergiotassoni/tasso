package traduce

// Settimo giro sui video veri (r3: gli stessi 9 giochi, 5 minuti più avanti). Regole universali:
//   - pezzi di frase disegnati da soli: "here." sotto "…while I'm" (tradotto a parte: "Qui."),
//     "area." in fondo a un paragrafo, "elect" (una parola minuscola da sola), "press to
//     sprint." (riga ripresa a metà: la "P" letta minuscola). Un pezzo che comincia
//     minuscolo, o una parola sola chiusa dal punto subito sotto un'altra riga della stessa
//     frase, non si disegna mai da solo; "Press" da solo nemmeno;
//   - segni letti dall'icona accanto a una voce ("/Back" → "/Indietto", "'Close", "[Shipwreck",
//     "/SUITS /SETTINGS"): il segno si toglie prima di tradurre e la traduzione non copre l'icona;
//   - scritte del canale o del logo lette male ("Gamer's Litüe Playgrmjnd"): due nomi
//     sconosciuti, uno con lettere impossibili → resta l'inglese;
//   - parolacce nate da un errore dell'OCR ("baking the cunt!" era "cake"): nel dubbio l'inglese.

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// ---------------------------------------------------------------- segni letti dall'icona

// segniIcona: segni che l'OCR legge dal bordo di un'icona attaccata alla parola.
const segniIcona = "/\\|[]()'‘’`{}"

// coppiaSegno: il segno che chiude (o apre) quello dato.
func coppiaSegno(c rune) rune {
	switch c {
	case '[':
		return ']'
	case ']':
		return '['
	case '(':
		return ')'
	case ')':
		return '('
	case '{':
		return '}'
	case '}':
		return '{'
	case '\'', '‘', '’', '`':
		return '\''
	}
	return 0
}

// contrazioni inglesi con l'apostrofo davanti: l'apostrofo è della parola
var apostrofoDavanti = map[string]bool{"em": true, "bout": true, "cause": true, "til": true, "tis": true,
	"twas": true, "n": true, "round": true, "cept": true, "nuff": true, "sup": true, "kay": true, "ello": true}

// segnoSpaiato: il segno c in testa (o in fondo) alla scritta s non ha il suo compagno.
func segnoSpaiato(s string, c rune, inTesta bool) bool {
	if !strings.ContainsRune(segniIcona, c) {
		return false
	}
	resto := []rune(s)
	if inTesta {
		resto = resto[1:]
	} else {
		resto = resto[:len(resto)-1]
	}
	switch c {
	case '/', '\\', '|':
		return true
	case '\'', '‘', '’', '`':
		if inTesta {
			w := strings.ToLower(strings.FieldsFunc(string(resto), func(r rune) bool { return !unicode.IsLetter(r) })[0])
			if apostrofoDavanti[w] {
				return false
			}
		}
		return !strings.ContainsAny(string(resto), "'‘’`")
	}
	return !strings.ContainsRune(string(resto), coppiaSegno(c))
}

// togliSegniIcona: voci corte con un segno spaiato attaccato in testa o in fondo ("/Back",
// "'Close", "[Shipwreck", "SAVE]"): il segno è l'icona (o il disegno della voce) e non si
// traduce né si copre. Una fila di voci con il segno davanti a ognuna ("/SUITS /SETTINGS") si
// divide in voci. Solo righe corte (fino a 5 parole) con le posizioni delle parole.
func togliSegniIcona(b []Blocco) []Blocco {
	var out []Blocco
	for _, bl := range b {
		p := append([]ParolaOCR(nil), bl.parole()...)
		if max(1, bl.Righe) != 1 || len(p) == 0 || len(p) > 5 || len(p) != len(strings.Fields(bl.Testo)) {
			out = append(out, bl)
			continue
		}
		sort.SliceStable(p, func(i, j int) bool { return p[i].X < p[j].X })
		// fila di voci: ogni parola dopo la prima che comincia con "/" apre una voce nuova
		var gruppi [][]ParolaOCR
		cur := []ParolaOCR{p[0]}
		for i := 1; i < len(p); i++ {
			if strings.HasPrefix(p[i].T, "/") && strings.HasPrefix(p[0].T, "/") && len([]rune(p[i].T)) > 1 {
				gruppi = append(gruppi, cur)
				cur = nil
			}
			cur = append(cur, p[i])
		}
		gruppi = append(gruppi, cur)
		for _, g := range gruppi {
			n := bl
			if len(gruppi) > 1 {
				n = bloccoDaPosizioni(g, bl)
			}
			out = append(out, senzaSegnoIcona(n))
		}
	}
	return out
}

// senzaSegnoIcona: toglie il segno spaiato in testa e in fondo alla voce (e il suo spazio).
func senzaSegnoIcona(bl Blocco) Blocco {
	p := append([]ParolaOCR(nil), bl.parole()...)
	if len(p) == 0 || len(p) != len(strings.Fields(bl.Testo)) {
		return bl
	}
	cambiata := false
	if r := []rune(p[0].T); len(r) >= 3 && unicode.IsLetter(r[1]) && segnoSpaiato(bl.Testo, r[0], true) {
		taglio := p[0].W / float64(len(r))
		p[0].T = string(r[1:])
		p[0].X += taglio
		p[0].W -= taglio
		cambiata = true
	}
	u := len(p) - 1
	if r := []rune(p[u].T); len(r) >= 3 && unicode.IsLetter(r[len(r)-2]) && segnoSpaiato(testoParole(p), r[len(r)-1], false) && r[len(r)-1] != '\'' && r[len(r)-1] != ')' {
		taglio := p[u].W / float64(len(r))
		p[u].T = string(r[:len(r)-1])
		p[u].W -= taglio
		cambiata = true
	}
	if !cambiata {
		return bl
	}
	n := bloccoDaPosizioni(p, bl)
	if n.X > bl.X {
		n.IconaSx = n.X // l'icona a sinistra resta del gioco
	}
	if n.X+n.W < bl.X+bl.W {
		n.IconaDx = bl.X + bl.W
	}
	return n
}

// ---------------------------------------------------------------- pezzi di frase da soli

// pezzoDiFrase: la scritta è un pezzo che continua qualcosa (comincia minuscola) o una parola
// sola chiusa dal punto ("here.", "area.", "Here.").
func pezzoDiFrase(t string) bool {
	t = strings.TrimSpace(t)
	if iniziaMinuscola(t) {
		return true
	}
	return parolaSolaChiusa(t)
}

// parolaSolaChiusa: una parola sola di lettere chiusa da . ! ? ("here.", "Here.").
func parolaSolaChiusa(t string) bool {
	w := strings.Fields(strings.TrimSpace(t))
	if len(w) != 1 {
		return false
	}
	p := strings.TrimRight(w[0], ".!?…")
	if p == w[0] || len([]rune(p)) < 2 {
		return false
	}
	for _, c := range p {
		if !unicode.IsLetter(c) && c != '\'' {
			return false
		}
	}
	return true
}

// comandoTasto: la scritta finisce con un verbo che vuole l'icona del tasto dopo ("Press",
// "Tap Q to throw. Hold").
func comandoTasto(t string) bool {
	w := strings.Fields(strings.TrimSpace(t))
	return len(w) > 0 && verbiTasto[strings.ToLower(strings.Trim(w[len(w)-1], ".,:;"))]
}

// pezzoSenzaTesta: la scritta pronta p non può essere disegnata da sola. tenute: le altre
// scritte pronte. Casi:
//   - pezzo minuscolo o parola sola chiusa dal punto subito sotto un'altra scritta pronta della
//     stessa frase: sono due metà tradotte ognuna per conto suo ("…Mentre sono" / "Qui.");
//   - pezzo minuscolo sulla stessa riga dopo un'altra scritta pronta che non è un comando con
//     l'icona del tasto ("Press [⇧] to sprint." va bene);
//   - pezzo minuscolo senza nessuna scritta prima (la sua testa non è stata letta: "elect",
//     "press to sprint."), salvo le voci di menu esatte del catalogo scritte minuscole;
//   - "Press"/"Hold" da solo, senza il seguito tradotto sulla stessa riga.
func pezzoSenzaTesta(p Blocco, tenute []Blocco) bool {
	t := strings.TrimSpace(p.Testo)
	if comandoTasto(t) && len(strings.Fields(t)) == 1 {
		for _, q := range tenute {
			if q.X > p.X && righeSchermo(q) == 1 && lineaSimile(p, q) && iniziaMinuscola(q.Testo) && q.X-(p.X+p.W) <= 6*max(p.H, q.H) {
				return false
			}
		}
		return true
	}
	if !pezzoDiFrase(t) {
		return false
	}
	testa := false
	for _, q := range tenute {
		if q.X == p.X && q.Y == p.Y && q.W == p.W && q.H == p.H {
			continue
		}
		if q.Y < p.Y && seguitoSotto(q, p) {
			return true // due metà della stessa frase tradotte a parte
		}
		if righeSchermo(p) == 1 && righeSchermo(q) == 1 && lineaSimile(p, q) && q.X+q.W <= p.X+p.H/2 &&
			float64(p.X-(q.X+q.W)) <= 6*math.Max(float64(p.H), float64(q.H)) {
			if !comandoTasto(q.Testo) {
				return true
			}
			testa = true
		}
	}
	if testa {
		return false
	}
	if iniziaMinuscola(t) {
		// nessuna testa letta: il pezzo continua qualcosa che non si vede tradotto
		return !(p.Affid == letturaCatalog && !strings.ContainsAny(t, ".!?,"))
	}
	return false
}

// ---------------------------------------------------------------- loghi e nomi letti male

// nomeImpossibile: una parola con lettere che in inglese non stanno insieme: cinque consonanti di
// fila ("Playgrmjnd") o una lettera accentata in mezzo a una parola inglese ("Litüe").
func nomeImpossibile(w string) bool {
	r := []rune(strings.ToLower(strings.Trim(w, ".,:;!?\"'()")))
	cons := 0
	for i, c := range r {
		if strings.ContainsRune("bcdfghjklmnpqrstvwxz", c) {
			cons++
			if cons >= 5 {
				return true
			}
		} else {
			cons = 0
		}
		if c > 127 && unicode.IsLetter(c) && i > 0 && i < len(r)-1 {
			return true
		}
	}
	return false
}

// logoLettoMale: scritta corta (fino a 5 parole) con almeno due parole sconosciute con la
// maiuscola (per l'OCR sono "nomi") e almeno una impossibile: è il logo o il nome di un canale
// letto male ("Gamer's Litüe Playgrmjnd"), non un nome vero da lasciare in una frase tradotta.
func logoLettoMale(s string) bool {
	w := strings.Fields(s)
	if len(w) < 2 || len(w) > 5 {
		return false
	}
	sconosciuti, impossibili := 0, 0
	for _, x := range w {
		t := strings.Trim(x, ".,:;!?\"'()")
		r := []rune(t)
		if len(r) < 3 || !unicode.IsUpper(r[0]) || esiste(strings.TrimSuffix(strings.ToLower(t), "'s")) {
			continue
		}
		sconosciuti++
		if nomeImpossibile(t) {
			impossibili++
		}
	}
	return sconosciuti >= 2 && impossibili >= 1
}

// ---------------------------------------------------------------- parolacce dell'OCR

// parolacceConfondibili: parolacce che l'OCR può leggere da una parola comune simile
// ("cake" → "cunt", "duck" → "fuck", "shift" → "shit").
var parolacceConfondibili = map[string]bool{"cunt": true, "fuck": true, "shit": true, "dick": true, "cock": true,
	"piss": true, "tits": true, "twat": true, "slut": true, "whore": true, "bitch": true, "pussy": true}

// determinanti: davanti a un nome ("the cake", "my dick"): la parolaccia lì è usata come un
// oggetto qualsiasi, proprio dove l'OCR mette la parola letta male.
var determinanti = map[string]bool{"the": true, "a": true, "an": true, "my": true, "your": true, "his": true,
	"her": true, "our": true, "their": true, "this": true, "that": true, "some": true}

// parolacciaDubbia: nella riga c'è una parolaccia usata come un oggetto ("baking the cunt!") e
// c'è una parola comune che differisce per una o due lettere: può essere un errore dell'OCR, e
// tradotta sarebbe volgare a sproposito. Le parolacce usate come tali ("you cunt", "fucking
// hell", "shit!") restano: sono del gioco.
func parolacciaDubbia(s string) bool {
	w := strings.Fields(strings.ToLower(s))
	for i, x := range w {
		t := strings.Trim(x, ".,:;!?\"'()-…")
		if !parolacceConfondibili[t] || i == 0 {
			continue
		}
		if !determinanti[strings.Trim(w[i-1], ".,:;!?\"'()")] {
			continue
		}
		for c, l := range vocabolario() {
			if l <= 35 && !parolacceConfondibili[c] && len(c) >= len(t)-1 && len(c) <= len(t)+1 && distanzaModifica(c, t) <= 2 && c[0] == t[0] {
				return true
			}
		}
	}
	return false
}
