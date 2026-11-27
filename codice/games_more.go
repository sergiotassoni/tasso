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
	"io"
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
			dep, _, _ := sk.GetStringValue("dependsOn")
			sk.Close()
			if name == "" || dep != "" || len(dir) <= 3 || !dirExists(dir) {
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
		if strings.TrimSpace(dir) == "" {
			continue
		}
		dir = filepath.Clean(strings.ReplaceAll(dir, "/", `\`))
		if len(dir) <= 3 || !dirExists(dir) {
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
	xbSafeRe  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,200}$`)
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
				if f := fam[strings.ToLower(string(m[1]))]; f != "" && xbSafeRe.MatchString(f) && xbSafeRe.MatchString(appID) {
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
	notGameNameRe = regexp.MustCompile(`(?i)^(steam|battle\.net|ea app|ea desktop|origin|ubisoft connect|uplay|gog galaxy( \d+)?|rockstar games launcher|rockstar games social club|social club|amazon games|epic games launcher|epic online services|riot client|xbox|itch)$|\b(redistributable|runtime|directx|visual c\+\+|anti-?cheat|easyanticheat|battleye|punkbuster|physx|\.net framework|sdk|dedicated server|mod manager|uninstaller)\b`)
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

// gameMarkers: file o cartelle che hanno quasi solo i giochi (ne basta uno).
var gameMarkers = []string{
	"UnityPlayer.dll", "UnityCrashHandler64.exe", "UnityCrashHandler32.exe", // Unity
	`Engine\Binaries`, `Engine\Content\Paks`, // Unreal
	"data.win", "options.ini.gmx", // GameMaker
	"renpy", `game\script_version.txt`, // Ren'Py
	`www\js\rpg_core.js`, `www\js\rmmz_core.js`, `js\rmmz_core.js`, "Game.rgss3a", "Game.rgssad", "RGSS301.dll", "RGSS104E.dll", // RPG Maker
	"steam_api.dll", "steam_api64.dll", "steam_appid.txt", "EOSSDK-Win64-Shipping.dll", "galaxy64.dll", "Galaxy.dll", // SDK dei negozi
	"love.dll", "bink2w64.dll", "binkw32.dll", "binkw64.dll", "MonoBleedingEdge",
}

// gameMarkersWeak: li hanno anche programmi normali: ne servono almeno due.
var gameMarkersWeak = []string{"fmod.dll", "fmodex.dll", "fmodstudio.dll", "d3dx9_43.dll", "xinput1_3.dll", "SDL2.dll", "openal32.dll", "nw.dll", "physxloader.dll", "PhysX3_x64.dll"}

func looksLikeGame(dir string) bool {
	for _, m := range gameMarkers {
		if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
			return true
		}
	}
	if pcks, _ := filepath.Glob(filepath.Join(dir, "*.pck")); len(pcks) > 0 { // Godot
		return true
	}
	if d, _ := filepath.Glob(filepath.Join(dir, "*_Data", "globalgamemanagers")); len(d) > 0 { // Unity
		return true
	}
	weak := 0
	for _, m := range gameMarkersWeak {
		if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
			if weak++; weak >= 2 {
				return true
			}
		}
	}
	return false
}

// notGameExeRe: installatori e strumenti che stanno spesso nella cartella del gioco.
var notGameExeRe = regexp.MustCompile(`(?i)(unins|uninst|setup|install|redist|vcredist|vc_redist|dxsetup|directx|dotnet|oalinst|physx|crash|report|helper|update|patch|config|settings|benchmark|server|editor|ue4prereq|prereq|easyanticheat|battleye|cefprocess|webhelper|7z|unitycrash)`)

// bestExe: il programma del gioco in una cartella: preferisco quello con un nome simile alla cartella, se no il più grande.
func bestExe(dir string) string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	want := normTitle(filepath.Base(dir))
	best, bestScore := "", int64(-1)
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(n), ".exe") || notGameExeRe.MatchString(n) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		score := fi.Size()
		if nt := normTitle(strings.TrimSuffix(n, filepath.Ext(n))); len(nt) >= 3 && len(want) >= 3 && (strings.Contains(want, nt) || strings.Contains(nt, want)) {
			score += 1 << 40
		}
		if score > bestScore {
			best, bestScore = filepath.Join(dir, n), score
		}
	}
	return best
}

// gameDirExe: il programma del gioco (anche dentro Binaries\Win64 per Unreal).
func gameDirExe(dir string) string {
	if e := bestExe(dir); e != "" {
		return e
	}
	for _, sub := range []string{"bin", "Bin", "x64", `Binaries\Win64`, "Game", "Content"} {
		if e := bestExe(filepath.Join(dir, sub)); e != "" {
			return e
		}
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "*", "Binaries", "Win64")); len(m) > 0 {
		if e := bestExe(m[0]); e != "" {
			return e
		}
	}
	return ""
}

func fixedDrives() []string {
	var out []string
	for c := 'C'; c <= 'Z'; c++ {
		d := string(c) + `:\`
		p, _ := windows.UTF16PtrFromString(d)
		if windows.GetDriveType(p) == windows.DRIVE_FIXED && dirExists(d) {
			out = append(out, d)
		}
	}
	return out
}

// skipDirRe: cartelle di Program Files che non sono giochi (si risparmia tempo).
var skipDirRe = regexp.MustCompile(`(?i)^(common files|windows.*|microsoft.*|internet explorer|modifiablewindowsapps|windowsapps|reference assemblies|dotnet|nvidia.*|amd|intel|realtek|google|mozilla.*|7-zip|winrar|java|uninstall information|msbuild|iis.*|steam|epic games|riot games|gog galaxy|ea|ea games|origin|ubisoft|battle\.net|rockstar games|amazon games|xboxgames|windowspowershell|package cache|tasso)$`)

func scanFolders(known map[string]bool, deadline time.Time) []gameEntry {
	type root struct {
		path    string
		depth   int
		anyExe  bool // cartelle solo di giochi: va bene qualsiasi programma (anche i giochi senza motore riconoscibile)
		private bool // Desktop e Download: il nome non va mandato a Steam per la copertina
		store   string
	}
	var roots []root
	add := func(p string, depth int, anyExe, private bool, store string) {
		if p == "" || len(p) <= 3 || !filepath.IsAbs(p) {
			return
		}
		roots = append(roots, root{p, depth, anyExe, private, store})
	}
	for _, d := range fixedDrives() {
		for _, n := range []string{"Games", "Giochi", "Game", "Gioco", "GOG Games", "EA Games", "Ubisoft Games"} {
			add(filepath.Join(d, n), 1, true, false, "Altro")
		}
		add(filepath.Join(d, "Program Files"), 0, false, false, "Altro")
		add(filepath.Join(d, "Program Files (x86)"), 0, false, false, "Altro")
	}
	if home := os.Getenv("USERPROFILE"); home != "" {
		add(filepath.Join(home, "Games"), 1, true, false, "Altro")
		add(filepath.Join(home, "Giochi"), 1, true, false, "Altro")
		add(filepath.Join(home, "Desktop"), 0, false, true, "Altro")
		add(filepath.Join(home, "Downloads"), 0, false, true, "Altro")
	}
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		add(filepath.Join(la, "Programs"), 0, false, false, "Altro")
	}
	if ad := os.Getenv("APPDATA"); ad != "" {
		add(filepath.Join(ad, "itch", "apps"), 0, true, false, "itch.io")
	}
	var out []gameEntry
	seen := map[string]bool{}
	var walk func(r root, dir string, depth int)
	walk = func(r root, dir string, depth int) {
		ents, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range ents {
			if time.Now().After(deadline) {
				return
			}
			if !e.IsDir() || e.Type()&os.ModeSymlink != 0 || skipDirRe.MatchString(e.Name()) || strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "$") {
				continue
			}
			p := filepath.Join(dir, e.Name())
			lp := strings.ToLower(p)
			if seen[lp] || known[lp] || insideKnown(lp, known) {
				continue
			}
			seen[lp] = true
			if r.anyExe || looksLikeGame(p) {
				if exe := gameDirExe(p); exe != "" {
					out = append(out, gameEntry{ID: "dir:" + shortHash(lp), Name: niceName(e.Name()), Store: r.store, exe: exe, dir: p, Icon: exe, private: r.private})
					known[lp] = true
					continue
				}
			}
			if depth > 0 {
				walk(r, p, depth-1)
			}
		}
	}
	for _, r := range roots {
		if dirExists(r.path) && !time.Now().After(deadline) {
			walk(r, r.path, r.depth)
		}
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

var verTailRe = regexp.MustCompile(`(?i)([\s._-]+v\d+(\.\d+)*|[\s._-]+\d+(\.\d+){2,})[\w.()-]*$`)

// niceName: "the_witcher-3_v1.2.0" -> "The witcher 3" (tolgo solo la versione in fondo)
func niceName(n string) string {
	s := verTailRe.ReplaceAllString(n, "")
	if strings.TrimSpace(s) == "" {
		s = n
	}
	s = strings.NewReplacer("_", " ", "-", " ").Replace(s)
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
		dir := filepath.Dir(c.Exe)
		if isBroadDir(dir) {
			dir = "" // niente "Regola con Tasso" su cartelle come Download o Desktop
		}
		// Nota: per riconoscere un gioco aggiunto a mano conta solo il suo programma esatto, mai la
		// cartella (vedi libraryGamePaths).
		out = append(out, gameEntry{ID: "man:" + shortHash(c.Exe), Name: c.Name, Store: "Mano", exe: c.Exe, dir: dir, Icon: c.Exe, private: true})
	}
	return out
}

var genericDirRe = regexp.MustCompile(`(?i)^(bin|binaries|win64|win32|x64|x86|game|games|giochi|downloads|download|desktop|documents|documenti|program files.*|programs)$`)

// isBroadDir: cartelle troppo grandi per "Regola con Tasso" e per riconoscere un gioco dalla
// cartella (Desktop, Download, Documenti anche se sono in OneDrive o su un altro disco, Desktop
// pubblico, radici di OneDrive, Program Files, radice del disco, Windows, AppData). Anche le
// cartelle "generiche" appena sotto la radice di un disco (D:\Download, D:\Portable, D:\Programmi...).
func isBroadDir(d string) bool {
	if d == "" {
		return true
	}
	d = strings.ToLower(strings.TrimRight(filepath.Clean(d), `\`))
	if len(d) <= 3 {
		return true
	}
	if broadDirSet()[d] {
		return true
	}
	if parent := filepath.Dir(d); len(strings.TrimRight(parent, `\`)) <= 2 && topGenericDirRe.MatchString(filepath.Base(d)) {
		return true
	}
	win := strings.ToLower(winDir())
	return win != "" && strings.HasPrefix(d, win+`\`)
}

// topGenericDirRe: nomi di cartelle che, subito sotto la radice di un disco, contengono di solito tanti programmi diversi.
var topGenericDirRe = regexp.MustCompile(`(?i)^(downloads?|desktop|documents|documenti|scaricati|portable|portatili|programs|programmi|program files.*|apps|applicazioni|software|tools|strumenti|utility|utilities|users|utenti|onedrive.*)$`)

var (
	broadOnce sync.Once
	broadDirs map[string]bool
)

// broadDirSet: le cartelle "larghe" vere, chieste a Windows una volta sola (non dalle variabili d'ambiente).
func broadDirSet() map[string]bool {
	broadOnce.Do(func() {
		m := map[string]bool{}
		add := func(p string) {
			if p != "" {
				m[strings.ToLower(strings.TrimRight(filepath.Clean(p), `\`))] = true
			}
		}
		for _, id := range []*windows.KNOWNFOLDERID{windows.FOLDERID_Profile, windows.FOLDERID_Desktop, windows.FOLDERID_Downloads,
			windows.FOLDERID_Documents, windows.FOLDERID_PublicDesktop, windows.FOLDERID_PublicDownloads, windows.FOLDERID_PublicDocuments,
			windows.FOLDERID_Public, windows.FOLDERID_OneDrive, windows.FOLDERID_ProgramFiles, windows.FOLDERID_ProgramFilesX86,
			windows.FOLDERID_ProgramData, windows.FOLDERID_LocalAppData, windows.FOLDERID_RoamingAppData, windows.FOLDERID_Windows} {
			add(cartellaNota(id))
		}
		// tutte le radici di OneDrive (personale e lavoro), come le scrive OneDrive nel registro
		if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\OneDrive\Accounts`, registry.ENUMERATE_SUB_KEYS); err == nil {
			names, _ := k.ReadSubKeyNames(50)
			k.Close()
			for _, n := range names {
				if a, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\OneDrive\Accounts\`+n, registry.QUERY_VALUE); err == nil {
					if v, _, err := a.GetStringValue("UserFolder"); err == nil && filepath.IsAbs(v) {
						add(v)
						// Desktop, Documenti e Download dentro OneDrive (anche se Windows non li ha spostati lì)
						for _, sub := range []string{"Desktop", "Documents", "Documenti", "Downloads"} {
							add(filepath.Join(v, sub))
						}
					}
					a.Close()
				}
			}
		}
		broadDirs = m
	})
	return broadDirs
}

// addGame: finestra di Windows per scegliere il programma del gioco.
func addGame() string {
	p := pickFile(trGo("Scegli il programma del gioco (.exe)"), trGo("Programmi")+"\x00*.exe\x00\x00")
	if p == "" {
		return "cancel"
	}
	if !strings.HasSuffix(strings.ToLower(p), ".exe") || !fileExists(p) {
		return "Scegli un file .exe."
	}
	name := niceName(strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)))
	if gn := filepath.Base(filepath.Dir(p)); len(gn) > 2 && !genericDirRe.MatchString(gn) && !isBroadDir(filepath.Dir(p)) {
		name = niceName(gn)
	}
	// Già in libreria (trovato da Tasso o aggiunto prima, magari nascosto): lo rimostro e lo dico.
	gia, giaID := "", ""
	libMu.Lock()
	for id, g := range library {
		if g.exe != "" && strings.EqualFold(g.exe, p) {
			gia, giaID = g.Name, id
			break
		}
	}
	libMu.Unlock()
	updateSettings(func(s *Settings) {
		if giaID != "" {
			s.HiddenGames = withoutStr(s.HiddenGames, giaID)
		}
		s.HiddenGames = withoutStr(s.HiddenGames, "man:"+shortHash(p))
		for _, c := range s.CustomGames {
			if strings.EqualFold(c.Exe, p) {
				if gia == "" {
					gia = c.Name
				}
				return
			}
		}
		if gia == "" && len(s.CustomGames) < 300 {
			s.CustomGames = append(s.CustomGames, CustomGame{Name: name, Exe: p})
		}
	})
	rescanLibrary()
	if gia != "" {
		return "gia:" + gia
	}
	return "ok"
}

// hideGame: toglie un gioco dalla libreria (quelli aggiunti a mano si cancellano, gli altri si nascondono).
func hideGame(id string, on bool) string {
	if len(id) > 200 {
		return "Gioco non valido."
	}
	libMu.Lock()
	if strings.HasPrefix(id, "man:") && on {
		delete(library, id)
	} else if g, ok := library[id]; ok {
		// anche nella libreria in memoria: un gioco nascosto non conta più come gioco
		g.Hidden = on
		library[id] = g
	}
	libMu.Unlock()
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
		s.HiddenGames = withoutStr(s.HiddenGames, id)
		if on && len(s.HiddenGames) < 1000 {
			s.HiddenGames = append(s.HiddenGames, id)
		}
	})
	return "ok"
}

func unhideAllGames() string {
	updateSettings(func(s *Settings) { s.HiddenGames = nil })
	rescanLibrary()
	return "ok"
}

func withoutStr(list []string, x string) []string {
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
	Hdr  string `json:"h,omitempty"` // copertina orizzontale vera di Steam (indirizzo con il codice), per i giochi nuovi
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

func loadCoverCache() {
	if coverCache != nil {
		return
	}
	coverCache = map[string]coverHit{}
	if st, err := os.Stat(coverCacheFile()); err == nil && st.Size() < 4<<20 {
		if b, err := leggiFileApp(coverCacheFile()); err == nil {
			_ = json.Unmarshal(b, &coverCache)
		}
	}
	for k, h := range coverCache {
		if h.App != "" && strings.Trim(h.App, "0123456789") != "" {
			delete(coverCache, k)
		}
	}
}

// cachedCover: la risposta già nota (known=false se va chiesta a Steam).
func cachedCover(name string) (app string, known bool) {
	key := normTitle(name)
	if len(key) < 2 {
		return "", true
	}
	coverMu.Lock()
	defer coverMu.Unlock()
	loadCoverCache()
	h, ok := coverCache[key]
	if ok && (h.App != "" || time.Since(time.Unix(h.When, 0)) < 7*24*time.Hour) {
		return h.App, true
	}
	return "", false
}

func steamCoverFor(name string) string {
	key := normTitle(name)
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
	ok2 := resp.StatusCode == 200 && json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r) == nil
	resp.Body.Close()
	if !ok2 {
		return ""
	}
	for _, it := range r.Items {
		if normTitle(it.Name) == key && it.ID > 0 {
			app = strconv.Itoa(it.ID)
			break
		}
	}
	coverMu.Lock()
	loadCoverCache()
	coverCache[key] = coverHit{App: app, When: time.Now().Unix()}
	if b, err := json.Marshal(coverCache); err == nil {
		_ = scriviFileApp(coverCacheFile(), b, 0o644) // F20-6: niente collegamenti o giunzioni
	}
	coverMu.Unlock()
	return app
}

func steamCDN(app string) (string, string) {
	cdn := "https://shared.cloudflare.steamstatic.com/store_item_assets/steam/apps/" + app + "/"
	return cdn + "library_600x900.jpg", cdn + "header.jpg"
}

// steamHdrRe: solo immagini di Steam (niente indirizzi strani nella pagina).
var steamHdrRe = regexp.MustCompile(`^https://(shared|cdn)\.(akamai|cloudflare|fastly)\.steamstatic\.com/store_item_assets/steam/apps/[0-9]+/[^\s"'<>]+\.(jpg|png)(\?t=[0-9]+)?$`)

// cachedHeader: la copertina di riserva già nota per questo numero Steam (known=false se va chiesta).
func cachedHeader(app string) (string, bool) {
	coverMu.Lock()
	defer coverMu.Unlock()
	loadCoverCache()
	h, ok := coverCache["#"+app]
	if ok && (h.Hdr != "" || time.Since(time.Unix(h.When, 0)) < 7*24*time.Hour) {
		return h.Hdr, true
	}
	return "", false
}

// steamHeaderFor: i giochi usciti da poco hanno le immagini di Steam solo in un indirizzo con un
// codice dentro, quindi library_600x900.jpg e header.jpg "a mano" non esistono (404).
// Chiedo a Steam l'indirizzo giusto dell'immagine orizzontale (manda solo il numero del gioco).
func steamHeaderFor(app string) string {
	hdr := ""
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get("https://store.steampowered.com/api/appdetails?appids=" + url.QueryEscape(app) + "&filters=basic")
	if err != nil {
		// Niente rete: riprovo tra un'ora.
		coverMu.Lock()
		loadCoverCache()
		coverCache["#"+app] = coverHit{When: time.Now().Add(-7*24*time.Hour + time.Hour).Unix()}
		coverMu.Unlock()
		return ""
	}
	var r map[string]struct {
		Success bool `json:"success"`
		Data    struct {
			Header string `json:"header_image"`
		} `json:"data"`
	}
	ok2 := resp.StatusCode == 200 && json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&r) == nil
	resp.Body.Close()
	if !ok2 {
		// Steam occupato (429) o rete lenta: lo segno, così riprovo tra un'ora e non a ogni scansione.
		coverMu.Lock()
		loadCoverCache()
		coverCache["#"+app] = coverHit{When: time.Now().Add(-7*24*time.Hour + time.Hour).Unix()}
		coverMu.Unlock()
		return ""
	}
	if d, ok := r[app]; ok && d.Success && steamHdrRe.MatchString(d.Data.Header) {
		hdr = d.Data.Header
	}
	coverMu.Lock()
	loadCoverCache()
	coverCache["#"+app] = coverHit{Hdr: hdr, When: time.Now().Unix()}
	if b, err := json.Marshal(coverCache); err == nil {
		_ = scriviFileApp(coverCacheFile(), b, 0o644) // F20-6: niente collegamenti o giunzioni
	}
	coverMu.Unlock()
	return hdr
}

// fillCovers: copertine per chi non ce l'ha. Prima quelle già note (senza internet),
// poi al massimo 40 ricerche nuove per volta, 4 alla volta. Desktop, Download e giochi aggiunti a mano: niente ricerca.
func fillCovers(all []gameEntry) bool {
	changed := false
	setMu.Lock()
	cercaPerNome := settings.CoverInternet
	setMu.Unlock()
	var ask []*gameEntry
	type hdrAsk struct {
		g   *gameEntry
		app string
	}
	var askHdr []hdrAsk
	// needHdr: la copertina viene da internet (non dal PC): preparo quella di riserva giusta.
	needHdr := func(g *gameEntry, app string) {
		if !strings.HasPrefix(g.Img, "https://") {
			return
		}
		if h, known := cachedHeader(app); known {
			if h != "" && g.ImgAlt != h {
				g.ImgAlt = h
				changed = true
			}
			return
		}
		if len(askHdr) < 40 {
			askHdr = append(askHdr, hdrAsk{g, app})
		}
	}
	for i := range all {
		g := &all[i]
		if g.Store == "Steam" && g.steamApp != "" {
			needHdr(g, g.steamApp)
			continue
		}
		if g.Img != "" || g.Store == "Steam" || g.private {
			continue
		}
		if app, known := cachedCover(g.Name); known {
			if app != "" {
				g.Img, g.ImgAlt = steamCDN(app)
				changed = true
				needHdr(g, app)
			}
			continue
		}
		// Senza il sì dell'utente i nomi dei giochi non escono dal PC: niente ricerca su Steam.
		if coverDaCercare(cercaPerNome, len(ask)) {
			ask = append(ask, g)
		}
	}
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, g := range ask {
		wg.Add(1)
		sem <- struct{}{}
		go func(g *gameEntry) {
			defer wg.Done()
			defer func() { <-sem }()
			defer guard("games_more.go:fillCovers")
			if app := steamCoverFor(g.Name); app != "" {
				h, known := cachedHeader(app)
				if !known {
					h = steamHeaderFor(app)
				}
				mu.Lock()
				g.Img, g.ImgAlt = steamCDN(app)
				if h != "" {
					g.ImgAlt = h
				}
				changed = true
				mu.Unlock()
			}
		}(g)
	}
	for _, a := range askHdr {
		wg.Add(1)
		sem <- struct{}{}
		go func(a hdrAsk) {
			defer wg.Done()
			defer func() { <-sem }()
			defer guard("games_more.go:fillCovers hdr")
			if h := steamHeaderFor(a.app); h != "" {
				mu.Lock()
				a.g.ImgAlt = h
				changed = true
				mu.Unlock()
			}
		}(a)
	}
	wg.Wait()
	return changed
}

// coverDaCercare: la copertina di un gioco non di Steam si cerca su Steam per nome solo con
// "Copertine da internet" acceso, e al massimo 40 per volta.
func coverDaCercare(permesso bool, giaInCoda int) bool { return permesso && giaInCoda < 40 }

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

// userUninstallCmd: programma e argomenti di disinstallazione di una voce HKCU.
func userUninstallCmd(key string) (string, string) {
	const pre = `HKEY_CURRENT_USER\`
	if !strings.HasPrefix(strings.ToUpper(key), pre) {
		return "", ""
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, key[len(pre):], registry.QUERY_VALUE)
	if err != nil {
		return "", ""
	}
	cmd, _, _ := k.GetStringValue("UninstallString")
	k.Close()
	cmd = strings.TrimSpace(expandEnv(cmd))
	exe := exeFromCmd(cmd)
	if exe == "" {
		return "", ""
	}
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(cmd, `"`+exe+`"`), exe))
	if !filepath.IsAbs(exe) {
		exe = filepath.Join(os.Getenv("SystemRoot"), "System32", exe)
	}
	if !fileExists(exe) {
		return "", ""
	}
	return exe, rest
}

// rescanLibrary: rifà la libreria; se ne sta già girando una (es. mentre cerca le copertine) parte subito dopo.
func rescanLibrary() {
	if async("library", scanLibrary) == "busy" {
		go func() {
			defer guard("games_more.go:rescanLibrary")
			scanLibrary()
		}()
	}
}
