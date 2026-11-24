//go:build windows
// +build windows

package edge

// Modifica di Tasso Zip: file trascinati nella finestra.
//
// La pagina, quando lasci cadere dei file, chiama
// chrome.webview.postMessageWithAdditionalObjects("tassozip:drop", files).
// Qui leggo i percorsi veri dei file (ICoreWebView2File.Path) e li passo a DropHook.
// Serve WebView2 Runtime 113 o più nuovo; se è più vecchio non trovo l'interfaccia e
// non succede niente (si possono sempre usare i pulsanti "Scegli").

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DropMessage è il messaggio che la pagina manda insieme ai file trascinati.
const DropMessage = "tassozip:drop"

// DropHook riceve i percorsi dei file trascinati nella finestra (nel filo della finestra:
// deve solo passarli avanti, senza lavori lunghi).
var DropHook func(paths []string)

var iidWebMessageReceivedEventArgs2 = NewGUID("{06fc7ab7-c90c-4297-9389-33ca01cf6d5e}")

type webMessageReceivedEventArgs2Vtbl struct {
	_IUnknownVtbl
	GetSource                ComProc
	GetWebMessageAsJSON      ComProc
	TryGetWebMessageAsString ComProc
	GetAdditionalObjects     ComProc
}

type webMessageReceivedEventArgs2 struct {
	vtbl *webMessageReceivedEventArgs2Vtbl
}

type objectCollectionViewVtbl struct {
	_IUnknownVtbl
	GetCount        ComProc
	GetValueAtIndex ComProc
}

type objectCollectionView struct {
	vtbl *objectCollectionViewVtbl
}

type coreWebView2FileVtbl struct {
	_IUnknownVtbl
	GetPath ComProc
}

type coreWebView2File struct {
	vtbl *coreWebView2FileVtbl
}

// handleDrop: true se il messaggio era quello dei file trascinati (anche se non sono
// riuscito a leggere i percorsi): in quel caso non va passato alle funzioni legate con Bind.
func handleDrop(message string, args *iCoreWebView2WebMessageReceivedEventArgs) bool {
	if !strings.HasPrefix(message, DropMessage) {
		return false
	}
	if args == nil || DropHook == nil {
		return true
	}
	var a2 *webMessageReceivedEventArgs2
	hr, _, _ := args.vtbl.QueryInterface.Call(
		uintptr(unsafe.Pointer(args)),
		uintptr(unsafe.Pointer(iidWebMessageReceivedEventArgs2)),
		uintptr(unsafe.Pointer(&a2)),
	)
	if hr != 0 || a2 == nil {
		return true // WebView2 troppo vecchio: niente percorsi
	}
	defer a2.vtbl.Release.Call(uintptr(unsafe.Pointer(a2)))

	var coll *objectCollectionView
	hr, _, _ = a2.vtbl.GetAdditionalObjects.Call(uintptr(unsafe.Pointer(a2)), uintptr(unsafe.Pointer(&coll)))
	if hr != 0 || coll == nil {
		return true
	}
	defer coll.vtbl.Release.Call(uintptr(unsafe.Pointer(coll)))

	var count uint32
	hr, _, _ = coll.vtbl.GetCount.Call(uintptr(unsafe.Pointer(coll)), uintptr(unsafe.Pointer(&count)))
	if hr != 0 {
		return true
	}
	if count > 10000 {
		count = 10000
	}
	var paths []string
	for i := uint32(0); i < count; i++ {
		var f *coreWebView2File
		hr, _, _ = coll.vtbl.GetValueAtIndex.Call(uintptr(unsafe.Pointer(coll)), uintptr(i), uintptr(unsafe.Pointer(&f)))
		if hr != 0 || f == nil {
			continue
		}
		var p *uint16
		hr, _, _ = f.vtbl.GetPath.Call(uintptr(unsafe.Pointer(f)), uintptr(unsafe.Pointer(&p)))
		if hr == 0 && p != nil {
			paths = append(paths, windows.UTF16PtrToString(p))
			windows.CoTaskMemFree(unsafe.Pointer(p))
		}
		f.vtbl.Release.Call(uintptr(unsafe.Pointer(f)))
	}
	if len(paths) > 0 {
		DropHook(paths)
	}
	return true
}
