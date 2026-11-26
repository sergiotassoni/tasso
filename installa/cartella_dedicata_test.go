package installa

import (
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
