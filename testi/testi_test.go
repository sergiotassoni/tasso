package testi

import (
	"strings"
	"testing"
)

// Ogni lingua deve avere tutte le chiavi dell'italiano, con gli stessi segnaposto {n}.
func TestTutteLeChiavi(t *testing.T) {
	d := Tutti()
	if len(d) != len(Lingue) {
		t.Fatalf("lingue nel file: %d, attese %d", len(d), len(Lingue))
	}
	it := d["it"]
	if len(it) < 50 {
		t.Fatalf("italiano incompleto: %d chiavi", len(it))
	}
	for _, l := range Lingue {
		m, ok := d[l]
		if !ok {
			t.Errorf("manca la lingua %s", l)
			continue
		}
		for k, v := range it {
			s, ok := m[k]
			if !ok || strings.TrimSpace(s) == "" {
				t.Errorf("%s: manca %q", l, k)
				continue
			}
			for _, seg := range []string{"{n}", "{i}", "{file}", "{orig}", "{comp}", "{perc}"} {
				if strings.Contains(v, seg) != strings.Contains(s, seg) {
					t.Errorf("%s/%s: segnaposto %s diverso", l, k, seg)
				}
			}
		}
		for k := range m {
			if _, ok := it[k]; !ok {
				t.Errorf("%s: chiave in più %q", l, k)
			}
		}
	}
}

func TestScegli(t *testing.T) {
	if Scegli("", "pt-BR") != "pt" || Scegli("ja-JP") != "en" || Scegli("IT") != "it" || Scegli("xx", "de_DE") != "de" {
		t.Error("Scegli")
	}
	if T("pl", "comprimi") != "Spakuj" || T("zz", "comprimi") != "Compress" {
		t.Error("T")
	}
}
