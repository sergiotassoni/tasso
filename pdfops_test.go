// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ---------------------------------------------------------------- file di prova

type pagProva struct {
	w, h  float64
	rot   int
	testo string
}

// pdfProva: un PDF semplice scritto a mano (catalogo, pagine, testo in Helvetica).
func pdfProva(pagine []pagProva) []byte {
	var b bytes.Buffer
	var off []int
	obj := func(s string) {
		off = append(off, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", len(off), s)
	}
	b.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	n := len(pagine)
	// 1 catalogo, 2 pagine, 3 font, poi per ogni pagina: pagina e contenuto
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	kids := ""
	for i := 0; i < n; i++ {
		kids += fmt.Sprintf("%d 0 R ", 4+2*i)
	}
	obj(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids, n))
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	for i, p := range pagine {
		rot := ""
		if p.rot != 0 {
			rot = fmt.Sprintf(" /Rotate %d", p.rot)
		}
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %g %g]%s /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", p.w, p.h, rot, 5+2*i))
		cs := fmt.Sprintf("BT /F1 24 Tf 72 %g Td (%s) Tj ET\n0 0 1 rg 72 72 100 50 re f\n", p.h-100, p.testo)
		// un po' di riempimento, perché il file abbia una lunghezza realistica
		cs += strings.Repeat("% riga di riempimento per avere un file un po' piu' lungo\n", 40)
		obj(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(cs), cs))
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f\r\n", len(off)+1)
	for _, o := range off {
		fmt.Fprintf(&b, "%010d 00000 n\r\n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R /ID [<0123456789abcdef0123456789abcdef> <0123456789abcdef0123456789abcdef>] >>\nstartxref\n%d\n%%%%EOF\n", len(off)+1, xref)
	return b.Bytes()
}

func a4(testo string) pagProva  { return pagProva{595.28, 841.89, 0, testo} }
func a4o(testo string) pagProva { return pagProva{841.89, 595.28, 0, testo} }

const pwUtente = "Segreta-123"
const pwProprietario = "Proprietario-456"

// cifra: protegge un PDF con AES-256 (pdfcpu).
func cifra(t *testing.T, data []byte, upw, opw string, perm model.PermissionFlags) []byte {
	t.Helper()
	conf := model.NewAESConfiguration(upw, opw, 256)
	conf.Offline = true
	conf.Permissions = perm
	var out bytes.Buffer
	if err := api.Encrypt(context.Background(), bytes.NewReader(data), &out, conf); err != nil {
		t.Fatalf("cifratura: %v", err)
	}
	return out.Bytes()
}

func leggiCtxProva(t *testing.T, data []byte) *model.Context {
	t.Helper()
	ctx, err := api.ReadContext(context.Background(), bytes.NewReader(data), confBase(""))
	if err != nil {
		t.Fatalf("il PDF prodotto non si legge: %v", err)
	}
	if err := api.ValidateContext(context.Background(), ctx); err != nil {
		t.Fatalf("il PDF prodotto non è valido: %v", err)
	}
	return ctx
}

func pagine(t *testing.T, data []byte) int {
	t.Helper()
	return leggiCtxProva(t, data).PageCount
}

func rotazionePagina(t *testing.T, ctx *model.Context, p int) int {
	t.Helper()
	_, _, attrs, err := ctx.PageDict(context.Background(), p, false)
	if err != nil {
		t.Fatal(err)
	}
	return ((attrs.Rotate % 360) + 360) % 360
}

// salvaProva: se TASSOPDF_PROVE è impostata, i file di prova finiscono lì (per guardarli a mano).
func salvaProva(t *testing.T, nome string, b []byte) {
	if d := os.Getenv("TASSOPDF_PROVE"); d != "" {
		_ = os.MkdirAll(d, 0o755)
		_ = os.WriteFile(filepath.Join(d, nome), b, 0o644)
	}
}

// ---------------------------------------------------------------- Sblocca

func TestSbloccaPasswordAES256(t *testing.T) {
	orig := pdfProva([]pagProva{a4("Contratto pagina 1"), a4("Contratto pagina 2")})
	prot := cifra(t, orig, pwUtente, pwProprietario, model.PermissionsNone)
	salvaProva(t, "protetto-aes256.pdf", prot)

	inf, err := Info(prot, "")
	if err != nil {
		t.Fatal(err)
	}
	if !inf.ServePassword || !inf.Cifrato {
		t.Fatalf("doveva chiedere la password: %+v", inf)
	}
	if _, err := Sblocca(prot, "", true); CodiceErrore(err) != "password" {
		t.Fatalf("senza password: atteso codice password, avuto %v", err)
	}
	if _, err := Sblocca(prot, "sbagliata", true); CodiceErrore(err) != "password" {
		t.Fatalf("password sbagliata: atteso codice password, avuto %v", err)
	}
	for _, pw := range []string{pwUtente, pwProprietario} {
		lib, err := Sblocca(prot, pw, true)
		if err != nil {
			t.Fatalf("con la password giusta (%s): %v", pw, err)
		}
		ctx := leggiCtxProva(t, lib)
		if ctx.Encrypt != nil {
			t.Fatal("la copia è ancora protetta")
		}
		if ctx.PageCount != 2 {
			t.Fatalf("pagine: %d", ctx.PageCount)
		}
		inf, err := Info(lib, "")
		if err != nil || inf.Cifrato || inf.Bloccato {
			t.Fatalf("info copia libera: %+v %v", inf, err)
		}
		salvaProva(t, "sbloccato.pdf", lib)
	}
}

func TestSbloccaSoloBlocchi(t *testing.T) {
	orig := pdfProva([]pagProva{a4("Solo blocchi")})
	prot := cifra(t, orig, "", pwProprietario, model.PermissionsNone)
	inf, err := Info(prot, "")
	if err != nil {
		t.Fatal(err)
	}
	if inf.ServePassword || !inf.Bloccato {
		t.Fatalf("doveva aprirsi senza password ma con i blocchi: %+v", inf)
	}
	lib, err := Sblocca(prot, "", true)
	if err != nil {
		t.Fatal(err)
	}
	ctx := leggiCtxProva(t, lib)
	if ctx.Encrypt != nil || ctx.PageCount != 1 {
		t.Fatalf("copia non libera: enc=%v pagine=%d", ctx.Encrypt, ctx.PageCount)
	}
}

func TestSbloccaNonProtetto(t *testing.T) {
	if _, err := Sblocca(pdfProva([]pagProva{a4("libero")}), "", true); CodiceErrore(err) != "non_protetto" {
		t.Fatalf("atteso non_protetto, avuto %v", err)
	}
}

func TestNonPDF(t *testing.T) {
	if _, err := Info([]byte("ciao, non sono un PDF"), ""); CodiceErrore(err) != "non_pdf" {
		t.Fatalf("atteso non_pdf, avuto %v", err)
	}
}

// ---------------------------------------------------------------- Riparazione

func TestRiparaTroncato(t *testing.T) {
	var pp []pagProva
	for i := 1; i <= 6; i++ {
		pp = append(pp, a4(fmt.Sprintf("Pagina %d", i)))
	}
	orig := pdfProva(pp)
	for _, frac := range []float64{0.97, 0.8, 0.6} {
		tronc := orig[:int(float64(len(orig))*frac)]
		salvaProva(t, fmt.Sprintf("troncato-%02.0f.pdf", frac*100), tronc)
		rip, err := Ripara(tronc, "")
		if err != nil {
			t.Fatalf("troncato al %.0f%%: %v", frac*100, err)
		}
		n := pagine(t, rip)
		if n < 1 {
			t.Fatalf("troncato al %.0f%%: nessuna pagina", frac*100)
		}
		t.Logf("troncato al %.0f%%: riparato con %d pagine", frac*100, n)
	}
}

func TestRiparaIndiceSbagliato(t *testing.T) {
	orig := pdfProva([]pagProva{a4("Uno"), a4("Due")})
	// sposto tutti i numeri dell'indice: pdfcpu (o la ricostruzione) deve cavarsela lo stesso
	s := string(orig)
	i := strings.Index(s, "xref\n")
	rotto := s[:i] + strings.ReplaceAll(s[i:], " 00000 n", "9 00000 n")
	rip, err := Ripara([]byte(rotto), "")
	if err != nil {
		t.Fatal(err)
	}
	if n := pagine(t, rip); n != 2 {
		t.Fatalf("pagine: %d", n)
	}
}

func TestRicostruisci(t *testing.T) {
	orig := pdfProva([]pagProva{a4("Uno")})
	senzaIndice := orig[:bytes.Index(orig, []byte("xref\n"))]
	ric, err := ricostruisci(senzaIndice, false)
	if err != nil {
		t.Fatal(err)
	}
	if n := pagine(t, ric); n != 1 {
		t.Fatalf("pagine: %d", n)
	}
}

// ---------------------------------------------------------------- Unisci

func jpegProva(t *testing.T, w, h, orientamento int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 255 / w), uint8(y * 255 / h), 120, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	j := buf.Bytes()
	if orientamento == 0 {
		return j
	}
	// segmento APP1 con un EXIF minimo (TIFF little endian, un solo tag Orientation)
	var tiff bytes.Buffer
	tiff.WriteString("II")
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(42))
	_ = binary.Write(&tiff, binary.LittleEndian, uint32(8))
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(1))
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(0x0112))
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(3))
	_ = binary.Write(&tiff, binary.LittleEndian, uint32(1))
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(orientamento))
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(0))
	_ = binary.Write(&tiff, binary.LittleEndian, uint32(0))
	seg := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	app1 := []byte{0xFF, 0xE1, byte((len(seg) + 2) >> 8), byte(len(seg) + 2)}
	out := append([]byte{}, j[:2]...)
	out = append(out, app1...)
	out = append(out, seg...)
	out = append(out, j[2:]...)
	return out
}

func pngProva(t *testing.T, w, h int, trasparente bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := uint8(255)
			if trasparente && (x+y)%3 != 0 {
				a = 0
			}
			img.Set(x, y, color.NRGBA{20, 40, 160, a})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestOrientamentoEXIF(t *testing.T) {
	for _, o := range []int{1, 3, 6, 8} {
		if got := orientamentoEXIF(jpegProva(t, 40, 30, o)); got != o {
			t.Fatalf("orientamento %d letto come %d", o, got)
		}
	}
	if got := orientamentoEXIF(jpegProva(t, 40, 30, 0)); got != 1 {
		t.Fatalf("senza EXIF: %d", got)
	}
	if TipoFile(jpegProva(t, 10, 10, 0)) != "jpg" || TipoFile(pngProva(t, 10, 10, false)) != "png" {
		t.Fatal("TipoFile")
	}
	if TipoFile([]byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00")) != "heic" {
		t.Fatal("TipoFile heic")
	}
}

func TestUnisci(t *testing.T) {
	pdfA := pdfProva([]pagProva{a4("A1"), a4("A2")})
	pdfB := pdfProva([]pagProva{a4o("B orizzontale")})
	prot := cifra(t, pdfProva([]pagProva{a4("Protetto")}), pwUtente, pwProprietario, model.PermissionsNone)
	foto := jpegProva(t, 400, 300, 6) // foto del telefono girata: EXIF 6 = da girare di 90°
	pngF := pngProva(t, 300, 400, false)

	pezzi := []Pezzo{
		{Nome: "a.pdf", Dati: pdfA},
		{Nome: "foto.jpg", Dati: foto, Foto: true},
		{Nome: "b.pdf", Dati: pdfB, Rotazione: 90},
		{Nome: "scan.png", Dati: pngF, Foto: true, Rotazione: 180},
		{Nome: "protetto.pdf", Dati: prot, Password: pwUtente},
	}
	out, err := Unisci(pezzi)
	if err != nil {
		t.Fatal(err)
	}
	salvaProva(t, "unito.pdf", out)
	ctx := leggiCtxProva(t, out)
	if ctx.PageCount != 6 {
		t.Fatalf("pagine: %d (attese 6)", ctx.PageCount)
	}
	if ctx.Encrypt != nil {
		t.Fatal("il risultato non deve essere protetto")
	}
	attese := map[int]int{1: 0, 2: 0, 3: 90, 4: 90, 5: 180, 6: 0}
	for p, r := range attese {
		if got := rotazionePagina(t, ctx, p); got != r {
			t.Errorf("pagina %d: rotazione %d, attesa %d", p, got, r)
		}
	}

	// senza password il file protetto deve dare un errore sul pezzo giusto
	pezzi[4].Password = ""
	_, err = Unisci(pezzi)
	var ep *ErrPezzo
	if !asErrPezzo(err, &ep) || ep.Indice != 4 || CodiceErrore(err) != "password" {
		t.Fatalf("atteso errore password sul pezzo 5, avuto %v", err)
	}
}

func asErrPezzo(err error, ep **ErrPezzo) bool {
	for e := err; e != nil; {
		if x, ok := e.(*ErrPezzo); ok {
			*ep = x
			return true
		}
		u, ok := e.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		e = u.Unwrap()
	}
	return false
}

func TestFotoNonValida(t *testing.T) {
	if _, err := FotoInPDF([]byte("non sono una foto")); CodiceErrore(err) != "formato" {
		t.Fatalf("atteso formato, avuto %v", err)
	}
	if _, err := FotoInPDF([]byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00")); CodiceErrore(err) != "heic" {
		t.Fatalf("atteso heic, avuto %v", err)
	}
}

// ---------------------------------------------------------------- Firma

func vicino(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestMatriceTimbro(t *testing.T) {
	vb := types.NewRectangle(0, 0, 600, 800)
	tb := Timbro{X: 0.1, Y: 0.2, L: 0.25, A: 0.05}
	// pagina dritta: x=60, larghezza 150, altezza 40, y dal basso = 800-160-40 = 600
	m := matriceTimbro(vb, 0, tb)
	if !vicino(m[0], 150) || !vicino(m[3], 40) || !vicino(m[4], 60) || !vicino(m[5], 600) || m[1] != 0 || m[2] != 0 {
		t.Fatalf("rot 0: %v", m)
	}
	// punto in alto a sinistra dell'immagine (0,1) per ogni rotazione: deve stare nel punto
	// giusto della pagina vista girata.
	applica := func(m mat, x, y float64) (float64, float64) {
		return m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]
	}
	tl := Timbro{X: 0, Y: 0, L: 0.1, A: 0.1}
	casi := map[int][2]float64{
		0:   {0, 800},   // in alto a sinistra della pagina dritta
		90:  {0, 0},     // girata a destra: l'angolo in alto a sinistra visto è in basso a sinistra
		180: {600, 0},   // capovolta
		270: {600, 800}, // girata a sinistra
	}
	for rot, atteso := range casi {
		x, y := applica(matriceTimbro(vb, rot, tl), 0, 1)
		if !vicino(x, atteso[0]) || !vicino(y, atteso[1]) {
			t.Errorf("rot %d: angolo (%g,%g), atteso %v", rot, x, y, atteso)
		}
	}
	// CropBox spostata: si somma l'origine
	vb2 := types.NewRectangle(10, 20, 610, 820)
	m2 := matriceTimbro(vb2, 0, tb)
	if !vicino(m2[4], 70) || !vicino(m2[5], 620) {
		t.Fatalf("cropbox: %v", m2)
	}
}

func contenutoPagina(t *testing.T, ctx *model.Context, p int) string {
	t.Helper()
	bb, err := ctx.PageContent(mustPageDict(t, ctx, p), p)
	if err != nil {
		t.Fatal(err)
	}
	return string(bb)
}

func mustPageDict(t *testing.T, ctx *model.Context, p int) types.Dict {
	d, _, _, err := ctx.PageDict(context.Background(), p, false)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestTimbra(t *testing.T) {
	orig := pdfProva([]pagProva{a4("Contratto"), a4o("Allegato"), {595.28, 841.89, 90, "Girata"}})
	firma := pngProva(t, 300, 100, true)
	spunta := pngProva(t, 50, 50, true)
	out, err := Timbra(orig, "", []Timbro{
		{Pagina: 1, PNG: firma, X: 0.55, Y: 0.85, L: 0.35, A: 0.08},
		{Pagina: 0, PNG: spunta, X: 0.05, Y: 0.05, L: 0.04, A: 0.03}, // su tutte le pagine
	})
	if err != nil {
		t.Fatal(err)
	}
	salvaProva(t, "firmato.pdf", out)
	ctx := leggiCtxProva(t, out)
	if ctx.PageCount != 3 {
		t.Fatalf("pagine %d", ctx.PageCount)
	}
	c1 := contenutoPagina(t, ctx, 1)
	// firma su pagina A4: larghezza 0.35*595.28 = 208.348, x = 327.404
	if !strings.Contains(c1, "208.3480 0 0 67.3512 327.4040") {
		t.Fatalf("matrice della firma non trovata in pagina 1:\n%s", c1[len(c1)-300:])
	}
	if strings.Count(c1, " Do Q") != 2 {
		t.Fatalf("pagina 1: attese 2 immagini")
	}
	for p := 2; p <= 3; p++ {
		if strings.Count(contenutoPagina(t, ctx, p), " Do Q") != 1 {
			t.Fatalf("pagina %d: attesa 1 immagine (la spunta)", p)
		}
	}
	// il testo originale deve esserci ancora
	if !strings.Contains(c1, "(Contratto) Tj") {
		t.Fatal("contenuto originale perso")
	}
	// l'immagine della firma deve avere la trasparenza (SMask)
	if !strings.Contains(string(out), "/SMask") {
		t.Log("attenzione: nessuna SMask trovata nel file (potrebbe essere compressa in un object stream)")
	}
}

func TestTimbraProtetto(t *testing.T) {
	prot := cifra(t, pdfProva([]pagProva{a4("Protetto")}), pwUtente, pwProprietario, model.PermissionsNone)
	firma := pngProva(t, 30, 10, true)
	if _, err := Timbra(prot, "", []Timbro{{Pagina: 1, PNG: firma, X: .1, Y: .1, L: .2, A: .1}}); CodiceErrore(err) != "password" {
		t.Fatalf("atteso password, avuto %v", err)
	}
	out, err := Timbra(prot, pwUtente, []Timbro{{Pagina: 1, PNG: firma, X: .1, Y: .1, L: .2, A: .1}})
	if err != nil {
		t.Fatal(err)
	}
	if ctx := leggiCtxProva(t, out); ctx.Encrypt != nil {
		t.Fatal("ancora protetto")
	}
}

func TestTimbraTroncato(t *testing.T) {
	orig := pdfProva([]pagProva{a4("Uno"), a4("Due"), a4("Tre")})
	tronc := orig[:len(orig)-200]
	out, err := Timbra(tronc, "", []Timbro{{Pagina: 1, PNG: pngProva(t, 30, 10, true), X: .1, Y: .1, L: .2, A: .1}})
	if err != nil {
		t.Fatal(err)
	}
	if pagine(t, out) < 1 {
		t.Fatal("nessuna pagina")
	}
}
