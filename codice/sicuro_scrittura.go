package main

// Scritture sicure in %LOCALAPPDATA%\Tasso.
//
// Tasso gira come amministratore ma la sua cartella dati (appDir) è dell'utente: un programma
// normale può trasformare appDir, logs o un file (boost.json, crash.txt...) in una giunzione o in
// un collegamento simbolico verso un file di Windows, e far scrivere Tasso là dentro.
// Prima di scrivere controllo con Lstat ogni pezzo del percorso da appDir in giù: se uno è un
// collegamento simbolico (ModeSymlink) o una giunzione/altro punto di rianalisi (ModeIrregular,
// così le giunzioni risultano da Go 1.23), non scrivo.
//
// Limiti (scritti nel rapporto O3-sicdati): resta una piccola corsa tra il controllo e l'apertura
// (TOCTOU); i collegamenti fisici (hard link) non si vedono con Lstat.

import (
	"errors"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var errPercorsoLink = errors.New("percorso con collegamento o giunzione: non scrivo")

// èLink: true se il file o la cartella è un collegamento simbolico o una giunzione.
func èLink(fi os.FileInfo) bool {
	return fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0
}

// sottoApp restituisce i pezzi di p sotto base (base escluso). ok=false se p non è sotto base.
func sottoApp(base, p string) ([]string, bool) {
	base = filepath.Clean(base)
	p = filepath.Clean(p)
	rel, err := filepath.Rel(base, p)
	if err == nil && rel == "." {
		return nil, true // p è proprio base
	}
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, false
	}
	return strings.Split(rel, string(filepath.Separator)), true
}

// controllaPercorsoIn: errPercorsoLink se base o una cartella tra base e p (p compreso) è un
// collegamento o una giunzione. I pezzi che non esistono ancora vanno bene (li crea Tasso).
// I percorsi fuori da base non li controllo (non sono della cartella dati).
func controllaPercorsoIn(base, p string) error {
	pezzi, ok := sottoApp(base, p)
	if !ok {
		return nil
	}
	cur := filepath.Clean(base)
	if fi, err := os.Lstat(cur); err == nil && èLink(fi) {
		return errPercorsoLink
	}
	for _, s := range pezzi {
		cur = filepath.Join(cur, s)
		fi, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				return nil // da qui in giù non c'è ancora niente
			}
			return err
		}
		if èLink(fi) {
			return errPercorsoLink
		}
	}
	return nil
}

// controllaPercorsoApp: come controllaPercorsoIn con la cartella dati di Tasso.
func controllaPercorsoApp(p string) error {
	if appDir == "" {
		return nil
	}
	return controllaPercorsoIn(appDir, p)
}

// scriviFileApp: come os.WriteFile, ma non scrive se il percorso passa da un collegamento o una
// giunzione sotto la cartella dati di Tasso.
func scriviFileApp(p string, b []byte, perm os.FileMode) error {
	if err := controllaPercorsoApp(p); err != nil {
		return err
	}
	return os.WriteFile(p, b, perm)
}

// rinominaApp: come os.Rename, ma solo se né il file di partenza né quello di arrivo passano da un
// collegamento o una giunzione (un collegamento come arrivo verrebbe sostituito, non seguito, ma la
// cartella sì).
func rinominaApp(da, a string) error {
	if err := controllaPercorsoApp(filepath.Dir(da)); err != nil {
		return err
	}
	if err := controllaPercorsoApp(filepath.Dir(a)); err != nil {
		return err
	}
	return os.Rename(da, a)
}

// apriLogApp: apre (o crea) un file di registro sotto la cartella dati con i flag dati.
// Se il file stesso è un collegamento o una giunzione lo sposto da parte (rinomino il
// collegamento, non quello a cui punta) e ne creo uno vero; se lo è una cartella del percorso
// non scrivo proprio (restituisce errPercorsoLink).
func apriLogApp(p string, flag int) (*os.File, error) {
	if err := controllaPercorsoApp(filepath.Dir(p)); err != nil {
		return nil, err
	}
	if fi, err := os.Lstat(p); err == nil && èLink(fi) {
		via := p + ".link-" + time.Now().Format("20060102-150405")
		if os.Rename(p, via) != nil {
			if os.Remove(p) != nil { // toglie il collegamento, non il file a cui punta
				return nil, errPercorsoLink
			}
		}
		if fi2, err := os.Lstat(p); err == nil && èLink(fi2) {
			return nil, errPercorsoLink
		}
	}
	return os.OpenFile(p, flag, 0o644)
}

// leggiFileApp: come os.ReadFile, ma non segue collegamenti né giunzioni. Tasso gira come
// amministratore e questi file (boost.json, settings.json...) stanno nella cartella dell'utente:
// un collegamento farebbe leggere a Tasso un file qualsiasi del PC come se fosse suo.
// Controllo il percorso da appDir in giù, il file stesso (deve essere un file normale) e, dopo
// averlo aperto, che il file aperto sia proprio quello controllato (os.SameFile), così anche un
// cambio fatto tra il controllo e l'apertura si vede.
func leggiFileApp(p string) ([]byte, error) {
	if err := controllaPercorsoApp(filepath.Dir(p)); err != nil {
		return nil, err
	}
	fi, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, errPercorsoLink
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi2, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(fi, fi2) || !apertoInApp(f, p) {
		return nil, errPercorsoLink
	}
	return io.ReadAll(f)
}

// apertoInApp: se p sta sotto appDir, il file aperto deve stare davvero sotto appDir anche dopo
// aver risolto collegamenti e giunzioni (GetFinalPathNameByHandle): così una cartella in mezzo
// cambiata in giunzione dopo controllaPercorsoApp si vede lo stesso.
func apertoInApp(f *os.File, p string) bool {
	if appDir == "" {
		return true
	}
	if _, sotto := sottoApp(appDir, p); !sotto {
		return true
	}
	h, err := apriCartellaPerNome(appDir)
	if err != nil {
		return false
	}
	radice := percorsoFinale(h)
	windows.CloseHandle(h)
	vero := percorsoFinale(windows.Handle(f.Fd()))
	return radice != "" && vero != "" && dentroDi(vero, radice)
}
