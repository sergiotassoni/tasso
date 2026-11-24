// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

//go:build !windows && !linux

package archivio

import "os"

// Solo per compilare su altri sistemi (Tasso Zip è un programma per Windows).
func rinominaSeLibero(da, a string) error {
	if _, e := os.Lstat(a); e == nil {
		return errEsisteGia
	}
	return os.Rename(da, a)
}

func spazioLibero(cartella string) int64 { return -1 }

func codePageOEM() int { return 437 }
