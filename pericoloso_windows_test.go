package main

import "testing"

func TestPericolosoNuove(t *testing.T) {
	prima := assocPericolosa
	assocPericolosa = func(string) bool { return false } // qui conta solo l'elenco di Tasso Zip
	defer func() { assocPericolosa = prima }()
	for _, n := range []string{"a.msc", "b.appref-ms", "c.XLL", "d.wsc", "e.sct", "f.diagcab", "g.search-ms", "h.ps1xml", "i.mht", "j.MHTML", "k.exe. ",
		// CTRL-ZP-02, ZP02-10: estensioni degli attacchi recenti
		"l.one", "m.onepkg", "n.rdp", "o.cab", "p.msu", "q.wsb", "r.xbap", "s.ocx", "t.vb", "u.ws", "v.shb", "w.theme", "x.themepack",
		"y.website", "z.mde", "aa.accde",
		// RECENSIONE-3b, R3b-2
		"script.py", "finestra.PYW", "a.pyz", "a.pyzw", "a.pyc", "a.pyo", "avvia.jnlp", "cerca.searchConnector-ms",
		"a.appcontent-ms", "cert.cer", "cert.crt", "cert.der", "a.mst", "a.msh", "a.msh1", "a.msh2", "a.mshxml",
		"a.ps2", "a.ps2xml", "a.psc1", "a.psc2", "query.iqy", "foglio.slk", "db.mdb", "db.accdb", "a.ade", "a.adp",
		"guida.its", "stampanti.printerexport", "script.py\u00a0",
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

// R3b-2: se Windows (AssocIsDangerous) dice che un tipo è pericoloso, chiedo anche se non è
// nell'elenco di Tasso Zip. Il controllo riceve l'estensione pulita, in minuscolo.
func TestPericolosoPerWindows(t *testing.T) {
	prima := assocPericolosa
	defer func() { assocPericolosa = prima }()
	var viste []string
	assocPericolosa = func(est string) bool { viste = append(viste, est); return est == ".nuovotipo" }
	if !pericoloso("cartella/strano.NuovoTipo. ") {
		t.Error("un tipo pericoloso per Windows non chiede conferma")
	}
	if pericoloso("foto.jpg") {
		t.Error("foto.jpg visto come pericoloso")
	}
	if len(viste) != 2 || viste[0] != ".nuovotipo" || viste[1] != ".jpg" {
		t.Errorf("estensioni passate a Windows: %q", viste)
	}
}

// Su Windows vero: AssocIsDangerous conosce almeno .exe, e non segna .txt.
func TestAssocIsDangerousVero(t *testing.T) {
	if procAssocIsDangerous.Find() != nil {
		t.Skip("shlwapi senza AssocIsDangerous")
	}
	if !assocPericolosa(".exe") {
		t.Error("AssocIsDangerous(.exe) = false")
	}
	if assocPericolosa(".txt") || assocPericolosa("") {
		t.Error("AssocIsDangerous segna .txt o niente")
	}
}
