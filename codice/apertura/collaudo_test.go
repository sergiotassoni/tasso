package apertura

import (
	"reflect"
	"testing"
)

// SoloDaAttivita: la pulizia degli argomenti della build di prova (collaudo.go).
func TestSoloDaAttivita(t *testing.T) {
	argv := []string{"Tasso.exe", ArgDaAttivita, "x", "--passo=of_new_call", segno, "--guardia=6", "--min", "--collaudo"}
	if got := SoloDaAttivita(argv); !reflect.DeepEqual(got, []string{"Tasso.exe", ArgDaAttivita, segno, "--min"}) {
		t.Fatalf("%q", got)
	}
	if got := SoloDaAttivita([]string{"Tasso.exe", ArgDaAttivita, "--passo=x"}); !reflect.DeepEqual(got, []string{"Tasso.exe", ArgDaAttivita}) {
		t.Fatalf("senza segno: %q", got)
	}
	// non dall'attività: tale e quale (le prove normali funzionano)
	n := []string{"Tasso.exe", "--passo=of_new_call", ArgDaAttivita}
	if got := SoloDaAttivita(n); !reflect.DeepEqual(got, n) {
		t.Fatalf("%q", got)
	}
}
