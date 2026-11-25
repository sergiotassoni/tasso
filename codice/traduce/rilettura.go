package traduce

// Seconda lettura delle righe incerte (tutti i giochi). Le scritte dei giochi hanno caratteri
// stretti, spaziati, con bordi e frange di colore: la prima lettura a grandezza naturale a volte
// sbaglia. Le righe con voto letturaDubbia si ritagliano, si ingrandiscono 2–4 volte e si
// preparano in tre modi: grigio con contrasto, grigio rovesciato (scuro su chiaro diventa chiaro
// su scuro e viceversa) e "solo le lettere" (soglia sul colore delle lettere, nero su bianco).
// Tutti i pezzi vanno in UN'immagine sola (un mosaico) letta con una sola chiamata all'OCR, e per
// ogni riga vince la lettura più affidabile. Le righe uguali (stessi pixel) non si rileggono:
// il risultato resta in memoria.

import (
	"image"
	"image/color"
	"sort"
	"strings"
	"sync"
)

// pezzoRilettura: una riga incerta da rileggere.
type pezzoRilettura struct {
	riga   int             // indice della riga nell'elenco dell'OCR (-1: zona non letta)
	rett   image.Rectangle // dove sta nella cattura (con un po' di margine)
	k      int             // ingrandimento
	chiave string          // impronta dei pixel (per la memoria delle riletture)
	tipo   int             // pezzoIncerto, pezzoIncompleto, pezzoNonLetto (seconda_lettura.go)
	ingoia []int           // righe lette a parte sulla stessa linea, dentro il riquadro allargato
}

// tipi di pezzi da rileggere
const (
	pezzoIncerto    = iota // riga letta male: vince la lettura più affidabile
	pezzoIncompleto        // riga con una parola non letta accanto: rilettura allargata
	pezzoNonLetto          // zona con la forma di una scritta dove l'OCR non ha letto niente
)

// massimi della seconda lettura per schermata (il mosaico resta una chiamata sola all'OCR)
const (
	maxPezziIncerti    = 8
	maxPezziIncompleti = 4
	maxZoneNonLette    = 3
)

// variantiRiga: le tre preparazioni del ritaglio (già in grigio), ingrandite k volte.
func variantiRiga(rit *image.RGBA, k int) []*image.Gray {
	g := ingrandisci(grigio(rit), k)
	contrasto(g)
	inv := image.NewGray(g.Bounds())
	for i, v := range g.Pix {
		inv.Pix[i] = 255 - v
	}
	// solo le lettere: colori dello sfondo (bordo) e delle lettere (i più lontani dallo sfondo),
	// ogni pixel diventa nero se è più vicino alle lettere, bianco se allo sfondo
	b := Blocco{X: 0, Y: 0, W: rit.Bounds().Dx(), H: rit.Bounds().Dy()}
	riquadro := rit.Bounds()
	// coloriBlocco guarda il bordo 3 pixel fuori dal riquadro: qui il ritaglio ha già il margine
	b.X, b.Y, b.W, b.H = riquadro.Min.X+3, riquadro.Min.Y+3, riquadro.Dx()-6, riquadro.Dy()-6
	if b.W > 4 && b.H > 4 {
		coloriBloccoSemplice(rit, &b)
	}
	sol := image.NewGray(rit.Bounds())
	for y := riquadro.Min.Y; y < riquadro.Max.Y; y++ {
		for x := riquadro.Min.X; x < riquadro.Max.X; x++ {
			c := rit.RGBAAt(x, y)
			v := uint8(255)
			if distanza(c, b.Inchiostro) < distanza(c, b.Sfondo) {
				v = 0
			}
			sol.SetGray(x, y, color.Gray{Y: v})
		}
	}
	return []*image.Gray{g, inv, ingrandisci(sol, k)}
}

// coloriBloccoSemplice: sfondo (colore più comune sul bordo) e inchiostro (i pixel interni più
// lontani dallo sfondo), senza i calcoli di contorno e grassetto di coloriBlocco.
func coloriBloccoSemplice(img *image.RGBA, b *Blocco) {
	r := image.Rect(b.X-3, b.Y-3, b.X+b.W+3, b.Y+b.H+3).Intersect(img.Bounds())
	conta := map[[3]uint8]int{}
	var somma = map[[3]uint8][4]int{}
	agg := func(x, y int) {
		c := img.RGBAAt(x, y)
		k := [3]uint8{c.R >> 4, c.G >> 4, c.B >> 4}
		conta[k]++
		s := somma[k]
		somma[k] = [4]int{s[0] + int(c.R), s[1] + int(c.G), s[2] + int(c.B), s[3] + 1}
	}
	for x := r.Min.X; x < r.Max.X; x++ {
		agg(x, r.Min.Y)
		agg(x, r.Max.Y-1)
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		agg(r.Min.X, y)
		agg(r.Max.X-1, y)
	}
	var meglio [3]uint8
	n := -1
	for k, v := range conta {
		if v > n {
			meglio, n = k, v
		}
	}
	s := somma[meglio]
	if s[3] == 0 {
		return
	}
	b.Sfondo = color.RGBA{uint8(s[0] / s[3]), uint8(s[1] / s[3]), uint8(s[2] / s[3]), 255}
	var px []color.RGBA
	in := image.Rect(b.X, b.Y, b.X+b.W, b.Y+b.H).Intersect(img.Bounds())
	for y := in.Min.Y; y < in.Max.Y; y++ {
		for x := in.Min.X; x < in.Max.X; x++ {
			px = append(px, img.RGBAAt(x, y))
		}
	}
	if len(px) == 0 {
		return
	}
	sort.Slice(px, func(i, j int) bool { return distanza(px[i], b.Sfondo) > distanza(px[j], b.Sfondo) })
	k := max(1, len(px)/10)
	var ir, ig, ib int
	for _, c := range px[:k] {
		ir, ig, ib = ir+int(c.R), ig+int(c.G), ib+int(c.B)
	}
	b.Inchiostro = color.RGBA{uint8(ir / k), uint8(ig / k), uint8(ib / k), 255}
}

// componiMosaico: i pezzi uno sotto l'altro, con 24 pixel di spazio (del colore del bordo di ogni
// pezzo, così l'OCR non vede righe finte). Restituisce dove sta ogni pezzo.
func componiMosaico(pezzi []*image.Gray) (*image.Gray, []image.Rectangle) {
	const spazio = 24
	w, h := 0, spazio
	for _, p := range pezzi {
		w = max(w, p.Bounds().Dx())
		h += p.Bounds().Dy() + spazio
	}
	w += 2 * spazio
	out := image.NewGray(image.Rect(0, 0, w, h))
	var dove []image.Rectangle
	y := spazio
	for _, p := range pezzi {
		pb := p.Bounds()
		// colore del bordo del pezzo: riempio la sua fascia (spazi compresi) con quello
		bordo := p.GrayAt(pb.Min.X, pb.Min.Y).Y
		for yy := y - spazio/2; yy < y+pb.Dy()+spazio/2 && yy < h; yy++ {
			for xx := 0; xx < w; xx++ {
				out.Pix[yy*out.Stride+xx] = bordo
			}
		}
		for yy := 0; yy < pb.Dy(); yy++ {
			copy(out.Pix[(y+yy)*out.Stride+spazio:], p.Pix[yy*p.Stride:yy*p.Stride+pb.Dx()])
		}
		dove = append(dove, image.Rect(spazio, y, spazio+pb.Dx(), y+pb.Dy()))
		y += pb.Dy() + spazio
	}
	return out, dove
}

// assegnaMosaico: le parole lette nel mosaico, divise per pezzo (centro della parola dentro il
// pezzo), con le coordinate relative al pezzo.
func assegnaMosaico(righe []RigaOCR, dove []image.Rectangle) [][]ParolaOCR {
	out := make([][]ParolaOCR, len(dove))
	for _, r := range righe {
		for _, w := range r.Parole {
			cx, cy := w.X+w.W/2, w.Y+w.H/2
			for i, d := range dove {
				if cx >= float64(d.Min.X) && cx < float64(d.Max.X) && cy >= float64(d.Min.Y) && cy < float64(d.Max.Y) {
					w2 := w
					w2.X -= float64(d.Min.X)
					w2.Y -= float64(d.Min.Y)
					out[i] = append(out[i], w2)
					break
				}
			}
		}
	}
	for i := range out {
		sort.SliceStable(out[i], func(a, b int) bool { return out[i][a].X < out[i][b].X })
	}
	return out
}

// testoParole: il testo delle parole in fila.
func testoParole(p []ParolaOCR) string {
	var t []string
	for _, w := range p {
		t = append(t, w.T)
	}
	return strings.Join(t, " ")
}

// migliorLettura: tra le letture di una riga (la prima e quelle nuove), la più affidabile; a pari
// voto quella con meno parole sospette. Restituisce l'indice.
func migliorLettura(letture []string) int {
	best, bv, bs := 0, -1, 1<<30
	for i, t := range letture {
		if strings.TrimSpace(t) == "" {
			continue
		}
		c, v := correggiRigaOCR(t)
		_, f, r := valutaParole(c)
		sosp := f*2 + r
		if v > bv || (v == bv && sosp < bs) {
			best, bv, bs = i, v, sosp
		}
	}
	return best
}

// memoria delle riletture: impronta dei pixel della riga -> parole rilette (relative al riquadro)
var (
	muRiletture sync.Mutex
	riletture   = map[string][]ParolaOCR{}
)

// improntaRiga: i pixel della riga in breve (per non rileggere la stessa riga a ogni giro).
func improntaRiga(img *image.RGBA, r image.Rectangle) string {
	rit := ritaglioScritta(img, r)
	var b strings.Builder
	for _, v := range rit {
		b.WriteByte('a' + v>>4)
	}
	b.WriteString(r.Size().String())
	return b.String()
}

// pezziDaRileggere: le righe incerte (al massimo maxPezzi), con il loro riquadro e l'ingrandimento.
func pezziDaRileggere(img *image.RGBA, righe []RigaOCR, f float64, maxPezzi int) []pezzoRilettura {
	var out []pezzoRilettura
	for i, r := range righe {
		if len(out) >= maxPezzi {
			break
		}
		t := testoParole(r.Parole)
		if !strings.ContainsFunc(t, func(c rune) bool { return c >= 'A' && c <= 'z' }) {
			continue
		}
		if _, v := correggiRigaOCR(t); v != letturaDubbia {
			continue
		}
		x0, y0, x1, y1, h := rettRiga(r, f)
		if h < 6 || h > 120 {
			continue
		}
		mx, my := int(h*0.6+0.5), int(h*0.35+0.5)
		rett := image.Rect(x0-mx, y0-my, x1+mx, y1+my).Intersect(img.Bounds())
		if rett.Empty() {
			continue
		}
		// lettere alte almeno ~48 pixel: 2, 3 o 4 volte
		k := clamp(int(48/h+0.99), 2, 4)
		for k > 1 && rett.Dx()*k > 3800 {
			k--
		}
		out = append(out, pezzoRilettura{riga: i, rett: rett, k: k, chiave: improntaRiga(img, rett)})
	}
	return out
}

// rileggiIncerteCon: rilegge le righe incerte con leggi (l'OCR di Windows, o uno finto nei test).
// Le righe migliorate prendono il posto delle vecchie (stesse coordinate della prima lettura, scala f).
func rileggiIncerteCon(img *image.RGBA, righe []RigaOCR, f float64, leggi func(*image.Gray, []image.Rectangle) ([]RigaOCR, error)) []RigaOCR {
	if img == nil || len(righe) == 0 || leggi == nil {
		return righe
	}
	if f <= 0 {
		f = 1
	}
	pezzi := pezziDaRileggere(img, righe, f, maxPezziIncerti)
	gia := map[int]bool{}
	for _, p := range pezzi {
		gia[p.riga] = true
	}
	inc := pezziIncompleti(img, righe, f, gia, maxPezziIncompleti)
	pezzi = append(pezzi, inc...)
	for _, z := range zoneTestoNonLetto(img, righe, f, maxZoneNonLette) {
		sovrapposta := false
		for _, p := range inc {
			if s := z.Intersect(p.rett); s.Dx()*s.Dy()*2 >= z.Dx()*z.Dy() {
				sovrapposta = true
			}
		}
		if sovrapposta {
			continue
		}
		k := clamp(int(48/float64(max(1, z.Dy()*6/10))+0.99), 2, 4)
		for k > 1 && z.Dx()*k > 3800 {
			k--
		}
		pezzi = append(pezzi, pezzoRilettura{riga: -1, rett: z, k: k, chiave: "zona" + improntaRiga(img, z), tipo: pezzoNonLetto})
	}
	if len(pezzi) == 0 {
		return righe
	}
	out := append([]RigaOCR(nil), righe...)
	var nuove []RigaOCR // righe trovate nelle zone non lette
	applica := func(p pezzoRilettura, parole []ParolaOCR) {
		if len(parole) == 0 {
			return
		}
		r := rigaDaRilette(parole, p, f)
		switch p.tipo {
		case pezzoNonLetto:
			nuove = append(nuove, r)
		default:
			out[p.riga] = r
			for _, j := range p.ingoia {
				out[j] = RigaOCR{} // ora fa parte della riga riletta
			}
		}
	}
	var daLeggere []pezzoRilettura
	var immagini []*image.Gray
	muRiletture.Lock()
	for _, p := range pezzi {
		if parole, ok := riletture[p.chiave]; ok {
			applica(p, parole)
			continue
		}
		daLeggere = append(daLeggere, p)
	}
	muRiletture.Unlock()
	altezza := 0
	var scelti []pezzoRilettura
	for _, p := range daLeggere {
		rit := image.NewRGBA(image.Rect(0, 0, p.rett.Dx(), p.rett.Dy()))
		for y := 0; y < p.rett.Dy(); y++ {
			copy(rit.Pix[y*rit.Stride:y*rit.Stride+p.rett.Dx()*4], img.Pix[img.PixOffset(p.rett.Min.X, p.rett.Min.Y+y):img.PixOffset(p.rett.Max.X, p.rett.Min.Y+y)])
		}
		v := variantiRiga(rit, p.k)
		h := 0
		for _, g := range v {
			h += g.Bounds().Dy() + 24
		}
		if altezza+h > 3800 { // il mosaico non deve superare il limite dell'OCR
			break
		}
		altezza += h
		immagini = append(immagini, v...)
		scelti = append(scelti, p)
	}
	if len(scelti) == 0 {
		return conNuove(out, nuove)
	}
	mosaico, dove := componiMosaico(immagini)
	if osservaPezzi != nil {
		osservaPezzi(scelti)
	}
	lette, err := leggi(mosaico, dove)
	if err != nil {
		return conNuove(out, nuove)
	}
	perPezzo := assegnaMosaico(lette, dove)
	for n, p := range scelti {
		var candidati [][]ParolaOCR
		for v := 0; v < 3; v++ {
			candidati = append(candidati, perPezzo[n*3+v])
		}
		tieni := sceltaRilettura(p, righe, candidati)
		muRiletture.Lock()
		if len(riletture) > 600 {
			riletture = map[string][]ParolaOCR{}
		}
		riletture[p.chiave] = tieni // anche "niente di meglio" (vuoto): non rileggo a ogni giro
		muRiletture.Unlock()
		applica(p, tieni)
	}
	return conNuove(out, nuove)
}

// osservaPezzi: solo per i test e il banco senza Windows (sapere quali pezzi sono nel mosaico).
var osservaPezzi func([]pezzoRilettura)

// conNuove: le righe con quelle trovate nelle zone non lette in fondo (le righe inglobate in
// una riga riletta restano vuote: non contano).
func conNuove(out, nuove []RigaOCR) []RigaOCR {
	return append(out, nuove...)
}

// sceltaRilettura: tra le tre letture del pezzo, quella da tenere (nil = niente di meglio).
func sceltaRilettura(p pezzoRilettura, righe []RigaOCR, candidati [][]ParolaOCR) []ParolaOCR {
	switch p.tipo {
	case pezzoIncompleto:
		// la prima lettura: la riga e i pezzi inglobati, da sinistra a destra
		var prima []ParolaOCR
		prima = append(prima, righe[p.riga].Parole...)
		for _, j := range p.ingoia {
			prima = append(prima, righe[j].Parole...)
		}
		sort.SliceStable(prima, func(a, b int) bool { return prima[a].X < prima[b].X })
		vecchia := testoParole(prima)
		letture := []string{""}
		var buoni [][]ParolaOCR
		for _, c := range candidati {
			if t := testoParole(c); letturaPiuCompleta(vecchia, t) {
				letture = append(letture, t)
				buoni = append(buoni, c)
			}
		}
		if len(buoni) == 0 {
			return nil
		}
		return buoni[max(0, migliorLettura(letture)-1)]
	case pezzoNonLetto:
		letture := []string{""}
		var buoni [][]ParolaOCR
		for _, c := range candidati {
			if t := testoParole(c); letturaNuovaBuona(t) {
				letture = append(letture, t)
				buoni = append(buoni, c)
			}
		}
		if len(buoni) == 0 {
			return nil
		}
		return buoni[max(0, migliorLettura(letture)-1)]
	}
	letture := []string{testoParole(righe[p.riga].Parole)}
	for _, c := range candidati {
		letture = append(letture, testoParole(c))
	}
	if best := migliorLettura(letture); best > 0 {
		return candidati[best-1]
	}
	return nil
}

// rigaDaRilette: le parole rilette (pixel del pezzo ingrandito) nelle coordinate della prima
// lettura (cattura × f).
func rigaDaRilette(parole []ParolaOCR, p pezzoRilettura, f float64) RigaOCR {
	var r RigaOCR
	k := float64(p.k)
	for _, w := range parole {
		r.Parole = append(r.Parole, ParolaOCR{T: lettereOCRStrane(w.T),
			X: (w.X/k + float64(p.rett.Min.X)) * f, Y: (w.Y/k + float64(p.rett.Min.Y)) * f,
			W: w.W / k * f, H: w.H / k * f})
	}
	return r
}

// rettRiga: rettangolo della riga e altezza mediana delle parole, in pixel della cattura.
func rettRiga(r RigaOCR, f float64) (x0, y0, x1, y1 int, h float64) {
	if len(r.Parole) == 0 {
		return
	}
	a, b, c, d := 1e18, 1e18, -1e18, -1e18
	var hs []float64
	for _, w := range r.Parole {
		a, b = min(a, w.X), min(b, w.Y)
		c, d = max(c, w.X+w.W), max(d, w.Y+w.H)
		hs = append(hs, w.H/f)
	}
	sort.Float64s(hs)
	return int(a / f), int(b / f), int(c/f + 0.5), int(d/f + 0.5), hs[len(hs)/2]
}
