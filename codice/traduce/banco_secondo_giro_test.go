package traduce

// Secondo giro sul banco di prova vero (60 schermate, OCR di Windows): file pulsanti lette come
// una riga, nomi di fantasia nei titoli, parole attaccate con una lettera confusa, etichette lette
// a parte, testo storto (cartelli ruotati), parole non lette accanto alla riga e scritte a basso
// contrasto mai lette. Coordinate e letture prese dal report vero; ogni regola è per tutti i giochi.

import (
	"image"
	"image/color"
	"math"
	"strings"
	"testing"
	"time"
)

// riga di parole con le coordinate date (x, larghezza) sulla stessa linea.
func rigaParole(y, h float64, parole ...any) RigaOCR {
	var r RigaOCR
	for i := 0; i+2 < len(parole); i += 3 {
		r.Parole = append(r.Parole, ParolaOCR{T: parole[i].(string), X: parole[i+1].(float64), Y: y, W: parole[i+2].(float64), H: h})
	}
	return r
}

// disegnaParole: ogni parola come aste sottili (lettere) che riempiono il suo riquadro.
func disegnaParole(img *image.RGBA, r RigaOCR, c color.RGBA) {
	for _, w := range r.Parole {
		for x := int(w.X); x+3 <= int(w.X+w.W); x += 5 {
			for y := int(w.Y); y < int(w.Y+w.H); y++ {
				for k := 0; k < 3; k++ {
					img.SetRGBA(x+k, y, c)
				}
			}
		}
	}
}

// 1) "Apply   Reset to Default   Back" (26-impostazioni, una riga dell'OCR): tre pulsanti, ognuno
// tradotto e coperto per conto suo, e nessuno scartato come "riga incompleta" per l'inchiostro
// del pulsante vicino.
func TestFilaPulsantiDivisa(t *testing.T) {
	r := rigaParole(1017, 24, "Apply", 140.0, 81.0, "Reset", 240.8, 77.0, "to", 324.8, 27.0, "Default", 358.6, 99.0, "Back", 477.9, 68.0)
	sf, ink := color.RGBA{18, 22, 30, 255}, color.RGBA{200, 200, 200, 255}
	img := immagineFinta(800, 1080, sf)
	disegnaParole(img, r, ink)
	bl := blocchiLetti(img, []RigaOCR{r}, 1, nil, time.Now())
	if got := strings.Join(chiaviBlocchi(bl), " | "); got != "Apply | Reset to Default | Back" {
		t.Fatalf("fila di pulsanti: %q", got)
	}
	for i := range bl {
		b := bl[i]
		if scrittaContinuaTra(img, b, bl) {
			t.Errorf("%q: l'inchiostro del pulsante vicino preso per una parola non letta", b.Testo)
		}
		if m := controllaCompleta(img, &b, bl); m != "" {
			t.Errorf("%q scartato: %s", b.Testo, m)
		}
	}
	// righe che restano intere: frasi, titoli spaziati, comandi
	for _, r := range []RigaOCR{
		rigaParole(931, 35, "MARCUS:", 286.0, 189.0, "I", 487.8, 14.0, "don't", 514.7, 113.0, "trust", 640.1, 104.0, "him.", 756.4, 94.0, "He", 863.4, 59.0, "knows", 934.9, 138.0),
		rigaParole(872, 31, "Press", 820.3, 81.0, "SPACE", 910.0, 83.0, "to", 1000.8, 29.0, "skip", 1038.1, 62.0),
		rigaParole(200, 60, "IRON", 646.0, 180.0, "HARBOR", 876.0, 260.0), // titolo con le lettere spaziate
		rigaParole(200, 60, "SHADOWS", 165.0, 300.0, "OF", 520.0, 70.0, "THE", 640.0, 120.0, "NORTH", 810.0, 220.0),
		rigaParole(305, 32, "Graphics", 160.0, 126.0, "Quality", 294.3, 104.0),
		rigaParole(400, 30, "Go", 100.0, 40.0, "to", 150.0, 30.0, "the", 210.0, 50.0, "Map", 270.0, 60.0),       // buco prima di "the": la frase continua
		rigaParole(400, 30, "Return", 100.0, 90.0, "to", 200.0, 30.0, "Main", 260.0, 70.0, "Menu", 340.0, 70.0), // dopo "to" la frase continua
	} {
		bl := blocchiLetti(nil, []RigaOCR{r}, 1, nil, time.Now())
		if len(bl) != 1 {
			t.Errorf("riga divisa per sbaglio: %q", chiaviBlocchi(bl))
		}
	}
}

// 2) Titolo con un nome di fantasia: "EMBER KINGDOM" -> "REGNO DI EMBER" va bene (il nome resta);
// le mezze traduzioni vere restano scartate.
func TestTitoloConNomeDiFantasia(t *testing.T) {
	for _, c := range [][2]string{
		{"EMBER KINGDOM", "REGNO DI EMBER"},
		{"Ember Kingdom", "Il regno di Ember"},
		{"THE RAVEN THRONE", "IL TRONO DI RAVEN"},
	} {
		if rigaMezzaTradotta(c[0], c[1]) {
			t.Errorf("%q -> %q: il nome nel titolo va bene", c[0], c[1])
		}
	}
	for _, c := range [][2]string{
		{"NEW GAME", "NUOVO GAME"},                   // parola comune: andava tradotta
		{"SHADOWS OF THE NORTH", "SHADOWS DEL NORD"}, // "shadow" è comune
		{"IRON HARBOR", "PORTO IRON"},
		{"EMBER KINGDOM", "EMBER KINGDOM DI"},             // niente tradotto davvero
		{"Find the ember stone", "Trova la ember pietra"}, // frase, non titolo
		{"EMBER RAVEN KINGDOM", "REGNO EMBER RAVEN"},      // due parole lasciate: troppo
	} {
		if !rigaMezzaTradotta(c[0], c[1]) {
			t.Errorf("%q -> %q: è mezza in inglese", c[0], c[1])
		}
	}
}

// 3) Parole attaccate dall'OCR con una lettera confusa: riparate prima di tradurre.
func TestParoleAttaccateConfuse(t *testing.T) {
	for in, out := range map[string]string{
		"MARCUS: I don't trusthifn. He knows more than he's telling us.": "MARCUS: I don't trust him. He knows more than he's telling us.",
		"I can seethern now.":                   "I can see them now.",
		"Palworld is great":                     "Palworld is great", // i nomi non si toccano
		"nettling is moving in the baser ent.":  "nettling is moving in the baser ent.",
		"nettling is moving in the baserr ent.": "nettling is moving in the baser ent.", // una parola letta male, non due ("has err")
	} {
		if got, _ := correggiRigaOCR(in); got != out {
			t.Errorf("%q -> %q, atteso %q", in, got, out)
		}
	}
}

// 4) "TIP:" letto a parte, la frase dopo sulla stessa linea: tornano una scritta sola se in mezzo
// non manca niente; con inchiostro in mezzo (parola non letta) no.
func TestEtichettaLettaAParte(t *testing.T) {
	sf, ink := color.RGBA{10, 10, 14, 255}, color.RGBA{230, 230, 230, 255}
	tip := rigaParole(946, 22, "TIP:", 619.0, 49.0)
	frase := rigaParole(946, 22, "You", 710.0, 50.0, "can", 768.0, 48.0, "sprint", 824.0, 80.0)
	img := immagineFinta(1300, 1080, sf)
	disegnaParole(img, tip, ink)
	disegnaParole(img, frase, ink)
	bl := blocchiLetti(img, []RigaOCR{tip, frase}, 1, nil, time.Now())
	if got := strings.Join(chiaviBlocchi(bl), " | "); got != "TIP: You can sprint" {
		t.Fatalf("etichetta non unita: %q", got)
	}
	// parola non letta in mezzo: restano separate (e la frase resta incompleta)
	img2 := immagineFinta(1300, 1080, sf)
	disegnaParole(img2, tip, ink)
	disegnaParole(img2, frase, ink)
	disegnaParole(img2, rigaParole(946, 22, "X", 676.0, 22.0), ink)
	if bl := blocchiLetti(img2, []RigaOCR{tip, frase}, 1, nil, time.Now()); len(bl) != 2 {
		t.Fatalf("unite con una parola persa in mezzo: %q", chiaviBlocchi(bl))
	}
}

// 5a) Testo storto: Windows dà i rettangoli nell'immagine raddrizzata (TextAngle). Il report vero
// di 42-cartello aveva "STORAGE" a (484,782) invece che a (463,748): riportato indietro torna
// sulle lettere vere.
func TestRaddrizzaParole(t *testing.T) {
	righe := []RigaOCR{{Parole: []ParolaOCR{{T: "STORAGE", X: 484, Y: 782, W: 208, H: 60}}},
		{Parole: []ParolaOCR{{T: "KEEP", X: 1479, Y: 692, W: 96, H: 48}}}}
	out := raddrizzaParole(righe, 4.3, 1920, 1080)
	centro := func(w ParolaOCR) (float64, float64) { return w.X + w.W/2, w.Y + w.H/2 }
	for i, vero := range [][2]float64{{567, 784}, {1512, 764}} {
		x, y := centro(out[i].Parole[0])
		if math.Abs(x-vero[0]) > 8 || math.Abs(y-vero[1]) > 10 {
			t.Errorf("%s: centro (%.0f,%.0f), vero (%.0f,%.0f)", out[i].Parole[0].T, x, y, vero[0], vero[1])
		}
	}
	if same := raddrizzaParole(righe, 0, 1920, 1080); same[0].Parole[0].X != 484 {
		t.Error("angolo 0: niente da cambiare")
	}
}

// 5b) Cartello ruotato (bianco su targa verde, come 42-cartello): i rettangoli come li dà Windows
// (nell'immagine raddrizzata) riportati indietro con l'angolo coprono le lettere vere e la riga è
// completa. Anche più storta (10 gradi) la fascia della prima e dell'ultima parola non prende il
// fondo fuori dalla targa; una parola vera non letta accanto si vede ancora.
func TestCartelloRuotatoCompleto(t *testing.T) {
	for _, gradi := range []float64{4.3, 10} {
		fondo, targa, ink := color.RGBA{20, 40, 50, 255}, color.RGBA{30, 120, 50, 255}, color.RGBA{245, 245, 245, 255}
		img := immagineFinta(1920, 1080, fondo)
		pend := math.Tan(gradi * math.Pi / 180)
		for x := 380; x < 960; x++ { // targa storta, poco più alta delle lettere
			y0 := 720 + int(pend*float64(x-380))
			for y := y0; y < y0+100; y++ {
				img.SetRGBA(x, y, targa)
			}
		}
		parole := []ParolaOCR{{T: "STORAGE", X: 464, W: 207}, {T: "ROOM", X: 682, W: 144}, {T: "B", X: 837, W: 34}}
		for i := range parole {
			w := &parole[i]
			w.Y = 740 + pend*(w.X-380)                        // la cima scende con la targa
			w.H = 60 + pend*w.W                               // rettangolo dritto della parola storta
			for x := int(w.X); x+6 <= int(w.X+w.W); x += 10 { // aste delle lettere, storte come la targa
				top := 740 + int(pend*float64(x-380))
				for y := top; y < top+60; y++ {
					for k := 0; k < 6; k++ {
						img.SetRGBA(x+k, y, ink)
					}
				}
			}
		}
		// come le dava Windows: nell'immagine raddrizzata (ruotata indietro attorno al centro)
		storte := raddrizzaParole([]RigaOCR{{Parole: parole}}, -gradi, 1920, 1080)
		for i := range storte[0].Parole { // Windows dà le parole dritte: alte come le lettere
			storte[0].Parole[i].W, storte[0].Parole[i].H = parole[i].W, 60
		}
		dritte := raddrizzaParole(storte, gradi, 1920, 1080)
		b := bloccoDaParole(dritte[0].Parole, 1)
		if scrittaContinuaTra(img, b, nil) {
			t.Fatalf("%.1f gradi: cartello preso per una riga incompleta (%d,%d %dx%d)", gradi, b.X, b.Y, b.W, b.H)
		}
		// una parola vera non letta dopo "B" sulla targa si vede ancora
		for x := 892; x+6 <= 932; x += 10 {
			top := 740 + int(pend*float64(x-380))
			for y := top; y < top+60; y++ {
				for k := 0; k < 6; k++ {
					img.SetRGBA(x+k, y, ink)
				}
			}
		}
		if !scrittaContinuaTra(img, b, nil) {
			t.Fatalf("%.1f gradi: parola non letta sulla targa non vista", gradi)
		}
	}
}

// 7) Zone con la forma di una scritta dove l'OCR non ha letto niente: grigio 170 su beige 228 in
// un riquadro di dialogo ("Press Enter to continue"). Non: le righe lette, il lato storto di un
// disegno, il bordo verticale di un riquadro.
func TestZoneTestoNonLetto(t *testing.T) {
	img := immagineFinta(1920, 1080, color.RGBA{120, 70, 40, 255})
	pannello := color.RGBA{228, 222, 205, 255}
	for y := 750; y < 1030; y++ {
		for x := 180; x < 1740; x++ {
			img.SetRGBA(x, y, pannello)
		}
	}
	letta := rigaParole(841, 31, "Welcome,", 230.0, 142.0, "traveler!", 380.0, 118.0)
	disegnaParole(img, letta, color.RGBA{30, 30, 30, 255})
	nonLetta := rigaParole(996, 18, "Press", 1427.0, 63.0, "Enter", 1497.0, 59.0, "to", 1563.0, 22.0, "continue", 1592.0, 98.0)
	disegnaParole(img, nonLetta, color.RGBA{170, 170, 170, 255})
	// disegno con un lato storto e un bordo verticale, fuori dal pannello
	for y := 100; y < 400; y++ {
		for x := 300; x < 300+(y-100)*2; x++ {
			img.SetRGBA(x, y, color.RGBA{60, 60, 200, 255})
		}
		for x := 1500; x < 1510; x++ {
			img.SetRGBA(x, y, color.RGBA{250, 250, 250, 255})
		}
	}
	zone := zoneTestoNonLetto(img, []RigaOCR{letta}, 1, 3)
	if len(zone) != 1 {
		t.Fatalf("zone: %v (attesa una: Press Enter to continue)", zone)
	}
	z := zone[0]
	if !(image.Rect(1427, 996, 1690, 1014)).In(z) || z.Dx() > 400 || z.Dy() > 60 {
		t.Fatalf("zona %v non copre la scritta (1427,996)-(1690,1014)", z)
	}
	// con la scritta letta, nessuna zona
	if zone := zoneTestoNonLetto(img, []RigaOCR{letta, nonLetta}, 1, 3); len(zone) != 0 {
		t.Fatalf("zone su righe già lette: %v", zone)
	}
	// il ritaglio a basso contrasto diventa nero su bianco pieno (contrasto stirato)
	rit := image.NewRGBA(image.Rect(0, 0, z.Dx(), z.Dy()))
	for y := 0; y < z.Dy(); y++ {
		for x := 0; x < z.Dx(); x++ {
			rit.SetRGBA(x, y, img.RGBAAt(z.Min.X+x, z.Min.Y+y))
		}
	}
	g := variantiRiga(rit, 2)[0]
	lo, hi := 255, 0
	for _, v := range g.Pix {
		lo, hi = min(lo, int(v)), max(hi, int(v))
	}
	if lo > 10 || hi < 245 {
		t.Fatalf("contrasto non stirato: %d..%d", lo, hi)
	}
}

// 4/6) Righe incomplete rilette allargate: "You can sprint…" con "TIP:" non letto a sinistra,
// "Press SPACE to" senza "skip", "Ammo: 24" con "120" letto a parte (e "/" perso in mezzo).
// La rilettura (finta qui: l'OCR di Windows) prende il posto della riga e dei pezzi inglobati;
// la zona non letta diventa una riga nuova. Una lettura peggiore non cambia niente.
func TestRileggiIncompleteEZone(t *testing.T) {
	sf, ink := color.RGBA{12, 12, 16, 255}, color.RGBA{235, 235, 235, 255}
	img := immagineFinta(1920, 1080, sf)
	tip := rigaParole(946, 22, "TIP:", 667.0, 49.0)
	frase := rigaParole(946, 22, "You", 723.7, 50.0, "can", 781.4, 48.0, "sprint", 837.2, 80.0)
	disegnaParole(img, tip, ink)
	disegnaParole(img, frase, ink)
	ammo := rigaParole(866, 19, "Ammo:", 845.2, 103.0, "24", 956.8, 36.0)
	cento := rigaParole(866, 19, "120", 1020.6, 54.0)
	disegnaParole(img, ammo, ink)
	disegnaParole(img, cento, ink)
	disegnaParole(img, rigaParole(866, 21, "/", 1002.0, 10.0), ink)
	press := rigaParole(500, 31, "Press", 820.3, 81.0, "SPACE", 910.0, 83.0, "to", 1000.8, 29.0)
	disegnaParole(img, press, ink)
	disegnaParole(img, rigaParole(500, 31, "skip", 1038.1, 62.0), ink)
	righe := []RigaOCR{frase, ammo, cento, press}

	pezzi := pezziIncompleti(img, righe, 1, nil, 4)
	trovato := map[string]pezzoRilettura{}
	for _, p := range pezzi {
		trovato[testoParole(righe[p.riga].Parole)] = p
	}
	if p, ok := trovato["You can sprint"]; !ok || p.rett.Min.X > 667 {
		t.Fatalf("\"You can sprint\" non riletta con TIP: %+v", pezzi)
	}
	if p, ok := trovato["Ammo: 24"]; !ok || len(p.ingoia) != 1 || p.rett.Max.X < 1074 {
		t.Fatalf("\"Ammo: 24\" non riletta insieme a \"120\": %+v", pezzi)
	}
	if p, ok := trovato["Press SPACE to"]; !ok || p.rett.Max.X < 1100 {
		t.Fatalf("\"Press SPACE to\" non riletta con skip: %+v", pezzi)
	}

	risposte := map[string]string{"You can sprint": "TIP: You can sprint", "Ammo: 24": "Ammo: 24 / 120", "Press SPACE to": "Press SPACE to skip"}
	var visti []pezzoRilettura
	osservaPezzi = func(s []pezzoRilettura) { visti = s }
	defer func() { osservaPezzi = nil }()
	leggi := func(_ *image.Gray, dove []image.Rectangle) ([]RigaOCR, error) {
		var out []RigaOCR
		for n, p := range visti {
			testo := "qwzx vvkq" // la zona e le altre preparazioni: spazzatura
			if p.riga >= 0 {
				testo = risposte[testoParole(righe[p.riga].Parole)]
			}
			d := dove[n*3]
			var r RigaOCR
			x := float64(d.Min.X + 10)
			for _, w := range strings.Fields(testo) {
				r.Parole = append(r.Parole, ParolaOCR{T: w, X: x, Y: float64(d.Min.Y + 8), W: 40, H: 30})
				x += 50
			}
			out = append(out, r)
		}
		return out, nil
	}
	out := rileggiIncerteCon(img, righe, 1, leggi)
	var testi []string
	for _, r := range out {
		if len(r.Parole) > 0 {
			testi = append(testi, testoParole(r.Parole))
		}
	}
	if got := strings.Join(testi, " | "); got != "TIP: You can sprint | Ammo: 24 / 120 | Press SPACE to skip" {
		t.Fatalf("dopo la rilettura: %q", got)
	}
	// rilettura sbagliata (un'altra scritta, o spazzatura): la riga resta com'era
	if letturaPiuCompleta("You can sprint", "Objective: Reach the extraction point") || letturaPiuCompleta("You can sprint", "You can") {
		t.Error("rilettura non pertinente accettata")
	}
	if letturaNuovaBuona("qwzx vvkq") || !letturaNuovaBuona("Press Enter to continue") || letturaNuovaBuona("90") {
		t.Error("controllo delle scritte nuove")
	}
}
