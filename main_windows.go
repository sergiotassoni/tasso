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

var (
	finestra   webview2.WebView
	lingua     atomic.Value // string
	inCorso    atomic.Bool  // installazione o disinstallazione in corso
	annullaMu  sync.Mutex
	annullaFn  context.CancelFunc
	elenco     *installa.Elenco
	errElenco  error
	reg        = registroWin{}
	modoDisins bool
	baseDisins string
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
	modoDisins = !pacco.Pieno
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
	lingua.Store(testi.DaWindows(linguaWindows()))

	// una sola copia alla volta
	nome := "Local\\TassoSetup"
	mn, _ := windows.UTF16PtrFromString(nome)
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
		for i := 0; i < 10; i++ { // i processi di WebView2 escono un attimo dopo la finestra
			if os.RemoveAll(dati) == nil {
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
		cancellaAllUscita()
	}()

	scala := 1.0
	if procGetDpiForSystem.Find() == nil {
		if dpi, _, _ := procGetDpiForSystem.Call(); dpi >= 96 {
			scala = float64(dpi) / 96
		}
	}
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
	return &installa.Installatore{Sorgente: pacco.Sorgente(), Elenco: elenco, Registro: reg,
		CreaCollegamento: creaCollegamento, TogliFile: togliFile}
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
			comp = append(comp, map[string]any{"id": c, "installato": inst})
		}
		var installata any
		if st != nil {
			installata = map[string]any{"versione": st.Versione, "componenti": st.Componenti}
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
		p = filepath.Clean(p)
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
	_ = w.Bind("goChiudiProgrammi", func(base string, cc []string) bool {
		return chiudiProgrammi(filepath.Clean(base), componenti(cc))
	})
	_ = w.Bind("goInstalla", func(o struct {
		Cartella   string   `json:"cartella"`
		Componenti []string `json:"componenti"`
		Desktop    bool     `json:"desktop"`
		Lingua     string   `json:"lingua"`
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
		opz := installa.Opzioni{Base: filepath.Clean(o.Cartella), Componenti: componenti(o.Componenti), Desktop: o.Desktop,
			Lingua: l, CartellaMenu: menuStart(), CartellaDesktop: desktopPubblico()}
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
	_ = w.Bind("goDisinstalla", func(cc []string) map[string]any {
		if !inCorso.CompareAndSwap(false, true) {
			return map[string]any{"ok": false, "dettagli": "…"}
		}
		defer inCorso.Store(false)
		in := &installa.Installatore{Registro: reg, TogliFile: togliFile}
		r, err := in.Disinstalla(baseDisins, componenti(cc))
		aggiornaIcone()
		if err != nil {
			return map[string]any{"ok": false, "dettagli": err.Error()}
		}
		return map[string]any{"ok": true, "tuttoTolto": r.TuttoTolto, "nonTolti": r.NonTolti}
	})
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
	_, err := in.Installa(ctx, o)
	aggiornaIcone()
	var avviso *installa.AvvisoCollegamenti
	if err != nil && !errors.As(err, &avviso) {
		r := map[string]any{"ok": false, "codice": "err_generico", "dettagli": err.Error()}
		var ec *installa.ErroreCartella
		switch {
		case errors.Is(err, installa.ErrAnnullato):
			r["codice"] = "err_annullato"
		case errors.Is(err, installa.ErrVerifica):
			r["codice"] = "err_verifica"
		case errors.As(err, &ec):
			r["codice"] = ec.Codice
		}
		manda("finito", r)
		return
	}
	var apribili []installa.Programma
	for _, p := range installa.ProgrammiPer(o.Componenti) {
		apribili = append(apribili, p)
	}
	manda("finito", map[string]any{"ok": true, "avvisoCollegamenti": avviso != nil, "apribili": apribili})
}
