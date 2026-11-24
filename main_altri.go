// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

//go:build !windows

package main

import (
	"fmt"
	"os"
)

// Tasso Zip è un programma per Windows. Su altri sistemi si compilano solo le parti
// che comprimono ed estraggono (pacchetto archivio), per i test.
func main() {
	fmt.Fprintln(os.Stderr, "Tasso Zip è un programma per Windows. / Tasso Zip is a Windows program.")
	os.Exit(1)
}
