package installa

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Le voci che Tasso Zip e Tasso PDF scrivono da soli in HKCU (menu del tasto destro, "Apri
// con") e le loro cartelle in %LOCALAPPDATA% vanno via con il programma; quelle di un'altra
// copia (un altro exe) e le cose che non sono loro restano.
func TestDisinstallaToglieTracceUtente(t *testing.T) {
	p := nuovaProva(t, "3.1")
	locale := filepath.Join(t.TempDir(), "AppData", "Local")
	p.in.CartellaDatiUtente = func() string { return locale }
	if _, err := p.in.Installa(context.Background(), p.opzioni(TuttiComponenti...)); err != nil {
		t.Fatal(err)
	}
	zipExe := filepath.Join(p.base, "Tasso Zip", "TassoZip.exe")
	pdfExe := filepath.Join(p.base, "Tasso PDF", "TassoPDF.exe")
	cl := `Software\Classes\`
	r := p.reg
	// come fanno attivaMenu / attivaAssociazioni di Tasso Zip e attivaMenu di Tasso PDF
	r.ScriviTesto(HKCU, cl+`*\shell\TassoZip`, "MUIVerb", "Tasso Zip")
	r.ScriviTesto(HKCU, cl+`*\shell\TassoZip\shell\1comprimi\command`, "", `"`+zipExe+`" --comprimi "%1"`)
	r.ScriviTesto(HKCU, cl+`Directory\shell\TassoZip\shell\2zip\command`, "", `"`+zipExe+`" --zip "%1"`)
	r.ScriviTesto(HKCU, cl+`SystemFileAssociations\.7z\shell\TassoZip\shell\1apri\command`, "", `"`+zipExe+`" --apri "%1"`)
	r.ScriviTesto(HKCU, cl+progIDZip+`\shell\open\command`, "", `"`+zipExe+`" --apri "%1"`)
	r.ScriviTesto(HKCU, cl+`Applications\TassoZip.exe\shell\open\command`, "", `"`+zipExe+`" --apri "%1"`)
	r.ScriviTesto(HKCU, `Software\Tasso\TassoZip\Capabilities\FileAssociations`, ".zip", progIDZip)
	r.ScriviTesto(HKCU, `Software\RegisteredApplications`, "Tasso Zip", `Software\Tasso\TassoZip\Capabilities`)
	r.ScriviTesto(HKCU, `Software\RegisteredApplications`, "Altro", `Software\Altro`)
	r.ScriviTesto(HKCU, cl+`.zip\OpenWithProgids`, progIDZip, "")
	r.ScriviTesto(HKCU, cl+`.zip`, "", progIDZip)
	r.ScriviTesto(HKCU, cl+`.rar\OpenWithProgids`, progIDZip, "")
	r.ScriviTesto(HKCU, cl+`.rar\OpenWithProgids`, "WinRAR", "")
	r.ScriviTesto(HKCU, cl+`.rar`, "", "WinRAR") // scelta dell'utente: resta
	r.ScriviTesto(HKCU, cl+`SystemFileAssociations\.pdf\shell\TassoPDF\shell\1apri\command`, "", `"`+pdfExe+`" --apri "%1"`)
	r.ScriviTesto(HKCU, cl+`SystemFileAssociations\.png\shell\TassoPDF\shell\1unisci\command`, "", `"`+pdfExe+`" --unisci "%1"`)
	r.ScriviTesto(HKCU, cl+`SystemFileAssociations\.png\shell\Altro\command`, "", `altro.exe`)
	for _, d := range []string{"Tasso Zip", "Tasso PDF", "Altro"} {
		os.MkdirAll(filepath.Join(locale, d, "WebView2"), 0o755)
		os.WriteFile(filepath.Join(locale, d, "impostazioni.json"), []byte("{}"), 0o644)
	}

	if _, err := p.in.Disinstalla(p.base, []Componente{Zip}); err != nil {
		t.Fatal(err)
	}
	for _, k := range r.Chiavi() {
		if strings.Contains(k, "tassozip") {
			t.Errorf("rimasta: %s", k)
		}
	}
	if _, ok := r.LeggiTesto(HKCU, `Software\RegisteredApplications`, "Tasso Zip"); ok {
		t.Error("RegisteredApplications rimasta")
	}
	if _, ok := r.LeggiTesto(HKCU, `Software\RegisteredApplications`, "Altro"); !ok {
		t.Error("tolta una voce non nostra")
	}
	if v, ok := r.LeggiTesto(HKCU, cl+`.rar`, ""); !ok || v != "WinRAR" {
		t.Error("tolta la scelta dell'utente per .rar")
	}
	if _, ok := r.LeggiTesto(HKCU, cl+`.rar\OpenWithProgids`, "WinRAR"); !ok {
		t.Error("tolto WinRAR da Apri con")
	}
	if !r.Vuota(HKCU, cl+`.zip`) {
		t.Error(".zip non pulita")
	}
	if esiste(filepath.Join(locale, "Tasso Zip")) || !esiste(filepath.Join(locale, "Tasso PDF")) || !esiste(filepath.Join(locale, "Altro")) {
		t.Error("cartelle dei dati sbagliate dopo aver tolto Zip")
	}
	if _, ok := r.LeggiTesto(HKCU, cl+`SystemFileAssociations\.pdf\shell\TassoPDF\shell\1apri\command`, ""); !ok {
		t.Fatal("tolto il menu di Tasso PDF insieme a Zip")
	}

	// Il menu di Tasso PDF che avvia un'ALTRA copia (es. portatile) non si tocca.
	altroPDF := `"D:\Portatili\TassoPDF.exe" --apri "%1"`
	r.ScriviTesto(HKCU, cl+`SystemFileAssociations\.pdf\shell\TassoPDF\shell\1apri\command`, "", altroPDF)
	if _, err := p.in.Disinstalla(p.base, []Componente{PDF}); err != nil {
		t.Fatal(err)
	}
	if v, _ := r.LeggiTesto(HKCU, cl+`SystemFileAssociations\.pdf\shell\TassoPDF\shell\1apri\command`, ""); v != altroPDF {
		t.Fatal("tolto il menu di un'altra copia di Tasso PDF")
	}
	if esiste(filepath.Join(locale, "Tasso PDF")) {
		t.Error("cartella dei dati di Tasso PDF rimasta")
	}
	if _, ok := r.LeggiTesto(HKCU, cl+`SystemFileAssociations\.png\shell\Altro\command`, ""); !ok {
		t.Error("tolta una voce non nostra su .png")
	}

	// Stessa cosa con il menu che avvia proprio la copia installata: va via (anche .jpg ecc.)
	p2 := nuovaProva(t, "3.1")
	if _, err := p2.in.Installa(context.Background(), p2.opzioni(PDF)); err != nil {
		t.Fatal(err)
	}
	pdf2 := filepath.Join(p2.base, "Tasso PDF", "TassoPDF.exe")
	p2.reg.ScriviTesto(HKCU, cl+`SystemFileAssociations\.pdf\shell\TassoPDF\shell\1apri\command`, "", `"`+pdf2+`" --apri "%1"`)
	p2.reg.ScriviTesto(HKCU, cl+`SystemFileAssociations\.jpg\shell\TassoPDF\shell\1unisci\command`, "", `"`+pdf2+`" --unisci "%1"`)
	if r, err := p2.in.Disinstalla(p2.base, []Componente{PDF}); err != nil || !r.TuttoTolto {
		t.Fatal(err)
	}
	if k := p2.reg.Chiavi(); len(k) != 0 {
		t.Fatalf("registro non pulito: %v", k)
	}
}
