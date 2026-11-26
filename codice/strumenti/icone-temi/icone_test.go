package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"tasso/icotema"
)

func leggi(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Senza icone dei temi il .syso deve restare quello di prima (manifest, versione, icona 2).
func TestSenzaTemiUgualeAPrima(t *testing.T) {
	syso := leggi(t, "../../rsrc_windows_amd64.syso")
	base, err := ComponiSyso(syso, nil)
	if err != nil {
		t.Fatal(err)
	}
	ris0, _ := LeggiRisorse(syso)
	ris1, err := LeggiRisorse(base)
	if err != nil {
		t.Fatal(err)
	}
	var normali []Risorsa
	for _, r := range ris0 {
		if !(r.Tipo == RT_GROUP_ICON && r.ID >= icotema.PrimoID || r.Tipo == RT_ICON && r.ID >= icotema.PrimaImmagine) {
			normali = append(normali, r)
		}
	}
	if len(ris1) != len(normali) {
		t.Fatalf("risorse %d, prima %d", len(ris1), len(normali))
	}
	for i := range ris1 {
		if ris1[i].Tipo != normali[i].Tipo || ris1[i].ID != normali[i].ID || !bytes.Equal(ris1[i].Dati, normali[i].Dati) {
			t.Fatalf("risorsa %d diversa", i)
		}
	}
}

func TestConIconeDeiTemi(t *testing.T) {
	syso := leggi(t, "../../rsrc_windows_amd64.syso")
	ico := leggi(t, "../../assets/tasso.ico")
	imm, _ := LeggiICO(ico)
	nuovo, err := ComponiSyso(syso, map[uint16][]byte{201: ico, 203: ico})
	if err != nil {
		t.Fatal(err)
	}
	// rifarlo dal risultato dà lo stesso file (le vecchie icone dei temi si tolgono bene)
	ancora, err := ComponiSyso(nuovo, map[uint16][]byte{201: ico, 203: ico})
	if err != nil || !bytes.Equal(ancora, nuovo) {
		t.Fatalf("non stabile: %v", err)
	}
	ris, _ := LeggiRisorse(nuovo)
	gruppi := map[uint32][]byte{}
	immagini := map[uint32][]byte{}
	for _, r := range ris {
		switch r.Tipo {
		case RT_GROUP_ICON:
			gruppi[r.ID] = r.Dati
		case RT_ICON:
			immagini[r.ID] = r.Dati
		}
	}
	for _, id := range []uint32{2, 201, 203} {
		if gruppi[id] == nil {
			t.Fatalf("manca il gruppo %d", id)
		}
	}
	vv, err := leggiGruppoIcone(gruppi[203])
	if err != nil || len(vv) != len(imm) {
		t.Fatalf("gruppo 203: %v", err)
	}
	for i, v := range vv {
		if int(v.ID) != icotema.PrimaImmagine+len(imm)+i || !bytes.Equal(immagini[uint32(v.ID)], imm[i].Dati) {
			t.Fatalf("immagine %d del gruppo 203 sbagliata (ID %d)", i, v.ID)
		}
	}
	// togliendo i temi si torna esattamente al .syso senza temi
	senza, _ := ComponiSyso(nuovo, nil)
	base, _ := ComponiSyso(syso, nil)
	if !bytes.Equal(senza, base) {
		t.Fatal("tolte le icone dei temi il .syso non torna come prima")
	}
}

func TestCopiaDaCartellaDiT1(t *testing.T) {
	dir := t.TempDir()
	da := filepath.Join(dir, "temi")
	cod := filepath.Join(dir, "codice")
	ico := leggi(t, "../../assets/tasso.ico")
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256"><ellipse cx="74" cy="98" rx="9" ry="9" fill="#F5C542"/></svg>`)
	os.MkdirAll(filepath.Join(da, "oro"), 0o755)
	os.MkdirAll(filepath.Join(da, "Sbagliato"), 0o755)
	os.MkdirAll(filepath.Join(cod, "assets", "temi"), 0o755)
	os.WriteFile(filepath.Join(da, "oro", "tasso-gaming.ico"), ico, 0o644)
	os.WriteFile(filepath.Join(da, "oro", "tasso-gaming.svg"), svg, 0o644)
	os.WriteFile(filepath.Join(da, "oro", "tasso-ufficio.ico"), []byte("non è un ico"), 0o644)
	os.WriteFile(filepath.Join(cod, "rsrc_windows_amd64.syso"), leggi(t, "../../rsrc_windows_amd64.syso"), 0o644)
	if err := esegui([]string{"-da", da, "-codice", cod}, io.Discard); err == nil {
		t.Fatal("un .ico rotto deve fermare tutto")
	}
	os.Remove(filepath.Join(da, "oro", "tasso-ufficio.ico"))
	if err := esegui([]string{"-da", da, "-codice", cod}, io.Discard); err != nil {
		t.Fatal(err)
	}
	el, err := icotema.Leggi(leggi(t, filepath.Join(cod, "assets", "temi", "elenco.txt")))
	if err != nil || el.Trova("oro", icotema.Gaming) != 201 || len(el.Voci) != 1 {
		t.Fatalf("elenco %v %+v", err, el.Voci)
	}
	if !bytes.Equal(leggi(t, filepath.Join(cod, "assets", "temi", "oro", "tasso-gaming.svg")), svg) {
		t.Fatal("svg non copiato")
	}
	if err := esegui([]string{"-codice", cod, "-controlla"}, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestSVGSicuro(t *testing.T) {
	ok := []string{
		`<svg viewBox="0 0 256 256"><defs><linearGradient id="met"/></defs><rect fill="url(#met)"/><ellipse fill="#FFF"/></svg>`,
	}
	no := []string{
		`<svg><script>alert(1)</script></svg>`,
		`<svg onload="x()"></svg>`,
		`<svg><rect onclick = "x()"/></svg>`,
		`<svg><a href="https://x"><rect/></a></svg>`,
		`<svg><image xlink:href="x.png"/></svg>`,
		`<svg><foreignObject></foreignObject></svg>`,
		`<svg><rect style="fill:url(http://x)"/></svg>`,
		`non è un svg`,
	}
	for _, s := range ok {
		if !SVGSicuro([]byte(s)) {
			t.Errorf("rifiutato: %s", s)
		}
	}
	for _, s := range no {
		if SVGSicuro([]byte(s)) {
			t.Errorf("accettato: %s", s)
		}
	}
	// tutti i disegni veri di T1, se ci sono
	if m, _ := filepath.Glob("/home/claude/ecosistema/icone/temi/*/tasso-*.svg"); len(m) > 0 {
		for _, p := range m {
			if !SVGSicuro(leggi(t, p)) {
				t.Errorf("disegno vero rifiutato: %s", p)
			}
		}
	}
}
