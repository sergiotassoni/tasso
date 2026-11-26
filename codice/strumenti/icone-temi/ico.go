package main

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Immagine è una delle misure contenute in un file .ico.
type Immagine struct {
	Larghezza, Altezza uint8 // 0 = 256
	Colori             uint8
	Piani              uint16
	BitPerPixel        uint16
	Dati               []byte // PNG oppure BMP senza intestazione di file, così come è nel .ico
}

// LeggiICO legge un file .ico (anche con più misure) e restituisce le sue immagini.
//
// Formato: ICONDIR (6 byte) + n * ICONDIRENTRY (16 byte) + dati delle immagini.
func LeggiICO(b []byte) ([]Immagine, error) {
	if len(b) < 6 {
		return nil, errors.New("ico: file troppo corto")
	}
	if binary.LittleEndian.Uint16(b[0:]) != 0 || binary.LittleEndian.Uint16(b[2:]) != 1 {
		return nil, errors.New("ico: non è un file .ico (intestazione sbagliata)")
	}
	n := int(binary.LittleEndian.Uint16(b[4:]))
	if n == 0 {
		return nil, errors.New("ico: nessuna immagine")
	}
	if len(b) < 6+16*n {
		return nil, errors.New("ico: elenco immagini troncato")
	}
	out := make([]Immagine, 0, n)
	for i := 0; i < n; i++ {
		e := b[6+16*i:]
		dim := binary.LittleEndian.Uint32(e[8:])
		off := binary.LittleEndian.Uint32(e[12:])
		if uint64(off)+uint64(dim) > uint64(len(b)) || dim == 0 {
			return nil, fmt.Errorf("ico: immagine %d fuori dal file", i+1)
		}
		out = append(out, Immagine{
			Larghezza:   e[0],
			Altezza:     e[1],
			Colori:      e[2],
			Piani:       binary.LittleEndian.Uint16(e[4:]),
			BitPerPixel: binary.LittleEndian.Uint16(e[6:]),
			Dati:        b[off : off+dim],
		})
	}
	return out, nil
}

// gruppoIcone costruisce il contenuto di una risorsa RT_GROUP_ICON (GRPICONDIR):
// come l'ICONDIR del file .ico ma ogni voce è lunga 14 byte e al posto
// dell'offset c'è l'ID della risorsa RT_ICON.
func gruppoIcone(imm []Immagine, primoID uint16) []byte {
	b := make([]byte, 6+14*len(imm))
	binary.LittleEndian.PutUint16(b[2:], 1)
	binary.LittleEndian.PutUint16(b[4:], uint16(len(imm)))
	for i, im := range imm {
		e := b[6+14*i:]
		e[0], e[1], e[2], e[3] = im.Larghezza, im.Altezza, im.Colori, 0
		binary.LittleEndian.PutUint16(e[4:], im.Piani)
		binary.LittleEndian.PutUint16(e[6:], im.BitPerPixel)
		binary.LittleEndian.PutUint32(e[8:], uint32(len(im.Dati)))
		binary.LittleEndian.PutUint16(e[12:], primoID+uint16(i))
	}
	return b
}

// VoceGruppo è una voce letta da una risorsa RT_GROUP_ICON.
type VoceGruppo struct {
	Larghezza, Altezza uint8
	Dimensione         uint32
	ID                 uint16
}

func leggiGruppoIcone(b []byte) ([]VoceGruppo, error) {
	if len(b) < 6 || binary.LittleEndian.Uint16(b[2:]) != 1 {
		return nil, errors.New("gruppo icone non valido")
	}
	n := int(binary.LittleEndian.Uint16(b[4:]))
	if len(b) < 6+14*n {
		return nil, errors.New("gruppo icone troncato")
	}
	out := make([]VoceGruppo, n)
	for i := range out {
		e := b[6+14*i:]
		out[i] = VoceGruppo{e[0], e[1], binary.LittleEndian.Uint32(e[8:]), binary.LittleEndian.Uint16(e[12:])}
	}
	return out, nil
}
