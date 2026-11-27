package traduce

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Elenco della 3.1.1 (senza firma): tolgo winhttp e doorstop, rinomino BepInEx (plugin dell'utente
// salvi), non tocco i file del gioco e metto da parte l'elenco. Un elenco con firma sbagliata no.
func TestTogliModVecchia(t *testing.T) {
	dirApp := t.TempDir()
	gioco := filepath.Join(t.TempDir(), "Gioco")
	scrivi := func(p, s string) {
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	scrivi(filepath.Join(gioco, "Gioco.exe"), "exe")
	scrivi(filepath.Join(gioco, "GameAssembly.dll"), "dll")
	scrivi(filepath.Join(gioco, "Gioco_Data", "x"), "x")
	scrivi(filepath.Join(gioco, "winhttp.dll"), "doorstop")
	scrivi(filepath.Join(gioco, "doorstop_config.ini"), "ini")
	scrivi(filepath.Join(gioco, "BepInEx", "plugins", "MioPlugin.dll"), "utente")
	scrivi(filepath.Join(gioco, "dotnet", "x.dll"), "bepinex6")
	m := ManifestoMod{Gioco: "Gioco", Cartella: gioco, Tipo: "il2cpp", Exe: "Gioco.exe", Aggiunti: []string{"BepInEx", "dotnet", "winhttp.dll", "Gioco.exe"}}

	// BepInEx messo dall'utente (senza il traduttore di Tasso): non lo tocco
	mv0 := m
	if err := togliModVecchia(t.TempDir(), "k0", mv0); err == nil || !esisteVoce(filepath.Join(gioco, "winhttp.dll")) {
		t.Fatal("tolto un BepInEx che non è di Tasso")
	}
	scrivi(filepath.Join(gioco, "BepInEx", "plugins", "XUnity.AutoTranslator", "XUnity.AutoTranslator.Plugin.Core.dll"), "x")
	scrivi(filepath.Join(gioco, "BepInEx", "config", "AutoTranslatorConfig.ini"), "[Custom]\nUrl=http://127.0.0.1:47321/gioco/k/t\n")
	b, _ := json.Marshal(m)
	scrivi(filepath.Join(cartellaCopie(dirApp, "k"), "manifesto.json"), string(b))

	if _, ok := leggiManifesto(dirApp, "k"); ok {
		t.Fatal("un elenco senza firma non si usa come valido")
	}
	mv, ok := leggiManifestoVecchio(dirApp, "k")
	if !ok {
		t.Fatal("elenco della 3.1.1 non riconosciuto")
	}
	if err := togliModVecchia(dirApp, "k", mv); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"winhttp.dll", "doorstop_config.ini", "BepInEx", "dotnet"} {
		if esisteVoce(filepath.Join(gioco, n)) {
			t.Fatalf("%s è rimasto", n)
		}
	}
	if !esisteVoce(filepath.Join(gioco, "Gioco.exe")) || !esisteVoce(filepath.Join(gioco, "GameAssembly.dll")) {
		t.Fatal("toccato un file del gioco")
	}
	rin, _ := filepath.Glob(filepath.Join(gioco, "BepInEx-tolto-da-Tasso-*", "plugins", "MioPlugin.dll"))
	if len(rin) != 1 {
		t.Fatal("il plugin dell'utente non è stato tenuto")
	}
	if esisteVoce(filepath.Join(cartellaCopie(dirApp, "k"), "manifesto.json")) || ModGiaPresenti(gioco) != "" {
		t.Fatal("l'elenco vecchio o il caricatore bloccano ancora la nuova traduzione")
	}

	// elenco Unreal della 3.1.1: messo da parte, nessun file toccato
	cartU := t.TempDir()
	scrivi(filepath.Join(cartU, "vuota", "segnaposto"), "x")
	_ = os.Remove(filepath.Join(cartU, "vuota", "segnaposto"))
	mu := ManifestoMod{Cartella: cartU, Tipo: "unreal", CreataMods: true, Aggiunti: []string{filepath.Join("vuota", nomeModUnreal)}}
	b, _ = json.Marshal(mu)
	scrivi(filepath.Join(cartellaCopie(dirApp, "ku"), "manifesto.json"), string(b))
	if err := togliModVecchia(dirApp, "ku", mu); err != nil || !esisteVoce(filepath.Join(cartU, "vuota")) {
		t.Fatalf("elenco Unreal vecchio: %v", err)
	}

	// firma presente ma sbagliata: non è un elenco vecchio
	m.Firma = "falsa"
	b, _ = json.Marshal(m)
	scrivi(filepath.Join(cartellaCopie(dirApp, "k2"), "manifesto.json"), string(b))
	if _, ok := leggiManifestoVecchio(dirApp, "k2"); ok {
		t.Fatal("elenco con firma sbagliata trattato come vecchio")
	}
	// cartella che non è un gioco Unity: non tocco niente
	altro := t.TempDir()
	scrivi(filepath.Join(altro, "winhttp.dll"), "di Windows")
	if err := togliModVecchia(dirApp, "k3", ManifestoMod{Cartella: altro, Tipo: "il2cpp"}); err == nil || !esisteVoce(filepath.Join(altro, "winhttp.dll")) {
		t.Fatal("toccata una cartella che non è un gioco")
	}
}
