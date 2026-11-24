package traduce

import "testing"

func TestDecidiGioco(t *testing.T) {
	casi := []struct {
		nome string
		f    infoFinestra
		si   bool
	}{
		{"Chrome con YouTube a schermo intero", infoFinestra{Exe: "chrome.exe", SchermoIntero: true}, false},
		{"Edge in finestra", infoFinestra{Exe: "msedge.exe"}, false},
		{"Firefox a schermo intero", infoFinestra{Exe: "firefox.exe", SchermoIntero: true}, false},
		{"desktop (Esplora risorse)", infoFinestra{Exe: "explorer.exe", SchermoIntero: true}, false},
		{"Tasso", infoFinestra{Exe: "tasso.exe", Nostra: true, SchermoIntero: true}, false},
		{"Discord", infoFinestra{Exe: "discord.exe"}, false},
		{"Word a schermo intero", infoFinestra{Exe: "winword.exe", SchermoIntero: true}, false},
		{"VLC a schermo intero", infoFinestra{Exe: "vlc.exe", SchermoIntero: true}, false},
		{"programma di Windows", infoFinestra{Exe: "systemsettings.exe", DiWindows: true, SchermoIntero: true}, false},
		{"Steam (il negozio)", infoFinestra{Exe: "steam.exe", DaNegozio: true}, false},
		{"gioco di Steam in finestra", infoFinestra{Exe: "repo.exe", DaNegozio: true}, true},
		{"gioco della libreria di Tasso in finestra", infoFinestra{Exe: "game.exe", DiTasso: true}, true},
		{"gioco già tradotto (profilo)", infoFinestra{Exe: "nms.exe", Profilo: true}, true},
		{"gioco sconosciuto senza bordi", infoFinestra{Exe: "indiegame.exe", SchermoIntero: true}, true},
		{"programma sconosciuto in finestra", infoFinestra{Exe: "calc2.exe"}, false},
		{"schermata già in traduzione", infoFinestra{Exe: "game.exe", Tradotto: true}, true},
		{"senza nome", infoFinestra{}, false},
	}
	for _, c := range casi {
		if got := decidiGioco(c.f); got != c.si {
			t.Errorf("%s: %v, atteso %v", c.nome, got, c.si)
		}
	}
}
