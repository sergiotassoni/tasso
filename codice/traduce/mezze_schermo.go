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
		motivo == "uguale all'inglese", motivo == "forma strana", motivo == motivoNomeCodice,
		strings.HasPrefix(motivo, "nome di un veicolo"), motivo == motivoTitoloVideo, motivo == motivoNomeGioco:
		return false
	}
	return true
}

// motivoNomeCodice: nome di un posto con un codice ("1S4V1 POINT"): resta in inglese.
const motivoNomeCodice = "nome con un codice (resta uguale)"

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
			if conIngleseAccanto(p, ingl) || pezzoSenzaTesta(p, tenute) || sceltaConIngleseInColonna(p, ingl) {
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
		return stessaLineaSchermo(p, e)
	}
	// una riga di un paragrafo e un pezzo sulla stessa linea ("Press [E] to shoot… Hold" /
	// "for a powerful shot." in inglese, "E while aiming" tradotto accanto alla prima riga)
	if righeSchermo(p) > 1 || righeSchermo(e) > 1 {
		for _, rp := range righeDelBlocco(p) {
			for _, re := range righeDelBlocco(e) {
				if lineaSimile(rp, re) && stessaLineaSchermo(rp, re) {
					return true
				}
			}
		}
	}
	// riga sopra o sotto della stessa frase
	sopra, sotto := p, e
	if e.Y < p.Y {
		sopra, sotto = e, p
	}
	return seguitoSotto(sopra, sotto)
}

func righeSchermo(b Blocco) int { return max(1, b.Righe) }

// stessaLineaSchermo: p ed e, sulla stessa linea, sono pezzi della stessa frase: accavallati,
// fratelli vicini che non sono due voci di una fila, o pezzi che si continuano anche con l'icona
// di un tasto in mezzo o con altezze diverse ("…Hold" | "Q to aim.", "…one we" | "lost...").
func stessaLineaSchermo(p, e Blocco) bool {
	sx, dx := p, e
	if e.X < p.X {
		sx, dx = e, p
	}
	if dx.X < sx.X+sx.W {
		return true // accavallati sulla stessa linea
	}
	if fratelli(sx, dx) && (!vociSeparate(sx, dx) || pezzoDiFraseSchermo(sx) || pezzoDiFraseSchermo(dx)) {
		// (due pulsanti di una fila restano separati; ma se uno dei due è un pezzo di frase,
		// "Control, this is Delta" | "We have visuals on Alpha" | "Possible": sullo schermo è
		// UNA riga, e si vede intera in italiano o intera in inglese)
		return true
	}
	return pezziStessaLinea(sx, dx)
}

// pezzoDiFraseSchermo: il pezzo di una riga è un pezzo di frase e non una voce di una fila di pulsanti:
// almeno 4 parole, oppure finisce con la punteggiatura di una frase o con una parola che vuole il
// seguito.
func pezzoDiFraseSchermo(b Blocco) bool {
	t := strings.TrimSpace(b.Testo)
	if len(strings.Fields(t)) >= 4 {
		return true
	}
	return len(strings.Fields(t)) >= 2 && (strings.HasSuffix(t, ",") || chiedeSeguito(t))
}

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
	if len(nomeChiParla(sotto.Testo)) > 0 {
		return false // "Brodie: The cable's stuck.": chi parla comincia una battuta nuova
	}
	// una parola sola chiusa dal punto sotto una riga non finita ("…while I'm" / "Here.")
	pezzo := parolaSolaChiusa(sotto.Testo) && !tuttoMaiuscolo(t)
	if lontana {
		return iniziaMinuscola(sotto.Testo) || pezzo
	}
	return iniziaMinuscola(sotto.Testo) || pezzo || strings.HasSuffix(t, ",") || strings.HasSuffix(t, "-") ||
		strings.HasSuffix(t, "—") || chiedeSeguito(t)
}

// sceltaConIngleseInColonna: p è una voce di un elenco di scelte una sotto l'altra ("I'll buy!" /
// "Never mind.") e una voce vicina della stessa colonna resta in inglese. Le scelte sono un
// gruppo: tutte tradotte o tutte in inglese, mai "I'll buy!" accanto a "Non importa.". Si
// ripete in senzaMezzeRighe, così la voce tolta fa togliere anche quella dopo.
func sceltaConIngleseInColonna(p Blocco, inglesi []Blocco) bool {
	if !voceCorta(p) {
		return false
	}
	for _, e := range inglesi {
		if e.X == p.X && e.Y == p.Y && e.W == p.W && e.H == p.H {
			continue
		}
		if voceCorta(e) && stessaColonnaVoci(p, e) {
			return true
		}
	}
	return false
}

// voceCorta: una riga sola, poche parole, non una battuta con chi parla ("Caz: Yes.").
func voceCorta(b Blocco) bool {
	t := strings.TrimSpace(b.Testo)
	return righeSchermo(b) == 1 && b.H > 0 && t != "" && len(t) <= 40 && len(strings.Fields(t)) <= 6 &&
		len(nomeChiParla(t)) == 0
}

// stessaColonnaVoci: a e b sono due voci una sotto l'altra della stessa colonna: stessa
// grandezza, buco piccolo (al massimo una riga) e allineate a sinistra o al centro.
func stessaColonnaVoci(a, b Blocco) bool {
	h := min(a.H, b.H)
	// (un'icona del tasto nella riga, "Move the camera with (R)", la fa un po' più alta)
	if float64(max(a.H, b.H)) > 1.5*float64(h) {
		return false
	}
	sopra, sotto := a, b
	if b.Y < a.Y {
		sopra, sotto = b, a
	}
	buco := sotto.Y - (sopra.Y + sopra.H)
	if buco < -h/4 || buco > h*3/2 {
		return false
	}
	return abs(a.X-b.X) <= h || abs((a.X+a.W/2)-(b.X+b.W/2)) <= h
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
