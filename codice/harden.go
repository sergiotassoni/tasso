package main

// Tasso gira da amministratore, ma eredita l'ambiente dell'utente: le variabili in
// HKCU\Environment e le cartelle "dell'utente" le può cambiare qualsiasi programma senza
// privilegi. Qui rimetto le variabili che decidono DOVE stanno i programmi (Program Files,
// ProgramData, Path, moduli di PowerShell) ai valori veri di Windows, e chiedo a Windows di non
// seguire i collegamenti di cartella creati da utenti senza privilegi.

import (
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var pSetMitigation = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetProcessMitigationPolicy")

func ambienteSicuro() {
	cartelle := []struct {
		id   *windows.KNOWNFOLDERID
		nomi []string
	}{
		{windows.FOLDERID_ProgramFiles, []string{"ProgramFiles", "ProgramW6432"}},
		{windows.FOLDERID_ProgramFilesX86, []string{"ProgramFiles(x86)"}},
		{windows.FOLDERID_ProgramFilesCommon, []string{"CommonProgramFiles", "CommonProgramW6432"}},
		{windows.FOLDERID_ProgramFilesCommonX86, []string{"CommonProgramFiles(x86)"}},
		{windows.FOLDERID_ProgramData, []string{"ProgramData", "ALLUSERSPROFILE"}},
		{windows.FOLDERID_Public, []string{"PUBLIC"}},
	}
	for _, c := range cartelle {
		if p, err := windows.KnownFolderPath(c.id, 0); err == nil && p != "" {
			for _, n := range c.nomi {
				_ = os.Setenv(n, p)
			}
		}
	}
	// la cartella del profilo la sa Windows (ProfileList, solo amministratori), non la variabile
	if p, err := windows.GetCurrentProcessToken().GetUserProfileDirectory(); err == nil && p != "" {
		_ = os.Setenv("USERPROFILE", p)
	}
	// Path: solo quello di sistema. Le cartelle messe dall'utente davanti a System32 farebbero
	// partire da amministratore un programma qualsiasi con lo stesso nome
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`, registry.QUERY_VALUE); err == nil {
		if v, _, err := k.GetStringValue("Path"); err == nil && v != "" {
			if e, err := registry.ExpandString(v); err == nil && e != "" {
				v = e
			}
			_ = os.Setenv("Path", v)
		}
		k.Close()
	}
	// moduli di PowerShell solo dalle cartelle di sistema (non da Documenti)
	if sys, err := windows.GetSystemDirectory(); err == nil && sys != "" {
		_ = os.Setenv("PSModulePath", strings.Join([]string{
			filepath.Join(programFilesDir(), "WindowsPowerShell", "Modules"),
			filepath.Join(sys, "WindowsPowerShell", "v1.0", "Modules"),
		}, ";"))
	}
	// Redirection Guard (Windows 10 21H2+ / 11): i collegamenti di cartella creati da un utente
	// senza privilegi non vengono seguiti da Tasso. Sulle versioni vecchie non c'è: niente errore.
	flags := uint32(1)                                                             // EnforceRedirectionTrust
	pSetMitigation.Call(16, uintptr(unsafe.Pointer(&flags)), unsafe.Sizeof(flags)) // ProcessRedirectionTrustPolicy
}
