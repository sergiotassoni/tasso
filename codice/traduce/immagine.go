package traduce

// Preparazione dell'immagine catturata per la lettura del testo, e confronto tra due
// catture per capire se il dialogo è cambiato. Non dipende da Windows.

import (
	"encoding/binary"
	"image"
	"image/png"
	"io"
	"strings"
)

// grigio converte in scala di grigi.
func grigio(src *image.RGBA) *image.Gray {
	b := src.Bounds()
	g := image.NewGray(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := src.PixOffset(x, y)
			r, gg, bb := int(src.Pix[i]), int(src.Pix[i+1]), int(src.Pix[i+2])
			g.Pix[g.PixOffset(x, y)] = uint8((299*r + 587*gg + 114*bb) / 1000)
		}
	}
	return g
}

// ingrandisci di un fattore intero con interpolazione bilineare: la lettura di Windows
// riconosce molto meglio il testo piccolo dei giochi se le lettere sono più alte.
func ingrandisci(src *image.Gray, f int) *image.Gray {
	if f <= 1 {
		return src
	}
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewGray(image.Rect(0, 0, sw*f, sh*f))
	for y := 0; y < sh*f; y++ {
		fy := (float64(y)+0.5)/float64(f) - 0.5
		y0 := clamp(int(fy), 0, sh-1)
		y1 := clamp(y0+1, 0, sh-1)
		wy := fy - float64(int(fy))
		if fy < 0 {
			wy = 0
		}
		for x := 0; x < sw*f; x++ {
			fx := (float64(x)+0.5)/float64(f) - 0.5
			x0 := clamp(int(fx), 0, sw-1)
			x1 := clamp(x0+1, 0, sw-1)
			wx := fx - float64(int(fx))
			if fx < 0 {
				wx = 0
			}
			p := func(xx, yy int) float64 { return float64(src.Pix[src.PixOffset(xx+src.Rect.Min.X, yy+src.Rect.Min.Y)]) }
			v := (p(x0, y0)*(1-wx)+p(x1, y0)*wx)*(1-wy) + (p(x0, y1)*(1-wx)+p(x1, y1)*wx)*wy
			dst.Pix[dst.PixOffset(x, y)] = uint8(v + 0.5)
		}
	}
	return dst
}

func clamp(v, a, b int) int {
	if v < a {
		return a
	}
	if v > b {
		return b
	}
	return v
}

// contrasto allarga i toni (toglie l'1% più scuro e più chiaro), come un "auto-livelli".
func contrasto(g *image.Gray) {
	var ist [256]int
	for _, v := range g.Pix {
		ist[v]++
	}
	n := len(g.Pix)
	taglio := n / 100
	lo, hi, acc := 0, 255, 0
	for lo < 255 && acc+ist[lo] <= taglio {
		acc += ist[lo]
		lo++
	}
	acc = 0
	for hi > 0 && acc+ist[hi] <= taglio {
		acc += ist[hi]
		hi--
	}
	if hi-lo < 16 {
		return // immagine quasi uniforme: non tocco niente
	}
	for i, v := range g.Pix {
		g.Pix[i] = uint8(clamp((int(v)-lo)*255/(hi-lo), 0, 255))
	}
}

// fattoreIngrandimento: punto ad avere almeno ~360 px di altezza, senza superare 3x
// e senza superare il limite di dimensione della lettura di Windows.
func fattoreIngrandimento(w, h int) int {
	f := 1
	for f < 3 && h*f < 360 {
		f++
	}
	for f > 1 && (w*f > 4000 || h*f > 4000) {
		f--
	}
	return f
}

// preparaPerOCR: grigio + ingrandimento + contrasto, poi PNG.
func preparaPerOCR(src *image.RGBA, w io.Writer) error {
	g := grigio(src)
	g = ingrandisci(g, fattoreIngrandimento(g.Bounds().Dx(), g.Bounds().Dy()))
	contrasto(g)
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	return enc.Encode(w, g)
}

// Impronta: miniatura 64x16 in grigio, per confrontare due catture in modo tollerante
// (piccole animazioni dello sfondo non contano come "testo cambiato").
type Impronta [64 * 16]uint8

func calcolaImpronta(src *image.RGBA) Impronta {
	var imp Impronta
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return imp
	}
	for ty := 0; ty < 16; ty++ {
		for tx := 0; tx < 64; tx++ {
			x0, x1 := tx*w/64, (tx+1)*w/64
			y0, y1 := ty*h/16, (ty+1)*h/16
			if x1 <= x0 {
				x1 = x0 + 1
			}
			if y1 <= y0 {
				y1 = y0 + 1
			}
			var somma, n int
			for y := y0; y < y1 && y < h; y += 2 {
				for x := x0; x < x1 && x < w; x += 2 {
					i := src.PixOffset(b.Min.X+x, b.Min.Y+y)
					somma += (299*int(src.Pix[i]) + 587*int(src.Pix[i+1]) + 114*int(src.Pix[i+2])) / 1000
					n++
				}
			}
			if n > 0 {
				imp[ty*64+tx] = uint8(somma / n)
			}
		}
	}
	return imp
}

// differenza media tra due impronte (0 = identiche, 255 = opposte)
// celleCambiate: quante celle sono cambiate di più di soglia (una riga di dialogo nuova nello
// stesso riquadro cambia poche celle, ma molto: la media da sola non la vedeva).
func (a Impronta) celleCambiate(b Impronta, soglia int) int {
	n := 0
	for i := range a {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		if d > soglia {
			n++
		}
	}
	return n
}

// cambiata: la schermata è diversa (scena nuova, o parole nuove nello stesso riquadro).
func (a Impronta) cambiata(b Impronta) bool {
	return a.differenza(b) > sogliaCambiato || a.celleCambiate(b, 10) >= 3
}

func (a Impronta) differenza(b Impronta) float64 {
	var s int
	for i := range a {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		s += d
	}
	return float64(s) / float64(len(a))
}

// fattorePagina: per tutta la schermata. Sopra 2600 pixel di larghezza (2K grande, 4K) la
// dimezzo prima di leggerla: l'OCR va 3 volte più veloce e legge lo stesso testo.
func fattorePagina(w, h int) float64 {
	if w > 2600 {
		return 0.5
	}
	return float64(fattoreIngrandimento(w, h))
}

func preparaPaginaPerOCR(src *image.RGBA, w io.Writer) (float64, error) {
	g := grigio(src)
	f := fattorePagina(g.Bounds().Dx(), g.Bounds().Dy())
	if f < 1 {
		g = dimezza(g)
	} else {
		g = ingrandisci(g, int(f))
	}
	contrasto(g)
	return f, scriviBMP(w, g)
}

// scriviBMP: immagine grigia in BMP a 8 bit, senza compressione. Per la schermata intera è
// molto più svelto del PNG (niente compressione da fare né da disfare) e Windows lo legge uguale.
func scriviBMP(w io.Writer, g *image.Gray) error {
	b := g.Bounds()
	wd, ht := b.Dx(), b.Dy()
	riga := (wd + 3) &^ 3
	dati := riga * ht
	off := 14 + 40 + 256*4
	out := make([]byte, off+dati)
	le := binary.LittleEndian
	out[0], out[1] = 'B', 'M'
	le.PutUint32(out[2:], uint32(off+dati))
	le.PutUint32(out[10:], uint32(off))
	le.PutUint32(out[14:], 40)
	le.PutUint32(out[18:], uint32(wd))
	le.PutUint32(out[22:], uint32(ht)) // positivo: righe dal basso
	le.PutUint16(out[26:], 1)
	le.PutUint16(out[28:], 8)
	le.PutUint32(out[34:], uint32(dati))
	le.PutUint32(out[46:], 256)
	for i := 0; i < 256; i++ {
		p := 54 + i*4
		out[p], out[p+1], out[p+2] = byte(i), byte(i), byte(i)
	}
	for y := 0; y < ht; y++ {
		src := g.Pix[(y)*g.Stride : y*g.Stride+wd]
		copy(out[off+(ht-1-y)*riga:], src)
	}
	_, err := w.Write(out)
	return err
}

// dimezza: media di ogni quadrato 2x2 (le lettere restano morbide, non spezzate).
func dimezza(g *image.Gray) *image.Gray {
	b := g.Bounds()
	w, h := b.Dx()/2, b.Dy()/2
	out := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sx, sy := b.Min.X+2*x, b.Min.Y+2*y
			v := int(g.GrayAt(sx, sy).Y) + int(g.GrayAt(sx+1, sy).Y) + int(g.GrayAt(sx, sy+1).Y) + int(g.GrayAt(sx+1, sy+1).Y)
			out.Pix[y*out.Stride+x] = uint8(v / 4)
		}
	}
	return out
}

// preparaLuminosoPerOCR: secondo tentativo per le scritte chiare o colorate su sfondi
// movimentati (giallo col bordo nero sopra un paesaggio): tengo solo i pixel chiari o
// molto colorati e li faccio diventare nero su bianco.
func preparaLuminosoPerOCR(src *image.RGBA, w io.Writer) error {
	b := src.Bounds()
	g := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := src.RGBAAt(b.Min.X+x, b.Min.Y+y)
			mx := max(c.R, c.G, c.B)
			mn := min(c.R, c.G, c.B)
			v := uint8(255)
			if mx >= 180 || int(mx)-int(mn) >= 110 {
				v = 0
			}
			g.Pix[y*g.Stride+x] = v
		}
	}
	g = ingrandisci(g, max(2, fattoreIngrandimento(g.Bounds().Dx(), g.Bounds().Dy())))
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	return enc.Encode(w, g)
}

// stranezza: quota di caratteri che in un dialogo inglese non ci sono quasi mai
// (simboli letti al posto delle lettere). Più bassa = lettura migliore.
func stranezza(t string) float64 {
	n, strani, lettere := 0, 0, 0
	for _, r := range t {
		if r == ' ' || r == '\n' || r == '\r' {
			continue
		}
		n++
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			lettere++
		case (r >= '0' && r <= '9') || strings.ContainsRune(".,'!?:;-\"()%+/", r):
		default:
			strani++
		}
	}
	if lettere < 2 {
		return 1
	}
	return float64(strani) / float64(n)
}

// bordiCambiati: i contorni netti (lettere, cornici) sono cambiati in almeno una zona dello
// schermo. Uno sfondo sfocato che si muove (pianeta dietro il menu di No Man's Sky, fumo, luci)
// cambia la luce ma quasi non ha contorni netti: senza questo controllo la pagina veniva riletta
// ogni secondo anche a menu fermo (lettura 0,5 s di processore a vuoto, 24/11).
func bordiCambiati(a, b *image.RGBA) bool {
	if a == nil || b == nil || a.Bounds() != b.Bounds() {
		return true
	}
	r := a.Bounds()
	w, h := r.Dx(), r.Dy()
	if w < 8 || h < 8 {
		return true
	}
	const cx, cy = 32, 12
	var diff, tot [cx * cy]int
	luce := func(img *image.RGBA, x, y int) int {
		i := img.PixOffset(r.Min.X+x, r.Min.Y+y)
		return (299*int(img.Pix[i]) + 587*int(img.Pix[i+1]) + 114*int(img.Pix[i+2])) / 1000
	}
	bordo := func(img *image.RGBA, x, y int) bool {
		d := luce(img, x+1, y) - luce(img, x-1, y)
		if d < 0 {
			d = -d
		}
		if d > 45 {
			return true
		}
		d = luce(img, x, y+1) - luce(img, x, y-1)
		return d > 45 || d < -45
	}
	for y := 1; y < h-1; y += 2 {
		for x := 1; x < w-1; x += 2 {
			c := (y*cy/h)*cx + x*cx/w
			tot[c]++
			if bordo(a, x, y) != bordo(b, x, y) {
				diff[c]++
			}
		}
	}
	for c := range diff {
		if diff[c] >= 8 && diff[c]*50 >= tot[c] { // almeno il 2% dei punti della zona
			return true
		}
	}
	return false
}
