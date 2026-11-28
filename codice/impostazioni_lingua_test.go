package main

import (
	"testing"
	"time"
)

// TestImpostazioniPoiLingua: il blocco riprodotto sul PC di Sergio. Impostazioni (goChanges, con un
// gioco regolato) prendeva setMu due volte e restava fermo per sempre; il clic su "English"
// (goSetLang → updateSettings) aspettava setMu: pulsanti delle lingue grigi, interfaccia in
// italiano, coda dei pulsanti ferma. Ora tutte e due finiscono subito, anche in inglese.
func TestImpostazioniPoiLingua(t *testing.T) {
	vecchioDir, vecchie := appDir, settings
	appDir = t.TempDir()
	t.Cleanup(func() {
		setMu.Lock()
		appDir, settings = vecchioDir, vecchie
		setMu.Unlock()
	})
	setMu.Lock()
	settings = Settings{Mode: "normale", Lang: "en", GameTunes: map[string]*GameTune{"g1": {Name: "Gioco di prova", Applied: "2026-11-27 20:00"}}}
	setMu.Unlock()
	entro := func(nome string, f func()) {
		t.Helper()
		fatto := make(chan struct{})
		go func() { defer close(fatto); f() }()
		select {
		case <-fatto:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s ferma da 5 secondi: setMu preso due volte", nome)
		}
	}
	var c map[string]interface{}
	entro("Impostazioni (changesList)", func() { c = changesList() })
	if c == nil {
		t.Fatal("changesList senza risposta")
	}
	entro("cambio di lingua (updateSettings)", func() { updateSettings(func(s *Settings) { s.Lang = "it" }) })
	entro("Portachiavi (vaultFillCfg)", func() { _ = vaultFillCfg() })
}
