// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package settezip

import (
	"bytes"
	"context"
	"fmt"
	"hash/crc32"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bodgit/sevenzip"
)

type caso struct {
	nome     string
	cartella bool
	dati     []byte
}

func generaTesto(n int, seme int64) []byte {
	r := rand.New(rand.NewSource(seme))
	parole := []string{"Tasso", "zip", "comprime", "file", "cartella", "ciao", "à", "è", "🦡", "\n"}
	var b bytes.Buffer
	for b.Len() < n {
		b.WriteString(parole[r.Intn(len(parole))])
		b.WriteByte(' ')
	}
	return b.Bytes()[:n]
}

func casuale(n int, seme int64) []byte {
	b := make([]byte, n)
	rand.New(rand.NewSource(seme)).Read(b)
	return b
}

func scrivi(t *testing.T, casi []caso, o Opzioni) (string, Risultato) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "prova.7z")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	quando := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	var voci []Voce
	for _, c := range casi {
		c := c
		voci = append(voci, Voce{Nome: c.nome, Cartella: c.cartella, Dim: int64(len(c.dati)), Quando: quando, GiaCompresso: strings.HasSuffix(c.nome, ".jpg"),
			Apri: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(c.dati)), nil }})
	}
	o.DirTemp = filepath.Dir(p)
	ris, err := Scrivi(context.Background(), f, voci, o)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	return p, ris
}

func verifica(t *testing.T, p string, casi []caso) {
	t.Helper()
	r, err := sevenzip.OpenReader(p)
	if err != nil {
		t.Fatalf("bodgit non apre: %v", err)
	}
	defer r.Close()
	attesi := map[string]caso{}
	for _, c := range casi {
		attesi[c.nome] = c
	}
	if len(r.File) != len(casi) {
		t.Fatalf("file: %d invece di %d", len(r.File), len(casi))
	}
	for _, f := range r.File {
		c, ok := attesi[strings.TrimSuffix(f.Name, "/")]
		if !ok {
			t.Fatalf("nome inatteso %q", f.Name)
		}
		if c.cartella != f.FileInfo().IsDir() {
			t.Fatalf("%s: cartella sbagliata", f.Name)
		}
		if !f.Modified.Equal(time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)) {
			t.Fatalf("%s: data %v", f.Name, f.Modified)
		}
		if c.cartella {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		if !bytes.Equal(got, c.dati) {
			t.Fatalf("%s: contenuto diverso (%d vs %d byte)", f.Name, len(got), len(c.dati))
		}
	}
	// 7z vero, se c'è.
	for _, prog := range []string{"7zz", "7z", "7za"} {
		if _, err := lookPath(prog); err == nil {
			if out, err := comando(prog, "t", p); err != nil {
				t.Fatalf("%s t: %v\n%s", prog, err, out)
			}
			break
		}
	}
}

func insiemeMisto() []caso {
	return []caso{
		{nome: "Documenti", cartella: true},
		{nome: "Documenti/lettera è più.txt", dati: generaTesto(300000, 1)},
		{nome: "Documenti/vuoto.txt"},
		{nome: "Documenti/Sotto/Più/Giù", cartella: true},
		{nome: "Documenti/Sotto", cartella: true},
		{nome: "Documenti/Sotto/Più", cartella: true},
		{nome: "Documenti/Sotto/Più/Giù/emoji 🦡😀.bin", dati: casuale(70000, 2)},
		{nome: "Documenti/Sotto/uno.txt", dati: []byte("x")},
		{nome: "Documenti/Cartella vuota", cartella: true},
		{nome: "日本語/ファイル.txt", dati: generaTesto(5000, 3)},
		{nome: "日本語", cartella: true},
		{nome: "altro vuoto"},
		{nome: "Foto/mare.jpg", dati: casuale(3*65536+123, 4)},
		{nome: "Foto/piccola.jpg", dati: casuale(10, 5)},
		{nome: "Foto/esatta.jpg", dati: casuale(65536, 6)},
	}
}

func TestMistoLivelli(t *testing.T) {
	for _, liv := range []Livello{Veloce, Normale, Massima, Ultra} {
		for _, blocco := range []int64{0, -1, 100000} {
			for _, th := range []int{1, 4} {
				t.Run(fmt.Sprintf("l%d_b%d_t%d", liv, blocco, th), func(t *testing.T) {
					casi := insiemeMisto()
					p, ris := scrivi(t, casi, Opzioni{Livello: liv, Blocco: blocco, Thread: th})
					if ris.File != 9 || ris.Cartelle != 6 {
						t.Fatalf("conteggi %+v", ris)
					}
					verifica(t, p, casi)
				})
			}
		}
	}
}

func TestSoloVuoti(t *testing.T) {
	casi := []caso{{nome: "a", cartella: true}, {nome: "a/b"}, {nome: "c"}}
	p, _ := scrivi(t, casi, Opzioni{Livello: Normale})
	verifica(t, p, casi)
	casi = []caso{{nome: "solo", cartella: true}}
	p, _ = scrivi(t, casi, Opzioni{Livello: Normale})
	verifica(t, p, casi)
}

func TestTantiFile(t *testing.T) {
	var casi []caso
	for i := 0; i < 3000; i++ {
		casi = append(casi, caso{nome: fmt.Sprintf("d%d/f%04d.txt", i%17, i), dati: generaTesto(1+i*7%2000, int64(i))})
	}
	for i := 0; i < 17; i++ {
		casi = append(casi, caso{nome: fmt.Sprintf("d%d", i), cartella: true})
	}
	p, ris := scrivi(t, casi, Opzioni{Livello: Massima, Thread: 3})
	t.Logf("%+v", ris)
	verifica(t, p, casi)
}

func TestGrande(t *testing.T) {
	if testing.Short() || os.Getenv("TASSOZIP_GRANDE") == "" {
		t.Skip("TASSOZIP_GRANDE=1 per il file da 200 MB")
	}
	dati := make([]byte, 0, 200<<20)
	for i := 0; len(dati) < 200<<20; i++ {
		if i%3 == 0 {
			dati = append(dati, casuale(1<<20, int64(i))...)
		} else {
			dati = append(dati, generaTesto(1<<20, int64(i))...)
		}
	}
	casi := []caso{{nome: "grande.bin", dati: dati}, {nome: "piccolo.txt", dati: []byte(strings.Repeat("ciao ", 1000))}}
	inizio := time.Now()
	p, ris := scrivi(t, casi, Opzioni{Livello: Massima})
	fi, _ := os.Stat(p)
	t.Logf("200 MB -> %d byte in %v (%+v)", fi.Size(), time.Since(inizio), ris)
	verifica(t, p, casi)
}

func TestNumeri(t *testing.T) {
	for _, v := range []uint64{0, 1, 127, 128, 255, 16383, 16384, 1 << 20, 1<<56 - 1, 1 << 56, 1<<64 - 1} {
		var w bufWriter
		w.num(v)
		got, n := leggiNum(w.b)
		if got != v || n != len(w.b) {
			t.Fatalf("%d: %d (%d byte)", v, got, n)
		}
	}
	if codiceDizionario(64<<20) != 28 || codiceDizionario(1<<20) != 16 || codiceDizionario(3<<20) != 19 {
		t.Fatal("codice dizionario")
	}
}

func leggiNum(b []byte) (uint64, int) {
	first := b[0]
	mask := byte(0x80)
	var v uint64
	for i := 0; i < 8; i++ {
		if first&mask == 0 {
			high := uint64(first & (mask - 1))
			v |= high << (8 * uint(i))
			return v, i + 1
		}
		v |= uint64(b[1+i]) << (8 * uint(i))
		mask >>= 1
	}
	return v, 9
}

func TestCrc32Combina(t *testing.T) {
	for _, n := range []int{0, 1, 7, 100, 4096, 1 << 20} {
		a, b := casuale(n/2+3, int64(n)), casuale(n, int64(n+1))
		atteso := crc32.ChecksumIEEE(append(append([]byte{}, a...), b...))
		if got := crc32Combina(crc32.ChecksumIEEE(a), crc32.ChecksumIEEE(b), int64(len(b))); got != atteso {
			t.Fatalf("n=%d: %08x invece di %08x", n, got, atteso)
		}
	}
}

// Segmenti piccoli: i file vengono divisi fra più segmenti compressi in parallelo
// (a cavallo dei file), e il flusso LZMA2 unito deve restare valido.
func TestSegmentiParalleli(t *testing.T) {
	casi := insiemeMisto()
	casi = append(casi, caso{nome: "grande testo.txt", dati: generaTesto(3<<20, 9)}, caso{nome: "grande casuale.bin", dati: casuale(1<<20+17, 10)})
	for _, seg := range []int64{1 << 16, 100003, 1 << 20} {
		for _, th := range []int{2, 4} {
			for _, blocco := range []int64{0, -1} {
				p, ris := scrivi(t, casi, Opzioni{Livello: Normale, Segmento: seg, Thread: th, Blocco: blocco})
				if ris.Thread < 2 {
					t.Fatalf("thread: %+v", ris)
				}
				verifica(t, p, casi)
			}
		}
	}
}
