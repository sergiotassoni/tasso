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
//     c'era la scelta di un altro programma, Windows non la lascia rimettere: lo diciamo. Se
//     la scelta di Tasso Zip ("Apri con > TassoZip.exe > Sempre") l'aveva fatta l'utente prima
//     di accendere, resta (con Applications\TassoZip.exe): torna com'era.

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

// progIdScelta: il programma scritto in una delle chiavi di scelteUtente. In UserChoiceLatest
// Windows 11 lo mette nella sottochiave "ProgId" (valore "ProgId"; la chiave ha solo "Hash"):
// la leggo per prima, poi il valore diretto (UserChoice, e per sicurezza UserChoiceLatest).
func progIdScelta(chiave string) string {
	for _, c := range []string{chiave + `\ProgId`, chiave} {
		k, err := registry.OpenKey(registry.CURRENT_USER, c, registry.QUERY_VALUE)
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

// sceltaUtente legge il programma scelto dall'utente per l'estensione ("" = nessuna scelta).
func sceltaUtente(e string) string {
	for _, c := range scelteUtente {
		if v := progIdScelta(chiaveScelte + e + `\` + c); v != "" {
			return v
		}
	}
	return ""
}

// primaTassoZip: in Prima, "la scelta dell'utente portava già a Tasso Zip" (fatta da lui con
// "Apri con > Cerca un'altra app > TassoZip.exe > Sempre" prima di accendere l'interruttore):
// spegnendo non la cancello, perché c'era prima. valoreAppExePrima: Applications\TassoZip.exe
// c'era già (scritta da Windows con quella scelta): spegnendo resta.
const (
	primaTassoZip     = "=tassozip"
	valoreAppExePrima = "AppExePrima"
)

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
	return attivaAssociazioniCon(exe, l)
}

// attivaAssociazioniCon: come attivaAssociazioni, verso l'exe indicato.
func attivaAssociazioniCon(exe, l string) ([]string, error) {
	var errs []error
	// prima di tutto mi segno la scelta dell'utente com'era (solo la prima volta)
	if k, _, err := registry.CreateKey(registry.CURRENT_USER, chiavePrima, registry.QUERY_VALUE|registry.SET_VALUE); err == nil {
		appExeNuova := !associazioniAttive() // prima accensione: Applications\TassoZip.exe non è ancora nostra
		for _, e := range estensioniAssoc {
			if _, _, err := k.GetStringValue(e); err == nil {
				continue // già segnata da un'accensione di prima
			}
			v := sceltaUtente(e)
			if strings.EqualFold(v, `Applications\TassoZip.exe`) {
				v = primaTassoZip // scelta dall'utente da "Apri con": spegnendo resta
			} else if diTassoZip(v) {
				v = "" // il nostro tipo di file, rimasto da prima: va via con lui
			}
			_ = k.SetStringValue(e, v) // "" = nessuna scelta
		}
		if _, _, err := k.GetStringValue(valoreAppExePrima); err != nil && appExeNuova {
			c, err := registry.OpenKey(registry.CURRENT_USER, chiaveAppExe, registry.QUERY_VALUE)
			c2 := "0"
			if err == nil {
				c.Close()
				c2 = "1"
			}
			_ = k.SetStringValue(valoreAppExePrima, c2)
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
	appExePrima := false
	if k, err := registry.OpenKey(registry.CURRENT_USER, chiavePrima, registry.QUERY_VALUE); err == nil {
		for _, e := range estensioniAssoc {
			if v, _, err := k.GetStringValue(e); err == nil {
				prima[e] = v
			}
		}
		v, _, _ := k.GetStringValue(valoreAppExePrima)
		appExePrima = v == "1"
		k.Close()
	}
	for _, e := range estensioniAssoc {
		tolta := false
		for _, c := range scelteUtente {
			if prima[e] == primaTassoZip {
				break // la scelta di Tasso Zip l'aveva fatta l'utente prima di accendere: resta
			}
			if !diTassoZip(progIdScelta(chiaveScelte + e + `\` + c)) {
				continue
			}
			// UserChoiceLatest ha la sottochiave ProgId: RegDeleteKey da sola non la cancellerebbe
			if err := cancellaAlbero(registry.CURRENT_USER, chiaveScelte+e+`\`+c); err != nil {
				errs = append(errs, err) // la pagina dice "non sono riuscito", non "fatto"
				continue
			}
			tolta = true
		}
		togliResti(e, appExePrima)
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
	togliere := []string{`Software\Classes\` + progID, `Software\Tasso\TassoZip`}
	if !appExePrima {
		togliere = append(togliere, chiaveAppExe)
	}
	for _, c := range togliere {
		if err := cancellaAlbero(registry.CURRENT_USER, c); err != nil {
			errs = append(errs, err)
		}
	}
	togliTassoVuota()
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\RegisteredApplications`, registry.SET_VALUE); err == nil {
		_ = k.DeleteValue(nomeRegApp)
		k.Close()
	}
	avvisaExplorer()
	return daRiscegliere, errors.Join(errs...)
}

// togliResti: quello che Windows stesso scrive quando si usa Tasso Zip e che porta a Tasso Zip
// (innocuo, ma il registro deve tornare com'era): TassoZip.Archivio in FileExts\<ext>\OpenWithProgids,
// TassoZip.exe in FileExts\<ext>\OpenWithList (con la sua lettera in MRUList) e gli avvisi
// "nuova app" in ApplicationAssociationToasts. Con appExePrima (l'utente aveva già scelto
// TassoZip.exe da "Apri con" prima di accendere) quelli di TassoZip.exe restano.
func togliResti(e string, appExePrima bool) {
	if k, err := registry.OpenKey(registry.CURRENT_USER, chiaveScelte+e+`\OpenWithProgids`, registry.SET_VALUE); err == nil {
		_ = k.DeleteValue(progID)
		k.Close()
	}
	if appExePrima {
		if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\ApplicationAssociationToasts`, registry.SET_VALUE); err == nil {
			_ = k.DeleteValue(progID + "_" + e)
			k.Close()
		}
		return
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, chiaveScelte+e+`\OpenWithList`, registry.QUERY_VALUE|registry.SET_VALUE); err == nil {
		mru, _, _ := k.GetStringValue("MRUList")
		for c := 'a'; c <= 'z'; c++ {
			if v, _, err := k.GetStringValue(string(c)); err == nil && strings.EqualFold(v, "TassoZip.exe") {
				if k.DeleteValue(string(c)) == nil {
					mru = strings.ReplaceAll(mru, string(c), "")
				}
			}
		}
		if m, _, err := k.GetStringValue("MRUList"); err == nil && m != mru {
			_ = k.SetStringValue("MRUList", mru)
		}
		k.Close()
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\ApplicationAssociationToasts`, registry.SET_VALUE); err == nil {
		_ = k.DeleteValue(progID + "_" + e)
		_ = k.DeleteValue(`Applications\TassoZip.exe_` + e)
		k.Close()
	}
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
