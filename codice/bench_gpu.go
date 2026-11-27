package main

// Voto al PC (3.2): scheda video letta da DirectX (DXGI, senza PowerShell e senza scaricare
// niente), dati di RAM, processore e scheda madre, temperatura sotto carico se leggibile, e il
// salvataggio / la copia negli appunti della "cartolina" del voto fatta dalla finestra.

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"tasso/votopc"
)

var (
	dxgiDLL            = windows.NewLazySystemDLL("dxgi.dll")
	pCreateDXGIFactory = dxgiDLL.NewProc("CreateDXGIFactory1")
	iidDXGIFactory1    = windows.GUID{Data1: 0x770aae78, Data2: 0xf26f, Data3: 0x4dba, Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87}}
)

// dxgiAdapterDesc1 = DXGI_ADAPTER_DESC1.
type dxgiAdapterDesc1 struct {
	Description           [128]uint16
	VendorID              uint32
	DeviceID              uint32
	SubSysID              uint32
	Revision              uint32
	DedicatedVideoMemory  uintptr
	DedicatedSystemMemory uintptr
	SharedSystemMemory    uintptr
	LuidLow               uint32
	LuidHigh              int32
	Flags                 uint32
}

func comCall(obj uintptr, idx int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(puntatore(obj))
	fn := *(*uintptr)(puntatore(vtbl + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{obj}, args...)...)
	return r
}

// schedeVideo: tutte le schede che vede DirectX (integrata, dedicata, adattatori finti).
func schedeVideo() (out []votopc.Scheda) {
	defer guard("bench_gpu.go:schedeVideo")
	if pCreateDXGIFactory.Find() != nil {
		return nil
	}
	var fac uintptr
	if hr, _, _ := pCreateDXGIFactory.Call(uintptr(unsafe.Pointer(&iidDXGIFactory1)), uintptr(unsafe.Pointer(&fac))); hr != 0 || fac == 0 {
		return nil
	}
	defer comCall(fac, 2) // Release
	for i := uintptr(0); i < 16; i++ {
		var ad uintptr
		if hr := comCall(fac, 12, i, uintptr(unsafe.Pointer(&ad))); hr != 0 || ad == 0 { // EnumAdapters1
			break
		}
		var d dxgiAdapterDesc1
		hr := comCall(ad, 10, uintptr(unsafe.Pointer(&d))) // GetDesc1
		comCall(ad, 2)
		if hr != 0 {
			continue
		}
		out = append(out, votopc.Scheda{
			Nome:     strings.TrimSpace(windows.UTF16ToString(d.Description[:])),
			VRAMMB:   uint64(d.DedicatedVideoMemory) >> 20,
			Vendor:   d.VendorID,
			Software: d.Flags&2 != 0, // DXGI_ADAPTER_FLAG_SOFTWARE
		})
	}
	return out
}

// infoPC: nomi e numeri che Windows conosce (servono per il risultato e per il voto della RAM).
type infoPC struct {
	CPU    string  `json:"cpu"`
	Board  string  `json:"board"`
	BIOS   string  `json:"bios"`
	RAMGB  float64 `json:"ramGB"`
	RAMMTs int     `json:"ramMTs"`
	RAMDDR int     `json:"ramDdr"`
	Banchi int     `json:"banchi"`
}

const infoPCScript = `
$cpu = Get-CimInstance Win32_Processor -ErrorAction SilentlyContinue | Select-Object -First 1
$bb = Get-CimInstance Win32_BaseBoard -ErrorAction SilentlyContinue | Select-Object -First 1
$bi = Get-CimInstance Win32_BIOS -ErrorAction SilentlyContinue | Select-Object -First 1
$mem = @(Get-CimInstance Win32_PhysicalMemory -ErrorAction SilentlyContinue)
$tot = [long]0; $mts = 0; $ddr = 0
foreach ($m in $mem) {
  if ($m.Capacity) { $tot += [long]$m.Capacity }
  $s = [int]$m.ConfiguredClockSpeed; if (-not $s) { $s = [int]$m.Speed }
  if ($s -gt $mts) { $mts = $s }
  $t = [int]$m.SMBIOSMemoryType; if ($t -eq 24) { $ddr = 3 } elseif ($t -eq 26) { $ddr = 4 } elseif ($t -eq 34) { $ddr = 5 } elseif ($t -eq 35) { $ddr = 5 }
}
$o = [ordered]@{
  cpu = (([string]$cpu.Name) -replace '\s+', ' ').Trim()
  board = ((([string]$bb.Manufacturer).Trim() + ' ' + ([string]$bb.Product).Trim()).Trim())
  bios = ((([string]$bi.SMBIOSBIOSVersion).Trim()))
  ramGB = [math]::Round($tot / 1GB, 0); ramMTs = $mts; ramDdr = $ddr; banchi = $mem.Count
}
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject $o -Compress))
`

// Nomi di scheda madre che non dicono niente (PC assemblati senza dati nel BIOS).
var reNomeVuoto = regexp.MustCompile(`(?i)^(to be filled by o\.?e\.?m\.?|default string|system product name|not applicable|n/?a|none|unknown|o\.?e\.?m\.?)?$`)

func leggiInfoPC() infoPC {
	var in infoPC
	if r := runPS(infoPCScript, 40*time.Second, nil); r.JSON != "" {
		_ = json.Unmarshal([]byte(r.JSON), &in)
	}
	for _, p := range []*string{&in.Board, &in.BIOS} {
		var parti []string
		for _, w := range strings.Fields(*p) {
			if !reNomeVuoto.MatchString(w) {
				parti = append(parti, w)
			}
		}
		*p = strings.Join(parti, " ")
		if reNomeVuoto.MatchString(*p) || strings.Contains(strings.ToLower(*p), "filled by") || strings.Contains(strings.ToLower(*p), "default string") {
			*p = ""
		}
	}
	if in.RAMMTs < 400 || in.RAMMTs > 20000 {
		in.RAMMTs = 0
	}
	return in
}

// tempSottoCarico: solo se c'è LibreHardwareMonitor aperto (l'unico modo affidabile di leggere la
// temperatura vera del processore, come nella pagina Hardware). Scalda tutti i core per circa 8
// secondi e legge la temperatura mentre il carico è ancora acceso. 0 = non letta.
func tempSottoCarico() float64 {
	if !exeRunning([]string{"LibreHardwareMonitor.exe"}) {
		return 0
	}
	emit("onBenchProgress", "Misuro il raffreddamento: scaldo il processore per 8 secondi…")
	var stop atomic.Bool
	done := make(chan struct{})
	n := runtime.NumCPU() - 1 // un core resta libero per leggere il sensore
	if n < 1 {
		n = 1
	}
	for i := 0; i < n; i++ {
		go func() {
			defer guard("bench_gpu.go:caldo")
			cpuWork(&stop)
			done <- struct{}{}
		}()
	}
	time.Sleep(5 * time.Second)
	best := 0.0
	for k := 0; k < 2; k++ {
		r := runPS(cpuTempLHMScript, 6*time.Second, nil)
		if f, err := strconv.ParseFloat(strings.TrimSpace(r.JSON), 64); err == nil && f > 15 && f < 115 && f > best {
			best = f
		}
	}
	stop.Store(true)
	for i := 0; i < n; i++ {
		<-done
	}
	return best
}

// ---------------- Cartolina del voto: salva e copia ----------------

func pngDaDataURL(s string) ([]byte, image.Image, error) {
	const pre = "data:image/png;base64,"
	if !strings.HasPrefix(s, pre) || len(s) > 40<<20 {
		return nil, nil, errors.New("immagine non valida")
	}
	b, err := base64.StdEncoding.DecodeString(s[len(pre):])
	if err != nil {
		return nil, nil, err
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, nil, err
	}
	if r := img.Bounds(); r.Dx() > 4096 || r.Dy() > 4096 {
		return nil, nil, errors.New("immagine troppo grande")
	}
	return b, img, nil
}

// salvaCartolina: Immagini\Tasso\voto-tasso-AAAA-MM-GG-hhmm-forma.png, poi apre la cartella con il
// file selezionato.
func salvaCartolina(dataURL, forma string) string {
	b, _, err := pngDaDataURL(dataURL)
	if err != nil {
		return "no"
	}
	dir, err := windows.KnownFolderPath(windows.FOLDERID_Pictures, 0)
	if err != nil || dir == "" {
		return "no"
	}
	dir = filepath.Join(dir, "Tasso")
	if os.MkdirAll(dir, 0o755) != nil {
		return "no"
	}
	if forma != "verticale" {
		forma = "quadrata"
	}
	base := "voto-tasso-" + time.Now().Format("2006-01-02-1504") + "-" + forma
	p := filepath.Join(dir, base+".png")
	for i := 2; i < 100; i++ {
		if _, err := os.Stat(p); err != nil {
			break
		}
		p = filepath.Join(dir, base+"-"+strconv.Itoa(i)+".png")
	}
	if os.WriteFile(p, b, 0o644) != nil {
		return "no"
	}
	openFolderOf(p)
	return "ok"
}

// dibDa: l'immagine come CF_DIB (BITMAPINFOHEADER + pixel BGRA dal basso verso l'alto), il formato
// che capiscono tutti i programmi (Paint, Word, Discord, WhatsApp Desktop...).
func dibDa(img image.Image) []byte {
	r := img.Bounds()
	w, h := r.Dx(), r.Dy()
	out := make([]byte, 40+w*h*4)
	binary.LittleEndian.PutUint32(out[0:], 40)
	binary.LittleEndian.PutUint32(out[4:], uint32(w))
	binary.LittleEndian.PutUint32(out[8:], uint32(h))
	binary.LittleEndian.PutUint16(out[12:], 1)
	binary.LittleEndian.PutUint16(out[14:], 32)
	binary.LittleEndian.PutUint32(out[20:], uint32(w*h*4))
	px := out[40:]
	for y := 0; y < h; y++ {
		row := px[(h-1-y)*w*4:]
		for x := 0; x < w; x++ {
			cr, cg, cb, _ := img.At(r.Min.X+x, r.Min.Y+y).RGBA()
			row[x*4], row[x*4+1], row[x*4+2], row[x*4+3] = byte(cb>>8), byte(cg>>8), byte(cr>>8), 255
		}
	}
	return out
}

// copiaCartolina: negli appunti come immagine (CF_DIB) e anche come "PNG" per chi lo preferisce.
func copiaCartolina(dataURL string) string {
	b, img, err := pngDaDataURL(dataURL)
	if err != nil {
		return "no"
	}
	dib := dibDa(img)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if !openClip() {
		return "no"
	}
	defer pCloseClip.Call()
	pEmptyClip.Call()
	ok := false
	if h := globalBytes(dib); h != 0 {
		if r, _, _ := pSetClipData.Call(8, h); r != 0 { // CF_DIB
			ok = true
		} else {
			pGlobalFree.Call(h)
		}
	}
	if n, _ := windows.UTF16PtrFromString("PNG"); n != nil {
		if f, _, _ := pRegClipFmt.Call(uintptr(unsafe.Pointer(n))); f != 0 {
			if h := globalBytes(b); h != 0 {
				if r, _, _ := pSetClipData.Call(f, h); r != 0 {
					ok = true
				} else {
					pGlobalFree.Call(h)
				}
			}
		}
	}
	if !ok {
		return "no"
	}
	return "ok"
}
