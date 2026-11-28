package archivio

// Correzioni di CTRL-ZP-01 (comprimere ed estrarre).

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Trovato correggendo ZP01: con tanti file piccoli e ripetitivi (più di 1 MB) il .7z andava in
// panico nell'encoder LZMA2 (indice fuori dal buffer circolare). Andata e ritorno identici.
func TestZP01SetteZipFilePiccoliRipetitivi(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	for d := 0; d < 20; d++ {
		dd := filepath.Join(src, fmt.Sprintf("c%02d", d))
		os.MkdirAll(dd, 0o755)
		for i := 0; i < 1000; i++ {
			os.WriteFile(filepath.Join(dd, fmt.Sprintf("f%d.txt", i)), []byte(fmt.Sprintf("contenuto piccolo del file numero %d %s", i*d, strings.Repeat("ab", i%7))), 0o644)
		}
	}
	for _, l := range []Livello{LivelloVeloce, LivelloMassima, LivelloUltra} {
		dove := filepath.Join(dir, string(l))
		os.MkdirAll(dove, 0o755)
		r, err := Comprimi(context.Background(), []string{src}, OpzioniComprimi{Formato: Formato7z, Livello: l, Cartella: dove})
		if err != nil {
			t.Fatal(l, err)
		}
		e, err := Estrai(context.Background(), r.Archivio, OpzioniEstrai{})
		if err != nil || e.File != 20000 {
			t.Fatalf("%s: estratti %d: %v", l, e.File, err)
		}
		if a, b := impronta(t, src), impronta(t, e.Cartella); a != b {
			t.Fatalf("%s: contenuto diverso dopo andata e ritorno", l)
		}
	}
}

func impronta(t *testing.T, dir string) string {
	h := sha256.New()
	filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		r, _ := filepath.Rel(dir, p)
		b, _ := os.ReadFile(p)
		fmt.Fprintf(h, "%s %x\n", filepath.ToSlash(r), sha256.Sum256(b))
		return nil
	})
	return fmt.Sprintf("%x", h.Sum(nil))
}

// ZP01-1: zip con la sola data MS-DOS: è ora locale.
func TestZP01DataZipLocale(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "dos.zip")
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	h := &zip.FileHeader{Name: "a.txt", Method: zip.Store}
	h.ModifiedDate = uint16((2024-1980)<<9 | 7<<5 | 15) // 15 luglio 2024
	h.ModifiedTime = uint16(23<<11 | 13<<5 | 20/2)      // 23:13:20
	w, _ := zw.CreateRaw(&zip.FileHeader{Name: h.Name, Method: zip.Store, ModifiedDate: h.ModifiedDate, ModifiedTime: h.ModifiedTime,
		CRC32: crc32.ChecksumIEEE([]byte("ciao")), CompressedSize64: 4, UncompressedSize64: 4})
	w.Write([]byte("ciao"))
	zw.Close()
	os.WriteFile(p, b.Bytes(), 0o644)
	r, err := Estrai(context.Background(), p, OpzioniEstrai{})
	if err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(filepath.Join(r.Cartella, "a.txt"))
	atteso := time.Date(2024, 7, 15, 23, 13, 20, 0, time.Local)
	if !fi.ModTime().Equal(atteso) {
		t.Errorf("data %v, attesa %v (ora locale)", fi.ModTime(), atteso)
	}
	el, _ := Elenca(context.Background(), p, "")
	if !el.Voci[0].Quando.Equal(atteso) {
		t.Errorf("elenco: %v", el.Voci[0].Quando)
	}
	// uno zip di Tasso Zip (con la data estesa) resta giusto
	src := filepath.Join(dir, "x.txt")
	os.WriteFile(src, []byte("x"), 0o644)
	q := time.Date(2001, 3, 4, 5, 6, 8, 0, time.Local)
	os.Chtimes(src, q, q)
	c, err := Comprimi(context.Background(), []string{src}, OpzioniComprimi{Formato: FormatoZip, Cartella: filepath.Join(dir)})
	if err != nil {
		t.Fatal(err)
	}
	r, err = Estrai(context.Background(), c.Archivio, OpzioniEstrai{Destinazione: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(r.UltimoFile); !fi.ModTime().Equal(q) {
		t.Errorf("zip di Tasso Zip: %v invece di %v", fi.ModTime(), q)
	}
}

// ZP01-3 e ZP01-11: gli hard link dei tar escono come file (copia del file già estratto) e
// compaiono nell'elenco; i symlink restano saltati; il file "contiguo" è nell'elenco.
func TestZP01TarHardLink(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "hl.tar")
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	tw.WriteHeader(&tar.Header{Name: "hl/b.txt", Typeflag: tar.TypeReg, Size: 5, Mode: 0o644})
	tw.Write([]byte("dati\n"))
	tw.WriteHeader(&tar.Header{Name: "hl/a.txt", Typeflag: tar.TypeLink, Linkname: "hl/b.txt"})
	tw.WriteHeader(&tar.Header{Name: "hl/fuori.txt", Typeflag: tar.TypeLink, Linkname: "../../etc/passwd"})
	tw.WriteHeader(&tar.Header{Name: "hl/s.txt", Typeflag: tar.TypeSymlink, Linkname: "b.txt"})
	tw.WriteHeader(&tar.Header{Name: "hl/c.txt", Typeflag: tar.TypeCont, Size: 2, Mode: 0o644})
	tw.Write([]byte("ok"))
	tw.Close()
	os.WriteFile(p, b.Bytes(), 0o644)
	r, err := Estrai(context.Background(), p, OpzioniEstrai{})
	if err != nil {
		t.Fatal(err)
	}
	if x, _ := os.ReadFile(filepath.Join(r.Cartella, "a.txt")); string(x) != "dati\n" {
		t.Errorf("hard link: %q", x)
	}
	if r.File != 3 || len(r.Saltati) != 2 {
		t.Errorf("file=%d saltati=%v", r.File, r.Saltati)
	}
	el, err := Elenca(context.Background(), p, "")
	if err != nil {
		t.Fatal(err)
	}
	nomi := map[string]bool{}
	for _, v := range el.Voci {
		nomi[v.Nome] = true
	}
	if !nomi["hl/a.txt"] || !nomi["hl/c.txt"] {
		t.Errorf("elenco: %v", nomi)
	}
	if _, err := Verifica(context.Background(), p, OpzioniVerifica{}); err != nil {
		t.Errorf("verifica: %v", err)
	}
}

// ZP01-4: metodi zip. BZip2 e LZMA si aprono; un metodo sconosciuto non è "rovinato".
func TestZP01MetodiZip(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"py_bzip2.zip", "py_lzma.zip"} {
		p := filepath.Join("testdata", f)
		if _, err := os.Stat(p); err != nil {
			t.Skip("manca " + p)
		}
		r, err := Estrai(context.Background(), p, OpzioniEstrai{Destinazione: dir})
		if err != nil || r.File == 0 {
			t.Errorf("%s: file=%d err=%v", f, r.File, err)
		}
		if _, err := Verifica(context.Background(), p, OpzioniVerifica{}); err != nil {
			t.Errorf("%s verifica: %v", f, err)
		}
	}
	// metodo 9 (Deflate64): nessun decompressore
	p := filepath.Join(dir, "d64.zip")
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	w, _ := zw.CreateRaw(&zip.FileHeader{Name: "x.bin", Method: 9, CRC32: 1, CompressedSize64: 3, UncompressedSize64: 3})
	w.Write([]byte{1, 2, 3})
	zw.Close()
	os.WriteFile(p, b.Bytes(), 0o644)
	if _, err := Estrai(context.Background(), p, OpzioniEstrai{}); Codice(err) != "metodo" {
		t.Errorf("Deflate64: %q (%v)", Codice(err), err)
	}
	if _, err := Verifica(context.Background(), p, OpzioniVerifica{}); Codice(err) != "metodo" {
		t.Errorf("Deflate64 verifica: %q", Codice(err))
	}
}

// ZP01-5 e ZP01-6: "Estrai qui" con nomi già presenti e nomi lunghissimi: sono contati.
func TestZP01RinominatiQui(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.zip")
	lungo := strings.Repeat("è", 200) + ".txt"
	if !suWindows {
		lungo = strings.Repeat("e", 230) + ".txt" // fuori da Windows il limite è in byte
	}
	zipConNomi(t, p, []string{"uno.txt", "sub/due.txt", lungo})
	dest := filepath.Join(dir, "qui")
	os.MkdirAll(filepath.Join(dest, "sub"), 0o755)
	os.WriteFile(filepath.Join(dest, "uno.txt"), []byte("mio"), 0o644)
	r, err := Estrai(context.Background(), p, OpzioniEstrai{Modo: ModoQui, Destinazione: dest})
	if err != nil {
		t.Fatal(err)
	}
	if r.Rinominati != 2 {
		t.Errorf("rinominati %d, attesi 2 (uno.txt e sub)", r.Rinominati)
	}
	if x, _ := os.ReadFile(filepath.Join(dest, "uno.txt")); string(x) != "mio" {
		t.Error("toccato il file dell'utente")
	}
	// 200 "è" sono 400 byte ma 204 caratteri UTF-16: su Windows il nome resta intero
	if _, err := os.Stat(filepath.Join(dest, lungo)); err != nil {
		t.Errorf("nome lungo tagliato: %v", err)
	}
	if n := PulisciNome(strings.Repeat("日", 300) + ".pdf"); lunghezzaWindows(n) > maxNomeWindows || !strings.HasSuffix(n, ".pdf") {
		t.Errorf("nome di 300 caratteri: %d caratteri", len([]rune(n)))
	}
}

// ZP01-7, ZP01-8, ZP01-9: date delle cartelle, sola lettura, date prima del 1981.
func TestZP01DateESolaLettura(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "Foto")
	os.MkdirAll(filepath.Join(src, "sub"), 0o755)
	vecchia := time.Date(1975, 5, 6, 7, 8, 10, 0, time.Local)
	os.WriteFile(filepath.Join(src, "sub", "vecchio.txt"), []byte("1975"), 0o644)
	os.Chtimes(filepath.Join(src, "sub", "vecchio.txt"), vecchia, vecchia)
	os.WriteFile(filepath.Join(src, "sola.txt"), []byte("non toccare"), 0o444)
	dc := time.Date(2001, 2, 3, 4, 5, 6, 0, time.Local)
	os.Chtimes(filepath.Join(src, "sub"), dc, dc)
	for _, f := range []Formato{FormatoZip, Formato7z, FormatoTarGz} {
		dove := filepath.Join(dir, string(f))
		os.MkdirAll(dove, 0o755)
		c, err := Comprimi(context.Background(), []string{src}, OpzioniComprimi{Formato: f, Cartella: dove})
		if err != nil {
			t.Fatal(f, err)
		}
		r, err := Estrai(context.Background(), c.Archivio, OpzioniEstrai{Destinazione: dove})
		if err != nil {
			t.Fatal(f, err)
		}
		if fi, _ := os.Stat(filepath.Join(r.Cartella, "sub")); !fi.ModTime().Equal(dc) {
			t.Errorf("%s: data della cartella %v", f, fi.ModTime())
		}
		if fi, _ := os.Stat(filepath.Join(r.Cartella, "sola.txt")); fi.Mode().Perm()&0o200 != 0 {
			t.Errorf("%s: sola lettura persa (%v)", f, fi.Mode())
		}
		fi, _ := os.Stat(filepath.Join(r.Cartella, "sub", "vecchio.txt"))
		atteso := vecchia
		if f == FormatoZip {
			atteso = time.Date(1980, 1, 1, 0, 0, 0, 0, time.Local) // lo zip non va prima del 1980
		}
		if !fi.ModTime().Equal(atteso) {
			t.Errorf("%s: data del file del 1975: %v (attesa %v)", f, fi.ModTime(), atteso)
		}
	}
}

// ZP01-10: il nome e la data che porta dentro un .gz singolo.
func TestZP01GzNome(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "scaricato.gz")
	var b bytes.Buffer
	gw := gzip.NewWriter(&b)
	gw.Name = "rapporto.txt"
	gw.ModTime = time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	gw.Write([]byte("testo"))
	gw.Close()
	os.WriteFile(p, b.Bytes(), 0o644)
	r, err := Estrai(context.Background(), p, OpzioniEstrai{Modo: ModoQui})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(r.UltimoFile) != "rapporto.txt" {
		t.Errorf("nome: %s", r.UltimoFile)
	}
	if fi, _ := os.Stat(r.UltimoFile); !fi.ModTime().Equal(gw.ModTime) {
		t.Errorf("data: %v", fi.ModTime())
	}
	// un nome pericoloso dentro il gz resta dentro la cartella
	p2 := filepath.Join(dir, "x.gz")
	b.Reset()
	gw = gzip.NewWriter(&b)
	gw.Name = `..\..\fuori.txt`
	gw.Write([]byte("x"))
	gw.Close()
	os.WriteFile(p2, b.Bytes(), 0o644)
	d := filepath.Join(dir, "d")
	os.Mkdir(d, 0o755)
	r, err = Estrai(context.Background(), p2, OpzioniEstrai{Modo: ModoQui, Destinazione: d})
	if err != nil || filepath.Dir(r.UltimoFile) != d {
		t.Errorf("nome con ..: %v %v", r.UltimoFile, err)
	}
}

// ZP01-12: tar vuoti.
func TestZP01TarVuoto(t *testing.T) {
	dir := t.TempDir()
	var b bytes.Buffer
	tar.NewWriter(&b).Close()
	p := filepath.Join(dir, "vuoto.tar")
	os.WriteFile(p, b.Bytes(), 0o644)
	if r, err := Estrai(context.Background(), p, OpzioniEstrai{}); err != nil || r.File != 0 {
		t.Errorf("tar vuoto: %v", err)
	}
	var g bytes.Buffer
	gw := gzip.NewWriter(&g)
	io.Copy(gw, bytes.NewReader(b.Bytes()))
	gw.Close()
	pg := filepath.Join(dir, "vuoto.tar.gz")
	os.WriteFile(pg, g.Bytes(), 0o644)
	r, err := Estrai(context.Background(), pg, OpzioniEstrai{})
	if err != nil || r.File != 0 {
		t.Errorf("tar.gz vuoto: file=%d %v", r.File, err)
	}
	if el, err := Elenca(context.Background(), pg, ""); err != nil || len(el.Voci) != 0 {
		t.Errorf("elenco tar.gz vuoto: %v %v", el.Voci, err)
	}
}
