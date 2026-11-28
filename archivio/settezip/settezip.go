// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

// Package settezip scrive archivi .7z (formato descritto in 7zFormat.txt di 7-Zip,
// di pubblico dominio) con il metodo LZMA2. Scritto da noi: le librerie Go libere
// sanno solo leggere i .7z.
//
// Cosa scrive: blocchi solidi (tanti file in un flusso solo), compressi anche in
// parallelo su più core; intestazione compressa; nomi UTF-16; data di modifica;
// attributi di Windows; cartelle vuote e file vuoti; CRC32 di ogni file.
package settezip

import (
	"bufio"
	"compress/flate"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"runtime"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/ulikunitz/xz/lzma"
)

// Livello di compressione.
type Livello int

const (
	Veloce  Livello = 1
	Normale Livello = 2
	Massima Livello = 3
	Ultra   Livello = 4
)

// Voce: un file o una cartella da mettere nell'archivio.
type Voce struct {
	Nome      string // con "/" come separatore
	Cartella  bool
	Dim       int64 // dimensione prevista (0 = file vuoto)
	Quando    time.Time
	Attributi uint32 // attributi di Windows; 0 = automatici
	// GiaCompresso: foto, video, archivi... non si comprimono più: vanno in un blocco a
	// parte, messi così come sono (sempre LZMA2, con i "pezzi non compressi" previsti dal
	// formato), senza perdere tempo.
	GiaCompresso bool
	// Apri apre il contenuto (solo per i file con Dim > 0).
	Apri func() (io.ReadCloser, error)
}

// Opzioni di scrittura.
type Opzioni struct {
	Livello    Livello
	Dizionario int   // byte; 0 = secondo il livello e la RAM libera
	Thread     int   // 0 = secondo i core e la RAM
	Blocco     int64 // dimensione di un blocco solido; 0 = secondo il livello; <0 = non solido (un file per blocco)
	Segmento   int64 // byte compressi da un core per volta; 0 = DimSegmento(dizionario)
	RAMLibera  int64 // byte di RAM libera (0 = non si sa)
	DirTemp    string
	// Progresso riceve i byte letti (può essere chiamata da più goroutine insieme).
	Progresso func(n int64, nome string)
	// Saltato: un file non leggibile (non entra nell'archivio).
	Saltato func(nome string, err error)
}

// Risultato della scrittura.
type Risultato struct {
	File, Cartelle int
	Originale      int64
	Blocchi        int
	Dizionario     int
	Thread         int
	Copiati        int64 // byte messi senza comprimerli (già compressi)
}

const (
	kEnd                  = 0x00
	kHeader               = 0x01
	kMainStreamsInfo      = 0x04
	kFilesInfo            = 0x05
	kPackInfo             = 0x06
	kUnPackInfo           = 0x07
	kSubStreamsInfo       = 0x08
	kSize                 = 0x09
	kCRC                  = 0x0A
	kFolder               = 0x0B
	kCodersUnPackSize     = 0x0C
	kNumUnPackStream      = 0x0D
	kEmptyStream          = 0x0E
	kEmptyFile            = 0x0F
	kName                 = 0x11
	kMTime                = 0x14
	kWinAttributes        = 0x15
	kEncodedHeader        = 0x17
	attrDirectory         = 0x10
	attrArchive           = 0x20
	attrUnixExtension     = 0x8000
	firmaLen              = 32
	epocaFiletime         = 116444736000000000
	minDizionario         = 1 << 20
	maxDizionarioPredefin = 64 << 20
	maxDizionarioUltra    = 64 << 20 // come Massima: Ultra cambia i segmenti (vedi Scrivi)
	memoriaPerByteDiz     = 3        // stima: memoria dell'encoder ~ 3 volte il dizionario
	memoriaFissaEncoder   = 48 << 20
)

var firma = []byte{'7', 'z', 0xBC, 0xAF, 0x27, 0x1C, 0, 4}

// ErrAnnullato: lavoro annullato.
var ErrAnnullato = errors.New("annullato")

// ErrInterno: errore imprevisto dentro il compressore (un panic recuperato).
var ErrInterno = errors.New("errore interno del compressore 7z")

// Ricerca: quanto cerca l'encoder LZMA2 per ogni livello (profondità e lunghezza che basta,
// vedi lzma.Writer2Config). È questo che rende veri i livelli: con dati più piccoli del
// dizionario il dizionario da solo non cambiava niente, e "Ultra" poteva uscire più grande
// di "Veloce" (CTRL-ZP-01, ZP01-2). Massima è come prima.
func Ricerca(l Livello) (profondita, basta int) {
	switch l {
	case Veloce:
		return 6, 24
	case Normale:
		return 24, 64
	case Ultra:
		return 256, 0
	}
	return 64, 0
}

// Parametri calcola dizionario, thread e blocco per il livello e la RAM.
func Parametri(o Opzioni, totale int64) (diz int, thread int, blocco int64) {
	diz, blocco = 16<<20, 256<<20
	switch o.Livello {
	case Veloce:
		diz, blocco = 4<<20, 32<<20
	case Massima:
		diz, blocco = maxDizionarioPredefin, 1<<30
	case Ultra:
		diz, blocco = maxDizionarioUltra, 4<<30
	}
	if o.Dizionario > 0 {
		diz = o.Dizionario
	}
	// Inutile un dizionario più grande dei dati.
	for diz > minDizionario && int64(diz/2) >= totale {
		diz /= 2
	}
	ram := o.RAMLibera
	if ram > 0 {
		// Teniamo l'encoder sotto metà della RAM libera.
		for diz > minDizionario && int64(diz)*memoriaPerByteDiz+memoriaFissaEncoder > ram/2 {
			diz /= 2
		}
	}
	if diz < minDizionario {
		diz = minDizionario
	}
	thread = o.Thread
	if thread <= 0 {
		thread = runtime.NumCPU()
		if thread > 16 {
			thread = 16
		}
	}
	if ram > 0 {
		perThread := int64(diz)*memoriaPerByteDiz + memoriaFissaEncoder
		for thread > 1 && int64(thread)*perThread > ram/2 {
			thread--
		}
	}
	if o.Blocco != 0 {
		blocco = o.Blocco
	}
	// Con più core non servono blocchi più piccoli: ogni blocco grande si divide in
	// segmenti compressi in parallelo (vedi DimSegmento).
	return
}

// cartella (folder) del 7z: un blocco solido.
type blocco struct {
	voci      []int // indici in voci (tutti i file del blocco, nell'ordine)
	impacc    int64 // byte compressi
	scompatto int64
	dimFile   []int64  // dimensione di ogni file entrato
	crcFile   []uint32 // CRC32 di ogni file entrato
	entrati   []int    // indici dei file entrati davvero
	segmenti  []*segmento
	copia     bool // dati già compressi: LZMA2 con soli pezzi non compressi
}

// segmento: un pezzo del flusso di un blocco, compresso per conto suo (anche in parallelo).
// Il flusso LZMA2 del blocco è l'unione dei segmenti: ognuno comincia azzerando il
// dizionario (come fa 7-Zip con più core), e solo l'ultimo ha il byte finale 0x00.
type segmento struct {
	copia  bool
	pezzi  []pezzo
	crc    []uint32 // CRC32 di ogni pezzo
	impacc int64    // byte compressi, compreso lo 0x00 finale
	tmp    string   // file temporaneo (compressione in parallelo)
	err    error
}

// pezzo: byte [da, a) del file voci[voce].
type pezzo struct {
	voce  int
	da, a int64
}

// ErrCambiato: un file è cambiato (più corto) mentre lo comprimevo.
var ErrCambiato = errors.New("un file è cambiato mentre lo comprimevo")

type contatore struct {
	w   io.Writer
	n   int64
	err error // il primo errore di scrittura (l'encoder LZMA2 a volte lo perde in Close)
}

func (c *contatore) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	if err != nil && c.err == nil {
		c.err = err
	}
	return n, err
}

// DimSegmento: quanti byte (non compressi) comprime ogni core per volta. Più grande =
// compressione un po' migliore; più piccolo = più core al lavoro sullo stesso file.
func DimSegmento(diz int) int64 {
	s := int64(diz) * 2
	if s < 32<<20 {
		s = 32 << 20
	}
	return s
}

// Scrivi crea l'archivio .7z in f (deve essere vuoto, all'inizio).
func Scrivi(ctx context.Context, f *os.File, voci []Voce, o Opzioni) (ris Risultato, err error) {
	// Prima controllo che ogni file si possa aprire: quelli bloccati (es. aperti da un altro
	// programma) restano fuori e lo dico.
	fuori := map[int]bool{}
	var totale int64
	for i, v := range voci {
		if v.Cartella || v.Dim <= 0 {
			continue
		}
		if ctx.Err() != nil {
			return ris, ErrAnnullato
		}
		r, e := v.Apri()
		if e != nil {
			fuori[i] = true
			if o.Saltato != nil {
				o.Saltato(v.Nome, e)
			}
			continue
		}
		// Nome sconosciuto ma dati già compressi (o cifrati)? Una prova veloce lo dice:
		// LZMA2 su dati così è lentissimo e non guadagna niente.
		if !v.GiaCompresso && v.Dim >= minCampione && sembraGiaCompresso(r, v.Dim) {
			voci[i].GiaCompresso = true
		}
		r.Close()
		totale += v.Dim
	}
	diz, thread, dimBlocco := Parametri(o, totale)
	seg := o.Segmento
	if seg <= 0 {
		seg = DimSegmento(diz)
		if o.Livello == Ultra {
			seg = 4 * int64(diz) // segmenti grandi: un po' più piccolo, più lento
		}
	}
	ris.Dizionario = diz

	// I blocchi solidi: i file non vuoti in ordine, divisi per dimensione; prima quelli da
	// comprimere, poi quelli già compressi.
	var blocchi []*blocco
	var daComprimere int64
	for _, copia := range []bool{false, true} {
		var cur *blocco
		var curDim int64
		for i, v := range voci {
			if v.Cartella || v.Dim <= 0 || fuori[i] || v.GiaCompresso != copia {
				continue
			}
			if cur == nil || dimBlocco < 0 || (curDim > 0 && curDim+v.Dim > dimBlocco) {
				cur = &blocco{copia: copia}
				blocchi = append(blocchi, cur)
				curDim = 0
			}
			cur.voci = append(cur.voci, i)
			curDim += v.Dim
			if !copia {
				daComprimere += v.Dim
			} else {
				ris.Copiati += v.Dim
			}
		}
	}
	// Con più core e pochi dati, segmenti più piccoli perché lavorino tutti.
	if o.Segmento <= 0 && thread > 1 && o.Livello != Ultra {
		if per := (daComprimere + int64(thread) - 1) / int64(thread); per < seg {
			seg = per
			if seg < 4<<20 {
				seg = 4 << 20
			}
		}
	}
	// I segmenti di ogni blocco (con un core solo: un segmento per blocco, compressione migliore).
	var lavori []*segmento
	for _, b := range blocchi {
		var tot int64
		for _, i := range b.voci {
			tot += voci[i].Dim
		}
		n := int64(1)
		if thread > 1 && tot > seg && !b.copia {
			n = (tot + seg - 1) / seg
		}
		per := (tot + n - 1) / n
		s := &segmento{copia: b.copia}
		var dentro int64
		for _, i := range b.voci {
			var da int64
			for da < voci[i].Dim {
				k := voci[i].Dim - da
				if spazio := per - dentro; k > spazio {
					k = spazio
				}
				s.pezzi = append(s.pezzi, pezzo{voce: i, da: da, a: da + k})
				da += k
				dentro += k
				if dentro >= per {
					b.segmenti = append(b.segmenti, s)
					s = &segmento{copia: b.copia}
					dentro = 0
				}
			}
		}
		if len(s.pezzi) > 0 {
			b.segmenti = append(b.segmenti, s)
		}
		lavori = append(lavori, b.segmenti...)
	}
	if thread > len(lavori) {
		thread = len(lavori)
	}
	if thread < 1 {
		thread = 1
	}
	ris.Thread = thread

	if _, err = f.Write(make([]byte, firmaLen)); err != nil {
		return ris, err
	}
	prof, basta := Ricerca(o.Livello)
	cfg := lzma.Writer2Config{DictCap: diz, Matcher: lzma.HashTable4, BufSize: 1 << 16, Profondita: prof, Basta: basta}
	if err = cfg.Verify(); err != nil {
		return ris, err
	}

	comprimi := func(s *segmento, w io.Writer) {
		// Un errore imprevisto (panic) nel compressore diventa un errore dell'archivio, non la
		// chiusura di Tasso Zip: nei lavori in parallelo (go func più sotto) i recover della
		// finestra non arrivano (PRESTAZIONI-ZIP-PDF, Z2/C7).
		defer func() {
			if r := recover(); r != nil {
				s.err = fmt.Errorf("%w: %v", ErrInterno, r)
			}
		}()
		cw := &contatore{w: w}
		var lw io.WriteCloser
		if s.copia {
			lw = &scrittoreCopia{w: cw}
		} else {
			l2, e := cfg.NewWriter2(cw)
			if e != nil {
				s.err = e
				return
			}
			lw = l2
		}
		buf := make([]byte, 256<<10)
		for _, p := range s.pezzi {
			v := voci[p.voce]
			r, e := v.Apri()
			if e != nil {
				s.err = fmt.Errorf("%s: %w", v.Nome, e)
				return
			}
			if p.da > 0 {
				if sk, ok := r.(io.Seeker); ok {
					_, e = sk.Seek(p.da, io.SeekStart)
				} else {
					_, e = io.CopyN(io.Discard, r, p.da)
				}
				if e != nil {
					r.Close()
					s.err = fmt.Errorf("%s: %w", v.Nome, ErrCambiato)
					return
				}
			}
			h := crc32.NewIEEE()
			resta := p.a - p.da
			for resta > 0 {
				if ctx.Err() != nil {
					r.Close()
					s.err = ErrAnnullato
					return
				}
				b := buf
				if int64(len(b)) > resta {
					b = b[:resta]
				}
				k, eL := r.Read(b)
				if k > 0 {
					if _, e := lw.Write(b[:k]); e != nil {
						r.Close()
						s.err = e
						return
					}
					h.Write(b[:k])
					resta -= int64(k)
					if o.Progresso != nil {
						o.Progresso(int64(k), v.Nome)
					}
				}
				if eL == io.EOF && resta > 0 {
					r.Close()
					s.err = fmt.Errorf("%s: %w", v.Nome, ErrCambiato)
					return
				}
				if eL != nil && eL != io.EOF {
					r.Close()
					s.err = fmt.Errorf("%s: %w", v.Nome, eL)
					return
				}
			}
			r.Close()
			s.crc = append(s.crc, h.Sum32())
		}
		if e := lw.Close(); e != nil {
			s.err = e
			return
		}
		if cw.err != nil {
			s.err = cw.err
			return
		}
		s.impacc = cw.n
	}

	out := bufio.NewWriterSize(f, 1<<20)
	if thread <= 1 {
		// Un core: ogni blocco ha un segmento solo, scritto direttamente nell'archivio.
		for _, s := range lavori {
			comprimi(s, out)
			if s.err != nil {
				return ris, s.err
			}
		}
	} else {
		// In parallelo: ogni segmento in un file temporaneo, poi li unisco in ordine.
		defer func() {
			for _, s := range lavori {
				if s.tmp != "" {
					os.Remove(s.tmp)
				}
			}
		}()
		var wg sync.WaitGroup
		sem := make(chan struct{}, thread)
		cctx, ferma := context.WithCancel(ctx)
		defer ferma()
		ctxOrig := ctx
		ctx = cctx
		for _, s := range lavori {
			s := s
			sem <- struct{}{}
			if ctx.Err() != nil {
				<-sem
				break
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				t, e := os.CreateTemp(o.DirTemp, ".tassozip-*.tmp")
				if e != nil {
					s.err = e
					ferma()
					return
				}
				s.tmp = t.Name()
				bw := bufio.NewWriterSize(t, 1<<20)
				comprimi(s, bw)
				if s.err == nil {
					s.err = bw.Flush()
				}
				if e := t.Close(); s.err == nil {
					s.err = e
				}
				if s.err != nil {
					ferma()
				}
			}()
		}
		wg.Wait()
		ctx = ctxOrig
		if ctx.Err() != nil {
			return ris, ErrAnnullato
		}
		for _, s := range lavori {
			if s.err != nil && !errors.Is(s.err, ErrAnnullato) {
				return ris, s.err
			}
		}
		for _, s := range lavori {
			if s.err != nil || s.tmp == "" {
				return ris, ErrAnnullato
			}
		}
		for _, b := range blocchi {
			for k, s := range b.segmenti {
				t, e := os.Open(s.tmp)
				if e != nil {
					return ris, e
				}
				n := s.impacc
				if k < len(b.segmenti)-1 {
					n-- // tolgo lo 0x00 finale: il flusso continua col segmento dopo
				}
				_, e = io.CopyN(out, t, n)
				if e == nil && k < len(b.segmenti)-1 {
					var fine [1]byte
					if _, e = io.ReadFull(t, fine[:]); e == nil && fine[0] != 0 {
						e = errors.New("segmento LZMA2 senza fine")
					}
				}
				t.Close()
				os.Remove(s.tmp)
				s.tmp = ""
				if e != nil {
					return ris, e
				}
			}
		}
	}
	if ctx.Err() != nil {
		return ris, ErrAnnullato
	}
	// Dimensioni e CRC di ogni file (un file può essere diviso fra più segmenti).
	for _, b := range blocchi {
		for k, s := range b.segmenti {
			b.impacc += s.impacc
			if k < len(b.segmenti)-1 {
				b.impacc--
			}
			for j, p := range s.pezzi {
				n := p.a - p.da
				if p.da == 0 {
					b.entrati = append(b.entrati, p.voce)
					b.dimFile = append(b.dimFile, n)
					b.crcFile = append(b.crcFile, s.crc[j])
				} else {
					u := len(b.crcFile) - 1
					b.crcFile[u] = crc32Combina(b.crcFile[u], s.crc[j], n)
					b.dimFile[u] += n
				}
				b.scompatto += n
			}
		}
	}

	// File dell'archivio: tutte le cartelle, i file entrati e i file vuoti.
	entrato := map[int]bool{}
	var usati []*blocco
	var impTot int64
	for _, b := range blocchi {
		for _, i := range b.entrati {
			entrato[i] = true
		}
		if len(b.entrati) > 0 || b.impacc > 0 {
			usati = append(usati, b)
		}
		impTot += b.impacc
	}
	var elenco []int // ordine nel 7z: prima i file con flusso (in ordine di blocco), poi gli altri
	for _, b := range usati {
		elenco = append(elenco, b.entrati...)
	}
	for i, v := range voci {
		if entrato[i] {
			continue
		}
		if v.Cartella || v.Dim <= 0 {
			elenco = append(elenco, i)
		}
	}
	for _, i := range elenco {
		if voci[i].Cartella {
			ris.Cartelle++
		} else {
			ris.File++
		}
	}
	for _, b := range usati {
		ris.Originale += b.scompatto
	}
	ris.Blocchi = len(usati)

	h := scriviHeader(diz, voci, elenco, entrato, usati)

	// Intestazione compressa (LZMA2) subito dopo i dati.
	var hc contatore
	var hbuf bufWriter
	hc.w = &hbuf
	hcfg := lzma.Writer2Config{DictCap: 1 << 20, Matcher: lzma.HashTable4}
	lw, err := hcfg.NewWriter2(&hc)
	if err != nil {
		return ris, err
	}
	if _, err = lw.Write(h); err != nil {
		return ris, err
	}
	if err = lw.Close(); err != nil {
		return ris, err
	}
	if _, err = out.Write(hbuf.b); err != nil {
		return ris, err
	}
	var eh bufWriter
	eh.byte(kEncodedHeader)
	eh.byte(kPackInfo)
	eh.num(uint64(impTot))
	eh.num(1)
	eh.byte(kSize)
	eh.num(uint64(len(hbuf.b)))
	eh.byte(kEnd)
	eh.byte(kUnPackInfo)
	eh.byte(kFolder)
	eh.num(1)
	eh.byte(0)
	scriviCoder(&eh, 1<<20)
	eh.byte(kCodersUnPackSize)
	eh.num(uint64(len(h)))
	eh.byte(kCRC)
	eh.byte(1)
	eh.u32(crc32.ChecksumIEEE(h))
	eh.byte(kEnd)
	eh.byte(kEnd)
	if _, err = out.Write(eh.b); err != nil {
		return ris, err
	}
	if err = out.Flush(); err != nil {
		return ris, err
	}

	// Firma iniziale con la posizione dell'intestazione.
	var sh [firmaLen]byte
	copy(sh[:], firma)
	binary.LittleEndian.PutUint64(sh[12:], uint64(impTot)+uint64(len(hbuf.b)))
	binary.LittleEndian.PutUint64(sh[20:], uint64(len(eh.b)))
	binary.LittleEndian.PutUint32(sh[28:], crc32.ChecksumIEEE(eh.b))
	binary.LittleEndian.PutUint32(sh[8:], crc32.ChecksumIEEE(sh[12:32]))
	if _, err = f.WriteAt(sh[:], 0); err != nil {
		return ris, err
	}
	return ris, nil
}

func scriviCoder(w *bufWriter, diz int) {
	w.num(1)         // un coder
	w.byte(0x20 | 1) // id di 1 byte, con proprietà
	w.byte(0x21)     // LZMA2
	w.num(1)         // 1 byte di proprietà
	w.byte(codiceDizionario(diz))
}

// codiceDizionario: il byte di proprietà LZMA2 per il dizionario.
func codiceDizionario(diz int) byte {
	for c := 0; c < 40; c++ {
		if int64(2|(c&1))<<(uint(c)/2+11) >= int64(diz) {
			return byte(c)
		}
	}
	return 40
}

func scriviHeader(diz int, voci []Voce, elenco []int, entrato map[int]bool, blocchi []*blocco) []byte {
	var w bufWriter
	w.byte(kHeader)
	if len(blocchi) > 0 {
		w.byte(kMainStreamsInfo)
		w.byte(kPackInfo)
		w.num(0)
		w.num(uint64(len(blocchi)))
		w.byte(kSize)
		for _, b := range blocchi {
			w.num(uint64(b.impacc))
		}
		w.byte(kEnd)
		w.byte(kUnPackInfo)
		w.byte(kFolder)
		w.num(uint64(len(blocchi)))
		w.byte(0)
		for _, b := range blocchi {
			if b.copia {
				scriviCoder(&w, 1<<20)
			} else {
				scriviCoder(&w, diz)
			}
		}
		w.byte(kCodersUnPackSize)
		for _, b := range blocchi {
			w.num(uint64(b.scompatto))
		}
		w.byte(kEnd)
		w.byte(kSubStreamsInfo)
		w.byte(kNumUnPackStream)
		for _, b := range blocchi {
			w.num(uint64(len(b.entrati)))
		}
		w.byte(kSize)
		for _, b := range blocchi {
			for j := 0; j+1 < len(b.dimFile); j++ {
				w.num(uint64(b.dimFile[j]))
			}
		}
		w.byte(kCRC)
		w.byte(1)
		for _, b := range blocchi {
			for _, c := range b.crcFile {
				w.u32(c)
			}
		}
		w.byte(kEnd)
		w.byte(kEnd)
	}
	n := len(elenco)
	w.byte(kFilesInfo)
	w.num(uint64(n))

	vuoti := make([]bool, n)
	var quantiVuoti int
	var fileVuoti []bool
	for k, i := range elenco {
		if !entrato[i] {
			vuoti[k] = true
			quantiVuoti++
			fileVuoti = append(fileVuoti, !voci[i].Cartella)
		}
	}
	if quantiVuoti > 0 {
		bv := bitVettore(vuoti)
		w.byte(kEmptyStream)
		w.num(uint64(len(bv)))
		w.b = append(w.b, bv...)
		almenoUno := false
		for _, x := range fileVuoti {
			almenoUno = almenoUno || x
		}
		if almenoUno {
			bv := bitVettore(fileVuoti)
			w.byte(kEmptyFile)
			w.num(uint64(len(bv)))
			w.b = append(w.b, bv...)
		}
	}

	var nomi []byte
	for _, i := range elenco {
		for _, u := range utf16.Encode([]rune(voci[i].Nome)) {
			nomi = append(nomi, byte(u), byte(u>>8))
		}
		nomi = append(nomi, 0, 0)
	}
	w.byte(kName)
	w.num(uint64(len(nomi) + 1))
	w.byte(0)
	w.b = append(w.b, nomi...)

	w.byte(kMTime)
	w.num(uint64(2 + 8*n))
	w.byte(1)
	w.byte(0)
	for _, i := range elenco {
		w.u64(filetime(voci[i].Quando))
	}

	w.byte(kWinAttributes)
	w.num(uint64(2 + 4*n))
	w.byte(1)
	w.byte(0)
	for _, i := range elenco {
		a := voci[i].Attributi
		if a == 0 {
			if voci[i].Cartella {
				a = attrDirectory | attrUnixExtension | (0o40755 << 16)
			} else {
				a = attrArchive | attrUnixExtension | (0o100644 << 16)
			}
		}
		w.u32(a)
	}
	w.byte(kEnd)
	w.byte(kEnd)
	return w.b
}

func filetime(t time.Time) uint64 {
	if t.IsZero() {
		t = time.Now()
	}
	v := t.UnixNano()/100 + epocaFiletime
	if v < 0 {
		return 0
	}
	return uint64(v)
}

func bitVettore(b []bool) []byte {
	out := make([]byte, (len(b)+7)/8)
	for i, x := range b {
		if x {
			out[i/8] |= 0x80 >> uint(i%8)
		}
	}
	return out
}

type bufWriter struct{ b []byte }

func (w *bufWriter) Write(p []byte) (int, error) { w.b = append(w.b, p...); return len(p), nil }
func (w *bufWriter) byte(c byte)                 { w.b = append(w.b, c) }
func (w *bufWriter) u32(v uint32)                { w.b = binary.LittleEndian.AppendUint32(w.b, v) }
func (w *bufWriter) u64(v uint64)                { w.b = binary.LittleEndian.AppendUint64(w.b, v) }

// num scrive un numero nel formato a lunghezza variabile del 7z.
func (w *bufWriter) num(v uint64) {
	first := byte(0)
	mask := byte(0x80)
	i := 0
	for ; i < 8; i++ {
		if v < uint64(1)<<(7*uint(i+1)) {
			first |= byte(v >> (8 * uint(i)))
			break
		}
		first |= mask
		mask >>= 1
	}
	w.b = append(w.b, first)
	for j := 0; j < i; j++ {
		w.b = append(w.b, byte(v>>(8*uint(j))))
	}
}

// crc32Combina: il CRC32 di A+B sapendo crc(A), crc(B) e la lunghezza di B
// (lo stesso calcolo di crc32_combine di zlib).
func crc32Combina(crc1, crc2 uint32, len2 int64) uint32 {
	if len2 <= 0 {
		return crc1
	}
	var pari, dispari [32]uint32
	dispari[0] = 0xedb88320 // polinomio CRC-32 (IEEE), rovesciato
	riga := uint32(1)
	for n := 1; n < 32; n++ {
		dispari[n] = riga
		riga <<= 1
	}
	quadrato(&pari, &dispari) // operatore per 2 bit a zero
	quadrato(&dispari, &pari) // per 4 bit a zero
	for {
		quadrato(&pari, &dispari)
		if len2&1 != 0 {
			crc1 = perMatrice(&pari, crc1)
		}
		len2 >>= 1
		if len2 == 0 {
			break
		}
		quadrato(&dispari, &pari)
		if len2&1 != 0 {
			crc1 = perMatrice(&dispari, crc1)
		}
		len2 >>= 1
		if len2 == 0 {
			break
		}
	}
	return crc1 ^ crc2
}

func perMatrice(m *[32]uint32, v uint32) uint32 {
	var s uint32
	for i := 0; v != 0; i++ {
		if v&1 != 0 {
			s ^= m[i]
		}
		v >>= 1
	}
	return s
}

func quadrato(q, m *[32]uint32) {
	for n := 0; n < 32; n++ {
		q[n] = perMatrice(m, m[n])
	}
}

// scrittoreCopia scrive un flusso LZMA2 fatto solo di pezzi non compressi (al massimo
// 64 KB ciascuno, 3 byte di intestazione): è LZMA2 standard, lo legge ogni programma, e
// non spreca tempo a comprimere dati che non si comprimono.
type scrittoreCopia struct {
	w     io.Writer
	buf   []byte
	primo bool // il primo pezzo è già stato scritto
	err   error
}

const pezzoCopia = 1 << 16

func (c *scrittoreCopia) Write(p []byte) (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	n := len(p)
	for len(p) > 0 {
		if c.buf == nil {
			c.buf = make([]byte, 0, pezzoCopia)
		}
		k := pezzoCopia - len(c.buf)
		if k > len(p) {
			k = len(p)
		}
		c.buf = append(c.buf, p[:k]...)
		p = p[k:]
		if len(c.buf) == pezzoCopia {
			if err := c.svuota(); err != nil {
				return n - len(p), err
			}
		}
	}
	return n, nil
}

func (c *scrittoreCopia) svuota() error {
	if len(c.buf) == 0 {
		return nil
	}
	ctl := byte(0x02) // pezzo non compresso, senza azzerare il dizionario
	if !c.primo {
		ctl = 0x01 // il primo azzera il dizionario
		c.primo = true
	}
	k := len(c.buf) - 1
	if _, err := c.w.Write([]byte{ctl, byte(k >> 8), byte(k)}); err != nil {
		c.err = err
		return err
	}
	if _, err := c.w.Write(c.buf); err != nil {
		c.err = err
		return err
	}
	c.buf = c.buf[:0]
	return nil
}

func (c *scrittoreCopia) Close() error {
	if err := c.svuota(); err != nil {
		return err
	}
	_, err := c.w.Write([]byte{0})
	return err
}

// minCampione: sotto questa dimensione non vale la pena provare.
const minCampione = 1 << 20

// sembraGiaCompresso prova a comprimere con Deflate (veloce) due pezzi da 128 KB del file
// (inizio e metà): se non scendono almeno del 4%, i dati sono già compressi o cifrati.
func sembraGiaCompresso(r io.Reader, dim int64) bool {
	const pezzo = 128 << 10
	buf := make([]byte, 0, 2*pezzo)
	b := make([]byte, pezzo)
	n, _ := io.ReadFull(r, b)
	buf = append(buf, b[:n]...)
	if sk, ok := r.(io.Seeker); ok && dim > 4*pezzo {
		if _, err := sk.Seek(dim/2, io.SeekStart); err == nil {
			n, _ = io.ReadFull(r, b)
			buf = append(buf, b[:n]...)
		}
	}
	if len(buf) < pezzo/2 {
		return false
	}
	var c contatore
	c.w = io.Discard
	fw, err := flate.NewWriter(&c, flate.BestSpeed)
	if err != nil {
		return false
	}
	fw.Write(buf)
	fw.Close()
	return c.n*100 >= int64(len(buf))*96
}
