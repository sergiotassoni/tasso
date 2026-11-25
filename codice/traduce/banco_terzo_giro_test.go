package traduce

// Terzo giro sul banco di prova vero (OCR di Windows, risultati-3): le righe che sul PC vero
// restavano in inglese. Le immagini in testdata/banco3 sono ritagli delle schermate vere del
// banco; i rettangoli delle parole sono quelli che dà Windows (stretti sulle lettere), misurati
// sulle lettere vere. Ogni regola vale per tutti i giochi, non per queste scritte.

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"time"
)

func apriBanco3(t *testing.T, nome string) *image.RGBA {
	t.Helper()
	img, err := apriImmagineRGBA("testdata/banco3/" + nome + ".png")
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// spostaRiga: le parole della schermata intera nelle coordinate del ritaglio (origine ox, oy).
func spostaRiga(r RigaOCR, ox, oy float64) RigaOCR {
	out := RigaOCR{Parole: append([]ParolaOCR(nil), r.Parole...)}
	for i := range out.Parole {
		out.Parole[i].X -= ox
		out.Parole[i].Y -= oy
	}
	return out
}

// allarga: ogni parola più larga di d pixel per lato (Windows a volte dà i riquadri con un po' di margine).
func allarga(r RigaOCR, d float64) RigaOCR {
	out := RigaOCR{Parole: append([]ParolaOCR(nil), r.Parole...)}
	for i := range out.Parole {
		out.Parole[i].X -= d
		out.Parole[i].W += 2 * d
	}
	return out
}

// 1) 26/27/28-impostazioni: "Apply   Reset to Default   Back" con le parole strette di Windows
// (buchi 23, 8, 10, 23 px): la mediana dei buchi cadeva già su un buco grande e la fila restava
// una riga sola, poi scartata come "mezza in inglese". Ora i buchi di due grandezze nette dividono
// le voci, anche con i riquadri un po' più larghi.
func TestFilaPulsantiParoleStrette(t *testing.T) {
	img := apriBanco3(t, "26-impostazioni") // ritaglio da (100,990)
	vera := rigaParole(1016, 27, "Apply", 141.0, 79.0, "Reset", 243.0, 74.0, "to", 325.0, 26.0, "Default", 361.0, 96.0, "Back", 480.0, 65.0)
	for _, d := range []float64{0, 2} {
		r := spostaRiga(allarga(vera, d), 100, 990)
		bl := blocchiLetti(img, []RigaOCR{r}, 1, nil, time.Now())
		if got := strings.Join(chiaviBlocchi(bl), " | "); got != "Apply | Reset to Default | Back" {
			t.Fatalf("margine %v: fila di pulsanti non divisa: %q", d, got)
		}
		for i := range bl {
			b := bl[i]
			if m := controllaCompleta(img, &b, bl); m != "" {
				t.Errorf("margine %v: %q scartato: %s", d, b.Testo, m)
			}
		}
	}
	// righe che restano intere anche con la regola nuova
	for _, r := range []RigaOCR{
		// due frasi: dopo il punto lo spazio un po' più largo non divide
		rigaParole(930, 34, "The", 210.0, 60.0, "bridge", 278.0, 100.0, "collapsed", 386.0, 150.0, "years", 544.0, 85.0, "ago.", 637.0, 62.0, "You'll", 717.0, 95.0, "need", 820.0, 72.0),
		rigaParole(400, 30, "Talk", 100.0, 60.0, "to", 168.0, 28.0, "Marcus", 220.0, 100.0),                     // dopo "to" la frase continua
		rigaParole(400, 30, "Find", 100.0, 60.0, "The", 168.0, 50.0, "Lost", 226.0, 60.0, "Sword", 294.0, 80.0), // spazi uguali
		rigaParole(862, 24, "Ammo:", 846.0, 94.0, "24", 959.0, 33.0, "/", 1002.0, 9.0, "120", 1023.0, 50.0),
	} {
		if bl := blocchiLetti(nil, []RigaOCR{r}, 1, nil, time.Now()); len(bl) != 1 {
			t.Errorf("riga divisa per sbaglio: %q", chiaviBlocchi(bl))
		}
	}
}

// 1b) Se una fila di voci arriva comunque intera al traduttore e viene scartata come mezza, si
// riprova voce per voce (vociDiUnaFila). Le frasi e le righe con spazi uguali non si dividono.
func TestVociDiUnaFila(t *testing.T) {
	vera := rigaParole(1016, 27, "Apply", 141.0, 79.0, "Reset", 243.0, 74.0, "to", 325.0, 26.0, "Default", 361.0, 96.0, "Back", 480.0, 65.0)
	b := bloccoDaParole(vera.Parole, 1)
	b.Testo = "Apply Reset to Default Back"
	var got []string
	for _, v := range vociDiUnaFila(b) {
		got = append(got, v.Testo)
	}
	if strings.Join(got, " | ") != "Apply | Reset to Default | Back" {
		t.Fatalf("voci: %q", got)
	}
	for _, r := range []RigaOCR{
		rigaParole(400, 30, "Find", 100.0, 60.0, "The", 168.0, 50.0, "Lost", 226.0, 60.0, "Sword", 294.0, 80.0),
		rigaParole(400, 30, "You", 100.0, 50.0, "can", 168.0, 50.0, "sprint", 260.0, 80.0, "now.", 350.0, 60.0),
		rigaParole(400, 30, "Return", 100.0, 90.0, "to", 200.0, 30.0, "Main", 260.0, 70.0, "Menu", 340.0, 70.0),
	} {
		b := bloccoDaParole(r.Parole, 1)
		if v := vociDiUnaFila(b); v != nil {
			t.Errorf("%q divisa per sbaglio in %d voci", b.Testo, len(v))
		}
	}
}

// 2) 07-sottotitolo: "MARCUS: I don't trust him. ..." letto perfetto ma scartato come mezzo
// inglese: il nome di chi parla resta com'è (il traduttore lo lascia apposta) e non conta.
func TestNomeChiParlaNonMezzo(t *testing.T) {
	en := "MARCUS: I don't trust him. He knows more than he's telling us."
	if rigaMezzaTradotta(en, "MARCUS: Non mi fido di lui. Sa più di quanto ci dica.") {
		t.Fatal("il nome di chi parla preso per inglese rimasto")
	}
	if !rigaMezzaTradotta(en, "MARCUS: Non mi fido di lui. He knows more di quanto ci dica.") {
		t.Fatal("la frase mezza inglese dopo il nome non vista")
	}
	for in, nomi := range map[string]string{
		"MARCUS: I don't trust him.":            "MARCUS",
		"OLD MAN: The bridge collapsed.":        "OLD MAN",
		"Kael: Follow me, quickly.":             "Kael",
		"Objective: Reach the extraction point": "",
		"Item collected: Rusty Key":             "",
		"TIP:":                                  "",
	} {
		if got := strings.Join(nomeChiParla(in), " "); got != nomi {
			t.Errorf("%q: nome %q, atteso %q", in, got, nomi)
		}
	}
}

// 3) 29-caricamento: "TIP:" su un fondo chiaro non letto, la frase dopo letta intera. L'inchiostro
// non letto è corto (4 lettere) e la frase è completa: si traduce la frase (coperta solo lei).
// Una parola lunga o più parole non lette accanto lasciano la riga incompleta.
func TestFraseConEtichettaNonLetta(t *testing.T) {
	img := apriBanco3(t, "29-caricamento") // ritaglio da (560,900)
	r := spostaRiga(rigaParole(946, 29, "You", 724.0, 47.0, "can", 783.0, 44.0, "sprint", 838.0, 79.0, "by", 929.0, 28.0,
		"holding", 969.0, 100.0, "the", 1077.0, 44.0, "Shift", 1131.0, 61.0, "key.", 1204.0, 47.0), 560, 900)
	bl := blocchiLetti(img, []RigaOCR{r}, 1, nil, time.Now())
	if len(bl) != 1 {
		t.Fatalf("blocchi: %q", chiaviBlocchi(bl))
	}
	b := bl[0]
	if !scrittaContinuaTra(img, b, bl) {
		t.Fatal("l'inchiostro di \"TIP:\" non visto (il test non prova niente)")
	}
	x0 := b.X
	if m := controllaCompleta(img, &b, bl); m != "" {
		t.Fatalf("frase completa scartata: %s", m)
	}
	if b.X < x0-3 {
		t.Errorf("il riquadro si è allargato sull'etichetta non letta: %d -> %d", x0, b.X)
	}
	// frase non completa (senza maiuscola): resta incompleta
	r2 := spostaRiga(rigaParole(946, 29, "can", 783.0, 44.0, "sprint", 838.0, 79.0, "by", 929.0, 28.0,
		"holding", 969.0, 100.0, "the", 1077.0, 44.0, "Shift", 1131.0, 61.0, "key.", 1204.0, 47.0), 560, 900)
	b2 := bloccoDaParole(r2.Parole, 1)
	b2.Testo = testoParole(r2.Parole)
	if m := controllaCompleta(img, &b2, nil); m == "" {
		t.Error("\"can sprint ... key.\" con \"TIP: You\" non letto accettata")
	}
	// parola lunga non letta accanto a una frase completa: resta incompleta (anche a destra)
	sf, ink := color.RGBA{12, 12, 16, 255}, color.RGBA{230, 230, 230, 255}
	frase := rigaParole(500, 28, "Repair", 400.0, 90.0, "your", 498.0, 60.0, "weapons", 566.0, 120.0, "now.", 694.0, 60.0)
	for _, persa := range []RigaOCR{
		rigaParole(500, 28, "Blacksmiths:", 210.0, 180.0),             // a sinistra, lunga
		rigaParole(500, 28, "Then", 764.0, 60.0, "rest", 832.0, 60.0), // a destra, due parole
	} {
		img2 := immagineFinta(1200, 700, sf)
		disegnaParole(img2, frase, ink)
		disegnaParole(img2, persa, ink)
		b := bloccoDaParole(frase.Parole, 1)
		b.Testo = testoParole(frase.Parole)
		if m := controllaCompleta(img2, &b, nil); m == "" {
			t.Errorf("frase con %q non letto accanto accettata", testoParole(persa.Parole))
		}
	}
	// etichetta corta a destra ("x2", icona): la frase completa si traduce
	img3 := immagineFinta(1200, 700, sf)
	disegnaParole(img3, frase, ink)
	disegnaParole(img3, rigaParole(500, 28, "x2", 768.0, 34.0), ink)
	b3 := bloccoDaParole(frase.Parole, 1)
	b3.Testo = testoParole(frase.Parole)
	if m := controllaCompleta(img3, &b3, nil); m != "" {
		t.Errorf("frase completa con poco inchiostro a destra scartata: %s", m)
	}
}

// 4) 20/23-hud: "Ammo: 24" senza "/ 120". La riga si rilegge (contatore con il seguito accanto) e
// la barra letta come "I" o "l" tra due numeri torna "/": la rilettura vale.
func TestContatoreRiletto(t *testing.T) {
	img := apriBanco3(t, "20-hud") // ritaglio da (780,820)
	r := spostaRiga(rigaParole(862, 24, "Ammo:", 846.0, 94.0, "24", 959.0, 33.0), 780, 820)
	p := pezziIncompleti(img, []RigaOCR{r}, 1, map[int]bool{}, 3)
	if len(p) != 1 || p[0].rett.Max.X < 1073-780 {
		t.Fatalf("\"Ammo: 24\" non riletto fino a \"120\": %v", p)
	}
	for in, out := range map[string]string{
		"Ammo: 24 I 120":  "Ammo: 24 / 120",
		"Ammo: 24 l 12O":  "Ammo: 24 / 120",
		"Ammo: 24|120":    "Ammo: 24/120",
		"Ammo: 24 / 120":  "Ammo: 24 / 120",
		"Press I to open": "Press I to open", // la I tra parole resta
		"Level 2 I think": "Level 2 I think",
	} {
		if got, _ := correggiRigaOCR(in); got != out {
			t.Errorf("%q -> %q, atteso %q", in, got, out)
		}
	}
	if !letturaPiuCompleta("Ammo: 24", "Ammo: 24 I 120") {
		t.Error("la rilettura \"Ammo: 24 I 120\" non vale")
	}
}

// 5) 08/14-sottotitolo: lettere bianche con il bordino scuro su un fondo chiaro. La terza
// preparazione della rilettura tiene solo il bianco (le lettere), e la riga incerta si rilegge
// con il riquadro allargato fino alla parola letta a metà ("nettling" per "Something").
func TestSottotitoliBianchiSuChiaro(t *testing.T) {
	for _, c := range []struct {
		nome string
		r    image.Rectangle // la riga nel ritaglio
	}{
		{"08-sottotitolo", image.Rect(100, 25, 710, 85)},
		{"14-sottotitolo", image.Rect(66, 30, 900, 98)},
	} {
		img := apriBanco3(t, c.nome)
		rit := image.NewRGBA(image.Rect(0, 0, c.r.Dx(), c.r.Dy()))
		for y := 0; y < c.r.Dy(); y++ {
			for x := 0; x < c.r.Dx(); x++ {
				rit.SetRGBA(x, y, img.RGBAAt(c.r.Min.X+x, c.r.Min.Y+y))
			}
		}
		b := Blocco{X: 3, Y: 3, W: rit.Bounds().Dx() - 6, H: rit.Bounds().Dy() - 6}
		coloriBloccoSemplice(rit, &b)
		if lettereBianche(rit, b) == nil {
			t.Errorf("%s: lettere bianche su fondo chiaro non riconosciute", c.nome)
		}
	}
	// lettere chiare su fondo scuro (26-impostazioni): la preparazione resta quella di sempre
	img := apriBanco3(t, "26-impostazioni")
	rit := image.NewRGBA(image.Rect(0, 0, 480, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 480; x++ {
			rit.SetRGBA(x, y, img.RGBAAt(20+x, 5+y))
		}
	}
	b := Blocco{X: 3, Y: 3, W: 474, H: 54}
	coloriBloccoSemplice(rit, &b)
	if lettereBianche(rit, b) != nil {
		t.Error("26-impostazioni: preparazione per lettere bianche usata su fondo scuro")
	}
	// 14: la riga letta "nettling is rnoving in the baserr ent." (da 626) si rilegge da "Something" (556)
	img14 := apriBanco3(t, "14-sottotitolo") // ritaglio da (480,880)
	r := spostaRiga(rigaParole(929, 34, "nettling", 626.0, 150.0, "is", 790.0, 30.0, "rnoving", 835.0, 150.0, "in", 1000.0, 30.0,
		"the", 1045.0, 60.0, "baserr", 1120.0, 140.0, "ent.", 1270.0, 92.0), 480, 880)
	x0, _, x1, _, _ := rettRiga(r, 1)
	if n0, _ := allargaSuInchiostro(img14, []RigaOCR{r}, 0, 1, x0, x1); n0 > 556-480+8 { // (la rilettura aggiunge ancora 0,6 altezze di margine)
		t.Errorf("riquadro della rilettura non allargato fino a \"Something\": x0=%d", n0)
	}
}
