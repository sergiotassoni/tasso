//go:build windows

package traduce

// Sorveglianza del correttore IA mentre si gioca: ogni 15 secondi guarda quanta memoria video
// lascia libera il gioco e sceglie da sola il modo (tutto sulla scheda, in parte sul processore,
// spento). Scende subito se il gioco ha bisogno di memoria; risale solo dopo un minuto stabile.
// Se il processore è troppo occupato, le correzioni si mettono in pausa.

import (
	"fmt"
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
	avviataIA           time.Time    // ultima accensione riuscita: dopo 10 minuti senza problemi i fallimenti si azzerano
	mortiViste          int64        // chiusure da sole del motore già contate
	stratiMeta          atomic.Int64 // strati sulla scheda quando l'IA è divisa (dipende dalla scheda)
	stratiPiccolaAtt    atomic.Int64 // strati della Piccola sulla scheda (si stringe con i giochi pesanti)
)

// livelloCambiato: l'utente ha appena cambiato livello dell'IA: si applica subito, anche salendo.
var livelloCambiato atomic.Bool

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

// svegliaIA: la traduzione è appena partita (F7 o gioco riconosciuto): controllo subito se accendere l'IA.
var svegliaIA = make(chan struct{}, 1)

func sveglia() {
	select {
	case svegliaIA <- struct{}{}:
	default:
	}
}

func sorvegliaIA() {
	defer guard("sorveglia-ia")
	ram, core := memoriaPC(), coreFisici()
	piu := 0
	for {
		scelto := livelloCambiato.Swap(false)
		if scelto {
			prossimoTentativoIA = time.Time{} // scelta dell'utente: riprovo subito, anche dopo un fallimento
		}
		voluto, stratiVoluti := "", 0
		if acceso.Load() && iaPresente() && leggiImp().Motore != "mozilla" {
			conMedia = mediaPresente()
			conScorta = conMedia // la Piccola è la Media divisa tra scheda e processore
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
			pv := sceltaLivello(g, ram, core, leggiImp().LivelloIA)
			voluto, stratiVoluti = pv.Nome, pv.Strati
			if pv.Nome == "piccola" && pv.Strati > 0 && (attuale != "piccola" || !ia.Attivo()) {
				stratiPiccolaAtt.Store(int64(pv.Strati))
			}
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
		morti := ia.morti.Load()
		if attuale != "" && !ia.Attivo() { // il motore si è chiuso
			profiloIA.Store("")
			traduttore.ImpostaCorrettore(nil, nil)
			if morti != mortiViste {
				// chiuso da solo (es. memoria video finita): conta come un fallimento, con un'attesa
				// che cresce, invece di ricaricare 5 GB ogni 15 secondi e far scattare il gioco
				fallitaIA("il motore IA si è chiuso da solo (forse la memoria video è finita)")
			}
			attuale = ""
		}
		mortiViste = morti
		if attuale != "" && falliteIA > 0 && time.Since(avviataIA) > 10*time.Minute {
			falliteIA = 0 // regge da 10 minuti: i problemi di prima non contano più
		}
		if voluto != "" && time.Now().Before(prossimoTentativoIA) {
			voluto = "" // ha fallito da poco: aspetto prima di riprovare
		}
		// stesso livello, ma il gioco ha preso altra memoria video: servono meno strati sulla
		// scheda. Lo tratto come una discesa: riavvio con meno strati (uno in meno, di margine).
		stringi := false
		if voluto != "" && voluto == attuale && ia.Attivo() && stratiVoluti > 0 {
			if ora, minimo := stratiDi(attuale); ora > 0 && stratiVoluti <= ora-2 {
				if n := max(stratiVoluti-1, minimo); n < ora {
					stringi = true
					impostaStrati(attuale, n)
				}
			}
		}
		switch {
		case rangoProfilo(voluto) < rangoProfilo(attuale) || stringi:
			piu = 0
			applicaProfiloIA(voluto, ram, core)
			mortiViste = ia.morti.Load() // le chiusure durante un avvio fallito le conta già applicaProfiloIA
		case rangoProfilo(voluto) > rangoProfilo(attuale):
			piu++
			// da spenta parte subito (al primo controllo, cioè appena premi F7): prima servivano
			// due controlli (30 s) e in una partita breve l'IA non arrivava mai a correggere.
			// Anche quando l'utente ha appena scelto il livello.
			if piu >= controlliPerSalire || attuale == "" || scelto {
				piu = 0
				applicaProfiloIA(voluto, ram, core)
				mortiViste = ia.morti.Load()
			}
		default:
			piu = 0
		}
		select {
		case <-svegliaIA:
		case <-time.After(ogniControllo):
		}
	}
}

func memoriaProfilo(n string) float64 {
	switch n {
	case "scheda":
		return vramIntero
	case "piccola":
		if n := int(stratiPiccolaAtt.Load()); n > 0 {
			return vramPiccolaStrati(n)
		}
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

// stratiDi: strati sulla scheda adesso e minimo, per i livelli che si possono stringere (0 se no).
func stratiDi(nome string) (ora, minimo int) {
	switch nome {
	case "piccola":
		return int(stratiPiccolaAtt.Load()), stratiPiccolaMin
	case "meta":
		return int(stratiMeta.Load()), stratiParziali
	}
	return 0, 0
}

func impostaStrati(nome string, n int) {
	switch nome {
	case "piccola":
		stratiPiccolaAtt.Store(int64(n))
	case "meta":
		stratiMeta.Store(int64(n))
	}
}

// fallitaIA: l'IA non è partita o si è chiusa da sola. Prima di riprovare aspetto sempre di più:
// 2, 4, 8, 16, 32 minuti.
func fallitaIA(motivo string) {
	falliteIA++
	attesa := time.Duration(1<<min(falliteIA, 5)) * time.Minute
	prossimoTentativoIA = time.Now().Add(attesa)
	if falliteIA <= 2 {
		emetti("info", map[string]string{"testo": "Correttore IA: " + motivo + ". Continuo con il traduttore veloce e riprovo tra " + fmt.Sprint(int(attesa.Minutes())) + " minuti."})
	}
}

func applicaProfiloIA(nome string, ram float64, core int) {
	ia.Ferma()
	traduttore.ImpostaCorrettore(nil, nil)
	profiloIA.Store("")
	if nome != "" {
		p := ProfiloCorrettore{Nome: nome, Strati: -1}
		if nome == "piccola" {
			p.Strati = int(stratiPiccolaAtt.Load())
			if p.Strati <= 0 {
				p.Strati = stratiPiccola
			}
		}
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
			modello = fileMediaIA()
		case "media":
			modello = fileMediaIA()
		}
		if err := ia.Avvia(dirMotoreIA(), modello); err != nil {
			fallitaIA("non avviato (" + err.Error() + ")")
			return
		}
		// mentre il modello caricava (anche qualche minuto) la traduzione può essere stata spenta
		// o l'utente può aver scelto solo Mozilla: allora lo spengo subito
		if !acceso.Load() || leggiImp().Motore == "mozilla" {
			ia.Ferma()
			emetti("aggiorna", stato())
			return
		}
		avviataIA = time.Now()
		profiloIA.Store(nome)
		traduttore.ImpostaCorrettore(motoreIA{ia}, cpuOccupata.Load)
	}
	emetti("aggiorna", stato())
}
