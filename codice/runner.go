package main

// Esecuzione degli script PowerShell: file temporanei, tempi massimi, messaggi di avanzamento.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

type psResult struct {
	Freed   int64
	Reboot  bool
	JSON    string
	Err     error
	TooSlow bool // fermato (tutto o in parte) perché superava il tempo massimo
	Skipped bool // il passaggio dice di essersi saltato (riga SKIPPED), es. Windows aspetta già un riavvio
	WUInst  int  // aggiornamenti di Windows installati (riga WUINST n)
}

var scriptSeq atomic.Int64

// passoConFoto: i passaggi che cambiano impostazioni di Windows coperte dalla foto di "com'era"
// (piano energetico, limite del processore, Modalità Gioco, privacy): prima di loro la foto si fa per
// forza, così "Rimetti tutto com'era" e Torna indietro li possono annullare. Senza foto non partono.
// "Pochi FPS" e "Il gioco va a scatti" prima cambiavano il piano energetico senza nessuna foto.
var passoConFoto = map[string]bool{"privacy": true, "backup": true, "fix_game_fps": true, "fix_game_lag": true}

var (
	scriptDirOnce sync.Once
	scriptDirPath string
)

// scriptDir: gli script partono come amministratore, quindi li scrivo in Programmi\Tasso\lavoro,
// dove un programma normale dell'utente non può scrivere né sostituirli prima che partano.
// Solo se non si può (caso raro) uso la cartella di Tasso in AppData, come prima.
func scriptDir() string {
	scriptDirOnce.Do(func() {
		if pf := programFilesDir(); pf != "" {
			d := filepath.Join(cartellaTasso(), "lavoro") // ecosistema.go: di base Programmi\Tasso
			if os.MkdirAll(d, 0o755) == nil {
				scriptDirPath = d
				go pulisciScriptVecchi(d)
				go pulisciTempTasso() // lavoro\tmp: i temporanei dei programmi avviati da Tasso (ambiente.go)
				return
			}
		}
		scriptDirPath = filepath.Join(appDir, "scripts")
		_ = os.MkdirAll(scriptDirPath, 0o755)
	})
	return scriptDirPath
}

// cartellaLavoroProtetta: Programmi\Tasso\lavoro se scriptDir l'ha potuta usare, "" se è nel
// ripiego in AppData (dove un programma dell'utente può scrivere).
func cartellaLavoroProtetta() string {
	d := scriptDir()
	if appDir != "" && filepath.Clean(d) == filepath.Clean(filepath.Join(appDir, "scripts")) {
		return ""
	}
	return d
}

// pulisciScriptVecchi: se Tasso è stato chiuso mentre uno script girava (o Windows non ha lasciato
// cancellare il file subito), il .ps1 resta in lavoro. Tolgo i nostri più vecchi di un giorno:
// tasso_*.ps1 (passaggi), tt_*.ps1 e tt_ocr_*.ps1 (traduzione: lettura del testo, scheda video).
// Prima toglievo solo tasso_*.ps1 e i tt_ si accumulavano a decine.
func pulisciScriptVecchi(d string) {
	defer guard("pulisciScriptVecchi")
	l, _ := filepath.Glob(filepath.Join(d, "tasso_*.ps1"))
	l2, _ := filepath.Glob(filepath.Join(d, "tt_*.ps1"))
	l = append(l, l2...)
	for _, f := range l {
		if fi, err := os.Stat(f); err == nil && !fi.IsDir() && time.Since(fi.ModTime()) > 24*time.Hour {
			_ = os.Remove(f)
		}
	}
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
	// Le cartelle vere vanno per prime (fuori dalla traduzione: sono percorsi, non frasi).
	body := "\uFEFF" + strings.ReplaceAll(psCartelle()+"\n"+trPS(psPrelude+"\n"+auto+"\n"+script), "\n", "\r\n")
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
// "FREED n" sommano lo spazio liberato, "REBOOT" segnala un riavvio, "JSON ..." restituisce dati,
// "PROG x" (da 0 a 1) dice a che punto è il passaggio (per la barra).
func runPS(script string, timeout time.Duration, onMsg func(string)) psResult {
	return runPSCtx(context.Background(), script, timeout, onMsg)
}

func runPSCtx(parent context.Context, script string, timeout time.Duration, onMsg func(string)) psResult {
	return runPSOpt(parent, script, timeout, onMsg, nil, false)
}

// psDeadline: la riga che dice allo script quanti secondi ha (Time-Left in steps.go): così i comandi
// lunghi dentro lo script si fermano da soli un po' prima che Tasso debba chiudere tutto.
func psDeadline(timeout time.Duration) string {
	s := int(timeout / time.Second)
	if s < 1 {
		s = 1
	}
	return "$TassoSecondi = " + strconv.Itoa(s) + "\n"
}

// runPSOpt: come runPSCtx; onProg riceve l'avanzamento (PROG), heartbeat=true scrive nel registro
// "sto ancora lavorando" quando il passaggio resta in silenzio a lungo (così non sembra fermo).
func runPSOpt(parent context.Context, script string, timeout time.Duration, onMsg func(string), onProg func(float64), heartbeat bool) psResult {
	return runPSInfo(parent, script, timeout, onMsg, onProg, nil, heartbeat)
}

// runPSInfo: come runPSOpt; onInfo riceve le righe "INFO ..." (la riga sotto il passaggio in corso,
// es. "Installo 3 aggiornamenti: 2 di 3 fatti").
func runPSInfo(parent context.Context, script string, timeout time.Duration, onMsg func(string), onProg func(float64), onInfo func(string), heartbeat bool) psResult {
	res := psResult{}
	path, err := writeScript(psDeadline(timeout) + script)
	if err != nil {
		res.Err = err
		return res
	}
	// dopo l'uso: subito, e se Windows lo tiene ancora aperto (antivirus che lo controlla, PowerShell
	// appena chiuso) riprovo per qualche secondo invece di lasciarlo lì
	defer togliScript(path)
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	// L'uscita la leggo da una pipe mia: se un programma aperto dallo script (dism, sfc...) restasse
	// vivo tenendola aperta, smetto di aspettarla appena PowerShell è finito o è stato fermato.
	pr, pw, err := os.Pipe()
	if err != nil {
		res.Err = err
		return res
	}
	cmd := exec.Command(sysExe("powershell.exe"), "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", path)
	// cartella di lavoro = System32: i programmi chiamati per nome (dism, sfc, reg…) non possono
	// essere presi da una cartella dove chiunque può scrivere (per es. quella di Tasso.exe)
	cmd.Dir = sistema32()
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	cmd.Stdout = pw
	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		res.Err = err
		return res
	}
	pw.Close() // la mia copia: la tiene solo PowerShell (e i suoi figli)
	// Job Object: se il passaggio va fermato (tempo scaduto o tasto Ferma) chiudo anche
	// tutti i programmi che PowerShell ha aperto (dism, sfc, defrag...), non solo PowerShell.
	job := newKillJob(cmd.Process.Pid)
	defer job.close()
	pid := cmd.Process.Pid
	// Una maniglia mia su PowerShell: finché è aperta Windows non riusa il suo numero (PID), così
	// taskkill (solo se il Job Object non c'è) non può mai colpire un altro programma.
	ph, _ := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if ph != 0 {
		defer windows.CloseHandle(ph)
	}

	lines := make(chan string, 256)
	readDone := make(chan struct{})
	quit := make(chan struct{}) // chiuso all'uscita: la lettura non resta appesa su una coda piena
	defer close(quit)
	go func() {
		defer guard("runner.go:lettura")
		defer close(readDone)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
		for sc.Scan() {
			select {
			case lines <- sc.Text():
			case <-quit:
				return
			}
		}
		if sc.Err() != nil {
			// Riga troppo lunga: svuoto il resto, altrimenti PowerShell resta bloccato sulla scrittura.
			_, _ = io.Copy(io.Discard, pr)
		}
	}()
	procDone := make(chan error, 1)
	go func() {
		defer guard("runner.go:attesa")
		procDone <- cmd.Wait()
	}()

	handle := func(raw string) {
		line := strings.TrimPrefix(strings.TrimRight(raw, "\r"), "\uFEFF")
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
		case line == "SLOW": // una parte lunga del passaggio è stata fermata perché troppo lenta
			res.TooSlow = true
		case line == "SKIPPED":
			res.Skipped = true
		case strings.HasPrefix(line, "WUINST "):
			if n, e := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "WUINST "))); e == nil && n > 0 {
				res.WUInst += n
			}
		case strings.HasPrefix(line, "INFO "):
			if onInfo != nil {
				onInfo(strings.TrimPrefix(line, "INFO "))
			}
		case strings.HasPrefix(line, "JSON "):
			res.JSON = strings.TrimPrefix(line, "JSON ")
		case strings.HasPrefix(line, "PROG "):
			if f, e := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line, "PROG ")), 64); e == nil && onProg != nil && f >= 0 && f <= 1 {
				onProg(f)
			}
		}
	}

	start := time.Now()
	lastOut := start
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	stopped := false // fermato da me (tempo scaduto o Ferma)
	var waitErr error
	exited := false
	ctxDone := ctx.Done()
loop:
	for {
		select {
		case l := <-lines:
			lastOut = time.Now()
			handle(l)
		case <-readDone:
			// Fine dell'uscita: aspetto PowerShell (al massimo 30 s, poi lo chiudo io).
			for !exited {
				select {
				case waitErr = <-procDone:
					exited = true
				case <-ctxDone:
					ctxDone = nil
					stopped = true
					killTree(job, pid, ph, cmd)
				case <-time.After(30 * time.Second):
					killTree(job, pid, ph, cmd)
				}
			}
			break loop
		case waitErr = <-procDone:
			exited = true
			// PowerShell è finito: leggo quello che resta, ma non aspetto per sempre un nipote che
			// tiene aperta la pipe (lo chiudo solo se il passaggio era già da fermare).
			grace := time.After(5 * time.Second)
		drain:
			for {
				select {
				case l := <-lines:
					handle(l)
				case <-readDone:
					break drain
				case <-grace:
					break drain
				}
			}
			break loop
		case <-ctxDone:
			ctxDone = nil
			stopped = true
			killTree(job, pid, ph, cmd)
		case <-tick.C:
			if stopped {
				// Chiuso ma ancora qui dopo 15 s: riprovo a chiudere l'albero.
				killTree(job, pid, ph, cmd)
				continue
			}
			if heartbeat && onMsg != nil && time.Since(lastOut) >= 90*time.Second {
				lastOut = time.Now()
				el := int(time.Since(start).Minutes())
				mx := int(timeout.Minutes())
				onMsg(fmt.Sprintf(trGo("… sto ancora lavorando su questo passaggio (%d min su un massimo di %d, poi lo salto)."), el, mx))
			}
		}
	}
	// Le righe già arrivate e non ancora lette (la lettura le mette in coda prima di finire).
	for more := true; more; {
		select {
		case l := <-lines:
			handle(l)
		default:
			more = false
		}
	}
	_ = pr.Close() // se un nipote la tiene aperta, la lettura in sospeso finisce da sola quando lui esce
	res.Err = waitErr
	if stopped || ctx.Err() != nil {
		if ctx.Err() == context.DeadlineExceeded {
			res.TooSlow = true
			if onMsg != nil {
				onMsg(fmt.Sprintf(trGo("Saltato perché troppo lento: dopo %d minuti l'ho fermato e vado avanti con il prossimo."), int(timeout.Minutes()+0.5)))
			}
		} else if onMsg != nil {
			onMsg("Passaggio fermato.")
		}
	}
	return res
}

// killTree chiude PowerShell e tutti i programmi che ha aperto. Prima il Job Object (prende anche i
// nipoti); se Windows non l'aveva accettato, taskkill /T sul PID di PowerShell (solo il suo albero).
func killTree(job *killJob, pid int, ph windows.Handle, cmd *exec.Cmd) {
	if job.kill() {
		return
	}
	// taskkill solo se PowerShell è ancora vivo (e la mia maniglia tiene fermo il suo PID).
	if ph == 0 {
		_ = cmd.Process.Kill()
		return
	}
	if ev, _ := windows.WaitForSingleObject(ph, 0); ev != uint32(windows.WAIT_TIMEOUT) {
		return
	}
	tk := exec.Command(sysExe("taskkill.exe"), "/T", "/F", "/PID", strconv.Itoa(pid))
	tk.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if tk.Start() == nil {
		done := make(chan struct{})
		go func() { _ = tk.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			_ = tk.Process.Kill()
		}
	}
	_ = cmd.Process.Kill()
}

var (
	runMu     sync.Mutex
	running   bool
	cancelReq atomic.Bool
)

// claimRun prende il "posto" della pulizia per un lavoro lungo che cambia Windows (Rimetti com'era,
// Torna indietro): così una pulizia programmata non può partire nel mezzo. false = c'è già qualcosa.
func claimRun() bool {
	runMu.Lock()
	defer runMu.Unlock()
	if running || updating.Load() {
		return false
	}
	running = true
	return true
}

func releaseRun() {
	runMu.Lock()
	running = false
	runMu.Unlock()
}

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

func startLevel(level int, fine string, winupdate bool) string {
	if level < 1 || level > 5 {
		return "livello non valido"
	}
	var ids []string
	ufficio := officeMode()
	for _, s := range allSteps {
		// 3.1: in Ufficio niente pulizia delle cache dei giochi (come nella lista dei passaggi).
		if ufficio && s.ID == "game_cache" {
			continue
		}
		if s.Level > 0 && s.Level <= level {
			ids = append(ids, s.ID)
		}
	}
	return startSteps(ids, level, fine, winupdate)
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
	runSingle   atomic.Bool // un solo passaggio scelto da te, con "Quando ha finito: Lascia acceso"
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

// startSteps avvia la pulizia. fine: "acceso", "riavvia" o "spegni" (Quando ha finito);
// winupdate=false toglie gli aggiornamenti di Windows ("Aggiorna anche Windows" tolto).
func startSteps(ids []string, level int, fine string, winupdate bool) string {
	fine = sceltaFine(fine)
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
	if !winupdate && len(sel) > 1 {
		sel = senzaPasso(sel, "winupdate")
	}
	sel = winupdatePerUltimo(sel) // 3.1.1: Windows Update sempre per ultimo
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
	runSingle.Store((len(sel) == 1 || level == 0) && fine == fineAcceso) // Risolvi (livello 0): si può chiedere/aprire come per un passaggio solo
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
					"done": 0, "total": len(sel), "log": "", "shutdown": false, "restart": false,
				})
			}
		}()
		// Il PC resta sveglio fino alla fine (anche se la pulizia va in errore o la fermi).
		sveglio := tienSveglio()
		defer sveglio()
		defer func() {
			for _, x := range sel {
				if x.ID == "wipe_free" {
					cleanWipeTemp() // anche se interrotto o scaduto: i file di riempimento non devono restare
					break
				}
			}
			endRunCtx()
			runAuto.Store(false)
			runSingle.Store(false)
			runMu.Lock()
			running = false
			runMu.Unlock()
		}()
		logPath := filepath.Join(appDir, "logs", "tasso_"+time.Now().Format("20060102_150405")+".txt")
		lf, _ := apriLogApp(logPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC) // come os.Create, senza giunzioni
		defer func() {
			if lf != nil {
				lf.Close()
			}
		}()
		logLine := func(s string) {
			line := time.Now().Format("15:04:05") + "  " + trGo(s) // il file di registro nella lingua di Tasso
			if lf != nil {
				fmt.Fprintln(lf, line)
			}
			emit("onLog", s)
		}

		list := make([]map[string]interface{}, 0, len(sel))
		for _, s := range sel {
			list = append(list, map[string]interface{}{"id": s.ID, "name": s.Name, "minutes": s.Minutes})
		}
		emit("onRunStart", map[string]interface{}{"level": level, "steps": list, "end": fine})
		logLine(fmt.Sprintf(trGo("Tasso %s - inizio (%d passaggi)"), appVersion, len(sel)))

		snapOK := true
		for _, s := range sel {
			if passoConFoto[s.ID] {
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
		slow := 0 // passaggi saltati perché troppo lenti
		freePrima := spazioLiberoDischi()
		reboot := false
		done := 0
		var doneNames []string
		var passi []passoFatto // per il "Bentornato"
		wuInst := 0
		wuCacheSaltata := false
		for i, s := range sel {
			if cancelReq.Load() {
				logLine("Interrotto da te: i passaggi rimanenti sono stati saltati.")
				break
			}
			emit("onStepStart", map[string]interface{}{"index": i})
			logLine("» " + s.Name)
			if passoConFoto[s.ID] && s.ID != "backup" && !snapOK {
				// Senza la foto di com'era Windows non si potrebbe tornare indietro: lo salto.
				logLine("   Saltato: non sono riuscito a salvare com'era Windows, quindi per sicurezza non cambio niente.")
				passi = append(passi, passoFatto{Nome: s.Name, Saltato: true})
				done++
				emit("onStepDone", map[string]interface{}{"index": i, "freed": 0})
				continue
			}
			idx := i
			lastProg := -1.0
			r := eseguiPasso(runCtx(), s, func(m string) { logLine("   " + m) }, func(f float64) {
				if f-lastProg >= 0.01 || f >= 1 { // al massimo 100 aggiornamenti per passaggio
					lastProg = f
					emit("onStepProgress", map[string]interface{}{"index": idx, "frac": f})
				}
			}, func(t string) {
				emit("onStepInfo", map[string]interface{}{"index": idx, "text": t})
			})
			if r.TooSlow {
				slow++
			}
			if r.Err != nil && !cancelReq.Load() && r.Freed == 0 {
				logLine("   (dettaglio tecnico: " + r.Err.Error() + ")")
			}
			total += r.Freed
			if r.Reboot {
				reboot = true
			}
			if s.ID == "winupdate" {
				wuInst += r.WUInst
			}
			if s.ID == "wu_cache" && r.Skipped {
				wuCacheSaltata = true // Windows stava aggiornando: il riepilogo lo ricorda (Tasso 3.1.1)
			}
			passi = append(passi, passoFatto{Nome: s.Name, Liberato: r.Freed, Saltato: r.TooSlow || r.Skipped})
			done++
			doneNames = append(doneNames, s.Name)
			emit("onStepDone", map[string]interface{}{"index": i, "freed": r.Freed})
		}
		// Spazio liberato onesto: oltre alla somma di quello che i passaggi dicono di aver tolto,
		// misuro lo spazio libero dei dischi prima e dopo (DISM, Ottimizzazione recapito, punti di
		// ripristino e Windows stesso liberano spazio che i passaggi non sanno contare). Aspetto un
		// attimo perché Windows finisca di liberarlo. Mai un numero negativo (spazioOnesto).
		somma := total
		if len(freePrima) > 0 {
			select {
			case <-runCtx().Done():
			case <-time.After(5 * time.Second):
			}
			reale := diffSpazioLibero(freePrima, spazioLiberoDischi())
			total = spazioOnesto(somma, reale)
			if total != somma {
				logLine(fmt.Sprintf(trGo("Spazio liberato: i passaggi ne hanno contato %s, sui dischi ne è tornato libero %s: ti dico quello vero dei dischi."), fmtBytes(somma), fmtBytes(reale)))
			}
		}
		if slow > 0 {
			logLine(fmt.Sprintf(trGo("Passaggi fermati prima della fine perché troppo lenti: %d (li puoi rifare da soli più tardi)."), slow))
		}
		dur := time.Since(start)
		logLine(fmt.Sprintf(trGo("Fine: liberati %s in %s"), fmtBytes(total), dur.Round(time.Second)))
		if level > 0 && done == len(sel) {
			updateSettings(func(st *Settings) {
				st.LastClean = &LastClean{Date: time.Now().Format("2006-01-02"), Level: level, Freed: total}
			})
			discordAfterClean(total)
		}
		if benchOn && !cancelReq.Load() {
			logLine("Voto al PC dopo la pulizia…")
			after := runBench("dopo")
			logLine(fmt.Sprintf(trGo("Leggerezza di Windows: %d → %d su 100. Velocità dei componenti: %d → %d su 100."), before.Light, after.Light, score100(before.Score), score100(after.Score)))
			emit("onBench", map[string]interface{}{"result": after, "before": before, "history": benchHistory()})
		}
		cancelled := cancelReq.Load()
		shutdown, restart, fineErr := false, false, false
		if fine != fineAcceso && !cancelled {
			riavvia := fine == fineRiavvia
			if riavvia {
				logLine("Quando ha finito: riavvio. Il PC si riavvia tra 60 secondi (puoi annullarlo da Tasso).")
			} else {
				logLine("Quando ha finito: spengo. Il PC si spegne tra 60 secondi (puoi annullarlo da Tasso).")
			}
			msg := trGo("Tasso ha finito la manutenzione: il PC si spegne tra 60 secondi.")
			if riavvia {
				msg = trGo("Tasso ha finito la manutenzione: il PC si riavvia tra 60 secondi.")
			}
			riep := riepilogoPulizia{
				Livello: level, Fine: fine, FinitaAlle: time.Now().Format("15:04"), Quando: time.Now().Unix(),
				Liberato: total, Aggiornamenti: wuInst, Fatti: done, Totale: len(sel), Passi: passi,
			}
			if pianificaFine(riavvia, fineSecondi, msg) {
				restart, shutdown = riavvia, !riavvia
				salvaBentornato(riep)
			} else {
				fineErr = true
				bentornatoInSospeso(riep) // se poi "Riavvia adesso" parte, il Bentornato c'è lo stesso
				logLine("Windows non ha accettato il riavvio o lo spegnimento: lo puoi chiedere da Tasso con il pulsante.")
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
			"restart":   restart,
			"end":       fine,
			"endErr":    fineErr,
			"endSecs":   fineSecondi,
			"updates":   wuInst,
			"wuSkipped": wuCacheSaltata,
			"settings":  getSettings(),
		})
		go func() { defer guard("numeri"); statsRun(level, doneNames, total, int(dur.Seconds()), cancelled) }()
		go maybeAutoExit()
	}()
	return "ok"
}

// spazioLiberoDischi: lo spazio libero di ogni disco fisso (C:, D:...), per lettera.
func spazioLiberoDischi() map[string]uint64 {
	out := map[string]uint64{}
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return out
	}
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		rp, err := windows.UTF16PtrFromString(root)
		if err != nil || windows.GetDriveType(rp) != windows.DRIVE_FIXED {
			continue
		}
		var free, tot, totFree uint64
		if windows.GetDiskFreeSpaceEx(rp, &free, &tot, &totFree) == nil {
			out[root] = totFree
		}
	}
	return out
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

// lettura: un controllo lungo (PowerShell) che risponde alla pagina con un emit. Ne gira uno solo
// alla volta; dopo un cambio di lingua (riusa) la pagina nuova riceve l'ultimo risultato già pronto
// invece di far ripartire un altro PowerShell (i18n.go: la pagina si ricarica a ogni cambio).
type lettura struct {
	mu      sync.Mutex
	inCorso bool
	ancora  bool // chiesta di nuovo mentre girava (es. dopo una pulizia): rifaccio una volta
	dati    map[string]interface{}
}

func (l *lettura) chiedi(riusa bool, evento string, f func() map[string]interface{}) {
	l.mu.Lock()
	if l.inCorso { // il risultato arriverà comunque alla pagina di adesso
		l.ancora = l.ancora || !riusa
		l.mu.Unlock()
		return
	}
	if riusa && l.dati != nil {
		d := l.dati
		l.mu.Unlock()
		emit(evento, d)
		return
	}
	l.inCorso = true
	l.mu.Unlock()
	go func() {
		defer guard("runner.go:lettura " + evento)
		defer func() { // anche se f si ferma con un errore, la prossima richiesta può ripartire
			l.mu.Lock()
			l.inCorso, l.ancora = false, false
			l.mu.Unlock()
		}()
		for {
			d := f()
			l.mu.Lock()
			if d != nil {
				l.dati = d
			}
			di := l.ancora
			l.ancora = false
			l.mu.Unlock()
			if !di {
				emit(evento, d)
				return
			}
		}
	}()
}

var letturaAnalisi, letturaHardware lettura

func analyze(riusa bool) {
	letturaAnalisi.chiedi(riusa, "onAnalysis", func() map[string]interface{} {
		r := runPS(analyzeScript, 3*time.Minute, nil)
		var data map[string]interface{}
		if r.JSON != "" {
			_ = json.Unmarshal([]byte(r.JSON), &data)
		}
		return data
	})
}

func hardwareInfo(riusa bool) {
	letturaHardware.chiedi(riusa, "onHardware", leggiHardware)
}

func leggiHardware() map[string]interface{} {
	r := runPS(hardwareScript, 2*time.Minute, nil)
	var data map[string]interface{}
	if r.JSON != "" {
		_ = json.Unmarshal([]byte(r.JSON), &data)
	}
	// Risoluzione e Hz dello schermo principale: Windows li dà giusti per ogni schermo, mentre WMI
	// ne dà uno solo per scheda video (spesso quello sbagliato con più monitor).
	if data != nil {
		for _, s := range screens() {
			if s.Primary {
				data["screen"] = map[string]interface{}{"res": fmt.Sprintf("%dx%d", s.W, s.H), "hz": s.Hz}
			}
		}
	}
	return data
}

var modeMu sync.Mutex

// setMode cambia modalità e dice se ci è riuscita.
func setMode(id string) bool {
	script, ok := modeScripts[id]
	if !ok {
		emit("onModeDone", map[string]interface{}{"id": id, "ok": false, "msgs": []string{"Modalità sconosciuta"}})
		return false
	}
	modeMu.Lock()
	defer modeMu.Unlock()
	if !ensureSnapshot() {
		emit("onModeDone", map[string]interface{}{"id": id, "ok": false, "msgs": []string{"Non riesco a salvare com'era Windows, quindi per sicurezza non cambio niente. Riprova tra poco."}})
		return false
	}
	// Ottimizza tutto: se i fumetti delle notifiche sono in pausa per una partita, li rimetto com'erano
	// prima che Save-Prev li salvi spenti come "tuoi" (tornando a Normale resterebbero spenti per sempre).
	// Con una modalità per i giochi accesa la pausa non riparte: le notifiche le gestisce lei.
	ottNotificheFine()
	// Le impostazioni "tue" si salvano solo se adesso il PC è in Normale (o com'era):
	// se è già in Gaming/FPS/Film salverei quelle messe da Tasso.
	setMu.Lock()
	prev := settings.Mode
	setMu.Unlock()
	canSave := "$TassoCanSave = $false\n"
	if prev == "" || prev == "normale" || prev == "originale" {
		canSave = "$TassoCanSave = $true\n"
	}
	// Se il Boost ha tolto l'accelerazione del mouse (solo finché gioca), Save-Prev deve salvare
	// quella di prima, non lo 0,0,0 di adesso: se no "Normale" la spegnerebbe per sempre.
	mousePrev := "$TassoMousePrev = ''\n"
	bMu.Lock()
	if m := loadBoost().Mouse; m != nil {
		mousePrev = fmt.Sprintf("$TassoMousePrev = '%d,%d,%d'\n", m[0], m[1], m[2])
	}
	bMu.Unlock()
	// Undo-FpsMouse rimette l'accelerazione solo uscendo da FPS competitivo (che l'aveva spenta).
	fromFps := "$TassoFromFps = $false\n"
	if prev == "fps" {
		fromFps = "$TassoFromFps = $true\n"
	}
	// Modalità riunione accesa: notifiche spente e piano al massimo li ha messi lei. Save-Prev salva
	// quelli di prima della riunione (dal suo file) e nessuna modalità riaccende le notifiche.
	riunione := riunionePrima(fileUfficio("riunione"))
	if strings.HasPrefix(riunione, "$TassoInMeeting = $true") && !strings.Contains(riunione, "$TassoMeetOk = $true") {
		canSave = "$TassoCanSave = $false\n" // file illeggibile: non so com'era, non salvo niente come "tuo"
	}
	var msgs []string
	r := runPS(canSave+mousePrev+fromFps+riunione+script, 2*time.Minute, func(m string) { msgs = append(msgs, m) })
	if r.Err == nil {
		updateSettings(func(s *Settings) { s.Mode = id })
	}
	emit("onModeDone", map[string]interface{}{"id": id, "ok": r.Err == nil, "msgs": msgs, "settings": getSettings()})
	return r.Err == nil
}

// riunionePrima: le variabili per gli script delle modalità con la Modalità riunione accesa.
// $TassoMeetToast e $TassoMeetPlan sono le notifiche e il piano di prima della riunione, letti dal
// suo file e controllati (solo 0/1 e un GUID: niente altro finisce nello script).
func riunionePrima(f string) string {
	b, err := os.ReadFile(f)
	if err != nil {
		return "$TassoInMeeting = $false\n$TassoMeetOk = $false\n$TassoMeetToast = '1'\n$TassoMeetPlan = ''\n$TassoMeetFile = ''\n"
	}
	file := "$TassoMeetFile = '" + strings.ReplaceAll(f, "'", "''") + "'\n"
	var st struct {
		Toast interface{} `json:"toast"`
		Plan  string      `json:"plan"`
	}
	if json.Unmarshal(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")), &st) != nil {
		return "$TassoInMeeting = $true\n$TassoMeetOk = $false\n$TassoMeetToast = '0'\n$TassoMeetPlan = ''\n" + file
	}
	// toast vuoto: a fine riunione Fine-Riunione toglie il valore e Windows tiene le notifiche accese
	toast := "1"
	if v := fmt.Sprint(st.Toast); v == "0" || v == "1" {
		toast = v
	}
	plan := ""
	if guidPiano.MatchString(st.Plan) {
		plan = st.Plan
	}
	return "$TassoInMeeting = $true\n$TassoMeetOk = $true\n$TassoMeetToast = '" + toast + "'\n$TassoMeetPlan = '" + plan + "'\n" + file
}

var guidPiano = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// togliScript: cancella lo script; se il file è ancora aperto da qualcuno riprova in sottofondo
// (fino a circa un minuto). Quelli che restano li toglie pulisciScriptVecchi al prossimo avvio.
func togliScript(p string) {
	if os.Remove(p) == nil || !esisteFile(p) {
		return
	}
	go func() {
		defer guard("togliScript")
		for i := 0; i < 12; i++ {
			time.Sleep(time.Duration(250*(i+1)) * time.Millisecond)
			if os.Remove(p) == nil || !esisteFile(p) {
				return
			}
		}
	}()
}

func esisteFile(p string) bool { _, err := os.Stat(p); return err == nil }
