package segreti

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

const hookProva = "https://discord.com/api/webhooks/123456789012345678/AbCdEfGhIjKlMnOpQrStUvWxYz0123456789_-abcdefghijklmnopqrstuv"

// finta: al posto di DPAPI (qui non c'è Windows), un testo che non contiene quello in chiaro.
func finta(s string) string { return "DPAPI:" + hex.EncodeToString([]byte(s)) }
func sblocca(s string) string {
	b, _ := hex.DecodeString(strings.TrimPrefix(s, "DPAPI:"))
	return string(b)
}

// Un settings.json della 3.1 con il webhook in chiaro: al primo avvio si protegge e il testo in
// chiaro sparisce dal file.
func TestMigraDaChiaro(t *testing.T) {
	chiaro, enc, cambiato := Migra(hookProva, "", finta, sblocca)
	if chiaro != hookProva || enc != finta(hookProva) || !cambiato {
		t.Fatalf("%q %q %v", chiaro, enc, cambiato)
	}
	// già protetto: niente da riscrivere, in memoria torna quello vero
	chiaro, enc2, cambiato := Migra("", enc, finta, sblocca)
	if chiaro != hookProva || enc2 != enc || cambiato {
		t.Fatalf("%q %q %v", chiaro, enc2, cambiato)
	}
	// Windows non riesce a proteggere: il file si riscrive lo stesso, senza il testo in chiaro
	chiaro, enc, cambiato = Migra(hookProva, "", func(string) string { return "" }, sblocca)
	if chiaro != hookProva || enc != "" || !cambiato {
		t.Fatalf("%q %q %v", chiaro, enc, cambiato)
	}
	if c, e, x := Migra("", "", finta, sblocca); c != "" || e != "" || x {
		t.Fatal("niente webhook: niente da fare")
	}
}

// La struttura delle impostazioni di Tasso: il webhook in memoria non va su disco, quello vecchio
// in chiaro si legge ma dopo la migrazione non si scrive più.
type impFinte struct {
	Hook    string `json:"-"`
	HookEnc string `json:"discordWebhookEnc,omitempty"`
	Vecchio string `json:"discordWebhook,omitempty"`
}

func TestFileDopoMigrazione(t *testing.T) {
	var s impFinte
	if err := json.Unmarshal([]byte(`{"discordWebhook":"`+hookProva+`","discordOn":true}`), &s); err != nil {
		t.Fatal(err)
	}
	s.Hook, s.HookEnc, _ = Migra(s.Vecchio, s.HookEnc, finta, sblocca)
	s.Vecchio = ""
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), "AbCdEfGh") || !strings.Contains(string(b), "DPAPI:") {
		t.Fatalf("token in chiaro nel file: %s", b)
	}
	if strings.Contains(string(b), `"discordWebhook"`) {
		t.Fatalf("resta la chiave in chiaro: %s", b)
	}
	if s.Hook != hookProva {
		t.Fatal("webhook perso")
	}
}

func TestTogliInChiaro(t *testing.T) {
	in := []byte("{\n  \"discordWebhook\": \"" + hookProva + "\",\n  \"discordOn\": true\n}")
	out, ok := TogliInChiaro(in)
	if !ok || strings.Contains(string(out), "AbCdEfGh") || !strings.Contains(string(out), `"discordWebhook": ""`) {
		t.Fatalf("%v %s", ok, out)
	}
	// file rovinato (non è JSON): il token sparisce lo stesso
	out, ok = TogliInChiaro([]byte(`{"x":1,"discordWebhook":"` + hookProva + `", "rotto`))
	if !ok || strings.Contains(string(out), "AbCdEfGh") {
		t.Fatalf("%v %s", ok, out)
	}
	if _, ok := TogliInChiaro([]byte(`{"discordWebhookEnc":"QUJD","discordWebhook":""}`)); ok {
		t.Fatal("niente da togliere")
	}
}

func TestMaschera(t *testing.T) {
	m := Maschera(hookProva)
	if strings.Contains(m, "AbCdEfGh") || !strings.HasPrefix(m, "https://discord.com/api/webhooks/123456789012345678/") {
		t.Fatalf("%q", m)
	}
	if Maschera("https://esempio/AbCdEfGhIjKlMnOpQrStUvWxYz") != Mascherato || Maschera("") != "" {
		t.Fatal("indirizzo strano mostrato")
	}
	if s := Nasconde("errore con "+hookProva+" e basta", hookProva); strings.Contains(s, "AbCdEfGh") {
		t.Fatalf("%q", s)
	}
}
