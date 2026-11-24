//go:build collaudo

package main

// Nome zz_: il suo init deve girare dopo quelli di steps_*.go (che aggiungono i passaggi).
// Solo per le prove (build con -tags collaudo, mai nella versione pubblicata):
// Tasso-collaudo.exe --passo=of_new_call esegue un passaggio e scrive i messaggi in
// %LOCALAPPDATA%\Tasso\logs\passo-<id>.txt, senza aprire la finestra.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	for _, a := range os.Args[1:] {
		if !strings.HasPrefix(a, "--passo=") {
			continue
		}
		id := strings.TrimPrefix(a, "--passo=")
		dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "Tasso", "logs")
		_ = os.MkdirAll(dir, 0o755)
		f, err := os.Create(filepath.Join(dir, "passo-"+id+".txt"))
		if err != nil {
			os.Exit(2)
		}
		found := false
		for _, s := range allSteps {
			if s.ID != id {
				continue
			}
			found = true
			t0 := time.Now()
			r := runPS(s.Script, s.Timeout, func(m string) { fmt.Fprintln(f, time.Now().Format("15:04:05")+"  "+m) })
			fmt.Fprintf(f, "-- fine in %.1f s, errore: %v\n", time.Since(t0).Seconds(), r.Err)
		}
		if !found {
			fmt.Fprintln(f, "passaggio non trovato: "+id)
		}
		f.Close()
		os.Exit(0)
	}
}
