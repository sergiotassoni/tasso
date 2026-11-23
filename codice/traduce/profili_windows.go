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

func eNostro(exe string) bool {
	return exe == "" || exe == "tassotraduce.exe" || exe == nostroExe() || exe == "explorer.exe" || exe == "msedgewebview2.exe"
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
	if eNostro(exe) {
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
	if eNostro(exe) {
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
	ov.gioco = fg
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
