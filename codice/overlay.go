package main

// Pannellino in gioco: una scritta piccola e trasparente nell'angolo in alto con CPU, scheda
// video e RAM. È una normale finestra "sempre in primo piano" che non si può cliccare:
// non entra dentro il gioco, quindi gli anti-cheat non hanno niente da dire.
// Si vede nei giochi in finestra o "schermo intero senza bordi".

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	gdi32            = windows.NewLazySystemDLL("gdi32.dll")
	pRegisterClassEx = user32.NewProc("RegisterClassExW")
	pCreateWindowEx  = user32.NewProc("CreateWindowExW")
	pDefWindowProc   = user32.NewProc("DefWindowProcW")
	pGetMessage      = user32.NewProc("GetMessageW")
	pTranslateMsg    = user32.NewProc("TranslateMessage")
	pDispatchMsg     = user32.NewProc("DispatchMessageW")
	pSetLayeredAttr  = user32.NewProc("SetLayeredWindowAttributes")
	pSetTimer        = user32.NewProc("SetTimer")
	pInvalidateRect  = user32.NewProc("InvalidateRect")
	pBeginPaint      = user32.NewProc("BeginPaint")
	pEndPaint        = user32.NewProc("EndPaint")
	pFillRect        = user32.NewProc("FillRect")
	pSetWindowPos    = user32.NewProc("SetWindowPos")
	pGetSysMetrics   = user32.NewProc("GetSystemMetrics")
	pRegisterHotKey  = user32.NewProc("RegisterHotKey")
	pUnregHotKey     = user32.NewProc("UnregisterHotKey")
	pSendMessage     = user32.NewProc("SendMessageW")
	pGetClientRect   = user32.NewProc("GetClientRect")
	pCreateFont      = gdi32.NewProc("CreateFontW")
	pCreateBrush     = gdi32.NewProc("CreateSolidBrush")
	pSelectObject    = gdi32.NewProc("SelectObject")
	pSetBkMode       = gdi32.NewProc("SetBkMode")
	pSetTextColor    = gdi32.NewProc("SetTextColor")
	pTextOut         = gdi32.NewProc("TextOutW")
	pTextExtent      = gdi32.NewProc("GetTextExtentPoint32W")
)

const (
	ovW, ovH   = 560, 30
	ovKey      = 0x00010001 // colore "trasparente" (quasi nero, non usato dal testo)
	wmOvUpdate = 0x0400 + 7
)

type ovPaint struct {
	hdc         uintptr
	fErase      int32
	rcPaint     rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

type ovMsg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	ptX     int32
	ptY     int32
	private uint32
}

type ovClass struct {
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

var (
	ovTick         int
	ovLastSig      string
	pGetWindowLong = user32.NewProc("GetWindowLongW")
	ovMu           sync.Mutex
	ovHwnd         uintptr
	ovInGame       bool
	ovManual       bool // mostrato a mano (tasto in Tasso o Ctrl+Maiusc+F12)
	ovSuppress     bool // nascosto a mano durante questa partita
	ovShown        bool
	ovFont         uintptr
	ovKeyBrush     uintptr
	ovCPUTemp      float64 = -1
	ovOnce         sync.Once
	ovTempStart    sync.Once
)

func startOverlay() {
	ovOnce.Do(func() {
		go overlayThread()
	})
}

func overlayThread() {
	runtime.LockOSThread()
	cls, _ := windows.UTF16PtrFromString("TassoOverlay")
	wc := ovClass{lpfnWndProc: windows.NewCallback(ovWndProc), lpszClassName: cls}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	if r, _, _ := pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return
	}
	exStyle := uintptr(0x80000 | 0x20 | 0x8 | 0x80 | 0x08000000) // LAYERED|TRANSPARENT|TOPMOST|TOOLWINDOW|NOACTIVATE
	h, _, _ := pCreateWindowEx.Call(exStyle, uintptr(unsafe.Pointer(cls)), 0, 0x80000000, 0, 0, ovW, ovH, 0, 0, 0, 0)
	if h == 0 {
		return
	}
	pSetLayeredAttr.Call(h, ovKey, 245, 1|2) // COLORKEY | ALPHA
	face, _ := windows.UTF16PtrFromString("Segoe UI")
	ovFont, _, _ = pCreateFont.Call(19, 0, 0, 0, 700, 0, 0, 0, 1, 0, 0, 3, 0, uintptr(unsafe.Pointer(face)))
	ovKeyBrush, _, _ = pCreateBrush.Call(ovKey)
	pSetTimer.Call(h, 1, 1000, 0)
	pRegisterHotKey.Call(h, 1, 0x0002|0x0004|0x4000, 0x7B) // Ctrl+Maiusc+F12 (senza ripetizione)
	// Il tasto dell'autoclick si registra solo quando apri la pagina Giochi (acArm), così non
	// "ruba" F6 alle altre app se l'autoclick non lo usi.
	ovMu.Lock()
	ovHwnd = h
	ovMu.Unlock()
	ovApply(h)
	var m ovMsg
	for {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		pTranslateMsg.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMsg.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func ovWndProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case 0x000F: // WM_PAINT
		ovPaintNow(hwnd)
		return 0
	case 0x0113: // WM_TIMER
		ovMu.Lock()
		shown, auto := ovShown, ovInGame && !ovManual && !ovSuppress
		ovMu.Unlock()
		if auto && shown != gameInFront() {
			ovApply(hwnd) // sei passato a un'altra finestra (o sei tornato al gioco): nascondo/mostro
			return 0
		}
		if shown {
			// Mi rimetto sopra a tutto solo ogni 4 secondi e solo se non sono già in cima:
			// ri-assegnare "sempre in primo piano" ogni secondo faceva sfarfallare lo schermo.
			ovTick++
			if ovTick%4 == 0 && !ovIsTopmost(hwnd) {
				pSetWindowPos.Call(hwnd, ^uintptr(0), 0, 0, 0, 0, 0x1|0x2|0x10|0x4) // TOPMOST, NOSIZE|NOMOVE|NOACTIVATE|NOREDRAW
			}
			// Ridipingo solo se i numeri sono cambiati davvero (niente sfarfallio).
			if sig := ovSig(); sig != ovLastSig {
				ovLastSig = sig
				pInvalidateRect.Call(hwnd, 0, 0) // 0 = non cancellare lo sfondo prima: meno flicker
			}
		}
		return 0
	case 0x0312: // WM_HOTKEY
		if wp == 2 { // F6: autoclick
			acMu.Lock()
			running := acRunning
			acMu.Unlock()
			if running {
				go autoClickStop()
			} else {
				go autoClickStart(true)
			}
			return 0
		}
		ovMu.Lock()
		if ovShown {
			ovManual, ovSuppress = false, ovInGame
		} else {
			ovManual, ovSuppress = true, false
		}
		ovMu.Unlock()
		ovApply(hwnd)
		return 0
	case wmOvUpdate:
		ovApply(hwnd)
		return 0
	case wmOvUpdate + 1: // registra (o cambia) il tasto dell'autoclick
		pUnregHotKey.Call(hwnd, 2)
		if acRegister(hwnd) {
			return 1
		}
		return 0
	case wmOvUpdate + 2: // libera il tasto dell'autoclick
		pUnregHotKey.Call(hwnd, 2)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wp, lp)
	return r
}

// ovApply decide se mostrare il pannellino e dove (sempre sul thread della finestra).
func ovApply(hwnd uintptr) {
	setMu.Lock()
	auto, pos := settings.OverlayOn && !settings.Paused, settings.OverlayPos
	setMu.Unlock()
	ovMu.Lock()
	show := ovManual || (auto && ovInGame && !ovSuppress && gameInFront())
	ovShown = show
	ovMu.Unlock()
	if !show {
		pShowWin.Call(hwnd, 0) // SW_HIDE
		return
	}
	sw, _, _ := pGetSysMetrics.Call(0) // larghezza dello schermo principale
	x := int32(sw) - ovW - 14
	if pos == "sx" {
		x = 14
	}
	pSetWindowPos.Call(hwnd, ^uintptr(0), uintptr(x), 8, ovW, ovH, 0x10|0x40) // NOACTIVATE|SHOWWINDOW
	pShowWin.Call(hwnd, 4)                                                    // SW_SHOWNOACTIVATE
	pInvalidateRect.Call(hwnd, 0, 1)
	ovTempStart.Do(func() { go cpuTempLoop() })
}

type ovSeg struct {
	text  string
	color uint32
}

func rgb(r, g, b uint32) uint32 { return r | g<<8 | b<<16 }

func tempColor(t float64, warn, hot float64) uint32 {
	switch {
	case t >= hot:
		return rgb(255, 90, 90)
	case t >= warn:
		return rgb(255, 196, 70)
	}
	return rgb(120, 255, 190)
}

func ovSegments() []ovSeg {
	ls := liveStats()
	white := rgb(245, 245, 250)
	var segs []ovSeg
	cpu, _ := ls["cpu"].(float64)
	segs = append(segs, ovSeg{"CPU ", rgb(190, 160, 255)}, ovSeg{fmt.Sprintf("%d%%", int(cpu+0.5)), white})
	ovMu.Lock()
	ct := ovCPUTemp
	ovMu.Unlock()
	if ct > 0 {
		segs = append(segs, ovSeg{fmt.Sprintf(" %d°", int(ct+0.5)), tempColor(ct, 80, 90)})
	}
	if g, ok := ls["gpu"].(map[string]interface{}); ok && g != nil {
		segs = append(segs, ovSeg{"    GPU ", rgb(190, 160, 255)})
		if u, ok := g["util"].(float64); ok {
			segs = append(segs, ovSeg{fmt.Sprintf("%d%%", int(u+0.5)), white})
		}
		if t, ok := g["temp"].(float64); ok {
			segs = append(segs, ovSeg{fmt.Sprintf(" %d°", int(t+0.5)), tempColor(t, 78, 86)})
		}
	}
	if tot, _ := ls["ramTotal"].(uint64); tot > 0 {
		used, _ := ls["ramUsed"].(uint64)
		p := float64(used) * 100 / float64(tot)
		c := white
		if p >= 90 {
			c = rgb(255, 196, 70)
		}
		segs = append(segs, ovSeg{"    RAM ", rgb(190, 160, 255)}, ovSeg{fmt.Sprintf("%d%%", int(p+0.5)), c})
	}
	segs = append(segs, ovSeg{"    " + time.Now().Format("15:04"), rgb(170, 175, 190)})
	return segs
}

func ovPaintNow(hwnd uintptr) {
	var ps ovPaint
	hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var cr rect
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&cr)))
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&cr)), ovKeyBrush)
	pSelectObject.Call(hdc, ovFont)
	pSetBkMode.Call(hdc, 1) // TRANSPARENT
	segs := ovSegments()
	// Larghezza totale per allineare a destra (o a sinistra).
	widths := make([]int32, len(segs))
	var total int32
	for i, s := range segs {
		u, _ := windows.UTF16FromString(s.text)
		var sz struct{ cx, cy int32 }
		pTextExtent.Call(hdc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&sz)))
		widths[i] = sz.cx
		total += sz.cx
	}
	setMu.Lock()
	left := settings.OverlayPos == "sx"
	setMu.Unlock()
	x := cr.Right - total - 6
	if left {
		x = 6
	}
	for i, s := range segs {
		u, _ := windows.UTF16FromString(s.text)
		n := uintptr(len(u) - 1)
		// Contorno scuro per leggerlo su qualsiasi sfondo del gioco.
		pSetTextColor.Call(hdc, uintptr(rgb(0, 0, 0)))
		for _, d := range [][2]int32{{-1, 0}, {1, 0}, {0, -1}, {0, 1}, {1, 1}} {
			pTextOut.Call(hdc, uintptr(x+d[0]), uintptr(4+d[1]), uintptr(unsafe.Pointer(&u[0])), n)
		}
		pSetTextColor.Call(hdc, uintptr(s.color))
		pTextOut.Call(hdc, uintptr(x), 4, uintptr(unsafe.Pointer(&u[0])), n)
		x += widths[i]
	}
}

func ovPost() {
	ovMu.Lock()
	h := ovHwnd
	ovMu.Unlock()
	if h != 0 {
		pPostMessage.Call(h, wmOvUpdate, 0, 0)
	}
}

func overlayGameStarted() {
	ovMu.Lock()
	ovInGame, ovSuppress = true, false
	ovMu.Unlock()
	ovPost()
}

func overlayGameEnded() {
	ovMu.Lock()
	ovInGame, ovSuppress = false, false
	ovMu.Unlock()
	ovPost()
}

func setOverlay(on bool, pos string) string {
	if pos != "sx" {
		pos = "dx"
	}
	updateSettings(func(s *Settings) { s.OverlayOn = on; s.OverlayPos = pos })
	ovPost()
	return "ok"
}

func overlayShow(show bool) string {
	ovMu.Lock()
	ovManual = show
	if !show {
		ovSuppress = ovInGame
	}
	ovMu.Unlock()
	ovPost()
	return "ok"
}

// cpuTempLoop legge la temperatura del processore ogni 20 secondi, solo mentre il pannellino è visibile.
// Windows non la dà sempre: se c'è LibreHardwareMonitor aperto uso quella (più precisa), altrimenti
// il sensore ACPI; se nessuno dei due risponde, il pannellino mostra solo la percentuale.
func cpuTempLoop() {
	for {
		ovMu.Lock()
		shown := ovShown
		ovMu.Unlock()
		if shown {
			r := runPS(cpuTempScript, 30*time.Second, nil)
			v := -1.0
			if f, err := strconv.ParseFloat(strings.TrimSpace(r.JSON), 64); err == nil && f > 15 && f < 115 {
				v = f
			}
			ovMu.Lock()
			ovCPUTemp = v
			ovMu.Unlock()
		}
		time.Sleep(60 * time.Second) // ogni minuto: lanciare PowerShell più spesso darebbe piccoli scatti nel gioco
	}
}

const cpuTempScript = `
$t = $null
try {
  $s = Get-CimInstance -Namespace root/LibreHardwareMonitor -ClassName Sensor -ErrorAction Stop | Where-Object { $_.SensorType -eq 'Temperature' -and ($_.Name -match 'CPU Package|Core \(Tctl|Tctl|Tdie|CPU Total') } | Select-Object -First 1
  if ($s) { $t = [double]$s.Value }
} catch {}
if (-not $t) {
  try {
    $z = Get-CimInstance -Namespace root/wmi -ClassName MSAcpi_ThermalZoneTemperature -ErrorAction Stop | Sort-Object -Property CurrentTemperature -Descending | Select-Object -First 1
    if ($z) { $c = ([double]$z.CurrentTemperature / 10) - 273.15; if ($c -gt 28 -or $c -lt 27) { $t = $c } }
  } catch {}
}
if ($t) { [Console]::Out.WriteLine('JSON ' + ([math]::Round($t, 1)).ToString([Globalization.CultureInfo]::InvariantCulture)) }
`

// acRegister registra il tasto scelto per l'autoclick (di base F6). Va chiamata sul thread del pannellino.
func acRegister(hwnd uintptr) bool {
	setMu.Lock()
	vk, mods := settings.AcVK, settings.AcMods
	setMu.Unlock()
	if vk == 0 {
		vk = 0x75 // F6
	}
	r, _, _ := pRegisterHotKey.Call(hwnd, 2, uintptr(mods&0x7|0x4000), uintptr(vk))
	return r != 0
}

// acArm registra il tasto dell'autoclick (pagina Giochi aperta). Restituisce false se è già usato da altri.
func acArm() bool {
	setMu.Lock()
	paused := settings.Paused
	setMu.Unlock()
	ovMu.Lock()
	h := ovHwnd
	ovMu.Unlock()
	if h == 0 || paused {
		return false
	}
	r, _, _ := pSendMessage.Call(h, wmOvUpdate+1, 0, 0)
	return r != 0
}

func acDisarm() {
	ovMu.Lock()
	h := ovHwnd
	ovMu.Unlock()
	if h != 0 {
		pSendMessage.Call(h, wmOvUpdate+2, 0, 0)
	}
}

func setAutoClickKey(vk, mods uint32) string {
	if vk == 0 || vk > 0xFE || vk == 0x1B || vk == 0x0D || vk == 0x5B || vk == 0x5C { // niente Esc, Invio, tasto Windows
		return "Questo tasto non si può usare: scegline un altro."
	}
	single := (vk >= 0x70 && vk <= 0x87) || vk == 0x13 || vk == 0x91 || vk == 0x2D // F1-F24, Pausa, Bloc Scorr, Ins
	if mods&0x7 == 0 && !single {
		return "Da solo questo tasto ti servirebbe nelle altre app: usa un tasto F (F1-F12) oppure aggiungi Ctrl, Alt o Maiusc."
	}
	setMu.Lock()
	oldVK, oldMods := settings.AcVK, settings.AcMods
	setMu.Unlock()
	updateSettings(func(s *Settings) { s.AcVK, s.AcMods = vk, mods&0x7 })
	ovMu.Lock()
	h := ovHwnd
	ovMu.Unlock()
	if h == 0 {
		return "ok"
	}
	if r, _, _ := pSendMessage.Call(h, wmOvUpdate+1, 0, 0); r == 0 {
		updateSettings(func(s *Settings) { s.AcVK, s.AcMods = oldVK, oldMods })
		pSendMessage.Call(h, wmOvUpdate+1, 0, 0)
		return "Questo tasto è già usato da un altro programma: scegline un altro."
	}
	return "ok"
}

// gameInFront: il pannellino automatico si vede solo quando il gioco è la finestra davanti,
// così non resta sopra Chrome o Discord se esci un attimo dal gioco.
func gameInFront() bool {
	pid := gamePid.Load()
	if pid == 0 {
		return false
	}
	fg, _, _ := pGetFgWindow.Call()
	if fg == 0 {
		return false
	}
	var fpid uint32
	pGetWindowPid.Call(fg, uintptr(unsafe.Pointer(&fpid)))
	return fpid == pid
}

// ovIsTopmost dice se la finestra ha già il flag "sempre in primo piano" (WS_EX_TOPMOST = 0x8).
func ovIsTopmost(hwnd uintptr) bool {
	ex, _, _ := pGetWindowLong.Call(hwnd, ^uintptr(19)) // GWL_EXSTYLE = -20
	return ex&0x8 != 0
}

// ovSig è una firma dei numeri mostrati: se non cambia, non ridipingo (niente sfarfallio).
func ovSig() string {
	var b strings.Builder
	for _, s := range ovSegments() {
		b.WriteString(s.text)
	}
	return b.String()
}
