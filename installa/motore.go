package installa

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Errori che l'interfaccia riconosce.
var (
	ErrAnnullato = errors.New("installazione annullata")
	ErrVerifica  = errors.New("un file copiato non corrisponde all'originale (SHA-256)")
)

// Collegamento: un .lnk nel menu Start o sul Desktop.
type Collegamento struct {
	Percorso     string // ...\Tasso.lnk
	Destinazione string // ...\Tasso.exe
	Cartella     string // cartella di lavoro
	Descrizione  string
	Programma    Programma
}

// Avanzamento: per la barra della pagina "Installazione".
type Avanzamento struct {
	Fase   string `json:"fase"` // prepara, copia, sposta, registro, collegamenti, fine
	File   string `json:"file,omitempty"`
	Fatto  int64  `json:"fatto"`
	Totale int64  `json:"totale"`
}

// Installatore mette insieme tutto quello che serve. I campi func hanno un valore
// predefinito (os.Rename, os.Remove...) e nei test si sostituiscono per provare gli errori.
type Installatore struct {
	Sorgente fs.FS // i file incorporati (embed.FS), con elenco.json
	Elenco   *Elenco
	Registro Registro

	CreaCollegamento func(Collegamento) error
	TogliFile        func(percorso string) error // anche i .lnk; su Windows sa togliere il disinstallatore aperto
	Rinomina         func(da, a string) error
	Avanzamento      func(Avanzamento)
	Adesso           func() time.Time
}

// Opzioni scelte nelle pagine dell'installatore.
type Opzioni struct {
	Base            string       // C:\Program Files\Tasso
	Componenti      []Componente // caselle spuntate
	Desktop         bool         // collegamenti anche sul Desktop
	Lingua          string       // it, en, es, fr, de, pt, pl
	CartellaMenu    string       // menu Start di tutti gli utenti
	CartellaDesktop string       // Desktop pubblico
}

func (in *Installatore) rinomina(a, b string) error {
	if in.Rinomina != nil {
		return in.Rinomina(a, b)
	}
	return os.Rename(a, b)
}

func (in *Installatore) togli(p string) error {
	if in.TogliFile != nil {
		return in.TogliFile(p)
	}
	return os.Remove(p)
}

func (in *Installatore) adesso() time.Time {
	if in.Adesso != nil {
		return in.Adesso()
	}
	return time.Now()
}

func (in *Installatore) avanza(a Avanzamento) {
	if in.Avanzamento != nil {
		in.Avanzamento(a)
	}
}

// NomiCollegamento: il nome del .lnk di ogni programma.
var NomiCollegamento = map[Programma]string{
	ProgTasso: "Tasso.lnk",
	ProgZip:   "Tasso Zip.lnk",
	ProgPDF:   "Tasso PDF.lnk",
}

// CollegamentiPer prepara i collegamenti dei programmi installati.
func (in *Installatore) CollegamentiPer(pp []Programma, o Opzioni) []Collegamento {
	var out []Collegamento
	for _, id := range pp {
		p := in.Elenco.Prog(id)
		nome := NomiCollegamento[id]
		if p == nil || p.Avvio == "" || nome == "" {
			continue
		}
		dir := CartellaDi(o.Base, id)
		dest, ok := percorsoFile(dir, p.Avvio)
		if !ok {
			continue
		}
		cartelle := []string{o.CartellaMenu}
		if o.Desktop {
			cartelle = append(cartelle, o.CartellaDesktop)
		}
		for _, c := range cartelle {
			if c == "" {
				continue
			}
			out = append(out, Collegamento{Percorso: filepath.Join(c, nome), Destinazione: dest,
				Cartella: dir, Descrizione: p.Nome, Programma: id})
		}
	}
	return out
}

// mossa: un file spostato durante la fase finale (per tornare indietro se qualcosa va storto).
type mossa struct{ da, a string }

// Installa copia i programmi scelti. Fasi:
//  1. copia ogni file in <cartella>\.tasso-setup-nuovo e lo rilegge dal disco per
//     controllare l'SHA-256 (si può annullare: niente è ancora cambiato);
//  2. sposta i file vecchi in .tasso-setup-vecchio e quelli nuovi al loro posto
//     (solo rinomine, velocissime; se una non riesce rimette tutto com'era);
//  3. scrive tasso-setup.json, il registro e i collegamenti;
//  4. cancella le cartelle temporanee.
//
// I file di Tasso che non sono dell'installatore (es. la cartella "lavoro" di Tasso)
// non vengono toccati. Un componente installato prima e non scelto adesso resta com'è.
func (in *Installatore) Installa(ctx context.Context, o Opzioni) (*Stato, error) {
	scelti, err := Normalizza(o.Componenti)
	if err != nil {
		return nil, err
	}
	if o.Base == "" || !filepath.IsAbs(o.Base) {
		return nil, &ErroreCartella{ErrCartNonAssol}
	}
	o.Base = filepath.Clean(o.Base)
	progs := ProgrammiPer(scelti)
	for _, id := range progs {
		if in.Elenco.Prog(id) == nil {
			return nil, fmt.Errorf("il programma %q non è dentro questo installatore", id)
		}
	}
	if in.Elenco.Prog(ProgSetup) != nil {
		progs = append(progs, ProgSetup)
	}

	in.avanza(Avanzamento{Fase: "prepara"})
	creataBase := false
	if _, err := os.Stat(o.Base); os.IsNotExist(err) {
		creataBase = true
	}
	if err := os.MkdirAll(o.Base, 0o755); err != nil {
		return nil, err
	}
	in.Ripara(o.Base) // resti di un'installazione interrotta
	vecchio, err := LeggiStato(o.Base)
	if err != nil {
		vecchio = nil // file rovinato: si riscrive (i file dei programmi si sovrascrivono comunque)
	}
	nuovi := filepath.Join(o.Base, cartellaNuovi)
	vecchi := filepath.Join(o.Base, cartellaVecchi)
	pulisci := func() {
		_ = os.RemoveAll(nuovi)
		if creataBase {
			_ = os.Remove(o.Base) // solo se è vuota
		}
	}

	// 1. copia e controllo
	totale := 2 * in.Elenco.Dimensione(progs)
	var fatto int64
	for _, id := range progs {
		p := in.Elenco.Prog(id)
		for _, f := range p.File {
			if err := ctx.Err(); err != nil {
				pulisci()
				return nil, ErrAnnullato
			}
			dst, _ := percorsoFile(filepath.Join(nuovi, string(id)), f.Nome)
			in.avanza(Avanzamento{Fase: "copia", File: f.Nome, Fatto: fatto, Totale: totale})
			n, err := in.copiaControlla(ctx, path.Join(string(id), f.Nome), dst, f, func(d int64) {
				fatto += d
				in.avanza(Avanzamento{Fase: "copia", File: f.Nome, Fatto: fatto, Totale: totale})
			})
			_ = n
			if err != nil {
				pulisci()
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return nil, ErrAnnullato
				}
				return nil, err
			}
		}
	}
	if ctx.Err() != nil {
		pulisci()
		return nil, ErrAnnullato
	}

	// stato nuovo
	st := &Stato{Versione: in.Elenco.Versione, Cartella: o.Base, Lingua: o.Lingua,
		Programmi: map[Programma]StatoProg{}, Data: in.adesso().Format("20060102")}
	if vecchio != nil {
		st.Componenti = vecchio.Componenti
		for k, v := range vecchio.Programmi {
			st.Programmi[k] = v
		}
		st.Collegamenti = vecchio.Collegamenti
	}
	st.Componenti = Unione(st.Componenti, scelti)
	for _, id := range progs {
		p := in.Elenco.Prog(id)
		st.Programmi[id] = StatoProg{Versione: p.Versione, Cartella: CartellaDi(o.Base, id), Avvio: p.Avvio, File: p.File}
	}
	datiStato, err := st.dati()
	if err != nil {
		pulisci()
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(nuovi, FileStato), datiStato, 0o644); err != nil {
		pulisci()
		return nil, err
	}

	// 2. sposta (da qui non si annulla più: sono pochi istanti)
	in.avanza(Avanzamento{Fase: "sposta", Fatto: totale, Totale: totale})
	var fatte []mossa
	indietro := func() {
		for i := len(fatte) - 1; i >= 0; i-- {
			_ = in.rinomina(fatte[i].a, fatte[i].da)
		}
		_ = os.RemoveAll(vecchi)
		pulisci()
	}
	sposta := func(da, a string) error {
		if err := os.MkdirAll(filepath.Dir(a), 0o755); err != nil {
			return err
		}
		if err := in.rinomina(da, a); err != nil {
			return err
		}
		fatte = append(fatte, mossa{da, a})
		return nil
	}
	metti := func(nuovo, dst, copia string) error {
		if _, err := os.Lstat(dst); err == nil {
			if err := sposta(dst, copia); err != nil {
				return fmt.Errorf("non riesco a sostituire %s (è aperto?): %w", dst, err)
			}
		}
		if err := sposta(nuovo, dst); err != nil {
			return fmt.Errorf("non riesco a mettere %s: %w", dst, err)
		}
		return nil
	}
	for _, id := range progs {
		p := in.Elenco.Prog(id)
		dir := CartellaDi(o.Base, id)
		nuoviQui := map[string]bool{}
		for _, f := range p.File {
			nuoviQui[strings.ToLower(f.Nome)] = true
			src, _ := percorsoFile(filepath.Join(nuovi, string(id)), f.Nome)
			dst, _ := percorsoFile(dir, f.Nome)
			copia, _ := percorsoFile(filepath.Join(vecchi, string(id)), f.Nome)
			if err := metti(src, dst, copia); err != nil {
				indietro()
				return nil, err
			}
		}
		// file della versione vecchia che la nuova non ha più: via anche loro (in .tasso-setup-vecchio)
		if vecchio != nil {
			for _, f := range vecchio.Programmi[id].File {
				if nuoviQui[strings.ToLower(f.Nome)] {
					continue
				}
				dst, ok := percorsoFile(dir, f.Nome)
				copia, ok2 := percorsoFile(filepath.Join(vecchi, string(id)), f.Nome)
				if !ok || !ok2 {
					continue
				}
				if _, err := os.Lstat(dst); err == nil {
					if err := sposta(dst, copia); err != nil {
						indietro()
						return nil, fmt.Errorf("non riesco a togliere %s (è aperto?): %w", dst, err)
					}
				}
			}
		}
	}
	if err := metti(filepath.Join(nuovi, FileStato), filepath.Join(o.Base, FileStato), filepath.Join(vecchi, FileStato)); err != nil {
		indietro()
		return nil, err
	}

	// 3. registro (se non riesce, si torna indietro con i file)
	in.avanza(Avanzamento{Fase: "registro", Fatto: totale, Totale: totale})
	if err := in.scriviRegistro(st, o.Lingua); err != nil {
		indietro()
		if vecchio != nil {
			_ = in.scriviRegistro(vecchio, vecchio.Lingua)
		} else {
			_ = in.togliRegistro()
		}
		return nil, fmt.Errorf("non riesco a scrivere nel registro di Windows: %w", err)
	}

	// collegamenti: se uno non riesce non è grave (i programmi ci sono), lo si dice e basta
	in.avanza(Avanzamento{Fase: "collegamenti", Fatto: totale, Totale: totale})
	var errColl error
	if in.CreaCollegamento != nil {
		gia := map[string]bool{}
		for _, c := range st.Collegamenti {
			gia[strings.ToLower(c.Percorso)] = true
		}
		for _, c := range in.CollegamentiPer(ProgrammiPer(scelti), o) {
			if err := in.CreaCollegamento(c); err != nil {
				errColl = err
				continue
			}
			if !gia[strings.ToLower(c.Percorso)] {
				st.Collegamenti = append(st.Collegamenti, StatoCollegamento{Percorso: c.Percorso, Programma: c.Programma})
				gia[strings.ToLower(c.Percorso)] = true
			}
		}
		if b, err := st.dati(); err == nil {
			_ = scriviAtomico(filepath.Join(o.Base, FileStato), b)
		}
	}

	// 4. pulizia
	_ = os.RemoveAll(nuovi)
	if err := os.RemoveAll(vecchi); err != nil {
		// un file vecchio ancora aperto: resta lì, Ripara lo toglie la prossima volta
		_ = err
	}
	in.avanza(Avanzamento{Fase: "fine", Fatto: totale, Totale: totale})
	if errColl != nil {
		return st, &AvvisoCollegamenti{errColl}
	}
	return st, nil
}

// AvvisoCollegamenti: installazione riuscita ma un collegamento non si è creato.
type AvvisoCollegamenti struct{ Err error }

func (a *AvvisoCollegamenti) Error() string { return "collegamento non creato: " + a.Err.Error() }

// copiaControlla copia un file incorporato, calcola l'SHA-256 mentre scrive, poi lo
// rilegge dal disco e ricalcola: entrambe le impronte devono essere quella dell'elenco.
func (in *Installatore) copiaControlla(ctx context.Context, src, dst string, f FileElenco, passo func(int64)) (int64, error) {
	r, err := in.Sorgente.Open(src)
	if err != nil {
		return 0, fmt.Errorf("manca %s dentro l'installatore: %w", src, err)
	}
	defer r.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	w, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return 0, err
	}
	h := sha256.New()
	n, err := copiaConPassi(ctx, io.MultiWriter(w, h), r, passo)
	if err == nil {
		err = w.Sync()
	}
	if e := w.Close(); err == nil {
		err = e
	}
	if err != nil {
		return n, err
	}
	if hex.EncodeToString(h.Sum(nil)) != strings.ToLower(f.SHA256) || n != f.Dim {
		return n, fmt.Errorf("%w: %s (dentro l'installatore è rovinato)", ErrVerifica, f.Nome)
	}
	// rilettura dal disco
	rf, err := os.Open(dst)
	if err != nil {
		return n, err
	}
	defer rf.Close()
	h2 := sha256.New()
	if _, err := copiaConPassi(ctx, h2, rf, passo); err != nil {
		return n, err
	}
	if hex.EncodeToString(h2.Sum(nil)) != strings.ToLower(f.SHA256) {
		return n, fmt.Errorf("%w: %s", ErrVerifica, f.Nome)
	}
	return n, nil
}

func copiaConPassi(ctx context.Context, w io.Writer, r io.Reader, passo func(int64)) (int64, error) {
	buf := make([]byte, 1<<20)
	var tot int64
	for {
		if err := ctx.Err(); err != nil {
			return tot, err
		}
		n, err := r.Read(buf)
		if n > 0 {
			if _, e := w.Write(buf[:n]); e != nil {
				return tot, e
			}
			tot += int64(n)
			passo(int64(n))
		}
		if err == io.EOF {
			return tot, nil
		}
		if err != nil {
			return tot, err
		}
	}
}

func scriviAtomico(p string, b []byte) error {
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Ripara sistema i resti di un'installazione interrotta (PC spento a metà):
// i file vecchi che non hanno più il nuovo al loro posto tornano dove erano,
// poi le cartelle temporanee si cancellano.
func (in *Installatore) Ripara(base string) {
	vecchi := filepath.Join(base, cartellaVecchi)
	if _, err := os.Stat(vecchi); err == nil {
		_ = filepath.WalkDir(vecchi, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(vecchi, p)
			if err != nil {
				return nil
			}
			parti := strings.SplitN(filepath.ToSlash(rel), "/", 2)
			var dst string
			if len(parti) == 1 && parti[0] == FileStato {
				dst = filepath.Join(base, FileStato)
			} else if len(parti) == 2 {
				dst, _ = percorsoFile(CartellaDi(base, Programma(parti[0])), parti[1])
			}
			if dst != "" {
				if _, err := os.Lstat(dst); os.IsNotExist(err) {
					_ = os.MkdirAll(filepath.Dir(dst), 0o755)
					_ = in.rinomina(p, dst)
				}
			}
			return nil
		})
		_ = os.RemoveAll(vecchi)
	}
	_ = os.RemoveAll(filepath.Join(base, cartellaNuovi))
}

// scriviRegistro: "Programmi e funzionalità" (HKLM\...\Uninstall\Tasso),
// HKLM\Software\Tasso (cartella, componenti, modalità) e HKCU\Software\Tasso\Lingua.
func (in *Installatore) scriviRegistro(st *Stato, lingua string) error {
	r := in.Registro
	if r == nil {
		return nil
	}
	base := st.Cartella
	disinst := filepath.Join(base, NomeDisinstalla)
	icona := disinst + ",0"
	if p, ok := st.Programmi[ProgTasso]; ok && p.Avvio != "" {
		if e, ok := percorsoFile(base, p.Avvio); ok {
			icona = e + ",0"
		}
	}
	kb := uint32((st.Dimensione() + 1023) / 1024)
	testi := [][2]string{
		{"DisplayName", "Tasso"},
		{"DisplayVersion", st.Versione},
		{"Publisher", "Sergio Tassoni"},
		{"DisplayIcon", icona},
		{"InstallLocation", base},
		{"UninstallString", `"` + disinst + `"`},
		{"InstallDate", st.Data},
		{"Comments", "Tasso Gaming, Tasso Ufficio, Tasso Zip, Tasso PDF"},
	}
	for _, t := range testi {
		if err := r.ScriviTesto(HKLM, ChiaveDisinstalla, t[0], t[1]); err != nil {
			return err
		}
	}
	for _, n := range [][2]any{{"NoModify", uint32(1)}, {"NoRepair", uint32(1)}, {"EstimatedSize", kb}} {
		if err := r.ScriviNumero(HKLM, ChiaveDisinstalla, n[0].(string), n[1].(uint32)); err != nil {
			return err
		}
	}
	if err := in.scriviTasso(st); err != nil {
		return err
	}
	if lingua != "" {
		if err := r.ScriviTesto(HKCU, ChiaveTasso, "Lingua", lingua); err != nil {
			return err
		}
	}
	return nil
}

// scriviTasso: HKLM\Software\Tasso. Valori:
//
//	Cartella    C:\Program Files\Tasso
//	Componenti  gaming,ufficio,zip,pdf
//	Modalita    gioco | ufficio | entrambi   (solo se Tasso è installato)
//	Versione    versione dell'ecosistema
func (in *Installatore) scriviTasso(st *Stato) error {
	r := in.Registro
	if err := r.ScriviTesto(HKLM, ChiaveTasso, "Cartella", st.Cartella); err != nil {
		return err
	}
	if err := r.ScriviTesto(HKLM, ChiaveTasso, "Componenti", TestoComponenti(st.Componenti)); err != nil {
		return err
	}
	if err := r.ScriviTesto(HKLM, ChiaveTasso, "Versione", st.Versione); err != nil {
		return err
	}
	if m := ModalitaTasso(st.Componenti); m != "" {
		return r.ScriviTesto(HKLM, ChiaveTasso, "Modalita", m)
	}
	return r.TogliValore(HKLM, ChiaveTasso, "Modalita")
}
