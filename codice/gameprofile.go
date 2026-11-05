package main

// Profilo gioco: scegli una volta le impostazioni che ti servono per giocare e le accendi o spegni
// con un solo interruttore. Finché è acceso resta acceso anche dopo aver spento o riavviato il PC:
// a ogni accensione Tasso controlla che sia tutto come l'hai scelto e rimette quello che Windows
// (o un programma del produttore del PC) ha cambiato. Si spegne solo quando lo spegni tu.

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

const taskProfilo = "Tasso\\Profilo gioco"

type gameProfile struct {
	On       bool     `json:"on"`
	Mode     string   `json:"mode"` // "gaming" o "fps"
	Cmds     []string `json:"cmds"`
	Overlay  bool     `json:"overlay"`
	AutoGame bool     `json:"autoGame"`
	Standby  bool     `json:"standby"`
	// Cosa c'era prima di accenderlo, per rimetterlo quando lo spegni.
	Added       []string `json:"added,omitempty"`
	PrevMode    string   `json:"prevMode,omitempty"`
	PrevOverlay bool     `json:"prevOverlay,omitempty"`
	PrevAuto    bool     `json:"prevAuto,omitempty"`
	Since       string   `json:"since,omitempty"`
	LastCheck   string   `json:"lastCheck,omitempty"`
	LastFix     []string `json:"lastFix,omitempty"`
}

func defaultProfile() *gameProfile {
	return &gameProfile{Mode: "gaming", Cmds: []string{"gamedvr"}, AutoGame: true}
}

// profileCopy: una copia del profilo salvato (o quello di partenza).
func profileCopy() *gameProfile {
	setMu.Lock()
	defer setMu.Unlock()
	if settings.GameProfile == nil {
		return defaultProfile()
	}
	p := *settings.GameProfile
	p.Cmds = append([]string(nil), p.Cmds...)
	p.Added = append([]string(nil), p.Added...)
	p.LastFix = append([]string(nil), p.LastFix...)
	return &p
}

func saveProfile(p *gameProfile) {
	updateSettings(func(s *Settings) { c := *p; s.GameProfile = &c })
}

func cleanCmds(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, id := range in {
		if _, ok := gamingTitles[id]; ok && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func hasStr(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func profileInfo() map[string]interface{} {
	p := profileCopy()
	setMu.Lock()
	auto, paused := settings.Autostart, settings.Paused
	setMu.Unlock()
	return map[string]interface{}{"profile": p, "autostart": auto, "paused": paused}
}

// profileTask: se Tasso non parte già con Windows, un'attività leggera all'accesso rimette il profilo
// e si chiude subito, senza aprire la finestra.
func profileTask(on bool) {
	setMu.Lock()
	auto, paused := settings.Autostart, settings.Paused
	setMu.Unlock()
	if on && !auto && !paused {
		// Modalità gioco automatica e pannellino funzionano solo con Tasso aperto: in quel caso
		// all'accesso parte Tasso ridotto a icona; altrimenti basta la copia veloce che si chiude subito.
		arg := "--profile"
		if p := profileCopy(); p.AutoGame || p.Overlay {
			arg = "--min"
		}
		if !createLogonTask("Profilo gioco", arg) && taskExe() == "" {
			// Non lascio un'attività vecchia che punta a un Tasso in una cartella non sicura
			// (se invece è solo un errore passeggero, quella che c'è resta).
			_, _ = runHidden("schtasks.exe", "/Delete", "/TN", taskProfilo, "/F")
		}
		return
	}
	_, _ = runHidden("schtasks.exe", "/Delete", "/TN", taskProfilo, "/F")
}

func highPlanActive() bool {
	out, _ := runHidden("powercfg.exe", "/getactivescheme")
	l := strings.ToLower(out)
	return strings.Contains(l, "8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c") || strings.Contains(l, "prestazioni elevate") ||
		strings.Contains(l, "high performance") || strings.Contains(l, "ultimate") || strings.Contains(l, "prestazioni ottimali") ||
		!planExists("8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c") // PC con solo "Bilanciato": Tasso usa la modalità "Prestazioni migliori"
}

// planExists: il piano energetico c'è su questo PC? (molti portatili nuovi hanno solo Bilanciato)
func planExists(guid string) bool {
	out, _ := runHidden("powercfg.exe", "/list")
	l := strings.ToLower(out)
	if strings.Contains(l, guid) {
		return true
	}
	// Tasso può averne creato una copia con un altro codice ma con lo stesso nome.
	return strings.Contains(l, "prestazioni elevate") || strings.Contains(l, "high performance")
}

func profileOn(p *gameProfile) []string {
	var msgs []string
	setMu.Lock()
	cur, ov, ag := settings.Mode, settings.OverlayOn, settings.AutoGame
	setMu.Unlock()
	if !p.On {
		p.PrevMode = cur
		if _, ok := modeScripts[cur]; !ok {
			p.PrevMode = "normale"
		}
		p.PrevOverlay, p.PrevAuto = ov, ag
		p.Added = nil
	}
	st := gamingState()
	for _, id := range p.Cmds {
		if on, _ := st[id].(bool); on {
			continue
		}
		if r := gamingSet(id, true); strings.HasPrefix(r, "ok:") {
			if !hasStr(p.Added, id) {
				p.Added = append(p.Added, id)
			}
		} else {
			msgs = append(msgs, gamingTitles[id]+": "+r)
		}
	}
	setMode(p.Mode)
	if p.AutoGame && !ag {
		setAutoGame(true)
	}
	if p.Overlay && !ov {
		setOverlay(true, overlayPos())
	}
	p.On = true
	p.Since = time.Now().Format("02/01/2006 15:04")
	saveProfile(p)
	profileTask(true)
	return append([]string{"Profilo gioco acceso. Resta acceso anche quando spegni o riavvii il PC, finché non lo spegni tu."}, msgs...)
}

func profileOff(p *gameProfile) []string {
	for _, id := range p.Added {
		gamingSet(id, false)
	}
	setMu.Lock()
	cur := settings.Mode
	setMu.Unlock()
	if cur == p.Mode {
		back := p.PrevMode
		if _, ok := modeScripts[back]; !ok {
			back = "normale"
		}
		if back != p.Mode {
			setMode(back)
		}
	}
	if p.Overlay && !p.PrevOverlay {
		setOverlay(false, overlayPos())
	}
	if p.AutoGame && !p.PrevAuto && !autoGameNeeded() {
		setAutoGame(false)
	}
	p.On, p.Added, p.Since, p.LastFix = false, nil, "", nil
	saveProfile(p)
	profileTask(false)
	return []string{"Profilo gioco spento: ho rimesso le cose com'erano prima di accenderlo."}
}

// profileSet accende o spegne il profilo, con le scelte che arrivano dall'interfaccia.
var profileMu sync.Mutex

func profileSet(on bool, mode string, cmds []string, overlay, autoGame, standby bool) {
	profileMu.Lock()
	defer profileMu.Unlock()
	p := profileCopy()
	if mode != "fps" {
		mode = "gaming"
	}
	newCmds := cleanCmds(cmds)
	var msgs []string
	if on && p.On {
		// Profilo già acceso e scelte cambiate: tolgo solo i comandi che non vuoi più.
		var keep []string
		for _, id := range p.Added {
			if hasStr(newCmds, id) {
				keep = append(keep, id)
			} else {
				gamingSet(id, false)
			}
		}
		p.Added = keep
		if p.Overlay && !overlay && !p.PrevOverlay {
			setOverlay(false, overlayPos())
		}
		if p.AutoGame && !autoGame && !p.PrevAuto && !autoGameNeeded() {
			setAutoGame(false)
		}
	}
	p.Mode, p.Cmds, p.Overlay, p.AutoGame, p.Standby = mode, newCmds, overlay, autoGame, standby
	if p.Overlay {
		p.AutoGame = true // il pannellino appare da solo solo con la modalità gioco automatica
	}
	if on {
		msgs = profileOn(p)
	} else if p.On {
		msgs = profileOff(p)
	} else {
		saveProfile(p)
		msgs = []string{"Profilo salvato. Accendilo quando vuoi giocare."}
	}
	info := profileInfo()
	info["msgs"] = msgs
	info["settings"] = getSettings()
	emit("onProfile", info)
}

// profileCheck: all'accensione del PC rimette quello che è cambiato. Restituisce cosa ha sistemato.
func profileCheck() []string {
	profileMu.Lock()
	defer profileMu.Unlock()
	p := profileCopy()
	setMu.Lock()
	on, paused, cur := p.On, settings.Paused, settings.Mode
	setMu.Unlock()
	if !on || paused {
		return nil
	}
	var fixed []string
	if cur != p.Mode || !highPlanActive() {
		fixed = append(fixed, "Il piano energetico non era più al massimo (di solito lo cambia un aggiornamento di Windows o il programma del produttore del PC): rimesso.")
	}
	setMode(p.Mode) // la rimetto sempre: è veloce e non cambia niente se era già giusta
	st := gamingState()
	for _, id := range p.Cmds {
		if v, _ := st[id].(bool); !v {
			if r := gamingSet(id, true); strings.HasPrefix(r, "ok:") {
				fixed = append(fixed, gamingTitles[id]+": era stata spenta, riaccesa.")
				if !hasStr(p.Added, id) {
					p.Added = append(p.Added, id)
				}
			}
		}
	}
	setMu.Lock()
	ov, ag := settings.OverlayOn, settings.AutoGame
	setMu.Unlock()
	if p.AutoGame && !ag {
		setAutoGame(true)
		fixed = append(fixed, "Modalità gioco automatica riaccesa.")
	}
	if p.Overlay && !ov {
		setOverlay(true, overlayPos())
		fixed = append(fixed, "Pannellino in gioco riacceso.")
	}
	p.LastCheck = time.Now().Format("02/01/2006 15:04")
	p.LastFix = fixed
	// Rileggo solo i campi miei: nel frattempo setMode e gamingSet hanno salvato altre cose.
	updateSettings(func(s *Settings) {
		if s.GameProfile != nil && s.GameProfile.On {
			s.GameProfile.Added, s.GameProfile.LastCheck, s.GameProfile.LastFix = p.Added, p.LastCheck, p.LastFix
		}
	})
	return fixed
}

// profileForget: quando rimetti tutto com'era, il profilo si spegne (senza rimettere niente: ci pensa il ripristino).
// profileOffNow: spegne il profilo (da "Torna indietro"), solo se è acceso.
func profileOffNow() string {
	profileMu.Lock()
	defer profileMu.Unlock()
	p := profileCopy()
	if !p.On {
		return "Il profilo gioco era già spento."
	}
	return strings.Join(profileOff(p), " ")
}

func profileForget() {
	setMu.Lock()
	on := settings.GameProfile != nil && settings.GameProfile.On
	setMu.Unlock()
	if !on {
		return
	}
	updateSettings(func(s *Settings) {
		s.GameProfile.On, s.GameProfile.Added, s.GameProfile.Since, s.GameProfile.LastFix = false, nil, "", nil
	})
	profileTask(false)
}

// profileDropCmd: se rimetti com'era un comando gaming da "Torna indietro", il profilo non lo riaccende più.
func profileDropCmd(id string) {
	updateSettings(func(s *Settings) {
		if s.GameProfile == nil {
			return
		}
		var c, a []string
		for _, x := range s.GameProfile.Cmds {
			if x != id {
				c = append(c, x)
			}
		}
		for _, x := range s.GameProfile.Added {
			if x != id {
				a = append(a, x)
			}
		}
		s.GameProfile.Cmds, s.GameProfile.Added = c, a
	})
}

func overlayPos() string {
	setMu.Lock()
	defer setMu.Unlock()
	return settings.OverlayPos
}

func profileIsOn() bool {
	setMu.Lock()
	defer setMu.Unlock()
	return settings.GameProfile != nil && settings.GameProfile.On
}

type profileReq struct {
	On       bool     `json:"on"`
	Mode     string   `json:"mode"`
	Cmds     []string `json:"cmds"`
	Overlay  bool     `json:"overlay"`
	AutoGame bool     `json:"autoGame"`
	Standby  bool     `json:"standby"`
}

// runProfileOnly: avviato dall'attività "Profilo gioco" all'accesso a Windows.
func runProfileOnly() {
	mname, _ := windows.UTF16PtrFromString("Local\\TassoSingleInstance")
	h, err := windows.CreateMutex(nil, true, mname)
	if err == windows.ERROR_ALREADY_EXISTS {
		if h != 0 {
			windows.CloseHandle(h)
		}
		return // Tasso è già aperto: il controllo lo fa lui
	}
	if h != 0 {
		defer windows.CloseHandle(h) // finché lavoro, un Tasso aperto a mano aspetta che finisca
	}
	exePath, _ = os.Executable()
	appDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "Tasso")
	_ = os.MkdirAll(filepath.Join(appDir, "logs"), 0o755)
	loadSettings()
	time.Sleep(20 * time.Second) // lascio finire l'avvio di Windows e dei programmi del produttore
	profileCheck()
}

// ---------------- Potenza per ogni gioco ----------------
// Scegli un gioco e quanta potenza vuoi: quando lo apri Tasso la mette da sola e quando lo chiudi
// rimette com'era. Livelli: "gaming" (Potenza alta), "max" (Massima potenza: FPS competitivo e
// memoria in standby svuotata all'avvio del gioco). Si basa sulla Modalità gioco automatica.

type gamePower struct {
	Name string   `json:"name"`
	Mode string   `json:"mode"`
	Exes []string `json:"exes"`
}

func gamePowerList() map[string]interface{} {
	setMu.Lock()
	defer setMu.Unlock()
	out := map[string]gamePower{}
	for id, g := range settings.GamePower {
		out[id] = g
	}
	return map[string]interface{}{"items": out, "autoGame": settings.AutoGame}
}

// setGamePower: mode "" toglie la regola per quel gioco.
func setGamePower(id, mode string) map[string]interface{} {
	libMu.Lock()
	g, ok := library[id]
	libMu.Unlock()
	res := map[string]interface{}{"ok": false}
	if mode != "" && mode != "gaming" && mode != "max" {
		mode = "gaming"
	}
	setMu.Lock()
	old, had := settings.GamePower[id]
	setMu.Unlock()
	var exes []string
	name := old.Name
	if ok {
		name = g.Name
		for _, e := range gameExes(g.dir) {
			exes = append(exes, strings.ToLower(strings.TrimSuffix(filepath.Base(e), filepath.Ext(e))))
		}
	} else if had {
		exes = old.Exes
	}
	if mode != "" && len(exes) == 0 {
		res["error"] = "Non trovo il programma di questo gioco nella sua cartella: aggiorna la libreria, oppure apri il gioco una volta e riprova."
		res["list"] = gamePowerList()
		return res
	}
	updateSettings(func(s *Settings) {
		if s.GamePower == nil {
			s.GamePower = map[string]gamePower{}
		}
		if mode == "" {
			delete(s.GamePower, id)
			if s.AutoGameTune && !autoGameKept(s) {
				used := false
				for _, o := range s.GameTunes {
					if o != nil && o.Opts.Mode != "" {
						used = true
					}
				}
				if !used {
					s.AutoGame, s.AutoGameTune = false, false
				}
			}
			return
		}
		s.GamePower[id] = gamePower{Name: name, Mode: mode, Exes: exes}
		if !s.AutoGame {
			s.AutoGame, s.AutoGameTune = true, true
		}
	})
	res["ok"] = true
	res["removed"] = mode == ""
	res["list"] = gamePowerList()
	res["settings"] = getSettings()
	return res
}

// gamePowerFor: la potenza scelta per questo gioco (nome del programma senza .exe), se c'è.
// Va chiamata con setMu preso.
func gamePowerFor(exe string) string {
	for _, g := range settings.GamePower {
		for _, x := range g.Exes {
			if x == exe {
				return g.Mode
			}
		}
	}
	return ""
}

// autoGameNeeded: qualcosa d'altro (potenza per gioco, Regola con Tasso) usa la modalità gioco automatica.
func autoGameNeeded() bool {
	setMu.Lock()
	defer setMu.Unlock()
	if len(settings.GamePower) > 0 {
		return true
	}
	for _, t := range settings.GameTunes {
		if t != nil && t.Opts.Mode != "" {
			return true
		}
	}
	return false
}

// autoGameKept: la Modalità gioco automatica serve ancora (potenza per un gioco o profilo gioco che la usa).
// Va chiamata con le impostazioni già bloccate (dentro updateSettings).
func autoGameKept(s *Settings) bool {
	return len(s.GamePower) > 0 || (s.GameProfile != nil && s.GameProfile.On && s.GameProfile.AutoGame)
}
