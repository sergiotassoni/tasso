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
	livelloFallitoIA    string       // ultimo livello che non è partito o si è chiuso da solo
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
	// cambio degli strati sulla scheda senza cambiare livello: costa un riavvio dell'IA, quindi
	// solo per differenze vere (almeno mezzo GB) e che durano, non per uno sbalzo del gioco
	margineStratiGB       = 0.5
	controlliPerStringere = 3 // 45 secondi di fila (2 se servono almeno 1 GB in più)
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
	// strati: quanti controlli di fila chiedono meno (o più) strati, e quanti al massimo (o al minimo)
	stretti, stratiStretti, larghi, stratiLarghi := 0, 0, 0, 0
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
			if !scelto && time.Now().Before(prossimoTentativoIA) {
				// un livello ha fallito da poco: finché aspetto, solo i livelli sotto quello
				// (così la Piccola che andava non resta spenta perché la Media non è partita)
				pv = sottoIlFallito(pv, g, ram, core)
			}
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
				fallitaIA(attuale, "il motore IA si è chiuso da solo (forse la memoria video è finita)")
			}
			attuale = ""
		}
		mortiViste = morti
		if attuale != "" && falliteIA > 0 && time.Since(avviataIA) > 10*time.Minute {
			falliteIA = 0 // regge da 10 minuti: i problemi di prima non contano più
		}
		if voluto != "" && time.Now().Before(prossimoTentativoIA) && rangoProfilo(voluto) >= rangoProfilo(livelloFallitoIA) {
			voluto = "" // ha fallito da poco: aspetto prima di riprovare
		}
		// stesso livello, ma il gioco ha preso altra memoria video: servono meno strati sulla
		// scheda. Riavvio con meno strati (uno in meno, di margine) solo se servono almeno mezzo GB
		// e per qualche controllo di fila: uno sbalzo normale del gioco non fa riavviare l'IA.
		// Al contrario, se il gioco ne lascia di più per un minuto, gli strati risalgono.
		stringi, allarga := false, false
		ora, minimo, massimo := stratiDi(attuale)
		if voluto != "" && voluto == attuale && ia.Attivo() && stratiVoluti > 0 && ora > 0 {
			gbOra := memoriaStrati(attuale, ora)
			switch {
			case gbOra-memoriaStrati(attuale, stratiVoluti) >= margineStratiGB:
				larghi = 0
				if stretti == 0 || stratiVoluti > stratiStretti {
					stratiStretti = stratiVoluti // conta il bisogno più piccolo visto di fila
				}
				stretti++
				serve := controlliPerStringere
				if gbOra-memoriaStrati(attuale, stratiStretti) >= 2*margineStratiGB {
					serve = 2
				}
				if stretti >= serve {
					if n := max(stratiStretti-1, minimo); n < ora {
						stringi = true
						impostaStrati(attuale, n)
					}
					stretti = 0
				}
			case memoriaStrati(attuale, min(stratiVoluti-1, massimo))-gbOra >= margineStratiGB:
				stretti = 0
				if larghi == 0 || stratiVoluti < stratiLarghi {
					stratiLarghi = stratiVoluti // conta la memoria più piccola vista di fila
				}
				larghi++
				if larghi >= controlliPerSalire {
					if n := min(stratiLarghi-1, massimo); n > ora && memoriaStrati(attuale, n)-gbOra >= margineStratiGB {
						allarga = true
						impostaStrati(attuale, n)
					}
					larghi = 0
				}
			default:
				stretti, larghi = 0, 0
			}
		} else {
			stretti, larghi = 0, 0
		}
		switch {
		case rangoProfilo(voluto) < rangoProfilo(attuale) || stringi || allarga:
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

// stratiDi: strati sulla scheda adesso, minimo e massimo, per i livelli che si possono stringere
// (0 se no).
func stratiDi(nome string) (ora, minimo, massimo int) {
	switch nome {
	case "piccola":
		return int(stratiPiccolaAtt.Load()), stratiPiccolaMin, stratiPiccola
	case "meta":
		return int(stratiMeta.Load()), stratiParziali, stratiModello - 1
	}
	return 0, 0, 0
}

// memoriaStrati: memoria video del livello con n strati sulla scheda.
func memoriaStrati(nome string, n int) float64 {
	if nome == "piccola" {
		return vramPiccolaStrati(n)
	}
	return vramStrati(n)
}

// sottoIlFallito: durante l'attesa dopo un fallimento, il livello scelto se è sotto quello che
// ha fallito; altrimenti il primo livello più basso che ci sta (Media, poi Piccola); se non ce
// n'è, niente IA (solo Mozilla) fino al prossimo tentativo.
func sottoIlFallito(pv ProfiloCorrettore, g InfoGPU, ram float64, core int) ProfiloCorrettore {
	r := rangoProfilo(livelloFallitoIA)
	if r == 0 {
		return ProfiloCorrettore{} // non so quale è fallito: aspetto, come prima
	}
	if rangoProfilo(pv.Nome) < r {
		return pv
	}
	for _, l := range []string{"media", "piccola"} {
		if p := sceltaLivello(g, ram, core, l); p.Nome != "" && rangoProfilo(p.Nome) < r {
			return p
		}
	}
	return ProfiloCorrettore{}
}

func impostaStrati(nome string, n int) {
	switch nome {
	case "piccola":
		stratiPiccolaAtt.Store(int64(n))
	case "meta":
		stratiMeta.Store(int64(n))
	}
}

// fallitaIA: l'IA (al livello dato) non è partita o si è chiusa da sola. Prima di riprovare
// quel livello aspetto sempre di più: 2, 4, 8, 16, 32 minuti. I livelli sotto si possono usare.
func fallitaIA(livello, motivo string) {
	falliteIA++
	livelloFallitoIA = livello
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
			fallitaIA(nome, "non avviato ("+err.Error()+")")
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
