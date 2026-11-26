package traduce

// Traduzioni fisse (26/11, prova in PC Cafe Simulator): girando la visuale la stessa scritta
// inglese veniva riletta e a volte tradotta in modo diverso (motore diverso, correzione dell'IA
// arrivata a metà, memoria aggiornata): sotto gli occhi del giocatore l'italiano cambiava.
// Regola: la prima traduzione mostrata di una scritta resta QUELLA per tutta la sessione di Tasso.
// Quando la stessa scritta inglese ricompare (anche dopo aver guardato altrove) esce subito la
// stessa traduzione, senza passare dal traduttore.
//
// La chiave è il testo inglese esatto, solo normalizzato: minuscolo, spazi singoli, senza la
// punteggiatura in fondo. Con la coppia di lingue: cambiando lingua le traduzioni fisse della
// lingua di prima non valgono. Le letture con una lettera che l'OCR scambia (l/I/1, O/0, rn/m)
// trovano la traduzione fissa solo se le parole diverse NON sono entrambe parole vere: "Hel1o"
// trova "Hello", ma "Fall" non trova "Fail", "Mall" non trova "Mail", "Walt" non trova "Wait"
// (collaudo 26/11: con la chiave "lettere confuse" parole diverse prendevano la stessa
// traduzione per tutta la sessione).

import (
	"strings"
	"sync"
	"unicode"
)

type traduzioniFisse struct {
	mu     sync.Mutex
	m      map[string]string   // chiave esatta -> traduzione
	simili map[string][]string // chiave con le lettere confuse -> chiavi esatte
	ordine []string            // per togliere le più vecchie oltre il limite
}

const maxFisse = 5000

// fissePagina: le traduzioni fisse della traduzione dello schermo (tutta la sessione).
var fissePagina = &traduzioniFisse{}

func coppiaDi(tr *Traduttore) string {
	if tr != nil && tr.Coppia != nil {
		da, a := tr.Coppia()
		return da + ">" + a
	}
	return "en>it"
}

// chiaveFissa: coppia + testo esatto normalizzato ("" se non c'è testo).
func chiaveFissa(coppia, testo string) string {
	t := strings.ToLower(strings.Join(strings.Fields(testo), " "))
	t = strings.TrimRightFunc(t, func(r rune) bool { return unicode.IsPunct(r) && r != '+' && r != '%' && r != ')' })
	if lettereConfuse(t) == "" {
		return ""
	}
	return coppia + "|" + t
}

// chiaveSimile: coppia + lettere confuse + numeri e segni esatti (come stessaScritta).
func chiaveSimile(coppia, testo string) string {
	l := lettereConfuse(testo)
	if l == "" {
		return ""
	}
	return coppia + "|" + l + "|" + numeriESegni(testo)
}

// soloErroriOCR: a e b (chiavi esatte con la stessa chiave simile) differiscono solo in parole
// dove almeno una delle due non è una parola vera: è la stessa scritta letta male, non un'altra
// parola ("Fail"/"Fall" sono due parole vere: scritte diverse).
func soloErroriOCR(a, b string) bool {
	wa, wb := strings.Fields(a), strings.Fields(b)
	if len(wa) != len(wb) {
		// spazi diversi ("Holdto" / "Hold to"): lettera per lettera è uguale, si accetta solo se
		// una delle due letture ha una parola che non esiste
		for _, w := range append(wa, wb...) {
			if p := strings.Trim(w, ".,;:!?\"'()[]"); p != "" && !esiste(p) && !strings.ContainsAny(p, "0123456789") {
				return true
			}
		}
		return false
	}
	for i := range wa {
		if wa[i] == wb[i] {
			continue
		}
		pa, pb := strings.Trim(wa[i], ".,;:!?\"'()[]"), strings.Trim(wb[i], ".,;:!?\"'()[]")
		if pa == pb {
			continue
		}
		if esiste(pa) && esiste(pb) {
			return false
		}
	}
	return true
}

// cerca: la traduzione già mostrata di questa scritta, se c'è.
func (f *traduzioniFisse) cerca(coppia, testo string) (string, bool) {
	if f == nil {
		return "", false
	}
	k := chiaveFissa(coppia, testo)
	if k == "" {
		return "", false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if v, ok := f.m[k]; ok {
		return v, true
	}
	pre := coppia + "|"
	for _, e := range f.simili[chiaveSimile(coppia, testo)] {
		if soloErroriOCR(strings.TrimPrefix(k, pre), strings.TrimPrefix(e, pre)) {
			if v, ok := f.m[e]; ok {
				return v, true
			}
		}
	}
	return "", false
}

// fissa: da adesso la scritta testo si traduce sempre tradotto. Se c'era già, vince la prima.
func (f *traduzioniFisse) fissa(coppia, testo, tradotto string) {
	if f == nil || strings.TrimSpace(tradotto) == "" {
		return
	}
	k := chiaveFissa(coppia, testo)
	if k == "" {
		return
	}
	if _, ok := f.cerca(coppia, testo); ok {
		return // già fissata (anche come la stessa scritta letta con un errore dell'OCR)
	}
	ks := chiaveSimile(coppia, testo)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.m == nil {
		f.m, f.simili = map[string]string{}, map[string][]string{}
	}
	if _, ok := f.m[k]; ok {
		return
	}
	f.m[k] = tradotto
	f.simili[ks] = append(f.simili[ks], k)
	f.ordine = append(f.ordine, k)
	if len(f.ordine) > maxFisse {
		via := f.ordine[0]
		delete(f.m, via)
		f.ordine = f.ordine[1:]
		for sk, l := range f.simili { // poche voci per chiave: la pulizia costa poco
			for i, e := range l {
				if e == via {
					f.simili[sk] = append(l[:i:i], l[i+1:]...)
					if len(f.simili[sk]) == 0 {
						delete(f.simili, sk)
					}
					break
				}
			}
		}
	}
}

// fissata: la scritta ha già una traduzione fissa (qualunque sia).
func (f *traduzioniFisse) fissata(coppia, testo string) bool {
	_, ok := f.cerca(coppia, testo)
	return ok
}

// fissaMostrate: fissa le traduzioni delle scritte disegnate davvero (dopo tutti i filtri).
func fissaMostrate(f *traduzioniFisse, coppia string, mostrate []Blocco) {
	for _, b := range mostrate {
		if b.DaFissare != "" {
			f.fissa(coppia, b.Testo, b.DaFissare)
		}
	}
}
