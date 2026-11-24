// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Limiti contro le "zip bomb" (archivi piccoli che diventano enormi una volta estratti).
type Limiti struct {
	// Rapporto massimo tra dati estratti e dimensione dell'archivio...
	MaxRapporto int64
	// ...che conta solo oltre questa quantità di dati (sotto, qualunque rapporto va bene).
	SogliaRapporto int64
	// Massimo assoluto di dati estratti (0 = nessun massimo oltre al rapporto e al disco).
	MaxTotale int64
	// Numero massimo di voci (file e cartelle) nell'archivio.
	MaxVoci int
	// Spazio da lasciare sempre libero sul disco.
	MargineDisco int64
}

// LimitiPredefiniti: valori pensati per non bloccare mai un archivio normale.
// Un archivio di 10 MB può dare fino a 10 GB (rapporto 1000), e comunque fino a 8 GB
// qualunque sia il rapporto; mai più di 2 TB in tutto; sul disco restano sempre 512 MB liberi.
func LimitiPredefiniti() Limiti {
	return Limiti{
		MaxRapporto:    1000,
		SogliaRapporto: 8 << 30,
		MaxTotale:      2 << 40,
		MaxVoci:        1_000_000,
		MargineDisco:   512 << 20,
	}
}

// consentito: quanti byte si possono estrarre da un archivio grande dimArchivio.
func (l Limiti) consentito(dimArchivio int64) int64 {
	max := l.SogliaRapporto
	if l.MaxRapporto > 0 && dimArchivio > 0 && dimArchivio < (1<<62)/l.MaxRapporto {
		if r := dimArchivio * l.MaxRapporto; r > max {
			max = r
		}
	}
	if l.MaxTotale > 0 && max > l.MaxTotale {
		max = l.MaxTotale
	}
	if max <= 0 {
		max = 1 << 62
	}
	return max
}

// Avanzamento: a che punto è il lavoro.
type Avanzamento struct {
	Fatto  int64  // byte fatti
	Totale int64  // byte in tutto (0 se non si sa)
	Nome   string // file su cui sta lavorando
}

// FunzioneAvanzamento riceve l'avanzamento (spesso: chi la usa deve rallentare gli aggiornamenti).
type FunzioneAvanzamento func(Avanzamento)

// nomeTemporaneo: nome della cartella o del file di lavoro, accanto al risultato.
func nomeTemporaneo(ext string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return ".tassozip-" + hex.EncodeToString(b) + ext
}

// creaCartellaTemporanea crea la cartella di lavoro dentro "dove".
func creaCartellaTemporanea(dove string) (string, error) {
	for i := 0; i < 20; i++ {
		p := filepath.Join(dove, nomeTemporaneo(".tmp"))
		err := os.Mkdir(p, 0o755)
		if err == nil {
			return p, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", err
		}
	}
	return "", errors.New("non riesco a creare la cartella di lavoro")
}

// spostaConNomeLibero sposta "da" nella cartella "dove" con il nome "nome", oppure
// "nome (2)", "nome (3)"... se esiste già. Non sovrascrive mai. Restituisce il percorso finale.
func spostaConNomeLibero(da, dove, nome string, cartella bool) (string, error) {
	for n := 1; n < 100000; n++ {
		a := filepath.Join(dove, NomeNumero(nome, cartella, n))
		err := rinominaSeLibero(da, a)
		if err == nil {
			return a, nil
		}
		if !errors.Is(err, errEsisteGia) {
			return "", err
		}
	}
	return "", errors.New("troppi file con lo stesso nome")
}

// destinazione: la cartella di lavoro in cui si estraggono i file, con tutti i controlli.
type destinazione struct {
	ctx         context.Context
	radice      string // cartella di lavoro (nuova e vuota)
	limiti      Limiti
	consentito  int64
	scritti     int64
	voci        int
	file        int
	rinominati  int
	saltati     []string
	cartelle    map[string]string // percorso nell'archivio (chiave) -> percorso vero relativo
	avanzamento func(nome string, n int64)
	prossimoCtl int64 // quando ricontrollare lo spazio libero
	buf         []byte
	// filtro: nil = tutto; altrimenti dice se la voce va estratta e con che nome.
	filtro func(nome string) (string, bool)
	ultimo string // percorso relativo (alla radice) dell'ultimo file scritto
}

// NormalizzaNome: il nome di una voce come lo mostra Tasso Zip ("/" come separatore,
// niente "/" all'inizio o alla fine, niente pezzi "." o vuoti).
func NormalizzaNome(n string) string {
	n = strings.ReplaceAll(n, `\`, "/")
	var pezzi []string
	for _, p := range strings.Split(n, "/") {
		if p == "" || p == "." {
			continue
		}
		pezzi = append(pezzi, p)
	}
	return strings.Join(pezzi, "/")
}

// filtroSelezione: solo le voci scelte (e quello che c'è dentro le cartelle scelte),
// togliendo il prefisso indicato.
func filtroSelezione(sel []string, prefisso string) func(string) (string, bool) {
	prefisso = NormalizzaNome(prefisso)
	if len(sel) == 0 && prefisso == "" {
		return nil
	}
	scelti := make([]string, 0, len(sel))
	for _, x := range sel {
		if x = NormalizzaNome(x); x != "" {
			scelti = append(scelti, x)
		}
	}
	return func(nome string) (string, bool) {
		n := NormalizzaNome(nome)
		if len(scelti) > 0 {
			ok := false
			for _, x := range scelti {
				if n == x || strings.HasPrefix(n, x+"/") {
					ok = true
					break
				}
			}
			if !ok {
				return "", false
			}
		}
		if prefisso != "" {
			if !strings.HasPrefix(n, prefisso+"/") {
				return "", false
			}
			n = n[len(prefisso)+1:]
		}
		return n, n != ""
	}
}

// vuole: la voce va estratta? (per non aprire nemmeno quelle da saltare)
func (d *destinazione) vuole(nome string) bool {
	if d.filtro == nil {
		return true
	}
	_, ok := d.filtro(nome)
	return ok
}

func nuovaDestinazione(ctx context.Context, radice string, limiti Limiti, dimArchivio int64) *destinazione {
	return &destinazione{
		ctx:        ctx,
		radice:     radice,
		limiti:     limiti,
		consentito: limiti.consentito(dimArchivio),
		cartelle:   map[string]string{},
		buf:        make([]byte, 256<<10),
	}
}

func chiave(s string) string {
	if suWindows {
		return strings.ToLower(s) // su Windows "Foto" e "foto" sono la stessa cartella
	}
	return s
}

// controllaDichiarato: prima di cominciare, se l'archivio dichiara già troppi dati, mi fermo subito.
func (d *destinazione) controllaDichiarato(totale int64, voci int) error {
	if d.limiti.MaxVoci > 0 && voci > d.limiti.MaxVoci {
		return ErrTroppeVoci
	}
	if totale > d.consentito {
		return ErrBomba
	}
	if lib := spazioLibero(d.radice); lib >= 0 && totale > lib-d.limiti.MargineDisco {
		return fmt.Errorf("%w: servono %d MB", ErrSpazio, totale>>20)
	}
	return nil
}

func (d *destinazione) annullato() error {
	select {
	case <-d.ctx.Done():
		return ErrAnnullato
	default:
		return nil
	}
}

func (d *destinazione) nuovaVoce() error {
	d.voci++
	if d.limiti.MaxVoci > 0 && d.voci > d.limiti.MaxVoci {
		return ErrTroppeVoci
	}
	return d.annullato()
}

// salta: una voce non estratta (collegamenti, nomi pericolosi...), con il motivo.
func (d *destinazione) salta(nome, motivo string) {
	if len(d.saltati) < 1000 {
		d.saltati = append(d.saltati, nome+" ("+motivo+")")
	}
}

// cartella crea (se serve) la cartella dell'archivio fatta di questi pezzi e ne restituisce
// il percorso vero relativo. Se al posto di una cartella c'è già un file con quel nome,
// la cartella prende un nome nuovo "(2)" e tutte le voci dentro la seguono.
func (d *destinazione) cartella(pezzi []string) (string, error) {
	vero := ""
	arch := ""
	for _, p := range pezzi {
		if arch == "" {
			arch = p
		} else {
			arch = arch + "/" + p
		}
		if v, ok := d.cartelle[chiave(arch)]; ok {
			vero = v
			continue
		}
		var scelto string
		for n := 1; ; n++ {
			if n > 10000 {
				return "", errors.New("troppi nomi uguali")
			}
			cand := filepath.Join(vero, NomeNumero(p, true, n))
			full := filepath.Join(d.radice, cand)
			err := os.Mkdir(full, 0o755)
			if err == nil {
				scelto = cand
				if n > 1 {
					d.rinominati++
				}
				break
			}
			if errors.Is(err, fs.ErrExist) {
				fi, e := os.Lstat(full)
				if e == nil && fi.IsDir() && fi.Mode()&fs.ModeSymlink == 0 {
					scelto = cand // la cartella c'è già (creata da noi): va bene
					break
				}
				continue // c'è un file con quel nome: provo "(2)"
			}
			return "", err
		}
		d.cartelle[chiave(arch)] = scelto
		vero = scelto
	}
	return vero, nil
}

// creaCartella: voce "cartella" dell'archivio.
func (d *destinazione) creaCartella(nome string, quando time.Time) error {
	if d.filtro != nil {
		n, ok := d.filtro(nome)
		if !ok {
			return d.annullato()
		}
		nome = n
	}
	if err := d.nuovaVoce(); err != nil {
		return err
	}
	pezzi, ok := NomeSicuro(nome)
	if !ok {
		d.salta(nome, "percorso non sicuro")
		return nil
	}
	_, err := d.cartella(pezzi)
	return err
}

// creaFile: voce "file" dell'archivio. Non sovrascrive mai: se il nome c'è già usa "(2)".
func (d *destinazione) creaFile(nome string, r io.Reader, quando time.Time) error {
	if d.filtro != nil {
		n, ok := d.filtro(nome)
		if !ok {
			return d.annullato()
		}
		nome = n
	}
	if err := d.nuovaVoce(); err != nil {
		return err
	}
	pezzi, ok := NomeSicuro(nome)
	if !ok {
		d.salta(nome, "percorso non sicuro")
		return nil
	}
	dir, err := d.cartella(pezzi[:len(pezzi)-1])
	if err != nil {
		return err
	}
	ultimo := pezzi[len(pezzi)-1]
	var f *os.File
	var percorso string
	for n := 1; ; n++ {
		if n > 10000 {
			return errors.New("troppi nomi uguali")
		}
		percorso = filepath.Join(d.radice, dir, NomeNumero(ultimo, false, n))
		f, err = os.OpenFile(percorso, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			if n > 1 {
				d.rinominati++
			}
			break
		}
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return err
	}
	_, errCopia := d.copia(f, r, nome)
	errChiudi := f.Close()
	if errCopia != nil {
		return errCopia
	}
	if errChiudi != nil {
		return errChiudi
	}
	if !quando.IsZero() && quando.Year() > 1980 && quando.Year() < 2200 {
		_ = os.Chtimes(percorso, quando, quando)
	}
	if rel, e := filepath.Rel(d.radice, percorso); e == nil {
		d.ultimo = rel
	}
	d.file++
	return nil
}

// copia scrive i dati controllando: annulla, limite contro le zip bomb e spazio libero.
func (d *destinazione) copia(w io.Writer, r io.Reader, nome string) (int64, error) {
	var tot int64
	for {
		if err := d.annullato(); err != nil {
			return tot, err
		}
		n, errL := r.Read(d.buf)
		if n > 0 {
			d.scritti += int64(n)
			if d.scritti > d.consentito {
				return tot, ErrBomba
			}
			if d.scritti >= d.prossimoCtl {
				d.prossimoCtl = d.scritti + 64<<20
				if lib := spazioLibero(d.radice); lib >= 0 && lib < d.limiti.MargineDisco {
					return tot, ErrSpazio
				}
			}
			if _, errS := w.Write(d.buf[:n]); errS != nil {
				return tot, errS
			}
			tot += int64(n)
			if d.avanzamento != nil {
				d.avanzamento(nome, int64(n))
			}
		}
		if errL == io.EOF {
			return tot, nil
		}
		if errL != nil {
			return tot, errL
		}
	}
}

// concludi sposta il risultato dalla cartella di lavoro alla cartella finale.
// Restituisce la cartella (o, in ModoQui, la cartella di destinazione) e il percorso
// finale dell'ultimo file scritto.
//
//	ModoNormale:  se dentro c'è una sola cartella, uso quella (niente "Foto\Foto\...");
//	ModoCartella: cartella col nome dell'archivio (se dentro c'è solo una cartella con
//	              lo stesso nome, uso quella);
//	ModoQui:      ogni elemento va direttamente in "dove" (mai sovrascrivere: "(2)").
func (d *destinazione) concludi(dove, nomeBase string, modo ModoEstrai) (string, string, error) {
	voci, err := os.ReadDir(d.radice)
	if err != nil {
		return "", "", err
	}
	primo, resto := "", ""
	if d.ultimo != "" {
		pz := strings.SplitN(d.ultimo, string(filepath.Separator), 2)
		primo = pz[0]
		if len(pz) == 2 {
			resto = pz[1]
		}
	}
	unaCartella := len(voci) == 1 && voci[0].IsDir() && voci[0].Type()&fs.ModeSymlink == 0
	if modo == ModoQui {
		ultimo := ""
		for _, v := range voci {
			fin, err := spostaConNomeLibero(filepath.Join(d.radice, v.Name()), dove, v.Name(), v.IsDir())
			if err != nil {
				return "", "", err
			}
			if v.Name() == primo {
				ultimo = filepath.Join(fin, resto)
			}
		}
		_ = os.Remove(d.radice)
		return dove, ultimo, nil
	}
	if unaCartella && (modo == ModoNormale || strings.EqualFold(voci[0].Name(), nomeBase)) {
		dentro := filepath.Join(d.radice, voci[0].Name())
		finale, err := spostaConNomeLibero(dentro, dove, voci[0].Name(), true)
		if err != nil {
			return "", "", err
		}
		_ = os.Remove(d.radice) // ora è vuota
		ultimo := ""
		if d.ultimo != "" {
			ultimo = filepath.Join(finale, resto)
		}
		return finale, ultimo, nil
	}
	finale, err := spostaConNomeLibero(d.radice, dove, nomeBase, true)
	if err != nil {
		return "", "", err
	}
	ultimo := ""
	if d.ultimo != "" {
		ultimo = filepath.Join(finale, d.ultimo)
	}
	return finale, ultimo, nil
}
