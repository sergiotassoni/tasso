//go:build windows

// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

// Il sottomenu "Tasso PDF" nel menu del tasto destro (come in Tasso Zip).
// Tutto in HKEY_CURRENT_USER\Software\Classes: vale solo per l'utente e NON servono i permessi
// da amministratore. Su Windows 11 compare sotto "Mostra altre opzioni".
// Sui PDF: Apri / Unisci / Firma / Sblocca / Comprimi. Sulle foto JPG e PNG: Unisci.

import (
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const verboMenu = "TassoPDF"

var vociPDF = []string{"apri", "unisci", "firma", "sblocca", "comprimi"}

func radiciMenu() map[string][]string {
	m := map[string][]string{`Software\Classes\SystemFileAssociations\.pdf\shell\` + verboMenu: vociPDF}
	for _, e := range []string{".jpg", ".jpeg", ".png"} {
		m[`Software\Classes\SystemFileAssociations\`+e+`\shell\`+verboMenu] = []string{"unisci"}
	}
	return m
}

func comandoMenu(exe, modo string) string { return `"` + exe + `" --` + modo + ` "%1"` }

func attivaMenu(l string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	var errs []error
	for radice, voci := range radiciMenu() {
		_ = cancellaAlbero(registry.CURRENT_USER, radice)
		k, _, err := registry.CreateKey(registry.CURRENT_USER, radice, registry.SET_VALUE)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		_ = k.SetStringValue("MUIVerb", "Tasso PDF")
		_ = k.SetStringValue("Icon", `"`+exe+`",0`)
		_ = k.SetStringValue("SubCommands", "")
		k.Close()
		for i, v := range voci {
			chiave := radice + `\shell\` + string(rune('1'+i)) + v
			c, _, err := registry.CreateKey(registry.CURRENT_USER, chiave, registry.SET_VALUE)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			_ = c.SetStringValue("MUIVerb", testoMenu(l, v))
			_ = c.SetStringValue("Icon", `"`+exe+`",0`)
			c.Close()
			c, _, err = registry.CreateKey(registry.CURRENT_USER, chiave+`\command`, registry.SET_VALUE)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if err := c.SetStringValue("", comandoMenu(exe, v)); err != nil {
				errs = append(errs, err)
			}
			c.Close()
		}
	}
	avvisaExplorer()
	return errors.Join(errs...)
}

// togliMenu cancella solo le chiavi create da Tasso PDF.
func togliMenu() error {
	var errs []error
	for radice := range radiciMenu() {
		if err := cancellaAlbero(registry.CURRENT_USER, radice); err != nil {
			errs = append(errs, err)
		}
	}
	avvisaExplorer()
	return errors.Join(errs...)
}

func cancellaAlbero(base registry.Key, percorso string) error {
	k, err := registry.OpenKey(base, percorso, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) || errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return nil
		}
		return err
	}
	figli, _ := k.ReadSubKeyNames(-1)
	k.Close()
	for _, f := range figli {
		if err := cancellaAlbero(base, percorso+`\`+f); err != nil {
			return err
		}
	}
	err = registry.DeleteKey(base, percorso)
	if err != nil && !errors.Is(err, registry.ErrNotExist) && !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return err
	}
	return nil
}

func leggiComandoMenu() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\SystemFileAssociations\.pdf\shell\`+verboMenu+`\shell\1apri\command`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	s, _, _ := k.GetStringValue("")
	return s
}

func menuAttivoWin() bool { return leggiComandoMenu() != "" }

// menuPuntaA: le voci avviano proprio questo exe?
func menuPuntaA(exe string) bool {
	return strings.EqualFold(leggiComandoMenu(), comandoMenu(exe, "apri"))
}

func impostaMenuWin(on bool, l string) error {
	if on {
		return attivaMenu(l)
	}
	return togliMenu()
}

var procSHChangeNotify = windows.NewLazySystemDLL("shell32.dll").NewProc("SHChangeNotify")

// avvisaExplorer: Esplora file rilegge le associazioni (SHCNE_ASSOCCHANGED).
func avvisaExplorer() { procSHChangeNotify.Call(0x08000000, 0, 0, 0) }

func init() {
	piattaforma.menuAttivo = menuAttivoWin
	piattaforma.impostaMenu = impostaMenuWin
}
