package traduce

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func init() {
	// nei test la "cartella sicura" è una cartella temporanea
	CartellaSicura, _ = os.MkdirTemp("", "tasso-sicura")
}

// Un elenco scritto da un altro programma (senza la firma di Tasso) non si usa: Tasso non toglie
// né rimette niente da lì. Un originale cambiato dopo non si rimette nel gioco.
func TestManifestoFalso(t *testing.T) {
	app, gioco := t.TempDir(), t.TempDir()
	m := ManifestoMod{Gioco: "Prova", Cartella: gioco, Sostituiti: []string{"x.dll"}}
	d := cartellaCopie(app, "prova")
	_ = os.MkdirAll(filepath.Join(d, "originali"), 0o755)
	_ = os.WriteFile(filepath.Join(d, "originali", "x.dll"), []byte("vero"), 0o644)
	if err := scriviManifesto(app, "prova", m); err != nil {
		t.Fatal(err)
	}
	if _, ok := leggiManifesto(app, "prova"); !ok {
		t.Fatal("l'elenco firmato da Tasso non si legge")
	}
	// un altro programma cambia l'originale messo da parte: non lo rimetto
	_ = os.WriteFile(filepath.Join(d, "originali", "x.dll"), []byte("CODICE DI UN ALTRO"), 0o644)
	_ = os.WriteFile(filepath.Join(gioco, "x.dll"), []byte("tradotto"), 0o644)
	if err := TogliModGioco(app, "prova"); err == nil {
		t.Error("originale cambiato: doveva dire che non l'ha rimesso")
	}
	if b, _ := os.ReadFile(filepath.Join(gioco, "x.dll")); string(b) == "CODICE DI UN ALTRO" {
		t.Fatal("ha copiato nel gioco un originale cambiato da un altro programma")
	}
	// elenco finto scritto da un altro programma
	b, _ := json.Marshal(ManifestoMod{Gioco: "Finto", Cartella: gioco, Aggiunti: []string{"x.dll"}})
	_ = os.WriteFile(filepath.Join(d, "manifesto.json"), b, 0o644)
	if _, ok := leggiManifesto(app, "prova"); ok {
		t.Fatal("elenco senza firma accettato")
	}
	_ = TogliModGioco(app, "prova")
	if _, err := os.Stat(filepath.Join(gioco, "x.dll")); err != nil {
		t.Fatal("ha cancellato un file seguendo un elenco finto")
	}
}

// Un elenco finto messo prima che Tasso abbia mai firmato niente (nessuna chiave, utente nuovo)
// non diventa mai valido: niente "firma degli elenchi vecchi".
func TestManifestoFintoPrimaDellaChiave(t *testing.T) {
	vecchia, vecchiaMem := CartellaSicura, chiaveMem
	CartellaSicura, chiaveMem = t.TempDir(), nil
	defer func() { CartellaSicura, chiaveMem = vecchia, vecchiaMem }()
	app, gioco := t.TempDir(), t.TempDir()
	d := cartellaCopie(app, "finto")
	_ = os.MkdirAll(d, 0o755)
	b, _ := json.Marshal(ManifestoMod{Gioco: "Finto", Cartella: gioco, Aggiunti: []string{"x.dll"}})
	_ = os.WriteFile(filepath.Join(d, "manifesto.json"), b, 0o644)
	for i := 0; i < 3; i++ {
		if _, ok := leggiManifesto(app, "finto"); ok {
			t.Fatal("elenco finto accettato")
		}
	}
}
