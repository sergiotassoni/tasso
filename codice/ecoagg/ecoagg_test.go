package ecoagg

import (
	"strings"
	"testing"
)

var sha = strings.Repeat("0f", 32)

func TestLeggiManifesto(t *testing.T) {
	m, err := LeggiManifesto([]byte(strings.ToUpper(sha) + "  TassoSetup.exe  3.2\r\ntasso 3.2\nzip 1.3.0\npdf 1.1.0\nnuova-cosa 1 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.SHA256 != sha || m.Versione != "3.2" || m.Versioni["tasso"] != "3.2" || m.Versioni["zip"] != "1.3.0" || m.Versioni["pdf"] != "1.1.0" {
		t.Fatalf("%+v", m)
	}
	cattivi := []string{
		"",
		"<html>404</html>",
		sha + "  Tasso.zip  3.2\ntasso 3.2",
		sha + "  TassoSetup.exe  3.2-beta\ntasso 3.2",
		"1234  TassoSetup.exe  3.2\ntasso 3.2",
		sha + "  TassoSetup.exe  3.2\n", // nessuna versione dei programmi
		sha + "  TassoSetup.exe  3.2\ntasso <b>",
	}
	for _, c := range cattivi {
		if _, err := LeggiManifesto([]byte(c)); err == nil {
			t.Errorf("accettato: %q", c)
		}
	}
}

func TestPiuNuova(t *testing.T) {
	if !PiuNuova("3.10", "3.9") || !PiuNuova("1.2.1", "1.2") || PiuNuova("1.2", "1.2.0") || PiuNuova("3.1", "3.2") || !PiuNuova("1.0", "") || PiuNuova("x", "1") {
		t.Fatal("PiuNuova")
	}
}

func TestComponenti(t *testing.T) {
	st, err := LeggiStato([]byte(`{"versione":"3.1","lingua":"de","componenti":["ufficio","pdf"],
		"programmi":{"tasso":{"versione":"3.1"},"pdf":{"versione":"1.1.0"},"setup":{"versione":"3.1"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	// senza manifesto: solo le versioni installate (Tasso: quella che gira, se è più nuova)
	cc := Componenti(st, nil, "3.1.1")
	if len(cc) != 2 || cc[0].Nome != "Tasso" || cc[0].Installata != "3.1.1" || cc[1].ID != "pdf" || cc[1].Disponibile != "" || QualcosaDiNuovo(cc) {
		t.Fatalf("%+v", cc)
	}
	m, _ := LeggiManifesto([]byte(sha + "  TassoSetup.exe  3.2\ntasso 3.1.1\nzip 2.0\npdf 1.2.0\n"))
	cc = Componenti(st, &m, "3.1.1")
	if cc[0].Nuova || !cc[1].Nuova || cc[1].Disponibile != "1.2.0" || !QualcosaDiNuovo(cc) {
		t.Fatalf("%+v", cc)
	}
	// Zip non installato: non conta, anche se è nuovo
	m, _ = LeggiManifesto([]byte(sha + "  TassoSetup.exe  3.2\ntasso 3.0\nzip 9.0\npdf 1.0\n"))
	if QualcosaDiNuovo(Componenti(st, &m, "3.1")) {
		t.Fatal("mai indietro, e Zip non c'è")
	}
	if _, err := LeggiStato([]byte("{rotto")); err == nil {
		t.Fatal("json rotto accettato")
	}
}
