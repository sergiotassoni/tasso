package main

import (
	"testing"
	"time"
)

func TestProssimoControlloAgg(t *testing.T) {
	casi := []struct {
		spento, noAuto, gioco, apps, occupato bool
		controlla                             bool
		dopo                                  time.Duration
	}{
		{controlla: true},
		{noAuto: true},
		{spento: true},
		{gioco: true, dopo: controlloAggRiprova},
		{apps: true, dopo: controlloAggRiprova},
		{occupato: true, dopo: controlloAggRiprova},
		{noAuto: true, gioco: true}, // spento vince: non riprovo nemmeno
	}
	for i, c := range casi {
		ok, d := prossimoControlloAgg(c.spento, c.noAuto, c.gioco, c.apps, c.occupato)
		if ok != c.controlla || d != c.dopo {
			t.Errorf("caso %d: %v %v, voglio %v %v", i, ok, d, c.controlla, c.dopo)
		}
	}
	if controlloAggOgni < 12*time.Hour || controlloAggOgni > 24*time.Hour {
		t.Fatalf("ogni %v: deve essere tra 12 e 24 ore", controlloAggOgni)
	}
}
