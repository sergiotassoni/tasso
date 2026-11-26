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

func TestMotivi(t *testing.T) {
	v := Motivi("en", []string{"foto.lnk (collegamento)", "a/b.txt (rovinato)", "… (resto_illeggibile)", "strano (2) (sconosciuto)", "senza"})
	atteso := []string{"foto.lnk (link)", "a/b.txt (damaged data)", "… (the rest of the archive is unreadable)", "strano (2) (sconosciuto)", "senza"}
	for i := range atteso {
		if v[i] != atteso[i] {
			t.Errorf("%d: %q, atteso %q", i, v[i], atteso[i])
		}
	}
	// ogni codice usato dal programma ha il suo testo in tutte le lingue
	for _, c := range []string{"collegamento", "percorso_non_sicuro", "file_speciale", "non_leggibile", "rovinato",
		"resto_illeggibile", "incompleto", "intestazione_rovinata", "con_password", "dati_rovinati"} {
		for _, l := range Lingue {
			if _, ok := Tutti()[l]["motivo_"+c]; !ok {
				t.Errorf("%s: manca motivo_%s", l, c)
			}
		}
	}
}

func TestDettagli(t *testing.T) {
	d := Dettagli("de", "archivio rovinato o incompleto (sevenzip: bad header)")
	if strings.Contains(d, "rovinato") || !strings.Contains(d, "sevenzip: bad header") {
		t.Fatalf("dettagli: %q", d)
	}
	if Dettagli("it", "password sbagliata") != "password sbagliata" {
		t.Fatal("in italiano resta com'è")
	}
	for _, f := range frasiErrori {
		if T("en", f.chiave) == f.chiave {
			t.Errorf("manca il testo %s", f.chiave)
		}
	}
}

// Nessun nome di altri programmi per archivi nei testi della finestra.
func TestNiente7ZipWinRAR(t *testing.T) {
	for l, m := range Tutti() {
		for k, v := range m {
			for _, x := range []string{"7-Zip", "7zip", "WinRAR", "WinZip", "PeaZip", "NanaZip"} {
				if strings.Contains(strings.ToLower(v), strings.ToLower(x)) {
					t.Errorf("%s/%s nomina %s: %q", l, k, x, v)
				}
			}
		}
	}
}
