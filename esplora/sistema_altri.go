// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

//go:build !windows

package esplora

import (
	"io/fs"
	"os"
	"strings"
)

// nascosto: fuori da Windows (solo per i test) i file che cominciano con il punto.
func nascosto(nome string, fi fs.FileInfo) bool { return strings.HasPrefix(nome, ".") }

func giunzione(p string) bool {
	fi, err := os.Lstat(p)
	return err != nil || fi.Mode()&fs.ModeSymlink != 0
}
