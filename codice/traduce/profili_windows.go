//go:build windows

package traduce

// Profili per gioco: quando premi F9 dentro un gioco, zona, posizione e grandezza del testo
// vengono ricordate per quel gioco (riconosciuto dal nome del suo .exe). Quando torni in quel
// gioco, si ricaricano da sole.

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	muProfilo                 sync.Mutex
	profiloAttivo             string
	ultimaFinestra            uintptr
)

// exeDiFinestra: "gioco.exe" della finestra, in minuscolo ("" se non si riesce).
func exeDiFinestra(h uintptr) string {
	p := percorsoDiFinestra(h)
	if p == "" {
		return ""
	}
	return strings.ToLower(filepath.Base(p))
}

// percorsoDiFinestra: percorso completo del .exe della finestra.
func percorsoDiFinestra(h uintptr) string {
	if h == 0 {
		return ""
	}
	var pid uint32
	pGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return ""
	}
	ph, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(ph)
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(ph, 0, &buf[0], &n); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// programmi che non sono mai giochi: niente profilo, niente "Profilo caricato" (strumenti di
// Windows, browser, app di lavoro). Si aggiungono a nonGiochi e nonGiochiNegozi.

// nonGiocoMai: true per i programmi a cui non si dà mai un profilo: i nostri, quelli in elenco e
// quelli che stanno nella cartella di Windows (C:\Windows\...).
func nonGiocoMai(h uintptr, exe string) bool {
	if eNostro(exe) || maiProfilo[exe] || nonGiochi[exe] || nonGiochiNegozi[exe] {
		return true
	}
	p := strings.ToLower(percorsoDiFinestra(h))
	if w := strings.ToLower(os.Getenv("SystemRoot")); w != "" && strings.HasPrefix(p, strings.TrimRight(w, `\`)+`\`) {
		return true
	}
	if !strings.Contains(p, `\windowsapps\microsoft.`) {
		return false
	}
	_, gioco := riconosciGioco(p) // app di Windows dallo Store (Strumento di cattura...), ma non i giochi Xbox
	return !gioco
}

func eNostro(exe string) bool {
	// anche un'altra copia di Tasso aperta insieme (Tasso-3_1-prova.exe, "Tasso Gaming.exe"):
	// la traduzione non deve mai leggere le finestre di Tasso (foto di Sergio 28/11: "Steam" → "Vapore")
	return exe == "" || exe == "tassotraduce.exe" || exe == nostroExe() || exe == "explorer.exe" || exe == "msedgewebview2.exe" ||
		strings.HasPrefix(exe, "tasso")
}

// nostroExe: il nome del programma che contiene Tasso Traduce (Tasso.exe, o una sua copia rinominata).
var nostroExe = sync.OnceValue(func() string {
	p, err := os.Executable()
	if err != nil {
		return "tasso.exe"
	}
	return strings.ToLower(filepath.Base(p))
})

// ricordaProfilo: salva le scelte attuali nel profilo del gioco su cui hai premuto F9.
func ricordaProfilo() {
	ov.mu.Lock()
	g := ov.gioco
	ov.mu.Unlock()
	exe := exeDiFinestra(g)
	if nonGiocoMai(g, exe) {
		return
	}
	muImp.Lock()
	if imp.Profili == nil {
		imp.Profili = map[string]Profilo{}
	}
	pr := imp.Profili[exe] // tengo la zona della chat, se c'è
	pr.Zona, pr.Posizione, pr.DimFont = imp.Zona, imp.Posizione, imp.DimFont
	imp.Profili[exe] = pr
	muImp.Unlock()
	muProfilo.Lock()
	profiloAttivo = exe
	muProfilo.Unlock()
	salvaImp()
}

// controllaProfilo: se in primo piano c'è un gioco con un profilo, lo carica.
func controllaProfilo() {
	fg, _, _ := pGetForeground.Call()
	muProfilo.Lock()
	stessa := fg == ultimaFinestra
	ultimaFinestra = fg
	muProfilo.Unlock()
	if stessa {
		return
	}
	exe := exeDiFinestra(fg)
	if nonGiocoMai(fg, exe) {
		return
	}
	muImp.Lock()
	p, ok := imp.Profili[exe]
	muImp.Unlock()
	muProfilo.Lock()
	gia := profiloAttivo == exe
	if ok {
		profiloAttivo = exe
	}
	muProfilo.Unlock()
	if !ok {
		return
	}
	// la finestra del gioco può essere nuova anche con lo stesso profilo (gioco chiuso e
	// riaperto): la finestrella deve seguire quella di adesso
	ov.mu.Lock()
	ov.gioco, ov.exeGioco = fg, exe
	ov.mu.Unlock()
	if gia {
		return
	}
	muImp.Lock()
	imp.Zona = p.Zona
	if p.Posizione != "" {
		imp.Posizione = p.Posizione
	}
	if p.DimFont > 0 {
		imp.DimFont = p.DimFont
	}
	pos, font := imp.Posizione, imp.DimFont
	muImp.Unlock()
	salvaImp()
	ov.ImpostaPosizione(pos)
	ov.ImpostaFont(font)
	muImpronte.Lock()
	ultimaImp = Impronta{}
	muImpronte.Unlock()
	emetti("aggiorna", stato())
	emetti("info", map[string]string{"testo": "Profilo caricato: " + exe})
}

// finestraNostra: la finestra è di Tasso (questo programma o un'altra copia di Tasso).
func finestraNostra(h uintptr) bool {
	if h == 0 {
		return true
	}
	var pid uint32
	pGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&pid)))
	if pid == windows.GetCurrentProcessId() {
		return true
	}
	return eNostro(exeDiFinestra(h)) || h == sel.hwnd.Load() || (wv != nil && h == uintptr(wv.Window()))
}

var (
	muPerTradurre    sync.Mutex
	cachePerTradurre = map[uintptr]esitoFinestra{}
)

type esitoFinestra struct {
	si     bool
	quando time.Time
}

// dimenticaFinestre: le decisioni ricordate non valgono più (profilo nuovo, libreria cambiata).
func dimenticaFinestre() {
	muPerTradurre.Lock()
	clear(cachePerTradurre)
	muPerTradurre.Unlock()
}

// finestraPerTradurre: in primo piano c'è un gioco da tradurre (gioco_davanti.go decide).
// Ricordato per 2 secondi per finestra: la chiamano più fili, anche ogni 400 ms.
func finestraPerTradurre(h uintptr) bool {
	if h == 0 || finestraNostra(h) {
		return false
	}
	if paginaAttiva.Load() && h == paginaGiocoA.Load() && !paginaUnaVolta.Load() {
		return true
	}
	muPerTradurre.Lock()
	if e, ok := cachePerTradurre[h]; ok && time.Since(e.quando) < 2*time.Second {
		muPerTradurre.Unlock()
		return e.si
	}
	muPerTradurre.Unlock()
	si := finestraPerTradurreOra(h)
	muPerTradurre.Lock()
	if len(cachePerTradurre) > 64 {
		cachePerTradurre = map[uintptr]esitoFinestra{}
	}
	cachePerTradurre[h] = esitoFinestra{si, time.Now()}
	muPerTradurre.Unlock()
	return si
}

func finestraPerTradurreOra(h uintptr) bool {
	exe := exeDiFinestra(h)
	p := strings.ToLower(percorsoDiFinestra(h))
	f := infoFinestra{Exe: exe, Percorso: p, Nostra: eNostro(exe) || h == sel.hwnd.Load()}
	if wv != nil && h == uintptr(wv.Window()) {
		f.Nostra = true
	}
	if w := strings.ToLower(os.Getenv("SystemRoot")); w != "" && strings.HasPrefix(p, strings.TrimRight(w, `\`)+`\`) {
		f.DiWindows = true
	}
	_, f.DaNegozio = riconosciGioco(p)
	if strings.Contains(p, `\windowsapps\microsoft.`) && !f.DaNegozio {
		f.DiWindows = true
	}
	muImp.Lock()
	_, f.Profilo = imp.Profili[exe]
	muImp.Unlock()
	if Ospite.GiocoDiTasso != nil {
		f.DiTasso = Ospite.GiocoDiTasso(p)
	}
	if !f.DiTasso && !f.Nostra {
		// in libreria c'è il launcher ("Hytale Launcher") e il gioco gira con un altro programma
		_, f.DiTasso = giocoParenteDiFinestra(h)
	}
	f.Tradotto = paginaAttiva.Load() && h == paginaGiocoA.Load() && !paginaUnaVolta.Load()
	f.SchermoIntero = aSchermoIntero(h)
	return decidiGioco(f)
}
