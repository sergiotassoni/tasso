package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// 3.2: Salvia e Lavanda non si scelgono più. Chi le aveva passa a Carta e Perla, e il cambio
// finisce in settings.json (al prossimo avvio non c'è più niente da cambiare).
func TestTemiUfficioRitiratiMigrati(t *testing.T) {
	app, _ := preparaApp(t)
	vecchie, vecchieRO := settings, settingsReadOnly
	t.Cleanup(func() { settings, settingsReadOnly = vecchie, vecchieRO })
	for _, c := range []struct{ prima, dopo string }{
		{"salvia", "carta"}, {"lavanda", "perla"},
		{"grafite", "grafite"}, {"tortora", "tortora"}, {"carta", "carta"}, {"", ""},
		{"notte", "notte"}, // valida, non si sceglie: in Ufficio si vede Azzurro (come prima)
	} {
		b, _ := json.Marshal(map[string]interface{}{"mode": "normale", "levelsV": 5, "profile": "ufficio", "theme": "oro", "themeOffice": c.prima})
		if err := os.WriteFile(filepath.Join(app, "settings.json"), b, 0o644); err != nil {
			t.Fatal(err)
		}
		settings, settingsReadOnly = Settings{}, false
		loadSettings()
		if settings.ThemeOffice != c.dopo || settings.Theme != "oro" {
			t.Errorf("%q: in memoria %q (Gaming %q), voglio %q", c.prima, settings.ThemeOffice, settings.Theme, c.dopo)
		}
		var su map[string]interface{}
		b, _ = os.ReadFile(filepath.Join(app, "settings.json"))
		if json.Unmarshal(b, &su) != nil || su["themeOffice"] != c.dopo {
			t.Errorf("%q: nel file %v, voglio %q", c.prima, su["themeOffice"], c.dopo)
		}
		if fixSettingsDefaults() {
			t.Errorf("%q: secondo giro ancora da cambiare", c.prima)
		}
	}
	// salvia e lavanda restano valori accettati (impostazioni vecchie, setTheme da un'interfaccia vecchia)
	for _, id := range []string{"salvia", "lavanda", "notte", "grafite", "bosco", "prugna", "petrolio", "tortora", "ardesia"} {
		if !validThemes[id] {
			t.Errorf("%s non è in validThemes", id)
		}
	}
}
