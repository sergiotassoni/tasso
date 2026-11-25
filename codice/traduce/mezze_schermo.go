package traduce

// Ultimo controllo prima di disegnare: MAI mezza riga tradotta sullo schermo.
//
// Le regole prima (fratelli, righe vicine, letture doppie) guardano una scritta alla volta. Qui
// si guarda il risultato intero: le scritte pronte da disegnare e quelle che restano in inglese
// con parole vere (lette male, incomplete, mezze…). Una scritta pronta che tocca una scritta
// rimasta in inglese della stessa frase non si disegna, perché sullo schermo si vedrebbe una
// frase metà italiana e metà inglese:
//   - stessa riga, riquadri accavallati (la stessa riga letta due volte, un pezzo tradotto);
//   - stessa riga, pezzi vicini come parole della stessa frase (non due pulsanti di una fila);
//   - riga sopra o sotto della stessa frase (la frase continua: virgola, minuscola…).
// Vale anche per le traduzioni rimaste dal fotogramma prima (tieniFerme, recuperaPerse): se lì
// adesso la lettura dice "inglese", la traduzione vecchia non si ridisegna sopra.
//
// Si ripete finché non cambia niente: una scritta tolta resta in inglese e può far togliere la
// sua vicina.

import (
	"image"
	"math"
	"strings"
)

// ingleseVisibile: la scritta lasciata com'è per questo motivo mostra parole inglesi che fanno
// parte di una frase. Non contano i segni e i numeri, le scritte uguali in italiano, i nomi.
func ingleseVisibile(motivo string) bool {
	switch {
	case motivo == "", motivo == "non è testo da tradurre", motivo == motivoUgualeIt,
		motivo == "uguale all'inglese", motivo == "forma strana",
		strings.HasPrefix(motivo, "nome di un veicolo"):
		return false
	}
	return true
}

// motivoMezzaRiga: il motivo scritto nel banco per le scritte tolte da questo controllo.
const motivoMezzaRiga = "pezzo di una frase rimasta in inglese (mai mezza riga tradotta)"

// senzaMezzeRighe: le scritte pronte senza quelle che toccano una scritta della stessa frase
// rimasta in inglese. Restituisce le tenute e le tolte.
func senzaMezzeRighe(pronti, inglesi []Blocco) (tenute, tolte []Blocco) {
	tenute = append([]Blocco(nil), pronti...)
	ingl := append([]Blocco(nil), inglesi...)
	for {
		via := -1
		for i, p := range tenute {
			if conIngleseAccanto(p, ingl) || pezzoSenzaTesta(p, tenute) {
				via = i
				break
			}
		}
		if via < 0 {
			return tenute, tolte
		}
		tolte = append(tolte, tenute[via])
		ingl = append(ingl, tenute[via])
		tenute = append(tenute[:via], tenute[via+1:]...)
	}
}

// conIngleseAccanto: la scritta pronta p tocca una scritta in inglese della stessa frase.
func conIngleseAccanto(p Blocco, inglesi []Blocco) bool {
	for _, e := range inglesi {
		if e.X == p.X && e.Y == p.Y && e.W == p.W && e.H == p.H {
			continue
		}
		if stessaFraseSchermo(p, e) {
			return true
		}
	}
	return false
}

// stessaFraseSchermo: p (da disegnare) ed e (in inglese) sono pezzi della stessa frase sullo
// schermo.
func stessaFraseSchermo(p, e Blocco) bool {
	rp := image.Rect(p.X, p.Y, p.X+p.W, p.Y+p.H)
	re := image.Rect(e.X, e.Y, e.X+e.W, e.Y+e.H)
	if in := rp.Intersect(re); !in.Empty() {
		ae := re.Dx() * re.Dy()
		if in.Dx()*in.Dy()*10 >= ae*9 {
			return false // l'inglese sta tutto sotto la traduzione: la toppa lo copre
		}
		if in.Dx()*in.Dy()*5 >= min(ae, rp.Dx()*rp.Dy()) {
			return true // riquadri accavallati: metà coperta, metà inglese
		}
	}
	// stessa riga
	if righeSchermo(p) == 1 && righeSchermo(e) == 1 && lineaSimile(p, e) {
		sx, dx := p, e
		if e.X < p.X {
			sx, dx = e, p
		}
		if dx.X < sx.X+sx.W {
			return true // accavallati sulla stessa linea
		}
		if fratelli(sx, dx) && !vociSeparate(sx, dx) {
			return true
		}
		return false
	}
	// riga sopra o sotto della stessa frase
	sopra, sotto := p, e
	if e.Y < p.Y {
		sopra, sotto = e, p
	}
	return seguitoSotto(sopra, sotto)
}

func righeSchermo(b Blocco) int { return max(1, b.Righe) }

// seguitoSotto: la scritta sotto continua la frase di quella sopra: subito sotto (fino a una
// riga e mezza di buco con l'interlinea larga dei dialoghi, se sotto comincia minuscola), stessa grandezza, in colonna (stesso inizio, stesso centro o
// accavallate in larghezza) e la frase non è finita (sopra finisce con virgola o con una parola
// che vuole il seguito, o sotto comincia minuscola).
func seguitoSotto(sopra, sotto Blocco) bool {
	hs := float64(sopra.H) / float64(righeSchermo(sopra))
	hb := float64(sotto.H) / float64(righeSchermo(sotto))
	if hs <= 0 || hb <= 0 {
		return false
	}
	if math.Max(hs, hb) > 1.35*math.Min(hs, hb) {
		// le parole senza gambe ("area.", "here.") sono più basse: conta la grandezza del carattere
		if !(righeSchermo(sopra) == 1 && righeSchermo(sotto) == 1 && sopra.Em > 0 && sotto.Em > 0 &&
			math.Max(sopra.Em, sotto.Em) <= 1.3*math.Min(sopra.Em, sotto.Em)) {
			return false
		}
		hs, hb = sopra.Em*0.8, sotto.Em*0.8
	}
	buco := float64(sotto.Y - (sopra.Y + sopra.H))
	if buco < -0.5*math.Min(hs, hb) || buco > 1.5*math.Max(hs, hb) {
		return false
	}
	lontana := buco > 0.8*math.Max(hs, hb) // interlinea larga: solo se sotto comincia minuscola
	comune := min(sopra.X+sopra.W, sotto.X+sotto.W) - max(sopra.X, sotto.X)
	centri := abs((sopra.X+sopra.W/2)-(sotto.X+sotto.W/2)) <= int(hs)
	if comune*10 < min(sopra.W, sotto.W)*3 && !centri {
		return false
	}
	t := strings.TrimSpace(sopra.Testo)
	if fineFraseTesto(t) {
		return false
	}
	// una parola sola chiusa dal punto sotto una riga non finita ("…while I'm" / "Here.")
	pezzo := parolaSolaChiusa(sotto.Testo) && !tuttoMaiuscolo(t)
	if lontana {
		return iniziaMinuscola(sotto.Testo) || pezzo
	}
	return iniziaMinuscola(sotto.Testo) || pezzo || strings.HasSuffix(t, ",") || strings.HasSuffix(t, "-") ||
		strings.HasSuffix(t, "—") || chiedeSeguito(t)
}

// mezzeDisegnate: quante scritte pronte toccano una scritta della stessa frase rimasta in
// inglese (per il banco: dopo senzaMezzeRighe deve essere 0).
func mezzeDisegnate(pronti, inglesi []Blocco) int {
	n := 0
	for _, p := range pronti {
		if conIngleseAccanto(p, inglesi) {
			n++
		}
	}
	return n
}
