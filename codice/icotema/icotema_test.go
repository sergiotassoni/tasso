package icotema

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestLeggiScriviTrova(t *testing.T) {
	e, err := Leggi([]byte("# prova\n\n202 oro ufficio\n201 oro gaming\n203 neonrgb gaming\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Voci) != 3 || e.Voci[0].ID != 201 {
		t.Fatalf("voci %+v", e.Voci)
	}
	if e.IDPer("oro", Gaming) != 201 || e.IDPer("oro", Ufficio) != 202 {
		t.Fatal("oro sbagliato")
	}
	// Ufficio senza icona sua: quella Gaming dello stesso tema
	if e.IDPer("neonrgb", Ufficio) != 203 {
		t.Fatal("neonrgb ufficio")
	}
	// tema senza icone, e tema di sempre: icona normale
	for _, tema := range []string{"tasso", "aurora", "", "ufficio"} {
		if e.IDPer(tema, Gaming) != IDNormale || e.IDPer(tema, Ufficio) != IDNormale {
			t.Fatalf("%q dovrebbe avere l'icona normale", tema)
		}
	}
	e2, err := Leggi(e.Scrivi())
	if err != nil || len(e2.Voci) != 3 || e2.Trova("neonrgb", Gaming) != 203 {
		t.Fatalf("andata e ritorno: %v %+v", err, e2.Voci)
	}
}

func TestLeggiSbagliato(t *testing.T) {
	for _, s := range []string{
		"201 oro",                            // poche parole
		"20 oro gaming",                      // numero troppo basso (2 è l'icona normale)
		"1000 oro gaming",                    // numero troppo alto
		"201 Oro gaming",                     // maiuscole
		"201 tasso gaming",                   // il tema di sempre non ha un'icona a parte
		"201 oro/.. gaming",                  // niente percorsi
		"201 oro grande",                     // variante sconosciuta
		"201 oro gaming\n201 argento gaming", // numero ripetuto
		"201 oro gaming\n202 oro gaming",     // tema ripetuto
	} {
		if _, err := Leggi([]byte(s)); err == nil {
			t.Errorf("%q accettato", s)
		}
	}
}

func TestAggiungiNumeriStabili(t *testing.T) {
	e, _ := Leggi([]byte("201 oro gaming\n203 argento gaming\n"))
	if id, _ := e.Aggiungi("oro", Gaming); id != 201 {
		t.Fatalf("oro deve restare 201, è %d", id)
	}
	if id, _ := e.Aggiungi("neonrgb", Gaming); id != 202 {
		t.Fatalf("primo libero 202, è %d", id)
	}
	if id, _ := e.Aggiungi("argento", Ufficio); id != 204 {
		t.Fatalf("poi 204, è %d", id)
	}
	if _, err := e.Aggiungi("tasso", Gaming); err == nil {
		t.Fatal("tasso accettato")
	}
	if !strings.Contains(string(e.Scrivi()), "202 neonrgb gaming") {
		t.Fatal(string(e.Scrivi()))
	}
}

func TestTemaInUsoEPosizione(t *testing.T) {
	cases := []struct{ prof, tema, uff, want string }{
		{"gioco", "oro", "carta", "oro"},
		{"", "", "", "tasso"},
		{"ufficio", "oro", "", "ufficio"},
		{"ufficio", "oro", "argento", "ufficio"}, // argento è un tema Gaming: in Ufficio si vede Azzurro
		{"ufficio", "oro", "carta", "carta"},
		{"gioco", "notte", "", "tasso"}, // notte non si sceglie: l'interfaccia mostra Tasso
		{"ufficio", "", "notte", "ufficio"},
		{"gioco", "carta", "", "tasso"}, // tema dell'Ufficio salvato in Gaming
		{"entrambi", "argento", "carta", "argento"},
	}
	for _, c := range cases {
		if g := TemaInUso(c.prof, c.tema, c.uff); g != c.want {
			t.Errorf("TemaInUso(%q,%q,%q) = %q, voglio %q", c.prof, c.tema, c.uff, g, c.want)
		}
	}
	if Variante("ufficio") != Ufficio || Variante("gioco") != Gaming || Variante("entrambi") != Gaming {
		t.Fatal("variante")
	}
	if PosizioneIcona(IDNormale) != 0 || PosizioneIcona(0) != 0 || PosizioneIcona(201) != -201 {
		t.Fatal("posizione")
	}
}

func TestPreparaSVG(t *testing.T) {
	in := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256"><title>TASSO GAMING</title><defs><linearGradient id="met"/><linearGradient id="metR"/></defs>` +
		`<rect fill="url(#met)"/><rect stroke="url(#metR)"/><ellipse cx="74.00" cy="98" rx="9"/><ellipse cx="126.00" cy="98" rx="9"/><ellipse cx="10" cy="98"/></svg>`
	out := PreparaSVG(in, "p-")
	for _, w := range []string{`id="p-met"`, `id="p-metR"`, `url(#p-met)`, `url(#p-metR)`, `<ellipse class="pup" cx="74.00"`, `<ellipse class="pup" cx="126.00"`, `<ellipse cx="10"`, `aria-hidden="true"`} {
		if !strings.Contains(out, w) {
			t.Errorf("manca %s in %s", w, out)
		}
	}
	if strings.Contains(out, "<title>") || strings.Contains(out, `url(#met)`) || strings.Count(out, "p-p-") > 0 {
		t.Errorf("sbagliato: %s", out)
	}
}

// I temi di ogni modalità sono gli stessi dell'interfaccia (OFFICE_THEMES e GAME_THEMES in
// ui/index.html) e ogni tema che si sceglie, tranne "tasso", ha le sue due icone nell'elenco.
func TestTemiComeInterfaccia(t *testing.T) {
	ui, err := os.ReadFile("../ui/index.html")
	if err != nil {
		t.Skip("ui/index.html non trovato:", err)
	}
	lista := func(nome string) []string {
		m := regexp.MustCompile(`const ` + nome + ` = \[([^\]]*)\]`).FindSubmatch(ui)
		if m == nil {
			t.Fatalf("%s non trovato in ui/index.html", nome)
		}
		return regexp.MustCompile(`'([a-z0-9]+)'`).FindAllString(string(m[1]), -1)
	}
	for _, c := range []struct {
		nome string
		lst  []string
	}{{"OFFICE_THEMES", TemiUfficio}, {"GAME_THEMES", TemiGaming}} {
		js := lista(c.nome)
		var g []string
		for _, x := range c.lst {
			g = append(g, "'"+x+"'")
		}
		if strings.Join(js, ",") != strings.Join(g, ",") {
			t.Errorf("%s: interfaccia %v, icotema %v", c.nome, js, g)
		}
	}
	b, err := os.ReadFile("../assets/temi/elenco.txt")
	if err != nil {
		t.Fatal(err)
	}
	e, err := Leggi(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, tema := range append(append([]string{}, TemiUfficio...), TemiGaming...) {
		if tema == "tasso" {
			continue
		}
		for _, v := range []string{Gaming, Ufficio} {
			if e.Trova(tema, v) == 0 {
				t.Errorf("manca l'icona %s %s in elenco.txt", tema, v)
			}
		}
	}
}
