package unreal

// Scritte dentro gli asset (tabelle di testo, widget, oggetti): in Unreal ogni scritta visibile è
// un FText. Nei file cotti un FText "di base" è scritto così:
//
//	uint32 flags, int8 tipo (0 = base), FString namespace, FString chiave, FString testo
//
// Il gioco mostra "testo" (se il file di lingua .locres non ha già una traduzione per quella
// chiave). Qui trovo questi FText dentro un .uexp, cambio il testo e sistemo le grandezze scritte
// nel .uasset (grandezza e posizione di ogni "export"), così il file resta valido.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sort"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// PezzoTesto: un testo trovato. Pos e Fine delimitano l'FString del testo (prefisso compreso).
type PezzoTesto struct {
	Pos, Fine int
	Chiave    string
	Testo     string
	Etichetta int // posizione dell'int32 "grandezza" della proprietà (formato con etichette), -1 se non c'è
}

// leggiFS: FString alla posizione p. ok=false se non sembra una stringa vera.
func leggiFS(b []byte, p int, maxLen int) (s string, fine int, ok bool) {
	if p < 0 || p+4 > len(b) {
		return "", 0, false
	}
	n := int(int32(binary.LittleEndian.Uint32(b[p:])))
	p += 4
	switch {
	case n == 0:
		return "", p, true
	case n > 0:
		if n > maxLen || p+n > len(b) || b[p+n-1] != 0 {
			return "", 0, false
		}
		raw := b[p : p+n-1]
		if !utf8.Valid(raw) && !isLatin1Testo(raw) {
			return "", 0, false
		}
		r := make([]rune, len(raw))
		for i, c := range raw {
			r[i] = rune(c)
			if c < 0x20 && c != '\n' && c != '\r' && c != '\t' {
				return "", 0, false
			}
			if c == 0 {
				return "", 0, false
			}
		}
		return string(r), p + n, true
	default:
		n = -n
		if n > maxLen || p+2*n > len(b) || b[p+2*n-2] != 0 || b[p+2*n-1] != 0 {
			return "", 0, false
		}
		u := make([]uint16, n-1)
		for i := range u {
			u[i] = binary.LittleEndian.Uint16(b[p+2*i:])
			if u[i] == 0 || (u[i] < 0x20 && u[i] != '\n' && u[i] != '\r' && u[i] != '\t') {
				return "", 0, false
			}
		}
		return string(utf16.Decode(u)), p + 2*n, true
	}
}

func isLatin1Testo(b []byte) bool {
	for _, c := range b {
		if c >= 0x80 && c < 0xA0 {
			return false
		}
	}
	return true
}

// TrovaTesti cerca gli FText di base in un .uexp.
func TrovaTesti(b []byte) []PezzoTesto {
	var out []PezzoTesto
	for i := 0; i+5+12 <= len(b); i++ {
		if b[i+4] != 0 { // tipo "base"
			continue
		}
		flags := binary.LittleEndian.Uint32(b[i:])
		if flags > 0xFF {
			continue
		}
		_, p, ok := leggiFS(b, i+5, 512) // namespace
		if !ok {
			continue
		}
		chiave, p2, ok := leggiFS(b, p, 512)
		if !ok || chiave == "" {
			continue
		}
		testo, p3, ok := leggiFS(b, p2, 1<<16)
		if !ok || testo == "" || !haLettere(testo) {
			continue
		}
		pz := PezzoTesto{Pos: p2, Fine: p3, Chiave: chiave, Testo: testo, Etichetta: -1}
		// formato con etichette: [int32 grandezza][int32 indice][uint8 guid=0] subito prima
		if i >= 9 && b[i-1] == 0 && binary.LittleEndian.Uint32(b[i-5:]) == 0 && int(binary.LittleEndian.Uint32(b[i-9:])) == p3-i {
			pz.Etichetta = i - 9
		}
		out = append(out, pz)
		i = p3 - 1
	}
	return out
}

func haLettere(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func codificaFS(s string) []byte {
	var w scrittore
	if s == "" {
		w.u32(0)
		return w.Bytes()
	}
	w.fstring(s)
	return w.Bytes()
}

// cambio: a Pos (nel .uexp) la grandezza cambia di Delta byte.
type cambio struct{ Pos, Delta int }

// SostituisciTesti: nuovo .uexp con i testi cambiati (nuovi[i] per pezzi[i], "" = lascia).
func SostituisciTesti(uexp []byte, pezzi []PezzoTesto, nuovi []string) ([]byte, []cambio) {
	var out bytes.Buffer
	var cambi []cambio
	prec := 0
	ordine := make([]int, len(pezzi))
	for i := range ordine {
		ordine[i] = i
	}
	sort.Slice(ordine, func(a, b int) bool { return pezzi[ordine[a]].Pos < pezzi[ordine[b]].Pos })
	// prima le etichette: le grandezze delle proprietà cambiano insieme al testo
	etich := map[int]int{}
	for _, i := range ordine {
		if nuovi[i] == "" || nuovi[i] == pezzi[i].Testo {
			continue
		}
		d := len(codificaFS(nuovi[i])) - (pezzi[i].Fine - pezzi[i].Pos)
		if pezzi[i].Etichetta >= 0 {
			etich[pezzi[i].Etichetta] += d
		}
	}
	modificato := append([]byte(nil), uexp...)
	for pos, d := range etich {
		v := int32(binary.LittleEndian.Uint32(modificato[pos:]))
		binary.LittleEndian.PutUint32(modificato[pos:], uint32(v+int32(d)))
	}
	for _, i := range ordine {
		p := pezzi[i]
		if nuovi[i] == "" || nuovi[i] == p.Testo || p.Pos < prec {
			continue
		}
		nuovo := codificaFS(nuovi[i])
		out.Write(modificato[prec:p.Pos])
		out.Write(nuovo)
		prec = p.Fine
		if d := len(nuovo) - (p.Fine - p.Pos); d != 0 {
			cambi = append(cambi, cambio{Pos: p.Pos, Delta: d})
		}
	}
	out.Write(modificato[prec:])
	return out.Bytes(), cambi
}

// CorreggiUasset: aggiorna nel .uasset le grandezze e le posizioni degli export e la posizione
// dei dati "bulk" dopo i cambi fatti nel .uexp (vecchio = grandezza del .uexp prima).
// Gli export si trovano senza leggere tutta l'intestazione: nei pacchetti cotti divisi il primo
// export comincia esattamente alla fine del .uasset, e ogni export comincia dove finisce l'altro.
func CorreggiUasset(uasset []byte, vecchioUexp int, cambi []cambio) ([]byte, error) {
	if len(cambi) == 0 {
		return uasset, nil
	}
	h := int64(len(uasset))
	out := append([]byte(nil), uasset...)
	// cerco tutte le coppie (SerialSize, SerialOffset) con SerialOffset = h
	type exp struct {
		pos       int // posizione di SerialSize
		size, off int64
	}
	var primi []int
	for p := 8; p+8 <= len(out); p++ {
		if int64(binary.LittleEndian.Uint64(out[p:])) == h {
			s := int64(binary.LittleEndian.Uint64(out[p-8:]))
			if s > 0 && s <= int64(vecchioUexp) {
				primi = append(primi, p-8)
			}
		}
	}
	if len(primi) == 0 {
		return nil, errors.New("tabella degli export non trovata")
	}
	var lista []exp
	trovata := false
	for _, p0 := range primi {
		s0 := int64(binary.LittleEndian.Uint64(out[p0:]))
		lista = []exp{{p0, s0, h}}
		if h+s0 >= h+int64(vecchioUexp)-4 {
			trovata = true // un solo export
			break
		}
		// passo tra un export e l'altro: cerco il prossimo SerialOffset = h+s0
		for passo := 60; passo <= 200 && !trovata; passo += 4 {
			l := []exp{{p0, s0, h}}
			fine := h + s0
			for {
				q := l[len(l)-1].pos + passo
				if q+16 > len(out) {
					break
				}
				s := int64(binary.LittleEndian.Uint64(out[q:]))
				o := int64(binary.LittleEndian.Uint64(out[q+8:]))
				if o != fine || s <= 0 {
					break
				}
				l = append(l, exp{q, s, o})
				fine = o + s
			}
			if fine >= h+int64(vecchioUexp)-4 && fine <= h+int64(vecchioUexp) {
				lista, trovata = l, true
			}
		}
		if trovata {
			break
		}
	}
	if !trovata {
		return nil, errors.New("export non coerenti con il file dei dati")
	}
	totale := 0
	for _, c := range cambi {
		totale += c.Delta
	}
	for _, e := range lista {
		inizio, fine := e.off-h, e.off-h+e.size // nel .uexp
		dSize, dOff := 0, 0
		for _, c := range cambi {
			if int64(c.Pos) < inizio {
				dOff += c.Delta
			} else if int64(c.Pos) < fine {
				dSize += c.Delta
			}
		}
		binary.LittleEndian.PutUint64(out[e.pos:], uint64(e.size+int64(dSize)))
		binary.LittleEndian.PutUint64(out[e.pos+8:], uint64(e.off+int64(dOff)))
	}
	// BulkDataStartOffset (int64) nell'intestazione: di solito = fine degli export
	finale := lista[len(lista)-1].off + lista[len(lista)-1].size
	limite := lista[0].pos
	for p := 0; p+8 <= limite; p++ {
		v := int64(binary.LittleEndian.Uint64(out[p:]))
		if v == finale || v == h+int64(vecchioUexp) {
			binary.LittleEndian.PutUint64(out[p:], uint64(v+int64(totale)))
			p += 7
		}
	}
	return out, nil
}
