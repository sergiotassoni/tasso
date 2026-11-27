package main

// Controllo FPS (3.1): cerca i problemi che fanno perdere MOLTI FPS (anche più del 20%) e dice
// onestamente quanto si guadagna sistemandoli. Le regolazioni di Windows del Boost danno di solito
// 0-10%; i guadagni grandi vengono da qui, ma solo se il PC ha quel problema:
//   1. schermo a 60 Hz quando può andare a 144/165       -> il Tasso lo sistema (con 15 s per confermare)
//   2. portatile: gioco sulla scheda video integrata     -> il Tasso mette quella potente per il gioco
//   3. gioco bloccato da V-Sync o da un limite di FPS     -> visto dalla misura, si cambia nel gioco
//   4. scheda video al limite senza DLSS/FSR/XeSS         -> visto dalla misura, si cambia nel gioco
//   5. RAM lenta (XMP/EXPO spento) o un solo banco        -> si cambia nel BIOS, il Tasso spiega come
//   6. processore o scheda video troppo caldi              -> avviso
//   7. driver della scheda video vecchio                   -> apre la pagina ufficiale
//   + portatile a batteria.
// Il Tasso non cambia mai da solo il gioco né il BIOS.

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type fpsFinding struct {
	ID     string `json:"id"`
	Sev    string `json:"sev"`    // "big" = guadagno grande, "mid", "info"
	Title  string `json:"title"`  // il problema, in parole semplici
	Why    string `json:"why"`    // perché conta
	Gain   string `json:"gain"`   // guadagno realistico
	Fix    string `json:"fix"`    // cosa fare
	Action string `json:"action"` // "hz", "gpu", "tune", "driver:<id>", "" = solo consiglio
}

var (
	lastFpsMu sync.Mutex
	lastFps   *fpsResult
)

func setLastFps(r *fpsResult) {
	lastFpsMu.Lock()
	lastFps = r
	lastFpsMu.Unlock()
}

func getLastFps() *fpsResult {
	lastFpsMu.Lock()
	defer lastFpsMu.Unlock()
	return lastFps
}

// ---------------- Schermo ----------------

var (
	pEnumDisplayDevices  = user32.NewProc("EnumDisplayDevicesW")
	pEnumDisplaySettings = user32.NewProc("EnumDisplaySettingsW")
	pChangeDisplay       = user32.NewProc("ChangeDisplaySettingsExW")
)

type displayDevice struct {
	Cb           uint32
	DeviceName   [32]uint16
	DeviceString [128]uint16
	StateFlags   uint32
	DeviceID     [128]uint16
	DeviceKey    [128]uint16
}

type devMode struct {
	DeviceName                                    [32]uint16
	SpecVersion, DriverVersion, Size, DriverExtra uint16
	Fields                                        uint32
	PositionX, PositionY                          int32
	DisplayOrientation, DisplayFixedOutput        uint32
	Color, Duplex, YResolution, TTOption, Collate int16
	FormName                                      [32]uint16
	LogPixels                                     uint16
	BitsPerPel, PelsWidth, PelsHeight             uint32
	DisplayFlags, DisplayFrequency                uint32
	ICMMethod, ICMIntent, MediaType, DitherType   uint32
	Reserved1, Reserved2                          uint32
	PanningWidth, PanningHeight                   uint32
}

type screenInfo struct {
	Dev     string `json:"dev"`
	Name    string `json:"name"`
	W       uint32 `json:"w"`
	H       uint32 `json:"h"`
	Hz      int    `json:"hz"`
	MaxHz   int    `json:"maxHz"`
	Primary bool   `json:"primary"`
}

// screens: per ogni schermo acceso la frequenza attuale e la più alta possibile alla stessa
// risoluzione (solo modi non interlacciati che il driver dichiara).
func screens() []screenInfo {
	var out []screenInfo
	for i := uint32(0); i < 16; i++ {
		var dd displayDevice
		dd.Cb = uint32(unsafe.Sizeof(dd))
		if r, _, _ := pEnumDisplayDevices.Call(0, uintptr(i), uintptr(unsafe.Pointer(&dd)), 0); r == 0 {
			break
		}
		if dd.StateFlags&0x1 == 0 { // non collegato al desktop
			continue
		}
		cur := devMode{}
		cur.Size = uint16(unsafe.Sizeof(cur))
		if r, _, _ := pEnumDisplaySettings.Call(uintptr(unsafe.Pointer(&dd.DeviceName[0])), 0xFFFFFFFF, uintptr(unsafe.Pointer(&cur))); r == 0 {
			continue
		}
		s := screenInfo{Dev: windows.UTF16ToString(dd.DeviceName[:]), Name: windows.UTF16ToString(dd.DeviceString[:]),
			W: cur.PelsWidth, H: cur.PelsHeight, Hz: int(cur.DisplayFrequency), Primary: dd.StateFlags&0x4 != 0}
		s.MaxHz = s.Hz
		for m := uint32(0); m < 2000; m++ {
			dm := devMode{}
			dm.Size = uint16(unsafe.Sizeof(dm))
			if r, _, _ := pEnumDisplaySettings.Call(uintptr(unsafe.Pointer(&dd.DeviceName[0])), uintptr(m), uintptr(unsafe.Pointer(&dm))); r == 0 {
				break
			}
			if dm.PelsWidth == cur.PelsWidth && dm.PelsHeight == cur.PelsHeight && dm.BitsPerPel == cur.BitsPerPel &&
				dm.DisplayFlags&0x2 == 0 && int(dm.DisplayFrequency) > s.MaxHz && dm.DisplayFrequency <= 1000 { // anche 540 e 600 Hz
				s.MaxHz = int(dm.DisplayFrequency)
			}
		}
		out = append(out, s)
	}
	return out
}

var (
	pMonitorFromWindow = user32.NewProc("MonitorFromWindow")
	pGetMonitorInfo    = user32.NewProc("GetMonitorInfoW")
)

// gameScreenHz: frequenza dello schermo dove c'è la finestra davanti (il gioco, alla fine della misura).
func gameScreenHz() int {
	h, _, _ := pForegroundWin.Call()
	if h != 0 {
		mon, _, _ := pMonitorFromWindow.Call(h, 2) // MONITOR_DEFAULTTONEAREST
		if mon != 0 {
			var mi struct {
				Size          uint32
				Monitor, Work [4]int32
				Flags         uint32
				Device        [32]uint16
			}
			mi.Size = uint32(unsafe.Sizeof(mi))
			if r, _, _ := pGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi))); r != 0 {
				var dm devMode
				dm.Size = uint16(unsafe.Sizeof(dm))
				if r, _, _ := pEnumDisplaySettings.Call(uintptr(unsafe.Pointer(&mi.Device[0])), 0xFFFFFFFF, uintptr(unsafe.Pointer(&dm))); r != 0 {
					return int(dm.DisplayFrequency)
				}
			}
		}
	}
	for _, s := range screens() {
		if s.Primary {
			return s.Hz
		}
	}
	return 0
}

// hzWorth: lo stesso criterio per il controllo e per "Sistemalo": almeno 15 Hz in più e oltre i 60.
func hzWorth(s screenInfo) bool { return s.Hz > 0 && s.MaxHz >= s.Hz+15 && s.MaxHz > 60 }

var (
	hzMu    sync.Mutex
	hzUndo  map[string]int // schermo -> frequenza di prima, finché non confermi
	hzTimer *time.Timer
)

// setScreenHz cambia la frequenza di uno schermo. save=false: solo per adesso (se il Tasso si
// chiude o il PC si riavvia torna quella salvata); save=true: la salva in Windows.
func setScreenHz(dev string, hz int, save bool) bool {
	var dm devMode
	dm.Size = uint16(unsafe.Sizeof(dm))
	name, err := windows.UTF16PtrFromString(dev)
	if err != nil {
		return false
	}
	if r, _, _ := pEnumDisplaySettings.Call(uintptr(unsafe.Pointer(name)), 0xFFFFFFFF, uintptr(unsafe.Pointer(&dm))); r == 0 {
		return false
	}
	dm.DisplayFrequency = uint32(hz)
	dm.Fields = 0x400000 | 0x80000 | 0x100000                                                                           // frequenza, larghezza, altezza
	if r, _, _ := pChangeDisplay.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&dm)), 0, 0x2, 0); r != 0 { // CDS_TEST
		return false
	}
	flags := uintptr(0)
	if save {
		flags = 0x1 // CDS_UPDATEREGISTRY
	}
	r, _, _ := pChangeDisplay.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&dm)), 0, flags, 0)
	return r == 0
}

// screenHzMax prova la frequenza più alta sugli schermi che ne hanno bisogno, SENZA salvarla.
// Se non confermi entro 15 secondi (per esempio perché lo schermo è nero) torna com'era; se il
// Tasso si chiude prima, la frequenza salvata in Windows è ancora quella vecchia.
func screenHzMax() {
	if onBattery() {
		emit("onFpsHzTry", map[string]interface{}{"changed": []string{}, "msg": "Sei a batteria: molti portatili tengono 60 Hz apposta per consumare meno. Collega il caricatore e riprova."})
		return
	}
	hzMu.Lock()
	if hzUndo == nil {
		hzUndo = map[string]int{}
	}
	var done []string
	for _, s := range screens() {
		if !hzWorth(s) {
			continue
		}
		if setScreenHz(s.Dev, s.MaxHz, false) {
			if _, already := hzUndo[s.Dev]; !already {
				hzUndo[s.Dev] = s.Hz
			}
			done = append(done, strconv.Itoa(s.Hz)+" → "+strconv.Itoa(s.MaxHz)+" Hz")
		}
	}
	if len(done) > 0 {
		if hzTimer != nil {
			hzTimer.Stop()
		}
		hzTimer = time.AfterFunc(15*time.Second, func() { screenHzUndo(true) })
	}
	hzMu.Unlock()
	emit("onFpsHzTry", map[string]interface{}{"changed": done})
}

// screenHzUndo rimette subito la frequenza di prima (pulsante "Torna com'era" o tempo scaduto).
func screenHzUndo(timeout bool) {
	hzMu.Lock()
	undo := hzUndo
	hzUndo = nil
	if hzTimer != nil {
		hzTimer.Stop()
		hzTimer = nil
	}
	hzMu.Unlock()
	for dev, hz := range undo {
		setScreenHz(dev, hz, false)
	}
	if len(undo) > 0 {
		emit("onFpsHz", map[string]interface{}{"kept": false, "timeout": timeout})
	}
}

// screenHzUndoAllaChiusura: Tasso si chiude durante la prova dei 15 secondi. Il cambio fatto da
// Windows resta anche dopo la chiusura: rimetto la frequenza di prima (senza parlare con la finestra).
func screenHzUndoAllaChiusura() {
	hzMu.Lock()
	undo := hzUndo
	hzUndo = nil
	if hzTimer != nil {
		hzTimer.Stop()
		hzTimer = nil
	}
	hzMu.Unlock()
	for dev, hz := range undo {
		setScreenHz(dev, hz, false)
	}
}

// vicinoAlLimite: gli FPS sono quasi uguali ai Hz dello schermo o a un limite tipico (V-Sync,
// limite FPS): allora la scheda video "aspetta" il limite, non il processore.
func vicinoAlLimite(avg float64, hz int) bool {
	for _, c := range []float64{float64(hz), 30, 60, 72, 75, 90, 100, 120} {
		if c > 0 && math.Abs(avg-c)/c < 0.03 {
			return true
		}
	}
	return false
}

// hzTenuto: in questa sessione hai confermato con "Tengo" una frequenza nuova dello schermo
// (resta anche dopo "Rimetti com'era" del Boost: è una scelta tua).
var hzTenuto atomic.Bool

// screenHzKeep: hai confermato che lo schermo si vede bene: ora la salvo in Windows.
func screenHzKeep() {
	hzMu.Lock()
	undo := hzUndo
	hzUndo = nil
	if hzTimer != nil {
		hzTimer.Stop()
		hzTimer = nil
	}
	hzMu.Unlock()
	if len(undo) == 0 {
		return // tempo già scaduto: è già tornato com'era
	}
	hzTenuto.Store(true)
	for _, s := range screens() {
		if _, ok := undo[s.Dev]; ok {
			setScreenHz(s.Dev, s.Hz, true)
		}
	}
	emit("onFpsHz", map[string]interface{}{"kept": true})
}

// ---------------- Temperature e memoria video ----------------

// Solo LibreHardwareMonitor: il sensore ACPI di Windows spesso non è il processore.
const cpuTempLHMScript = `
try {
  $s = Get-CimInstance -Namespace root/LibreHardwareMonitor -ClassName Sensor -ErrorAction Stop | Where-Object { $_.SensorType -eq 'Temperature' -and ($_.Name -match 'CPU Package|Core \(Tctl|Tctl|Tdie') } | Select-Object -First 1
  if ($s) { [Console]::Out.WriteLine('JSON ' + ([math]::Round([double]$s.Value, 1)).ToString([Globalization.CultureInfo]::InvariantCulture)) }
} catch {}
`

type tempRead struct {
	cpu, gpu, vram float64
	throttle       bool
}

// readTemps: temperature e memoria video lette verso la fine della misura (in parallelo, così la
// misura non si allunga). 0 = non disponibile.
func readTemps() tempRead {
	var t tempRead
	r := runPS(cpuTempLHMScript, 8*time.Second, nil)
	if f, err := strconv.ParseFloat(strings.TrimSpace(r.JSON), 64); err == nil && f > 15 && f < 115 {
		t.cpu = f
	}
	sampleGPUNow()
	liveMu.Lock()
	if g := gpuFresh(); g != nil {
		if v, ok := g["temp"].(float64); ok {
			t.gpu = v
		}
		u, ok1 := g["memUsed"].(float64)
		tot, ok2 := g["memTot"].(float64)
		if ok1 && ok2 && tot > 0 {
			t.vram = math.Round(u / tot * 100)
		}
	}
	liveMu.Unlock()
	if nvsmi != "" {
		for _, q := range []string{"clocks_event_reasons.hw_thermal_slowdown,clocks_event_reasons.sw_thermal_slowdown", "clocks_throttle_reasons.hw_thermal_slowdown,clocks_throttle_reasons.sw_thermal_slowdown"} {
			out, err := runHiddenTimeout(3*time.Second, nvsmi, "--query-gpu="+q, "--format=csv,noheader")
			if err != nil || strings.Contains(strings.ToLower(out), "field") {
				continue
			}
			for _, f := range strings.Split(strings.Split(strings.TrimSpace(out), "\n")[0], ",") {
				if strings.TrimSpace(strings.ToLower(f)) == "active" {
					t.throttle = true
				}
			}
			break
		}
	}
	return t
}

// ---------------- RAM, schede video e driver (PowerShell, una volta) ----------------

const fpsCheckScript = `
$mem = @(Get-CimInstance Win32_PhysicalMemory -ErrorAction SilentlyContinue)
$gpus = @(Get-CimInstance Win32_VideoController -ErrorAction SilentlyContinue)
$bat = @(Get-CimInstance Win32_Battery -ErrorAction SilentlyContinue)
$o = [ordered]@{
  laptop = ($bat.Count -gt 0)
  ram = @($mem | ForEach-Object { [ordered]@{ type = [int]$_.SMBIOSMemoryType; speed = [int]$_.Speed; conf = [int]$_.ConfiguredClockSpeed; size = [long]$_.Capacity; part = ([string]$_.PartNumber).Trim() } })
  gpus = @($gpus | ForEach-Object { [ordered]@{ name = [string]$_.Name; date = $(if ($_.DriverDate) { $_.DriverDate.ToString('yyyy-MM-dd') } else { '' }) } })
}
[Console]::Out.WriteLine('JSON ' + ($o | ConvertTo-Json -Compress -Depth 4))
`

type pcFacts struct {
	Laptop bool `json:"laptop"`
	RAM    []struct {
		Type  int    `json:"type"`
		Speed int    `json:"speed"`
		Conf  int    `json:"conf"`
		Size  int64  `json:"size"`
		Part  string `json:"part"`
	} `json:"ram"`
	GPUs []struct {
		Name string `json:"name"`
		Date string `json:"date"`
	} `json:"gpus"`
}

func discreteGPU(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "nvidia") || strings.Contains(n, "radeon rx") || strings.Contains(n, "radeon pro") ||
		strings.Contains(n, "arc(tm) a") || strings.Contains(n, "arc a") || strings.Contains(n, "arc(tm) b") || strings.Contains(n, "arc b")
}

// integratedGPU: solo le vere schede video integrate nel processore (non quelle virtuali di
// Parsec, Desktop remoto, DisplayLink...).
func integratedGPU(name string) bool {
	n := strings.ToLower(name)
	if discreteGPU(name) {
		return false
	}
	return strings.Contains(n, "intel") && (strings.Contains(n, "uhd") || strings.Contains(n, "iris") || strings.Contains(n, "hd graphics") || strings.Contains(n, "arc(tm) graphics") || strings.Contains(n, "arc graphics")) ||
		strings.Contains(n, "amd radeon(tm) graphics") || strings.Contains(n, "amd radeon graphics") || strings.Contains(n, "radeon(tm) vega") ||
		regexpRadeonM.MatchString(n)
}

var regexpRadeonM = regexp.MustCompile(`radeon\(tm\)? \d{3}m\b|radeon \d{3}m\b`)

// ramRated: velocità scritta nel codice del banco (es. F4-3600C16, CMK16GX4M2B3200C16), se c'è.
var ramSpeedRe = regexp.MustCompile(`\d{4}`)

func ramRated(part string) int {
	best := 0
	for _, m := range ramSpeedRe.FindAllString(part, -1) {
		v, _ := strconv.Atoi(m)
		if v >= 2400 && v <= 8800 && v%100 == 0 || v == 2666 || v == 2933 || v == 3466 {
			if v > best {
				best = v
			}
		}
	}
	return best
}

func gpuVendorPage(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "nvidia"):
		return "nvidia"
	case strings.Contains(n, "amd") || strings.Contains(n, "radeon"):
		return "amd"
	case strings.Contains(n, "intel"):
		return "intel"
	}
	return ""
}

// gamePrefHigh: Windows usa già la scheda video potente per questo gioco?
func gamePrefHigh(path string) bool {
	v, ok := regGet(registry.CURRENT_USER, gpuPrefKey, path)
	return ok && strings.Contains(v, "GpuPreference=2")
}

// ---------------- Analisi ----------------

// fpsFindings: la parte che decide, separata dalla lettura del PC così si può provare anche su Linux.
// Regola: meglio non dire niente che dare un falso allarme.
func fpsFindings(f pcFacts, scr []screenInfo, last *fpsResult, battery, gpuPrefHigh bool, now time.Time) []fpsFinding {
	var out []fpsFinding
	add := func(x fpsFinding) { out = append(out, x) }
	disc, integ := false, false
	for _, g := range f.GPUs {
		if discreteGPU(g.Name) {
			disc = true
		} else if integratedGPU(g.Name) {
			integ = true
		}
	}

	// Schermo collegato alla scheda madre invece che alla scheda video (PC fisso).
	if disc && !f.Laptop {
		for _, s := range scr {
			if s.Primary && integratedGPU(s.Name) {
				add(fpsFinding{ID: "port", Sev: "big", Title: "Lo schermo è collegato alla scheda madre, non alla scheda video",
					Why:  "Così i giochi possono girare sulla grafica del processore, molto più lenta, o fare un passaggio in più.",
					Gain: "Spesso il doppio degli FPS o più.",
					Fix:  "Spegni il PC e sposta il cavo dello schermo nelle uscite della scheda video (quelle più in basso, dietro il PC).", Action: ""})
				break
			}
		}
	}
	// 1. Schermo più lento di quanto può.
	if !battery {
		for _, s := range scr {
			if hzWorth(s) {
				add(fpsFinding{ID: "hz", Sev: "big", Title: "Lo schermo va a " + strconv.Itoa(s.Hz) + " Hz ma può andare a " + strconv.Itoa(s.MaxHz) + " Hz",
					Why:  "Lo schermo mostra al massimo " + strconv.Itoa(s.Hz) + " immagini al secondo, anche se il gioco ne fa di più.",
					Gain: "Il gioco diventa molto più fluido (non cambia il numero di FPS del gioco, cambia quanti ne vedi).",
					Fix:  "Premi Sistemalo: hai 15 secondi per confermare che lo schermo si vede bene, altrimenti torna com'era. Se diventa nero o sfarfalla, il cavo potrebbe non reggere: usa DisplayPort o HDMI 2.1.", Action: "hz"})
				break
			}
		}
	}
	// 2. Portatile con due schede video: Windows non ha una preferenza salvata per il gioco.
	if f.Laptop && disc && integ && last != nil && last.path != "" && !gpuPrefHigh {
		add(fpsFinding{ID: "gpu", Sev: "mid", Title: last.Game + " potrebbe partire sulla scheda video integrata",
			Why:  "Questo portatile ha due schede video e Windows non ha una preferenza salvata per questo gioco. Molti giochi scelgono da soli quella potente, ma non tutti.",
			Gain: "Solo se ora usa quella integrata: anche il doppio degli FPS.",
			Fix:  "Premi Sistemalo: il Tasso dice a Windows di usare la scheda video potente per questo gioco. Poi riavvia il gioco e misura di nuovo.", Action: "gpu"})
	}
	// 3, 4 e altri: dalla misura.
	if last != nil && last.Avg > 0 {
		capped := 0.0
		if !battery && last.GpuBusy > 0 && last.GpuBusy < 80 && last.Spread > 0 && last.Spread < 5 {
			for _, c := range []float64{30, 60, 72, 75, 90, 100, 120} {
				if math.Abs(last.Avg-c)/c < 0.02 && (last.Hz == 0 || c < float64(last.Hz)*0.9) {
					capped = c
					break
				}
			}
		}
		switch {
		case capped > 0:
			add(fpsFinding{ID: "cap", Sev: "big", Title: last.Game + " è bloccato a " + strconv.Itoa(int(capped)) + " FPS",
				Why:  "Gli FPS sono fermi e la scheda video non è al limite, mentre lo schermo va a " + strconv.Itoa(last.Hz) + " Hz: c'è un limite (V-Sync o limite FPS) nel gioco o nel pannello della scheda video.",
				Gain: "Finché la scheda video arriva al massimo: spesso +20% e oltre.",
				Fix:  "Nel gioco V-Sync spento e limite FPS uguale ai Hz dello schermo (con G-Sync/FreeSync: V-Sync acceso nel pannello NVIDIA/AMD).", Action: "tune"})
		case last.GpuBusy >= 90 && last.Avg < 100:
			add(fpsFinding{ID: "upscale", Sev: "big", Title: "La scheda video è al massimo in " + last.Game,
				Why:  "Lavora per il " + strconv.Itoa(int(last.GpuBusy)) + "% di ogni fotogramma: il limite è lei, non Windows.",
				Gain: "DLSS, FSR o XeSS su Qualità: di solito +20–40% (se non sono già accesi). La generazione di fotogrammi arriva quasi al doppio degli FPS mostrati, ma serve partire da almeno 50–60 FPS.",
				Fix:  "Nelle impostazioni grafiche del gioco accendi DLSS (NVIDIA), FSR o XeSS su Qualità, oppure abbassa la grafica di un livello. \"Regola con Tasso\" ti dice la grafica giusta per il tuo PC.", Action: "tune"})
		case last.GpuBusy > 0 && last.GpuBusy < 60 && last.Avg < 120 && !vicinoAlLimite(last.Avg, last.Hz):
			add(fpsFinding{ID: "cpu", Sev: "mid", Title: "In " + last.Game + " il limite è il processore",
				Why:  "La scheda video lavora solo il " + strconv.Itoa(int(last.GpuBusy)) + "% del tempo: aspetta il processore.",
				Gain: "Abbassare distanza di visibilità, folla, ombre e fisica: +10–25%. Qui la generazione di fotogrammi aiuta, DLSS/FSR da soli poco.",
				Fix:  "Chiudi browser e programmi pesanti prima di giocare (il Boost lo fa per quelli in background) e abbassa quelle opzioni nel gioco.", Action: ""})
		}
		if last.VramPct >= 95 {
			add(fpsFinding{ID: "vram", Sev: "mid", Title: "La memoria della scheda video è piena (" + strconv.Itoa(int(last.VramPct)) + "%)",
				Why:  "Quando è piena il gioco deve usare la RAM normale, molto più lenta: arrivano cali e scatti.",
				Gain: "Meno scatti e cali, a volte +10–20% nei momenti pesanti.",
				Fix:  "Nel gioco abbassa la qualità delle texture di un livello.", Action: ""})
		}
		// Caldo: solo quando la scheda video rallenta davvero, o con temperature alte anche per un portatile.
		gpuLimit := 87.0
		if f.Laptop {
			gpuLimit = 92
		}
		if (last.Throttle && !f.Laptop) || last.GpuTemp >= gpuLimit {
			t := "La scheda video rallenta per il caldo"
			if last.GpuTemp > 0 {
				t += " (" + strconv.Itoa(int(last.GpuTemp)) + " °C)"
			}
			add(fpsFinding{ID: "gpuhot", Sev: "mid", Title: t,
				Why:  "Quando è troppo calda abbassa la sua velocità per proteggersi.",
				Gain: "Di solito +5–15% quando smette di rallentare.",
				Fix:  "Pulisci la polvere da ventole e filtri, lascia spazio dietro e sotto il PC. Sui portatili usa una base con ventole.", Action: ""})
		}
		if last.CpuTemp >= 98 {
			add(fpsFinding{ID: "cpuhot", Sev: "mid", Title: "Il processore è al limite di temperatura (" + strconv.Itoa(int(last.CpuTemp)) + " °C)",
				Why:  "A questa temperatura il processore abbassa la sua velocità. Su molti processori recenti un po' è normale.",
				Gain: "Di solito +5–15% nei giochi che usano molto il processore.",
				Fix:  "Pulisci la polvere dal dissipatore e controlla che la ventola giri. Se il PC ha qualche anno, la pasta termica va cambiata (meglio da un tecnico).", Action: ""})
		}
	}
	// 5. RAM
	if len(f.RAM) > 0 {
		ddr, conf, rated := 0, 0, 0
		var total int64
		lp := false
		for _, m := range f.RAM {
			total += m.Size
			if m.Type == 30 || m.Type == 35 || m.Type == 36 {
				lp = true // LPDDR saldata: spesso un solo modulo ma già a più canali
			}
			if m.Type > ddr {
				ddr = m.Type
			}
			c := m.Conf
			if c == 0 {
				c = m.Speed
			}
			if conf == 0 || (c > 0 && c < conf) {
				conf = c
			}
			if r := ramRated(m.Part); r > rated {
				rated = r
			}
		}
		if len(f.RAM) == 1 && total >= 4<<30 && !lp && (ddr == 26 || ddr == 34 || ddr == 24) {
			add(fpsFinding{ID: "ram1", Sev: "big", Title: "C'è un solo banco di RAM",
				Why:  "Con un banco solo la memoria lavora a metà strada (single channel). Pesa soprattutto con la grafica integrata e nei giochi che usano tanto il processore.",
				Gain: "Con due banchi uguali: di solito +10–25% (con DDR5 un po' meno), con la grafica integrata anche +30–50%.",
				Fix:  "Aggiungi un secondo banco identico (meglio comprare un kit da due). Sui portatili controlla prima che ci sia lo slot libero.", Action: ""})
		}
		if !f.Laptop && (ddr == 26 || ddr == 34) && conf > 0 && rated >= conf+200 {
			add(fpsFinding{ID: "xmp", Sev: "mid", Title: "La RAM va a " + strconv.Itoa(conf) + " invece di " + strconv.Itoa(rated) + " (XMP o EXPO spento)",
				Why:  "I tuoi banchi sono fatti per andare a " + strconv.Itoa(rated) + ", ma solo se nel BIOS è acceso XMP (Intel) o EXPO (AMD).",
				Gain: "Di solito +5–15%, soprattutto nei cali di FPS.",
				Fix:  "Nel BIOS (tasto Canc o F2 all'accensione) cerca XMP, D.O.C.P. o EXPO e scegli il Profilo 1. Con DDR5 il primo avvio può restare nero qualche minuto: aspetta. Se poi il PC si blocca o va in crash, rimetti XMP spento.", Action: ""})
		}
	}
	// 7. Driver video
	for _, g := range f.GPUs {
		if disc && !discreteGPU(g.Name) {
			continue
		}
		d, err := time.Parse("2006-01-02", g.Date)
		if err != nil {
			continue
		}
		if months := int(now.Sub(d).Hours() / 24 / 30); months >= 6 {
			if v := gpuVendorPage(g.Name); v != "" {
				fix := "Premi Apri: si apre la pagina ufficiale per scaricare il driver giusto."
				if v == "nvidia" {
					fix = "Premi Aggiorna il driver: il Tasso scarica il driver ufficiale da NVIDIA, controlla la firma digitale, crea un punto di ripristino e lo installa."
				} else if v == "amd" {
					fix = "Premi Aggiorna il driver: il Tasso prende da Windows Update il driver AMD certificato, crea un punto di ripristino e lo installa."
				}
				if f.Laptop && v == "intel" {
					fix += " Se il sito non accetta il driver per il tuo portatile, usa quello del produttore del portatile."
				}
				add(fpsFinding{ID: "driver", Sev: "mid", Title: "Il driver della scheda video è vecchio (" + strconv.Itoa(months) + " mesi fa)",
					Why:  "I driver nuovi sistemano errori e a volte migliorano i giochi usciti da poco.",
					Gain: "Di solito pochi FPS; nei giochi nuovi a volte +10% o più, e meno crash.",
					Fix:  fix, Action: "driver:" + v})
				break
			}
		}
	}
	// Batteria
	if battery {
		add(fpsFinding{ID: "battery", Sev: "big", Title: "Il portatile va a batteria",
			Why:  "A batteria processore e scheda video vanno molto più piano per consumare meno.",
			Gain: "Con il caricatore attaccato: spesso +30–100%.",
			Fix:  "Collega il caricatore prima di giocare.", Action: ""})
	}
	return out
}

func fpsCheck() map[string]interface{} {
	var f pcFacts
	if r := runPS(fpsCheckScript, time.Minute, nil); r.JSON != "" {
		_ = json.Unmarshal([]byte(r.JSON), &f)
	}
	last := getLastFps()
	prefHigh := false
	if last != nil && last.path != "" {
		prefHigh = gamePrefHigh(last.path)
	}
	found := fpsFindings(f, screens(), last, onBattery(), prefHigh, time.Now())
	return map[string]interface{}{"found": found, "measured": last != nil}
}

// fpsSetGpu: scheda video potente per il gioco misurato. Ricordo il valore di prima, così "Rimetti
// com'era" lo rimette.
func fpsSetGpu() string {
	last := getLastFps()
	if last == nil || last.path == "" || !strings.HasSuffix(strings.ToLower(last.path), ".exe") {
		return "Prima misura gli FPS con il gioco aperto."
	}
	old, had := regGet(registry.CURRENT_USER, gpuPrefKey, last.path)
	// Se l'aveva già cambiata "Regola con Tasso", il valore vero di prima è quello che si era segnato lei.
	orig, known := gpuPrefOrig(last.path)
	bMu.Lock()
	s := loadBoost()
	if s.GpuPrev == nil {
		s.GpuPrev = map[string]string{}
	}
	if _, saved := s.GpuPrev[last.path]; !saved {
		switch {
		case known:
			s.GpuPrev[last.path] = orig
		case had:
			s.GpuPrev[last.path] = "1" + old
		default:
			s.GpuPrev[last.path] = "0"
		}
	}
	saveBoost()
	bMu.Unlock()
	val := "GpuPreference=2;"
	if had {
		var parts []string
		for _, p := range strings.Split(old, ";") {
			if p != "" && !strings.HasPrefix(p, "GpuPreference=") {
				parts = append(parts, p)
			}
		}
		val = strings.Join(append([]string{"GpuPreference=2"}, parts...), ";") + ";"
	}
	if regSet(registry.CURRENT_USER, gpuPrefKey, last.path, val) != nil {
		return "Windows non mi ha lasciato cambiare questa impostazione."
	}
	return "ok:Fatto: " + last.Game + " userà la scheda video potente. Riavvia il gioco e misura di nuovo."
}

// gpuPrefRestore: rimette le preferenze della scheda video com'erano (da "Rimetti com'era"), solo
// dove c'è ancora la scheda potente messa da Tasso.
func gpuPrefRestore(prev map[string]string) {
	for path, v := range prev {
		if !strings.HasSuffix(strings.ToLower(path), ".exe") {
			continue
		}
		gpuPrefUndo(path, v)
	}
}

// fpsAutoFix: dopo il Boost (o una misura automatica) il Tasso sistema da solo quello che si può
// sistemare in modo sicuro e reversibile, e mostra il resto come consigli.
//   - scheda video potente per il gioco (portatili): da solo, si rimette con "Rimetti com'era"
//   - frequenza dello schermo: solo quando premi BOOST (sei davanti al Tasso), con 15 secondi per
//     confermare; mai durante una partita, perché non potresti vedere la conferma
//   - il resto (grafica nel gioco, BIOS, cavi, polvere) si fa a mano: il Tasso spiega come
func fpsAutoFix(interactive bool) {
	setMu.Lock()
	paused := settings.Paused
	setMu.Unlock()
	if paused { // Tasso spento: non cambio niente (né schermo né registro)
		return
	}
	r := fpsCheck()
	found, _ := r["found"].([]fpsFinding)
	var left []fpsFinding
	var msgs []string
	for _, f := range found {
		switch {
		case f.Action == "gpu":
			if m := fpsSetGpu(); strings.HasPrefix(m, "ok:") {
				msgs = append(msgs, strings.TrimPrefix(m, "ok:"))
				continue
			}
		case f.Action == "hz" && interactive:
			go screenHzMax()
			continue
		}
		left = append(left, f)
	}
	r["found"] = left
	r["auto"] = msgs
	emit("onFpsCheck", r)
}
