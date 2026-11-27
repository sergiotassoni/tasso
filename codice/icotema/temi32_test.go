package icotema

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// 3.2: Salvia e Lavanda tolte dalla scelta dell'Ufficio, al loro posto Carta e Perla; le loro icone
// e i loro numeri restano (i collegamenti vecchi puntano lì) e nessun tema nuovo li riusa.
func TestTemiUfficioRitirati(t *testing.T) {
	for vecchio, nuovo := range map[string]string{"salvia": "carta", "lavanda": "perla"} {
		if n, ok := TemaUfficioSostituto(vecchio); !ok || n != nuovo {
			t.Errorf("%s: sostituto %q %v, voglio %q", vecchio, n, ok, nuovo)
		}
		if contiene(TemiUfficio, vecchio) {
			t.Errorf("%s si sceglie ancora", vecchio)
		}
		if !contiene(TemiUfficio, nuovo) {
			t.Errorf("il sostituto %s non si sceglie", nuovo)
		}
	}
	for _, tema := range append(append([]string{"notte", ""}, TemiUfficio...), TemiGaming...) {
		if _, ok := TemaUfficioSostituto(tema); ok {
			t.Errorf("%q non è ritirato", tema)
		}
	}
	want := []string{"ufficio", "carta", "perla", "grafite", "bosco", "prugna", "petrolio", "tortora", "ardesia"}
	if strings.Join(TemiUfficio, ",") != strings.Join(want, ",") {
		t.Errorf("fila dell'Ufficio %v", TemiUfficio)
	}
	b, err := os.ReadFile("../assets/temi/elenco.txt")
	if err != nil {
		t.Fatal(err)
	}
	e, err := Leggi(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		tema, variante string
		id             uint16
	}{{"lavanda", Gaming, 217}, {"lavanda", Ufficio, 218}, {"salvia", Gaming, 231}, {"salvia", Ufficio, 232}, {"ufficio", Ufficio, 234}} {
		if g := e.Trova(c.tema, c.variante); g != c.id {
			t.Errorf("%s %s: numero %d, deve restare %d", c.tema, c.variante, g, c.id)
		}
		if _, err := os.Stat(filepath.Join("..", "assets", "temi", c.tema, "tasso-"+c.variante+".ico")); err != nil {
			t.Errorf("%s %s: l'icona deve restare dentro Tasso.exe: %v", c.tema, c.variante, err)
		}
	}
	for _, tema := range []string{"grafite", "bosco", "prugna", "petrolio", "tortora"} {
		for _, v := range []string{Gaming, Ufficio} {
			if id := e.Trova(tema, v); id <= 234 {
				t.Errorf("%s %s: numero %d (i numeri nuovi partono da 235)", tema, v, id)
			}
			for _, est := range []string{".ico", ".svg"} {
				if _, err := os.Stat(filepath.Join("..", "assets", "temi", tema, "tasso-"+v+est)); err != nil {
					t.Errorf("%s: %v", tema, err)
				}
			}
		}
	}
}

// Tutti i temi che si scelgono (e quelli ritirati) sono accettati da settings.go.
func TestTemiValidiInSettings(t *testing.T) {
	b, err := os.ReadFile("../settings.go")
	if err != nil {
		t.Skip("settings.go non trovato:", err)
	}
	m := regexp.MustCompile(`(?s)var validThemes = map\[string\]bool\{(.*?)\}`).FindSubmatch(b)
	if m == nil {
		t.Fatal("validThemes non trovato")
	}
	for _, tema := range append(append([]string{"salvia", "lavanda", "notte"}, TemiUfficio...), TemiGaming...) {
		if !strings.Contains(string(m[1]), `"`+tema+`": true`) {
			t.Errorf("%s non è in validThemes", tema)
		}
	}
}

// Colori dei temi dell'Ufficio (THEMES in ui/index.html): ogni scritta si legge, almeno 4,5:1
// (WCAG AA), anche il colore principale sulla voce attiva del menu e la scritta sui pulsanti
// (normali, gialli e rossi); a2, a3, a4 e rosso sia sui pannelli sia sullo sfondo.
// I temi scuri non sono nell'elenco dei temi chiari di applyTheme.
func TestContrastoTemiUfficio(t *testing.T) {
	ui, err := os.ReadFile("../ui/index.html")
	if err != nil {
		t.Skip("ui/index.html non trovato:", err)
	}
	temi := map[string]map[string]string{}
	riga := regexp.MustCompile(`\{id:'([a-z0-9]+)',[^\n]*?v:\{([^}]*)\}`)
	coppia := regexp.MustCompile(`([a-z0-9]+):'(#[0-9A-Fa-f]{6})'`)
	for _, r := range riga.FindAllSubmatch(ui, -1) {
		v := map[string]string{}
		for _, c := range coppia.FindAllSubmatch(r[2], -1) {
			v[string(c[1])] = string(c[2])
		}
		temi[string(r[1])] = v
	}
	m := regexp.MustCompile(`const light = \[([^\]]*)\]`).FindSubmatch(ui)
	if m == nil {
		t.Fatal("elenco light di applyTheme non trovato")
	}
	chiari := string(m[1])
	scuri := map[string]bool{"grafite": true, "bosco": true, "prugna": true, "petrolio": true, "tortora": true, "ardesia": true}
	for _, id := range TemiUfficio {
		v := temi[id]
		if v == nil {
			t.Errorf("%s non è in THEMES", id)
			continue
		}
		manca := false
		for _, k := range []string{"bg", "side", "panel", "line", "text", "muted", "a1", "a2", "a3", "a4", "ink", "red"} {
			if v[k] == "" {
				t.Errorf("%s: manca %s", id, k)
				manca = true
			}
		}
		if manca {
			continue
		}
		menu := mescola(v["a1"], v["panel"], 0.13) // voce attiva del menu in Ufficio (come applyTheme)
		warnBg, onWarn := pulsanteGiallo(v["a4"])
		// --a1t: il colore principale usato come testo, avvicinato al testo da applyTheme solo quanto serve
		a1t := coloreTesto(v, []string{v["bg"], v["panel"], v["side"], mescola(v["text"], v["panel"], 0.05), menu})
		if scuri[id] && a1t != strings.ToLower(v["a1"]) {
			t.Errorf("%s: il colore principale %s va corretto da Tasso (%s): nei temi nuovi deve leggersi già così", id, v["a1"], a1t)
		}
		for _, c := range []struct{ nome, a, b string }{
			{"testo/sfondo", v["text"], v["bg"]}, {"testo/pannello", v["text"], v["panel"]}, {"testo/barra", v["text"], v["side"]},
			{"grigio/sfondo", v["muted"], v["bg"]}, {"grigio/pannello", v["muted"], v["panel"]},
			{"principale/sfondo", v["a1"], v["bg"]}, {"principale/pannello", v["a1"], v["panel"]}, {"principale come testo/menu attivo", a1t, menu}, {"principale come testo/sfondo", a1t, v["bg"]},
			{"scritta sui pulsanti", v["ink"], v["a1"]},
			{"a2/pannello", v["a2"], v["panel"]}, {"a3/pannello", v["a3"], v["panel"]}, {"a4/pannello", v["a4"], v["panel"]}, {"rosso/pannello", v["red"], v["panel"]},
			{"a2/sfondo", v["a2"], v["bg"]}, {"a3/sfondo", v["a3"], v["bg"]}, {"a4/sfondo", v["a4"], v["bg"]}, {"rosso/sfondo", v["red"], v["bg"]},
			{"scritta sul pulsante giallo", onWarn, warnBg}, {"scritta sul pulsante rosso", onRed(v["red"]), v["red"]},
		} {
			if r := contrasto(c.a, c.b); r < 4.5 {
				t.Errorf("%s %s: %.2f:1 (serve almeno 4,5)", id, c.nome, r)
			}
		}
		if scuri[id] {
			if strings.Contains(chiari, "'"+id+"'") {
				t.Errorf("%s è scuro ma è nell'elenco light", id)
			}
			if luce(v["bg"]) > 0.1 {
				t.Errorf("%s: sfondo troppo chiaro per un tema scuro", id)
			}
			if v["side"] != v["panel"] {
				t.Errorf("%s: barra laterale e pannelli diversi (come Ardesia devono essere uguali)", id)
			}
		} else if !strings.Contains(chiari, "'"+id+"'") {
			t.Errorf("%s è chiaro ma non è nell'elenco light", id)
		}
	}
}

func rgb(h string) [3]float64 {
	n, _ := strconv.ParseUint(strings.TrimPrefix(h, "#"), 16, 32)
	return [3]float64{float64(n >> 16 & 255), float64(n >> 8 & 255), float64(n & 255)}
}

func mescola(a, b string, p float64) string {
	x, y := rgb(a), rgb(b)
	s := "#"
	for i := 0; i < 3; i++ {
		s += strconv.FormatUint(uint64(math.Round(x[i]*p+y[i]*(1-p)))|0x100, 16)[1:]
	}
	return s
}

// coloreTesto: come --a1t in applyTheme (ui/index.html).
func coloreTesto(v map[string]string, fondi []string) string {
	at := strings.ToLower(v["a1"])
	minimo := func(c string) float64 {
		m := 99.0
		for _, f := range fondi {
			m = math.Min(m, contrasto(c, f))
		}
		return m
	}
	for p := 1.0; p >= 0.4 && minimo(at) < 4.6; p -= 0.02 {
		at = mescola(v["a1"], v["text"], p)
	}
	return at
}

// pulsanteGiallo: come --warnbg e --onwarn in applyTheme (ui/index.html): scritta bianca o scura,
// quella che si legge meglio; se non basta, il fondo si scurisce (o schiarisce) un poco.
func pulsanteGiallo(a4 string) (fondo, scritta string) {
	const bianco, scuro = "#ffffff", "#241500"
	scritta = scuro
	verso := "#ffffff"
	if contrasto(bianco, a4) >= contrasto(scuro, a4) {
		scritta, verso = bianco, "#000000"
	}
	fondo = strings.ToLower(a4)
	for p := 1.0; p >= 0.6 && contrasto(scritta, fondo) < 4.6; p -= 0.02 {
		fondo = mescola(a4, verso, p)
	}
	return fondo, scritta
}

// onRed: come --onred in applyTheme: bianca sui rossi scuri, quasi nera sui rossi chiari.
func onRed(red string) string {
	if luce(red) > 0.22 {
		return "#1a0b09"
	}
	return "#ffffff"
}

func luce(h string) float64 {
	c := rgb(h)
	f := func(x float64) float64 {
		x /= 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*f(c[0]) + 0.7152*f(c[1]) + 0.0722*f(c[2])
}

func contrasto(a, b string) float64 {
	x, y := luce(a), luce(b)
	return (math.Max(x, y) + 0.05) / (math.Min(x, y) + 0.05)
}
