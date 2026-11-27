// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

//go:build windows

package main

// "Il tuo nome" (Ecosistema Tasso 3.2): lo stesso nome di Tasso e Tasso PDF, in
// HKCU\Software\Tasso, valore "Nome" (REG_SZ). Tasso Zip gira come l'utente collegato, quindi
// HKCU è già quello giusto. Solo sul PC, mai in rete. Le regole sono in nome/.

import (
	"sync"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows/registry"

	"tassozip/nome"
)

const chiaveTasso = `Software\Tasso`

var nomeMu sync.Mutex

type archivioNome struct{}

func (archivioNome) Leggi() (string, bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, chiaveTasso, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer k.Close()
	v, tipo, err := k.GetStringValue("Nome")
	if err != nil || tipo != registry.SZ {
		return "", false
	}
	return v, true
}

func (archivioNome) Scrivi(n string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, chiaveTasso, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue("Nome", n)
}

func (archivioNome) Togli() error { return nil } // lo toglie "Togli Tasso dal PC" o il disinstallatore

// leggiNome: il nome salvato, "" se non c'è o non è valido.
func leggiNome() string {
	nomeMu.Lock()
	defer nomeMu.Unlock()
	return nome.Leggi(archivioNome{})
}

func collegaNome(w webview2.WebView) {
	_ = w.Bind("goNome", func() string { return leggiNome() })
	_ = w.Bind("goNomeSalva", func(scritto string) map[string]interface{} {
		nomeMu.Lock()
		n, e := nome.Salva(archivioNome{}, scritto)
		nomeMu.Unlock()
		if e.Codice != nome.OK {
			return map[string]interface{}{"ok": false, "errore": e.Codice, "simbolo": e.Simbolo}
		}
		return map[string]interface{}{"ok": true, "nome": n}
	})
}
