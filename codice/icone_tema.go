package main

// Icone che cambiano col tema (Gaming e Ufficio; Tasso Zip e Tasso PDF restano sempre le loro).
// Quando cambia il tema o la modalità, e all'avvio:
//   - icona della finestra e della barra delle applicazioni (WM_SETICON grande e piccola);
//   - icona vicino all'orologio (tray.go, NIM_MODIFY);
//   - collegamenti di Tasso sul Desktop e nel menu Start (quelli dell'installatore, e quelli
//     appuntati sulla barra): cambia SOLO l'icona del collegamento, e solo se il collegamento apre
//     proprio questo Tasso.exe. Mai creati, spostati o cancellati collegamenti.
//   - logo dentro Tasso: il disegno (SVG) del tema, con gli occhi che si muovono come prima
//     (goIconaTema, ui/index.html).
//
// Le icone sono risorse dentro Tasso.exe (icotema, strumenti/icone-temi): i collegamenti puntano a
// "Tasso.exe,-numero", un file che c'è sempre. Col tema "tasso" tutto torna all'icona di sempre
// (risorsa 2, indice 0: come scrive l'installatore). Le icone fuori da Tasso sono sempre ferme,
// anche con Neon RGB.

import (
	"embed"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"tasso/icotema"
)

//go:embed assets/temi/elenco.txt assets/temi/*/*.svg
var temiFS embed.FS

var (
	elencoIcone   icotema.Elenco
	iconaTemaID   atomic.Uint32 // l'icona da mostrare adesso (numero della risorsa)
	iconeFinMu    sync.Mutex
	iconeFinGr    uintptr // icone della finestra caricate da me (da liberare al cambio)
	iconeFinPi    uintptr
	collegamMu    sync.Mutex
	pDestroyIcoTe = user32.NewProc("DestroyIcon")
	pSHChangeNoti = shell32.NewProc("SHChangeNotify")
)

const wmTrayTema = 0x8000 + 44 // WM_APP+44: tray.go ricarica l'icona del tema

func init() {
	if b, err := temiFS.ReadFile("assets/temi/elenco.txt"); err == nil {
		elencoIcone, _ = icotema.Leggi(b) // elenco rotto: tutti i temi con l'icona normale
	}
}

// idIconaInUso: il numero dell'icona del tema che si vede adesso (impostazioni salvate).
func idIconaInUso() uint16 {
	setMu.Lock()
	p, t, u := settings.Profile, settings.Theme, settings.ThemeOffice
	setMu.Unlock()
	return elencoIcone.IDPer(icotema.TemaInUso(p, t, u), icotema.Variante(p))
}

// idIconaAttuale: quella da usare per finestra e tray (2 se non ancora decisa).
func idIconaAttuale() uintptr {
	if v := iconaTemaID.Load(); v != 0 {
		return uintptr(v)
	}
	return icotema.IDNormale
}

// applicaIconeTema mette l'icona del tema in uso dappertutto. Si può chiamare da qualsiasi filo.
func applicaIconeTema() {
	id := idIconaInUso()
	prima := iconaTemaID.Swap(uint32(id))
	if w := wv; w != nil {
		if h := uintptr(w.Window()); h != 0 {
			iconeFinestra(h, id)
		}
	}
	if prima != 0 && prima != uint32(id) {
		if h := trayHwnd.Load(); h != 0 {
			pPostMessage.Call(h, wmTrayTema, 0, 0)
		}
	}
	go func() {
		defer guard("icone-collegamenti")
		aggiornaCollegamenti(id)
	}()
}

// iconeFinestra: WM_SETICON grande (barra delle applicazioni, Alt+Tab) e piccola (titolo).
func iconeFinestra(h uintptr, id uint16) {
	var hinst windows.Handle
	_ = windows.GetModuleHandleEx(0, nil, &hinst)
	bx, _, _ := pGetSysMetrics.Call(11) // SM_CXICON
	by, _, _ := pGetSysMetrics.Call(12)
	sx, _, _ := pGetSysMetrics.Call(49) // SM_CXSMICON
	sy, _, _ := pGetSysMetrics.Call(50)
	gr, _, _ := pLoadImage.Call(uintptr(hinst), uintptr(id), 1, bx, by, 0)
	pi, _, _ := pLoadImage.Call(uintptr(hinst), uintptr(id), 1, sx, sy, 0)
	if gr == 0 || pi == 0 { // risorsa che non c'è (non dovrebbe succedere): icona normale
		if gr != 0 {
			pDestroyIcoTe.Call(gr)
		}
		if pi != 0 {
			pDestroyIcoTe.Call(pi)
		}
		gr, _, _ = pLoadImage.Call(uintptr(hinst), icotema.IDNormale, 1, bx, by, 0)
		pi, _, _ = pLoadImage.Call(uintptr(hinst), icotema.IDNormale, 1, sx, sy, 0)
		if gr == 0 || pi == 0 {
			return
		}
	}
	iconeFinMu.Lock()
	defer iconeFinMu.Unlock()
	pSendMessage.Call(h, 0x0080, 1, gr) // WM_SETICON, ICON_BIG
	pSendMessage.Call(h, 0x0080, 0, pi) // ICON_SMALL
	if iconeFinGr != 0 {
		pDestroyIcoTe.Call(iconeFinGr)
	}
	if iconeFinPi != 0 {
		pDestroyIcoTe.Call(iconeFinPi)
	}
	iconeFinGr, iconeFinPi = gr, pi
}

// svgIconaTema: il logo del tema per la pagina di Tasso, "" = logo di sempre. Gli id interni
// (sfumature, maschere) prendono un prefisso, così due disegni nella pagina non si pestano; gli
// occhi prendono la classe "pup" (si muovono come nel logo di sempre); il titolo va via
// (il logo è decorativo, accanto c'è già la scritta TASSO).
func svgIconaTema(tema string, ufficio bool) string {
	if !icotema.TemaValido(tema) {
		return ""
	}
	vv := []string{icotema.Gaming}
	if ufficio {
		vv = []string{icotema.Ufficio, icotema.Gaming}
	}
	for _, v := range vv {
		if elencoIcone.Trova(tema, v) == 0 {
			continue
		}
		if b, err := temiFS.ReadFile("assets/temi/" + tema + "/tasso-" + v + ".svg"); err == nil {
			return icotema.PreparaSVG(string(b), "tt-"+tema+"-"+v+"-")
		}
	}
	return ""
}

// ---------------- collegamenti (.lnk) ----------------

var (
	clsidShellLinkT  = windows.GUID{Data1: 0x00021401, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIShellLinkWT  = windows.GUID{Data1: 0x000214F9, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIPersistFileT = windows.GUID{Data1: 0x0000010B, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
)

type comT struct{ vtbl *[32]uintptr }

func comChiama(o *comT, i int, a ...uintptr) bool {
	r, _, _ := syscall.SyscallN(o.vtbl[i], append([]uintptr{uintptr(unsafe.Pointer(o))}, a...)...)
	return int32(r) >= 0
}

// nomiCollegamenti: i nomi che usa l'installatore (installa/motore.go) per Tasso.
var nomiCollegamenti = []string{"Tasso Gaming.lnk", "Tasso Ufficio.lnk", "Tasso.lnk"}

// cartelleCollegamenti: menu Start e Desktop di tutti (installatore), quelli dell'utente e la
// barra delle applicazioni (collegamenti appuntati).
func cartelleCollegamenti() []string {
	var out []string
	for _, id := range []*windows.KNOWNFOLDERID{windows.FOLDERID_CommonPrograms, windows.FOLDERID_PublicDesktop,
		windows.FOLDERID_Programs, windows.FOLDERID_Desktop} {
		if p := cartellaNota(id); p != "" {
			out = append(out, p)
		}
	}
	if p := cartellaNota(windows.FOLDERID_RoamingAppData); p != "" {
		out = append(out, filepath.Join(p, `Microsoft\Internet Explorer\Quick Launch\User Pinned\TaskBar`))
	}
	return out
}

// aggiornaCollegamenti cambia l'icona dei collegamenti che aprono questo Tasso.exe.
func aggiornaCollegamenti(id uint16) {
	exe := exePath
	if exe == "" {
		return
	}
	if a, err := filepath.Abs(exe); err == nil {
		exe = filepath.Clean(a)
	}
	collegamMu.Lock()
	defer collegamMu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err == nil {
		defer windows.CoUninitialize()
	}
	visti := map[string]bool{}
	for _, dir := range cartelleCollegamenti() {
		for _, n := range nomiCollegamenti {
			p := filepath.Join(dir, n)
			if visti[strings.ToLower(p)] {
				continue
			}
			visti[strings.ToLower(p)] = true
			fi, err := os.Lstat(p)
			if err != nil || !fi.Mode().IsRegular() {
				continue // niente, o un collegamento simbolico: non tocco
			}
			if iconaCollegamento(p, exe, icotema.PosizioneIcona(id)) {
				pp, _ := windows.UTF16PtrFromString(p)
				pSHChangeNoti.Call(0x00002000, 0x0005, uintptr(unsafe.Pointer(pp)), 0) // SHCNE_UPDATEITEM, SHCNF_PATHW
			}
		}
	}
}

// iconaCollegamento: true se ha cambiato l'icona del collegamento p (che deve aprire exe).
func iconaCollegamento(p, exe string, indice int32) bool {
	var sl *comT
	r, _, _ := pCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidShellLinkT)), 0, 1, // CLSCTX_INPROC_SERVER
		uintptr(unsafe.Pointer(&iidIShellLinkWT)), uintptr(unsafe.Pointer(&sl)))
	if int32(r) < 0 || sl == nil {
		return false
	}
	defer comChiama(sl, 2) // Release
	var pf *comT
	if !comChiama(sl, 0, uintptr(unsafe.Pointer(&iidIPersistFileT)), uintptr(unsafe.Pointer(&pf))) || pf == nil {
		return false
	}
	defer comChiama(pf, 2)
	pp, _ := windows.UTF16PtrFromString(p)
	if !comChiama(pf, 5, uintptr(unsafe.Pointer(pp)), 0) { // IPersistFile::Load(p, STGM_READ)
		return false
	}
	// IShellLinkW: 3 GetPath, 16 GetIconLocation, 17 SetIconLocation
	buf := make([]uint16, 1024)
	if !comChiama(sl, 3, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, 4) { // SLGP_RAWPATH
		return false
	}
	dest := windows.UTF16ToString(buf)
	dest = espandiAmbiente(dest)
	if dest == "" || !strings.EqualFold(filepath.Clean(dest), exe) {
		return false // non apre questo Tasso: non è nostro, non lo tocco
	}
	ico := make([]uint16, 1024)
	var idx int32
	if comChiama(sl, 16, uintptr(unsafe.Pointer(&ico[0])), uintptr(len(ico)), uintptr(unsafe.Pointer(&idx))) {
		cur := espandiAmbiente(windows.UTF16ToString(ico))
		if idx == indice && (strings.EqualFold(filepath.Clean(cur), exe) || cur == "" && indice == 0) {
			return false // è già giusta
		}
	}
	pe, _ := windows.UTF16PtrFromString(exe)
	if !comChiama(sl, 17, uintptr(unsafe.Pointer(pe)), uintptr(uint32(indice))) {
		return false
	}
	return comChiama(pf, 6, uintptr(unsafe.Pointer(pp)), 1) // Save(p, TRUE)
}

// espandiAmbiente: %ProgramFiles% e simili in un percorso di un collegamento.
func espandiAmbiente(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	src, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return s
	}
	buf := make([]uint16, 1024)
	n, err := windows.ExpandEnvironmentStrings(src, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || int(n) > len(buf) {
		return s
	}
	return windows.UTF16ToString(buf)
}
