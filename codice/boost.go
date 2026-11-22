package main

// BOOST (3.1): un solo pulsante che prepara il PC per giocare, misura gli FPS veri prima e dopo
// e rimette tutto com'era. Usa solo regolazioni che il Tasso sapeva già fare (comandi gaming,
// modalità Gaming, chiusura gentile dei programmi in background, memoria in standby) e le mette
// insieme. Non tocca mai i file del gioco né la sua memoria: niente problemi con gli anticheat.
//
// Onestà sugli FPS: le regolazioni di Windows di solito danno da 0 al 10% in più, e aiutano
// soprattutto contro gli scatti (1% low) sui PC con tanti programmi aperti. Per questo il Tasso
// misura davvero, con PresentMon di Intel (gratuito, licenza MIT, firmato da Intel): legge gli
// eventi di Windows sui fotogrammi mostrati, come fanno i contatori FPS dei giochi, senza entrare
// nel gioco. Se la differenza è piccola lo dice.
//
// Boost automatico ("capacità di adattamento"): quando parte un gioco il Tasso accende il Boost da
// solo; la modalità giusta per QUEL gioco la sceglie la modalità gioco automatica (Gaming, FPS,
// Potenza per ogni gioco o Regola con Tasso). Con programmi normali non fa niente. Un minuto dopo
// la chiusura del gioco rimette tutto com'era e riapre i programmi chiusi.

import (
	"bytes"
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

	webview2 "github.com/jchv/go-webview2"
)

//go:embed strumenti/PresentMon.exe
var presentMonExe []byte

// SHA-256 di PresentMon 2.4.1 x64 (release ufficiale Intel su GitHub). Se il file sul disco non è
// identico non lo avvio.
const presentMonSHA = "d74183e7ae630f72cd3690be0373ecbfdc6cbb86578148aab8fa2a7166068f34"

// I comandi gaming che il Boost accende (solo se erano spenti). La pianificazione GPU (hags) no:
// vuole il riavvio del PC e su alcuni PC peggiora le cose.
var boostCmds = []string{"gamedvr", "mmcss", "nagle", "winopt"}

type fpsResult struct {
	Game    string  `json:"game"`
	When    string  `json:"when"`
	Boost   bool    `json:"boost"`
	Seconds float64 `json:"seconds"`
	Frames  int     `json:"frames"`
	Avg     float64 `json:"avg"`     // FPS medi
	Low1    float64 `json:"low1"`    // 1% low: la media del centesimo di fotogrammi più lenti
	Stutter int     `json:"stutter"` // scatti: fotogrammi lunghi più del doppio del normale
}

type boostState struct {
	On       bool                    `json:"on"`
	Auto     bool                    `json:"auto"`   // Boost automatico acceso
	ByAuto   bool                    `json:"byAuto"` // acceso da solo all'apertura di un gioco
	Game     string                  `json:"game"`   // gioco per cui è acceso (se automatico)
	Since    string                  `json:"since"`
	Cmds     []string                `json:"cmds"`     // comandi accesi dal Boost (da rispegnere)
	PrevMode string                  `json:"prevMode"` // modalità da rimettere ("" = non l'ha cambiata il Boost)
	Closed   []string                `json:"closed"`   // programmi chiusi (percorso), da riaprire
	Before   *fpsResult              `json:"before,omitempty"`
	After    *fpsResult              `json:"after,omitempty"`
	History  map[string][]*fpsResult `json:"history,omitempty"` // per gioco, ultime misure
}

var (
	pMessageBeep = user32.NewProc("MessageBeep")
	bMu          sync.Mutex
	bState       *boostState
	fpsMu        sync.Mutex // una misura alla volta
	bAutoRun     sync.Once
)

func boostFile() string { return filepath.Join(appDir, "boost.json") }

// loadBoost: lo stato resta salvato, così "Rimetti com'era" funziona anche dopo un riavvio di Tasso.
// Chiamare con bMu preso.
func loadBoost() *boostState {
	if bState != nil {
		return bState
	}
	bState = &boostState{}
	if b, err := os.ReadFile(boostFile()); err == nil {
		_ = json.Unmarshal(b, bState)
	}
	// boost.json si può modificare senza essere amministratore: tengo solo valori ammessi.
	var ok []string
	for _, c := range bState.Cmds {
		if hasStr(boostCmds, c) && !hasStr(ok, c) {
			ok = append(ok, c)
		}
	}
	bState.Cmds = ok
	if _, valid := modeScripts[bState.PrevMode]; !valid {
		bState.PrevMode = ""
	}
	if bState.History == nil {
		bState.History = map[string][]*fpsResult{}
	}
	return bState
}

func saveBoost() {
	if bState == nil {
		return
	}
	b, err := json.MarshalIndent(bState, "", "  ")
	if err == nil {
		_ = os.WriteFile(boostFile(), b, 0o644)
	}
}

func boostInfo() map[string]interface{} {
	bMu.Lock()
	defer bMu.Unlock()
	s := loadBoost()
	out := map[string]interface{}{
		"on": s.On, "auto": s.Auto, "byAuto": s.ByAuto, "game": s.Game, "since": s.Since,
		"before": s.Before, "after": s.After,
	}
	if s.Before != nil && s.After != nil {
		out["compare"] = fpsCompare(s.Before, s.After)
	}
	return out
}

// ---------------- Accendi / spegni ----------------

// boostOn accende il Boost. game = "" se acceso a mano. Con il Boost automatico la modalità
// la sceglie la modalità gioco automatica, quindi qui non la cambio (setMode=false).
func boostOn(game string, byAuto bool) []string {
	setMu.Lock()
	paused, mode := settings.Paused, settings.Mode
	setMu.Unlock()
	if paused {
		return []string{"Tasso è spento: riaccendilo per usare il Boost."}
	}
	bMu.Lock()
	s := loadBoost()
	if s.On {
		bMu.Unlock()
		return []string{"Il Boost è già acceso."}
	}
	s.On, s.ByAuto, s.Game, s.Since = true, byAuto, game, time.Now().Format(time.RFC3339)
	s.Cmds, s.Closed, s.PrevMode = nil, nil, ""
	saveBoost() // subito: se Tasso si chiude a metà, "Rimetti com'era" sa che c'è qualcosa da rimettere
	bMu.Unlock()

	var msgs []string
	// 1) Comandi gaming: accendo solo quelli spenti e mi ricordo quali, per rispegnere solo quelli.
	st := gamingState()
	for _, id := range boostCmds {
		if on, _ := st[id].(bool); on {
			continue
		}
		if id == "winopt" && winBuild() < 22000 {
			continue
		}
		r := gamingSet(id, true)
		if strings.HasPrefix(r, "ok:") {
			bMu.Lock()
			s.Cmds = append(s.Cmds, id)
			saveBoost()
			bMu.Unlock()
			msgs = append(msgs, strings.TrimPrefix(r, "ok:"))
		}
	}
	// 2) Modalità Gaming (piano energetico al massimo, notifiche in pausa), se non c'è già.
	setMu.Lock()
	autoGame := settings.AutoGame
	setMu.Unlock()
	if !(byAuto && autoGame) && mode != "gaming" && mode != "fps" {
		prev := mode
		if _, ok := modeScripts[prev]; !ok {
			prev = "normale"
		}
		bMu.Lock()
		s.PrevMode = prev
		saveBoost()
		bMu.Unlock()
		setMode("gaming")
		msgs = append(msgs, "Modalità Gaming: processore e scheda video al massimo, notifiche in pausa.")
	}
	// 3) Programmi in background che non servono (sincronizzazione e chat senza finestra aperta).
	//    Quelli con una finestra non li chiudo da solo: potresti avere un lavoro non salvato.
	for _, h := range listHeavy() {
		if !h.Suggest || h.Window {
			continue
		}
		if closeGently(strings.ToLower(h.Name) + ".exe") {
			bMu.Lock()
			s.Closed = append(s.Closed, h.path)
			saveBoost()
			bMu.Unlock()
			msgs = append(msgs, "Chiuso in background: "+h.Name+" (lo riapro quando hai finito).")
		}
	}
	// 4) Memoria in standby: la svuoto una volta, così il gioco trova RAM libera subito.
	if r := purgeStandby(); strings.HasPrefix(r, "ok:") {
		msgs = append(msgs, "Memoria liberata.")
	}
	if len(msgs) == 0 {
		msgs = append(msgs, "Il PC era già pronto per giocare: non c'era niente da cambiare.")
	}
	return msgs
}

// boostOff rimette com'era solo quello che il Boost ha cambiato.
func boostOff() []string {
	bMu.Lock()
	s := loadBoost()
	if !s.On {
		bMu.Unlock()
		return []string{"Il Boost era già spento."}
	}
	cmds, prev, closed := s.Cmds, s.PrevMode, s.Closed
	bMu.Unlock()

	var msgs []string
	for _, id := range cmds {
		if r := gamingSet(id, false); strings.HasPrefix(r, "ok:") {
			msgs = append(msgs, strings.TrimPrefix(r, "ok:"))
		}
	}
	if prev != "" {
		setMu.Lock()
		cur := settings.Mode
		setMu.Unlock()
		if cur == "gaming" { // se nel frattempo hai scelto tu un'altra modalità, la lascio
			setMode(prev)
			msgs = append(msgs, "Rimessa la modalità di prima.")
		}
	}
	for _, p := range closed {
		// Riaperto tramite Esplora file, così parte come programma normale e non da amministratore.
		if p != "" && fileExists(p) && openPath(filepath.Join(os.Getenv("SystemRoot"), "explorer.exe"), "\""+p+"\"") {
			msgs = append(msgs, "Riaperto: "+strings.TrimSuffix(filepath.Base(p), ".exe"))
		}
	}
	bMu.Lock()
	s.On, s.ByAuto, s.Game, s.Since, s.Cmds, s.PrevMode, s.Closed = false, false, "", "", nil, "", nil
	saveBoost()
	bMu.Unlock()
	msgs = append(msgs, "Tutto com'era prima del Boost.")
	return msgs
}

// ---------------- Pulsante: Boost con misura prima e dopo ----------------

// boostWithMeasure: misura 30 secondi senza Boost, accende il Boost, misura altri 30 secondi.
// Hai 10 secondi per tornare nel gioco: un bip all'inizio e due alla fine di ogni misura.
func boostWithMeasure(measure bool) {
	if !measure {
		msgs := boostOn("", false)
		emit("onBoost", map[string]interface{}{"msgs": msgs, "state": boostInfo()})
		return
	}
	bMu.Lock()
	on := loadBoost().On
	bMu.Unlock()
	if on {
		emit("onBoost", map[string]interface{}{"msgs": []string{"Il Boost è già acceso: premi \"Rimetti com'era\" e riprova per misurare prima e dopo."}, "state": boostInfo()})
		return
	}
	emit("onBoostStep", map[string]interface{}{"step": "before", "msg": "Torna nel gioco entro 10 secondi e gioca normalmente: misuro gli FPS per 30 secondi."})
	before, err := fpsMeasure(10, 30, false)
	if err != nil {
		emit("onBoost", map[string]interface{}{"msgs": []string{err.Error()}, "state": boostInfo()})
		return
	}
	emit("onBoostStep", map[string]interface{}{"step": "apply", "msg": "Misura fatta. Accendo il Boost…"})
	msgs := boostOn("", false)
	time.Sleep(5 * time.Second) // lascio assestare Windows dopo il cambio
	emit("onBoostStep", map[string]interface{}{"step": "after", "msg": "Boost acceso. Continua a giocare nello stesso posto: misuro altri 30 secondi."})
	after, err := fpsMeasure(0, 30, true)
	if err != nil {
		msgs = append(msgs, "Seconda misura non riuscita: "+err.Error())
	}
	bMu.Lock()
	s := loadBoost()
	s.Before, s.After = before, after
	saveBoost()
	bMu.Unlock()
	emit("onBoost", map[string]interface{}{"msgs": msgs, "state": boostInfo()})
}

// fpsCompare: confronto onesto. Sotto il 3% non si può dire che sia merito del Boost.
func fpsCompare(a, b *fpsResult) map[string]interface{} {
	out := map[string]interface{}{"sameGame": strings.EqualFold(a.Game, b.Game)}
	if a.Avg <= 0 || b.Avg <= 0 {
		return out
	}
	da := (b.Avg - a.Avg) / a.Avg * 100
	dl := 0.0
	if a.Low1 > 0 {
		dl = (b.Low1 - a.Low1) / a.Low1 * 100
	}
	out["avgPct"], out["lowPct"] = math.Round(da*10)/10, math.Round(dl*10)/10
	var verdict string
	switch {
	case !strings.EqualFold(a.Game, b.Game):
		verdict = "Le due misure sono di programmi diversi: non si possono confrontare."
	case math.Abs(da) < 3 && math.Abs(dl) < 5:
		verdict = "Differenza piccola: è dentro la variazione normale tra due momenti di gioco. Il tuo PC era già in forma."
	case da >= 3 || dl >= 5:
		verdict = fmt.Sprintf("Con il Boost: FPS medi %+.0f%%, cali (1%% low) %+.0f%%. Vale se hai giocato nella stessa zona e con la stessa grafica.", da, dl)
	default:
		verdict = "Con il Boost gli FPS sono scesi: probabilmente la scena era più pesante. Riprova nello stesso punto del gioco."
	}
	out["verdict"] = verdict
	return out
}

// ---------------- Misura FPS (PresentMon) ----------------

var notFpsApp = map[string]bool{
	"dwm.exe": true, "tasso.exe": true, "msedgewebview2.exe": true, "explorer.exe": true,
	"msedge.exe": true, "chrome.exe": true, "firefox.exe": true, "opera.exe": true, "brave.exe": true,
	"discord.exe": true, "steamwebhelper.exe": true, "obs64.exe": true, "nvidia overlay.exe": true,
	"textinputhost.exe": true, "searchhost.exe": true, "startmenuexperiencehost.exe": true,
	"shellexperiencehost.exe": true, "applicationframehost.exe": true, "claude.exe": true,
}

// presentMonPath scrive PresentMon in Programmi\Tasso\strumenti (solo un amministratore può
// modificarlo) e controlla che sia identico all'originale prima di ogni uso.
func presentMonPath() (string, error) {
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		return "", errors.New("cartella Programmi non trovata")
	}
	dir := filepath.Join(pf, "Tasso", "strumenti")
	p := filepath.Join(dir, "PresentMon.exe")
	if fileSHA(p) != presentMonSHA {
		if fileSHA256(presentMonExe) != presentMonSHA {
			return "", errors.New("il misuratore di FPS dentro Tasso è danneggiato")
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		tmp := p + ".nuovo"
		if err := os.WriteFile(tmp, presentMonExe, 0o755); err != nil {
			return "", err
		}
		_ = os.Remove(p)
		if err := os.Rename(tmp, p); err != nil {
			return "", err
		}
		if fileSHA(p) != presentMonSHA {
			return "", errors.New("non riesco a preparare il misuratore di FPS")
		}
	}
	return p, nil
}

func fileSHA256(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func fileSHA(p string) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func beep(n int) {
	for i := 0; i < n; i++ {
		pMessageBeep.Call(0xFFFFFFFF)
		time.Sleep(250 * time.Millisecond)
	}
}

// fpsMeasure misura per secs secondi dopo delay secondi. Se c'è un gioco riconosciuto misura
// quello, altrimenti il programma che ha mostrato più fotogrammi (escluso Windows e i browser).
func fpsMeasure(delay, secs int, boosted bool) (*fpsResult, error) {
	if !fpsMu.TryLock() {
		return nil, errors.New("c'è già una misura in corso")
	}
	defer fpsMu.Unlock()
	pm, err := presentMonPath()
	if err != nil {
		return nil, errors.New("Misura FPS non disponibile: " + err.Error() + ".")
	}
	csvPath := filepath.Join(os.TempDir(), fmt.Sprintf("tasso-fps-%d.csv", time.Now().UnixNano()))
	defer os.Remove(csvPath)
	args := []string{"--output_file", csvPath, "--timed", strconv.Itoa(secs), "--terminate_after_timed",
		"--session_name", "TassoFPS", "--stop_existing_session", "--no_console_stats",
		"--no_track_input", "--no_track_gpu"}
	if delay > 0 {
		args = append(args, "--delay", strconv.Itoa(delay))
	}
	if name := runningGame(); name != "" {
		args = append(args, "--process_id", strconv.Itoa(int(gamePid.Load())))
	} else {
		for e := range notFpsApp {
			args = append(args, "--exclude", e)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(delay+secs+30)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, pm, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, errors.New("non riesco ad avviare la misura degli FPS")
	}
	go func() {
		defer guard("boost.go:beep")
		time.Sleep(time.Duration(delay) * time.Second)
		beep(1) // inizia
	}()
	err = cmd.Wait()
	beep(2) // finita
	f, ferr := os.Open(csvPath)
	if ferr != nil {
		if err != nil {
			return nil, errors.New("la misura degli FPS si è fermata: riprova")
		}
		return nil, errors.New("nessun gioco visto: apri il gioco e tienilo davanti mentre misuro")
	}
	defer f.Close()
	res, perr := parsePresentMon(f)
	if perr != nil {
		return nil, perr
	}
	res.Boost, res.When = boosted, time.Now().Format(time.RFC3339)
	bMu.Lock()
	s := loadBoost()
	k := strings.ToLower(res.Game)
	s.History[k] = append(s.History[k], res)
	if n := len(s.History[k]); n > 10 {
		s.History[k] = s.History[k][n-10:]
	}
	saveBoost()
	bMu.Unlock()
	return res, nil
}

// parsePresentMon legge il CSV di PresentMon e calcola FPS medi, 1% low e scatti del programma
// con più fotogrammi. Separata dal resto così si può provare anche senza Windows.
func parsePresentMon(r io.Reader) (*fpsResult, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	head, err := cr.Read()
	if err != nil {
		return nil, errors.New("nessun gioco visto: apri il gioco e tienilo davanti mentre misuro")
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(strings.TrimPrefix(h, "\ufeff"))] = i
	}
	ia, ok1 := col["Application"]
	ip, ok2 := col["ProcessID"]
	// Nome della colonna del tempo tra due fotogrammi: versione attuale, 1.x e 2.x di PresentMon.
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
		app string
		ft  []float64
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
		if err != nil || v <= 0 || v > 1000 { // NA o pause lunghissime (menu, caricamento)
			continue
		}
		g := groups[rec[ip]]
		if g == nil {
			g = &grp{app: app}
			groups[rec[ip]] = g
		}
		g.ft = append(g.ft, v)
	}
	var best *grp
	for _, g := range groups {
		if best == nil || len(g.ft) > len(best.ft) {
			best = g
		}
	}
	if best == nil || len(best.ft) < 100 {
		return nil, errors.New("nessun gioco visto: apri il gioco e tienilo davanti mentre misuro")
	}
	return fpsStats(strings.TrimSuffix(best.app, ".exe"), best.ft), nil
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
	stutter := 0
	for _, v := range ft {
		if v > 2*median && v > 1000.0/60 { // sopra i 60 FPS un fotogramma "doppio" non si nota
			stutter++
		}
	}
	round := func(x float64) float64 { return math.Round(x*10) / 10 }
	return &fpsResult{
		Game: app, Seconds: round(total / 1000), Frames: len(ft),
		Avg: round(float64(len(ft)) * 1000 / total), Low1: round(1000 / (slow / float64(n))), Stutter: stutter,
	}
}

// ---------------- Boost automatico ----------------

func setBoostAuto(on bool) string {
	bMu.Lock()
	s := loadBoost()
	s.Auto = on
	saveBoost()
	wasAuto := s.On && s.ByAuto
	bMu.Unlock()
	if on {
		// La modalità giusta per ogni gioco la mette la modalità gioco automatica.
		setMu.Lock()
		ag := settings.AutoGame
		setMu.Unlock()
		if !ag {
			setAutoGame(true)
		}
		startBoostWatcher()
		return "ok:Boost automatico acceso: quando apri un gioco il Tasso prepara il PC da solo e misura gli FPS."
	}
	if wasAuto {
		go boostOff()
	}
	return "ok:Boost automatico spento."
}

// startBoostWatcher: da chiamare all'avvio di Tasso (e quando accendi il Boost automatico).
func startBoostWatcher() {
	bAutoRun.Do(func() {
		go func() {
			defer guard("boost.go:watcher")
			var missing int
			var measured string
			var since time.Time
			t := time.NewTicker(10 * time.Second)
			defer t.Stop()
			for range t.C {
				bMu.Lock()
				s := loadBoost()
				auto, on, byAuto, game := s.Auto, s.On, s.ByAuto, s.Game
				bMu.Unlock()
				setMu.Lock()
				paused := settings.Paused
				setMu.Unlock()
				name := ""
				if auto && !paused {
					name = runningGame()
				}
				switch {
				case name != "" && !on:
					missing, measured, since = 0, "", time.Now()
					msgs := boostOn(name, true)
					emit("onBoost", map[string]interface{}{"msgs": msgs, "state": boostInfo(), "auto": true})
				case name != "" && on && byAuto:
					missing = 0
					if !strings.EqualFold(name, game) { // un altro gioco: aggiorno il nome
						bMu.Lock()
						s.Game = name
						saveBoost()
						bMu.Unlock()
						measured, since = "", time.Now()
					}
					// Una misura per partita, dopo 2 minuti di gioco (così salto il caricamento).
					if measured != name && time.Since(since) > 2*time.Minute {
						measured = name
						go func(g string) {
							defer guard("boost.go:automeasure")
							if r, err := fpsMeasure(0, 60, true); err == nil {
								emit("onFpsAuto", map[string]interface{}{"game": g, "result": r})
							}
						}(name)
					}
				case name == "" && on && byAuto:
					missing++
					if missing >= 6 || !auto || paused { // un minuto senza giochi aperti
						missing = 0
						msgs := boostOff()
						emit("onBoost", map[string]interface{}{"msgs": msgs, "state": boostInfo(), "auto": true})
					}
				}
			}
		}()
	})
}

// fpsHistory: le ultime misure di ogni gioco, per la schermata.
func fpsHistory() map[string][]*fpsResult {
	bMu.Lock()
	defer bMu.Unlock()
	out := map[string][]*fpsResult{}
	for k, v := range loadBoost().History {
		out[k] = append([]*fpsResult(nil), v...)
	}
	return out
}

// bindBoost collega il Boost all'interfaccia (una riga in main.go) e fa ripartire il Boost
// automatico se era acceso.
func bindBoost(w webview2.WebView) {
	_ = w.Bind("goBoostAll", func(measure bool) string { return async("boostall", func() { boostWithMeasure(measure) }) })
	_ = w.Bind("goBoostAllOff", func() string {
		return async("boostall", func() { emit("onBoost", map[string]interface{}{"msgs": boostOff(), "state": boostInfo()}) })
	})
	_ = w.Bind("goBoostInfo", func() map[string]interface{} { return boostInfo() })
	_ = w.Bind("goBoostAuto", func(on bool) string { return setBoostAuto(on) })
	_ = w.Bind("goFpsMeasure", func() string {
		return async("fps", func() {
			r, err := fpsMeasure(10, 30, false)
			if err != nil {
				emit("onFps", map[string]interface{}{"ok": false, "msg": err.Error()})
				return
			}
			emit("onFps", map[string]interface{}{"ok": true, "result": r})
		})
	})
	_ = w.Bind("goFpsHistory", func() map[string][]*fpsResult { return fpsHistory() })
	bMu.Lock()
	auto := loadBoost().Auto
	bMu.Unlock()
	if auto {
		startBoostWatcher()
	}
}
