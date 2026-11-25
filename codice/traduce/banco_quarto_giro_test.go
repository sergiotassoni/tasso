package traduce

// Quarto giro sul banco di prova vero (OCR di Windows, risultati-4): l'unico caso che mostrava
// sullo schermo una traduzione sbagliata. Regola: meglio l'inglese del gioco che mezza frase o
// una frase senza senso. Ogni regola vale per tutti i giochi, non per questa scritta.

import (
	"strings"
	"testing"
	"time"
)

// 30-caricamento: "TIP: Crouching makes less noise when enemies are nearby." (una riga sola,
// centrata, y≈945) letta da Windows in due pezzi sulla stessa linea, con "les(s)" perso o letto
// due volte in mezzo:
//
//	[563,945] "TIP: Crouching makes less"          -> "Consiglio: Accovacciarsi rende meno"
//	[886,945] "s noise when enemies arepe earby."  -> "s rumore quando i nemici sonoorecchio by."
//
// I due pezzi sono UNA frase (il primo finisce con "less", il secondo comincia minuscolo): si
// uniscono e si giudica la riga intera; letta male, restano in inglese tutti e due.
func TestFraseInDuePezziStorpiata(t *testing.T) {
	img := apriBanco3(t, "30-caricamento") // ritaglio da (500,900)
	pezzoA := func(wLess float64) RigaOCR {
		return rigaParole(946, 29, "TIP:", 563.0, 46.0, "Crouching", 622.0, 133.0, "makes", 766.0, 84.0, "less", 862.0, wLess)
	}
	pezzoB := func(xS, wS, h float64) RigaOCR {
		return rigaParole(946+(29-h)/2, h, "s", xS, wS, "noise", 922.0, 69.0, "when", 1002.0, 69.0, "enemies", 1084.0, 112.0,
			"arepe", 1207.0, 60.0, "earby.", 1272.0, 82.0)
	}
	casi := []struct {
		nome string
		a, b RigaOCR
	}{
		{"attaccati", pezzoA(24), pezzoB(886, 23, 29)},          // "les" | "s" (la s a metà)
		{"s letta due volte", pezzoA(48), pezzoB(886, 23, 29)},  // "less" e di nuovo "s" sopra
		{"buco di una parola", pezzoA(20), pezzoB(899, 10, 29)}, // "les" corto, buco più largo di uno spazio
		{"riletto più basso", pezzoA(48), pezzoB(899, 10, 22)},  // pezzo riletto con il riquadro più basso
		{"senza la s", pezzoA(20), rigaParole(946, 29, "noise", 922.0, 69.0, "when", 1002.0, 69.0, "enemies", 1084.0, 112.0, "arepe", 1207.0, 60.0, "earby.", 1272.0, 82.0)},
	}
	for _, c := range casi {
		righe := []RigaOCR{spostaRiga(c.a, 500, 900), spostaRiga(c.b, 500, 900)}
		bl := blocchiLetti(img, righe, 1, nil, time.Now())
		if len(bl) != 1 {
			t.Errorf("%s: i due pezzi della frase non uniti: %q", c.nome, chiaviBlocchi(bl))
		}
		for i := range bl {
			b := bl[i]
			if motivoNonTradurre(b) != "" {
				continue
			}
			if controllaCompleta(img, &b, bl) != "" {
				continue
			}
			t.Errorf("%s: %q si mostrerebbe tradotta (affidabilità %d)", c.nome, b.Testo, b.Affid)
		}
	}
	// la stessa riga letta bene in due pezzi: unita e tradotta intera
	buona := []RigaOCR{
		spostaRiga(pezzoA(48), 500, 900),
		spostaRiga(rigaParole(946, 29, "noise", 922.0, 69.0, "when", 1002.0, 69.0, "enemies", 1084.0, 112.0, "are", 1207.0, 39.0, "nearby.", 1259.0, 95.0), 500, 900),
	}
	bl := blocchiLetti(img, buona, 1, nil, time.Now())
	if len(bl) != 1 || motivoNonTradurre(bl[0]) != "" {
		t.Fatalf("frase letta bene in due pezzi: %q", chiaviBlocchi(bl))
	}
	if !strings.Contains(bl[0].Testo, "less noise when enemies are nearby") {
		t.Errorf("frase unita: %q", bl[0].Testo)
	}
}

// Il pezzo storpiato da solo non passa: lettera sola minuscola all'inizio ("s noise…"), parola
// inventata in un pezzo che comincia minuscolo, due parole inventate nella lettura grezza.
func TestPezzoStorpiatoNonPassa(t *testing.T) {
	for _, s := range []string{
		"s noise when enemies arepe earby.",
		"s noise when enemies are nearby.",
		"noise when enemies arepe nearby.",
		"TIP: Crouching makes less noise when enemies arepe earby.",
		"TIP: Crouching makes less s noise when enemies arepe earby.",
	} {
		if c, a := correggiRigaOCR(s); a != letturaDubbia {
			t.Errorf("%q -> %q affidabilità %d: doveva restare in inglese", s, c, a)
		}
	}
	for _, s := range []string{
		"TIP: Crouching makes less noise when enemies are nearby.",
		"a sturdy blade forged in the mountains.",
		"Low health! Find a medkit",
	} {
		if c, a := correggiRigaOCR(s); a == letturaDubbia {
			t.Errorf("%q -> %q scartata", s, c)
		}
	}
}

// Le file di pulsanti non dividono una frase: si divide solo dove il pezzo dopo comincia con la
// maiuscola o un numero e quello prima non finisce con una parola che vuole il seguito.
func TestPulsantiNonDividonoFrasi(t *testing.T) {
	// buco largo dopo "less" e "Noise" con la maiuscola: resta una frase
	r := rigaParole(946, 29, "TIP:", 563.0, 46.0, "Crouching", 622.0, 133.0, "makes", 766.0, 84.0, "less", 862.0, 48.0,
		"Noise", 960.0, 69.0, "when", 1040.0, 69.0, "enemies", 1122.0, 112.0)
	if bl := blocchiLetti(nil, []RigaOCR{r}, 1, nil, time.Now()); len(bl) != 1 {
		t.Errorf("frase divisa come una fila di pulsanti: %q", chiaviBlocchi(bl))
	}
	b := bloccoDaParole(r.Parole, 1)
	b.Testo = testoParole(r.Parole)
	b.Grezzo = b.Testo
	if v := vociDiUnaFila(b); v != nil {
		t.Errorf("vociDiUnaFila ha diviso una frase: %d voci", len(v))
	}
	// pulsanti veri: "Show more" (due parole) chiude una voce
	f := rigaParole(1016, 27, "Show", 141.0, 70.0, "more", 219.0, 66.0, "Back", 360.0, 65.0)
	if got := strings.Join(chiaviBlocchi(blocchiLetti(nil, []RigaOCR{f}, 1, nil, time.Now())), " | "); got != "Show more | Back" {
		t.Errorf("fila di pulsanti: %q", got)
	}
}
