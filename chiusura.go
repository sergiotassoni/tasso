// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

// chiusura.go: la X della finestra (e Alt+F4) quando ci sono documenti aperti (F16-1).
// Chiudere una scheda modificata chiede conferma: chiudere tutta la finestra deve fare lo stesso.
// La pagina dice a Go se ha documenti aperti (goDocumentiAperti). Se sì, la X non chiude subito:
// Go chiede alla pagina (tassoChiudiFinestra), che controlla se ci sono modifiche non salvate e
// chiede "Chiudi senza salvare". Solo dopo la risposta la pagina chiama goChiudiFinestra.

import (
	"errors"
	"sync"
	"time"
)

// Se la pagina non risponde entro questo tempo (bloccata), la X successiva chiude come prima:
// la finestra non deve mai restare impossibile da chiudere.
const attesaRispostaChiusura = 5 * time.Second

var chiusura struct {
	mu      sync.Mutex
	aperti  bool      // la pagina ha almeno un documento aperto
	numero  int       // l'ultimo aggiornamento ricevuto (le chiamate "Subito" possono arrivare in disordine)
	chiesta time.Time // quando ho chiesto alla pagina; zero = nessuna domanda senza risposta
	// ZP07-8: la pagina ha lavoro non salvato (per lo spegnimento di Windows, che non può aspettare)
	lavoro    bool
	numLavoro int
}

// Impostate da main_windows.go (e da prova_ui.go per le prove).
var (
	chiediChiusuraPagina func() // chiede alla pagina se si può chiudere (tassoChiudiFinestra)
	chiudiFinestraVera   func() // chiude davvero, senza chiedere di nuovo
	// avvisaLavoro: Windows deve sapere perché Tasso PDF ferma lo spegnimento (ShutdownBlockReason)
	avvisaLavoro func(on bool)
)

// goLavoroAperto: la pagina dice se ha lavoro non salvato o non applicato (lavoroNonSalvato).
func goLavoroAperto(n int, on bool) {
	chiusura.mu.Lock()
	if n <= chiusura.numLavoro {
		chiusura.mu.Unlock()
		return
	}
	cambia := chiusura.lavoro != on
	chiusura.numLavoro, chiusura.lavoro = n, on
	chiusura.mu.Unlock()
	if cambia && avvisaLavoro != nil {
		avvisaLavoro(on)
	}
}

// suFineSessione: Windows si spegne o l'utente esce (WM_QUERYENDSESSION, ZP07-8). Con lavoro non
// salvato rispondo «non ancora» (true) e mostro la domanda della X; altrimenti Windows va avanti.
func suFineSessione() bool {
	chiusura.mu.Lock()
	lavoro := chiusura.lavoro
	chiusura.mu.Unlock()
	if !lavoro {
		return false
	}
	if chiediChiusuraPagina != nil {
		chiediChiusuraPagina()
	}
	return true
}

// goDocumentiAperti: la pagina dice se ha documenti aperti. n cresce a ogni chiamata.
func goDocumentiAperti(n int, aperti bool) {
	chiusura.mu.Lock()
	defer chiusura.mu.Unlock()
	if n <= chiusura.numero {
		return
	}
	chiusura.numero, chiusura.aperti = n, aperti
}

// goChiusuraVista: la pagina ha ricevuto la domanda (risponde l'utente, anche dopo molto).
func goChiusuraVista() {
	chiusura.mu.Lock()
	chiusura.chiesta = time.Time{}
	chiusura.mu.Unlock()
}

// goChiudiFinestra: l'utente ha scelto di chiudere (o non c'era niente da salvare).
func goChiudiFinestra() error {
	if chiudiFinestraVera == nil {
		return errors.New("generico|chiusura non disponibile")
	}
	chiudiFinestraVera()
	return nil
}

// chiusuraX decide cosa fa la X. resta = la finestra resta aperta; chiedi = devo chiedere alla pagina.
func chiusuraX(ora time.Time) (resta, chiedi bool) {
	chiusura.mu.Lock()
	defer chiusura.mu.Unlock()
	if !chiusura.aperti {
		return false, false
	}
	if !chiusura.chiesta.IsZero() {
		if ora.Sub(chiusura.chiesta) > attesaRispostaChiusura {
			chiusura.chiesta = time.Time{}
			return false, false // la pagina non risponde: chiudo come prima
		}
		return true, false // ho appena chiesto (doppio clic sulla X): aspetto la risposta
	}
	chiusura.chiesta = ora
	return true, true
}

// suChiusuraX: per webview2.ChiusuraHook. Gira nel filo della finestra: veloce.
func suChiusuraX() bool {
	resta, chiedi := chiusuraX(time.Now())
	if chiedi {
		if chiediChiusuraPagina == nil {
			goChiusuraVista()
			return false
		}
		chiediChiusuraPagina()
	}
	return resta
}
