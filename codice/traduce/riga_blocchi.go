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
				if c.X < a.X || !stessaLinea(a, c) {
					continue
				}
				buco := c.X - (a.X + a.W)
				if buco < -min(a.H, c.H)/2 || buco > sogliaParole(a, c) {
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
		if len(gruppi) < 2 {
			out = append(out, bl)
			continue
		}
		for _, g := range gruppi {
			out = append(out, bloccoDaPosizioni(g, bl))
		}
	}
	return out
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
	// colori semplici (sfondo del bordo, inchiostro più lontano): qui serve solo distinguere le
	// lettere dallo sfondo, anche quando il contorno o l'ombra confondono i colori del disegno
	coloriBloccoSemplice(img, &b)
	b.HaContorno = false
	if !inchiostroCredibile(img, &b) {
		return false // non so separare le lettere dallo sfondo: nessuna prova di una parola tagliata
	}
	em := b.Em
	if em <= 0 {
		em = float64(b.H) * 1.1
	}
	largo := int(1.3*em + 0.5)
	minCol := max(2, b.H*15/100)   // una colonna "piena" ha inchiostro in almeno il 15% dell'altezza
	minRun := max(2, int(em*0.08)) // e una lettera è larga almeno un dodicesimo di em (anche i caratteri a pixel)
	banda := max(1, b.H*85/100-b.H*15/100)
	tutta := max(minCol, banda*90/100)            // colonna piena da cima a fondo della fascia
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
				if col[i] >= tutta {
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
	dx := colonneInchiostro(img, &b, b.X+b.W, b.X+b.W+largo)
	if controlla(dx, b.X+b.W, 1) {
		return true
	}
	sx := colonneInchiostro(img, &b, b.X-largo, b.X)
	return controlla(sx, b.X-largo, -1)
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
