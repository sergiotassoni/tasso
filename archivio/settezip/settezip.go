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
	"context"
	"encoding/binary"
	"errors"
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
	// Apri apre il contenuto (solo per i file con Dim > 0).
	Apri func() (io.ReadCloser, error)
}

// Opzioni di scrittura.
type Opzioni struct {
	Livello    Livello
	Dizionario int   // byte; 0 = secondo il livello e la RAM libera
	Thread     int   // 0 = secondo i core e la RAM
	Blocco     int64 // dimensione di un blocco solido; 0 = secondo il livello; <0 = non solido (un file per blocco)
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
	maxDizionarioUltra    = 128 << 20
	memoriaPerByteDiz     = 3 // stima: memoria dell'encoder ~ 3 volte il dizionario
	memoriaFissaEncoder   = 48 << 20
)

var firma = []byte{'7', 'z', 0xBC, 0xAF, 0x27, 0x1C, 0, 4}

// ErrAnnullato: lavoro annullato.
var ErrAnnullato = errors.New("annullato")

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
	// Con più core: blocchi più piccoli per usarli tutti (ma mai sotto 4 volte il dizionario).
	if blocco > 0 && thread > 1 && totale > 0 {
		per := totale / int64(thread)
		if min := int64(diz) * 4; per < min {
			per = min
		}
		if per < blocco {
			blocco = per
		}
	}
	return
}

// cartella (folder) del 7z: un blocco solido.
type blocco struct {
	voci      []int // indici in voci
	impacc    int64 // byte compressi
	scompatto int64
	dimFile   []int64  // dimensione reale di ogni file entrato
	crcFile   []uint32 // CRC32 di ogni file entrato
	entrati   []int    // indici dei file entrati davvero
	tmp       string   // file temporaneo (compressione in parallelo)
	err       error
}

type contatore struct {
	w io.Writer
	n int64
}

func (c *contatore) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// Scrivi crea l'archivio .7z in f (deve essere vuoto, all'inizio).
func Scrivi(ctx context.Context, f *os.File, voci []Voce, o Opzioni) (ris Risultato, err error) {
	var totale int64
	for _, v := range voci {
		if !v.Cartella {
			totale += v.Dim
		}
	}
	diz, thread, dimBlocco := Parametri(o, totale)
	ris.Dizionario, ris.Thread = diz, thread

	// I blocchi solidi: i file non vuoti in ordine, divisi per dimensione.
	var blocchi []*blocco
	var cur *blocco
	var curDim int64
	for i, v := range voci {
		if v.Cartella || v.Dim <= 0 {
			continue
		}
		if cur == nil || dimBlocco < 0 || (curDim > 0 && curDim+v.Dim > dimBlocco) {
			cur = &blocco{}
			blocchi = append(blocchi, cur)
			curDim = 0
		}
		cur.voci = append(cur.voci, i)
		curDim += v.Dim
	}
	if thread > len(blocchi) {
		thread = len(blocchi)
	}
	if thread < 1 {
		thread = 1
	}
	ris.Thread = thread

	if _, err = f.Write(make([]byte, firmaLen)); err != nil {
		return ris, err
	}
	cfg := lzma.Writer2Config{DictCap: diz, Matcher: lzma.HashTable4, BufSize: 1 << 16}
	if err = cfg.Verify(); err != nil {
		return ris, err
	}

	comprimi := func(b *blocco, w io.Writer) {
		cw := &contatore{w: w}
		lw, e := cfg.NewWriter2(cw)
		if e != nil {
			b.err = e
			return
		}
		buf := make([]byte, 256<<10)
		for _, i := range b.voci {
			v := voci[i]
			if ctx.Err() != nil {
				b.err = ErrAnnullato
				return
			}
			r, e := v.Apri()
			if e != nil {
				if o.Saltato != nil {
					o.Saltato(v.Nome, e)
				}
				continue
			}
			h := crc32.NewIEEE()
			var n int64
			for {
				if ctx.Err() != nil {
					r.Close()
					b.err = ErrAnnullato
					return
				}
				k, eL := r.Read(buf)
				if k > 0 {
					if _, e := lw.Write(buf[:k]); e != nil {
						r.Close()
						b.err = e
						return
					}
					h.Write(buf[:k])
					n += int64(k)
					if o.Progresso != nil {
						o.Progresso(int64(k), v.Nome)
					}
				}
				if eL == io.EOF {
					break
				}
				if eL != nil {
					r.Close()
					b.err = eL
					return
				}
			}
			r.Close()
			if n == 0 {
				// Il file è diventato vuoto: lo segno come file vuoto (niente flusso).
				voci[i].Dim = 0
				continue
			}
			b.entrati = append(b.entrati, i)
			b.dimFile = append(b.dimFile, n)
			b.crcFile = append(b.crcFile, h.Sum32())
			b.scompatto += n
		}
		if e := lw.Close(); e != nil {
			b.err = e
			return
		}
		b.impacc = cw.n
	}

	out := bufio.NewWriterSize(f, 1<<20)
	if thread <= 1 {
		for _, b := range blocchi {
			comprimi(b, out)
			if b.err != nil {
				return ris, b.err
			}
		}
	} else {
		// In parallelo: ogni blocco in un file temporaneo, poi li unisco in ordine.
		defer func() {
			for _, b := range blocchi {
				if b.tmp != "" {
					os.Remove(b.tmp)
				}
			}
		}()
		var wg sync.WaitGroup
		sem := make(chan struct{}, thread)
		cctx, ferma := context.WithCancel(ctx)
		defer ferma()
		ctxOrig := ctx
		ctx = cctx
		for _, b := range blocchi {
			b := b
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				if ctx.Err() != nil {
					b.err = ErrAnnullato
					return
				}
				t, e := os.CreateTemp(o.DirTemp, ".tassozip-*.tmp")
				if e != nil {
					b.err = e
					ferma()
					return
				}
				b.tmp = t.Name()
				bw := bufio.NewWriterSize(t, 1<<20)
				comprimi(b, bw)
				if b.err == nil {
					b.err = bw.Flush()
				}
				if e := t.Close(); b.err == nil {
					b.err = e
				}
				if b.err != nil {
					ferma()
				}
			}()
		}
		wg.Wait()
		ctx = ctxOrig
		if ctx.Err() != nil {
			return ris, ErrAnnullato
		}
		for _, b := range blocchi {
			if b.err != nil {
				return ris, b.err
			}
		}
		for _, b := range blocchi {
			t, e := os.Open(b.tmp)
			if e != nil {
				return ris, e
			}
			n, e := io.Copy(out, t)
			t.Close()
			os.Remove(b.tmp)
			b.tmp = ""
			if e != nil {
				return ris, e
			}
			if n != b.impacc {
				return ris, errors.New("blocco temporaneo incompleto")
			}
		}
	}
	if ctx.Err() != nil {
		return ris, ErrAnnullato
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
		for range blocchi {
			scriviCoder(&w, diz)
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
