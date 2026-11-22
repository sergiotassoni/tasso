//go:build collaudo && windows

package traduce

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOCRScala(t *testing.T) {
	f := os.Getenv("SCENA")
	if f == "" {
		t.Skip()
	}
	img, err := caricaPNG(f)
	if err != nil {
		t.Fatal(err)
	}
	dirApp = t.TempDir()
	l, err := avviaOCR(filepath.Join(dirApp, "lavoro"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Chiudi()
	for _, scala := range []int{1, 2} {
		g := grigio(img)
		if scala > 1 {
			g = ingrandisci(g, scala)
		}
		contrasto(g)
		var b bytes.Buffer
		png.Encode(&b, g)
		t0 := time.Now()
		righe, err := l.LeggiPosizioni(b.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		var testi []string
		for _, r := range righe {
			var w []string
			for _, p := range r.Parole {
				w = append(w, p.T)
			}
			testi = append(testi, strings.Join(w, " "))
		}
		t.Logf("scala %d: %d righe in %v\n%s", scala, len(righe), time.Since(t0), strings.Join(testi, "\n"))
	}
}
