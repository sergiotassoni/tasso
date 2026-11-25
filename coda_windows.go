// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

//go:build windows

package main

// Dal menu del tasto destro, se scegli 5 file, Windows avvia Tasso Zip 5 volte (una per file).
// Il primo che parte diventa "il primo" (un mutex di Windows) e apre la finestra; gli altri
// scrivono il loro percorso in un piccolo file di coda e si chiudono subito, senza finestra.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

func fileCoda(modo string) string { return filepath.Join(cartella, "coda-"+modo+".txt") }

// sonoIlPrimo: true se nessun altro Tasso Zip sta raccogliendo i file per questo modo.
func sonoIlPrimo(modo string) (bool, func()) {
	nome, _ := windows.UTF16PtrFromString(`Local\TassoZip-` + modo)
	h, err := windows.CreateMutex(nil, true, nome)
	if err != nil {
		if h != 0 {
			windows.CloseHandle(h)
		}
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return false, func() {}
		}
		return true, func() {} // mutex non disponibile: apro comunque la finestra
	}
	return true, func() {
		_ = windows.ReleaseMutex(h)
		windows.CloseHandle(h)
		_ = os.Remove(fileCoda(modo))
	}
}

// passaAlPrimo aggiunge i percorsi alla coda (riprova se il file è occupato in quel momento).
func passaAlPrimo(modo string, percorsi []string) {
	riga := strings.Join(percorsi, "\n") + "\n"
	for i := 0; i < 40; i++ {
		f, err := os.OpenFile(fileCoda(modo), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
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

// leggiCoda prende i percorsi in coda e svuota il file.
func leggiCoda(modo string) []string {
	// Prima la sposto (se un altro Tasso Zip la sta scrivendo proprio adesso Windows dice di
	// no e riprovo al giro dopo): così nessun percorso scritto fra lettura e svuotamento va perso.
	p := fileCoda(modo)
	presa := p + ".lettura"
	if err := os.Rename(p, presa); err != nil {
		return nil
	}
	b, err := os.ReadFile(presa)
	_ = os.Remove(presa)
	if err != nil || len(b) == 0 {
		return nil
	}
	var out []string
	codaVistiMu.Lock()
	defer codaVistiMu.Unlock()
	for _, r := range strings.Split(string(b), "\n") {
		r = strings.TrimSpace(r)
		if r == "" || codaVisti[strings.ToLower(r)] {
			continue
		}
		codaVisti[strings.ToLower(r)] = true
		out = append(out, r)
	}
	return out
}

// raccogliCoda: aspetta un attimo gli altri Tasso Zip partiti insieme e unisce i loro file.
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

// ascoltaCoda: per qualche secondo dopo l'apertura arrivano ancora file in ritardo: li passo alla pagina.
func ascoltaCoda(modo string) {
	for i := 0; i < 40; i++ {
		time.Sleep(250 * time.Millisecond)
		if nuovi := leggiCoda(modo); len(nuovi) > 0 {
			manda("aggiunti", nuovi)
		}
	}
}
