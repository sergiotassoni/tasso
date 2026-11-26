package traduce

// Banco del mattino 26/11/2026 (30 video di gameplay inglesi, build 3c64612): le cause vere
// dell'instabilità e delle traduzioni sbagliate trovate nel report, una prova per ciascuna.

import (
	"image"
	"image/color"
	"math/rand"
	"strings"
	"testing"
	"time"
)

// Una parola intera diversa nella stessa scritta è un'altra scritta: la traduzione vecchia
// ("TUTORIAL: ATTACCHI DI BLOCCHI") non deve restare su "TUTORIAL: HEAVY ATTACKS".
func TestMattinoParolaCambiataNonEStessaScritta(t *testing.T) {
	if stessaScritta("TUTORIAL: BLOCK ATTACKS", "TUTORIAL: HEAVY ATTACKS") {
		t.Error("BLOCK/HEAVY: scritte diverse prese per la stessa")
	}
	// gli errori tipici dell'OCR restano la stessa scritta
	for _, c := range [][2]string{
		{"Use a health station to heal up. Hold until you've fully restored yourself.", "Use a health station to heal up. Hold uniil you've fully restored yourself."},
		{"Back to Form (Tutorial)", "Back to Form (Tutorichl)"},
		{"Let's get our things together, as far as I'm concerned.", "Let's get our things together, as far as I'm Amcerned."},
	} {
		if !stessaScritta(c[0], c[1]) {
			t.Errorf("%q / %q: è la stessa scritta letta male", c[0], c[1])
		}
	}
	// stabilizza non rimette la traduzione vecchia sulla scritta nuova
	vecchi := []Blocco{{Testo: "TUTORIAL: BLOCK ATTACKS", Tradotto: "TUTORIAL: PARA GLI ATTACCHI", X: 1422, Y: 406, W: 180, H: 16, Righe: 1}}
	nuovi := []Blocco{{Testo: "TUTORIAL: HEAVY ATTACKS", Tradotto: "TUTORIAL: ATTACCHI PESANTI", X: 1422, Y: 406, W: 182, H: 16, Righe: 1}}
	stabilizza(nuovi, vecchi)
	if nuovi[0].Tradotto != "TUTORIAL: ATTACCHI PESANTI" {
		t.Errorf("traduzione vecchia rimasta sulla scritta nuova: %q", nuovi[0].Tradotto)
	}
}

// Una scritta ferma letta con un riquadro un po' diverso (OCR) non "si muove con la scena".
func TestMattinoScrittaFermaNonSiMuove(t *testing.T) {
	v := &votazione{}
	t0 := time.Now()
	a := Blocco{Testo: "Close Tutorial", X: 1609, Y: 506, W: 110, H: 16}
	v.mossa(a, t0)
	b := a
	b.W, b.H = 140, 20 // un segno in più letto a destra, riquadro più alto
	if v.mossa(b, t0.Add(300*time.Millisecond)) {
		t.Error("stessa scritta ferma, riquadro un po' diverso: presa per scritta del mondo")
	}
	c := Blocco{Testo: "Horror Games House", X: 1773, Y: 102, W: 120, H: 14}
	v.mossa(c, t0)
	c2 := Blocco{Testo: "Horror Games House", X: 1771, Y: 99, W: 124, H: 18}
	if v.mossa(c2, t0.Add(300*time.Millisecond)) {
		t.Error("firma ferma letta due pixel più in là presa per scritta che si muove")
	}
	// una scritta che si sposta davvero resta del mondo
	d := Blocco{Testo: "CARGO HOIST", X: 691, Y: 169, W: 82, H: 11}
	v.mossa(d, t0)
	d.X += 40
	if !v.mossa(d, t0.Add(2*time.Second)) {
		t.Error("spostata di 40 pixel di lato: si muove con la scena")
	}
	// e anche una che si avvicina (cresce da tutti i lati)
	e := Blocco{Testo: "GAS STATION", X: 600, Y: 300, W: 80, H: 12}
	v.mossa(e, t0)
	e = Blocco{Testo: "GAS STATION", X: 570, Y: 285, W: 140, H: 22}
	if !v.mossa(e, t0.Add(2*time.Second)) {
		t.Error("insegna che si avvicina (cresce da tutti i lati): si muove con la scena")
	}
}

// La firma del canale nell'angolo in alto a destra resta in inglese; le voci del gioco lì
// accanto ("Score", "TAC-COM: OFF") e le voci in colonna no.
func TestMattinoFirmaCanaleNellAngolo(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	rnd := rand.New(rand.NewSource(5))
	for i := 0; i < len(img.Pix); i += 4 {
		v := uint8(40 + rnd.Intn(100))
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = v, v, v, 255
	}
	firma := Blocco{Testo: "Horror Games House", X: 1771, Y: 99, W: 124, H: 16, Righe: 1, Affid: letturaBuona}
	if m := motivoMondo(img, firma, []Blocco{firma}); m != motivoFirmaAngolo {
		t.Errorf("firma del canale: %q", m)
	}
	pezzo := Blocco{Testo: "or Games House", X: 1791, Y: 101, W: 100, H: 16, Righe: 1, Affid: letturaBuona}
	if m := motivoMondo(img, pezzo, []Blocco{pezzo}); m != motivoFirmaAngolo {
		t.Errorf("firma letta a metà: %q", m)
	}
	for _, b := range []Blocco{
		{Testo: "Score", X: 1800, Y: 60, W: 60, H: 16},
		{Testo: "RI TAC-COM: OFF", X: 1700, Y: 40, W: 150, H: 16},
		{Testo: "Quests (F9)", X: 273, Y: 11, W: 90, H: 16},          // in alto a sinistra
		{Testo: "Close Tutorial", X: 1609, Y: 506, W: 110, H: 16},    // a metà altezza
		{Testo: "Back to Menu", X: 1760, Y: 40, W: 120, H: 16},       // con "to": un comando
		{Testo: "Horror Games House", X: 1771, Y: 99, W: 124, H: 60}, // grande: titolo, non firma
	} {
		b.Righe, b.Affid = 1, letturaBuona
		if firmaNellAngolo(img, b, []Blocco{b}) {
			t.Errorf("%q presa per firma del canale", b.Testo)
		}
	}
	// voci in colonna nell'angolo: un elenco del gioco
	col := []Blocco{
		{Testo: "Fast Travel", X: 1720, Y: 20, W: 110, H: 16, Righe: 1, Affid: letturaBuona},
		{Testo: "Side Quest", X: 1720, Y: 50, W: 110, H: 16, Righe: 1, Affid: letturaBuona},
		{Testo: "Main Quest", X: 1720, Y: 80, W: 110, H: 16, Righe: 1, Affid: letturaBuona},
	}
	if firmaNellAngolo(img, col[1], col) {
		t.Error("voce di un elenco nell'angolo presa per firma del canale")
	}
	_ = color.RGBA{}
}

// Le frasi pronte non si portano dietro un apice letto in fondo dall'OCR.
func TestMattinoApiceSoloInFondo(t *testing.T) {
	if v, ok := fraseMenu("FIGHT YOUR WAY OUT'"); !ok || v != "Apriti la strada combattendo" {
		t.Errorf("FIGHT YOUR WAY OUT': %q %v", v, ok)
	}
	if got := conPunteggiatura("Apriti la strada combattendo", "FIGHT YOUR WAY OUT'"); got != "APRITI LA STRADA COMBATTENDO" {
		t.Errorf("conPunteggiatura: %q", got)
	}
	// la virgoletta che chiude una citazione aperta resta
	if got := conPunteggiatura("Pronto", "\"Ready\""); got != "Pronto\"" {
		t.Errorf("citazione: %q", got)
	}
	if got := conPunteggiatura("Caricamento", "Loading..."); got != "Caricamento..." {
		t.Errorf("puntini: %q", got)
	}
}

// Un obiettivo di quattro parole con il segno della missione accanto non è il nome di un tasto:
// non deve stare nel riquadro stretto dei comandi.
func TestMattinoObiettivoConIconaNonEComando(t *testing.T) {
	ob := Blocco{Testo: "FIGHT YOUR WAY OUT", X: 54, Y: 38, W: 200, H: 18, Righe: 1, IconaSx: 30}
	if etichettaComando(ob) {
		t.Error("obiettivo con icona preso per comando")
	}
	for _, b := range []Blocco{
		{Testo: "Sprint", X: 100, Y: 900, W: 60, H: 16, Righe: 1, IconaSx: 70},
		{Testo: "Open Map", X: 100, Y: 900, W: 80, H: 16, Righe: 1, IconaSx: 70},
		{Testo: "Pick up item", X: 100, Y: 900, W: 110, H: 16, Righe: 1, IconaSx: 70},
		{Testo: "Hold to skip", X: 100, Y: 900, W: 110, H: 16, Righe: 1},
	} {
		if !etichettaComando(b) {
			t.Errorf("%q: è l'etichetta di un comando", b.Testo)
		}
	}
}

// Comandi con il tasto davanti: si traduce la voce, il tasto resta.
func TestMattinoTastoDavanti(t *testing.T) {
	casi := map[string]string{
		"[X] Transfer":    "[X] Trasferisci",
		"O Transfer":      "O Trasferisci",
		"0 Transfer":      "0 Trasferisci",
		"X Transfer":      "X Trasferisci",
		"[Esc] CANCEL":    "[Esc] ANNULLA",
		"Esc) CANCEL":     "[Esc] ANNULLA",
		"(ESC) CANCEL":    "[ESC] ANNULLA",
		"SPACE SKIP":      "SPAZIO SALTA",
		"[SPACE] CONFIRM": "[SPAZIO] CONFERMA",
		"O Continue":      "O Continua",
	}
	for en, atteso := range casi {
		if v, ok := fraseMenuConNumero(en); !ok || v != atteso {
			t.Errorf("%q: %q %v (atteso %q)", en, v, ok, atteso)
		}
	}
	// "A" e "I" senza parentesi sono parole: niente tasto davanti
	if v, ok := tastoDavanti("A New Game"); ok {
		t.Errorf("A New Game: %q", v)
	}
	// "E to open" non è una voce col tasto davanti
	if v, ok := tastoDavanti("E to open"); ok {
		t.Errorf("E to open: %q", v)
	}
	// i comandi "Hold"/"Press" restano quelli di prima
	for en, atteso := range map[string]string{"Transfer": "Trasferisci", "Hold to Skip": "Tieni premuto per saltare"} {
		if v, ok := fraseMenuConNumero(en); !ok || v != atteso {
			t.Errorf("%q: %q %v", en, v, ok)
		}
	}
	if v, ok := fraseMenuConNumero("Press E to interact"); !ok || v == "" {
		t.Errorf("Press E to interact: %q %v", v, ok)
	}
}

// Le voci tradotte male dal traduttore veloce nel banco ora escono dalle frasi pronte.
func TestMattinoFrasiPronteNuove(t *testing.T) {
	casi := map[string]string{
		"Close Tutorial":           "Chiudi tutorial",
		"CHARACTER SELECT":         "Selezione del personaggio",
		"CONFIRM CHARACTER CHOICE": "Conferma la scelta del personaggio",
		"CYCLE TARGETS":            "Cambia bersaglio",
		"BAYONET CHARGE":           "Carica alla baionetta",
		"PLACE BACK":               "Rimetti a posto",
		"Press to Preview":         "Premi per l'anteprima",
		"Clear the enemy waves":    "Sconfiggi le ondate nemiche",
		"COLLECTIBLES PICKUP":      "Raccolta dei collezionabili",
	}
	for en, atteso := range casi {
		if v, ok := fraseMenuConNumero(en); !ok || v != atteso {
			t.Errorf("%q: %q %v (atteso %q)", en, v, ok, atteso)
		}
	}
	for en, atteso := range map[string]string{
		"TUTORIAL: BLOCK ATTACKS": "TUTORIAL: PARA GLI ATTACCHI",
		"TUTORIAL: HEAVY ATTACKS": "TUTORIAL: ATTACCHI PESANTI",
		"TUTORIAL: SPELLS":        "TUTORIAL: INCANTESIMI",
		"TUTORIAL: DASH":          "TUTORIAL: BALZO",
	} {
		if v, ok := fraseMenuConNumero(en); !ok || v != atteso {
			t.Errorf("%q: %q %v (atteso %q)", en, v, ok, atteso)
		}
	}
}

// Più comandi sulla stessa riga con le icone, e i tasti lunghi tra parentesi.
func TestMattinoVociConIconeETastiLunghi(t *testing.T) {
	casi := map[string]string{
		"@ Next Character @ Previous Character @ Restore Default": "@ Personaggio successivo @ Personaggio precedente @ Ripristina predefinito",
		"@ Select @ Change Car":                                   "@ Seleziona @ Cambia auto",
		"[END] END TURN":                                          "[END] FINE TURNO",
		"END) END TURN":                                           "[END] FINE TURNO",
		"O Help":                                                  "O Aiuto",
		"EDIT LICENSE PLATE":                                      "Modifica la targa",
		"TUTORIAL: KICK":                                          "Tutorial: calcio",
		"End Turn":                                                "Fine turno",
	}
	for en, atteso := range casi {
		v, ok := fraseMenuConNumero(en)
		if !ok || !strings.EqualFold(v, atteso) {
			t.Errorf("%q: %q %v (atteso %q)", en, v, ok, atteso)
		}
	}
	// senza parentesi le parole che sono anche nomi di tasti restano parole
	for _, en := range []string{"Enter Name", "Home Page", "End Game", "Del Mar"} {
		if v, ok := tastoDavanti(en); ok {
			t.Errorf("%q preso per tasto davanti: %q", en, v)
		}
	}
	// una voce sola con l'icona davanti non cambia ("@ ACTION COST" resta quella di prima)
	if v, ok := vociConIcone("@ Action Cost"); ok {
		t.Errorf("una voce sola: %q", v)
	}
	if v, ok := vociConIcone("Press@to switch weapons."); ok {
		t.Errorf("frase con l'icona in mezzo: %q", v)
	}
}

// I comandi tradotti stanno nel loro riquadro (rimpiccioliti al massimo fino a dove si leggono).
func TestMattinoComandiNelRiquadro(t *testing.T) {
	for en, it := range map[string]string{
		"[X] Transfer": "[X] Trasferisci",
		"O Transfer":   "O Trasferisci",
		"Transfer":     "Trasferisci",
		"Hold to Skip": "Tieni premuto per saltare",
		"SPACE SKIP":   "SPAZIO SALTA",
		"[Esc] CANCEL": "[Esc] ANNULLA",
	} {
		b := Blocco{Testo: en, Tradotto: it, X: 1500, Y: 1000, W: int(larghezzaLettere(en) * 16), H: 16, Righe: 1, IconaSx: 1470}
		if m := adattaAlRiquadro(&b); m != "" {
			t.Errorf("%q -> %q: %s", en, it, m)
		}
	}
}
