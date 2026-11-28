//go:build windows
// +build windows

package webview2

import (
	"encoding/json"
	"errors"
	"log"
	"reflect"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/jchv/go-webview2/internal/w32"
	"github.com/jchv/go-webview2/pkg/edge"

	"fmt"
	"golang.org/x/sys/windows"
	"runtime/debug"
)

var (
	windowContext     = map[uintptr]interface{}{}
	windowContextSync sync.RWMutex
)

func getWindowContext(wnd uintptr) interface{} {
	windowContextSync.RLock()
	defer windowContextSync.RUnlock()
	return windowContext[wnd]
}

func setWindowContext(wnd uintptr, data interface{}) {
	windowContextSync.Lock()
	defer windowContextSync.Unlock()
	windowContext[wnd] = data
}

type browser interface {
	Embed(hwnd uintptr) bool
	Resize()
	Navigate(url string)
	NavigateToString(htmlContent string)
	Init(script string)
	Eval(script string)
	NotifyParentWindowPositionChanged() error
	Focus()
}

type webview struct {
	hwnd       uintptr
	mainthread uintptr
	browser    browser
	autofocus  bool
	maxsz      w32.Point
	minsz      w32.Point
	m          sync.Mutex
	bindings   map[string]interface{}
	dispatchq  []func()
	// La coda delle chiamate dei pulsanti (modifica di Tasso, vedi msgcb e lavoraCoda).
	codaMu     sync.Mutex
	codaLista  []rpcMessage
	codaEvento windows.Handle // segnala al filo della coda che è arrivata una chiamata (o che deve chiudere)
	codaAvvio  bool           // il filo della coda è partito
	codaChiusa bool           // la WebView è stata chiusa: niente più chiamate, il filo esce
	// Modifica di Tasso: 1 dopo Destroy(), così la chiusura chiesta dal programma non passa da ChiusuraHook.
	chiusuraVera int32
}

// Funzioni di Windows per far girare i messaggi nel filo della coda (non sono in internal/w32).
var (
	user32Coda                      = windows.NewLazySystemDLL("user32.dll")
	user32MsgWaitForMultipleObjects = user32Coda.NewProc("MsgWaitForMultipleObjectsEx")
	user32PeekMessageW              = user32Coda.NewProc("PeekMessageW")
)

// SuFinestra: funzioni legate con Bind che devono girare nel filo della finestra (aprono
// finestre di Windows come "Scegli un file", che vogliono quel filo). Modifica di Tasso.
var SuFinestra = map[string]bool{}

// Subito: funzioni veloci e di sola lettura (es. "Ferma", "sta lavorando?", i numeri dal vivo) che
// girano subito in un filo tutto loro, senza aspettare in coda dietro un lavoro lento.
// Modifica di Tasso: si impostano prima di aprire la pagina, come SuFinestra.
var Subito = map[string]bool{}

type WindowOptions struct {
	Title  string
	Width  uint
	Height uint
	IconId uint
	Center bool
}

type WebViewOptions struct {
	Window unsafe.Pointer
	Debug  bool

	// DataPath specifies the datapath for the WebView2 runtime to use for the
	// browser instance.
	DataPath string

	// AutoFocus will try to keep the WebView2 widget focused when the window
	// is focused.
	AutoFocus bool

	// WindowOptions customizes the window that is created to embed the
	// WebView2 widget.
	WindowOptions WindowOptions

	// Modifiche di Tasso Zip (vedi pkg/edge/guardiano.go).
	// NavigazionePermessa: gli indirizzi che la pagina (e i suoi riquadri) possono aprire.
	NavigazionePermessa func(uri string) bool
	// PopupRichiesto: la pagina ha chiesto una finestra nuova (non si apre mai).
	PopupRichiesto func(uri string)
	// FontePermessa: le pagine da cui si accettano i messaggi per le funzioni legate con Bind.
	FontePermessa func(uri string) bool
}

// New creates a new webview in a new window.
func New(debug bool) WebView { return NewWithOptions(WebViewOptions{Debug: debug}) }

// NewWindow creates a new webview using an existing window.
//
// Deprecated: Use NewWithOptions.
func NewWindow(debug bool, window unsafe.Pointer) WebView {
	return NewWithOptions(WebViewOptions{Debug: debug, Window: window})
}

// NewWithOptions creates a new webview using the provided options.
func NewWithOptions(options WebViewOptions) WebView {
	w := &webview{}
	w.bindings = map[string]interface{}{}
	w.autofocus = options.AutoFocus

	chromium := edge.NewChromium()
	chromium.MessageCallback = w.msgcb
	chromium.DataPath = options.DataPath
	chromium.NavigazionePermessa = options.NavigazionePermessa
	chromium.PopupRichiesto = options.PopupRichiesto
	chromium.FontePermessa = options.FontePermessa
	chromium.SetPermission(edge.CoreWebView2PermissionKindClipboardRead, edge.CoreWebView2PermissionStateAllow)

	w.browser = chromium
	w.mainthread, _, _ = w32.Kernel32GetCurrentThreadID.Call()
	if !w.CreateWithOptions(options.WindowOptions) {
		return nil
	}

	settings, err := chromium.GetSettings()
	if err != nil {
		log.Fatal(err)
	}
	// disable context menu
	err = settings.PutAreDefaultContextMenusEnabled(options.Debug)
	if err != nil {
		log.Fatal(err)
	}
	// disable developer tools
	err = settings.PutAreDevToolsEnabled(options.Debug)
	if err != nil {
		log.Fatal(err)
	}
	// Tasso: niente tasti da browser (F5/Ctrl+R ricarica e lascia la pagina bianca, Ctrl+F, Ctrl+P,
	// Alt+Freccia sinistra, zoom da tastiera). Secondo Microsoft restano attivi i tasti di modifica
	// del testo (Ctrl+C, Ctrl+V, Ctrl+X, Ctrl+A, Ctrl+Z) e di movimento (Home, Fine, Pagina su/giù).
	// Niente zoom con Ctrl+rotellina (Indietro bloccato in JS con popstate). Senza log.Fatal:
	// una versione vecchia di WebView2 che non li conosce non deve impedire a Tasso di aprirsi.
	if !options.Debug {
		_ = settings.PutAreBrowserAcceleratorKeysEnabled(false)
		_ = settings.PutIsZoomControlEnabled(false)
	}

	return w
}

type rpcMessage struct {
	ID     int               `json:"id"`
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

func jsString(v interface{}) string { b, _ := json.Marshal(v); return string(b) }

// msgcb: modifica di Tasso. Prima ogni pulsante girava nel filo della finestra: se il lavoro
// durava (leggere un file, chiedere qualcosa a Windows) la finestra restava ferma e Tasso
// sembrava lento ad aprirsi o bloccato. Ora le chiamate girano in un filo a parte, una alla
// volta e nell'ordine in cui arrivano (come prima), e la finestra resta sempre libera.
// Le risposte tornano alla pagina con Dispatch, come prima.
func (w *webview) msgcb(msg string) {
	d := rpcMessage{}
	if err := json.Unmarshal([]byte(msg), &d); err != nil {
		log.Printf("invalid RPC message: %v", err)
		return
	}
	if SuFinestra[d.Method] {
		w.rispondi(d)
		return
	}
	if Subito[d.Method] {
		go w.rispondiSicuro(d)
		return
	}
	w.codaMu.Lock()
	defer w.codaMu.Unlock()
	if w.codaChiusa {
		return // la finestra è chiusa: nessuno aspetta più la risposta
	}
	if !w.codaAvvio {
		ev, err := windows.CreateEvent(nil, 0, 0, nil) // si azzera da solo quando il filo si sveglia
		if err != nil {
			// Caso rarissimo: senza evento non posso svegliare il filo. Rispondo in un filo a parte,
			// meglio che lasciare il pulsante senza risposta.
			go w.rispondiSicuro(d)
			return
		}
		w.codaEvento = ev
		w.codaAvvio = true
		go w.lavoraCoda()
	}
	w.codaLista = append(w.codaLista, d)
	_ = windows.SetEvent(w.codaEvento)
}

// chiudiCoda: la WebView è stata chiusa: la coda non accetta più chiamate e il suo filo esce.
func (w *webview) chiudiCoda() {
	w.codaMu.Lock()
	defer w.codaMu.Unlock()
	if w.codaChiusa {
		return
	}
	w.codaChiusa = true
	w.codaLista = nil
	if w.codaAvvio {
		_ = windows.SetEvent(w.codaEvento) // sveglio il filo, che vede la coda chiusa ed esce
	}
}

// prossimaChiamata: la prima chiamata in coda. fine = la coda è chiusa e il filo deve uscire.
func (w *webview) prossimaChiamata() (d rpcMessage, ok, fine bool) {
	w.codaMu.Lock()
	defer w.codaMu.Unlock()
	if w.codaChiusa {
		return d, false, true
	}
	if len(w.codaLista) == 0 {
		return d, false, false
	}
	d = w.codaLista[0]
	w.codaLista[0] = rpcMessage{}
	w.codaLista = w.codaLista[1:]
	if len(w.codaLista) == 0 {
		w.codaLista = nil // libero la memoria della lista vecchia
	}
	return d, true, false
}

// lavoraCoda: il filo dove girano, una alla volta e nell'ordine di arrivo, le chiamate dei pulsanti.
// Molte usano COM (WMI, Esplora file, collegamenti), che va preparato per il filo: il filo resta
// sempre lo stesso (LockOSThread) e COM si prepara una volta sola, come nel filo della finestra.
// Un filo con COM "a filo singolo" deve far girare i messaggi di Windows (COM li usa per le sue
// chiamate e per le finestre nascoste che crea): quando non c'è niente da fare aspetto con
// MsgWaitForMultipleObjectsEx sia una nuova chiamata sia un messaggio, e servo i messaggi.
func (w *webview) lavoraCoda() {
	runtime.LockOSThread()
	comOK := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED) == nil
	defer func() {
		if comOK {
			windows.CoUninitialize()
		}
		w.codaMu.Lock()
		_ = windows.CloseHandle(w.codaEvento)
		w.codaEvento = 0
		w.codaMu.Unlock()
		runtime.UnlockOSThread()
	}()
	const (
		qsAllInput         = 0x04FF // QS_ALLINPUT
		mwmoInputAvailable = 0x0004 // MWMO_INPUTAVAILABLE: si sveglia anche per messaggi già visti ma non ancora tolti
		attesaInfinita     = 0xFFFFFFFF
	)
	for {
		pompaMessaggi()
		d, ok, fine := w.prossimaChiamata()
		if fine {
			return
		}
		if ok {
			w.rispondiSicuro(d)
			continue
		}
		ev := w.codaEvento
		_, _, _ = user32MsgWaitForMultipleObjects.Call(1, uintptr(unsafe.Pointer(&ev)), attesaInfinita, qsAllInput, mwmoInputAvailable)
	}
}

// pompaMessaggi: serve i messaggi di Windows arrivati al filo della coda, senza aspettare.
func pompaMessaggi() {
	const pmRemove = 0x0001
	var msg w32.Msg
	for i := 0; i < 1000; i++ { // un limite, per non restare qui se arrivano messaggi senza sosta
		r, _, _ := user32PeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, pmRemove)
		if r == 0 {
			return
		}
		if msg.Message == w32.WMQuit {
			continue // la chiusura di Tasso passa dal filo della finestra, non da qui
		}
		_, _, _ = w32.User32TranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		_, _, _ = w32.User32DispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// rispondiSicuro: come rispondi, ma un errore grave non ferma la coda (le altre chiamate continuano).
func (w *webview) rispondiSicuro(d rpcMessage) {
	defer func() {
		if r := recover(); r != nil {
			if PanicHook != nil {
				PanicHook(d.Method, r, debug.Stack())
			}
		}
	}()
	w.rispondi(d)
}

func (w *webview) rispondi(d rpcMessage) {
	id := strconv.Itoa(d.ID)
	if res, err := w.callbinding(d); err != nil {
		w.Dispatch(func() {
			w.Eval("window._rpc[" + id + "].reject(" + jsString(err.Error()) + "); window._rpc[" + id + "] = undefined")
		})
	} else if b, err := json.Marshal(res); err != nil {
		w.Dispatch(func() {
			w.Eval("window._rpc[" + id + "].reject(" + jsString(err.Error()) + "); window._rpc[" + id + "] = undefined")
		})
	} else {
		w.Dispatch(func() {
			w.Eval("window._rpc[" + id + "].resolve(" + string(b) + "); window._rpc[" + id + "] = undefined")
		})
	}
}

// PanicHook, se impostato, riceve gli errori gravi (panic) di una funzione legata con Bind.
// Modifica di Tasso: un errore in un solo pulsante non deve chiudere tutto il programma.
var PanicHook func(method string, r interface{}, stack []byte)

// ChiusuraHook: modifica di Tasso. Se c'è e risponde true, la X della finestra (WM_CLOSE: X, Alt+F4,
// "Chiudi finestra" della barra) non chiude il programma: ci pensa la funzione (Tasso nasconde la
// finestra nell'icona vicino all'orologio). Gira nel filo della finestra: deve essere veloce.
// Destroy() chiude sempre davvero, senza chiedere.
var ChiusuraHook func() bool

func (w *webview) callbinding(d rpcMessage) (res0 interface{}, err0 error) {
	defer func() {
		if r := recover(); r != nil {
			if PanicHook != nil {
				PanicHook(d.Method, r, debug.Stack())
			}
			res0, err0 = nil, fmt.Errorf("errore interno in %s", d.Method)
		}
	}()
	w.m.Lock()
	f, ok := w.bindings[d.Method]
	w.m.Unlock()
	if !ok {
		return nil, nil
	}

	v := reflect.ValueOf(f)
	isVariadic := v.Type().IsVariadic()
	numIn := v.Type().NumIn()
	if (isVariadic && len(d.Params) < numIn-1) || (!isVariadic && len(d.Params) != numIn) {
		return nil, errors.New("function arguments mismatch")
	}
	args := []reflect.Value{}
	for i := range d.Params {
		var arg reflect.Value
		if isVariadic && i >= numIn-1 {
			arg = reflect.New(v.Type().In(numIn - 1).Elem())
		} else {
			arg = reflect.New(v.Type().In(i))
		}
		if err := json.Unmarshal(d.Params[i], arg.Interface()); err != nil {
			return nil, err
		}
		args = append(args, arg.Elem())
	}

	errorType := reflect.TypeOf((*error)(nil)).Elem()
	res := v.Call(args)
	switch len(res) {
	case 0:
		// No results from the function, just return nil
		return nil, nil

	case 1:
		// One result may be a value, or an error
		if res[0].Type().Implements(errorType) {
			if res[0].Interface() != nil {
				return nil, res[0].Interface().(error)
			}
			return nil, nil
		}
		return res[0].Interface(), nil

	case 2:
		// Two results: first one is value, second is error
		if !res[1].Type().Implements(errorType) {
			return nil, errors.New("second return value must be an error")
		}
		if res[1].Interface() == nil {
			return res[0].Interface(), nil
		}
		return res[0].Interface(), res[1].Interface().(error)

	default:
		return nil, errors.New("unexpected number of return values")
	}
}

func wndproc(hwnd, msg, wp, lp uintptr) uintptr {
	if w, ok := getWindowContext(hwnd).(*webview); ok {
		switch msg {
		case w32.WMMove, w32.WMMoving:
			_ = w.browser.NotifyParentWindowPositionChanged()
		case w32.WMNCLButtonDown:
			_, _, _ = w32.User32SetFocus.Call(w.hwnd)
			r, _, _ := w32.User32DefWindowProcW.Call(hwnd, msg, wp, lp)
			return r
		case w32.WMSize:
			w.browser.Resize()
		case w32.WMActivate:
			if wp == w32.WAInactive {
				break
			}
			if w.autofocus {
				w.browser.Focus()
			}
		case w32.WMClose:
			// Modifica di Tasso: la X può nascondere la finestra invece di chiuderla (vedi ChiusuraHook).
			if atomic.LoadInt32(&w.chiusuraVera) == 0 && ChiusuraHook != nil && ChiusuraHook() {
				break
			}
			_, _, _ = w32.User32DestroyWindow.Call(hwnd)
		case w32.WMDestroy:
			w.Terminate()
		case w32.WMGetMinMaxInfo:
			lpmmi := (*w32.MinMaxInfo)(unsafe.Pointer(lp))
			if w.maxsz.X > 0 && w.maxsz.Y > 0 {
				lpmmi.PtMaxSize = w.maxsz
				lpmmi.PtMaxTrackSize = w.maxsz
			}
			if w.minsz.X > 0 && w.minsz.Y > 0 {
				lpmmi.PtMinTrackSize = w.minsz
			}
		default:
			r, _, _ := w32.User32DefWindowProcW.Call(hwnd, msg, wp, lp)
			return r
		}
		return 0
	}
	r, _, _ := w32.User32DefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

func (w *webview) Create(debug bool, window unsafe.Pointer) bool {
	// This function signature stopped making sense a long time ago.
	// It is but legacy cruft at this point.
	return w.CreateWithOptions(WindowOptions{})
}

func (w *webview) CreateWithOptions(opts WindowOptions) bool {
	var hinstance windows.Handle
	_ = windows.GetModuleHandleEx(0, nil, &hinstance)

	var icon uintptr
	if opts.IconId == 0 {
		// load default icon
		icow, _, _ := w32.User32GetSystemMetrics.Call(w32.SystemMetricsCxIcon)
		icoh, _, _ := w32.User32GetSystemMetrics.Call(w32.SystemMetricsCyIcon)
		icon, _, _ = w32.User32LoadImageW.Call(uintptr(hinstance), 32512, icow, icoh, 0)
	} else {
		// load icon from resource
		icon, _, _ = w32.User32LoadImageW.Call(uintptr(hinstance), uintptr(opts.IconId), 1, 0, 0, w32.LR_DEFAULTSIZE|w32.LR_SHARED)
	}

	className, _ := windows.UTF16PtrFromString("webview")
	wc := w32.WndClassExW{
		CbSize:        uint32(unsafe.Sizeof(w32.WndClassExW{})),
		HInstance:     hinstance,
		LpszClassName: className,
		HIcon:         windows.Handle(icon),
		HIconSm:       windows.Handle(icon),
		LpfnWndProc:   windows.NewCallback(wndproc),
	}
	_, _, _ = w32.User32RegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	windowName, _ := windows.UTF16PtrFromString(opts.Title)

	windowWidth := opts.Width
	if windowWidth == 0 {
		windowWidth = 640
	}
	windowHeight := opts.Height
	if windowHeight == 0 {
		windowHeight = 480
	}

	var posX, posY uint
	if opts.Center {
		// get screen size
		screenWidth, _, _ := w32.User32GetSystemMetrics.Call(w32.SM_CXSCREEN)
		screenHeight, _, _ := w32.User32GetSystemMetrics.Call(w32.SM_CYSCREEN)
		// calculate window position
		posX = (uint(screenWidth) - windowWidth) / 2
		posY = (uint(screenHeight) - windowHeight) / 2
	} else {
		// use default position
		posX = w32.CW_USEDEFAULT
		posY = w32.CW_USEDEFAULT
	}

	w.hwnd, _, _ = w32.User32CreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		0xCF0000, // WS_OVERLAPPEDWINDOW
		uintptr(posX),
		uintptr(posY),
		uintptr(windowWidth),
		uintptr(windowHeight),
		0,
		0,
		uintptr(hinstance),
		0,
	)
	setWindowContext(w.hwnd, w)

	_, _, _ = w32.User32ShowWindow.Call(w.hwnd, w32.SWShow)
	_, _, _ = w32.User32UpdateWindow.Call(w.hwnd)
	_, _, _ = w32.User32SetFocus.Call(w.hwnd)

	if !w.browser.Embed(w.hwnd) {
		return false
	}
	w.browser.Resize()
	return true
}

func (w *webview) Destroy() {
	w.chiudiCoda()                        // modifica di Tasso: la coda dei pulsanti si chiude con la WebView
	atomic.StoreInt32(&w.chiusuraVera, 1) // modifica di Tasso: chiusura vera, non passa da ChiusuraHook
	_, _, _ = w32.User32PostMessageW.Call(w.hwnd, w32.WMClose, 0, 0)
}

func (w *webview) Run() {
	var msg w32.Msg
	for {
		_, _, _ = w32.User32GetMessageW.Call(
			uintptr(unsafe.Pointer(&msg)),
			0,
			0,
			0,
		)
		if msg.Message == w32.WMApp {
			w.m.Lock()
			q := append([]func(){}, w.dispatchq...)
			w.dispatchq = []func(){}
			w.m.Unlock()
			for _, v := range q {
				v()
			}
		} else if msg.Message == w32.WMQuit {
			return
		}
		r, _, _ := w32.User32GetAncestor.Call(uintptr(msg.Hwnd), w32.GARoot)
		r, _, _ = w32.User32IsDialogMessage.Call(r, uintptr(unsafe.Pointer(&msg)))
		if r != 0 {
			continue
		}
		_, _, _ = w32.User32TranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		_, _, _ = w32.User32DispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func (w *webview) Terminate() {
	w.chiudiCoda() // modifica di Tasso: la coda dei pulsanti si chiude con la WebView
	_, _, _ = w32.User32PostQuitMessage.Call(0)
}

func (w *webview) Window() unsafe.Pointer {
	return unsafe.Pointer(w.hwnd)
}

func (w *webview) Navigate(url string) {
	w.browser.Navigate(url)
}

func (w *webview) SetHtml(html string) {
	w.browser.NavigateToString(html)
}

func (w *webview) SetTitle(title string) {
	_title, err := windows.UTF16FromString(title)
	if err != nil {
		_title, _ = windows.UTF16FromString("")
	}
	_, _, _ = w32.User32SetWindowTextW.Call(w.hwnd, uintptr(unsafe.Pointer(&_title[0])))
}

func (w *webview) SetSize(width int, height int, hints Hint) {
	index := w32.GWLStyle
	style := w32.GetWindowLong(w.hwnd, index)
	if hints == HintFixed {
		style &^= (w32.WSThickFrame | w32.WSMaximizeBox)
	} else {
		style |= (w32.WSThickFrame | w32.WSMaximizeBox)
	}
	w32.SetWindowLong(w.hwnd, index, style)

	if hints == HintMax {
		w.maxsz.X = int32(width)
		w.maxsz.Y = int32(height)
	} else if hints == HintMin {
		w.minsz.X = int32(width)
		w.minsz.Y = int32(height)
	} else {
		r := w32.Rect{}
		r.Left = 0
		r.Top = 0
		r.Right = int32(width)
		r.Bottom = int32(height)
		_, _, _ = w32.User32AdjustWindowRect.Call(uintptr(unsafe.Pointer(&r)), w32.WSOverlappedWindow, 0)
		_, _, _ = w32.User32SetWindowPos.Call(
			w.hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top),
			w32.SWPNoZOrder|w32.SWPNoActivate|w32.SWPNoMove|w32.SWPFrameChanged)
		w.browser.Resize()
	}
}

func (w *webview) Init(js string) {
	w.browser.Init(js)
}

func (w *webview) Eval(js string) {
	w.browser.Eval(js)
}

func (w *webview) Dispatch(f func()) {
	w.m.Lock()
	w.dispatchq = append(w.dispatchq, f)
	w.m.Unlock()
	_, _, _ = w32.User32PostThreadMessageW.Call(w.mainthread, w32.WMApp, 0, 0)
}

func (w *webview) Bind(name string, f interface{}) error {
	v := reflect.ValueOf(f)
	if v.Kind() != reflect.Func {
		return errors.New("only functions can be bound")
	}
	if n := v.Type().NumOut(); n > 2 {
		return errors.New("function may only return a value or a value+error")
	}
	w.m.Lock()
	w.bindings[name] = f
	w.m.Unlock()

	w.Init("(function() { var name = " + jsString(name) + ";" + `
		var RPC = window._rpc = (window._rpc || {nextSeq: 1});
		window[name] = function() {
		  var seq = RPC.nextSeq++;
		  var promise = new Promise(function(resolve, reject) {
			RPC[seq] = {
			  resolve: resolve,
			  reject: reject,
			};
		  });
		  window.external.invoke(JSON.stringify({
			id: seq,
			method: name,
			params: Array.prototype.slice.call(arguments),
		  }));
		  return promise;
		}
	})()`)

	return nil
}
