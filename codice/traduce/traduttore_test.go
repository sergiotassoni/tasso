package traduce

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

type motoreFinto struct {
	nome  string
	err   error
	ultim RichiestaTraduzione
	n     int
}

func (m *motoreFinto) Nome() string { return m.nome }
func (m *motoreFinto) Traduci(_ context.Context, r RichiestaTraduzione) (string, error) {
	m.n++
	m.ultim = r
	if m.err != nil {
		return "", m.err
	}
	return "IT:" + r.Testo, nil
}

func TestRiservaCacheContesto(t *testing.T) {
	dir := t.TempDir()
	ia := &motoreFinto{nome: "ia", err: errors.New("scheda occupata")}
	moz := &motoreFinto{nome: "mozilla"}
	tr := NuovoTraduttore(filepath.Join(dir, "cache.json"), glossarioBase(), ia, moz)

	r := tr.Traduci(context.Background(), "Old Keeper: Your HP is\nlow.")
	if r.Motore != "mozilla" || r.Tradotto != "IT:Your PV is low." || r.Parlante != "Old Keeper" {
		t.Fatalf("risultato inatteso: %+v", r)
	}
	if len(ia.ultim.Glossario) != 1 || ia.ultim.Glossario[0].Italiano != "PV" {
		t.Fatalf("glossario non passato: %+v", ia.ultim.Glossario)
	}
	r2 := tr.Traduci(context.Background(), "Move, now!")
	if ia.ultim.Contesto[0] != "Your HP is low." || r2.Motore != "mozilla" {
		t.Fatalf("contesto non passato: %+v", ia.ultim.Contesto)
	}
	// stessa frase: dalla memoria, senza chiamare i motori
	prima := moz.n
	if r3 := tr.Traduci(context.Background(), "Move, now!"); r3.Motore != "memoria" || moz.n != prima {
		t.Fatalf("cache non usata: %+v", r3)
	}
	// la cache resta dopo la riapertura
	tr2 := NuovoTraduttore(filepath.Join(dir, "cache.json"), glossarioBase())
	if r4 := tr2.Traduci(context.Background(), "Move, now!"); r4.Tradotto != "IT:Move, now!" {
		t.Fatalf("cache su disco non letta: %+v", r4)
	}
}

func TestTuttiFalliscono(t *testing.T) {
	tr := NuovoTraduttore("", glossarioBase(), &motoreFinto{nome: "ia", err: errors.New("spento")})
	if r := tr.Traduci(context.Background(), "Hello there."); r.Errore == "" || r.Tradotto != "" {
		t.Fatalf("doveva segnalare l'errore: %+v", r)
	}
}

func TestClientIA(t *testing.T) {
	var ricevuto map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&ricevuto)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"<think>\n</think>\n\"Ricarico, coprimi!\""}}]}`))
	}))
	defer srv.Close()
	c := &ClientIA{URL: srv.URL}
	out, err := c.Traduci(context.Background(), RichiestaTraduzione{Testo: "Reloading, cover me!", Contesto: []string{"Enemy on the roof!"}})
	if err != nil || out != "Ricarico, coprimi!" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	msgs := ricevuto["messages"].([]interface{})
	user := msgs[1].(map[string]interface{})["content"].(string)
	if !strings.Contains(user, "Enemy on the roof!") || !strings.Contains(user, "Reloading, cover me!") {
		t.Fatalf("prompt senza contesto: %s", user)
	}
}

func TestQuantitaInFondo(t *testing.T) {
	for _, c := range []struct{ in, nome, q string }{
		{"Health Potion x3", "Health Potion", " x3"},
		{"Arrows (20)", "Arrows", " (20)"},
		{"Gold 150", "Gold", " 150"},
		{"Lockpick x5", "Lockpick", " x5"},
	} {
		m := reQuantita.FindStringSubmatchIndex(c.in)
		if m == nil || c.in[:m[2]] != c.nome || c.in[m[2]:] != c.q {
			t.Errorf("%q: %v", c.in, m)
		}
	}
	if reQuantita.MatchString("Level 3 complete") {
		t.Error("non è una quantità in fondo")
	}
	if !daTradurre("Lockpick x5") {
		t.Error("Lockpick è nel glossario: va tradotto")
	}
}

func TestAumentoStatistica(t *testing.T) {
	m := reAumento.FindStringSubmatch("Health Regen Increased 6.7%")
	if m == nil || m[1] != "Health Regen" || m[3] != "6.7" {
		t.Fatalf("%v", m)
	}
	if reAumento.MatchString("Growth Rate is Increased") {
		t.Error("senza numero non è una statistica")
	}
}

func TestOggettoDiMateriale(t *testing.T) {
	tr := &Traduttore{}
	tr.glossarioPieno = Glossario{}.conGiochi()
	for in, atteso := range map[string]string{
		"Iron Helmet": "Elmo di ferro", "Steel Sword": "Spada d'acciaio", "Leather Boots": "Stivali di cuoio",
		"Silver Ring": "Anello d'argento", "Wooden Shield": "Scudo di legno",
	} {
		if got, ok := tr.oggettoDiMateriale(in); !ok || got != atteso {
			t.Errorf("%q -> %q (%v), atteso %q", in, got, ok, atteso)
		}
	}
	if _, ok := tr.oggettoDiMateriale("Iron Will"); ok {
		t.Error("Will non è un oggetto")
	}
}

func TestLibreriaGioco(t *testing.T) {
	d := t.TempDir()
	f := fileLibreriaPer(d, "Path of Titans: Edizione 2")
	if filepath.Base(f) != "path-of-titans-edizione-2.json" {
		t.Fatalf("nome file: %s", f)
	}
	tr := NuovoTraduttore("", Glossario{}, &motoreFinto{nome: "Mozilla"})
	tr.ApriLibreria(f)
	r := tr.TraduciScritta(context.Background(), "The bridge is broken.")
	tr.SalvaMemoria()
	if ContaLibreria(f) != 1 {
		t.Fatalf("libreria: %d", ContaLibreria(f))
	}
	tr2 := NuovoTraduttore("", Glossario{}, &motoreFinto{nome: "Mozilla"})
	tr2.ApriLibreria(f)
	r2 := tr2.TraduciScritta(context.Background(), "The bridge is broken.")
	if r2.Motore != "libreria" || r2.Tradotto != r.Tradotto {
		t.Fatalf("dalla libreria: %+v", r2)
	}
}

func TestSembraStrana(t *testing.T) {
	casi := map[[2]string]bool{
		{"The bridge is broken.", "Il ponte è rotto."}:     false,
		{"The bridge is broken.", "The bridge is broken."}: true,
		{"Go to the old mill and find the key.", "Vai."}:   true,
		{"Find the key", "Trova the key"}:                  true,
		{"Health Potion x3", "Pozione di salute x3"}:       false,
	}
	for c, strana := range casi {
		if (sembraStrana(c[0], c[1]) != "") != strana {
			t.Errorf("%q -> %q: %q", c[0], c[1], sembraStrana(c[0], c[1]))
		}
	}
}
