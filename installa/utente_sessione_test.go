package installa

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// UAC con un altro account: le tracce di Zip (menu del tasto destro, dati) sono dell'utente della
// sessione e vanno tolte da lì; la lingua si scrive (e si toglie) per tutti e due.
func TestUtenteDiversoDallAmministratore(t *testing.T) {
	p := nuovaProva(t, "3.1")
	utente := NuovoRegistroMemoria()
	datiUtente, datiAdmin := t.TempDir(), t.TempDir()
	p.in.RegistroUtente = utente
	p.in.CartellaDatiSessione = func() string { return datiUtente }
	p.in.CartellaDatiUtente = func() string { return datiAdmin }
	var conUtente []string // R3b-1: la cartella dell'utente si toglie solo con i suoi permessi
	p.in.TogliCartellaSessione = func(d string) error { conUtente = append(conUtente, d); return os.RemoveAll(d) }
	if _, err := p.in.Installa(context.Background(), p.opzioni(Zip)); err != nil {
		t.Fatal(err)
	}
	if v, _ := utente.LeggiTesto(HKCU, ChiaveTasso, "Lingua"); v != "it" {
		t.Fatal("lingua non scritta per l'utente della sessione")
	}
	exe := filepath.Join(p.base, "Tasso Zip", "TassoZip.exe")
	k := `Software\Classes\*\shell\TassoZip`
	utente.ScriviTesto(HKCU, k+`\shell\1comprimi\command`, "", `"`+exe+`" comprimi "%1"`)
	os.MkdirAll(filepath.Join(datiUtente, "Tasso Zip"), 0o755)
	os.MkdirAll(filepath.Join(datiAdmin, "Tasso Zip"), 0o755)
	if _, err := p.in.Disinstalla(p.base, []Componente{Zip}); err != nil {
		t.Fatal(err)
	}
	if !utente.Vuota(HKCU, k) || esiste(filepath.Join(datiUtente, "Tasso Zip")) {
		t.Fatal("tracce di Zip dell'utente della sessione rimaste")
	}
	if !esiste(filepath.Join(datiAdmin, "Tasso Zip")) {
		t.Fatal("tolta la cartella dell'amministratore al posto di quella dell'utente")
	}
	if len(conUtente) != 1 || conUtente[0] != filepath.Join(datiUtente, "Tasso Zip") {
		t.Fatalf("cartella dell'utente non tolta con i suoi permessi: %q", conUtente)
	}
	if _, ok := utente.LeggiTesto(HKCU, ChiaveTasso, "Lingua"); ok {
		t.Fatal("lingua dell'utente rimasta")
	}
}

// RECENSIONE-3b, R3b-1: senza il modo di cancellare con i permessi dell'utente della sessione, la
// sua cartella dati NON si cancella da amministratore (il percorso lo sceglie lui, dal suo HKCU):
// resta, e la disinstallazione lo dice. Anche se TogliCartellaSessione dà errore (per esempio
// perché la giunzione porta in una cartella che l'utente non può toccare), non si riprova da
// amministratore.
func TestCartellaSessioneMaiDaAmministratore(t *testing.T) {
	for _, conFunzione := range []bool{false, true} {
		p := nuovaProva(t, "3.1")
		p.in.RegistroUtente = NuovoRegistroMemoria()
		datiUtente := t.TempDir()
		p.in.CartellaDatiSessione = func() string { return datiUtente }
		chiamate := 0
		if conFunzione {
			p.in.TogliCartellaSessione = func(string) error { chiamate++; return os.ErrPermission }
		}
		if _, err := p.in.Installa(context.Background(), p.opzioni(Zip, PDF)); err != nil {
			t.Fatal(err)
		}
		dz, dp := filepath.Join(datiUtente, "Tasso Zip"), filepath.Join(datiUtente, "Tasso PDF")
		os.MkdirAll(filepath.Join(dz, "Temp"), 0o755)
		os.MkdirAll(dp, 0o755)
		os.WriteFile(filepath.Join(dz, "impostazioni.json"), []byte("{}"), 0o644)
		ris, err := p.in.Disinstalla(p.base, []Componente{Zip, PDF})
		if ris == nil {
			t.Fatalf("disinstallazione: %v", err)
		}
		detti := strings.Join(ris.NonTolti, "|")
		if !strings.Contains(detti, dz) || !strings.Contains(detti, dp) {
			t.Fatalf("con funzione=%v: le cartelle rimaste non sono dette: %q", conFunzione, ris.NonTolti)
		}
		if !esiste(dz) || !esiste(filepath.Join(dz, "impostazioni.json")) || !esiste(dp) {
			t.Fatalf("con funzione=%v: cartella dell'utente cancellata da amministratore", conFunzione)
		}
		if conFunzione && chiamate != 2 {
			t.Fatalf("TogliCartellaSessione chiamata %d volte, attese 2", chiamate)
		}
	}
}
