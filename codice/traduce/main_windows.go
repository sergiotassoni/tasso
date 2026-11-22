//go:build windows

// Tasso Traduce - traduce in italiano i dialoghi inglesi dei giochi, sul PC, senza internet.
// Strumento a sé della famiglia Tasso, di Sergio Tassoni. Licenza GPL-3.0.
package traduce

import (
	"runtime"
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

const versione = "0.16"

//go:embed ui
var fileUI embed.FS

var (
	wv webview2.WebView
)

// ---------------------------------------------------------------- impostazioni

type Impostazioni struct {
	Zona      Zona   `json:"zona"`
	Auto      bool   `json:"auto"`
	MostraEN  bool   `json:"mostra_en"`
	DimFont   int    `json:"dim_font"`
	Posizione string `json:"posizione"`
	TuttoAuto bool   `json:"tutto_auto"` // traduce da solo tutte le scritte dei giochi
	Motore    string `json:"motore"`     // "auto", "ia", "mozilla"
	MozillaSi bool   `json:"mozilla_si"`
	IASi      bool   `json:"ia_si"`
	Glossario Glossario
	// Profili: le scelte fatte per ogni gioco (nome dell'exe, es. "thecursed.exe").
	Profili map[string]Profilo `json:"profili"`
	// Versione delle impostazioni: dalla 2 niente parte da sola (richiesta di Sergio 7/11/2026)
	Versione int `json:"versione"`
	// Tasti: scelti dall'utente (nome funzione -> codice, vedi tasti.go)
	Tasti map[string]uint32 `json:"tasti"`
	// GiochiTradotti: giochi con la traduzione "applicata" dalla Traduzione gioco a passi:
	// partono da soli quando li apri (nome del gioco come lo riconosco da Steam, Epic…)
	GiochiTradotti map[string]bool `json:"giochi_tradotti"`
	// CartelleTradotte (3.1): le cartelle dei giochi tradotti, per riconoscerli anche quando
	// non sono nelle cartelle dei negozi che Traduce conosce (Xbox, Riot, aggiunti a mano…).
	CartelleTradotte []string `json:"cartelle_tradotte,omitempty"`
	// lingua del gioco ("auto" = la riconosco dalle scritte) e lingua in cui tradurre
	LinguaDa string `json:"lingua_da"`
	LinguaA  string `json:"lingua_a"`
	// 3.1 chat degli altri: zona della chat del gioco e interruttore
	ZonaChat  Zona `json:"zona_chat"`
	ChatAltri bool `json:"chat_altri,omitempty"`
}

// Profilo di un gioco: si carica da solo quando quel gioco è in primo piano.
type Profilo struct {
	Zona      Zona   `json:"zona"`
	Posizione string `json:"posizione"`
	DimFont   int    `json:"dim_font"`
}

var (
	muImp sync.Mutex
	imp   = Impostazioni{DimFont: 24, Motore: "auto", Versione: 2}
)

func fileImp() string { return filepath.Join(dirApp, "impostazioni.json") }

func caricaImp() {
	imp.Glossario = glossarioBase()
	if b, err := os.ReadFile(fileImp()); err == nil {
		_ = json.Unmarshal(b, &imp)
	}
	if imp.Glossario.Termini == nil {
		imp.Glossario = glossarioBase()
	}
	if imp.Versione < 2 { // le versioni vecchie partivano "in quinta": spengo le partenze da sole
		imp.Auto, imp.TuttoAuto, imp.Versione = false, false, 2
		imp.Zona = Zona{} // niente zona vecchia: di base si traduce tutto lo schermo del gioco
	}
}

func salvaImp() {
	muImp.Lock()
	b, _ := json.MarshalIndent(imp, "", "  ")
	muImp.Unlock()
	tmp := fileImp() + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, fileImp())
	}
}

func leggiImp() Impostazioni {
	muImp.Lock()
	defer muImp.Unlock()
	return imp
}

// ---------------------------------------------------------------- avvio dentro Tasso

// Tasso Traduce è una parte di Tasso: Tasso lo prepara con Prepara (prima di caricare la sua
// pagina, così i collegamenti ttXxx esistono già) e poi lo avvia con Avvia. Il traduttore
// Mozilla (WebAssembly) gira in una seconda finestra WebView2 invisibile ("motore"), che parte
// solo quando serve: chi non usa la traduzione non spende memoria.
var (
	motoreMu   sync.Mutex
	motore     webview2.WebView // finestra invisibile del traduttore Mozilla
	motoreURL  string           // pagina del motore sul server locale (percorso segreto)
	motoreDati string           // cartella dei dati di WebView2 (la stessa di Tasso)
	avviato    atomic.Bool
	srvLocale  *http.Server
)

// Prepara collega le funzioni ttXxx alla finestra di Tasso. Va chiamata prima di SetHtml.
// dati = cartella dei dati di WebView2 di Tasso.
func Prepara(w webview2.WebView, dati string) {
	base, err := os.UserCacheDir() // %LOCALAPPDATA%: stessa cartella di Tasso Traduce da solo (modelli già scaricati)
	if err != nil {
		base = os.TempDir()
	}
	dirApp = filepath.Join(base, "TassoTraduce")
	_ = os.MkdirAll(filepath.Join(dirApp, "logs"), 0o755)
	avviaLogErrori()
	caricaImp()
	wv = w
	motoreDati = dati
	collega(w)
}

// Avvia fa partire il resto in sottofondo (server locale, librerie di frasi, finestrella
// sopra i giochi). Non legge lo schermo finché l'utente non accende la traduzione.
func Avvia() {
	if wv == nil || !avviato.CompareAndSwap(false, true) {
		return
	}
	srv, indirizzo, err := avviaServerLocale()
	if err != nil {
		logErrore("server-locale", err, nil)
		return
	}
	srvLocale = srv
	motoreMu.Lock()
	motoreURL = indirizzo
	motoreMu.Unlock()
	traduttore = NuovoTraduttore(filepath.Join(dirApp, "memoria-traduzioni.json"), leggiImp().Glossario, moz)
	traduttore.QuandoCorretta(correzioneArrivata)
	traduttore.Coppia = coppiaAttiva
	avviaServerModGioco()
	go func() { // le librerie pronte prima della prima scritta
		defer guard("librerie")
		caricaCataloghi()
		caricaLiberi()
	}()
	quandoCambiaIA = func() { emetti("aggiorna", stato()) }
	avviaFinestrella(leggiImp().DimFont, suTasto, suZonaScelta)
	ov.ImpostaPosizione(leggiImp().Posizione)
	ov.ImpostaInglese(leggiImp().MostraEN)
	go cicloAutomatico()
	go cicloChatAltri()
	go cicloTuttoAutomatico()
	go preparaMotori()
	go func() { // le traduzioni nuove su disco ogni 2 minuti (se Windows si spegne di colpo)
		defer guard("salva-memoria")
		for range time.Tick(2 * time.Minute) {
			traduttore.SalvaMemoria()
		}
	}()
	if leggiImp().TuttoAuto || len(leggiImp().GiochiTradotti) > 0 {
		assicuraMotore() // traduzione "da sola" scelta dall'utente: il motore deve essere pronto
	}
}

// Chiudi salva tutto quando Tasso si chiude.
func Chiudi() {
	if !avviato.Load() {
		return
	}
	if traduttore != nil {
		traduttore.SalvaMemoria() // libreria del gioco e correzioni dell'IA: niente va perso all'uscita
	}
	ia.Ferma()
	sv.ferma()
	if ocr != nil {
		ocr.Chiudi()
	}
	motoreMu.Lock()
	m := motore
	motoreMu.Unlock()
	if m != nil {
		m.Dispatch(func() { m.Terminate() })
	}
	if srvLocale != nil {
		_ = srvLocale.Close()
	}
}

// assicuraMotore apre (una volta sola) la finestra invisibile con il traduttore Mozilla.
// Serve il traduttore già scaricato; altrimenti parte dopo lo scaricamento.
// motoreAvviato: il motore è aperto (o si sta aprendo). Se la finestra non parte torna false,
// così il prossimo tentativo (scaricamento, interruttore) riprova invece di restare bloccato.
var (
	motoreAvviato atomic.Bool
	motoreAvvioMu sync.Mutex
)

func assicuraMotore() {
	if !avviato.Load() || !mozillaPresente() {
		return
	}
	motoreMu.Lock()
	url := motoreURL
	motoreMu.Unlock()
	if url == "" {
		return
	}
	motoreAvvioMu.Lock()
	defer motoreAvvioMu.Unlock()
	if motoreAvviato.Load() {
		motoreMu.Lock()
		m := motore
		motoreMu.Unlock()
		if e, _ := mozErr.Load().(string); m != nil && !mozOK.Load() && e != "" {
			// il motore c'è ma ha detto che non è pronto (file mancanti o rovinati): ricarico la sua pagina
			mozErr.Store("") // l'errore vecchio non vale più: aspetto la risposta della pagina ricaricata
			m.Dispatch(func() { m.Navigate(url) })
		}
		return
	}
	motoreAvviato.Store(true)
	mozErr.Store("")
	func() {
		pronta := make(chan struct{})
		go func() {
			defer guard("motore")
			runtime.LockOSThread() // la finestra del motore ha il suo giro di messaggi su questo filo
			m := creaMotore()
			if m == nil {
				motoreAvviato.Store(false)
				close(pronta)
				mozillaPronto(false, "il traduttore non è partito (WebView2)")
				emetti("aggiorna", stato())
				return
			}
			_ = m.Bind("goMozillaPronto", func(ok bool, errore string) string {
				mozillaPronto(ok, errore)
				emetti("aggiorna", stato())
				return "ok"
			})
			_ = m.Bind("goMozFatto", func(id int64, testo, errore string) string { moz.fatto(id, testo, errore); return "ok" })
			motoreMu.Lock()
			motore = m
			motoreMu.Unlock()
			close(pronta)
			m.Navigate(url)
			m.Run()
			motoreMu.Lock()
			motore = nil
			motoreMu.Unlock()
			motoreAvviato.Store(false)
		}()
		<-pronta
	}()
}

// evalMotore esegue del codice nella pagina del motore (se c'è).
func evalMotore(js string) bool {
	motoreMu.Lock()
	m := motore
	motoreMu.Unlock()
	if m == nil {
		return false
	}
	m.Dispatch(func() { m.Eval(js) })
	return true
}

// creaMotore: finestra WebView2 fuori dallo schermo, senza icona nella barra e senza prendere
// il fuoco. Non la nascondo del tutto: una WebView nascosta può rallentare i suoi calcoli.
func creaMotore() (w webview2.WebView) {
	defer func() {
		if r := recover(); r != nil {
			logErrore("motore-finestra", r, debug.Stack())
			w = nil
		}
	}()
	w = webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  motoreDati,
		AutoFocus: false,
		WindowOptions: webview2.WindowOptions{
			Title: "Tasso Traduce (motore)", Width: 320, Height: 240,
		},
	})
	if w == nil {
		return nil
	}
	nascondiMotore(uintptr(w.Window()))
	return w
}

func finestraErrore(testo string) {
	t, _ := windows.UTF16PtrFromString(testo)
	c, _ := windows.UTF16PtrFromString("Tasso Traduce")
	user32.NewProc("MessageBoxW").Call(0, uintptr(unsafePtr(t)), uintptr(unsafePtr(c)), 0x10)
}

func finestraInfo(testo string) {
	t, _ := windows.UTF16PtrFromString(testo)
	c, _ := windows.UTF16PtrFromString("Tasso Traduce")
	user32.NewProc("MessageBoxW").Call(0, uintptr(unsafePtr(t)), uintptr(unsafePtr(c)), 0x40)
}

func avviaLogErrori() {
	p := filepath.Join(dirApp, "logs", "crash.txt")
	if fi, err := os.Stat(p); err == nil && fi.Size() > 1<<20 {
		_ = os.Rename(p, p+".old")
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString("--- Tasso Traduce " + versione + " aperto il " + time.Now().Format("02/01/2006 15:04:05") + "\r\n")
	_ = debug.SetCrashOutput(f, debug.CrashOptions{})
}

func logErrore(dove string, r interface{}, stack []byte) {
	f, err := os.OpenFile(filepath.Join(dirApp, "logs", "crash.txt"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "--- Errore fermato il %s in %s: %v\r\n%s\r\n", time.Now().Format("02/01/2006 15:04:05"), dove, r, stack)
}

// guard: con defer all'inizio di ogni goroutine, così un errore non chiude il programma.
func guard(dove string) {
	if r := recover(); r != nil {
		logErrore(dove, r, debug.Stack())
	}
}

// ---------------------------------------------------------------- server locale per l'interfaccia

// L'interfaccia e il traduttore Mozilla (WebAssembly) girano dentro WebView2 e leggono i file
// da questo piccolo server, che ascolta SOLO su 127.0.0.1 e solo con un percorso segreto.
func avviaServerLocale() (*http.Server, string, error) {
	tok := make([]byte, 16)
	_, _ = rand.Read(tok)
	pre := "/" + hex.EncodeToString(tok) + "/"
	ui, _ := fs.Sub(fileUI, "ui")
	mux := http.NewServeMux()
	mux.Handle(pre, http.StripPrefix(pre, http.FileServer(http.FS(ui))))
	mux.HandleFunc(pre+"modelli/registry.json", func(w http.ResponseWriter, r *http.Request) {
		reg := map[string]map[string]interface{}{"enit": {}}
		for _, f := range fileMozillaENIT {
			parte := "model"
			if strings.HasPrefix(f.Nome, "lex") {
				parte = "lex"
			} else if strings.HasPrefix(f.Nome, "vocab") {
				parte = "vocab"
			}
			reg["enit"][parte] = map[string]interface{}{"name": pre + "modelli/" + f.Nome, "size": f.Byte, "expectedSha256Hash": f.SHA256}
		}
		reg["iten"] = map[string]interface{}{}
		for _, f := range fileMozillaITEN {
			parte := "model"
			if strings.HasPrefix(f.Nome, "lex") {
				parte = "lex"
			} else if strings.HasPrefix(f.Nome, "vocab") {
				parte = "vocab"
			}
			reg["iten"][parte] = map[string]interface{}{"name": pre + "modelli-iten/" + f.Nome, "size": f.Byte, "expectedSha256Hash": f.SHA256}
		}
		// le altre lingue scaricate (ognuna verso e dall'inglese)
		for _, l := range lingueGioco {
			for _, c := range [][2]string{{l.Codice, "en"}, {"en", l.Codice}} {
				if c[0] == c[1] || c == [2]string{"en", "it"} || c == [2]string{"it", "en"} || !coppiaPresente(dirApp, c[0], c[1]) {
					continue
				}
				k := c[0] + c[1]
				reg[k] = map[string]interface{}{}
				for _, f := range filesCoppia(c[0], c[1]) {
					reg[k][parteFile(f.Nome)] = map[string]interface{}{"name": pre + "modelli-lingue/" + k + "/" + f.Nome, "size": f.Byte, "expectedSha256Hash": f.SHA256}
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(reg)
	})
	mux.Handle(pre+"modelli-lingue/", http.StripPrefix(pre+"modelli-lingue/", http.FileServer(http.Dir(filepath.Join(dirApp, "modelli", "lingue")))))
	mux.Handle(pre+"modelli/", http.StripPrefix(pre+"modelli/", http.FileServer(http.Dir(dirMozilla()))))
	mux.Handle(pre+"modelli-iten/", http.StripPrefix(pre+"modelli-iten/", http.FileServer(http.Dir(dirMozillaITEN()))))
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", err
	}
	srv := &http.Server{Handler: soloLocale(mux), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		defer guard("server-locale")
		_ = srv.Serve(l)
	}()
	return srv, fmt.Sprintf("http://%s%smotore.html", l.Addr().String(), pre), nil
}

func soloLocale(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if host != "127.0.0.1" {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".wasm") {
			w.Header().Set("Content-Type", "application/wasm")
		}
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------- collegamento con l'interfaccia

func emetti(nome string, dati interface{}) {
	if wv == nil {
		return
	}
	b, err := json.Marshal(dati)
	if err != nil {
		return
	}
	js := "window.tt && window.tt." + nome + " && window.tt." + nome + "(" + string(b) + ");"
	wv.Dispatch(func() { wv.Eval(js) })
}

func collega(w webview2.WebView) {
	_ = w.Bind("ttStato", func() map[string]interface{} { return stato() })
	// ttPrepara: la pagina Giochi è aperta (mai in Ufficio) e il traduttore è già scaricato:
	// lo preparo, così l'interruttore e la procedura lo trovano pronto
	_ = w.Bind("ttPrepara", func() string {
		if mozillaPresente() && !motoreAvviato.Load() {
			go assicuraMotore()
		}
		return "ok"
	})
	_ = w.Bind("ttScaricaMozilla", func() string { go scaricaMozilla(); return "ok" })
	_ = w.Bind("ttAnalizzaPC", func() string {
		go func() {
			defer guard("analisi")
			emetti("analisi", analizzaPC())
		}()
		return "ok"
	})
	_ = w.Bind("ttScaricaIA", func() string { go scaricaIA(); return "ok" })
	_ = w.Bind("ttScaricaChat", func() string { go scaricaChat(); return "ok" })
	_ = w.Bind("ttApplicaTraduzione", func(nome, cartella string, _ bool, modo string) string {
		assicuraMotore()
		if !acceso.Load() {
			acceso.Store(true)
			cambiaTasti(tastiCompleti(leggiImp().Tasti))
		}
		if modo == "file" {
			go applicaModGioco(nome, cartella)
		} else {
			go applicaTraduzione(nome, cartella)
		}
		return "ok"
	})
	_ = w.Bind("ttLingue", func() []map[string]any { return elencoLingue() })
	_ = w.Bind("ttImpostaLingue", func(da, a string) string {
		if _, ok := linguaDa(a); !ok {
			return "no"
		}
		if _, ok := linguaDa(da); !ok && da != "auto" {
			return "no"
		}
		if da == a || (da == "auto" && a == "en") {
			return "uguali"
		}
		muImp.Lock()
		cambiata := imp.LinguaDa != da || imp.LinguaA != a
		imp.LinguaDa, imp.LinguaA = da, a
		muImp.Unlock()
		salvaImp()
		if cambiata {
			linguaRilevata.Store("") // nuova scelta: riparto da zero con il riconoscimento
		}
		return "ok"
	})
	_ = w.Bind("ttInstallaOCR", func(codice string) string { return installaLetturaOCR(codice) })
	// 3.1: dopo "Apri il gioco" di Tasso controllo se la traduzione parte davvero.
	_ = w.Bind("ttVerifica", func(cartella string) string { return VerificaDopoAvvio(cartella) })
	_ = w.Bind("ttApriGioco", func(cartella string) string {
		assicuraMotore()
		if !acceso.Load() {
			acceso.Store(true)
			cambiaTasti(tastiCompleti(leggiImp().Tasti))
		}
		return apriGioco(cartella)
	})
	_ = w.Bind("ttFermaTraduzione", func(cartella string) string {
		if e := fermaModGioco(cartella); e != "" {
			return e
		}
		impostaGiocoTradotto(cartella, false)
		emetti("aggiorna", stato())
		return "ok"
	})
	_ = w.Bind("ttScaricaVoci", func() string { go scaricaVoci(); return "ok" })
	_ = w.Bind("ttAccendiVoci", func(on bool) map[string]string {
		if on && !acceso.Load() {
			return map[string]string{"errore": "Prima accendi la traduzione dei giochi con l'interruttore in Giochi."}
		}
		go func() {
			defer guard("voci-finestra")
			if err := accendiVoci(on); err != nil {
				emetti("info", map[string]string{"testo": "Voci: " + err.Error()})
			}
			emetti("aggiorna", stato())
		}()
		return map[string]string{}
	})
	// le traduzioni passano da Mozilla dentro questa stessa pagina: se aspettassi qui bloccherei
	// la pagina, quindi rispondo subito e mando il risultato con l'evento "chat"
	_ = w.Bind("ttTraduciChat", func(testo string) string {
		assicuraMotore()
		go func() {
			defer guard("chat-finestra")
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			en, err := traduciChat(ctx, testo)
			if err != nil {
				emetti("chat", map[string]string{"errore": err.Error()})
				return
			}
			copiato := "si"
			if scriviAppunti(en) != nil {
				copiato = "no"
			}
			emetti("chat", map[string]string{"it": testo, "en": en, "copiato": copiato})
		}()
		return "ok"
	})
	_ = w.Bind("ttScegliZona", func() string { selPerChat.Store(false); ov.DimenticaGioco(); ov.ApriSelezione(); return "ok" })
	_ = w.Bind("ttScegliZonaChat", func() string { scegliZonaChat(); return "ok" })
	_ = w.Bind("ttImpostaChatAltri", func(on bool) string {
		if on && !leggiImp().ZonaChat.Valida() {
			return "zona" // prima va scelta la zona della chat
		}
		impostaChatAltri(on)
		return "ok"
	})
	_ = w.Bind("ttTraduciOra", func() string { go traduciOra(true); return "ok" })
	_ = w.Bind("ttImpostaAuto", func(on bool) string { impostaAuto(on); return "ok" })
	_ = w.Bind("ttImpostaTasto", func(nome string, codice uint32) map[string]string {
		muImp.Lock()
		attuali := tastiCompleti(imp.Tasti)
		muImp.Unlock()
		if err := controllaTasto(nome, codice, attuali); err != nil {
			return map[string]string{"errore": err.Error()}
		}
		vecchio := attuali[nome]
		attuali[nome] = codice
		if acceso.Load() && cambiaTasti(attuali) > 0 { // preso da un altro programma: rimetto com'era
			attuali[nome] = vecchio
			cambiaTasti(attuali)
			return map[string]string{"errore": etichettaTasto(codice) + " lo usa già un altro programma: scegline un altro."}
		}
		muImp.Lock()
		imp.Tasti = attuali
		muImp.Unlock()
		salvaImp()
		return map[string]string{"etichetta": etichettaTasto(codice)}
	})
	_ = w.Bind("ttTastiDiBase", func() string {
		muImp.Lock()
		imp.Tasti = nil
		muImp.Unlock()
		salvaImp()
		if acceso.Load() {
			if n := cambiaTasti(tastiCompleti(nil)); n > 0 {
				return fmt.Sprintf("occupati:%d", n)
			}
		}
		return "ok"
	})
	_ = w.Bind("ttAccendi", func(on bool) bool {
		if on {
			assicuraMotore()
		}
		acceso.Store(on)
		if on {
			if n := cambiaTasti(tastiCompleti(leggiImp().Tasti)); n > 0 {
				emetti("info", map[string]string{"testo": fmt.Sprintf("%d tasti rapidi sono già usati da un altro programma: cambiali in Giochi → Impostazioni → Tasti nel gioco.", n)})
			}
		} else {
			cambiaTasti(map[string]uint32{}) // spento: lascio liberi i tasti
		}
		if !on {
			_ = accendiVoci(false) // spento: niente più ascolto dell'audio
		}
		if !on { // spento: tolgo subito ogni traduzione dallo schermo
			paginaAttiva.Store(false)
			paginaGiro.Add(1)
			NascondiPagina()
			ov.Nascondi()
		}
		return on
	})
	_ = w.Bind("ttGiochi", func() []map[string]any { return elencoGiochi() })
	_ = w.Bind("ttImpostaTuttoAuto", func(on bool) bool {
		muImp.Lock()
		imp.TuttoAuto = on
		muImp.Unlock()
		salvaImp()
		if !on && paginaAttiva.Load() {
			paginaAttiva.Store(false)
			paginaGiro.Add(1)
			NascondiPagina()
		}
		return on
	})
	_ = w.Bind("ttImpostaInglese", func(on bool) string {
		muImp.Lock()
		imp.MostraEN = on
		muImp.Unlock()
		salvaImp()
		ov.ImpostaInglese(on)
		return "ok"
	})
	_ = w.Bind("ttImpostaPosizione", func(p string) string {
		switch p {
		case "sopra", "sotto", "copri", "lato", "sottotitoli":
		default:
			p = "sopra"
		}
		muImp.Lock()
		imp.Posizione = p
		muImp.Unlock()
		salvaImp()
		ov.ImpostaPosizione(p)
		ricordaProfilo()
		return p
	})
	_ = w.Bind("ttImpostaFont", func(px int) int {
		muImp.Lock()
		imp.DimFont = clamp(px, 14, 48)
		px = imp.DimFont
		muImp.Unlock()
		salvaImp()
		ov.ImpostaFont(px)
		ricordaProfilo()
		return px
	})
	_ = w.Bind("ttImpostaMotore", func(m string) string {
		if m != "auto" && m != "ia" && m != "mozilla" {
			return "no"
		}
		muImp.Lock()
		imp.Motore = m
		muImp.Unlock()
		salvaImp()
		go preparaMotori()
		return "ok"
	})
	_ = w.Bind("ttCronologia", func() []Risultato { return cronologia() })
	_ = w.Bind("ttSalvaCronologia", func() string { return salvaCronologia() })
	_ = w.Bind("ttGlossario", func() Glossario { return leggiImp().Glossario })
	_ = w.Bind("ttSalvaGlossario", func(g Glossario) string {
		if g.Termini == nil {
			g.Termini = map[string]string{}
		}
		muImp.Lock()
		imp.Glossario = g
		muImp.Unlock()
		salvaImp()
		traduttore.ImpostaGlossario(g)
		return "ok"
	})
}

// ---------------------------------------------------------------- stato generale

var (
	traduttore *Traduttore
	ocr        *LettoreOCR
	ocrErr     string
	ia         = &ServerIA{}
	gpu        InfoGPU
	motoreAtt  atomic.Value // stringa: "IA", "Mozilla", ""
	mozOK      atomic.Bool
	mozErr     atomic.Value
	scaricando atomic.Bool
	occupato   atomic.Bool
)

func stato() map[string]interface{} {
	i := leggiImp()
	ma, _ := motoreAtt.Load().(string)
	me, _ := mozErr.Load().(string)
	return map[string]interface{}{
		"versione":        versione,
		"zona":            i.Zona,
		"auto":            i.Auto,
		"mostra_en":       i.MostraEN,
		"dim_font":        i.DimFont,
		"posizione":       i.Posizione,
		"tutto_auto":      i.TuttoAuto,
		"chat_altri":      i.ChatAltri,
		"zona_chat_ok":    i.ZonaChat.Valida(),
		"acceso":          acceso.Load(),
		"tasti":           etichetteTasti(i.Tasti),
		"motore":          i.Motore,
		"mozilla_file":    mozillaPresente(),
		"mozilla_ok":      mozOK.Load(),
		"mozilla_err":     me,
		"ia_file":         iaPresenteSvelto(),
		"ia_offerta":      motoreIADisponibile(),
		"ia_pc_ok":        iaAdattaPC(),
		"ia_pc_motivo":    iaMotivoPC(),
		"ia_attiva":       ia.Attivo(),
		"ia_profilo":      profiloAttuale(),
		"ia_corrette":     correzioniFatte(),
		"gpu":             gpu,
		"motore_ora":      ma,
		"ocr_err":         ocrErr,
		"scaricando":      scaricando.Load(),
		"chat_file":       chatPresente(),
		"giochi_tradotti": i.GiochiTradotti,
		"lingua_da":       i.LinguaDa,
		"lingua_a":        i.LinguaA,
		"voci":            statoVoci(),
		"scaricando_chat": scaricandoChat.Load(),
	}
}

func mozillaPresente() bool {
	for _, f := range fileMozillaENIT {
		if _, err := os.Stat(filepath.Join(dirMozilla(), f.Nome)); err != nil {
			return false
		}
	}
	return true
}

func scaricaMozilla() {
	defer guard("scarica-mozilla")
	if !scaricando.CompareAndSwap(false, true) {
		return
	}
	defer scaricando.Store(false)
	muImp.Lock()
	imp.MozillaSi = true
	muImp.Unlock()
	salvaImp()
	err := scaricaTutti(context.Background(), dirMozilla(), fileMozillaENIT, func(f, t int64) {
		emetti("avanzamento", map[string]interface{}{"cosa": "mozilla", "fatti": f, "totale": t})
	})
	if err != nil {
		emetti("erroreScarica", map[string]string{"cosa": "mozilla", "errore": err.Error()})
		return
	}
	assicuraMotore() // il traduttore è arrivato: apro il motore
	emetti("scaricato", map[string]string{"cosa": "mozilla"})
}

func mozillaPronto(ok bool, errore string) {
	mozOK.Store(ok)
	mozErr.Store(errore)
	if ok {
		go preparaMotori()
		avviaCollaudoSeServe()
	}
}

// preparaMotori sceglie i motori in base alle impostazioni e alla scheda video.
var muMotori sync.Mutex

func profiloAttuale() string {
	p, _ := profiloIA.Load().(string)
	return p
}

func correzioniFatte() map[string]int64 {
	f, c, n := traduttore.StatoCorrezioni()
	return map[string]int64{"fatte": f, "cambiate": c, "attesa": int64(n)}
}

func preparaMotori() {
	defer guard("motori")
	muMotori.Lock()
	defer muMotori.Unlock()
	// Mozilla traduce sempre e subito; l'IA (se installata) fa solo da correttore in sottofondo:
	// come traduttore da zero andava peggio di Mozilla (prova del 7/11/2026).
	var motori []Motore
	if mozOK.Load() {
		motori = append(motori, moz)
	}
	traduttore.ImpostaMotori(motori...)
	if len(motori) > 0 {
		motoreAtt.Store(motori[0].Nome())
	} else {
		motoreAtt.Store("")
	}
	if iaPresente() {
		avviaSorveglianzaIA()
	}
	emetti("aggiorna", stato())
}

// ---------------------------------------------------------------- motore Mozilla (dentro WebView2)

type ponteMozilla struct {
	mu     sync.Mutex
	id     int64
	attesa map[int64]chan [2]string
}

var moz = &ponteMozilla{attesa: map[int64]chan [2]string{}}

func (p *ponteMozilla) Nome() string { return "Mozilla" }

type chiaveLotto struct{}

// conLotto: le traduzioni fatte con questo contesto vanno al traduttore "a gruppi" (più fili
// insieme), usato per le schermate con tante scritte tradotte tutte insieme.
func conLotto(ctx context.Context) context.Context {
	return context.WithValue(ctx, chiaveLotto{}, true)
}

func (p *ponteMozilla) Traduci(ctx context.Context, r RichiestaTraduzione) (string, error) {
	return p.traduciCoppia(ctx, r.Testo, "en", "it")
}

// TraduciCoppia: per le lingue diverse da inglese → italiano (con l'inglese in mezzo se serve).
func (p *ponteMozilla) TraduciCoppia(ctx context.Context, testo, da, a string) (string, error) {
	return p.traduciCoppia(ctx, testo, da, a)
}

// traduciCoppia: Mozilla dentro WebView2, da una lingua all'altra ("en"→"it" o "it"→"en").
func (p *ponteMozilla) traduciCoppia(ctx context.Context, testo, da, a string) (string, error) {
	if !mozOK.Load() {
		return "", errors.New("traduttore Mozilla non pronto")
	}
	p.mu.Lock()
	p.id++
	id := p.id
	ch := make(chan [2]string, 1)
	p.attesa[id] = ch
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.attesa, id)
		p.mu.Unlock()
	}()
	b, _ := json.Marshal(testo)
	lotto := ctx.Value(chiaveLotto{}) != nil
	js := fmt.Sprintf("window.tt && window.tt.moz(%d, %s, %q, %q, %v);", id, b, da, a, lotto)
	if !evalMotore(js) {
		return "", errors.New("traduttore Mozilla non pronto")
	}
	select {
	case v := <-ch:
		if v[1] != "" {
			return "", errors.New(v[1])
		}
		return v[0], nil
	case <-time.After(15 * time.Second):
		return "", errors.New("il traduttore Mozilla non risponde")
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (p *ponteMozilla) fatto(id int64, testo, errore string) {
	p.mu.Lock()
	ch := p.attesa[id]
	p.mu.Unlock()
	if ch != nil {
		select {
		case ch <- [2]string{testo, errore}:
		default:
		}
	}
}

// ---------------------------------------------------------------- tasti, zona, traduzione

// acceso: interruttore generale. All'avvio è sempre spento: niente legge lo schermo finché
// non lo accendi tu.
var acceso atomic.Bool

func suTasto(id int) {
	if !acceso.Load() && id != tastoCronologia {
		emetti("info", map[string]string{"testo": "La traduzione dei giochi è spenta: accendila con l'interruttore in Giochi."})
		return
	}
	switch id {
	case tastoSeleziona:
		ov.RicordaGioco() // F9 premuto mentre giochi: quella è la finestra del gioco
		ov.ApriSelezione()
	case tastoTraduci:
		traduciOra(true)
	case tastoAuto:
		impostaAuto(!leggiImp().Auto)
	case tastoInglese:
		muImp.Lock()
		imp.MostraEN = !imp.MostraEN
		on := imp.MostraEN
		muImp.Unlock()
		salvaImp()
		ov.ImpostaInglese(on)
		emetti("aggiorna", stato())
	case tastoPagina:
		attivaPagina()
	case tastoChat:
		go suTastoChat()
	case tastoVoci:
		go func() {
			defer guard("commuta-voci")
			on, err := commutaVoci()
			if err != nil {
				avvisoBreve("Voci: " + err.Error())
			} else if on {
				avvisoBreve("Sottotitoli delle voci accesi: ascolto l'audio del gioco (tutto resta sul PC)")
			} else {
				avvisoBreve("Sottotitoli delle voci spenti")
			}
			emetti("aggiorna", stato())
		}()
	case tastoCronologia:
		emetti("apriCronologia", nil)
		if wv != nil {
			h := uintptr(wv.Window())
			pShowWindow.Call(h, 9) // SW_RESTORE
			pSetForeground.Call(h)
		}
	}
}

func suZonaScelta(z Zona) {
	if selPerChat.Swap(false) {
		zonaChatScelta(z)
		return
	}
	muImp.Lock()
	imp.Zona = z
	muImp.Unlock()
	ricordaProfilo()
	salvaImp()
	muImpronte.Lock()
	ultimaImp = Impronta{}
	muImpronte.Unlock()
	emetti("aggiorna", stato())
	traduciOra(true)
}

func impostaAuto(on bool) {
	muImp.Lock()
	imp.Auto = on
	muImp.Unlock()
	salvaImp()
	if !on {
		ov.Nascondi()
	}
	emetti("aggiorna", stato())
}

var (
	muImpronte   sync.Mutex
	ultimaImp    Impronta // impronta dell'ultima zona letta
	precedente   Impronta // cattura del giro prima (per aspettare che il testo smetta di scorrere)
	ultimoTesto  string
	muCronologia sync.Mutex
	righe        []Risultato
)

// soglie di cambiamento (0-255): sotto "fermo" il testo non si muove più
const (
	sogliaCambiato = 4.0
	sogliaFermo    = 1.2
)

func cicloAutomatico() {
	defer guard("ciclo")
	for {
		time.Sleep(450 * time.Millisecond)
		controllaProfilo()
		i := leggiImp()
		if !acceso.Load() || !i.Auto || !i.Zona.Valida() || occupato.Load() || paginaAttiva.Load() {
			continue
		}
		// davanti c'è Tasso Traduce stesso o il desktop: niente da tradurre, risparmio il PC
		if fg, _, _ := pGetForeground.Call(); eNostro(exeDiFinestra(fg)) {
			continue
		}
		img, err := cattura(i.Zona)
		if err != nil {
			continue
		}
		adesso := calcolaImpronta(img)
		if i.Posizione == "sottotitoli" {
			// nei video lo sfondo si muove sempre: niente confronto delle immagini, leggo il
			// testo ogni volta (12 ms) e traduco solo quando cambia o sparisce
			leggiETraduci(img, adesso, false)
			continue
		}
		muImpronte.Lock()
		fermo := adesso.differenza(precedente) < sogliaFermo
		precedente = adesso
		cambiato := adesso.differenza(ultimaImp) > sogliaCambiato
		muImpronte.Unlock()
		// Il dialogo è cambiato rispetto all'ultimo letto, e ha smesso di comparire lettera per lettera.
		if cambiato && fermo {
			leggiETraduci(img, adesso, false)
		}
	}
}

func traduciOra(forza bool) {
	defer guard("traduci-ora")
	i := leggiImp()
	if !i.Zona.Valida() {
		// nessuna zona scelta: traduco tutto lo schermo del gioco (quello dove sta la sua finestra)
		attivaPagina()
		return
	}
	img, err := cattura(i.Zona)
	if err != nil {
		emetti("info", map[string]string{"testo": err.Error()})
		return
	}
	leggiETraduci(img, calcolaImpronta(img), forza)
}

func leggiETraduci(img *image.RGBA, impronta Impronta, forza bool) {
	if !occupato.CompareAndSwap(false, true) {
		return
	}
	defer occupato.Store(false)
	muImpronte.Lock()
	ultimaImp = impronta
	muImpronte.Unlock()
	if ocr == nil {
		l, err := avviaOCR(filepath.Join(dirApp, "lavoro"))
		if err != nil {
			ocrErr = "La lettura del testo di Windows non parte: " + err.Error()
			emetti("info", map[string]string{"testo": ocrErr})
			return
		}
		ocr, ocrErr = l, ""
	}
	var buf bytes.Buffer
	if err := preparaPerOCR(img, &buf); err != nil {
		return
	}
	t0 := time.Now()
	grezzo, err := ocr.Leggi(buf.Bytes())
	if err != nil {
		emetti("info", map[string]string{"testo": "Lettura del testo: " + err.Error()})
		return
	}
	// lettura dubbia (simboli strani o quasi niente): riprovo tenendo solo le scritte chiare
	// o colorate, e tengo la lettura più sensata
	if stranezza(grezzo) > 0.02 {
		var buf2 bytes.Buffer
		if preparaLuminosoPerOCR(img, &buf2) == nil {
			if g2, err2 := ocr.Leggi(buf2.Bytes()); err2 == nil && stranezza(g2) < stranezza(grezzo) {
				grezzo = g2
			}
		}
	}
	msOCR := time.Since(t0).Milliseconds()
	if g := ov.giocoAttuale(); g != 0 {
		apriLibreriaPer(g)
	} else if fg, _, _ := pGetForeground.Call(); fg != 0 {
		apriLibreriaPer(fg)
	}
	pulito := pulisci(grezzo)
	if pulito == "" {
		// il dialogo o il sottotitolo è sparito: tolgo anche la traduzione
		if ultimoTesto != "" {
			ultimoTesto = ""
			muDialogo.Lock()
			dialogoMostrato = mostratoDialogo{}
			muDialogo.Unlock()
			ov.Nascondi()
		}
		return
	}
	if pulito == ultimoTesto && !forza {
		return
	}
	ultimoTesto = pulito
	z := leggiImp().Zona
	ov.Mostra(z, "…", pulito, "")
	r := traduttore.Traduci(context.Background(), grezzo)
	if r.Errore != "" {
		ov.Mostra(z, "Non riesco a tradurre adesso.", pulito, r.Errore)
		emetti("info", map[string]string{"testo": "Traduzione non riuscita: " + r.Errore})
		return
	}
	testo := r.Tradotto
	if r.Parlante != "" {
		testo = r.Parlante + ": " + testo
	}
	auto := nomeTasto("auto") + " automatico: no"
	if leggiImp().Auto {
		auto = "● Automatico"
	}
	statoRiga := fmt.Sprintf("%s   ·   %s   ·   %.1f s   ·   %s ritraduci   ·   %s inglese", auto, r.Motore, float64(msOCR+r.Millisec)/1000, nomeTasto("traduci"), nomeTasto("inglese"))
	ov.Mostra(z, testo, r.Originale, statoRiga)
	muDialogo.Lock()
	dialogoMostrato = mostratoDialogo{z, r.Parlante, r.Tradotto, r.Originale, statoRiga}
	muDialogo.Unlock()
	r.Millisec += msOCR // tempo totale: lettura + traduzione
	muCronologia.Lock()
	righe = append(righe, r)
	if len(righe) > 1000 {
		righe = righe[len(righe)-1000:]
	}
	muCronologia.Unlock()
	emetti("riga", r)
}

// ultimo dialogo mostrato nella finestrella: serve per sostituirlo quando arriva la correzione
type mostratoDialogo struct {
	z                                  Zona
	parlante, tradotto, inglese, stato string
}

var (
	muDialogo       sync.Mutex
	dialogoMostrato mostratoDialogo
)

// correzioneArrivata: l'IA ha migliorato una frase. Se è ancora sullo schermo (scritte della
// schermata o finestrella dei dialoghi) la cambio subito.
func correzioneArrivata(inglese, bozza, corretta string) {
	modCorretta(inglese, bozza, corretta)
	if paginaAttiva.Load() {
		sostituisciTraduzione(inglese, bozza, corretta)
	}
	if ov.staMostrandoVoce() {
		if ov.cambiaVoce(bozza, corretta) {
			return
		}
	}
	muDialogo.Lock()
	d := dialogoMostrato
	ok := d.tradotto != "" && d.tradotto == bozza
	if ok {
		dialogoMostrato.tradotto = corretta
	}
	muDialogo.Unlock()
	if ok {
		testo := corretta
		if d.parlante != "" {
			testo = d.parlante + ": " + corretta
		}
		if !ov.staMostrandoVoce() {
			ov.Mostra(d.z, testo, d.inglese, strings.Replace(d.stato, "Mozilla", "Mozilla + IA", 1))
		}
	}
}

func cronologia() []Risultato {
	muCronologia.Lock()
	defer muCronologia.Unlock()
	return append([]Risultato(nil), righe...)
}

// salvaCronologia scrive i dialoghi tradotti in Documenti\Tasso Traduce (solo sul PC).
func salvaCronologia() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, "Documents", "Tasso Traduce")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	p := filepath.Join(dir, "dialoghi-"+time.Now().Format("2006-01-02_15-04")+".txt")
	var b strings.Builder
	for _, r := range cronologia() {
		chi := ""
		if r.Parlante != "" {
			chi = r.Parlante + ": "
		}
		b.WriteString(chi + r.Originale + "\r\n→ " + chi + r.Tradotto + "\r\n\r\n")
	}
	if os.WriteFile(p, []byte("\uFEFF"+b.String()), 0o644) != nil {
		return ""
	}
	return p
}

func unsafePtr(p *uint16) unsafe.Pointer { return unsafe.Pointer(p) }

func etichetteTasti(t map[string]uint32) map[string]string {
	out := map[string]string{}
	for k, v := range tastiCompleti(t) {
		out[k] = etichettaTasto(v)
	}
	return out
}

// creaFinestra: se WebView2 non parte (succede subito dopo l'avvio di Windows) lo dico invece
// di chiudermi in silenzio.
func creaFinestra() (w webview2.WebView) {
	defer func() {
		if r := recover(); r != nil {
			logErrore("finestra", r, debug.Stack())
			finestraErrore("La finestra di Tasso Traduce non si è aperta (WebView2 di Windows non ha risposto). Chiudi e riapri Tasso; se succede ancora, riavvia il PC.")
			w = nil
		}
	}()
	return webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  filepath.Join(dirApp, cartellaWebView),
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title: "Tasso Traduce", Width: 960, Height: 680, Center: true,
		},
	})
}

// nomeUnaCopia: una sola copia di Tasso Traduce aperta (le prove ne usano un altro, così non
// chiudono la copia vera dell'utente).
var nomeUnaCopia = "Local\\TassoTraduceUnaCopia"

var cartellaWebView = "WebView2"
