// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// testoDiPagina: il testo scritto nel contenuto della pagina p (le stringhe (…) dei Tj).
func testoDiPagina(t *testing.T, ctx *model.Context, p int) string {
	t.Helper()
	return contenutoPagina(t, ctx, p)
}

// tuttiGliStream: il contenuto decodificato di tutti gli stream del file (per cercare se un
// testo è rimasto da qualche parte).
func tuttiGliStream(t *testing.T, data []byte) string {
	t.Helper()
	ctx := leggiCtxProva(t, data)
	var b strings.Builder
	for _, e := range ctx.XRefTable.Table {
		if e == nil || e.Free || e.Object == nil {
			continue
		}
		if sd, ok := e.Object.(types.StreamDict); ok {
			if err := sd.Decode(); err == nil {
				b.Write(sd.Content)
			}
			b.Write(sd.Raw)
		}
		b.WriteString(e.Object.String())
	}
	return b.String() + string(data)
}

func treePagine() []byte {
	return pdfProva([]pagProva{a4("Uno"), a4("Due"), a4o("Tre")})
}

func TestOrganizzaRiordinaRuotaElimina(t *testing.T) {
	orig := treePagine()
	// nuovo ordine: 3, 1 (girata a destra); la 2 è eliminata
	out, err := Organizza([][]byte{orig}, []PaginaSeq{{Fonte: 0, Pagina: 3}, {Fonte: 0, Pagina: 1, Rot: 90}})
	if err != nil {
		t.Fatal(err)
	}
	salvaProva(t, "organizzato.pdf", out)
	ctx := leggiCtxProva(t, out)
	if ctx.PageCount != 2 {
		t.Fatalf("pagine %d", ctx.PageCount)
	}
	if !strings.Contains(testoDiPagina(t, ctx, 1), "(Tre)") || !strings.Contains(testoDiPagina(t, ctx, 2), "(Uno)") {
		t.Fatal("ordine sbagliato")
	}
	if rotazionePagina(t, ctx, 2) != 90 || rotazionePagina(t, ctx, 1) != 0 {
		t.Fatal("rotazione sbagliata")
	}
	// l'originale non è cambiato
	if pagine(t, orig) != 3 {
		t.Fatal("originale toccato")
	}
}

func TestOrganizzaInserisciEVuota(t *testing.T) {
	orig := treePagine()
	altro := pdfProva([]pagProva{a4("Allegato A"), a4("Allegato B")})
	out, err := Organizza([][]byte{orig, altro}, []PaginaSeq{
		{Fonte: 0, Pagina: 1}, {Fonte: 1, Pagina: 2}, {Fonte: -1, W: 595.28, H: 841.89}, {Fonte: 0, Pagina: 2}, {Fonte: 0, Pagina: 3}, {Fonte: 0, Pagina: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := leggiCtxProva(t, out)
	if ctx.PageCount != 6 {
		t.Fatalf("pagine %d", ctx.PageCount)
	}
	if !strings.Contains(testoDiPagina(t, ctx, 2), "(Allegato B)") {
		t.Fatal("pagina inserita mancante")
	}
	if c, _ := ctx.PageContent(mustPageDict(t, ctx, 3), 3); bytes.Contains(c, []byte("Tj")) {
		t.Fatalf("la pagina vuota non è vuota: %q", c)
	}
	if !strings.Contains(testoDiPagina(t, ctx, 6), "(Uno)") {
		t.Fatal("pagina ripetuta mancante")
	}
}

func TestEstraiPagine(t *testing.T) {
	out, err := Organizza([][]byte{treePagine()}, []PaginaSeq{{Fonte: 0, Pagina: 2}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := leggiCtxProva(t, out)
	if ctx.PageCount != 1 || !strings.Contains(testoDiPagina(t, ctx, 1), "(Due)") {
		t.Fatal("estrazione")
	}
	if _, err := Organizza([][]byte{treePagine()}, []PaginaSeq{{Fonte: 0, Pagina: 9}}); CodiceErrore(err) != "intervallo" {
		t.Fatalf("pagina inesistente: %v", err)
	}
	if _, err := Organizza([][]byte{treePagine()}, nil); CodiceErrore(err) != "nessuna_pagina" {
		t.Fatalf("nessuna pagina: %v", err)
	}
}

func TestProteggiEApri(t *testing.T) {
	orig := treePagine()
	out, err := Proteggi(orig, "", OpzProteggi{Apertura: "Apri-789"})
	if err != nil {
		t.Fatal(err)
	}
	salvaProva(t, "protetto.pdf", out)
	inf, err := Info(out, "")
	if err != nil || !inf.ServePassword || !inf.Cifrato {
		t.Fatalf("senza password si apre: %+v %v", inf, err)
	}
	inf, err = Info(out, "Apri-789")
	if err != nil || !inf.PasswordGiusta || inf.Pagine != 3 {
		t.Fatalf("con la password non si apre: %+v %v", inf, err)
	}
	if !strings.Contains(string(out), "/AESV3") {
		t.Fatal("non è AES-256")
	}
	// solo permessi: si apre senza password ma è bloccato
	out2, err := Proteggi(orig, "", OpzProteggi{Blocca: true, Permessi: "Capo-1", Stampa: true})
	if err != nil {
		t.Fatal(err)
	}
	inf, err = Info(out2, "")
	if err != nil || inf.ServePassword || !inf.Bloccato {
		t.Fatalf("permessi: %+v %v", inf, err)
	}
	if _, err := Proteggi(orig, "", OpzProteggi{Blocca: true}); CodiceErrore(err) != "pw_permessi" {
		t.Fatalf("serve la password dei permessi: %v", err)
	}
	if _, err := Proteggi(orig, "", OpzProteggi{Apertura: "x", Blocca: true, Permessi: "x"}); CodiceErrore(err) != "pw_uguali" {
		t.Fatalf("password uguali: %v", err)
	}
	// e si può risbloccare con la password giusta
	if _, err := Sblocca(out, "Apri-789", true); err != nil {
		t.Fatal(err)
	}
}

// pdfConFoto: un PDF con una foto grande (sfumature, che il PNG comprime male).
func pdfConFoto(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2400, 1800))
	for y := 0; y < 1800; y++ {
		for x := 0; x < 2400; x++ {
			img.Set(x, y, color.NRGBA{uint8(x * 255 / 2400), uint8(y * 255 / 1800), uint8((x*7 + y*13 + (x*y)%17) % 256), 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	out, err := Timbra(pdfProva([]pagProva{a4("Foto")}), "", []Timbro{{Pagina: 1, PNG: buf.Bytes(), X: .1, Y: .1, L: .8, A: .45}})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestComprimi(t *testing.T) {
	orig := pdfConFoto(t)
	solo, err := Comprimi(orig, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(solo) > len(orig) {
		t.Fatalf("peggiorato: %d -> %d", len(orig), len(solo))
	}
	forte, err := Comprimi(orig, "", true)
	if err != nil {
		t.Fatal(err)
	}
	salvaProva(t, "compresso.pdf", forte)
	t.Logf("comprimi: %d -> %d (solo ottimizza %d)", len(orig), len(forte), len(solo))
	if len(forte) >= len(orig)/2 {
		t.Fatalf("le foto non sono state alleggerite: %d -> %d", len(orig), len(forte))
	}
	ctx := leggiCtxProva(t, forte)
	if ctx.PageCount != 1 || !strings.Contains(testoDiPagina(t, ctx, 1), "(Foto)") {
		t.Fatal("contenuto perso")
	}
	// un PDF senza foto non peggiora
	semplice := treePagine()
	c, err := Comprimi(semplice, "", true)
	if err != nil || len(c) > len(semplice) {
		t.Fatalf("PDF semplice peggiorato: %d -> %d %v", len(semplice), len(c), err)
	}
}

func TestDisegnaTestoImmagineFiligrana(t *testing.T) {
	orig := pdfProva([]pagProva{a4("Uno"), {595.28, 841.89, 90, "Girata"}, a4("Tre")})
	out, err := Disegna(orig, "", []Elemento{
		{Tipo: "testo", Pagina: 1, X: .1, Y: .1, L: .5, A: .05, Testo: "Caffè già pronto\nseconda riga", Font: "times", Dim: 14, Colore: "#c03020"},
		{Tipo: "immagine", Pagina: 2, X: .5, Y: .5, L: .2, A: .1, Img: pngProva(t, 40, 20, true)},
		{Tipo: "testo", Pagine: "", X: 0, Y: .4, L: 1, A: .2, Testo: "BOZZA", Font: "helvb", Dim: 80, Colore: "#888888", Opacita: .3, Rot: 45, Allinea: "c"},
		{Tipo: "testo", Pagine: "2-", X: .05, Y: .95, L: .9, A: .03, Testo: "Pagina {p} di {n}", Dim: 9, Allinea: "c"},
		{Tipo: "rett", Pagina: 3, X: .1, Y: .1, L: .1, A: .1, Colore: "#ffff00", Sotto: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	salvaProva(t, "disegnato.pdf", out)
	ctx := leggiCtxProva(t, out)
	c1, c2, c3 := testoDiPagina(t, ctx, 1), testoDiPagina(t, ctx, 2), testoDiPagina(t, ctx, 3)
	enc, _ := winAnsi("Caffè già pronto")
	if !strings.Contains(c1, strings.ToUpper(string(hexDi(enc)))) {
		t.Fatalf("testo aggiunto mancante:\n%s", c1)
	}
	if !strings.Contains(c1, "(Uno) Tj") {
		t.Fatal("contenuto originale perso")
	}
	if strings.Count(c2, " Do") != 1 {
		t.Fatal("immagine mancante a pagina 2")
	}
	for _, c := range []string{c1, c2, c3} {
		if !strings.Contains(c, hexDi([]byte("BOZZA"))) || !strings.Contains(c, " gs") {
			t.Fatal("filigrana mancante o senza trasparenza")
		}
	}
	if strings.Contains(c1, hexDi([]byte("Pagina 1 di 3"))) || !strings.Contains(c2, hexDi([]byte("Pagina 2 di 3"))) || !strings.Contains(c3, hexDi([]byte("Pagina 3 di 3"))) {
		t.Fatal("numeri di pagina sbagliati")
	}
	if !strings.Contains(c3, "re f") || strings.Index(c3, "re f") > strings.Index(c3, "(Tre) Tj") {
		t.Fatal("il rettangolo dietro non è prima del contenuto")
	}
	if _, err := Disegna(orig, "", []Elemento{{Tipo: "testo", Pagine: "7-9", L: 1, A: 1, Testo: "x"}}); CodiceErrore(err) != "intervallo" {
		t.Fatalf("intervallo: %v", err)
	}
}

func hexDi(b []byte) string {
	const h = "0123456789ABCDEF"
	var s strings.Builder
	for _, c := range b {
		s.WriteByte(h[c>>4])
		s.WriteByte(h[c&15])
	}
	return s.String()
}

func TestWinAnsi(t *testing.T) {
	if !TestoAdatto("Perché è già così? Ça, Straße, € – “ok”") {
		t.Fatal("testo latino rifiutato")
	}
	if TestoAdatto("Zażółć gęślą jaźń") {
		t.Fatal("il polacco con ł non è nei caratteri standard")
	}
}

func TestPagineScelte(t *testing.T) {
	m, err := pagineScelte("1-2, 4, 6-", 7)
	if err != nil || len(m) != 5 || !m[1] || !m[2] || !m[4] || !m[6] || !m[7] || m[3] {
		t.Fatalf("%v %v", m, err)
	}
	if _, err := pagineScelte("3-1", 5); err == nil {
		t.Fatal("intervallo al contrario accettato")
	}
	if m, _ := pagineScelte("", 3); len(m) != 3 {
		t.Fatal("vuoto = tutte")
	}
}

func TestCollegamenti(t *testing.T) {
	out, err := AggiungiCollegamenti(treePagine(), "", []Collegamento{
		{Pagina: 1, X: .1, Y: .1, L: .3, A: .05, URL: "www.esempio.it/pagina"},
		{Pagina: 2, X: .1, Y: .1, L: .3, A: .05, Verso: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	salvaProva(t, "collegamenti.pdf", out)
	s := tuttiGliStream(t, out)
	if strings.Count(s, "Subtype, Link") < 2 || !strings.Contains(s, "https://www.esempio.it/pagina") || !strings.Contains(s, "Fit]") {
		i := strings.Index(s, "/Link")
		t.Fatalf("collegamenti mancanti: %.400s", s[max(0, i-200):])
	}
	for _, u := range []string{"file:///C:/Windows/system32/calc.exe", "javascript:alert(1)", ""} {
		if _, err := AggiungiCollegamenti(treePagine(), "", []Collegamento{{Pagina: 1, L: .1, A: .1, URL: u, Verso: 0}}); err == nil {
			t.Fatalf("indirizzo accettato: %q", u)
		}
	}
}

func TestOscuraToglieDavveroIlTesto(t *testing.T) {
	orig := pdfProva([]pagProva{a4("Pubblico"), {595.28, 841.89, 90, "Segretissimo IBAN IT60X0542811101000000123456"}, a4("Altro")})
	// immagine della pagina 2 (fatta dalla pagina del programma, qui finta)
	img := pngProva(t, 120, 170, false)
	out, err := Oscura(orig, "", []PaginaOscurata{{Pagina: 2, Img: img}})
	if err != nil {
		t.Fatal(err)
	}
	salvaProva(t, "oscurato.pdf", out)
	ctx := leggiCtxProva(t, out)
	if ctx.PageCount != 3 {
		t.Fatal("pagine")
	}
	tutto := tuttiGliStream(t, out)
	if strings.Contains(tutto, "Segretissimo") || strings.Contains(tutto, "IT60X") {
		t.Fatal("il testo oscurato è ancora nel file")
	}
	if !strings.Contains(testoDiPagina(t, ctx, 1), "(Pubblico)") || !strings.Contains(testoDiPagina(t, ctx, 3), "(Altro)") {
		t.Fatal("le altre pagine sono cambiate")
	}
	c2 := testoDiPagina(t, ctx, 2)
	if strings.Contains(c2, "Tj") || !strings.Contains(c2, "/TPoscurata Do") {
		t.Fatalf("pagina 2 non è solo immagine: %q", c2)
	}
	// la pagina era girata di 90°: ora è dritta e larga quanto si vedeva
	if rotazionePagina(t, ctx, 2) != 0 {
		t.Fatal("rotazione")
	}
	_, _, attrs, _ := ctx.PageDict(bg, 2, false)
	if w := attrs.MediaBox.Width(); w < 841 || w > 842 {
		t.Fatalf("larghezza %v", w)
	}
}
