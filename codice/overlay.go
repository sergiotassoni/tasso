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
	ovTick                 int
	ovLastSig              string
	pGetWindowLong         = user32.NewProc("GetWindowLongW")
	pSetWndDisplayAffinity = user32.NewProc("SetWindowDisplayAffinity")
	ovMu                   sync.Mutex
	ovHwnd                 uintptr
	ovInGame               bool
	ovManual               bool // mostrato a mano (tasto in Tasso o Ctrl+Maiusc+F12)
	ovSuppress             bool // nascosto a mano durante questa partita
	ovShown                bool
	ovFont                 uintptr
	ovKeyBrush             uintptr
	ovCPUTemp              float64 = -1
	ovOnce                 sync.Once
	ovTempStart            sync.Once
	ovHotkey               bool // Ctrl+Maiusc+F12 registrato (solo sul thread del pannellino)
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
	// Come le finestre di Tasso Traduce: il pannellino non compare nelle catture dello schermo
	// (Windows 10 2004+), così la traduzione dello schermo non legge i suoi numeri.
	pSetWndDisplayAffinity.Call(h, 0x11) // WDA_EXCLUDEFROMCAPTURE
	face, _ := windows.UTF16PtrFromString("Segoe UI")
	ovFont, _, _ = pCreateFont.Call(19, 0, 0, 0, 700, 0, 0, 0, 1, 0, 0, 3, 0, uintptr(unsafe.Pointer(face)))
	ovKeyBrush, _, _ = pCreateBrush.Call(ovKey)
	pSetTimer.Call(h, 1, 1000, 0)
	// Ctrl+Maiusc+F12 si registra in ovApply solo in Gaming con il pannellino acceso (in Word è Stampa).
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
	case wmOvUpdate + 3: // tasto del pannellino cambiato (tasti_rapidi.go)
		return ovKeyRegister(hwnd)
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wp, lp)
	return r
}

// ovApply decide se mostrare il pannellino e dove (sempre sul thread della finestra).
func ovApply(hwnd uintptr) {
	setMu.Lock()
	office := settings.Profile == "ufficio"
	paused := settings.Paused
	auto, pos := settings.OverlayOn && !paused && !office, settings.OverlayPos
	setMu.Unlock()
	ovMu.Lock()
	manuale := ovManual
	ovMu.Unlock()
	// Il tasto Ctrl+Maiusc+F12 (o quello scelto) è di Tasso solo quando serve: pannellino acceso da solo o aperto a
	// mano con "Mostra ora" (così si chiude anche dal gioco), mai in Ufficio né in pausa.
	vuoleTasto := (auto || manuale) && !office && !paused
	if vuoleTasto != ovHotkey {
		if vuoleTasto {
			// di base Ctrl+Maiusc+F12 (tasti_rapidi.go)
			c := ovKeyCode()
			r, _, _ := pRegisterHotKey.Call(hwnd, 1, uintptr(c>>16)|0x4000, uintptr(c&0xFFFF)) // senza ripetizione
			ovHotkey = r != 0
		} else {
			pUnregHotKey.Call(hwnd, 1)
			ovHotkey = false
		}
	}
	ovMu.Lock()
	if office {
		ovManual = false // in Ufficio niente pannellino, neanche chiesto a mano
	}
	show := ovManual || (auto && ovInGame && !ovSuppress && gameInFront())
	ovShown = show
	ovMu.Unlock()
	if !show {
		pShowWin.Call(hwnd, 0) // SW_HIDE
		return
	}
	sw, _, _ := pGetSysMetrics.Call(0) // larghezza dello schermo principale
	sh, _, _ := pGetSysMetrics.Call(1) // altezza
	x := int32(sw) - ovW - 14
	if ovLeft(pos) {
		x = 14
	}
	y := int32(8)
	if ovBottom(pos) {
		y = int32(sh) - ovH - 8
	}
	pSetWindowPos.Call(hwnd, ^uintptr(0), uintptr(x), uintptr(y), ovW, ovH, 0x10|0x40) // NOACTIVATE|SHOWWINDOW
	pShowWin.Call(hwnd, 4)                                                             // SW_SHOWNOACTIVATE
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
	white, lab := rgb(245, 245, 250), rgb(190, 160, 255)
	on := ovItems()
	var segs []ovSeg
	sep := func() {
		if len(segs) > 0 {
			segs = append(segs, ovSeg{"    ", white})
		}
	}
	if on["cpu"] || on["cputemp"] {
		ovMu.Lock()
		ct := ovCPUTemp
		ovMu.Unlock()
		if on["cpu"] || ct > 0 {
			sep()
			segs = append(segs, ovSeg{"CPU ", lab})
		}
		if on["cpu"] {
			cpu, _ := ls["cpu"].(float64)
			segs = append(segs, ovSeg{fmt.Sprintf("%d%%", int(cpu+0.5)), white})
		}
		if on["cputemp"] && ct > 0 {
			segs = append(segs, ovSeg{strings.Repeat(" ", b2i(on["cpu"])) + fmt.Sprintf("%d°", int(ct+0.5)), tempColor(ct, 80, 90)})
		}
	}
	if g, ok := ls["gpu"].(map[string]interface{}); ok && g != nil && (on["gpu"] || on["gputemp"]) {
		u, uok := g["util"].(float64)
		t, tok := g["temp"].(float64)
		if (on["gpu"] && uok) || (on["gputemp"] && tok) {
			sep()
			segs = append(segs, ovSeg{"GPU ", lab})
			if on["gpu"] && uok {
				segs = append(segs, ovSeg{fmt.Sprintf("%d%%", int(u+0.5)), white})
			}
			if on["gputemp"] && tok {
				segs = append(segs, ovSeg{strings.Repeat(" ", b2i(on["gpu"])) + fmt.Sprintf("%d°", int(t+0.5)), tempColor(t, 78, 86)})
			}
		}
	}
	if tot, _ := ls["ramTotal"].(uint64); tot > 0 && on["ram"] {
		used, _ := ls["ramUsed"].(uint64)
		p := float64(used) * 100 / float64(tot)
		c := white
		if p >= 90 {
			c = rgb(255, 196, 70)
		}
		sep()
		segs = append(segs, ovSeg{"RAM ", lab}, ovSeg{fmt.Sprintf("%d%%", int(p+0.5)), c})
	}
	if on["ora"] {
		sep()
		segs = append(segs, ovSeg{time.Now().Format("15:04"), rgb(170, 175, 190)})
	}
	if len(segs) == 0 {
		segs = append(segs, ovSeg{"Tasso", lab})
	}
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
	left := ovLeft(settings.OverlayPos)
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
	if pos != "sx" && pos != "sxb" && pos != "dxb" {
		pos = "dx"
	}
	updateSettings(func(s *Settings) { s.OverlayOn = on; s.OverlayPos = pos })
	ovPost()
	return "ok"
}

func overlayShow(show bool) string {
	if show {
		// Stessi controlli di ovApply: in Ufficio il pannellino non si mostra, neanche a mano.
		setMu.Lock()
		office := settings.Profile == "ufficio"
		setMu.Unlock()
		if office {
			return "In modalità Ufficio il pannellino non si mostra."
		}
		ovMu.Lock()
		pronto := ovHwnd != 0
		ovMu.Unlock()
		if !pronto {
			return "Il pannellino non è ancora pronto: riprova tra poco."
		}
	}
	ovMu.Lock()
	ovManual = show
	if !show {
		ovSuppress = ovInGame
	}
	ovMu.Unlock()
	ovPost()
	return "ok"
}

// cpuTempLoop legge la temperatura del processore: ogni minuto mentre il pannellino è visibile,
// ogni 15 secondi mentre la pagina Hardware è davanti (Tasso in primo piano, mai durante un gioco).
// Windows non la dà sempre: se c'è LibreHardwareMonitor aperto uso quella (l'unica che è davvero il
// processore); altrimenti il sensore ACPI, che di solito è la scheda madre: la pagina Hardware la mostra
// come "Sensore scheda madre", il pannellino no. Dopo 3 letture vuote (o ACPI sempre uguale) smetto per
// tutta la sessione: riprovo solo se si apre LibreHardwareMonitor (controllo i processi, senza PowerShell).
func cpuTempLoop() {
	var last, lhmCheck time.Time
	var acpiLast float64
	acpiSame, empty := 0, 0
	stopped := false
	lhmWas := false
	src := ""
	for {
		ovMu.Lock()
		shown := ovShown
		watch := time.Since(hwWatchAt) < 30*time.Second && !ovInGame
		ovMu.Unlock()
		if stopped || src == "A" {
			// niente letture inutili: controllo solo (ogni 10 secondi) se è comparso LibreHardwareMonitor
			if time.Since(lhmCheck) >= 10*time.Second {
				lhmCheck = time.Now()
				lhm := exeRunning([]string{"LibreHardwareMonitor.exe"})
				if lhm && !lhmWas {
					stopped, empty, acpiSame, acpiLast, src = false, 0, 0, 0, ""
					last = time.Time{}
				}
				lhmWas = lhm
			}
			if stopped {
				time.Sleep(2 * time.Second)
				continue
			}
		}
		if watch && !shown && time.Since(last) >= 15*time.Second {
			bMu.Lock()
			partita := bGame != ""
			bMu.Unlock()
			if partita || runningGame() != "" {
				watch = false // un gioco è aperto: niente PowerShell per la pagina Hardware
			}
		}
		if watch && wv != nil {
			if fg, _, _ := pGetFgWindow.Call(); fg != uintptr(wv.Window()) {
				watch = false // Tasso non è in primo piano: nessuno guarda la pagina Hardware
			}
		}
		if shown && !watch && (!ovItems()["cputemp"] || src == "A") {
			// il pannellino non mostra la temperatura (o c'è solo il sensore della scheda madre,
			// che lì non mostro come CPU): niente PowerShell
			time.Sleep(2 * time.Second)
			continue
		}
		// Nel gioco ogni minuto (lanciare PowerShell più spesso darebbe piccoli scatti);
		// sulla pagina Hardware ogni 15 secondi, così vedi la temperatura dal vivo.
		every := 60 * time.Second
		if watch {
			every = 15 * time.Second // la pagina Hardware è davanti: lettura fresca anche col pannellino acceso
		}
		if (shown || watch) && time.Since(last) >= every {
			last = time.Now()
			r := runPS(cpuTempScript, 30*time.Second, nil)
			v := -1.0
			raw := strings.TrimSpace(r.JSON)
			s := ""
			if strings.HasPrefix(raw, "A") || strings.HasPrefix(raw, "L") {
				s = raw[:1]
			}
			raw = strings.TrimLeft(raw, "AL")
			if f, err := strconv.ParseFloat(raw, 64); err == nil && f > 15 && f < 115 {
				v = f
			}
			// il sensore ACPI su molti PC è la scheda madre e dà sempre lo stesso numero:
			// se resta identico per 3 letture di fila non è un sensore vero, non lo mostro
			if s == "A" && v > 0 {
				if v == acpiLast {
					acpiSame++
				} else {
					acpiLast, acpiSame = v, 1
				}
				if acpiSame >= 3 {
					v = -1
				}
			} else {
				acpiLast, acpiSame = 0, 0
			}
			if v > 0 {
				empty = 0
				src = s
			} else {
				empty++
				src = ""
				if empty >= 3 || acpiSame >= 3 {
					stopped = true // 3 letture vuote o ferme: per questa sessione basta PowerShell
					lhmWas = exeRunning([]string{"LibreHardwareMonitor.exe"})
					lhmCheck = time.Now()
				}
			}
			ovMu.Lock()
			ovCPUTemp = -1
			ovCPUTempSrc = s
			if s == "L" {
				ovCPUTemp = v // solo LibreHardwareMonitor è davvero il processore
			}
			hwBoardTemp = -1
			if s == "A" {
				hwBoardTemp = v
			}
			ovCPUTempAt = time.Now()
			ovMu.Unlock()
		}
		time.Sleep(2 * time.Second)
	}
}

// hwWatchAt: l'ultima volta che la pagina Hardware era aperta (per leggere la temperatura del processore).
var hwWatchAt time.Time

// ovCPUTempAt: quando è stata letta ovCPUTemp (una lettura vecchia non si mostra come attuale).
var ovCPUTempAt time.Time

// ovCPUTempSrc: da dove viene l'ultima lettura ("L" LibreHardwareMonitor, "A" sensore ACPI, "" nessuna).
// hwBoardTemp: la temperatura del sensore ACPI (di solito la scheda madre, non il processore).
var (
	ovCPUTempSrc string
	hwBoardTemp  float64 = -1
)

// hwWatch: la pagina Hardware è aperta, quindi leggo anche la temperatura del processore.
// cpuTemp c'è solo se viene da LibreHardwareMonitor; il sensore ACPI arriva come boardTemp.
func hwWatch() map[string]interface{} {
	ovMu.Lock()
	hwWatchAt = time.Now()
	t, b, src := ovCPUTemp, hwBoardTemp, ovCPUTempSrc
	if time.Since(ovCPUTempAt) > 25*time.Second {
		t, b, src = -1, -1, ""
	}
	ovMu.Unlock()
	ovTempStart.Do(func() { go cpuTempLoop() })
	out := map[string]interface{}{}
	if t > 0 && src == "L" {
		out["cpuTemp"] = t
		out["src"] = "L"
	} else if b > 0 && src == "A" {
		out["boardTemp"] = b
		out["src"] = "A"
	}
	return out
}

const cpuTempScript = `
$t = $null
try {
  $s = Get-CimInstance -Namespace root/LibreHardwareMonitor -ClassName Sensor -ErrorAction Stop | Where-Object { $_.SensorType -eq 'Temperature' -and ($_.Name -match 'CPU Package|Core \(Tctl|Tctl|Tdie|CPU Total') } | Select-Object -First 1
  if ($s) { $t = [double]$s.Value; $src = 'L' }
} catch {}
if (-not $t) {
  try {
    $z = Get-CimInstance -Namespace root/wmi -ClassName MSAcpi_ThermalZoneTemperature -ErrorAction Stop | Sort-Object -Property CurrentTemperature -Descending | Select-Object -First 1
    if ($z) { $c = ([double]$z.CurrentTemperature / 10) - 273.15; if ($c -gt 28 -or $c -lt 27) { $t = $c; $src = 'A' } }
  } catch {}
}
if ($t) { [Console]::Out.WriteLine('JSON ' + $src + ([math]::Round($t, 1)).ToString([Globalization.CultureInfo]::InvariantCulture)) }
`

// acRegister registra il tasto scelto per l'autoclick (di base F6). Va chiamata sul thread del pannellino.
func acRegister(hwnd uintptr) bool {
	setMu.Lock()
	vk, mods := settings.AcVK, settings.AcMods
	setMu.Unlock()
	if vk == 0 {
		vk = 0x75 // F6
	}
	r, _, _ := pRegisterHotKey.Call(hwnd, 2, uintptr(mods&0xF|0x4000), uintptr(vk))
	return r != 0
}

// acArm registra il tasto dell'autoclick (pagina Giochi aperta). Restituisce false se è già usato da altri.
func acArm() bool {
	setMu.Lock()
	paused := settings.Paused || settings.Profile == "ufficio" // in Ufficio F6 resta alle altre app
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
	// stesse regole di tutti i tasti rapidi di Tasso (tasti_rapidi.go, traduce/tasti.go): niente
	// Esc, lettere da sole, combinazioni di Windows, né un tasto già usato da un'altra funzione
	if vk == 0x0D {
		return "Questo tasto non si può usare: scegline un altro."
	}
	if m := controllaTastoTasso((mods&0xF)<<16|vk, nomeTastoAc); m != "" {
		return m
	}
	setMu.Lock()
	oldVK, oldMods := settings.AcVK, settings.AcMods
	setMu.Unlock()
	updateSettings(func(s *Settings) { s.AcVK, s.AcMods = vk, mods&0xF })
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

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
