package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

// Tipi di risorsa di Windows usati qui.
const (
	RT_ICON       = 3
	RT_GROUP_ICON = 14
	RT_VERSION    = 16
	RT_MANIFEST   = 24
)

// Lingua delle risorse (inglese USA, come fanno rsrc e windres): Windows trova
// comunque icona e manifest qualunque sia la lingua del sistema.
const linguaRisorse = 0x0409

// Risorsa è un singolo blocco di dati con tipo e numero (ID).
type Risorsa struct {
	Tipo uint32
	ID   uint32
	Dati []byte
}

// ScriviSyso costruisce un file oggetto COFF (.syso) con una sola sezione .rsrc.
// Il compilatore Go, quando trova un .syso nella cartella del pacchetto, lo unisce
// all'exe e mette la sezione .rsrc nel posto giusto (risorse di Windows).
//
// Struttura del file:
//
//	intestazione COFF (20 byte)
//	intestazione della sezione .rsrc (40 byte)
//	dati della sezione: albero tipo > ID > lingua, voci dati, blocchi di dati
//	rilocazioni (10 byte l'una): una per ogni voce dati, perché l'indirizzo
//	  dei dati (RVA) si conosce solo quando il linker sistema la sezione nell'exe
//	tabella dei simboli (un simbolo, ".rsrc") e tabella delle stringhe (vuota)
func ScriviSyso(ris []Risorsa, macchina uint16) ([]byte, error) {
	if len(ris) == 0 {
		return nil, errors.New("nessuna risorsa da scrivere")
	}
	// ordina per tipo e poi per ID: Windows cerca le voci con una ricerca binaria
	ris = append([]Risorsa(nil), ris...)
	sort.Slice(ris, func(i, j int) bool {
		if ris[i].Tipo != ris[j].Tipo {
			return ris[i].Tipo < ris[j].Tipo
		}
		return ris[i].ID < ris[j].ID
	})
	for i := 1; i < len(ris); i++ {
		if ris[i].Tipo == ris[i-1].Tipo && ris[i].ID == ris[i-1].ID {
			return nil, fmt.Errorf("risorsa doppia: tipo %d, ID %d", ris[i].Tipo, ris[i].ID)
		}
	}
	type gruppo struct {
		tipo uint32
		voci []int // indici in ris
	}
	var gruppi []gruppo
	for i, r := range ris {
		if len(gruppi) == 0 || gruppi[len(gruppi)-1].tipo != r.Tipo {
			gruppi = append(gruppi, gruppo{tipo: r.Tipo})
		}
		g := &gruppi[len(gruppi)-1]
		g.voci = append(g.voci, i)
	}

	const dimDir, dimVoce, dimDati = 16, 8, 16
	// posizioni dentro la sezione
	pos := dimDir + dimVoce*len(gruppi) // dopo la cartella principale
	posTipo := make([]int, len(gruppi))
	for gi, g := range gruppi {
		posTipo[gi] = pos
		pos += dimDir + dimVoce*len(g.voci)
	}
	posLingua := make([]int, len(ris))
	for i := range ris {
		posLingua[i] = pos
		pos += dimDir + dimVoce
	}
	posVoceDati := make([]int, len(ris))
	for i := range ris {
		posVoceDati[i] = pos
		pos += dimDati
	}
	posBlocco := make([]int, len(ris))
	for i, r := range ris {
		pos = (pos + 7) &^ 7
		posBlocco[i] = pos
		pos += len(r.Dati)
	}
	pos = (pos + 7) &^ 7
	sez := make([]byte, pos)
	le := binary.LittleEndian
	scriviDir := func(at, nID int) {
		// Characteristics, TimeDateStamp, Major, Minor = 0; nomi = 0; ID = nID
		le.PutUint16(sez[at+14:], uint16(nID))
	}
	scriviDir(0, len(gruppi))
	for gi, g := range gruppi {
		v := dimDir + dimVoce*gi
		le.PutUint32(sez[v:], g.tipo)
		le.PutUint32(sez[v+4:], 0x80000000|uint32(posTipo[gi]))
		scriviDir(posTipo[gi], len(g.voci))
		for k, ri := range g.voci {
			v := posTipo[gi] + dimDir + dimVoce*k
			le.PutUint32(sez[v:], ris[ri].ID)
			le.PutUint32(sez[v+4:], 0x80000000|uint32(posLingua[ri]))
		}
	}
	for i, r := range ris {
		scriviDir(posLingua[i], 1)
		v := posLingua[i] + dimDir
		le.PutUint32(sez[v:], linguaRisorse)
		le.PutUint32(sez[v+4:], uint32(posVoceDati[i])) // foglia: senza il bit alto
		d := posVoceDati[i]
		le.PutUint32(sez[d:], uint32(posBlocco[i])) // il linker aggiunge l'indirizzo della sezione
		le.PutUint32(sez[d+4:], uint32(len(r.Dati)))
		copy(sez[posBlocco[i]:], r.Dati)
	}

	var tipoRil uint16
	switch macchina {
	case pe.IMAGE_FILE_MACHINE_AMD64:
		tipoRil = 0x03 // IMAGE_REL_AMD64_ADDR32NB
	case pe.IMAGE_FILE_MACHINE_ARM64:
		tipoRil = 0x02 // IMAGE_REL_ARM64_ADDR32NB
	case pe.IMAGE_FILE_MACHINE_I386:
		tipoRil = 0x07 // IMAGE_REL_I386_DIR32NB
	default:
		return nil, fmt.Errorf("architettura %#x non gestita", macchina)
	}

	const dimTesta, dimSezione = 20, 40
	inizioDati := dimTesta + dimSezione
	inizioRil := inizioDati + len(sez)
	inizioSimboli := inizioRil + 10*len(ris)

	var out bytes.Buffer
	w := func(v any) { _ = binary.Write(&out, le, v) }
	// intestazione COFF
	w(macchina)
	w(uint16(1))             // una sezione
	w(uint32(0))             // data: 0 così il file è sempre uguale
	w(uint32(inizioSimboli)) // tabella dei simboli
	w(uint32(1))             // un simbolo
	w(uint16(0))             // nessuna intestazione opzionale
	w(uint16(0x0104))        // 32BIT_MACHINE | LINE_NUMS_STRIPPED, come windres
	// intestazione della sezione
	out.WriteString(".rsrc\x00\x00\x00")
	w(uint32(0))          // VirtualSize
	w(uint32(0))          // VirtualAddress
	w(uint32(len(sez)))   // SizeOfRawData
	w(uint32(inizioDati)) // PointerToRawData
	w(uint32(inizioRil))  // PointerToRelocations
	w(uint32(0))          // PointerToLinenumbers
	w(uint16(len(ris)))   // NumberOfRelocations
	w(uint16(0))          // NumberOfLinenumbers
	w(uint32(0x40000040)) // INITIALIZED_DATA | MEM_READ
	out.Write(sez)
	for i := range ris {
		w(uint32(posVoceDati[i])) // dove sta l'indirizzo da sistemare
		w(uint32(0))              // simbolo 0 = inizio della sezione .rsrc
		w(tipoRil)
	}
	// simbolo ".rsrc" (statico, sezione 1, valore 0)
	out.WriteString(".rsrc\x00\x00\x00")
	w(uint32(0))
	w(uint16(1))
	w(uint16(0))
	w(uint8(3)) // IMAGE_SYM_CLASS_STATIC
	w(uint8(0))
	// tabella delle stringhe vuota: solo la sua lunghezza (4)
	w(uint32(4))
	return out.Bytes(), nil
}

// LeggiRisorse legge le risorse da un exe (PE) o da un file .syso (COFF).
func LeggiRisorse(b []byte) ([]Risorsa, error) {
	f, err := pe.NewFile(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("file non leggibile: %w", err)
	}
	defer f.Close()
	var sez *pe.Section
	var rvaRisorse uint32
	if f.OptionalHeader != nil {
		// exe: la cartella delle risorse è la voce 2 della tabella delle cartelle dati
		switch oh := f.OptionalHeader.(type) {
		case *pe.OptionalHeader64:
			if len(oh.DataDirectory) > 2 {
				rvaRisorse = oh.DataDirectory[2].VirtualAddress
			}
		case *pe.OptionalHeader32:
			if len(oh.DataDirectory) > 2 {
				rvaRisorse = oh.DataDirectory[2].VirtualAddress
			}
		}
		if rvaRisorse == 0 {
			return nil, errors.New("l'exe non contiene risorse")
		}
		for _, s := range f.Sections {
			if rvaRisorse >= s.VirtualAddress && rvaRisorse < s.VirtualAddress+max(s.VirtualSize, s.Size) {
				sez = s
				break
			}
		}
	} else {
		sez = f.Section(".rsrc")
	}
	if sez == nil {
		return nil, errors.New("sezione delle risorse non trovata")
	}
	dati, err := sez.Data()
	if err != nil {
		return nil, err
	}
	base := uint32(0) // in un .syso gli indirizzi sono già relativi alla sezione
	inizio := uint32(0)
	if f.OptionalHeader != nil {
		base = sez.VirtualAddress
		inizio = rvaRisorse - sez.VirtualAddress
	}
	le := binary.LittleEndian
	leggiDir := func(at uint32) ([][2]uint32, error) {
		if uint64(at)+16 > uint64(len(dati)) {
			return nil, errors.New("cartella delle risorse fuori dalla sezione")
		}
		n := uint32(le.Uint16(dati[at+12:])) + uint32(le.Uint16(dati[at+14:]))
		if uint64(at)+16+8*uint64(n) > uint64(len(dati)) {
			return nil, errors.New("cartella delle risorse troncata")
		}
		out := make([][2]uint32, n)
		for i := uint32(0); i < n; i++ {
			v := at + 16 + 8*i
			out[i] = [2]uint32{le.Uint32(dati[v:]), le.Uint32(dati[v+4:])}
		}
		return out, nil
	}
	var out []Risorsa
	tipi, err := leggiDir(inizio)
	if err != nil {
		return nil, err
	}
	for _, t := range tipi {
		if t[1]&0x80000000 == 0 {
			continue
		}
		ids, err := leggiDir(inizio + t[1]&0x7FFFFFFF)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if id[1]&0x80000000 == 0 {
				continue
			}
			lingue, err := leggiDir(inizio + id[1]&0x7FFFFFFF)
			if err != nil {
				return nil, err
			}
			for _, l := range lingue {
				d := inizio + l[1]
				if l[1]&0x80000000 != 0 || uint64(d)+16 > uint64(len(dati)) {
					return nil, errors.New("voce dati non valida")
				}
				rva, dim := le.Uint32(dati[d:]), le.Uint32(dati[d+4:])
				off := rva - base
				if f.OptionalHeader == nil {
					off = rva + inizio
				}
				if rva < base || uint64(off)+uint64(dim) > uint64(len(dati)) {
					return nil, fmt.Errorf("dati della risorsa %d/%d fuori dalla sezione", t[0], id[0])
				}
				out = append(out, Risorsa{Tipo: t[0], ID: id[0], Dati: dati[off : off+dim]})
			}
		}
	}
	return out, nil
}
