//go:build disinstallatore

package pacco

import (
	"embed"
	"io/fs"
)

// Pieno: false nel disinstallatore (niente programmi dentro).
const Pieno = false

var vuoto embed.FS

// Sorgente: vuota.
func Sorgente() fs.FS { return vuoto }
