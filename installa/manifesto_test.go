package installa

import (
	"strings"
	"testing"
)

func TestManifesto(t *testing.T) {
	_, e := pacchetto(t, "3.2")
	versioni(e, map[Programma]string{ProgZip: "1.3.0", ProgPDF: "1.1.0"})
	sha := strings.Repeat("ab", 32)
	m, err := Manifesto(strings.ToUpper(sha), e)
	if err != nil {
		t.Fatal(err)
	}
	atteso := sha + "  TassoSetup.exe  3.2\ntasso 3.2\nzip 1.3.0\npdf 1.1.0\n"
	if m != atteso {
		t.Fatalf("manifesto:\n%s\natteso:\n%s", m, atteso)
	}
	if _, err := Manifesto("123", e); err == nil {
		t.Fatal("impronta corta accettata")
	}
	versioni(e, map[Programma]string{ProgZip: "1.3 beta"})
	if _, err := Manifesto(sha, e); err == nil {
		t.Fatal("versione strana accettata")
	}
}
