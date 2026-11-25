package traduce

// Quinto giro sul banco di prova vero (OCR di Windows, risultati-5): una frase letta in due pezzi
// con una parola spezzata in mezzo ("ar" | "Ire"). Regola universale dei pezzi fratelli: pezzi
// sulla stessa linea, della stessa altezza e vicini sono UNA frase; si giudica la riga intera e,
// se non va, restano in inglese tutti: mai un fratello tradotto da solo.

import (
	"image"
	"strings"
	"testing"
	"time"
)

// 30-caricamento: "TIP: Crouching makes less noise when enemies are nearby." (y≈945) letta così:
//
//	[563,945]  "TIP: Crouching makes less noise when enemies ar"  -> affidabilità 0
//	[1217,946] "Ire nearby."                                      -> "Sono nelle vicinanze."
func TestFratelliParolaSpezzata(t *testing.T) {
	img := apriBanco3(t, "30-caricamento") // ritaglio da (500,900)
	a := rigaParole(945, 29, "TIP:", 563.0, 46.0, "Crouching", 622.0, 133.0, "makes", 766.0, 84.0, "less", 862.0, 48.0,
		"noise", 922.0, 69.0, "when", 1002.0, 69.0, "enemies", 1084.0, 112.0, "ar", 1207.0, 26.0)
	b := rigaParole(946, 29, "Ire", 1217.0, 30.0, "nearby.", 1259.0, 95.0)
	for _, im := range []bool{false, true} {
		righe := []RigaOCR{a, b}
		var bl []Blocco
		if im {
			bl = blocchiLetti(img, []RigaOCR{spostaRiga(a, 500, 900), spostaRiga(b, 500, 900)}, 1, nil, time.Now())
		} else {
			bl = blocchiLetti(nil, righe, 1, nil, time.Now())
		}
		if len(bl) != 1 {
			t.Errorf("immagine=%v: i due pezzi non uniti: %q", im, chiaviBlocchi(bl))
		}
		for i := range bl {
			x := bl[i]
			if motivoNonTradurre(x) == "" && controllaCompleta(func() *image.RGBA {
				if im {
					return img
				}
				return nil
			}(), &x, bl) == "" {
				t.Errorf("immagine=%v: %q si mostrerebbe tradotta", im, x.Testo)
			}
		}
	}
}

// Se i fratelli restano divisi (unione non riuscita), quello letto bene non si traduce da solo.
func TestFratelloMaiDaSolo(t *testing.T) {
	sx := bloccoDaParole(rigaParole(945, 29, "TIP:", 563.0, 46.0, "Crouching", 622.0, 133.0, "makes", 766.0, 84.0, "less", 862.0, 48.0,
		"noise", 922.0, 69.0, "when", 1002.0, 69.0, "enemies", 1084.0, 112.0, "ar", 1207.0, 26.0).Parole, 1)
	sx.Testo, sx.Affid = "TIP: Crouching makes less noise when enemies ar", letturaDubbia
	dx := bloccoDaParole(rigaParole(946, 29, "Ire", 1217.0, 30.0, "nearby.", 1259.0, 95.0).Parole, 1)
	dx.Testo, dx.Affid = "Ire nearby.", 1
	tutte := []Blocco{sx, dx}
	if m := controllaCompleta(nil, &dx, tutte); m == "" {
		t.Errorf("%q tradotta da sola accanto a un fratello in inglese", dx.Testo)
	}
	// anche con un fratello sinistro buono, "Ire" in testa è un pezzo di parola spezzata
	sx.Affid = 1
	if m := motivoFratello(dx, []Blocco{sx, dx}); m == "" {
		t.Errorf("%q con un fratello a sinistra: doveva restare in inglese", dx.Testo)
	}
	// "are nearby." (parola vera) accanto a un fratello buono: nessun motivo dai fratelli
	ok := dx
	ok.Testo = "Are nearby."
	if m := motivoFratello(ok, []Blocco{sx, ok}); m != "" {
		t.Errorf("%q: %s", ok.Testo, m)
	}
}

func TestPezziDiParola(t *testing.T) {
	for _, s := range []string{"Ire nearby.", "Ny house", "Ese"} {
		if !iniziaConFrammento(s) {
			t.Errorf("%q: doveva essere un pezzo di parola", s)
		}
	}
	for _, s := range []string{"Are nearby.", "The end", "HP 50", "OK", "Ok", "Back", "Yes", "4K", "Apply"} {
		if iniziaConFrammento(s) {
			t.Errorf("%q: non è un pezzo di parola", s)
		}
	}
	if !finisceConFrammento("when enemies ar") || finisceConFrammento("when enemies are") || finisceConFrammento("Show more") {
		t.Error("finisceConFrammento")
	}
	if _, a := correggiRigaOCR("TIP: Crouching makes less noise when enemies ar Ire nearby."); a != letturaDubbia {
		t.Errorf("riga con la parola spezzata \"ar Ire\": affidabilità %d", a)
	}
	if pezzoTroncato("Game saved.", "Ire") || pezzoTroncato("TIP:", "Ese") {
		t.Error("dopo la fine di una frase non si unisce per un pezzo di parola")
	}
}

// Le voci vere di una fila di pulsanti restano separate e si traducono ognuna per conto suo,
// anche se una è letta male.
func TestFratelliNonToccanoPulsanti(t *testing.T) {
	r := rigaParole(1017, 24, "Apply", 140.0, 81.0, "Reset", 240.8, 77.0, "to", 324.8, 27.0, "Default", 358.6, 99.0, "Back", 477.9, 68.0)
	bl := blocchiLetti(nil, []RigaOCR{r}, 1, nil, time.Now())
	if got := strings.Join(chiaviBlocchi(bl), " | "); got != "Apply | Reset to Default | Back" {
		t.Fatalf("fila di pulsanti: %q", got)
	}
	bl[1].Affid = letturaDubbia
	for _, i := range []int{0, 2} {
		if m := motivoFratello(bl[i], bl); m != "" {
			t.Errorf("%q scartata per il pulsante vicino: %s", bl[i].Testo, m)
		}
	}
	f := rigaParole(1016, 27, "Show", 141.0, 70.0, "more", 219.0, 66.0, "Back", 360.0, 65.0)
	bf := blocchiLetti(nil, []RigaOCR{f}, 1, nil, time.Now())
	for i := range bf {
		if m := motivoFratello(bf[i], bf); m != "" {
			t.Errorf("%q: %s", bf[i].Testo, m)
		}
	}
}
