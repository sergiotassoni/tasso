package traduce

// Traduzioni fisse (26/11, prova in PC Cafe Simulator): girando la visuale la stessa scritta
// inglese veniva riletta e a volte tradotta in modo diverso (motore diverso, correzione dell'IA
// arrivata a metà, memoria aggiornata): sotto gli occhi del giocatore l'italiano cambiava.
// Regola: la prima traduzione mostrata di una scritta resta QUELLA per tutta la sessione di Tasso.
// Quando la stessa scritta inglese ricompare (anche dopo aver guardato altrove) esce subito la
// stessa traduzione, senza passare dal traduttore.
//
// La chiave è il testo inglese "normalizzato" come in stessaScritta: minuscolo, senza spazi né
// segni, con le lettere che l'OCR scambia ridotte a una (l/I/1, O/0, rn/m), più i numeri e i
// segni esatti ("Level 5" e "Level 6" restano due scritte diverse). Con la coppia di lingue:
// cambiando lingua le traduzioni fisse della lingua di prima non valgono.

import (
	"strings"
	"sync"
)

type traduzioniFisse struct {
	mu     sync.Mutex
	m      map[string]string
	ordine []string // per togliere le più vecchie oltre il limite
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

func chiaveFissa(coppia, testo string) string {
	t := strings.TrimSpace(testo)
	if t == "" {
		return ""
	}
	l := lettereConfuse(t)
	if l == "" {
		return ""
	}
	return coppia + "|" + l + "|" + numeriESegni(t)
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
	v, ok := f.m[k]
	return v, ok
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
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.m == nil {
		f.m = map[string]string{}
	}
	if _, ok := f.m[k]; ok {
		return
	}
	f.m[k] = tradotto
	f.ordine = append(f.ordine, k)
	if len(f.ordine) > maxFisse {
		delete(f.m, f.ordine[0])
		f.ordine = f.ordine[1:]
	}
}

// fissata: la scritta ha già una traduzione fissa (qualunque sia).
func (f *traduzioniFisse) fissata(coppia, testo string) bool {
	_, ok := f.cerca(coppia, testo)
	return ok
}
