// Tasso Setup: installatore unico dell'Ecosistema Tasso (Tasso Gaming, Tasso Ufficio,
// Tasso Zip, Tasso PDF). Lo stesso codice, compilato con -tags disinstallatore,
// diventa "Disinstalla Tasso.exe".
package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/webviewloader"
	"golang.org/x/sys/windows"

	"tassosetup/installa"
	"tassosetup/pacco"
	"tassosetup/testi"
	"tassosetup/ui"
)

// Pagina Microsoft del WebView2 Runtime: si apre SOLO se l'utente risponde Sì.
const paginaWebView2 = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"

// nomeMutex: una sola copia di Tasso Setup alla volta (anche con l'aggiornamento silenzioso).
const nomeMutex = "Local\\TassoSetup"

var (
	finestra webview2.WebView
	lingua   atomic.Value // string
	inCorso  atomic.Bool  // installazione o disinstallazione in corso
	// tassoPulito: Tasso ha già tolto le sue cose (goPreparaDisinstalla)
	tassoPulito atomic.Bool
	annullaMu   sync.Mutex
	annullaFn   context.CancelFunc
	elenco      *installa.Elenco
	errElenco   error
	reg         = registroWin{}
	modoDisins  bool
	baseDisins  string
	// --scegli=gaming,ufficio: nel disinstallatore sono già spuntati solo questi (lo usa
	// "Togli Tasso dal PC" dentro Tasso). Si possono sempre cambiare.
	preScelti []installa.Componente
)

func L() string {
	if l, ok := lingua.Load().(string); ok && l != "" {
		return l
	}
	return "en"
}

func main() {
	protezioneAvvio()
	installa.ImpostaDiario(filepath.Join(os.TempDir(), installa.NomeDiario))
	installa.Diario("---- Tasso Setup avviato (%s), argomenti %q", versioneSetup(), os.Args[1:])
	modoDisins = !pacco.Pieno
	lingua.Store(testi.DaWindows(linguaWindows()))
	// Aggiornamento silenzioso chiesto da Tasso (aggiorna_windows.go): niente pagine, esce con 0/1/2.
	silenzioso, solo, riapri := false, false, false
	for _, a := range os.Args[1:] {
		switch a {
		case argAggiornaSilenzioso:
			silenzioso = true
		case argSoloControlla:
			solo = true
		case argRiapriTasso:
			riapri = true
		}
	}
	if silenzioso {
		os.Exit(aggiornaSilenzioso(solo, riapri))
	}
	for _, a := range os.Args[1:] {
		if a == "--disinstalla" {
			modoDisins = true
		}
		if v, ok := strings.CutPrefix(a, "--scegli="); ok {
			if cc, err := installa.Normalizza(componenti(strings.Split(v, ","))); err == nil {
				preScelti = cc
			}
		}
	}

	// una sola copia alla volta
	mn, _ := windows.UTF16PtrFromString(nomeMutex)
	h, err := windows.CreateMutex(nil, true, mn)
	if err == windows.ERROR_ALREADY_EXISTS {
		portaDavanti()
		return
	}
	if h != 0 {
		defer windows.CloseHandle(h)
	}

	if modoDisins {
		me, _ := os.Executable()
		baseDisins = filepath.Dir(me)
		if st, _ := installa.LeggiStato(baseDisins); st == nil {
			if b, ok := installa.Installato(reg); ok {
				baseDisins = b
			}
		}
		if st, _ := installa.LeggiStato(baseDisins); st != nil && testi.Valida(st.Lingua) {
			lingua.Store(st.Lingua)
		}
	} else {
		elenco, errElenco = installa.LeggiElenco(pacco.Sorgente())
		if b, ok := installa.Installato(reg); ok {
			if st, _ := installa.LeggiStato(b); st != nil && testi.Valida(st.Lingua) {
				lingua.Store(st.Lingua)
			}
		}
	}

	// WebView2 Runtime: senza non si può mostrare la pagina. Spiego e offro il collegamento
	// di Microsoft solo se l'utente lo chiede (risponde Sì). Nessuna connessione da parte nostra.
	if v, err := webviewloader.GetInstalledVersion(); err != nil || v == "" {
		tit, _ := windows.UTF16PtrFromString(testi.T(L(), "wv2_titolo"))
		txt, _ := windows.UTF16PtrFromString(testi.T(L(), "wv2_testo"))
		const mbYesNo, mbIconWarning, idYes = 0x4, 0x30, 6
		if r, _ := windows.MessageBox(0, txt, tit, mbYesNo|mbIconWarning); r == idYes {
			verbo, _ := windows.UTF16PtrFromString("open")
			url, _ := windows.UTF16PtrFromString(paginaWebView2)
			_ = windows.ShellExecute(0, verbo, url, nil, nil, 1)
		}
		return
	}

	dati, _ := os.MkdirTemp("", "TassoSetup-WebView2-")
	defer func() {
		togliDatiWebView2(dati)
		cancellaAllUscita()
	}()

	scala := scalaSchermo()
	webview2.SuFinestra["goScegliCartella"] = true // finestra di Windows legata alla nostra
	webview2.Subito["goAnnulla"] = true
	webview2.ChiusuraHook = func() bool {
		if inCorso.Load() {
			manda("chiusura", nil)
			return true // non si chiude a metà
		}
		return false
	}
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  dati,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  testi.T(L(), "titolo"),
			Width:  uint(900 * scala),
			Height: uint(640 * scala),
			IconId: 2,
			Center: true,
		},
	})
	if w == nil {
		return
	}
	finestra = w
	w.SetSize(int(760*scala), int(560*scala), webview2.HintMin)
	collega(w)
	w.SetHtml(ui.Pagina())
	w.Run()
	w.Destroy()
}

// togliDatiWebView2 cancella la cartella temporanea della WebView2. I processi di WebView2
// escono un attimo dopo la finestra: riprova per circa 3 secondi.
func togliDatiWebView2(dati string) {
	for i := 0; i < 10; i++ {
		if os.RemoveAll(dati) == nil {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// scalaSchermo: 1 a 96 DPI, 1.5 a 144 DPI... (per le dimensioni delle finestre).
func scalaSchermo() float64 {
	scala := 1.0
	if procGetDpiForSystem.Find() == nil {
		if dpi, _, _ := procGetDpiForSystem.Call(); dpi >= 96 {
			scala = float64(dpi) / 96
		}
	}
	return scala
}

func portaDavanti() {
	tit, _ := windows.UTF16PtrFromString(testi.T(L(), "titolo"))
	if hw, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(tit))); hw != 0 {
		procSetForegroundWindow.Call(hw)
	}
}

// manda un evento alla pagina: window.TS.<nome>(dati).
func manda(nome string, dati any) {
	w := finestra
	if w == nil {
		return
	}
	b, err := json.Marshal(dati)
	if err != nil {
		return
	}
	js := "window.TS && TS." + nome + " && TS." + nome + "(" + string(b) + ")"
	w.Dispatch(func() { w.Eval(js) })
}

func componenti(ss []string) []installa.Componente {
	out := make([]installa.Componente, 0, len(ss))
	for _, s := range ss {
		out = append(out, installa.Componente(s))
	}
	return out
}

func nuovoInstallatore() *installa.Installatore {
	return conUtenteSessione(&installa.Installatore{Sorgente: pacco.Sorgente(), Elenco: elenco, Registro: reg,
		CreaCollegamento: creaCollegamento, TogliFile: togliFile, CancellaAlRiavvio: cancellaAlRiavvio,
		PreparaCartella: preparaCartellaTasso})
}

// versioneSetup: la versione dell'ecosistema dentro questo installatore (per il diario).
func versioneSetup() string {
	if !pacco.Pieno {
		return "disinstallatore"
	}
	if e, err := installa.LeggiElenco(pacco.Sorgente()); err == nil {
		return e.Versione
	}
	return "?"
}

func collega(w webview2.WebView) {
	_ = w.Bind("goAvvio", func() map[string]any {
		var lingue []map[string]string
		for _, l := range testi.Lingue {
			lingue = append(lingue, map[string]string{"codice": l.Codice, "nome": l.Nome})
		}
		cartella := installa.CartellaPredefinita(programFiles())
		var st *installa.Stato
		if modoDisins {
			cartella = baseDisins
			st, _ = installa.LeggiStato(baseDisins)
		} else if b, ok := installa.Installato(reg); ok {
			if s, _ := installa.LeggiStato(b); s != nil {
				st, cartella = s, b
			}
		}
		var comp []map[string]any
		for _, c := range installa.TuttiComponenti {
			p := installa.ProgrammaDi(c)
			if !modoDisins && elenco != nil && elenco.Prog(p) == nil {
				continue // non incorporato in questo installatore
			}
			inst := ""
			if st != nil {
				for _, x := range st.Componenti {
					if x == c {
						inst = st.Programmi[p].Versione
						if inst == "" {
							inst = "?"
						}
					}
				}
			}
			// già installato in una versione più nuova di quella di questo installatore: non si
			// spunta e non si sostituisce (mai tornare indietro, come l'aggiornamento silenzioso)
			piuNuovo := ""
			if st != nil && !modoDisins && elenco != nil && inst != "" {
				if ep := elenco.Prog(p); ep != nil && installa.PiuNuova(versioneInstallata(st, p), ep.Versione) {
					piuNuovo = versioneInstallata(st, p)
				}
			}
			comp = append(comp, map[string]any{"id": c, "installato": inst, "piuNuovo": piuNuovo})
		}
		var installata any
		if st != nil {
			// aggiornamento: la casella del Desktop parte spuntata se i collegamenti lì ci sono già
			// vecchio: questo installatore è più vecchio dell'ecosistema installato (avviso)
			vecchio := !modoDisins && elenco != nil && installa.PiuNuova(st.Versione, elenco.Versione)
			installata = map[string]any{"versione": st.Versione, "componenti": st.Componenti,
				"desktop": installa.DesktopUsato(st, desktopPubblico()), "vecchio": vecchio}
		}
		ver := ""
		if elenco != nil {
			ver = elenco.Versione
		} else if st != nil {
			ver = st.Versione
		}
		modo := "installa"
		if modoDisins {
			modo = "disinstalla"
		}
		pre := []installa.Componente{}
		if modoDisins {
			pre = append(pre, preScelti...)
		}
		return map[string]any{"modo": modo, "lingua": L(), "lingue": lingue, "testi": testi.Dizionario,
			"componenti": comp, "cartella": cartella, "installata": installata, "versione": ver, "preScelti": pre,
			"vuoto": !modoDisins && (elenco == nil || errElenco != nil)}
	})
	_ = w.Bind("goLingua", func(l string) bool {
		if testi.Valida(l) {
			lingua.Store(l)
			w.SetTitle(testi.T(l, "titolo"))
			return true
		}
		return false
	})
	_ = w.Bind("goControllaCartella", func(p string, cc []string) map[string]any {
		p = strings.TrimSpace(p)
		out := map[string]any{"errore": ""}
		if err := installa.ControllaCartellaWin(p, cartellaWindows()); err != nil {
			var ec *installa.ErroreCartella
			if errors.As(err, &ec) {
				out["errore"] = ec.Codice
			} else {
				out["errore"] = installa.ErrCartCaratteri
			}
			return out
		}
		// Tasso va sempre in una cartella sua (...\Tasso): vedi installa.CartellaDedicata
		p = filepath.Clean(installa.CartellaDedicata(p, infoCartella))
		out["cartella"] = p
		scelti, _ := installa.Normalizza(componenti(cc))
		progs := installa.ProgrammiPer(scelti)
		var serve int64
		if elenco != nil {
			serve = elenco.Dimensione(append(progs, installa.ProgSetup))
		}
		out["percorsi"] = map[string]string{
			"tasso": installa.CartellaDi(p, installa.ProgTasso),
			"zip":   installa.CartellaDi(p, installa.ProgZip),
			"pdf":   installa.CartellaDi(p, installa.ProgPDF),
		}
		out["serve"] = serve
		out["libero"] = spazioLibero(p)
		out["fuoriProgrammi"] = !installa.DentroProgrammi(p, programFiles())
		if out["fuoriProgrammi"] == true {
			// fuori da Programmi: si usa solo una cartella che nessun utente normale può cambiare
			s := sicurezzaCartella(p)
			if _, err := installa.DecidiCartellaSicura(s); err != nil && !installa.InfoCartellaDisco(p).DiTasso {
				out["errore"] = installa.ErrCartNonSicura
				var ec *installa.ErroreCartella
				if errors.As(err, &ec) {
					out["errore"] = ec.Codice
				}
			}
			out["senzaPermessi"] = s.SenzaPermessi // avviso: chiunque può cambiare i file
		}
		return out
	})
	_ = w.Bind("goScegliCartella", func(attuale, titolo string) string {
		return scegliCartella(uintptr(w.Window()), attuale, titolo)
	})
	_ = w.Bind("goProgrammiAperti", func(base string, cc []string) []string {
		var nomi []string
		visti := map[string]bool{}
		for _, a := range programmiAperti(filepath.Clean(base), componenti(cc)) {
			if !visti[a.nome] {
				visti[a.nome] = true
				nomi = append(nomi, a.nome)
			}
		}
		return nomi
	})
	// goPreparaDisinstalla: prima di chiudere gli altri programmi, se va via Tasso chiudo SOLO Tasso
	// e gli chiedo di togliere le sue cose (Tasso.exe --togli-silenzioso). Se rifiuta (un gioco
	// tradotto è aperto, la modalità riunione è accesa...) lo riapro e mi fermo: Zip e PDF restano
	// aperti, niente è stato tolto. "ok", "chiuso" (Tasso non si chiude) o il motivo del rifiuto.
	_ = w.Bind("goPreparaDisinstalla", func(cc []string) map[string]any {
		st, _ := installa.LeggiStato(baseDisins)
		if !modoDisins || len(preScelti) > 0 || !installa.TogliTasso(st, componenti(cc)) {
			return map[string]any{"ok": true}
		}
		eraAperto := tassoGira(baseDisins)
		if eraAperto && !chiudiProgrammi(baseDisins, []installa.Componente{installa.Gaming, installa.Ufficio}) {
			return map[string]any{"ok": false, "chiuso": false}
		}
		if err := pulisciTasso(baseDisins); err != nil {
			installa.Diario("Tasso non ha tolto le sue cose: %v", err)
			if eraAperto {
				riapriTasso(baseDisins)
			}
			return map[string]any{"ok": false, "rifiuta": true, "dettagli": err.Error()}
		}
		tassoPulito.Store(true)
		return map[string]any{"ok": true}
	})
	_ = w.Bind("goChiudiProgrammi", func(base string, cc []string) bool {
		return chiudiProgrammi(filepath.Clean(base), componenti(cc))
	})
	_ = w.Bind("goInstalla", func(o struct {
		Cartella   string   `json:"cartella"`
		Componenti []string `json:"componenti"`
		Desktop    bool     `json:"desktop"`
		// la casella era spuntata perché c'erano già i collegamenti sul Desktop e l'hai tolta tu
		DesktopTogli bool   `json:"desktopTogli"`
		Lingua       string `json:"lingua"`
	}) any {
		if modoDisins || elenco == nil {
			return map[string]any{"codice": "err_vuoto"}
		}
		if !inCorso.CompareAndSwap(false, true) {
			return "ok"
		}
		if err := installa.ControllaCartellaWin(o.Cartella, cartellaWindows()); err != nil {
			inCorso.Store(false)
			var ec *installa.ErroreCartella
			errors.As(err, &ec)
			return map[string]any{"codice": ec.Codice}
		}
		l := testi.Normalizza(o.Lingua)
		if l == "" {
			l = L()
		}
		// sempre una cartella sua: mai C:\Program Files o D:\Giochi come "cartella di Tasso"
		base := filepath.Clean(installa.CartellaDedicata(o.Cartella, infoCartella))
		scelti := componenti(o.Componenti)
		tieni := false
		if st, _ := installa.LeggiStato(base); st != nil {
			// mai tornare indietro: i programmi già più nuovi restano come sono
			scelti = installa.SenzaPiuNuovi(st, elenco, scelti, versioneTasso(base))
			tieni = installa.PiuNuova(st.Versione, elenco.Versione)
			if len(scelti) == 0 {
				inCorso.Store(false)
				return map[string]any{"codice": "agg_vecchio", "dettagli": elenco.Versione}
			}
		}
		opz := installa.Opzioni{Base: base, Componenti: scelti, Desktop: o.Desktop,
			DesktopTogli: o.DesktopTogli && !o.Desktop, Lingua: l, CartellaMenu: menuStart(), CartellaDesktop: desktopPubblico(),
			TieniDisinstallatore: tieni}
		ctx, annulla := context.WithCancel(context.Background())
		annullaMu.Lock()
		annullaFn = annulla
		annullaMu.Unlock()
		go installaInFondo(ctx, opz)
		return "ok"
	})
	_ = w.Bind("goAnnulla", func() bool {
		annullaMu.Lock()
		defer annullaMu.Unlock()
		if annullaFn != nil {
			annullaFn()
		}
		return true
	})
	_ = w.Bind("goApri", func(ids []string) bool {
		base, ok := installa.Installato(reg)
		if !ok {
			return false
		}
		for _, id := range ids {
			switch installa.Programma(id) {
			case installa.ProgTasso:
				apri(filepath.Join(base, "Tasso.exe"), false)
			case installa.ProgZip:
				apri(filepath.Join(installa.CartellaDi(base, installa.ProgZip), "TassoZip.exe"), true)
			case installa.ProgPDF:
				apri(filepath.Join(installa.CartellaDi(base, installa.ProgPDF), "TassoPDF.exe"), true)
			}
		}
		return true
	})
	_ = w.Bind("goChiudi", func() bool {
		if inCorso.Load() {
			return false
		}
		w.Dispatch(func() { w.Terminate() })
		return true
	})
	_ = w.Bind("goDisinstalla", func(cc []string, togliDati bool) map[string]any {
		if !inCorso.CompareAndSwap(false, true) {
			return map[string]any{"ok": false, "dettagli": "…"}
		}
		defer inCorso.Store(false)
		// CreaCollegamento serve anche qui: se resta Tasso con un'altra modalità (es. tolto
		// Gaming, resta Ufficio) il collegamento "Tasso Gaming" diventa "Tasso Ufficio".
		in := &installa.Installatore{Registro: reg, TogliFile: togliFile, CreaCollegamento: creaCollegamento,
			CartellaDatiUtente: datiLocaliUtente, CancellaAlRiavvio: cancellaAlRiavvio, TogliAttivita: togliAttivitaTasso}
		conUtenteSessione(in)
		st, _ := installa.LeggiStato(baseDisins)
		viaTasso := installa.TogliTasso(st, componenti(cc))
		installa.Diario("disinstallo %v da %s (dati: %v, via Tasso: %v)", cc, baseDisins, togliDati, viaTasso)
		// Da "Togli Tasso dal PC" (--scegli=) Tasso ha già tolto le sue cose e i suoi dati: non lo
		// riapro. Da App e funzionalità invece Tasso toglie le sue cose prima di sparire: PRIMA di
		// chiudere qualsiasi altro programma, così se rifiuta (un gioco tradotto aperto...) non ho
		// chiuso niente per niente.
		if viaTasso && len(preScelti) == 0 && !tassoPulito.Load() {
			// (di solito l'ha già fatto goPreparaDisinstalla, prima di chiudere Zip e PDF)
			if err := pulisciTasso(baseDisins); err != nil {
				installa.Diario("Tasso non ha tolto le sue cose: %v", err)
				return map[string]any{"ok": false, "rifiuta": true, "dettagli": err.Error()}
			}
		}
		chiudiAiutanti(baseDisins, componenti(cc), nil, nil)
		r, err := in.DisinstallaCon(baseDisins, componenti(cc), installa.OpzioniDisinstalla{
			TogliDati: togliDati && viaTasso && len(preScelti) == 0, TassoGiaPulito: true})
		if err != nil {
			installa.Diario("disinstallazione NON riuscita: %v", err)
		}
		aggiornaIcone()
		var rif *installa.ErroreTassoRifiuta
		if errors.As(err, &rif) {
			return map[string]any{"ok": false, "rifiuta": true, "dettagli": rif.Error()}
		}
		if err != nil {
			return map[string]any{"ok": false, "dettagli": err.Error()}
		}
		if viaTasso {
			togliCollegamentoVecchio(baseDisins) // il Tasso.lnk della 3.0 sistemato all'installazione
		}
		return map[string]any{"ok": true, "tuttoTolto": r.TuttoTolto, "nonTolti": r.NonTolti, "alRiavvio": r.AlRiavvio, "datiTenuti": r.DatiTenuti}
	})
}

// infoCartella: installa.InfoCartellaDisco, ma i "resti di un Tasso disinstallato" contano solo
// dentro Programmi (lì solo gli amministratori scrivono). Fuori, una cartella non vuota resta da
// controllare come prima (si aggiunge \Tasso e valgono i controlli di sicurezza).
func infoCartella(p string) installa.InfoCartella {
	i := installa.InfoCartellaDisco(p)
	if i.SoloResti && !installa.DentroProgrammi(p, programFiles()) {
		i.SoloResti = false
	}
	return i
}

// versioneInstallata: la versione installata di un programma (per Tasso vale anche il file
// VERSIONE, che Tasso riscrive quando si aggiorna da solo).
func versioneInstallata(st *installa.Stato, p installa.Programma) string {
	v := st.Programmi[p].Versione
	if p == installa.ProgTasso {
		v = installa.Maggiore(v, versioneTasso(st.Cartella))
	}
	return v
}

func installaInFondo(ctx context.Context, o installa.Opzioni) {
	defer inCorso.Store(false)
	in := nuovoInstallatore()
	var ultimo time.Time
	var ultimaFase string
	in.Avanzamento = func(a installa.Avanzamento) {
		// al massimo 20 aggiornamenti al secondo (più i cambi di fase)
		if a.Fase == ultimaFase && time.Since(ultimo) < 50*time.Millisecond {
			return
		}
		ultimo, ultimaFase = time.Now(), a.Fase
		manda("avanza", a)
	}
	// processi rimasti da prima (es. un llama-server di un Tasso chiuso male) che tengono occupati i file
	chiudiAiutanti(o.Base, o.Componenti, nil, nil)
	_, err := in.Installa(ctx, o)
	aggiornaIcone()
	var avviso *installa.AvvisoCollegamenti
	if err == nil || errors.As(err, &avviso) {
		// reinstallato prima di riavviare dopo una disinstallazione: Windows non deve cancellare i file nuovi
		annullaCancellazioniPendenti(o.Base, o.Componenti)
		preparaAperturaTasso(o.Base, o.Componenti) // Tasso senza scudo: l'attività \Tasso\Apri Tasso
		// collegamenti di Tasso anche sul Desktop: il Tasso.lnk della 3.0 portatile non deve più
		// aprire la 3.0 scaricata (solo se è proprio quello, vedi CollegamentoVecchioDaAggiornare)
		if o.Desktop {
			for _, p := range installa.ProgrammiPer(o.Componenti) {
				if p == installa.ProgTasso {
					aggiornaCollegamentoVecchio(o.Base)
				}
			}
		}
	}
	if err != nil && !errors.As(err, &avviso) {
		r := map[string]any{"ok": false, "codice": "err_generico", "dettagli": err.Error(), "diario": installa.PercorsoDiario()}
		var ec *installa.ErroreCartella
		var eb *installa.ErroreBloccato
		switch {
		case errors.Is(err, installa.ErrAnnullato):
			r["codice"] = "err_annullato"
		case errors.Is(err, installa.ErrVerifica):
			r["codice"] = "err_verifica"
		case errors.Is(err, installa.ErrRiparaNonRiuscito) || errors.As(err, new(*installa.ErroreNonRimesso)):
			r["codice"] = "err_non_rimesso"
		case errors.As(err, &eb):
			r["codice"], r["dettagli"] = "err_bloccato", filepath.Base(eb.File)
		case errors.As(err, &ec):
			r["codice"] = ec.Codice
		}
		manda("finito", r)
		return
	}
	var apribili []installa.Programma
	apribili = append(apribili, installa.ProgrammiPer(o.Componenti)...)
	manda("finito", map[string]any{"ok": true, "avvisoCollegamenti": avviso != nil, "apribili": apribili})
}

// protezioneAvvio: l'installatore gira come amministratore (come Tasso, vedi codice/main.go
// hardenProcess): le DLL si caricano solo da System32, mai dalla cartella dell'exe (es. Download,
// dove qualcuno potrebbe aver messo una DLL falsa), e le variabili WEBVIEW2_* dell'utente
// (cambiabili senza permessi) si tolgono prima di aprire la finestra. Va chiamata per prima.
func protezioneAvvio() {
	if err := windows.SetDefaultDllDirectories(windows.LOAD_LIBRARY_SEARCH_SYSTEM32); err != nil {
		installa.Diario("SetDefaultDllDirectories: %v", err)
	}
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i > 0 && strings.HasPrefix(strings.ToUpper(kv[:i]), "WEBVIEW2_") {
			_ = os.Unsetenv(kv[:i])
		}
	}
}
