package main

import (
	"os"
	"path/filepath"
)

// openFolderOf apre Esplora file con il file già selezionato.
func openFolderOf(path string) string {
	if _, err := os.Stat(path); err != nil {
		return "no"
	}
	openPath(filepath.Join(os.Getenv("SystemRoot"), "explorer.exe"), "/select,\""+path+"\"")
	return "ok"
}
