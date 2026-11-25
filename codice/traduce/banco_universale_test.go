package traduce

// Casi presi dal banco di prova su 60 schermate (47 finte di tanti generi + foto vere), con le
// letture vere dell'OCR di Windows: righe giuste che venivano scartate per niente, righe di due
// voci unite, spazzatura tradotta. Ogni controllo è generale, per tutti i giochi.

import (
	"context"
	"image/color"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Riquadro della pixel art: il bordo del riquadro accanto alla scritta (una linea che continua
// sopra e sotto) non è una parola non letta.
func TestScrittaContinuaBordoRiquadro(t *testing.T) {
	sf, ink := color.RGBA{0, 0, 40, 255}, color.RGBA{240, 240, 240, 255}
	img := immagineFinta(900, 300, sf)
	// bordo bianco verticale del riquadro, da y=20 a y=280, a 28 px dalla scritta
	for y := 20; y < 280; y++ {
		for x := 40; x < 48; x++ {
			img.SetRGBA(x, y, ink)
		}
	}
	fine := parolaFinta(img, 72, 120, 8, 6, 30, ink) // lettere sottili con spazi (pixel art)
	b := Blocco{Testo: "Press SPACE to skip", X: 72, Y: 120, W: fine - 72, H: 30, Em: 34, Sfondo: sf, Inchiostro: ink}
	if scrittaContinua(img, b) {
		t.Fatal("il bordo del riquadro preso per una parola non letta")
	}
}

// Zona piena dello stesso colore accanto alla scritta (cielo chiaro accanto a lettere bianche,
// fondo scuro accanto al contorno scuro): la riga finisce lì.
func TestScrittaContinuaZonaPiena(t *testing.T) {
	sf, ink := color.RGBA{30, 60, 30, 255}, color.RGBA{235, 235, 235, 255}
	img := immagineFinta(900, 300, sf)
	fine := parolaFinta(img, 100, 120, 10, 6, 30, ink)
	for y := 60; y < 250; y++ { // un edificio chiaro dietro, subito dopo la scritta
		for x := fine + 8; x < fine+200; x++ {
			img.SetRGBA(x, y, color.RGBA{225, 228, 230, 255})
		}
	}
	b := Blocco{Testo: "KEEP OUT", X: 100, Y: 120, W: fine - 100, H: 30, Em: 34, Sfondo: sf, Inchiostro: ink}
	if scrittaContinua(img, b) {
		t.Fatal("una zona piena accanto presa per una parola non letta")
	}
}

// Caratteri sottili (tratti di un pixel, liste fitte): la parola non letta a sinistra
// ("Your" prima di "wound is infected") si vede lo stesso.
func TestScrittaContinuaTrattiSottili(t *testing.T) {
	sf, ink := color.RGBA{5, 5, 13, 255}, color.RGBA{228, 228, 229, 255}
	img := immagineFinta(600, 100, sf)
	for _, x := range []int{60, 66, 71, 76} { // aste di un pixel: la parola non letta
		for y := 40; y < 56; y++ {
			img.SetRGBA(x, y, ink)
		}
	}
	fine := parolaFinta(img, 86, 40, 12, 1, 16, ink)
	b := Blocco{Testo: "wound is infected.", X: 86, Y: 40, W: fine - 86, H: 16, Em: 20, Sfondo: sf, Inchiostro: ink}
	if !scrittaContinua(img, b) {
		t.Fatal("parola sottile non letta a sinistra non vista")
	}
}

// Letture vere dell'OCR di Windows: il voto di affidabilità.
func TestAffidabilitaLettureBanco(t *testing.T) {
	for _, c := range []struct {
		letto, corretto string
		buona           bool
	}{
		{"Low health! Find a medkit", "Low health! Find a medkit", true}, // una parola da gioco sconosciuta
		{"Low health!find a medkit", "Low health! find a medkit", true},  // spazio perso dopo "!"
		{"MARCUS: I don't trust him. He knows more", "", true},           // nome di chi parla
		{"OLD MAN: The bridge collapsed years ago. You'll need another way across.", "", true},
		{"Item collected: Rusty Key", "", true},
		{"FIP: Crojucning rnakes 123, noise when enemies are earby.", "", false}, // spazzatura
		{"DOCTOR:Nour woundäsinfected. You need to rest for a while.", "", false},
		{"nettling is rnoving in the baserr ent.", "", false},
		{"FELS", "", false},
		{"Hold| to opel tie aoc", "", false},
		{"Find a rnedkit", "Find a medkit", false}, // "rn" per "m": riparata, ma corta con una parola sconosciuta (come prima)
		{"Find a rnedklt", "", false},              // due confusioni: lettura sbagliata, non una parola nuova
		// parole attaccate dall'OCR con una lettera confusa ("m" letta "fn"): riparate, la frase è buona
		{"MARCUS: I don't trusthifn. He knows more than he's telling us.", "MARCUS: I don't trust him. He knows more than he's telling us.", true},
	} {
		got, v := correggiRigaOCR(c.letto)
		if c.corretto != "" && got != c.corretto {
			t.Errorf("%q corretto in %q, atteso %q", c.letto, got, c.corretto)
		}
		if (v != letturaDubbia) != c.buona {
			t.Errorf("%q: voto %d, buona attesa %v", c.letto, v, c.buona)
		}
	}
}

// "%" letto "0/0": "800/0" è "80%" (una divisione per zero non è mai una scritta vera).
func TestPercentualeLettaMale(t *testing.T) {
	for in, out := range map[string]string{
		"< 800/0 >": "< 80% >", "800/0": "80%", "Volume 450/0": "Volume 45%", "12.50/0": "12.5%",
		"1000/0": "100%", "100o/o": "100%", "24 / 120": "24 / 120", "3/10": "3/10", "10/05": "10/05",
	} {
		if got := correggiPercentuale(in); got != out {
			t.Errorf("%q -> %q, atteso %q", in, got, out)
		}
	}
}

// Traduzioni giuste che erano scartate come "mezze in inglese": prestiti dei giochi, tasti,
// nomi di oggetti dopo un'etichetta. Le righe mezze vere restano scartate.
func TestMezzaTradottaPrestitiENomi(t *testing.T) {
	for _, c := range [][2]string{
		{"Exit to Desktop", "Esci al desktop"},
		{"Find Lobby", "Trova lobby"},
		{"Master Volume", "Volume principale"},
		{"Checkpoint reached", "Checkpoint raggiunto"},
		{"Item collected: Rusty Key", "Oggetto raccolto: Rusty Key"},
		{"Quest completed: Old Friends", "Missione completata: Old Friends"},
		{"TIP: You can sprint by holding the Shift key.", "SUGGERIMENTO: Puoi scattare tenendo premuto il tasto Shift."},
		{"Apply Reset to Default Back", "Applica Reset a Default Indietro"},
		{"Low health! Find a medkit", "Salute bassa! Trova un medkit"},
	} {
		if rigaMezzaTradotta(c[0], c[1]) {
			t.Errorf("%q -> %q: non è mezza in inglese", c[0], c[1])
		}
	}
	for _, c := range [][2]string{
		{"Open the vault", "Apri il vault"},
		{"Move to fill the progress bar", "Move per riempire la barra"},
		{"Item collected: Rusty Key", "Oggetto collected: Rusty Key"},
		{"Press SPACE to skip", "Premi SPACE to skip"},
	} {
		if !rigaMezzaTradotta(c[0], c[1]) {
			t.Errorf("%q -> %q: è mezza in inglese", c[0], c[1])
		}
	}
	// le frasi pronte (scritte a mano) non si scartano mai come mezze
	b := Blocco{Testo: "Exit to Desktop"}
	if m := motivoScartaTraduzione(b, Risultato{Tradotto: "Esci al desktop", Motore: "catalogo"}); m != "" {
		t.Errorf("frase pronta scartata: %s", m)
	}
	if m := motivoScartaTraduzione(Blocco{Testo: "Video"}, Risultato{Tradotto: "Video", Motore: "catalogo"}); m != motivoUgualeIt {
		t.Errorf("parola uguale in italiano: %q", m)
	}
}

// Valori delle impostazioni: "< Off >", "◀ English ▶", "<No>": le frecce si tolgono (restano
// del gioco, come scritte a parte), il valore si traduce; numeri e sigle restano.
func TestFrecceSelettore(t *testing.T) {
	righe := []RigaOCR{
		{Parole: []ParolaOCR{par("<", 1300, 300, 14, 26), par("Off", 1322, 300, 40, 26), par(">", 1370, 300, 14, 26)}},
		{Parole: []ParolaOCR{par("<No>", 1300, 400, 70, 26)}},
		{Parole: []ParolaOCR{par("◀", 1250, 500, 20, 26), par("English", 1280, 500, 100, 26), par("▶", 1390, 500, 20, 26)}},
		{Parole: []ParolaOCR{par("<", 1200, 600, 14, 26), par("1920", 1222, 600, 60, 26), par("x", 1288, 600, 12, 26), par("1080", 1306, 600, 60, 26), par(">", 1372, 600, 14, 26)}},
		{Parole: []ParolaOCR{par("<", 1300, 700, 14, 26), par("800/0", 1322, 700, 70, 26), par(">", 1400, 700, 14, 26)}},
	}
	bl := blocchiLetti(nil, righe, 1, nil, time.Now())
	testi := map[string]Blocco{}
	for _, b := range bl {
		testi[b.Testo] = b
	}
	for _, v := range []string{"Off", "No", "English", "1920 x 1080", "80%"} {
		if _, ok := testi[v]; !ok {
			t.Fatalf("valore %q non separato dalle frecce: %v", v, chiaviBlocchi(bl))
		}
	}
	if b := testi["Off"]; b.X != 1322 || b.W != 40 {
		t.Errorf("riquadro di Off: %+v", b)
	}
	if b := testi["No"]; b.X < 1305 || b.X+b.W > 1365 {
		t.Errorf("riquadro di No (frecce attaccate): %d..%d", b.X, b.X+b.W)
	}
	if motivoNonTradurre(testi["Off"]) != "" || motivoNonTradurre(testi["English"]) != "" {
		t.Error("Off/English da tradurre")
	}
	if motivoNonTradurre(testi["1920 x 1080"]) == "" || motivoNonTradurre(testi["80%"]) == "" {
		t.Error("i numeri non si traducono")
	}
	tr := NuovoTraduttore(filepath.Join(t.TempDir(), "mem.json"), glossarioBase())
	for en, it := range map[string]string{"Off": "Disattivato", "On": "Attivato", "English": "Inglese", "High": "Alto", "No": "No"} {
		if r := tr.TraduciScritta(context.Background(), en); r.Tradotto != it {
			t.Errorf("%q -> %q (%s), atteso %q", en, r.Tradotto, r.Motore, it)
		}
	}
}

// Liste fitte (pixel art, missioni, 40 px tra le righe): due voci una sotto l'altra non sono
// una frase sola. Il paragrafo vero (frase che continua) resta unito.
func TestRigheDiListaNonUnite(t *testing.T) {
	riga := func(testo string, x, y, h float64) RigaOCR {
		var p []ParolaOCR
		for _, w := range strings.Fields(testo) {
			l := float64(len(w)) * h * 0.55
			p = append(p, par(w, x, y, l, h))
			x += l + h*0.35
		}
		return RigaOCR{Parole: p}
	}
	casi := []struct {
		righe  []string
		attese []string
	}{
		{[]string{"Objective: Reach the extraction point", "Something is moving in the basement."},
			[]string{"Objective: Reach the extraction point", "Something is moving in the basement."}},
		{[]string{"Objective: Find the missing documents", "Press E to interact"},
			[]string{"Objective: Find the missing documents", "Press E to interact"}},
		{[]string{"I don't trust him. He knows more than he's telling u", "Objective: Find the missing documents", "Checkpoint reached"},
			[]string{"I don't trust him. He knows more than he's telling u", "Objective: Find the missing documents", "Checkpoint reached"}},
		{[]string{"n't trust him. He knows more than he's telling u", "elective: Find the missing documents"},
			[]string{"n't trust him. He knows more than he's telling u", "elective: Find the missing documents"}},
		// paragrafi veri: restano una scritta
		{[]string{"A new season will begin on the first day of", "Each month."},
			[]string{"A new season will begin on the first day of Each month."}},
		{[]string{"The bridge collapsed years ago and now you", "need to find another way across."},
			[]string{"The bridge collapsed years ago and now you need to find another way across."}},
		{[]string{"Bring the stolen supplies back to the old", "Marcus at the harbor before nightfall."},
			[]string{"Bring the stolen supplies back to the old Marcus at the harbor before nightfall."}},
	}
	for _, c := range casi {
		var rr []RigaOCR
		for i, r := range c.righe {
			rr = append(rr, riga(r, 60, 320+float64(i)*40, 22))
		}
		bl := blocchiLetti(nil, rr, 1, nil, time.Now())
		got := chiaviBlocchi(bl)
		if strings.Join(got, " | ") != strings.Join(c.attese, " | ") {
			t.Errorf("righe %q -> %q, attese %q", c.righe, got, c.attese)
		}
	}
}

// Contatori dei giochi: parole con numeri ("Ammo: 24 / 120") si traducono; numeri e unità no.
func TestTestoConNumeri(t *testing.T) {
	if !daTradurre("Ammo: 24 / 120") || !daTradurre("Wave 3 of 10") {
		t.Error("scritte con numeri da tradurre")
	}
	for _, s := range []string{"185/425", "47.5 KM/h", "24 / 120", "HP 100/100", "v1.4.2"} {
		if daTradurre(s) {
			t.Errorf("%q non è testo da tradurre", s)
		}
	}
}

func chiaviBlocchi(bl []Blocco) []string {
	var out []string
	for _, b := range bl {
		if strings.ContainsAny(b.Testo, "<>◀▶") {
			continue
		}
		out = append(out, b.Testo)
	}
	return out
}

// Sottotitolo lungo su UNA linea che l'OCR dà in due pezzi vicini: torna una frase sola.
func TestSottotitoloSpezzatoRiunito(t *testing.T) {
	righe := []RigaOCR{
		{Parole: []ParolaOCR{par("MARCUS:", 289, 926, 150, 40), par("I", 450, 926, 10, 40), par("don't", 470, 926, 100, 40),
			par("trust", 582, 926, 90, 40), par("him.", 684, 926, 80, 40), par("He", 776, 926, 44, 40), par("knows", 832, 926, 116, 40), par("more", 960, 926, 96, 40)}},
		{Parole: []ParolaOCR{par("than", 1070, 927, 86, 40), par("he's", 1168, 927, 80, 40), par("telling", 1260, 927, 130, 40), par("us.", 1402, 927, 50, 40)}},
	}
	bl := blocchiLetti(nil, righe, 1, nil, time.Now())
	if len(bl) != 1 || bl[0].Testo != "MARCUS: I don't trust him. He knows more than he's telling us." {
		t.Fatalf("sottotitolo non riunito: %q", chiaviBlocchi(bl))
	}
	if _, v := correggiRigaOCR(bl[0].Testo); v == letturaDubbia {
		t.Fatal("sottotitolo giusto con voto dubbio")
	}
}
