// Controllo dei file delle lingue di Tasso (en.json, es.json, fr.json, de.json).
// Gira anche su Linux: go test ./i18n/
package i18n

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Le lingue tradotte oltre all'inglese (l'italiano è nel codice).
var lingue = []string{"es", "fr", "de"}

type diz map[string]map[string]string // sezione (ui, go, ps) -> testo italiano -> traduzione

func leggi(t *testing.T, l string) diz {
	t.Helper()
	b, err := os.ReadFile(l + ".json")
	if err != nil {
		t.Fatalf("%s.json: %v", l, err)
	}
	var d diz
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("%s.json non è JSON valido: %v", l, err)
	}
	return d
}

// segni: le parti che una traduzione deve tenere uguali: segnaposto ({0}, {INVIO}, %d, %s...),
// tag HTML e a capo.
var reSegni = regexp.MustCompile(`\{[^{}]*\}|%[-+0-9.]*[sdvfqx]|%%|<[^<>]+>|\\n|\n|&[a-z]+;`)

func segni(s string) []string {
	m := reSegni.FindAllString(s, -1)
	sort.Strings(m)
	return m
}

func spaziAiLati(s string) (string, string) {
	a := s[:len(s)-len(strings.TrimLeft(s, " \t\n"))]
	b := s[len(strings.TrimRight(s, " \t\n")):]
	return a, b
}

// Ogni lingua ha tutte le chiavi dell'inglese, nessuna in più, nessuna vuota, con gli stessi
// segnaposto, gli stessi tag HTML e gli stessi spazi all'inizio e alla fine (molti testi sono
// pezzi di frase che si attaccano a numeri e nomi).
func TestLingueComplete(t *testing.T) {
	en := leggi(t, "en")
	for _, sez := range []string{"ui", "go", "ps"} {
		if len(en[sez]) == 0 {
			t.Fatalf("en.json: sezione %q vuota", sez)
		}
	}
	for _, l := range lingue {
		d := leggi(t, l)
		for sez := range d {
			if _, ok := en[sez]; !ok {
				t.Errorf("%s.json: sezione in più %q", l, sez)
			}
		}
		mancano, diversi := 0, 0
		for sez, voci := range en {
			for k, ve := range voci {
				v, ok := d[sez][k]
				if !ok || strings.TrimSpace(v) == "" && strings.TrimSpace(ve) != "" {
					mancano++
					if mancano <= 20 {
						t.Errorf("%s.json/%s: manca %q", l, sez, k)
					}
					continue
				}
				if a, b := segni(ve), segni(v); strings.Join(a, "\x00") != strings.Join(b, "\x00") {
					diversi++
					if diversi <= 40 {
						t.Errorf("%s.json/%s: segnaposto o tag diversi in %q\n en: %q\n %s: %q", l, sez, k, a, l, b)
					}
				}
				// Lo spazio all'inizio o alla fine dell'inglese deve restare (si attacca a numeri e nomi);
				// all'inizio può mancare solo se la traduzione comincia con la punteggiatura (", bevor…").
				// In più va bene (in francese "45 %", "Mode :").
				ea, eb := spaziAiLati(ve)
				la, lb := spaziAiLati(v)
				if ea != "" && la == "" && !strings.ContainsAny(v[:1], ",.:;)!?") || eb != "" && lb == "" {
					diversi++
					if diversi <= 40 {
						t.Errorf("%s.json/%s: spazio all'inizio o alla fine perso in %q (en %q, %s %q)", l, sez, k, ve, l, v)
					}
				}
			}
			for k := range d[sez] {
				if _, ok := voci[k]; !ok {
					t.Errorf("%s.json/%s: chiave in più %q", l, sez, k)
				}
			}
		}
		if mancano > 0 {
			t.Errorf("%s.json: mancano %d testi in tutto", l, mancano)
		}
	}
}

// I nomi dei programmi restano quelli di Tasso.
func TestNomiTasso(t *testing.T) {
	en := leggi(t, "en")
	for _, l := range lingue {
		d := leggi(t, l)
		for sez, voci := range en {
			for k, ve := range voci {
				v, ok := d[sez][k]
				if !ok {
					continue // lo segnala già TestLingueComplete
				}
				for _, n := range []string{"Tasso Zip", "Tasso PDF", "Tasso"} {
					if strings.Count(ve, n) > 0 && strings.Count(v, n) == 0 {
						t.Errorf("%s.json/%s: manca il nome %q in %q", l, sez, n, v)
						break
					}
				}
			}
		}
	}
}
