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

	"tasso/autotipo"
)

const (
	vpElenco  = 1 // account del sito
	vpChiuso  = 2 // portachiavi chiuso: "Sblocca il portachiavi"
	vpNessuno = 3 // nessun account per il sito: "Salva questo account"
	vpGenera  = 4 // campo password di una registrazione: "Crea password sicura e salvala"

	wmVtMostra   = 0x0400 + 41
	wmVtNascondi = 0x0400 + 42
	wmVtTasto    = 0x0400 + 43
	wmVtSposta   = 0x0400 + 44 // ricalcola la posizione (es. sotto è comparsa un'altra tendina)
	wmVtSalva    = 0x0400 + 45 // apre la finestra "Salva questo account"
	vtHotID      = 7
	vtMaxRighe   = 5
)

type vtPopDati struct {
	modo           int
	voci           []vtVoce
	anchor         rect
	top            uintptr
	ctx            string
	hint           string
	first          bool
	isPw           bool
	web            bool     // ctx è un sito (letto dalla barra del browser)
	sopra          bool     // sotto il campo c'è già un'altra tendina: va sopra
	utente         string   // email o utente già scritto nel campo (mai la password)
	gen            []uint16 // password generata (vpGenera), con lo zero finale: si azzera quando si chiude
	simboli, corta bool
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
	vtgLayered     = user32.NewProc("SetLayeredWindowAttributes")
	vtgGetDC       = user32.NewProc("GetDC")
	vtgRelDC       = user32.NewProc("ReleaseDC")
	vtgTextExt     = gdi32.NewProc("GetTextExtentPoint32W")
	vtgIsDlgMsg    = user32.NewProc("IsDialogMessageW")

	vtPopMu    sync.Mutex
	vtPop      vtPopDati
	vtPopHwnd  uintptr
	vtHover    = -1
	vtScale    = 1.0
	vtFonts    [8]uintptr // titolo, nome, piccolo, iniziale, pulsante, icone, password, piccolo grassetto
	vtFontSc   float64
	vtIcon     uintptr
	vtMouseGiu bool
	vtFocusCB  = windows.NewCallback(vtWinEvent)
	vtHotOK    bool
)

// Colori di Tasso (COLORREF = 0x00BBGGRR). Con la trasparenza (vtAlpha) e una pagina bianca dietro
// testo e grigi restano sopra 4,5:1 di contrasto (vedi TestVtContrasto).
const (
	vcBg       = 0x241714 // vetro scuro #141724
	vcLine     = 0x422F2A // #2A2F42
	vcHover    = 0x382823 // #232838
	vcText     = 0xF8F4F2 // --text  #F2F4F8
	vcMuted    = 0xD8C8C3 // #C3C8D8 (più chiaro di --muted: sul vetro serve contrasto)
	vcSotto    = 0xD6C5BF // #BFC5D6
	vcA1       = 0xA6F32F // --a1    #2FF3A6
	vcA1Fondo  = 0x363618 // a1 al 14% sul vetro
	vcA1Bordo  = 0x4B591C // a1 al 30% sul vetro
	vcA1Chiaro = 0xC9F87F // #7FF8C9
	vcInk      = 0x0D1404 // --ink   #04140D
	vcA2       = 0xFF7BA7 // --a2    #A77BFF
	vcA3       = 0xFFA84D // --a3    #4DA8FF
	vcA4       = 0x47B5FF // --a4    #FFB547
	vcLink     = 0xFFC68C // #8CC6FF
	vcCampo    = 0x19100E // #0E1019
	vcBordo    = 0x46332E // #2E3346
	vcBordoEst = 0x4A3B38 // #383B4A
	vcLuce     = 0x4F3F3B // riflesso in alto
	vcNiente   = 0xFFFFFFFF
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
	ex := uintptr(0x8 | 0x80 | 0x08000000 | 0x80000)                                                             // TOPMOST | TOOLWINDOW | NOACTIVATE | LAYERED
	h, _, _ := vtgCreateWin.Call(ex, uintptr(unsafe.Pointer(cls)), 0, 0x80000000, 0, 0, 300, 100, 0, 0, inst, 0) // WS_POPUP
	if h == 0 {
		return
	}
	vtgLayered.Call(h, 0, vtAlpha, 2) // LWA_ALPHA: vetro un po' trasparente
	svRegistra(inst)
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
		if sv := svHwnd; sv != 0 {
			if r, _, _ := vtgIsDlgMsg.Call(sv, uintptr(unsafe.Pointer(&m))); r != 0 {
				continue // Tab, Invio ed Esc nella finestra "Salva questo account"
			}
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
	clear(vtPop.gen)
	vtPop = d
	vtHover = -1
	h := vtPopHwnd
	vtPopMu.Unlock()
	if h != 0 {
		vtgPost.Call(h, wmVtMostra, 0, 0)
	}
}

// vtSpostaSopra: sotto il campo è comparsa un'altra tendina: la finestrella va sopra il campo.
func vtSpostaSopra() {
	vtPopMu.Lock()
	vis := vtPop.modo != 0
	vtPop.sopra = vis
	h := vtPopHwnd
	vtPopMu.Unlock()
	if h != 0 && vis {
		vtgPost.Call(h, wmVtSposta, 0, 0)
	}
}

func vtNascondi() {
	vtPopMu.Lock()
	was := vtPop.modo != 0
	clear(vtPop.gen) // la password generata e non usata non resta in memoria
	vtPop = vtPopDati{}
	h := vtPopHwnd
	vtPopMu.Unlock()
	if h != 0 && was {
		vtgPost.Call(h, wmVtNascondi, 0, 0)
	}
}

// ---------------- disegno ----------------
//
// Stile Tasso "vetro scuro": la finestra è un po' trasparente (WS_EX_LAYERED con alpha uguale per
// tutta la finestra, così il testo resta nitido); i colori sono scelti perché testo e grigi restino
// sopra 4,5:1 di contrasto anche con una pagina bianca dietro.

func vtS(v float64) int32 { return int32(v*vtScale + 0.5) }

const vtAlpha = 226 // trasparenza della finestrella (255 = piena)

type vtLay struct {
	w, h             int32
	head, siteY      int32
	sep              int32 // riga sotto il sito
	warnY, warn      int32
	rowTop, rowH     int32
	rows             int
	bodyY, bodyH     int32
	genBox, regen    rect
	strY             int32
	chipL, chipS     rect
	chipC            rect
	noteY            int32
	foot             int32
	btn1, btn2, esc  rect
	close            rect
	btn1T, btn2T     string
	btn1Pri, conSito bool
}

// vtMisura: larghezza del testo con il carattere dato (pixel).
func vtMisura(font uintptr, s string) int32 {
	u, err := windows.UTF16FromString(s)
	if err != nil || len(u) < 2 {
		return 0
	}
	dc, _, _ := vtgGetDC.Call(0)
	if dc == 0 {
		return int32(len(u)) * vtS(7)
	}
	old, _, _ := vtgSelect.Call(dc, font)
	var sz struct{ cx, cy int32 }
	vtgTextExt.Call(dc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&sz)))
	vtgSelect.Call(dc, old)
	vtgRelDC.Call(0, dc)
	return sz.cx
}

func vtLayout(d vtPopDati) vtLay {
	vtFontsPronti()
	l := vtLay{w: vtS(368), head: vtS(48), rowH: vtS(52)}
	l.close = rect{l.w - vtS(42), vtS(10), l.w - vtS(12), vtS(40)}
	y := l.head
	l.conSito = d.modo != vpChiuso
	if l.conSito {
		l.siteY = y
		y += vtS(26)
	}
	l.sep = y
	y += vtS(6)
	if d.first && d.modo == vpElenco {
		l.warnY, l.warn = y, vtS(40)
		y += l.warn
	}
	switch d.modo {
	case vpElenco:
		l.rows = len(d.voci)
		if l.rows > vtMaxRighe {
			l.rows = vtMaxRighe
		}
	case vpChiuso:
		l.rows = 1
	}
	l.rowTop = y
	y += int32(l.rows) * l.rowH
	switch d.modo {
	case vpNessuno:
		l.bodyY, l.bodyH = y+vtS(4), vtS(44)
		y = l.bodyY + l.bodyH
	case vpGenera:
		l.genBox = rect{vtS(14), y + vtS(6), l.w - vtS(14), y + vtS(54)}
		l.regen = rect{l.genBox.Right - vtS(40), l.genBox.Top + vtS(8), l.genBox.Right - vtS(8), l.genBox.Bottom - vtS(8)}
		l.strY = l.genBox.Bottom + vtS(8)
		cy := l.strY + vtS(26)
		x := vtS(14)
		mk := func(t string) rect {
			r := rect{x, cy, x + vtMisura(vtFonts[2], t) + vtS(20), cy + vtS(26)}
			x = r.Right + vtS(8)
			return r
		}
		l.chipL = mk(trGo("Lettere e numeri"))
		l.chipS = mk(trGo("Simboli"))
		l.chipC = mk(trGo("Più corta"))
		l.noteY = cy + vtS(34)
		y = l.noteY + vtS(38)
	}
	y += vtS(4)
	l.foot = y
	by := y + vtS(10)
	switch d.modo {
	case vpElenco, vpNessuno:
		l.btn1T, l.btn2T = "+  "+trGo("Salva questo account"), trGo("Altri account…")
		l.btn1Pri = d.modo == vpNessuno
	case vpGenera:
		l.btn1T, l.btn2T, l.btn1Pri = trGo("Usa e salva"), trGo("Scrivo io la mia"), true
	}
	x := vtS(14)
	if l.btn1T != "" {
		l.btn1 = rect{x, by, x + vtMisura(vtFonts[4], l.btn1T) + vtS(30), by + vtS(36)}
		x = l.btn1.Right + vtS(6)
	}
	if l.btn2T != "" {
		l.btn2 = rect{x, by, x + vtMisura(vtFonts[4], l.btn2T) + vtS(20), by + vtS(36)}
	}
	escT := "Esc"
	if d.modo == vpChiuso {
		escT = trGo("Esc per chiudere")
	}
	l.esc = rect{l.w - vtS(14) - vtMisura(vtFonts[2], escT), by, l.w - vtS(14), by + vtS(36)}
	if l.btn2.Right > l.esc.Left-vtS(4) && l.btn2T != "" { // lingue con testi lunghi: niente "Esc"
		l.esc = rect{}
	}
	l.h = by + vtS(36) + vtS(12)
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
	mk := func(nome string, px float64, peso uintptr) uintptr {
		face, _ := windows.UTF16PtrFromString(nome)
		f, _, _ := vtgCreateFont.Call(uintptr(-vtS(px)), 0, 0, 0, peso, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face))) // CLEARTYPE_QUALITY
		return f
	}
	vtFonts[0] = mk("Segoe UI Semibold", 14.5, 600)
	vtFonts[1] = mk("Segoe UI", 14.5, 700)
	vtFonts[2] = mk("Segoe UI", 12.5, 400)
	vtFonts[3] = mk("Segoe UI", 15, 800)
	vtFonts[4] = mk("Segoe UI Semibold", 13, 600)
	vtFonts[5] = mk("Segoe MDL2 Assets", 11, 400) // icone (✕ ✓ ⟳ occhio): ci sono da Windows 10
	vtFonts[6] = mk("Consolas", 17, 700)
	vtFonts[7] = mk("Segoe UI Semibold", 12.5, 600)
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
	vtTestoU16(dc, u, r, font, col, flags)
}

// vtTestoU16 scrive testo UTF-16 con lo zero finale (anche la password generata, senza farne una string).
func vtTestoU16(dc uintptr, u []uint16, r rect, font uintptr, col uint32, flags uintptr) {
	n := 0
	for n < len(u) && u[n] != 0 {
		n++
	}
	if n == 0 {
		return
	}
	vtgSelect.Call(dc, font)
	vtgTextColor.Call(dc, uintptr(col))
	vtgDrawText.Call(dc, uintptr(unsafe.Pointer(&u[0])), uintptr(n), uintptr(unsafe.Pointer(&r)), flags)
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

// vtTondo: rettangolo arrotondato pieno, con il bordo (bordo = vcNiente: senza).
func vtTondo(dc uintptr, r rect, raggio int32, pieno, bordo uint32) {
	b, _, _ := vtgBrush.Call(uintptr(pieno))
	var pen uintptr
	if bordo == vcNiente {
		pen, _, _ = vtgStockObj.Call(8) // NULL_PEN
	} else {
		pen, _, _ = vtgCreatePen.Call(0, 1, uintptr(bordo))
	}
	ob, _, _ := vtgSelect.Call(dc, b)
	op, _, _ := vtgSelect.Call(dc, pen)
	vtgRoundRect.Call(dc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom), uintptr(raggio), uintptr(raggio))
	vtgSelect.Call(dc, ob)
	vtgSelect.Call(dc, op)
	vtgDelObj.Call(b)
	if bordo != vcNiente {
		vtgDelObj.Call(pen)
	}
}

// vtForza: quanto è forte la password generata, in parole.
func vtForza(n int) string {
	if n >= 18 {
		return trGo("Fortissima") + " · " + itoa(n) + " " + trGo("caratteri")
	}
	return trGo("Molto forte") + " · " + itoa(n) + " " + trGo("caratteri")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func vtLungGen(d vtPopDati) int {
	if d.corta {
		return 14
	}
	return 20
}

func vtDisegna(hwnd, dc uintptr, d vtPopDati, hover int) {
	l := vtLayout(d)
	mdc, _, _ := vtgCompatDC.Call(dc)
	bmp, _, _ := vtgCompatBmp.Call(dc, uintptr(l.w), uintptr(l.h))
	old, _, _ := vtgSelect.Call(mdc, bmp)
	vtgBkMode.Call(mdc, 1) // TRANSPARENT
	vtRiempiRett(mdc, rect{0, 0, l.w, l.h}, vcBg)
	vtRiempiRett(mdc, rect{0, 0, l.w, vtS(1)}, vcLuce) // riflesso in alto, come il vetro
	pad := vtS(14)
	// intestazione: tasso + domanda + ✕
	if vtIcon != 0 {
		vtgDrawIcon.Call(mdc, uintptr(pad), uintptr((l.head-vtS(24))/2), vtIcon, uintptr(vtS(24)), uintptr(vtS(24)), 0, 0, 3)
	}
	titolo := trGo("Vuoi usare uno di questi account?")
	switch d.modo {
	case vpChiuso:
		titolo = trGo("Portachiavi di Tasso")
	case vpNessuno:
		titolo = trGo("Nessun account salvato qui")
	case vpGenera:
		titolo = trGo("Crea password sicura e salvala")
	}
	if d.modo == vpElenco && d.isPw && d.hint != "" {
		titolo = trGo("Usa la password di questo account?")
	}
	vtTesto(mdc, titolo, rect{pad + vtS(34), 0, l.close.Left - vtS(4), l.head}, vtFonts[0], vcText, dtRiga)
	closeCol := uint32(vcMuted)
	if hover == -3 {
		vtTondo(mdc, l.close, vtS(8), vcHover, vcNiente)
		closeCol = vcText
	}
	vtTesto(mdc, "", l.close, vtFonts[5], closeCol, dtRiga|dtCentro)
	// sito riconosciuto (dominio letto dalla barra del browser)
	if l.conSito {
		cy := l.siteY + (vtS(26)-vtS(18))/2
		chk := rect{pad, cy, pad + vtS(18), cy + vtS(18)}
		vtTondo(mdc, chk, vtS(6), vcA1Fondo, vcNiente)
		vtTesto(mdc, "", chk, vtFonts[5], vcA1, dtRiga|dtCentro)
		et := trGo("Sito riconosciuto:")
		switch {
		case !d.web:
			et = trGo("Programma:")
		case d.modo == vpNessuno:
			et = trGo("Sito:")
		case d.modo == vpGenera:
			et = trGo("Nuovo account su")
		}
		x := chk.Right + vtS(8)
		vtTesto(mdc, et, rect{x, l.siteY, l.w - pad, l.siteY + vtS(26)}, vtFonts[2], vcMuted, dtRiga)
		x += vtMisura(vtFonts[2], et) + vtS(5)
		vtTesto(mdc, d.ctx, rect{x, l.siteY, l.w - pad, l.siteY + vtS(26)}, vtFonts[7], vcText, dtRiga)
	}
	vtRiempiRett(mdc, rect{0, l.sep, l.w, l.sep + 1}, vcLine)
	if l.warn > 0 {
		vtTesto(mdc, trGo("Tasso scriverà utente e password nella finestra che hai davanti: controlla che sia quella giusta."), rect{pad, l.warnY, l.w - pad, l.warnY + l.warn}, vtFonts[2], vcA4, dtCapo)
	}
	// righe degli account
	colori := []uint32{vcA1, vcA3, vcA2, vcA4}
	for i := 0; i < l.rows; i++ {
		top := l.rowTop + int32(i)*l.rowH
		r := rect{vtS(6), top + vtS(2), l.w - vtS(6), top + l.rowH - vtS(2)}
		if hover == i {
			vtTondo(mdc, r, vtS(11), vcHover, vcNiente)
			vtTondo(mdc, rect{r.Left, r.Top + vtS(12), r.Left + vtS(3), r.Bottom - vtS(12)}, vtS(3), vcA1, vcNiente)
		}
		var nome, sotto, iniz string
		accent := colori[i%len(colori)]
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
		}
		if iniz == "" {
			for _, c := range nome {
				iniz = string(c)
				break
			}
			iniz = stringsToUpper(iniz)
		}
		cs := vtS(34)
		cx, cy := vtS(16), top+(l.rowH-cs)/2
		vtTondo(mdc, rect{cx, cy, cx + cs, cy + cs}, vtS(10), accent, vcNiente)
		vtTesto(mdc, iniz, rect{cx, cy, cx + cs, cy + cs}, vtFonts[3], vcInk, dtRiga|dtCentro)
		tx := cx + cs + vtS(12)
		vtTesto(mdc, nome, rect{tx, top + vtS(6), l.w - pad, top + l.rowH/2 + vtS(2)}, vtFonts[1], vcText, dtRiga)
		vtTesto(mdc, sotto, rect{tx, top + l.rowH/2, l.w - pad, top + l.rowH - vtS(6)}, vtFonts[2], vcSotto, dtRiga)
	}
	// corpo: nessun account / password generata
	switch d.modo {
	case vpNessuno:
		msg := trGo("Vuoi che Tasso si ricordi l'account che usi qui?")
		if d.utente != "" {
			msg = trGo("Hai scritto") + " " + d.utente + ". " + trGo("Vuoi che Tasso si ricordi questo account?")
		}
		vtTesto(mdc, msg, rect{vtS(16), l.bodyY, l.w - vtS(16), l.bodyY + l.bodyH}, vtFonts[2], vcSotto, dtCapo)
	case vpGenera:
		vtTondo(mdc, l.genBox, vtS(12), vcCampo, vcBordo)
		vtTestoU16(mdc, d.gen, rect{l.genBox.Left + vtS(14), l.genBox.Top, l.regen.Left - vtS(4), l.genBox.Bottom}, vtFonts[6], vcText, dtRiga)
		rc := uint32(vcMuted)
		if hover == -7 {
			vtTondo(mdc, l.regen, vtS(8), vcHover, vcNiente)
			rc = vcText
		}
		vtTesto(mdc, "", l.regen, vtFonts[5], rc, dtRiga|dtCentro)
		f := vtForza(vtLungGen(d))
		vtTesto(mdc, f, rect{pad, l.strY, l.w - pad, l.strY + vtS(18)}, vtFonts[2], vcSotto, dtRiga)
		bx := pad + vtMisura(vtFonts[2], f) + vtS(10)
		by := l.strY + vtS(7)
		if bx < l.w-pad-vtS(30) {
			vtTondo(mdc, rect{bx, by, l.w - pad, by + vtS(5)}, vtS(5), vcLine, vcNiente)
			pieno := l.w - pad
			if d.corta {
				pieno = bx + (l.w-pad-bx)*4/5
			}
			vtTondo(mdc, rect{bx, by, (bx + pieno) / 2, by + vtS(5)}, vtS(5), vcA1, vcNiente)
			vtTondo(mdc, rect{(bx + pieno) / 2, by, pieno, by + vtS(5)}, vtS(5), vcA3, vcNiente)
		}
		chip := func(r rect, t string, on bool, hv bool) {
			fondo, bordo, col := uint32(vcBg), uint32(vcBordo), uint32(vcSotto)
			if on {
				fondo, bordo, col = vcA1Fondo, vcA1Bordo, vcA1Chiaro
			}
			if hv {
				fondo = vcHover
			}
			vtTondo(mdc, r, r.Bottom-r.Top, fondo, bordo)
			vtTesto(mdc, t, r, vtFonts[2], col, dtRiga|dtCentro)
		}
		chip(l.chipL, trGo("Lettere e numeri"), true, false)
		chip(l.chipS, trGo("Simboli"), d.simboli, hover == -8)
		chip(l.chipC, trGo("Più corta"), d.corta, hover == -9)
		vtTesto(mdc, trGo("Tasso la scrive nei campi password di questa pagina e la salva nel portachiavi per")+" "+d.ctx+".", rect{vtS(16), l.noteY, l.w - vtS(16), l.noteY + vtS(38)}, vtFonts[2], vcMuted, dtCapo)
	}
	// piede: pulsanti
	if d.modo == vpElenco {
		vtRiempiRett(mdc, rect{0, l.foot, l.w, l.foot + 1}, vcLine)
	}
	if l.btn1T != "" {
		hv := hover == -4 || hover == -5
		switch {
		case l.btn1Pri:
			c := uint32(vcA1)
			if hv {
				c = vcA1Chiaro
			}
			vtTondo(mdc, l.btn1, vtS(10), c, vcNiente)
			vtTesto(mdc, l.btn1T, l.btn1, vtFonts[4], vcInk, dtRiga|dtCentro)
		default:
			c := uint32(vcA1Fondo)
			if hv {
				c = vcA1Bordo
			}
			vtTondo(mdc, l.btn1, vtS(10), c, vcA1Bordo)
			vtTesto(mdc, l.btn1T, l.btn1, vtFonts[4], vcA1Chiaro, dtRiga|dtCentro)
		}
	}
	if l.btn2T != "" {
		c := uint32(vcLink)
		if hover == -2 || hover == -6 {
			c = vcText
		}
		vtTesto(mdc, l.btn2T, l.btn2, vtFonts[4], c, dtRiga|dtCentro)
	}
	if l.esc.Right > 0 {
		escT := "Esc"
		if d.modo == vpChiuso {
			escT = trGo("Esc per chiudere")
		}
		vtTesto(mdc, escT, l.esc, vtFonts[2], vcMuted, dtRiga|dtDestra)
	}
	// bordo arrotondato
	pen, _, _ := vtgCreatePen.Call(0, 1, vcBordoEst)
	nb, _, _ := vtgStockObj.Call(5) // NULL_BRUSH
	op, _, _ := vtgSelect.Call(mdc, pen)
	ob, _, _ := vtgSelect.Call(mdc, nb)
	vtgRoundRect.Call(mdc, 0, 0, uintptr(l.w), uintptr(l.h), uintptr(vtS(16)), uintptr(vtS(16)))
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

func dentroR(r rect, x, y int32) bool {
	return r.Right > r.Left && x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom
}

// vtColpito: cosa c'è sotto il punto (coordinate della finestrella).
// -1 niente, -2 Altri account, -3 ✕, -4 Salva questo account, -5 Usa e salva, -6 Scrivo io la mia,
// -7 rigenera, -8 Simboli, -9 Più corta, >=0 riga.
func vtColpito(d vtPopDati, x, y int32) int {
	l := vtLayout(d)
	switch {
	case dentroR(l.close, x, y):
		return -3
	case y >= l.rowTop && y < l.rowTop+int32(l.rows)*l.rowH:
		return int((y - l.rowTop) / l.rowH)
	}
	switch d.modo {
	case vpElenco, vpNessuno:
		if dentroR(l.btn1, x, y) {
			return -4
		}
		if dentroR(l.btn2, x, y) {
			return -2
		}
	case vpGenera:
		switch {
		case dentroR(l.btn1, x, y):
			return -5
		case dentroR(l.btn2, x, y):
			return -6
		case dentroR(l.regen, x, y):
			return -7
		case dentroR(l.chipS, x, y):
			return -8
		case dentroR(l.chipC, x, y):
			return -9
		}
	}
	return -1
}

// vtPosizione: dove mettere la finestrella (l = larghezza e altezza già in pixel). Allineata a
// sinistra del campo, sotto; sopra se sotto c'è già la tendina di un altro gestore di password
// (sopra = true) o se sotto non ci sta. Sempre dentro lo schermo.
func vtPosizione(campo, schermo rect, w, h, margine int32, sopra bool) (int32, int32) {
	x := campo.Left
	sotto := campo.Bottom + margine
	su := campo.Top - h - margine
	y := sotto
	entraSu := su >= schermo.Top
	entraGiu := sotto+h <= schermo.Bottom || schermo.Bottom == 0
	if (sopra && entraSu) || (!entraGiu && entraSu) {
		y = su
	}
	if x+w > schermo.Right && schermo.Right > 0 {
		x = schermo.Right - w - margine
	}
	if x < schermo.Left {
		x = schermo.Left + margine
	}
	if y+h > schermo.Bottom && schermo.Bottom > 0 {
		y = schermo.Bottom - h - margine
	}
	if y < schermo.Top {
		y = schermo.Top + margine
	}
	return x, y
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
	x, y := vtPosizione(d.anchor, mi.work, l.w, l.h, vtS(6), d.sopra)
	rgn, _, _ := vtgRoundRgn.Call(0, 0, uintptr(l.w+1), uintptr(l.h+1), uintptr(vtS(16)), uintptr(vtS(16)))
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
	case wmVtMostra, wmVtSposta:
		vtPosiziona(hwnd)
		return 0
	case wmVtSalva:
		svApri()
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
		var pw []uint16
		if n == -5 && len(d.gen) > 1 && d.gen[0] != 0 {
			// copia fatta col lucchetto: il filo di UI Automation può azzerare vtPop.gen
			// (vtMostra/vtNascondi) appena lo lascio, e "Usa e salva" scriverebbe una password vuota
			pw = append([]uint16(nil), d.gen...) // la copia la azzera chi la usa
		}
		vtPopMu.Unlock()
		switch {
		case n == -3, n == -6:
			vtNascondi()
		case n == -2:
			vtNascondi()
			go vtMostraTasso("onVaultPick", map[string]interface{}{"ctx": d.ctx})
		case n == -4:
			vtNascondi()
			vtInvia(vtRichiesta{tipo: vrSalva})
		case n == -5 && pw != nil:
			vtNascondi()
			vtInvia(vtRichiesta{tipo: vrGenera, pw: pw})
		case n == -7 || n == -8 || n == -9:
			vtPopMu.Lock()
			if vtPop.modo == vpGenera {
				switch n {
				case -8:
					vtPop.simboli = !vtPop.simboli
				case -9:
					vtPop.corta = !vtPop.corta
				}
				clear(vtPop.gen)
				vtPop.gen = autotipo.GeneraPassword(vtLungGen(vtPop), vtPop.simboli)
			}
			vtPopMu.Unlock()
			vtgInvalidate.Call(hwnd, 0, 0)
		case n >= 0 && d.modo == vpElenco && n < len(d.voci):
			id := d.voci[n].ID
			vtNascondi()
			vtInvia(vtRichiesta{tipo: vrRiempi, id: id})
		case n >= 0 && d.modo == vpChiuso:
			vtNascondi()
			go vtMostraTasso("onVaultShow", map[string]interface{}{})
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
