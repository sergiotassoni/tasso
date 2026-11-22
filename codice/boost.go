package main

// BOOST (3.1): un solo pulsante che prepara il PC per giocare, misura gli FPS veri e rimette
// tutto com'era. Usa regolazioni che il Tasso sapeva già fare e le mette insieme. Non tocca i file
// del gioco né la sua memoria.
//
// Onestà (dopo la revisione con 4 revisori + scettico):
//   - Le regolazioni di Windows di solito danno da 0 al 10% di FPS in più; aiutano soprattutto sui
//     PC con tanti programmi aperti e sui portatili. I salti enormi delle pubblicità (92 -> 218)
//     vengono quasi sempre da grafica diversa, DLSS/FSR o generazione di fotogrammi.
//   - Rete, priorità ai giochi e ottimizzazioni in finestra valgono dal prossimo avvio del gioco
//     (la priorità a volte dal riavvio del PC): per questo il Boost le lascia accese finché non premi
//     "Rimetti com'era", invece di accenderle e spegnerle a ogni partita.
//   - Niente svuotamento della memoria in standby durante il gioco: toglierebbe al gioco i suoi file
//     già in memoria e farebbe più scatti, non meno.
//   - La misura FPS usa PresentMon di Intel (gratuito, licenza MIT, firmato da Intel): legge gli
//     eventi di Windows sui fotogrammi, come i contatori FPS, senza entrare nel gioco. Il confronto
//     prima/dopo dice "simile" quando la differenza è nella variazione normale.
//
// Boost automatico ("capacità di adattamento"): la modalità giusta per QUEL gioco la mette la
// modalità gioco automatica (Gaming, FPS, Potenza per ogni gioco, Regola con Tasso), che il Boost
// automatico accende. In più a ogni partita il Boost chiude i programmi di sincronizzazione e chat in
// background e li riapre un minuto dopo la fine. Con programmi normali non fa niente.

import (
	"bufio"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

//go:embed strumenti/PresentMon.exe
var presentMonExe []byte

// SHA-256 di PresentMon 2.4.1 x64 (release ufficiale Intel su GitHub).
const presentMonSHA = "d74183e7ae630f72cd3690be0373ecbfdc6cbb86578148aab8fa2a7166068f34"

// Comandi gaming del Boost (accesi solo se erano spenti, rispenti con "Rimetti com'era").
// Niente pianificazione GPU (vuole il riavvio e su alcuni PC peggiora).
var boostCmds = []string{"gamedvr", "mmcss", "nagle", "winopt"}

// Programmi che il Boost chiude, solo se sono in background (senza finestra) e non sono app dello
// Store: sincronizzazione e chat. Mai Office, Outlook, Acrobat o programmi con lavoro non salvato.
var boostClosable = map[string]bool{
	"onedrive.exe": true, "dropbox.exe": true, "googledrivefs.exe": true, "icloudservices.exe": true,
	"iclouddrive.exe": true, "teams.exe": true, "zoom.exe": true, "skype.exe": true,
}

type fpsResult struct {
	Game      string  `json:"game"`
	When      string  `json:"when"`
	Boost     bool    `json:"boost"`
	Seconds   float64 `json:"seconds"`
	Frames    int     `json:"frames"`
	Avg       float64 `json:"avg"`       // FPS medi
	Low1      float64 `json:"low1"`      // 1% low: media del centesimo di fotogrammi più lenti
	Stutter   int     `json:"stutter"`   // scatti: fotogrammi lunghi più del doppio del normale
	StutterPM float64 `json:"stutterPm"` // scatti al minuto (per confrontare misure di durata diversa)
	GpuBusy   float64 `json:"gpuBusy"`   // % del tempo di ogni fotogramma in cui la scheda video lavora (alto = limite della scheda video)
	CpuTemp   float64 `json:"cpuTemp,omitempty"`
	GpuTemp   float64 `json:"gpuTemp,omitempty"`
	Throttle  bool    `json:"throttle,omitempty"` // la scheda video NVIDIA ha rallentato per il caldo
	Hz        int     `json:"hz,omitempty"`       // frequenza dello schermo durante la misura
	Spread    float64 `json:"spread,omitempty"`   // differenza % tra fotogrammi veloci e lenti (bassa = FPS fermi su un limite)
	VramPct   float64 `json:"vramPct,omitempty"`  // memoria della scheda video usata (%)
	pid       string
	path      string
}

type boostState struct {
	On       bool                    `json:"on"`       // Boost acceso a mano
	Auto     bool                    `json:"auto"`     // Boost automatico acceso
	Cmds     []string                `json:"cmds"`     // comandi accesi dal Boost (da rispegnere)
	PrevMode string                  `json:"prevMode"` // modalità da rimettere ("" = non l'ha cambiata il Boost)
	PrevAG   *bool                   `json:"prevAutoGame,omitempty"`
	GpuPrev  map[string]string       `json:"gpuPrev,omitempty"` // gioco -> scelta della scheda video di prima ("0" = nessuna)
	Mouse    *[3]int32               `json:"mouse,omitempty"`   // accelerazione del mouse di prima (solo se l'ha tolta il Boost)
	History  map[string][]*fpsResult `json:"history,omitempty"` // per gioco, ultime misure
}

var (
	pMessageBeep    = user32.NewProc("MessageBeep")
	pForegroundWin  = user32.NewProc("GetForegroundWindow")
	pGetPowerStatus = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemPowerStatus")

	bMu     sync.Mutex // protegge bState, bClosed, bSkip, bGame
	bState  *boostState
	bOpMu   sync.Mutex // un'accensione/spegnimento alla volta, dall'inizio alla fine
	fpsMu   sync.Mutex // una misura alla volta
	bWatch  sync.Once
	bClosed []string // programmi chiusi da riaprire (solo in memoria: mai letti da un file)
	bSkip   string   // gioco per cui hai premuto "Rimetti com'era": niente Boost automatico finché resta aperto
	bGame   string   // gioco della partita col Boost automatico in corso
)

func boostFile() string { return filepath.Join(appDir, "boost.json") }

// safeName: i nomi dei giochi arrivano da fuori (nome del programma): tengo solo caratteri semplici.
func safeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= 60 {
			break
		}
		if r == ' ' || r == '.' || r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// loadBoost legge lo stato salvato. boost.json si può modificare senza essere amministratore:
// tengo solo valori ammessi. Chiamare con bMu preso.
func loadBoost() *boostState {
	if bState != nil {
		return bState
	}
	raw := &boostState{}
	if b, err := os.ReadFile(boostFile()); err == nil {
		_ = json.Unmarshal(b, raw)
	}
	s := &boostState{On: raw.On, Auto: raw.Auto, PrevAG: raw.PrevAG, History: map[string][]*fpsResult{}}
	for _, c := range raw.Cmds {
		if hasStr(boostCmds, c) && !hasStr(s.Cmds, c) {
			s.Cmds = append(s.Cmds, c)
		}
	}
	for k, v := range raw.GpuPrev {
		if strings.HasSuffix(strings.ToLower(k), ".exe") && len(k) < 400 && len(v) < 300 && (v == "0" || strings.HasPrefix(v, "1")) {
			if s.GpuPrev == nil {
				s.GpuPrev = map[string]string{}
			}
			s.GpuPrev[k] = v
		}
	}
	if _, ok := modeScripts[raw.PrevMode]; ok {
		s.PrevMode = raw.PrevMode
	}
	for k, v := range raw.History {
		if k == "" || k != strings.ToLower(safeName(k)) || len(v) > 10 {
			continue
		}
		for _, r := range v {
			if r != nil {
				r.Game = safeName(r.Game)
				s.History[k] = append(s.History[k], r)
			}
		}
	}
	bState = s
	return s
}

func saveBoost() {
	if bState == nil {
		return
	}
	if b, err := json.MarshalIndent(bState, "", "  "); err == nil {
		_ = os.WriteFile(boostFile(), b, 0o644)
	}
}

func boostInfo() map[string]interface{} {
	bMu.Lock()
	defer bMu.Unlock()
	s := loadBoost()
	return map[string]interface{}{"on": s.On, "auto": s.Auto, "game": bGame, "battery": onBattery(), "gpuFix": len(s.GpuPrev) > 0}
}

// onBattery: portatile senza caricatore. In quel caso niente piano energetico al massimo.
func onBattery() bool {
	var st struct {
		ACLineStatus, BatteryFlag, BatteryLifePercent, SystemStatusFlag byte
		BatteryLifeTime, BatteryFullLifeTime                            uint32
	}
	r, _, _ := pGetPowerStatus.Call(uintptr(unsafe.Pointer(&st)))
	return r != 0 && st.ACLineStatus == 0
}

func removeStr(list []string, v string) []string {
	var out []string
	for _, x := range list {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

// ---------------- Parti del Boost ----------------

// boostCmdsOn accende i comandi spenti e si ricorda quali. Salvo PRIMA di cambiare: se il Tasso si
// chiude a metà, "Rimetti com'era" sa comunque cosa rispegnere.
func boostCmdsOn() int {
	st := gamingState()
	n := 0
	for _, id := range boostCmds {
		if on, _ := st[id].(bool); on {
			continue
		}
		if id == "winopt" && winBuild() < 22000 {
			continue
		}
		bMu.Lock()
		s := loadBoost()
		if !hasStr(s.Cmds, id) {
			s.Cmds = append(s.Cmds, id)
		}
		saveBoost()
		bMu.Unlock()
		if strings.HasPrefix(gamingSet(id, true), "ok:") {
			n++
		} else {
			bMu.Lock()
			s.Cmds = removeStr(s.Cmds, id)
			saveBoost()
			bMu.Unlock()
		}
	}
	return n
}

// boostCmdsOff rispegne solo i comandi accesi dal Boost. Se uno non si spegne resta nell'elenco,
// così si può riprovare.
func boostCmdsOff() int {
	bMu.Lock()
	cmds := append([]string(nil), loadBoost().Cmds...)
	bMu.Unlock()
	setMu.Lock()
	active := map[string]bool{}
	for k, v := range settings.GamingCmds {
		active[k] = v
	}
	setMu.Unlock()
	n := 0
	for _, id := range cmds {
		ok := true
		if active[id] { // se "Rimetti com'era" generale l'ha già spento, non lo tocco
			ok = strings.HasPrefix(gamingSet(id, false), "ok:")
		}
		if ok {
			n++
			bMu.Lock()
			s := loadBoost()
			s.Cmds = removeStr(s.Cmds, id)
			saveBoost()
			bMu.Unlock()
		}
	}
	return n
}

// closeBackground chiude con gentilezza (mai di forza) i programmi di sincronizzazione e chat che
// girano senza finestra. Restituisce i nomi chiusi.
func closeBackground() []string {
	withWin := pidsWithWindow(1, 1)
	keep := boostKeepSet()
	var names []string
	for exe := range boostClosable {
		if keep[exe] {
			continue // "Non chiudere mai anche": scelto da te
		}
		pids := pidsByExe(exe)
		if len(pids) == 0 {
			continue
		}
		path, skip := "", false
		for _, p := range pids {
			if withWin[p] {
				skip = true
			}
			if pp := processPath(p); pp != "" {
				path = pp
			}
		}
		if skip || path == "" || strings.Contains(strings.ToLower(path), `\windowsapps\`) {
			continue
		}
		_, _ = runHidden("taskkill.exe", "/IM", exe) // senza /F: chiusura normale, finiscono di salvare
		for i := 0; i < 16 && len(pidsByExe(exe)) > 0; i++ {
			time.Sleep(500 * time.Millisecond)
		}
		if len(pidsByExe(exe)) == 0 {
			bMu.Lock()
			bClosed = append(bClosed, path)
			bMu.Unlock()
			names = append(names, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
		}
	}
	return names
}

// reopenBackground riapre i programmi chiusi (se non sono già ripartiti da soli), come utente
// normale tramite Esplora file.
func reopenBackground() []string {
	bMu.Lock()
	list := bClosed
	bClosed = nil
	bMu.Unlock()
	win, err := windows.GetSystemWindowsDirectory()
	if err != nil {
		return nil
	}
	explorer := filepath.Join(win, "explorer.exe")
	var names []string
	for _, p := range list {
		if len(pidsByExe(filepath.Base(p))) > 0 || !fileExists(p) {
			continue
		}
		if openPath(explorer, "\""+p+"\"") {
			names = append(names, strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)))
		}
	}
	return names
}

// ---------------- Accendi / spegni (pulsante) ----------------

func boostOn() []string {
	msgs, _ := boostOnOK()
	return msgs
}

// boostOnOK: come boostOn, e dice anche se il Boost è stato acceso adesso (false = rifiutato:
// Tasso spento o Boost già acceso).
func boostOnOK() ([]string, bool) {
	bOpMu.Lock()
	defer bOpMu.Unlock()
	setMu.Lock()
	paused, mode := settings.Paused, settings.Mode
	setMu.Unlock()
	if paused {
		return []string{"Tasso è spento: riaccendilo per usare il Boost."}, false
	}
	bMu.Lock()
	s := loadBoost()
	if s.On || bGame != "" { // acceso a mano o dal Boost automatico durante una partita
		bMu.Unlock()
		return []string{"Il Boost è già acceso."}, false
	}
	s.On = true
	saveBoost()
	bMu.Unlock()

	var msgs []string
	if n := boostCmdsOn(); n > 0 {
		msgs = append(msgs, "Accese le regolazioni per giocare (registrazione in background spenta, priorità ai giochi, rete, giochi in finestra). Valgono dal prossimo avvio del gioco.")
	}
	if onBattery() {
		msgs = append(msgs, "Sei a batteria: lascio il risparmio energetico. Collega il caricatore per avere tutta la potenza.")
	} else if mode != "gaming" && mode != "fps" {
		prev := mode
		if _, ok := modeScripts[prev]; !ok {
			prev = "normale"
		}
		bMu.Lock()
		s.PrevMode = prev
		saveBoost()
		bMu.Unlock()
		setMode("gaming")
		msgs = append(msgs, "Modalità Gaming: processore al massimo, notifiche in pausa.")
	}
	if names := closeBackground(); len(names) > 0 {
		msgs = append(msgs, "Chiusi in background: "+strings.Join(names, ", ")+". Li riapro quando premi \"Rimetti com'era\".")
	}
	if m := mouseAccelOff(safeName(runningGame())); m != "" {
		msgs = append(msgs, m)
		startBoostWatcher() // serve per rimettere il mouse quando chiudi il gioco
	}
	if len(msgs) == 0 {
		msgs = append(msgs, "Il PC era già pronto per giocare: non c'era niente da cambiare.")
	}
	return msgs, true
}

// restoreBoostMode rimette la modalità di prima, se l'ha cambiata il Boost e non l'hai cambiata tu.
func restoreBoostMode(prev string) bool {
	if prev == "" {
		return false
	}
	setMu.Lock()
	cur, target, before := settings.Mode, settings.GameTarget, settings.ModeBeforeGame
	setMu.Unlock()
	// Partita in corso con la modalità gioco automatica: a fine gioco deve tornare la TUA modalità,
	// non "gaming" messa dal Boost.
	if before == "gaming" || before == "fps" {
		updateSettings(func(st *Settings) { st.ModeBeforeGame = prev })
	}
	if cur == "gaming" || (target != "" && cur == target) {
		setMode(prev)
		return true
	}
	return false
}

// boostOff ("Rimetti com'era"): spegne il Boost e anche il Boost automatico, e rimette solo
// quello che il Boost ha cambiato.
func boostOff(explicit bool) []string {
	bOpMu.Lock()
	defer bOpMu.Unlock()
	bMu.Lock()
	s := loadBoost()
	prev, auto, prevAG := s.PrevMode, s.Auto, s.PrevAG
	var gpuPrev map[string]string
	if explicit { // la scheda video per i giochi la rimette solo il pulsante "Rimetti com'era"
		gpuPrev = s.GpuPrev
		s.GpuPrev = nil
	}
	s.On, s.Auto, s.PrevMode, s.PrevAG = false, false, "", nil
	if bGame != "" {
		bSkip = bGame
	}
	bGame = ""
	saveBoost()
	bMu.Unlock()

	var msgs []string
	if boostCmdsOff() > 0 {
		msgs = append(msgs, "Regolazioni per giocare rimesse come prima.")
	}
	if len(gpuPrev) > 0 {
		gpuPrefRestore(gpuPrev)
		msgs = append(msgs, "Scelta della scheda video per i giochi rimessa com'era.")
	}
	if restoreBoostMode(prev) {
		msgs = append(msgs, "Rimessa la modalità di prima.")
	}
	if auto && prevAG != nil && !*prevAG {
		setAutoGame(false)
	}
	if names := reopenBackground(); len(names) > 0 {
		msgs = append(msgs, "Riaperti: "+strings.Join(names, ", ")+".")
	}
	if m := mouseAccelRestore(); m != "" {
		msgs = append(msgs, m)
	}
	bMu.Lock()
	left := len(loadBoost().Cmds)
	bMu.Unlock()
	if left > 0 {
		msgs = append(msgs, "Qualcosa non si è potuto rimettere: premi di nuovo \"Rimetti com'era\".")
	} else {
		msgs = append(msgs, "Tutto com'era prima del Boost.")
	}
	return msgs
}

// ---------------- Boost automatico ----------------

func setBoostAuto(on bool) string {
	if !on {
		bMu.Lock()
		manual := loadBoost().On
		bMu.Unlock()
		if !manual {
			go func() {
				defer guard("boost.go:autooff")
				emit("onBoost", map[string]interface{}{"msgs": boostOff(false), "state": boostInfo()})
			}()
			return "ok:Boost automatico spento: rimetto tutto com'era."
		}
		bMu.Lock()
		s := loadBoost()
		s.Auto = false
		saveBoost()
		bMu.Unlock()
		return "ok:Boost automatico spento. Il Boost acceso a mano resta acceso."
	}
	setMu.Lock()
	ag := settings.AutoGame
	setMu.Unlock()
	bMu.Lock()
	s := loadBoost()
	if !s.Auto {
		s.PrevAG = &ag
	}
	s.Auto = true
	saveBoost()
	bMu.Unlock()
	if !ag {
		setAutoGame(true) // sceglie la modalità giusta per ogni gioco
	}
	go func() {
		defer guard("boost.go:autoon")
		bOpMu.Lock()
		boostCmdsOn() // una volta sola, restano accese: valgono dal prossimo avvio del gioco
		bOpMu.Unlock()
	}()
	startBoostWatcher()
	return "ok:Boost automatico acceso: quando apri un gioco il Tasso prepara il PC da solo e misura gli FPS."
}

// startBoostWatcher: guarda ogni 10 secondi se c'è un gioco aperto.
func startBoostWatcher() {
	bWatch.Do(func() {
		go func() {
			defer guard("boost.go:watcher")
			var missing int
			var measured string
			var since time.Time
			t := time.NewTicker(10 * time.Second)
			defer t.Stop()
			for range t.C {
				bMu.Lock()
				auto, game, skip := loadBoost().Auto, bGame, bSkip
				bMu.Unlock()
				setMu.Lock()
				paused := settings.Paused
				setMu.Unlock()
				name := ""
				if auto && !paused {
					name = safeName(runningGame())
				}
				if name == "" && skip != "" {
					bMu.Lock()
					bSkip = ""
					bMu.Unlock()
				}
				// Boost a mano (senza automatico): il mouse torna com'era appena chiudi lo sparatutto.
				if game == "" && mouseAccelActive() && !competitiveGame(safeName(runningGame())) {
					if m := mouseAccelRestore(); m != "" {
						emit("onBoost", map[string]interface{}{"msgs": []string{m}, "state": boostInfo(), "auto": true})
					}
				}
				switch {
				case name != "" && game == "" && !strings.EqualFold(name, skip):
					missing, measured, since = 0, "", time.Now()
					bOpMu.Lock()
					bMu.Lock()
					bGame = name
					bMu.Unlock()
					closed := closeBackground()
					bOpMu.Unlock()
					msgs := []string{"Boost automatico per " + name + "."}
					if len(closed) > 0 {
						msgs = append(msgs, "Chiusi in background: "+strings.Join(closed, ", ")+". Li riapro quando chiudi il gioco.")
					}
					if m := mouseAccelOff(name); m != "" {
						msgs = append(msgs, m)
					}
					emit("onBoost", map[string]interface{}{"msgs": msgs, "state": boostInfo(), "auto": true})
				case name != "" && game != "":
					missing = 0
					if !strings.EqualFold(name, game) {
						bMu.Lock()
						if bGame != "" {
							bGame = name
						}
						bMu.Unlock()
						measured, since = "", time.Now()
						// Cambio gioco: mouse senza accelerazione solo se anche il nuovo è competitivo.
						m := ""
						if competitiveGame(name) {
							m = mouseAccelOff(name)
						} else {
							m = mouseAccelRestore()
						}
						if m != "" {
							emit("onBoost", map[string]interface{}{"msgs": []string{m}, "state": boostInfo(), "auto": true})
						}
					}
					// Una misura per partita, dopo 2 minuti (così salto il caricamento), senza bip.
					if measured != name && time.Since(since) > 2*time.Minute {
						measured = name
						go func(g string) {
							defer guard("boost.go:automeasure")
							if r, err := fpsMeasure(0, 60, true, false, g); err == nil {
								emit("onFpsAuto", map[string]interface{}{"game": g, "result": r})
								fpsAutoFix(false)
							}
						}(name)
					}
				case name == "" && game != "":
					missing++
					if missing >= 6 || !auto || paused { // un minuto senza giochi aperti
						missing = 0
						bOpMu.Lock()
						bMu.Lock()
						bGame = ""
						bMu.Unlock()
						names := reopenBackground()
						bOpMu.Unlock()
						msgs := []string{"Gioco chiuso."}
						if len(names) > 0 {
							msgs = append(msgs, "Riaperti: "+strings.Join(names, ", ")+".")
						}
						if m := mouseAccelRestore(); m != "" {
							msgs = append(msgs, m)
						}
						emit("onBoost", map[string]interface{}{"msgs": msgs, "state": boostInfo(), "auto": true})
					}
				}
			}
		}()
	})
}

// ---------------- Misura FPS (PresentMon) ----------------

var notFpsApp = map[string]bool{
	"dwm.exe": true, "tasso.exe": true, "msedgewebview2.exe": true, "explorer.exe": true,
	"msedge.exe": true, "chrome.exe": true, "firefox.exe": true, "opera.exe": true, "brave.exe": true,
	"discord.exe": true, "steamwebhelper.exe": true, "obs64.exe": true, "nvidia overlay.exe": true,
	"textinputhost.exe": true, "searchhost.exe": true, "startmenuexperiencehost.exe": true,
	"shellexperiencehost.exe": true, "applicationframehost.exe": true, "claude.exe": true,
}

// presentMonDir: Programmi\Tasso\strumenti. Il percorso lo chiedo a Windows, non alle variabili
// d'ambiente (che un utente normale può cambiare).
func presentMonDir() (string, error) {
	pf, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil || pf == "" {
		return "", errors.New("cartella Programmi non trovata")
	}
	return filepath.Join(pf, "Tasso", "strumenti"), nil
}

// lockedPresentMon apre PresentMon bloccando scrittura, rinomina e cancellazione, controlla che sia
// identico all'originale e lascia il file bloccato finché il chiamante non chiude la maniglia
// (cioè finché PresentMon non è partito): nessuno può sostituirlo in mezzo.
func lockedPresentMon() (string, windows.Handle, error) {
	dir, err := presentMonDir()
	if err != nil {
		return "", 0, err
	}
	p := filepath.Join(dir, "PresentMon.exe")
	open := func() (windows.Handle, error) {
		u, _ := windows.UTF16PtrFromString(p)
		return windows.CreateFile(u, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	}
	check := func(h windows.Handle) bool {
		hs := sha256.New()
		buf := make([]byte, 64*1024)
		for {
			var n uint32
			if err := windows.ReadFile(h, buf, &n, nil); err != nil {
				return false
			}
			if n == 0 {
				break
			}
			hs.Write(buf[:n])
		}
		return hex.EncodeToString(hs.Sum(nil)) == presentMonSHA
	}
	if h, err := open(); err == nil {
		if check(h) {
			return p, h, nil
		}
		windows.CloseHandle(h)
	}
	sum := sha256.Sum256(presentMonExe)
	if hex.EncodeToString(sum[:]) != presentMonSHA {
		return "", 0, errors.New("il misuratore di FPS dentro Tasso è danneggiato")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, err
	}
	tmp := p + ".nuovo"
	_ = os.Remove(tmp)
	if err := os.WriteFile(tmp, presentMonExe, 0o755); err != nil {
		return "", 0, err
	}
	_ = os.Remove(p)
	if err := os.Rename(tmp, p); err != nil {
		return "", 0, err
	}
	h, err := open()
	if err != nil {
		return "", 0, err
	}
	if !check(h) {
		windows.CloseHandle(h)
		return "", 0, errors.New("non riesco a preparare il misuratore di FPS")
	}
	return p, h, nil
}

func beep(n int) {
	for i := 0; i < n; i++ {
		pMessageBeep.Call(0xFFFFFFFF)
		time.Sleep(250 * time.Millisecond)
	}
}

func foregroundPid() uint32 {
	h, _, _ := pForegroundWin.Call()
	if h == 0 {
		return 0
	}
	var pid uint32
	pGetWindowPid.Call(h, uintptr(unsafe.Pointer(&pid)))
	return pid
}

// fpsMeasure misura per secs secondi dopo delay secondi, leggendo i dati da PresentMon senza file
// intermedi. prefer = nome del gioco riconosciuto (se c'è), altrimenti vince la finestra con più
// fotogrammi. Il gioco deve restare davanti: se non lo era, la misura non vale.
func fpsMeasure(delay, secs int, boosted, beeps bool, prefer string) (*fpsResult, error) {
	if !fpsMu.TryLock() {
		return nil, errors.New("c'è già una misura in corso")
	}
	defer fpsMu.Unlock()
	pm, lock, err := lockedPresentMon()
	if err != nil {
		return nil, errors.New("Misura FPS non disponibile: " + err.Error() + ".")
	}
	args := []string{"--output_stdout", "--timed", strconv.Itoa(secs), "--terminate_after_timed",
		"--session_name", "TassoFPS", "--stop_existing_session", "--no_console_stats", "--no_track_input"}
	if delay > 0 {
		args = append(args, "--delay", strconv.Itoa(delay))
	}
	for e := range notFpsApp {
		args = append(args, "--exclude", e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(delay+secs+30)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, pm, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	cmd.Dir = filepath.Dir(pm)
	out, err := cmd.StdoutPipe()
	if err != nil {
		windows.CloseHandle(lock)
		return nil, errors.New("non riesco ad avviare la misura degli FPS")
	}
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	startErr := cmd.Start()
	windows.CloseHandle(lock)
	if startErr != nil {
		return nil, errors.New("non riesco ad avviare la misura degli FPS")
	}
	done := make(chan struct{})
	temps := make(chan tempRead, 1)
	go func() {
		defer guard("boost.go:temps")
		// Verso la fine della misura, mentre il gioco lavora ancora.
		select {
		case <-done:
		case <-time.After(time.Duration(delay+secs-8) * time.Second):
		}
		temps <- readTemps()
	}()
	fg := map[uint32]bool{}
	var fgMu sync.Mutex
	go func() {
		defer guard("boost.go:fg")
		select {
		case <-done:
			return
		case <-time.After(time.Duration(delay)*time.Second + 2*time.Second):
		}
		if beeps {
			beep(1) // inizia
		}
		fgMu.Lock()
		fg[foregroundPid()] = true
		fgMu.Unlock()
	}()
	res, perr := parsePresentMon(bufio.NewReader(out), prefer)
	_, _ = io.Copy(io.Discard, out)
	werr := cmd.Wait()
	close(done)
	hzAtEnd := gameScreenHz()
	fgMu.Lock()
	fg[foregroundPid()] = true
	fgMu.Unlock()
	if beeps {
		beep(2) // finita
	}
	if werr != nil && perr != nil {
		var ee *exec.ExitError
		if (errors.As(werr, &ee) && ee.ExitCode() == 6) || strings.Contains(strings.ToLower(errBuf.String()), "access") {
			return nil, errors.New("Windows non mi lascia misurare gli FPS: serve aprire Tasso come amministratore")
		}
		return nil, errors.New("la misura degli FPS si è fermata: riprova")
	}
	if perr != nil {
		return nil, perr
	}
	pidN, _ := strconv.ParseUint(res.pid, 10, 32)
	fgMu.Lock()
	front := fg[uint32(pidN)]
	fgMu.Unlock()
	if !front {
		return nil, errors.New("il gioco non era in primo piano durante la misura: tienilo davanti e riprova")
	}
	res.Boost, res.When = boosted, time.Now().Format(time.RFC3339)
	// Il programma con quel numero deve essere ancora il gioco (i numeri dei processi si riusano).
	if pp := processPath(uint32(pidN)); pp != "" && strings.EqualFold(safeName(strings.TrimSuffix(filepath.Base(pp), filepath.Ext(pp))), res.Game) {
		res.path = pp
	}
	res.Hz = hzAtEnd
	select {
	case t := <-temps:
		res.CpuTemp, res.GpuTemp, res.Throttle, res.VramPct = t.cpu, t.gpu, t.throttle, t.vram
	case <-time.After(10 * time.Second):
	}
	setLastFps(res)
	bMu.Lock()
	s := loadBoost()
	if k := strings.ToLower(res.Game); k != "" {
		s.History[k] = append(s.History[k], res)
		if n := len(s.History[k]); n > 10 {
			s.History[k] = s.History[k][n-10:]
		}
		saveBoost()
	}
	bMu.Unlock()
	return res, nil
}

// parsePresentMon legge il CSV di PresentMon e calcola FPS medi, 1% low e scatti della finestra del
// gioco (prefer) o di quella con più fotogrammi. I tempi tra due fotogrammi valgono per singola
// finestra (swapchain), quindi raggruppo per programma + finestra.
func parsePresentMon(r io.Reader, prefer string) (*fpsResult, error) {
	errNone := errors.New("nessun gioco visto: apri il gioco e tienilo davanti mentre misuro")
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	head, err := cr.Read()
	if err != nil {
		return nil, errNone
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(strings.TrimPrefix(h, "\ufeff"))] = i
	}
	ib, okB := col["MsGPUBusy"]
	if !okB {
		ib, okB = col["GPUBusy"]
	}
	ia, ok1 := col["Application"]
	ip, ok2 := col["ProcessID"]
	is, okS := col["SwapChainAddress"]
	// Colonna del tempo tra due fotogrammi: versione attuale, 1.x e 2.x di PresentMon.
	it, ok3 := col["MsBetweenPresents"]
	for _, alt := range []string{"msBetweenPresents", "FrameTime"} {
		if !ok3 {
			it, ok3 = col[alt]
		}
	}
	if !ok1 || !ok2 || !ok3 {
		return nil, errors.New("il misuratore di FPS ha dato un risultato che non conosco")
	}
	type grp struct {
		app, pid string
		ft       []float64
		busy     float64
		busyFt   float64
		nBusy    int
	}
	groups := map[string]*grp{}
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(rec) <= it || len(rec) <= ia || len(rec) <= ip {
			continue
		}
		app := rec[ia]
		if notFpsApp[strings.ToLower(app)] {
			continue
		}
		v, err := strconv.ParseFloat(rec[it], 64)
		// 0 = primo fotogramma; oltre 2 secondi = pausa, menu o cambio finestra, non uno scatto.
		if err != nil || v <= 0 || v > 2000 {
			continue
		}
		key := rec[ip]
		if okS && len(rec) > is {
			key += "/" + rec[is]
		}
		g := groups[key]
		if g == nil {
			if len(groups) > 500 {
				continue
			}
			g = &grp{app: app, pid: rec[ip]}
			groups[key] = g
		}
		g.ft = append(g.ft, v)
		if okB && len(rec) > ib {
			if b, err := strconv.ParseFloat(rec[ib], 64); err == nil && b >= 0 && b <= 2000 {
				g.busy += b
				g.busyFt += v
				g.nBusy++
			}
		}
	}
	var best *grp
	isPref := func(g *grp) bool {
		return prefer != "" && strings.EqualFold(safeName(strings.TrimSuffix(g.app, ".exe")), prefer)
	}
	for _, g := range groups {
		if len(g.ft) < 100 {
			continue
		}
		if best == nil || (isPref(g) && !isPref(best)) || (isPref(g) == isPref(best) && len(g.ft) > len(best.ft)) {
			best = g
		}
	}
	if best == nil {
		return nil, errNone
	}
	res := fpsStats(safeName(strings.TrimSuffix(best.app, ".exe")), best.ft)
	res.pid = best.pid
	if best.nBusy > len(best.ft)/2 && best.busyFt > 0 {
		res.GpuBusy = math.Round(math.Min(100, best.busy/best.busyFt*100))
	}
	return res, nil
}

func fpsStats(app string, ft []float64) *fpsResult {
	total := 0.0
	for _, v := range ft {
		total += v
	}
	s := append([]float64(nil), ft...)
	sort.Float64s(s)
	median := s[len(s)/2]
	n := len(s) / 100
	if n < 1 {
		n = 1
	}
	slow := 0.0
	for _, v := range s[len(s)-n:] {
		slow += v
	}
	p5, p95 := s[len(s)*5/100], s[len(s)*95/100]
	stutter := 0
	for _, v := range ft {
		if v > 2*median && v > 1000.0/60 { // sopra i 60 FPS un fotogramma "doppio" non si nota
			stutter++
		}
	}
	round := func(x float64) float64 { return math.Round(x*10) / 10 }
	return &fpsResult{
		Game: app, Seconds: round(total / 1000), Frames: len(ft),
		Avg: round(float64(len(ft)) * 1000 / total), Low1: round(1000 / (slow / float64(n))),
		Stutter: stutter, StutterPM: round(float64(stutter) / (total / 60000)),
		Spread: round((p95 - p5) / median * 100),
	}
}

// fpsCompare: confronto onesto tra una misura senza Boost e una con. Una sola misura di gioco
// libero varia facilmente del 5-10%: sotto queste soglie, o se FPS medi e cali vanno in direzioni
// diverse, il risultato è "più o meno uguale". Guadagni e perdite sono trattati allo stesso modo.
func fpsCompare(a, b *fpsResult) map[string]interface{} {
	out := map[string]interface{}{"sameGame": strings.EqualFold(a.Game, b.Game)}
	if a.Avg <= 0 || b.Avg <= 0 || a.Low1 <= 0 {
		return out
	}
	da := (b.Avg - a.Avg) / a.Avg * 100
	dl := (b.Low1 - a.Low1) / a.Low1 * 100
	out["avgPct"], out["lowPct"] = math.Round(da*10)/10, math.Round(dl*10)/10
	var verdict string
	switch {
	case !strings.EqualFold(a.Game, b.Game):
		verdict = "Le due misure sono di giochi diversi: non si possono confrontare."
	case (da >= 5 && dl >= 0) || (dl >= 10 && da >= 0):
		verdict = fmt.Sprintf("Con il Boost va meglio: FPS medi %+.0f%%, cali (1%% low) %+.0f%%. Vale se hai giocato nello stesso posto e con la stessa grafica.", da, dl)
	case (da <= -5 && dl <= 0) || (dl <= -10 && da <= 0):
		verdict = fmt.Sprintf("Con il Boost va peggio: FPS medi %+.0f%%, cali %+.0f%%. Può essere la scena; se succede di nuovo nello stesso posto, premi \"Rimetti com'era\".", da, dl)
	default:
		verdict = "Più o meno uguale: la differenza è dentro la variazione normale tra due momenti di gioco."
	}
	out["verdict"] = verdict
	return out
}

// fpsSummary: per ogni gioco l'ultima misura senza Boost, l'ultima con Boost e il confronto.
func fpsSummary() map[string]interface{} {
	bMu.Lock()
	defer bMu.Unlock()
	out := map[string]interface{}{}
	for k, list := range loadBoost().History {
		var off, on *fpsResult
		for _, r := range list {
			if r.Boost {
				on = r
			} else {
				off = r
			}
		}
		e := map[string]interface{}{"without": off, "with": on, "list": list}
		if off != nil && on != nil {
			e["compare"] = fpsCompare(off, on)
		}
		out[k] = e
	}
	return out
}

// ---------------- Collegamento all'interfaccia ----------------

// bindBoost collega il Boost all'interfaccia (una riga in main.go) e fa ripartire il Boost
// automatico se era acceso.
func bindBoost(w webview2.WebView) {
	_ = w.Bind("goBoostAll", func() string {
		return async("boostall", func() {
			msgs, ok := boostOnOK()
			emit("onBoost", map[string]interface{}{"msgs": msgs, "state": boostInfo()})
			if ok { // niente sistemazioni se il Boost ha rifiutato (Tasso spento o già acceso)
				fpsAutoFix(true)
			}
		})
	})
	_ = w.Bind("goBoostAllOff", func() string {
		return async("boostoff", func() { emit("onBoost", map[string]interface{}{"msgs": boostOff(true), "state": boostInfo()}) })
	})
	_ = w.Bind("goBoostInfo", func() map[string]interface{} { return boostInfo() })
	_ = w.Bind("goBoostAuto", func(on bool) string { return setBoostAuto(on) })
	_ = w.Bind("goFpsMeasure", func() string {
		return async("fps", func() {
			bMu.Lock()
			s := loadBoost()
			boosted := s.On || s.Auto
			bMu.Unlock()
			r, err := fpsMeasure(10, 30, boosted, true, safeName(runningGame()))
			if err != nil {
				emit("onFps", map[string]interface{}{"ok": false, "msg": err.Error()})
				return
			}
			emit("onFps", map[string]interface{}{"ok": true, "result": r, "summary": fpsSummary()})
		})
	})
	_ = w.Bind("goFpsSummary", func() map[string]interface{} { return fpsSummary() })
	_ = w.Bind("goFpsCheck", func() string {
		return async("fpscheck", func() { emit("onFpsCheck", fpsCheck()) })
	})
	_ = w.Bind("goFpsFixHz", func() string { return async("fpshz", screenHzMax) })
	_ = w.Bind("goFpsKeepHz", func() string { go screenHzKeep(); return "ok" })
	_ = w.Bind("goFpsUndoHz", func() string { go screenHzUndo(false); return "ok" })
	_ = w.Bind("goFpsFixGpu", func() string { return fpsSetGpu() })
	_ = w.Bind("goFpsDriver", func(id string) string {
		if id != "nvidia" && id != "amd" && id != "intel" {
			return "no"
		}
		return openVendorPage(id)
	})
	bMu.Lock()
	auto := loadBoost().Auto
	bMu.Unlock()
	// Il Tasso si era chiuso con il mouse ancora senza accelerazione: lo rimetto com'era.
	if !competitiveGame(safeName(runningGame())) {
		mouseAccelRestore()
	} else {
		startBoostWatcher()
	}
	if auto {
		startBoostWatcher()
	}
}
