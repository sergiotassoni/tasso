//go:build windows

package traduce

// Quanto è potente il PC (memoria, processori, scheda video): serve a consigliare il correttore IA.

import (
	"runtime"
	"sync"
	"sync/atomic"
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

var (
	pcOnce sync.Once
	pcRAM  float64
	pcCore int
	pcGPU  atomic.Pointer[InfoGPU] // scheda video misurata una volta (nil = la sto ancora misurando)
)

// gpuMisura: la misura della scheda video è partita (una volta sola).
var gpuMisura atomic.Bool

func pcRisorse() (float64, int) {
	pcOnce.Do(func() {
		pcRAM, pcCore = memoriaPC(), coreFisici()
	})
	// La misura della scheda video usa PowerShell (qualche secondo): la faccio solo quando serve
	// (correttore IA offerto in questa versione) e mai mentre davanti c'è un gioco.
	if pcGPU.Load() == nil && motoreIADisponibile() && !giocoDavanti() && gpuMisura.CompareAndSwap(false, true) {
		go func() {
			defer guard("misura-gpu")
			g := controllaGPU()
			pcGPU.Store(&g)
			emetti("aggiorna", stato())
		}()
	}
	return pcRAM, pcCore
}

// giocoDavanti: in primo piano c'è un altro programma (non Tasso, non il desktop).
func giocoDavanti() bool {
	fg, _, _ := pGetForeground.Call()
	return fg != 0 && !eNostro(exeDiFinestra(fg))
}

// profiloPC: la scelta del correttore per questo PC; ok=false finché la scheda non è misurata.
func profiloPC() (ProfiloCorrettore, bool) {
	ram, core := pcRisorse()
	g := pcGPU.Load()
	if g == nil {
		return ProfiloCorrettore{}, false
	}
	if g.Nome == "" && g.MemoriaGB == 0 { // misura non riuscita o nessuna scheda dedicata
		m := g.Motivo
		if m == "" {
			m = "Non sono riuscito a misurare la scheda video."
		}
		return ProfiloCorrettore{Motivo: m}, true
	}
	// conto la scheda a riposo: il gioco non è ancora aperto
	return sceltaCorrettore(InfoGPU{Nome: g.Nome, MemoriaGB: g.MemoriaGB}, ram, core), true
}

// iaAdattaPC (3.1): l'IA su questo PC può partire? Finché non ho misurato la scheda dico di sì
// (il pulsante resta; la scelta vera la rifà il correttore quando parte).
func iaAdattaPC() bool {
	p, ok := profiloPC()
	return !ok || p.Nome != ""
}

// iaMotivoPC: perché sì o perché no, in una frase.
func iaMotivoPC() string {
	p, _ := profiloPC()
	return p.Motivo
}
