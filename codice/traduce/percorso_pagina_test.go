package traduce

import (
	"testing"
	"time"
)

// Il percorso dalle righe dell'OCR alle scritte: riga spezzata dall'OCR riunita, etichetta
// divisa al separatore, letture corrette con le frasi pronte, spazzatura lasciata in inglese.
func TestBlocchiLettiPercorso(t *testing.T) {
	righe := []RigaOCR{
		// "FOCUS > Move to fill" e "the progress bur": due righe per l'OCR, una per il gioco
		{Parole: []ParolaOCR{{T: "FOCUS", X: 20, Y: 900, W: 80, H: 24}, {T: ">", X: 108, Y: 903, W: 12, H: 18}, {T: "Move", X: 128, Y: 900, W: 60, H: 24}, {T: "to", X: 194, Y: 900, W: 22, H: 24}, {T: "fill", X: 222, Y: 900, W: 40, H: 24}}},
		{Parole: []ParolaOCR{{T: "the", X: 270, Y: 901, W: 36, H: 24}, {T: "progress", X: 312, Y: 901, W: 100, H: 24}, {T: "bur", X: 418, Y: 901, W: 40, H: 24}}},
		// menu: "JOIN" e "FRIEND" spezzate, "UBIIC GAME" letta male
		{Parole: []ParolaOCR{{T: "JOIN", X: 300, Y: 300, W: 90, H: 40}}},
		{Parole: []ParolaOCR{{T: "FRIEND", X: 402, Y: 301, W: 140, H: 40}}},
		{Parole: []ParolaOCR{{T: "UBIIC", X: 300, Y: 220, W: 110, H: 40}, {T: "GAME", X: 420, Y: 220, W: 100, H: 40}}},
		{Parole: []ParolaOCR{{T: "Xqwvz", X: 300, Y: 600, W: 110, H: 40}, {T: "zzkt", X: 420, Y: 600, W: 100, H: 40}}},
	}
	bl := blocchiLetti(nil, righe, 1, nil, time.Now())
	vuoi := map[string]int{
		"FOCUS":                         letturaCatalog,
		"Move to fill the progress bar": letturaCatalog,
		"JOIN FRIEND":                   letturaCatalog,
		"PUBLIC GAME":                   letturaCatalog,
	}
	trovate := map[string]bool{}
	for _, b := range bl {
		if a, ok := vuoi[b.Testo]; ok {
			trovate[b.Testo] = true
			if b.Affid != a {
				t.Errorf("%q: affidabilità %d, attesa %d", b.Testo, b.Affid, a)
			}
		}
		if b.Testo == "Xqwvz zzkt" && motivoNonTradurre(b) == "" {
			t.Errorf("spazzatura da tradurre: %+v", b)
		}
	}
	for k := range vuoi {
		if !trovate[k] {
			t.Errorf("manca %q in %v", k, testi(bl))
		}
	}
}

func testi(b []Blocco) []string {
	var out []string
	for _, x := range b {
		out = append(out, x.Testo+" ["+x.Grezzo+"]")
	}
	return out
}
