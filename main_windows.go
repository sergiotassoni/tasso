// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni.
//
// Questo programma è software libero: puoi ridistribuirlo e/o modificarlo secondo i termini
// della GNU General Public License versione 3, come pubblicata dalla Free Software Foundation.
// Questo programma è distribuito nella speranza che sia utile, ma SENZA ALCUNA GARANZIA.
// Vedi il file LICENSE per i dettagli.

//go:build windows

package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"

	"tassozip/archivio"
	"tassozip/testi"
)

// Versione di Tasso Zip.
const Versione = "1.3.2"

//go:embed ui/index.html
var paginaHTML string

//go:embed ui/caratteri.css
var caratteriCSS string

// Licenze: viaggiano dentro l'exe (anche se qualcuno lo copia da solo) e si leggono dal link
// "Licenze" in basso nella finestra.
//
//go:embed LICENSE
var testoLicenza string

//go:embed LICENZE.txt
var testoLicenzeTerzi string

var (
	finestra webview2.WebView
	chiusa   atomic.Bool    // la finestra è chiusa: niente più messaggi alla pagina
	lavori   sync.WaitGroup // lavori in corso (per aspettarli alla chiusura)
	cartella string         // %LOCALAPPDATA%\Tasso Zip

	lavoroMu    sync.Mutex
	lavoroStop  context.CancelFunc // non nil mentre comprime o estrae
	ultimoAgg   time.Time
	codaModo    string                   // il modo, se aperto dal menu del tasto destro (vedi modiRiga)
	codaVisti   = map[string]time.Time{} // percorso (minuscolo) -> quando è arrivato
	codaVistiMu sync.Mutex
)

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	procMessageBoxW  = user32.NewProc("MessageBoxW")
	procGetDpiForSys = user32.NewProc("GetDpiForSystem")
	shell32          = windows.NewLazySystemDLL("shell32.dll")
	procShellExecute = shell32.NewProc("ShellExecuteW")
)

func main() {
	// Le DLL di sistema si cercano solo in System32 (niente DLL false accanto all'exe).
	_ = windows.SetDefaultDllDirectories(windows.LOAD_LIBRARY_SEARCH_SYSTEM32)

	cartella = filepath.Join(os.Getenv("LOCALAPPDATA"), "Tasso Zip")
	if os.Getenv("LOCALAPPDATA") == "" {
		cartella = filepath.Join(os.TempDir(), "Tasso Zip")
	}
	_ = os.MkdirAll(cartella, 0o755)
	defer func() {
		if r := recover(); r != nil {
			registraErrore("errore grave: %v\n%s", r, debug.Stack())
		}
	}()
	caricaImpostazioni()

	// Argomenti: --comprimi, --zip, --7z, --estrai, --estrai-qui, --estrai-cartella, --apri
	// seguiti dai file; --lingua=xx; oppure file trascinati sull'icona.
	modo := ""
	var percorsi []string
	for _, a := range os.Args[1:] {
		switch {
		case modiRiga[strings.TrimPrefix(a, "--")] && strings.HasPrefix(a, "--"):
			modo = strings.TrimPrefix(a, "--")
		case strings.HasPrefix(a, "--lingua="):
			linguaRiga = testi.Normalizza(strings.TrimPrefix(a, "--lingua="))
		case strings.HasPrefix(a, "--"):
			// sconosciuto: lo ignoro
		default:
			if p, err := filepath.Abs(nomeLungo(a)); err == nil {
				percorsi = append(percorsi, p)
			}
		}
	}

	// Un archivio solo, senza altro (doppio clic, "Apri con", trascinato sull'icona): la vista.
	if modo == "" && len(percorsi) == 1 && archivio.SembraArchivio(percorsi[0]) {
		if fi, err := os.Stat(percorsi[0]); err == nil && !fi.IsDir() {
			modo = "apri"
		}
	}
	if modo == "apri" && len(percorsi) > 1 {
		percorsi = percorsi[:1]
	}
	go pulisciTemp()
	go pulisciAvanziAvvio(percorsi)

	// Dal menu del tasto destro Windows apre un Tasso Zip per ogni file scelto: il primo
	// raccoglie tutti gli altri (che gli passano i loro file e si chiudono subito).
	// "Apri" no: ogni archivio ha la sua finestra.
	if modo != "" && modo != "apri" && len(percorsi) > 0 {
		primo, chiudi := sonoIlPrimo(modo)
		if !primo {
			passaAlPrimo(modo, percorsi)
			return
		}
		defer chiudi()
		codaModo = modo
		percorsi = raccogliCoda(modo, percorsi, 700*time.Millisecond, 5*time.Second)
	}

	// Menu del tasto destro e doppio clic già accesi: li riscrivo da solo se sono di una versione
	// di prima, se la lingua è cambiata o se l'exe a cui puntano non c'è più. Mai verso una copia
	// aperta da TEMP o da Download quando c'è quella installata (vedi exePerVoci).
	aggiornaVociRegistrate()

	mini := modiMini[modo] && len(percorsi) > 0
	w := nuovaFinestra(mini)
	if w == nil {
		// Caso raro: con la ricerca DLL solo in System32 la finestra non si apre. Riprovo con
		// la ricerca sicura standard di Windows (come fa Tasso).
		_ = windows.SetDefaultDllDirectories(windows.LOAD_LIBRARY_SEARCH_DEFAULT_DIRS)
		w = nuovaFinestra(mini)
	}
	if w == nil {
		messaggio(testi.T(lingua(), "webview_manca"), "Tasso Zip")
		apriURL("https://go.microsoft.com/fwlink/p/?LinkId=2124703")
		return
	}
	finestra = w
	defer w.Destroy()
	webview2.PanicHook = func(m string, r interface{}, st []byte) { registraErrore("errore in %s: %v\n%s", m, r, st) }
	for _, n := range []string{"goScegliFile", "goScegliCartella", "goScegliArchivio", "goScegliDestinazione", "goCestino"} {
		webview2.SuFinestra[n] = true
	}
	webview2.Subito["goAnnulla"] = true
	webview2.Subito["goFermaTrova"] = true
	edge.DropHook = func(p []string) { manda("trascinati", p) }
	collega(w)
	collegaEsplora()

	iniziali := map[string]interface{}{"modo": modo, "percorsi": percorsi, "mini": mini}
	if modo == "" && len(percorsi) > 0 {
		iniziali["modo"] = "auto"
	}
	b, _ := json.Marshal(iniziali)
	w.Init("window.__TZ_INIZIO__ = " + string(b) + ";")
	w.SetHtml(strings.Replace(paginaHTML, "/*CARATTERI*/", caratteriCSS, 1))
	if codaModo != "" {
		go ascoltaCoda(codaModo)
	}
	w.Run()
	chiusa.Store(true)
	defer pulisciTempAllaChiusura()

	lavoroMu.Lock()
	if lavoroStop != nil {
		lavoroStop() // finestra chiusa durante un lavoro: annullo (niente resta a metà)
	}
	lavoroMu.Unlock()
	// Aspetto che il lavoro annullato abbia tolto i suoi file temporanei (al massimo 20 secondi).
	fatto := make(chan struct{})
	go func() { lavori.Wait(); close(fatto) }()
	select {
	case <-fatto:
	case <-time.After(20 * time.Second):
	}
}

// modiRiga: i modi che si possono chiedere dalla riga di comando (e dal tasto destro).
var modiRiga = map[string]bool{
	"comprimi": true, "zip": true, "7z": true,
	"estrai": true, "estrai-qui": true, "estrai-cartella": true, "estrai-in": true, "apri": true,
}

// modiMini: lavori diretti dal tasto destro, con la finestrella di avanzamento.
var modiMini = map[string]bool{"zip": true, "7z": true, "estrai-qui": true, "estrai-cartella": true, "estrai-in": true}

func nuovaFinestra(mini bool) webview2.WebView {
	scala := 1.0
	if procGetDpiForSys.Find() == nil {
		if dpi, _, _ := procGetDpiForSys.Call(); dpi >= 96 {
			scala = float64(dpi) / 96
		}
	}
	larg, alt, minL, minA := 1000.0, 720.0, 760.0, 560.0
	if mini {
		larg, alt, minL, minA = 560, 400, 460, 340
	}
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  filepath.Join(cartella, "WebView2"),
		AutoFocus: true,
		// La pagina di Tasso Zip non va mai altrove (un link trascinato nella finestra non apre
		// quel sito al posto della pagina), niente finestre nuove, e le funzioni del programma
		// rispondono solo alla pagina interna.
		NavigazionePermessa: navigazionePermessa,
		PopupRichiesto:      func(string) {},
		FontePermessa:       fontePermessa,
		WindowOptions: webview2.WindowOptions{
			Title:  "Tasso Zip",
			Width:  uint(larg * scala),
			Height: uint(alt * scala),
			IconId: 2,
			Center: true,
		},
	})
	if w != nil {
		w.SetSize(int(minL*scala), int(minA*scala), webview2.HintMin)
	}
	return w
}

var (
	paginaMu    sync.Mutex
	paginaPrima string // l'indirizzo della pagina interna (la prima navigazione, quella di SetHtml)
	paginaVista bool
)

// esterno: indirizzi di siti o di file, mai permessi alla finestra.
func esterno(u string) bool {
	l := strings.ToLower(strings.TrimSpace(u))
	for _, s := range []string{"http:", "https:", "file:", "ftp:", "ws:", "wss:", "javascript:", "blob:", "filesystem:", "\\\\", "//"} {
		if strings.HasPrefix(l, s) {
			return true
		}
	}
	return false
}

// navigazionePermessa: solo la pagina interna. La prima navigazione è quella di SetHtml (la
// pagina di Tasso Zip): la segno; dopo, solo lei (per esempio se si ricarica). Ogni altra
// (un link trascinato, un indirizzo scritto da una pagina) viene annullata.
func navigazionePermessa(u string) bool {
	if esterno(u) {
		return false
	}
	paginaMu.Lock()
	defer paginaMu.Unlock()
	if !paginaVista {
		paginaVista, paginaPrima = true, u
		return true
	}
	return u == paginaPrima
}

// fontePermessa: i messaggi per le funzioni del programma arrivano solo dalla pagina interna
// (scritta con SetHtml: WebView2 la indica come "about:blank" o "data:text/html…"). Mai da un
// sito o da un file; altre pagine non si aprono comunque (navigazionePermessa).
func fontePermessa(u string) bool {
	if esterno(u) {
		return false
	}
	paginaMu.Lock()
	defer paginaMu.Unlock()
	return u == "" || u == "about:blank" || strings.HasPrefix(u, "data:text/html") || paginaVista && u == paginaPrima
}

// manda un evento alla pagina: window.tz.<nome>(dati).
func manda(nome string, dati interface{}) {
	w := finestra
	if w == nil || chiusa.Load() {
		return
	}
	b, err := json.Marshal(dati)
	if err != nil {
		return
	}
	js := "window.tz && tz." + nome + " && tz." + nome + "(" + string(b) + ")"
	w.Dispatch(func() { w.Eval(js) })
}

func collega(w webview2.WebView) {
	collegaAggiornamenti(w) // aggiornamenti_windows.go
	collegaNome(w)          // nome_windows.go: "Il tuo nome", lo stesso di Tasso e Tasso PDF
	_ = w.Bind("goAvvio", func() map[string]interface{} {
		return map[string]interface{}{
			"lingua":   lingua(),
			"lingue":   testi.Lingue,
			"testi":    testi.Tutti(),
			"versione": Versione,
			"menu":     menuAttivo(),
			"assoc":    associazioniAttive(),
			"formato":  impostazioniCopia().Formato,
			"livello":  impostazioniCopia().Livello,
			"desktop":  cartellaDesktop(),
			"formati":  archivio.FormatiCreabili,
			"nome":     leggiNome(), // "Il tuo nome" ("" = senza nome)
		}
	})
	_ = w.Bind("goScegliFile", func() []string {
		return scegli(testi.T(lingua(), "scegli_file"), false, true, nil)
	})
	_ = w.Bind("goScegliCartella", func() []string {
		return scegli(testi.T(lingua(), "scegli_cartella"), true, true, nil)
	})
	_ = w.Bind("goScegliArchivio", func() []string {
		return scegli(testi.T(lingua(), "scegli_archivio"), false, true, []filtro{
			{testi.T(lingua(), "estrai") + " (zip, 7z, rar, tar, gz, bz2, xz, zst)", "*.zip;*.7z;*.rar;*.tar;*.gz;*.tgz;*.bz2;*.tbz2;*.tbz;*.xz;*.txz;*.zst;*.tzst;*.001;*.r00"},
			{"*.*", "*.*"},
		})
	})
	_ = w.Bind("goInfo", info)
	_ = w.Bind("goComprimi", avviaComprimi)
	_ = w.Bind("goEstrai", func(percorso, password, modo string) string {
		return avviaEstrai(percorso, archivio.OpzioniEstrai{Password: password, Modo: modoEstrai(modo)}, "estrai")
	})
	_ = w.Bind("goEstraiSelezione", func(percorso, password string, sel []string, prefisso string) string {
		return avviaEstrai(percorso, archivio.OpzioniEstrai{Password: password, Selezione: sel, TogliPrefisso: prefisso}, "estrai")
	})
	_ = w.Bind("goElenca", avviaElenca)
	_ = w.Bind("goApriVoce", avviaApriVoce)
	_ = w.Bind("goChiudi", func() string {
		finestra.Dispatch(func() { finestra.Terminate() })
		return "ok"
	})
	_ = w.Bind("goAssocia", func(on bool) map[string]interface{} {
		// "altre": accendendo, le estensioni che il doppio clic non apre con Tasso Zip;
		// spegnendo, quelle per cui l'utente deve scegliere di nuovo il programma di prima.
		var altre []string
		var err error
		if on {
			altre, err = attivaAssociazioni(lingua())
		} else {
			altre, err = togliAssociazioni()
		}
		if err != nil {
			registraErrore("associazioni: %v", err)
		}
		if altre == nil {
			altre = []string{}
		}
		return map[string]interface{}{"ok": err == nil, "on": associazioniAttive(), "altre": altre}
	})
	_ = w.Bind("goAnnulla", func() string {
		lavoroMu.Lock()
		defer lavoroMu.Unlock()
		if lavoroStop != nil {
			lavoroStop()
		}
		return "ok"
	})
	_ = w.Bind("goLingua", func(l string) string {
		l = testi.Normalizza(l)
		if l == "" {
			return "no"
		}
		linguaRiga = ""
		modificaImpostazioni(func(s *impostazioni) { s.Lingua = l })
		// anche le voci del tasto destro nella lingua nuova (verso lo stesso exe di prima: vedi exePerVoci)
		if menuAttivo() {
			if exe := exePerVoci(leggiComando()); exe != "" {
				_ = attivaMenuCon(exe, l)
			}
		}
		if associazioniAttive() {
			if exe := exePerVoci(leggiComandoApri()); exe != "" {
				_, _ = attivaAssociazioniCon(exe, l)
			}
		}
		return "ok"
	})
	_ = w.Bind("goFormato", func(f string) string {
		if !archivio.FormatoValido(f) {
			return "no"
		}
		modificaImpostazioni(func(s *impostazioni) { s.Formato = f })
		return "ok"
	})
	_ = w.Bind("goLivello", func(l string) string {
		if !livelloValido(l) {
			return "no"
		}
		modificaImpostazioni(func(s *impostazioni) { s.Livello = l })
		return "ok"
	})
	_ = w.Bind("goMenu", func(on bool) map[string]interface{} {
		var err error
		if on {
			err = attivaMenu(lingua())
		} else {
			err = togliMenu()
		}
		if err != nil {
			registraErrore("menu tasto destro: %v", err)
		}
		return map[string]interface{}{"ok": err == nil, "on": menuAttivo()}
	})
	_ = w.Bind("goLicenze", func() string {
		p := filepath.Join(cartella, "Licenze di Tasso Zip.txt")
		testo := strings.ReplaceAll(testoLicenzeTerzi+"\n\n==== Tasso Zip: GNU GPL 3.0 ====\n\n"+testoLicenza, "\r\n", "\n")
		if err := os.WriteFile(p, []byte(strings.ReplaceAll(testo, "\n", "\r\n")), 0o644); err != nil {
			return "errore"
		}
		if shellExecute(p, "") <= 32 {
			return "errore"
		}
		return "ok"
	})
	_ = w.Bind("goApri", func(p string) string {
		if !mostraInEsplora(p) {
			return "errore" // il file non c'è più (o Esplora file non parte): la pagina lo dice
		}
		return "ok"
	})
}

// Elemento scelto, come lo mostra la pagina.
type elemento struct {
	Percorso string `json:"percorso"`
	Nome     string `json:"nome"`
	Cartella bool   `json:"cartella"`
	Dim      int64  `json:"dim"`
	Archivio bool   `json:"archivio"`
	Dove     string `json:"dove"`
}

func info(percorsi []string) []elemento {
	var out []elemento
	for _, p := range percorsi {
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		e := elemento{Percorso: p, Nome: filepath.Base(p), Cartella: fi.IsDir(), Dove: filepath.Dir(p)}
		if !fi.IsDir() {
			e.Dim = fi.Size()
			e.Archivio = archivio.SembraArchivio(p)
		}
		out = append(out, e)
	}
	return out
}

// inizia un lavoro (uno alla volta). Restituisce il contesto oppure nil se ne è già in corso uno.
func inizia() (context.Context, context.CancelFunc) {
	lavoroMu.Lock()
	defer lavoroMu.Unlock()
	if lavoroStop != nil {
		return nil, nil
	}
	ctx, stop := context.WithCancel(context.Background())
	lavoroStop = stop
	lavori.Add(1)
	return ctx, stop
}

func finisci() {
	lavoroMu.Lock()
	if lavoroStop != nil {
		lavoroStop()
	}
	lavoroStop = nil
	lavoroMu.Unlock()
	lavori.Done()
}

// avanzamento verso la pagina, al massimo 10 volte al secondo.
func avanzamento(a archivio.Avanzamento) {
	lavoroMu.Lock()
	ora := time.Now()
	if ora.Sub(ultimoAgg) < 100*time.Millisecond {
		lavoroMu.Unlock()
		return
	}
	ultimoAgg = ora
	lavoroMu.Unlock()
	manda("avanza", map[string]interface{}{"fatto": a.Fatto, "totale": a.Totale, "nome": filepath.Base(filepath.FromSlash(a.Nome))})
}

func erroreVersoPagina(tipo, percorso string, err error) {
	c := archivio.Codice(err)
	if c != "annullato" && c != "password" && c != "password_sbagliata" && c != "zip_cifrato" && c != "zip_cifrato_aes" {
		registraErrore("%s %s: %v", tipo, percorso, err)
	}
	manda("errore", map[string]interface{}{"tipo": tipo, "codice": c, "dettagli": testi.Dettagli(lingua(), err.Error()), "percorso": percorso})
}

func avviaComprimi(percorsi []string, formato, nome, livello, dove string) string {
	if len(percorsi) == 0 {
		return "vuoto"
	}
	multiplo := testi.T(lingua(), "nome_multiplo")
	if codaModo == "zip" || codaModo == "7z" {
		// Dal tasto destro con più file: l'archivio prende il nome della loro cartella.
		if d := filepath.Base(filepath.Dir(percorsi[0])); d != "" && d != "." && !strings.HasSuffix(d, `\`) && !strings.HasSuffix(d, ":") {
			multiplo = d
		}
	}
	ctx, _ := inizia()
	if ctx == nil {
		return "occupato"
	}
	go func() {
		defer finisci()
		defer func() {
			if r := recover(); r != nil {
				registraErrore("comprimi: %v\n%s", r, debug.Stack())
				manda("errore", map[string]interface{}{"tipo": "comprimi", "codice": "generico", "dettagli": fmt.Sprint(r)})
			}
		}()
		ris, err := archivio.Comprimi(ctx, percorsi, archivio.OpzioniComprimi{
			Formato:         archivio.Formato(formato),
			Livello:         archivio.Livello(livello),
			Nome:            nome,
			NomeMultiplo:    multiplo,
			Cartella:        dove,
			CartellaRiserva: cartellaDesktop(),
			Avanzamento:     avanzamento,
		})
		if err != nil {
			erroreVersoPagina("comprimi", "", err)
			return
		}
		manda("fine", map[string]interface{}{
			"tipo": "comprimi", "percorso": ris.Archivio, "file": ris.File, "originale": ris.Originale,
			"compresso": ris.Compresso, "saltati": testi.Motivi(lingua(), ris.Saltati), "riserva": ris.Riserva,
		})
	}()
	return "ok"
}

func modoEstrai(m string) archivio.ModoEstrai {
	switch m {
	case "qui":
		return archivio.ModoQui
	case "cartella":
		return archivio.ModoCartella
	}
	return archivio.ModoNormale
}

func avviaEstrai(percorso string, opz archivio.OpzioniEstrai, tipo string) string {
	ctx, _ := inizia()
	if ctx == nil {
		return "occupato"
	}
	opz.Avanzamento = avanzamento
	opz.CartellaRiserva = cartellaDesktop()
	go func() {
		defer finisci()
		defer func() {
			if r := recover(); r != nil {
				registraErrore("estrai: %v\n%s", r, debug.Stack())
				manda("errore", map[string]interface{}{"tipo": tipo, "codice": "generico", "dettagli": fmt.Sprint(r), "percorso": percorso})
			}
		}()
		ris, err := archivio.Estrai(ctx, percorso, opz)
		if err != nil {
			erroreVersoPagina(tipo, percorso, err)
			return
		}
		manda("fine", map[string]interface{}{
			"tipo": tipo, "percorso": ris.Cartella, "file": ris.File, "rinominati": ris.Rinominati,
			"saltati": testi.Motivi(lingua(), ris.Saltati), "falliti": testi.Motivi(lingua(), ris.Falliti), "sommaSbagliata": ris.SommaSbagliata,
			"riserva": ris.Riserva, "archivio": percorso, "qui": opz.Modo == archivio.ModoQui,
			"nonEstratti": testi.ConMotivo(ris.Saltati, "con_password", "metodo"),
		})
	}()
	return "ok"
}

// avviaElenca legge l'elenco dell'archivio (per i tar compressi può volerci un po').
func avviaElenca(percorso, password string) string {
	ctx, _ := inizia()
	if ctx == nil {
		return "occupato"
	}
	go func() {
		defer finisci()
		defer func() {
			if r := recover(); r != nil {
				registraErrore("elenca: %v\n%s", r, debug.Stack())
				manda("errore", map[string]interface{}{"tipo": "elenco", "codice": "generico", "dettagli": fmt.Sprint(r), "percorso": percorso})
			}
		}()
		el, err := archivio.Elenca(ctx, percorso, password)
		if err != nil {
			erroreVersoPagina("elenco", percorso, err)
			return
		}
		manda("elenco", map[string]interface{}{"percorso": percorso, "elenco": el})
	}()
	return "ok"
}

// estensioniPericolose: programmi e script. Prima di aprirli con un doppio clic chiedo.
var estensioniPericolose = map[string]bool{
	".exe": true, ".com": true, ".bat": true, ".cmd": true, ".ps1": true, ".psm1": true, ".psd1": true,
	".vbs": true, ".vbe": true, ".js": true, ".jse": true, ".wsf": true, ".wsh": true, ".hta": true,
	".msi": true, ".msp": true, ".msix": true, ".msixbundle": true, ".appx": true, ".appxbundle": true,
	".appinstaller": true, ".application": true, ".lnk": true, ".url": true, ".scr": true, ".pif": true,
	".cpl": true, ".reg": true, ".inf": true, ".jar": true, ".chm": true, ".gadget": true, ".dll": true,
	".sys": true, ".scf": true, ".settingcontent-ms": true, ".library-ms": true, ".iso": true, ".img": true, ".vhd": true, ".vhdx": true,
	".msc": true, ".appref-ms": true, ".xll": true, ".wsc": true, ".sct": true, ".diagcab": true, ".search-ms": true,
	".ps1xml": true, ".mht": true, ".mhtml": true,
	// usati negli attacchi più recenti (OneNote, desktop remoto, pacchetti, temi, collegamenti web)
	".one": true, ".onepkg": true, ".rdp": true, ".cab": true, ".msu": true, ".wsb": true, ".xbap": true,
	".ocx": true, ".vb": true, ".ws": true, ".shb": true, ".theme": true, ".themepack": true,
	".website": true, ".mde": true, ".accde": true,
	// RECENSIONE-3b, R3b-2: script Python (partono senza avviso se Python c'è), Java Web Start,
	// connettori di ricerca, certificati (procedura di importazione), Monad/PowerShell 2, query
	// web di Excel, Access, guida e porte delle stampanti (elenco degli allegati bloccati di Outlook)
	".py": true, ".pyw": true, ".pyz": true, ".pyzw": true, ".pyc": true, ".pyo": true,
	".jnlp": true, ".searchconnector-ms": true, ".appcontent-ms": true, ".cer": true, ".crt": true, ".der": true,
	".mst": true, ".msh": true, ".msh1": true, ".msh2": true, ".mshxml": true, ".msh1xml": true, ".msh2xml": true,
	".ps2": true, ".ps2xml": true, ".psc1": true, ".psc2": true,
	".iqy": true, ".slk": true, ".mdb": true, ".accdb": true, ".ade": true, ".adp": true,
	".its": true, ".printerexport": true,
}

// assocPericolosa: Windows stesso considera pericoloso questo tipo di file (AssocIsDangerous di
// shlwapi: l'elenco di Windows e i tipi segnati FTA_AlwaysUnsafe). È una variabile per i test.
var assocPericolosa = func(est string) bool {
	if est == "" || est == "." || procAssocIsDangerous.Find() != nil {
		return false
	}
	p, err := windows.UTF16PtrFromString(est)
	if err != nil {
		return false
	}
	r, _, _ := procAssocIsDangerous.Call(uintptr(unsafe.Pointer(p)))
	return r != 0
}

var procAssocIsDangerous = windows.NewLazySystemDLL("shlwapi.dll").NewProc("AssocIsDangerous")

// pericoloso: il file (l'ultimo pezzo del nome) è un programma o uno script? Il nome si guarda
// dopo la STESSA pulizia dell'estrazione (archivio.PulisciNome): "fattura.exe" seguito da uno
// spazio speciale (U+00A0, U+3000…) sul disco diventa "fattura.exe", e va confermato.
func pericoloso(nome string) bool {
	n := strings.ReplaceAll(nome, `\`, "/")
	if i := strings.LastIndex(n, "/"); i >= 0 {
		n = n[i+1:]
	}
	if p := archivio.PulisciNome(n); p != "" {
		n = p
	}
	est := strings.ToLower(filepath.Ext(strings.TrimRight(n, ". ")))
	return estensioniPericolose[est] || assocPericolosa(est)
}

// avviaApriVoce estrae una voce in una cartella temporanea di Tasso Zip e la apre con il
// programma predefinito. Per programmi e script serve la conferma (confermato=true).
func avviaApriVoce(percorso, password, nome string, confermato bool) string {
	if pericoloso(nome) && !confermato {
		return "conferma"
	}
	ctx, _ := inizia()
	if ctx == nil {
		return "occupato"
	}
	go func() {
		defer finisci()
		defer func() {
			if r := recover(); r != nil {
				registraErrore("apri voce: %v\n%s", r, debug.Stack())
				manda("errore", map[string]interface{}{"tipo": "apri", "codice": "generico", "dettagli": fmt.Sprint(r), "percorso": percorso})
			}
		}()
		base := filepath.Join(cartella, "Temp")
		_ = os.MkdirAll(base, 0o755)
		dir, err := os.MkdirTemp(base, "apri-")
		if err != nil {
			erroreVersoPagina("apri", percorso, err)
			return
		}
		apertiMu.Lock()
		aperti = append(aperti, dir)
		apertiMu.Unlock()
		prefisso := ""
		if i := strings.LastIndex(nome, "/"); i > 0 {
			prefisso = nome[:i]
		}
		// Solo la voce esatta: non quello che c'è in una cartella con lo stesso nome
		// ("fattura.pdf" file e "fattura.pdf/avvia.exe").
		ris, err := archivio.Estrai(ctx, percorso, archivio.OpzioniEstrai{
			Password: password, Selezione: []string{nome}, TogliPrefisso: prefisso, Esatto: true,
			Modo: archivio.ModoQui, Destinazione: dir, Avanzamento: avanzamento,
		})
		if err == nil && ris.UltimoFile == "" {
			err = archivio.ErrRovinato
		}
		if err != nil {
			_ = os.RemoveAll(dir)
			erroreVersoPagina("apri", percorso, err)
			return
		}
		// Prima di aprire ricontrollo il file vero: deve essere proprio la voce scelta (un file
		// solo, con il suo nome pulito) e, se è un programma, ci vuole la conferma.
		file := ris.UltimoFile
		atteso := archivio.PulisciNome(nome[strings.LastIndex(nome, "/")+1:])
		if ris.File != 1 || filepath.Base(file) != atteso {
			_ = os.RemoveAll(dir)
			registraErrore("apri voce %s: uscito %q (%d file) invece di %q", nome, filepath.Base(file), ris.File, atteso)
			manda("errore", map[string]interface{}{"tipo": "apri", "codice": "generico", "percorso": percorso,
				"dettagli": testi.Dettagli(lingua(), "voce diversa da quella scelta")})
			return
		}
		if pericoloso(file) && !confermato {
			_ = os.RemoveAll(dir)
			manda("confermaApri", map[string]interface{}{"voce": nome, "nome": filepath.Base(file)})
			return
		}
		// Sola lettura: Word, Excel e gli altri non salvano sopra la copia temporanea (che poi
		// si cancella) ma chiedono "Salva con nome". La pagina dice che è una copia.
		_ = os.Chmod(file, 0o444)
		// ShellExecute nel filo della finestra (COM già pronto lì, come vuole Windows).
		if w := finestra; w != nil && !chiusa.Load() {
			w.Dispatch(func() { apriFile(file) })
		}
		manda("aperto", map[string]interface{}{"nome": nome, "percorso": ris.UltimoFile})
	}()
	return "ok"
}

// pulisciAvanziAvvio: cartelle e file di lavoro rimasti da lavori interrotti di colpo (PC
// spento, programma chiuso a forza): sul Desktop (cartella di riserva) e accanto ai file
// passati al programma.
func pulisciAvanziAvvio(percorsi []string) {
	visti := map[string]bool{}
	for _, d := range append([]string{cartellaDesktop()}, percorsi...) {
		if d == "" {
			continue
		}
		if fi, err := os.Stat(d); err == nil && !fi.IsDir() {
			d = filepath.Dir(d)
		}
		if k := strings.ToLower(d); !visti[k] {
			visti[k] = true
			archivio.PulisciAvanzi(d)
		}
	}
}

// pulisciTempAllaChiusura: chiudendo la finestra tolgo le copie dei file aperti dagli archivi
// (anche da quelli con password: non restano in chiaro sul disco). Windows non lascia cancellare
// quelle ancora aperte in un programma: restano, e vanno via al prossimo avvio dopo un giorno.
// Solo le cartelle create da questa finestra: quelle di un'altra finestra di Tasso Zip ancora
// aperta non si toccano.
func pulisciTempAllaChiusura() {
	apertiMu.Lock()
	dirs := aperti
	aperti = nil
	apertiMu.Unlock()
	for _, d := range dirs {
		_ = os.RemoveAll(d) // se il file è ancora aperto resta (con la sua cartella)
	}
}

var (
	apertiMu sync.Mutex
	aperti   []string // cartelle apri-… create da questa finestra
)

// pulisciTemp toglie i file aperti dagli archivi più di un giorno fa.
func pulisciTemp() {
	base := filepath.Join(cartella, "Temp")
	voci, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, v := range voci {
		if !v.IsDir() || !strings.HasPrefix(v.Name(), "apri-") {
			continue
		}
		if fi, err := v.Info(); err == nil && time.Since(fi.ModTime()) > 24*time.Hour {
			_ = os.RemoveAll(filepath.Join(base, v.Name()))
		}
	}
}

// ---------------- Windows: piccoli aiuti ----------------

func messaggio(testo, titolo string) {
	t, _ := windows.UTF16PtrFromString(testo)
	c, _ := windows.UTF16PtrFromString(titolo)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x40)
}

func shellExecute(file, parametri string) uintptr {
	return shellExecuteVerbo("open", file, parametri)
}

func shellExecuteVerbo(verbo, file, parametri string) uintptr {
	var v *uint16
	if verbo != "" {
		v, _ = windows.UTF16PtrFromString(verbo)
	}
	f, _ := windows.UTF16PtrFromString(file)
	var p *uint16
	if parametri != "" {
		p, _ = windows.UTF16PtrFromString(parametri)
	}
	r, _, _ := procShellExecute.Call(0, uintptr(unsafe.Pointer(v)), uintptr(unsafe.Pointer(f)), uintptr(unsafe.Pointer(p)), 0, 1)
	return r
}

// apriFile apre un file con il suo programma predefinito; se non ce n'è uno, Windows
// mostra "Come vuoi aprire questo file?".
func apriFile(p string) {
	if r := shellExecuteVerbo("", p, ""); r <= 32 {
		shellExecuteVerbo("openas", p, "")
	}
}

// apriURL: solo la pagina ufficiale Microsoft di WebView2, quando manca. Tasso Zip non usa
// internet: l'unica altra pagina che apre nel browser è quella delle versioni di Tasso
// (goPaginaVersioni in aggiornamenti_windows.go), e solo dopo che l'utente ha detto sì.
func apriURL(u string) {
	if strings.HasPrefix(u, "https://go.microsoft.com/") {
		shellExecute(u, "")
	}
}

// mostraInEsplora apre Esplora file con il file o la cartella selezionati.
// Una cartella si apre, un file si mostra selezionato nella sua cartella.
// Risponde false se il file non c'è più o se Esplora file non è partito.
func mostraInEsplora(p string) bool {
	fi, err := os.Stat(p)
	if err != nil {
		return false
	}
	explorer := filepath.Join(os.Getenv("SystemRoot"), "explorer.exe")
	if os.Getenv("SystemRoot") == "" {
		explorer = `C:\Windows\explorer.exe`
	}
	if fi.IsDir() {
		return shellExecute(explorer, `"`+p+`"`) > 32
	}
	return shellExecute(explorer, `/select,"`+p+`"`) > 32
}

// nomeLungo: con percorsi lunghi Esplora file può passare il nome corto 8.3 ("FOTOGR~1.JPG"):
// lo trasformo nel nome vero, così l'archivio e l'elenco hanno i nomi giusti.
func nomeLungo(p string) string {
	if !strings.Contains(p, "~") {
		return p
	}
	u, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return p
	}
	buf := make([]uint16, 1024)
	for i := 0; i < 2; i++ {
		n, err := windows.GetLongPathName(u, &buf[0], uint32(len(buf)))
		if err != nil || n == 0 {
			return p
		}
		if int(n) < len(buf) {
			return windows.UTF16ToString(buf[:n])
		}
		buf = make([]uint16, n+1)
	}
	return p
}

func cartellaDesktop() string {
	p, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0)
	if err != nil {
		return ""
	}
	return p
}
