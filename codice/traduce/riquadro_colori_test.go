package traduce

import "testing"

// Ogni scritta del riquadro si legge con contrasto almeno 4,5:1, qualunque cosa ci sia sotto
// (nero, bianco, grigio, colori dei giochi), e l'inglese del gioco sotto non traspare.
func TestContrastoRiquadro(t *testing.T) {
	sotto := []uint32{0x000000, 0xFFFFFF, 0x808080, 0xB0B0B0, 0x0000FF, 0x00FF00, 0xFF0000, 0x40C0F0}
	scritte := map[string]uint32{"traduzione": colTestoRiquadro, "inglese": colIngleseRiquadro, "stato": colStatoRiquadro}
	for _, s := range sotto {
		fondo := fondoSopra(colFondoRiquadro, s, opacitaRiquadro)
		for nome, c := range scritte {
			if k := contrastoWCAG(c, fondo); k < 4.5 {
				t.Errorf("%s su fondo %06x (sotto %06x): contrasto %.2f < 4,5", nome, fondo, s, k)
			}
		}
		// il testo del gioco sotto (stesso colore della traduzione, il caso peggiore) non si vede
		if k := contrastoWCAG(fondoSopra(colFondoRiquadro, colTestoRiquadro, opacitaRiquadro), fondo); k > 1.05 {
			t.Errorf("l'inglese sotto traspare (contrasto %.2f)", k)
		}
	}
	if k := contrastoWCAG(colTestoRiquadro, colTastoChiudi); k < 4.5 {
		t.Errorf("X: contrasto %.2f", k)
	}
	if k := contrastoWCAG(colTestoRiquadro, colTastoChiudiSopra); k < 4.5 {
		t.Errorf("X col mouse sopra: contrasto %.2f", k)
	}
}
