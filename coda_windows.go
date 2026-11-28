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
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

func fileCoda(modo string) string { return filepath.Join(cartella, "coda-"+modo+".txt") }

// filePid: il numero del processo del primo (per lasciargli portare la finestra davanti).
func filePid(modo string) string { return filepath.Join(cartella, "coda-"+modo+".pid") }

var (
	procAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procShowWindow               = user32.NewProc("ShowWindow")
	procIsIconic                 = user32.NewProc("IsIconic")
	procFlashWindow              = user32.NewProc("FlashWindow")
)

// lasciaDavantiIlPrimo: Windows lascia portare una finestra davanti solo a chi l'utente ha appena
// usato (qui: questo processo, avviato dal tasto destro). Passo il permesso al primo.
func lasciaDavantiIlPrimo(modo string) {
	b, err := os.ReadFile(filePid(modo))
	if err != nil {
		return
	}
	if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && pid > 0 {
		procAllowSetForegroundWindow.Call(uintptr(pid))
	}
}

// portaDavanti: file arrivati da un tasto destro nuovo mentre la finestra è già aperta (magari
// ridotta a icona o dietro Esplora file): la mostro, così si vede che il clic ha fatto qualcosa.
func portaDavanti() {
	w := finestra
	if w == nil || chiusa.Load() {
		return
	}
	w.Dispatch(func() {
		h := uintptr(w.Window())
		if h == 0 {
			return
		}
		if r, _, _ := procIsIconic.Call(h); r != 0 {
			procShowWindow.Call(h, 9) // SW_RESTORE
		}
		if r, _, _ := procSetForegroundWindow.Call(h); r == 0 {
			procFlashWindow.Call(h, 1) // se Windows non lo permette, almeno lampeggia
		}
	})
}

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
	_ = os.WriteFile(filePid(modo), []byte(strconv.Itoa(os.Getpid())), 0o644)
	return true, func() {
		_ = os.Remove(filePid(modo))
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

// etàMassimaRiga: le righe della coda più vecchie di così sono di un tasto destro di prima,
// rimaste perché la finestra è stata chiusa a forza (Gestione attività, PC spento): si buttano.
const etàMassimaRiga = time.Minute

// passaAlPrimo aggiunge i percorsi alla coda (riprova se il file è occupato in quel momento).
// Ogni riga ha davanti l'ora in cui è stata scritta.
func passaAlPrimo(modo string, percorsi []string) {
	lasciaDavantiIlPrimo(modo)
	ora := strconv.FormatInt(time.Now().UnixMilli(), 10)
	var b strings.Builder
	for _, p := range percorsi {
		b.WriteString(ora + "\t" + p + "\n")
	}
	riga := b.String()
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
		quando, percorso, ok := strings.Cut(strings.TrimSpace(r), "\t")
		ms, err := strconv.ParseInt(quando, 10, 64)
		if !ok || err != nil || ora.Sub(time.UnixMilli(ms)) > etàMassimaRiga {
			continue // riga vecchia (o senza ora): non è di questa selezione
		}
		r = strings.TrimSpace(percorso)
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

// raccogliCoda: aspetta gli altri Tasso Zip partiti insieme e unisce i loro file. Windows li
// avvia uno per file e con Defender o un disco lento gli ultimi arrivano tardi: la raccolta
// finisce solo quando per "quiete" non arriva più niente (al massimo dopo "massimo").
func raccogliCoda(modo string, miei []string, quiete, massimo time.Duration) []string {
	codaVistiMu.Lock()
	for _, p := range miei {
		codaVisti[strings.ToLower(p)] = time.Now()
	}
	codaVistiMu.Unlock()
	tutti := append([]string{}, miei...)
	inizio := time.Now()
	ultimo := inizio
	for time.Since(ultimo) < quiete && time.Since(inizio) < massimo {
		time.Sleep(75 * time.Millisecond)
		if nuovi := leggiCoda(modo); len(nuovi) > 0 {
			tutti = append(tutti, nuovi...)
			ultimo = time.Now()
		}
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
			portaDavanti()
		}
	}
}
