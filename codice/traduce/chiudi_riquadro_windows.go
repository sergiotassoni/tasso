//go:build windows

package traduce

// La X sul riquadro della traduzione (28/11, Sergio non sapeva come spegnerlo). Il riquadro
// lascia passare i clic al gioco; la X è una finestrella a parte, piccola, sopra il suo angolo
// in alto a destra: si clicca senza rubare il fuoco al gioco (NOACTIVATE e MA_NOACTIVATE) e
// spegne come F7. Fuori dalle catture: l'OCR non la legge.

import (
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	chiudiHwnd   atomic.Uintptr
	chiudiSopra  atomic.Bool // il mouse è sopra la X (si disegna più chiara)
	pTrackMouse  = user32.NewProc("TrackMouseEvent")
	pSetCursorTT = user32.NewProc("SetCursor")
)

type trackMouseEvent struct {
	cbSize      uint32
	dwFlags     uint32
	hwndTrack   uintptr
	dwHoverTime uint32
}

// creaTastoChiudi: dal filo della finestrella (stesso filo dei messaggi del riquadro).
func creaTastoChiudi() {
	classe, _ := windows.UTF16PtrFromString("TassoTraduceChiudi")
	cur, _, _ := pLoadCursor.Call(0, 32649) // IDC_HAND
	wc := wndClassEx{lpfnWndProc: windows.NewCallback(chiudiProc), lpszClassName: classe, hCursor: cur}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	// LAYERED | TOPMOST | TOOLWINDOW | NOACTIVATE: niente TRANSPARENT, questa si clicca
	ex := uintptr(0x80000 | 0x8 | 0x80 | 0x08000000)
	h, _, _ := pCreateWindowEx.Call(ex, uintptr(unsafe.Pointer(classe)), 0, 0x80000000, 0, 0, 28, 28, 0, 0, 0, 0)
	if h == 0 {
		return
	}
	pSetLayeredAttr.Call(h, 0, 255, 2) // piena
	pSetDisplayAffin.Call(h, affinita())
	chiudiHwnd.Store(h)
}

// latoChiudi: grandezza della X per la grandezza del testo del riquadro.
func latoChiudi(dimFont int) int32 {
	return int32(max(26, dimFont+6))
}

// mostraTastoChiudi: la X nell'angolo in alto a destra del riquadro (x, y, larghezza del riquadro).
func mostraTastoChiudi(x, y, larg int32, dimFont int) {
	h := chiudiHwnd.Load()
	if h == 0 {
		return
	}
	l := latoChiudi(dimFont)
	pSetWindowPos.Call(h, ^uintptr(0), uintptr(x+larg-l-4), uintptr(y+4), uintptr(l), uintptr(l), swpNoActivate)
	pShowWindow.Call(h, 4) // SW_SHOWNOACTIVATE
	pInvalidateRect.Call(h, 0, 1)
}

func nascondiTastoChiudi() {
	if h := chiudiHwnd.Load(); h != 0 {
		pShowWindow.Call(h, 0)
	}
}

func chiudiProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case 0x0021: // WM_MOUSEACTIVATE: il clic non porta il fuoco via dal gioco
		return 3 // MA_NOACTIVATE
	case 0x0200: // WM_MOUSEMOVE
		if !chiudiSopra.Swap(true) {
			t := trackMouseEvent{dwFlags: 0x2, hwndTrack: hwnd} // TME_LEAVE
			t.cbSize = uint32(unsafe.Sizeof(t))
			pTrackMouse.Call(uintptr(unsafe.Pointer(&t)))
			pInvalidateRect.Call(hwnd, 0, 1)
		}
		return 0
	case 0x02A3: // WM_MOUSELEAVE
		chiudiSopra.Store(false)
		pInvalidateRect.Call(hwnd, 0, 1)
		return 0
	case 0x0202: // WM_LBUTTONUP
		pShowWindow.Call(hwnd, 0)
		go func() {
			defer guard("chiudi-riquadro")
			spegniDalRiquadro()
		}()
		return 0
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		var r rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
		fondo := uintptr(colTastoChiudi)
		if chiudiSopra.Load() {
			fondo = uintptr(colTastoChiudiSopra)
		}
		br, _, _ := pCreateBrush.Call(fondo)
		pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), br)
		pDeleteObject.Call(br)
		bordo, _, _ := pCreateBrush.Call(0x00A6F32F) // verde menta del riquadro (BGR)
		pFrameRect.Call(hdc, uintptr(unsafe.Pointer(&r)), bordo)
		pDeleteObject.Call(bordo)
		pSetBkMode.Call(hdc, 1)
		face, _ := windows.UTF16PtrFromString("Segoe UI Symbol")
		px := (r.Bottom - r.Top) * 2 / 3
		font, _, _ := pCreateFont.Call(uintptr(-px), 0, 0, 0, 700, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
		vecchio, _, _ := pSelectObject.Call(hdc, font)
		pSetTextColor.Call(hdc, 0x00F8F4F2)
		t := testoW("✕")
		pDrawText.Call(hdc, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&r)), 0x1|0x4|0x20|0x800) // CENTER|VCENTER|SINGLELINE|NOPREFIX
		pSelectObject.Call(hdc, vecchio)
		pDeleteObject.Call(font)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wp, lp)
	return r
}
