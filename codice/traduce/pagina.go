package traduce

// Modalità "tutta la schermata" (F6): per i menu pieni di scritte. L'OCR dà ogni parola con la
// sua posizione; qui le raggruppo in blocchi (una voce di menu, una riga di lista, un paragrafo),
// e per ogni blocco calcolo il colore dello sfondo e del testo, così la traduzione viene
// disegnata esattamente sopra l'inglese e sembra scritta dal gioco.

import (
	"image"
	"image/color"
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
	Testo      string
	X, Y, W, H int
	Righe      int
	Sfondo     color.RGBA
	Inchiostro color.RGBA
	Tradotto   string
	Em         float64     // grandezza del carattere stimata dalle lettere (pixel)
	LargMax    int         // spazio libero a destra prima della scritta vicina
	Fondo      *image.RGBA // sfondo ricostruito (sfumature del gioco), grande come il blocco +3 px per lato
	Tratto     float64     // spessore medio delle lettere diviso la grandezza (grassetto se alto)
}

// grassetto: lettere spesse come quelle in neretto dei giochi.
func (b Blocco) grassetto() bool { return b.Tratto >= sogliaGrassetto }

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
			if buco < 1.1*hMed { // spazio tra parole ~0,3-0,5 volte l'altezza: oltre è un'altra colonna
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
	for _, w := range p {
		x0, y0 = min(x0, w.X), min(y0, w.Y)
		x1, y1 = max(x1, w.X+w.W), max(y1, w.Y+w.H)
		parole = append(parole, w.T)
	}
	return Blocco{
		Testo: strings.Join(parole, " "),
		X:     int(x0 / f), Y: int(y0 / f), W: int((x1-x0)/f + 0.5), H: int((y1-y0)/f + 0.5), Righe: 1,
		Em: emParole(p) / f,
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
		b[i].LargMax = max(lim, b[i].W)
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
		for {
			trovato := -1
			for j := i + 1; j < len(b); j++ {
				if usato[j] {
					continue
				}
				n := b[j]
				gap := n.Y - (cur.Y + cur.H)
				if gap < -hRiga/2 || gap > hRiga {
					continue
				}
				if abs(n.X-cur.X) > hRiga || float64(n.H) > 1.4*float64(hRiga) || float64(n.H) < float64(hRiga)/1.4 {
					continue
				}
				if continuaFrase(cur.Testo, n.Testo) {
					trovato = j
				}
				break
			}
			if trovato < 0 {
				break
			}
			n := b[trovato]
			usato[trovato] = true
			x1, y1 := max(cur.X+cur.W, n.X+n.W), n.Y+n.H
			cur.X = min(cur.X, n.X)
			cur.W, cur.H = x1-cur.X, y1-cur.Y
			if strings.HasSuffix(cur.Testo, "-") {
				cur.Testo = strings.TrimSuffix(cur.Testo, "-") + n.Testo
			} else {
				cur.Testo += " " + n.Testo
			}
			cur.Righe++
		}
		out = append(out, cur)
	}
	return out
}

func continuaFrase(sopra, sotto string) bool {
	sotto = strings.TrimSpace(sotto)
	if sotto == "" {
		return false
	}
	if strings.HasPrefix(sotto, "-") || strings.HasPrefix(sotto, "•") {
		return false
	}
	r := []rune(sotto)[0]
	if unicode.IsLower(r) {
		return true
	}
	return strings.HasSuffix(sopra, ",") || strings.HasSuffix(sopra, "-")
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
	b.Tratto = spessoreTratto(img, in, b.Inchiostro, b.Sfondo, b.Em)
	if distanza(b.Inchiostro, b.Sfondo) < 120 { // troppo simile: leggibilità prima di tutto
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
	x0, y0 := b.X-m, b.Y-m
	w, h := b.W+2*m, b.H+2*m
	if w <= 0 || h <= 0 {
		return nil
	}
	bd := img.Bounds()
	yTop, yBot := clamp(y0-1, bd.Min.Y, bd.Max.Y-1), clamp(y0+h, bd.Min.Y, bd.Max.Y-1)
	riga := func(y int) []color.RGBA {
		out := make([]color.RGBA, w)
		for i := 0; i < w; i++ {
			var r, g, bl, n int
			for k := -3; k <= 3; k++ { // media su 7 pixel: niente pezzi di lettere
				x := clamp(x0+i+k, bd.Min.X, bd.Max.X-1)
				c := img.RGBAAt(x, y)
				if distanza(c, b.Inchiostro) < distanza(c, b.Sfondo) {
					continue // pixel che sembra testo: non lo uso
				}
				r, g, bl, n = r+int(c.R), g+int(c.G), bl+int(c.B), n+1
			}
			if n == 0 {
				out[i] = b.Sfondo
			} else {
				out[i] = color.RGBA{uint8(r / n), uint8(g / n), uint8(bl / n), 255}
			}
		}
		return out
	}
	sopra, sotto := riga(yTop), riga(yBot)
	p := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		t := float64(y) / float64(max(1, h-1))
		for x := 0; x < w; x++ {
			a, c := sopra[x], sotto[x]
			p.SetRGBA(x, y, color.RGBA{
				uint8(float64(a.R)*(1-t) + float64(c.R)*t),
				uint8(float64(a.G)*(1-t) + float64(c.G)*t),
				uint8(float64(a.B)*(1-t) + float64(c.B)*t), 255})
		}
	}
	return p
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
	for k := 0; k < g; k++ {
		var em []float64
		var rr, gg, bb []int
		for i := range b {
			if gruppo[i] == k {
				em = append(em, b[i].Em)
				rr, gg, bb = append(rr, int(b[i].Inchiostro.R)), append(gg, int(b[i].Inchiostro.G)), append(bb, int(b[i].Inchiostro.B))
			}
		}
		if len(em) < 2 {
			continue
		}
		sort.Float64s(em)
		sort.Ints(rr)
		sort.Ints(gg)
		sort.Ints(bb)
		m := len(em) / 2
		for i := range b {
			if gruppo[i] == k {
				b[i].Em = em[m]
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
func ricolora(img *image.RGBA, bl []Blocco) ([]Blocco, bool) {
	out := make([]Blocco, len(bl))
	copy(out, bl)
	cambiato := false
	for i, b := range out {
		if b.Inchiostro == coloreTestoFascia && b.Sfondo == coloreFascia {
			continue // fascia dei video: resta com'è
		}
		c := b
		coloriBlocco(img, &c)
		if distanza(c.Sfondo, b.Sfondo) > 40 || distanza(c.Inchiostro, b.Inchiostro) > 90 {
			b.Sfondo, b.Inchiostro = c.Sfondo, c.Inchiostro
			b.Fondo = fondoRicostruito(img, b)
			out[i] = b
			cambiato = true
		}
	}
	return out, cambiato
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
