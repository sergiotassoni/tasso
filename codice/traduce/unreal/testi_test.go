package unreal

import (
	"encoding/binary"
	"testing"
)

func ftext(ns, key, src string) []byte {
	var w scrittore
	w.u32(0)
	w.u8(0)
	w.Write(codificaFS(ns))
	w.Write(codificaFS(key))
	w.Write(codificaFS(src))
	return w.Bytes()
}

func TestTestiInAsset(t *testing.T) {
	// .uexp con due export: il primo con due testi, il secondo con uno; poi la firma finale
	var e1, e2 scrittore
	e1.Write([]byte{1, 2, 3})
	e1.Write(ftext("", "A1B2", "Start Game"))
	e1.Write(ftext("UI", "K2", "Options"))
	e2.Write(ftext("", "K3", "Quit"))
	e2.Write([]byte{9, 9})
	uexp := append(append(append([]byte{}, e1.Bytes()...), e2.Bytes()...), 0xC1, 0x83, 0x2A, 0x9E)
	// .uasset finto: intestazione con BulkDataStartOffset, poi 2 export (passo 72)
	h := 300
	ua := make([]byte, h)
	binary.LittleEndian.PutUint64(ua[40:], uint64(h+len(uexp)-4)) // bulk
	put := func(pos int, size, off int) {
		binary.LittleEndian.PutUint64(ua[pos:], uint64(size))
		binary.LittleEndian.PutUint64(ua[pos+8:], uint64(off))
	}
	put(100, e1.Len(), h)
	put(172, e2.Len(), h+e1.Len())
	pezzi := TrovaTesti(uexp)
	if len(pezzi) != 3 || pezzi[0].Testo != "Start Game" || pezzi[2].Testo != "Quit" {
		t.Fatalf("%+v", pezzi)
	}
	nuovo, cambi := SostituisciTesti(uexp, pezzi, []string{"Inizia la partita", "", "Esci àè"})
	ua2, err := CorreggiUasset(ua, len(uexp), cambi)
	if err != nil {
		t.Fatal(err)
	}
	s1 := int(binary.LittleEndian.Uint64(ua2[100:]))
	s2 := int(binary.LittleEndian.Uint64(ua2[172:]))
	o2 := int(binary.LittleEndian.Uint64(ua2[180:]))
	if o2 != h+s1 || s1+s2+4 != len(nuovo) {
		t.Fatalf("grandezze sbagliate %d %d %d %d", s1, s2, o2, len(nuovo))
	}
	if int(binary.LittleEndian.Uint64(ua2[40:])) != h+len(nuovo)-4 {
		t.Fatal("bulk non aggiornato")
	}
	p2 := TrovaTesti(nuovo)
	if len(p2) != 3 || p2[0].Testo != "Inizia la partita" || p2[1].Testo != "Options" || p2[2].Testo != "Esci àè" {
		t.Fatalf("%+v", p2)
	}
}
