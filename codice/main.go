// Tasso - pulizia, manutenzione e controllo del PC. Creato da Sergio Tassoni.
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

//go:embed ui/index.html
var indexHTML string

const appVersion = "3.0"

var (
	wv       webview2.WebView
	appDir   string
	exePath  string
	user32   = windows.NewLazySystemDLL("user32.dll")
	pShowWin = user32.NewProc("ShowWindow")
	pFindWin = user32.NewProc("FindWindowW")
	pSetFg   = user32.NewProc("SetForegroundWindow")
	pMsgBox  = user32.NewProc("MessageBoxW")
)

func main() {
	hardenProcess()
	// Profilo gioco all'accensione del PC (Tasso non parte con Windows): rimetto il profilo e chiudo,
	// senza finestra e senza tenere occupato Tasso, così se lo apri tu si apre normalmente.
	for _, a := range os.Args[1:] {
		if a == "--profile" {
			runProfileOnly()
			return
		}
	}
	// Una sola copia di Tasso alla volta: se è già aperto lo porta in primo piano.
	mname, _ := windows.UTF16PtrFromString("Local\\TassoSingleInstance")
	// La copia aperta tiene il mutex per tutta la sua vita (initialOwner = true).
	h, err := windows.CreateMutex(nil, true, mname)
	if err == windows.ERROR_ALREADY_EXISTS {
		// Pulizia programmata con Tasso già aperto: la passo alla copia aperta, senza disturbare.
		for _, a := range os.Args[1:] {
			if strings.HasPrefix(a, "--auto=") && askRunningInstance(strings.TrimPrefix(a, "--auto=")) {
				return
			}
		}
		// Nessuna finestra: o Tasso si sta ancora aprendo, o è la copia veloce del Profilo gioco
		// all'accensione. Aspetto (al massimo 2 minuti) che la finestra compaia o che l'altra copia finisca.
		// Dopo un aggiornamento la copia vecchia si sta chiudendo: la aspetto e basta.
		afterUpd, autoArg := false, ""
		for _, a := range os.Args[1:] {
			afterUpd = afterUpd || a == "--after-update" || a == "--restart"
			if strings.HasPrefix(a, "--auto=") {
				autoArg = strings.TrimPrefix(a, "--auto=")
			}
		}
		got := false
		for i := 0; i < 120 && !got; i++ {
			if autoArg != "" {
				// Pulizia programmata: la copia aperta forse si sta ancora avviando, riprovo a passargliela.
				if askRunningInstance(autoArg) {
					return
				}
			} else if !afterUpd && bringToFront() {
				return
			}
			ev, _ := windows.WaitForSingleObject(h, 1000)
			got = ev == windows.WAIT_OBJECT_0 || ev == windows.WAIT_ABANDONED
		}
		if !got {
			return
		}
	}
	defer windows.CloseHandle(h)

	exePath, _ = os.Executable()
	appDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "Tasso")
	_ = os.MkdirAll(filepath.Join(appDir, "logs"), 0o755)
	startCrashLog()
	webview2.PanicHook = logPanic
	loadSettings()

	autoLevel := 0
	minimized := false
	for _, a := range os.Args[1:] {
		switch {
		case strings.HasPrefix(a, "--auto="):
			switch strings.TrimPrefix(a, "--auto=") {
			case "1":
				autoLevel = 1
			case "2":
				autoLevel = 2
			case "3":
				autoLevel = 3
			case "4":
				autoLevel = 4
			case "5":
				autoLevel = 5
			}
		case a == "--min":
			minimized = true
		case a == "--scheduled":
			scheduledRun.Store(true)
		}
	}

	setMu.Lock()
	if settings.Paused {
		autoLevel = 0 // Tasso è spento: niente pulizie automatiche
	}
	setMu.Unlock()
	if scheduledRun.Load() && autoLevel == 0 {
		return // attività pianificata con Tasso spento: non apro neanche la finestra
	}
	if scheduledRun.Load() {
		markAutoRun(autoLevel, 30*time.Minute) // su un PC lento appena acceso la finestra può metterci un po'

	}
	listenAutoRequests()

	w := newWindow()
	if w == nil && dllSafe {
		// Per sicurezza le DLL si cercano solo in System32. Se così la finestra non si apre
		// (caso raro), riprovo con la ricerca sicura standard di Windows. WebView2Loader.dll
		// resta comunque solo quella interna (vedi third_party/go-webview2).
		_ = windows.SetDefaultDllDirectories(windows.LOAD_LIBRARY_SEARCH_DEFAULT_DIRS)
		w = newWindow()
	}
	if w == nil {
		messageBox("Per funzionare Tasso ha bisogno di Microsoft Edge WebView2, un componente gratuito di Windows.\n\nPremi OK: si apre la pagina ufficiale Microsoft per installarlo, poi riapri Tasso.", "Tasso")
		openURL("https://go.microsoft.com/fwlink/p/?LinkId=2124703")
		return
	}
	wv = w
	defer w.Destroy()

	bindAll(w)
	startLiveMonitor()
	startGameWatcher()
	startOverlay()
	startDiscordReporter()
	go refreshScheduledTasks()
	go cleanOldExe()
	// Resto di un "Cancella lo spazio libero" interrotto (per es. PC spento a metà): può occupare tanti GB.
	go func() {
		defer guard("main.go:153")
		if sd := os.Getenv("SystemDrive"); sd != "" {
			p := filepath.Join(sd+`\`, "EFSTMPWP")
			if fi, err := os.Lstat(p); err == nil && fi.IsDir() && fi.Mode()&os.ModeSymlink == 0 && fi.Mode()&os.ModeIrregular == 0 {
				_ = os.RemoveAll(p)
			}
		}
	}()
	if profileIsOn() {
		go func() {
			defer guard("main.go:162")
			time.Sleep(6 * time.Second)
			if fixed := profileCheck(); len(fixed) > 0 {
				emit("onProfileCheck", map[string]interface{}{"fixed": fixed, "profile": profileCopy()})
			}
		}()
	}

	boot := map[string]interface{}{
		"version":   appVersion,
		"lang":      curLang(),
		"autoLevel": autoLevel,
		"cassetta":  strings.HasPrefix(cassettaURL, "https://"),
		"settings":  getSettings(),
		"steps":     stepList(),
	}
	b, _ := json.Marshal(boot)
	w.Init("window.__TASSO_BOOT__ = " + string(b) + "; window.__TASSO_I18N__ = " + uiDictJSON() + ";")
	w.SetHtml(indexHTML)
	if minimized {
		pShowWin.Call(uintptr(w.Window()), 6) // SW_MINIMIZE
	}
	w.Run()
}

func newWindow() webview2.WebView {
	return webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  filepath.Join(appDir, "WebView2"),
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "Tasso",
			Width:  1220,
			Height: 800,
			IconId: 2,
			Center: true,
		},
	})
}

var dllSafe bool

// hardenProcess: Tasso gira come amministratore, quindi all'avvio chiude due porte che un
// programma normale potrebbe usare per prendere il controllo di Tasso:
//   - le DLL si caricano solo da System32, mai dalla cartella di Tasso.exe (es. Download), dove
//     qualcuno potrebbe aver messo una DLL falsa;
//   - le variabili WEBVIEW2_* dell'utente (cambiabili senza permessi) potrebbero aprire la finestra
//     di Tasso ad altri programmi o farle usare un browser finto: le tolgo prima di aprirla.
func hardenProcess() {
	dllSafe = windows.SetDefaultDllDirectories(windows.LOAD_LIBRARY_SEARCH_SYSTEM32) == nil
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i > 0 && strings.HasPrefix(strings.ToUpper(kv[:i]), "WEBVIEW2_") {
			_ = os.Unsetenv(kv[:i])
		}
	}
}

// startCrashLog: se Tasso si chiude per un errore grave, Go scrive il motivo in logs\crash.txt
// (con data e versione), così si può capire cosa è successo sul PC di chi lo usa.
// logPanic: scrive in logs\crash.txt un errore grave che Tasso ha fermato senza chiudersi.
func logPanic(where string, r interface{}, stack []byte) {
	p := filepath.Join(appDir, "logs", "crash.txt")
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "--- Errore fermato (Tasso resta aperto) il %s in %s: %v\r\n%s\r\n", time.Now().Format("02/01/2006 15:04:05"), where, r, stack)
}

// guard: da mettere con defer all'inizio di ogni goroutine, così un errore non chiude Tasso.
func guard(where string) {
	if r := recover(); r != nil {
		logPanic(where, r, debug.Stack())
	}
}

func startCrashLog() {
	p := filepath.Join(appDir, "logs", "crash.txt")
	if fi, err := os.Stat(p); err == nil && fi.Size() > 1<<20 {
		_ = os.Rename(p, p+".old") // non lo lascio crescere all'infinito
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close() // SetCrashOutput tiene una sua copia del file
	// Una riga a ogni apertura: se poi sotto compare un errore, si sa con quale versione e quando.
	_, _ = f.WriteString("--- Tasso " + appVersion + " aperto il " + time.Now().Format("02/01/2006 15:04:05") + "\r\n")
	_ = debug.SetCrashOutput(f, debug.CrashOptions{})
}

// emit invia un evento all'interfaccia: window.tasso.<name>(payload)
func emit(name string, payload interface{}) {
	if wv == nil {
		return
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	// window.__tx traduce i messaggi in inglese (se la lingua è l'inglese; in italiano non fa niente).
	js := "window.tasso && window.tasso." + name + " && window.tasso." + name + "(window.__tx ? window.__tx(" + string(b) + ") : " + string(b) + ");"
	wv.Dispatch(func() { wv.Eval(js) })
}

func bringToFront() bool {
	// Cerco la finestra di Tasso per classe ("webview") e titolo, così non confondo
	// la finestra di Esplora file della cartella "Tasso".
	c, _ := windows.UTF16PtrFromString("webview")
	t, _ := windows.UTF16PtrFromString("Tasso")
	hwnd, _, _ := pFindWin.Call(uintptr(unsafe.Pointer(c)), uintptr(unsafe.Pointer(t)))
	if hwnd != 0 {
		pShowWin.Call(hwnd, 9) // SW_RESTORE
		pSetFg.Call(hwnd)
		return true
	}
	return false
}

func messageBox(text, title string) {
	t, _ := windows.UTF16PtrFromString(trGo(text))
	c, _ := windows.UTF16PtrFromString(title)
	pMsgBox.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x40)
}

func openURL(u string) {
	if isWebURL(u) && openPath(explorerExe(), "\""+u+"\"") {
		return // aperto come utente normale, non da amministratore
	}
	verb, _ := windows.UTF16PtrFromString("open")
	target, _ := windows.UTF16PtrFromString(u)
	_ = windows.ShellExecute(0, verb, target, nil, nil, windows.SW_SHOWNORMAL)
}

func openPath(p string, args string) bool {
	verb, _ := windows.UTF16PtrFromString("open")
	target, _ := windows.UTF16PtrFromString(p)
	var a *uint16
	if args != "" {
		a, _ = windows.UTF16PtrFromString(args)
	}
	return windows.ShellExecute(0, verb, target, a, nil, windows.SW_SHOWNORMAL) == nil
}

func bindAll(w webview2.WebView) {
	_ = w.Bind("goGetSettings", func() map[string]interface{} { return getSettings() })
	_ = w.Bind("goSetProfile", func(id, p string) string { return bgCall("sched", id, func() string { return setProfile(p) }) })
	_ = w.Bind("goSetLook", func(p string) string { return setLook(p) })
	_ = w.Bind("goAnalyze", func() string { go analyze(); return "ok" })
	_ = w.Bind("goHardware", func() string { go hardwareInfo(); return "ok" })
	_ = w.Bind("goLive", func() map[string]interface{} { return liveStats() })
	_ = w.Bind("goStartLevel", func(level int, off bool) string { return startLevel(level, off) })
	_ = w.Bind("goStartSteps", func(ids []string, off bool) string { return startSteps(ids, 0, off) })
	_ = w.Bind("goCancelShutdown", func() string { return cancelShutdown() })
	_ = w.Bind("goSetTheme", func(id, look string) string { return setTheme(id, look) })
	_ = w.Bind("goSetStats", func(on bool) string { return setStats(on) })
	_ = w.Bind("goStatsHW", func(osName, cpu, gpu string, ram float64) string { return statsHardware(osName, cpu, gpu, ram) })
	_ = w.Bind("goSetSmartAlerts", func(on bool) string { return setSmartAlerts(on) })
	_ = w.Bind("goSetPaused", func(id string, on bool) string { return bgCall("sched", id, func() string { return setPaused(on) }) })
	_ = w.Bind("goRestore", func(pause bool) string { return restoreAndMaybePause(pause) })
	_ = w.Bind("goQuit", func() string {
		go func() {
			defer guard("main.go:320")
			time.Sleep(150 * time.Millisecond)
			wv.Dispatch(func() { wv.Terminate() })
		}()
		return "ok"
	})
	_ = w.Bind("goStartupList", func() string { return async("startup", startupList) })
	_ = w.Bind("goStartupSet", func(scope, name string, enable bool) string {
		return async("startup", func() { startupSet(scope, name, enable) })
	})
	_ = w.Bind("goPrograms", func() string { return async("programs", programsList) })
	_ = w.Bind("goUninstall", func(key string) string { return async("uninstall", func() { uninstallProgram(key) }) })
	_ = w.Bind("goRemoveLeftovers", func(paths []string, keys []string) string {
		return async("leftovers", func() { removeLeftovers(paths, keys) })
	})
	_ = w.Bind("goBackup", func() string { return async("backup", backup) })
	_ = w.Bind("goDesktopShortcut", func() string { return desktopShortcut() })
	_ = w.Bind("goPerfScan", func() string { return async("perf", perfScan) })
	_ = w.Bind("goPerfProfile", func(id string, on bool) string {
		return bgCall("perf", id, func() string { return setPerfProfile(on) })
	})
	_ = w.Bind("goPerfTool", func(id string) string { return openPerfTool(id) })
	_ = w.Bind("goSetAlert", func(key string, on bool) string { return setAlert(key, on) })
	_ = w.Bind("goBloat", func() string { return async("bloat", bloatScan) })
	_ = w.Bind("goBloatRemove", func(ids []string) string { return async("bloat", func() { bloatRemove(ids) }) })
	_ = w.Bind("goScanFiles", func(extra string, minMB int) string { return async("files", func() { filesScan(extra, minMB) }) })
	_ = w.Bind("goPickScanFolder", func() string { return async("pickfolder", pickScanFolder) })
	_ = w.Bind("goRecycle", func(paths []string, kind string) string {
		return async("recycle", func() { recyclePaths(paths, kind) })
	})
	_ = w.Bind("goPickFiles", func() []string { return pickFiles() })
	_ = w.Bind("goShred", func(paths []string) string { return async("shred", func() { shredFiles(paths) }) })
	_ = w.Bind("goCancel", func() string { cancelRun(); return "ok" })
	_ = w.Bind("goIsRunning", func() bool { return isRunning() })
	_ = w.Bind("goSetMode", func(id string) string { go setMode(id); return "ok" })
	_ = w.Bind("goSetAutostart", func(id string, on bool) string { return bgCall("sched", id, func() string { return setAutostart(on) }) })
	_ = w.Bind("goSetWeekly", func(id string, on bool) string { return bgCall("sched", id, func() string { return setWeekly(on) }) })
	_ = w.Bind("goSetWeeklyPlan", func(id string, day, hour, level, every int) string {
		return bgCall("sched", id, func() string { return setWeeklyPlan(day, hour, level, every) })
	})
	_ = w.Bind("goOpenLogs", func() string { openPath(filepath.Join(appDir, "logs"), ""); return "ok" })
	_ = w.Bind("goOpenURL", func(u string) string {
		if strings.HasPrefix(u, "https://") {
			openURL(u)
		}
		return "ok"
	})
	_ = w.Bind("goBiosSearch", func(board string) string {
		openURL("https://www.google.com/search?q=" + urlQuery(board+" BIOS download ufficiale"))
		return "ok"
	})
	_ = w.Bind("goGpuPanel", func(vendor string) string { return openGpuPanel(vendor) })
	_ = w.Bind("goHealth", func() string { return async("health", healthCheck) })
	_ = w.Bind("goDetect", func() string { return async("detect", detectProblems) })
	_ = w.Bind("goSpeedTest", func() string { return async("speed", speedTest) })
	_ = w.Bind("goUpdatesList", func() string { return async("updates", updatesList) })
	_ = w.Bind("goUpdatesApply", func(ids []string) string { return async("updates", func() { updatesApply(ids) }) })
	_ = w.Bind("goBench", func() string { return async("bench", benchNow) })
	_ = w.Bind("goBenchHistory", func() []benchResult { return benchHistory() })
	_ = w.Bind("goSetBenchAround", func(on bool) string { return setBenchAround(on) })
	_ = w.Bind("goChanges", func() map[string]interface{} { return changesList() })
	_ = w.Bind("goUndoChange", func(kind, id string) string { return async("restore", func() { undoChange(kind, id) }) })
	_ = w.Bind("goGamePing", func(region string) string { return async("ping", func() { gamePing(region) }) })
	_ = w.Bind("goNetUsage", func() string { return async("net", netUsage) })
	_ = w.Bind("goWifi", func() string { return async("wifi", wifiList) })
	_ = w.Bind("goDiskMap", func() string { return async("diskmap", diskMap) })
	_ = w.Bind("goSetDiscord", func(url string, on bool) string { return setDiscord(url, on) })
	_ = w.Bind("goDiscordTest", func() string { return async("discord", discordSendNow) })
	_ = w.Bind("goDiscordEvery", func(h int) string { return setDiscordEvery(h) })
	_ = w.Bind("goDiscordBot", func(token string, on bool) string { return async("discordbot", func() { setDiscordBot(token, on) }) })
	_ = w.Bind("goDiscordBotState", func() map[string]interface{} { return getBotState() })
	_ = w.Bind("goSetAutoGame", func(on bool) string { return setAutoGame(on) })
	_ = w.Bind("goOverlaySet", func(on bool, pos string) string { return setOverlay(on, pos) })
	_ = w.Bind("goOverlayShow", func(show bool) string { return overlayShow(show) })
	_ = w.Bind("goLibrary", func() string { return async("library", scanLibrary) })
	_ = w.Bind("goLaunchGame", func(id string) string { return launchGame(id) })
	_ = w.Bind("goGamePower", func() map[string]interface{} { return gamePowerList() })
	_ = w.Bind("goSetGamePower", func(id, mode string) string {
		return async("gpower", func() { emit("onGamePower", setGamePower(id, mode)) })
	})
	_ = w.Bind("goProfile", func() map[string]interface{} { return profileInfo() })
	_ = w.Bind("goProfileSet", func(r profileReq) string {
		return async("profile", func() { profileSet(r.On, r.Mode, r.Cmds, r.Overlay, r.AutoGame, r.Standby) })
	})
	_ = w.Bind("goGamingState", func() map[string]interface{} { return gamingState() })
	_ = w.Bind("goGamingSet", func(id string, on bool) string {
		if !on {
			profileDropCmd(id) // spento a mano: il profilo gioco non lo riaccende più
		}
		return gamingSet(id, on)
	})
	_ = w.Bind("goPurgeStandby", func() string { return purgeStandby() })
	_ = w.Bind("goSetLang", func(l string) string { return setLang(l) })
	_ = w.Bind("goGpuReset", func() string { return resetGpuDriver() })
	_ = w.Bind("goGameTune", func(id string) string { return async("tune", func() { gameTuneInfo(id) }) })
	_ = w.Bind("goGameTuneApply", func(id, preset string, o tuneOpts) string {
		return async("tune", func() { gameTuneApply(id, preset, o) })
	})
	_ = w.Bind("goGameTuneUndo", func(id string) string { return async("tune", func() { gameTuneUndo(id) }) })
	_ = w.Bind("goBoostScan", func() string { return async("boostscan", boostScan) })
	_ = w.Bind("goBoostStart", func(names []string) string { return async("boost", func() { boostStart(names) }) })
	_ = w.Bind("goBoostEnd", func() string { return async("boost", boostEnd) })
	_ = w.Bind("goAutoClickSet", func(c acConfig) string { return autoClickSet(c) })
	_ = w.Bind("goAutoClickStart", func() string { return autoClickStart(false) })
	_ = w.Bind("goAutoClickStop", func() string { return autoClickStop() })
	_ = w.Bind("goAutoClickKey", func(vk, mods uint32) string { return setAutoClickKey(vk, mods) })
	_ = w.Bind("goAutoClickArm", func() bool { return acArm() })
	_ = w.Bind("goSelfUpdate", func() string { return async("update", selfUpdate) })
	_ = w.Bind("goCheckUpdate", func() string { return async("update", func() { emit("onUpdate", checkUpdate()) }) })
	_ = w.Bind("goOpenDiscord", func() string { openURL(discordURL); return "ok" })
	_ = w.Bind("goFeedbackPhoto", func() map[string]interface{} { return pickFeedbackPhoto() })
	_ = w.Bind("goFeedbackSend", func(kind, text, contact, pc string, photos []string, stars int, publish bool, name string) string {
		return async("feedback", func() { sendFeedback(kind, text, contact, pc, photos, stars, publish, name) })
	})
	_ = w.Bind("goFeedbackMail", func(kind, text, pc string) string { return feedbackMailOpen(kind, text, pc) })
	_ = w.Bind("goFeedbackAsked", func() string { return feedbackAsked() })
	_ = w.Bind("goIcons", func(req map[string][]string) string { return queueIcons(req) })
	_ = w.Bind("goDrivers", func() string { return async("drivers", driversScan) })
	_ = w.Bind("goDriversCheck", func() string { return async("drvcheck", driversCheck) })
	_ = w.Bind("goDriversInstall", func(ids []string) string { return async("drvcheck", func() { driversInstall(ids) }) })
	_ = w.Bind("goDriverFix", func(id string) string { return async("drvfix", func() { driverFix(id) }) })
	_ = w.Bind("goDriversAuto", func(on bool) string { return setDriversAuto(on) })
	_ = w.Bind("goVendorPage", func(id string) string { return openVendorPage(id) })
	_ = w.Bind("goRevealFile", func(p string) string { return openFolderOf(p) })
	_ = w.Bind("goAutoClickPick", func() string { return async("acpick", autoClickPick) })
	_ = w.Bind("goOpenFolder", func(p string) string {
		// Solo cartelle che esistono davvero: niente programmi o comandi.
		if st, err := os.Stat(p); err != nil || !st.IsDir() {
			return "no"
		}
		openUser(p, "")
		return "ok"
	})
	_ = w.Bind("goOpenWin", func(id string) string { return openWinPage(id) })
	_ = w.Bind("goVaultStatus", func() map[string]interface{} { return vaultStatus() })
	_ = w.Bind("goVaultCreate", func(pin string) string { return vaultCreate(pin) })
	_ = w.Bind("goVaultUnlock", func(pin string) string { return vaultUnlock(pin) })
	_ = w.Bind("goVaultLock", func() string { return vaultLock() })
	_ = w.Bind("goVaultList", func() map[string]interface{} { return vaultList() })
	_ = w.Bind("goVaultSave", func(e vaultEntry) string { return vaultSave(e) })
	_ = w.Bind("goVaultDelete", func(id string) string { return vaultDelete(id) })
	_ = w.Bind("goVaultReveal", func(id string) string { return vaultReveal(id) })
	_ = w.Bind("goVaultCopy", func(id, what string) string { return vaultCopy(id, what) })
	_ = w.Bind("goVaultChangePin", func(o, n string) string { return vaultChangePIN(o, n) })
	_ = w.Bind("goVaultReset", func() string { return vaultReset() })
	_ = w.Bind("goCopySecret", func(t string) string { return copySecret(t, true) })
	_ = w.Bind("goVaultGen", func(n int, sym bool) string { return genPassword(n, sym) })
	_ = w.Bind("goVaultPwned", func(id string) string {
		return async("pwned", func() { r := vaultPwned(id); r["id"] = id; emit("onPwned", r) })
	})
	_ = w.Bind("goPwnedCheck", func(pw string) string {
		return async("pwned", func() { r := pwnedCount(pw); r["id"] = ""; emit("onPwned", r) })
	})
	_ = w.Bind("goRestart", func() string {
		runHidden("shutdown.exe", "/r", "/t", "15", "/c", "Tasso riavvia il PC tra 15 secondi per completare la manutenzione.")
		return "ok"
	})
}

func urlQuery(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		switch {
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.':
			b.WriteByte(c)
		case c == ' ':
			b.WriteByte('+')
		default:
			b.WriteString("%")
			b.WriteString(strings.ToUpper(string("0123456789abcdef"[c>>4])) + strings.ToUpper(string("0123456789abcdef"[c&15])))
		}
	}
	return b.String()
}

func openGpuPanel(vendor string) string {
	v := strings.ToLower(vendor)
	pf := os.Getenv("ProgramFiles")
	switch {
	case strings.Contains(v, "nvidia"):
		app := filepath.Join(pf, "NVIDIA Corporation", "NVIDIA app", "CEF", "NVIDIA app.exe")
		if fileExists(app) && openPath(app, "") {
			return "ok"
		}
		if openPath("explorer.exe", "shell:AppsFolder\\NVIDIACorp.NVIDIAControlPanel_56jk4w8tv9wjp!NVIDIACorp.NVIDIAControlPanel") {
			return "ok"
		}
		openURL("https://www.nvidia.com/it-it/software/nvidia-app/")
		return "web"
	case strings.Contains(v, "amd") || strings.Contains(v, "radeon"):
		app := filepath.Join(pf, "AMD", "CNext", "CNext", "RadeonSoftware.exe")
		if fileExists(app) && openPath(app, "") {
			return "ok"
		}
		openURL("https://www.amd.com/it/support/download/drivers.html")
		return "web"
	case strings.Contains(v, "intel"):
		openURL("https://www.intel.it/content/www/it/it/support/detect.html")
		return "web"
	}
	return "none"
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// openWinPage apre una pagina delle Impostazioni di Windows (solo quelle di questo elenco).
func openWinPage(id string) string {
	pages := map[string]string{
		"firewall":    "windowsdefender://network",
		"security":    "windowsdefender:",
		"gamebar":     "ms-settings:gaming-gamebar",
		"graphics":    "ms-settings:display-advancedgraphics",
		"startupapps": "ms-settings:startupapps",
		"display":     "ms-settings:display",
	}
	u, ok := pages[id]
	if !ok {
		return "no"
	}
	openURL(u)
	return "ok"
}
