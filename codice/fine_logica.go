package main

// 3.1.1 "Un clic, esci di casa, torni e trovi tutto fatto": la parte senza Windows (si prova anche
// su Linux): la scelta "Quando ha finito", Windows Update per ultimo, la riprova del riavvio e il
// riepilogo del "Bentornato".

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// Quando ha finito la pulizia: lascia acceso, riavvia o spegni.
const (
	fineAcceso  = "acceso"
	fineRiavvia = "riavvia"
	fineSpegni  = "spegni"
)

// fineSecondi: prima di riavviare o spegnere avviso per 60 secondi (si può annullare).
const fineSecondi = 60

// sceltaFine legge la scelta che arriva dalla finestra. Compatibile con la 3.1, dove arrivava un
// vero/falso ("Spegni il PC quando ha finito"): true = spegni, false = lascia acceso.
func sceltaFine(v interface{}) string {
	switch x := v.(type) {
	case bool:
		if x {
			return fineSpegni
		}
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case fineRiavvia:
			return fineRiavvia
		case fineSpegni:
			return fineSpegni
		}
	}
	return fineAcceso
}

// winupdatePerUltimo: gli aggiornamenti di Windows sempre per ultimi (dopo tutta la pulizia), così
// il riavvio finale li completa e un aggiornamento lungo non tiene ferma la pulizia. L'ordine degli
// altri passaggi resta quello di prima.
func winupdatePerUltimo(sel []Step) []Step {
	out := make([]Step, 0, len(sel))
	var wu []Step
	for _, s := range sel {
		if s.ID == "winupdate" {
			wu = append(wu, s)
			continue
		}
		out = append(out, s)
	}
	return append(out, wu...)
}

// senzaPasso toglie un passaggio dalla lista (es. "Aggiorna anche Windows" tolto).
func senzaPasso(sel []Step, id string) []Step {
	out := make([]Step, 0, len(sel))
	for _, s := range sel {
		if s.ID != id {
			out = append(out, s)
		}
	}
	return out
}

// Codici di uscita di shutdown.exe che contano qui.
const (
	errSpegnimentoProgrammato = 1190 // ERROR_SHUTDOWN_IS_SCHEDULED: c'è già un riavvio/spegnimento in attesa
)

// esitoShutdown: com'è andata con shutdown.exe.
type esitoShutdown struct {
	OK        bool
	Codice    int  // l'ultimo codice di uscita (0 = accettato)
	Riprovato bool // c'era già uno spegnimento programmato: l'ho annullato e ho riprovato
}

// shutdownConRiprova chiede a Windows di riavviare/spegnere con shutdown.exe. run esegue
// shutdown.exe con quegli argomenti e restituisce il codice di uscita (nei test è una funzione finta).
// Se Windows risponde 1190 (ce n'è già uno programmato, magari da Windows Update o da un'altra app)
// lo annullo con /a e riprovo una volta. Con un altro errore non insisto: ci pensa ExitWindowsEx.
func shutdownConRiprova(run func(args ...string) int, args []string) esitoShutdown {
	c := run(args...)
	if c == 0 {
		return esitoShutdown{OK: true}
	}
	if c != errSpegnimentoProgrammato {
		return esitoShutdown{Codice: c}
	}
	run("/a")
	c = run(args...)
	return esitoShutdown{OK: c == 0, Codice: c, Riprovato: true}
}

// riservaSecondi: anche quando shutdown.exe risponde "ok", se dopo il suo tempo + questi secondi
// Tasso è ancora vivo (Windows non è partito davvero: il caso del portatile di Sergio, dove il conto
// alla rovescia si vedeva ma il PC non si riavviava) ci provo da solo con ExitWindowsEx. La finestra
// dice "non è partito" dopo il tempo + 30 s, quindi prima tento io.
const riservaSecondi = 20

// fineDeps: le parti Windows di riavvio/spegnimento (nei test sono finte).
type fineDeps struct {
	shutdown    func(args ...string) int // shutdown.exe, restituisce il codice di uscita
	puoSpegnere func() bool              // il privilegio di spegnimento c'è
	arma        func(dopo time.Duration) // conto alla rovescia di Tasso, poi ExitWindowsEx (annullabile)
	esci        func() error             // ExitWindowsEx subito
}

// pianificaFineCon: riavvia/spegne tra secs secondi. Prima shutdown.exe (con riprova se ce n'era già
// uno programmato) e in più la riserva di Tasso (ExitWindowsEx dopo secs+riservaSecondi, se Windows
// non è partito); se shutdown.exe non accetta, il conto alla rovescia lo tiene Tasso (ExitWindowsEx
// dopo secs). false = non si è potuto (la finestra lo dice).
func pianificaFineCon(d fineDeps, riavvia bool, secs int, msg string) bool {
	k := "/s"
	if riavvia {
		k = "/r"
	}
	args := []string{k, "/t", strconv.Itoa(secs)}
	if msg != "" {
		args = append(args, "/c", msg)
	}
	if shutdownConRiprova(d.shutdown, args).OK {
		d.arma(time.Duration(secs+riservaSecondi) * time.Second)
		return true
	}
	if !d.puoSpegnere() {
		return false
	}
	d.arma(time.Duration(secs) * time.Second)
	return true
}

// adessoFineCon: "Riavvia adesso" / "Spegni adesso". Prima shutdown.exe /t 0 (con riprova) più la
// riserva di Tasso; se non accetta, ExitWindowsEx subito.
func adessoFineCon(d fineDeps, riavvia bool) bool {
	k := "/s"
	if riavvia {
		k = "/r"
	}
	if shutdownConRiprova(d.shutdown, []string{k, "/t", "0"}).OK {
		d.arma(time.Duration(riservaSecondi) * time.Second)
		return true
	}
	return d.esci() == nil
}

// passoFatto: un passaggio della pulizia finita, per il "Bentornato".
type passoFatto struct {
	Nome     string `json:"name"`
	Liberato int64  `json:"freed"`
	Saltato  bool   `json:"skipped,omitempty"`
}

// riepilogoPulizia: quello che il "Bentornato" mostra al prossimo avvio dopo il riavvio/spegnimento
// scelto nella pulizia.
type riepilogoPulizia struct {
	Livello       int          `json:"level"`
	Fine          string       `json:"end"`  // riavvia o spegni
	FinitaAlle    string       `json:"at"`   // "23:12"
	Quando        int64        `json:"when"` // Unix: per capire se dopo c'è stato davvero un riavvio
	Liberato      int64        `json:"freed"`
	Aggiornamenti int          `json:"updates"`
	Fatti         int          `json:"done"`
	Totale        int          `json:"total"`
	Passi         []passoFatto `json:"steps"`
}

// passiInVista: i passaggi da mostrare nel "Bentornato": quelli che hanno liberato più spazio
// (al massimo n, a pari spazio l'ordine della pulizia), poi quanti altri ce ne sono e quelli saltati.
func passiInVista(passi []passoFatto, n int) (vista []passoFatto, altri int, saltati []string) {
	var ok []passoFatto
	for _, p := range passi {
		if p.Saltato {
			saltati = append(saltati, p.Nome)
			continue
		}
		ok = append(ok, p)
	}
	sort.SliceStable(ok, func(i, j int) bool { return ok[i].Liberato > ok[j].Liberato })
	if len(ok) > n {
		return ok[:n], len(ok) - n, saltati
	}
	return ok, 0, saltati
}

// bentornatoDaMostrare: il riepilogo va mostrato solo se dopo la fine della pulizia il PC è stato
// davvero riavviato o spento (Windows acceso dopo quell'ora) e non è troppo vecchio (7 giorni).
func bentornatoDaMostrare(r riepilogoPulizia, avvioWindows, adesso int64) bool {
	if r.Quando <= 0 || (r.Fine != fineRiavvia && r.Fine != fineSpegni) {
		return false
	}
	if adesso-r.Quando > 7*24*3600 {
		return false
	}
	return avvioWindows > r.Quando
}
