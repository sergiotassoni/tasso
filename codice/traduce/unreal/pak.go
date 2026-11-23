// Package unreal legge i file .pak dei giochi Unreal Engine (solo lettura) e ne crea di nuovi,
// piccoli, con i file di lingua tradotti. Gli originali del gioco non si toccano mai.
package unreal

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf16"
)

const magiaPak = 0x5A6F12E1

// Voce: un file dentro il pak.
type Voce struct {
	Nome        string // percorso relativo al punto di montaggio, con '/'
	Offset      int64
	Size        int64
	Uncomp      int64
	Compr       uint32 // 0 = nessuna, altrimenti indice+1 nei nomi del pak
	Cifrato     bool
	BloccoSize  uint32
	Blocchi     [][2]int64 // inizio e fine (relativi a Offset dalla versione 5)
	intestaSize int64
}

// Pak: un .pak aperto in lettura.
type Pak struct {
	f          *os.File
	Versione   int32
	Montaggio  string
	Voci       []Voce
	Compress   []string // nomi dei metodi (indice 0 = metodo 1)
	IndiceCifr bool
}

// ApriPak legge il piè di pagina e l'indice. Non scrive niente.
func ApriPak(percorso string) (*Pak, error) {
	f, err := os.Open(percorso)
	if err != nil {
		return nil, err
	}
	p := &Pak{f: f}
	if err := p.leggiPiede(); err != nil {
		f.Close()
		return nil, err
	}
	return p, nil
}

func (p *Pak) Chiudi() error { return p.f.Close() }

func (p *Pak) leggiPiede() error {
	st, err := p.f.Stat()
	if err != nil {
		return err
	}
	n := min(st.Size(), 512)
	coda := make([]byte, n)
	if _, err := p.f.ReadAt(coda, st.Size()-n); err != nil {
		return err
	}
	// cerco la magia partendo dal fondo; il piè di pagina più grande (v9) è 222 byte
	for i := len(coda) - 44; i >= 0; i-- {
		if binary.LittleEndian.Uint32(coda[i:]) != magiaPak {
			continue
		}
		ver := int32(binary.LittleEndian.Uint32(coda[i+4:]))
		if ver < 1 || ver > 12 {
			continue
		}
		dopo := 4 + 4 + 8 + 8 + 20 // magia, versione, offset, size, hash
		if ver == 9 {
			dopo++ // indice "congelato"
		}
		nomi := 0
		if ver >= 8 {
			nomi = 5 // v8a ne ha 4: provo prima 5, poi 4
		}
		for _, nn := range []int{nomi, 4} {
			if nn != nomi && ver != 8 {
				break
			}
			if i+dopo+nn*32 != len(coda) {
				continue
			}
			off := int64(binary.LittleEndian.Uint64(coda[i+8:]))
			size := int64(binary.LittleEndian.Uint64(coda[i+16:]))
			if ver >= 4 && i >= 1 {
				p.IndiceCifr = coda[i-1] != 0
			}
			p.Versione = ver
			for k := 0; k < nn; k++ {
				s := coda[i+dopo+k*32 : i+dopo+(k+1)*32]
				if z := bytes.IndexByte(s, 0); z >= 0 {
					s = s[:z]
				}
				p.Compress = append(p.Compress, string(s))
			}
			if ver < 8 {
				p.Compress = []string{"Zlib", "Gzip", "", "", "Custom"}
			}
			if p.IndiceCifr {
				return errors.New("l'elenco dei file del gioco è cifrato")
			}
			if off <= 0 || size <= 0 || off+size > st.Size() || size > 1<<30 {
				return errors.New("elenco dei file non valido")
			}
			idx := make([]byte, size)
			if _, err := p.f.ReadAt(idx, off); err != nil {
				return err
			}
			if ver >= 10 {
				return p.leggiIndiceNuovo(idx, st.Size())
			}
			return p.leggiIndiceVecchio(idx)
		}
	}
	return errors.New("non è un file .pak di Unreal")
}

// ---- lettura a pezzi ----

type lettore struct {
	b   []byte
	pos int
	err error
}

func (r *lettore) prendi(n int) []byte {
	if r.err != nil || n < 0 || r.pos+n > len(r.b) {
		r.err = errors.New("dati finiti prima del previsto")
		return make([]byte, max(n, 0))
	}
	s := r.b[r.pos : r.pos+n]
	r.pos += n
	return s
}
func (r *lettore) u8() uint8   { return r.prendi(1)[0] }
func (r *lettore) u32() uint32 { return binary.LittleEndian.Uint32(r.prendi(4)) }
func (r *lettore) i32() int32  { return int32(r.u32()) }
func (r *lettore) u64() uint64 { return binary.LittleEndian.Uint64(r.prendi(8)) }
func (r *lettore) i64() int64  { return int64(r.u64()) }

// fstring: stringa di Unreal (lunghezza con lo zero finale; negativa = UTF-16).
func (r *lettore) fstring() string {
	n := r.i32()
	switch {
	case n == 0:
		return ""
	case n > 0:
		if n > 1<<20 {
			r.err = errors.New("testo troppo lungo")
			return ""
		}
		b := r.prendi(int(n))
		return strings.TrimRight(string(latin1(b)), "\x00")
	default:
		if n < -(1 << 20) {
			r.err = errors.New("testo troppo lungo")
			return ""
		}
		b := r.prendi(int(-n) * 2)
		u := make([]uint16, -n)
		for i := range u {
			u[i] = binary.LittleEndian.Uint16(b[2*i:])
		}
		return strings.TrimRight(string(utf16.Decode(u)), "\x00")
	}
}

// latin1: le stringhe "ANSI" di Unreal sono byte singoli (in pratica ASCII).
func latin1(b []byte) []rune {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return r
}

// voce: un FPakEntry come è scritto nell'indice (e prima dei dati).
func (p *Pak) voce(r *lettore) Voce {
	v := Voce{}
	inizio := r.pos
	v.Offset, v.Size, v.Uncomp = r.i64(), r.i64(), r.i64()
	if p.Versione >= 8 {
		v.Compr = r.u32()
	} else {
		fl := r.u32()
		switch {
		case fl&0x01 != 0:
			v.Compr = 1 // zlib
		case fl&0x02 != 0:
			v.Compr = 2 // gzip
		case fl&0x04 != 0:
			v.Compr = 5 // custom
		}
	}
	if p.Versione == 1 {
		r.u64() // data
	}
	r.prendi(20) // hash
	if p.Versione >= 3 {
		if v.Compr != 0 {
			nb := r.i32()
			if nb < 0 || nb > 1<<20 {
				r.err = errors.New("blocchi non validi")
				return v
			}
			for k := int32(0); k < nb; k++ {
				v.Blocchi = append(v.Blocchi, [2]int64{r.i64(), r.i64()})
			}
		}
		v.Cifrato = r.u8()&1 != 0
		v.BloccoSize = r.u32()
	}
	v.intestaSize = int64(r.pos - inizio)
	return v
}

func (p *Pak) leggiIndiceVecchio(idx []byte) error {
	r := &lettore{b: idx}
	p.Montaggio = r.fstring()
	n := r.i32()
	if n < 0 || n > 5_000_000 {
		return errors.New("elenco dei file non valido")
	}
	for k := int32(0); k < n && r.err == nil; k++ {
		nome := r.fstring()
		v := p.voce(r)
		v.Nome = nome
		p.Voci = append(p.Voci, v)
	}
	return r.err
}

func (p *Pak) leggiIndiceNuovo(idx []byte, fine int64) error {
	r := &lettore{b: idx}
	p.Montaggio = r.fstring()
	r.i32() // numero di voci
	r.u64() // seme del codice dei percorsi
	if r.u32() != 0 {
		r.i64()
		r.i64()
		r.prendi(20)
	}
	haDir := r.u32() != 0
	var dirOff, dirSize int64
	if haDir {
		dirOff, dirSize = r.i64(), r.i64()
		r.prendi(20)
	}
	ne := r.i32()
	if ne < 0 || int(ne) > len(idx) {
		return errors.New("elenco dei file non valido")
	}
	codificate := r.prendi(int(ne))
	nf := r.i32()
	if nf < 0 || nf > 5_000_000 {
		return errors.New("elenco dei file non valido")
	}
	var file []Voce
	for k := int32(0); k < nf && r.err == nil; k++ {
		file = append(file, p.voce(r))
	}
	if r.err != nil {
		return r.err
	}
	if !haDir {
		return errors.New("il gioco non ha l'elenco completo dei file")
	}
	if dirOff <= 0 || dirSize <= 0 || dirOff+dirSize > fine || dirSize > 1<<30 {
		return errors.New("elenco delle cartelle non valido")
	}
	db := make([]byte, dirSize)
	if _, err := p.f.ReadAt(db, dirOff); err != nil {
		return err
	}
	d := &lettore{b: db}
	nd := d.i32()
	if nd < 0 || nd > 5_000_000 {
		return errors.New("elenco delle cartelle non valido")
	}
	for k := int32(0); k < nd && d.err == nil; k++ {
		dir := d.fstring()
		nfile := d.i32()
		if nfile < 0 || nfile > 5_000_000 {
			return errors.New("elenco delle cartelle non valido")
		}
		for j := int32(0); j < nfile && d.err == nil; j++ {
			nome := d.fstring()
			loc := d.i32()
			var v Voce
			if loc >= 0 {
				var err error
				v, err = p.decodifica(codificate, int(loc))
				if err != nil {
					return err
				}
			} else {
				i := int(-loc - 1)
				if i >= len(file) {
					return errors.New("voce non valida")
				}
				v = file[i]
			}
			percorso := strings.TrimPrefix(dir, "/") + nome
			v.Nome = percorso
			p.Voci = append(p.Voci, v)
		}
	}
	return d.err
}

// decodifica: voce compatta dell'indice (dalla versione 10).
func (p *Pak) decodifica(b []byte, pos int) (Voce, error) {
	if pos < 0 || pos+4 > len(b) {
		return Voce{}, errors.New("voce non valida")
	}
	r := &lettore{b: b, pos: pos}
	bits := r.u32()
	v := Voce{}
	bs := bits & 0x3f
	if bs == 0x3f {
		v.BloccoSize = r.u32()
	} else {
		v.BloccoSize = bs << 11
	}
	nb := int((bits >> 6) & 0xffff)
	v.Cifrato = bits&(1<<22) != 0
	v.Compr = (bits >> 23) & 0x3f
	leggi := func(a32 bool) int64 {
		if a32 {
			return int64(r.u32())
		}
		return r.i64()
	}
	v.Offset = leggi(bits&(1<<31) != 0)
	v.Uncomp = leggi(bits&(1<<30) != 0)
	if v.Compr != 0 {
		v.Size = leggi(bits&(1<<29) != 0)
	} else {
		v.Size = v.Uncomp
	}
	// grandezza dell'intestazione scritta prima dei dati (FPakEntry)
	v.intestaSize = 8 + 8 + 8 + 4 + 20 + 1 + 4
	if v.Compr != 0 {
		v.intestaSize += 4 + int64(nb)*16
	}
	if nb > 0 {
		inizio := v.intestaSize
		if nb == 1 && !v.Cifrato {
			v.Blocchi = [][2]int64{{inizio, inizio + v.Size}}
		} else {
			for k := 0; k < nb; k++ {
				s := int64(r.u32())
				v.Blocchi = append(v.Blocchi, [2]int64{inizio, inizio + s})
				if v.Cifrato {
					s = (s + 15) &^ 15
				}
				inizio += s
			}
		}
	}
	return v, r.err
}

// Trova: la voce con questo percorso (senza differenza tra maiuscole e minuscole).
func (p *Pak) Trova(nome string) (Voce, bool) {
	for _, v := range p.Voci {
		if strings.EqualFold(v.Nome, nome) {
			return v, true
		}
	}
	return Voce{}, false
}

// Leggi: il contenuto del file, decompresso. Errore se cifrato o con un metodo che non conosco.
func (p *Pak) Leggi(v Voce) ([]byte, error) {
	if v.Cifrato {
		return nil, errors.New("file cifrato")
	}
	if v.Uncomp < 0 || v.Uncomp > 256<<20 {
		return nil, errors.New("file troppo grande")
	}
	if v.Compr == 0 {
		out := make([]byte, v.Uncomp)
		_, err := p.f.ReadAt(out, v.Offset+v.intestaSize)
		return out, err
	}
	metodo := ""
	if int(v.Compr)-1 < len(p.Compress) {
		metodo = strings.ToLower(p.Compress[v.Compr-1])
	}
	base := int64(0)
	if p.Versione >= 5 {
		base = v.Offset // blocchi relativi alla voce
	}
	var out bytes.Buffer
	resto := v.Uncomp
	for _, bl := range v.Blocchi {
		if bl[1] < bl[0] || bl[1]-bl[0] > 64<<20 {
			return nil, errors.New("blocco non valido")
		}
		src := make([]byte, bl[1]-bl[0])
		if _, err := p.f.ReadAt(src, base+bl[0]); err != nil {
			return nil, err
		}
		n := min(resto, int64(v.BloccoSize))
		if v.BloccoSize == 0 {
			n = resto
		}
		var dec []byte
		var err error
		switch metodo {
		case "zlib":
			var zr io.ReadCloser
			zr, err = zlib.NewReader(bytes.NewReader(src))
			if err == nil {
				dec, err = io.ReadAll(io.LimitReader(zr, n+1))
				zr.Close()
			}
		case "oodle":
			dec, err = DecomprimiOodle(src, int(n))
		default:
			return nil, fmt.Errorf("compressione %q non supportata", metodo)
		}
		if err != nil {
			return nil, err
		}
		if int64(len(dec)) != n {
			return nil, errors.New("grandezza sbagliata dopo la decompressione")
		}
		out.Write(dec)
		resto -= n
	}
	if resto != 0 {
		return nil, errors.New("file incompleto")
	}
	return out.Bytes(), nil
}
