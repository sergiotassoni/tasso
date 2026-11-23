package traduce

import (
	"image"
	"image/color"
	"testing"
)

// Lettere bianche con il bordino nero su un cielo chiaro: l'inchiostro è il bianco.
func TestRiempimentoBiancoBordoNero(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 60))
	cielo := color.RGBA{150, 190, 230, 255}
	for y := 0; y < 60; y++ {
		for x := 0; x < 200; x++ {
			img.SetRGBA(x, y, cielo)
		}
	}
	// 6 "lettere": barre verticali 6px bianche con bordo nero 2px
	for k := 0; k < 6; k++ {
		x0 := 20 + k*25
		for y := 12; y < 48; y++ {
			for x := x0 - 2; x < x0+8; x++ {
				img.SetRGBA(x, y, color.RGBA{20, 20, 25, 255})
			}
		}
		for y := 14; y < 46; y++ {
			for x := x0; x < x0+6; x++ {
				img.SetRGBA(x, y, color.RGBA{250, 250, 250, 255})
			}
		}
	}
	b := Blocco{X: 16, Y: 10, W: 150, H: 40, Em: 36}
	coloriBlocco(img, &b)
	if b.Inchiostro.R < 200 {
		t.Fatalf("inchiostro %v: doveva essere bianco", b.Inchiostro)
	}
	if !b.HaContorno || b.Contorno.R > 60 {
		t.Fatalf("contorno %v %v: doveva essere nero", b.HaContorno, b.Contorno)
	}
}

// Lettere bianche senza bordo su fondo scuro: nessun contorno inventato.
func TestRiempimentoSenzaBordo(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 200; x++ {
			img.SetRGBA(x, y, color.RGBA{20, 22, 30, 255})
		}
	}
	for k := 0; k < 6; k++ {
		x0 := 20 + k*25
		for y := 14; y < 46; y++ {
			for x := x0; x < x0+6; x++ {
				img.SetRGBA(x, y, color.RGBA{240, 240, 240, 255})
			}
		}
	}
	b := Blocco{X: 16, Y: 10, W: 150, H: 40, Em: 36}
	coloriBlocco(img, &b)
	if b.Inchiostro.R < 200 || b.HaContorno {
		t.Fatalf("inchiostro %v contorno %v", b.Inchiostro, b.HaContorno)
	}
}
