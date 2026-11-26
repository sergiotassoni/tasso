package installa

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCartellaDedicata(t *testing.T) {
	info := map[string]InfoCartella{
		`C:\Program Files`:       {Esiste: true},
		`D:\Giochi`:              {Esiste: true},
		`C:\Program Files\Tasso`: {Esiste: true, DiTasso: true},
		`D:\Tasso`:               {Esiste: true}, // una cartella "Tasso" dell'utente, piena di altro
		`E:\Tasso`:               {Esiste: true, Vuota: true},
		`F:\Mio\TASSO`:           {},
		`D:\Strumenti\Vecchio`:   {Esiste: true, DiTasso: true}, // installazione di prima: resta lì
	}
	f := func(p string) InfoCartella { return info[p] }
	casi := map[string]string{
		`C:\Program Files`:       `C:\Program Files\Tasso`,
		`C:\Program Files\`:      `C:\Program Files\Tasso`,
		`D:\Giochi`:              `D:\Giochi\Tasso`,
		`C:\Program Files\Tasso`: `C:\Program Files\Tasso`,
		`D:\Tasso`:               `D:\Tasso\Tasso`,
		`E:\Tasso`:               `E:\Tasso`,
		`F:\Mio\TASSO`:           `F:\Mio\TASSO`,
		`D:\Strumenti\Vecchio`:   `D:\Strumenti\Vecchio`,
		`G:\Nuova\Cartella`:      `G:\Nuova\Cartella\Tasso`,
	}
	for in, want := range casi {
		if got := CartellaDedicata(in, f); got != want {
			t.Errorf("CartellaDedicata(%q) = %q, voglio %q", in, got, want)
		}
	}
}

func TestInfoCartellaDisco(t *testing.T) {
	d := t.TempDir()
	if i := InfoCartellaDisco(filepath.Join(d, "manca")); i.Esiste {
		t.Fatal("una cartella che non c'è risulta esistente")
	}
	if i := InfoCartellaDisco(d); !i.Esiste || !i.Vuota || i.DiTasso {
		t.Fatalf("cartella vuota: %+v", i)
	}
	os.WriteFile(filepath.Join(d, "altro.txt"), []byte("x"), 0o644)
	if i := InfoCartellaDisco(d); i.Vuota || i.DiTasso {
		t.Fatalf("cartella con altro: %+v", i)
	}
	os.WriteFile(filepath.Join(d, FileStato), []byte("{}"), 0o644)
	if i := InfoCartellaDisco(d); !i.DiTasso {
		t.Fatalf("cartella di Tasso non riconosciuta: %+v", i)
	}
}

func TestDecidiCartellaSicura(t *testing.T) {
	casi := []struct {
		s    SicurezzaCartella
		crea bool
		ok   bool
	}{
		{SicurezzaCartella{AntenatiAdmin: true}, true, true},          // nuova sotto D:\ o C:\Programmi miei
		{SicurezzaCartella{}, false, false},                           // nuova dentro una cartella di un utente
		{SicurezzaCartella{Esiste: true, DiTasso: true}, false, true}, // aggiornamento
		{SicurezzaCartella{Esiste: true, Vuota: true, ProprietarioAdmin: true, AntenatiAdmin: true}, false, true},
		{SicurezzaCartella{Esiste: true, Vuota: true, AntenatiAdmin: true}, false, false},             // "Tasso" creata prima da un utente
		{SicurezzaCartella{Esiste: true, ProprietarioAdmin: true, AntenatiAdmin: true}, false, false}, // non vuota
		{SicurezzaCartella{Esiste: true, DiTasso: true, Collegamento: true}, false, false},            // giunzione
	}
	for i, c := range casi {
		crea, err := DecidiCartellaSicura(c.s)
		if crea != c.crea || (err == nil) != c.ok {
			t.Errorf("%d: crea %v err %v", i, crea, err)
		}
		if err != nil {
			var ec *ErroreCartella
			if !errors.As(err, &ec) || ec.Codice != ErrCartNonSicura {
				t.Errorf("%d: codice %v", i, err)
			}
		}
	}
}

// Disco senza permessi (FAT32, exFAT): cartella nuova, vuota o già di Tasso sì; un'altra cartella
// con dentro altro no, con il messaggio del disco senza permessi. Una giunzione mai.
func TestCartellaSuDiscoSenzaPermessi(t *testing.T) {
	for _, c := range []struct {
		s    SicurezzaCartella
		crea bool
		err  string
	}{
		{SicurezzaCartella{SenzaPermessi: true}, true, ""},
		{SicurezzaCartella{SenzaPermessi: true, Esiste: true, Vuota: true}, false, ""},
		{SicurezzaCartella{SenzaPermessi: true, Esiste: true, DiTasso: true}, false, ""},
		{SicurezzaCartella{SenzaPermessi: true, Esiste: true}, false, ErrCartSenzaPermessi},
		{SicurezzaCartella{SenzaPermessi: true, Esiste: true, Vuota: true, Collegamento: true}, false, ErrCartNonSicura},
		// NTFS: come prima
		{SicurezzaCartella{Esiste: true, Vuota: true}, false, ErrCartNonSicura},
	} {
		crea, err := DecidiCartellaSicura(c.s)
		codice := ""
		if e, ok := err.(*ErroreCartella); ok {
			codice = e.Codice
		}
		if crea != c.crea || codice != c.err {
			t.Errorf("%+v: crea=%v errore=%q, volevo crea=%v errore=%q", c.s, crea, codice, c.crea, c.err)
		}
	}
}
