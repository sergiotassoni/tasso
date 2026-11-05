package main

// Voto al PC (benchmark semplice). Due voti separati, per essere onesti:
//   - Velocità dei componenti: processore (1 e tutti i core), memoria e disco. Dipende dall'hardware:
//     una pulizia la cambia poco (di solito solo il disco, se era quasi pieno).
//   - Leggerezza di Windows (0-100): memoria occupata, processi in background, programmi all'avvio,
//     spazio libero e file inutili. È quella che migliora davvero dopo una pulizia.
// 1000 punti = un PC da gioco di fascia media di oggi (processore a 6 core, RAM DDR4, SSD NVMe).

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type benchResult struct {
	When     string  `json:"when"`
	Label    string  `json:"label"` // "", "prima", "dopo"
	Score    int     `json:"score"` // velocità dei componenti
	CPU1     int     `json:"cpu1"`
	CPUN     int     `json:"cpuN"`
	RAM      int     `json:"ram"`
	Disk     int     `json:"disk"`
	Light    int     `json:"light"` // leggerezza 0-100
	RAMPct   int     `json:"ramPct"`
	Procs    int     `json:"procs"`
	Startup  int     `json:"startup"`
	FreePct  int     `json:"freePct"`
	JunkGB   float64 `json:"junkGB"`
	Cores    int     `json:"cores"`
	DiskSkip bool    `json:"diskSkip,omitempty"`
}

// Valori di riferimento (= 1000 punti).
const (
	refCPU1  = 180e6  // giri al secondo su un core
	refCPUN  = 1300e6 // giri al secondo su tutti i core
	refRAM   = 12e9   // byte copiati al secondo
	refDiskW = 1500e6 // scrittura sequenziale, byte/s
	refDiskR = 2500e6 // lettura sequenziale, byte/s
	refRand  = 45e6   // lettura casuale 4K, byte/s
)

var benchSink atomic.Uint64

// cpuWork: calcoli misti (interi, salti, memoria piccola) uguali per tutti i processori.
func cpuWork(stop *atomic.Bool) uint64 {
	var tbl [1024]uint64
	x := uint64(88172645463325252)
	var n uint64
	for !stop.Load() {
		for i := 0; i < 4096; i++ {
			x ^= x << 13
			x ^= x >> 7
			x ^= x << 17
			j := x & 1023
			tbl[j] += x
			if tbl[j]&1 == 0 {
				x += tbl[(j+7)&1023]
			}
		}
		n += 4096
	}
	benchSink.Add(x + tbl[0])
	return n
}

func cpuScore(threads int, d time.Duration) float64 {
	var stop atomic.Bool
	var total atomic.Uint64
	var wg sync.WaitGroup
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func() {
			defer guard("bench.go:82")
			defer wg.Done()
			runtime.LockOSThread()
			total.Add(cpuWork(&stop))
		}()
	}
	start := time.Now()
	time.Sleep(d)
	stop.Store(true)
	wg.Wait()
	return float64(total.Load()) / time.Since(start).Seconds()
}

func ramScore() float64 {
	const size = 128 << 20
	src := make([]byte, size)
	dst := make([]byte, size)
	for i := range src {
		src[i] = byte(i)
	}
	copy(dst, src) // riscaldamento
	start := time.Now()
	var bytes float64
	for time.Since(start) < 1500*time.Millisecond {
		copy(dst, src)
		bytes += size
	}
	benchSink.Add(uint64(dst[size-1]))
	return bytes / time.Since(start).Seconds()
}

// alignedBuf: il disco senza cache di Windows vuole memoria allineata a 4096 byte.
func alignedBuf(n int) ([]byte, func()) {
	p, err := windows.VirtualAlloc(0, uintptr(n), windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if err != nil {
		return nil, func() {}
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(p)), n), func() { _ = windows.VirtualFree(p, 0, windows.MEM_RELEASE) }
}

// diskScore: scrittura e lettura vere sul disco di Windows (senza la cache), poi lettura casuale 4K.
// Usa un file temporaneo di 256 MB che poi cancella.
func diskScore() (float64, bool) {
	var free, total, tf uint64
	root, _ := windows.UTF16PtrFromString(os.Getenv("SystemDrive") + "\\")
	if windows.GetDiskFreeSpaceEx(root, &free, &total, &tf) != nil || free < 2<<30 {
		return 0, false // meno di 2 GB liberi: non scrivo niente
	}
	const size = 256 << 20
	const chunk = 4 << 20
	path := filepath.Join(appDir, "voto-disco.tmp")
	defer os.Remove(path)
	buf, release := alignedBuf(chunk)
	if buf == nil {
		return 0, false
	}
	defer release()
	for i := range buf {
		buf[i] = byte(i * 31)
	}
	p16, _ := windows.UTF16PtrFromString(path)
	const noBuf, writeThrough = 0x20000000, 0x80000000
	h, err := windows.CreateFile(p16, windows.GENERIC_WRITE, 0, nil, windows.CREATE_ALWAYS, windows.FILE_ATTRIBUTE_TEMPORARY|noBuf|writeThrough, 0)
	if err != nil {
		return 0, false
	}
	start := time.Now()
	var n uint32
	for done := 0; done < size; done += chunk {
		if windows.WriteFile(h, buf, &n, nil) != nil {
			windows.CloseHandle(h)
			return 0, false
		}
	}
	_ = windows.FlushFileBuffers(h)
	w := float64(size) / time.Since(start).Seconds()
	windows.CloseHandle(h)

	h, err = windows.CreateFile(p16, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, noBuf, 0)
	if err != nil {
		return 0, false
	}
	defer windows.CloseHandle(h)
	start = time.Now()
	for done := 0; done < size; done += chunk {
		if windows.ReadFile(h, buf, &n, nil) != nil || n == 0 {
			break
		}
	}
	r := float64(size) / time.Since(start).Seconds()

	small := buf[:4096]
	x := uint64(time.Now().UnixNano()) | 1
	var reads float64
	start = time.Now()
	for time.Since(start) < 1500*time.Millisecond {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		off := int64(x%uint64(size/4096)) * 4096
		if _, err := windows.Seek(h, off, 0); err != nil {
			break
		}
		if windows.ReadFile(h, small, &n, nil) != nil {
			break
		}
		reads++
	}
	rnd := reads * 4096 / time.Since(start).Seconds()
	// Media geometrica dei tre, pesata come li senti tu: aprire programmi = letture casuali.
	return math.Pow(w/refDiskW, 0.25) * math.Pow(r/refDiskR, 0.25) * math.Pow(rnd/refRand, 0.5), true
}

func procCount() int {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	n := 0
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		n++
	}
	return n
}

func clampPts(v float64) int { return int(math.Round(math.Max(1, math.Min(v*1000, 9999)))) }

// benchMu: due misure insieme si disturberebbero (e il disco darebbe 0).
var benchMu sync.Mutex

func runBench(label string) benchResult {
	benchMu.Lock()
	defer benchMu.Unlock()
	res := benchResult{When: time.Now().Format("2006-01-02 15:04"), Label: label, Cores: runtime.NumCPU()}
	emit("onBenchProgress", "Misuro il processore su un core…")
	c1 := cpuScore(1, 2*time.Second)
	emit("onBenchProgress", "Misuro il processore su tutti i core…")
	cn := cpuScore(runtime.NumCPU(), 2*time.Second)
	emit("onBenchProgress", "Misuro la memoria…")
	ram := ramScore()
	emit("onBenchProgress", "Misuro il disco (256 MB di prova, poi li cancello)…")
	dk, ok := diskScore()
	res.CPU1, res.CPUN, res.RAM = clampPts(c1/refCPU1), clampPts(cn/refCPUN), clampPts(ram/refRAM)
	parts := []float64{float64(res.CPU1), float64(res.CPUN), float64(res.RAM)}
	weights := []float64{0.3, 0.25, 0.15}
	if ok {
		res.Disk = clampPts(dk)
		parts, weights = append(parts, float64(res.Disk)), append(weights, 0.3)
	} else {
		res.DiskSkip = true
	}
	lg, wsum := 0.0, 0.0
	for i := range parts {
		lg += weights[i] * math.Log(parts[i])
		wsum += weights[i]
	}
	res.Score = int(math.Round(math.Exp(lg / wsum)))

	emit("onBenchProgress", "Guardo quanto è leggero Windows…")
	ls := liveStats()
	rt, _ := ls["ramTotal"].(uint64)
	ru, _ := ls["ramUsed"].(uint64)
	df, _ := ls["diskFree"].(uint64)
	dt, _ := ls["diskTotal"].(uint64)
	if rt > 0 {
		res.RAMPct = int(ru * 100 / rt)
	}
	if dt > 0 {
		res.FreePct = int(df * 100 / dt)
	}
	res.Procs = procCount()
	var an map[string]interface{}
	if r := runPS(analyzeScript, 3*time.Minute, nil); r.JSON != "" {
		_ = json.Unmarshal([]byte(r.JSON), &an)
	}
	if v, ok := an["startup"].(float64); ok {
		res.Startup = int(v)
	}
	if v, ok := an["junk"].(float64); ok {
		res.JunkGB = math.Round(v/(1<<30)*10) / 10
	}
	light := 100.0
	light -= math.Max(0, math.Min(25, float64(res.RAMPct-40)*0.6))   // memoria occupata da ferma
	light -= math.Max(0, math.Min(15, float64(res.Procs-130)*0.15))  // processi in background
	light -= math.Max(0, math.Min(20, float64(res.Startup-4)*2.5))   // programmi all'avvio
	light -= math.Max(0, math.Min(20, float64(20-res.FreePct)*1.25)) // spazio libero sotto il 20%
	light -= math.Max(0, math.Min(20, (res.JunkGB-0.5)*4))           // file inutili
	res.Light = int(math.Round(math.Max(0, light)))

	updateSettings(func(s *Settings) {
		s.Bench = append(s.Bench, res)
		if len(s.Bench) > 12 {
			s.Bench = s.Bench[len(s.Bench)-12:]
		}
	})
	return res
}

// benchNow: il pulsante "Misura il voto".
func benchNow() {
	r := runBench("")
	emit("onBench", map[string]interface{}{"result": r, "history": benchHistory()})
}

func benchHistory() []benchResult {
	setMu.Lock()
	defer setMu.Unlock()
	return append([]benchResult(nil), settings.Bench...)
}

func setBenchAround(on bool) string {
	updateSettings(func(s *Settings) { s.BenchAround = on })
	return "ok"
}

// score100 porta i punti dei pezzi (1000 = PC medio) a un voto da 1 a 100, come nella schermata.
func score100(v int) int {
	x := math.Round(70 + 25*math.Log2(math.Max(1, float64(v))/1000))
	return int(math.Max(1, math.Min(100, x)))
}
