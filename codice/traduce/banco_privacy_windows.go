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
	bpGetWindow   = bpUser32.NewProc("GetWindow")
	bpTopWindow   = bpUser32.NewProc("GetTopWindow")
	bpIsVisible   = bpUser32.NewProc("IsWindowVisible")
	bpLongPtr     = bpUser32.NewProc("GetWindowLongPtrW")
	bpThreadPid   = bpUser32.NewProc("GetWindowThreadProcessId")
	bpDwm         = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmGetWindowAttribute")
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

// davantiSchermoIntero: davanti c'è una finestra che copre tutto il suo schermo e non è la
// scrivania. Prima la finestra col fuoco; se non lo è (il fuoco è rimasto su un'altra finestra
// mentre il video a schermo intero è comunque sopra tutto, prova del 26/11 12:00: 4 fotogrammi in
// 50 minuti), la prima finestra vera dall'alto (visibile, non nascosta da Windows, non di questo
// programma, non una finestrella trasparente sopra le altre).
func davantiSchermoIntero() bool {
	h, _, _ := bpForeground.Call()
	if finestraSchermoIntero(h) {
		return true
	}
	return finestraSchermoIntero(primaFinestraVera())
}

func primaFinestraVera() uintptr {
	mio := uint32(windows.GetCurrentProcessId())
	h, _, _ := bpTopWindow.Call(0)
	for i := 0; h != 0 && i < 500; i++ {
		if vera(h, mio) {
			return h
		}
		h, _, _ = bpGetWindow.Call(h, 2) // GW_HWNDNEXT
	}
	return 0
}

func vera(h uintptr, mio uint32) bool {
	if v, _, _ := bpIsVisible.Call(h); v == 0 {
		return false
	}
	var pid uint32
	bpThreadPid.Call(h, uintptr(unsafe.Pointer(&pid)))
	if pid == mio {
		return false
	}
	gwlExStyle := -20
	ex, _, _ := bpLongPtr.Call(h, uintptr(gwlExStyle))
	if ex&0x20 != 0 || ex&0x80 != 0 { // WS_EX_TRANSPARENT, WS_EX_TOOLWINDOW
		return false
	}
	var cloaked uint32
	if r, _, _ := bpDwm.Call(h, 14, uintptr(unsafe.Pointer(&cloaked)), 4); r == 0 && cloaked != 0 { // DWMWA_CLOAKED
		return false
	}
	var r bpRect
	if ok, _, _ := bpWindowRect.Call(h, uintptr(unsafe.Pointer(&r))); ok == 0 || r.R-r.L < 50 || r.B-r.T < 50 {
		return false
	}
	return true
}

func finestraSchermoIntero(h uintptr) bool {
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
