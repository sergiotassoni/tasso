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
}

// Disinstalla toglie i componenti scelti, e solo i file scritti in tasso-setup.json.
//   - Gaming senza Ufficio (o il contrario): Tasso.exe resta, cambia solo la modalità.
//   - Gaming e Ufficio insieme (o l'unico dei due rimasto): via Tasso.exe e il suo collegamento.
//     La cartella "lavoro" e le impostazioni di Tasso non si toccano: sono di Tasso
//     (le toglie "Togli Tasso dal PC" dentro Tasso).
//   - Zip, PDF: via i loro file, i collegamenti e la loro cartella se resta vuota.
//   - Se non resta niente: via anche disinstallatore, tasso-setup.json, le voci
//     del registro dell'installatore e la cartella se è vuota.
func (in *Installatore) Disinstalla(base string, togli []Componente) (*Risultato, error) {
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
			if err := in.togli(fp); err != nil && !os.IsNotExist(err) {
				ris.NonTolti = append(ris.NonTolti, fp)
			}
			togliCartelleVuote(filepath.Dir(fp), dir)
		}
		if dir != base {
			_ = os.Remove(dir) // solo se vuota
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
