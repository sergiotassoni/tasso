package traduce

// Collaudo finale 26/11 (prova su 6 giochi mai visti e banchi della notte e del mattino): la
// traduzione vecchia sulla voce nuova di una lista che scorre, le maiuscole prese dalla memoria,
// la decisione di tradurre che cambiava a ogni fotogramma, i nomi di chi parla tradotti, "Ms."
// che spezzava la frase, i pezzi del report con la traduzione della riga intera.

import (
	"context"
	"image"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Voci diverse di un inventario non sono la stessa scritta, anche con un numero di parole diverso.
func TestCollaudoListaVociDiverse(t *testing.T) {
	for _, c := range [][2]string{
		{"Rusty Iron Cuirass", "Rusty Iron War Axe"},
		{"Rusty Iron Greaves", "Rusty Iron War Axe"},
		{"Rusty Iron Cuirass", "Rusty Iron Greaves"},
		{"Teacup with Vinegar", "Teacup with Water"},
		{"Sack Cloth Pants", "Sack Cloth Shirt"},
		{"Weak Potion of Healing", "Weak Potion of Sorcery"},
		{"TUTORIAL: BLOCK ATTACKS", "TUTORIAL: HEAVY ATTACKS"},
		{"Find where Rory hid the sugar.", "Send the sugar to the children."},
	} {
		if stessaScritta(c[0], c[1]) || stessaScritta(c[1], c[0]) {
			t.Errorf("%q / %q: voci diverse prese per la stessa scritta", c[0], c[1])
		}
	}
	// gli errori dell'OCR restano la stessa scritta
	for _, c := range [][2]string{
		{"Use a health station to heal up. Hold until you've fully restored yourself.", "Use a health station to heal up. Hold uniil you've fully restored yourself."},
		{"Use a health station to heal up. Hold until you've fully restored yourself.", "Use a health station to heal up. Hold fully restored yourself."},
		{"Back to Form (Tutorial)", "Back to Form (Tutorichl)"},
		{"Let's get our things together, as far as I'm concerned.", "Let's get our things together, as far as I'm Amcerned."},
		{"Find where Rory hid the sugar.", "Find where Roryhid the sugar."},
		{"Complete the primary weapon", "Completethe primary weapon"},
		{"Rusty Iron War Axe", "Rusty Iron War Xxe"},
	} {
		if !stessaScritta(c[0], c[1]) {
			t.Errorf("%q / %q: è la stessa scritta letta male", c[0], c[1])
		}
	}
}

// L'inventario scorre di una riga: ogni voce tiene la SUA traduzione, mai quella della voce che
// stava lì prima (banco del mattino f.0024: "Rusty Iron War Axe" → "Corazza di ferro arrugginita").
func TestCollaudoListaCheScorre(t *testing.T) {
	voci := []struct{ en, it string }{
		{"Rusty Iron bow", "Arco di ferro arrugginito"},
		{"Rusty Iron Cuirass", "Corazza di ferro arrugginita"},
		{"Rusty Iron Greaves", "Schinieri di ferro arrugginiti"},
		{"Rusty Iron War Axe", "Ascia da guerra di ferro arrugginita"},
		{"Sack Cloth Pants", "Pantaloni di tela grezza"},
	}
	riga := func(i, y int) Blocco {
		return Blocco{Testo: voci[i].en, Tradotto: voci[i].it, X: 218, Y: y, W: 180, H: 20, Righe: 1}
	}
	var vecchi []Blocco
	for i := 0; i < 4; i++ {
		vecchi = append(vecchi, riga(i, 338+i*54))
	}
	// un giro dopo: la lista è salita di una riga (la voce i+1 dove c'era la voce i)
	var nuovi []Blocco
	for i := 1; i < 5; i++ {
		nuovi = append(nuovi, riga(i, 338+(i-1)*54+1))
	}
	stabilizza(nuovi, vecchi)
	for k, n := range nuovi {
		if n.Tradotto != voci[k+1].it {
			t.Errorf("%q a y=%d: mostrata %q (della voce di prima), doveva essere %q", n.Testo, n.Y, n.Tradotto, voci[k+1].it)
		}
	}
	// la stessa lista ferma, letta con un errore dell'OCR: la traduzione già sullo schermo resta
	ferma := []Blocco{riga(3, 500)}
	ferma[0].Testo, ferma[0].Tradotto = "Rusty Iron War Xxe", "Ascia da guerra di ferro arrugginitaa"
	stabilizza(ferma, []Blocco{riga(3, 500)})
	if ferma[0].Tradotto != voci[3].it {
		t.Errorf("stessa voce letta male: traduzione cambiata in %q", ferma[0].Tradotto)
	}
}

// La traduzione presa dalla memoria segue le maiuscole della scritta di adesso.
func TestCollaudoMaiuscoleDellaScritta(t *testing.T) {
	for _, c := range [][3]string{
		{"Reach the camp", "RAGGIUNGI IL CAMPO", "Raggiungi il campo"},
		{"REACH THE CAMP", "Raggiungi il campo", "RAGGIUNGI IL CAMPO"},
		{"Edit License Plate", "MODIFICA LA TARGA", "Modifica la targa"},
		{"Press E to open", "PREMI E PER APRIRE", "Premi E per aprire"},
		{"Bond: Get to the camp.", "BOND: RAGGIUNGI IL CAMPO.", "Bond: raggiungi il campo."},
		{"Your HP is low", "I TUOI PV SONO BASSI", "I tuoi PV sono bassi"},
		{"Reach the camp", "Raggiungi il campo", "Raggiungi il campo"},
		{"Lock On", "Aggancia bersaglio", "Aggancia bersaglio"},
	} {
		if got := maiuscoleDellaScritta(c[0], c[1]); got != c[2] {
			t.Errorf("maiuscoleDellaScritta(%q, %q) = %q, volevo %q", c[0], c[1], got, c[2])
		}
	}
	// nel percorso dello schermo: la traduzione fissata per "REACH THE CAMP" non esce maiuscola
	f := &traduzioniFisse{}
	f.fissa("en>it", "REACH THE CAMP", "RAGGIUNGI IL CAMPO")
	r := traduciTutteFisse(context.Background(), nil, []Blocco{{Testo: "Reach the camp"}}, true, f)
	if r[0].Tradotto != "Raggiungi il campo" {
		t.Errorf("dalla traduzione fissa: %q", r[0].Tradotto)
	}
}

// Una scritta già tradotta e mostrata non torna in inglese per un giudizio che cambia da un
// fotogramma all'altro; le regole contro le righe mezze inglesi restano.
func TestCollaudoDecisioneTenuta(t *testing.T) {
	f := &traduzioniFisse{}
	c := &confermeDecisione{}
	t0 := time.Unix(1000, 0)
	bl := []Blocco{{Testo: "Early Morning", Affid: letturaDubbia}, {Testo: "After School", Affid: letturaDubbia}}
	segnaGiaMostrate(f, c, "en>it", bl, t0)
	if bl[0].GiaMostrata || motivoNonTradurre(bl[0]) != "lettura incerta" {
		t.Fatalf("mai mostrata: la lettura incerta resta in inglese (%+v, %q)", bl[0], motivoNonTradurre(bl[0]))
	}
	f.fissa("en>it", "Early Morning", "Mattina presto")
	mostrata := []Blocco{{Testo: "Early Morning", Tradotto: "Mattina presto"}}
	c.segna("en>it", mostrata, mostrata, t0)
	c.segna("en>it", mostrata, mostrata, t0.Add(250*time.Millisecond))
	segnaGiaMostrate(f, c, "en>it", bl, t0.Add(500*time.Millisecond))
	if !bl[0].GiaMostrata || bl[1].GiaMostrata {
		t.Fatalf("segnate male: %v %v", bl[0].GiaMostrata, bl[1].GiaMostrata)
	}
	if m := motivoNonTradurre(bl[0]); m != "" {
		t.Errorf("già mostrata, stessa scritta: torna in inglese (%q)", m)
	}
	// solo il testo esatto: una lettera diversa può essere un'altra scritta
	if f.mostrataPrima("en>it", "Early Mourning") {
		t.Error("un'altra parola presa per la scritta già mostrata")
	}
	for _, m := range []string{"scritta del mondo (si muove con la scena)", "lettura incerta"} {
		if !motivoTenibile(m) {
			t.Errorf("%q: dovrebbe cedere alla decisione già presa", m)
		}
	}
	for _, m := range []string{motivoFirmaAngolo, "riga incompleta (parola accanto non letta)", "traduzioni sovrapposte",
		motivoMezzaRiga, "pezzo di una frase rimasta in inglese (mai mezza riga tradotta)"} {
		if motivoTenibile(m) {
			t.Errorf("%q: non deve mai cedere (righe mezze inglesi)", m)
		}
	}
}

// Il nome di chi parla resta com'è anche nelle scritte dello schermo, la battuta si traduce.
func TestCollaudoNomeDiChiParla(t *testing.T) {
	if n, b := separaParlante("Cope << You're on flare duty, okay? >>"); n != "Cope" || b != "<< You're on flare duty, okay? >>" {
		t.Errorf("formato radio: %q / %q", n, b)
	}
	moz := &motoreFinto{nome: "Mozilla"}
	tr := NuovoTraduttore(filepath.Join(t.TempDir(), "c.json"), glossarioBase(), moz)
	for _, c := range []struct{ en, nome, battuta string }{
		{"SHADE: Come on, aren't we friends?", "SHADE: ", "Come on, aren't we friends?"},
		{"SHADE: *screams*", "SHADE: ", "*screams*"},
		{"Cope << One more time. >>", "Cope ", "<< One more time. >>"},
		{"Bond: You're dodging the question.", "Bond: ", "You're dodging the question."},
	} {
		r := tr.TraduciScritta(context.Background(), c.en)
		if !strings.HasPrefix(r.Tradotto, c.nome) || strings.Contains(moz.ultim.Testo, strings.TrimSpace(strings.TrimSuffix(c.nome, ": "))) {
			t.Errorf("%q → %q (al traduttore: %q): il nome non doveva passare dal traduttore", c.en, r.Tradotto, moz.ultim.Testo)
		}
	}
	// le etichette con il loro valore non sono battute
	for _, s := range []string{"Lockpicks Left: 3", "Security Level: 5", "Difficulty: Easy", "Objective: Find the key."} {
		if _, _, _, ok := tr.battutaConNome(s); ok {
			t.Errorf("%q: etichetta presa per una battuta", s)
		}
	}
}

// "Ms." non spezza la frase.
func TestCollaudoTitoloConPunto(t *testing.T) {
	if got := senzaPuntoTitoli("I'm Ms. Toriumi. I teach Composition."); got != "I'm Ms Toriumi. I teach Composition." {
		t.Errorf("senzaPuntoTitoli: %q", got)
	}
	if got := senzaPuntoTitoli("Ask Dr. Kim and Sgt. Rivers."); got != "Ask Dr Kim and Sgt Rivers." {
		t.Errorf("senzaPuntoTitoli: %q", got)
	}
	if got := senzaPuntoTitoli("Turn left at the St. here"); got != "Turn left at the St. here" {
		t.Errorf("punto non davanti a un nome toccato: %q", got)
	}
	moz := &motoreFinto{nome: "Mozilla"}
	tr := NuovoTraduttore(filepath.Join(t.TempDir(), "c.json"), glossarioBase(), moz)
	tr.TraduciScritta(context.Background(), "I'm Ms. Toriumi. I teach Composition.")
	if strings.Contains(moz.ultim.Testo, "Ms.") {
		t.Errorf("al traduttore è arrivato %q", moz.ultim.Testo)
	}
}

// Il report attribuisce la traduzione tenuta solo alla riga con lo stesso riquadro, non ai pezzi
// letti lì dentro.
func TestCollaudoReportStessoRiquadro(t *testing.T) {
	intera := image.Rect(100, 100, 400, 120)
	if !stessoRiquadro(intera, image.Rect(102, 101, 398, 121)) {
		t.Error("la stessa riga letta con 2 pixel di differenza")
	}
	if stessoRiquadro(intera, image.Rect(380, 100, 395, 120)) {
		t.Error("un pezzo (\"X\") dentro la riga preso per la riga")
	}
}

// La barra delle schede scorre: sulla scheda nuova ("PROSTHETICS", letta ma "non è testo da
// tradurre") non resta la traduzione della scheda di prima. La stessa scritta letta a pezzi o
// con lettere sbagliate tiene la traduzione.
func TestCollaudoTraduzioneVecchiaSuUnAltraScritta(t *testing.T) {
	vecchia := Blocco{Testo: "HAIR COLOR", Tradotto: "COLORE DEI CAPELLI", X: 667, Y: 166, W: 120, H: 20}
	fresca := Blocco{Testo: "HAIR COLOR", Tradotto: "COLORE DEI CAPELLI", X: 483, Y: 167, W: 120, H: 20}
	letti := []Blocco{fresca, {Testo: "PROSTHETICS", X: 661, Y: 166, W: 130, H: 20}}
	out := senzaScritteCambiate([]Blocco{fresca, vecchia}, []Blocco{fresca}, letti)
	if len(out) != 1 || out[0].X != 483 {
		t.Errorf("la traduzione di HAIR COLOR resta su PROSTHETICS: %+v", out)
	}
	for _, l := range []string{"HAIR COLOR", "HAlR C0LOR", "HAIR", "|"} {
		out := senzaScritteCambiate([]Blocco{vecchia}, nil, []Blocco{{Testo: l, X: 667, Y: 166, W: 120, H: 20}})
		if len(out) != 1 {
			t.Errorf("lettura %q della stessa scritta: traduzione tolta", l)
		}
	}
	// una scritta accanto che tocca appena il riquadro non conta
	out = senzaScritteCambiate([]Blocco{vecchia}, nil, []Blocco{{Testo: "PROSTHETICS", X: 780, Y: 166, W: 130, H: 20}})
	if len(out) != 1 {
		t.Error("la scritta accanto ha tolto la traduzione")
	}
}

// La traduzione di una lettura sbagliata non diventa la traduzione fissa della sessione.
func TestCollaudoLetturaSbagliataNonSiFissa(t *testing.T) {
	for _, s := range []string{"Early Meroing", "Locate the cahp", "They are all agned."} {
		if daFissare(Blocco{Testo: s}, "x") != "" {
			t.Errorf("%q: lettura sbagliata fissata", s)
		}
	}
	for _, s := range []string{"Early Morning", "Follow Lunka", "Lunka: Come on! We have places to be!", "REACH THE CAMP", "Bond: You're dodging the question."} {
		if daFissare(Blocco{Testo: s}, "x") == "" {
			t.Errorf("%q: lettura buona non fissata", s)
		}
	}
}

// La decisione tenuta scade: vale solo dopo almeno 2 fotogrammi decisi da soli, e non oltre 20
// secondi dall'ultima conferma; i fotogrammi tenuti solo grazie a lei non la rinnovano.
// Una scritta del mondo tradotta per sbaglio una volta non resta tradotta per la sessione.
func TestCollaudoDecisioneScade(t *testing.T) {
	f := &traduzioniFisse{}
	c := &confermeDecisione{}
	t0 := time.Unix(5000, 0)
	cartello := Blocco{Testo: "KEEP OUT", Tradotto: "VIETATO L'INGRESSO"}
	f.fissa("en>it", cartello.Testo, cartello.Tradotto)
	c.segna("en>it", []Blocco{cartello}, []Blocco{cartello}, t0) // tradotta per sbaglio una volta
	bl := []Blocco{{Testo: "KEEP OUT"}}
	segnaGiaMostrate(f, c, "en>it", bl, t0.Add(time.Second))
	if bl[0].GiaMostrata {
		t.Fatal("una sola volta tradotta: la decisione non si deve tenere")
	}
	// due fotogrammi decisi da soli: si tiene
	c.segna("en>it", []Blocco{cartello}, []Blocco{cartello}, t0.Add(250*time.Millisecond))
	segnaGiaMostrate(f, c, "en>it", bl, t0.Add(time.Second))
	if !bl[0].GiaMostrata {
		t.Fatal("confermata due volte: la decisione si tiene")
	}
	// tenuta solo grazie alla decisione (Tenuta) o rimasta dal fotogramma prima: non rinnova
	tenuta := cartello
	tenuta.Tenuta = true
	for k := 1; k <= 100; k++ {
		ora := t0.Add(time.Duration(k) * 250 * time.Millisecond)
		c.segna("en>it", []Blocco{tenuta}, []Blocco{tenuta}, ora)
		c.segna("en>it", []Blocco{cartello}, nil, ora) // dal fotogramma prima: non fresca
	}
	segnaGiaMostrate(f, c, "en>it", bl, t0.Add(10*time.Minute))
	if !bl[0].GiaMostrata {
		t.Error("confermata due volte: resta tradotta per tutta la partita (regola di Sergio, niente salti)")
	}
	// controllaCompleta segna Tenuta quando passa solo per la decisione (lettura incerta)
	b := Blocco{Testo: "Early Morning", Affid: letturaDubbia, GiaMostrata: true}
	controllaCompleta(nil, &b, nil)
	if !b.Tenuta {
		t.Error("lettura incerta passata per la decisione tenuta: Tenuta non segnata")
	}
	b = Blocco{Testo: "Early Morning", Affid: 1, GiaMostrata: true}
	controllaCompleta(nil, &b, nil)
	if b.Tenuta {
		t.Error("lettura buona: è una conferma vera, non Tenuta")
	}
}

// La lettura giusta sostituisce subito la traduzione di quella sbagliata; letture alternate
// giusta/sbagliata non fanno sfarfallare.
func TestCollaudoIsteresiLetturaGiusta(t *testing.T) {
	img := immagineMondo(600, 300, 0, 0, 1)
	f := lettereIrregolari(img, 100, 100, 24)
	img2 := immagineMondo(600, 300, 0, 0, 1)
	lettereIrregolari(img2, 100, 100, 24)
	ist := &isteresi{}
	t0 := time.Unix(1000, 0)
	sbagliata := bloccoMostrato(img, "Reach the comp", "Raggiungere il Comp", 100, 100, f-100, 24)
	giusta := bloccoMostrato(img2, "Reach the camp", "Raggiungi il campo", 100, 100, f-100, 24)
	giusta.GiaMostrata = true // già mostrata e confermata prima (fisse + conferme)
	vis, _ := ist.applica([]Blocco{sbagliata}, nil, nil, img, t0)
	vis, _ = ist.applica([]Blocco{giusta}, vis, nil, img2, t0.Add(250*time.Millisecond))
	if len(vis) != 1 || vis[0].Tradotto != "Raggiungi il campo" {
		t.Fatalf("lettura giusta dopo quella sbagliata: %+v", vis)
	}
	// alternate: sbagliata, giusta, sbagliata, sbagliata... resta sempre la giusta
	ora := t0.Add(500 * time.Millisecond)
	for k := 0; k < 8; k++ {
		n := sbagliata
		if k%3 == 1 {
			n = giusta
		}
		n.Img = img2
		vis, _ = ist.applica([]Blocco{n}, vis, nil, img2, ora)
		if len(vis) != 1 || vis[0].Tradotto != "Raggiungi il campo" {
			t.Fatalf("giro %d (%q): mostrata %+v", k, n.Testo, vis)
		}
		ora = ora.Add(250 * time.Millisecond)
	}
	// parola inglese che non esiste: "Early Meroing" → "Early Morning" subito
	ist2 := &isteresi{}
	a := bloccoMostrato(img, "Early Meroing", "La prima meroing", 100, 100, f-100, 24)
	b := bloccoMostrato(img2, "Early Morning", "Mattina presto", 100, 100, f-100, 24)
	vis, _ = ist2.applica([]Blocco{a}, nil, nil, img, t0)
	vis, _ = ist2.applica([]Blocco{b}, vis, nil, img2, t0.Add(250*time.Millisecond))
	if len(vis) != 1 || vis[0].Tradotto != "Mattina presto" {
		t.Errorf("Early Morning dopo Early Meroing: %+v", vis)
	}
	vis, _ = ist2.applica([]Blocco{a}, vis, nil, img2, t0.Add(500*time.Millisecond))
	if len(vis) != 1 || vis[0].Tradotto != "Mattina presto" {
		t.Errorf("una lettura sbagliata sola ha rimesso la traduzione sbagliata: %+v", vis)
	}
}

// Schermata ferma (nessuna rilettura, nessun giro mostrato): la decisione non scade col solo
// tempo. Quando poi qualcos'altro cambia e si rilegge tutto, la scritta resta tradotta.
func TestCollaudoDecisioneSchermataFerma(t *testing.T) {
	f := &traduzioniFisse{}
	c := &confermeDecisione{}
	t0 := time.Unix(9000, 0)
	menu := Blocco{Testo: "Early Morning", Tradotto: "Mattina presto"}
	f.fissa("en>it", menu.Testo, menu.Tradotto)
	c.segna("en>it", []Blocco{menu}, []Blocco{menu}, t0)
	c.segna("en>it", []Blocco{menu}, []Blocco{menu}, t0.Add(250*time.Millisecond))
	bl := []Blocco{{Testo: "Early Morning", Affid: letturaDubbia}}
	segnaGiaMostrate(f, c, "en>it", bl, t0.Add(90*time.Second)) // un minuto e mezzo fermo
	if !bl[0].GiaMostrata {
		t.Fatal("schermata ferma: la decisione non deve scadere solo per il tempo")
	}
	// e neanche con lo schermo che si muove: resta tradotta per tutta la partita (regola di Sergio)
	for k := 0; k < scadenzaGiri+1; k++ {
		c.segna("en>it", nil, nil, t0.Add(time.Duration(91+k)*time.Second))
	}
	segnaGiaMostrate(f, c, "en>it", bl, t0.Add(150*time.Second))
	if !bl[0].GiaMostrata {
		t.Fatal("confermata: resta tradotta anche dopo tanti giri (niente salti italiano/inglese)")
	}
}

// Regola di Sergio 26/11: due letture della stessa scritta una sopra l'altra non fanno tornare in
// inglese quella già tradotta e confermata ("Early Morning" che saltava a ogni fotogramma).
func TestSovrappostaTieneLaConfermata(t *testing.T) {
	conf := Blocco{Testo: "Early Morning", Tradotto: "Mattina presto", X: 100, Y: 100, W: 200, H: 30, GiaMostrata: true}
	altra := Blocco{Testo: "Early Mornin", Tradotto: "Mattina presto", X: 105, Y: 102, W: 190, H: 28}
	got := senzaSovrapposte([]Blocco{conf, altra})
	if len(got) != 1 || got[0].Testo != conf.Testo {
		t.Fatalf("deve restare solo quella confermata: %+v", got)
	}
	if !copertaDaTenuta(altra, got) {
		t.Fatal("la lettura tolta sta sotto la confermata: non è un pezzo in inglese")
	}
	if tenute, _ := senzaMezzeRighe(got, nil); len(tenute) != 1 {
		t.Fatal("la confermata deve restare disegnata")
	}
	// nessuna delle due confermata: via tutte e due come prima
	a, b := conf, altra
	a.GiaMostrata = false
	if got := senzaSovrapposte([]Blocco{a, b}); len(got) != 0 {
		t.Fatalf("senza conferma restano in inglese tutte e due: %+v", got)
	}
}
