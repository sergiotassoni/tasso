// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

//go:build windows

package rianalisi

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// collegamentoSistema legge l'etichetta del punto di rianalisi con FindFirstFile (non apre il
// file: un file di OneDrive solo online non viene scaricato). Nel dubbio: collegamento.
func collegamentoSistema(p string) bool {
	q := p
	if a, err := filepath.Abs(p); err == nil && len(a) >= 248 && !strings.HasPrefix(a, `\\`) {
		q = `\\?\` + a // percorsi lunghi
	}
	u, err := windows.UTF16PtrFromString(q)
	if err != nil {
		return true
	}
	var fd windows.Win32finddata
	h, err := windows.FindFirstFile(u, &fd)
	if err != nil {
		return true
	}
	windows.FindClose(h)
	if fd.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0 {
		return false
	}
	return collegamentoEtichetta(fd.Reserved0)
}
