package traduce

// Cataloghi scritti per Tasso Traduce: glossario da gioco (termini) e frasi fisse dei menu
// (traduzioni già pronte e controllate, che escono subito con qualsiasi motore).

import (
	"bufio"
	_ "embed"
	"strings"
	"sync"
	"unicode"
)

//go:embed dati/glossario-giochi.tsv
var glossarioGiochiTSV string

//go:embed dati/frasi-menu.tsv
var frasiMenuTSV string

var (
	catOnce    sync.Once
	termGiochi map[string]string
	frasiMenu  map[string]string
)

func leggiTSV(s string) map[string]string {
	m := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		l := sc.Text()
		if strings.HasPrefix(l, "#") || strings.TrimSpace(l) == "" {
			continue
		}
		en, it, ok := strings.Cut(l, "\t")
		if ok && strings.TrimSpace(en) != "" && strings.TrimSpace(it) != "" {
			m[strings.TrimSpace(en)] = strings.TrimSpace(it)
		}
	}
	return m
}

func caricaCataloghi() {
	catOnce.Do(func() {
		termGiochi = leggiTSV(glossarioGiochiTSV)
		frasiMenu = map[string]string{}
		for en, it := range leggiTSV(frasiMenuTSV) {
			frasiMenu[chiaveFrase(en)] = it
		}
	})
}

// chiaveFrase: minuscole, spazi singoli, senza punteggiatura finale.
func chiaveFrase(s string) string {
	s = strings.ToLower(strings.Join(strings.Fields(s), " "))
	return strings.TrimRightFunc(s, func(r rune) bool { return unicode.IsPunct(r) && r != '+' })
}

// fraseMenu: traduzione pronta se la battuta è una frase fissa conosciuta. Rimette la
// punteggiatura finale dell'originale ("Loading..." -> "Caricamento...").
func fraseMenu(battuta string) (string, bool) {
	caricaCataloghi()
	b := strings.TrimSpace(battuta)
	if b == "" || len(b) > 120 {
		return "", false
	}
	it, ok := frasiMenu[chiaveFrase(b)]
	if !ok {
		return "", false
	}
	fine := b[len(strings.TrimRightFunc(b, func(r rune) bool { return unicode.IsPunct(r) && r != '+' })):]
	itSenza := strings.TrimRightFunc(it, func(r rune) bool { return unicode.IsPunct(r) && r != '+' })
	if fine != "" {
		return itSenza + fine, true
	}
	return it, true
}

// conGiochi: il glossario dell'utente con sotto il glossario da gioco (l'utente vince).
func (g Glossario) conGiochi() Glossario {
	caricaCataloghi()
	out := Glossario{NonTradurre: g.NonTradurre, Termini: make(map[string]string, len(termGiochi)+len(g.Termini))}
	for k, v := range termGiochi {
		out.Termini[k] = v
	}
	for k, v := range g.Termini {
		out.Termini[k] = v
	}
	for _, n := range g.NonTradurre {
		delete(out.Termini, n)
	}
	return out
}
