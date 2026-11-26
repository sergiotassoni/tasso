package main

import (
	"bytes"
	"testing"
)

// O3: un PDF senza indice (xref) compresso non deve tornare indietro così com'è (rovinato).
func TestComprimiPDFSenzaIndiceRestituisceCopiaRiparata(t *testing.T) {
	orig := pdfProva([]pagProva{a4("uno")})
	i := bytes.Index(orig, []byte("xref\n"))
	rotto := append(append([]byte{}, orig[:i]...), []byte("%%EOF\n")...)
	out, err := Comprimi(rotto, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(out, rotto) {
		t.Fatal("restituito il file rovinato così com'è")
	}
	if !finisceBene(out) {
		t.Fatal("la copia non ha l'indice in fondo")
	}
	if n := pagine(t, out); n != 1 {
		t.Fatalf("pagine: %d", n)
	}
	// un PDF sano che non si riesce a rimpicciolire resta quello di prima
	if !finisceBene(orig) {
		t.Fatal("finisceBene sbaglia su un PDF sano")
	}
}

// O3: "/MediaBox []" sulla pagina: Ripara (e Info) ora funzionano e la pagina diventa A4.
func TestMediaBoxVuotaSullaPaginaDiventaA4(t *testing.T) {
	orig := pdfProva([]pagProva{a4("uno"), a4("due")})
	box := []byte("/MediaBox [0 0 595.28 841.89]")
	j := bytes.Index(orig, box)
	rotto := append([]byte{}, orig...)
	copy(rotto[j:], []byte("/MediaBox [                  ]"))
	out, err := Ripara(rotto, "")
	if err != nil {
		t.Fatalf("Ripara: %v", err)
	}
	if n := pagine(t, out); n != 2 {
		t.Fatalf("pagine: %d", n)
	}
	if _, err := Info(rotto, ""); err != nil {
		t.Fatalf("Info: %v", err)
	}
}
