package traduce

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func pieno(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestImpronta(t *testing.T) {
	a := pieno(300, 80, color.RGBA{10, 10, 10, 255})
	b := pieno(300, 80, color.RGBA{10, 10, 10, 255})
	if d := calcolaImpronta(a).differenza(calcolaImpronta(b)); d != 0 {
		t.Fatalf("immagini uguali, differenza %v", d)
	}
	// "testo" bianco su metà immagine: deve risultare cambiata
	for y := 20; y < 60; y++ {
		for x := 0; x < 150; x++ {
			b.SetRGBA(x, y, color.RGBA{250, 250, 250, 255})
		}
	}
	if d := calcolaImpronta(a).differenza(calcolaImpronta(b)); d < 20 {
		t.Fatalf("cambiamento non visto, differenza %v", d)
	}
	// un solo pixel diverso (animazione minima): quasi nessuna differenza
	c := pieno(300, 80, color.RGBA{10, 10, 10, 255})
	c.SetRGBA(5, 5, color.RGBA{255, 0, 0, 255})
	if d := calcolaImpronta(a).differenza(calcolaImpronta(c)); d > 1 {
		t.Fatalf("troppo sensibile, differenza %v", d)
	}
}

func TestPreparaPerOCR(t *testing.T) {
	src := pieno(400, 100, color.RGBA{40, 40, 60, 255})
	for x := 50; x < 350; x++ {
		src.SetRGBA(x, 50, color.RGBA{230, 230, 230, 255})
	}
	var buf bytes.Buffer
	if err := preparaPerOCR(src, &buf); err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dy() != 300 || img.Bounds().Dx() != 1200 {
		t.Fatalf("ingrandimento sbagliato: %v", img.Bounds())
	}
	if fattoreIngrandimento(1500, 1500) != 1 || fattoreIngrandimento(3000, 100) != 1 || fattoreIngrandimento(800, 150) != 3 {
		t.Fatal("fattore di ingrandimento sbagliato")
	}
}

func TestPagina4K(t *testing.T) {
	if fattorePagina(3840, 2160) != 0.5 || fattorePagina(1920, 1080) != 1 {
		t.Fatal("fattore pagina")
	}
	src := image.NewRGBA(image.Rect(0, 0, 3840, 2160))
	var buf bytes.Buffer
	f, err := preparaPaginaPerOCR(src, &buf)
	if err != nil || f != 0.5 {
		t.Fatal(err, f)
	}
	b := buf.Bytes()
	if len(b) < 54 || b[0] != 'B' || b[1] != 'M' || binary.LittleEndian.Uint32(b[18:]) != 1920 || binary.LittleEndian.Uint32(b[22:]) != 1080 {
		t.Fatalf("attesa BMP 1920x1080: %d byte", len(b))
	}
}

func TestStranezza(t *testing.T) {
	if stranezza("We have to leave before sunrise.") != 0 {
		t.Error("frase pulita")
	}
	if stranezza("W£ h@ve t© l€ave") <= stranezza("We have to leave") {
		t.Error("simboli strani")
	}
	if stranezza("") != 1 {
		t.Error("vuoto")
	}
}

func TestLuminoso(t *testing.T) {
	img := pieno(100, 40, color.RGBA{40, 60, 30, 255})
	for x := 10; x < 30; x++ {
		for y := 16; y < 25; y++ {
			img.SetRGBA(x, y, color.RGBA{250, 220, 40, 255}) // giallo
		}
	}
	var buf bytes.Buffer
	if err := preparaLuminosoPerOCR(img, &buf); err != nil {
		t.Fatal(err)
	}
	im, _ := png.Decode(&buf)
	f := im.Bounds().Dx() / 100
	if r, _, _, _ := im.At(15*f, 20*f).RGBA(); r > 0x2000 {
		t.Error("il giallo deve diventare nero")
	}
	if r, _, _, _ := im.At(60*f, 5*f).RGBA(); r == 0 {
		t.Error("lo sfondo deve diventare bianco")
	}
}

func TestCelleDiBlocchi(t *testing.T) {
	r := image.Rect(0, 0, 3200, 1200) // 100x100 px per zona
	c := celleDiBlocchi([]Blocco{{X: 150, Y: 250, W: 100, H: 30}}, r)
	n := 0
	for _, v := range c {
		if v {
			n++
		}
	}
	if n != 2 || !c[2*celleBordiX+1] || !c[2*celleBordiX+2] {
		t.Fatalf("zone sotto la scritta sbagliate: %d", n)
	}
	if c2 := celleDiBlocchi(nil, image.Rect(0, 0, 4, 4)); len(c2) != celleBordiX*celleBordiY {
		t.Fatal("lunghezza")
	}
}
