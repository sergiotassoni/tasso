// Impronta: scrive dist/TassoSetup.sha256 (vedi installa/manifesto.go), il file che Tasso legge per
// sapere se c'è un aggiornamento dell'ecosistema. Lo chiama compila.sh; dopo aver FIRMATO
// TassoSetup.exe va rifatto (la firma cambia l'impronta):
//
//	go run ./strumenti/impronta
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"

	"tassosetup/installa"
)

func main() {
	exe := flag.String("exe", "dist/"+installa.NomeSetupPubblicato, "installatore da pubblicare")
	pacco := flag.String("pacco", "pacco/file", "cartella con elenco.json (quella usata per compilare)")
	uscita := flag.String("o", "dist/"+installa.NomeManifesto, "file da scrivere")
	flag.Parse()
	e, err := installa.LeggiElenco(os.DirFS(*pacco))
	esci(err)
	f, err := os.Open(*exe)
	esci(err)
	h := sha256.New()
	_, err = io.Copy(h, f)
	f.Close()
	esci(err)
	m, err := installa.Manifesto(hex.EncodeToString(h.Sum(nil)), e)
	esci(err)
	esci(os.WriteFile(*uscita, []byte(m), 0o644))
	fmt.Print(m)
}

func esci(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "impronta:", err)
		os.Exit(1)
	}
}
