package main

// "Togli Tasso dal PC" (3.1): toglie tutto quello che Tasso ha messo e lascia stare i file della
// persona. Ordine: prima le cose che si possono ancora annullare (impostazioni, giochi); se un gioco
// tradotto è aperto mi fermo PRIMA di cancellare qualcosa. Alla fine le cartelle di Tasso e il
// suo file si cancellano dopo che Tasso si è chiuso (mentre è aperto i file sono in uso).
//
// Con l'Ecosistema Tasso (installatore Tasso Setup, ecosistema.go) nella cartella di Tasso ci sono
// anche Tasso Zip e Tasso PDF, che qui non si toccano MAI:
//   - c'è il disinstallatore dell'installatore: Tasso toglie le sue cose (impostazioni, traduzioni,
//     attività, cartelle di lavoro) e poi, chiudendosi, apre il disinstallatore con Tasso Gaming e
//     Tasso Ufficio già spuntati: è lui che toglie Tasso.exe, i collegamenti e la voce in
//     "Programmi e funzionalità" (e Zip e PDF solo se li spunti);
//   - niente disinstallatore ma ci sono le cartelle Tasso Zip / Tasso PDF: tolgo solo i file e le
//     cartelle di Tasso, e la cartella di Tasso resta se non è vuota.

import (
	"fmt"
	"io"
	"io/fs"
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

// pianoTogli: cosa si cancella dopo la chiusura. Percorsi esatti ricavati dalle cartelle di Windows
// e dalla cartella di Tasso (ecosistema.go), mai da un valore scritto su disco.
type pianoTogli struct {
	intere  []string // cartelle che si cancellano per intero
	file    []string // file che si cancellano
	seVuote []string // cartelle che si tolgono solo se restano vuote
	disinst string   // disinstallatore dell'ecosistema da aprire alla chiusura ("" se non c'è)
	eco     bool     // c'è l'ecosistema (disinstallatore o Tasso Zip / Tasso PDF): Zip e PDF restano
}

// nomeCartellaTasso: le cartelle che si cancellano per intero hanno un nome che comincia con
// "Tasso" o sono le sottocartelle di lavoro di Tasso (lavoro, driver, strumenti).
func nomeCartellaTasso(d string) bool {
	b := strings.ToLower(filepath.Base(d))
	return strings.HasPrefix(b, "tasso") || b == "lavoro" || b == "driver" || b == "strumenti"
}

func cartellaVera(p string) bool {
	fi, err := os.Lstat(p)
	return err == nil && fi.Mode().IsDir()
}

func pianoTogliTasso() pianoTogli {
	var p pianoTogli
	// cartella di Windows vera, non la variabile d'ambiente (che un programma qualsiasi può cambiare)
	if local, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0); err == nil && local != "" {
		p.intere = append(p.intere, filepath.Join(local, "Tasso"), filepath.Join(local, "TassoTraduce"))
	}
	pf := programFilesDir()
	base := cartellaTasso()
	std := filepath.Join(pf, "Tasso")
	disinst, dirInst := disinstallatoreEcosistema()
	p.disinst = disinst
	p.eco = disinst != "" || cartellaVera(filepath.Join(base, "Tasso Zip")) || cartellaVera(filepath.Join(base, "Tasso PDF"))
	exe, _ := os.Executable()
	if exe != "" {
		exe, _ = filepath.Abs(exe)
	}
	if !p.eco && strings.EqualFold(base, std) {
		p.intere = append(p.intere, base) // come prima: C:\Program Files\Tasso è solo di Tasso
	} else {
		// Solo le cose di Tasso: Tasso Zip, Tasso PDF, il disinstallatore e tasso-setup.json restano.
		for _, d := range []string{"lavoro", "driver", "strumenti"} {
			p.intere = append(p.intere, filepath.Join(base, d))
		}
		// copie a metà di un aggiornamento (installProgramCopy, selfupdate): non sono nell'elenco
		// dell'installatore, e senza toglierle la cartella non resterebbe mai vuota
		for _, f := range []string{"Tasso.exe.new", "Tasso.exe.old", "Tasso.new.exe", "Tasso.old.exe"} {
			p.file = append(p.file, filepath.Join(base, f))
		}
		// Tasso.exe, VERSIONE e LICENSE: se li ha messi l'installatore li toglie il suo disinstallatore
		if disinst == "" || !strings.EqualFold(base, dirInst) {
			for _, f := range []string{"Tasso.exe", "VERSIONE", "LICENSE"} {
				p.file = append(p.file, filepath.Join(base, f))
			}
		}
		p.seVuote = append(p.seVuote, base)
	}
	// il file di Tasso che sta girando, se non lo toglie già qualcos'altro: il disinstallatore
	// (è il suo Tasso.exe) o la cancellazione della cartella intera. Mai dentro Tasso Zip / Tasso PDF.
	if exe != "" && reExeTasso.MatchString(filepath.Base(exe)) {
		dir := filepath.Dir(exe)
		suo := disinst != "" && strings.EqualFold(dir, dirInst)
		zipPdf := false
		for _, b := range []string{base, dirInst} {
			if b != "" && (dentroDi(exe, filepath.Join(b, "Tasso Zip")) || dentroDi(exe, filepath.Join(b, "Tasso PDF"))) {
				zipPdf = true
			}
		}
		giaVia := !p.eco && strings.EqualFold(base, std) && dentroDi(exe, base)
		gia := false
		for _, f := range p.file {
			gia = gia || strings.EqualFold(f, exe)
		}
		if !suo && !zipPdf && !giaVia && !gia {
			p.file = append(p.file, exe)
		}
	}
	return p
}

// cartelleTasso: le cartelle che si cancellano per intero (per la finestra di conferma).
func cartelleTasso() []string { return pianoTogliTasso().intere }

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
	piano, msg, fatto := togliTassoPassi(rimetti, passo)
	if msg != "" {
		return msg
	}
	defer func() {
		if !*fatto {
			releaseRun()
		}
	}()
	// 5. cartelle e file di Tasso: dopo la chiusura
	if err := cancellaDopoLaChiusura(piano); err != nil {
		// il prefisso tradotto qui: con un percorso dentro, la finestra non traduce i pezzi
		return trGo("Ho tolto tutto tranne le cartelle di Tasso: ") + err.Error()
	}
	if piano.disinst != "" {
		disinstAllaChiusura.Store(piano.disinst) // ecosistema.go: si apre quando la finestra si chiude
		passo(3, "Le cartelle di Tasso si cancellano appena Tasso si chiude. Poi si apre il disinstallatore di Tasso: lì togli il programma (Tasso Zip e Tasso PDF restano se non li spunti)")
	} else if piano.eco {
		passo(3, "Le cartelle e il file di Tasso si cancellano appena Tasso si chiude (Tasso Zip e Tasso PDF restano)")
	} else {
		passo(3, "Le cartelle e il file di Tasso si cancellano appena Tasso si chiude")
	}
	trayStop() // tray.go: via l'icona vicino all'orologio (e da qui la X chiude davvero)
	togliFinito.Store(true)
	*fatto = true
	return ""
}

// togliTassoPassi: i passi 1-4 di "Togli Tasso dal PC" (impostazioni, traduzioni nei giochi,
// attività, copie di sicurezza e registro), senza cancellare cartelle né chiudere Tasso. msg = il
// motivo se si è fermato ("" = tutto fatto). Se va tutto bene il posto di claimRun resta preso:
// chi chiama lo lascia (*fatto false) o lo tiene fino alla chiusura (*fatto true).
func togliTassoPassi(rimetti bool, passo func(int, string)) (piano pianoTogli, msg string, fatto *bool) {
	fatto = new(bool)
	msg = togliTassoPassiDentro(rimetti, passo, &piano)
	return piano, msg, fatto
}

func togliTassoPassiDentro(rimetti bool, passo func(int, string), pianoOut *pianoTogli) string {
	// 1. impostazioni di Windows e cose accese da Tasso
	if rimetti {
		msgs, ok, avvioNo := restoreOriginalDett()
		if !ok {
			return "Non ho finito di rimettere le impostazioni: " + strings.Join(msgs, " ") + " Non ho tolto niente: riprova."
		}
		if len(avvioNo) > 0 {
			// un programma all'avvio che non torna com'era non blocca la disinstallazione: avviso e vado avanti
			passo(0, msgAvvioFallitiTogli(avvioNo))
		} else {
			passo(0, "Impostazioni di Windows rimesse com'erano")
		}
	} else {
		passo(0, "Boost, clic automatico e modalità dell'Ufficio spenti (le altre impostazioni restano come sono)")
	}
	// da qui nessuna pulizia programmata deve partire (restoreOriginal ha già preso e lasciato il posto)
	if !claimRun() {
		return "Tasso sta facendo un altro lavoro (pulizia o aggiornamento): aspetta che finisca e riprova. Tasso non è stato tolto."
	}
	// il posto resta preso se va tutto bene: lo lascia chi chiama (togliTasso, togliSilenzioso)
	finito := false
	defer func() {
		if !finito {
			releaseRun()
		}
	}()
	autoClickStop()
	boostOff(true) // come "Rimetti com'era" del Boost (se non era acceso non fa niente)
	officeSpegniTutto()
	// Se la Modalità riunione o il blocco dell'Ufficio sono ancora accesi, le loro impostazioni di
	// prima stanno in quei file: cancellando le cartelle non si potrebbero più rimettere. Mi fermo qui.
	for _, n := range []string{"riunione.json", "blocco.json"} {
		if _, err := os.Stat(filepath.Join(officeDir(), n)); err == nil {
			return "Non sono riuscito a spegnere la Modalità riunione o il blocco del PC dell'Ufficio: spegnili dalla sezione Ufficio di Tasso e riprova. Tasso non è stato tolto."
		}
	}

	// 2. traduzioni dentro i giochi: se un gioco è aperto mi fermo qui
	if traduce.CartellaDati() == "" {
		// Tasso Traduce non è partito (Tasso.exe --togli-silenzioso senza traduce.Prepara): senza la
		// sua cartella non trovo le copie dei giochi, quindi non dico "nessuna traduzione"
		return "Non trovo la cartella di Tasso Traduce: apri Tasso e usa \"Togli Tasso dal PC\". Tasso non è stato tolto."
	}
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
	piano := pianoTogliTasso()
	_ = registry.DeleteKey(registry.CURRENT_USER, `Software\Tasso\PrimaDelleModalita`)
	_ = registry.DeleteKey(registry.LOCAL_MACHINE, `SOFTWARE\Tasso\PuntoRipristino`)
	if piano.eco {
		// Software\Tasso è anche dell'installatore (Lingua, Modalita, Cartella...: le leggono Zip,
		// PDF e il disinstallatore): tolgo solo le cose di Tasso.
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\Tasso\AudioUSB`)
	} else {
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\Tasso`)
		_ = registry.DeleteKey(registry.LOCAL_MACHINE, `SOFTWARE\Tasso`)
	}
	togliCollegamentoDesktop()
	*pianoOut = piano
	finito = true
	return ""
}

// ---------------- Tasso.exe --togli-silenzioso (lo usa il disinstallatore) ----------------

// argTogliSilenzioso: il disinstallatore di Tasso Setup (anche da App e funzionalità) chiede a
// Tasso di togliere le sue cose prima di cancellare Tasso.exe: Boost, clic automatico e modalità
// dell'Ufficio spenti, traduzioni tolte dai giochi, attività pianificate, copie di sicurezza in
// Documenti, chiavi del registro di Tasso. Senza finestra. Le cartelle (lavoro, strumenti,
// driver, %LOCALAPPDATA%) le toglie il disinstallatore (i dati solo se l'utente lo conferma).
const argTogliSilenzioso = "--togli-silenzioso"

// segnoTogliSilenzioso: il disinstallatore avvia Tasso con --togli-silenzioso solo se questa
// scritta è dentro Tasso.exe (un Tasso che non conosce l'argomento si aprirebbe normalmente).
// C'è nell'exe solo se main chiama togliSilenziosoSeChiesto.
const segnoTogliSilenzioso = "tasso-togli-silenzioso-v1"

// togliSilenziosoSeChiesto: da chiamare in main dopo loadSettings (con il mutex della copia unica
// preso). Se è stato chiesto --togli-silenzioso fa il lavoro e restituisce il codice d'uscita.
// Uscita (righe sullo standard output, che legge il disinstallatore): "TOGLI-OK" e codice 0, oppure
// "TOGLI-NO <motivo>" e codice 1 (adesso non si può: niente è stato tolto).
func togliSilenziosoSeChiesto() (codice int, chiesto bool) {
	for _, a := range os.Args[1:] {
		if a == argTogliSilenzioso {
			return togliSilenzioso(os.Stdout), true
		}
	}
	return 0, false
}

func togliSilenzioso(out io.Writer) int {
	fmt.Fprintln(out, segnoTogliSilenzioso)
	_, msg, _ := togliTassoPassi(false, func(int, string) {})
	if msg != "" {
		fmt.Fprintln(out, "TOGLI-NO "+unaRiga(msg))
		return 1
	}
	releaseRun()
	fmt.Fprintln(out, "TOGLI-OK")
	return 0
}

// unaRiga: il motivo su una riga sola (il disinstallatore legge fino al primo a capo).
func unaRiga(s string) string {
	return strings.Join(strings.Fields(s), " ")
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
	// (dalla 3.2 in Programmi\Tasso\lavoro; guardo anche il posto vecchio in AppData)
	for _, copie := range []string{traduce.CartellaCopie(), traduce.CartellaCopieVecchia()} {
		if copie == "" {
			continue
		}
		voci, _ := os.ReadDir(copie)
		for _, v := range voci {
			if !v.IsDir() || v.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
				continue // il segno dello spostamento, link e giunzioni: non li porto via
			}
			sposta(filepath.Join(copie, v.Name()), filepath.Join(dest, "giochi", v.Name()))
		}
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
func cancellaDopoLaChiusura(piano pianoTogli) error {
	testo, err := scriptCancellazione(piano)
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

func scriptCancellazione(piano pianoTogli) (string, error) {
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
	strano := func(p string) bool {
		return !filepath.IsAbs(p) || strings.HasPrefix(p, `\\`) || strings.ContainsAny(p, "\"%&|<>^\r\n")
	}
	for _, d := range piano.intere {
		if strano(d) || !nomeCartellaTasso(d) || len(d) <= 3 {
			return "", fmt.Errorf("percorso strano: %s", d)
		}
		// mai le cartelle di Tasso Zip e Tasso PDF né una cartella che le contiene
		if cartellaVera(filepath.Join(d, "Tasso Zip")) || cartellaVera(filepath.Join(d, "Tasso PDF")) {
			return "", fmt.Errorf("dentro ci sono Tasso Zip o Tasso PDF: %s", d)
		}
		// due tentativi: i programmi della traduzione possono chiudersi qualche secondo dopo
		fmt.Fprintf(&cmd, "rmdir /s /q \"%s\" 2>nul\r\nif exist \"%s\" (\"%s\" -n 6 127.0.0.1 >nul & rmdir /s /q \"%s\" 2>nul)\r\n", d, d, pg, d)
	}
	for _, f := range piano.file {
		if strano(f) || !fileTasso(filepath.Base(f)) {
			return "", fmt.Errorf("percorso strano: %s", f)
		}
		fmt.Fprintf(&cmd, "if exist \"%s\" del /f /q \"%s\" 2>nul\r\n", f, f)
	}
	for _, d := range piano.seVuote {
		if strano(d) || len(d) <= 3 {
			return "", fmt.Errorf("percorso strano: %s", d)
		}
		fmt.Fprintf(&cmd, "rmdir \"%s\" 2>nul\r\n", d) // senza /s: solo se è vuota
	}
	// il comando si cancella da solo alla fine
	cmd.WriteString("(goto) 2>nul & del /f /q \"%~f0\"\r\n")
	return cmd.String(), nil
}

// fileTasso: i soli nomi di file che "Togli Tasso" cancella.
func fileTasso(n string) bool {
	switch strings.ToLower(n) {
	case "versione", "license":
		return true
	}
	return reExeTasso.MatchString(n) || strings.EqualFold(n, "Tasso.exe.new") || strings.EqualFold(n, "Tasso.exe.old")
}

// sistema32: la vera cartella System32 (chiesta a Windows, non presa da SystemRoot dell'ambiente).
func sistema32() string {
	if d, err := windows.GetSystemDirectory(); err == nil && d != "" {
		return d
	}
	return `C:\Windows\System32`
}
