//go:build !disinstallatore

package pacco

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"tassosetup/installa"
)

// TestPaccoVero: se compila.sh ha già preparato i file, li installa davvero in una
// cartella temporanea (registro simulato) e poi li toglie.
func TestPaccoVero(t *testing.T) {
	e, err := installa.LeggiElenco(Sorgente())
	if err != nil {
		t.Skip("file non ancora preparati (compila.sh):", err)
	}
	base := filepath.Join(t.TempDir(), "Tasso")
	reg := installa.NuovoRegistroMemoria()
	in := &installa.Installatore{Sorgente: Sorgente(), Elenco: e, Registro: reg}
	st, err := in.Installa(context.Background(), installa.Opzioni{Base: base, Componenti: installa.TuttiComponenti, Lingua: "it"})
	if err != nil {
		t.Fatal(err)
	}
	// anche gli avvisi delle licenze di terze parti, accanto a ogni programma
	for _, f := range []string{"Tasso.exe", "VERSIONE", "LICENSE", "LICENZE.txt", "LICENSES.txt", installa.NomeDisinstalla,
		"Tasso Zip/TassoZip.exe", "Tasso Zip/LICENZE.txt", "Tasso PDF/TassoPDF.exe", "Tasso PDF/LICENZE.txt"} {
		if _, err := os.Stat(filepath.Join(base, filepath.FromSlash(f))); err != nil {
			t.Errorf("manca %s", f)
		}
	}
	if len(st.Programmi) != 4 {
		t.Fatalf("programmi: %d", len(st.Programmi))
	}
	r, err := in.Disinstalla(base, installa.TuttiComponenti)
	if err != nil || !r.TuttoTolto {
		t.Fatal(r, err)
	}
	if _, err := os.Stat(base); !os.IsNotExist(err) {
		t.Fatal("cartella rimasta")
	}
}
