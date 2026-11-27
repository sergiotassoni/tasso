package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// A1 + X3: la prima scelta della modalità di Tasso (anche dall'installatore) non mette mai il PC in
// Gaming o FPS per sempre: con Gaming resta in Normale (piano energetico e notifiche come prima), e chi
// ha già un'altra modalità del PC non viene toccato.
func TestPrimoAvvioMaiGaming(t *testing.T) {
	casi := []struct{ profilo, prima, vuoi string }{
		{"gioco", "", "normale"},
		{"gioco", "normale", "normale"},
		{"gioco", "gaming", ""},
		{"gioco", "fps", ""},
		{"gioco", "film", ""},
		{"gioco", "originale", ""},
		{"ufficio", "normale", "normale"},
		{"entrambi", "normale", "normale"},
		{"boh", "normale", ""},
	}
	for _, c := range casi {
		if g := modoPrimoAvvio(c.profilo, c.prima); g != c.vuoi {
			t.Errorf("modoPrimoAvvio(%q, %q) = %q, voglio %q", c.profilo, c.prima, g, c.vuoi)
		}
	}
	b, err := os.ReadFile("settings.go")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?s)func profiloPC\(p string\) \{.*?\n\}`).Find(b)
	if m == nil {
		t.Fatal("profiloPC non trovata")
	}
	if strings.Contains(string(m), `"gaming"`) || strings.Contains(string(m), `"fps"`) {
		t.Error("profiloPC non deve mettere Gaming o FPS")
	}
}

// A2 + X2: gli avvisi intelligenti tacciono solo mentre si gioca (Boost acceso o gioco aperto), non per
// la modalità salvata; temperatura e salute dei dischi passano sempre; "già avvisato" solo se l'avviso è
// partito davvero (prima l'avviso del disco si perdeva per tutta la sessione).
func TestAvvisiIntelligenti(t *testing.T) {
	b, err := os.ReadFile("ui/index.html")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	f := regexp.MustCompile(`(?s)function alertSmart\(text, action, sempre\)\{.*?\n\}`).FindString(s)
	if f == "" {
		t.Fatal("alertSmart non trovata")
	}
	if strings.Contains(f, "S.mode") || !strings.Contains(f, "BST.on || BST.game") {
		t.Error("alertSmart deve guardare il Boost, non la modalità salvata")
	}
	for _, q := range []string{`diskAlerted = true; alertSmart`, `ramAlerted = true; (isOffice() ? alertSmart`} {
		if strings.Contains(s, q) {
			t.Errorf("segno messo prima dell'avviso: %s", q)
		}
	}
	if !strings.Contains(s, `{label:_t('Pulisci ora'), fn: () => confirmLevel(3)})) diskAlerted = true;`) {
		t.Error("diskAlerted va messo solo se alertSmart risponde true")
	}
	if n := strings.Count(s, ", null, true)) "); n < 2 {
		t.Errorf("le temperature devono passare sempre (sempre = true): %d", n)
	}
	if !strings.Contains(s, `show('hw')}, true)) healthAlerted = true;`) {
		t.Error("la salute dei dischi deve passare sempre")
	}
}

// B27: gli script si traducono (trPS traduce i testi tra apici): un confronto con un testo che il
// dizionario traduce non scatta più in inglese, francese o tedesco. Nella mappa del disco il limite
// di tempo per gli altri dischi ora usa un segno interno ("altro"), non il nome del gruppo.
func TestConfrontiNonTradotti(t *testing.T) {
	if strings.Contains(diskMapScript, "$t.group -like") || strings.Contains(diskMapScript, "-ne ('Disco ") || !strings.Contains(diskMapScript, "$t.altro -and") {
		t.Error("mappa del disco: il limite per gli altri dischi deve usare $t.altro")
	}
	re := regexp.MustCompile(`-(?:eq|ne|like|notlike|match|notmatch)\s*\(?\s*'((?:[^']|'')*)'`)
	for _, l := range []string{"en", "es", "fr", "de"} {
		d := dizLingua(l)
		if d == nil {
			t.Fatalf("manca il dizionario %s", l)
		}
		for nome, sc := range map[string]string{"diskMapScript": diskMapScript, "detectScript": detectScript, "netUsageScript": netUsageScript} {
			for _, m := range re.FindAllStringSubmatch(sc, -1) {
				lit := strings.ReplaceAll(m[1], "''", "'")
				if v, ok := d.PS[lit]; ok && v != lit {
					t.Errorf("%s (%s): confronto con un testo tradotto: %q", nome, l, lit)
				}
			}
		}
	}
}

// B11: il segnale del Wi-Fi si legge in ogni lingua di Windows (prima solo "Segnale" e "Signal").
func TestSegnaleWifiOgniLingua(t *testing.T) {
	m := regexp.MustCompile(`\$wi -match '([^']+)' -and \[int\]\$Matches\[1\] -lt 40`).FindStringSubmatch(detectScript)
	if m == nil {
		t.Fatal("controllo del segnale non trovato in detectScript (o soglia diversa da 40)")
	}
	re := regexp.MustCompile(m[1])
	for _, out := range []string{
		"    Nome                   : Wi-Fi\n    SSID                   : Casa\n    Segnale                : 35%\n",
		"    Name                   : Wi-Fi\n    Receive rate (Mbps)    : 144\n    Signal                 : 35%\n",
		"    Nombre                 : Wi-Fi\n    Señal                  : 35%\n",
		"    Nom                    : Wi-Fi\n    Signal                 : 35 %\n",
		"    Name                   : WLAN\n    Signal                 : 35%\n",
		"    Nome                   : Wi-Fi\n    Sinal                  : 35%\n",
	} {
		s := re.FindStringSubmatch(strings.ReplaceAll(out, "\n", "\r\n"))
		if s == nil || s[1] != "35" {
			t.Errorf("segnale non letto in:\n%s", out)
		}
	}
	if !strings.Contains(netUsageScript, `Select-String -Pattern '^\s*[^:]+:\s*(\d+)\s*%'`) {
		t.Error("Internet (netUsageScript) deve leggere il segnale allo stesso modo")
	}
}
