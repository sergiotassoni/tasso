//go:build windows

package traduce

// Cattura dello schermo, finestrella della traduzione sopra il gioco, scelta della zona
// e tasti rapidi. Tutto con finestre normali di Windows: niente entra dentro il gioco,
// quindi gli anti-cheat non hanno niente da controllare.

import (
	"image"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")

	pRegisterClassEx  = user32.NewProc("RegisterClassExW")
	pCreateWindowEx   = user32.NewProc("CreateWindowExW")
	pDefWindowProc    = user32.NewProc("DefWindowProcW")
	pGetMessage       = user32.NewProc("GetMessageW")
	pTranslateMsg     = user32.NewProc("TranslateMessage")
	pDispatchMsg      = user32.NewProc("DispatchMessageW")
	pPostMessage      = user32.NewProc("PostMessageW")
	pSetLayeredAttr   = user32.NewProc("SetLayeredWindowAttributes")
	pShowWindow       = user32.NewProc("ShowWindow")
	pSetWindowPos     = user32.NewProc("SetWindowPos")
	pGetWindowLongPtr = user32.NewProc("GetWindowLongPtrW")
	pSetWindowLongPtr = user32.NewProc("SetWindowLongPtrW")
	pInvalidateRect   = user32.NewProc("InvalidateRect")
	pBeginPaint       = user32.NewProc("BeginPaint")
	pEndPaint         = user32.NewProc("EndPaint")
	pFillRect         = user32.NewProc("FillRect")
	pFrameRect        = user32.NewProc("FrameRect")
	pDrawText         = user32.NewProc("DrawTextW")
	pGetSysMetrics    = user32.NewProc("GetSystemMetrics")
	pRegisterHotKey   = user32.NewProc("RegisterHotKey")
	pGetDC            = user32.NewProc("GetDC")
	pReleaseDC        = user32.NewProc("ReleaseDC")
	pSetCapture       = user32.NewProc("SetCapture")
	pReleaseCapture   = user32.NewProc("ReleaseCapture")
	pDestroyWindow    = user32.NewProc("DestroyWindow")
	pSetForeground    = user32.NewProc("SetForegroundWindow")
	pLoadCursor       = user32.NewProc("LoadCursorW")
	pSetDisplayAffin  = user32.NewProc("SetWindowDisplayAffinity")
	pGetClientRect    = user32.NewProc("GetClientRect")
	pGetForeground    = user32.NewProc("GetForegroundWindow")
	pSetTimer         = user32.NewProc("SetTimer")
	pClientToScreen   = user32.NewProc("ClientToScreen")
	pIsWindow         = user32.NewProc("IsWindow")

	pCreateCompatDC    = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatBmp   = gdi32.NewProc("CreateCompatibleBitmap")
	pSelectObject      = gdi32.NewProc("SelectObject")
	pBitBlt            = gdi32.NewProc("BitBlt")
	pGetDIBits         = gdi32.NewProc("GetDIBits")
	pSetDIBits         = gdi32.NewProc("SetDIBits")
	pSetDIBitsToDevice = gdi32.NewProc("SetDIBitsToDevice")
	pDeleteObject      = gdi32.NewProc("DeleteObject")
	pDeleteDC          = gdi32.NewProc("DeleteDC")
	pCreateFont        = gdi32.NewProc("CreateFontW")
	pGetTextMetrics    = gdi32.NewProc("GetTextMetricsW")
	pCreateBrush       = gdi32.NewProc("CreateSolidBrush")
	pSetBkMode         = gdi32.NewProc("SetBkMode")
	pSetTextColor      = gdi32.NewProc("SetTextColor")
)

type rect struct{ Left, Top, Right, Bottom int32 }

type paintStruct struct {
	hdc         uintptr
	fErase      int32
	rcPaint     rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

type msgW struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	ptX     int32
	ptY     int32
	private uint32
}

type wndClassEx struct {
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

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

const (
	wmPaint       = 0x000F
	wmHotkey      = 0x0312
	wmKeyDown     = 0x0100
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	wmMouseMove   = 0x0200
	wmRButtonDown = 0x0204
	wmApp         = 0x8000
	wmAggiorna    = wmApp + 1 // ridisegna la finestrella
	wmSeleziona   = wmApp + 2 // apri la scelta della zona
	wmMostra      = wmApp + 3 // mostra/nascondi la finestrella
	wmTasti       = wmApp + 4 // registra di nuovo i tasti rapidi

	smXVirtual  = 76
	smYVirtual  = 77
	smCXVirtual = 78
	smCYVirtual = 79

	swpNoActivate = 0x10
)

// ---------------------------------------------------------------- cattura

// cattura copia un rettangolo dello schermo (coordinate del desktop virtuale).
var (
	muClassica     sync.Mutex
	ultimaClassica *image.RGBA
	zonaClassica   Zona
)

// cattura: prima la duplicazione del desktop (pochi ms); se non c'è, la cattura classica.
func cattura(z Zona) (*image.RGBA, error) {
	img, _, err := catturaGen(z, 0)
	return img, err
}

// catturaGen: come cattura, e dice anche il numero del fotogramma dello schermo (0 = non lo so,
// cattura classica). Con ultimo = il numero della cattura precedente della stessa zona, se lo
// schermo non ha mandato niente di nuovo restituisce errStessoFotogramma senza rifare
// l'immagine: chi chiama riusa quella che ha già (e la sua impronta).
func catturaGen(z Zona, ultimo uint64) (*image.RGBA, uint64, error) {
	var img *image.RGBA
	var gen uint64
	err := errDXGINonDisponibile
	if !dxgiInaffidabile.Load() {
		img, gen, err = catturaDXGIGen(z, ultimo)
	}
	if err == errStessoFotogramma {
		return nil, gen, err
	}
	if err == nil {
		if !catturaNera(img) {
			return img, gen, nil
		}
		// tutta nera: scena davvero scura o duplicazione che non funziona? Controllo una volta
		// ogni tanto con la cattura classica
		if time.Since(time.Unix(0, controlloNero.Load())) > 3*time.Second {
			controlloNero.Store(time.Now().UnixNano())
			if c, e := catturaClassica(z); e == nil && !catturaNera(c) {
				// una dissolvenza dal nero può ingannare: la spengo solo dopo 3 volte di fila
				if conferme.Add(1) >= 3 {
					dxgiInaffidabile.Store(true)
				}
				return c, 0, nil
			}
			conferme.Store(0)
		}
		return img, gen, nil
	}
	muClassica.Lock()
	defer muClassica.Unlock()
	// la copia si riusa solo se la duplicazione ha già dimostrato di mandare i fotogrammi
	// (con i monitor spenti o in casi strani non li manda mai: allora cattura classica sempre)
	if err == errNessunFotogramma && dxgiFunziona.Load() && ultimaClassica != nil && zonaClassica == z {
		// lo schermo non è cambiato dall'ultima cattura classica: è ancora quella
		out := image.NewRGBA(ultimaClassica.Bounds())
		copy(out.Pix, ultimaClassica.Pix)
		return out, 0, nil
	}
	img, err = catturaClassica(z)
	if err == nil {
		ultimaClassica, zonaClassica = img, z
		out := image.NewRGBA(img.Bounds())
		copy(out.Pix, img.Pix)
		return out, 0, nil
	}
	return nil, 0, err
}

var (
	dxgiInaffidabile atomic.Bool  // la duplicazione dà immagini nere su questo PC: non la uso
	controlloNero    atomic.Int64 // ultima volta che ho confrontato con la cattura classica
	conferme         atomic.Int32 // volte di fila che la cattura veloce era nera e quella classica no
)

// catturaClassica: BitBlt dallo schermo (lento su alcuni PC: circa 250 ms su quello di Sergio).
func catturaClassica(z Zona) (*image.RGBA, error) {
	w, h := int32(z.W), int32(z.H)
	if w <= 0 || h <= 0 {
		return nil, errZonaVuota
	}
	scr, _, _ := pGetDC.Call(0)
	if scr == 0 {
		return nil, errCattura
	}
	defer pReleaseDC.Call(0, scr)
	mem, _, _ := pCreateCompatDC.Call(scr)
	if mem == 0 {
		return nil, errCattura
	}
	defer pDeleteDC.Call(mem)
	bmp, _, _ := pCreateCompatBmp.Call(scr, uintptr(w), uintptr(h))
	if bmp == 0 {
		return nil, errCattura
	}
	defer pDeleteObject.Call(bmp)
	old, _, _ := pSelectObject.Call(mem, bmp)
	defer pSelectObject.Call(mem, old)
	const srcCopy = 0x00CC0020
	if r, _, _ := pBitBlt.Call(mem, 0, 0, uintptr(w), uintptr(h), scr, uintptr(int32(z.X)), uintptr(int32(z.Y)), srcCopy); r == 0 { // senza CAPTUREBLT: 10 volte più svelto (le finestre "a strati" come le nostre non servono)
		return nil, errCattura
	}
	bi := bitmapInfoHeader{biWidth: w, biHeight: -h, biPlanes: 1, biBitCount: 32}
	bi.biSize = uint32(unsafe.Sizeof(bi))
	buf := make([]byte, int(w)*int(h)*4)
	if r, _, _ := pGetDIBits.Call(mem, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), 0); r == 0 {
		return nil, errCattura
	}
	img := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	for i := 0; i < len(buf); i += 4 { // BGRA -> RGBA
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = buf[i+2], buf[i+1], buf[i], 255
	}
	return img, nil
}

// ---------------------------------------------------------------- finestrella sopra il gioco

type finestrella struct {
	mu        sync.Mutex
	hwnd      uintptr
	testo     string
	inglese   string
	stato     string
	visibile  bool
	mostraEN  bool
	dimFont   int
	posizione string // sopra | sotto | copri | lato
	fontT     uintptr
	fontP     uintptr
	sfondo    uintptr
	bordo     uintptr
	zona      Zona
	gioco     uintptr // finestra del gioco (quella in primo piano quando hai premuto F9)
	exeGioco  string  // programma del gioco: vale anche per una finestra nuova dello stesso gioco
	nascosta  bool    // nascosta perché sei uscito dal gioco (Alt+Tab)
	voce      bool    // sta mostrando il sottotitolo di una voce (fascia in basso, senza stato)
	salvato   *dialogoSalvato
	tasti     func(id int)
	selezione func(Zona)
}

var ov = &finestrella{dimFont: 24}

const (
	tastoSeleziona = 1 + iota
	tastoTraduci
	tastoAuto
	tastoInglese
	tastoCronologia
	tastoPagina
	tastoChat
	tastoVoci
)

func avviaFinestrella(dimFont int, tasti func(int), selezione func(Zona)) {
	ov.dimFont, ov.tasti, ov.selezione = dimFont, tasti, selezione
	pronta := make(chan struct{})
	go func() {
		defer guard("finestrella")
		runtime.LockOSThread()
		classe, _ := windows.UTF16PtrFromString("TassoTraduceOverlay")
		wc := wndClassEx{lpfnWndProc: windows.NewCallback(ovProc), lpszClassName: classe}
		wc.cbSize = uint32(unsafe.Sizeof(wc))
		pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
		// LAYERED | TRANSPARENT (i clic passano al gioco) | TOPMOST | TOOLWINDOW | NOACTIVATE
		ex := uintptr(0x80000 | 0x20 | 0x8 | 0x80 | 0x08000000)
		h, _, _ := pCreateWindowEx.Call(ex, uintptr(unsafe.Pointer(classe)), 0, 0x80000000, 0, 0, 800, 120, 0, 0, 0, 0)
		ov.mu.Lock()
		ov.hwnd = h
		ov.mu.Unlock()
		if h != 0 {
			pSetLayeredAttr.Call(h, 0, 228, 2) // LWA_ALPHA: leggermente trasparente
			// La finestrella non compare nelle catture dello schermo (Windows 10 2004+):
			// così non rileggo la mia stessa traduzione.
			pSetDisplayAffin.Call(h, affinita()) // WDA_EXCLUDEFROMCAPTURE
			ov.creaFont()
			ov.sfondo, _, _ = pCreateBrush.Call(0x000D0807) // #07080D
			ov.bordo, _, _ = pCreateBrush.Call(0x00A6F32F)  // #2FF3A6 (BGR)
			// Tasti rapidi validi in tutto Windows, anche mentre giochi.
			pSetTimer.Call(h, 1, 400, 0) // controllo se il gioco è ancora in primo piano
			// i tasti rapidi si registrano solo quando accendi Tasso Traduce (goAccendi):
			// da spento F6…F11 restano liberi per gli altri programmi
			creaFinestraPagina()
			// traduzione accesa prima che la finestrella esistesse: cambiaTasti non ha potuto
			// registrare i tasti, li registro adesso
			if acceso.Load() {
				registraTasti(h, tastiCompleti(leggiImp().Tasti))
			}
			// 3.1: secondo riquadro, per la chat degli altri (stessa classe, stesso disegno)
			if hc, _, _ := pCreateWindowEx.Call(ex, uintptr(unsafe.Pointer(classe)), 0, 0x80000000, 0, 0, 420, 120, 0, 0, 0, 0); hc != 0 {
				pSetLayeredAttr.Call(hc, 0, 228, 2)
				pSetDisplayAffin.Call(hc, affinita())
				ovChat.mu.Lock()
				ovChat.creaFont()
				ovChat.sfondo, _, _ = pCreateBrush.Call(0x000D0807)
				ovChat.bordo, _, _ = pCreateBrush.Call(0x00FF7BA9) // #A97BFF (BGR): viola, diverso dai dialoghi
				ovChat.hwnd = hc
				ovChat.mu.Unlock()
			}
		}
		close(pronta)
		var m msgW
		for {
			r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
			pTranslateMsg.Call(uintptr(unsafe.Pointer(&m)))
			pDispatchMsg.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()
	<-pronta
}

func (f *finestrella) creaFont() {
	face, _ := windows.UTF16PtrFromString("Segoe UI")
	if f.fontT != 0 {
		pDeleteObject.Call(f.fontT)
		pDeleteObject.Call(f.fontP)
	}
	f.fontT, _, _ = pCreateFont.Call(uintptr(f.dimFont), 0, 0, 0, 600, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
	f.fontP, _, _ = pCreateFont.Call(uintptr(f.dimFont*2/3), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
}

// Mostra aggiorna il testo della finestrella e la mette sopra la zona dei dialoghi.
func (f *finestrella) Mostra(z Zona, testo, inglese, stato string) {
	f.mu.Lock()
	f.zona, f.testo, f.inglese, f.stato, f.visibile, f.voce, f.salvato = z, testo, inglese, stato, true, false, nil
	h := f.hwnd
	f.mu.Unlock()
	if h != 0 {
		pPostMessage.Call(h, wmAggiorna, 0, 0)
	}
}

// MostraVoce: sottotitolo di una voce, fascia scura centrata sulla zona, senza riga di stato.
type dialogoSalvato struct {
	zona                  Zona
	testo, inglese, stato string
}

func (f *finestrella) staMostrandoVoce() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.voce && f.visibile
}

// cambiaVoce: la correzione dell'IA arriva per il sottotitolo mostrato adesso.
func (f *finestrella) cambiaVoce(vecchio, nuovo string) bool {
	f.mu.Lock()
	ok := f.voce && f.testo == vecchio
	if ok {
		f.testo = nuovo
	}
	h := f.hwnd
	f.mu.Unlock()
	if ok && h != 0 {
		pPostMessage.Call(h, wmAggiorna, 0, 0)
	}
	return ok
}

func (f *finestrella) MostraVoce(z Zona, testo, inglese string) {
	f.mu.Lock()
	if !f.voce && f.visibile && f.testo != "" {
		f.salvato = &dialogoSalvato{f.zona, f.testo, f.inglese, f.stato}
	}
	f.zona, f.testo, f.inglese, f.stato, f.visibile, f.voce, f.nascosta = z, testo, inglese, "", true, true, false
	h := f.hwnd
	f.mu.Unlock()
	if h != 0 {
		pPostMessage.Call(h, wmAggiorna, 0, 0)
	}
}

// NascondiVoce: toglie il sottotitolo solo se c'è ancora una voce (non un dialogo letto dallo schermo).
func (f *finestrella) NascondiVoce() {
	f.mu.Lock()
	if !f.voce {
		f.mu.Unlock()
		return
	}
	f.visibile, f.voce = false, false
	if d := f.salvato; d != nil { // c'era un dialogo sotto: lo rimetto
		f.zona, f.testo, f.inglese, f.stato, f.visibile = d.zona, d.testo, d.inglese, d.stato, true
		f.salvato = nil
	}
	vis := f.visibile
	h := f.hwnd
	f.mu.Unlock()
	if h != 0 {
		if vis {
			pPostMessage.Call(h, wmAggiorna, 0, 0)
		} else {
			pPostMessage.Call(h, wmMostra, 0, 0)
		}
	}
}

// Nascondi toglie il dialogo (anche quello salvato sotto una voce, così non torna più).
// Se sta mostrando il sottotitolo di una voce, il sottotitolo resta fino alla sua fine.
func (f *finestrella) Nascondi() {
	f.mu.Lock()
	f.salvato = nil
	if f.voce {
		f.mu.Unlock()
		return
	}
	f.visibile = false
	h := f.hwnd
	f.mu.Unlock()
	if h != 0 {
		pPostMessage.Call(h, wmMostra, 0, 0)
	}
}

// Spegni toglie tutto: dialogo, dialogo salvato e sottotitolo della voce.
func (f *finestrella) Spegni() {
	f.mu.Lock()
	f.visibile, f.voce, f.salvato = false, false, nil
	h := f.hwnd
	f.mu.Unlock()
	if h != 0 {
		pPostMessage.Call(h, wmMostra, 0, 0)
	}
}

func (f *finestrella) ImpostaInglese(on bool) {
	f.mu.Lock()
	f.mostraEN = on
	h := f.hwnd
	f.mu.Unlock()
	if h != 0 {
		pPostMessage.Call(h, wmAggiorna, 0, 0)
	}
}

func (f *finestrella) ImpostaPosizione(p string) {
	f.mu.Lock()
	f.posizione = p
	h := f.hwnd
	f.mu.Unlock()
	if h != 0 {
		pPostMessage.Call(h, wmAggiorna, 0, 0)
	}
}

func (f *finestrella) ImpostaFont(px int) {
	f.mu.Lock()
	f.dimFont = clamp(px, 14, 48)
	h := f.hwnd
	f.mu.Unlock()
	if h != 0 {
		pPostMessage.Call(h, wmAggiorna, 1, 0)
	}
}

// RicordaGioco: chiamata quando premi F9 mentre giochi, prima di aprire la scelta della zona.
func (f *finestrella) RicordaGioco() {
	fg, _, _ := pGetForeground.Call()
	f.impostaGioco(fg)
}

// impostaGioco: h diventa la finestra del gioco, se è davvero un altro programma (non il velo
// della scelta, non le nostre finestre, non il desktop).
func (f *finestrella) impostaGioco(h uintptr) bool {
	exe := exeDiFinestra(h)
	if h == 0 || h == sel.hwnd.Load() || eNostro(exe) {
		return false
	}
	f.mu.Lock()
	f.gioco, f.exeGioco = h, exe
	f.mu.Unlock()
	return true
}

// giocoDavanti: fg è il gioco ricordato (la sua finestra, o un'altra finestra dello stesso
// programma, per esempio il gioco chiuso e riaperto). Senza gioco ricordato vale qualsiasi
// finestra. Una finestra nuova del gioco diventa quella ricordata.
func (f *finestrella) giocoDavanti(fg uintptr) bool {
	f.mu.Lock()
	g, exe := f.gioco, f.exeGioco
	f.mu.Unlock()
	if exe == "" || (fg == g && g != 0) {
		return true
	}
	if fg == 0 || exeDiFinestra(fg) != exe {
		return false
	}
	f.mu.Lock()
	if f.exeGioco == exe {
		f.gioco = fg
	}
	f.mu.Unlock()
	return true
}

var (
	pWindowFromPoint = user32.NewProc("WindowFromPoint")
	pGetAncestor     = user32.NewProc("GetAncestor")
)

// finestraInZona: la finestra principale che sta sotto il centro della zona (per sapere di
// quale gioco è la zona scelta col pulsante in Tasso, quando davanti c'era Tasso).
func finestraInZona(z Zona) uintptr {
	x, y := int32(z.X+z.W/2), int32(z.Y+z.H/2)
	pt := uintptr(uint32(x)) | uintptr(uint32(y))<<32 // POINT passato per valore (64 bit)
	h, _, _ := pWindowFromPoint.Call(pt)
	if h == 0 {
		return 0
	}
	r, _, _ := pGetAncestor.Call(h, 2) // GA_ROOT
	if r != 0 {
		h = r
	}
	return h
}

func (f *finestrella) giocoAttuale() uintptr {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gioco
}

func (f *finestrella) ApriSelezione() {
	f.mu.Lock()
	h := f.hwnd
	f.mu.Unlock()
	if h != 0 {
		pPostMessage.Call(h, wmSeleziona, 0, 0)
	}
}

const (
	dtWordBreak = 0x10
	dtCalcRect  = 0x400
	dtNoPrefix  = 0x800
	dtLeft      = 0x0
)

// misura l'altezza del testo con la larghezza data
func misura(hdc, font uintptr, s string, larg int32) int32 {
	if s == "" {
		return 0
	}
	pSelectObject.Call(hdc, font)
	r := rect{0, 0, larg, 0}
	t := testoW(s) // senza caratteri nulli: con uno dentro, &t[0] chiudeva Tasso
	pDrawText.Call(hdc, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&r)), dtWordBreak|dtCalcRect|dtNoPrefix)
	return r.Bottom - r.Top
}

func disegna(hdc, font uintptr, colore uintptr, s string, r rect) {
	if s == "" {
		return
	}
	pSelectObject.Call(hdc, font)
	pSetTextColor.Call(hdc, colore)
	t := testoW(s)
	pDrawText.Call(hdc, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&r)), dtWordBreak|dtNoPrefix|dtLeft)
}

const margine = 16

func disegnaCentrato(hdc, font uintptr, colore uintptr, s string, r rect) {
	if s == "" {
		return
	}
	pSelectObject.Call(hdc, font)
	pSetTextColor.Call(hdc, colore)
	t := testoW(s)
	pDrawText.Call(hdc, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&r)), dtWordBreak|dtNoPrefix|0x1) // DT_CENTER
}

// posiziona: stessa larghezza della zona, appena sopra (o sotto, se sopra non c'è spazio).
func (f *finestrella) posiziona(hwnd uintptr) {
	f.mu.Lock()
	z, testo, en, stato, mostraEN, vis, pos := f.zona, f.testo, f.inglese, f.stato, f.mostraEN, f.visibile && !f.nascosta, f.posizione
	if f.voce {
		pos, stato = "sottotitoli", ""
	}
	f.mu.Unlock()
	if !vis || testo == "" {
		pShowWindow.Call(hwnd, 0)
		return
	}
	larg := int32(z.W)
	if larg < 420 {
		larg = 420
	}
	if pos == "lato" && larg > 640 {
		larg = 640
	}
	if pos == "sottotitoli" {
		larg = int32(z.W) // fascia larga come la zona dei sottotitoli
	}
	hdc, _, _ := pGetDC.Call(hwnd)
	alt := int32(margine) + misura(hdc, f.fontT, testo, larg-2*margine)
	if mostraEN && en != "" {
		alt += misura(hdc, f.fontP, en, larg-2*margine) + 6
	}
	if stato != "" {
		alt += misura(hdc, f.fontP, stato, larg-2*margine) + 4
	}
	alt += margine
	if pos == "sottotitoli" && alt < int32(z.H) {
		alt = int32(z.H) // copre tutti i sottotitoli inglesi
	}
	pReleaseDC.Call(hwnd, hdc)
	vy, _, _ := pGetSysMetrics.Call(smYVirtual)
	vx, _, _ := pGetSysMetrics.Call(smXVirtual)
	vw, _, _ := pGetSysMetrics.Call(smCXVirtual)
	vh, _, _ := pGetSysMetrics.Call(smCYVirtual)
	x, y := posizioneFinestrella(pos, z, larg, alt, int32(vx), int32(vy), int32(vw), int32(vh))
	pSetWindowPos.Call(hwnd, ^uintptr(0), uintptr(x), uintptr(y), uintptr(larg), uintptr(alt), swpNoActivate)
	pShowWindow.Call(hwnd, 4) // SW_SHOWNOACTIVATE
	pInvalidateRect.Call(hwnd, 0, 1)
}

func ovProc(hwnd, msg, wp, lp uintptr) uintptr {
	f := finDi(hwnd)
	switch msg {
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		var r rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
		pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), f.sfondo)
		pFrameRect.Call(hdc, uintptr(unsafe.Pointer(&r)), f.bordo)
		pSetBkMode.Call(hdc, 1) // TRANSPARENT
		f.mu.Lock()
		testo, en, stato, mostraEN, cinema := f.testo, f.inglese, f.stato, f.mostraEN, f.posizione == "sottotitoli" || f.voce
		f.mu.Unlock()
		larg := r.Right - 2*margine
		y := int32(margine)
		if cinema { // sottotitoli: testo centrato, senza riga di stato, fascia senza bordo verde
			pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), f.sfondo)
			ht := misura(hdc, f.fontT, testo, larg)
			y = (r.Bottom - ht) / 2
			if y < 4 {
				y = 4
			}
			disegnaCentrato(hdc, f.fontT, 0x00F8F4F2, testo, rect{margine, y, margine + larg, y + ht})
			pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			return 0
		}
		if mostraEN && en != "" {
			hh := misura(hdc, f.fontP, en, larg)
			disegna(hdc, f.fontP, 0x00A7918B, en, rect{margine, y, margine + larg, y + hh}) // grigio
			y += hh + 6
		}
		ht := misura(hdc, f.fontT, testo, larg)
		disegna(hdc, f.fontT, 0x00F8F4F2, testo, rect{margine, y, margine + larg, y + ht}) // bianco
		y += ht + 4
		if stato != "" {
			disegna(hdc, f.fontP, 0x00A6F32F, stato, rect{margine, y, margine + larg, r.Bottom})
		}
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmAggiorna:
		if wp == 1 {
			f.mu.Lock()
			f.creaFont()
			f.mu.Unlock()
		}
		f.posiziona(hwnd)
		return 0
	case wmMostra:
		f.posiziona(hwnd)
		return 0
	case 0x0113: // WM_TIMER: se esci dal gioco (Alt+Tab) la finestrella si nasconde, quando torni riappare
		fg, _, _ := pGetForeground.Call()
		f.mu.Lock()
		g := f.gioco
		f.mu.Unlock()
		chiuso := false
		if g != 0 {
			if v, _, _ := pIsWindow.Call(g); v == 0 {
				chiuso = true
				// il gioco è stato chiuso: la sua ultima traduzione non deve ricomparire sul
				// desktop o sopra un altro programma (il sottotitolo di una voce resta)
				f.mu.Lock()
				if f.gioco == g {
					// tengo il nome del programma: se riapri il gioco la traduzione torna, ma sopra
					// un altro programma no (con exeGioco vuoto giocoDavanti direbbe sì a tutti)
					f.gioco, f.salvato = 0, nil
					if !f.voce {
						f.visibile = false
					}
				}
				f.mu.Unlock()
			}
		}
		davanti := fg == sel.hwnd.Load() || f.giocoDavanti(fg)
		f.mu.Lock()
		// il sottotitolo di una voce segue la finestra in primo piano: non lo nascondo
		via := !f.voce && !davanti
		cambia := via != f.nascosta || chiuso
		f.nascosta = via
		f.mu.Unlock()
		if cambia {
			f.posiziona(hwnd)
		}
		return 0
	case wmSeleziona:
		apriSelezione()
		return 0
	case wmTasti:
		pag := tastiDaRegistrare.Load()
		if pag == nil {
			return 0
		}
		f := registraTasti(hwnd, *pag)
		tastiFalliti.Store(&f)
		return uintptr(len(f))
	case wmHotkey:
		// lo stesso tasto non deve arrivare due volte, dal gancio e da Windows: conta il primo
		daGancio := lp == lpDalGancio
		if u, ok := tastoVisto[wp]; ok && u.gancio != daGancio && time.Since(u.quando) < 300*time.Millisecond {
			return 0
		}
		tastoVisto[wp] = tastoArrivato{time.Now(), daGancio}
		if f.tasti != nil {
			go func(id int) {
				defer guard("tasto")
				f.tasti(id)
			}(int(wp))
		}
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wp, lp)
	return r
}

// finDi: quale riquadro è (quello dei dialoghi o quello della chat degli altri).
func finDi(hwnd uintptr) *finestrella {
	ovChat.mu.Lock()
	h := ovChat.hwnd
	ovChat.mu.Unlock()
	if h != 0 && hwnd == h {
		return ovChat
	}
	return ov
}

// ---------------------------------------------------------------- scelta della zona

var sel struct {
	hwnd           atomic.Uintptr // letta anche da altri fili (timer, chat degli altri)
	attivo         bool
	x0, y0, x1, y1 int32
	vx, vy         int32
	pennello       uintptr
	registrata     bool
}

// apriSelezione copre tutti gli schermi con un velo scuro: si trascina un rettangolo
// sopra i dialoghi, Esc o tasto destro annulla. Gira nel thread della finestrella.
func apriSelezione() {
	if sel.hwnd.Load() != 0 {
		return
	}
	classe, _ := windows.UTF16PtrFromString("TassoTraduceZona")
	if !sel.registrata {
		cur, _, _ := pLoadCursor.Call(0, 32515) // IDC_CROSS
		wc := wndClassEx{lpfnWndProc: windows.NewCallback(selProc), lpszClassName: classe, hCursor: cur}
		wc.cbSize = uint32(unsafe.Sizeof(wc))
		pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
		sel.pennello, _, _ = pCreateBrush.Call(0x00A6F32F)
		sel.registrata = true
	}
	vx, _, _ := pGetSysMetrics.Call(smXVirtual)
	vy, _, _ := pGetSysMetrics.Call(smYVirtual)
	vw, _, _ := pGetSysMetrics.Call(smCXVirtual)
	vh, _, _ := pGetSysMetrics.Call(smCYVirtual)
	sel.vx, sel.vy = int32(vx), int32(vy)
	sel.attivo = false
	ex := uintptr(0x80000 | 0x8 | 0x80) // LAYERED | TOPMOST | TOOLWINDOW
	h, _, _ := pCreateWindowEx.Call(ex, uintptr(unsafe.Pointer(classe)), 0, 0x80000000|0x10000000,
		uintptr(int32(vx)), uintptr(int32(vy)), vw, vh, 0, 0, 0, 0)
	if h == 0 {
		return
	}
	sel.hwnd.Store(h)
	pSetLayeredAttr.Call(h, 0, 110, 2)
	pSetForeground.Call(h)
}

func chiudiSelezione(z *Zona) {
	h := sel.hwnd.Swap(0)
	pReleaseCapture.Call()
	pDestroyWindow.Call(h)
	if z == nil {
		selPerChat.Store(false) // scelta annullata: la prossima zona torna a essere quella dei dialoghi
	}
	if z != nil && ov.selezione != nil {
		zz := *z
		go func() {
			defer guard("zona")
			ov.selezione(zz)
		}()
	}
}

func selProc(hwnd, msg, wp, lp uintptr) uintptr {
	x := int32(int16(lp & 0xFFFF))
	y := int32(int16((lp >> 16) & 0xFFFF))
	switch msg {
	case wmLButtonDown:
		sel.attivo = true
		sel.x0, sel.y0, sel.x1, sel.y1 = x, y, x, y
		pSetCapture.Call(hwnd)
		return 0
	case wmMouseMove:
		if sel.attivo {
			sel.x1, sel.y1 = x, y
			pInvalidateRect.Call(hwnd, 0, 1)
		}
		return 0
	case wmLButtonUp:
		if !sel.attivo {
			return 0
		}
		sel.attivo = false
		z := zonaDaPunti(int(sel.x0+sel.vx), int(sel.y0+sel.vy), int(x+sel.vx), int(y+sel.vy))
		if z.W < 20 || z.H < 12 {
			chiudiSelezione(nil)
			return 0
		}
		chiudiSelezione(&z)
		return 0
	case wmRButtonDown:
		chiudiSelezione(nil)
		return 0
	case wmKeyDown:
		if wp == 0x1B { // Esc
			chiudiSelezione(nil)
		}
		return 0
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		var r rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
		nero, _, _ := pCreateBrush.Call(0)
		pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), nero)
		pDeleteObject.Call(nero)
		pSetBkMode.Call(hdc, 1)
		face, _ := windows.UTF16PtrFromString("Segoe UI")
		font, _, _ := pCreateFont.Call(30, 0, 0, 0, 600, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
		disegna(hdc, font, 0x00FFFFFF, testoVelo(selPerChat.Load()), rect{40, 40, r.Right - 40, 120})
		pDeleteObject.Call(font)
		if sel.attivo {
			a, b, c, d := sel.x0, sel.y0, sel.x1, sel.y1
			if a > c {
				a, c = c, a
			}
			if b > d {
				b, d = d, b
			}
			for i := int32(0); i < 3; i++ { // bordo spesso 3 px
				rr := rect{a - i, b - i, c + i, d + i}
				pFrameRect.Call(hdc, uintptr(unsafe.Pointer(&rr)), sel.pennello)
			}
		}
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wp, lp)
	return r
}

// testoVelo: la scritta sul velo della scelta della zona (dialoghi o chat degli altri).
func testoVelo(chat bool) string {
	if chat {
		return testoTasso("Trascina un rettangolo sopra la chat del gioco   ·   Esc o tasto destro per annullare",
			"Drag a rectangle over the game chat   ·   Esc or right-click to cancel")
	}
	return testoTasso("Trascina un rettangolo sopra i dialoghi   ·   Esc o tasto destro per annullare",
		"Drag a rectangle over the dialogue   ·   Esc or right-click to cancel")
}

var (
	pUnregisterHotKey = user32.NewProc("UnregisterHotKey")
	pSendMessage      = user32.NewProc("SendMessageW")
	tastiDaRegistrare atomic.Pointer[map[string]uint32]
)

// tastoVisto: l'ultima volta che è arrivato ogni tasto rapido e da dove (solo dal filo della finestrella)
type tastoArrivato struct {
	quando time.Time
	gancio bool
}

var tastoVisto = map[uintptr]tastoArrivato{}

var idTasti = map[string]uintptr{
	"zona": tastoSeleziona, "traduci": tastoTraduci, "auto": tastoAuto,
	"inglese": tastoInglese, "cronologia": tastoCronologia, "pagina": tastoPagina,
	"chat": tastoChat, "voci": tastoVoci,
}

// registraTasti (sul filo della finestrella): toglie i tasti vecchi e mette quelli nuovi.
// Restituisce le funzioni dei tasti che non si sono potuti registrare (già presi da un altro
// programma), in ordine.
func registraTasti(h uintptr, t map[string]uint32) []string {
	var falliti []string
	presi := map[uint32]uintptr{} // codice del tasto -> id, per il gancio della tastiera
	for _, id := range idTasti {
		pUnregisterHotKey.Call(h, id)
	}
	// Traduzione spenta (resta solo il tasto che la accende, F7): niente tasto rapido di tutto
	// Windows. Solo il gancio della tastiera, che lo prende quando davanti c'è un gioco (anche
	// a schermo intero non conosciuto); negli altri programmi F7 resta loro (Word, browser…:
	// prima premerlo sul desktop accendeva la traduzione e apriva il motore, collaudo 26/11).
	if soloTastoNelGioco(t) {
		for nome, id := range idTasti {
			if c := t[nome]; c != 0 {
				presi[c] = id
			}
		}
		aggiornaGancio(h, presi)
		if gancioOra.Load() != nil {
			return nil
		}
		presi = map[uint32]uintptr{} // niente gancio: si torna al tasto rapido normale
	}
	for nome, id := range idTasti {
		c := t[nome]
		if c == 0 {
			continue
		}
		mod, vk := uintptr(c>>16), uintptr(c&0xFFFF)
		if ok, _, _ := pRegisterHotKey.Call(h, id, mod|0x4000, vk); ok == 0 { // 0x4000: MOD_NOREPEAT
			falliti = append(falliti, nome)
			continue
		}
		presi[c] = id
	}
	aggiornaGancio(h, presi)
	sort.Strings(falliti)
	return falliti
}

// soloTastoNelGioco: i tasti sono solo quello che accende la traduzione (traduzione spenta).
func soloTastoNelGioco(t map[string]uint32) bool {
	n := 0
	for _, c := range t {
		if c != 0 {
			n++
		}
	}
	return n == 1 && t["pagina"] != 0 && !acceso.Load()
}

// ---------------------------------------------------------------- tasti che il gioco non deve vedere

// Molti giochi leggono la tastiera direttamente (Raw Input, DirectInput): ricevono F7…F11 anche
// se Tasso li ha registrati come tasti rapidi, e F9 in tanti giochi è il "caricamento rapido".
// Mentre davanti c'è un gioco, un gancio della tastiera (WH_KEYBOARD_LL, lo stesso che usano
// Discord e OBS per i loro tasti) ingoia SOLO i tasti di Tasso: il gioco non li vede, e Tasso
// riceve il suo tasto rapido come sempre. Tutti gli altri tasti passano intatti. Con la
// traduzione spenta i tasti registrati sono solo quello che la accende: il gancio resta e ingoia
// solo quello (prima F7 da spento arrivava anche al gioco).
// Il gancio ha un filo tutto suo che non fa altro: risponde subito e non rallenta la tastiera.

var (
	pSetHook        = user32.NewProc("SetWindowsHookExW")
	pUnhook         = user32.NewProc("UnhookWindowsHookEx")
	pCallNextHook   = user32.NewProc("CallNextHookEx")
	pGetAsyncKey    = user32.NewProc("GetAsyncKeyState")
	pPostThreadMsg  = user32.NewProc("PostThreadMessageW")
	pPeekMessage    = user32.NewProc("PeekMessageW")
	pSetWinEvent    = user32.NewProc("SetWinEventHook")
	pUnhookWinEvent = user32.NewProc("UnhookWinEvent")
	pGetModuleHandl = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW")

	tastiIngoiati       atomic.Pointer[map[uint32]uintptr] // codice del tasto (come in tasti.go) -> id del tasto rapido
	ovGancio            atomic.Uintptr                     // la finestrella che riceve i tasti rapidi
	muGancio            sync.Mutex
	gancioFilo          *istanzaGancio                // il gancio acceso (nil = spento), con muGancio
	gancioOra           atomic.Pointer[istanzaGancio] // lo stesso, letto dal gancio senza bloccare
	callbackGanc        = sync.OnceValue(func() uintptr { return windows.NewCallback(gancioTastiera) })
	callbackDav         = sync.OnceValue(func() uintptr { return windows.NewCallback(finestraDavanti) })
	finestreControllate atomic.Pointer[map[uintptr]finestraControllata] // finestre già controllate (solo seguiGiocoDavanti scrive)
)

// istanzaGancio: un gancio acceso, con il suo stato. Ogni accensione ne ha uno nuovo: un gancio
// vecchio che si sta ancora spegnendo non tocca lo stato di quello nuovo.
type istanzaGancio struct {
	id           uint32         // filo del gancio
	finito       chan struct{}  // chiuso quando il gancio è stato tolto davvero
	giocoInPrimo atomic.Uintptr // la finestra in primo piano, se è un gioco (0 se no)
	davanti      chan uintptr   // finestra nuova in primo piano (da finestraDavanti)

	// solo dal filo del gancio
	tastoGiu    [256]bool   // tasto di Tasso premuto e ingoiato: anche il suo rilascio va ingoiato
	tastoQuando [256]uint32 // ora (ms) dell'ultima pressione ingoiata
}

// finestraControllata: se una finestra è di un gioco, e quando l'ho controllato.
type finestraControllata struct {
	gioco  bool
	quando int64 // UnixNano
}

// lpDalGancio: segno nel WM_HOTKEY mandato dal gancio (quelli di Windows hanno tasto e modificatori)
const lpDalGancio = ^uintptr(0)

type kbdllHook struct {
	vk, scan, flags, tempo uint32
	extra                  uintptr
}

// aggiornaGancio (dal filo della finestrella): accende il gancio se ci sono tasti di Tasso (anche
// a traduzione spenta: allora c'è solo il tasto che la accende), se no lo spegne.
func aggiornaGancio(h uintptr, presi map[uint32]uintptr) {
	if len(presi) == 0 {
		tastiIngoiati.Store(nil)
		fermaGancio()
		return
	}
	ovGancio.Store(h)
	tastiIngoiati.Store(&presi)
	avviaGancio()
}

func avviaGancio() {
	muGancio.Lock()
	defer muGancio.Unlock()
	if g := gancioFilo; g != nil {
		select {
		case <-g.finito: // finito da solo: lo riaccendo
			gancioFilo = nil
			gancioOra.Store(nil)
		default:
			return // già acceso
		}
	}
	g := &istanzaGancio{finito: make(chan struct{}), davanti: make(chan uintptr, 1)}
	pronto := make(chan uint32, 1)
	go func() {
		defer close(g.finito) // per ultimo: dopo aver tolto il gancio
		defer guard("gancio-tastiera")
		inviato := false
		defer func() {
			if !inviato {
				pronto <- 0 // anche dopo un errore: avviaGancio non resta ad aspettare
			}
		}()
		runtime.LockOSThread() // il gancio risponde sul filo che lo ha messo: resta questo
		ferma := make(chan struct{})
		defer close(ferma)
		var m msgW
		pPeekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 0) // crea la coda dei messaggi del filo
		g.id = windows.GetCurrentThreadId()
		mod, _, _ := pGetModuleHandl.Call(0)
		gancioOra.Store(g) // prima del gancio: il gancio lo legge da subito
		gancio, _, _ := pSetHook.Call(13 /*WH_KEYBOARD_LL*/, callbackGanc(), mod, 0)
		if gancio == 0 {
			gancioOra.CompareAndSwap(g, nil)
			return // niente gancio: restano i tasti rapidi normali
		}
		defer pUnhook.Call(gancio)
		// la finestra in primo piano cambia (Alt+Tab): lo so subito, non al controllo dopo.
		// 3 = EVENT_SYSTEM_FOREGROUND, 2 = WINEVENT_SKIPOWNPROCESS (fuori dal processo, su questo filo)
		if ev, _, _ := pSetWinEvent.Call(3, 3, 0, callbackDav(), 0, 0, 2); ev != 0 {
			defer pUnhookWinEvent.Call(ev)
		}
		go seguiGiocoDavanti(g, ferma)
		inviato = true
		pronto <- g.id
		for {
			r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 { // WM_QUIT da fermaGancio
				return
			}
			pTranslateMsg.Call(uintptr(unsafe.Pointer(&m)))
			pDispatchMsg.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()
	if <-pronto == 0 {
		<-g.finito
		return
	}
	gancioFilo = g
}

// fermaGancio: toglie il gancio e aspetta che sia tolto davvero. Prima non aspettava: F7
// spento e riacceso in fretta poteva lasciare due ganci accesi (tasto eseguito due volte).
func fermaGancio() {
	muGancio.Lock()
	defer muGancio.Unlock()
	g := gancioFilo
	if g == nil {
		return
	}
	gancioFilo = nil
	gancioOra.CompareAndSwap(g, nil) // da subito il gancio vecchio non ingoia più niente
	pPostThreadMsg.Call(uintptr(g.id), 0x0012 /*WM_QUIT*/, 0, 0)
	select {
	case <-g.finito:
	case <-time.After(2 * time.Second): // non deve mai succedere: non blocco la finestrella per sempre
	}
}

// finestraDavanti: chiamata da Windows (sul filo del gancio) quando cambia la finestra in primo
// piano. Non fa nessun controllo qui (il gancio deve restare svelto): passa la finestra a
// seguiGiocoDavanti, che la controlla subito.
func finestraDavanti(hook, evento, hwnd, idObj, idFiglio, filo, tempo uintptr) uintptr {
	if g := gancioOra.Load(); g != nil && evento == 3 {
		select {
		case g.davanti <- hwnd:
		default: // ce n'è già una da controllare: seguiGiocoDavanti rilegge comunque la finestra davanti
		}
	}
	return 0
}

// seguiGiocoDavanti: guarda se in primo piano c'è un gioco, subito a ogni cambio di finestra
// (finestraDavanti) e comunque ogni 250 ms. Il controllo (percorso del programma, giochi di
// Steam) si fa qui e non nel gancio, che deve rispondere subito.
func seguiGiocoDavanti(g *istanzaGancio, ferma <-chan struct{}) {
	defer guard("gioco-davanti")
	defer g.giocoInPrimo.Store(0)
	var ultimo uintptr
	var gioco bool
	var quando time.Time
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for {
		fg, _, _ := pGetForeground.Call()
		if fg != ultimo || time.Since(quando) > 5*time.Second {
			// un gioco riconosciuto, o una finestra che la traduzione considera un gioco (anche a
			// schermo intero non conosciuto): lì i tasti di Tasso sono di Tasso
			ultimo, quando, gioco = fg, time.Now(), finestraDiGioco(fg) || finestraPerTradurre(fg)
			ricordaGioco(fg, gioco, quando)
		}
		if gioco {
			g.giocoInPrimo.Store(fg)
		} else {
			g.giocoInPrimo.Store(0)
		}
		select {
		case <-ferma:
			return
		case <-g.davanti:
		case <-t.C:
		}
	}
}

// ricordaGioco: tiene da parte se la finestra h è un gioco, per il gancio (copia nuova della
// mappa: il gancio la legge senza blocchi). Al massimo 64 finestre.
func ricordaGioco(h uintptr, gioco bool, quando time.Time) {
	if h == 0 {
		return
	}
	vecchia := finestreControllate.Load()
	if vecchia != nil {
		if v, ok := (*vecchia)[h]; ok && v.gioco == gioco && quando.UnixNano()-v.quando < int64(time.Second) {
			return // niente di nuovo: non rifaccio la mappa
		}
	}
	nuova := map[uintptr]finestraControllata{}
	if vecchia != nil && len(*vecchia) < 64 {
		for k, v := range *vecchia {
			nuova[k] = v
		}
	}
	nuova[h] = finestraControllata{gioco, quando.UnixNano()}
	finestreControllate.Store(&nuova)
}

// eGiocoDavanti (dal gancio, svelto): fg è il gioco in primo piano. Se seguiGiocoDavanti non ha
// ancora visto il cambio di finestra (Alt+Tab appena fatto), uso quello che sapevo già di
// questa finestra, se controllata negli ultimi 10 minuti.
func eGiocoDavanti(g *istanzaGancio, fg uintptr) bool {
	if fg == 0 {
		return false
	}
	if fg == g.giocoInPrimo.Load() {
		return true
	}
	if m := finestreControllate.Load(); m != nil {
		if v, ok := (*m)[fg]; ok && v.gioco && time.Now().UnixNano()-v.quando < int64(10*time.Minute) {
			return true
		}
	}
	return false
}

// finestraDiGioco: la finestra h è di un gioco (riconosciuto, o quello che stai traducendo) e
// non di Tasso.
func finestraDiGioco(h uintptr) bool {
	if h == 0 || h == sel.hwnd.Load() {
		return false
	}
	var pid uint32
	pGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 || pid == windows.GetCurrentProcessId() || eNostro(exeDiFinestra(h)) {
		return false
	}
	if (paginaAttiva.Load() && h == paginaGiocoA.Load()) || h == ov.giocoAttuale() {
		return true
	}
	_, ok := riconosciGioco(percorsoDiFinestra(h))
	return ok
}

// modificatoriPremuti: Alt=1, Ctrl=2, Maiusc=4 come in tasti.go; 8 se c'è il tasto Windows
// (nessun tasto di Tasso lo usa: allora non ingoio niente).
func modificatoriPremuti() uint32 {
	giu := func(vk uintptr) bool {
		r, _, _ := pGetAsyncKey.Call(vk)
		return r&0x8000 != 0
	}
	var m uint32
	if giu(0x12) {
		m |= 1
	}
	if giu(0x11) {
		m |= 2
	}
	if giu(0x10) {
		m |= 4
	}
	if giu(0x5B) || giu(0x5C) {
		m |= 8
	}
	return m
}

// gancioTastiera: chiamato da Windows per ogni tasto, prima che arrivi a qualunque programma.
func gancioTastiera(nCode, wp, lp uintptr) uintptr {
	if g := gancioOra.Load(); g != nil && int32(nCode) >= 0 && lp != 0 {
		k := *(**kbdllHook)(unsafe.Pointer(&lp)) // lp è un puntatore di Windows alla struttura del tasto
		vk := k.vk & 0xFF
		switch wp {
		case 0x101, 0x105: // WM_KEYUP, WM_SYSKEYUP
			if g.tastoGiu[vk] {
				g.tastoGiu[vk] = false
				return 1 // anche il rilascio non arriva al gioco
			}
		case 0x100, 0x104: // WM_KEYDOWN, WM_SYSKEYDOWN
			if t := tastiIngoiati.Load(); t != nil && k.flags&0x10 == 0 { // 0x10: tasto simulato da un programma
				if fg, _, _ := pGetForeground.Call(); eGiocoDavanti(g, fg) {
					if id, ok := (*t)[modificatoriPremuti()<<16|vk]; ok {
						// tenuto premuto: Windows ripete la pressione; il tasto rapido parte una
						// volta sola (come MOD_NOREPEAT). Una pressione dopo più di un secondo è nuova
						ripetuto := g.tastoGiu[vk] && k.tempo-g.tastoQuando[vk] < 1200
						g.tastoGiu[vk], g.tastoQuando[vk] = true, k.tempo
						if !ripetuto {
							pPostMessage.Call(ovGancio.Load(), wmHotkey, id, lpDalGancio)
						}
						return 1
					}
				}
			}
		}
	}
	r, _, _ := pCallNextHook.Call(0, nCode, wp, lp)
	return r
}

// cambiaTasti: dal programma, chiede alla finestrella di usare questi tasti. Restituisce le
// funzioni dei tasti già presi da un altro programma.
var (
	muCambiaTasti sync.Mutex
	tastiFalliti  atomic.Pointer[[]string]
)

func cambiaTasti(t map[string]uint32) []string {
	ov.mu.Lock()
	h := ov.hwnd
	ov.mu.Unlock()
	if h == 0 {
		return nil
	}
	muCambiaTasti.Lock() // una richiesta alla volta: il risultato è quello di questa richiesta
	defer muCambiaTasti.Unlock()
	tastiDaRegistrare.Store(&t)
	tastiFalliti.Store(nil)
	pSendMessage.Call(h, wmTasti, 0, 0)
	if f := tastiFalliti.Load(); f != nil {
		return *f
	}
	return nil
}

// nomiTasti: i tasti di queste funzioni ("F9, F10"), per dire quali sono occupati.
func nomiTasti(funzioni []string, t map[string]uint32) string {
	var p []string
	for _, f := range funzioni {
		p = append(p, etichettaTasto(t[f]))
	}
	return strings.Join(p, ", ")
}

// nascondiMotore porta la finestra del motore fuori dallo schermo, senza icona nella barra
// delle applicazioni e senza che prenda il fuoco, ed esclusa dalle catture (l'OCR non la legge
// mai). NON è LAYERED/TRANSPARENT: con LAYERED WebView2 non parte. Se Windows la riporta su
// uno schermo (monitor, risoluzione, DPI) la rimette fuori la finestra stessa (FuoriSchermo in
// go-webview2). Si può chiamare più volte.
func nascondiMotore(h uintptr) {
	if h == 0 {
		return
	}
	const gwlExStyle = ^uintptr(19) // -20
	ex, _, _ := pGetWindowLongPtr.Call(h, gwlExStyle)
	// LAYERED | TRANSPARENT | TOOLWINDOW | NOACTIVATE, niente APPWINDOW
	nuovo := (ex | 0x80 | 0x08000000) &^ (0x40000 | 0x80000 | 0x20) // TOOLWINDOW|NOACTIVATE; niente APPWINDOW e niente LAYERED/TRANSPARENT (con LAYERED il motore WebView2 non partiva)
	cambiato := nuovo != ex
	if cambiato {
		pSetWindowLongPtr.Call(h, gwlExStyle, nuovo)
	}
	// SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE|SWP_FRAMECHANGED
	// Fuori dallo schermo ma NON a -32000: quella è la posizione delle finestre ridotte a icona, e
	// Windows disegnava la barretta "Tasso Traduce (motore)" in basso a sinistra (26/11/2026).
	vx, _, _ := pGetSysMetrics.Call(76) // SM_XVIRTUALSCREEN
	vy, _, _ := pGetSysMetrics.Call(77) // SM_YVIRTUALSCREEN
	fx, fy := int32(vx)-4000, int32(vy)-4000
	pSetWindowPos.Call(h, 0, uintptr(fx), uintptr(fy), 0, 0, 0x1|0x4|0x10|0x20)
	if cambiato {
		// la barra delle applicazioni toglie il pulsante solo se la finestra si nasconde e riappare
		pShowWindow.Call(h, 0) // SW_HIDE
	}
	pShowWindow.Call(h, 4)         // SW_SHOWNOACTIVATE: visibile per WebView2, ma fuori dallo schermo
	pSetDisplayAffin.Call(h, 0x11) // WDA_EXCLUDEFROMCAPTURE
}

// nomeTasto: il tasto scelto adesso per una funzione, da scrivere nei messaggi (es. "F7").
func nomeTasto(funzione string) string {
	return etichettaTasto(tastiCompleti(leggiImp().Tasti)[funzione])
}
