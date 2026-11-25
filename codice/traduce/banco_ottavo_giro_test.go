package traduce

// Ottavo giro: gli stessi 9 video (r5.txt, build e91b296, 1328 fotogrammi). Il banco diceva
// "mezze disegnate 0" ma sullo schermo c'erano ancora righe metà inglesi e metà italiane.
// Casi veri (testo letto dall'OCR e posizione nel fotogramma a 1920x1080):
//   - 0655 "Tap [Q] to quickly throw the item. Hold [Q] to aim." → "…Hold Q per mirare.";
//   - 1037 paragrafo "Press [E] to shoot with Koby, or hold [mouse] to aim. Hold [E] while aiming /
//     for a powerful shot." → "E mentre mira" accanto all'inglese;
//   - 0428 "Brodie: rm son', Caz, he's not the only one we" tradotta accanto a "lost..." inglese;
//     "WEIGHT" → "PESO" sulla targa gialla di una macchina;
//   - 0657 "Press [↑] to sprint." letto "Press t to sprint." → "Premi t per sprint.";
//   - 1137 "/ Back" → "/ Ritorno", "1S4V1 POINT" → "1S4V1 PUNTO";
//   - righe pulite lasciate in inglese: "Caz, don't let me die…", "Koby's Capabilities", "Mr
//     McCleary", "wh-what", "Use" da solo, traduzioni giuste scartate come "inventate".

import (
	"strings"
	"testing"
)

// 0655: la riga divisa da Windows attorno alla seconda icona: il primo pezzo finisce a x≈1105,
// l'icona [Q] è nel buco, "Q to aim." comincia a 1131. Con il buco dell'icona più largo (fino a
// quasi 4 em) i due pezzi diventano lo stesso una frase sola.
func TestComandoConIconaUnaFrase(t *testing.T) {
	for _, c := range []struct{ fine, h float64 }{{1105, 20}, {1105, 16}, {1070, 18}} {
		a := rigaDa("Tap Q toquickly throw the item. Hold", 670, 822, c.fine-670, c.h)
		q := rigaDa("Q to aim.", 1131, 822, 92, c.h)
		bl, pronti := disegnate(a, q)
		if len(bl) != 1 || !strings.HasSuffix(bl[0].Testo, "Hold Q to aim.") {
			t.Errorf("%v: riga non unita: %q", c, chiaviBlocchi(bl))
		}
		for _, p := range pronti {
			if strings.TrimSpace(p.Testo) == "Q to aim." {
				t.Errorf("%v: \"Q to aim.\" disegnata da sola", c)
			}
		}
	}
	// se non si uniscono (la prima parte resta in inglese, altezze diverse, grandezza del
	// carattere non stimata): il pezzo tradotto non si disegna e il banco lo conta
	a := Blocco{Testo: "Tap Q to quickly throw the item. Hold", X: 670, Y: 822, W: 435, H: 25, Righe: 1, Affid: letturaBuona}
	q := Blocco{Testo: "Q to aim.", X: 1131, Y: 826, W: 92, H: 18, Righe: 1, Affid: letturaBuona, Tradotto: "Q per mirare."}
	if n := mezzeDisegnate([]Blocco{q}, []Blocco{a}); n != 1 {
		t.Errorf("mezze disegnate %d, doveva contare 1", n)
	}
	if tenute, _ := senzaMezzeRighe([]Blocco{q}, []Blocco{a}); len(tenute) != 0 {
		t.Errorf("\"Q per mirare.\" disegnata accanto alla prima parte in inglese")
	}
	// due voci di una fila di pulsanti restano separate
	s := Blocco{Testo: "Options", X: 600, Y: 900, W: 90, H: 22, Righe: 1, Affid: letturaCatalog}
	d := Blocco{Testo: "Quit Game", X: 760, Y: 900, W: 120, H: 22, Righe: 1, Affid: letturaCatalog}
	if stessaFraseSchermo(d, s) {
		t.Error("due voci di una fila prese per una frase")
	}
}

// comandoAperto e inizioTasto sui pezzi veri e su quelli che non sono comandi.
func TestComandoAperto(t *testing.T) {
	si := []string{"Press", "Tap Q to quickly throw the item. Hold", "Press E to shoot with Koby. or hold to aim Hold", "…or hold"}
	no := []string{"Long Press", "Ready to use", "Put the call on hold", "Keep on hold"}
	for _, s := range si {
		if !comandoAperto(s) {
			t.Errorf("%q: doveva volere il seguito", s)
		}
	}
	for _, s := range no {
		if comandoAperto(s) {
			t.Errorf("%q: non è un comando aperto", s)
		}
	}
	for _, s := range []string{"Q to aim.", "E while aiming", "Shift to sprint", "F5 to save"} {
		if !inizioTasto(s) {
			t.Errorf("%q: comincia con il tasto", s)
		}
	}
	for _, s := range []string{"Quit", "Options menu", "Ok then"} {
		if inizioTasto(s) {
			t.Errorf("%q: non comincia con un tasto", s)
		}
	}
}

// 1037: paragrafo di due righe; Windows divide la prima riga all'ultima icona. Il pezzo a destra
// fa parte della prima riga: si unisce e il paragrafo si traduce intero.
func TestParagrafoConIconaIntero(t *testing.T) {
	r1 := rigaLettere("Press E to shoot with Koby. or hold to aim Hold", 622, 541, 24)
	dx := rigaLettere("E while aiming", 1151, 541, 24)
	r2 := rigaLettere("for a powerful shot.", 622, 570, 24)
	bl, pronti := disegnate(r1, dx, r2)
	if len(bl) != 1 || !strings.Contains(bl[0].Testo, "Hold E while aiming for a powerful shot.") {
		t.Errorf("paragrafo non unito: %q", chiaviBlocchi(bl))
	}
	for _, p := range pronti {
		if strings.HasPrefix(p.Testo, "E while") {
			t.Errorf("\"E while aiming\" disegnata da sola")
		}
	}
	// il paragrafo (due righe) resta in inglese e il pezzo a destra della prima riga è pronto:
	// il controllo finale guarda le righe del paragrafo una per una
	par := unisciParagrafi([]Blocco{bloccoDaParole(r1.Parole, 1), bloccoDaParole(r2.Parole, 1)})
	if len(par) != 1 || par[0].Righe != 2 {
		t.Fatalf("paragrafo di prova: %q", chiaviBlocchi(par))
	}
	pezzo := bloccoDaParole(dx.Parole, 1)
	pezzo.Tradotto = "E mentre mira"
	if tenute, _ := senzaMezzeRighe([]Blocco{pezzo}, par); len(tenute) != 0 {
		t.Errorf("\"E mentre mira\" disegnata accanto al paragrafo inglese")
	}
	if n := mezzeDisegnate([]Blocco{pezzo}, par); n != 1 {
		t.Errorf("mezze disegnate %d, doveva contare 1", n)
	}
	// un'altra scritta sotto il paragrafo, staccata, resta libera
	altro := Blocco{Testo: "Close", X: 1228, Y: 964, W: 70, H: 22, Righe: 1, Affid: letturaCatalog}
	if tenute, _ := senzaMezzeRighe([]Blocco{altro}, par); len(tenute) != 1 {
		t.Errorf("\"Close\" tolta per colpa del paragrafo")
	}
}

// 0428: "…he's not the only one we" e "lost..." sulla stessa linea con riquadri di altezze
// molto diverse (puntini, niente gambe): pezzi della stessa frase. E "rm" è una lettura sbagliata.
func TestSottotitoloPezzoAltezzaDiversa(t *testing.T) {
	s := Blocco{Testo: "Brodie: I'm sorry, Caz, he's not the only one we", X: 632, Y: 822, W: 560, H: 26, Righe: 1, Affid: letturaBuona}
	l := Blocco{Testo: "lost...", X: 1200, Y: 829, W: 62, H: 15, Righe: 1, Affid: letturaCatalog}
	if !stessaFraseSchermo(s, l) {
		t.Error("\"lost...\" non riconosciuta come seguito della riga")
	}
	if tenute, _ := senzaMezzeRighe([]Blocco{s}, []Blocco{l}); len(tenute) != 0 {
		t.Error("sottotitolo tradotto accanto a \"lost...\" in inglese")
	}
	b := Blocco{Testo: "Brodie: rm son', Caz, he's not the only one we", Affid: letturaBuona, X: 632, Y: 822, W: 560, H: 26, Righe: 1}
	if m := motivoNonTradurre(b); m != "lettura incerta" {
		t.Errorf("\"rm son'\": motivo %q, doveva essere lettura incerta", m)
	}
	for _, ok := range []string{"Caz: Trouble with the mrs?", "Hmm, I see.", "Shh! Be quiet.", "Lvl 5 required: 120 xp", "Get to the 2nd floor."} {
		if parolaSenzaVocali(ok) {
			t.Errorf("%q: parola vera presa per sbagliata", ok)
		}
	}
	// una battuta nuova (con il nome di chi parla) non è il seguito della riga sopra
	sopra := Blocco{Testo: "Caz: Right... Yeah, wh-what do you need", X: 712, Y: 886, W: 500, H: 26, Righe: 1}
	sotto := Blocco{Testo: "Brodie: The cable's stuck.", X: 796, Y: 915, W: 330, H: 26, Righe: 1}
	if seguitoSotto(sopra, sotto) {
		t.Error("battuta di un altro personaggio presa per il seguito")
	}
}

// 0428: "WEIGHT" (catalogo) sulla targa gialla "CLUMP WEIGHT / WINCH CONTROL" della macchina:
// lettere scure su fondo più chiaro, "CLUMP" accanto non letto → resta del gioco.
// Ritaglio testdata/video5/0428-targa.png: fotogramma portato a 1920x1080, da (240,440).
func TestParolaDiCartelloLettaAMeta(t *testing.T) {
	img, err := apriImmagineRGBA("testdata/video5/0428-targa.png")
	if err != nil {
		t.Fatal(err)
	}
	w := Blocco{Testo: "WEIGHT", X: 87, Y: 28, W: 64, H: 16, Righe: 1, Affid: letturaCatalog, Em: 20}
	c := Blocco{Testo: "cog", X: 85, Y: 52, W: 40, H: 14, Righe: 1, Affid: letturaBuona, Em: 20}
	x := w
	if m := controllaCompleta(img, &x, []Blocco{w, c}); m == "" {
		t.Error("\"WEIGHT\" della targa tradotta: doveva restare in inglese")
	}
}

// 0657: la freccia dell'icona letta "t": tolta, la scritta si divide attorno all'icona.
func TestFrecciaDelTastoLettaLettera(t *testing.T) {
	bl, pronti := disegnate(rigaLettere("Press t to sprint.", 856, 794, 26))
	for _, b := range bl {
		if strings.Contains(" "+b.Testo+" ", " t ") {
			t.Errorf("la lettera dell'icona è rimasta nel testo: %q", b.Testo)
		}
	}
	if len(bl) != 2 || bl[0].Testo != "Press" || bl[1].Testo != "to sprint." {
		t.Fatalf("scritta non divisa attorno all'icona: %q", chiaviBlocchi(bl))
	}
	if bl[0].IconaDx == 0 || bl[1].IconaSx == 0 {
		t.Errorf("l'icona non è segnata: IconaDx %d IconaSx %d", bl[0].IconaDx, bl[1].IconaSx)
	}
	if len(pronti) != 2 {
		t.Errorf("\"Press [↑] to sprint.\": i due pezzi vanno tradotti tutti e due, pronti %q", testiBlocchi(pronti))
	}
	// "Press A to jump" (la lettera maiuscola è il tasto nominato) e "Press a button" restano
	for _, s := range []string{"Press A to jump.", "Press a button to continue."} {
		if i := iconaInMezzo(strings.Fields(s)); i >= 0 {
			t.Errorf("%q: %q presa per un'icona", s, strings.Fields(s)[i])
		}
	}
}

// 1137: "/ Back" (il segno staccato), "1S4V1 POINT" (nome con un codice).
func TestSegnoStaccatoENomeConCodice(t *testing.T) {
	bl := togliSegniIcona([]Blocco{bloccoDaParole(rigaLettere("/ Back", 1784, 1016, 22).Parole, 1)})
	if len(bl) != 1 || bl[0].Testo != "Back" || bl[0].IconaSx == 0 {
		t.Errorf("\"/ Back\": %q (IconaSx %d)", chiaviBlocchi(bl), bl[0].IconaSx)
	}
	for _, s := range []string{"1S4V1 POINT", "SECTOR 7G", "B2 HANGAR"} {
		if m := motivoNonTradurre(Blocco{Testo: s, Affid: letturaBuona, W: 200, H: 30, Righe: 1}); m != motivoNomeCodice {
			t.Errorf("%q: motivo %q, doveva restare (nome con un codice)", s, m)
		}
	}
	for _, s := range []string{"FAST TRAVEL", "CHAPTER 2", "SAVE"} {
		if nomeConCodice(s) {
			t.Errorf("%q preso per un nome con un codice", s)
		}
	}
}

// righe pulite che restavano in inglese per niente.
func TestRighePuliteRecuperate(t *testing.T) {
	for _, s := range []string{"Gregor: Caz, don't let me die man, please...", "Koby's Capabilities",
		"Roy: And you, Mr McCleary, you can fuss and cuss all you like.", "Caz: Right... Yeah, wh-what do you need?"} {
		c, a := correggiRigaOCR(s)
		if m := motivoNonTradurre(Blocco{Testo: c, Affid: a, W: 600, H: 30, Righe: 1}); m != "" {
			t.Errorf("%q: resta in inglese (%s)", s, m)
		}
	}
	// "Use" da solo accanto all'icona è una voce intera
	u := Blocco{Testo: "Use", X: 1104, Y: 532, W: 50, H: 22, Righe: 1, Affid: letturaCatalog}
	if pezzoSenzaTesta(u, []Blocco{u}) {
		t.Error("\"Use\" da solo tolto")
	}
	// traduzioni giuste con parole che l'elenco non aveva
	for _, c := range [][2]string{
		{"Suze: You left it on a bench at Argyle Street!", "Suze: L'hai lasciato su una panchina in Argyle Street!"},
		{"Barkov: My reserves of manpower are inexhaustible.", "Barkov: Le mie riserve di manodopera sono inesauribili."},
		{"Caz: Ah Jesus.", "Caz: Ah Gesù."},
		{"Turning Points map out major story beats and branching across each episode.", "I punti di svolta delineano i principali momenti della storia e le ramificazioni in ogni episodio."},
	} {
		if inv, w := traduzioneInventata(c[0], c[1]); inv {
			t.Errorf("%q: parole prese per inventate %q", c[1], w)
		}
	}
}
