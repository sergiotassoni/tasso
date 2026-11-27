package traduce

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// C2 (REV-INT-32b): un originale cambiato dopo non blocca per sempre. La traduzione si toglie, il
// file cambiato non si rimette, l'errore dice di usare "Verifica file" (non "il gioco è aperto") e
// gli originali restano in una cartella "da controllare" (togliendo Tasso vanno in Documenti).
func TestOriginaleCambiatoNonBlocca(t *testing.T) {
	app := t.TempDir()
	gioco := filepath.Join(t.TempDir(), "Gioco")
	for p, s := range map[string]string{"Gioco.exe": "exe", "GameAssembly.dll": "dll", filepath.Join("Gioco_Data", "x"): "x", "winhttp.dll": "doorstop", "x.dll": "tradotto"} {
		_ = os.MkdirAll(filepath.Dir(filepath.Join(gioco, p)), 0o755)
		_ = os.WriteFile(filepath.Join(gioco, p), []byte(s), 0o644)
	}
	m := ManifestoMod{Gioco: "Gioco", Cartella: gioco, Tipo: "il2cpp", Exe: "Gioco.exe", Aggiunti: []string{"winhttp.dll"}, Sostituiti: []string{"x.dll"}}
	d := cartellaCopie(app, "g")
	_ = os.MkdirAll(filepath.Join(d, "originali"), 0o755)
	_ = os.WriteFile(filepath.Join(d, "originali", "x.dll"), []byte("vero"), 0o644)
	if err := scriviManifesto(app, "g", m); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(d, "originali", "x.dll"), []byte("CAMBIATO"), 0o644)
	err := TogliModGioco(app, "g")
	var dc *ErrOriginaliDaControllare
	if !errors.As(err, &dc) || len(dc.File) != 1 || dc.File[0] != "x.dll" {
		t.Fatalf("atteso ErrOriginaliDaControllare, ho %v", err)
	}
	if strings.Contains(err.Error(), "aperto") || !strings.Contains(err.Error(), "Verifica file") {
		t.Fatalf("messaggio sbagliato: %s", err)
	}
	if esisteVoce(filepath.Join(gioco, "winhttp.dll")) {
		t.Fatal("la traduzione non è stata tolta")
	}
	if b, _ := os.ReadFile(filepath.Join(gioco, "x.dll")); string(b) != "tradotto" {
		t.Fatal("ha toccato il file del gioco con un originale cambiato")
	}
	if esisteVoce(filepath.Join(d, "manifesto.json")) {
		t.Fatal("l'elenco resta: la prossima volta si bloccherebbe di nuovo")
	}
	if b, err := os.ReadFile(filepath.Join(dc.Dove, "x.dll")); err != nil || string(b) != "CAMBIATO" || !strings.Contains(filepath.Base(dc.Dove), "da-controllare") {
		t.Fatalf("copia da controllare non tenuta: %s %v", dc.Dove, err)
	}
	// seconda volta: niente da togliere, niente blocco
	if err := TogliModGioco(app, "g"); err != nil {
		t.Fatal(err)
	}
}

// R1 (REV-INT-32b): cartella del gioco come collegamento o giunzione. Non si installa (con il
// motivo), e per un gioco già tradotto così "Ferma"/"Togli Tasso" danno "da sistemare a mano"
// (Verifica file, dove sono gli originali), mai un errore "in uso" che fa riprovare all'infinito.
func TestCartellaGiocoCollegamento(t *testing.T) {
	vera := filepath.Join(t.TempDir(), "Gioco")
	for p, s := range map[string]string{"Gioco.exe": "exe", "GameAssembly.dll": "dll", filepath.Join("Gioco_Data", "x"): "x", "winhttp.dll": "doorstop"} {
		_ = os.MkdirAll(filepath.Dir(filepath.Join(vera, p)), 0o755)
		_ = os.WriteFile(filepath.Join(vera, p), []byte(s), 0o644)
	}
	link := filepath.Join(t.TempDir(), "GiocoLink")
	if err := os.Symlink(vera, link); err != nil {
		t.Skip("collegamenti non disponibili:", err)
	}
	if ok, motivo := ModPossibile(link); ok || motivo != motivoCartellaCollegamento {
		t.Fatalf("ModPossibile su un collegamento: %v %q", ok, motivo)
	}
	if ok, _ := ModPossibile(vera); !ok && percorsoSenzaLink(vera) {
		// (la cartella vera va bene, salvo i controlli dei dati del gioco)
		t.Log("cartella vera non possibile per altri motivi")
	}
	app := t.TempDir()
	if _, err := InstallaModGioco(app, "Gioco", link, "g", nil, "en", "it"); err == nil || !strings.Contains(err.Error(), "collegamento") {
		t.Fatalf("installata su un collegamento: %v", err)
	}
	m := ManifestoMod{Gioco: "Gioco", Cartella: link, Tipo: "il2cpp", Exe: "Gioco.exe", Aggiunti: []string{"winhttp.dll"}}
	if err := scriviManifesto(app, "g", m); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		err := TogliModGioco(app, "g")
		var am *ErrDaSistemareAMano
		var iu *errInUso
		if !errors.As(err, &am) || errors.As(err, &iu) {
			t.Fatalf("giro %d: atteso ErrDaSistemareAMano, ho %v", i, err)
		}
		if !strings.Contains(err.Error(), "Verifica file") || strings.Contains(err.Error(), "aperto") || !strings.Contains(err.Error(), "originali") {
			t.Fatalf("messaggio sbagliato: %s", err)
		}
	}
	if !esisteVoce(filepath.Join(vera, "winhttp.dll")) {
		t.Fatal("ha scritto nel gioco passando dal collegamento")
	}
}

// R6: un file aggiunto che passa da un collegamento resta nel gioco e il messaggio lo dice
// ("tranne questi file"), invece di dire "Ho tolto la traduzione".
func TestAggiuntoDietroCollegamento(t *testing.T) {
	app := t.TempDir()
	gioco := filepath.Join(t.TempDir(), "Gioco")
	for p, s := range map[string]string{"Gioco.exe": "exe", "GameAssembly.dll": "dll", filepath.Join("Gioco_Data", "x"): "x", "winhttp.dll": "doorstop"} {
		_ = os.MkdirAll(filepath.Dir(filepath.Join(gioco, p)), 0o755)
		_ = os.WriteFile(filepath.Join(gioco, p), []byte(s), 0o644)
	}
	altrove := t.TempDir()
	if err := os.Symlink(altrove, filepath.Join(gioco, "BepInEx")); err != nil {
		t.Skip(err)
	}
	m := ManifestoMod{Gioco: "Gioco", Cartella: gioco, Tipo: "il2cpp", Exe: "Gioco.exe", Aggiunti: []string{"winhttp.dll", "BepInEx"}}
	if err := scriviManifesto(app, "g", m); err != nil {
		t.Fatal(err)
	}
	err := TogliModGioco(app, "g")
	var dc *ErrOriginaliDaControllare
	if !errors.As(err, &dc) || len(dc.NonTolti) != 1 || dc.NonTolti[0] != "BepInEx" {
		t.Fatalf("atteso ErrOriginaliDaControllare con BepInEx non tolto, ho %v", err)
	}
	if !strings.Contains(err.Error(), "tranne") || esisteVoce(filepath.Join(gioco, "winhttp.dll")) {
		t.Fatalf("messaggio o rimozione sbagliati: %s", err)
	}
}
