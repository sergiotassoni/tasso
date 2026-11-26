// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Tipo di archivio.
type Tipo string

const (
	TipoZip Tipo = "zip"
	Tipo7z  Tipo = "7z"
	TipoRar Tipo = "rar"
	TipoTar Tipo = "tar"
	TipoGz  Tipo = "gz"
	TipoBz2 Tipo = "bz2"
	TipoXz  Tipo = "xz"
	TipoZst Tipo = "zst"
)

// estensioniArchivio: per capire al volo (senza aprire il file) se è un archivio.
var estensioniArchivio = []string{
	".zip", ".7z", ".rar", ".tar", ".gz", ".tgz", ".bz2", ".tbz2", ".tbz", ".xz", ".txz", ".zst", ".tzst",
}

// SembraArchivio: true se dal nome il file sembra un archivio che Tasso Zip sa aprire.
func SembraArchivio(percorso string) bool {
	n := minuscolo(percorso)
	if m := reNumero3.FindStringSubmatch(n); m != nil {
		return true // nome.001, nome.7z.002...
	}
	if reVecchiaRar.MatchString(n) {
		return true // nome.r00
	}
	for _, e := range estensioniArchivio {
		if strings.HasSuffix(n, e) {
			return true
		}
	}
	return false
}

// riconosciDati capisce il tipo dai primi byte.
func riconosciDati(b []byte) (Tipo, bool) {
	switch {
	case bytes.HasPrefix(b, []byte("7z\xBC\xAF\x27\x1C")):
		return Tipo7z, true
	case bytes.HasPrefix(b, []byte("Rar!\x1A\x07")):
		return TipoRar, true
	case bytes.HasPrefix(b, []byte("PK\x03\x04")), bytes.HasPrefix(b, []byte("PK\x05\x06")):
		return TipoZip, true
	case bytes.HasPrefix(b, []byte{0x1F, 0x8B}):
		return TipoGz, true
	case bytes.HasPrefix(b, []byte("BZh")):
		return TipoBz2, true
	case bytes.HasPrefix(b, []byte{0xFD, '7', 'z', 'X', 'Z', 0x00}):
		return TipoXz, true
	case bytes.HasPrefix(b, []byte{0x28, 0xB5, 0x2F, 0xFD}):
		return TipoZst, true
	case sembraTar(b):
		return TipoTar, true
	}
	return "", false
}

// sembraTar: intestazione tar valida (con "ustar" o con la somma di controllo giusta).
func sembraTar(b []byte) bool {
	if len(b) < 512 {
		return false
	}
	if bytes.Equal(b[257:262], []byte("ustar")) {
		return true
	}
	// vecchi tar senza "ustar": controllo la somma
	campo := strings.Trim(string(b[148:156]), " \x00")
	if campo == "" {
		return false
	}
	var atteso int64
	for _, c := range campo {
		if c < '0' || c > '7' {
			return false
		}
		atteso = atteso*8 + int64(c-'0')
	}
	var somma int64
	for i := 0; i < 512; i++ {
		if i >= 148 && i < 156 {
			somma += ' '
		} else {
			somma += int64(b[i])
		}
	}
	return somma == atteso && b[0] != 0
}

// OpzioniEstrai: come estrarre.
type OpzioniEstrai struct {
	Password        string
	Limiti          *Limiti // nil = LimitiPredefiniti()
	Avanzamento     FunzioneAvanzamento
	CartellaRiserva string // dove estrarre se accanto all'archivio non si può scrivere (es. Desktop)

	// Modo: dove finiscono i file.
	//   ModoNormale (""): cartella nuova accanto all'archivio (se dentro c'è una sola
	//                     cartella, si usa quella);
	//   ModoQui:          direttamente nella cartella (i nomi già presenti diventano "(2)");
	//   ModoCartella:     sempre in una cartella col nome dell'archivio.
	Modo ModoEstrai
	// Destinazione: la cartella in cui estrarre ("" = quella dell'archivio).
	Destinazione string
	// Selezione: solo queste voci (nomi come li dà Elenca; una cartella vale con tutto
	// quello che contiene). Vuota = tutto.
	Selezione []string
	// TogliPrefisso: cartella dell'archivio da togliere davanti ai nomi (es. "Foto/2024":
	// "Foto/2024/mare.jpg" esce come "mare.jpg").
	TogliPrefisso string
}

// ModoEstrai: dove finiscono i file estratti.
type ModoEstrai string

const (
	ModoNormale  ModoEstrai = ""
	ModoQui      ModoEstrai = "qui"
	ModoCartella ModoEstrai = "cartella"
)

// RisultatoEstrai: cosa è stato fatto.
type RisultatoEstrai struct {
	Cartella   string   // la cartella nuova con i file
	File       int      // file estratti
	Rinominati int      // nomi doppi a cui ho aggiunto " (2)"
	Saltati    []string // voci non estratte, con il motivo (collegamenti, nomi pericolosi...)
	Falliti    []string // file con i dati rovinati, non estratti (gli altri sì)
	// SommaSbagliata: tar compresso con la somma di controllo finale sbagliata: i file sono
	// usciti, ma qualcuno potrebbe avere dati sbagliati (non si sa quale).
	SommaSbagliata bool
	Riserva        bool // true se ho usato CartellaRiserva
	// UltimoFile: percorso finale dell'ultimo file estratto (per aprirlo subito).
	UltimoFile string
}

// lavoroEstrai: tutto quello che serve durante un'estrazione.
type lavoroEstrai struct {
	ctx      context.Context
	opz      OpzioniEstrai
	parti    []string
	vol      *volumi
	d        *destinazione
	fatto    int64
	totale   int64
	flusso   bool // avanzamento in base ai byte letti dall'archivio (tar, gz, bz2, xz)
	multipli bool
	// cifratiBuoni: file cifrati già usciti giusti (allora la password è giusta e un errore
	// dopo vuol dire file rovinato, non password sbagliata).
	cifratiBuoni int
	// sommaSbagliata: la somma di controllo finale del flusso compresso non torna.
	sommaSbagliata bool
}

func (l *lavoroEstrai) avanza(nome string, n int64) {
	if l.opz.Avanzamento == nil {
		return
	}
	l.fatto += n
	a := Avanzamento{Fatto: l.fatto, Totale: l.totale, Nome: nome}
	if l.flusso && l.vol != nil {
		a.Fatto = l.vol.letti.Load()
		a.Totale = l.vol.Size()
	}
	l.opz.Avanzamento(a)
}

// Estrai estrae l'archivio in una cartella NUOVA accanto all'archivio.
// Lavora in una cartella temporanea e la rinomina solo alla fine: se annulli o qualcosa
// va storto non resta niente a metà.
func Estrai(ctx context.Context, percorso string, opz OpzioniEstrai) (ris RisultatoEstrai, err error) {
	percorso, err = filepath.Abs(percorso)
	if err != nil {
		return ris, err
	}
	fi, err := os.Stat(percorso)
	if err != nil {
		return ris, err
	}
	if !fi.Mode().IsRegular() {
		return ris, ErrFormato
	}
	limiti := LimitiPredefiniti()
	if opz.Limiti != nil {
		limiti = *opz.Limiti
	}
	l := &lavoroEstrai{ctx: ctx, opz: opz}
	percorso, tipo, err := l.apri(percorso)
	if err != nil {
		return ris, err
	}
	defer l.vol.Close()

	// Cartella di lavoro accanto all'archivio (o nella cartella di riserva).
	dove := filepath.Dir(l.parti[0])
	if opz.Destinazione != "" {
		dove = opz.Destinazione
	}
	PulisciAvanzi(dove)
	tmp, err := creaCartellaTemporanea(dove)
	if err != nil && opz.CartellaRiserva != "" {
		dove = opz.CartellaRiserva
		ris.Riserva = true
		PulisciAvanzi(dove)
		tmp, err = creaCartellaTemporanea(dove)
	}
	if err != nil {
		return ris, fmt.Errorf("%w: %v", ErrNonScrivibile, err)
	}
	sblocca := bloccaLavoro(tmp)
	finito := false
	defer func() {
		sblocca()
		if !finito {
			_ = os.RemoveAll(tmp) // annullato o errore: non lascio niente a metà
		}
	}()

	l.d = nuovaDestinazione(ctx, tmp, limiti, l.vol.Size())
	l.d.zona = zonaArchivio(l.parti)
	l.d.avanzamento = l.avanza
	l.d.filtro = filtroSelezione(opz.Selezione, opz.TogliPrefisso)

	switch tipo {
	case TipoZip:
		err = l.zip()
	case Tipo7z:
		err = l.setteZip()
	case TipoRar:
		err = l.rar(percorso)
	case TipoTar, TipoGz, TipoBz2, TipoXz, TipoZst:
		l.flusso = true
		err = l.flussoTar(tipo, percorso)
	default:
		err = ErrFormato
	}
	if err == nil {
		err = l.d.annullato()
	}
	if err == nil && len(l.d.falliti) > 0 && l.d.file == 0 {
		err = l.erroreLettura(l.d.primoErrore) // nessun file buono: l'archivio è rovinato
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			err = ErrAnnullato
		}
		return ris, err
	}

	sblocca()
	finale, ultimo, err := l.d.concludi(dove, NomeBaseArchivio(percorso), opz.Modo)
	if err != nil {
		return ris, err
	}
	finito = true
	ris.Cartella = finale
	ris.UltimoFile = ultimo
	ris.File = l.d.file
	ris.Rinominati = l.d.rinominati
	ris.Saltati = l.d.saltati
	ris.Falliti = l.d.falliti
	ris.SommaSbagliata = l.sommaSbagliata
	return ris, nil
}

// soloQuesto: l'errore riguarda solo i dati di questo file (rovinati): si salta il file e si
// va avanti con gli altri. Annullo, password, memoria, spazio, disco che non scrive, zip bomb,
// troppe voci: si ferma tutto. Nella Verifica ogni errore ferma (deve dire "rovinato").
func (l *lavoroEstrai) soloQuesto(err error) bool {
	if err == nil || l.d == nil || l.d.prova {
		return false
	}
	for _, e := range []error{ErrAnnullato, context.Canceled, ErrBomba, ErrSpazio, ErrTroppeVoci, ErrPassword,
		ErrPasswordSbagliata, ErrZipCifrato, ErrMemoria, ErrParteMancante} {
		if errors.Is(err, e) {
			return false
		}
	}
	if erroreMemoria(err) || erroreDiscoPieno(err) {
		return false
	}
	if l.multipli && errors.Is(err, io.ErrUnexpectedEOF) {
		return false // manca una parte: lo dice ErrParteMancante
	}
	var pe *fs.PathError
	if errors.As(err, &pe) && pe.Op != "read" {
		return false // errore del disco su cui scrivo
	}
	return true
}

// sommaFinale: finito il tar, la somma di controllo del compressore (in fondo al flusso) non
// torna: qualche file già estratto può avere i dati sbagliati, ma non si sa quale. Nella
// Verifica è un errore ("rovinato"); estraendo i file restano e alla fine lo dico chiaramente
// (RisultatoEstrai.SommaSbagliata: avviso nella finestra).
func (l *lavoroEstrai) sommaFinale(err error) error {
	if !l.soloQuesto(err) || l.d.file == 0 {
		return err
	}
	if l.d.primoErrore == nil {
		l.d.primoErrore = err
	}
	l.sommaSbagliata = true
	return nil
}

// resto: dopo un errore la lettura dell'archivio non può andare avanti. Se qualche file è
// già uscito buono lo tengo e segno che il resto non si è potuto leggere; se no è un errore.
func (l *lavoroEstrai) resto(err error) error {
	if !l.soloQuesto(err) || l.d.file == 0 {
		return err
	}
	if l.d.primoErrore == nil {
		l.d.primoErrore = err
	}
	l.d.falliti = append(l.d.falliti, "… (resto_illeggibile)")
	return nil
}

// apri trova le parti dell'archivio, le apre e ne riconosce il tipo dai primi byte.
// Restituisce il percorso da usare (per i rar divisi: la prima parte).
func (l *lavoroEstrai) apri(percorso string) (string, Tipo, error) {
	n := minuscolo(percorso)
	if strings.HasSuffix(n, ".z01") || (strings.HasSuffix(n, ".zip") && esisteParteZip(percorso)) {
		return "", "", ErrZipDiviso
	}

	// Le parti: "nome.part1.rar"... (le apre la libreria rar), oppure "nome.001", "nome.002"...
	tipo := Tipo("")
	if reParteRarN.MatchString(filepath.Base(percorso)) || reVecchiaRar.MatchString(n) {
		p, e := primaParteRar(percorso)
		if e != nil {
			return "", "", e
		}
		percorso, tipo = p, TipoRar
		l.parti = []string{p}
	} else {
		parti, multi, e := partiNumerate(percorso)
		if e != nil {
			return "", "", e
		}
		l.parti, l.multipli = parti, multi
	}

	var err error
	l.vol, err = apriVolumi(l.parti)
	if err != nil {
		return "", "", err
	}

	if tipo == "" {
		testa := make([]byte, 1024)
		k, _ := l.vol.ReadAt(testa, 0)
		testa = testa[:k]
		var ok bool
		tipo, ok = riconosciDati(testa)
		if !ok {
			if bytes.HasPrefix(testa, []byte("PK\x07\x08")) {
				l.vol.Close()
				return "", "", ErrZipDiviso
			}
			// zip con qualcosa davanti (es. autoestraente): lo riconosce la lettura dalla fine
			if _, e := zip.NewReader(l.vol, l.vol.Size()); e == nil || errors.Is(e, zip.ErrInsecurePath) {
				tipo = TipoZip
			} else {
				l.vol.Close()
				return "", "", ErrFormato
			}
		}
	}
	l.vol.letti.Store(0)
	return percorso, tipo, nil
}

// esisteParteZip: c'è "nome.z01" accanto a "nome.zip"? Allora è uno zip diviso.
func esisteParteZip(percorso string) bool {
	base := strings.TrimSuffix(percorso, filepath.Ext(percorso))
	for _, e := range []string{".z01", ".Z01"} {
		if _, err := os.Stat(base + e); err == nil {
			return true
		}
	}
	return false
}

// erroreLettura: trasforma gli errori di lettura in messaggi chiari.
func (l *lavoroEstrai) erroreLettura(err error) error {
	if err == nil {
		return nil
	}
	for _, e := range []error{ErrAnnullato, ErrBomba, ErrSpazio, ErrTroppeVoci, ErrPassword, ErrPasswordSbagliata, ErrZipCifrato, ErrMemoria} {
		if errors.Is(err, e) {
			return err
		}
	}
	if erroreMemoria(err) {
		return fmt.Errorf("%w (%v)", ErrMemoria, err)
	}
	if errors.Is(err, context.Canceled) {
		return ErrAnnullato
	}
	var pe *fs.PathError
	if errors.As(err, &pe) && pe.Op != "read" {
		return err // errore del disco su cui scrivo (permessi, disco pieno...)
	}
	if l.multipli {
		return fmt.Errorf("%w (%v)", ErrParteMancante, err)
	}
	return fmt.Errorf("%w (%v)", ErrRovinato, err)
}
