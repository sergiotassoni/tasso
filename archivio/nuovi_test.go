// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// esempioGrande: come esempio, più un file grande e ripetitivo (bzip2: più blocchi da 900 KB)
// e uno casuale (non si comprime).
func esempioGrande(t *testing.T, radice string) string {
	src := esempio(t, radice)
	var b strings.Builder
	for i := 0; b.Len() < 3<<20; i++ {
		fmt.Fprintf(&b, "riga %d: Tasso Zip comprime ed estrae i tuoi file %x\n", i, i*7919)
	}
	scrivi(t, filepath.Join(src, "grande.txt"), b.String())
	cas := make([]byte, 300<<10)
	rand.Read(cas)
	scrivi(t, filepath.Join(src, "documenti", "casuale.bin"), string(cas))
	scrivi(t, filepath.Join(src, "vuoto.txt"), "")
	return src
}

// leggiTarConStdlib: rilegge un tar (anche compresso) con le librerie standard (non con il
// nostro codice di estrazione) e restituisce nome -> contenuto.
func leggiTarConStdlib(t *testing.T, p string, f Formato) map[string]string {
	t.Helper()
	fh, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	var r io.Reader = fh
	switch f {
	case FormatoTarGz:
		g, err := gzip.NewReader(fh)
		if err != nil {
			t.Fatal(err)
		}
		r = g
	case FormatoTarBz2:
		r = bzip2.NewReader(fh) // compress/bzip2 della libreria standard di Go
	case FormatoTarXz:
		x, err := xz.NewReader(fh)
		if err != nil {
			t.Fatal(err)
		}
		r = x
	case FormatoTarZst:
		z, err := zstd.NewReader(fh)
		if err != nil {
			t.Fatal(err)
		}
		defer z.Close()
		r = z
	}
	m := map[string]string{}
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeDir {
			m[strings.TrimSuffix(h.Name, "/")+"/"] = ""
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		m[h.Name] = string(b)
	}
	// e il flusso compresso deve finire bene (somme di controllo finali)
	if _, err := io.Copy(io.Discard, r); err != nil {
		t.Fatal(err)
	}
	return m
}

func leggiZipConStdlib(t *testing.T, p string) map[string]string {
	t.Helper()
	zr, err := zip.OpenReader(p)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	m := map[string]string{}
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "/") {
			m[f.Name] = ""
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc) // controlla anche il CRC32
		rc.Close()
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		m[f.Name] = string(b)
	}
	return m
}

// Ogni formato creato si rilegge byte per byte con le librerie standard e, se c'è, con il
// "tar" del sistema (GNU tar con gzip, bzip2, xz).
func TestFormatiCreatiRilettiBytePerByte(t *testing.T) {
	dir := t.TempDir()
	src := esempioGrande(t, dir)
	atteso := map[string]string{}
	for k, v := range albero(t, src) {
		atteso["Progetto/"+k] = v
	}
	atteso["Progetto/"] = ""
	for _, f := range FormatiCreabili {
		for _, liv := range []Livello{LivelloVeloce, LivelloMassima} {
			if (f == FormatoZip || f == FormatoTar) && liv != LivelloMassima { // il livello non conta (LIVELLO_FORMATI)
				continue
			}
			t.Run(string(f)+"-"+string(liv), func(t *testing.T) {
				out := t.TempDir()
				ris, err := Comprimi(context.Background(), []string{src}, OpzioniComprimi{Formato: f, Livello: liv, Cartella: out})
				if err != nil {
					t.Fatal(err)
				}
				if !strings.HasSuffix(ris.Archivio, f.Estensione()) {
					t.Fatalf("estensione: %s", ris.Archivio)
				}
				var got map[string]string
				switch f {
				case FormatoZip:
					got = leggiZipConStdlib(t, ris.Archivio)
				case Formato7z:
					// il 7z si rilegge con bodgit/sevenzip: lo fa già il nostro Estrai
					re, err := Estrai(context.Background(), ris.Archivio, OpzioniEstrai{Destinazione: out, Modo: ModoCartella})
					if err != nil {
						t.Fatal(err)
					}
					got = map[string]string{"Progetto/": ""}
					for k, v := range albero(t, re.Cartella) {
						got["Progetto/"+k] = v
					}
				default:
					got = leggiTarConStdlib(t, ris.Archivio, f)
				}
				if !uguali(got, atteso) {
					t.Fatalf("contenuto diverso (%d voci, attese %d)", len(got), len(atteso))
				}
				// Verifica dice che è tutto a posto, con i numeri giusti.
				v, err := Verifica(context.Background(), ris.Archivio, OpzioniVerifica{})
				if err != nil {
					t.Fatal(err)
				}
				if v.File != 6 || v.Byte != ris.Originale {
					t.Fatalf("verifica: %+v (originale %d)", v, ris.Originale)
				}
				tarSistema(t, f, ris.Archivio, atteso)
			})
		}
	}
}

// tarSistema: se nel sistema c'è GNU tar (con il programma di compressione), estraggo con lui.
func tarSistema(t *testing.T, f Formato, arch string, atteso map[string]string) {
	opz := map[Formato]string{FormatoTar: "", FormatoTarGz: "z", FormatoTarBz2: "j", FormatoTarXz: "J"}
	o, ok := opz[f]
	if !ok {
		return
	}
	prog := map[string]string{"z": "gzip", "j": "bzip2", "J": "xz"}[o]
	if _, err := exec.LookPath("tar"); err != nil {
		return
	}
	if prog != "" {
		if _, err := exec.LookPath(prog); err != nil {
			return
		}
	}
	dest := t.TempDir()
	if b, err := exec.Command("tar", "-x"+o+"f", arch, "-C", dest).CombinedOutput(); err != nil {
		t.Fatalf("tar del sistema: %v\n%s", err, b)
	}
	got := map[string]string{}
	for k, v := range albero(t, dest) {
		got[k] = v
	}
	if !uguali(got, atteso) {
		t.Fatalf("tar del sistema: contenuto diverso")
	}
}

func TestFormatoValido(t *testing.T) {
	for _, f := range []string{"zip", "7z", "tar", "targz", "tarxz", "tarbz2", "tarzst"} {
		if !FormatoValido(f) {
			t.Errorf("%s non valido", f)
		}
	}
	for _, f := range []string{"rar", "", "exe"} {
		if FormatoValido(f) {
			t.Errorf("%s valido", f)
		}
	}
}

// ---------- Verifica ----------

func TestVerificaArchiviDiProva(t *testing.T) {
	casi := []struct {
		nome, password string
		file           int
	}{
		{"lzma2.7z", "", -1}, {"t2.7z", "password", -1}, {"multi.7z.001", "", -1},
		{"test.part01.rar", "", -1}, {"unarr-test.rar", "", -1}, {"cartella.tar.bz2", "", -1}, {"nota.txt.bz2", "", 1},
	}
	for _, c := range casi {
		r, err := Verifica(context.Background(), filepath.Join("testdata", c.nome), OpzioniVerifica{Password: c.password})
		if err != nil {
			t.Errorf("%s: %v", c.nome, err)
			continue
		}
		if r.File == 0 || (c.file > 0 && r.File != c.file) {
			t.Errorf("%s: %+v", c.nome, r)
		}
	}
	// senza password: la chiede
	if _, err := Verifica(context.Background(), filepath.Join("testdata", "t2.7z"), OpzioniVerifica{}); Codice(err) != "password" {
		t.Errorf("t2.7z senza password: %v", err)
	}
	if _, err := Verifica(context.Background(), filepath.Join("testdata", "t2.7z"), OpzioniVerifica{Password: "no"}); Codice(err) != "password_sbagliata" {
		t.Errorf("t2.7z password sbagliata: %v", err)
	}
}

// Verifica non scrive niente sul disco.
func TestVerificaNonScrive(t *testing.T) {
	altra, arch := creaEsempio(t, FormatoZip)
	prima := nomi(t, altra)
	if _, err := Verifica(context.Background(), arch, OpzioniVerifica{}); err != nil {
		t.Fatal(err)
	}
	if dopo := nomi(t, altra); strings.Join(prima, "|") != strings.Join(dopo, "|") {
		t.Fatalf("Verifica ha scritto: %v", dopo)
	}
}

// Un byte cambiato nei dati: Verifica se ne accorge (zip, 7z, tar.gz, tar.bz2, tar.xz, tar.zst).
func TestVerificaTrovaDanni(t *testing.T) {
	for _, f := range []Formato{FormatoZip, Formato7z, FormatoTarGz, FormatoTarBz2, FormatoTarXz, FormatoTarZst} {
		t.Run(string(f), func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "Dati")
			var b strings.Builder
			for i := 0; b.Len() < 400<<10; i++ {
				fmt.Fprintf(&b, "%d Tasso %x\n", i, i*31)
			}
			scrivi(t, filepath.Join(src, "a.txt"), b.String())
			ris, err := Comprimi(context.Background(), []string{src}, OpzioniComprimi{Formato: f})
			if err != nil {
				t.Fatal(err)
			}
			dati, _ := os.ReadFile(ris.Archivio)
			// cambio un byte nel mezzo (nei dati compressi, non nelle intestazioni)
			i := len(dati) / 2
			dati[i] ^= 0x55
			os.WriteFile(ris.Archivio, dati, 0o644)
			_, err = Verifica(context.Background(), ris.Archivio, OpzioniVerifica{})
			if err == nil {
				t.Fatal("danno non trovato")
			}
			if c := Codice(err); c != "rovinato" && c != "formato" && c != "generico" {
				t.Fatalf("codice %s (%v)", c, err)
			}
		})
	}
}

func TestVerificaAnnulla(t *testing.T) {
	_, arch := creaEsempio(t, FormatoZip)
	ctx, stop := context.WithCancel(context.Background())
	stop()
	if _, err := Verifica(ctx, arch, OpzioniVerifica{}); !errors.Is(err, ErrAnnullato) {
		t.Fatalf("atteso annullato: %v", err)
	}
}

// ---------- Info (metodo) ----------

func TestElencaMetodo(t *testing.T) {
	casi := map[Formato]string{FormatoZip: "Store, Deflate", Formato7z: "7z", FormatoTarGz: "gzip", FormatoTarBz2: "bzip2", FormatoTar: "tar"}
	for f, m := range casi {
		_, arch := creaEsempio(t, f)
		el, err := Elenca(context.Background(), arch, "")
		if err != nil {
			t.Fatal(err)
		}
		if el.Metodo != m {
			t.Errorf("%s: metodo %q (atteso %q)", f, el.Metodo, m)
		}
	}
	el, err := Elenca(context.Background(), filepath.Join("testdata", "lzma2.7z"), "")
	if err != nil {
		t.Fatal(err)
	}
	_ = el.Solido // dipende dal file: basta che non dia errore
}

// ---------- Ripara (zip) ----------

// zipDiProva: uno zip fatto come fanno quasi tutti i programmi (Deflate con descrittore dopo
// i dati, Store, una cartella, nomi UTF-8, uno zip dentro lo zip), più uno con le dimensioni
// nell'intestazione (CreateRaw).
func zipDiProva(t *testing.T, p string) map[string]string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	atteso := map[string]string{}
	agg := func(nome, dati string, metodo uint16) {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: nome, Method: metodo})
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(w, dati)
		atteso[nome] = dati
	}
	agg("cartella/", "", zip.Store)
	agg("cartella/relazione è già.txt", strings.Repeat("Tasso Zip ripara. ", 5000), zip.Deflate)
	agg("foto.jpg", "finta foto PK\x03\x04 con una firma falsa dentro PK\x07\x08", zip.Store)
	// uno zip (memorizzato) dentro lo zip: le sue intestazioni non devono confondere
	var dentro bytes.Buffer
	z2 := zip.NewWriter(&dentro)
	w2, _ := z2.Create("interno.txt")
	io.WriteString(w2, "sono dentro")
	z2.Close()
	agg("altro.zip", dentro.String(), zip.Store)
	agg("vuoto.txt", "", zip.Deflate)
	cas := make([]byte, 70000)
	rand.Read(cas)
	agg("casuale.bin", string(cas), zip.Deflate)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return atteso
}

func TestRiparaZipSenzaIndice(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "Rotto.zip")
	atteso := zipDiProva(t, p)
	dati, _ := os.ReadFile(p)
	// Tolgo la directory centrale (come un download interrotto alla fine).
	i := inizioIndice(dati)
	os.WriteFile(p, dati[:i+10], 0o644)
	if _, err := zip.OpenReader(p); err == nil {
		t.Fatal("lo zip doveva essere rotto")
	}
	ris, err := RiparaZip(context.Background(), p, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(ris.Archivio) != "Rotto (riparato).zip" {
		t.Fatalf("nome: %s", ris.Archivio)
	}
	if ris.Salvati != len(atteso) || len(ris.Persi) != 0 {
		t.Fatalf("salvati %d (attesi %d), persi %v", ris.Salvati, len(atteso), ris.Persi)
	}
	got := leggiZipConStdlib(t, ris.Archivio)
	if !uguali(got, atteso) {
		t.Fatalf("contenuto diverso:\n%v", got)
	}
	// Anche Tasso Zip lo apre e lo verifica.
	if _, err := Verifica(context.Background(), ris.Archivio, OpzioniVerifica{}); err != nil {
		t.Fatal(err)
	}
	// L'originale non è cambiato e una seconda riparazione non sovrascrive.
	if b, _ := os.ReadFile(p); len(b) != i+10 {
		t.Fatal("originale toccato")
	}
	r2, err := RiparaZip(context.Background(), p, "", nil)
	if err != nil || filepath.Base(r2.Archivio) != "Rotto (riparato) (2).zip" {
		t.Fatalf("seconda: %v %v", r2.Archivio, err)
	}
	nienteAvanzi(t, dir)
}

func TestRiparaZipConDatiRovinati(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "Danni.zip")
	atteso := zipDiProva(t, p)
	dati, _ := os.ReadFile(p)
	// rovino i dati di "casuale.bin" (dopo la sua intestazione)
	k := bytes.Index(dati, []byte("casuale.bin"))
	dati[k+11+100] ^= 0xFF
	os.WriteFile(p, dati, 0o644)
	if _, err := Verifica(context.Background(), p, OpzioniVerifica{}); err == nil {
		t.Fatal("Verifica non trova il danno")
	}
	ris, err := RiparaZip(context.Background(), p, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ris.Persi) != 1 || !strings.HasPrefix(ris.Persi[0], "casuale.bin") {
		t.Fatalf("persi: %v", ris.Persi)
	}
	delete(atteso, "casuale.bin")
	if got := leggiZipConStdlib(t, ris.Archivio); !uguali(got, atteso) {
		t.Fatalf("contenuto diverso: %v", got)
	}
}

// Zip con le dimensioni nell'intestazione (senza descrittore) e nomi non UTF-8.
func TestRiparaZipDimensioniNellIntestazione(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "Vecchio.zip")
	ris, err := Comprimi(context.Background(), []string{esempio(t, dir)}, OpzioniComprimi{Formato: FormatoZip})
	if err != nil {
		t.Fatal(err)
	}
	// Riscrivo con CreateRaw (dimensioni nell'intestazione locale).
	zr, _ := zip.OpenReader(ris.Archivio)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	atteso := map[string]string{}
	for _, f := range zr.File {
		fh := f.FileHeader
		fh.Flags &^= 8
		w, err := zw.CreateRaw(&fh)
		if err != nil {
			t.Fatal(err)
		}
		rr, _ := f.OpenRaw()
		io.Copy(w, rr)
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		atteso[f.Name] = string(b)
	}
	zr.Close()
	zw.Close()
	dati := buf.Bytes()
	i := inizioIndice(dati)
	os.WriteFile(p, dati[:i], 0o644)
	r, err := RiparaZip(context.Background(), p, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := leggiZipConStdlib(t, r.Archivio); !uguali(got, atteso) {
		t.Fatalf("contenuto diverso: %v", got)
	}
}

func TestRiparaNonZip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nota.txt")
	scrivi(t, p, "niente zip qui")
	if _, err := RiparaZip(context.Background(), p, "", nil); err == nil {
		t.Fatal("doveva fallire")
	}
	p = filepath.Join(dir, "vuoto.zip")
	scrivi(t, p, "PK\x03\x04 rotto")
	if _, err := RiparaZip(context.Background(), p, "", nil); Codice(err) != "rovinato" {
		t.Fatalf("atteso rovinato: %v", err)
	}
	nienteAvanzi(t, dir)
}

// inizioIndice: dove comincia la directory centrale (dalla fine dello zip, record EOCD).
func inizioIndice(dati []byte) int {
	e := bytes.LastIndex(dati, []byte("PK\x05\x06"))
	return int(uint32(dati[e+16]) | uint32(dati[e+17])<<8 | uint32(dati[e+18])<<16 | uint32(dati[e+19])<<24)
}
