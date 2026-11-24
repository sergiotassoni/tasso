//go:build windows

// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

// Dal menu del tasto destro, se scegli 5 file, Windows avvia Tasso PDF 5 volte (una per file).
// Il primo che parte (un mutex di Windows) apre la finestra; gli altri scrivono il loro
// percorso in un piccolo file di coda e si chiudono subito, senza finestra (come Tasso Zip).

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

var (
	codaVisti   = map[string]bool{}
	codaVistiMu sync.Mutex
)

func fileCoda(modo string) string { return filepath.Join(cartellaDatiWin(), "coda-"+modo+".txt") }

func sonoIlPrimo(modo string) (bool, func()) {
	nome, _ := windows.UTF16PtrFromString(`Local\TassoPDF-` + modo)
	h, err := windows.CreateMutex(nil, true, nome)
	if err != nil {
		if h != 0 {
			windows.CloseHandle(h)
		}
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return false, func() {}
		}
		return true, func() {}
	}
	return true, func() {
		_ = windows.ReleaseMutex(h)
		windows.CloseHandle(h)
		_ = os.Remove(fileCoda(modo))
	}
}

func passaAlPrimo(modo string, percorsi []string) {
	_ = os.MkdirAll(cartellaDatiWin(), 0o700)
	riga := strings.Join(percorsi, "\n") + "\n"
	for i := 0; i < 40; i++ {
		f, err := os.OpenFile(fileCoda(modo), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
		if err == nil {
			_, err = f.WriteString(riga)
			f.Close()
			if err == nil {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func leggiCoda(modo string) []string {
	p := fileCoda(modo)
	b, err := os.ReadFile(p)
	if err != nil || len(b) == 0 {
		return nil
	}
	if fi, err := os.Stat(p); err == nil && fi.Size() == int64(len(b)) {
		_ = os.Truncate(p, 0)
	}
	var out []string
	codaVistiMu.Lock()
	defer codaVistiMu.Unlock()
	for _, r := range strings.Split(string(b), "\n") {
		r = strings.TrimSpace(r)
		if r == "" || codaVisti[strings.ToLower(r)] {
			continue
		}
		if fi, err := os.Stat(r); err != nil || fi.IsDir() {
			continue
		}
		codaVisti[strings.ToLower(r)] = true
		out = append(out, r)
	}
	return out
}

func raccogliCoda(modo string, miei []string, attesa time.Duration) []string {
	codaVistiMu.Lock()
	for _, p := range miei {
		codaVisti[strings.ToLower(p)] = true
	}
	codaVistiMu.Unlock()
	tutti := append([]string{}, miei...)
	fine := time.Now().Add(attesa)
	for time.Now().Before(fine) {
		time.Sleep(75 * time.Millisecond)
		tutti = append(tutti, leggiCoda(modo)...)
	}
	return tutti
}

// ascoltaCoda: per qualche secondo arrivano ancora file in ritardo; la pagina li chiede con goFileAvvio.
func ascoltaCoda(modo string) {
	for i := 0; i < 40; i++ {
		time.Sleep(250 * time.Millisecond)
		if nuovi := leggiCoda(modo); len(nuovi) > 0 {
			arrivano(nuovi)
		}
	}
}
