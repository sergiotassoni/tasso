package archivio

// CTRL-ZP-01, ZP01-2: i livelli sono veri. Sulla stessa cartella (testi veri: il codice di
// Tasso Zip) "Ultra" non esce mai più grande di "Massima", "Massima" di "Normale" e
// "Normale" di "Veloce"; e "Veloce" è davvero più grande di "Ultra".

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestZP01LivelliVeri(t *testing.T) {
	if testing.Short() {
		t.Skip("lento")
	}
	src := filepath.Join(t.TempDir(), "codice")
	os.MkdirAll(src, 0o755)
	var tot int64
	for _, g := range []string{"*.go", "../*.go", "../ui/*.html", "../testi/*.json", "../esplora/*.go", "settezip/*.go"} {
		ff, _ := filepath.Glob(g)
		for _, f := range ff {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			tot += int64(len(b))
			os.WriteFile(filepath.Join(src, filepath.Base(filepath.Dir(f))+"_"+filepath.Base(f)), b, 0o644)
		}
	}
	if tot < 500<<10 {
		t.Fatalf("pochi dati per la prova: %d", tot)
	}
	livelli := []Livello{LivelloVeloce, LivelloNormale, LivelloMassima, LivelloUltra}
	for _, f := range []Formato{Formato7z, FormatoTarXz, FormatoTarGz, FormatoTarBz2, FormatoTarZst} {
		dim := map[Livello]int64{}
		for _, l := range livelli {
			dove := filepath.Join(t.TempDir(), string(l))
			os.MkdirAll(dove, 0o755)
			inizio := time.Now()
			r, err := Comprimi(context.Background(), []string{src}, OpzioniComprimi{Formato: f, Livello: l, Cartella: dove})
			if err != nil {
				t.Fatal(f, l, err)
			}
			dim[l] = r.Compresso
			t.Logf("%-7s %-8s %9d byte  %v", f, l, r.Compresso, time.Since(inizio).Round(time.Millisecond))
		}
		for i := 1; i < len(livelli); i++ {
			if dim[livelli[i]] > dim[livelli[i-1]] {
				t.Errorf("%s: %s (%d) più grande di %s (%d)", f, livelli[i], dim[livelli[i]], livelli[i-1], dim[livelli[i-1]])
			}
		}
		if dim[LivelloUltra] >= dim[LivelloVeloce] {
			t.Errorf("%s: Ultra (%d) non più piccolo di Veloce (%d)", f, dim[LivelloUltra], dim[LivelloVeloce])
		}
	}
}
