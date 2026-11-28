package traduce

import "testing"

func TestGiocoDaLauncher(t *testing.T) {
	voci := []VoceLibreria{
		{Nome: "Hytale Launcher", Exe: `C:\Program Files\Hypixel Studios\Hytale Launcher\hytale-launcher.exe`},
		{Nome: "Mio Gioco", Exe: `D:\Giochi\MioGioco\Launcher.exe`},
		{Nome: "Altro", Exe: `D:\Giochi\Altro\altro.exe`},
	}
	editori := map[string]string{
		`c:\program files\hypixel studios\hytale launcher\hytale-launcher.exe`: "Hypixel Studios",
		`d:\giochi\altro\altro.exe`: "Microsoft Corporation",
	}
	ed := func(p string) string { return editori[lower(p)] }
	casi := []struct {
		p    datiProcesso
		vuoi bool
	}{
		// gioco in AppData, figlio del launcher
		{datiProcesso{Percorso: `C:\Users\s\AppData\Roaming\Hytale\install\release\package\game\latest\Client\HytaleClient.exe`}, true},
		{datiProcesso{Percorso: `E:\x\client.exe`, Antenati: []string{`C:\Program Files\Hypixel Studios\Hytale Launcher\hytale-launcher.exe`}}, true},
		// stessa cartella / sottocartella del launcher
		{datiProcesso{Percorso: `D:\Giochi\MioGioco\Bin\Win64\Game-Win64-Shipping.exe`}, true},
		// stesso editore, non generico
		{datiProcesso{Percorso: `E:\y\qualcosa.exe`, Editore: "Hypixel Studios"}, true},
		// editore generico: no
		{datiProcesso{Percorso: `C:\Windows\notepad.exe`, Editore: "Microsoft Corporation"}, false},
		// programma qualsiasi
		{datiProcesso{Percorso: `C:\Program Files\Google\Chrome\Application\chrome.exe`, Antenati: []string{`c:\windows\explorer.exe`}}, false},
	}
	for _, c := range casi {
		if _, ok := giocoDaLibreria(c.p, voci, ed); ok != c.vuoi {
			t.Errorf("%s: %v, atteso %v", c.p.Percorso, ok, c.vuoi)
		}
	}
	if senzaLauncher("Hytale Launcher") != "Hytale" || senzaLauncher("Launcher") != "Launcher" {
		t.Error("senzaLauncher")
	}
}

func lower(s string) string {
	b := []rune(s)
	for i, r := range b {
		if r >= 'A' && r <= 'Z' {
			b[i] = r + 32
		}
	}
	return string(b)
}
