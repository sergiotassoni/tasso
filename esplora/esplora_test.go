// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package esplora

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func scrivi(t *testing.T, p string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, make([]byte, n), 0o644); err != nil {
		t.Fatal(err)
	}
}

func nomiDi(v []Voce) string {
	var s []string
	for _, x := range v {
		s = append(s, x.Nome)
	}
	return strings.Join(s, "|")
}

func TestElenca(t *testing.T) {
	d := t.TempDir()
	scrivi(t, filepath.Join(d, "foto 10.jpg"), 10)
	scrivi(t, filepath.Join(d, "foto 2.jpg"), 20)
	scrivi(t, filepath.Join(d, "Archivio.zip"), 30)
	scrivi(t, filepath.Join(d, "backup.tar.gz"), 5)
	scrivi(t, filepath.Join(d, ".nascosto"), 1)
	scrivi(t, filepath.Join(d, "zeta", "dentro.txt"), 1)
	os.Mkdir(filepath.Join(d, "Alfa"), 0o755)
	os.Symlink(filepath.Join(d, "zeta"), filepath.Join(d, "collegamento"))

	el, err := Elenca(context.Background(), d, Opzioni{})
	if err != nil {
		t.Fatal(err)
	}
	if got := nomiDi(el.Voci); got != "Alfa|collegamento|zeta|Archivio.zip|backup.tar.gz|foto 2.jpg|foto 10.jpg" {
		t.Fatalf("ordine: %s", got)
	}
	if el.Nascosti != 1 || el.Troppi || el.Padre != filepath.Dir(d) {
		t.Fatalf("%+v", el)
	}
	for _, v := range el.Voci {
		switch v.Nome {
		case "Archivio.zip":
			if v.Categoria != CatArchivio || !v.Archivio || v.Dim != 30 || v.Ext != "zip" {
				t.Errorf("%+v", v)
			}
		case "backup.tar.gz":
			if v.Ext != "tar.gz" || !v.Archivio {
				t.Errorf("%+v", v)
			}
		case "foto 2.jpg":
			if v.Categoria != CatImmagine {
				t.Errorf("%+v", v)
			}
		case "collegamento":
			if !v.Cartella {
				t.Errorf("il collegamento a una cartella è una cartella: %+v", v)
			}
		}
	}
	el, _ = Elenca(context.Background(), d, Opzioni{Nascosti: true})
	if el.Nascosti != 0 || !strings.Contains(nomiDi(el.Voci), ".nascosto") {
		t.Fatal("nascosti non mostrati")
	}
	if _, err := Elenca(context.Background(), filepath.Join(d, "Archivio.zip"), Opzioni{}); err != ErrNonCartella {
		t.Fatalf("file al posto della cartella: %v", err)
	}
	if _, err := Elenca(context.Background(), filepath.Join(d, "non-c'è"), Opzioni{}); err == nil {
		t.Fatal("cartella che non esiste")
	}
}

func TestElencaLimite(t *testing.T) {
	d := t.TempDir()
	for i := 0; i < 700; i++ {
		scrivi(t, filepath.Join(d, fmt.Sprintf("f%04d.txt", i)), 1)
	}
	el, err := Elenca(context.Background(), d, Opzioni{Max: 300})
	if err != nil {
		t.Fatal(err)
	}
	if len(el.Voci) != 300 || !el.Troppi {
		t.Fatalf("%d %v", len(el.Voci), el.Troppi)
	}
	ctx, stop := context.WithCancel(context.Background())
	stop()
	if _, err := Elenca(ctx, d, Opzioni{}); err == nil {
		t.Fatal("annullato")
	}
}

func TestOrdina(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	v := []Voce{
		{Nome: "b.txt", Dim: 300, Quando: t0.Add(3 * time.Hour), Categoria: CatDocumento, Ext: "txt"},
		{Nome: "A.zip", Dim: 100, Quando: t0.Add(1 * time.Hour), Categoria: CatArchivio, Ext: "zip"},
		{Nome: "Cartella", Cartella: true, Categoria: CatCartella},
		{Nome: "c.jpg", Dim: 200, Quando: t0.Add(2 * time.Hour), Categoria: CatImmagine, Ext: "jpg"},
		{Nome: "àncora", Cartella: true, Categoria: CatCartella},
	}
	casi := []struct {
		campo  string
		disc   bool
		atteso string
	}{
		{"nome", false, "àncora|Cartella|A.zip|b.txt|c.jpg"},
		{"nome", true, "Cartella|àncora|c.jpg|b.txt|A.zip"},
		{"dim", false, "àncora|Cartella|A.zip|c.jpg|b.txt"},
		{"dim", true, "Cartella|àncora|b.txt|c.jpg|A.zip"},
		{"quando", true, "Cartella|àncora|b.txt|c.jpg|A.zip"},
		{"tipo", false, "àncora|Cartella|A.zip|b.txt|c.jpg"},
	}
	for _, c := range casi {
		Ordina(v, c.campo, c.disc)
		if got := nomiDi(v); got != c.atteso {
			t.Errorf("%s %v: %s (atteso %s)", c.campo, c.disc, got, c.atteso)
		}
	}
}

func TestConfrontaNomi(t *testing.T) {
	minori := [][2]string{{"foto 2", "foto 10"}, {"a", "B"}, {"file01", "file2"}, {"è", "f"}, {"abc", "abcd"}, {"x9", "x10"}}
	for _, m := range minori {
		if ConfrontaNomi(m[0], m[1]) >= 0 || ConfrontaNomi(m[1], m[0]) <= 0 {
			t.Errorf("%q < %q", m[0], m[1])
		}
	}
	if ConfrontaNomi("Foto", "foto") != 0 {
		t.Error("maiuscole")
	}
}

func TestCategoria(t *testing.T) {
	casi := map[string]string{
		"a.ZIP": CatArchivio, "b.tar.gz": CatArchivio, "c.png": CatImmagine, "d.mkv": CatVideo, "e.mp3": CatAudio,
		"f.pdf": CatDocumento, "g.exe": CatEseguibile, "h.xyz": CatGenerico, "README": CatGenerico, "film.7z.001": CatArchivio,
	}
	for n, c := range casi {
		if got := Categoria(n, false); got != c {
			t.Errorf("%s: %s (atteso %s)", n, got, c)
		}
	}
	if Categoria("x.zip", true) != CatCartella {
		t.Error("cartella")
	}
	if Estensione(".bashrc") != "bashrc" && Estensione(".bashrc") != "" {
		t.Error("estensione")
	}
}

func TestTrova(t *testing.T) {
	d := t.TempDir()
	scrivi(t, filepath.Join(d, "Relazione finale.pdf"), 1)
	scrivi(t, filepath.Join(d, "lavoro", "relazione bozza.docx"), 1)
	scrivi(t, filepath.Join(d, "lavoro", "vecchi", "RELAZIONE 2020.pdf"), 1)
	scrivi(t, filepath.Join(d, "lavoro", "vecchi", "altro.txt"), 1)
	scrivi(t, filepath.Join(d, ".segreto", "relazione nascosta.txt"), 1)
	scrivi(t, filepath.Join(d, "perché.txt"), 1)
	os.Symlink(d, filepath.Join(d, "lavoro", "giro")) // collegamento alla radice: niente giro infinito

	r, err := Trova(context.Background(), d, "relazione", OpzioniTrova{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Voci) != 3 || r.Troppi || r.Fermata {
		t.Fatalf("%+v", r)
	}
	if r.Voci[0].Nome != "Relazione finale.pdf" || r.Voci[0].Dove != "" || r.Voci[2].Dove != filepath.Join("lavoro", "vecchi") {
		t.Fatalf("ordine o cartelle: %+v", r.Voci)
	}
	// con * come in Windows
	r, _ = Trova(context.Background(), d, "*.pdf", OpzioniTrova{})
	if len(r.Voci) != 2 {
		t.Fatalf("*.pdf: %+v", r.Voci)
	}
	// senza accenti
	r, _ = Trova(context.Background(), d, "perche", OpzioniTrova{})
	if len(r.Voci) != 1 {
		t.Fatalf("accenti: %+v", r.Voci)
	}
	// nascosti
	r, _ = Trova(context.Background(), d, "relazione", OpzioniTrova{Nascosti: true})
	if len(r.Voci) != 4 {
		t.Fatalf("nascosti: %d", len(r.Voci))
	}
	// limiti
	r, _ = Trova(context.Background(), d, "relazione", OpzioniTrova{MaxRisultati: 2})
	if len(r.Voci) != 2 || !r.Troppi {
		t.Fatalf("limite risultati: %+v", r)
	}
	r, _ = Trova(context.Background(), d, "relazione", OpzioniTrova{MaxCartelle: 1})
	if len(r.Voci) != 1 || !r.Fermata {
		t.Fatalf("limite cartelle: %+v", r)
	}
	r, _ = Trova(context.Background(), d, "   ", OpzioniTrova{})
	if len(r.Voci) != 0 {
		t.Fatal("ricerca vuota")
	}
	ctx, stop := context.WithCancel(context.Background())
	stop()
	if _, err := Trova(ctx, d, "x", OpzioniTrova{}); err == nil {
		t.Fatal("annullata")
	}
}

func TestTrovaMoltiFile(t *testing.T) {
	d := t.TempDir()
	for i := 0; i < 50; i++ {
		for j := 0; j < 20; j++ {
			scrivi(t, filepath.Join(d, fmt.Sprintf("c%02d", i), fmt.Sprintf("foto%02d.jpg", j)), 1)
		}
	}
	r, err := Trova(context.Background(), d, "foto", OpzioniTrova{MaxRisultati: 100})
	if err != nil || len(r.Voci) != 100 || !r.Troppi {
		t.Fatalf("%v %d", err, len(r.Voci))
	}
}

func TestMisura(t *testing.T) {
	d := t.TempDir()
	scrivi(t, filepath.Join(d, "a", "1.bin"), 1000)
	scrivi(t, filepath.Join(d, "a", "b", "2.bin"), 2000)
	scrivi(t, filepath.Join(d, "3.bin"), 500)
	os.Symlink(filepath.Join(d, "a"), filepath.Join(d, "a", "giro"))
	m := MisuraPercorsi(context.Background(), []string{filepath.Join(d, "a"), filepath.Join(d, "3.bin")}, 0, 0)
	if m.File != 3 || m.Cartelle != 1 || m.Byte != 3500 || m.Fermata {
		t.Fatalf("%+v", m)
	}
	m = MisuraPercorsi(context.Background(), []string{d}, 2, 0)
	if !m.Fermata {
		t.Fatalf("limite: %+v", m)
	}
}

func TestPadre(t *testing.T) {
	if Padre("/") != "" || Padre("/home/x") != "/home" {
		t.Error(Padre("/"), Padre("/home/x"))
	}
}
