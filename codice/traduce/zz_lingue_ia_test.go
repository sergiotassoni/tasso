//go:build collaudo && windows

package traduce

// Correttore IA per le altre lingue, sulle uscite di Mozilla delle frasi FLORES.
//   CARTELLA=.../qualita-lingue  MODELLO=...gguf  MOTORE=cartella llama  META=sviluppo|verifica
//   go test -tags collaudo -run TestLingueIA -timeout 5h -v .

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestLingueIA(t *testing.T) {
	dir, modello, motore := os.Getenv("CARTELLA"), os.Getenv("MODELLO"), os.Getenv("MOTORE")
	if dir == "" || modello == "" {
		t.Skip()
	}
	dirApp = t.TempDir()
	_ = os.MkdirAll(filepath.Join(dirApp, "logs"), 0o755)
	pari := os.Getenv("META") == "verifica"
	s := &ServerIA{Contesto: 2048, Strati: 99}
	if err := s.Avvia(motore, modello); err != nil {
		t.Fatal(err)
	}
	defer s.Ferma()
	files, _ := filepath.Glob(filepath.Join(dir, "frasi-*.jsonl"))
	sort.Strings(files)
	if sel := os.Getenv("COPPIE"); sel != "" { // es. "fr-it,ja-it"
		var scelti []string
		for _, f := range files {
			for _, c := range strings.Split(sel, ",") {
				if strings.HasSuffix(f, "frasi-"+c+".jsonl") {
					scelti = append(scelti, f)
				}
			}
		}
		files = scelti
	}
	massimo := 1 << 30
	if v := os.Getenv("FRASI"); v != "" {
		fmt.Sscan(v, &massimo)
	}
	var righe []string
	for _, f := range files {
		fh, _ := os.Open(f)
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		var hypM, hypC, ref []string
		var da, a string
		meglio, peggio, cambiate := 0, 0, 0
		var tempo time.Duration
		for sc.Scan() {
			var r struct {
				ID                   int
				Da, A, Src, Moz, Ref string
			}
			if json.Unmarshal(sc.Bytes(), &r) != nil || (r.ID%2 == 0) != pari {
				continue
			}
			if len(ref) >= massimo {
				break
			}
			da, a = r.Da, r.A
			t0 := time.Now()
			out, err := s.chat(context.Background(), costruisciCorrezioneCoppia(r.Src, r.Moz, r.Da, r.A))
			tempo += time.Since(t0)
			fin := r.Moz
			if err == nil {
				out = pulisciRisposta(out)
				if correzioneAccettabileCoppia(r.Src, r.Moz, out) {
					fin = out
				}
			}
			if fin != r.Moz {
				cambiate++
				x, y := chrFpp([]string{r.Moz}, []string{r.Ref}), chrFpp([]string{fin}, []string{r.Ref})
				if y > x {
					meglio++
				} else if y < x {
					peggio++
				}
			}
			hypM, hypC, ref = append(hypM, r.Moz), append(hypC, fin), append(ref, r.Ref)
		}
		fh.Close()
		if len(ref) == 0 {
			continue
		}
		riga := fmt.Sprintf("%s→%s: Mozilla %.1f → con IA %.1f (cambiate %d: meglio %d, peggio %d) · %.0f ms a frase",
			da, a, chrFpp(hypM, ref), chrFpp(hypC, ref), cambiate, meglio, peggio, float64(tempo.Milliseconds())/float64(len(ref)))
		t.Log(riga)
		righe = append(righe, riga)
		_ = os.WriteFile(filepath.Join(dir, "ia-"+os.Getenv("META")+".txt"), []byte(strings.Join(righe, "\r\n")+"\r\n"), 0o644)
	}
	_ = os.WriteFile(filepath.Join(dir, "ia-"+os.Getenv("META")+".txt"), []byte(strings.Join(righe, "\r\n")+"\r\n"), 0o644)
}
