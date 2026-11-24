// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

// "Ripara" per gli zip: se la fine dello zip (la "directory centrale", l'indice di tutti i file)
// è rovinata o manca (download interrotto, chiavetta tolta troppo presto...), i file spesso
// ci sono ancora: ognuno ha davanti la sua piccola intestazione ("PK\x03\x04"). Qui scandisco
// l'archivio dall'inizio, ritrovo le intestazioni, controllo i dati di ogni file con la sua
// somma CRC32 e scrivo uno zip NUOVO ("Nome (riparato).zip") con un indice nuovo.
// L'originale non si tocca mai. I file con i dati rovinati non si possono salvare: li elenco.
// Si fa solo per lo zip: per 7z e rar non c'è un modo sicuro.

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/flate"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// RisultatoRipara: cosa è stato salvato.
type RisultatoRipara struct {
	Archivio string   `json:"archivio"` // lo zip nuovo
	Salvati  int      `json:"salvati"`  // voci salvate (file e cartelle)
	Persi    []string `json:"persi"`    // voci che non si potevano salvare, con il motivo
	Byte     int64    `json:"byte"`     // byte dei file salvati (decompressi)
}

const (
	firmaLocale      = "PK\x03\x04"
	firmaDescrittore = "PK\x07\x08"
	maxVociRipara    = 1_000_000
)

// voceRitrovata: una voce dello zip ritrovata scandendo l'archivio.
type voceRitrovata struct {
	nome          string
	flags, metodo uint16
	ora, data     uint16
	crc           uint32
	comp, decomp  uint64
	inizioDati    int64
	fine          int64 // dove finisce la voce (dati e descrittore compresi)
}

// RiparaZip ricostruisce lo zip. dove = cartella in cui salvarlo ("" = accanto all'originale).
func RiparaZip(ctx context.Context, percorso, dove string, avanzamento FunzioneAvanzamento) (ris RisultatoRipara, err error) {
	percorso, err = filepath.Abs(percorso)
	if err != nil {
		return ris, err
	}
	f, err := os.Open(percorso)
	if err != nil {
		return ris, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return ris, err
	}
	if !fi.Mode().IsRegular() {
		return ris, ErrFormato
	}
	dim := fi.Size()
	testa := make([]byte, 4)
	if _, err := f.ReadAt(testa, 0); err != nil || string(testa) != firmaLocale {
		// Senza una prima intestazione al posto giusto non è uno zip che so riparare
		// (o è un autoestraente, o non è uno zip).
		if !strings.EqualFold(filepath.Ext(percorso), ".zip") {
			return ris, ErrFormato
		}
	}
	if dove == "" {
		dove = filepath.Dir(percorso)
	}
	tmp := filepath.Join(dove, nomeTemporaneo(".tmp"))
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return ris, fmt.Errorf("%w: %v", ErrNonScrivibile, err)
	}
	finito := false
	defer func() {
		if out != nil {
			out.Close()
		}
		if !finito {
			_ = os.Remove(tmp)
		}
	}()
	bw := bufio.NewWriterSize(out, 1<<20)
	zw := zip.NewWriter(bw)

	perdi := func(nome, motivo string) {
		if len(ris.Persi) < 1000 {
			ris.Persi = append(ris.Persi, nome+" ("+motivo+")")
		}
	}
	var pos int64
	cerca := newCercatore(f, dim)
	for voci := 0; voci < maxVociRipara; voci++ {
		if ctx.Err() != nil {
			return ris, ErrAnnullato
		}
		off, ok := cerca.prossima(pos, firmaLocale)
		if !ok {
			break
		}
		if avanzamento != nil {
			avanzamento(Avanzamento{Fatto: off, Totale: dim})
		}
		v, motivo := leggiVoce(ctx, f, dim, off, cerca)
		if errors.Is(ctx.Err(), context.Canceled) {
			return ris, ErrAnnullato
		}
		if motivo != "" {
			if v.nome != "" {
				perdi(nomeLeggibile(v.nome, v.flags), motivo)
			}
			pos = off + 4 // può essere un falso "PK": cerco più avanti
			continue
		}
		fh := &zip.FileHeader{
			Name:               v.nome,
			NonUTF8:            v.flags&0x800 == 0,
			Method:             v.metodo,
			ModifiedTime:       v.ora,
			ModifiedDate:       v.data,
			CRC32:              v.crc,
			CompressedSize64:   v.comp,
			UncompressedSize64: v.decomp,
		}
		if strings.HasSuffix(v.nome, "/") {
			fh.SetMode(0o755 | os.ModeDir)
		}
		w, err := zw.CreateRaw(fh)
		if err != nil {
			return ris, err
		}
		if _, err := io.Copy(w, io.NewSectionReader(f, v.inizioDati, int64(v.comp))); err != nil {
			return ris, err
		}
		ris.Salvati++
		ris.Byte += int64(v.decomp)
		pos = v.fine
	}
	if ris.Salvati == 0 {
		return ris, ErrRovinato
	}
	if err := zw.Close(); err != nil {
		return ris, err
	}
	if err := bw.Flush(); err != nil {
		return ris, err
	}
	if err := out.Sync(); err != nil {
		return ris, err
	}
	if err := out.Close(); err != nil {
		out = nil
		return ris, err
	}
	out = nil
	base, _ := DividiNome(filepath.Base(percorso), false)
	finale, err := spostaConNomeLibero(tmp, dove, base+" (riparato).zip", false)
	if err != nil {
		return ris, err
	}
	finito = true
	ris.Archivio = finale
	return ris, nil
}

func nomeLeggibile(n string, flags uint16) string {
	if flags&0x800 != 0 || utf8.ValidString(n) {
		return strings.ToValidUTF8(n, "_")
	}
	return decodificaNomeZip(n)
}

// leggiVoce legge l'intestazione in off e controlla i dati. motivo != "" se non va bene.
func leggiVoce(ctx context.Context, f io.ReaderAt, dim, off int64, cerca *cercatore) (v voceRitrovata, motivo string) {
	h := make([]byte, 30)
	if _, err := f.ReadAt(h, off); err != nil {
		return v, "incompleto"
	}
	le := binary.LittleEndian
	v.flags = le.Uint16(h[6:])
	v.metodo = le.Uint16(h[8:])
	v.ora, v.data = le.Uint16(h[10:]), le.Uint16(h[12:])
	v.crc = le.Uint32(h[14:])
	v.comp, v.decomp = uint64(le.Uint32(h[18:])), uint64(le.Uint32(h[22:]))
	nl, el := int64(le.Uint16(h[26:])), int64(le.Uint16(h[28:]))
	if nl == 0 || off+30+nl+el > dim {
		return v, "intestazione rovinata"
	}
	nb := make([]byte, nl+el)
	if _, err := f.ReadAt(nb, off+30); err != nil {
		return v, "incompleto"
	}
	v.nome = string(nb[:nl])
	if bytes.IndexByte(nb[:nl], 0) >= 0 {
		v.nome = ""
		return v, "intestazione rovinata"
	}
	extra := nb[nl:]
	zip64 := false
	// Extra ZIP64 (0x0001): le dimensioni vere quando non stanno in 32 bit.
	for len(extra) >= 4 {
		id, sz := le.Uint16(extra), int(le.Uint16(extra[2:]))
		if 4+sz > len(extra) {
			break
		}
		if id == 0x0001 {
			zip64 = true
			d := extra[4 : 4+sz]
			if v.decomp == 0xFFFFFFFF && len(d) >= 8 {
				v.decomp, d = le.Uint64(d), d[8:]
			}
			if v.comp == 0xFFFFFFFF && len(d) >= 8 {
				v.comp = le.Uint64(d)
			}
		}
		extra = extra[4+sz:]
	}
	v.inizioDati = off + 30 + nl + el
	if v.flags&1 != 0 {
		return v, "con password"
	}
	if v.metodo != zip.Store && v.metodo != zip.Deflate {
		return v, fmt.Sprintf("metodo %d non supportato", v.metodo)
	}
	if strings.HasSuffix(v.nome, "/") && v.comp == 0 && v.flags&8 == 0 {
		v.fine = v.inizioDati
		v.decomp, v.crc = 0, 0
		return v, "" // cartella
	}

	if v.flags&8 == 0 {
		// Dimensioni scritte nell'intestazione.
		if int64(v.comp) < 0 || v.inizioDati+int64(v.comp) > dim {
			return v, "incompleto"
		}
		crc, n, err := controllaDati(ctx, io.NewSectionReader(f, v.inizioDati, int64(v.comp)), v.metodo)
		if err != nil || crc != v.crc || uint64(n) != v.decomp {
			return v, "dati rovinati"
		}
		v.fine = v.inizioDati + int64(v.comp)
		return v, ""
	}

	// Dimensioni scritte DOPO i dati (descrittore): le ritrovo.
	if v.metodo == zip.Deflate {
		cr := &contaByte{r: bufio.NewReaderSize(io.NewSectionReader(f, v.inizioDati, dim-v.inizioDati), 256<<10)}
		crc, n, err := controllaDati(ctx, cr, zip.Deflate)
		if err != nil {
			return v, "dati rovinati"
		}
		v.comp, v.decomp = uint64(cr.n), uint64(n)
		fine, ok := descrittore(f, dim, v.inizioDati+cr.n, crc, v.comp, v.decomp, zip64)
		if !ok {
			// Descrittore mancante o rovinato: accetto solo se il CRC dell'intestazione
			// c'è e torna (il deflate da solo non ha una somma di controllo).
			if v.crc == 0 || v.crc != crc {
				return v, "dati rovinati"
			}
			fine = v.inizioDati + cr.n
		}
		v.crc, v.fine = crc, fine
		return v, ""
	}
	// Store con descrittore: cerco "PK\x07\x08" seguito dalla dimensione giusta.
	for p := v.inizioDati; ; {
		q, ok := cerca.prossima(p, firmaDescrittore)
		if !ok || ctx.Err() != nil {
			return v, "incompleto"
		}
		comp := uint64(q - v.inizioDati)
		b := make([]byte, 20)
		k, _ := f.ReadAt(b, q)
		b = b[:k]
		if len(b) >= 16 && uint64(le.Uint32(b[8:])) == comp && uint64(le.Uint32(b[12:])) == comp ||
			len(b) >= 20 && le.Uint64(b[8:]) == comp {
			crc, n, err := controllaDati(ctx, io.NewSectionReader(f, v.inizioDati, int64(comp)), zip.Store)
			if err == nil && crc == le.Uint32(b[4:]) {
				v.comp, v.decomp, v.crc = comp, uint64(n), crc
				if len(b) >= 16 && uint64(le.Uint32(b[8:])) == comp {
					v.fine = q + 16
				} else {
					v.fine = q + 24
				}
				return v, ""
			}
		}
		p = q + 4
	}
}

// descrittore: dopo i dati c'è (forse con la firma) crc, compresso, decompresso. Restituisce
// dove finisce.
func descrittore(f io.ReaderAt, dim, at int64, crc uint32, comp, decomp uint64, zip64 bool) (int64, bool) {
	le := binary.LittleEndian
	b := make([]byte, 28)
	k, _ := f.ReadAt(b, at)
	b = b[:k]
	prova := func(d []byte, lung int64) (int64, bool) {
		if zip64 && len(d) >= 20 && le.Uint32(d) == crc && le.Uint64(d[4:]) == comp && le.Uint64(d[12:]) == decomp {
			return at + lung + 20, true
		}
		if len(d) >= 12 && le.Uint32(d) == crc && uint64(le.Uint32(d[4:])) == comp&0xFFFFFFFF && uint64(le.Uint32(d[8:])) == decomp&0xFFFFFFFF {
			return at + lung + 12, true
		}
		if len(d) >= 20 && le.Uint32(d) == crc && le.Uint64(d[4:]) == comp && le.Uint64(d[12:]) == decomp {
			return at + lung + 20, true
		}
		return 0, false
	}
	if len(b) >= 4 && string(b[:4]) == firmaDescrittore {
		if e, ok := prova(b[4:], 4); ok {
			return e, true
		}
	}
	if e, ok := prova(b, 0); ok {
		return e, true
	}
	return 0, false
}

// controllaDati decomprime e restituisce CRC32 e byte decompressi.
func controllaDati(ctx context.Context, r io.Reader, metodo uint16) (uint32, int64, error) {
	var src io.Reader = r
	if metodo == zip.Deflate {
		fr := flate.NewReader(r)
		defer fr.Close()
		src = fr
	}
	h := crc32.NewIEEE()
	n, err := io.Copy(h, &lettoreAnnullabile{ctx: ctx, r: src})
	return h.Sum32(), n, err
}

// contaByte conta i byte letti davvero dal decompressore (che usa ReadByte: non legge oltre).
type contaByte struct {
	r *bufio.Reader
	n int64
}

func (c *contaByte) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func (c *contaByte) ReadByte() (byte, error) {
	b, err := c.r.ReadByte()
	if err == nil {
		c.n++
	}
	return b, err
}

// cercatore trova le firme "PK.." nel file, a blocchi.
type cercatore struct {
	f   io.ReaderAt
	dim int64
	buf []byte
}

func newCercatore(f io.ReaderAt, dim int64) *cercatore {
	return &cercatore{f: f, dim: dim, buf: make([]byte, 1<<20)}
}

func (c *cercatore) prossima(da int64, firma string) (int64, bool) {
	fb := []byte(firma)
	for da < c.dim {
		n, err := c.f.ReadAt(c.buf, da)
		if n <= 0 {
			_ = err
			return 0, false
		}
		if i := bytes.Index(c.buf[:n], fb); i >= 0 {
			return da + int64(i), true
		}
		if int64(n) < int64(len(c.buf)) {
			return 0, false
		}
		da += int64(n) - int64(len(fb)-1)
	}
	return 0, false
}
