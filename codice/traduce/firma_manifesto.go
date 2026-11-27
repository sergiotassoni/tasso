package traduce

// Firma degli elenchi "manifesto.json" delle traduzioni dentro i giochi.
// Gli elenchi stanno in AppData (lì può scrivere qualsiasi programma dell'utente, anche senza
// privilegi), ma Tasso li usa da amministratore per togliere file e rimettere gli originali nella
// cartella del gioco. Senza firma un programma qualsiasi potrebbe scrivere un elenco finto e far
// copiare a Tasso un suo file in una cartella protetta. La chiave della firma sta in
// Programmi\Tasso, dove scrivono solo gli amministratori.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// CartellaSicura: Programmi\Tasso (solo amministratori). La imposta il programma principale.
var CartellaSicura string

var (
	muChiave  sync.Mutex
	chiaveMem []byte
)

// proteggiChiave: su Windows la chiave la possono leggere solo gli amministratori e SYSTEM
// (in Programmi i file ereditano "lettura per tutti": con la chiave chiunque firmerebbe un
// elenco finto). Lo imposta la parte Windows.
var proteggiChiave func(p string) error

// chiaveFirma: la chiave della firma (la crea la prima volta).
func chiaveFirma() (k []byte, nuova bool, err error) {
	muChiave.Lock()
	defer muChiave.Unlock()
	if chiaveMem != nil {
		return chiaveMem, false, nil
	}
	if CartellaSicura == "" {
		return nil, false, errors.New("manca la cartella sicura")
	}
	p := filepath.Join(CartellaSicura, "chiave-traduzioni")
	b, err := os.ReadFile(p)
	if err == nil && len(b) == 32 {
		if proteggiChiave != nil && proteggiChiave(p) != nil {
			return nil, false, errors.New("non riesco a proteggere la chiave")
		}
		chiaveMem = b
		return b, false, nil
	}
	if err == nil {
		// chiave rovinata (per esempio corrente saltata mentre la scrivevo): in Programmi\Tasso
		// scrivono solo gli amministratori, la rifaccio
		_ = os.Remove(p)
	}
	b = make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return nil, false, err
	}
	_ = os.MkdirAll(CartellaSicura, 0o755)
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		// un'altra copia di Tasso l'ha appena creata: uso la sua
		if b2, e := os.ReadFile(p); e == nil && len(b2) == 32 {
			chiaveMem = b2
			return b2, false, nil
		}
		return nil, false, err
	}
	// protezione prima di scriverci la chiave (il file è ancora vuoto)
	if proteggiChiave != nil {
		if err := proteggiChiave(p); err != nil {
			f.Close()
			_ = os.Remove(p)
			return nil, false, err
		}
	}
	_, err = f.Write(b)
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		_ = os.Remove(p)
		return nil, false, err
	}
	chiaveMem = b
	return b, true, nil
}

func firmaDi(k []byte, m ManifestoMod) string {
	m.Firma = ""
	b, _ := json.Marshal(m)
	h := hmac.New(sha256.New, k)
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

func improntaByte(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func improntaFile(p string) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// firmaManifesto: aggiunge le impronte degli originali messi da parte e la firma.
func firmaManifesto(dirApp, chiave string, m *ManifestoMod) error {
	k, _, err := chiaveFirma()
	if err != nil {
		return errors.New("non riesco a preparare la firma dell'elenco dei file (cartella Tasso in Programmi)")
	}
	m.Impronte = map[string]string{}
	for _, s := range m.Sostituiti {
		if i := improntaFile(filepath.Join(cartellaCopie(dirApp, chiave), "originali", s)); i != "" {
			m.Impronte[s] = i
		}
	}
	m.Firma = firmaDi(k, *m)
	return nil
}

// manifestoFirmato: l'elenco è stato scritto da Tasso e non è stato cambiato dopo.
// (Niente "firma degli elenchi vecchi": la 3.0 non scriveva elenchi, e firmare quello che si
// trova sul disco vorrebbe dire firmare anche un elenco finto messo lì da un altro programma.)
func manifestoFirmato(dirApp string, m ManifestoMod) bool {
	k, _, err := chiaveFirma()
	if err != nil {
		return false
	}
	return m.Firma != "" && hmac.Equal([]byte(m.Firma), []byte(firmaDi(k, m)))
}

// senzaCollegamenti: dalla cartella del gioco fino al file, nessuna cartella è un collegamento
// o una giunzione (che porterebbe la scrittura in un altro posto).
func senzaCollegamenti(base, rel string) bool {
	if st, err := os.Lstat(base); err != nil || st.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 || !st.IsDir() {
		return false
	}
	p := base
	parti := strings.Split(filepath.Clean(rel), string(filepath.Separator))
	for i, x := range parti {
		if x == "" || x == "." || x == ".." {
			return false
		}
		p = filepath.Join(p, x)
		st, err := os.Lstat(p)
		if err != nil {
			return os.IsNotExist(err) // il resto non c'è ancora: lo creo io
		}
		if st.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return false
		}
		if i < len(parti)-1 && !st.IsDir() {
			return false
		}
	}
	return true
}
