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
	got, n := TogliCancellazioniPendenti(voci, base)
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
	if g, n := TogliCancellazioniPendenti(dispari, base); n != 0 || len(g) != 1 {
		t.Fatal("voci dispari toccate")
	}
}
