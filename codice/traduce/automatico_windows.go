//go:build windows

package traduce

// Modalità "tutto automatico": nessun tasto. Quando in primo piano c'è un gioco (a schermo
// intero / senza bordi, oppure un gioco già usato con Tasso Traduce), il programma traduce da
// solo tutte le scritte della sua finestra, al loro posto (come F6). Se esci dal gioco si ferma,
// quando torni riparte. F6 dentro il gioco mette in pausa / riprende per quel gioco.

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"
)

var (
	pMonitorFromWindow = user32.NewProc("MonitorFromWindow")
	pGetMonitorInfo    = user32.NewProc("GetMonitorInfoW")
	pGetWindowRect     = user32.NewProc("GetWindowRect")

	muPausa       sync.Mutex
	pausaPerGioco = map[string]bool{} // solo in questa sessione
)

func inPausa(exe string) bool {
	muPausa.Lock()
	defer muPausa.Unlock()
	return pausaPerGioco[exe]
}

func impostaPausa(exe string, si bool) {
	if exe == "" {
		return
	}
	muPausa.Lock()
	defer muPausa.Unlock()
	if si {
		pausaPerGioco[exe] = true
	} else {
		delete(pausaPerGioco, exe)
	}
}

type monitorInfo struct {
	cbSize    uint32
	rcMonitor rect
	rcWork    rect
	dwFlags   uint32
}

// programmi che non sono giochi anche se a schermo intero
var nonGiochi = map[string]bool{
	"chrome.exe": true, "msedge.exe": true, "firefox.exe": true, "brave.exe": true, "opera.exe": true,
	"discord.exe": true, "code.exe": true, "notepad.exe": true, "explorer.exe": true, "teams.exe": true,
	"ms-teams.exe": true, "obs64.exe": true, "steamwebhelper.exe": true, "steam.exe": true,
	"epicgameslauncher.exe": true, "applicationframehost.exe": true, "searchhost.exe": true,
	"startmenuexperiencehost.exe": true, "shellexperiencehost.exe": true, "lockapp.exe": true,
	"taskmgr.exe": true, "winword.exe": true, "excel.exe": true, "powerpnt.exe": true, "acrobat.exe": true,
	"vlc.exe": false, // i video player vanno bene: i sottotitoli si traducono
}

// aSchermoIntero: la finestra copre tutto il suo monitor (gioco a schermo intero o senza bordi).
func aSchermoIntero(h uintptr) bool {
	var r rect
	if ok, _, _ := pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return false
	}
	mon, _, _ := pMonitorFromWindow.Call(h, 2) // MONITOR_DEFAULTTONEAREST
	mi := monitorInfo{}
	mi.cbSize = uint32(unsafe.Sizeof(mi))
	if ok, _, _ := pGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi))); ok == 0 {
		return false
	}
	m := mi.rcMonitor
	return r.Left <= m.Left && r.Top <= m.Top && r.Right >= m.Right && r.Bottom >= m.Bottom
}

func eGioco(h uintptr, exe string) bool {
	if nonGiocoMai(h, exe) {
		return false
	}
	muImp.Lock()
	_, conosciuto := imp.Profili[exe]
	muImp.Unlock()
	if conosciuto || aSchermoIntero(h) {
		return true
	}
	// installato da Steam, Epic, GOG, Xbox…: è un gioco anche se gira in finestra
	_, daNegozio := riconosciGioco(percorsoDiFinestra(h))
	return daNegozio
}

// cicloTuttoAutomatico guarda ogni secondo quale finestra è in primo piano.
var (
	avvisatoAntiCheat = map[string]bool{}
	proposto          = map[uintptr]bool{}
)

func cicloTuttoAutomatico() {
	defer guard("tutto-automatico")
	visto := map[uintptr]time.Time{}
	for {
		time.Sleep(time.Second)
		if !acceso.Load() {
			continue
		}
		fg, _, _ := pGetForeground.Call()
		for h := range visto { // il conto dei secondi davanti riparte quando il gioco va dietro
			if h != fg {
				delete(visto, h)
			}
		}
		if fg == 0 || (paginaAttiva.Load() && paginaGiocoA.Load() == fg) {
			continue
		}
		exe := exeDiFinestra(fg)
		if !eGioco(fg, exe) || inPausa(exe) {
			continue
		}
		if g, ok := riconosciGioco(percorsoDiFinestra(fg)); ok && ConAntiCheat(g) && !avvisatoAntiCheat[exe] {
			// la traduzione dello schermo non tocca il gioco: va bene anche con l'anti-imbroglio
			avvisatoAntiCheat[exe] = true
			emetti("info", map[string]string{"testo": messaggioAntiCheat(g.Nome)})
		}
		// aspetto 2 secondi di gioco in primo piano (non parto durante un Alt+Tab veloce)
		if t, ok := visto[fg]; !ok {
			visto[fg] = time.Now()
			continue
		} else if time.Since(t) < 2*time.Second {
			continue
		}
		if _, ok := areaFinestra(fg); !ok {
			continue
		}
		nomeG, cartG := exe, ""
		if g, ok := riconosciGioco(percorsoDiFinestra(fg)); ok {
			nomeG, cartG = g.Nome, g.Cartella
		}
		if !giocoTradotto(cartG) {
			// 3.1: giochi tradotti da Tasso fuori dalle cartelle dei negozi (Xbox, Riot, aggiunti a mano)
			if c := cartellaTradottaDi(percorsoDiFinestra(fg)); c != "" {
				cartG = c
			}
		}
		if modDaUsare(cartG, time.Since(visto[fg]), nomeG) {
			continue // tradotto dentro il gioco: la sovrimpressione non serve
		}
		if !leggiImp().TuttoAuto && !giocoTradotto(cartG) {
			// non parto da solo: propongo la traduzione, una volta per finestra
			if !proposto[fg] {
				proposto[fg] = true
				proponiTraduzione(fg, nomeG)
			}
			continue
		}
		avviaPaginaSu(fg)
		nome := exe
		if g, ok := riconosciGioco(percorsoDiFinestra(fg)); ok {
			nome = g.Nome + " (" + g.Piattaforma + ")"
		}
		emetti("info", map[string]string{"testo": "Gioco trovato: " + nome + ". Traduco tutto da solo. " + nomeTasto("pagina") + " nel gioco per mettere in pausa."})
		muImp.Lock()
		if imp.Profili == nil {
			imp.Profili = map[string]Profilo{}
		}
		if _, ok := imp.Profili[exe]; !ok {
			imp.Profili[exe] = Profilo{Posizione: imp.Posizione, DimFont: imp.DimFont}
		}
		muImp.Unlock()
		salvaImp()
	}
}

// proponiTraduzione: un avviso in alto nel gioco per 6 secondi. Non si clicca (il clic va al
// gioco): si accetta con F6.
func proponiTraduzione(fg uintptr, nome string) {
	area, ok := areaFinestra(fg)
	if !ok {
		return
	}
	tasto := etichettaTasto(tastiCompleti(leggiImp().Tasti)["pagina"])
	_, a := coppiaAttiva()
	testo := testoTasso("Tasso: ho riconosciuto "+nome+". Premi "+tasto+" per tradurlo in "+nomeLingua(a)+".",
		"Tasso: I recognised "+nome+". Press "+tasto+" to translate it.")
	mostraAvviso(area, testo, 22, 20, 6*time.Second)
	emetti("info", map[string]string{"testo": testoTasso("Ho riconosciuto "+nome+": premi "+tasto+" nel gioco per tradurlo.",
		"I recognised "+nome+": press "+tasto+" in the game to translate it.")})
}

// nomeGioco: nome vero (Steam/Epic…) se riconosciuto, altrimenti il nome dell'exe senza ".exe".
func nomeGioco(h uintptr) string {
	if g, ok := riconosciGioco(percorsoDiFinestra(h)); ok {
		return g.Nome
	}
	return strings.TrimSuffix(exeDiFinestra(h), ".exe")
}

// apriLibreriaPer: da qui in poi le traduzioni finiscono nella libreria di quel gioco.
func apriLibreriaPer(h uintptr) {
	exe := exeDiFinestra(h)
	if traduttore == nil || eNostro(exe) {
		return
	}
	p := percorsoDiFinestra(h)
	cart := cartellaTradottaDi(p)
	if g, ok := riconosciGioco(p); ok {
		cart = g.Cartella
	}
	traduttore.ApriLibreria(fileLibreriaGioco(cart, nomeGioco(h)))
}

// nomeRiconosciuto: il nome con cui la finestrella chiamava il gioco di questa cartella (per
// ritrovare le librerie salvate col nome dalle versioni di prima).
func nomeRiconosciuto(cartella string) string {
	if cartella == "" {
		return ""
	}
	if g, ok := riconosciGioco(filepath.Join(cartella, "x.exe")); ok {
		return g.Nome
	}
	return ""
}

// fileLibreriaGioco: la libreria di un gioco, per cartella: lo stesso gioco ha nomi diversi in
// Tasso, nei negozi e nel nome dell'exe, la cartella è una sola. Le librerie salvate col nome
// (versioni di prima) si spostano qui la prima volta. Senza cartella: col nome, come prima.
func fileLibreriaGioco(cartella string, nomi ...string) string {
	dir := filepath.Join(dirApp, "librerie")
	if cartella == "" {
		nome := ""
		if len(nomi) > 0 {
			nome = nomi[0]
		}
		return fileLibreriaPer(dir, nome)
	}
	nuovo := filepath.Join(dir, "gioco-"+chiaveGioco(cartella)+".json")
	if _, err := os.Stat(nuovo); err == nil {
		return nuovo
	}
	for _, n := range nomi {
		if n == "" {
			continue
		}
		vecchio := fileLibreriaPer(dir, n)
		if _, err := os.Stat(vecchio); err != nil {
			continue
		}
		if os.Rename(vecchio, nuovo) != nil {
			return vecchio
		}
		dubbi := func(f string) string { return strings.TrimSuffix(f, ".json") + "-da-controllare.json" }
		_ = os.Rename(dubbi(vecchio), dubbi(nuovo))
		return nuovo
	}
	return nuovo
}
