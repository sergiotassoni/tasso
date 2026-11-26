// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

// Verifica di secondo giro: somma di controllo finale dei tar compressi, dizionari degli .xz.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	"github.com/ulikunitz/xz"
)

// tarDati: un tar con un file (dati.txt) dentro.
func tarDati(t *testing.T, dati []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	if err := tw.WriteHeader(&tar.Header{Name: "dati.txt", Size: int64(len(dati)), Mode: 0o644, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	tw.Write(dati)
	tw.Close()
	return b.Bytes()
}

func gzipLivello(t *testing.T, b []byte, livello int) []byte {
	t.Helper()
	var out bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&out, livello)
	gz.Write(b)
	gz.Close()
	return out.Bytes()
}

func salvaIn(t *testing.T, nome string, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), nome)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// Un byte dei dati cambiato in un tar.gz "non compresso" (blocchi stored): il deflate non se
// ne accorge, solo il CRC32 in fondo al gzip. Prima: Verifica "OK" e file sbagliato estratto.
func TestTarGzByteCambiatoSiVede(t *testing.T) {
	dati := bytes.Repeat([]byte("ABCDEFGH"), 4096)
	b := gzipLivello(t, tarDati(t, dati), gzip.NoCompression)
	i := bytes.Index(b, []byte("ABCDEFGHABCDEFGH")) + 1000
	b[i] ^= 0x20
	p := salvaIn(t, "x.tar.gz", b)

	if _, err := Verifica(context.Background(), p, OpzioniVerifica{}); Codice(err) != "rovinato" {
		t.Fatalf("Verifica doveva dire rovinato: %v", err)
	}
	if _, err := Elenca(context.Background(), p, ""); Codice(err) != "rovinato" {
		t.Fatalf("Elenca doveva dire rovinato: %v", err)
	}
	ris, err := Estrai(context.Background(), p, OpzioniEstrai{})
	if err != nil {
		t.Fatalf("Estrai: %v", err)
	}
	if !ris.SommaSbagliata {
		t.Fatal("Estrai doveva avvisare che la somma di controllo non torna")
	}
	// lo stesso archivio sano: niente avviso
	sano := salvaIn(t, "sano.tar.gz", gzipLivello(t, tarDati(t, dati), gzip.NoCompression))
	if _, err := Verifica(context.Background(), sano, OpzioniVerifica{}); err != nil {
		t.Fatalf("archivio sano: %v", err)
	}
	ris, err = Estrai(context.Background(), sano, OpzioniEstrai{})
	if err != nil || ris.SommaSbagliata || ris.File != 1 {
		t.Fatalf("archivio sano: %+v %v", ris, err)
	}
}

// Zeri in più dopo il gzip (alcuni programmi riempiono così) e gzip fatti di più membri:
// vanno bene, non sono danni.
func TestTarGzRiempimentoEMembri(t *testing.T) {
	dati := bytes.Repeat([]byte("riempimento "), 3000)
	tb := tarDati(t, dati)
	casi := map[string][]byte{
		"zeri.tar.gz":   append(gzipLivello(t, tb, gzip.BestCompression), make([]byte, 1000)...),
		"zeri5.tar.gz":  append(gzipLivello(t, tb, gzip.BestCompression), make([]byte, 5)...),
		"membri.tar.gz": append(gzipLivello(t, tb[:len(tb)/3], 6), gzipLivello(t, tb[len(tb)/3:], 6)...),
	}
	for nome, b := range casi {
		p := salvaIn(t, nome, b)
		if _, err := Verifica(context.Background(), p, OpzioniVerifica{}); err != nil {
			t.Errorf("%s: Verifica: %v", nome, err)
		}
		ris, err := Estrai(context.Background(), p, OpzioniEstrai{})
		if err != nil || ris.SommaSbagliata || ris.File != 1 {
			t.Errorf("%s: Estrai: %+v %v", nome, ris, err)
			continue
		}
		got, _ := os.ReadFile(ris.UltimoFile)
		if !bytes.Equal(got, dati) {
			t.Errorf("%s: dati diversi", nome)
		}
	}
}

// Un byte cambiato in tanti punti diversi dei tar compressi creati da Tasso Zip: la Verifica
// se ne accorge sempre (prima il tar.gz a volte no: TestVerificaTrovaDanni ballava).
func TestVerificaTrovaSempreIDanni(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "Dati")
	var b bytes.Buffer
	for i := 0; b.Len() < 200<<10; i++ {
		fmt.Fprintf(&b, "%d Tasso %x %x\n", i, uint32(i)*2654435761, i*31) // poco comprimibile
	}
	scrivi(t, filepath.Join(src, "a.txt"), b.String())
	for _, f := range []Formato{FormatoTarGz, FormatoTarBz2, FormatoTarXz, FormatoTarZst} {
		ris, err := Comprimi(context.Background(), []string{src}, OpzioniComprimi{Formato: f})
		if err != nil {
			t.Fatal(err)
		}
		orig, _ := os.ReadFile(ris.Archivio)
		non := 0
		for k := 1; k <= 24; k++ {
			dati := append([]byte(nil), orig...)
			i := len(dati) * k / 26 // tutto il file, intestazioni escluse il più possibile
			dati[i] ^= 0x55
			p := filepath.Join(dir, "danno"+f.Estensione())
			os.WriteFile(p, dati, 0o644)
			if _, err := Verifica(context.Background(), p, OpzioniVerifica{}); err == nil {
				// Verifica dice OK: va bene solo se il danno non cambia niente (per es. il primo
				// byte del flusso LZMA, che i decompressori ignorano): i file devono uscire giusti.
				r, err := Estrai(context.Background(), p, OpzioniEstrai{Destinazione: t.TempDir()})
				got, _ := os.ReadFile(r.UltimoFile)
				if err != nil || filepath.Base(r.UltimoFile) != "a.txt" || string(got) != b.String() {
					non++
					t.Logf("%s: byte %d cambiato: Verifica OK ma dati diversi (%v)", f, i, err)
				}
			}
		}
		if non > 0 {
			t.Errorf("%s: %d danni su 24 non trovati (dati sbagliati senza errore)", f, non)
		}
	}
}

// xzConDizionario: un .xz vero (un flusso) con il codice del dizionario cambiato (e il CRC
// dell'intestazione del blocco rifatto): 40 = 4 GB, 37 = 1,5 GB.
func xzConDizionario(t *testing.T, codice byte, dati []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	w, err := xz.NewWriter(&out)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(dati)
	w.Close()
	b := out.Bytes()
	h := (int(b[12]) + 1) * 4
	bh := b[12 : 12+h]
	i := bytes.Index(bh[2:], []byte{0x21, 0x01})
	if i < 0 {
		t.Fatal("filtro LZMA2 non trovato")
	}
	bh[2+i+2] = codice
	binary.LittleEndian.PutUint32(bh[h-4:], crc32.ChecksumIEEE(bh[:h-4]))
	return b
}

func TestXzDizionarioEnormeDichiarato(t *testing.T) {
	memoriaPerProve = 1 << 30 // limite: 512 MB
	defer func() { memoriaPerProve = 0 }()
	dati := bytes.Repeat([]byte("ciao "), 200)
	for _, c := range []byte{40, 37} {
		for _, nome := range []string{"x.xz", "x.tar.xz"} {
			b := xzConDizionario(t, c, dati)
			if nome == "x.tar.xz" {
				b = xzConDizionario(t, c, tarDati(t, dati))
			}
			p := salvaIn(t, nome, b)
			if _, err := Elenca(context.Background(), p, ""); Codice(err) != "memoria" {
				t.Errorf("%s codice %d: Elenca doveva dire memoria: %v", nome, c, err)
			}
			if _, err := Estrai(context.Background(), p, OpzioniEstrai{}); Codice(err) != "memoria" {
				t.Errorf("%s codice %d: Estrai doveva dire memoria: %v", nome, c, err)
			}
			if _, err := Verifica(context.Background(), p, OpzioniVerifica{}); Codice(err) != "memoria" {
				t.Errorf("%s codice %d: Verifica doveva dire memoria: %v", nome, c, err)
			}
		}
	}
	// il dizionario enorme nel SECONDO flusso (due .xz uno dopo l'altro): lo trovo dall'indice
	buono := xzConDizionario(t, 20, dati) // 8 MB?
	doppio := append(append([]byte(nil), buono...), xzConDizionario(t, 40, dati)...)
	if _, err := Estrai(context.Background(), salvaIn(t, "doppio.xz", doppio), OpzioniEstrai{}); Codice(err) != "memoria" {
		t.Errorf("secondo flusso con 4 GB: doveva dire memoria: %v", err)
	}
	// un .xz normale si apre
	ris, err := Estrai(context.Background(), salvaIn(t, "normale.xz", buono), OpzioniEstrai{})
	if err != nil || ris.File != 1 {
		t.Fatalf(".xz normale: %+v %v", ris, err)
	}
	blocchi, err := blocchiXz(bytes.NewReader(doppio), int64(len(doppio)))
	if err != nil || len(blocchi) != 2 || blocchi[0] != 12 || blocchi[1] != int64(len(buono))+12 {
		t.Fatalf("blocchi: %v %v", blocchi, err)
	}
}
