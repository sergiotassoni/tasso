// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

// foto.go: le foto del telefono (JPG, PNG; HEIC su Windows se il PC sa leggerlo) diventano
// pagine A4, girate per il verso giusto (l'orientamento scritto dal telefono nei dati EXIF).

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// TipoFile: "pdf", "jpg", "png", "heic" o "" (sconosciuto), guardando i primi byte.
func TipoFile(b []byte) string {
	switch {
	case sembraPDF(b):
		return "pdf"
	case len(b) > 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return "jpg"
	case len(b) > 8 && bytes.Equal(b[:8], []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case len(b) > 12 && string(b[4:8]) == "ftyp":
		marca := string(b[8:12])
		for _, m := range []string{"heic", "heix", "hevc", "hevx", "heim", "heis", "mif1", "msf1", "avif"} {
			if marca == m {
				return "heic"
			}
		}
	}
	return ""
}

// orientamentoEXIF: il valore 1..8 del tag Orientation (0x0112) di un JPEG, 1 se non c'è.
func orientamentoEXIF(b []byte) int {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return 1
		}
		marker := b[i+1]
		if marker == 0xD8 || (marker >= 0xD0 && marker <= 0xD7) || marker == 0x01 {
			i += 2
			continue
		}
		if marker == 0xDA || marker == 0xD9 { // inizio dell'immagine vera: niente EXIF
			return 1
		}
		l := int(binary.BigEndian.Uint16(b[i+2:]))
		if l < 2 || i+2+l > len(b) {
			return 1
		}
		seg := b[i+4 : i+2+l]
		if marker == 0xE1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			return leggiTIFFOrientamento(seg[6:])
		}
		i += 2 + l
	}
	return 1
}

func leggiTIFFOrientamento(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	off := int(bo.Uint32(t[4:]))
	if off < 8 || off+2 > len(t) {
		return 1
	}
	n := int(bo.Uint16(t[off:]))
	for k := 0; k < n && k < 512; k++ {
		e := off + 2 + k*12
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 {
			v := int(bo.Uint16(t[e+8:]))
			if v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// specchia: applica all'immagine le trasformazioni EXIF con lo specchio (2, 4, 5, 7), rare.
func specchia(src image.Image, o int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	var dst *image.RGBA
	if o == 5 || o == 7 {
		dst = image.NewRGBA(image.Rect(0, 0, h, w))
	} else {
		dst = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := src.At(b.Min.X+x, b.Min.Y+y)
			switch o {
			case 2: // specchio orizzontale
				dst.Set(w-1-x, y, c)
			case 4: // specchio verticale
				dst.Set(x, h-1-y, c)
			case 5: // trasposta
				dst.Set(y, x, c)
			case 7: // trasversa
				dst.Set(h-1-y, w-1-x, c)
			default:
				dst.Set(x, y, c)
			}
		}
	}
	return dst
}

// decodificaHEIC è impostata su Windows (heic_windows.go): usa i componenti di Windows.
var decodificaHEIC func([]byte) (image.Image, error)

// FotoInPDF: una foto diventa un PDF di una pagina A4 (verticale o orizzontale come la foto).
func FotoInPDF(data []byte) ([]byte, error) {
	tipo := TipoFile(data)
	rot := 0
	switch tipo {
	case "jpg":
		switch o := orientamentoEXIF(data); o {
		case 3:
			rot = 180
		case 6:
			rot = 90
		case 8:
			rot = 270
		case 2, 4, 5, 7:
			img, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				return nil, errc("foto", err)
			}
			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, specchia(img, o), &jpeg.Options{Quality: 92}); err != nil {
				return nil, errc("foto", err)
			}
			data = buf.Bytes()
		}
	case "png":
	case "heic":
		if decodificaHEIC == nil {
			return nil, errc("heic", nil)
		}
		img, err := decodificaHEIC(data)
		if err != nil {
			return nil, errc("heic", err)
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92}); err != nil {
			return nil, errc("foto", err)
		}
		data = buf.Bytes()
	default:
		return nil, errc("formato", nil)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, errc("foto", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 20000 || cfg.Height > 20000 {
		return nil, errc("foto", nil)
	}
	a4 := types.PaperSize["A4"]
	dim := &types.Dim{Width: a4.Width, Height: a4.Height}
	if cfg.Width > cfg.Height { // foto orizzontale: pagina orizzontale
		dim.Width, dim.Height = a4.Height, a4.Width
	}
	imp := &pdfcpu.Import{
		PageDim:  dim,
		PageSize: "A4",
		UserDim:  true,
		Pos:      types.Center,
		Scale:    0.95,
		InpUnit:  types.POINTS,
	}
	var out bytes.Buffer
	conf := confBase("")
	if err := api.ImportImages(context.Background(), nil, &out, []io.Reader{bytes.NewReader(data)}, imp, conf); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "image") {
			return nil, errc("foto", err)
		}
		return nil, classifica(err)
	}
	return ruota(out.Bytes(), rot)
}
