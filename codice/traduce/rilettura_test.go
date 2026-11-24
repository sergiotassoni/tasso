package traduce

import (
	"image"
	"image/color"
	"testing"
)

// La riga letta male viene riletta (ingrandita, in tre preparazioni) e vince la lettura buona.
func TestRileggiIncerte(t *testing.T) {
	sf, ink := color.RGBA{30, 60, 160, 255}, color.RGBA{245, 245, 245, 255} // bianco su pulsante blu
	img := immagineFinta(800, 300, sf)
	fine := parolaFinta(img, 100, 100, 10, 14, 30, ink)
	righe := []RigaOCR{
		{Parole: []ParolaOCR{{T: "qurmt", X: 100, Y: 100, W: 70, H: 30}, {T: "zzkt", X: 180, Y: 100, W: float64(fine - 180), H: 30}}},
		{Parole: []ParolaOCR{{T: "Settings", X: 100, Y: 200, W: 120, H: 30}}}, // già buona: non si rilegge
	}
	chiamate := 0
	leggi := func(g *image.Gray, dove []image.Rectangle) ([]RigaOCR, error) {
		chiamate++
		if len(dove) != 3 {
			t.Fatalf("pezzi nel mosaico: %d (attesi 3: una riga × 3 preparazioni)", len(dove))
		}
		for _, d := range dove {
			if !d.In(g.Bounds()) {
				t.Fatalf("pezzo fuori dal mosaico")
			}
		}
		testi := [][]string{{"qurmt", "zzkt"}, {"PUBLIC", "GAME"}, {"PUBIIC", "GAMF"}}
		var out []RigaOCR
		for i, d := range dove {
			x := float64(d.Min.X + 20)
			var r RigaOCR
			for _, w := range testi[i] {
				r.Parole = append(r.Parole, ParolaOCR{T: w, X: x, Y: float64(d.Min.Y + 10), W: 100, H: 60})
				x += 120
			}
			out = append(out, r)
		}
		return out, nil
	}
	out := rileggiIncerteCon(img, righe, 1, leggi)
	if chiamate != 1 {
		t.Fatalf("chiamate all'OCR: %d, attesa 1 (mosaico)", chiamate)
	}
	if got := testoParole(out[0].Parole); got != "PUBLIC GAME" {
		t.Fatalf("rilettura: %q", got)
	}
	if out[0].Parole[0].X < 60 || out[0].Parole[0].X > 140 || out[0].Parole[0].Y < 60 || out[0].Parole[0].Y > 130 {
		t.Fatalf("coordinate non riportate nella cattura: %+v", out[0].Parole[0])
	}
	if testoParole(out[1].Parole) != "Settings" {
		t.Fatalf("riga buona cambiata: %+v", out[1])
	}
	// stessi pixel: la seconda volta niente OCR (memoria delle riletture)
	out2 := rileggiIncerteCon(img, righe, 1, leggi)
	if chiamate != 1 || testoParole(out2[0].Parole) != "PUBLIC GAME" {
		t.Fatalf("rilettura non ricordata: chiamate %d, %q", chiamate, testoParole(out2[0].Parole))
	}
}

// Le tre preparazioni: ingrandite, e "solo le lettere" nero su bianco anche per scritte scure su chiaro.
func TestVariantiRiga(t *testing.T) {
	for _, c := range [][2]color.RGBA{{{20, 20, 20, 255}, {250, 250, 250, 255}}, {{240, 235, 220, 255}, {40, 30, 30, 255}}} {
		img := immagineFinta(120, 40, c[0])
		parolaFinta(img, 20, 8, 4, 8, 24, c[1])
		v := variantiRiga(img, 3)
		if len(v) != 3 || v[0].Bounds().Dx() != 360 || v[2].Bounds().Dy() != 120 {
			t.Fatalf("varianti sbagliate")
		}
		// nella terza le lettere sono nere e lo sfondo bianco
		if v[2].GrayAt(5, 5).Y != 255 || v[2].GrayAt(3*24, 3*20).Y != 0 {
			t.Fatalf("solo-lettere: sfondo %d lettera %d", v[2].GrayAt(5, 5).Y, v[2].GrayAt(3*24, 3*20).Y)
		}
	}
}
