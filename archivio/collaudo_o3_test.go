// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

import (
	"archive/tar"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Un tar con un "file contiguo" (tipo '7', è un file normale) veniva saltato come
// "file speciale": il file mancava dall'estrazione.
func TestTarFileContiguo(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "vecchio.tar")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	dati := []byte("contenuto")
	if err := tw.WriteHeader(&tar.Header{Name: "nota.txt", Typeflag: tar.TypeCont, Size: int64(len(dati)), Mode: 0o644, Format: tar.FormatUSTAR}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(dati); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	re, err := Estrai(context.Background(), p, OpzioniEstrai{})
	if err != nil {
		t.Fatal(err)
	}
	if got := albero(t, re.Cartella); got["nota.txt"] != "contenuto" {
		t.Fatalf("estratti: %v", got)
	}
}
