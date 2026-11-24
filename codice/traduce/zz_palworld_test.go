package traduce

import (
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"testing"
)

func TestPalworldColori(t *testing.T) {
	f, err := os.Open("/tmp/claude-0/pw/menu.jpg")
	if err != nil {
		t.Skip()
	}
	src, _ := jpeg.Decode(f)
	img := image.NewRGBA(src.Bounds())
	draw.Draw(img, img.Bounds(), src, image.Point{}, draw.Src)
	for _, b := range []Blocco{
		{Testo: "Start Game", X: 183, Y: 31, W: 130, H: 22, Em: 23},
		{Testo: "Join Multiplayer Game", X: 119, Y: 74, W: 258, H: 24, Em: 23},
		{Testo: "Global Palbox", X: 176, Y: 177, W: 144, H: 20, Em: 23},
		{Testo: "Options", X: 207, Y: 252, W: 82, H: 23, Em: 23},
		{Testo: "News", X: 220, Y: 290, W: 56, H: 17, Em: 23},
	} {
		coloriBlocco(img, &b)
		contornoLettere(img, &b)
		ombraLettere(img, &b)
		fmt.Println(b.HaOmbraG, b.OmbraGioco, b.Testo, "sfondo", b.Sfondo, "ink", b.Inchiostro, "cont", b.HaContorno, b.Contorno, "tratto", b.Tratto)
	}
}

func TestPalworldPulita(t *testing.T) {
	f, err := os.Open("/tmp/claude-0/pw/menu.jpg")
	if err != nil {
		t.Skip()
	}
	src, _ := jpeg.Decode(f)
	img := image.NewRGBA(src.Bounds())
	draw.Draw(img, img.Bounds(), src, image.Point{}, draw.Src)
	bl := []Blocco{
		{Testo: "Start Game", X: 183, Y: 31, W: 130, H: 22, Em: 23},
		{Testo: "Join Multiplayer Game", X: 119, Y: 74, W: 258, H: 24, Em: 23},
		{Testo: "Join Multiplayer Game (Invite Code)", X: 42, Y: 117, W: 412, H: 25, Em: 23},
		{Testo: "Global Palbox", X: 176, Y: 177, W: 144, H: 20, Em: 23},
		{Testo: "Survival Guide", X: 173, Y: 213, W: 149, H: 24, Em: 23},
		{Testo: "Options", X: 207, Y: 252, W: 82, H: 23, Em: 23},
		{Testo: "News", X: 220, Y: 290, W: 56, H: 17, Em: 23},
		{Testo: "Credits", X: 210, Y: 326, W: 75, H: 19, Em: 23},
		{Testo: "Quit Game", X: 193, Y: 362, W: 109, H: 23, Em: 23},
	}
	for i := range bl {
		coloriBlocco(img, &bl[i])
		contornoLettere(img, &bl[i])
		bl[i].Img = img
		fmt.Println(bl[i].Testo, bl[i].Sfondo, bl[i].Inchiostro, bl[i].HaContorno, bl[i].Contorno)
	}
	for i := range bl {
		bl[i].Marg = margineToppa(bl[i], bl)
	}
	p := pulisciPagina(img, bl)
	o, _ := os.Create("/tmp/claude-0/pw/pulita.png")
	png.Encode(o, p)
	o.Close()
}

func carica(t *testing.T, p string) *image.RGBA {
	f, err := os.Open(p)
	if err != nil {
		t.Skip()
	}
	src, _ := jpeg.Decode(f)
	img := image.NewRGBA(src.Bounds())
	draw.Draw(img, img.Bounds(), src, image.Point{}, draw.Src)
	return img
}

func TestPalworldMouse(t *testing.T) {
	img := carica(t, "/tmp/claude-0/pw/menu.jpg")
	hov := carica(t, "/tmp/claude-0/pw/hover.jpg")
	if img.Bounds() != hov.Bounds() {
		t.Skip("grandezze diverse")
	}
	bl := []Blocco{
		{Testo: "Global Palbox", X: 176, Y: 177, W: 144, H: 20, Em: 23},
		{Testo: "Survival Guide", X: 173, Y: 213, W: 149, H: 24, Em: 23},
		{Testo: "Options", X: 207, Y: 252, W: 82, H: 23, Em: 23},
		{Testo: "News", X: 220, Y: 290, W: 56, H: 17, Em: 23},
	}
	for i := range bl {
		coloriBlocco(img, &bl[i])
		bl[i].Img = img
	}
	for i := range bl {
		bl[i].Marg = margineToppa(bl[i], bl)
	}
	p := pulisciPagina(img, bl)
	for i := range bl {
		bl[i].Pulita = p
	}
	nuovi, ok := ricolora(hov, bl)
	fmt.Println("cambiato", ok)
	for i := range nuovi {
		fmt.Println(nuovi[i].Testo, nuovi[i].X, nuovi[i].Y, nuovi[i].W, nuovi[i].H, nuovi[i].Em, nuovi[i].Inchiostro, nuovi[i].Sfondo)
	}
	o, _ := os.Create("/tmp/claude-0/pw/pulita-mouse.png")
	png.Encode(o, nuovi[0].Pulita)
	o.Close()
}
