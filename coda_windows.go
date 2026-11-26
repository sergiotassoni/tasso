// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

//go:build windows

package main

// Dal menu del tasto destro, se scegli 5 file, Windows avvia Tasso Zip 5 volte (una per file).
// Il primo che parte diventa "il primo" (un mutex di Windows) e apre la finestra; gli altri
// scrivono il loro percorso in un piccolo file di coda e si chiudono subito, senza finestra.
// Il primo legge la coda per TUTTO il tempo in cui la finestra è aperta (prima solo 10
// secondi: un secondo "Estrai qui" fatto dopo si perdeva senza dire niente). Alla chiusura,
// se nella coda è rimasto qualcosa, apre un Tasso Zip nuovo per quei file.

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
		// arrivati proprio mentre chiudevo: non li perdo, li passo a un Tasso Zip nuovo
		resto := leggiCodaTutti(modo)
		time.Sleep(300 * time.Millisecond) // chi ha visto il mutex un attimo prima sta scrivendo
		resto = append(resto, leggiCoda(modo)...)
		if len(resto) > 0 {
			riapriPer(modo, resto)
		}
		_ = os.Remove(fileCoda(modo))
	}
}

// riapriPer avvia un Tasso Zip nuovo con questi file (lo stesso comando del tasto destro).
func riapriPer(modo string, percorsi []string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	// tanti file: più avvii, ognuno sotto il limite della riga di comando (si riuniscono da soli)
	for i, args := range comandiRiapertura(modo, percorsi, maxParametri) {
		if i > 0 {
			time.Sleep(150 * time.Millisecond) // il primo prende il "posto del primo"
		}
		shellExecute(exe, args)
	}
}

// visto da poco: lo stesso percorso arrivato due volte nello stesso avvio di Windows (un
// processo per file) conta una volta sola; lo stesso archivio scelto di nuovo più tardi
// (per esempio estratto una seconda volta) invece si rifà.
const finestraDoppi = 3 * time.Second

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
	ora := time.Now()
	for _, r := range strings.Split(string(b), "\n") {
		r = strings.TrimSpace(r)
		k := strings.ToLower(r)
		if r == "" {
			continue
		}
		if t, ok := codaVisti[k]; ok && ora.Sub(t) < finestraDoppi {
			continue
		}
		codaVisti[k] = ora
		out = append(out, r)
	}
	return out
}

// leggiCodaTutti: quello che è rimasto in coda, senza togliere i doppi (alla chiusura).
func leggiCodaTutti(modo string) []string {
	codaVistiMu.Lock()
	codaVisti = map[string]time.Time{}
	codaVistiMu.Unlock()
	return leggiCoda(modo)
}

// raccogliCoda: aspetta un attimo gli altri Tasso Zip partiti insieme e unisce i loro file.
func raccogliCoda(modo string, miei []string, attesa time.Duration) []string {
	codaVistiMu.Lock()
	for _, p := range miei {
		codaVisti[strings.ToLower(p)] = time.Now()
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

// ascoltaCoda: finché la finestra è aperta passo alla pagina i file arrivati dopo (in ritardo
// dallo stesso tasto destro, o da un tasto destro nuovo): la pagina li fa subito o, se sta
// lavorando, appena finisce.
func ascoltaCoda(modo string) {
	for i := 0; !chiusa.Load(); i++ {
		pausa := 250 * time.Millisecond
		if i > 40 {
			pausa = 400 * time.Millisecond
		}
		time.Sleep(pausa)
		if chiusa.Load() {
			return // alla chiusura ci pensa sonoIlPrimo (riapre per quelli rimasti)
		}
		if nuovi := leggiCoda(modo); len(nuovi) > 0 {
			manda("aggiunti", nuovi)
		}
	}
}
