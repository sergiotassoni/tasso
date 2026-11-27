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
	tunes := map[string]string{}
	for id, t := range settings.GameTunes {
		if t != nil {
			n := t.Name
			if n == "" {
				n = id
			}
			tunes[id] = n + " · regolato il " + t.Applied
		}
	}
	var cmds []string
	for id, on := range settings.GamingCmds {
		if on {
			cmds = append(cmds, id)
		}
	}
	setMu.Unlock()

	var items []map[string]interface{}
	if p := profileCopy(); p.On {
		items = append(items, map[string]interface{}{"kind": "profile", "id": "", "title": "Profilo gioco acceso",
			"detail": trGo("Acceso dal " + p.Since + ". Spegnendolo rimetto la modalità e i comandi gaming com'erano prima.")}) // con la data (/) la finestra non tradurrebbe la frase
	}
	if hasSnap {
		items = append(items, map[string]interface{}{"kind": "windows", "id": "", "title": "Impostazioni di Windows",
			"detail": "Piano energetico, schermo e sospensione, notifiche, mouse, Modalità Gioco, privacy. Salvate com'erano il " + snapAt + "."})
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
		items = append(items, map[string]interface{}{"kind": "tune", "id": id, "title": "Regola con Tasso", "detail": tunes[id]})
	}
	if n := attiveOtt(); n > 0 {
		items = append(items, map[string]interface{}{"kind": "ottimizza", "id": "", "title": "Ottimizza tutto",
			"detail": "Le impostazioni di Windows cambiate da Ottimizza tutto: ognuna si rimette com'era anche dalla sua pagina."})
	}
	regs, _ := filepath.Glob(filepath.Join(appDir, "backup", "*.reg"))
	return map[string]interface{}{"items": items, "regBackups": len(regs), "undoFiles": fileDaRimettere(), "notUndo": nonSiRimette}
}

// nonSiRimette: quello che "Rimetti tutto com'era" NON rimette (detto chiaro nel pannello).
var nonSiRimette = []string{
	"Le riparazioni di \"Risolvi un problema\" (servizi riaccesi, stampante predefinita, viste delle cartelle, Office, tastiera...): ognuna ha il suo file per tornare indietro, nell'elenco qui sotto.",
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
		l = append(l, fileUndo{nome: nome, quando: t.Format("02/01/2006 15:04"), file: n, t: t})
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
	switch kind {
	case "windows":
		setMu.Lock()
		snap := append(json.RawMessage(nil), settings.Snapshot...)
		setMu.Unlock()
		if len(snap) == 0 {
			msg = "Non c'è niente da rimettere."
			break
		}
		if !claimRun() {
			msg = "Sta girando una pulizia: aspetta che finisca, poi riprova."
			break
		}
		modeMu.Lock()
		ok := runRestoreScript(snap, nil)
		modeMu.Unlock()
		releaseRun()
		if !ok {
			msg = "Non sono riuscito a rimettere le impostazioni di Windows: le tengo salvate, riprova tra poco."
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
			msg = "Non c'è niente da rimettere."
			break
		}
		runPSArgs(startupSetScript, map[string]interface{}{"scope": parts[0], "name": parts[1], "enable": was}, time.Minute, nil)
		updateSettings(func(s *Settings) { delete(s.StartupOrig, id) })
	case "gaming":
		if _, ok := gamingTitles[id]; !ok {
			return
		}
		profileDropCmd(id)
		if r := gamingSet(id, false); strings.HasPrefix(r, "ok:") {
			msg = strings.TrimPrefix(r, "ok:")
		} else {
			msg = r
		}
	case "profile":
		msg = profileOffNow()
	case "tune":
		if gameTuneBusy(id) {
			msg = "Il gioco è aperto: chiudilo e riprova, così le sue impostazioni tornano com'erano davvero."
		} else if r := gameTuneUndoQuiet(id, false); r == "" {
			msg = "Questo gioco non era regolato."
		} else if r == "!ini" {
			msg = msgIniNonRimesso
		}
	case "ottimizza":
		if _, ok := rimettiTuttoOtt(); ok {
			msg = "Modifiche di Ottimizza tutto rimesse com'erano."
		} else {
			msg = "Non sono riuscito a rimettere tutte le modifiche di Ottimizza tutto: riprova tra poco."
		}
	case "regfolder":
		_ = os.MkdirAll(filepath.Join(appDir, "backup"), 0o755)
		openPath(explorerExe(), "\""+filepath.Join(appDir, "backup")+"\"")
		return
	case "undofolder":
		_ = os.MkdirAll(filepath.Join(appDir, "backup-registro"), 0o755)
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
	emit("onUndoDone", map[string]interface{}{"msg": msg, "changes": changesList(), "settings": getSettings()})
}
