package traduce

// Prove dei salvataggi su disco, della coda del correttore e delle altre correzioni del 24/11.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMemoriaTogliePiuVecchie(t *testing.T) {
	tr := NuovoTraduttore("", Glossario{})
	tr.mu.Lock()
	for i := 0; i <= maxCache; i++ {
		tr.mettiInCacheLocked(fmt.Sprintf("\x00frase %d", i), "x", true)
	}
	n := len(tr.cache)
	_, primaC := tr.cache["\x00frase 0"]
	_, ultimaC := tr.cache[fmt.Sprintf("\x00frase %d", maxCache)]
	ordine := len(tr.usoCache)
	tr.mu.Unlock()
	if n == 0 || n > maxCache || primaC || !ultimaC || ordine != n {
		t.Fatalf("memoria: %d voci (ordine %d), prima %v, ultima %v", n, ordine, primaC, ultimaC)
	}
}

func TestMemoriaSuDiscoInOrdine(t *testing.T) {
	f := filepath.Join(t.TempDir(), "cache.json")
	tr := NuovoTraduttore(f, Glossario{}, &motoreFinto{nome: "Mozilla"})
	tr.Traduci(context.Background(), "The merchant waits by the old river tonight.")
	tr.Traduci(context.Background(), "Bring three wolf pelts to the blacksmith.")
	tr.SalvaMemoria()
	st1, err := os.Stat(f)
	if err != nil {
		t.Fatal(err)
	}
	// niente di nuovo: il file non si riscrive
	_ = os.Chtimes(f, time.Unix(1000, 0), time.Unix(1000, 0))
	tr.SalvaMemoria()
	if st2, _ := os.Stat(f); !st2.ModTime().Equal(time.Unix(1000, 0)) || st2.Size() != st1.Size() {
		t.Fatal("memoria riscritta senza modifiche")
	}
	tr2 := NuovoTraduttore(f, Glossario{})
	if ch := tr2.chiaviPerUsoLocked(); len(ch) != 2 || !strings.Contains(ch[0], "The merchant") {
		t.Fatalf("ordine perso: %q", ch)
	}
	// i file delle versioni di prima (mappa) si leggono ancora
	vecchio := filepath.Join(t.TempDir(), "vecchia.json")
	_ = os.WriteFile(vecchio, []byte(`{"\u0000Hello":"Ciao"}`), 0o644)
	if tr3 := NuovoTraduttore(vecchio, Glossario{}); tr3.cache["\x00Hello"] != "Ciao" || len(tr3.usoCache) != 1 {
		t.Fatalf("formato vecchio non letto: %v", tr3.cache)
	}
}

func TestRigheDaFileNonInMemoria(t *testing.T) {
	d := t.TempDir()
	tr := NuovoTraduttore(filepath.Join(d, "cache.json"), Glossario{}, &motoreFinto{nome: "Mozilla"})
	tr.ApriLibreria(filepath.Join(d, "gioco.json"))
	r := tr.TraduciPer(context.Background(), "The bridge to the north is guarded day and night.", true, "en", "it")
	if r.Tradotto == "" {
		t.Fatalf("%+v", r)
	}
	tr.mu.Lock()
	nc, nl, prog := len(tr.cache), len(tr.libreria), tr.salvaInArrivo
	tr.mu.Unlock()
	if nc != 0 || nl != 1 || prog {
		t.Fatalf("memoria %d, libreria %d, salvataggio programmato %v", nc, nl, prog)
	}
}

func TestLibreriaIlleggibileMessaDaParte(t *testing.T) {
	d := t.TempDir()
	f := filepath.Join(d, "gioco.json")
	_ = os.WriteFile(f, []byte(`{"rotta": `), 0o644)
	tr := NuovoTraduttore("", Glossario{}, &motoreFinto{nome: "Mozilla"})
	tr.ApriLibreria(f)
	tr.TraduciScritta(context.Background(), "The bridge is broken.")
	tr.SalvaMemoria()
	if ContaLibreria(f) != 1 {
		t.Fatalf("libreria nuova: %d", ContaLibreria(f))
	}
	bak, _ := filepath.Glob(filepath.Join(d, "gioco-illeggibile-*.json.bak"))
	if len(bak) != 1 {
		t.Fatalf("copia della libreria rovinata: %v", bak)
	}
	if b, _ := os.ReadFile(bak[0]); string(b) != `{"rotta": ` {
		t.Fatalf("copia cambiata: %q", b)
	}
}

func TestQuasiUgualeSceltaFissa(t *testing.T) {
	cand := []candidatoQuasi{
		{"Hello there, traveller. Welcome to our villaqe.", "B"},
		{"Hello there, traveller. Welcome to our village.", "A"},
		{"Hello there, traveller. Welcome to our vilage.", "C"},
		{"Something else entirely, my friend from far away.", "X"},
	}
	in := "Hello there, traveller. Welcome to our village!"
	for i := 0; i < 5; i++ {
		// stesso risultato in qualunque ordine
		cand[0], cand[i%len(cand)] = cand[i%len(cand)], cand[0]
		if v, ok := sceltaQuasiUguale(in, cand); !ok || v != "A" {
			t.Fatalf("scelta %q %v", v, ok)
		}
	}
	tr := NuovoTraduttore("", Glossario{}, &motoreFinto{nome: "Mozilla"})
	tr.ApriLibreria(filepath.Join(t.TempDir(), "g.json"))
	tr.TraduciScritta(context.Background(), "Hello there, traveller. Welcome to our village.")
	r := tr.TraduciScritta(context.Background(), "Hel1o there, traveller. Welcome to our village.")
	if r.Motore != "libreria" || r.Tradotto != "IT:Hello there, traveller. Welcome to our village." {
		t.Fatalf("quasi uguale non trovata: %+v", r)
	}
}

type correttoreSpento struct {
	mu  sync.Mutex
	err error
	n   int
}

func (c *correttoreSpento) Nome() string { return "IA" }
func (c *correttoreSpento) Traduci(ctx context.Context, r RichiestaTraduzione) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	if c.err != nil {
		return "", c.err
	}
	return r.Bozza, nil
}

func TestCorrezioneMotoreSpentoNonVista(t *testing.T) {
	tr := NuovoTraduttore("", Glossario{}, mozFinto{})
	ia := &correttoreSpento{err: errMotoreSpento}
	tr.ImpostaCorrettore(ia, nil)
	l := lavoroCorrezione{chiave: "\x00Yes.", battuta: "Yes.", bozza: "Sì."}
	tr.unaCorrezione(l)
	tr.corr.mu.Lock()
	vista := tr.corr.viste[l.chiave]
	tr.corr.mu.Unlock()
	if vista {
		t.Fatal("con il motore spento la frase non deve contare come vista")
	}
	ia.mu.Lock()
	ia.err = nil
	ia.mu.Unlock()
	tr.unaCorrezione(l)
	tr.corr.mu.Lock()
	vista = tr.corr.viste[l.chiave]
	tr.corr.mu.Unlock()
	if !vista {
		t.Fatal("dopo una risposta la frase è vista")
	}
	// tre errori di fila: basta riprovare
	ia.mu.Lock()
	ia.err = errors.New("tempo scaduto")
	ia.mu.Unlock()
	l2 := lavoroCorrezione{chiave: "\x00No.", battuta: "No.", bozza: "No."}
	for i := 0; i < 3; i++ {
		tr.unaCorrezione(l2)
	}
	tr.corr.mu.Lock()
	vista = tr.corr.viste[l2.chiave]
	tr.corr.mu.Unlock()
	if !vista {
		t.Fatal("dopo 3 errori la frase non va più riprovata")
	}
}

func TestRispostaTagliataScartata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Привет, как"},"finish_reason":"length"}]}`))
	}))
	defer srv.Close()
	c := &ClientIA{URL: srv.URL}
	if _, err := c.Traduci(context.Background(), RichiestaTraduzione{Testo: "Hello, how are you?", Bozza: "Привет, как дела?"}); !errors.Is(err, errRispostaScartata) {
		t.Fatalf("risposta tagliata accettata: %v", err)
	}
}

func TestTokenLingueNonLatine(t *testing.T) {
	en := "Hello, how are you today? I hope everything is going well."
	lat := tokenMassimi(RichiestaTraduzione{Testo: en, Bozza: "Ciao, come stai oggi? Spero che vada tutto bene."})
	ja := tokenMassimi(RichiestaTraduzione{Testo: en, Bozza: "こんにちは、今日はお元気ですか？すべてが順調であることを願っています。"})
	if ja <= len([]rune("こんにちは、今日はお元気ですか？すべてが順調であることを願っています。")) || ja <= lat/2 {
		t.Fatalf("giapponese: %d pezzi (italiano %d)", ja, lat)
	}
	if !nonLatina("Привет") || nonLatina("Ciao, perché") {
		t.Fatal("nonLatina")
	}
}

func TestStessaFraseInglese(t *testing.T) {
	if !stessaFraseInglese("Old Keeper: Your HP is\nlow.", "Your HP is low.") {
		t.Error("con il nome davanti e l'a capo è la stessa frase")
	}
	if stessaFraseInglese("Yeah.", "Yes.") {
		t.Error("Yes. e Yeah. sono frasi diverse")
	}
	if !stessaFraseInglese("Yes.", "Yes.") {
		t.Error("uguale")
	}
}

type mozCoppie struct{ n int }

func (m *mozCoppie) Nome() string { return "Mozilla" }
func (m *mozCoppie) Traduci(ctx context.Context, r RichiestaTraduzione) (string, error) {
	return "IT:" + r.Testo, nil
}
func (m *mozCoppie) TraduciCoppia(ctx context.Context, testo, da, a string) (string, error) {
	m.n++
	return a + ":" + testo, nil
}

func TestMessaggiChatNonSalvati(t *testing.T) {
	m := &mozCoppie{}
	tr := NuovoTraduttore("", Glossario{}, m)
	tr.Coppia = func() (string, string) { return "en", "de" }
	tr.ImpostaCorrettore(correttoreFinto{"x"}, func() bool { return true }) // in pausa: la coda resta piena
	tr.TraduciMessaggio(context.Background(), "Marco: we need to go now")
	tr.mu.Lock()
	n := len(tr.altreCoppie)
	tr.mu.Unlock()
	_, _, inCoda := tr.StatoCorrezioni()
	if n != 0 || inCoda != 0 {
		t.Fatalf("messaggio della chat salvato: memoria %d, coda %d", n, inCoda)
	}
	// le scritte del gioco nella stessa coppia invece si ricordano
	tr.TraduciScritta(context.Background(), "The merchant waits by the old river tonight.")
	tr.mu.Lock()
	n = len(tr.altreCoppie)
	tr.mu.Unlock()
	if n != 1 {
		t.Fatalf("scritta del gioco non ricordata: %d", n)
	}
}

func TestImprontaRicordataSuDisco(t *testing.T) {
	vecchia, vecchiaP := dirApp, Ospite.CartellaProtetta
	dirApp = t.TempDir()
	protetta := t.TempDir() // F20-7: i controlli si ricordano solo nella cartella protetta
	Ospite.CartellaProtetta = func() string { return protetta }
	defer func() { dirApp, Ospite.CartellaProtetta = vecchia, vecchiaP }()
	azzera := func() {
		muVerificati.Lock()
		verificati, verificatiDisco = map[string]bool{}, map[string]bool{}
		muVerificati.Unlock()
		caricaVerDisco = sync.Once{}
	}
	azzera()
	defer azzera()
	dir := t.TempDir()
	p := filepath.Join(dir, "modello.gguf")
	_ = os.WriteFile(p, []byte("modello giusto"), 0o644)
	h := sha256.Sum256([]byte("modello giusto"))
	f := FileDaScaricare{Nome: "modello.gguf", SHA256: hex.EncodeToString(h[:]), Byte: 14}
	data := time.Unix(1700000000, 0)
	_ = os.Chtimes(p, data, data)
	if !presente(dir, f) {
		t.Fatal("file giusto non riconosciuto")
	}
	// Tasso si riapre (memoria vuota) e il file non è cambiato (stessa dimensione e data):
	// si usa l'esito salvato, senza rileggere. Lo provo cambiando il contenuto ma non la data.
	azzera()
	_ = os.WriteFile(p, []byte("modello GIUSTO"), 0o644)
	_ = os.Chtimes(p, data, data)
	if !presente(dir, f) {
		t.Fatal("esito su disco non usato")
	}
	// data cambiata: si ricontrolla davvero
	azzera()
	_ = os.Chtimes(p, data.Add(time.Hour), data.Add(time.Hour))
	if presente(dir, f) {
		t.Fatal("file cambiato accettato senza controllo")
	}
	if _, err := os.Stat(filepath.Join(protetta, "modelli", "impronte-controllate.json")); err != nil {
		t.Fatal("controllo non ricordato nella cartella protetta:", err)
	}
	if _, err := os.Stat(filepath.Join(dirApp, "modelli", "impronte-controllate.json")); err == nil {
		t.Fatal("controllo ricordato in AppData")
	}
}

// CTRL-FIN-20 F20-7: un "controllato" scritto in AppData (da un programma qualsiasi dell'utente) non
// vale, e senza cartella protetta niente si ricorda su disco.
func TestImpronteInAppDataIgnorate(t *testing.T) {
	vecchia, vecchiaP := dirApp, Ospite.CartellaProtetta
	dirApp = t.TempDir()
	defer func() { dirApp, Ospite.CartellaProtetta = vecchia, vecchiaP }()
	azzera := func() {
		muVerificati.Lock()
		verificati, verificatiDisco = map[string]bool{}, map[string]bool{}
		muVerificati.Unlock()
		caricaVerDisco = sync.Once{}
	}
	azzera()
	defer azzera()
	dir := t.TempDir()
	p := filepath.Join(dir, "modello.gguf")
	_ = os.WriteFile(p, []byte("modello CAMBIATO"), 0o644)
	h := sha256.Sum256([]byte("modello giusto!!"))
	f := FileDaScaricare{Nome: "modello.gguf", SHA256: hex.EncodeToString(h[:]), Byte: 16}
	data := time.Unix(1700000000, 0)
	_ = os.Chtimes(p, data, data)
	finto, _ := json.Marshal([]string{chiaveVerifica(p, 16, data.UnixNano(), f.SHA256)})
	_ = os.MkdirAll(filepath.Join(dirApp, "modelli"), 0o755)
	_ = os.WriteFile(filepath.Join(dirApp, "modelli", "impronte-controllate.json"), finto, 0o644)
	for _, prot := range []func() string{nil, func() string { return "" }} {
		Ospite.CartellaProtetta = prot
		azzera()
		if fileVerificati() != "" {
			t.Fatal("senza cartella protetta c'è un file dei controlli")
		}
		if presente(dir, f) {
			t.Fatal("modello cambiato accettato per un controllo scritto in AppData")
		}
	}
}

func TestMemoriaToglieMenoUsate(t *testing.T) {
	tr := NuovoTraduttore("", Glossario{})
	tr.mu.Lock()
	for i := 0; i < maxCache; i++ {
		tr.mettiInCacheLocked(fmt.Sprintf("\x00frase %d", i), "x", true)
	}
	tr.mu.Unlock()
	// la prima frase inserita viene trovata di nuovo: non deve essere tra quelle tolte
	tr.mu.Lock()
	tr.cache["\x00frase 0"] = "Ciao"
	tr.mu.Unlock()
	if r := tr.TraduciScritta(context.Background(), "frase 0"); r.Motore != "memoria" {
		t.Fatalf("frase non trovata in memoria: %+v", r)
	}
	tr.mu.Lock()
	tr.mettiInCacheLocked("\x00una in più", "x", true)
	_, prima := tr.cache["\x00frase 0"]
	_, seconda := tr.cache["\x00frase 1"]
	n := len(tr.cache)
	tr.mu.Unlock()
	if !prima || seconda || n > maxCache {
		t.Fatalf("usata di recente tenuta %v, vecchia mai usata tolta %v, voci %d", prima, !seconda, n)
	}
}

func TestMemoriaFileNuovoEVecchio(t *testing.T) {
	d := t.TempDir()
	vecchio := filepath.Join(d, "memoria-traduzioni.json")
	nuovo := filepath.Join(d, "memoria-traduzioni-2.json")
	_ = os.WriteFile(vecchio, []byte(`{"\u0000Hello":"Ciao"}`), 0o644)
	tr := NuovoTraduttore(nuovo, Glossario{})
	tr.passaDaVecchiaMemoria(vecchio)
	tr.SalvaMemoria()
	if b, _ := os.ReadFile(vecchio); string(b) != `{"\u0000Hello":"Ciao"}` {
		t.Fatalf("file vecchio cambiato: %q", b)
	}
	tr2 := NuovoTraduttore(nuovo, Glossario{})
	if tr2.cache["\x00Hello"] != "Ciao" {
		t.Fatalf("frasi non passate al file nuovo: %v", tr2.cache)
	}
	// file nuovo rovinato: messo da parte, non sovrascritto; le frasi del file vecchio ripartono
	_ = os.WriteFile(nuovo, []byte(`[["rotto`), 0o644)
	tr3 := NuovoTraduttore(nuovo, Glossario{}, &motoreFinto{nome: "Mozilla"})
	tr3.passaDaVecchiaMemoria(vecchio)
	tr3.Traduci(context.Background(), "The bridge is broken.")
	tr3.SalvaMemoria()
	bak, _ := filepath.Glob(filepath.Join(d, "memoria-traduzioni-2-illeggibile-*.json.bak"))
	if len(bak) != 1 {
		t.Fatalf("copia della memoria rovinata: %v", bak)
	}
	if b, _ := os.ReadFile(bak[0]); string(b) != `[["rotto` {
		t.Fatalf("copia cambiata: %q", b)
	}
	if tr4 := NuovoTraduttore(nuovo, Glossario{}); len(tr4.cache) != 2 || tr4.cache["\x00Hello"] != "Ciao" {
		t.Fatalf("memoria nuova: %v", tr4.cache)
	}
}

func TestApriLibreriaNonPerdeLeTraduzioni(t *testing.T) {
	d := t.TempDir()
	f1, f2 := filepath.Join(d, "uno.json"), filepath.Join(d, "due.json")
	tr := NuovoTraduttore("", Glossario{}, &motoreFinto{nome: "Mozilla"})
	tr.ApriLibreria(f1)
	tr.TraduciScritta(context.Background(), "The bridge is broken.")
	// una traduzione arriva nella vecchia mentre la nuova si sta leggendo: simulata mettendola
	// subito prima dello scambio (la vecchia si salva dopo, quindi c'è)
	tr.mu.Lock()
	tr.mettiInLibreriaLocked("\x00Late line.", "Riga tardi.")
	tr.mu.Unlock()
	tr.ApriLibreria(f2)
	if ContaLibreria(f1) != 2 {
		t.Fatalf("libreria vecchia: %d frasi", ContaLibreria(f1))
	}
	tr.mu.Lock()
	n := len(tr.libreria)
	tr.mu.Unlock()
	if n != 0 {
		t.Fatalf("libreria nuova: %d", n)
	}
}

func TestLibreriaPienaNonCresce(t *testing.T) {
	tr := NuovoTraduttore("", Glossario{}, &motoreFinto{nome: "Mozilla"})
	tr.ApriLibreria(filepath.Join(t.TempDir(), "g.json"))
	tr.mu.Lock()
	tr.libByte = maxLibreriaCresce
	tr.mu.Unlock()
	tr.TraduciScritta(context.Background(), "The bridge is broken.")
	tr.mu.Lock()
	nl, nc := len(tr.libreria), len(tr.cache)
	tr.mu.Unlock()
	if nl != 0 || nc != 1 {
		t.Fatalf("libreria piena: libreria %d, memoria %d", nl, nc)
	}
}

func TestImpronteProgrammiSempreControllate(t *testing.T) {
	vecchia := dirApp
	dirApp = t.TempDir()
	defer func() { dirApp = vecchia }()
	azzera := func() {
		muVerificati.Lock()
		verificati, verificatiDisco = map[string]bool{}, map[string]bool{}
		muVerificati.Unlock()
		caricaVerDisco = sync.Once{}
	}
	azzera()
	defer azzera()
	dir := t.TempDir()
	p := filepath.Join(dir, "llama-server.exe")
	_ = os.WriteFile(p, []byte("programma giusto"), 0o644)
	h := sha256.Sum256([]byte("programma giusto"))
	f := FileDaScaricare{Nome: "llama-server.exe", SHA256: hex.EncodeToString(h[:])}
	data := time.Unix(1700000000, 0)
	_ = os.Chtimes(p, data, data)
	if !presente(dir, f) {
		t.Fatal("programma giusto non riconosciuto")
	}
	// cambiato con la stessa dimensione e la stessa data, nella stessa sessione: si vede subito
	_ = os.WriteFile(p, []byte("programma GIUSTO"), 0o644)
	_ = os.Chtimes(p, data, data)
	if presente(dir, f) {
		t.Fatal("programma cambiato accettato per la memoria")
	}
	// un esito "giusto" scritto su disco (anche da un altro programma) non vale per .exe e .dll
	azzera()
	chiave := chiaveVerifica(p, 16, data.UnixNano(), f.SHA256)
	b, _ := json.Marshal([]string{chiave})
	_ = os.MkdirAll(filepath.Join(dirApp, "modelli"), 0o755)
	_ = os.WriteFile(fileVerificati(), b, 0o644)
	if presente(dir, f) {
		t.Fatal("esito su disco usato per un programma")
	}
}
