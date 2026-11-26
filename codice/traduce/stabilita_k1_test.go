package traduce

// Prove dei salti italiano/inglese rimasti nella prova sui video delle 15:56 (versione 10b418d).
// Blocchi e testi presi dal report (prova-1556-report.txt); le altezze sono quelle tipiche di
// quelle scritte a 1920x1080.

import (
	"image/color"
	"testing"
	"time"
)

// f.70 (Persona 3): le scelte del dialogo "...Thanks." / "Nice to meet you!" / "What's your
// first name?" erano unite in un paragrafo: la scelta "Nice to meet you!" mostrava "Piacere di
// conoscerti! Come ti chiami?" e al fotogramma dopo "Piacere di conoscerti!". f.400 (Dawnwalker):
// "What's going on?" diventava "Che succede? Cosa mi hai fatto?".
func TestK1ScelteNonUnite(t *testing.T) {
	casi := [][]RigaOCR{
		{
			rigaDa("...Thanks.", 1189, 560, 120, 34),
			rigaDa("Nice to meet you!", 1125, 688, 230, 34),
			rigaDa("What's your first name?", 1082, 748, 290, 34),
		},
		{
			rigaDa("What's going on?", 200, 511, 200, 34),
			rigaDa("What did you do to me?", 155, 563, 290, 34),
		},
	}
	for _, righe := range casi {
		// la causa: unisciParagrafi le unisce (righe centrate dello "stesso sottotitolo")
		bl := segmenti(righe, 1)
		for i := range bl {
			bl[i].Grezzo = bl[i].Testo
			bl[i].Testo, bl[i].Affid = correggiRigaOCR(bl[i].Grezzo)
		}
		if u := unisciParagrafi(append([]Blocco(nil), bl...)); len(u) == len(righe) {
			t.Logf("unisciParagrafi non le unisce più: la prova resta valida per blocchiLetti")
		} else {
			t.Logf("unite da unisciParagrafi: %q", chiaviBlocchi(u))
		}
		got := blocchiLetti(nil, righe, 1, nil, time.Now())
		if len(got) != len(righe) {
			t.Fatalf("le scelte devono restare separate: %q", chiaviBlocchi(got))
		}
		for _, b := range got {
			if b.Righe > 1 {
				t.Errorf("scelta unita a un'altra: %q", b.Testo)
			}
		}
	}
	// un dialogo vero su due righe resta un paragrafo
	righe := []RigaOCR{
		rigaDa("Yeah, I know. It's just a", 1104, 627, 330, 30),
		rigaDa("coincidence. It must be fate.", 1104, 668, 360, 30),
	}
	if got := blocchiLetti(nil, righe, 1, nil, time.Now()); len(got) != 1 || got[0].Righe != 2 {
		t.Errorf("il dialogo su due righe deve restare unito: %q", chiaviBlocchi(got))
	}
}

// f.27 e f.59: "Early Morning" letto due volte una sopra l'altra ("Early Mornin" e
// "iarIyMorning", tutte e due corrette in "Early Morning"), f.175: "IMPACT IN 86" due volte
// nello stesso posto. Prima restavano in inglese tutte e due (traduzioni sovrapposte).
func TestK1StessaScrittaLettaDueVolte(t *testing.T) {
	a := Blocco{Testo: "Early Morning", Tradotto: "Mattina presto", X: 1510, Y: 101, W: 250, H: 40, Righe: 1, GiaMostrata: true}
	b := Blocco{Testo: "Early Morning", Tradotto: "Mattina presto", X: 1525, Y: 100, W: 235, H: 40, Righe: 1, GiaMostrata: true}
	got := senzaSovrapposte([]Blocco{a, b})
	if len(got) != 1 || got[0].X != a.X {
		t.Fatalf("deve restare una sola lettura (la più grande): %+v", got)
	}
	if !copertaDaTenuta(b, got) {
		t.Error("la lettura tolta è la stessa scritta: non è un pezzo in inglese")
	}
	if tenute, _ := senzaMezzeRighe(got, nil); len(tenute) != 1 {
		t.Error("la scritta deve restare disegnata")
	}
	// anche non ancora confermate, con la stessa traduzione
	i1 := Blocco{Testo: "IMPACT IN 86", Tradotto: "IMPATTO IN 86", X: 932, Y: 84, W: 74, H: 17, Righe: 1}
	i2 := i1
	if got := senzaSovrapposte([]Blocco{i1, i2}); len(got) != 1 {
		t.Errorf("IMPACT IN 86 letto due volte: %+v", got)
	}
	// due scritte diverse una sopra l'altra: via tutte e due, come prima
	c := Blocco{Testo: "Early Mourning", Tradotto: "Lutto mattutino", X: 1510, Y: 101, W: 250, H: 40, Righe: 1}
	if got := senzaSovrapposte([]Blocco{a, c}); len(got) != 1 || got[0].Testo != a.Testo {
		t.Errorf("la confermata resta, l'altra va via: %+v", got)
	}
	a.GiaMostrata = false
	if got := senzaSovrapposte([]Blocco{a, c}); len(got) != 0 {
		t.Errorf("due letture diverse non confermate: restano in inglese tutte e due (%+v)", got)
	}
}

// f.130-134 (The Last Flame? HUD della nave): "Meteoric iron: 30 / 30 units" sotto "ORDER OF THE
// DAV" (in inglese, mezza) saltava tra italiano e inglese; f.198 e f.241: "IMPACT IN 86/85"
// sopra "m' this shiFt" (la riga sotto letta senza il numero davanti) tornava in inglese.
func TestK1TitoloMaiuscoloSopraRigaNormale(t *testing.T) {
	metal := Blocco{Testo: "Meteoric iron: 30 / 30 units", Tradotto: "Ferro meteorico: 30 / 30 unità", X: 41, Y: 60, W: 228, H: 18, Righe: 1, Em: 18, GiaMostrata: true}
	ordine := Blocco{Testo: "ORDER OF THE DAV", X: 41, Y: 37, W: 150, H: 18, Righe: 1, Em: 18}
	if tenute, _ := senzaMezzeRighe([]Blocco{metal}, []Blocco{ordine}); len(tenute) != 1 {
		t.Error("Meteoric iron: il titolo maiuscolo sopra non è un'altra voce dello stesso elenco")
	}
	impatto := Blocco{Testo: "IMPACT IN 86", Tradotto: "IMPATTO IN 86", X: 932, Y: 84, W: 74, H: 17, Righe: 1, Em: 17, GiaMostrata: true}
	sotto := Blocco{Testo: "m' this shiFt", X: 930, Y: 105, W: 80, H: 17, Righe: 1, Em: 17}
	if tenute, _ := senzaMezzeRighe([]Blocco{impatto}, []Blocco{sotto}); len(tenute) != 1 {
		t.Error("IMPACT IN 86: la riga normale sotto non continua l'avviso maiuscolo")
	}
	if seguitoSotto(impatto, sotto) {
		t.Error("seguitoSotto: avviso maiuscolo e riga normale sono due scritte")
	}
	// restano le regole di prima: le scelte scritte allo stesso modo sono un gruppo
	compra := Blocco{Testo: "I'll buy!", Tradotto: "Compro!", X: 600, Y: 500, W: 90, H: 24, Righe: 1, Em: 24}
	lascia := Blocco{Testo: "Never mind.", X: 600, Y: 530, W: 110, H: 24, Righe: 1, Em: 24}
	if tenute, _ := senzaMezzeRighe([]Blocco{compra}, []Blocco{lascia}); len(tenute) != 0 {
		t.Error("scelte dello stesso elenco: tutte tradotte o tutte in inglese")
	}
	// e una riga maiuscola che chiede il seguito continua sotto
	su := Blocco{Testo: "PRESS THE BUTTON TO", X: 100, Y: 100, W: 200, H: 20, Righe: 1, Em: 20}
	giu := Blocco{Testo: "open the door", X: 100, Y: 124, W: 140, H: 20, Righe: 1, Em: 20}
	if !seguitoSotto(su, giu) {
		t.Error("riga maiuscola che finisce sospesa: la frase continua sotto")
	}
}

// f.195: "IMPACT IN 86" (già tradotto e confermato) tornava in inglese come "pezzo di una frase
// rimasta in inglese" per un pezzetto letto male accanto ("nay g." al posto di "DAV 5").
func TestK1FratelloCortoNonTogliaLaConfermata(t *testing.T) {
	impatto := Blocco{Testo: "IMPACT IN 86", X: 932, Y: 84, W: 74, H: 17, Righe: 1, Em: 17, Affid: letturaBuona}
	pezzo := Blocco{Testo: "nay g.", X: 880, Y: 84, W: 46, H: 17, Righe: 1, Em: 17, Affid: letturaDubbia}
	tutte := []Blocco{pezzo, impatto}
	b := impatto
	if m := controllaCompleta(nil, &b, tutte); m == "" {
		t.Skip("il pezzetto accanto non fa più restare in inglese nemmeno una scritta nuova")
	}
	b = impatto
	b.GiaMostrata = true
	if m := controllaCompleta(nil, &b, tutte); m != "" {
		t.Errorf("già confermata: un pezzetto di due parole accanto non la rimette in inglese (%q)", m)
	}
	// un pezzo lungo della stessa frase letto male conta sempre (mai mezza riga)
	lungo := Blocco{Testo: "nay g. the core is", X: 800, Y: 84, W: 126, H: 17, Righe: 1, Em: 17, Affid: letturaDubbia}
	b = impatto
	b.GiaMostrata = true
	b2 := impatto
	m1 := controllaCompleta(nil, &b2, []Blocco{lungo, impatto})
	if m := controllaCompleta(nil, &b, []Blocco{lungo, impatto}); m != m1 {
		t.Errorf("pezzo lungo accanto: confermata %q, nuova %q (deve essere uguale)", m, m1)
	}
}

// f.267-269: "Defeat the enemies." (già tradotto e confermato) letto per un fotogramma come
// "eat the enemies." (lettura incerta, le lettere sfumano): tornava in inglese per un giro.
func TestK1LettaAMetaResta(t *testing.T) {
	img1 := immagineMondo(700, 300, 0, 0, 1)
	f := lettereIrregolari(img1, 100, 100, 24)
	img2 := immagineMondo(700, 300, 0, 0, 1)
	// le lettere sfumano: più scure e senza le prime (la forma sotto è cambiata)
	grigio := color.RGBA{150, 150, 160, 255}
	for y := 100; y < 124; y++ {
		for x := 140; x < f; x += 3 {
			img2.SetRGBA(x, y, grigio)
		}
	}
	vecchia := bloccoMostrato(img1, "Defeat the enemies.", "Sconfiggi i nemici.", 100, 100, f-100, 24)
	vecchia.GiaMostrata = true
	if !formaCambiata(img1, img2, vecchia) {
		t.Skip("l'immagine di prova non cambia la forma: la prova non direbbe niente")
	}
	t0 := time.Unix(2000, 0)
	pezzo := Blocco{Testo: "eat the enemies.", X: 133, Y: 102, W: f - 140, H: 22, Righe: 1, Affid: letturaDubbia}
	for _, conf := range []bool{true, false} {
		ist := &isteresi{}
		v := vecchia
		v.GiaMostrata = conf
		vis, _ := ist.applica([]Blocco{v}, nil, nil, img1, t0)
		vis, ingl := ist.applica(nil, vis, []Blocco{pezzo}, img2, t0.Add(250*time.Millisecond))
		if conf && (len(vis) != 1 || vis[0].Tradotto != "Sconfiggi i nemici." || len(ingl) != 0) {
			t.Errorf("confermata e letta a metà: deve restare tradotta (%+v, inglesi %d)", vis, len(ingl))
		}
		if !conf && len(vis) != 0 {
			t.Errorf("non confermata, forma cambiata: regola di prima (%+v)", vis)
		}
	}
	// lì dentro adesso c'è un'altra scritta: la traduzione vecchia non resta
	ist := &isteresi{}
	vis, _ := ist.applica([]Blocco{vecchia}, nil, nil, img1, t0)
	altra := Blocco{Testo: "Find the exit.", X: 133, Y: 102, W: f - 140, H: 22, Righe: 1, Affid: letturaDubbia}
	if vis, _ = ist.applica(nil, vis, []Blocco{altra}, img2, t0.Add(250*time.Millisecond)); len(vis) != 0 {
		t.Errorf("un'altra scritta: la traduzione vecchia non deve restare (%+v)", vis)
	}
}
