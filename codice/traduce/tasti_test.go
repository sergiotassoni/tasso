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
