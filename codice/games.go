package main

// Giochi: libreria unica (Steam, Epic, Riot) e "Ottimizza prima di giocare".

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// ---------------- Libreria giochi ----------------

type gameEntry struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Store   string `json:"store"`
	Size    int64  `json:"size"`
	Img     string `json:"img,omitempty"`  // copertina: file locale (in base64) o indirizzo https
	ImgAlt  string `json:"img2,omitempty"` // seconda scelta se la prima non si carica
	launch  string // indirizzo steam:// o com.epicgames.launcher://
	exe     string // programma da avviare (Riot)
	exeArgs string
	dir     string // cartella dove è installato (serve a "Regola con Tasso")
}

var (
	libMu   sync.Mutex
	library = map[string]gameEntry{}
)

var riotPathRe = regexp.MustCompile(`product_install_full_path:\s*"?([^"\r\n]+)"?`)

var vdfKV = regexp.MustCompile(`"([^"]+)"\s+"((?:[^"\\]|\\.)*)"`)

func vdfValues(b []byte, key string) []string {
	var out []string
	for _, m := range vdfKV.FindAllSubmatch(b, -1) {
		if strings.EqualFold(string(m[1]), key) {
			out = append(out, strings.ReplaceAll(string(m[2]), `\\`, `\`))
		}
	}
	return out
}

func steamRoot() string {
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, path := range []string{`Software\Valve\Steam`, `SOFTWARE\WOW6432Node\Valve\Steam`} {
			k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			for _, name := range []string{"SteamPath", "InstallPath"} {
				if v, _, err := k.GetStringValue(name); err == nil && v != "" {
					k.Close()
					return filepath.Clean(strings.ReplaceAll(v, "/", `\`))
				}
			}
			k.Close()
		}
	}
	return ""
}

// Programmi di Steam che non sono giochi.
var steamNotGames = map[string]bool{"228980": true, "1070560": true, "1391110": true, "1628350": true, "431960": true, "250820": true, "1493710": true}

// coverBudget: al massimo ~12 MB di copertine dal disco, le altre arrivano da internet.
var coverBudget int

func scanSteam() []gameEntry {
	coverBudget = 12 << 20
	root := steamRoot()
	if root == "" {
		return nil
	}
	libs := []string{root}
	if b, err := os.ReadFile(filepath.Join(root, "steamapps", "libraryfolders.vdf")); err == nil {
		libs = append(libs, vdfValues(b, "path")...)
	}
	seen := map[string]bool{}
	var out []gameEntry
	for _, l := range libs {
		dir := filepath.Join(filepath.Clean(l), "steamapps")
		if seen[strings.ToLower(dir)] {
			continue
		}
		seen[strings.ToLower(dir)] = true
		files, _ := filepath.Glob(filepath.Join(dir, "appmanifest_*.acf"))
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			ids, names := vdfValues(b, "appid"), vdfValues(b, "name")
			gdir := ""
			if inst := vdfValues(b, "installdir"); len(inst) > 0 {
				gdir = filepath.Join(dir, "common", inst[0])
			}
			if len(ids) == 0 || len(names) == 0 || steamNotGames[ids[0]] {
				continue
			}
			n := names[0]
			ln := strings.ToLower(n)
			if strings.Contains(ln, "redistributable") || strings.Contains(ln, "steamvr") || strings.HasPrefix(ln, "proton") || strings.Contains(ln, "steam linux runtime") || strings.Contains(ln, "dedicated server") {
				continue
			}
			var size int64
			if s := vdfValues(b, "SizeOnDisk"); len(s) > 0 {
				fmt.Sscan(s[0], &size)
			}
			img := ""
			if coverBudget > 0 {
				img = steamCover(root, ids[0])
				coverBudget -= len(img)
			}
			cdn := "https://shared.cloudflare.steamstatic.com/store_item_assets/steam/apps/" + ids[0] + "/"
			if img == "" {
				img = cdn + "library_600x900.jpg"
			}
			out = append(out, gameEntry{ID: "steam:" + ids[0], Name: n, Store: "Steam", Size: size, Img: img, ImgAlt: cdn + "header.jpg", launch: "steam://rungameid/" + ids[0], dir: gdir})
		}
	}
	return out
}

func scanEpic() []gameEntry {
	dir := filepath.Join(os.Getenv("ProgramData"), "Epic", "EpicGamesLauncher", "Data", "Manifests")
	files, _ := filepath.Glob(filepath.Join(dir, "*.item"))
	var out []gameEntry
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var m struct {
			DisplayName, AppName, CatalogNamespace, CatalogItemId, InstallLocation string
			InstallSize                                                            int64
			AppCategories                                                          []string
		}
		if json.Unmarshal(b, &m) != nil || m.AppName == "" || m.DisplayName == "" {
			continue
		}
		isGame := len(m.AppCategories) == 0
		for _, c := range m.AppCategories {
			if c == "games" {
				isGame = true
			}
		}
		if !isGame || (m.InstallLocation != "" && !dirExists(m.InstallLocation)) {
			continue
		}
		id := url.PathEscape(m.CatalogNamespace + ":" + m.CatalogItemId + ":" + m.AppName)
		out = append(out, gameEntry{ID: "epic:" + m.AppName, Name: m.DisplayName, Store: "Epic", Size: m.InstallSize,
			launch: "com.epicgames.launcher://apps/" + id + "?action=launch&silent=true", dir: m.InstallLocation})
	}
	return out
}

func scanRiot() []gameEntry {
	pd := os.Getenv("ProgramData")
	var client string
	if b, err := os.ReadFile(filepath.Join(pd, "Riot Games", "RiotClientInstalls.json")); err == nil {
		var m map[string]interface{}
		if json.Unmarshal(b, &m) == nil {
			for _, k := range []string{"rc_default", "rc_live"} {
				if s, ok := m[k].(string); ok && fileExists(s) {
					client = s
					break
				}
			}
		}
	}
	if client == "" {
		return nil
	}
	products := []struct{ id, name string }{{"league_of_legends", "League of Legends"}, {"valorant", "VALORANT"}, {"bacon", "Legends of Runeterra"}, {"lion", "2XKO"}}
	var out []gameEntry
	for _, p := range products {
		meta := filepath.Join(pd, "Riot Games", "Metadata", p.id+".live", p.id+".live.product_settings.yaml")
		if !fileExists(meta) {
			continue
		}
		ico := fileDataURI(filepath.Join(pd, "Riot Games", "Metadata", p.id+".live", p.id+".live.ico"), "image/x-icon", 2<<20)
		gdir := ""
		if b, err := os.ReadFile(meta); err == nil {
			if m := riotPathRe.FindSubmatch(b); m != nil {
				gdir = filepath.Clean(strings.ReplaceAll(string(m[1]), "/", `\`))
			}
		}
		out = append(out, gameEntry{ID: "riot:" + p.id, Name: p.name, Store: "Riot", Img: ico, exe: client, exeArgs: "--launch-product=" + p.id + " --launch-patchline=live", dir: gdir})
	}
	return out
}

// steamCover cerca la copertina già scaricata da Steam sul PC (così si vede anche senza internet).
func steamCover(root, id string) string {
	lc := filepath.Join(root, "appcache", "librarycache")
	var cands []string
	for _, name := range []string{"library_600x900.jpg", "library_600x900_2x.jpg", "header.jpg"} {
		cands = append(cands, filepath.Join(lc, id+"_"+name), filepath.Join(lc, id, name))
		if m, _ := filepath.Glob(filepath.Join(lc, id, "*", name)); len(m) > 0 {
			cands = append(cands, m...)
		}
	}
	for _, c := range cands {
		if d := fileDataURI(c, "image/jpeg", 300<<10); d != "" {
			return d
		}
	}
	return ""
}

func fileDataURI(path, mime string, max int64) string {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() || st.Size() == 0 || st.Size() > max {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func scanLibrary() {
	all := append(append(scanSteam(), scanEpic()...), scanRiot()...)
	sort.Slice(all, func(i, j int) bool { return strings.ToLower(all[i].Name) < strings.ToLower(all[j].Name) })
	libMu.Lock()
	library = map[string]gameEntry{}
	for _, g := range all {
		library[g.ID] = g
	}
	libMu.Unlock()
	emit("onLibrary", all)
}

// launchGame avvia solo un gioco trovato dalla scansione (niente indirizzi arbitrari dalla pagina).
func launchGame(id string) string {
	libMu.Lock()
	g, ok := library[id]
	libMu.Unlock()
	if !ok {
		return "Gioco non trovato: aggiorna la libreria."
	}
	if g.exe != "" {
		if !openUser(g.exe, g.exeArgs) {
			return "Non riesco ad avviare " + g.Name + "."
		}
		return "ok"
	}
	if g.launch == "" {
		return "Non so come avviare " + g.Name + "."
	}
	openURL(g.launch)
	return "ok"
}

// ---------------- Ottimizza prima di giocare ----------------

type heavyProc struct {
	Pid     uint32 `json:"pid"`
	Name    string `json:"name"`
	MB      int64  `json:"mb"`
	Suggest bool   `json:"suggest"`
	Window  bool   `json:"window"`
	path    string
	Exe     string `json:"exe"` // per l'icona
}

// Mai da chiudere: Windows, driver, antivirus, Tasso stesso, audio e voce.
var neverClose = map[string]bool{
	"system": true, "registry": true, "smss.exe": true, "csrss.exe": true, "wininit.exe": true, "winlogon.exe": true, "services.exe": true,
	"lsass.exe": true, "svchost.exe": true, "dwm.exe": true, "explorer.exe": true, "fontdrvhost.exe": true, "sihost.exe": true,
	"taskhostw.exe": true, "ctfmon.exe": true, "searchhost.exe": true, "startmenuexperiencehost.exe": true, "shellexperiencehost.exe": true,
	"runtimebroker.exe": true, "audiodg.exe": true, "msmpeng.exe": true, "nissrv.exe": true, "securityhealthservice.exe": true,
	"memory compression": true, "secure system": true, "tasso.exe": true, "msedgewebview2.exe": true, "conhost.exe": true,
	"nvcontainer.exe": true, "nvdisplay.container.exe": true, "amdrsserv.exe": true, "radeonsoftware.exe": true, "igfxem.exe": true,
	"claude.exe": true, "discord.exe": true, "steam.exe": true, "steamwebhelper.exe": true, "epicgameslauncher.exe": true, "riotclientservices.exe": true,
	"vgc.exe": true, "vgtray.exe": true, "easyanticheat.exe": true, "beservice.exe": true, "textinputhost.exe": true, "lockapp.exe": true,
	"spoolsv.exe": true, "wudfhost.exe": true, "dashost.exe": true, "smartscreen.exe": true, "sgrmbroker.exe": true,
	// launcher e client dei giochi
	"battle.net.exe": true, "agent.exe": true, "eadesktop.exe": true, "eabackgroundservice.exe": true, "ubisoftconnect.exe": true,
	"upc.exe": true, "galaxyclient.exe": true, "gogalaxy.exe": true, "riotclientux.exe": true, "riotclientuxrender.exe": true,
	"leagueclient.exe": true, "leagueclientux.exe": true, "leagueclientuxrender.exe": true, "xboxapp.exe": true, "xboxpcapp.exe": true,
	"gamingservices.exe": true, "gamebar.exe": true, "gamebarftserver.exe": true,
	// antivirus
	"avastui.exe": true, "avgui.exe": true, "bdagent.exe": true, "egui.exe": true, "avpui.exe": true, "avp.exe": true, "mbam.exe": true,
	"mbamtray.exe": true, "mcuicnt.exe": true, "nortonui.exe": true, "ns.exe": true, "kaspersky.exe": true,
	// driver, periferiche, registrazione
	"nvidia app.exe": true, "nvidia overlay.exe": true, "nvidia share.exe": true, "nvcplui.exe": true, "lghub.exe": true,
	"lghub_agent.exe": true, "lghub_system_tray.exe": true, "icue.exe": true, "obs64.exe": true, "streamdeck.exe": true,
	"razer synapse 3.exe": true, "razercentralservice.exe": true, "msiafterburner.exe": true, "rtss.exe": true,
	// streaming e registrazione: mai chiusi, servono per trasmettere la partita
	"obs32.exe": true, "streamlabs obs.exe": true, "streamlabs desktop.exe": true, "xsplit.core.exe": true, "xsplit.broadcaster.exe": true,
	"twitchstudio.exe": true, "twitch studio.exe": true, "medal.exe": true, "outplayed.exe": true, "overwolf.exe": true,
	"streamer.bot.exe": true, "elgato 4k capture utility.exe": true, "camera hub.exe": true, "elgatostudio.exe": true,
	"amdrssrcext.exe": true, "radeonsoftwarecli.exe": true, "nvsphelper64.exe": true, "nvoawrappercache.exe": true,
	"vlc.exe": true, "spotify.exe": true, "discordptb.exe": true, "discordcanary.exe": true, "teamspeak.exe": true, "ts3client_win64.exe": true,
}

// Programmi in background che di solito si possono chiudere senza problemi mentre giochi.
var suggestClose = map[string]bool{
	"onedrive.exe": true, "teams.exe": true, "ms-teams.exe": true, "skype.exe": true, "acrobat.exe": true, "acrord32.exe": true, "adobe desktop service.exe": true,
	"creative cloud.exe": true, "dropbox.exe": true, "googledrivefs.exe": true, "icloudservices.exe": true, "zoom.exe": true,
	"outlook.exe": true, "winword.exe": true, "excel.exe": true, "powerpnt.exe": true, "phoneexperiencehost.exe": true, "widgets.exe": true,
}

// Solo questi (sincronizzazione e chat in background) si possono chiudere "di forza" se non rispondono:
// non hanno documenti aperti da perdere.
var forceOK = map[string]bool{
	"onedrive.exe": true, "teams.exe": true, "ms-teams.exe": true, "skype.exe": true, "dropbox.exe": true,
	"googledrivefs.exe": true, "icloudservices.exe": true, "zoom.exe": true, "phoneexperiencehost.exe": true, "widgets.exe": true,
}

var (
	pGetProcMem = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")
	boostMu     sync.Mutex
	boostClosed []string // programmi chiusi da riaprire a fine partita
)

type procMemCounters struct {
	cb, PageFaultCount                                 uint32
	PeakWorkingSetSize, WorkingSetSize                 uintptr
	QuotaPeakPagedPoolUsage, QuotaPagedPoolUsage       uintptr
	QuotaPeakNonPagedPoolUsage, QuotaNonPagedPoolUsage uintptr
	PagefileUsage, PeakPagefileUsage                   uintptr
}

func listHeavy() []heavyProc {
	withWin := pidsWithWindow(200, 150)
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	self := uint32(os.Getpid())
	byName := map[string]*heavyProc{}
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		name := windows.UTF16ToString(pe.ExeFile[:])
		ln := strings.ToLower(name)
		if pe.ProcessID == self || pe.ProcessID <= 4 || neverClose[ln] {
			continue
		}
		h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pe.ProcessID)
		if e != nil {
			continue
		}
		var pmc procMemCounters
		pmc.cb = uint32(unsafe.Sizeof(pmc))
		pGetProcMem.Call(uintptr(h), uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.cb))
		windows.CloseHandle(h)
		path := processPath(pe.ProcessID)
		lp := strings.ToLower(path)
		if path == "" || strings.HasPrefix(lp, strings.ToLower(os.Getenv("SystemRoot"))+`\`) {
			continue // programmi di Windows: non si toccano
		}
		mb := int64(pmc.WorkingSetSize / (1024 * 1024))
		hp := byName[ln]
		if hp == nil {
			hp = &heavyProc{Pid: pe.ProcessID, Name: strings.TrimSuffix(name, ".exe"), path: path, Exe: path, Suggest: suggestClose[ln]}
			byName[ln] = hp
		}
		hp.MB += mb
		if withWin[pe.ProcessID] {
			hp.Window = true
		}
	}
	var out []heavyProc
	for _, hp := range byName {
		if hp.MB >= 150 || (hp.Suggest && hp.MB >= 40) {
			out = append(out, *hp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MB > out[j].MB })
	if len(out) > 25 {
		out = out[:25]
	}
	boostMu.Lock()
	lastHeavy = map[string]string{}
	for _, h := range out {
		lastHeavy[strings.ToLower(h.Name)] = h.path
	}
	boostMu.Unlock()
	return out
}

var lastHeavy = map[string]string{}

func boostScan() {
	emit("onBoostList", listHeavy())
}

// boostStart chiude con gentilezza i programmi scelti (come la X della finestra, così ti chiedono
// di salvare se serve) e mette la modalità Gaming. Mai chiusure forzate di programmi con finestre.
func boostStart(names []string) {
	setMu.Lock()
	paused := settings.Paused
	setMu.Unlock()
	if paused {
		emit("onBoostDone", map[string]interface{}{"ok": false, "msgs": []string{"Tasso è spento: riaccendilo per ottimizzare."}, "active": false})
		return
	}
	var closed, msgs []string
	for _, n := range names {
		ln := strings.TrimSuffix(strings.ToLower(n), ".exe")
		boostMu.Lock()
		path, ok := lastHeavy[ln]
		boostMu.Unlock()
		if !ok || neverClose[ln+".exe"] {
			continue
		}
		if closeGently(ln + ".exe") {
			closed = append(closed, path)
			msgs = append(msgs, "Chiuso: "+n)
		} else {
			msgs = append(msgs, n+" non si è chiuso (forse ti chiede di salvare): lascialo o chiudilo tu.")
		}
	}
	setMu.Lock()
	prev := settings.Mode
	setMu.Unlock()
	boostMu.Lock()
	boostClosed = append(boostClosed, closed...)
	boostMu.Unlock()
	if prev != "gaming" && prev != "fps" {
		if _, ok := modeScripts[prev]; !ok {
			prev = "normale"
		}
		updateSettings(func(s *Settings) { s.ModeBeforeBoost = prev })
		setMode("gaming")
	}
	emit("onBoostDone", map[string]interface{}{"ok": true, "msgs": msgs, "active": true})
}

// boostEnd: fine partita, rimette la modalità di prima e riapre i programmi chiusi.
func boostEnd() {
	boostMu.Lock()
	closed := boostClosed
	boostClosed = nil
	boostMu.Unlock()
	setMu.Lock()
	back := settings.ModeBeforeBoost
	cur := settings.Mode
	setMu.Unlock()
	if back != "" {
		updateSettings(func(s *Settings) { s.ModeBeforeBoost = "" })
		if cur == "gaming" {
			setMode(back)
		}
	}
	var msgs []string
	for _, p := range closed {
		// Riaperto tramite Esplora file, così parte come un programma normale e non da amministratore.
		if p != "" && fileExists(p) && openPath(filepath.Join(os.Getenv("SystemRoot"), "explorer.exe"), "\""+p+"\"") {
			msgs = append(msgs, "Riaperto: "+strings.TrimSuffix(filepath.Base(p), ".exe"))
		}
	}
	emit("onBoostDone", map[string]interface{}{"ok": true, "msgs": msgs, "active": false})
}

var (
	pPostMessage = user32.NewProc("PostMessageW")
)

// closeGently manda WM_CLOSE alle finestre del programma; per quelli senza finestra (OneDrive,
// Teams in background) usa la chiusura normale di Windows. Aspetta fino a 8 secondi.
func closeGently(exe string) bool {
	pids := pidsByExe(exe)
	if len(pids) == 0 {
		return true
	}
	wins := windowsOfPids(pids)
	for _, h := range wins {
		pPostMessage.Call(h, 0x0010, 0, 0) // WM_CLOSE
	}
	if len(wins) == 0 {
		_, _ = runHidden("taskkill.exe", "/IM", exe) // senza /F: chiusura normale
	}
	for i := 0; i < 16; i++ {
		time.Sleep(500 * time.Millisecond)
		if len(pidsByExe(exe)) == 0 {
			return true
		}
	}
	// Senza finestre visibili (programmi solo in background): li chiudo davvero.
	if len(wins) == 0 && forceOK[exe] {
		_, _ = runHidden("taskkill.exe", "/IM", exe, "/F")
		time.Sleep(time.Second)
		return len(pidsByExe(exe)) == 0
	}
	return false
}

func pidsByExe(exe string) []uint32 {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var out []uint32
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if strings.EqualFold(windows.UTF16ToString(pe.ExeFile[:]), exe) {
			out = append(out, pe.ProcessID)
		}
	}
	return out
}

var (
	winsMu   sync.Mutex
	winsWant map[uint32]bool
	winsOut  []uintptr
	winsCb   = windows.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		if v, _, _ := pIsWindowVisible.Call(hwnd); v == 0 {
			return 1
		}
		var pid uint32
		pGetWindowPid.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if winsWant[pid] {
			winsOut = append(winsOut, hwnd)
		}
		return 1
	})
)

func windowsOfPids(pids []uint32) []uintptr {
	winsMu.Lock()
	defer winsMu.Unlock()
	winsWant = map[uint32]bool{}
	for _, p := range pids {
		winsWant[p] = true
	}
	winsOut = nil
	pEnumWindows.Call(winsCb, 0)
	return winsOut
}
