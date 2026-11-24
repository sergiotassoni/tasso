//go:build windows

package traduce

// La finestrella del badge di stato (badge.go): sempre sopra, trasparente ai clic, fuori dalle
// catture (non viene riletta dall'OCR), con trasparenza vera per pixel (UpdateLayeredWindow).
// Ha un filo tutto suo: badgeStato() non aspetta mai (si può chiamare da qualsiasi filo).

import (
	"image"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pUpdateLayered  = user32.NewProc("UpdateLayeredWindow")
	pKillTimer      = user32.NewProc("KillTimer")
	pBadgeDIB       = gdi32.NewProc("CreateDIBSection")
	pBadgeGdiFlush  = gdi32.NewProc("GdiFlush")
	badgeHwnd       atomic.Uintptr
	badgeOnce       sync.Once
	badgeStatoOra   atomic.Int32
	badgeDa         atomic.Int64 // quando è cambiato lo stato (UnixNano)
	badgeArea       atomic.Pointer[Zona]
	badgeConPuntino = true // dopo i 2 secondi resta un puntino minuscolo mentre traduco (facoltativo)
	cacheTestoBadge = map[string]testoMisurato{}
	muCacheTestoBdg sync.Mutex
)

const wmBadge = wmApp + 20

type testoMisurato struct {
	maschera []uint8
	w, h     int
}

// badgeStato: mostra il badge in questo stato sopra la finestra in primo piano.
func badgeStato(stato int) {
	badgeOnce.Do(func() { go filoBadge() })
	fg, _, _ := pGetForeground.Call()
	if a, ok := areaFinestra(fg); ok {
		badgeArea.Store(&a)
	} else if g := paginaGiocoA.Load(); g != 0 {
		if a, ok := areaFinestra(g); ok {
			badgeArea.Store(&a)
		}
	}
	if stato == statoBadgeAttivo && badgeStatoOra.Load() == statoBadgeAttivo &&
		time.Since(time.Unix(0, badgeDa.Load())) < 30*time.Second {
		return // ogni schermata nuova non rifà comparire il badge: solo all'accensione
	}
	badgeStatoOra.Store(int32(stato))
	badgeDa.Store(time.Now().UnixNano())
	for i := 0; i < 50 && badgeHwnd.Load() == 0; i++ { // la prima volta: aspetto la finestrella (al massimo 0,5 s)
		time.Sleep(10 * time.Millisecond)
	}
	if h := badgeHwnd.Load(); h != 0 {
		pPostMessage.Call(h, wmBadge, 0, 0)
	}
}

func filoBadge() {
	defer guard("badge")
	runtime.LockOSThread()
	classe, _ := windows.UTF16PtrFromString("TassoTraduceBadge")
	wc := wndClassEx{lpfnWndProc: windows.NewCallback(badgeProc), lpszClassName: classe}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	// LAYERED | TRANSPARENT (niente clic) | TOPMOST | TOOLWINDOW | NOACTIVATE
	ex := uintptr(0x80000 | 0x20 | 0x8 | 0x80 | 0x08000000)
	h, _, _ := pCreateWindowEx.Call(ex, uintptr(unsafe.Pointer(classe)), 0, 0x80000000, 0, 0, 10, 10, 0, 0, 0, 0)
	if h == 0 {
		return
	}
	pSetDisplayAffin.Call(h, affinita()) // l'OCR non deve leggere il badge
	badgeHwnd.Store(h)
	var m msgW
	for {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		pTranslateMsg.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMsg.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func badgeProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case wmBadge:
		pSetTimer.Call(hwnd, 1, 50, 0)
		aggiornaBadge(hwnd)
		return 0
	case 0x0113: // WM_TIMER
		if !aggiornaBadge(hwnd) {
			pKillTimer.Call(hwnd, 1)
		}
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wp, lp)
	return r
}

// aggiornaBadge: disegna il badge per lo stato e il tempo di adesso. false = finito (timer fermo).
func aggiornaBadge(hwnd uintptr) bool {
	stato := int(badgeStatoOra.Load())
	da := time.Since(time.Unix(0, badgeDa.Load()))
	area := badgeArea.Load()
	nascondi := func() bool {
		pShowWindow.Call(hwnd, 0)
		return false
	}
	if area == nil || stato == statoBadgeNiente {
		return nascondi()
	}
	// il gioco non è più davanti (Alt+Tab): via subito
	fg, _, _ := pGetForeground.Call()
	if stato != statoBadgeNonGioco && paginaGiocoA.Load() != 0 && fg != paginaGiocoA.Load() {
		return nascondi()
	}
	scala := math.Max(1, math.Min(2, float64(area.H)/1080))
	alfa, pulsa := 1.0, 0.0
	var img *image.RGBA
	switch {
	case stato == statoBadgeTraduco:
		if da > 20*time.Second || !paginaAttiva.Load() {
			return nascondi()
		}
		pulsa = 0.5 + 0.5*math.Sin(float64(da.Milliseconds())/1000*2*math.Pi/1.4) // un respiro ogni 1,4 s
	case da < 2*time.Second:
	case da < 2300*time.Millisecond:
		alfa = 1 - float64(da-2*time.Second)/float64(300*time.Millisecond)
	default:
		if stato == statoBadgeAttivo && badgeConPuntino && paginaAttiva.Load() && !paginaUnaVolta.Load() {
			img = puntinoBadge(scala) // resta solo un puntino minuscolo
		} else {
			return nascondi()
		}
	}
	if img == nil {
		t := misuraTestoBadge(testoBadge(stato, nomeTasto("pagina")), scala)
		img = componiBadge(t.maschera, t.w, t.h, scala, pulsa, stato == statoBadgeSpento || stato == statoBadgeNonGioco, alfa)
	}
	w := img.Bounds().Dx()
	x := int32(area.X + (area.W-w)/2)
	y := int32(area.Y + int(14*scala))
	mostraLayered(hwnd, img, x, y)
	// il puntino fermo non ha bisogno del timer veloce: lo controllo due volte al secondo
	if img.Bounds().Dx() < 20 {
		pSetTimer.Call(hwnd, 1, 500, 0)
	}
	return true
}

// mostraLayered: mette l'immagine (con trasparenza per pixel) nella finestra, in x,y dello schermo.
func mostraLayered(hwnd uintptr, img *image.RGBA, x, y int32) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	scr, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, scr)
	mem, _, _ := pCreateCompatDC.Call(scr)
	defer pDeleteDC.Call(mem)
	bi := bitmapInfoHeader{biWidth: int32(w), biHeight: int32(-h), biPlanes: 1, biBitCount: 32}
	bi.biSize = uint32(unsafe.Sizeof(bi))
	var bits unsafe.Pointer
	bmp, _, _ := pBadgeDIB.Call(mem, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == nil {
		return
	}
	defer pDeleteObject.Call(bmp)
	old, _, _ := pSelectObject.Call(mem, bmp)
	defer pSelectObject.Call(mem, old)
	copy(unsafe.Slice((*byte)(bits), w*h*4), premoltiplicaBGRA(img))
	pBadgeGdiFlush.Call()
	pos := [2]int32{x, y}
	dim := [2]int32{int32(w), int32(h)}
	zero := [2]int32{0, 0}
	blend := uint32(0x01FF0000) // AC_SRC_OVER, 0, 255, AC_SRC_ALPHA
	pUpdateLayered.Call(hwnd, scr, uintptr(unsafe.Pointer(&pos)), uintptr(unsafe.Pointer(&dim)), mem,
		uintptr(unsafe.Pointer(&zero)), 0, uintptr(unsafe.Pointer(&blend)), 2) // ULW_ALPHA
	pShowWindow.Call(hwnd, 4)                                       // SW_SHOWNOACTIVATE
	pSetWindowPos.Call(hwnd, ^uintptr(0), 0, 0, 0, 0, 0x1|0x2|0x10) // sempre sopra, senza fuoco
}

// misuraTestoBadge: il testo scritto con Segoe UI Semibold, bianco su nero, letto come maschera.
func misuraTestoBadge(s string, scala float64) testoMisurato {
	px := int32(15*scala + 0.5)
	chiave := s + "\x00" + string(rune(px))
	muCacheTestoBdg.Lock()
	if t, ok := cacheTestoBadge[chiave]; ok {
		muCacheTestoBdg.Unlock()
		return t
	}
	muCacheTestoBdg.Unlock()
	nome, _ := windows.UTF16PtrFromString("Segoe UI Semibold")
	// altezza negativa = grandezza del carattere; 4 = ANTIALIASED_QUALITY (niente frange colorate)
	font, _, _ := pCreateFont.Call(uintptr(-px), 0, 0, 0, 600, 0, 0, 0, 1, 0, 0, 4, 0, uintptr(unsafe.Pointer(nome)))
	defer pDeleteObject.Call(font)
	mem, _, _ := pCreateCompatDC.Call(0)
	defer pDeleteDC.Call(mem)
	pSelectObject.Call(mem, font)
	t := testoW(s)
	var r rect
	pDrawText.Call(mem, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&r)), 0x400|0x20|0x800) // CALCRECT|SINGLELINE|NOPREFIX
	w, h := int(r.Right-r.Left)+2, int(r.Bottom-r.Top)
	if w <= 2 || h <= 0 {
		return testoMisurato{maschera: []uint8{0}, w: 1, h: 1}
	}
	bi := bitmapInfoHeader{biWidth: int32(w), biHeight: int32(-h), biPlanes: 1, biBitCount: 32}
	bi.biSize = uint32(unsafe.Sizeof(bi))
	var bits unsafe.Pointer
	bmp, _, _ := pBadgeDIB.Call(mem, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == nil {
		return testoMisurato{maschera: []uint8{0}, w: 1, h: 1}
	}
	defer pDeleteObject.Call(bmp)
	pSelectObject.Call(mem, bmp)
	pSelectObject.Call(mem, font)
	pSetBkMode.Call(mem, 1)
	pSetTextColor.Call(mem, 0x00FFFFFF)
	rr := rect{1, 0, int32(w), int32(h)}
	pDrawText.Call(mem, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&rr)), 0x20|0x800)
	pBadgeGdiFlush.Call()
	px4 := unsafe.Slice((*byte)(bits), w*h*4)
	m := testoMisurato{maschera: make([]uint8, w*h), w: w, h: h}
	for i := range m.maschera {
		m.maschera[i] = max(px4[i*4], px4[i*4+1], px4[i*4+2])
	}
	muCacheTestoBdg.Lock()
	cacheTestoBadge[chiave] = m
	muCacheTestoBdg.Unlock()
	return m
}
