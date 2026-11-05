package main

// Comandi gaming: le regolazioni di Windows che contano per giocare, fatte da Tasso in modo
// sicuro e reversibile (spento = valore normale di Windows): priorità ai giochi (MMCSS),
// memoria in standby, rete a bassa latenza (Nagle e limitazione di rete spenti),
// registrazione in background spenta, pianificazione GPU hardware, ottimizzazioni per i
// giochi in finestra, riavvio rapido del driver video, cache degli shader.

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	sysProfKey  = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Multimedia\SystemProfile`
	gamesTask   = sysProfKey + `\Tasks\Games`
	gfxDrvKey   = `SYSTEM\CurrentControlSet\Control\GraphicsDrivers`
	tcpIfaceKey = `SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces`
	dxUserKey   = `Software\Microsoft\DirectX\UserGpuPreferences`
)

func regDword(root registry.Key, path, name string) (uint32, bool) {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return 0, false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(name)
	return uint32(v), err == nil
}

func setDword(root registry.Key, path, name string, v uint32) error {
	k, _, err := registry.CreateKey(root, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetDWordValue(name, v)
}

func winBuild() int {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return 0
	}
	defer k.Close()
	s, _, _ := k.GetStringValue("CurrentBuildNumber")
	n, _ := strconv.Atoi(s)
	return n
}

// dxGlobal legge/scrive le opzioni globali DirectX di Windows ("Chiave=valore;Chiave=valore;").
func dxGlobal() map[string]string {
	m := map[string]string{}
	v, _ := regGet(registry.CURRENT_USER, dxUserKey, "DirectXUserGlobalSettings")
	for _, part := range strings.Split(v, ";") {
		if i := strings.Index(part, "="); i > 0 {
			m[strings.TrimSpace(part[:i])] = strings.TrimSpace(part[i+1:])
		}
	}
	return m
}

func setDxGlobal(m map[string]string) error {
	var b strings.Builder
	for k, v := range m {
		b.WriteString(k + "=" + v + ";")
	}
	if b.Len() == 0 {
		regDel(registry.CURRENT_USER, dxUserKey, "DirectXUserGlobalSettings")
		return nil
	}
	return regSet(registry.CURRENT_USER, dxUserKey, "DirectXUserGlobalSettings", b.String())
}

// regPrev ricorda un valore del registro com'era prima che Tasso lo cambiasse,
// così "spento" e "Rimetti com'era" lo rimettono identico (anche "non c'era").
type regPrev struct {
	HKLM bool   `json:"hklm,omitempty"`
	Path string `json:"path"`
	Name string `json:"name"`
	Kind string `json:"kind"` // "none", "dword", "sz"
	D    uint32 `json:"d,omitempty"`
	S    string `json:"s,omitempty"`
}

func rootOf(hklm bool) registry.Key {
	if hklm {
		return registry.LOCAL_MACHINE
	}
	return registry.CURRENT_USER
}

func snapReg(hklm bool, path, name string) regPrev {
	p := regPrev{HKLM: hklm, Path: path, Name: name, Kind: "none"}
	k, err := registry.OpenKey(rootOf(hklm), path, registry.QUERY_VALUE)
	if err != nil {
		return p
	}
	defer k.Close()
	if v, t, err := k.GetIntegerValue(name); err == nil && t == registry.DWORD {
		p.Kind, p.D = "dword", uint32(v)
	} else if sv, _, err := k.GetStringValue(name); err == nil {
		p.Kind, p.S = "sz", sv
	}
	return p
}

func (p regPrev) restore() {
	switch p.Kind {
	case "dword":
		_ = setDword(rootOf(p.HKLM), p.Path, p.Name, p.D)
	case "sz":
		_ = regSet(rootOf(p.HKLM), p.Path, p.Name, p.S)
	default:
		regDel(rootOf(p.HKLM), p.Path, p.Name)
	}
}

func regPrevKey(p regPrev) string {
	return fmt.Sprintf("%t|%s|%s", p.HKLM, strings.ToLower(p.Path), strings.ToLower(p.Name))
}

// gamingTargets: i valori che ogni comando cambia.
func gamingTargets(id string) []regPrev {
	var t []regPrev
	add := func(hklm bool, path string, names ...string) {
		for _, n := range names {
			t = append(t, snapReg(hklm, path, n))
		}
	}
	switch id {
	case "gamedvr":
		add(false, `System\GameConfigStore`, "GameDVR_Enabled")
		add(false, `Software\Microsoft\Windows\CurrentVersion\GameDVR`, "AppCaptureEnabled")
	case "mmcss":
		add(true, sysProfKey, "SystemResponsiveness")
		add(true, gamesTask, "GPU Priority", "Priority", "Scheduling Category", "SFIO Priority")
	case "nagle":
		add(true, sysProfKey, "NetworkThrottlingIndex")
		for _, p := range nagleIfaces() {
			add(true, p, "TcpAckFrequency", "TCPNoDelay")
		}
	case "hags":
		add(true, gfxDrvKey, "HwSchMode")
	case "winopt":
		add(false, dxUserKey, "DirectXUserGlobalSettings")
	}
	return t
}

// gamingRestore rimette i valori salvati prima dell'accensione; false se non c'era niente di salvato.
func gamingRestore(id string) bool {
	setMu.Lock()
	prev, ok := settings.GamingPrev[id]
	setMu.Unlock()
	if !ok {
		return false
	}
	// settings.json si può modificare senza essere amministratore: rimetto solo i valori che
	// questo comando cambia davvero (stesse chiavi di gamingTargets), mai altre chiavi del registro.
	allowed := map[string]bool{}
	for _, t := range gamingTargets(id) {
		allowed[regPrevKey(t)] = true
	}
	for _, p := range prev {
		if !allowed[regPrevKey(p)] {
			continue
		}
		switch p.Kind {
		case "dword", "sz", "none":
			p.restore()
		}
	}
	updateSettings(func(s *Settings) { delete(s.GamingPrev, id) })
	return true
}

func gamingState() map[string]interface{} {
	st := map[string]interface{}{}
	dvr, ok1 := regDword(registry.CURRENT_USER, `System\GameConfigStore`, "GameDVR_Enabled")
	cap, ok2 := regDword(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\GameDVR`, "AppCaptureEnabled")
	st["gamedvr"] = ok1 && ok2 && dvr == 0 && cap == 0
	sr, _ := regDword(registry.LOCAL_MACHINE, sysProfKey, "SystemResponsiveness")
	pr, _ := regDword(registry.LOCAL_MACHINE, gamesTask, "Priority")
	st["mmcss"] = sr == 10 && pr == 6
	nt, _ := regDword(registry.LOCAL_MACHINE, sysProfKey, "NetworkThrottlingIndex")
	st["nagle"] = nt == 0xffffffff
	hs, _ := regDword(registry.LOCAL_MACHINE, gfxDrvKey, "HwSchMode")
	st["hags"] = hs == 2
	st["winopt"] = dxGlobal()["SwapEffectUpgradeEnable"] == "1"
	st["win11"] = winBuild() >= 22000
	// Avviso "serve un riavvio" finché il PC non è stato riavviato dopo il cambio.
	setMu.Lock()
	when := settings.HagsReboot
	setMu.Unlock()
	if t, err := time.Parse(time.RFC3339, when); err == nil {
		up, _, _ := pTick64.Call()
		boot := time.Now().Add(-time.Duration(up) * time.Millisecond)
		if boot.After(t) {
			updateSettings(func(s *Settings) { s.HagsReboot = "" })
		} else {
			st["rebootHags"] = true
		}
	}
	return st
}

func gamingSet(id string, on bool) string {
	var err error
	msg := ""
	if on {
		// Prima di cambiare ricordo com'era (solo la prima volta: dopo sarebbe già il valore di Tasso).
		setMu.Lock()
		_, saved := settings.GamingPrev[id]
		setMu.Unlock()
		if !saved {
			if t := gamingTargets(id); len(t) > 0 {
				updateSettings(func(s *Settings) {
					if s.GamingPrev == nil {
						s.GamingPrev = map[string][]regPrev{}
					}
					s.GamingPrev[id] = t
				})
			}
		}
	} else if id != "winopt" || winBuild() >= 22000 {
		if gamingRestore(id) {
			if id == "hags" {
				updateSettings(func(s *Settings) { s.HagsReboot = time.Now().Format(time.RFC3339) })
			}
			updateSettings(func(s *Settings) { delete(s.GamingCmds, id) })
			return "ok:" + map[string]string{
				"gamedvr": "Registrazione in background: rimessa com'era prima di Tasso.",
				"mmcss":   "Priorità ai giochi: rimessa com'era prima di Tasso.",
				"nagle":   "Rete: rimessa com'era prima di Tasso.",
				"hags":    "Pianificazione GPU hardware rimessa com'era prima di Tasso: vale dopo il riavvio del PC.",
				"winopt":  "Ottimizzazioni per i giochi in finestra: rimesse com'erano prima di Tasso.",
			}[id]
		}
	}
	switch id {
	case "gamedvr":
		v := uint32(1)
		if on {
			v = 0
		}
		err = setDword(registry.CURRENT_USER, `System\GameConfigStore`, "GameDVR_Enabled", v)
		if err == nil {
			err = setDword(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\GameDVR`, "AppCaptureEnabled", v)
		}
		msg = map[bool]string{true: "Registrazione in background spenta: qualche FPS in più.", false: "Registrazione in background di Windows riaccesa."}[on]
	case "mmcss":
		if on {
			err = setDword(registry.LOCAL_MACHINE, sysProfKey, "SystemResponsiveness", 10)
			if err == nil {
				_ = setDword(registry.LOCAL_MACHINE, gamesTask, "GPU Priority", 8)
				_ = setDword(registry.LOCAL_MACHINE, gamesTask, "Priority", 6)
				_ = regSet(registry.LOCAL_MACHINE, gamesTask, "Scheduling Category", "High")
				_ = regSet(registry.LOCAL_MACHINE, gamesTask, "SFIO Priority", "High")
			}
			msg = "Priorità ai giochi accesa: Windows dà più spazio al gioco rispetto ai programmi in background."
		} else {
			err = setDword(registry.LOCAL_MACHINE, sysProfKey, "SystemResponsiveness", 20)
			if err == nil {
				_ = setDword(registry.LOCAL_MACHINE, gamesTask, "GPU Priority", 8)
				_ = setDword(registry.LOCAL_MACHINE, gamesTask, "Priority", 2)
				_ = regSet(registry.LOCAL_MACHINE, gamesTask, "Scheduling Category", "Medium")
				_ = regSet(registry.LOCAL_MACHINE, gamesTask, "SFIO Priority", "Normal")
			}
			msg = "Priorità ai giochi: rimessi i valori normali di Windows."
		}
	case "nagle":
		if on {
			err = setDword(registry.LOCAL_MACHINE, sysProfKey, "NetworkThrottlingIndex", 0xffffffff)
		} else {
			err = setDword(registry.LOCAL_MACHINE, sysProfKey, "NetworkThrottlingIndex", 10)
		}
		if err == nil {
			setNagle(on)
		}
		msg = map[bool]string{true: "Rete a bassa latenza accesa: i pacchetti del gioco partono subito. Vale dal prossimo avvio del gioco.", false: "Rete: rimessi i valori normali di Windows."}[on]
	case "hags":
		v := uint32(1)
		if on {
			v = 2
		}
		err = setDword(registry.LOCAL_MACHINE, gfxDrvKey, "HwSchMode", v)
		if err == nil {
			updateSettings(func(s *Settings) { s.HagsReboot = time.Now().Format(time.RFC3339) })
		}
		msg = map[bool]string{true: "Pianificazione GPU hardware accesa: vale dopo il riavvio del PC.", false: "Pianificazione GPU hardware spenta: vale dopo il riavvio del PC."}[on]
	case "winopt":
		if winBuild() < 22000 {
			return "Questa opzione c'è solo su Windows 11."
		}
		m := dxGlobal()
		if on {
			m["SwapEffectUpgradeEnable"] = "1"
		} else {
			delete(m, "SwapEffectUpgradeEnable")
		}
		err = setDxGlobal(m)
		msg = map[bool]string{true: "Ottimizzazioni per i giochi in finestra accese: meno ritardo nei giochi in finestra e senza bordi.", false: "Ottimizzazioni per i giochi in finestra: rimesse come Windows."}[on]
	default:
		return "Comando sconosciuto."
	}
	if err != nil {
		return "Windows non mi ha lasciato cambiare questa impostazione."
	}
	updateSettings(func(s *Settings) {
		if s.GamingCmds == nil {
			s.GamingCmds = map[string]bool{}
		}
		if on {
			s.GamingCmds[id] = true
		} else {
			delete(s.GamingCmds, id)
		}
	})
	return "ok:" + msg
}

// nagleIfaces: le schede di rete che hanno un indirizzo.
func nagleIfaces() []string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, tcpIfaceKey, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil
	}
	names, _ := k.ReadSubKeyNames(-1)
	k.Close()
	var out []string
	for _, n := range names {
		p := tcpIfaceKey + `\` + n
		ik, err := registry.OpenKey(registry.LOCAL_MACHINE, p, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		dhcp, _, _ := ik.GetStringValue("DhcpIPAddress")
		ips, _, _ := ik.GetStringsValue("IPAddress")
		ik.Close()
		if (dhcp != "" && dhcp != "0.0.0.0") || (len(ips) > 0 && ips[0] != "" && ips[0] != "0.0.0.0") {
			out = append(out, p)
		}
	}
	return out
}

// setNagle: TcpAckFrequency e TCPNoDelay sulle schede di rete che hanno un indirizzo.
// Spento senza valori salvati = come Windows di fabbrica (i due valori non ci sono).
func setNagle(on bool) {
	for _, p := range nagleIfaces() {
		ik, err := registry.OpenKey(registry.LOCAL_MACHINE, p, registry.SET_VALUE)
		if err != nil {
			continue
		}
		if on {
			_ = ik.SetDWordValue("TcpAckFrequency", 1)
			_ = ik.SetDWordValue("TCPNoDelay", 1)
		} else {
			_ = ik.DeleteValue("TcpAckFrequency")
			_ = ik.DeleteValue("TCPNoDelay")
		}
		ik.Close()
	}
}

// gamingResetAll: per "Rimetti com'era", spegne i comandi gaming che Tasso aveva acceso.
func gamingResetAll() []string {
	setMu.Lock()
	var ids []string
	for id, on := range settings.GamingCmds {
		if on {
			ids = append(ids, id)
		}
	}
	setMu.Unlock()
	var msgs []string
	for _, id := range ids {
		if r := gamingSet(id, false); strings.HasPrefix(r, "ok:") {
			msgs = append(msgs, strings.TrimPrefix(r, "ok:"))
		}
	}
	return msgs
}

// ---------------- Azioni immediate ----------------

var pNtSetSysInfo = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtSetSystemInformation")

// purgeStandby svuota la memoria in standby. Utile solo se un gioco scatta dopo
// ore di gioco: Windows la riempie di nuovo da solo, ed è normale.
func purgeStandby() string {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &tok); err != nil {
		return "Non ho i permessi per farlo."
	}
	defer tok.Close()
	var luid windows.LUID
	name, _ := windows.UTF16PtrFromString("SeProfileSingleProcessPrivilege")
	if err := windows.LookupPrivilegeValue(nil, name, &luid); err != nil {
		return "Non ho i permessi per farlo."
	}
	tp := windows.Tokenprivileges{PrivilegeCount: 1}
	tp.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
	if err := windows.AdjustTokenPrivileges(tok, false, &tp, 0, nil, nil); err != nil {
		return "Non ho i permessi per farlo."
	}
	cmd := int32(4)                                                                      // MemoryPurgeStandbyList
	r, _, _ := pNtSetSysInfo.Call(80, uintptr(unsafe.Pointer(&cmd)), unsafe.Sizeof(cmd)) // SystemMemoryListInformation
	if r != 0 {
		return fmt.Sprintf("Windows non ha accettato il comando (codice %#x).", r)
	}
	return "ok:Memoria in standby svuotata. Se un gioco scattava dopo tanto tempo, ora dovrebbe andare più liscio."
}

var pKeybd = user32.NewProc("keybd_event")

// resetGpuDriver preme Win+Ctrl+Maiusc+B: Windows riavvia il driver video in 2 secondi
// (lo schermo lampeggia). Utile se lo schermo resta nero o il gioco va a scatti dopo un crash.
func resetGpuDriver() string {
	keys := []uintptr{0x5B, 0x11, 0x10, 0x42}
	for _, k := range keys {
		pKeybd.Call(k, 0, 0, 0)
	}
	time.Sleep(60 * time.Millisecond)
	for i := len(keys) - 1; i >= 0; i-- {
		pKeybd.Call(keys[i], 0, 2, 0)
	}
	return "ok:Driver video riavviato: lo schermo lampeggia un attimo, è normale."
}
