package main

// Temperatura e uso delle schede video AMD Radeon.
// Tasso chiama la libreria "ADL" che il driver AMD installa già in Windows (atiadlxx.dll):
// nessun programma esterno, nessun file di altri dentro Tasso. Il codice qui è scritto da zero.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	adlDLL      = windows.NewLazySystemDLL("atiadlxx.dll")
	adlCreate   = adlDLL.NewProc("ADL2_Main_Control_Create")
	adlNumAd    = adlDLL.NewProc("ADL2_Adapter_NumberOfAdapters_Get")
	adlAdInfo   = adlDLL.NewProc("ADL2_Adapter_AdapterInfo_Get")
	adlPMLog    = adlDLL.NewProc("ADL2_New_QueryPMLogData_Get")
	pLocalAlloc = kernel32.NewProc("LocalAlloc")
	adlMu       sync.Mutex
	adlCtx      uintptr
	adlIndex    int32 = -1
	adlName     string
	adlTried    bool
	adlAllocCb  = windows.NewCallback(func(size uintptr) uintptr { r, _, _ := pLocalAlloc.Call(0x40, size); return r })
)

// Struttura AdapterInfo di ADL (1572 byte).
type adlAdapterInfo struct {
	Size          int32
	AdapterIndex  int32
	UDID          [256]byte
	Bus, Dev, Fn  int32
	VendorID      int32
	AdapterName   [256]byte
	DisplayName   [256]byte
	Present       int32
	Exist         int32
	DriverPath    [256]byte
	DriverPathExt [256]byte
	PNPString     [256]byte
	OSDisplayIdx  int32
}

type adlSensor struct{ Supported, Value int32 }

type adlPMLogOut struct {
	Size    int32
	Sensors [256]adlSensor
}

// Posizioni dei sensori nella lista di ADL (ADL_PMLOG_SENSORS).
const (
	adlFanPct    = 15
	adlTempEdge  = 8
	adlActGfx    = 19
	adlTempHot   = 27
	adlTempMem   = 9
	adlFanRPM    = 14
	adlAsicPower = 23
)

func adlInit() bool {
	adlMu.Lock()
	defer adlMu.Unlock()
	if adlTried {
		return adlIndex >= 0
	}
	adlTried = true
	if adlDLL.Load() != nil || adlCreate.Find() != nil || adlPMLog.Find() != nil {
		return false
	}
	if r, _, _ := adlCreate.Call(adlAllocCb, 1, uintptr(unsafe.Pointer(&adlCtx))); r != 0 {
		return false
	}
	var n int32
	adlNumAd.Call(adlCtx, uintptr(unsafe.Pointer(&n)))
	if n <= 0 || n > 64 {
		return false
	}
	infos := make([]adlAdapterInfo, n)
	for i := range infos {
		infos[i].Size = int32(unsafe.Sizeof(infos[0]))
	}
	if r, _, _ := adlAdInfo.Call(adlCtx, uintptr(unsafe.Pointer(&infos[0])), uintptr(len(infos))*unsafe.Sizeof(infos[0])); r != 0 {
		return false
	}
	for _, in := range infos {
		if in.VendorID != 1002 && in.VendorID != 0x1002 {
			continue
		}
		var out adlPMLogOut
		out.Size = int32(unsafe.Sizeof(out))
		if r, _, _ := adlPMLog.Call(adlCtx, uintptr(in.AdapterIndex), uintptr(unsafe.Pointer(&out))); r == 0 && out.Sensors[adlTempEdge].Supported != 0 {
			adlIndex = in.AdapterIndex
			adlName = cstr(in.AdapterName[:])
			adlDump(&out)
			return true
		}
	}
	return false
}

func cstr(b []byte) string {
	if i := strings.IndexByte(string(b), 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimSpace(string(b))
}

// adlDump scrive una volta l'elenco dei sensori nei registri: serve a controllare che i numeri siano giusti.
func adlDump(out *adlPMLogOut) {
	var sb strings.Builder
	sb.WriteString("Sensori AMD (" + adlName + ") " + time.Now().Format("2006-01-02 15:04") + "\r\n")
	for i := 0; i < 60; i++ {
		if out.Sensors[i].Supported != 0 {
			sb.WriteString(fmt.Sprintf("%d = %d\r\n", i, out.Sensors[i].Value))
		}
	}
	_ = os.WriteFile(filepath.Join(appDir, "logs", "sensori_amd.txt"), []byte(sb.String()), 0o644)
}

// sampleAMD legge temperatura, uso e ventola. Restituisce false se non c'è una Radeon leggibile.
func sampleAMD() bool {
	if !adlInit() {
		return false
	}
	var out adlPMLogOut
	out.Size = int32(unsafe.Sizeof(out))
	adlMu.Lock()
	r, _, _ := adlPMLog.Call(adlCtx, uintptr(adlIndex), uintptr(unsafe.Pointer(&out)))
	adlMu.Unlock()
	if r != 0 {
		return false
	}
	g := map[string]interface{}{"name": adlName}
	val := func(i int, lo, hi int32) interface{} {
		s := out.Sensors[i]
		if s.Supported == 0 || s.Value < lo || s.Value > hi {
			return nil
		}
		return float64(s.Value)
	}
	if t := val(adlTempEdge, 1, 125); t != nil {
		g["temp"] = t
	}
	if t := val(adlTempHot, 1, 125); t != nil {
		g["hotspot"] = t
	}
	if u := val(adlActGfx, 0, 100); u != nil {
		g["util"] = u
	}
	if f := val(adlFanPct, 0, 100); f != nil {
		g["fan"] = f
	}
	if _, ok := g["temp"]; !ok {
		return false
	}
	liveMu.Lock()
	if gpuLive != nil {
		if u, ok := gpuLive["util"]; ok && gpuLive["temp"] != nil {
			g["util"] = u // tengo la percentuale dei contatori di Windows se c'è già
		}
	}
	gpuLive, gpuAt, gpuTempAt = g, time.Now(), time.Now()
	liveMu.Unlock()
	return true
}
