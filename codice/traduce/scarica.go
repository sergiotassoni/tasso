package traduce

// Scaricamento dei file grandi (traduttore Mozilla, motore IA) SOLO dopo il sì dell'utente,
// con controllo SHA-256: un file che non corrisponde viene cancellato e mai usato.

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// FileDaScaricare descrive un file con la sua impronta SHA-256 (del file finale, già decompresso).
type FileDaScaricare struct {
	URL    string `json:"url"`
	Nome   string `json:"nome"`   // nome del file sul disco
	SHA256 string `json:"sha256"` // obbligatoria: senza impronta il file non si scarica
	Byte   int64  `json:"byte"`   // dimensione finale, per la barra
	Gzip   bool   `json:"gzip"`   // il server lo manda compresso con gzip
}

var clientScarica = &http.Client{Timeout: 6 * time.Hour, CheckRedirect: func(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme != "https" || len(via) > 10 {
		return errors.New("scaricamento: rimando non sicuro rifiutato")
	}
	return nil
}}

var errImprontaMancante = errors.New("impronta di sicurezza mancante: file non scaricato")

// presente dice se il file c'è già ed è quello giusto (controllo dell'impronta SHA-256).
//
// Programmi e librerie (.exe, .dll): l'impronta si calcola davvero OGNI volta, senza ricordare
// niente. Tasso li avvia da amministratore e la cartella dei modelli la può cambiare qualsiasi
// programma dell'utente: anche dimensione e data si possono rimettere uguali, quindi ricordare
// "era giusto" permetterebbe di saltare il controllo con un file cambiato.
//
// Modelli .gguf (anche 10 GB): il controllo riuscito si ricorda, anche su disco, finché il file
// non cambia percorso, dimensione o data, così all'avvio non si rileggono tutti.
// Gli altri file (dati del traduttore Mozilla, zip) si ricordano solo finché Tasso resta aperto.
func presente(dir string, f FileDaScaricare) bool {
	p := filepath.Join(dir, f.Nome)
	st, err := os.Stat(p)
	if err != nil || !st.Mode().IsRegular() || (f.Byte > 0 && st.Size() != f.Byte) {
		return false
	}
	if eseguibile(f.Nome) {
		h, err := impronta(p)
		return err == nil && strings.EqualFold(h, f.SHA256)
	}
	chiave := chiaveVerifica(p, st.Size(), st.ModTime().UnixNano(), f.SHA256)
	suDisco := ricordaSuDisco(p)
	if suDisco {
		caricaVerificati()
	}
	muVerificati.Lock()
	ok, visto := verificati[chiave]
	if !visto && suDisco && verificatiDisco[chiave] {
		ok, visto = true, true
		verificati[chiave] = true
	}
	muVerificati.Unlock()
	if visto {
		return ok
	}
	h, err := impronta(p)
	ok = err == nil && strings.EqualFold(h, f.SHA256)
	muVerificati.Lock()
	verificati[chiave] = ok
	muVerificati.Unlock()
	if ok && suDisco {
		ricordaVerificato(p, chiave)
	}
	return ok
}

// eseguibile: programmi e librerie, che si controllano sempre per intero.
func eseguibile(nome string) bool {
	b := strings.ToLower(nome)
	return strings.HasSuffix(b, ".exe") || strings.HasSuffix(b, ".dll")
}

// ricordaSuDisco: solo i modelli .gguf (grandi, e non sono programmi) si ricordano su disco.
func ricordaSuDisco(p string) bool {
	return strings.HasSuffix(strings.ToLower(p), ".gguf")
}

var (
	muVerificati     sync.Mutex
	verificati       = map[string]bool{}
	verificatiDisco  = map[string]bool{} // controlli riusciti nelle volte prima (dal file)
	caricaVerDisco   sync.Once
	muFileVerificati sync.Mutex
)

// chiaveVerifica: percorso, dimensione, data di modifica e impronta attesa del file.
func chiaveVerifica(p string, dim, data int64, sha string) string {
	return fmt.Sprintf("%s|%d|%d|%s", strings.ToLower(p), dim, data, strings.ToLower(sha))
}

// fileVerificati: dove si ricordano i controlli riusciti dei modelli. CTRL-FIN-20 F20-7: mai in
// AppData (la cartella dei modelli la può scrivere qualsiasi programma dell'utente, che potrebbe
// aggiungere un "controllato" falso per un modello cambiato), solo nella cartella protetta di Tasso
// (Programmi\Tasso\lavoro, dove scrivono solo gli amministratori). Senza cartella protetta ("" =
// Tasso usa il ripiego in AppData, o Tasso Traduce da solo) i controlli valgono solo finché il
// programma resta aperto.
func fileVerificati() string {
	if Ospite.CartellaProtetta == nil {
		return ""
	}
	d := Ospite.CartellaProtetta()
	if d == "" || !filepath.IsAbs(d) {
		return ""
	}
	return filepath.Join(d, "modelli", "impronte-controllate.json")
}

func caricaVerificati() {
	caricaVerDisco.Do(func() {
		f := fileVerificati()
		if f == "" {
			return
		}
		b, err := os.ReadFile(f)
		if err != nil || len(b) > 1<<20 {
			return
		}
		var l []string
		if json.Unmarshal(b, &l) != nil {
			return
		}
		muVerificati.Lock()
		for _, k := range l {
			// le voci di .exe e .dll scritte dalle versioni di prima non valgono più
			if p, _, ok := strings.Cut(k, "|"); ok && ricordaSuDisco(p) {
				verificatiDisco[k] = true
			}
		}
		muVerificati.Unlock()
	})
}

// ricordaVerificato: scrive su disco che il file p (con questa chiave) è stato controllato ed è
// giusto. Le chiavi vecchie dello stesso file (dimensione o data diverse) si tolgono.
func ricordaVerificato(p, chiave string) {
	f := fileVerificati()
	if f == "" {
		return
	}
	muFileVerificati.Lock()
	defer muFileVerificati.Unlock()
	pre := strings.ToLower(p) + "|"
	muVerificati.Lock()
	for k := range verificatiDisco {
		if strings.HasPrefix(k, pre) && k != chiave && strings.Count(k[len(pre):], "|") == 2 {
			delete(verificatiDisco, k)
		}
	}
	verificatiDisco[chiave] = true
	l := make([]string, 0, len(verificatiDisco))
	for k := range verificatiDisco {
		l = append(l, k)
	}
	muVerificati.Unlock()
	sort.Strings(l)
	b, err := json.Marshal(l)
	if err != nil {
		return
	}
	if creaCartellaDati(filepath.Dir(f)) == nil {
		_ = scriviDati(f, b) // R3-1: niente collegamenti o giunzioni sotto TassoTraduce
	}
}

func impronta(p string) (string, error) {
	fh, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer fh.Close()
	h := sha256.New()
	if _, err := io.Copy(h, fh); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// msgErroreScarica: cosa dire all'utente di uno scaricamento non riuscito, senza nomi di file,
// indirizzi né errori tecnici (quelli li scrive il registro, logScarica).
func msgErroreScarica(err error) string {
	var pe *os.PathError
	var le *os.LinkError
	switch {
	case errors.Is(err, errDatiLink):
		return msgDatiLink
	case errors.Is(err, errScaricaRete):
		return "connessione non riuscita: controlla internet e riprova."
	case errors.Is(err, errScaricaSicurezza):
		return "il file scaricato non è quello giusto e l'ho cancellato: riprova più tardi."
	case errors.As(err, &pe), errors.As(err, &le):
		return "non riesco a salvare il file sul disco: controlla lo spazio libero e riprova."
	}
	return err.Error()
}

// scaricaTutti scarica i file mancanti. avanzamento(fatti, totale) viene chiamata spesso.
func scaricaTutti(ctx context.Context, dir string, files []FileDaScaricare, avanzamento func(int64, int64)) error {
	if err := creaCartellaDati(dir); err != nil {
		return err
	}
	var totale, fatti int64
	for _, f := range files {
		totale += f.Byte
	}
	for _, f := range files {
		if f.SHA256 == "" {
			return fmt.Errorf("%s: %w", f.Nome, errImprontaMancante)
		}
		if presente(dir, f) {
			fatti += f.Byte
			avanzamento(fatti, totale)
			continue
		}
		base := fatti
		err := scaricaUno(ctx, dir, f, func(n int64) { avanzamento(base+n, totale) })
		if err != nil {
			return err
		}
		fatti += f.Byte
		avanzamento(fatti, totale)
	}
	return nil
}

// Per dire all'utente cosa è successo senza l'errore tecnico (che resta nel registro): la rete o il
// controllo dell'impronta. Restano dentro l'errore con %w (errors.Is).
var (
	errScaricaRete      = errors.New("connessione non riuscita")
	errScaricaSicurezza = errors.New("controllo di sicurezza fallito, il file non è quello atteso (cancellato)")
)

func scaricaUno(ctx context.Context, dir string, f FileDaScaricare, avanzamento func(int64)) error {
	if strings.ContainsAny(f.Nome, `/\:`) || f.Nome == "" || f.Nome == "." || f.Nome == ".." {
		return fmt.Errorf("nome file non valido: %q", f.Nome)
	}
	if !strings.HasPrefix(f.URL, "https://") {
		return fmt.Errorf("%s: solo indirizzi https", f.Nome)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", f.URL, nil)
	if err != nil {
		return err
	}
	// Gestisco io la compressione, così il conteggio dei byte è quello vero.
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := clientScarica.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w: %w", f.Nome, errScaricaRete, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s: %w: il server ha risposto %d", f.Nome, errScaricaRete, resp.StatusCode)
	}
	var sorgente io.Reader = resp.Body
	if f.Gzip || resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("%s: %w", f.Nome, err)
		}
		defer gz.Close()
		sorgente = gz
	}
	// R3-1: un file nuovo con un nome a caso (O_EXCL), solo se la cartella dei modelli non passa
	// da collegamenti o giunzioni: un .parziale preparato da un altro programma non si segue.
	// Prima tolgo i pezzi rimasti da uno scaricamento interrotto (Tasso chiuso a metà).
	if vecchi, _ := filepath.Glob(filepath.Join(dir, f.Nome+".parziale*")); len(vecchi) > 0 {
		for _, v := range vecchi {
			_ = rimuoviDati(v)
		}
	}
	out, err := creaFileDati(filepath.Join(dir, f.Nome+".parziale"))
	if err != nil {
		return err
	}
	tmp := out.Name()
	h := sha256.New()
	cont := &contatore{fn: avanzamento}
	limite := f.Byte + f.Byte/10 + 1<<20 // un file molto più grande del previsto non è quello giusto
	_, err = io.Copy(io.MultiWriter(out, h, cont), io.LimitReader(sorgente, limite))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		var pe *os.PathError
		if errors.As(err, &pe) { // scrittura sul disco
			return fmt.Errorf("%s: %w", f.Nome, err)
		}
		return fmt.Errorf("%s: %w: %w", f.Nome, errScaricaRete, err) // la connessione si è interrotta
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, f.SHA256) {
		os.Remove(tmp)
		return fmt.Errorf("%s: %w", f.Nome, errScaricaSicurezza)
	}
	if err := rinominaDati(tmp, filepath.Join(dir, f.Nome)); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

type contatore struct {
	n  int64
	fn func(int64)
	t  time.Time
}

func (c *contatore) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	if time.Since(c.t) > 200*time.Millisecond {
		c.t = time.Now()
		c.fn(c.n)
	}
	return len(p), nil
}

// ---------------------------------------------------------------- cosa si scarica

// Traduttore Mozilla inglese→italiano (licenza MPL-2.0), dal deposito ufficiale dei modelli
// Bergamot. Impronte controllate il 7/11/2026 con l'indice ufficiale
// https://bergamot.s3.amazonaws.com/models/index.json e ricalcolate sui file scaricati.
// Modello inglese→italiano 2.1 di Mozilla (quello di Firefox, "base", MPL-2.0), impronte dal
// registro ufficiale di Firefox (firefox.settings.services.mozilla.com, 22/11/2026). Rispetto al
// modello 1.0 di Bergamot: 1000 frasi da 69,1 a 70,3, con il correttore IA da 70,7 a 72,0.
var fileMozillaENIT = []FileDaScaricare{
	{URL: "https://firefox-settings-attachments.cdn.mozilla.net/main-workspace/translations-models/29ae0d70-5e37-49e8-8da6-3e2c58da61d1.bin", Nome: "model.enit.intgemm.alphas.bin",
		SHA256: "248f47568788ecc351da7e5e07064d4153b4f71e011364ae2c931ffeec4d1cc2", Byte: 31561787},
	{URL: "https://firefox-settings-attachments.cdn.mozilla.net/main-workspace/translations-models/3730ff60-2291-4c87-888d-2dc4593b20db.bin", Nome: "lex.50.50.enit.s2t.bin",
		SHA256: "8b21914804625b2777dae7fdb636eb78f02a3eb8b7bceaa50f29ac740961da93", Byte: 4133192},
	{URL: "https://firefox-settings-attachments.cdn.mozilla.net/main-workspace/translations-models/d9d5ffa9-b919-4491-9ab6-3dec34459768.spm", Nome: "vocab.enit.spm",
		SHA256: "3ef0211d4ae6db21440892f180f2019fe2bfc110a330ffa9d2eca9665e4f2bc5", Byte: 812724},
}
