//go:build !windows

package traduce

import (
	"os"
	"path/filepath"
)

// Fuori da Windows (solo i test) non c'è GetFinalPathNameByHandle: la radice è il percorso pulito
// (rifiutata se è un collegamento) e per i file bastano i controlli con Lstat e os.SameFile.
func radiceVera(p string) (string, error) {
	fi, err := os.Lstat(p)
	if err != nil {
		return "", err
	}
	if fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 || !fi.IsDir() {
		return "", errLink
	}
	return filepath.Abs(p)
}

func controllaAperto(f *os.File, radice string, unNome bool) error { return nil }
