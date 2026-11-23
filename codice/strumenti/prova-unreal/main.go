// prova-unreal: SOLO LETTURA. Apre i .pak di un gioco Unreal, elenca i file di lingua, ne
// decomprime e legge uno, e prova a scrivere un pak di prova in una cartella temporanea (mai
// nella cartella del gioco). Serve a verificare il lettore su dati veri.
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tasso/traduce/unreal"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("uso: prova-unreal <cartella Paks> [uscita]")
		return
	}
	paks, _ := filepath.Glob(filepath.Join(os.Args[1], "*.pak"))
	for _, pp := range paks {
		t0 := time.Now()
		p, err := unreal.ApriPak(pp)
		if err != nil {
			fmt.Println("PAK", filepath.Base(pp), "ERRORE:", err)
			continue
		}
		fmt.Printf("PAK %s v%d montaggio=%q voci=%d compress=%v (%.1fs)\n", filepath.Base(pp), p.Versione, p.Montaggio, len(p.Voci), p.Compress, time.Since(t0).Seconds())
		var loc []unreal.Voce
		for _, v := range p.Voci {
			if strings.HasSuffix(strings.ToLower(v.Nome), ".locres") {
				loc = append(loc, v)
			}
		}
		fmt.Println("  file di lingua:", len(loc))
		for i, v := range loc {
			if i < 40 {
				fmt.Printf("  %s size=%d uncomp=%d compr=%d cifrato=%v\n", v.Nome, v.Size, v.Uncomp, v.Compr, v.Cifrato)
			}
		}
		for _, v := range loc {
			n := strings.ToLower(v.Nome)
			if !strings.Contains(n, "/en/") {
				continue
			}
			t1 := time.Now()
			b, err := p.Leggi(v)
			if err != nil {
				fmt.Println("  LEGGI", v.Nome, "ERRORE:", err)
				continue
			}
			l, err := unreal.LeggiLocres(b)
			if err != nil {
				fmt.Println("  LOCRES", v.Nome, "ERRORE:", err)
				continue
			}
			same := bytes.Equal(l.Scrivi(), b)
			fmt.Printf("  OK %s: %d byte, versione %d, %d frasi, riscrittura identica=%v (%.2fs)\n", v.Nome, len(b), l.Versione, len(l.Frasi), same, time.Since(t1).Seconds())
			for k := 0; k < len(l.Frasi) && k < 12; k++ {
				fmt.Printf("    %q\n", l.Frasi[k])
			}
			if len(os.Args) > 2 {
				out := filepath.Join(os.Args[2], "prova_P.pak")
				var w bytes.Buffer
				if err := unreal.ScriviPak(&w, []unreal.FileNuovo{{Nome: strings.TrimPrefix(v.Nome, "/"), Dati: l.Scrivi()}}); err == nil {
					_ = os.WriteFile(out, w.Bytes(), 0o644)
					q, err := unreal.ApriPak(out)
					if err == nil {
						_, ok := q.Trova(v.Nome)
						q.Chiudi()
						fmt.Println("  pak di prova riletto, file trovato:", ok)
					} else {
						fmt.Println("  pak di prova ERRORE:", err)
					}
				}
			}
		}
		p.Chiudi()
	}
}
