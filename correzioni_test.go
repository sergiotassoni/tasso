// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

// Test delle correzioni del collaudo finale (trovati-09 e trovati-09b).

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// pdf20: lo stesso PDF di prova con l'intestazione %PDF-2.0 (stessa lunghezza: l'indice resta giusto).
func pdf20(testo string) []byte {
	b := pdfProva([]pagProva{a4(testo)})
	return bytes.Replace(b, []byte("%PDF-1.7"), []byte("%PDF-2.0"), 1)
}

func TestUnisciEOrganizzaConPDF20(t *testing.T) {
	v17 := pdfProva([]pagProva{a4("uno"), a4("due")})
	v20 := pdf20("tre")
	if c := leggiCtxProva(t, v20); c.XRefTable.Version() != model.V20 {
		t.Fatalf("il file di prova non è 2.0: %v", c.XRefTable.Version())
	}
	out, err := Unisci([]Pezzo{{Nome: "a.pdf", Dati: v17}, {Nome: "b.pdf", Dati: v20}})
	if err != nil {
		t.Fatalf("unione 1.7 + 2.0: %v", err)
	}
	if n := pagine(t, out); n != 3 {
		t.Fatalf("pagine: %d", n)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-2.0")) {
		t.Fatalf("il risultato deve essere 2.0: %q", out[:8])
	}
	org, err := Organizza([][]byte{v17, v20}, []PaginaSeq{{Fonte: 0, Pagina: 1}, {Fonte: 1, Pagina: 1}})
	if err != nil {
		t.Fatalf("inserisci pagine da un PDF 2.0: %v", err)
	}
	if n := pagine(t, org); n != 2 {
		t.Fatalf("pagine organizza: %d", n)
	}
}

func TestTroncatoERovinatoNonNonPDF(t *testing.T) {
	b := pdfProva([]pagProva{a4("uno"), a4("due"), a4("tre")})
	for _, frazione := range []int{10, 20} {
		_, err := Info(b[:len(b)*frazione/100], "")
		if c := CodiceErrore(err); c == "non_pdf" {
			t.Fatalf("troncato al %d%%: un PDF rovinato non deve dire \"non è un PDF\" (%v)", frazione, err)
		}
	}
	if CodiceErrore(classifica(errorsNew("root version: missing root dict"))) != "rovinato" {
		t.Fatal("\"root version\" deve dare rovinato")
	}
	if CodiceErrore(classifica(errorsNew("no header version available"))) != "non_pdf" {
		t.Fatal("intestazione mancante: non_pdf")
	}
}

type erroreSemplice string

func (e erroreSemplice) Error() string { return string(e) }
func errorsNew(s string) error         { return erroreSemplice(s) }

// pdfXRefStream: un PDF "moderno" scritto a mano: catalogo ed elenco delle pagine dentro un
// object stream, indice compresso /Type /XRef (come Word, Chrome e gli scanner recenti).
func pdfXRefStream(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	off3 := b.Len()
	b.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] >>\nendobj\n")
	o1 := "<< /Type /Catalog /Pages 2 0 R >>"
	o2 := "<< /Type /Pages /Kids [3 0 R 6 0 R] /Count 2 >>"
	testa := fmt.Sprintf("1 0 2 %d ", len(o1)+1)
	var z bytes.Buffer // compresso, come nei file veri: il catalogo non si vede "in chiaro"
	zw := zlib.NewWriter(&z)
	zw.Write([]byte(testa + o1 + " " + o2))
	zw.Close()
	off4 := b.Len()
	fmt.Fprintf(&b, "4 0 obj\n<< /Type /ObjStm /N 2 /First %d /Filter /FlateDecode /Length %d >>\nstream\n", len(testa), z.Len())
	b.Write(z.Bytes())
	b.WriteString("\nendstream\nendobj\n")
	off6 := b.Len()
	b.WriteString("6 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] >>\nendobj\n")
	off5 := b.Len()
	riga := func(tipo byte, a uint32, c uint16) []byte {
		r := []byte{tipo, 0, 0, 0, 0, 0, 0}
		binary.BigEndian.PutUint32(r[1:5], a)
		binary.BigEndian.PutUint16(r[5:7], c)
		return r
	}
	dati := bytes.Join([][]byte{riga(0, 0, 65535), riga(2, 4, 0), riga(2, 4, 1), riga(1, uint32(off3), 0),
		riga(1, uint32(off4), 0), riga(1, uint32(off5), 0), riga(1, uint32(off6), 0)}, nil)
	fmt.Fprintf(&b, "5 0 obj\n<< /Type /XRef /Size 7 /W [1 4 2] /Root 1 0 R /Length %d >>\nstream\n", len(dati))
	b.Write(dati)
	b.WriteString("\nendstream\nendobj\n")
	fmt.Fprintf(&b, "startxref\n%d\n%%%%EOF\n", off5)
	out := b.Bytes()
	if n := pagine(t, out); n != 2 {
		t.Fatalf("il file di prova non si legge: %d pagine", n)
	}
	if !reXRef.Match(out) || reCatalog.Match(out) {
		t.Fatal("file di prova sbagliato")
	}
	return out
}

func TestRiparaPDFConXRefStream(t *testing.T) {
	for _, nome := range []string{"a mano", "qpdf"} {
		var b []byte
		if nome == "qpdf" {
			b = mustRead(t, filepath.Join("testdata", "xref-stream.pdf"))
		} else {
			b = pdfXRefStream(t)
		}
		n0 := pagine(t, b)
		// startxref sbagliato (il guasto tipico di un file salvato male)
		i := bytes.LastIndex(b, []byte("startxref"))
		rotto := append(append([]byte{}, b[:i]...), []byte("startxref\n123\n%%EOF\n")...)
		inf, err := Info(rotto, "")
		if err != nil {
			t.Fatalf("%s: info: %v", nome, err)
		}
		if inf.Pagine != n0 || !inf.Riparato {
			t.Fatalf("%s: doveva essere letto con la riparazione: %+v", nome, inf)
		}
		out, err := Ripara(rotto, "")
		if err != nil {
			t.Fatalf("%s: ripara: %v", nome, err)
		}
		if n := pagine(t, out); n != n0 {
			t.Fatalf("%s: pagine riparate: %d", nome, n)
		}
	}
}

func TestTimbraPaginaOltreLaFine(t *testing.T) {
	b := pdfProva([]pagProva{a4("uno"), a4("due")})
	png := pngProva(t, 20, 10, true)
	if _, err := Timbra(b, "", []Timbro{{Pagina: 3, PNG: png, X: .1, Y: .1, L: .2, A: .1}}); CodiceErrore(err) != "pagina_fuori" {
		t.Fatalf("pagina 3 di 2: atteso pagina_fuori, avuto %v", err)
	}
	if _, err := Timbra(b, "", []Timbro{{Pagina: 2, PNG: png, X: .1, Y: .1, L: .2, A: .1}}); err != nil {
		t.Fatalf("pagina 2 di 2: %v", err)
	}
}

// pdfAMano: un PDF scritto oggetto per oggetto (oggetto 1 = catalogo), con l'indice giusto.
func pdfAMano(oggetti ...string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	var off []int
	for i, o := range oggetti {
		off = append(off, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	x := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f\r\n", len(oggetti)+1)
	for _, o := range off {
		fmt.Fprintf(&b, "%010d 00000 n\r\n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(oggetti)+1, x)
	return b.Bytes()
}

// conFirmaDigitale: un PDF con un modulo che ha un campo firma (/FT /Sig con /V), come un PAdES.
func conFirmaDigitale(conSigFlags bool) []byte {
	sf := ""
	if conSigFlags {
		sf = " /SigFlags 3"
	}
	return pdfAMano(
		"<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R]"+sf+" >> >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] >>",
		"<< /T (Firme) /Kids [5 0 R] >>",
		"<< /FT /Sig /T (Firma1) /Parent 4 0 R /V 6 0 R >>",
		"<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /ETSI.CAdES.detached /Contents <00> /ByteRange [0 0 0 0] >>",
	)
}

func TestRiconosceFirmaDigitale(t *testing.T) {
	b := pdfProva([]pagProva{a4("contratto")})
	if inf, err := Info(b, ""); err != nil || inf.FirmatoDigitalmente {
		t.Fatalf("PDF senza firma: %+v %v", inf, err)
	}
	for _, flag := range []bool{true, false} {
		f := conFirmaDigitale(flag)
		inf, err := Info(f, "")
		if err != nil || !inf.FirmatoDigitalmente {
			t.Fatalf("PDF con firma digitale (SigFlags=%v): %+v %v", flag, inf, err)
		}
	}
}

func TestSbloccaDueModi(t *testing.T) {
	orig := pdfProva([]pagProva{a4("pagina")})
	soloBlocchi := cifra(t, orig, "", pwProprietario, model.PermissionsNone)
	conPw := cifra(t, orig, pwUtente, pwProprietario, model.PermissionsNone)
	pwSenzaBlocchi := cifra(t, orig, pwUtente, pwProprietario, model.PermissionsAll)

	// B (quello attivo): senza la spunta non si fa niente.
	if modoSblocco != SbloccoConConferma {
		t.Fatalf("il modo predefinito deve essere B (spunta), è %q", modoSblocco)
	}
	if _, err := Sblocca(soloBlocchi, "", false); CodiceErrore(err) != "conferma_diritti" {
		t.Fatalf("B senza spunta: %v", err)
	}
	if _, err := Sblocca(soloBlocchi, "", true); err != nil {
		t.Fatalf("B con spunta: %v", err)
	}
	if _, err := Sblocca(conPw, "", true); CodiceErrore(err) != "password" {
		t.Fatalf("B con password di apertura mancante: %v", err)
	}

	// A: i blocchi si tolgono solo con la password del proprietario.
	casi := []struct {
		nome   string
		dati   []byte
		pw     string
		codice string
	}{
		{"solo blocchi, niente password", soloBlocchi, "", "password_proprietario"},
		{"solo blocchi, password sbagliata", soloBlocchi, "boh", "password"},
		{"solo blocchi, password del proprietario", soloBlocchi, pwProprietario, ""},
		{"password e blocchi, password di apertura", conPw, pwUtente, "password_proprietario"},
		{"password e blocchi, password del proprietario", conPw, pwProprietario, ""},
		{"password senza blocchi, password di apertura", pwSenzaBlocchi, pwUtente, ""},
	}
	for _, c := range casi {
		out, err := sbloccaCon(SbloccoConProprietario, c.dati, c.pw, false)
		if CodiceErrore(err) != c.codice {
			t.Errorf("A, %s: atteso %q, avuto %v", c.nome, c.codice, err)
			continue
		}
		if c.codice == "" {
			if ctx := leggiCtxProva(t, out); ctx.Encrypt != nil {
				t.Errorf("A, %s: la copia è ancora protetta", c.nome)
			}
		}
	}
}

func TestMarchioDelWebSulleCopie(t *testing.T) {
	dir := t.TempDir()
	scaricato := filepath.Join(dir, "Scaricato.pdf")
	locale := filepath.Join(dir, "Locale.pdf")
	_ = os.WriteFile(scaricato, pdfProva([]pagProva{a4("dal web")}), 0o644)
	_ = os.WriteFile(locale, pdfProva([]pagProva{a4("mio")}), 0o644)

	// Finto NTFS: le zone stanno in una mappa.
	zone := map[string][]byte{scaricato: []byte("[ZoneTransfer]\r\nZoneId=3\r\nHostUrl=https://esempio.it/\r\n")}
	scritte := map[string][]byte{}
	vl, vs, vf := piattaforma.leggiZona, piattaforma.scriviZona, piattaforma.salvaFile
	defer func() { piattaforma.leggiZona, piattaforma.scriviZona, piattaforma.salvaFile = vl, vs, vf }()
	piattaforma.leggiZona = func(p string) []byte { return zone[p] }
	piattaforma.scriviZona = func(p string, z []byte) error { scritte[p] = z; return nil }

	vWeb, err := apriPercorso(scaricato)
	if err != nil {
		t.Fatal(err)
	}
	vLoc, err := apriPercorso(locale)
	if err != nil {
		t.Fatal(err)
	}
	firma := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngProva(t, 30, 10, true))
	rf, err := goFirma(vWeb.ID, "", []TimbroPagina{{Timbro: Timbro{Pagina: 1, X: .1, Y: .1, L: .2, A: .05}, PNG64: firma}})
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "Firmato.pdf")
	piattaforma.salvaFile = func(nome, cartella, titolo, ext string) (string, error) { return dest, nil }
	if _, err := goSalva(rf.ID, "x.pdf", "Salva"); err != nil {
		t.Fatal(err)
	}
	if idZona(scritte[dest]) != 3 {
		t.Fatalf("la copia firmata di un file scaricato deve avere ZoneId=3: %q", scritte[dest])
	}
	// copia di un file locale: nessun marchio
	rl, err := goRipara(vLoc.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	dest2 := filepath.Join(dir, "Riparato.pdf")
	piattaforma.salvaFile = func(nome, cartella, titolo, ext string) (string, error) { return dest2, nil }
	if _, err := goSalva(rl.ID, "x.pdf", "Salva"); err != nil {
		t.Fatal(err)
	}
	if _, ok := scritte[dest2]; ok {
		t.Fatal("una copia di un file locale non deve avere il marchio del web")
	}
	// unione locale + scaricato: il marchio più severo
	ru, err := goUnisci([]PezzoPagina{{ID: vLoc.ID}, {ID: vWeb.ID}})
	if err != nil {
		t.Fatal(err)
	}
	dest3 := filepath.Join(dir, "Unito.pdf")
	piattaforma.salvaFile = func(nome, cartella, titolo, ext string) (string, error) { return dest3, nil }
	if _, err := goSalva(ru.ID, "x.pdf", "Salva"); err != nil {
		t.Fatal(err)
	}
	if idZona(scritte[dest3]) != 3 {
		t.Fatalf("unione con un file scaricato: marchio mancante (%q)", scritte[dest3])
	}
	if idZona([]byte("[ZoneTransfer]\nZoneId=4\n")) != 4 || idZona(nil) != 0 {
		t.Fatal("idZona")
	}
	if z := zonaPiuSevera(nil, []byte("ZoneId=3"), []byte("ZoneId=4"), nil); idZona(z) != 4 {
		t.Fatal("zonaPiuSevera")
	}
}

func TestNomeConAltraEstensioneNonSostituisceInSilenzio(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "Doc.pdf")
	_ = os.WriteFile(src, pdfProva([]pagProva{a4("doc")}), 0o644)
	esistente := filepath.Join(dir, "Nota.txt.pdf")
	_ = os.WriteFile(esistente, []byte("NON TOCCARMI"), 0o644)
	vf := piattaforma.salvaFile
	defer func() { piattaforma.salvaFile = vf }()
	// l'utente scrive "Nota.txt": Windows ha chiesto conferma solo per "Nota.txt"
	piattaforma.salvaFile = func(nome, cartella, titolo, ext string) (string, error) { return filepath.Join(dir, "Nota.txt"), nil }
	v, err := apriPercorso(src)
	if err != nil {
		t.Fatal(err)
	}
	r, err := goRipara(v.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := goSalva(r.ID, "x.pdf", "Salva")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(esistente); string(b) != "NON TOCCARMI" {
		t.Fatal("il file esistente è stato sostituito senza conferma")
	}
	if filepath.Base(p) != "Nota.txt (2).pdf" {
		t.Fatalf("nome scelto: %q", p)
	}
}

func TestScriviNuovoNonSovrascrive(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pagina - 1.png")
	if err := scriviNuovo(p, []byte("primo")); err != nil {
		t.Fatal(err)
	}
	if err := scriviNuovo(p, []byte("secondo")); err == nil || !os.IsExist(err) {
		t.Fatalf("doveva dire che il file esiste: %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "primo" {
		t.Fatal("sovrascritto")
	}
}

// heifProva: un contenitore HEIF minimo con l'immagine principale (id 1) e le proprietà date.
func heifProva(proprieta ...[]byte) []byte {
	box := func(tipo string, corpo ...[]byte) []byte {
		c := bytes.Join(corpo, nil)
		b := make([]byte, 8, 8+len(c))
		binary.BigEndian.PutUint32(b, uint32(8+len(c)))
		copy(b[4:], tipo)
		return append(b, c...)
	}
	ftyp := box("ftyp", []byte("heic\x00\x00\x00\x00mif1heic"))
	pitm := box("pitm", []byte{0, 0, 0, 0, 0, 1})
	ispe := box("ispe", []byte{0, 0, 0, 0, 0, 0, 0, 4, 0, 0, 0, 2})
	ipco := box("ipco", append([][]byte{ispe}, proprieta...)...)
	assoc := []byte{0, 1, byte(1 + len(proprieta)), 0x01}
	for i := range proprieta {
		assoc = append(assoc, byte(0x80|(i+2)))
	}
	ipma := box("ipma", []byte{0, 0, 0, 0, 0, 0, 0, 1}, assoc)
	iprp := box("iprp", ipco, ipma)
	meta := box("meta", []byte{0, 0, 0, 0}, pitm, iprp)
	return append(ftyp, meta...)
}

func TestVersoFotoHEIC(t *testing.T) {
	rosso := color.RGBA{255, 0, 0, 255}
	finto := func([]byte) (image.Image, error) { // 4x2, rosso in alto a sinistra
		img := image.NewRGBA(image.Rect(0, 0, 4, 2))
		img.Set(0, 0, rosso)
		return img, nil
	}
	dec := conVersoHEIC(finto)
	cosa := func(b []byte) (int, int, int, int) {
		img, err := dec(b)
		if err != nil {
			t.Fatal(err)
		}
		bb := img.Bounds()
		for y := 0; y < bb.Dy(); y++ {
			for x := 0; x < bb.Dx(); x++ {
				if r, _, _, _ := img.At(x, y).RGBA(); r > 0xf000 {
					return bb.Dx(), bb.Dy(), x, y
				}
			}
		}
		return bb.Dx(), bb.Dy(), -1, -1
	}
	// iPhone in verticale: irot = 3 (270° antiorario = 90° orario): il rosso va in alto a destra.
	if w, h, x, y := cosa(heifProva(heifBox("irot", 3))); w != 2 || h != 4 || x != 1 || y != 0 {
		t.Fatalf("irot 3: %dx%d rosso in %d,%d", w, h, x, y)
	}
	// irot = 1 (90° antiorario): il rosso va in basso a sinistra.
	if w, h, x, y := cosa(heifProva(heifBox("irot", 1))); w != 2 || h != 4 || x != 0 || y != 3 {
		t.Fatalf("irot 1: %dx%d rosso in %d,%d", w, h, x, y)
	}
	// irot = 2 (180°)
	if w, h, x, y := cosa(heifProva(heifBox("irot", 2))); w != 4 || h != 2 || x != 3 || y != 1 {
		t.Fatalf("irot 2: %dx%d rosso in %d,%d", w, h, x, y)
	}
	// imir asse verticale (destra<->sinistra)
	if w, h, x, y := cosa(heifProva(heifBox("imir", 0))); w != 4 || h != 2 || x != 3 || y != 0 {
		t.Fatalf("imir: %dx%d rosso in %d,%d", w, h, x, y)
	}
	// nessuna proprietà o file non HEIF: la foto resta com'è
	if w, h, x, y := cosa(heifProva()); w != 4 || h != 2 || x != 0 || y != 0 {
		t.Fatalf("senza rotazione: %dx%d rosso in %d,%d", w, h, x, y)
	}
	if w, h, _, _ := cosa([]byte("non sono una foto")); w != 4 || h != 2 {
		t.Fatal("file strano")
	}
}

// heifBox: una proprietà (es. imir) già "inscatolata", da mettere in ipco.
func heifBox(tipo string, v byte) []byte {
	b := []byte{0, 0, 0, 9, 0, 0, 0, 0, v}
	copy(b[4:8], tipo)
	return b
}

func TestOrdinaPerNome(t *testing.T) {
	pp := []string{`C:\f\Foto 10.jpg`, `C:\f\foto 2.jpg`, `C:\f\Foto 1.jpg`, `C:\f\b.pdf`, `C:\f\A.pdf`}
	ordinaPerNome(pp)
	got := strings.Join(pp, "|")
	want := strings.Join([]string{`C:\f\A.pdf`, `C:\f\b.pdf`, `C:\f\Foto 1.jpg`, `C:\f\foto 2.jpg`, `C:\f\Foto 10.jpg`}, "|")
	if got != want {
		t.Fatalf("ordine:\n%s\n%s", got, want)
	}
}
