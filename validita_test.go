// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// validoRigoroso: il PDF prodotto passa il controllo RIGOROSO di pdfcpu (non quello
// tollerante con cui leggiamo i file degli altri) e, se c'è sul PC, anche "qpdf --check".
func validoRigoroso(t *testing.T, nome string, b []byte, pw string) {
	t.Helper()
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationStrict
	conf.Offline = true
	if pw != "" {
		conf.UserPW, conf.OwnerPW = pw, pw
	}
	if err := api.Validate(context.Background(), bytes.NewReader(b), conf, nil); err != nil {
		t.Errorf("%s: non valido (pdfcpu, rigoroso): %v", nome, err)
	}
	q, err := exec.LookPath("qpdf")
	if err != nil {
		return
	}
	p := filepath.Join(t.TempDir(), "x.pdf")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"--check", p}
	if pw != "" {
		args = append([]string{"--password=" + pw}, args...)
	}
	// qpdf: 0 = tutto bene, 3 = solo avvisi, 2 = errori
	if out, err := exec.Command(q, args...).CombinedOutput(); err != nil {
		t.Errorf("%s: qpdf --check: %v\n%s", nome, err, out)
	}
}

// Ogni operazione che scrive un PDF deve dare un file valido.
func TestOgniOperazioneDaPDFValido(t *testing.T) {
	orig := pdfProva([]pagProva{a4("Uno"), {595.28, 841.89, 90, "Girata"}, a4o("Tre")})
	png := pngProva(t, 60, 40, true)
	jpg := jpegProva(t, 80, 60, 6)
	casi := []struct {
		nome string
		fai  func() ([]byte, error)
		pw   string
	}{
		{"ripara", func() ([]byte, error) { return Ripara(orig[:len(orig)*85/100], "") }, ""},
		{"ricostruisci", func() ([]byte, error) { return ricostruisci(orig, true) }, ""},
		{"ruota", func() ([]byte, error) { return ruota(orig, 90) }, ""},
		{"unisci", func() ([]byte, error) {
			return Unisci([]Pezzo{{Nome: "a.pdf", Dati: orig}, {Nome: "f.jpg", Dati: jpg, Foto: true}, {Nome: "b.pdf", Dati: orig, Rotazione: 180}})
		}, ""},
		{"foto", func() ([]byte, error) { return FotoInPDF(jpg) }, ""},
		{"firma", func() ([]byte, error) {
			return Timbra(orig, "", []Timbro{{Pagina: 0, PNG: png, X: .6, Y: .8, L: .3, A: .1}})
		}, ""},
		{"disegna", func() ([]byte, error) {
			return Disegna(orig, "", []Elemento{
				{Tipo: "testo", Pagina: 1, X: .1, Y: .1, L: .5, A: .05, Testo: "Caffè\nriga due", Font: "timesb", Dim: 14, Colore: "#c03020"},
				{Tipo: "immagine", Pagina: 2, X: .5, Y: .5, L: .2, A: .1, Img: png},
				{Tipo: "testo", X: 0, Y: .4, L: 1, A: .2, Testo: "BOZZA", Font: "helvb", Dim: 80, Opacita: .3, Rot: 45, Allinea: "c"},
				{Tipo: "rett", Pagina: 3, X: .1, Y: .1, L: .1, A: .1, Colore: "#ffff00", Sotto: true},
			})
		}, ""},
		{"collegamenti", func() ([]byte, error) {
			return AggiungiCollegamenti(orig, "", []Collegamento{{Pagina: 1, X: .1, Y: .1, L: .3, A: .05, URL: "www.esempio.it"}, {Pagina: 2, L: .2, A: .2, Verso: 3}})
		}, ""},
		{"organizza", func() ([]byte, error) {
			return Organizza([][]byte{orig, treePagine()}, []PaginaSeq{{Fonte: 0, Pagina: 3, Rot: 90}, {Fonte: -1, W: 595.28, H: 841.89}, {Fonte: 1, Pagina: 1}, {Fonte: 0, Pagina: 1}})
		}, ""},
		{"proteggi", func() ([]byte, error) {
			return Proteggi(orig, "", OpzProteggi{Apertura: "Apri-789", Blocca: true, Permessi: "Perm-123", Stampa: true})
		}, "Apri-789"},
		{"comprimi", func() ([]byte, error) { return Comprimi(pdfConFoto(t), "", true) }, ""},
		{"oscura", func() ([]byte, error) { return Oscura(orig, "", []PaginaOscurata{{Pagina: 2, Img: png}}) }, ""},
	}
	for _, c := range casi {
		b, err := c.fai()
		if err != nil {
			t.Errorf("%s: %v", c.nome, err)
			continue
		}
		validoRigoroso(t, c.nome, b, c.pw)
		if c.pw == "" {
			// e si rilegge, aperto e controllato come fa il programma
			if _, err := api.ReadContext(context.Background(), bytes.NewReader(b), confBase("")); err != nil {
				t.Errorf("%s: non si rilegge: %v", c.nome, err)
			}
		}
	}
	// Ripara dei file scaricati a metà (anche quando serve il nuovo elenco delle pagine).
	for _, perc := range []int{55, 60, 70, 80, 90, 97} {
		b, err := Ripara(orig[:len(orig)*perc/100], "")
		if err != nil {
			t.Errorf("ripara %d%%: %v", perc, err)
			continue
		}
		validoRigoroso(t, fmt.Sprintf("ripara %d%%", perc), b, "")
	}
	// Sblocca del file protetto: torna un PDF valido senza password.
	p, err := Proteggi(orig, "", OpzProteggi{Apertura: "Apri-789"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := Sblocca(p, "Apri-789", true)
	if err != nil {
		t.Fatal(err)
	}
	validoRigoroso(t, "sblocca", s, "")
}
