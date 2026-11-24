package traduce

import (
	"image"
	"image/color"
	"testing"
)

// La rotella sposta l'elenco: le scritte si ritrovano più su → pagina spostata.
// Mouse sopra una riga, riquadri illuminati, finestrella di descrizione: le scritte restano
// ferme → solo cambio di colore, le traduzioni restano (No Man's Sky, inventario).
func TestPaginaSpostata(t *testing.T) {
	const W, H = 400, 300
	// righe con "lettere" diverse: barrette chiare a posizioni diverse per ogni riga
	disegna := func(off int) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, W, H))
		for i := range img.Pix {
			img.Pix[i] = 40
		}
		for r := 0; r < 7; r++ {
			y0 := 20 + r*40 - off
			for y := y0 + 4; y < y0+16; y++ {
				if y < 0 || y >= H {
					continue
				}
				for x := 10; x < 360; x++ {
					if (x*(r+3)+(y-y0)*(r%3+1))%11 < 4 {
						img.SetRGBA(x, y, color.RGBA{230, 230, 230, 255})
					}
				}
			}
		}
		return img
	}
	a := disegna(0)
	var bl []Blocco
	for r := 0; r < 5; r++ {
		bl = append(bl, Blocco{X: 10, Y: 20 + r*40, W: 150, H: 20, Img: a})
		bl = append(bl, Blocco{X: 250, Y: 20 + r*40, W: 100, H: 20, Img: a})
	}
	copia := func() *image.RGBA {
		c := image.NewRGBA(a.Bounds())
		copy(c.Pix, a.Pix)
		return c
	}
	colora := func(img *image.RGBA, y0, y1 int) {
		for y := y0; y < y1; y++ {
			for x := 0; x < W; x++ {
				img.SetRGBA(x, y, color.RGBA{220, 190, 60, 255})
			}
		}
	}
	mouse := copia()
	colora(mouse, 60, 80)
	if paginaSpostata(mouse, bl) {
		t.Fatal("il mouse sopra una riga non deve contare come rotella")
	}
	tante := copia()
	colora(tante, 20, 140) // finestrella o riquadri illuminati su tre righe: le scritte non si spostano
	if paginaSpostata(tante, bl) {
		t.Fatal("una finestrella sopra tre righe non è una rotella")
	}
	luce := copia()
	for i := 0; i < len(luce.Pix); i += 4 { // tutto più chiaro (sfondo animato)
		luce.Pix[i] = min(255, luce.Pix[i]+30)
	}
	if paginaSpostata(luce, bl) {
		t.Fatal("uno sfondo che cambia luce non è una rotella")
	}
	if !paginaSpostata(disegna(15), bl) {
		t.Fatal("la rotella che sposta l'elenco di 15 pixel non è stata vista")
	}
	if !paginaSpostata(disegna(-40), bl) {
		t.Fatal("la rotella in giù di una riga non è stata vista")
	}
}

func TestScenaCambiata(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 100, 100))
	bl := []Blocco{{X: 0, Y: 0, W: 50, H: 20, Img: a}, {X: 0, Y: 40, W: 50, H: 20, Img: a}, {X: 0, Y: 70, W: 50, H: 20, Img: a}}
	b := image.NewRGBA(a.Bounds())
	for y := 70; y < 90; y++ {
		for x := 0; x < 50; x++ {
			b.SetRGBA(x, y, color.RGBA{200, 200, 200, 255})
		}
	}
	if scenaCambiata(b, bl) {
		t.Fatal("una sola scritta cambiata non è una scena nuova")
	}
	for i := range b.Pix {
		b.Pix[i] = 200
	}
	if !scenaCambiata(b, bl) {
		t.Fatal("tutto cambiato: scena nuova")
	}
}

func TestTieniFerme(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 100, 100))
	vecchio := Blocco{Testo: "CARGO", Tradotto: "CARICO", X: 10, Y: 10, W: 40, H: 12, Em: 11, Img: a}
	nuovo := Blocco{Testo: "CARGO", Tradotto: "CARICO", X: 11, Y: 9, W: 41, H: 13, Em: 12, Img: a}
	altro := Blocco{Testo: "Used for:", Tradotto: "Utilizzato per:", X: 10, Y: 50, W: 40, H: 12, Img: a}
	out := tieniFerme([]Blocco{nuovo, altro}, []Blocco{vecchio}, a)
	if out[0].X != 10 || out[0].Em != 11 || out[1].Testo != "Used for:" {
		t.Fatalf("la scritta ferma deve restare identica: %+v", out[0])
	}
	b := image.NewRGBA(a.Bounds())
	for i := range b.Pix {
		b.Pix[i] = 220
	}
	if out := tieniFerme([]Blocco{nuovo}, []Blocco{vecchio}, b); out[0].X != 11 {
		t.Fatal("se l'inglese sotto è cambiato si usa la lettura nuova")
	}
}

func TestTitoloMaiuscoloNonSiUnisce(t *testing.T) {
	if continuaFrase("CRAFT MORE", "reate additional Microprocessor") {
		t.Fatal("il titolo maiuscolo non continua nella riga sotto")
	}
	if !continuaFrase("Stack items, split stacks or move", "to other inventories") {
		t.Fatal("una frase normale andata a capo deve unirsi")
	}
}

func TestBordiCambiati(t *testing.T) {
	fai := func(luceFondo uint8, scritta int) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, 640, 240))
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = luceFondo, luceFondo/2, 30, 255
		}
		for y := 100; y < 116; y++ { // una "parola": barrette verticali
			for x := 50; x < 250; x++ {
				if (x+scritta)%7 < 2 {
					img.SetRGBA(x, y, color.RGBA{250, 250, 250, 255})
				}
			}
		}
		return img
	}
	a := fai(60, 0)
	if bordiCambiati(a, fai(90, 0)) {
		t.Fatal("solo lo sfondo più chiaro: le scritte sono le stesse")
	}
	if !bordiCambiati(a, fai(60, 3)) {
		t.Fatal("scritta diversa: va riletta")
	}
}

func TestStesseScritte(t *testing.T) {
	a := []Blocco{{Testo: "CARGO"}, {Testo: "Used for:"}, {Testo: "DISCARD"}, {Testo: "Units"}, {Testo: "Entity"}}
	b := []Blocco{{Testo: "CARGO"}, {Testo: "Used for:"}, {Testo: "DISCARD"}, {Testo: "Units"}, {Testo: "Quicksilver"}}
	if !stesseScritte(a, b) {
		t.Fatal("una scritta su cinque diversa: stessa pagina")
	}
	c := []Blocco{{Testo: "SODIUM"}, {Testo: "Used for:"}, {Testo: "A common element"}, {Testo: "Units"}, {Testo: "Entity"}}
	if stesseScritte(a, c) {
		t.Fatal("finestrella di un altro oggetto: scritte diverse")
	}
}

func TestZonaMenu(t *testing.T) {
	if !sembraMenu("General Audio\nAudio Output Device System Default Device\nGame Volume") {
		t.Fatal("impostazioni: righe corte")
	}
	if sembraMenu("I told you to bring the cargo back before the storm hits the outpost.\nNow we have nothing left.") {
		t.Fatal("un dialogo non è un menu")
	}
	r := righeDiBlocchi([]Blocco{{Testo: "System Default Device", X: 900, Y: 52, H: 20}, {Testo: "General Audio", X: 400, Y: 10, H: 22}, {Testo: "Audio Output Device", X: 400, Y: 50, H: 20}})
	if len(r) != 2 || r[1][0].Testo != "Audio Output Device" || r[1][1].Testo != "System Default Device" {
		t.Fatalf("%+v", r)
	}
}

func TestDialogoNonEMenu(t *testing.T) {
	if sembraMenu("JOHN\nWe need to go now.") {
		t.Fatal("nome + battuta preso per menu")
	}
	if sembraMenu("JOHN\nWe need to go.\nNow!") {
		t.Fatal("dialogo su più righe preso per menu")
	}
	if !sembraMenu("Volume\nMusic\nSound Effects\nVoice") {
		t.Fatal("menu non riconosciuto")
	}
}

// fondoConScritte: sfondo scuro con due "voci di menu" chiare (barrette come lettere).
func fondoConScritte(colore1 color.RGBA, forma2 int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 400, 200))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 20, 24, 30, 255
	}
	for y := 40; y < 56; y++ {
		for x := 30; x < 150; x++ {
			if (x+y)%6 < 2 {
				img.SetRGBA(x, y, colore1)
			}
		}
	}
	for y := 100; y < 116; y++ {
		for x := 30; x < 150; x++ {
			if (x*forma2+y)%7 < 2 {
				img.SetRGBA(x, y, color.RGBA{235, 235, 235, 255})
			}
		}
	}
	return img
}

// Mouse sopra una voce: diventa dorata. Il resto dello schermo è identico, quindi l'impronta di
// tutta la schermata cambia poco; la tinta della voce invece sì, e solo lei va ricolorata.
func TestTintaMouseSopra(t *testing.T) {
	bianco := color.RGBA{235, 235, 235, 255}
	a := fondoConScritte(bianco, 1)
	bl := []Blocco{
		{Testo: "Gear", Tradotto: "Equipaggiamento", X: 30, Y: 40, W: 120, H: 16, Righe: 1, Em: 16, Img: a},
		{Testo: "Options", Tradotto: "Opzioni", X: 30, Y: 100, W: 120, H: 16, Righe: 1, Em: 16, Img: a},
	}
	for i := range bl {
		coloriBlocco(a, &bl[i])
		bl[i].Tinta = tintaBlocco(a, bl[i])
	}
	bl[0].Pulita, bl[1].Pulita = pulisciPagina(a, bl), pulisciPagina(a, bl)
	viste := map[[4]int][12]uint8{}
	if _, n := blocchiDaRicolorare(a, bl, viste); n != 0 {
		t.Fatal("niente è cambiato: niente da ricolorare")
	}
	oro := fondoConScritte(color.RGBA{240, 190, 60, 255}, 1)
	solo, n := blocchiDaRicolorare(oro, bl, viste)
	if n != 1 || !solo[0] || solo[1] {
		t.Fatalf("solo la voce sotto il mouse va ricolorata: %v", solo)
	}
	nuovi, ok := ricoloraSolo(oro, bl, solo)
	if !ok || distanza(nuovi[0].Inchiostro, color.RGBA{240, 190, 60, 255}) > 60 {
		t.Fatalf("la traduzione non è diventata dorata: %v %v", ok, nuovi[0].Inchiostro)
	}
	if nuovi[1].Inchiostro != bl[1].Inchiostro || nuovi[1].Tinta != bl[1].Tinta {
		t.Fatal("la voce non toccata deve restare identica")
	}
	if nuovi[0].Img != oro || nuovi[1].Img != oro || nuovi[0].Pulita != nuovi[1].Pulita {
		t.Fatal("tutte le scritte devono puntare alla cattura nuova (le vecchie si liberano)")
	}
	// stessa immagine di nuovo: la tinta già vista non fa ricontrollare
	nuovi[0].Tinta = [12]uint8{1}
	if _, n := blocchiDaRicolorare(oro, nuovi, viste); n != 0 {
		t.Fatal("tinta già controllata: niente da rifare")
	}
}

// Una voce con altre lettere sparisce subito; una che ha cambiato solo colore resta.
func TestTogliCambiate(t *testing.T) {
	a := fondoConScritte(color.RGBA{235, 235, 235, 255}, 1)
	bl := []Blocco{
		{Testo: "Gear", Tradotto: "Equipaggiamento", X: 30, Y: 40, W: 120, H: 16, Righe: 1, Img: a},
		{Testo: "Options", Tradotto: "Opzioni", X: 30, Y: 100, W: 120, H: 16, Righe: 1, Img: a},
	}
	b := fondoConScritte(color.RGBA{240, 190, 60, 255}, 3) // voce 1 dorata, voce 2 con altre lettere
	resto, tolte := togliCambiate(b, bl)
	if tolte != 1 || len(resto) != 1 || resto[0].Testo != "Gear" {
		t.Fatalf("doveva togliere solo la voce con le lettere cambiate: %d %+v", tolte, resto)
	}
}

// L'OCR legge la stessa scritta piccola in modo un po' diverso: stabilizza tiene la traduzione E
// il testo di prima, così tieniFerme la lascia identica (niente tremolio).
func TestStabilizzaPoiTieniFerme(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 300, 100))
	vecchio := Blocco{Testo: "Used for crafting", Tradotto: "Usato per creare", X: 10, Y: 10, W: 120, H: 12, Righe: 1, Img: a}
	nuovo := Blocco{Testo: "Usedfor crafting", Tradotto: "Usato per artigianato", X: 11, Y: 9, W: 121, H: 13, Righe: 1, Img: a}
	n := []Blocco{nuovo}
	stabilizza(n, []Blocco{vecchio})
	out := tieniFerme(n, []Blocco{vecchio}, a)
	if out[0].X != 10 || out[0].Y != 10 || out[0].Tradotto != "Usato per creare" {
		t.Fatalf("la scritta doveva restare ferma: %+v", out[0])
	}
}

// La scritta tenuta punta alla cattura nuova e la sua cancellatura passa nella pagina pulita nuova.
func TestTieniFermePulitaNuova(t *testing.T) {
	a := fondoConScritte(color.RGBA{235, 235, 235, 255}, 1)
	vecchio := Blocco{Testo: "Gear", Tradotto: "Equipaggiamento", X: 30, Y: 40, W: 120, H: 16, Righe: 1, Em: 16, Img: a}
	coloriBlocco(a, &vecchio)
	vecchio.Pulita = pulisciPagina(a, []Blocco{vecchio})
	b := image.NewRGBA(a.Bounds()) // cattura nuova, identica
	copy(b.Pix, a.Pix)
	pulNuova := image.NewRGBA(b.Bounds())
	copy(pulNuova.Pix, b.Pix) // (la lettura nuova qui non ha cancellato niente)
	nuovo := vecchio
	nuovo.X, nuovo.Img, nuovo.Pulita = 31, b, pulNuova
	out := tieniFerme([]Blocco{nuovo}, []Blocco{vecchio}, b)
	if out[0].X != 30 || out[0].Img != b || out[0].Pulita != pulNuova {
		t.Fatalf("doveva tenere la vecchia, puntata alle immagini nuove: %+v", out[0])
	}
	diversi := 0
	for i := range pulNuova.Pix {
		if pulNuova.Pix[i] != b.Pix[i] {
			diversi++
		}
	}
	if diversi == 0 {
		t.Fatal("le lettere cancellate della scritta tenuta non sono passate nella pagina pulita nuova")
	}
}

// La traduzione resta nel riquadro: senza bordo trovato poco più dell'inglese, mai fino alla
// scritta vicina o al bordo dello schermo; con il bordo vero fino a lì.
func TestLargConsentita(t *testing.T) {
	b := Blocco{X: 100, W: 40, Em: 16, LargMax: 900}
	if l := largConsentita(b); l > 40+40 || l < 60 {
		t.Fatalf("senza bordo: %d", l)
	}
	b.Bordo, b.LargMax = true, 200
	if l := largConsentita(b); l != 200 {
		t.Fatalf("con il bordo del riquadro: %d", l)
	}
	b.Bordo, b.LargMax = false, 50
	if l := largConsentita(b); l != 50 {
		t.Fatalf("mai oltre la scritta vicina: %d", l)
	}
}
