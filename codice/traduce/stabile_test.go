package traduce

import (
	"image/color"
	"testing"
	"time"
)

// La stessa scritta (stessi pixel) letta in modi diversi a ogni rilettura: vince la lettura vista
// più volte o quella del catalogo, e non torna indietro per una lettura sbagliata isolata.
func TestVotazioneStabile(t *testing.T) {
	sf, ink := color.RGBA{20, 20, 30, 255}, color.RGBA{240, 240, 240, 255}
	img := immagineFinta(600, 200, sf)
	fine := parolaFinta(img, 50, 50, 12, 12, 24, ink)
	b := Blocco{X: 50, Y: 50, W: fine - 50, H: 24}
	v := &votazione{}
	t0 := time.Now()
	giusta := "Carry that thing to fill the progress bar"
	seq := []struct {
		letto string
		affid int
	}{
		{giusta, letturaCatalog},
		{"Curry thing thing to fill the progress bur", letturaDubbia},
		{"Curry thing to fill the progress bar", letturaBuona},
		{"Curry thing to fill the progress bar", letturaBuona},
		{"", letturaDubbia},
	}
	for i, s := range seq {
		got, a := v.scegli(img, b, s.letto, s.affid, t0.Add(time.Duration(i)*time.Second))
		if got != giusta || a != letturaCatalog {
			t.Fatalf("passo %d (%q): scelta %q %d, attesa la frase pronta", i, s.letto, got, a)
		}
	}
	// a pari voti resta quella già mostrata
	v2 := &votazione{}
	v2.scegli(img, b, "Open the door", letturaBuona, t0)
	if got, _ := v2.scegli(img, b, "Open the doer", letturaBuona, t0); got != "Open the door" {
		t.Fatalf("isteresi: %q", got)
	}
	// i pixel cambiano (scritta nuova nello stesso posto): si ricomincia, vince la nuova
	img2 := immagineFinta(600, 200, sf)
	parolaFinta(img2, 50, 50, 5, 30, 24, ink)
	if got, _ := v.scegli(img2, b, "Press E to open", letturaCatalog, t0.Add(10*time.Second)); got != "Press E to open" {
		t.Fatalf("scritta cambiata ma tenuta la vecchia: %q", got)
	}
	// posto nuovo, lettura dubbia e nient'altro: resta dubbia
	b3 := Blocco{X: 300, Y: 150, W: 100, H: 20}
	if _, a := v.scegli(img, b3, "FELS", letturaDubbia, t0); a != letturaDubbia {
		t.Fatalf("una lettura dubbia non diventa buona da sola")
	}
}

// Scritta tradotta che la nuova lettura ha saltato: resta se i pixel sono gli stessi, va via se
// la scritta è cambiata.
func TestRecuperaPerse(t *testing.T) {
	sf, ink := color.RGBA{20, 20, 30, 255}, color.RGBA{240, 240, 240, 255}
	img := immagineFinta(600, 200, sf)
	fine := parolaFinta(img, 50, 50, 8, 12, 24, ink)
	vecchia := Blocco{X: 50, Y: 50, W: fine - 50, H: 24, Testo: "Settings", Tradotto: "Impostazioni", Img: img}
	img2 := immagineFinta(600, 200, sf)
	parolaFinta(img2, 50, 50, 8, 12, 24, ink)
	if out := recuperaPerse(nil, []Blocco{vecchia}, img2); len(out) != 1 || out[0].Tradotto != "Impostazioni" {
		t.Fatalf("scritta uguale persa: %+v", out)
	}
	img3 := immagineFinta(600, 200, sf) // la scritta è sparita
	if out := recuperaPerse(nil, []Blocco{vecchia}, img3); len(out) != 0 {
		t.Fatalf("scritta sparita ma traduzione rimasta")
	}
	// c'è già una scritta nuova in quel posto: vince la nuova
	nuova := Blocco{X: 52, Y: 50, W: 80, H: 24, Testo: "Options", Tradotto: "Opzioni"}
	if out := recuperaPerse([]Blocco{nuova}, []Blocco{vecchia}, img2); len(out) != 1 || out[0].Tradotto != "Opzioni" {
		t.Fatalf("doppione: %+v", out)
	}
}
