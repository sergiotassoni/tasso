package traduce

import (
	"testing"
	"time"
)

// Gioco in tedesco: la riga resta come l'ha letta l'OCR (niente "Spiel been den") e si traduce.
func TestBlocchiNonInglese(t *testing.T) {
	r := RigaOCR{Parole: []ParolaOCR{{T: "Spiel", X: 300, Y: 300, W: 100, H: 40}, {T: "beenden", X: 412, Y: 300, W: 160, H: 40}}}
	bl := blocchiLettiIn(nil, []RigaOCR{r}, 1, nil, time.Now(), false)
	if len(bl) != 1 || bl[0].Testo != "Spiel beenden" || !bl[0].NonInglese {
		t.Fatalf("letto: %+v", bl)
	}
	if m := motivoNonTradurre(bl[0]); m != "" {
		t.Fatalf("scartata: %q", m)
	}
}
