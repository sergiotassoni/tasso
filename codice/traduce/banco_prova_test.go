package traduce

import (
	"context"
	"encoding/json"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// motore finto "italiano": traduce parola per parola da un piccolo dizionario; le parole che non
// conosce le lascia in inglese (come fa a volte il motore vero), così si vede che le righe mezze
// tradotte NON arrivano mai sullo schermo.
type motoreDizionario struct{}

func (motoreDizionario) Nome() string { return "Mozilla" }
func (motoreDizionario) Traduci(_ context.Context, r RichiestaTraduzione) (string, error) {
	diz := map[string]string{"the": "il", "bridge": "ponte", "collapsed": "è crollato", "behind": "dietro", "you": "di te",
		"open": "apri", "door": "porta", "use": "usa", "to": "per", "jump": "saltare"}
	var out []string
	for _, w := range strings.Fields(r.Testo) {
		k := strings.ToLower(strings.Trim(w, ".,!?"))
		if v, ok := diz[k]; ok {
			out = append(out, v)
		} else {
			out = append(out, w)
		}
	}
	return strings.Join(out, " "), nil
}

// Il banco di prova su Linux: immagini finte + OCR finto (file .ocr.json), percorso vero.
func TestBancoProvaCartellaFinta(t *testing.T) {
	dir := t.TempDir()
	sf, ink := color.RGBA{20, 22, 30, 255}, color.RGBA{240, 240, 240, 255}
	img := immagineFinta(900, 400, sf)
	f1 := parolaFinta(img, 50, 40, 10, 14, 30, ink)
	f2 := parolaFinta(img, 50, 120, 6, 14, 30, ink)
	f3 := parolaFinta(img, 50, 200, 4, 14, 30, ink)
	f4 := parolaFinta(img, 50, 280, 12, 10, 22, ink)
	fh, _ := os.Create(filepath.Join(dir, "gioco1.png"))
	_ = png.Encode(fh, img)
	fh.Close()
	righe := []RigaOCR{
		{Parole: []ParolaOCR{{T: "Prcss", X: 50, Y: 40, W: 80, H: 30}, {T: "any", X: 140, Y: 40, W: 50, H: 30}, {T: "kev", X: 200, Y: 40, W: 50, H: 30}, {T: "to", X: 260, Y: 40, W: 30, H: 30}, {T: "continue", X: 300, Y: 40, W: float64(f1 - 300), H: 30}}},
		{Parole: []ParolaOCR{{T: "Open", X: 50, Y: 120, W: 60, H: 30}, {T: "the", X: 116, Y: 120, W: 40, H: 30}, {T: "vault", X: 162, Y: 120, W: float64(f2 - 162), H: 30}}}, // "vault" non nel dizionario finto: mezza
		{Parole: []ParolaOCR{{T: "FELS", X: 50, Y: 200, W: float64(f3 - 50), H: 30}}},
		{Parole: []ParolaOCR{{T: "The", X: 50, Y: 280, W: 40, H: 22}, {T: "bridge", X: 96, Y: 280, W: 70, H: 22}, {T: "collapsed", X: 172, Y: 280, W: float64(f4 - 172), H: 22}}},
	}
	b, _ := json.Marshal(righe)
	_ = os.WriteFile(filepath.Join(dir, "gioco1.ocr.json"), b, 0o644)
	tr := NuovoTraduttore(filepath.Join(dir, "mem.json"), glossarioBase(), motoreDizionario{})
	o := opzioniBanco{leggi: leggiOCRFinto(filepath.Join(dir, "gioco1.png")), tr: tr, disegna: disegnaCopertura}
	esiti, err := bancoCartella(dir, o)
	if err != nil || len(esiti) != 1 {
		t.Fatalf("banco: %v %d", err, len(esiti))
	}
	e := esiti[0]
	if e.MezzeMostrate != 0 {
		t.Fatalf("righe mezze tradotte disegnate: %d", e.MezzeMostrate)
	}
	visti := map[string]infoRiga{}
	for _, r := range e.Righe {
		visti[r.Corretto] = r
	}
	if r := visti["Press any key to continue"]; r.Scartata != "" || r.Fonte != "catalogo" || r.Tradotto != "Premi un tasto qualsiasi per continuare" {
		t.Errorf("frase pronta: %+v", r)
	}
	if r := visti["Open the vault"]; !strings.HasPrefix(r.Scartata, "mezza") {
		t.Errorf("riga mezza tradotta non scartata: %+v", r)
	}
	if r := visti["FELS"]; r.Scartata == "" {
		t.Errorf("lettura incerta tradotta: %+v", r)
	}
	if r := visti["The bridge collapsed"]; r.Scartata != "" || r.Tradotto == "" {
		t.Errorf("frase normale: %+v", r)
	}
	for _, f := range []string{"banco-prova.txt", "banco-prova.html", "gioco1-tradotto.png"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("manca %s", f)
		}
	}
	txt, _ := os.ReadFile(filepath.Join(dir, "banco-prova.txt"))
	if !strings.Contains(string(txt), "Righe mezze tradotte DISEGNATE: 0") {
		t.Errorf("riepilogo: %s", txt)
	}
}

// TASSO_BANCO=<cartella> go test -run TestBancoProvaCartellaVera: il banco su una cartella di
// schermate con i file .ocr.json (OCR finto), senza motore (solo frasi pronte e memoria).
func TestBancoProvaCartellaVera(t *testing.T) {
	dir := os.Getenv("TASSO_BANCO")
	if dir == "" {
		t.Skip("TASSO_BANCO non impostata")
	}
	var motori []Motore
	if os.Getenv("TASSO_BANCO_MOTORE") == "finto" {
		motori = append(motori, motoreRovescio{}) // ogni parola "tradotta" (rovesciata): si vede cosa arriva al motore
	}
	tr := NuovoTraduttore(filepath.Join(t.TempDir(), "mem.json"), glossarioBase(), motori...)
	var esiti []esitoImmagine
	for _, p := range immaginiCartella(dir) {
		o := opzioniBanco{leggi: leggiOCRFinto(p), tr: tr, disegna: disegnaCopertura}
		img, err := apriImmagineRGBA(p)
		if err != nil {
			t.Fatal(err)
		}
		e, pronti := elaboraImmagine(img, filepath.Base(p), o)
		_ = disegnaCopertura(img, pronti, strings.TrimSuffix(p, filepath.Ext(p))+"-tradotto.png")
		t.Logf("%s: mostrate %d, in inglese %d, mezze disegnate %d", e.Nome, e.Mostrate, e.Scartate, e.MezzeMostrate)
		esiti = append(esiti, e)
	}
	if err := scriviReportBanco(dir, esiti, nil); err != nil {
		t.Fatal(err)
	}
}

// motoreRovescio: motore finto che "traduce" rovesciando ogni parola (nessuna parola resta inglese).
type motoreRovescio struct{}

func (motoreRovescio) Nome() string { return "Mozilla" }
func (motoreRovescio) Traduci(_ context.Context, r RichiestaTraduzione) (string, error) {
	var out []string
	for _, w := range strings.Fields(r.Testo) {
		if segnapostoTasto(w) {
			out = append(out, w)
			continue
		}
		c := []rune(w)
		for i, j := 0, len(c)-1; i < j; i, j = i+1, j-1 {
			c[i], c[j] = c[j], c[i]
		}
		out = append(out, string(c))
	}
	return strings.Join(out, " "), nil
}
