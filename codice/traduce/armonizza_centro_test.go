package traduce

import "testing"

func TestArmonizzaCentrati(t *testing.T) {
	// menu centrato: centri uguali (x+w/2 = 500), inizi diversi, una voce letta più grande
	b := []Blocco{
		{X: 440, Y: 500, W: 120, H: 20, Em: 18},
		{X: 380, Y: 540, W: 240, H: 22, Em: 23},
		{X: 460, Y: 580, W: 80, H: 20, Em: 18},
	}
	armonizza(b)
	for i := range b {
		if !b[i].Centro {
			t.Fatalf("voce %d non centrata", i)
		}
	}
	if b[1].Em != 18 {
		t.Fatalf("grandezza non uniformata: %v", b[1].Em)
	}
	// elenco allineato a sinistra: non è centrato
	s := []Blocco{{X: 100, Y: 100, W: 120, H: 20, Em: 18}, {X: 100, Y: 130, W: 60, H: 20, Em: 18}}
	armonizza(s)
	if s[0].Centro || s[1].Centro {
		t.Fatal("elenco a sinistra preso per centrato")
	}
	if l, r := margineFondo(Blocco{W: 100, Centro: true}); l != r || l <= 3 {
		t.Fatalf("margine centrato %d %d", l, r)
	}
}
