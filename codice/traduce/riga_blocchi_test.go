package traduce

import (
	"image"
	"image/color"
	"testing"
)

// immagine finta: sfondo e "lettere" (rettangolini) dove dico io.
func immagineFinta(w, h int, sf color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = sf.R, sf.G, sf.B, 255
	}
	return img
}

// parolaFinta: disegna una parola di n lettere larghe lw (spazio 2 px) da x, alta h da y.
func parolaFinta(img *image.RGBA, x, y, n, lw, h int, c color.RGBA) int {
	for k := 0; k < n; k++ {
		for yy := y; yy < y+h; yy++ {
			for xx := x + k*(lw+2); xx < x+k*(lw+2)+lw; xx++ {
				img.SetRGBA(xx, yy, c)
			}
		}
	}
	return x + n*(lw+2) - 2
}

func TestUnisciStessaRiga(t *testing.T) {
	// "JOIN" e "FRIEND" divise dall'OCR (stessa linea, spazio di una parola): tornano una riga
	a := Blocco{Testo: "JOIN", X: 100, Y: 300, W: 80, H: 30, Righe: 1, Em: 34}
	b := Blocco{Testo: "FRIEND", X: 192, Y: 301, W: 120, H: 30, Righe: 1, Em: 34}
	// una colonna lontana sulla stessa riga resta separata ("Volume" ....... "80%")
	c := Blocco{Testo: "80%", X: 700, Y: 300, W: 50, H: 30, Righe: 1, Em: 34}
	// la riga sotto non si unisce
	d := Blocco{Testo: "SINGLEPLAYER", X: 100, Y: 350, W: 200, H: 30, Righe: 1, Em: 34}
	// testo piccolo accanto a un titolo grande: grandezze diverse, separati
	e := Blocco{Testo: "v0.2.1", X: 320, Y: 305, W: 40, H: 12, Righe: 1, Em: 14}
	out := unisciStessaRiga([]Blocco{b, d, c, a, e})
	trovato := false
	for _, x := range out {
		switch x.Testo {
		case "JOIN FRIEND":
			trovato = true
			if x.X != 100 || x.W != 212 {
				t.Errorf("riquadro unito sbagliato: %+v", x)
			}
		case "JOIN", "FRIEND":
			t.Errorf("pezzo rimasto da solo: %q", x.Testo)
		}
	}
	if !trovato || len(out) != 4 {
		t.Fatalf("unione sbagliata: %d scritte %+v", len(out), out)
	}
	// "FOCUS > Move to fill" + "the progress bar": una riga sola anche con testo piccolo
	f := Blocco{Testo: "FOCUS > Move to fill", X: 20, Y: 900, W: 300, H: 22, Righe: 1, Em: 26}
	g := Blocco{Testo: "the progress bar", X: 328, Y: 899, W: 200, H: 24, Righe: 1, Em: 26}
	if o := unisciStessaRiga([]Blocco{g, f}); len(o) != 1 || o[0].Testo != "FOCUS > Move to fill the progress bar" {
		t.Fatalf("riga non unita: %+v", o)
	}
}

func TestSpezzaSeparatori(t *testing.T) {
	p := []ParolaOCR{{T: "FOCUS", X: 20, Y: 900, W: 80, H: 22}, {T: ">", X: 108, Y: 902, W: 12, H: 18},
		{T: "Move", X: 128, Y: 900, W: 60, H: 22}, {T: "to", X: 194, Y: 900, W: 20, H: 22}, {T: "fill", X: 220, Y: 900, W: 40, H: 22}}
	b := Blocco{Testo: "FOCUS > Move to fill", X: 20, Y: 900, W: 240, H: 22, Righe: 1, Em: 26, Pos: &p}
	out := spezzaSeparatori([]Blocco{b})
	if len(out) != 2 || out[0].Testo != "FOCUS" || out[1].Testo != "Move to fill" {
		t.Fatalf("divisione sbagliata: %+v", out)
	}
	if out[1].X != 128 || out[0].X+out[0].W > 108 {
		t.Fatalf("il segno > non deve finire in nessun riquadro: %+v", out)
	}
	// segno attaccato: ">Move"
	p2 := []ParolaOCR{{T: "Map", X: 0, Y: 0, W: 50, H: 20}, {T: "|Inventory", X: 60, Y: 0, W: 150, H: 20}}
	p3 := append(p2, ParolaOCR{T: "x", X: 300, W: 1, H: 20})
	o2 := spezzaSeparatori([]Blocco{{Testo: "Map |Inventory", W: 210, H: 20, Righe: 1, Pos: &p3}})
	if len(o2) != 2 || o2[1].Testo != "Inventory x" {
		t.Fatalf("segno attaccato: %+v", o2)
	}
	// "Score: 10 - 5" senza separatori veri: resta com'è
	o3 := spezzaSeparatori([]Blocco{{Testo: "Rock - Paper - Scissors", Righe: 1, Pos: &[]ParolaOCR{{T: "Rock"}, {T: "-"}, {T: "Paper"}}}})
	if len(o3) != 1 {
		t.Fatalf("divisa per niente: %+v", o3)
	}
}

// Scritta con una parola non letta accanto: la riga è incompleta (non si traduce mezza).
// Vale per testo chiaro su scuro e scuro su chiaro.
func TestScrittaContinua(t *testing.T) {
	for _, colori := range [][2]color.RGBA{
		{{20, 24, 30, 255}, {240, 240, 240, 255}},  // chiaro su scuro
		{{235, 230, 220, 255}, {30, 30, 40, 255}},  // scuro su chiaro
		{{40, 90, 200, 255}, {250, 250, 250, 255}}, // bianco su pulsante blu
	} {
		sf, ink := colori[0], colori[1]
		img := immagineFinta(800, 200, sf)
		fineJoin := parolaFinta(img, 100, 50, 4, 14, 30, ink) // "JOIN"
		parolaFinta(img, fineJoin+10, 50, 6, 14, 30, ink)     // "FRIEND" a uno spazio di distanza
		b := Blocco{X: 100, Y: 50, W: fineJoin - 100, H: 30, Em: 34, Sfondo: sf, Inchiostro: ink}
		if !scrittaContinua(img, b) {
			t.Errorf("%v: la parola accanto non letta doveva essere vista", colori)
		}
		// la parola intera letta: niente accanto
		img2 := immagineFinta(800, 200, sf)
		fine := parolaFinta(img2, 100, 50, 10, 14, 30, ink)
		b2 := Blocco{X: 100, Y: 50, W: fine - 100, H: 30, Em: 34, Sfondo: sf, Inchiostro: ink}
		if scrittaContinua(img2, b2) {
			t.Errorf("%v: niente parole accanto, ma detto incompleta", colori)
		}
		// parola non letta a SINISTRA ("QUIT" non letto, "GAME" sì)
		img3 := immagineFinta(800, 200, sf)
		fq := parolaFinta(img3, 100, 50, 4, 14, 30, ink)
		fg := parolaFinta(img3, fq+10, 50, 4, 14, 30, ink)
		b3 := Blocco{X: fq + 10, Y: 50, W: fg - fq - 10, H: 30, Em: 34, Sfondo: sf, Inchiostro: ink}
		if !scrittaContinua(img3, b3) {
			t.Errorf("%v: parola non letta a sinistra non vista", colori)
		}
		// un'icona lontana (colonna dopo 2 em) non conta
		img4 := immagineFinta(800, 200, sf)
		f4 := parolaFinta(img4, 100, 50, 5, 14, 30, ink)
		parolaFinta(img4, f4+90, 50, 1, 20, 30, ink)
		b4 := Blocco{X: 100, Y: 50, W: f4 - 100, H: 30, Em: 34, Sfondo: sf, Inchiostro: ink}
		if scrittaContinua(img4, b4) {
			t.Errorf("%v: un segno lontano non è la stessa riga", colori)
		}
	}
}

// Il riquadro si allarga fino alle lettere vere (frange di colore, lettera tagliata dall'OCR).
func TestAdattaAInchiostro(t *testing.T) {
	sf, ink := color.RGBA{10, 10, 20, 255}, color.RGBA{250, 250, 250, 255}
	img := immagineFinta(400, 120, sf)
	fine := parolaFinta(img, 50, 40, 5, 12, 26, ink)
	// l'OCR ha dato un riquadro più stretto di 4 px a destra, 3 a sinistra, 2 sopra
	b := Blocco{X: 53, Y: 42, W: fine - 4 - 53, H: 24, Em: 28, Sfondo: sf, Inchiostro: ink}
	adattaAInchiostro(img, &b)
	if b.X != 50 || b.X+b.W != fine || b.Y != 40 || b.Y+b.H != 66 {
		t.Fatalf("riquadro non adattato: %d,%d %dx%d (lettere 50..%d, 40..66)", b.X, b.Y, b.W, b.H, fine)
	}
}

// La toppa copre sempre tutta la scritta inglese, con margine, e tutta la scritta italiana.
func TestCalcolaToppa(t *testing.T) {
	b := Blocco{X: 100, Y: 200, W: 150, H: 30}
	// italiano più corto, centrato dentro
	testo := image.Rect(110, 198, 220, 234)
	r := calcolaToppa(b, testo, b.W, 4, true)
	ingl := image.Rect(b.X, b.Y, b.X+b.W, b.Y+b.H)
	if !ingl.In(r) || r.Min.X > 96 || r.Max.X < 254 || r.Min.Y > 196 {
		t.Fatalf("la toppa non copre l'inglese con margine: %v", r)
	}
	if r.Max.Y != 234 {
		t.Fatalf("una riga sola non deve scendere sulla riga sotto: %v", r)
	}
	// italiano più lungo: la toppa arriva fino in fondo alla scritta nuova
	testo = image.Rect(98, 198, 330, 234)
	r = calcolaToppa(b, testo, b.W, 4, true)
	if r.Max.X < 328+4 {
		t.Fatalf("la scritta italiana esce dalla toppa: %v", r)
	}
	// con la pagina pulita la toppa contiene tutto il rettangolo del testo
	b.Pulita = image.NewRGBA(image.Rect(0, 0, 10, 10))
	testo = image.Rect(90, 190, 340, 240)
	if r = calcolaToppa(b, testo, b.W, 4, true); !testo.In(r) {
		t.Fatalf("testo fuori dalla toppa: %v %v", testo, r)
	}
	// paragrafo di più righe: copre tutta l'altezza dell'italiano
	p := Blocco{X: 10, Y: 10, W: 300, H: 60, Righe: 3}
	r = calcolaToppa(p, image.Rect(8, 8, 300, 92), 300, 3, false)
	if r.Max.Y < 90+3 { // la scritta finisce a 90 (il rettangolo ha 2 px di aria)
		t.Fatalf("paragrafo tagliato: %v", r)
	}
}

// Lo sfondo ricostruito sotto la scritta prende i colori attorno (sfumatura) e non le lettere.
func TestFondoRicostruitoSfumatura(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 300, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 300; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(40 + x/3), uint8(60 + y), 120, 255}) // sfumatura nelle due direzioni
		}
	}
	ink := color.RGBA{255, 255, 255, 255}
	fine := parolaFinta(img, 100, 40, 6, 10, 20, ink)
	b := Blocco{X: 100, Y: 40, W: fine - 100, H: 20, Em: 22}
	coloriBlocco(img, &b)
	f := fondoRicostruito(img, b)
	if f == nil {
		t.Fatal("niente sfondo")
	}
	ml, _ := margineFondo(b)
	for y := 0; y < f.Bounds().Dy(); y += 3 {
		for x := 0; x < f.Bounds().Dx(); x += 3 {
			c := f.RGBAAt(x, y)
			gx, gy := b.X-ml+x, b.Y-3+y
			atteso := color.RGBA{uint8(40 + gx/3), uint8(60 + gy), 120, 255}
			if distanza(c, ink) < 200 || distanza(c, atteso) > 30 {
				t.Fatalf("pixel %d,%d dello sfondo: %v, atteso circa %v", gx, gy, c, atteso)
			}
		}
	}
}
