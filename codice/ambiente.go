package main

// Ambiente pulito per Tasso e per tutto quello che Tasso fa partire (PowerShell, cmd, schtasks...).
//
// Tasso gira come amministratore, ma le sue variabili d'ambiente arrivano dall'utente: un programma
// senza permessi può cambiarle in HKCU\Environment (TEMP, SystemRoot, PSModulePath, COR_PROFILER...)
// e così far caricare a un PowerShell da amministratore un modulo o una DLL sua. All'avvio, prima di
// qualsiasi altra cosa, rimetto le variabili importanti ai valori veri chiesti a Windows: le eredita
// ogni programma che Tasso fa partire, e anche os.Getenv in Tasso legge quelle vere.

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// cartelleVere: le cartelle chieste a Windows all'avvio (mai prese dalle variabili d'ambiente).
var cartelleVere cartelle

type cartelle struct {
	Win, Sys32               string // C:\Windows e C:\Windows\System32
	PF, PF86, PD             string // Programmi, Programmi (x86), ProgramData
	Local, Roaming, Profilo  string // AppData\Local, AppData\Roaming e cartella dell'utente
	Pubblica                 string // C:\Users\Public
	Common, Common86         string // Programmi\File comuni
	TempTasso, TempTassoNota string // la cartella dei temporanei dei programmi avviati da Tasso (e perché, se non è quella solita)
	TempOriginale            string // il TEMP dell'utente com'era prima che Tasso lo cambiasse, se è da pulire ("" se no)
}

// pathDiSistema: il PATH per Tasso e per tutto quello che fa partire: le cartelle di Windows, poi il
// PATH di sistema (HKLM\...\Session Manager\Environment) espanso, senza le cartelle dell'utente,
// senza percorsi relativi o di rete e senza doppioni.
func pathDiSistema(c *cartelle) string {
	voci := []string{c.Sys32, c.Win, filepath.Join(c.Sys32, "Wbem"), filepath.Join(c.Sys32, "WindowsPowerShell", "v1.0")}
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`, registry.QUERY_VALUE); err == nil {
		if v, _, err := k.GetStringValue("Path"); err == nil {
			// %SystemRoot% e simili: le variabili qui sono già quelle vere (messe sopra in ambientePulito).
			if e, err := registry.ExpandString(v); err == nil {
				v = e
			}
			voci = append(voci, strings.Split(v, ";")...)
		}
		_ = k.Close()
	}
	return unisciPath(voci, []string{c.Profilo, c.Local, c.Roaming})
}

// unisciPath: mette insieme le cartelle del PATH togliendo i doppioni (senza badare a maiuscole e
// alla \ finale), quelle vuote, relative, di rete, con variabili non espanse e quelle dentro "via".
func unisciPath(voci, via []string) string {
	visti := map[string]bool{}
	var out []string
	for _, v := range voci {
		v = strings.TrimSpace(strings.Trim(strings.TrimSpace(v), `"`))
		if v == "" || strings.Contains(v, "%") || strings.HasPrefix(v, `\\`) || strings.HasPrefix(v, "//") {
			continue
		}
		if vol := filepath.VolumeName(v); len(vol) != 2 || !filepath.IsAbs(v) {
			continue // percorso relativo (es. "." = la cartella di lavoro): mai
		}
		v = filepath.Clean(v)
		escludi := false
		for _, d := range via {
			if d != "" && dentroDi(v, d) {
				escludi = true
				break
			}
		}
		if escludi {
			continue
		}
		k := strings.ToLower(strings.TrimRight(v, `\`))
		if visti[k] {
			continue
		}
		visti[k] = true
		out = append(out, v)
	}
	return strings.Join(out, ";")
}

// tempOriginale: il TEMP dell'utente com'era all'avvio, da pulire insieme ad AppData\Local\Temp,
// solo se è una cartella normale: non dentro Windows, Programmi o ProgramData, non un disco intero,
// non in rete, non dentro il profilo di un altro utente, e non una cartella che contiene le cartelle
// dell'utente (es. TEMP = C:\Users\nome svuoterebbe tutto il profilo). Se Tasso è stato riaperto
// da sé stesso, il TEMP ereditato è già quello di Tasso: allora lo leggo da HKCU\Environment.
func tempOriginale(c *cartelle, t string) string {
	pulisci := func(p string) string {
		p = strings.TrimSpace(strings.Trim(strings.TrimSpace(p), `"`))
		if p == "" || strings.Contains(p, "%") {
			return ""
		}
		if q, ok := percorsoLocale(p); ok {
			return q
		}
		return ""
	}
	tassoTemp := filepath.Join(cartellaTasso(), "lavoro", "tmp")
	t = pulisci(t)
	if t == "" || dentroDi(t, tassoTemp) || (c.Win != "" && dentroDi(t, filepath.Join(c.Win, "Temp"))) {
		t = ""
		if k, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE); err == nil {
			if v, _, err := k.GetStringValue("TEMP"); err == nil {
				if e, err := registry.ExpandString(v); err == nil {
					v = e
				}
				t = pulisci(v)
			}
			_ = k.Close()
		}
	}
	if t == "" || len(t) <= 3 {
		return ""
	}
	if c.Local != "" && strings.EqualFold(t, filepath.Join(c.Local, "Temp")) {
		return "" // è quella solita: la pulisce già $TassoTempUtente
	}
	for _, d := range []string{c.Win, c.PF, c.PF86, c.PD, c.Common, c.Common86, tassoTemp} {
		if d != "" && dentroDi(t, d) {
			return ""
		}
	}
	// Non deve contenere (o essere) le cartelle dell'utente o quelle di Windows.
	for _, d := range []string{c.Profilo, c.Local, c.Roaming, c.Pubblica, c.Win, c.PF, c.PF86, c.PD} {
		if d != "" && dentroDi(d, t) {
			return ""
		}
	}
	if c.Profilo != "" {
		if utenti := filepath.Dir(c.Profilo); dentroDi(t, utenti) && !dentroDi(t, c.Profilo) {
			return "" // profilo di un altro utente
		}
	}
	// Cartelle speciali in cima a un disco (Cestino, ripristino, avvio): mai.
	primo := strings.ToLower(strings.SplitN(strings.TrimLeft(t[2:], `\`), `\`, 2)[0])
	if strings.HasPrefix(primo, "$") || primo == "system volume information" || primo == "recovery" || primo == "boot" || primo == "efi" || primo == "windows.old" {
		return ""
	}
	if fi, err := os.Stat(t); err != nil || !fi.IsDir() {
		return ""
	}
	return t
}

// winDir: la vera cartella di Windows (chiesta a Windows, non presa da SystemRoot o windir).
func winDir() string {
	if cartelleVere.Win != "" {
		return cartelleVere.Win
	}
	if d, err := windows.GetWindowsDirectory(); err == nil && d != "" {
		return d
	}
	return `C:\Windows`
}

// cartellaNota: una cartella di Windows (Programmi, AppData...) chiesta a Windows. "" se non c'è.
func cartellaNota(id *windows.KNOWNFOLDERID) string {
	if p, err := windows.KnownFolderPath(id, 0); err == nil && p != "" {
		return filepath.Clean(p)
	}
	return ""
}

// ambientePulito: va chiamata una volta sola, all'inizio di main (in hardenProcess), prima di
// qualsiasi goroutine e prima di far partire qualunque programma.
func ambientePulito() {
	c := &cartelleVere
	c.Win = winDir()
	c.Sys32 = sistema32()
	c.PF = programFilesDir()
	c.PF86 = cartellaNota(windows.FOLDERID_ProgramFilesX86)
	c.PD = cartellaNota(windows.FOLDERID_ProgramData)
	c.Local = cartellaNota(windows.FOLDERID_LocalAppData)
	c.Roaming = cartellaNota(windows.FOLDERID_RoamingAppData)
	c.Profilo = cartellaNota(windows.FOLDERID_Profile)
	c.Pubblica = cartellaNota(windows.FOLDERID_Public)
	c.Common = cartellaNota(windows.FOLDERID_ProgramFilesCommon)
	c.Common86 = cartellaNota(windows.FOLDERID_ProgramFilesCommonX86)

	// Variabili che fanno caricare codice dentro i programmi .NET (PowerShell compreso): via tutte.
	// Anche il percorso della cache dei moduli di PowerShell: deciderebbe dove scrive un PowerShell da amministratore.
	for _, kv := range os.Environ() {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue // le variabili nascoste di Windows tipo "=C:" restano
		}
		k := strings.ToUpper(kv[:i])
		for _, pre := range []string{"COR_", "COMPLUS_", "DOTNET_", "CORECLR_"} {
			if strings.HasPrefix(k, pre) {
				_ = os.Unsetenv(kv[:i])
			}
		}
		if k == "PSMODULEANALYSISCACHEPATH" {
			_ = os.Unsetenv(kv[:i])
		}
	}

	set := func(k, v string) {
		if v != "" {
			_ = os.Setenv(k, v)
		}
	}
	set("SystemRoot", c.Win)
	set("windir", c.Win)
	set("SystemDrive", filepath.VolumeName(c.Win))
	set("ComSpec", filepath.Join(c.Sys32, "cmd.exe"))
	set("ProgramFiles", c.PF)
	set("ProgramW6432", c.PF)
	set("ProgramFiles(x86)", c.PF86)
	set("CommonProgramFiles", c.Common)
	set("CommonProgramW6432", c.Common)
	set("CommonProgramFiles(x86)", c.Common86)
	set("ProgramData", c.PD)
	set("ALLUSERSPROFILE", c.PD)
	set("PUBLIC", c.Pubblica)
	// Le cartelle dell'utente restano quelle VERE dell'utente (Tasso gira col suo account, solo con
	// più permessi): gli script che puliscono la sua cache in AppData continuano a funzionare.
	set("LOCALAPPDATA", c.Local)
	set("APPDATA", c.Roaming)
	set("USERPROFILE", c.Profilo)
	// Moduli di PowerShell solo dalle cartelle di Windows e di Programmi (scrivibili solo da amministratore),
	// mai da Documenti dell'utente.
	set("PSModulePath", filepath.Join(c.Sys32, "WindowsPowerShell", "v1.0", "Modules")+";"+filepath.Join(c.PF, "WindowsPowerShell", "Modules"))
	// PATH: prima le cartelle di Windows, poi solo quelle del PATH di sistema (HKLM), mai quelle
	// dell'utente: un programma chiamato solo per nome da un PowerShell da amministratore non deve
	// poter arrivare da una cartella dove scrive un programma qualsiasi (per esempio gli alias in
	// AppData\Local\Microsoft\WindowsApps). I doppioni si tolgono: il PATH non si allunga a ogni riavvio.
	set("PATH", pathDiSistema(c))

	// La cartella dei temporanei vera dell'utente (es. D:\Temp), prima di cambiarla qui sotto: la
	// pulizia la svuota insieme ad AppData\Local\Temp (vedi tempOriginale).
	c.TempOriginale = tempOriginale(c, os.Getenv("TEMP"))

	// Temporanei: in Programmi\Tasso\lavoro\tmp, dove un programma normale non può scrivere né
	// mettere file finti che poi un installatore da amministratore caricherebbe.
	c.TempTasso = filepath.Join(cartellaTasso(), "lavoro", "tmp") // ecosistema.go: di base Programmi\Tasso
	if os.MkdirAll(c.TempTasso, 0o755) != nil || !cartellaScrivibile(c.TempTasso) {
		// Programmi non è scrivibile (caso raro, es. criteri dell'azienda): uso Windows\Temp, la
		// cartella dei temporanei dei servizi di Windows. Lì un utente normale può creare file suoi
		// ma non leggere né cambiare quelli creati da un amministratore: è comunque molto meglio
		// della cartella temporanea dell'utente, che un programma qualsiasi può riempire di file finti.
		c.TempTassoNota = "Programmi non è scrivibile: uso Windows\\Temp per i temporanei"
		c.TempTasso = filepath.Join(c.Win, "Temp")
	}
	set("TEMP", c.TempTasso)
	set("TMP", c.TempTasso)

	// La cartella di lavoro: System32 e non quella da cui è partito Tasso (es. Download), perché
	// Windows cerca i programmi chiamati solo per nome anche nella cartella di lavoro.
	_ = os.Chdir(c.Sys32)
}

// cartellaScrivibile: prova a creare e togliere un file vuoto.
func cartellaScrivibile(d string) bool {
	f, err := os.CreateTemp(d, "prova-*.tmp")
	if err != nil {
		return false
	}
	n := f.Name()
	_ = f.Close()
	_ = os.Remove(n)
	return true
}

// pulisciTempTasso: i programmi lasciano spesso file nella cartella dei temporanei. In quella di
// Tasso tolgo quello che ha più di due giorni (solo lì, mai in Windows\Temp, che pulisce il passaggio apposta).
func pulisciTempTasso() {
	defer guard("pulisciTempTasso")
	d := cartelleVere.TempTasso
	if d == "" || cartelleVere.TempTassoNota != "" {
		return
	}
	l, err := os.ReadDir(d)
	if err != nil {
		return
	}
	for _, e := range l {
		p := filepath.Join(d, e.Name())
		fi, err := os.Lstat(p)
		if err != nil || time.Since(fi.ModTime()) < 48*time.Hour {
			continue
		}
		// os.RemoveAll non segue i collegamenti: toglie il collegamento, non quello a cui punta.
		_ = os.RemoveAll(p)
	}
}

// psQuote: un testo dentro apici singoli di PowerShell.
func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// psCartelle: le cartelle vere passate a ogni script (non prese da $env:, che uno script potrebbe
// aver ricevuto cambiato). $TassoTempUtente è la cartella temporanea dell'utente (AppData\Local\Temp),
// quella che pulisce il passaggio "File temporanei"; $env:TEMP invece ora è quella di Tasso.
func psCartelle() string {
	c := cartelleVere
	if c.Win == "" { // ambientePulito non ancora chiamata (non dovrebbe succedere)
		c.Win, c.Sys32, c.PF = winDir(), sistema32(), programFilesDir()
		c.Local = cartellaNota(windows.FOLDERID_LocalAppData)
		c.Roaming = cartellaNota(windows.FOLDERID_RoamingAppData)
		c.PD = cartellaNota(windows.FOLDERID_ProgramData)
		c.PF86 = cartellaNota(windows.FOLDERID_ProgramFilesX86)
		c.Profilo = cartellaNota(windows.FOLDERID_Profile)
	}
	tu := ""
	if c.Local != "" {
		tu = filepath.Join(c.Local, "Temp")
	}
	return "$TassoWin = " + psQuote(c.Win) +
		"\n$TassoSys32 = " + psQuote(c.Sys32) +
		"\n$TassoPF = " + psQuote(c.PF) +
		"\n$TassoPF86 = " + psQuote(c.PF86) +
		"\n$TassoPD = " + psQuote(c.PD) +
		"\n$TassoLocal = " + psQuote(c.Local) +
		"\n$TassoRoaming = " + psQuote(c.Roaming) +
		"\n$TassoProfilo = " + psQuote(c.Profilo) +
		"\n$TassoTempUtente = " + psQuote(tu) +
		"\n$TassoTempOriginale = " + psQuote(c.TempOriginale) +
		"\n$TassoExe = " + psQuote(tassoExe()) +
		"\n$TassoCartella = " + psQuote(cartellaTasso()) +
		"\n$TassoSysDrive = " + psQuote(filepath.VolumeName(c.Win))
}
