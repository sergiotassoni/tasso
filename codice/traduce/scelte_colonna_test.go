package traduce

import "testing"

// Le scelte una sotto l'altra sono un gruppo: se una resta in inglese ("I'll buy!"), anche
// "Never mind." resta in inglese invece di diventare "Non importa." da sola.
func TestScelteInColonnaTutteOVia(t *testing.T) {
	compra := Blocco{Testo: "I'll buy!", X: 600, Y: 700, W: 110, H: 26, Righe: 1, Em: 26}
	lascia := Blocco{Testo: "Never mind.", Tradotto: "Non importa.", X: 600, Y: 740, W: 140, H: 26, Righe: 1, Em: 26}
	altro := Blocco{Testo: "Tell me more.", Tradotto: "Dimmi di più.", X: 600, Y: 780, W: 150, H: 26, Righe: 1, Em: 26}
	tenute, tolte := senzaMezzeRighe([]Blocco{lascia, altro}, []Blocco{compra})
	if len(tenute) != 0 || len(tolte) != 2 {
		t.Errorf("scelte mezze tradotte: tenute %q", testiBlocchi(tenute))
	}
	// tutte con la traduzione: si disegnano tutte
	compra.Tradotto = "Compro!"
	if tenute, _ := senzaMezzeRighe([]Blocco{compra, lascia, altro}, nil); len(tenute) != 3 {
		t.Errorf("scelte tutte tradotte: tenute %q", testiBlocchi(tenute))
	}
	// una scritta inglese lontana (altra colonna, o molto più in basso) non conta
	lontana := Blocco{Testo: "Some text here", X: 1300, Y: 700, W: 160, H: 26, Righe: 1, Em: 26}
	sotto := Blocco{Testo: "Quest log", X: 600, Y: 960, W: 120, H: 26, Righe: 1, Em: 26}
	if tenute, _ := senzaMezzeRighe([]Blocco{lascia}, []Blocco{lontana, sotto}); len(tenute) != 1 {
		t.Errorf("scritta lontana trattata come scelta dello stesso elenco")
	}
	// una frase lunga di due righe non è una scelta
	par := Blocco{Testo: "The merchant looks at you and waits for an answer about the price", X: 600, Y: 650, W: 700, H: 52, Righe: 2, Em: 26}
	if tenute, _ := senzaMezzeRighe([]Blocco{lascia}, []Blocco{par}); len(tenute) != 1 {
		t.Errorf("paragrafo sopra trattato come scelta")
	}
}
