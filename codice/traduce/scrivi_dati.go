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
// controllano niente: lì valgono le regole di chi chiama. I collegamenti fisici non si vedono con Lstat.
//
// RECENSIONE-3b, R3b-4: dopo il controllo, la scrittura vera (creazione, apertura, rinomino,
// cancellazione, cartelle) passa da os.Root aperto su dirApp: lavora a maniglie dentro dirApp e non
// segue collegamenti o giunzioni che portano fuori (le giunzioni hanno sempre un percorso assoluto,
// quindi os.Root non le segue mai). Così non c'è più la corsa tra il controllo e la scrittura, per
// esempio alla fine di uno scaricamento lungo. dirApp lo apro dalla cartella sopra (AppData\Local,
// controllata da Tasso all'avvio, R3b-3): anche dirApp stesso non può essere una giunzione.

import (
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
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

// radiceDati: dirApp aperta come os.Root, partendo dalla cartella sopra (così una giunzione al posto
// di dirApp non si segue). crea=true la crea se non c'è ancora. Va chiusa da chi la chiede.
func radiceDati(crea bool) (*os.Root, error) {
	base := filepath.Clean(dirApp)
	su, err := os.OpenRoot(filepath.Dir(base))
	if err != nil {
		return nil, err
	}
	defer su.Close()
	r, err := su.OpenRoot(filepath.Base(base))
	if err != nil && crea && errors.Is(err, os.ErrNotExist) {
		if err = su.Mkdir(filepath.Base(base), 0o755); err == nil || errors.Is(err, os.ErrExist) {
			r, err = su.OpenRoot(filepath.Base(base))
		}
	}
	return r, err
}

// relDati: p come nome relativo a dirApp, per os.Root ("." per dirApp). ok=false se p non è sotto dirApp.
func relDati(p string) (string, bool) {
	pezzi, ok := sottoDati(p)
	if !ok {
		return "", false
	}
	if len(pezzi) == 0 {
		return ".", true
	}
	return filepath.Join(pezzi...), true
}

// apriInDati: apre o crea p dentro dirApp con os.Root (vedi sopra).
func apriInDati(rel string, flag int, perm os.FileMode) (*os.File, error) {
	r, err := radiceDati(flag&os.O_CREATE != 0)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return r.OpenFile(rel, flag, perm)
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
	if rel, ok := relDati(d); ok {
		r, err := radiceDati(true)
		if err != nil {
			return err
		}
		err = r.MkdirAll(rel, 0o755)
		r.Close()
		if err != nil {
			return err
		}
	} else if err := os.MkdirAll(d, 0o755); err != nil {
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
	rel, ok := relDati(p)
	if !ok {
		return os.CreateTemp(filepath.Dir(p), filepath.Base(p)+".*.tmp")
	}
	r, err := radiceDati(false)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	// Come os.CreateTemp, ma dentro la radice: nome a caso e O_EXCL (un nome già preso non si apre).
	for i := 0; ; i++ {
		f, err := r.OpenFile(rel+"."+strconv.FormatUint(rand.Uint64N(1e10), 10)+".tmp", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil || !errors.Is(err, os.ErrExist) || i >= 1000 {
			return f, err
		}
	}
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
	rda, ok1 := relDati(da)
	ra, ok2 := relDati(a)
	if !ok1 || !ok2 {
		return os.Rename(da, a)
	}
	r, err := radiceDati(false)
	if err != nil {
		return err
	}
	defer r.Close()
	return r.Rename(rda, ra)
}

// rimuoviDati: os.Remove, solo se la cartella è sicura (un collegamento si toglie lui, non il file a
// cui porta).
func rimuoviDati(p string) error {
	if err := cartellaDatiSicura(p); err != nil {
		return err
	}
	rel, ok := relDati(p)
	if !ok {
		return os.Remove(p)
	}
	r, err := radiceDati(false)
	if err != nil {
		return err
	}
	defer r.Close()
	return r.Remove(rel)
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
	if rel, ok := relDati(p); ok {
		return apriInDati(rel, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
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
	if rel, ok := relDati(p); ok {
		return apriInDati(rel, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	}
	return os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
}

// ruotaLogDati: il registro p diventa p.old se è più grande di max (solo con la cartella sicura).
func ruotaLogDati(p string, max int64) {
	if fi, err := os.Lstat(p); err == nil && fi.Mode().IsRegular() && fi.Size() > max {
		_ = rinominaDati(p, p+".old")
	}
}
