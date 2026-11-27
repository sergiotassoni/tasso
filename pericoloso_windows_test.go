package main

import "testing"

func TestPericolosoNuove(t *testing.T) {
	for _, n := range []string{"a.msc", "b.appref-ms", "c.XLL", "d.wsc", "e.sct", "f.diagcab", "g.search-ms", "h.ps1xml", "i.mht", "j.MHTML", "k.exe. "} {
		if !pericoloso(n) {
			t.Errorf("%q non visto come pericoloso", n)
		}
	}
	for _, n := range []string{"foto.jpg", "nota.txt", "documento.pdf"} {
		if pericoloso(n) {
			t.Errorf("%q visto come pericoloso", n)
		}
	}
}
