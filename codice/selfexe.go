package main

import (
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// Il proprio Tasso.exe resta aperto in sola lettura, senza permettere ad altri di scriverlo,
// rinominarlo o cancellarlo: così, se Tasso gira da Download, un programma senza privilegi non può
// sostituirlo mentre è aperto e farsi copiare in Programmi (dove partono le attività da amministratore).
var (
	selfMu  sync.Mutex
	selfExe *os.File
)

// lockSelfExe blocca il file e dice se ci è riuscito (riprova per un secondo: un antivirus può
// tenerlo aperto un attimo).
func lockSelfExe() bool {
	selfMu.Lock()
	defer selfMu.Unlock()
	if selfExe != nil {
		return true
	}
	if exePath == "" {
		return false
	}
	p, err := windows.UTF16PtrFromString(exePath)
	if err != nil {
		return false
	}
	for i := 0; i < 5; i++ {
		h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
		if err == nil {
			selfExe = os.NewFile(uintptr(h), exePath)
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// unlockSelfExe: prima di sostituire Tasso.exe con l'aggiornamento.
func unlockSelfExe() {
	selfMu.Lock()
	defer selfMu.Unlock()
	if selfExe != nil {
		_ = selfExe.Close()
		selfExe = nil
	}
}

// readSelfExe: i byte del Tasso.exe aperto, SOLO dal file bloccato. Senza blocco non leggo dal
// disco: il file potrebbe essere stato sostituito, e da qui finisce in Programmi o viene avviato.
func readSelfExe() ([]byte, error) {
	selfMu.Lock()
	defer selfMu.Unlock()
	if selfExe == nil {
		return nil, errors.New("Tasso.exe non bloccato")
	}
	fi, err := selfExe.Stat()
	if err != nil {
		return nil, err
	}
	b := make([]byte, fi.Size()) // misura esatta: niente buffer che raddoppiano (l'exe è ~40 MB)
	if _, err := io.ReadFull(io.NewSectionReader(selfExe, 0, fi.Size()), b); err != nil {
		return nil, err
	}
	return b, nil
}

// programFilesDir: la vera cartella Programmi. Non uso la variabile d'ambiente ProgramFiles, che
// un programma senza privilegi può cambiare (HKCU\Environment) e che Tasso, da amministratore, erediterebbe.
func programFilesDir() string {
	if p, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0); err == nil && p != "" {
		return p
	}
	return `C:\Program Files` // mai la variabile d'ambiente (un programma senza privilegi la può cambiare)
}
