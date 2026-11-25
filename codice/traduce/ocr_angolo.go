package traduce

// Testo un po' storto (cartelli ruotati, scritte inclinate nei giochi): l'OCR di Windows trova
// l'angolo del testo (OcrResult.TextAngle, gradi in senso orario attorno al centro
// dell'immagine) e dà i rettangoli delle parole nell'immagine RADDRIZZATA di quell'angolo. Senza
// riportarli indietro tutte le parole della schermata sono spostate (anche quelle dritte): il
// riquadro di "STORAGE ROOM B" partiva 35 px più in basso delle lettere vere, e il controllo delle
// parole non lette vedeva la targa verde come "parola accanto".

import "math"

// raddrizzaParole: i rettangoli delle parole riportati nell'immagine vera (larga w, alta h). Ogni
// parola diventa il rettangolo dritto che contiene la parola ruotata.
func raddrizzaParole(righe []RigaOCR, angolo float64, w, h int) []RigaOCR {
	if math.Abs(angolo) < 0.3 || math.Abs(angolo) > 45 || w <= 0 || h <= 0 || math.IsNaN(angolo) {
		return righe
	}
	rad := angolo * math.Pi / 180
	c, s := math.Cos(rad), math.Sin(rad)
	cx, cy := float64(w)/2, float64(h)/2
	out := make([]RigaOCR, len(righe))
	for i, r := range righe {
		out[i].Parole = make([]ParolaOCR, len(r.Parole))
		for j, p := range r.Parole {
			dx, dy := p.X+p.W/2-cx, p.Y+p.H/2-cy
			// rotazione in senso orario sullo schermo (y verso il basso)
			nx, ny := cx+dx*c-dy*s, cy+dx*s+dy*c
			ww := p.W*math.Abs(c) + p.H*math.Abs(s)
			hh := p.W*math.Abs(s) + p.H*math.Abs(c)
			out[i].Parole[j] = ParolaOCR{T: p.T, X: nx - ww/2, Y: ny - hh/2, W: ww, H: hh}
		}
	}
	return out
}
