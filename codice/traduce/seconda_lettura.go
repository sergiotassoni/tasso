package traduce

// Seconda lettura mirata (tutti i giochi), nella stessa chiamata all'OCR della rilettura delle
// righe incerte (rilettura.go), così il tempo in più è poco e solo quando serve:
//
//   - righe incomplete: accanto alla riga letta c'è inchiostro di lettere che l'OCR non ha letto
//     ("TIP:" prima di "You can sprint…", "skip" dopo "Press SPACE to", "/ 120" dopo "Ammo: 24").
//     Si rilegge la riga allargata fin dove arriva l'inchiostro: se la nuova lettura è buona e
//     più lunga prende il posto della vecchia (e dei pezzi letti a parte sulla stessa linea).
//   - scritte non lette affatto: testo chiaro su chiaro o scuro su scuro (grigio 170 su beige 228,
//     "Press Enter to continue" in un riquadro di dialogo) che l'OCR di Windows salta del tutto.
//     Cerco zone con la forma di una riga di testo (tanti bordi fitti, alte come una scritta,
//     larghe) in un fondo liscio (dentro un riquadro), lontane dalle righe lette; al massimo
//     3 per schermata. Il ritaglio con il contrasto stirato (min/max della zona) si rilegge.
//
// Le parti di questo file non usano Windows: si provano su Linux con immagini finte.

import (
	"image"
	"sort"
	"strings"
	"unicode"
)

// zoneTestoNonLetto: zone (pixel della cattura) che sembrano una riga di testo ma dove l'OCR non
// ha letto niente. righe e f: la prima lettura (scala f). Al massimo maxZone, le più grandi.
func zoneTestoNonLetto(img *image.RGBA, righe []RigaOCR, f float64, maxZone int) []image.Rectangle {
	if img == nil || maxZone <= 0 {
		return nil
	}
	if f <= 0 {
		f = 1
	}
	bd := img.Bounds()
	const cs = 6 // lato della cella (pixel)
	gw, gh := bd.Dx()/cs, bd.Dy()/cs
	if gw < 8 || gh < 4 {
		return nil
	}
	// 1) bordi orizzontali fitti per cella (una riga di pixel su due: basta per le lettere)
	bordi := make([]uint8, gw*gh)
	lum := func(x, y int) int {
		i := img.PixOffset(x, y)
		p := img.Pix[i : i+3 : i+3]
		return (int(p[0])*299 + int(p[1])*587 + int(p[2])*114) / 1000
	}
	for cy := 0; cy < gh; cy++ {
		for yy := 0; yy < cs; yy += 2 {
			y := bd.Min.Y + cy*cs + yy
			prima, ora := lum(bd.Min.X, y), lum(bd.Min.X+1, y)
			for x := bd.Min.X + 1; x < bd.Min.X+gw*cs-1; x++ {
				dopo := lum(x+1, y)
				if d := dopo - prima; d >= 14 || d <= -14 {
					c := (x - bd.Min.X) / cs
					if bordi[cy*gw+c] < 255 {
						bordi[cy*gw+c]++
					}
				}
				prima, ora = ora, dopo
			}
		}
	}
	const minBordi = 3 // su 18 pixel guardati: la cella ha pezzi di lettere
	segnata := make([]bool, gw*gh)
	for i, v := range bordi {
		segnata[i] = v >= minBordi
	}
	// bordi lunghi in verticale (il lato di un riquadro, di un edificio): più alti di qualsiasi
	// riga di testo, non sono lettere (e unirebbero la riga vicina in una zona troppo alta)
	lettera := append([]bool(nil), segnata...)
	const maxAlte = 14 // celle: 84 pixel
	for cx := 0; cx < gw; cx++ {
		for cy := 0; cy < gh; {
			if !segnata[cy*gw+cx] {
				cy++
				continue
			}
			k := cy
			for k < gh && segnata[k*gw+cx] {
				k++
			}
			if k-cy > maxAlte {
				for y := cy; y < k; y++ {
					lettera[y*gw+cx] = false
				}
			}
			cy = k
		}
	}
	// 2) le righe già lette (con un margine) non contano
	coperta := make([]bool, gw*gh)
	for _, r := range righe {
		x0, y0, x1, y1, h := rettRiga(r, f)
		if len(r.Parole) == 0 {
			continue
		}
		mx, my := int(h*0.6+0.5)+cs, int(h*0.35+0.5)+cs
		for cy := max(0, (y0-my-bd.Min.Y)/cs); cy <= min(gh-1, (y1+my-bd.Min.Y)/cs); cy++ {
			for cx := max(0, (x0-mx-bd.Min.X)/cs); cx <= min(gw-1, (x1+mx-bd.Min.X)/cs); cx++ {
				coperta[cy*gw+cx] = true
			}
		}
	}
	// 3) celle di testo: segnate e non coperte; i buchi corti tra due lettere si chiudono
	testo := make([]bool, gw*gh)
	for cy := 0; cy < gh; cy++ {
		ultima := -100
		for cx := 0; cx < gw; cx++ {
			i := cy*gw + cx
			if !lettera[i] || coperta[i] {
				continue
			}
			if cx-ultima <= 3 {
				for k := ultima + 1; k < cx; k++ {
					if !coperta[cy*gw+k] {
						testo[cy*gw+k] = true
					}
				}
			}
			testo[i] = true
			ultima = cx
		}
	}
	// 4) gruppi di celle unite (4 vicine); un gruppo alto (la riga toccata dal lato di un disegno)
	// si divide nelle fasce di righe di celle piene: ogni fascia è una riga di testo possibile
	type zona struct {
		r     image.Rectangle // in celle
		celle int
	}
	var zone []zona
	visto := make([]bool, gw*gh)
	coda := make([]int, 0, 256)
	var gruppo []int
	for i0 := range testo {
		if !testo[i0] || visto[i0] {
			continue
		}
		coda = append(coda[:0], i0)
		gruppo = gruppo[:0]
		visto[i0] = true
		rg := image.Rect(i0%gw, i0/gw, i0%gw+1, i0/gw+1)
		for len(coda) > 0 {
			i := coda[len(coda)-1]
			coda = coda[:len(coda)-1]
			gruppo = append(gruppo, i)
			cx, cy := i%gw, i/gw
			rg = rg.Union(image.Rect(cx, cy, cx+1, cy+1))
			for _, n := range [4][2]int{{cx - 1, cy}, {cx + 1, cy}, {cx, cy - 1}, {cx, cy + 1}} {
				if n[0] < 0 || n[1] < 0 || n[0] >= gw || n[1] >= gh {
					continue
				}
				j := n[1]*gw + n[0]
				if testo[j] && !visto[j] {
					visto[j] = true
					coda = append(coda, j)
				}
			}
		}
		if len(gruppo) < 6 {
			continue
		}
		// celle per riga del gruppo
		perRiga := make([]int, rg.Dy())
		for _, i := range gruppo {
			perRiga[i/gw-rg.Min.Y]++
		}
		massimo := 0
		for _, n := range perRiga {
			massimo = max(massimo, n)
		}
		soglia := max(4, massimo/4)
		for y := 0; y < len(perRiga); {
			if perRiga[y] < soglia {
				y++
				continue
			}
			k := y
			for k < len(perRiga) && perRiga[k] >= soglia {
				k++
			}
			z := zona{r: image.Rectangle{Min: image.Pt(gw, 0), Max: image.Pt(-1, 0)}}
			for _, i := range gruppo {
				if cy := i/gw - rg.Min.Y; cy >= y && cy < k {
					cx := i % gw
					z.r.Min.X, z.r.Max.X = min(z.r.Min.X, cx), max(z.r.Max.X, cx+1)
					z.celle++
				}
			}
			z.r.Min.Y, z.r.Max.Y = rg.Min.Y+y, rg.Min.Y+k
			zone = append(zone, z)
			y = k
		}
	}
	// 5) forma di una riga di testo in un fondo liscio
	var buone []image.Rectangle
	var punti []int
	for _, z := range zone {
		wC, hC := z.r.Dx(), z.r.Dy()
		hPx, wPx := hC*cs, wC*cs
		if hPx < 12 || hPx > 72 || wPx < 48 || wPx < 3*hPx || wPx > bd.Dx()*8/10 {
			continue
		}
		if z.celle*100 < wC*hC*40 {
			continue // celle sparse: rumore o disegno, non una riga di lettere
		}
		// fondo liscio attorno (dentro un riquadro): poche celle con bordi nell'anello attorno
		anello, conBordi := 0, 0
		for cy := z.r.Min.Y - 2; cy < z.r.Max.Y+2; cy++ {
			for cx := z.r.Min.X - 2; cx < z.r.Max.X+2; cx++ {
				if cy < 0 || cx < 0 || cy >= gh || cx >= gw {
					continue
				}
				if (image.Point{cx, cy}).In(z.r) {
					continue
				}
				i := cy*gw + cx
				if coperta[i] {
					continue
				}
				anello++
				if segnata[i] {
					conBordi++
				}
			}
		}
		if anello < 8 || conBordi*100 > anello*15 {
			continue
		}
		// colonne di lettere: in una riga di testo i bordi sono in tante colonne diverse, non in
		// una o due righe di celle sole (una linea orizzontale, il bordo di un riquadro)
		righePiene := 0
		for cy := z.r.Min.Y; cy < z.r.Max.Y; cy++ {
			n := 0
			for cx := z.r.Min.X; cx < z.r.Max.X; cx++ {
				if segnata[cy*gw+cx] {
					n++
				}
			}
			if n*100 >= wC*30 {
				righePiene++
			}
		}
		if righePiene < 2 && hC > 2 {
			continue
		}
		r := image.Rect(z.r.Min.X*cs, z.r.Min.Y*cs, z.r.Max.X*cs, z.r.Max.Y*cs).Add(bd.Min)
		// tratti di lettere: lungo una riga di pixel della zona i bordi sono tanti e fitti (le aste
		// delle lettere), non uno o due (il lato storto di un disegno)
		if !trattiFitti(r, lum) {
			continue
		}
		// margine largo: le celle piene sono la parte centrale delle lettere (le maiuscole e le
		// gambe escono sopra e sotto)
		mx, my := max(8, hPx), max(cs+2, hPx*6/10)
		buone = append(buone, image.Rect(r.Min.X-mx, r.Min.Y-my, r.Max.X+mx, r.Max.Y+my).Intersect(bd))
		punti = append(punti, z.celle)
	}
	ord := make([]int, len(buone))
	for i := range ord {
		ord[i] = i
	}
	sort.SliceStable(ord, func(a, b int) bool { return punti[ord[a]] > punti[ord[b]] })
	var out []image.Rectangle
	for _, i := range ord {
		if len(out) >= maxZone {
			break
		}
		out = append(out, buone[i])
	}
	return out
}

// trattiFitti: in tre righe di pixel della zona (a un terzo, metà, due terzi dell'altezza) i
// pezzi di bordo sono in media almeno 6 e almeno uno ogni 1,5 altezze di larghezza.
func trattiFitti(r image.Rectangle, lum func(x, y int) int) bool {
	tot := 0
	for _, q := range []int{35, 50, 65} {
		y := r.Min.Y + r.Dy()*q/100
		dentro := false
		for x := r.Min.X + 1; x < r.Max.X-1; x++ {
			d := lum(x+1, y) - lum(x-1, y)
			bordo := d >= 14 || d <= -14
			if bordo && !dentro {
				tot++
			}
			dentro = bordo
		}
	}
	media := tot / 3
	return media >= 6 && float64(media) >= float64(r.Dx())/(1.5*float64(r.Dy()))
}

// estendiSuInchiostro: fin dove arriva l'inchiostro di lettere attaccato alla riga, a sinistra e a
// destra (buchi fino a 1,2 em: lo spazio tra due parole; al massimo 14 em per lato). Restituisce
// le x di inizio e fine della riga allargata.
func estendiSuInchiostro(img *image.RGBA, b Blocco) (int, int) {
	coloriBloccoSemplice(img, &b)
	b.HaContorno = false
	em := b.Em
	if em <= 0 {
		em = float64(b.H) * 1.1
	}
	maxExt := int(14*em + 0.5)
	buco := int(1.2*em + 0.5)
	minCol := max(2, b.H*15/100)
	estendi := func(col []int, verso int) int {
		ultimo, vuote := 0, 0
		for k := 0; k < len(col); k++ {
			i := k
			if verso < 0 {
				i = len(col) - 1 - k
			}
			if col[i] >= minCol {
				ultimo, vuote = k+1, 0
				continue
			}
			if vuote++; vuote > buco {
				break
			}
		}
		return ultimo
	}
	bd := img.Bounds()
	x1 := b.X + b.W + estendi(colonneInchiostro(img, &b, b.X+b.W, min(bd.Max.X, b.X+b.W+maxExt)), 1)
	da := max(bd.Min.X, b.X-maxExt)
	x0 := b.X - estendi(colonneInchiostro(img, &b, da, b.X), -1)
	return x0, x1
}

// pezziIncompleti: le righe della prima lettura con una parola non letta accanto (al massimo
// maxPezzi), con il riquadro allargato fino all'inchiostro e le altre righe lette sulla stessa
// linea che il riquadro allargato ingloba (si rileggono insieme, poi diventano una riga sola).
func pezziIncompleti(img *image.RGBA, righe []RigaOCR, f float64, gia map[int]bool, maxPezzi int) []pezzoRilettura {
	if img == nil || maxPezzi <= 0 {
		return nil
	}
	blocchi := make([]Blocco, len(righe))
	for i, r := range righe {
		if len(r.Parole) > 0 {
			blocchi[i] = bloccoDaParole(r.Parole, f)
		}
	}
	var out []pezzoRilettura
	preso := map[int]bool{}
	for i, r := range righe {
		if len(out) >= maxPezzi {
			break
		}
		if gia[i] || preso[i] || len(r.Parole) == 0 {
			continue
		}
		t := testoParole(r.Parole)
		if !strings.ContainsFunc(t, unicode.IsLetter) {
			continue
		}
		if c, v := correggiRigaOCR(t); !daTradurre(c) || (v == letturaCatalog && !strings.ContainsFunc(c, unicode.IsDigit)) {
			// numeri soli, e frasi pronte intere (come controllaCompleta: le frecce del selettore
			// non contano). I contatori ("Ammo: 24" senza "/ 120") si rileggono.
			continue
		}
		b := blocchi[i]
		if b.H < 8 || b.H > 120 {
			continue
		}
		if !scrittaContinuaTra(img, b, blocchi) {
			continue
		}
		x0, x1 := estendiSuInchiostro(img, b)
		if x1-x0 <= b.W+2 {
			continue
		}
		// le righe lette a parte sulla stessa linea dentro il riquadro allargato
		var ingoia []int
		for j, c := range blocchi {
			if j == i || len(righe[j].Parole) == 0 || gia[j] || preso[j] || !stessaLinea(b, c) {
				continue
			}
			if c.X <= x1+2 && c.X+c.W >= x0-2 { // tocca il riquadro allargato: si rilegge insieme
				ingoia = append(ingoia, j)
				x0, x1 = min(x0, c.X), max(x1, c.X+c.W)
			}
		}
		h := float64(b.H)
		y0, y1 := b.Y, b.Y+b.H
		for _, j := range ingoia {
			y0, y1 = min(y0, blocchi[j].Y), max(y1, blocchi[j].Y+blocchi[j].H)
		}
		mx, my := int(h*0.6+0.5), int(h*0.35+0.5)
		rett := image.Rect(x0-mx, y0-my, x1+mx, y1+my).Intersect(img.Bounds())
		if rett.Empty() {
			continue
		}
		k := clamp(int(48/h+0.99), 2, 4)
		for k > 1 && rett.Dx()*k > 3800 {
			k--
		}
		preso[i] = true
		for _, j := range ingoia {
			preso[j] = true
		}
		out = append(out, pezzoRilettura{riga: i, rett: rett, k: k, chiave: "inc" + improntaRiga(img, rett), tipo: pezzoIncompleto, ingoia: ingoia})
	}
	return out
}

// letturaPiuCompleta: la rilettura di una riga incompleta vale se è buona, più lunga della prima
// lettura e contiene quasi tutte le sue parole (non un'altra scritta letta per sbaglio).
func letturaPiuCompleta(prima, nuova string) bool {
	c, v := correggiRigaOCR(nuova)
	if v == letturaDubbia || len([]rune(c)) <= len([]rune(strings.TrimSpace(prima))) {
		return false
	}
	vecchie := paroleNude(prima)
	if len(vecchie) == 0 {
		return true
	}
	// un prezzo o un numero da solo non continua con delle parole ("$600" -> "$600 one": era la
	// riga sotto, prova 26/11 15:56)
	if !strings.ContainsFunc(prima, unicode.IsLetter) {
		return false
	}
	// le parole aggiunte sono solo paroline ("Oh, by" -> "Oh, by on this"): è un pezzo di
	// un'altra scritta, non il seguito della frase
	gia := map[string]bool{}
	for _, w := range vecchie {
		gia[w] = true
	}
	aggiunte, piene := 0, 0
	for _, w := range paroleNude(c) {
		if gia[w] {
			continue
		}
		aggiunte++
		if !parolina[w] {
			piene++
		}
	}
	if aggiunte > 0 && piene == 0 {
		return false
	}
	nuove := map[string]bool{}
	for _, w := range paroleNude(c) {
		nuove[w] = true
	}
	for _, w := range paroleNude(nuova) {
		nuove[w] = true
	}
	ok := 0
	for _, w := range vecchie {
		if nuove[w] {
			ok++
		}
	}
	return ok*100 >= len(vecchie)*60
}

// parolina: parole vuote dell'inglese (articoli, preposizioni, pronomi, numeri in lettere): da
// sole non completano una frase.
var parolina = map[string]bool{"a": true, "an": true, "the": true, "of": true, "on": true, "in": true, "at": true,
	"by": true, "to": true, "for": true, "from": true, "with": true, "this": true, "that": true, "these": true,
	"those": true, "it": true, "is": true, "and": true, "or": true, "as": true, "so": true, "up": true, "one": true,
	"two": true, "no": true, "oh": true, "be": true, "do": true, "if": true, "me": true, "my": true, "we": true,
	"us": true, "he": true, "she": true, "his": true, "her": true, "you": true, "your": true, "its": true, "our": true,
	"them": true, "they": true, "him": true, "out": true, "off": true, "into": true, "onto": true, "than": true}

// letturaNuovaBuona: la lettura di una zona non letta vale se è testo vero (non spazzatura).
func letturaNuovaBuona(t string) bool {
	c, v := correggiRigaOCR(t)
	if v == letturaDubbia || !daTradurre(c) {
		return false
	}
	return v == letturaCatalog || len(paroleNude(c)) >= 2
}

// paroleNude: le parole minuscole senza punteggiatura (di 2+ lettere).
func paroleNude(s string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\'' }) {
		if len([]rune(w)) >= 2 {
			out = append(out, w)
		}
	}
	return out
}
