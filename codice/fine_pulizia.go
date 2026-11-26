package main

// 3.1.1: la parte Windows di "Quando ha finito": PC sveglio durante la pulizia, batteria,
// riavvio/spegnimento che parte davvero (shutdown.exe con riprova, poi ExitWindowsEx) e il
// riepilogo del "Bentornato".

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pSetThreadExecState = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetThreadExecutionState")
	pGetTickCount64     = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetTickCount64")
)

const (
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
)

// tienSveglio: il PC non va in sospensione finché la pulizia lavora (lo schermo invece si può
// spegnere). SetThreadExecutionState vale per il thread che la chiama: per questo una goroutine
// tutta sua, ferma sullo stesso thread fino alla fine. Se Tasso si chiude o va in crash, Windows
// toglie la richiesta da solo (finisce con il processo). La funzione restituita la toglie.
func tienSveglio() func() {
	stop := make(chan struct{})
	pronto := make(chan struct{})
	go func() {
		defer guard("tienSveglio")
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		_, _, _ = pSetThreadExecState.Call(uintptr(esContinuous | esSystemRequired))
		close(pronto)
		<-stop
		_, _, _ = pSetThreadExecState.Call(uintptr(esContinuous))
	}()
	select {
	case <-pronto:
	case <-time.After(2 * time.Second):
	}
	var once sync.Once
	return func() { once.Do(func() { close(stop) }) }
}

// statoBatteria: il portatile va a batteria (senza caricatore) e a che percentuale (-1 = non si sa).
func statoBatteria() map[string]interface{} {
	var st struct {
		ACLineStatus, BatteryFlag, BatteryLifePercent, SystemStatusFlag byte
		BatteryLifeTime, BatteryFullLifeTime                            uint32
	}
	r, _, _ := pGetPowerStatus.Call(uintptr(unsafe.Pointer(&st)))
	pct := -1
	if r != 0 && st.BatteryLifePercent <= 100 {
		pct = int(st.BatteryLifePercent)
	}
	return map[string]interface{}{"battery": r != 0 && st.ACLineStatus == 0, "pct": pct}
}

// shutdownExe esegue shutdown.exe nascosto e restituisce il codice di uscita (-1 = non partito).
func shutdownExe(args ...string) int {
	cmd := exec.Command(sysExe("shutdown.exe"), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return -1
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			return 0
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		return -1
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
		return -1
	}
}

// Il conto alla rovescia di Tasso (quando shutdown.exe non è stato accettato, o come riserva se
// l'ha accettato ma Windows non parte): alla fine ExitWindowsEx. Si annulla con cancelShutdown
// (Annulla). Finché il conto è in corso il PC resta sveglio: dopo ore senza nessuno davanti, appena
// la pulizia toglieva la richiesta Windows poteva sospendersi durante i 60 secondi e il riavvio non
// partiva più (si trovava il PC addormentato, con gli aggiornamenti da finire).
var (
	fineMu      sync.Mutex
	fineTimer   *time.Timer
	fineSveglio func()
)

func annullaFineTasso() {
	fineMu.Lock()
	if fineTimer != nil {
		fineTimer.Stop()
		fineTimer = nil
	}
	if fineSveglio != nil {
		fineSveglio()
		fineSveglio = nil
	}
	fineMu.Unlock()
}

// armaFine: tra "dopo" riavvia (riavvia=true) o spegne con ExitWindowsEx, se Tasso è ancora vivo.
func armaFine(riavvia bool, dopo time.Duration) {
	annullaFineTasso()
	sv := tienSveglio()
	fineMu.Lock()
	fineSveglio = sv
	var t *time.Timer
	t = time.AfterFunc(dopo, func() {
		defer guard("fineTasso")
		_ = exitWindows(riavvia)
		// Se Windows non è partito lascio dormire il PC tra 2 minuti (la finestra dice "non è partito").
		time.Sleep(2 * time.Minute)
		fineMu.Lock()
		if fineTimer == t {
			fineTimer = nil
			if fineSveglio != nil {
				fineSveglio()
				fineSveglio = nil
			}
		}
		fineMu.Unlock()
	})
	fineTimer = t
	fineMu.Unlock()
}

// exitWindows riavvia o spegne subito con ExitWindowsEx (serve il privilegio di spegnimento).
func exitWindows(riavvia bool) error {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &tok); err != nil {
		return err
	}
	defer tok.Close()
	var luid windows.LUID
	name, _ := windows.UTF16PtrFromString("SeShutdownPrivilege")
	if err := windows.LookupPrivilegeValue(nil, name, &luid); err != nil {
		return err
	}
	tp := windows.Tokenprivileges{PrivilegeCount: 1}
	tp.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
	if err := windows.AdjustTokenPrivileges(tok, false, &tp, 0, nil, nil); err != nil {
		return err
	}
	fl := uint32(windows.EWX_POWEROFF | windows.EWX_FORCEIFHUNG)
	if riavvia {
		fl = windows.EWX_REBOOT | windows.EWX_FORCEIFHUNG
	}
	return windows.ExitWindowsEx(fl, windows.SHTDN_REASON_MAJOR_OPERATINGSYSTEM|windows.SHTDN_REASON_MINOR_UPGRADE|windows.SHTDN_REASON_FLAG_PLANNED)
}

func fineDepsVeri(riavvia bool) fineDeps {
	return fineDeps{
		shutdown:    shutdownExe,
		puoSpegnere: puoSpegnere,
		arma:        func(dopo time.Duration) { armaFine(riavvia, dopo) },
		esci:        func() error { return exitWindows(riavvia) },
	}
}

// pianificaFine: riavvia (riavvia=true) o spegne tra secs secondi (vedi pianificaFineCon).
func pianificaFine(riavvia bool, secs int, msg string) bool {
	annullaFineTasso()
	return pianificaFineCon(fineDepsVeri(riavvia), riavvia, secs, msg)
}

// adessoFine: "Riavvia adesso" / "Spegni adesso": tolgo quello programmato e chiedo subito.
func adessoFine(riavvia bool) bool {
	annullaFineTasso()
	if !adessoFineCon(fineDepsVeri(riavvia), riavvia) {
		return false
	}
	salvaBentornatoInSospeso()
	return true
}

// puoSpegnere: il privilegio di spegnimento si può abilitare (senza spegnere niente).
func puoSpegnere() bool {
	var tok windows.Token
	if windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &tok) != nil {
		return false
	}
	defer tok.Close()
	var luid windows.LUID
	name, _ := windows.UTF16PtrFromString("SeShutdownPrivilege")
	return windows.LookupPrivilegeValue(nil, name, &luid) == nil
}

// ---------- Bentornato ----------

func bentornatoFile() string { return filepath.Join(appDir, "bentornato.json") }

func salvaBentornato(r riepilogoPulizia) {
	b, err := json.Marshal(r)
	if err == nil {
		_ = scriviFileApp(bentornatoFile(), b, 0o644)
	}
}

func togliBentornato() {
	bentMu.Lock()
	bentSospeso = nil
	bentMu.Unlock()
	_ = os.Remove(bentornatoFile())
}

// Il riepilogo di una pulizia con "Riavvia"/"Spegni" che Windows non aveva accettato: se poi premi
// "Riavvia adesso" e questa volta parte, al ritorno c'è lo stesso il "Bentornato".
var (
	bentMu      sync.Mutex
	bentSospeso *riepilogoPulizia
)

func bentornatoInSospeso(r riepilogoPulizia) {
	bentMu.Lock()
	bentSospeso = &r
	bentMu.Unlock()
}

func salvaBentornatoInSospeso() {
	bentMu.Lock()
	r := bentSospeso
	bentSospeso = nil
	bentMu.Unlock()
	if r != nil {
		salvaBentornato(*r)
	}
}

// avvioWindows: quando si è acceso Windows l'ultima volta (Unix).
func avvioWindows() int64 {
	ms, _, _ := pGetTickCount64.Call()
	return time.Now().Add(-time.Duration(ms) * time.Millisecond).Unix()
}

// leggiBentornato: il riepilogo da mostrare una volta sola (poi il file si cancella); nil se non
// c'è o se il PC non è stato riavviato/spento dopo la pulizia.
func leggiBentornato() map[string]interface{} {
	b, err := os.ReadFile(bentornatoFile())
	if err != nil {
		return nil
	}
	var r riepilogoPulizia
	if json.Unmarshal(b, &r) != nil {
		togliBentornato()
		return nil
	}
	now := time.Now().Unix()
	if !bentornatoDaMostrare(r, avvioWindows(), now) {
		if now-r.Quando > 7*24*3600 || r.Quando <= 0 {
			togliBentornato()
		}
		return nil // (ancora) nessun riavvio: lo tengo per il prossimo avvio
	}
	togliBentornato()
	vista, altri, saltati := passiInVista(r.Passi, 6)
	return map[string]interface{}{
		"level": r.Livello, "end": r.Fine, "at": r.FinitaAlle, "freed": r.Liberato, "updates": r.Aggiornamenti,
		"done": r.Fatti, "total": r.Totale, "steps": vista, "others": altri, "skipped": saltati,
	}
}
