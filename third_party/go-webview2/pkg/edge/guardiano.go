//go:build windows
// +build windows

package edge

// Modifica di Tasso PDF: due "guardiani" della finestra.
//   - NavigationStarting / FrameNavigationStarting: la pagina e i suoi riquadri possono andare
//     solo agli indirizzi permessi (le pagine interne del programma); tutto il resto viene annullato.
//   - NewWindowRequested: nessuna finestra nuova (popup): la richiesta viene segnata come gestita.

import (
	"unsafe"

	"github.com/jchv/go-webview2/internal/w32"
	"golang.org/x/sys/windows"
)

type _guardianoVtbl struct {
	_IUnknownVtbl
	Invoke ComProc
}

type guardianoHandler struct {
	vtbl *_guardianoVtbl
	fn   func(sender, args uintptr) uintptr
}

func _guardianoQueryInterface(this *guardianoHandler, refiid, object uintptr) uintptr { return 0 }
func _guardianoAddRef(this *guardianoHandler) uintptr                                 { return 1 }
func _guardianoRelease(this *guardianoHandler) uintptr                                { return 1 }
func _guardianoInvoke(this *guardianoHandler, sender, args uintptr) uintptr {
	if this.fn == nil {
		return 0
	}
	return this.fn(sender, args)
}

var _guardianoFn = _guardianoVtbl{
	_IUnknownVtbl{
		NewComProc(_guardianoQueryInterface),
		NewComProc(_guardianoAddRef),
		NewComProc(_guardianoRelease),
	},
	NewComProc(_guardianoInvoke),
}

func newGuardiano(fn func(sender, args uintptr) uintptr) *guardianoHandler {
	return &guardianoHandler{vtbl: &_guardianoFn, fn: fn}
}

// ICoreWebView2NavigationStartingEventArgs: IUnknown, get_Uri, get_IsUserInitiated,
// get_IsRedirected, get_RequestHeaders, get_Cancel, put_Cancel, get_NavigationId.
type navStartingArgsVtbl struct {
	_IUnknownVtbl
	GetUri            ComProc
	GetIsUserInit     ComProc
	GetIsRedirected   ComProc
	GetRequestHeaders ComProc
	GetCancel         ComProc
	PutCancel         ComProc
	GetNavigationId   ComProc
}

type navStartingArgs struct {
	vtbl *navStartingArgsVtbl
}

// ICoreWebView2NewWindowRequestedEventArgs: IUnknown, get_Uri, put_NewWindow, get_NewWindow,
// put_Handled, get_Handled, get_IsUserInitiated, GetDeferral, get_WindowFeatures.
type newWindowArgsVtbl struct {
	_IUnknownVtbl
	GetUri          ComProc
	PutNewWindow    ComProc
	GetNewWindow    ComProc
	PutHandled      ComProc
	GetHandled      ComProc
	GetIsUserInit   ComProc
	GetDeferral     ComProc
	GetWindowFeatur ComProc
}

type newWindowArgs struct {
	vtbl *newWindowArgsVtbl
}

func leggiUri(proc ComProc, args uintptr) string {
	var p *uint16
	_, _, _ = proc.Call(args, uintptr(unsafe.Pointer(&p)))
	if p == nil {
		return ""
	}
	s := w32.Utf16PtrToString(p)
	windows.CoTaskMemFree(unsafe.Pointer(p))
	return s
}

// installaGuardiani: chiamata quando la WebView è pronta (vedi CreateCoreWebView2ControllerCompleted).
func (e *Chromium) installaGuardiani() {
	if e.NavigazionePermessa == nil {
		return
	}
	nav := func(_, args uintptr) uintptr {
		a := (*navStartingArgs)(unsafe.Pointer(args))
		uri := leggiUri(a.vtbl.GetUri, args)
		if !e.NavigazionePermessa(uri) {
			_, _, _ = a.vtbl.PutCancel.Call(args, 1)
		}
		return 0
	}
	e.guardNav = newGuardiano(nav)
	e.guardFrame = newGuardiano(nav)
	e.guardPopup = newGuardiano(func(_, args uintptr) uintptr {
		a := (*newWindowArgs)(unsafe.Pointer(args))
		uri := leggiUri(a.vtbl.GetUri, args)
		_, _, _ = a.vtbl.PutHandled.Call(args, 1) // niente finestra nuova, mai
		if e.PopupRichiesto != nil {
			go e.PopupRichiesto(uri)
		}
		return 0
	})
	var token _EventRegistrationToken
	_, _, _ = e.webview.vtbl.AddNavigationStarting.Call(uintptr(unsafe.Pointer(e.webview)), uintptr(unsafe.Pointer(e.guardNav)), uintptr(unsafe.Pointer(&token)))
	_, _, _ = e.webview.vtbl.AddFrameNavigationStarting.Call(uintptr(unsafe.Pointer(e.webview)), uintptr(unsafe.Pointer(e.guardFrame)), uintptr(unsafe.Pointer(&token)))
	_, _, _ = e.webview.vtbl.AddNewWindowRequested.Call(uintptr(unsafe.Pointer(e.webview)), uintptr(unsafe.Pointer(e.guardPopup)), uintptr(unsafe.Pointer(&token)))
}

// RispondiRisorsa: risponde a una richiesta della pagina con dati del programma (modifica di Tasso PDF).
func (e *Chromium) RispondiRisorsa(args *ICoreWebView2WebResourceRequestedEventArgs, body []byte, status int, reason, headers string) {
	if e.environment == nil {
		return
	}
	resp, err := e.environment.CreateWebResourceResponse(body, status, reason, headers)
	if err != nil || resp == nil {
		return
	}
	_ = args.PutResponse(resp)
	_, _, _ = resp.vtbl.Release.Call(uintptr(unsafe.Pointer(resp)))
}
