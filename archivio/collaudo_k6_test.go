// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

// Collaudo fase 2 (K6): archivi a flusso tagliati a qualunque lunghezza.

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/ulikunitz/xz"
)

// Un archivio a flusso tagliato (scaricamento non finito) deve essere "rovinato" per
// qualunque lunghezza (zip e 7z li ho provati a parte: già giusti). Prima un .tar.xz tagliato a 12-23 byte (dentro l'intestazione del primo blocco)
// o subito prima dell'indice usciva come file vuoto o incompleto con Verifica "OK".
func TestTagliatiOgniLunghezza(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	b := make([]byte, 700)
	rand.New(rand.NewSource(3)).Read(b)
	scrivi(t, filepath.Join(src, "a.bin"), string(b))
	for _, f := range []Formato{FormatoTarGz, FormatoTarBz2, FormatoTarXz, FormatoTarZst} {
		out := filepath.Join(dir, "o"+string(f))
		os.Mkdir(out, 0o755)
		rc, err := Comprimi(context.Background(), []string{src}, OpzioniComprimi{Formato: f, Cartella: out})
		if err != nil {
			t.Fatal(err)
		}
		tutto, _ := os.ReadFile(rc.Archivio)
		var passati []int
		for n := 1; n < len(tutto); n++ {
			p := filepath.Join(dir, fmt.Sprintf("t%d%s", n, f.Estensione()))
			os.WriteFile(p, tutto[:n], 0o644)
			if _, err := Verifica(context.Background(), p, OpzioniVerifica{}); err == nil {
				passati = append(passati, n)
			}
			os.Remove(p)
		}
		if len(passati) > 0 {
			t.Errorf("%s (%d byte): tagliato a %v la Verifica dice OK", f, len(tutto), passati)
		}
	}
}

// Lo stesso per un .xz semplice (un file solo), in Verifica, Elenca ed Estrai; e gli .xz
// giusti con riempimento di zeri o con due flussi devono restare buoni.
func TestXzTagliatoEFineGiusta(t *testing.T) {
	dati := bytes.Repeat([]byte("Tasso Zip comprime bene. "), 400)
	flusso := func() []byte {
		var b bytes.Buffer
		w, _ := xz.NewWriter(&b)
		w.Write(dati)
		w.Close()
		return b.Bytes()
	}
	uno := flusso()
	for _, n := range []int{12, 13, 18, 23, len(uno) - 24, len(uno) - 12, len(uno) - 1} {
		p := salvaIn(t, "nota.txt.xz", uno[:n])
		if _, err := Verifica(context.Background(), p, OpzioniVerifica{}); Codice(err) != "rovinato" {
			t.Errorf("tagliato a %d: verifica %v", n, err)
		}
		if _, err := Elenca(context.Background(), p, ""); Codice(err) != "rovinato" {
			t.Errorf("tagliato a %d: elenca %v", n, err)
		}
		if _, err := Estrai(context.Background(), p, OpzioniEstrai{}); Codice(err) != "rovinato" {
			t.Errorf("tagliato a %d: estrai %v", n, err)
		}
		nienteAvanzi(t, filepath.Dir(p))
	}
	buoni := map[string][]byte{
		"zeri":       append(append([]byte(nil), uno...), make([]byte, 8)...),
		"due flussi": append(append(append([]byte(nil), uno...), make([]byte, 4)...), flusso()...),
	}
	for nome, b := range buoni {
		p := salvaIn(t, "nota.txt.xz", b)
		if _, err := Verifica(context.Background(), p, OpzioniVerifica{}); err != nil {
			t.Errorf("%s: verifica %v", nome, err)
		}
		re, err := Estrai(context.Background(), p, OpzioniEstrai{})
		if err != nil {
			t.Fatalf("%s: estrai %v", nome, err)
		}
		got, _ := os.ReadFile(filepath.Join(re.Cartella, "nota.txt"))
		want := len(dati)
		if nome == "due flussi" {
			want *= 2
		}
		if len(got) != want {
			t.Errorf("%s: estratti %d byte, attesi %d", nome, len(got), want)
		}
	}
}

