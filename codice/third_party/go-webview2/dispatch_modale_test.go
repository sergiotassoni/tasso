//go:build windows
// +build windows

package webview2

// Blocco al primo avvio (FIX-BLOCCO-AVVIO): le risposte per la pagina (Dispatch) devono arrivare
// anche quando il filo della finestra è dentro un ciclo di messaggi interno (menu, spostamento,
// finestra modale). Un ciclo così consegna solo i messaggi delle finestre: quelli al filo
// (PostThreadMessage, come prima) si perdono. Qui il ciclo interno è simulato: si servono solo
// i messaggi con una finestra, come fa DispatchMessage di un ciclo modale.

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/jchv/go-webview2/internal/w32"
	"golang.org/x/sys/windows"
)

func finestraDiProva(t *testing.T) *webview {
	t.Helper()
	var hinst windows.Handle
	_ = windows.GetModuleHandleEx(0, nil, &hinst)
	cls, _ := windows.UTF16PtrFromString("TassoProvaDispatch")
	wc := w32.WndClassExW{
		CbSize:        uint32(unsafe.Sizeof(w32.WndClassExW{})),
		HInstance:     hinst,
		LpszClassName: cls,
		LpfnWndProc:   windows.NewCallback(wndproc),
	}
	_, _, _ = w32.User32RegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	h, _, _ := w32.User32CreateWindowExW.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(cls)), 0, 0, 0, 10, 10, 0, 0, uintptr(hinst), 0)
	if h == 0 {
		t.Skip("finestra di prova non creata")
	}
	w := &webview{hwnd: h}
	w.mainthread, _, _ = w32.Kernel32GetCurrentThreadID.Call()
	w.fuori = true // la X non passa da ChiusuraHook
	setWindowContext(h, w)
	atomic.StoreInt32(&w.dispatchPronto, 1)
	t.Cleanup(func() {
		atomic.StoreInt32(&w.dispatchPronto, 0)
		windowContextSync.Lock()
		delete(windowContext, h)
		windowContextSync.Unlock()
		_, _, _ = w32.User32DestroyWindow.Call(h)
	})
	return w
}

// cicloModale: serve per un po' solo i messaggi delle finestre, come un menu o una finestra modale.
func cicloModale(fatto *atomic.Bool, quanto time.Duration) {
	const pmRemove = 0x0001
	var msg w32.Msg
	for t0 := time.Now(); time.Since(t0) < quanto && !fatto.Load(); {
		r, _, _ := user32PeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, pmRemove)
		if r == 0 {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		if msg.Hwnd == 0 {
			continue // messaggio al filo: in un ciclo interno si perde
		}
		_, _, _ = w32.User32DispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func TestDispatchNelCicloModale(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	w := finestraDiProva(t)
	var fatto atomic.Bool
	go w.Dispatch(func() { fatto.Store(true) })
	cicloModale(&fatto, 3*time.Second)
	if !fatto.Load() {
		t.Fatal("la risposta mandata con Dispatch si è persa nel ciclo interno")
	}
}

func TestTimerSvuotaLaCoda(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	w := finestraDiProva(t)
	_, _, _ = user32SetTimer.Call(w.hwnd, idTimerDispatch, 250, 0)
	defer user32KillTimer.Call(w.hwnd, idTimerDispatch)
	var fatto atomic.Bool
	// messo in coda senza nessun segnale (come un segnale perso): lo deve prendere il timer
	w.m.Lock()
	w.dispatchq = append(w.dispatchq, func() { fatto.Store(true) })
	w.m.Unlock()
	cicloModale(&fatto, 3*time.Second)
	if !fatto.Load() {
		t.Fatal("il timer non ha svuotato la coda di Dispatch")
	}
}

func TestSvuotaDispatchSenzaRientri(t *testing.T) {
	w := &webview{}
	var ordine []int
	w.dispatchq = []func(){
		func() {
			ordine = append(ordine, 1)
			w.Dispatch(func() { ordine = append(ordine, 3) })
			w.svuotaDispatch()
		},
		func() { ordine = append(ordine, 2) },
	}
	w.mainthread, _, _ = w32.Kernel32GetCurrentThreadID.Call()
	w.svuotaDispatch()
	if len(ordine) != 3 || ordine[0] != 1 || ordine[1] != 2 || ordine[2] != 3 {
		t.Fatalf("ordine sbagliato o rientro: %v", ordine)
	}
	// il messaggio al filo mandato da Dispatch (finestra non pronta) lo tolgo, per non lasciarlo in giro
	var msg w32.Msg
	_, _, _ = user32PeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, w32.WMApp, w32.WMApp, 1)
}
