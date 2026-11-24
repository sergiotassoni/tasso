//go:build windows && collaudo

package traduce

// Prova del riconoscimento dell'anti-cheat sulle librerie vere, senza toccare niente:
// Tasso-collaudo.exe --prova-anticheat "D:\SteamLibrary\steamapps\common;C:\...\Giochi"
// Per ogni cartella di gioco scrive se Tasso vede un anti-cheat e cosa farebbe "Applica":
// sullo schermo (anti-cheat senza conferma) o dentro il gioco solo con la conferma del rischio.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	for i, a := range os.Args {
		if a != "--prova-anticheat" || i+1 >= len(os.Args) {
			continue
		}
		var b strings.Builder
		for _, lib := range strings.Split(os.Args[i+1], ";") {
			voci, err := os.ReadDir(strings.TrimSpace(lib))
			if err != nil {
				fmt.Fprintf(&b, "%s: non leggibile (%v)\n", lib, err)
				continue
			}
			for _, v := range voci {
				if !v.IsDir() {
					continue
				}
				c := filepath.Join(lib, v.Name())
				ac := haAntiCheat(c)
				cosa := "si può tradurre dentro il gioco (se è Unity) o sullo schermo"
				if ac {
					cosa = "ANTI-CHEAT: Applica traduce sullo schermo; dentro il gioco solo spuntando 'Ho capito il rischio di ban'"
				}
				fmt.Fprintf(&b, "%-45s anti-cheat=%-5v %s\n", v.Name(), ac, cosa)
			}
		}
		dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "Tasso", "logs")
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, "prova-anticheat.txt"), []byte(b.String()), 0o644)
		os.Exit(0)
	}
}
