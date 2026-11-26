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
	// CartellaDatiUtente: %LOCALAPPDATA% dell'utente (lì Tasso Zip e Tasso PDF tengono
	// impostazioni e file temporanei). nil o "" = non toccare niente.
	CartellaDatiUtente func() string
	// UAC "con un altro account" (utente standard che scrive la password di un amministratore):
	// l'installatore gira come l'amministratore, ma Tasso Zip e Tasso PDF girano come l'utente
	// della sessione e scrivono nel SUO HKCU e nella SUA %LOCALAPPDATA%. RegistroUtente (HKCU =
	// quello dell'utente della sessione) e CartellaDatiSessione servono per le loro tracce e per la
	// lingua. nil = lo stesso utente (il caso normale) o i test.
	RegistroUtente       Registro
	CartellaDatiSessione func() string
	// File occupati (vedi riprova.go). nil = i valori veri: codici di Windows, time.Sleep, e
	// nessuna cancellazione al riavvio.
	Bloccato          func(error) bool
	Dormi             func(time.Duration)
	CancellaAlRiavvio func(percorso string) error // MoveFileEx(MOVEFILE_DELAY_UNTIL_REBOOT)
	// PreparaCartella: crea (o controlla) la cartella base. Fuori da Programmi la cartella nuova
	// nasce già con i permessi di Programmi (proprietario Amministratori, gli utenti solo leggono):
	// creata e poi protetta, nel frattempo un programma qualsiasi poteva metterci un file suo.
	// Una cartella che c'è già si usa solo se è sicura (DecidiCartellaSicura). creata = l'ha
	// creata adesso. nil = os.MkdirAll (Linux, test).
	PreparaCartella func(base string) (creata bool, err error)
	// PulisciTasso: disinstallando Tasso, Tasso stesso toglie le sue cose (Tasso.exe
	// --togli-silenzioso: attività, Boost, Ufficio, traduzioni nei giochi). Un errore ferma la
	// disinstallazione prima di togliere qualsiasi cosa. nil = niente (test).
	PulisciTasso func(base string) error
	// TogliAttivita: toglie la cartella \Tasso\ dell'Utilità di pianificazione con le sue attività.
	TogliAttivita func() error
}

// Opzioni scelte nelle pagine dell'installatore.
type Opzioni struct {
	Base       string       // C:\Program Files\Tasso
	Componenti []Componente // caselle spuntate
	Desktop    bool         // collegamenti anche sul Desktop (per tutti i programmi scelti)
	// DesktopTogli: la casella del Desktop era spuntata (c'erano già i collegamenti) e l'hai tolta tu:
	// via i collegamenti sul Desktop dei programmi scelti. Senza Desktop e senza DesktopTogli i
	// collegamenti che sono già sul Desktop restano (e si rifanno), ma non se ne aggiungono altri.
	DesktopTogli    bool
	Lingua          string // it, en, es, fr, de, pt, pl
	CartellaMenu    string // menu Start di tutti gli utenti
	CartellaDesktop string // Desktop pubblico
	// TieniDisinstallatore: non sostituire "Disinstalla Tasso.exe" (aggiornamento silenzioso con un
	// installatore di una versione dell'ecosistema più vecchia di quella installata).
	TieniDisinstallatore bool
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

// togliCartella toglie una cartella con tutto quello che c'è dentro (se non c'è va bene).
func (in *Installatore) togliCartella(p string) error {
	_ = os.RemoveAll(p)
	if _, err := os.Lstat(p); err == nil {
		return fmt.Errorf("%s: non tolta del tutto (un file è aperto?)", p)
	}
	return nil
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
// Per Tasso il nome dipende dalle modalità scelte: vedi nomeCollegamento.
var NomiCollegamento = map[Programma]string{
	ProgTasso: "Tasso Gaming.lnk",
	ProgZip:   "Tasso Zip.lnk",
	ProgPDF:   "Tasso PDF.lnk",
}

// CollegamentiPer prepara i collegamenti dei programmi installati.
func (in *Installatore) CollegamentiPer(pp []Programma, o Opzioni) []Collegamento {
	var out []Collegamento
	for _, id := range pp {
		p := in.Elenco.Prog(id)
		nome := nomeCollegamento(id, o)
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

// nomeCollegamento: "Tasso Gaming" se c'è la parte Gaming, "Tasso Ufficio" se c'è solo Ufficio.
func nomeCollegamento(id Programma, o Opzioni) string {
	if id == ProgTasso && ModalitaTasso(o.Componenti) == ModalitaUfficio {
		return "Tasso Ufficio.lnk"
	}
	return NomiCollegamento[id]
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
	Diario("installo %v in %s (lingua %s)", o.Componenti, o.Base, o.Lingua)
	st, err := in.installa(ctx, o)
	if err != nil {
		Diario("installazione NON riuscita: %v", err)
	}
	return st, err
}

func (in *Installatore) installa(ctx context.Context, o Opzioni) (*Stato, error) {
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
	if in.Elenco.Prog(ProgSetup) != nil && !o.TieniDisinstallatore {
		progs = append(progs, ProgSetup)
	}

	in.avanza(Avanzamento{Fase: "prepara"})
	creataBase := false
	if in.PreparaCartella != nil {
		// fuori da Programmi: cartella creata già protetta, o controllata e protetta (vedi
		// DecidiCartellaSicura); dentro Programmi una cartella normale
		c, err := in.PreparaCartella(o.Base)
		if err != nil {
			Diario("cartella %s non usata: %v", o.Base, err)
			return nil, err
		}
		creataBase = c
	} else {
		if _, err := os.Stat(o.Base); os.IsNotExist(err) {
			creataBase = true
		}
		if err := os.MkdirAll(o.Base, 0o755); err != nil {
			return nil, err
		}
	}
	if err := in.Ripara(o.Base); err != nil { // resti di un'installazione interrotta
		return nil, err
	}
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
	Diario("1. copio e controllo i file nuovi in %s", nuovi)
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
		if o.TieniDisinstallatore {
			st.Versione = Maggiore(vecchio.Versione, st.Versione) // mai una versione dell'ecosistema più vecchia
		}
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
	Diario("2. metto i file nuovi al loro posto")
	in.avanza(Avanzamento{Fase: "sposta", Fatto: totale, Totale: totale})
	// segno: c'è finché la fase 2 non è finita. Se il PC si spegne a metà, Ripara lo trova e
	// rimette TUTTI i file vecchi (dentro c'è anche l'elenco dei file rinominati accanto).
	segno := filepath.Join(o.Base, fileInCorso)
	if err := os.WriteFile(segno, nil, 0o644); err != nil {
		pulisci()
		return nil, err
	}
	var fatte []mossa
	// indietro rimette i file com'erano. Se anche un solo file non torna al suo posto, la copia
	// dei file vecchi (.tasso-setup-vecchio) NON si cancella: è l'unica che resta, e Ripara
	// riprova la prossima volta (anche dopo un riavvio).
	indietro := func() bool {
		Diario("torno indietro: rimetto i file com'erano (%d spostamenti)", len(fatte))
		ok := true
		for i := len(fatte) - 1; i >= 0; i-- {
			m := fatte[i]
			if err := in.conRiprova("rimetto "+m.da, func() error { return in.rinomina(m.a, m.da) }); err != nil {
				ok = false
			}
		}
		if !ok {
			Diario("NON sono riuscito a rimettere tutti i file: tengo %s, %s e %s (Ripara riprova)", vecchi, nuovi, segno)
			return false
		}
		_ = os.RemoveAll(vecchi)
		_ = os.Remove(segno)
		pulisci()
		return true
	}
	// tornaIndietro: indietro, e se non è riuscito lo dice insieme all'errore di prima.
	tornaIndietro := func(err error) error {
		if indietro() {
			return err
		}
		return &ErroreNonRimesso{Err: err}
	}
	sposta := func(da, a string) error {
		if err := os.MkdirAll(filepath.Dir(a), 0o755); err != nil {
			return err
		}
		if err := in.conRiprova("sposto "+da+" -> "+a, func() error { return in.rinomina(da, a) }); err != nil {
			return err
		}
		fatte = append(fatte, mossa{da, a})
		return nil
	}
	var accanto []string // file vecchi rimasti occupati, spostati accanto (Tasso.exe.vecchio-1)
	// togliDiMezzo sposta il file vecchio dst in copia; se resta occupato, ultima possibilità:
	// lo rinomino accanto (Windows lo permette anche con un exe o una dll in uso).
	togliDiMezzo := func(dst, copia string) error {
		err := sposta(dst, copia)
		if err == nil || !in.bloccato(err) {
			return err
		}
		lato := accantoLibero(dst)
		annotaAccanto(segno, lato, dst) // prima di rinominare: se il PC si spegne, Ripara lo sa
		if e := in.rinomina(dst, lato); e != nil {
			Diario("anche la rinomina accanto (%s) non riesce: %v", lato, e)
			return &ErroreBloccato{File: dst, Err: err}
		}
		Diario("%s occupato: rinominato in %s (si cancella dopo)", dst, lato)
		fatte = append(fatte, mossa{dst, lato})
		accanto = append(accanto, lato)
		return nil
	}
	metti := func(nuovo, dst, copia string) error {
		if _, err := os.Lstat(dst); err == nil {
			if err := togliDiMezzo(dst, copia); err != nil {
				var eb *ErroreBloccato
				if errors.As(err, &eb) {
					return err
				}
				return fmt.Errorf("non riesco a sostituire %s (è aperto?): %w", dst, err)
			}
		}
		if err := sposta(nuovo, dst); err != nil {
			if in.bloccato(err) {
				return &ErroreBloccato{File: dst, Err: err}
			}
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
				return nil, tornaIndietro(err)
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
					if err := togliDiMezzo(dst, copia); err != nil {
						var eb *ErroreBloccato
						if !errors.As(err, &eb) {
							err = fmt.Errorf("non riesco a togliere %s (è aperto?): %w", dst, err)
						}
						return nil, tornaIndietro(err)
					}
				}
			}
		}
	}
	if err := metti(filepath.Join(nuovi, FileStato), filepath.Join(o.Base, FileStato), filepath.Join(vecchi, FileStato)); err != nil {
		return nil, tornaIndietro(err)
	}

	// 3. registro (se non riesce, si torna indietro con i file)
	Diario("3. registro di Windows e collegamenti")
	in.avanza(Avanzamento{Fase: "registro", Fatto: totale, Totale: totale})
	if err := in.scriviRegistro(st, o.Lingua); err != nil {
		rimesso := indietro()
		if vecchio != nil {
			_ = in.scriviRegistro(vecchio, vecchio.Lingua)
		} else {
			_ = in.togliRegistro()
		}
		err = fmt.Errorf("non riesco a scrivere nel registro di Windows: %w", err)
		if !rimesso {
			err = &ErroreNonRimesso{Err: err}
		}
		return nil, err
	}

	// file e registro a posto: da qui un'interruzione non deve più rimettere i file vecchi.
	// Prima via .tasso-setup-nuovo: con lui ancora lì e senza segno, Ripara rimetterebbe i file
	// vecchi che la versione nuova ha tolto (caso "resti di un installatore di prima").
	_ = os.RemoveAll(nuovi)
	if err := in.conRiprova("tolgo "+segno, func() error {
		if e := os.Remove(segno); e != nil && !os.IsNotExist(e) {
			return e
		}
		return nil
	}); err != nil {
		Diario("non riesco a togliere %s: %v", segno, err)
	}

	// collegamenti: se uno non riesce non è grave (i programmi ci sono), lo si dice e basta
	in.avanza(Avanzamento{Fase: "collegamenti", Fatto: totale, Totale: totale})
	var errColl error
	if in.CreaCollegamento != nil {
		gia := map[string]bool{}
		for _, c := range st.Collegamenti {
			gia[strings.ToLower(c.Percorso)] = true
		}
		// Il nome del collegamento di Tasso ("Tasso Gaming" o "Tasso Ufficio") dipende da tutto
		// quello che sarà installato, non solo da quello scelto adesso (es. reinstallo solo Ufficio
		// ma Gaming c'è già: resta "Tasso Gaming").
		oNomi := o
		oNomi.Componenti = st.Componenti
		progScelti := ProgrammiPer(scelti)
		// Desktop: con la casella spuntata per tutti; altrimenti solo per i programmi che lì ce
		// l'hanno già (mai toglierli se non l'hai chiesto tu)
		var sulDesktop map[Programma]bool
		if !o.Desktop && !o.DesktopTogli {
			sulDesktop = DesktopEsistenti(st, o.CartellaDesktop)
		}
		oNomi.Desktop = true
		desk := filepath.Clean(o.CartellaDesktop)
		var nuoviColl []Collegamento
		for _, c := range in.CollegamentiPer(progScelti, oNomi) {
			if o.CartellaDesktop != "" && strings.EqualFold(filepath.Dir(c.Percorso), desk) &&
				!o.Desktop && !sulDesktop[c.Programma] {
				continue
			}
			nuoviColl = append(nuoviColl, c)
		}
		voluti := map[string]bool{}
		tassoOra := false
		for _, id := range progScelti {
			tassoOra = tassoOra || id == ProgTasso
		}
		for _, c := range nuoviColl {
			voluti[strings.ToLower(c.Percorso)] = true
		}
		// collegamenti di Tasso con un nome vecchio (es. "Tasso.lnk" delle versioni precedenti): li
		// tolgo, ma solo se adesso installo anche Tasso (se aggiorno solo Zip, quelli di Tasso restano)
		// Con DesktopTogli via anche quelli sul Desktop dei programmi scelti.
		scelto := map[Programma]bool{}
		for _, id := range progScelti {
			scelto[id] = true
		}
		tenuti := st.Collegamenti[:0]
		for _, c := range st.Collegamenti {
			vecchioNome := tassoOra && c.Programma == ProgTasso && !voluti[strings.ToLower(c.Percorso)]
			togliDesk := o.DesktopTogli && o.CartellaDesktop != "" && scelto[c.Programma] &&
				!voluti[strings.ToLower(c.Percorso)] && strings.EqualFold(filepath.Dir(filepath.Clean(c.Percorso)), desk)
			if (vecchioNome || togliDesk) && strings.EqualFold(filepath.Ext(c.Percorso), ".lnk") {
				_ = in.togli(c.Percorso)
				delete(gia, strings.ToLower(c.Percorso))
				continue
			}
			tenuti = append(tenuti, c)
		}
		st.Collegamenti = tenuti
		for _, c := range nuoviColl {
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

	// 4. pulizia: un file vecchio ancora aperto resta lì e si cancella al prossimo riavvio
	// (e comunque Ripara lo toglie la prossima volta)
	_ = os.RemoveAll(nuovi)
	if err := os.RemoveAll(vecchi); err != nil {
		Diario("cartella %s non tolta del tutto: %v", vecchi, err)
		in.cancellaAlRiavvio(vecchi)
	}
	for _, f := range accanto {
		if err := os.Remove(f); err != nil {
			Diario("%s ancora occupato: si cancella al prossimo riavvio", f)
			in.cancellaAlRiavvio(f)
		}
	}
	Diario("installazione finita: versione %s in %s", st.Versione, o.Base)
	in.avanza(Avanzamento{Fase: "fine", Fatto: totale, Totale: totale})
	if errColl != nil {
		return st, &AvvisoCollegamenti{errColl}
	}
	return st, nil
}

// cancellaAlRiavvio fa cancellare a Windows al prossimo riavvio il file p, o la cartella p con
// quello che è rimasto dentro (prima i file, poi le cartelle, dalla più interna).
func (in *Installatore) cancellaAlRiavvio(p string) {
	if in.CancellaAlRiavvio == nil {
		return
	}
	var file, cartelle []string
	_ = filepath.WalkDir(p, func(q string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			cartelle = append(cartelle, q)
		} else {
			file = append(file, q)
		}
		return nil
	})
	for i := len(cartelle) - 1; i >= 0; i-- {
		file = append(file, cartelle[i])
	}
	for _, f := range file {
		if err := in.CancellaAlRiavvio(f); err != nil {
			Diario("non riesco a programmare la cancellazione di %s: %v", f, err)
		}
	}
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
	var w *os.File
	if err := in.conRiprova("scrivo "+dst, func() (e error) {
		w, e = os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
		return e
	}); err != nil {
		if in.bloccato(err) {
			return 0, &ErroreBloccato{File: dst, Err: err}
		}
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

// fileInCorso: c'è dall'inizio della fase 2 fino alla fine dell'installazione (o del ritorno
// indietro). Dentro, una riga per ogni file vecchio rinominato accanto: "<accanto>\t<file>".
const fileInCorso = ".tasso-setup-in-corso"

// ErroreNonRimesso: l'installazione non è riuscita e non tutti i file vecchi sono tornati al loro
// posto. La loro copia resta in .tasso-setup-vecchio: riaprendo l'installatore (anche dopo un
// riavvio) Ripara li rimette.
type ErroreNonRimesso struct{ Err error }

func (e *ErroreNonRimesso) Error() string {
	return "non tutti i file sono tornati com'erano (la copia resta in " + cartellaVecchi + ", riavvia il PC e riapri l'installatore): " + e.Err.Error()
}
func (e *ErroreNonRimesso) Unwrap() error { return e.Err }

// ErrRiparaNonRiuscito: un'installazione interrotta non si è potuta sistemare: non installo
// sopra (rischierei di perdere la copia dei file vecchi).
var ErrRiparaNonRiuscito = errors.New("un'installazione interrotta non si è potuta sistemare: riavvia il PC e riprova")

func annotaAccanto(segno, lato, dst string) {
	f, err := os.OpenFile(segno, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		Diario("non riesco ad annotare %s: %v", lato, err)
		return
	}
	defer f.Close()
	_, _ = f.WriteString(lato + "\t" + dst + "\n")
}

// dstDaVecchi: dove va un file della copia dei vecchi ("" se il nome non è valido).
func dstDaVecchi(base, vecchi, p string) string {
	rel, err := filepath.Rel(vecchi, p)
	if err != nil {
		return ""
	}
	parti := strings.SplitN(filepath.ToSlash(rel), "/", 2)
	if len(parti) == 1 && parti[0] == FileStato {
		return filepath.Join(base, FileStato)
	}
	if len(parti) == 2 {
		if d, ok := percorsoFile(CartellaDi(base, Programma(parti[0])), parti[1]); ok {
			return d
		}
	}
	return ""
}

// rimetti mette il file vecchio p al posto di dst (togliendo il nuovo, se c'è).
func (in *Installatore) rimetti(p, dst string) error {
	if _, err := os.Lstat(dst); err == nil {
		if err := in.conRiprova("tolgo il nuovo "+dst, func() error { return os.Remove(dst) }); err != nil {
			return err
		}
	}
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	return in.conRiprova("rimetto "+dst, func() error { return in.rinomina(p, dst) })
}

// Ripara sistema i resti di un'installazione interrotta (PC spento a metà):
//   - fase 2 non finita (c'è .tasso-setup-in-corso): TUTTI i file vecchi tornano al loro posto,
//     anche quelli già sostituiti (niente versioni mischiate), compresi i file rinominati accanto;
//   - resti di un installatore di prima (c'è ancora .tasso-setup-nuovo, niente segno): tornano i
//     file vecchi che non hanno più niente al loro posto;
//   - installazione finita ma copia dei vecchi rimasta (un file era occupato): si cancella e basta
//     (mai rimettere file che la versione nuova ha tolto).
//
// Le copie dei vecchi si cancellano solo se ogni file è tornato al suo posto: altrimenti restano
// e Ripara restituisce ErrRiparaNonRiuscito (non si installa sopra).
func (in *Installatore) Ripara(base string) error {
	vecchi := filepath.Join(base, cartellaVecchi)
	nuovi := filepath.Join(base, cartellaNuovi)
	segno := filepath.Join(base, fileInCorso)
	_, errSegno := os.Lstat(segno)
	_, errVecchi := os.Stat(vecchi)
	_, errNuovi := os.Stat(nuovi)
	ok := true
	switch {
	case errSegno == nil && errNuovi != nil:
		// segno senza .tasso-setup-nuovo: l'installazione è finita (la cartella dei nuovi si toglie
		// subito prima del segno), solo il segno non si era cancellato. Niente da rimettere: mai i
		// *.vecchio-N, sarebbe la versione vecchia sopra quella nuova.
		Diario("Ripara: segno rimasto dopo un'installazione finita in %s, niente da rimettere", base)
	case errSegno == nil:
		Diario("Ripara: installazione interrotta a metà in %s, rimetto tutti i file vecchi", base)
		if errVecchi == nil {
			_ = filepath.WalkDir(vecchi, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				if dst := dstDaVecchi(base, vecchi, p); dst != "" && in.rimetti(p, dst) != nil {
					ok = false
				}
				return nil
			})
		}
		if b, err := os.ReadFile(segno); err == nil {
			for _, riga := range strings.Split(string(b), "\n") {
				lato, dst, trovato := strings.Cut(strings.TrimSpace(riga), "\t")
				if !trovato || lato == "" || dst == "" {
					continue
				}
				if _, err := os.Lstat(lato); err == nil && in.rimetti(lato, dst) != nil {
					ok = false
				}
			}
		}
	case errVecchi == nil && errNuovi == nil:
		_ = filepath.WalkDir(vecchi, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if dst := dstDaVecchi(base, vecchi, p); dst != "" {
				if _, err := os.Lstat(dst); os.IsNotExist(err) && in.rimetti(p, dst) != nil {
					ok = false
				}
			}
			return nil
		})
	}
	if !ok {
		Diario("Ripara: non tutti i file sono tornati al loro posto, tengo %s", vecchi)
		return ErrRiparaNonRiuscito
	}
	if errVecchi == nil {
		if err := os.RemoveAll(vecchi); err != nil {
			Diario("cartella %s non tolta del tutto: %v", vecchi, err)
			in.cancellaAlRiavvio(vecchi)
		}
	}
	_ = os.RemoveAll(nuovi)
	_ = os.Remove(segno)
	return nil
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
		// anche per l'utente della sessione (Zip e PDF girano come lui); non è grave se non riesce
		if ru := in.RegistroUtente; ru != nil {
			if err := ru.ScriviTesto(HKCU, ChiaveTasso, "Lingua", lingua); err != nil {
				Diario("lingua non scritta per l'utente della sessione: %v", err)
			}
		}
	}
	return nil
}

// registroTracce: dove sono le tracce di Zip e PDF (HKCU dell'utente della sessione).
func (in *Installatore) registroTracce() Registro {
	if in.RegistroUtente != nil {
		return in.RegistroUtente
	}
	return in.Registro
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
