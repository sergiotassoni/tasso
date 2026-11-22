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
	"strconv"
	"strings"
	"sync"
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
				dm.DisplayFlags&0x2 == 0 && int(dm.DisplayFrequency) > s.MaxHz && dm.DisplayFrequency <= 500 {
				s.MaxHz = int(dm.DisplayFrequency)
			}
		}
		out = append(out, s)
	}
	return out
}

func primaryHz() int {
	for _, s := range screens() {
		if s.Primary {
			return s.Hz
		}
	}
	return 0
}

var (
	hzMu     sync.Mutex
	hzUndo   map[string]int // schermo -> frequenza di prima, finché non confermi
	hzTimers map[string]*time.Timer
)

// setScreenHz cambia la frequenza di uno schermo. Se non confermi entro 15 secondi (per esempio
// perché lo schermo è diventato nero) rimette quella di prima, come fa Windows.
func setScreenHz(dev string, hz int) bool {
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
	dm.Fields = 0x400000 | 0x80000 | 0x100000                                                              // frequenza, larghezza, altezza
	r, _, _ := pChangeDisplay.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&dm)), 0, 0x1, 0) // CDS_UPDATEREGISTRY
	return r == 0
}

func screenHzMax() map[string]interface{} {
	hzMu.Lock()
	defer hzMu.Unlock()
	if hzUndo == nil {
		hzUndo, hzTimers = map[string]int{}, map[string]*time.Timer{}
	}
	var done []string
	for _, s := range screens() {
		if s.MaxHz <= s.Hz || s.Hz <= 0 {
			continue
		}
		if setScreenHz(s.Dev, s.MaxHz) {
			dev, prev := s.Dev, s.Hz
			hzUndo[dev] = prev
			if t := hzTimers[dev]; t != nil {
				t.Stop()
			}
			hzTimers[dev] = time.AfterFunc(15*time.Second, func() {
				hzMu.Lock()
				p, ok := hzUndo[dev]
				delete(hzUndo, dev)
				delete(hzTimers, dev)
				hzMu.Unlock()
				if ok {
					setScreenHz(dev, p)
					emit("onFpsHz", map[string]interface{}{"kept": false})
				}
			})
			done = append(done, strconv.Itoa(s.Hz)+" → "+strconv.Itoa(s.MaxHz)+" Hz")
		}
	}
	return map[string]interface{}{"changed": done}
}

// screenHzKeep: hai confermato che lo schermo si vede bene.
func screenHzKeep() {
	hzMu.Lock()
	defer hzMu.Unlock()
	for dev, t := range hzTimers {
		t.Stop()
		delete(hzTimers, dev)
		delete(hzUndo, dev)
	}
	emit("onFpsHz", map[string]interface{}{"kept": true})
}

// ---------------- Temperature ----------------

// readTemps: processore (LibreHardwareMonitor o sensore ACPI) e scheda video, letti alla fine della
// misura, quando il gioco ha appena lavorato. -1 / 0 = non disponibile.
func readTemps() (cpu, gpu float64, throttle bool) {
	r := runPS(cpuTempScript, 20*time.Second, nil)
	if f, err := strconv.ParseFloat(strings.TrimSpace(r.JSON), 64); err == nil && f > 15 && f < 115 {
		cpu = f
	}
	sampleGPUNow()
	liveMu.Lock()
	if g := gpuFresh(); g != nil {
		if t, ok := g["temp"].(float64); ok {
			gpu = t
		}
	}
	liveMu.Unlock()
	if nvsmi != "" {
		out, err := runHiddenTimeout(3*time.Second, nvsmi, "--query-gpu=clocks_throttle_reasons.hw_thermal_slowdown,clocks_throttle_reasons.sw_thermal_slowdown", "--format=csv,noheader")
		if err == nil && strings.Contains(strings.ToLower(out), "active") && !strings.Contains(strings.ToLower(out), "not active, not active") {
			for _, f := range strings.Split(strings.Split(out, "\n")[0], ",") {
				if strings.TrimSpace(strings.ToLower(f)) == "active" {
					throttle = true
				}
			}
		}
	}
	return
}

// ---------------- RAM, schede video e driver (PowerShell, una volta) ----------------

const fpsCheckScript = `
$mem = @(Get-CimInstance Win32_PhysicalMemory -ErrorAction SilentlyContinue)
$gpus = @(Get-CimInstance Win32_VideoController -ErrorAction SilentlyContinue)
$bat = @(Get-CimInstance Win32_Battery -ErrorAction SilentlyContinue)
$o = [ordered]@{
  laptop = ($bat.Count -gt 0)
  ram = @($mem | ForEach-Object { [ordered]@{ type = [int]$_.SMBIOSMemoryType; speed = [int]$_.Speed; conf = [int]$_.ConfiguredClockSpeed; size = [long]$_.Capacity } })
  gpus = @($gpus | ForEach-Object { [ordered]@{ name = [string]$_.Name; date = $(if ($_.DriverDate) { $_.DriverDate.ToString('yyyy-MM-dd') } else { '' }) } })
}
[Console]::Out.WriteLine('JSON ' + ($o | ConvertTo-Json -Compress -Depth 4))
`

type pcFacts struct {
	Laptop bool `json:"laptop"`
	RAM    []struct {
		Type  int   `json:"type"`
		Speed int   `json:"speed"`
		Conf  int   `json:"conf"`
		Size  int64 `json:"size"`
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
func fpsFindings(f pcFacts, scr []screenInfo, last *fpsResult, battery, gpuPrefHigh bool, now time.Time) []fpsFinding {
	var out []fpsFinding
	add := func(x fpsFinding) { out = append(out, x) }

	// 1. Schermo
	for _, s := range scr {
		if s.MaxHz >= s.Hz+15 && s.MaxHz > 60 {
			add(fpsFinding{ID: "hz", Sev: "big", Title: "Lo schermo va a " + strconv.Itoa(s.Hz) + " Hz ma può andare a " + strconv.Itoa(s.MaxHz) + " Hz",
				Why:  "Vedi al massimo " + strconv.Itoa(s.Hz) + " immagini al secondo, anche se il gioco ne fa di più.",
				Gain: "Fino a " + strconv.Itoa(s.MaxHz) + " FPS visibili invece di " + strconv.Itoa(s.Hz) + ": il gioco sembra molto più fluido.",
				Fix:  "Premi Sistema: hai 15 secondi per confermare che lo schermo si vede bene, altrimenti torna com'era.", Action: "hz"})
			break
		}
	}
	// 2. Portatile con due schede video: il gioco deve usare quella potente.
	disc, integ := false, false
	for _, g := range f.GPUs {
		if discreteGPU(g.Name) {
			disc = true
		} else {
			integ = true
		}
	}
	if disc && integ && last != nil && last.path != "" && !gpuPrefHigh {
		add(fpsFinding{ID: "gpu", Sev: "big", Title: last.Game + " potrebbe usare la scheda video integrata",
			Why:  "Questo PC ha due schede video e Windows non ha scelto quella potente per questo gioco.",
			Gain: "Se il gioco usava quella integrata: anche il doppio degli FPS o più.",
			Fix:  "Premi Sistema: il Tasso dice a Windows di usare la scheda video potente per questo gioco. Poi riavvia il gioco.", Action: "gpu"})
	}
	// 3 e 4. Dalla misura
	if last != nil && last.Avg > 0 {
		capped := false
		for _, c := range []float64{30, 60, 72, 75, 90, 100, 120, 144, 165, 170, 180, 240, float64(last.Hz)} {
			if c > 0 && math.Abs(last.Avg-c)/c < 0.03 && last.Low1 > c*0.8 && (last.GpuBusy == 0 || last.GpuBusy < 85) {
				capped = true
				break
			}
		}
		if capped {
			add(fpsFinding{ID: "cap", Sev: "big", Title: last.Game + " è bloccato a circa " + strconv.Itoa(int(math.Round(last.Avg))) + " FPS",
				Why:  "Gli FPS sono fermi su un numero tondo e la scheda video non è al limite: c'è un limite (V-Sync o limite FPS) nel gioco o nel pannello della scheda video.",
				Gain: "Se il tuo schermo va più veloce: +20% e oltre, finché la scheda video arriva al massimo.",
				Fix:  "Nel gioco metti V-Sync spento e limite FPS uguale ai Hz del tuo schermo (o 3 in meno se hai G-Sync/FreeSync). Se il limite è nel pannello NVIDIA o AMD, togli \"Frequenza fotogrammi massima\".", Action: "tune"})
		} else if last.GpuBusy >= 90 && last.Avg < 100 {
			add(fpsFinding{ID: "upscale", Sev: "big", Title: "La scheda video è al massimo in " + last.Game,
				Why:  "Lavora per il " + strconv.Itoa(int(last.GpuBusy)) + "% di ogni fotogramma: il limite è lei, non Windows.",
				Gain: "DLSS, FSR o XeSS in modalità Qualità: di solito +30–60%. La generazione di fotogrammi raddoppia gli FPS mostrati, ma aggiunge un po' di ritardo.",
				Fix:  "Nelle impostazioni grafiche del gioco accendi DLSS (NVIDIA), FSR o XeSS su Qualità. \"Regola con Tasso\" ti dice anche la grafica giusta per il tuo PC.", Action: "tune"})
		} else if last.GpuBusy > 0 && last.GpuBusy < 60 && last.Avg < 120 {
			add(fpsFinding{ID: "cpu", Sev: "mid", Title: "In " + last.Game + " il limite è il processore",
				Why:  "La scheda video lavora solo il " + strconv.Itoa(int(last.GpuBusy)) + "% del tempo: aspetta il processore.",
				Gain: "Abbassare le opzioni che pesano sul processore (distanza di visibilità, folla, ombre, fisica): +10–25%. DLSS/FSR qui aiutano poco.",
				Fix:  "Chiudi browser e programmi pesanti prima di giocare (il Boost lo fa per quelli in background) e abbassa quelle opzioni nel gioco.", Action: ""})
		}
		// 6. Caldo
		if last.Throttle || last.GpuTemp >= 87 {
			add(fpsFinding{ID: "gpuhot", Sev: "big", Title: "La scheda video scalda troppo (" + strconv.Itoa(int(last.GpuTemp)) + " °C)",
				Why:  "Quando è troppo calda rallenta da sola per proteggersi.",
				Gain: "+10–30% quando smette di rallentare.",
				Fix:  "Pulisci la polvere da ventole e filtri, lascia spazio dietro e sotto il PC. Sui portatili usa una base con ventole.", Action: ""})
		}
		if last.CpuTemp >= 95 {
			add(fpsFinding{ID: "cpuhot", Sev: "big", Title: "Il processore scalda troppo (" + strconv.Itoa(int(last.CpuTemp)) + " °C)",
				Why:  "Sopra i 95 °C il processore abbassa la sua velocità.",
				Gain: "+10–30% nei giochi che usano molto il processore.",
				Fix:  "Pulisci la polvere dal dissipatore e controlla che la ventola giri. Se il PC ha qualche anno, la pasta termica va cambiata (meglio da un tecnico).", Action: ""})
		}
	}
	// 5. RAM
	if len(f.RAM) > 0 {
		var total int64
		ddr, conf := 0, 0
		for _, m := range f.RAM {
			total += m.Size
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
		}
		if len(f.RAM) == 1 && total >= 4<<30 {
			add(fpsFinding{ID: "ram1", Sev: "big", Title: "C'è un solo banco di RAM",
				Why:  "Con un banco solo la memoria lavora a metà strada (single channel). Pesa molto con la scheda video integrata e nei giochi che usano tanto il processore.",
				Gain: "Con due banchi uguali: di solito +10–25%, con la scheda video integrata anche +30–50%.",
				Fix:  "Aggiungi un secondo banco uguale (stessa marca, misura e velocità). Sui portatili controlla prima che ci sia lo slot libero.", Action: ""})
		}
		if !f.Laptop && ((ddr == 26 && conf > 0 && conf <= 2666) || (ddr == 34 && conf > 0 && conf <= 4800)) {
			kind := "DDR4"
			if ddr == 34 {
				kind = "DDR5"
			}
			add(fpsFinding{ID: "xmp", Sev: "mid", Title: "La RAM va a " + strconv.Itoa(conf) + " MHz (velocità base " + kind + ")",
				Why:  "Molti banchi da gioco sono fatti per andare più veloci, ma solo se nel BIOS è acceso XMP (Intel) o EXPO (AMD). Se i tuoi banchi sono normali, va già bene così.",
				Gain: "Se i banchi lo permettono: +5–20%, soprattutto nei cali di FPS.",
				Fix:  "Riavvia ed entra nel BIOS (di solito tasto Canc o F2 all'accensione), cerca XMP, D.O.C.P. o EXPO e scegli il Profilo 1. Se il PC non si avvia più, il BIOS torna da solo come prima dopo qualche tentativo.", Action: ""})
		}
	}
	// 7. Driver video
	for _, g := range f.GPUs {
		if !discreteGPU(g.Name) && disc {
			continue
		}
		d, err := time.Parse("2006-01-02", g.Date)
		if err != nil {
			continue
		}
		if months := int(now.Sub(d).Hours() / 24 / 30); months >= 6 {
			if v := gpuVendorPage(g.Name); v != "" {
				add(fpsFinding{ID: "driver", Sev: "mid", Title: "Il driver della scheda video ha " + strconv.Itoa(months) + " mesi",
					Why:  "I driver nuovi sistemano errori e a volte migliorano i giochi usciti da poco.",
					Gain: "Di solito pochi FPS; nei giochi nuovi a volte +10% o più, e meno crash.",
					Fix:  "Premi Apri: si apre la pagina ufficiale per scaricare il driver giusto.", Action: "driver:" + v})
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
	bMu.Lock()
	s := loadBoost()
	if s.GpuPrev == nil {
		s.GpuPrev = map[string]string{}
	}
	if _, saved := s.GpuPrev[last.path]; !saved {
		if had {
			s.GpuPrev[last.path] = "1" + old
		} else {
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

// gpuPrefRestore: rimette le preferenze della scheda video com'erano (da "Rimetti com'era").
func gpuPrefRestore(prev map[string]string) {
	for path, v := range prev {
		if !strings.HasSuffix(strings.ToLower(path), ".exe") || v == "" {
			continue
		}
		if v[0] == '1' {
			_ = regSet(registry.CURRENT_USER, gpuPrefKey, path, v[1:])
		} else {
			regDel(registry.CURRENT_USER, gpuPrefKey, path)
		}
	}
}
