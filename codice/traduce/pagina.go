package traduce

// Modalità "tutta la schermata" (F6): per i menu pieni di scritte. L'OCR dà ogni parola con la
// sua posizione; qui le raggruppo in blocchi (una voce di menu, una riga di lista, un paragrafo),
// e per ogni blocco calcolo il colore dello sfondo e del testo, così la traduzione viene
// disegnata esattamente sopra l'inglese e sembra scritta dal gioco.

import (
	"image"
	"image/color"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

type ParolaOCR struct {
	T string  `json:"t"`
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

type RigaOCR struct {
	Parole []ParolaOCR `json:"p"`
}

// Blocco: testo e rettangolo in pixel della cattura (non ingrandita).
type Blocco struct {
	Testo       string
	X, Y, W, H  int
	Righe       int
	Sfondo      color.RGBA
	Inchiostro  color.RGBA
	Tradotto    string
	Em          float64     // grandezza del carattere stimata dalle lettere (pixel)
	LargMax     int         // spazio libero a destra prima della scritta vicina
	AltMax      int         // spazio libero in basso (dalla Y) prima della scritta sotto
	Fondo       *image.RGBA // sfondo ricostruito (sfumature del gioco), grande come il blocco +3 px per lato
	Tratto      float64     // spessore medio delle lettere diviso la grandezza (grassetto se alto)
	FondoX      int         // quanti pixel il Fondo parte a sinistra di X (di solito 3)
	Centro      bool        // scritta centrata come le sue vicine (menu al centro): la traduzione resta centrata
	Ombra       bool        // sfondo che si muove: la scritta ha un'ombra scura, come i testi dei giochi
	Img         *image.RGBA // la cattura del gioco: la toppa copia i suoi pixel veri
	Contorno    color.RGBA  // colore del contorno delle lettere (scritte bordate dei giochi)
	HaContorno  bool
	OmbraGioco  color.RGBA // ombra scura che il gioco mette sotto le lettere (in basso a destra)
	HaOmbraG    bool
	Inchiostro2 color.RGBA // secondo colore delle lettere nella stessa riga (parole evidenziate)
	Ha2         bool
	Pulita      *image.RGBA  // la cattura con TUTTE le scritte inglesi tradotte cancellate
	PxColonna   int          // testo piccolo in colonna (notizie): stessa grandezza per tutte le righe
	ColRif      string       // riga di riferimento della colonna, per scegliere il carattere
	ColRifW     int          // sua larghezza nel gioco
	ColLarg     int          // larghezza della colonna
	Marg        int          // margine della toppa attorno all'inglese (0 = non calcolato): mai fino alla scritta vicina
	Bordo       bool         // LargMax arriva al bordo vero del riquadro del gioco (casella, cornice)
	Tinta       [12]uint8    // colore medio di 4 pezzi del riquadro inglese: se cambia, il gioco ha ricolorato la voce
	Pos         *[]ParolaOCR // le parole lette con il loro rettangolo (pixel della cattura): per dividere e unire le righe (puntatore: Blocco resta confrontabile)
	Grezzo      string       // il testo come l'ha letto l'OCR, prima delle correzioni (per il banco di prova)
	Affid       int          // quanto mi fido della lettura (letturaDubbia/Buona/Catalog, vedi ocr_riga.go)
	Mossa       bool         // la stessa scritta pochi secondi fa era altrove (si muove con la scena: scritta del mondo)
	// IconaDx / IconaSx: dove comincia l'icona del tasto tolta a destra della scritta ("Swing
	// Sword (Y)") e dove finisce quella tolta a sinistra (0 = nessuna). La traduzione non ci va
	// mai sopra: con l'icona a destra cresce verso sinistra (SpazioSx: spazio libero a sinistra).
	IconaDx, IconaSx int
	SpazioSx         int
}

// parole: le parole lette della scritta, con il loro rettangolo (nil se non le so).
func (b Blocco) parole() []ParolaOCR {
	if b.Pos == nil {
		return nil
	}
	return *b.Pos
}

// largConsentita: quanto può essere larga al massimo la traduzione di questa scritta. Fino al
// bordo del riquadro del gioco se l'ho trovato (o alla larghezza della colonna); se no solo poco
// più dell'inglese: mai fino alla scritta vicina o al bordo dello schermo, uscirebbe dal pulsante.
func largConsentita(b Blocco) int {
	lim := max(b.LargMax, b.W)
	if b.Bordo || b.PxColonna > 0 {
		return lim
	}
	return min(lim, b.W+max(int(2*b.Em+0.5), b.W*40/100, 8))
}

// largConIcona: con l'icona del tasto a destra, quanto può essere larga la traduzione: fino
// all'icona (largConsentita, che non la supera) più lo spazio libero a sinistra, come per
// l'inglese (al massimo 2 em o il 40% in più). Senza icona: largConsentita.
func largConIcona(b Blocco) int {
	l := largConsentita(b)
	if b.IconaDx <= b.X || b.SpazioSx <= 0 {
		return l
	}
	return l + min(b.SpazioSx, max(int(2*b.Em+0.5), b.W*40/100, 8))
}

// inizioConIcona: dove comincia una traduzione larga w. Allineata a sinistra come l'inglese; con
// l'icona del tasto a destra, se non ci sta prima dell'icona, si sposta a sinistra quanto serve
// (finisce dove poteva finire, mai sopra l'icona).
func inizioConIcona(b Blocco, w int) int {
	if b.IconaDx <= b.X {
		return b.X
	}
	destra := b.X + largConsentita(b)
	return min(b.X, destra-w)
}

// grassetto: lettere spesse come quelle in neretto dei giochi.
func (b Blocco) grassetto() bool { return b.Tratto >= sogliaGrassetto }

// pesoCarattere: lo spessore del carattere come quello del gioco (normale, semi-neretto, neretto).
func (b Blocco) pesoCarattere() uintptr {
	if b.Em > 0 && b.Em < 20 { // lettere piccole: il tratto di 2 pixel non vuol dire grassetto
		switch px := b.Tratto * b.Em; {
		case px >= 3.2:
			return 700
		case px >= 2.6:
			return 600
		}
		return 400
	}
	switch {
	case b.Tratto >= sogliaGrassetto:
		return 700
	case b.Tratto >= 0.08:
		return 600
	}
	return 400
}

const sogliaGrassetto = 0.135

// segmenti spezza le righe dell'OCR dove c'è un grande spazio vuoto tra due parole (colonne
// diverse sulla stessa riga: "Asset Sex: Male      Diet: Carnivore").
func segmenti(righe []RigaOCR, fattore float64) []Blocco {
	if fattore <= 0 {
		fattore = 1
	}
	var out []Blocco
	for _, r := range righe {
		if len(r.Parole) == 0 {
			continue
		}
		for _, sotto := range spezzaInAltezza(r.Parole) {
			out = append(out, segmentiRiga(sotto, fattore)...)
		}
	}
	return out
}

// spezzaInAltezza: a volte l'OCR mette in una riga parole di due righe diverse (colonne
// vicine). Le separo in base all'altezza del centro delle parole.
func spezzaInAltezza(p []ParolaOCR) [][]ParolaOCR {
	q := append([]ParolaOCR(nil), p...)
	sort.Slice(q, func(i, j int) bool { return q[i].Y+q[i].H/2 < q[j].Y+q[j].H/2 })
	hMed := 0.0
	for _, w := range q {
		hMed += w.H
	}
	hMed /= float64(len(q))
	var gruppi [][]ParolaOCR
	for _, w := range q {
		c := w.Y + w.H/2
		if n := len(gruppi); n > 0 {
			u := gruppi[n-1][len(gruppi[n-1])-1]
			if c-(u.Y+u.H/2) < 0.6*hMed {
				gruppi[n-1] = append(gruppi[n-1], w)
				continue
			}
		}
		gruppi = append(gruppi, []ParolaOCR{w})
	}
	return gruppi
}

func segmentiRiga(p []ParolaOCR, fattore float64) []Blocco {
	var out []Blocco
	sort.Slice(p, func(i, j int) bool { return p[i].X < p[j].X })
	hMed := 0.0
	for _, w := range p {
		hMed += w.H
	}
	hMed /= float64(len(p))
	inizio := 0
	for i := 1; i <= len(p); i++ {
		if i < len(p) {
			buco := p[i].X - (p[i-1].X + p[i-1].W)
			soglia := max(1.1*hMed, 28*fattore)
			if hMed < 17*fattore {
				// scritte piccole: una parola saltata dall'OCR lascia un buco largo, ma una
				// colonna vera è più lontana
				soglia = max(soglia, 5*hMed)
			}
			if buco < soglia { // spazio tra parole ~0,3-0,5 volte l'altezza: oltre è un'altra colonna
				continue
			}
		}
		out = append(out, bloccoDaParole(p[inizio:i], fattore))
		inizio = i
	}
	return out
}

func bloccoDaParole(p []ParolaOCR, f float64) Blocco {
	x0, y0, x1, y1 := 1e18, 1e18, -1e18, -1e18
	var parole []string
	pos := make([]ParolaOCR, 0, len(p))
	for _, w := range p {
		x0, y0 = min(x0, w.X), min(y0, w.Y)
		x1, y1 = max(x1, w.X+w.W), max(y1, w.Y+w.H)
		parole = append(parole, w.T)
		pos = append(pos, ParolaOCR{T: w.T, X: w.X / f, Y: w.Y / f, W: w.W / f, H: w.H / f})
	}
	return Blocco{
		Testo: strings.Join(parole, " "),
		X:     int(x0 / f), Y: int(y0 / f), W: int((x1-x0)/f + 0.5), H: int((y1-y0)/f + 0.5), Righe: 1,
		Em: emParole(p) / f, Pos: &pos,
	}
}

// emParole: la grandezza del carattere (em) dalle altezze delle parole. Un "Male" (maiuscola, niente
// gambe sotto) è alto circa 0,74 em, un "Weight" (sopra e sotto) circa 0,95 em, un "one" 0,53 em.
func emParole(p []ParolaOCR) float64 {
	var v []float64
	for _, w := range p {
		sopra, sotto, lettere := false, false, 0
		for _, c := range w.T {
			switch {
			case unicode.IsUpper(c) || unicode.IsDigit(c) || strings.ContainsRune("bdfhklt", c):
				sopra = true
			case strings.ContainsRune("gjpqy", c):
				sotto = true
			}
			if unicode.IsLetter(c) || unicode.IsDigit(c) {
				lettere++
			}
		}
		if lettere < 2 {
			continue
		}
		switch {
		case sopra && sotto:
			v = append(v, w.H/0.95)
		case sopra || sotto:
			v = append(v, w.H/0.74)
		default:
			v = append(v, w.H/0.53)
		}
	}
	if len(v) == 0 {
		return 0
	}
	sort.Float64s(v)
	return v[len(v)/2]
}

// spazioLibero: per ogni scritta, quanto può allargarsi verso destra prima di toccare la scritta
// vicina sulla stessa riga (o il bordo). L'italiano è spesso più lungo dell'inglese.
func spazioLibero(b []Blocco, larghezza int) {
	for i := range b {
		lim := larghezza - b[i].X - 4
		for j := range b {
			if i == j || b[j].X <= b[i].X {
				continue
			}
			if b[j].Y < b[i].Y+b[i].H && b[i].Y < b[j].Y+b[j].H { // si sovrappongono in altezza
				lim = min(lim, b[j].X-b[i].X-10)
			}
		}
		if b[i].IconaDx > b[i].X { // l'icona del tasto a destra: la traduzione si ferma prima
			lim = min(lim, b[i].IconaDx-b[i].X-max(4, b[i].H/4))
		}
		// spazio libero a sinistra (serve se un'icona a destra la ferma: cresce di là)
		sx := b[i].X - 4
		for j := range b {
			if i != j && b[j].X+b[j].W <= b[i].X && b[j].Y < b[i].Y+b[i].H && b[i].Y < b[j].Y+b[j].H {
				sx = min(sx, b[i].X-(b[j].X+b[j].W)-10)
			}
		}
		if b[i].IconaSx > 0 {
			sx = min(sx, b[i].X-b[i].IconaSx-max(4, b[i].H/4))
		}
		b[i].SpazioSx = max(0, sx)
		b[i].LargMax = max(lim, b[i].W)
		// in basso: fino alla scritta sotto che sta nella stessa colonna
		giu := b[i].H * 3
		for j := range b {
			if i == j || b[j].Y <= b[i].Y {
				continue
			}
			if b[j].X < b[i].X+max(b[i].W, b[i].LargMax) && b[i].X < b[j].X+b[j].W {
				giu = min(giu, b[j].Y-b[i].Y-max(4, b[i].H/(3*max(1, b[i].Righe))))
			}
		}
		b[i].AltMax = max(giu, b[i].H)
	}
}

// unisciParagrafi: righe una sotto l'altra, allineate a sinistra, con la frase che continua
// (la riga sotto inizia minuscola o quella sopra finisce con virgola/trattino) diventano un blocco.
// Le liste ("- Perfect Diet", "Boar, Located: ...") restano righe separate.
func unisciParagrafi(b []Blocco) []Blocco {
	sort.SliceStable(b, func(i, j int) bool {
		if abs(b[i].Y-b[j].Y) > min(b[i].H, b[j].H)/2 {
			return b[i].Y < b[j].Y
		}
		return b[i].X < b[j].X
	})
	usato := make([]bool, len(b))
	var out []Blocco
	for i := range b {
		if usato[i] {
			continue
		}
		cur := b[i]
		usato[i] = true
		hRiga := cur.H
		ultimaW, ultimoX := cur.W, cur.X
		centro := false
		for {
			trovato := -1
			for j := i + 1; j < len(b); j++ {
				if usato[j] {
					continue
				}
				n := b[j]
				gap := n.Y - (cur.Y + cur.H)
				// righe più spaziate (riquadri dei dialoghi con l'interlinea larga): fino a una
				// riga e mezza, ma solo se la frase continua davvero (vedi sotto)
				lontana := gap > hRiga
				if gap < -hRiga/2 || gap > hRiga*3/2 {
					continue
				}
				sinistra := abs(n.X-cur.X) <= hRiga
				// sottotitoli e riquadri centrati: le righe hanno lo stesso centro, non lo stesso inizio
				centrata := !sinistra && abs((n.X+n.W/2)-(ultimoX+ultimaW/2)) <= hRiga
				if (!sinistra && !centrata) || !altezzeSimili(cur, n, hRiga) {
					continue
				}
				if lontana {
					// "Oh, you haven't seen him yet...because Mido is" / "blocking the path?"
					// (Zelda): la riga sopra non finisce la frase e quella sotto la continua
					if continuaFrase(cur.Testo, n.Testo) && !fineFraseTesto(cur.Testo) && cur.Righe <= 3 {
						trovato = j
						centro = centro || centrata
					}
					break
				}
				if continuaFrase(cur.Testo, n.Testo) || rigaAndataACapo(cur.Testo, ultimaW, n) ||
					(centrata && gap <= hRiga*4/5 && righeStessoSottotitolo(cur.Testo, n.Testo)) {
					trovato = j
					centro = centro || centrata
				}
				break
			}
			if trovato < 0 {
				break
			}
			n := b[trovato]
			ultimoX = n.X
			usato[trovato] = true
			ultimaW = n.W
			x1, y1 := max(cur.X+cur.W, n.X+n.W), n.Y+n.H
			cur.X = min(cur.X, n.X)
			cur.W, cur.H = x1-cur.X, y1-cur.Y
			if strings.HasSuffix(cur.Testo, "-") {
				cur.Testo = strings.TrimSuffix(cur.Testo, "-") + n.Testo
			} else {
				cur.Testo += " " + n.Testo
			}
			unite := append(append([]ParolaOCR(nil), cur.parole()...), n.parole()...)
			cur.Pos = &unite
			cur.Righe++
		}
		if centro {
			cur.Centro = true // la traduzione resta centrata come il sottotitolo
		}
		out = append(out, cur)
	}
	return out
}

// altezzeSimili: la riga n ha la grandezza della riga sopra (alta hRiga). Le parole di Windows
// sono strette sulle lettere: "here." o "area." (senza gambe sopra o sotto) sono molto più basse
// di una riga con "probably", ma la grandezza del carattere (Em, stimata dalla forma delle
// lettere) è la stessa.
func altezzeSimili(cur, n Blocco, hRiga int) bool {
	if float64(n.H) <= 1.4*float64(hRiga) && float64(n.H) >= float64(hRiga)/1.4 {
		return true
	}
	return cur.Em > 0 && n.Em > 0 && math.Max(cur.Em, n.Em) <= 1.3*math.Min(cur.Em, n.Em)
}

// righeStessoSottotitolo: due righe centrate una sotto l'altra sono lo stesso sottotitolo (una frase
// che va a capo): almeno una delle due è una frase lunga (4+ parole, con minuscole), la seconda non
// comincia una voce nuova ("MUIR: …", "Objective: …") e non sono voci di un menu centrato.
func righeStessoSottotitolo(sopra, sotto string) bool {
	if tuttoMaiuscolo(sopra) || tuttoMaiuscolo(sotto) {
		return false
	}
	ws, wt := strings.Fields(sopra), strings.Fields(sotto)
	if len(ws) < 4 && len(wt) < 4 {
		return false
	}
	if !strings.ContainsFunc(sopra+sotto, unicode.IsLower) {
		return false
	}
	// una nuova frase dello stesso sottotitolo ("The basement can be…") resta unita; un altro
	// personaggio che parla ("MUIR: …") no
	c := campiFrase(strings.TrimSpace(sotto))
	return !(len(c) > 1 && (c[0].dueDopo || etichettaIniziale(c) > 0))
}

// rigaAndataACapo: testo lungo andato a capo ("...on the first day of" / "Each month."): la riga
// sopra è piena (larga almeno quanto quella sotto) e non finisce con un punto. Un titolo corto
// ("Monthly Seasons") sopra una riga lunga resta separato.
func rigaAndataACapo(sopra string, larghSopra int, sotto Blocco) bool {
	sopra = strings.TrimSpace(sopra)
	if sopra == "" || strings.ContainsAny(sopra[len(sopra)-1:], ".!?:;") {
		return false
	}
	// "…to track" / "Secrets. Look for…": la riga sotto comincia con una parola sola chiusa dal
	// punto o dalla virgola: è la fine della frase di sopra, non una voce nuova
	primaChiusa := false
	if w := strings.Fields(sotto.Testo); len(w) > 1 && (strings.HasSuffix(w[0], ".") || strings.HasSuffix(w[0], ",")) && len([]rune(w[0])) > 3 {
		primaChiusa = true
	}
	if c := campiFrase(sotto.Testo); !primaChiusa && inizioNuovaVoce(sotto.Testo) && !(len(c) > 0 && !c[0].dueDopo && etichettaIniziale(c) == 0 && finisceSospesa(sopra)) {
		return false
	}
	if len(strings.Fields(sopra)) < 5 {
		return false
	}
	return float64(larghSopra) >= 0.75*float64(sotto.W)
}

// paroleSospese: parole con cui una frase inglese non finisce mai ("...on the first day of").
var paroleSospese = map[string]bool{"of": true, "the": true, "a": true, "an": true, "to": true, "in": true,
	"on": true, "at": true, "for": true, "with": true, "and": true, "or": true, "but": true, "from": true,
	"by": true, "your": true, "my": true, "our": true, "their": true, "his": true, "her": true, "its": true,
	"is": true, "are": true, "was": true, "were": true, "be": true, "that": true, "than": true, "into": true}

// finisceSospesa: la riga finisce con una parola che vuole il seguito: la riga sotto lo è anche
// se comincia con la maiuscola ("...on the first day of" / "Each month.").
func finisceSospesa(s string) bool {
	w := strings.Fields(strings.ToLower(strings.TrimSpace(s)))
	return len(w) > 0 && paroleSospese[w[len(w)-1]]
}

// inizioNuovaVoce: la riga comincia una scritta nuova, non continua quella sopra: un'etichetta
// con i due punti ("Objective:", "TIP:", "MARCUS:") o una parola comune con la maiuscola
// ("Something is moving...", "Press E to..."). Nelle liste fitte (missioni, pixel art) le
// righe sono vicine come quelle di un paragrafo: unirle farebbe una frase sola senza senso.
// Un nome proprio ("Marcus at the dock.") o "I" possono invece continuare la frase sopra.
func inizioNuovaVoce(s string) bool {
	c := campiFrase(strings.TrimSpace(s))
	if len(c) > 1 && (c[0].dueDopo || etichettaIniziale(c) > 0) {
		return true // (anche minuscola: l'OCR perde a volte la prima lettera, "elective:")
	}
	if len(c) == 0 || !c[0].maiuscolaIn {
		return false
	}
	w := strings.ToLower(strings.Trim(c[0].w, "'"))
	if len([]rune(w)) < 2 || w == "i" || strings.HasPrefix(w, "i'") {
		return false
	}
	l, ok := vocabolario()[w]
	return ok && l <= 35
}

// separaAttaccate: con i caratteri stretti l'OCR attacca le parole ("thetime remaining"). Una
// parola che non esiste ma si divide in due o tre parole vere viene separata.
func separaAttaccate(s string) string {
	campi := strings.Fields(s)
	cambiato := false
	for i, c := range campi {
		nucleo := strings.TrimFunc(c, func(r rune) bool { return !unicode.IsLetter(r) && r != '\'' })
		if len(nucleo) < 5 || esiste(nucleo) || termGiochi[nucleo] != "" || strings.ToUpper(nucleo) == nucleo || !unicode.IsLower([]rune(nucleo)[0]) {
			continue // i nomi ("Palworld") non si toccano
		}
		if parti := dividiParola(nucleo, 3); parti != nil {
			campi[i] = strings.Replace(c, nucleo, strings.Join(parti, " "), 1)
			cambiato = true
		} else if parti := dividiConfusa(nucleo); parti != nil {
			campi[i] = strings.Replace(c, nucleo, strings.Join(parti, " "), 1)
			cambiato = true
		}
	}
	if !cambiato {
		return s
	}
	return strings.Join(campi, " ")
}

// confusioniAttaccate: lettere che l'OCR scambia quando attacca due parole ("trusthifn" =
// "trust him": "m" letta "fn"; "rn" per "m", "n" per "m" e viceversa, "li" per "h"...).
var confusioniAttaccate = [][2]string{{"fn", "m"}, {"rn", "m"}, {"in", "m"}, {"ni", "m"}, {"n", "m"}, {"m", "n"},
	{"m", "rn"}, {"ri", "n"}, {"li", "h"}, {"lt", "k"}, {"cl", "d"}, {"vv", "w"}, {"f", "t"}, {"t", "f"},
	{"l", "i"}, {"i", "l"}, {"c", "e"}, {"e", "c"}, {"h", "b"}, {"b", "h"}, {"u", "n"}, {"n", "u"}, {"o", "a"}, {"a", "o"}}

// dividiConfusa: una parola inventata che, cambiando UNA confusione tipica dell'OCR, si divide in
// due o tre parole vere e comunissime ("trusthifn" -> "trust him"). Le parti poco comuni non
// contano (troppo facile trovare parole rare in qualsiasi spazzatura: "baser" non diventa "has err").
func dividiConfusa(w string) []string {
	if !isLower(w) || len(w) < 6 || len(w) > 24 {
		return nil
	}
	// è UNA parola letta male ("baserr" = "baser" con una lettera doppia, "waitinq"): non si divide
	if len(candidati(w)) > 0 {
		return nil
	}
	for i := 1; i < len(w); i++ {
		if w[i] == w[i-1] && esiste(w[:i]+w[i+1:]) {
			return nil
		}
	}
	var meglio []string
	for _, c := range confusioniAttaccate {
		for i := 0; ; {
			j := strings.Index(w[i:], c[0])
			if j < 0 {
				break
			}
			k := i + j
			v := w[:k] + c[1] + w[k+len(c[0]):]
			i = k + 1
			if esiste(v) {
				continue // è una parola sola corretta: ci pensa correggiParole
			}
			if parolaValidaOCR(v) || paroleUgualiIt[v] {
				return []string{v} // parola dei giochi letta male ("rnedkit" -> "medkit"): resta una
			}
			parti := dividiParola(v, 3)
			if len(parti) < 2 || (meglio != nil && len(parti) >= len(meglio)) {
				continue
			}
			buone := true
			for _, p := range parti {
				if parolaRara(p) || (len(p) < 3 && !paroleCorteVere[p]) {
					buone = false
					break
				}
				if l, ok := vocabolario()[p]; ok && l > 20 {
					buone = false // parola poco comune: non mi fido
					break
				}
			}
			if buone {
				meglio = parti
			}
		}
	}
	return meglio
}

var paroleCorteVere = map[string]bool{"a": true, "i": true, "of": true, "to": true, "in": true, "on": true, "is": true, "be": true, "by": true, "at": true, "an": true, "or": true, "as": true, "it": true, "we": true, "no": true, "up": true, "my": true, "me": true, "do": true, "so": true, "if": true}

func dividiParola(w string, maxParti int) []string {
	if maxParti <= 0 {
		return nil
	}
	if (len(w) >= 3 && esiste(w)) || paroleCorteVere[strings.ToLower(w)] {
		return []string{w}
	}
	if maxParti == 1 {
		return nil
	}
	// prima le divisioni con il primo pezzo più lungo: "thetime" -> "the time", non "th etime"
	for k := len(w) - 1; k >= 1; k-- {
		a := w[:k]
		if !((len(a) >= 3 && esiste(a)) || paroleCorteVere[strings.ToLower(a)]) {
			continue
		}
		if resto := dividiParola(w[k:], maxParti-1); resto != nil {
			return append([]string{a}, resto...)
		}
	}
	return nil
}

func continuaFrase(sopra, sotto string) bool {
	sotto = strings.TrimSpace(sotto)
	if sotto == "" {
		return false
	}
	if strings.HasPrefix(sotto, "-") || strings.HasPrefix(sotto, "•") {
		return false
	}
	// titolo tutto maiuscolo sopra una riga normale: sono due scritte diverse anche se l'OCR ha
	// perso la maiuscola della seconda ("CRAFT MORE" / "reate additional...", No Man's Sky)
	if tuttoMaiuscolo(sopra) && !tuttoMaiuscolo(sotto) {
		return false
	}
	if strings.HasSuffix(sopra, ",") || strings.HasSuffix(sopra, "-") {
		return true
	}
	// la riga sotto comincia con un'etichetta ("Objective: ...", anche letta male "elective:"):
	// è una voce nuova della lista, non il seguito della frase
	if c := campiFrase(sotto); len(c) > 1 && c[0].dueDopo {
		return false
	}
	return unicode.IsLower([]rune(sotto)[0])
}

// tuttoMaiuscolo: almeno due lettere e nessuna minuscola.
func tuttoMaiuscolo(s string) bool {
	n := 0
	for _, r := range s {
		if unicode.IsLower(r) {
			return false
		}
		if unicode.IsLetter(r) {
			n++
		}
	}
	return n >= 2
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// coloriBlocco: lo sfondo è il colore più comune sul bordo del blocco; l'inchiostro è il colore
// interno più lontano dallo sfondo (il testo del gioco). Così copro l'inglese con il suo sfondo.
func coloriBlocco(img *image.RGBA, b *Blocco) {
	r := image.Rect(b.X-3, b.Y-3, b.X+b.W+3, b.Y+b.H+3).Intersect(img.Bounds())
	if r.Empty() {
		b.Sfondo, b.Inchiostro = color.RGBA{10, 12, 18, 255}, color.RGBA{240, 240, 240, 255}
		return
	}
	conta := map[[3]uint8]int{}
	aggiungi := func(x, y int) {
		c := img.RGBAAt(x, y)
		conta[[3]uint8{c.R >> 3, c.G >> 3, c.B >> 3}]++
	}
	for x := r.Min.X; x < r.Max.X; x++ {
		aggiungi(x, r.Min.Y)
		aggiungi(x, r.Max.Y-1)
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		aggiungi(r.Min.X, y)
		aggiungi(r.Max.X-1, y)
	}
	var meglio [3]uint8
	n := -1
	for k, v := range conta {
		if v > n {
			meglio, n = k, v
		}
	}
	// media dei pixel del bordo vicini al colore più comune
	var sr, sg, sb, cnt int
	for x := r.Min.X; x < r.Max.X; x++ {
		for _, y := range []int{r.Min.Y, r.Max.Y - 1} {
			c := img.RGBAAt(x, y)
			if [3]uint8{c.R >> 3, c.G >> 3, c.B >> 3} == meglio {
				sr, sg, sb, cnt = sr+int(c.R), sg+int(c.G), sb+int(c.B), cnt+1
			}
		}
	}
	if cnt == 0 {
		cnt, sr, sg, sb = 1, int(meglio[0])<<3, int(meglio[1])<<3, int(meglio[2])<<3
	}
	b.Sfondo = color.RGBA{uint8(sr / cnt), uint8(sg / cnt), uint8(sb / cnt), 255}
	// inchiostro: tra i pixel interni, i più distanti dallo sfondo (10% più distante)
	type pd struct {
		c color.RGBA
		d int
	}
	var px []pd
	in := image.Rect(b.X, b.Y, b.X+b.W, b.Y+b.H).Intersect(img.Bounds())
	passo := max(1, in.Dx()*in.Dy()/4000)
	i := 0
	for y := in.Min.Y; y < in.Max.Y; y++ {
		for x := in.Min.X; x < in.Max.X; x++ {
			i++
			if i%passo != 0 {
				continue
			}
			c := img.RGBAAt(x, y)
			px = append(px, pd{c, distanza(c, b.Sfondo)})
		}
	}
	if len(px) == 0 {
		b.Inchiostro = contrastante(b.Sfondo)
		return
	}
	sort.Slice(px, func(i, j int) bool { return px[i].d > px[j].d })
	k := max(1, len(px)/10)
	var ir, ig, ib int
	for _, p := range px[:k] {
		ir, ig, ib = ir+int(p.c.R), ig+int(p.c.G), ib+int(p.c.B)
	}
	b.Inchiostro = color.RGBA{uint8(ir / k), uint8(ig / k), uint8(ib / k), 255}
	riempimentoEContorno(img, in, b)
	secondoInchiostro(img, in, b)
	b.Tratto = spessoreTratto(img, in, b.Inchiostro, b.Sfondo, b.Em)
	if !b.HaContorno && distanza(b.Inchiostro, b.Sfondo) < 120 { // troppo simile: leggibilità prima di tutto
		b.Inchiostro = contrastante(b.Sfondo)
	}
}

func distanza(a, b color.RGBA) int {
	return abs(int(a.R)-int(b.R)) + abs(int(a.G)-int(b.G)) + abs(int(a.B)-int(b.B))
}

func contrastante(c color.RGBA) color.RGBA {
	if int(c.R)*299+int(c.G)*587+int(c.B)*114 > 128000 {
		return color.RGBA{15, 15, 15, 255}
	}
	return color.RGBA{245, 245, 245, 255}
}

// daTradurre: salto numeri, sigle, simboli e codici ("47.5 KM/h", "185/425", "α").
func daTradurre(s string) bool {
	lettere := 0
	for _, c := range s {
		if unicode.IsLetter(c) && c < 0x370 {
			lettere++
		}
	}
	if lettere < 2 || !sembraTesto(s) {
		return false
	}
	// "S REPORT": l'OCR ha preso solo la coda di una parola tagliata; meglio lasciare l'originale
	if w := strings.Fields(s); len(w) > 1 && len([]rune(w[0])) == 1 && unicode.IsUpper([]rune(w[0])[0]) &&
		w[0] != "A" && w[0] != "I" && strings.ToUpper(s) == s {
		return false
	}
	// nome del giocatore e piattaforma ("1LMESS1A (STEAM)"): un nome non si traduce
	if nomeGiocatore(s) {
		return false
	}
	if _, ok := fraseMenu(s); ok { // carica anche il glossario da gioco
		return true
	}
	// almeno una parola inglese vera di 3+ lettere: scarta i pezzi di grafica letti come lettere
	for _, w := range strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && r != '\'' }) {
		if len(w) >= 3 && (esiste(w) || termGiochi[w] != "") {
			return true
		}
	}
	return false
}

// testoConNumeri: scritta con parole e numeri ("Ammo: 24 / 120", "Wave 3 of 10"): senza contare
// cifre e spazi, quasi tutto è lettere, e c'è una parola di almeno 3 lettere.
func testoConNumeri(s string) bool {
	lettere, segni := 0, 0
	for _, c := range s {
		switch {
		case unicode.IsLetter(c):
			lettere++
		case unicode.IsDigit(c) || unicode.IsSpace(c):
		default:
			segni++
		}
	}
	if lettere < 3 || lettere*100 < (lettere+segni)*60 {
		return false
	}
	for _, w := range strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) }) {
		if len([]rune(w)) >= 3 {
			return true
		}
	}
	return false
}

// formaPlausibile: una riga di testo è più larga che alta (scarta colonne di simboli).
func formaPlausibile(b Blocco) bool {
	righe := max(1, b.Righe)
	return b.H/righe <= 2*b.W
}

// fondoRicostruito: ricostruisce lo sfondo sotto la scritta sfumando, colonna per colonna, tra la
// riga di pixel appena sopra e quella appena sotto. Così sopra una sfumatura o un pannello
// colorato la toppa non si vede quasi (meglio di un rettangolo pieno).
func fondoRicostruito(img *image.RGBA, b Blocco) *image.RGBA {
	const m = 3
	ml, mr := margineFondo(b)
	x0, y0 := b.X-ml, b.Y-m
	w, h := b.W+ml+mr, b.H+2*m
	if w <= 0 || h <= 0 {
		return nil
	}
	bd := img.Bounds()
	// un punto del bordo: media dei pixel vicini lungo il bordo, saltando quelli che sembrano
	// lettere. Media larga (13 pixel): niente strisce dovute a un solo pixel diverso.
	media := func(x, y, dx, dy int) color.RGBA {
		var r, g, bl, n int
		for k := -6; k <= 6; k++ {
			xx := clamp(x+k*dx, bd.Min.X, bd.Max.X-1)
			yy := clamp(y+k*dy, bd.Min.Y, bd.Max.Y-1)
			c := img.RGBAAt(xx, yy)
			if distanza(c, b.Inchiostro) < distanza(c, b.Sfondo) {
				continue // pixel che sembra testo: non lo uso
			}
			r, g, bl, n = r+int(c.R), g+int(c.G), bl+int(c.B), n+1
		}
		if n == 0 {
			return b.Sfondo
		}
		return color.RGBA{uint8(r / n), uint8(g / n), uint8(bl / n), 255}
	}
	yTop, yBot := clamp(y0-1, bd.Min.Y, bd.Max.Y-1), clamp(y0+h, bd.Min.Y, bd.Max.Y-1)
	xSx, xDx := clamp(x0-1, bd.Min.X, bd.Max.X-1), clamp(x0+w, bd.Min.X, bd.Max.X-1)
	sopra, sotto := make([]color.RGBA, w), make([]color.RGBA, w)
	for i := 0; i < w; i++ {
		sopra[i], sotto[i] = media(x0+i, yTop, 1, 0), media(x0+i, yBot, 1, 0)
	}
	sx, dx := make([]color.RGBA, h), make([]color.RGBA, h)
	for j := 0; j < h; j++ {
		sx[j], dx[j] = media(xSx, y0+j, 0, 1), media(xDx, y0+j, 0, 1)
	}
	// superficie che unisce i quattro bordi (sopra, sotto, sinistra, destra): le sfumature in
	// tutte le direzioni, senza le strisce verticali di prima
	ang := [4][3]float64{}
	for k, c := range []color.RGBA{sopra[0], sopra[w-1], sotto[0], sotto[w-1]} {
		ang[k] = [3]float64{float64(c.R), float64(c.G), float64(c.B)}
	}
	p := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		v := float64(y) / float64(max(1, h-1))
		for x := 0; x < w; x++ {
			u := float64(x) / float64(max(1, w-1))
			t, bo, l, r := sopra[x], sotto[x], sx[y], dx[y]
			var out [3]float64
			for c := 0; c < 3; c++ {
				ct := float64([]uint8{t.R, t.G, t.B}[c])
				cb := float64([]uint8{bo.R, bo.G, bo.B}[c])
				cl := float64([]uint8{l.R, l.G, l.B}[c])
				cr := float64([]uint8{r.R, r.G, r.B}[c])
				out[c] = (1-v)*ct + v*cb + (1-u)*cl + u*cr -
					((1-u)*(1-v)*ang[0][c] + u*(1-v)*ang[1][c] + (1-u)*v*ang[2][c] + u*v*ang[3][c])
				out[c] = math.Max(0, math.Min(255, out[c]))
			}
			p.SetRGBA(x, y, color.RGBA{uint8(out[0] + 0.5), uint8(out[1] + 0.5), uint8(out[2] + 0.5), 255})
		}
	}
	return p
}

// margineFondo: quanto sfondo ricostruire a sinistra e a destra della scritta inglese. La
// traduzione italiana è spesso più lunga: per una scritta centrata cresce dai due lati, per una
// allineata a sinistra verso destra (fino alla scritta vicina, LargMax).
func margineFondo(b Blocco) (int, int) {
	ml, mr := 3, 3
	if b.Centro {
		e := min(200, b.W/2+20)
		ml, mr = ml+e, mr+e
	} else if b.LargMax > b.W {
		mr += min(300, b.LargMax-b.W)
	}
	if b.IconaDx > b.X && b.SpazioSx > 0 { // con l'icona a destra la traduzione cresce a sinistra
		ml += min(300, b.SpazioSx)
	}
	return ml, mr
}

var (
	coloreFascia      = color.RGBA{12, 12, 14, 255}
	coloreTestoFascia = color.RGBA{248, 248, 248, 255}
)

// sfondoInMovimento: il bordo attorno alla scritta è cambiato tra due catture a 250 ms
// (un video sotto ai sottotitoli). Le scritte dei menu invece hanno lo sfondo fermo.
func sfondoInMovimento(a, b *image.RGBA, bl Blocco) bool {
	if a == nil || b == nil || a.Bounds() != b.Bounds() {
		return false
	}
	r := image.Rect(bl.X-4, bl.Y-4, bl.X+bl.W+4, bl.Y+bl.H+4).Intersect(a.Bounds())
	if r.Empty() {
		return false
	}
	tot, n := 0, 0
	for x := r.Min.X; x < r.Max.X; x += 2 {
		for _, y := range []int{r.Min.Y, r.Max.Y - 1} {
			tot += distanza(a.RGBAAt(x, y), b.RGBAAt(x, y))
			n++
		}
	}
	return n > 0 && tot/n > 24
}

// armonizza: le scritte della stessa colonna (stessa x, grandezza simile) prendono la stessa
// grandezza e lo stesso colore: una lista di menu resta ordinata come nel gioco.
func armonizza(b []Blocco) {
	gruppo := make([]int, len(b))
	for i := range gruppo {
		gruppo[i] = -1
	}
	g := 0
	for i := range b {
		if gruppo[i] >= 0 || b[i].Em <= 0 {
			continue
		}
		gruppo[i] = g
		for j := i + 1; j < len(b); j++ {
			if gruppo[j] < 0 && b[j].Em > 0 && abs(b[j].X-b[i].X) <= 8 &&
				b[j].Em/b[i].Em < 1.35 && b[i].Em/b[j].Em < 1.35 && abs(b[j].Y-b[i].Y) < 20*int(b[i].Em) {
				gruppo[j] = g
			}
		}
		g++
	}
	// menu centrati (molti giochi): stesso centro ma inizio diverso. Le metto insieme lo stesso
	// e la traduzione resta centrata sotto il centro dell'originale.
	for i := range b {
		if b[i].Em <= 0 {
			continue
		}
		ci := b[i].X + b[i].W/2
		for j := range b {
			if j == i || b[j].Em <= 0 || (gruppo[i] == gruppo[j] && gruppo[i] >= 0) {
				continue
			}
			cj := b[j].X + b[j].W/2
			// solo scritte corte (voci di menu): due frasi lunghe con lo stesso centro è un caso
			if float64(b[i].W) > 20*b[i].Em || float64(b[j].W) > 20*b[j].Em {
				continue
			}
			if abs(ci-cj) <= max(6, int(b[i].Em/2)) && abs(b[j].X-b[i].X) > 8 &&
				b[j].Em/b[i].Em < 1.35 && b[i].Em/b[j].Em < 1.35 && abs(b[j].Y-b[i].Y) < 6*int(b[i].Em+1) {
				b[i].Centro, b[j].Centro = true, true
				// stesso menu: unisco i due gruppi interi (non solo le voci rimaste sole)
				if gi, gj := gruppo[i], gruppo[j]; gi >= 0 && gj >= 0 && gi != gj {
					for k := range gruppo {
						if gruppo[k] == gj {
							gruppo[k] = gi
						}
					}
				}
			}
		}
	}
	for k := 0; k < g; k++ {
		// voci centrate: se metà del gruppo è centrata, lo è tutto il menu
		var cen, tot int
		for i := range b {
			if gruppo[i] == k {
				tot++
				if b[i].Centro {
					cen++
				}
			}
		}
		if cen >= 2 && cen*2 >= tot {
			for i := range b {
				if gruppo[i] == k {
					b[i].Centro = true
				}
			}
		}
		// colore letto al contrario (l'ombra scura presa per la lettera): se le altre voci sono
		// chiare sul loro fondo e questa è scura, prende il colore delle altre
		var chiare, scure []int
		for i := range b {
			if gruppo[i] != k {
				continue
			}
			if luce(b[i].Inchiostro) > luce(b[i].Sfondo) {
				chiare = append(chiare, i)
			} else {
				scure = append(scure, i)
			}
		}
		magg, mino := chiare, scure
		if len(scure) > len(chiare) {
			magg, mino = scure, chiare
		}
		if len(magg) >= 2 && len(magg) >= 2*len(mino) {
			for _, i := range mino {
				j := magg[0]
				b[i].Inchiostro = b[j].Inchiostro
				b[i].HaContorno, b[i].Contorno = b[j].HaContorno, b[j].Contorno
			}
		}
	}
	for k := 0; k < g; k++ {
		// ombra e bordino uguali per tutte le voci: o tutte o nessuna (decide la maggioranza)
		var tot, om, co int
		var omC, coC color.RGBA
		for i := range b {
			if gruppo[i] != k {
				continue
			}
			tot++
			if b[i].HaOmbraG {
				om++
				omC = b[i].OmbraGioco
			}
			if b[i].HaContorno {
				co++
				coC = b[i].Contorno
			}
		}
		if tot >= 2 {
			for i := range b {
				if gruppo[i] != k {
					continue
				}
				b[i].HaOmbraG = om*2 > tot
				if b[i].HaOmbraG && b[i].OmbraGioco.A == 0 {
					b[i].OmbraGioco = omC
				}
				b[i].HaContorno = co*2 > tot
				if b[i].HaContorno && b[i].Contorno.A == 0 {
					b[i].Contorno = coC
				}
			}
		}
	}
	for k := 0; k < g; k++ {
		var em, tr []float64
		var rr, gg, bb []int
		for i := range b {
			if gruppo[i] == k {
				em = append(em, b[i].Em)
				tr = append(tr, b[i].Tratto)
				rr, gg, bb = append(rr, int(b[i].Inchiostro.R)), append(gg, int(b[i].Inchiostro.G)), append(bb, int(b[i].Inchiostro.B))
			}
		}
		if len(em) < 2 {
			continue
		}
		sort.Float64s(em)
		sort.Float64s(tr)
		sort.Ints(rr)
		sort.Ints(gg)
		sort.Ints(bb)
		m := len(em) / 2
		for i := range b {
			if gruppo[i] == k {
				b[i].Em = em[m]
				b[i].Tratto = tr[m] // stesso spessore: niente voci in neretto e altre no
				med := color.RGBA{uint8(rr[m]), uint8(gg[m]), uint8(bb[m]), 255}
				// solo se il colore letto è vicino a quello della colonna: una scritta
				// colorata apposta (verde, rossa…) resta del suo colore
				if b[i].Inchiostro != coloreTestoFascia && distanza(b[i].Inchiostro, med) < 50 {
					b[i].Inchiostro = med
				}
			}
		}
	}
}

// dentro: il centro del blocco cade nella zona z.
func dentro(b Blocco, z Zona) bool {
	cx, cy := b.X+b.W/2, b.Y+b.H/2
	return cx >= z.X && cx < z.X+z.W && cy >= z.Y && cy < z.Y+z.H
}

// ricolora: il gioco ha cambiato colore sotto qualche scritta (pulsante evidenziato dal
// mouse, voce selezionata): rifaccio sfondo e colore solo di quelle, senza rileggere né
// ritradurre. Così l'effetto del pulsante si vede anche sulla traduzione.
func ricolora(img *image.RGBA, bl []Blocco) ([]Blocco, bool) { return ricoloraSolo(img, bl, nil) }

// ricoloraSolo: come ricolora, ma guarda solo le scritte segnate in solo (nil = tutte): quelle
// la cui tinta è cambiata (blocchiDaRicolorare). Le altre non si ricalcolano.
func ricoloraSolo(img *image.RGBA, bl []Blocco, solo []bool) ([]Blocco, bool) {
	out := make([]Blocco, len(bl))
	copy(out, bl)
	cambiati := make([]bool, len(out))
	quanti := 0
	for i, b := range out {
		if solo != nil && (i >= len(solo) || !solo[i]) {
			continue
		}
		if b.Inchiostro == coloreTestoFascia && b.Sfondo == coloreFascia {
			continue // fascia dei video: resta com'è
		}
		c := b
		coloriBlocco(img, &c)
		mossa := b.Img != nil && b.Img.Bounds() == img.Bounds() && zonaCambiata(b.Img, img, b)
		if !mossa && distanza(c.Sfondo, b.Sfondo) <= 40 && distanza(c.Inchiostro, b.Inchiostro) <= 90 {
			continue
		}
		// la voce è cambiata nel gioco (mouse sopra, selezionata): rileggo colori, bordino e
		// ombra da com'è adesso, così la traduzione reagisce come l'originale
		riquadraDiNuovo(img, &c) // la voce può ingrandirsi (Palworld): stessa grandezza nuova
		coloriBlocco(img, &c)
		c.HaContorno, c.HaOmbraG = false, false
		contornoLettere(img, &c)
		ombraLettere(img, &c)
		c.Fondo = fondoRicostruito(img, c)
		c.Tinta = tintaBlocco(img, c)
		out[i] = c
		cambiati[i] = true
		quanti++
	}
	if quanti == 0 {
		return out, false
	}
	// nuova pagina pulita sull'immagine nuova: le voci cambiate si cancellano da capo; delle
	// altre copio solo i pixel già cancellati (le loro lettere inglesi) dalla loro pagina pulita,
	// senza rifare i conti. Tutte puntano poi alle due immagini nuove: le vecchie si liberano
	pul := image.NewRGBA(img.Bounds())
	copy(pul.Pix, img.Pix)
	var daRifare []Blocco
	for i, b := range out {
		if cambiati[i] || b.Pulita == nil || b.Img == nil || b.Img.Bounds() != img.Bounds() {
			daRifare = append(daRifare, b)
			continue
		}
		copiaCancellati(pul, b)
	}
	cancellaInglese(pul, img, daRifare)
	for i := range out {
		out[i].Img, out[i].Pulita = img, pul
	}
	return out, true
}

// copiaCancellati: nella pagina pulita pul metto i pixel che la pagina pulita di b aveva
// cancellato (dove era diversa dalla sua cattura), solo attorno a b.
func copiaCancellati(pul *image.RGBA, b Blocco) {
	if b.Pulita == nil || b.Img == nil {
		return
	}
	m := max(b.Marg, int(b.Em*0.15+0.5), 2) + 3
	r := image.Rect(b.X-m, b.Y-m, b.X+b.W+m, b.Y+b.H+m).Intersect(pul.Bounds()).Intersect(b.Pulita.Bounds()).Intersect(b.Img.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if c := b.Pulita.RGBAAt(x, y); c != b.Img.RGBAAt(x, y) {
				pul.SetRGBA(x, y, c)
			}
		}
	}
}

// tintaBlocco: il colore medio di 4 pezzi (da sinistra a destra) del riquadro della scritta,
// guardando un pixel su quattro. Costa poco: si può fare per ogni scritta a ogni giro. Se il
// gioco ricolora la voce (mouse sopra: diventa dorata) la tinta cambia anche se il resto dello
// schermo è identico.
func tintaBlocco(img *image.RGBA, b Blocco) [12]uint8 {
	var t [12]uint8
	r := image.Rect(b.X, b.Y, b.X+b.W, b.Y+b.H).Intersect(img.Bounds())
	if r.Dx() < 4 || r.Dy() < 2 {
		return t
	}
	for k := 0; k < 4; k++ {
		x0, x1 := r.Min.X+r.Dx()*k/4, r.Min.X+r.Dx()*(k+1)/4
		var sr, sg, sb, n int
		for y := r.Min.Y; y < r.Max.Y; y += 2 {
			for x := x0; x < x1; x += 2 {
				c := img.RGBAAt(x, y)
				sr, sg, sb, n = sr+int(c.R), sg+int(c.G), sb+int(c.B), n+1
			}
		}
		if n > 0 {
			t[k*3], t[k*3+1], t[k*3+2] = uint8(sr/n), uint8(sg/n), uint8(sb/n)
		}
	}
	return t
}

// tintaCambiata: almeno un pezzo ha cambiato colore più della soglia (su un canale).
func tintaCambiata(a, b [12]uint8, soglia int) bool {
	for i := range a {
		if abs(int(a[i])-int(b[i])) > soglia {
			return true
		}
	}
	return false
}

// blocchiDaRicolorare: le scritte mostrate la cui tinta è cambiata nell'immagine nuova, da sole,
// qualunque cosa faccia il resto dello schermo (prima contava solo l'impronta di tutto lo
// schermo, e un pulsante che diventa giallo non bastava a superarla). viste ricorda l'ultima
// tinta già controllata di ogni scritta: una scritta sopra uno sfondo che si muove non viene
// ricontrollata a ogni giro, solo quando cambia di nuovo.
func blocchiDaRicolorare(img *image.RGBA, bl []Blocco, viste map[[4]int][12]uint8) ([]bool, int) {
	solo := make([]bool, len(bl))
	n := 0
	if len(viste) > 1000 {
		clear(viste)
	}
	for i, b := range bl {
		if b.Img == nil || b.Img.Bounds() != img.Bounds() || (b.Inchiostro == coloreTestoFascia && b.Sfondo == coloreFascia) {
			continue
		}
		t := tintaBlocco(img, b)
		k := [4]int{b.X, b.Y, b.W, b.H}
		rif, ok := viste[k]
		if !ok {
			if b.Tinta == ([12]uint8{}) { // tinta mai calcolata: questa è il riferimento
				viste[k] = t
				continue
			}
			rif = b.Tinta
		}
		soglia := 6
		if b.Ombra { // sfondo che si muove: solo un cambio forte
			soglia = 24
		}
		if tintaCambiata(t, rif, soglia) {
			solo[i] = true
			n++
			viste[k] = t
		}
	}
	return solo, n
}

// togliCambiate: le scritte sotto cui l'inglese è davvero cambiato (altre lettere, non solo un
// altro colore) spariscono subito, appena lo vedo: la traduzione vecchia non resta sopra un
// testo diverso mentre leggo quello nuovo. Restituisce le scritte che restano e quante ne ha tolte.
//
// Tolgo subito solo se la scritta è scorsa o se la sagoma delle lettere è cambiata davvero.
// Il bagliore o il piccolo ingrandimento del pulsante sotto il mouse (Palworld) cambia i pixel
// ma non la sagoma: quella scritta resta "da verificare" e decide la rilettura che parte subito
// dopo (tieniFerme tiene la traduzione solo se il testo letto è lo stesso). Prima l'italiano
// spariva per 200-600 ms a ogni passaggio del mouse: sfarfallio.
func togliCambiate(img *image.RGBA, bl []Blocco) ([]Blocco, int) {
	var resto []Blocco
	tolte := 0
	for _, b := range bl {
		if b.Img != nil && !(b.Inchiostro == coloreTestoFascia && b.Sfondo == coloreFascia) {
			if b.Img.Bounds() != img.Bounds() {
				tolte++
				continue
			}
			if zonaCambiata(b.Img, img, b) && inchiostroCambiato(b.Img, img, b) &&
				(spostamentoScritta(b.Img, img, b) != 0 || formaCambiata(b.Img, img, b)) {
				tolte++
				continue
			}
		}
		resto = append(resto, b)
	}
	return resto, tolte
}

// formaCambiata: la sagoma delle lettere di bl è diversa tra a e b, con una tolleranza di 1-2
// pixel. Per ogni immagine separo l'inchiostro dallo sfondo con i suoi colori (così un cambio di
// colore non conta) e cerco le lettere di un'immagine nell'altra, allargate di 1 pixel e
// spostate fino a 2 pixel: se una buona parte non si ritrova, la scritta è un'altra. Conta solo
// l'inchiostro pieno: l'alone di un bagliore attorno alle lettere non è una lettera nuova.
func formaCambiata(a, b *image.RGBA, bl Blocco) bool {
	return persiForma(a, b, bl) > sogliaForma
}

// sogliaForma: oltre questa parte di lettere che non si ritrovano, la scritta è un'altra.
const sogliaForma = 0.2

// persiForma: quanta parte delle lettere non si ritrova tra a e b (0 = tutte, 1 = nessuna), nel
// verso peggiore (lettere sparite o lettere nuove).
func persiForma(a, b *image.RGBA, bl Blocco) float64 {
	r := image.Rect(bl.X, bl.Y, bl.X+bl.W, bl.Y+bl.H).Intersect(a.Bounds()).Intersect(b.Bounds())
	w, h := r.Dx(), r.Dy()
	if w < 4 || h < 4 {
		return 0
	}
	// piena: inchiostro vero (vicino al colore delle lettere); larga: tutto quello che somiglia
	// più all'inchiostro che allo sfondo (bordi sfumati, alone), allargato di 1 pixel a croce
	maschere := func(img *image.RGBA) (piena, larga []bool) {
		c := bl
		coloriBlocco(img, &c)
		contrasto := distanza(c.Inchiostro, c.Sfondo)
		if contrasto < 30 { // niente contrasto: non si capisce la forma
			return nil, nil
		}
		piena, simile := make([]bool, w*h), make([]bool, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				p := img.RGBAAt(r.Min.X+x, r.Min.Y+y)
				di := distanza(p, c.Inchiostro)
				simile[y*w+x] = di < distanza(p, c.Sfondo)
				piena[y*w+x] = di*3 < contrasto
			}
		}
		larga = make([]bool, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i := y*w + x
				larga[i] = simile[i] || (x > 0 && simile[i-1]) || (x+1 < w && simile[i+1]) ||
					(y > 0 && simile[i-w]) || (y+1 < h && simile[i+w])
			}
		}
		return piena, larga
	}
	pa, la := maschere(a)
	pb, lb := maschere(b)
	if pa == nil || pb == nil {
		// prima c'erano lettere leggibili e adesso no (o il contrario): via. Se non si leggono
		// in nessuna delle due decide la rilettura
		if (pa == nil) != (pb == nil) {
			return 1
		}
		return 0
	}
	// persi: parte dell'inchiostro pieno di m che non cade nella maschera larga l. La scritta è
	// divisa in pezzi larghi circa quanto è alta, e ogni pezzo prova lo spostamento che va meglio
	// (fino a 2 pixel in orizzontale e 1 in verticale): così anche un pulsante ingrandito di
	// qualche percento attorno al centro (le lettere ai lati si spostano un po' di più) si ritrova
	persi := func(m, l []bool) float64 {
		ink, tot := 0, 0
		pezzo := max(12, h)
		for x0 := 0; x0 < w; x0 += pezzo {
			x1 := min(w, x0+pezzo)
			inkP, meglio := 0, -1
			for dy := -1; dy <= 1; dy++ {
				for dx := -2; dx <= 2; dx++ {
					n, k := 0, 0
					for y := 0; y < h; y++ {
						y2 := y + dy
						for x := x0; x < x1; x++ {
							if !m[y*w+x] {
								continue
							}
							k++
							x2 := x + dx
							if x2 < 0 || x2 >= w || y2 < 0 || y2 >= h || !l[y2*w+x2] {
								n++
							}
						}
					}
					inkP = k
					if meglio < 0 || n < meglio {
						meglio = n
					}
				}
			}
			ink += inkP
			tot += max(meglio, 0)
		}
		if ink == 0 {
			return 0
		}
		return float64(tot) / float64(ink)
	}
	return max(persi(pa, lb), persi(pb, la))
}

// paginaSpostata: le scritte inglesi sotto le traduzioni sono SCORSE in su o in giù (rotella,
// elenco che scorre) su almeno 2 righe diverse, nello stesso verso. Cerco ogni scritta vecchia
// poco più su o poco più giù nell'immagine nuova: se la ritrovo lì, si è spostata.
// Il mouse sopra una voce, un riquadro che si illumina, una finestrella di descrizione o uno
// sfondo animato cambiano i colori ma non spostano le scritte: non contano (visto in No Man's
// Sky, inventario, 23/11: prima ogni passaggio del mouse toglieva e rileggeva tutto).
// Rotella vista in PC Cafe Simulator, Impostazioni > Audio, 23/11.
func paginaSpostata(img *image.RGBA, bl []Blocco) bool {
	var su, giu []int // centro verticale delle righe scorse
	conta := func(righe []int, c, h int) ([]int, bool) {
		for _, r := range righe {
			if d := r - c; d < max(h, 6) && -d < max(h, 6) {
				return righe, false
			}
		}
		return append(righe, c), true
	}
	for _, b := range bl {
		if b.Inchiostro == coloreTestoFascia && b.Sfondo == coloreFascia {
			continue
		}
		if b.Img == nil || b.Img.Bounds() != img.Bounds() || !zonaCambiata(b.Img, img, b) {
			continue
		}
		dy := spostamentoScritta(b.Img, img, b)
		if dy == 0 {
			continue
		}
		c := b.Y + b.H/2
		if dy < 0 {
			su, _ = conta(su, c, b.H)
		} else {
			giu, _ = conta(giu, c, b.H)
		}
		if len(su) >= 2 || len(giu) >= 2 {
			return true
		}
	}
	return false
}

// scenaCambiata: sotto più di metà delle traduzioni l'inglese non c'è più (schermata nuova).
// Una finestrella di descrizione che si apre col mouse cambia solo la sua parte: le altre
// traduzioni restano al loro posto senza sfarfallare (No Man's Sky, inventario, 23/11).
func scenaCambiata(img *image.RGBA, bl []Blocco) bool {
	tot, cambiate := 0, 0
	for _, b := range bl {
		if b.Inchiostro == coloreTestoFascia && b.Sfondo == coloreFascia {
			continue
		}
		if b.Img == nil || b.Img.Bounds() != img.Bounds() {
			return true
		}
		tot++
		if zonaCambiata(b.Img, img, b) {
			cambiate++
		}
	}
	return tot == 0 || cambiate*2 > tot
}

// spostamentoScritta: di quanti pixel in verticale la scritta b (presa da vecchia) si ritrova
// in nuova. 0 se non si ritrova (è cambiata sul posto: colore, evidenziazione, finestrella).
func spostamentoScritta(vecchia, nuova *image.RGBA, b Blocco) int {
	r := image.Rect(b.X, b.Y, b.X+b.W, b.Y+b.H).Intersect(vecchia.Bounds())
	if r.Dx() < 4 || r.Dy() < 4 {
		return 0
	}
	passo := max(2, r.Dy()/8)
	media := func(dy int) int {
		tot, n := 0, 0
		for y := r.Min.Y; y < r.Max.Y; y += passo {
			y2 := y + dy
			if y2 < nuova.Rect.Min.Y || y2 >= nuova.Rect.Max.Y {
				return 1 << 20
			}
			for x := r.Min.X; x < r.Max.X; x += 2 {
				tot += distanza(vecchia.RGBAAt(x, y), nuova.RGBAAt(x, y2))
				n++
			}
		}
		if n == 0 {
			return 1 << 20
		}
		return tot / n
	}
	// la scritta deve avere disegno (lettere), se no si "ritrova" ovunque su un fondo piatto
	piatta := true
	c0 := vecchia.RGBAAt(r.Min.X, r.Min.Y)
	for y := r.Min.Y; y < r.Max.Y && piatta; y += passo {
		for x := r.Min.X; x < r.Max.X; x += 2 {
			if distanza(vecchia.RGBAAt(x, y), c0) > 40 {
				piatta = false
				break
			}
		}
	}
	if piatta {
		return 0
	}
	d0 := media(0)
	meglio, dyM := 1<<20, 0
	lim := max(4*b.H, 24)
	for dy := -lim; dy <= lim; dy++ {
		if dy == 0 {
			continue
		}
		if m := media(dy); m < meglio {
			meglio, dyM = m, dy
		}
	}
	if meglio <= 12 && meglio*3 < d0 {
		return dyM
	}
	return 0
}

// zonaCambiata: le lettere inglesi di b sono cambiate tra le due immagini (colore, alone,
// riquadro di evidenziazione): differenza media sopra 18 nella zona della scritta.
// inchiostroCambiato: la FORMA delle lettere è cambiata ("On" → "Off", descrizione nuova).
// Col mouse sopra inchiostro e sfondo cambiano colore insieme ma la forma resta: niente rilettura.
func inchiostroCambiato(a, b *image.RGBA, bl Blocco) bool {
	ca, cb := bl, bl
	coloriBlocco(a, &ca)
	coloriBlocco(b, &cb)
	r := image.Rect(bl.X, bl.Y, bl.X+bl.W, bl.Y+bl.H).Intersect(a.Bounds()).Intersect(b.Bounds())
	diff, ink := 0, 0
	for y := r.Min.Y; y < r.Max.Y; y += 2 {
		for x := r.Min.X; x < r.Max.X; x += 2 {
			pa, pb := a.RGBAAt(x, y), b.RGBAAt(x, y)
			ia := distanza(pa, ca.Inchiostro) < distanza(pa, ca.Sfondo)
			ib := distanza(pb, cb.Inchiostro) < distanza(pb, cb.Sfondo)
			if ia || ib {
				ink++
			}
			if ia != ib {
				diff++
			}
		}
	}
	return ink > 0 && diff*100 > ink*15
}

func zonaCambiata(a, b *image.RGBA, bl Blocco) bool {
	r := image.Rect(bl.X, bl.Y, bl.X+bl.W, bl.Y+bl.H).Intersect(a.Bounds())
	if r.Empty() {
		return false
	}
	tot, n := 0, 0
	for y := r.Min.Y; y < r.Max.Y; y += 2 {
		for x := r.Min.X; x < r.Max.X; x += 2 {
			tot += distanza(a.RGBAAt(x, y), b.RGBAAt(x, y))
			n++
		}
	}
	return n > 0 && tot/n > 18
}

// spessoreTratto: lunghezza mediana dei tratti orizzontali d'inchiostro, divisa per la
// grandezza del carattere. Normali intorno a 0,06-0,08, neretto sopra 0,14.
func spessoreTratto(img *image.RGBA, in image.Rectangle, ink, sf color.RGBA, em float64) float64 {
	if em < 14 || in.Empty() { // lettere piccole: lo spessore non si misura bene
		return 0
	}
	var corse []int
	passoY := max(1, in.Dy()/12)
	for y := in.Min.Y; y < in.Max.Y; y += passoY {
		n := 0
		for x := in.Min.X; x <= in.Max.X; x++ {
			if x < in.Max.X {
				c := img.RGBAAt(x, y)
				if distanza(c, ink) < distanza(c, sf) {
					n++
					continue
				}
			}
			if n > 0 && float64(n) < em*0.6 { // le barre lunghe (sottolineature, T) non contano
				corse = append(corse, n)
			}
			n = 0
		}
	}
	if len(corse) < 8 {
		return 0
	}
	sort.Ints(corse)
	// -1: il bordo sfumato delle lettere aggiunge circa un pixel a ogni tratto
	return float64(corse[len(corse)/2]-1) / em
}

// fondoPreciso: la toppa sotto la traduzione, fatta come la farebbe un grafico: si copiano i
// pixel veri del gioco (bordi delle caselle, sfumature, icone restano identici) e si cancellano
// SOLO i pixel delle lettere inglesi, riempiendoli con i colori che li circondano. Il rettangolo
// x0..x1, y0..y1 è quello che occupa la traduzione (può essere più largo dell'inglese).
func fondoPreciso(b Blocco, x0, y0, x1, y1 int) (*image.RGBA, int, int) {
	img := b.Img
	r := image.Rect(x0, y0, x1, y1).Intersect(img.Bounds())
	if r.Empty() {
		return nil, 0, 0
	}
	w, h := r.Dx(), r.Dy()
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		copy(out.Pix[y*out.Stride:y*out.Stride+w*4], img.Pix[img.PixOffset(r.Min.X, r.Min.Y+y):img.PixOffset(r.Min.X, r.Min.Y+y)+w*4])
	}
	// stima dello sfondo liscio sotto la scritta inglese (dai quattro bordi del suo riquadro)
	// margine: ombre, bordini e lettere che l'OCR taglia (la "J" iniziale) stanno appena fuori
	pad := max(2, int(b.Em*0.15+0.5))
	if b.Marg > 0 {
		pad = b.Marg
	}
	bx0, by0 := b.X-pad, b.Y-pad
	bw, bh := b.W+2*pad, b.H+2*pad
	stima := coonsRiquadro(img, b, bx0, by0, bw, bh)
	soglia := max(36, distanza(b.Inchiostro, b.Sfondo)/4)
	lettereChiare := distanza(b.Inchiostro, b.Sfondo) >= 120
	mask := make([]bool, w*h)
	for y := 0; y < bh; y++ {
		for x := 0; x < bw; x++ {
			gx, gy := bx0+x, by0+y
			ox, oy := gx-r.Min.X, gy-r.Min.Y
			if ox < 0 || oy < 0 || ox >= w || oy >= h {
				continue
			}
			c := out.RGBAAt(ox, oy)
			dSf := distanza(c, stima.RGBAAt(x, y))
			if dSf <= soglia {
				continue
			}
			// su un fondo disegnato (erba, cielo, rocce) non tutto ciò che è diverso dalla stima è
			// una lettera: cancello solo i pixel più vicini al colore delle lettere (o del loro
			// bordino) che al fondo. Il resto del disegno del gioco resta com'è, nitido.
			dInk := distanza(c, b.Inchiostro)
			if b.Ha2 {
				dInk = min(dInk, distanza(c, b.Inchiostro2))
			}
			if lettereChiare && !(dInk < dSf && dInk < 130) &&
				!(b.HaContorno && distanza(c, b.Contorno) < dSf) {
				continue
			}
			mask[oy*w+ox] = true
		}
	}
	// ombre e aloni del gioco attaccati alle lettere: vicino (3 pixel) a una lettera basta
	// che il pixel sia un po' diverso dal fondo
	if lettereChiare {
		vicino := make([]bool, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if !mask[y*w+x] {
					continue
				}
				for dy := -3; dy <= 3; dy++ {
					for dx := -3; dx <= 3; dx++ {
						if xx, yy := x+dx, y+dy; xx >= 0 && yy >= 0 && xx < w && yy < h {
							vicino[yy*w+xx] = true
						}
					}
				}
			}
		}
		for y := 0; y < bh; y++ {
			for x := 0; x < bw; x++ {
				ox, oy := bx0+x-r.Min.X, by0+y-r.Min.Y
				if ox < 0 || oy < 0 || ox >= w || oy >= h || mask[oy*w+ox] || !vicino[oy*w+ox] {
					continue
				}
				if distanza(out.RGBAAt(ox, oy), stima.RGBAAt(x, y)) > soglia/2 {
					mask[oy*w+ox] = true
				}
			}
		}
	}
	// allargo di 1-2 pixel: i bordi sfumati delle lettere (antialias, alone)
	rag := 1
	if b.Em >= 22 {
		rag = 2
	}
	larga := make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !mask[y*w+x] {
				continue
			}
			for dy := -rag; dy <= rag; dy++ {
				for dx := -rag; dx <= rag; dx++ {
					xx, yy := x+dx, y+dy
					if xx >= 0 && yy >= 0 && xx < w && yy < h {
						larga[yy*w+xx] = true
					}
				}
			}
		}
	}
	// riempio dall'esterno verso l'interno con la media dei vicini già buoni
	noto := make([]bool, w*h)
	for i := range noto {
		noto[i] = !larga[i]
	}
	for giro := 0; giro < 80; giro++ {
		var nuovi []int
		var colori []color.RGBA
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i := y*w + x
				if noto[i] {
					continue
				}
				var sr, sg, sb, n int
				for _, d := range [8][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}, {-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
					xx, yy := x+d[0], y+d[1]
					if xx < 0 || yy < 0 || xx >= w || yy >= h || !noto[yy*w+xx] {
						continue
					}
					c := out.RGBAAt(xx, yy)
					sr, sg, sb, n = sr+int(c.R), sg+int(c.G), sb+int(c.B), n+1
				}
				if n > 0 {
					nuovi = append(nuovi, i)
					colori = append(colori, color.RGBA{uint8(sr / n), uint8(sg / n), uint8(sb / n), 255})
				}
			}
		}
		if len(nuovi) == 0 {
			break
		}
		for k, i := range nuovi {
			out.SetRGBA(i%w, i/w, colori[k])
			noto[i] = true
		}
	}
	for i := range noto { // rimasti (caso raro): la stima liscia
		if !noto[i] {
			x, y := i%w, i/w
			out.SetRGBA(x, y, stima.RGBAAt(clamp(x+r.Min.X-bx0, 0, bw-1), clamp(y+r.Min.Y-by0, 0, bh-1)))
		}
	}
	return out, r.Min.X, r.Min.Y
}

// coonsRiquadro: sfondo liscio stimato dai quattro bordi del riquadro x0,y0,w,h.
func coonsRiquadro(img *image.RGBA, b Blocco, x0, y0, w, h int) *image.RGBA {
	bb := b
	bb.X, bb.W, bb.Centro, bb.LargMax = x0+3, w-6, false, 0
	bb.Y, bb.H = y0+3, h-6
	return fondoRicostruito(img, bb)
}

// iconaAccanto: subito a destra della scritta (dopo un piccolo buco, entro un em e mezzo) c'è un
// disegno del colore delle lettere che l'OCR non ha letto: l'icona di un tasto ("Swing Sword
// (Y)", il cerchio bianco con la lettera). Restituisce la x dove comincia (0 = niente). La
// traduzione non deve andarci sopra.
func iconaAccanto(img *image.RGBA, b Blocco) int {
	if img == nil || max(1, b.Righe) != 1 || b.H < 6 || distanza(b.Inchiostro, b.Sfondo) < 120 {
		return 0
	}
	bd := img.Bounds()
	em := b.Em
	if em <= 0 {
		em = float64(b.H) * 1.1
	}
	y0, y1 := max(b.Y, bd.Min.Y), min(b.Y+b.H, bd.Max.Y)
	if y1-y0 < 4 {
		return 0
	}
	colonna := func(x int) int {
		n := 0
		for y := y0; y < y1; y++ {
			c := img.RGBAAt(x, y)
			if distanza(c, b.Inchiostro) < 70 && distanza(c, b.Sfondo) > 90 {
				n++
			}
		}
		return n
	}
	n := y1 - y0
	fine := min(b.X+b.W+int(1.5*em+0.5), b.X+max(b.W, b.LargMax), bd.Max.X)
	buco, serveBuco := 0, max(2, int(em/5))
	for x := b.X + b.W + 1; x < fine; x++ {
		if colonna(x)*10 < n {
			buco++
			continue
		}
		if buco < serveBuco {
			buco = 0 // ancora la coda delle lettere: non è un'icona
			continue
		}
		// un'icona è un disegno largo almeno un terzo di em (colonne con inchiostro senza buchi;
		// dentro il cerchio la lettera scura del tasto lascia poco inchiostro, ma non zero), pieno
		// in qualche colonna per buona parte dell'altezza
		larga, piena := 0, 0
		for k := x; k < min(x+int(2*em), bd.Max.X); k++ {
			c := colonna(k)
			if c*10 < n {
				break
			}
			larga++
			piena = max(piena, c)
		}
		if float64(larga) >= em/3 && piena*10 >= n*5 {
			return x
		}
		buco = 0
		x += larga
	}
	return 0
}

// liberoFinoAlBordo: quanto può allargarsi a destra la traduzione prima di toccare un bordo,
// un'icona o un'altra cosa disegnata dal gioco (colonna dove la maggior parte dei pixel non è
// più lo sfondo della scritta). Così la scritta italiana non va sopra i contorni delle caselle.
func liberoFinoAlBordo(img *image.RGBA, b Blocco) int {
	l, _ := liberoFinoAlBordoB(img, b)
	return l
}

// liberoFinoAlBordoB: come liberoFinoAlBordo, e dice anche se ha trovato un bordo vero.
func liberoFinoAlBordoB(img *image.RGBA, b Blocco) (int, bool) {
	lim := b.LargMax
	bd := img.Bounds()
	y0, y1 := max(b.Y, bd.Min.Y), min(b.Y+b.H, bd.Max.Y)
	if y1-y0 < 3 {
		return max(b.W, lim), false
	}
	// un bordo vero (casella, cornice) è una linea dritta: stessa tinta dall'alto in basso e
	// diversa dalla colonna accanto. Un disegno (erba, rocce, cielo) cambia di riga in riga: lì
	// la traduzione può passare, come nel gioco.
	for x := b.X + b.W + 2; x < b.X+b.LargMax && x < bd.Max.X; x++ {
		var col []color.RGBA
		diversi := 0
		for y := y0; y < y1; y++ {
			c := img.RGBAAt(x, y)
			col = append(col, c)
			if distanza(c, b.Sfondo) > 60 {
				diversi++
			}
		}
		n := len(col)
		if diversi*10 < n*8 {
			continue
		}
		m := col[n/2]
		simili, salto := 0, 0
		for i, c := range col {
			if distanza(c, m) < 45 {
				simili++
			}
			if x-2 >= bd.Min.X && distanza(c, img.RGBAAt(x-2, y0+i)) > 60 {
				salto++
			}
		}
		if simili*10 >= n*8 && salto*10 >= n*7 {
			return max(b.W, x-b.X-6), true
		}
	}
	return max(b.W, lim), false
}

// contornoLettere: molti giochi scrivono lettere chiare con un bordino scuro (o il contrario).
// Se l'inglese ce l'ha, anche la traduzione lo deve avere, dello stesso colore.
func contornoLettere(img *image.RGBA, b *Blocco) {
	in := image.Rect(b.X-1, b.Y-1, b.X+b.W+1, b.Y+b.H+1).Intersect(img.Bounds())
	if b.HaContorno || in.Empty() || distanza(b.Inchiostro, b.Sfondo) < 120 {
		return
	}
	inchiostro := func(x, y int) bool {
		if !(image.Point{x, y}.In(in)) {
			return false
		}
		return distanza(img.RGBAAt(x, y), b.Inchiostro) < 70
	}
	var sr, sg, sb, nc, ni int
	for y := in.Min.Y; y < in.Max.Y; y++ {
		for x := in.Min.X; x < in.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if distanza(c, b.Inchiostro) < 70 {
				ni++
				continue
			}
			if distanza(c, b.Inchiostro) < 150 || distanza(c, b.Sfondo) < 90 {
				continue
			}
			vicino := false
			for _, d := range [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				if inchiostro(x+d[0], y+d[1]) {
					vicino = true
					break
				}
			}
			if vicino {
				sr, sg, sb, nc = sr+int(c.R), sg+int(c.G), sb+int(c.B), nc+1
			}
		}
	}
	if ni > 20 && nc*10 > ni*3 {
		b.Contorno = color.RGBA{uint8(sr / nc), uint8(sg / nc), uint8(sb / nc), 255}
		b.HaContorno = contornoVero(b.Inchiostro, b.Contorno, b.Sfondo)
	}
}

// riempimentoEContorno: lettere bianche con il bordino nero su un cielo chiaro (molti menu). Il
// colore "più lontano dallo sfondo" è il bordino, non la lettera: divido i pixel diversi dallo
// sfondo in due gruppi e la lettera è quello che tocca meno lo sfondo (il bordino lo tocca sempre).
func riempimentoEContorno(img *image.RGBA, in image.Rectangle, b *Blocco) {
	if in.Dx() < 4 || in.Dy() < 4 {
		return
	}
	type pt struct {
		x, y int
		c    color.RGBA
	}
	var px []pt
	for y := in.Min.Y; y < in.Max.Y; y++ {
		for x := in.Min.X; x < in.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if distanza(c, b.Sfondo) > 90 {
				px = append(px, pt{x, y, c})
			}
		}
	}
	if len(px) < 30 {
		return
	}
	// due centri: il più lontano dallo sfondo, e il più lontano da quello
	c1 := px[0].c
	for _, p := range px {
		if distanza(p.c, b.Sfondo) > distanza(c1, b.Sfondo) {
			c1 = p.c
		}
	}
	c2 := c1
	for _, p := range px {
		if distanza(p.c, c1) > distanza(c2, c1) {
			c2 = p.c
		}
	}
	if distanza(c1, c2) < 150 {
		return // un colore solo: niente bordino
	}
	gr := make([]int, len(px))
	for it := 0; it < 6; it++ {
		var s [2][4]int
		for i, p := range px {
			g := 0
			if distanza(p.c, c2) < distanza(p.c, c1) {
				g = 1
			}
			gr[i] = g
			s[g][0] += int(p.c.R)
			s[g][1] += int(p.c.G)
			s[g][2] += int(p.c.B)
			s[g][3]++
		}
		if s[0][3] == 0 || s[1][3] == 0 {
			return
		}
		c1 = color.RGBA{uint8(s[0][0] / s[0][3]), uint8(s[0][1] / s[0][3]), uint8(s[0][2] / s[0][3]), 255}
		c2 = color.RGBA{uint8(s[1][0] / s[1][3]), uint8(s[1][1] / s[1][3]), uint8(s[1][2] / s[1][3]), 255}
	}
	if distanza(c1, c2) < 150 {
		return
	}
	var n, tocca [2]int
	for i, p := range px {
		g := gr[i]
		n[g]++
		for _, d := range [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
			q := image.Point{p.x + d[0], p.y + d[1]}
			if q.In(in) && distanza(img.RGBAAt(q.X, q.Y), b.Sfondo) < 60 {
				tocca[g]++
				break
			}
		}
	}
	tot := n[0] + n[1]
	if n[0]*100 < tot*12 || n[1]*100 < tot*12 {
		return // uno dei due è solo rumore
	}
	r0 := float64(tocca[0]) / float64(n[0])
	r1 := float64(tocca[1]) / float64(n[1])
	ink, cont := c1, c2
	if r1 < r0*0.7 {
		ink, cont = c2, c1
	} else if !(r0 < r1*0.7) {
		return // non si capisce: lascio com'era
	}
	if contornoVero(ink, cont, b.Sfondo) {
		b.Inchiostro, b.Contorno, b.HaContorno = ink, cont, true
	}
}

func luce(c color.RGBA) int { return (int(c.R)*299 + int(c.G)*587 + int(c.B)*114) / 1000 }

// contornoVero: un bordino è più scuro dello sfondo se le lettere sono più chiare (o il
// contrario). Un colore a metà tra lettere e sfondo è solo il bordo sfumato delle lettere.
func contornoVero(ink, cont, sf color.RGBA) bool {
	li, lc, ls := luce(ink), luce(cont), luce(sf)
	return (li > ls && ls-lc >= 30) || (li < ls && lc-ls >= 30)
}

// lineaDiBase: dove poggiano le lettere dell'originale (y). Il riquadro dell'OCR va dalla cima
// delle lettere al fondo: se ci sono lettere con la gamba sotto (g, p, y…) il fondo è più giù
// della linea di base. La traduzione poggia sulla stessa linea: non "scende" di qualche pixel.
func lineaDiBase(b Blocco) int {
	sotto := false
	for _, c := range b.Testo {
		if strings.ContainsRune("gjpqy(),;", c) {
			sotto = true
			break
		}
	}
	y := b.Y + b.H
	if sotto && b.Em > 0 {
		y -= int(b.Em*0.21 + 0.5)
	}
	return y
}

// margineToppa: quanto la toppa può uscire dal riquadro dell'inglese per prendere ombre, bordini e
// lettere tagliate dall'OCR, senza arrivare alle scritte vicine (altri = tutte le scritte lette).
func margineToppa(b Blocco, altri []Blocco) int {
	m := max(2, int(b.Em*0.2+0.5))
	for _, o := range altri {
		if o.X == b.X && o.Y == b.Y && o.W == b.W && o.H == b.H {
			continue
		}
		// distanza tra i due riquadri (0 se si toccano)
		dx := max(0, max(o.X-(b.X+b.W), b.X-(o.X+o.W)))
		dy := max(0, max(o.Y-(b.Y+b.H), b.Y-(o.Y+o.H)))
		g := max(dx, dy)
		if dx > 0 && dy > 0 {
			continue // in diagonale: la toppa resta nel suo rettangolo
		}
		if lim := (g - 1) / 2; lim < m {
			m = lim
		}
	}
	return max(1, m)
}

// pulisciPagina: una copia della cattura con le lettere inglesi di tutte le scritte tradotte
// cancellate (sfondo ricostruito sotto). Ogni toppa copia da qui: una toppa larga non riporta
// mai le lettere inglesi della scritta vicina.
func pulisciPagina(img *image.RGBA, bl []Blocco) *image.RGBA {
	out := image.NewRGBA(img.Bounds())
	copy(out.Pix, img.Pix)
	cancellaInglese(out, img, bl)
	return out
}

// cancellaInglese: in out (copia di img) cancella le lettere inglesi delle scritte bl.
func cancellaInglese(out, img *image.RGBA, bl []Blocco) {
	for _, b := range bl {
		b.Img = img
		m := b.Marg
		if m <= 0 {
			m = max(2, int(b.Em*0.15+0.5))
		}
		// 3 pixel in più: l'anello esterno dà i colori veri da cui riempire
		f, fx, fy := fondoPreciso(b, b.X-m-3, b.Y-m-3, b.X+b.W+m+3, b.Y+b.H+m+3)
		if f == nil {
			continue
		}
		fb := f.Bounds()
		for y := 0; y < fb.Dy(); y++ {
			for x := 0; x < fb.Dx(); x++ {
				if c := f.RGBAAt(x, y); c != img.RGBAAt(fx+x, fy+y) { // solo i pixel cancellati
					out.SetRGBA(fx+x, fy+y, c)
				}
			}
		}
	}
}

var rePiattaforma = regexp.MustCompile(`(?i)\((steam|epic|xbox|psn|playstation|gog|ea|ubisoft|battle\.net)\)\s*$`)

// nomeGiocatore: "NOME (STEAM)", oppure una sola parola tutta maiuscola con cifre in mezzo
// ("1LMESS1A", "XX4LEX"): è un nome, non una scritta del gioco.
func nomeGiocatore(s string) bool {
	if rePiattaforma.MatchString(strings.TrimSpace(s)) {
		return true
	}
	w := strings.Fields(s)
	if len(w) != 1 || strings.ToUpper(w[0]) != w[0] {
		return false
	}
	cifre, lettere := 0, 0
	for _, c := range w[0] {
		if unicode.IsDigit(c) {
			cifre++
		} else if unicode.IsLetter(c) {
			lettere++
		}
	}
	return cifre > 0 && lettere > 1
}

// componiTesto: i pixel (RGBA) del rettangolo r con la scritta sopra la pagina pulita. alfa è la
// copertura delle lettere (0-255) per ogni pixel di r. Il bordino (sp pixel) o l'ombra prendono
// i colori letti dal gioco. Dove non c'è niente da coprire metto il colore trasparente della
// finestra (magenta), così si vede il gioco vero (anteprima=true: il gioco della cattura).
func componiTesto(b Blocco, r image.Rectangle, alfa []uint8, sp int, anteprima bool) []byte {
	w, h := r.Dx(), r.Dy()
	at := func(x, y int) int {
		if x < 0 || y < 0 || x >= w || y >= h {
			return 0
		}
		return int(alfa[y*w+x])
	}
	out := make([]byte, w*h*4)
	ombra := contrastante(b.Inchiostro)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			gx, gy := r.Min.X+x, r.Min.Y+y
			base := b.Pulita.RGBAAt(gx, gy)
			ai := at(x, y)
			ao, as := 0, 0
			if b.HaContorno && !b.Ombra {
				for dy := -sp; dy <= sp; dy++ {
					for dx := -sp; dx <= sp; dx++ {
						ao = max(ao, at(x-dx, y-dy))
					}
				}
			}
			if b.Ombra {
				as = max(at(x-1, y-1), at(x-2, y-2)*3/4, at(x+1, y-1)/2, at(x-1, y+1)/2) * 3 / 4
			} else if b.HaOmbraG {
				d := max(1, int(b.Em/14+0.5))
				as = max(at(x-d, y-d), at(x-d-1, y-d-1)/2)
			}
			i := (y*w + x) * 4
			if ai == 0 && ao == 0 && as == 0 {
				// niente lettere qui: alfa 0 = lascia quello che c'è già (la toppa di questo
				// riquadro, o le lettere già scritte di una riga vicina che si toccano)
				out[i], out[i+1], out[i+2], out[i+3] = base.R, base.G, base.B, 0
				if base == b.Img.RGBAAt(gx, gy) && !anteprima {
					out[i], out[i+1], out[i+2] = 0xFF, 0x00, 0xFF // trasparente
				}
				continue
			}
			c := [3]int{int(base.R), int(base.G), int(base.B)}
			mescola := func(col color.RGBA, a int) {
				if a <= 0 {
					return
				}
				c[0] = (c[0]*(255-a) + int(col.R)*a) / 255
				c[1] = (c[1]*(255-a) + int(col.G)*a) / 255
				c[2] = (c[2]*(255-a) + int(col.B)*a) / 255
			}
			if b.HaOmbraG && !b.Ombra {
				mescola(b.OmbraGioco, as/2)
			} else {
				mescola(ombra, as)
			}
			mescola(b.Contorno, ao)
			mescola(b.Inchiostro, ai)
			out[i], out[i+1], out[i+2], out[i+3] = uint8(c[0]), uint8(c[1]), uint8(c[2]), 255
			if out[i] == 0xFF && out[i+1] == 0 && out[i+2] == 0xFF {
				out[i+2] = 0xFE // non deve diventare trasparente per caso
			}
		}
	}
	return out
}

// ombraLettere: molti giochi mettono sotto le lettere chiare un'ombra scura spostata in basso a
// destra. La guardo 1-2 pixel sotto ogni lettera: se lì è più scuro del fondo, la rifaccio uguale.
func ombraLettere(img *image.RGBA, b *Blocco) {
	in := image.Rect(b.X, b.Y, b.X+b.W, b.Y+b.H).Intersect(img.Bounds())
	if in.Empty() || b.HaContorno || luce(b.Inchiostro) <= luce(b.Sfondo)+40 {
		return
	}
	d := max(1, int(b.Em/14+0.5))
	var sr, sg, sb, n, ni int
	for y := in.Min.Y; y < in.Max.Y; y++ {
		for x := in.Min.X; x < in.Max.X; x++ {
			if distanza(img.RGBAAt(x, y), b.Inchiostro) > 60 {
				continue
			}
			ni++
			q := image.Point{x + d, y + d}
			if !q.In(img.Bounds()) {
				continue
			}
			c := img.RGBAAt(q.X, q.Y)
			if distanza(c, b.Inchiostro) < 100 {
				continue // ancora lettera
			}
			sr, sg, sb, n = sr+int(c.R), sg+int(c.G), sb+int(c.B), n+1
		}
	}
	if ni < 20 || n*5 < ni {
		return
	}
	om := color.RGBA{uint8(sr / n), uint8(sg / n), uint8(sb / n), 255}
	if luce(om) <= luce(b.Sfondo)-25 {
		b.OmbraGioco, b.HaOmbraG = om, true
	}
}

// riquadraDiNuovo: dopo un cambio (mouse sopra) le lettere inglesi possono essere più grandi o
// spostate. Cerco dove sono adesso (pixel del colore delle lettere attorno al riquadro di prima)
// e aggiorno riquadro e grandezza: la traduzione si ingrandisce come l'originale.
func riquadraDiNuovo(img *image.RGBA, b *Blocco) {
	ex, ey := max(4, b.W/4), max(4, b.H/2)
	r := image.Rect(b.X-ex, b.Y-ey, b.X+b.W+ex, b.Y+b.H+ey).Intersect(img.Bounds())
	if r.Empty() || b.H < 6 {
		return
	}
	c := *b
	coloriBlocco(img, &c)
	if distanza(c.Inchiostro, c.Sfondo) < 120 {
		return
	}
	// righe e colonne con abbastanza pixel "lettera"
	rig := make([]int, r.Dy())
	col := make([]int, r.Dx())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if distanza(img.RGBAAt(x, y), c.Inchiostro) < 70 {
				rig[y-r.Min.Y]++
				col[x-r.Min.X]++
			}
		}
	}
	// la fascia di righe che contiene il centro della scritta di prima
	cy := b.Y + b.H/2 - r.Min.Y
	if cy < 0 || cy >= len(rig) || rig[cy] == 0 {
		return
	}
	y0, y1 := cy, cy
	for y0 > 0 && rig[y0-1] > 0 {
		y0--
	}
	for y1 < len(rig)-1 && rig[y1+1] > 0 {
		y1++
	}
	nh := y1 - y0 + 1
	if nh < b.H*8/10 || nh > b.H*16/10 || y0 == 0 || y1 == len(rig)-1 {
		return // non sembra la stessa scritta: lascio com'era
	}
	x0, x1 := -1, -1
	for x, n := range col {
		if n > 0 {
			if x0 < 0 {
				x0 = x
			}
			x1 = x
		}
	}
	if x0 <= 0 || x1 >= len(col)-1 {
		return
	}
	nw := x1 - x0 + 1
	if nw < b.W*8/10 || nw > b.W*16/10 {
		return
	}
	k := float64(nh) / float64(b.H)
	b.X, b.Y, b.W, b.H = r.Min.X+x0, r.Min.Y+y0, nw, nh
	b.Em *= k
}

// secondoInchiostro: una riga con parole di due colori (testo bianco con parole gialle
// evidenziate): anche il secondo colore va cancellato.
func secondoInchiostro(img *image.RGBA, in image.Rectangle, b *Blocco) {
	b.Ha2 = false
	var forti, n, sr, sg, sb int
	for y := in.Min.Y; y < in.Max.Y; y++ {
		for x := in.Min.X; x < in.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if distanza(c, b.Sfondo) < 150 {
				continue
			}
			forti++
			if distanza(c, b.Inchiostro) > 150 && (!b.HaContorno || distanza(c, b.Contorno) > 150) {
				n++
				sr, sg, sb = sr+int(c.R), sg+int(c.G), sb+int(c.B)
			}
		}
	}
	if forti >= 40 && n*100 >= forti*8 {
		b.Inchiostro2 = color.RGBA{uint8(sr / n), uint8(sg / n), uint8(sb / n), 255}
		b.Ha2 = true
	}
}

// rifinisci: la traduzione nella stessa forma dell'originale: TUTTA MAIUSCOLA se l'originale lo
// è, senza il punto finale se l'originale non ce l'ha (pulsanti e titoli).
func rifinisci(orig, trad string) string {
	trad = strings.TrimSpace(trad)
	lettere, maiusc := 0, 0
	for _, r := range orig {
		if unicode.IsLetter(r) {
			lettere++
			if unicode.IsUpper(r) {
				maiusc++
			}
		}
	}
	if lettere >= 2 && maiusc == lettere {
		trad = strings.ToUpper(trad)
	}
	o := strings.TrimSpace(orig)
	if o != "" && !strings.ContainsAny(o[len(o)-1:], ".!?:;…") && strings.HasSuffix(trad, ".") && !strings.HasSuffix(trad, "...") {
		trad = strings.TrimSuffix(trad, ".")
	}
	return trad
}

// correggiNomeGioco: il nome del gioco letto male dall'OCR ("Lia (s Bar", "Ilia (s Bar") torna
// quello giusto, così il traduttore non lo storpia.
func correggiNomeGioco(testo, nome string) string {
	nw := strings.Fields(nome)
	if len(nw) == 0 || len([]rune(nome)) < 4 {
		return testo
	}
	soloLett := func(s string) string {
		var b strings.Builder
		for _, r := range strings.ToLower(s) {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	obiettivo := soloLett(nome)
	if len(obiettivo) < 4 {
		return testo
	}
	w := strings.Fields(testo)
	lim := 1
	if len(obiettivo) >= 7 {
		lim = 2
	}
	for i := 0; i < len(w); i++ {
		// la finestra di parole più vicina al nome (a pari distanza, quella con il numero di
		// parole del nome): così "in No Man's Sky" non si mangia la "in".
		bestK, bestD := 0, lim+1
		for k := len(nw) + 1; k >= max(1, len(nw)-1); k-- {
			if i+k > len(w) {
				continue
			}
			pezzo := soloLett(strings.Join(w[i:i+k], ""))
			if pezzo == "" {
				continue
			}
			d := distanzaModifica(pezzo, obiettivo)
			if d < bestD || d == bestD && abs(k-len(nw)) < abs(bestK-len(nw)) {
				bestK, bestD = k, d
			}
		}
		if bestK == 0 {
			continue
		}
		k := bestK
		if bestD == 0 && strings.TrimRightFunc(strings.Join(w[i:i+k], " "), unicode.IsPunct) == nome {
			i += k - 1 // già giusto
			continue
		}
		// la parola dopo o prima potrebbe essere già il nome giusto senza questa
		if i+1 < len(w) && k > 1 && distanzaModifica(soloLett(strings.Join(w[i+1:min(len(w), i+1+len(nw))], "")), obiettivo) == 0 {
			continue
		}
		resto := ""
		ultima := w[i+k-1]
		if j := strings.LastIndexFunc(ultima, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }); j >= 0 && j+1 < len(ultima) {
			resto = ultima[j+1:]
		}
		w = append(append(append([]string{}, w[:i]...), nome+resto), w[i+k:]...)
	}
	return strings.Join(w, " ")
}

func distanzaModifica(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prec := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prec {
		prec[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			c := 1
			if ra[i-1] == rb[j-1] {
				c = 0
			}
			cur[j] = min(prec[j]+1, cur[j-1]+1, prec[j-1]+c)
		}
		prec, cur = cur, prec
	}
	return prec[len(rb)]
}

// letturaAffidabile: in una frase lunga quasi tutte le parole devono esistere. Se l'OCR ha
// letto male ("La cucitura curmt", "Nkes e Repmts") meglio lasciare l'inglese del gioco che
// mettere sopra una traduzione storpiata.
func letturaAffidabile(s string) bool {
	tot, buone := 0, 0
	for _, m := range reParolaLettere.FindAllStringIndex(s, -1) {
		w := s[m[0]:m[1]]
		if len([]rune(w)) < 3 {
			continue
		}
		tot++
		if parolaValidaOCR(w) || parolaComposta(w) || nomeDiChiParla(s, m[0], m[1]) {
			buone++
		}
	}
	if tot < 4 {
		return true
	}
	return buone*100 >= tot*65
}

var reParolaLettere = regexp.MustCompile(`[\p{L}']+`)

// nomeDiChiParla: la parola s[a:z] è un nome proprio (maiuscola e poi minuscole) nel posto dove
// i dialoghi mettono i nomi: chi parla ("Caz:") o chi viene chiamato, dopo una virgola e prima
// della fine della frase ("…do you need, Scoob?", "…alright, Caz."). Lì un nome che il
// vocabolario non conosce non è una lettura sbagliata. Altrove una parola sconosciuta con la
// maiuscola ("VII Igok at") resta sospetta.
func nomeDiChiParla(s string, a, z int) bool {
	if !nomeProprio(s[a:z]) {
		return false
	}
	dopo := strings.TrimLeft(s[z:], " ")
	prima := strings.TrimRight(s[:a], " ")
	if strings.HasPrefix(dopo, ":") {
		return true
	}
	return strings.HasSuffix(prima, ",") && (dopo == "" || strings.ContainsAny(dopo[:1], ".?!,"))
}

// nomeProprio: maiuscola e poi solo minuscole ("Scoob", "Caz"): nome di una persona o di un posto.
func nomeProprio(w string) bool {
	r := []rune(strings.Trim(w, "'"))
	if len(r) < 2 || !unicode.IsUpper(r[0]) {
		return false
	}
	for _, c := range r[1:] {
		if !unicode.IsLower(c) && c != '\'' {
			return false
		}
	}
	return true
}

// lettereOCRStrane: nelle scritte inglesi piccole l'OCR di Windows legge "ch" come "å" e "sp"
// come "»" ("matåmaking", "di»lays").
func lettereOCRStrane(s string) string {
	if !strings.ContainsAny(s, "å»ß") {
		return s
	}
	return strings.NewReplacer("å", "ch", "»", "sp", "ß", "cs").Replace(s)
}

// colonnePiccole: righe e paragrafi di testo piccolo allineati a sinistra uno sotto l'altro
// (le notizie di Liar's Bar) diventano una colonna: stessa grandezza, stesso carattere e la
// stessa larghezza per andare a capo, come nel gioco.
func colonnePiccole(b []Blocco) {
	usato := make([]bool, len(b))
	for i := range b {
		if usato[i] || b[i].Em <= 0 || b[i].Em >= 20 || b[i].Centro {
			continue
		}
		gruppo := []int{i}
		usato[i] = true
		for j := i + 1; j < len(b); j++ {
			if !usato[j] && b[j].Em > 0 && b[j].Em < 20 && !b[j].Centro && abs(b[j].X-b[i].X) <= 6 &&
				math.Abs(b[j].Em-b[i].Em) <= 1.6 { // i titoletti (più grandi) fanno colonna a parte
				gruppo = append(gruppo, j)
				usato[j] = true
			}
		}
		if len(gruppo) < 3 {
			continue
		}
		var em []float64
		destra, rif, rifW := 0, "", 0
		for _, k := range gruppo {
			em = append(em, b[k].Em)
			destra = max(destra, b[k].X+b[k].W)
			if b[k].Righe <= 1 && b[k].W > rifW {
				rif, rifW = b[k].Testo, b[k].W
			}
		}
		sort.Float64s(em)
		px := int(em[len(em)/2] + 0.5)
		for _, k := range gruppo {
			b[k].PxColonna = px
			b[k].ColRif, b[k].ColRifW = rif, rifW
			b[k].ColLarg = destra - b[k].X
			b[k].LargMax = max(b[k].LargMax, destra-b[k].X)
		}
	}
}

// stabilizza: il gioco si muove (fumo, luci) e la schermata viene riletta spesso; l'OCR a volte
// legge le stesse scritte piccole in modo un po' diverso. Se una scritta sta nello stesso posto
// e il testo letto è quasi uguale, tengo la traduzione che è già sullo schermo: niente parole
// che cambiano da sole sotto gli occhi.
func stabilizza(nuovi, vecchi []Blocco) {
	for i := range nuovi {
		n := &nuovi[i]
		for _, o := range vecchi {
			if o.Tradotto == "" || max(1, o.Righe) != max(1, n.Righe) || abs(o.X-n.X) > 8 || abs(o.Y-n.Y) > 8 ||
				abs(o.W-n.W)*5 > max(o.W, n.W) {
				continue
			}
			if stessaScritta(o.Testo, n.Testo) {
				// anche il testo letto prima: così tieniFerme la riconosce come la stessa scritta
				// e la lascia identica (niente tremolio di 1-2 pixel a ogni rilettura)
				n.Tradotto, n.Testo = o.Tradotto, o.Testo
			}
			break
		}
	}
}

// stessaScritta: due letture della stessa scritta che differiscono solo per gli errori tipici
// dell'OCR. I numeri e i segni devono essere identici: "Level 5" → "Level 6" o "Volume 50" →
// "Volume 80" è un cambio vero del gioco, non un errore di lettura. Nelle scritte corte (voci di
// menu, valori) accetto solo le lettere che l'OCR confonde (l/I/1, O/0, rn/m) e gli spazi; nelle
// frasi lunghe anche qualche lettera letta male ("curmt" al posto di "current"), fino al 30%.
func stessaScritta(a, b string) bool {
	if a == b {
		return true
	}
	if numeriESegni(a) != numeriESegni(b) {
		return false
	}
	la, lb := lettereConfuse(a), lettereConfuse(b)
	if la == lb {
		return true
	}
	if len(strings.Fields(a)) < 3 || len(strings.Fields(b)) < 3 {
		return false
	}
	lung := max(len([]rune(a)), len([]rune(b)))
	if lung < 15 {
		return false
	}
	if parolaCambiata(a, b) {
		return false
	}
	return distanzaModifica(la, lb)*10 <= lung*3
}

// parolaCambiata: con lo stesso numero di parole, una parola lunga è diventata un'altra parola
// intera ("TUTORIAL: BLOCK ATTACKS" → "TUTORIAL: HEAVY ATTACKS"): è un'altra scritta, non la
// stessa letta male (banco del mattino 26/11: la traduzione vecchia restava sulla scritta nuova).
// L'OCR sbaglia qualche lettera ("uniil"/"until", "curmt"/"current"), non tutte.
func parolaCambiata(a, b string) bool {
	wa, wb := strings.Fields(a), strings.Fields(b)
	if len(wa) != len(wb) {
		return false
	}
	for i := range wa {
		x := lettereConfuse(strings.ToLower(strings.Trim(wa[i], ".,;:!?\"'()[]")))
		y := lettereConfuse(strings.ToLower(strings.Trim(wb[i], ".,;:!?\"'()[]")))
		n := max(len([]rune(x)), len([]rune(y)))
		if n >= 4 && distanzaModifica(x, y)*2 > n {
			return true
		}
	}
	return false
}

// numeriESegni: le cifre e i segni (+ - % / :) della scritta, in ordine. Dentro un numero la O
// letta al posto dello 0 e la l/I letta al posto dell'1 contano come cifre ("1O0" = "100").
// Dentro una parola invece una cifra è un errore dell'OCR ("0ptions", "Hel1o") e non conta,
// tranne le cifre in fondo ("Level5": numero attaccato alla parola).
func numeriESegni(s string) string {
	var out []rune
	for _, w := range strings.Fields(s) {
		rw := []rune(w)
		d, ultimaLettera := 0, -1
		for i, r := range rw {
			if unicode.IsDigit(r) {
				d++
			} else if unicode.IsLetter(r) {
				ultimaLettera = i
			}
		}
		// un numero: cifre, e al massimo le lettere che l'OCR scambia con le cifre ("1OO%")
		numero := d > 0 && strings.IndexFunc(w, func(r rune) bool {
			return unicode.IsLetter(r) && !strings.ContainsRune("OolI", r)
		}) < 0
		for i, r := range rw {
			switch {
			case unicode.IsDigit(r) && (numero || i > ultimaLettera):
				out = append(out, r)
			case numero && (r == 'O' || r == 'o'):
				out = append(out, '0')
			case numero && (r == 'l' || r == 'I' || r == '|'):
				out = append(out, '1')
			case strings.ContainsRune("+-%/:", r):
				out = append(out, r)
			}
		}
		out = append(out, ' ')
	}
	return strings.TrimSpace(string(out))
}

// lettereConfuse: la scritta senza spazi né segni, minuscola, con le lettere che l'OCR scambia
// ridotte a una sola (l/I/1/| → l, O/0 → o, rn → m).
func lettereConfuse(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r == '1' || r == '|' || r == 'i':
			b.WriteRune('l')
		case r == '0':
			b.WriteRune('o')
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		}
	}
	return strings.ReplaceAll(b.String(), "rn", "m")
}

// soloNome: lettere e cifre minuscole, per confrontare un titolo letto con il nome del gioco.
func soloNome(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// tieniFerme: le scritte che erano già tradotte e sotto cui l'inglese non è cambiato restano
// IDENTICHE a prima (stessa posizione, grandezza, toppa). Ogni rilettura dà posizioni che
// variano di 1-2 pixel: ridisegnandole a ogni passaggio del mouse le scritte "tremavano"
// (Sergio, No Man's Sky, inventario, 24/11). Cambiano solo quelle davvero nuove.
//
// La scritta tenuta punta poi alla cattura nuova e alla sua pagina pulita (dove ricopio le sue
// lettere cancellate): così non tiene in memoria le immagini delle letture vecchie.
func tieniFerme(nuovi, vecchi []Blocco, img *image.RGBA) []Blocco {
	if len(vecchi) == 0 || img == nil {
		return nuovi
	}
	vicino := func(a, b, d int) bool { return a-b <= d && b-a <= d }
	usato := make([]bool, len(vecchi))
	out := make([]Blocco, len(nuovi))
	copy(out, nuovi)
	var pul *image.RGBA // la pagina pulita della lettura nuova
	for _, n := range nuovi {
		if n.Pulita != nil && n.Img == img && n.Pulita.Bounds() == img.Bounds() {
			pul = n.Pulita
			break
		}
	}
	for i, n := range out {
		for j, o := range vecchi {
			if usato[j] || o.Testo != n.Testo || o.Tradotto != n.Tradotto || o.Img == nil || o.Img.Bounds() != img.Bounds() {
				continue
			}
			if !vicino(o.X, n.X, 4) || !vicino(o.Y, n.Y, 4) || !vicino(o.W, n.W, 8) || !vicino(o.H, n.H, 4) {
				continue
			}
			if zonaCambiata(o.Img, img, o) {
				continue
			}
			if o.Pulita == nil {
				o.Img = img
			} else if pul != nil {
				copiaCancellati(pul, o)
				o.Img, o.Pulita = img, pul
			}
			if o.Img == img {
				o.Tinta = tintaBlocco(img, o)
			}
			out[i], usato[j] = o, true
			break
		}
	}
	return out
}

// sembraMenu: il testo letto in una zona ha più righe corte (voci di menu, impostazioni,
// nome e valore), non un dialogo. Allora le scritte si traducono una per una: unite in una
// frase sola non avevano senso ("Dispositivo di uscita audio audio generale Sistema di volume
// di gioco predefinito", PC Cafe Simulator, F9, 24/11).
func sembraMenu(grezzo string) bool {
	var righe []string
	for _, r := range strings.Split(strings.ReplaceAll(grezzo, "\r", ""), "\n") {
		if strings.TrimSpace(r) != "" {
			righe = append(righe, r)
		}
	}
	// almeno 3 voci; "NOME\nFrase" è un dialogo con chi parla, non un menu
	if len(righe) < 3 {
		return false
	}
	if t := strings.TrimSpace(righe[0]); tuttoMaiuscolo(t) && len(strings.Fields(t)) <= 3 && strings.ContainsAny(strings.TrimSpace(righe[len(righe)-1])[len(strings.TrimSpace(righe[len(righe)-1]))-1:], ".!?") {
		return false
	}
	parole := 0
	for _, r := range righe {
		parole += len(strings.Fields(r))
	}
	return float64(parole)/float64(len(righe)) <= 4.5
}

// righeDiBlocchi: le scritte raggruppate per riga (dall'alto in basso), ognuna da sinistra a
// destra: "Audio Output Device" e "System Default Device" sulla stessa riga.
func righeDiBlocchi(bl []Blocco) [][]Blocco {
	b := append([]Blocco(nil), bl...)
	sort.SliceStable(b, func(i, j int) bool { return b[i].Y+b[i].H/2 < b[j].Y+b[j].H/2 })
	var out [][]Blocco
	for _, x := range b {
		c := x.Y + x.H/2
		if n := len(out); n > 0 {
			u := out[n-1][0]
			if abs(c-(u.Y+u.H/2)) < max(u.H, x.H)/2+1 {
				out[n-1] = append(out[n-1], x)
				continue
			}
		}
		out = append(out, []Blocco{x})
	}
	for _, r := range out {
		sort.SliceStable(r, func(i, j int) bool { return r[i].X < r[j].X })
	}
	return out
}
