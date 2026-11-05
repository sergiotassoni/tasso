package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32        = windows.NewLazySystemDLL("kernel32.dll")
	pGetSystemTimes = kernel32.NewProc("GetSystemTimes")
	pGlobalMem      = kernel32.NewProc("GlobalMemoryStatusEx")
)

type memStatus struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

var (
	liveMu              sync.Mutex
	lastIdle, lastTotal uint64
	cpuPct              float64
	gpuLive             map[string]interface{}
	gpuAt, gpuTempAt    time.Time // quando l'ho letta: un valore vecchio non lo mostro come attuale
	nvsmi               string
)

func ft(f windows.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }

func sampleCPU() {
	var idle, kern, user windows.Filetime
	r, _, _ := pGetSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kern)), uintptr(unsafe.Pointer(&user)))
	if r == 0 {
		return
	}
	i, t := ft(idle), ft(kern)+ft(user)
	liveMu.Lock()
	if lastTotal != 0 && t > lastTotal {
		dt := float64(t - lastTotal)
		di := float64(i - lastIdle)
		cpuPct = (1 - di/dt) * 100
		if cpuPct < 0 {
			cpuPct = 0
		}
	}
	lastIdle, lastTotal = i, t
	liveMu.Unlock()
}

func findNvidiaSmi() string {
	cands := []string{
		filepath.Join(os.Getenv("SystemRoot"), "System32", "nvidia-smi.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "NVIDIA Corporation", "NVSMI", "nvidia-smi.exe"),
	}
	for _, c := range cands {
		if fileExists(c) {
			return c
		}
	}
	return ""
}

func sampleGPU() {
	if nvsmi == "" {
		return
	}
	out, err := runHiddenTimeout(3*time.Second, nvsmi, "--query-gpu=name,temperature.gpu,utilization.gpu,fan.speed,memory.used,memory.total", "--format=csv,noheader,nounits")
	if err != nil {
		return
	}
	line := strings.TrimSpace(strings.Split(out, "\n")[0])
	parts := strings.Split(line, ",")
	if len(parts) < 6 {
		return
	}
	num := func(s string) interface{} {
		v, e := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if e != nil {
			return nil
		}
		return v
	}
	g := map[string]interface{}{
		"name":    strings.TrimSpace(parts[0]),
		"temp":    num(parts[1]),
		"util":    num(parts[2]),
		"fan":     num(parts[3]),
		"memUsed": num(parts[4]),
		"memTot":  num(parts[5]),
	}
	liveMu.Lock()
	gpuLive, gpuAt, gpuTempAt = g, time.Now(), time.Now()
	liveMu.Unlock()
}

// startLiveMonitor legge processore e scheda video in background, con pochissimo consumo.
func startLiveMonitor() {
	nvsmi = findNvidiaSmi()
	if nvsmi == "" {
		pdhInit()
	}
	sampleCPU()
	go func() {
		defer guard("live.go:117")
		n := 0
		for {
			time.Sleep(2 * time.Second)
			sampleCPU()
			// La scheda video la leggo solo se qualcuno la guarda (finestra aperta o pannellino in gioco):
			// sui portatili leggerla sveglia la scheda video e consuma batteria.
			if n%2 == 0 && gpuWatched() {
				gpuSampleMu.Lock()
				if nvsmi != "" {
					sampleGPU()
				} else {
					sampleAMD()
					sampleGPUPdh()
				}
				gpuSampleMu.Unlock()
			}
			n++
		}
	}()
}

func liveStats() map[string]interface{} {
	m := memStatus{Length: uint32(unsafe.Sizeof(memStatus{}))}
	pGlobalMem.Call(uintptr(unsafe.Pointer(&m)))
	var free, total, totalFree uint64
	root, _ := windows.UTF16PtrFromString(os.Getenv("SystemDrive") + "\\")
	_ = windows.GetDiskFreeSpaceEx(root, &free, &total, &totalFree)
	liveMu.Lock()
	defer liveMu.Unlock()
	return map[string]interface{}{
		"cpu":       cpuPct,
		"ramTotal":  m.TotalPhys,
		"ramUsed":   m.TotalPhys - m.AvailPhys,
		"diskFree":  free,
		"diskTotal": total,
		"gpu":       gpuFresh(),
	}
}

var gpuSampleMu sync.Mutex

// gpuFresh: i dati della scheda video solo se letti da poco (va chiamata con liveMu preso).
func gpuFresh() map[string]interface{} {
	if time.Since(gpuAt) > 15*time.Second {
		return nil
	}
	return copyMap(gpuLive)
}

// sampleGPUNow legge subito la scheda video (per il report su Discord quando Tasso è a icona).
func sampleGPUNow() {
	liveMu.Lock()
	fresh := time.Since(gpuAt) <= 15*time.Second
	liveMu.Unlock()
	if fresh {
		return
	}
	gpuSampleMu.Lock()
	defer gpuSampleMu.Unlock()
	if nvsmi != "" {
		sampleGPU()
	} else {
		sampleAMD()
		sampleGPUPdh()
	}
}

// ---- Uso della scheda video per AMD e Intel (contatori di Windows, come Gestione attività) ----

var (
	pdh              = windows.NewLazySystemDLL("pdh.dll")
	pPdhOpenQuery    = pdh.NewProc("PdhOpenQueryW")
	pPdhAddCounter   = pdh.NewProc("PdhAddEnglishCounterW")
	pPdhCollect      = pdh.NewProc("PdhCollectQueryData")
	pPdhGetArray     = pdh.NewProc("PdhGetFormattedCounterArrayW")
	pdhQuery, pdhCnt uintptr
	pdhOK            bool
)

type pdhItem struct {
	name   *uint16
	status uint32
	_      uint32
	value  float64
}

func pdhInit() {
	if pPdhOpenQuery.Find() != nil {
		return
	}
	if r, _, _ := pPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&pdhQuery))); r != 0 {
		return
	}
	path, _ := windows.UTF16PtrFromString(`\GPU Engine(*engtype_3D)\Utilization Percentage`)
	if r, _, _ := pPdhAddCounter.Call(pdhQuery, uintptr(unsafe.Pointer(path)), 0, uintptr(unsafe.Pointer(&pdhCnt))); r != 0 {
		return
	}
	pPdhCollect.Call(pdhQuery)
	pdhOK = true
}

func sampleGPUPdh() {
	if !pdhOK {
		return
	}
	if r, _, _ := pPdhCollect.Call(pdhQuery); r != 0 {
		return
	}
	var size, count uint32
	pPdhGetArray.Call(pdhCnt, 0x200, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if size == 0 || size > 4<<20 {
		return
	}
	buf := make([]byte, size)
	if r, _, _ := pPdhGetArray.Call(pdhCnt, 0x200, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0]))); r != 0 {
		return
	}
	items := unsafe.Slice((*pdhItem)(unsafe.Pointer(&buf[0])), count)
	sum := 0.0
	for _, it := range items {
		if it.status == 0 || it.status == 1 { // PDH_CSTATUS_VALID_DATA / NEW_DATA
			sum += it.value
		}
	}
	if sum > 100 {
		sum = 100
	}
	liveMu.Lock()
	// La temperatura la tengo solo se l'ho letta adesso: altrimenti sembrerebbe attuale.
	if gpuLive != nil && gpuLive["temp"] != nil && time.Since(gpuTempAt) < 10*time.Second {
		g := copyMap(gpuLive)
		g["util"] = sum // stessa percentuale di Gestione attività
		gpuLive = g
	} else {
		gpuLive = map[string]interface{}{"util": sum}
	}
	gpuAt = time.Now()
	liveMu.Unlock()
}

// copyMap: chi legge i sensori riceve una copia, così nessuno legge la mappa mentre viene aggiornata.
func copyMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	c := make(map[string]interface{}, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

// gpuWatched: la finestra di Tasso è aperta (non a icona) oppure il pannellino è visibile.
func gpuWatched() bool {
	ovMu.Lock()
	ov := ovShown
	ovMu.Unlock()
	if ov {
		return true
	}
	if wv == nil {
		return false
	}
	r, _, _ := pIsIconic.Call(uintptr(wv.Window()))
	return r == 0
}
