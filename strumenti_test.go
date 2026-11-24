// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLeggiArgomenti(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "Contratto.pdf")
	_ = os.WriteFile(f, pdfProva([]pagProva{a4("x")}), 0o644)
	if m, pp := leggiArgomenti([]string{"--unisci", f}); m != "unisci" || len(pp) != 1 {
		t.Fatalf("%s %v", m, pp)
	}
	if m, pp := leggiArgomenti([]string{f}); m != "apri" || len(pp) != 1 {
		t.Fatalf("senza modo: %s %v", m, pp)
	}
	if m, pp := leggiArgomenti([]string{"--firma"}); m != "" || len(pp) != 0 {
		t.Fatal("modo senza file")
	}
	if m, _ := leggiArgomenti([]string{"--formatta", f}); m != "apri" {
		t.Fatal("modo sconosciuto")
	}
	if _, pp := leggiArgomenti([]string{filepath.Join(dir, "non-esiste.pdf"), dir}); len(pp) != 0 {
		t.Fatal("file inesistenti o cartelle accettati")
	}
	avvio.modo, avvio.percorsi = "unisci", []string{f}
	fa := goFileAvvio()
	if fa.Modo != "unisci" || len(fa.File) != 1 || fa.File[0].Nome != "Contratto.pdf" {
		t.Fatalf("%+v", fa)
	}
	arrivano([]string{f})
	if fa := goFileAvvio(); len(fa.File) != 1 {
		t.Fatal("file arrivati dopo")
	}
	if fa := goFileAvvio(); len(fa.File) != 0 {
		t.Fatal("consegnati due volte")
	}
	avvio.modo = ""
}

func TestSerieEsportaETesto(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "Contratto.pdf")
	_ = os.WriteFile(orig, pdfProva([]pagProva{a4("x")}), 0o644)
	vi, err := apriPercorso(orig)
	if err != nil {
		t.Fatal(err)
	}
	vecchio := piattaforma.salvaFile
	defer func() { piattaforma.salvaFile = vecchio }()
	piattaforma.salvaFile = func(nome, cartella, titolo, ext string) (string, error) {
		return filepath.Join(dir, strings.TrimSuffix(nome, filepath.Ext(nome))), nil
	}
	tok, err := goSerieInizio(vi.ID, "Contratto.png", "", "png")
	if err != nil || tok == "" {
		t.Fatal(err)
	}
	png := base64.StdEncoding.EncodeToString(pngProva(t, 4, 4, false))
	p1, err := goSerieScrivi(tok, 1, png)
	if err != nil || filepath.Base(p1) != "Contratto - 1.png" {
		t.Fatalf("%s %v", p1, err)
	}
	p1b, _ := goSerieScrivi(tok, 1, png)
	if filepath.Base(p1b) != "Contratto - 1 (2).png" {
		t.Fatalf("non deve sovrascrivere: %s", p1b)
	}
	goSerieFine(tok)
	if _, err := goSerieScrivi(tok, 2, png); err == nil {
		t.Fatal("serie chiusa ancora attiva")
	}
	if _, err := goSerieInizio(vi.ID, "x.exe", "", "exe"); err == nil {
		t.Fatal("estensione non permessa")
	}
	// testo estratto
	tv := goCaricaTesto("Contratto.txt", "riga 1\nriga 2")
	p, err := goSalvaTipo(tv.ID, "Contratto.txt", "", "txt")
	if err != nil || filepath.Ext(p) != ".txt" {
		t.Fatalf("%s %v", p, err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "riga 1\r\nriga 2") {
		t.Fatalf("contenuto %q", b)
	}
	// mai sopra l'originale, neanche come .pdf scelto a mano
	piattaforma.salvaFile = func(nome, cartella, titolo, ext string) (string, error) { return orig, nil }
	if _, err := goSalvaTipo(vi.ID, "x.pdf", "", "pdf"); err == nil || !strings.HasPrefix(err.Error(), "originale|") {
		t.Fatalf("originale sovrascritto: %v", err)
	}
}

func TestNomeValidoEstensioni(t *testing.T) {
	if nomeValido("Foto.png") != "Foto.png" || nomeValido("testo.txt") != "testo.txt" {
		t.Fatal("estensioni permesse cambiate")
	}
	if n := nomeValido(strings.Repeat("a", 300) + ".txt"); !strings.HasSuffix(n, ".txt") || len([]rune(n)) > 180 {
		t.Fatal(n)
	}
}

func TestGoStrumentiSuVoce(t *testing.T) {
	id := goCaricaInizio("Prova.pdf")
	_ = goCaricaPezzo(id, base64.StdEncoding.EncodeToString(treePagine()))
	if _, err := goCaricaFine(id); err != nil {
		t.Fatal(err)
	}
	r, err := goOrganizza(id, "", nil, []PaginaSeq{{Fonte: 0, Pagina: 2}, {Fonte: -1, W: 300, H: 300}})
	if err != nil || r.Nome != "Prova.pdf" || r.ID == id {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := goDisegna(id, "", []ElementoPagina{{Elemento: Elemento{Tipo: "immagine", Pagina: 1, L: .1, A: .1}, Img64: base64.StdEncoding.EncodeToString(pngProva(t, 8, 8, false))}}); err != nil {
		t.Fatal(err)
	}
	if _, err := goComprimi(id, "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := goProteggi(id, "", OpzProteggi{Apertura: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := goOscura(id, "", []PaginaOscurataPagina{{Pagina: 1, Img64: base64.StdEncoding.EncodeToString(pngProva(t, 8, 8, false))}}); err != nil {
		t.Fatal(err)
	}
	if _, err := goCollegamenti(id, "", []Collegamento{{Pagina: 1, L: .1, A: .1, URL: "https://esempio.it"}}); err != nil {
		t.Fatal(err)
	}
	// l'originale in memoria è rimasto quello
	v, _ := prendiVoce(id)
	if pagine(t, v.Dati) != 3 {
		t.Fatal("originale cambiato")
	}
}
