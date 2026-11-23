package traduce

import (
	"image"
	"image/color"
	"testing"
)

// La rotella sposta l'elenco: più righe cambiano sotto le traduzioni → pagina spostata.
// Il mouse sopra una riga (nome e valore) cambia una riga sola → resta un semplice cambio di colore.
func TestPaginaSpostata(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 400, 300))
	for i := range a.Pix {
		a.Pix[i] = 40
	}
	var bl []Blocco
	for r := 0; r < 5; r++ {
		bl = append(bl, Blocco{X: 10, Y: 20 + r*40, W: 150, H: 20, Img: a})
		bl = append(bl, Blocco{X: 250, Y: 20 + r*40, W: 100, H: 20, Img: a})
	}
	colora := func(img *image.RGBA, y0, y1 int) {
		for y := y0; y < y1; y++ {
			for x := 0; x < 400; x++ {
				img.SetRGBA(x, y, color.RGBA{220, 190, 60, 255})
			}
		}
	}
	mouse := image.NewRGBA(a.Bounds())
	copy(mouse.Pix, a.Pix)
	colora(mouse, 60, 80) // una riga evidenziata, due scritte
	if paginaSpostata(mouse, bl) {
		t.Fatal("il mouse sopra una riga non deve contare come rotella")
	}
	rot := image.NewRGBA(a.Bounds())
	copy(rot.Pix, a.Pix)
	colora(rot, 20, 140) // tre righe cambiate
	if !paginaSpostata(rot, bl) {
		t.Fatal("la rotella che sposta tre righe non è stata vista")
	}
}
