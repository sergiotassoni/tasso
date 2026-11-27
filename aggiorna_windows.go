package main

// Aggiornamento silenzioso dell'Ecosistema Tasso, avviato da Tasso (Impostazioni → Aggiornamenti):
//
//	TassoSetup.exe --aggiorna-silenzioso [--riapri-tasso] [--solo-controlla]
//
// Aggiorna SOLO i programmi già installati (stessa cartella, stesse caselle, stessa lingua, il
// Desktop solo i collegamenti che c'erano già) e solo quelli che in questo installatore sono più
// nuovi: mai una versione più vecchia. Nessuna pagina dell'installatore: solo una finestrella con
// la barra. Prima chiude bene i programmi da aggiornare (Tasso con "Tasso.exe --chiudi", Zip e
// PDF come allo spegnimento di Windows); alla fine riapre Tasso se era aperto.
//   --riapri-tasso   Tasso ci ha avviato e si sta chiudendo da solo: lo aspetto e alla fine lo riapro.
//   --solo-controlla nessuna finestra e nessuna modifica: dice solo se c'è qualcosa da aggiornare.
// Codici d'uscita: 0 aggiornato (con --solo-controlla: c'è qualcosa da aggiornare),
// 1 errore (sul PC non è cambiato niente), 2 niente da fare (non installato, o versioni uguali o
// più vecchie).

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/webviewloader"
	"golang.org/x/sys/windows"

	"tassosetup/installa"
	"tassosetup/pacco"
	"tassosetup/testi"
	"tassosetup/ui"
)

const (
	argAggiornaSilenzioso = "--aggiorna-silenzioso"
	argSoloControlla      = "--solo-controlla"
	argRiapriTasso        = "--riapri-tasso"
)

// pianoSilenzioso: cosa c'è da aggiornare. Il codice è quello d'uscita se non c'è niente da fare.
func pianoSilenzioso() (*installa.PianoAggiornamento, int) {
	if !pacco.Pieno {
		return nil, installa.UscitaErrore // il disinstallatore non ha i programmi dentro
	}
	e, err := installa.LeggiElenco(pacco.Sorgente())
	if err != nil {
		return nil, installa.UscitaErrore
	}
	base, ok := installa.Installato(reg)
	if !ok {
		return nil, installa.UscitaNienteDaFare
	}
	if installa.ControllaCartellaWin(base, cartellaWindows()) != nil {
		return nil, installa.UscitaErrore
	}
	base = filepath.Clean(base)
	st, err := installa.LeggiStato(base)
	if err != nil {
		return nil, installa.UscitaErrore
	}
	if st == nil {
		return nil, installa.UscitaNienteDaFare
	}
	piano, err := installa.PreparaAggiornamento(st, e, versioneTasso(base), menuStart(), desktopPubblico())
	if errors.Is(err, installa.ErrNienteDaAggiornare) {
		return nil, installa.UscitaNienteDaFare
	}
	if err != nil {
		return nil, installa.UscitaErrore
	}
	elenco = e
	if testi.Valida(st.Lingua) {
		lingua.Store(st.Lingua)
	}
	return piano, installa.UscitaFatto
}

// versioneTasso: il file VERSIONE accanto a Tasso.exe (lo riscrive anche Tasso quando si aggiorna da solo).
func versioneTasso(base string) string {
	p := filepath.Join(base, "VERSIONE")
	fi, err := os.Lstat(p)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > 64 {
		return ""
	}
	b, _ := os.ReadFile(p)
	return strings.TrimSpace(string(b))
}

// aggiornaSilenzioso fa tutto e restituisce il codice d'uscita.
func aggiornaSilenzioso(soloControlla, riapri bool) int {
	if riapri && !soloControlla {
		// Tasso si è chiuso per farsi aggiornare: qualunque cosa succeda qui (piano non valido,
		// errore, panico), alla fine si riapre. riapriTasso non fa niente se è già aperto.
		defer func() {
			if r := recover(); r != nil {
				installa.Diario("errore grave nell'aggiornamento silenzioso: %v", r)
			}
			if base, ok := installa.Installato(reg); ok {
				riapriTasso(filepath.Clean(base))
			}
		}()
	}
	piano, codice := pianoSilenzioso()
	if piano == nil || soloControlla {
		return codice
	}
	// una sola copia di Tasso Setup alla volta (anche con l'installatore normale aperto)
	mn, _ := windows.UTF16PtrFromString(nomeMutex)
	h, err := windows.CreateMutex(nil, true, mn)
	if err == windows.ERROR_ALREADY_EXISTS || h == 0 {
		if h != 0 {
			windows.CloseHandle(h)
		}
		if riapri {
			riapriTasso(piano.Opzioni.Base)
		}
		return installa.UscitaErrore
	}
	defer windows.CloseHandle(h)
	defer cancellaCopiaAggiornamento()
	return conBarra(func(avanza func(testo string, perc int, errore bool)) int {
		codice, motivo := eseguiAggiornamento(piano, riapri, avanza)
		installa.Diario("aggiornamento silenzioso finito: codice %d %s", codice, motivo)
		if codice == installa.UscitaFatto {
			avanza(testi.T(L(), "aggs_fatto"), 100, false)
			time.Sleep(800 * time.Millisecond)
		} else {
			testo := testi.T(L(), "aggs_errore")
			if motivo != "" {
				testo = motivo + " " + testo
			}
			avanza(testo, -1, true)
			time.Sleep(6 * time.Second) // il tempo di leggerlo, poi la finestrella si chiude da sola
		}
		return codice
	})
}

// eseguiAggiornamento restituisce il codice d'uscita e, se non è riuscito, il motivo in parole semplici.
func eseguiAggiornamento(piano *installa.PianoAggiornamento, riapri bool, avanza func(string, int, bool)) (int, string) {
	o := piano.Opzioni
	base := o.Base
	avanza(testi.T(L(), "aggs_chiudo"), 0, false)
	tassoAperto := riapri
	// Tasso che mi ha avviato si sta chiudendo da solo: fotografo adesso i suoi figli (WebView2,
	// llama-server...) per chiuderli dopo, se restano
	prima := fotoProcessi()
	var radici []uint32
	for _, a := range programmiAperti(base, o.Componenti) {
		radici = append(radici, a.pid)
	}
	installa.Diario("aggiornamento silenzioso di %v in %s (riapri=%v, aperti %d)", o.Componenti, base, riapri, len(radici))
	if riapri {
		// Tasso ci ha avviato e si sta chiudendo da solo: lo aspetto (al massimo un minuto)
		fine := time.Now().Add(time.Minute)
		for time.Now().Before(fine) && tassoGira(base) {
			time.Sleep(500 * time.Millisecond)
		}
	}
	if tassoGira(base) {
		tassoAperto = true
	}
	// Tasso Zip e Tasso PDF aperti: potrebbero avere lavoro non salvato (un archivio a metà, un PDF
	// modificato). Non li chiudo a forza: lo dico, e si riprova quando li hai chiusi.
	for _, a := range programmiAperti(base, o.Componenti) {
		if a.nome != "Tasso" {
			installa.Diario("%s è aperto: non aggiorno", a.nome)
			if tassoAperto {
				riapriTasso(base)
			}
			return installa.UscitaErrore, testi.T(L(), "aggs_aperti")
		}
	}
	if len(programmiAperti(base, o.Componenti)) > 0 && !chiudiProgrammi(base, o.Componenti) {
		// Tasso sta facendo una pulizia (rifiuta --chiudi) o un programma non si chiude: non tocco niente
		if tassoAperto {
			riapriTasso(base)
		}
		return installa.UscitaErrore, testi.T(L(), "aperti_errore")
	}
	chiudiAiutanti(base, o.Componenti, radici, prima)
	in := nuovoInstallatore()
	var ultimo time.Time
	in.Avanzamento = func(a installa.Avanzamento) {
		if a.Fase == "copia" && time.Since(ultimo) < 80*time.Millisecond {
			return
		}
		ultimo = time.Now()
		perc := 0
		if a.Totale > 0 {
			perc = int(a.Fatto * 100 / a.Totale)
		}
		testo := testi.T(L(), "fase_"+a.Fase)
		if a.Fase == "copia" {
			testo = strings.Replace(testo, "%s", a.File, 1)
		}
		if a.Fase == "fine" {
			perc, testo = 100, testi.T(L(), "aggs_fatto")
		}
		avanza(testo, perc, false)
	}
	_, err := in.Installa(context.Background(), o)
	if errors.Is(err, installa.ErrFileBloccato) {
		// un file è ancora occupato anche dopo 15 secondi: richiudo quello che è rimasto e riprovo
		// una volta (Installa ha rimesso tutto com'era, quindi si può rifare da capo)
		installa.Diario("file occupato: richiudo i processi rimasti e riprovo una volta")
		chiudiAiutanti(base, o.Componenti, nil, nil)
		time.Sleep(2 * time.Second)
		_, err = in.Installa(context.Background(), o)
	}
	aggiornaIcone()
	var avviso *installa.AvvisoCollegamenti
	codice, motivo := installa.UscitaFatto, ""
	if err != nil && !errors.As(err, &avviso) {
		codice = installa.UscitaErrore
		var eb *installa.ErroreBloccato
		if errors.As(err, &eb) {
			motivo = strings.Replace(testi.T(L(), "aggs_bloccato"), "%s", filepath.Base(eb.File), 1)
		}
	}
	if codice == installa.UscitaFatto {
		annullaCancellazioniPendenti(base, o.Componenti) // vedi installaInFondo
		preparaAperturaTasso(base, o.Componenti)         // vedi installaInFondo
	}
	if tassoAperto {
		riapriTasso(base)
	}
	return codice, motivo
}

// tassoGira: il Tasso.exe della cartella installata è aperto.
func tassoGira(base string) bool {
	for _, a := range programmiAperti(base, []installa.Componente{installa.Gaming}) {
		if a.nome == "Tasso" {
			return true
		}
	}
	return false
}

// riapriTasso apre Tasso (se non è già aperto). "--after-update": se la copia vecchia si sta
// ancora chiudendo, quella nuova la aspetta invece di portarla davanti.
func riapriTasso(base string) {
	exe := filepath.Join(base, "Tasso.exe")
	if fi, err := os.Lstat(exe); err != nil || !fi.Mode().IsRegular() || tassoGira(base) {
		return
	}
	cmd := exec.Command(exe, "--after-update")
	cmd.Dir = base
	_ = cmd.Start()
}

// cancellaCopiaAggiornamento: Tasso avvia una copia scaricata e controllata in Programmi
// ("tasso-verifica-<numero>.exe"); alla fine la tolgo (si cancella appena questo programma esce).
func cancellaCopiaAggiornamento() {
	me, err := os.Executable()
	if err != nil || !strings.HasPrefix(strings.ToLower(filepath.Base(me)), "tasso-verifica-") {
		return
	}
	daCancellareAllaFine = me
	cancellaAllUscita()
}

// conBarra mostra la finestrella con la barra mentre lavoro gira, e restituisce il suo codice.
// Senza WebView2 lavora lo stesso, senza finestra.
func conBarra(lavoro func(avanza func(testo string, perc int, errore bool)) int) int {
	if v, err := webviewloader.GetInstalledVersion(); err != nil || v == "" {
		return lavoro(func(string, int, bool) {})
	}
	dati := nuoviDatiWebView2()
	defer togliDatiWebView2(dati)
	scala := scalaSchermo()
	var finito atomic.Bool
	webview2.ChiusuraHook = func() bool { return !finito.Load() } // niente X a metà aggiornamento
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath: dati,
		WindowOptions: webview2.WindowOptions{
			Title:  testi.T(L(), "aggs_titolo"),
			Width:  uint(460 * scala),
			Height: uint(175 * scala),
			IconId: 2,
			Center: true,
		},
	})
	if w == nil {
		return lavoro(func(string, int, bool) {})
	}
	w.SetSize(int(460*scala), int(175*scala), webview2.HintFixed)
	w.SetHtml(ui.PaginaAggiorna(testi.T(L(), "aggs_titolo"), testi.T(L(), "aggs_chiudo")))
	var codice atomic.Int32
	go func() {
		defer func() {
			if r := recover(); r != nil { // un errore grave non deve lasciare la finestrella aperta per sempre
				installa.Diario("errore grave: %v", r)
				codice.Store(int32(installa.UscitaErrore))
				finito.Store(true)
				w.Dispatch(func() { w.Terminate() })
			}
		}()
		c := lavoro(func(testo string, perc int, errore bool) {
			b, _ := json.Marshal(map[string]any{"testo": testo, "perc": perc, "errore": errore})
			w.Dispatch(func() { w.Eval("window.TS && TS.avanza(" + string(b) + ")") })
		})
		codice.Store(int32(c))
		finito.Store(true)
		w.Dispatch(func() { w.Terminate() })
	}()
	w.Run()
	w.Destroy()
	return int(codice.Load())
}
