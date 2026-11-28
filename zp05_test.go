// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

// zp05_test.go: CTRL-ZP-05. Ruota, Elimina, Estrai e Organizza tengono moduli, segnalibri,
// allegati, lingua, etichette e titolo (ZP05-1); le pagine scritte con gli spazi (ZP05-2).

import (
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

type catalogo struct {
	pagine, campi, segnalibri int
	lang, etichette, allegati bool
	titolo                    string
}

func leggiCatalogo(t *testing.T, b []byte) catalogo {
	t.Helper()
	ctx, _, err := leggi(b, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.EnsurePageCount(); err != nil {
		t.Fatal(err)
	}
	var c catalogo
	c.pagine = ctx.PageCount
	r := ctx.RootDict
	if af, _ := ctx.DereferenceDict(r["AcroForm"]); af != nil {
		f, _ := ctx.DereferenceArray(af["Fields"])
		c.campi = len(f)
	}
	if o, _ := ctx.DereferenceDict(r["Outlines"]); o != nil {
		cur := o["First"]
		for cur != nil && c.segnalibri < 1000 {
			d, _ := ctx.DereferenceDict(cur)
			if d == nil {
				break
			}
			c.segnalibri++
			cur = d["Next"]
		}
	}
	_, c.lang = r.Find("Lang")
	_, c.etichette = r.Find("PageLabels")
	if n, _ := ctx.DereferenceDict(r["Names"]); n != nil {
		_, c.allegati = n.Find("EmbeddedFiles")
	}
	if ctx.Info != nil {
		if d, _ := ctx.DereferenceDict(*ctx.Info); d != nil {
			if s, ok := d["Title"].(types.StringLiteral); ok {
				c.titolo = s.Value()
			}
			if s, ok := d["Title"].(types.HexLiteral); ok {
				c.titolo = s.Value()
			}
		}
	}
	return c
}

func seqTutte(n int) []PaginaSeq {
	s := make([]PaginaSeq, n)
	for i := range s {
		s[i] = PaginaSeq{Fonte: 0, Pagina: i + 1}
	}
	return s
}

func TestZP05OrganizzaTieneIlCatalogo(t *testing.T) {
	for _, nome := range []string{"moduli.pdf", "segnalibri.pdf", "ricco.pdf"} {
		b, err := os.ReadFile("testdata/" + nome)
		if err != nil {
			t.Fatal(err)
		}
		prima := leggiCatalogo(t, b)
		// Ruota solo la pagina 1
		s := seqTutte(prima.pagine)
		s[0].Rot = 90
		out, err := Organizza([][]byte{b}, s)
		if err != nil {
			t.Fatalf("%s ruota: %v", nome, err)
		}
		if dopo := leggiCatalogo(t, out); dopo != prima {
			t.Errorf("%s ruota: prima %+v dopo %+v", nome, prima, dopo)
		}
		// Ordine al contrario
		inv := seqTutte(prima.pagine)
		for i, j := 0, len(inv)-1; i < j; i, j = i+1, j-1 {
			inv[i], inv[j] = inv[j], inv[i]
		}
		out, err = Organizza([][]byte{b}, inv)
		if err != nil {
			t.Fatalf("%s inverti: %v", nome, err)
		}
		if dopo := leggiCatalogo(t, out); dopo != prima {
			t.Errorf("%s inverti: prima %+v dopo %+v", nome, prima, dopo)
		}
	}
}

func TestZP05EliminaTogliePuntatori(t *testing.T) {
	b, _ := os.ReadFile("testdata/segnalibri.pdf")
	prima := leggiCatalogo(t, b)
	if prima.segnalibri != 5 || prima.pagine != 5 {
		t.Fatalf("file di prova: %+v", prima)
	}
	// Elimina le pagine 2 e 3: i loro segnalibri spariscono, gli altri restano.
	out, err := Organizza([][]byte{b}, []PaginaSeq{{Pagina: 1}, {Pagina: 4}, {Pagina: 5}})
	if err != nil {
		t.Fatal(err)
	}
	dopo := leggiCatalogo(t, out)
	if dopo.pagine != 3 || dopo.segnalibri != 3 {
		t.Errorf("dopo elimina: %+v", dopo)
	}
	// Le pagine tolte non restano nascoste nella copia (nessun oggetto /Page in più)
	if n := strings.Count(string(out), "/Type /Page\n") + strings.Count(string(out), "/Type /Page/") + strings.Count(string(out), "/Type /Page>"); n > 3 {
		t.Errorf("pagine tolte ancora nel file: %d", n)
	}
	// Moduli: eliminando la pagina 2 resta solo il campo della pagina 1 e la casella
	m, _ := os.ReadFile("testdata/moduli.pdf")
	out, err = Organizza([][]byte{m}, []PaginaSeq{{Pagina: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if c := leggiCatalogo(t, out); c.pagine != 1 || c.campi != 2 {
		t.Errorf("moduli dopo elimina p2: %+v", c)
	}
	out, err = Organizza([][]byte{m}, []PaginaSeq{{Pagina: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if c := leggiCatalogo(t, out); c.pagine != 1 || c.campi != 1 {
		t.Errorf("moduli dopo elimina p1: %+v", c)
	}
	// Estrai (anche dal documento ricco) tiene titolo e lingua
	r, _ := os.ReadFile("testdata/ricco.pdf")
	pr := leggiCatalogo(t, r)
	out, err = Organizza([][]byte{r}, []PaginaSeq{{Pagina: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if c := leggiCatalogo(t, out); c.titolo != pr.titolo || c.lang != pr.lang || c.allegati != pr.allegati {
		t.Errorf("estrai: prima %+v dopo %+v", pr, c)
	}
}

func TestZP05RotazioneEreditata(t *testing.T) {
	b, _ := os.ReadFile("testdata/ereditato.pdf")
	s := seqTutte(leggiCatalogo(t, b).pagine)
	s[0].Rot = 90
	out, err := Organizza([][]byte{b}, s)
	if err != nil {
		t.Fatal(err)
	}
	ctx, _, err := leggi(out, "")
	if err != nil {
		t.Fatal(err)
	}
	_, _, inh, err := ctx.PageDict(bg, 1, false)
	if err != nil || inh.Rotate != 180 {
		t.Errorf("rotazione ereditata 90 + 90: %v %v", inh, err)
	}
	if _, _, inh2, _ := ctx.PageDict(bg, 2, false); inh2 == nil || inh2.Rotate != 90 {
		t.Errorf("pagina 2 deve restare a 90: %+v", inh2)
	}
}

func TestZP05PagineConSpazi(t *testing.T) {
	for _, s := range []string{"2-3", "2 - 3", "2- 3", "2 -3", " 2 – 3 "} {
		m, err := pagineScelte(s, 10)
		if err != nil || len(m) != 2 || !m[2] || !m[3] {
			t.Errorf("%q: %v %v", s, m, err)
		}
	}
	if m, err := pagineScelte("1, 4 - 5, 8-", 10); err != nil || len(m) != 6 {
		t.Errorf("misto: %v %v", m, err)
	}
	if _, err := pagineScelte("-", 10); err == nil {
		t.Error("un trattino da solo")
	}
}

// ZP05-3: una filigrana a immagine su una pagina A4 e su una A3 orizzontale mantiene le
// proporzioni dell'immagine (prima era schiacciata sulla pagina di forma diversa).
func TestZP05ImmagineSuMisureDiverse(t *testing.T) {
	b := pdfProva([]pagProva{a4("uno"), {1190.55, 841.89, 0, "due"}})
	png := pngProva(t, 200, 100, false) // larga il doppio che alta
	e := Elemento{Tipo: "immagine", Pagina: 0, X: 0.25, Y: 0.4, L: 0.5, A: 0.5 * 0.5 * 595.28 / 841.89, Img: png, Prop: 0.5}
	out, err := Disegna(b, "", []Elemento{e})
	if err != nil {
		t.Fatal(err)
	}
	ctx, _, err := leggi(out, "")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`([0-9.]+) 0 0 ([0-9.]+) [0-9.-]+ [0-9.-]+ cm /TPim`)
	for p := 1; p <= 2; p++ {
		d, _, _, _ := ctx.PageDict(bg, p, false)
		c, err := ctx.PageContent(d, p)
		if err != nil {
			t.Fatal(err)
		}
		m := re.FindStringSubmatch(string(c))
		if m == nil {
			t.Fatalf("pagina %d: immagine non trovata", p)
		}
		w, _ := strconv.ParseFloat(m[1], 64)
		h, _ := strconv.ParseFloat(m[2], 64)
		if math.Abs(h/w-0.5) > 0.01 {
			t.Errorf("pagina %d: proporzione %.3f invece di 0.5", p, h/w)
		}
	}
}

// ZP05-5: un PDF 1.3 con un allegato (voce della 1.4) si unisce e si inserisce.
func TestZP05UnisciVersioneVecchia(t *testing.T) {
	v13, _ := os.ReadFile("testdata/v13-allegato.pdf")
	altro := pdfProva([]pagProva{a4("uno")})
	for _, parti := range [][][]byte{{altro, v13}, {v13, altro}} {
		out, err := unisciParti(parti)
		if err != nil {
			t.Fatal(err)
		}
		if c := leggiCatalogo(t, out); c.pagine != 1+leggiCatalogo(t, v13).pagine {
			t.Errorf("pagine %d", c.pagine)
		}
	}
}
