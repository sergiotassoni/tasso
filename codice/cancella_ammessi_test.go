package main

import (
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
)

func TestPercorsoAmmesso(t *testing.T) {
	a := cartelleAmmesse{
		intere: []string{`C:\Windows\Temp`, `C:\NVIDIA`, `C:\Users\a\AppData\Local\Temp`, `D:\Temp`},
		dentro: []string{`C:\Users\a\AppData\Local`, `C:\Users\a\AppData\Roaming`},
	}
	si := []string{
		`C:\Windows\Temp`, `C:\Windows\Temp\x\y.tmp`, `C:\NVIDIA\DisplayDriver`, `c:\users\A\appdata\local\Google\Chrome\User Data\Default\Cache`,
		`C:\Users\a\AppData\Roaming\discord\Cache`, `C:\Users\a\AppData\Local\Temp`, `D:\Temp\abc`,
		`D:\SteamLibrary\steamapps\downloading\730`, `C:\$Recycle.Bin\S-1-5-21-1-2-3-1001\$RABC123.jpg`,
	}
	no := []string{
		`C:\Users\a\AppData\Local`, `C:\Users\a\AppData\Roaming`, `C:\Windows`, `C:\Windows\System32`, `C:\Windows\Temp2`,
		`C:\Program Files\Steam`, `C:\ProgramData\Microsoft`, `C:\Users\a\Documents`, `C:\Users\a\Desktop\foto.jpg`,
		`C:\Users\b\AppData\Local\Temp`, `D:\SteamLibrary\steamapps\downloading`, `C:\`, `D:\`, ``,
		`C:\Users\a\AppData\Local\..\..\Documents`, `C:\$Recycle.Bin`, `C:\$Recycle.Bin\S-1-5-21-1-2-3-1001`,
	}
	for _, p := range si {
		if !percorsoAmmesso(p, a) {
			t.Errorf("percorsoAmmesso(%q) = false, volevo true", p)
		}
	}
	for _, p := range no {
		if percorsoAmmesso(p, a) {
			t.Errorf("percorsoAmmesso(%q) = true, volevo false", p)
		}
	}
}

// psQuote: con apici normali, tipografici, accenti, a capo e nomi preparati apposta, il valore
// arriva identico e niente può uscire dalla stringa.
func TestPsQuote(t *testing.T) {
	semplice := regexp.MustCompile(`^'[ -&(-_a-~]*'$`) // ASCII stampabile senza ' e `
	b64 := regexp.MustCompile(`^\(\[Text\.Encoding\]::UTF8\.GetString\(\[Convert\]::FromBase64String\('([A-Za-z0-9+/=]*)'\)\)\)$`)
	casi := []string{
		`C:\Users\Mario\AppData\Local`, `C:\Users\D’Amico\AppData\Local`, `C:\Users\O'Brien`,
		`C:\x’;Start-Process calc;’`, `C:\x‘;calc;‘`, `C:\x‚;calc;‛`, "C:\\x'\n;calc;#", "C:\\x`$(calc)", `C:\Users\José`,
		`C:\x";calc;"`, `C:\x$(calc)`, ``,
	}
	for _, s := range casi {
		q := psQuote(s)
		if m := b64.FindStringSubmatch(q); m != nil {
			d, err := base64.StdEncoding.DecodeString(m[1])
			if err != nil || string(d) != s {
				t.Errorf("psQuote(%q): base64 non torna uguale", s)
			}
			continue
		}
		if !semplice.MatchString(q) || q[1:len(q)-1] != s {
			t.Errorf("psQuote(%q) = %q: non è una stringa sicura", s, q)
		}
		for _, r := range "‘’‚‛`" {
			if strings.ContainsRune(q, r) {
				t.Errorf("psQuote(%q) contiene %q", s, r)
			}
		}
	}
}
