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
	"tasso/traduce"
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
			return "Non sono riuscito ad attivare la Pulizia programmata: non posso copiare Tasso nella cartella Programmi."
		}
		days := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
		var out []string
		runPSArgs(weeklyTaskScript, map[string]interface{}{
			"exe": taskExe(), "day": days[day], "at": fmt.Sprintf("%02d:00", hour), "level": level, "every": every,
		}, 2*time.Minute, func(m string) { out = append(out, m) })
		if !strings.Contains(strings.Join(out, "\n"), "TASK_OK") {
			return "Non sono riuscito ad attivare la Pulizia programmata."
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

// autoRiavvioServe: l'ultima pulizia finita ha chiesto un riavvio che Tasso non ha fatto (F07-5).
var autoRiavvioServe atomic.Bool

func autoReqPath() string { return filepath.Join(appDir, "pulizia-richiesta.txt") }

// askRunningInstance scrive il livello richiesto e sveglia la copia di Tasso già aperta.
func askRunningInstance(level string) bool {
	if n, err := strconv.Atoi(level); err != nil || n < 1 || n > 5 {
		return false
	}
	// la cartella vera di Windows, non la variabile d'ambiente; e niente collegamenti o giunzioni
	// (F20-6: questo Tasso può girare da amministratore, la cartella è dell'utente)
	if la := cartellaNota(windows.FOLDERID_LocalAppData); la != "" {
		appDir = filepath.Join(la, "Tasso")
	} else {
		appDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "Tasso")
	}
	if controllaPercorsoApp(appDir) == nil {
		_ = os.Remove(autoReqPath()) // un collegamento al suo posto si toglie, non si segue
	}
	if scriviFileApp(autoReqPath(), []byte(level), 0o600) != nil {
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
			b, err := leggiFileApp(autoReqPath())
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
			avviaQuandoNessunGioco(lvl)
		}
	}()
}

var aspettaGioco atomic.Bool

// avviaQuandoNessunGioco: la pulizia programmata non parte mentre giochi (driver, DISM e
// deframmentazione rallenterebbero o fermerebbero il gioco): aspetto che il gioco sia chiuso,
// al massimo 4 ore, poi la salto fino alla prossima volta.
// Lo stesso durante una chiamata (Modalità riunione accesa o microfono in uso): dal livello 3 gli
// aggiornamenti di Windows si installano lo stesso, anche con la pausa della riunione.
func avviaQuandoNessunGioco(lvl int) {
	if !occupatoPerPulizia() {
		go avviaPuliziaAuto(lvl)
		return
	}
	if !aspettaGioco.CompareAndSwap(false, true) {
		return // sto già aspettando
	}
	go func() {
		defer guard("weekly.go:gioco")
		defer aspettaGioco.Store(false)
		fine := time.Now().Add(4 * time.Hour)
		for time.Now().Before(fine) {
			time.Sleep(2 * time.Minute)
			if scheduledRun.Load() && wv != nil {
				if r, _, _ := pIsIconic.Call(uintptr(wv.Window())); r == 0 {
					diventaNormale() // l'hai aperto tu mentre aspettavo: è un Tasso normale (con la traduzione)
				}
			}
			if !occupatoPerPulizia() {
				time.Sleep(time.Minute) // il gioco (o la chiamata) è appena finito: lascio respirare il PC
				if occupatoPerPulizia() {
					continue // hai aperto un altro gioco (o il launcher l'ha riaperto): aspetto ancora
				}
				if isRunning() {
					go maybeAutoExit() // c'è già una pulizia in corso: a fine pulizia si chiude da solo
					return
				}
				avviaPuliziaAuto(lvl)
				return
			}
		}
		go maybeAutoExit() // hai giocato più di 4 ore: la pulizia la faccio la prossima volta
	}()
}

// avviaPuliziaAuto: chiede alla finestra di far partire la pulizia programmata. Se la finestra
// non era ancora pronta e il messaggio si è perso (la pulizia non ha preso il suo segno), lo
// richiedo qualche volta; se non parte proprio, la sessione pianificata si chiude da sola.
func avviaPuliziaAuto(lvl int) {
	markAutoRun(lvl, 10*time.Minute)
	emit("onAutoRun", lvl)
	for i := 0; i < 8; i++ {
		time.Sleep(30 * time.Second)
		autoMu.Lock()
		ancora := autoPending == lvl
		autoMu.Unlock()
		if !ancora || isRunning() {
			return // partita (il segno l'ha preso la pulizia)
		}
		emit("onAutoRun", lvl)
	}
	autoMu.Lock()
	if autoPending == lvl {
		autoPending = 0
	}
	autoMu.Unlock()
	go maybeAutoExit()
}

// diventaNormale: la sessione aperta dall'attività pianificata diventa un Tasso normale.
// Tasso Traduce all'avvio era stato saltato: parte adesso (Avvia parte una volta sola).
func diventaNormale() {
	if scheduledRun.Swap(false) {
		go func() {
			defer guard("traduce")
			traduce.Avvia()
		}()
	}
}

// maybeAutoExit: se Tasso l'ha aperto l'attività pianificata e nessuno l'ha guardato
// (finestra ancora ridotta a icona), a fine pulizia si chiude da solo.
func maybeAutoExit() {
	if !scheduledRun.Load() || wv == nil {
		return
	}
	time.Sleep(45 * time.Second)
	if r, _, _ := pIsIconic.Call(uintptr(wv.Window())); r == 0 {
		diventaNormale() // l'hai aperto tu: da ora è un Tasso normale, non si chiude da solo
		return
	}
	// Se c'è ancora un lavoro in corso riprovo ogni minuto (al massimo mezz'ora).
	for i := 0; i < 30; i++ {
		if r, _, _ := pIsIconic.Call(uintptr(wv.Window())); r == 0 {
			diventaNormale()
			return
		}
		busyMu.Lock()
		anyBusy := false
		for _, b := range busy {
			anyBusy = anyBusy || b
		}
		busyMu.Unlock()
		if !anyBusy && !isRunning() {
			// F07-5: se la pulizia ha chiesto un riavvio, Tasso non sparisce in silenzio: lo dice vicino
			// all'orologio e resta aperto (ridotto a icona) con la nota e il pulsante "Riavvia ora".
			if autoRiavvioServe.Load() {
				trayAvviso("Tasso", "La pulizia programmata è finita: riavvia il PC per completare aggiornamenti e riparazioni.", false)
				diventaNormale()
				return
			}
			// Come "Chiudi Tasso": Boost, modalità della partita e frequenza in prova tornano com'erano.
			boostSpegniPerUscita()
			modalitaGiocoPerUscita()
			screenHzUndo(false)
			wv.Dispatch(func() { wv.Terminate() })
			return
		}
		time.Sleep(time.Minute)
	}
	diventaNormale()
}

// occupatoPerPulizia: c'è un gioco aperto o una chiamata in corso (Modalità riunione accesa o un
// programma che usa il microfono): la pulizia programmata aspetta.
func occupatoPerPulizia() bool {
	if runningGame() != "" {
		return true
	}
	if _, err := os.Stat(fileUfficio("riunione")); err == nil {
		return true
	}
	return micInUso()
}
