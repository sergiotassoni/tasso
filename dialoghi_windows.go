// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

//go:build windows

package main

// Le finestre di Windows "Apri" (IFileOpenDialog), per scegliere file, cartelle o archivi.
// Devono girare nel filo della finestra (vedi webview2.SuFinestra in main_windows.go).

import (
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	clsidFileOpenDialog = windows.GUID{Data1: 0xDC1C5A9C, Data2: 0xE88A, Data3: 0x4DDE, Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidIFileOpenDialog  = windows.GUID{Data1: 0xD57C7288, Data2: 0xD4AD, Data3: 0x4768, Data4: [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}

	ole32                = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
)

const (
	fosNoChangeDir      = 0x8
	fosPickFolders      = 0x20
	fosForceFileSystem  = 0x40
	fosAllowMultiSelect = 0x200
	fosPathMustExist    = 0x800
	fosFileMustExist    = 0x1000

	sigdnFileSysPath = 0x80058000
	clsctxInproc     = 0x1
)

// Posizioni nella tabella dei metodi (vtable) delle interfacce COM usate.
const (
	vRelease         = 2
	vShow            = 3
	vSetFileTypes    = 4
	vSetOptions      = 9
	vGetOptions      = 10
	vSetTitle        = 17
	vGetResults      = 27 // IFileOpenDialog
	vItemGetDispName = 5  // IShellItem::GetDisplayName
	vArrayGetCount   = 7  // IShellItemArray::GetCount
	vArrayGetItemAt  = 8  // IShellItemArray::GetItemAt
)

type comObj struct{ vtbl *[64]uintptr }

func (o *comObj) call(i int, args ...uintptr) uintptr {
	a := append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)
	r, _, _ := syscall.SyscallN(o.vtbl[i], a...)
	return r
}

func (o *comObj) release() { o.call(vRelease) }

type filtro struct {
	nome, schema string
}

// scegli apre la finestra "Apri" di Windows. Restituisce i percorsi scelti (nil se annulli).
func scegli(titolo string, cartelle, multipli bool, filtri []filtro) []string {
	var d *comObj
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidFileOpenDialog)), 0, clsctxInproc,
		uintptr(unsafe.Pointer(&iidIFileOpenDialog)), uintptr(unsafe.Pointer(&d)))
	if hr != 0 || d == nil {
		registraErrore("finestra Apri: CoCreateInstance 0x%x", hr)
		return nil
	}
	defer d.release()

	var opz uint32
	d.call(vGetOptions, uintptr(unsafe.Pointer(&opz)))
	opz |= fosNoChangeDir | fosForceFileSystem | fosPathMustExist
	if cartelle {
		opz |= fosPickFolders
	} else {
		opz |= fosFileMustExist
	}
	if multipli {
		opz |= fosAllowMultiSelect
	}
	d.call(vSetOptions, uintptr(opz))
	if t, err := windows.UTF16PtrFromString(titolo); err == nil {
		d.call(vSetTitle, uintptr(unsafe.Pointer(t)))
	}
	type spec struct{ nome, schema *uint16 }
	var specs []spec
	defer runtime.KeepAlive(&specs) // i testi dei filtri devono restare vivi finché la finestra è aperta
	if len(filtri) > 0 && !cartelle {
		specs = make([]spec, 0, len(filtri))
		for _, f := range filtri {
			n, _ := windows.UTF16PtrFromString(f.nome)
			s, _ := windows.UTF16PtrFromString(f.schema)
			specs = append(specs, spec{n, s})
		}
		d.call(vSetFileTypes, uintptr(len(specs)), uintptr(unsafe.Pointer(&specs[0])))
	}

	var hwnd uintptr
	if finestra != nil {
		hwnd = uintptr(finestra.Window())
	}
	if hr := d.call(vShow, hwnd); hr != 0 {
		return nil // annullato (0x800704C7) o errore
	}
	var arr *comObj
	if hr := d.call(vGetResults, uintptr(unsafe.Pointer(&arr))); hr != 0 || arr == nil {
		return nil
	}
	defer arr.release()
	var n uint32
	arr.call(vArrayGetCount, uintptr(unsafe.Pointer(&n)))
	var out []string
	for i := uint32(0); i < n && i < 10000; i++ {
		var it *comObj
		if hr := arr.call(vArrayGetItemAt, uintptr(i), uintptr(unsafe.Pointer(&it))); hr != 0 || it == nil {
			continue
		}
		var p *uint16
		if hr := it.call(vItemGetDispName, sigdnFileSysPath, uintptr(unsafe.Pointer(&p))); hr == 0 && p != nil {
			out = append(out, windows.UTF16PtrToString(p))
			windows.CoTaskMemFree(unsafe.Pointer(p))
		}
		it.release()
	}
	return out
}
