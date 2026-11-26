package installa

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHaFirma(t *testing.T) {
	d := t.TempDir()
	finto := filepath.Join(d, "finto.exe")
	os.WriteFile(finto, []byte("non sono un exe"), 0o644)
	if _, err := HaFirma(finto); err == nil {
		t.Fatal("un file qualsiasi risulta un exe")
	}
	// l'exe di Tasso nel pacco (quando c'è): le build di prova non sono firmate
	for _, p := range []string{"../pacco/file/tasso/Tasso.exe", "../build/Disinstalla Tasso.exe"} {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if _, err := HaFirma(p); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
}
