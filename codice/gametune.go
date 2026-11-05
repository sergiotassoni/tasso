package main

// "Regola con Tasso": per ogni gioco Tasso capisce che tipo di gioco è (sparatutto/competitivo,
// grafico/avventura, oppure "prestazionale" cioè simulazione, strategia, sopravvivenza) e quanto è
// potente il PC, poi consiglia la grafica e applica da solo quello che può applicare in modo sicuro:
//   - scheda video ad alte prestazioni e "ottimizzazioni per i giochi in finestra" (impostazioni di Windows)
//   - modalità del PC (Gaming o FPS) che parte da sola quando apri quel gioco
//   - qualità grafica, VSync e limite FPS dentro il gioco, solo per i giochi Unreal Engine (file di
//     impostazioni GameUserSettings.ini, con copia di sicurezza)
//   - facoltativi: priorità alta del processore, ottimizzazioni a schermo intero spente
// Anti-Lag, Radeon Boost, Enhanced Sync / Reflex si impostano nel pannello del driver: AMD e NVIDIA non
// permettono ad altri programmi di cambiarli, quindi Tasso dice esattamente cosa mettere e apre il pannello.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type GameTune struct {
	Name    string            `json:"name,omitempty"`
	Preset  string            `json:"preset"`
	Opts    tuneOpts          `json:"opts"`
	Exes    []string          `json:"exes"`
	Prev    map[string]string `json:"prev,omitempty"` // valori di prima, per "Annulla regolazione"
	Ini     string            `json:"ini,omitempty"`
	Applied string            `json:"applied"`
}

type tuneOpts struct {
	GpuHigh  bool   `json:"gpuHigh"`
	Mode     string `json:"mode"` // "gaming", "fps" o "" (nessuna)
	InGame   bool   `json:"inGame"`
	Quality  int    `json:"quality"` // 0 basso, 1 medio, 2 alto, 3 ultra
	VSync    bool   `json:"vsync"`
	FPSCap   int    `json:"fpsCap"` // 0 = nessun limite
	Priority bool   `json:"priority"`
	NoFSO    bool   `json:"noFso"`
}

var qualityNames = []string{"Basso", "Medio", "Alto", "Ultra"}

// ---------------- Tipo di gioco ----------------

var gameKinds = []struct {
	kind  string
	names []string
}{
	{"competitivo", []string{"valorant", "league of legends", "counter-strike", "cs2", "apex legends", "fortnite", "overwatch", "call of duty", "warzone", "rainbow six", "pubg", "rocket league", "arc raiders", "marvel rivals", "the finals", "dota", "deadlock", "battlefield", "escape from tarkov", "hunt: showdown", "delta force", "splitgate", "team fortress", "2xko", "street fighter", "tekken", "mortal kombat", "rust", "dead by daylight", "xdefiant", "halo infinite", "paladins", "smite", "brawlhalla", "war thunder", "world of tanks"}},
	{"grafico", []string{"cyberpunk", "red dead", "no man's sky", "subnautica", "elden ring", "starfield", "hogwarts", "witcher", "baldur", "god of war", "horizon", "spider-man", "alan wake", "black myth", "assassin's creed", "forza horizon", "flight simulator", "stalker", "s.t.a.l.k.e.r", "dragon's dogma", "final fantasy", "resident evil", "the last of us", "ghost of tsushima", "monster hunter", "indiana jones", "kingdom come", "avowed", "death stranding", "hellblade", "metro exodus", "dying light", "atomic heart", "lies of p", "star wars", "ark: survival ascended", "mafia", "silent hill", "doom", "clair obscur"}},
	{"prestazionale", []string{"palworld", "fishing planet", "liar's bar", "minecraft", "valheim", "satisfactory", "factorio", "cities: skylines", "euro truck", "american truck", "farming simulator", "the sims", "stardew", "terraria", "civilization", "total war", "age of empires", "hearts of iron", "stellaris", "rimworld", "7 days to die", "the forest", "sons of the forest", "grounded", "enshrouded", "raft", "phasmophobia", "lethal company", "among us", "fall guys", "sea of thieves", "genshin", "honkai", "wuthering waves", "legends of runeterra", "hearthstone", "teamfight tactics"}},
}

var kindLabel = map[string]string{
	"competitivo":   "Sparatutto / competitivo",
	"grafico":       "Grafico / avventura",
	"prestazionale": "Prestazionale (simulazione, sopravvivenza, strategia)",
}

func steamGenres(appid string) (genres, cats []string) {
	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Get("https://store.steampowered.com/api/appdetails?appids=" + appid + "&filters=genres,categories&l=english")
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()
	var m map[string]struct {
		Success bool `json:"success"`
		Data    struct {
			Genres     []struct{ Description string } `json:"genres"`
			Categories []struct{ Description string } `json:"categories"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&m) != nil {
		return nil, nil
	}
	d := m[appid].Data
	for _, g := range d.Genres {
		genres = append(genres, strings.ToLower(g.Description))
	}
	for _, c := range d.Categories {
		cats = append(cats, strings.ToLower(c.Description))
	}
	return
}

func gameKind(g gameEntry) (string, string) {
	n := strings.ToLower(g.Name)
	for _, k := range gameKinds {
		for _, name := range k.names {
			if strings.Contains(n, name) {
				return k.kind, "Tasso conosce questo gioco"
			}
		}
	}
	if strings.HasPrefix(g.ID, "steam:") {
		genres, cats := steamGenres(strings.TrimPrefix(g.ID, "steam:"))
		has := func(list []string, s string) bool {
			for _, x := range list {
				if strings.Contains(x, s) {
					return true
				}
			}
			return false
		}
		switch {
		case has(cats, "pvp") && (has(genres, "action") || has(genres, "sports") || has(genres, "racing")):
			return "competitivo", "dal negozio di Steam: azione con sfide online contro altri giocatori"
		case (has(genres, "rpg") || has(genres, "adventure")) && !has(genres, "indie") && !has(genres, "casual"):
			return "grafico", "dal negozio di Steam: avventura / gioco di ruolo"
		case len(genres) > 0:
			return "prestazionale", "dal negozio di Steam: " + strings.Join(genres, ", ")
		}
	}
	return "prestazionale", "tipo non riconosciuto: Tasso usa una regolazione bilanciata"
}

// ---------------- Potenza del PC ----------------

var (
	rtxRe   = regexp.MustCompile(`(?i)rtx\s*(\d{2})(\d{2})`)
	gtxRe   = regexp.MustCompile(`(?i)gtx\s*(\d{3,4})`)
	radeRe  = regexp.MustCompile(`(?i)rx\s*(\d{4})`)
	radeOld = regexp.MustCompile(`(?i)rx\s*(\d{3})\b`)
	arcRe   = regexp.MustCompile(`(?i)arc(\(tm\))?\s*[ab]\d{3}`)
)

// gpuTier: 0 base, 1 media, 2 alta, 3 top.
func gpuTier(name string) int {
	if m := rtxRe.FindStringSubmatch(name); m != nil {
		gen, _ := strconv.Atoi(m[1])
		model, _ := strconv.Atoi(m[2])
		switch {
		case model >= 80:
			return 3
		case model == 70:
			if gen >= 40 {
				return 3
			}
			return 2
		case model == 60:
			if gen >= 30 {
				return 2
			}
			return 1
		default:
			return 1
		}
	}
	if m := gtxRe.FindStringSubmatch(name); m != nil {
		n, _ := strconv.Atoi(m[1])
		if n >= 1650 || n == 1070 || n == 1080 {
			return 1
		}
		return 0
	}
	if m := radeRe.FindStringSubmatch(name); m != nil {
		n, _ := strconv.Atoi(m[1])
		gen, model := n/1000, (n/100)%10
		switch gen {
		case 9: // RX 9070 / 9060: il modello è la terza cifra
			model = (n / 10) % 10
			if model >= 7 {
				return 3
			}
			return 2
		case 7:
			if model >= 9 {
				return 3
			}
			if model >= 7 {
				return 2
			}
			return 1
		case 6:
			if model >= 7 {
				return 2
			}
			if model == 6 {
				return 1
			}
			return 0
		case 5:
			if model >= 7 {
				return 1
			}
			return 0
		}
		return 1
	}
	if radeOld.MatchString(name) {
		return 0
	}
	if arcRe.MatchString(name) {
		return 1
	}
	return 0 // integrate (Intel UHD/Iris, Radeon Graphics) o sconosciute
}

var tierLabel = []string{"base", "media", "alta", "top"}

func pcTier() (int, string, int) {
	hwOnce.Do(loadHwCache)
	gpu := ""
	hz := 0
	for _, g := range hwCache.GPUs {
		if gpu == "" || gpuTier(g.Name) > gpuTier(gpu) {
			gpu = g.Name
		}
		if g.Hz > hz {
			hz = g.Hz
		}
	}
	t := gpuTier(gpu)
	ramGB := int(hwCache.RAM.Total / (1 << 30))
	why := strings.TrimSpace(shortName(gpu))
	if why == "" {
		why = "scheda video non riconosciuta"
	}
	if ramGB > 0 {
		why += fmt.Sprintf(", %d GB di RAM", ramGB+1)
		if ramGB < 15 && t > 0 {
			t-- // poca RAM: un gradino sotto
			why += " (poca RAM: un gradino sotto)"
		}
	}
	return t, why, hz
}

// ---------------- Consigli ----------------

type tuneRec struct {
	Preset   string     `json:"preset"`
	Label    string     `json:"label"`
	Opts     tuneOpts   `json:"opts"`
	Graphics [][]string `json:"graphics"` // righe "voce", "valore"
	Driver   [][]string `json:"driver"`
}

func recommend(preset string, tier, hz int, vendor string) tuneRec {
	r := tuneRec{Preset: preset}
	if hz < 50 {
		hz = 60
	}
	up := []string{"Prestazioni", "Bilanciato", "Qualità", "Qualità (o nativa)"}
	switch preset {
	case "competitivo":
		r.Label = "Competitivo: più FPS e meno ritardo"
		q := []int{0, 0, 1, 1}[tier]
		r.Opts = tuneOpts{GpuHigh: true, Mode: "fps", InGame: true, Quality: q, VSync: false, FPSCap: 0}
		upTxt := "spento (risoluzione nativa)"
		if tier <= 1 {
			upTxt = "FSR / DLSS / XeSS " + up[tier+1]
		}
		r.Graphics = [][]string{
			{"Qualità generale", qualityNames[q] + " (texture pure alte se hai VRAM)"},
			{"Ombre, nebbia, effetti, erba", "Basso: si vede meglio il nemico"},
			{"Upscaling", upTxt},
			{"VSync", "Spento"},
			{"Limite FPS", "Nessuno (o il doppio dei " + strconv.Itoa(hz) + " Hz del monitor)"},
			{"Sfocatura movimento", "Spenta"},
			{"Modalità finestra", "Schermo intero"},
		}
	case "grafico":
		r.Label = "Qualità: il gioco più bello possibile, fluido"
		q := []int{0, 1, 2, 3}[tier]
		r.Opts = tuneOpts{GpuHigh: true, Mode: "gaming", InGame: true, Quality: q, VSync: false, FPSCap: hz - 3}
		r.Graphics = [][]string{
			{"Qualità generale", qualityNames[q]},
			{"Upscaling", "FSR / DLSS / XeSS " + up[tier]},
			{"VSync", "Spento nel gioco, con FreeSync / G-Sync acceso (se il monitor non li ha: VSync acceso)"},
			{"Limite FPS", strconv.Itoa(hz-3) + " (3 sotto i " + strconv.Itoa(hz) + " Hz: sincronizzazione ottimale, niente strappi)"},
			{"Ray tracing", map[int]string{0: "Spento", 1: "Spento", 2: "Basso o spento", 3: "Medio/Alto"}[tier]},
			{"Generazione di fotogrammi", "Solo se fai già almeno 60 FPS"},
		}
	default:
		preset = "prestazionale"
		r.Preset = preset
		r.Label = "Bilanciato: stabile e senza scatti"
		q := []int{0, 1, 2, 2}[tier]
		r.Opts = tuneOpts{GpuHigh: true, Mode: "gaming", InGame: true, Quality: q, VSync: false, FPSCap: hz}
		upTxt := "spento"
		if tier <= 1 {
			upTxt = "FSR / DLSS / XeSS " + up[tier+1]
		}
		r.Graphics = [][]string{
			{"Qualità generale", qualityNames[q]},
			{"Distanza visiva", "Media: è quella che pesa di più nei mondi grandi"},
			{"Upscaling", upTxt},
			{"VSync", "Spento, con FreeSync / G-Sync acceso"},
			{"Limite FPS", strconv.Itoa(hz) + " (come il monitor: niente lavoro sprecato, PC più fresco)"},
		}
	}
	comp := preset == "competitivo"
	yes := func(b bool) string {
		if b {
			return "Attivo"
		}
		return "Spento"
	}
	switch {
	case strings.Contains(vendor, "amd") || strings.Contains(vendor, "radeon"):
		r.Driver = [][]string{
			{"Radeon Anti-Lag", yes(comp)},
			{"Radeon Boost", yes(comp && tier <= 1)},
			{"Enhanced Sync (sincronizzazione ottimale)", yes(preset == "prestazionale")},
			{"Radeon Chill", "Spento"},
			{"Attendi sincronizzazione verticale", "Spento, a meno che l'applicazione non lo specifichi"},
			{"AMD FreeSync", "Attivo (se il monitor lo supporta)"},
			{"Radeon Super Resolution", yes(tier == 0 && !comp)},
		}
	case strings.Contains(vendor, "nvidia") || strings.Contains(vendor, "geforce"):
		low := "Attiva"
		if comp {
			low = "Ultra (o NVIDIA Reflex Attivo + Boost nel gioco)"
		}
		r.Driver = [][]string{
			{"Modalità bassa latenza", low},
			{"Sincronizzazione verticale", map[bool]string{true: "Disattivata", false: "Veloce (Fast Sync)"}[comp]},
			{"G-SYNC", "Attivo (se il monitor lo supporta)"},
			{"Modalità di gestione energia", "Preferisci prestazioni massime"},
		}
	default:
		r.Driver = [][]string{{"Pannello della scheda video", "Bassa latenza attiva, VSync spento, sincronizzazione adattiva attiva"}}
	}
	return r
}

// ---------------- Programmi del gioco e file di impostazioni ----------------

var skipExe = regexp.MustCompile(`(?i)(unins|setup|install|redist|vcredist|dxsetup|dxweb|crash|report|launcher|anticheat|easyanticheat|battleye|^eac|prereq|dotnet|helper|update|patch|cef|webhelper|overlay|benchmark|config|settings|uploader|bugsplat|sentry|cleanup|vc_redist|unitycrash|ue4|ue5prereq)`)

// gameExes trova i programmi veri del gioco (al massimo 3), saltando installatori e programmi di servizio.
func gameExes(dir string) []string {
	if dir == "" || !dirExists(dir) {
		return nil
	}
	type cand struct {
		path  string
		size  int64
		score int
	}
	var list []cand
	base := strings.Count(filepath.Clean(dir), string(os.PathSeparator))
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.Count(p, string(os.PathSeparator))-base > 4 {
				return filepath.SkipDir
			}
			ln := strings.ToLower(d.Name())
			if ln == "redist" || ln == "_commonredist" || ln == "directx" || ln == "support" || ln == "easyanticheat" || ln == "battleye" {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !strings.EqualFold(filepath.Ext(name), ".exe") || skipExe.MatchString(name) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() < 1<<20 {
			return nil
		}
		sc := 0
		lp := strings.ToLower(p)
		if strings.Contains(lp, `\binaries\win64\`) {
			sc += 2
		}
		if strings.Contains(strings.ToLower(name), "shipping") {
			sc += 3
		}
		list = append(list, cand{p, info.Size(), sc})
		return nil
	})
	sort.Slice(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		return list[i].size > list[j].size
	})
	var out []string
	for i := 0; i < len(list) && i < 3; i++ {
		out = append(out, list[i].path)
	}
	return out
}

// unrealIni cerca il file GameUserSettings.ini di un gioco Unreal Engine.
func unrealIni(dir string) string {
	if dir == "" {
		return ""
	}
	ents, _ := os.ReadDir(dir)
	la := os.Getenv("LOCALAPPDATA")
	for _, e := range ents {
		if !e.IsDir() || !dirExists(filepath.Join(dir, e.Name(), "Binaries", "Win64")) {
			continue
		}
		for _, sub := range []string{"Windows", "WindowsNoEditor", "WindowsClient"} {
			p := filepath.Join(la, e.Name(), "Saved", "Config", sub, "GameUserSettings.ini")
			if fileExists(p) {
				return p
			}
		}
	}
	return ""
}

var sgKeys = []string{"sg.ViewDistanceQuality", "sg.AntiAliasingQuality", "sg.ShadowQuality", "sg.GlobalIlluminationQuality", "sg.ReflectionQuality", "sg.PostProcessQuality", "sg.TextureQuality", "sg.EffectsQuality", "sg.FoliageQuality", "sg.ShadingQuality"}

// writeUnrealIni cambia solo qualità, VSync e limite FPS; tutto il resto del file resta uguale.
func writeUnrealIni(path string, o tuneOpts) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	bak := path + ".tasso-backup"
	if !fileExists(bak) {
		if err := os.WriteFile(bak, b, 0o644); err != nil {
			return err
		}
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	want := map[string]map[string]string{"[scalabilitygroups]": {}}
	q := strconv.Itoa(o.Quality)
	for _, k := range sgKeys {
		want["[scalabilitygroups]"][k] = q
	}
	gus := ""
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") && strings.Contains(strings.ToLower(t), "gameusersettings]") {
			gus = strings.ToLower(t)
			break
		}
	}
	if gus == "" {
		gus = "[/script/engine.gameusersettings]"
	}
	vs := "False"
	if o.VSync {
		vs = "True"
	}
	want[gus] = map[string]string{"bUseVSync": vs, "FrameRateLimit": fmt.Sprintf("%.6f", float64(o.FPSCap)), "bUseDynamicResolution": "False"}

	done := map[string]map[string]bool{}
	var out []string
	cur := ""
	flush := func() {
		if w, ok := want[cur]; ok {
			for k, v := range w {
				if !done[cur][k] {
					out = append(out, k+"="+v)
					done[cur][k] = true
				}
			}
		}
	}
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			// prima di cambiare sezione aggiungo le chiavi mancanti (prima delle righe vuote finali)
			trail := 0
			for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
				out = out[:len(out)-1]
				trail++
			}
			flush()
			for i := 0; i < trail; i++ {
				out = append(out, "")
			}
			cur = strings.ToLower(t)
			if done[cur] == nil {
				done[cur] = map[string]bool{}
			}
			out = append(out, l)
			continue
		}
		if w, ok := want[cur]; ok {
			if i := strings.Index(t, "="); i > 0 {
				k := strings.TrimSpace(t[:i])
				for wk, wv := range w {
					if strings.EqualFold(k, wk) {
						l = wk + "=" + wv
						done[cur][wk] = true
					}
				}
			}
		}
		out = append(out, l)
	}
	flush()
	for sec, w := range want {
		if done[sec] != nil {
			continue
		}
		header := "[ScalabilityGroups]"
		if sec != "[scalabilitygroups]" {
			header = "[/Script/Engine.GameUserSettings]"
		}
		out = append(out, "", header)
		keys := make([]string, 0, len(w))
		for k := range w {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out = append(out, k+"="+w[k])
		}
	}
	res := strings.Join(out, "\r\n")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(res), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func exeRunning(exes []string) bool {
	want := map[string]bool{}
	for _, e := range exes {
		want[strings.ToLower(filepath.Base(e))] = true
	}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if want[strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))] {
			return true
		}
	}
	return false
}

// ---------------- Informazioni per la finestra "Regola" ----------------

func gpuVendor() string {
	hwOnce.Do(loadHwCache)
	v := ""
	for _, g := range hwCache.GPUs {
		n := strings.ToLower(g.Name)
		if strings.Contains(n, "nvidia") || strings.Contains(n, "geforce") || strings.Contains(n, "rtx") {
			return "nvidia"
		}
		if strings.Contains(n, "radeon") || strings.Contains(n, "amd") {
			v = "amd"
		} else if v == "" && strings.Contains(n, "intel") {
			v = "intel"
		}
	}
	return v
}

func gameTuneInfo(id string) {
	libMu.Lock()
	g, ok := library[id]
	libMu.Unlock()
	if !ok {
		emit("onGameTune", map[string]interface{}{"id": id, "error": "Gioco non trovato: aggiorna la libreria."})
		return
	}
	kind, kindWhy := gameKind(g)
	tier, tierWhy, hz := pcTier()
	vendor := gpuVendor()
	recs := map[string]tuneRec{}
	for _, p := range []string{"competitivo", "grafico", "prestazionale"} {
		recs[p] = recommend(p, tier, hz, vendor)
	}
	exes := gameExes(g.dir)
	ini := unrealIni(g.dir)
	setMu.Lock()
	var cur *GameTune
	if t, ok := settings.GameTunes[id]; ok && t != nil {
		c := *t
		cur = &c
	}
	autoGame := settings.AutoGame
	setMu.Unlock()
	var names []string
	for _, e := range exes {
		names = append(names, filepath.Base(e))
	}
	emit("onGameTune", map[string]interface{}{
		"id": id, "name": g.Name, "kind": kind, "kindLabel": kindLabel[kind], "kindWhy": kindWhy,
		"tier": tier, "tierLabel": tierLabel[tier], "tierWhy": tierWhy, "hz": hz, "vendor": vendor,
		"recs": recs, "exes": names, "hasExe": len(exes) > 0, "ini": ini != "", "current": cur,
		"autoGame": autoGame, "dir": g.dir != "",
	})
}

// ---------------- Applica / annulla ----------------

const (
	gpuPrefKey = `Software\Microsoft\DirectX\UserGpuPreferences`
	layersKey  = `Software\Microsoft\Windows NT\CurrentVersion\AppCompatFlags\Layers`
	ifeoKey    = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Image File Execution Options\`
)

func regGet(root registry.Key, path, name string) (string, bool) {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	return v, err == nil
}

func regSet(root registry.Key, path, name, val string) error {
	k, _, err := registry.CreateKey(root, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(name, val)
}

func regDel(root registry.Key, path, name string) {
	k, err := registry.OpenKey(root, path, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	_ = k.DeleteValue(name)
}

func gameTuneApply(id, preset string, o tuneOpts) {
	libMu.Lock()
	g, ok := library[id]
	libMu.Unlock()
	if !ok {
		emit("onGameTuneDone", map[string]interface{}{"id": id, "ok": false, "msgs": []string{"Gioco non trovato: aggiorna la libreria."}})
		return
	}
	if o.Quality < 0 || o.Quality > 3 {
		o.Quality = 2
	}
	if o.FPSCap < 0 || o.FPSCap > 1000 {
		o.FPSCap = 0
	}
	if o.Mode != "gaming" && o.Mode != "fps" {
		o.Mode = ""
	}
	exes := gameExes(g.dir)
	var msgs []string
	// Se c'era già una regolazione, riparto dai valori originali.
	gameTuneUndoQuiet(id)
	prev := map[string]string{}
	remember := func(key string, root registry.Key, path, name string) {
		if v, ok := regGet(root, path, name); ok {
			prev[key] = "1" + v
		} else {
			prev[key] = "0"
		}
	}
	if len(exes) == 0 && (o.GpuHigh || o.Priority || o.NoFSO || o.Mode != "") {
		msgs = append(msgs, "Non ho trovato il programma del gioco nella sua cartella: le impostazioni di Windows per questo gioco non si possono applicare.")
	}
	for _, e := range exes {
		if o.GpuHigh {
			remember("gpu|"+e, registry.CURRENT_USER, gpuPrefKey, e)
			if regSet(registry.CURRENT_USER, gpuPrefKey, e, "GpuPreference=2;SwapEffectUpgradeEnable=1;") == nil {
				msgs = append(msgs, "✓ Scheda video ad alte prestazioni e ottimizzazioni per i giochi in finestra: "+filepath.Base(e))
			}
		}
		if o.NoFSO {
			remember("fso|"+e, registry.CURRENT_USER, layersKey, e)
			// Aggiungo il mio flag a quelli che c'erano già (es. "esegui come amministratore").
			flags := []string{}
			if old, ok := regGet(registry.CURRENT_USER, layersKey, e); ok {
				for _, f := range strings.Fields(old) {
					if f != "~" && f != "DISABLEDXMAXIMIZEDWINDOWEDMODE" {
						flags = append(flags, f)
					}
				}
			}
			flags = append(flags, "DISABLEDXMAXIMIZEDWINDOWEDMODE")
			if regSet(registry.CURRENT_USER, layersKey, e, "~ "+strings.Join(flags, " ")) == nil {
				msgs = append(msgs, "✓ Ottimizzazioni a schermo intero spente: "+filepath.Base(e))
			}
		}
		if o.Priority {
			base := filepath.Base(e)
			k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, ifeoKey+base+`\PerfOptions`, registry.SET_VALUE|registry.QUERY_VALUE)
			if err == nil {
				if v, _, err2 := k.GetIntegerValue("CpuPriorityClass"); err2 == nil {
					prev["prio|"+base] = "1" + strconv.FormatUint(v, 10)
				} else {
					prev["prio|"+base] = "0"
				}
				if k.SetDWordValue("CpuPriorityClass", 6) == nil {
					msgs = append(msgs, "✓ Priorità del processore sopra il normale: "+base)
				}
				k.Close()
			}
		}
	}
	modeExes := []string{}
	if o.Mode != "" && len(exes) > 0 {
		for _, e := range exes {
			modeExes = append(modeExes, strings.ToLower(strings.TrimSuffix(filepath.Base(e), filepath.Ext(e))))
		}
		updateSettings(func(s *Settings) {
			if s.GameModes == nil {
				s.GameModes = map[string]string{}
			}
			for _, x := range modeExes {
				s.GameModes[x] = o.Mode
			}
			if !s.AutoGame {
				s.AutoGame, s.AutoGameTune = true, true
			}
		})
		msgs = append(msgs, "✓ Quando apri "+g.Name+" Tasso mette da solo la modalità "+modeName(o.Mode)+" (e la toglie quando chiudi). Ho acceso la Modalità gioco automatica.")
	}
	ini := ""
	if o.InGame {
		ini = unrealIni(g.dir)
		switch {
		case ini == "":
			msgs = append(msgs, "• Grafica dentro il gioco: questo gioco non usa un file di impostazioni che Tasso sa cambiare in sicurezza. Imposta i valori consigliati dal menu Opzioni del gioco (li trovi qui sopra).")
		case exeRunning(exes):
			msgs = append(msgs, "• Grafica dentro il gioco: chiudi "+g.Name+" e premi di nuovo Applica (con il gioco aperto le modifiche verrebbero perse).")
			ini = ""
		default:
			if err := writeUnrealIni(ini, o); err != nil {
				msgs = append(msgs, "• Grafica dentro il gioco: non sono riuscito a scrivere il file delle impostazioni.")
				ini = ""
			} else {
				cap := "nessun limite"
				if o.FPSCap > 0 {
					cap = strconv.Itoa(o.FPSCap) + " FPS"
				}
				vs := "spento"
				if o.VSync {
					vs = "acceso"
				}
				msgs = append(msgs, "✓ Grafica dentro il gioco: qualità "+qualityNames[o.Quality]+", VSync "+vs+", limite "+cap+". Copia di sicurezza del file salvata. Alcuni giochi ricalcolano la grafica al primo avvio: se succede, controlla nel menu.")
			}
		}
	}
	updateSettings(func(s *Settings) {
		if s.GameTunes == nil {
			s.GameTunes = map[string]*GameTune{}
		}
		s.GameTunes[id] = &GameTune{Name: g.Name, Preset: preset, Opts: o, Exes: exes, Prev: prev, Ini: ini, Applied: time.Now().Format("2006-01-02 15:04")}
	})
	msgs = append(msgs, "• Anti-Lag, Boost e sincronizzazione si impostano nel pannello della scheda video: premi \"Apri il pannello\" e metti i valori indicati.")
	emit("onGameTuneDone", map[string]interface{}{"id": id, "ok": true, "msgs": msgs, "settings": getSettings()})
}

// gameTuneUndoQuiet rimette com'era quello che Tasso aveva cambiato per questo gioco.
func gameTuneUndoQuiet(id string) bool {
	setMu.Lock()
	t := settings.GameTunes[id]
	setMu.Unlock()
	if t == nil {
		return false
	}
	for key, v := range t.Prev {
		parts := strings.SplitN(key, "|", 2)
		if len(parts) != 2 {
			continue
		}
		had, old := strings.HasPrefix(v, "1"), ""
		if len(v) > 0 {
			old = v[1:]
		}
		switch parts[0] {
		case "gpu":
			if had {
				_ = regSet(registry.CURRENT_USER, gpuPrefKey, parts[1], old)
			} else {
				regDel(registry.CURRENT_USER, gpuPrefKey, parts[1])
			}
		case "fso":
			if had {
				_ = regSet(registry.CURRENT_USER, layersKey, parts[1], old)
			} else {
				regDel(registry.CURRENT_USER, layersKey, parts[1])
			}
		case "prio":
			path := ifeoKey + parts[1] + `\PerfOptions`
			if had {
				if k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.SET_VALUE); err == nil {
					n, _ := strconv.ParseUint(old, 10, 32)
					_ = k.SetDWordValue("CpuPriorityClass", uint32(n))
					k.Close()
				}
			} else {
				regDel(registry.LOCAL_MACHINE, path, "CpuPriorityClass")
				// La chiave si cancella solo se è rimasta davvero vuota (altri programmi possono usarla).
				if k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE); err == nil {
					st, err := k.Stat()
					k.Close()
					if err == nil && st.ValueCount == 0 && st.SubKeyCount == 0 {
						_ = registry.DeleteKey(registry.LOCAL_MACHINE, path)
					}
				}
			}
		}
	}
	if t.Ini != "" {
		bak := t.Ini + ".tasso-backup"
		if b, err := os.ReadFile(bak); err == nil {
			if os.WriteFile(t.Ini, b, 0o644) == nil {
				_ = os.Remove(bak)
			}
		}
	}
	updateSettings(func(s *Settings) {
		for _, e := range t.Exes {
			delete(s.GameModes, strings.ToLower(strings.TrimSuffix(filepath.Base(e), filepath.Ext(e))))
		}
		delete(s.GameTunes, id)
		// Se la Modalità gioco automatica l'aveva accesa una regolazione, la spengo quando nessun gioco la usa più.
		if s.AutoGameTune {
			used := false
			for _, o := range s.GameTunes {
				if o != nil && o.Opts.Mode != "" {
					used = true
				}
			}
			if !used && !autoGameKept(s) {
				s.AutoGame, s.AutoGameTune = false, false
			}
		}
	})
	return true
}

func gameTuneUndo(id string) {
	msgs := []string{"Questo gioco non era regolato da Tasso."}
	if gameTuneUndoQuiet(id) {
		msgs = []string{"✓ Regolazione tolta: impostazioni di Windows e file della grafica rimessi com'erano."}
	}
	emit("onGameTuneDone", map[string]interface{}{"id": id, "ok": true, "undo": true, "msgs": msgs, "settings": getSettings()})
}

// gameModeFor: la modalità scelta per questo gioco (nome del programma senza .exe), se c'è.
func gameModeFor(exe string) string {
	setMu.Lock()
	defer setMu.Unlock()
	e := strings.ToLower(exe)
	if m := gamePowerFor(e); m != "" { // "Potenza per ogni gioco" vince su Regola con Tasso
		return m
	}
	return settings.GameModes[e]
}
