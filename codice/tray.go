package main

// Icona di Tasso nell'area di notifica (in basso a destra, vicino a Wi-Fi e volume, anche sotto la
// freccetta). Con l'icona Tasso resta acceso in background:
//   - clic (o doppio clic) sull'icona: riapre la finestra e la porta davanti;
//   - clic destro: Apri Tasso, Boost / Rimetti com'era, Traduzione dei giochi, Modalità, Chiudi Tasso
//     (in Ufficio niente Boost e niente traduzione);
//   - la X della finestra la nasconde qui (Impostazioni → "Resta acceso vicino all'orologio");
//     la prima volta un avviso dall'icona spiega come chiuderlo davvero. Ridurre a icona resta normale.
//
// Tutto il lavoro con l'icona gira su un solo filo (LockOSThread) con una sua finestra nascosta e il
// suo ciclo dei messaggi. La finestra è una finestra vera mai mostrata e non "message-only": solo
// le finestre vere ricevono "TaskbarCreated", che Windows manda quando Esplora risorse si riavvia
// (l'icona va rimessa). Nessuna chiamata alla WebView fuori da wv.Dispatch.

import (
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf16"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"tasso/traduce"
)

var (
	shell32              = windows.NewLazySystemDLL("shell32.dll")
	pShellNotifyIcon     = shell32.NewProc("Shell_NotifyIconW")
	pRegisterWindowMsg   = user32.NewProc("RegisterWindowMessageW")
	pChangeWndMsgFilter  = user32.NewProc("ChangeWindowMessageFilterEx")
	pCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	pAppendMenu          = user32.NewProc("AppendMenuW")
	pTrackPopupMenuEx    = user32.NewProc("TrackPopupMenuEx")
	pDestroyMenu         = user32.NewProc("DestroyMenu")
	pSetMenuDefaultItem  = user32.NewProc("SetMenuDefaultItem")
	pEndMenu             = user32.NewProc("EndMenu")
	pLoadImage           = user32.NewProc("LoadImageW")
	pLoadIcon            = user32.NewProc("LoadIconW")
	pDestroyWindowTray   = user32.NewProc("DestroyWindow")
	pPostQuitMessageTray = user32.NewProc("PostQuitMessage")
	pKillTimer           = user32.NewProc("KillTimer")
)

// NOTIFYICONDATAW (Vista e successivi): 976 byte a 64 bit.
type notifyIconData struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     uintptr
}

const (
	trayID        = 1
	wmTrayIcona   = 0x8000 + 41 // WM_APP+41: messaggi dell'icona (clic, menu)
	wmTrayFerma   = 0x8000 + 42 // togli l'icona e chiudi il filo
	wmTrayAvviso  = 0x8000 + 43 // mostra l'avviso preparato in trayAvvisoDa
	trayTimerAdd  = 1           // riprova a mettere l'icona (Esplora risorse non ancora pronto)
	nimAdd        = 0
	nimModify     = 1
	nimDelete     = 2
	nimSetVersion = 4
	nifMessage    = 0x01
	nifIcon       = 0x02
	nifTip        = 0x04
	nifInfo       = 0x10
	nifShowTip    = 0x80
	niifUser      = 0x04
	niifLargeIcon = 0x20
	niifQuiet     = 0x80 // NIIF_RESPECT_QUIET_TIME
	ninSelect     = 0x400
	ninKeySelect  = 0x401
	ninBalloonClk = 0x405
	wmContextMenu = 0x007B
	wmLButtonDbl  = 0x0203
	wmTimerMsg    = 0x0113
	wmDestroyMsg  = 0x0002

	// voci del menu
	tmApri       = 1
	tmBoost      = 2
	tmRimetti    = 3
	tmTraduzione = 4
	tmChiudi     = 9
	tmModo       = 20 // + indice in trayModi
)

var trayModi = []struct{ id, nome string }{
	{"normale", "Normale"}, {"gaming", "Gaming"}, {"fps", "FPS competitivo"}, {"film", "Film"}, {"silenzioso", "Silenzioso"},
}

var (
	trayOnce       sync.Once
	trayHwnd       atomic.Uintptr
	trayIcona      atomic.Bool // l'icona è davvero nell'area di notifica (solo allora la X nasconde)
	trayFermata    atomic.Bool // trayStop chiamata: niente più icona
	trayFinito     = make(chan struct{})
	trayMsgBarra   uint32 // "TaskbarCreated"
	trayIco        uintptr
	trayIcoBig     uintptr
	trayIcoProprie bool // trayIco/trayIcoBig vengono da LoadImage (non condivise)
	trayAvvMu      sync.Mutex
	trayAvvTit     string
	trayAvvTxt     string
	trayAvvQuiet   bool
)

// startTray mette l'icona (da chiamare una volta, dopo aver creato la finestra di Tasso).
func startTray() {
	trayOnce.Do(func() {
		webview2.ChiusuraHook = trayChiusuraX
		go trayFilo()
	})
}

// trayStop toglie l'icona: chiusura vera, aggiornamento, Togli Tasso. Si può chiamare più volte.
func trayStop() {
	if trayFermata.Swap(true) {
		return
	}
	trayIcona.Store(false)
	h := trayHwnd.Load()
	if h == 0 {
		return
	}
	pPostMessage.Call(h, wmTrayFerma, 0, 0)
	select {
	case <-trayFinito:
	case <-time.After(2 * time.Second):
		// il filo dell'icona non risponde: la tolgo da qui (Shell_NotifyIcon va bene da ogni filo)
		nid := notifyIconData{HWnd: h, UID: trayID}
		nid.CbSize = uint32(unsafe.Sizeof(nid))
		pShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	}
}

// bindTray: l'interruttore delle Impostazioni.
func bindTray(w webview2.WebView) {
	_ = w.Bind("goSetCloseToTray", func(on bool) string {
		updateSettings(func(s *Settings) { s.NoCloseToTray = !on })
		return "ok"
	})
}

// trayChiusuraX: la X della finestra (gira nel filo della finestra, via ChiusuraHook). true = nascosta.
func trayChiusuraX() bool {
	if wv == nil || !trayIcona.Load() || trayFermata.Load() {
		return false // senza icona la X chiude davvero, se no Tasso resterebbe acceso senza modo di riaprirlo
	}
	setMu.Lock()
	no, visto := settings.NoCloseToTray, settings.TrayTipShown
	setMu.Unlock()
	if no {
		return false
	}
	pShowWin.Call(uintptr(wv.Window()), 0) // SW_HIDE
	if !visto {
		go func() {
			defer guard("tray.go:avviso")
			updateSettings(func(s *Settings) { s.TrayTipShown = true })
			trayAvviso("Tasso resta acceso qui", "Clic sull'icona per riaprirlo, clic destro per chiuderlo.", false)
		}()
	}
	return true
}

// mostraFinestra riapre una finestra nascosta o ridotta a icona e la porta davanti. Se era
// ingrandita resta ingrandita (SW_RESTORE solo se è ridotta a icona).
func mostraFinestra(hwnd uintptr) {
	pShowWin.Call(hwnd, 5) // SW_SHOW
	if ic, _, _ := pIsIconic.Call(hwnd); ic != 0 {
		pShowWin.Call(hwnd, 9) // SW_RESTORE
	}
	pSetFg.Call(hwnd)
}

// trayApri: riapre Tasso (nel filo della finestra).
func trayApri() {
	if wv == nil {
		return
	}
	wv.Dispatch(func() { mostraFinestra(uintptr(wv.Window())) })
}

// trayAvviso mostra un avviso dall'icona (su Windows 10/11 diventa una notifica). Testi in italiano:
// li traduco qui. quiet: rispetta "Non disturbare" e la prima ora dopo l'installazione.
func trayAvviso(titolo, testo string, quiet bool) {
	h := trayHwnd.Load()
	if h == 0 || !trayIcona.Load() {
		return
	}
	trayAvvMu.Lock()
	trayAvvTit, trayAvvTxt, trayAvvQuiet = trGo(titolo), trGo(testo), quiet
	trayAvvMu.Unlock()
	pPostMessage.Call(h, wmTrayAvviso, 0, 0)
}

// trayTesto: testo di una voce del menu, tradotto e con la prima lettera maiuscola.
func trayTesto(s string) string {
	r := []rune(trGo(s))
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return strings.ReplaceAll(string(r), "&", "&&")
}

func copiaUTF16(dst []uint16, s string) {
	u := utf16.Encode([]rune(s))
	if len(u) > len(dst)-1 {
		u = u[:len(dst)-1]
	}
	n := copy(dst, u)
	dst[n] = 0
}

// ---------------- Il filo dell'icona ----------------

func trayFilo() {
	defer guard("tray.go:filo")
	runtime.LockOSThread()
	defer close(trayFinito)
	var hinst windows.Handle
	_ = windows.GetModuleHandleEx(0, nil, &hinst)
	cls, _ := windows.UTF16PtrFromString("TassoIcona")
	wc := ovClass{lpfnWndProc: windows.NewCallback(trayWndProc), lpszClassName: cls, hInstance: uintptr(hinst)}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	if r, _, _ := pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return
	}
	// WS_EX_TOOLWINDOW, WS_POPUP, mai mostrata: niente pulsante nella barra delle applicazioni.
	h, _, _ := pCreateWindowEx.Call(0x80, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(cls)), 0x80000000, 0, 0, 0, 0, 0, 0, uintptr(hinst), 0)
	if h == 0 {
		return
	}
	defer pDestroyWindowTray.Call(h) // se il ciclo esce senza wmTrayFerma (per sicurezza)
	nome, _ := windows.UTF16PtrFromString("TaskbarCreated")
	r, _, _ := pRegisterWindowMsg.Call(uintptr(unsafe.Pointer(nome)))
	trayMsgBarra = uint32(r)
	// Tasso gira da amministratore: senza questo Windows non lascerebbe arrivare i messaggi di
	// Esplora risorse (che gira come utente normale): TaskbarCreated e i clic sull'icona.
	for _, m := range []uintptr{uintptr(trayMsgBarra), wmTrayIcona} {
		if m != 0 {
			pChangeWndMsgFilter.Call(h, m, 1, 0) // MSGFLT_ALLOW
		}
	}
	trayCaricaIcone(uintptr(hinst))
	trayHwnd.Store(h)
	if trayFermata.Load() {
		return
	}
	if !trayAggiungi(h) {
		pSetTimer.Call(h, trayTimerAdd, 3000, 0) // Esplora risorse non è ancora pronto (appena acceso il PC)
	}
	var m ovMsg
	for {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		pTranslateMsg.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMsg.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// trayCaricaIcone: il tassino dalle risorse di Tasso.exe: l'icona del tema in uso
// (icone_tema.go), di base IconId 2 (assets/tasso.ico).
func trayCaricaIcone(hinst uintptr) {
	id := idIconaAttuale()
	cx, _, _ := pGetSysMetrics.Call(49)                      // SM_CXSMICON
	cy, _, _ := pGetSysMetrics.Call(50)                      // SM_CYSMICON
	trayIco, _, _ = pLoadImage.Call(hinst, id, 1, cx, cy, 0) // IMAGE_ICON
	bx, _, _ := pGetSysMetrics.Call(11)                      // SM_CXICON
	by, _, _ := pGetSysMetrics.Call(12)                      // SM_CYICON
	trayIcoBig, _, _ = pLoadImage.Call(hinst, id, 1, bx, by, 0)
	if trayIco == 0 && id != 2 {
		trayIco, _, _ = pLoadImage.Call(hinst, 2, 1, cx, cy, 0)
		trayIcoBig, _, _ = pLoadImage.Call(hinst, 2, 1, bx, by, 0)
	}
	trayIcoProprie = trayIco != 0 // caricate con LoadImage: si possono liberare al cambio di tema
	if trayIco == 0 {
		trayIco, _, _ = pLoadIcon.Call(hinst, 2)
	}
	if trayIco == 0 {
		trayIco, _, _ = pLoadIcon.Call(0, 32512) // IDI_APPLICATION
	}
}

func trayDati(h uintptr) notifyIconData {
	nid := notifyIconData{HWnd: h, UID: trayID}
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	return nid
}

// trayAggiungi mette l'icona (anche dopo il riavvio di Esplora risorse).
func trayAggiungi(h uintptr) bool {
	nid := trayDati(h)
	nid.UFlags = nifMessage | nifIcon | nifTip | nifShowTip
	nid.UCallbackMessage = wmTrayIcona
	nid.HIcon = trayIco
	copiaUTF16(nid.SzTip[:], "Tasso "+appVersion)
	ok, _, _ := pShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	if ok == 0 {
		// c'è già (TaskbarCreated arrivato due volte): la aggiorno
		ok, _, _ = pShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
	}
	if ok == 0 {
		return false
	}
	nid.UVersion = 4 // NOTIFYICON_VERSION_4: clic, tastiera e menu come le icone di Windows
	pShellNotifyIcon.Call(nimSetVersion, uintptr(unsafe.Pointer(&nid)))
	if trayFermata.Load() { // trayStop arrivata mentre la mettevo
		d := trayDati(h)
		pShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&d)))
		return true
	}
	trayIcona.Store(true)
	return true
}

func trayWndProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch {
	case msg == wmTrayIcona:
		switch uint32(lp & 0xFFFF) {
		case ninSelect, ninKeySelect, wmLButtonDbl, ninBalloonClk:
			trayApri()
		case wmContextMenu:
			x, y := int32(int16(wp&0xFFFF)), int32(int16((wp>>16)&0xFFFF))
			trayMenu(hwnd, x, y)
		}
		return 0
	case msg == wmTrayAvviso:
		trayAvvMu.Lock()
		tit, txt, quiet := trayAvvTit, trayAvvTxt, trayAvvQuiet
		trayAvvMu.Unlock()
		if trayIcona.Load() {
			nid := trayDati(hwnd)
			nid.UFlags = nifInfo
			copiaUTF16(nid.SzInfoTitle[:], tit)
			copiaUTF16(nid.SzInfo[:], txt)
			nid.DwInfoFlags = niifUser
			if trayIcoBig != 0 {
				nid.HBalloonIcon = trayIcoBig
				nid.DwInfoFlags |= niifLargeIcon
			} else {
				nid.HBalloonIcon = trayIco
			}
			if quiet {
				nid.DwInfoFlags |= niifQuiet
			}
			pShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
		}
		return 0
	case msg == wmTrayTema:
		// è cambiato il tema (icone_tema.go): nuova icona, la vecchia si libera dopo averla tolta
		vecchia, vecchiaBig, proprie := trayIco, trayIcoBig, trayIcoProprie
		var hinst windows.Handle
		_ = windows.GetModuleHandleEx(0, nil, &hinst)
		trayCaricaIcone(uintptr(hinst))
		if trayIcona.Load() {
			nid := trayDati(hwnd)
			nid.UFlags = nifIcon
			nid.HIcon = trayIco
			pShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
		}
		for _, v := range []uintptr{vecchia, vecchiaBig} {
			if proprie && v != 0 && v != trayIco && v != trayIcoBig {
				pDestroyIcoTe.Call(v) // mai quelle di LoadIcon (condivise)
			}
		}
		return 0
	case msg == wmTrayFerma:
		pEndMenu.Call()
		nid := trayDati(hwnd)
		pShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
		trayIcona.Store(false)
		pDestroyWindowTray.Call(hwnd)
		return 0
	case msg == wmTimerMsg && wp == trayTimerAdd:
		if trayFermata.Load() || trayAggiungi(hwnd) {
			pKillTimer.Call(hwnd, trayTimerAdd)
		}
		return 0
	case msg == 0x0011: // WM_QUERYENDSESSION: Windows si spegne o ci si disconnette
		salvaAlloSpegnimento()
		return 1 // Tasso non blocca mai lo spegnimento
	case msg == 0x0016: // WM_ENDSESSION
		if wp != 0 {
			salvaAlloSpegnimento()
		}
		return 0
	case msg == wmDestroyMsg:
		pPostQuitMessageTray.Call(0)
		return 0
	case trayMsgBarra != 0 && msg == uintptr(trayMsgBarra):
		// Esplora risorse si è riavviato (o è cambiata la scala dello schermo): rimetto l'icona.
		if !trayFermata.Load() {
			trayIcona.Store(false)
			var hinst windows.Handle
			_ = windows.GetModuleHandleEx(0, nil, &hinst)
			trayCaricaIcone(uintptr(hinst))
			if !trayAggiungi(hwnd) {
				pSetTimer.Call(hwnd, trayTimerAdd, 3000, 0)
			}
		}
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wp, lp)
	return r
}

// ---------------- Menu del clic destro ----------------

func trayMenu(hwnd uintptr, x, y int32) {
	if x == 0 && y == 0 {
		var p struct{ X, Y int32 }
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
		x, y = p.X, p.Y
	}
	setMu.Lock()
	paused, mode := settings.Paused, settings.Mode
	setMu.Unlock()
	if mode == "" {
		mode = "normale"
	}
	office := officeMode()

	m, _, _ := pCreatePopupMenu.Call()
	if m == 0 {
		return
	}
	defer pDestroyMenu.Call(m)
	add := func(menu uintptr, flags, id uintptr, s string) {
		p, _ := windows.UTF16PtrFromString(trayTesto(s))
		pAppendMenu.Call(menu, flags, id, uintptr(unsafe.Pointer(p)))
	}
	const mfGrayed, mfChecked, mfPopup, mfSeparator = 0x1, 0x8, 0x10, 0x800
	grigio := func(off bool) uintptr {
		if off {
			return mfGrayed
		}
		return 0
	}
	add(m, 0, tmApri, "Apri Tasso")
	pSetMenuDefaultItem.Call(m, tmApri, 0) // in grassetto, come il clic sull'icona
	pAppendMenu.Call(m, mfSeparator, 0, 0)
	if !office {
		b := boostInfo()
		on, _ := b["on"].(bool)
		pending, _ := b["pending"].(bool)
		game, _ := b["game"].(string)
		add(m, grigio(paused || on || game != ""), tmBoost, "Boost")
		add(m, grigio(!on && !pending && game == ""), tmRimetti, "Rimetti com'era")
		f := uintptr(0)
		if traduce.Accesa() {
			f = mfChecked
		}
		add(m, f, tmTraduzione, "Traduzione dei giochi")
	}
	sub, _, _ := pCreatePopupMenu.Call()
	if sub != 0 {
		for i, md := range trayModi {
			// in Ufficio Gaming e FPS competitivo non ci sono (si vedono solo se sono già accese)
			if office && (md.id == "gaming" || md.id == "fps") && md.id != mode {
				continue
			}
			f := grigio(paused)
			if md.id == mode {
				f |= mfChecked
			}
			add(sub, f, uintptr(tmModo+i), md.nome)
		}
		add(m, mfPopup, sub, "Modalità del PC") // il sottomenu si distrugge con il menu
	}
	pAppendMenu.Call(m, mfSeparator, 0, 0)
	add(m, 0, tmChiudi, "Chiudi Tasso")

	// Senza SetForegroundWindow il menu non si chiude cliccando altrove (regola di Windows).
	pSetFg.Call(hwnd)
	align := uintptr(0x20)                          // TPM_BOTTOMALIGN
	if a, _, _ := pGetSysMetrics.Call(40); a != 0 { // SM_MENUDROPALIGNMENT
		align |= 0x8 // TPM_RIGHTALIGN
	}
	cmd, _, _ := pTrackPopupMenuEx.Call(m, 0x100|0x80|0x2|align, uintptr(x), uintptr(y), hwnd, 0) // RETURNCMD|NONOTIFY|RIGHTBUTTON
	pPostMessage.Call(hwnd, 0, 0, 0)                                                              // WM_NULL
	if cmd != 0 {
		go trayComando(int(cmd))
	}
}

// trayComando: la voce scelta nel menu (fuori dal filo dell'icona: i lavori possono essere lunghi).
func trayComando(cmd int) {
	defer guard("tray.go:comando")
	switch {
	case cmd == tmApri:
		trayApri()
	case cmd == tmBoost:
		if officeMode() {
			return
		}
		// come il pulsante Boost (goBoostAll in boost.go)
		if async("boostall", func() {
			msgs, ok := boostOnOK()
			emit("onBoost", map[string]interface{}{"msgs": msgs, "state": boostInfo()})
			trayAvvisoMsgs("Boost", msgs)
			if ok {
				// Dal menu dell'icona la finestra di Tasso di solito non si vede: niente prova della
				// frequenza dello schermo (la conferma di 15 secondi resterebbe nascosta).
				fpsAutoFix(false)
			}
		}) == "busy" {
			trayAvviso("Tasso", "Tasso sta lavorando: riprova quando ha finito.", false)
		}
	case cmd == tmRimetti:
		// come "Rimetti com'era" (goBoostAllOff in boost.go)
		if async("boostoff", func() {
			msgs := boostOff(true)
			emit("onBoost", map[string]interface{}{"msgs": msgs, "state": boostInfo()})
			trayAvvisoMsgs("Rimetti com'era", msgs)
		}) == "busy" {
			trayAvviso("Tasso", "Tasso sta lavorando: riprova quando ha finito.", false)
		}
	case cmd == tmTraduzione:
		if officeMode() {
			return
		}
		on := !traduce.Accesa()
		r, perche := traduce.Accendi(on)
		switch {
		case perche != "":
			trayAvviso("Traduzione dei giochi", perche, false)
		case r:
			// il tasto scelto in Giochi → Impostazioni → Tasti rapidi (di base F7)
			trayAvviso("Traduzione dei giochi", trGo("Traduzione accesa. Nel gioco premi ")+traduce.NomeTasto("pagina")+trGo(" per vedere le scritte in italiano."), false)
		default:
			trayAvviso("Traduzione dei giochi", "Traduzione spenta.", false)
		}
	case cmd >= tmModo && cmd < tmModo+len(trayModi):
		md := trayModi[cmd-tmModo]
		setMu.Lock()
		paused := settings.Paused
		setMu.Unlock()
		if paused {
			return
		}
		// come le schede delle Modalità (goSetMode in main.go)
		if setMode(md.id) {
			trayAvviso("Modalità del PC", trGo("Modalità attivata: ")+trayTesto(md.nome), true)
		} else {
			trayAvviso("Modalità del PC", "Non riuscito, riprova.", false)
		}
	case cmd == tmChiudi:
		// come la X della finestra delle Impostazioni: non a metà di una pulizia o di Togli Tasso
		if tassoNonSiChiude() { // ecosistema.go (lo stesso controllo di Tasso.exe --chiudi)
			trayApri()
			trayAvviso("Tasso", "Aspetta la fine della pulizia prima di chiudere.", false)
			return
		}
		chiudiTasso()
	}
}

// trayAvvisoMsgs: i messaggi del Boost in un avviso (tradotti uno per uno, come fa la finestra).
func trayAvvisoMsgs(titolo string, msgs []string) {
	t := make([]string, 0, len(msgs))
	for _, m := range msgs {
		t = append(t, trGo(m))
	}
	trayAvviso(titolo, strings.Join(t, " "), false)
}
