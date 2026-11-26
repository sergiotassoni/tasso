// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

//go:build windows

package esplora

import (
	"io/fs"
	"os"
	"syscall"
	"tassozip/rianalisi"
)

const (
	attrNascosto  = 0x2   // FILE_ATTRIBUTE_HIDDEN
	attrSistema   = 0x4   // FILE_ATTRIBUTE_SYSTEM
	attrRianalisi = 0x400 // FILE_ATTRIBUTE_REPARSE_POINT (giunzioni, collegamenti)
)

func attributi(fi fs.FileInfo) uint32 {
	if d, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok && d != nil {
		return d.FileAttributes
	}
	return 0
}

// nascosto: file nascosti o di sistema di Windows (come li nasconde Esplora file).
func nascosto(nome string, fi fs.FileInfo) bool {
	return attributi(fi)&(attrNascosto|attrSistema) != 0
}

// giunzione: cartella che in realtà porta altrove (giunzione, collegamento): non la percorro.
// Le cartelle di OneDrive ("File su richiesta") e quelle compresse da Windows sono punti di
// rianalisi anche loro, ma sono cartelle vere: quelle si percorrono.
func giunzione(p string) bool {
	fi, err := os.Lstat(p)
	if err != nil || fi.Mode()&fs.ModeSymlink != 0 {
		return true
	}
	if attributi(fi)&attrRianalisi == 0 && fi.Mode()&fs.ModeIrregular == 0 {
		return false
	}
	return rianalisi.PortaAltrove(p)
}
