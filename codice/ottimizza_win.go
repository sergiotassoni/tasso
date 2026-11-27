package main

// Ottimizza tutto (Tasso 3.2): la parte con Windows. La logica (quali impostazioni, già a posto,
// salva e rimetti, prima e dopo) è in ottimizza/, qui ci sono il registro vero, il piano
// energetico (powrprof), le impostazioni del desktop (SystemParametersInfo), il file di stato
// per utente nella cartella protetta lavoro\ottimizza, il backup delle chiavi toccate, il giro
// completo con la finestra e "Togli Tasso dal PC".
//
// Regole di sempre: prima si salva com'era (file di stato e backup del registro), poi si cambia;
// ogni cosa si rimette com'era con un interruttore, anche dopo un riavvio; se una modifica non c'è
// o non riesce si salta con un messaggio, senza fermare le altre.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"tasso/ottimizza"
)

// ---------------- Registro vero ----------------

type regVero struct{}

const sidServizioRete = "S-1-5-20"

func radiceOtt(r ottimizza.Radice, chiave string) (registry.Key, string, bool) {
	switch r {
	case ottimizza.HKCU:
		return registry.CURRENT_USER, chiave, true
	case ottimizza.HKLM:
		return registry.LOCAL_MACHINE, chiave, true
	case ottimizza.HKNS:
		return registry.USERS, sidServizioRete + `\` + chiave, true
	}
	return 0, "", false
}

func (regVero) Leggi(r ottimizza.Radice, chiave, nome string) (ottimizza.Valore, error) {
	v := ottimizza.Valore{Radice: r, Chiave: chiave, Nome: nome}
	root, path, ok := radiceOtt(r, chiave)
	if !ok {
		return v, ottimizza.ErrNonAccessibile
	}
	if r == ottimizza.HKNS {
		// la parte del registro del Servizio di rete c'è solo se Windows l'ha caricata
		h, err := registry.OpenKey(registry.USERS, sidServizioRete, registry.QUERY_VALUE)
		if err != nil {
			return v, ottimizza.ErrNonAccessibile
		}
		h.Close()
	}
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if errors.Is(err, registry.ErrNotExist) {
		return v, nil
	}
	if err != nil {
		return v, ottimizza.ErrNonAccessibile
	}
	defer k.Close()
	_, tipo, err := k.GetValue(nome, nil)
	if errors.Is(err, registry.ErrNotExist) {
		return v, nil
	}
	if err != nil && !errors.Is(err, registry.ErrShortBuffer) {
		return v, ottimizza.ErrNonAccessibile
	}
	switch tipo {
	case registry.DWORD:
		n, _, err := k.GetIntegerValue(nome)
		if err != nil {
			return v, ottimizza.ErrNonAccessibile
		}
		v.Esiste, v.Tipo, v.Num = true, ottimizza.TipoDword, uint32(n)
	case registry.SZ:
		s, _, err := k.GetStringValue(nome)
		if err != nil {
			return v, ottimizza.ErrNonAccessibile
		}
		v.Esiste, v.Tipo, v.Testo = true, ottimizza.TipoSz, s
	default:
		// un tipo che Tasso non scrive mai: non lo tocco (la modifica si salta; nel rimetti vale
		// come "l'hai cambiato tu")
		return v, ottimizza.ErrTipoDiverso
	}
	return v, nil
}

func (regVero) Scrivi(v ottimizza.Valore) error {
	root, path, ok := radiceOtt(v.Radice, v.Chiave)
	if !ok {
		return ottimizza.ErrNonAccessibile
	}
	k, _, err := registry.CreateKey(root, path, registry.SET_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return err
	}
	defer k.Close()
	switch v.Tipo {
	case ottimizza.TipoDword:
		return k.SetDWordValue(v.Nome, v.Num)
	case ottimizza.TipoSz:
		return k.SetStringValue(v.Nome, v.Testo)
	}
	return errors.New("tipo non valido")
}

func (regVero) Cancella(r ottimizza.Radice, chiave, nome string) error {
	root, path, ok := radiceOtt(r, chiave)
	if !ok {
		return ottimizza.ErrNonAccessibile
	}
	k, err := registry.OpenKey(root, path, registry.SET_VALUE|registry.WOW64_64KEY)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(nome); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

func (regVero) Nomi(r ottimizza.Radice, chiave string) ([]string, error) {
	root, path, ok := radiceOtt(r, chiave)
	if !ok {
		return nil, ottimizza.ErrNonAccessibile
	}
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return nil, nil
	}
	defer k.Close()
	return k.ReadValueNames(-1)
}

// regConCopia: prima di scrivere un valore per la prima volta, ne salva com'era in un file .reg
// di questo giro in %LOCALAPPDATA%\Tasso\backup-registro (come le riparazioni di "Risolvi un
// problema": con doppio clic torna com'era; "Togli Tasso dal PC" li sposta in Documenti). Dentro ci
// sono solo i valori toccati da Tasso (quelli che prima non c'erano come "nome"=-), mai chiavi intere.
type regConCopia struct {
	ottimizza.Registro
	mu     sync.Mutex
	visti  map[string]bool
	valori []ottimizza.Valore
	dir    string // "" = %LOCALAPPDATA%\Tasso\backup-registro
	file   string
}

func nuovoRegConCopia(base ottimizza.Registro) *regConCopia {
	return &regConCopia{Registro: base, visti: map[string]bool{}, file: "ottimizza-tutto_" + time.Now().Format("20060102_150405") + ".reg"}
}

func (c *regConCopia) primaDiScrivere(r ottimizza.Radice, chiave, nome string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := string(r) + `\` + strings.ToLower(chiave) + `|` + strings.ToLower(nome)
	if c.visti[id] {
		return
	}
	c.visti[id] = true
	v, err := c.Registro.Leggi(r, chiave, nome)
	if err != nil {
		return // non leggibile: la scrittura non riuscirà comunque
	}
	c.valori = append(c.valori, v)
	dir := c.dir
	if dir == "" {
		dir = filepath.Join(appDir, "backup-registro")
	}
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	// .reg come lo scrive Windows: UTF-16 con BOM
	u := utf16.Encode([]rune(ottimizza.TestoReg(c.valori)))
	b := make([]byte, 2, 2+2*len(u))
	b[0], b[1] = 0xFF, 0xFE
	for _, x := range u {
		b = append(b, byte(x), byte(x>>8))
	}
	_ = scriviFileApp(filepath.Join(dir, c.file), b, 0o644)
}

func (c *regConCopia) Scrivi(v ottimizza.Valore) error {
	c.primaDiScrivere(v.Radice, v.Chiave, v.Nome)
	return c.Registro.Scrivi(v)
}

func (c *regConCopia) Cancella(r ottimizza.Radice, chiave, nome string) error {
	c.primaDiScrivere(r, chiave, nome)
	return c.Registro.Cancella(r, chiave, nome)
}

// ---------------- Piano energetico (powrprof) ----------------

var (
	powrprofOtt    = windows.NewLazySystemDLL("powrprof.dll")
	pPwrGetActive  = powrprofOtt.NewProc("PowerGetActiveScheme")
	pPwrSetActive  = powrprofOtt.NewProc("PowerSetActiveScheme")
	pPwrReadAC     = powrprofOtt.NewProc("PowerReadACValueIndex")
	pPwrReadDC     = powrprofOtt.NewProc("PowerReadDCValueIndex")
	pPwrWriteAC    = powrprofOtt.NewProc("PowerWriteACValueIndex")
	pPwrWriteDC    = powrprofOtt.NewProc("PowerWriteDCValueIndex")
	pPwrEnumerate  = powrprofOtt.NewProc("PowerEnumerate")
	pLocalFreeOtt  = windows.NewLazySystemDLL("kernel32.dll").NewProc("LocalFree")
	errPowrprof    = errors.New("powrprof non disponibile")
	chiavePianiOtt = `SYSTEM\CurrentControlSet\Control\Power\User\PowerSchemes`
)

type enVera struct{}

func guidOtt(s string) (windows.GUID, error) {
	if !ottimizza.GUIDValido(s) {
		return windows.GUID{}, errors.New("GUID non valido")
	}
	return windows.GUIDFromString("{" + s + "}")
}

func testoGUID(g windows.GUID) string { return strings.ToLower(strings.Trim(g.String(), "{}")) }

func (enVera) PianoAttivo() (string, error) {
	if pPwrGetActive.Find() != nil {
		return "", errPowrprof
	}
	var p *windows.GUID
	if r, _, _ := pPwrGetActive.Call(0, uintptr(unsafe.Pointer(&p))); r != 0 || p == nil {
		return "", fmt.Errorf("PowerGetActiveScheme: %d", r)
	}
	s := testoGUID(*p)
	pLocalFreeOtt.Call(uintptr(unsafe.Pointer(p)))
	return s, nil
}

func (enVera) PianoEsiste(guid string) bool {
	if pPwrEnumerate.Find() != nil {
		return false
	}
	const accessScheme = 16
	for i := uintptr(0); i < 64; i++ {
		var g windows.GUID
		n := uint32(unsafe.Sizeof(g))
		if r, _, _ := pPwrEnumerate.Call(0, 0, 0, accessScheme, i, uintptr(unsafe.Pointer(&g)), uintptr(unsafe.Pointer(&n))); r != 0 {
			return false
		}
		if testoGUID(g) == guid {
			return true
		}
	}
	return false
}

func (enVera) AttivaPiano(guid string) error {
	g, err := guidOtt(guid)
	if err != nil {
		return err
	}
	if r, _, _ := pPwrSetActive.Call(0, uintptr(unsafe.Pointer(&g))); r != 0 {
		return fmt.Errorf("PowerSetActiveScheme: %d", r)
	}
	return nil
}

func (enVera) Overlay() (string, error) {
	s, ok := regGet(registry.LOCAL_MACHINE, chiavePianiOtt, "ActiveOverlayAcPowerScheme")
	s = strings.ToLower(strings.Trim(strings.TrimSpace(s), "{}"))
	if !ok || s == "" {
		return ottimizza.OverlayNessuno, nil
	}
	if !ottimizza.GUIDValido(s) {
		return "", errors.New("modalità non valida")
	}
	return s, nil
}

func (e enVera) ImpostaOverlay(guid string) error {
	if !ottimizza.GUIDValido(guid) {
		return errors.New("GUID non valido")
	}
	if _, err := runHiddenTimeout(20*time.Second, "powercfg.exe", "/overlaysetactive", guid); err != nil {
		return err
	}
	return nil
}

func (enVera) leggi(p *windows.LazyProc, piano, gruppo, imp string) (uint32, error) {
	gp, e1 := guidOtt(piano)
	gg, e2 := guidOtt(gruppo)
	gi, e3 := guidOtt(imp)
	if e1 != nil || e2 != nil || e3 != nil {
		return 0, errors.New("GUID non valido")
	}
	var v uint32
	if r, _, _ := p.Call(0, uintptr(unsafe.Pointer(&gp)), uintptr(unsafe.Pointer(&gg)), uintptr(unsafe.Pointer(&gi)), uintptr(unsafe.Pointer(&v))); r != 0 {
		return 0, fmt.Errorf("lettura del piano: %d", r)
	}
	return v, nil
}

func (e enVera) Leggi(piano, gruppo, imp string) (uint32, uint32, error) {
	if pPwrReadAC.Find() != nil {
		return 0, 0, errPowrprof
	}
	ac, err := e.leggi(pPwrReadAC, piano, gruppo, imp)
	if err != nil {
		return 0, 0, err
	}
	dc, err := e.leggi(pPwrReadDC, piano, gruppo, imp)
	return ac, dc, err
}

func (e enVera) scrivi(p *windows.LazyProc, piano, gruppo, imp string, v uint32) error {
	gp, e1 := guidOtt(piano)
	gg, e2 := guidOtt(gruppo)
	gi, e3 := guidOtt(imp)
	if e1 != nil || e2 != nil || e3 != nil {
		return errors.New("GUID non valido")
	}
	if r, _, _ := p.Call(0, uintptr(unsafe.Pointer(&gp)), uintptr(unsafe.Pointer(&gg)), uintptr(unsafe.Pointer(&gi)), uintptr(v)); r != 0 {
		return fmt.Errorf("scrittura del piano: %d", r)
	}
	// se è il piano attivo, Windows usa il valore nuovo solo dopo averlo "riattivato"
	if att, err := e.PianoAttivo(); err == nil && att == piano {
		_ = e.AttivaPiano(att)
	}
	return nil
}

func (e enVera) ScriviAC(piano, gruppo, imp string, v uint32) error {
	return e.scrivi(pPwrWriteAC, piano, gruppo, imp, v)
}

func (e enVera) ScriviDC(piano, gruppo, imp string, v uint32) error {
	return e.scrivi(pPwrWriteDC, piano, gruppo, imp, v)
}

// ---------------- Impostazioni del desktop (SystemParametersInfo) ----------------

type sisVero struct{}

const (
	spiGetAnimation           = 0x0048
	spiSetAnimation           = 0x0049
	spiGetDropShadow          = 0x1024
	spiSetDropShadow          = 0x1025
	spiGetClientAreaAnimation = 0x1042
	spiSetClientAreaAnimation = 0x1043
	spifSalvaEAvvisa          = 3 // SPIF_UPDATEINIFILE | SPIF_SENDCHANGE
)

type animationInfo struct {
	Size uint32
	Min  int32
}

func (sisVero) LeggiSPI(id string) (bool, error) {
	var r uintptr
	switch id {
	case ottimizza.SPIAnimazioni, ottimizza.SPIOmbre:
		var b int32
		code := uintptr(spiGetClientAreaAnimation)
		if id == ottimizza.SPIOmbre {
			code = spiGetDropShadow
		}
		r, _, _ = pSysParamInfo.Call(code, 0, uintptr(unsafe.Pointer(&b)), 0)
		if r == 0 {
			return false, errors.New("SystemParametersInfo")
		}
		return b != 0, nil
	case ottimizza.SPIFinestre:
		ai := animationInfo{Size: uint32(unsafe.Sizeof(animationInfo{}))}
		r, _, _ = pSysParamInfo.Call(spiGetAnimation, uintptr(ai.Size), uintptr(unsafe.Pointer(&ai)), 0)
		if r == 0 {
			return false, errors.New("SystemParametersInfo")
		}
		return ai.Min != 0, nil
	}
	return false, errors.New("impostazione sconosciuta")
}

func (sisVero) ScriviSPI(id string, on bool) error {
	v := uintptr(0)
	if on {
		v = 1
	}
	var r uintptr
	switch id {
	case ottimizza.SPIAnimazioni:
		r, _, _ = pSysParamInfo.Call(spiSetClientAreaAnimation, 0, v, spifSalvaEAvvisa)
	case ottimizza.SPIOmbre:
		r, _, _ = pSysParamInfo.Call(spiSetDropShadow, 0, v, spifSalvaEAvvisa)
	case ottimizza.SPIFinestre:
		ai := animationInfo{Size: uint32(unsafe.Sizeof(animationInfo{})), Min: int32(v)}
		r, _, _ = pSysParamInfo.Call(spiSetAnimation, uintptr(ai.Size), uintptr(unsafe.Pointer(&ai)), spifSalvaEAvvisa)
	default:
		return errors.New("impostazione sconosciuta")
	}
	if r == 0 {
		return errors.New("SystemParametersInfo")
	}
	return nil
}

func (sisVero) ImpostaMouse(s1, s2, vel int) error {
	m := [3]int32{int32(s1), int32(s2), int32(vel)}
	if r, _, _ := pSysParamInfo.Call(spiSetMouse, 0, uintptr(unsafe.Pointer(&m[0])), spifSalvaEAvvisa); r == 0 {
		return errors.New("SystemParametersInfo")
	}
	return nil
}

func (sisVero) Avvisa(area string) {
	p, err := windows.UTF16PtrFromString(area)
	if err != nil {
		return
	}
	var res uintptr
	// HWND_BROADCAST, WM_SETTINGCHANGE, SMTO_ABORTIFHUNG, al massimo un secondo
	pSendMsgTimeout.Call(0xFFFF, 0x1A, 0, uintptr(unsafe.Pointer(p)), 2, 1000, uintptr(unsafe.Pointer(&res)))
}

// ---------------- Preferenze di Tasso e pulizia ----------------

type prefVere struct{}

func (prefVere) NotificheGioco() bool {
	setMu.Lock()
	defer setMu.Unlock()
	return settings.OttNotifGioco
}

func (prefVere) ImpostaNotificheGioco(on bool) error {
	updateSettings(func(s *Settings) { s.OttNotifGioco = on })
	if !on {
		ottNotificheFine()
	}
	return nil
}

// pulVero: i passaggi di "Pulisci" (gli stessi script), uno alla volta. "backup" prima salva la
// foto di com'era Windows, come la pulizia Media (passoConFoto).
type pulVero struct {
	ctx context.Context
	log func(string)
}

func (p pulVero) Pulisci(id string) (int64, error) {
	var st *Step
	for i := range allSteps {
		if allSteps[i].ID == id {
			st = &allSteps[i]
			break
		}
	}
	if st == nil {
		return 0, errors.New("passaggio sconosciuto")
	}
	if passoConFoto[id] && !ensureSnapshot() && id != "backup" {
		return 0, errors.New("foto di com'era non salvata")
	}
	if p.log != nil {
		p.log("» " + st.Name)
	}
	r := runPSInfo(p.ctx, st.Script, st.Timeout, func(m string) {
		if p.log != nil {
			p.log("   " + m)
		}
	}, nil, nil, true)
	if r.TooSlow {
		return r.Freed, errors.New("troppo lento")
	}
	return r.Freed, nil
}

// ---------------- Il PC ----------------

// adattatoreOtt: una scheda video vista da DirectX, con il suo LUID (per chiedere al driver).
type adattatoreOtt struct {
	nome     string
	vendor   uint32
	software bool
	luidLow  uint32
	luidHigh int32
}

func adattatoriOtt() (out []adattatoreOtt) {
	defer guard("ottimizza:adattatori")
	if pCreateDXGIFactory.Find() != nil {
		return nil
	}
	var fac uintptr
	if hr, _, _ := pCreateDXGIFactory.Call(uintptr(unsafe.Pointer(&iidDXGIFactory1)), uintptr(unsafe.Pointer(&fac))); uint32(hr) != 0 || fac == 0 {
		return nil
	}
	defer comCall(fac, 2)
	for i := uintptr(0); i < 16; i++ {
		var ad uintptr
		if hr := comCall(fac, 12, i, uintptr(unsafe.Pointer(&ad))); uint32(hr) != 0 || ad == 0 {
			break
		}
		var d dxgiAdapterDesc1
		hr := comCall(ad, 10, uintptr(unsafe.Pointer(&d)))
		comCall(ad, 2)
		if uint32(hr) != 0 {
			continue
		}
		out = append(out, adattatoreOtt{nome: strings.TrimSpace(windows.UTF16ToString(d.Description[:])), vendor: d.VendorID,
			software: d.Flags&2 != 0 || d.VendorID == 0x1414, luidLow: d.LuidLow, luidHigh: d.LuidHigh})
	}
	return out
}

var (
	pKmtOpenLuid  = windows.NewLazySystemDLL("gdi32.dll").NewProc("D3DKMTOpenAdapterFromLuid")
	pKmtQueryInfo = windows.NewLazySystemDLL("gdi32.dll").NewProc("D3DKMTQueryAdapterInfo")
	pKmtClose     = windows.NewLazySystemDLL("gdi32.dll").NewProc("D3DKMTCloseAdapter")
)

// hagsOtt: la pianificazione GPU hardware, chiesta al driver (KMTQAITYPE_WDDM_2_7_CAPS = 70):
// supportata e accesa. Su Windows più vecchi o driver che non la conoscono: false, false.
func hagsOtt(a adattatoreOtt) (supportata, accesa bool) {
	defer guard("ottimizza:hags")
	if pKmtOpenLuid.Find() != nil || pKmtQueryInfo.Find() != nil {
		return false, false
	}
	var open struct {
		LuidLow  uint32
		LuidHigh int32
		Adapter  uint32
	}
	open.LuidLow, open.LuidHigh = a.luidLow, a.luidHigh
	if r, _, _ := pKmtOpenLuid.Call(uintptr(unsafe.Pointer(&open))); r != 0 {
		return false, false
	}
	defer func() {
		cl := struct{ Adapter uint32 }{open.Adapter}
		pKmtClose.Call(uintptr(unsafe.Pointer(&cl)))
	}()
	var caps uint32
	q := struct {
		Adapter uint32
		Tipo    uint32
		Dati    uintptr
		Dim     uint32
		_       uint32
	}{Adapter: open.Adapter, Tipo: 70, Dati: uintptr(unsafe.Pointer(&caps)), Dim: 4}
	if r, _, _ := pKmtQueryInfo.Call(uintptr(unsafe.Pointer(&q))); r != 0 {
		return false, false
	}
	return caps&1 != 0, caps&2 != 0
}

// giochiOtt: i programmi dei giochi trovati da Tasso (nome com'è scritto, solo quelli che ci sono).
func giochiOtt() []string {
	libMu.Lock()
	var l []string
	for _, g := range library {
		if g.Hidden || g.exe == "" || notGameExe[strings.ToLower(filepath.Base(g.exe))] {
			continue
		}
		l = append(l, filepath.Clean(g.exe))
	}
	libMu.Unlock()
	var out []string
	for _, p := range l {
		if fileExists(p) {
			out = append(out, p)
		}
	}
	return out
}

// batteriaOtt: GetSystemPowerStatus.BatteryFlag (128 = nessuna batteria, 255 = non si sa).
func batteriaOtt() uint8 {
	var st struct {
		ACLineStatus, BatteryFlag, BatteryLifePercent, SystemStatusFlag byte
		BatteryLifeTime, BatteryFullLifeTime                            uint32
	}
	if r, _, _ := pGetPowerStatus.Call(uintptr(unsafe.Pointer(&st))); r == 0 {
		return 255
	}
	return st.BatteryFlag
}

// modoTassoOtt: la modalità di Tasso accesa che gestisce piano, mouse e Modalità Gioco ("" = nessuna).
func modoTassoOtt() string {
	setMu.Lock()
	m := settings.Mode
	setMu.Unlock()
	switch m {
	case "gaming", "fps", "film", "silenzioso":
		return m
	}
	return ""
}

// altroUtenteOtt: Tasso da amministratore con un altro account (UAC con la password di un altro
// amministratore). Le impostazioni dell'utente collegato non sono raggiungibili da qui.
func altroUtenteOtt() bool { return elevato() && utenteDiverso(sidUtente()) }

func pcOtt() ottimizza.PC {
	in := leggiInfoPC()
	letto := in.RAMGB > 0 || in.PCType != 0 || len(in.Telai) > 0
	pc := ottimizza.PC{Admin: elevato(), Caricatore: !onBattery(), Build: winBuild(), Giochi: giochiOtt(),
		AltroUtente: altroUtenteOtt(), ModoTasso: modoTassoOtt()}
	pc.Portatile, pc.PortatileDubbio = ottimizza.DecidiPortatile(letto, in.Portatile, batteriaOtt())
	for _, a := range adattatoriOtt() {
		if a.software {
			continue
		}
		pc.SchedeVere++
		if s, on := hagsOtt(a); s {
			pc.HAGS = true
			if on {
				pc.HAGSAcceso = true
			}
		}
	}
	return pc
}

// ---------------- File di stato ----------------

var (
	ottMu       sync.Mutex  // il file di stato: uno alla volta
	ottInCorso  atomic.Bool // il giro di Ottimizza tutto sta lavorando
	ottStop     atomic.Bool // premuto "Fermati"
	ottSkip     atomic.Bool // premuto "Salta": il resto della categoria si salta
	ottRovinato atomic.Bool // il file di stato era illeggibile (messo da parte)
)

func sidOtt() string {
	if sid := sidUtente(); sid != "" {
		return sid
	}
	return "utente"
}

// fileStatoOtt: Programmi\Tasso\lavoro\ottimizza\stato-<utente>.json (lì scrivono solo gli
// amministratori). Senza permessi (account normale) Tasso non può scrivere lì: usa la sua cartella
// dell'utente, e da quel file non rimette mai valori di tutto il PC (ottimizza.Leggi admin=false).
func fileStatoOtt() (string, bool) {
	nome := "stato-" + sidOtt() + ".json"
	if elevato() {
		if d := cartellaLavoroProtetta(); d != "" {
			d = filepath.Join(d, "ottimizza")
			if os.MkdirAll(d, 0o755) == nil {
				return filepath.Join(d, nome), true
			}
		}
	}
	d := filepath.Join(appDir, "ottimizza")
	_ = os.MkdirAll(d, 0o755)
	return filepath.Join(d, nome), false
}

func fileStatoUtenteOtt() string {
	return filepath.Join(appDir, "ottimizza", "stato-"+sidOtt()+".json")
}

// leggiStatoFile: il file (o, se è vuoto o rovinato, la sua copia .prima). admin: come ottimizza.Leggi.
func leggiStatoFile(p string, protetto, admin bool) (*ottimizza.Stato, bool, error) {
	leggi := func(f string) ([]byte, error) {
		if protetto {
			return os.ReadFile(f)
		}
		return leggiFileApp(f)
	}
	b, err := leggi(p)
	if err != nil {
		return nil, false, err
	}
	st, scartate, err := ottimizza.Leggi(b, admin)
	if err != nil {
		// file a metà (corrente mancata mentre si scriveva): la copia di prima
		if b2, e2 := leggi(p + ".prima"); e2 == nil {
			if st2, sc2, e3 := ottimizza.Leggi(b2, admin); e3 == nil {
				return st2, sc2 > 0, nil
			}
		}
		return nil, true, err
	}
	return st, scartate > 0, nil
}

// caricaOtt legge il file di stato (chiamare con ottMu preso). Da amministratore unisce anche il
// file dell'utente di un giro fatto senza permessi e poi lo toglie: lo stato vero sta in un posto solo.
func caricaOtt() *ottimizza.Stato {
	vuoto := &ottimizza.Stato{Versione: ottimizza.VersioneStato, Risultati: map[string]*ottimizza.Risultato{}}
	p, protetto := fileStatoOtt()
	st, scartate, err := leggiStatoFile(p, protetto, protetto)
	if err != nil && !os.IsNotExist(err) && !errors.Is(err, errPercorsoLink) {
		ottRovinato.Store(true)
		via := filepath.Join(filepath.Dir(p), "stato-illeggibile-"+time.Now().Format("20060102-150405")+".json")
		if protetto {
			_ = os.Rename(p, via)
		} else {
			_ = rinominaApp(p, via)
		}
	}
	if st == nil {
		st = vuoto
	}
	if scartate {
		logPanic("ottimizza", "file di stato: voci non valide scartate", nil)
	}
	if st.Risultati == nil {
		st.Risultati = map[string]*ottimizza.Risultato{}
	}
	if protetto {
		u := fileStatoUtenteOtt()
		if su, _, e := leggiStatoFile(u, false, false); e == nil {
			st.Unisci(su)
			if salvaOtt(st) == nil {
				_ = os.Remove(u)
				_ = os.Remove(u + ".prima")
			}
		}
	}
	return st
}

// scriviSicuroOtt: file nuovo scritto davvero su disco (Sync), copia .prima di quello vecchio,
// poi al posto di quello vecchio: mai un file a metà.
func scriviSicuroOtt(p string, b []byte, protetto bool) error {
	if !protetto {
		if err := controllaPercorsoApp(filepath.Dir(p)); err != nil {
			return err
		}
	}
	scrivi := func(f string, dati []byte) error {
		// mai seguire un collegamento messo al posto del file (Tasso amministratore nella cartella
		// dell'utente): lo tolgo, poi creo il file nuovo
		if fi, e := os.Lstat(f); e == nil && (èLink(fi) || !fi.Mode().IsRegular()) {
			if !èLink(fi) || os.Remove(f) != nil {
				return fmt.Errorf("%s non è un file normale", filepath.Base(f))
			}
		}
		h, err := os.OpenFile(f, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		if _, err := h.Write(dati); err != nil {
			h.Close()
			return err
		}
		if err := h.Sync(); err != nil {
			h.Close()
			return err
		}
		return h.Close()
	}
	if old, err := os.ReadFile(p); err == nil && len(old) > 0 {
		if fi, e := os.Lstat(p); e == nil && !èLink(fi) {
			_ = scrivi(p+".prima", old)
		}
	}
	tmp := p + ".nuovo"
	if err := scrivi(tmp, b); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// salvaOtt scrive il file di stato. Chiamare con ottMu preso.
func salvaOtt(st *ottimizza.Stato) error {
	b, err := st.Codifica()
	if err != nil {
		return err
	}
	p, protetto := fileStatoOtt()
	return scriviSicuroOtt(p, b, protetto)
}

// ambienteOtt: registro con backup, piano energetico, desktop, preferenze e pulizia veri.
// Per cambiare (giro, riaccendere un interruttore): il .reg di backup tiene i valori di prima.
func ambienteOtt(ctx context.Context, log func(string)) *ottimizza.Ambiente {
	return &ottimizza.Ambiente{Reg: nuovoRegConCopia(regVero{}), En: enVera{}, Sis: sisVero{}, Pref: prefVere{}, Pul: pulVero{ctx: ctx, log: log}, PC: pcOtt(),
		Attendi: func(d time.Duration) { time.Sleep(d) }}
}

// ambienteRimettiOtt: come ambienteOtt ma senza .reg di backup. Rimettendo com'era, "com'era" è il
// valore di Tasso: un .reg con quello, con un doppio clic (anche da "Torna indietro"), rifarebbe le
// modifiche. Il .reg del giro, con i valori di prima di Tasso, resta quello buono.
func ambienteRimettiOtt(ctx context.Context, log func(string)) *ottimizza.Ambiente {
	a := ambienteOtt(ctx, log)
	a.Reg = regVero{}
	return a
}

// ---------------- Informazioni per la finestra ----------------

func modoAttualeOtt() string {
	if officeMode() {
		return ottimizza.ModoUfficio
	}
	return ottimizza.ModoGioco
}

// infoOtt: lo stato per la finestra (voci, ultimi risultati con la tabella prima → dopo).
func infoOtt() map[string]interface{} {
	ottMu.Lock()
	st := caricaOtt()
	ottMu.Unlock()
	return infoDaStatoOtt(st)
}

func infoDaStatoOtt(st *ottimizza.Stato) map[string]interface{} {
	voci := []map[string]interface{}{}
	for _, v := range st.Voci {
		m, _ := ottimizza.Trova(v.ID)
		voci = append(voci, map[string]interface{}{"id": v.ID, "stato": v.Stato, "motivo": v.Motivo, "quando": v.Quando, "riavvio": m.Riavvio, "nonRimette": m.NonRimette})
	}
	ris := map[string]interface{}{}
	for modo, r := range st.Risultati {
		if r == nil {
			continue
		}
		righe, piccolo := ottimizza.Confronta(*r)
		ris[modo] = map[string]interface{}{"risultato": r, "righe": righe, "piccolo": piccolo}
	}
	return map[string]interface{}{
		"modo": modoAttualeOtt(), "inCorso": ottInCorso.Load(), "voci": voci, "risultati": ris,
		"attive": len(st.Attive()), "rovinato": ottRovinato.Load(), "admin": elevato(),
	}
}

// ---------------- Rimetti com'era ----------------

func rimettiOtt(id string) ottimizza.Esito {
	if ottInCorso.Load() {
		return ottimizza.Esito{ID: id, Stato: ottimizza.VoceErrore, Motivo: "Aspetta che Ottimizza tutto finisca, poi riprova."}
	}
	a := ambienteRimettiOtt(context.Background(), nil)
	ottMu.Lock()
	defer ottMu.Unlock()
	st := caricaOtt()
	return ottimizza.Rimetti(id, a, st, salvaOtt)
}

// applicaUnaOtt: riaccende una sola modifica (interruttore riacceso nell'elenco).
func applicaUnaOtt(id string) ottimizza.Esito {
	m, ok := ottimizza.Trova(id)
	if !ok || m.NonRimette {
		return ottimizza.Esito{ID: id, Stato: ottimizza.VoceErrore, Motivo: ottimizza.MsgNienteDa}
	}
	if ottInCorso.Load() {
		return ottimizza.Esito{ID: id, Stato: ottimizza.VoceErrore, Motivo: "Aspetta che Ottimizza tutto finisca, poi riprova."}
	}
	a := ambienteOtt(context.Background(), nil)
	ottMu.Lock()
	defer ottMu.Unlock()
	st := caricaOtt()
	return ottimizza.Esegui(m, a, st, salvaOtt)
}

// rimettiTuttoOtt: tutte le modifiche attive (anche da "Rimetti tutto com'era" e da "Togli Tasso").
// forza=false: con un giro in corso non fa niente (lo dice).
func rimettiTuttoOtt(forza bool) ([]ottimizza.Esito, bool) {
	if !forza && ottInCorso.Load() {
		return []ottimizza.Esito{{Stato: ottimizza.VoceErrore, Motivo: "Aspetta che Ottimizza tutto finisca, poi riprova."}}, false
	}
	ottMu.Lock()
	st := caricaOtt()
	n := len(st.Attive())
	ottMu.Unlock()
	if n == 0 {
		return nil, true
	}
	a := ambienteRimettiOtt(context.Background(), nil)
	ottMu.Lock()
	defer ottMu.Unlock()
	st = caricaOtt()
	return ottimizza.RimettiTutto(a, st, salvaOtt)
}

// attiveOtt: quante modifiche di Ottimizza tutto ci sono da rimettere.
func attiveOtt() int {
	ottMu.Lock()
	defer ottMu.Unlock()
	return len(caricaOtt().Attive())
}

// fermaGiroOtt: ferma un giro in corso (come "Fermati") e aspetta al massimo 2 minuti che finisca.
func fermaGiroOtt() {
	if !ottInCorso.Load() {
		return
	}
	ottStop.Store(true)
	ottCancelMu.Lock()
	if ottCancel != nil {
		ottCancel()
	}
	ottCancelMu.Unlock()
	for i := 0; i < 240 && ottInCorso.Load(); i++ {
		time.Sleep(500 * time.Millisecond)
	}
}

// ottSpegniTutto: "Togli Tasso dal PC" (come la Modalità riunione e il blocco del PC): rimette
// com'erano le modifiche di Ottimizza tutto e la pausa delle notifiche durante il gioco. Non blocca
// mai la disinstallazione: quello che non torna (un altro account, un valore che Windows non lascia
// rimettere, un file rovinato) va con le copie di sicurezza in Documenti e si dice (avvisi).
func ottSpegniTutto() (avvisi []string) {
	fermaGiroOtt()
	// una modalità di Tasso accesa prima torna a Normale: rimette i suoi "com'era", così Ottimizza
	// trova i suoi valori e li rimette (senza, la modalità rimetterebbe dopo quelli di Ottimizza)
	if modoTassoOtt() != "" {
		setMode("normale")
	}
	ottNotificheFine()
	updateSettings(func(s *Settings) { s.OttNotifGioco = false })
	if _, ok := rimettiTuttoOtt(true); !ok {
		if p, _ := fileStatoOtt(); copiaPerDocumentiOtt(p) {
			avvisi = append(avvisi, "Ottimizza tutto: alcune impostazioni non sono tornate com'erano (Windows non le lascia rimettere). Tolgo Tasso lo stesso: l'elenco con i valori di prima va con le copie di sicurezza in Documenti.")
		}
	}
	avvisi = append(avvisi, ottAltriFile()...)
	return avvisi
}

// copiaPerDocumentiOtt: una copia del file in %LOCALAPPDATA%\Tasso\backup-registro, che "Togli
// Tasso dal PC" sposta in Documenti con le altre copie di sicurezza.
func copiaPerDocumentiOtt(p string) bool {
	b, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	dir := filepath.Join(appDir, "backup-registro")
	if os.MkdirAll(dir, 0o755) != nil {
		return false
	}
	nome := "ottimizza-tutto-" + strings.TrimSuffix(filepath.Base(p), ".json") + "_" + time.Now().Format("20060102_150405") + ".json"
	return scriviFileApp(filepath.Join(dir, nome), b, 0o644) == nil
}

const leggimiRegOtt = "Questo file rimette com'erano le impostazioni cambiate da Ottimizza tutto di Tasso.\nCome si usa: entra in Windows con l'account di cui sono le impostazioni, fai doppio clic sul file .reg con lo stesso nome, rispondi Sì alle domande di Windows, poi esci e rientra.\nDentro ci sono solo i valori di prima: non rifà nessuna modifica. Dopo lo puoi cancellare."

// regAltroAccountOtt: dopo Togli Tasso, per le modifiche di un altro account che Tasso non può
// raggiungere, un .reg con i valori di prima (solo HKCU, solo quelli toccati) in Documenti pubblici\Tasso,
// che tutti gli account del PC vedono, più una copia con le copie di sicurezza in Documenti. Va aperto
// da quell'account: HKEY_CURRENT_USER è il suo. Dentro ci sono solo i valori di prima di Tasso: un
// doppio clic rimette com'era, mai le modifiche.
func regAltroAccountOtt(st *ottimizza.Stato) bool {
	v := ottimizza.ValoriUtente(st)
	if len(v) == 0 {
		return false
	}
	u := utf16.Encode([]rune(ottimizza.TestoReg(v)))
	b := make([]byte, 2, 2+2*len(u))
	b[0], b[1] = 0xFF, 0xFE
	for _, x := range u {
		b = append(b, byte(x), byte(x>>8))
	}
	nome := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return '-'
		}
		return r
	}, trGo("Tasso - rimetti com'era le impostazioni di questo account")) + "_" + time.Now().Format("20060102_150405") + ".reg"
	ok := false
	if pub, err := windows.KnownFolderPath(windows.FOLDERID_PublicDocuments, 0); err == nil && pub != "" {
		d := filepath.Join(pub, "Tasso")
		if fi, e := os.Lstat(d); e == nil && èLink(fi) {
			d = "" // mai seguire un collegamento in una cartella che tutti possono scrivere
		}
		if d != "" && os.MkdirAll(d, 0o755) == nil {
			f := filepath.Join(d, nome)
			if fi, e := os.Lstat(f); !(e == nil && èLink(fi)) {
				ok = os.WriteFile(f, b, 0o644) == nil
			}
			// due righe per chi lo trova lì
			g := filepath.Join(d, strings.TrimSuffix(nome, ".reg")+".txt")
			if fi, e := os.Lstat(g); ok && !(e == nil && èLink(fi)) {
				_ = os.WriteFile(g, []byte("\ufeff"+strings.ReplaceAll(trGo(leggimiRegOtt), "\n", "\r\n")), 0o644)
			}
		}
	}
	if dir := filepath.Join(appDir, "backup-registro", "altro account"); os.MkdirAll(dir, 0o755) == nil {
		if scriviFileApp(filepath.Join(dir, nome), b, 0o644) == nil {
			ok = true
		}
	}
	return ok
}

// ottAltriFile: i file di stato e delle notifiche diversi da quello dell'utente di adesso (un
// altro account del PC, un account cancellato, un giro fatto senza permessi in un'altra cartella).
// Le modifiche di tutto il PC (HKLM, piano energetico) le rimetto io; quelle di un altro account
// non le posso raggiungere: il file va con le copie di sicurezza e lo dico. Mai un blocco.
func ottAltriFile() []string {
	var avvisi []string
	mio, _ := fileStatoOtt()
	type file struct {
		p        string
		protetto bool
	}
	var l []file
	if d := cartellaLavoroProtetta(); d != "" {
		f, _ := filepath.Glob(filepath.Join(d, "ottimizza", "*.json"))
		for _, x := range f {
			l = append(l, file{x, true})
		}
	}
	f, _ := filepath.Glob(filepath.Join(appDir, "ottimizza", "*.json"))
	for _, x := range f {
		l = append(l, file{x, false})
	}
	altri, illeggibili, conReg := 0, 0, 0
	for _, x := range l {
		base := filepath.Base(x.p)
		if strings.EqualFold(x.p, mio) || strings.HasPrefix(base, "notifiche-") && strings.EqualFold(x.p, fileNotificheOtt()) {
			continue
		}
		if strings.HasPrefix(base, "notifiche-") {
			// pausa delle notifiche di un altro account: si riaccendono quando quell'account riapre Tasso
			if copiaPerDocumentiOtt(x.p) {
				altri++
			}
			continue
		}
		if !strings.HasPrefix(base, "stato-") {
			continue
		}
		if strings.Contains(base, "illeggibile") {
			if copiaPerDocumentiOtt(x.p) {
				illeggibili++
			}
			continue
		}
		st, _, err := leggiStatoFile(x.p, x.protetto, x.protetto)
		if err != nil {
			if copiaPerDocumentiOtt(x.p) {
				illeggibili++
			}
			continue
		}
		if len(st.Attive()) == 0 {
			continue
		}
		// solo le modifiche di tutto il PC: quelle dell'utente sono di un altro account
		a := ambienteRimettiOtt(context.Background(), nil)
		a.PC.AltroUtente = true
		salva := func(s *ottimizza.Stato) error {
			b, err := s.Codifica()
			if err != nil {
				return err
			}
			return scriviSicuroOtt(x.p, b, x.protetto)
		}
		ottimizza.RimettiTutto(a, st, salva)
		if len(st.Attive()) > 0 {
			if copiaPerDocumentiOtt(x.p) {
				altri++
			}
			if regAltroAccountOtt(st) {
				conReg++
			}
		}
	}
	if conReg > 0 {
		avvisi = append(avvisi, "Ottimizza tutto: alcune impostazioni di un altro account del PC non sono tornate com'erano (Tasso non può cambiare le impostazioni di un altro account). Per rimetterle: entra in Windows con quell'account, apri Documenti pubblici, cartella Tasso, fai doppio clic sul file \"Tasso - rimetti com'era…\", rispondi Sì, poi esci e rientra.")
	} else if altri > 0 {
		avvisi = append(avvisi, "Ottimizza tutto: alcune impostazioni di un altro account del PC non sono tornate com'erano (Tasso non può cambiare le impostazioni di un altro account). Tolgo Tasso lo stesso: l'elenco va con le copie di sicurezza in Documenti.")
	}
	if illeggibili > 0 {
		avvisi = append(avvisi, "Ottimizza tutto: un file con le impostazioni di prima era rovinato. Tolgo Tasso lo stesso: il file va con le copie di sicurezza in Documenti.")
	}
	return avvisi
}

// msgRimettiOtt: per "Rimetti tutto com'era" delle Impostazioni.
func msgRimettiOtt() ([]string, bool) {
	es, ok := rimettiTuttoOtt(false)
	if len(es) == 0 {
		return nil, true
	}
	if !ok {
		return []string{"Ottimizza tutto: non sono riuscito a rimettere tutto com'era. Riprova tra poco dalla pagina Ottimizza tutto."}, false
	}
	return []string{"Ottimizza tutto: modifiche rimesse com'erano."}, true
}

// ---------------- Notifiche in pausa durante il gioco ----------------

// Con "notifiche in pausa durante il gioco" accesa (e Tasso acceso), quando parte un gioco Tasso
// spegne i fumetti delle notifiche e li riaccende un minuto dopo la fine della partita. Com'era
// prima sta sempre nello stesso posto, %LOCALAPPDATA%\Tasso\ottimizza\notifiche-<utente>.json
// (con o senza permessi), scritto prima del cambio: se Tasso o il PC si spengono a partita in
// corso, alla prossima apertura Tasso rimette com'era. Il file contiene solo "com'era" (none o 1):
// qualsiasi altra cosa riaccende i fumetti, mai spenti per sempre.
// Se la modalità di Tasso per i giochi (Gaming, FPS, Film) è già accesa, le notifiche le mette lei.

const chiaveToastOtt = `Software\Microsoft\Windows\CurrentVersion\PushNotifications`

func fileNotificheOtt() string {
	return filepath.Join(appDir, "ottimizza", "notifiche-"+sidOtt()+".json")
}

var ottNotifMu sync.Mutex

// ottNotificheInizio: spegne i fumetti per la partita (se erano accesi).
func ottNotificheInizio() {
	ottNotifMu.Lock()
	defer ottNotifMu.Unlock()
	if altroUtenteOtt() {
		return // i fumetti sono dell'utente collegato, non dell'account di Tasso
	}
	f := fileNotificheOtt()
	if _, err := os.Stat(f); err == nil {
		return // già in pausa
	}
	v, ok := regDword(registry.CURRENT_USER, chiaveToastOtt, "ToastEnabled")
	if ok && v == 0 {
		return // già spenti (da te o da un'altra parte di Tasso): non li tocco
	}
	prima := "none"
	if ok {
		prima = fmt.Sprint(v)
	}
	b, _ := json.Marshal(map[string]string{"prima": prima, "at": time.Now().Format(time.RFC3339)})
	_ = os.MkdirAll(filepath.Dir(f), 0o755)
	if scriviSicuroOtt(f, b, false) != nil {
		return // senza il file non cambio niente
	}
	if setDword(registry.CURRENT_USER, chiaveToastOtt, "ToastEnabled", 0) != nil {
		_ = os.Remove(f)
	}
}

// ottNotificheFine: rimette i fumetti com'erano, solo se sono ancora spenti come li ha messi Tasso.
// Guarda anche il posto della prima versione (lavoro\ottimizza).
func ottNotificheFine() {
	ottNotifMu.Lock()
	defer ottNotifMu.Unlock()
	files := []string{fileNotificheOtt()}
	if d := cartellaLavoroProtetta(); d != "" {
		files = append(files, filepath.Join(d, "ottimizza", "notifiche-"+sidOtt()+".json"))
	}
	for i, f := range files {
		var b []byte
		var err error
		if i == 0 {
			b, err = leggiFileApp(f) // cartella dell'utente: niente collegamenti
		} else {
			b, err = os.ReadFile(f) // cartella protetta (solo amministratori)
		}
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			b = nil // illeggibile: "prima" vuoto = riaccendo i fumetti, mai spenti per sempre
		}
		var s struct {
			Prima string `json:"prima"`
		}
		_ = json.Unmarshal(b, &s)
		v, ok := regDword(registry.CURRENT_USER, chiaveToastOtt, "ToastEnabled")
		switch ottimizza.NotificheDaRimettere(s.Prima, v, ok) {
		case "cancella":
			regDel(registry.CURRENT_USER, chiaveToastOtt, "ToastEnabled")
		case "accendi":
			_ = setDword(registry.CURRENT_USER, chiaveToastOtt, "ToastEnabled", 1)
		}
		_ = os.Remove(f)
		_ = os.Remove(f + ".prima")
	}
}

// ottGuardiaGiochi: gira dentro Tasso. Ogni 10 secondi guarda se c'è un gioco aperto.
func ottGuardiaGiochi() {
	defer guard("ottimizza:guardia-giochi")
	time.Sleep(30 * time.Second)
	if runningGame() == "" {
		ottNotificheFine() // pausa rimasta da un Tasso chiuso a partita in corso
	}
	senza := 0
	for {
		time.Sleep(10 * time.Second)
		setMu.Lock()
		on, modo, spento := settings.OttNotifGioco, settings.Mode, settings.Paused
		setMu.Unlock()
		if !on || spento {
			ottNotificheFine()
			continue
		}
		if runningGame() != "" {
			senza = 0
			if modo != "gaming" && modo != "fps" && modo != "film" {
				ottNotificheInizio()
			}
			continue
		}
		senza++
		if senza >= 6 { // un minuto senza giochi aperti
			ottNotificheFine()
		}
	}
}

// ---------------- Collegamenti con la finestra ----------------

func bindOttimizza(w webview2.WebView) {
	webview2.Subito["goOttSalta"] = true
	webview2.Subito["goOttFerma"] = true
	webview2.Subito["goOttInfo"] = true
	_ = w.Bind("goOttInfo", func() map[string]interface{} { return infoOtt() })
	_ = w.Bind("goOttAvvia", func(modo string) string {
		if modo != ottimizza.ModoGioco && modo != ottimizza.ModoUfficio {
			return "no"
		}
		if isRunning() || ottInCorso.Load() {
			return "busy"
		}
		return async("ottimizza", func() { giroOtt(modo) })
	})
	_ = w.Bind("goOttSalta", func() string { ottSkip.Store(true); return "ok" })
	_ = w.Bind("goOttFerma", func() string {
		ottStop.Store(true)
		ottCancelMu.Lock()
		if ottCancel != nil {
			ottCancel()
		}
		ottCancelMu.Unlock()
		return "ok"
	})
	_ = w.Bind("goOttRimetti", func(id string) string {
		return async("ottimizza-rimetti", func() {
			e := rimettiOtt(id)
			emit("onOttRimesso", map[string]interface{}{"esito": e, "info": infoOtt()})
		})
	})
	_ = w.Bind("goOttApplica", func(id string) string {
		return async("ottimizza-rimetti", func() {
			e := applicaUnaOtt(id)
			emit("onOttRimesso", map[string]interface{}{"esito": e, "info": infoOtt()})
		})
	})
	_ = w.Bind("goOttRimettiTutto", func() string {
		return async("ottimizza-rimetti", func() {
			es, ok := rimettiTuttoOtt(false)
			emit("onOttRimesso", map[string]interface{}{"tutto": true, "ok": ok, "n": len(es), "info": infoOtt()})
		})
	})
	_ = w.Bind("goOttAccensione", func() string {
		return async("ottimizza-acc", func() {
			if accensioneDopoOtt() {
				emit("onOttAggiornato", infoOtt())
			}
		})
	})
	_ = w.Bind("goOttSalva", func(dataURL string) string { return salvaBigliettoOtt(dataURL) })
}

// salvaBigliettoOtt: Immagini\Tasso\ottimizza-tasso-AAAA-MM-GG-hhmm.png, poi apre la cartella.
func salvaBigliettoOtt(dataURL string) string {
	b, _, err := pngDaDataURL(dataURL)
	if err != nil {
		return "no"
	}
	dir, err := windows.KnownFolderPath(windows.FOLDERID_Pictures, 0)
	if err != nil || dir == "" {
		return "no"
	}
	dir = filepath.Join(dir, "Tasso")
	if os.MkdirAll(dir, 0o755) != nil {
		return "no"
	}
	base := "ottimizza-tasso-" + time.Now().Format("2006-01-02-1504")
	p := filepath.Join(dir, base+".png")
	for i := 2; i < 100; i++ {
		if _, err := os.Stat(p); err != nil {
			break
		}
		p = filepath.Join(dir, fmt.Sprintf("%s-%d.png", base, i))
	}
	if os.WriteFile(p, b, 0o644) != nil {
		return "no"
	}
	openFolderOf(p)
	return "ok"
}

// ottAllAvvio: all'apertura di Tasso. Guardia delle notifiche durante il gioco e, in Ufficio,
// l'accensione "dopo" (Windows la scrive qualche minuto dopo la riaccensione del PC).
func ottAllAvvio() {
	defer guard("ottimizza:avvio")
	go ottGuardiaGiochi()
	for i := 0; i < 8; i++ {
		time.Sleep(4 * time.Minute)
		ottMu.Lock()
		st := caricaOtt()
		ottMu.Unlock()
		r := st.Risultati[ottimizza.ModoUfficio]
		if r == nil || !r.AccInAttesa {
			return
		}
		if accensioneDopoOtt() {
			emit("onOttAggiornato", infoOtt())
			return
		}
	}
}
