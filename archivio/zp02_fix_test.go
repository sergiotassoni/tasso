package archivio

// Correzioni di CTRL-ZP-02: controlli veri (le prove del controllore, zp02_test.go, scrivono
// solo quello che vedono).

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

// ZP02-1: con Esatto esce solo la voce scelta, non la cartella con lo stesso nome.
func TestZP02FixApriVoceEsatta(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "doc.zip")
	zipConNomi(t, p, []string{"fattura.pdf", "fattura.pdf/avvia.exe", "a/fattura.pdf", `a\fattura.pdf\avvia.cmd`})
	for _, c := range []struct{ sel, pref string }{{"fattura.pdf", ""}, {"a/fattura.pdf", "a"}} {
		tmp := filepath.Join(dir, "apri-"+strings.ReplaceAll(c.sel, "/", "_"))
		os.Mkdir(tmp, 0o755)
		r, err := Estrai(context.Background(), p, OpzioniEstrai{Selezione: []string{c.sel}, TogliPrefisso: c.pref, Esatto: true, Modo: ModoQui, Destinazione: tmp})
		if err != nil {
			t.Fatal(err)
		}
		if r.File != 1 || filepath.Base(r.UltimoFile) != "fattura.pdf" {
			t.Errorf("%s: file=%d UltimoFile=%q", c.sel, r.File, r.UltimoFile)
		}
		voci, _ := os.ReadDir(tmp)
		if len(voci) != 1 || voci[0].IsDir() {
			t.Errorf("%s: nella cartella temporanea c'è %v", c.sel, voci)
		}
	}
	// senza Esatto (estrai la selezione) la cartella scelta esce con quello che contiene, come prima
	tmp := filepath.Join(dir, "sel")
	os.Mkdir(tmp, 0o755)
	r, err := Estrai(context.Background(), p, OpzioniEstrai{Selezione: []string{"fattura.pdf"}, Modo: ModoQui, Destinazione: tmp})
	if err != nil || r.File != 2 {
		t.Errorf("selezione normale: file=%d err=%v", r.File, err)
	}
}

// ZP02-2 e ZP02-12: la pulizia del nome (spazi Unicode in fondo) e ".." con uno spazio speciale.
func TestZP02FixPulisciNome(t *testing.T) {
	for n, atteso := range map[string]string{
		"fattura.exe ": "fattura.exe", "fattura.exe　": "fattura.exe", "fattura.exe ": "fattura.exe",
		"fattura.bat\u0085": "fattura.bat", "fattura.exe . ": "fattura.exe", "foto.jpg": "foto.jpg",
	} {
		if g := PulisciNome(n); g != atteso {
			t.Errorf("PulisciNome(%q) = %q, atteso %q", n, g, atteso)
		}
	}
	for _, n := range []string{".. /x.txt", "a/..　/x.txt", "..  /x.txt"} {
		if _, ok := NomeSicuro(n); ok {
			t.Errorf("%q accettato: doveva essere rifiutato come \".. /\"", n)
		}
	}
}

// ZP02-4: 7z con dati cifrati senza compressione (solo AES) e nomi in chiaro: senza password
// si chiede la password, e l'elenco dice "con password".
func TestZP02FixSetteZipSoloAES(t *testing.T) {
	dir := t.TempDir()
	arch := filepath.Join(dir, "busta.7z")
	os.WriteFile(arch, setteZipCifrato("busta.txt", []byte("segreto\n"), "pàss", false), 0o644)
	if _, err := Estrai(context.Background(), arch, OpzioniEstrai{}); !errors.Is(err, ErrPassword) {
		t.Errorf("estrai senza password: %v", err)
	}
	if _, err := Verifica(context.Background(), arch, OpzioniVerifica{}); !errors.Is(err, ErrPassword) {
		t.Errorf("verifica senza password: %v", err)
	}
	el, err := Elenca(context.Background(), arch, "")
	if err != nil || !el.Cifrato {
		t.Errorf("elenco: cifrato=%v err=%v", el.Cifrato, err)
	}
	if r, err := Estrai(context.Background(), arch, OpzioniEstrai{Password: "pàss"}); err != nil || r.File != 1 {
		t.Errorf("con la password giusta: %v", err)
	}
	// un 7z normale non è "con password"
	p := filepath.Join(dir, "chiaro.7z")
	os.WriteFile(p, setteZipCopia("nota.txt", []byte("ciao"), 0x20), 0o644)
	if el, err := Elenca(context.Background(), p, ""); err != nil || el.Cifrato {
		t.Errorf("7z in chiaro: cifrato=%v err=%v", el.Cifrato, err)
	}
}

// ZP02-6 e ZP02-7: zip con password solo su alcuni file; AES riconosciuto.
func TestZP02FixZipMisto(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "misto.zip")
	zipCifratoFinto(t, p, 8, true)
	r, err := Estrai(context.Background(), p, OpzioniEstrai{})
	if err != nil || r.File != 1 || len(r.Saltati) != 1 || !strings.Contains(r.Saltati[0], "con_password") {
		t.Fatalf("misto: file=%d saltati=%v err=%v", r.File, r.Saltati, err)
	}
	if _, err := os.Stat(filepath.Join(r.Cartella, "libero.txt")); err != nil {
		t.Error("il file in chiaro non è uscito")
	}
	// apri voce sul file con la password: zip_cifrato, non "rovinato"
	if _, err := Estrai(context.Background(), p, OpzioniEstrai{Selezione: []string{"segreto.txt"}, Esatto: true}); Codice(err) != "zip_cifrato" {
		t.Errorf("solo il file cifrato: %q", Codice(err))
	}
	// la verifica non può dire "a posto"
	if _, err := Verifica(context.Background(), p, OpzioniVerifica{}); Codice(err) != "zip_cifrato" {
		t.Errorf("verifica del misto: %q", Codice(err))
	}
	tutto := filepath.Join(dir, "tutto.zip")
	zipCifratoFinto(t, tutto, 8, false)
	if _, err := Estrai(context.Background(), tutto, OpzioniEstrai{}); Codice(err) != "zip_cifrato" {
		t.Errorf("tutto cifrato: %q", Codice(err))
	}
	aes := filepath.Join(dir, "aes.zip")
	zipCifratoFinto(t, aes, 99, false)
	if _, err := Estrai(context.Background(), aes, OpzioniEstrai{}); Codice(err) != "zip_cifrato_aes" {
		t.Errorf("AES: %q", Codice(err))
	}
	if !errors.Is(ErrZipCifratoAES, ErrZipCifrato) {
		t.Error("ErrZipCifratoAES deve valere anche come ErrZipCifrato")
	}
}

// ZP02-8: nei 7z i collegamenti di Windows (attributo 0x400 con i dati del collegamento) non escono.
func TestZP02FixSetteZipCollegamento(t *testing.T) {
	dir := t.TempDir()
	// REPARSE_DATA_BUFFER di un collegamento simbolico (IO_REPARSE_TAG_SYMLINK)
	link := make([]byte, 8+12)
	binary.LittleEndian.PutUint32(link[0:], 0xA000000C)
	binary.LittleEndian.PutUint16(link[4:], 12)
	p := filepath.Join(dir, "link.7z")
	os.WriteFile(p, setteZipCopia("collegamento", link, 0x20|0x400), 0o644)
	r, err := Estrai(context.Background(), p, OpzioniEstrai{})
	if err != nil && !errors.Is(err, ErrRovinato) {
		t.Fatal(err)
	}
	if r.File != 0 || len(r.Saltati) != 1 || !strings.Contains(r.Saltati[0], "collegamento") {
		t.Errorf("collegamento: file=%d saltati=%v err=%v", r.File, r.Saltati, err)
	}
	// stesso attributo ma dati veri (file di OneDrive): esce
	p2 := filepath.Join(dir, "onedrive.7z")
	os.WriteFile(p2, setteZipCopia("nota.txt", []byte("testo normale di una nota"), 0x20|0x400), 0o644)
	r, err = Estrai(context.Background(), p2, OpzioniEstrai{})
	if err != nil || r.File != 1 {
		t.Errorf("file con attributo 0x400 e dati veri: file=%d err=%v", r.File, err)
	}
	if b, _ := os.ReadFile(r.UltimoFile); string(b) != "testo normale di una nota" {
		t.Errorf("dati: %q", b)
	}
}

// ZP02-9: senza rapporto alto, al massimo 1 GB.
func TestZP02FixSoglia(t *testing.T) {
	l := LimitiPredefiniti()
	if c := l.consentito(199 << 10); c != 1<<30 {
		t.Errorf("archivio di 199 KB: %d MB consentiti", c>>20)
	}
	if c := l.consentito(10 << 20); c != 10<<20*1000 {
		t.Errorf("archivio di 10 MB: %d MB consentiti", c>>20)
	}
}

// ZP02-11: comprimendo, i file di lavoro stanno in una cartella di lavoro (che alla fine non
// c'è più) e non finiscono nell'archivio, anche se lo salvo dentro la cartella che comprimo.
func TestZP02FixComprimiCartellaLavoro(t *testing.T) {
	for _, f := range []Formato{FormatoZip, Formato7z} {
		dir := t.TempDir()
		src := filepath.Join(dir, "Foto")
		os.MkdirAll(src, 0o755)
		for i := 0; i < 5; i++ {
			os.WriteFile(filepath.Join(src, "f"+string(rune('a'+i))+".txt"), bytes.Repeat([]byte("dati "), 2000), 0o644)
		}
		r, err := Comprimi(context.Background(), []string{src}, OpzioniComprimi{Formato: f, Cartella: src})
		if err != nil {
			t.Fatal(err)
		}
		voci, _ := os.ReadDir(src)
		for _, v := range voci {
			if strings.HasPrefix(v.Name(), ".tassozip-") {
				t.Errorf("%s: rimasto %s", f, v.Name())
			}
		}
		el, err := Elenca(context.Background(), r.Archivio, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range el.Voci {
			if strings.Contains(v.Nome, ".tassozip-") || strings.Contains(v.Nome, ".in-corso") {
				t.Errorf("%s: nell'archivio c'è %s", f, v.Nome)
			}
		}
	}
}

// setteZipCopia: un 7z con un file solo, senza compressione (coder "Copy"), con gli attributi.
func setteZipCopia(nome string, dati []byte, attr uint32) []byte {
	var h bytes.Buffer
	h.WriteByte(0x01) // Header
	h.WriteByte(0x04) // MainStreamsInfo
	h.WriteByte(0x06) // PackInfo
	h.Write(num7(0))
	h.Write(num7(1))
	h.WriteByte(0x09)
	h.Write(num7(uint64(len(dati))))
	h.WriteByte(0x00)
	h.WriteByte(0x07) // UnpackInfo
	h.WriteByte(0x0B)
	h.Write(num7(1))
	h.WriteByte(0x00)
	h.Write(num7(1))  // un coder
	h.WriteByte(0x01) // id di 1 byte, senza proprietà
	h.WriteByte(0x00) // Copy
	h.WriteByte(0x0C)
	h.Write(num7(uint64(len(dati))))
	h.WriteByte(0x0A)
	h.WriteByte(0x01)
	binary.Write(&h, binary.LittleEndian, crc32.ChecksumIEEE(dati))
	h.WriteByte(0x00)
	h.WriteByte(0x00)
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
	h.WriteByte(0x15) // attributi
	h.Write(num7(6))
	h.WriteByte(0x01) // tutti definiti
	h.WriteByte(0x00) // non esterni
	binary.Write(&h, binary.LittleEndian, attr)
	h.WriteByte(0x00)
	h.WriteByte(0x00)
	testa := h.Bytes()
	var out bytes.Buffer
	out.Write([]byte("7z\xBC\xAF\x27\x1C\x00\x04"))
	var sh [20]byte
	binary.LittleEndian.PutUint64(sh[0:], uint64(len(dati)))
	binary.LittleEndian.PutUint64(sh[8:], uint64(len(testa)))
	binary.LittleEndian.PutUint32(sh[16:], crc32.ChecksumIEEE(testa))
	binary.Write(&out, binary.LittleEndian, crc32.ChecksumIEEE(sh[:]))
	out.Write(sh[:])
	out.Write(dati)
	out.Write(testa)
	return out.Bytes()
}
