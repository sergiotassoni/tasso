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
}

// OpzioniDisinstalla: scelte della pagina "Disinstalla".
type OpzioniDisinstalla struct {
	// TogliDati: via anche i dati di Tasso in %LOCALAPPDATA% (Tasso e TassoTraduce: impostazioni,
	// memoria delle traduzioni, modelli scaricati, Portachiavi). Solo se l'utente lo conferma.
	TogliDati bool
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

// Disinstalla toglie i componenti scelti, e solo i file scritti in tasso-setup.json.
//   - Gaming senza Ufficio (o il contrario): Tasso.exe resta, cambia solo la modalità.
//   - Gaming e Ufficio insieme (o l'unico dei due rimasto): via Tasso.exe e il suo collegamento.
//     La cartella "lavoro" e le impostazioni di Tasso non si toccano: sono di Tasso
//     (le toglie "Togli Tasso dal PC" dentro Tasso).
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
	progPrima := ProgrammiPer(st.Componenti)
	progDopo := map[Programma]bool{}
	for _, p := range ProgrammiPer(restano) {
		progDopo[p] = true
	}
	tutto := len(restano) == 0
	ris := &Risultato{}

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
	if viaTasso && in.PulisciTasso != nil {
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
			in.togliCoseDiTasso(base, od, ris)
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
		_ = os.Remove(base) // solo se vuota (es. resta la cartella "lavoro" di Tasso: non è mia)
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
	if in.CancellaAlRiavvio != nil {
		in.cancellaAlRiavvio(d)
		Diario("%s non tolta del tutto: il resto si cancella al prossimo riavvio", d)
		ris.AlRiavvio = append(ris.AlRiavvio, d)
		return
	}
	ris.NonTolti = append(ris.NonTolti, d)
}

var errOccupata = errors.New("qualcosa dentro è ancora aperto")

// togliCoseDiTasso: dopo Tasso.exe, le cose che Tasso ha messo da solo: le sue cartelle di lavoro,
// le attività pianificate \Tasso\, le sue chiavi del registro e (se confermato) i suoi dati.
func (in *Installatore) togliCoseDiTasso(base string, od OpzioniDisinstalla, ris *Risultato) {
	for _, d := range SottocartelleTasso {
		in.togliCartellaORiavvio(filepath.Join(base, d), ris)
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
				in.togliCartellaORiavvio(filepath.Join(radice, n), ris)
			}
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
	// Le altre cose di Tasso in HKCU\Software\Tasso (AudioUSB...) restano.
	_ = r.TogliValore(HKCU, ChiaveTasso, "Lingua")
	if r.Vuota(HKCU, ChiaveTasso) {
		_ = r.TogliChiave(HKCU, ChiaveTasso)
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
