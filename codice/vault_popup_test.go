package main

import (
	"math"
	"testing"
)

// La finestrella va sotto il campo, allineata a sinistra; sopra se sotto c'è un'altra tendina o
// se non ci sta; sempre dentro lo schermo.
func TestVtPosizione(t *testing.T) {
	schermo := rect{0, 0, 1920, 1040}
	campo := rect{700, 400, 1100, 450}
	if x, y := vtPosizione(campo, schermo, 368, 260, 6, false); x != 700 || y != 456 {
		t.Errorf("sotto: %d,%d", x, y)
	}
	if x, y := vtPosizione(campo, schermo, 368, 260, 6, true); x != 700 || y != 400-260-6 {
		t.Errorf("sopra (altra tendina sotto): %d,%d", x, y)
	}
	basso := rect{700, 900, 1100, 950}
	if _, y := vtPosizione(basso, schermo, 368, 260, 6, false); y != 900-260-6 {
		t.Errorf("non ci sta sotto: deve andare sopra, y=%d", y)
	}
	alto := rect{700, 40, 1100, 90}
	if _, y := vtPosizione(alto, schermo, 368, 260, 6, true); y != 96 {
		t.Errorf("sopra non ci sta: resta sotto, y=%d", y)
	}
	destra := rect{1800, 400, 1900, 450}
	if x, _ := vtPosizione(destra, schermo, 368, 260, 6, false); x+368 > 1920 {
		t.Errorf("esce a destra: x=%d", x)
	}
	sinistra := rect{-50, 400, 100, 450}
	if x, _ := vtPosizione(sinistra, schermo, 368, 260, 6, false); x < 0 {
		t.Errorf("esce a sinistra: x=%d", x)
	}
}

func lum(c uint32) float64 {
	f := func(v float64) float64 {
		v /= 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	r, g, b := float64(c&0xFF), float64((c>>8)&0xFF), float64((c>>16)&0xFF)
	return 0.2126*f(r) + 0.7152*f(g) + 0.0722*f(b)
}

// sopra: colore della finestrella mescolato con lo sfondo dietro (trasparenza vtAlpha).
func sopra(c, dietro uint32) uint32 {
	a := float64(vtAlpha) / 255
	var out uint32
	for s := 0; s < 24; s += 8 {
		v := a*float64((c>>s)&0xFF) + (1-a)*float64((dietro>>s)&0xFF)
		out |= uint32(v+0.5) << s
	}
	return out
}

func contrasto(a, b uint32) float64 {
	x, y := lum(a), lum(b)
	if y > x {
		x, y = y, x
	}
	return (x + 0.05) / (y + 0.05)
}

// Testo e grigi della finestrella restano sopra 4,5:1 anche con una pagina bianca, nera o grigia dietro.
func TestVtContrasto(t *testing.T) {
	for _, dietro := range []uint32{0xFFFFFF, 0x000000, 0x808080, 0xF0F0F0} {
		fondo := sopra(vcBg, dietro)
		for nome, c := range map[string]uint32{"testo": vcText, "grigio": vcMuted, "sotto": vcSotto, "link": vcLink, "verde": vcA1Chiaro} {
			if k := contrasto(sopra(c, dietro), fondo); k < 4.5 {
				t.Errorf("%s su sfondo %06X: contrasto %.2f < 4.5", nome, dietro, k)
			}
		}
		// pulsante verde pieno: testo scuro
		if k := contrasto(sopra(vcInk, dietro), sopra(vcA1, dietro)); k < 4.5 {
			t.Errorf("pulsante verde su %06X: %.2f", dietro, k)
		}
		if k := contrasto(sopra(vcText, dietro), sopra(vcCampo, dietro)); k < 4.5 {
			t.Errorf("testo nei campi su %06X: %.2f", dietro, k)
		}
	}
}

func TestSvControllaEUguali(t *testing.T) {
	if svControlla("", 10) == "" || svControlla("Lumio", 0) == "" || svControlla("Lumio", 12) != "" {
		t.Error("svControlla sbagliato")
	}
	a := []uint16{'a', 'b', 0}
	if !svUguali(a, []uint16{'a', 'b', 0}) || svUguali(a, []uint16{'a', 'c', 0}) || svUguali(a, nil) || svUguali(nil, nil) {
		t.Error("svUguali sbagliato")
	}
}

func TestVtForzaEItoa(t *testing.T) {
	if itoa(0) != "0" || itoa(20) != "20" || itoa(1234) != "1234" {
		t.Error("itoa")
	}
	if vtLungGen(vtPopDati{}) != 20 || vtLungGen(vtPopDati{corta: true}) != 14 {
		t.Error("lunghezze della password generata")
	}
}

// La password generata nella finestrella si azzera quando la finestrella si chiude.
func TestVtNascondiAzzeraPassword(t *testing.T) {
	gen := []uint16{'X', 'y', '7', 0}
	vtPopMu.Lock()
	vtPop = vtPopDati{modo: vpGenera, gen: gen}
	vtPopMu.Unlock()
	vtNascondi()
	for _, c := range gen {
		if c != 0 {
			t.Fatalf("password non azzerata: %v", gen)
		}
	}
}
