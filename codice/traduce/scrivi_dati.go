package traduce

// Scritture sicure in %LOCALAPPDATA%\TassoTraduce (RECENSIONE-3, R3-1), come F20-6 per
// %LOCALAPPDATA%\Tasso (sicuro_scrittura.go di Tasso).
//
// Dentro Tasso il traduttore gira come amministratore, ma la sua cartella dati (dirApp) è
// dell'utente: un programma normale può trasformare dirApp, logs, modelli\ia o lavoro in una
// giunzione (anche verso "\RPC Control") e far scrivere, rinominare o cancellare a Tasso un file di
// Windows. Prima di scrivere controllo con Lstat dirApp e ogni pezzo del percorso da dirApp in giù:
// se uno è un collegamento simbolico o una giunzione (ModeSymlink, ModeIrregular), non scrivo.
// I file nuovi si scrivono con un nome a caso creato con O_EXCL (un collegamento già pronto con
// quel nome non si segue) e poi si rinominano al loro posto: il rinomino sostituisce un
// collegamento invece di seguirlo.
//
// Fuori da dirApp (Programmi\Tasso, Documenti, cartelle dei giochi) queste funzioni non
// controllano niente: lì valgono le regole di chi chiama. Limite, come in Tasso: resta una
// piccola corsa tra il controllo e l'apertura; i collegamenti fisici non si vedono con Lstat.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// errDatiLink: il percorso dentro TassoTraduce passa da un collegamento o da una giunzione.
var errDatiLink = errors.New("la cartella dei dati di Tasso Traduce passa da un collegamento o da una giunzione: per sicurezza non ci scrivo")

// msgDatiLink: cosa dire all'utente quando uno scaricamento si ferma per errDatiLink (sezione "go").
const msgDatiLink = "la cartella dei dati di Tasso Traduce (AppData\\Local\\TassoTraduce) porta a un'altra cartella (collegamento o giunzione): per sicurezza Tasso non ci scarica niente. Rimetti una cartella TassoTraduce normale e riprova."

// sottoDati: i pezzi di p sotto dirApp. ok=false se dirApp non c'è o p non è sotto dirApp.
func sottoDati(p string) (pezzi []string, ok bool) {
	if dirApp == "" {
		return nil, false
	}
	base := filepath.Clean(dirApp)
	rel, err := filepath.Rel(base, filepath.Clean(p))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, false
	}
	if rel == "." {
		return nil, true
	}
	return strings.Split(rel, string(filepath.Separator)), true
}

// percorsoDatiSicuro: nil se p non è sotto dirApp, oppure se né dirApp né un pezzo tra dirApp e p
// (p compreso, se c'è) è un collegamento o una giunzione. I pezzi che non ci sono ancora vanno
// bene: li crea Tasso.
func percorsoDatiSicuro(p string) error {
	pezzi, ok := sottoDati(p)
	if !ok {
		return nil
	}
	cur := filepath.Clean(dirApp)
	if fi, err := os.Lstat(cur); err == nil && fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return errDatiLink
	}
	for _, s := range pezzi {
		cur = filepath.Join(cur, s)
		fi, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return errDatiLink
		}
	}
	return nil
}

// cartellaDatiSicura: come percorsoDatiSicuro per la cartella di p.
func cartellaDatiSicura(p string) error { return percorsoDatiSicuro(filepath.Dir(p)) }

// creaCartellaDati: os.MkdirAll, solo se il percorso non passa da collegamenti o giunzioni (prima e
// dopo averlo creato).
func creaCartellaDati(d string) error {
	if err := percorsoDatiSicuro(d); err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	return percorsoDatiSicuro(d)
}

// creaFileDati: un file nuovo, con un nome a caso accanto a p (O_EXCL), da rinominare al suo posto
// con rinominaDati quando è finito. La cartella deve essere sicura.
func creaFileDati(p string) (*os.File, error) {
	if err := cartellaDatiSicura(p); err != nil {
		return nil, err
	}
	return os.CreateTemp(filepath.Dir(p), filepath.Base(p)+".*.tmp")
}

// scriviDati: come os.WriteFile sotto dirApp, ma senza seguire collegamenti o giunzioni: scrive un
// file nuovo con un nome a caso e lo rinomina al posto di p.
func scriviDati(p string, b []byte) error {
	f, err := creaFileDati(p)
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = rinominaDati(tmp, p)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

// rinominaDati: os.Rename, solo se le cartelle di partenza e di arrivo sono sicure.
func rinominaDati(da, a string) error {
	if err := cartellaDatiSicura(da); err != nil {
		return err
	}
	if err := cartellaDatiSicura(a); err != nil {
		return err
	}
	return os.Rename(da, a)
}

// rimuoviDati: os.Remove, solo se la cartella è sicura (un collegamento si toglie lui, non il file a
// cui porta).
func rimuoviDati(p string) error {
	if err := cartellaDatiSicura(p); err != nil {
		return err
	}
	return os.Remove(p)
}

// apriLogDati: apre in aggiunta (o crea) un file di registro sotto dirApp. Se il file stesso è un
// collegamento o una giunzione lo tolgo (si toglie il collegamento, non il file a cui porta) e ne
// creo uno vero; se lo è una cartella del percorso non scrivo.
func apriLogDati(p string) (*os.File, error) {
	if err := cartellaDatiSicura(p); err != nil {
		return nil, err
	}
	if _, ok := sottoDati(p); ok {
		if fi, err := os.Lstat(p); err == nil && fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			if os.Remove(p) != nil {
				return nil, errDatiLink
			}
		}
	}
	return os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
}

// creaLogDati: come os.Create per un registro sotto dirApp (lo riscrive da zero), senza seguire
// collegamenti: il collegamento con quel nome si toglie prima.
func creaLogDati(p string) (*os.File, error) {
	if err := cartellaDatiSicura(p); err != nil {
		return nil, err
	}
	if _, ok := sottoDati(p); ok {
		if fi, err := os.Lstat(p); err == nil && fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			if os.Remove(p) != nil {
				return nil, errDatiLink
			}
		}
	}
	return os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
}

// ruotaLogDati: il registro p diventa p.old se è più grande di max (solo con la cartella sicura).
func ruotaLogDati(p string, max int64) {
	if fi, err := os.Lstat(p); err == nil && fi.Mode().IsRegular() && fi.Size() > max {
		_ = rinominaDati(p, p+".old")
	}
}
