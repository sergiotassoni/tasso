// Package fotopulita toglie da una foto i dati nascosti che non servono a vedere l'immagine:
// EXIF (posizione GPS, modello e numero di serie del telefono, data e ora), XMP, IPTC e commenti.
// Lo usa "Scrivici" prima di mandare una foto: chi la riceve vede l'immagine e basta.
//
// Non ricomprime niente (la foto resta identica pixel per pixel): toglie solo i pezzi di
// metadati da JPEG, PNG e WEBP. GIF e BMP non hanno EXIF e restano come sono.
// Se il file non si legge bene come immagine, Pulisci restituisce un errore: meglio non
// mandare la foto che mandarla con la posizione dentro.
package fotopulita

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// ErrRovinata: la foto non ha la struttura che dice di avere.
var ErrRovinata = errors.New("foto rovinata o in un formato strano")

// Pulisci restituisce la foto senza metadati. tipo è il tipo MIME (image/jpeg, image/png...).
func Pulisci(b []byte, tipo string) ([]byte, error) {
	switch tipo {
	case "image/jpeg":
		return pulisciJPEG(b)
	case "image/png":
		return pulisciPNG(b)
	case "image/webp":
		return pulisciWEBP(b)
	}
	return b, nil
}

// pulisciJPEG toglie APP1 (EXIF, XMP), APP13 (IPTC/Photoshop), da APP3 a APP12, APP15 e i
// commenti (COM). Restano APP0 (JFIF), APP2 (profilo colore ICC) e APP14 (Adobe: serve a
// leggere bene i colori), e tutto quello che c'è dall'inizio dell'immagine (SOS) in poi.
func pulisciJPEG(b []byte) ([]byte, error) {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return nil, ErrRovinata
	}
	out := make([]byte, 0, len(b))
	out = append(out, 0xFF, 0xD8)
	i := 2
	for {
		if i+4 > len(b) || b[i] != 0xFF {
			return nil, ErrRovinata
		}
		m := b[i+1]
		if m == 0xFF { // riempimento
			i++
			continue
		}
		if m == 0xD9 { // fine immagine senza dati: strano, ma va bene così
			return append(out, b[i:]...), nil
		}
		if m >= 0xD0 && m <= 0xD7 || m == 0x01 { // segni senza lunghezza
			out = append(out, b[i:i+2]...)
			i += 2
			continue
		}
		n := int(binary.BigEndian.Uint16(b[i+2 : i+4]))
		if n < 2 || i+2+n > len(b) {
			return nil, ErrRovinata
		}
		fine := i + 2 + n
		if m == 0xDA { // SOS: da qui in poi sono i dati dell'immagine, li copio così come sono
			return append(out, b[i:]...), nil
		}
		togli := m == 0xFE || (m >= 0xE1 && m <= 0xEF && m != 0xE2 && m != 0xEE)
		if !togli {
			out = append(out, b[i:fine]...)
		}
		i = fine
	}
}

var firmaPNG = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}

// pulisciPNG toglie i blocchi di testo (tEXt, zTXt, iTXt: lì finiscono XMP e i commenti),
// eXIf e tIME. Gli altri restano (anche quelli sconosciuti necessari: lettera iniziale maiuscola).
func pulisciPNG(b []byte) ([]byte, error) {
	if len(b) < 8 || !bytes.Equal(b[:8], firmaPNG) {
		return nil, ErrRovinata
	}
	out := make([]byte, 0, len(b))
	out = append(out, firmaPNG...)
	i := 8
	for i < len(b) {
		if i+12 > len(b) {
			return nil, ErrRovinata
		}
		n := int(binary.BigEndian.Uint32(b[i : i+4]))
		if n < 0 || n > len(b) || i+12+n > len(b) {
			return nil, ErrRovinata
		}
		tipo := string(b[i+4 : i+8])
		fine := i + 12 + n
		switch tipo {
		case "tEXt", "zTXt", "iTXt", "eXIf", "tIME":
		default:
			out = append(out, b[i:fine]...)
		}
		i = fine
		if tipo == "IEND" {
			return out, nil
		}
	}
	return nil, ErrRovinata
}

// pulisciWEBP toglie i blocchi EXIF e XMP e spegne i loro segni nell'intestazione VP8X.
func pulisciWEBP(b []byte) ([]byte, error) {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		return nil, ErrRovinata
	}
	corpo := make([]byte, 0, len(b))
	corpo = append(corpo, "WEBP"...)
	i := 12
	for i < len(b) {
		if i+8 > len(b) {
			return nil, ErrRovinata
		}
		tipo := string(b[i : i+4])
		n := int(binary.LittleEndian.Uint32(b[i+4 : i+8]))
		pad := n & 1
		if n < 0 || i+8+n > len(b) {
			return nil, ErrRovinata
		}
		fine := i + 8 + n + pad
		if fine > len(b) {
			fine = len(b) // l'ultimo byte di riempimento a volte manca
		}
		switch tipo {
		case "EXIF", "XMP ":
		case "VP8X":
			pezzo := append([]byte(nil), b[i:fine]...)
			if n >= 1 {
				pezzo[8] &^= 0x08 | 0x04 // segni "c'è EXIF" e "c'è XMP"
			}
			corpo = append(corpo, pezzo...)
		default:
			corpo = append(corpo, b[i:fine]...)
		}
		i = fine
	}
	out := make([]byte, 8, 8+len(corpo))
	copy(out, "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(corpo)))
	return append(out, corpo...), nil
}
