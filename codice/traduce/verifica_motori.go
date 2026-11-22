package traduce

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// verificaMotore: prima di avviare un motore controllo che nella sua cartella ci siano solo
// programmi e librerie conosciuti, uguali a quelli dello zip ufficiale (impronte in
// impronte_motori.go). Un file estraneo o cambiato = motore non avviato.
func verificaMotore(dir string, impronte map[string]string, exe string) error {
	voci, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	trovatoExe := false
	for _, v := range voci {
		nome := v.Name()
		b := strings.ToLower(nome)
		if v.IsDir() || !(strings.HasSuffix(b, ".exe") || strings.HasSuffix(b, ".dll")) {
			continue
		}
		h, ok := impronte[nome]
		if !ok {
			return fmt.Errorf("nella cartella del motore c'è un file sconosciuto (%s): non lo avvio", nome)
		}
		if !presente(dir, FileDaScaricare{Nome: nome, SHA256: h}) {
			return fmt.Errorf("il file %s del motore è stato cambiato: non lo avvio (riscaricalo)", nome)
		}
		if nome == exe {
			trovatoExe = true
		}
	}
	if !trovatoExe {
		return fmt.Errorf("manca %s", filepath.Join(dir, exe))
	}
	return nil
}
