// Correzioni finali C (CTRL-FIN-07 e CTRL-FIN-10). Gira anche su Linux: go test ./i18n/
package i18n

import (
	"os"
	"strings"
	"testing"
)

// F07-3 e F07-22: la finestra riconosce le frasi del backup confrontandole con la loro traduzione
// (TG): devono essere uguali negli script, in index.html e nella sezione "ps" di ogni lingua.
func TestFrasiBackupRiconosciute(t *testing.T) {
	html, err := os.ReadFile("../ui/index.html")
	if err != nil {
		t.Skip("index.html non trovato")
	}
	steps, _ := os.ReadFile("../steps.go")
	frasi := []string{
		"Backup creato. Se qualcosa non ti piace puoi tornare indietro da Ripristino configurazione di sistema.",
		"Uso il backup di poco fa (",
		"Protezione sistema era spenta su questo PC: l'ho accesa per poter fare il backup (usa un po' di spazio del disco). Se la vuoi spenta: Pannello di controllo > Sistema > Protezione sistema > Configura > Disattiva.",
	}
	for _, f := range frasi {
		if !strings.Contains(string(html), "'"+strings.ReplaceAll(f, "'", `\'`)+"'") {
			t.Errorf("index.html non ha la frase %q", f)
		}
		if !strings.Contains(string(steps), "'"+strings.ReplaceAll(f, "'", "''")) {
			t.Errorf("steps.go non dice più %q: la finestra non la riconoscerebbe", f)
		}
		for _, l := range append([]string{"en"}, lingue...) {
			if _, ok := leggi(t, l)["ps"][f]; !ok {
				t.Errorf("%s.json/ps: manca %q", l, f)
			}
		}
	}
}

// F10-2: gli script dell'Ufficio non leggono le scritte di Windows in italiano o inglese
// (su Windows in spagnolo, francese o tedesco davano messaggi falsi).
func TestUfficioSenzaScritteDiWindows(t *testing.T) {
	var tutto string
	for _, f := range []string{"../steps_office.go", "../steps_office2.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Skip(f + " non trovato")
		}
		tutto += string(b)
	}
	for _, s := range []string{"(?:Destinazione|Target)", "(?:Segnale|Signal)", "(Nessuno|None)", "Scadenza password|Password expires",
		"(?:Server proxy|Proxy Server)", "(?:Banda|Band)", "Tipo radio|Tipo di radio|Radio type", "Crittografia completata", "^(NONE|NESSUNO)$"} {
		if strings.Contains(tutto, s) {
			t.Errorf("uno script dell'Ufficio cerca ancora %q nelle scritte di Windows", s)
		}
	}
	// F10-4: i servizi spenti si riaccendono solo dopo aver salvato com'erano.
	for _, s := range []string{"Save-ServiceStart 'WSearch'", "Save-ServiceStart 'W32Time'", "Save-ServiceStart 'stisvc'"} {
		if !strings.Contains(tutto, s) {
			t.Errorf("manca %q prima di riaccendere il servizio", s)
		}
	}
}
