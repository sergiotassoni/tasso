package fotopulita

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func immagine() *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		m.Set(x, x, color.RGBA{200, 10, 10, 255})
	}
	return m
}

const gps = "GPSLatitude 45.4642 N GPSLongitude 9.1900 E SerialNumber 12345"

func TestJPEGSenzaEXIF(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, immagine(), nil); err != nil {
		t.Fatal(err)
	}
	orig := buf.Bytes()
	// inserisco dopo SOI un APP1 "Exif" con dentro la posizione, e un commento
	app1 := append([]byte("Exif\x00\x00"), gps...)
	seg := func(m byte, d []byte) []byte {
		s := []byte{0xFF, m, 0, 0}
		binary.BigEndian.PutUint16(s[2:], uint16(len(d)+2))
		return append(s, d...)
	}
	sporca := append([]byte{0xFF, 0xD8}, seg(0xE1, app1)...)
	sporca = append(sporca, seg(0xFE, []byte("commento "+gps))...)
	sporca = append(sporca, orig[2:]...)
	pulita, err := Pulisci(sporca, "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(pulita, []byte("GPSLatitude")) || bytes.Contains(pulita, []byte("Exif")) {
		t.Fatal("dentro c'è ancora l'EXIF")
	}
	if !bytes.Equal(pulita, orig) {
		t.Fatalf("la foto pulita doveva tornare uguale all'originale (%d contro %d byte)", len(pulita), len(orig))
	}
	if _, err := jpeg.Decode(bytes.NewReader(pulita)); err != nil {
		t.Fatal("non si legge più:", err)
	}
}

func TestPNGSenzaTesti(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, immagine()); err != nil {
		t.Fatal(err)
	}
	orig := buf.Bytes()
	blocco := func(tipo string, d []byte) []byte {
		s := make([]byte, 8)
		binary.BigEndian.PutUint32(s, uint32(len(d)))
		copy(s[4:], tipo)
		s = append(s, d...)
		c := make([]byte, 4)
		binary.BigEndian.PutUint32(c, crcPNG(tipo, d))
		return append(s, c...)
	}
	// dopo IHDR (8 firma + 25 IHDR) metto eXIf e tEXt
	sporca := append([]byte(nil), orig[:33]...)
	sporca = append(sporca, blocco("eXIf", []byte(gps))...)
	sporca = append(sporca, blocco("tEXt", []byte("Comment\x00"+gps))...)
	sporca = append(sporca, orig[33:]...)
	if _, err := png.Decode(bytes.NewReader(sporca)); err != nil {
		t.Fatal("PNG di prova sbagliato:", err)
	}
	pulita, err := Pulisci(sporca, "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(pulita, []byte("GPSLatitude")) || !bytes.Equal(pulita, orig) {
		t.Fatal("i testi dovevano sparire e la foto tornare uguale")
	}
}

func TestWEBPSenzaEXIF(t *testing.T) {
	blocco := func(tipo string, d []byte) []byte {
		s := make([]byte, 8)
		copy(s, tipo)
		binary.LittleEndian.PutUint32(s[4:], uint32(len(d)))
		s = append(s, d...)
		if len(d)%2 == 1 {
			s = append(s, 0)
		}
		return s
	}
	vp8x := blocco("VP8X", []byte{0x08 | 0x04, 0, 0, 0, 7, 0, 0, 7, 0, 0})
	img := blocco("VP8L", []byte{0x2f, 1, 2, 3, 4})
	corpo := append([]byte("WEBP"), vp8x...)
	corpo = append(corpo, img...)
	corpo = append(corpo, blocco("EXIF", []byte(gps))...)
	corpo = append(corpo, blocco("XMP ", []byte("<x>"+gps+"</x>"))...)
	sporca := append([]byte("RIFF\x00\x00\x00\x00"), corpo...)
	binary.LittleEndian.PutUint32(sporca[4:8], uint32(len(corpo)))
	pulita, err := Pulisci(sporca, "image/webp")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(pulita, []byte("GPSLatitude")) {
		t.Fatal("dentro c'è ancora l'EXIF")
	}
	if pulita[20]&0x0C != 0 {
		t.Fatal("i segni EXIF/XMP di VP8X dovevano spegnersi")
	}
	if int(binary.LittleEndian.Uint32(pulita[4:8])) != len(pulita)-8 {
		t.Fatal("lunghezza RIFF sbagliata")
	}
	if !bytes.Contains(pulita, img) {
		t.Fatal("l'immagine è sparita")
	}
}

func TestRovinateRifiutate(t *testing.T) {
	for _, c := range []struct {
		b    []byte
		tipo string
	}{
		{[]byte{0xFF, 0xD8, 0xFF, 0xE1, 0xFF, 0xFF}, "image/jpeg"},
		{[]byte{0xFF, 0xD8, 0x00}, "image/jpeg"},
		{append(append([]byte(nil), firmaPNG...), 0, 0, 0, 200, 'I', 'H', 'D', 'R'), "image/png"},
		{[]byte("RIFF\x10\x00\x00\x00WEBPEXIF\xff\xff\xff\x00"), "image/webp"},
	} {
		if _, err := Pulisci(c.b, c.tipo); err == nil {
			t.Errorf("%s rovinata accettata: %x", c.tipo, c.b)
		}
	}
	if b, err := Pulisci([]byte("GIF89a..."), "image/gif"); err != nil || string(b) != "GIF89a..." {
		t.Error("GIF deve restare com'è")
	}
}

// crcPNG serve solo alle prove (per costruire PNG validi).
func crcPNG(tipo string, dati []byte) uint32 {
	h := crc32.NewIEEE()
	h.Write([]byte(tipo))
	h.Write(dati)
	return h.Sum32()
}
