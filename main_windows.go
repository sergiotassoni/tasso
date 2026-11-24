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
const Versione = "1.0.0"

//go:embed ui/index.html
var paginaHTML string

//go:embed ui/caratteri.css
var caratteriCSS string

var (
	finestra webview2.WebView
	chiusa   atomic.Bool    // la finestra è chiusa: niente più messaggi alla pagina
	lavori   sync.WaitGroup // lavori in corso (per aspettarli alla chiusura)
	cartella string         // %LOCALAPPDATA%\Tasso Zip

	lavoroMu    sync.Mutex
	lavoroStop  context.CancelFunc // non nil mentre comprime o estrae
	ultimoAgg   time.Time
	codaModo    string // "comprimi" o "estrai" se aperto dal menu del tasto destro
	codaVisti   = map[string]bool{}
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

	// Argomenti: --comprimi <file...>, --estrai <file...>, --lingua=xx, oppure file trascinati sull'icona.
	modo := ""
	var percorsi []string
	for _, a := range os.Args[1:] {
		switch {
		case a == "--comprimi" || a == "--estrai":
			modo = strings.TrimPrefix(a, "--")
		case strings.HasPrefix(a, "--lingua="):
			linguaRiga = testi.Normalizza(strings.TrimPrefix(a, "--lingua="))
		case strings.HasPrefix(a, "--"):
			// sconosciuto: lo ignoro
		default:
			if p, err := filepath.Abs(a); err == nil {
				percorsi = append(percorsi, p)
			}
		}
	}

	// Dal menu del tasto destro Windows apre un Tasso Zip per ogni file scelto: il primo
	// raccoglie tutti gli altri (che gli passano i loro file e si chiudono subito).
	if modo != "" && len(percorsi) > 0 {
		primo, chiudi := sonoIlPrimo(modo)
		if !primo {
			passaAlPrimo(modo, percorsi)
			return
		}
		defer chiudi()
		codaModo = modo
		percorsi = raccogliCoda(modo, percorsi, 450*time.Millisecond)
	}

	// Se il menu del tasto destro è attivo ma l'exe è stato spostato, aggiorno i percorsi.
	if menuAttivo() {
		if exe, err := os.Executable(); err == nil && !menuPuntaA(exe) {
			_ = attivaMenu(lingua())
		}
	}

	w := nuovaFinestra()
	if w == nil {
		// Caso raro: con la ricerca DLL solo in System32 la finestra non si apre. Riprovo con
		// la ricerca sicura standard di Windows (come fa Tasso).
		_ = windows.SetDefaultDllDirectories(windows.LOAD_LIBRARY_SEARCH_DEFAULT_DIRS)
		w = nuovaFinestra()
	}
	if w == nil {
		messaggio(testi.T(lingua(), "webview_manca"), "Tasso Zip")
		apriURL("https://go.microsoft.com/fwlink/p/?LinkId=2124703")
		return
	}
	finestra = w
	defer w.Destroy()
	webview2.PanicHook = func(m string, r interface{}, st []byte) { registraErrore("errore in %s: %v\n%s", m, r, st) }
	for _, n := range []string{"goScegliFile", "goScegliCartella", "goScegliArchivio"} {
		webview2.SuFinestra[n] = true
	}
	webview2.Subito["goAnnulla"] = true
	edge.DropHook = func(p []string) { manda("trascinati", p) }
	collega(w)

	iniziali := map[string]interface{}{"modo": modo, "percorsi": percorsi}
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

func nuovaFinestra() webview2.WebView {
	scala := 1.0
	if procGetDpiForSys.Find() == nil {
		if dpi, _, _ := procGetDpiForSys.Call(); dpi >= 96 {
			scala = float64(dpi) / 96
		}
	}
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  filepath.Join(cartella, "WebView2"),
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "Tasso Zip",
			Width:  uint(1000 * scala),
			Height: uint(720 * scala),
			IconId: 2,
			Center: true,
		},
	})
	if w != nil {
		w.SetSize(int(760*scala), int(560*scala), webview2.HintMin)
	}
	return w
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
	_ = w.Bind("goAvvio", func() map[string]interface{} {
		return map[string]interface{}{
			"lingua":   lingua(),
			"lingue":   testi.Lingue,
			"testi":    testi.Tutti(),
			"versione": Versione,
			"menu":     menuAttivo(),
			"formato":  impostazioniCopia().Formato,
			"desktop":  cartellaDesktop(),
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
			{testi.T(lingua(), "estrai") + " (zip, 7z, rar, tar, gz, bz2, xz)", "*.zip;*.7z;*.rar;*.tar;*.gz;*.tgz;*.bz2;*.tbz2;*.tbz;*.xz;*.txz;*.001;*.r00"},
			{"*.*", "*.*"},
		})
	})
	_ = w.Bind("goInfo", info)
	_ = w.Bind("goComprimi", avviaComprimi)
	_ = w.Bind("goEstrai", avviaEstrai)
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
		if menuAttivo() {
			_ = attivaMenu(l) // anche le voci del tasto destro nella lingua nuova
		}
		return "ok"
	})
	_ = w.Bind("goFormato", func(f string) string {
		if f != string(archivio.FormatoZip) && f != string(archivio.FormatoTarXz) {
			return "no"
		}
		modificaImpostazioni(func(s *impostazioni) { s.Formato = f })
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
	_ = w.Bind("goApri", func(p string) string {
		mostraInEsplora(p)
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
	if c != "annullato" && c != "password" && c != "password_sbagliata" {
		registraErrore("%s %s: %v", tipo, percorso, err)
	}
	manda("errore", map[string]interface{}{"tipo": tipo, "codice": c, "dettagli": err.Error(), "percorso": percorso})
}

func avviaComprimi(percorsi []string, formato, nome string) string {
	if len(percorsi) == 0 {
		return "vuoto"
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
			Nome:            nome,
			NomeMultiplo:    testi.T(lingua(), "nome_multiplo"),
			CartellaRiserva: cartellaDesktop(),
			Avanzamento:     avanzamento,
		})
		if err != nil {
			erroreVersoPagina("comprimi", "", err)
			return
		}
		manda("fine", map[string]interface{}{
			"tipo": "comprimi", "percorso": ris.Archivio, "file": ris.File, "originale": ris.Originale,
			"compresso": ris.Compresso, "saltati": ris.Saltati, "riserva": ris.Riserva,
		})
	}()
	return "ok"
}

func avviaEstrai(percorso, password string) string {
	ctx, _ := inizia()
	if ctx == nil {
		return "occupato"
	}
	go func() {
		defer finisci()
		defer func() {
			if r := recover(); r != nil {
				registraErrore("estrai: %v\n%s", r, debug.Stack())
				manda("errore", map[string]interface{}{"tipo": "estrai", "codice": "generico", "dettagli": fmt.Sprint(r), "percorso": percorso})
			}
		}()
		ris, err := archivio.Estrai(ctx, percorso, archivio.OpzioniEstrai{
			Password:        password,
			Avanzamento:     avanzamento,
			CartellaRiserva: cartellaDesktop(),
		})
		if err != nil {
			erroreVersoPagina("estrai", percorso, err)
			return
		}
		manda("fine", map[string]interface{}{
			"tipo": "estrai", "percorso": ris.Cartella, "file": ris.File, "rinominati": ris.Rinominati,
			"saltati": ris.Saltati, "riserva": ris.Riserva, "archivio": percorso,
		})
	}()
	return "ok"
}

// ---------------- Windows: piccoli aiuti ----------------

func messaggio(testo, titolo string) {
	t, _ := windows.UTF16PtrFromString(testo)
	c, _ := windows.UTF16PtrFromString(titolo)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x40)
}

func shellExecute(file, parametri string) {
	verbo, _ := windows.UTF16PtrFromString("open")
	f, _ := windows.UTF16PtrFromString(file)
	var p *uint16
	if parametri != "" {
		p, _ = windows.UTF16PtrFromString(parametri)
	}
	procShellExecute.Call(0, uintptr(unsafe.Pointer(verbo)), uintptr(unsafe.Pointer(f)), uintptr(unsafe.Pointer(p)), 0, 1)
}

// apriURL: solo la pagina ufficiale Microsoft di WebView2 (Tasso Zip non usa internet).
func apriURL(u string) {
	if strings.HasPrefix(u, "https://go.microsoft.com/") {
		shellExecute(u, "")
	}
}

// mostraInEsplora apre Esplora file con il file o la cartella selezionati.
// Una cartella si apre, un file si mostra selezionato nella sua cartella.
func mostraInEsplora(p string) {
	fi, err := os.Stat(p)
	if err != nil {
		return
	}
	explorer := filepath.Join(os.Getenv("SystemRoot"), "explorer.exe")
	if os.Getenv("SystemRoot") == "" {
		explorer = `C:\Windows\explorer.exe`
	}
	if fi.IsDir() {
		shellExecute(explorer, `"`+p+`"`)
		return
	}
	shellExecute(explorer, `/select,"`+p+`"`)
}

func cartellaDesktop() string {
	p, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0)
	if err != nil {
		return ""
	}
	return p
}
