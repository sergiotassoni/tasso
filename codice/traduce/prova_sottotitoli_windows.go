//go:build windows && collaudo

package traduce

// Prova dei sottotitoli con la lettura vera (OCR di Windows + traduttore vero):
// Tasso-collaudo.exe --prova-sottotitoli "C:\...\prove\sottotitoli"
// Mostra le immagini della cartella una dopo l'altra (una ogni mezzo secondo, come un video con
// lo sfondo che si muove), legge la fascia dei sottotitoli come fa il ciclo automatico e scrive in
// sottotitoli.txt (nella stessa cartella) cosa ha tradotto e quando. Atteso: una traduzione per
// ogni sottotitolo diverso, nessuna per i sottotitoli a metà e per le letture ripetute.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var cartellaSottotitoli string

func init() {
	for i, a := range os.Args {
		if a == "--prova-sottotitoli" && i+1 < len(os.Args) {
			cartellaSottotitoli = os.Args[i+1]
			fotoVisibili = true
		}
	}
}

func provaSottotitoliSeServe() bool {
	if cartellaSottotitoli == "" || !collaudoAvviato.CompareAndSwap(false, true) {
		return cartellaSottotitoli != ""
	}
	go eseguiProvaSottotitoli()
	return true
}

func eseguiProvaSottotitoli() {
	defer guard("prova-sottotitoli")
	defer func() { wv.Dispatch(func() { wv.Terminate() }) }()
	var out []string
	scrivi := func(f string, a ...any) {
		out = append(out, time.Now().Format("15:04:05.000")+"  "+fmt.Sprintf(f, a...))
	}
	defer func() {
		_ = os.WriteFile(filepath.Join(cartellaSottotitoli, "sottotitoli.txt"), []byte(strings.Join(out, "\r\n")+"\r\n"), 0o644)
	}()
	scene, _ := filepath.Glob(filepath.Join(cartellaSottotitoli, "*.png"))
	sort.Strings(scene)
	if len(scene) == 0 {
		scrivi("nessuna immagine")
		return
	}
	prima, err := caricaPNG(scene[0])
	if err != nil {
		scrivi("immagine non leggibile: %v", err)
		return
	}
	fin := mostraImmagine(prima)
	avanti(fin)
	r := rettangolo(fin)
	zona := Zona{X: r.X, Y: r.Y + r.H - 200, W: r.W, H: 190}
	muImp.Lock()
	vecchie := imp
	imp.Zona, imp.Posizione = zona, "sottotitoli"
	muImp.Unlock()
	defer func() {
		muImp.Lock()
		imp = vecchie
		muImp.Unlock()
		ov.Nascondi()
		pShowWindow.Call(fin, 0)
	}()
	acceso.Store(true)
	ultimoTesto, candidatoSub = "", ""
	scrivi("finestra %v, zona sottotitoli %v, %d immagini", r, zona, len(scene))
	time.Sleep(time.Second)
	tradotte := 0
	var cambio time.Time
	for _, s := range scene {
		img, err := caricaPNG(s)
		if err != nil {
			scrivi("%s: non leggibile", filepath.Base(s))
			continue
		}
		cambiaImmagine(fin, img)
		cambio = time.Now()
		time.Sleep(450 * time.Millisecond) // come il ciclo automatico
		muCronologia.Lock()
		n0 := len(righe)
		muCronologia.Unlock()
		c, err := cattura(zona)
		if err != nil {
			scrivi("%s: cattura non riuscita: %v", filepath.Base(s), err)
			continue
		}
		leggiETraduci(c, calcolaImpronta(c), false)
		muCronologia.Lock()
		nuove := append([]Risultato(nil), righe[n0:]...)
		muCronologia.Unlock()
		if len(nuove) == 0 {
			scrivi("%s: niente di nuovo (ultimo tradotto: %q)", filepath.Base(s), ultimoTesto)
		}
		for _, x := range nuove {
			tradotte++
			scrivi("%s: TRADOTTO dopo %d ms dal cambio: %q -> %q (chi parla: %q)", filepath.Base(s), time.Since(cambio).Milliseconds(), x.Originale, x.Tradotto, x.Parlante)
		}
	}
	scrivi("traduzioni fatte: %d (attese: una per ogni sottotitolo diverso)", tradotte)
}
