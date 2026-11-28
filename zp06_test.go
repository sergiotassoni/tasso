// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

// zp06_test.go: CTRL-ZP-06. Cifratura AES-256 con R=6 (ZP06-1), lettura per gli screen reader
// (ZP06-5), contenuto attivo segnalato da Info (ZP06-8).

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"regexp"
	"strconv"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/filter"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func TestZP06CifraturaR6(t *testing.T) {
	b := pdfProva([]pagProva{a4("segreto"), a4("due")})
	for _, o := range []OpzProteggi{
		{Apertura: "città€"},
		{Apertura: "apri", Blocca: true, Permessi: "padrone", Stampa: true},
		{Blocca: true, Permessi: "padrone"},
	} {
		out, err := Proteggi(b, "", o)
		if err != nil {
			t.Fatal(err)
		}
		ctx, err := leggiConPassword(out, o.Apertura)
		if err != nil {
			t.Fatalf("%+v: non si apre con la sua password: %v", o, err)
		}
		if ctx.E == nil || ctx.E.R != 6 || ctx.E.V != 5 {
			t.Fatalf("%+v: E=%+v, serve V=5 R=6", o, ctx.E)
		}
		if o.Apertura != "" {
			if _, err := leggiConPassword(out, "sbagliata"); err == nil {
				t.Errorf("%+v: si apre con una password sbagliata", o)
			}
		}
		if o.Blocca {
			if _, err := leggiConPassword(out, o.Permessi); err != nil {
				t.Errorf("la password del proprietario non funziona: %v", err)
			}
			p := model.PermissionFlags(ctx.E.P)
			if p&model.PermissionExtractRev3 == 0 {
				t.Error("ZP06-5: la lettura per gli screen reader deve restare permessa")
			}
			if p&model.PermissionExtract != 0 {
				t.Error("la copia non scelta non deve essere permessa")
			}
		}
		inf, err := Info(out, o.Apertura)
		if err != nil || inf.Pagine != 2 {
			t.Errorf("Info: %+v %v", inf, err)
		}
	}
}

func TestZP06ContenutoAttivo(t *testing.T) {
	b := pdfProva([]pagProva{a4("uno")})
	if inf, _ := Info(b, ""); inf.Attivo {
		t.Fatal("un PDF semplice non ha contenuto attivo")
	}
	for _, azione := range []types.Dict{
		{"S": types.Name("JavaScript"), "JS": types.StringLiteral("app.alert(1)")},
		{"S": types.Name("Launch"), "F": types.StringLiteral("calc.exe")},
	} {
		ctx, _, err := leggi(b, "")
		if err != nil {
			t.Fatal(err)
		}
		ctx.RootDict["OpenAction"] = azione
		out, err := scrivi(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if inf, _ := Info(out, ""); !inf.Attivo {
			t.Errorf("%v non segnalato", azione["S"])
		}
	}
}

// conASCII85: ogni immagine del PDF con uno strato ASCII85 davanti agli altri filtri (come i PDF
// di ReportLab).
func conASCII85(t *testing.T, b []byte) []byte {
	t.Helper()
	ctx, _, err := leggi(b, "")
	if err != nil {
		t.Fatal(err)
	}
	fatte := 0
	for _, e := range ctx.Table {
		if e == nil || e.Object == nil {
			continue
		}
		sd, ok := e.Object.(types.StreamDict)
		if !ok || sd.Subtype() == nil || *sd.Subtype() != "Image" || len(sd.FilterPipeline) != 1 {
			continue
		}
		f, _ := filter.NewFilter(filter.ASCII85, nil)
		r, err := f.Encode(bytes.NewReader(sd.Raw))
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(r)
		sd.Raw = raw
		n := int64(len(raw))
		sd.StreamLength = &n
		sd.Dict["Length"] = types.Integer(n)
		sd.Dict["Filter"] = types.Array{types.Name(filter.ASCII85), types.Name(sd.FilterPipeline[0].Name)}
		sd.FilterPipeline = append([]types.PDFFilter{{Name: filter.ASCII85}}, sd.FilterPipeline...)
		e.Object = sd
		fatte++
	}
	if fatte == 0 {
		t.Fatal("nessuna immagine")
	}
	out, err := scrivi(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// ZP06-2: Comprimi riduce anche le foto con ASCII85 + Flate o ASCII85 + DCT.
func TestZP06ComprimiASCII85(t *testing.T) {
	flate := conASCII85(t, pdfConFoto(t))
	forte, err := Comprimi(flate, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(forte) >= len(flate)/2 {
		t.Errorf("ASCII85 + Flate: %d -> %d", len(flate), len(forte))
	}
	dct := conASCII85(t, forte) // la foto ora è JPEG, con ASCII85 davanti
	leggera, err := Comprimi(dct, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(leggera) > len(dct)*85/100 {
		t.Errorf("ASCII85 + DCT, leggera: %d -> %d", len(dct), len(leggera))
	}
	if bytes.Contains(leggera, []byte("ASCII85Decode")) {
		t.Error("resta ASCII85")
	}
}

// ZP06-3: la firma «su tutte le pagine» resta grande uguale e non si deforma su una pagina
// orizzontale o più piccola.
func TestZP06FirmaTutteLePagine(t *testing.T) {
	b := pdfProva([]pagProva{a4("uno"), a4o("due"), {419.53, 595.28, 0, "A5"}})
	png := pngProva(t, 300, 82, false) // proporzione 0,273
	l := 170 / 595.28
	a := l * (82.0 / 300) * (595.28 / 841.89)
	out, err := Timbra(b, "", []Timbro{{Pagina: 0, PNG: png, X: .5, Y: .5, L: l, A: a, LPt: 170}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, _, err := leggi(out, "")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`q ([0-9.]+) 0 0 ([0-9.]+) [0-9.-]+ [0-9.-]+ cm /TassoPDF`)
	for p := 1; p <= 3; p++ {
		d, _, _, _ := ctx.PageDict(bg, p, false)
		c, err := ctx.PageContent(d, p)
		if err != nil {
			t.Fatal(err)
		}
		m := re.FindStringSubmatch(string(c))
		if m == nil {
			t.Fatalf("pagina %d: firma non trovata", p)
		}
		w, _ := strconv.ParseFloat(m[1], 64)
		h, _ := strconv.ParseFloat(m[2], 64)
		if math.Abs(w-170) > 0.5 || math.Abs(h/w-82.0/300) > 0.005 {
			t.Errorf("pagina %d: %.1f x %.1f (serve 170 di larghezza, proporzione %.3f)", p, w, h, 82.0/300)
		}
	}
}

// ZP06-7: le immagini esportate dicono la loro risoluzione (PNG pHYs, JPEG JFIF).
func TestZP06DPI(t *testing.T) {
	p := conDPI(pngProva(t, 20, 10, false), 150)
	cfg, err := png.DecodeConfig(bytes.NewReader(p))
	if err != nil || cfg.Width != 20 {
		t.Fatalf("PNG rovinato: %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(p)); err != nil {
		t.Fatalf("PNG rovinato: %v", err)
	}
	i := bytes.Index(p, []byte("pHYs"))
	if i < 0 || binary.BigEndian.Uint32(p[i+4:]) != 5906 || p[i+12] != 1 {
		t.Errorf("pHYs mancante o sbagliato")
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, image.NewGray(image.Rect(0, 0, 8, 8)), nil) // senza JFIF
	j := conDPI(buf.Bytes(), 300)
	if _, err := jpeg.Decode(bytes.NewReader(j)); err != nil {
		t.Fatalf("JPEG rovinato: %v", err)
	}
	if string(j[6:11]) != "JFIF\x00" || j[13] != 1 || binary.BigEndian.Uint16(j[14:]) != 300 {
		t.Errorf("JFIF: % x", j[:20])
	}
	j2 := conDPI(j, 150) // con JFIF già presente: cambia solo la densità
	if len(j2) != len(j) || binary.BigEndian.Uint16(j2[16:]) != 150 {
		t.Error("JFIF esistente")
	}
}
