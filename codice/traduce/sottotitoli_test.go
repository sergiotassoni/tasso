package traduce

import "testing"

func TestSottotitoloPronto(t *testing.T) {
	ultimo, cand := "", ""
	giro := func(letto string) bool {
		ok, c := sottotitoloPronto(letto, ultimo, cand)
		cand = c
		if ok {
			ultimo = letto
		}
		return ok
	}
	// compare a metà, poi intero due volte: traduco solo la seconda lettura intera
	if giro("We have to") || giro("We have to leave before the storm") {
		t.Fatal("tradotto un sottotitolo che stava ancora comparendo")
	}
	if !giro("We have to leave before the storm") {
		t.Fatal("sottotitolo fermo non tradotto")
	}
	// l'OCR sbaglia una lettera sullo stesso sottotitolo: niente nuova traduzione
	if giro("We have to 1eave before the storm") || giro("We have to leave before the storm") {
		t.Fatal("ritradotto lo stesso sottotitolo")
	}
	// una negazione cambia il senso: è un sottotitolo nuovo
	giro("We do not have to leave before the storm")
	if !giro("We do not have to leave before the storm") {
		t.Fatal("sottotitolo nuovo non tradotto")
	}
}

func TestRigheSottotitolo(t *testing.T) {
	if g := righeSottotitolo("o — c\nMARA: Keep your voice down."); g != "MARA: Keep your voice down." {
		t.Errorf("pezzetti dello sfondo rimasti: %q", g)
	}
	if g := righeSottotitolo("No."); g != "No." {
		t.Errorf("sottotitolo corto tolto: %q", g)
	}
	if g := righeSottotitolo("I never wanted any of this,\nbut we don't have a choice."); g != "I never wanted any of this,\nbut we don't have a choice." {
		t.Errorf("riga vera tolta: %q", g)
	}
}
