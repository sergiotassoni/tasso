// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

// ruota_test.go: FIX-PDF-RUOTA. I passi visti sul PC: apri, ruota, chiudi senza salvare: il
// file aperto resta identico. Apri, salva una copia, ruota: la copia resta identica. Ruotare
// crea solo una versione in memoria; sul disco scrive soltanto «Salva una copia», e mai sopra
// un file aperto.

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func impronta(t *testing.T, p string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

func ruotaTutte(t *testing.T, id string, gradi int) VoceInfo {
	t.Helper()
	v, err := prendiVoce(id)
	if err != nil {
		t.Fatal(err)
	}
	b, err := v.contenuto()
	if err != nil {
		t.Fatal(err)
	}
	n, err := contaPagine(b)
	if err != nil {
		t.Fatal(err)
	}
	var seq []PaginaSeq
	for i := 1; i <= n; i++ {
		seq = append(seq, PaginaSeq{Fonte: 0, Pagina: i, Rot: gradi})
	}
	r, err := goOrganizza(id, "", nil, seq)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRuotaNonToccaIFileDellUtente(t *testing.T) {
	dir := t.TempDir()
	b, err := os.ReadFile(filepath.Join("testdata", "segnalibri.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	orig := filepath.Join(dir, "prova-unito - copia.pdf")
	if err := os.WriteFile(orig, b, 0o644); err != nil {
		t.Fatal(err)
	}
	h0 := impronta(t, orig)

	salvati := filepath.Join(dir, "salvati")
	if err := os.Mkdir(salvati, 0o755); err != nil {
		t.Fatal(err)
	}
	vecchio := piattaforma.salvaFile
	defer func() { piattaforma.salvaFile = vecchio }()
	dove := ""
	piattaforma.salvaFile = func(nome, cartella, titolo, ext string) (string, error) { return dove, nil }

	// 1. apri, ruota (anche due volte), chiudi senza salvare
	vi, err := apriPercorso(orig)
	if err != nil {
		t.Fatal(err)
	}
	r1 := ruotaTutte(t, vi.ID, 90)
	r2 := ruotaTutte(t, r1.ID, 90)
	goDimentica(r2.ID)
	goDimentica(r1.ID)
	goDimentica(vi.ID)
	if impronta(t, orig) != h0 {
		t.Fatal("ruotando è cambiato il file aperto")
	}

	// 2. apri, salva una copia, ruota, chiudi senza salvare: la copia resta com'era
	vi, err = apriPercorso(orig)
	if err != nil {
		t.Fatal(err)
	}
	copia := filepath.Join(salvati, "prova-unito - copia - copia.pdf")
	dove = copia
	if p, err := goSalva(vi.ID, "x.pdf", "Salva"); err != nil || p != copia {
		t.Fatalf("salva una copia: %q %v", p, err)
	}
	hc := impronta(t, copia)
	if hc != h0 {
		t.Fatal("la copia non è uguale all'originale")
	}
	r := ruotaTutte(t, vi.ID, 90)
	if impronta(t, copia) != hc || impronta(t, orig) != h0 {
		t.Fatal("ruotando dopo «Salva una copia» è cambiata la copia o l'originale")
	}

	// 3. la versione ruotata non si può salvare sopra il file aperto
	dove = orig
	if _, err := goSalva(r.ID, "x.pdf", "Salva"); err == nil || !strings.HasPrefix(err.Error(), "originale") {
		t.Fatalf("salvare sopra il file aperto: atteso «originale», avuto %v", err)
	}
	if impronta(t, orig) != h0 {
		t.Fatal("il file aperto è stato sovrascritto")
	}

	// 4. «Gira le pagine e salva»: solo la copia nuova ha le pagine girate
	girata := filepath.Join(salvati, "prova-unito - copia - modificato.pdf")
	dove = girata
	if _, err := goSalva(r.ID, "x.pdf", "Salva"); err != nil {
		t.Fatal(err)
	}
	gb, err := os.ReadFile(girata)
	if err != nil {
		t.Fatal(err)
	}
	ctx, _, err := leggi(gb, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.EnsurePageCount(); err != nil {
		t.Fatal(err)
	}
	for p := 1; p <= ctx.PageCount; p++ {
		_, _, inh, err := ctx.PageDict(bg, p, false)
		if err != nil {
			t.Fatal(err)
		}
		if inh == nil || inh.Rotate != 90 {
			t.Fatalf("pagina %d della copia girata: rotazione sbagliata", p)
		}
	}
	if impronta(t, copia) != hc || impronta(t, orig) != h0 {
		t.Fatal("salvando la copia girata è cambiato un altro file")
	}
	goDimentica(r.ID)
	goDimentica(vi.ID)
}
