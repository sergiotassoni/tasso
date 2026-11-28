package traduce

// Avviso di stato della traduzione (variante 1 delle anteprime, in attesa della scelta del
// proprietario): un piccolo badge in un angolo in basso del gioco (28/11: in alto al centro
// copriva il menu di Hytale; se il mouse ci passa sopra salta nell'altro angolo), scuro semitrasparente con angoli
// arrotondati, stemma del tasso, puntino verde menta che pulsa piano mentre traduco e un testo
// breve. Compare per 2 secondi e poi sparisce (resta, se acceso, solo un puntino minuscolo).
// Non copre il gioco e non prende i clic (finestra trasparente ai clic, fuori dalle catture).
// Qui il disegno, senza Windows: il testo arriva già come maschera (coperture 0-255).

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"
)

//go:embed dati/stemma-tasso-48.png
var stemmaTassoPNG []byte

var stemmaTasso = sync.OnceValue(func() *image.RGBA {
	src, err := png.Decode(bytes.NewReader(stemmaTassoPNG))
	if err != nil {
		return nil
	}
	b := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.Set(x, y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
})

// stati del badge
const (
	statoBadgeNiente   = iota
	statoBadgeTraduco  // "Traduco…": puntino che pulsa, resta finché la prima schermata è pronta
	statoBadgeAttivo   // "Traduzione attiva · F7": 2 secondi
	statoBadgeSpento   // "Traduzione spenta": 2 secondi, puntino grigio
	statoBadgeNonGioco // "Qui non c'è un gioco": 2 secondi, puntino grigio
)

var (
	colMenta     = color.RGBA{47, 243, 166, 255}
	colGrigioPun = color.RGBA{150, 156, 170, 255}
	colFondoBdg  = color.RGBA{16, 19, 30, 205}
	colTestoBdg  = color.RGBA{240, 244, 248, 240}
)

// misureBadge: altezza, margini e lato dello stemma per una scala (1 = schermo 1080p).
func misureBadge(scala float64) (h, pad, stemma, spazio, punto int) {
	return int(34*scala + 0.5), int(12*scala + 0.5), int(20*scala + 0.5), int(8*scala + 0.5), int(10*scala + 0.5)
}

// larghezzaBadge: quanto è largo il badge con un testo largo tw pixel.
func larghezzaBadge(tw int, scala float64) int {
	_, pad, st, sp, pu := misureBadge(scala)
	return pad + st + sp + pu + sp + tw + pad
}

// copertura di un rettangolo arrotondato nel punto (x+0.5, y+0.5), con bordo morbido di un pixel.
func dentroArrotondato(x, y, w, h int, r float64) float64 {
	px, py := float64(x)+0.5, float64(y)+0.5
	cx := math.Max(r, math.Min(float64(w)-r, px))
	cy := math.Max(r, math.Min(float64(h)-r, py))
	d := math.Hypot(px-cx, py-cy) - r
	return math.Max(0, math.Min(1, 0.5-d))
}

// mescola: c sopra base con copertura a (0-1), colori NON premoltiplicati.
func mescola(base color.RGBA, c color.RGBA, a float64) color.RGBA {
	a *= float64(c.A) / 255
	if a <= 0 {
		return base
	}
	ba := float64(base.A) / 255
	oa := a + ba*(1-a)
	if oa <= 0 {
		return color.RGBA{}
	}
	f := func(cc, bb uint8) uint8 {
		return uint8(math.Round((float64(cc)*a + float64(bb)*ba*(1-a)) / oa))
	}
	return color.RGBA{f(c.R, base.R), f(c.G, base.G), f(c.B, base.B), uint8(math.Round(oa * 255))}
}

// componiBadge: il badge (colori non premoltiplicati). testo: copertura del testo (tw×th), già
// misurato con il carattere vero; pulsa 0-1 (alone del puntino); spento: puntino grigio; alfa:
// trasparenza generale (per la dissolvenza).
func componiBadge(testo []uint8, tw, th int, scala, pulsa float64, spento bool, alfa float64) *image.RGBA {
	h, pad, st, sp, pu := misureBadge(scala)
	w := larghezzaBadge(tw, scala)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	r := float64(h) / 2
	bordo := color.RGBA{colMenta.R, colMenta.G, colMenta.B, 90}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fuori := dentroArrotondato(x, y, w, h, r)
			if fuori <= 0 {
				continue
			}
			dentro := 0.0 // il bordino: la parte tra il riquadro e quello più piccolo di 1,2 pixel
			lb := 1.2 * scala
			if x >= 1 && y >= 1 {
				dentro = dentroArrotondato(x-int(lb), y-int(lb), w-2*int(lb), h-2*int(lb), r-lb)
			}
			c := mescola(color.RGBA{}, colFondoBdg, fuori)
			c = mescola(c, bordo, math.Max(0, fuori-dentro))
			img.SetRGBA(x, y, c)
		}
	}
	x := pad
	if s := stemmaTasso(); s != nil {
		// stemma rimpicciolito (media dei pixel di partenza che cadono in ogni pixel)
		sb := s.Bounds()
		y0 := (h - st) / 2
		for j := 0; j < st; j++ {
			for i := 0; i < st; i++ {
				var rr, gg, bb, aa float64
				n := 0.0
				for v := j * sb.Dy() / st; v < (j+1)*sb.Dy()/st; v++ {
					for u := i * sb.Dx() / st; u < (i+1)*sb.Dx()/st; u++ {
						c := s.RGBAAt(u, v)
						a := float64(c.A) / 255
						rr, gg, bb, aa = rr+float64(c.R)*a, gg+float64(c.G)*a, bb+float64(c.B)*a, aa+a
						n++
					}
				}
				if n == 0 || aa == 0 {
					continue
				}
				c := color.RGBA{uint8(rr / aa), uint8(gg / aa), uint8(bb / aa), 255}
				img.SetRGBA(x+i, y0+j, mescola(img.RGBAAt(x+i, y0+j), c, aa/n))
			}
		}
		x += st + sp
	}
	// puntino: alone che pulsa e punto pieno
	pc := colMenta
	if spento {
		pc, pulsa = colGrigioPun, 0
	}
	cx, cy := float64(x)+float64(pu)/2, float64(h)/2
	rp := 4 * scala
	ra := rp * (1.8 + 0.6*pulsa)
	for yy := 0; yy < h; yy++ {
		for xx := x - pu; xx < x+2*pu && xx < w; xx++ {
			if xx < 0 {
				continue
			}
			d := math.Hypot(float64(xx)+0.5-cx, float64(yy)+0.5-cy)
			if pulsa > 0 && d < ra {
				al := (1 - d/ra) * 0.45 * pulsa
				img.SetRGBA(xx, yy, mescola(img.RGBAAt(xx, yy), pc, al))
			}
			if a := math.Max(0, math.Min(1, rp+0.5-d)); a > 0 {
				img.SetRGBA(xx, yy, mescola(img.RGBAAt(xx, yy), pc, a))
			}
		}
	}
	x += pu + sp
	// testo, centrato in altezza
	ty := (h - th) / 2
	for j := 0; j < th; j++ {
		for i := 0; i < tw; i++ {
			a := float64(testo[j*tw+i]) / 255
			if a > 0 && x+i < w && ty+j >= 0 && ty+j < h {
				img.SetRGBA(x+i, ty+j, mescola(img.RGBAAt(x+i, ty+j), colTestoBdg, a))
			}
		}
	}
	if alfa < 1 {
		for i := 3; i < len(img.Pix); i += 4 {
			img.Pix[i] = uint8(float64(img.Pix[i]) * math.Max(0, alfa))
		}
	}
	return img
}

// puntinoBadge: il puntino minuscolo che resta dopo i 2 secondi (traduzione accesa).
func puntinoBadge(scala float64) *image.RGBA {
	l := max(6, int(8*scala+0.5))
	img := image.NewRGBA(image.Rect(0, 0, l, l))
	c := float64(l) / 2
	for y := 0; y < l; y++ {
		for x := 0; x < l; x++ {
			d := math.Hypot(float64(x)+0.5-c, float64(y)+0.5-c)
			if a := math.Max(0, math.Min(1, c-0.5-d+0.5)); a > 0 {
				img.SetRGBA(x, y, mescola(color.RGBA{}, color.RGBA{colMenta.R, colMenta.G, colMenta.B, 170}, a))
			}
		}
	}
	return img
}

// posizioneBadge: dove va il badge largo w e alto h nella finestra del gioco. angolo 0 = in
// basso a sinistra, 1 = in basso a destra; un po' staccato dal bordo. In alto al centro (dove
// era prima) i giochi mettono le schede dei menu.
func posizioneBadge(area Zona, w, h int, scala float64, angolo int) (int32, int32) {
	m := int(18*scala + 0.5)
	x := area.X + m
	if angolo == 1 {
		x = area.X + area.W - w - m
	}
	y := area.Y + area.H - h - m
	if x < area.X {
		x = area.X
	}
	if y < area.Y {
		y = area.Y
	}
	return int32(x), int32(y)
}

// testoBadge: il testo di ogni stato.
func testoBadge(stato int, tasto string) string {
	switch stato {
	case statoBadgeTraduco:
		return testoTasso("Traduco…", "Translating…")
	case statoBadgeAttivo:
		return testoTasso("Traduzione attiva · ", "Translation on · ") + tasto
	case statoBadgeSpento:
		return testoTasso("Traduzione spenta", "Translation off")
	case statoBadgeNonGioco:
		return testoTasso("Qui non c'è un gioco", "No game here")
	}
	return ""
}

// premoltiplica: per UpdateLayeredWindow i colori vanno moltiplicati per la trasparenza (BGRA).
func premoltiplicaBGRA(img *image.RGBA) []byte {
	out := make([]byte, len(img.Pix))
	for i := 0; i < len(img.Pix); i += 4 {
		a := uint32(img.Pix[i+3])
		out[i] = uint8(uint32(img.Pix[i+2]) * a / 255)
		out[i+1] = uint8(uint32(img.Pix[i+1]) * a / 255)
		out[i+2] = uint8(uint32(img.Pix[i]) * a / 255)
		out[i+3] = uint8(a)
	}
	return out
}
