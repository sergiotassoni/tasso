package traduce

import "testing"

func TestTasti(t *testing.T) {
	if etichettaTasto(0x75) != "F6" || etichettaTasto(2<<16|'T') != "Ctrl+T" || etichettaTasto(3<<16|0x71) != "Ctrl+Alt+F2" {
		t.Error("etichette")
	}
	b := tastiCompleti(nil)
	if controllaTasto("pagina", 'W', b) == nil {
		t.Error("W da solo non va bene")
	}
	if controllaTasto("pagina", 2<<16|'W', b) != nil {
		t.Error("Ctrl+W va bene")
	}
	if controllaTasto("pagina", 0x78, b) == nil {
		t.Error("F9 lo usa già la zona")
	}
	if controllaTasto("pagina", 0x7B, b) != nil {
		t.Error("F12 va bene")
	}
}

func TestTastiNuovi(t *testing.T) {
	if e := etichettaTasto(tastiDiBase["chat"]); e != "Ctrl+Alt+T" {
		t.Errorf("chat: %s", e)
	}
	if e := etichettaTasto(tastiDiBase["voci"]); e != "Ctrl+Alt+V" {
		t.Errorf("voci: %s", e)
	}
}

func TestTastiVietati(t *testing.T) {
	b := tastiCompleti(nil)
	no := map[string]uint32{
		"Ctrl+Alt+Canc": 3<<16 | 0x2E, "Win+L": 8<<16 | 'L', "Alt+Tab": 1<<16 | 0x09, "Alt+F4": 1<<16 | 0x73,
		"Esc": 0x1B, "A": 'A', "Maiusc+A": 4<<16 | 'A', "5": '5', "Ctrl da solo": 2<<16 | 0x11, "AltGr+E": 3<<16 | 'E',
	}
	for n, c := range no {
		if controllaTasto("pagina", c, b) == nil {
			t.Errorf("%s non deve andare bene", n)
		}
	}
	si := map[string]uint32{"F1": 0x70, "F24": 0x87, "Win+K": 8<<16 | 'K', "Ctrl+Maiusc+F5": 6<<16 | 0x74, "Alt+1": 1<<16 | '1', "Maiusc+F2": 4<<16 | 0x71}
	for n, c := range si {
		if err := controllaTasto("pagina", c, b); err != nil {
			t.Errorf("%s deve andare bene: %v", n, err)
		}
	}
	if e := etichettaTasto(8<<16 | 'K'); e != "Win+K" {
		t.Errorf("etichetta Win: %s", e)
	}
	AltriTasti = func() map[uint32]string { return map[uint32]string{0x75: "Autoclick"} }
	defer func() { AltriTasti = nil }()
	if err := controllaTasto("pagina", 0x75, b); err == nil {
		t.Error("F6 lo usa l'autoclick")
	}
}
