package traduce

import (
	"context"
	"strings"
	"testing"
	"time"
)

// motore finto: traduce parola per parola come Bergamot sulla libreria di Tasso (28/11)
type motoreNomi struct{}

func (motoreNomi) Nome() string { return "finto" }
func (motoreNomi) Traduci(_ context.Context, r RichiestaTraduzione) (string, error) {
	sost := strings.NewReplacer("Steam", "Vapore", "Hytale Launcher", "Lanciatore Hytale", "Launcher", "Lanciatore",
		"Release date", "Data di uscita", "Recently played", "Giocati di recente", "Talk to", "Parla con", "in the tavern", "nella taverna")
	return sost.Replace(r.Testo), nil
}

func conLibreriaFinta(t *testing.T) {
	prima := Ospite.VociLibreria
	Ospite.VociLibreria = func() []VoceLibreria {
		return []VoceLibreria{{Nome: "Hytale Launcher", Exe: `C:\Program Files\Hypixel Studios\Hytale Launcher\hytale-launcher.exe`},
			{Nome: "Subnautica 2"}, {Nome: "PC Cafe Simulator"}, {Nome: "CRACK"}, {Nome: "The Forest"}}
	}
	dimenticaNomiProtetti()
	t.Cleanup(func() { Ospite.VociLibreria = prima; dimenticaNomiProtetti() })
}

func TestDividiProtetti(t *testing.T) {
	conLibreriaFinta(t)
	casi := map[string]string{
		"Steam · 15,3 GB":                "[Steam] · [15,3 GB]",
		"Hytale Launcher":                "[Hytale Launcher]",
		"Release date — Steam 9.2 GB":    "Release date — [Steam] [9.2 GB]",
		"Lando94 joined":                 "[Lando94] joined",
		"1LMess1a":                       "[1LMess1a]",
		"Let off steam":                  "Let off steam", // parola normale, non il negozio
		"Crack the safe":                 "Crack the safe",
		"Go to the forest":               "Go to the forest",
		"The Forest — 12 GB":             "[The Forest] — [12 GB]",
		"hytale launcher":                "[hytale launcher]",
		"CRACK":                          "[CRACK]",
		"Press F7 to pause":              "Press F7 to pause",
		"144 Hz monitor":                 "[144 Hz] monitor",
		"Subnautica 2 — Recently played": "[Subnautica 2] — Recently played",
	}
	for in, atteso := range casi {
		var b strings.Builder
		for _, p := range dividiProtetti(in, nomiProtetti()) {
			if p.protetto {
				b.WriteString("[" + p.testo + "]")
			} else {
				b.WriteString(p.testo)
			}
		}
		if b.String() != atteso {
			t.Errorf("%q -> %q, atteso %q", in, b.String(), atteso)
		}
	}
}

func TestNomiNonTradotti(t *testing.T) {
	conLibreriaFinta(t)
	tr := &Traduttore{motori: []Motore{motoreNomi{}}, cache: map[string]string{}}
	for in, atteso := range map[string]string{
		"Steam":                        "Steam",
		"Hytale Launcher":              "Hytale Launcher",
		"Release date — Steam 15.3 GB": "Data di uscita — Steam 15.3 GB",
		"Talk to Hytale Launcher in the tavern please now": "Parla con Hytale Launcher nella taverna please now",
	} {
		r := tr.traduciIn(context.Background(), in, false, "en", "it")
		if r.Tradotto != atteso {
			t.Errorf("%q -> %q (%s), atteso %q", in, r.Tradotto, r.Motore, atteso)
		}
	}
	if r := tr.traduciIn(context.Background(), "Steam", false, "en", "it"); traduzioneDaMostrare("Steam", r.Tradotto) {
		t.Error("un nome rimasto uguale non va mostrato come tradotto")
	}
}

func TestRimettiNomi(t *testing.T) {
	got := rimettiNomiProtetti("Buy it on Steam today, it is a really good deal for everyone",
		"Compralo su Vapore oggi, è un vero affare per tutti", []string{"Steam"}, func(string) string { return "Vapore" })
	if got != "Compralo su Steam oggi, è un vero affare per tutti" {
		t.Error(got)
	}
	_ = time.Now
}
