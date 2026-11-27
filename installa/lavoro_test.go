package installa

import (
	"testing"
	"time"
)

func TestDecidiCartellaLavoro(t *testing.T) {
	casi := []struct {
		esiste, cartella, coll, admin bool
		want                          AzioneLavoro
	}{
		{false, false, false, false, LavoroCrea},
		{true, true, false, true, LavoroUsa},
		{true, true, false, false, LavoroSpostaVia}, // creata da un utente
		{true, true, true, true, LavoroSpostaVia},   // giunzione
		{true, false, false, true, LavoroSpostaVia}, // un file
	}
	for _, c := range casi {
		if got := DecidiCartellaLavoro(c.esiste, c.cartella, c.coll, c.admin); got != c.want {
			t.Errorf("%+v: %v", c, got)
		}
	}
}

func TestDatiWebView2Vecchi(t *testing.T) {
	ora := time.Now()
	if !DatiWebView2Vecchi("WebView2-123", ora.Add(-2*time.Hour), ora) {
		t.Fatal("vecchia non tolta")
	}
	if DatiWebView2Vecchi("WebView2-123", ora.Add(-time.Minute), ora) {
		t.Fatal("nuova tolta")
	}
	if DatiWebView2Vecchi("TassoSetup.log", ora.Add(-48*time.Hour), ora) {
		t.Fatal("diario tolto")
	}
}
