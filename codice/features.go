package main

// Funzioni di base usate da molte pagine: avvio degli script PowerShell, programmi che partono
// con Windows, programmi installati e lavori lunghi in sottofondo.

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// runPSArgs passa dati allo script tramite un file JSON (così nomi e percorsi strani non rompono nulla).
// Nello script i dati sono disponibili nella variabile $A.
func runPSArgs(script string, args interface{}, timeout time.Duration, onMsg func(string)) psResult {
	b, err := json.Marshal(args)
	if err != nil {
		return psResult{Err: err}
	}
	// I dati vanno dentro lo script stesso (una riga JSON in una here-string), niente file a parte
	// che un altro programma potrebbe cambiare prima che lo script parta.
	head := "$A = @'\n" + string(b) + "\n'@ | ConvertFrom-Json\n"
	return runPS(head+script, timeout, onMsg)
}

func decodeJSON(s string) interface{} {
	if s == "" {
		return nil
	}
	var v interface{}
	if json.Unmarshal([]byte(s), &v) != nil {
		return nil
	}
	return v
}

var busyMu sync.Mutex
var busy = map[string]bool{}

// updating: mentre Tasso si aggiorna non parte nient'altro (verrebbe chiuso a metà).
var updating atomic.Bool

// otherBusy dice se c'è un lavoro in corso diverso da "except".
func otherBusy(except string) bool {
	busyMu.Lock()
	defer busyMu.Unlock()
	for k, v := range busy {
		if v && k != except {
			return true
		}
	}
	return false
}

// async esegue f in background una sola volta per tipo; restituisce "busy" se è già in corso.
func async(kind string, f func()) string {
	if updating.Load() && kind != "update" {
		return "busy"
	}
	busyMu.Lock()
	if busy[kind] {
		busyMu.Unlock()
		return "busy"
	}
	busy[kind] = true
	busyMu.Unlock()
	go func() {
		defer guard("features.go:75")
		defer func() {
			busyMu.Lock()
			busy[kind] = false
			busyMu.Unlock()
		}()
		defer func() {
			if r := recover(); r != nil {
				logPanic("lavoro "+kind, r, debug.Stack()) // in crash.txt, per capire cosa è successo
				emit("onBgError", map[string]interface{}{"kind": kind, "msg": "Qualcosa è andato storto. Riprova; se succede ancora, riavvia Tasso."})
			}
		}()
		f()
	}()
	return "ok"
}

// ---------------- Programmi all'avvio ----------------

func startupList() {
	r := runPS(startupListScript, 2*time.Minute, nil)
	list, _ := decodeJSON(r.JSON).([]interface{})
	if list == nil {
		// Lettura non riuscita: non cancello l'elenco dei programmi già visti, se no la volta dopo
		// conterebbe come "prima volta" e i programmi nuovi all'avvio non verrebbero segnalati.
		emit("onStartup", map[string]interface{}{"items": []interface{}{}, "fresh": []string{},
			"error": "Non sono riuscito a leggere i programmi all'avvio: riprova tra poco."})
		return
	}
	setMu.Lock()
	known := map[string]bool{}
	for _, k := range settings.KnownStartup {
		known[k] = true
	}
	// Primo giro, o primo giro con l'elenco della 2.5 (ora ci sono anche le app dello Store):
	// imparo l'elenco senza segnare tutto come "nuovo".
	first := len(settings.KnownStartup) == 0 || settings.StartupV < 2
	setMu.Unlock()
	var names []string
	var fresh []string
	for _, it := range list {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		key := m["key"]
		if key == nil {
			key = m["name"]
		}
		id := fmt.Sprint(m["scope"]) + "|" + fmt.Sprint(key)
		names = append(names, id)
		if !first && !known[id] {
			m["new"] = true
			fresh = append(fresh, fmt.Sprint(m["name"]))
		}
	}
	sort.Strings(names)
	updateSettings(func(s *Settings) { s.KnownStartup, s.StartupV = names, 2 })
	if list == nil {
		list = []interface{}{}
	}
	emit("onStartup", map[string]interface{}{"items": list, "fresh": fresh})
}

// appxStartupKey: "famiglia del pacchetto\TaskId" di un'app dello Store, niente percorsi strani.
var appxStartupKey = regexp.MustCompile(`^[A-Za-z0-9._-]{3,120}\\[A-Za-z0-9._-]{1,120}$`)

func startupSet(scope, name string, enable bool) {
	switch scope {
	case "hkcu", "hklm", "hklm32", "fu", "fc":
	case "appx":
		if !appxStartupKey.MatchString(name) {
			return
		}
	default:
		return
	}
	// Com'era lo salvo prima di scrivere (se Tasso si chiude a metà, si può rimettere), ma se Windows
	// non lascia cambiare la voce lo tolgo di nuovo: in Torna indietro non resta un cambio mai fatto.
	id := scope + "|" + name
	setMu.Lock()
	_, giaSalvato := settings.StartupOrig[id]
	setMu.Unlock()
	rememberStartup(scope, name, enable)
	var msgs []string
	r := runPSArgs(startupSetScript, map[string]interface{}{"scope": scope, "name": name, "enable": enable}, time.Minute, func(m string) { msgs = append(msgs, m) })
	ok := startupRimessoOK(r)
	updateSettings(func(s *Settings) {
		v, c := s.StartupOrig[id]
		if !c {
			return
		}
		// non riuscito (e la voce l'avevo appena aggiunta), oppure tornato com'era prima di Tasso
		if (!ok && !giaSalvato) || (ok && v == enable) {
			delete(s.StartupOrig, id)
		}
	})
	msg := ""
	if !ok {
		msg = "Windows non mi lascia cambiare questa voce."
		if len(msgs) > 0 && msgs[len(msgs)-1] != "Fatto." {
			msg = msgs[len(msgs)-1]
		}
	}
	emit("onStartupSet", map[string]interface{}{"key": id, "ok": ok, "enable": enable, "msg": msg})
	startupList()
}

// ---------------- Disinstallazione ----------------

func programsList() {
	r := runPS(programsListScript, 3*time.Minute, nil)
	list, ok := decodeJSON(r.JSON).([]interface{})
	if !ok {
		// Lettura non riuscita: lo dico, invece di mostrare "Nessun programma trovato".
		emit("onPrograms", map[string]interface{}{"error": "Non sono riuscito a leggere i programmi installati: riprova tra poco."})
		return
	}
	emit("onPrograms", list)
}

func uninstallProgram(key string) {
	if !validUninstallKey(key) {
		emit("onUninstallDone", map[string]interface{}{"ok": false, "msgs": []string{"Voce non valida."}})
		return
	}
	var msgs []string
	r := runPSArgs(uninstallScript, map[string]interface{}{"key": key}, 60*time.Minute, func(m string) { msgs = append(msgs, m) })
	emit("onUninstallDone", map[string]interface{}{"ok": true, "msgs": msgs, "result": decodeJSON(r.JSON)})
}

// uninstallKeyRe: solo una voce diretta di "Programmi installati" (HKLM o HKCU, anche WOW6432Node).
var uninstallKeyRe = regexp.MustCompile(`(?i)^HKEY_(LOCAL_MACHINE|CURRENT_USER)\\SOFTWARE\\(WOW6432Node\\)?Microsoft\\Windows\\CurrentVersion\\Uninstall\\[^\\/\x00-\x1f]+$`)

// validUninstallKey: la voce arriva dalla finestra, quindi accetto solo una voce diretta di
// "Programmi installati" (niente "." o ".." per risalire ad altre chiavi).
func validUninstallKey(key string) bool {
	return uninstallKeyRe.MatchString(key) && !strings.HasSuffix(key, `\.`) && !strings.HasSuffix(key, `\..`)
}

// removeOrphanProgram (3.1): toglie dall'elenco un programma che risulta già disinstallato.
func removeOrphanProgram(key string) {
	if !validUninstallKey(key) {
		emit("onOrphanDone", map[string]interface{}{"ok": false, "msgs": []string{"Voce non valida."}})
		return
	}
	var msgs []string
	bdir := filepath.Join(appDir, "backup")
	r := runPSArgs(removeOrphanScript, map[string]interface{}{"key": key, "dir": bdir}, 2*time.Minute, func(m string) { msgs = append(msgs, m) })
	ok := false
	if m, _ := decodeJSON(r.JSON).(map[string]interface{}); m != nil {
		ok, _ = m["ok"].(bool)
	}
	if len(msgs) == 0 {
		msgs = []string{"Non sono riuscito a toglierlo dall'elenco."}
	}
	emit("onOrphanDone", map[string]interface{}{"ok": ok, "msgs": msgs})
}

var leftoverBlock = map[string]bool{
	"microsoft": true, "common files": true, "windowsapps": true, "windows defender": true, "windows nt": true,
	"internet explorer": true, "microsoft.net": true, "packages": true, "temp": true, "programs": true,
	"windows": true, "windows mail": true, "windows media player": true, "windows photo viewer": true,
	"windowspowershell": true, "modifiablewindowsapps": true, "package cache": true, "nvidia corporation": true,
	"amd": true, "intel": true, "realtek": true, "google": true, "mozilla": true,
}

// safeLeftover accetta solo cartelle dentro Programmi, AppData o ProgramData, mai le cartelle principali o di sistema.
func safeLeftover(p string) bool {
	p = filepath.Clean(p)
	if !filepath.IsAbs(p) || strings.Contains(p, "..") {
		return false
	}
	// Le cartelle vere chieste a Windows, non le variabili d'ambiente (cambiabili senza permessi).
	roots := []string{programFilesDir(), cartellaNota(windows.FOLDERID_ProgramFilesX86), cartellaNota(windows.FOLDERID_LocalAppData), cartellaNota(windows.FOLDERID_RoamingAppData), cartellaNota(windows.FOLDERID_ProgramData)}
	lp := strings.ToLower(p)
	for _, r := range roots {
		if r == "" {
			continue
		}
		lr := strings.ToLower(filepath.Clean(r)) + "\\"
		if strings.HasPrefix(lp, lr) {
			rest := strings.TrimPrefix(lp, lr)
			parts := strings.Split(rest, "\\")
			if rest == "" {
				return false
			}
			// %LOCALAPPDATA%\Programs\<programma> è ammesso, la cartella Programs da sola no
			if leftoverBlock[parts[0]] && !(parts[0] == "programs" && len(parts) >= 2 && parts[1] != "") {
				return false
			}
			st, err := os.Stat(p)
			return err == nil && st.IsDir()
		}
	}
	return false
}

// safeRegKey accetta solo chiavi sotto Software (utente o macchina), mai chiavi di sistema o condivise.
func safeRegKey(k string) bool {
	u := strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(k, "/", "\\")))
	prefixes := []string{`HKEY_CURRENT_USER\SOFTWARE\`, `HKEY_LOCAL_MACHINE\SOFTWARE\WOW6432NODE\`, `HKEY_LOCAL_MACHINE\SOFTWARE\`}
	rest := ""
	ok := false
	for _, p := range prefixes {
		if strings.HasPrefix(u, p) {
			rest = strings.TrimPrefix(u, p)
			ok = true
			break
		}
	}
	if !ok || rest == "" || strings.Contains(rest, "..") {
		return false
	}
	first := strings.Split(rest, "\\")[0]
	block := map[string]bool{"MICROSOFT": true, "WINDOWS": true, "CLASSES": true, "CLIENTS": true, "POLICIES": true,
		"WOW6432NODE": true, "REGISTEREDAPPLICATIONS": true, "INTEL": true, "NVIDIA": true, "AMD": true,
		"REALTEK": true, "GOOGLE": true, "KHRONOS": true, "ODBC": true, "RPC": true, "PARTNER": true, "MOZILLA": true, "VALVE": true}
	return !block[first]
}

// removeLeftovers sposta nel Cestino le cartelle rimaste (solo quelle sicure) e rimuove dal registro
// le chiavi rimaste, facendo prima un backup .reg di ognuna.
func removeLeftovers(paths, keys []string) {
	var msgs []string
	var okPaths []string
	for _, p := range paths {
		if safeLeftover(p) {
			okPaths = append(okPaths, p)
		}
	}
	if len(okPaths) > 0 {
		runPSArgs(recycleScript, okPaths, 20*time.Minute, func(m string) { msgs = append(msgs, m) })
	}
	var okKeys []string
	for _, k := range keys {
		if safeRegKey(k) {
			okKeys = append(okKeys, k)
		}
	}
	if len(okKeys) > 0 {
		bdir := filepath.Join(appDir, "backup")
		_ = creaCartellaApp(bdir)
		runPSArgs(regRemoveScript, map[string]interface{}{"keys": okKeys, "dir": bdir}, 10*time.Minute, func(m string) { msgs = append(msgs, m) })
	}
	if len(okPaths) == 0 && len(okKeys) == 0 {
		msgs = append(msgs, "Niente da rimuovere.")
	}
	emit("onLeftoversDone", map[string]interface{}{"msgs": msgs})
}

// ---------------- App preinstallate ----------------

type bloat struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Junk bool   `json:"junk"`
}

var bloatList = []bloat{
	{"*CandyCrush*", "Candy Crush", true},
	{"*BubbleWitch*", "Bubble Witch", true},
	{"*TikTok*", "TikTok", true},
	{"*Disney*", "Disney+", false},
	{"*Facebook*", "Facebook", false},
	{"*Instagram*", "Instagram", false},
	{"AmazonVideo.PrimeVideo", "Prime Video", false},
	{"SpotifyAB.SpotifyMusic", "Spotify (preinstallato)", false},
	{"Microsoft.MicrosoftSolitaireCollection", "Solitaire Collection", true},
	{"Microsoft.BingNews", "Microsoft Notizie", true},
	{"Microsoft.BingWeather", "Meteo", false},
	{"Microsoft.GetHelp", "Assistenza", true},
	{"Microsoft.Getstarted", "Suggerimenti", true},
	{"Microsoft.WindowsFeedbackHub", "Hub di Feedback", true},
	{"Microsoft.MixedReality.Portal", "Portale Realtà mista", true},
	{"Microsoft.People", "Contatti", true},
	{"Microsoft.ZuneVideo", "Film e TV", false},
	{"Microsoft.MicrosoftOfficeHub", "App Microsoft 365", false}, // in Ufficio può servire: niente "pubblicità" nel nome
	{"Microsoft.549981C3F5F10", "Cortana", true},
	{"Clipchamp.Clipchamp", "Clipchamp", false},
	{"Microsoft.PowerAutomateDesktop", "Power Automate", false},
	{"MicrosoftTeams", "Teams (personale)", false},
	{"Microsoft.SkypeApp", "Skype", false},
	{"Microsoft.Todos", "To Do", false},
	{"Microsoft.WindowsMaps", "Mappe", false},
	{"Microsoft.MicrosoftStickyNotes", "Sticky Notes", false}, // le note solo sul PC si copiano prima in backup (bloatRemoveScript)
}

func bloatScan() {
	r := runPSArgs(bloatListScript, bloatList, 3*time.Minute, nil)
	list, _ := decodeJSON(r.JSON).([]interface{})
	if list == nil {
		list = []interface{}{}
	}
	emit("onBloat", list)
	// I loghi delle app arrivano dopo la lista (icone_programmi.go).
	iconeAppPreinstallate(list)
}

func bloatRemove(ids []string) {
	ok := map[string]bool{}
	for _, b := range bloatList {
		ok[b.ID] = true
	}
	var sel []string
	nomi := map[string]string{} // id -> nome mostrato nell'elenco, già nella lingua di Tasso
	for _, id := range ids {
		if ok[id] {
			sel = append(sel, id)
		}
	}
	for _, b := range bloatList {
		nomi[b.ID] = trGo(b.Name)
	}
	var msgs []string
	if len(sel) > 0 {
		// La cartella backup di Tasso: lì va la copia dei dati delle app che li tengono solo sul PC (Sticky Notes).
		bdir := filepath.Join(appDir, "backup")
		_ = creaCartellaApp(bdir)
		runPSArgs(bloatRemoveScript, map[string]interface{}{"ids": sel, "dir": bdir, "names": nomi}, 10*time.Minute, func(m string) { msgs = append(msgs, m) })
	}
	emit("onBloatDone", map[string]interface{}{"msgs": msgs})
	bloatScan()
}

// ---------------- File doppi, grandi, cartelle vuote, scorciatoie rotte ----------------

var (
	filesExtraMu sync.Mutex
	filesExtra   string // cartella o disco in più scelto dall'utente per l'ultima analisi
	pickedExtra  string // l'ultima cartella scelta con Sfoglia: solo questa si può analizzare
)

var noTouchRe = regexp.MustCompile(`(?i)\\(windows|program files( \(x86\))?|programdata|\$recycle\.bin|system volume information|windowsapps|steamapps|steamlibrary|epic games|riot games|xboxgames|battle\.net|ubisoft game launcher|ea games|gog galaxy|appdata|windows\.old|\$windows\.~bt|\$windows\.~ws|\$winreagent|recovery|perflogs)(\\|$)`)

// noTouchDir: cartelle di Windows, dei programmi e dei giochi. Lì i "doppioni" servono davvero.
func noTouchDir(p string) bool { return noTouchRe.MatchString(filepath.Clean(p) + `\`) }

// pickScanFolder: finestra di Windows per scegliere una cartella o un disco in più.
func pickScanFolder() {
	var hwnd uintptr
	if wv != nil {
		hwnd = uintptr(wv.Window())
	}
	// La finestra di scelta si apre sopra Tasso (non dietro).
	r := runPSArgs(pickFolderScript, map[string]interface{}{"hwnd": uint64(hwnd)}, 10*time.Minute, nil)
	m, _ := decodeJSON(r.JSON).(map[string]interface{})
	p, _ := m["path"].(string)
	if p != "" {
		if st, err := os.Stat(p); err != nil || !st.IsDir() || noTouchDir(p) {
			msg := "Questa cartella è di Windows, dei programmi o dei giochi: lì Tasso non cerca doppioni, perché servono."
			if officeMode() { // in Ufficio niente giochi nei testi
				msg = "Questa cartella è di Windows o dei programmi: lì Tasso non cerca doppioni, perché servono."
			}
			emit("onScanFolder", map[string]interface{}{"error": msg})
			return
		}
	}
	if p != "" {
		filesExtraMu.Lock()
		pickedExtra = filepath.Clean(p)
		filesExtraMu.Unlock()
	}
	emit("onScanFolder", map[string]interface{}{"path": p})
}

func filesScan(extra string, minMB int) {
	if extra != "" {
		filesExtraMu.Lock()
		ok := strings.EqualFold(filepath.Clean(extra), pickedExtra)
		filesExtraMu.Unlock()
		if st, err := os.Stat(extra); !ok || err != nil || !st.IsDir() || noTouchDir(extra) {
			extra = ""
		}
	}
	if minMB != 100 && minMB != 300 && minMB != 1000 {
		minMB = 300
	}
	filesExtraMu.Lock()
	filesExtra = filepath.Clean(extra)
	if extra == "" {
		filesExtra = ""
	}
	filesExtraMu.Unlock()
	var msgs []string
	r := runPSArgs(filesScanScript, map[string]interface{}{"extra": extra, "min": int64(minMB) << 20}, 45*time.Minute, func(m string) { msgs = append(msgs, m); emit("onFilesProgress", m) })
	emit("onFiles", map[string]interface{}{"data": decodeJSON(r.JSON), "msgs": msgs})
}

// inScanExtra: il file sta nella cartella in più analizzata (e non in una cartella da non toccare).
func inScanExtra(p string) bool {
	filesExtraMu.Lock()
	root := filesExtra
	filesExtraMu.Unlock()
	if root == "" || noTouchDir(p) {
		return false
	}
	lp, lr := strings.ToLower(filepath.Clean(p)), strings.ToLower(root)
	if !strings.HasSuffix(lr, `\`) {
		lr += `\`
	}
	return strings.HasPrefix(lp, lr) && len(lp) > len(lr)
}

// safeUserPath accetta solo percorsi dentro la cartella utente, il Desktop pubblico o il menu Start.
func safeUserPath(p string) bool {
	p = filepath.Clean(p)
	if !filepath.IsAbs(p) || strings.Contains(p, "..") {
		return false
	}
	lp := strings.ToLower(p)
	// Le cartelle vere chieste a Windows, non le variabili d'ambiente (cambiabili senza permessi).
	roots := []string{cartellaNota(windows.FOLDERID_Profile)}
	if pub := cartellaNota(windows.FOLDERID_Public); pub != "" {
		roots = append(roots, filepath.Join(pub, "Desktop"))
	}
	if pd := cartellaNota(windows.FOLDERID_ProgramData); pd != "" {
		roots = append(roots, filepath.Join(pd, "Microsoft", "Windows", "Start Menu"))
	}
	blocked := []string{"appdata", "desktop", "documents", "pictures", "videos", "music", "downloads", "onedrive"}
	for _, r := range roots {
		if r == "" {
			continue
		}
		lr := strings.ToLower(filepath.Clean(r)) + "\\"
		if strings.HasPrefix(lp, lr) {
			rest := strings.TrimPrefix(lp, lr)
			if rest == "" {
				return false
			}
			parts := strings.Split(rest, "\\")
			if len(parts) == 1 {
				for _, b := range blocked {
					if parts[0] == b {
						return false
					}
				}
			}
			if parts[0] == "appdata" {
				return false
			}
			return true
		}
	}
	return false
}

func recyclePaths(paths []string, kind string) {
	var ok []string
	for _, p := range paths {
		if (kind == "leftover" && safeLeftover(p)) || (kind != "leftover" && (safeUserPath(p) || (kind == "files" && inScanExtra(p)))) {
			ok = append(ok, filepath.Clean(p))
		}
	}
	var msgs []string
	var res interface{}
	if len(ok) > 0 {
		r := runPSArgs(recycleScript, ok, 30*time.Minute, func(m string) { msgs = append(msgs, m) })
		res = decodeJSON(r.JSON)
	} else {
		msgs = append(msgs, "Niente da spostare.")
	}
	emit("onRecycleDone", map[string]interface{}{"kind": kind, "msgs": msgs, "result": res, "skipped": len(paths) - len(ok)})
}

// ---------------- Distruggi file ----------------

type openFileName struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	flagsEx           uint32
}

var (
	comdlg32     = windows.NewLazySystemDLL("comdlg32.dll")
	pGetOpenFile = comdlg32.NewProc("GetOpenFileNameW")
)

// pickFiles: finestra di Windows per scegliere uno o più file da distruggere.
func pickFiles() []string {
	buf := make([]uint16, 65536)
	// Il filtro contiene degli zeri di separazione: StringToUTF16 andrebbe in panic.
	filter := utf16.Encode([]rune(trGo("Tutti i file") + "\x00*.*\x00\x00"))
	title, _ := windows.UTF16PtrFromString(trGo("Scegli i file da distruggere (puoi sceglierne più di uno)"))
	var hwnd uintptr
	if wv != nil {
		hwnd = uintptr(wv.Window())
	}
	ofn := openFileName{
		hwndOwner:   hwnd,
		lpstrFilter: &filter[0],
		lpstrFile:   &buf[0],
		nMaxFile:    uint32(len(buf)),
		lpstrTitle:  title,
		flags:       0x00001000 | 0x00000800 | 0x00000008 | 0x00080000 | 0x00000200 | 0x00100000, // FILEMUSTEXIST|PATHMUSTEXIST|NOCHANGEDIR|EXPLORER|ALLOWMULTISELECT|NODEREFERENCELINKS (un collegamento .lnk distrugge il collegamento, non il file a cui punta)
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	r, _, _ := pGetOpenFile.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return nil
	}
	// Un file: "C:\\cartella\\file". Più file: "C:\\cartella\x00file1\x00file2\x00\x00".
	var parts []string
	start := 0
	for i, c := range buf {
		if c == 0 {
			if i == start {
				break
			}
			parts = append(parts, windows.UTF16ToString(buf[start:i]))
			start = i + 1
		}
	}
	var out []string
	if len(parts) == 1 {
		out = parts
	} else if len(parts) > 1 {
		for _, f := range parts[1:] {
			out = append(out, filepath.Join(parts[0], f))
		}
	}
	pickMu.Lock()
	lastPicked = map[string]bool{}
	for _, p := range out {
		lastPicked[p] = true
	}
	pickMu.Unlock()
	return out
}

// lastPicked: solo i file scelti con la finestra "Scegli" si possono distruggere.
var (
	pickMu     sync.Mutex
	lastPicked map[string]bool
)

// shredMotivo: perché un file scelto non si può distruggere ("" = si può). Il motivo vero, da dire
// all'utente (prima tutti i rifiuti finivano in "sono di sistema, aperti in un altro programma o spariti").
func shredMotivo(p string) string {
	pickMu.Lock()
	ok := lastPicked[p]
	pickMu.Unlock()
	if !ok {
		return "non l'hai scelto tu dalla finestra"
	}
	lp := strings.ToLower(p)
	if strings.HasPrefix(lp, `\\?\`) || strings.HasPrefix(lp, `\\.\`) {
		return "percorso speciale di Windows"
	}
	// Nomi corti tipo PROGRA~1: li accetto solo se il nome lungo è proprio quello (i file di
	// Office come "~$documento.docx" hanno la tilde nel nome vero).
	if strings.Contains(p, "~") && !nomeLungoUguale(p) {
		return "nome corto di Windows (scegli il file con il suo nome completo)"
	}
	if lfi, err := os.Lstat(p); err != nil {
		return "non c'è più"
	} else if lfi.Mode()&os.ModeSymlink != 0 || lfi.Mode()&os.ModeIrregular != 0 {
		return "è un collegamento: non distruggo il file a cui porta"
	}
	if real, err := filepath.EvalSymlinks(p); err == nil && !strings.EqualFold(real, p) {
		return "è un collegamento: non distruggo il file a cui porta"
	}
	cl := strings.ToLower(filepath.Clean(p))
	for _, r := range []string{winDir(), programFilesDir(), cartellaNota(windows.FOLDERID_ProgramFilesX86), cartellaNota(windows.FOLDERID_ProgramData)} {
		if r != "" && strings.HasPrefix(cl, strings.ToLower(filepath.Clean(r))+"\\") {
			return "è un file di Windows o di un programma"
		}
	}
	st, err := os.Stat(p)
	if err != nil {
		return "non c'è più"
	}
	if !st.Mode().IsRegular() {
		return "non è un file normale"
	}
	return ""
}

// nomeLungoUguale: il percorso con la tilde è già il nome lungo (non un nome corto 8.3).
func nomeLungoUguale(p string) bool {
	u, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return false
	}
	buf := make([]uint16, 1024)
	n, err := windows.GetLongPathName(u, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || int(n) >= len(buf) {
		return false
	}
	return strings.EqualFold(windows.UTF16ToString(buf[:n]), p)
}

// cartelleOneDrive: le cartelle di OneDrive dell'utente (personale e di lavoro). Lì una copia del
// file resta nella cronologia delle versioni e nel Cestino di OneDrive anche dopo la distruzione.
func cartelleOneDrive() []string {
	var out []string
	for _, v := range []string{"OneDrive", "OneDriveConsumer", "OneDriveCommercial"} {
		if d := strings.TrimSpace(os.Getenv(v)); d != "" {
			out = append(out, filepath.Clean(d))
		}
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\OneDrive\Accounts`, registry.ENUMERATE_SUB_KEYS); err == nil {
		nomi, _ := k.ReadSubKeyNames(50)
		k.Close()
		for _, n := range nomi {
			if a, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\OneDrive\Accounts\`+n, registry.QUERY_VALUE); err == nil {
				if d, _, err := a.GetStringValue("UserFolder"); err == nil && strings.TrimSpace(d) != "" {
					out = append(out, filepath.Clean(d))
				}
				a.Close()
			}
		}
	}
	return out
}

// shredFiles distrugge i file scelti; alla fine chiede agli SSD di cancellare davvero i blocchi liberi (TRIM).
func shredFiles(paths []string) {
	ok := 0
	var rifiutati []string
	drives := map[string]bool{}
	od := cartelleOneDrive()
	inOneDrive := 0
	for i, p := range paths {
		// numeri a parte: la pagina decide quando avvisare senza leggere il testo (che è tradotto)
		emit("onShredProgress", map[string]interface{}{"i": i + 1, "n": len(paths), "msg": fmt.Sprintf(trGo("Distruggo %d di %d: %s"), i+1, len(paths), filepath.Base(p))})
		if motivo := shredFile(p); motivo == "" {
			ok++
			drives[strings.ToUpper(filepath.VolumeName(p))] = true
			for _, d := range od {
				if dentroDi(filepath.Clean(p), d) {
					inOneDrive++
					break
				}
			}
		} else {
			rifiutati = append(rifiutati, filepath.Base(p)+" ("+trGo(motivo)+")")
		}
	}
	pickMu.Lock()
	lastPicked = nil
	pickMu.Unlock()
	if ok > 0 {
		var letters []string
		for d := range drives {
			if len(d) == 2 && d[1] == ':' {
				letters = append(letters, d[:1])
			}
		}
		emit("onShredProgress", map[string]interface{}{"trim": true, "msg": "Chiedo al disco di cancellare davvero lo spazio liberato (TRIM)…"})
		runPSArgs(retrimScript, letters, 10*time.Minute, nil)
	}
	msg := fmt.Sprintf(trGo("Distrutti %d file: il loro contenuto sul disco è stato sovrascritto."), ok)
	if ok == 1 {
		msg = trGo("Distrutto 1 file: il suo contenuto sul disco è stato sovrascritto.")
	}
	if ok > 0 {
		msg += trGo(" Possono restarne copie altrove: nei punti di ripristino e nella Cronologia file di Windows, e sugli SSD in celle che Windows non lascia sovrascrivere (il TRIM aiuta, ma non è una garanzia).")
	}
	if inOneDrive > 0 {
		msg += fmt.Sprintf(trGo(" %d erano in OneDrive: lì restano nella cronologia delle versioni e nel Cestino di OneDrive. Toglili anche da onedrive.com (Cestino e versioni precedenti)."), inOneDrive)
	}
	if len(rifiutati) > 0 {
		msg += fmt.Sprintf(trGo(" %d non distrutti: %s."), len(rifiutati), strings.Join(rifiutati, ", "))
	}
	emit("onShredDone", map[string]interface{}{"ok": ok > 0 && len(rifiutati) == 0, "msg": msg})
}

const retrimScript = `
foreach ($l in @($A)) {
  $l = [string]$l
  if ($l -notmatch '^[A-Za-z]$') { continue }
  try {
    $dn = (Get-Partition -DriveLetter $l -ErrorAction Stop).DiskNumber
    $pd = Get-PhysicalDisk -ErrorAction Stop | Where-Object { [string]$_.DeviceId -eq [string]$dn } | Select-Object -First 1
    if ([string]$pd.MediaType -eq 'SSD') { Optimize-Volume -DriveLetter $l -ReTrim -ErrorAction SilentlyContinue | Out-Null }
  } catch {}
}
`

// shredFile sovrascrive il file con dati casuali, lo rinomina e lo cancella. Restituisce "" se è
// andato tutto bene, altrimenti il motivo (da dire all'utente).
func shredFile(p string) string {
	if m := shredMotivo(p); m != "" {
		return m
	}
	// "Sola lettura": la tolgo (il file l'hai scelto tu per distruggerlo).
	if st, err := os.Stat(p); err == nil && st.Mode().Perm()&0o200 == 0 {
		_ = os.Chmod(p, st.Mode().Perm()|0o200)
	}
	err := func() error {
		f, err := os.OpenFile(p, os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		st, err := f.Stat()
		if err != nil {
			f.Close()
			return err
		}
		size := st.Size()
		chunk := make([]byte, 1<<20)
		var done int64
		for done < size {
			n := int64(len(chunk))
			if size-done < n {
				n = size - done
			}
			if _, err := rand.Read(chunk[:n]); err != nil {
				f.Close()
				return err
			}
			if _, err := f.WriteAt(chunk[:n], done); err != nil {
				f.Close()
				return err
			}
			done += n
		}
		_ = f.Sync()
		f.Close()
		nb := make([]byte, 8)
		_, _ = rand.Read(nb)
		np := filepath.Join(filepath.Dir(p), fmt.Sprintf("%x.tmp", nb))
		if os.Rename(p, np) == nil {
			p = np
		}
		return os.Remove(p)
	}()
	if err == nil {
		return ""
	}
	if errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return "è aperto in un altro programma: chiudilo e riprova"
	}
	if os.IsPermission(err) {
		return "Windows non mi lascia scriverlo (permessi)"
	}
	return "non sono riuscito a sovrascriverlo"
}

func cancelShutdown() string {
	annullaFineTasso() // il conto alla rovescia di Tasso (quando shutdown.exe non era stato accettato)
	togliBentornato()  // niente riavvio: niente "Bentornato" al prossimo avvio
	_, _ = runHidden("shutdown.exe", "/a")
	return "ok"
}

// bgCall: come async, ma la risposta arriva a JS con onBg {id, r} (vedi callBg nell'interfaccia).
func bgCall(kind, id string, f func() string) string {
	return async(kind, func() {
		r := "Errore interno."
		defer func() { emit("onBg", map[string]interface{}{"id": id, "r": r}) }()
		r = f()
	})
}
