package traduce

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"tasso/traduce/unreal"
)

func pakProva(t *testing.T, voci []unreal.FileNuovo) string {
	paks := filepath.Join(t.TempDir(), "G", "Content", "Paks")
	_ = os.MkdirAll(paks, 0o755)
	f, _ := os.Create(filepath.Join(paks, "G-Windows.pak"))
	if err := unreal.ScriviPak(f, voci); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return paks
}

func molte(n int) []string {
	var s []string
	for i := 0; i < n; i++ {
		s = append(s, fmt.Sprintf("Frase tradotta numero %d", i))
	}
	return s
}

func TestGiocoHaLingua(t *testing.T) {
	piena := locresV1(molte(200))
	vuota := locresV1([]string{"Ciao"})
	t.Logf("piena=%d vuota=%d; 4096 = circa %d frasi da 25 car.", len(piena), len(vuota), 4096/30)
	casi := []struct {
		nome string
		v    []unreal.FileNuovo
		vuoi bool
	}{
		{"gioco tradotto", []unreal.FileNuovo{{Nome: "G/Content/Localization/Game/it/Game.locres", Dati: piena}}, true},
		{"locres quasi vuoto", []unreal.FileNuovo{{Nome: "G/Content/Localization/Game/it/Game.locres", Dati: vuota}}, false},
		{"solo Engine", []unreal.FileNuovo{{Nome: "Engine/Content/Localization/Engine/it/Engine.locres", Dati: piena}}, false},
		{"plugin del motore", []unreal.FileNuovo{{Nome: "Engine/Plugins/X/Content/Localization/X/it/X.locres", Dati: piena}}, false},
		{"progetto EngineRush", []unreal.FileNuovo{{Nome: "EngineRush/Content/Localization/Game/it/Game.locres", Dati: piena}}, true},
		{"altra lingua", []unreal.FileNuovo{{Nome: "G/Content/Localization/Game/de/Game.locres", Dati: piena}}, false},
	}
	for _, c := range casi {
		if got := GiocoHaLingua(pakProva(t, c.v), "it"); got != c.vuoi {
			t.Errorf("%s: %v, volevo %v", c.nome, got, c.vuoi)
		}
	}
}
