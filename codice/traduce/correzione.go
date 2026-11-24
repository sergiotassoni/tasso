package traduce

// Correzione in sottofondo: Mozilla mostra subito la traduzione; l'IA (se c'è e se il PC regge)
// la ricontrolla dopo, una frase alla volta, e salva la versione corretta nella libreria del gioco.
// La volta dopo che la frase compare, si vede già corretta. Prova 1000 frasi: Mozilla 68,1 → 70,2.

import (
	"context"
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
	viste    map[string]bool                       // frasi già ricontrollate dall'IA in questa sessione
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

// accodaCorrezione: chiamata dopo una traduzione nuova di Mozilla. Se la coda è piena, la frase
// resta com'è (meglio una traduzione un po' meno precisa che rallentare il gioco).
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

// unaCorrezione: un lavoro della coda. Un errore qui non ferma il correttore (prima bastava un
// errore per bloccare la coda per sempre).
func (t *Traduttore) unaCorrezione(l lavoroCorrezione) {
	defer func() {
		_ = recover()
		t.corr.mu.Lock()
		delete(t.corr.inCoda, l.chiave)
		t.corr.viste[l.chiave] = true
		t.corr.mu.Unlock()
	}()
	for {
		t.corr.mu.Lock()
		m, pausa := t.corr.motore, t.corr.inPausa
		t.corr.mu.Unlock()
		if m == nil {
			return
		}
		if pausa != nil && pausa() {
			time.Sleep(2 * time.Second)
			continue
		}
		t.correggiUna(m, l)
		return
	}
}

func (t *Traduttore) correggiUna(m Motore, l lavoroCorrezione) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	defer t.corr.fatte.Add(1)
	corr, err := m.Traduci(ctx, RichiestaTraduzione{Testo: l.battuta, Glossario: l.glossario, Schermata: l.schermata, Bozza: l.bozza, Da: l.da, A: l.a})
	if err != nil {
		return
	}
	if l.da != "" {
		// altre lingue: niente regole fatte per l'italiano
		corr = pulisciRisposta(corr)
		if corr == l.bozza || !correzioneAccettabileCoppia(l.battuta, l.bozza, corr) {
			return
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
		return
	}
	corr = sigleItaliane(correggiTerminiFrase(l.battuta, corr), l.glossario)
	if corr == l.bozza || !correzioneAccettabile(l.battuta, l.bozza, corr) {
		return
	}
	t.corr.cambiate.Add(1)
	t.mu.Lock()
	// solo se nel frattempo nessuno ha cambiato la frase (es. l'utente con il glossario)
	if v, ok := t.cache[l.chiave]; ok && v == l.bozza {
		t.cache[l.chiave] = corr
	}
	if t.libreria != nil {
		if v, ok := t.libreria[l.chiave]; ok && v == l.bozza {
			t.libreria[l.chiave] = corr
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
}

// StatoCorrezioni: frasi ricontrollate e frasi migliorate dall'avvio.
func (t *Traduttore) StatoCorrezioni() (fatte, cambiate int64, inAttesa int) {
	t.corr.mu.Lock()
	n := len(t.corr.inCoda)
	t.corr.mu.Unlock()
	return t.corr.fatte.Load(), t.corr.cambiate.Load(), n
}
