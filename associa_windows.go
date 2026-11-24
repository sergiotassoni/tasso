// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

//go:build windows

package main

// "Apri gli archivi con doppio clic con Tasso Zip": solo se l'utente accende l'interruttore
// (mai in silenzio). Tutto in HKEY_CURRENT_USER (niente amministratore):
//   - il tipo di file TassoZip.Archivio (icona e comando "--apri");
//   - Tasso Zip fra i programmi "Apri con" di ogni estensione (OpenWithProgids);
//   - il programma predefinito per le estensioni che per l'utente non ne hanno già uno
//     scelto in HKCU (ce lo segniamo, per rimettere tutto com'era quando si spegne);
//   - Tasso Zip fra le "App predefinite" di Windows (Capabilities, RegisteredApplications).
// Windows 10 e 11 non lasciano a nessun programma cambiare da solo la scelta fatta
// dall'utente con "Apri con > Usa sempre questa app" (UserChoice): in quel caso Windows
// chiede all'utente, o si sceglie Tasso Zip da "Apri con".

import (
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"

	"tassozip/testi"
)

const (
	progID       = "TassoZip.Archivio"
	chiaveCapab  = `Software\Tasso\TassoZip\Capabilities`
	nomeRegApp   = "Tasso Zip"
	chiaveAppExe = `Software\Classes\Applications\TassoZip.exe`
)

var estensioniAssoc = []string{".zip", ".7z", ".rar", ".tar", ".gz", ".tgz", ".bz2", ".tbz2", ".xz", ".txz", ".zst", ".tzst"}

func comandoApri(exe string) string { return `"` + exe + `" --apri "%1"` }

// attivaAssociazioni registra Tasso Zip per aprire gli archivi. Restituisce le estensioni
// di cui è diventato il programma predefinito (da salvare nelle impostazioni).
func attivaAssociazioni(l string) ([]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	var errs []error
	imposta := func(chiave string, valori map[string]string) {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, chiave, registry.SET_VALUE)
		if err != nil {
			errs = append(errs, err)
			return
		}
		defer k.Close()
		for n, v := range valori {
			if err := k.SetStringValue(n, v); err != nil {
				errs = append(errs, err)
			}
		}
	}
	descr := testi.T(l, "assoc_tipo")
	imposta(`Software\Classes\`+progID, map[string]string{"": descr, "FriendlyTypeName": descr})
	imposta(`Software\Classes\`+progID+`\DefaultIcon`, map[string]string{"": `"` + exe + `",0`})
	imposta(`Software\Classes\`+progID+`\shell`, map[string]string{"": "open"})
	imposta(`Software\Classes\`+progID+`\shell\open`, map[string]string{"MUIVerb": testi.T(l, "menu_apri"), "Icon": `"` + exe + `",0`})
	imposta(`Software\Classes\`+progID+`\shell\open\command`, map[string]string{"": comandoApri(exe)})
	imposta(chiaveAppExe+`\shell\open\command`, map[string]string{"": comandoApri(exe)})
	imposta(chiaveAppExe, map[string]string{"FriendlyAppName": "Tasso Zip"})
	imposta(chiaveCapab, map[string]string{"ApplicationName": "Tasso Zip", "ApplicationDescription": testi.T(l, "sottotitolo"), "ApplicationIcon": `"` + exe + `",0`})

	var predefinite []string
	for _, e := range estensioniAssoc {
		imposta(chiaveAppExe+`\SupportedTypes`, map[string]string{e: ""})
		imposta(chiaveCapab+`\FileAssociations`, map[string]string{e: progID})
		imposta(`Software\Classes\`+e+`\OpenWithProgids`, map[string]string{progID: ""})
		// Predefinito solo se l'utente non ha già un tipo scelto per sé.
		k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\`+e, registry.QUERY_VALUE|registry.SET_VALUE)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		v, _, e2 := k.GetStringValue("")
		if e2 != nil || v == "" || v == progID {
			if err := k.SetStringValue("", progID); err == nil {
				predefinite = append(predefinite, e)
			} else {
				errs = append(errs, err)
			}
		}
		k.Close()
	}
	imposta(`Software\RegisteredApplications`, map[string]string{nomeRegApp: chiaveCapab})
	avvisaExplorer()
	return predefinite, errors.Join(errs...)
}

// togliAssociazioni rimette tutto com'era: toglie solo quello che ha messo Tasso Zip.
func togliAssociazioni() error {
	var errs []error
	for _, e := range estensioniAssoc {
		if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\`+e+`\OpenWithProgids`, registry.SET_VALUE); err == nil {
			_ = k.DeleteValue(progID)
			k.Close()
		}
		if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\`+e, registry.QUERY_VALUE|registry.SET_VALUE); err == nil {
			if v, _, _ := k.GetStringValue(""); v == progID {
				if err := k.DeleteValue(""); err != nil {
					errs = append(errs, err)
				}
			}
			k.Close()
		}
	}
	for _, c := range []string{`Software\Classes\` + progID, chiaveAppExe, `Software\Tasso\TassoZip`} {
		if err := cancellaAlbero(registry.CURRENT_USER, c); err != nil {
			errs = append(errs, err)
		}
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\RegisteredApplications`, registry.SET_VALUE); err == nil {
		_ = k.DeleteValue(nomeRegApp)
		k.Close()
	}
	avvisaExplorer()
	return errors.Join(errs...)
}

func leggiComandoApri() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\`+progID+`\shell\open\command`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	s, _, _ := k.GetStringValue("")
	return s
}

func associazioniAttive() bool { return leggiComandoApri() != "" }

func associazioniPuntanoA(exe string) bool {
	return strings.EqualFold(leggiComandoApri(), comandoApri(exe))
}
