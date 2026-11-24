package traduce

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// le voci scaricabili hanno sempre impronta, dimensione e indirizzo https
func TestRisorseIACompilate(t *testing.T) {
	esa := regexp.MustCompile(`^[0-9a-f]{64}$`)
	r := risorseIA
	for nome, f := range map[string]FileDaScaricare{"motore": r.Motore, "modello": r.Modello, "media": r.Media, "riserva": r.ModelloRiserva, "media riserva": r.MediaRiserva} {
		if !esa.MatchString(f.SHA256) || f.Byte <= 0 || !strings.HasPrefix(f.URL, "https://") || f.Nome == "" {
			t.Errorf("%s: voce incompleta %+v", nome, f)
		}
	}
	if r.Modello.Nome == r.ModelloRiserva.Nome || r.Media.Nome == r.MediaRiserva.Nome {
		t.Error("il file nuovo e quello di riserva devono avere nomi diversi")
	}
	if !motoreIADisponibile() {
		t.Error("il motore potente deve risultare disponibile")
	}
	if !strings.Contains(r.Motore.URL, "/b12380/") {
		t.Error("lo zip del motore deve essere della build delle impronte (b12380)")
	}
}

func TestImpronteLlama(t *testing.T) {
	esa := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for i, m := range versioniLlama {
		if len(m) != 31 || m["llama-server.exe"] == "" {
			t.Fatalf("build %d: %d impronte", i, len(m))
		}
		for n, h := range m {
			if !esa.MatchString(h) {
				t.Errorf("build %d, %s: impronta non valida", i, n)
			}
		}
	}
	if improntaLlama["llama-server.exe"] == improntaLlamaB12189["llama-server.exe"] {
		t.Error("le due build devono avere un llama-server.exe diverso")
	}
	dir := t.TempDir()
	if got := impronteLlamaDi(dir); got["llama-server.exe"] != improntaLlama["llama-server.exe"] {
		t.Error("cartella vuota: si usano le impronte della build nuova")
	}
}

func scriviConImpronta(t *testing.T, dir, nome string, dati []byte) FileDaScaricare {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, nome), dati, 0o644); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(dati)
	return FileDaScaricare{URL: "https://esempio/" + nome, Nome: nome, SHA256: hex.EncodeToString(h[:]), Byte: int64(len(dati))}
}

// chi ha già il modello di prima continua a usarlo; appena c'è quello nuovo si usa il nuovo
func TestModelloDiRiserva(t *testing.T) {
	vecchiaApp, vecchieRisorse := dirApp, risorseIA
	defer func() { dirApp, risorseIA = vecchiaApp, vecchieRisorse }()
	dirApp = t.TempDir()
	dir := dirIA()
	if err := os.MkdirAll(filepath.Join(dir, "llama"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "llama", "llama-server.exe"), []byte("finto"), 0o644)

	vecchio := scriviConImpronta(t, dir, "vecchio.gguf", []byte("modello di prima"))
	mediaVecchia := scriviConImpronta(t, dir, "media-vecchia.gguf", []byte("media di prima"))
	nuovo := FileDaScaricare{URL: "https://esempio/nuovo.gguf", Nome: "nuovo.gguf", SHA256: strings.Repeat("ab", 32), Byte: 12}
	mediaNuova := FileDaScaricare{URL: "https://esempio/media-nuova.gguf", Nome: "media-nuova.gguf", SHA256: strings.Repeat("cd", 32), Byte: 10}
	risorseIA.Modello, risorseIA.ModelloRiserva = nuovo, vecchio
	risorseIA.Media, risorseIA.MediaRiserva = mediaNuova, mediaVecchia

	if !iaPresente() || !mediaPresente() {
		t.Fatal("con i file di prima l'IA deve risultare installata")
	}
	if filepath.Base(fileModelloIA()) != "vecchio.gguf" || filepath.Base(fileMediaIA()) != "media-vecchia.gguf" {
		t.Fatalf("senza i file nuovi si usano quelli di prima: %s %s", fileModelloIA(), fileMediaIA())
	}
	if !iaDaAggiornare() {
		t.Error("con il modello di prima l'IA è da aggiornare")
	}
	pulisciRiserveIA() // i nuovi non ci sono: non deve cancellare niente
	if _, err := os.Stat(filepath.Join(dir, "vecchio.gguf")); err != nil {
		t.Fatal("cancellato il modello di prima senza avere il nuovo")
	}

	// arrivano i file nuovi (verificati)
	risorseIA.Modello = scriviConImpronta(t, dir, "nuovo.gguf", []byte("modello nuovo"))
	risorseIA.Media = scriviConImpronta(t, dir, "media-nuova.gguf", []byte("media nuova"))
	if filepath.Base(fileModelloIA()) != "nuovo.gguf" || filepath.Base(fileMediaIA()) != "media-nuova.gguf" {
		t.Fatalf("con i file nuovi si usano i nuovi: %s %s", fileModelloIA(), fileMediaIA())
	}
	pulisciRiserveIA()
	for _, n := range []string{"vecchio.gguf", "media-vecchia.gguf"} {
		if _, err := os.Stat(filepath.Join(dir, n)); !os.IsNotExist(err) {
			t.Errorf("%s doveva essere cancellato", n)
		}
	}
	if !iaPresente() || !mediaPresente() {
		t.Error("con i file nuovi l'IA deve risultare installata")
	}

	// niente file: non installata, e il percorso è quello del file nuovo (da scaricare)
	os.Remove(filepath.Join(dir, "nuovo.gguf"))
	if iaPresente() {
		t.Error("senza modello l'IA non è installata")
	}
	if filepath.Base(fileModelloIA()) != "nuovo.gguf" {
		t.Error("senza nessun file il percorso è quello del modello nuovo")
	}
}

func TestOpzioniVelociIA(t *testing.T) {
	nessuna := func(string) string { return "" }
	a := strings.Join(opzioniVelociIA(nessuna), " ")
	if a != "-fa on -ctk q8_0 -ctv q8_0" {
		t.Fatalf("opzioni di base: %q", a)
	}
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if a := strings.Join(opzioniVelociIA(env(map[string]string{"TASSO_IA_FA": "off"})), " "); a != "-fa off" {
		t.Errorf("flash attention spenta: niente memoria compressa, ho %q", a)
	}
	if a := strings.Join(opzioniVelociIA(env(map[string]string{"TASSO_IA_KV": "f16", "TASSO_IA_FA": "auto"})), " "); a != "-fa auto" {
		t.Errorf("memoria normale: %q", a)
	}
	if a := strings.Join(opzioniVelociIA(env(map[string]string{"TASSO_IA_FA": "--qualcosa", "TASSO_IA_KV": "q2; rm"})), " "); a != "-fa on -ctk q8_0 -ctv q8_0" {
		t.Errorf("valori sconosciuti non devono passare: %q", a)
	}
	a = strings.Join(opzioniVelociIA(env(map[string]string{"TASSO_IA_NGRAM": "1"})), " ")
	if !strings.Contains(a, "--spec-type ngram-simple --spec-ngram-simple-size-n 3 --spec-ngram-simple-size-m 8") {
		t.Errorf("n-gram acceso: %q", a)
	}
	if strings.Contains(strings.Join(opzioniVelociIA(nessuna), " "), "--spec-type") != iaNgram {
		t.Error("l'opzione n-gram deve seguire la costante iaNgram (spenta di base)")
	}
	if iaNgram {
		t.Error("n-gram deve essere spento di base finché non è misurato")
	}
}
