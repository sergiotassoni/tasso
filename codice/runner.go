package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type psResult struct {
	Freed  int64
	Reboot bool
	JSON   string
	Err    error
}

var scriptSeq atomic.Int64

var (
	scriptDirOnce sync.Once
	scriptDirPath string
)

// scriptDir: gli script partono come amministratore, quindi li scrivo in Programmi\Tasso\lavoro,
// dove un programma normale dell'utente non può scrivere né sostituirli prima che partano.
// Solo se non si può (caso raro) uso la cartella di Tasso in AppData, come prima.
func scriptDir() string {
	scriptDirOnce.Do(func() {
		if pf := os.Getenv("ProgramFiles"); pf != "" {
			d := filepath.Join(pf, "Tasso", "lavoro")
			if os.MkdirAll(d, 0o755) == nil {
				scriptDirPath = d
				return
			}
		}
		scriptDirPath = filepath.Join(appDir, "scripts")
		_ = os.MkdirAll(scriptDirPath, 0o755)
	})
	return scriptDirPath
}

// writeScript salva lo script come normale file .ps1 (UTF-8 con BOM, così PowerShell 5.1 legge bene le lettere accentate).
func writeScript(script string) (string, error) {
	p := filepath.Join(scriptDir(), fmt.Sprintf("tasso_%d_%d_%d.ps1", os.Getpid(), scriptSeq.Add(1), time.Now().UnixNano()))
	// $TassoAuto: lo script gira dalla pulizia programmata (nessuno davanti al PC), quindi va più cauto.
	auto := "$TassoAuto = $false"
	if runAuto.Load() {
		auto = "$TassoAuto = $true"
	}
	auto += "\n$TassoLang = '" + curLang() + "'"
	if runSingle.Load() {
		auto += "\n$TassoSingle = $true"
	} else {
		auto += "\n$TassoSingle = $false"
	}
	body := "\uFEFF" + strings.ReplaceAll(trPS(psPrelude+"\n"+auto+"\n"+script), "\n", "\r\n")
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return p, err
}

// runPS esegue uno script PowerShell nascosto. Le righe "MSG ..." vanno nel registro,
// "FREED n" sommano lo spazio liberato, "REBOOT" segnala un riavvio, "JSON ..." restituisce dati.
func runPS(script string, timeout time.Duration, onMsg func(string)) psResult {
	return runPSCtx(context.Background(), script, timeout, onMsg)
}

func runPSCtx(parent context.Context, script string, timeout time.Duration, onMsg func(string)) psResult {
	res := psResult{}
	path, err := writeScript(script)
	if err != nil {
		res.Err = err
		return res
	}
	defer os.Remove(path)
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, sysExe("powershell.exe"), "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.StdoutPipe()
	if err != nil {
		res.Err = err
		return res
	}
	if err := cmd.Start(); err != nil {
		res.Err = err
		return res
	}
	// Job Object: se il passaggio va fermato (tempo scaduto o tasto Ferma) chiudo anche
	// tutti i programmi che PowerShell ha aperto (dism, sfc, defrag...), non solo PowerShell.
	job := newKillJob(cmd.Process.Pid)
	stopWatch := make(chan struct{})
	go func() {
		defer guard("runner.go:109")
		select {
		case <-ctx.Done():
			job.kill()
		case <-stopWatch:
		}
	}()
	defer func() { close(stopWatch); job.close() }()
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimPrefix(strings.TrimRight(sc.Text(), "\r"), "\uFEFF")
		switch {
		case strings.HasPrefix(line, "MSG "):
			if onMsg != nil {
				onMsg(strings.TrimPrefix(line, "MSG "))
			}
		case strings.HasPrefix(line, "FREED "):
			if n, e := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "FREED ")), 10, 64); e == nil && n > 0 {
				res.Freed += n
			}
		case line == "REBOOT":
			res.Reboot = true
		case strings.HasPrefix(line, "JSON "):
			res.JSON = strings.TrimPrefix(line, "JSON ")
		}
	}
	if sc.Err() != nil {
		// Riga troppo lunga: svuoto il resto, altrimenti PowerShell resta bloccato fino al tempo massimo.
		_, _ = io.Copy(io.Discard, out)
	}
	res.Err = cmd.Wait()
	if ctx.Err() == context.DeadlineExceeded && onMsg != nil {
		onMsg("Questo passaggio ci metteva troppo: l'ho fermato e vado avanti.")
	} else if ctx.Err() == context.Canceled && onMsg != nil {
		onMsg("Passaggio fermato.")
	}
	return res
}

var (
	runMu     sync.Mutex
	running   bool
	cancelReq atomic.Bool
)

func isRunning() bool {
	runMu.Lock()
	defer runMu.Unlock()
	return running
}

// runCtx è il contesto della pulizia in corso: il tasto Ferma lo annulla e ferma subito
// anche il passaggio che sta lavorando.
var (
	runCtxMu  sync.Mutex
	curCtx    = context.Background()
	curCancel context.CancelFunc
)

func runCtx() context.Context {
	runCtxMu.Lock()
	defer runCtxMu.Unlock()
	return curCtx
}

func newRunCtx() {
	runCtxMu.Lock()
	curCtx, curCancel = context.WithCancel(context.Background())
	runCtxMu.Unlock()
}

func endRunCtx() {
	runCtxMu.Lock()
	if curCancel != nil {
		curCancel()
	}
	curCtx, curCancel = context.Background(), nil
	runCtxMu.Unlock()
}

func cancelRun() {
	cancelReq.Store(true)
	runCtxMu.Lock()
	if curCancel != nil {
		curCancel()
	}
	runCtxMu.Unlock()
}

func startLevel(level int, shutdownAfter bool) string {
	if level < 1 || level > 5 {
		return "livello non valido"
	}
	var ids []string
	for _, s := range allSteps {
		if s.Level > 0 && s.Level <= level {
			ids = append(ids, s.ID)
		}
	}
	return startSteps(ids, level, shutdownAfter)
}

// Pulizia programmata: chi la chiede (avvio con --scheduled o la copia già aperta svegliata
// dall'attività) segna il livello qui; la pulizia di quel livello che parte entro poco (3 o 30 minuti) è
// "automatica" (nessuno davanti al PC) e solo quella gira con $TassoAuto.
var (
	autoMu      sync.Mutex
	autoPending int
	autoAt      time.Time
	autoTTL     time.Duration
	runAuto     atomic.Bool
	runSingle   atomic.Bool // un solo passaggio scelto da te, senza "spegni al termine"
)

func markAutoRun(level int, ttl time.Duration) {
	autoMu.Lock()
	autoPending, autoAt, autoTTL = level, time.Now(), ttl
	autoMu.Unlock()
}

func takeAutoRun(level int) bool {
	autoMu.Lock()
	defer autoMu.Unlock()
	ok := level > 0 && autoPending == level && time.Since(autoAt) < autoTTL
	autoPending = 0
	return ok
}

func startSteps(ids []string, level int, shutdownAfter bool) string {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var sel []Step
	for _, s := range allSteps {
		if want[s.ID] {
			sel = append(sel, s)
		}
	}
	if len(sel) == 0 {
		return "nessun passaggio"
	}
	runMu.Lock()
	if updating.Load() {
		runMu.Unlock()
		return "Tasso si sta aggiornando: riprova tra un minuto."
	}
	if running {
		runMu.Unlock()
		return "busy"
	}
	running = true
	runMu.Unlock()
	runAuto.Store(takeAutoRun(level))
	runSingle.Store(len(sel) == 1 && !shutdownAfter)
	cancelReq.Store(false)
	newRunCtx()

	go func() {
		// Se un passaggio va in errore grave, Tasso non si chiude: lo scrive in crash.txt
		// e mostra la pulizia come interrotta, così l'interfaccia non resta bloccata.
		defer func() {
			if r := recover(); r != nil {
				logPanic("pulizia", r, debug.Stack())
				emit("onRunDone", map[string]interface{}{
					"freed": 0, "seconds": 0, "reboot": false, "cancelled": true,
					"done": 0, "total": len(sel), "log": "", "shutdown": false,
				})
			}
		}()
		defer func() {
			endRunCtx()
			runAuto.Store(false)
			runSingle.Store(false)
			runMu.Lock()
			running = false
			runMu.Unlock()
		}()
		logPath := filepath.Join(appDir, "logs", "tasso_"+time.Now().Format("20060102_150405")+".txt")
		lf, _ := os.Create(logPath)
		defer func() {
			if lf != nil {
				lf.Close()
			}
		}()
		logLine := func(s string) {
			line := time.Now().Format("15:04:05") + "  " + s
			if lf != nil {
				fmt.Fprintln(lf, line)
			}
			emit("onLog", s)
		}

		list := make([]map[string]interface{}, 0, len(sel))
		for _, s := range sel {
			list = append(list, map[string]interface{}{"id": s.ID, "name": s.Name, "minutes": s.Minutes})
		}
		emit("onRunStart", map[string]interface{}{"level": level, "steps": list})
		logLine(fmt.Sprintf("Tasso %s - inizio (%d passaggi)", appVersion, len(sel)))

		snapOK := true
		for _, s := range sel {
			if s.ID == "privacy" || s.ID == "backup" {
				snapOK = ensureSnapshot() // salva le impostazioni originali prima di cambiarle
				break
			}
		}
		setMu.Lock()
		benchOn := settings.BenchAround && level > 0
		setMu.Unlock()
		var before benchResult
		if benchOn {
			logLine("Voto al PC prima della pulizia (circa 20 secondi)…")
			before = runBench("prima")
		}
		start := time.Now()
		var total int64
		reboot := false
		done := 0
		var doneNames []string
		for i, s := range sel {
			if cancelReq.Load() {
				logLine("Interrotto da te: i passaggi rimanenti sono stati saltati.")
				break
			}
			emit("onStepStart", map[string]interface{}{"index": i})
			logLine("» " + s.Name)
			if s.ID == "privacy" && !snapOK {
				// Senza la foto di com'era Windows non si potrebbe tornare indietro: lo salto.
				logLine("   Saltato: non sono riuscito a salvare com'era Windows, quindi non cambio la privacy.")
				done++
				emit("onStepDone", map[string]interface{}{"index": i, "freed": 0})
				continue
			}
			r := runPSCtx(runCtx(), s.Script, s.Timeout, func(m string) { logLine("   " + m) })
			if r.Err != nil && !cancelReq.Load() && r.Freed == 0 {
				logLine("   (dettaglio tecnico: " + r.Err.Error() + ")")
			}
			total += r.Freed
			if r.Reboot {
				reboot = true
			}
			done++
			doneNames = append(doneNames, s.Name)
			emit("onStepDone", map[string]interface{}{"index": i, "freed": r.Freed})
		}
		dur := time.Since(start)
		logLine(fmt.Sprintf("Fine: liberati %s in %s", fmtBytes(total), dur.Round(time.Second)))
		if level > 0 && done == len(sel) {
			updateSettings(func(st *Settings) {
				st.LastClean = &LastClean{Date: time.Now().Format("2006-01-02"), Level: level, Freed: total}
			})
			discordAfterClean(total)
		}
		if benchOn && !cancelReq.Load() {
			logLine("Voto al PC dopo la pulizia…")
			after := runBench("dopo")
			logLine(fmt.Sprintf("Leggerezza di Windows: %d → %d su 100. Velocità dei componenti: %d → %d su 100.", before.Light, after.Light, score100(before.Score), score100(after.Score)))
			emit("onBench", map[string]interface{}{"result": after, "before": before, "history": benchHistory()})
		}
		cancelled := cancelReq.Load()
		shutdown := false
		if shutdownAfter && !cancelled {
			logLine("Spegnimento automatico: il PC si spegne tra 2 minuti (puoi annullarlo da Tasso).")
			if _, err := runHidden("shutdown.exe", "/s", "/t", "120", "/c", "Tasso ha finito la manutenzione: il PC si spegne tra 2 minuti."); err == nil {
				shutdown = true
			}
		}
		emit("onRunDone", map[string]interface{}{
			"freed":     total,
			"seconds":   int(dur.Seconds()),
			"reboot":    reboot,
			"cancelled": cancelled,
			"done":      done,
			"total":     len(sel),
			"log":       logPath,
			"shutdown":  shutdown,
			"settings":  getSettings(),
		})
		go func() { defer guard("numeri"); statsRun(level, doneNames, total, int(dur.Seconds()), cancelled) }()
		go maybeAutoExit()
	}()
	return "ok"
}

func fmtBytes(b int64) string {
	f := float64(b)
	switch {
	case f >= 1<<30:
		return strings.Replace(fmt.Sprintf("%.2f GB", f/(1<<30)), ".", ",", 1)
	case f >= 1<<20:
		return strings.Replace(fmt.Sprintf("%.1f MB", f/(1<<20)), ".", ",", 1)
	default:
		return fmt.Sprintf("%.0f KB", f/1024)
	}
}

func analyze() {
	r := runPS(analyzeScript, 3*time.Minute, nil)
	var data map[string]interface{}
	if r.JSON != "" {
		_ = json.Unmarshal([]byte(r.JSON), &data)
	}
	emit("onAnalysis", data)
}

func hardwareInfo() {
	r := runPS(hardwareScript, 2*time.Minute, nil)
	var data map[string]interface{}
	if r.JSON != "" {
		_ = json.Unmarshal([]byte(r.JSON), &data)
	}
	emit("onHardware", data)
}

var modeMu sync.Mutex

func setMode(id string) {
	script, ok := modeScripts[id]
	if !ok {
		emit("onModeDone", map[string]interface{}{"id": id, "ok": false, "msgs": []string{"Modalità sconosciuta"}})
		return
	}
	modeMu.Lock()
	defer modeMu.Unlock()
	if !ensureSnapshot() {
		emit("onModeDone", map[string]interface{}{"id": id, "ok": false, "msgs": []string{"Non riesco a salvare com'era Windows, quindi per sicurezza non cambio niente. Riprova tra poco."}})
		return
	}
	// Le impostazioni "tue" si salvano solo se adesso il PC è in Normale (o com'era):
	// se è già in Gaming/FPS/Film salverei quelle messe da Tasso.
	setMu.Lock()
	prev := settings.Mode
	setMu.Unlock()
	canSave := "$TassoCanSave = $false\n"
	if prev == "" || prev == "normale" || prev == "originale" {
		canSave = "$TassoCanSave = $true\n"
	}
	var msgs []string
	r := runPS(canSave+script, 2*time.Minute, func(m string) { msgs = append(msgs, m) })
	if r.Err == nil {
		updateSettings(func(s *Settings) { s.Mode = id })
	}
	emit("onModeDone", map[string]interface{}{"id": id, "ok": r.Err == nil, "msgs": msgs, "settings": getSettings()})
}
