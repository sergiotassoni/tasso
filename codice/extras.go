package main

// Funzioni nuove della 2.1: salute del PC, internet (velocità, consumo, Wi-Fi),
// mappa del disco e modalità gioco automatica.

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ---------------- Salute del PC ----------------

func healthCheck() {
	r := runPS(healthScript, 3*time.Minute, nil)
	emit("onHealth", decodeJSON(r.JSON))
}

// ---------------- Internet ----------------

// speedTest misura ping, download e upload con i server di Cloudflare (gratuiti, senza account).
func speedTest() {
	res := map[string]interface{}{"ok": false}
	defer func() { emit("onSpeed", res) }()
	emit("onSpeedProgress", "Misuro il ping…")

	// Ping: tempo di connessione TCP, il migliore di 5 tentativi.
	best := time.Duration(0)
	for i := 0; i < 5; i++ {
		t := time.Now()
		c, err := net.DialTimeout("tcp", "speed.cloudflare.com:443", 3*time.Second)
		if err != nil {
			continue
		}
		d := time.Since(t)
		_ = c.Close()
		if best == 0 || d < best {
			best = d
		}
	}
	if best == 0 {
		res["error"] = "Non riesco a raggiungere internet. Controlla il cavo o il Wi-Fi e riprova."
		return
	}
	res["pingMs"] = best.Milliseconds()

	client := &http.Client{Timeout: 40 * time.Second, Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}}

	emit("onSpeedProgress", "Misuro la velocità in download…")
	down := measureDownload(client, 12*time.Second)
	res["downMbps"] = down

	emit("onSpeedProgress", "Misuro la velocità in upload…")
	up := measureUpload(client)
	res["upMbps"] = up

	res["ok"] = down > 0
	if down <= 0 {
		res["error"] = "Il test di download non è riuscito: riprova tra poco."
	}
}

// measureDownload scarica a ondate per al massimo maxDur e restituisce i Mbit/s medi.
func measureDownload(client *http.Client, maxDur time.Duration) float64 {
	ctx, cancel := context.WithTimeout(context.Background(), maxDur)
	defer cancel()
	var total int64
	start := time.Now()
	var mu sync.Mutex
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer guard("extras.go:83")
			defer wg.Done()
			buf := make([]byte, 64*1024)
			for ctx.Err() == nil {
				req, err := http.NewRequestWithContext(ctx, "GET", "https://speed.cloudflare.com/__down?bytes=25000000", nil)
				if err != nil {
					return
				}
				resp, err := client.Do(req)
				if err != nil {
					return
				}
				if resp.StatusCode != 200 {
					_ = resp.Body.Close()
					return
				}
				for {
					n, err := resp.Body.Read(buf)
					if n > 0 {
						mu.Lock()
						total += int64(n)
						mu.Unlock()
					}
					if err != nil {
						break
					}
				}
				_ = resp.Body.Close()
			}
		}()
	}
	wg.Wait()
	sec := time.Since(start).Seconds()
	if sec <= 0 || total < 1_000_000 {
		return 0
	}
	return float64(total) * 8 / sec / 1_000_000
}

// countingReader conta i byte man mano che partono, così anche una linea lenta dà un risultato.
type countingReader struct {
	r  io.Reader
	n  *int64
	mu *sync.Mutex
}

func (c countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.mu.Lock()
	*c.n += int64(n)
	c.mu.Unlock()
	return n, err
}

func measureUpload(client *http.Client) float64 {
	payload := make([]byte, 1024*1024)
	_, _ = rand.Read(payload)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var total int64
	var mu sync.Mutex
	start := time.Now()
	for ctx.Err() == nil {
		req, err := http.NewRequestWithContext(ctx, "POST", "https://speed.cloudflare.com/__up", countingReader{bytes.NewReader(payload), &total, &mu})
		if err != nil {
			break
		}
		req.ContentLength = int64(len(payload))
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := client.Do(req)
		if err != nil {
			break
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode >= 300 {
			break
		}
	}
	sec := time.Since(start).Seconds()
	mu.Lock()
	t := total
	mu.Unlock()
	if sec <= 0 || t < 100_000 {
		return 0
	}
	return float64(t) * 8 / sec / 1_000_000
}

func netUsage() {
	r := runPS(netUsageScript, 2*time.Minute, nil)
	emit("onNet", decodeJSON(r.JSON))
}

func wifiList() {
	r := runPSArgs(wifiScript, map[string]string{"base": scriptDir()}, 2*time.Minute, nil)
	emitRaw("onWifi", wifiRisultato(decodeJSON(r.JSON))) // nomi delle reti e password: mai tradotti
}

// wifiRisultato: l'elenco delle reti, oppure {error} se la lettura non è riuscita (prima un errore
// si vedeva come "Nessuna rete Wi-Fi salvata su questo PC").
func wifiRisultato(v interface{}) interface{} {
	switch x := v.(type) {
	case []interface{}:
		return x
	case map[string]interface{}:
		if _, ok := x["error"]; ok {
			return map[string]interface{}{"error": trGo("Non posso leggere le password del Wi-Fi in modo sicuro su questo PC (la cartella privata di Tasso non si crea): non le mostro.")}
		}
		return []interface{}{x} // una rete sola: PowerShell la scrive come oggetto singolo
	}
	return map[string]interface{}{"error": trGo("Non sono riuscito a leggere le reti Wi-Fi salvate: riprova tra poco.")}
}

// ---------------- Mappa del disco ----------------

func diskMap() {
	r := runPS(diskMapScript, 20*time.Minute, func(m string) { emit("onDiskMapProgress", m) })
	emit("onDiskMap", decodeJSON(r.JSON))
}

// ---------------- Modalità gioco automatica ----------------

// Cartelle tipiche dei giochi: se parte un .exe da qui, è un gioco.
var gameDirs = []string{
	`\steamapps\common\`, `\epic games\`, `\riot games\`, `\xboxgames\`, `\ea games\`,
	`\ubisoft game launcher\games\`, `\gog galaxy\games\`, `\battle.net\`, `\rockstar games\`,
	`\windowsapps\`, // solo se il nome è in knownGameExe
}

// Programmi da non considerare giochi anche se stanno in quelle cartelle.
// Cartelle che contengono servizi e programmi d'appoggio, non giochi.
var notGameDirs = []string{`\launcher\`, `\engine\binaries\`, `\__installer\`, `\wallpaper_engine\`, `\_commonredist\`, `\redist\`, `\directx\`, `\easyanticheat\`, `\battleye\`}

var notGameExe = map[string]bool{
	"unrealcefsubprocess.exe": true, "epiconlineservices.exe": true, "epiconlineserviceshost.exe": true, "epiconlineservicesuserhelper.exe": true,
	"rockstarservice.exe": true, "rockstarerrorhandler.exe": true, "playgtav.exe": true, "battle.net helper.exe": true, "blizzard battle.net.exe": true,
	"touchup.exe": true, "cleanup.exe": true, "dxsetup.exe": true, "vc_redist.x64.exe": true, "vc_redist.x86.exe": true, "uplaywebcore.exe": true,
	"ubisoftconnect.exe": true, "upc_uplay.exe": true, "eabackgroundservice.exe": true, "origin.exe": true, "gogalaxy.exe": true, "galaxyclient.exe": true,
	"wallpaper32.exe": true, "wallpaper64.exe": true, "webwallpaper32.exe": true,
	"steam.exe": true, "steamwebhelper.exe": true, "epicgameslauncher.exe": true, "epicwebhelper.exe": true,
	"riotclientservices.exe": true, "riotclientux.exe": true, "riotclientuxrender.exe": true, "riotclientcrashhandler.exe": true,
	"leagueclient.exe": true, "leagueclientux.exe": true, "leagueclientuxrender.exe": true, "battle.net.exe": true,
	"agent.exe": true, "eadesktop.exe": true, "upc.exe": true, "unitycrashhandler64.exe": true, "crashreportclient.exe": true,
	"easyanticheat.exe": true, "vanguard.exe": true, "vgc.exe": true, "socialclubhelper.exe": true, "launcher.exe": true,
}

var knownGameExe = map[string]bool{
	"minecraft.windows.exe": true, "forzahorizon5.exe": true, "forzahorizon4.exe": true, "halo infinite.exe": true,
}

var (
	gameMu      sync.Mutex
	gameActive  bool
	gameName    string
	gameMissing int
)

// gameTarget: la modalità che Tasso ha messo da solo all'apertura del gioco.
var gameTarget atomic.Value

// gameResumeDone: al primo giro dopo l'avvio controllo se era rimasta in sospeso una partita
// (PC spento in gioco): in quel caso rimetto la modalità di prima.
var gameResumeDone atomic.Bool

func startGameWatcher() {
	// Poco dopo l'avvio cerco i giochi in sottofondo (senza copertine e senza toccare la finestra):
	// così riconosco i giochi della libreria anche se non apri la pagina Giochi.
	go func() {
		defer guard("extras.go:startGameWatcher libreria")
		time.Sleep(20 * time.Second)
		scanLibraryQuiet()
	}()
	go func() {
		defer guard("extras.go:230")
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for range t.C {
			gameTick()
		}
	}()
}

func gameTick() {
	setMu.Lock()
	on := settings.AutoGame && !settings.Paused && settings.Profile != "ufficio" // in Ufficio niente modalità gioco automatica
	forAll := autoGameForAll(&settings)
	prev := settings.Mode
	left := settings.ModeBeforeGame
	setMu.Unlock()
	if !gameResumeDone.Swap(true) && left != "" && runningGame() == "" {
		endGameMode()
		return
	}
	if !on {
		// Automatico spento (o Tasso in pausa) a partita in corso: rimetto la modalità di prima.
		gameMu.Lock()
		wasActive := gameActive
		gameActive, gameMissing = false, 0
		gameMu.Unlock()
		if wasActive {
			endGameMode()
		}
		return
	}
	name := runningGame()
	gameMu.Lock()
	if name != "" {
		gameMissing = 0
		if gameActive && !strings.EqualFold(name, gameName) {
			// Hai cambiato gioco senza passare un minuto senza giochi: metto la modalità scelta per
			// il nuovo gioco. La modalità "di prima" resta quella di quando hai aperto il primo.
			gameName = name
			gameMu.Unlock()
			target, m := gameModeTarget(name)
			if m == "" {
				return // il nuovo gioco non ha una sua scelta: lascio quella che c'è
			}
			setMu.Lock()
			before := settings.ModeBeforeGame
			setMu.Unlock()
			if prev != target {
				back := before
				if back == "" { // il primo gioco non aveva cambiato niente: ricordo la modalità di adesso
					back = prev
					if _, ok := modeScripts[back]; !ok {
						back = "normale"
					}
				}
				updateSettings(func(s *Settings) { s.ModeBeforeGame, s.GameTarget = back, target })
				gameTarget.Store(target)
				go setMode(target)
			}
			emit("onGameMode", map[string]interface{}{"active": true, "game": name, "mode": target})
			return
		}
		if !gameActive {
			gameActive, gameName = true, name
			gameMu.Unlock()
			// Modalità scelta per questo gioco (Potenza per ogni gioco o Regola con Tasso), altrimenti Gaming.
			target, m := gameModeTarget(name)
			// Con una scelta per questo gioco la metto sempre; senza, non abbasso FPS a Gaming, e non
			// cambio niente se la modalità automatica è accesa solo per i giochi con una loro potenza.
			if (m != "" && prev != target) || (m == "" && forAll && prev != "gaming" && prev != "fps") {
				back := prev
				if _, ok := modeScripts[back]; !ok {
					back = "normale" // es. dopo "Rimetti com'era": non è una modalità da rimettere
				}
				updateSettings(func(s *Settings) { s.ModeBeforeGame, s.GameTarget = back, target })
				gameTarget.Store(target)
				go setMode(target)
			}
			overlayGameStarted()
			if m != "" || forAll { // senza una scelta per questo gioco non ho cambiato niente: niente avviso
				emit("onGameMode", map[string]interface{}{"active": true, "game": name, "mode": target})
			}
			return
		}
		gameMu.Unlock()
		return
	}
	if gameActive {
		gameMissing++
		if gameMissing >= 6 { // un minuto senza giochi aperti
			gameActive = false
			gameMu.Unlock()
			endGameMode()
			return
		}
	}
	gameMu.Unlock()
}

// gameModeTarget: la modalità da mettere per questo gioco e la scelta salvata per lui ("" = nessuna).
func gameModeTarget(name string) (target, m string) {
	target = "gaming"
	m = gameModeFor(name)
	if m == "fps" || m == "gaming" {
		target = m
	}
	if m == "max" { // Massima potenza: FPS competitivo
		target = "fps"
	}
	return target, m
}

// endGameMode rimette la modalità di prima, ma solo se nel frattempo non l'hai cambiata tu.
func endGameMode() {
	setMu.Lock()
	back := settings.ModeBeforeGame
	cur := settings.Mode
	saved := settings.GameTarget
	setMu.Unlock()
	restored := ""
	if back != "" {
		updateSettings(func(s *Settings) { s.ModeBeforeGame, s.GameTarget = "", "" })
		t, _ := gameTarget.Load().(string)
		if t == "" {
			t = saved
		}
		if cur == "gaming" || (t != "" && cur == t) {
			restored = back
			go setMode(back)
		}
		gameTarget.Store("")
	}
	overlayGameEnded()
	gameMu.Lock()
	g := gameName
	gameMu.Unlock()
	emit("onGameMode", map[string]interface{}{"active": false, "game": g, "back": restored})
}

func setAutoGame(on bool) string {
	updateSettings(func(s *Settings) { s.AutoGame, s.AutoGameTune = on, false })
	return "ok"
}

// runningGame restituisce il nome del primo gioco aperto, oppure "".
// gamePid: il processo del gioco aperto (serve al pannellino per sapere se il gioco è davanti).
var gamePid atomic.Uint32

func runningGame() string {
	withWin := pidsWithBigWindow()
	libDirs, libExes := libraryGamePaths()
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		exe := strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))
		if notGameExe[exe] || !strings.HasSuffix(exe, ".exe") || !withWin[pe.ProcessID] {
			continue
		}
		path := strings.ToLower(processPath(pe.ProcessID))
		if path == "" {
			continue
		}
		if isGamePath(path, exe, libDirs, libExes) {
			gamePid.Store(pe.ProcessID)
			return strings.TrimSuffix(windows.UTF16ToString(pe.ExeFile[:]), ".exe")
		}
	}
	return ""
}

// isGamePath: il programma (percorso e nome in minuscolo) è un gioco? Sì se sta in una cartella
// tipica dei giochi, oppure nella cartella di un gioco della libreria di Tasso (Battle.net in
// Program Files, C:\Games, GOG, giochi aggiunti a mano...). Mai nelle cartelle di servizio.
func isGamePath(path, exe string, libDirs []string, libExes map[string]bool) bool {
	for _, nd := range notGameDirs {
		if strings.Contains(path, nd) {
			return false
		}
	}
	if libExes[path] {
		return true
	}
	for _, d := range libDirs {
		if strings.HasPrefix(path, d) {
			return true
		}
	}
	for _, d := range gameDirs {
		if strings.Contains(path, d) {
			return d != `\windowsapps\` || knownGameExe[exe]
		}
	}
	return false
}

// libraryGamePaths: le cartelle (in minuscolo, con \ finale) e i programmi dei giochi della
// libreria. Le cartelle troppo larghe (Desktop, Download, Program Files...) non contano: lì
// conta solo il programma esatto del gioco. Per i giochi aggiunti a mano conta sempre e solo il
// programma esatto: la loro cartella può contenere altri programmi normali.
func libraryGamePaths() ([]string, map[string]bool) {
	var dirs []string
	exes := map[string]bool{}
	libMu.Lock()
	defer libMu.Unlock()
	for _, g := range library {
		if g.Hidden {
			continue
		}
		if g.exe != "" && !notGameExe[strings.ToLower(filepath.Base(g.exe))] {
			exes[strings.ToLower(filepath.Clean(g.exe))] = true
		}
		if g.dir == "" || strings.HasPrefix(g.ID, "man:") {
			continue
		}
		d := strings.ToLower(strings.TrimRight(filepath.Clean(g.dir), `\`))
		if len(d) <= 3 || !filepath.IsAbs(d) || isBroadDir(d) {
			continue
		}
		dirs = append(dirs, d+`\`)
	}
	return dirs, exes
}

func processPath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

var (
	pEnumWindows       = user32.NewProc("EnumWindows")
	pGetWindowPid      = user32.NewProc("GetWindowThreadProcessId")
	pIsWindowVisible   = user32.NewProc("IsWindowVisible")
	pIsIconic          = user32.NewProc("IsIconic")
	pGetWindowRect     = user32.NewProc("GetWindowRect")
	enumMu             sync.Mutex
	enumPids           map[uint32]bool
	enumMinW, enumMinH int32
	enumCallback       = windows.NewCallback(enumWinProc)
)

type rect struct{ Left, Top, Right, Bottom int32 }

func enumWinProc(hwnd uintptr, _ uintptr) uintptr {
	if v, _, _ := pIsWindowVisible.Call(hwnd); v == 0 {
		return 1
	}
	// Gioco ridotto a icona (sei andato su Chrome un attimo): conta lo stesso come aperto.
	if ic, _, _ := pIsIconic.Call(hwnd); ic != 0 {
		var pid uint32
		pGetWindowPid.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid != 0 {
			enumPids[pid] = true
		}
		return 1
	}
	var r rect
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	if r.Right-r.Left < enumMinW || r.Bottom-r.Top < enumMinH {
		return 1
	}
	var pid uint32
	pGetWindowPid.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid != 0 {
		enumPids[pid] = true
	}
	return 1
}

// pidsWithBigWindow: i processi che hanno una finestra visibile grande (un gioco ce l'ha,
// i servizi e gli helper in background no).
func pidsWithBigWindow() map[uint32]bool { return pidsWithWindow(640, 400) }

func pidsWithWindow(minW, minH int32) map[uint32]bool {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumPids = map[uint32]bool{}
	enumMinW, enumMinH = minW, minH
	pEnumWindows.Call(enumCallback, 0)
	return enumPids
}
