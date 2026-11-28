package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Sicurezza (collaudo sul PC, 3.1): settings.json teneva in chiaro l'indirizzo del webhook di
// Discord, con il suo token. Dalla 3.2 su disco c'è solo la versione protetta con DPAPI (come il
// token del bot e il Portachiavi), il file vecchio si sistema al primo avvio e la finestra vede
// l'indirizzo senza il token. Gira su Windows (DPAPI vera).

const hookSegreto = "https://discord.com/api/webhooks/123456789012345678/AbCdEfGhIjKlMnOpQrStUvWxYz0123456789_-abcdefghijklmnopqrstuv"
const tokenSegreto = "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789"

func impostazioniPulite(t *testing.T) {
	t.Helper()
	vecchie, ro := settings, settingsReadOnly
	settings, settingsReadOnly = Settings{Mode: "normale"}, false
	t.Cleanup(func() { settings, settingsReadOnly = vecchie, ro })
}

func nessunTokenSuDisco(t *testing.T, app string) {
	t.Helper()
	_ = filepath.Walk(app, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if b, _ := os.ReadFile(p); strings.Contains(string(b), tokenSegreto) {
			t.Errorf("token del webhook in chiaro in %s", p)
		}
		return nil
	})
}

func TestWebhookVecchioProtettoAlPrimoAvvio(t *testing.T) {
	app, _ := preparaApp(t)
	impostazioniPulite(t)
	vecchio := `{"onboarded":true,"profile":"gioco","mode":"normale","theme":"tasso","levelsV":5,"discordWebhook":"` + hookSegreto + `","discordOn":true}`
	if err := os.WriteFile(filepath.Join(app, "settings.json"), []byte(vecchio), 0o644); err != nil {
		t.Fatal(err)
	}
	// una copia fatta da una versione di prima (.prima) con lo stesso testo in chiaro
	if err := os.WriteFile(filepath.Join(app, "settings.json.prima"), []byte(vecchio), 0o644); err != nil {
		t.Fatal(err)
	}
	loadSettings()
	if settings.DiscordWebhook != hookSegreto || !settings.DiscordOn {
		t.Fatalf("webhook perso nella migrazione: %q %v", settings.DiscordWebhook, settings.DiscordOn)
	}
	b, err := os.ReadFile(filepath.Join(app, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var su map[string]interface{}
	if err := json.Unmarshal(b, &su); err != nil {
		t.Fatal(err)
	}
	if enc, _ := su["discordWebhookEnc"].(string); enc == "" || unprotectString(enc) != hookSegreto {
		t.Fatalf("webhook non protetto con DPAPI: %v", su["discordWebhookEnc"])
	}
	if _, c := su["discordWebhook"]; c {
		t.Fatal("resta la chiave in chiaro")
	}
	nessunTokenSuDisco(t, app)

	// riaprendo Tasso il webhook protetto si rilegge
	settings = Settings{Mode: "normale"}
	loadSettings()
	if settings.DiscordWebhook != hookSegreto {
		t.Fatalf("webhook protetto non riletto: %q", settings.DiscordWebhook)
	}
	// la finestra non vede il token, ma con l'indirizzo mascherato spegne e riaccende lo stesso webhook
	m := getSettings()
	if s, _ := m["discordWebhook"].(string); s == "" || strings.Contains(s, tokenSegreto) {
		t.Fatalf("la finestra vede il token: %q", s)
	}
	if _, c := m["discordWebhookEnc"]; c {
		t.Fatal("la versione protetta va alla finestra")
	}
	if r := setDiscord(m["discordWebhook"].(string), false); r != "ok" || settings.DiscordWebhook != hookSegreto || settings.DiscordOn {
		t.Fatalf("spegni con l'indirizzo mascherato: %q %q %v", r, settings.DiscordWebhook, settings.DiscordOn)
	}
	nessunTokenSuDisco(t, app)
}

func TestSetDiscordSalvaProtetto(t *testing.T) {
	app, _ := preparaApp(t)
	impostazioniPulite(t)
	if r := setDiscord(hookSegreto, true); r != "ok" {
		t.Fatal(r)
	}
	nessunTokenSuDisco(t, app)
	if r := setDiscord("", false); r != "ok" || settings.DiscordWebhook != "" || settings.DiscordWebhookEnc != "" {
		t.Fatalf("webhook non tolto: %q %+v", r, settings.DiscordWebhookEnc)
	}
	// un indirizzo sbagliato non torna nel messaggio
	if r := setDiscord("https://example.com/"+tokenSegreto, true); r == "ok" || strings.Contains(r, tokenSegreto) {
		t.Fatalf("%q", r)
	}
}
