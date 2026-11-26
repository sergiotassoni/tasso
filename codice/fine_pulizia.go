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
	"strconv"
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

// Il conto alla rovescia di Tasso, quando shutdown.exe non è stato accettato: alla fine
// ExitWindowsEx. Si annulla con cancelShutdown (Annulla).
var (
	fineMu    sync.Mutex
	fineTimer *time.Timer
)

func annullaFineTasso() {
	fineMu.Lock()
	if fineTimer != nil {
		fineTimer.Stop()
		fineTimer = nil
	}
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

// pianificaFine: riavvia (riavvia=true) o spegne tra secs secondi. Prima shutdown.exe (con
// riprova se ce n'era già uno programmato); se Windows non lo accetta, il conto alla rovescia lo
// tiene Tasso e alla fine usa ExitWindowsEx. false = non si è potuto (la finestra lo dice).
func pianificaFine(riavvia bool, secs int, msg string) bool {
	annullaFineTasso()
	k := "/s"
	if riavvia {
		k = "/r"
	}
	args := []string{k, "/t", strconv.Itoa(secs), "/c", msg}
	e := shutdownConRiprova(shutdownExe, args)
	if e.OK {
		return true
	}
	// shutdown.exe non ha accettato (es. 1115 spegnimento già in corso, Windows Update al lavoro):
	// conto alla rovescia mio, poi ExitWindowsEx. Controllo subito che il privilegio ci sia.
	if !puoSpegnere() {
		return false
	}
	fineMu.Lock()
	fineTimer = time.AfterFunc(time.Duration(secs)*time.Second, func() {
		defer guard("fineTasso")
		_ = exitWindows(riavvia)
	})
	fineMu.Unlock()
	return true
}

// adessoFine: "Riavvia adesso" / "Spegni adesso": tolgo quello programmato e chiedo subito.
func adessoFine(riavvia bool) bool {
	annullaFineTasso()
	k := "/s"
	if riavvia {
		k = "/r"
	}
	e := shutdownConRiprova(shutdownExe, []string{k, "/t", "0"})
	if e.OK {
		return true
	}
	return exitWindows(riavvia) == nil
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

func togliBentornato() { _ = os.Remove(bentornatoFile()) }

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
