package installa

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPiuNuova(t *testing.T) {
	si := [][2]string{{"3.2", "3.1"}, {"3.10", "3.9"}, {"1.2.1", "1.2"}, {"2", "1.9.9"}, {"1.0", ""}, {"1.0", "x"}}
	no := [][2]string{{"3.1", "3.1"}, {"1.2", "1.2.0"}, {"3.1", "3.2"}, {"", "1.0"}, {"x", "1.0"}, {"1.-2", "1.0"}, {"+3", "1"}, {"3..1", "1"}}
	for _, c := range si {
		if !PiuNuova(c[0], c[1]) {
			t.Errorf("%q dovrebbe essere più nuova di %q", c[0], c[1])
		}
	}
	for _, c := range no {
		if PiuNuova(c[0], c[1]) {
			t.Errorf("%q non dovrebbe essere più nuova di %q", c[0], c[1])
		}
	}
	if Maggiore("3.1", "3.2") != "3.2" || Maggiore("3.2", "3.1") != "3.2" || Maggiore("3.1", "") != "3.1" {
		t.Error("Maggiore")
	}
}

// versioni: cambia la versione di alcuni programmi dentro l'installatore finto.
func versioni(e *Elenco, v map[Programma]string) {
	for i := range e.Programmi {
		if x, ok := v[e.Programmi[i].ID]; ok {
			e.Programmi[i].Versione = x
		}
	}
}

func TestAggiornamentoSilenzioso(t *testing.T) {
	p := nuovaProva(t, "3.1")
	o := p.opzioni(Ufficio, Zip, PDF) // solo Ufficio, con il Desktop
	o.Desktop = true
	o.Lingua = "de"
	if _, err := p.in.Installa(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	st, _ := LeggiStato(p.base)

	// stesso installatore: niente da fare
	if _, err := PreparaAggiornamento(st, p.in.Elenco, "3.1", p.menu, p.desk); !errors.Is(err, ErrNienteDaAggiornare) {
		t.Fatalf("stesse versioni: %v", err)
	}
	// niente installato
	if _, err := PreparaAggiornamento(nil, p.in.Elenco, "", p.menu, p.desk); !errors.Is(err, ErrNienteDaAggiornare) {
		t.Fatalf("niente installato: %v", err)
	}

	// installatore nuovo: Tasso 3.2 e PDF 1.0 (più vecchio di 3.1: mai indietro), Zip uguale
	src, e := pacchetto(t, "3.2")
	versioni(e, map[Programma]string{ProgZip: "3.1", ProgPDF: "1.0"})
	piano, err := PreparaAggiornamento(st, e, "3.1", p.menu, p.desk)
	if err != nil {
		t.Fatal(err)
	}
	if len(piano.Programmi) != 1 || piano.Programmi[0].Programma != ProgTasso || piano.Programmi[0].Installata != "3.1" || piano.Programmi[0].Nuova != "3.2" {
		t.Fatalf("programmi da aggiornare: %+v", piano.Programmi)
	}
	op := piano.Opzioni
	if TestoComponenti(op.Componenti) != "ufficio" || op.Base != p.base || op.Lingua != "de" || op.Desktop || op.DesktopTogli || op.TieniDisinstallatore {
		t.Fatalf("opzioni: %+v", op)
	}
	p.in.Sorgente, p.in.Elenco = src, e
	st2, err := p.in.Installa(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	if leggi(t, filepath.Join(p.base, "Tasso.exe")) != "tasso 3.2" {
		t.Fatal("Tasso non aggiornato")
	}
	if leggi(t, filepath.Join(p.base, "Tasso PDF", "TassoPDF.exe")) != "pdf 3.1" || leggi(t, filepath.Join(p.base, "Tasso Zip", "TassoZip.exe")) != "zip 3.1" {
		t.Fatal("PDF o Zip cambiati")
	}
	if TestoComponenti(st2.Componenti) != "ufficio,zip,pdf" || st2.Programmi[ProgPDF].Versione != "3.1" || st2.Programmi[ProgTasso].Versione != "3.2" {
		t.Fatalf("stato dopo: %+v", st2)
	}
	if v, _ := p.reg.LeggiTesto(HKLM, ChiaveTasso, "Modalita"); v != "ufficio" {
		t.Fatalf("modalità cambiata: %q", v)
	}
	if v, _ := p.reg.LeggiTesto(HKCU, ChiaveTasso, "Lingua"); v != "de" {
		t.Fatalf("lingua cambiata: %q", v)
	}
	// collegamento di Tasso: sempre "Tasso Ufficio", nel menu e sul Desktop
	if !esiste(filepath.Join(p.desk, "Tasso Ufficio.lnk")) || !esiste(filepath.Join(p.menu, "Tasso Ufficio.lnk")) || esiste(filepath.Join(p.desk, "Tasso Gaming.lnk")) {
		t.Fatal("collegamenti di Tasso sbagliati")
	}
	// ora la stessa cosa è installata: niente da fare
	if _, err := PreparaAggiornamento(st2, e, "3.2", p.menu, p.desk); !errors.Is(err, ErrNienteDaAggiornare) {
		t.Fatalf("di nuovo: %v", err)
	}
}

// Tasso aggiornato da solo (file VERSIONE più nuovo di tasso-setup.json): non lo rimetto indietro.
func TestAggiornamentoSilenziosoVersioneTasso(t *testing.T) {
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(Gaming, Zip)); err != nil {
		t.Fatal(err)
	}
	st, _ := LeggiStato(p.base)
	_, e := pacchetto(t, "3.2")
	// Tasso c'è già in 3.3 (più nuovo del 3.2 dell'installatore), Zip 3.2 > 3.1: solo Zip
	piano, err := PreparaAggiornamento(st, e, "3.3", p.menu, p.desk)
	if err != nil {
		t.Fatal(err)
	}
	if len(piano.Programmi) != 1 || piano.Programmi[0].Programma != ProgZip || TestoComponenti(piano.Opzioni.Componenti) != "zip" {
		t.Fatalf("piano: %+v", piano.Programmi)
	}
	if piano.Opzioni.Desktop {
		t.Fatal("Desktop acceso senza collegamenti sul Desktop")
	}
}

// Desktop: se il collegamento l'hai tolto tu, non lo rimetto.
func TestAggiornamentoSilenziosoDesktopTolto(t *testing.T) {
	p := nuovaProva(t, "3.1")
	o := p.opzioni(Gaming, Ufficio)
	o.Desktop = true
	if _, err := p.in.Installa(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(p.desk, "Tasso Gaming.lnk"))
	st, _ := LeggiStato(p.base)
	src, e := pacchetto(t, "3.2")
	piano, err := PreparaAggiornamento(st, e, "", p.menu, p.desk)
	if err != nil {
		t.Fatal(err)
	}
	if piano.Opzioni.Desktop || TestoComponenti(piano.Opzioni.Componenti) != "gaming,ufficio" {
		t.Fatalf("opzioni: %+v", piano.Opzioni)
	}
	p.in.Sorgente, p.in.Elenco = src, e
	if _, err := p.in.Installa(context.Background(), piano.Opzioni); err != nil {
		t.Fatal(err)
	}
	if esiste(filepath.Join(p.desk, "Tasso Gaming.lnk")) || !esiste(filepath.Join(p.menu, "Tasso Gaming.lnk")) {
		t.Fatal("collegamenti dopo l'aggiornamento")
	}
}

// Installatore con l'ecosistema più vecchio (es. 3.0) ma con un PDF più nuovo: si aggiorna PDF,
// il disinstallatore e la versione dell'ecosistema restano quelli più nuovi.
func TestAggiornamentoSilenziosoTieneDisinstallatore(t *testing.T) {
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(PDF)); err != nil {
		t.Fatal(err)
	}
	st, _ := LeggiStato(p.base)
	src, e := pacchetto(t, "3.0")
	versioni(e, map[Programma]string{ProgPDF: "3.5"})
	piano, err := PreparaAggiornamento(st, e, "", p.menu, p.desk)
	if err != nil {
		t.Fatal(err)
	}
	if !piano.Opzioni.TieniDisinstallatore {
		t.Fatal("il disinstallatore più nuovo verrebbe sostituito")
	}
	p.in.Sorgente, p.in.Elenco = src, e
	st2, err := p.in.Installa(context.Background(), piano.Opzioni)
	if err != nil {
		t.Fatal(err)
	}
	if leggi(t, filepath.Join(p.base, NomeDisinstalla)) != "disinst 3.1" || leggi(t, filepath.Join(p.base, "Tasso PDF", "TassoPDF.exe")) != "pdf 3.0" {
		t.Fatal("file sbagliati")
	}
	if st2.Versione != "3.1" || st2.Programmi[ProgPDF].Versione != "3.5" || st2.Programmi[ProgSetup].Versione != "3.1" {
		t.Fatalf("stato: %+v", st2)
	}
	if v, _ := p.reg.LeggiTesto(HKLM, ChiaveDisinstalla, "DisplayVersion"); v != "3.1" {
		t.Fatalf("DisplayVersion %q", v)
	}
}

// Silenzioso: i collegamenti sul Desktop restano esattamente quelli che c'erano (per programma).
func TestAggiornamentoSilenziosoDesktopPerProgramma(t *testing.T) {
	p := nuovaProva(t, "3.1")
	o := p.opzioni(Gaming, Zip, PDF)
	o.Desktop = true
	if _, err := p.in.Installa(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(p.desk, "Tasso Zip.lnk")) // Zip sul Desktop l'hai tolto tu
	st, _ := LeggiStato(p.base)
	if es := DesktopEsistenti(st, p.desk); !es[ProgTasso] || es[ProgZip] || !es[ProgPDF] || !DesktopUsato(st, p.desk) {
		t.Fatalf("esistenti: %v", es)
	}
	src, e := pacchetto(t, "3.2")
	piano, err := PreparaAggiornamento(st, e, "", p.menu, p.desk)
	if err != nil {
		t.Fatal(err)
	}
	p.in.Sorgente, p.in.Elenco = src, e
	if _, err := p.in.Installa(context.Background(), piano.Opzioni); err != nil {
		t.Fatal(err)
	}
	if !esiste(filepath.Join(p.desk, "Tasso Gaming.lnk")) || !esiste(filepath.Join(p.desk, "Tasso PDF.lnk")) ||
		esiste(filepath.Join(p.desk, "Tasso Zip.lnk")) {
		t.Fatal("Desktop cambiato dall'aggiornamento silenzioso")
	}
	if !strings.Contains(p.coll[filepath.Join(p.desk, "Tasso Gaming.lnk")].Destinazione, "Tasso.exe") {
		t.Fatal("collegamento sul Desktop non rifatto")
	}
}

// Aggiornamento con le pagine e la casella del Desktop non spuntata (come nella prova vera):
// il "Tasso Gaming" che c'era sul Desktop resta, e non se ne aggiungono altri.
func TestAggiornamentoDesktopNonSpuntatoTiene(t *testing.T) {
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(Gaming, Zip)); err != nil {
		t.Fatal(err)
	}
	// collegamento messo sul Desktop (anche solo il file, non scritto in tasso-setup.json)
	os.WriteFile(filepath.Join(p.desk, "Tasso Gaming.lnk"), []byte("x"), 0o644)
	st, _ := LeggiStato(p.base)
	if !DesktopUsato(st, p.desk) {
		t.Fatal("la casella del Desktop deve partire spuntata")
	}
	src, e := pacchetto(t, "3.2")
	p.in.Sorgente, p.in.Elenco = src, e
	if _, err := p.in.Installa(context.Background(), p.opzioni(Gaming, Zip)); err != nil {
		t.Fatal(err)
	}
	if !esiste(filepath.Join(p.desk, "Tasso Gaming.lnk")) || esiste(filepath.Join(p.desk, "Tasso Zip.lnk")) {
		t.Fatal("Desktop: doveva restare solo Tasso Gaming")
	}
	// spuntata: si aggiunge anche Zip
	o := p.opzioni(Gaming, Zip)
	o.Desktop = true
	if _, err := p.in.Installa(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if !esiste(filepath.Join(p.desk, "Tasso Gaming.lnk")) || !esiste(filepath.Join(p.desk, "Tasso Zip.lnk")) {
		t.Fatal("Desktop spuntato: mancano collegamenti")
	}
	// tolta la spunta da te: via quelli dei programmi scelti (solo Zip), Tasso resta
	o = p.opzioni(Zip)
	o.DesktopTogli = true
	if _, err := p.in.Installa(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if !esiste(filepath.Join(p.desk, "Tasso Gaming.lnk")) || esiste(filepath.Join(p.desk, "Tasso Zip.lnk")) ||
		!esiste(filepath.Join(p.menu, "Tasso Zip.lnk")) {
		t.Fatal("DesktopTogli su Zip")
	}
	o = p.opzioni(Gaming, Zip)
	o.DesktopTogli = true
	st2, err := p.in.Installa(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if esiste(filepath.Join(p.desk, "Tasso Gaming.lnk")) || !esiste(filepath.Join(p.menu, "Tasso Gaming.lnk")) || DesktopUsato(st2, p.desk) {
		t.Fatal("DesktopTogli: il Desktop doveva restare vuoto")
	}
}

func TestSenzaPiuNuovi(t *testing.T) {
	st := &Stato{Programmi: map[Programma]StatoProg{ProgTasso: {Versione: "3.1"}, ProgZip: {Versione: "1.5"}}}
	e := &Elenco{Programmi: []ProgElenco{{ID: ProgTasso, Versione: "3.1"}, {ID: ProgZip, Versione: "1.4"}, {ID: ProgPDF, Versione: "1.0"}}}
	got := SenzaPiuNuovi(st, e, []Componente{Gaming, Zip, PDF}, "3.2")
	if len(got) != 1 || got[0] != PDF {
		t.Fatalf("restano %v: Tasso (VERSIONE 3.2) e Zip (1.5) sono più nuovi", got)
	}
	if got := SenzaPiuNuovi(st, e, []Componente{Gaming}, ""); len(got) != 1 {
		t.Fatalf("stessa versione: si reinstalla (%v)", got)
	}
	if got := SenzaPiuNuovi(nil, e, []Componente{Gaming}, ""); len(got) != 1 {
		t.Fatal("prima installazione")
	}
}
