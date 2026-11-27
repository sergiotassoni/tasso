package main

// Il passo "Privacy: stop pubblicità e tracciamento" e "Privacy: ripristina" (Pulisci > Sicurezza e
// privacy), con il motore di Ottimizza tutto (ottimizza.Privacy): prima si salva com'era ogni
// singolo valore, poi si cambia, si rilegge; "Privacy: ripristina" rimette i valori di prima (non
// più "1 oppure cancellato"), solo dove c'è ancora quello di Tasso, e cancella quelli che prima non
// c'erano. Il file di stato è privacy-<SID>.json, nella stessa cartella di quello di Ottimizza tutto.
//
// Regola "chi l'ha messo?" con Ottimizza tutto (stessi valori di consigli e pubblicità): se un valore
// l'ha messo Ottimizza, Privacy prende il suo com'era vero e Ottimizza non lo rimette più
// (Ambiente.Tenuti in ambienteOtt). Così "Rimetti com'era" di Ottimizza non disfa la Privacy, e
// "Privacy: ripristina" rimette i valori di prima di Tasso.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"

	"tasso/ottimizza"
)

var privMu sync.Mutex // il file di stato della Privacy: uno alla volta (mai preso insieme a ottMu)

func filePrivacy() (string, bool) { return fileStatoNomeOtt("privacy-" + sidOtt() + ".json") }

// caricaPrivacy: lo stato del passo Privacy (vuoto se non c'è o è rovinato).
func caricaPrivacy() *ottimizza.Stato {
	privMu.Lock()
	defer privMu.Unlock()
	return caricaPrivacyLock()
}

func caricaPrivacyLock() *ottimizza.Stato {
	vuoto := &ottimizza.Stato{Versione: ottimizza.VersioneStato, Risultati: map[string]*ottimizza.Risultato{}}
	p, protetto := filePrivacy()
	st, _, err := leggiStatoFile(p, protetto, protetto)
	if err != nil || st == nil {
		st = vuoto
	}
	if protetto {
		// un passo fatto senza permessi: lo unisco a quello protetto e tolgo il file dell'utente
		u := filepathPrivacyUtente()
		if su, _, e := leggiStatoFile(u, false, false); e == nil {
			st.Unisci(su)
			if salvaPrivacy(st) == nil {
				_ = os.Remove(u)
				_ = os.Remove(u + ".prima")
			}
		}
	}
	return st
}

func filepathPrivacyUtente() string {
	u := fileStatoUtenteOtt()
	return strings.TrimSuffix(u, "stato-"+sidOtt()+".json") + "privacy-" + sidOtt() + ".json"
}

func salvaPrivacy(st *ottimizza.Stato) error {
	b, err := st.Codifica()
	if err != nil {
		return err
	}
	p, protetto := filePrivacy()
	return scriviSicuroOtt(p, b, protetto)
}

func ambientePrivacy() *ottimizza.Ambiente {
	pc := pcOtt()
	return &ottimizza.Ambiente{Reg: regVero{}, Sis: sisVero{}, PC: pc}
}

// privacyApplica: il passo "Privacy: stop pubblicità e tracciamento".
func privacyApplica(log func(string)) {
	// com'era vero dei valori messi da Ottimizza tutto (solo lettura del suo file di stato)
	stOtt := leggiSoloStato(fileStatoOtt)
	a := ambientePrivacy()
	a.Eredita = func(v ottimizza.Valore) (ottimizza.Valore, bool) { return ottimizza.PrimaVera(stOtt, v) }
	privMu.Lock()
	st := caricaPrivacyLock()
	e := ottimizza.Esegui(ottimizza.Privacy(), a, st, salvaPrivacy)
	privMu.Unlock()
	switch e.Stato {
	case ottimizza.VoceFatta:
		log("Pubblicità nel menu Start, suggerimenti, ID pubblicitario e raccolta dati extra: spenti.")
		log("Alcune modifiche si vedono dopo il riavvio. Con \"Privacy: ripristina\" tornano i valori che c'erano prima.")
	case ottimizza.VoceAPosto:
		log("Era già tutto spento: non ho cambiato niente.")
	default:
		log(e.Motivo)
	}
	if !a.PC.Admin {
		log("Senza i permessi di amministratore non ho cambiato il livello dei dati di diagnostica e i suggerimenti dal web nella ricerca: li decide l'amministratore.")
	} else if ottimizza.TelemetriaAlta(a.Reg) {
		log("Il livello dei dati di diagnostica lo ha deciso chi gestisce questo PC (azienda, scuola o un programma): l'ho lasciato com'è.")
	}
}

// privacyRimetti: "Privacy: ripristina". Rimette i valori di prima del passo Privacy. Per un passo
// fatto con una versione vecchia di Tasso (senza file di stato) usa la foto di com'era Windows,
// salvata prima del passo.
func privacyRimetti(log func(string)) {
	a := ambientePrivacy()
	privMu.Lock()
	st := caricaPrivacyLock()
	v := st.Voce(ottimizza.IDPrivacy)
	vecchio := false
	if v == nil || !v.Attiva() {
		if p, ok := privacyDallaFoto(); ok {
			st.Metti(ottimizza.Voce{ID: ottimizza.IDPrivacy, Stato: ottimizza.VoceFatta, Prima: p})
			vecchio = true
		}
	}
	e := ottimizza.RimettiM(ottimizza.Privacy(), a, st, salvaPrivacy)
	privMu.Unlock()
	switch {
	case e.Stato == ottimizza.VoceRimessa && e.Motivo == ottimizza.MsgNienteDa:
		log("Tasso non ha cambiato queste impostazioni (o non ha salvato com'erano): non tocco niente. Le trovi in Impostazioni > Privacy e sicurezza.")
	case e.Stato == ottimizza.VoceRimessa:
		if vecchio {
			log("Impostazioni di privacy rimesse com'erano prima di Tasso (dalla foto salvata prima del primo cambio).")
		} else {
			log("Impostazioni di privacy rimesse com'erano prima di Tasso.")
		}
	case e.Stato == ottimizza.VoceLasciata:
		log("Impostazioni di privacy rimesse com'erano prima di Tasso. Quelle che avevi già cambiato tu le ho lasciate come sono.")
	default:
		log(e.Motivo)
	}
}

// privacyRimettiTutto: per "Rimetti tutto com'era" (prima della foto). nil se non c'è niente.
func privacyRimettiTutto() []string {
	privMu.Lock()
	st := caricaPrivacyLock()
	if len(st.Attive()) == 0 {
		privMu.Unlock()
		return nil
	}
	e := ottimizza.RimettiM(ottimizza.Privacy(), ambientePrivacy(), st, salvaPrivacy)
	privMu.Unlock()
	if e.Stato == ottimizza.VoceErrore {
		return []string{"Privacy: " + trGo(e.Motivo)}
	}
	return nil
}

// fotoReg: una riga "reg" della foto di com'era Windows (snapshotScript in restore.go).
type fotoReg struct {
	Path   string      `json:"path"`
	Name   string      `json:"name"`
	Exists bool        `json:"exists"`
	Value  interface{} `json:"value,omitempty"`
	Kind   string      `json:"kind,omitempty"`
}

// valoreDaFoto: la riga della foto come valore del registro di Ottimizza (solo HKCU/HKLM, DWORD o testo).
func valoreDaFoto(r fotoReg) (ottimizza.Valore, bool) {
	var v ottimizza.Valore
	switch {
	case strings.HasPrefix(strings.ToUpper(r.Path), `HKCU:\`):
		v.Radice, v.Chiave = ottimizza.HKCU, r.Path[6:]
	case strings.HasPrefix(strings.ToUpper(r.Path), `HKLM:\`):
		v.Radice, v.Chiave = ottimizza.HKLM, r.Path[6:]
	default:
		return v, false
	}
	v.Nome = r.Name
	if !r.Exists {
		return v, true
	}
	v.Esiste = true
	switch r.Kind {
	case "DWord":
		f, ok := r.Value.(float64)
		if !ok || f < 0 || f > 0xFFFFFFFF || f != float64(uint32(f)) {
			return v, false
		}
		v.Tipo, v.Num = ottimizza.TipoDword, uint32(f)
	case "String":
		s, ok := r.Value.(string)
		if !ok {
			return v, false
		}
		v.Tipo, v.Testo = ottimizza.TipoSz, s
	default:
		return v, false
	}
	return v, true
}

// valoreInFoto: la riga della foto con il valore v (com'era o "non c'era").
func valoreInFoto(r *fotoReg, v ottimizza.Valore) {
	r.Exists, r.Value, r.Kind = v.Esiste, nil, ""
	if !v.Esiste {
		return
	}
	if v.Tipo == ottimizza.TipoDword {
		r.Value, r.Kind = float64(v.Num), "DWord"
	} else {
		r.Value, r.Kind = v.Testo, "String"
	}
}

// privacyDallaFoto: il com'era dei valori del passo Privacy dalla foto (per un passo fatto con una
// versione di Tasso senza file di stato). Vale solo per i valori che il passo cambia davvero.
func privacyDallaFoto() (ottimizza.Precedente, bool) {
	setMu.Lock()
	snap := append(json.RawMessage(nil), settings.Snapshot...)
	setMu.Unlock()
	clean := sanitizeSnapshot(snap)
	if clean == nil {
		return ottimizza.Precedente{}, false
	}
	b, _ := json.Marshal(clean["reg"])
	var righe []fotoReg
	if json.Unmarshal(b, &righe) != nil {
		return ottimizza.Precedente{}, false
	}
	var p ottimizza.Precedente
	for _, m := range ottimizza.ValoriPrivacy() {
		for _, r := range righe {
			v, ok := valoreDaFoto(r)
			if ok && v.Stesso(m) {
				v.Radice, v.Chiave, v.Nome = m.Radice, m.Chiave, m.Nome
				p.Valori = append(p.Valori, v)
				p.Messi = append(p.Messi, m)
				break
			}
		}
	}
	if len(p.Messi) == 0 {
		return ottimizza.Precedente{}, false
	}
	// passa dallo stesso controllo dei file di stato
	q, ok := ottimizza.Privacy().G.Valida(p, elevato())
	return q, ok && !q.Vuoto
}

// fotoConPrimaVera: nella foto di com'era Windows, i valori messi da Ottimizza tutto o dal passo
// Privacy ancora attivi diventano il loro com'era vero (di prima di Tasso). Così "Rimetti tutto
// com'era" non rimette mai un valore di Tasso, anche se la foto è stata fatta dopo.
func fotoConPrimaVera(raw string) string {
	var foto map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &foto) != nil {
		return raw
	}
	// solo lettura, senza blocchi: la foto si fa anche dentro un giro di Ottimizza tutto (ottMu preso)
	stOtt, stPriv := leggiSoloStato(fileStatoOtt), leggiSoloStato(filePrivacy)
	cambiate := false
	// piano energetico e modalità a corrente (voci piano e modoac di Ottimizza tutto)
	var attivo, modoAC string
	_ = json.Unmarshal(foto["active"], &attivo)
	_ = json.Unmarshal(foto["overlayAc"], &modoAC)
	if a2, m2 := ottimizza.PianoVero(stOtt, attivo, modoAC); a2 != attivo || m2 != modoAC {
		if a2 != attivo {
			foto["active"], _ = json.Marshal(a2)
		}
		if m2 != modoAC {
			foto["overlayAc"], _ = json.Marshal(m2)
		}
		cambiate = true
	}
	var righe []fotoReg
	if json.Unmarshal(foto["reg"], &righe) != nil {
		righe = nil
	}
	for i := range righe {
		adesso, ok := valoreDaFoto(righe[i])
		if !ok {
			continue
		}
		for _, st := range []*ottimizza.Stato{stOtt, stPriv} {
			if prima, ok := ottimizza.PrimaVera(st, adesso); ok {
				valoreInFoto(&righe[i], prima)
				cambiate = true
				break
			}
		}
	}
	if !cambiate {
		return raw
	}
	if righe != nil {
		b, err := json.Marshal(righe)
		if err != nil {
			return raw
		}
		foto["reg"] = b
	}
	out, err := json.Marshal(foto)
	if err != nil {
		return raw
	}
	return string(out)
}

// leggiSoloStato: il file di stato letto e basta (niente unioni, niente salvataggi). nil se non c'è.
func leggiSoloStato(file func() (string, bool)) *ottimizza.Stato {
	p, protetto := file()
	st, _, err := leggiStatoFile(p, protetto, protetto)
	if err != nil {
		return nil
	}
	return st
}

var errNienteGo = errors.New("passo senza codice")

// I due passi Privacy girano in Go (Step.Go), non più come script.
func init() {
	for i := range allSteps {
		switch allSteps[i].ID {
		case "privacy":
			allSteps[i].Go = privacyApplica
		case "privacy_undo":
			allSteps[i].Go = privacyRimetti
		}
	}
}

// eseguiPasso: un passaggio di Pulisci, in Go (Step.Go) o con il suo script.
func eseguiPasso(ctx context.Context, s Step, onMsg func(string), onProg func(float64), onInfo func(string)) psResult {
	if s.Go != nil {
		s.Go(onMsg)
		return psResult{}
	}
	if s.Script == "" {
		return psResult{Err: errNienteGo}
	}
	return runPSInfo(ctx, s.Script, s.Timeout, onMsg, onProg, onInfo, true)
}
