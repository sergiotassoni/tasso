package traduce

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestRiconosciGioco(t *testing.T) {
	casi := []struct {
		percorso, nome, piatt string
		ok                    bool
	}{
		{`D:\SteamLibrary\steamapps\common\Path of Titans\PathOfTitans\Binaries\Win64\PathOfTitans-Win64-Shipping.exe`, "Path of Titans", "Steam", true},
		{`C:\Program Files\Epic Games\Fortnite\FortniteGame\Binaries\Win64\FortniteClient-Win64-Shipping.exe`, "Fortnite", "Epic Games", true},
		{`C:\Program Files (x86)\GOG Galaxy\Games\The Witcher 3\bin\x64\witcher3.exe`, "The Witcher 3", "GOG", true},
		{`C:\XboxGames\Starfield\Content\Starfield.exe`, "Starfield", "Xbox", true},
		{`C:\Program Files\EA Games\Hollow_Knight\hk.exe`, "Hollow Knight", "EA", true},
		{`C:\Program Files (x86)\Steam\steam.exe`, "", "", false},
		{`C:\Program Files (x86)\Steam\steamapps\common\Steamworks Shared\x.exe`, "", "", false},
		{`C:\Windows\notepad.exe`, "", "", false},
		{`C:\Program Files\WindowsApps\Microsoft.WindowsCalculator_11.0_x64__8wekyb3d8bbwe\Calc.exe`, "", "", false},
		{`C:\Program Files\WindowsApps\Microsoft.MinecraftUWP_1.20.0_x64__8wekyb3d8bbwe\Minecraft.Windows.exe`, "MinecraftUWP", "Microsoft Store", true},
	}
	for _, c := range casi {
		g, ok := riconosciGioco(c.percorso)
		if ok != c.ok || g.Nome != c.nome || g.Piattaforma != c.piatt {
			t.Errorf("%s -> %+v %v", c.percorso, g, ok)
		}
	}
}

func TestNomeSteam(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "appmanifest_1.acf"), []byte("\"AppState\"\n{\n\t\"appid\"\t\t\"1\"\n\t\"name\"\t\t\"Hollow Knight: Silksong\"\n\t\"installdir\"\t\t\"Silksong\"\n}"), 0o644)
	if n := nomeSteam(d, "silksong"); n != "Hollow Knight: Silksong" {
		t.Errorf("nome Steam: %q", n)
	}
}

func TestGiochiSteamEpic(t *testing.T) {
	steam := t.TempDir()
	lib2 := t.TempDir()
	os.MkdirAll(filepath.Join(steam, "steamapps"), 0o755)
	os.MkdirAll(filepath.Join(lib2, "steamapps"), 0o755)
	os.WriteFile(filepath.Join(steam, "steamapps", "libraryfolders.vdf"), []byte("\"libraryfolders\"\n{\n\"0\"\n{\n\"path\"\t\t\""+steam+"\"\n}\n\"1\"\n{\n\"path\"\t\t\""+lib2+"\"\n}\n}"), 0o644)
	os.WriteFile(filepath.Join(steam, "steamapps", "appmanifest_1.acf"), []byte(`"name" "Path of Titans" "installdir" "Path of Titans"`), 0o644)
	os.WriteFile(filepath.Join(lib2, "steamapps", "appmanifest_2.acf"), []byte(`"name" "Hades" "installdir" "Hades"`), 0o644)
	os.WriteFile(filepath.Join(lib2, "steamapps", "appmanifest_3.acf"), []byte(`"name" "Steamworks Common Redistributables" "installdir" "Steamworks Shared"`), 0o644)
	os.MkdirAll(filepath.Join(steam, "appcache", "librarycache", "77", "abc"), 0o755)
	os.WriteFile(filepath.Join(steam, "appcache", "librarycache", "77", "abc", "library_600x900.jpg"), []byte("jpg"), 0o644)
	if c := copertinaSteam(steam, "77"); filepath.Base(c) != "library_600x900.jpg" {
		t.Errorf("copertina: %q", c)
	}
	g := giochiSteam(steam)
	if len(g) != 2 {
		t.Fatalf("attesi 2 giochi Steam: %+v", g)
	}
	epic := t.TempDir()
	os.WriteFile(filepath.Join(epic, "a.item"), []byte(`{"DisplayName": "Fortnite", "InstallLocation": "C:\\Program Files\\Epic Games\\Fortnite"}`), 0o644)
	e := giochiEpic(epic)
	if len(e) != 1 || e[0].Nome != "Fortnite" || e[0].Cartella != `C:\Program Files\Epic Games\Fortnite` {
		t.Fatalf("Epic: %+v", e)
	}
}

func TestAntiCheat(t *testing.T) {
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, "EasyAntiCheat"), 0o755)
	if !ConAntiCheat(GiocoRiconosciuto{Cartella: d, Piattaforma: "Steam"}) {
		t.Error("EasyAntiCheat non visto")
	}
	if ConAntiCheat(GiocoRiconosciuto{Cartella: t.TempDir(), Piattaforma: "Steam"}) {
		t.Error("niente anti-imbroglio")
	}
	// cartelle tipiche più in basso: Gioco\Binaries\Win64\EasyAntiCheat e BattlEye in una sottocartella
	d2 := t.TempDir()
	os.MkdirAll(filepath.Join(d2, "Gioco", "Binaries", "Win64", "EasyAntiCheat"), 0o755)
	if !ConAntiCheat(GiocoRiconosciuto{Cartella: d2, Piattaforma: "Steam"}) {
		t.Error("EasyAntiCheat in Gioco\\Binaries\\Win64 non visto")
	}
	d3 := t.TempDir()
	os.MkdirAll(filepath.Join(d3, "Gioco", "BattlEye"), 0o755)
	if !ConAntiCheat(GiocoRiconosciuto{Cartella: d3, Piattaforma: "Steam"}) {
		t.Error("BattlEye in sottocartella non visto")
	}
	// cartella Engine enorme: la salto, e Binaries viene guardata per prima
	d5 := t.TempDir()
	for i := 0; i < 400; i++ {
		os.MkdirAll(filepath.Join(d5, "Engine", fmt.Sprintf("m%03d", i)), 0o755)
		os.MkdirAll(filepath.Join(d5, "Altro", fmt.Sprintf("m%03d", i)), 0o755)
	}
	os.MkdirAll(filepath.Join(d5, "Zeta", "Binaries", "Win64", "EasyAntiCheat"), 0o755)
	if !ConAntiCheat(GiocoRiconosciuto{Nome: "Zeta", Cartella: d5, Piattaforma: "Steam"}) {
		t.Error("EasyAntiCheat non visto dietro una cartella enorme")
	}
	// troppo in basso (oltre 3 livelli) o con nomi comuni: niente
	d4 := t.TempDir()
	os.MkdirAll(filepath.Join(d4, "a", "b", "c", "d", "BattlEye"), 0o755)
	os.MkdirAll(filepath.Join(d4, "Content", "Paks"), 0o755)
	if ConAntiCheat(GiocoRiconosciuto{Cartella: d4, Piattaforma: "Steam"}) {
		t.Error("anti-imbroglio visto troppo in basso")
	}
	g, ok := riconosciGioco(`C:\Riot Games\League of Legends\Game\League of Legends.exe`)
	if !ok || !ConAntiCheat(g) {
		t.Errorf("League: %+v %v", g, ok)
	}
	if _, ok := riconosciGioco(`C:\Riot Games\Riot Client\RiotClientServices.exe`); ok {
		t.Error("il client Riot non è un gioco")
	}
}

func TestLinguaSteam(t *testing.T) {
	d := t.TempDir()
	sa := filepath.Join(d, "steamapps")
	os.MkdirAll(filepath.Join(sa, "common", "Palworld"), 0o755)
	os.WriteFile(filepath.Join(sa, "appmanifest_1623730.acf"), []byte("\"AppState\"\n{\n\t\"appid\"\t\t\"1623730\"\n\t\"installdir\"\t\t\"Palworld\"\n\t\"UserConfig\"\n\t{\n\t\t\"language\"\t\t\"koreana\"\n\t}\n}"), 0o644)
	if l := linguaSteam(filepath.Join(sa, "common", "Palworld")); l != "ko" {
		t.Fatalf("coreano: %q", l)
	}
	if l := linguaSteam(filepath.Join(sa, "common", "Altro")); l != "" {
		t.Fatalf("gioco diverso: %q", l)
	}
	if l := linguaSteam(filepath.Join(d, "Giochi", "Palworld")); l != "" {
		t.Fatalf("non Steam: %q", l)
	}
}
