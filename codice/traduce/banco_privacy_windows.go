//go:build windows

package traduce

// Privacy del banco sullo schermo (TassoProva): si fotografa solo quando davanti c'è una finestra
// a schermo intero (il gioco o il video). Nella prova finale del 26/11 il primo fotogramma era la
// scrivania: nel report erano finiti nomi, l'email, la rete Wi-Fi e il nome del PC.

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	bpUser32      = windows.NewLazySystemDLL("user32.dll")
	bpForeground  = bpUser32.NewProc("GetForegroundWindow")
	bpWindowRect  = bpUser32.NewProc("GetWindowRect")
	bpMonitorFrom = bpUser32.NewProc("MonitorFromWindow")
	bpMonitorInfo = bpUser32.NewProc("GetMonitorInfoW")
	bpClassName   = bpUser32.NewProc("GetClassNameW")
	bpIsIconic    = bpUser32.NewProc("IsIconic")
)

type bpRect struct{ L, T, R, B int32 }

type bpMonInfo struct {
	Size          uint32
	Monitor, Work bpRect
	Flags         uint32
}

// classiScrivania: le finestre di Windows che non sono un gioco né un video (scrivania, barra,
// Esplora file, menu Start).
var classiScrivania = map[string]bool{"progman": true, "workerw": true, "shell_traywnd": true, "cabinetwclass": true,
	"explorewclass": true, "windows.ui.core.corewindow": true, "shell_secondarytraywnd": true, "notifyiconoverflowwindow": true}

// davantiSchermoIntero: la finestra davanti copre tutto il suo schermo e non è la scrivania.
func davantiSchermoIntero() bool {
	h, _, _ := bpForeground.Call()
	if h == 0 {
		return false
	}
	if r, _, _ := bpIsIconic.Call(h); r != 0 {
		return false
	}
	buf := make([]uint16, 128)
	n, _, _ := bpClassName.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if classiScrivania[strings.ToLower(windows.UTF16ToString(buf[:n]))] {
		return false
	}
	var r bpRect
	if ok, _, _ := bpWindowRect.Call(h, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return false
	}
	mon, _, _ := bpMonitorFrom.Call(h, 2) // MONITOR_DEFAULTTONEAREST
	mi := bpMonInfo{Size: uint32(unsafe.Sizeof(bpMonInfo{}))}
	if ok, _, _ := bpMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi))); ok == 0 {
		return false
	}
	m := mi.Monitor
	return r.L <= m.L+2 && r.T <= m.T+2 && r.R >= m.R-2 && r.B >= m.B-2
}
