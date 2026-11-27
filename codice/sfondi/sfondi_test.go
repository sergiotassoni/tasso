package sfondi

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestPerTema(t *testing.T) {
	casi := []struct {
		tema    string
		ufficio bool
		atteso  string
	}{
		{"tasso", false, "gaming"}, {"neonrgb", false, "neonrgb"}, {"oro", false, "oro"},
		{"argento", false, "argento"}, {"aurora", false, ""}, {"lava", false, ""}, {"belva", false, ""},
		{"ufficio", true, "nebbia"}, {"ardesia", true, "blunotte"}, {"carta", true, "sabbia"},
		{"salvia", true, ""}, {"lavanda", true, ""},
		{"tasso", true, ""}, {"ardesia", false, ""}, {"", false, ""}, {"nonesiste", true, ""},
	}
	for _, c := range casi {
		if g := PerTema(c.tema, c.ufficio); g != c.atteso {
			t.Errorf("PerTema(%q, %v) = %q, atteso %q", c.tema, c.ufficio, g, c.atteso)
		}
	}
	// Ogni sfondo della mappa esiste, ha un nome, una miniatura e il file 4K da installare.
	for _, m := range []map[string]string{perGaming, perUfficio} {
		for tema, id := range m {
			if !Valido(id) {
				t.Errorf("tema %q: sfondo %q sconosciuto", tema, id)
			}
		}
	}
	for _, id := range Tutti() {
		if Nome(id) == "" || File(id) == "" {
			t.Errorf("sfondo %q senza nome o file", id)
		}
		b := Mini(id)
		if len(b) < 1000 || !bytes.HasPrefix(b, []byte{0xFF, 0xD8}) {
			t.Errorf("miniatura di %q mancante o non JPEG", id)
		}
		if len(b) > 60_000 {
			t.Errorf("miniatura di %q troppo grande: %d byte", id, len(b))
		}
		if !strings.HasPrefix(MiniDataURL(id), "data:image/jpeg;base64,/9j/") {
			t.Errorf("data URL di %q sbagliato", id)
		}
		fi, err := os.Stat("4k/" + File(id))
		if err != nil {
			t.Errorf("manca lo sfondo 4K di %q: %v", id, err)
		} else if fi.Size() > 1_600_000 {
			t.Errorf("sfondo 4K di %q troppo grande: %d byte", id, fi.Size())
		}
	}
	if len(Tutti()) != len(nomi) {
		t.Errorf("Tutti() e nomi non coincidono")
	}
	if File("../x") != "" || Mini("../x") != nil || MiniDataURL("") != "" {
		t.Errorf("ID non valido accettato")
	}
}

func TestDaSalvareERipristino(t *testing.T) {
	app := `C:\Users\Sergio\AppData\Local\Tasso\sfondi`
	prog := `C:\Program Files\Tasso\sfondi`
	// Lo sfondo dell'utente si salva; uno di Tasso no (si tiene quello salvato prima).
	if !DaSalvare(`C:\Users\Sergio\Pictures\mare.jpg`, app, prog) {
		t.Error("lo sfondo dell'utente va salvato")
	}
	if !DaSalvare("", app, prog) {
		t.Error("anche 'nessuna immagine' va salvato (si rimette il colore)")
	}
	if DaSalvare(`c:\users\sergio\appdata\local\tasso\SFONDI\tasso-oro.jpg`, app, prog) {
		t.Error("lo sfondo Oro di Tasso non è 'il tuo sfondo'")
	}
	if DaSalvare(`C:\Program Files\Tasso\sfondi\tasso-gaming.jpg`, app, prog) {
		t.Error("lo sfondo Gaming in Programmi non è 'il tuo sfondo'")
	}
	if !DaSalvare(`C:\Users\Sergio\AppData\Local\Tasso\sfondi-miei\tasso-oro.jpg`, app, prog) {
		t.Error("una cartella che solo comincia con lo stesso nome non è quella di Tasso")
	}
	if !DaSalvare(`C:\Users\Sergio\AppData\Local\Tasso\sfondi\mio-sfondo-prima.jpg`, app, prog) {
		t.Error("la copia del tuo sfondo resta tua")
	}

	esiste := map[string]bool{`C:\Users\Sergio\Pictures\mare.jpg`: true, app + `\mio-sfondo-prima.jpg`: true}
	usa := func(p string) bool { return esiste[p] }
	copia := app + `\mio-sfondo-prima.jpg`
	if p, ok := Ripristino(`C:\Users\Sergio\Pictures\mare.jpg`, copia, usa); !ok || p != `C:\Users\Sergio\Pictures\mare.jpg` {
		t.Errorf("rimetti: atteso l'originale, avuto %q %v", p, ok)
	}
	if p, ok := Ripristino(`C:\Users\Sergio\Pictures\cancellato.jpg`, copia, usa); !ok || p != copia {
		t.Errorf("originale sparito: attesa la copia, avuto %q %v", p, ok)
	}
	if p, ok := Ripristino(`\\server\foto\mare.jpg`, copia, func(string) bool { return true }); !ok || p != copia {
		t.Errorf("percorso di rete: attesa la copia, avuto %q %v", p, ok)
	}
	if p, ok := Ripristino(`C:\Windows\System32\config\SAM`, copia, func(string) bool { return true }); !ok || p != copia {
		t.Errorf("file non immagine: attesa la copia, avuto %q %v", p, ok)
	}
	if p, ok := Ripristino("", copia, usa); !ok || p != "" {
		t.Errorf("nessuna immagine prima: atteso \"\" ok, avuto %q %v", p, ok)
	}
	if _, ok := Ripristino(`D:\sparito.jpg`, `C:\sparita.jpg`, usa); ok {
		t.Error("niente da rimettere: atteso ok=false")
	}
}

func TestSottoELocale(t *testing.T) {
	if !Sotto(`C:\A\B\c.jpg`, `c:\a`) || !Sotto(`C:\A`, `C:\A\`) || Sotto(`C:\AB\c.jpg`, `C:\A`) {
		t.Error("Sotto sbagliata")
	}
	if Sotto(`C:\A\..\Windows\x.jpg`, `C:\A`) || Sotto("", `C:\A`) || Sotto(`C:\A`, "") {
		t.Error("Sotto accetta .. o vuoti")
	}
	for _, p := range []string{`C:\x.jpg`, `d:/f/x.png`, `\\?\C:\x.jpg`} {
		if !Locale(p) {
			t.Errorf("Locale(%q) = false", p)
		}
	}
	for _, p := range []string{`\\srv\x.jpg`, `x.jpg`, `C:x.jpg`, ``, `1:\x`} {
		if Locale(p) {
			t.Errorf("Locale(%q) = true", p)
		}
	}
}

func TestDaChiedere(t *testing.T) {
	if !DaChiedere("oro", "", false, nil) {
		t.Error("la prima volta si chiede")
	}
	if DaChiedere("oro", "", false, []string{"gaming", "oro"}) {
		t.Error("una volta sola per sfondo")
	}
	if DaChiedere("oro", "", true, nil) {
		t.Error("'Non chiedermelo più' vale per sempre")
	}
	if DaChiedere("oro", "oro", false, nil) {
		t.Error("se è già messo non si chiede")
	}
	if DaChiedere("", "", false, nil) || DaChiedere("xyz", "", false, nil) {
		t.Error("niente sfondo: niente domanda")
	}
}
