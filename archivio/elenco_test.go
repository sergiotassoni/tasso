// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var tuttiIFormati = FormatiCreabili

func creaEsempio(t *testing.T, f Formato) (dir, arch string) {
	t.Helper()
	dir = t.TempDir()
	src := esempio(t, dir)
	ris, err := Comprimi(context.Background(), []string{src}, OpzioniComprimi{Formato: f})
	if err != nil {
		t.Fatal(err)
	}
	altra := filepath.Join(dir, "altra")
	os.Mkdir(altra, 0o755)
	arch = filepath.Join(altra, filepath.Base(ris.Archivio))
	if err := os.Rename(ris.Archivio, arch); err != nil {
		t.Fatal(err)
	}
	return altra, arch
}

func TestElenca(t *testing.T) {
	for _, f := range tuttiIFormati {
		t.Run(string(f), func(t *testing.T) {
			_, arch := creaEsempio(t, f)
			el, err := Elenca(context.Background(), arch, "")
			if err != nil {
				t.Fatal(err)
			}
			var nomi []string
			for _, v := range el.Voci {
				s := v.Nome
				if v.Cartella {
					s += "/"
				}
				nomi = append(nomi, s)
				if v.Nome == "Progetto/documenti/relazione è già.txt" && v.Dim != int64(len("Tasso Zip comprime bene. ")*2000) {
					t.Errorf("dimensione: %d", v.Dim)
				}
				if !v.Cartella && v.Quando.IsZero() {
					t.Errorf("%s senza data", v.Nome)
				}
			}
			sort.Strings(nomi)
			atteso := []string{"Progetto/", "Progetto/documenti/", "Progetto/documenti/foto.jpg",
				"Progetto/documenti/relazione è già.txt", "Progetto/leggimi.txt", "Progetto/vuota/"}
			if strings.Join(nomi, "|") != strings.Join(atteso, "|") {
				t.Fatalf("elenco:\n%v\n%v", nomi, atteso)
			}
		})
	}
}

func TestEstraiSelezionatiEModi(t *testing.T) {
	for _, f := range tuttiIFormati {
		t.Run(string(f), func(t *testing.T) {
			altra, arch := creaEsempio(t, f)

			// Solo un file, togliendo la cartella in cui sto guardando: va "qui" in una cartella temporanea.
			tmp := filepath.Join(altra, "tmp-apri")
			os.Mkdir(tmp, 0o755)
			r, err := Estrai(context.Background(), arch, OpzioniEstrai{
				Selezione: []string{"Progetto/documenti/relazione è già.txt"}, TogliPrefisso: "Progetto/documenti",
				Modo: ModoQui, Destinazione: tmp,
			})
			if err != nil {
				t.Fatal(err)
			}
			if r.File != 1 || r.UltimoFile != filepath.Join(tmp, "relazione è già.txt") {
				t.Fatalf("%+v", r)
			}
			if got := albero(t, tmp); len(got) != 1 {
				t.Fatalf("%d voci", len(got))
			}
			// Di nuovo: non sovrascrive, diventa "(2)".
			r, err = Estrai(context.Background(), arch, OpzioniEstrai{
				Selezione: []string{"Progetto/documenti/relazione è già.txt"}, TogliPrefisso: "Progetto/documenti",
				Modo: ModoQui, Destinazione: tmp,
			})
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Base(r.UltimoFile) != "relazione è già (2).txt" {
				t.Fatalf("%+v", r)
			}
			nienteAvanzi(t, tmp)

			// Una cartella selezionata: con tutto il contenuto.
			r, err = Estrai(context.Background(), arch, OpzioniEstrai{Selezione: []string{"Progetto/documenti"}})
			if err != nil {
				t.Fatal(err)
			}
			got := albero(t, r.Cartella)
			if filepath.Base(r.Cartella) != "Progetto" || len(got) != 3 || got["documenti/foto.jpg"] != "finta foto" {
				t.Fatalf("%s %d", r.Cartella, len(got))
			}

			// "Estrai qui": la cartella Progetto finisce accanto all'archivio (c'è già: "Progetto (2)").
			r, err = Estrai(context.Background(), arch, OpzioniEstrai{Modo: ModoQui})
			if err != nil {
				t.Fatal(err)
			}
			if r.Cartella != altra {
				t.Fatalf("%+v", r)
			}
			if _, err := os.Stat(filepath.Join(altra, "Progetto (2)", "leggimi.txt")); err != nil {
				t.Fatal(err)
			}
			// Una seconda volta: "Progetto (3)".
			if _, err = Estrai(context.Background(), arch, OpzioniEstrai{Modo: ModoQui}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(altra, "Progetto (3)", "leggimi.txt")); err != nil {
				t.Fatal(err)
			}
			// "Estrai in Progetto\": la cartella si chiama come l'archivio (qui coincide con quella dentro).
			r, err = Estrai(context.Background(), arch, OpzioniEstrai{Modo: ModoCartella})
			if err != nil {
				t.Fatal(err)
			}
			if r.Cartella != filepath.Join(altra, "Progetto (4)") {
				t.Fatalf("%+v", r)
			}
			if _, err := os.Stat(filepath.Join(r.Cartella, "leggimi.txt")); err != nil {
				t.Fatal(err)
			}
			nienteAvanzi(t, altra)
		})
	}
}

// Un archivio con più cose sciolte: "Estrai in nome\" le mette in una cartella col nome dell'archivio,
// "Estrai qui" le mette accanto.
func TestModoCartellaVociSciolte(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "Sciolti.zip")
	f, _ := os.Create(p)
	zw := zip.NewWriter(f)
	for _, n := range []string{"a.txt", "b.txt", "sotto/c.txt"} {
		w, _ := zw.Create(n)
		w.Write([]byte(n))
	}
	zw.Close()
	f.Close()
	r, err := Estrai(context.Background(), p, OpzioniEstrai{Modo: ModoCartella})
	if err != nil {
		t.Fatal(err)
	}
	if r.Cartella != filepath.Join(dir, "Sciolti") || len(albero(t, r.Cartella)) != 4 {
		t.Fatalf("%+v", r)
	}
	qui := filepath.Join(dir, "qui")
	os.Mkdir(qui, 0o755)
	scrivi(t, filepath.Join(qui, "a.txt"), "già qui")
	if _, err := Estrai(context.Background(), p, OpzioniEstrai{Modo: ModoQui, Destinazione: qui}); err != nil {
		t.Fatal(err)
	}
	got := albero(t, qui)
	if got["a.txt"] != "già qui" || got["a (2).txt"] != "a.txt" || got["sotto/c.txt"] != "sotto/c.txt" {
		t.Fatalf("%v", got)
	}
	nienteAvanzi(t, qui)
}

// La selezione non deve mai far uscire dalla cartella.
func TestSelezioneZipSlip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cattivo.zip")
	f, _ := os.Create(p)
	zw := zip.NewWriter(f)
	for _, n := range []string{"../fuori.txt", "dentro/../../fuori2.txt", "ok/bene.txt"} {
		w, _ := zw.CreateHeader(&zip.FileHeader{Name: n})
		w.Write([]byte("x"))
	}
	zw.Close()
	f.Close()
	dest := filepath.Join(dir, "d")
	os.Mkdir(dest, 0o755)
	r, err := Estrai(context.Background(), p, OpzioniEstrai{Modo: ModoQui, Destinazione: dest, Selezione: []string{"../fuori.txt", "dentro", "ok"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "fuori.txt")); err == nil {
		t.Fatal("uscito dalla cartella")
	}
	if _, err := os.Stat(filepath.Join(dir, "fuori2.txt")); err == nil {
		t.Fatal("uscito dalla cartella")
	}
	if r.File != 1 || len(r.Saltati) != 2 {
		t.Fatalf("%+v", r)
	}
}
