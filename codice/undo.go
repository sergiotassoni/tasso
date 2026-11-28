package main

// "Torna indietro": l'elenco di tutto quello che Tasso ha cambiato, con un pulsante per ogni cosa
// e uno per rimettere tutto com'era. Le impostazioni restano finché non premi tu: spegnere
// o riavviare il PC non cambia niente.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var gamingTitles = map[string]string{
	"gamedvr": "Registrazione in background spenta",
	"mmcss":   "Priorità ai giochi",
	"nagle":   "Rete a bassa latenza",
	"hags":    "Pianificazione GPU hardware",
	"winopt":  "Ottimizzazioni per i giochi in finestra",
}

func changesList() map[string]interface{} {
	setMu.Lock()
	snapAt, hasSnap := settings.SnapshotAt, len(settings.Snapshot) > 0
	orig := map[string]bool{}
	for k, v := range settings.StartupOrig {
		orig[k] = v
	}
	// Blocco al primo avvio: trGo prende setMu (curLang): chiamarla qui, con setMu già preso, fermava
	// per sempre Impostazioni, la coda dei pulsanti e la X. La traduzione si fa dopo l'Unlock.
	tunes := map[string]string{}
	for id, t := range settings.GameTunes {
		if t != nil {
			n := t.Name
			if n == "" {
				n = id
			}
			tunes[id] = n + "\x00" + t.Applied
		}
	}
	var cmds []string
	for id, on := range settings.GamingCmds {
		if on {
			cmds = append(cmds, id)
		}
	}
	setMu.Unlock()
	regolato := trGo(" · regolato il ") // con la data (/) la finestra non tradurrebbe il pezzo (F20-12)
	for id, v := range tunes {
		tunes[id] = strings.Replace(v, "\x00", regolato, 1)
	}

	var items []map[string]interface{}
	if p := profileCopy(); p.On {
		items = append(items, map[string]interface{}{"kind": "profile", "id": "", "title": "Profilo gioco acceso",
			"detail": trGo("Acceso dal " + p.Since + ". Spegnendolo rimetto la modalità e i comandi gaming com'erano prima.")}) // con la data (/) la finestra non tradurrebbe la frase
	}
	if hasSnap {
		// la data come nelle altre righe (27/11/2026 10:15), non 2026-11-27 (F14-13, F20-13); il testo lo
		// traduco qui perché con la data (/) la finestra non lo tradurrebbe
		items = append(items, map[string]interface{}{"kind": "windows", "id": "", "title": "Impostazioni di Windows",
			"detail": trGo("Piano energetico, schermo e sospensione, notifiche, mouse, Modalità Gioco, privacy. Salvate com'erano il ") + dataFoto(snapAt) + "."})
	}
	var ids []string
	for id := range orig {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		parts := strings.SplitN(id, "|", 2)
		if len(parts) != 2 {
			continue
		}
		name := parts[1]
		if parts[0] == "appx" {
			if i := strings.Index(name, "_"); i > 0 {
				name = name[:i]
			}
		}
		was := "Prima di Tasso era spento."
		if orig[id] {
			was = "Prima di Tasso era acceso."
		}
		items = append(items, map[string]interface{}{"kind": "startup", "id": id, "title": "All'avvio: " + name, "detail": was})
	}
	sort.Strings(cmds)
	for _, id := range cmds {
		items = append(items, map[string]interface{}{"kind": "gaming", "id": id, "title": "Comando gaming: " + gamingTitles[id], "detail": "Torna esattamente al valore che c'era prima."})
	}
	var tids []string
	for id := range tunes {
		tids = append(tids, id)
	}
	sort.Strings(tids)
	for _, id := range tids {
		items = append(items, map[string]interface{}{"kind": "tune", "id": id, "title": "Impostazioni consigliate", "detail": tunes[id]})
	}
	if n := attiveOtt(); n > 0 {
		items = append(items, map[string]interface{}{"kind": "ottimizza", "id": "", "title": "Ottimizza tutto",
			"detail": "Rimetti le rimette tutte com'erano; una alla volta dalla pagina Ottimizza tutto."})
	}
	regs, _ := filepath.Glob(filepath.Join(appDir, "backup", "*.reg"))
	return map[string]interface{}{"items": items, "regBackups": len(regs), "undoFiles": fileDaRimettere(), "notUndo": nonSiRimette}
}

// dataFoto: "2026-11-20 10:15" (come è salvata in settings.json) -> "20/11/2026 10:15".
func dataFoto(s string) string {
	if t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local); err == nil {
		return t.Format("02/01/2006 15:04")
	}
	return s
}

// nonSiRimette: quello che "Rimetti tutto com'era" NON rimette (detto chiaro nel pannello).
// F20-10: non tutte le riparazioni lasciano un file (alcune rimettono solo i valori normali di Windows).
var nonSiRimette = []string{
	"Le riparazioni di \"Risolvi un problema\" (servizi riaccesi, stampante predefinita, viste delle cartelle, Office, tastiera...): quasi tutte hanno il loro file per tornare indietro, nell'elenco sopra; le altre rimettono solo i valori normali di Windows.",
	"I file cancellati dalle pulizie (temporanei, cache, Cestino svuotato) e quelli distrutti.",
	"I programmi disinstallati, le app di Windows tolte e i resti tolti (cartelle nel Cestino, registro nel backup .reg).",
	"I driver e gli aggiornamenti di Windows installati: per quelli c'è il punto di ripristino di Windows.",
	"Le tracce cancellate (file recenti, cronologia di Esegui, appunti).",
}

// fileDaRimettere: i file "per tornare come prima" che lasciano le riparazioni (Save-Undo e
// Backup-Reg in %LOCALAPPDATA%\Tasso\backup-registro, nome_aaaammgg_hhmmss.reg/.cmd), dal più nuovo.
func fileDaRimettere() []map[string]interface{} {
	var out []map[string]interface{}
	voci, err := os.ReadDir(filepath.Join(appDir, "backup-registro"))
	if err != nil {
		return out
	}
	type fileUndo struct {
		nome, quando, file string
		t                  time.Time
	}
	var l []fileUndo
	for _, v := range voci {
		n := v.Name()
		ext := strings.ToLower(filepath.Ext(n))
		if v.IsDir() || (ext != ".reg" && ext != ".cmd") {
			continue
		}
		base := strings.TrimSuffix(n, filepath.Ext(n))
		nome, t := nomeEDataUndo(base)
		if t.IsZero() {
			if fi, err := v.Info(); err == nil {
				t = fi.ModTime()
			}
		}
		l = append(l, fileUndo{nome: titoloFileUndo(nome), quando: t.Format("02/01/2006 15:04"), file: n, t: t})
	}
	sort.Slice(l, func(i, j int) bool { return l[i].t.After(l[j].t) })
	if len(l) > 50 {
		l = l[:50]
	}
	for _, f := range l {
		out = append(out, map[string]interface{}{"name": f.nome, "when": f.quando, "file": f.file})
	}
	return out
}

// titoliFileUndo: il nome dei file delle riparazioni (Save-Undo e Backup-Reg in steps_*.go) in
// parole, tradotto (F14-12, F20-12: prima in tutte le lingue restava "viste cartelle", "zip windows"...).
var titoliFileUndo = map[string]string{
	"avvio rapido":                "Avvio rapido di Windows",
	"barra nascosta":              "Barra delle applicazioni nascosta",
	"bloc num avvio":              "Bloc Num all'accensione del PC",
	"bloc num utente":             "Bloc Num del tuo account",
	"cronologia appunti":          "Cronologia degli appunti",
	"esplora file apertura":       "Apertura di Esplora file",
	"file nascosti":               "File nascosti",
	"finestra in primo piano":     "Finestre in primo piano",
	"formati regionali":           "Formati di data e numeri",
	"icone desktop":               "Icone del desktop",
	"icone sistema desktop":       "Icone di sistema del desktop",
	"modalita gioco":              "Modalità Gioco",
	"mouse":                       "Mouse",
	"mouse accelerazione":         "Accelerazione del mouse",
	"mouse rotellina":             "Rotellina del mouse",
	"notifiche app":               "Notifiche delle app",
	"notifiche windows":           "Notifiche di Windows",
	"office grafica":              "Grafica di Office",
	"ora server":                  "Sincronizzazione dell'ora",
	"outlook classico":            "Outlook classico",
	"outlook componenti spenti":   "Componenti di Outlook spenti",
	"percorsi lunghi":             "Percorsi lunghi dei file",
	"posta predefinita pc":        "Programma di posta predefinito del PC",
	"posta predefinita utente":    "Programma di posta predefinito",
	"proxy":                       "Proxy di internet",
	"registrazione in sottofondo": "Registrazione in sottofondo dei giochi",
	"ricerca windows":             "Ricerca di Windows",
	"salvaschermo":                "Salvaschermo",
	"schermo nitido":              "Testo nitido sullo schermo",
	"tasti accessibilita":         "Tasti di accessibilità",
	"tastiera scorciatoie":        "Scorciatoie della tastiera",
	"tasto stamp":                 "Tasto Stamp",
	"tasto windows":               "Tasto Windows",
	"tasto windows tastiera":      "Tasto Windows della tastiera",
	"usb audio":                   "Risparmio energetico delle porte USB",
	"viste cartelle":              "Viste delle cartelle",
	"viste cartelle elenco":       "Elenco delle viste delle cartelle",
	"volume chiamate":             "Volume durante le chiamate",
	"zip windows":                 "File zip di Windows",
}

// titoloFileUndo: il titolo tradotto; i nomi con una parte variabile ("office accesso 16.0",
// "outlook TeamsAddin.FastConnect") hanno la prima parte tradotta e il resto com'è.
func titoloFileUndo(nome string) string {
	if t, ok := titoliFileUndo[strings.ToLower(nome)]; ok {
		return trGo(t)
	}
	for _, p := range [][2]string{{"office accesso ", "Accesso a Office "}, {"outlook ", "Outlook: "}} {
		if strings.HasPrefix(strings.ToLower(nome), p[0]) {
			return trGo(p[1]) + nome[len(p[0]):]
		}
	}
	return nome
}

// nomeEDataUndo: "viste-cartelle_20261126_101500" -> ("viste cartelle", 26/11/2026 10:15).
func nomeEDataUndo(base string) (string, time.Time) {
	p := strings.Split(base, "_")
	if len(p) >= 3 {
		if t, err := time.ParseInLocation("20060102_150405", p[len(p)-2]+"_"+p[len(p)-1], time.Local); err == nil {
			return strings.ReplaceAll(strings.Join(p[:len(p)-2], "_"), "-", " "), t
		}
	}
	return strings.ReplaceAll(base, "-", " "), time.Time{}
}

// undoChange rimette com'era una sola cosa.
func undoChange(kind, id string) {
	msg := "Fatto: rimesso com'era."
	riuscito := true // F14-10, F20-16: la finestra mostra gli errori con il colore degli avvisi
	switch kind {
	case "windows":
		setMu.Lock()
		snap := append(json.RawMessage(nil), settings.Snapshot...)
		setMu.Unlock()
		if len(snap) == 0 {
			msg, riuscito = "Non c'è niente da rimettere.", false
			break
		}
		if !claimRun() {
			msg, riuscito = "Sta girando una pulizia: aspetta che finisca, poi riprova.", false
			break
		}
		modeMu.Lock()
		ok := runRestoreScript(snap, nil)
		modeMu.Unlock()
		releaseRun()
		if !ok {
			msg, riuscito = "Non sono riuscito a rimettere le impostazioni di Windows: le tengo salvate, riprova tra poco.", false
			break
		}
		profileForget()
		updateSettings(func(s *Settings) { s.Snapshot, s.SnapshotAt, s.Mode = nil, "", "originale" })
		msg = "Impostazioni di Windows rimesse com'erano prima di Tasso."
	case "startup":
		setMu.Lock()
		was, ok := settings.StartupOrig[id]
		setMu.Unlock()
		parts := strings.SplitN(id, "|", 2)
		if !ok || len(parts) != 2 {
			msg, riuscito = "Non c'è niente da rimettere.", false
			break
		}
		// Lo script scrive "JSON ok" solo se, rileggendo, il valore è quello giusto: se no la voce resta
		// in Torna indietro e si può riprovare.
		r := runPSArgs(startupSetScript, map[string]interface{}{"scope": parts[0], "name": parts[1], "enable": was}, time.Minute, nil)
		if r.Err != nil || r.TooSlow || r.JSON != "ok" {
			msg, riuscito = "Non sono riuscito a rimetterlo com'era: riprova tra poco.", false
			break
		}
		updateSettings(func(s *Settings) { delete(s.StartupOrig, id) })
	case "gaming":
		if _, ok := gamingTitles[id]; !ok {
			return
		}
		profileDropCmd(id)
		if r := gamingSet(id, false); strings.HasPrefix(r, "ok:") {
			msg = strings.TrimPrefix(r, "ok:")
		} else {
			msg, riuscito = r, false
		}
	case "profile":
		msg = profileOffNow()
	case "tune":
		if gameTuneBusy(id) {
			msg, riuscito = "Il gioco è aperto: chiudilo e riprova, così le sue impostazioni tornano com'erano davvero.", false
		} else if r := gameTuneUndoQuiet(id, false); r == "" {
			msg, riuscito = "Questo gioco non era regolato.", false
		} else if r == "!ini" {
			msg, riuscito = msgIniNonRimesso, false
		}
	case "ottimizza":
		if es, ok := rimettiTuttoOtt(false); ok {
			msg = "Modifiche di Ottimizza tutto rimesse com'erano."
		} else {
			msg, riuscito = strings.Join(msgRestaOtt(es), " "), false // F04-2: il motivo vero e cosa fare
		}
	case "regfolder":
		_ = creaCartellaApp(filepath.Join(appDir, "backup"))
		openPath(explorerExe(), "\""+filepath.Join(appDir, "backup")+"\"")
		return
	case "undofolder":
		_ = creaCartellaApp(filepath.Join(appDir, "backup-registro"))
		openPath(explorerExe(), "\""+filepath.Join(appDir, "backup-registro")+"\"")
		return
	case "restorepoint":
		openPath(filepath.Join(sistema32(), "rstrui.exe"), "")
		return
	case "recycle":
		openPath(explorerExe(), "shell:RecycleBinFolder")
		return
	default:
		return
	}
	emit("onUndoDone", map[string]interface{}{"msg": msg, "ok": riuscito, "changes": changesList(), "settings": getSettings()})
}
