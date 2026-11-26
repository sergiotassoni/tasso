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

// I testi fissi della pagina Impostazioni cambiano lingua: ogni pezzo di testo scritto nella
// pagina (tra un tag e l'altro, come li traduce translateStatic) ha la sua traduzione in tutte
// le lingue. Restano fuori solo i nomi delle lingue, i nomi dei pulsanti di Discord (che su
// Discord sono in inglese) e il comando !stato.
func TestImpostazioniTradotte(t *testing.T) {
	b, err := os.ReadFile("../ui/index.html")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	a := strings.Index(s, `<section class="view" id="v-settings">`)
	if a < 0 {
		t.Fatal("pagina Impostazioni non trovata")
	}
	f := strings.Index(s[a:], "</section>")
	pag := s[a : a+f]
	fuori := map[string]bool{"Lingua · Language": true, "!stato": true, "TASSO": true,
		"Italiano": true, "English": true, "Español": true, "Français": true, "Deutsch": true,
		"New Application": true, "Create": true, "Bot": true, "Reset Token": true, "Copy": true,
		"Message Content Intent": true, "Save Changes": true}
	lettere := regexp.MustCompile(`[A-Za-zÀ-ÿ]`)
	tutte := map[string]diz{"en": leggi(t, "en")}
	for _, l := range lingue {
		tutte[l] = leggi(t, l)
	}
	for _, pezzo := range regexp.MustCompile(`<[^>]*>`).Split(pag, -1) {
		k := strings.TrimSpace(pezzo)
		if k == "" || !lettere.MatchString(k) || fuori[k] {
			continue
		}
		for l, d := range tutte {
			if _, ok := d["ui"][k]; !ok {
				t.Errorf("%s.json/ui: manca il testo delle Impostazioni %q", l, k)
			}
		}
	}
	// Il sottotitolo della lingua: in ogni lingua c'è la frase nella lingua stessa, non l'italiano.
	k := "Tasso si riapre nella lingua scelta. · Tasso reopens in the language you choose."
	for l, d := range tutte {
		if v := d["ui"][k]; v == "" || strings.Contains(v, "si riapre") {
			t.Errorf("%s.json/ui: il sottotitolo della lingua è ancora in italiano: %q", l, v)
		}
	}
}

// Il report su Discord (discord.go) passa da trAnyGo: ogni etichetta con l'emoji deve avere la sua traduzione,
// se no chi usa Tasso in un'altra lingua riceve il report in italiano.
func TestReportDiscordTradotto(t *testing.T) {
	b, err := os.ReadFile("../discord.go")
	if err != nil {
		t.Skip("discord.go non trovato")
	}
	src := string(b)
	etichette := []string{"💯 Salute", "⚙️ Modalità", "⏱️ Acceso da", "🧹 Ultima pulizia", "🔁 Prossimo report", "💾 Dischi",
		"Il PC è a posto 👍", "Già a posto:", "🦡 Stato del PC", " · solo lettura", " · scrivi !stato per un report"}
	tutte := map[string]diz{"en": leggi(t, "en")}
	for _, l := range lingue {
		tutte[l] = leggi(t, l)
	}
	for _, e := range etichette {
		if !strings.Contains(src, e) {
			t.Errorf("discord.go non usa più %q: aggiorna questo test", e)
		}
		for l, d := range tutte {
			if d["go"][e] == "" {
				t.Errorf("%s.json/go: manca l'etichetta del report Discord %q", l, e)
			}
		}
	}
	// Nelle altre lingue il comando del bot è !report (lo accetta anche il bot, discord.go: botAsk).
	for l, d := range tutte {
		for sez, m := range d {
			for k, v := range m {
				if strings.Contains(k, "!stato") && strings.Contains(v, "!stato") {
					t.Errorf("%s.json/%s: %q dice ancora !stato", l, sez, k)
				}
			}
		}
	}
}

// La pagina "Licenze dei componenti" dentro Tasso deve dire esattamente quello che c'è in
// dist/LICENZE.txt e dist/LICENSES.txt (l'inglese vale per tutte le lingue diverse dall'italiano).
func TestLicenzeNellaFinestra(t *testing.T) {
	b, err := os.ReadFile("../ui/index.html")
	if err != nil {
		t.Skip("index.html non trovato")
	}
	s := string(b)
	const inizio = "<script>(function(){var L="
	i := strings.Index(s, inizio)
	j := strings.Index(s[i+1:], ";function f(){")
	if i < 0 || j < 0 {
		t.Fatal("non trovo le licenze in index.html")
	}
	var L map[string]string
	if err := json.Unmarshal([]byte(s[i+len(inizio):i+1+j]), &L); err != nil {
		t.Fatal(err)
	}
	for l, f := range map[string]string{"it": "../dist/LICENZE.txt", "en": "../dist/LICENSES.txt"} {
		d, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		txt := strings.Trim(strings.ReplaceAll(strings.TrimPrefix(string(d), "\ufeff"), "\r\n", "\n"), "\n")
		if L[l] != txt {
			t.Errorf("licenze %s in index.html diverse da %s: rigenerale dal file", l, f)
		}
	}
	if !strings.Contains(s[i:], `(l==="it")?L.it:L.en`) {
		t.Error("le lingue diverse dall'italiano devono vedere le licenze in inglese")
	}
}

// I testi della sezione "ps" finiscono dentro stringhe tra apici degli script PowerShell: un apice
// tipografico (‘ ’ ‚ ‛) per PowerShell chiude la stringa e romperebbe lo script (o peggio).
func TestPsSenzaApiciTipografici(t *testing.T) {
	for _, l := range append([]string{"en"}, lingue...) {
		for k, v := range leggi(t, l)["ps"] {
			if strings.ContainsAny(v, "‘’‚‛") {
				t.Errorf("%s.json, ps %q: la traduzione contiene un apice tipografico, usa '", l, k)
			}
		}
	}
}
