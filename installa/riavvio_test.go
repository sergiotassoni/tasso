package installa

import (
	"reflect"
	"testing"
)

func TestTogliCancellazioniPendenti(t *testing.T) {
	base := `C:\Program Files\Tasso`
	voci := []string{
		`\??\C:\Program Files\Tasso\Tasso.exe`, "", // via: è il Tasso nuovo
		`\??\C:\Program Files\Tasso\lavoro\tmp\a.txt`, "", // via
		`\??\C:\Program Files\Tasso\lavoro`, "", // via
		`\??\C:\Program Files\Tasso\Tasso.exe.vecchio-1`, "", // resta: copia da buttare
		`\??\C:\Program Files\Tasso\.tasso-setup-vecchio\tasso\Tasso.exe`, "", // resta
		`\??\C:\Program Files\Tasso\Tasso.exe.old`, "", // resta
		`\??\C:\Program Files\TassoAltro\x.dll`, "", // resta: fuori da Tasso
		`\??\C:\Program Files\Tasso\a.tmp`, `\??\C:\Program Files\Tasso\b.dll`, // resta: rinomina vera
		`\??\C:\Windows\Temp\y.tmp`, "", // resta
		`\??\C:\Program Files\Tasso`, "", // via: la cartella stessa
	}
	got, n := TogliCancellazioniPendenti(voci, base, []Programma{ProgTasso, ProgSetup})
	want := []string{
		`\??\C:\Program Files\Tasso\Tasso.exe.vecchio-1`, "",
		`\??\C:\Program Files\Tasso\.tasso-setup-vecchio\tasso\Tasso.exe`, "",
		`\??\C:\Program Files\Tasso\Tasso.exe.old`, "",
		`\??\C:\Program Files\TassoAltro\x.dll`, "",
		`\??\C:\Program Files\Tasso\a.tmp`, `\??\C:\Program Files\Tasso\b.dll`,
		`\??\C:\Windows\Temp\y.tmp`, "",
	}
	if n != 4 || !reflect.DeepEqual(got, want) {
		t.Fatalf("tolte %d\n%q", n, got)
	}
	// non a coppie: non tocco niente
	dispari := []string{`\??\C:\Program Files\Tasso\Tasso.exe`}
	if g, n := TogliCancellazioniPendenti(dispari, base, []Programma{ProgTasso}); n != 0 || len(g) != 1 {
		t.Fatal("voci dispari toccate")
	}
	// reinstallo solo Tasso: Zip e PDF tolti prima restano da cancellare; il disinstallatore no
	solo := []string{
		`\??\C:\Program Files\Tasso\Tasso Zip\TassoZip.exe`, "", // resta: Zip non si reinstalla
		`\??\C:\Program Files\Tasso\Tasso Zip`, "", // resta
		`\??\C:\Program Files\Tasso\tasso pdf\TassoPDF.exe`, "", // resta
		`\??\C:\Program Files\Tasso\Disinstalla Tasso.exe`, "", // via: si riscrive sempre
		`\??\C:\Program Files\Tasso\Tasso.exe`, "", // via
	}
	g, n := TogliCancellazioniPendenti(solo, base, []Programma{ProgTasso, ProgSetup})
	if n != 2 || !reflect.DeepEqual(g, solo[:6]) {
		t.Fatalf("solo Tasso: tolte %d\n%q", n, g)
	}
	// reinstallo solo Zip: via solo i file di Zip (e il disinstallatore), Tasso.exe resta
	g, n = TogliCancellazioniPendenti(solo, base, []Programma{ProgZip, ProgSetup})
	want = []string{solo[4], "", solo[8], ""}
	if n != 3 || !reflect.DeepEqual(g, want) {
		t.Fatalf("solo Zip: tolte %d\n%q", n, g)
	}
}
