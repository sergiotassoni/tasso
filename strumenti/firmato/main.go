// Firmato: controlla che gli exe abbiano la firma digitale dentro. Lo usa compila.sh con
// FIRMA=si (installatore da pubblicare). Esce con 1 se uno non è firmato.
//
//	go run ./strumenti/firmato file1.exe file2.exe ...
package main

import (
	"fmt"
	"os"

	"tassosetup/installa"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "uso: firmato file.exe ...")
		os.Exit(2)
	}
	ok := true
	for _, p := range os.Args[1:] {
		f, err := installa.HaFirma(p)
		switch {
		case err != nil:
			fmt.Println("ERRORE    ", p, err)
			ok = false
		case f:
			fmt.Println("firmato   ", p)
		default:
			fmt.Println("NON FIRMATO", p)
			ok = false
		}
	}
	if !ok {
		os.Exit(1)
	}
}
