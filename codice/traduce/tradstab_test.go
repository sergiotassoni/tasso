package traduce

import "testing"

// Banco 19:22 f.45-49: "Early Morning" già tradotta e confermata ("Mattina presto") tornava in
// inglese ogni volta che sotto si leggeva male l'obiettivo ("t Head to the faculty office J").
func TestTitoloConfermatoNonSaltaPerRigaSotto(t *testing.T) {
	titolo := Blocco{Testo: "Early Morning", X: 1513, Y: 101, W: 230, H: 34, Righe: 1, Tradotto: "Mattina presto", GiaMostrata: true}
	sotto := Blocco{Testo: "t Head to the faculty office J", X: 1400, Y: 178, W: 420, H: 30, Righe: 1}
	if tenute, _ := senzaMezzeRighe([]Blocco{titolo}, []Blocco{sotto}); len(tenute) != 1 {
		t.Fatal("il titolo confermato è tornato in inglese per la riga sotto")
	}
	if n := mezzeDisegnate([]Blocco{titolo}, []Blocco{sotto}); n != 0 {
		t.Fatalf("mezze disegnate = %d", n)
	}
	// senza la decisione tenuta resta la regola di prima (via)
	nuovo := titolo
	nuovo.GiaMostrata = false
	if tenute, _ := senzaMezzeRighe([]Blocco{nuovo}, []Blocco{sotto}); len(tenute) != 0 {
		t.Fatal("una scritta non confermata deve seguire la riga sotto in inglese")
	}
}

// Mai mezza riga: la stessa riga o i riquadri accavallati tolgono anche la scritta confermata, e
// la prima riga lunga di un dialogo (4+ parole) segue sempre la riga sotto.
func TestConfermataMaiMezzaRiga(t *testing.T) {
	titolo := Blocco{Testo: "Early Morning", X: 1513, Y: 101, W: 230, H: 34, Righe: 1, Tradotto: "Mattina presto", GiaMostrata: true}
	stessa := Blocco{Testo: "Early Morning_ EVO t Head to the faculty office J", X: 1361, Y: 95, W: 700, H: 40, Righe: 1}
	if tenute, _ := senzaMezzeRighe([]Blocco{titolo}, []Blocco{stessa}); len(tenute) != 0 {
		t.Fatal("riquadri accavallati: mezza riga disegnata")
	}
	accanto := Blocco{Testo: "is here for the day", X: 1750, Y: 101, W: 260, H: 34, Righe: 1}
	if tenute, _ := senzaMezzeRighe([]Blocco{titolo}, []Blocco{accanto}); len(tenute) != 0 {
		t.Fatal("stessa riga: mezza riga disegnata")
	}
	prima := Blocco{Testo: "Oh, and does she have a", X: 500, Y: 800, W: 500, H: 34, Righe: 1, Tradotto: "Oh, e ha un", GiaMostrata: true}
	seguito := Blocco{Testo: "boyfriend? Nobody seems to know.", X: 500, Y: 840, W: 560, H: 34, Righe: 1}
	if tenute, _ := senzaMezzeRighe([]Blocco{prima}, []Blocco{seguito}); len(tenute) != 0 {
		t.Fatal("prima riga di un dialogo tradotta sopra il seguito in inglese")
	}
	corto := Blocco{Testo: "Talk to the", X: 500, Y: 800, W: 200, H: 34, Righe: 1, Tradotto: "Parla con il", GiaMostrata: true}
	if tenute, _ := senzaMezzeRighe([]Blocco{corto}, []Blocco{Blocco{Testo: "teacher in the hall", X: 500, Y: 840, W: 300, H: 34, Righe: 1}}); len(tenute) != 0 {
		t.Fatal("riga corta che chiede il seguito tradotta sopra l'inglese")
	}
}
