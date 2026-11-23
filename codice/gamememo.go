package main

// 3.1 "Prepara il gioco a passi": Tasso si ricorda le scelte fatte per ogni gioco (lingua,
// traduzione, potenza, impostazioni consigliate) e la volta dopo le propone già pronte.

import (
	"strings"
	"tasso/traduce"
	"time"
	"unicode/utf8"
)

type GameMemo struct {
	Da    string `json:"da,omitempty"`    // lingua del gioco ("auto", "en"…)
	A     string `json:"a,omitempty"`     // lingua voluta
	Modo  string `json:"modo,omitempty"`  // "schermo" o "file"
	IA    bool   `json:"ia,omitempty"`    // traduzione più precisa con l'IA
	Skip  bool   `json:"skip,omitempty"`  // "Non serve tradurre"
	Pot   string `json:"pot,omitempty"`   // "normale", "gaming", "max"
	Auto  bool   `json:"auto,omitempty"`  // Boost automatico per questo gioco
	NoLim bool   `json:"nolim,omitempty"` // nessun limite agli FPS
	When  string `json:"when,omitempty"`  // data dell'ultima volta (2006-01-02)
}

// cleanMemo tiene solo valori conosciuti: il file delle impostazioni non si riempie di testo a caso.
func cleanMemo(m GameMemo) GameMemo {
	if m.Da != "auto" && !traduce.LinguaValida(m.Da) {
		m.Da = ""
	}
	if !traduce.LinguaValida(m.A) {
		m.A = ""
	}
	if m.Modo != "schermo" && m.Modo != "file" {
		m.Modo = ""
	}
	if m.Pot != "normale" && m.Pot != "gaming" && m.Pot != "max" {
		m.Pot = ""
	}
	m.When = time.Now().Format("2006-01-02")
	return m
}

// setGameMemo salva le scelte di un gioco della libreria (al massimo 500 giochi).
func setGameMemo(id string, m GameMemo) string {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 300 || !utf8.ValidString(id) {
		return "Gioco non valido."
	}
	libMu.Lock()
	_, ok := library[id]
	libMu.Unlock()
	if !ok {
		return "Gioco non trovato: aggiorna la libreria."
	}
	c := cleanMemo(m)
	updateSettings(func(s *Settings) {
		if memoEmpty(c) {
			// Niente da ricordare (né traduzione né potenza): non salvo un ricordo vuoto.
			delete(s.GameMemo, id)
			return
		}
		if s.GameMemo == nil {
			s.GameMemo = map[string]*GameMemo{}
		}
		if _, gia := s.GameMemo[id]; !gia {
			// Al massimo 500 giochi: faccio spazio togliendo quelli ricordati da più tempo.
			for len(s.GameMemo) >= 500 {
				delete(s.GameMemo, oldestMemo(s.GameMemo))
			}
		}
		s.GameMemo[id] = &c
	})
	return "ok"
}

// memoEmpty: il ricordo non dice niente da mostrare o da rimettere.
func memoEmpty(m GameMemo) bool {
	return !m.Skip && m.A == "" && m.Pot == "" && !m.Auto && !m.NoLim
}

// oldestMemo: il gioco ricordato da più tempo (a pari data, il primo in ordine di nome).
func oldestMemo(all map[string]*GameMemo) string {
	old, when := "", ""
	for id, m := range all {
		w := ""
		if m != nil {
			w = m.When
		}
		if old == "" || w < when || (w == when && id < old) {
			old, when = id, w
		}
	}
	return old
}

// gameTrInfo prepara la traduzione per un gioco della libreria di Tasso (cartella e avvio).
func gameTrInfo(id string) map[string]any {
	libMu.Lock()
	g, ok := library[id]
	libMu.Unlock()
	if !ok {
		return map[string]any{"errore": "Gioco non trovato: aggiorna la libreria."}
	}
	return traduce.InfoGioco(g.Name, g.Store, g.dir, g.launch)
}
