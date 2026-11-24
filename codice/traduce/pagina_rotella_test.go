package traduce

import (
	"image"
	"image/color"
	"testing"
)

// La rotella sposta l'elenco: le scritte si ritrovano più su → pagina spostata.
// Mouse sopra una riga, riquadri illuminati, finestrella di descrizione: le scritte restano
// ferme → solo cambio di colore, le traduzioni restano (No Man's Sky, inventario).
func TestPaginaSpostata(t *testing.T) {
	const W, H = 400, 300
	// righe con "lettere" diverse: barrette chiare a posizioni diverse per ogni riga
	disegna := func(off int) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, W, H))
		for i := range img.Pix {
			img.Pix[i] = 40
		}
		for r := 0; r < 7; r++ {
			y0 := 20 + r*40 - off
			for y := y0 + 4; y < y0+16; y++ {
				if y < 0 || y >= H {
					continue
				}
				for x := 10; x < 360; x++ {
					if (x*(r+3)+(y-y0)*(r%3+1))%11 < 4 {
						img.SetRGBA(x, y, color.RGBA{230, 230, 230, 255})
					}
				}
			}
		}
		return img
	}
	a := disegna(0)
	var bl []Blocco
	for r := 0; r < 5; r++ {
		bl = append(bl, Blocco{X: 10, Y: 20 + r*40, W: 150, H: 20, Img: a})
		bl = append(bl, Blocco{X: 250, Y: 20 + r*40, W: 100, H: 20, Img: a})
	}
	copia := func() *image.RGBA {
		c := image.NewRGBA(a.Bounds())
		copy(c.Pix, a.Pix)
		return c
	}
	colora := func(img *image.RGBA, y0, y1 int) {
		for y := y0; y < y1; y++ {
			for x := 0; x < W; x++ {
				img.SetRGBA(x, y, color.RGBA{220, 190, 60, 255})
			}
		}
	}
	mouse := copia()
	colora(mouse, 60, 80)
	if paginaSpostata(mouse, bl) {
		t.Fatal("il mouse sopra una riga non deve contare come rotella")
	}
	tante := copia()
	colora(tante, 20, 140) // finestrella o riquadri illuminati su tre righe: le scritte non si spostano
	if paginaSpostata(tante, bl) {
		t.Fatal("una finestrella sopra tre righe non è una rotella")
	}
	luce := copia()
	for i := 0; i < len(luce.Pix); i += 4 { // tutto più chiaro (sfondo animato)
		luce.Pix[i] = min(255, luce.Pix[i]+30)
	}
	if paginaSpostata(luce, bl) {
		t.Fatal("uno sfondo che cambia luce non è una rotella")
	}
	if !paginaSpostata(disegna(15), bl) {
		t.Fatal("la rotella che sposta l'elenco di 15 pixel non è stata vista")
	}
	if !paginaSpostata(disegna(-40), bl) {
		t.Fatal("la rotella in giù di una riga non è stata vista")
	}
}

func TestScenaCambiata(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 100, 100))
	bl := []Blocco{{X: 0, Y: 0, W: 50, H: 20, Img: a}, {X: 0, Y: 40, W: 50, H: 20, Img: a}, {X: 0, Y: 70, W: 50, H: 20, Img: a}}
	b := image.NewRGBA(a.Bounds())
	for y := 70; y < 90; y++ {
		for x := 0; x < 50; x++ {
			b.SetRGBA(x, y, color.RGBA{200, 200, 200, 255})
		}
	}
	if scenaCambiata(b, bl) {
		t.Fatal("una sola scritta cambiata non è una scena nuova")
	}
	for i := range b.Pix {
		b.Pix[i] = 200
	}
	if !scenaCambiata(b, bl) {
		t.Fatal("tutto cambiato: scena nuova")
	}
}

func TestTieniFerme(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 100, 100))
	vecchio := Blocco{Testo: "CARGO", Tradotto: "CARICO", X: 10, Y: 10, W: 40, H: 12, Em: 11, Img: a}
	nuovo := Blocco{Testo: "CARGO", Tradotto: "CARICO", X: 11, Y: 9, W: 41, H: 13, Em: 12, Img: a}
	altro := Blocco{Testo: "Used for:", Tradotto: "Utilizzato per:", X: 10, Y: 50, W: 40, H: 12, Img: a}
	out := tieniFerme([]Blocco{nuovo, altro}, []Blocco{vecchio}, a)
	if out[0].X != 10 || out[0].Em != 11 || out[1].Testo != "Used for:" {
		t.Fatalf("la scritta ferma deve restare identica: %+v", out[0])
	}
	b := image.NewRGBA(a.Bounds())
	for i := range b.Pix {
		b.Pix[i] = 220
	}
	if out := tieniFerme([]Blocco{nuovo}, []Blocco{vecchio}, b); out[0].X != 11 {
		t.Fatal("se l'inglese sotto è cambiato si usa la lettura nuova")
	}
}

func TestTitoloMaiuscoloNonSiUnisce(t *testing.T) {
	if continuaFrase("CRAFT MORE", "reate additional Microprocessor") {
		t.Fatal("il titolo maiuscolo non continua nella riga sotto")
	}
	if !continuaFrase("Stack items, split stacks or move", "to other inventories") {
		t.Fatal("una frase normale andata a capo deve unirsi")
	}
}

func TestBordiCambiati(t *testing.T) {
	fai := func(luceFondo uint8, scritta int) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, 640, 240))
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = luceFondo, luceFondo/2, 30, 255
		}
		for y := 100; y < 116; y++ { // una "parola": barrette verticali
			for x := 50; x < 250; x++ {
				if (x+scritta)%7 < 2 {
					img.SetRGBA(x, y, color.RGBA{250, 250, 250, 255})
				}
			}
		}
		return img
	}
	a := fai(60, 0)
	if bordiCambiati(a, fai(90, 0)) {
		t.Fatal("solo lo sfondo più chiaro: le scritte sono le stesse")
	}
	if !bordiCambiati(a, fai(60, 3)) {
		t.Fatal("scritta diversa: va riletta")
	}
}
