package main

// Portachiavi > "Salva questo account": una piccola finestra nello stile della finestrella
// (vetro scuro di Tasso) con nome dell'account (preso dal sito, modificabile), sito (letto dalla
// barra del browser, non modificabile), email o utente (letto dal campo della pagina) e password
// da scrivere una volta, oppure "Crea password sicura". Questa finestra prende la tastiera (serve
// per scrivere la password); vive sullo stesso filo della finestrella (vault_popup.go).
//
// Sicurezza: si apre solo a portachiavi aperto e mai su giochi o finestre di Tasso; la password
// non va mai su disco in chiaro né nei log; le copie in memoria e il testo del campo si azzerano
// alla chiusura; la finestra è esclusa dalle catture dello schermo.

import (
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"tasso/autotipo"
)

const (
	svIDNome   = 101
	svIDSito   = 102
	svIDUtente = 103
	svIDPw     = 104
)

type svDati struct {
	nome, sito, utente string
	web                bool
	top                uintptr
	anchor             rect
}

var (
	svgDestroy   = user32.NewProc("DestroyWindow")
	svgSetText   = user32.NewProc("SetWindowTextW")
	svgGetText   = user32.NewProc("GetWindowTextW")
	svgGetTextL  = user32.NewProc("GetWindowTextLengthW")
	svgSetFocus  = user32.NewProc("SetFocus")
	svgGetFocus  = user32.NewProc("GetFocus")
	svgSetFg     = user32.NewProc("SetForegroundWindow")
	svgBkColor   = gdi32.NewProc("SetBkColor")
	svgMoveWin   = user32.NewProc("MoveWindow")
	svgGetDlgItm = user32.NewProc("GetDlgItem")

	svMu       sync.Mutex
	svProssimo svDati  // dati preparati dal filo di UI Automation, letti da svApri
	svHwnd     uintptr // solo sul filo della finestrella
	svD        svDati
	svGen      []uint16 // password creata con "Crea password sicura" (zero finale)
	svVedi     bool     // password visibile
	svMsg      string
	svHover    int
	svFont     uintptr
	svFontSc   float64
	svFondo    uintptr // pennello dei campi
	svCls      *uint16
	svInst     uintptr
)

// svRegistra registra la classe della finestra (dal filo della finestrella, all'avvio).
func svRegistra(inst uintptr) {
	cls, _ := windows.UTF16PtrFromString("TassoPortachiaviSalva")
	cur, _, _ := vtgLoadCursor.Call(0, 32512)
	wc := vtWndClass{lpfnWndProc: windows.NewCallback(svWndProc), lpszClassName: cls, hInstance: inst, hCursor: cur}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	if r, _, _ := vtgRegClass.Call(uintptr(unsafe.Pointer(&wc))); r != 0 {
		svCls, svInst = cls, inst
	}
}

// svPrepara (filo di UI Automation): chiede alla finestrella di aprire la finestra Salva.
func svPrepara(d svDati) {
	svMu.Lock()
	svProssimo = d
	svMu.Unlock()
	vtPopMu.Lock()
	h := vtPopHwnd
	vtPopMu.Unlock()
	if h != 0 {
		vtgPost.Call(h, wmVtSalva, 0, 0)
	}
}

type svLay struct {
	w, h, head  int32
	box         [4]rect // nome, sito, utente, password
	lab         [4]int32
	occhio      rect
	crea        rect
	msgY        int32
	annulla     rect
	salva       rect
	close       rect
	creaT, salT string
	annT        string
}

func svLayout() svLay {
	vtFontsPronti()
	l := svLay{w: vtS(420), head: vtS(50)}
	l.close = rect{l.w - vtS(42), vtS(10), l.w - vtS(12), vtS(40)}
	y := l.head + vtS(10)
	for i := 0; i < 4; i++ {
		l.lab[i] = y
		l.box[i] = rect{vtS(16), y + vtS(22), l.w - vtS(16), y + vtS(60)}
		y += vtS(70)
	}
	l.occhio = rect{l.box[3].Right - vtS(36), l.box[3].Top + vtS(4), l.box[3].Right - vtS(4), l.box[3].Bottom - vtS(4)}
	l.creaT = trGo("Crea password sicura")
	l.crea = rect{vtS(16), y, vtS(16) + vtMisura(vtFonts[4], l.creaT) + vtS(30), y + vtS(34)}
	l.msgY = l.crea.Bottom + vtS(8)
	by := l.msgY + vtS(40)
	l.salT, l.annT = trGo("Salva nel portachiavi"), trGo("Annulla")
	sw := vtMisura(vtFonts[4], l.salT) + vtS(32)
	aw := vtMisura(vtFonts[4], l.annT) + vtS(28)
	l.salva = rect{l.w - vtS(16) - sw, by, l.w - vtS(16), by + vtS(36)}
	l.annulla = rect{l.salva.Left - vtS(8) - aw, by, l.salva.Left - vtS(8), by + vtS(36)}
	l.h = by + vtS(36) + vtS(16)
	return l
}

// svApri (filo della finestrella): crea e mostra la finestra Salva.
func svApri() {
	svMu.Lock()
	d := svProssimo
	svProssimo = svDati{}
	svMu.Unlock()
	if svCls == nil || d.top == 0 {
		return
	}
	if !vtAperto() {
		go vtMostraTasso("onVaultShow", map[string]interface{}{})
		return
	}
	if svHwnd != 0 {
		svChiudi()
	}
	svD, svMsg, svVedi, svHover = d, "", false, 0
	if svFontSc != vtScale || svFont == 0 {
		if svFont != 0 {
			vtgDelObj.Call(svFont)
		}
		face, _ := windows.UTF16PtrFromString("Segoe UI")
		svFont, _, _ = vtgCreateFont.Call(uintptr(-vtS(14)), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
		svFontSc = vtScale
	}
	if svFondo == 0 {
		svFondo, _, _ = vtgBrush.Call(vcCampo)
	}
	l := svLayout()
	ex := uintptr(0x8 | 0x80 | 0x80000)                                                                                                           // TOPMOST | TOOLWINDOW | LAYERED
	h, _, _ := vtgCreateWin.Call(ex, uintptr(unsafe.Pointer(svCls)), 0, 0x80000000|0x02000000, 0, 0, uintptr(l.w), uintptr(l.h), 0, 0, svInst, 0) // WS_POPUP | WS_CLIPCHILDREN
	if h == 0 {
		return
	}
	svHwnd = h
	vtgLayered.Call(h, 0, 242, 2) // quasi piena: qui si scrive
	vtgAffinity.Call(h, 0x11)     // fuori dalle catture dello schermo
	edit, _ := windows.UTF16PtrFromString("EDIT")
	testi := []string{d.nome, d.sito, d.utente, ""}
	for i := 0; i < 4; i++ {
		st := uintptr(0x40000000 | 0x10000000 | 0x80) // CHILD | VISIBLE | ES_AUTOHSCROLL
		if i != 1 {
			st |= 0x00010000 // WS_TABSTOP
		} else {
			st |= 0x0800 // ES_READONLY: il sito lo legge Tasso, non si cambia
		}
		if i == 3 {
			st |= 0x20 // ES_PASSWORD
		}
		b := l.box[i]
		r := rect{b.Left + vtS(12), b.Top + (b.Bottom-b.Top-vtS(20))/2, b.Right - vtS(12), 0}
		if i == 3 {
			r.Right = l.occhio.Left - vtS(4)
		}
		c, _, _ := vtgCreateWin.Call(0, uintptr(unsafe.Pointer(edit)), 0, st, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(vtS(20)), h, uintptr(svIDNome+i), svInst, 0)
		if c == 0 {
			svChiudi()
			return
		}
		vtgSend.Call(c, 0x0030, svFont, 0) // WM_SETFONT
		vtgSend.Call(c, 0x00C5, 500, 0)    // EM_LIMITTEXT
		if testi[i] != "" {
			t := vtU16(testi[i])
			svgSetText.Call(c, uintptr(unsafe.Pointer(&t[0])))
		}
	}
	// posizione: vicino al campo della pagina, sempre dentro lo schermo
	mon, _, _ := vtgMonFromRect.Call(uintptr(unsafe.Pointer(&d.anchor)), 2)
	mi := vtMonInfoT{}
	mi.cbSize = uint32(unsafe.Sizeof(mi))
	vtgMonInfo.Call(mon, uintptr(unsafe.Pointer(&mi)))
	x, y := vtPosizione(rect{d.anchor.Left - vtS(4), d.anchor.Top - vtS(150), d.anchor.Right, d.anchor.Top - vtS(150)}, mi.work, l.w, l.h, 0, false)
	rgn, _, _ := vtgRoundRgn.Call(0, 0, uintptr(l.w+1), uintptr(l.h+1), uintptr(vtS(16)), uintptr(vtS(16)))
	vtgSetRgn.Call(h, rgn, 1)
	vtgSetPos.Call(h, ^uintptr(0), uintptr(x), uintptr(y), uintptr(l.w), uintptr(l.h), 0x40) // SHOWWINDOW
	svgSetFg.Call(h)
	primo := svIDPw
	if d.utente == "" {
		primo = svIDUtente
	}
	c, _, _ := svgGetDlgItm.Call(h, uintptr(primo))
	svgSetFocus.Call(c)
}

func svCampo(id int) uintptr {
	if svHwnd == 0 {
		return 0
	}
	c, _, _ := svgGetDlgItm.Call(svHwnd, uintptr(id))
	return c
}

func svTesto(id int) string {
	c := svCampo(id)
	n, _, _ := svgGetTextL.Call(c)
	if c == 0 || n == 0 || n > 600 {
		return ""
	}
	b := make([]uint16, n+1)
	svgGetText.Call(c, uintptr(unsafe.Pointer(&b[0])), n+1)
	return strings.TrimSpace(windows.UTF16ToString(b))
}

// svPw legge la password scritta (UTF-16 con lo zero finale): chi la usa la azzera.
func svPw() []uint16 {
	c := svCampo(svIDPw)
	n, _, _ := svgGetTextL.Call(c)
	if c == 0 || n == 0 || n > 500 {
		return nil
	}
	b := make([]uint16, n+1)
	svgGetText.Call(c, uintptr(unsafe.Pointer(&b[0])), n+1)
	return b
}

func svUguali(a, b []uint16) bool {
	if len(a) != len(b) || len(a) == 0 {
		return false
	}
	d := uint16(0)
	for i := range a {
		d |= a[i] ^ b[i]
	}
	return d == 0
}

// svCancellaCampo sovrascrive il testo del campo password prima di toglierlo.
func svCancellaCampo() {
	c := svCampo(svIDPw)
	if c == 0 {
		return
	}
	n, _, _ := svgGetTextL.Call(c)
	if n > 0 && n <= 500 {
		sp := make([]uint16, n+1)
		for i := 0; i < int(n); i++ {
			sp[i] = ' '
		}
		svgSetText.Call(c, uintptr(unsafe.Pointer(&sp[0])))
	}
	z := []uint16{0}
	svgSetText.Call(c, uintptr(unsafe.Pointer(&z[0])))
	vtgSend.Call(c, 0x00CD, 0, 0) // EM_EMPTYUNDOBUFFER
}

func svChiudi() {
	if svHwnd == 0 {
		return
	}
	svCancellaCampo()
	h := svHwnd
	svHwnd = 0
	svgDestroy.Call(h)
	clear(svGen)
	svGen, svMsg, svD = nil, "", svDati{}
}

func svCrea() {
	c := svCampo(svIDPw)
	if c == 0 {
		return
	}
	clear(svGen)
	svGen = autotipo.GeneraPassword(20, true)
	svgSetText.Call(c, uintptr(unsafe.Pointer(&svGen[0])))
	svVedi = true
	vtgSend.Call(c, 0x00CC, 0, 0) // EM_SETPASSWORDCHAR 0: la vedi
	vtgInvalidate.Call(c, 0, 1)
	svMsg = ""
	vtgInvalidate.Call(svHwnd, 0, 0)
}

func svOcchio() {
	c := svCampo(svIDPw)
	svVedi = !svVedi
	ch := uintptr(0x25CF) // ●
	if svVedi {
		ch = 0
	}
	vtgSend.Call(c, 0x00CC, ch, 0)
	vtgInvalidate.Call(c, 0, 1)
	vtgInvalidate.Call(svHwnd, 0, 0)
}

// svControlla: cosa manca per salvare ("" = si può).
func svControlla(nome string, pwLen int) string {
	switch {
	case strings.TrimSpace(nome) == "":
		return "Scrivi un nome per l'account."
	case pwLen == 0:
		return "Scrivi la password o premi «Crea password sicura»."
	}
	return ""
}

func svSalva() {
	nome, utente := svTesto(svIDNome), svTesto(svIDUtente)
	pw := svPw()
	defer clear(pw)
	n := 0
	if len(pw) > 1 {
		n = len(pw) - 1
	}
	if m := svControlla(nome, n); m != "" {
		svMsg = m
		vtgInvalidate.Call(svHwnd, 0, 0)
		return
	}
	if !vtAperto() {
		svMsg = "Il portachiavi si è chiuso: aprilo in Tasso e riprova."
		vtgInvalidate.Call(svHwnd, 0, 0)
		return
	}
	e := vaultEntry{Site: nome, User: utente, Pw: windows.UTF16ToString(pw), Web: svD.sito}
	r := vaultSave(e)
	e.Pw = ""
	if r != "ok" {
		if r == "locked" {
			r = "Il portachiavi si è chiuso: aprilo in Tasso e riprova."
		}
		svMsg = r
		vtgInvalidate.Call(svHwnd, 0, 0)
		return
	}
	creata := svUguali(pw, svGen)
	top := svD.top
	svChiudi()
	if creata { // la password creata qui va anche nel campo del sito
		vtInvia(vtRichiesta{tipo: vrRiempiPw, pw: append([]uint16(nil), pw...), top: top})
	}
	vtFatto(trGo("Account salvato nel portachiavi:") + " " + nome)
}

func svDisegna(hwnd, dc uintptr) {
	l := svLayout()
	mdc, _, _ := vtgCompatDC.Call(dc)
	bmp, _, _ := vtgCompatBmp.Call(dc, uintptr(l.w), uintptr(l.h))
	old, _, _ := vtgSelect.Call(mdc, bmp)
	vtgBkMode.Call(mdc, 1)
	vtRiempiRett(mdc, rect{0, 0, l.w, l.h}, vcBg)
	vtRiempiRett(mdc, rect{0, 0, l.w, vtS(1)}, vcLuce)
	if vtIcon != 0 {
		vtgDrawIcon.Call(mdc, uintptr(vtS(16)), uintptr((l.head-vtS(24))/2), vtIcon, uintptr(vtS(24)), uintptr(vtS(24)), 0, 0, 3)
	}
	vtTesto(mdc, trGo("Salva questo account"), rect{vtS(50), 0, l.close.Left, l.head}, vtFonts[0], vcText, dtRiga)
	cc := uint32(vcMuted)
	if svHover == 1 {
		vtTondo(mdc, l.close, vtS(8), vcHover, vcNiente)
		cc = vcText
	}
	vtTesto(mdc, "", l.close, vtFonts[5], cc, dtRiga|dtCentro)
	vtRiempiRett(mdc, rect{0, l.head, l.w, l.head + 1}, vcLine)
	etichette := []string{trGo("Nome dell'account"), trGo("Sito"), trGo("Email o nome utente"), trGo("Password")}
	if !svD.web {
		etichette[1] = trGo("Programma")
	}
	fuoco, _, _ := svgGetFocus.Call()
	for i := 0; i < 4; i++ {
		vtTesto(mdc, etichette[i], rect{vtS(16), l.lab[i], l.w - vtS(16), l.lab[i] + vtS(20)}, vtFonts[7], vcSotto, dtRiga)
		bordo := uint32(vcBordo)
		if c := svCampo(svIDNome + i); c != 0 && c == fuoco {
			bordo = vcA1
		}
		vtTondo(mdc, l.box[i], vtS(10), vcCampo, bordo)
	}
	nota := trGo("letto dalla barra del browser")
	if !svD.web {
		nota = ""
	}
	vtTesto(mdc, nota, rect{vtS(16), l.lab[1], l.w - vtS(16), l.lab[1] + vtS(20)}, vtFonts[2], vcA1Chiaro, dtRiga|dtDestra)
	oc := uint32(vcMuted)
	if svHover == 2 {
		vtTondo(mdc, l.occhio, vtS(8), vcHover, vcNiente)
		oc = vcText
	}
	occ := "" // occhio
	if svVedi {
		occ = "" // occhio barrato
	}
	vtTesto(mdc, occ, l.occhio, vtFonts[5], oc, dtRiga|dtCentro)
	cf := uint32(vcA1Fondo)
	if svHover == 3 {
		cf = vcA1Bordo
	}
	vtTondo(mdc, l.crea, vtS(10), cf, vcA1Bordo)
	vtTesto(mdc, l.creaT, l.crea, vtFonts[4], vcA1Chiaro, dtRiga|dtCentro)
	msg, col := trGo("La password viene cifrata subito nel portachiavi: Tasso non la scrive mai in chiaro sul disco."), uint32(vcMuted)
	if svMsg != "" {
		msg, col = trGo(svMsg), vcA4
	}
	vtTesto(mdc, msg, rect{vtS(16), l.msgY, l.w - vtS(16), l.msgY + vtS(38)}, vtFonts[2], col, dtCapo)
	ab := uint32(vcBg)
	if svHover == 4 {
		ab = vcHover
	}
	vtTondo(mdc, l.annulla, vtS(10), ab, vcBordo)
	vtTesto(mdc, l.annT, l.annulla, vtFonts[4], vcText, dtRiga|dtCentro)
	sb := uint32(vcA1)
	if svHover == 5 {
		sb = vcA1Chiaro
	}
	vtTondo(mdc, l.salva, vtS(10), sb, vcNiente)
	vtTesto(mdc, l.salT, l.salva, vtFonts[4], vcInk, dtRiga|dtCentro)
	pen, _, _ := vtgCreatePen.Call(0, 1, vcBordoEst)
	nb, _, _ := vtgStockObj.Call(5)
	op, _, _ := vtgSelect.Call(mdc, pen)
	ob, _, _ := vtgSelect.Call(mdc, nb)
	vtgRoundRect.Call(mdc, 0, 0, uintptr(l.w), uintptr(l.h), uintptr(vtS(16)), uintptr(vtS(16)))
	vtgSelect.Call(mdc, op)
	vtgSelect.Call(mdc, ob)
	vtgDelObj.Call(pen)
	vtgBitBlt.Call(dc, 0, 0, uintptr(l.w), uintptr(l.h), mdc, 0, 0, 0x00CC0020)
	vtgSelect.Call(mdc, old)
	vtgDelObj.Call(bmp)
	vtgDeleteDC.Call(mdc)
}

// svColpito: 1 ✕, 2 occhio, 3 Crea password sicura, 4 Annulla, 5 Salva, 0 niente.
func svColpito(x, y int32) int {
	l := svLayout()
	switch {
	case dentroR(l.close, x, y):
		return 1
	case dentroR(l.occhio, x, y):
		return 2
	case dentroR(l.crea, x, y):
		return 3
	case dentroR(l.annulla, x, y):
		return 4
	case dentroR(l.salva, x, y):
		return 5
	}
	return 0
}

func svWndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case 0x000F: // WM_PAINT
		var ps vtPaintT
		dc, _, _ := vtgBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if hwnd == svHwnd {
			svDisegna(hwnd, dc)
		}
		vtgEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case 0x0014: // WM_ERASEBKGND
		return 1
	case 0x0133, 0x0138: // WM_CTLCOLOREDIT, WM_CTLCOLORSTATIC (il sito, di sola lettura)
		col := uint32(vcText)
		if msg == 0x0138 {
			col = vcSotto
		}
		vtgTextColor.Call(wp, uintptr(col))
		svgBkColor.Call(wp, vcCampo)
		return svFondo
	case 0x0111: // WM_COMMAND
		id, code := wp&0xFFFF, (wp>>16)&0xFFFF
		switch {
		case id == 1: // IDOK (Invio)
			svSalva()
		case id == 2: // IDCANCEL (Esc)
			svChiudi()
		case code == 0x0100 || code == 0x0200: // EN_SETFOCUS, EN_KILLFOCUS: bordo verde del campo attivo
			vtgInvalidate.Call(hwnd, 0, 0)
		case code == 0x0300 && id == svIDPw && svMsg != "": // EN_CHANGE
			svMsg = ""
			vtgInvalidate.Call(hwnd, 0, 0)
		}
		return 0
	case 0x0020: // WM_SETCURSOR
		if wp == hwnd {
			id := uintptr(32512)
			if svHover != 0 {
				id = 32649
			}
			c, _, _ := vtgLoadCursor.Call(0, id)
			vtgSetCursor.Call(c)
			return 1
		}
	case 0x0200: // WM_MOUSEMOVE
		x, y := int32(int16(lp&0xFFFF)), int32(int16((lp>>16)&0xFFFF))
		if n := svColpito(x, y); n != svHover {
			svHover = n
			vtgInvalidate.Call(hwnd, 0, 0)
		}
		return 0
	case 0x0202: // WM_LBUTTONUP
		x, y := int32(int16(lp&0xFFFF)), int32(int16((lp>>16)&0xFFFF))
		switch svColpito(x, y) {
		case 1, 4:
			svChiudi()
		case 2:
			svOcchio()
		case 3:
			svCrea()
		case 5:
			svSalva()
		}
		return 0
	case 0x0002: // WM_DESTROY
		if hwnd == svHwnd {
			svHwnd = 0
		}
	}
	r, _, _ := vtgDefProc.Call(hwnd, uintptr(msg), wp, lp)
	return r
}
