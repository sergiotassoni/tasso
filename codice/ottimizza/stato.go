package ottimizza

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"time"
)

// Stati di una voce del file di stato.
const (
	VoceInCorso  = "incorso"  // salvato com'era, il cambio sta per partire (o si è interrotto a metà)
	VoceFatta    = "fatta"    // cambiata da Tasso: si può rimettere com'era
	VoceAPosto   = "aposto"   // era già a posto: niente cambiato
	VoceSaltata  = "saltata"  // non c'è su questo PC, o non si poteva fare
	VoceErrore   = "errore"   // Windows non l'ha lasciata cambiare: lasciata com'era
	VoceRimessa  = "rimessa"  // rimessa com'era
	VoceLasciata = "lasciata" // l'avevi già cambiata tu: lasciata com'è
	VocePulita   = "pulita"   // pulizia fatta (non si rimette)
)

// Voce: una modifica nel file di stato.
type Voce struct {
	ID     string     `json:"id"`
	Stato  string     `json:"stato"`
	Motivo string     `json:"motivo,omitempty"`
	Quando string     `json:"quando,omitempty"` // RFC3339
	Prima  Precedente `json:"prima"`
}

// Attiva: la voce ha un cambio di Tasso da rimettere.
func (v Voce) Attiva() bool { return v.Stato == VoceFatta || v.Stato == VoceInCorso }

// Misura: i numeri di una misura (prima o dopo).
type Misura struct {
	Quando     string         `json:"quando,omitempty"`
	FPSMedi    float64        `json:"fpsMedi,omitempty"`
	FPSBassi   float64        `json:"fpsBassi,omitempty"` // 1%: media del centesimo di fotogrammi più lenti
	FPSOk      bool           `json:"fpsOk,omitempty"`
	FPSMotivo  string         `json:"fpsMotivo,omitempty"`
	Voto       int            `json:"voto,omitempty"`
	Leggerezza int            `json:"leggerezza,omitempty"`
	Accensione int            `json:"accensioneMs,omitempty"` // tempo di accensione letto da Windows
	AccQuando  string         `json:"accQuando,omitempty"`    // quando è stata quella accensione
	AccMotivo  string         `json:"accMotivo,omitempty"`
	Apertura   map[string]int `json:"aperturaMs,omitempty"` // tempo di apertura di ogni programma
}

// Componente: una riga del biglietto (nome del pezzo e voto da 1 a 100).
type Componente struct {
	Chiave string `json:"chiave"`
	Nome   string `json:"nome"`
	Voto   int    `json:"voto"`
}

// Risultato: l'ultimo Ottimizza tutto di una modalità.
type Risultato struct {
	Modo        string       `json:"modo"`
	Quando      string       `json:"quando"`
	Prima       Misura       `json:"prima"`
	Dopo        *Misura      `json:"dopo,omitempty"`
	Componenti  []Componente `json:"componenti,omitempty"`
	Fatte       int          `json:"fatte"`  // cambiate e si rimettono com'erano
	Pulite      int          `json:"pulite"` // pulizie fatte (non si rimettono)
	Totale      int          `json:"totale"`
	AccInAttesa bool         `json:"accInAttesa,omitempty"` // Ufficio: l'accensione "dopo" arriva alla prossima riaccensione
	Fermato     bool         `json:"fermato,omitempty"`
}

// Stato: il file per utente in Programmi\Tasso\lavoro\ottimizza.
type Stato struct {
	Versione  int                   `json:"versione"`
	Voci      []Voce                `json:"voci"`
	Risultati map[string]*Risultato `json:"risultati,omitempty"`
}

// VersioneStato: la versione del file.
const VersioneStato = 1

// Voce: la voce con questo id (nil se non c'è).
func (s *Stato) Voce(id string) *Voce {
	for i := range s.Voci {
		if s.Voci[i].ID == id {
			return &s.Voci[i]
		}
	}
	return nil
}

// Metti: sostituisce (o aggiunge) la voce con lo stesso id.
func (s *Stato) Metti(v Voce) {
	if x := s.Voce(v.ID); x != nil {
		*x = v
		return
	}
	s.Voci = append(s.Voci, v)
}

// Attive: gli id delle voci con un cambio di Tasso da rimettere.
func (s *Stato) Attive() []string {
	var out []string
	for _, v := range s.Voci {
		if v.Attiva() {
			out = append(out, v.ID)
		}
	}
	return out
}

// Codifica: il file da scrivere (sempre JSON valido, ordinato).
func (s *Stato) Codifica() ([]byte, error) {
	if s.Versione == 0 {
		s.Versione = VersioneStato
	}
	sort.SliceStable(s.Voci, func(i, j int) bool { return s.Voci[i].ID < s.Voci[j].ID })
	return json.MarshalIndent(s, "", " ")
}

var ErrStatoRovinato = errors.New("file di stato illeggibile")

// Leggi: legge il file di stato e tiene solo quello che le modifiche di Tasso possono aver scritto
// davvero (Valida di ogni modifica). Il file sta in una cartella dove scrivono solo gli
// amministratori, ma se Tasso gira senza permessi usa la sua cartella dell'utente: admin=false
// scarta i valori di tutto il PC. scartate: quante voci attive non si possono usare.
func Leggi(b []byte, admin bool) (*Stato, int, error) {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	var in Stato
	if err := json.Unmarshal(b, &in); err != nil {
		return nil, 0, ErrStatoRovinato
	}
	out := &Stato{Versione: VersioneStato, Risultati: map[string]*Risultato{}}
	scartate := 0
	visti := map[string]bool{}
	for _, v := range in.Voci {
		m, ok := Trova(v.ID)
		if !ok || visti[v.ID] {
			if v.Attiva() {
				scartate++
			}
			continue
		}
		visti[v.ID] = true
		pulita := Voce{ID: v.ID, Stato: v.Stato, Motivo: limitaTesto(v.Motivo, 300)}
		if _, err := time.Parse(time.RFC3339, v.Quando); err == nil {
			pulita.Quando = v.Quando
		}
		switch v.Stato {
		case VoceInCorso, VoceFatta:
			p, ok := m.G.Valida(v.Prima, admin)
			if !ok || p.Vuoto && !m.NonRimette {
				scartate++
				continue
			}
			pulita.Prima = p
		case VoceAPosto, VoceSaltata, VoceErrore, VoceRimessa, VoceLasciata, VocePulita:
		default:
			continue
		}
		out.Voci = append(out.Voci, pulita)
	}
	for _, modo := range []string{ModoGioco, ModoUfficio} {
		if r := in.Risultati[modo]; r != nil && r.Modo == modo {
			out.Risultati[modo] = pulisciRisultato(r)
		}
	}
	return out, scartate, nil
}

func limitaTesto(s string, n int) string {
	if !testoSicuro(s, n) {
		return ""
	}
	return s
}

// pulisciRisultato: i numeri restano numeri ragionevoli, i testi corti.
func pulisciRisultato(r *Risultato) *Risultato {
	m := func(x Misura) Misura {
		if x.FPSMedi < 0 || x.FPSMedi > 100000 || x.FPSBassi < 0 || x.FPSBassi > 100000 {
			x.FPSMedi, x.FPSBassi, x.FPSOk = 0, 0, false
		}
		if x.Voto < 0 || x.Voto > 100 {
			x.Voto = 0
		}
		if x.Leggerezza < 0 || x.Leggerezza > 100 {
			x.Leggerezza = 0
		}
		if x.Accensione < 0 || x.Accensione > 3600000 {
			x.Accensione = 0
		}
		x.FPSMotivo, x.AccMotivo = limitaTesto(x.FPSMotivo, 300), limitaTesto(x.AccMotivo, 300)
		x.Quando, x.AccQuando = limitaTesto(x.Quando, 40), limitaTesto(x.AccQuando, 40)
		ap := map[string]int{}
		for k, v := range x.Apertura {
			if (k == "esplora" || k == "blocconote" || k == "browser") && v > 0 && v < 120000 {
				ap[k] = v
			}
		}
		x.Apertura = ap
		return x
	}
	out := &Risultato{Modo: r.Modo, Quando: limitaTesto(r.Quando, 40), Prima: m(r.Prima), AccInAttesa: r.AccInAttesa, Fermato: r.Fermato}
	if r.Dopo != nil {
		d := m(*r.Dopo)
		out.Dopo = &d
	}
	if r.Fatte >= 0 && r.Fatte <= 50 && r.Totale >= 0 && r.Totale <= 50 && r.Pulite >= 0 && r.Pulite <= 10 {
		out.Fatte, out.Totale, out.Pulite = r.Fatte, r.Totale, r.Pulite
	}
	for i, c := range r.Componenti {
		if i >= 8 {
			break
		}
		if c.Voto >= 0 && c.Voto <= 100 {
			out.Componenti = append(out.Componenti, Componente{Chiave: limitaTesto(c.Chiave, 30), Nome: limitaTesto(c.Nome, 120), Voto: c.Voto})
		}
	}
	return out
}

// Unisci: le voci del file dell'utente (un giro fatto con Tasso senza permessi, letto con
// admin=false: solo impostazioni dell'utente) dentro lo stato del file protetto. Una voce entra se
// nel file protetto non c'è già una voce attiva con lo stesso id; i risultati che mancano pure.
// Restituisce quante voci attive sono entrate. Dopo, il file dell'utente va tolto: lo stato vero
// sta tutto nel file protetto (così "Togli Tasso" non trova più voci vecchie da nessun'altra parte).
func (s *Stato) Unisci(u *Stato) int {
	if u == nil {
		return 0
	}
	n := 0
	for _, v := range u.Voci {
		if old := s.Voce(v.ID); old != nil && (old.Attiva() || !v.Attiva()) {
			continue
		}
		s.Metti(v)
		if v.Attiva() {
			n++
		}
	}
	if s.Risultati == nil {
		s.Risultati = map[string]*Risultato{}
	}
	for k, r := range u.Risultati {
		if s.Risultati[k] == nil {
			s.Risultati[k] = r
		}
	}
	return n
}
