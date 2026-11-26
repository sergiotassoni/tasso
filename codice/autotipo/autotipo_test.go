package autotipo

import (
	"testing"
	"time"
)

func tipi(p []Passo) []int {
	var t []int
	for _, x := range p {
		t = append(t, x.Tipo)
	}
	return t
}

func ugualiInt(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSequenzaBase(t *testing.T) {
	p, err := LeggiSequenza("")
	if err != nil {
		t.Fatal(err)
	}
	if !ugualiInt(tipi(p), []int{Utente, Tab, Password, Invio}) {
		t.Fatalf("sequenza base sbagliata: %v", tipi(p))
	}
	if !HaInvio(p) {
		t.Fatal("la base preme Invio")
	}
}

func TestSequenzaGoogle(t *testing.T) {
	p, err := LeggiSequenza("{UTENTE}{INVIO}{ATTESA 2s}{PASSWORD}{INVIO}")
	if err != nil {
		t.Fatal(err)
	}
	if !ugualiInt(tipi(p), []int{Utente, Invio, Attesa, Password, Invio}) {
		t.Fatalf("passi: %v", tipi(p))
	}
	if p[2].Durata != 2*time.Second {
		t.Fatalf("attesa: %v", p[2].Durata)
	}
}

func TestSequenzeVarie(t *testing.T) {
	casi := []struct {
		s     string
		tipi  []int
		erra  bool
		testo string
	}{
		{s: "{utente}{tab}{password}", tipi: []int{Utente, Tab, Password}},
		{s: "{USERNAME}{TAB}{PASSWORD}{ENTER}", tipi: []int{Utente, Tab, Password, Invio}},
		{s: "{UTENTE}{ATTESA 500ms}{PASSWORD}", tipi: []int{Utente, Attesa, Password}},
		{s: "{UTENTE}{ATTESA 1,5}{PASSWORD}", tipi: []int{Utente, Attesa, Password}},
		{s: "{UTENTE}@ditta.it{TAB}{PASSWORD}", tipi: []int{Utente, Testo, Tab, Password}, testo: "@ditta.it"},
		{s: "{{}x{}}", tipi: []int{Testo}, testo: "{x}"},
		{s: "{UTENTE", erra: true},
		{s: "UTENTE}", erra: true},
		{s: "{CANCELLA}", erra: true},
		{s: "{ATTESA}", erra: true},
		{s: "{ATTESA 11s}", erra: true},
		{s: "{ATTESA -1}", erra: true},
		{s: "{ATTESA abc}", erra: true},
		{s: "{ATTESA 10s}{ATTESA 10s}{ATTESA 10s}{ATTESA 1s}", erra: true},
	}
	for _, c := range casi {
		p, err := LeggiSequenza(c.s)
		if c.erra {
			if err == nil {
				t.Errorf("%q: volevo un errore", c.s)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: errore %v", c.s, err)
			continue
		}
		if !ugualiInt(tipi(p), c.tipi) {
			t.Errorf("%q: passi %v, volevo %v", c.s, tipi(p), c.tipi)
		}
		if c.testo != "" {
			ok := false
			for _, x := range p {
				if x.Tipo == Testo && x.Testo == c.testo {
					ok = true
				}
			}
			if !ok {
				t.Errorf("%q: testo fisso %q non trovato", c.s, c.testo)
			}
		}
	}
}

func TestSenzaUtenteEDalPassword(t *testing.T) {
	p, _ := LeggiSequenza("")
	if !ugualiInt(tipi(SenzaUtente(p)), []int{Password, Invio}) {
		t.Fatalf("SenzaUtente: %v", tipi(SenzaUtente(p)))
	}
	if !ugualiInt(tipi(DalPassword(p)), []int{Password, Invio}) {
		t.Fatalf("DalPassword: %v", tipi(DalPassword(p)))
	}
	q, _ := LeggiSequenza("{UTENTE}{INVIO}")
	if !ugualiInt(tipi(DalPassword(q)), []int{Password}) {
		t.Fatalf("DalPassword senza password: %v", tipi(DalPassword(q)))
	}
}

func TestTitolo(t *testing.T) {
	casi := []struct {
		schema, titolo string
		ok             bool
	}{
		{"Steam", "Steam", true},
		{"steam", "Accedi a Steam", true},
		{"Accedi - Google Account", "Accedi – Google Account - Google Chrome", true},
		{"Accedi*Google", "Accedi - Account Google - Google Chrome", true},
		{"Accedi*Google", "Google - Accedi", false},
		{"*Steam", "Accedi a Steam", true},
		{"Steam*Accedi", "Steam - Accedi", true},
		{"Accedi*Steam*Epic", "Accedi a Steam", false},
		{"Epic;Steam", "Steam", true},
		{"*", "qualsiasi", false},
		{"", "Steam", false},
		{"Steam", "", false},
		{"Battle.net", "Discord", false},
	}
	for _, c := range casi {
		if got := TitoloCorrisponde(c.schema, c.titolo); got != c.ok {
			t.Errorf("TitoloCorrisponde(%q, %q) = %v, volevo %v", c.schema, c.titolo, got, c.ok)
		}
	}
}

func TestSiti(t *testing.T) {
	casi := []struct {
		a, b string
		ok   bool
	}{
		{"accounts.google.com", "google.com", true},
		{"https://accounts.google.com/v3/signin?x=1", "https://www.google.com", true},
		{"google.com.truffa.it", "google.com", false},
		{"google-login.com", "google.com", false},
		{"googlecom.it", "google.com", false},
		{"https://google.com@truffa.it/", "google.com", false},
		{"www.bbc.co.uk", "bbc.co.uk", true},
		{"evil.co.uk", "bbc.co.uk", false},
		{"store.steampowered.com:443", "steampowered.com", true},
		{"192.168.1.1", "192.168.1.1", true},
		{"192.168.1.1", "192.168.1.2", false},
		{"", "", false},
		{"cerca qualcosa", "google.com", false},
	}
	for _, c := range casi {
		if got := StessoSito(c.a, c.b); got != c.ok {
			t.Errorf("StessoSito(%q, %q) = %v, volevo %v", c.a, c.b, got, c.ok)
		}
	}
}

func TestCorrisponde(t *testing.T) {
	gmail := Voce{Site: "Gmail"}
	steam := Voce{Site: "Steam"}
	mio := Voce{Site: "Il mio gestionale", Win: "Gestionale*Accesso"}
	dom := Voce{Site: "Banca", Web: "https://www.banca-esempio.it/login"}
	casi := []struct {
		v  Voce
		c  Contesto
		ok bool
	}{
		{gmail, Contesto{Host: "accounts.google.com", Exe: "chrome.exe"}, true},
		{gmail, Contesto{Host: "accounts.google.com.truffa.it", Exe: "chrome.exe"}, false},
		{gmail, Contesto{Exe: "chrome.exe", Title: "Gmail - Google Chrome"}, false}, // senza indirizzo, niente
		{steam, Contesto{Exe: "steamwebhelper.exe", Title: "Accedi a Steam"}, true},
		{steam, Contesto{Host: "store.steampowered.com", Exe: "msedge.exe"}, true},
		{steam, Contesto{Host: "steampowered.com.login-gratis.ru", Exe: "msedge.exe"}, false},
		{steam, Contesto{Exe: "discord.exe", Title: "Steam"}, false},
		{mio, Contesto{Exe: "gest.exe", Title: "Gestionale 2024 - Accesso"}, true},
		{mio, Contesto{Exe: "chrome.exe", Host: "gestionale.it", Title: "Gestionale - Accesso"}, false},
		{dom, Contesto{Exe: "firefox.exe", Host: "banca-esempio.it"}, true},
		{dom, Contesto{Exe: "firefox.exe", Host: "banca-esempio.it.sicurezza.com"}, false},
		{Voce{Site: "google.com"}, Contesto{Exe: "brave.exe", Host: "mail.google.com"}, true},
		{Voce{Site: "Chrome"}, Contesto{Exe: "chrome.exe", Title: "Chrome"}, false},
	}
	for i, c := range casi {
		if got := Corrisponde(c.v, c.c); got != c.ok {
			t.Errorf("caso %d: Corrisponde(%+v, %+v) = %v, volevo %v", i, c.v, c.c, got, c.ok)
		}
	}
}

func TestTogliBrowserECampo(t *testing.T) {
	if got := TogliBrowser("Accedi - Account Google - Google Chrome"); got != "accedi - account google" {
		t.Errorf("TogliBrowser: %q", got)
	}
	if !CampoAccesso("Indirizzo email o numero di telefono", "") || !CampoAccesso("", "identifierId") || CampoAccesso("Cerca", "q") {
		t.Error("CampoAccesso sbagliato")
	}
}

// "://" dentro la parte dopo il sito (es. un parametro ?r=https://...) non cambia il sito.
func TestNormHostSchemaNeiParametri(t *testing.T) {
	casi := map[string]string{
		"finto-sito.com/login?r=https://www.amazon.it/":                                   "finto-sito.com",
		"finto-sito.com?x=https://banca.it":                                               "finto-sito.com",
		"finto-sito.com#https://banca.it":                                                 "finto-sito.com",
		"accounts.google.com/v3/signin/identifier?continue=https://mail.google.com/mail/": "accounts.google.com",
		"https://accedi.esempio.it/login?r=https://altro.it":                              "accedi.esempio.it",
		"HTTPS://WWW.Esempio.IT/":                                                         "esempio.it",
		"esempio.it":                                                                      "esempio.it",
	}
	for in, want := range casi {
		if got := NormHost(in); got != want {
			t.Errorf("NormHost(%q) = %q, voglio %q", in, got, want)
		}
	}
	if Corrisponde(Voce{Site: "Amazon", Web: "amazon.it"}, Contesto{Exe: "chrome.exe", Host: NormHost("finto-sito.com/login?r=https://www.amazon.it/")}) {
		t.Error("un parametro con https://www.amazon.it non deve far proporre l'account Amazon")
	}
}
