package main

// RECENSIONE-3, R3-3: togliendo Tasso dal disinstallatore (App e funzionalità), se Windows non
// lascia rimettere qualcosa com'era la disinstallazione va avanti (F20-1). Prima però i valori di
// com'era non si devono perdere: la foto di Windows (settings.json, Snapshot), i programmi
// all'avvio da rimettere (StartupOrig) e i valori salvati dalle modalità e dal profilo
// (HKCU\Software\Tasso\PrimaDelleModalita e PrimaDelProfilo), che dopo si cancellano. Vanno in un
// file che "Togli Tasso" porta in Documenti con le altre copie di sicurezza.

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"
)

// cartellaNonRimesse: dove aspetta il file, dentro la cartella dati (salvaCopieDiSicurezza lo
// sposta in Documenti\Tasso - copie di sicurezza\impostazioni non rimesse com'erano).
func cartellaNonRimesse() string { return filepath.Join(appDir, "impostazioni-non-rimesse") }

// nomeNonRimesseDocumenti: il nome della cartella in Documenti.
const nomeNonRimesseDocumenti = "impostazioni non rimesse com'erano"

// msgNonRimesseSalvate: l'avviso che lo dice (sezione "go" delle lingue).
const msgNonRimesseSalvate = "I valori che c'erano prima di Tasso li ho salvati in Documenti > Tasso - copie di sicurezza > impostazioni non rimesse com'erano: servono per rimetterli a mano (o per chi ti aiuta)."

// leggimiNonRimesse: il file che spiega (tradotto con trGo).
const leggimiNonRimesse = "Togliendo Tasso, Windows non ha lasciato rimettere com'erano alcune impostazioni.\nIl file che comincia con impostazioni-di-prima contiene i valori che c'erano prima di Tasso: il piano energetico e i tempi di spegnimento dello schermo e di sospensione (PrimaDelleModalita e PrimaDelProfilo), la foto delle impostazioni di Windows (snapshot) e i programmi all'avvio da rimettere (startupOrig, true = acceso).\nIn \"nonTornate\" c'è cosa non è tornato com'era. Puoi rimettere queste cose a mano in Impostazioni di Windows > Sistema > Alimentazione e in Gestione attività > Avvio, oppure mostrare il file a chi ti aiuta.\nTasso non usa più questo file: puoi cancellarlo quando vuoi."

// valoriChiaveUtente: i valori di una chiave di HKCU (nome -> valore). nil se la chiave non c'è.
func valoriChiaveUtente(percorso string) map[string]interface{} {
	k, err := registry.OpenKey(registry.CURRENT_USER, percorso, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	nomi, _ := k.ReadValueNames(-1)
	out := map[string]interface{}{}
	for _, n := range nomi {
		if s, _, err := k.GetStringValue(n); err == nil {
			out[n] = s
		} else if v, _, err := k.GetIntegerValue(n); err == nil {
			out[n] = v
		} else if l, _, err := k.GetStringsValue(n); err == nil {
			out[n] = l
		} else if b, _, err := k.GetBinaryValue(n); err == nil {
			out[n] = b
		}
	}
	return out
}

// salvaNonRimesse scrive impostazioni-di-prima.json (e il file che spiega) nella cartella dati,
// per le copie di sicurezza. true se è scritto. Niente segreti: solo i valori di com'era.
func salvaNonRimesse(nonTornate []string) bool {
	setMu.Lock()
	snap := append(json.RawMessage(nil), settings.Snapshot...)
	snapAt := settings.SnapshotAt
	avvio := map[string]bool{}
	for k, v := range settings.StartupOrig {
		avvio[k] = v
	}
	setMu.Unlock()
	dati := map[string]interface{}{
		"quando":     time.Now().Format(time.RFC3339),
		"versione":   appVersion,
		"nonTornate": nonTornate,
	}
	if len(snap) > 0 && json.Valid(snap) {
		dati["snapshot"], dati["snapshotAt"] = snap, snapAt
	}
	if len(avvio) > 0 {
		dati["startupOrig"] = avvio
	}
	if v := valoriChiaveUtente(`Software\Tasso\PrimaDelleModalita`); len(v) > 0 {
		dati["PrimaDelleModalita"] = v
	}
	if v := valoriChiaveUtente(`Software\Tasso\PrimaDelProfilo`); len(v) > 0 {
		dati["PrimaDelProfilo"] = v
	}
	b, err := json.MarshalIndent(dati, "", "  ")
	if err != nil {
		return false
	}
	d := cartellaNonRimesse()
	// sicuro_scrittura.go: niente collegamenti o giunzioni nella cartella dati (Tasso è amministratore)
	if creaCartellaApp(d) != nil {
		return false
	}
	nome := "impostazioni-di-prima_" + time.Now().Format("20060102_150405")
	if scriviFileApp(filepath.Join(d, nome+".json"), b, 0o644) != nil {
		return false
	}
	_ = scriviFileApp(filepath.Join(d, "LEGGIMI.txt"), []byte("\ufeff"+strings.ReplaceAll(trGo(leggimiNonRimesse), "\n", "\r\n")), 0o644)
	return true
}
