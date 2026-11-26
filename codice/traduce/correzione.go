package traduce

// Correzione in sottofondo: Mozilla mostra subito la traduzione; l'IA (se c'è e se il PC regge)
// la ricontrolla dopo, una frase alla volta, e salva la versione corretta nella libreria del gioco.
// La volta dopo che la frase compare, si vede già corretta. Prova 1000 frasi: Mozilla 68,1 → 70,2.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type lavoroCorrezione struct {
	chiave, battuta, bozza string
	glossario              []voceTrovata
	schermata              []string // le altre scritte della schermata: l'IA capisce di cosa si parla
	da, a                  string   // coppia di lingue se non è inglese → italiano
}

type correttoreSfondo struct {
	mu       sync.Mutex
	motore   Motore // nil = spento
	coda     chan lavoroCorrezione
	inCoda   map[string]bool
	daParte  []lavoroCorrezione                    // frasi arrivate col correttore spento (riavvio): tornano in coda quando si riaccende
	viste    map[string]bool                       // frasi già ricontrollate dall'IA in questa sessione
	errori   map[string]int                        // frasi per cui l'IA ha dato errore (dopo 3 basta)
	inPausa  func() bool                           // vero quando il gioco ha bisogno del PC (processore pieno)
	quando   func(inglese, bozza, corretta string) // avvisata quando una frase è stata migliorata
	fatte    atomic.Int64
	cambiate atomic.Int64
	avvio    sync.Once
}

const maxCodaCorrezioni = 300

// ImpostaCorrettore accende (m != nil) o spegne (m == nil) la correzione in sottofondo.
func (t *Traduttore) ImpostaCorrettore(m Motore, inPausa func() bool) {
	c := &t.corr
	c.mu.Lock()
	c.motore, c.inPausa = m, inPausa
	if c.coda == nil {
		c.coda = make(chan lavoroCorrezione, maxCodaCorrezioni)
		c.inCoda = map[string]bool{}
		c.viste = map[string]bool{}
		c.errori = map[string]int{}
	}
	if m != nil && len(c.daParte) > 0 {
		// il correttore si è riacceso: le frasi messe da parte tornano in coda
		for _, l := range c.daParte {
			select {
			case c.coda <- l:
			default:
				delete(c.inCoda, l.chiave) // coda piena: la frase resta com'è
			}
		}
		c.daParte = nil
	}
	c.mu.Unlock()
	if m != nil {
		c.avvio.Do(func() { go t.lavoraCorrezioni() })
	}
}

// QuandoCorretta: f viene chiamata ogni volta che l'IA migliora una frase, così la scritta già
// sullo schermo può cambiare subito invece di aspettare la volta dopo.
func (t *Traduttore) QuandoCorretta(f func(inglese, bozza, corretta string)) {
	t.corr.mu.Lock()
	t.corr.quando = f
	t.corr.mu.Unlock()
}

// accodaCorrezioneCoppia: come accodaCorrezione, per le lingue diverse da inglese → italiano.
func (t *Traduttore) accodaCorrezioneCoppia(chiave, testo, bozza, da, a string) {
	c := &t.corr
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.motore == nil || c.coda == nil || c.inCoda[chiave] || !CorrettoreCoppiaAttivo {
		return
	}
	select {
	case c.coda <- lavoroCorrezione{chiave: chiave, battuta: testo, bozza: bozza, da: da, a: a}:
		c.inCoda[chiave] = true
	default:
	}
}

// CorrettoreCoppiaAttivo: il correttore IA anche per le lingue diverse da inglese → italiano
// (acceso solo se la prova sulle frasi FLORES dice che migliora).
var CorrettoreCoppiaAttivo = true

// accodaCorrezione: chiamata dopo una traduzione nuova di Mozilla. Se la coda è piena, la frase
// resta com'è (meglio una traduzione un po' meno precisa che rallentare il gioco).
func (t *Traduttore) accodaCorrezione(chiave, battuta, bozza string, g []voceTrovata, schermata []string) {
	c := &t.corr
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.motore == nil || c.coda == nil || c.inCoda[chiave] || c.viste[chiave] {
		return
	}
	select {
	case c.coda <- lavoroCorrezione{chiave: chiave, battuta: battuta, bozza: bozza, glossario: g, schermata: schermata}:
		c.inCoda[chiave] = true
	default:
	}
}

func (t *Traduttore) lavoraCorrezioni() {
	for l := range t.corr.coda {
		t.unaCorrezione(l)
	}
}

// esito di una correzione: serve per sapere se la frase è stata davvero vista dall'IA
const (
	nonProvata   = iota // motore spento (es. cambio di livello dell'IA): la frase tornerà in coda
	messaDaParte        // motore spento: la frase aspetta da parte che il correttore si riaccenda
	conErrore           // il motore c'era ma non ha risposto (tempo scaduto...): riprovo, al massimo 3 volte
	risposta            // l'IA ha risposto (corretta, uguale o scartata): non serve rifarla
)

// errRispostaScartata: l'IA ha risposto, ma la risposta non va bene (tagliata, copiata da un
// esempio). Ripetere darebbe lo stesso risultato: la frase conta come vista.
var errRispostaScartata = errors.New("motore IA: risposta scartata")

// errMotoreSpento: il motore IA non è acceso (spento o in riavvio): la frase non conta come vista.
var errMotoreSpento = errors.New("motore IA spento")

// unaCorrezione: un lavoro della coda. Un errore qui non ferma il correttore (prima bastava un
// errore per bloccare la coda per sempre). La frase si segna "già vista" solo se l'IA ha
// risposto. Se il correttore è spento (riavvio dell'IA, cambio di livello) la frase non si
// butta: va da parte e torna in coda quando il correttore si riaccende (prima la coda di 300
// frasi si svuotava in un attimo a ogni riavvio).
func (t *Traduttore) unaCorrezione(l lavoroCorrezione) {
	esito := nonProvata
	defer func() {
		if recover() != nil {
			esito = risposta // una frase che fa cadere il correttore non la riprovo
		}
		c := &t.corr
		if esito == messaDaParte {
			return // resta "in coda": è nelle frasi da parte
		}
		c.mu.Lock()
		delete(c.inCoda, l.chiave)
		switch esito {
		case risposta:
			c.viste[l.chiave] = true
			delete(c.errori, l.chiave)
		case conErrore:
			if c.errori[l.chiave]++; c.errori[l.chiave] >= 3 {
				c.viste[l.chiave] = true
				delete(c.errori, l.chiave)
			}
		}
		c.mu.Unlock()
	}()
	spente := 0
	for {
		t.corr.mu.Lock()
		m, pausa := t.corr.motore, t.corr.inPausa
		if m == nil {
			// controllato col lucchetto: se il correttore si riaccende adesso, ImpostaCorrettore
			// trova la frase già da parte e la rimette in coda
			if len(t.corr.daParte) < maxCodaCorrezioni {
				t.corr.daParte = append(t.corr.daParte, l)
				esito = messaDaParte
			}
			t.corr.mu.Unlock()
			return
		}
		t.corr.mu.Unlock()
		if pausa != nil && pausa() {
			time.Sleep(2 * time.Second)
			continue
		}
		esito = t.correggiUna(m, l)
		if esito == nonProvata && spente < 15 {
			// il motore si è appena spento ma il correttore non ancora (succede durante un
			// riavvio): riprovo tra poco, e se intanto si spegne la frase va da parte
			spente++
			time.Sleep(2 * time.Second)
			continue
		}
		return
	}
}

func (t *Traduttore) correggiUna(m Motore, l lavoroCorrezione) int {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	corr, err := m.Traduci(ctx, RichiestaTraduzione{Testo: l.battuta, Glossario: l.glossario, Schermata: l.schermata, Bozza: l.bozza, Da: l.da, A: l.a})
	switch {
	case errors.Is(err, errMotoreSpento):
		return nonProvata
	case errors.Is(err, errRispostaScartata):
		t.corr.fatte.Add(1)
		return risposta
	case err != nil:
		t.corr.fatte.Add(1)
		return conErrore
	}
	t.corr.fatte.Add(1)
	if l.da != "" {
		// altre lingue: niente regole fatte per l'italiano
		corr = pulisciRisposta(corr)
		if corr == l.bozza || !correzioneAccettabileCoppia(l.battuta, l.bozza, corr) {
			return risposta
		}
		t.corr.cambiate.Add(1)
		t.mu.Lock()
		if v, ok := t.altreCoppie[l.chiave]; ok && v == l.bozza {
			t.altreCoppie[l.chiave] = corr
		}
		t.mu.Unlock()
		t.corr.mu.Lock()
		f := t.corr.quando
		t.corr.mu.Unlock()
		if f != nil {
			f(l.battuta, l.bozza, corr)
		}
		return risposta
	}
	corr = sigleItaliane(correggiTerminiFrase(l.battuta, corr), l.glossario)
	if corr == l.bozza || !correzioneAccettabile(l.battuta, l.bozza, corr) {
		return risposta
	}
	t.corr.cambiate.Add(1)
	t.mu.Lock()
	// solo se nel frattempo nessuno ha cambiato la frase (es. l'utente con il glossario)
	if v, ok := t.cache[l.chiave]; ok && v == l.bozza {
		t.mettiInCacheLocked(l.chiave, corr, true)
	}
	if t.libreria != nil {
		if v, ok := t.libreria[l.chiave]; ok && v == l.bozza {
			t.mettiInLibreriaLocked(l.chiave, corr)
		}
	}
	if t.corretteIA == nil {
		t.corretteIA = map[string]bool{}
	}
	t.corretteIA[l.chiave] = true
	t.mu.Unlock()
	t.corr.mu.Lock()
	f := t.corr.quando
	t.corr.mu.Unlock()
	if f != nil {
		f(l.battuta, l.bozza, corr)
	}
	return risposta
}

// stessaFraseInglese: la frase inglese del dialogo mostrato (originale, con l'eventuale "Nome:"
// davanti e gli a capo) è quella corretta dall'IA? Confrontare solo la bozza non basta: "Yes." e
// "Yeah." hanno tutte e due la bozza "Sì." e la correzione di una finiva sull'altra.
func stessaFraseInglese(originale, inglese string) bool {
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	e := norm(inglese)
	if e == "" {
		return false
	}
	if norm(originale) == e {
		return true
	}
	_, battuta := separaParlante(originale)
	return norm(battuta) == e
}

// StatoCorrezioni: frasi ricontrollate e frasi migliorate dall'avvio.
func (t *Traduttore) StatoCorrezioni() (fatte, cambiate int64, inAttesa int) {
	t.corr.mu.Lock()
	n := len(t.corr.inCoda)
	t.corr.mu.Unlock()
	return t.corr.fatte.Load(), t.corr.cambiate.Load(), n
}
