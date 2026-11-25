package traduce

// Banco sui video veri (25/11/2026): 45 minuti di gameplay di 9 giochi su YouTube a schermo
// intero, TassoProva in modalità schermo, un fotogramma ogni 2 secondi (report r1.txt). Le
// immagini in testdata/video1 sono i fotogrammi salvati (1280x720, jpg): qui le coordinate delle
// parole sono in quei pixel (quelle del report sono della cattura 1920x1080, divise per 1,5).

import (
	"context"
	"image"
	"image/color"
	"math/rand"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func apriVideo1(t *testing.T, nome string) *image.RGBA {
	t.Helper()
	img, err := apriImmagineRGBA("testdata/video1/" + nome + ".jpg")
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// errore 1 (fotogramma 433): sottotitolo su due righe centrate; la prima restava in inglese
// ("sick, sick" preso per una parola letta due volte) e la seconda veniva tradotta da sola.
func TestVideoSottotitoloDueRigheCentrate(t *testing.T) {
	img := apriVideo1(t, "0433")
	r1 := rigaParole(612, 16, "Rennick:", 315.0, 66.0, "I", 390.0, 5.0, "am", 401.0, 22.0, "sick,", 430.0, 33.0, "sick", 469.0, 30.0,
		"to", 504.0, 15.0, "the", 525.0, 25.0, "fucking", 556.0, 58.0, "stomach", 620.0, 66.0, "of", 693.0, 18.0, "you", 716.0, 24.0,
		"little", 747.0, 36.0, "bastards", 789.0, 67.0, "messing", 863.0, 63.0, "with", 932.0, 34.0)
	r2 := rigaParole(635, 16, "my", 585.0, 23.0, "operation!", 613.0, 82.0)
	bl := blocchiLetti(img, []RigaOCR{r1, r2}, 1, nil, time.Now())
	if len(bl) != 1 || bl[0].Righe != 2 || !bl[0].Centro {
		t.Fatalf("le due righe del sottotitolo non sono un paragrafo centrato: %q", chiaviBlocchi(bl))
	}
	b := bl[0]
	if !strings.HasSuffix(b.Testo, "messing with my operation!") || b.Affid == letturaDubbia {
		t.Fatalf("paragrafo %q affidabilità %d", b.Testo, b.Affid)
	}
	if m := motivoNonTradurre(b); m != "" {
		t.Fatalf("paragrafo scartato: %s", m)
	}
	if m := controllaCompleta(img, &b, bl); m != "" {
		t.Fatalf("paragrafo scartato: %s", m)
	}
	if paroleDoppie("I am sick, sick to the stomach") || !paroleDoppie("open the the door") {
		t.Error("paroleDoppie: la ripetizione con la virgola è voluta, quella senza è dell'OCR")
	}
}

// errore 2 (fotogramma 375): "down there?" sotto la prima riga è lo stesso sottotitolo.
func TestVideoSottotitoloFrammentoSotto(t *testing.T) {
	r1 := rigaParole(612, 16, "Scooby:", 331.0, 60.0, "Ahh,", 397.0, 36.0, "could", 439.0, 40.0, "you", 485.0, 28.0, "do", 519.0, 18.0,
		"me", 543.0, 22.0, "a", 571.0, 8.0, "quick", 585.0, 40.0, "fix", 631.0, 22.0, "on", 659.0, 18.0, "the", 683.0, 24.0, "crew", 713.0, 36.0,
		"lift", 755.0, 26.0, "fusebox", 787.0, 58.0, "while", 851.0, 38.0, "you're", 895.0, 54.0)
	r2 := rigaParole(635, 16, "down", 590.0, 42.0, "there?", 638.0, 52.0)
	bl := blocchiLetti(nil, []RigaOCR{r1, r2}, 1, nil, time.Now())
	if len(bl) != 1 || !strings.HasSuffix(bl[0].Testo, "while you're down there?") {
		t.Fatalf("sottotitolo non unito: %q", chiaviBlocchi(bl))
	}
	// un menu centrato (voci corte una sotto l'altra) resta diviso
	m := blocchiLetti(nil, []RigaOCR{rigaParole(400, 20, "New", 600.0, 40.0, "Game", 646.0, 50.0), rigaParole(428, 20, "Options", 610.0, 76.0),
		rigaParole(456, 20, "Quit", 625.0, 44.0)}, 1, nil, time.Now())
	if len(m) != 3 {
		t.Errorf("menu centrato unito: %q", chiaviBlocchi(m))
	}
}

// errore 3 (fotogramma 605): prima riga letta a pezzi; "contact you." sotto non si traduce da sola.
func TestVideoSecondaRigaDaSola(t *testing.T) {
	img := apriVideo1(t, "0605")
	righe := []RigaOCR{
		rigaParole(617, 15, "ne", 515.0, 20.0),
		rigaParole(617, 15, "yo", 697.0, 18.0, "information,", 736.0, 89.0, "I", 832.0, 3.0, "will", 839.0, 15.0, "make", 868.0, 42.0,
			"sure", 916.0, 29.0, "to", 951.0, 14.0),
		rigaParole(638, 13, "contact", 591.0, 61.0, "you.", 657.0, 32.0),
	}
	bl := blocchiLetti(img, righe, 1, nil, time.Now())
	for i := range bl {
		b := bl[i]
		if !strings.HasPrefix(b.Testo, "contact") {
			continue
		}
		if b.Righe == 1 && motivoNonTradurre(b) == "" && controllaCompleta(img, &b, bl) == "" {
			t.Fatalf("%q tradotta da sola sotto una prima riga in inglese", b.Testo)
		}
		return
	}
	t.Fatalf("seconda riga non trovata: %q", chiaviBlocchi(bl))
}

// errore 4 (fotogramma 100): riquadro dei suggerimenti con l'icona del tasto in mezzo alla prima
// riga. Prima: "Press" unita alla riga sotto e "• to reveal…" a parte, due traduzioni una sopra
// l'altra. Ora è UN paragrafo.
func TestVideoSuggerimentoConIcona(t *testing.T) {
	righe := []RigaOCR{
		rigaParole(203, 11, "Press", 34.0, 39.0),
		rigaParole(203, 11, "•", 87.0, 6.0, "to", 106.0, 15.0, "reveal", 128.0, 42.0, "the", 177.0, 24.0, "HUD", 208.0, 30.0, "and", 244.0, 25.0),
		rigaParole(224, 11, "view", 33.0, 32.0, "your", 70.0, 32.0, "current", 108.0, 55.0, "objective.", 169.0, 72.0),
	}
	for _, conImg := range []bool{false, true} {
		var img *image.RGBA
		if conImg {
			img = apriVideo1(t, "0100")
		}
		bl := blocchiLetti(img, righe, 1, nil, time.Now())
		if len(bl) != 1 || bl[0].Righe != 2 || bl[0].Testo != "Press to reveal the HUD and view your current objective." {
			t.Fatalf("immagine=%v: %q", conImg, chiaviBlocchi(bl))
		}
		if conImg {
			b := bl[0]
			if m := controllaCompleta(img, &b, bl); m != "" {
				t.Errorf("suggerimento scartato: %s", m)
			}
		}
	}
	// mai due traduzioni una sopra l'altra
	a := Blocco{Testo: "Press view your current objective.", X: 34, Y: 203, W: 207, H: 32, Righe: 2, Tradotto: "x"}
	c := Blocco{Testo: "to reveal the HUD and", X: 106, Y: 203, W: 163, H: 11, Righe: 1, Tradotto: "y"}
	d := Blocco{Testo: "Follow the plane", X: 570, Y: 680, W: 120, H: 14, Righe: 1, Tradotto: "z"}
	if got := senzaSovrapposte([]Blocco{a, c, d}); len(got) != 1 || got[0].Testo != d.Testo {
		t.Errorf("sovrapposte: %q", chiaviBlocchi(got))
	}
	// il nome del veicolo sopra il codice resta com'è
	nome := Blocco{Testo: "LURE PREDATOR", X: 75, Y: 512, W: 90, H: 10, Righe: 1, Affid: letturaBuona}
	codice := Blocco{Testo: "L35P", X: 110, Y: 533, W: 28, H: 12, Righe: 1}
	if m := motivoMondo(apriVideo1(t, "0100"), nome, []Blocco{nome, codice}); m == "" {
		t.Error("LURE PREDATOR sopra L35P: è il nome di un veicolo, resta com'è")
	}
}

// errore 5 (fotogramma 120): il voto nel tempo metteva il testo di DUE righe ("…a sharper turn.")
// nel riquadro della sola prima riga: la seconda restava inglese sotto la traduzione. E una riga
// che continua sotto (lettere non lette) non si traduce da sola.
func TestVideoVotoRiquadroDiverso(t *testing.T) {
	img := apriVideo1(t, "0120")
	v := &votazione{}
	t0 := time.Now()
	due := Blocco{X: 34, Y: 200, W: 270, H: 40, Righe: 1}
	una := Blocco{X: 34, Y: 200, W: 257, H: 17, Righe: 1}
	v.scegli(img, due, "Hold while steering to perform a sharper turn.", letturaBuona, t0)
	v.scegli(img, due, "Hold while steering to perform a sharper turn.", letturaBuona, t0.Add(time.Second))
	if got, _ := v.scegli(img, una, "Hold while steering to perform", letturaBuona, t0.Add(2*time.Second)); got != "Hold while steering to perform" {
		t.Errorf("il voto di un riquadro di due righe è finito su una riga: %q", got)
	}
	righe := []RigaOCR{rigaParole(203, 14, "Hold", 34.0, 32.0, "while", 99.0, 36.0, "steering", 142.0, 60.0, "to", 208.0, 16.0, "perform", 230.0, 61.0)}
	bl := blocchiLetti(img, righe, 1, nil, time.Now())
	if len(bl) != 1 {
		t.Fatalf("%q", chiaviBlocchi(bl))
	}
	b := bl[0]
	if m := controllaCompleta(img, &b, bl); !strings.Contains(m, "riga sotto") {
		t.Errorf("riga con la seguente non letta: %q", m)
	}
	for _, it := range []string{"Tenere premuto mentre si sterza per eseguire una svolta più nitina.", "Tenere premuto mentre si guida per eseguire un giro più Sharp"} {
		if got := correggiTerminiFrase("Hold while steering to perform a sharper turn.", it); !strings.Contains(got, "curva più stretta") || strings.Contains(got, "si guida") {
			t.Errorf("%q -> %q", it, got)
		}
	}
	// con la seconda riga letta è un paragrafo e si traduce
	righe = append(righe, rigaParole(224, 14, "a", 34.0, 6.0, "sharper", 47.0, 55.0, "turn.", 108.0, 33.0))
	bl = blocchiLetti(img, righe, 1, nil, time.Now())
	if len(bl) != 1 || bl[0].Righe != 2 {
		t.Fatalf("%q", chiaviBlocchi(bl))
	}
	b = bl[0]
	if m := controllaCompleta(img, &b, bl); m != "" {
		t.Errorf("paragrafo intero scartato: %s", m)
	}
}

// errore 6 (fotogrammi 318 e 22): insegne della scena tradotte. Ora restano in inglese; i
// sottotitoli dello stesso fotogramma no.
func TestVideoScritteDelMondo(t *testing.T) {
	img := apriVideo1(t, "0318")
	dan := Blocco{Testo: "DAN", X: 1177, Y: 22, W: 70, H: 12, Righe: 1, Affid: letturaBuona}
	insegna := Blocco{Testo: "CARGO HOIST", X: 691, Y: 169, W: 82, H: 11, Righe: 1, Affid: letturaBuona}
	sott := Blocco{Testo: "Muir No, I can beat Innes here all by myself.", X: 470, Y: 636, W: 347, H: 14, Righe: 1, Affid: letturaBuona}
	tutte := []Blocco{dan, insegna, sott}
	if m := motivoMondo(img, insegna, tutte); m == "" {
		t.Error("CARGO HOIST (insegna nel magazzino) tradotta")
	}
	if m := motivoMondo(img, sott, tutte); m != "" {
		t.Errorf("sottotitolo preso per scritta del mondo: %s", m)
	}
	img = apriVideo1(t, "0022")
	s1 := Blocco{Testo: "when they're like, \"Hey, give me something!", X: 480, Y: 622, W: 317, H: 18, Righe: 1, Affid: letturaBuona}
	s2 := Blocco{Testo: "I want something that you have!\"", X: 540, Y: 645, W: 200, H: 17, Righe: 1, Affid: letturaBuona}
	for _, s := range []Blocco{s1, s2} {
		if m := motivoMondo(img, s, []Blocco{s1, s2}); m != "" {
			t.Errorf("%q: %s", s.Testo, m)
		}
	}
	if m := letturaSpazzatura("••FORCED-TO CORK!e"); m == "" {
		t.Error("\"••FORCED-TO CORK!e\" (cartello letto male) tradotto")
	}
	// una scritta che si sposta di lato tra un fotogramma e l'altro si muove con la telecamera
	v := &votazione{}
	t0 := time.Now()
	b := Blocco{Testo: "CARGO HOIST", X: 691, Y: 169, W: 82, H: 11}
	if v.mossa(b, t0) {
		t.Error("prima volta: non può essersi mossa")
	}
	b.X += 40
	if !v.mossa(b, t0.Add(2*time.Second)) {
		t.Error("spostata di 40 pixel di lato: si muove con la scena")
	}
	sub := Blocco{Testo: "Muir No, I can beat Innes here all by myself.", X: 470, Y: 636, W: 347, H: 14}
	v.mossa(sub, t0)
	if v.mossa(sub, t0.Add(2*time.Second)) {
		t.Error("sottotitolo fermo preso per mosso")
	}
	lista := Blocco{Testo: "Load Game", X: 100, Y: 300, W: 90, H: 20}
	v.mossa(lista, t0)
	lista.Y += 40
	if v.mossa(lista, t0.Add(2*time.Second)) {
		t.Error("voce di una lista che scorre in verticale presa per scritta del mondo")
	}
}

// Nomi sparsi sulla mappa (regola del proprietario: restano in inglese) e la legenda in colonna
// su un pannello piatto (si traduce).
func TestMappaNomiSparsi(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	rnd := rand.New(rand.NewSource(3))
	for i := 0; i < len(img.Pix); i += 4 { // mappa: terreno pieno di dettagli
		v := uint8(60 + rnd.Intn(120))
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = v, v+10, v/2, 255
	}
	pannello := color.RGBA{20, 22, 28, 255}
	for y := 80; y < 300; y++ {
		for x := 1000; x < 1240; x++ {
			img.SetRGBA(x, y, pannello)
		}
	}
	nomi := []Blocco{
		{Testo: "Kokiri Forest", X: 300, Y: 200, W: 110, H: 14},
		{Testo: "Lost Woods", X: 520, Y: 330, W: 90, H: 14},
		{Testo: "Hyrule Field", X: 700, Y: 160, W: 100, H: 14},
		{Testo: "LAKE HYLIA", X: 420, Y: 520, W: 95, H: 14},
		{Testo: "Death Mountain", X: 640, Y: 470, W: 120, H: 14},
	}
	legenda := []Blocco{
		{Testo: "Main Quest", X: 1030, Y: 100, W: 90, H: 14},
		{Testo: "Side Quest", X: 1030, Y: 130, W: 90, H: 14},
		{Testo: "Fast Travel", X: 1030, Y: 160, W: 95, H: 14},
	}
	for i := range nomi {
		nomi[i].Righe, nomi[i].Affid = 1, letturaBuona
	}
	for i := range legenda {
		legenda[i].Righe, legenda[i].Affid = 1, letturaBuona
	}
	tutte := append(append([]Blocco(nil), nomi...), legenda...)
	for _, n := range nomi {
		if m := motivoMondo(img, n, tutte); m == "" {
			p, mm := punteggioMondo(img, n, tutte)
			t.Errorf("nome sulla mappa %q tradotto (%d %v)", n.Testo, p, mm)
		}
	}
	for _, l := range legenda {
		if m := motivoMondo(img, l, tutte); m != "" {
			t.Errorf("voce della legenda %q: %s", l.Testo, m)
		}
	}
}

// errori 7, 8, 9: spazzatura ("T wig h T", "Carry;" letto dal nome "Garry"), icone dei pulsanti
// lette come lettere ("Swing Sword O", "Back O") e azioni comuni nelle frasi pronte.
func TestVideoSpazzaturaEIcone(t *testing.T) {
	for _, s := range []string{"T wig h T", "Carry;", "x y z word"} {
		if letturaSpazzatura(s) == "" {
			t.Errorf("%q: è spazzatura dell'OCR", s)
		}
	}
	for _, s := range []string{"I wish I understood", "Press E or F", "Carry", "Plan A", "Hold Q to scan object", "Back"} {
		if m := letturaSpazzatura(s); m != "" {
			t.Errorf("%q: %s", s, m)
		}
	}
	casi := []struct {
		r    RigaOCR
		atte string
	}{
		{rigaParole(656, 16, "Swing", 520.0, 40.0, "Sword", 566.0, 44.0, "O", 622.0, 14.0), "Swing Sword"},
		{rigaParole(691, 13, "Back", 1185.0, 34.0, "O", 1229.0, 14.0), "Back"},
		{rigaParole(691, 13, "Press", 1100.0, 40.0, "A", 1145.0, 9.0), "Press A"},
		{rigaParole(691, 13, "•", 1100.0, 10.0, "Interact", 1118.0, 60.0), "Interact"},
	}
	for _, c := range casi {
		bl := blocchiLetti(nil, []RigaOCR{c.r}, 1, nil, time.Now())
		trovato := false
		for _, b := range bl {
			if b.Testo == c.atte {
				trovato = true
				if c.atte == "Back" && b.X+b.W > 1220 {
					t.Errorf("il riquadro copre ancora l'icona: %+v", b)
				}
			} else if motivoNonTradurre(b) == "" {
				t.Errorf("%q: l'icona %q si tradurrebbe", chiaviBlocchi(bl), b.Testo)
			}
		}
		if !trovato {
			t.Errorf("%q: atteso %q", chiaviBlocchi(bl), c.atte)
		}
	}
	for in, atteso := range map[string]string{"Swing Sword": "Colpo di spada", "Jump Attack": "Attacco in salto", "Toggle Pages": "Cambia pagina"} {
		if got, ok := fraseMenuConNumero(in); !ok || got != atteso {
			t.Errorf("%q -> %q, atteso %q", in, got, atteso)
		}
	}
	// diario (fotogramma 658): una parola straniera già nella frase inglese ("signora") non rende
	// la traduzione "mezza in inglese"; la spazzatura dell'OCR sì
	en := "I wish so that I could rest my soul in that sunburnt convent of Taroella. Would I find you there, Juan? Or signora Perosi, back from the beyond."
	it := "Vorrei poter riposare la mia anima in quel convento bruciato dal sole di Taroella. Ti troverei lì, Juan? O la signora Perosi, tornata dall'aldilà."
	if rigaMezzaTradotta(en, it) {
		t.Error("paragrafo del diario con \"signora\" scartato come mezzo inglese")
	}
	if !rigaMezzaTradotta("Hold thingthing to run", "Tieni premuto thingthing per correre") {
		t.Error("spazzatura copiata nella traduzione non vista")
	}
	// "T wig h T" sotto i paragrafi del diario non si traduce
	b := Blocco{Testo: "T wig h T", X: 727, Y: 583, W: 60, H: 14, Righe: 1, Affid: letturaBuona}
	if motivoNonTradurre(b) == "" {
		t.Error("\"T wig h T\" tradotta")
	}
}

// errore 10: tra due fotogrammi di fila con la stessa scritta, la traduzione già mostrata resta
// (come nel gioco): una lettura peggiore non la toglie e non la cambia.
func TestVideoStabileTraFotogrammi(t *testing.T) {
	dir := t.TempDir()
	sf, ink := color.RGBA{20, 22, 30, 255}, color.RGBA{240, 240, 240, 255}
	img := immagineFinta(900, 400, sf)
	f := parolaFinta(img, 50, 40, 10, 14, 30, ink)
	img2 := immagineFinta(900, 400, sf)
	parolaFinta(img2, 50, 40, 10, 14, 30, ink)
	buona := []RigaOCR{{Parole: []ParolaOCR{{T: "Press", X: 50, Y: 40, W: 80, H: 30}, {T: "any", X: 140, Y: 40, W: 50, H: 30},
		{T: "key", X: 200, Y: 40, W: 50, H: 30}, {T: "to", X: 260, Y: 40, W: 30, H: 30}, {T: "continue", X: 300, Y: 40, W: float64(f - 300), H: 30}}}}
	brutta := []RigaOCR{{Parole: []ParolaOCR{{T: "Prxqz", X: 50, Y: 40, W: 80, H: 30}, {T: "zny", X: 140, Y: 40, W: 50, H: 30},
		{T: "kqv", X: 200, Y: 40, W: 50, H: 30}, {T: "tn", X: 260, Y: 40, W: 30, H: 30}, {T: "cxntinvq", X: 300, Y: 40, W: float64(f - 300), H: 30}}}}
	lettura := buona
	tr := NuovoTraduttore(filepath.Join(dir, "mem.json"), glossarioBase(), motoreDizionario{})
	o := opzioniBanco{leggi: func(*image.RGBA) ([]RigaOCR, float64, error) { return lettura, 1, nil }, tr: tr,
		voti: &votazione{}, stato: &statoBanco{}}
	e1, p1 := elaboraImmagine(img, "f1", o)
	if len(p1) != 1 || e1.Mostrate != 1 {
		t.Fatalf("primo fotogramma: %+v", e1.Righe)
	}
	lettura = brutta
	e2, p2 := elaboraImmagine(img2, "f2", o)
	if len(p2) != 1 || p2[0].Tradotto != p1[0].Tradotto {
		t.Fatalf("secondo fotogramma (stessi pixel, lettura peggiore): %+v", e2.Righe)
	}
	if ins := confrontaFotogrammi(e1, e2); len(ins) != 0 {
		t.Errorf("instabilità tra due fotogrammi uguali: %+v", ins)
	}
	_ = context.Background()
}
