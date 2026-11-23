// esplora-pak: elenca le voci di un pak (anche ricostruito in parte) con posizione e grandezza.
package main

import (
	"fmt"
	"os"
	"strings"

	"tasso/traduce/unreal"
)

func main() {
	p, err := unreal.ApriPak(os.Args[1])
	if err != nil {
		fmt.Println("ERRORE", err)
		return
	}
	filtro := ""
	if len(os.Args) > 2 {
		filtro = strings.ToLower(os.Args[2])
	}
	for _, v := range p.Voci {
		if filtro == "" || strings.Contains(strings.ToLower(v.Nome), filtro) {
			fmt.Printf("%s\t%d\t%d\t%d\t%d\n", v.Nome, v.Offset, v.Size, v.Uncomp, v.Compr)
		}
	}
}
