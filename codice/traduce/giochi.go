package traduce

// Riconoscimento dei giochi dalla cartella in cui sono installati: Steam, Epic, GOG, Xbox/
// Microsoft Store, EA, Ubisoft, Battle.net, Rockstar, itch.io, Amazon. Si leggono solo i
// percorsi e, per Steam, il file appmanifest_*.acf di Steam (per il nome vero del gioco).
// Nessun file del gioco viene aperto o toccato, niente internet.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type GiocoRiconosciuto struct {
	Nome        string // "Path of Titans"
	Piattaforma string // "Steam"
	Cartella    string // cartella di installazione del gioco
}

var negozi = []struct {
	piattaforma string
	segno       string // pezzo del percorso, in minuscolo con le barre "\"
}{
	{"Steam", `\steamapps\common\`},
	{"Epic Games", `\epic games\`},
	{"GOG", `\gog galaxy\games\`},
	{"GOG", `\gog games\`},
	{"Xbox", `\xboxgames\`},
	{"Microsoft Store", `\windowsapps\`},
	{"EA", `\ea games\`},
	{"EA", `\origin games\`},
	{"Ubisoft", `\ubisoft game launcher\games\`},
	{"Battle.net", `\battle.net\games\`},
	{"Rockstar", `\rockstar games\`},
	{"itch.io", `\itch\apps\`},
	{"Amazon", `\amazon games\library\`},
	{"Riot", `\riot games\`},
	{"Bethesda", `\bethesda.net launcher\games\`},
}

// launcher e programmi dei negozi stessi: non sono giochi
var nonGiochiNegozi = map[string]bool{
	"steam.exe": true, "steamwebhelper.exe": true, "epicgameslauncher.exe": true, "galaxyclient.exe": true,
	"eadesktop.exe": true, "origin.exe": true, "upc.exe": true, "ubisoftconnect.exe": true,
	"battle.net.exe": true, "rockstarlauncher.exe": true, "itch.exe": true, "amazon games ui.exe": true,
	"unitycrashhandler64.exe": true, "riotclientservices.exe": true, "riotclientux.exe": true, "vgc.exe": true, "vgtray.exe": true, "crashreportclient.exe": true, "easyanticheat.exe": true,
}

// cartelle dentro le librerie dei negozi che non sono giochi
var cartelleNonGiochi = map[string]bool{
	"steamworks shared": true, "steam controller configs": true, "directx": true, "_commonredist": true,
}

// riconosciGioco: dal percorso completo del .exe capisce se è un gioco installato da un negozio.
var (
	muGiochiVisti sync.Mutex
	giochiVisti   = map[string]giocoVisto{}
)

type giocoVisto struct {
	g  GiocoRiconosciuto
	ok bool
}

// riconosciGioco: come riconosciGiocoDaCapo, ma ricorda il risultato per ogni exe (si chiama
// più volte al secondo mentre si gioca: niente letture continue dei file di Steam).
func riconosciGioco(percorso string) (GiocoRiconosciuto, bool) {
	muGiochiVisti.Lock()
	v, ok := giochiVisti[percorso]
	muGiochiVisti.Unlock()
	if ok {
		return v.g, v.ok
	}
	g, si := riconosciGiocoDaCapo(percorso)
	muGiochiVisti.Lock()
	if len(giochiVisti) > 500 {
		giochiVisti = map[string]giocoVisto{}
	}
	giochiVisti[percorso] = giocoVisto{g, si}
	muGiochiVisti.Unlock()
	return g, si
}

func riconosciGiocoDaCapo(percorso string) (GiocoRiconosciuto, bool) {
	p := strings.ReplaceAll(percorso, "/", `\`)
	basso := strings.ToLower(p)
	if nonGiochiNegozi[strings.ToLower(filepath.Base(strings.ReplaceAll(p, `\`, "/")))] {
		return GiocoRiconosciuto{}, false
	}
	for _, n := range negozi {
		i := strings.Index(basso, n.segno)
		if i < 0 {
			continue
		}
		resto := p[i+len(n.segno):]
		cartella, _, ok := strings.Cut(resto, `\`)
		if !ok || cartella == "" { // l'exe deve stare dentro la cartella di un gioco
			continue
		}
		if cartelleNonGiochi[strings.ToLower(cartella)] { // installatori e redistribuibili dei negozi
			return GiocoRiconosciuto{}, false
		}
		g := GiocoRiconosciuto{Nome: nomeLeggibile(cartella), Piattaforma: n.piattaforma, Cartella: p[:i+len(n.segno)] + cartella}
		if n.piattaforma == "Microsoft Store" {
			g.Nome = nomePacchetto(cartella)
			if g.Nome == "" {
				continue
			}
		}
		if n.piattaforma == "Steam" {
			if nome := nomeSteam(p[:i+len(`\steamapps\`)], cartella); nome != "" {
				g.Nome = nome
			}
		}
		return g, true
	}
	return GiocoRiconosciuto{}, false
}

// nomeLeggibile: "Hollow_Knight" -> "Hollow Knight"
func nomeLeggibile(c string) string {
	return strings.TrimSpace(strings.NewReplacer("_", " ", "  ", " ").Replace(c))
}

// nomePacchetto: "Microsoft.MinecraftUWP_1.20..._x64__8wekyb3d8bbwe" -> "MinecraftUWP".
// Solo i pacchetti di giochi noti a Windows (Xbox) passano: gli altri sono app normali.
func nomePacchetto(c string) string {
	nome, _, _ := strings.Cut(c, "_")
	l := strings.ToLower(nome)
	if !strings.Contains(l, "game") && !strings.Contains(l, "xbox") && !strings.Contains(l, "minecraft") {
		return ""
	}
	if i := strings.LastIndex(nome, "."); i >= 0 {
		nome = nome[i+1:]
	}
	return nome
}

var (
	reAcfDir  = regexp.MustCompile(`(?i)"installdir"\s+"([^"]+)"`)
	reAcfNome = regexp.MustCompile(`(?i)"name"\s+"([^"]+)"`)
)

// nomeSteam: legge i file appmanifest_*.acf di Steam (non del gioco) per il nome ufficiale.
func nomeSteam(dirSteamapps, cartella string) string {
	dir := strings.ReplaceAll(dirSteamapps, `\`, string(os.PathSeparator))
	file, _ := filepath.Glob(filepath.Join(dir, "appmanifest_*.acf"))
	for _, f := range file {
		st, err := os.Stat(f)
		if err != nil || st.Size() > 1<<20 {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		d := reAcfDir.FindSubmatch(b)
		n := reAcfNome.FindSubmatch(b)
		if d != nil && n != nil && strings.EqualFold(string(d[1]), cartella) {
			return string(n[1])
		}
	}
	return ""
}

// GiocoInstallato: un gioco trovato nelle librerie dei negozi sul PC.
type GiocoInstallato struct {
	Nome        string `json:"nome"`
	Piattaforma string `json:"piattaforma"`
	Cartella    string `json:"cartella"`
	Copertina   string `json:"-"` // file immagine già presente sul PC (cache di Steam)
	Avvio       string `json:"-"` // indirizzo per avviarlo dal suo negozio (steam://…), se c'è
}

var reAcfID = regexp.MustCompile(`(?i)"appid"\s+"(\d+)"`)

// copertinaSteam: Steam tiene già sul PC le copertine dei giochi della libreria
// (appcache\librarycache). Le uso così come sono, niente internet.
func copertinaSteam(dirSteam, appid string) string {
	base := filepath.Join(dirSteam, "appcache", "librarycache")
	for _, nome := range []string{"library_600x900.jpg", "library_capsule.jpg", "header.jpg", "library_header.jpg"} {
		// formato vecchio: 123_library_600x900.jpg
		if f := filepath.Join(base, appid+"_"+nome); esisteFile(f) {
			return f
		}
		// formato nuovo: 123\library_600x900.jpg oppure 123\<codice>\library_600x900.jpg
		if f := filepath.Join(base, appid, nome); esisteFile(f) {
			return f
		}
		if m, _ := filepath.Glob(filepath.Join(base, appid, "*", nome)); len(m) > 0 {
			return m[0]
		}
	}
	return ""
}

func esisteFile(f string) bool {
	st, err := os.Stat(f)
	return err == nil && !st.IsDir() && st.Size() < 4<<20
}

var reVdfPath = regexp.MustCompile(`(?i)"path"\s+"([^"]+)"`)

// giochiSteam: legge libraryfolders.vdf e gli appmanifest di ogni libreria di Steam.
func giochiSteam(dirSteam string) []GiocoInstallato {
	librerie := []string{dirSteam}
	if b, err := os.ReadFile(filepath.Join(dirSteam, "steamapps", "libraryfolders.vdf")); err == nil && len(b) < 1<<20 {
		for _, m := range reVdfPath.FindAllSubmatch(b, -1) {
			librerie = append(librerie, strings.ReplaceAll(string(m[1]), `\\`, `\`))
		}
	}
	visti := map[string]bool{}
	var out []GiocoInstallato
	for _, lib := range librerie {
		lib = strings.ReplaceAll(lib, `\`, string(os.PathSeparator))
		file, _ := filepath.Glob(filepath.Join(lib, "steamapps", "appmanifest_*.acf"))
		for _, f := range file {
			b, err := os.ReadFile(f)
			if err != nil || len(b) > 1<<20 {
				continue
			}
			d, n := reAcfDir.FindSubmatch(b), reAcfNome.FindSubmatch(b)
			if d == nil || n == nil || nonGioco(string(n[1])) {
				continue
			}
			c := filepath.Join(lib, "steamapps", "common", string(d[1]))
			if visti[strings.ToLower(c)] {
				continue
			}
			visti[strings.ToLower(c)] = true
			cop, avvio := "", ""
			if id := reAcfID.FindSubmatch(b); id != nil {
				cop = copertinaSteam(dirSteam, string(id[1]))
				avvio = "steam://rungameid/" + string(id[1])
			}
			out = append(out, GiocoInstallato{string(n[1]), "Steam", c, cop, avvio})
		}
	}
	return out
}

// strumenti di Steam che non sono giochi
func nonGioco(nome string) bool {
	l := strings.ToLower(nome)
	return strings.Contains(l, "redistributable") || strings.Contains(l, "steamworks") ||
		strings.Contains(l, "proton") || strings.Contains(l, "steam linux runtime") || strings.HasSuffix(l, " sdk") ||
		strings.Contains(l, "dedicated server") || strings.Contains(l, "soundtrack")
}

var (
	reEpicNome = regexp.MustCompile(`"DisplayName"\s*:\s*"([^"]+)"`)
	reEpicApp  = regexp.MustCompile(`"AppName"\s*:\s*"([A-Za-z0-9._-]+)"`)
	reEpicDir  = regexp.MustCompile(`"InstallLocation"\s*:\s*"([^"]+)"`)
)

// giochiEpic: i file .item dei giochi installati con Epic Games Launcher.
func giochiEpic(dirManifesti string) []GiocoInstallato {
	file, _ := filepath.Glob(filepath.Join(dirManifesti, "*.item"))
	var out []GiocoInstallato
	for _, f := range file {
		b, err := os.ReadFile(f)
		if err != nil || len(b) > 1<<20 {
			continue
		}
		n, d := reEpicNome.FindSubmatch(b), reEpicDir.FindSubmatch(b)
		if n == nil || d == nil {
			continue
		}
		avvio := ""
		if a := reEpicApp.FindSubmatch(b); a != nil {
			avvio = "com.epicgames.launcher://apps/" + string(a[1]) + "?action=launch&silent=true"
		}
		out = append(out, GiocoInstallato{string(n[1]), "Epic Games", strings.ReplaceAll(string(d[1]), `\\`, `\`), "", avvio})
	}
	return out
}

// ConAntiCheat: il gioco ha un sistema anti-imbroglio (EasyAntiCheat, BattlEye, Vanguard di
// Riot…). Guardo solo i nomi di file e cartelle, senza aprire niente. Questi giochi non li
// traduco in automatico (potrebbero non gradire una finestra sopra il gioco) e Tasso non tocca
// i loro file né le loro impostazioni.
func ConAntiCheat(g GiocoRiconosciuto) bool {
	if g.Piattaforma == "Riot" {
		return true
	}
	base := strings.ReplaceAll(g.Cartella, `\`, string(os.PathSeparator))
	if base == "" {
		return false
	}
	// Il risultato si ricorda per qualche minuto: il controllo legge fino a 300 cartelle
	// e durante il gioco viene chiesto spesso (anche ogni secondo).
	acMu.Lock()
	if r, ok := acCache[base+"|"+g.Nome]; ok && time.Since(r.quando) < 5*time.Minute {
		acMu.Unlock()
		return r.si
	}
	acMu.Unlock()
	si := conAntiCheatCartella(base, g.Nome)
	acMu.Lock()
	if len(acCache) > 500 {
		acCache = map[string]acRisultato{}
	}
	acCache[base+"|"+g.Nome] = acRisultato{si: si, quando: time.Now()}
	acMu.Unlock()
	return si
}

type acRisultato struct {
	si     bool
	quando time.Time
}

var (
	acMu    sync.Mutex
	acCache = map[string]acRisultato{}
)

func conAntiCheatCartella(base, nome string) bool {
	for _, d := range [][]string{{"EasyAntiCheat"}, {"EasyAntiCheat_EOS"}, {"BattlEye"}, {"Binaries", "Win64", "EasyAntiCheat"}} {
		if st, err := os.Stat(filepath.Join(append([]string{base}, d...)...)); err == nil && st.IsDir() {
			return true
		}
	}
	// altri segni: lanciatori protetti e driver anti-imbroglio accanto all'exe
	voci, _ := os.ReadDir(base)
	for _, v := range voci {
		if segnoAntiCheat(v.Name(), segniAntiCheat) {
			return true
		}
	}
	// Sottocartelle tipiche (Binaries\Win64\EasyAntiCheat, Gioco\Binaries\Win64\BattlEye…), fino a
	// 3 livelli sotto la cartella del gioco, solo con i nomi più sicuri. Per non diventare lenta con i
	// giochi enormi leggo al massimo 300 cartelle: prima quelle con nomi utili (Binaries, Win64, il
	// nome del gioco…), e salto quelle note pesanti e senza programmi (Engine, Content, Paks…).
	type dir struct {
		path  string
		depth int
	}
	nomeGioco := nomePiatto(nome)
	var prima, dopo []dir
	aggiungi := func(path, nome string, depth int) {
		n := strings.ToLower(nome)
		if cartellePesanti[n] {
			return
		}
		f := nomePiatto(n)
		if cartelleUtili[n] || (len(f) >= 3 && nomeGioco != "" && (strings.Contains(nomeGioco, f) || strings.Contains(f, nomeGioco))) {
			prima = append(prima, dir{path, depth})
		} else {
			dopo = append(dopo, dir{path, depth})
		}
	}
	for _, v := range voci {
		if v.IsDir() {
			aggiungi(filepath.Join(base, v.Name()), v.Name(), 1)
		}
	}
	for lette := 0; (len(prima) > 0 || len(dopo) > 0) && lette < 300; lette++ {
		var d dir
		if len(prima) > 0 {
			d, prima = prima[0], prima[1:]
		} else {
			d, dopo = dopo[0], dopo[1:]
		}
		sotto, err := os.ReadDir(d.path)
		if err != nil {
			continue
		}
		for _, v := range sotto {
			if segnoAntiCheat(v.Name(), segniAntiCheatSicuri) {
				return true
			}
			if v.IsDir() && d.depth < 3 {
				aggiungi(filepath.Join(d.path, v.Name()), v.Name(), d.depth+1)
			}
		}
	}
	return false
}

var (
	segniAntiCheat       = []string{"start_protected_game", "easyanticheat", "battleye", "beservice", "mhyprot", "xigncode", "gameguard", "npgamemon", "vgk", "faceit", "ace-", "anticheat"}
	segniAntiCheatSicuri = []string{"start_protected_game", "easyanticheat", "battleye", "beservice", "eaanticheat"}
)

// Cartelle da guardare per prime e cartelle da saltare (grandi e senza programmi).
var (
	cartelleUtili   = map[string]bool{"binaries": true, "win64": true, "win32": true, "bin": true, "x64": true, "easyanticheat": true, "battleye": true, "game": true}
	cartellePesanti = map[string]bool{"engine": true, "content": true, "paks": true, "movies": true, "localization": true, "_commonredist": true, "redist": true, "directx": true, "saved": true, "logs": true}
)

// nomePiatto: il nome in minuscolo, senza spazi e segni ("Dead by Daylight" -> "deadbydaylight").
func nomePiatto(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func segnoAntiCheat(nome string, segni []string) bool {
	n := strings.ToLower(nome)
	for _, segno := range segni {
		if strings.Contains(n, segno) {
			return true
		}
	}
	return false
}
