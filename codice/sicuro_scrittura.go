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
// Limiti (scritti nel rapporto O3-sicdati): i collegamenti fisici (hard link) non si vedono con Lstat.
// RECENSIONE-3b, R3b-4: la corsa tra il controllo e la scrittura è chiusa con os.Root: dopo il
// controllo, scrittura, apertura dei registri e rinomino lavorano a maniglie dentro appDir (aperta
// dalla cartella sopra, AppData\Local, controllata all'avvio: R3b-3) e non seguono collegamenti o
// giunzioni che portano fuori (le giunzioni hanno sempre un percorso assoluto: os.Root non le segue).

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

// radiceApp: appDir aperta come os.Root dalla cartella sopra (una giunzione al posto di appDir non si
// segue). Va chiusa da chi la chiede.
func radiceApp() (*os.Root, error) {
	base := filepath.Clean(appDir)
	su, err := os.OpenRoot(filepath.Dir(base))
	if err != nil {
		return nil, err
	}
	defer su.Close()
	return su.OpenRoot(filepath.Base(base))
}

// relApp: p come nome relativo ad appDir, per os.Root. ok=false se p non è dentro appDir (o è appDir).
func relApp(p string) (string, bool) {
	if appDir == "" {
		return "", false
	}
	pezzi, ok := sottoApp(appDir, p)
	if !ok || len(pezzi) == 0 {
		return "", false
	}
	return filepath.Join(pezzi...), true
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
	if rel, ok := relApp(p); ok {
		r, err := radiceApp()
		if err != nil {
			return err
		}
		defer r.Close()
		return r.WriteFile(rel, b, perm)
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
	rda, ok1 := relApp(da)
	ra, ok2 := relApp(a)
	if ok1 && ok2 {
		r, err := radiceApp()
		if err != nil {
			return err
		}
		defer r.Close()
		return r.Rename(rda, ra)
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
	if rel, ok := relApp(p); ok {
		r, err := radiceApp()
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return r.OpenFile(rel, flag, 0o644)
	}
	return os.OpenFile(p, flag, 0o644)
}

// errNonVerificato: Windows non sa dire dove porta davvero il file aperto (GetFinalPathNameByHandle
// non risponde) o il file è cambiato proprio mentre lo aprivo. NON vuol dire che sia un collegamento:
// chi lo riceve riprova e, se non basta, non scrive sopra il file (le impostazioni non si perdono).
var errNonVerificato = errors.New("non riesco a controllare dove porta il file: non lo uso adesso")

// leggiFileApp: come os.ReadFile, ma non segue collegamenti né giunzioni. Tasso gira come
// amministratore e questi file (boost.json, settings.json...) stanno nella cartella dell'utente:
// un collegamento farebbe leggere a Tasso un file qualsiasi del PC come se fosse suo.
// Controllo il percorso da appDir in giù e il file stesso (deve essere un file normale); il percorso
// vero di appDir lo calcolo prima di aprire il file, aprendo appDir senza seguire collegamenti; dopo
// l'apertura il file deve essere quello controllato (os.SameFile) e il suo percorso vero deve stare
// sotto quello di appDir. Errori: errPercorsoLink (collegamento vero o file fuori), errNonVerificato
// (non si sa), o quello di Lstat.
func leggiFileApp(p string) ([]byte, error) {
	if err := controllaPercorsoApp(filepath.Dir(p)); err != nil {
		return nil, err
	}
	radice := ""
	if _, sotto := sottoApp(appDir, p); appDir != "" && sotto {
		r, err := radiceVeraApp(appDir)
		if err != nil {
			return nil, err
		}
		radice = r
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
	if err != nil || !os.SameFile(fi, fi2) {
		return nil, errNonVerificato
	}
	if radice != "" {
		vero := percorsoFinale(windows.Handle(f.Fd()))
		if vero == "" {
			return nil, errNonVerificato
		}
		if !dentroDi(vero, radice) {
			return nil, errPercorsoLink
		}
	}
	return io.ReadAll(f)
}

// radiceVeraApp: il percorso vero di appDir, aperta senza seguire collegamenti
// (FILE_FLAG_OPEN_REPARSE_POINT): se appDir stessa è un collegamento → errPercorsoLink.
func radiceVeraApp(d string) (string, error) {
	u, err := windows.UTF16PtrFromString(d)
	if err != nil {
		return "", errNonVerificato
	}
	h, err := windows.CreateFile(u, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return "", err
		}
		return "", errNonVerificato
	}
	defer windows.CloseHandle(h)
	var fi windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &fi) != nil {
		return "", errNonVerificato
	}
	if fi.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || fi.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return "", errPercorsoLink
	}
	v := percorsoFinale(h)
	if v == "" {
		return "", errNonVerificato
	}
	return v, nil
}
