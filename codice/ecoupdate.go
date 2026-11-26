package main

// Aggiornamento di tutto l'Ecosistema Tasso (Tasso, Tasso Zip, Tasso PDF) dalle Impostazioni.
//
// Se Tasso è quello installato da Tasso Setup (accanto c'è tasso-setup.json con "Disinstalla
// Tasso.exe", vedi ecosistema.go), "Controlla aggiornamenti" legge TassoSetup.sha256 dalla stessa
// fonte di Tasso.zip (il progetto su GitHub, ramo main) e confronta le versioni di ogni programma
// installato. "Aggiorna ora" (o "Aggiorna da solo") scarica TassoSetup.exe, controlla l'impronta
// SHA-256 e la firma digitale con gli stessi controlli di Tasso.exe (selfupdate.go), lo prova con
// "--aggiorna-silenzioso --solo-controlla" e, se c'è davvero qualcosa da fare, Tasso si chiude e
// lo avvia con "--aggiorna-silenzioso --riapri-tasso": l'installatore aggiorna solo i programmi già
// installati (mai indietro) con una piccola barra e alla fine riapre Tasso.
// Senza ecosistema, o se TassoSetup.sha256 non c'è, resta l'aggiornamento del solo Tasso.exe.
//
// File da pubblicare nel progetto (accanto a Tasso.zip, Tasso.sha256 e VERSION):
//   TassoSetup.exe     l'installatore firmato
//   TassoSetup.sha256  "<sha256>  TassoSetup.exe  <versione ecosistema>" + righe "tasso|zip|pdf <versione>"
//                      (lo scrive l'installatore: strumenti/impronta, dopo la firma)

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"tasso/ecoagg"
)

const (
	setupURL    = "https://github.com/sergiotassoni/tasso/raw/main/" + ecoagg.NomeSetup
	setupShaURL = "https://raw.githubusercontent.com/sergiotassoni/tasso/main/" + ecoagg.NomeManifesto
	// Tasso.exe --controlla-aggiornamenti (lo usano Tasso Zip e Tasso PDF): apre Impostazioni →
	// Aggiornamenti e controlla. Se Tasso è già aperto glielo dice con l'evento qui sotto.
	argControllaAgg    = "--controlla-aggiornamenti"
	eventoControllaAgg = `Local\TassoControllaAggiornamenti`
	// dopo un aggiornamento dell'ecosistema non riuscito non riprovo da solo per un giorno
	ecoRiprova = 24 * time.Hour
)

// chiusuraAgg: Tasso ha avviato l'installatore e si sta chiudendo (Tasso.exe --chiudi non rifiuta).
var chiusuraAgg atomic.Bool

// ecoInstallato: la cartella e tasso-setup.json, se il Tasso aperto è quello dell'ecosistema.
func ecoInstallato() (string, *ecoagg.Stato) {
	_, dir := disinstallatoreEcosistema()
	if dir == "" {
		return "", nil
	}
	me, err := os.Executable()
	if err != nil || !strings.EqualFold(filepath.Clean(me), filepath.Join(filepath.Clean(dir), "Tasso.exe")) {
		return "", nil // un altro Tasso (es. lo zip in Download): si aggiorna da solo come prima
	}
	p := filepath.Join(dir, "tasso-setup.json")
	if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() || fi.Size() > 4<<20 {
		return "", nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", nil
	}
	st, err := ecoagg.LeggiStato(b)
	if err != nil || !st.Ha("tasso") {
		return "", nil
	}
	return dir, st
}

// ecoInfo: per la pagina all'apertura (BOOT.eco): i programmi installati con le loro versioni.
func ecoInfo() map[string]interface{} {
	_, st := ecoInstallato()
	if st == nil {
		return nil
	}
	return map[string]interface{}{"componenti": ecoagg.Componenti(st, nil, appVersion)}
}

// checkEcoUpdate: il controllo con TassoSetup.sha256. nil = niente ecosistema o file non
// raggiungibile: allora vale il controllo del solo Tasso (version.go).
func checkEcoUpdate() map[string]interface{} {
	_, st := ecoInstallato()
	if st == nil {
		return nil
	}
	b, err := httpGet(setupShaURL, 4096, 12*time.Second)
	if err != nil {
		return nil
	}
	m, err := ecoagg.LeggiManifesto(b)
	if err != nil {
		return nil
	}
	cc := ecoagg.Componenti(st, &m, appVersion)
	res := map[string]interface{}{"current": appVersion, "download": setupURL, "componenti": cc,
		"latest": m.Versione, "newer": ecoagg.QualcosaDiNuovo(cc), "eco": true, "sha": m.SHA256}
	setMu.Lock()
	t := settings.EcoTentata
	setMu.Unlock()
	if sha, quando, ok := strings.Cut(t, "|"); ok && sha == m.SHA256 {
		if n, err := strconv.ParseInt(quando, 10, 64); err == nil && time.Since(time.Unix(n, 0)) < ecoRiprova {
			res["tentata"] = true // ci ho già provato da poco con questo stesso installatore e non è andata
		}
	}
	return res
}

// giocoAperto: c'è un gioco aperto o una partita col Boost (come "Aggiorna da solo").
func giocoAperto() bool {
	bMu.Lock()
	inPartita := bGame != ""
	bMu.Unlock()
	return inPartita || runningGame() != ""
}

// ecoAltriAperti: Tasso Zip o Tasso PDF dell'ecosistema sono aperti (l'aggiornamento li chiuderebbe).
func ecoAltriAperti() bool {
	dir, st := ecoInstallato()
	if st == nil {
		return false
	}
	cerca := map[string]bool{
		strings.ToLower(filepath.Join(dir, "Tasso Zip", "TassoZip.exe")): true,
		strings.ToLower(filepath.Join(dir, "Tasso PDF", "TassoPDF.exe")): true,
	}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		n := strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))
		if n != "tassozip.exe" && n != "tassopdf.exe" {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pe.ProcessID)
		if err != nil {
			continue
		}
		buf := make([]uint16, 1024)
		sz := uint32(len(buf))
		ok := windows.QueryFullProcessImageName(h, 0, &buf[0], &sz) == nil && cerca[strings.ToLower(windows.UTF16ToString(buf[:sz]))]
		windows.CloseHandle(h)
		if ok {
			return true
		}
	}
	return false
}

const msgEcoAltriAperti = "Tasso Zip o Tasso PDF sono aperti e potresti avere lavoro non salvato: chiudili e premi di nuovo Aggiorna."

// ecoSelfUpdate: scarica, controlla e avvia l'installatore (u è il risultato di checkEcoUpdate).
func ecoSelfUpdate(u map[string]interface{}) {
	if giocoAperto() {
		updMsg("error", "Hai un gioco aperto: aggiorna quando hai finito di giocare.")
		return
	}
	// Tasso Zip e Tasso PDF aperti: l'installatore li chiuderebbe, anche con un archivio a metà o
	// un PDF non salvato. Prima li chiudi tu (vale anche per "Aggiorna ora", non solo per "da solo").
	if ecoAltriAperti() {
		updMsg("error", msgEcoAltriAperti)
		return
	}
	want, _ := u["sha"].(string)
	updMsg("work", "Scarico l'aggiornamento dei programmi di Tasso…")
	data, err := httpGet(setupURL, 200<<20, 15*time.Minute)
	if err != nil {
		updMsg("error", "Download non riuscito: controlla la connessione e riprova.")
		return
	}
	h := sha256.Sum256(data)
	if want == "" || hex.EncodeToString(h[:]) != want {
		updMsg("error", "Il file scaricato non è quello giusto (impronta diversa): non lo uso. Riprova tra qualche minuto.")
		return
	}
	if len(data) < 1<<20 || !bytes.HasPrefix(data, []byte("MZ")) {
		updMsg("error", "Il file scaricato è rovinato: riprova.")
		return
	}
	// Da qui in poi non deve partire nient'altro: tra poco Tasso si chiude.
	updating.Store(true)
	if isRunning() || otherBusy("update") {
		updating.Store(false)
		updMsg("error", "Tasso sta facendo un altro lavoro: aspetta che finisca, poi aggiorna.")
		return
	}
	cur, err := os.Executable()
	if err != nil {
		updating.Store(false)
		updMsg("error", "Non trovo dove si trova Tasso.")
		return
	}
	if exeHasSignature(cur) {
		updMsg("work", "Controllo la firma digitale…")
	} else {
		updMsg("work", "Controllo che il file sia quello giusto…")
	}
	// Come per Tasso.exe: una copia nuova in Programmi (ci scrivono solo gli amministratori), su cui
	// controllo la firma e che poi avvio così com'è, senza che nessuno possa cambiarla nel frattempo.
	setup, err := copiaPerVerifica(data)
	if err != nil {
		updating.Store(false)
		updMsg("error", "Non riesco a preparare il controllo della firma: per sicurezza non aggiorno. Scarica lo zip a mano da Info.")
		return
	}
	fallito := func(msg string) {
		_ = os.Remove(setup)
		updating.Store(false)
		updMsg("error", msg)
	}
	if msg := checkUpdateSignature(cur, setup); msg != "" {
		fallito(msg)
		return
	}
	// Prova: l'installatore dice se c'è davvero qualcosa da aggiornare (0) o no (2), senza toccare niente.
	switch provaSetup(setup) {
	case 0:
	case 2:
		fallito("Hai già l'ultima versione.")
		return
	default:
		fallito("L'installatore scaricato non riesce ad aggiornare i programmi adesso. Riprova più tardi.")
		return
	}
	updateSettings(func(s *Settings) { s.EcoTentata = want + "|" + strconv.FormatInt(time.Now().Unix(), 10) })
	updMsg("done", "Fatto! Tasso si chiude, aggiorna i programmi e si riapre tra un attimo.")
	time.Sleep(1500 * time.Millisecond)
	// di nuovo, subito prima di partire: durante il download (minuti) potresti averli aperti
	if ecoAltriAperti() {
		fallito(msgEcoAltriAperti)
		return
	}
	modeMu.Lock() // come selfUpdate: non chiudo Tasso a metà di un cambio di modalità
	cmd := exec.Command(setup, "--aggiorna-silenzioso", "--riapri-tasso")
	cmd.Dir = filepath.Dir(setup)
	if err := cmd.Start(); err != nil {
		modeMu.Unlock()
		fallito("Non riesco ad avviare l'aggiornamento. Riprova più tardi.")
		return
	}
	chiusuraAgg.Store(true)
	trayStop()
	wv.Dispatch(func() { wv.Terminate() })
}

// provaSetup: "TassoSetup.exe --aggiorna-silenzioso --solo-controlla" (nessuna finestra, nessuna
// modifica). 0 c'è da aggiornare, 2 niente da fare, altro = errore.
func provaSetup(exe string) int {
	ctx, annulla := context.WithTimeout(context.Background(), 2*time.Minute)
	defer annulla()
	cmd := exec.CommandContext(ctx, exe, "--aggiorna-silenzioso", "--solo-controlla")
	cmd.Dir = filepath.Dir(exe)
	err := cmd.Run()
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ctx.Err() == nil {
		return ee.ExitCode()
	}
	return 1
}

// cleanEcoSetup toglie da Programmi le copie dell'installatore rimaste da un aggiornamento
// (riprova per un po': l'installatore che ha riaperto Tasso si sta ancora chiudendo).
func cleanEcoSetup() {
	pf := programFilesDir()
	for i := 0; i < 15; i++ {
		resti, _ := filepath.Glob(filepath.Join(pf, "tasso-verifica-*.exe"))
		rimasti := 0
		for _, p := range resti {
			if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
				continue
			}
			if strings.HasSuffix(filepath.Base(p), "-"+strconv.Itoa(os.Getpid())+".exe") {
				continue // la mia, se sto controllando proprio adesso
			}
			if os.Remove(p) != nil {
				rimasti++
			}
		}
		if rimasti == 0 {
			return
		}
		time.Sleep(4 * time.Second)
	}
}

// ---------------- Tasso.exe --controlla-aggiornamenti ----------------

// segnalaControllaAgg: Tasso è già aperto, gli dico di aprire Aggiornamenti e controllare.
func segnalaControllaAgg() {
	n, _ := windows.UTF16PtrFromString(eventoControllaAgg)
	if ev, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, n); err == nil {
		_ = windows.SetEvent(ev)
		windows.CloseHandle(ev)
	}
}

// ascoltaControllaAgg: la copia aperta aspetta le richieste di Tasso.exe --controlla-aggiornamenti.
func ascoltaControllaAgg() {
	n, _ := windows.UTF16PtrFromString(eventoControllaAgg)
	ev, err := windows.CreateEvent(nil, 0, 0, n)
	if err != nil || ev == 0 {
		return
	}
	go func() {
		defer guard("ecoupdate.go:ascoltaControllaAgg")
		for {
			if r, _ := windows.WaitForSingleObject(ev, windows.INFINITE); r != windows.WAIT_OBJECT_0 {
				return
			}
			trayApri()
			emit("onControllaAgg", map[string]interface{}{})
		}
	}()
}
