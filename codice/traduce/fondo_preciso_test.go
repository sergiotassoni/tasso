package traduce

import (
	"image"
	"image/color"
	"testing"
)

func TestFondoPrecisoTieneIBordi(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 200; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(20 + x/4), 30, uint8(60 + y), 255}) // sfumatura
		}
	}
	// bordo verticale di una casella a x=120
	for y := 0; y < 60; y++ {
		img.SetRGBA(120, y, color.RGBA{200, 180, 40, 255})
	}
	// "lettere": barrette bianche tra x=20 e 80, y=20..34
	for x := 20; x < 80; x += 6 {
		for y := 20; y < 34; y++ {
			img.SetRGBA(x, y, color.RGBA{250, 250, 250, 255})
			img.SetRGBA(x+1, y, color.RGBA{250, 250, 250, 255})
		}
	}
	b := Blocco{X: 18, Y: 18, W: 64, H: 18, Em: 14, Img: img}
	coloriBlocco(img, &b)
	b.LargMax = 170
	if l := liberoFinoAlBordo(img, b); l >= 120-18 {
		t.Fatalf("la traduzione andrebbe sopra il bordo: %d", l)
	}
	f, fx, fy := fondoPreciso(b, 15, 15, 140, 40)
	// il bordo della casella è identico
	if c := f.RGBAAt(120-fx, 25-fy); c != (color.RGBA{200, 180, 40, 255}) {
		t.Fatalf("bordo cambiato: %v", c)
	}
	// le lettere sono sparite: pixel vicini allo sfondo
	for x := 20; x < 80; x += 6 {
		c := f.RGBAAt(x-fx, 27-fy)
		atteso := img.RGBAAt(x+3, 27)
		if distanza(c, atteso) > 20 {
			t.Fatalf("lettera rimasta a x=%d: %v contro %v", x, c, atteso)
		}
	}
}

func TestContornoLettere(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 40))
	for i := range img.Pix {
		img.Pix[i] = 120
		if i%4 == 3 {
			img.Pix[i] = 255
		}
	}
	for x := 10; x < 80; x += 7 { // lettere bianche con bordo nero
		for y := 10; y < 30; y++ {
			for dx := -1; dx <= 3; dx++ {
				img.SetRGBA(x+dx, y, color.RGBA{10, 10, 10, 255})
			}
			img.SetRGBA(x, y, color.RGBA{250, 250, 250, 255})
			img.SetRGBA(x+1, y, color.RGBA{250, 250, 250, 255})
			img.SetRGBA(x+2, y, color.RGBA{250, 250, 250, 255})
		}
	}
	b := Blocco{X: 8, Y: 9, W: 76, H: 22, Em: 18}
	coloriBlocco(img, &b)
	contornoLettere(img, &b)
	if !b.HaContorno || distanza(b.Contorno, color.RGBA{10, 10, 10, 255}) > 30 {
		t.Fatalf("contorno non visto: %v %v ink=%v sf=%v", b.HaContorno, b.Contorno, b.Inchiostro, b.Sfondo)
	}
	// testo semplice: niente contorno
	img2 := image.NewRGBA(image.Rect(0, 0, 100, 40))
	for x := 10; x < 80; x += 7 {
		for y := 10; y < 30; y++ {
			img2.SetRGBA(x, y, color.RGBA{250, 250, 250, 255})
		}
	}
	b2 := Blocco{X: 8, Y: 9, W: 76, H: 22, Em: 18}
	coloriBlocco(img2, &b2)
	contornoLettere(img2, &b2)
	if b2.HaContorno {
		t.Fatal("contorno inventato")
	}
}
