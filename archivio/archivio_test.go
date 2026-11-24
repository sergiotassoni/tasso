// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ulikunitz/xz"
)

// ---------- aiuti ----------

func scrivi(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// albero: tutti i file (percorso relativo con "/" -> contenuto) e le cartelle ("x/" -> "").
func albero(t *testing.T, dir string) map[string]string {
	t.Helper()
	m := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == dir {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if d.Type()&fs.ModeSymlink != 0 {
			t.Errorf("trovato un collegamento simbolico: %s", rel)
			return nil
		}
		if d.IsDir() {
			m[rel+"/"] = ""
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		m[rel] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func nomi(t *testing.T, dir string) []string {
	t.Helper()
	v, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range v {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

// nienteAvanzi: nella cartella non devono restare file o cartelle di lavoro.
func nienteAvanzi(t *testing.T, dir string) {
	t.Helper()
	for _, n := range nomi(t, dir) {
		if strings.HasPrefix(n, ".tassozip-") {
			t.Errorf("è rimasto un file di lavoro: %s", n)
		}
	}
}

func copiaFile(t *testing.T, da, a string) {
	t.Helper()
	b, err := os.ReadFile(da)
	if err != nil {
		t.Fatal(err)
	}
	scrivi(t, a, string(b))
}

func creaZip(t *testing.T, p string, voci [][2]string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, v := range voci {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: v[0], Method: zip.Deflate})
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(w, v[1])
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func esempio(t *testing.T, radice string) string {
	src := filepath.Join(radice, "Progetto")
	scrivi(t, filepath.Join(src, "leggimi.txt"), "Ciao!\n")
	scrivi(t, filepath.Join(src, "documenti", "relazione è già.txt"), strings.Repeat("Tasso Zip comprime bene. ", 2000))
	scrivi(t, filepath.Join(src, "documenti", "foto.jpg"), "finta foto")
	if err := os.MkdirAll(filepath.Join(src, "vuota"), 0o755); err != nil {
		t.Fatal(err)
	}
	return src
}

// ---------- comprimi ed estrai ----------

func TestComprimiEstrai(t *testing.T) {
	for _, f := range []Formato{FormatoZip, FormatoTarXz} {
		t.Run(string(f), func(t *testing.T) {
			dir := t.TempDir()
			src := esempio(t, dir)
			atteso := albero(t, src)

			var ultimi Avanzamento
			ris, err := Comprimi(context.Background(), []string{src}, OpzioniComprimi{
				Formato:     f,
				Avanzamento: func(a Avanzamento) { ultimi = a },
			})
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Base(ris.Archivio) != "Progetto"+f.Estensione() {
				t.Fatalf("nome archivio: %s", ris.Archivio)
			}
			if ris.File != 3 || ris.Compresso <= 0 || ris.Compresso >= ris.Originale {
				t.Fatalf("risultato strano: %+v", ris)
			}
			if ultimi.Totale != ris.Originale || ultimi.Fatto != ultimi.Totale {
				t.Fatalf("avanzamento finale: %+v", ultimi)
			}
			nienteAvanzi(t, dir)

			// Estraggo in un'altra cartella per non confondermi con l'originale.
			altra := filepath.Join(dir, "altra")
			os.Mkdir(altra, 0o755)
			arch := filepath.Join(altra, filepath.Base(ris.Archivio))
			if err := os.Rename(ris.Archivio, arch); err != nil {
				t.Fatal(err)
			}
			re, err := Estrai(context.Background(), arch, OpzioniEstrai{})
			if err != nil {
				t.Fatal(err)
			}
			if re.Cartella != filepath.Join(altra, "Progetto") {
				t.Fatalf("cartella: %s", re.Cartella)
			}
			if got := albero(t, re.Cartella); !uguali(got, atteso) {
				t.Fatalf("contenuto diverso:\n%v\n%v", got, atteso)
			}
			nienteAvanzi(t, altra)
		})
	}
}

func uguali(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestComprimiPiuElementi(t *testing.T) {
	dir := t.TempDir()
	scrivi(t, filepath.Join(dir, "a", "nota.txt"), "A")
	scrivi(t, filepath.Join(dir, "b", "nota.txt"), "B")
	scrivi(t, filepath.Join(dir, "c.txt"), "C")
	ris, err := Comprimi(context.Background(),
		[]string{filepath.Join(dir, "a", "nota.txt"), filepath.Join(dir, "b", "nota.txt"), filepath.Join(dir, "c.txt")},
		OpzioniComprimi{Formato: FormatoZip, NomeMultiplo: "Archivio", Cartella: dir})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(ris.Archivio) != "Archivio.zip" {
		t.Fatal(ris.Archivio)
	}
	re, err := Estrai(context.Background(), ris.Archivio, OpzioniEstrai{})
	if err != nil {
		t.Fatal(err)
	}
	got := albero(t, re.Cartella)
	if got["nota.txt"] != "A" || got["nota (2).txt"] != "B" || got["c.txt"] != "C" {
		t.Fatalf("nomi doppi non gestiti: %v", got)
	}
}

// ---------- mai sovrascrivere ----------

func TestNomiDoppiMaiSovrascrivere(t *testing.T) {
	dir := t.TempDir()
	src := esempio(t, dir)
	// C'è già un "Progetto.zip" che non c'entra niente: non va toccato.
	scrivi(t, filepath.Join(dir, "Progetto.zip"), "non toccarmi")
	r1, err := Comprimi(context.Background(), []string{src}, OpzioniComprimi{Formato: FormatoZip})
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Comprimi(context.Background(), []string{src}, OpzioniComprimi{Formato: FormatoZip})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(r1.Archivio) != "Progetto (2).zip" || filepath.Base(r2.Archivio) != "Progetto (3).zip" {
		t.Fatalf("nomi: %s %s", r1.Archivio, r2.Archivio)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "Progetto.zip")); string(b) != "non toccarmi" {
		t.Fatal("ha sovrascritto un file esistente!")
	}
	// Estraggo due volte accanto alla cartella originale "Progetto": nascono "Progetto (2)" e "(3)".
	e1, err := Estrai(context.Background(), r1.Archivio, OpzioniEstrai{})
	if err != nil {
		t.Fatal(err)
	}
	e2, err := Estrai(context.Background(), r1.Archivio, OpzioniEstrai{})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(e1.Cartella) != "Progetto (2)" || filepath.Base(e2.Cartella) != "Progetto (3)" {
		t.Fatalf("cartelle: %s %s", e1.Cartella, e2.Cartella)
	}
	if b, _ := os.ReadFile(filepath.Join(src, "leggimi.txt")); string(b) != "Ciao!\n" {
		t.Fatal("l'originale è cambiato!")
	}
}

func TestVociDoppieNellArchivio(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "doppi.zip")
	creaZip(t, p, [][2]string{
		{"a.txt", "primo"}, {"a.txt", "secondo"}, {"A.TXT", "terzo"},
		{"x", "sono un file"}, {"x/dentro.txt", "sono in una cartella"},
		{"b.txt", "uno"}, {"c/", ""}, {"c/d.txt", "due"},
	})
	re, err := Estrai(context.Background(), p, OpzioniEstrai{})
	if err != nil {
		t.Fatal(err)
	}
	got := albero(t, re.Cartella)
	if got["a.txt"] != "primo" || got["a (2).txt"] != "secondo" {
		t.Fatalf("doppi: %v", got)
	}
	if got["x"] != "sono un file" || got["x (2)/dentro.txt"] != "sono in una cartella" {
		t.Fatalf("file e cartella con lo stesso nome: %v", got)
	}
	if re.Rinominati < 2 {
		t.Fatalf("rinominati: %d", re.Rinominati)
	}
}

// ---------- sicurezza: percorsi che escono ----------

func TestZipSlip(t *testing.T) {
	dir := t.TempDir()
	sotto := filepath.Join(dir, "sotto")
	os.Mkdir(sotto, 0o755)
	p := filepath.Join(sotto, "cattivo.zip")
	creaZip(t, p, [][2]string{
		{"../fuori.txt", "x"},
		{"..\\fuori2.txt", "x"},
		{"/assoluto.txt", "x"},
		{"\\\\server\\share\\rete.txt", "x"},
		{"C:/Windows/win.txt", "x"},
		{"c:\\win2.txt", "x"},
		{"a/../../fuori3.txt", "x"},
		{"a/./b/.. /fuori4.txt", "x"},
		{"buono/ok.txt", "bene"},
		{"con.txt", "riservato"},
		{"due:punti?.txt", "strano"},
		{"spazi e punti. . .", "fine"},
	})
	re, err := Estrai(context.Background(), p, OpzioniEstrai{})
	if err != nil {
		t.Fatal(err)
	}
	// Fuori dalla cartella "sotto" non deve esserci niente di nuovo.
	if n := nomi(t, dir); len(n) != 1 || n[0] != "sotto" {
		t.Fatalf("qualcosa è uscito: %v", n)
	}
	if n := nomi(t, sotto); len(n) != 2 {
		t.Fatalf("nella cartella dell'archivio: %v", n)
	}
	got := albero(t, re.Cartella)
	for k := range got {
		if strings.Contains(k, "fuori") || strings.Contains(k, "win") || strings.Contains(k, "assoluto") || strings.Contains(k, "rete") {
			t.Errorf("voce pericolosa estratta: %s", k)
		}
	}
	if got["buono/ok.txt"] != "bene" || got["_con.txt"] != "riservato" || got["due_punti_.txt"] != "strano" || got["spazi e punti"] != "fine" {
		t.Fatalf("voci buone: %v", got)
	}
	if len(re.Saltati) != 8 {
		t.Fatalf("saltati: %d %v", len(re.Saltati), re.Saltati)
	}
}

func TestTarCollegamenti(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "link.tar")
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.WriteHeader(&tar.Header{Typeflag: tar.TypeSymlink, Name: "esci", Linkname: "/etc"})
	tw.WriteHeader(&tar.Header{Typeflag: tar.TypeSymlink, Name: "su", Linkname: "../.."})
	tw.WriteHeader(&tar.Header{Typeflag: tar.TypeLink, Name: "duro", Linkname: "/etc/passwd"})
	tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "esci/passwd", Size: 4, Mode: 0o644})
	tw.Write([]byte("ciao"))
	tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "../fuori", Size: 1, Mode: 0o644})
	tw.Write([]byte("x"))
	tw.Close()
	os.WriteFile(p, buf.Bytes(), 0o644)

	re, err := Estrai(context.Background(), p, OpzioniEstrai{})
	if err != nil {
		t.Fatal(err)
	}
	got := albero(t, re.Cartella) // segnala errore se trova collegamenti
	// Dentro c'è una sola cartella ("esci", ora vera cartella e non collegamento): diventa lei il risultato.
	if got["passwd"] != "ciao" || filepath.Base(re.Cartella) != "esci" || len(re.Saltati) != 4 {
		t.Fatalf("got %v saltati %v", got, re.Saltati)
	}
	if n := nomi(t, dir); len(n) != 2 {
		t.Fatalf("qualcosa è uscito: %v", n)
	}
}

func TestFileDiProva(t *testing.T) {
	casi := []struct {
		nome      string
		file      []string
		password  string
		errore    error
		controlla func(t *testing.T, m map[string]string, r RisultatoEstrai)
	}{
		{nome: "zip con collegamenti", file: []string{"symlinks.zip"}, controlla: func(t *testing.T, m map[string]string, r RisultatoEstrai) {
			if len(r.Saltati) == 0 {
				t.Errorf("collegamenti non saltati: %v", m)
			}
		}},
		{nome: "tar con collegamenti fisici", file: []string{"gnu-hardlinks.tar"}, controlla: func(t *testing.T, m map[string]string, r RisultatoEstrai) {
			if len(r.Saltati) == 0 || r.File == 0 {
				t.Errorf("risultato: %+v %v", r, m)
			}
		}},
		{nome: "7z lzma2", file: []string{"lzma2.7z"}},
		{nome: "7z diviso in parti", file: []string{"multi.7z.001", "multi.7z.002", "multi.7z.003", "multi.7z.004", "multi.7z.005", "multi.7z.006"}},
		{nome: "7z cifrato senza password", file: []string{"t2.7z"}, errore: ErrPassword},
		{nome: "7z cifrato password sbagliata", file: []string{"t2.7z"}, password: "sbagliata", errore: ErrPasswordSbagliata},
		{nome: "7z cifrato password giusta", file: []string{"t2.7z"}, password: "password"},
		{nome: "7z solo dati cifrati senza password", file: []string{"t4.7z"}, errore: ErrPassword},
		{nome: "7z solo dati cifrati password sbagliata", file: []string{"t4.7z"}, password: "sbagliata", errore: ErrPasswordSbagliata},
		{nome: "7z solo dati cifrati password giusta", file: []string{"t4.7z"}, password: "password"},
		{nome: "rar", file: []string{"unarr-test.rar"}},
		{nome: "rar diviso in parti", file: []string{"test.part01.rar", "test.part02.rar"}},
		{nome: "tar.bz2", file: []string{"cartella.tar.bz2"}, controlla: func(t *testing.T, m map[string]string, r RisultatoEstrai) {
			if m["uno.txt"] != "uno\n" || m["due.txt"] != "due\n" {
				t.Errorf("%v", m)
			}
			if filepath.Base(r.Cartella) != "cartella" {
				t.Errorf("cartella %s", r.Cartella)
			}
		}},
		{nome: "bz2 singolo", file: []string{"nota.txt.bz2"}, controlla: func(t *testing.T, m map[string]string, r RisultatoEstrai) {
			if m["nota.txt"] != "Ciao da Tasso Zip!\nRiga due.\n" {
				t.Errorf("%v", m)
			}
		}},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range c.file {
				copiaFile(t, filepath.Join("testdata", f), filepath.Join(dir, f))
			}
			r, err := Estrai(context.Background(), filepath.Join(dir, c.file[0]), OpzioniEstrai{Password: c.password})
			if c.errore != nil {
				if !errors.Is(err, c.errore) {
					t.Fatalf("errore atteso %v, avuto %v", c.errore, err)
				}
				nienteAvanzi(t, dir)
				if len(nomi(t, dir)) != len(c.file) {
					t.Fatalf("è rimasto qualcosa: %v", nomi(t, dir))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			m := albero(t, r.Cartella)
			if r.File == 0 {
				t.Fatalf("nessun file estratto: %+v", r)
			}
			if c.controlla != nil {
				c.controlla(t, m, r)
			}
			nienteAvanzi(t, dir)
		})
	}
}

func TestPartiDaMezzo(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"multi.7z.001", "multi.7z.002", "multi.7z.003", "multi.7z.004", "multi.7z.005", "multi.7z.006", "test.part01.rar", "test.part02.rar"} {
		copiaFile(t, filepath.Join("testdata", f), filepath.Join(dir, f))
	}
	// Anche se scegli la terza parte, si parte dalla prima.
	if _, err := Estrai(context.Background(), filepath.Join(dir, "multi.7z.003"), OpzioniEstrai{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Estrai(context.Background(), filepath.Join(dir, "test.part02.rar"), OpzioniEstrai{}); err != nil {
		t.Fatal(err)
	}
}

func TestParteMancante(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"multi.7z.001", "multi.7z.002", "multi.7z.004", "test.part01.rar"} {
		copiaFile(t, filepath.Join("testdata", f), filepath.Join(dir, f))
	}
	_, err := Estrai(context.Background(), filepath.Join(dir, "multi.7z.001"), OpzioniEstrai{})
	if !errors.Is(err, ErrParteMancante) {
		t.Fatalf("7z: %v", err)
	}
	_, err = Estrai(context.Background(), filepath.Join(dir, "test.part01.rar"), OpzioniEstrai{})
	if !errors.Is(err, ErrParteMancante) {
		t.Fatalf("rar: %v", err)
	}
	nienteAvanzi(t, dir)
}

func TestZipDivisoInParti001(t *testing.T) {
	dir := t.TempDir()
	src := esempio(t, dir)
	ris, err := Comprimi(context.Background(), []string{src}, OpzioniComprimi{Formato: FormatoZip})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(ris.Archivio)
	os.Remove(ris.Archivio)
	altra := filepath.Join(dir, "parti")
	os.Mkdir(altra, 0o755)
	pezzo := len(b)/3 + 1
	for i := 0; i*pezzo < len(b); i++ {
		fine := (i + 1) * pezzo
		if fine > len(b) {
			fine = len(b)
		}
		os.WriteFile(filepath.Join(altra, "Progetto.zip.00"+string(rune('1'+i))), b[i*pezzo:fine], 0o644)
	}
	re, err := Estrai(context.Background(), filepath.Join(altra, "Progetto.zip.002"), OpzioniEstrai{})
	if err != nil {
		t.Fatal(err)
	}
	if !uguali(albero(t, re.Cartella), albero(t, src)) {
		t.Fatal("contenuto diverso")
	}
}

func TestGzEXz(t *testing.T) {
	dir := t.TempDir()
	// .gz di un file solo
	var gb bytes.Buffer
	gw := gzip.NewWriter(&gb)
	gw.Write([]byte("contenuto gz"))
	gw.Close()
	os.WriteFile(filepath.Join(dir, "lettera.txt.gz"), gb.Bytes(), 0o644)
	r, err := Estrai(context.Background(), filepath.Join(dir, "lettera.txt.gz"), OpzioniEstrai{})
	if err != nil {
		t.Fatal(err)
	}
	if m := albero(t, r.Cartella); m["lettera.txt"] != "contenuto gz" || filepath.Base(r.Cartella) != "lettera.txt" {
		t.Fatalf("%v %s", m, r.Cartella)
	}
	// .xz di un file solo
	var xb bytes.Buffer
	xw, _ := xz.NewWriter(&xb)
	xw.Write([]byte("contenuto xz"))
	xw.Close()
	os.WriteFile(filepath.Join(dir, "dati.csv.xz"), xb.Bytes(), 0o644)
	r, err = Estrai(context.Background(), filepath.Join(dir, "dati.csv.xz"), OpzioniEstrai{})
	if err != nil {
		t.Fatal(err)
	}
	if m := albero(t, r.Cartella); m["dati.csv"] != "contenuto xz" {
		t.Fatalf("%v", m)
	}
}

func TestZipCifratoEFormatoSconosciuto(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.CreateHeader(&zip.FileHeader{Name: "segreto.txt", Flags: 1})
	w.Write([]byte("xx"))
	zw.Close()
	os.WriteFile(filepath.Join(dir, "cifrato.zip"), buf.Bytes(), 0o644)
	if _, err := Estrai(context.Background(), filepath.Join(dir, "cifrato.zip"), OpzioniEstrai{}); !errors.Is(err, ErrZipCifrato) {
		t.Fatalf("zip cifrato: %v", err)
	}
	scrivi(t, filepath.Join(dir, "finto.zip"), "non sono uno zip, solo testo qualunque")
	if _, err := Estrai(context.Background(), filepath.Join(dir, "finto.zip"), OpzioniEstrai{}); !errors.Is(err, ErrFormato) {
		t.Fatalf("formato: %v", err)
	}
	scrivi(t, filepath.Join(dir, "vecchio.z01"), "x")
	if _, err := Estrai(context.Background(), filepath.Join(dir, "vecchio.z01"), OpzioniEstrai{}); !errors.Is(err, ErrZipDiviso) {
		t.Fatalf("zip diviso: %v", err)
	}
	nienteAvanzi(t, dir)
}

// ---------- sicurezza: zip bomb ----------

func TestZipBomb(t *testing.T) {
	dir := t.TempDir()
	zeri := make([]byte, 20<<20)
	// zip: la dimensione dichiarata è già troppa -> si ferma prima di cominciare
	p := filepath.Join(dir, "bomba.zip")
	creaZip(t, p, [][2]string{{"zeri.bin", string(zeri)}})
	lim := Limiti{MaxRapporto: 10, SogliaRapporto: 1 << 20, MaxVoci: 1000}
	if _, err := Estrai(context.Background(), p, OpzioniEstrai{Limiti: &lim}); !errors.Is(err, ErrBomba) {
		t.Fatalf("zip: %v", err)
	}
	// gz: nessuna dimensione dichiarata -> si ferma mentre scrive
	var gb bytes.Buffer
	gw, _ := gzip.NewWriterLevel(&gb, gzip.BestCompression)
	gw.Write(zeri)
	gw.Close()
	os.WriteFile(filepath.Join(dir, "bomba.bin.gz"), gb.Bytes(), 0o644)
	if _, err := Estrai(context.Background(), filepath.Join(dir, "bomba.bin.gz"), OpzioniEstrai{Limiti: &lim}); !errors.Is(err, ErrBomba) {
		t.Fatalf("gz: %v", err)
	}
	// troppe voci
	var voci [][2]string
	for i := 0; i < 50; i++ {
		voci = append(voci, [2]string{"f" + string(rune('A'+i%26)) + string(rune('a'+i/26)), "x"})
	}
	creaZip(t, filepath.Join(dir, "tante.zip"), voci)
	lim2 := LimitiPredefiniti()
	lim2.MaxVoci = 10
	if _, err := Estrai(context.Background(), filepath.Join(dir, "tante.zip"), OpzioniEstrai{Limiti: &lim2}); !errors.Is(err, ErrTroppeVoci) {
		t.Fatalf("voci: %v", err)
	}
	if n := nomi(t, dir); len(n) != 3 {
		t.Fatalf("è rimasto qualcosa: %v", n)
	}
	// Con i limiti normali lo stesso zip si estrae (20 MB sono pochi).
	if _, err := Estrai(context.Background(), p, OpzioniEstrai{}); err != nil {
		t.Fatal(err)
	}
}

// ---------- annulla ----------

func TestAnnulla(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "Grande")
	casuale := make([]byte, 8<<20)
	rand.Read(casuale)
	scrivi(t, filepath.Join(src, "a.bin"), string(casuale))
	scrivi(t, filepath.Join(src, "b.bin"), string(casuale))

	for _, f := range []Formato{FormatoZip, FormatoTarXz} {
		ctx, annulla := context.WithCancel(context.Background())
		_, err := Comprimi(ctx, []string{src}, OpzioniComprimi{Formato: f, Avanzamento: func(a Avanzamento) {
			if a.Fatto > 1<<20 {
				annulla()
			}
		}})
		annulla()
		if !errors.Is(err, ErrAnnullato) {
			t.Fatalf("%s: atteso annullato, avuto %v", f, err)
		}
		if n := nomi(t, dir); len(n) != 1 {
			t.Fatalf("%s: è rimasto qualcosa: %v", f, n)
		}
	}

	// Estrazione annullata a metà: nessuna cartella nuova.
	ris, err := Comprimi(context.Background(), []string{src}, OpzioniComprimi{Formato: FormatoZip})
	if err != nil {
		t.Fatal(err)
	}
	altra := filepath.Join(dir, "altra")
	os.Mkdir(altra, 0o755)
	arch := filepath.Join(altra, "Grande.zip")
	os.Rename(ris.Archivio, arch)
	ctx, annulla := context.WithCancel(context.Background())
	_, err = Estrai(ctx, arch, OpzioniEstrai{Avanzamento: func(a Avanzamento) {
		if a.Fatto > 9<<20 {
			annulla()
		}
	}})
	annulla()
	if !errors.Is(err, ErrAnnullato) {
		t.Fatalf("estrai: atteso annullato, avuto %v", err)
	}
	if n := nomi(t, altra); len(n) != 1 || n[0] != "Grande.zip" {
		t.Fatalf("è rimasto qualcosa: %v", n)
	}
	if Codice(err) != "annullato" {
		t.Fatal(Codice(err))
	}
}

// ---------- nomi ----------

func TestNomeSicuro(t *testing.T) {
	buoni := map[string]string{
		"a/b/c.txt":     "a/b/c.txt",
		`a\b\c.txt`:     "a/b/c.txt",
		"./a.txt":       "a.txt",
		"a//b.txt":      "a/b.txt",
		"nul.txt":       "_nul.txt",
		"COM1":          "_COM1",
		"fine. ":        "fine",
		"a/b:c":         "a/b_c",
		"file\x01.txt":  "file_.txt",
		"perché è così": "perché è così",
	}
	for in, out := range buoni {
		p, ok := NomeSicuro(in)
		if !ok || strings.Join(p, "/") != out {
			t.Errorf("%q -> %v %v (atteso %q)", in, p, ok, out)
		}
	}
	for _, in := range []string{"../a", "a/../../b", "/etc/passwd", `\\server\x`, "C:/x", "c:x", "..", ". ./", "a/.. /b", "a/... /b", ""} {
		if p, ok := NomeSicuro(in); ok {
			t.Errorf("%q accettato: %v", in, p)
		}
	}
}

func TestNomi(t *testing.T) {
	casi := map[string]string{
		"Foto.zip": "Foto", "Foto.tar.gz": "Foto", "Lavoro.part1.rar": "Lavoro", "Lavoro.part02.rar": "Lavoro",
		"Film.7z.001": "Film", "dati.tgz": "dati", "x.r00": "x", "relazione.txt.gz": "relazione.txt",
	}
	for in, out := range casi {
		if got := NomeBaseArchivio(in); got != out {
			t.Errorf("%s -> %s (atteso %s)", in, got, out)
		}
	}
	if NomeNumero("Foto.tar.xz", false, 2) != "Foto (2).tar.xz" || NomeNumero("Foto", true, 3) != "Foto (3)" || NomeNumero(".bashrc", false, 2) != ".bashrc (2)" {
		t.Error("NomeNumero")
	}
	for _, n := range []string{"a.zip", "b.7z", "c.rar", "d.tar.gz", "e.7z.001", "f.part2.rar", "g.r01"} {
		if !SembraArchivio(n) {
			t.Errorf("%s non riconosciuto", n)
		}
	}
	if SembraArchivio("foto.jpg") {
		t.Error("foto.jpg non è un archivio")
	}
}

// Le date dei file estratti restano quelle originali.
func TestDate(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "d")
	scrivi(t, filepath.Join(src, "vecchio.txt"), "x")
	quando := time.Date(2020, 5, 17, 10, 30, 0, 0, time.UTC)
	os.Chtimes(filepath.Join(src, "vecchio.txt"), quando, quando)
	for _, f := range []Formato{FormatoZip, FormatoTarXz} {
		ris, err := Comprimi(context.Background(), []string{src}, OpzioniComprimi{Formato: f, Cartella: dir})
		if err != nil {
			t.Fatal(err)
		}
		re, err := Estrai(context.Background(), ris.Archivio, OpzioniEstrai{})
		if err != nil {
			t.Fatal(err)
		}
		fi, err := os.Stat(filepath.Join(re.Cartella, "vecchio.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if d := fi.ModTime().Sub(quando); d > 2*time.Second || d < -2*time.Second {
			t.Errorf("%s: data %v invece di %v", f, fi.ModTime(), quando)
		}
	}
}
