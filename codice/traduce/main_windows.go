//go:build windows

// Tasso Traduce - traduce in italiano i dialoghi inglesi dei giochi, sul PC, senza internet.
// Strumento a sé della famiglia Tasso, di Sergio Tassoni. Licenza GPL-3.0.
package traduce

import (
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
	"runtime"
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
	LivelloIA string `json:"livello_ia"` // "auto" (o vuoto), "grande", "media", "piccola": scelto nella procedura a passi
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
	ZonaChat  Zona   `json:"zona_chat"`           // 3.1: dove sta la chat degli altri in questo gioco
	ChatDove  string `json:"chat_dove,omitempty"` // cartella del programma (per dire nella pagina del gioco se è scelta)
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

// muFileImp: un salvataggio alla volta (due insieme scriverebbero lo stesso .tmp).
var muFileImp sync.Mutex

func salvaImp() {
	muFileImp.Lock()
	defer muFileImp.Unlock()
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
	go func() { // script della traduzione rimasti da sessioni vecchie (più di un giorno)
		defer guard("pulisci-script")
		pulisciScriptTT(cartellaScript(), time.Now())
	}()
	motoreMu.Lock()
	motoreURL = indirizzo
	motoreMu.Unlock()
	// memoria in un file nuovo (formato a lista): le versioni di prima leggono il vecchio come una
	// mappa e lo sovrascriverebbero. Il vecchio resta, e la prima volta le sue frasi passano qui.
	traduttore = NuovoTraduttore(filepath.Join(dirApp, "memoria-traduzioni-2.json"), leggiImp().Glossario, moz)
	traduttore.passaDaVecchiaMemoria(filepath.Join(dirApp, "memoria-traduzioni.json"))
	traduttore.QuandoCorretta(correzioneArrivata)
	traduttore.Coppia = coppiaAttiva
	avviaServerModGioco()
	go func() { // le librerie pronte prima della prima scritta
		defer guard("librerie")
		// Qualche secondo dopo: così all'avvio il processore resta libero per aprire la finestra di Tasso.
		time.Sleep(4 * time.Second)
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
	caricaLingueOCRInSottofondo() // PowerShell in sottofondo: la finestra non aspetta
	go func() {                   // le traduzioni nuove su disco ogni 2 minuti (se Windows si spegne di colpo)
		defer guard("salva-memoria")
		for range time.Tick(2 * time.Minute) {
			traduttore.SalvaMemoria()
		}
	}()
	if !InUfficio.Load() && (leggiImp().TuttoAuto || len(leggiImp().GiochiTradotti) > 0) {
		assicuraMotore() // traduzione "da sola" scelta dall'utente: il motore deve essere pronto
	}
	// all'avvio la traduzione resta spenta (scelta di Sergio del 7/11: niente legge lo schermo da
	// solo), ma F7 premuto nel gioco la accende
	cambiaTasti(soloTastoAccendi())
}

// InUfficio: Tasso è in modalità Ufficio (niente giochi né traduzione). Lo imposta Tasso
// all'avvio e a ogni cambio di modalità con ImpostaUfficio.
var InUfficio atomic.Bool

// ImpostaUfficio segna la modalità di Tasso. Tornando a Gaming riapre il motore se serve
// (traduzione "da sola" o giochi tradotti dentro il gioco).
func ImpostaUfficio(on bool) {
	if InUfficio.Swap(on) == on || on || !avviato.Load() {
		return
	}
	if !acceso.Load() {
		go cambiaTasti(soloTastoAccendi()) // di nuovo Gaming: F7 nel gioco accende la traduzione
	}
	if leggiImp().TuttoAuto || len(leggiImp().GiochiTradotti) > 0 {
		go func() {
			defer guard("motore-gaming")
			assicuraMotore()
		}()
	}
}

// SalvaAdesso: impostazioni e memoria delle traduzioni su disco, senza fermare niente.
// Serve quando Windows si spegne o si disconnette (lì Tasso può essere chiuso senza Chiudi).
func SalvaAdesso() {
	if !avviato.Load() {
		return
	}
	salvaImp()
	if traduttore != nil {
		traduttore.SalvaMemoria()
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
	if traduttore != nil {
		// una correzione dell'IA arrivata mentre il motore si chiudeva: salvo anche quella
		// (salva solo se qualcosa è cambiato, quindi di solito non scrive niente)
		traduttore.SalvaMemoria()
	}
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
			rinascondiMotore(m)
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
				mozillaPronto(false, "la finestra nascosta del traduttore non si è aperta")
				emetti("aggiorna", stato())
				return
			}
			_ = m.Bind("goMozillaPronto", func(ok bool, errore string) string {
				h := uintptr(m.Window())
				m.Dispatch(func() { nascondiMotore(h) }) // pagina caricata: la rimetto via, per sicurezza
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
			rinascondiMotore(m)
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
	// la finestra è fuori dallo schermo: Edge non deve crederla coperta e rallentare la pagina
	if os.Getenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS") == "" {
		_ = os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "--disable-features=CalculateNativeWinOcclusion --disable-background-timer-throttling --disable-renderer-backgrounding --disable-backgrounding-occluded-windows")
	}
	w = webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  motoreDati,
		AutoFocus: false,
		WindowOptions: webview2.WindowOptions{
			Title: "Tasso Traduce (motore)", Width: 320, Height: 240,
			Nascosta: true, // nasce già fuori dallo schermo, trasparente ai clic e senza pulsante nella barra
		},
	})
	if w == nil {
		return nil
	}
	nascondiMotore(uintptr(w.Window()))
	return w
}

// rinascondiMotore ripete nascondiMotore nel filo della finestra (subito e dopo 0,5, 2 e 5
// secondi): WebView2 a volte rimette a posto la finestra mentre si avvia o carica la pagina.
func rinascondiMotore(m webview2.WebView) {
	if m == nil {
		return
	}
	h := uintptr(m.Window())
	m.Dispatch(func() { nascondiMotore(h) })
	for _, d := range []time.Duration{500 * time.Millisecond, 2 * time.Second, 5 * time.Second} {
		time.AfterFunc(d, func() {
			motoreMu.Lock()
			ancora := motore == m
			motoreMu.Unlock()
			if ancora {
				m.Dispatch(func() { nascondiMotore(h) })
			}
		})
	}
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
	_ = w.Bind("ttScaricaIA", func() string { go scaricaIA(); return "ok" })
	_ = w.Bind("ttScaricaChat", func() string { go scaricaChat(); return "ok" })
	_ = w.Bind("ttApplicaTraduzione", func(nome, cartella string, rischioOK bool, modo string) string {
		accendiTraduzione()
		if modo == "file" {
			go applicaModGioco(nome, cartella, rischioOK)
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
		if da == a {
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
	// lingue scelte in Impostazioni ma non scaricate: scarico i modelli che mancano
	_ = w.Bind("ttScaricaLingue", func(da, a string) string {
		if da == "" || da == "auto" {
			da = "en"
		}
		if _, ok := linguaDa(da); !ok {
			return "no"
		}
		if _, ok := linguaDa(a); !ok {
			return "no"
		}
		if !scaricandoLin.CompareAndSwap(false, true) {
			return "occupato"
		}
		go func() {
			defer guard("scarica-lingue")
			defer scaricandoLin.Store(false)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
			defer cancel()
			err := scaricaLingue(ctx, da, a, func(p int) {
				emetti("avanzamento", map[string]interface{}{"cosa": "lingue", "fatti": p, "totale": 100})
			})
			if err != nil {
				emetti("erroreScarica", map[string]string{"cosa": "lingue", "errore": err.Error()})
				return
			}
			if da == "en" && a == "it" {
				assicuraMotore() // il traduttore veloce è arrivato adesso: apro il motore
			} else {
				ricaricaMozilla()
			}
			emetti("lingue", elencoLingue())
			emetti("scaricato", map[string]string{"cosa": "lingue"})
		}()
		return "ok"
	})
	_ = w.Bind("ttInstallaOCR", func(codice string) string { return installaLetturaOCR(codice) })
	// 3.1: dopo "Apri il gioco" di Tasso controllo se la traduzione parte davvero.
	_ = w.Bind("ttVerifica", func(cartella string) string { return VerificaDopoAvvio(cartella) })
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
				emetti("info", map[string]string{"testo": testoTasso("Voci: ", "Voices: ") + trTasso(err.Error())})
			}
			emetti("aggiorna", stato())
		}()
		return map[string]string{}
	})
	// le traduzioni passano da Mozilla dentro questa stessa pagina: se aspettassi qui bloccherei
	// la pagina, quindi rispondo subito e mando il risultato con l'evento "chat"
	_ = w.Bind("ttScegliZona", func() string { go scegliZonaGioco(); return "ok" })
	_ = w.Bind("ttScegliZonaChat", func() string { go scegliZonaChat(); return "ok" })
	_ = w.Bind("ttImpostaChatAltri", func(on bool) string {
		if on && !zonaChatPresente(leggiImp()) {
			return "zona" // prima va scelta la zona della chat
		}
		if on {
			accendiTraduzione() // la chat degli altri legge lo schermo solo con la traduzione accesa
		}
		impostaChatAltri(on)
		return "ok"
	})
	_ = w.Bind("ttImpostaTasto", func(nome string, codice uint32) map[string]string {
		muImp.Lock()
		attuali := tastiCompleti(imp.Tasti)
		muImp.Unlock()
		if err := controllaTasto(nome, codice, attuali); err != nil {
			return map[string]string{"errore": err.Error()}
		}
		vecchio := attuali[nome]
		attuali[nome] = codice
		occupato := map[string]string{"errore": etichettaTasto(codice) + " lo usa già Windows o un altro programma: scegline un altro."}
		var altri []string // altri tasti (non quello nuovo) presi da un altro programma
		if acceso.Load() {
			for _, f := range cambiaTasti(attuali) {
				if f == nome { // il tasto nuovo è preso da un altro programma: rimetto com'era
					attuali[nome] = vecchio
					cambiaTasti(attuali)
					return occupato
				}
				altri = append(altri, f)
			}
		} else if !InUfficio.Load() {
			// traduzione spenta: provo lo stesso il tasto nuovo (un attimo), così dico subito se è
			// già preso, poi rimetto solo il tasto che accende la traduzione dal gioco
			f := cambiaTasti(map[string]uint32{nome: codice})
			if len(f) > 0 {
				cambiaTasti(soloTastoAccendi())
				return occupato
			}
		}
		muImp.Lock()
		imp.Tasti = attuali
		muImp.Unlock()
		salvaImp()
		if !acceso.Load() {
			cambiaTasti(soloTastoAccendi()) // subito il tasto nuovo, senza riavviare
		}
		r := map[string]string{"etichetta": etichettaTasto(codice)}
		if len(altri) > 0 {
			r["avviso"] = "Già usati da un altro programma: " + nomiTasti(altri, attuali) + "."
		}
		return r
	})
	_ = w.Bind("ttTastiDiBase", func() string {
		muImp.Lock()
		imp.Tasti = nil
		muImp.Unlock()
		salvaImp()
		if acceso.Load() {
			if f := cambiaTasti(tastiCompleti(nil)); len(f) > 0 {
				return fmt.Sprintf("occupati:%d", len(f))
			}
		} else if f := cambiaTasti(soloTastoAccendi()); len(f) > 0 {
			return "occupati:1"
		}
		return "ok"
	})
	_ = w.Bind("ttAccendi", func(on bool) bool { return accendi(on) })
	// passando a Ufficio la traduzione si spegne e i tasti dei giochi si tolgono; tornando a Gaming
	// tornano i tasti (la traduzione la riaccendi tu con F7)
	_ = w.Bind("ttSpegniPerUfficio", func() bool {
		accendi(false)
		cambiaTasti(map[string]uint32{}) // in Ufficio nessun tasto dei giochi
		return false
	})
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
		sveglia() // l'IA si accende o si spegne subito, non al prossimo controllo
		return "ok"
	})
	_ = w.Bind("ttImpostaLivelloIA", func(l string) string {
		if l != "auto" && l != "grande" && l != "media" && l != "piccola" {
			return "no"
		}
		muImp.Lock()
		imp.LivelloIA = l
		muImp.Unlock()
		salvaImp()
		livelloCambiato.Store(true) // si applica subito, anche se è un livello più alto
		sveglia()
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
		if traduttore != nil {
			traduttore.ImpostaGlossario(g)
		}
		return "ok"
	})
}

// ---------------------------------------------------------------- stato generale

var (
	traduttore *Traduttore
	ocr        *LettoreOCR
	ia         = &ServerIA{}
	motoreAtt  atomic.Value // stringa: "IA", "Mozilla", ""
	mozOK      atomic.Bool
	mozErr     atomic.Value
	scaricando atomic.Bool
	occupato   atomic.Bool
)

// gpu e ocrErr si scrivono da altri fili mentre stato() li legge: sempre col lucchetto.
var (
	muStatoVario sync.Mutex
	gpu          InfoGPU
	ocrErr       string
)

func gpuAttuale() InfoGPU {
	muStatoVario.Lock()
	defer muStatoVario.Unlock()
	return gpu
}

func impostaGPU(g InfoGPU) {
	muStatoVario.Lock()
	gpu = g
	muStatoVario.Unlock()
}

func ocrErrore() string {
	muStatoVario.Lock()
	defer muStatoVario.Unlock()
	return ocrErr
}

func impostaOcrErr(s string) {
	muStatoVario.Lock()
	ocrErr = s
	muStatoVario.Unlock()
}

// giochiTradottiCopia: copia della mappa, fatta col lucchetto (json.Marshal della mappa vera
// mentre impostaGiocoTradotto la cambia farebbe chiudere Tasso).
func giochiTradottiCopia() map[string]bool {
	muImp.Lock()
	defer muImp.Unlock()
	out := make(map[string]bool, len(imp.GiochiTradotti))
	for k, v := range imp.GiochiTradotti {
		out[k] = v
	}
	return out
}

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
		"zona_chat_ok":    zonaChatPresente(i),
		"chat_giochi":     giochiConChat(),
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
		"ia_livello":      i.LivelloIA,
		"ia_media":        mediaPresente(),
		"ia_piccola":      mediaPresente(),
		"ia_gb":           map[string]float64{"piccola": vramPiccola, "piccola_min": vramPiccolaStrati(stratiPiccolaMin), "media": vramMedia, "grande": vramIntero},
		"ia_corrette":     correzioniFatte(),
		"gpu":             gpuAttuale(),
		"motore_ora":      ma,
		"ocr_err":         ocrErrore(),
		"scaricando":      scaricando.Load(),
		"chat_file":       chatPresente(),
		"giochi_tradotti": giochiTradottiCopia(),
		"lingua_da":       i.LinguaDa,
		"lingua_a":        linguaArrivo(i),
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
		if paginaAttiva.Load() {
			// le scritte rimaste in inglese aspettando il traduttore si traducono adesso
			paginaRileggi.Store(true)
		}
	}
}

// preparaMotori sceglie i motori in base alle impostazioni e alla scheda video.
var muMotori sync.Mutex

func profiloAttuale() string {
	p, _ := profiloIA.Load().(string)
	return p
}

func correzioniFatte() map[string]int64 {
	if traduttore == nil { // Tasso Traduce non ancora avviato
		return map[string]int64{"fatte": 0, "cambiate": 0, "attesa": 0}
	}
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
		return "", errors.New("traduttore veloce non pronto")
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
		return "", errors.New("traduttore veloce non pronto")
	}
	select {
	case v := <-ch:
		if v[1] != "" {
			return "", errors.New(v[1])
		}
		return v[0], nil
	case <-time.After(15 * time.Second):
		return "", errors.New("il traduttore veloce non risponde")
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

// accendiTraduzione accende la traduzione dei giochi (motore e tasti rapidi), se era spenta.
func accendiTraduzione() {
	if !avviato.Load() {
		return // Tasso Traduce non è ancora partito: niente tasti senza la finestrella
	}
	assicuraMotore()
	if acceso.CompareAndSwap(false, true) {
		cambiaTasti(tastiCompleti(leggiImp().Tasti))
	}
}

func suTasto(id int) {
	if !acceso.Load() && id == tastoPagina && !InUfficio.Load() {
		// traduttore spento (gioco aperto fuori da Tasso, o interruttore mai acceso): il tasto
		// nel gioco lo accende da solo, se il traduttore è scaricato, e lo dice con un avviso
		if !mozOK.Load() && !mozillaPresente() {
			avvisoBreve(testoTasso("Tasso: prima scarica il traduttore in Tasso → Giochi → Impostazioni.",
				"Tasso: first download the translator in Tasso → Games → Settings."))
			return
		}
		if !accendi(true) {
			emetti("info", map[string]string{"testo": "Non riesco ad accendere la traduzione adesso: chiudi Tasso e riaprilo, poi riprova."})
			return
		}
		emetti("aggiorna", stato())
		// niente fascia nera: il badge in alto al centro dice "Traduco…" appena parte la schermata
		emetti("info", map[string]string{"testo": testoTasso("Traduttore acceso: traduco le scritte del gioco.", "Translator on: translating the game text.")})
	}
	if !acceso.Load() && id != tastoCronologia {
		emetti("info", map[string]string{"testo": "La traduzione dei giochi è spenta: accendila con l'interruttore in Giochi."})
		return
	}
	switch id {
	case tastoSeleziona:
		if sel.hwnd.Load() != 0 {
			return // la scelta è già aperta: il velo davanti non è il gioco
		}
		selPerChat.Store(false) // F9 sceglie sempre la zona dei dialoghi
		ov.RicordaGioco()       // F9 premuto mentre giochi: quella è la finestra del gioco
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
				avvisoBreve(testoTasso("Voci: ", "Voices: ") + trTasso(err.Error()))
			} else if on {
				avvisoBreve(testoTasso("Sottotitoli delle voci accesi: ascolto l'audio del gioco (tutto resta sul PC)",
					"Voice subtitles on: I'm listening to the game audio (everything stays on the PC)"))
			} else {
				avvisoBreve(testoTasso("Sottotitoli delle voci spenti", "Voice subtitles off"))
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
	if ov.giocoAttuale() == 0 {
		// zona scelta col pulsante in Tasso: il gioco è la finestra sotto il riquadro
		ov.impostaGioco(finestraInZona(z))
	}
	if paginaAttiva.Load() {
		// hai scelto una zona: traduco solo quella. La traduzione di tutta la schermata si ferma,
		// se no c'erano due traduzioni una sopra l'altra (PC Cafe Simulator, 24/11)
		paginaAttiva.Store(false)
		paginaGiro.Add(1)
		NascondiPagina()
		// e il tutto automatico non la riaccende da solo su questo gioco (ci sarebbero di nuovo due traduzioni)
		impostaPausa(exeDiFinestra(paginaGiocoA.Load()), true)
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
		ov.Spegni()
	}
	emetti("aggiorna", stato())
}

var (
	muImpronte   sync.Mutex
	ultimaImp    Impronta // impronta dell'ultima zona letta
	precedente   Impronta // cattura del giro prima (per aspettare che il testo smetta di scorrere)
	ultimoTesto  string
	candidatoSub string      // sottotitoli: ultima lettura, in attesa di una seconda uguale
	ultimoGrezzo string      // ultima lettura dell'OCR (per le prove)
	erroreSub    time.Time   // sottotitoli: ultimo errore di traduzione (non riprovo a raffica)
	rileggi      atomic.Bool // traduzione riaccesa: il testo di adesso va rimostrato
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
		fg, _, _ := pGetForeground.Call()
		if eNostro(exeDiFinestra(fg)) {
			continue
		}
		// davanti c'è un altro programma (non il gioco della zona): stessa regola della finestrella
		if !ov.giocoDavanti(fg) {
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

// assicuraOCR: il lettore di Windows pronto e nella lingua del gioco di adesso (se la lingua
// è cambiata lo riapro). Va chiamata con occupato preso.
func assicuraOCR() error {
	if da, _ := coppiaAttiva(); ocr != nil {
		if tag, _ := ocrPerLingua(da); tag != ocr.lingua {
			ocr.Chiudi() // la lingua del gioco è cambiata: riparto con il lettore giusto
			ocr = nil
		}
	}
	if ocr == nil {
		l, err := avviaOCR(filepath.Join(dirApp, "lavoro"))
		if err != nil {
			return err
		}
		ocr = l
		impostaOcrErr("")
	}
	return nil
}

func traduciOra(forza bool) {
	defer guard("traduci-ora")
	i := leggiImp()
	if !i.Zona.Valida() {
		if paginaAttiva.Load() {
			// la schermata si sta già traducendo: "ritraduci" la rilegge, non la spegne
			paginaRileggi.Store(true)
			return
		}
		// nessuna zona scelta: traduco tutto lo schermo del gioco (quello dove sta la sua finestra)
		attivaPaginaCome(true) // a richiesta: anche fuori da un gioco, una volta sola
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
	if err := assicuraOCR(); err != nil {
		e := "La lettura del testo di Windows non parte: " + err.Error()
		impostaOcrErr(e)
		emetti("info", map[string]string{"testo": e})
		return
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
	// Sottotitoli: lettere chiare col bordo scuro sopra un video che si muove; la lettura "solo
	// scritte chiare" è quasi sempre la migliore (prova del 24/11), quindi la faccio sempre
	sub := leggiImp().Posizione == "sottotitoli"
	if stranezza(grezzo) > 0.02 || sub {
		var buf2 bytes.Buffer
		if preparaLuminosoPerOCR(img, &buf2) == nil {
			if g2, err2 := ocr.Leggi(buf2.Bytes()); err2 == nil && (stranezza(g2) < stranezza(grezzo) || (sub && stranezza(g2) == stranezza(grezzo))) {
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
	if sub {
		pulito = righeSottotitolo(pulito)
		grezzo = righeSottotitolo(grezzo)
	}
	ultimoGrezzo = grezzo
	if rileggi.Swap(false) {
		ultimoTesto, candidatoSub = "", ""
	}
	if pulito == "" {
		candidatoSub = ""
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
	if !forza && sub {
		if time.Since(erroreSub) < 5*time.Second {
			return // il traduttore ha appena dato errore: riprovo tra poco, non due volte al secondo
		}
		var ok bool
		if ok, candidatoSub = sottotitoloPronto(pulito, ultimoTesto, candidatoSub); !ok {
			return
		}
	}
	if !acceso.Load() || paginaAttiva.Load() {
		return // spenta mentre leggevo: niente "…" rimasto sullo schermo
	}
	ultimoTesto = pulito
	z := leggiImp().Zona
	ov.Mostra(z, "…", pulito, "")
	var r Risultato
	if sembraMenu(grezzo) {
		r = traduciZonaMenu(img, grezzo)
	} else {
		r = traduttore.Traduci(context.Background(), grezzo)
	}
	if !acceso.Load() || paginaAttiva.Load() {
		ultimoTesto = "" // tornando alla traduzione normale questo dialogo si rimostra
		if !acceso.Load() {
			ov.Nascondi() // via anche il "…" (la schermata intera invece non la tocco)
		}
		return
	}
	if r.Errore != "" {
		ultimoTesto = "" // alla prossima lettura ci riprovo (il traduttore può essere ancora in partenza)
		if sub {
			erroreSub = time.Now()
		}
		ov.Mostra(z, testoTasso("Non riesco a tradurre adesso.", "I can't translate right now."), pulito, r.Errore)
		emetti("info", map[string]string{"testo": "Traduzione non riuscita: " + r.Errore})
		return
	}
	testo := r.Tradotto
	if r.Parlante != "" {
		testo = r.Parlante + ": " + testo
	}
	auto := nomeTasto("auto") + testoTasso(" automatico: no", " automatic: off")
	if leggiImp().Auto {
		auto = testoTasso("● Automatico", "● Automatic")
	}
	statoRiga := fmt.Sprintf(testoTasso("%s   ·   %s   ·   %.1f s   ·   %s ritraduci   ·   %s inglese", "%s   ·   %s   ·   %.1f s   ·   %s translate again   ·   %s English"),
		auto, nomeMotore(r.Motore), float64(msOCR+r.Millisec)/1000, nomeTasto("traduci"), nomeTasto("inglese"))
	ov.Mostra(z, testo, r.Originale, statoRiga)
	muDialogo.Lock()
	dialogoMostrato = mostratoDialogo{z, r.Parlante, r.Tradotto, r.Originale, statoRiga}
	muDialogo.Unlock()
	r.Millisec += msOCR // tempo totale: lettura + traduzione
	aggiungiCronologia(r, false)
	emetti("riga", r)
}

// traduciZonaMenu: una zona con più voci (menu, impostazioni): rileggo le scritte con la loro
// posizione e traduco ognuna da sola, una riga per riga di gioco (nome e valore con " — ").
// Se la lettura con le posizioni non riesce, traduco il testo intero come prima.
func traduciZonaMenu(img *image.RGBA, grezzo string) Risultato {
	t0 := time.Now()
	var buf bytes.Buffer
	f, err := preparaPaginaPerOCR(img, &buf)
	if err != nil {
		return traduttore.Traduci(context.Background(), grezzo)
	}
	righeOCR, err := ocr.LeggiPosizioni(buf.Bytes())
	if err != nil {
		return traduttore.Traduci(context.Background(), grezzo)
	}
	blocchi := segmenti(righeOCR, f)
	for i := range blocchi {
		blocchi[i].Testo = correggiParole(separaAttaccate(blocchi[i].Testo))
	}
	blocchi = unisciParagrafi(blocchi)
	if len(blocchi) < 2 {
		return traduttore.Traduci(context.Background(), grezzo)
	}
	ctx := conLotto(context.Background())
	tradotti := make([]Risultato, len(blocchi))
	var wg sync.WaitGroup
	for i := range blocchi {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer guard("zona-voce")
			tradotti[i] = traduttore.TraduciScritta(ctx, blocchi[i].Testo)
		}(i)
	}
	wg.Wait()
	trad := map[string]string{}
	motore := ""
	errori := 0
	primoErrore := ""
	for _, r := range tradotti {
		if r.Errore != "" || strings.TrimSpace(r.Tradotto) == "" {
			errori++
			if primoErrore == "" {
				primoErrore = r.Errore
			}
		}
	}
	if errori == len(tradotti) {
		// nessuna voce tradotta (motore spento o occupato): lo dico, invece di mostrare l'inglese
		if primoErrore == "" {
			primoErrore = "Non riesco a tradurre adesso: riprova tra poco."
		}
		return Risultato{Originale: grezzo, Errore: primoErrore, Millisec: time.Since(t0).Milliseconds()}
	}
	for i, b := range blocchi {
		t := strings.TrimSpace(tradotti[i].Tradotto)
		if t == "" {
			t = b.Testo
		} else if motore == "" {
			motore = tradotti[i].Motore
		}
		trad[fmt.Sprint(b.X, ",", b.Y)] = t
	}
	var it, en []string
	for _, riga := range righeDiBlocchi(blocchi) {
		var a, b []string
		for _, x := range riga {
			a = append(a, trad[fmt.Sprint(x.X, ",", x.Y)])
			b = append(b, x.Testo)
		}
		it = append(it, strings.Join(a, " — "))
		en = append(en, strings.Join(b, " — "))
	}
	return Risultato{Originale: strings.Join(en, "\n"), Tradotto: strings.Join(it, "\n"), Motore: motore, Millisec: time.Since(t0).Milliseconds()}
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
	// la traduzione già mostrata di una scritta dello schermo resta quella per tutta la sessione
	// (fisse.go): la correzione vale per le scritte non ancora viste
	if paginaAttiva.Load() && !fissePagina.fissata(coppiaDi(traduttore), inglese) {
		sostituisciTraduzione(inglese, bozza, corretta)
	}
	if ov.staMostrandoVoce() {
		if ov.cambiaVoce(bozza, corretta) {
			return
		}
	}
	muDialogo.Lock()
	d := dialogoMostrato
	// anche la frase inglese deve essere la stessa: "Yes." e "Yeah." hanno la stessa bozza "Sì."
	ok := d.tradotto != "" && d.tradotto == bozza && stessaFraseInglese(d.inglese, inglese)
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
			ov.Mostra(d.z, testo, d.inglese, strings.Replace(d.stato, nomeMotore("Mozilla"), testoTasso("Traduzione veloce + IA", "Fast translation + AI"), 1))
		}
	}
}

// aggiungiCronologia: una riga nella cronologia (F8), al massimo 1000. Con unica salta la
// scritta se è già tra le ultime (le schermate e i giochi chiedono spesso le stesse scritte).
func aggiungiCronologia(r Risultato, unica bool) {
	muCronologia.Lock()
	defer muCronologia.Unlock()
	if unica {
		for i := len(righe) - 1; i >= 0 && i >= len(righe)-300; i-- {
			if righe[i].Originale == r.Originale && righe[i].Tradotto == r.Tradotto {
				return
			}
		}
	}
	righe = append(righe, r)
	if len(righe) > 1000 {
		righe = righe[len(righe)-1000:]
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

// nomeUnaCopia: una sola copia di Tasso Traduce aperta (le prove ne usano un altro, così non
// chiudono la copia vera dell'utente).
var nomeUnaCopia = "Local\\TassoTraduceUnaCopia"

var cartellaWebView = "WebView2"

// accendi: l'interruttore Traduzione (dalla finestra, da F7 nel gioco o spegnendo per Ufficio).
func accendi(on bool) bool {
	if on && !avviato.Load() {
		return false // Tasso Traduce non è ancora partito (Tasso aperto dall'attività pianificata)
	}
	if on {
		assicuraMotore()
	}
	acceso.Store(on)
	if on {
		rileggi.Store(true) // il dialogo di adesso si rimostra anche se è lo stesso di prima
		sveglia()           // l'IA si prepara subito
		go preparaOCR()     // la lettura di Windows parte adesso: il primo F7 non aspetta PowerShell
		t := tastiCompleti(leggiImp().Tasti)
		if f := cambiaTasti(t); len(f) > 0 {
			emetti("info", map[string]string{"testo": "Già usati da un altro programma: " + nomiTasti(f, t) + ". Cambiali in Giochi → Impostazioni → Tasti nel gioco."})
		}
	} else {
		cambiaTasti(soloTastoAccendi()) // spento: libero i tasti, tranne quello che la riaccende dal gioco
	}
	if !on {
		_ = accendiVoci(false) // spento: niente più ascolto dell'audio
	}
	if !on { // spento: tolgo subito ogni traduzione dallo schermo
		paginaAttiva.Store(false)
		paginaGiro.Add(1)
		NascondiPagina()
		ov.Spegni()
		ovChat.Nascondi()
	}
	return on
}

// soloTastoAccendi: a traduzione spenta resta solo il tasto della schermata (F7): premuto nel
// gioco la accende, anche se il gioco non è stato aperto da Tasso. In Ufficio nessun tasto.
func soloTastoAccendi() map[string]uint32 {
	if InUfficio.Load() {
		return map[string]uint32{}
	}
	return map[string]uint32{"pagina": tastiCompleti(leggiImp().Tasti)["pagina"]}
}

// preparaOCR: accende la lettura del testo di Windows in sottofondo (PowerShell + WinRT impiegano
// qualche secondo la prima volta). Se è già occupata da una lettura non fa niente.
func preparaOCR() {
	defer guard("prepara-ocr")
	if !occupato.CompareAndSwap(false, true) {
		return
	}
	defer occupato.Store(false)
	if err := assicuraOCR(); err != nil {
		impostaOcrErr(err.Error())
	}
}
