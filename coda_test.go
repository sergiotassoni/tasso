// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestComandiRiapertura(t *testing.T) {
	var pp []string
	for i := 0; i < 500; i++ {
		pp = append(pp, fmt.Sprintf(`C:\Users\Utente\Documenti\Una cartella con un nome lungo\file numero %03d.zip`, i))
	}
	pp = append(pp, `C:\x"y.zip`, "")
	cc := comandiRiapertura("estrai-qui", pp, maxParametri)
	if len(cc) < 2 {
		t.Fatalf("500 percorsi lunghi in %d comandi", len(cc))
	}
	visti := 0
	for _, c := range cc {
		if len(c) > maxParametri {
			t.Fatalf("comando troppo lungo: %d", len(c))
		}
		if !strings.HasPrefix(c, "--estrai-qui ") {
			t.Fatalf("manca il modo: %q", c[:20])
		}
		visti += strings.Count(c, `"`) / 2
	}
	if visti != 500 {
		t.Fatalf("percorsi passati: %d invece di 500", visti)
	}
	if c := comandiRiapertura("zip", []string{`C:\a.txt`}, maxParametri); len(c) != 1 || c[0] != `--zip "C:\a.txt"` {
		t.Fatalf("uno solo: %q", c)
	}
	if c := comandiRiapertura("zip", nil, maxParametri); len(c) != 0 {
		t.Fatal("niente file: niente avvii")
	}
}
