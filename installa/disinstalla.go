package installa

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Risultato della disinstallazione.
type Risultato struct {
	Stato      *Stato   // quello che resta (nil se è stato tolto tutto)
	TuttoTolto bool     // non resta niente: via anche disinstallatore e voce in "Programmi e funzionalità"
	NonTolti   []string // file che non si sono potuti cancellare (aperti?)
	// AlRiavvio: file ancora occupati anche dopo aver riprovato: li cancella Windows al prossimo riavvio.
	AlRiavvio []string
	// DatiTenuti: con "togli anche i miei dati", le cartelle tenute apposta (CartelleDaTenere:
	// originali dei giochi tradotti, copie del registro) perché non erano vuote. La pagina finale
	// lo dice.
	DatiTenuti []string
	// base: la cartella dell'installazione. Al riavvio si cancella solo quello che è dentro: i
	// file fuori (dati in %LOCALAPPDATA%...) restano tra i NonTolti, come dice la pagina finale.
	base string
}

// OpzioniDisinstalla: scelte della pagina "Disinstalla".
type OpzioniDisinstalla struct {
	// TogliDati: via anche i dati di Tasso in %LOCALAPPDATA% (Tasso e TassoTraduce: impostazioni,
	// memoria delle traduzioni, modelli scaricati, Portachiavi). Solo se l'utente lo conferma.
	TogliDati bool
	// TassoGiaPulito: Tasso ha già tolto le sue cose (PulisciTasso chiamata prima, per esempio
	// prima di chiudere gli altri programmi): non lo richiamo.
	TassoGiaPulito bool
}

// TogliTasso: togliendo questi componenti va via anche Tasso.exe (Gaming e Ufficio tutti e due,
// o l'unico dei due che c'è). false se st è nil.
func TogliTasso(st *Stato, togli []Componente) bool {
	if st == nil {
		return false
	}
	togli, err := normalizza(togli)
	if err != nil || len(togli) == 0 {
		return false
	}
	prima, dopo := false, false
	for _, p := range ProgrammiPer(st.Componenti) {
		prima = prima || p == ProgTasso
	}
	for _, p := range ProgrammiPer(Meno(st.Componenti, togli)) {
		dopo = dopo || p == ProgTasso
	}
	return prima && !dopo
}

// CartellaSoloTasso: la cartella base è solo di Tasso (si chiama Tasso, o dentro ci sono soltanto
// cose di Tasso Setup: i file e le cartelle dei programmi dell'elenco, le cartelle di lavoro di
// Tasso, i file dell'installatore). Solo allora lavoro, strumenti e driver si toccano: le
// installazioni di prima in una cartella condivisa (D:\Programmi, C:\Program Files) possono avere
// una cartella "driver" o "strumenti" di un altro programma.
func CartellaSoloTasso(base string, st *Stato) bool {
	if st == nil {
		return false
	}
	if strings.EqualFold(filepath.Base(filepath.Clean(base)), CartellaTassoStd) {
		return true
	}
	noti := map[string]bool{strings.ToLower(FileStato): true, strings.ToLower(FileStato + ".tmp"): true,
		cartellaNuovi: true, cartellaVecchi: true, fileInCorso: true, cartellaScarto: true}
	for _, d := range SottocartelleTasso {
		noti[d] = true
	}
	for _, f := range ResiduiAggiornamentoTasso {
		noti[strings.ToLower(f)] = true
	}
	for id, sp := range st.Programmi {
		dir := CartellaDi(base, id)
		if !strings.EqualFold(filepath.Clean(dir), filepath.Clean(base)) {
			if rel, err := filepath.Rel(base, dir); err == nil {
				noti[strings.ToLower(strings.SplitN(filepath.ToSlash(rel), "/", 2)[0])] = true
			}
			continue
		}
		for _, f := range sp.File {
			noti[strings.ToLower(strings.SplitN(strings.ReplaceAll(f.Nome, `\`, "/"), "/", 2)[0])] = true
		}
	}
	voci, err := os.ReadDir(base)
	if err != nil {
		return false
	}
	for _, v := range voci {
		n := strings.ToLower(v.Name())
		if noti[n] || strings.Contains(n, ".vecchio-") {
			continue
		}
		return false // qualcosa che non è di Tasso: cartella condivisa
	}
	return true
}

// ChiaviSoloTasso: chiavi del registro scritte da Tasso stesso (non dall'installatore) che con
// Tasso vanno via. Le altre in Software\Tasso restano (le usano Zip, PDF e l'installatore).
var ChiaviSoloTasso = []struct {
	Radice Radice
	Chiave string
}{
	{HKCU, ChiaveTasso + `\AudioUSB`},
	{HKCU, ChiaveTasso + `\PrimaDelleModalita`},
	{HKLM, ChiaveTasso + `\PuntoRipristino`},
}

// CartelleDatiTasso: le cartelle dei dati di Tasso in %LOCALAPPDATA%.
var CartelleDatiTasso = []string{"Tasso", "TassoTraduce"}

// CartelleDaTenere: dentro i dati di Tasso, le cartelle che non si cancellano mai, nemmeno con
// "togli anche i miei dati": gli originali dei file dei giochi tradotti (copie-giochi) e le copie
// del registro. Tasso, quando toglie le sue cose (--togli-silenzioso), le sposta in
// Documenti\Tasso - copie di sicurezza e qui restano vuote; se non l'ha fatto (Tasso vecchio senza
// --togli-silenzioso, bloccato o in errore) sono ancora qui, e cancellarle vorrebbe dire perdere
// per sempre i file originali dei giochi.
var CartelleDaTenere = map[string][]string{
	"Tasso":        {"backup", "backup-registro"},
	"TassoTraduce": {"copie-giochi"},
}

// ResiduiAggiornamentoTasso: copie a metà che Tasso lascia accanto a Tasso.exe quando si aggiorna da
// solo o copia sé stesso in Programmi (selfupdate.go, installProgramCopy). Non sono nell'elenco
// dell'installatore: senza toglierle la cartella di Tasso non resterebbe mai vuota (e resterebbe un
// Tasso.exe vecchio in Programmi).
var ResiduiAggiornamentoTasso = []string{"Tasso.exe.new", "Tasso.exe.old", "Tasso.new.exe", "Tasso.old.exe"}

// Disinstalla toglie i componenti scelti, e solo i file scritti in tasso-setup.json.
//   - Gaming senza Ufficio (o il contrario): Tasso.exe resta, cambia solo la modalità.
//   - Gaming e Ufficio insieme (o l'unico dei due rimasto): via Tasso.exe e il suo collegamento,
//     e le cose di Tasso come in DisinstallaCon (togliCoseDiTasso). I dati di Tasso in
//     %LOCALAPPDATA% restano (solo DisinstallaCon con TogliDati li toglie).
//   - Zip, PDF: via i loro file, i collegamenti e la loro cartella se resta vuota.
//   - Se non resta niente: via anche disinstallatore, tasso-setup.json, le voci
//     del registro dell'installatore e la cartella se è vuota.
func (in *Installatore) Disinstalla(base string, togli []Componente) (*Risultato, error) {
	return in.DisinstallaCon(base, togli, OpzioniDisinstalla{})
}

// DisinstallaCon: come Disinstalla, con le scelte della pagina. Quando va via Tasso (Gaming e
// Ufficio), prima Tasso stesso toglie le sue cose (PulisciTasso: attività, Boost, Ufficio,
// traduzioni nei giochi); se rifiuta non si tocca niente. Poi vanno via anche le sue cartelle di
// lavoro (lavoro, strumenti, driver), le attività pianificate \Tasso\ e le sue chiavi del registro.
func (in *Installatore) DisinstallaCon(base string, togli []Componente, od OpzioniDisinstalla) (*Risultato, error) {
	base = filepath.Clean(base)
	st, err := LeggiStato(base)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, fmt.Errorf("in %s non c'è un'installazione di Tasso Setup", base)
	}
	togli, err = normalizza(togli)
	if err != nil {
		return nil, err
	}
	if len(togli) == 0 {
		return nil, errors.New("nessun componente da togliere")
	}
	restano := Meno(st.Componenti, togli)
	stIniziale := copiaStato(st) // per CartellaSoloTasso: com'era prima di togliere
	progPrima := ProgrammiPer(st.Componenti)
	progDopo := map[Programma]bool{}
	for _, p := range ProgrammiPer(restano) {
		progDopo[p] = true
	}
	tutto := len(restano) == 0
	ris := &Risultato{base: base}

	var via []Programma
	for _, p := range progPrima {
		if !progDopo[p] {
			via = append(via, p)
		}
	}
	if tutto {
		via = append(via, ProgSetup)
	}
	viaTasso := false
	for _, p := range via {
		viaTasso = viaTasso || p == ProgTasso
	}
	if viaTasso && in.PulisciTasso != nil && !od.TassoGiaPulito {
		// Tasso toglie le sue cose finché il suo exe c'è ancora. Se rifiuta (un gioco tradotto è
		// aperto, la modalità riunione è accesa...) non tolgo niente e lo dico.
		if err := in.PulisciTasso(base); err != nil {
			Diario("Tasso non ha tolto le sue cose: %v", err)
			return nil, &ErroreTassoRifiuta{Err: err}
		}
	}
	// prima i collegamenti (così nessuno apre un programma mentre lo tolgo)
	var restColl []StatoCollegamento
	for _, c := range st.Collegamenti {
		tolto := false
		for _, p := range via {
			if c.Programma == p {
				tolto = true
			}
		}
		if !tolto {
			restColl = append(restColl, c)
			continue
		}
		if !strings.EqualFold(filepath.Ext(c.Percorso), ".lnk") {
			continue // un collegamento è sempre un .lnk: qualunque altra cosa non la tocco
		}
		if err := in.togli(c.Percorso); err != nil && !os.IsNotExist(err) {
			ris.NonTolti = append(ris.NonTolti, c.Percorso)
		}
	}
	st.Collegamenti = restColl

	// Tasso resta ma cambia modalità (es. tolto Gaming, resta Ufficio): il collegamento cambia nome
	if progDopo[ProgTasso] && in.CreaCollegamento != nil {
		oNuove := Opzioni{Componenti: restano}
		nuovoNome := nomeCollegamento(ProgTasso, oNuove)
		for i, c := range st.Collegamenti {
			if c.Programma != ProgTasso || !strings.EqualFold(filepath.Ext(c.Percorso), ".lnk") ||
				strings.EqualFold(filepath.Base(c.Percorso), nuovoNome) {
				continue
			}
			nuovo := filepath.Join(filepath.Dir(c.Percorso), nuovoNome)
			dest, ok := percorsoFile(CartellaDi(base, ProgTasso), "Tasso.exe")
			if !ok {
				continue
			}
			if err := in.CreaCollegamento(Collegamento{Percorso: nuovo, Destinazione: dest,
				Cartella: CartellaDi(base, ProgTasso), Descrizione: "Tasso", Programma: ProgTasso}); err == nil {
				_ = in.togli(c.Percorso)
				st.Collegamenti[i].Percorso = nuovo
			}
		}
	}

	for _, id := range via {
		p, ok := st.Programmi[id]
		if !ok {
			continue
		}
		dir := CartellaDi(base, id)
		for _, f := range p.File {
			fp, ok := percorsoFile(dir, f.Nome)
			if !ok {
				continue
			}
			in.togliORiavvio(fp, ris)
			togliCartelleVuote(filepath.Dir(fp), dir)
		}
		if dir != base {
			_ = os.Remove(dir) // solo se vuota
		}
		// menu del tasto destro, "Apri con" e dati in %LOCALAPPDATA% messi dal programma
		ris.NonTolti = append(ris.NonTolti, in.togliTracceUtente(base, id)...)
		if id == ProgTasso {
			in.togliCoseDiTasso(base, stIniziale, od, ris)
		}
		delete(st.Programmi, id)
	}
	st.Componenti = restano

	if tutto {
		_ = os.Remove(filepath.Join(base, FileStato))
		if err := in.togliRegistro(); err != nil {
			return ris, err
		}
		_ = os.RemoveAll(filepath.Join(base, cartellaNuovi))
		_ = os.RemoveAll(filepath.Join(base, cartellaScarto))
		_ = os.RemoveAll(filepath.Join(base, cartellaVecchi))
		_ = os.Remove(filepath.Join(base, fileInCorso))
		_ = os.Remove(filepath.Join(base, FileStato+".tmp"))
		// file vecchi rinominati accanto da un aggiornamento (Tasso.exe.vecchio-1): se ancora
		// occupati, al riavvio
		if voci, err := os.ReadDir(base); err == nil {
			for _, v := range voci {
				if !v.IsDir() && strings.Contains(strings.ToLower(v.Name()), ".vecchio-") {
					in.togliORiavvio(filepath.Join(base, v.Name()), ris)
				}
			}
		}
		_ = os.Remove(base) // solo se vuota (in una cartella condivisa restano le cose degli altri)
		ris.TuttoTolto = true
		return ris, nil
	}
	b, err := st.dati()
	if err != nil {
		return ris, err
	}
	if err := scriviAtomico(filepath.Join(base, FileStato), b); err != nil {
		return ris, err
	}
	if err := in.scriviRegistro(st, ""); err != nil {
		return ris, err
	}
	ris.Stato = st
	return ris, nil
}

// ErroreTassoRifiuta: Tasso non ha potuto togliere le sue cose: non è stato tolto niente.
type ErroreTassoRifiuta struct{ Err error }

func (e *ErroreTassoRifiuta) Error() string { return e.Err.Error() }
func (e *ErroreTassoRifiuta) Unwrap() error { return e.Err }

// togliORiavvio toglie un file riprovando se è occupato; se resta occupato lo fa cancellare a
// Windows al prossimo riavvio (e lo segna in AlRiavvio, o in NonTolti se neanche questo riesce).
func (in *Installatore) togliORiavvio(fp string, ris *Risultato) {
	err := in.conRiprova("tolgo "+fp, func() error {
		if e := in.togli(fp); e != nil && !os.IsNotExist(e) {
			return e
		}
		return nil
	})
	if err == nil {
		return
	}
	if !ris.alRiavvioPermesso(fp) {
		ris.NonTolti = append(ris.NonTolti, fp)
		return
	}
	if in.CancellaAlRiavvio != nil && in.CancellaAlRiavvio(fp) == nil {
		Diario("%s occupato: si cancella al prossimo riavvio", fp)
		ris.AlRiavvio = append(ris.AlRiavvio, fp)
		return
	}
	ris.NonTolti = append(ris.NonTolti, fp)
}

// togliCartellaORiavvio toglie una cartella intera; quello che resta occupato si cancella al riavvio.
func (in *Installatore) togliCartellaORiavvio(d string, ris *Risultato) {
	if _, err := os.Lstat(d); err != nil {
		return
	}
	_, _ = Riprova(func() error {
		_ = os.RemoveAll(d)
		if _, err := os.Lstat(d); err == nil {
			return &os.PathError{Op: "remove", Path: d, Err: errOccupata}
		}
		return nil
	}, func(error) bool { return true }, in.dormi, 8, PausaRiprova)
	if _, err := os.Lstat(d); err != nil {
		return
	}
	if in.CancellaAlRiavvio != nil && ris.alRiavvioPermesso(d) {
		in.cancellaAlRiavvio(d)
		Diario("%s non tolta del tutto: il resto si cancella al prossimo riavvio", d)
		ris.AlRiavvio = append(ris.AlRiavvio, d)
		return
	}
	ris.NonTolti = append(ris.NonTolti, d)
}

// alRiavvioPermesso: p si può far cancellare a Windows al riavvio: solo dentro la cartella
// dell'installazione (fuori, per esempio i dati in %LOCALAPPDATA%, resta tra i non tolti).
func (r *Risultato) alRiavvioPermesso(p string) bool {
	return r.base == "" || Dentro(p, r.base)
}

var errOccupata = errors.New("qualcosa dentro è ancora aperto")

// togliCoseDiTasso: dopo Tasso.exe, le cose che Tasso ha messo da solo: le sue cartelle di lavoro,
// le attività pianificate \Tasso\, le sue chiavi del registro e (se confermato) i suoi dati.
func (in *Installatore) togliCoseDiTasso(base string, st *Stato, od OpzioniDisinstalla, ris *Risultato) {
	if CartellaSoloTasso(base, st) {
		for _, d := range SottocartelleTasso {
			in.togliCartellaORiavvio(filepath.Join(base, d), ris)
		}
		for _, f := range ResiduiAggiornamentoTasso {
			if fi, err := os.Lstat(filepath.Join(base, f)); err == nil && fi.Mode().IsRegular() {
				in.togliORiavvio(filepath.Join(base, f), ris)
			}
		}
	} else {
		Diario("%s non è solo di Tasso: lavoro, strumenti e driver restano (potrebbero essere di altri programmi)", base)
	}
	if in.TogliAttivita != nil {
		if err := in.TogliAttivita(); err != nil {
			Diario("attività pianificate di Tasso non tolte: %v", err)
			ris.NonTolti = append(ris.NonTolti, `Utilità di pianificazione \Tasso\`)
		}
	}
	if r := in.Registro; r != nil {
		for _, k := range ChiaviSoloTasso {
			_ = r.TogliAlbero(k.Radice, k.Chiave)
		}
	}
	if od.TogliDati && in.CartellaDatiUtente != nil {
		if radice := in.CartellaDatiUtente(); radice != "" && filepath.IsAbs(radice) {
			for _, n := range CartelleDatiTasso {
				in.togliDatiTenendo(filepath.Join(radice, n), CartelleDaTenere[n], ris)
			}
		}
	}
}

// togliDatiTenendo toglie la cartella dei dati d, ma le sue sottocartelle tieni che hanno dentro
// qualcosa restano (vedi CartelleDaTenere). Se non ce n'è nessuna, via la cartella intera.
func (in *Installatore) togliDatiTenendo(d string, tieni []string, ris *Risultato) {
	var tenute []string
	for _, t := range tieni {
		if voci, err := os.ReadDir(filepath.Join(d, t)); err == nil && len(voci) > 0 {
			tenute = append(tenute, t)
		}
	}
	if len(tenute) == 0 {
		in.togliCartellaORiavvio(d, ris)
		return
	}
	Diario("%s: tengo %v (originali dei giochi o copie del registro che Tasso non ha messo in Documenti)", d, tenute)
	for _, t := range tenute {
		ris.DatiTenuti = append(ris.DatiTenuti, filepath.Join(d, t))
	}
	voci, err := os.ReadDir(d)
	if err != nil {
		return
	}
	for _, v := range voci {
		tenuta := false
		for _, t := range tenute {
			tenuta = tenuta || strings.EqualFold(v.Name(), t)
		}
		if tenuta {
			continue
		}
		p := filepath.Join(d, v.Name())
		if v.IsDir() {
			in.togliCartellaORiavvio(p, ris)
		} else {
			in.togliORiavvio(p, ris)
		}
	}
}

// togliCartelleVuote toglie le cartelle vuote da dir fino a fine (esclusa).
func togliCartelleVuote(dir, fine string) {
	for dir != fine && strings.HasPrefix(dir, fine) {
		if os.Remove(dir) != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

// togliRegistro toglie quello che l'installatore ha scritto nel registro.
func (in *Installatore) togliRegistro() error {
	r := in.Registro
	if r == nil {
		return nil
	}
	if err := r.TogliChiave(HKLM, ChiaveDisinstalla); err != nil {
		return err
	}
	for _, n := range []string{"Cartella", "Componenti", "Modalita", "Versione"} {
		_ = r.TogliValore(HKLM, ChiaveTasso, n)
	}
	if r.Vuota(HKLM, ChiaveTasso) {
		_ = r.TogliChiave(HKLM, ChiaveTasso)
	}
	// HKCU\Software\Tasso\Lingua: l'ha scritta l'installatore, va via con l'ultimo programma.
	// Le chiavi di Tasso stesso (ChiaviSoloTasso) le toglie togliCoseDiTasso; le altre restano.
	for _, rr := range []Registro{r, in.RegistroUtente} {
		if rr == nil {
			continue
		}
		_ = rr.TogliValore(HKCU, ChiaveTasso, "Lingua")
		if rr.Vuota(HKCU, ChiaveTasso) {
			_ = rr.TogliChiave(HKCU, ChiaveTasso)
		}
	}
	return nil
}

// Installato legge dal registro dov'è installato Tasso (per l'aggiornamento).
func Installato(r Registro) (string, bool) {
	if r == nil {
		return "", false
	}
	if c, ok := r.LeggiTesto(HKLM, ChiaveTasso, "Cartella"); ok && c != "" {
		return c, true
	}
	if c, ok := r.LeggiTesto(HKLM, ChiaveDisinstalla, "InstallLocation"); ok && c != "" {
		return c, true
	}
	return "", false
}

func copiaStato(st *Stato) *Stato {
	c := *st
	c.Programmi = map[Programma]StatoProg{}
	for k, v := range st.Programmi {
		c.Programmi[k] = v
	}
	return &c
}
