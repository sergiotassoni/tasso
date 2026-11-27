//go:build windows

package traduce

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/windows"
)

// I motori (IA e voce) partono da amministratore da una cartella in AppData, dove può scrivere
// anche un programma senza privilegi. Prima di avviarli apro ogni .exe e .dll bloccandoli in
// scrittura (nessuno li può cambiare, rinominare o cancellare finché il motore gira), e
// controllo l'impronta leggendo proprio dal file bloccato, sempre (niente risultato ricordato).
var (
	muBloccati sync.Mutex
	bloccati   = map[string][]windows.Handle{}
)

func verificaEBloccaMotore(dir string, impronte map[string]string, exe string) error {
	sbloccaMotore(dir)
	if st, err := os.Lstat(dir); err != nil || st.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return fmt.Errorf("la cartella del motore non va bene: non lo avvio")
	}
	voci, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var presi []windows.Handle
	fallito := func(e error) error {
		for _, h := range presi {
			windows.CloseHandle(h)
		}
		return e
	}
	trovatoExe := false
	for _, v := range voci {
		nome := v.Name()
		b := strings.ToLower(nome)
		if v.IsDir() || !(strings.HasSuffix(b, ".exe") || strings.HasSuffix(b, ".dll")) {
			continue
		}
		atteso, ok := impronte[nome]
		if !ok {
			return fallito(fmt.Errorf("nella cartella del motore c'è un file sconosciuto (%s): non lo avvio", nome))
		}
		p, _ := windows.UTF16PtrFromString(filepath.Join(dir, nome))
		// FILE_FLAG_OPEN_REPARSE_POINT: un collegamento non lo seguo (e poi non torna l'impronta)
		h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err != nil {
			return fallito(fmt.Errorf("il file %s del motore è in uso o non si apre: non lo avvio", nome))
		}
		presi = append(presi, h)
		f := os.NewFile(uintptr(h), nome)
		hs := sha256.New()
		_, err = io.Copy(hs, f)
		if err != nil || !strings.EqualFold(hex.EncodeToString(hs.Sum(nil)), atteso) {
			return fallito(fmt.Errorf("il file %s del motore è stato cambiato: non lo avvio (riscaricalo)", nome))
		}
		if nome == exe {
			trovatoExe = true
		}
	}
	if !trovatoExe {
		return fallito(fmt.Errorf("manca %s", filepath.Join(dir, exe)))
	}
	muBloccati.Lock()
	bloccati[dir] = presi
	muBloccati.Unlock()
	return nil
}

// sbloccaMotore: il motore è fermo, i suoi file si possono di nuovo cambiare (aggiornamento).
func sbloccaMotore(dir string) {
	muBloccati.Lock()
	hs := bloccati[dir]
	delete(bloccati, dir)
	muBloccati.Unlock()
	for _, h := range hs {
		windows.CloseHandle(h)
	}
}
