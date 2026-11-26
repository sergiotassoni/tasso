package installa

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Se tornando indietro un file non si rimette, la copia dei vecchi NON si cancella, e Ripara la
// rimette dopo.
func TestIndietroNonRiuscitoTieneVecchi(t *testing.T) {
	p := nuovaProva(t, "3.0")
	if _, err := p.in.Installa(context.Background(), p.opzioni(TuttiComponenti...)); err != nil {
		t.Fatal(err)
	}
	src, e := pacchetto(t, "3.1")
	p.in.Sorgente, p.in.Elenco = src, e
	tassoExe := filepath.Join(p.base, "Tasso.exe")
	p.in.Rinomina = func(a, b string) error {
		if strings.HasSuffix(a, filepath.Join("Tasso PDF", "TassoPDF.exe")) && strings.Contains(b, cartellaVecchi) {
			return errors.New("file in uso")
		}
		if strings.Contains(a, cartellaVecchi) && b == tassoExe {
			return errors.New("non si rimette")
		}
		return os.Rename(a, b)
	}
	_, err := p.in.Installa(context.Background(), p.opzioni(TuttiComponenti...))
	var enr *ErroreNonRimesso
	if !errors.As(err, &enr) {
		t.Fatalf("atteso ErroreNonRimesso, ho %v", err)
	}
	copia := filepath.Join(p.base, cartellaVecchi, "tasso", "Tasso.exe")
	if leggi(t, copia) != "tasso 3.0" {
		t.Fatal("la copia del Tasso.exe vecchio è stata cancellata")
	}
	if !esiste(filepath.Join(p.base, fileInCorso)) {
		t.Fatal("il segno dell'installazione a metà è sparito")
	}
	// riaprendo l'installatore, Ripara rimette tutto (e solo dopo cancella la copia)
	p.in.Rinomina = nil
	if err := p.in.Ripara(p.base); err != nil {
		t.Fatal(err)
	}
	if leggi(t, tassoExe) != "tasso 3.0" || esiste(filepath.Join(p.base, cartellaVecchi)) || esiste(filepath.Join(p.base, fileInCorso)) {
		t.Fatal("Ripara non ha rimesso Tasso.exe")
	}
}

// Ripara che non riesce: la copia resta e non si installa sopra.
func TestRiparaNonRiuscitoNonInstallaSopra(t *testing.T) {
	p := nuovaProva(t, "3.0")
	if _, err := p.in.Installa(context.Background(), p.opzioni(Gaming)); err != nil {
		t.Fatal(err)
	}
	v := filepath.Join(p.base, cartellaVecchi, "tasso", "Tasso.exe")
	os.MkdirAll(filepath.Dir(v), 0o755)
	os.Rename(filepath.Join(p.base, "Tasso.exe"), v)
	os.WriteFile(filepath.Join(p.base, fileInCorso), nil, 0o644)
	os.MkdirAll(filepath.Join(p.base, cartellaNuovi, "tasso"), 0o755) // fase 2 a metà: i nuovi ci sono ancora
	p.in.Rinomina = func(a, b string) error { return errors.New("bloccato") }
	src, e := pacchetto(t, "3.1")
	p.in.Sorgente, p.in.Elenco = src, e
	if _, err := p.in.Installa(context.Background(), p.opzioni(Gaming)); !errors.Is(err, ErrRiparaNonRiuscito) {
		t.Fatalf("atteso ErrRiparaNonRiuscito, ho %v", err)
	}
	if leggi(t, v) != "tasso 3.0" {
		t.Fatal("la copia è stata toccata")
	}
}

// PC spento a metà della fase 2: tornano TUTTI i file vecchi, anche quelli già sostituiti e
// quelli rinominati accanto (Tasso.exe.vecchio-1).
func TestRiparaFase2Interrotta(t *testing.T) {
	p := nuovaProva(t, "3.0")
	if _, err := p.in.Installa(context.Background(), p.opzioni(TuttiComponenti...)); err != nil {
		t.Fatal(err)
	}
	b := p.base
	// TassoZip.exe già sostituito (il vecchio è nella copia)
	zip := filepath.Join(b, "Tasso Zip", "TassoZip.exe")
	vz := filepath.Join(b, cartellaVecchi, "zip", "TassoZip.exe")
	os.MkdirAll(filepath.Dir(vz), 0o755)
	os.Rename(zip, vz)
	os.WriteFile(zip, []byte("zip 3.1"), 0o644)
	// Tasso.exe occupato: rinominato accanto, il nuovo non ancora messo
	exe := filepath.Join(b, "Tasso.exe")
	lato := exe + ".vecchio-1"
	os.Rename(exe, lato)
	// TassoPDF.exe occupato: rinominato accanto, il nuovo già messo
	pdf := filepath.Join(b, "Tasso PDF", "TassoPDF.exe")
	latoPdf := pdf + ".vecchio-1"
	os.Rename(pdf, latoPdf)
	os.WriteFile(pdf, []byte("pdf 3.1"), 0o644)
	os.WriteFile(filepath.Join(b, fileInCorso), []byte(lato+"\t"+exe+"\n"+latoPdf+"\t"+pdf+"\n"), 0o644)
	os.MkdirAll(filepath.Join(b, cartellaNuovi, "tasso"), 0o755)
	if err := p.in.Ripara(b); err != nil {
		t.Fatal(err)
	}
	if leggi(t, zip) != "zip 3.0" || leggi(t, exe) != "tasso 3.0" || leggi(t, pdf) != "pdf 3.0" {
		t.Fatal("non tutti i file vecchi sono tornati")
	}
	for _, x := range []string{lato, latoPdf, filepath.Join(b, cartellaVecchi), filepath.Join(b, cartellaNuovi), filepath.Join(b, fileInCorso)} {
		if esiste(x) {
			t.Fatalf("resta %s", x)
		}
	}
}

// Installazione finita ma la copia dei vecchi è rimasta (file occupato): Ripara la cancella e
// non rimette i file che la versione nuova ha tolto.
func TestRiparaDopoInstallazioneFinita(t *testing.T) {
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(Gaming)); err != nil {
		t.Fatal(err)
	}
	v := filepath.Join(p.base, cartellaVecchi, "tasso", "vecchio.dll")
	os.MkdirAll(filepath.Dir(v), 0o755)
	os.WriteFile(v, []byte("x"), 0o644)
	if err := p.in.Ripara(p.base); err != nil {
		t.Fatal(err)
	}
	if esiste(filepath.Join(p.base, "vecchio.dll")) || esiste(filepath.Join(p.base, cartellaVecchi)) {
		t.Fatal("Ripara ha rimesso un file tolto dalla versione nuova")
	}
}

func TestInstallaFinitaSenzaSegno(t *testing.T) {
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(TuttiComponenti...)); err != nil {
		t.Fatal(err)
	}
	if esiste(filepath.Join(p.base, fileInCorso)) {
		t.Fatal("il segno resta dopo un'installazione riuscita")
	}
}

func TestPreparaCartella(t *testing.T) {
	p := nuovaProva(t, "3.1")
	var preparate []string
	p.in.PreparaCartella = func(b string) (bool, error) {
		preparate = append(preparate, b)
		return true, os.MkdirAll(b, 0o755)
	}
	if _, err := p.in.Installa(context.Background(), p.opzioni(Gaming)); err != nil {
		t.Fatal(err)
	}
	if len(preparate) != 1 || preparate[0] != p.base {
		t.Fatalf("cartella preparata: %v", preparate)
	}
	// cartella non sicura: non si installa e non si crea niente
	q := nuovaProva(t, "3.1")
	q.in.PreparaCartella = func(string) (bool, error) { return false, &ErroreCartella{ErrCartNonSicura} }
	_, err := q.in.Installa(context.Background(), q.opzioni(Gaming))
	var ec *ErroreCartella
	if !errors.As(err, &ec) || ec.Codice != ErrCartNonSicura || esiste(q.base) {
		t.Fatalf("errore %v, cartella rimasta %v", err, esiste(q.base))
	}
}

// Segno rimasto dopo un'installazione finita (non si era cancellato) e un Tasso.exe.vecchio-1
// ancora lì (si cancella al riavvio): Ripara NON rimette la versione vecchia sopra la nuova.
func TestRiparaSegnoRimastoDopoFine(t *testing.T) {
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(Gaming)); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(p.base, "Tasso.exe")
	lato := exe + ".vecchio-1"
	os.WriteFile(lato, []byte("tasso 3.0"), 0o644)
	os.WriteFile(filepath.Join(p.base, fileInCorso), []byte(lato+"\t"+exe+"\n"), 0o644)
	if err := p.in.Ripara(p.base); err != nil {
		t.Fatal(err)
	}
	if leggi(t, exe) != "tasso 3.1" {
		t.Fatal("Ripara ha rimesso la versione vecchia")
	}
	if esiste(filepath.Join(p.base, fileInCorso)) {
		t.Fatal("il segno resta")
	}
}

// Installazione riuscita: la cartella dei nuovi sparisce prima del segno (P1): se il PC si
// spegne dopo, Ripara non rimette i file tolti dalla versione nuova.
func TestNuoviToltiPrimaDelSegno(t *testing.T) {
	p := nuovaProva(t, "3.1")
	visto := false
	p.in.CreaCollegamento = func(Collegamento) error {
		// durante i collegamenti (dopo il registro) .tasso-setup-nuovo non c'è già più
		if esiste(filepath.Join(p.base, cartellaNuovi)) {
			t.Error("cartella dei nuovi ancora lì durante i collegamenti")
		}
		visto = true
		return nil
	}
	if _, err := p.in.Installa(context.Background(), p.opzioni(Gaming)); err != nil {
		t.Fatal(err)
	}
	if !visto {
		t.Skip("nessun collegamento creato in questa prova")
	}
}

// PC spento mentre si cancellava la cartella dei nuovi: con la rinomina atomica resta solo
// .tasso-setup-via accanto al segno. Ripara la toglie e NON rimette i file vecchi.
func TestRiparaScartoDeiNuoviRimasto(t *testing.T) {
	p := nuovaProva(t, "3.1")
	if _, err := p.in.Installa(context.Background(), p.opzioni(Gaming)); err != nil {
		t.Fatal(err)
	}
	if esiste(filepath.Join(p.base, cartellaScarto)) || esiste(filepath.Join(p.base, cartellaNuovi)) {
		t.Fatal("installazione finita: restano le cartelle di lavoro dell'installatore")
	}
	exe := filepath.Join(p.base, "Tasso.exe")
	scarto := filepath.Join(p.base, cartellaScarto)
	os.MkdirAll(filepath.Join(scarto, "gaming"), 0o755)
	os.WriteFile(filepath.Join(scarto, "gaming", "pezzo.bin"), []byte("x"), 0o644)
	vecchi := filepath.Join(p.base, cartellaVecchi)
	os.MkdirAll(vecchi, 0o755)
	os.WriteFile(filepath.Join(vecchi, "Tasso.exe"), []byte("tasso 3.0"), 0o644)
	os.WriteFile(filepath.Join(p.base, fileInCorso), nil, 0o644)
	if err := p.in.Ripara(p.base); err != nil {
		t.Fatal(err)
	}
	if leggi(t, exe) != "tasso 3.1" {
		t.Fatal("Ripara ha rimesso la versione vecchia sopra un'installazione finita")
	}
	if esiste(scarto) || esiste(filepath.Join(p.base, fileInCorso)) {
		t.Fatal("restano lo scarto o il segno")
	}
}
