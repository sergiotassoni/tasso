package traduce

import (
	"context"
	"math"
	"strings"
	"testing"
)

func TestChrFUgualeASacrebleu(t *testing.T) {
	// valore calcolato con sacrebleu 2.x (corpus_chrf, word_order=2) sugli stessi dati: 64.50
	es, err := esitoMozillaRegistrato()
	if err != nil {
		t.Fatal(err)
	}
	if len(es.Frasi) != 1000 {
		t.Fatalf("frasi: %d", len(es.Frasi))
	}
	var hyp, ref []string
	for _, f := range es.Frasi[:100] {
		hyp, ref = append(hyp, f.Uscita), append(ref, f.IT)
	}
	if c := chrFpp(hyp, ref); math.Abs(c-64.50) > 0.2 {
		t.Fatalf("chrF++ prime 100 = %.2f, sacrebleu dice 64.50", c)
	}
	if chrFpp([]string{"Ciao a tutti."}, []string{"Ciao a tutti."}) < 99.9 {
		t.Fatal("frasi identiche devono valere 100")
	}
}

func TestEseguiProva(t *testing.T) {
	frasi, err := caricaFrasiProva()
	if err != nil || len(frasi) != 1000 {
		t.Fatalf("frasi: %d %v", len(frasi), err)
	}
	m := &motoreFinto{nome: "finto"}
	es := eseguiProva(context.Background(), m, frasi[:5])
	if len(es.Frasi) != 5 || m.n != 5 || !strings.HasPrefix(es.Frasi[0].Uscita, "IT:") {
		t.Fatalf("esito: %+v", es)
	}
	if len(m.ultim.Contesto) == 0 {
		t.Fatal("il contesto delle frasi precedenti non è stato passato")
	}
	if r := rapportoProva([]EsitoProva{es}); !strings.Contains(r, "| finto |") {
		t.Fatal("rapporto senza il motore")
	}
}
