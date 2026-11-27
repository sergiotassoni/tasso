package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// In rilascio servono tutti e 7 gli sfondi, per nome.
func TestSfondiTasso(t *testing.T) {
	dir := t.TempDir()
	for _, id := range sfondiAttesi[:3] {
		if err := os.WriteFile(filepath.Join(dir, "tasso-"+id+".jpg"), []byte{0xFF, 0xD8, 0xFF}, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(dir, "tasso-altro.jpg"), []byte("x"), 0o644)
	out, err := sfondiTasso(dir)
	if err == nil || len(out) != 3 || !strings.Contains(err.Error(), "tasso-sabbia.jpg") {
		t.Fatalf("3 su 7: atteso errore con i mancanti e 3 file, avuto %v %v", out, err)
	}
	for _, id := range sfondiAttesi[3:] {
		_ = os.WriteFile(filepath.Join(dir, "tasso-"+id+".jpg"), []byte{0xFF, 0xD8, 0xFF}, 0o644)
	}
	out, err = sfondiTasso(dir)
	if err != nil || len(out) != 7 {
		t.Fatalf("7 su 7: atteso ok, avuto %d %v", len(out), err)
	}
	for _, f := range out {
		if !strings.HasPrefix(f[1], "sfondi/tasso-") || strings.Contains(f[1], "altro") {
			t.Errorf("nome dentro sbagliato: %q", f[1])
		}
	}
	if _, err := sfondiTasso(""); err == nil {
		t.Error("cartella vuota accettata")
	}
}
