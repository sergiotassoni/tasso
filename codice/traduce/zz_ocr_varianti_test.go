//go:build collaudo && windows

package traduce

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOCRVarianti: SCENA=foto.png ZONA=x0,y0,x1,y1 — legge la zona in vari modi e scrive il testo.
func TestOCRVarianti(t *testing.T) {
	f := os.Getenv("SCENA")
	if f == "" {
		t.Skip()
	}
	img, err := caricaPNG(f)
	if err != nil {
		t.Fatal(err)
	}
	var z image.Rectangle
	fmt.Sscanf(os.Getenv("ZONA"), "%d,%d,%d,%d", &z.Min.X, &z.Min.Y, &z.Max.X, &z.Max.Y)
	rit := image.NewRGBA(image.Rect(0, 0, z.Dx(), z.Dy()))
	draw.Draw(rit, rit.Bounds(), img, z.Min, draw.Src)
	dirApp = t.TempDir()
	l, err := avviaOCR(filepath.Join(dirApp, "lavoro"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Chiudi()
	var out strings.Builder
	for _, k := range []int{1, 2, 3, 4} {
		for _, modo := range []string{"grigio", "inverso", "soglia"} {
			g := ingrandisci(grigio(rit), k)
			contrasto(g)
			switch modo {
			case "inverso":
				for i := range g.Pix {
					g.Pix[i] = 255 - g.Pix[i]
				}
			case "soglia":
				var s int
				for _, v := range g.Pix {
					s += int(v)
				}
				m := s / len(g.Pix)
				for i, v := range g.Pix {
					if int(v) > m+40 {
						g.Pix[i] = 0
					} else {
						g.Pix[i] = 255
					}
				}
			}
			var b bytes.Buffer
			scriviBMP(&b, g)
			righe, err := l.LeggiPosizioni(b.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&out, "===== scala %d %s\n", k, modo)
			for _, r := range righe {
				var w []string
				for _, p := range r.Parole {
					w = append(w, p.T)
				}
				fmt.Fprintln(&out, strings.Join(w, " "))
			}
		}
	}
	os.WriteFile(os.Getenv("USCITA"), []byte(out.String()), 0o644)
}
