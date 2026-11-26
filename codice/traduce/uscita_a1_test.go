package traduce

import (
	"image"
	"math/rand"
	"strings"
	"testing"
	"time"
)

// Prova finale (26/11, f0284, Gears of War): sottotitolo letto in 4 pezzi sulla stessa riga, due
// in inglese e due tradotti. Sullo schermo si vedeva "Control, this is Delta. Abbiamo immagini su
// Alphae Possibile KIA. No sign of resonator.": la riga intera resta in inglese.
func TestRigaInteraMaiMezza(t *testing.T) {
	a := Blocco{Testo: "Control, this is Delta", X: 430, Y: 962, W: 270, H: 28, Righe: 1, Em: 28}
	b := Blocco{Testo: "We have-visuals on Alphae", Tradotto: "Abbiamo immagini su Alphae", X: 710, Y: 962, W: 310, H: 28, Righe: 1, Em: 28}
	c := Blocco{Testo: "Possible", Tradotto: "Possibile", X: 1048, Y: 962, W: 100, H: 28, Righe: 1, Em: 28}
	d := Blocco{Testo: "KIA. No sign of resonator", X: 1162, Y: 957, W: 330, H: 33, Righe: 1, Em: 30}
	tenute, tolte := senzaMezzeRighe([]Blocco{b, c}, []Blocco{a, d})
	if len(tenute) != 0 || len(tolte) != 2 {
		t.Fatalf("riga mezza italiana e mezza inglese: tenute %q", testiBlocchi(tenute))
	}
	if n := mezzeDisegnate([]Blocco{b, c}, []Blocco{a, d}); n == 0 {
		t.Error("il conteggio del banco deve vedere la riga mezza (stessa regola del disegno)")
	}
	// tutta tradotta: si disegna
	a.Tradotto, d.Tradotto = "Controllo, qui Delta", "Possibile KIA. Nessuna traccia del risonatore"
	if tenute, _ := senzaMezzeRighe([]Blocco{a, b, c, d}, nil); len(tenute) != 4 {
		t.Errorf("riga tutta tradotta: tenute %q", testiBlocchi(tenute))
	}
	// due pulsanti corti di una fila restano indipendenti
	p1 := Blocco{Testo: "Inventory", Tradotto: "Inventario", X: 100, Y: 20, W: 120, H: 24, Righe: 1, Em: 24}
	p2 := Blocco{Testo: "Map", X: 250, Y: 20, W: 50, H: 24, Righe: 1, Em: 24}
	if tenute, _ := senzaMezzeRighe([]Blocco{p1}, []Blocco{p2}); len(tenute) != 1 {
		t.Error("due voci corte di una fila trattate come una frase")
	}
}

// f0433 (Dawnwalker): elenco puntato nello stesso riquadro, "Hold to aim" tradotto e "Move the
// camera with (R)" sotto in inglese (riga un po' più alta per l'icona del tasto).
func TestElencoNelRiquadroTuttoOVia(t *testing.T) {
	su := Blocco{Testo: "Hold to aim", Tradotto: "Tieni premuto per mirare", X: 713, Y: 648, W: 150, H: 24, Righe: 1, Em: 24}
	giu := Blocco{Testo: "Move the camera with", X: 713, Y: 684, W: 230, H: 34, Righe: 1, Em: 24}
	if tenute, _ := senzaMezzeRighe([]Blocco{su}, []Blocco{giu}); len(tenute) != 0 {
		t.Errorf("voce dell'elenco tradotta accanto a una rimasta in inglese")
	}
}

func TestFraseTroncataInFondo(t *testing.T) {
	b := Blocco{Testo: "A quick tap of lets you Shadowstep forward without", Righe: 1, Affid: letturaBuona}
	if m := fraseTroncata(b); m == "" {
		t.Error("frase che finisce con 'without' (la riga sotto non letta): non va tradotta da sola")
	}
	for _, s := range []string{"Press", "Hold to aim", "Welcome to our school.", "Defeat the enemies"} {
		if m := fraseTroncata(Blocco{Testo: s, Righe: 1}); m != "" {
			t.Errorf("%q presa per troncata", s)
		}
	}
	_ = strings.TrimSpace
}

func TestTitoliDeiVideo(t *testing.T) {
	for _, s := range []string{
		"007 FIRST LIGHT Gameplay Walkfirough Part 1 14K 60FPS PC ULTRAI - No Commentary (FULL GAME)",
		"THE BLOOD OF DAWNWALKER Gameplay Walkthrough Part 1 FULL GAME [4K 60FPS PS5 PROI - No Commentary",
		"ONE MORE DONUT - Full Gameplay Walktgrough Part 1 [FULL GAMEI No Comnmtary",
	} {
		if !titoloVideo(s) || motivoNonTradurre(Blocco{Testo: s, Righe: 1, Affid: letturaBuona}) == "" {
			t.Errorf("titolo del video da tradurre: %q", s)
		}
	}
	for _, s := range []string{"Locate the camp", "Part of the plan is to wait.", "Play the game", "Full health restored."} {
		if titoloVideo(s) {
			t.Errorf("%q preso per titolo di un video", s)
		}
	}
}

func TestNomeDelGiocoResta(t *testing.T) {
	for _, s := range []string{"The Blood of Dawnwalker", "THE BLOOD OF DAWNWALKER"} {
		if !nomeGioco(s) {
			t.Errorf("%q: è il titolo del gioco, resta in inglese", s)
		}
	}
	for _, s := range []string{"Follow Lunka", "Bring Lunka back", "All Good Things", "Defeat the enemies", "Directional Block", "Perfect Block"} {
		if nomeGioco(s) {
			t.Errorf("%q preso per il nome del gioco", s)
		}
	}
}

// "Locate the camp" -> "Inodia il campo" (007, sempre a schermo): "inodia" non è italiano
// (prima passava come "in" + "odia").
func TestParoleInventateProvaFinale(t *testing.T) {
	casi := [][2]string{
		{"Locate the camp", "Inodia il campo"},
		{"They are all armed.", "Sono tutti aggegnati."},
	}
	for _, c := range casi {
		if !italianoInventato(c[0], c[1]) {
			t.Errorf("%q: parola inventata non vista", c[1])
		}
	}
	for _, c := range [][2]string{
		{"Reload the weapon", "Ricarica l'arma"},
		{"Unlock the door", "Sblocca la porta"},
		{"Restart the level", "Riavvia il livello"},
		{"Disable the trap", "Disattiva la trappola"},
		{"Locate the camp", "Individua il campo"},
		{"Retrieve the documents", "Recupera i documenti"},
		{"Reinstall", "Reinstalla"},
	} {
		if italianoInventato(c[0], c[1]) {
			t.Errorf("%q presa per inventata (%v)", c[1], paroleInventateIt(c[0], c[1]))
		}
	}
}

func TestFrasiRipetuteTolte(t *testing.T) {
	casi := [][3]string{
		{"Lunka: There are more of them. Quick! Follow me!", "Lunka: Ce ne sono altri. Presto! Seguimi! Seguimi!", "Lunka: Ce ne sono altri. Presto! Seguimi!"},
		{"Come on! We have places to be!", "Forza! Forza! Abbiamo posti dove stare!", "Forza! Abbiamo posti dove stare!"},
		{"What? I was just bein' friendly.", "Cosa? Cosa? Stavo solo per essere amichevole.", "Cosa? Stavo solo per essere amichevole."},
		{"What's going on?", "Che succede? Cosa?", "Che succede?"},
		{"What's going on? What did you do to me?", "Che succede? Cosa? Cosa mi hai fatto?", "Che succede? Cosa mi hai fatto?"},
		// restano
		{"No! No!", "No! No!", "No! No!"},
		{"What? Really?", "Cosa? Davvero?", "Cosa? Davvero?"},
		{"Welcome to our school.", "Benvenuti nella nostra scuola.", "Benvenuti nella nostra scuola."},
		{"Hurry, hurry!", "Presto! Presto!", "Presto! Presto!"},
	}
	for _, c := range casi {
		if got := senzaRipetizioni(c[0], c[1]); got != c[2] {
			t.Errorf("%q -> %q, volevo %q", c[1], got, c[2])
		}
	}
}

func TestTerminiProvaFinale(t *testing.T) {
	casi := [][3]string{
		{"Restores 60% of Health Segment.", "Ripristina il 60% del segmento sanitario.", "Ripristina il 60% della tacca di salute."},
		{"You can toggle Target Lock with", "Puoi attivare la serratura di destinazione con", "Puoi attivare il blocco del bersaglio con"},
		{"To use Active Abilities, you need Activation Charges.", "Per utilizzare le Abilità Attive, è necessario Attivare le spese.", "Per utilizzare le Abilità Attive, servono le cariche di attivazione."},
		{"But the lieutenant here will be feeding you intel.", "Ma il tenente qui ti darà da mangiare.", "Ma il tenente qui ti passerà le informazioni."},
		{"C) JOURNAL", "C) GIORNALE", "C) DIARIO"},
		{"The principal continues with his speech...", "Il principale continua con il suo discorso...", "Il preside continua con il suo discorso..."},
		{"I'm Ms. Toriumi. I teach Composition.", "Sono la signora. Toriumi. Insegno composizione.", "Sono la signora Toriumi. Insegno composizione."},
		{"Hostile 1: They rigged the entire camp and perimeter.", "Hanno truccato l'intero campo e il perimetro.", "Hanno minato l'intero campo e il perimetro."},
		{"Have you seen the classroom assignments?", "Hai visto gli incarichi in classe?", "Hai visto le assegnazioni delle classi?"},
		{"Hey, it's Yuka-tan!", "Ehi, sono Yuka-tan!", "Ehi, è Yuka-tan!"},
	}
	for _, c := range casi {
		if got := correggiTerminiFrase(c[0], c[1]); got != c[2] {
			t.Errorf("%q -> %q, volevo %q", c[1], got, c[2])
		}
	}
	for en, it := range map[string]string{"Principal": "Preside", "Nice to meet you! What's your first name?": "Piacere di conoscerti! Come ti chiami?",
		"Locate the camp": "Individua il campo", "Target Lock": "Blocco del bersaglio", "KING RAVEN": "KING RAVEN"} {
		if v, ok := fraseMenuConNumero(en); !ok || !strings.EqualFold(v, it) {
			t.Errorf("frase pronta %q -> %q %v, volevo %q", en, v, ok, it)
		}
	}
	if motivoNonTradurre(Blocco{Testo: "4/7 Tug", Righe: 1, Affid: letturaBuona}) == "" {
		t.Error("la data del calendario del gioco non va tradotta")
	}
}

// Collaudo 02b: un'insegna che si avvicina cresce di un 30% a ogni lettura, col centro quasi
// fermo. Ogni passo da solo sembra "l'OCR che sbaglia di poco"; dalla prima lettura è cresciuta
// molto: è una scritta del mondo.
func TestInsegnaCheCresceAPoco(t *testing.T) {
	v := &votazione{}
	t0 := time.Unix(1_000_000, 0)
	w, h := 100, 20
	mosse := false
	for i := 0; i < 4; i++ {
		b := Blocco{Testo: "GAS STATION", X: 600 - w/2, Y: 300 - h/2, W: w, H: h}
		if v.mossa(b, t0.Add(time.Duration(i)*1500*time.Millisecond)) {
			mosse = true
		}
		w, h = w*13/10, h*13/10
	}
	if !mosse {
		t.Error("insegna cresciuta del 30% a ogni lettura (più del doppio in 4,5 s) non riconosciuta come scritta del mondo")
	}
	// una scritta ferma letta un po' diversa ogni volta resta ferma
	v2 := &votazione{}
	for i, d := range []int{0, 4, -3, 5} {
		b := Blocco{Testo: "Close Tutorial", X: 500, Y: 800, W: 160 + d, H: 22}
		if v2.mossa(b, t0.Add(time.Duration(i)*time.Second)) {
			t.Errorf("scritta ferma presa per mossa alla lettura %d", i)
		}
	}
}

// Collaudo 02b: gli obiettivi in alto a destra ("Find Marcus", "Abandoned Mine"), con le lettere
// nette, non sono la firma del canale.
func TestObiettiviNonSonoFirma(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	rnd := rand.New(rand.NewSource(7))
	for i := 0; i < len(img.Pix); i += 4 {
		v := uint8(20 + rnd.Intn(40))
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = v, v, v, 255
	}
	scrivi := func(b Blocco, luce uint8) {
		for y := b.Y + 3; y < b.Y+b.H-3; y++ {
			for x := b.X; x < b.X+b.W; x += 3 {
				i := img.PixOffset(x, y)
				img.Pix[i], img.Pix[i+1], img.Pix[i+2] = luce, luce, luce
			}
		}
	}
	for _, b := range []Blocco{
		{Testo: "Abandoned Mine", X: 1750, Y: 60, W: 140, H: 18},
		{Testo: "Find Marcus", X: 1760, Y: 100, W: 110, H: 18},
	} {
		b.Righe, b.Affid = 1, letturaBuona
		scrivi(b, 245)
		if firmaNellAngolo(img, b, []Blocco{b}) {
			t.Errorf("%q (obiettivo, lettere nette) presa per firma del canale", b.Testo)
		}
	}
	// la firma semitrasparente sì
	f := Blocco{Testo: "Horror Games House", X: 1771, Y: 990, W: 124, H: 16, Righe: 1, Affid: letturaBuona}
	scrivi(f, 120)
	if !firmaNellAngolo(img, f, []Blocco{f}) {
		t.Error("firma semitrasparente del canale non riconosciuta")
	}
}
