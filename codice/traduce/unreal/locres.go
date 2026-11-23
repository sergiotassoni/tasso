package unreal

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// File di lingua di Unreal (.locres). Dalla versione 1 le frasi stanno tutte in un elenco in
// fondo al file: per tradurre cambio solo quell'elenco e lascio identico tutto il resto
// (chiavi, codici delle frasi originali), così il gioco le riconosce come sue.

var magiaLocres = []byte{0x0E, 0x14, 0x74, 0x75, 0x67, 0x4A, 0x03, 0xFC, 0x4A, 0x15, 0x90, 0x9D, 0xC3, 0x37, 0x7F, 0x1B}

// Locres: un file di lingua letto.
type Locres struct {
	Versione int
	testa    []byte   // tutto quello che viene prima dell'elenco delle frasi
	Frasi    []string // le frasi, nell'ordine del file
	conti    []int32  // quante volte è usata ogni frase (dalla versione 2)
}

// LeggiLocres legge un .locres versione 1, 2 o 3.
func LeggiLocres(b []byte) (*Locres, error) {
	if len(b) < 25 || !bytes.Equal(b[:16], magiaLocres) {
		return nil, errors.New("file di lingua di tipo vecchio: non supportato")
	}
	l := &Locres{Versione: int(b[16])}
	if l.Versione < 1 || l.Versione > 3 {
		return nil, errors.New("versione del file di lingua non supportata")
	}
	off := int64(binary.LittleEndian.Uint64(b[17:25]))
	if off < 25 || off >= int64(len(b)) {
		return nil, errors.New("file di lingua non valido")
	}
	l.testa = append([]byte(nil), b[:off]...)
	r := &lettore{b: b, pos: int(off)}
	n := r.i32()
	if n < 0 || n > 5_000_000 {
		return nil, errors.New("file di lingua non valido")
	}
	for k := int32(0); k < n && r.err == nil; k++ {
		l.Frasi = append(l.Frasi, r.fstring())
		if l.Versione >= 2 {
			l.conti = append(l.conti, r.i32())
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	return l, nil
}

// Scrivi: il file con le frasi attuali (stessa testa, stesso numero di frasi).
func (l *Locres) Scrivi() []byte {
	var w scrittore
	w.Write(l.testa)
	w.u32(uint32(len(l.Frasi)))
	for i, s := range l.Frasi {
		w.fstring(s)
		if l.Versione >= 2 {
			c := int32(1)
			if i < len(l.conti) {
				c = l.conti[i]
			}
			w.u32(uint32(c))
		}
	}
	return w.Bytes()
}
