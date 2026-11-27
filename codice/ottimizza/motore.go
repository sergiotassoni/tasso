package ottimizza

import (
	"errors"
	"time"
)

// Messaggi (in italiano: la finestra li traduce con il dizionario "go").
const (
	MsgAPosto        = "Era già a posto: la lascio così."
	MsgGiaFatta      = "Già sistemata da Tasso."
	MsgNonLeggo      = "Non riesco a leggere questa impostazione su questo PC: la salto."
	MsgNonSalvo      = "Non riesco a salvare com'era: per sicurezza non la cambio."
	MsgNonCambia     = "Windows non mi ha lasciato cambiarla: è rimasta com'era."
	MsgNonResta      = "Il cambio non è rimasto: l'ho lasciata com'era."
	MsgNonC          = "Su questo PC non c'è: la salto."
	MsgFatta         = "Fatto."
	MsgFattaRiavvio  = "Fatto: vale dal prossimo riavvio del PC."
	MsgPulita        = "Fatto. I file tolti non servivano più (questa non si rimette com'era)."
	MsgPuliziaErrore = "Pulizia fatta in parte: alcuni file erano in uso."
	MsgRimessa       = "Rimessa com'era."
	MsgLasciata      = "L'avevi già cambiata tu (o un'altra parte di Tasso): la lascio com'è."
	MsgNonRimetto    = "Non sono riuscito a rimetterla com'era: riprova tra poco."
	MsgNienteDa      = "Non c'è niente da rimettere."
	MsgAltroUtente   = "Tasso è aperto con un altro account amministratore: questa impostazione è del tuo account e non la cambio. Apri Tasso dal tuo account."
	MsgModoAcceso    = "Una modalità di Tasso (Gaming, FPS, Film o Silenziosa) è accesa e gestisce già questa impostazione: la lascio a lei."
	MsgModoRimetti   = "Una modalità di Tasso (Gaming, FPS, Film o Silenziosa) è accesa: torna alla modalità Normale e poi rimetti com'era."
)

// Esito: cosa è successo a una modifica (per la finestra).
type Esito struct {
	ID     string `json:"id"`
	Stato  string `json:"stato"`
	Motivo string `json:"motivo"`
}

// Salvatore: scrive il file di stato su disco (deve riuscire PRIMA di ogni cambio).
type Salvatore func(*Stato) error

func adesso() string { return time.Now().Format(time.RFC3339) }

// Esegui fa una modifica: già a posto, salva com'era (su disco), applica, verifica. Se qualcosa
// non va, rimette quello che aveva già scritto e va avanti (non ferma tutto). Il "com'era" vero
// non si perde mai: una voce si chiude solo dopo che i suoi valori sono tornati com'erano (o
// l'utente li ha cambiati); se il rimetti non riesce, la voce resta attiva con il suo com'era.
func Esegui(m Modifica, a *Ambiente, st *Stato, salva Salvatore) Esito {
	fine := func(stato, motivo string, prima Precedente) Esito {
		v := Voce{ID: m.ID, Stato: stato, Motivo: motivo, Quando: adesso(), Prima: prima}
		// una voce già attiva non si perde mai (il suo "com'era" è quello vero di prima di Tasso)
		if old := st.Voce(m.ID); old == nil || !old.Attiva() || stato == VoceFatta || stato == VoceInCorso {
			st.Metti(v)
		}
		_ = salva(st)
		return Esito{ID: m.ID, Stato: stato, Motivo: motivo}
	}
	if m.Utente && a.PC.AltroUtente {
		return fine(VoceSaltata, MsgAltroUtente, Precedente{})
	}
	if m.Modo && a.PC.ModoTasso != "" {
		return fine(VoceSaltata, MsgModoAcceso, Precedente{})
	}
	if ok, motivo := m.G.Disponibile(a); !ok {
		if motivo == "" {
			motivo = MsgNonC
		}
		return fine(VoceSaltata, motivo, Precedente{})
	}
	// Già cambiata da Tasso in un giro di prima e ancora com'era stata messa: non la riscrivo (il
	// "com'era" salvato allora è quello vero, uno nuovo sarebbe già quello di Tasso).
	if old := st.Voce(m.ID); old != nil && old.Attiva() && !m.NonRimette {
		if m.G.Verifica(a, old.Prima) {
			old.Stato = VoceFatta
			_ = salva(st)
			return Esito{ID: m.ID, Stato: VoceAPosto, Motivo: MsgGiaFatta}
		}
		// Cambiata in parte (da te, o un giro interrotto a metà): prima rimetto com'erano i valori
		// che hanno ancora quello di Tasso, poi riparto da com'è adesso. Se non riesco, la voce
		// resta com'è (con il suo com'era) e questa volta la salto.
		if _, err := m.G.Rimetti(perRimettere(a), old.Prima); err != nil {
			return Esito{ID: m.ID, Stato: VoceErrore, Motivo: MsgNonRimetto}
		}
		old.Stato, old.Motivo, old.Prima = VoceLasciata, MsgLasciata, Precedente{}
		if err := salva(st); err != nil {
			return Esito{ID: m.ID, Stato: VoceSaltata, Motivo: MsgNonSalvo}
		}
	}
	if !m.NonRimette {
		ok, err := m.G.APosto(a)
		if err != nil {
			return fine(VoceSaltata, MsgNonLeggo, Precedente{})
		}
		if ok {
			return fine(VoceAPosto, MsgAPosto, Precedente{})
		}
	}
	p, err := m.G.Prepara(a)
	if err != nil {
		if errors.Is(err, ErrNonDisponibile) {
			return fine(VoceSaltata, MsgNonC, Precedente{})
		}
		return fine(VoceSaltata, MsgNonLeggo, Precedente{})
	}
	if p.Vuoto && !m.NonRimette {
		return fine(VoceAPosto, MsgAPosto, Precedente{})
	}
	if m.NonRimette {
		if err := m.G.Applica(a, p); err != nil {
			return fine(VocePulita, MsgPuliziaErrore, Precedente{})
		}
		return fine(VocePulita, MsgPulita, Precedente{})
	}
	// prima su disco (ogni valore com'era), poi il cambio: se Tasso o il PC si spengono a metà, al
	// prossimo giro o "Rimetti com'era" si rimette lo stesso (solo dove c'è ancora il valore di Tasso)
	st.Metti(Voce{ID: m.ID, Stato: VoceInCorso, Quando: adesso(), Prima: p})
	if err := salva(st); err != nil {
		st.Metti(Voce{ID: m.ID, Stato: VoceSaltata, Motivo: MsgNonSalvo, Quando: adesso()})
		return Esito{ID: m.ID, Stato: VoceSaltata, Motivo: MsgNonSalvo}
	}
	annulla := func(motivo string) Esito {
		if _, err := m.G.Rimetti(perRimettere(a), p); err != nil {
			// non sono riuscito a rimettere: la voce resta "in corso" con il suo com'era
			st.Metti(Voce{ID: m.ID, Stato: VoceInCorso, Motivo: MsgNonRimetto, Quando: adesso(), Prima: p})
			_ = salva(st)
			return Esito{ID: m.ID, Stato: VoceErrore, Motivo: motivo}
		}
		st.Metti(Voce{ID: m.ID, Stato: VoceErrore, Motivo: motivo, Quando: adesso()})
		_ = salva(st)
		return Esito{ID: m.ID, Stato: VoceErrore, Motivo: motivo}
	}
	if err := m.G.Applica(a, p); err != nil {
		return annulla(MsgNonCambia)
	}
	if m.VerificaLenta && a.Attendi != nil {
		a.Attendi(3 * time.Second) // Windows potrebbe rimettere il suo valore subito dopo
	}
	if !m.G.Verifica(a, p) {
		return annulla(MsgNonResta)
	}
	motivo := MsgFatta
	if m.Riavvio {
		motivo = MsgFattaRiavvio
	}
	st.Metti(Voce{ID: m.ID, Stato: VoceFatta, Motivo: motivo, Quando: adesso(), Prima: p})
	_ = salva(st)
	return Esito{ID: m.ID, Stato: VoceFatta, Motivo: motivo}
}

// Rimetti rimette com'era una modifica fatta da Tasso. Se non riesce la voce resta attiva (con il
// suo com'era) e si può riprovare.
func Rimetti(id string, a *Ambiente, st *Stato, salva Salvatore) Esito {
	m, ok := Trova(id)
	v := st.Voce(id)
	if !ok || v == nil || !v.Attiva() {
		return Esito{ID: id, Stato: VoceRimessa, Motivo: MsgNienteDa}
	}
	if m.Utente && a.PC.AltroUtente {
		return Esito{ID: id, Stato: VoceErrore, Motivo: MsgAltroUtente}
	}
	if m.Modo && a.PC.ModoTasso != "" {
		return Esito{ID: id, Stato: VoceErrore, Motivo: MsgModoRimetti}
	}
	r, err := m.G.Rimetti(perRimettere(a), v.Prima)
	if err != nil {
		return Esito{ID: id, Stato: VoceErrore, Motivo: MsgNonRimetto}
	}
	stato, motivo := VoceRimessa, MsgRimessa
	if r == RimessoLascia {
		stato, motivo = VoceLasciata, MsgLasciata
	}
	v.Stato, v.Motivo, v.Quando, v.Prima = stato, motivo, adesso(), Precedente{}
	_ = salva(st)
	return Esito{ID: id, Stato: stato, Motivo: motivo}
}

// RimettiTutto rimette com'erano tutte le modifiche attive.
// ok=false se qualcuna non è tornata (resta salvata: si può riprovare).
func RimettiTutto(a *Ambiente, st *Stato, salva Salvatore) ([]Esito, bool) {
	ids := st.Attive()
	var out []Esito
	tutto := true
	for i := len(ids) - 1; i >= 0; i-- {
		e := Rimetti(ids[i], a, st, salva)
		if e.Stato == VoceErrore {
			tutto = false
		}
		out = append(out, e)
	}
	return out, tutto
}
