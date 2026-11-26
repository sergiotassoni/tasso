package archivio

// Prove per le correzioni del collaudo finale (Fixer D, 26/11/2026).

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"tassozip/archivio/settezip"
)

// zonaFinta: su Linux non ci sono i flussi ":Zone.Identifier": li simulo in memoria.
func zonaFinta(t *testing.T, archivi ...string) *sync.Map {
	t.Helper()
	segnati := &sync.Map{}
	vecchiaL, vecchiaS := leggiZona, scriviZona
	leggiZona = func(p string) []byte {
		for _, a := range archivi {
			if p == a {
				return []byte("[ZoneTransfer]\r\nZoneId=3\r\n")
			}
		}
		return nil
	}
	scriviZona = func(p string, b []byte) { segnati.Store(p, string(b)) }
	t.Cleanup(func() { leggiZona, scriviZona = vecchiaL, vecchiaS })
	return segnati
}

func fileIn(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	_ = filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err == nil && fi.Mode().IsRegular() {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// Marchio del web su TUTTI i file estratti, in ogni modo di estrazione.
func TestMarchioDelWebSuOgniFile(t *testing.T) {
	dir := t.TempDir()
	arch := filepath.Join(dir, "scaricato.zip")
	creaZip(t, arch, [][2]string{{"a.txt", "uno"}, {"cartella/b.js", "due"}, {"cartella/sotto/c.docm", "tre"}})
	segnati := zonaFinta(t, arch)
	for _, modo := range []ModoEstrai{ModoNormale, ModoQui, ModoCartella} {
		dest := filepath.Join(dir, "dest-"+string(modo))
		if err := os.MkdirAll(dest, 0o755); err != nil {
			t.Fatal(err)
		}
		ris, err := Estrai(context.Background(), arch, OpzioniEstrai{Modo: modo, Destinazione: dest})
		if err != nil {
			t.Fatal(err)
		}
		if ris.File != 3 {
			t.Fatalf("%q: %d file", modo, ris.File)
		}
		// il marchio va sui file dove sono stati scritti (nella cartella di lavoro): controllo
		// che ogni nome estratto abbia avuto il marchio
		marchiati := map[string]bool{}
		segnati.Range(func(k, v any) bool {
			if !strings.Contains(v.(string), "ZoneId=3") {
				t.Errorf("marchio sbagliato su %v", k)
			}
			marchiati[filepath.Base(k.(string))] = true
			return true
		})
		for _, f := range fileIn(t, dest) {
			if !marchiati[filepath.Base(f)] {
				t.Errorf("%q: %s senza marchio del web", modo, f)
			}
		}
	}
	// archivio senza marchio (non viene da internet): niente marchio sui file
	locale := filepath.Join(dir, "mio.zip")
	creaZip(t, locale, [][2]string{{"x.txt", "x"}})
	segnati.Range(func(k, _ any) bool { segnati.Delete(k); return true })
	if _, err := Estrai(context.Background(), locale, OpzioniEstrai{Destinazione: dir}); err != nil {
		t.Fatal(err)
	}
	segnati.Range(func(k, _ any) bool { t.Errorf("marchio messo su %v senza motivo", k); return false })
}

// Anche l'archivio riparato si porta dietro il marchio.
func TestMarchioDelWebRiparato(t *testing.T) {
	dir := t.TempDir()
	arch := filepath.Join(dir, "rotto.zip")
	creaZip(t, arch, [][2]string{{"a.txt", strings.Repeat("ciao ", 100)}})
	segnati := zonaFinta(t, arch)
	ris, err := RiparaZip(context.Background(), arch, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := segnati.Load(ris.Archivio); !ok {
		t.Fatalf("archivio riparato %s senza marchio", ris.Archivio)
	}
}

// zip con un file rovinato in mezzo: gli altri escono, quello rovinato è nell'elenco.
func TestZipUnFileRovinato(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	segno := bytes.Repeat([]byte("ROVINAMI"), 200)
	for _, v := range []struct {
		n string
		d []byte
	}{{"a.txt", []byte("primo")}, {"b.bin", segno}, {"c.txt", []byte("terzo")}} {
		w, _ := zw.CreateHeader(&zip.FileHeader{Name: v.n, Method: zip.Store})
		w.Write(v.d)
	}
	zw.Close()
	b := buf.Bytes()
	i := bytes.Index(b, segno)
	b[i+500] ^= 0xFF // CRC di b.bin sbagliato
	arch := filepath.Join(dir, "x.zip")
	os.WriteFile(arch, b, 0o644)
	ris, err := Estrai(context.Background(), arch, OpzioniEstrai{})
	if err != nil {
		t.Fatalf("doveva estrarre i file buoni: %v", err)
	}
	if ris.File != 2 || len(ris.Falliti) != 1 || !strings.HasPrefix(ris.Falliti[0], "b.bin (") {
		t.Fatalf("file %d, falliti %v", ris.File, ris.Falliti)
	}
	got := albero(t, ris.Cartella)
	if got["a.txt"] != "primo" || got["c.txt"] != "terzo" {
		t.Fatalf("estratti: %v", got)
	}
	if _, ok := got["b.bin"]; ok {
		t.Fatal("il file rovinato non deve restare a metà")
	}
}

// Se non esce niente di buono resta l'errore "rovinato" (e niente cartella).
func TestZipTuttoRovinato(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	segno := bytes.Repeat([]byte("SOLOQUESTO"), 100)
	w, _ := zw.CreateHeader(&zip.FileHeader{Name: "unico.bin", Method: zip.Store})
	w.Write(segno)
	zw.Close()
	b := buf.Bytes()
	b[bytes.Index(b, segno)+10] ^= 1
	arch := filepath.Join(dir, "x.zip")
	os.WriteFile(arch, b, 0o644)
	_, err := Estrai(context.Background(), arch, OpzioniEstrai{})
	if Codice(err) != "rovinato" {
		t.Fatalf("errore %v", err)
	}
	voci, _ := os.ReadDir(dir)
	if len(voci) != 1 {
		t.Fatalf("resti nella cartella: %v", voci)
	}
}

// 7z (blocco "copia" per i file già compressi) con un file rovinato in mezzo: i CRC degli altri
// tornano, escono, e quello rovinato è nell'elenco.
func Test7zUnFileRovinato(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "foto")
	r := rand.New(rand.NewSource(7))
	dati := map[string][]byte{}
	for _, n := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		d := make([]byte, 40000)
		r.Read(d)
		dati[n] = d
		os.MkdirAll(src, 0o755)
		os.WriteFile(filepath.Join(src, n), d, 0o644)
	}
	ris, err := Comprimi(context.Background(), []string{src}, OpzioniComprimi{Formato: Formato7z, Cartella: dir})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(ris.Archivio)
	i := bytes.Index(b, dati["b.jpg"][:64])
	if i < 0 {
		t.Skip("i dati non sono in chiaro in questo archivio")
	}
	b[i+20000] ^= 0xFF
	os.WriteFile(ris.Archivio, b, 0o644)
	re, err := Estrai(context.Background(), ris.Archivio, OpzioniEstrai{Destinazione: filepath.Join(dir)})
	if err != nil {
		t.Fatalf("doveva estrarre i file buoni: %v", err)
	}
	if re.File != 2 || len(re.Falliti) != 1 || !strings.Contains(re.Falliti[0], "b.jpg") {
		t.Fatalf("file %d, falliti %v", re.File, re.Falliti)
	}
	for _, n := range []string{"a.jpg", "c.jpg"} {
		got, err := os.ReadFile(filepath.Join(re.Cartella, n))
		if err != nil || !bytes.Equal(got, dati[n]) {
			t.Fatalf("%s non estratto giusto: %v", n, err)
		}
	}
	if _, err := os.Stat(filepath.Join(re.Cartella, "b.jpg")); err == nil {
		t.Fatal("b.jpg rovinato non deve restare")
	}
	// la Verifica invece dice "rovinato"
	if _, err := Verifica(context.Background(), ris.Archivio, OpzioniVerifica{}); Codice(err) != "rovinato" {
		t.Fatalf("verifica: %v", err)
	}
}

// 7z con un nome non sicuro (../evil.txt): saltato, e il file dopo esce giusto (prima: tutto
// fallito con "dati non corretti", perché i dati del file saltato non venivano letti).
func Test7zNomeNonSicuroNonBloccaGliAltri(t *testing.T) {
	dir := t.TempDir()
	arch := filepath.Join(dir, "x.7z")
	f, _ := os.Create(arch)
	voci := []settezip.Voce{
		{Nome: "../evil.txt", Dim: 7, Apri: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("cattivo")), nil }},
		{Nome: "ok.txt", Dim: 5, Apri: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("bravo")), nil }},
	}
	if _, err := settezip.Scrivi(context.Background(), f, voci, settezip.Opzioni{Livello: settezip.Veloce, Thread: 1}); err != nil {
		t.Fatal(err)
	}
	f.Close()
	ris, err := Estrai(context.Background(), arch, OpzioniEstrai{})
	if err != nil {
		t.Fatalf("errore: %v", err)
	}
	got := albero(t, ris.Cartella)
	if got["ok.txt"] != "bravo" || len(ris.Saltati) != 1 || !strings.Contains(ris.Saltati[0], "percorso_non_sicuro") {
		t.Fatalf("estratti %v, saltati %v", got, ris.Saltati)
	}
	if _, err := os.Stat(filepath.Join(dir, "evil.txt")); err == nil {
		t.Fatal("uscito dalla cartella")
	}
}

// Un 7z di pochi byte che dichiara un dizionario da 3 GB non fa più riservare 3 GB.
func TestDizionarioEnormeDichiarato(t *testing.T) {
	// il dizionario si misura sui dati: 3 GB dichiarati per 100 byte di dati = 4 KB
	if d := dizionarioServe(3<<30, 100); d != 4096 {
		t.Fatalf("dizionario %d", d)
	}
	if d := dizionarioServe(64<<20, 1<<40); d != 64<<20 {
		t.Fatalf("dizionario %d", d)
	}
	// dati grandi davvero e poca memoria libera: errore chiaro, prima di riservare
	memoriaPerProve = 1 << 30
	defer func() { memoriaPerProve = 0 }()
	_, err := nuovoLZMA2([]byte{39}, 3<<30, []io.ReadCloser{io.NopCloser(bytes.NewReader(nil))})
	if !errors.Is(err, ErrMemoria) || Codice(err) != "memoria" {
		t.Fatalf("errore %v", err)
	}
	p := []byte{0x5D, 0, 0, 0, 0xC0} // LZMA, dizionario 3 GB
	if _, err := nuovoLZMA(p, 3<<30, []io.ReadCloser{io.NopCloser(bytes.NewReader(nil))}); !errors.Is(err, ErrMemoria) {
		t.Fatalf("lzma: %v", err)
	}
	// fino a 256 MB va sempre bene
	if err := controllaDizionario(200 << 20); err != nil {
		t.Fatal(err)
	}
}

// Il 7z vero con l'intestazione che dichiara 3 GB si apre e si estrae (dizionario ridotto).
func TestDizionarioRidottoSiEstrae(t *testing.T) {
	dir := t.TempDir()
	arch := filepath.Join(dir, "x.7z")
	f, _ := os.Create(arch)
	testo := strings.Repeat("ciao mondo ", 1000)
	_, err := settezip.Scrivi(context.Background(), f, []settezip.Voce{{Nome: "a.txt", Dim: int64(len(testo)),
		Apri: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(testo)), nil }}},
		settezip.Opzioni{Livello: settezip.Veloce, Thread: 1})
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(arch)
	// la proprietà LZMA2 (0x21, 1 byte) nell'intestazione non compressa diventa 39 = 3 GB
	n := 0
	off := 32 + int(binary.LittleEndian.Uint64(b[12:20]))
	size := int(binary.LittleEndian.Uint64(b[20:28]))
	for i := off; i+3 < off+size; i++ {
		if b[i] == 0x21 && b[i+1] == 0x21 && b[i+2] == 0x01 {
			b[i+3] = 39
			n++
		}
	}
	if n == 0 {
		t.Skip("proprietà non trovate (intestazione compressa)")
	}
	// rifaccio le somme di controllo dell'intestazione (come un archivio fatto apposta)
	binary.LittleEndian.PutUint32(b[28:], crc32.ChecksumIEEE(b[off:off+size]))
	binary.LittleEndian.PutUint32(b[8:], crc32.ChecksumIEEE(b[12:32]))
	os.WriteFile(arch, b, 0o644)
	memoriaPerProve = 512 << 20 // con 3 GB veri l'errore sarebbe ErrMemoria
	defer func() { memoriaPerProve = 0 }()
	ris, err := Estrai(context.Background(), arch, OpzioniEstrai{})
	if err != nil {
		t.Fatalf("errore: %v", err)
	}
	if albero(t, ris.Cartella)["a.txt"] != testo {
		t.Fatal("contenuto")
	}
}

// Avanzi di lavori interrotti: via quelli vecchi, restano quelli nuovi e gli altri file.
func TestPulisciAvanzi(t *testing.T) {
	dir := t.TempDir()
	vecchio := time.Now().Add(-48 * time.Hour)
	mk := func(nome string, cartella, vecchia bool) string {
		p := filepath.Join(dir, nome)
		if cartella {
			os.MkdirAll(filepath.Join(p, "dentro"), 0o755)
			os.WriteFile(filepath.Join(p, "dentro", "f"), []byte("x"), 0o644)
		} else {
			os.WriteFile(p, []byte("x"), 0o644)
		}
		if vecchia {
			os.Chtimes(p, vecchio, vecchio)
		}
		return p
	}
	a := mk(".tassozip-aaaaaaaaaaaa.tmp", true, true)
	b := mk(".tassozip-bbbbbbbbbbbb.tmp", false, true)
	c := mk(".tassozip-cccccccccccc.tmp", true, false) // lavoro in corso (recente)
	d := mk("mio.tmp", false, true)
	e := mk(".tassozip-nota.txt", false, true)
	if n := PulisciAvanzi(dir); n != 2 {
		t.Fatalf("tolti %d", n)
	}
	for _, p := range []string{a, b} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s doveva sparire", p)
		}
	}
	for _, p := range []string{c, d, e} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s doveva restare", p)
		}
	}
}

// "Estrai qui": se lo spostamento finale si ferma a metà, niente resta nella cartella.
func TestEstraiQuiTuttoONiente(t *testing.T) {
	dir := t.TempDir()
	arch := filepath.Join(dir, "x.zip")
	creaZip(t, arch, [][2]string{{"uno.txt", "1"}, {"due.txt", "2"}, {"tre.txt", "3"}})
	dest := filepath.Join(dir, "dest")
	os.MkdirAll(dest, 0o755)
	vecchio := spostaQui
	n := 0
	spostaQui = func(da, dove, nome string, cartella bool) (string, error) {
		n++
		if n == 3 {
			return "", errors.New("disco staccato")
		}
		return vecchio(da, dove, nome, cartella)
	}
	defer func() { spostaQui = vecchio }()
	if _, err := Estrai(context.Background(), arch, OpzioniEstrai{Modo: ModoQui, Destinazione: dest}); err == nil {
		t.Fatal("doveva fallire")
	}
	voci, _ := os.ReadDir(dest)
	if len(voci) != 0 {
		t.Fatalf("restati a metà: %v", voci)
	}
}

// 7z in parallelo: sul disco quasi pieno un core solo (scrive direttamente nell'archivio).
func TestThreadPerSpazio(t *testing.T) {
	if threadPerSpazio(1<<30, 10<<30) != 1 {
		t.Fatal("poco spazio: un core")
	}
	if threadPerSpazio(100<<30, 10<<30) != 0 || threadPerSpazio(-1, 10<<30) != 0 {
		t.Fatal("spazio abbondante o sconosciuto: decide settezip")
	}
}

// I motivi sono codici (li traduce la finestra).
func TestMotiviSonoCodici(t *testing.T) {
	dir := t.TempDir()
	arch := filepath.Join(dir, "x.zip")
	creaZip(t, arch, [][2]string{{"../fuori.txt", "x"}, {"ok.txt", "y"}})
	ris, err := Estrai(context.Background(), arch, OpzioniEstrai{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ris.Saltati) != 1 || !strings.HasSuffix(ris.Saltati[0], "(percorso_non_sicuro)") {
		t.Fatalf("saltati %v", ris.Saltati)
	}
}
