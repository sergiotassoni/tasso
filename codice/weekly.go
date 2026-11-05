package main

// Pulizia automatica settimanale: giorno, ora e livello a scelta. Se a quell'ora il PC
// era spento, la pulizia parte appena lo riaccendi. Tasso si apre ridotto a icona e,
// se l'aveva aperto l'attività, si richiude da solo a fine pulizia.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows"
)

// weeklyPlan: giorno (0 domenica ... 6 sabato), ora, livello (1 Classica, 2 Normale, 3 Media)
// e ogni quante settimane (1 o 2).
func weeklyPlan() (day, hour, level, every int) {
	setMu.Lock()
	defer setMu.Unlock()
	day, hour, level, every = settings.WeeklyDay, settings.WeeklyHour, settings.WeeklyLevel, settings.WeeklyEvery
	if day < 0 || day > 6 {
		day = 0
	}
	if hour < 1 || hour > 23 {
		hour = 20
	}
	if level < 1 || level > 5 {
		level = 2
	}
	if every != 2 {
		every = 1
	}
	return
}

func setWeekly(on bool) string {
	if on {
		day, hour, level, every := weeklyPlan()
		if taskExe() == "" {
			return "Non sono riuscito a programmare la pulizia settimanale: non posso copiare Tasso nella cartella Programmi."
		}
		days := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
		var out []string
		runPSArgs(weeklyTaskScript, map[string]interface{}{
			"exe": taskExe(), "day": days[day], "at": fmt.Sprintf("%02d:00", hour), "level": level, "every": every,
		}, 2*time.Minute, func(m string) { out = append(out, m) })
		if !strings.Contains(strings.Join(out, "\n"), "TASK_OK") {
			return "Non sono riuscito a programmare la pulizia settimanale."
		}
	} else {
		_, _ = runHidden("schtasks.exe", "/Delete", "/TN", taskWeek, "/F")
	}
	updateSettings(func(s *Settings) { s.Weekly = on })
	return "ok"
}

// setWeeklyPlan cambia giorno, ora e livello e, se è accesa, riprogramma l'attività.
func setWeeklyPlan(day, hour, level, every int) string {
	if day < 0 || day > 6 || hour < 1 || hour > 23 || level < 1 || level > 5 || (every != 1 && every != 2) {
		return "Scelta non valida."
	}
	updateSettings(func(s *Settings) { s.WeeklyDay, s.WeeklyHour, s.WeeklyLevel, s.WeeklyEvery = day, hour, level, every })
	setMu.Lock()
	on := settings.Weekly && !settings.Paused
	setMu.Unlock()
	if on {
		return setWeekly(true)
	}
	return "ok"
}

const weeklyTaskScript = `
try {
  $act = New-ScheduledTaskAction -Execute ([string]$A.exe) -Argument ('--auto=' + [int]$A.level + ' --min --scheduled')
  $trg = New-ScheduledTaskTrigger -Weekly -WeeksInterval ([int]$A.every) -DaysOfWeek ([string]$A.day) -At ([string]$A.at)
  $set = New-ScheduledTaskSettingsSet -StartWhenAvailable -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit ([TimeSpan]::Zero) -MultipleInstances IgnoreNew
  $who = [Security.Principal.WindowsIdentity]::GetCurrent().Name
  $prn = New-ScheduledTaskPrincipal -UserId $who -LogonType Interactive -RunLevel Highest
  Register-ScheduledTask -TaskPath '\Tasso\' -TaskName 'Pulizia settimanale' -Action $act -Trigger $trg -Settings $set -Principal $prn -Force -ErrorAction Stop | Out-Null
  Say 'TASK_OK'
} catch { Say ('Errore: ' + $_.Exception.Message) }
`

// ---------------- Pulizia programmata con Tasso già aperto ----------------

var scheduledRun atomic.Bool

func autoReqPath() string { return filepath.Join(appDir, "pulizia-richiesta.txt") }

// askRunningInstance scrive il livello richiesto e sveglia la copia di Tasso già aperta.
func askRunningInstance(level string) bool {
	if n, err := strconv.Atoi(level); err != nil || n < 1 || n > 5 {
		return false
	}
	appDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "Tasso")
	if os.WriteFile(autoReqPath(), []byte(level), 0o600) != nil {
		return false
	}
	name, _ := windows.UTF16PtrFromString("Local\\TassoAutoRun")
	ev, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(ev)
	return windows.SetEvent(ev) == nil
}

// listenAutoRequests: la copia aperta aspetta le richieste e avvia la pulizia nella sua finestra.
func listenAutoRequests() {
	name, _ := windows.UTF16PtrFromString("Local\\TassoAutoRun")
	ev, err := windows.CreateEvent(nil, 0, 0, name)
	if err != nil {
		return
	}
	go func() {
		defer guard("weekly.go:119")
		for {
			if r, _ := windows.WaitForSingleObject(ev, windows.INFINITE); r != windows.WAIT_OBJECT_0 {
				return
			}
			b, err := os.ReadFile(autoReqPath())
			_ = os.Remove(autoReqPath())
			if err != nil {
				continue
			}
			lvl, _ := strconv.Atoi(strings.TrimSpace(string(b)))
			setMu.Lock()
			paused := settings.Paused
			setMu.Unlock()
			if paused || lvl < 1 || lvl > 5 {
				continue
			}
			if isRunning() {
				continue // sta già girando una pulizia: questa la salto
			}
			markAutoRun(lvl, 3*time.Minute)
			emit("onAutoRun", lvl)
		}
	}()
}

// maybeAutoExit: se Tasso l'ha aperto l'attività pianificata e nessuno l'ha guardato
// (finestra ancora ridotta a icona), a fine pulizia si chiude da solo.
func maybeAutoExit() {
	if !scheduledRun.Load() || wv == nil {
		return
	}
	time.Sleep(45 * time.Second)
	if r, _, _ := pIsIconic.Call(uintptr(wv.Window())); r == 0 {
		scheduledRun.Store(false) // l'hai aperto tu: da ora è un Tasso normale, non si chiude da solo
		return
	}
	// Se c'è ancora un lavoro in corso riprovo ogni minuto (al massimo mezz'ora).
	for i := 0; i < 30; i++ {
		if r, _, _ := pIsIconic.Call(uintptr(wv.Window())); r == 0 {
			scheduledRun.Store(false)
			return
		}
		busyMu.Lock()
		anyBusy := false
		for _, b := range busy {
			anyBusy = anyBusy || b
		}
		busyMu.Unlock()
		if !anyBusy && !isRunning() {
			wv.Dispatch(func() { wv.Terminate() })
			return
		}
		time.Sleep(time.Minute)
	}
	scheduledRun.Store(false)
}
