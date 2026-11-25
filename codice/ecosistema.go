package main

// Tasso dentro l'Ecosistema Tasso (installatore unico "Tasso Setup", vedi ecosistema/installatore).
// L'installatore scrive:
//   - HKLM\Software\Tasso\Modalita = gioco | ufficio | entrambi (cosa hai installato: Tasso Gaming,
//     Tasso Ufficio o tutti e due);
//   - HKCU\Software\Tasso\Lingua (la lingua scelta nell'installatore, la leggono anche Zip e PDF);
//   - <cartella>\VERSIONE accanto a Tasso.exe, <cartella>\Disinstalla Tasso.exe e la voce
//     "Tasso" in "Programmi e funzionalità"; Tasso Zip e Tasso PDF in <cartella>\Tasso Zip e
//     <cartella>\Tasso PDF.
// Qui Tasso legge queste cose. Senza installatore (Tasso scaricato da solo) tutto resta come prima.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	chiaveEcosistema   = `Software\Tasso`
	chiaveDisinstalla  = `Software\Microsoft\Windows\CurrentVersion\Uninstall\Tasso`
	nomeDisinstallaEco = "Disinstalla Tasso.exe"
	argChiudi          = "--chiudi"
)

// ---------------- Modalità scelta nell'installatore ----------------

var (
	modInstOnce sync.Once
	modInst     string
)

// modalitaInstallatore: "gioco", "ufficio", "entrambi" o "" (niente installatore, o valore strano).
// Letta una volta sola: cambia solo con l'installatore, che prima chiude Tasso.
func modalitaInstallatore() string {
	modInstOnce.Do(func() {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, chiaveEcosistema, registry.QUERY_VALUE|registry.WOW64_64KEY)
		if err != nil {
			return
		}
		defer k.Close()
		v, _, err := k.GetStringValue("Modalita")
		if err != nil {
			return
		}
		switch v = strings.ToLower(strings.TrimSpace(v)); v {
		case "gioco", "ufficio", "entrambi":
			modInst = v
		}
	})
	return modInst
}

// modalitaFissa: "gioco" o "ufficio" se l'installatore ha messo solo quella (allora non si cambia
// modalità da Tasso), "" se si può cambiare (tutte e due installate, o Tasso senza installatore).
func modalitaFissa() string {
	if m := modalitaInstallatore(); m == "gioco" || m == "ufficio" {
		return m
	}
	return ""
}

// modalitaPermessa: con una modalità fissa si può scegliere solo quella.
func modalitaPermessa(p string) bool {
	f := modalitaFissa()
	return f == "" || p == f
}

const msgModalitaFissa = "Su questo PC è installata solo questa modalità di Tasso. Per avere anche l'altra riapri l'installatore di Tasso (Tasso Setup)."

// applicaModalitaInstallatore: da chiamare subito dopo loadSettings, prima di aprire la finestra.
//   - Nessuna scelta alla prima apertura (profile vuoto): uso quella dell'installatore e la domanda
//     iniziale non compare. Restituisce la modalità da applicare anche al PC (come il pulsante
//     "Continua" della prima apertura), "" se non c'è niente da fare.
//   - Modalità fissa diversa da quella salvata (per esempio hai tolto Tasso Gaming con il
//     disinstallatore e resta Ufficio): Tasso passa a quella installata, così non resti in una
//     modalità che non puoi più cambiare. Cambia solo cosa vedi, come il menu delle modalità
//     (cambiata = true: a finestra aperta va chiamato lookChanged, come per il menu).
func applicaModalitaInstallatore() (pc string, cambiata bool) {
	m := modalitaInstallatore()
	if m == "" {
		return "", false
	}
	setMu.Lock()
	prof := settings.Profile
	setMu.Unlock()
	switch {
	case prof == "":
		updateSettings(func(s *Settings) {
			s.Onboarded = true
			s.Profile = m
		})
		return m, false
	case m == "ufficio" && prof != "ufficio":
		updateSettings(func(s *Settings) { s.Onboarded, s.Profile = true, "ufficio" })
		return "", true
	case m == "gioco" && prof == "ufficio":
		updateSettings(func(s *Settings) { s.Onboarded, s.Profile = true, "gioco" })
		return "", true
	}
	return "", false
}

// ---------------- Lingua scelta nell'installatore ----------------

// lingueDisponibili: le lingue di Tasso. "it" è quella scritta nel codice, le altre hanno il loro
// file i18n/<codice>.json. Per aggiungerne una: il file i18n/xx.json (stesse chiavi di en.json,
// lo controlla i18n_lingue_test.go), il suo codice qui, in go:embed di i18n.go, in linguaDaLangID
// (i18n.go) e il bottone nella finestra (data-lang in ui/index.html).
func lingueDisponibili() []string { return []string{"it", "en", "es", "fr", "de"} }

func linguaDisponibile(l string) bool {
	for _, x := range lingueDisponibili() {
		if x == l {
			return true
		}
	}
	return false
}

var (
	linguaInstOnce sync.Once
	linguaInst     string
)

// linguaInstallatore: HKCU\Software\Tasso\Lingua portata a una lingua di Tasso: se Tasso ce l'ha
// quella, altrimenti l'inglese (es. "pt" → "en" finché non c'è i18n/pt.json). "" se non c'è.
func linguaInstallatore() string {
	linguaInstOnce.Do(func() {
		k, err := registry.OpenKey(registry.CURRENT_USER, chiaveEcosistema, registry.QUERY_VALUE)
		if err != nil {
			return
		}
		defer k.Close()
		v, _, err := k.GetStringValue("Lingua")
		if err != nil {
			return
		}
		v = strings.ToLower(strings.TrimSpace(v))
		if i := strings.IndexAny(v, "-_"); i > 0 {
			v = v[:i] // "pt-BR" → "pt"
		}
		if v == "" {
			return
		}
		if linguaDisponibile(v) {
			linguaInst = v
		} else {
			linguaInst = "en"
		}
	})
	return linguaInst
}

// scriviLinguaEcosistema: la lingua scelta nelle Impostazioni di Tasso va anche in
// HKCU\Software\Tasso\Lingua, così Tasso Zip e Tasso PDF la seguono (se lì non ne hai scelta
// un'altra) e il prossimo aggiornamento dell'installatore parla la stessa lingua.
func scriviLinguaEcosistema(l string) {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, chiaveEcosistema, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	_ = k.SetStringValue("Lingua", l)
}

// ---------------- Cartella di Tasso in Programmi ----------------

var (
	cartTassoOnce sync.Once
	cartTasso     string
)

// cartellaTasso: dove Tasso tiene le sue cose da amministratore (lavoro, lavoro\tmp, lavoro\ia,
// lavoro\ufficio, driver, strumenti, la copia per le attività pianificate e VERSIONE).
// Di base è C:\Program Files\Tasso. Se l'installatore ha messo Tasso in un'altra cartella DENTRO
// Programmi (lì scrivono solo gli amministratori) e Tasso parte da lì (accanto c'è VERSIONE), uso
// quella. Una cartella fuori da Programmi (es. D:\Tasso) non la uso mai per queste cose: un
// programma normale potrebbe cambiarne i file, e Tasso li usa da amministratore; lì resta
// C:\Program Files\Tasso, come dice anche l'installatore.
func cartellaTasso() string {
	cartTassoOnce.Do(func() {
		pf := programFilesDir()
		std := filepath.Join(pf, "Tasso")
		cartTasso = std
		exe, err := os.Executable()
		if err != nil || exe == "" {
			return
		}
		exe, _ = filepath.Abs(exe)
		dir := filepath.Clean(filepath.Dir(exe))
		if strings.EqualFold(dir, std) || !strings.EqualFold(filepath.Base(exe), "Tasso.exe") {
			return
		}
		protetta := false
		for _, r := range []string{pf, cartellaNota(windows.FOLDERID_ProgramFilesX86)} {
			if r != "" && dentroDi(dir, r) && !strings.EqualFold(strings.TrimRight(dir, `\`), strings.TrimRight(r, `\`)) {
				protetta = true
			}
		}
		if !protetta || strings.ContainsAny(dir, "\"%&|<>^\r\n") {
			return
		}
		cartellaVera := func(p string) bool {
			fi, err := os.Lstat(p)
			return err == nil && fi.Mode().IsDir()
		}
		fi, err := os.Lstat(filepath.Join(dir, "VERSIONE"))
		if err != nil || !fi.Mode().IsRegular() || !cartellaVera(dir) {
			return
		}
		// Il lavoro di prima (es. la Modalità riunione accesa, con le impostazioni da rimettere) sta
		// già in C:\Program Files\Tasso: finché la nuova cartella non ha il suo, resto lì.
		if cartellaVera(filepath.Join(std, "lavoro")) && !cartellaVera(filepath.Join(dir, "lavoro")) {
			return
		}
		cartTasso = dir
	})
	return cartTasso
}

// ---------------- Disinstallatore dell'ecosistema ----------------

// disinstallatoreEcosistema: "Disinstalla Tasso.exe" dell'installatore, se c'è, e la sua cartella.
// Lo cerco accanto a Tasso (cartellaTasso), in C:\Program Files\Tasso e nella voce "Tasso" di
// "Programmi e funzionalità" (HKLM: la scrivono solo gli amministratori). Va bene solo un file vero
// con quel nome esatto, con accanto tasso-setup.json (l'elenco di cosa ha installato).
func disinstallatoreEcosistema() (exe, cartella string) {
	var cand []string
	cand = append(cand, filepath.Join(cartellaTasso(), nomeDisinstallaEco), filepath.Join(programFilesDir(), "Tasso", nomeDisinstallaEco))
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, chiaveDisinstalla, registry.QUERY_VALUE|registry.WOW64_64KEY); err == nil {
		if v, _, err := k.GetStringValue("UninstallString"); err == nil {
			v = strings.TrimSpace(v)
			if strings.HasPrefix(v, `"`) {
				if i := strings.Index(v[1:], `"`); i >= 0 {
					v = v[1 : i+1]
				}
			}
			cand = append(cand, v)
		}
		if v, _, err := k.GetStringValue("InstallLocation"); err == nil && strings.TrimSpace(v) != "" {
			cand = append(cand, filepath.Join(strings.TrimSpace(v), nomeDisinstallaEco))
		}
		k.Close()
	}
	for _, c := range cand {
		if c == "" || !filepath.IsAbs(c) || strings.HasPrefix(c, `\\`) || strings.ContainsAny(c, "\"%&|<>^\r\n") {
			continue
		}
		c = filepath.Clean(c)
		if !strings.EqualFold(filepath.Base(c), nomeDisinstallaEco) {
			continue
		}
		fi, err := os.Lstat(c)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		if st, err := os.Lstat(filepath.Join(filepath.Dir(c), "tasso-setup.json")); err != nil || !st.Mode().IsRegular() {
			continue
		}
		return c, filepath.Dir(c)
	}
	return "", ""
}

// dentroProgrammi: p è dentro la vera cartella Programmi (o Programmi (x86)), dove scrivono solo
// gli amministratori.
func dentroProgrammi(p string) bool {
	for _, r := range []string{programFilesDir(), cartellaNota(windows.FOLDERID_ProgramFilesX86)} {
		if r != "" && dentroDi(p, r) && !strings.EqualFold(strings.TrimRight(p, `\`), strings.TrimRight(r, `\`)) {
			return true
		}
	}
	return false
}

// disinstAllaChiusura: il disinstallatore da aprire quando la finestra di Tasso si è chiusa
// (dopo "Togli Tasso dal PC", vedi uninstall.go). Si apre una volta sola.
var (
	disinstAllaChiusura atomic.Value // string
	disinstAperto       sync.Once
)

// apriDisinstallatoreSeServe apre il disinstallatore dell'ecosistema con già spuntati Tasso
// Gaming e Tasso Ufficio (Zip e PDF restano se non li spunti tu). Se sta in Programmi lo avvio
// direttamente (Tasso è già amministratore); se sta in una cartella dove può scrivere chiunque lo
// faccio aprire a Esplora file, così Windows chiede il permesso (e lì non si possono passare
// argomenti: si apre con le sue scelte di base).
func apriDisinstallatoreSeServe() {
	exe, _ := disinstAllaChiusura.Load().(string)
	if exe == "" {
		return
	}
	disinstAperto.Do(func() {
		defer guard("apriDisinstallatore")
		if !fileExists(exe) {
			return
		}
		if dentroProgrammi(exe) {
			verb, _ := windows.UTF16PtrFromString("open")
			f, _ := windows.UTF16PtrFromString(exe)
			a, _ := windows.UTF16PtrFromString("--disinstalla --scegli=gaming,ufficio")
			d, _ := windows.UTF16PtrFromString(filepath.Dir(exe))
			if windows.ShellExecute(0, verb, f, a, d, windows.SW_SHOWNORMAL) == nil {
				return
			}
		}
		openPath(explorerExe(), "\""+exe+"\"")
	})
}

// ---------------- Tasso.exe --chiudi ----------------
//
// L'installatore (o chiunque sia amministratore) può chiedere al Tasso aperto di chiudersi bene:
// la copia nuova trova quella aperta con lo stesso mutex della copia unica (Local\TassoSingleInstance),
// la sveglia con l'evento Local\TassoChiudi e aspetta che il mutex si liberi (= Tasso chiuso).
// Se Tasso sta lavorando (pulizia, aggiornamento, Togli Tasso) rifiuta: accende l'evento
// Local\TassoChiudiNo e mostra l'avviso "Aspetta la fine della pulizia".
// Codice di uscita: 0 chiuso (o non era aperto), 1 rifiutato o non chiuso entro 20 secondi.

const (
	eventoChiudi   = `Local\TassoChiudi`
	eventoChiudiNo = `Local\TassoChiudiNo`
)

func chiudiCopiaAperta() int {
	runtime.LockOSThread() // il mutex appartiene al filo che lo prende
	mn, _ := windows.UTF16PtrFromString(`Local\TassoSingleInstance`)
	h, err := windows.CreateMutex(nil, false, mn)
	if h == 0 {
		return 1 // non posso vedere la copia aperta (permessi?)
	}
	defer windows.CloseHandle(h)
	if err != windows.ERROR_ALREADY_EXISTS {
		return 0 // Tasso non è aperto: niente da chiudere
	}
	nn, _ := windows.UTF16PtrFromString(eventoChiudiNo)
	no, _ := windows.CreateEvent(nil, 1, 0, nn) // a reset manuale: resta acceso finché lo leggo
	if no != 0 {
		defer windows.CloseHandle(no)
		_ = windows.ResetEvent(no) // resto di una richiesta di prima
	}
	en, _ := windows.UTF16PtrFromString(eventoChiudi)
	chiesto := false
	fine := time.Now().Add(20 * time.Second)
	for time.Now().Before(fine) {
		if !chiesto {
			// Tasso si sta ancora aprendo: l'evento compare quando la finestra c'è. Riprovo.
			if ev, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, en); err == nil {
				chiesto = windows.SetEvent(ev) == nil
				windows.CloseHandle(ev)
			}
		}
		hh := []windows.Handle{h}
		if no != 0 {
			hh = append(hh, no)
		}
		r, err := windows.WaitForMultipleObjects(hh, false, 500)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		switch r {
		case windows.WAIT_OBJECT_0, windows.WAIT_ABANDONED:
			_ = windows.ReleaseMutex(h)
			return 0 // il mutex è libero: la copia aperta si è chiusa
		case windows.WAIT_OBJECT_0 + 1:
			return 1 // rifiutato: Tasso sta lavorando
		}
	}
	return 1
}

// tassoNonSiChiude: c'è un lavoro che non si deve interrompere (come la voce "Chiudi Tasso"
// dell'icona vicino all'orologio). Dopo "Togli Tasso dal PC" finito si chiude sempre.
func tassoNonSiChiude() bool {
	if togliFinito.Load() || chiusuraAgg.Load() {
		return false // dopo "Togli Tasso dal PC", o Tasso si chiude per l'aggiornamento (ecoupdate.go)
	}
	busyMu.Lock()
	togli := busy["uninstall-tasso"]
	busyMu.Unlock()
	return isRunning() || togli || updating.Load()
}

var togliFinito atomic.Bool

// ascoltaChiudi: la copia aperta aspetta le richieste di Tasso.exe --chiudi.
// Da chiamare quando la finestra c'è (wv pronto).
func ascoltaChiudi() {
	en, _ := windows.UTF16PtrFromString(eventoChiudi)
	ev, err := windows.CreateEvent(nil, 0, 0, en)
	if err != nil || ev == 0 {
		return
	}
	go func() {
		defer guard("ecosistema.go:ascoltaChiudi")
		for {
			if r, _ := windows.WaitForSingleObject(ev, windows.INFINITE); r != windows.WAIT_OBJECT_0 {
				return
			}
			if tassoNonSiChiude() {
				nn, _ := windows.UTF16PtrFromString(eventoChiudiNo)
				if no, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, nn); err == nil {
					_ = windows.SetEvent(no)
					windows.CloseHandle(no)
				}
				trayApri()
				trayAvviso("Tasso", "Aspetta la fine della pulizia prima di chiudere.", false)
				continue
			}
			salvaAlloSpegnimento() // impostazioni e memoria delle traduzioni, come allo spegnimento
			chiudiTasso()
			return
		}
	}()
}
