package traduce

import (
	"context"
	"testing"
	"time"
)

type correttoreFinto struct{ risposta string }

func (c correttoreFinto) Nome() string { return "IA" }
func (c correttoreFinto) Traduci(ctx context.Context, r RichiestaTraduzione) (string, error) {
	if r.Bozza == "" {
		panic("il correttore deve ricevere la bozza")
	}
	return c.risposta, nil
}

type mozFinto struct{}

func (mozFinto) Nome() string { return "Mozilla" }
func (mozFinto) Traduci(ctx context.Context, r RichiestaTraduzione) (string, error) {
	return "E' stanco. Sotto di lui prima che si infortuna.", nil
}

func TestCorrezioneInSottofondo(t *testing.T) {
	tr := NuovoTraduttore("", Glossario{}, mozFinto{})
	tr.libreria = map[string]string{}
	tr.ImpostaCorrettore(correttoreFinto{"È stanco. Sostituiscilo prima che si infortuni."}, nil)
	en := "He's tired. Sub him before he gets injured."
	r := tr.Traduci(context.Background(), en)
	if r.Motore != "Mozilla" {
		t.Fatalf("prima volta deve rispondere subito Mozilla: %+v", r)
	}
	for i := 0; i < 100; i++ {
		if f, _, _ := tr.StatoCorrezioni(); f > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	r2 := tr.Traduci(context.Background(), en)
	if r2.Tradotto != "È stanco. Sostituiscilo prima che si infortuni." {
		t.Fatalf("la seconda volta deve uscire la versione corretta: %+v", r2)
	}
}

func TestCorrezioneInventataScartata(t *testing.T) {
	tr := NuovoTraduttore("", Glossario{}, mozFinto{})
	tr.ImpostaCorrettore(correttoreFinto{"Sotto di lui. Poi il drago vola sopra la montagna e tutti scappano verso il mare lontano."}, nil)
	en := "He's tired. Sub him before he gets injured."
	tr.Traduci(context.Background(), en)
	for i := 0; i < 100; i++ {
		if f, _, _ := tr.StatoCorrezioni(); f > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if r := tr.Traduci(context.Background(), en); r.Tradotto != "È stanco. Sotto di lui prima che si infortuna." {
		t.Fatalf("invenzione accettata: %+v", r)
	}
}
