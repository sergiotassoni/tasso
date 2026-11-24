// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

//go:build windows

package archivio

import (
	"errors"

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

var procGetOEMCP = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetOEMCP")
