package installa

import (
	"context"
	"os"
	"path/filepath"
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
	if _, ok := utente.LeggiTesto(HKCU, ChiaveTasso, "Lingua"); ok {
		t.Fatal("lingua dell'utente rimasta")
	}
}
