//go:build windows

package traduce

// Sorveglianza del correttore IA mentre si gioca: ogni 15 secondi guarda quanta memoria video
// lascia libera il gioco e sceglie da sola il modo (tutto sulla scheda, in parte sul processore,
// spento). Scende subito se il gioco ha bisogno di memoria; risale solo dopo un minuto stabile.
// Se il processore è troppo occupato, le correzioni si mettono in pausa.

import (
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pGetSystemTimes = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemTimes")
	cpuOccupata     atomic.Bool
	cpuGioco        atomic.Int64 // carico del processore senza l'IA, in millesimi
	profiloIA       atomic.Value // string: "", "piccola", "meta", "scheda"
	avvioSorveglia  sync.Once
)

// solo il filo di sorvegliaIA li usa
var (
	falliteIA           int
	prossimoTentativoIA time.Time
	stratiMeta          atomic.Int64 // strati sulla scheda quando l'IA è divisa (dipende dalla scheda)
)

const (
	ogniControllo      = 15 * time.Second
	controlliPerSalire = 4 // un minuto stabile prima di usare più memoria
	sogliaCPU          = 0.85
)

func rangoProfilo(n string) int {
	switch n {
	case "scheda":
		return 4
	case "media":
		return 3
	case "meta":
		return 2
	case "piccola":
		return 1
	}
	return 0
}

func avviaSorveglianzaIA() {
	avvioSorveglia.Do(func() {
		go sorvegliaCPU()
		go sorvegliaIA()
	})
}

// sorvegliaCPU: carico del processore ogni 2 secondi con GetSystemTimes.
func sorvegliaCPU() {
	defer guard("cpu")
	var ia0, k0, u0 uint64
	leggi := func() (idle, kernel, user uint64) {
		var a, b, c [2]uint32
		pGetSystemTimes.Call(uintptr(unsafe.Pointer(&a)), uintptr(unsafe.Pointer(&b)), uintptr(unsafe.Pointer(&c)))
		f := func(x [2]uint32) uint64 { return uint64(x[1])<<32 | uint64(x[0]) }
		return f(a), f(b), f(c)
	}
	ia0, k0, u0 = leggi()
	l0 := ia.tempoProcessore()
	for {
		time.Sleep(2 * time.Second)
		i1, k1, u1 := leggi()
		l1 := ia.tempoProcessore()
		tot := (k1 - k0) + (u1 - u0) // il tempo kernel comprende quello inattivo
		if tot > 0 {
			// il lavoro dell'IA stessa non conta: altrimenti, con il modello diviso tra scheda e
			// processore, l'IA si metterebbe in pausa da sola a ogni frase
			occupato := float64(tot) - float64(i1-ia0)
			if l1 >= l0 {
				occupato -= float64(l1 - l0)
			}
			occ := occupato / float64(tot)
			cpuGioco.Store(int64(occ * 1000))
			cpuOccupata.Store(occ > sogliaCPU)
		}
		ia0, k0, u0, l0 = i1, k1, u1, l1
	}
}

func sorvegliaIA() {
	defer guard("sorveglia-ia")
	ram, core := memoriaPC(), coreFisici()
	piu := 0
	for {
		voluto := ""
		if acceso.Load() && iaPresente() && leggiImp().Motore != "mozilla" {
			conScorta, conMedia = scortaPresente(), mediaPresente()
			g := controllaGPU()
			attuale, _ := profiloIA.Load().(string)
			if ia.Attivo() {
				// la memoria usata comprende quella del correttore: la tolgo per vedere quanta ne lascia il gioco
				g.UsataGB -= memoriaProfilo(attuale)
				if g.UsataGB < 0 {
					g.UsataGB = 0
				}
			}
			impostaGPU(valutaGPU(g))
			pv := sceltaCorrettore(g, ram, core)
			voluto = pv.Nome
			if pv.Nome == "meta" && pv.Strati > 0 && (attuale != "meta" || !ia.Attivo()) {
				stratiMeta.Store(int64(pv.Strati)) // si usa alla prossima accensione
			}
			if voluto == "meta" {
				libera := ramLibera()
				if attuale == "meta" && ia.Attivo() {
					libera += 2 // la RAM che l'IA usa già si libera se la spengo
				}
				if libera < ramLiberaMeta { // il gioco ha bisogno di RAM: lascio solo Mozilla
					voluto = ""
				}
			}
		}
		attuale, _ := profiloIA.Load().(string)
		if attuale != "" && !ia.Attivo() { // il motore si è chiuso da solo (es. memoria video finita)
			profiloIA.Store("")
			traduttore.ImpostaCorrettore(nil, nil)
			attuale = ""
		}
		if voluto != "" && time.Now().Before(prossimoTentativoIA) {
			voluto = "" // ha fallito da poco: aspetto prima di riprovare
		}
		switch {
		case rangoProfilo(voluto) < rangoProfilo(attuale):
			piu = 0
			applicaProfiloIA(voluto, ram, core)
		case rangoProfilo(voluto) > rangoProfilo(attuale):
			piu++
			if piu >= controlliPerSalire || attuale == "" && piu >= 2 {
				piu = 0
				applicaProfiloIA(voluto, ram, core)
			}
		default:
			piu = 0
		}
		time.Sleep(ogniControllo)
	}
}

func memoriaProfilo(n string) float64 {
	switch n {
	case "scheda":
		return vramIntero
	case "piccola":
		return vramPiccola
	case "media":
		return vramMedia
	case "meta":
		if n := int(stratiMeta.Load()); n > 0 {
			return vramStrati(n)
		}
		return vramMeta
	}
	return 0
}

func applicaProfiloIA(nome string, ram float64, core int) {
	ia.Ferma()
	traduttore.ImpostaCorrettore(nil, nil)
	profiloIA.Store("")
	if nome != "" {
		p := ProfiloCorrettore{Nome: nome, Strati: -1}
		if nome == "meta" {
			p.Strati = int(stratiMeta.Load())
			if p.Strati <= 0 {
				p.Strati = stratiParziali
			}
		}
		ia.impostaProfilo(p)
		modello := fileModelloIA()
		switch nome {
		case "piccola":
			modello = fileScortaIA()
		case "media":
			modello = fileMediaIA()
		}
		if err := ia.Avvia(dirMotoreIA(), modello); err != nil {
			falliteIA++
			attesa := time.Duration(1<<min(falliteIA, 5)) * time.Minute // 2, 4, 8, 16, 32 minuti
			prossimoTentativoIA = time.Now().Add(attesa)
			if falliteIA <= 2 {
				emetti("info", map[string]string{"testo": "Correttore IA non avviato: " + err.Error() + ". Continuo con il traduttore veloce."})
			}
			return
		}
		falliteIA = 0
		profiloIA.Store(nome)
		traduttore.ImpostaCorrettore(motoreIA{ia}, cpuOccupata.Load)
	}
	emetti("aggiorna", stato())
}
