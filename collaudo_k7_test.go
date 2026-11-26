// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

import (
	"bytes"
	"testing"
	"time"
)

// Un PDF rovinato con un numero di oggetto enorme ("4000000000 0 obj"): la riparazione scriveva
// un indice con una riga per ogni numero fino a quello (miliardi di righe: memoria esaurita e
// programma chiuso di colpo; con 977106 erano 20 MB e decine di secondi per un file di 3 KB).
func TestRicostruisciNumeroOggettoEnorme(t *testing.T) {
	orig := pdfProva([]pagProva{a4("uno"), a4("due")})
	i := bytes.LastIndex(orig, []byte("xref"))
	var b bytes.Buffer
	b.Write(orig[:i])
	b.WriteString("4000000000 0 obj\n<< /Producer (rovinato) >>\nendobj\n")
	b.WriteString("startxref\n999999\n%%EOF\n") // indice sbagliato: serve la riparazione
	rotto := b.Bytes()

	for _, nuovo := range []bool{false, true} {
		r, err := ricostruisci(rotto, nuovo)
		if err != nil {
			t.Fatalf("ricostruisci(%v): %v", nuovo, err)
		}
		if len(r) > len(rotto)+4096 {
			t.Fatalf("ricostruisci(%v): file di %d byte da uno di %d (indice gonfiato)", nuovo, len(r), len(rotto))
		}
	}
	t0 := time.Now()
	inf, err := Info(rotto, "")
	if err != nil || inf.Pagine != 2 {
		t.Fatalf("Info: %+v %v", inf, err)
	}
	out, err := Ripara(rotto, "")
	if err != nil {
		t.Fatalf("Ripara: %v", err)
	}
	if n := pagine(t, out); n != 2 {
		t.Fatalf("pagine dopo la riparazione: %d", n)
	}
	if d := time.Since(t0); d > 20*time.Second {
		t.Fatalf("troppo lento: %v", d)
	}
}

// "/MediaBox [ ]" (vuoto) mandava pdfcpu in panico: nel programma diventava "errore interno"
// (o, senza la protezione delle chiamate, la chiusura). Ora è un PDF rovinato come gli altri.
func TestMediaBoxVuotaNonVaInPanico(t *testing.T) {
	orig := pdfProva([]pagProva{a4("uno"), a4("due")})
	box := []byte("/MediaBox [0 0 595.28 841.89]")
	i := bytes.Index(orig, box)
	if i < 0 {
		t.Fatal("MediaBox non trovata")
	}
	rotto := append([]byte{}, orig...)
	copy(rotto[i:], []byte("/MediaBox [                  ]")) // stessa lunghezza: l'indice resta giusto
	for nome, f := range map[string]func() error{
		"Info":   func() error { _, e := Info(rotto, ""); return e },
		"Ripara": func() error { _, e := Ripara(rotto, ""); return e },
		"Firma": func() error {
			_, e := Timbra(rotto, "", []Timbro{{Pagina: 2, PNG: pngProva(t, 20, 10, false), X: .1, Y: .1, L: .2, A: .1}})
			return e
		},
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: panico %v", nome, r)
				}
			}()
			if err := f(); err != nil && CodiceErrore(err) != "rovinato" {
				t.Errorf("%s: errore %v (atteso nessuno o \"rovinato\")", nome, err)
			}
		}()
	}
}
