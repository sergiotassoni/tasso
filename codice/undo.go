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
			"detail": "Acceso dal " + p.Since + ". Spegnendolo rimetto la modalità e i comandi gaming com'erano prima."})
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
		was := "spento"
		if orig[id] {
			was = "acceso"
		}
		items = append(items, map[string]interface{}{"kind": "startup", "id": id, "title": "All'avvio: " + name, "detail": "Prima di Tasso era " + was + "."})
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
	regs, _ := filepath.Glob(filepath.Join(appDir, "backup", "*.reg"))
	return map[string]interface{}{"items": items, "regBackups": len(regs)}
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
		if isRunning() {
			msg = "Sta girando una pulizia: aspetta che finisca, poi riprova."
			break
		}
		modeMu.Lock()
		ok := runRestoreScript(snap, nil)
		modeMu.Unlock()
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
		if !gameTuneUndoQuiet(id) {
			msg = "Questo gioco non era regolato."
		}
	case "regfolder":
		_ = os.MkdirAll(filepath.Join(appDir, "backup"), 0o755)
		openPath(filepath.Join(os.Getenv("SystemRoot"), "explorer.exe"), "\""+filepath.Join(appDir, "backup")+"\"")
		return
	case "restorepoint":
		openPath(filepath.Join(os.Getenv("SystemRoot"), "System32", "rstrui.exe"), "")
		return
	case "recycle":
		openPath(filepath.Join(os.Getenv("SystemRoot"), "explorer.exe"), "shell:RecycleBinFolder")
		return
	default:
		return
	}
	emit("onUndoDone", map[string]interface{}{"msg": msg, "changes": changesList(), "settings": getSettings()})
}
