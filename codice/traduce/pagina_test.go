package traduce

import (
	"image"
	"image/color"
	"testing"
)

func par(t string, x, y, w, h float64) ParolaOCR { return ParolaOCR{t, x, y, w, h} }

func TestSegmentiEParagrafi(t *testing.T) {
	righe := []RigaOCR{
		{Parole: []ParolaOCR{par("Asset", 10, 10, 50, 16), par("Sex:", 64, 10, 40, 16), par("Male", 108, 10, 40, 16), par("Diet:", 250, 10, 40, 16), par("Carnivore", 294, 10, 80, 16)}},
		{Parole: []ParolaOCR{par("-", 10, 40, 6, 16), par("Perfect", 20, 40, 60, 16), par("Diet", 84, 40, 40, 16)}},
		{Parole: []ParolaOCR{par("-", 10, 62, 6, 16), par("Growth", 20, 62, 60, 16), par("Rate", 84, 62, 40, 16)}},
		{Parole: []ParolaOCR{par("The", 10, 100, 30, 16), par("bridge", 44, 100, 50, 16), par("collapsed", 98, 100, 70, 16)}},
		{Parole: []ParolaOCR{par("last", 10, 120, 30, 16), par("winter.", 44, 120, 50, 16)}},
	}
	b := unisciParagrafi(segmenti(righe, 2))
	testi := []string{}
	for _, x := range b {
		testi = append(testi, x.Testo)
	}
	atteso := []string{"Asset Sex: Male", "Diet: Carnivore", "- Perfect Diet", "- Growth Rate", "The bridge collapsed last winter."}
	if len(testi) != len(atteso) {
		t.Fatalf("blocchi: %q", testi)
	}
	for i := range atteso {
		if testi[i] != atteso[i] {
			t.Fatalf("blocchi: %q", testi)
		}
	}
	if b[0].X != 5 || b[0].Y != 5 || b[0].W != 69 { // fattore 2: coordinate dimezzate
		t.Fatalf("rettangolo: %+v", b[0])
	}
	if b[4].Righe != 2 {
		t.Fatalf("paragrafo: %+v", b[4])
	}
}

func TestColoriBlocco(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 60))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 8, 20, 30, 255 // blu scuro
	}
	for x := 30; x < 150; x += 2 { // "testo" azzurro
		for y := 22; y < 36; y++ {
			img.SetRGBA(x, y, color.RGBA{120, 210, 230, 255})
		}
	}
	b := Blocco{X: 28, Y: 20, W: 125, H: 18}
	coloriBlocco(img, &b)
	if distanza(b.Sfondo, color.RGBA{8, 20, 30, 255}) > 12 {
		t.Fatalf("sfondo %+v", b.Sfondo)
	}
	if distanza(b.Inchiostro, color.RGBA{120, 210, 230, 255}) > 30 {
		t.Fatalf("inchiostro %+v", b.Inchiostro)
	}
	if daTradurre("185/425") || daTradurre("47.5 KM/h") == false && false || daTradurre("α") || !daTradurre("Preferred Food:") {
		t.Fatal("daTradurre")
	}
}

func TestFondoRicostruito(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 60))
	for y := 0; y < 60; y++ { // sfumatura verticale da nero a blu
		for x := 0; x < 100; x++ {
			img.SetRGBA(x, y, color.RGBA{0, 0, uint8(y * 4), 255})
		}
	}
	for x := 20; x < 80; x += 3 {
		for y := 22; y < 34; y++ {
			img.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	b := Blocco{X: 20, Y: 22, W: 60, H: 12}
	coloriBlocco(img, &b)
	f := fondoRicostruito(img, b)
	if f == nil || f.Bounds().Dx() != 66 || f.Bounds().Dy() != 18 {
		t.Fatalf("fondo: %v", f.Bounds())
	}
	alto, basso := f.RGBAAt(30, 0), f.RGBAAt(30, 17)
	if alto.B >= basso.B || alto.R > 10 || basso.R > 10 {
		t.Fatalf("la sfumatura deve restare: %v %v", alto, basso)
	}
}

func TestDentro(t *testing.T) {
	z := Zona{X: 100, Y: 100, W: 200, H: 100}
	if !dentro(Blocco{X: 150, Y: 120, W: 40, H: 20}, z) || dentro(Blocco{X: 10, Y: 10, W: 40, H: 20}, z) {
		t.Error("dentro sbagliato")
	}
}

func TestParolaTagliata(t *testing.T) {
	if daTradurre("S REPORT") {
		t.Error("coda di parola tagliata: non va tradotta")
	}
	if !daTradurre("A NEW QUEST") {
		t.Error("A è una parola vera")
	}
}

func TestRicoloraPulsante(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 60))
	for i := range img.Pix {
		img.Pix[i] = 20
		if i%4 == 3 {
			img.Pix[i] = 255
		}
	}
	b := Blocco{X: 20, Y: 20, W: 80, H: 20, Righe: 1, Tradotto: "Indietro"}
	coloriBlocco(img, &b)
	if _, ok := ricolora(img, []Blocco{b}); ok {
		t.Fatal("niente è cambiato: non devo ridisegnare")
	}
	// il gioco evidenzia il pulsante: sfondo chiaro
	for y := 10; y < 50; y++ {
		for x := 10; x < 110; x++ {
			img.SetRGBA(x, y, color.RGBA{200, 180, 60, 255})
		}
	}
	nuovi, ok := ricolora(img, []Blocco{b})
	if !ok || distanza(nuovi[0].Sfondo, color.RGBA{200, 180, 60, 255}) > 30 {
		t.Fatalf("pulsante evidenziato non seguito: %v %v", ok, nuovi[0].Sfondo)
	}
}
