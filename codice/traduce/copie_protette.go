package traduce

// Copie di sicurezza delle traduzioni nei giochi (manifesto.json e originali dei file sostituiti):
// Tasso le usa da amministratore per rimettere a posto i giochi. In %LOCALAPPDATA% le può cambiare
// qualunque programma dell'utente (per esempio far dire al manifesto "rimetti questo file al posto
// di quello"). Dalla 3.2 stanno nella cartella protetta di Tasso (Programmi\Tasso\lavoro), dove
// solo gli amministratori scrivono. Le copie vecchie si spostano lì da sole, una volta sola.

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// errLink: c'è un collegamento o una giunzione, o il file porta fuori dalla cartella attesa.
// errNonVerificato: Windows non sa dire dove sta davvero il file (GetFinalPathNameByHandle non
// risponde, per esempio su certi dischi o filtri): non vuol dire che sia un collegamento, quindi chi
// lo riceve non deve trattare il file come rovinato né scriverci sopra.
var (
	errLink          = errors.New("collegamento o giunzione: non leggo")
	errNonVerificato = errors.New("non riesco a controllare dove porta il file: non lo uso adesso")
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
	// Decido una volta sola, ma solo quando Tasso ha già detto qual è la cartella protetta
	// (Ospite.CartellaProtetta): prima, per esempio in --togli-silenzioso, non fisso niente.
	if !copieDeciso && dirApp != "" && Ospite.CartellaProtetta != nil {
		copieDeciso = true
		if d := Ospite.CartellaProtetta(); d != "" && os.MkdirAll(filepath.Join(d, "copie-giochi"), 0o755) == nil {
			if err := spostaCopieVecchie(dirApp, d); err != nil {
				scriviLogCopie("copie dei giochi non spostate tutte (riprovo al prossimo avvio): " + err.Error())
			}
			copieProtetta = d
		}
	}
	if copieProtetta != "" {
		return copieProtetta
	}
	return dirApp
}

// basiCopie: tutti i posti dove possono esserci copie dei giochi: quello in uso e, se è diverso,
// quello vecchio in AppData (copie che non si sono potute spostare, o fatte da Tasso Traduce da
// solo). Chi toglie le traduzioni guarda sempre tutti e due.
func basiCopie() []string {
	var out []string
	for _, b := range []string{baseCopie(), dirApp} {
		if b == "" {
			continue
		}
		gia := false
		for _, o := range out {
			if filepath.Clean(o) == filepath.Clean(b) {
				gia = true
			}
		}
		if !gia {
			out = append(out, b)
		}
	}
	return out
}

// baseDi: il posto (tra basiCopie) dove c'è l'elenco di quel gioco; se non c'è da nessuna parte,
// quello in uso. Così "Ferma traduzione" trova anche un gioco tradotto da Tasso Traduce da solo.
func baseDi(chiave string) string {
	for _, b := range basiCopie() {
		if _, err := os.Lstat(filepath.Join(b, "copie-giochi", chiave, "manifesto.json")); err == nil {
			return b
		}
	}
	return baseCopie()
}

// doppione: la copia di chiave in base è un doppione di quella nella cartella in uso (vale quella).
func doppione(base, chiave string) bool {
	uso := baseCopie()
	if filepath.Clean(base) == filepath.Clean(uso) {
		return false
	}
	_, err := os.Lstat(filepath.Join(uso, "copie-giochi", chiave, "manifesto.json"))
	return err == nil
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
// Un gioco che c'è già nella cartella nuova non lo tocco. Ogni gioco copiato (o già presente nella
// nuova) lo tolgo subito dalla vecchia: un gioco ha sempre una copia sola. Se tutto va bene tolgo la
// cartella vecchia e metto il segno; se qualcosa non va, i giochi non copiati restano e riprovo.
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
	// Il percorso vero della cartella vecchia lo calcolo adesso, una volta sola, aprendola senza
	// seguire collegamenti: se dopo qualcuno la cambia in una giunzione, i file portano fuori da
	// questa stringa e non li copio.
	radice, err := radiceVera(old)
	if errors.Is(err, errLink) {
		return fatto()
	}
	if err != nil {
		return err // non so dove porta: riprovo al prossimo avvio
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
		vecchio := filepath.Join(old, v.Name())
		if _, err := os.Lstat(dest); err == nil {
			// c'è già nella cartella nuova (spostato a un avvio di prima): quella vale, la vecchia via,
			// così ogni gioco ha una copia sola
			_ = os.RemoveAll(vecchio)
			continue
		}
		if err := copiaCartellaDentro(vecchio, dest, radice); err != nil {
			_ = os.RemoveAll(dest) // niente copie a metà
			errs = append(errs, errors.New(v.Name()+": "+err.Error()))
			continue
		}
		_ = os.RemoveAll(vecchio) // copiato: tolgo subito la copia vecchia di questo gioco
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
// giunzioni (Tasso lo legge da amministratore dalla cartella dell'utente), se il file aperto è
// proprio quello controllato e se il suo percorso vero sta sotto il percorso vero di base, calcolato
// una volta sola prima di aprire il file. Errori: errLink (collegamento vero o file fuori),
// errNonVerificato (non si sa), o quello di Lstat (per esempio il file non c'è).
func leggiSenzaLink(base, rel string) ([]byte, error) {
	radice, err := radiceVera(base)
	if err != nil {
		return nil, err
	}
	if !senzaLink(base, rel) {
		return nil, errLink
	}
	p := filepath.Join(base, rel)
	fi, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, errLink
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi2, err := f.Stat()
	if err != nil || !os.SameFile(fi, fi2) {
		return nil, errNonVerificato // cambiato proprio adesso (anche un salvataggio): riprovo dopo
	}
	if err := controllaAperto(f, radice, false); err != nil {
		return nil, err
	}
	return io.ReadAll(f)
}

// dentroFinale: p (percorso vero) è radice o sta sotto di lei. Lettere maiuscole e minuscole
// uguali, come in Windows. Vuoti = no.
func dentroFinale(p, radice string) bool {
	p, radice = strings.TrimRight(p, `\/`), strings.TrimRight(radice, `\/`)
	if p == "" || radice == "" {
		return false
	}
	if strings.EqualFold(p, radice) {
		return true
	}
	return len(p) > len(radice) && (p[len(radice)] == '\\' || p[len(radice)] == '/') && strings.EqualFold(p[:len(radice)], radice)
}

// copiaCartellaDentro: come copiaCartella, per lo spostamento delle copie vecchie (la cartella di
// partenza è dell'utente, Tasso è amministratore). Salto link e giunzioni senza seguirli; ogni file
// lo apro, controllo che sia quello visto con Lstat e che il suo percorso vero sia ancora dentro la
// cartella di partenza (una cartella cambiata in giunzione a metà copia non porta fuori), e copio
// dal file già aperto.
func copiaCartellaDentro(da, a, radice string) error {
	return filepath.Walk(da, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(da, p)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return errors.New("percorso fuori dalla cartella di partenza")
		}
		if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil // link o giunzione: non lo seguo
		}
		if !senzaLink(a, rel) {
			return errors.New("nel percorso di " + rel + " c'è un link o una giunzione")
		}
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(a, rel), 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copiaFileDentro(p, filepath.Join(a, rel), info, radice)
	})
}

func copiaFileDentro(p, dest string, visto os.FileInfo, radice string) error {
	in, err := os.Open(p)
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(visto, fi) || !fi.Mode().IsRegular() {
		return errors.New(p + " è cambiato mentre lo copiavo")
	}
	if err := controllaAperto(in, radice, true); err != nil {
		return errors.New(p + ": " + err.Error())
	}
	if fd, err := os.Lstat(dest); err == nil && !fd.Mode().IsRegular() {
		return errors.New(dest + " non è un file normale")
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// èCartellaProtetta: base è la cartella protetta in uso (Programmi\Tasso\lavoro).
func èCartellaProtetta(base string) bool {
	copieMu.Lock()
	d := copieProtetta
	copieMu.Unlock()
	return d != "" && filepath.Clean(d) == filepath.Clean(base)
}

// leggiSenzaLinkSemplice: come leggiSenzaLink senza il percorso vero (solo per la cartella protetta).
func leggiSenzaLinkSemplice(base, rel string) ([]byte, error) {
	if !senzaLink(base, rel) {
		return nil, errLink
	}
	p := filepath.Join(base, rel)
	fi, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, errLink
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if fi2, err := f.Stat(); err != nil || !os.SameFile(fi, fi2) {
		return nil, errNonVerificato
	}
	return io.ReadAll(f)
}

// manifestoAffidabile: dentro Tasso (che gira da amministratore) un elenco che sta in AppData, dove
// qualunque programma dell'utente scrive, lo uso solo se porta a un gioco vero dove Tasso ha messo
// davvero la sua traduzione: mai cartelle di Windows, mai cartelle che non sono di giochi. Gli
// elenchi nella cartella protetta e Tasso Traduce da solo (che non è amministratore) restano come prima.
func manifestoAffidabile(base string, m ManifestoMod) bool {
	if Ospite.CartellaProtetta == nil || èCartellaProtetta(base) {
		return true
	}
	if m.Cartella == "" || !filepath.IsAbs(m.Cartella) || cartellaDiSistema(m.Cartella) {
		return false
	}
	if Ospite.GiocoDiTasso != nil && !Ospite.GiocoDiTasso(filepath.Join(m.Cartella, filepath.Base(m.Exe))) {
		return false
	}
	return segnoTassoNelGioco(m)
}

// segnoTassoNelGioco: nella cartella del gioco c'è quello che ci ha messo Tasso. Unity: il file di
// XUnity scritto da Tasso, con dentro l'indirizzo del traduttore sul PC e il segreto di questo
// elenco. Unreal: il pak zz_TassoTraduce_P.pak indicato nell'elenco. Nessun collegamento in mezzo.
func segnoTassoNelGioco(m ManifestoMod) bool {
	if m.Tipo == "unreal" {
		for _, a := range m.Aggiunti {
			if filepath.IsAbs(a) || strings.Contains(a, "..") || !strings.EqualFold(filepath.Base(a), nomeModUnreal) {
				continue
			}
			if fi, err := os.Lstat(filepath.Join(m.Cartella, a)); err == nil && fi.Mode().IsRegular() && senzaLink(m.Cartella, a) {
				return true
			}
		}
		return false
	}
	if len(m.Token) < 8 {
		return false
	}
	rel := filepath.Join("BepInEx", "config", "AutoTranslatorConfig.ini")
	if !senzaLink(m.Cartella, rel) {
		return false
	}
	p := filepath.Join(m.Cartella, rel)
	if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() || fi.Size() > 1<<20 {
		return false
	}
	b, err := os.ReadFile(p)
	return err == nil && strings.Contains(string(b), "/gioco/") && strings.Contains(string(b), m.Token)
}

// cartellaDiSistema: dentro la cartella di Windows (o è la radice di un disco).
func cartellaDiSistema(c string) bool {
	c = filepath.Clean(c)
	if len(c) <= 3 {
		return true
	}
	w := os.Getenv("SystemRoot")
	if w == "" {
		w = os.Getenv("windir")
	}
	return w != "" && dentroFinale(c, w)
}

// elencoIgnorato: in AppData c'è un elenco di quel gioco che si legge ma che Tasso non usa
// (manifestoAffidabile: per esempio il file di XUnity di Tasso non c'è più). dove = la cartella con
// gli originali. Serve per dire all'utente cosa fare, invece di non dire niente.
func elencoIgnorato(chiave string) (m ManifestoMod, dove string, ok bool) {
	if dirApp == "" || èCartellaProtetta(dirApp) {
		return m, "", false
	}
	b, err := leggiSenzaLink(dirApp, filepath.Join("copie-giochi", chiave, "manifesto.json"))
	if err != nil || json.Unmarshal(b, &m) != nil || manifestoAffidabile(dirApp, m) {
		return m, "", false
	}
	return m, filepath.Join(dirApp, "copie-giochi", chiave, "originali"), true
}

// GiochiDaSistemareAMano: i giochi con un elenco ignorato in AppData (per "Togli Tasso dal PC").
func GiochiDaSistemareAMano() []string {
	if dirApp == "" {
		return nil
	}
	var out []string
	voci, _ := os.ReadDir(filepath.Join(dirApp, "copie-giochi"))
	for _, v := range voci {
		if !v.IsDir() || v.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 || doppione(dirApp, v.Name()) {
			continue
		}
		if m, _, ok := elencoIgnorato(v.Name()); ok {
			n := m.Gioco
			if n == "" {
				n = v.Name()
			}
			out = append(out, n)
		}
	}
	return out
}

// msgSistemaAMano: cosa fare per un gioco che Tasso non può rimettere com'era da qui.
func msgSistemaAMano(gioco, dove string) string {
	return "Non riesco a togliere da qui la traduzione dentro " + gioco + ": usa \"Verifica file\" nel programma da cui lanci il gioco. Gli originali dei file sono in " + dove + "."
}
