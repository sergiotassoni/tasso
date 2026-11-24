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
	"bytes"
	"encoding/json"
	"errors"
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

	"tasso/traduce"

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
	"prestazionale": "Prestazionale: simulazione, sopravvivenza, strategia",
}

// kindUnknownLabel: gioco che Tasso non riconosce. Usa la regolazione "prestazionale", che nei
// pulsanti si chiama "Bilanciato": stesso nome, così la scritta e il pulsante "Automatico (Bilanciato)" tornano.
const kindUnknownLabel = "Bilanciato, tipo non riconosciuto"

// steamGenreIT: i generi più comuni del negozio di Steam (che li dà in inglese) detti in italiano.
// Quelli che non sono qui non si mostrano a chi usa Tasso in italiano.
var steamGenreIT = map[string]string{
	"action":                "azione",
	"adventure":             "avventura",
	"casual":                "casual",
	"indie":                 "indie",
	"massively multiplayer": "multigiocatore di massa",
	"racing":                "corse",
	"rpg":                   "gioco di ruolo",
	"simulation":            "simulazione",
	"sports":                "sport",
	"strategy":              "strategia",
	"early access":          "accesso anticipato",
	"free to play":          "gratuito",
}

// steamGenresShown: i generi di Steam da mostrare, nella lingua di Tasso.
func steamGenresShown(genres []string) string {
	var out []string
	for _, g := range genres {
		if isEN() {
			out = append(out, g)
		} else if it, ok := steamGenreIT[g]; ok {
			out = append(out, it)
		}
	}
	return strings.Join(out, ", ")
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
			if list := steamGenresShown(genres); list != "" {
				return "prestazionale", "dal negozio di Steam: " + list
			}
			return "prestazionale", "dal negozio di Steam"
		}
	}
	return "prestazionale", "" // tipo non riconosciuto: vedi kindUnknownLabel
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

// screenHzNow: la frequenza dello schermo adesso (quello principale, o il più veloce), 0 se non si sa.
// Gli Hz della scheda video in hwCache si leggono una volta sola: se nel frattempo hai cambiato
// schermo o frequenza, il limite FPS consigliato sarebbe sbagliato.
func screenHzNow() int {
	hz := 0
	for _, sc := range screens() {
		if sc.Primary && sc.Hz > 1 {
			return sc.Hz
		}
		if sc.Hz > hz {
			hz = sc.Hz
		}
	}
	if hz <= 1 { // 0 o 1 = "frequenza predefinita dell'hardware": non si sa
		return 0
	}
	return hz
}

func pcTier() (int, string, int) {
	hwOnce.Do(loadHwCache)
	gpu := ""
	hz := screenHzNow()
	cacheHz := 0
	for _, g := range hwCache.GPUs {
		if gpu == "" || gpuTier(g.Name) > gpuTier(gpu) {
			gpu = g.Name
		}
		if g.Hz > cacheHz {
			cacheHz = g.Hz
		}
	}
	if hz == 0 {
		hz = cacheHz
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
			why += ", poca RAM: un gradino sotto"
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
		r.Opts = tuneOpts{GpuHigh: true, Mode: "gaming", InGame: true, Quality: q, VSync: false, FPSCap: hz}
		r.Graphics = [][]string{
			{"Qualità generale", qualityNames[q]},
			{"Upscaling", "FSR / DLSS / XeSS " + up[tier]},
			{"VSync", "Spento nel gioco, con FreeSync / G-Sync acceso (se il monitor non li ha: VSync acceso)"},
			{"Limite FPS", strconv.Itoa(hz) + " (come il monitor: niente lavoro sprecato, PC più fresco)"},
			{"Ray tracing", map[int]string{0: "Spento", 1: "Spento", 2: "Basso o spento", 3: "Medio/Alto"}[tier]},
			{"Generazione di fotogrammi", "Solo se fai già almeno 60 FPS"},
		}
	default:
		preset = "prestazionale"
		r.Preset = preset
		r.Label = "Bilanciato: stabile e senza scatti"
		q := []int{0, 1, 2, 2}[tier]
		r.Opts = tuneOpts{GpuHigh: true, Mode: "gaming", InGame: true, Quality: q, VSync: false, FPSCap: hz}
		upTxt := "spento (risoluzione nativa)"
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

// Programmi da saltare sempre: installatori e programmi di servizio, riconosciuti dall'INIZIO del
// nome (o dal nome intero), non da una parola in mezzo. Prima una parola qualsiasi bastava:
// CrashBandicoot.exe ("crash") e Dispatch.exe ("patch") venivano scartati.
var skipExe = regexp.MustCompile(`(?i)^(unins|uninst|setup|install|redist|vcredist|vc_redist|dxsetup|dxweb|directx|dotnet|ndp\d|oalinst|physx|prereq|ue4prereq|ue5prereq|easyanticheat|eac(launcher|setup)?\.exe$|battleye|beservice|unitycrashhandler|crashreportclient|crashhandler|crashpad|crashsender|start_protected_game|bugsplat|sentry|steamwebhelper|cefsharp|unrealcefsubprocess|quicksfv|dxwebsetup|epiconlineservices)`)

// toolWords: parole tipiche dei programmi d'appoggio. Non scartano il programma: gli tolgono punti,
// così vince il gioco vero, ma se c'è solo lui (es. CrashBandicoot.exe) resta.
var toolWords = []string{"crash", "report", "launcher", "helper", "update", "patch", "config", "settings", "overlay", "benchmark", "uploader", "cleanup", "cef", "webhelper", "server", "editor", "tool", "anticheat"}

// setupWords: installatori e disinstallatori con il nome in mezzo (UplayInstaller.exe, GameSetup.exe,
// Gioco_uninstall.exe, Touchup.exe). Tolgono più punti: restano sotto zero anche se il nome
// somiglia alla cartella del gioco.
var setupWords = []string{"install", "setup", "uninst", "redist", "touchup"}

// gameExes trova i programmi veri del gioco (al massimo 3), saltando installatori e programmi di servizio.
// I giochi Unity hanno un avviatore piccolo (circa 650 KB) con accanto UnityPlayer.dll o la cartella
// <nome>_Data: li accetto anche se sono piccoli, come quelli con il nome simile alla cartella del
// gioco. Gli altri programmi sotto 1 MB li prendo solo se nella cartella non c'è nient'altro.
func gameExes(dir string) []string {
	if dir == "" || !dirExists(dir) {
		return nil
	}
	type cand struct {
		path  string
		size  int64
		score int
		ok    bool // grande, avviatore Unity o con il nome simile alla cartella
	}
	var list []cand
	folder := normTitle(filepath.Base(filepath.Clean(dir)))
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
			if ln == "redist" || ln == "_commonredist" || ln == "directx" || ln == "support" || ln == "easyanticheat" || ln == "battleye" ||
				ln == "__installer" || ln == "_redist" || ln == "prerequisites" { // cartelle degli installatori
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !strings.EqualFold(filepath.Ext(name), ".exe") || skipExe.MatchString(name) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() < 64<<10 {
			return nil
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		ls := strings.ToLower(stem)
		pdir := filepath.Dir(p)
		unity := fileExists(filepath.Join(pdir, "UnityPlayer.dll")) || dirExists(filepath.Join(pdir, stem+"_Data"))
		sc := 0
		similar := false
		if unity {
			sc += 4
		}
		lp := strings.ToLower(p)
		if strings.Contains(lp, `\binaries\win64\`) {
			sc += 2
		}
		if strings.Contains(ls, "shipping") {
			sc += 3
		}
		// nome simile alla cartella del gioco (CrashBandicoot.exe in "Crash Bandicoot N. Sane Trilogy")
		if n := normTitle(stem); len(n) >= 4 && len(folder) >= 4 && (strings.Contains(folder, n) || strings.Contains(n, folder)) {
			sc += 6
			similar = true
		}
		for _, w := range toolWords {
			if strings.Contains(ls, w) {
				sc -= 5
				similar = false // un programma d'appoggio piccolo non conta come "buono" per il nome
				break
			}
		}
		for _, w := range setupWords {
			if strings.Contains(ls, w) {
				sc -= 10
				similar = false
				break
			}
		}
		list = append(list, cand{p, info.Size(), sc, unity || similar || info.Size() >= 1<<20})
		return nil
	})
	// I programmi piccoli (non Unity e con un nome diverso dalla cartella) contano solo se non c'è
	// nessun altro candidato.
	good := list[:0:0]
	for _, c := range list {
		if c.ok {
			good = append(good, c)
		}
	}
	if len(good) > 0 {
		list = good
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		return list[i].size > list[j].size
	})
	// Prima i programmi senza punti in meno; quelli "d'appoggio" solo se non c'è altro (ne basta uno).
	var out []string
	for i := 0; i < len(list) && len(out) < 3; i++ {
		if list[i].score >= 0 {
			out = append(out, list[i].path)
		}
	}
	if len(out) == 0 && len(list) > 0 {
		out = append(out, list[0].path)
	}
	return out
}

// gameExesFor: i programmi del gioco nella sua cartella; se non ne trovo, uso il programma che
// Tasso già conosce per quel gioco (es. aggiunto a mano o trovato da GOG), ma solo se sta dentro
// la cartella del gioco.
func gameExesFor(g gameEntry) []string {
	if exes := gameExes(g.dir); len(exes) > 0 {
		return exes
	}
	if g.exe == "" || g.dir == "" || !strings.EqualFold(filepath.Ext(g.exe), ".exe") || !fileExists(g.exe) {
		return nil
	}
	d := strings.ToLower(strings.TrimRight(filepath.Clean(g.dir), `\`)) + `\`
	if strings.HasPrefix(strings.ToLower(filepath.Clean(g.exe)), d) {
		return []string{g.exe}
	}
	return nil
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
// errIniUTF16: il file è scritto in UTF-16 (alcuni giochi lo fanno): non lo tocco, per non rovinarlo.
var errIniUTF16 = errors.New("file UTF-16")

func writeUnrealIni(path string, o tuneOpts) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(b) >= 2 && ((b[0] == 0xFF && b[1] == 0xFE) || (b[0] == 0xFE && b[1] == 0xFF)) || bytes.IndexByte(b, 0) >= 0 {
		return errIniUTF16
	}
	bom := bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}) // UTF-8 con BOM: lo tolgo e lo rimetto alla fine
	bak := path + ".tasso-backup"
	if !fileExists(bak) {
		if err := os.WriteFile(bak, b, 0o644); err != nil {
			return err
		}
	}
	if bom {
		b = b[3:]
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
	if bom {
		res = "\xEF\xBB\xBF" + res
	}
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
	kLabel := kindLabel[kind]
	if kindWhy == "" {
		kLabel = kindUnknownLabel
	}
	tier, tierWhy, hz := pcTier()
	vendor := gpuVendor()
	recs := map[string]tuneRec{}
	for _, p := range []string{"competitivo", "grafico", "prestazionale"} {
		recs[p] = recommend(p, tier, hz, vendor)
	}
	exes := gameExesFor(g)
	ini := unrealIni(g.dir)
	// Giochi con anti-cheat: Tasso non tocca i loro file né le impostazioni (niente .ini, niente
	// priorità del processore). La grafica la imposti tu dal menu del gioco.
	ac := gameAntiCheat(g)
	if ac {
		ini = ""
		for p, r := range recs {
			r.Opts.InGame, r.Opts.Priority = false, false
			recs[p] = r
		}
	}
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
		"id": id, "name": g.Name, "kind": kind, "kindLabel": kLabel, "kindWhy": kindWhy,
		"tier": tier, "tierLabel": tierLabel[tier], "tierWhy": tierWhy, "hz": hz, "vendor": vendor,
		"recs": recs, "exes": names, "hasExe": len(exes) > 0, "ini": ini != "", "current": cur,
		"autoGame": autoGame, "dir": g.dir != "", "anticheat": ac,
	})
}

// gameAntiCheat: il gioco ha un anti-cheat (guarda solo i nomi di file e cartelle).
func gameAntiCheat(g gameEntry) bool {
	return traduce.ConAntiCheat(traduce.GiocoRiconosciuto{Nome: g.Name, Piattaforma: g.Store, Cartella: g.dir})
}

// gpuPrefOrig: il valore di prima della scheda video per questo programma, se Tasso l'ha già
// cambiato da un'altra parte (Boost o Regola con Tasso). Così il valore originale si ricorda una
// volta sola e l'annullamento rimette quello vero, in qualunque ordine si annulli.
// Va chiamata senza setMu e bMu presi.
func gpuPrefOrig(path string) (string, bool) {
	setMu.Lock()
	for _, t := range settings.GameTunes {
		if t == nil {
			continue
		}
		for k, v := range t.Prev {
			if strings.HasPrefix(k, "gpu|") && strings.EqualFold(k[4:], path) {
				setMu.Unlock()
				return v, true
			}
		}
	}
	setMu.Unlock()
	bMu.Lock()
	defer bMu.Unlock()
	for k, v := range loadBoost().GpuPrev {
		if strings.EqualFold(k, path) {
			return v, true
		}
	}
	return "", false
}

// gpuPrefUndo rimette il valore di prima ("1"+valore, oppure "0" = non c'era), ma solo se adesso
// c'è ancora la scheda potente messa da Tasso: se l'hai cambiata tu, o l'ha già rimessa un'altra
// parte di Tasso, non la tocco.
func gpuPrefUndo(path, prev string) {
	if prev == "" {
		return
	}
	if cur, ok := regGet(registry.CURRENT_USER, gpuPrefKey, path); !ok || !strings.Contains(cur, "GpuPreference=2") {
		return
	}
	if prev[0] == '1' {
		_ = regSet(registry.CURRENT_USER, gpuPrefKey, path, prev[1:])
	} else {
		regDel(registry.CURRENT_USER, gpuPrefKey, path)
	}
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
	exes := gameExesFor(g)
	var msgs, failed []string
	fail := func(m string) { msgs, failed = append(msgs, m), append(failed, m) }
	// Giochi con anti-cheat: niente file del gioco e niente priorità del processore.
	ac := gameAntiCheat(g)
	if ac && (o.InGame || o.Priority) {
		o.InGame, o.Priority = false, false
		msgs = append(msgs, "• Questo gioco ha un anti-cheat: Tasso non tocca i suoi file né la priorità del processore. La grafica la imposti tu dal menu del gioco.")
	}
	// Se c'era già una regolazione, riparto dai valori originali del registro. Il file della grafica
	// no: la sua copia di sicurezza resta quella della prima volta.
	oldIni := gameTuneUndoQuiet(id, true)
	prev := map[string]string{}
	remember := func(key string, root registry.Key, path, name string) {
		if v, ok := regGet(root, path, name); ok {
			prev[key] = "1" + v
		} else {
			prev[key] = "0"
		}
	}
	if len(exes) == 0 && (o.GpuHigh || o.Priority || o.NoFSO || o.Mode != "") {
		fail("Non ho trovato il programma del gioco nella sua cartella: le impostazioni di Windows per questo gioco non si possono applicare.")
	}
	for _, e := range exes {
		if o.GpuHigh {
			if v, ok := gpuPrefOrig(e); ok { // l'aveva già cambiata il Boost: mi tengo il valore vero di prima
				prev["gpu|"+e] = v
			} else {
				remember("gpu|"+e, registry.CURRENT_USER, gpuPrefKey, e)
			}
			if regSet(registry.CURRENT_USER, gpuPrefKey, e, "GpuPreference=2;SwapEffectUpgradeEnable=1;") == nil {
				msgs = append(msgs, "✓ Scheda video ad alte prestazioni e ottimizzazioni per i giochi in finestra: "+filepath.Base(e))
			} else {
				fail("✗ Windows non mi ha lasciato cambiare la scheda video per " + filepath.Base(e) + ".")
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
			} else {
				fail("✗ Windows non mi ha lasciato spegnere le ottimizzazioni a schermo intero per " + filepath.Base(e) + ".")
			}
		}
		if o.Priority {
			base := filepath.Base(e)
			if !ifeoNameOK(base) {
				// La priorità di Windows vale per il NOME del programma, in tutto il PC: con un nome
				// generico (Game.exe, nw.exe...) cambierebbe anche altri programmi.
				fail("• Priorità del processore: il programma si chiama " + base + ", un nome troppo comune (lo usano anche altri programmi): Tasso non la cambia.")
				continue
			}
			// Ricordo se la chiave del programma l'ho creata io: all'annullamento la tolgo se è vuota.
			if _, done := prev["ifeo|"+base]; !done {
				if ek, err := registry.OpenKey(registry.LOCAL_MACHINE, ifeoKey+base, registry.QUERY_VALUE); err == nil {
					ek.Close()
					prev["ifeo|"+base] = "1"
				} else {
					prev["ifeo|"+base] = "0"
				}
			}
			k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, ifeoKey+base+`\PerfOptions`, registry.SET_VALUE|registry.QUERY_VALUE)
			if err == nil {
				if _, done := prev["prio|"+base]; done {
					// Stesso nome già visto (due exe del gioco con lo stesso nome): tengo il valore vero di prima.
				} else if v, _, err2 := k.GetIntegerValue("CpuPriorityClass"); err2 == nil {
					prev["prio|"+base] = "1" + strconv.FormatUint(v, 10)
				} else {
					prev["prio|"+base] = "0"
				}
				if k.SetDWordValue("CpuPriorityClass", 6) == nil {
					msgs = append(msgs, "✓ Priorità del processore sopra il normale: "+base)
				} else {
					fail("✗ Windows non mi ha lasciato cambiare la priorità del processore per " + base + ".")
				}
				k.Close()
			} else {
				fail("✗ Windows non mi ha lasciato cambiare la priorità del processore per " + base + ".")
			}
		}
	}
	modeExes := []string{}
	if o.Mode != "" && len(exes) > 0 {
		for _, e := range exes {
			modeExes = append(modeExes, strings.ToLower(strings.TrimSuffix(filepath.Base(e), filepath.Ext(e))))
		}
		turnedOn := false
		updateSettings(func(s *Settings) {
			if s.GameModes == nil {
				s.GameModes = map[string]string{}
			}
			for _, x := range modeExes {
				s.GameModes[x] = o.Mode
			}
			if !s.AutoGame {
				s.AutoGame, s.AutoGameTune = true, true
				turnedOn = true
			}
		})
		m := "✓ Quando apri " + g.Name + " Tasso mette da solo la modalità " + modeName(o.Mode) + " (e la toglie quando chiudi)."
		if turnedOn {
			m += " Ho acceso la Modalità gioco automatica."
		}
		msgs = append(msgs, m)
	}
	// ini: il file che "Annulla regolazione" rimette com'era. Se questa volta non lo scrivo, tengo
	// quello della regolazione di prima (la sua copia di sicurezza è ancora quella originale).
	ini := ""
	if o.InGame {
		ini = unrealIni(g.dir)
		switch {
		case ini == "":
			fail("• Grafica dentro il gioco: questo gioco non usa un file di impostazioni che Tasso sa cambiare in sicurezza. Imposta i valori consigliati dal menu Opzioni del gioco (li trovi qui sopra).")
		case exeRunning(exes):
			fail("• Grafica dentro il gioco: chiudi " + g.Name + " e premi di nuovo Applica (con il gioco aperto le modifiche verrebbero perse).")
			ini = ""
		default:
			if err := writeUnrealIni(ini, o); errors.Is(err, errIniUTF16) {
				fail("• Grafica dentro il gioco: il file delle impostazioni di questo gioco è in un formato (UTF-16) che Tasso non cambia, per non rovinarlo. Imposta i valori consigliati dal menu Opzioni del gioco.")
				ini = ""
			} else if err != nil {
				fail("• Grafica dentro il gioco: non sono riuscito a scrivere il file delle impostazioni.")
				ini = ""
			} else {
				cap := "nessun limite FPS"
				if o.FPSCap > 0 {
					cap = "limite " + strconv.Itoa(o.FPSCap) + " FPS"
				}
				vs := "VSync spento"
				if o.VSync {
					vs = "VSync acceso"
				}
				msgs = append(msgs, "✓ Grafica dentro il gioco: "+qualityText[o.Quality]+", "+vs+", "+cap+". Copia di sicurezza del file salvata. Alcuni giochi ricalcolano la grafica al primo avvio: se succede, controlla nel menu.")
			}
		}
	}
	if !o.InGame && oldIni != "" {
		// Questa volta "Grafica dentro il gioco" è tolta: rimetto il file com'era prima di Tasso.
		switch {
		case !fileExists(oldIni + ".tasso-backup"): // nessuna copia: non c'è niente da rimettere
			oldIni = ""
		case exeRunning(exes):
			fail("• Grafica dentro il gioco: chiudi " + g.Name + " e premi di nuovo Applica per rimetterla com'era (con il gioco aperto verrebbe persa).")
		case restoreIniBackup(oldIni):
			msgs = append(msgs, "✓ Grafica dentro il gioco rimessa com'era.")
			oldIni = ""
		default:
			fail("• Grafica dentro il gioco: non sono riuscito a rimettere com'era il file delle impostazioni.")
		}
	}
	if ini == "" {
		ini = oldIni
	}
	updateSettings(func(s *Settings) {
		if s.GameTunes == nil {
			s.GameTunes = map[string]*GameTune{}
		}
		s.GameTunes[id] = &GameTune{Name: g.Name, Preset: preset, Opts: o, Exes: exes, Prev: prev, Ini: ini, Applied: time.Now().Format("2006-01-02 15:04")}
		autoGameTuneCleanup(s)
	})
	msgs = append(msgs, "• Anti-Lag, Boost e sincronizzazione si impostano nel pannello della scheda video: premi \"Apri il pannello\" e metti i valori indicati.")
	// partial: qualcosa che hai chiesto non è stato fatto (le righe in failed).
	emit("onGameTuneDone", map[string]interface{}{"id": id, "ok": true, "partial": len(failed) > 0, "failed": failed, "msgs": msgs, "settings": getSettings()})
}

// deleteKeyIfEmpty cancella una chiave di HKLM solo se non ha più né valori né sottochiavi.
func deleteKeyIfEmpty(path string) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	st, err := k.Stat()
	k.Close()
	if err == nil && st.ValueCount == 0 && st.SubKeyCount == 0 {
		_ = registry.DeleteKey(registry.LOCAL_MACHINE, path)
	}
}

// genericExe: nomi di programmi usati da tanti giochi e programmi diversi (motori, avviatori).
var genericExe = map[string]bool{
	"game": true, "nw": true, "main": true, "start": true, "launch": true, "launcher": true, "play": true,
	"client": true, "app": true, "application": true, "run": true, "runtime": true, "player": true,
	"java": true, "javaw": true, "python": true, "pythonw": true, "node": true, "electron": true,
	"godot": true, "love": true, "dosbox": true, "scummvm": true, "retroarch": true, "server": true,
	"bootstrap": true, "game64": true, "gamex64": true, "game_x64": true, "win64": true, "x64": true,
	"unity": true, "unityplayer": true, "renpy": true, "lib": true, "bin": true, "setup": true,
}

// ifeoNameOK: il nome del programma è abbastanza particolare per cambiargli la priorità
// (vale per tutti i programmi con quel nome): almeno 5 lettere, non generico, niente percorsi.
func ifeoNameOK(base string) bool {
	if base == "" || strings.ContainsAny(base, `\/`) || !strings.EqualFold(filepath.Ext(base), ".exe") {
		return false
	}
	stem := strings.ToLower(strings.TrimSuffix(base, filepath.Ext(base)))
	return len([]rune(stem)) >= 5 && !genericExe[stem]
}

// qualityText: "qualità Alto" ecc., intere, così si traducono anche in inglese.
var qualityText = []string{"qualità Basso", "qualità Medio", "qualità Alto", "qualità Ultra"}

// gameTuneUndoQuiet rimette com'era quello che Tasso aveva cambiato per questo gioco.
// Con reapply (stai premendo di nuovo Applica) rimette solo il registro: il file della grafica e
// la sua copia di sicurezza della prima volta restano come sono, e restituisce il percorso del
// file da tenere per "Annulla regolazione". Senza reapply restituisce "x" se c'era una regolazione.
func gameTuneUndoQuiet(id string, reapply bool) string {
	setMu.Lock()
	t := settings.GameTunes[id]
	setMu.Unlock()
	if t == nil {
		return ""
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
			gpuPrefUndo(parts[1], v)
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
				deleteKeyIfEmpty(path)
			}
			// IFEO\<nome>.exe: la tolgo solo se l'aveva creata Tasso ed è rimasta vuota.
			if t.Prev["ifeo|"+parts[1]] == "0" && !strings.ContainsAny(parts[1], `\/`) {
				deleteKeyIfEmpty(ifeoKey + parts[1])
			}
		}
	}
	// 3.1: il percorso viene da settings.json, che un programma qualsiasi può cambiare: rimetto il backup
	// solo sul file delle impostazioni del gioco, e mai attraverso collegamenti o giunzioni.
	if reapply {
		updateSettings(func(s *Settings) {
			for _, e := range t.Exes {
				delete(s.GameModes, strings.ToLower(strings.TrimSuffix(filepath.Base(e), filepath.Ext(e))))
			}
			// la regolazione resta, senza i valori del registro (già rimessi): la riscrive Applica
			if cur := s.GameTunes[id]; cur != nil {
				c := *cur
				c.Prev = nil
				s.GameTunes[id] = &c
			}
		})
		if t.Ini != "" && safeIniPath(t.Ini) {
			return t.Ini
		}
		return ""
	}
	if t.Ini != "" {
		restoreIniBackup(t.Ini)
	}
	updateSettings(func(s *Settings) {
		for _, e := range t.Exes {
			delete(s.GameModes, strings.ToLower(strings.TrimSuffix(filepath.Base(e), filepath.Ext(e))))
		}
		delete(s.GameTunes, id)
		autoGameTuneCleanup(s)
	})
	return "x"
}

// autoGameTuneCleanup: se la Modalità gioco automatica l'aveva accesa una regolazione o una potenza
// per gioco, la spengo quando nessuno la usa più. Va chiamata dentro updateSettings.
func autoGameTuneCleanup(s *Settings) {
	if !s.AutoGameTune || autoGameKept(s) {
		return
	}
	for _, o := range s.GameTunes {
		if o != nil && o.Opts.Mode != "" {
			return
		}
	}
	s.AutoGame, s.AutoGameTune = false, false
}

// restoreIniBackup rimette il file della grafica dalla sua copia di sicurezza e la cancella.
// 3.1: il percorso viene da settings.json, che un programma qualsiasi può cambiare: solo sul file
// delle impostazioni del gioco, e mai attraverso collegamenti o giunzioni.
func restoreIniBackup(path string) bool {
	if !safeIniPath(path) {
		return false
	}
	bak := path + ".tasso-backup"
	b, err := os.ReadFile(bak)
	if err != nil {
		return false
	}
	if os.WriteFile(path, b, 0o644) != nil {
		return false
	}
	_ = os.Remove(bak)
	return true
}

// gameTuneBusy: il gioco regolato è aperto e ha un file della grafica rimesso da Tasso. Se lo
// rimettessi adesso, alla chiusura il gioco lo riscriverebbe e la copia di prima andrebbe persa.
func gameTuneBusy(id string) bool {
	setMu.Lock()
	t := settings.GameTunes[id]
	setMu.Unlock()
	return t != nil && t.Ini != "" && exeRunning(t.Exes)
}

func gameTuneUndo(id string) {
	msgs := []string{"Questo gioco non era regolato da Tasso."}
	if gameTuneBusy(id) {
		msgs = []string{"Il gioco è aperto: chiudilo e riprova, così le sue impostazioni tornano com'erano davvero."}
		emit("onGameTuneDone", map[string]interface{}{"id": id, "ok": false, "undo": true, "msgs": msgs, "settings": getSettings()})
		return
	}
	if gameTuneUndoQuiet(id, false) != "" {
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

// safeIniPath: solo GameUserSettings.ini, percorso assoluto, e nessun collegamento/giunzione
// né sul file, né sul backup, né sulle cartelle che li contengono.
func safeIniPath(p string) bool {
	if !filepath.IsAbs(p) || !strings.EqualFold(filepath.Base(p), "GameUserSettings.ini") {
		return false
	}
	isLink := func(q string) bool {
		fi, err := os.Lstat(q)
		if err != nil {
			return false // non esiste: va bene, non è un collegamento
		}
		return fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0
	}
	if isLink(p) || isLink(p+".tasso-backup") {
		return false
	}
	for d := filepath.Dir(p); ; d = filepath.Dir(d) {
		if isLink(d) {
			return false
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	return true
}
