// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

import (
	"os"
	"testing"
)

// PDF con la protezione vecchia (RC4 128) e password con l'accento: l'utente scrive "città"
// e il file si deve aprire, sbloccare e unire; una password sbagliata resta sbagliata.
func TestPasswordAccentataProtezioneVecchia(t *testing.T) {
	d, err := os.ReadFile("testdata/rc4-password-accentata.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := leggi(d, "città"); err != nil {
		t.Fatalf("leggi con città: %v", err)
	}
	out, err := Sblocca(d, "città", true)
	if err != nil {
		t.Fatalf("Sblocca: %v", err)
	}
	if i, err := Info(out, ""); err != nil || i.Pagine != 3 {
		t.Fatalf("copia sbloccata: pagine=%d err=%v", i.Pagine, err)
	}
	if _, err := Unisci([]Pezzo{{Nome: "a", Dati: d, Password: "città"}, {Nome: "b", Dati: out}}); err != nil {
		t.Fatalf("Unisci: %v", err)
	}
	for _, pw := range []string{"citta", "cittá", "città!", ""} {
		if _, err := Sblocca(d, pw, true); CodiceErrore(err) != "password" {
			t.Errorf("password %q: atteso errore password, avuto %v", pw, err)
		}
	}
}

func TestPdfDocEncoding(t *testing.T) {
	casi := []struct {
		in, out string
		ok      bool
	}{
		{"città", "citt\xe0", true},
		{"5€", "5\xa0", true},
		{"abc", "", false}, // solo ASCII: niente seconda prova
		{"日本", "", false},  // non c'è in PDFDocEncoding
	}
	for _, c := range casi {
		o, ok := pdfDocEncoding(c.in)
		if ok != c.ok || (ok && o != c.out) {
			t.Errorf("pdfDocEncoding(%q) = %q,%v", c.in, o, ok)
		}
	}
}
