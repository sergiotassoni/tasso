// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRisorse(t *testing.T) {
	if _, ct, ok := risorsa("https://tassopdf.localhost/"); !ok || !strings.HasPrefix(ct, "text/html") {
		t.Fatal("pagina principale non servita")
	}
	if _, ct, ok := risorsa("https://tassopdf.localhost/pdfjs/build/pdf.worker.mjs"); !ok || !strings.HasPrefix(ct, "text/javascript") {
		t.Fatal("worker di PDF.js non servito")
	}
	if _, _, ok := risorsa("https://tassopdf.localhost/pdfjs/web/viewer.html?file=x#page=2"); !ok {
		t.Fatal("visualizzatore non servito")
	}
	// niente internet e niente uscite dalla cartella dell'interfaccia
	for _, u := range []string{
		"https://example.com/", "http://tassopdf.localhost/", "https://tassopdf.localhost.example.com/",
		"https://tassopdf.localhost/../app.go", "https://tassopdf.localhost/%2e%2e/app.go", "file:///C:/Windows/win.ini",
		"https://tassopdf.localhost/file/inesistente",
	} {
		if _, _, ok := risorsa(u); ok {
			t.Errorf("non doveva rispondere a %s", u)
		}
	}
	if !navigazionePermessa("https://tassopdf.localhost/index.html") || navigazionePermessa("https://example.com/") ||
		navigazionePermessa("file:///C:/x.pdf") || navigazionePermessa("https://tassopdf.localhost.evil.com/") {
		t.Fatal("navigazionePermessa")
	}
}

func TestFileInterno(t *testing.T) {
	pdf := pdfProva([]pagProva{a4("uno")})
	id := goCaricaInizio(`C:\Utenti\Mario\Documenti\Contratto.pdf`)
	if err := goCaricaPezzo(id, base64.StdEncoding.EncodeToString(pdf[:100])); err != nil {
		t.Fatal(err)
	}
	if err := goCaricaPezzo(id, base64.StdEncoding.EncodeToString(pdf[100:])); err != nil {
		t.Fatal(err)
	}
	vi, err := goCaricaFine(id)
	if err != nil || vi.Tipo != "pdf" || vi.Nome != "Contratto.pdf" || vi.Dim != int64(len(pdf)) {
		t.Fatalf("caricamento: %+v %v", vi, err)
	}
	b, ct, ok := risorsa("https://tassopdf.localhost/file/" + id)
	if !ok || ct != "application/pdf" || len(b) != len(pdf) {
		t.Fatal("file interno")
	}
	goDimentica(id)
	if _, _, ok := risorsa("https://tassopdf.localhost/file/" + id); ok {
		t.Fatal("file dimenticato ancora servito")
	}
}

func TestErroriPerPagina(t *testing.T) {
	_, err := goInfo("nessuno", "")
	if err == nil || !strings.HasPrefix(err.Error(), "perso|") {
		t.Fatalf("errore per la pagina: %v", err)
	}
	e := perPagina(&ErrPezzo{Indice: 2, Err: errc("password", nil)})
	if !strings.HasPrefix(e.Error(), "pezzo:2|password|") {
		t.Fatalf("errore pezzo: %v", e)
	}
}

// Il file originale non si sovrascrive mai, anche se nella finestra "Salva" l'utente lo sceglie.
func TestSalvaNonSovrascriveOriginale(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "Contratto.pdf")
	data := pdfProva([]pagProva{a4("Contratto")})
	if err := os.WriteFile(orig, data, 0o644); err != nil {
		t.Fatal(err)
	}
	vi, err := apriPercorso(orig)
	if err != nil {
		t.Fatal(err)
	}
	firma := base64.StdEncoding.EncodeToString(pngProva(t, 40, 20, true))
	res, err := goFirma(vi.ID, "", []TimbroPagina{{Timbro: Timbro{Pagina: 1, X: .1, Y: .1, L: .3, A: .05}, PNG64: "data:image/png;base64," + firma}})
	if err != nil {
		t.Fatal(err)
	}
	vecchio := piattaforma.salvaFile
	defer func() { piattaforma.salvaFile = vecchio }()

	var cartellaProposta, nomeProposto string
	piattaforma.salvaFile = func(nome, cartella, titolo string) (string, error) {
		cartellaProposta, nomeProposto = cartella, nome
		return orig, nil // l'utente sceglie proprio l'originale
	}
	if _, err := goSalva(res.ID, "Contratto - firmato.pdf", "Salva"); err == nil || !strings.HasPrefix(err.Error(), "originale|") {
		t.Fatalf("doveva rifiutare l'originale: %v", err)
	}
	if cartellaProposta != dir || nomeProposto != "Contratto - firmato.pdf" {
		t.Fatalf("proposta: %q %q", cartellaProposta, nomeProposto)
	}
	if b, _ := os.ReadFile(orig); string(b) != string(data) {
		t.Fatal("l'originale è stato modificato!")
	}
	// nome nuovo: salva, senza toccare l'originale
	nuovo := filepath.Join(dir, "Contratto - firmato")
	piattaforma.salvaFile = func(nome, cartella, titolo string) (string, error) { return nuovo, nil }
	p, err := goSalva(res.ID, "Contratto - firmato.pdf", "Salva")
	if err != nil || p != nuovo+".pdf" {
		t.Fatalf("salvataggio: %q %v", p, err)
	}
	if n := pagine(t, mustRead(t, p)); n != 1 {
		t.Fatalf("pagine %d", n)
	}
	if b, _ := os.ReadFile(orig); string(b) != string(data) {
		t.Fatal("l'originale è stato modificato!")
	}
	// annullato
	piattaforma.salvaFile = func(nome, cartella, titolo string) (string, error) { return "", nil }
	if p, err := goSalva(res.ID, "x.pdf", "Salva"); p != "" || err != nil {
		t.Fatal("annullamento")
	}
	// niente file temporanei lasciati in giro
	ee, _ := os.ReadDir(dir)
	for _, e := range ee {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("file temporaneo rimasto: %s", e.Name())
		}
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestNomeValido(t *testing.T) {
	if n := nomeValido(`Con:tra"tto?.pdf`); n != "Con_tra_tto_.pdf" {
		t.Fatal(n)
	}
	if nomeValido("  ") != "documento.pdf" {
		t.Fatal("vuoto")
	}
}

func TestLinkSoloWebEPosta(t *testing.T) {
	aperti := []string{}
	vecchio := piattaforma.apriLink
	defer func() { piattaforma.apriLink = vecchio }()
	piattaforma.apriLink = func(u string) error { aperti = append(aperti, u); return nil }
	for _, u := range []string{"https://example.com/a", "mailto:a@b.it"} {
		if err := goApriLink(u); err != nil {
			t.Fatalf("%s: %v", u, err)
		}
	}
	for _, u := range []string{"file:///C:/Windows/System32/calc.exe", "javascript:alert(1)", `C:\x.exe`, "ms-settings:", "https://"} {
		if err := goApriLink(u); err == nil {
			t.Errorf("non doveva aprire %s", u)
		}
	}
	if len(aperti) != 2 {
		t.Fatalf("aperti: %v", aperti)
	}
}

func TestLingua(t *testing.T) {
	dir := t.TempDir()
	v1, v2, v3 := piattaforma.cartellaDati, piattaforma.linguaTasso, piattaforma.linguaSistema
	defer func() { piattaforma.cartellaDati, piattaforma.linguaTasso, piattaforma.linguaSistema = v1, v2, v3 }()
	piattaforma.cartellaDati = func() string { return dir }
	piattaforma.linguaSistema = func() string { return "de" }
	piattaforma.linguaTasso = func() string { return "" }
	if l := linguaIniziale(); l != "de" {
		t.Fatalf("sistema: %s", l)
	}
	piattaforma.linguaTasso = func() string { return "es-ES" }
	if l := linguaIniziale(); l != "es" {
		t.Fatalf("registro Tasso: %s", l)
	}
	if err := goLingua("pl"); err != nil {
		t.Fatal(err)
	}
	if l := linguaIniziale(); l != "pl" {
		t.Fatalf("scelta in alto: %s", l)
	}
	if goLingua("xx") == nil {
		t.Fatal("lingua sconosciuta accettata")
	}
	piattaforma.cartellaDati = func() string { return "" }
	piattaforma.linguaTasso = func() string { return "" }
	piattaforma.linguaSistema = func() string { return "ja" }
	if l := linguaIniziale(); l != "en" {
		t.Fatalf("predefinita: %s", l)
	}
}

func TestFirmaSalvata(t *testing.T) {
	dir := t.TempDir()
	v1, v2, v3 := piattaforma.cartellaDati, piattaforma.proteggi, piattaforma.svela
	defer func() { piattaforma.cartellaDati, piattaforma.proteggi, piattaforma.svela = v1, v2, v3 }()
	piattaforma.cartellaDati = func() string { return dir }
	piattaforma.proteggi = func(b []byte) ([]byte, error) {
		o := make([]byte, len(b))
		for i := range b {
			o[i] = b[i] ^ 0x5A
		}
		return o, nil
	}
	piattaforma.svela = piattaforma.proteggi
	png := pngProva(t, 20, 10, true)
	if err := goSalvaFirma("data:image/png;base64," + base64.StdEncoding.EncodeToString(png)); err != nil {
		t.Fatal(err)
	}
	if !goAvvio().Firma {
		t.Fatal("la firma salvata non risulta")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "firma.dat"))
	if strings.Contains(string(raw), "PNG") {
		t.Fatal("la firma è salvata in chiaro")
	}
	s, err := goCaricaFirma()
	if err != nil || s != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(png) {
		t.Fatalf("rilettura: %v", err)
	}
	if err := goCancellaFirma(); err != nil || goAvvio().Firma {
		t.Fatal("cancellazione")
	}
}
