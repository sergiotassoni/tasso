package main

// "Togli Tasso dal PC" (3.1): toglie tutto quello che Tasso ha messo e lascia stare i file della
// persona. Ordine: prima le cose che si possono ancora annullare (impostazioni, giochi); se un gioco
// tradotto è aperto mi fermo PRIMA di cancellare qualcosa. Alla fine le cartelle di Tasso e il
// suo file si cancellano dopo che Tasso si è chiuso (mentre è aperto i file sono in uso).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"tasso/traduce"
)

// cartelleTasso: le cartelle che Tasso crea e che si cancellano per intero. Solo questi tre
// percorsi esatti, ricavati dalle cartelle di Windows (mai da un valore scritto su disco).
func cartelleTasso() []string {
	// cartella di Windows vera, non la variabile d'ambiente (che un programma qualsiasi può cambiare)
	local, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		local = ""
	}
	var out []string
	for _, d := range []string{filepath.Join(local, "Tasso"), filepath.Join(local, "TassoTraduce"), filepath.Join(programFilesDir(), "Tasso")} {
		if local == "" && !strings.HasPrefix(d, programFilesDir()) {
			continue
		}
		if filepath.IsAbs(d) && strings.EqualFold(filepath.Base(d)[:5], "Tasso") {
			out = append(out, d)
		}
	}
	return out
}

func dimensioneCartella(d string) int64 {
	var n int64
	_ = filepath.Walk(d, func(_ string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			n += fi.Size()
		}
		return nil
	})
	return n
}

// togliTassoInfo: per la finestra di conferma (quanto spazio libera, quante password nel portachiavi).
func togliTassoInfo() map[string]interface{} {
	var tot int64
	for _, d := range cartelleTasso() {
		tot += dimensioneCartella(d)
	}
	// il portachiavi è cifrato col PIN: so solo se c'è, non quante password contiene
	return map[string]interface{}{"bytes": tot, "vault": vaultExists()}
}

var reExeTasso = regexp.MustCompile(`(?i)^tasso[\w .-]*\.exe$`)

// togliTasso fa tutto il lavoro; passo(i, testo) aggiorna la finestra. Restituisce "" se è andato
// tutto bene (e Tasso si chiude da solo), altrimenti il motivo per cui si è fermato.
func togliTasso(rimetti bool, passo func(int, string)) string {
	// 1. impostazioni di Windows e cose accese da Tasso
	if rimetti {
		msgs, ok := restoreOriginal()
		if !ok {
			return "Non ho finito di rimettere le impostazioni: " + strings.Join(msgs, " ") + " Non ho tolto niente: riprova."
		}
		passo(0, "Impostazioni di Windows rimesse com'erano")
	} else {
		passo(0, "Boost, clic automatico e modalità dell'Ufficio spenti (le altre impostazioni restano come sono)")
	}
	// da qui nessuna pulizia programmata deve partire (restoreOriginal ha già preso e lasciato il posto)
	if !claimRun() {
		return "Tasso sta facendo un altro lavoro (pulizia o aggiornamento): aspetta che finisca e riprova. Tasso non è stato tolto."
	}
	// il posto resta preso fino alla chiusura se va tutto bene (nessuna pulizia nei 10 secondi finali)
	finito := false
	defer func() {
		if !finito {
			releaseRun()
		}
	}()
	autoClickStop()
	boostOff(true) // come "Rimetti com'era" del Boost (se non era acceso non fa niente)
	officeSpegniTutto()

	// 2. traduzioni dentro i giochi: se un gioco è aperto mi fermo qui
	tolti, falliti := traduce.TogliTutteLeTraduzioni()
	if len(falliti) == 1 && falliti[0] == "INSTALLA" {
		return "Tasso sta installando una traduzione in un gioco: aspetta che finisca e riprova. Tasso non è stato tolto."
	}
	if len(falliti) > 0 {
		return "Chiudi " + strings.Join(falliti, ", ") + " e riprova: con il gioco aperto non posso togliere la traduzione. Tasso non è stato tolto."
	}
	if len(tolti) > 0 {
		passo(1, trGo("Traduzioni tolte dai giochi: ")+strings.Join(tolti, ", "))
	} else {
		passo(1, "Nessuna traduzione dentro i giochi da togliere")
	}

	// 3. partenza con Windows e attività programmate
	setAutostart(false)
	setWeekly(false)
	_, _ = runHidden("schtasks.exe", "/Delete", "/TN", taskProfilo, "/F")
	runPS(`Get-ScheduledTask -TaskPath '\Tasso\' -ErrorAction SilentlyContinue | Unregister-ScheduledTask -Confirm:$false -ErrorAction SilentlyContinue
try { $s = New-Object -ComObject Schedule.Service; $s.Connect(); $s.GetFolder('\').DeleteFolder('Tasso', 0) } catch {}`, time.Minute, nil)
	passo(2, "Partenza con Windows e pulizie automatiche tolte")

	// 4. copie di sicurezza in Documenti (backup del registro, plugin dei giochi messi da parte)
	salvaCopieDiSicurezza()
	_ = registry.DeleteKey(registry.CURRENT_USER, `Software\Tasso\PrimaDelleModalita`)
	_ = registry.DeleteKey(registry.CURRENT_USER, `Software\Tasso`)
	_ = registry.DeleteKey(registry.LOCAL_MACHINE, `SOFTWARE\Tasso\PuntoRipristino`)
	_ = registry.DeleteKey(registry.LOCAL_MACHINE, `SOFTWARE\Tasso`)
	togliCollegamentoDesktop()

	// 5. cartelle e file di Tasso: dopo la chiusura
	if err := cancellaDopoLaChiusura(); err != nil {
		// il prefisso tradotto qui: con un percorso dentro, la finestra non traduce i pezzi
		return trGo("Ho tolto tutto tranne le cartelle di Tasso: ") + err.Error()
	}
	passo(3, "Le cartelle e il file di Tasso si cancellano appena Tasso si chiude")
	finito = true
	return ""
}

// salvaCopieDiSicurezza: i backup del registro e i plugin che l'utente aveva messo in un gioco
// tradotto finiscono in Documenti\Tasso - copie di sicurezza (non si cancellano mai).
func salvaCopieDiSicurezza() {
	doc, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dest := filepath.Join(doc, "Documents", "Tasso - copie di sicurezza")
	if p, e := windows.KnownFolderPath(windows.FOLDERID_Documents, 0); e == nil && p != "" {
		dest = filepath.Join(p, "Tasso - copie di sicurezza")
	}
	sposta := func(da, a string) {
		if voci, _ := os.ReadDir(da); len(voci) == 0 {
			return
		}
		_ = os.MkdirAll(filepath.Dir(a), 0o755)
		if os.Rename(da, a) != nil {
			_ = copiaCartellaSemplice(da, a)
		}
	}
	sposta(filepath.Join(appDir, "backup"), filepath.Join(dest, "backup del registro"))
	// backup fatti dalle voci dell'Ufficio (stessa cartella di Windows, non la variabile d'ambiente)
	if local, e := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0); e == nil && local != "" {
		sposta(filepath.Join(local, "Tasso", "backup-registro"), filepath.Join(dest, "backup del registro - ufficio"))
	}
	// copie dei giochi: tutto quello che resta (plugin messi da parte, originali di giochi che non
	// ho potuto sistemare perché disinstallati o su un disco staccato, elenchi illeggibili)
	copie := filepath.Join(traduce.CartellaDati(), "copie-giochi")
	voci, _ := os.ReadDir(copie)
	for _, v := range voci {
		sposta(filepath.Join(copie, v.Name()), filepath.Join(dest, "giochi", v.Name()))
	}
}

func copiaCartellaSemplice(da, a string) error {
	return filepath.Walk(da, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(da, p)
		t := filepath.Join(a, rel)
		if fi.IsDir() {
			return os.MkdirAll(t, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(t, b, 0o644)
	})
}

// togliCollegamentoDesktop: solo Desktop\Tasso.lnk, e solo se porta a questo Tasso.
func togliCollegamentoDesktop() {
	exe, _ := os.Executable()
	runPSArgs(`$lnk = Join-Path ([Environment]::GetFolderPath('Desktop')) 'Tasso.lnk'
if (Test-Path -LiteralPath $lnk) {
  $t = (New-Object -ComObject WScript.Shell).CreateShortcut($lnk).TargetPath
  if ($t -and $t -ieq [string]$A.exe) { Remove-Item -LiteralPath $lnk -Force -ErrorAction SilentlyContinue }
}`, map[string]interface{}{"exe": exe}, time.Minute, nil)
}

// cancellaDopoLaChiusura: un piccolo comando di Windows aspetta che Tasso sia chiuso e cancella
// le sue cartelle e il suo file. Percorsi controllati qui, mai presi da file su disco.
func cancellaDopoLaChiusura() error {
	testo, err := scriptCancellazione()
	if err != nil {
		return err
	}
	// il comando gira da amministratore: lo scrivo in Programmi (solo gli amministratori ci possono
	// scrivere), mai nella cartella temporanea dell'utente dove un altro programma potrebbe cambiarlo
	// prima che parta. O_EXCL: se c'è già un file con quel nome non lo riuso.
	pf := programFilesDir()
	if pf == "" {
		return fmt.Errorf("non trovo la cartella Programmi")
	}
	bat := filepath.Join(pf, "togli-tasso-"+strconv.Itoa(os.Getpid())+".cmd")
	_ = os.Remove(bat) // resto di un tentativo interrotto (in Programmi scrive solo un amministratore)
	f, err := os.OpenFile(bat, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, err = f.WriteString(testo)
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		_ = os.Remove(bat)
		return err
	}
	sys := sistema32()
	c := exec.Command(filepath.Join(sys, "cmd.exe"), "/d", "/c", bat)
	c.Dir = sys
	c.Env = []string{"SystemRoot=" + filepath.Dir(sys), "PATH=" + sys, "PATHEXT=.COM;.EXE"}
	// CREATE_NO_WINDOW (non DETACHED: tasklist, find e ping aprirebbero ognuno una finestra) | NEW_PROCESS_GROUP
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000 | 0x00000200}
	if err := c.Start(); err != nil {
		_ = os.Remove(bat)
		return err
	}
	return nil
}

func scriptCancellazione() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, _ = filepath.Abs(exe)
	var cmd strings.Builder
	cmd.WriteString("@echo off\r\n")
	// il file è in UTF-8: senza questo un nome utente con accenti (Nicolò) darebbe percorsi sbagliati
	fmt.Fprintf(&cmd, "\"%s\" 65001 >nul\r\n", filepath.Join(sistema32(), "chcp.com"))
	// aspetto la chiusura di Tasso (al massimo 2 minuti)
	// se dopo 2 minuti Tasso è ancora aperto non cancello niente (i file sarebbero in uso)
	sys := sistema32()
	if strings.ContainsAny(sys, "\"%&|<>^\r\n") {
		return "", fmt.Errorf("percorso strano: %s", sys)
	}
	tl, fd, pg := filepath.Join(sys, "tasklist.exe"), filepath.Join(sys, "find.exe"), filepath.Join(sys, "PING.EXE")
	fmt.Fprintf(&cmd, "for /L %%%%i in (1,1,120) do (\"%s\" /FI \"PID eq %d\" | \"%s\" \"%d\" >nul || goto via & \"%s\" -n 2 127.0.0.1 >nul)\r\n(goto) 2>nul & del /f /q \"%%~f0\"\r\n:via\r\n", tl, os.Getpid(), fd, os.Getpid(), pg)
	for _, d := range cartelleTasso() {
		if strings.ContainsAny(d, "\"%&|<>^\r\n") {
			return "", fmt.Errorf("percorso strano: %s", d)
		}
		// due tentativi: i programmi della traduzione possono chiudersi qualche secondo dopo
		fmt.Fprintf(&cmd, "rmdir /s /q \"%s\" 2>nul\r\nif exist \"%s\" (\"%s\" -n 6 127.0.0.1 >nul & rmdir /s /q \"%s\" 2>nul)\r\n", d, d, pg, d)
	}
	if reExeTasso.MatchString(filepath.Base(exe)) && !strings.ContainsAny(exe, "\"%&|<>^\r\n") {
		fmt.Fprintf(&cmd, "del /f /q \"%s\" 2>nul\r\n", exe)
	}
	// il comando si cancella da solo alla fine
	cmd.WriteString("(goto) 2>nul & del /f /q \"%~f0\"\r\n")
	return cmd.String(), nil
}

// sistema32: la vera cartella System32 (chiesta a Windows, non presa da SystemRoot dell'ambiente).
func sistema32() string {
	if d, err := windows.GetSystemDirectory(); err == nil && d != "" {
		return d
	}
	return `C:\Windows\System32`
}
