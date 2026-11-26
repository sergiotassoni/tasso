package main

// BOOST (3.1): un solo pulsante che prepara il PC per giocare, misura gli FPS veri e rimette
// tutto com'era. Usa regolazioni che il Tasso sapeva già fare e le mette insieme. Non tocca i file
// del gioco né la sua memoria.
//
// Onestà (dopo la revisione con 4 revisori + scettico):
//   - Le regolazioni di Windows di solito danno da 0 al 10% di FPS in più; aiutano soprattutto sui
//     PC con tanti programmi aperti e sui portatili. I salti enormi delle pubblicità (92 -> 218)
//     vengono quasi sempre da grafica diversa, DLSS/FSR o generazione di fotogrammi.
//   - Rete, priorità ai giochi e ottimizzazioni in finestra valgono dal prossimo avvio del gioco
//     (la priorità a volte dal riavvio del PC): per questo il Boost le lascia accese finché non premi
//     "Rimetti com'era", invece di accenderle e spegnerle a ogni partita.
//   - Niente svuotamento della memoria in standby durante il gioco: toglierebbe al gioco i suoi file
//     già in memoria e farebbe più scatti, non meno.
//   - La misura FPS usa PresentMon di Intel (gratuito, licenza MIT, firmato da Intel): legge gli
//     eventi di Windows sui fotogrammi, come i contatori FPS, senza entrare nel gioco. Il confronto
//     prima/dopo dice "simile" quando la differenza è nella variazione normale.
//
// Boost automatico ("capacità di adattamento"): la modalità giusta per QUEL gioco la mette la
// modalità gioco automatica (Gaming, FPS, Potenza per ogni gioco, Regola con Tasso), che il Boost
// automatico accende. In più a ogni partita il Boost chiude i programmi di sincronizzazione e chat in
// background e li riapre un minuto dopo la fine. Con programmi normali non fa niente.

import (
	"bufio"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

//go:embed strumenti/PresentMon.exe
var presentMonExe []byte

// SHA-256 di PresentMon 2.4.1 x64 (release ufficiale Intel su GitHub).
const presentMonSHA = "d74183e7ae630f72cd3690be0373ecbfdc6cbb86578148aab8fa2a7166068f34"

// Comandi gaming del Boost (accesi solo se erano spenti, rispenti con "Rimetti com'era").
// Niente pianificazione GPU (vuole il riavvio e su alcuni PC peggiora). Niente "Priorità ai giochi"
// (mmcss) e "Rete più reattiva" (nagle): quasi sempre non cambiano gli FPS, quindi il Boost non li
// promette più; restano nel pannello avanzato per chi li vuole.
var boostCmds = []string{"gamedvr", "winopt"}

// boostCmdsDaRimettere: anche i comandi che il Boost accendeva nelle versioni di prima, così un
// boost.json scritto prima dell'aggiornamento si rimette comunque com'era.
var boostCmdsDaRimettere = []string{"gamedvr", "mmcss", "nagle", "winopt"}

// Programmi che il Boost chiude, solo se sono in background (senza finestra) e non sono app dello
// Store: sincronizzazione e chat. Mai Office, Outlook, Acrobat o programmi con lavoro non salvato.
var boostClosable = map[string]bool{
	"onedrive.exe": true, "dropbox.exe": true, "googledrivefs.exe": true, "icloudservices.exe": true,
	"iclouddrive.exe": true, "teams.exe": true, "zoom.exe": true, "skype.exe": true,
}

type fpsResult struct {
	Game      string  `json:"game"`
	When      string  `json:"when"`
	Boost     bool    `json:"boost"`
	Seconds   float64 `json:"seconds"`
	Frames    int     `json:"frames"`
	Avg       float64 `json:"avg"`       // FPS medi
	Low1      float64 `json:"low1"`      // 1% low: media del centesimo di fotogrammi più lenti
	Stutter   int     `json:"stutter"`   // scatti: fotogrammi lunghi più del doppio del normale
	StutterPM float64 `json:"stutterPm"` // scatti al minuto (per confrontare misure di durata diversa)
	GpuBusy   float64 `json:"gpuBusy"`   // % del tempo di ogni fotogramma in cui la scheda video lavora (alto = limite della scheda video)
	CpuTemp   float64 `json:"cpuTemp,omitempty"`
	GpuTemp   float64 `json:"gpuTemp,omitempty"`
	Throttle  bool    `json:"throttle,omitempty"` // la scheda video NVIDIA ha rallentato per il caldo
	Hz        int     `json:"hz,omitempty"`       // frequenza dello schermo durante la misura
	Spread    float64 `json:"spread,omitempty"`   // differenza % tra fotogrammi veloci e lenti (bassa = FPS fermi su un limite)
	VramPct   float64 `json:"vramPct,omitempty"`  // memoria della scheda video usata (%)
	pid       string
	path      string
}

type boostState struct {
	On         bool                    `json:"on"`                   // Boost acceso a mano
	Auto       bool                    `json:"auto"`                 // Boost automatico acceso
	Cmds       []string                `json:"cmds"`                 // comandi accesi dal Boost (da rispegnere)
	PrevMode   string                  `json:"prevMode"`             // modalità da rimettere ("" = non l'ha cambiata il Boost)
	PrevSempre bool                    `json:"prevSempre,omitempty"` // la modalità Gaming si è accesa a metà: rimettere PrevMode anche se non risulta Gaming
	PrevAG     *bool                   `json:"prevAutoGame,omitempty"`
	GpuPrev    map[string]string       `json:"gpuPrev,omitempty"`  // gioco -> scelta della scheda video di prima ("0" = nessuna)
	Mouse      *[3]int32               `json:"mouse,omitempty"`    // accelerazione del mouse di prima (solo se l'ha tolta il Boost)
	MouseReg   string                  `json:"mouseReg,omitempty"` // MouseSpeed salvato in Windows quando il Boost l'ha tolta
	Closed     []string                `json:"closed,omitempty"`   // programmi chiusi da riaprire (anche se Tasso si riavvia)
	History    map[string][]*fpsResult `json:"history,omitempty"`  // per gioco, ultime misure
}

var (
	pMessageBeep    = user32.NewProc("MessageBeep")
	pForegroundWin  = user32.NewProc("GetForegroundWindow")
	pGetPowerStatus = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemPowerStatus")

	bMu     sync.Mutex // protegge bState, bClosed, bSkip, bGame
	bState  *boostState
	bOpMu   sync.Mutex // un'accensione/spegnimento alla volta, dall'inizio alla fine
	fpsMu   sync.Mutex // una misura alla volta
	bWatch  sync.Once
	bClosed []string  // programmi chiusi da riaprire (copia in boost.json, solo nomi ammessi in boostClosable)
	bSkip   string    // gioco per cui hai premuto "Rimetti com'era": niente Boost automatico finché resta aperto
	bGame   string    // gioco della partita col Boost automatico in corso
	bOnAt   time.Time // quando è stato acceso a mano il Boost (per lo spegnimento automatico)
	// bAutoFlag: copia di bState.Auto che si legge senza bMu (serve dentro updateSettings).
	bAutoFlag atomic.Bool
)

func boostFile() string { return filepath.Join(appDir, "boost.json") }

// boostSpegniDopo: il Boost acceso a mano si spegne da solo ("Rimetti com'era") dopo questo tempo
// di fila senza giochi aperti. È l'unico punto dove si sceglie: i testi della finestra lo leggono da
// boostInfo ("offMin").
const boostSpegniDopo = 15 * time.Minute

// boostDaSpegnere decide se il Boost acceso a mano va spento da solo. Il conteggio parte
// dall'accensione (accesoAlle) e si azzera ogni volta che c'è un gioco aperto (ultimoGioco = l'ultima
// volta che ne ho visto uno). Mai durante una pulizia o un aggiornamento (occupato): aspetta la fine.
func boostDaSpegnere(acceso, giocoAperto, occupato bool, accesoAlle, ultimoGioco, adesso time.Time) bool {
	if !acceso || giocoAperto || occupato {
		return false
	}
	da := accesoAlle
	if ultimoGioco.After(da) {
		da = ultimoGioco
	}
	return !da.IsZero() && adesso.Sub(da) >= boostSpegniDopo
}

// boostSpentoTesto: l'avviso quando il Boost si spegne da solo.
func boostSpentoTesto() string {
	return "Boost spento da solo: nessun gioco aperto da " + strconv.Itoa(int(boostSpegniDopo/time.Minute)) + " minuti. Il PC è tornato com'era."
}

// safeName: i nomi dei giochi arrivano da fuori (nome del programma): tengo solo caratteri semplici.
// nomeGiocoSicuro: come safeName, ma per un gioco aperto con il nome tutto in caratteri non latini
// (cinese, giapponese, cirillico) dà "gioco" invece di "": il gioco conta comunque come aperto.
func nomeGiocoSicuro(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	n := safeName(s)
	base := strings.TrimSpace(strings.TrimSuffix(strings.ToLower(n), ".exe"))
	if strings.Trim(base, " .-_") == "" { // rimasta solo l'estensione o la punteggiatura
		return "gioco"
	}
	return n
}

func safeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= 60 {
			break
		}
		if r == ' ' || r == '.' || r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// loadBoost legge lo stato salvato. boost.json si può modificare senza essere amministratore:
// tengo solo valori ammessi. Chiamare con bMu preso.
func loadBoost() *boostState {
	if bState != nil {
		return bState
	}
	raw := leggiBoostGrezzo(boostFile())
	s := &boostState{On: raw.On, Auto: raw.Auto, PrevAG: raw.PrevAG, History: map[string][]*fpsResult{}}
	for _, c := range raw.Cmds {
		if hasStr(boostCmdsDaRimettere, c) && !hasStr(s.Cmds, c) {
			s.Cmds = append(s.Cmds, c)
		}
	}
	for k, v := range raw.GpuPrev {
		if strings.HasSuffix(strings.ToLower(k), ".exe") && len(k) < 400 && len(v) < 300 && (v == "0" || strings.HasPrefix(v, "1")) {
			if s.GpuPrev == nil {
				s.GpuPrev = map[string]string{}
			}
			s.GpuPrev[k] = v
		}
	}
	if _, ok := modeScripts[raw.PrevMode]; ok {
		s.PrevMode = raw.PrevMode
		s.PrevSempre = raw.PrevSempre
	}
	// Mouse di prima: se il Tasso si è chiuso a metà partita, all'avvio lo rimetto com'era.
	if m := raw.Mouse; m != nil && m[0] >= 0 && m[0] <= 99 && m[1] >= 0 && m[1] <= 99 && m[2] >= 0 && m[2] <= 2 {
		v := *m
		s.Mouse = &v
		s.MouseReg = raw.MouseReg
	}
	// Programmi chiusi dal Boost: tengo solo percorsi completi di programmi dell'elenco boostClosable
	// (vengono riaperti come utente normale tramite Esplora file).
	for _, p := range raw.Closed {
		if len(p) < 400 && filepath.IsAbs(p) && boostClosable[strings.ToLower(filepath.Base(p))] && !hasStr(s.Closed, p) && len(s.Closed) < len(boostClosable) {
			s.Closed = append(s.Closed, p)
		}
	}
	if len(bClosed) == 0 {
		bClosed = append([]string(nil), s.Closed...)
	}
	for k, v := range raw.History {
		if k == "" || k != strings.ToLower(safeName(k)) || len(v) > 10 {
			continue
		}
		for _, r := range v {
			if r != nil {
				r.Game = safeName(r.Game)
				s.History[k] = append(s.History[k], r)
			}
		}
	}
	bState = s
	bAutoFlag.Store(s.Auto)
	return s
}

func saveBoost() {
	if bState == nil {
		return
	}
	bAutoFlag.Store(bState.Auto)
	if b, err := json.MarshalIndent(bState, "", "  "); err == nil {
		_ = scriviBoostSicuro(boostFile(), b)
	}
}

// scriviBoostSicuro: boost.json dice cosa rimettere dopo il Boost, quindi non deve mai restare a metà
// (PC spento mentre scrive). Scrivo in un file a parte, sposto quello vecchio in boost.json.bak e poi
// metto il nuovo al suo posto: in ogni momento c'è almeno un file intero.
func scriviBoostSicuro(path string, b []byte) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if _, err := os.Stat(path); err == nil {
		_ = os.Remove(path + ".bak")
		_ = os.Rename(path, path+".bak")
	}
	return os.Rename(tmp, path)
}

// leggiBoostGrezzo: boost.json così com'è; se manca o è rovinato uso la copia di prima (boost.json.bak).
// Un file rovinato lo tengo da parte (boost.json.rotto) invece di ripartire da zero in silenzio.
func leggiBoostGrezzo(path string) *boostState {
	for i, p := range []string{path, path + ".bak"} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		raw := &boostState{}
		if json.Unmarshal(b, raw) == nil {
			return raw
		}
		if i == 0 {
			_ = os.Remove(path + ".rotto")
			_ = os.Rename(path, path+".rotto")
		}
	}
	return &boostState{}
}

func boostInfo() map[string]interface{} {
	bMu.Lock()
	defer bMu.Unlock()
	s := loadBoost()
	// pending: è rimasto qualcosa da rimettere (regolazioni di Windows, modalità, scheda video, mouse,
	// programmi chiusi), anche con il Boost spento: "Rimetti com'era" serve ancora.
	pending := len(s.Cmds) > 0 || s.PrevMode != "" || len(s.GpuPrev) > 0 || s.Mouse != nil || len(bClosed) > 0
	return map[string]interface{}{"on": s.On, "auto": s.Auto, "game": bGame, "battery": onBattery(), "gpuFix": len(s.GpuPrev) > 0, "pending": pending, "offMin": int(boostSpegniDopo / time.Minute)}
}

// onBattery: portatile senza caricatore. In quel caso niente piano energetico al massimo.
func onBattery() bool {
	var st struct {
		ACLineStatus, BatteryFlag, BatteryLifePercent, SystemStatusFlag byte
		BatteryLifeTime, BatteryFullLifeTime                            uint32
	}
	r, _, _ := pGetPowerStatus.Call(uintptr(unsafe.Pointer(&st)))
	return r != 0 && st.ACLineStatus == 0
}

func removeStr(list []string, v string) []string {
	var out []string
	for _, x := range list {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

// ---------------- Parti del Boost ----------------

// Come si chiamano, per te, i comandi del Boost.
var boostCmdNames = map[string]string{
	"gamedvr": "registrazione in background spenta", "mmcss": "priorità ai giochi",
	"nagle": "rete più reattiva", "winopt": "giochi in finestra",
}

// boostCmdsOn accende i comandi spenti e si ricorda quali. Salvo PRIMA di cambiare: se il Tasso si
// chiude a metà, "Rimetti com'era" sa comunque cosa rispegnere. Restituisce i comandi accesi.
func boostCmdsOn() []string {
	st := gamingState()
	var done []string
	for _, id := range boostCmds {
		if on, _ := st[id].(bool); on {
			continue
		}
		if id == "winopt" && (winBuild() < 22000 || winoptSceltoSpento(dxGlobal())) {
			continue // Windows 10, o l'hai spenta tu nelle impostazioni della grafica: non la riaccendo
		}
		bMu.Lock()
		s := loadBoost()
		if !s.On && !s.Auto { // spento nel frattempo ("Rimetti com'era" o Boost automatico spento)
			bMu.Unlock()
			break
		}
		if !hasStr(s.Cmds, id) {
			s.Cmds = append(s.Cmds, id)
		}
		saveBoost()
		bMu.Unlock()
		if strings.HasPrefix(gamingSet(id, true), "ok:") {
			done = append(done, id)
		} else {
			bMu.Lock()
			s.Cmds = removeStr(s.Cmds, id)
			saveBoost()
			bMu.Unlock()
		}
	}
	return done
}

// boostCmdsText: "Accese le regolazioni per giocare (…)" con solo quelle accese davvero.
func boostCmdsText(ids []string) string {
	var names []string
	for _, id := range ids {
		names = append(names, boostCmdNames[id])
	}
	return "Accese le regolazioni per giocare (" + strings.Join(names, ", ") + "). Valgono dal prossimo avvio del gioco e restano finché premi \"Rimetti com'era\"."
}

// boostCmdsOff rispegne solo i comandi accesi dal Boost. Se uno non si spegne resta nell'elenco,
// così si può riprovare.
func boostCmdsOff() int {
	bMu.Lock()
	cmds := append([]string(nil), loadBoost().Cmds...)
	bMu.Unlock()
	setMu.Lock()
	active := map[string]bool{}
	for k, v := range settings.GamingCmds {
		active[k] = v
	}
	setMu.Unlock()
	n := 0
	for _, id := range cmds {
		ok := true
		if active[id] { // se "Rimetti com'era" generale l'ha già spento, non lo tocco
			ok = strings.HasPrefix(gamingSet(id, false), "ok:")
		}
		if ok {
			n++
			bMu.Lock()
			s := loadBoost()
			s.Cmds = removeStr(s.Cmds, id)
			saveBoost()
			bMu.Unlock()
		}
	}
	return n
}

// closeBackground chiude con gentilezza (mai di forza) i programmi di sincronizzazione e chat che
// girano senza finestra. Restituisce i nomi chiusi.
func closeBackground() []string {
	withWin := pidsWithWindow(1, 1)
	keep := boostKeepSet()
	var names []string
	for exe := range boostClosable {
		if keep[exe] {
			continue // "Non chiudere mai anche": scelto da te
		}
		pids := pidsByExe(exe)
		if len(pids) == 0 {
			continue
		}
		path, skip := "", false
		for _, p := range pids {
			if withWin[p] {
				skip = true
			}
			if pp := processPath(p); pp != "" {
				path = pp
			}
		}
		if skip || path == "" || strings.Contains(strings.ToLower(path), `\windowsapps\`) {
			continue
		}
		_, _ = runHidden("taskkill.exe", "/IM", exe) // senza /F: chiusura normale, finiscono di salvare
		for i := 0; i < 16 && len(pidsByExe(exe)) > 0; i++ {
			time.Sleep(500 * time.Millisecond)
		}
		if len(pidsByExe(exe)) == 0 {
			bMu.Lock()
			if !hasStr(bClosed, path) {
				bClosed = append(bClosed, path)
			}
			st := loadBoost()
			st.Closed = append([]string(nil), bClosed...)
			saveBoost()
			bMu.Unlock()
			names = append(names, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
		}
	}
	return names
}

// reopenBackground riapre i programmi chiusi (se non sono già ripartiti da soli), come utente
// normale tramite Esplora file.
// reopenMu: una riapertura alla volta, dall'inizio alla fine. Le chiamate possono arrivare quasi
// insieme (avvio dopo un crash, watcher a gioco finito, "Rimetti com'era"): senza, tutte e due
// leggerebbero lo stesso elenco e aprirebbero due volte lo stesso programma (explorer.exe lo avvia
// dopo un attimo, quindi il controllo "è già aperto" non basta).
var reopenMu sync.Mutex

func reopenBackground() []string {
	reopenMu.Lock()
	defer reopenMu.Unlock()
	bMu.Lock()
	list := append([]string(nil), bClosed...)
	bMu.Unlock()
	win, err := windows.GetSystemWindowsDirectory()
	if err != nil {
		return nil
	}
	explorer := filepath.Join(win, "explorer.exe")
	var names, restano []string
	for _, p := range list {
		if len(pidsByExe(filepath.Base(p))) > 0 || !fileExists(p) {
			continue // già ripartito da solo, o non c'è più: niente da riaprire
		}
		if openPath(explorer, "\""+p+"\"") {
			names = append(names, strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)))
		} else {
			restano = append(restano, p) // non si è riaperto: resta nell'elenco, "Rimetti com'era" ci riprova
		}
	}
	// Tolgo dall'elenco solo quelli sistemati (e tengo quelli chiusi nel frattempo da un'altra parte).
	bMu.Lock()
	st := loadBoost()
	bClosed = tieniDaRiaprire(bClosed, list, restano)
	st.Closed = append([]string(nil), bClosed...)
	saveBoost()
	bMu.Unlock()
	return names
}

// riapriAllAvvio: dopo un crash con il Boost già spento riapro i programmi chiusi solo se non c'è
// un gioco aperto (riconosciuto da Tasso o un programma a schermo intero davanti).
func riapriAllAvvio(daRiaprire, giocoAperto, schermoIntero bool) bool {
	return daRiaprire && !giocoAperto && !schermoIntero
}

// tieniDaRiaprire: dall'elenco di adesso tolgo quelli che ho provato a riaprire (provati), tranne quelli
// non riusciti (restano). Quelli aggiunti nel frattempo restano.
func tieniDaRiaprire(adesso, provati, restano []string) []string {
	var out []string
	for _, p := range adesso {
		if !hasStr(provati, p) || hasStr(restano, p) {
			out = append(out, p)
		}
	}
	return out
}

// ---------------- Accendi / spegni (pulsante) ----------------

// boostOnOK: come boostOn, e dice anche se il Boost è stato acceso adesso (false = rifiutato:
// Tasso spento o Boost già acceso).
func boostOnOK() ([]string, bool) {
	bOpMu.Lock()
	defer bOpMu.Unlock()
	setMu.Lock()
	paused, mode := settings.Paused, settings.Mode
	setMu.Unlock()
	if paused {
		return []string{"Tasso è spento: riaccendilo per usare il Boost."}, false
	}
	bMu.Lock()
	s := loadBoost()
	if s.On || bGame != "" { // acceso a mano o dal Boost automatico durante una partita
		bMu.Unlock()
		return []string{"Il Boost è già acceso."}, false
	}
	s.On = true
	bOnAt = time.Now() // da qui parte il conteggio dello spegnimento automatico
	saveBoost()
	bMu.Unlock()
	startBoostWatcher() // guarda i giochi aperti e spegne il Boost dopo boostSpegniDopo senza giochi

	var msgs []string
	if ids := boostCmdsOn(); len(ids) > 0 {
		msgs = append(msgs, boostCmdsText(ids))
	}
	if onBattery() {
		msgs = append(msgs, "Sei a batteria: lascio il risparmio energetico. Collega il caricatore per avere tutta la potenza.")
	} else if mode != "gaming" && mode != "fps" {
		prev := mode
		if _, ok := modeScripts[prev]; !ok {
			prev = "normale"
		}
		bMu.Lock()
		s.PrevMode = prev
		saveBoost()
		bMu.Unlock()
		if setMode("gaming") {
			msgs = append(msgs, "Modalità Gaming: processore al massimo, notifiche in pausa.")
		} else if setMode(prev) {
			// La modalità Gaming si è fermata a metà (piano energetico o notifiche già cambiati):
			// rimetto subito la modalità di prima, così non resta niente di mezzo.
			bMu.Lock()
			s.PrevMode, s.PrevSempre = "", false
			saveBoost()
			bMu.Unlock()
			msgs = append(msgs, "Non sono riuscito a mettere la modalità Gaming: il resto del Boost è acceso.")
		} else {
			// Non riesco nemmeno a rimettere quella di prima: lo ricordo, e "Rimetti com'era" ci riprova.
			bMu.Lock()
			s.PrevSempre = true
			saveBoost()
			bMu.Unlock()
			msgs = append(msgs, "Non sono riuscito a mettere la modalità Gaming: il resto del Boost è acceso.")
		}
	}
	if names := closeBackground(); len(names) > 0 {
		msgs = append(msgs, "Chiusi in background: "+strings.Join(names, ", ")+". Li riapro quando premi \"Rimetti com'era\".")
	}
	if m := mouseAccelOff(safeName(runningGame())); m != "" {
		msgs = append(msgs, m) // il watcher rimette il mouse quando chiudi il gioco
	}
	if len(msgs) == 0 {
		msgs = append(msgs, "Il PC era già pronto per giocare: non c'era niente da cambiare.")
	}
	return msgs, true
}

// restoreBoostMode rimette la modalità di prima, se l'ha cambiata il Boost e non l'hai cambiata tu.
// fatto: la modalità è stata rimessa; fallito: andava rimessa ma setMode non ci è riuscito.
func restoreBoostMode(prev string, sempre bool) (fatto, fallito bool) {
	if prev == "" {
		return false, false
	}
	setMu.Lock()
	cur, target, before := settings.Mode, settings.GameTarget, settings.ModeBeforeGame
	setMu.Unlock()
	// Partita in corso con la modalità gioco automatica: a fine gioco deve tornare la TUA modalità,
	// non "gaming" messa dal Boost.
	if before == "gaming" || before == "fps" {
		updateSettings(func(st *Settings) { st.ModeBeforeGame = prev })
	}
	if cur == "gaming" || (target != "" && cur == target) || sempre {
		ok := setMode(prev)
		return ok, !ok
	}
	return false, false
}

// boostOff ("Rimetti com'era"): spegne il Boost e anche il Boost automatico, e rimette solo
// quello che il Boost ha cambiato.
func boostOff(explicit bool) []string { return boostOffOpt(explicit, false) }

// boostOffOpt: con keepAuto (passaggio a Ufficio) annulla solo gli effetti del Boost e lascia la scelta
// "Boost automatico" com'è, così tornando a Gaming riprende da solo.
func boostOffOpt(explicit, keepAuto bool) []string {
	bOpMu.Lock()
	defer bOpMu.Unlock()
	bMu.Lock()
	s := loadBoost()
	prev, prevSempre, auto, prevAG := s.PrevMode, s.PrevSempre, s.Auto, s.PrevAG
	keepCmds := keepAuto && s.Auto // Ufficio col Boost automatico acceso: le regolazioni per giocare restano (come dice GIOCA)
	var gpuPrev map[string]string
	if explicit { // la scheda video per i giochi la rimette solo il pulsante "Rimetti com'era"
		gpuPrev = s.GpuPrev
		s.GpuPrev = nil
	}
	if keepAuto {
		s.On = false
		auto = false // la modalità gioco automatica non si tocca: serve ancora al Boost automatico
	} else {
		s.On, s.Auto, s.PrevAG = false, false, nil
		if bGame != "" {
			bSkip = bGame
		}
	}
	bGame = ""
	saveBoost()
	bMu.Unlock()

	var msgs []string
	if !keepCmds && boostCmdsOff() > 0 {
		msgs = append(msgs, "Regolazioni per giocare rimesse come prima.")
	}
	if len(gpuPrev) > 0 {
		gpuPrefRestore(gpuPrev)
		msgs = append(msgs, "Scelta della scheda video per i giochi rimessa com'era.")
	}
	// PrevMode resta in boost.json finché la modalità non è davvero rimessa: se setMode fallisce,
	// "Rimetti com'era" (o lo spegnimento automatico) ci riprova la volta dopo.
	modoFatto, modoFallito := restoreBoostMode(prev, prevSempre)
	if modoFatto {
		msgs = append(msgs, "Rimessa la modalità di prima.")
	}
	bMu.Lock()
	if st := loadBoost(); !modoFallito && st.PrevMode == prev {
		st.PrevMode, st.PrevSempre = "", false
		saveBoost()
	}
	bMu.Unlock()
	if auto && prevAG != nil && !*prevAG {
		if autoGameNeeded() {
			// La usano ancora la potenza per gioco o Regola con Tasso: resta accesa, ma solo per quei giochi.
			updateSettings(func(st *Settings) { st.AutoGameTune = true })
		} else {
			setAutoGame(false)
		}
	}
	if names := reopenBackground(); len(names) > 0 {
		msgs = append(msgs, "Riaperti: "+strings.Join(names, ", ")+".")
	}
	if m := mouseAccelRestore(); m != "" {
		msgs = append(msgs, m)
	}
	bMu.Lock()
	left, gpuLeft := len(loadBoost().Cmds), len(loadBoost().GpuPrev) > 0
	bMu.Unlock()
	switch {
	case left > 0 || modoFallito:
		msgs = append(msgs, "Qualcosa non si è potuto rimettere: premi di nuovo \"Rimetti com'era\".")
	case gpuLeft: // Boost automatico spento: la scheda video scelta per i giochi resta
		msgs = append(msgs, "Il resto è com'era prima del Boost. La scheda video scelta per i giochi la rimette \"Rimetti com'era\".")
	default:
		msgs = append(msgs, "Tutto com'era prima del Boost.")
	}
	return msgs
}

// boostOffForOffice: passando alla modalità Ufficio, se il Boost è acceso (o ha chiuso dei
// programmi) lo spengo come con "Rimetti com'era", che in Ufficio non si vede: così OneDrive,
// Teams e gli altri programmi chiusi tornano aperti.
func boostOffForOffice() {
	bMu.Lock()
	s := loadBoost()
	need := s.On || bGame != "" || len(bClosed) > 0
	bMu.Unlock()
	if !need {
		return
	}
	go func() {
		defer guard("boost.go:office")
		msgs := append([]string{"Modalità Ufficio: ho spento il Boost e rimesso com'era quello che aveva cambiato."}, boostOffOpt(false, true)...)
		emit("onBoost", map[string]interface{}{"msgs": msgs, "state": boostInfo()})
	}()
}

// boostSpegniPerUscita: chiudendo Tasso spengo il Boost acceso a mano (o quello automatico di una partita
// in corso) e riapro i programmi chiusi. La scelta "Boost automatico" resta, come passando a Ufficio.
func boostSpegniPerUscita() {
	bMu.Lock()
	s := loadBoost()
	need := boostDaRimettereAllUscita(s.On, bGame != "", len(bClosed) > 0, s.PrevMode != "")
	bMu.Unlock()
	if need {
		_ = boostOffOpt(false, true)
	}
}

// boostDaRimettere: il Boost ha ancora qualcosa di suo da rimettere (per il "Rimetti com'era" generale).
func boostDaRimettere() bool {
	bMu.Lock()
	defer bMu.Unlock()
	s := loadBoost()
	return s.On || bGame != "" || len(s.Cmds) > 0 || s.PrevMode != "" || len(s.GpuPrev) > 0 || s.Mouse != nil || len(bClosed) > 0
}

// boostDaRimettereAllUscita: c'è qualcosa del Boost che deve tornare com'era prima di chiudere Tasso.
func boostDaRimettereAllUscita(acceso, partitaAuto, programmiChiusi, modalitaDaRimettere bool) bool {
	return acceso || partitaAuto || programmiChiusi || modalitaDaRimettere
}

// primoPianoSchermoIntero: la finestra davanti copre tutto il suo schermo (gioco a schermo intero o
// senza bordi, video a schermo intero...). Il desktop e Esplora file non contano.
func primoPianoSchermoIntero() bool {
	h, _, _ := pForegroundWin.Call()
	if h == 0 {
		return false
	}
	var pid uint32
	pGetWindowPid.Call(h, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 || pid == windows.GetCurrentProcessId() || strings.EqualFold(filepath.Base(processPath(pid)), "explorer.exe") {
		return false
	}
	mon, _, _ := pMonitorFromWindow.Call(h, 2) // MONITOR_DEFAULTTONEAREST
	if mon == 0 {
		return false
	}
	var mi struct {
		Size          uint32
		Monitor, Work [4]int32
		Flags         uint32
	}
	mi.Size = uint32(unsafe.Sizeof(mi))
	if r, _, _ := pGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return false
	}
	var wr [4]int32
	if r, _, _ := pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&wr))); r == 0 {
		return false
	}
	return copreSchermo(wr, mi.Monitor)
}

// copreSchermo: il rettangolo della finestra (sinistra, alto, destra, basso) copre tutto lo schermo.
func copreSchermo(finestra, schermo [4]int32) bool {
	if schermo[2] <= schermo[0] || schermo[3] <= schermo[1] {
		return false
	}
	return finestra[0] <= schermo[0] && finestra[1] <= schermo[1] && finestra[2] >= schermo[2] && finestra[3] >= schermo[3]
}

// ---------------- Boost automatico ----------------

func setBoostAuto(on bool) string {
	if !on {
		bMu.Lock()
		manual := loadBoost().On
		bMu.Unlock()
		if !manual {
			go func() {
				defer guard("boost.go:autooff")
				emit("onBoost", map[string]interface{}{"msgs": boostOff(false), "state": boostInfo()})
			}()
			return "ok:Boost automatico spento: rimetto tutto com'era."
		}
		bMu.Lock()
		s := loadBoost()
		s.Auto = false
		prevAG := s.PrevAG
		s.PrevAG = nil
		saveBoost()
		bMu.Unlock()
		if prevAG != nil && !*prevAG {
			// La modalità gioco automatica l'aveva accesa il Boost automatico: la tengo solo se serve ad altro.
			if autoGameNeeded() {
				updateSettings(func(st *Settings) { st.AutoGameTune = true })
			} else {
				setAutoGame(false)
			}
		}
		return "ok:Boost automatico spento. Il Boost acceso a mano resta acceso."
	}
	setMu.Lock()
	ag := settings.AutoGame
	setMu.Unlock()
	bMu.Lock()
	s := loadBoost()
	if !s.Auto {
		s.PrevAG = &ag
	}
	s.Auto = true
	saveBoost()
	bMu.Unlock()
	if !ag {
		setAutoGame(true) // sceglie la modalità giusta per ogni gioco
	}
	go func() {
		defer guard("boost.go:autoon")
		bOpMu.Lock()
		boostCmdsOn() // una volta sola, restano accese: valgono dal prossimo avvio del gioco
		bOpMu.Unlock()
	}()
	startBoostWatcher()
	return "ok:Boost automatico acceso: quando apri un gioco Tasso prepara il PC da solo e misura gli FPS."
}

// boostAutoMigra (3.1, scelta di Sergio): il "Boost automatico" non si vede più nella finestra. Chi
// l'aveva acceso lo trova spento, senza avvisi, così non resta attivo senza che si veda. Il codice
// dell'automatico resta (setBoostAuto), ma nessun pulsante lo accende più. Le regolazioni per
// giocare già accese restano: le rimette "Rimetti com'era", che si vede finché c'è da rimettere.
func boostAutoMigra() {
	bMu.Lock()
	s := loadBoost()
	if !s.Auto {
		bMu.Unlock()
		return
	}
	s.Auto = false
	prevAG := s.PrevAG
	s.PrevAG = nil
	saveBoost()
	bMu.Unlock()
	if prevAG != nil && !*prevAG {
		// La modalità gioco automatica l'aveva accesa il Boost automatico: resta solo se serve ad altro
		// (potenza per gioco, Regola con Tasso, pannellino in gioco), e solo per quello.
		if autoGameNeeded() {
			updateSettings(func(st *Settings) { st.AutoGameTune = true })
		} else {
			setAutoGame(false)
		}
	}
}

// startBoostWatcher: guarda ogni 10 secondi se c'è un gioco aperto.
func startBoostWatcher() {
	bWatch.Do(func() {
		go func() {
			defer guard("boost.go:watcher")
			var missing int
			var measured string
			var since time.Time
			var ultimoGioco time.Time // Boost a mano: l'ultima volta che ho visto un gioco aperto
			var manGame string        // Boost a mano: gioco aperto adesso
			t := time.NewTicker(10 * time.Second)
			defer t.Stop()
			for range t.C {
				bMu.Lock()
				st := loadBoost()
				auto, game, skip, manual := st.Auto, bGame, bSkip, st.On
				if manual && bOnAt.IsZero() { // acceso prima che Tasso ripartisse: conto da adesso
					bOnAt = time.Now()
				}
				onAt := bOnAt
				bMu.Unlock()
				setMu.Lock()
				paused := settings.Paused || settings.Profile == "ufficio" // in Ufficio niente Boost automatico
				setMu.Unlock()
				cur := "" // gioco aperto adesso (lo cerco solo se serve)
				if manual || (auto && !paused) || mouseAccelActive() {
					cur = nomeGiocoSicuro(runningGame())
				}
				name := ""
				if auto && !paused {
					name = cur
				}
				// Boost acceso a mano: vale per qualunque gioco apri. Quando ne apri uno chiudo di nuovo i
				// programmi in background (se nel frattempo sono ripartiti), tolgo l'accelerazione del
				// mouse agli sparatutto e misuro gli FPS una volta, dopo 2 minuti.
				if manual && game == "" {
					if cur != "" {
						ultimoGioco = time.Now()
						if !strings.EqualFold(cur, manGame) {
							manGame, measured, since = cur, "", time.Now()
							var msgs []string
							if bOpMu.TryLock() { // se sto accendendo o spegnendo il Boost, salto
								// Ricontrollo dentro il lucchetto: "Rimetti com'era" può essere finito un attimo fa,
								// e con Tasso in pausa il Boost non chiude niente.
								bMu.Lock()
								ancora := loadBoost().On
								bMu.Unlock()
								setMu.Lock()
								inPausa := settings.Paused
								setMu.Unlock()
								if ancora && !inPausa {
									if closed := closeBackground(); len(closed) > 0 {
										msgs = append(msgs, "Chiusi in background: "+strings.Join(closed, ", ")+". Li riapro quando premi \"Rimetti com'era\".")
									}
									if m := mouseAccelOff(cur); m != "" {
										msgs = append(msgs, m)
									}
								}
								bOpMu.Unlock()
							}
							if len(msgs) > 0 {
								emit("onBoost", map[string]interface{}{"msgs": msgs, "state": boostInfo(), "auto": true})
							}
						}
						if measured != cur && time.Since(since) > 2*time.Minute {
							measured = cur
							go func(g string) {
								defer guard("boost.go:manmeasure")
								if r, err := fpsMeasure(0, 60, true, false, g); err == nil {
									emit("onFpsAuto", map[string]interface{}{"game": g, "result": r})
									fpsAutoFix(false)
								}
							}(cur)
						}
					} else {
						manGame = ""
					}
					// Spegnimento automatico: nessun gioco aperto da boostSpegniDopo, e niente pulizia o
					// aggiornamento in corso. Fa "Rimetti com'era" (anche programmi riaperti e mouse).
					// Un gioco che Tasso non riconosce (cartella strana, emulatore, Minecraft) di solito è a
					// schermo intero: finché c'è un programma a schermo intero davanti il Boost resta acceso.
					aperto := cur != ""
					if !aperto && primoPianoSchermoIntero() {
						aperto, ultimoGioco = true, time.Now()
					}
					if boostDaSpegnere(manual, aperto, isRunning() || updating.Load(), onAt, ultimoGioco, time.Now()) {
						if async("boostoff", func() {
							bMu.Lock()
							still := loadBoost().On && bOnAt.Equal(onAt) // nel frattempo non l'hai spento o riacceso tu
							bMu.Unlock()
							if !still {
								return
							}
							msgs := []string{boostSpentoTesto()}
							failed := ""
							for _, m := range boostOff(true) {
								if m == "Tutto com'era prima del Boost." {
									continue // lo dice già l'avviso
								}
								if strings.HasPrefix(m, "Qualcosa non si è potuto rimettere") {
									failed = m
								}
								msgs = append(msgs, m)
							}
							emit("onBoost", map[string]interface{}{"msgs": msgs, "state": boostInfo(), "auto": true, "autoOff": true})
							if failed != "" { // finestra di Tasso (se aperta) + avviso vicino all'orologio
								trayAvvisoMsgs("Boost", []string{boostSpentoTesto(), failed})
							} else {
								trayAvviso("Boost", boostSpentoTesto(), false)
							}
						}) == "ok" {
							ultimoGioco, manGame = time.Time{}, ""
						}
					}
				} else if !manual {
					ultimoGioco, manGame = time.Time{}, ""
				}
				if name == "" && skip != "" {
					bMu.Lock()
					bSkip = ""
					bMu.Unlock()
				}
				// Boost a mano (senza automatico): il mouse torna com'era appena chiudi lo sparatutto.
				if game == "" && mouseAccelActive() && !competitiveGame(cur) {
					if m := mouseAccelRestore(); m != "" {
						emit("onBoost", map[string]interface{}{"msgs": []string{m}, "state": boostInfo(), "auto": true})
					}
				}
				switch {
				case name != "" && game == "" && !strings.EqualFold(name, skip):
					missing, measured, since = 0, "", time.Now()
					bOpMu.Lock()
					bMu.Lock()
					bGame = name
					bMu.Unlock()
					closed := closeBackground()
					bOpMu.Unlock()
					msgs := []string{"Boost automatico per " + name + "."}
					if len(closed) > 0 {
						msgs = append(msgs, "Chiusi in background: "+strings.Join(closed, ", ")+". Li riapro quando chiudi il gioco.")
					}
					if m := mouseAccelOff(name); m != "" {
						msgs = append(msgs, m)
					}
					emit("onBoost", map[string]interface{}{"msgs": msgs, "state": boostInfo(), "auto": true})
				case name != "" && game != "":
					missing = 0
					if !strings.EqualFold(name, game) {
						bMu.Lock()
						if bGame != "" {
							bGame = name
						}
						bMu.Unlock()
						measured, since = "", time.Now()
						// Cambio gioco: mouse senza accelerazione solo se anche il nuovo è competitivo.
						m := ""
						if competitiveGame(name) {
							m = mouseAccelOff(name)
						} else {
							m = mouseAccelRestore()
						}
						if m != "" {
							emit("onBoost", map[string]interface{}{"msgs": []string{m}, "state": boostInfo(), "auto": true})
						}
					}
					// Una misura per partita, dopo 2 minuti (così salto il caricamento), senza bip.
					if measured != name && time.Since(since) > 2*time.Minute {
						measured = name
						go func(g string) {
							defer guard("boost.go:automeasure")
							if r, err := fpsMeasure(0, 60, true, false, g); err == nil {
								emit("onFpsAuto", map[string]interface{}{"game": g, "result": r})
								fpsAutoFix(false)
							}
						}(name)
					}
				case name == "" && game != "":
					missing++
					if missing >= 6 || !auto || paused { // un minuto senza giochi aperti
						missing = 0
						bOpMu.Lock()
						bMu.Lock()
						bGame = ""
						bMu.Unlock()
						names := reopenBackground()
						bOpMu.Unlock()
						msgs := []string{"Gioco chiuso."}
						if len(names) > 0 {
							msgs = append(msgs, "Riaperti: "+strings.Join(names, ", ")+".")
						}
						if m := mouseAccelRestore(); m != "" {
							msgs = append(msgs, m)
						}
						emit("onBoost", map[string]interface{}{"msgs": msgs, "state": boostInfo(), "auto": true})
					}
				}
			}
		}()
	})
}

// ---------------- Misura FPS (PresentMon) ----------------

var notFpsApp = map[string]bool{
	"dwm.exe": true, "tasso.exe": true, "msedgewebview2.exe": true, "explorer.exe": true,
	"msedge.exe": true, "chrome.exe": true, "firefox.exe": true, "opera.exe": true, "brave.exe": true,
	"discord.exe": true, "steamwebhelper.exe": true, "obs64.exe": true, "nvidia overlay.exe": true,
	"textinputhost.exe": true, "searchhost.exe": true, "startmenuexperiencehost.exe": true,
	"shellexperiencehost.exe": true, "applicationframehost.exe": true, "claude.exe": true,
}

// presentMonDir: Programmi\Tasso\strumenti. Il percorso lo chiedo a Windows, non alle variabili
// d'ambiente (che un utente normale può cambiare).
func presentMonDir() (string, error) {
	pf, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil || pf == "" {
		return "", errors.New("cartella Programmi non trovata")
	}
	return filepath.Join(cartellaTasso(), "strumenti"), nil // ecosistema.go: di base Programmi\Tasso
}

// lockedPresentMon apre PresentMon bloccando scrittura, rinomina e cancellazione, controlla che sia
// identico all'originale e lascia il file bloccato finché il chiamante non chiude la maniglia
// (cioè finché PresentMon non è partito): nessuno può sostituirlo in mezzo.
func lockedPresentMon() (string, windows.Handle, error) {
	dir, err := presentMonDir()
	if err != nil {
		return "", 0, err
	}
	p := filepath.Join(dir, "PresentMon.exe")
	open := func() (windows.Handle, error) {
		u, _ := windows.UTF16PtrFromString(p)
		return windows.CreateFile(u, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	}
	check := func(h windows.Handle) bool {
		hs := sha256.New()
		buf := make([]byte, 64*1024)
		for {
			var n uint32
			if err := windows.ReadFile(h, buf, &n, nil); err != nil {
				return false
			}
			if n == 0 {
				break
			}
			hs.Write(buf[:n])
		}
		return hex.EncodeToString(hs.Sum(nil)) == presentMonSHA
	}
	if h, err := open(); err == nil {
		if check(h) {
			return p, h, nil
		}
		windows.CloseHandle(h)
	}
	sum := sha256.Sum256(presentMonExe)
	if hex.EncodeToString(sum[:]) != presentMonSHA {
		return "", 0, errors.New("il misuratore di FPS dentro Tasso è danneggiato")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, err
	}
	tmp := p + ".nuovo"
	_ = os.Remove(tmp)
	if err := os.WriteFile(tmp, presentMonExe, 0o755); err != nil {
		return "", 0, err
	}
	_ = os.Remove(p)
	if err := os.Rename(tmp, p); err != nil {
		return "", 0, err
	}
	h, err := open()
	if err != nil {
		return "", 0, err
	}
	if !check(h) {
		windows.CloseHandle(h)
		return "", 0, errors.New("non riesco a preparare il misuratore di FPS")
	}
	return p, h, nil
}

func beep(n int) {
	for i := 0; i < n; i++ {
		pMessageBeep.Call(0xFFFFFFFF)
		time.Sleep(250 * time.Millisecond)
	}
}

// chiudiSessioneFPS ferma la sessione ETW lasciata aperta da un PresentMon fermato a forza.
func chiudiSessioneFPS() {
	pm, lock, err := lockedPresentMon() // di nuovo controllato (impronta) e tenuto bloccato fino all'avvio
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, pm, "--terminate_existing_session", "--session_name", "TassoFPS")
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	c.Dir = filepath.Dir(pm)
	err = c.Start()
	windows.CloseHandle(lock)
	if err == nil {
		_ = c.Wait()
	}
}

func foregroundPid() uint32 {
	h, _, _ := pForegroundWin.Call()
	if h == 0 {
		return 0
	}
	var pid uint32
	pGetWindowPid.Call(h, uintptr(unsafe.Pointer(&pid)))
	return pid
}

// fpsMeasure misura per secs secondi dopo delay secondi, leggendo i dati da PresentMon senza file
// intermedi. prefer = nome del gioco riconosciuto (se c'è), altrimenti vince la finestra con più
// fotogrammi. Il gioco deve restare davanti: se non lo era, la misura non vale.
func fpsMeasure(delay, secs int, boosted, beeps bool, prefer string) (*fpsResult, error) {
	if !fpsMu.TryLock() {
		return nil, errors.New("c'è già una misura in corso")
	}
	defer fpsMu.Unlock()
	pm, lock, err := lockedPresentMon()
	if err != nil {
		return nil, errors.New("Misura FPS non disponibile: " + err.Error() + ".")
	}
	args := []string{"--output_stdout", "--timed", strconv.Itoa(secs), "--terminate_after_timed",
		"--session_name", "TassoFPS", "--stop_existing_session", "--no_console_stats", "--no_track_input"}
	if delay > 0 {
		args = append(args, "--delay", strconv.Itoa(delay))
	}
	for e := range notFpsApp {
		args = append(args, "--exclude", e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(delay+secs+30)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, pm, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	cmd.Dir = filepath.Dir(pm)
	out, err := cmd.StdoutPipe()
	if err != nil {
		windows.CloseHandle(lock)
		return nil, errors.New("non riesco ad avviare la misura degli FPS")
	}
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	startErr := cmd.Start()
	windows.CloseHandle(lock)
	if startErr != nil {
		return nil, errors.New("non riesco ad avviare la misura degli FPS")
	}
	done := make(chan struct{})
	// chi è davanti, controllato ogni secondo durante la misura (prima solo due volte: bastava un
	// attimo sbagliato per buttare una misura buona, visto con PC Cafe Simulator il 24/11)
	fg := map[uint32]int{}
	fgTot := 0
	var fgMu sync.Mutex
	go func() {
		defer guard("boost.go:fg")
		select {
		case <-done:
			return
		case <-time.After(time.Duration(delay)*time.Second + 2*time.Second):
		}
		if beeps {
			beep(1) // inizia
		}
		for {
			fgMu.Lock()
			fg[foregroundPid()]++
			fgTot++
			fgMu.Unlock()
			select {
			case <-done:
				return
			case <-time.After(time.Second):
			}
		}
	}()
	res, perr := parsePresentMon(bufio.NewReader(out), prefer)
	_, _ = io.Copy(io.Discard, out)
	werr := cmd.Wait()
	close(done)
	if ctx.Err() != nil {
		// PresentMon fermato per tempo scaduto: la sessione ETW "TassoFPS" sopravvive al processo.
		chiudiSessioneFPS()
	}
	fgMu.Lock()
	durante := fgTot      // controlli fatti mentre misuravo
	fg[foregroundPid()]++ // ultimo controllo a misura finita: solo un complemento
	fgTot++
	fgMu.Unlock()
	hzAtEnd := gameScreenHz()
	if beeps {
		beep(2) // finita
	}
	if werr != nil && perr != nil {
		var ee *exec.ExitError
		if (errors.As(werr, &ee) && ee.ExitCode() == 6) || strings.Contains(strings.ToLower(errBuf.String()), "access") {
			return nil, errors.New("Windows non mi lascia misurare gli FPS: serve aprire Tasso come amministratore")
		}
		return nil, errors.New("la misura degli FPS si è fermata: riprova")
	}
	if perr != nil {
		return nil, perr
	}
	pidN, _ := strconv.ParseUint(res.pid, 10, 32)
	fgMu.Lock()
	// davanti quasi sempre: il gioco, o un suo processo con lo stesso nome
	// (alcuni giochi hanno un lanciatore e il gioco vero con due numeri diversi)
	davanti := 0
	for pid, n := range fg {
		if (pidN != 0 && pid == uint32(pidN)) || (pid != 0 && strings.EqualFold(safeName(strings.TrimSuffix(filepath.Base(processPath(pid)), ".exe")), res.Game)) {
			davanti += n
		}
	}
	// fuori al massimo 2 controlli (o un decimo nelle misure lunghe): con un terzo accettato un
	// alt-tab di 9 secondi su 30 passava, e dietro molti giochi rallentano (misura sbagliata)
	tolti := max(2, fgTot/10)
	if fgTot < 10 {
		tolti = 1 // misura fermata presto: con 4 controlli, 2 fuori sarebbe metà misura
	}
	front := fgTot > 0 && fgTot-davanti <= tolti
	fgMu.Unlock()
	if durante < 3 {
		// troppo pochi controlli per sapere chi era davanti: la misura non vale
		return nil, errors.New("la misura è stata troppo breve per controllare che il gioco fosse davanti: riprova")
	}
	if !front {
		return nil, errors.New("il gioco non era in primo piano durante la misura: tienilo davanti e riprova")
	}
	res.Boost, res.When = boosted, time.Now().Format(time.RFC3339)
	// Il programma con quel numero deve essere ancora il gioco (i numeri dei processi si riusano).
	if pp := processPath(uint32(pidN)); pp != "" && strings.EqualFold(safeName(strings.TrimSuffix(filepath.Base(pp), filepath.Ext(pp))), res.Game) {
		res.path = pp
	}
	res.Hz = hzAtEnd
	// Temperature e memoria video: le leggo subito DOPO la fine della registrazione (il gioco è
	// ancora aperto e caldo). Prima partivano 8 secondi prima della fine: PowerShell e nvidia-smi
	// rubavano tempo al processore e sporcavano l'1% low.
	t := readTemps()
	res.CpuTemp, res.GpuTemp, res.Throttle, res.VramPct = t.cpu, t.gpu, t.throttle, t.vram
	setLastFps(res)
	bMu.Lock()
	s := loadBoost()
	if k := strings.ToLower(res.Game); k != "" {
		s.History[k] = append(s.History[k], res)
		if n := len(s.History[k]); n > 10 {
			s.History[k] = s.History[k][n-10:]
		}
		saveBoost()
	}
	bMu.Unlock()
	return res, nil
}

// parsePresentMon legge il CSV di PresentMon e calcola FPS medi, 1% low e scatti della finestra del
// gioco (prefer) o di quella con più fotogrammi. I tempi tra due fotogrammi valgono per singola
// finestra (swapchain), quindi raggruppo per programma + finestra.
func parsePresentMon(r io.Reader, prefer string) (*fpsResult, error) {
	errNone := errors.New("nessun gioco visto: apri il gioco e tienilo davanti mentre misuro")
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	head, err := cr.Read()
	if err != nil {
		return nil, errNone
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(strings.TrimPrefix(h, "\ufeff"))] = i
	}
	ib, okB := col["MsGPUBusy"]
	if !okB {
		ib, okB = col["GPUBusy"]
	}
	ia, ok1 := col["Application"]
	ip, ok2 := col["ProcessID"]
	is, okS := col["SwapChainAddress"]
	// Colonna del tempo tra due fotogrammi: versione attuale, 1.x e 2.x di PresentMon.
	it, ok3 := col["MsBetweenPresents"]
	for _, alt := range []string{"msBetweenPresents", "FrameTime"} {
		if !ok3 {
			it, ok3 = col[alt]
		}
	}
	if !ok1 || !ok2 || !ok3 {
		return nil, errors.New("il misuratore di FPS ha dato un risultato che non conosco")
	}
	type grp struct {
		app, pid string
		ft       []float64
		busy     float64
		busyFt   float64
		nBusy    int
	}
	groups := map[string]*grp{}
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(rec) <= it || len(rec) <= ia || len(rec) <= ip {
			continue
		}
		app := rec[ia]
		if notFpsApp[strings.ToLower(app)] {
			continue
		}
		v, err := strconv.ParseFloat(rec[it], 64)
		// 0 = primo fotogramma; oltre 2 secondi = pausa, menu o cambio finestra, non uno scatto.
		if err != nil || v <= 0 || v > 2000 {
			continue
		}
		key := rec[ip]
		if okS && len(rec) > is {
			key += "/" + rec[is]
		}
		g := groups[key]
		if g == nil {
			if len(groups) > 500 {
				continue
			}
			g = &grp{app: app, pid: rec[ip]}
			groups[key] = g
		}
		g.ft = append(g.ft, v)
		if okB && len(rec) > ib {
			if b, err := strconv.ParseFloat(rec[ib], 64); err == nil && b >= 0 && b <= 2000 {
				g.busy += b
				g.busyFt += v
				g.nBusy++
			}
		}
	}
	var best *grp
	isPref := func(g *grp) bool {
		return prefer != "" && strings.EqualFold(safeName(strings.TrimSuffix(g.app, ".exe")), prefer)
	}
	for _, g := range groups {
		if len(g.ft) < 100 {
			continue
		}
		if best == nil || (isPref(g) && !isPref(best)) || (isPref(g) == isPref(best) && len(g.ft) > len(best.ft)) {
			best = g
		}
	}
	if best == nil {
		return nil, errNone
	}
	res := fpsStats(safeName(strings.TrimSuffix(best.app, ".exe")), best.ft)
	res.pid = best.pid
	if best.nBusy > len(best.ft)/2 && best.busyFt > 0 {
		res.GpuBusy = math.Round(math.Min(100, best.busy/best.busyFt*100))
	}
	return res, nil
}

func fpsStats(app string, ft []float64) *fpsResult {
	total := 0.0
	for _, v := range ft {
		total += v
	}
	s := append([]float64(nil), ft...)
	sort.Float64s(s)
	median := s[len(s)/2]
	n := len(s) / 100
	if n < 1 {
		n = 1
	}
	slow := 0.0
	for _, v := range s[len(s)-n:] {
		slow += v
	}
	p5, p95 := s[len(s)*5/100], s[len(s)*95/100]
	stutter := 0
	for _, v := range ft {
		if v > 2*median && v > 1000.0/60 { // sopra i 60 FPS un fotogramma "doppio" non si nota
			stutter++
		}
	}
	round := func(x float64) float64 { return math.Round(x*10) / 10 }
	return &fpsResult{
		Game: app, Seconds: round(total / 1000), Frames: len(ft),
		Avg: round(float64(len(ft)) * 1000 / total), Low1: round(1000 / (slow / float64(n))),
		Stutter: stutter, StutterPM: round(float64(stutter) / (total / 60000)),
		Spread: round((p95 - p5) / median * 100),
	}
}

// fpsCompare: confronto onesto tra una misura senza Boost e una con. Una sola misura di gioco
// libero varia facilmente del 5-10%: sotto queste soglie, o se FPS medi e cali vanno in direzioni
// diverse, il risultato è "più o meno uguale". Guadagni e perdite sono trattati allo stesso modo.
func fpsCompare(a, b *fpsResult) map[string]interface{} {
	out := map[string]interface{}{"sameGame": strings.EqualFold(a.Game, b.Game)}
	if a.Avg <= 0 || b.Avg <= 0 || a.Low1 <= 0 {
		return out
	}
	da := (b.Avg - a.Avg) / a.Avg * 100
	dl := (b.Low1 - a.Low1) / a.Low1 * 100
	out["avgPct"], out["lowPct"] = math.Round(da*10)/10, math.Round(dl*10)/10
	var verdict string
	switch {
	case !strings.EqualFold(a.Game, b.Game):
		verdict = "Le due misure sono di giochi diversi: non si possono confrontare."
	case (da >= 5 && dl >= 0) || (dl >= 10 && da >= 0):
		verdict = fmt.Sprintf("Con il Boost va meglio: FPS medi %+.0f%%, cali (1%% low) %+.0f%%. Vale se hai giocato nello stesso posto e con la stessa grafica.", da, dl)
	case (da <= -5 && dl <= 0) || (dl <= -10 && da <= 0):
		verdict = fmt.Sprintf("Con il Boost va peggio: FPS medi %+.0f%%, cali %+.0f%%. Può essere la scena; se succede di nuovo nello stesso posto, premi \"Rimetti com'era\".", da, dl)
	default:
		verdict = "Più o meno uguale: la differenza è dentro la variazione normale tra due momenti di gioco."
	}
	out["verdict"] = verdict
	return out
}

// fpsSummary: per ogni gioco l'ultima misura senza Boost, l'ultima con Boost e il confronto.
func fpsSummary() map[string]interface{} {
	bMu.Lock()
	defer bMu.Unlock()
	out := map[string]interface{}{}
	for k, list := range loadBoost().History {
		var off, on *fpsResult
		for _, r := range list {
			if r.Boost {
				on = r
			} else {
				off = r
			}
		}
		e := map[string]interface{}{"without": off, "with": on, "list": list}
		if off != nil && on != nil {
			e["compare"] = fpsCompare(off, on)
		}
		out[k] = e
	}
	return out
}

// ---------------- Collegamento all'interfaccia ----------------

// bindBoost collega il Boost all'interfaccia (una riga in main.go) e fa ripartire il Boost
// automatico se era acceso.
func bindBoost(w webview2.WebView) {
	_ = w.Bind("goBoostAll", func() string {
		return async("boostall", func() {
			msgs, ok := boostOnOK()
			emit("onBoost", map[string]interface{}{"msgs": msgs, "state": boostInfo()})
			if ok { // niente sistemazioni se il Boost ha rifiutato (Tasso spento o già acceso)
				fpsAutoFix(true)
			}
		})
	})
	_ = w.Bind("goBoostAllOff", func() string {
		return async("boostoff", func() { emit("onBoost", map[string]interface{}{"msgs": boostOff(true), "state": boostInfo()}) })
	})
	_ = w.Bind("goBoostInfo", func() map[string]interface{} { return boostInfo() })
	_ = w.Bind("goBoostAuto", func(on bool) string { return setBoostAuto(on) })
	_ = w.Bind("goFpsMeasure", func() string {
		return async("fps", func() {
			bMu.Lock()
			s := loadBoost()
			boosted := s.On || s.Auto
			bMu.Unlock()
			r, err := fpsMeasure(10, 30, boosted, true, safeName(runningGame()))
			if err != nil {
				emit("onFps", map[string]interface{}{"ok": false, "msg": err.Error()})
				return
			}
			emit("onFps", map[string]interface{}{"ok": true, "result": r, "summary": fpsSummary()})
		})
	})
	_ = w.Bind("goFpsSummary", func() map[string]interface{} { return fpsSummary() })
	_ = w.Bind("goFpsCheck", func() string {
		return async("fpscheck", func() { emit("onFpsCheck", fpsCheck()) })
	})
	_ = w.Bind("goFpsFixHz", func() string { return async("fpshz", screenHzMax) })
	_ = w.Bind("goFpsKeepHz", func() string { go screenHzKeep(); return "ok" })
	_ = w.Bind("goFpsUndoHz", func() string { go screenHzUndo(false); return "ok" })
	_ = w.Bind("goFpsFixGpu", func() string { return fpsSetGpu() })
	_ = w.Bind("goFpsDriver", func(id string) string {
		if id != "nvidia" && id != "amd" && id != "intel" {
			return "no"
		}
		return openVendorPage(id)
	})
	boostAutoMigra() // il Boost automatico non c'è più nella finestra: se era acceso lo spengo, in silenzio
	bMu.Lock()
	st := loadBoost()
	auto, manual := st.Auto, st.On
	daRiaprire := !manual && bGame == "" && len(bClosed) > 0
	bMu.Unlock()
	// Tasso si era chiuso (crash) con il Boost già spento ma programmi ancora chiusi: li riapro,
	// se non c'è un gioco aperto (se c'è, lo farà "Rimetti com'era").
	// Un gioco che Tasso non riconosce di solito è a schermo intero: anche lì non riapro niente
	// (Teams o OneDrive aprirebbero una finestra sopra il gioco).
	if riapriAllAvvio(daRiaprire, runningGame() != "", primoPianoSchermoIntero()) {
		go func() {
			defer guard("boost.go:riapri-avvio")
			reopenBackground()
		}()
	}
	// Il Tasso si era chiuso con il mouse ancora senza accelerazione: lo rimetto com'era.
	if !competitiveGame(safeName(runningGame())) {
		mouseAccelRestore()
	} else {
		startBoostWatcher()
	}
	if auto || manual { // Boost acceso a mano: serve anche per lo spegnimento automatico
		startBoostWatcher()
	}
}
