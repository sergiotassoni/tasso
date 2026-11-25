package traduce

import (
	"context"
	"path/filepath"
	"testing"
)

// etichetta: un suggerimento di un tasto letto dall'OCR, largo come lo scriverebbe il gioco con
// lettere alte px pixel (stessa stima di larghezzaLettere).
func etichetta(testo string, px float64) Blocco {
	w := int(larghezzaLettere(testo)*px + 0.5)
	return Blocco{Testo: testo, X: 400, Y: 600, W: w, H: int(px * 0.75), Righe: 1, Em: px, LargMax: w}
}

// PC Cafe Simulator, tutorial (26/11): "Hold" sopra il tasto T diventava "Tieni premuto" e
// usciva dal riquadro. Ora: corta ("Tieni"), mai oltre il riquadro; se non ci sta, inglese.
func TestRiquadroComandi(t *testing.T) {
	casi := []struct {
		inglese, it, atteso string // atteso "" = resta l'inglese
	}{
		{"Hold", "Tieni premuto", "Tieni"},
		{"HOLD", "TIENI PREMUTO", "TIENI"},
		{"Skip", "Salta", "Salta"},
		{"SKIP", "SALTA", "SALTA"},
		{"Press", "Premi", "Premi"},
		{"Collapse", "Comprimi", "Comprimi"},
		{"Hold", "Tenere una svolta più brusca.", ""}, // traduzione sbagliata e lunghissima: inglese
		{"Hold Shift", "Tieni premuto Shift", "Tieni Shift"},
		{"Press to sprint", "Premere per scattare", "Premere per scattare"}, // rimpicciolita ci sta: resta intera
		{"Tap", "Premere brevemente", ""},                                   // nemmeno corta ci sta: inglese
	}
	for _, c := range casi {
		b := etichetta(c.inglese, 18)
		b.Tradotto = c.it
		m := adattaAlRiquadro(&b)
		switch {
		case c.atteso == "" && m == "":
			t.Errorf("%q → %q: doveva restare in inglese", c.inglese, b.Tradotto)
		case c.atteso != "" && m != "":
			t.Errorf("%q → %q: scartata (%s), attesa %q", c.inglese, c.it, m, c.atteso)
		case c.atteso != "" && b.Tradotto != c.atteso:
			t.Errorf("%q → %q, attesa %q", c.inglese, b.Tradotto, c.atteso)
		}
		if m == "" && !ciStaNelRiquadro(b, b.Tradotto) {
			t.Errorf("%q → %q: esce dal riquadro", c.inglese, b.Tradotto)
		}
	}
	// con il bordo vero del pulsante trovato più in là, la frase intera ci sta
	b := etichetta("Hold", 18)
	b.Tradotto, b.LargMax, b.Bordo = "Tieni premuto", 200, true
	if m := adattaAlRiquadro(&b); m != "" || b.Tradotto != "Tieni premuto" {
		t.Errorf("pulsante largo: %q (%s)", b.Tradotto, m)
	}
	// l'icona del tasto a destra: il limite si ferma prima (LargMax già tagliato da preparaScritta)
	b = etichetta("Hold", 18)
	b.Tradotto, b.IconaDx, b.LargMax = "Tieni premuto", b.X+b.W+6, b.W
	if limiteComando(b) > b.IconaDx-b.X {
		t.Errorf("il limite va sopra l'icona del tasto: %d", limiteComando(b))
	}
	// le voci dei menu non sono etichette dei comandi: restano con le loro regole
	for _, s := range []string{"Options", "Back", "Continue", "A New Beginning", "Chapter 1", "Equipment"} {
		if etichettaComando(Blocco{Testo: s, Righe: 1}) {
			t.Errorf("%q presa per un suggerimento di un tasto", s)
		}
	}
	for _, s := range []string{"Hold", "Hold T Skip", "Shift Sprint", "Press E", "Collapse"} {
		if !etichettaComando(Blocco{Testo: s, Righe: 1}) {
			t.Errorf("%q non riconosciuta come suggerimento di un tasto", s)
		}
	}
}

// Il percorso vero: frase pronta del catalogo, poi la scelta che sta nel riquadro.
func TestRiquadroComandiDalCatalogo(t *testing.T) {
	tr := NuovoTraduttore(filepath.Join(t.TempDir(), "mem.json"), glossarioBase(), motoreDizionario{})
	for _, ing := range []string{"Hold", "HOLD", "Skip", "Press", "Collapse", "Hold T Skip", "Hold to skip", "Sprint by Holding Shift"} {
		r := tr.TraduciScritta(context.Background(), ing)
		b := etichetta(ing, 18)
		b.Tradotto = rifinisci(b.Testo, r.Tradotto)
		prima := b.Tradotto
		m := adattaAlRiquadro(&b)
		if m == "" && !ciStaNelRiquadro(b, b.Tradotto) {
			t.Errorf("%q → %q esce dal riquadro", ing, b.Tradotto)
		}
		t.Logf("%q → %q → %q %s", ing, prima, b.Tradotto, m)
	}
	r := tr.TraduciScritta(context.Background(), "Hold")
	b := etichetta("Hold", 18)
	b.Tradotto = r.Tradotto
	if adattaAlRiquadro(&b); b.Tradotto != "Tieni" {
		t.Errorf("\"Hold\" nel riquadro piccolo: %q (dal catalogo %q)", b.Tradotto, r.Tradotto)
	}
}

func TestVersioneCorta(t *testing.T) {
	for in, atteso := range map[string]string{
		"Tieni premuto T: salta":      "Tieni T: salta",
		"Corri tenendo premuto Shift": "Corri tenendo Shift",
		"premere per scattare":        "premi per scattare",
		"TIENI PREMUTO":               "TIENI",
		"Comprimi":                    "Riduci",
	} {
		if v, ok := versioneCorta(in); !ok || v != atteso {
			t.Errorf("%q → %q, atteso %q", in, v, atteso)
		}
	}
	if _, ok := versioneCorta("Salta"); ok {
		t.Error("niente da accorciare in \"Salta\"")
	}
}
