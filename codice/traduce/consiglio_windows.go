//go:build windows

package traduce

import (
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pGlobalMemoryStatusEx               = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")
	pGetPhysicallyInstalledSystemMemory = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetPhysicallyInstalledSystemMemory")
	pGetLogicalProcessorInformationEx   = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetLogicalProcessorInformationEx")
)

type statoMemoria struct {
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

func statoRAM() (statoMemoria, bool) {
	var m statoMemoria
	m.Length = uint32(unsafe.Sizeof(m))
	r, _, _ := pGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m)))
	return m, r != 0
}

// memoriaPC: la RAM montata sul PC in GB. Uso quella installata (un PC da 16 GB ne mostra a
// Windows circa 15,8 perché un pezzo è riservato): così i PC da 16 GB non restano esclusi.
func memoriaPC() float64 {
	var kb uint64
	if r, _, _ := pGetPhysicallyInstalledSystemMemory.Call(uintptr(unsafe.Pointer(&kb))); r != 0 && kb > 0 {
		return float64(kb) / (1024 * 1024)
	}
	if m, ok := statoRAM(); ok {
		return float64(m.TotalPhys) / gb
	}
	return 0
}

// ramLibera: RAM disponibile adesso, in GB (il gioco aperto compreso).
func ramLibera() float64 {
	if m, ok := statoRAM(); ok {
		return float64(m.AvailPhys) / gb
	}
	return 0
}

// coreFisici: i core veri del processore (un 4 core con 8 thread conta 4). Le regole di Sergio
// parlano di core: "almeno 8 core" per far lavorare l'IA anche sul processore.
func coreFisici() int {
	var n uint32
	pGetLogicalProcessorInformationEx.Call(0, 0, uintptr(unsafe.Pointer(&n))) // 0 = RelationProcessorCore
	if n == 0 || n > 1<<20 {
		return runtime.NumCPU()
	}
	buf := make([]byte, n)
	if r, _, _ := pGetLogicalProcessorInformationEx.Call(0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n))); r == 0 {
		return runtime.NumCPU()
	}
	if c := contaCore(buf[:n]); c > 0 {
		return c
	}
	return runtime.NumCPU()
}

func analizzaPC() AnalisiPC {
	g := controllaGPU()
	return consiglioPC(g, memoriaPC(), coreFisici(), motoreIADisponibile())
}

var (
	pcOnce sync.Once
	pcRAM  float64
	pcCore int
)

func pcRisorse() (float64, int) {
	pcOnce.Do(func() { pcRAM, pcCore = memoriaPC(), coreFisici() })
	return pcRAM, pcCore
}

// iaAdattaPC (3.1): l'IA su questo PC può partire? (memoria, processore e scheda video a riposo)
func iaAdattaPC() bool {
	ram, core := pcRisorse()
	return sceltaCorrettore(InfoGPU{Nome: gpu.Nome, MemoriaGB: gpu.MemoriaGB}, ram, core).Nome != ""
}

// iaMotivoPC: perché sì o perché no, in una frase.
func iaMotivoPC() string {
	ram, core := pcRisorse()
	return sceltaCorrettore(InfoGPU{Nome: gpu.Nome, MemoriaGB: gpu.MemoriaGB}, ram, core).Motivo
}
