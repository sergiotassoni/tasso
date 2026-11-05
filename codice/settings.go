package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

type LastClean struct {
	Date  string `json:"date"`
	Level int    `json:"level"`
	Freed int64  `json:"freed"`
}

type Settings struct {
	Onboarded    bool          `json:"onboarded"`
	Profile      string        `json:"profile"`
	Mode         string        `json:"mode"`
	Theme        string        `json:"theme"`
	Lang         string        `json:"lang,omitempty"`        // "it" o "en" (2.6.2): lingua di Tasso
	ThemeOffice  string        `json:"themeOffice,omitempty"` // tema scelto nella modalità Ufficio (2.6)
	Autostart    bool          `json:"autostart"`
	Weekly       bool          `json:"weekly"`
	WeeklyDay    int           `json:"weeklyDay"`
	WeeklyHour   int           `json:"weeklyHour,omitempty"`
	WeeklyLevel  int           `json:"weeklyLevel,omitempty"`
	WeeklyEvery  int           `json:"weeklyEvery,omitempty"`
	Bench        []benchResult `json:"bench,omitempty"`
	BenchAround  bool          `json:"benchAround,omitempty"`
	LevelsV      int           `json:"levelsV,omitempty"` // 5 = livelli della 2.5 (Classica ... Eccezionale)
	SmartAlerts  *bool         `json:"smartAlerts"`
	LastClean    *LastClean    `json:"lastClean"`
	KnownStartup []string      `json:"knownStartup"`
	Paused       bool          `json:"paused"`
	PrevAuto     bool          `json:"prevAutostart"`
	PrevWeekly   bool          `json:"prevWeekly"`
	AutoGame     bool          `json:"autoGame"`
	AutoGameTune bool          `json:"autoGameTune,omitempty"` // accesa da "Regola con Tasso", non da te
	// Stato del PC su Discord (webhook di un canale privato, solo lettura).
	DiscordWebhook string `json:"discordWebhook,omitempty"`
	DiscordOn      bool   `json:"discordOn"`
	DiscordLast    string `json:"discordLast,omitempty"`
	DiscordEvery   int    `json:"discordEvery,omitempty"` // ore tra un report e l'altro: 3, 6, 12 o 24
	// Bot facoltativo per il report su richiesta: token protetto con Windows (DPAPI), mai in chiaro.
	DiscordBot     string `json:"discordBot,omitempty"`
	DiscordBotOn   bool   `json:"discordBotOn"`
	DiscordBotID   string `json:"discordBotId,omitempty"`
	DiscordChannel string `json:"discordChannel,omitempty"`
	DiscordLastMsg string `json:"discordLastMsg,omitempty"`
	// Modalità da rimettere quando si chiude il gioco (modalità gioco automatica).
	ModeBeforeGame string `json:"modeBeforeGame,omitempty"`
	// Modalità da rimettere a fine partita dopo "Ottimizza prima di giocare".
	ModeBeforeBoost string `json:"modeBeforeBoost,omitempty"`
	// Pannellino in gioco: si mostra da solo quando parte un gioco; posizione "dx" o "sx".
	OverlayOn bool `json:"overlayOn"`
	// Tasto dell'autoclick (codice tasto di Windows e Ctrl/Alt/Maiusc); 0 = F6.
	AcVK       uint32 `json:"acVK,omitempty"`
	AcMods     uint32 `json:"acMods,omitempty"`
	OverlayPos string `json:"overlayPos,omitempty"`
	// Snapshot: le impostazioni di Windows com'erano prima che Tasso le cambiasse la prima volta.
	Snapshot    json.RawMessage `json:"snapshot,omitempty"`
	SnapshotAt  string          `json:"snapshotAt,omitempty"`
	StartupOrig map[string]bool `json:"startupOrig,omitempty"`
	// "Regola con Tasso": regolazioni per gioco e modalità del PC da mettere quando parte quel gioco.
	GameTunes map[string]*GameTune `json:"gameTunes,omitempty"`
	GameModes map[string]string    `json:"gameModes,omitempty"`
	// Comandi gaming accesi da Tasso (per poterli rimettere com'erano) e riavvio richiesto per la GPU.
	GamingCmds  map[string]bool      `json:"gamingCmds,omitempty"`
	PingRegion  string               `json:"pingRegion,omitempty"`
	StartupV    int                  `json:"startupV,omitempty"` // 2 = elenco nuovo (2.5) con le app dello Store
	GamingPrev  map[string][]regPrev `json:"gamingPrev,omitempty"`
	HagsReboot  string               `json:"hagsReboot,omitempty"`
	GameProfile *gameProfile         `json:"gameProfile,omitempty"`
	GamePower   map[string]gamePower `json:"gamePower,omitempty"`
	// Avvisi: cosa l'utente vuole che Tasso gli segnali (temperature, disco pieno, avvio, memoria, salute dischi).
	Alerts map[string]bool `json:"alerts,omitempty"`
	// Scrivici (3.0): la domanda dopo la prima pulizia si fa una volta sola; massimo 5 messaggi al giorno.
	FeedbackAsked bool   `json:"feedbackAsked,omitempty"`
	FeedbackDay   string `json:"feedbackDay,omitempty"`
	FeedbackCount int    `json:"feedbackCount,omitempty"`
	FeedbackSent  int    `json:"feedbackSent,omitempty"`
	// Numeri anonimi (3.0): nil = non ancora chiesto. Partono solo se la persona ha detto sì.
	Stats       *bool  `json:"stats"`
	StatsDay    string `json:"statsDay,omitempty"`
	StatsHello  string `json:"statsHello,omitempty"`
	StatsMonth  string `json:"statsMonth,omitempty"`
	StatsRuns   int    `json:"statsRuns,omitempty"`
	StatsRunDay string `json:"statsRunDay,omitempty"`
	StatsErr    int    `json:"statsErr,omitempty"`
}

// setPaused spegne o riaccende Tasso: da spento non parte con Windows, non fa pulizie
// automatiche e non mostra avvisi. Riaccendendolo torna tutto com'era.
func setPaused(on bool) string {
	setMu.Lock()
	paused, a, w, pa, pw := settings.Paused, settings.Autostart, settings.Weekly, settings.PrevAuto, settings.PrevWeekly
	setMu.Unlock()
	if on == paused {
		return "ok"
	}
	if on {
		autoClickStop()
		acDisarm()
		_, _ = runHidden("schtasks.exe", "/Delete", "/TN", taskAvvio, "/F")
		_, _ = runHidden("schtasks.exe", "/Delete", "/TN", taskWeek, "/F")
		_, _ = runHidden("schtasks.exe", "/Delete", "/TN", taskProfilo, "/F")
		updateSettings(func(s *Settings) {
			s.PrevAuto, s.PrevWeekly = a, w
			s.Autostart, s.Weekly = false, false
			s.Paused = true
		})
		return "ok"
	}
	updateSettings(func(s *Settings) { s.Paused = false })
	if pa {
		setAutostart(true)
	}
	if pw {
		setWeekly(true)
	}
	profileTask(profileIsOn())
	return "ok"
}

var validThemes = map[string]bool{"tasso": true, "aurora": true, "lava": true, "neon": true, "carbonio": true, "belva": true, "ghiaccio": true, "sabbia": true, "ufficio": true, "carta": true, "notte": true, "salvia": true, "lavanda": true, "ardesia": true}

// Avvisi: cosa l'utente vuole che Tasso gli segnali. Se non impostato, sono tutti attivi.
var alertKeys = []string{"temp", "disk", "startup", "ram", "health"}

func defaultAlerts() map[string]bool {
	m := map[string]bool{}
	for _, k := range alertKeys {
		m[k] = true
	}
	return m
}

func setAlert(key string, on bool) string {
	valid := false
	for _, k := range alertKeys {
		if k == key {
			valid = true
		}
	}
	if !valid {
		return "avviso sconosciuto"
	}
	updateSettings(func(s *Settings) {
		if s.Alerts == nil {
			s.Alerts = defaultAlerts()
		}
		s.Alerts[key] = on
	})
	return "ok"
}

func setTheme(id, look string) string {
	if !validThemes[id] {
		return "tema sconosciuto"
	}
	// Gaming e Ufficio ricordano ognuna il suo tema (look arriva dall'interfaccia: "ufficio" o "gioco").
	updateSettings(func(s *Settings) {
		if look == "ufficio" || (look == "" && s.Profile == "ufficio") {
			s.ThemeOffice = id
		} else {
			s.Theme = id
		}
	})
	return "ok"
}

// setLook cambia solo la modalità di Tasso (Gaming o Ufficio): schermate, tema e strumenti
// mostrati. Non tocca il PC (piano energetico, pulizia programmata): quello lo fa setProfile
// alla prima apertura.
func setLook(p string) string {
	if p != "gioco" && p != "ufficio" {
		return "modalità sconosciuta"
	}
	updateSettings(func(s *Settings) {
		s.Onboarded = true
		s.Profile = p
	})
	return "ok"
}

func setSmartAlerts(on bool) string {
	updateSettings(func(s *Settings) { v := on; s.SmartAlerts = &v })
	return "ok"
}

var (
	setMu    sync.Mutex
	settings = Settings{Mode: "normale"}
	// settingsReadOnly: il file c'era ma non si è potuto leggere; per non perderlo non lo riscrivo.
	settingsReadOnly bool
)

func settingsPath() string { return filepath.Join(appDir, "settings.json") }

func loadSettings() {
	b, err := os.ReadFile(settingsPath())
	for i := 0; i < 5 && err != nil && !os.IsNotExist(err); i++ {
		// File occupato un attimo (antivirus, backup): riprovo invece di ripartire da zero.
		time.Sleep(400 * time.Millisecond)
		b, err = os.ReadFile(settingsPath())
	}
	if err != nil && !os.IsNotExist(err) {
		settingsReadOnly = true // non lo sovrascrivo: dentro ci sono le impostazioni originali di Windows
	}
	if err == nil {
		if json.Unmarshal(b, &settings) != nil {
			// File rovinato: lo metto da parte invece di sovrascriverlo (dentro c'è il backup delle impostazioni).
			_ = os.WriteFile(settingsPath()+".bad", b, 0o644)
			settings = Settings{Mode: "normale"}
		}
	}
	if settings.Mode == "" {
		settings.Mode = "normale"
	}
	if settings.LevelsV < 5 {
		// Dalla 2.4: i livelli sono saliti di uno (è nata la Classica).
		if settings.LastClean != nil && settings.LastClean.Level > 0 {
			settings.LastClean.Level++
		}
		settings.LevelsV = 5
		if err == nil {
			saveSettings() // subito, così al prossimo avvio non li alzo di nuovo
		}
	}
	fixSettingsDefaults()
}

// fixSettingsDefaults mette i valori di partenza dove mancano (temi, avvisi).
func fixSettingsDefaults() {
	if !validThemes[settings.Theme] {
		settings.Theme = "tasso"
	}
	if settings.ThemeOffice != "" && !validThemes[settings.ThemeOffice] {
		settings.ThemeOffice = ""
	}
	if settings.SmartAlerts == nil {
		v := true
		settings.SmartAlerts = &v
	}
	if settings.Alerts == nil {
		settings.Alerts = defaultAlerts()
	} else {
		for _, k := range alertKeys {
			if _, ok := settings.Alerts[k]; !ok {
				settings.Alerts[k] = true
			}
		}
	}
}

// saveSettings scrive le impostazioni. Restituisce true se invece ha ricaricato il file vero
// (era illeggibile all'avvio): chi ha appena fatto un cambio lo deve rifare su quello.
func saveSettings() bool {
	if settingsReadOnly {
		// All'avvio il file non si leggeva: prima di scriverci sopra ne salvo una copia.
		old, err := os.ReadFile(settingsPath())
		if err != nil && !os.IsNotExist(err) {
			return false
		}
		if err == nil {
			_ = os.WriteFile(settingsPath()+".prima", old, 0o644)
			// Il file adesso si legge: quello è quello vero. Lo tengo così com'è invece di
			// sovrascriverlo con le impostazioni di partenza di questa sessione (si perde al massimo
			// l'ultimo cambio fatto adesso, non tutto il resto).
			var o Settings
			if json.Unmarshal(old, &o) == nil {
				if o.Mode == "" {
					o.Mode = "normale"
				}
				settings = o
				settingsReadOnly = false
				fixSettingsDefaults()
				return true
			}
		}
		settingsReadOnly = false
	}
	b, _ := json.MarshalIndent(settings, "", "  ")
	// Scrittura sicura: prima un file temporaneo, poi lo scambio (niente file a metà se il PC si spegne).
	tmp := settingsPath() + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		if os.Rename(tmp, settingsPath()) != nil {
			_ = os.WriteFile(settingsPath(), b, 0o644)
			_ = os.Remove(tmp)
		}
	}
	return false
}

func getSettings() map[string]interface{} {
	setMu.Lock()
	defer setMu.Unlock()
	b, _ := json.Marshal(settings)
	m := map[string]interface{}{}
	_ = json.Unmarshal(b, &m)
	// Il token del bot (anche se protetto) non va mai all'interfaccia: basta sapere se c'è.
	m["discordBotSaved"] = settings.DiscordBot != ""
	delete(m, "discordBot")
	delete(m, "vault")
	return m
}

func updateSettings(f func(s *Settings)) {
	setMu.Lock()
	f(&settings)
	if saveSettings() {
		f(&settings) // rifaccio il cambio sulle impostazioni vere appena ricaricate
		saveSettings()
	}
	setMu.Unlock()
}

// setProfile applica il profilo scelto alla prima apertura (gioco, ufficio, entrambi).
func setProfile(p string) string {
	switch p {
	case "gioco", "ufficio", "entrambi":
	default:
		return "profilo sconosciuto"
	}
	updateSettings(func(s *Settings) {
		s.Onboarded = true
		s.Profile = p
	})
	switch p {
	case "gioco":
		go setMode("gaming")
		setWeekly(false)
	case "ufficio":
		go setMode("normale")
		setWeekly(true)
	case "entrambi":
		go setMode("normale")
		setWeekly(true)
	}
	return "ok"
}

// sysExe: i programmi di Windows (schtasks, powercfg, taskkill, shutdown, powershell...) li avvio
// sempre dalla cartella di sistema, mai da un'altra cartella del PATH dove qualcuno potrebbe
// averne messo uno finto (Tasso gira come amministratore).
func sysExe(name string) string {
	if strings.ContainsAny(name, `\/:`) {
		return name
	}
	sys, err := windows.GetSystemDirectory()
	if err != nil || sys == "" {
		return name
	}
	p := filepath.Join(sys, name)
	if strings.EqualFold(name, "powershell.exe") {
		p = filepath.Join(sys, "WindowsPowerShell", "v1.0", "powershell.exe")
	}
	if fileExists(p) {
		return p
	}
	return name
}

// runHiddenTimeout è come runHidden ma chiude il programma se ci mette troppo (es. nvidia-smi bloccato).
func runHiddenTimeout(d time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	cmd := exec.CommandContext(ctx, sysExe(name), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runHidden(name string, args ...string) (string, error) {
	cmd := exec.Command(sysExe(name), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

const (
	taskAvvio = "Tasso\\Avvio con Windows"
	taskWeek  = "Tasso\\Pulizia settimanale"
)

// taskExe: le attività pianificate partono con i permessi da amministratore, quindi non
// devono puntare a una cartella dove chiunque può scrivere (es. Download). Copio Tasso.exe
// in Programmi\Tasso, che solo un amministratore può modificare, e uso quella copia.
// Se la copia non si può fare restituisce "": meglio nessuna attività che una che fa partire
// come amministratore un file che chiunque può sostituire.
// Accanto alla copia c'è VERSIONE: non rimetto mai una versione vecchia sopra una più nuova
// (es. dopo un aggiornamento fatto dalla copia in Programmi).
func taskExe() string {
	pf := os.Getenv("ProgramFiles")
	if pf == "" || exePath == "" {
		return ""
	}
	dir := filepath.Join(pf, "Tasso")
	dst := filepath.Join(dir, "Tasso.exe")
	if strings.EqualFold(filepath.Clean(exePath), dst) {
		return dst
	}
	verFile := filepath.Join(dir, "VERSIONE")
	if fileExists(dst) {
		if v, err := os.ReadFile(verFile); err == nil && verNewer(strings.TrimSpace(string(v)), appVersion) {
			return dst
		}
	}
	src, err := os.ReadFile(exePath)
	if err != nil {
		return existingOr(dst)
	}
	if old, err := os.ReadFile(dst); err == nil && bytes.Equal(old, src) {
		_ = os.WriteFile(verFile, []byte(appVersion), 0o644)
		return dst
	}
	if os.MkdirAll(dir, 0o755) != nil {
		return ""
	}
	if installProgramCopy(dir, src, appVersion) != nil {
		return existingOr(dst)
	}
	return dst
}

func existingOr(dst string) string {
	if fileExists(dst) {
		return dst
	}
	return ""
}

// installProgramCopy mette exe in Programmi\Tasso\Tasso.exe (prima in un file a parte, poi lo scambia)
// e scrive la versione in VERSIONE.
var progCopyMu sync.Mutex

func installProgramCopy(dir string, exe []byte, ver string) error {
	progCopyMu.Lock()
	defer progCopyMu.Unlock()
	dst := filepath.Join(dir, "Tasso.exe")
	tmp := dst + ".new"
	if err := os.WriteFile(tmp, exe, 0o755); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	old := dst + ".old"
	_ = os.Remove(old)
	if fileExists(dst) {
		// Se la copia in Programmi è aperta (Tasso partito da lì) la rinomino: Windows lo permette.
		if err := os.Rename(dst, old); err != nil {
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Rename(old, dst)
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Remove(old) // se è in uso resta lì e si toglie la volta dopo
	return os.WriteFile(filepath.Join(dir, "VERSIONE"), []byte(ver), 0o644)
}

// logonTaskScript crea un'attività che parte quando entri in Windows: anche a batteria,
// senza essere chiusa se stacchi il caricatore e senza tempo massimo (schtasks da solo
// metterebbe "solo con la corrente" e "chiudi dopo 3 giorni").
const logonTaskScript = `
try {
  $who = [Security.Principal.WindowsIdentity]::GetCurrent().Name
  $act = New-ScheduledTaskAction -Execute ([string]$A.exe) -Argument ([string]$A.arg)
  $trg = New-ScheduledTaskTrigger -AtLogOn -User $who
  $set = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit ([TimeSpan]::Zero) -MultipleInstances IgnoreNew
  $prn = New-ScheduledTaskPrincipal -UserId $who -LogonType Interactive -RunLevel Highest
  Register-ScheduledTask -TaskPath '\Tasso\' -TaskName ([string]$A.name) -Action $act -Trigger $trg -Settings $set -Principal $prn -Force -ErrorAction Stop | Out-Null
  Say 'TASK_OK'
} catch { Say ('Errore: ' + $_.Exception.Message) }
`

// createLogonTask: name è il nome dentro la cartella Tasso (es. "Avvio con Windows").
func createLogonTask(name, arg string) bool {
	exe := taskExe()
	if exe == "" {
		return false
	}
	ok := false
	runPSArgs(logonTaskScript, map[string]string{"name": name, "exe": exe, "arg": arg}, 2*time.Minute, func(m string) {
		if m == "TASK_OK" {
			ok = true
		}
	})
	return ok
}

func setAutostart(on bool) string {
	if on {
		if !createLogonTask("Avvio con Windows", "--min") {
			if taskExe() == "" {
				_, _ = runHidden("schtasks.exe", "/Delete", "/TN", taskAvvio, "/F")
			}
			return "Non sono riuscito a impostare l'avvio con Windows."
		}
	} else {
		_, _ = runHidden("schtasks.exe", "/Delete", "/TN", taskAvvio, "/F")
	}
	updateSettings(func(s *Settings) { s.Autostart = on })
	profileTask(profileIsOn())
	return "ok"
}

// refreshScheduledTasks aggiorna il percorso di Tasso.exe nelle attività pianificate,
// nel caso il file sia stato spostato.
func refreshScheduledTasks() {
	time.Sleep(3 * time.Second)
	setMu.Lock()
	a, w, p := settings.Autostart, settings.Weekly, settings.Paused
	setMu.Unlock()
	if p {
		return
	}
	if a {
		setAutostart(true)
	}
	if w {
		setWeekly(true)
	}
	if profileIsOn() {
		profileTask(true)
	}
}
