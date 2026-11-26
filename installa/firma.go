package installa

import (
	"debug/pe"
	"fmt"
)

// HaFirma dice se l'exe ha una firma Authenticode dentro (la directory "sicurezza" del PE non è
// vuota). Non dice se è valida (quello lo fa signtool verify su Windows): serve a compila.sh per
// non incorporare per sbaglio un exe non firmato in un installatore da pubblicare.
func HaFirma(percorso string) (bool, error) {
	f, err := pe.Open(percorso)
	if err != nil {
		return false, fmt.Errorf("%s non è un exe di Windows: %w", percorso, err)
	}
	defer f.Close()
	const sicurezza = 4 // IMAGE_DIRECTORY_ENTRY_SECURITY
	switch h := f.OptionalHeader.(type) {
	case *pe.OptionalHeader64:
		return h.NumberOfRvaAndSizes > sicurezza && h.DataDirectory[sicurezza].Size > 0, nil
	case *pe.OptionalHeader32:
		return h.NumberOfRvaAndSizes > sicurezza && h.DataDirectory[sicurezza].Size > 0, nil
	}
	return false, fmt.Errorf("%s: intestazione PE sconosciuta", percorso)
}
