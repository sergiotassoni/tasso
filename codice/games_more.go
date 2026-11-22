package main

// 3.1: più giochi nella libreria. Oltre a Steam, Epic e Riot:
// GOG, Ubisoft Connect, EA, Battle.net, Rockstar, Amazon Games, Xbox (PC Game Pass),
// itch.io, i giochi installati a mano (anche quelli non ufficiali) e quelli aggiunti con "Aggiungi un gioco".
// Tasso li elenca e basta: non giudica da dove arrivano.

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// ---------- voci di "Programmi installati" ----------

type uninstEntry struct {
	key, name, publisher, loc, uninst, icon string
	size                                    int64
}

func readUninstall() []uninstEntry {
	var out []uninstEntry
	roots := []struct {
		root registry.Key
		pre  string
		path string
	}{
		{registry.LOCAL_MACHINE, `HKEY_LOCAL_MACHINE\`, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.LOCAL_MACHINE, `HKEY_LOCAL_MACHINE\`, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.CURRENT_USER, `HKEY_CURRENT_USER\`, `Software\Microsoft\Windows\CurrentVersion\Uninstall`},
	}
	for _, r := range roots {
		k, err := registry.OpenKey(r.root, r.path, registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			continue
		}
		names, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, n := range names {
			sk, err := registry.OpenKey(r.root, r.path+`\`+n, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			get := func(v string) string { s, _, _ := sk.GetStringValue(v); return strings.TrimSpace(s) }
			e := uninstEntry{key: r.pre + r.path + `\` + n, name: get("DisplayName"), publisher: get("Publisher"),
				loc: strings.Trim(get("InstallLocation"), `"`), uninst: get("UninstallString"), icon: get("DisplayIcon")}
			if v, _, err := sk.GetIntegerValue("EstimatedSize"); err == nil {
				e.size = int64(v) * 1024
			}
			sc, _, _ := sk.GetIntegerValue("SystemComponent")
			sk.Close()
			if e.name == "" || sc == 1 {
				continue
			}
			out = append(out, e)
		}
	}
	return out
}

// ---------- GOG ----------

func scanGOG(un []uninstEntry) []gameEntry {
	var out []gameEntry
	for _, p := range []string{`SOFTWARE\WOW6432Node\GOG.com\Games`, `SOFTWARE\GOG.com\Games`} {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, p, registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			continue
		}
		ids, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, id := range ids {
			sk, err := registry.OpenKey(registry.LOCAL_MACHINE, p+`\`+id, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			name, _, _ := sk.GetStringValue("gameName")
			dir, _, _ := sk.GetStringValue("path")
			exe, _, _ := sk.GetStringValue("exe")
			args, _, _ := sk.GetStringValue("launchParam")
			sk.Close()
			if name == "" || !dirExists(dir) {
				continue
			}
			if !fileExists(exe) {
				exe = dirExe(dir)
			}
			if exe == "" {
				continue
			}
			g := gameEntry{ID: "gog:" + id, Name: name, Store: "GOG", exe: exe, exeArgs: args, dir: dir, Icon: exe}
			if e := uninstFor(un, dir); e != nil {
				g.Size, g.uninstKey = e.size, e.key
			}
			out = append(out, g)
		}
	}
	return out
}

// ---------- Ubisoft Connect ----------

func scanUbisoft(un []uninstEntry) []gameEntry {
	const p = `SOFTWARE\WOW6432Node\Ubisoft\Launcher\Installs`
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, p, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil
	}
	ids, _ := k.ReadSubKeyNames(-1)
	k.Close()
	var out []gameEntry
	for _, id := range ids {
		if strings.Trim(id, "0123456789") != "" {
			continue
		}
		sk, err := registry.OpenKey(registry.LOCAL_MACHINE, p+`\`+id, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		dir, _, _ := sk.GetStringValue("InstallDir")
		sk.Close()
		dir = filepath.Clean(strings.ReplaceAll(dir, "/", `\`))
		if !dirExists(dir) {
			continue
		}
		g := gameEntry{ID: "ubi:" + id, Name: filepath.Base(dir), Store: "Ubisoft", launch: "uplay://launch/" + id + "/0", dir: dir, Icon: "dir:" + dir}
		for i := range un {
			if strings.EqualFold(un[i].key[strings.LastIndex(un[i].key, `\`)+1:], "Uplay Install "+id) {
				g.Name, g.Size, g.uninstKey = un[i].name, un[i].size, un[i].key
				if un[i].icon != "" {
					g.Icon = un[i].icon
				}
			}
		}
		out = append(out, g)
	}
	return out
}

// ---------- Xbox / PC Game Pass (cartelle XboxGames) ----------

var (
	xbIdentRe = regexp.MustCompile(`<Identity[^>]*\sName="([^"]+)"`)
	xbExeRe   = regexp.MustCompile(`<Executable[^>]*\sName="([^"]+)"[^>]*\sId="([^"]+)"`)
	xbExeRe2  = regexp.MustCompile(`<Executable[^>]*\sId="([^"]+)"[^>]*\sName="([^"]+)"`)
	xbNameRe  = regexp.MustCompile(`DefaultDisplayName="([^"]+)"`)
	xbLogoRe  = regexp.MustCompile(`Square150x150Logo="([^"]+)"`)
)

func xboxFamilies() map[string]string {
	out := map[string]string{}
	const p = `Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\Repository\Packages`
	k, err := registry.OpenKey(registry.CURRENT_USER, p, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return out
	}
	names, _ := k.ReadSubKeyNames(-1)
	k.Close()
	for _, full := range names {
		parts := strings.Split(full, "_")
		if len(parts) == 5 {
			out[strings.ToLower(parts[0])] = parts[0] + "_" + parts[4]
		}
	}
	return out
}

func scanXbox() []gameEntry {
	var out []gameEntry
	var fam map[string]string
	for _, d := range fixedDrives() {
		dirs, _ := filepath.Glob(filepath.Join(d, "XboxGames", "*", "Content", "MicrosoftGame.config"))
		for _, cfg := range dirs {
			b, err := os.ReadFile(cfg)
			if err != nil || len(b) > 1<<20 {
				continue
			}
			content := filepath.Dir(cfg)
			gdir := filepath.Dir(content)
			name := filepath.Base(gdir)
			if m := xbNameRe.FindSubmatch(b); m != nil && !strings.HasPrefix(string(m[1]), "ms-resource") {
				name = string(m[1])
			}
			g := gameEntry{ID: "xbox:" + strings.ToLower(filepath.Base(gdir)), Name: name, Store: "Xbox", dir: content}
			var exeName, appID string
			if m := xbExeRe.FindSubmatch(b); m != nil {
				exeName, appID = string(m[1]), string(m[2])
			} else if m := xbExeRe2.FindSubmatch(b); m != nil {
				appID, exeName = string(m[1]), string(m[2])
			}
			if exeName != "" {
				g.Icon = filepath.Join(content, exeName)
			}
			if m := xbLogoRe.FindSubmatch(b); m != nil {
				logo := filepath.Join(content, filepath.FromSlash(string(m[1])))
				if !strings.Contains(string(m[1]), "..") {
					g.Img = fileDataURI(logo, "image/png", 400<<10)
				}
			}
			if m := xbIdentRe.FindSubmatch(b); m != nil && appID != "" {
				if fam == nil {
					fam = xboxFamilies()
				}
				if f := fam[strings.ToLower(string(m[1]))]; f != "" {
					g.launch = `shell:AppsFolder\` + f + "!" + appID
				}
			}
			if g.launch == "" && g.Icon != "" && fileExists(g.Icon) {
				g.exe = g.Icon
			}
			out = append(out, g)
		}
	}
	return out
}

// ---------- launcher che si vedono da "Programmi installati": EA, Battle.net, Rockstar, Amazon ----------

var (
	amazonIDRe = regexp.MustCompile(`(?i)-p\s+"?([A-Za-z0-9._-]+)`)
	// i launcher stessi e gli strumenti non sono giochi
	notGameNameRe = regexp.MustCompile(`(?i)^(steam|battle\.net|ea app|ea desktop|origin|ubisoft connect|uplay|gog galaxy|rockstar games (launcher|social club)|social club|amazon games|epic games launcher|riot client|xbox|itch|launcher|.*(redistributable|runtime|directx|visual c\+\+|anti-?cheat|easyanticheat|battleye|punkbuster|vulkan|physx|\.net|driver|benchmark|server|sdk|editor|tool(s|kit)?|mod manager|updater|uninstall)).*$`)
)

func storeFromUninstall(e uninstEntry) (store, launch string) {
	pub := strings.ToLower(e.publisher)
	low := strings.ToLower(e.uninst)
	switch {
	case strings.Contains(low, "amazon game remover"):
		if m := amazonIDRe.FindStringSubmatch(e.uninst); m != nil {
			return "Amazon", "amazon-games://play/" + m[1]
		}
		return "Amazon", ""
	case strings.Contains(pub, "blizzard"):
		return "Battle.net", ""
	case strings.Contains(pub, "electronic arts"):
		return "EA", ""
	case strings.Contains(pub, "rockstar games"):
		return "Rockstar", ""
	}
	return "", ""
}

// ---------- giochi installati a mano (anche non ufficiali): si riconoscono dai motori ----------

// gameMarkers: file o cartelle che hanno quasi solo i giochi.
var gameMarkers = []string{
	"UnityPlayer.dll", "UnityCrashHandler64.exe", "UnityCrashHandler32.exe", // Unity
	`Engine\Binaries`, `Engine\Content\Paks`, // Unreal
	"data.win", "options.ini.gmx", // GameMaker
	"renpy", `game\script_version.txt`, // Ren'Py
	`www\js\rpg_core.js`, `www\js\rmmz_core.js`, `js\rmmz_core.js`, "Game.rgss3a", "Game.rgssad", "RGSS301.dll", "RGSS104E.dll", // RPG Maker
	"steam_api.dll", "steam_api64.dll", "steam_appid.txt", "EOSSDK-Win64-Shipping.dll", "galaxy64.dll", "Galaxy.dll", // giochi con SDK dei negozi
	"fmod.dll", "fmodex.dll", "fmodstudio.dll", "bink2w64.dll", "binkw32.dll", "binkw64.dll", "d3dx9_43.dll", "xinput1_3.dll",
	"love.dll", "SDL2.dll", "openal32.dll", "MonoBleedingEdge", "nw.dll", // LÖVE, SDL, Mono, NW.js
}

func looksLikeGame(dir string) bool {
	for _, m := range gameMarkers {
		if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
			return true
		}
	}
	// Godot: un .pck accanto all'exe con lo stesso nome
	if pcks, _ := filepath.Glob(filepath.Join(dir, "*.pck")); len(pcks) > 0 {
		return true
	}
	// Unity: cartella "<nome>_Data"
	if d, _ := filepath.Glob(filepath.Join(dir, "*_Data", "globalgamemanagers")); len(d) > 0 {
		return true
	}
	return false
}

// gameDirExe: il programma del gioco (anche dentro Binaries\Win64 per Unreal).
func gameDirExe(dir string) string {
	if e := dirExe(dir); e != "" {
		return e
	}
	for _, sub := range []string{"bin", "Bin", "x64", `Binaries\Win64`, "Game", "Content"} {
		if e := dirExe(filepath.Join(dir, sub)); e != "" {
			return e
		}
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "*", "Binaries", "Win64")); len(m) > 0 {
		if e := dirExe(m[0]); e != "" {
			return e
		}
	}
	return ""
}

func fixedDrives() []string {
	var out []string
	for c := 'C'; c <= 'Z'; c++ {
		d := string(c) + `:\`
		if dirExists(d) {
			out = append(out, d)
		}
	}
	return out
}

// skipDirRe: cartelle di Program Files che non sono giochi (si risparmia tempo).
var skipDirRe = regexp.MustCompile(`(?i)^(common files|windows.*|microsoft.*|internet explorer|modifiablewindowsapps|windowsapps|reference assemblies|dotnet|nvidia.*|amd|intel|realtek|google|mozilla.*|7-zip|winrar|java|uninstall information|msbuild|iis.*|steam|epic games|riot games|gog galaxy|ea|ea games|origin|ubisoft|battle\.net|rockstar games|amazon games|xboxgames|windowspowershell|package cache|tasso)$`)

func scanFolders(known map[string]bool) []gameEntry {
	deadline := time.Now().Add(6 * time.Second)
	var roots []string
	home := os.Getenv("USERPROFILE")
	for _, d := range fixedDrives() {
		for _, n := range []string{"Games", "Giochi", "Game", "Gioco", "Program Files", "Program Files (x86)", "GOG Games", "EA Games", "Ubisoft Games"} {
			roots = append(roots, filepath.Join(d, n))
		}
	}
	itch := filepath.Join(os.Getenv("APPDATA"), "itch", "apps")
	roots = append(roots, filepath.Join(home, "Games"), filepath.Join(home, "Giochi"), filepath.Join(home, "Desktop"), filepath.Join(home, "Downloads"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs"), itch, filepath.Join(home, "AppData", "Local", "Games"))
	var out []gameEntry
	seen := map[string]bool{}
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if time.Now().After(deadline) {
			return
		}
		ents, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range ents {
			if !e.IsDir() || e.Type()&os.ModeSymlink != 0 || skipDirRe.MatchString(e.Name()) || strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "$") {
				continue
			}
			p := filepath.Join(dir, e.Name())
			lp := strings.ToLower(p)
			if seen[lp] || known[lp] || insideKnown(lp, known) {
				continue
			}
			seen[lp] = true
			if looksLikeGame(p) {
				if exe := gameDirExe(p); exe != "" {
					store := "Altro"
					if strings.HasPrefix(lp, strings.ToLower(itch)) {
						store = "itch.io"
					}
					out = append(out, gameEntry{ID: "dir:" + shortHash(lp), Name: niceName(e.Name()), Store: store, exe: exe, dir: p, Icon: exe})
					known[lp] = true
					continue
				}
			}
			if depth > 0 {
				walk(p, depth-1)
			}
		}
	}
	for _, r := range roots {
		if !dirExists(r) {
			continue
		}
		depth := 1
		if strings.Contains(strings.ToLower(r), `\program files`) || strings.HasSuffix(strings.ToLower(r), `\desktop`) || strings.HasSuffix(strings.ToLower(r), `\downloads`) {
			depth = 0
		}
		if strings.HasPrefix(strings.ToLower(r), strings.ToLower(itch)) {
			depth = 1
		}
		walk(r, depth)
	}
	return out
}

func insideKnown(lp string, known map[string]bool) bool {
	for k := range known {
		if strings.HasPrefix(lp, k+`\`) {
			return true
		}
	}
	return false
}

func shortHash(s string) string {
	h := sha1.Sum([]byte(strings.ToLower(s)))
	return hex.EncodeToString(h[:6])
}

// niceName: "the_witcher-3_v1.2" -> "The Witcher 3"
var verTailRe = regexp.MustCompile(`(?i)[\s._-]*(v|ver|build)?[\s._-]*\d+(\.\d+)+.*$`)

func niceName(n string) string {
	s := verTailRe.ReplaceAllString(n, "")
	if s == "" {
		s = n
	}
	s = strings.NewReplacer("_", " ", ".", " ").Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}

func uninstFor(un []uninstEntry, dir string) *uninstEntry {
	d := strings.ToLower(strings.TrimRight(filepath.Clean(dir), `\`))
	for i := range un {
		if un[i].loc != "" && strings.ToLower(strings.TrimRight(filepath.Clean(un[i].loc), `\`)) == d {
			return &un[i]
		}
	}
	return nil
}

// ---------- giochi aggiunti a mano ----------

type CustomGame struct {
	Name string `json:"name"`
	Exe  string `json:"exe"`
}

func customGames() []gameEntry {
	setMu.Lock()
	list := append([]CustomGame(nil), settings.CustomGames...)
	setMu.Unlock()
	var out []gameEntry
	for _, c := range list {
		if !fileExists(c.Exe) {
			continue
		}
		out = append(out, gameEntry{ID: "man:" + shortHash(c.Exe), Name: c.Name, Store: "Mano", exe: c.Exe, dir: filepath.Dir(c.Exe), Icon: c.Exe})
	}
	return out
}

// addGame: finestra di Windows per scegliere il programma del gioco.
func addGame() string {
	p := pickFile(trGo("Scegli il programma del gioco (.exe)"), "Programmi\x00*.exe\x00\x00")
	if p == "" {
		return "cancel"
	}
	if !strings.HasSuffix(strings.ToLower(p), ".exe") || !fileExists(p) {
		return "Scegli un file .exe."
	}
	name := niceName(strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)))
	if gn := filepath.Base(filepath.Dir(p)); gn != "" && !regexp.MustCompile(`(?i)^(bin|binaries|win64|win32|x64|x86|game)$`).MatchString(gn) && len(gn) > 2 {
		name = niceName(gn)
	}
	updateSettings(func(s *Settings) {
		for _, c := range s.CustomGames {
			if strings.EqualFold(c.Exe, p) {
				return
			}
		}
		if len(s.CustomGames) < 300 {
			s.CustomGames = append(s.CustomGames, CustomGame{Name: name, Exe: p})
		}
		s.HiddenGames = removeStr(s.HiddenGames, "man:"+shortHash(p))
	})
	go scanLibrary()
	return "ok"
}

// hideGame: toglie un gioco dalla libreria (quelli aggiunti a mano si cancellano, gli altri si nascondono).
func hideGame(id string, on bool) string {
	if len(id) > 200 {
		return "Gioco non valido."
	}
	updateSettings(func(s *Settings) {
		if strings.HasPrefix(id, "man:") && on {
			out := s.CustomGames[:0]
			for _, c := range s.CustomGames {
				if "man:"+shortHash(c.Exe) != id {
					out = append(out, c)
				}
			}
			s.CustomGames = out
			return
		}
		s.HiddenGames = removeStr(s.HiddenGames, id)
		if on && len(s.HiddenGames) < 1000 {
			s.HiddenGames = append(s.HiddenGames, id)
		}
	})
	return "ok"
}

func unhideAllGames() string {
	updateSettings(func(s *Settings) { s.HiddenGames = nil })
	go scanLibrary()
	return "ok"
}

func removeStr(list []string, x string) []string {
	out := list[:0:0]
	for _, v := range list {
		if v != x {
			out = append(out, v)
		}
	}
	return out
}

// ---------- copertine ----------

// epicCovers: le copertine che Epic ha già sul PC (catcache.bin = JSON in base64).
func epicCovers() map[string][2]string {
	out := map[string][2]string{}
	b, err := os.ReadFile(filepath.Join(os.Getenv("LOCALAPPDATA"), "EpicGamesLauncher", "Saved", "Data", "Catalog", "catcache.bin"))
	if err != nil || len(b) > 64<<20 {
		return out
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil {
		return out
	}
	var items []struct {
		ID          string `json:"id"`
		ReleaseInfo []struct {
			AppID string `json:"appId"`
		} `json:"releaseInfo"`
		KeyImages []struct {
			Type, URL string
		} `json:"keyImages"`
	}
	if json.Unmarshal(raw, &items) != nil {
		return out
	}
	for _, it := range items {
		var tall, wide string
		for _, k := range it.KeyImages {
			if !strings.HasPrefix(k.URL, "https://") {
				continue
			}
			switch k.Type {
			case "DieselGameBoxTall":
				tall = k.URL
			case "DieselGameBox", "Thumbnail":
				if wide == "" {
					wide = k.URL
				}
			}
		}
		if tall == "" && wide == "" {
			continue
		}
		for _, r := range it.ReleaseInfo {
			if r.AppID != "" {
				out[r.AppID] = [2]string{tall, wide}
			}
		}
	}
	return out
}

// Copertine da Steam per i giochi degli altri negozi: cerco il nome nel negozio di Steam
// e uso la copertina solo se il nome è proprio quello. Ricordo le risposte (anche i "non trovato").
var (
	coverMu    sync.Mutex
	coverCache map[string]coverHit
)

type coverHit struct {
	App  string `json:"a"`
	When int64  `json:"t"`
}

func coverCacheFile() string { return filepath.Join(appDir, "copertine.json") }

func normTitle(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func steamCoverFor(name string) string {
	key := normTitle(name)
	if len(key) < 2 {
		return ""
	}
	coverMu.Lock()
	if coverCache == nil {
		coverCache = map[string]coverHit{}
		if b, err := os.ReadFile(coverCacheFile()); err == nil {
			_ = json.Unmarshal(b, &coverCache)
		}
	}
	h, ok := coverCache[key]
	coverMu.Unlock()
	if ok && (h.App != "" || time.Since(time.Unix(h.When, 0)) < 7*24*time.Hour) {
		return h.App
	}
	app := ""
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get("https://store.steampowered.com/api/storesearch/?l=english&cc=US&term=" + url.QueryEscape(name))
	if err != nil {
		return ""
	}
	var r struct {
		Items []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	ok2 := resp.StatusCode == 200 && json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(&r) == nil
	resp.Body.Close()
	if !ok2 {
		return ""
	}
	for _, it := range r.Items {
		if normTitle(it.Name) == key {
			app = strconv.Itoa(it.ID)
			break
		}
	}
	coverMu.Lock()
	coverCache[key] = coverHit{App: app, When: time.Now().Unix()}
	if b, err := json.Marshal(coverCache); err == nil {
		_ = os.WriteFile(coverCacheFile(), b, 0o644)
	}
	coverMu.Unlock()
	return app
}

func steamCDN(app string) (string, string) {
	cdn := "https://shared.cloudflare.steamstatic.com/store_item_assets/steam/apps/" + app + "/"
	return cdn + "library_600x900.jpg", cdn + "header.jpg"
}

// fillCovers: copertine per chi non ce l'ha (al massimo 40 ricerche per volta, 4 alla volta).
func fillCovers(all []gameEntry) bool {
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	changed := false
	var mu sync.Mutex
	n := 0
	for i := range all {
		if all[i].Img != "" || all[i].Store == "Steam" {
			continue
		}
		if n++; n > 40 {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(g *gameEntry) {
			defer wg.Done()
			defer func() { <-sem }()
			defer guard("games_more.go:fillCovers")
			if app := steamCoverFor(g.Name); app != "" {
				mu.Lock()
				g.Img, g.ImgAlt = steamCDN(app)
				changed = true
				mu.Unlock()
			}
		}(&all[i])
	}
	wg.Wait()
	return changed
}

// pickFile: finestra di Windows per scegliere un file. filter: "Nome\x00*.est\x00\x00".
func pickFile(title, filter string) string {
	buf := make([]uint16, 4096)
	f := utf16.Encode([]rune(filter))
	t, _ := windows.UTF16PtrFromString(title)
	var hwnd uintptr
	if wv != nil {
		hwnd = uintptr(wv.Window())
	}
	ofn := openFileName{hwndOwner: hwnd, lpstrFilter: &f[0], lpstrFile: &buf[0], nMaxFile: uint32(len(buf)), lpstrTitle: t,
		flags: 0x00001000 | 0x00000800 | 0x00000008 | 0x00080000}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	if r, _, _ := pGetOpenFile.Call(uintptr(unsafe.Pointer(&ofn))); r == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}
