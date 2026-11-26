// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

// Avanzi di lavori interrotti di colpo (PC spento, programma chiuso a forza, memoria finita):
// le cartelle e i file di lavoro ".tassozip-….tmp" restavano accanto agli archivi, anche di
// molti GB. Prima di ogni lavoro in una cartella (e all'avvio, sul Desktop e accanto ai file
// aperti) tolgo quelli vecchi. Un lavoro in corso (anche di un'altra finestra di Tasso Zip) tiene
// aperto il file ".in-corso" dentro la sua cartella: su Windows non si può cancellare, e allora
// la cartella non si tocca.

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// vecchiezzaAvanzi: più vecchi di così sono avanzi (un lavoro vero aggiorna i suoi file).
var vecchiezzaAvanzi = 6 * time.Hour

const fileInCorso = ".in-corso"

func avanzo(nome string) bool {
	return strings.HasPrefix(nome, ".tassozip-") && strings.HasSuffix(nome, ".tmp")
}

// PulisciAvanzi toglie da dir le cartelle e i file di lavoro rimasti da lavori interrotti.
// Restituisce quanti ne ha tolti.
func PulisciAvanzi(dir string) int {
	if dir == "" {
		return 0
	}
	voci, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, v := range voci {
		if !avanzo(v.Name()) || v.Type()&os.ModeSymlink != 0 {
			continue
		}
		fi, err := v.Info()
		if err != nil || time.Since(fi.ModTime()) < vecchiezzaAvanzi {
			continue
		}
		p := filepath.Join(dir, v.Name())
		if v.IsDir() {
			// lavoro ancora in corso? (il file aperto non si cancella)
			if err := os.Remove(filepath.Join(p, fileInCorso)); err != nil && !os.IsNotExist(err) {
				continue
			}
			if os.RemoveAll(p) == nil {
				n++
			}
			continue
		}
		if os.Remove(p) == nil {
			n++
		}
	}
	return n
}

// bloccaLavoro: segna la cartella di lavoro come "in uso" (vedi sopra) e la nasconde.
// Restituisce la funzione che toglie il segno.
func bloccaLavoro(dir string) func() {
	nascondi(dir)
	f, err := os.OpenFile(filepath.Join(dir, fileInCorso), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return func() {}
	}
	fatto := false
	return func() {
		if fatto {
			return
		}
		fatto = true
		f.Close()
		_ = os.Remove(filepath.Join(dir, fileInCorso))
	}
}
