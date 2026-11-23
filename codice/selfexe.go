package main

import (
	"io"
	"os"
	"sync"

	"golang.org/x/sys/windows"
)

// Il proprio Tasso.exe resta aperto in sola lettura, senza permettere ad altri di scriverlo,
// rinominarlo o cancellarlo: così, se Tasso gira da Download, un programma senza privilegi non può
// sostituirlo mentre è aperto e farsi copiare in Programmi (dove partono le attività da amministratore).
var (
	selfMu  sync.Mutex
	selfExe *os.File
)

func lockSelfExe() {
	selfMu.Lock()
	defer selfMu.Unlock()
	if selfExe != nil || exePath == "" {
		return
	}
	p, err := windows.UTF16PtrFromString(exePath)
	if err != nil {
		return
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return // niente blocco: si legge come prima
	}
	selfExe = os.NewFile(uintptr(h), exePath)
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

// readSelfExe: i byte del Tasso.exe aperto (dal file bloccato, se c'è).
func readSelfExe() ([]byte, error) {
	selfMu.Lock()
	defer selfMu.Unlock()
	if selfExe == nil {
		return os.ReadFile(exePath)
	}
	fi, err := selfExe.Stat()
	if err != nil {
		return nil, err
	}
	return io.ReadAll(io.NewSectionReader(selfExe, 0, fi.Size()))
}

// programFilesDir: la vera cartella Programmi. Non uso la variabile d'ambiente ProgramFiles, che
// un programma senza privilegi può cambiare (HKCU\Environment) e che Tasso, da amministratore, erediterebbe.
func programFilesDir() string {
	if p, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0); err == nil && p != "" {
		return p
	}
	return os.Getenv("ProgramFiles")
}
