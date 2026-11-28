package main

import "testing"

func TestPericolosoNuove(t *testing.T) {
	for _, n := range []string{"a.msc", "b.appref-ms", "c.XLL", "d.wsc", "e.sct", "f.diagcab", "g.search-ms", "h.ps1xml", "i.mht", "j.MHTML", "k.exe. ",
		// CTRL-ZP-02, ZP02-10: estensioni degli attacchi recenti
		"l.one", "m.onepkg", "n.rdp", "o.cab", "p.msu", "q.wsb", "r.xbap", "s.ocx", "t.vb", "u.ws", "v.shb", "w.theme", "x.themepack",
		"y.website", "z.mde", "aa.accde",
		// ZP02-2: spazi speciali in fondo (sul disco diventano "fattura.exe")
		"fattura.exe\u00a0", "fattura.exe\u3000", "fattura.exe\u2002", "fattura.bat\u0085", "cartella/fattura.exe\u00a0",
		`C:\Users\Mario\AppData\Local\Tasso Zip\Temp\apri-1\avvia.exe`} {
		if !pericoloso(n) {
			t.Errorf("%q non visto come pericoloso", n)
		}
	}
	for _, n := range []string{"foto.jpg", "nota.txt", "documento.pdf", "fattura.pdf", "cartella.exe/fattura.pdf"} {
		if pericoloso(n) {
			t.Errorf("%q visto come pericoloso", n)
		}
	}
}

// ZP03-4: l'exe all'inizio del comando del menu.
func TestExeDelComando(t *testing.T) {
	for c, atteso := range map[string]string{
		`"C:\Program Files\Tasso\Tasso Zip\TassoZip.exe" --zip "%1"`: `C:\Program Files\Tasso\Tasso Zip\TassoZip.exe`,
		`TassoZip.exe --zip "%1"`:                                    "", "": "", `"`: "",
	} {
		if g := exeDelComando(c); g != atteso {
			t.Errorf("exeDelComando(%q) = %q", c, g)
		}
	}
}

// ZP03-2: solo la pagina interna.
func TestNavigazionePermessa(t *testing.T) {
	paginaVista, paginaPrima = false, ""
	if !navigazionePermessa("data:text/html;charset=utf-8;base64,PGh0bWw+") {
		t.Fatal("la pagina interna non si apre")
	}
	for _, u := range []string{"https://esempio.it/", "http://x", "file:///C:/x.html", "data:text/html,<b>altro</b>", `\\server\x`} {
		if navigazionePermessa(u) {
			t.Errorf("permesso %q", u)
		}
		if esterno(u) && fontePermessa(u) {
			t.Errorf("messaggi accettati da %q", u)
		}
	}
	if !navigazionePermessa("data:text/html;charset=utf-8;base64,PGh0bWw+") || !fontePermessa("about:blank") {
		t.Error("ricaricare la pagina interna deve andare")
	}
}
