package installa

import (
	"context"
	"testing"
)

// ZP-03: UserChoiceLatest com'è davvero su Windows 11 (kolbi.cz, 2025): la chiave ha solo "Hash",
// il ProgId sta nella sottochiave UserChoiceLatest\ProgId, valore "ProgId".
func TestZP03UserChoiceLatestVera(t *testing.T) {
	fe := `Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\`
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(Zip)); err != nil {
		t.Fatal(err)
	}
	r := p.reg
	r.ScriviTesto(HKCU, fe+`.zip\UserChoiceLatest`, "Hash", "1xZShZRatSi=")
	r.ScriviTesto(HKCU, fe+`.zip\UserChoiceLatest\ProgId`, "ProgId", progIDZip)
	if _, err := p.in.Disinstalla(p.base, []Componente{Zip}); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.LeggiTesto(HKCU, fe+`.zip\UserChoiceLatest\ProgId`, "ProgId"); ok {
		t.Errorf("dopo la disinstallazione resta UserChoiceLatest\\ProgId = %q (porta a un tipo di file che non c'è più)", v)
	}
}

// ZP-03: il menu riscritto da una copia aperta una volta da una cartella temporanea (exe che non c'è
// più): il disinstallatore lo lascia, e resta un menu che avvia un exe inesistente.
func TestZP03MenuDiCopiaSparita(t *testing.T) {
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(Zip)); err != nil {
		t.Fatal(err)
	}
	r := p.reg
	sparito := `C:\Users\Mario\AppData\Local\Temp\Temp1_TassoZip-1.3.2.zip\TassoZip.exe`
	k := `Software\Classes\*\shell\TassoZip\shell\1comprimi\command`
	r.ScriviTesto(HKCU, k, "", `"`+sparito+`" --comprimi "%1"`)
	if _, err := p.in.Disinstalla(p.base, []Componente{Zip}); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.LeggiTesto(HKCU, k, ""); ok {
		t.Errorf("menu del tasto destro rimasto dopo la disinstallazione: avvia %s, che non esiste", sparito)
	}
}

// ZP-03 (correzione): il registro finto si comporta come Windows: TogliChiave non toglie una
// chiave che ha sottochiavi (RegDeleteKey), TogliAlbero sì.
func TestZP03RegistroFintoComeWindows(t *testing.T) {
	r := NuovoRegistroMemoria()
	r.ScriviTesto(HKCU, `A\B`, "Hash", "x")
	r.ScriviTesto(HKCU, `A\B\ProgId`, "ProgId", "y")
	if err := r.TogliChiave(HKCU, `A\B`); err == nil {
		t.Error("TogliChiave ha tolto una chiave con sottochiavi")
	}
	if _, ok := r.LeggiTesto(HKCU, `A\B\ProgId`, "ProgId"); !ok {
		t.Error("sottochiave persa")
	}
	if err := r.TogliAlbero(HKCU, `A\B`); err != nil || !r.Vuota(HKCU, `A\B`) {
		t.Errorf("TogliAlbero: %v", err)
	}
}

// ZP-03, ZP03-14: i resti scritti da Windows che portano a Tasso Zip vanno via, gli altri restano.
func TestZP03RestiWindows(t *testing.T) {
	fe := `Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\`
	toasts := `Software\Microsoft\Windows\CurrentVersion\ApplicationAssociationToasts`
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(Zip)); err != nil {
		t.Fatal(err)
	}
	r := p.reg
	r.ScriviTesto(HKCU, fe+`.zip\OpenWithProgids`, progIDZip, "")
	r.ScriviTesto(HKCU, fe+`.zip\OpenWithProgids`, "7-Zip.zip", "")
	r.ScriviTesto(HKCU, fe+`.zip\OpenWithList`, "a", "7zFM.exe")
	r.ScriviTesto(HKCU, fe+`.zip\OpenWithList`, "b", "TassoZip.exe")
	r.ScriviTesto(HKCU, fe+`.zip\OpenWithList`, "MRUList", "ba")
	r.ScriviTesto(HKCU, toasts, progIDZip+"_.zip", "")
	r.ScriviTesto(HKCU, toasts, `Applications\TassoZip.exe_.zip`, "")
	r.ScriviTesto(HKCU, toasts, "7-Zip.zip_.zip", "")
	if _, err := p.in.Disinstalla(p.base, []Componente{Zip}); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.LeggiTesto(HKCU, fe+`.zip\OpenWithProgids`, progIDZip); ok {
		t.Error("OpenWithProgids di Tasso Zip rimasto")
	}
	if _, ok := r.LeggiTesto(HKCU, fe+`.zip\OpenWithList`, "b"); ok {
		t.Error("OpenWithList di Tasso Zip rimasto")
	}
	if v, _ := r.LeggiTesto(HKCU, fe+`.zip\OpenWithList`, "MRUList"); v != "a" {
		t.Errorf("MRUList = %q", v)
	}
	for _, n := range []string{progIDZip + "_.zip", `Applications\TassoZip.exe_.zip`} {
		if _, ok := r.LeggiTesto(HKCU, toasts, n); ok {
			t.Errorf("rimasto %s", n)
		}
	}
	for k, n := range map[string]string{fe + `.zip\OpenWithProgids`: "7-Zip.zip", fe + `.zip\OpenWithList`: "a", toasts: "7-Zip.zip_.zip"} {
		if _, ok := r.LeggiTesto(HKCU, k, n); !ok {
			t.Errorf("tolto %s\\%s, che non è di Tasso Zip", k, n)
		}
	}
}
