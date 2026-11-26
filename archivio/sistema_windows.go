// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

//go:build windows

package archivio

import (
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// rinominaSeLibero sposta da -> a solo se "a" non esiste ancora (mai sovrascrivere).
// MoveFileEx senza MOVEFILE_REPLACE_EXISTING: se "a" esiste, Windows rifiuta.
func rinominaSeLibero(da, a string) error {
	pda, err := windows.UTF16PtrFromString(da)
	if err != nil {
		return err
	}
	pa, err := windows.UTF16PtrFromString(a)
	if err != nil {
		return err
	}
	err = windows.MoveFileEx(pda, pa, 0)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_FILE_EXISTS) {
		return errEsisteGia
	}
	return err
}

// spazioLibero: byte liberi per l'utente sul disco della cartella (-1 se non si sa).
func spazioLibero(cartella string) int64 {
	p, err := windows.UTF16PtrFromString(cartella)
	if err != nil {
		return -1
	}
	var libero, totale, liberoTot uint64
	if err := windows.GetDiskFreeSpaceEx(p, &libero, &totale, &liberoTot); err != nil {
		return -1
	}
	if libero > 1<<62 {
		return 1 << 62
	}
	return int64(libero)
}

// codePageOEM: la tabella di caratteri "OEM" di Windows, usata dai vecchi zip per i nomi.
func codePageOEM() int {
	r, _, _ := procGetOEMCP.Call()
	return int(r)
}

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGetOEMCP             = kernel32.NewProc("GetOEMCP")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
)

// memoriaLibera: byte di RAM disponibili (0 se non si sa), con GlobalMemoryStatusEx.
func memoriaLibera() int64 {
	var m struct {
		Length, MemoryLoad                  uint32
		TotalPhys, AvailPhys, TotalPageFile uint64
		AvailPageFile, TotalVirtual         uint64
		AvailVirtual, AvailExtendedVirtual  uint64
	}
	m.Length = uint32(unsafe.Sizeof(m))
	r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m)))
	if r == 0 || m.AvailPhys > 1<<62 {
		return 0
	}
	return int64(m.AvailPhys)
}

// leggiZonaSistema: il "marchio del web" (flusso :Zone.Identifier) del file, nil se non c'è.
func leggiZonaSistema(p string) []byte {
	b, err := os.ReadFile(p + ":Zone.Identifier")
	if err != nil || len(b) == 0 || len(b) > 64<<10 {
		return nil
	}
	return b
}

// scriviZonaSistema mette il marchio del web sul file (così SmartScreen e Office avvisano).
func scriviZonaSistema(p string, b []byte) {
	_ = os.WriteFile(p+":Zone.Identifier", b, 0o644)
}

// nascondi: la cartella o il file di lavoro non si vede in Esplora file mentre lavoro.
func nascondi(p string) {
	if u, err := windows.UTF16PtrFromString(p); err == nil {
		if a, err := windows.GetFileAttributes(u); err == nil {
			_ = windows.SetFileAttributes(u, a|windows.FILE_ATTRIBUTE_HIDDEN)
		}
	}
}

// mostra toglie l'attributo "nascosto" messo da nascondi.
func mostra(p string) {
	if u, err := windows.UTF16PtrFromString(p); err == nil {
		if a, err := windows.GetFileAttributes(u); err == nil && a&windows.FILE_ATTRIBUTE_HIDDEN != 0 {
			_ = windows.SetFileAttributes(u, a&^windows.FILE_ATTRIBUTE_HIDDEN)
		}
	}
}
