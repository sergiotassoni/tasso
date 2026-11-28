package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// RECENSIONE-3, R3-3: con un ripristino non riuscito dal disinstallatore, i valori di com'era
// vanno in un file per le copie di sicurezza prima di cancellarsi (niente segreti dentro).
func TestSalvaNonRimesse(t *testing.T) {
	app, _ := preparaApp(t)
	impostazioniPulite(t)
	settings.Snapshot = json.RawMessage(`{"plan":"381b4222-f694-41f0-9685-ff5bb260df2e"}`)
	settings.StartupOrig = map[string]bool{"HKCU|Programma": true}
	settings.DiscordWebhook = hookSegreto
	if !salvaNonRimesse([]string{"Il piano energetico non è tornato."}) {
		t.Fatal("file non scritto")
	}
	l, _ := filepath.Glob(filepath.Join(app, "impostazioni-non-rimesse", "impostazioni-di-prima_*.json"))
	if len(l) != 1 {
		t.Fatalf("%v", l)
	}
	b, _ := os.ReadFile(l[0])
	for _, c := range []string{"381b4222", "HKCU|Programma", "Il piano energetico"} {
		if !strings.Contains(string(b), c) {
			t.Fatalf("manca %q: %s", c, b)
		}
	}
	if strings.Contains(string(b), tokenSegreto) {
		t.Fatal("il webhook è finito nella copia")
	}
	if _, err := os.Stat(filepath.Join(app, "impostazioni-non-rimesse", "LEGGIMI.txt")); err != nil {
		t.Fatal(err)
	}
}

// La cartella è una giunzione: non scrivo (Tasso è amministratore).
func TestSalvaNonRimesseGiunzione(t *testing.T) {
	app, fuori := preparaApp(t)
	impostazioniPulite(t)
	link(t, filepath.Dir(fuori), filepath.Join(app, "impostazioni-non-rimesse"))
	if salvaNonRimesse(nil) {
		t.Fatal("scritto attraverso la giunzione")
	}
	restaOriginale(t, fuori)
}
