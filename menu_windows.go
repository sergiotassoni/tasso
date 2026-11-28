// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

//go:build windows

package main

// Il sottomenu "Tasso Zip" nel menu del tasto destro.
// Tutto in HKEY_CURRENT_USER\Software\Classes: vale solo per l'utente e NON servono i
// permessi da amministratore. Su Windows 11 compare sotto "Mostra altre opzioni" (il menu
// nuovo di Windows 11 accetta solo programmi con un'estensione COM firmata e un pacchetto
// MSIX: Tasso Zip non lo fa).
//
// Sui file e sulle cartelle:  Comprimi in un archivio... / Comprimi in .zip / Comprimi in .7z
// Sugli archivi:              Apri con Tasso Zip / Estrai qui / Estrai in una cartella col suo nome /
//                             Estrai in... (scegli la cartella) / Comprimi in un archivio...
// Windows mette le voci del sottomenu in ordine di nome della chiave: per questo i numeri davanti.
//
// I nomi delle voci non possono contenere il nome del file scelto ("Comprimi in Foto.zip"):
// per quello Windows vuole un'estensione COM. Le voci dicono quindi ".zip" e ".7z".

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"tassozip/testi"
)

const verboMenu = "TassoZip"

// Voci di Tasso Zip 1.0 (da togliere quando si accende il menu nuovo).
var verbiVecchi = []string{"TassoZip.Comprimi", "TassoZip.Estrai"}

// estensioniMenu: i file su cui compare il sottomenu degli archivi.
var estensioniMenu = []string{".zip", ".7z", ".rar", ".tar", ".gz", ".tgz", ".bz2", ".tbz2", ".tbz", ".xz", ".txz", ".zst", ".tzst", ".001"}

type voceMenu struct {
	nome, testo, arg string
}

func vociFile(l string) []voceMenu {
	return []voceMenu{
		{"1comprimi", testi.T(l, "menu_comprimi_con"), "--comprimi"},
		{"2zip", testi.T(l, "menu_comprimi_zip"), "--zip"},
		{"3settezip", testi.T(l, "menu_comprimi_7z"), "--7z"},
	}
}

func vociArchivio(l string) []voceMenu {
	return []voceMenu{
		{"1apri", testi.T(l, "menu_apri"), "--apri"},
		{"2qui", testi.T(l, "menu_estrai_qui"), "--estrai-qui"},
		{"3cartella", testi.T(l, "menu_estrai_cartella"), "--estrai-cartella"},
		{"4in", testi.T(l, "menu_estrai_in"), "--estrai-in"},
		{"5comprimi", testi.T(l, "menu_comprimi_con"), "--comprimi"},
	}
}

// radiciMenu: dove sta il sottomenu e con quali voci.
func radiciMenu(l string) map[string][]voceMenu {
	m := map[string][]voceMenu{
		`Software\Classes\*\shell\` + verboMenu:         vociFile(l),
		`Software\Classes\Directory\shell\` + verboMenu: vociFile(l),
	}
	// Stesso nome del verbo: per gli archivi Windows usa questo al posto di quello di "*".
	for _, e := range estensioniMenu {
		m[`Software\Classes\SystemFileAssociations\`+e+`\shell\`+verboMenu] = vociArchivio(l)
	}
	return m
}

func comando(exe, arg string) string { return `"` + exe + `" ` + arg + ` "%1"` }

// valoreLinguaMenu: la lingua in cui sono scritte le voci (sulla chiave del sottomenu: va via
// con il menu). Se la lingua cambia da Tasso o dall'installatore, all'avvio le riscrivo.
const valoreLinguaMenu = "TassoZipLingua"

// attivaMenu crea (o aggiorna) il sottomenu nella lingua indicata, verso questo exe.
func attivaMenu(l string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return attivaMenuCon(exe, l)
}

// attivaMenuCon: come attivaMenu, verso l'exe indicato.
func attivaMenuCon(exe, l string) error {
	togliVecchie()
	var errs []error
	for radice, voci := range radiciMenu(l) {
		_ = cancellaAlbero(registry.CURRENT_USER, radice) // niente voci rimaste da una versione prima
		k, _, err := registry.CreateKey(registry.CURRENT_USER, radice, registry.SET_VALUE)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		_ = k.SetStringValue("MUIVerb", "Tasso Zip")
		_ = k.SetStringValue("Icon", `"`+exe+`",0`)
		_ = k.SetStringValue("SubCommands", "") // le voci sono nella sottochiave "shell"
		// Senza questo, con più di 15 file scelti Windows nasconde il sottomenu (come Tasso PDF).
		_ = k.SetStringValue("MultiSelectModel", "Player")
		_ = k.SetStringValue(valoreLinguaMenu, l)
		k.Close()
		for _, v := range voci {
			c, _, err := registry.CreateKey(registry.CURRENT_USER, radice+`\shell\`+v.nome, registry.SET_VALUE)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			_ = c.SetStringValue("MUIVerb", v.testo)
			_ = c.SetStringValue("Icon", `"`+exe+`",0`)
			_ = c.SetStringValue("MultiSelectModel", "Player")
			c.Close()
			c, _, err = registry.CreateKey(registry.CURRENT_USER, radice+`\shell\`+v.nome+`\command`, registry.SET_VALUE)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if err := c.SetStringValue("", comando(exe, v.arg)); err != nil {
				errs = append(errs, err)
			}
			c.Close()
		}
	}
	avvisaExplorer()
	return errors.Join(errs...)
}

// togliMenu cancella solo le chiavi create da Tasso Zip.
func togliMenu() error {
	var errs []error
	togliVecchie()
	for radice := range radiciMenu("it") {
		if err := cancellaAlbero(registry.CURRENT_USER, radice); err != nil {
			errs = append(errs, err)
		}
		togliGenitoriVuoti(radice) // "…\.zip\shell" e "…\.zip" create per il menu (come il disinstallatore)
	}
	togliTassoVuota()
	avvisaExplorer()
	return errors.Join(errs...)
}

// togliGenitoriVuoti: dopo "…\.zip\shell\TassoZip" toglie anche "…\.zip\shell" e "…\.zip" se
// sono rimaste vuote. Mai Software\Classes o sopra (la stessa regola del disinstallatore).
func togliGenitoriVuoti(chiave string) {
	for {
		i := strings.LastIndex(chiave, `\`)
		if i < 0 {
			return
		}
		chiave = chiave[:i]
		l := strings.ToLower(chiave)
		if l == `software\classes` || l == `software` || l == `software\tasso` || !strings.HasPrefix(l, `software\`) {
			return
		}
		if !chiaveVuota(chiave) || registry.DeleteKey(registry.CURRENT_USER, chiave) != nil {
			return
		}
	}
}

// togliTassoVuota: HKCU\Software\Tasso, se è rimasta vuota (Tasso Zip portatile, senza l'Ecosistema).
func togliTassoVuota() {
	if chiaveVuota(`Software\Tasso`) {
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\Tasso`)
	}
}

// chiaveVuota: la chiave esiste e non ha né valori né sottochiavi.
func chiaveVuota(chiave string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, chiave, registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return false
	}
	info, err := k.Stat()
	k.Close()
	return err == nil && info.ValueCount == 0 && info.SubKeyCount == 0
}

// togliVecchie: le voci "Comprimi con Tasso Zip" / "Estrai con Tasso Zip" di Tasso Zip 1.0.
func togliVecchie() {
	var radici []string
	for _, v := range verbiVecchi {
		radici = append(radici, `Software\Classes\*\shell\`+v, `Software\Classes\Directory\shell\`+v)
		for _, e := range estensioniMenu {
			radici = append(radici, `Software\Classes\SystemFileAssociations\`+e+`\shell\`+v)
		}
	}
	for _, r := range radici {
		_ = cancellaAlbero(registry.CURRENT_USER, r)
	}
}

// cancellaAlbero cancella una chiave con tutte le sue sottochiavi (non esiste: va bene).
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

func leggiComando() string {
	return leggiComandoDi(`Software\Classes\*\shell\` + verboMenu + `\shell\1comprimi\command`)
}

func leggiComandoDi(chiave string) string {
	k, err := registry.OpenKey(registry.CURRENT_USER, chiave, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	s, _, _ := k.GetStringValue("")
	return s
}

// menuVecchio: c'è ancora il menu di Tasso Zip 1.0?
func menuVecchio() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\*\shell\`+verbiVecchi[0]+`\command`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	k.Close()
	return true
}

func menuAttivo() bool { return leggiComando() != "" || menuVecchio() }

// menuPuntaA: le voci del menu avviano proprio questo exe (e sono quelle nuove, con
// "Estrai in...")? Se no, all'avvio Tasso Zip le riscrive da solo.
// Controlla anche MultiSelectModel (aggiunto nella 1.3.2): le voci di prima si riscrivono da sole.
func menuPuntaA(exe string) bool {
	nuova := leggiComandoDi(`Software\Classes\SystemFileAssociations\.zip\shell\` + verboMenu + `\shell\4in\command`)
	return strings.EqualFold(leggiComando(), comando(exe, "--comprimi")) && !menuVecchio() &&
		strings.EqualFold(nuova, comando(exe, "--estrai-in")) &&
		leggiValoreMenu(`Software\Classes\*\shell\`+verboMenu+`\shell\1comprimi`, "MultiSelectModel") == "Player"
}

// linguaMenu: la lingua in cui sono scritte le voci ("" = di una versione di prima).
func linguaMenu() string {
	return leggiValoreMenu(`Software\Classes\*\shell\`+verboMenu, valoreLinguaMenu)
}

func leggiValoreMenu(chiave, nome string) string {
	k, err := registry.OpenKey(registry.CURRENT_USER, chiave, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	s, _, _ := k.GetStringValue(nome)
	return s
}

// exeDelComando: l'exe all'inizio di un comando come `"C:\…\TassoZip.exe" --zip "%1"`.
func exeDelComando(c string) string {
	c = strings.TrimSpace(c)
	if !strings.HasPrefix(c, `"`) {
		return ""
	}
	i := strings.Index(c[1:], `"`)
	if i <= 0 {
		return ""
	}
	return c[1 : 1+i]
}

// exeInstallato: TassoZip.exe installato con l'Ecosistema (HKLM\Software\Tasso\Cartella:
// solo gli amministratori la cambiano). "" = nessuno.
func exeInstallato() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `Software\Tasso`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue("Cartella")
	if err != nil || strings.TrimSpace(v) == "" {
		return ""
	}
	p := filepath.Join(strings.TrimSpace(v), "Tasso Zip", "TassoZip.exe")
	if fi, err := os.Stat(p); err != nil || fi.IsDir() {
		return ""
	}
	return p
}

// exePerVoci: verso quale exe riscrivere da sole le voci già registrate (menu o doppio clic)
// che oggi avviano registrato. Questo exe, se sono proprio io o se sono quello installato;
// altrimenti l'altro, se c'è ancora (una copia aperta una volta da TEMP o da Download non
// "ruba" le voci); se l'altro non c'è più, quello installato o, senza, questo.
func exePerVoci(registrato string) string {
	mio, err := os.Executable()
	if err != nil {
		return ""
	}
	altro := exeDelComando(registrato)
	if altro == "" || strings.EqualFold(filepath.Clean(altro), filepath.Clean(mio)) {
		return mio
	}
	inst := exeInstallato()
	if inst != "" && strings.EqualFold(filepath.Clean(inst), filepath.Clean(mio)) {
		return mio
	}
	if fi, err := os.Stat(altro); err == nil && !fi.IsDir() {
		return altro
	}
	if inst != "" {
		return inst
	}
	return mio
}

// aggiornaVociRegistrate: all'avvio, menu e doppio clic accesi si riscrivono da soli se sono di
// una versione di prima, se la lingua è cambiata o se l'exe che avviano non c'è più.
func aggiornaVociRegistrate() {
	l := lingua()
	if menuAttivo() {
		if exe := exePerVoci(leggiComando()); exe != "" && (!menuPuntaA(exe) || linguaMenu() != l) {
			_ = attivaMenuCon(exe, l)
		}
	}
	if associazioniAttive() {
		if exe := exePerVoci(leggiComandoApri()); exe != "" && !associazioniPuntanoA(exe) {
			_, _ = attivaAssociazioniCon(exe, l)
		}
	}
}

var procSHChangeNotify = shell32.NewProc("SHChangeNotify")

// avvisaExplorer: Esplora file rilegge le associazioni (SHCNE_ASSOCCHANGED).
func avvisaExplorer() {
	procSHChangeNotify.Call(0x08000000, 0, 0, 0)
}
