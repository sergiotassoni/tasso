//go:build !windows

package main

import (
	"fmt"
	"os"
)

// Tasso Setup funziona solo su Windows. Su Linux si provano la logica (installa/),
// i testi (testi/) e la pagina (ui/) con "go test ./...".
func main() {
	fmt.Fprintln(os.Stderr, "Tasso Setup funziona solo su Windows (go test ./... prova la logica).")
	os.Exit(1)
}
