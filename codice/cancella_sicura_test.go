package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Prove dei controlli sui percorsi (girano solo su Windows, come tutto Tasso).

func TestDentroDi(t *testing.T) {
	casi := []struct {
		p, base string
		ok      bool
	}{
		{`C:\Users\a\AppData\Local\Temp`, `C:\Users\a\AppData\Local`, true},
		{`c:\users\A\appdata\local`, `C:\Users\a\AppData\Local\`, true},
		{`C:\Users\a\AppData\LocalX`, `C:\Users\a\AppData\Local`, false},
		{`C:\Windows`, `C:\Windows\System32`, false},
		{``, `C:\Windows`, false},
	}
	for _, c := range casi {
		if got := dentroDi(c.p, c.base); got != c.ok {
			t.Errorf("dentroDi(%q, %q) = %v, volevo %v", c.p, c.base, got, c.ok)
		}
	}
}

func TestPercorsoLocale(t *testing.T) {
	for _, p := range []string{``, `\\server\share\x`, `\\?\C:\x`, `relativo\x`, `C:x`, `C:\a\b:flusso`, `C:\a*`} {
		if _, ok := percorsoLocale(p); ok {
			t.Errorf("percorsoLocale(%q) accettato", p)
		}
	}
	if q, ok := percorsoLocale(`C:\Users\a\..\b\`); !ok || q != `C:\Users\b` {
		t.Errorf("percorsoLocale normale: %q %v", q, ok)
	}
}

func TestUnisciPath(t *testing.T) {
	got := unisciPath([]string{`C:\Windows\System32`, `c:\windows\system32\`, `.`, `%NONESISTE%\bin`, `\\server\bin`, `C:\Users\a\AppData\Local\Microsoft\WindowsApps`, `C:\Program Files\Git\cmd`},
		[]string{`C:\Users\a`})
	want := `C:\Windows\System32;C:\Program Files\Git\cmd`
	if got != want {
		t.Errorf("unisciPath = %q, volevo %q", got, want)
	}
}

func TestRadiceAccettabile(t *testing.T) {
	vecchie := cartelleVere
	defer func() { cartelleVere = vecchie }()
	cartelleVere = cartelle{Win: `C:\Windows`, PF: `C:\Program Files`, PF86: `C:\Program Files (x86)`, PD: `C:\ProgramData`, Profilo: `C:\Users\a`}
	if radiceAccettabile(`C:\Users\a\AppData\Local\Temp`, `C:\Users\a\AppData\Local\Temp`) != nil {
		t.Error("stessa cartella rifiutata")
	}
	if radiceAccettabile(`C:\Users\a\AppData\Local\Temp`, `R:\Temp`) != nil {
		t.Error("Temp su un altro disco rifiutata")
	}
	for _, f := range []string{`C:\Windows\System32`, `C:\Program Files\X`, `D:\`, `C:\Users\b\Documents`, ``} {
		if radiceAccettabile(`C:\Users\a\AppData\Local\Temp`, f) == nil {
			t.Errorf("radice che porta a %q accettata", f)
		}
	}
}

// F18-9: il motivo di un rifiuto (tra parentesi nei messaggi di Teams e di Office) è nella lingua di
// Tasso: i motivi dello script (steps.go) sono nella sezione ps, quelli di Go nella sezione go.
func TestMotiviRifiutoTradotti(t *testing.T) {
	src, err := os.ReadFile("cancella_sicura.go")
	if err != nil {
		t.Fatal(err)
	}
	var motiviGo []string
	for _, m := range regexp.MustCompile(`(?:errCS|rifiuta|motivoTradotto)\("([^"]+)"`).FindAllStringSubmatch(string(src), -1) {
		motiviGo = append(motiviGo, strings.TrimSuffix(m[1], ": "))
	}
	var motiviPS []string
	for _, m := range regexp.MustCompile(`\$script:TassoRifiuto = '((?:[^']|'')+)'`).FindAllStringSubmatch(psPrelude, -1) {
		motiviPS = append(motiviPS, strings.ReplaceAll(m[1], "''", "'"))
	}
	if len(motiviGo) < 10 || len(motiviPS) < 4 {
		t.Fatalf("trovati %d motivi di Go e %d dello script: troppo pochi", len(motiviGo), len(motiviPS))
	}
	for _, l := range []string{"en", "es", "fr", "de"} {
		d := dizLingua(l)
		if d == nil {
			t.Fatalf("manca il dizionario %s", l)
		}
		for _, m := range motiviGo {
			if d.Go[m] == "" {
				t.Errorf("%s: motivo di Go non tradotto: %q", l, m)
			}
		}
		for _, m := range motiviPS {
			if d.PS[m] == "" {
				t.Errorf("%s: motivo dello script non tradotto: %q", l, m)
			}
		}
	}
	t.Setenv("TASSO_LINGUA", "en")
	if got := motivoTradotto(`c'è una giunzione o un collegamento: Cache`); got != "there is a junction or a link: Cache" {
		t.Errorf("motivoTradotto in inglese = %q", got)
	}
	if got := motivoTradotto("Accesso negato."); got != "Accesso negato." {
		t.Errorf("un errore di Windows deve restare com'è: %q", got)
	}
	t.Setenv("TASSO_LINGUA", "it")
	if got := motivoTradotto("percorso non valido"); got != "percorso non valido" {
		t.Errorf("in italiano il motivo resta com'è: %q", got)
	}
	if !strings.Contains(psPrelude, "$psi.EnvironmentVariables['TASSO_LINGUA'] = [string]$TassoLang") {
		t.Error("lo script deve passare la lingua di Tasso a --cancella-sicura")
	}
}
