//go:build !disinstallatore

// Package pacco: i file dei programmi incorporati in TassoSetup.exe (go:embed).
// La cartella file/ la riempie strumenti/prepara (vedi compila.sh). Il disinstallatore
// si compila con -tags disinstallatore e non contiene i programmi (pacco_vuoto.go).
package pacco

import (
	"embed"
	"io/fs"
)

//go:embed all:file
var file embed.FS

// Pieno: true in TassoSetup.exe, false nel disinstallatore.
const Pieno = true

// Sorgente: i file, con elenco.json in cima.
func Sorgente() fs.FS {
	s, err := fs.Sub(file, "file")
	if err != nil {
		panic(err)
	}
	return s
}
