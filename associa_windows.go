// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

//go:build windows

package main

// "Apri gli archivi con doppio clic con Tasso Zip": solo se l'utente accende l'interruttore
// (mai in silenzio). Tutto in HKEY_CURRENT_USER (niente amministratore):
//   - il tipo di file TassoZip.Archivio (icona e comando "--apri");
//   - Tasso Zip fra i programmi "Apri con" di ogni estensione (OpenWithProgids);
//   - il programma predefinito per le estensioni che per l'utente non ne hanno già uno
//     scelto in HKCU (quando si spegne si toglie solo se vale ancora TassoZip.Archivio);
//   - Tasso Zip fra le "App predefinite" di Windows (Capabilities, RegisteredApplications).
// Windows 10 e 11 non lasciano a nessun programma cambiare da solo la scelta fatta
// dall'utente con "Apri con > Usa sempre questa app" (UserChoice): in quel caso Windows
// chiede all'utente, o si sceglie Tasso Zip da "Apri con". Tasso Zip non la scrive mai:
//   - accendendo, la legge solo per dire il vero ("fatto" solo se il doppio clic apre
//     davvero Tasso Zip) e si segna quella che c'era prima (Software\Tasso\TassoZip\Prima);
//   - spegnendo, se la scelta dell'utente porta a Tasso Zip la cancella: così Windows torna
//     al programma di prima invece di chiedere "Come vuoi aprire questo file?". Se prima
//     c'era la scelta di un altro programma, Windows non la lascia rimettere: lo diciamo.

import (
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"

	"tassozip/testi"
)

const (
	chiavePrima  = `Software\Tasso\TassoZip\Prima` // la scelta dell'utente com'era prima di accendere
	chiaveScelte = `Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\`
	progID       = "TassoZip.Archivio"
	chiaveCapab  = `Software\Tasso\TassoZip\Capabilities`
	nomeRegApp   = "Tasso Zip"
	chiaveAppExe = `Software\Classes\Applications\TassoZip.exe`
)

var estensioniAssoc = []string{".zip", ".7z", ".rar", ".tar", ".gz", ".tgz", ".bz2", ".tbz2", ".xz", ".txz", ".zst", ".tzst"}

func comandoApri(exe string) string { return `"` + exe + `" --apri "%1"` }

// scelteUtente: le chiavi di "Apri con > Usa sempre questa app" (UserChoiceLatest c'è nei
// Windows 11 più nuovi e, se c'è, conta lei).
var scelteUtente = []string{"UserChoiceLatest", "UserChoice"}

// sceltaUtente legge il programma scelto dall'utente per l'estensione ("" = nessuna scelta).
func sceltaUtente(e string) string {
	for _, c := range scelteUtente {
		k, err := registry.OpenKey(registry.CURRENT_USER, chiaveScelte+e+`\`+c, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		v, _, err := k.GetStringValue("ProgId")
		k.Close()
		if err == nil && v != "" {
			return v
		}
	}
	return ""
}

// diTassoZip: il tipo di file porta a Tasso Zip (il nostro, o "Apri con > Tasso Zip").
func diTassoZip(p string) bool {
	return strings.EqualFold(p, progID) || strings.EqualFold(p, `Applications\TassoZip.exe`)
}

// attivaAssociazioni registra Tasso Zip per aprire gli archivi. Restituisce le estensioni
// che con il doppio clic NON apriranno Tasso Zip (un altro tipo scelto in HKCU, o la scelta
// dell'utente in "Apri con"): la pagina lo dice invece di "fatto".
func attivaAssociazioni(l string) ([]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	var errs []error
	// prima di tutto mi segno la scelta dell'utente com'era (solo la prima volta)
	if k, _, err := registry.CreateKey(registry.CURRENT_USER, chiavePrima, registry.QUERY_VALUE|registry.SET_VALUE); err == nil {
		for _, e := range estensioniAssoc {
			if _, _, err := k.GetStringValue(e); err == nil {
				continue // già segnata da un'accensione di prima
			}
			if v := sceltaUtente(e); !diTassoZip(v) {
				_ = k.SetStringValue(e, v) // "" = nessuna scelta
			}
		}
		k.Close()
	} else {
		errs = append(errs, err)
	}
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

	var nonPredefinite []string
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
		predefinito := false
		if e2 != nil || v == "" || v == progID {
			if err := k.SetStringValue("", progID); err == nil {
				predefinito = true
			} else {
				errs = append(errs, err)
			}
		}
		k.Close()
		// conta la scelta dell'utente, se c'è: Tasso Zip non la cambia
		if s := sceltaUtente(e); s != "" {
			predefinito = diTassoZip(s)
		}
		if !predefinito {
			nonPredefinite = append(nonPredefinite, e)
		}
	}
	imposta(`Software\RegisteredApplications`, map[string]string{nomeRegApp: chiaveCapab})
	avvisaExplorer()
	return nonPredefinite, errors.Join(errs...)
}

// togliAssociazioni rimette tutto com'era: toglie solo quello che ha messo Tasso Zip, più la
// scelta "Apri con > Tasso Zip > Sempre" (altrimenti Windows chiederebbe "Come vuoi aprire
// questo file?"). Restituisce le estensioni per cui prima l'utente aveva scelto un altro
// programma: Windows non lascia rimettere quella scelta, quindi la pagina dice di rifarla.
func togliAssociazioni() ([]string, error) {
	var errs []error
	var daRiscegliere []string
	prima := map[string]string{}
	if k, err := registry.OpenKey(registry.CURRENT_USER, chiavePrima, registry.QUERY_VALUE); err == nil {
		for _, e := range estensioniAssoc {
			if v, _, err := k.GetStringValue(e); err == nil {
				prima[e] = v
			}
		}
		k.Close()
	}
	for _, e := range estensioniAssoc {
		tolta := false
		for _, c := range scelteUtente {
			k, err := registry.OpenKey(registry.CURRENT_USER, chiaveScelte+e+`\`+c, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			v, _, _ := k.GetStringValue("ProgId")
			k.Close()
			if !diTassoZip(v) {
				continue
			}
			if err := registry.DeleteKey(registry.CURRENT_USER, chiaveScelte+e+`\`+c); err != nil {
				errs = append(errs, err) // la pagina dice "non sono riuscito", non "fatto"
				continue
			}
			tolta = true
		}
		if tolta && prima[e] != "" && !diTassoZip(prima[e]) && sceltaUtente(e) == "" {
			daRiscegliere = append(daRiscegliere, e)
		}
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
		// le chiavi rimaste vuote (come fa il disinstallatore)
		cancellaSeVuota(`Software\Classes\` + e + `\OpenWithProgids`)
		cancellaSeVuota(`Software\Classes\` + e)
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
	return daRiscegliere, errors.Join(errs...)
}

// cancellaSeVuota cancella la chiave solo se non ha più né valori né sottochiavi.
func cancellaSeVuota(chiave string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, chiave, registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return
	}
	info, err := k.Stat()
	k.Close()
	if err == nil && info.ValueCount == 0 && info.SubKeyCount == 0 {
		_ = registry.DeleteKey(registry.CURRENT_USER, chiave)
	}
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
