package traduce

// Copie di sicurezza delle traduzioni nei giochi (manifesto.json e originali dei file sostituiti):
// Tasso le usa da amministratore per rimettere a posto i giochi. In %LOCALAPPDATA% le può cambiare
// qualunque programma dell'utente (per esempio far dire al manifesto "rimetti questo file al posto
// di quello"). Dalla 3.2 stanno nella cartella protetta di Tasso (Programmi\Tasso\lavoro), dove
// solo gli amministratori scrivono. Le copie vecchie si spostano lì da sole, una volta sola.

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// segnoCopieSpostate: c'è dentro la cartella protetta dopo lo spostamento delle copie vecchie.
// Senza questo segno, ogni avvio riprenderebbe da %LOCALAPPDATA% anche quello che ci mette dopo un
// altro programma.
const segnoCopieSpostate = ".spostate-da-appdata"

var (
	copieMu       sync.Mutex
	copieProtetta string // "" = ancora da decidere
	copieDeciso   bool
)

// baseCopie: la cartella che contiene "copie-giochi". Dentro Tasso è quella protetta; Tasso
// Traduce da solo (o se la cartella protetta non c'è) usa dirApp, come prima.
func baseCopie() string {
	copieMu.Lock()
	defer copieMu.Unlock()
	if !copieDeciso && dirApp != "" {
		copieDeciso = true
		if Ospite.CartellaProtetta != nil {
			if d := Ospite.CartellaProtetta(); d != "" && os.MkdirAll(filepath.Join(d, "copie-giochi"), 0o755) == nil {
				if err := spostaCopieVecchie(dirApp, d); err != nil {
					scriviLogCopie("copie dei giochi non spostate tutte (riprovo al prossimo avvio): " + err.Error())
				}
				copieProtetta = d
			}
		}
	}
	if copieProtetta != "" {
		return copieProtetta
	}
	return dirApp
}

// CartellaCopie: la cartella "copie-giochi" in uso (per la disinstallazione).
func CartellaCopie() string { return filepath.Join(baseCopie(), "copie-giochi") }

// CartellaCopieVecchia: dove stavano le copie fino alla 3.1 (%LOCALAPPDATA%\TassoTraduce\copie-giochi).
func CartellaCopieVecchia() string {
	if dirApp == "" {
		return ""
	}
	return filepath.Join(dirApp, "copie-giochi")
}

func scriviLogCopie(s string) {
	if dirApp == "" {
		return
	}
	f, err := os.OpenFile(filepath.Join(dirApp, "logs", "copie-giochi.txt"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(s + "\r\n")
}

// spostaCopieVecchie porta vecchia\copie-giochi\<gioco> in nuova\copie-giochi\<gioco>. Copio
// (non sposto): un file spostato sullo stesso disco si porta dietro i permessi di prima, cioè
// resterebbe modificabile dall'utente. Link e giunzioni non li seguo (copiaCartella li rifiuta).
// Un gioco che c'è già nella cartella nuova non lo tocco. Se tutto va bene tolgo la cartella
// vecchia e metto il segno; se qualcosa non va, la vecchia resta e riprovo al prossimo avvio.
func spostaCopieVecchie(vecchia, nuova string) error {
	segno := filepath.Join(nuova, "copie-giochi", segnoCopieSpostate)
	if _, err := os.Lstat(segno); err == nil {
		return nil
	}
	fatto := func() error { return os.WriteFile(segno, []byte("ok\r\n"), 0o644) }
	if filepath.Clean(vecchia) == filepath.Clean(nuova) {
		return fatto()
	}
	old := filepath.Join(vecchia, "copie-giochi")
	fi, err := os.Lstat(old)
	if os.IsNotExist(err) {
		return fatto()
	}
	if err != nil {
		return err
	}
	if !fi.IsDir() || fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return fatto() // non è una cartella vera: non la seguo e non ci torno più
	}
	voci, err := os.ReadDir(old)
	if err != nil {
		return err
	}
	var errs []error
	for _, v := range voci {
		if !v.IsDir() || v.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			continue
		}
		dest := filepath.Join(nuova, "copie-giochi", v.Name())
		if _, err := os.Lstat(dest); err == nil {
			continue
		}
		if err := copiaCartella(filepath.Join(old, v.Name()), dest); err != nil {
			_ = os.RemoveAll(dest) // niente copie a metà
			errs = append(errs, errors.New(v.Name()+": "+err.Error()))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	if err := os.RemoveAll(old); err != nil {
		scriviLogCopie("cartella vecchia delle copie non tolta: " + err.Error())
	}
	return fatto()
}

// leggiSenzaLink: il file base\rel, solo se né lui né le cartelle in mezzo sono collegamenti o
// giunzioni (Tasso lo legge da amministratore dalla cartella dell'utente) e se il file aperto è
// proprio quello controllato.
func leggiSenzaLink(base, rel string) ([]byte, error) {
	if !senzaLink(base, rel) {
		return nil, errors.New("collegamento o giunzione: non leggo")
	}
	p := filepath.Join(base, rel)
	fi, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, errors.New("non è un file normale: non leggo")
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi2, err := f.Stat()
	if err != nil || !os.SameFile(fi, fi2) {
		return nil, errors.New("il file è cambiato mentre lo aprivo: non leggo")
	}
	return io.ReadAll(f)
}
