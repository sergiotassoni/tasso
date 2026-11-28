//go:build windows

// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

// Dal menu del tasto destro, se scegli 5 file, Windows avvia Tasso PDF 5 volte (una per file).
// Il primo che parte (un mutex di Windows) apre la finestra; gli altri scrivono il loro
// percorso in un piccolo file di coda e si chiudono subito, senza finestra (come Tasso Zip).
// ZP07-5: un solo mutex per tutti i modi, anche avviando senza file (menu Start), e la prima
// copia legge la coda per tutta la sua vita: un doppio clic su un PDF molto dopo lo apre
// sempre nella finestra già aperta, che viene in primo piano.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	codaVisti   = map[string]time.Time{} // percorso -> quando è arrivato (contro i doppioni)
	codaVistiMu sync.Mutex
)

// voceCoda: una riga della coda: "modo\tpercorso" ("mostra\t" = solo in primo piano).
type voceCoda struct{ modo, percorso string }

func fileCoda() string { return filepath.Join(cartellaDatiWin(), "coda.txt") }

func sonoIlPrimo() (bool, func()) {
	nome, _ := windows.UTF16PtrFromString(`Local\TassoPDF`)
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
	_ = os.Remove(fileCoda()) // righe vecchie di una copia che si è chiusa male
	return true, func() {
		_ = windows.ReleaseMutex(h)
		windows.CloseHandle(h)
		_ = os.Remove(fileCoda())
	}
}

var (
	pAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
	pSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	pShowWindow               = user32.NewProc("ShowWindow")
	pIsIconic                 = user32.NewProc("IsIconic")
	pShutdownBlockReasonCrea  = user32.NewProc("ShutdownBlockReasonCreate")
	pShutdownBlockReasonTogli = user32.NewProc("ShutdownBlockReasonDestroy")
)

func passaAlPrimo(modo string, percorsi []string) {
	// La copia avviata dall'utente può dare il primo piano alla prima (ASFW_ANY).
	_, _, _ = pAllowSetForegroundWindow.Call(uintptr(0xFFFFFFFF))
	_ = os.MkdirAll(cartellaDatiWin(), 0o700)
	var b strings.Builder
	if len(percorsi) == 0 {
		b.WriteString("mostra\t\n")
	}
	for _, p := range percorsi {
		b.WriteString(modo + "\t" + p + "\n")
	}
	for i := 0; i < 40; i++ {
		f, err := os.OpenFile(fileCoda(), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
		if err == nil {
			_, err = f.WriteString(b.String())
			f.Close()
			if err == nil {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func leggiCoda() []voceCoda {
	p := fileCoda()
	b, err := os.ReadFile(p)
	if err != nil || len(b) == 0 {
		return nil
	}
	if fi, err := os.Stat(p); err == nil && fi.Size() == int64(len(b)) {
		_ = os.Truncate(p, 0)
	}
	var out []voceCoda
	codaVistiMu.Lock()
	defer codaVistiMu.Unlock()
	ora := time.Now()
	for _, r := range strings.Split(string(b), "\n") {
		m, q, ok := strings.Cut(strings.TrimSpace(r), "\t")
		if !ok {
			continue
		}
		if m == "mostra" {
			out = append(out, voceCoda{modo: m})
			continue
		}
		q = strings.TrimSpace(q)
		if !modiRiga[m] || q == "" {
			continue
		}
		k := strings.ToLower(q)
		if t, ok := codaVisti[k]; ok && ora.Sub(t) < 5*time.Second {
			continue // lo stesso file arrivato due volte insieme
		}
		if fi, err := os.Stat(q); err != nil || fi.IsDir() {
			continue
		}
		codaVisti[k] = ora
		out = append(out, voceCoda{modo: m, percorso: q})
	}
	return out
}

// raccogliCoda: all'avvio, per un attimo, i file delle altre copie avviate insieme. Quelli dello
// stesso modo vanno con i miei; gli altri arrivano dopo.
func raccogliCoda(modo string, miei []string, attesa time.Duration) ([]string, []voceCoda) {
	codaVistiMu.Lock()
	for _, p := range miei {
		codaVisti[strings.ToLower(p)] = time.Now()
	}
	codaVistiMu.Unlock()
	tutti := append([]string{}, miei...)
	var altri []voceCoda
	fine := time.Now().Add(attesa)
	for time.Now().Before(fine) {
		time.Sleep(75 * time.Millisecond)
		for _, v := range leggiCoda() {
			if v.modo == modo {
				tutti = append(tutti, v.percorso)
			} else if v.percorso != "" {
				altri = append(altri, v)
			}
		}
	}
	return tutti, altri
}

// ascoltaCoda: per tutta la vita del programma, i file (o il «vieni avanti») di altre copie.
func ascoltaCoda() {
	for {
		time.Sleep(300 * time.Millisecond)
		vv := leggiCoda()
		if len(vv) == 0 {
			continue
		}
		for _, v := range vv {
			if v.percorso != "" {
				arrivano(v.modo, []string{v.percorso})
			}
		}
		if nuoviArrivati != nil {
			nuoviArrivati(true)
		}
	}
}

// inPrimoPiano: la finestra già aperta viene davanti (anche se era ridotta a icona).
func inPrimoPiano(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	if r, _, _ := pIsIconic.Call(hwnd); r != 0 {
		_, _, _ = pShowWindow.Call(hwnd, 9) // SW_RESTORE
	}
	_, _, _ = pSetForegroundWindow.Call(hwnd)
}

// testiSpegnimento: il motivo che Windows mostra se Tasso PDF ferma lo spegnimento (ZP07-8).
var testiSpegnimento = map[string]string{
	"it": "Documenti PDF non salvati", "en": "Unsaved PDF documents", "es": "Documentos PDF sin guardar",
	"fr": "Documents PDF non enregistrés", "de": "Nicht gespeicherte PDF-Dokumente",
	"pt": "Documentos PDF não guardados", "pl": "Niezapisane dokumenty PDF",
}

// bloccaSpegnimento: con lavoro non salvato Windows sa perché Tasso PDF chiede di aspettare.
func bloccaSpegnimento(hwnd uintptr, on bool) {
	if hwnd == 0 {
		return
	}
	if !on {
		_, _, _ = pShutdownBlockReasonTogli.Call(hwnd)
		return
	}
	t := testiSpegnimento[linguaIniziale()]
	if t == "" {
		t = testiSpegnimento["en"]
	}
	p, _ := windows.UTF16PtrFromString(t)
	_, _, _ = pShutdownBlockReasonCrea.Call(hwnd, uintptr(unsafe.Pointer(p)))
}
