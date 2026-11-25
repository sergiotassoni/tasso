package traduce

// Sesto giro: i 9 video veri rifatti con la build bd5a49e (r2.txt, 1339 fotogrammi). Casi:
//   - 0600 (Directive 8020): la stessa riga letta due volte in pezzi accavallati, uno letto male
//     (resta in inglese) e uno tradotto e disegnato sopra metà riga → mai più: letture doppie
//     unite, pezzo dentro una riga incerta mai tradotto, controllo finale senzaMezzeRighe anche
//     sulle traduzioni rimaste dal fotogramma prima (0609: "Chiedi" su una riga inglese);
//   - 0019: "…that you hay !" → "have!" (verbo comune dopo il soggetto, confusione y/v);
//   - 0901: "Swing Sword (Y)" → la traduzione non va mai sopra l'icona del tasto, cresce a sinistra;
//   - 0370: "Caz: What do you need, Scoob?" (nomi di chi parla e di chi è chiamato) non è più
//     una lettura incerta;
//   - "traduzioni sovrapposte": le due letture della stessa riga diventano una riga sola.

import (
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// rigaDa: una riga dell'OCR con le parole del testo distribuite in [x, x+w) in proporzione
// alle lettere (come le dà Windows, più o meno).
func rigaDa(testo string, x, y, w, h float64) RigaOCR {
	parole := strings.Fields(testo)
	tot := 0
	for _, p := range parole {
		tot += len([]rune(p)) + 1
	}
	var r RigaOCR
	px := w / float64(tot)
	for _, p := range parole {
		lw := float64(len([]rune(p))) * px
		r.Parole = append(r.Parole, ParolaOCR{T: p, X: x, Y: y, W: lw, H: h})
		x += lw + px
	}
	return r
}

// decisione: come nel percorso vero (banco e gioco), le scritte da disegnare e quelle che
// restano in inglese con parole vere, con la traduzione finta "IT:<testo>".
func decisione(img *image.RGBA, bl []Blocco) (pronti, inglesi []Blocco) {
	for _, b := range bl {
		if m := motivoNonTradurre(b); m != "" {
			if ingleseVisibile(m) {
				inglesi = append(inglesi, b)
			}
			continue
		}
		x := b
		if m := controllaCompleta(img, &x, bl); m != "" {
			if ingleseVisibile(m) {
				inglesi = append(inglesi, x)
			}
			continue
		}
		x.Tradotto = "IT:" + x.Testo
		pronti = append(pronti, x)
	}
	pronti = senzaSovrapposte(pronti)
	pronti, _ = senzaMezzeRighe(pronti, inglesi)
	return pronti, inglesi
}

// fotogramma-0600 (r2): riga 1 letta due volte, accavallata; riga 2 "contact you." sotto.
//
//	[472,924]  "No, I lc nit. Je has gone missing. If you leave your information, I ill make" → incerta
//	[1019,924] "'e your information, I will make sure to"                                      → TRADOTTA (errore)
//	[885,954]  "contact you."
//
// Ritaglio testdata/video2/0600-sottotitolo.png: fotogramma a 1280x720 da (280,590).
func TestRigaLettaDueVolteMaiMezza(t *testing.T) {
	a := rigaDa("No, I lc nit. Je has gone missing. If you leave your information, I ill make", 35, 27, 577, 18)
	b := rigaDa("'e your information, I will make sure to", 399, 27, 289, 18)
	c := rigaDa("contact you.", 311, 46, 99, 18)
	img, err := apriImmagineRGBA("testdata/video2/0600-sottotitolo.png")
	if err != nil {
		t.Fatal(err)
	}
	for _, conImg := range []*image.RGBA{nil, img} {
		bl := blocchiLetti(conImg, []RigaOCR{a, b, c}, 1, nil, time.Now())
		pronti, _ := decisione(conImg, bl)
		if len(pronti) > 0 {
			t.Errorf("immagine=%v: disegnate %q (letto %q): la riga è in parte illeggibile, doveva restare tutta in inglese",
				conImg != nil, testiBlocchi(pronti), chiaviBlocchi(bl))
		}
	}
	// le due letture della riga 1 diventano una riga sola, con le parole migliori nella parte comune
	bl := unisciLettureDoppie([]Blocco{bloccoDaParole(a.Parole, 1), bloccoDaParole(b.Parole, 1)})
	if len(bl) != 1 || !strings.HasSuffix(bl[0].Testo, "your information, I will make sure to") {
		t.Errorf("letture doppie non unite bene: %q", chiaviBlocchi(bl))
	}
	// anche senza l'unione: il pezzo accavallato a una riga incerta non si traduce da solo
	sa, sb := bloccoDaParole(a.Parole, 1), bloccoDaParole(b.Parole, 1)
	sa.Affid, sb.Affid = letturaDubbia, letturaBuona
	if m := motivoFratello(sb, []Blocco{sa, sb}); m == "" {
		t.Errorf("%q accavallata a una riga incerta: doveva restare in inglese", sb.Testo)
	}
	// e il controllo finale toglie una traduzione accavallata a una scritta inglese
	sb.Tradotto = "e le tue informazioni, farò in modo di"
	if tenute, _ := senzaMezzeRighe([]Blocco{sb}, []Blocco{sa}); len(tenute) != 0 {
		t.Errorf("traduzione disegnata su metà di una riga inglese")
	}
	// "contact you." sotto una prima riga rimasta in inglese: mai da sola
	sc := bloccoDaParole(c.Parole, 1)
	sc.Testo = "contact you."
	sa.Testo = "No, I can't. Jeremy has gone missing. If you leave your information, I will make sure to"
	if tenute, _ := senzaMezzeRighe([]Blocco{sc}, []Blocco{sa}); len(tenute) != 0 {
		t.Errorf("%q disegnata da sola sotto la sua prima riga in inglese", sc.Testo)
	}
}

func testiBlocchi(b []Blocco) []string {
	var out []string
	for _, x := range b {
		out = append(out, x.Testo)
	}
	return out
}

// fotogramma-0609: "ask" tradotto nel fotogramma prima ("Chiedi") non torna sopra la riga intera
// letta adesso e lasciata in inglese ("Fine. I'll ask h m. Wait here and don't touch anything.").
func TestTraduzioneVecchiaMaiSuRigaInglese(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 100))
	vecchia := Blocco{Testo: "ask", Tradotto: "Chiedi", X: 120, Y: 40, W: 30, H: 18, Img: img}
	riga := Blocco{Testo: "Fine. I'll ask h m. Wait here and don't touch anything.", X: 40, Y: 40, W: 330, H: 18, Righe: 1}
	if out := recuperaPerse(nil, []Blocco{vecchia}, img, []Blocco{riga}); len(out) != 0 {
		t.Errorf("traduzione vecchia recuperata sopra una riga ora in inglese: %v", testiBlocchi(out))
	}
	// e se arriva lo stesso (tieniFerme, stabilizza), il controllo finale la toglie
	if tenute, tolte := senzaMezzeRighe([]Blocco{vecchia}, []Blocco{riga}); len(tenute) != 0 || len(tolte) != 1 {
		t.Errorf("mezza riga tradotta rimasta dal fotogramma prima")
	}
	// una scritta inglese tutta coperta dalla traduzione (lettura doppia più corta) non conta
	intera := Blocco{Testo: "Jump Attack", Tradotto: "Attacco in salto", X: 819, Y: 73, W: 160, H: 30, Righe: 1}
	pezzo := Blocco{Testo: "Jump Atta", X: 819, Y: 74, W: 130, H: 29, Righe: 1}
	if tenute, _ := senzaMezzeRighe([]Blocco{intera}, []Blocco{pezzo}); len(tenute) != 1 {
		t.Errorf("la lettura intera doveva restare: il pezzo inglese è tutto coperto")
	}
	// due pulsanti di una fila: uno in inglese non toglie l'altro
	p1 := Blocco{Testo: "Inventory", Tradotto: "Inventario", X: 100, Y: 20, W: 120, H: 24, Righe: 1, Em: 24}
	p2 := Blocco{Testo: "Xyzzt", X: 300, Y: 20, W: 70, H: 24, Righe: 1, Em: 24}
	if tenute, _ := senzaMezzeRighe([]Blocco{p1}, []Blocco{p2}); len(tenute) != 1 {
		t.Errorf("voci lontane di una fila trattate come una frase")
	}
	// due sottotitoli di persone diverse, uno sopra l'altro: frasi finite, nessun legame
	su := Blocco{Testo: "Caz: What do you need, Scoob?", X: 775, Y: 876, W: 380, H: 30, Righe: 1}
	giu := Blocco{Testo: "Scooby: Ahh, could you do me a quick fix?", Tradotto: "x", X: 496, Y: 918, W: 900, H: 30, Righe: 1}
	if tenute, _ := senzaMezzeRighe([]Blocco{giu}, []Blocco{su}); len(tenute) != 1 {
		t.Errorf("sottotitolo tolto per la riga sopra di un'altra persona")
	}
}

// fotogramma-0019: "I want something that you hay !" → "have!" (prima: "che tu fieno!").
func TestVerboFinaleConfuso(t *testing.T) {
	casi := map[string]string{
		"when they're like, \"Heyu give me something! I want something that you hay !\"": "I want something that you have!\"",
		"Do you nave?": "Do you have?",
	}
	for in, atteso := range casi {
		if c, _ := correggiRigaOCR(in); !strings.Contains(c, atteso) {
			t.Errorf("%q -> %q, atteso che contenga %q", in, c, atteso)
		}
	}
	// frasi giuste restano come sono
	for _, s := range []string{"I bet!", "Thank you sir!", "You fool!", "Roll in the hay!", "We did it!", "Are you sure?"} {
		if c := verboFinale(s); c != s {
			t.Errorf("%q cambiata in %q", s, c)
		}
	}
}

// fotogramma-0370: la battuta piccola sopra il sottotitolo, con il nome di chi parla e di chi
// viene chiamato, non è una lettura incerta. Le parole inventate con la maiuscola sì.
func TestNomiNeiDialoghi(t *testing.T) {
	for _, s := range []string{"Caz: What do you need, Scoob?", "Innes: You'll be alright, Caz."} {
		c, a := correggiRigaOCR(s)
		if m := motivoNonTradurre(Blocco{Testo: c, Affid: a}); m != "" {
			t.Errorf("%q: %s", s, m)
		}
	}
	for _, s := range []string{"look at any- VII Igok at", "Gamer's Litüe Playgrmjnd Walid-trough Comnmtary"} {
		if letturaAffidabile(s) {
			t.Errorf("%q: lettura storpiata presa per buona", s)
		}
	}
}

// fotogramma-0901: "Swing Sword (Y)": l'icona del tasto letta come "Y" si toglie e la traduzione
// ("Colpo di spada", più lunga) non ci va mai sopra: cresce verso sinistra.
func TestTraduzioneMaiSopraIconaTasto(t *testing.T) {
	r := rigaParole(996, 30, "Swing", 844.0, 80.0, "Sword", 934.0, 90.0, "Y", 1036.0, 34.0)
	bl := blocchiLetti(nil, []RigaOCR{r}, 1, nil, time.Now())
	if len(bl) != 1 || bl[0].Testo != "Swing Sword" {
		t.Fatalf("icona non tolta: %q", chiaviBlocchi(bl))
	}
	spazioLibero(bl, 1920)
	b := bl[0]
	if b.IconaDx != 1036 {
		t.Fatalf("posto dell'icona non ricordato: %d", b.IconaDx)
	}
	if b.X+largConsentita(b) > b.IconaDx {
		t.Errorf("la traduzione può arrivare a %d, l'icona comincia a %d", b.X+largConsentita(b), b.IconaDx)
	}
	// una traduzione più lunga dello spazio prima dell'icona: si sposta a sinistra
	w := b.W + 40
	if w > largConIcona(b) {
		t.Fatalf("lo spazio a sinistra non è stato usato: %d > %d", w, largConIcona(b))
	}
	tx := inizioConIcona(b, w)
	if tx+w > b.IconaDx || tx >= b.X {
		t.Errorf("traduzione da %d a %d: icona a %d, inglese da %d", tx, tx+w, b.IconaDx, b.X)
	}
	// senza icona resta allineata a sinistra
	n := b
	n.IconaDx = 0
	if inizioConIcona(n, w) != n.X {
		t.Errorf("senza icona la traduzione deve cominciare dove l'inglese")
	}
	// ritaglio vero (fotogramma 1280x720 da (480,640)): l'OCR ha letto solo "Swing Sword" (x
	// 84-197), il cerchio del tasto (x 207-228) non l'ha letto. La traduzione non ci va sopra.
	img, err := apriImmagineRGBA("testdata/video2/0901-tasto.png")
	if err != nil {
		t.Fatal(err)
	}
	rv := rigaParole(26, 18, "Swing", 84.0, 50.0, "Sword", 140.0, 57.0)
	bv := blocchiLetti(img, []RigaOCR{rv}, 1, nil, time.Now())
	spazioLibero(bv, img.Bounds().Dx())
	if len(bv) != 1 {
		t.Fatalf("ritaglio: %q", chiaviBlocchi(bv))
	}
	x := bv[0]
	if m := controllaCompleta(img, &x, bv); m != "" {
		t.Fatalf("ritaglio: %q non si traduce: %s", x.Testo, m)
	}
	p := preparaScritta(img, img, x, Risultato{Tradotto: "Colpo di spada", Motore: "catalogo"})
	if p.IconaDx < 205 || p.IconaDx > 210 {
		t.Fatalf("ritaglio: icona non trovata (IconaDx=%d, attesa ~207)", p.IconaDx)
	}
	if p.X+largConsentita(p) > p.IconaDx {
		t.Errorf("ritaglio: la traduzione può arrivare a %d, sopra l'icona (%d)", p.X+largConsentita(p), p.IconaDx)
	}
	if w := p.W * 125 / 100; w <= largConIcona(p) {
		if tx := inizioConIcona(p, w); tx+w > p.IconaDx {
			t.Errorf("ritaglio: traduzione larga %d da %d: sopra l'icona", w, tx)
		}
	} else {
		t.Errorf("ritaglio: niente spazio a sinistra per una traduzione un quarto più lunga (%d > %d)", w, largConIcona(p))
	}
}

// "traduzioni sovrapposte" di r2: la stessa riga letta in due pezzi accavallati torna una riga.
func TestLettureDoppieUnite(t *testing.T) {
	casi := []struct {
		a, c   RigaOCR
		attesa string
	}{
		{rigaDa("Have you ever seen anything like this?", 617, 955, 520, 30),
			rigaDa("er seen anything like this? Looks like a talisman.", 763, 954, 690, 30),
			"Have you ever seen anything like this? Looks like a talisman."},
		{rigaDa("Do you want to see Jerem", 755, 954, 330, 30),
			rigaDa("want to see Jeremy's room?", 842, 954, 380, 30),
			"Do you want to see Jeremy's room?"},
		{rigaDa("Jump Atta", 819, 74, 130, 29), rigaDa("Jump Attack", 819, 73, 160, 30), "Jump Attack"},
		{rigaDa("This revolt", 663, 946, 140, 30),
			rigaDa("revolt brings war to the peace-loving citizens of Urzikstan.", 716, 946, 780, 30),
			"This revolt brings war to the peace-loving citizens of Urzikstan."},
		{rigaDa("Find an oil filter. It will silenc", 762, 946, 440, 30),
			rigaDa("hill silence your weapon...", 980, 944, 330, 30),
			"Find an oil filter. It will silence your weapon..."},
	}
	for _, c := range casi {
		bl := unisciLettureDoppie([]Blocco{bloccoDaParole(c.a.Parole, 1), bloccoDaParole(c.c.Parole, 1)})
		if len(bl) != 1 || bl[0].Testo != c.attesa {
			t.Errorf("%q + %q -> %q, attesa %q", testoParole(c.a.Parole), testoParole(c.c.Parole), chiaviBlocchi(bl), c.attesa)
		}
	}
	// due scritte diverse sulla stessa linea che si toccano appena restano due
	x := rigaDa("Inventory", 100, 20, 120, 24)
	y := rigaDa("Map", 215, 20, 50, 24)
	if bl := unisciLettureDoppie([]Blocco{bloccoDaParole(x.Parole, 1), bloccoDaParole(y.Parole, 1)}); len(bl) != 2 {
		t.Errorf("scritte diverse unite: %q", chiaviBlocchi(bl))
	}
}

// fotogramma-1037 (Zelda): dialogo su due righe con l'interlinea larga. La prima era tradotta da
// sola ("…perché MidO lo è") e la seconda ("blocking the path?") restava inglese: ora sono una
// frase sola. Se restassero divise, il controllo finale non disegna la prima da sola.
func TestDialogoInterlineaLarga(t *testing.T) {
	a := rigaDa("Oh,you haven't seen him yet...because Mido is", 561, 825, 700, 30)
	b := rigaDa("blocking the path?", 560, 886, 280, 30)
	bl := blocchiLetti(nil, []RigaOCR{a, b}, 1, nil, time.Now())
	if len(bl) != 1 || bl[0].Righe != 2 {
		t.Errorf("righe non unite: %q", chiaviBlocchi(bl))
	}
	sa, sb := bloccoDaParole(a.Parole, 1), bloccoDaParole(b.Parole, 1)
	sa.Tradotto = "Oh, non l'hai ancora visto... perché Mido lo è"
	if tenute, _ := senzaMezzeRighe([]Blocco{sa}, []Blocco{sb}); len(tenute) != 0 {
		t.Errorf("prima riga disegnata da sola sopra la seconda in inglese")
	}
	// due battute diverse con l'interlinea larga restano divise
	c := rigaDa("Where is the Great Deku Tree?", 561, 825, 420, 30)
	d := rigaDa("Link: Follow me.", 560, 886, 230, 30)
	if bl := blocchiLetti(nil, []RigaOCR{c, d}, 1, nil, time.Now()); len(bl) != 2 {
		t.Errorf("battute diverse unite: %q", chiaviBlocchi(bl))
	}
}

// Banco come nel gioco (fotogrammi di fila, fotogramma-0608 → 0609): "ask" tradotto da solo nel
// primo; nel secondo la riga intera letta male resta in inglese. La traduzione vecchia non torna
// sopra la riga inglese e il conteggio delle mezze disegnate resta 0.
func TestBancoTraduzioneVecchiaSuRigaInglese(t *testing.T) {
	dir := t.TempDir()
	sf, ink := color.RGBA{20, 22, 30, 255}, color.RGBA{240, 240, 240, 255}
	img1 := immagineFinta(900, 200, sf)
	parolaFinta(img1, 300, 80, 3, 10, 24, ink) // "ask" da solo
	img2 := immagineFinta(900, 200, sf)
	x := 60
	for _, n := range []int{5, 4, 3, 1, 1, 5, 4, 3, 5, 9} { // "Fine. I'll ask h m. Wait here and don't anything."
		x = parolaFinta(img2, x, 80, n, 10, 24, ink) + 14
	}
	parolaFinta(img2, 300, 80, 3, 10, 24, ink) // "ask" negli stessi pixel
	letture := [][]RigaOCR{
		{{Parole: []ParolaOCR{{T: "ask", X: 300, Y: 80, W: 34, H: 24}}}},
		{rigaDa("Fine. Ixl'q aqk hxq mzv. Wqit hrre qnd dqn't tovch anytjing.", 60, 80, float64(x-74), 24)},
	}
	k := 0
	tr := NuovoTraduttore(filepath.Join(dir, "mem.json"), glossarioBase(), motoreDizionario{})
	o := opzioniBanco{leggi: func(*image.RGBA) ([]RigaOCR, float64, error) { return letture[k], 1, nil }, tr: tr,
		voti: &votazione{}, stato: &statoBanco{}}
	e1, p1 := elaboraImmagine(img1, "0608", o)
	if len(p1) != 1 {
		t.Fatalf("primo fotogramma: \"ask\" doveva essere tradotto: %+v", e1.Righe)
	}
	k = 1
	e2, p2 := elaboraImmagine(img2, "0609", o)
	if len(p2) != 0 || e2.Mostrate != 0 {
		t.Errorf("secondo fotogramma: disegnate %q sopra una riga rimasta in inglese (%+v)", testiBlocchi(p2), e2.Righe)
	}
	if e2.MezzeMostrate != 0 {
		t.Errorf("mezze disegnate: %d", e2.MezzeMostrate)
	}
}
