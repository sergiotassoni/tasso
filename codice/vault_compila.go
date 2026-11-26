package main

// Portachiavi > Compila: Tasso riempie da solo utente e password nel programma o nel sito davanti.
//
// Tre modi, tutti senza estensioni del browser:
//  1. Finestrella "Vuoi usare uno di questi account?" (come Proton Pass): clicchi nel campo utente o
//     password di un login, Tasso capisce con UI Automation di Windows che campo è e di che sito
//     (indirizzo letto dalla barra del browser, confronto per dominio vero: google.com.truffa.it
//     NON è google.com) e propone solo gli account di quel sito. Vedi vault_popup.go.
//  2. Pulsante "Compila" nell'elenco: Tasso si riduce a icona, torna sull'ultima finestra usata e
//     compila (se il sito non corrisponde, prima chiede in quale finestra).
//  3. Scorciatoia facoltativa (es. Ctrl+Alt+A, spenta di base) con la finestra di login davanti.
//
// Come compila: prima riempie i campi con UI Automation (non dipende dalla tastiera); se il campo
// non lo permette scrive lettera per lettera con SendInput (KEYEVENTF_UNICODE) seguendo la
// sequenza della voce, e prima di ogni tasto controlla che la finestra davanti sia ancora quella:
// se cambia si ferma subito. Mai nelle finestre di Tasso, mai nei giochi, mai senza portachiavi
// aperto. Niente viene scritto su disco o nei log; le copie della password in memoria si azzerano.

import (
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"tasso/autotipo"
	"tasso/traduce"
)

var (
	vtpGetAncestor    = user32.NewProc("GetAncestor")
	vtpGetWindowText  = user32.NewProc("GetWindowTextW")
	vtpGetWindowTextL = user32.NewProc("GetWindowTextLengthW")
	vtpGetClassName   = user32.NewProc("GetClassNameW")
	vtpGetWindow      = user32.NewProc("GetWindow")
	vtpIsWindow       = user32.NewProc("IsWindow")
	vtpIsIconic       = user32.NewProc("IsIconic")
	vtpIsVisible      = user32.NewProc("IsWindowVisible")
	vtpGetWindowLong  = user32.NewProc("GetWindowLongW")
	vtpGetFg          = user32.NewProc("GetForegroundWindow")
	vtpSetFg          = user32.NewProc("SetForegroundWindow")
	vtpBringTop       = user32.NewProc("BringWindowToTop")
	vtpShowAsync      = user32.NewProc("ShowWindowAsync")
	vtpShow           = user32.NewProc("ShowWindow")
	vtpWinPid         = user32.NewProc("GetWindowThreadProcessId")
	vtpAttach         = user32.NewProc("AttachThreadInput")
	vtpSendInput      = user32.NewProc("SendInput")
	vtpMapVK          = user32.NewProc("MapVirtualKeyW")
	vtpAsyncKey       = user32.NewProc("GetAsyncKeyState")
	vtpEnumWindows    = user32.NewProc("EnumWindows")
	vtpCurThread      = kernel32.NewProc("GetCurrentThreadId")
	vtpDwmAttr        = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmGetWindowAttribute")
	vtpClipSeq        = user32.NewProc("GetClipboardSequenceNumber")
	vtEnumCB          = windows.NewCallback(vtEnumProc)
	vtEnumMu          sync.Mutex
	vtEnumOut         []uintptr
	vtLastFg          atomic.Uintptr // l'ultima finestra (non di Tasso) che è stata davanti
	vtReq             = make(chan vtRichiesta, 16)
	vtOnce            sync.Once
	vtTokMu           sync.Mutex
	vtTokens          = map[string]vtTok{}
	vtMetaMu          sync.Mutex
	vtMeta            []autotipo.Voce // siti delle voci (niente password), per "Sblocca" a portachiavi chiuso
	vtMetaOK          bool
	vtUltimoMu        sync.Mutex
	vtUltimo          struct {
		id, chiave string
		quando     time.Time
	} // l'account appena usato: per i login in due passaggi (Google: prima email, poi password)
)

const (
	vrFocus       = iota + 1 // il campo con la tastiera è cambiato
	vrTasto                  // scorciatoia premuta
	vrRiempi                 // clic su un account della finestrella
	vrCompila                // pulsante Compila nell'elenco
	vrCompilaA               // Compila nella finestra scelta dall'elenco
	vrSalva                  // "Salva questo account" nella finestrella
	vrGenera                 // "Usa e salva" la password creata (registrazione)
	vrRiempiPw               // password creata nella finestra Salva: va anche nel campo del sito
	vrRicontrolla            // ricontrolla se sotto il campo è comparsa la tendina di un altro gestore
)

type vtRichiesta struct {
	tipo  int
	id    string
	token string
	pw    []uint16 // vrGenera, vrRiempiPw: chi la riceve la azzera
	top   uintptr
}

type vtTok struct {
	hwnd uintptr
	pid  uint32
	at   time.Time
}

// ---------------- impostazioni ----------------

func vtSuggestOn() bool {
	setMu.Lock()
	defer setMu.Unlock()
	return settings.VtSuggest != 2
}

func vtFirstOK() bool {
	setMu.Lock()
	defer setMu.Unlock()
	return settings.VtFirstOK
}

func vaultFillCfg() map[string]interface{} {
	setMu.Lock()
	defer setMu.Unlock()
	return map[string]interface{}{"suggest": settings.VtSuggest != 2, "key": settings.VtKey, "keyLabel": vtKeyLabel(settings.VtKey), "firstOK": settings.VtFirstOK}
}

func vaultFillSuggest(on bool) string {
	updateSettings(func(s *Settings) {
		s.VtSuggest = 0
		if !on {
			s.VtSuggest = 2
		}
	})
	if !on {
		vtNascondi()
	}
	return "ok"
}

func vaultFillAccept() string {
	updateSettings(func(s *Settings) { s.VtFirstOK = true })
	return "ok"
}

// Codice della scorciatoia: modificatori<<16 | tasto (1 Alt, 2 Ctrl, 4 Maiusc), come in Tasso Traduce.
func vtKeyLabel(c uint32) string {
	if c == 0 {
		return ""
	}
	vk, m := c&0xFFFF, c>>16
	var p []string
	if m&2 != 0 {
		p = append(p, "Ctrl")
	}
	if m&1 != 0 {
		p = append(p, "Alt")
	}
	if m&4 != 0 {
		p = append(p, trGo("Maiusc"))
	}
	if vk >= 'A' && vk <= 'Z' || vk >= '0' && vk <= '9' {
		p = append(p, string(rune(vk)))
	} else {
		p = append(p, "?")
	}
	return strings.Join(p, "+")
}

// vtKeyCheck: niente conflitti con i tasti di Tasso (F6 autoclick, F7-F11 e Ctrl+Alt+I/T/V della
// traduzione, Ctrl+Maiusc+F12 del pannellino) né con AltGr+E (€). Solo lettere o numeri con Ctrl o Alt.
func vtKeyCheck(c uint32) string {
	vk, m := c&0xFFFF, c>>16
	if m&3 == 0 || m > 7 {
		return "La scorciatoia deve avere Ctrl o Alt (per esempio Ctrl+Alt+A)."
	}
	if !(vk >= 'A' && vk <= 'Z' || vk >= '0' && vk <= '9') {
		return "Scegli una lettera o un numero (per esempio Ctrl+Alt+A)."
	}
	if m&3 == 3 && vk == 'E' {
		return "Questa scorciatoia la usa già Tasso (traduzione) o serve per scrivere €: scegline un'altra."
	}
	// i tasti della traduzione si possono cambiare: controllo quelli scelti adesso (tasti_rapidi.go)
	if f := traduce.TastoGiaUsato(c, traduce.TastiTraduzione(), nomeTastoVault); f != "" {
		if f == nomeTastoAc {
			return "Questa scorciatoia la usa già l'autoclick: scegline un'altra."
		}
		if f == nomeTastoOv {
			return "Questa scorciatoia la usa già il pannellino in gioco: scegline un'altra."
		}
		return "Questa scorciatoia la usa già Tasso (traduzione) o serve per scrivere €: scegline un'altra."
	}
	return ""
}

func vaultFillKey(c uint32) string {
	if c != 0 {
		if msg := vtKeyCheck(c); msg != "" {
			return msg
		}
		if !vtFirstOK() {
			return "first"
		}
	}
	setMu.Lock()
	old := settings.VtKey
	setMu.Unlock()
	updateSettings(func(s *Settings) { s.VtKey = c })
	if c != 0 && !vtRegistraTasto() {
		updateSettings(func(s *Settings) { s.VtKey = old })
		vtRegistraTasto()
		return "Questa scorciatoia è già usata da un altro programma: scegline un'altra."
	}
	if c == 0 {
		vtRegistraTasto()
	}
	return "ok"
}

// ---------------- finestre ----------------

func vtPid(h uintptr) uint32 {
	var pid uint32
	vtpWinPid.Call(h, uintptr(unsafe.Pointer(&pid)))
	return pid
}

func vtRoot(h uintptr) uintptr {
	if h == 0 {
		return 0
	}
	r, _, _ := vtpGetAncestor.Call(h, 2) // GA_ROOT
	if r == 0 {
		return h
	}
	return r
}

func vtFg() uintptr {
	h, _, _ := vtpGetFg.Call()
	return vtRoot(h)
}

func vtTitle(h uintptr) string {
	n, _, _ := vtpGetWindowTextL.Call(h)
	if n == 0 || n > 1000 {
		return ""
	}
	b := make([]uint16, n+1)
	vtpGetWindowText.Call(h, uintptr(unsafe.Pointer(&b[0])), n+1)
	return windows.UTF16ToString(b)
}

func vtClass(h uintptr) string {
	b := make([]uint16, 256)
	vtpGetClassName.Call(h, uintptr(unsafe.Pointer(&b[0])), 256)
	return windows.UTF16ToString(b)
}

func vtExe(pid uint32) (string, string) {
	p := processPath(pid)
	return p, strings.ToLower(filepath.Base(p))
}

// vtVietata dice perché in questa finestra non si può compilare ("" = si può).
func vtVietata(h uintptr) string {
	if h == 0 {
		return "Non trovo la finestra (con il desktop protetto di Windows, per esempio la richiesta di permessi da amministratore, Tasso non può scrivere)."
	}
	if r, _, _ := vtpIsWindow.Call(h); r == 0 {
		return "La finestra si è chiusa."
	}
	pid := vtPid(h)
	if pid == windows.GetCurrentProcessId() {
		return "Non compilo dentro Tasso."
	}
	switch vtClass(h) {
	case "Shell_TrayWnd", "Progman", "WorkerW", "Shell_SecondaryTrayWnd":
		return "Questa è la barra di Windows o il desktop, non un login."
	}
	path, exe := vtExe(pid)
	if path == "" {
		return "Non riesco a capire che programma è: per sicurezza non compilo."
	}
	if (exePath != "" && strings.EqualFold(path, exePath)) || strings.HasPrefix(exe, "tasso") {
		return "Non compilo dentro Tasso."
	}
	for _, a := range antiCheatExes {
		if strings.EqualFold(exe, a) {
			return "Dentro i giochi Tasso non compila (con l'anti-cheat potresti essere bannato). Accedi dal launcher del gioco."
		}
	}
	dirs, exes := libraryGamePaths()
	if pid == gamePid.Load() || isGamePath(strings.ToLower(path), exe, dirs, exes) {
		return "Dentro i giochi Tasso non compila (con l'anti-cheat potresti essere bannato). Accedi dal launcher del gioco."
	}
	return ""
}

// vtStessoProgramma: il campo è della finestra davanti? (Nei browser il campo può stare in un
// processo figlio dello stesso browser: conta che sia lo stesso programma.)
func vtStessoProgramma(pidCampo uint32, top uintptr) bool {
	pt := vtPid(top)
	if pidCampo == 0 || pt == 0 {
		return false
	}
	if pidCampo == pt {
		return true
	}
	_, a := vtExe(pidCampo)
	_, b := vtExe(pt)
	return a != "" && a == b && autotipo.Browser[a]
}

// vtAltTab: la finestra è una di quelle "vere" che vedi con Alt+Tab?
func vtAltTab(h uintptr) bool {
	if v, _, _ := vtpIsVisible.Call(h); v == 0 {
		return false
	}
	if o, _, _ := vtpGetWindow.Call(h, 4); o != 0 { // GW_OWNER
		return false
	}
	ex, _, _ := vtpGetWindowLong.Call(h, ^uintptr(19)) // GWL_EXSTYLE
	if ex&0x80 != 0 && ex&0x40000 == 0 {               // TOOLWINDOW senza APPWINDOW
		return false
	}
	var cloaked uint32
	if r, _, _ := vtpDwmAttr.Call(h, 14, uintptr(unsafe.Pointer(&cloaked)), 4); r == 0 && cloaked != 0 {
		return false // finestra nascosta delle app di Windows
	}
	return vtTitle(h) != ""
}

func vtEnumProc(h, _ uintptr) uintptr {
	if len(vtEnumOut) < 200 && vtAltTab(h) {
		vtEnumOut = append(vtEnumOut, h)
	}
	return 1
}

// vtFinestre: le finestre aperte in ordine (la più in alto prima).
func vtFinestre() []uintptr {
	vtEnumMu.Lock()
	defer vtEnumMu.Unlock()
	vtEnumOut = nil
	vtpEnumWindows.Call(vtEnumCB, 0)
	out := vtEnumOut
	vtEnumOut = nil
	return out
}

// vtUltima: l'ultima finestra usata prima di Tasso.
func vtUltima() uintptr {
	if h := vtLastFg.Load(); h != 0 {
		if r, _, _ := vtpIsWindow.Call(h); r != 0 && vtAltTab(h) {
			return h
		}
	}
	for _, h := range vtFinestre() {
		ex, _, _ := vtpGetWindowLong.Call(h, ^uintptr(19))
		if ex&0x8 == 0 && vtPid(h) != windows.GetCurrentProcessId() { // non "sempre in primo piano"
			return h
		}
	}
	return 0
}

// vtSegnaFg: chiamata quando cambia la finestra davanti (vault_popup.go).
func vtSegnaFg(h uintptr) {
	h = vtRoot(h)
	if h == 0 || vtPid(h) == windows.GetCurrentProcessId() || !vtAltTab(h) {
		return
	}
	switch vtClass(h) {
	case "Shell_TrayWnd", "Progman", "WorkerW", "Shell_SecondaryTrayWnd", "ForegroundStaging", "MultitaskingViewFrame", "XamlExplorerHostIslandWindow":
		return
	}
	vtLastFg.Store(h)
}

// vtContesto: sito (nei browser) o programma della finestra.
func vtContesto(u *uia, h uintptr) autotipo.Contesto {
	_, exe := vtExe(vtPid(h))
	c := autotipo.Contesto{Exe: exe, Title: vtTitle(h)}
	if autotipo.Browser[exe] && u != nil {
		c.Host = autotipo.NormHost(u.browserURL(h))
	}
	return c
}

func vtChiave(c autotipo.Contesto) string {
	if c.Host != "" {
		return "web:" + autotipo.Registrabile(c.Host)
	}
	return "exe:" + c.Exe
}

func vtEtichetta(c autotipo.Contesto) string {
	if c.Host != "" {
		return c.Host
	}
	if c.Title != "" {
		return c.Title
	}
	return c.Exe
}

// ---------------- voci del portachiavi ----------------

type vtVoce struct {
	ID, Site, User string
}

// vtAperto: il portachiavi è aperto? Non fa ripartire i 5 minuti (guardare non è usare).
func vtAperto() bool {
	return vaultKey != nil && time.Since(vaultTouched) <= vaultIdle
}

func vtMetaDa(items []vaultEntry) {
	m := make([]autotipo.Voce, 0, len(items))
	for _, e := range items {
		m = append(m, autotipo.Voce{Site: e.Site, Web: e.Web, Win: e.Win})
	}
	vtMetaMu.Lock()
	vtMeta, vtMetaOK = m, true
	vtMetaMu.Unlock()
	has := len(items) > 0
	setMu.Lock()
	cambia := settings.VtHasItems != has
	setMu.Unlock()
	if cambia {
		go updateSettings(func(s *Settings) { s.VtHasItems = has })
	}
}

// vtVoci: gli account che vanno bene per il contesto. locked = portachiavi chiuso.
func vtVoci(c autotipo.Contesto) (out []vtVoce, locked bool) {
	vaultMu.Lock()
	if !vtAperto() && vaultKey == nil && vaultExists() && vaultNoPin() {
		vaultMu.Unlock()
		vaultUnlock("") // portachiavi senza PIN: si apre da solo, come nella pagina
		vaultMu.Lock()
	}
	defer vaultMu.Unlock()
	if !vtAperto() {
		return nil, true
	}
	vtMetaDa(vaultItems)
	for _, e := range vaultItems {
		if autotipo.Corrisponde(autotipo.Voce{Site: e.Site, Web: e.Web, Win: e.Win}, c) {
			out = append(out, vtVoce{ID: e.ID, Site: e.Site, User: e.User})
		}
	}
	return out, false
}

// vtCeneSono: a portachiavi chiuso, c'è (per quello che ricordo dall'ultima apertura) un account per il contesto?
func vtCeneSono(c autotipo.Contesto) (sicuro bool, forse bool) {
	vtMetaMu.Lock()
	defer vtMetaMu.Unlock()
	if !vtMetaOK {
		setMu.Lock()
		has := settings.VtHasItems
		setMu.Unlock()
		return false, has
	}
	for _, v := range vtMeta {
		if autotipo.Corrisponde(v, c) {
			return true, true
		}
	}
	return false, false
}

// vtSegreto: copia di utente e password per il tempo della compilazione, poi azzerata.
type vtSegreto struct {
	user, pw []uint16 // con lo zero finale
	passi    []autotipo.Passo
	tasti    bool
	site     string
}

func (s *vtSegreto) cancella() {
	if s == nil {
		return
	}
	clear(s.user)
	clear(s.pw)
}

func vtU16(s string) []uint16 {
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return []uint16{0}
	}
	return u
}

func vtPrendi(id string) (*vtSegreto, string) {
	vaultMu.Lock()
	defer vaultMu.Unlock()
	if vaultLocked() { // qui sì: compilare è usare il portachiavi
		return nil, "locked"
	}
	i := vaultFind(id)
	if i < 0 {
		return nil, "Voce non trovata."
	}
	e := vaultItems[i]
	passi, err := autotipo.LeggiSequenza(e.Seq)
	if err != nil {
		passi, _ = autotipo.LeggiSequenza("")
	}
	if e.User == "" {
		passi = autotipo.SenzaUtente(passi)
	}
	return &vtSegreto{user: vtU16(e.User), pw: vtU16(e.Pw), passi: passi, tasti: e.Keys, site: e.Site}, ""
}

// ---------------- tastiera ----------------

type vtKbd struct {
	typ   uint32
	_     uint32
	vk    uint16
	scan  uint16
	flags uint32
	time  uint32
	_     uint32
	extra uintptr
	_     [8]byte
}

func vtSend(in []vtKbd) bool {
	if len(in) == 0 {
		return true
	}
	r, _, _ := vtpSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
	return int(r) == len(in)
}

func vtTastoVK(vk uint16) bool {
	sc, _, _ := vtpMapVK.Call(uintptr(vk), 0)
	return vtSend([]vtKbd{{typ: 1, vk: vk, scan: uint16(sc)}, {typ: 1, vk: vk, scan: uint16(sc), flags: 2}})
}

func vtUnit(u uint16) bool {
	return vtSend([]vtKbd{{typ: 1, scan: u, flags: 4}, {typ: 1, scan: u, flags: 4 | 2}}) // UNICODE, poi KEYUP
}

func vtPremuto(vk uintptr) bool {
	r, _, _ := vtpAsyncKey.Call(vk)
	return r&0x8000 != 0
}

// vtAspettaModificatori: con Ctrl o Alt ancora premuti (scorciatoia) le lettere diventerebbero comandi.
func vtAspettaModificatori() bool {
	for i := 0; i < 60; i++ {
		if !vtPremuto(0x10) && !vtPremuto(0x11) && !vtPremuto(0x12) && !vtPremuto(0x5B) && !vtPremuto(0x5C) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

var errVtCambiata = "Mi sono fermato: la finestra davanti è cambiata. Non ho scritto niente altrove."

// vtScrivi esegue la sequenza nella finestra target, controllando prima di ogni tasto.
func vtScrivi(target uintptr, passi []autotipo.Passo, s *vtSegreto) string {
	if !vtAspettaModificatori() {
		return "Lascia i tasti Ctrl, Alt, Maiusc e Windows e riprova."
	}
	vtPremuto(0x1B) // azzera lo stato "premuto da prima" di Esc
	ok := func() string {
		if vtFg() != target {
			return errVtCambiata
		}
		if vtPremuto(0x1B) {
			return "Fermato con Esc."
		}
		if vtPremuto(0x11) || vtPremuto(0x12) || vtPremuto(0x5B) {
			return "Mi sono fermato perché hai premuto Ctrl, Alt o il tasto Windows."
		}
		return ""
	}
	unita := func(u []uint16) string {
		for _, c := range u {
			if c == 0 {
				break
			}
			if m := ok(); m != "" {
				return m
			}
			if !vtUnit(c) {
				return "Windows non mi lascia scrivere in questa finestra."
			}
			time.Sleep(12 * time.Millisecond)
		}
		return ""
	}
	for _, p := range passi {
		var m string
		switch p.Tipo {
		case autotipo.Utente:
			m = unita(s.user)
		case autotipo.Password:
			m = unita(s.pw)
		case autotipo.Testo:
			t := vtU16(p.Testo)
			m = unita(t)
		case autotipo.Tab, autotipo.Invio:
			if m = ok(); m == "" {
				vk := uint16(0x09)
				if p.Tipo == autotipo.Invio {
					vk = 0x0D
				}
				if !vtTastoVK(vk) {
					m = "Windows non mi lascia scrivere in questa finestra."
				}
				time.Sleep(60 * time.Millisecond)
			}
		case autotipo.Attesa:
			fine := time.Now().Add(p.Durata)
			for time.Now().Before(fine) && m == "" {
				m = ok()
				time.Sleep(50 * time.Millisecond)
			}
		}
		if m != "" {
			return m
		}
	}
	return ""
}

// vtPortaDavanti porta davanti la finestra e aspetta che ci resti davvero.
func vtPortaDavanti(h uintptr) bool {
	if ic, _, _ := vtpIsIconic.Call(h); ic != 0 {
		vtpShow.Call(h, 9) // SW_RESTORE
	}
	for i := 0; i < 40; i++ {
		if vtFg() == h {
			time.Sleep(180 * time.Millisecond)
			if vtFg() == h {
				return true
			}
			continue
		}
		if i%4 == 0 {
			vtpSetFg.Call(h)
		}
		if i == 8 || i == 20 {
			// Ultima risorsa: aggancio la coda dei tasti a quella della finestra davanti.
			fg, _, _ := vtpGetFg.Call()
			var p uint32
			ft, _, _ := vtpWinPid.Call(fg, uintptr(unsafe.Pointer(&p)))
			ct, _, _ := vtpCurThread.Call()
			if ft != 0 && ft != ct {
				vtpAttach.Call(ct, ft, 1)
				vtpBringTop.Call(h)
				vtpSetFg.Call(h)
				vtpAttach.Call(ct, ft, 0)
			}
		}
		time.Sleep(60 * time.Millisecond)
	}
	return false
}

func vtTassoHwnd() uintptr {
	if wv == nil {
		return 0
	}
	return uintptr(wv.Window())
}

// vtMostraTasso riapre Tasso sul portachiavi (per sbloccarlo o scegliere un altro account).
func vtMostraTasso(evento string, dati map[string]interface{}) {
	trayApri()
	emitRaw(evento, dati)
}

// ---------------- compilazione ----------------

// vtRiempi compila partendo dal campo el (che ha la tastiera) nella finestra top.
// Prima prova a riempire i campi con UI Automation; se non si può, scrive i tasti.
func vtRiempi(u *uia, el comObj, top uintptr, s *vtSegreto) (string, bool) {
	isPw := el != 0 && elIsPassword(el)
	invio := autotipo.HaInvio(s.passi)
	controlla := func() string {
		if vtFg() != top {
			return errVtCambiata
		}
		if el != 0 && !elHasFocus(el) {
			return "Mi sono fermato: il campo non è più quello di prima."
		}
		return ""
	}
	if m := controlla(); m != "" {
		return m, false
	}
	soloUtente := false
	if el != 0 && !s.tasti && !elReadOnly(el) {
		fatto := false
		if isPw {
			fatto = len(s.pw) > 1 && elSetValue(el, s.pw)
		} else if len(s.user) > 1 && elSetValue(el, s.user) && elValue(el) == windows.UTF16ToString(s.user) {
			fatto = true
			root := u.fromHandle(top)
			pw := u.findPassword(root)
			root.release()
			if pw != 0 && len(s.pw) > 1 && !elReadOnly(pw) {
				if !elSetValue(pw, s.pw) {
					// il campo password non accetta: lo scrivo a tasti partendo da lì
					pw.call(3) // SetFocus
					time.Sleep(120 * time.Millisecond)
					if m := vtScrivi(top, autotipo.DalPassword(s.passi), s); m != "" {
						pw.release()
						return m, false
					}
					pw.release()
					return "", false
				}
				pw.release()
			} else {
				if pw != 0 {
					pw.release()
				}
				soloUtente = true // login in due passaggi: la password la chiedo nella pagina dopo
			}
		}
		if fatto {
			if invio {
				if !vtAspettaModificatori() {
					return "", soloUtente
				}
				if m := controlla(); m != "" {
					return m, soloUtente
				}
				time.Sleep(80 * time.Millisecond)
				vtTastoVK(0x0D)
			}
			return "", soloUtente
		}
	}
	// A tasti: dal campo password solo la parte da {PASSWORD} in poi.
	passi := s.passi
	if isPw {
		passi = autotipo.DalPassword(passi)
	}
	return vtScrivi(top, passi, s), false
}

func vtFatto(msg string) {
	if msg == "" {
		return
	}
	emit("onVaultFill", map[string]interface{}{"msg": msg})
	trayAvviso("Portachiavi", msg, false)
}

// ---------------- filo UI Automation ----------------

type vtStato struct {
	u      *uia
	el     comObj // campo con la tastiera
	top    uintptr
	ctx    autotipo.Contesto
	isPw   bool
	reg    bool // campo password di una registrazione
	nPw    int  // campi password nella finestra
	valido bool
}

func (st *vtStato) lascia() {
	st.el.release()
	st.el, st.top, st.valido = 0, 0, false
}

func vtLavoro() {
	defer guard("vault_compila.go:vtLavoro")
	runtime.LockOSThread()
	u, ok := newUIA()
	if !ok {
		return
	}
	st := &vtStato{u: u}
	for r := range vtReq {
		if r.tipo == vrFocus {
			// aspetto un attimo: un clic fa arrivare più cambi di fuoco di fila
			t := time.NewTimer(90 * time.Millisecond)
		attesa:
			for {
				select {
				case r2 := <-vtReq:
					if r2.tipo != vrFocus {
						st.focus()
						st.fai(r2)
						t.Stop()
						break attesa
					}
				case <-t.C:
					st.focus()
					break attesa
				}
			}
			continue
		}
		st.fai(r)
	}
}

func (st *vtStato) fai(r vtRichiesta) {
	defer guard("vault_compila.go:fai")
	switch r.tipo {
	case vrTasto:
		st.tasto()
	case vrRiempi:
		st.riempi(r.id)
	case vrCompila:
		st.compila(r.id, 0)
	case vrSalva:
		st.salva()
	case vrGenera:
		st.genera(r.pw)
	case vrRiempiPw:
		st.riempiPw(r.pw, r.top)
	case vrRicontrolla:
		st.ricontrolla()
	case vrCompilaA:
		vtTokMu.Lock()
		t, ok := vtTokens[r.token]
		vtTokMu.Unlock()
		if !ok || time.Since(t.at) > 3*time.Minute || vtPid(t.hwnd) != t.pid {
			emit("onVaultFill", map[string]interface{}{"msg": "La finestra scelta non c'è più: riprova."})
			return
		}
		st.compila(r.id, t.hwnd)
	}
}

// focus: il campo con la tastiera è cambiato. Decide se mostrare la finestrella.
func (st *vtStato) focus() {
	defer guard("vault_compila.go:focus")
	if !vtSuggestOn() || !vaultExists() {
		vtNascondi()
		return
	}
	e := st.u.focused()
	if e == 0 {
		return
	}
	if st.u.same(e, st.el) {
		e.release() // stesso campo: se l'avevi chiusa con Esc resta chiusa
		return
	}
	st.lascia()
	vtNascondi()
	st.el = e
	if elType(e) != uiaControlEdit || !elEnabled(e) || elOffscreen(e) {
		return
	}
	top := vtFg()
	if top == 0 || !vtStessoProgramma(elPid(e), top) || vtVietata(top) != "" {
		return
	}
	ctx := vtContesto(nil, top)
	if autotipo.Browser[ctx.Exe] {
		if !st.u.insideDocument(e) {
			return // barra degli indirizzi o della ricerca del browser
		}
		ctx.Host = autotipo.NormHost(st.u.browserURL(top))
		if ctx.Host == "" {
			return
		}
	}
	isPw := elIsPassword(e)
	if !isPw && !autotipo.CampoAccesso(elName(e), elAutoID(e)) {
		root := st.u.fromHandle(top)
		pw := st.u.findPassword(root)
		root.release()
		if pw == 0 {
			return
		}
		pw.release()
	}
	if elReadOnly(e) {
		return
	}
	st.reg, st.nPw = false, 0
	if isPw {
		campi := st.u.passwords(top)
		st.nPw = len(campi)
		releaseAll(campi)
		st.reg = autotipo.CampoRegistrazione(elName(e), elAutoID(e), vtTitle(top), st.nPw)
	}
	st.top, st.ctx, st.isPw, st.valido = top, ctx, isPw, true
	r, _ := elRect(e)
	st.mostra(r, false)
}

// mostra prepara la finestrella per il campo attuale. forza: anche senza account (scorciatoia).
func (st *vtStato) mostra(r rect, forza bool) {
	voci, locked := vtVoci(st.ctx)
	if locked {
		sicuro, forse := vtCeneSono(st.ctx)
		if sicuro || (forse && st.isPw) || forza {
			vtMostra(vtPopDati{modo: vpChiuso, anchor: r, top: st.top, ctx: vtEtichetta(st.ctx)})
		}
		return
	}
	base := vtPopDati{anchor: r, top: st.top, ctx: vtEtichetta(st.ctx), web: st.ctx.Host != "", isPw: st.isPw}
	if st.el != 0 {
		base.sopra = st.u.occupatoSotto(r, elPid(st.el))
		if !st.isPw {
			if u := strings.TrimSpace(elValue(st.el)); len(u) <= 120 {
				base.utente = u
			}
		}
	}
	defer st.ricontrollaDopo(base.sopra)
	// pagina per creare un account: "Crea password sicura e salvala" (se ci sono già account del
	// sito e un solo campo password, è più probabile un accesso: propongo gli account)
	if st.isPw && st.reg && (st.nPw >= 2 || len(voci) == 0) {
		base.modo, base.simboli = vpGenera, true
		base.gen = autotipo.GeneraPassword(20, true)
		vtMostra(base)
		return
	}
	// login in due passaggi: l'account appena usato in questo sito va per primo
	vtUltimoMu.Lock()
	uid, uk, uq := vtUltimo.id, vtUltimo.chiave, vtUltimo.quando
	vtUltimoMu.Unlock()
	hint := ""
	if st.isPw && uk == vtChiave(st.ctx) && time.Since(uq) < 3*time.Minute {
		for i, v := range voci {
			if v.ID == uid {
				voci[0], voci[i] = voci[i], voci[0]
				hint = uid
			}
		}
	}
	base.modo, base.voci, base.hint, base.first = vpElenco, voci, hint, !vtFirstOK()
	if len(voci) == 0 {
		base.modo = vpNessuno // "Nessun account salvato qui" + "Salva questo account"
	}
	_ = forza
	vtMostra(base)
}

// ricontrollaDopo: le tendine delle estensioni compaiono un attimo dopo il clic: ricontrollo.
func (st *vtStato) ricontrollaDopo(giaSopra bool) {
	if giaSopra {
		return
	}
	go func() {
		defer guard("vault_compila.go:ricontrollaDopo")
		time.Sleep(450 * time.Millisecond)
		vtInvia(vtRichiesta{tipo: vrRicontrolla})
	}()
}

func (st *vtStato) ricontrolla() {
	vtPopMu.Lock()
	vis := vtPop.modo != 0 && !vtPop.sopra && vtPop.top == st.top
	vtPopMu.Unlock()
	if !vis || !st.valido || st.el == 0 {
		return
	}
	if r, ok := elRect(st.el); ok && st.u.occupatoSotto(r, elPid(st.el)) {
		vtSpostaSopra()
	}
}

// stessoSito: la pagina davanti è ancora dello stesso sito di quando è comparsa la finestrella?
func (st *vtStato) stessoSito(top uintptr) bool {
	if top != st.top || vtFg() != top {
		return false
	}
	if st.ctx.Host == "" {
		return !autotipo.Browser[vtContesto(nil, top).Exe]
	}
	return autotipo.StessoSito(autotipo.NormHost(st.u.browserURL(top)), st.ctx.Host)
}

// salva: "Salva questo account". Legge il sito (barra del browser) e l'utente scritto nella pagina
// (le password Windows non le dà) e apre la finestra di salvataggio.
func (st *vtStato) salva() {
	if !st.valido || st.top == 0 {
		return
	}
	if m := vtVietata(st.top); m != "" {
		vtFatto(m)
		return
	}
	if !vtAperto() {
		vtMostraTasso("onVaultShow", map[string]interface{}{})
		return
	}
	utente := ""
	if st.el != 0 && !st.isPw {
		utente = strings.TrimSpace(elValue(st.el))
	}
	if utente == "" {
		utente = st.u.findUser(st.top)
	}
	d := svDati{utente: utente, top: st.top, web: st.ctx.Host != ""}
	if d.web {
		d.sito, d.nome = st.ctx.Host, autotipo.NomeDaSito(st.ctx.Host)
	} else {
		d.sito, d.nome = st.ctx.Exe, autotipo.TogliBrowser(st.ctx.Title)
		if d.nome == "" {
			d.nome = strings.TrimSuffix(st.ctx.Exe, ".exe")
		}
	}
	d.anchor, _ = elRect(st.el)
	if d.anchor.Right == 0 {
		pGetWindowRect.Call(st.top, uintptr(unsafe.Pointer(&d.anchor)))
	}
	svPrepara(d)
}

// scriviPw mette la password in tutti i campi password della pagina (password e "conferma"),
// tranne "password attuale". Torna quanti campi ha riempito.
func (st *vtStato) scriviPw(top uintptr, pw []uint16) int {
	campi := st.u.passwords(top)
	defer releaseAll(campi)
	n := 0
	for _, e := range campi {
		if elReadOnly(e) || !elEnabled(e) || elOffscreen(e) || !vtStessoProgramma(elPid(e), top) {
			continue
		}
		if st.ctx.Host != "" && !st.u.insideDocument(e) {
			continue
		}
		if !autotipo.CampoRegistrazione(elName(e), elAutoID(e), "", 2) {
			continue // "password attuale"
		}
		if elSetValue(e, pw) {
			n++
		}
	}
	return n
}

// genera: "Usa e salva" la password creata nella finestrella su una pagina di registrazione.
func (st *vtStato) genera(pw []uint16) {
	defer clear(pw)
	if !st.valido || st.top == 0 || len(pw) < 2 || pw[0] == 0 {
		return // pw[0] == 0: password vuota o già azzerata, non la scrivo e non la salvo
	}
	if m := vtVietata(st.top); m != "" {
		vtFatto(m)
		return
	}
	if !vtAperto() {
		vtMostraTasso("onVaultShow", map[string]interface{}{})
		return
	}
	if !st.stessoSito(st.top) {
		vtFatto(errVtCambiata)
		return
	}
	if st.scriviPw(st.top, pw) == 0 {
		// il campo non accetta: lo scrivo a tasti nel campo che ha la tastiera
		if st.el == 0 || !elHasFocus(st.el) {
			vtFatto("Non riesco a scrivere nel campo password: la password non è stata salvata.")
			return
		}
		s := &vtSegreto{pw: pw, passi: []autotipo.Passo{{Tipo: autotipo.Password}}}
		if m := vtScrivi(st.top, s.passi, s); m != "" {
			vtFatto(m)
			return
		}
	}
	nome := autotipo.NomeDaSito(st.ctx.Host)
	web := st.ctx.Host
	if web == "" {
		nome, web = autotipo.TogliBrowser(st.ctx.Title), st.ctx.Exe
		if nome == "" {
			nome = strings.TrimSuffix(st.ctx.Exe, ".exe")
		}
	}
	e := vaultEntry{Site: nome, User: st.u.findUser(st.top), Pw: windows.UTF16ToString(pw), Web: web}
	r := vaultSave(e)
	e.Pw = ""
	switch r {
	case "ok":
		vtFatto(trGo("Password sicura creata, scritta e salvata nel portachiavi:") + " " + nome)
	case "locked":
		vtFatto("Il portachiavi si è chiuso: la password è scritta nella pagina ma non è stata salvata.")
	default:
		vtFatto(r)
	}
}

// riempiPw: la password creata nella finestra Salva va anche nei campi password del sito.
func (st *vtStato) riempiPw(pw []uint16, top uintptr) {
	defer clear(pw)
	if !st.valido || top == 0 || top != st.top || len(pw) < 2 || pw[0] == 0 || vtVietata(top) != "" || !vtAperto() {
		return
	}
	if !vtPortaDavanti(top) || !st.stessoSito(top) {
		vtFatto("La password è salvata ma non l'ho scritta nella pagina: la finestra è cambiata.")
		return
	}
	st.scriviPw(top, pw)
}

// riempi: clic su un account nella finestrella.
func (st *vtStato) riempi(id string) {
	if !st.valido || st.el == 0 {
		return
	}
	s, msg := vtPrendi(id)
	if msg == "locked" {
		vtMostraTasso("onVaultShow", map[string]interface{}{})
		return
	}
	if s == nil {
		vtFatto(msg)
		return
	}
	defer s.cancella()
	if m := vtVietata(st.top); m != "" {
		vtFatto(m)
		return
	}
	if !vtFirstOK() {
		vaultFillAccept() // la finestrella la prima volta mostra l'avviso: cliccare l'account è accettarlo
	}
	m, soloUtente := vtRiempi(st.u, st.el, st.top, s)
	vtFatto(m)
	if m == "" {
		vtUltimoMu.Lock()
		vtUltimo.id, vtUltimo.chiave, vtUltimo.quando = "", "", time.Time{}
		if soloUtente {
			vtUltimo.id, vtUltimo.chiave, vtUltimo.quando = id, vtChiave(st.ctx), time.Now()
		}
		vtUltimoMu.Unlock()
		// il campo resta lo stesso: se dopo il cambio pagina arriva il campo password, la finestrella torna
	}
}

// tasto: scorciatoia premuta con la finestra di login davanti.
func (st *vtStato) tasto() {
	top := vtFg()
	if m := vtVietata(top); m != "" {
		vtFatto(m)
		return
	}
	e := st.u.focused()
	st.lascia()
	st.el = e
	ctx := vtContesto(st.u, top)
	st.top, st.ctx, st.valido = top, ctx, true
	st.isPw = e != 0 && elIsPassword(e)
	if e != 0 && (elType(e) != uiaControlEdit || !vtStessoProgramma(elPid(e), top)) {
		e.release()
		st.el = 0
	}
	r, ok := elRect(st.el)
	if !ok {
		var wr rect
		pGetWindowRect.Call(top, uintptr(unsafe.Pointer(&wr)))
		c := (wr.Left + wr.Right) / 2
		r = rect{c - 150, wr.Top + 120, c + 150, wr.Top + 150}
	}
	voci, locked := vtVoci(ctx)
	if !locked && len(voci) == 1 {
		s, msg := vtPrendi(voci[0].ID)
		if s == nil {
			vtFatto(msg)
			return
		}
		defer s.cancella()
		m, _ := vtRiempi(st.u, st.el, top, s)
		vtFatto(m)
		return
	}
	st.mostra(r, true)
}

// compila: pulsante "Compila" nell'elenco (target 0 = l'ultima finestra usata, se è del sito giusto).
func (st *vtStato) compila(id string, target uintptr) {
	if !vtFirstOK() {
		emit("onVaultFill", map[string]interface{}{"first": true, "id": id})
		return
	}
	vaultMu.Lock()
	i := vaultFind(id)
	var voce autotipo.Voce
	if i >= 0 && !vaultLocked() {
		voce = autotipo.Voce{Site: vaultItems[i].Site, Web: vaultItems[i].Web, Win: vaultItems[i].Win}
	} else {
		i = -1
	}
	vaultMu.Unlock()
	if i < 0 {
		emit("onVaultFill", map[string]interface{}{"msg": "Il portachiavi si è chiuso: riaprilo."})
		return
	}
	if target == 0 {
		if h := vtUltima(); h != 0 && vtVietata(h) == "" && autotipo.Corrisponde(voce, vtContesto(st.u, h)) {
			target = h
		} else {
			st.chiedi(id, voce, h)
			return
		}
	}
	if m := vtVietata(target); m != "" {
		emit("onVaultFill", map[string]interface{}{"msg": m})
		return
	}
	s, msg := vtPrendi(id)
	if s == nil {
		emit("onVaultFill", map[string]interface{}{"msg": msg})
		return
	}
	defer s.cancella()
	tasso := vtTassoHwnd()
	vtpSetFg.Call(target)
	if tasso != 0 {
		vtpShowAsync.Call(tasso, 7) // SW_SHOWMINNOACTIVE: Tasso si fa da parte
	}
	if !vtPortaDavanti(target) {
		vtFatto("Non riesco a portare davanti la finestra: cliccaci sopra e usa la finestrella o la scorciatoia.")
		return
	}
	if m := vtVietata(target); m != "" {
		vtFatto(m)
		return
	}
	e := st.u.focused()
	st.lascia()
	if e != 0 && (elType(e) != uiaControlEdit || !vtStessoProgramma(elPid(e), target)) {
		e.release()
		e = 0
	}
	st.el, st.top, st.ctx = e, target, vtContesto(nil, target)
	m, _ := vtRiempi(st.u, e, target, s)
	vtFatto(m)
}

// chiedi: elenco delle finestre aperte per scegliere dove compilare.
func (st *vtStato) chiedi(id string, voce autotipo.Voce, ultima uintptr) {
	vtTokMu.Lock()
	vtTokens = map[string]vtTok{}
	vtTokMu.Unlock()
	var list []map[string]interface{}
	add := func(h uintptr, last bool) {
		if len(list) >= 15 || vtVietata(h) != "" {
			return
		}
		c := vtContesto(st.u, h)
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		tok := hex.EncodeToString(b)
		vtTokMu.Lock()
		vtTokens[tok] = vtTok{hwnd: h, pid: vtPid(h), at: time.Now()}
		vtTokMu.Unlock()
		list = append(list, map[string]interface{}{"id": tok, "site": c.Title, "exe": c.Exe, "host": c.Host, "last": last, "match": autotipo.Corrisponde(voce, c)})
	}
	if ultima != 0 {
		add(ultima, true)
	}
	for _, h := range vtFinestre() {
		if h != ultima {
			add(h, false)
		}
	}
	emitRaw("onVaultFill", map[string]interface{}{"ask": true, "id": id, "windows": list})
}

// ---------------- avvio e collegamenti con la pagina ----------------

func vtInvia(r vtRichiesta) {
	select {
	case vtReq <- r:
	default: // coda piena: un evento di fuoco in meno non fa danni
	}
}

func startVaultFill() {
	vtOnce.Do(func() {
		go vtLavoro()
		go vtFilo()
	})
}

func bindVaultFill(w webview2.WebView) {
	startVaultFill()
	_ = w.Bind("goVaultFillCfg", func() map[string]interface{} { return vaultFillCfg() })
	_ = w.Bind("goVaultFillSuggest", func(on bool) string { return vaultFillSuggest(on) })
	_ = w.Bind("goVaultFillAccept", func() string { return vaultFillAccept() })
	_ = w.Bind("goVaultFillKey", func(c uint32) string { return vaultFillKey(c) })
	_ = w.Bind("goVaultFill", func(id string) string {
		vtInvia(vtRichiesta{tipo: vrCompila, id: id})
		return "ok"
	})
	_ = w.Bind("goVaultFillTo", func(id, token string) string {
		vtInvia(vtRichiesta{tipo: vrCompilaA, id: id, token: token})
		return "ok"
	})
}

// copySecretFor: come copySecret, ma si cancella dopo d (se negli appunti c'è ancora quel testo).
func copySecretFor(txt string, d time.Duration) string {
	if txt == "" || len(txt) > 4096 {
		return "Vuoto."
	}
	seq, ok := setClipboardSecret(txt)
	if !ok {
		return "Non riesco a usare gli appunti."
	}
	go func() {
		defer guard("vault_compila.go:copySecretFor")
		time.Sleep(d)
		// Il numero degli appunti cambia a ogni copia: se è ancora quello, c'è ancora il nostro testo.
		if r, _, _ := vtpClipSeq.Call(); uint32(r) == seq {
			clearClipboard()
		}
	}()
	return "ok"
}
