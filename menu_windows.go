// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

//go:build windows

package main

// Voci "Comprimi con Tasso Zip" ed "Estrai con Tasso Zip" nel menu del tasto destro.
// Tutto in HKEY_CURRENT_USER\Software\Classes: vale solo per l'utente e NON servono i
// permessi da amministratore. Su Windows 11 le voci compaiono sotto "Mostra altre opzioni".

import (
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"tassozip/testi"
)

const (
	verboComprimi = "TassoZip.Comprimi"
	verboEstrai   = "TassoZip.Estrai"
)

// estensioniMenu: i file su cui compare "Estrai con Tasso Zip".
var estensioniMenu = []string{".zip", ".7z", ".rar", ".tar", ".gz", ".tgz", ".bz2", ".tbz2", ".tbz", ".xz", ".txz", ".001"}

// chiaviMenu: tutte le chiavi che Tasso Zip crea (verbo -> argomento della riga di comando).
func chiaviMenu() map[string]string {
	m := map[string]string{
		`Software\Classes\*\shell\` + verboComprimi:         "--comprimi",
		`Software\Classes\Directory\shell\` + verboComprimi: "--comprimi",
	}
	for _, e := range estensioniMenu {
		m[`Software\Classes\SystemFileAssociations\`+e+`\shell\`+verboEstrai] = "--estrai"
	}
	return m
}

func comando(exe, arg string) string { return `"` + exe + `" ` + arg + ` "%1"` }

// attivaMenu crea (o aggiorna) le voci nella lingua indicata.
func attivaMenu(l string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	var errs []error
	for chiave, arg := range chiaviMenu() {
		etichetta := testi.T(l, "menu_comprimi")
		if arg == "--estrai" {
			etichetta = testi.T(l, "menu_estrai")
		}
		k, _, err := registry.CreateKey(registry.CURRENT_USER, chiave, registry.SET_VALUE)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		_ = k.SetStringValue("MUIVerb", etichetta)
		_ = k.SetStringValue("Icon", `"`+exe+`",0`)
		k.Close()
		c, _, err := registry.CreateKey(registry.CURRENT_USER, chiave+`\command`, registry.SET_VALUE)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := c.SetStringValue("", comando(exe, arg)); err != nil {
			errs = append(errs, err)
		}
		c.Close()
	}
	avvisaExplorer()
	return errors.Join(errs...)
}

// togliMenu cancella solo le chiavi create da Tasso Zip.
func togliMenu() error {
	var errs []error
	for chiave := range chiaviMenu() {
		for _, k := range []string{chiave + `\command`, chiave} {
			if err := registry.DeleteKey(registry.CURRENT_USER, k); err != nil && !errors.Is(err, registry.ErrNotExist) && !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
				errs = append(errs, err)
			}
		}
	}
	avvisaExplorer()
	return errors.Join(errs...)
}

func leggiComando() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\*\shell\`+verboComprimi+`\command`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	s, _, _ := k.GetStringValue("")
	return s
}

func menuAttivo() bool { return leggiComando() != "" }

// menuPuntaA: le voci del menu avviano proprio questo exe?
func menuPuntaA(exe string) bool {
	return strings.EqualFold(leggiComando(), comando(exe, "--comprimi"))
}

var procSHChangeNotify = windows.NewLazySystemDLL("shell32.dll").NewProc("SHChangeNotify")

// avvisaExplorer: Esplora file rilegge le associazioni (SHCNE_ASSOCCHANGED).
func avvisaExplorer() {
	procSHChangeNotify.Call(0x08000000, 0, 0, 0)
}
