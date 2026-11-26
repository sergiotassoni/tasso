// Prepara: copia in pacco/file i programmi da incorporare in TassoSetup.exe e scrive
// pacco/file/elenco.json (nomi, dimensioni, impronte SHA-256, versioni).
//
// Uso (lo chiama compila.sh):
//
//	go run ./strumenti/prepara -versione 3.1 \
//	   -tasso /tmp/tb7/Tasso.exe -tasso-versione 3.1 -tasso-licenza ../../tasso/LICENSE \
//	   -tasso-avvisi ../../tasso/codice/dist \
//	   -zip-dir ../tassozip -zip-versione 1.1.0 -pdf-dir ../tassopdf -pdf-versione 1.0.0 \
//	   -disinstalla "build/Disinstalla Tasso.exe"
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"tassosetup/installa"
)

func main() {
	uscita := flag.String("o", "pacco/file", "cartella dei file da incorporare")
	versione := flag.String("versione", "", "versione dell'ecosistema (Programmi e funzionalità)")
	tasso := flag.String("tasso", "/tmp/tb7/Tasso.exe", "Tasso.exe da incorporare (la build firmata)")
	tassoVer := flag.String("tasso-versione", "", "versione di Tasso (finisce nel file VERSIONE)")
	tassoLic := flag.String("tasso-licenza", "", "LICENSE di Tasso")
	// LICENZE.txt e LICENSES.txt: gli avvisi dei componenti di terze parti dentro Tasso.exe (PresentMon,
	// caratteri, WebView2, wazero, ooz, Bergamot...). Le licenze MIT, BSD, OFL e Apache chiedono che
	// arrivino insieme al programma: senza, l'installatore non si prepara.
	tassoAvvisi := flag.String("tasso-avvisi", "../../tasso/codice/dist", "cartella con LICENZE.txt e LICENSES.txt di Tasso")
	zipDir := flag.String("zip-dir", "../tassozip", "repository di Tasso Zip")
	zipVer := flag.String("zip-versione", "", "versione di Tasso Zip")
	pdfDir := flag.String("pdf-dir", "../tassopdf", "repository di Tasso PDF")
	pdfVer := flag.String("pdf-versione", "", "versione di Tasso PDF")
	disinst := flag.String("disinstalla", "build/"+installa.NomeDisinstalla, "disinstallatore compilato")
	flag.Parse()
	if *versione == "" || *tassoVer == "" || *zipVer == "" || *pdfVer == "" {
		esci(fmt.Errorf("servono -versione, -tasso-versione, -zip-versione e -pdf-versione"))
	}
	// svuota la cartella (tranne LEGGIMI.txt)
	voci, _ := os.ReadDir(*uscita)
	for _, v := range voci {
		if v.Name() != "LEGGIMI.txt" {
			esci(os.RemoveAll(filepath.Join(*uscita, v.Name())))
		}
	}
	e := installa.Elenco{Versione: *versione}
	aggiungi := func(id installa.Programma, nome, ver, avvio string, file [][2]string, testi map[string]string) {
		p := installa.ProgElenco{ID: id, Nome: nome, Versione: ver, Avvio: avvio}
		dir := filepath.Join(*uscita, string(id))
		esci(os.MkdirAll(dir, 0o755))
		for _, f := range file { // [origine, nome dentro]
			fe, err := copia(f[0], filepath.Join(dir, f[1]))
			esci(err)
			fe.Nome = f[1]
			p.File = append(p.File, fe)
		}
		for n, t := range testi {
			esci(os.WriteFile(filepath.Join(dir, n), []byte(t), 0o644))
			fe, err := impronta(filepath.Join(dir, n))
			esci(err)
			fe.Nome = n
			p.File = append(p.File, fe)
		}
		e.Programmi = append(e.Programmi, p)
	}
	tf := [][2]string{{*tasso, "Tasso.exe"}}
	if *tassoLic != "" {
		tf = append(tf, [2]string{*tassoLic, "LICENSE"})
	}
	for _, n := range []string{"LICENZE.txt", "LICENSES.txt"} {
		da := filepath.Join(*tassoAvvisi, n)
		if fi, err := os.Stat(da); err != nil || !fi.Mode().IsRegular() || fi.Size() == 0 {
			esci(fmt.Errorf("manca %s (avvisi delle licenze di Tasso): passa -tasso-avvisi", da))
		}
		tf = append(tf, [2]string{da, n})
	}
	// VERSIONE: lo legge Tasso (selfupdate.go/taskExeCheck) per sapere che versione c'è in Programmi
	aggiungi(installa.ProgTasso, "Tasso", *tassoVer, "Tasso.exe", tf, map[string]string{"VERSIONE": *tassoVer})
	aggiungi(installa.ProgZip, "Tasso Zip", *zipVer, "TassoZip.exe", [][2]string{
		{filepath.Join(*zipDir, "dist", "TassoZip.exe"), "TassoZip.exe"},
		{filepath.Join(*zipDir, "LEGGIMI.txt"), "LEGGIMI.txt"},
		{filepath.Join(*zipDir, "LICENSE"), "LICENSE"},
		{filepath.Join(*zipDir, "LICENZE.txt"), "LICENZE.txt"},
	}, nil)
	aggiungi(installa.ProgPDF, "Tasso PDF", *pdfVer, "TassoPDF.exe", [][2]string{
		{filepath.Join(*pdfDir, "dist", "TassoPDF.exe"), "TassoPDF.exe"},
		{filepath.Join(*pdfDir, "LEGGIMI.txt"), "LEGGIMI.txt"},
		{filepath.Join(*pdfDir, "LICENSE"), "LICENSE"},
		{filepath.Join(*pdfDir, "LICENZE.txt"), "LICENZE.txt"},
	}, nil)
	aggiungi(installa.ProgSetup, "Disinstalla Tasso", *versione, installa.NomeDisinstalla,
		[][2]string{{*disinst, installa.NomeDisinstalla}}, nil)
	b, err := json.MarshalIndent(e, "", "  ")
	esci(err)
	esci(os.WriteFile(filepath.Join(*uscita, installa.NomeElenco), b, 0o644))
	for _, p := range e.Programmi {
		for _, f := range p.File {
			fmt.Printf("%-6s %-22s %10d  %s\n", p.ID, f.Nome, f.Dim, f.SHA256)
		}
	}
}

func copia(da, a string) (installa.FileElenco, error) {
	r, err := os.Open(da)
	if err != nil {
		return installa.FileElenco{}, err
	}
	defer r.Close()
	w, err := os.Create(a)
	if err != nil {
		return installa.FileElenco{}, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), r)
	if e := w.Close(); err == nil {
		err = e
	}
	return installa.FileElenco{SHA256: hex.EncodeToString(h.Sum(nil)), Dim: n}, err
}

func impronta(p string) (installa.FileElenco, error) {
	b, err := os.ReadFile(p)
	h := sha256.Sum256(b)
	return installa.FileElenco{SHA256: hex.EncodeToString(h[:]), Dim: int64(len(b))}, err
}

func esci(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "prepara:", err)
		os.Exit(1)
	}
}
