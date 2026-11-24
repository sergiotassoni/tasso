// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestMisura comprime una cartella vera in tutti i formati e scrive dimensioni e tempi.
// Gli archivi restano in TASSOZIP_ESPORTA (per controllarli con altri programmi).
//
//	TASSOZIP_SORGENTE=/cartella TASSOZIP_ESPORTA=/uscita go test -run TestMisura -v ./archivio/
func TestMisura(t *testing.T) {
	src, dest := os.Getenv("TASSOZIP_SORGENTE"), os.Getenv("TASSOZIP_ESPORTA")
	if src == "" || dest == "" {
		t.Skip("serve TASSOZIP_SORGENTE e TASSOZIP_ESPORTA")
	}
	casi := []struct {
		nome string
		f    Formato
		l    Livello
	}{
		{"zip", FormatoZip, ""},
		{"7z-veloce", Formato7z, LivelloVeloce},
		{"7z-normale", Formato7z, LivelloNormale},
		{"7z-massima", Formato7z, LivelloMassima},
		{"7z-ultra", Formato7z, LivelloUltra},
		{"tarxz", FormatoTarXz, ""},
		{"tarzst", FormatoTarZst, ""},
	}
	for _, c := range casi {
		inizio := time.Now()
		r, err := Comprimi(context.Background(), []string{src}, OpzioniComprimi{Formato: c.f, Livello: c.l, Nome: c.nome, Cartella: dest})
		if err != nil {
			t.Fatalf("%s: %v", c.nome, err)
		}
		d := time.Since(inizio)
		t.Logf("%-11s %9.2f MB -> %8.2f MB (-%4.1f%%) in %6.1fs  [%s]", c.nome, float64(r.Originale)/1e6, float64(r.Compresso)/1e6,
			100*(1-float64(r.Compresso)/float64(r.Originale)), d.Seconds(), r.Archivio)
	}
}
