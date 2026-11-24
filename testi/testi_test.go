package testi

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

var segnaposto = regexp.MustCompile(`%[a-z]`)

// TestChiavi: ogni lingua ha esattamente le chiavi dell'italiano, nessun testo vuoto,
// gli stessi segnaposto (%s) nello stesso numero.
func TestChiavi(t *testing.T) {
	it := Dizionario["it"]
	if len(it) < 50 {
		t.Fatalf("italiano con poche chiavi: %d", len(it))
	}
	if len(Dizionario) != len(Lingue) {
		t.Fatalf("%d lingue nel dizionario, %d nel selettore", len(Dizionario), len(Lingue))
	}
	for _, l := range Lingue {
		d, ok := Dizionario[l.Codice]
		if !ok {
			t.Errorf("manca la lingua %s", l.Codice)
			continue
		}
		var mancano, inPiu []string
		for k := range it {
			if _, ok := d[k]; !ok {
				mancano = append(mancano, k)
			}
		}
		for k, v := range d {
			if _, ok := it[k]; !ok {
				inPiu = append(inPiu, k)
			}
			if strings.TrimSpace(v) == "" {
				t.Errorf("%s/%s vuoto", l.Codice, k)
			}
			a, b := segnaposto.FindAllString(it[k], -1), segnaposto.FindAllString(v, -1)
			if strings.Join(a, "") != strings.Join(b, "") {
				t.Errorf("%s/%s: segnaposto %v invece di %v", l.Codice, k, b, a)
			}
		}
		sort.Strings(mancano)
		sort.Strings(inPiu)
		if len(mancano) > 0 {
			t.Errorf("%s: mancano %v", l.Codice, mancano)
		}
		if len(inPiu) > 0 {
			t.Errorf("%s: chiavi in più %v", l.Codice, inPiu)
		}
	}
}

func TestLinguaWindows(t *testing.T) {
	casi := map[uint16]string{0x0410: "it", 0x0810: "it", 0x0409: "en", 0x0C0A: "es", 0x040C: "fr",
		0x0407: "de", 0x0416: "pt", 0x0816: "pt", 0x0415: "pl", 0x0411: "en"}
	for id, l := range casi {
		if g := DaWindows(id); g != l {
			t.Errorf("%#x: %s invece di %s", id, g, l)
		}
	}
	if Normalizza("PT-br") != "pt" || Normalizza("xx") != "" || Normalizza(" it ") != "it" {
		t.Error("Normalizza")
	}
	if T("de", "avanti") != "Weiter" || T("xx", "avanti") != "Avanti" || T("it", "nessuna") != "nessuna" {
		t.Error("T")
	}
}
