// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

// "Il tuo nome" (Ecosistema Tasso 3.2): lo stesso nome di Tasso e Tasso Zip, in
// HKCU\Software\Tasso, valore "Nome" (REG_SZ), solo sul PC e mai in rete. Su Windows lo legge e
// lo scrive nome_windows.go; le regole sono in nome/.

import (
	"sync"

	"tassopdf/nome"
)

var (
	nomeMu      sync.Mutex
	archivioNom nome.Archivio // nil fuori da Windows: niente nome
)

// leggiNome: il nome salvato, "" se non c'è o non è valido.
func leggiNome() string {
	nomeMu.Lock()
	defer nomeMu.Unlock()
	if archivioNom == nil {
		return ""
	}
	return nome.Leggi(archivioNom)
}

// EsitoNome: la risposta di goNomeSalva alla pagina.
type EsitoNome struct {
	Ok      bool   `json:"ok"`
	Nome    string `json:"nome,omitempty"`
	Errore  string `json:"errore,omitempty"`
	Simbolo string `json:"simbolo,omitempty"`
}

func goNome() string { return leggiNome() }

func goNomeSalva(scritto string) EsitoNome {
	nomeMu.Lock()
	defer nomeMu.Unlock()
	if archivioNom == nil {
		return EsitoNome{Errore: nome.CodiceRegistro}
	}
	n, e := nome.Salva(archivioNom, scritto)
	if e.Codice != nome.OK {
		return EsitoNome{Errore: e.Codice, Simbolo: e.Simbolo}
	}
	return EsitoNome{Ok: true, Nome: n}
}
