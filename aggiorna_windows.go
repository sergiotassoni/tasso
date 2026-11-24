package main

// Aggiornamento silenzioso dell'Ecosistema Tasso, avviato da Tasso (Impostazioni → Aggiornamenti):
//
//	TassoSetup.exe --aggiorna-silenzioso [--riapri-tasso] [--solo-controlla]
//
// Aggiorna SOLO i programmi già installati (stessa cartella, stesse caselle, stessa lingua, il
// Desktop solo se c'erano già i collegamenti) e solo quelli che in questo installatore sono più
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
	piano, codice := pianoSilenzioso()
	if piano == nil || soloControlla {
		return codice
	}
	// una sola copia di Tasso Setup alla volta (anche con l'installatore normale aperto)
	mn, _ := windows.UTF16PtrFromString("Local\\TassoSetup")
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
		codice := eseguiAggiornamento(piano, riapri, avanza)
		if codice == installa.UscitaFatto {
			avanza(testi.T(L(), "aggs_fatto"), 100, false)
			time.Sleep(800 * time.Millisecond)
		} else {
			avanza(testi.T(L(), "aggs_errore"), -1, true)
			time.Sleep(4 * time.Second) // il tempo di leggerlo, poi la finestrella si chiude da sola
		}
		return codice
	})
}

func eseguiAggiornamento(piano *installa.PianoAggiornamento, riapri bool, avanza func(string, int, bool)) int {
	o := piano.Opzioni
	base := o.Base
	avanza(testi.T(L(), "aggs_chiudo"), 0, false)
	tassoAperto := riapri
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
	if len(programmiAperti(base, o.Componenti)) > 0 && !chiudiProgrammi(base, o.Componenti) {
		// Tasso sta facendo una pulizia (rifiuta --chiudi) o un programma non si chiude: non tocco niente
		if tassoAperto {
			riapriTasso(base)
		}
		return installa.UscitaErrore
	}
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
	aggiornaIcone()
	var avviso *installa.AvvisoCollegamenti
	codice := installa.UscitaFatto
	if err != nil && !errors.As(err, &avviso) {
		codice = installa.UscitaErrore
	}
	if tassoAperto {
		riapriTasso(base)
	}
	return codice
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
	dati, _ := os.MkdirTemp("", "TassoSetup-WebView2-")
	defer func() {
		for i := 0; i < 10; i++ {
			if os.RemoveAll(dati) == nil {
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
	}()
	scala := 1.0
	if procGetDpiForSystem.Find() == nil {
		if dpi, _, _ := procGetDpiForSystem.Call(); dpi >= 96 {
			scala = float64(dpi) / 96
		}
	}
	var finito atomic.Bool
	webview2.ChiusuraHook = func() bool { return !finito.Load() } // niente X a metà aggiornamento
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath: dati,
		WindowOptions: webview2.WindowOptions{
			Title:  testi.T(L(), "aggs_titolo"),
			Width:  uint(460 * scala),
			Height: uint(150 * scala),
			IconId: 2,
			Center: true,
		},
	})
	if w == nil {
		return lavoro(func(string, int, bool) {})
	}
	w.SetSize(int(460*scala), int(150*scala), webview2.HintFixed)
	w.SetHtml(ui.PaginaAggiorna(testi.T(L(), "aggs_titolo"), testi.T(L(), "aggs_chiudo")))
	var codice atomic.Int32
	go func() {
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
