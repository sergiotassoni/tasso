//go:build windows

// TassoProva.exe: banco di prova della traduzione dello schermo, SENZA permessi di amministratore.
// Doppio clic e basta: per 20 minuti (o finché accanto all'exe compare un file STOP, o chiudi la
// finestrella) fotografa lo schermo ogni 2 secondi e su ogni fotogramma fa tutto il percorso della
// traduzione di Tasso (lettura del testo di Windows, correzione, righe intere, frasi pronte,
// traduttore veloce, riquadro di copertura). Salva accanto all'exe, in "risultati AAAA-MM-GG_HHMM":
// i fotogrammi (larghi 1280 px), gli stessi tradotti, report.html e report.txt.
// Se accanto all'exe c'è prova.txt con "cartella=<percorso>" lavora sulle immagini di quella
// cartella invece che sullo schermo. Usa i modelli già scaricati del traduttore, solo in lettura.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"tasso/traduce"
)

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	gdi32            = windows.NewLazySystemDLL("gdi32.dll")
	pRegisterClassEx = user32.NewProc("RegisterClassExW")
	pCreateWindowEx  = user32.NewProc("CreateWindowExW")
	pDefWindowProc   = user32.NewProc("DefWindowProcW")
	pGetMessage      = user32.NewProc("GetMessageW")
	pTranslateMsg    = user32.NewProc("TranslateMessage")
	pDispatchMsg     = user32.NewProc("DispatchMessageW")
	pPostQuit        = user32.NewProc("PostQuitMessage")
	pShowWindow      = user32.NewProc("ShowWindow")
	pInvalidateRect  = user32.NewProc("InvalidateRect")
	pBeginPaint      = user32.NewProc("BeginPaint")
	pEndPaint        = user32.NewProc("EndPaint")
	pDrawText        = user32.NewProc("DrawTextW")
	pGetClientRect   = user32.NewProc("GetClientRect")
	pFillRect        = user32.NewProc("FillRect")
	pLoadIcon        = user32.NewProc("LoadIconW")
	pLoadCursor      = user32.NewProc("LoadCursorW")
	pSetDpiCtx       = user32.NewProc("SetProcessDpiAwarenessContext")
	pSetAffinity     = user32.NewProc("SetWindowDisplayAffinity")
	pCreateFont      = gdi32.NewProc("CreateFontW")
	pSelectObject    = gdi32.NewProc("SelectObject")
	pSetBkMode       = gdi32.NewProc("SetBkMode")
	pSetTextColor    = gdi32.NewProc("SetTextColor")
	pCreateBrush     = gdi32.NewProc("CreateSolidBrush")
	pGetModuleHandle = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW")

	finestra  uintptr
	muTesto   sync.Mutex
	testoOra  = "Prova in corso… preparo il traduttore."
	ferma     atomic.Bool
	finito    atomic.Bool
	fontTesto uintptr
	fondo     uintptr
)

func init() { runtime.LockOSThread() } // la finestrella e i suoi messaggi stanno sul filo principale

type wndClassEx struct {
	cbSize                   uint32
	style                    uint32
	lpfnWndProc              uintptr
	cbClsExtra, cbWndExtra   int32
	hInstance, hIcon         uintptr
	hCursor, hbrBackground   uintptr
	lpszMenuName, lpszClassN *uint16
	hIconSm                  uintptr
}

type msgW struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      [2]int32
	private uint32
}

type rect struct{ L, T, R, B int32 }

type paintStruct struct {
	hdc        uintptr
	fErase     int32
	rc         rect
	fRestore   int32
	fIncUpdate int32
	res        [32]byte
}

func scrivi(s string) {
	muTesto.Lock()
	testoOra = s
	muTesto.Unlock()
	if finestra != 0 {
		pInvalidateRect.Call(finestra, 0, 1)
	}
}

func proc(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case 0x000F: // WM_PAINT
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		var r rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
		pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), fondo)
		pSetBkMode.Call(hdc, 1)
		pSetTextColor.Call(hdc, 0x00F8F4F0)
		pSelectObject.Call(hdc, fontTesto)
		muTesto.Lock()
		t, _ := windows.UTF16FromString(testoOra)
		muTesto.Unlock()
		r.L, r.T, r.R, r.B = r.L+18, r.T+16, r.R-18, r.B-12
		pDrawText.Call(hdc, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&r)), 0x10|0x800) // WORDBREAK|NOPREFIX
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case 0x0010: // WM_CLOSE: ferma la prova e scrive il report prima di uscire
		if finito.Load() {
			pPostQuit.Call(0)
			return 0
		}
		ferma.Store(true)
		scrivi("Fermo la prova e scrivo il report… (un momento)")
		return 0
	case 0x0002: // WM_DESTROY
		pPostQuit.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wp, lp)
	return r
}

// cartellaRisultati: accanto all'exe; se lì non si può scrivere, nei Documenti.
func cartellaRisultati(dirExe string) string {
	nome := "risultati " + time.Now().Format("2006-01-02_1504")
	d := filepath.Join(dirExe, nome)
	if err := os.MkdirAll(d, 0o755); err == nil {
		if f, err := os.Create(filepath.Join(d, ".prova")); err == nil {
			f.Close()
			_ = os.Remove(filepath.Join(d, ".prova"))
			return d
		}
	}
	home, _ := os.UserHomeDir()
	d = filepath.Join(home, "Documents", "TassoProva", nome)
	_ = os.MkdirAll(d, 0o755)
	return d
}

// leggiProva: prova.txt accanto all'exe ("cartella=…", "minuti=…").
func leggiProva(dirExe string) (cartella string, minuti int) {
	minuti = 20
	f, err := os.Open(filepath.Join(dirExe, "prova.txt"))
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\uFEFF")), "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "cartella":
			cartella = strings.Trim(strings.TrimSpace(v), `"`)
		case "minuti":
			fmt.Sscan(strings.TrimSpace(v), &minuti)
		}
	}
	return
}

func lavora(dirExe string) {
	defer finito.Store(true)
	out := cartellaRisultati(dirExe)
	cartella, minuti := leggiProva(dirExe)
	nota := ""
	if err := traduce.AvviaAutonomo(out, 90*time.Second); err != nil {
		nota = "Attenzione: " + err.Error() + "\n"
	}
	stop := func() bool {
		if ferma.Load() {
			return true
		}
		_, err := os.Stat(filepath.Join(dirExe, "STOP"))
		return err == nil
	}
	var err error
	if cartella != "" {
		scrivi(nota + "Prova in corso sulle immagini di:\n" + cartella + "\n(chiudi per fermare)")
		err = copiaImmagini(cartella, out)
		if err == nil {
			err = traduce.EseguiBancoCartella(out)
		}
	} else {
		fine := time.Now().Add(time.Duration(minuti) * time.Minute)
		scrivi(nota + "Prova in corso… (chiudi per fermare)\nFotografo lo schermo ogni 2 secondi per " + fmt.Sprint(minuti) + " minuti.")
		err = traduce.EseguiBancoSchermo(out, time.Duration(minuti)*time.Minute, 2*time.Second, 1280, stop, func(n int) {
			scrivi(fmt.Sprintf("%sProva in corso… (chiudi per fermare)\nFotogrammi: %d · mancano %d minuti\n%s", nota, n, int(time.Until(fine).Minutes()+0.99), traduce.StatoMotori()))
		})
	}
	for _, n := range [][2]string{{"banco-prova.html", "report.html"}, {"banco-prova.txt", "report.txt"}} {
		_ = os.Rename(filepath.Join(out, n[0]), filepath.Join(out, n[1]))
	}
	_ = os.RemoveAll(filepath.Join(out, "webview2")) // i dati del motore non servono più
	if err != nil {
		scrivi("Prova finita con un problema: " + err.Error() + "\nRisultati in:\n" + out + "\n(puoi chiudere)")
		return
	}
	scrivi("Prova finita. Risultati in:\n" + out + "\n(puoi chiudere)")
	if ferma.Load() {
		time.Sleep(1500 * time.Millisecond)
		pPostQuit.Call(0) // chiusa dall'utente: esco appena il report è scritto
	}
}

func copiaImmagini(da, a string) error {
	voci, err := os.ReadDir(da)
	if err != nil {
		return err
	}
	n := 0
	for _, v := range voci {
		e := strings.ToLower(filepath.Ext(v.Name()))
		if v.IsDir() || (e != ".png" && e != ".jpg" && e != ".jpeg") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(da, v.Name()))
		if err != nil {
			continue
		}
		if os.WriteFile(filepath.Join(a, v.Name()), b, 0o644) == nil {
			n++
		}
	}
	if n == 0 {
		return fmt.Errorf("nessuna immagine in %s", da)
	}
	return nil
}

func main() {
	pSetDpiCtx.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4): lo schermo vero, non ingrandito
	exe, _ := os.Executable()
	dirExe := filepath.Dir(exe)
	inst, _, _ := pGetModuleHandle.Call(0)
	classe, _ := windows.UTF16PtrFromString("TassoProva")
	titolo, _ := windows.UTF16PtrFromString("Tasso Prova")
	ico, _, _ := pLoadIcon.Call(inst, 2) // l'icona di Tasso nelle risorse (IconId 2)
	cur, _, _ := pLoadCursor.Call(0, 32512)
	fondo, _, _ = pCreateBrush.Call(0x00251714) // #141725
	wc := wndClassEx{lpfnWndProc: windows.NewCallback(proc), hInstance: inst, hIcon: ico, hIconSm: ico, hCursor: cur, hbrBackground: fondo, lpszClassN: classe}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	nf, _ := windows.UTF16PtrFromString("Segoe UI")
	fontTesto, _, _ = pCreateFont.Call(uintptr(^uint32(17)), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(nf))) // -18 px
	// finestrella piccola, sempre sopra (0x8 TOPMOST, 0x80 TOOLWINDOW no: deve avere l'icona), in basso a destra
	h, _, _ := pCreateWindowEx.Call(0x8, uintptr(unsafe.Pointer(classe)), uintptr(unsafe.Pointer(titolo)),
		0x00C00000|0x00080000|0x10000000, // CAPTION | SYSMENU | VISIBLE
		40, 40, 440, 190, 0, 0, inst, 0)
	finestra = h
	pSetAffinity.Call(h, 0x11) // WDA_EXCLUDEFROMCAPTURE: la finestrella non finisce nei fotogrammi
	pShowWindow.Call(h, 4)     // SW_SHOWNOACTIVATE: il gioco o il video restano davanti
	go lavora(dirExe)
	var m msgW
	for {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMsg.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMsg.Call(uintptr(unsafe.Pointer(&m)))
	}
	// uscita chiesta prima della fine: aspetto il report (al massimo 60 s)
	for i := 0; i < 600 && !finito.Load(); i++ {
		ferma.Store(true)
		time.Sleep(100 * time.Millisecond)
	}
}
