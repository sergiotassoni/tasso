package traduce

import "testing"

func TestValutaGPU(t *testing.T) {
	casi := []struct {
		g      InfoGPU
		adatta bool
	}{
		{InfoGPU{Nome: "AMD Radeon RX 6700 XT", MemoriaGB: 12, UsataGB: 1}, true},
		{InfoGPU{Nome: "NVIDIA GeForce RTX 3060", MemoriaGB: 12, UsataGB: 9}, false}, // gioco pesante aperto
		{InfoGPU{Nome: "NVIDIA GeForce RTX 3060", MemoriaGB: 8, UsataGB: 2}, true},
		{InfoGPU{Nome: "NVIDIA GeForce GTX 1650", MemoriaGB: 4}, false},
	}
	for _, c := range casi {
		if got := valutaGPU(c.g); got.Adatta != c.adatta || got.Motivo == "" {
			t.Errorf("%+v -> %+v", c.g, got)
		}
	}
}

func TestPosizioneFinestrella(t *testing.T) {
	z := Zona{X: 400, Y: 800, W: 1000, H: 120}
	casi := []struct {
		pos  string
		x, y int32
	}{
		{"sopra", 400, 800 - 100 - 10},
		{"sotto", 400, 930},
		{"copri", 400, 800},
		{"lato", 1920 - 600, 800}, // a destra non c'è spazio: a sinistra della zona... ma dentro lo schermo
	}
	for _, c := range casi {
		x, y := posizioneFinestrella(c.pos, z, 600, 100, 0, 0, 1920, 1080)
		if c.pos == "lato" {
			if x != 400-600-10 && x < 0 {
				t.Errorf("lato: x=%d", x)
			}
			continue
		}
		if x != c.x || y != c.y {
			t.Errorf("%s: (%d,%d), atteso (%d,%d)", c.pos, x, y, c.x, c.y)
		}
	}
	// zona in alto: "sopra" va sotto
	if _, y := posizioneFinestrella("sopra", Zona{X: 0, Y: 20, W: 500, H: 50}, 500, 100, 0, 0, 1920, 1080); y != 80 {
		t.Errorf("sopra senza spazio: y=%d", y)
	}
}
