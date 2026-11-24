package main

import "testing"

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
