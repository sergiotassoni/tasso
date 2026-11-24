//go:build collaudo

package traduce

import (
	"encoding/json"
	"os"
	"testing"
)

// go test -tags collaudo -run TestRitoccoSu1000 (COLLAUDO=collaudo.json)
func TestRitoccoSu1000(t *testing.T) {
	b, err := os.ReadFile(os.Getenv("COLLAUDO"))
	if err != nil {
		t.Skip(err)
	}
	var c struct {
		Mozilla EsitoProva `json:"mozilla"`
	}
	json.Unmarshal(b, &c)
	var h0, h1, ref []string
	cambiate := 0
	for _, f := range c.Mozilla.Frasi {
		h0 = append(h0, f.Uscita)
		n := correggiTerminiFrase(f.EN, f.Uscita)
		if n != f.Uscita {
			cambiate++
			if cambiate <= 40 {
				t.Logf("%s\n   %s\n   %s", f.EN, f.Uscita, n)
			}
		}
		h1 = append(h1, n)
		ref = append(ref, f.IT)
	}
	t.Logf("cambiate %d · chrF++ %.2f -> %.2f", cambiate, chrFpp(h0, ref), chrFpp(h1, ref))
}
