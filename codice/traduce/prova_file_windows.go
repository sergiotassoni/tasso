//go:build windows && collaudo

package traduce

// Prova dal vivo della traduzione dentro il gioco: installo, apro il gioco, aspetto, fotografo lo
// schermo ogni tanto, chiudo il gioco. Le mod restano installate (si tolgono con --togli-file).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	provaFileCartella string
	provaFileNome     string
	provaFileUscita   string
	provaFileSecondi  = 240
	togliFileCartella string
)

func init() {
	for i, a := range os.Args {
		if i+1 >= len(os.Args) {
			break
		}
		switch a {
		case "--prova-file":
			provaFileCartella = os.Args[i+1]
		case "--nome-gioco":
			provaFileNome = os.Args[i+1]
		case "--uscita":
			provaFileUscita = os.Args[i+1]
		case "--secondi":
			fmt.Sscan(os.Args[i+1], &provaFileSecondi)
		case "--togli-file":
			togliFileCartella = os.Args[i+1]
		}
	}
}

func provaFileSeServe() bool {
	if togliFileCartella != "" {
		go func() {
			defer func() { wv.Dispatch(func() { wv.Terminate() }) }()
			e := fermaModGioco(togliFileCartella)
			impostaGiocoTradotto(togliFileCartella, false)
			_ = os.WriteFile(filepath.Join(dirApp, "logs", "togli-file.txt"), []byte("esito: '"+e+"'\r\n"), 0o644)
		}()
		return true
	}
	if provaFileCartella == "" {
		return false
	}
	go eseguiProvaFile()
	return true
}

func eseguiProvaFile() {
	defer guard("prova-file")
	defer func() { wv.Dispatch(func() { wv.Terminate() }) }()
	_ = os.MkdirAll(provaFileUscita, 0o755)
	var note []string
	nota := func(s string) {
		note = append(note, time.Now().Format("15:04:05")+" "+s)
		_ = os.WriteFile(filepath.Join(provaFileUscita, "prova.txt"), []byte(strings.Join(note, "\r\n")+"\r\n"), 0o644)
	}
	if _, ok := leggiManifesto(dirApp, chiaveGioco(provaFileCartella)); !ok {
		t0 := time.Now()
		applicaModGioco(provaFileNome, provaFileCartella)
		_, ok := leggiManifesto(dirApp, chiaveGioco(provaFileCartella))
		nota(fmt.Sprintf("installazione: %v in %.1f s", ok, time.Since(t0).Seconds()))
		if !ok {
			return
		}
	}
	m, _ := leggiManifesto(dirApp, chiaveGioco(provaFileCartella))
	nota("aggiunti: " + strings.Join(m.Aggiunti, ", ") + " | sostituiti: " + strings.Join(m.Sostituiti, ", "))
	cmd := exec.Command(filepath.Join(m.Cartella, m.Exe))
	cmd.Dir = m.Cartella
	if err := cmd.Start(); err != nil {
		nota("avvio gioco: " + err.Error())
		return
	}
	nota("gioco avviato")
	inizio := time.Now()
	n := 0
	for time.Since(inizio) < time.Duration(provaFileSecondi)*time.Second {
		time.Sleep(15 * time.Second)
		n++
		w, h := dimensioniSchermo()
		img, err := cattura(Zona{X: 0, Y: 0, W: w, H: h})
		if err == nil {
			salvaPNG(img, filepath.Join(provaFileUscita, fmt.Sprintf("gioco-%03ds.png", int(time.Since(inizio).Seconds()))))
		}
		nota(fmt.Sprintf("%3.0f s: richieste di traduzione dal gioco %d, gioco aperto %v", time.Since(inizio).Seconds(), modRichieste.Load(), processoAperto(m.Exe)))
		if !processoAperto(m.Exe) && time.Since(inizio) > 60*time.Second {
			break
		}
	}
	_ = exec.Command("taskkill", "/IM", m.Exe, "/T", "/F").Run()
	nota("gioco chiuso")
	time.Sleep(3 * time.Second)
}

func dimensioniSchermo() (int, int) {
	w, _, _ := pGetSysMetrics.Call(0)
	h, _, _ := pGetSysMetrics.Call(1)
	return int(w), int(h)
}
