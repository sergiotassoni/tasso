package traduce

// Stabilità delle scritte già mostrate (collaudo finale 26/11, prova su 6 giochi mai visti).
//
//  1. Maiuscole: la traduzione fissata o presa dalla memoria segue le maiuscole della scritta di
//     ADESSO. La chiave delle traduzioni fisse è minuscola, quindi "Reach the camp" prendeva
//     "RAGGIUNGI IL CAMPO" fissata prima per "REACH THE CAMP" (007 First Light, 30 fotogrammi).
//  2. Decisione tenuta: una scritta già tradotta e mostrata in questa sessione (stesso testo,
//     normalizzato) non torna in inglese perché un fotogramma la giudica "scritta del mondo" o
//     "lettura incerta". "Early Morning" (Persona 3) cambiava una decina di volte tra italiano e
//     inglese, e "IMPACT IN 86" una quindicina. Le regole che evitano le righe mezze inglesi
//     (parola accanto non letta, pezzi della stessa frase, traduzioni sovrapposte) restano tutte:
//     tenere la decisione non deve mai lasciare mezza riga inglese accanto all'italiano.

import (
	"strings"
	"unicode"
)

// maiuscoleDellaScritta: it (traduzione presa dalla memoria) con le maiuscole della scritta en
// di adesso. Scritta tutta maiuscola: traduzione tutta maiuscola. Scritta normale e traduzione
// tutta maiuscola (salvata da una scritta maiuscola): solo l'iniziale maiuscola, e restano com'erano
// le parole che nella scritta inglese sono uguali (nomi, sigle, tasti: "Bond", "HP", "E").
func maiuscoleDellaScritta(en, it string) string {
	en = strings.TrimSpace(en)
	if en == "" || strings.TrimSpace(it) == "" {
		return it
	}
	if tuttoMaiuscolo(en) {
		if strings.ContainsFunc(it, unicode.IsLower) {
			return strings.ToUpper(it)
		}
		return it
	}
	if !tuttoMaiuscolo(it) || !strings.ContainsFunc(en, unicode.IsLower) {
		return it
	}
	uguali := map[string]bool{} // le parole maiuscole della scritta inglese (sigle, tasti)
	for _, w := range strings.FieldsFunc(en, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if !strings.ContainsFunc(w, unicode.IsLower) {
			uguali[w] = true
		}
	}
	nomi := map[string]string{} // le parole con l'iniziale maiuscola: "BOND" → "Bond"
	for _, w := range strings.FieldsFunc(en, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if r := []rune(w); len(r) > 1 && unicode.IsUpper(r[0]) && strings.ContainsFunc(w, unicode.IsLower) {
			nomi[strings.ToUpper(w)] = w
		}
	}
	var b strings.Builder
	parola := []rune{}
	primaParola := true
	chiudi := func() {
		if len(parola) == 0 {
			return
		}
		w := string(parola)
		switch {
		case len(parola) <= 3 && uguali[w], len(parola) == 2 && !paroleCorteItaliane[strings.ToLower(w)]:
			// sigla o tasto rimasti uguali ("HP", "E", "LMB") o sigla italiana ("PV" per "HP")
		case nomi[w] != "":
			w = nomi[w]
		default:
			w = strings.ToLower(w)
			if primaParola {
				r := []rune(w)
				r[0] = unicode.ToUpper(r[0])
				w = string(r)
			}
		}
		primaParola = false
		b.WriteString(w)
		parola = parola[:0]
	}
	for _, r := range it {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			parola = append(parola, r)
			continue
		}
		chiudi()
		b.WriteRune(r)
	}
	chiudi()
	out := b.String()
	if r := []rune(en); unicode.IsLower(r[0]) { // "press E" → l'iniziale minuscola come il gioco
		if ro := []rune(out); len(ro) > 0 && unicode.IsUpper(ro[0]) && !uguali[strings.Fields(out)[0]] {
			ro[0] = unicode.ToLower(ro[0])
			out = string(ro)
		}
	}
	return out
}

// mostrataPrima: la scritta (testo esatto, normalizzato come le traduzioni fisse) è già stata
// tradotta e disegnata in questa sessione. Solo il testo esatto: una lettura con una lettera
// diversa non conta, potrebbe essere un'altra scritta.
func (f *traduzioniFisse) mostrataPrima(coppia, testo string) bool {
	if f == nil {
		return false
	}
	k := chiaveFissa(coppia, testo)
	if k == "" {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.m[k]
	return ok
}

// segnaGiaMostrate: segna le scritte già tradotte e mostrate (vedi Blocco.GiaMostrata).
func segnaGiaMostrate(f *traduzioniFisse, coppia string, bl []Blocco) {
	for i := range bl {
		bl[i].GiaMostrata = f.mostrataPrima(coppia, bl[i].Testo)
	}
}

// motivoTenibile: i motivi per lasciare l'inglese che una scritta già mostrata in italiano non
// segue (cambiano da un fotogramma all'altro sulla stessa scritta ferma). Gli altri restano
// sempre: firma o logo nell'angolo, righe incomplete, pezzi di frase, traduzioni sovrapposte.
func motivoTenibile(m string) bool {
	switch {
	case m == motivoFirmaAngolo:
		return false
	case strings.HasPrefix(m, "scritta del mondo"):
		return true
	case m == "lettura incerta":
		return true
	}
	return false
}

// paroleCorteItaliane: le parole italiane di due lettere (le altre di due lettere in una
// traduzione tutta maiuscola sono sigle: "PV", "PM", "PE", "IA").
var paroleCorteItaliane = map[string]bool{"il": true, "lo": true, "la": true, "le": true, "li": true,
	"un": true, "di": true, "da": true, "in": true, "su": true, "tu": true, "io": true, "mi": true, "ti": true,
	"ci": true, "vi": true, "si": true, "ne": true, "se": true, "ma": true, "ho": true, "ha": true, "fa": true,
	"va": true, "sa": true, "no": true, "sì": true, "è": true, "ai": true, "al": true, "re": true, "ok": true,
	"tè": true, "dì": true, "là": true, "lì": true, "po": true, "ad": true, "ed": true, "od": true, "gi": true}
