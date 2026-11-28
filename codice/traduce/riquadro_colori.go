package traduce

// Colori del riquadro della traduzione (finestrella sopra il gioco) e controllo del contrasto.
// 28/11: il riquadro era trasparente all'89% e sotto si leggeva l'inglese grigio del gioco.
// Ora è pieno (opacità 255): il testo del gioco sotto non si vede più, e ogni scritta ha un
// contrasto di almeno 4,5:1 (WCAG AA) col fondo, qualunque cosa ci sia dietro.

import "math"

// valori per GDI (0x00BBGGRR)
const (
	opacitaRiquadro     = 255        // pieno: niente inglese che traspare
	colFondoRiquadro    = 0x000D0807 // #07080D
	colTestoRiquadro    = 0x00F8F4F2 // #F2F4F8 traduzione
	colIngleseRiquadro  = 0x00B8AEA8 // #A8AEB8 inglese (se lo vuoi vedere): più chiaro di prima
	colStatoRiquadro    = 0x00A6F32F // #2FF3A6 riga in basso
	colTastoChiudi      = 0x003A2F2A // #2A2F3A fondo della X
	colTastoChiudiSopra = 0x005F5046 // #46505F fondo della X col mouse sopra
)

// rgbDaGDI: da 0x00BBGGRR a r, g, b.
func rgbDaGDI(c uint32) (uint8, uint8, uint8) {
	return uint8(c), uint8(c >> 8), uint8(c >> 16)
}

// luminanza relativa (WCAG 2.x)
func luminanza(r, g, b uint8) float64 {
	lin := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// contrasto: rapporto di contrasto WCAG tra due colori GDI.
func contrastoWCAG(a, b uint32) float64 {
	la := luminanza(rgbDaGDI(a))
	lb := luminanza(rgbDaGDI(b))
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// fondoSopra: il colore che si vede col riquadro (opacità alfa 0-255) sopra il colore sotto.
func fondoSopra(fondo, sotto uint32, alfa uint8) uint32 {
	fr, fg, fb := rgbDaGDI(fondo)
	sr, sg, sb := rgbDaGDI(sotto)
	m := func(f, s uint8) uint32 {
		return uint32((uint32(f)*uint32(alfa) + uint32(s)*(255-uint32(alfa)) + 127) / 255)
	}
	return m(fr, sr) | m(fg, sg)<<8 | m(fb, sb)<<16
}
