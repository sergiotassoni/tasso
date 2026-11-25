package unreal

// Scrittura dei file .pak di Unreal Engine (la traduzione da mettere dentro il gioco).

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"io"
	"sort"
	"strings"
	"unicode/utf16"
)

// FileNuovo: un file da mettere nel pak nuovo (percorso relativo al montaggio, con '/').
type FileNuovo struct {
	Nome string
	Dati []byte
}

type scrittore struct{ bytes.Buffer }

func (w *scrittore) u8(v uint8)   { w.WriteByte(v) }
func (w *scrittore) u32(v uint32) { _ = binary.Write(w, binary.LittleEndian, v) }
func (w *scrittore) u64(v uint64) { _ = binary.Write(w, binary.LittleEndian, v) }
func (w *scrittore) fstring(s string) {
	if s == "" {
		w.u32(0)
		return
	}
	ascii := true
	for _, c := range s {
		if c > 0x7f {
			ascii = false
			break
		}
	}
	if ascii {
		w.u32(uint32(len(s) + 1))
		w.WriteString(s)
		w.u8(0)
		return
	}
	u := utf16.Encode([]rune(s))
	w.u32(uint32(int32(-(len(u) + 1))))
	for _, c := range u {
		_ = binary.Write(w, binary.LittleEndian, c)
	}
	w.Write([]byte{0, 0})
}

// hashPercorso: FNV-64 del percorso in minuscolo, in UTF-16, come FPakFile::HashPath.
func hashPercorso(percorso string, seme uint64) uint64 {
	const base, primo = 0xcbf29ce484222325, 0x00000100000001b3
	h := uint64(base) + seme
	for _, c := range utf16.Encode([]rune(strings.ToLower(percorso))) {
		for _, b := range []byte{byte(c), byte(c >> 8)} {
			h ^= uint64(b)
			h *= primo
		}
	}
	return h
}

// ScriviPak scrive un pak versione 11 senza compressione e senza cifratura, montato su
// "../../../" (la cartella sopra quella del gioco, come i pak originali).
func ScriviPak(out io.Writer, file []FileNuovo) error {
	const montaggio = "../../../"
	sort.Slice(file, func(i, j int) bool { return file[i].Nome < file[j].Nome })
	var dati scrittore
	var codificate scrittore
	posCodificata := make([]uint32, len(file))
	for i, f := range file {
		off := uint64(dati.Len())
		h := sha1.Sum(f.Dati)
		// intestazione prima dei dati (con offset 0, come UnrealPak)
		dati.u64(0)
		dati.u64(uint64(len(f.Dati)))
		dati.u64(uint64(len(f.Dati)))
		dati.u32(0) // nessuna compressione
		dati.Write(h[:])
		dati.u8(0)  // non cifrato
		dati.u32(0) // grandezza blocco
		dati.Write(f.Dati)
		// voce compatta: offset, grandezza in 32 bit se bastano
		posCodificata[i] = uint32(codificate.Len())
		bits := uint32(0)
		o32, s32 := off <= 0xffffffff, uint64(len(f.Dati)) <= 0xffffffff
		if o32 {
			bits |= 1 << 31
		}
		if s32 {
			bits |= 1<<30 | 1<<29
		}
		codificate.u32(bits)
		if o32 {
			codificate.u32(uint32(off))
		} else {
			codificate.u64(off)
		}
		if s32 {
			codificate.u32(uint32(len(f.Dati)))
		} else {
			codificate.u64(uint64(len(f.Dati)))
		}
	}
	const seme = 0x5441535341 // "TASSA": qualunque valore va bene, è scritto nell'indice
	// indice dei percorsi (codice -> voce)
	var phi scrittore
	phi.u32(uint32(len(file)))
	for i, f := range file {
		phi.u64(hashPercorso(f.Nome, seme))
		phi.u32(posCodificata[i])
	}
	phi.u32(0) // cartelle ridotte: nessuna
	// indice completo delle cartelle (con tutte le cartelle genitrici)
	cartelle := map[string][]int{"/": nil}
	for i, f := range file {
		dir, nome := "/", f.Nome
		if k := strings.LastIndex(f.Nome, "/"); k >= 0 {
			dir, nome = f.Nome[:k+1], f.Nome[k+1:]
		}
		_ = nome
		cartelle[dir] = append(cartelle[dir], i)
		for d := strings.TrimSuffix(dir, "/"); d != ""; {
			k := strings.LastIndex(d, "/")
			if k < 0 {
				break
			}
			d = d[:k]
			if _, ok := cartelle[d+"/"]; !ok {
				cartelle[d+"/"] = nil
			}
		}
		if k := strings.Index(dir, "/"); k >= 0 && k+1 < len(dir) {
			// prima cartella (es. "Pal/")
			if _, ok := cartelle[dir[:k+1]]; !ok {
				cartelle[dir[:k+1]] = nil
			}
		}
	}
	nomiDir := make([]string, 0, len(cartelle))
	for d := range cartelle {
		nomiDir = append(nomiDir, d)
	}
	sort.Strings(nomiDir)
	var fdi scrittore
	fdi.u32(uint32(len(nomiDir)))
	for _, d := range nomiDir {
		fdi.fstring(d)
		fdi.u32(uint32(len(cartelle[d])))
		for _, i := range cartelle[d] {
			nome := file[i].Nome[strings.LastIndex(file[i].Nome, "/")+1:]
			fdi.fstring(nome)
			fdi.u32(posCodificata[i])
		}
	}
	// indice principale
	inizioIndice := uint64(dati.Len())
	var idx scrittore
	idx.fstring(montaggio)
	idx.u32(uint32(len(file)))
	idx.u64(seme)
	// le posizioni dei due indici secondari vengono dopo l'indice principale: calcolo la sua
	// grandezza prima (è fissa a parte le voci codificate)
	grandezzaIdx := uint64(4 + len(montaggio) + 1 + 4 + 8 + 4 + 8 + 8 + 20 + 4 + 8 + 8 + 20 + 4 + codificate.Len() + 4)
	posPhi := inizioIndice + grandezzaIdx
	posFdi := posPhi + uint64(phi.Len())
	hp := sha1.Sum(phi.Bytes())
	hf := sha1.Sum(fdi.Bytes())
	idx.u32(1)
	idx.u64(posPhi)
	idx.u64(uint64(phi.Len()))
	idx.Write(hp[:])
	idx.u32(1)
	idx.u64(posFdi)
	idx.u64(uint64(fdi.Len()))
	idx.Write(hf[:])
	idx.u32(uint32(codificate.Len()))
	idx.Write(codificate.Bytes())
	idx.u32(0) // voci non codificate: nessuna
	if uint64(idx.Len()) != grandezzaIdx {
		panic("grandezza dell'indice calcolata male")
	}
	hi := sha1.Sum(idx.Bytes())
	// piè di pagina v11
	var piede scrittore
	piede.Write(make([]byte, 16)) // chiave di cifratura: nessuna
	piede.u8(0)                   // indice non cifrato
	piede.u32(magiaPak)
	piede.u32(11)
	piede.u64(inizioIndice)
	piede.u64(uint64(idx.Len()))
	piede.Write(hi[:])
	piede.Write(make([]byte, 5*32)) // nessun metodo di compressione
	for _, b := range [][]byte{dati.Bytes(), idx.Bytes(), phi.Bytes(), fdi.Bytes(), piede.Bytes()} {
		if _, err := out.Write(b); err != nil {
			return err
		}
	}
	return nil
}
