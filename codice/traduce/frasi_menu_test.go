package traduce

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// Controlli sul catalogo delle frasi pronte (dati/frasi-menu.tsv): almeno 5000 frasi, righe
// "inglese<TAB>italiano" pulite e nessuna frase che dopo chiaveFrase diventa uguale a un'altra
// (nella mappa ne resterebbe una a caso).
func TestFrasiMenuFile(t *testing.T) {
	if !utf8.ValidString(frasiMenuTSV) || strings.Contains(frasiMenuTSV, "\r") {
		t.Fatal("frasi-menu.tsv: deve essere UTF-8 senza ritorni a capo di Windows")
	}
	viste := map[string]int{}
	n := 0
	for i, l := range strings.Split(strings.TrimSuffix(frasiMenuTSV, "\n"), "\n") {
		if strings.HasPrefix(l, "#") {
			continue
		}
		en, it, ok := strings.Cut(l, "\t")
		if !ok || strings.Contains(it, "\t") || strings.TrimSpace(en) != en || strings.TrimSpace(it) != it || en == "" || it == "" {
			t.Errorf("riga %d malformata: %q", i+1, l)
			continue
		}
		k := chiaveFrase(en)
		if p, doppia := viste[k]; doppia {
			t.Errorf("riga %d %q: doppione della riga %d", i+1, en, p)
		}
		viste[k] = i + 1
		n++
	}
	if n < 5000 {
		t.Fatalf("frasi pronte: %d, ne servono almeno 5000", n)
	}
}

func TestFrasiMenuEsempi(t *testing.T) {
	for in, atteso := range map[string]string{
		"Quit to Desktop":                         "Esci al desktop",
		"Motion Blur Intensity":                   "Intensità sfocatura di movimento",
		"DLSS Quality":                            "DLSS Qualità",
		"Invert Mouse Y":                          "Inverti asse Y del mouse",
		"Are you sure you want to quit the game?": "Vuoi davvero uscire dal gioco?",
		"Saving...":                               "Salvataggio in corso...",
		"Extraction Point":                        "Punto di estrazione",
		"Your backpack is full!":                  "Il tuo zaino è pieno!",
		"Waiting for host to start":               "In attesa che l'host avvii la partita",
		"Field of View (FOV)":                     "Campo visivo (FOV)",
		"Render Distance: 12":                     "Distanza di rendering: 12",
		"Press E to loot":                         "Premi E per saccheggiare",
		"Press F to open the chest":               "F: apri il forziere",
		"Hold E to harvest":                       "Tieni premuto E per raccogliere",
		"Hold Q to scan object":                   "Tieni premuto Q: scansiona l'oggetto",
	} {
		if got, ok := fraseMenuConNumero(in); !ok || got != atteso {
			t.Errorf("%q -> %q %v, atteso %q", in, got, ok, atteso)
		}
	}
}
