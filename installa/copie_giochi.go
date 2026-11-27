package installa

// Originali dei giochi tradotti dentro la cartella di Tasso (dalla 3.2:
// <base>\lavoro\copie-giochi). Di solito li mette al sicuro Tasso stesso (Tasso.exe
// --togli-silenzioso li porta in Documenti). Se Tasso è andato in errore, si è bloccato, ha
// superato il tempo o è un Tasso che non conosce l'argomento, sono ancora lì: prima di
// cancellare la cartella "lavoro" li metto al sicuro io, in Documenti o, se non si può, in
// %ProgramData%\Tasso - copie di sicurezza. Se non si può da nessuna parte la disinstallazione si
// ferma prima di togliere qualsiasi cosa: gli originali non si perdono mai.

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// NomeCopieSicurezza: il nome della cartella delle copie di sicurezza (come in Tasso, uninstall.go).
const NomeCopieSicurezza = "Tasso - copie di sicurezza"

// ErroreCopieNonSalvate: gli originali dei giochi in Dove non si sono potuti mettere al sicuro:
// non ho tolto niente.
type ErroreCopieNonSalvate struct{ Dove []string }

func (e *ErroreCopieNonSalvate) Error() string {
	return "non riesco a mettere al sicuro gli originali dei giochi tradotti: " + strings.Join(e.Dove, ", ")
}

// CartellaCopieGiochi: <base>\lavoro\copie-giochi.
func CartellaCopieGiochi(base string) string { return filepath.Join(base, "lavoro", "copie-giochi") }

// giochiDaSalvare: le cartelle dei giochi dentro copie-giochi (il segno ".spostate-da-appdata",
// gli altri file, i collegamenti e le giunzioni non contano: non li seguo).
func giochiDaSalvare(base string) []string {
	src := CartellaCopieGiochi(base)
	if fi, err := os.Lstat(src); err != nil || !fi.IsDir() || fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return nil
	}
	voci, _ := os.ReadDir(src)
	var out []string
	for _, v := range voci {
		if !v.IsDir() || v.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			continue
		}
		if dentro, _ := os.ReadDir(filepath.Join(src, v.Name())); len(dentro) == 0 {
			continue
		}
		out = append(out, v.Name())
	}
	return out
}

// cartellaVeraOOneDrive: x è una cartella vera, non un collegamento o una giunzione. Le cartelle
// di OneDrive (file su richiesta) sono anche loro "reparse point", ma non portano altrove: vanno
// bene (stesso controllo di Tasso, uninstall.go).
func cartellaVeraOOneDrive(x string) bool {
	st, err := os.Lstat(x)
	if err != nil || st.Mode()&os.ModeSymlink != 0 {
		return false
	}
	if st.Mode()&os.ModeIrregular != 0 {
		_, e := os.Readlink(x)
		return e != nil // giunzione: no; OneDrive: sì
	}
	return st.IsDir()
}

// StradaSenzaCollegamenti: dalla radice del disco fino a dir (compresa), nessuna cartella è un
// collegamento o una giunzione (Documenti come giunzione porterebbe la scrittura, fatta da
// amministratore, in un altro posto). La radice del disco non conta.
func StradaSenzaCollegamenti(dir string) bool {
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) {
		return false
	}
	vol := filepath.VolumeName(dir)
	resto := strings.TrimPrefix(strings.TrimPrefix(dir, vol), string(filepath.Separator))
	p := vol + string(filepath.Separator)
	for _, x := range strings.Split(resto, string(filepath.Separator)) {
		if x == "" {
			continue
		}
		p = filepath.Join(p, x)
		if !cartellaVeraOOneDrive(p) {
			return false
		}
	}
	return true
}

// nomeLibero: dir\nome se è libero, se no "nome (copia del 27-11-2026)", poi con l'ora e un numero.
func nomeLibero(dir, nome string, ora time.Time) string {
	esiste := func(p string) bool { _, err := os.Lstat(p); return err == nil }
	p := filepath.Join(dir, nome)
	if !esiste(p) {
		return p
	}
	base := nome + " (copia del " + ora.Format("02-01-2006")
	if p = filepath.Join(dir, base+")"); !esiste(p) {
		return p
	}
	base += " " + ora.Format("15.04")
	for n := 1; ; n++ {
		s := base + ")"
		if n > 1 {
			s = base + " " + strconv.Itoa(n) + ")"
		}
		if p = filepath.Join(dir, s); !esiste(p) {
			return p
		}
	}
}

// copiaAlbero copia da in a (che non deve esistere) senza seguire collegamenti e giunzioni.
func copiaAlbero(da, a string) error {
	return filepath.Walk(da, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(da, p)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return errors.New("percorso fuori dalla cartella")
		}
		if fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			if fi.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		t := filepath.Join(a, rel)
		if fi.IsDir() {
			return os.MkdirAll(t, 0o755)
		}
		if !fi.Mode().IsRegular() {
			return nil
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(t, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}

// SalvaCopieGiochi mette al sicuro le cartelle dei giochi in <base>\lavoro\copie-giochi, nella
// prima delle mete (di solito Documenti\Tasso - copie di sicurezza, poi ProgramData) dove riesce:
// in <meta>\giochi\<gioco>. Nelle mete di Documenti copio (i file prendono i permessi di Documenti,
// rinominando si porterebbero dietro quelli di Programmi); nelle altre rinomino, poi copio.
// Restituisce dove sono finite. Se un gioco non va da nessuna parte: ErroreCopieNonSalvate (gli
// altri giochi, già messi al sicuro, restano dove sono finiti: non si perde niente).
func SalvaCopieGiochi(base string, mete []Meta, adesso time.Time) (salvate []string, err error) {
	giochi := giochiDaSalvare(base)
	if len(giochi) == 0 {
		return nil, nil
	}
	src := CartellaCopieGiochi(base)
	var nonSalvate []string
	for _, g := range giochi {
		da := filepath.Join(src, g)
		fatto := false
		for _, m := range mete {
			if m.Cartella == "" || !filepath.IsAbs(m.Cartella) || Dentro(m.Cartella, base) {
				continue
			}
			dir := filepath.Join(m.Cartella, "giochi")
			if !StradaSenzaCollegamenti(primaEsistente(dir)) {
				Diario("copie dei giochi: %s porta altrove (collegamento o giunzione), non la uso", m.Cartella)
				continue
			}
			if os.MkdirAll(dir, 0o755) != nil || !StradaSenzaCollegamenti(dir) {
				continue
			}
			a := nomeLibero(dir, g, adesso)
			if !m.Copia && os.Rename(da, a) == nil {
				salvate, fatto = append(salvate, a), true
				break
			}
			// copio in "<a> (copia non finita)" e rinomino solo alla fine (R3): una copia interrotta
			// non resta mai con il nome giusto accanto a quella buona
			tmp := nomeLibero(dir, filepath.Base(a)+" (copia non finita)", adesso)
			e := copiaAlbero(da, tmp)
			if e == nil {
				e = os.Rename(tmp, a)
			}
			if e == nil {
				_ = os.RemoveAll(da)
				salvate, fatto = append(salvate, a), true
				break
			}
			Diario("copie dei giochi: %s in %s non riuscita: %v", da, a, e)
			_ = os.RemoveAll(tmp) // solo la copia appena fatta: gli originali sono ancora in da
		}
		if !fatto {
			nonSalvate = append(nonSalvate, da)
		}
	}
	if len(salvate) > 0 {
		Diario("originali dei giochi messi al sicuro: %v", salvate)
	}
	if len(nonSalvate) > 0 {
		return salvate, &ErroreCopieNonSalvate{Dove: nonSalvate}
	}
	return salvate, nil
}

// Meta: dove mettere le copie. Copia = copiare invece di rinominare (Documenti).
type Meta struct {
	Cartella string
	Copia    bool
}

// primaEsistente: p, o la cartella più vicina sopra di lei che c'è già (prima di crearla controllo
// che la strada fino a lì non passi da collegamenti).
func primaEsistente(p string) string {
	p = filepath.Clean(p)
	for {
		if _, err := os.Lstat(p); err == nil {
			return p
		}
		su := filepath.Dir(p)
		if su == p {
			return p
		}
		p = su
	}
}

// salvaCopieGiochiPrima: in DisinstallaCon, prima di togliere qualsiasi cosa (se va via Tasso e la
// cartella è solo di Tasso, cioè "lavoro" verrà cancellata).
func (in *Installatore) salvaCopieGiochiPrima(base string, ris *Risultato) error {
	if len(giochiDaSalvare(base)) == 0 {
		return nil
	}
	var mete []Meta
	if in.MeteCopieGiochi != nil {
		mete = in.MeteCopieGiochi()
	}
	salvate, err := SalvaCopieGiochi(base, mete, in.adesso())
	ris.CopieSalvate = append(ris.CopieSalvate, salvate...)
	if err != nil {
		Diario("disinstallazione fermata: %v", err)
		return err
	}
	return nil
}

// LeggiUscitaTogli legge tutta l'uscita di Tasso --togli-silenzioso, riga per riga, senza limiti
// di lunghezza (una riga lunghissima non ferma la lettura: Tasso non resta bloccato a scrivere) e
// fino alla fine, anche dopo righe strane. Per ogni "TOGLI-PASSO n" chiama passo. Restituisce solo
// le righe che servono (TOGLI-*, il segno), ognuna al massimo di 4 KB: TOGLI-OK e TOGLI-NO non si
// perdono mai (R8).
func LeggiUscitaTogli(r io.Reader, passo func(int)) string {
	br := bufio.NewReaderSize(r, 64<<10)
	var out strings.Builder
	for {
		riga, err := br.ReadString('\n')
		if riga != "" {
			t := strings.TrimRight(riga, "\r\n")
			if n, ok := PassoTogli(t); ok && passo != nil {
				passo(n)
			}
			if strings.Contains(t, "TOGLI-") || strings.Contains(t, "tasso-togli-silenzioso") {
				if i := strings.Index(t, "TOGLI-"); i > 0 {
					t = t[i:]
				}
				if len(t) > 4<<10 {
					t = t[:4<<10]
				}
				out.WriteString(t + "\n")
			}
		}
		if err != nil {
			return out.String()
		}
	}
}

// PassoTogli: una riga "TOGLI-PASSO n" di Tasso --togli-silenzioso (n = 0 impostazioni, 1
// traduzioni nei giochi, 2 copie di sicurezza): il passo che Tasso comincia adesso.
func PassoTogli(riga string) (int, bool) {
	riga = strings.TrimSpace(riga)
	if !strings.HasPrefix(riga, "TOGLI-PASSO ") {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(riga, "TOGLI-PASSO ")))
	if err != nil || n < 0 || n > 2 {
		return 0, false
	}
	return n, true
}

// AvvisiTogli: le righe "TOGLI-AVVISO <testo>" di Tasso --togli-silenzioso (Tasso 3.2, F20-1):
// impostazioni di Windows che Tasso non è riuscito a rimettere com'erano. Il testo è già tradotto
// da Tasso. Non fermano la disinstallazione: la pagina finale li mostra. Al massimo 20.
func AvvisiTogli(uscita string) []string {
	var out []string
	for _, riga := range strings.Split(uscita, "\n") {
		riga = strings.TrimSpace(riga)
		if t, ok := strings.CutPrefix(riga, "TOGLI-AVVISO "); ok {
			if t = strings.TrimSpace(t); t != "" && len(out) < 20 {
				out = append(out, t)
			}
		}
	}
	return out
}

// EsitoTogliSilenzioso legge la risposta di "Tasso.exe --togli-silenzioso": ok solo con
// "TOGLI-OK"; con "TOGLI-NO <motivo>" il motivo (la disinstallazione si ferma). Errore, blocco o
// tempo scaduto senza nessuna delle due: ok=false, rifiuto=nil (si va avanti, e gli originali dei
// giochi rimasti in lavoro li mette al sicuro SalvaCopieGiochi).
func EsitoTogliSilenzioso(uscita string, errEsecuzione error) (ok bool, rifiuto error) {
	if strings.Contains(uscita, "TOGLI-OK") && errEsecuzione == nil {
		return true, nil
	}
	if i := strings.Index(uscita, "TOGLI-NO "); i >= 0 {
		motivo := strings.TrimSpace(strings.SplitN(uscita[i+len("TOGLI-NO "):], "\n", 2)[0])
		if motivo == "" {
			motivo = "Tasso non ha detto perché"
		}
		return false, errors.New(motivo)
	}
	if errEsecuzione != nil {
		Diario("Tasso --togli-silenzioso non ha finito (%v): metto al sicuro io quello che resta", errEsecuzione)
	} else {
		Diario("Tasso --togli-silenzioso non ha detto TOGLI-OK: metto al sicuro io quello che resta")
	}
	return false, nil
}
