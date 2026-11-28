package main

// "Togli Tasso dal PC" con la Modalità riunione o il blocco del PC accesi da un ALTRO account del
// PC (CTRL-FIN-14 F14-4 e F14-19, CTRL-FIN-20 F20-2). Prima Tasso si fermava per sempre ("spegnili
// dalla sezione Ufficio di Tasso"), anche dal disinstallatore, e da questo account non si poteva
// fare. Adesso non si ferma più, come Ottimizza tutto (ottAltriFile):
//   - quello che è di tutto il PC (la pausa di 1 giorno degli aggiornamenti messa da Tasso, il piano
//     di alimentazione) lo rimetto io, da amministratore;
//   - le impostazioni di quell'account (notifiche, screensaver) le rimetto io se Windows le ha
//     caricate (l'account è ancora collegato, per esempio con "Cambia utente"); se no lascio sul suo
//     Desktop (o nella sua cartella personale) un file .reg con i valori di prima e un .txt che
//     spiega, come Ottimizza tutto (cartellaProfiloOtt: solo cartelle che può scrivere solo lui);
//   - il file con i valori di prima va con le copie di sicurezza in Documenti.
// Ogni valore letto dal file si controlla come fa Fine-Riunione (il file sta in
// Programmi\Tasso\lavoro\ufficio, dove scrivono solo gli amministratori, ma non mi fido lo stesso).

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows/registry"

	"tasso/ottimizza"
)

const (
	chiaveNotificheUff = `Software\Microsoft\Windows\CurrentVersion\Notifications\Settings`
	chiaveToastUff     = `Software\Microsoft\Windows\CurrentVersion\PushNotifications`
	chiaveDesktopUff   = `Control Panel\Desktop`
	chiaveWUPausaUff   = `SOFTWARE\Microsoft\WindowsUpdate\UX\Settings`
	pianoMassimoUff    = "8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c" // "Prestazioni elevate"
)

var (
	reGUIDUff  = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	reGUIDIn   = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	reZeroUno  = regexp.MustCompile(`^[01]$`)
	reAttesa   = regexp.MustCompile(`^\d{0,6}$`)
	reZeroUnoV = regexp.MustCompile(`^[01]?$`)
)

// statoRiunioneUff e statoBloccoUff: i campi che scrivono i pulsanti (steps_office3.go).
type statoRiunioneUff struct {
	Noc    string `json:"noc"`
	Toast  string `json:"toast"`
	Plan   string `json:"plan"`
	WU     bool   `json:"wu"`
	WUFine string `json:"wufine"`
}

type statoBloccoUff struct {
	To  string `json:"to"`
	Act string `json:"act"`
	Sec string `json:"sec"`
	Exe string `json:"exe"`
}

// sidDaFileUff: il SID di un account vero dal nome riunione-<SID>.json o blocco-<SID>.json ("" se no).
func sidDaFileUff(base string) string {
	for _, pre := range []string{"riunione-", "blocco-"} {
		if strings.HasPrefix(base, pre) && strings.HasSuffix(base, ".json") {
			return ottimizza.SIDDaFileStato("stato-" + strings.TrimPrefix(base, pre))
		}
	}
	return ""
}

// valoriRiunioneUff: le impostazioni di quell'account che la Modalità riunione rimette (come Fine-Riunione).
func valoriRiunioneUff(s statoRiunioneUff) []ottimizza.Valore {
	v := func(chiave, nome, x string) ottimizza.Valore {
		r := ottimizza.Valore{Radice: ottimizza.HKCU, Chiave: chiave, Nome: nome}
		if reZeroUno.MatchString(x) {
			r.Esiste, r.Tipo, r.Num = true, ottimizza.TipoDword, uint32(x[0]-'0')
		}
		return r
	}
	return []ottimizza.Valore{v(chiaveNotificheUff, "NOC_GLOBAL_SETTING_TOASTS_ENABLED", s.Noc), v(chiaveToastUff, "ToastEnabled", s.Toast)}
}

// valoriBloccoUff: lo screensaver di prima (come il pulsante "Blocca il PC quando ti alzi").
// ok=false: valori strani, non li rimetto.
func valoriBloccoUff(s statoBloccoUff) ([]ottimizza.Valore, bool) {
	exe := strings.ToLower(s.Exe)
	if !reAttesa.MatchString(s.To) || !reZeroUnoV.MatchString(s.Act) || !reZeroUnoV.MatchString(s.Sec) ||
		(s.Exe != "" && (!strings.HasSuffix(exe, ".scr") || strings.ContainsAny(s.Exe, "\"\r\n\x00") || len(s.Exe) > 260)) {
		return nil, false
	}
	var out []ottimizza.Valore
	for _, x := range [][2]string{{"ScreenSaveTimeOut", s.To}, {"ScreenSaveActive", s.Act}, {"ScreenSaverIsSecure", s.Sec}, {"SCRNSAVE.EXE", s.Exe}} {
		r := ottimizza.Valore{Radice: ottimizza.HKCU, Chiave: chiaveDesktopUff, Nome: x[0]}
		if x[1] != "" {
			r.Esiste, r.Tipo, r.Testo = true, ottimizza.TipoSz, x[1]
		}
		out = append(out, r)
	}
	return out, true
}

// scriviValoriCaricati: se Windows ha caricato le impostazioni dell'account (HKEY_USERS\<SID>), le
// rimetto lì. false = account non collegato (o non riuscito): allora serve il file .reg.
func scriviValoriCaricati(sid string, valori []ottimizza.Valore) bool {
	radice, err := registry.OpenKey(registry.USERS, sid, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	radice.Close()
	tutti := true
	for _, v := range valori {
		if !v.Esiste {
			k, err := registry.OpenKey(registry.USERS, sid+`\`+v.Chiave, registry.SET_VALUE)
			if err != nil {
				continue // la chiave non c'è: il valore non c'è
			}
			if e := k.DeleteValue(v.Nome); e != nil && e != registry.ErrNotExist {
				tutti = false
			}
			k.Close()
			continue
		}
		k, _, err := registry.CreateKey(registry.USERS, sid+`\`+v.Chiave, registry.SET_VALUE)
		if err != nil {
			tutti = false
			continue
		}
		if v.Tipo == ottimizza.TipoDword {
			err = k.SetDWordValue(v.Nome, v.Num)
		} else {
			err = k.SetStringValue(v.Nome, v.Testo)
		}
		if err != nil {
			tutti = false
		}
		k.Close()
	}
	return tutti
}

const leggimiRegUff = "Questo file serve a rimettere com'erano le notifiche e lo screensaver di questo account, cambiati dalla Modalità riunione o da \"Blocca il PC quando ti alzi\" di Tasso.\nIl file .reg con lo stesso nome rimette i valori che c'erano prima. Attenzione: rimette quelli di prima anche se nel frattempo li hai cambiati tu.\nCome si usa: entra in Windows con questo account, fai doppio clic sul file .reg, rispondi Sì alle domande di Windows, poi esci da Windows e rientra.\nDopo puoi cancellare tutti e due i file."

// regAltroAccountUff: il file .reg (e il .txt che spiega) nella cartella di quell'account, come
// regAltroAccountOtt. Una copia va anche con le copie di sicurezza.
func regAltroAccountUff(sid string, valori []ottimizza.Valore) bool {
	if len(valori) == 0 || sid == "" {
		return false
	}
	u := utf16.Encode([]rune(ottimizza.TestoReg(valori)))
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
	}, trGo("Tasso - rimetti com'era le impostazioni di questo account")) + "_" + time.Now().Format("20060102_150405") + "_ufficio.reg"
	if dir := filepath.Join(appDir, "backup-registro", "altro account"); creaCartellaApp(dir) == nil {
		_ = scriviFileApp(filepath.Join(dir, nome), b, 0o644)
	}
	d := cartellaProfiloOtt(sid)
	if d == "" {
		return false
	}
	nuovo := func(f string, dati []byte) bool {
		h, err := os.OpenFile(f, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) // mai un file che c'è già
		if err != nil {
			return false
		}
		_, err = h.Write(dati)
		if e := h.Close(); err == nil {
			err = e
		}
		if err != nil {
			_ = os.Remove(f)
		}
		return err == nil
	}
	if !nuovo(filepath.Join(d, nome), b) {
		return false
	}
	_ = nuovo(filepath.Join(d, strings.TrimSuffix(nome, ".reg")+".txt"), []byte("\ufeff"+strings.ReplaceAll(trGo(leggimiRegUff), "\n", "\r\n")))
	return true
}

// copiaUffPerDocumenti: una copia del file in %LOCALAPPDATA%\Tasso\backup-registro, che "Togli
// Tasso dal PC" sposta in Documenti con le altre copie di sicurezza.
func copiaUffPerDocumenti(p string, b []byte) bool {
	dir := filepath.Join(appDir, "backup-registro")
	if creaCartellaApp(dir) != nil {
		return false
	}
	nome := "ufficio-altro-account-" + strings.TrimSuffix(filepath.Base(p), ".json") + "_" + time.Now().Format("20060102_150405") + ".json"
	return scriviFileApp(filepath.Join(dir, nome), b, 0o644) == nil
}

// togliPausaAggUff: toglie la pausa degli aggiornamenti solo se è ancora quella messa da Tasso.
// false = c'era e non sono riuscito a toglierla.
func togliPausaAggUff(fine string) bool {
	if fine == "" {
		return true
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, chiaveWUPausaUff, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return true // la chiave non c'è: niente pausa
	}
	defer k.Close()
	if ex, _, err := k.GetStringValue("PauseUpdatesExpiryTime"); err != nil || ex != fine {
		return true // cambiata da te nel frattempo (o già finita): resta com'è
	}
	tutti := true
	for _, n := range []string{"PauseUpdatesStartTime", "PauseUpdatesExpiryTime", "PauseFeatureUpdatesStartTime", "PauseFeatureUpdatesEndTime", "PauseQualityUpdatesStartTime", "PauseQualityUpdatesEndTime"} {
		if e := k.DeleteValue(n); e != nil && e != registry.ErrNotExist {
			tutti = false
		}
	}
	return tutti
}

// rimettiPianoUff: il piano di alimentazione di prima, solo se adesso c'è ancora "Prestazioni
// elevate" messo dalla riunione (il piano attivo è di tutto il PC).
func rimettiPianoUff(piano string) {
	if !reGUIDUff.MatchString(piano) || strings.EqualFold(piano, pianoMassimoUff) {
		return
	}
	pc := filepath.Join(sistema32(), "powercfg.exe")
	out, err := runHidden(pc, "/getactivescheme")
	if err != nil || !strings.EqualFold(reGUIDIn.FindString(out), pianoMassimoUff) {
		return
	}
	_, _ = runHidden(pc, "/setactive", piano)
}

// officeAltriAccount: le Modalità riunione e i blocchi del PC accesi da altri account. Non blocca mai
// "Togli Tasso": restituisce gli avvisi da mostrare (già tutto rimesso, file sul Desktop o niente).
func officeAltriAccount() (avvisi []string) {
	mioR, mioB := fileUfficio("riunione"), fileUfficio("blocco")
	fatti, conReg, senzaReg, rovinati, pausaNo := 0, 0, 0, 0, false
	for _, n := range []string{"riunione-*.json", "blocco-*.json"} {
		trovati, _ := filepath.Glob(filepath.Join(officeDir(), n))
		for _, f := range trovati {
			base := filepath.Base(f)
			if strings.EqualFold(f, mioR) || strings.EqualFold(f, mioB) || strings.Contains(base, "-illeggibile-") {
				continue // i miei li spegne officeSpegniTutto; quelli rovinati restano da parte
			}
			b, err := os.ReadFile(f)
			if err != nil || len(b) > 64<<10 {
				rovinati++
				continue
			}
			b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
			sid := sidDaFileUff(base)
			var valori []ottimizza.Valore
			ok := sid != ""
			if strings.HasPrefix(base, "riunione-") {
				var s statoRiunioneUff
				if ok = ok && json.Unmarshal(b, &s) == nil; ok {
					if s.WU && !togliPausaAggUff(s.WUFine) {
						pausaNo = true
					}
					rimettiPianoUff(s.Plan)
					valori = valoriRiunioneUff(s)
				}
			} else {
				var s statoBloccoUff
				if ok = ok && json.Unmarshal(b, &s) == nil; ok {
					valori, ok = valoriBloccoUff(s)
				}
			}
			salvato := copiaUffPerDocumenti(f, b)
			switch {
			case !ok:
				rovinati++
			case scriviValoriCaricati(sid, valori):
				fatti++
			case regAltroAccountUff(sid, valori):
				conReg++
			default:
				senzaReg++
			}
			if salvato || ok {
				// i valori di prima sono rimessi, in un file .reg o con le copie di sicurezza: questo
				// file non serve più, e non deve fermare "Togli Tasso"
				_ = os.Remove(f)
			}
		}
	}
	if fatti > 0 {
		avvisi = append(avvisi, "Modalità riunione o blocco del PC accesi in un altro account del PC: ho rimesso io le impostazioni di prima di quell'account (notifiche, screensaver, aggiornamenti e alimentazione).")
	}
	if conReg > 0 {
		avvisi = append(avvisi, "Modalità riunione o blocco del PC accesi in un altro account del PC: aggiornamenti e alimentazione li ho rimessi io, notifiche e screensaver di quell'account no (Tasso non può cambiarli da qui). Per rimetterli: entra in Windows con quell'account, fai doppio clic sul file \"Tasso - rimetti com'era…\" che trovi sul suo Desktop, rispondi Sì, poi esci e rientra.")
	}
	if senzaReg > 0 {
		avvisi = append(avvisi, "Modalità riunione o blocco del PC accesi in un altro account del PC: aggiornamenti e alimentazione li ho rimessi io, ma non ho potuto lasciare un file nella cartella di quell'account. Se l'account c'è ancora, entra in Windows con lui e controlla le notifiche (Impostazioni > Sistema > Notifiche) e lo screensaver (Impostazioni > Personalizzazione > Schermata di blocco > Screensaver). I valori di prima vanno con le copie di sicurezza in Documenti.")
	}
	if rovinati > 0 {
		avvisi = append(avvisi, "Un file della Modalità riunione o del blocco del PC di un altro account del PC era rovinato: tolgo Tasso lo stesso e il file va con le copie di sicurezza in Documenti. Se quell'account c'è ancora, entra in Windows con lui e controlla le notifiche, lo screensaver e, in Windows Update, che gli aggiornamenti non siano in pausa.")
	}
	if pausaNo {
		avvisi = append(avvisi, "Non sono riuscito a togliere la pausa degli aggiornamenti di Windows messa dalla Modalità riunione di un altro account: finisce da sola entro un giorno.")
	}
	return avvisi
}

// ufficioAncoraAcceso: la Modalità riunione o il blocco del PC di QUESTO account sono ancora accesi
// (officeSpegniTutto non è riuscito): le impostazioni di prima stanno solo in quei file.
func ufficioAncoraAcceso() bool {
	for _, f := range []string{fileUfficio("riunione"), fileUfficio("blocco"), filepath.Join(officeDir(), "riunione.json"), filepath.Join(officeDir(), "blocco.json")} {
		if _, err := os.Stat(f); err == nil {
			return true
		}
	}
	return false
}
