// Tasso 3.1.1 — "Bentornato" a finestra aperta: dopo il riavvio/spegnimento scelto a fine pulizia,
// Tasso partito con Windows nella tray apre da solo la finestra davanti, così il riepilogo si vede
// subito. Negli avvii normali con Windows resta nella tray. Con un gioco davanti non ruba il fuoco.
package main

import (
	"encoding/json"
	"os"
	"time"
	"unsafe"
)

var (
	bentAttach    = user32.NewProc("AttachThreadInput")
	bentBringTop  = user32.NewProc("BringWindowToTop")
	bentFlash     = user32.NewProc("FlashWindowEx")
	bentCurThread = kernel32.NewProc("GetCurrentThreadId")
)

// bentornatoPronto: c'è un Bentornato da mostrare a questo avvio. Guarda soltanto, non lo consuma
// (lo legge e lo cancella la pagina, con goBentornato: resta "una volta sola").
func bentornatoPronto() bool {
	b, err := os.ReadFile(bentornatoFile())
	if err != nil {
		return false
	}
	var r riepilogoPulizia
	if json.Unmarshal(b, &r) != nil {
		return false
	}
	return bentornatoDaMostrare(r, avvioWindows(), time.Now().Unix())
}

// apriBentornatoAllAvvio: chiamata da main prima di w.Run. Aspetta qualche secondo (all'avvio con
// Windows il desktop può non essere ancora pronto), poi apre la finestra davanti. Se c'è un gioco
// davanti riprova ogni 30 secondi (fino a circa 20 minuti); il Bentornato resta comunque nella
// pagina e si vede appena riapri Tasso dall'icona.
func apriBentornatoAllAvvio(partitoNascosto, puliziaAutomatica bool) {
	if !partitoNascosto || puliziaAutomatica || !bentornatoPronto() {
		return
	}
	go func() {
		defer guard("bentornato_apri.go")
		time.Sleep(7 * time.Second)
		for i := 0; i < 40; i++ {
			setMu.Lock()
			gaming := settings.Mode == "gaming"
			setMu.Unlock()
			schermo := primoPianoSchermoIntero()
			gioco := false
			if gaming && !schermo {
				gioco = runningGame() != ""
			}
			if bentornatoFinestra(true, true, false, schermo, gaming, gioco) == bentFinestraApri {
				if wv != nil {
					wv.Dispatch(func() { portaDavantiBentornato(uintptr(wv.Window())) })
					scriviAvvio("finestra aperta per il Bentornato")
				}
				return
			}
			time.Sleep(30 * time.Second)
		}
	}()
}

// portaDavantiBentornato (nel filo della finestra): mostra la finestra e la porta davanti. Windows
// non lascia prendere il fuoco a un programma in background: mi aggancio per un attimo al filo della
// finestra davanti (AttachThreadInput, come vault_compila.go); se non basta, lampeggia sulla barra.
func portaDavantiBentornato(h uintptr) {
	if h == 0 {
		return
	}
	if v, _, _ := pIsWindowVisible.Call(h); v != 0 {
		if ic, _, _ := pIsIconic.Call(h); ic == 0 {
			return // l'hai già aperta tu
		}
	}
	portaDavanti(h)
}

// portaDavanti: mostra la finestra (anche se è nascosta nell'icona vicino all'orologio) e la porta
// davanti. Se Windows rifiuta SetForegroundWindow (programma in background, avvio lento la prima
// volta dopo l'installazione) mi aggancio un attimo al filo della finestra davanti; se non basta
// ancora, la finestra lampeggia sulla barra delle applicazioni. Usata anche da bringToFront.
func portaDavanti(h uintptr) {
	if h == 0 {
		return
	}
	mostraFinestra(h) // tray.go: SW_SHOW, SW_RESTORE, SetForegroundWindow
	fg, _, _ := pForegroundWin.Call()
	if fg == h {
		return
	}
	ct, _, _ := bentCurThread.Call()
	if fg != 0 {
		if ft, _, _ := pGetWindowPid.Call(fg, 0); ft != 0 && ft != ct {
			bentAttach.Call(ct, ft, 1)
			bentBringTop.Call(h)
			pSetFg.Call(h)
			bentAttach.Call(ct, ft, 0)
		}
	}
	if fg2, _, _ := pForegroundWin.Call(); fg2 != h {
		fi := struct {
			Size    uint32
			Hwnd    uintptr
			Flags   uint32
			Count   uint32
			Timeout uint32
		}{Hwnd: h, Flags: 3 | 12, Count: 5} // FLASHW_ALL | FLASHW_TIMERNOFG
		fi.Size = uint32(unsafe.Sizeof(fi))
		bentFlash.Call(uintptr(unsafe.Pointer(&fi)))
	}
}
