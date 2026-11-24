package traduce

import "testing"

// super controllo 24/11: le correzioni della lettura non devono rovinare l'inglese giusto
func TestCorrezioniNonRovinano(t *testing.T) {
	for _, s := range []string{"You can't go", "I won't", "Equip the pickaxe", "to unequip items",
		"Set a waypoint", "Health regen", "Skyrim", "Tarkov", "noob", "rekt", "poggers", "smurf",
		"StarCraft", "GameStop", "FromSoftware", "Ron, come here!", "Ack", "in Lobby", "in a Game", "in Party"} {
		if g := correggiParole(s); g != s {
			t.Errorf("%q -> %q", s, g)
		}
	}
	if g := correggiParole("Oin Multiplayer"); g != "Join Multiplayer" {
		t.Errorf("Oin: %q", g)
	}
}

func TestCorrezioniCheServono(t *testing.T) {
	for in, want := range map[string]string{"Settigns": "Settings", "Contiune": "Continue", "Remaning Time": "Remaining Time",
		"Conecting to server": "Connecting to server", "recieve the item": "receive the item", "LoadGame": "Load Game",
		"JoinServer": "Join Server", "MultiplayerGame": "Multiplayer Game", "Tems": "Items"} {
		if g := correggiParole(in); g != want {
			t.Errorf("%q -> %q, atteso %q", in, g, want)
		}
	}
	for _, s := range []string{"despawned", "smurfing", "Kratos", "Tarkov", "GameStop", "DarkSouls"} {
		if g := correggiParole(s); g != s {
			t.Errorf("%q -> %q", s, g)
		}
	}
}
