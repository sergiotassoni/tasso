package main

// Ottimizza tutto: il giro completo (prima misura, modifiche una alla volta, seconda misura) e le
// misure dell'Ufficio (tempo di accensione letto da Windows, apertura dei programmi di tutti i giorni).

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"tasso/ottimizza"
	"tasso/votopc"
)

var (
	ottCancelMu sync.Mutex
	ottCancel   context.CancelFunc
)

// pausaOtt: aspetta d (per dare il tempo di vedere l'interruttore che si muove), o meno se premi Fermati.
func pausaOtt(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// giroOtt: Ottimizza tutto dall'inizio alla fine. Gira in sottofondo (async "ottimizza").
func giroOtt(modo string) {
	if !claimRun() {
		emit("onOttStato", map[string]interface{}{"fase": "errore", "msg": "Tasso sta facendo un altro lavoro (pulizia o aggiornamento): aspetta che finisca e riprova."})
		return
	}
	defer releaseRun()
	ottInCorso.Store(true)
	defer ottInCorso.Store(false)
	ottStop.Store(false)
	ottSkip.Store(false)
	ctx, cancel := context.WithCancel(context.Background())
	ottCancelMu.Lock()
	ottCancel = cancel
	ottCancelMu.Unlock()
	defer func() {
		ottCancelMu.Lock()
		ottCancel = nil
		ottCancelMu.Unlock()
		cancel()
	}()
	sveglio := tienSveglio() // il PC non va in sospensione a metà
	defer sveglio()

	logPath := filepath.Join(appDir, "logs", "ottimizza_"+time.Now().Format("20060102_150405")+".txt")
	lf, _ := apriLogApp(logPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC)
	var logMu sync.Mutex
	logf := func(s string) {
		logMu.Lock()
		defer logMu.Unlock()
		if lf != nil {
			fmt.Fprintln(lf, time.Now().Format("15:04:05")+"  "+trGo(s))
		}
	}
	defer func() {
		if lf != nil {
			lf.Close()
		}
	}()
	logf(fmt.Sprintf("Tasso %s - Ottimizza tutto (%s)", appVersion, modo))

	// 1. prima misura
	emit("onOttStato", map[string]interface{}{"fase": "misura", "modo": modo})
	prima, comp := misuraOtt(ctx, modo, true, logf)
	if ottStop.Load() {
		logf("Fermato da te durante la prima misura: non ho cambiato niente.")
		emit("onOttStato", map[string]interface{}{"fase": "fermato", "msg": "Fermato: non ho cambiato niente."})
		return
	}

	// 2. le modifiche, una alla volta. Prima la foto di com'era Windows (se non c'è ancora): così
	// "Rimetti tutto com'era" non prende mai come "com'era" il piano o un valore messo da Tasso.
	if !ensureSnapshot() {
		logf("Foto di com'era Windows non salvata: vado avanti, ogni modifica ha comunque il suo com'era.")
	}
	elenco := ottimizza.Elenco(modo)
	emit("onOttStato", map[string]interface{}{"fase": "sistemo", "modo": modo, "totale": len(elenco)})
	a := ambienteOtt(ctx, logf)
	var fatteOra []string
	fatte, pulite, fatti := 0, 0, 0
	for _, c := range ottimizza.Categorie(modo) {
		if ottStop.Load() {
			break
		}
		ottSkip.Store(false)
		emitRaw("onOttCat", map[string]interface{}{"cat": c.ID})
		if c.Chiedi {
			n := contaAvvioOtt()
			emitRaw("onOttAvvio", map[string]interface{}{"n": n})
			logf(fmt.Sprintf("Programmi all'avvio accesi: %d (decidi tu quali togliere).", n))
			pausaOtt(ctx, 1500*time.Millisecond)
			continue
		}
		for _, m := range elenco {
			if m.Categoria != c.ID || ottStop.Load() {
				continue
			}
			if ottSkip.Load() {
				e := ottimizza.Esito{ID: m.ID, Stato: ottimizza.VoceSaltata, Motivo: "Saltata da te: resta com'era."}
				fatti++
				emit("onOttPasso", map[string]interface{}{"id": m.ID, "cat": c.ID, "fase": "fine", "esito": e, "fatti": fatti, "totale": len(elenco)})
				continue
			}
			emitRaw("onOttPasso", map[string]interface{}{"id": m.ID, "cat": c.ID, "fase": "inizio", "fatti": fatti, "totale": len(elenco)})
			pausaOtt(ctx, 700*time.Millisecond)
			if ottStop.Load() {
				break
			}
			ottMu.Lock()
			st := caricaOtt()
			e := ottimizza.Esegui(m, a, st, salvaOtt)
			ottMu.Unlock()
			logf(m.ID + ": " + e.Stato + " - " + e.Motivo)
			if e.Stato == ottimizza.VoceFatta {
				fatteOra = append(fatteOra, m.ID)
			}
			if e.Stato == ottimizza.VoceFatta {
				fatte++
			}
			if e.Stato == ottimizza.VocePulita {
				pulite++
			}
			fatti++
			emit("onOttPasso", map[string]interface{}{"id": m.ID, "cat": c.ID, "fase": "fine", "esito": e, "fatti": fatti, "totale": len(elenco)})
			pausaOtt(ctx, 350*time.Millisecond)
		}
	}
	if ottStop.Load() {
		// Fermati: rimetto com'era quello che ho cambiato in questo giro
		a2 := ambienteRimettiOtt(context.Background(), logf)
		ottMu.Lock()
		st := caricaOtt()
		errori := 0
		for i := len(fatteOra) - 1; i >= 0; i-- {
			e := ottimizza.Rimetti(fatteOra[i], a2, st, salvaOtt)
			logf(fatteOra[i] + ": " + e.Stato + " - " + e.Motivo)
			if e.Stato == ottimizza.VoceErrore {
				errori++
			}
		}
		ottMu.Unlock()
		msg := "Fermato: ho rimesso com'era quello che avevo cambiato."
		switch {
		case errori > 0:
			msg = "Fermato: qualcosa non è tornato com'era. Lo trovi in Rimetti com'era, da dove puoi riprovare."
		case pulite > 0:
			msg = "Fermato: ho rimesso com'era quello che avevo cambiato (i file inutili già tolti non tornano)."
		case len(fatteOra) == 0:
			msg = "Fermato: non ho cambiato niente."
		}
		logf(msg)
		emit("onOttStato", map[string]interface{}{"fase": "fermato", "msg": msg})
		return
	}

	// 3. seconda misura e risultato
	emit("onOttStato", map[string]interface{}{"fase": "dopo", "modo": modo})
	dopo, comp2 := misuraOtt(context.Background(), modo, false, logf)
	if len(comp2) > 0 {
		comp = comp2
	}
	r := &ottimizza.Risultato{Modo: modo, Quando: time.Now().Format(time.RFC3339), Prima: prima, Dopo: &dopo, Componenti: comp, Fatte: fatte, Pulite: pulite, Totale: len(elenco)}
	if modo == ottimizza.ModoUfficio && prima.Accensione > 0 {
		r.AccInAttesa = true // l'accensione "dopo" si misura alla prossima riaccensione del PC
	}
	ottMu.Lock()
	st := caricaOtt()
	st.Risultati[modo] = r
	_ = salvaOtt(st)
	ottMu.Unlock()
	logf(fmt.Sprintf("Fine: %d modifiche fatte su %d.", fatte, len(elenco)))
	emit("onOttStato", map[string]interface{}{"fase": "fine", "info": infoOtt()})
}

// contaAvvioOtt: quanti programmi partono all'avvio (stesso elenco della pagina Avvio del PC).
func contaAvvioOtt() int {
	r := runPS(startupListScript, 2*time.Minute, nil)
	list, _ := decodeJSON(r.JSON).([]interface{})
	n := 0
	for _, it := range list {
		if m, ok := it.(map[string]interface{}); ok && m["enabled"] == true {
			n++
		}
	}
	return n
}

// ---------------- Le misure ----------------

func nomeWindowsOtt() string {
	if winBuild() >= 22000 {
		return "Windows 11"
	}
	return "Windows 10"
}

func tipoDiscoOtt(k string) string {
	switch k {
	case "nvme":
		return "SSD NVMe"
	case "ssd":
		return "SSD"
	case "hdd":
		return "HDD"
	}
	return ""
}

func ramTestoOtt(gb float64, tipo string, mts int) string {
	s := ""
	if gb > 0 {
		s = fmt.Sprintf("%.0f GB", gb)
		if tipo != "" {
			s += " " + tipo
		}
	}
	if mts > 0 {
		if s != "" {
			s += " · "
		}
		s += fmt.Sprintf("%d MT/s", mts)
	}
	return s
}

// misuraOtt: la misura di prima (prima=true) o di dopo. Manda alla finestra l'avanzamento, i
// componenti man mano che li legge e i numeri appena ci sono.
func misuraOtt(ctx context.Context, modo string, prima bool, logf func(string)) (ottimizza.Misura, []ottimizza.Componente) {
	m := ottimizza.Misura{Quando: time.Now().Format(time.RFC3339)}
	valori := map[string]interface{}{}
	var comps []map[string]interface{}
	durata := 40.0 // secondi stimati che restano
	prog := func(pct float64, testo string) {
		emit("onOttMisura", map[string]interface{}{"prima": prima, "pct": math.Round(pct), "testo": testo,
			"secondi": int(math.Max(1, durata*(100-pct)/100)), "valori": valori, "componenti": comps})
	}
	gioco := modo != ottimizza.ModoUfficio
	if gioco {
		durata = 70
	} else {
		durata = 55
	}
	if !prima {
		durata -= 10
	}
	// componenti (Leggo i componenti…)
	prog(1, "Leggo i componenti…")
	info := leggiInfoPC()
	schede := schedeVideo()
	best, _, _, gpuOk := votopc.SceltaGPU(schede, info.Portatile)
	ramTipo, _ := votopc.TipoRAMCPU(info.RAMSMBIOS, info.RAMMTs, info.CPU)
	voci := [][2]string{{"cpu", info.CPU}, {"gpu", ""}, {"ram", ramTestoOtt(info.RAMGB, ramTipo, info.RAMMTs)}, {"disco", tipoDiscoOtt(info.Disco)}}
	if gpuOk {
		voci[1][1] = best.Nome
	}
	for i, v := range voci {
		comps = append(comps, map[string]interface{}{"chiave": v[0], "nome": v[1]})
		prog(2+float64(i)*2, "Leggo i componenti…")
		pausaOtt(ctx, 350*time.Millisecond)
	}
	pct := 10.0
	if gioco {
		// la scena di prova DirectX: FPS medi e più bassi (1%)
		prog(pct, "Misuro gli FPS…")
		durate, err := scenaFPS(ctx, 30*time.Second, func(f float64) {
			testo := "Misuro gli FPS…"
			if f > 0.6 {
				testo = "Misuro gli FPS più bassi…"
			}
			prog(10+f*40, testo)
		})
		if err == nil {
			m.FPSMedi, m.FPSBassi, m.FPSOk = ottimizza.StatFotogrammi(durate)
		}
		if !m.FPSOk {
			m.FPSMotivo = "La scena di prova DirectX non è partita su questo PC: continuo senza FPS."
			if err != nil {
				logf("Scena di prova: " + err.Error())
			}
			emit("onOttAvviso", map[string]interface{}{"msg": m.FPSMotivo})
		} else {
			valori["fps"], valori["fps1"] = math.Round(m.FPSMedi), math.Round(m.FPSBassi)
			logf(fmt.Sprintf("FPS medi %.0f, più bassi (1%%) %.0f", m.FPSMedi, m.FPSBassi))
		}
		pct = 50
	} else {
		if prima {
			prog(pct, "Leggo il tempo di accensione…")
			l, motivo := leggiAccensioniOtt()
			if u, ok := ottimizza.UltimaAccensione(l); ok {
				m.Accensione, m.AccQuando = u.Ms, u.Inizio.Format(time.RFC3339)
				valori["acc"] = math.Round(float64(u.Ms)/100) / 10
				logf(fmt.Sprintf("Accensione del PC: %.1f s", float64(u.Ms)/1000))
			} else {
				m.AccMotivo = motivo
				emit("onOttAvviso", map[string]interface{}{"msg": motivo})
			}
		}
		pct = 15
		prog(pct, "Apro i programmi di tutti i giorni…")
		m.Apertura = aperturaOtt(ctx, func(f float64) { prog(15+f*30, "Apro i programmi di tutti i giorni…") }, logf)
		if p, _, ok := ottimizza.AperturaTotale(m.Apertura, nil); ok {
			valori["apertura"] = math.Round(p*10) / 10
		}
		pct = 45
	}
	if ctx.Err() != nil {
		return m, nil
	}
	// il Voto al PC completo (circa 25 secondi): l'avanzamento va avanti da solo
	prog(pct, "Calcolo il Voto al PC…")
	fermo := make(chan struct{})
	go func() {
		defer guard("ottimizza:avanzamento")
		t0 := time.Now()
		for {
			select {
			case <-fermo:
				return
			case <-time.After(700 * time.Millisecond):
				f := math.Min(0.97, time.Since(t0).Seconds()/26)
				prog(pct+(99-pct)*f, "Calcolo il Voto al PC…")
			}
		}
	}()
	etichetta := "ottimizza-dopo"
	if prima {
		etichetta = "ottimizza-prima"
	}
	r := runBench(etichetta)
	close(fermo)
	m.Voto, m.Leggerezza = r.Total, r.Light
	if m.Voto <= 0 {
		m.Voto = int(math.Max(1, math.Round(0.6*float64(score100(r.Score))+0.4*math.Max(1, float64(r.Light)))))
	}
	valori["voto"] = m.Voto
	var out []ottimizza.Componente
	if r.Params != nil {
		rp := r.Report
		nomeCPU, ram, disco := info.CPU, voci[2][1], voci[3][1]
		if rp != nil {
			if rp.Processore != "" {
				nomeCPU = rp.Processore
			}
			if s := ramTestoOtt(rp.RAMGB, rp.RAMTipo, rp.RAMMTs); s != "" {
				ram = s
			}
			if s := tipoDiscoOtt(rp.Disco); s != "" {
				disco = s
			}
		}
		out = []ottimizza.Componente{{Chiave: "cpu", Nome: nomeCPU, Voto: r.Params.Processore}}
		if r.Params.SchedaVideo > 0 {
			out = append(out, ottimizza.Componente{Chiave: "gpu", Nome: r.GPUName, Voto: r.Params.SchedaVideo})
		}
		out = append(out, ottimizza.Componente{Chiave: "ram", Nome: ram, Voto: r.Params.RAM})
		if r.Params.Disco > 0 {
			out = append(out, ottimizza.Componente{Chiave: "disco", Nome: disco, Voto: r.Params.Disco})
		}
		out = append(out, ottimizza.Componente{Chiave: "leggerezza", Nome: nomeWindowsOtt(), Voto: r.Params.Leggerezza})
		comps = nil
		for _, c := range out {
			comps = append(comps, map[string]interface{}{"chiave": c.Chiave, "nome": c.Nome})
		}
	}
	logf(fmt.Sprintf("Voto al PC: %d", m.Voto))
	prog(100, "Misura finita")
	return m, out
}

// ---------------- Tempo di accensione (Windows) ----------------

var (
	wevtapiOtt = windows.NewLazySystemDLL("wevtapi.dll")
	pEvtQuery  = wevtapiOtt.NewProc("EvtQuery")
	pEvtNext   = wevtapiOtt.NewProc("EvtNext")
	pEvtRender = wevtapiOtt.NewProc("EvtRender")
	pEvtClose  = wevtapiOtt.NewProc("EvtClose")
)

const (
	msgAccAdmin   = "Per leggere il tempo di accensione serve Tasso aperto come amministratore: lo salto."
	msgAccNoLog   = "Windows su questo PC non registra il tempo di accensione: lo salto."
	msgAccNessuna = "Windows non ha ancora registrato un'accensione da leggere (lo fa qualche minuto dopo l'accensione): lo salto."
)

// leggiAccensioniOtt: le ultime accensioni dal registro eventi Diagnostics-Performance (evento
// 100, il tempo di accensione che misura Windows stesso). Nessun testo nella lingua di Windows.
func leggiAccensioniOtt() ([]ottimizza.Accensione, string) {
	defer guard("ottimizza:accensione")
	if pEvtQuery.Find() != nil {
		return nil, msgAccNoLog
	}
	canale, _ := windows.UTF16PtrFromString("Microsoft-Windows-Diagnostics-Performance/Operational")
	query, _ := windows.UTF16PtrFromString("*[System[(EventID=100)]]")
	const channelPath, reverse = 0x1, 0x200
	h, _, errQ := pEvtQuery.Call(0, uintptr(unsafe.Pointer(canale)), uintptr(unsafe.Pointer(query)), channelPath|reverse)
	if h == 0 {
		if errQ == windows.ERROR_ACCESS_DENIED {
			return nil, msgAccAdmin
		}
		return nil, msgAccNoLog
	}
	defer pEvtClose.Call(h)
	var out []ottimizza.Accensione
	for giri := 0; giri < 3 && len(out) < 10; giri++ {
		var ev [10]uintptr
		var n uint32
		if r, _, _ := pEvtNext.Call(h, 10, uintptr(unsafe.Pointer(&ev[0])), 2000, 0, uintptr(unsafe.Pointer(&n))); r == 0 || n == 0 {
			break
		}
		for i := uint32(0); i < n; i++ {
			if x := renderEventoOtt(ev[i]); x != "" {
				if a, err := ottimizza.LeggiEvento100(x); err == nil {
					out = append(out, a)
				}
			}
			pEvtClose.Call(ev[i])
		}
	}
	if len(out) == 0 {
		return nil, msgAccNessuna
	}
	return out, ""
}

func renderEventoOtt(ev uintptr) string {
	const renderXML = 1
	var usato, props uint32
	pEvtRender.Call(0, ev, renderXML, 0, 0, uintptr(unsafe.Pointer(&usato)), uintptr(unsafe.Pointer(&props)))
	if usato == 0 || usato > 1<<20 {
		return ""
	}
	buf := make([]uint16, usato/2+1)
	if r, _, _ := pEvtRender.Call(0, ev, renderXML, uintptr(len(buf)*2), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&usato)), uintptr(unsafe.Pointer(&props))); r == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

// accensioneDopoOtt: in Ufficio, cerca la prima accensione dopo Ottimizza tutto e la mette nel
// risultato. true se il risultato è cambiato.
func accensioneDopoOtt() bool {
	ottMu.Lock()
	st := caricaOtt()
	r := st.Risultati[ottimizza.ModoUfficio]
	ottMu.Unlock()
	if r == nil || !r.AccInAttesa || r.Dopo == nil {
		return false
	}
	fatto, err := time.Parse(time.RFC3339, r.Quando)
	if err != nil {
		return false
	}
	l, _ := leggiAccensioniOtt()
	a, ok := ottimizza.AccensioneDopo(l, fatto)
	if !ok {
		return false
	}
	ottMu.Lock()
	defer ottMu.Unlock()
	st = caricaOtt()
	r = st.Risultati[ottimizza.ModoUfficio]
	if r == nil || r.Dopo == nil {
		return false
	}
	r.Dopo.Accensione, r.Dopo.AccQuando, r.AccInAttesa = a.Ms, a.Inizio.Format(time.RFC3339), false
	return salvaOtt(st) == nil
}

// ---------------- Apertura dei programmi di tutti i giorni ----------------

var (
	ottEnumMu   sync.Mutex
	ottEnumList []uintptr
	ottEnumCb   = windows.NewCallback(func(hwnd, _ uintptr) uintptr {
		ottEnumList = append(ottEnumList, hwnd)
		return 1
	})
	pGetWindowOtt = user32.NewProc("GetWindow")
)

// classeOtt: la classe della finestra (per riconoscere le finestre di Esplora file: CabinetWClass).
func classeOtt(h uintptr) string {
	buf := make([]uint16, 128)
	n, _, _ := vtpGetClassName.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

// finestreOtt: le finestre principali visibili con il programma che le ha aperte (in minuscolo).
// Per Esplora file conta solo la finestra delle cartelle (CabinetWClass): le finestre di copia dei
// file, il desktop e la barra non sono "la finestra aperta da Tasso".
func finestreOtt() map[uintptr]string {
	ottEnumMu.Lock()
	ottEnumList = nil
	pEnumWindows.Call(ottEnumCb, 0)
	l := append([]uintptr(nil), ottEnumList...)
	ottEnumMu.Unlock()
	out := map[uintptr]string{}
	for _, h := range l {
		if v, _, _ := pIsWindowVisible.Call(h); v == 0 {
			continue
		}
		if o, _, _ := pGetWindowOtt.Call(h, 4); o != 0 { // GW_OWNER: finestre secondarie no
			continue
		}
		var pid uint32
		pGetWindowPid.Call(h, uintptr(unsafe.Pointer(&pid)))
		if pid == 0 {
			continue
		}
		exe := strings.ToLower(filepath.Base(processPath(pid)))
		if exe == "explorer.exe" && classeOtt(h) != "CabinetWClass" {
			continue
		}
		out[h] = exe
	}
	return out
}

// chiudiNuoveOtt: la finestra aperta da Tasso arrivata dopo il tempo massimo, o con Fermati. Guarda
// per qualche secondo e chiude al massimo UNA finestra nuova di quel programma (Tasso ne ha aperta
// una sola): se nel frattempo ne apri tu un'altra, resta aperta. WM_CLOSE: il Blocco note chiede di
// salvare, niente lavoro perso.
func chiudiNuoveOtt(prima map[uintptr]string, exe map[string]bool) {
	for t0 := time.Now(); time.Since(t0) < 6*time.Second; time.Sleep(300 * time.Millisecond) {
		for h, e := range finestreOtt() {
			if _, c := prima[h]; !c && exe[e] {
				pPostMessage.Call(h, 0x0010, 0, 0) // WM_CLOSE
				return
			}
		}
	}
}

// browserOtt: il programma del browser predefinito (dalla scelta dell'utente per i file .html,
// poi per i link https). "" se non si capisce.
func browserOtt() string {
	for _, k := range []string{`Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\.html\UserChoice`, `Software\Microsoft\Windows\Shell\Associations\UrlAssociations\https\UserChoice`} {
		prog, ok := regGet(registry.CURRENT_USER, k, "ProgId")
		if !ok || prog == "" || strings.ContainsAny(prog, `\/`) {
			continue
		}
		cmd, ok := regGet(registry.CLASSES_ROOT, prog+`\shell\open\command`, "")
		if !ok {
			continue
		}
		cmd = strings.TrimSpace(cmd)
		exe := ""
		if strings.HasPrefix(cmd, `"`) {
			if i := strings.Index(cmd[1:], `"`); i > 0 {
				exe = cmd[1 : 1+i]
			}
		} else if i := strings.Index(strings.ToLower(cmd), ".exe"); i > 0 {
			exe = cmd[:i+4]
		}
		if strings.HasSuffix(strings.ToLower(exe), ".exe") {
			return strings.ToLower(filepath.Base(exe))
		}
	}
	return ""
}

// aspettaFinestraOtt: il tempo dall'apertura alla prima finestra nuova di uno di questi programmi.
func aspettaFinestraOtt(ctx context.Context, prima map[uintptr]string, exe map[string]bool, apri func() bool) (time.Duration, uintptr) {
	t0 := time.Now()
	if !apri() {
		return 0, 0
	}
	for time.Since(t0) < 20*time.Second && ctx.Err() == nil {
		time.Sleep(25 * time.Millisecond)
		for h, e := range finestreOtt() {
			if _, c := prima[h]; !c && exe[e] {
				return time.Since(t0), h
			}
		}
	}
	return 0, 0
}

// aperturaOtt: apre e richiude Esplora file, Blocco note e il browser predefinito, come utente
// normale, e misura quanto ci mette ognuno a mostrare la finestra. Il browser si misura solo se
// non è già aperto (se no si aprirebbe una scheda in una finestra che c'è già).
func aperturaOtt(ctx context.Context, onProg func(float64), logf func(string)) map[string]int {
	out := map[string]int{}
	docs := cartellaNota(windows.FOLDERID_Documents)
	if docs == "" {
		docs = os.Getenv("USERPROFILE")
	}
	pagina := filepath.Join(appDir, "tmp", "tasso-prova.html")
	_ = os.MkdirAll(filepath.Dir(pagina), 0o755)
	_ = scriviFileApp(pagina, []byte("<!DOCTYPE html><html><head><meta charset=\"utf-8\"><title>Tasso</title></head><body style=\"font-family:sans-serif\">Tasso: prova di apertura del browser. Si chiude da sola.</body></html>"), 0o644)
	br := browserOtt()
	browser := map[string]bool{}
	for _, e := range []string{"msedge.exe", "chrome.exe", "firefox.exe", "brave.exe", "opera.exe", "vivaldi.exe"} {
		browser[e] = true
	}
	if br != "" {
		browser = map[string]bool{br: true}
	}
	prove := []struct {
		id   string
		exe  map[string]bool
		apri func() bool
	}{
		{"esplora", map[string]bool{"explorer.exe": true}, func() bool { return docs != "" && openPath(explorerExe(), "\""+docs+"\"") }},
		{"blocconote", map[string]bool{"notepad.exe": true}, func() bool { return openPath(explorerExe(), "\""+filepath.Join(sistema32(), "notepad.exe")+"\"") }},
		{"browser", browser, func() bool { return fileExists(pagina) && openPath(explorerExe(), "\""+pagina+"\"") }},
	}
	for i, p := range prove {
		if ctx.Err() != nil {
			break
		}
		onProg(float64(i) / 3)
		prima := finestreOtt()
		if p.id == "browser" || p.id == "blocconote" {
			// già aperto: si aprirebbe solo una scheda (browser, Blocco note di Windows 11)
			aperto := false
			for _, e := range prima {
				if p.exe[e] {
					aperto = true
				}
			}
			if aperto || exeRunning(keysOtt(p.exe)) {
				logf(p.id + ": già aperto, non lo misuro (si aprirebbe solo una scheda).")
				continue
			}
		}
		d, h := aspettaFinestraOtt(ctx, prima, p.exe, p.apri)
		if h != 0 {
			out[p.id] = int(d / time.Millisecond)
			logf(fmt.Sprintf("Apertura %s: %.1f s", p.id, d.Seconds()))
			pausaOtt(ctx, 1200*time.Millisecond)
			pPostMessage.Call(h, 0x0010, 0, 0) // WM_CLOSE: si chiude solo la finestra aperta da Tasso
		} else {
			logf("Apertura " + p.id + ": nessuna finestra nuova, non la conto.")
			// arrivata dopo il tempo massimo o con Fermati: la chiudo lo stesso
			chiudiNuoveOtt(prima, p.exe)
		}
		pausaOtt(ctx, 1500*time.Millisecond)
	}
	onProg(1)
	_ = os.Remove(pagina)
	return out
}

func keysOtt(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
