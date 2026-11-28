package archivio

// Prove del controllore ZP-02 (password e sicurezza). Solo nella copia /tmp/zp-02/src.

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"unicode/utf16"
)

// ---------- 7z cifrato fatto a mano (coder AES-256 + SHA-256, senza compressione) ----------

func num7(v uint64) []byte {
	switch {
	case v < 0x80:
		return []byte{byte(v)}
	case v < 0x4000:
		return []byte{0x80 | byte(v>>8), byte(v)}
	case v < 0x200000:
		return []byte{0xC0 | byte(v>>16), byte(v), byte(v >> 8)}
	}
	b := make([]byte, 9)
	b[0] = 0xFF
	binary.LittleEndian.PutUint64(b[1:], v)
	return b
}

func chiave7z(pw string, salt []byte) []byte {
	u := utf16.Encode([]rune(pw))
	pb := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(pb[2*i:], c)
	}
	h := sha256.New()
	var ctr [8]byte
	for i := uint64(0); i < 1<<19; i++ {
		h.Write(salt)
		h.Write(pb)
		binary.LittleEndian.PutUint64(ctr[:], i)
		h.Write(ctr[:])
	}
	return h.Sum(nil)
}

func cifra(pw string, dati []byte) (cif, props []byte) {
	iv := bytes.Repeat([]byte{0x42}, 16)
	k := chiave7z(pw, nil)
	blk, _ := aes.NewCipher(k)
	pad := append([]byte{}, dati...)
	for len(pad)%16 != 0 || len(pad) == 0 {
		pad = append(pad, 0)
	}
	cif = make([]byte, len(pad))
	cipher.NewCBCEncrypter(blk, iv).CryptBlocks(cif, pad)
	props = append([]byte{0x13 | 0x40, 0x0F}, iv...)
	return
}

// streamsInfo: un pacchetto, una cartella con il solo coder AES.
func streamsInfo(packPos uint64, packSize int, props []byte, unpack int, crc uint32) []byte {
	var b bytes.Buffer
	b.WriteByte(0x06) // PackInfo
	b.Write(num7(packPos))
	b.Write(num7(1))
	b.WriteByte(0x09)
	b.Write(num7(uint64(packSize)))
	b.WriteByte(0x00)
	b.WriteByte(0x07) // UnpackInfo
	b.WriteByte(0x0B)
	b.Write(num7(1))
	b.WriteByte(0x00)
	b.Write(num7(1))  // coder
	b.WriteByte(0x24) // id 4 byte, con proprietà
	b.Write([]byte{0x06, 0xF1, 0x07, 0x01})
	b.Write(num7(uint64(len(props))))
	b.Write(props)
	b.WriteByte(0x0C)
	b.Write(num7(uint64(unpack)))
	b.WriteByte(0x0A)
	b.WriteByte(0x01)
	binary.Write(&b, binary.LittleEndian, crc)
	b.WriteByte(0x00)
	b.WriteByte(0x00)
	return b.Bytes()
}

func setteZipCifrato(nome string, dati []byte, pw string, testaCifrata bool) []byte {
	cif, props := cifra(pw, dati)
	var h bytes.Buffer
	h.WriteByte(0x01) // Header
	h.WriteByte(0x04) // MainStreamsInfo
	h.Write(streamsInfo(0, len(cif), props, len(dati), crc32.ChecksumIEEE(dati)))
	h.WriteByte(0x05) // FilesInfo
	h.Write(num7(1))
	u := utf16.Encode([]rune(nome))
	nb := make([]byte, 2*len(u)+2)
	for i, c := range u {
		binary.LittleEndian.PutUint16(nb[2*i:], c)
	}
	h.WriteByte(0x11)
	h.Write(num7(uint64(len(nb) + 1)))
	h.WriteByte(0x00)
	h.Write(nb)
	h.WriteByte(0x00)
	h.WriteByte(0x00)
	testa := h.Bytes()
	corpo := append([]byte{}, cif...)
	if testaCifrata {
		hc, hp := cifra(pw, testa)
		pos := len(corpo)
		corpo = append(corpo, hc...)
		var e bytes.Buffer
		e.WriteByte(0x17)
		e.Write(streamsInfo(uint64(pos), len(hc), hp, len(testa), crc32.ChecksumIEEE(testa)))
		testa = e.Bytes()
	}
	var out bytes.Buffer
	out.Write([]byte("7z\xBC\xAF\x27\x1C\x00\x04"))
	var sh [20]byte
	binary.LittleEndian.PutUint64(sh[0:], uint64(len(corpo)))
	binary.LittleEndian.PutUint64(sh[8:], uint64(len(testa)))
	binary.LittleEndian.PutUint32(sh[16:], crc32.ChecksumIEEE(testa))
	binary.Write(&out, binary.LittleEndian, crc32.ChecksumIEEE(sh[:]))
	out.Write(sh[:])
	out.Write(corpo)
	out.Write(testa)
	return out.Bytes()
}

func elencoDir(t *testing.T, dir string) []string {
	var out []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if p == dir || err != nil {
			return nil
		}
		r, _ := filepath.Rel(dir, p)
		if d.Type()&fs.ModeSymlink != 0 {
			r += " -> LINK"
		} else if d.IsDir() {
			r += "/"
		}
		out = append(out, r)
		return nil
	})
	sort.Strings(out)
	return out
}

func TestZP02Password7z(t *testing.T) {
	dati := []byte("contenuto segreto della busta paga\n")
	pws := []string{"password", "pàsswörd€", "Caffè", "日本語パス", "🔑chiave", " spazi ", "a\"b'c\\d<e>"}
	for _, cif := range []bool{false, true} {
		for _, pw := range pws {
			dir := t.TempDir()
			arch := filepath.Join(dir, "busta.7z")
			os.WriteFile(arch, setteZipCifrato("busta.txt", dati, pw, cif), 0o644)
			// giusta
			r, err := Estrai(context.Background(), arch, OpzioniEstrai{Password: pw})
			if err != nil {
				t.Errorf("testaCifrata=%v pw=%q giusta: %v", cif, pw, err)
				continue
			}
			b, _ := os.ReadFile(filepath.Join(r.Cartella, "busta.txt"))
			if !bytes.Equal(b, dati) {
				t.Errorf("pw=%q: dati diversi %q (cartella %s: %v)", pw, b, r.Cartella, elencoDir(t, dir))
			}
			// sbagliata / vuota / variante NFD
			for _, sb := range []string{"", "Password", strings.ToUpper(pw), "pa\u0300sswo\u0308rd€"} {
				if sb == pw {
					continue
				}
				_, err := Estrai(context.Background(), arch, OpzioniEstrai{Password: sb})
				c := Codice(err)
				if sb == "" && c != "password" || sb != "" && c != "password_sbagliata" {
					t.Errorf("testaCifrata=%v pw=%q prova=%q: codice %q (%v)", cif, pw, sb, c, err)
				}
				_, errE := Elenca(context.Background(), arch, sb)
				_, errV := Verifica(context.Background(), arch, OpzioniVerifica{Password: sb})
				t.Logf("testaCifrata=%v pw=%q prova=%q: estrai=%q elenca=%q verifica=%q", cif, pw, sb, c, Codice(errE), Codice(errV))
			}
			// nessun avanzo
			for _, v := range elencoDir(t, dir) {
				if strings.Contains(v, ".tassozip-") {
					t.Errorf("avanzo lasciato: %s", v)
				}
			}
			el, err := Elenca(context.Background(), arch, pw)
			t.Logf("elenco pw giusta cif=%v: %v err=%v cifrato=%v", cif, len(el.Voci), err, el.Cifrato)
		}
	}
}

// ---------- zip con password ----------

func zipCifratoFinto(t *testing.T, p string, metodo uint16, soloUno bool) {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	if soloUno {
		w, _ := zw.Create("libero.txt")
		w.Write([]byte("in chiaro"))
	}
	h := &zip.FileHeader{Name: "segreto.txt", Method: metodo, Flags: 1, CRC32: 1, CompressedSize64: 16, UncompressedSize64: 4}
	w, err := zw.CreateRaw(h)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(bytes.Repeat([]byte{7}, 16))
	zw.Close()
	os.WriteFile(p, b.Bytes(), 0o644)
}

func TestZP02ZipCifrato(t *testing.T) {
	for _, c := range []struct {
		n      string
		metodo uint16
		uno    bool
	}{{"zipcrypto", 8, false}, {"aes", 99, false}, {"misto", 8, true}} {
		dir := t.TempDir()
		p := filepath.Join(dir, c.n+".zip")
		zipCifratoFinto(t, p, c.metodo, c.uno)
		_, err := Estrai(context.Background(), p, OpzioniEstrai{})
		_, errP := Estrai(context.Background(), p, OpzioniEstrai{Password: "x"})
		el, errE := Elenca(context.Background(), p, "")
		_, errV := Verifica(context.Background(), p, OpzioniVerifica{})
		rr, errR := RiparaZip(context.Background(), p, "", nil)
		t.Logf("%s: estrai=%q (con pw=%q) elenca=%q cifrato=%v metodo=%q verifica=%q ripara=%q salvati=%d persi=%v -> %s",
			c.n, Codice(err), Codice(errP), Codice(errE), el.Cifrato, el.Metodo, Codice(errV), Codice(errR), rr.Salvati, rr.Persi, filepath.Base(rr.Archivio))
		t.Logf("   cartella: %v", elencoDir(t, dir))
	}
	// zip vero ZipCrypto fatto da /usr/bin/zip, se c'è
	if _, err := os.Stat("/tmp/zp-02/dati/zipcrypto.zip"); err == nil {
		_, err := Estrai(context.Background(), "/tmp/zp-02/dati/zipcrypto.zip", OpzioniEstrai{Password: "pàss"})
		t.Logf("zip -P vero: %q", Codice(err))
	}
}

// ---------- zip-slip, nomi riservati, nomi lunghi ----------

func zipConNomi(t *testing.T, p string, nomi []string) {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, n := range nomi {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: n, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte("dati di " + n))
	}
	zw.Close()
	os.WriteFile(p, b.Bytes(), 0o644)
}

func TestZP02Slip(t *testing.T) {
	nomi := []string{
		"../fuori1.txt", "..\\fuori2.txt", "/assoluto.txt", "\\assoluto2.txt", "C:\\Windows\\fuori3.txt",
		"C:fuori4.txt", "c:/fuori5.txt", "\\\\server\\share\\fuori6.txt", "//server/share/fuori7.txt",
		"\\\\?\\C:\\fuori8.txt", "\\\\.\\C:\\fuori9.txt", "a/../../fuori10.txt", "a/b/../../../fuori11.txt",
		".. /fuori12.txt", "... /fuori13.txt", "..\u00a0/fuori14.txt", "a/..\\..\\fuori15.txt", "./../fuori16.txt",
		"a/./b/ok.txt", "sotto/..rc", "...strano", "x/.../y.txt",
		"CON", "con.txt", "Con.tar.gz", "COM0.log", "lpt0", "COM¹.txt", "NUL .txt", "aux.", "PRN ", "CONIN$", "conout$.txt",
		"CLOCK$", "punto.", "spazio ", "cartella./dentro.txt", "cartella /dentro2.txt", "nome:flusso.txt", "file::$DATA",
		"a\x00b.txt", "tab\tnome.txt", "rtl\u202Efdp.exe", "spazio\u00a0finale.exe\u00a0", "ideo.exe\u3000",
		strings.Repeat("L", 300) + ".pdf", strings.Repeat("è", 200) + ".docx", strings.Repeat("x", 300) + ".settingcontent-ms",
		strings.Repeat("d", 200) + "/" + strings.Repeat("e", 200) + "/" + strings.Repeat("f", 200) + "/dentro.txt",
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "attacco.zip")
	zipConNomi(t, p, nomi)
	base := filepath.Join(dir, "dest")
	os.Mkdir(base, 0o755)
	r, err := Estrai(context.Background(), p, OpzioniEstrai{Destinazione: base})
	if err != nil {
		t.Fatalf("estrai: %v", err)
	}
	t.Logf("file=%d rinominati=%d saltati=%d", r.File, r.Rinominati, len(r.Saltati))
	for _, s := range r.Saltati {
		t.Logf("  saltato: %q", s)
	}
	for _, v := range elencoDir(t, dir) {
		if len(v) > 120 {
			v = v[:60] + "…" + v[len(v)-50:]
		}
		t.Logf("  sul disco: %q", v)
	}
	// niente fuori dalla destinazione
	filepath.WalkDir("/", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() && (p == "/proc" || p == "/sys" || p == "/dev" || p == "/usr" || p == "/home" || p == "/root" || p == "/mnt" || p == "/opt") {
			return fs.SkipDir
		}
		if strings.HasPrefix(d.Name(), "fuori") && strings.HasSuffix(d.Name(), ".txt") {
			t.Errorf("FILE FUORI: %s", p)
		}
		return nil
	})
}

// ---------- tar con collegamenti ----------

func TestZP02TarLink(t *testing.T) {
	dir := t.TempDir()
	segreto := filepath.Join(dir, "segreto.txt")
	os.WriteFile(segreto, []byte("SEGRETO"), 0o644)
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	add := func(h *tar.Header, dati string) {
		h.Size = int64(len(dati))
		if h.Mode == 0 {
			h.Mode = 0o644
		}
		tw.WriteHeader(h)
		tw.Write([]byte(dati))
	}
	add(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: dir}, "")
	add(&tar.Header{Name: "link/scritto.txt", Typeflag: tar.TypeReg}, "ATTRAVERSO IL LINK")
	add(&tar.Header{Name: "rel", Typeflag: tar.TypeSymlink, Linkname: "../../"}, "")
	add(&tar.Header{Name: "rel/x.txt", Typeflag: tar.TypeReg}, "rel")
	add(&tar.Header{Name: "duro", Typeflag: tar.TypeLink, Linkname: segreto}, "")
	add(&tar.Header{Name: "dev", Typeflag: tar.TypeChar, Devmajor: 1, Devminor: 3}, "")
	add(&tar.Header{Name: "fifo", Typeflag: tar.TypeFifo}, "")
	add(&tar.Header{Name: "gnulong" + strings.Repeat("z", 150) + "/../../fuori-tar.txt", Typeflag: tar.TypeReg}, "x")
	add(&tar.Header{Name: "giunzione\\..\\..\\fuori-tar2.txt", Typeflag: tar.TypeReg}, "x")
	tw.Close()
	var gzb bytes.Buffer
	gw := gzip.NewWriter(&gzb)
	gw.Write(b.Bytes())
	gw.Close()
	p := filepath.Join(dir, "link.tar.gz")
	os.WriteFile(p, gzb.Bytes(), 0o644)
	out := filepath.Join(dir, "out")
	os.Mkdir(out, 0o755)
	r, err := Estrai(context.Background(), p, OpzioniEstrai{Destinazione: out})
	t.Logf("err=%v file=%d saltati=%v", err, r.File, r.Saltati)
	for _, v := range elencoDir(t, dir) {
		t.Logf("  %s", v)
	}
	if bb, _ := os.ReadFile(segreto); string(bb) != "SEGRETO" {
		t.Error("segreto cambiato")
	}
	if _, e := os.Stat(filepath.Join(dir, "scritto.txt")); e == nil {
		t.Error("scritto attraverso il link")
	}
	// zip con symlink unix
	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	h := &zip.FileHeader{Name: "zl", Method: zip.Store}
	h.SetMode(fs.ModeSymlink | 0o777)
	w, _ := zw.CreateHeader(h)
	w.Write([]byte(dir))
	w, _ = zw.Create("zl/dentro.txt")
	w.Write([]byte("x"))
	zw.Close()
	pz := filepath.Join(dir, "l.zip")
	os.WriteFile(pz, zb.Bytes(), 0o644)
	out2 := filepath.Join(dir, "out2")
	os.Mkdir(out2, 0o755)
	r, err = Estrai(context.Background(), pz, OpzioniEstrai{Destinazione: out2})
	t.Logf("zip link: err=%v file=%d saltati=%v %v", err, r.File, r.Saltati, elencoDir(t, out2))
}

// ---------- bombe ----------

func TestZP02Bombe(t *testing.T) {
	dir := t.TempDir()
	// 1) zip a voci sovrapposte (stile Fifield): una sola voce di dati, 2000 voci nell'indice.
	var dati bytes.Buffer
	{
		var b bytes.Buffer
		zw := zip.NewWriter(&b)
		w, _ := zw.CreateHeader(&zip.FileHeader{Name: "a", Method: zip.Deflate})
		w.Write(make([]byte, 50<<20))
		zw.Close()
		dati.Write(b.Bytes())
	}
	zr, _ := zip.NewReader(bytes.NewReader(dati.Bytes()), int64(dati.Len()))
	fh := zr.File[0]
	off, _ := fh.DataOffset()
	locale := dati.Bytes()[:off+int64(fh.CompressedSize64)]
	var cd bytes.Buffer
	n := 2000
	for i := 0; i < n; i++ {
		nome := []byte("f" + strings.Repeat("0", 5-len(itoa(i))) + itoa(i))
		hdr := make([]byte, 46)
		copy(hdr, "PK\x01\x02")
		binary.LittleEndian.PutUint16(hdr[4:], 20)
		binary.LittleEndian.PutUint16(hdr[6:], 20)
		binary.LittleEndian.PutUint16(hdr[10:], 8)
		binary.LittleEndian.PutUint32(hdr[16:], fh.CRC32)
		binary.LittleEndian.PutUint32(hdr[20:], uint32(fh.CompressedSize64))
		binary.LittleEndian.PutUint32(hdr[24:], uint32(fh.UncompressedSize64))
		binary.LittleEndian.PutUint16(hdr[28:], uint16(len(nome)))
		binary.LittleEndian.PutUint32(hdr[42:], 0)
		cd.Write(hdr)
		cd.Write(nome)
	}
	var bomba bytes.Buffer
	bomba.Write(locale)
	cdOff := bomba.Len()
	bomba.Write(cd.Bytes())
	eocd := make([]byte, 22)
	copy(eocd, "PK\x05\x06")
	binary.LittleEndian.PutUint16(eocd[8:], uint16(n))
	binary.LittleEndian.PutUint16(eocd[10:], uint16(n))
	binary.LittleEndian.PutUint32(eocd[12:], uint32(cd.Len()))
	binary.LittleEndian.PutUint32(eocd[16:], uint32(cdOff))
	bomba.Write(eocd)
	pb := filepath.Join(dir, "sovrapposto.zip")
	os.WriteFile(pb, bomba.Bytes(), 0o644)
	_, err := Estrai(context.Background(), pb, OpzioniEstrai{})
	t.Logf("zip sovrapposto (%d KB, dichiara %d GB): %q %v", bomba.Len()>>10, int64(n)*50>>10, Codice(err), err)

	// 2) gz di zeri con limiti piccoli (per non scrivere GB): si ferma e non lascia niente
	var g bytes.Buffer
	gw, _ := gzip.NewWriterLevel(&g, 9)
	gw.Write(make([]byte, 200<<20))
	gw.Close()
	pg := filepath.Join(dir, "zeri.txt.gz")
	os.WriteFile(pg, g.Bytes(), 0o644)
	lim := LimitiPredefiniti()
	lim.SogliaRapporto = 20 << 20
	lim.MaxRapporto = 10
	_, err = Estrai(context.Background(), pg, OpzioniEstrai{Limiti: &lim})
	t.Logf("gz %d KB -> 200 MB, limite 20 MB: %q", g.Len()>>10, Codice(err))
	t.Logf("consentito con i limiti veri per un archivio di %d KB: %d MB", g.Len()>>10, LimitiPredefiniti().consentito(int64(g.Len()))>>20)

	// 3) a strati: zip dentro zip dentro zip
	interno := dati.Bytes()
	for i := 0; i < 3; i++ {
		var b bytes.Buffer
		zw := zip.NewWriter(&b)
		for j := 0; j < 10; j++ {
			w, _ := zw.CreateHeader(&zip.FileHeader{Name: "livello" + itoa(i) + "_" + itoa(j) + ".zip", Method: zip.Store})
			w.Write(interno)
		}
		zw.Close()
		interno = b.Bytes()
	}
	ps := filepath.Join(dir, "strati.zip")
	os.WriteFile(ps, interno, 0o644)
	r, err := Estrai(context.Background(), ps, OpzioniEstrai{})
	t.Logf("a strati (%d KB): %q file=%d (non entra negli zip interni)", len(interno)>>10, Codice(err), r.File)
	os.RemoveAll(r.Cartella)
	// 4) troppe voci
	lim2 := LimitiPredefiniti()
	lim2.MaxVoci = 1000
	_, err = Estrai(context.Background(), pb, OpzioniEstrai{Limiti: &lim2})
	t.Logf("2000 voci con MaxVoci 1000: %q", Codice(err))
	t.Logf("cartella alla fine: %v", elencoDir(t, dir))
}

func itoa(i int) string {
	return strings.TrimSpace(strings.Repeat(" ", 0) + func() string { return string([]byte(fmtInt(i))) }())
}
func fmtInt(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}

// ---------- apri voce: estensione che cambia, cartella con lo stesso nome ----------

// copia esatta di main_windows.go (pericoloso)
var estPer = map[string]bool{".exe": true, ".bat": true, ".cmd": true, ".js": true, ".lnk": true, ".hta": true, ".vbs": true, ".ps1": true, ".scr": true}

func pericolosoCopia(nome string) bool {
	return estPer[strings.ToLower(filepath.Ext(strings.TrimRight(nome, ". ")))]
}

func TestZP02ApriVoce(t *testing.T) {
	// a) nome con spazio "speciale" in fondo: la conferma non scatta, ma il file esce .exe
	for _, n := range []string{"fattura.exe\u00a0", "fattura.exe\u3000", "fattura.exe\u2002", "fattura.bat\u0085", "fattura.exe\u200b", "fattura.exe . ", "fattura.exe\t"} {
		pz, _ := NomeSicuro(n)
		fin := pz[len(pz)-1]
		t.Logf("%q: conferma=%v -> sul disco %q (pericoloso? %v)", n, pericolosoCopia(n), fin, pericolosoCopia(fin))
	}
	// b) file "fattura.pdf" e cartella "fattura.pdf/" con dentro un programma
	dir := t.TempDir()
	p := filepath.Join(dir, "doc.zip")
	zipConNomi(t, p, []string{"fattura.pdf", "fattura.pdf/avvia.exe"})
	el, _ := Elenca(context.Background(), p, "")
	for _, v := range el.Voci {
		t.Logf("elenco: %q cartella=%v", v.Nome, v.Cartella)
	}
	tmp := filepath.Join(dir, "apri-1")
	os.Mkdir(tmp, 0o755)
	r, err := Estrai(context.Background(), p, OpzioniEstrai{Selezione: []string{"fattura.pdf"}, Modo: ModoQui, Destinazione: tmp})
	t.Logf("doppio clic su \"fattura.pdf\" (conferma=%v): err=%v UltimoFile=%q", pericolosoCopia("fattura.pdf"), err, r.UltimoFile)
	t.Logf("   in Temp: %v", elencoDir(t, tmp))
	// c) "fattura.pdf" seguito da un secondo "fattura.pdf" che è un .exe? (stesso nome: stessa estensione)
	// d) apriVoce con un nome che si normalizza: "./fattura.pdf" e "fattura.pdf/../x.exe" (rifiutato)
	p2 := filepath.Join(dir, "doc2.zip")
	zipConNomi(t, p2, []string{"a/fattura.pdf", "a\\fattura.pdf\\avvia.cmd"})
	tmp2 := filepath.Join(dir, "apri-2")
	os.Mkdir(tmp2, 0o755)
	r, err = Estrai(context.Background(), p2, OpzioniEstrai{Selezione: []string{"a/fattura.pdf"}, TogliPrefisso: "a", Modo: ModoQui, Destinazione: tmp2})
	t.Logf("in sottocartella: err=%v UltimoFile=%q %v", err, r.UltimoFile, elencoDir(t, tmp2))
}

// ---------- marchio del web su tutti i percorsi ----------

func TestZP02Zona(t *testing.T) {
	vl, vs := leggiZona, scriviZona
	defer func() { leggiZona, scriviZona = vl, vs }()
	var mu sync.Mutex
	segnati := map[string]bool{}
	leggiZona = func(p string) []byte { return []byte("[ZoneTransfer]\r\nZoneId=3\r\n") }
	scriviZona = func(p string, b []byte) { mu.Lock(); segnati[filepath.Base(p)] = true; mu.Unlock() }
	dir := t.TempDir()
	p := filepath.Join(dir, "web.zip")
	zipConNomi(t, p, []string{"uno.docm", "sub/due.xlsm", "CON.txt", strings.Repeat("L", 300) + ".js"})
	for _, m := range []ModoEstrai{ModoNormale, ModoQui, ModoCartella} {
		d := filepath.Join(dir, "m"+string(m))
		os.Mkdir(d, 0o755)
		segnati = map[string]bool{}
		r, err := Estrai(context.Background(), p, OpzioniEstrai{Modo: m, Destinazione: d})
		t.Logf("modo %q: err=%v file=%d segnati=%d", m, err, r.File, len(segnati))
	}
	// 7z cifrato
	p7 := filepath.Join(dir, "web.7z")
	os.WriteFile(p7, setteZipCifrato("x.txt", []byte("ciao"), "pw", true), 0o644)
	segnati = map[string]bool{}
	_, err := Estrai(context.Background(), p7, OpzioniEstrai{Password: "pw"})
	t.Logf("7z cifrato: err=%v segnati=%v", err, segnati)
	// gz singolo
	var g bytes.Buffer
	gw := gzip.NewWriter(&g)
	gw.Write([]byte("ciao"))
	gw.Close()
	pg := filepath.Join(dir, "nota.txt.gz")
	os.WriteFile(pg, g.Bytes(), 0o644)
	segnati = map[string]bool{}
	_, err = Estrai(context.Background(), pg, OpzioniEstrai{})
	t.Logf("gz: err=%v segnati=%v", err, segnati)
}

var _ = io.EOF

func TestZP02Nbsp(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "n.zip")
	n := "fattura.exe "
	zipConNomi(t, p, []string{n})
	el, _ := Elenca(context.Background(), p, "")
	tmp := filepath.Join(dir, "apri")
	os.Mkdir(tmp, 0o755)
	r, err := Estrai(context.Background(), p, OpzioniEstrai{Selezione: []string{el.Voci[0].Nome}, Modo: ModoQui, Destinazione: tmp})
	t.Logf("elenco %q conferma=%v -> err=%v UltimoFile=%q", el.Voci[0].Nome, pericolosoCopia(el.Voci[0].Nome), err, filepath.Base(r.UltimoFile))
}
