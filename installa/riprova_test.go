package installa

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// errOccupato: ERROR_SHARING_VIOLATION come lo dà Windows dentro os.Rename.
func errOccupato(a, b string) error {
	return &os.LinkError{Op: "rename", Old: a, New: b, Err: syscall.Errno(32)}
}

func TestBloccatoWindows(t *testing.T) {
	for _, c := range []uintptr{5, 32, 33, 1224} {
		if !BloccatoWindows(&os.PathError{Op: "open", Path: "x", Err: syscall.Errno(c)}) {
			t.Errorf("codice %d doveva essere \"occupato\"", c)
		}
	}
	for _, e := range []error{nil, errors.New("file in uso"), syscall.Errno(2), &os.PathError{Err: syscall.Errno(183)}} {
		if BloccatoWindows(e) {
			t.Errorf("%v non è \"occupato\"", e)
		}
	}
}

func TestRiprova(t *testing.T) {
	var pause []time.Duration
	dormi := func(d time.Duration) { pause = append(pause, d) }
	occ := syscall.Errno(32)

	// occupato 3 volte, poi va
	n := 0
	tent, err := Riprova(func() error {
		if n++; n <= 3 {
			return occ
		}
		return nil
	}, BloccatoWindows, dormi, 60, PausaRiprova)
	if err != nil || tent != 4 || len(pause) != 3 || pause[0] != PausaRiprova {
		t.Fatalf("tentativi %d, pause %v, errore %v", tent, pause, err)
	}
	// un altro errore: nessun nuovo tentativo
	pause = nil
	tent, err = Riprova(func() error { return errors.New("disco pieno") }, BloccatoWindows, dormi, 60, PausaRiprova)
	if err == nil || tent != 1 || len(pause) != 0 {
		t.Fatalf("errore non da riprovare: tentativi %d, pause %d", tent, len(pause))
	}
	// sempre occupato: si ferma dopo 60 tentativi (15 secondi) con l'errore vero
	pause = nil
	tent, err = Riprova(func() error { return occ }, BloccatoWindows, dormi, TentativiRiprova, PausaRiprova)
	if !errors.Is(err, occ) || tent != TentativiRiprova || len(pause) != TentativiRiprova-1 {
		t.Fatalf("tentativi %d, pause %d, errore %v", tent, len(pause), err)
	}
	if tot := time.Duration(len(pause)) * PausaRiprova; tot < 14*time.Second || tot > 16*time.Second {
		t.Fatalf("attesa totale %v invece di circa 15 secondi", tot)
	}
}

// provaOccupata: Tasso 3.0 installato, poi l'aggiornamento 3.1 con Rinomina finta.
func provaOccupata(t *testing.T, rinomina func(a, b string) error) (*prova, *int) {
	p := nuovaProva(t, "3.0")
	if _, err := p.in.Installa(context.Background(), p.opzioni(TuttiComponenti...)); err != nil {
		t.Fatal(err)
	}
	src, e := pacchetto(t, "3.1")
	pause := 0
	p.in.Sorgente, p.in.Elenco = src, e
	p.in.Bloccato = BloccatoWindows
	p.in.Dormi = func(time.Duration) { pause++ }
	p.in.Rinomina = rinomina
	return p, &pause
}

func vaInVecchi(a, b string) bool {
	return strings.HasSuffix(a, string(filepath.Separator)+"Tasso.exe") && strings.Contains(b, cartellaVecchi)
}

// Tasso.exe appena chiuso: occupato per 5 tentativi, poi si libera e l'aggiornamento riesce.
func TestFileOccupatoPerPocoRiprova(t *testing.T) {
	volte := 0
	p, pause := provaOccupata(t, func(a, b string) error {
		if vaInVecchi(a, b) {
			if volte++; volte <= 5 {
				return errOccupato(a, b)
			}
		}
		return os.Rename(a, b)
	})
	if _, err := p.in.Installa(context.Background(), p.opzioni(TuttiComponenti...)); err != nil {
		t.Fatal(err)
	}
	if *pause != 5 || leggi(t, filepath.Join(p.base, "Tasso.exe")) != "tasso 3.1" {
		t.Fatalf("pause %d, Tasso.exe %q", *pause, leggi(t, filepath.Join(p.base, "Tasso.exe")))
	}
}

// Tasso.exe sempre occupato: dopo 15 secondi lo rinomino accanto (Tasso.exe.vecchio-1), metto il
// nuovo e alla fine cancello quello vecchio.
func TestFileSempreOccupatoRinominaAccanto(t *testing.T) {
	var accanto string
	p, pause := provaOccupata(t, func(a, b string) error {
		if vaInVecchi(a, b) {
			return errOccupato(a, b)
		}
		if strings.Contains(b, ".vecchio-") {
			accanto = b
		}
		return os.Rename(a, b)
	})
	var alRiavvio []string
	p.in.CancellaAlRiavvio = func(f string) error { alRiavvio = append(alRiavvio, f); return nil }
	// un Tasso.exe.vecchio-1 di prima c'è già: si usa il numero dopo
	os.WriteFile(filepath.Join(p.base, "Tasso.exe.vecchio-1"), []byte("vecchissimo"), 0o644)
	if _, err := p.in.Installa(context.Background(), p.opzioni(TuttiComponenti...)); err != nil {
		t.Fatal(err)
	}
	if *pause != TentativiRiprova-1 {
		t.Fatalf("pause %d", *pause)
	}
	if accanto != filepath.Join(p.base, "Tasso.exe.vecchio-2") {
		t.Fatalf("rinominato in %q", accanto)
	}
	if leggi(t, filepath.Join(p.base, "Tasso.exe")) != "tasso 3.1" || leggi(t, filepath.Join(p.base, "Tasso Zip", "TassoZip.exe")) != "zip 3.1" {
		t.Fatal("aggiornamento non fatto")
	}
	if esiste(accanto) || esiste(filepath.Join(p.base, cartellaVecchi)) || len(alRiavvio) != 0 {
		t.Fatalf("resti: accanto %v, al riavvio %v", esiste(accanto), alRiavvio)
	}
}

// Se dopo la rinomina accanto qualcosa va storto, si torna indietro anche da lì.
func TestRinominaAccantoTornaIndietro(t *testing.T) {
	p, _ := provaOccupata(t, func(a, b string) error {
		if vaInVecchi(a, b) {
			return errOccupato(a, b)
		}
		if strings.HasSuffix(a, "TassoPDF.exe") && strings.Contains(b, cartellaVecchi) {
			return errors.New("disco rotto")
		}
		return os.Rename(a, b)
	})
	_, err := p.in.Installa(context.Background(), p.opzioni(TuttiComponenti...))
	if err == nil || !strings.Contains(err.Error(), "disco rotto") {
		t.Fatalf("errore %v", err)
	}
	if leggi(t, filepath.Join(p.base, "Tasso.exe")) != "tasso 3.0" || esiste(filepath.Join(p.base, "Tasso.exe.vecchio-1")) {
		t.Fatal("Tasso.exe non rimesso com'era")
	}
}

// Occupato anche per la rinomina accanto: errore chiaro (ErrFileBloccato col nome del file) e niente cambiato.
func TestFileBloccatoErroreChiaro(t *testing.T) {
	p, _ := provaOccupata(t, func(a, b string) error {
		if strings.HasSuffix(a, string(filepath.Separator)+"Tasso.exe") && !strings.Contains(a, cartellaNuovi) {
			return errOccupato(a, b)
		}
		return os.Rename(a, b)
	})
	_, err := p.in.Installa(context.Background(), p.opzioni(TuttiComponenti...))
	var eb *ErroreBloccato
	if !errors.Is(err, ErrFileBloccato) || !errors.As(err, &eb) || eb.File != filepath.Join(p.base, "Tasso.exe") {
		t.Fatalf("errore %v", err)
	}
	if leggi(t, filepath.Join(p.base, "Tasso.exe")) != "tasso 3.0" || esiste(filepath.Join(p.base, cartellaNuovi)) {
		t.Fatal("doveva restare tutto com'era")
	}
	if v, _ := p.reg.LeggiTesto(HKLM, ChiaveDisinstalla, "DisplayVersion"); v != "3.0" {
		t.Fatalf("registro cambiato: %q", v)
	}
}

func TestDiario(t *testing.T) {
	f := filepath.Join(t.TempDir(), NomeDiario)
	ImpostaDiario(f)
	defer ImpostaDiario("")
	Diario("primo %s", "passo")
	Diario("errore: %v", errors.New("riga1\nriga2"))
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	righe := strings.Split(strings.TrimSpace(string(b)), "\r\n")
	if len(righe) != 2 || !strings.HasSuffix(righe[0], "primo passo") || !strings.HasSuffix(righe[1], "errore: riga1 riga2") ||
		!strings.HasPrefix(righe[0], time.Now().Format("2006-01-02")) {
		t.Fatalf("diario: %q", b)
	}
}
