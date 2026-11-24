package main

// La finestrella del Portachiavi "Vuoi usare uno di questi account?" che compare sotto il campo
// di login. È una finestra Win32 disegnata con GDI nei colori di Tasso: sempre in primo piano ma
// non prende mai la tastiera (WS_EX_NOACTIVATE), così il campo resta selezionato. Si chiude con Esc,
// con un clic fuori o quando cambi campo o finestra.
// Lo stesso filo ascolta i cambi di finestra e di campo (SetWinEventHook) e la scorciatoia.

import (
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	vpElenco  = 1 // account del sito
	vpChiuso  = 2 // portachiavi chiuso: "Sblocca il portachiavi"
	vpNessuno = 3 // scorciatoia senza account per il sito

	wmVtMostra   = 0x0400 + 41
	wmVtNascondi = 0x0400 + 42
	wmVtTasto    = 0x0400 + 43
	vtHotID      = 7
	vtMaxRighe   = 5
)

type vtPopDati struct {
	modo   int
	voci   []vtVoce
	anchor rect
	top    uintptr
	ctx    string
	hint   string
	first  bool
	isPw   bool
}

var (
	vtgDrawText    = user32.NewProc("DrawTextW")
	vtgRoundRgn    = gdi32.NewProc("CreateRoundRectRgn")
	vtgSetRgn      = user32.NewProc("SetWindowRgn")
	vtgRoundRect   = gdi32.NewProc("RoundRect")
	vtgCreatePen   = gdi32.NewProc("CreatePen")
	vtgStockObj    = gdi32.NewProc("GetStockObject")
	vtgEllipse     = gdi32.NewProc("Ellipse")
	vtgDrawIcon    = user32.NewProc("DrawIconEx")
	vtgLoadImage   = user32.NewProc("LoadImageW")
	vtgModule      = kernel32.NewProc("GetModuleHandleW")
	vtgDelObj      = gdi32.NewProc("DeleteObject")
	vtgKillTimer   = user32.NewProc("KillTimer")
	vtgSetTimer    = user32.NewProc("SetTimer")
	vtgPost        = user32.NewProc("PostMessageW")
	vtgSend        = user32.NewProc("SendMessageW")
	vtgWinHook     = user32.NewProc("SetWinEventHook")
	vtgMonFromRect = user32.NewProc("MonitorFromRect")
	vtgMonInfo     = user32.NewProc("GetMonitorInfoW")
	vtgDpi         = user32.NewProc("GetDpiForWindow")
	vtgSetCursor   = user32.NewProc("SetCursor")
	vtgLoadCursor  = user32.NewProc("LoadCursorW")
	vtgCursorPos   = user32.NewProc("GetCursorPos")
	vtgWinRect     = user32.NewProc("GetWindowRect")
	vtgSetPos      = user32.NewProc("SetWindowPos")
	vtgCompatDC    = gdi32.NewProc("CreateCompatibleDC")
	vtgCompatBmp   = gdi32.NewProc("CreateCompatibleBitmap")
	vtgBitBlt      = gdi32.NewProc("BitBlt")
	vtgDeleteDC    = gdi32.NewProc("DeleteDC")
	vtgRegClass    = user32.NewProc("RegisterClassExW")
	vtgCreateWin   = user32.NewProc("CreateWindowExW")
	vtgDefProc     = user32.NewProc("DefWindowProcW")
	vtgGetMsg      = user32.NewProc("GetMessageW")
	vtgTransMsg    = user32.NewProc("TranslateMessage")
	vtgDispMsg     = user32.NewProc("DispatchMessageW")
	vtgBeginPaint  = user32.NewProc("BeginPaint")
	vtgEndPaint    = user32.NewProc("EndPaint")
	vtgFillRect    = user32.NewProc("FillRect")
	vtgInvalidate  = user32.NewProc("InvalidateRect")
	vtgCreateFont  = gdi32.NewProc("CreateFontW")
	vtgBrush       = gdi32.NewProc("CreateSolidBrush")
	vtgSelect      = gdi32.NewProc("SelectObject")
	vtgBkMode      = gdi32.NewProc("SetBkMode")
	vtgTextColor   = gdi32.NewProc("SetTextColor")
	vtgRegHot      = user32.NewProc("RegisterHotKey")
	vtgUnregHot    = user32.NewProc("UnregisterHotKey")
	vtgAffinity    = user32.NewProc("SetWindowDisplayAffinity")

	vtPopMu    sync.Mutex
	vtPop      vtPopDati
	vtPopHwnd  uintptr
	vtHover    = -1
	vtScale    = 1.0
	vtFonts    [4]uintptr // titolo, nome, piccolo, iniziale
	vtFontSc   float64
	vtIcon     uintptr
	vtMouseGiu bool
	vtFocusCB  = windows.NewCallback(vtWinEvent)
	vtHotOK    bool
)

// Colori di Tasso (COLORREF = 0x00BBGGRR).
const (
	vcBg    = 0x1E1412 // --panel #12141E
	vcLine  = 0x3A2622 // --line  #22263A
	vcHover = 0x2E2320 // panel con un po' di testo sopra
	vcText  = 0xF8F4F2 // --text  #F2F4F8
	vcMuted = 0xA7918B // --muted #8B91A7
	vcA1    = 0xA6F32F // --a1    #2FF3A6
	vcInk   = 0x0D1404 // --ink   #04140D
	vcA3    = 0xFFA84D // --a3    #4DA8FF
	vcA4    = 0x47B5FF // --a4    #FFB547
)

type vtWndClass struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type vtMsgT struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	ptX     int32
	ptY     int32
	private uint32
}

type vtPaintT struct {
	hdc         uintptr
	fErase      int32
	rcPaint     rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

type vtMonInfoT struct {
	cbSize  uint32
	monitor rect
	work    rect
	flags   uint32
}

func vtFilo() {
	defer guard("vault_popup.go:vtFilo")
	runtime.LockOSThread()
	cls, _ := windows.UTF16PtrFromString("TassoPortachiaviProposta")
	inst, _, _ := vtgModule.Call(0)
	cur, _, _ := vtgLoadCursor.Call(0, 32512) // IDC_ARROW
	wc := vtWndClass{lpfnWndProc: windows.NewCallback(vtWndProc), lpszClassName: cls, hInstance: inst, hCursor: cur}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	if r, _, _ := vtgRegClass.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return
	}
	ex := uintptr(0x8 | 0x80 | 0x08000000)                                                                       // TOPMOST | TOOLWINDOW | NOACTIVATE
	h, _, _ := vtgCreateWin.Call(ex, uintptr(unsafe.Pointer(cls)), 0, 0x80000000, 0, 0, 300, 100, 0, 0, inst, 0) // WS_POPUP
	if h == 0 {
		return
	}
	vtgAffinity.Call(h, 0x11)                               // fuori dalle catture dello schermo (come il pannellino)
	vtIcon, _, _ = vtgLoadImage.Call(inst, 2, 1, 64, 64, 0) // icona del tasso (risorsa 2)
	vtPopMu.Lock()
	vtPopHwnd = h
	vtPopMu.Unlock()
	// Cambio di finestra davanti (sempre: serve a "Compila" per sapere l'ultima finestra usata) e
	// cambio di campo (la finestrella). Fuori dal processo di Tasso, senza entrare negli altri programmi.
	vtgWinHook.Call(3, 3, 0, vtFocusCB, 0, 0, 0|2)           // EVENT_SYSTEM_FOREGROUND, OUTOFCONTEXT|SKIPOWNPROCESS
	vtgWinHook.Call(0x8005, 0x8005, 0, vtFocusCB, 0, 0, 0|2) // EVENT_OBJECT_FOCUS
	vtRegistraSulFilo(h)
	var m vtMsgT
	for {
		r, _, _ := vtgGetMsg.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		vtgTransMsg.Call(uintptr(unsafe.Pointer(&m)))
		vtgDispMsg.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func vtWinEvent(_ uintptr, evento uintptr, hwnd uintptr, obj uintptr, _ uintptr, _ uintptr, _ uintptr) uintptr {
	ev, idObj := uint32(evento), int32(uint32(obj))
	switch ev {
	case 3:
		vtSegnaFg(hwnd)
		vtPopMu.Lock()
		top, vis := vtPop.top, vtPop.modo != 0
		vtPopMu.Unlock()
		if vis && vtRoot(hwnd) != top {
			vtNascondi()
		}
	case 0x8005:
		if idObj >= -4 && vtSuggestOn() { // OBJID_WINDOW, CLIENT, ecc.: niente caret o menu
			vtInvia(vtRichiesta{tipo: vrFocus})
		}
	}
	return 0
}

// vtRegistraTasto (ri)registra la scorciatoia scelta. true se riuscito (o se è spenta).
func vtRegistraTasto() bool {
	vtPopMu.Lock()
	h := vtPopHwnd
	vtPopMu.Unlock()
	if h == 0 {
		return true // il filo partirà e la registrerà da solo
	}
	r, _, _ := vtgSend.Call(h, wmVtTasto, 0, 0)
	return r != 0
}

func vtRegistraSulFilo(h uintptr) bool {
	vtgUnregHot.Call(h, vtHotID)
	vtHotOK = false
	setMu.Lock()
	c := settings.VtKey
	setMu.Unlock()
	if c == 0 {
		return true
	}
	r, _, _ := vtgRegHot.Call(h, vtHotID, uintptr(c>>16)|0x4000, uintptr(c&0xFFFF)) // MOD_NOREPEAT
	vtHotOK = r != 0
	return vtHotOK
}

func vtMostra(d vtPopDati) {
	vtPopMu.Lock()
	vtPop = d
	vtHover = -1
	h := vtPopHwnd
	vtPopMu.Unlock()
	if h != 0 {
		vtgPost.Call(h, wmVtMostra, 0, 0)
	}
}

func vtNascondi() {
	vtPopMu.Lock()
	was := vtPop.modo != 0
	vtPop = vtPopDati{}
	h := vtPopHwnd
	vtPopMu.Unlock()
	if h != 0 && was {
		vtgPost.Call(h, wmVtNascondi, 0, 0)
	}
}

// ---------------- disegno ----------------

func vtS(v float64) int32 { return int32(v*vtScale + 0.5) }

type vtLay struct {
	w, h         int32
	head, warn   int32
	rowTop, rowH int32
	rows         int
	foot         int32
	closeL       int32
}

func vtLayout(d vtPopDati) vtLay {
	l := vtLay{w: vtS(320), head: vtS(40), rowH: vtS(48), foot: vtS(32)}
	if d.first && d.modo == vpElenco {
		l.warn = vtS(46)
	}
	switch d.modo {
	case vpElenco:
		l.rows = len(d.voci)
		if l.rows > vtMaxRighe {
			l.rows = vtMaxRighe
		}
	default:
		l.rows = 1
	}
	l.rowTop = l.head + l.warn
	l.h = l.rowTop + int32(l.rows)*l.rowH + l.foot + vtS(4)
	l.closeL = l.w - vtS(34)
	return l
}

func vtFontsPronti() {
	if vtFontSc == vtScale && vtFonts[0] != 0 {
		return
	}
	for i, f := range vtFonts {
		if f != 0 {
			vtgDelObj.Call(f)
			vtFonts[i] = 0
		}
	}
	face, _ := windows.UTF16PtrFromString("Segoe UI")
	mk := func(px float64, peso uintptr) uintptr {
		f, _, _ := vtgCreateFont.Call(uintptr(-vtS(px)), 0, 0, 0, peso, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face))) // CLEARTYPE_QUALITY
		return f
	}
	vtFonts[0] = mk(12.5, 600)
	vtFonts[1] = mk(13.5, 700)
	vtFonts[2] = mk(12, 400)
	vtFonts[3] = mk(14, 800)
	vtFontSc = vtScale
}

func vtTesto(dc uintptr, s string, r rect, font uintptr, col uint32, flags uintptr) {
	if s == "" {
		return
	}
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return
	}
	vtgSelect.Call(dc, font)
	vtgTextColor.Call(dc, uintptr(col))
	vtgDrawText.Call(dc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&r)), flags)
}

const (
	dtRiga   = 0x20 | 0x4 | 0x8000 | 0x800 // SINGLELINE | VCENTER | END_ELLIPSIS | NOPREFIX
	dtCapo   = 0x10 | 0x800 | 0x2000       // WORDBREAK | NOPREFIX | EDITCONTROL
	dtDestra = 0x2
	dtCentro = 0x1
)

func vtRiempiRett(dc uintptr, r rect, col uint32) {
	b, _, _ := vtgBrush.Call(uintptr(col))
	vtgFillRect.Call(dc, uintptr(unsafe.Pointer(&r)), b)
	vtgDelObj.Call(b)
}

func vtDisegna(hwnd, dc uintptr, d vtPopDati, hover int) {
	l := vtLayout(d)
	mdc, _, _ := vtgCompatDC.Call(dc)
	bmp, _, _ := vtgCompatBmp.Call(dc, uintptr(l.w), uintptr(l.h))
	old, _, _ := vtgSelect.Call(mdc, bmp)
	vtFontsPronti()
	vtgBkMode.Call(mdc, 1) // TRANSPARENT
	vtRiempiRett(mdc, rect{0, 0, l.w, l.h}, vcBg)
	// intestazione: tasso piccolo + domanda + ✕
	pad := vtS(12)
	if vtIcon != 0 {
		vtgDrawIcon.Call(mdc, uintptr(pad), uintptr((l.head-vtS(20))/2), vtIcon, uintptr(vtS(20)), uintptr(vtS(20)), 0, 0, 3)
	}
	titolo := trGo("Vuoi usare uno di questi account?")
	switch d.modo {
	case vpChiuso:
		titolo = trGo("Portachiavi di Tasso")
	case vpNessuno:
		titolo = trGo("Nessun account salvato per questo sito")
	}
	if d.modo == vpElenco && d.isPw && d.hint != "" {
		titolo = trGo("Usa la password di questo account?")
	}
	vtTesto(mdc, titolo, rect{pad + vtS(28), 0, l.closeL, l.head}, vtFonts[0], vcText, dtRiga)
	closeCol := uint32(vcMuted)
	if hover == -3 {
		closeCol = vcText
	}
	vtTesto(mdc, "×", rect{l.closeL, 0, l.w - vtS(8), l.head}, vtFonts[2], closeCol, dtRiga|dtCentro)
	vtRiempiRett(mdc, rect{0, l.head - 1, l.w, l.head}, vcLine)
	if l.warn > 0 {
		vtTesto(mdc, trGo("Tasso scriverà utente e password nella finestra che hai davanti: controlla che sia quella giusta."), rect{pad, l.head + vtS(6), l.w - pad, l.head + l.warn}, vtFonts[2], vcA4, dtCapo)
	}
	// righe
	for i := 0; i < l.rows; i++ {
		top := l.rowTop + int32(i)*l.rowH
		r := rect{vtS(6), top + vtS(2), l.w - vtS(6), top + l.rowH - vtS(2)}
		if hover == i {
			vtRiempiRett(mdc, r, vcHover)
		}
		var nome, sotto, iniz string
		accent := uint32(vcA1)
		switch d.modo {
		case vpElenco:
			v := d.voci[i]
			nome, sotto = v.Site, v.User
			if sotto == "" {
				sotto = trGo("(senza nome utente)")
			}
			if v.ID == d.hint {
				sotto += "  ·  " + trGo("appena usato")
			}
		case vpChiuso:
			nome, sotto, accent = trGo("Sblocca il portachiavi"), trGo("Clic qui per aprire Tasso e scrivere il PIN"), vcA4
			iniz = "!"
		case vpNessuno:
			nome, sotto, accent = trGo("Scegli un account dal portachiavi"), d.ctx, vcA3
			iniz = "…"
		}
		if iniz == "" {
			for _, c := range nome {
				iniz = string(c)
				break
			}
			iniz = stringsToUpper(iniz)
		}
		// cerchio con l'iniziale
		cs := vtS(28)
		cx, cy := vtS(14), top+(l.rowH-cs)/2
		b, _, _ := vtgBrush.Call(uintptr(accent))
		pen, _, _ := vtgStockObj.Call(8) // NULL_PEN
		ob, _, _ := vtgSelect.Call(mdc, b)
		op, _, _ := vtgSelect.Call(mdc, pen)
		vtgEllipse.Call(mdc, uintptr(cx), uintptr(cy), uintptr(cx+cs), uintptr(cy+cs))
		vtgSelect.Call(mdc, ob)
		vtgSelect.Call(mdc, op)
		vtgDelObj.Call(b)
		vtTesto(mdc, iniz, rect{cx, cy, cx + cs, cy + cs}, vtFonts[3], vcInk, dtRiga|dtCentro)
		tx := cx + cs + vtS(10)
		vtTesto(mdc, nome, rect{tx, top + vtS(5), l.w - pad, top + l.rowH/2 + vtS(2)}, vtFonts[1], vcText, dtRiga)
		vtTesto(mdc, sotto, rect{tx, top + l.rowH/2, l.w - pad, top + l.rowH - vtS(5)}, vtFonts[2], vcMuted, dtRiga)
	}
	// piede
	ft := l.h - l.foot - vtS(2)
	vtRiempiRett(mdc, rect{0, ft, l.w, ft + 1}, vcLine)
	altri := trGo("Altri account…")
	if d.modo == vpChiuso {
		altri = ""
	}
	col := uint32(vcA3)
	if hover == -2 {
		col = vcText
	}
	vtTesto(mdc, altri, rect{pad, ft, l.w / 2, l.h}, vtFonts[2], col, dtRiga)
	vtTesto(mdc, trGo("Esc per chiudere"), rect{l.w / 2, ft, l.w - pad, l.h}, vtFonts[2], vcMuted, dtRiga|dtDestra)
	// bordo arrotondato
	pen, _, _ := vtgCreatePen.Call(0, 1, vcLine)
	nb, _, _ := vtgStockObj.Call(5) // NULL_BRUSH
	op, _, _ := vtgSelect.Call(mdc, pen)
	ob, _, _ := vtgSelect.Call(mdc, nb)
	vtgRoundRect.Call(mdc, 0, 0, uintptr(l.w), uintptr(l.h), uintptr(vtS(14)), uintptr(vtS(14)))
	vtgSelect.Call(mdc, op)
	vtgSelect.Call(mdc, ob)
	vtgDelObj.Call(pen)
	vtgBitBlt.Call(dc, 0, 0, uintptr(l.w), uintptr(l.h), mdc, 0, 0, 0x00CC0020) // SRCCOPY
	vtgSelect.Call(mdc, old)
	vtgDelObj.Call(bmp)
	vtgDeleteDC.Call(mdc)
}

func stringsToUpper(s string) string {
	r := []rune(s)
	for i, c := range r {
		if c >= 'a' && c <= 'z' {
			r[i] = c - 32
		} else if c >= 'à' && c <= 'þ' && c != '÷' {
			r[i] = c - 32
		}
	}
	return string(r)
}

// vtColpito: cosa c'è sotto il punto (coordinate della finestrella). -1 niente, -2 Altri account, -3 ✕, >=0 riga.
func vtColpito(d vtPopDati, x, y int32) int {
	l := vtLayout(d)
	if y < l.head {
		if x >= l.closeL {
			return -3
		}
		return -1
	}
	if y >= l.rowTop && y < l.rowTop+int32(l.rows)*l.rowH {
		return int((y - l.rowTop) / l.rowH)
	}
	if y >= l.h-l.foot-vtS(2) && x < l.w/2 && d.modo != vpChiuso {
		return -2
	}
	return -1
}

func vtPosiziona(hwnd uintptr) {
	vtPopMu.Lock()
	d := vtPop
	vtPopMu.Unlock()
	if d.modo == 0 {
		return
	}
	// prima sposto la finestrella sul monitor del campo, poi leggo la sua scala (DPI)
	vtgSetPos.Call(hwnd, ^uintptr(0), uintptr(d.anchor.Left), uintptr(d.anchor.Bottom), 0, 0, 0x1|0x10|0x200) // NOSIZE|NOACTIVATE|NOOWNERZORDER
	if dpi, _, _ := vtgDpi.Call(hwnd); dpi >= 72 {
		vtScale = float64(dpi) / 96
	}
	l := vtLayout(d)
	mon, _, _ := vtgMonFromRect.Call(uintptr(unsafe.Pointer(&d.anchor)), 2) // NEAREST
	mi := vtMonInfoT{}
	mi.cbSize = uint32(unsafe.Sizeof(mi))
	vtgMonInfo.Call(mon, uintptr(unsafe.Pointer(&mi)))
	x, y := d.anchor.Left, d.anchor.Bottom+vtS(4)
	if y+l.h > mi.work.Bottom && mi.work.Bottom > 0 {
		y = d.anchor.Top - l.h - vtS(4) // non ci sta sotto: va sopra il campo
	}
	if x+l.w > mi.work.Right && mi.work.Right > 0 {
		x = mi.work.Right - l.w - vtS(4)
	}
	if x < mi.work.Left {
		x = mi.work.Left + vtS(4)
	}
	if y < mi.work.Top {
		y = mi.work.Top + vtS(4)
	}
	rgn, _, _ := vtgRoundRgn.Call(0, 0, uintptr(l.w+1), uintptr(l.h+1), uintptr(vtS(14)), uintptr(vtS(14)))
	vtgSetRgn.Call(hwnd, rgn, 1)
	vtgSetPos.Call(hwnd, ^uintptr(0), uintptr(x), uintptr(y), uintptr(l.w), uintptr(l.h), 0x10|0x40|0x200) // NOACTIVATE|SHOWWINDOW
	vtgInvalidate.Call(hwnd, 0, 0)
	vtMouseGiu = vtPremuto(0x01) || vtPremuto(0x02)
	vtgSetTimer.Call(hwnd, 1, 60, 0)
}

func vtWndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case 0x0021: // WM_MOUSEACTIVATE: il clic non toglie la tastiera al campo
		return 3 // MA_NOACTIVATE
	case 0x000F: // WM_PAINT
		var ps vtPaintT
		dc, _, _ := vtgBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		vtPopMu.Lock()
		d, hv := vtPop, vtHover
		vtPopMu.Unlock()
		if d.modo != 0 {
			vtDisegna(hwnd, dc, d, hv)
		}
		vtgEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case 0x0014: // WM_ERASEBKGND
		return 1
	case wmVtMostra:
		vtPosiziona(hwnd)
		return 0
	case wmVtNascondi:
		vtPopMu.Lock()
		vis := vtPop.modo != 0
		vtPopMu.Unlock()
		if !vis {
			vtgKillTimer.Call(hwnd, 1)
			pShowWin.Call(hwnd, 0)
		}
		return 0
	case wmVtTasto:
		if vtRegistraSulFilo(hwnd) {
			return 1
		}
		return 0
	case 0x0312: // WM_HOTKEY
		if wp == vtHotID {
			vtInvia(vtRichiesta{tipo: vrTasto})
		}
		return 0
	case 0x0020: // WM_SETCURSOR
		vtPopMu.Lock()
		hv := vtHover
		vtPopMu.Unlock()
		id := uintptr(32512)
		if hv != -1 {
			id = 32649 // IDC_HAND
		}
		c, _, _ := vtgLoadCursor.Call(0, id)
		vtgSetCursor.Call(c)
		return 1
	case 0x0200: // WM_MOUSEMOVE
		x, y := int32(int16(lp&0xFFFF)), int32(int16((lp>>16)&0xFFFF))
		vtPopMu.Lock()
		n := vtColpito(vtPop, x, y)
		cambia := n != vtHover
		vtHover = n
		vtPopMu.Unlock()
		if cambia {
			vtgInvalidate.Call(hwnd, 0, 0)
		}
		return 0
	case 0x0202: // WM_LBUTTONUP
		x, y := int32(int16(lp&0xFFFF)), int32(int16((lp>>16)&0xFFFF))
		vtPopMu.Lock()
		d := vtPop
		n := vtColpito(d, x, y)
		vtPopMu.Unlock()
		switch {
		case n == -3:
			vtNascondi()
		case n == -2:
			vtNascondi()
			go vtMostraTasso("onVaultPick", map[string]interface{}{"ctx": d.ctx})
		case n >= 0 && d.modo == vpElenco && n < len(d.voci):
			id := d.voci[n].ID
			vtNascondi()
			vtInvia(vtRichiesta{tipo: vrRiempi, id: id})
		case n >= 0 && d.modo == vpChiuso:
			vtNascondi()
			go vtMostraTasso("onVaultShow", map[string]interface{}{})
		case n >= 0 && d.modo == vpNessuno:
			vtNascondi()
			go vtMostraTasso("onVaultPick", map[string]interface{}{"ctx": d.ctx})
		}
		return 0
	case 0x0113: // WM_TIMER: Esc, clic fuori, finestra cambiata
		vtPopMu.Lock()
		d := vtPop
		vtPopMu.Unlock()
		if d.modo == 0 {
			vtgKillTimer.Call(hwnd, 1)
			pShowWin.Call(hwnd, 0)
			return 0
		}
		giu := vtPremuto(0x01) || vtPremuto(0x02)
		var p point
		vtgCursorPos.Call(uintptr(unsafe.Pointer(&p)))
		var wr rect
		vtgWinRect.Call(hwnd, uintptr(unsafe.Pointer(&wr)))
		dentro := p.X >= wr.Left && p.X < wr.Right && p.Y >= wr.Top && p.Y < wr.Bottom
		if !dentro {
			vtPopMu.Lock()
			if vtHover != -1 {
				vtHover = -1
				vtgInvalidate.Call(hwnd, 0, 0)
			}
			vtPopMu.Unlock()
		}
		chiudi := vtPremuto(0x1B) || (giu && !vtMouseGiu && !dentro) || vtFg() != d.top
		vtMouseGiu = giu
		if chiudi {
			vtNascondi()
		}
		return 0
	}
	r, _, _ := vtgDefProc.Call(hwnd, uintptr(msg), wp, lp)
	return r
}
