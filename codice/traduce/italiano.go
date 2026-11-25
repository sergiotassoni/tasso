package traduce

// Controllo dell'italiano scritto dal traduttore veloce (Mozilla). A volte esce una parola che in
// italiano non esiste: "Inestare il negozio all'angolo", "PUNCI DI SVOGLIAMENTO", "Propriettario
// di negozi", "zatteri: (urlente)". Sullo schermo è peggio dell'inglese del gioco: una traduzione
// con una parola inventata non si mostra (resta l'inglese) e non si salva in memoria.
//
// L'elenco delle parole (dati/parole-italiane.txt.gz, poche decine di migliaia di forme base)
// viene dalle frasi italiane già nel programma e da un elenco di parole comuni; le forme dei verbi,
// i plurali, gli avverbi in -mente, le parole con l'apostrofo e con i pronomi attaccati
// ("distrarli") si riconoscono qui dalla loro forma base. Nel dubbio la parola NON è inventata:
// nomi propri, sigle, numeri, parole rimaste uguali all'inglese e parole inglesi usate in italiano
// ("sprint", "backup") non contano.

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"strings"
	"sync"
	"unicode"
)

//go:embed dati/parole-italiane.txt.gz
var paroleItalianeGz []byte

var (
	itOnce sync.Once
	itVoc  map[string]bool
)

func vocabolarioItaliano() map[string]bool {
	itOnce.Do(func() {
		itVoc = make(map[string]bool, 40000)
		zr, err := gzip.NewReader(bytes.NewReader(paroleItalianeGz))
		if err != nil {
			return
		}
		sc := bufio.NewScanner(zr)
		for sc.Scan() {
			w := strings.TrimSpace(sc.Text())
			if w != "" && !strings.HasPrefix(w, "#") {
				itVoc[w] = true
			}
		}
	})
	return itVoc
}

// desinenze dei verbi regolari per coniugazione (la radice + desinenza; la forma base è
// radice + are/ere/ire).
var (
	desinenzeAre = []string{"o", "i", "a", "iamo", "ate", "ano", "avo", "avi", "ava", "avamo", "avate", "avano",
		"erò", "erai", "erà", "eremo", "erete", "eranno", "erei", "eresti", "erebbe", "eremmo", "ereste", "erebbero",
		"ino", "iate", "assi", "asse", "assimo", "aste", "assero", "ai", "asti", "ò", "ammo", "arono",
		"ando", "ante", "anti", "ato", "ata", "ati", "are", "ar"}
	desinenzeEre = []string{"o", "i", "e", "iamo", "ete", "ono", "evo", "evi", "eva", "evamo", "evate", "evano",
		"erò", "erai", "erà", "eremo", "erete", "eranno", "erei", "eresti", "erebbe", "eremmo", "ereste", "erebbero",
		"a", "iate", "ano", "essi", "esse", "essimo", "este", "essero", "ei", "etti", "esti", "é", "è", "ette",
		"emmo", "erono", "ettero", "uto", "uta", "uti", "ute", "endo", "ente", "enti", "ere", "er"}
	desinenzeIre = []string{"o", "i", "e", "iamo", "ite", "ono", "isco", "isci", "isce", "iscono", "isca", "iscano",
		"ivo", "ivi", "iva", "ivamo", "ivate", "ivano", "irò", "irai", "irà", "iremo", "irete", "iranno", "irei",
		"iresti", "irebbe", "iremmo", "ireste", "irebbero", "a", "iate", "ano", "issi", "isse", "issimo", "iste",
		"issero", "ii", "isti", "ì", "immo", "irono", "ito", "ita", "iti", "endo", "ente", "enti", "ire", "ir"}
)

// pronomi attaccati in fondo ("prenderlo", "dimmi", "distrarli", "dandogliela")
var clitici = []string{"glielo", "gliela", "glieli", "gliele", "gliene", "melo", "mela", "meli", "mele", "telo",
	"tela", "teli", "tele", "selo", "sela", "seli", "sele", "celo", "cela", "ceneli", "velo", "vela", "mene", "tene",
	"sene", "cene", "vene", "gli", "lo", "la", "li", "le", "ne", "si", "mi", "ti", "ci", "vi"}

// prefissi che fanno parole nuove da parole note ("ricaricare", "sbloccare", "disattivare")
var prefissiIt = []string{"ri", "re", "dis", "s", "in", "im", "pre", "auto", "sovra", "sotto", "contro", "anti",
	"super", "multi", "inter", "ultra", "semi", "mini", "micro", "co", "post"}

// parolaItaliana: la parola (minuscola o no) è italiana, anche coniugata, al plurale, con
// l'apostrofo o con i pronomi attaccati.
func parolaItaliana(w string) bool {
	w = strings.ReplaceAll(strings.ToLower(w), "’", "'")
	// l'accento scritto con l'apostrofo ("perdero'", "e'", "pero'")
	if a, ok := strings.CutSuffix(w, "'"); ok && a != "" && !strings.Contains(a, "'") {
		if r := []rune(a); strings.ContainsRune("aeiou", r[len(r)-1]) {
			acc := map[rune]string{'a': "à", 'e': "è", 'i': "ì", 'o': "ò", 'u': "ù"}[r[len(r)-1]]
			if parolaItaliana(string(r[:len(r)-1])+acc) || (r[len(r)-1] == 'e' && parolaItaliana(string(r[:len(r)-1])+"é")) {
				return true
			}
		}
	}
	w = strings.Trim(w, "'")
	if w == "" {
		return true
	}
	if strings.Contains(w, "'") { // "dell'angolo", "c'è", "un'ombra"
		for _, p := range strings.Split(w, "'") {
			if p != "" && !parolaItaliana(p) {
				return false
			}
		}
		return true
	}
	return formaItaliana(w, 2)
}

func formaItaliana(w string, prof int) bool {
	v := vocabolarioItaliano()
	if len(v) == 0 || v[w] {
		return true
	}
	if formaFlessa(w) {
		return true
	}
	if prof <= 0 {
		return false
	}
	// pronomi attaccati, solo ai verbi: infinito senza la e ("prenderlo", "distrarli", "porlo"),
	// gerundio ("facendolo"), imperativo ("prendilo", "guardala"), "eccolo"
	for _, c := range clitici {
		r, ok := strings.CutSuffix(w, c)
		if !ok || len([]rune(r)) < 2 {
			continue
		}
		if strings.HasSuffix(r, "r") && (v[r+"e"] || v[r+"re"] || formaVerbo(r+"e") || formaVerbo(r+"re")) {
			return true
		}
		if r == "ecco" || ((strings.HasSuffix(r, "ando") || strings.HasSuffix(r, "endo") || strings.HasSuffix(r, "iamo")) && (v[r] || formaVerbo(r))) {
			return true
		}
		if (strings.ContainsAny(r[len(r)-1:], "aie") || strings.HasSuffix(r, "iamo")) && formaVerbo(r) {
			return true
		}
	}
	// prefissi
	for _, p := range prefissiIt {
		r, ok := strings.CutPrefix(w, p)
		if ok && len([]rune(r)) >= 4 && formaItaliana(r, prof-1) {
			return true
		}
	}
	return false
}

// formaFlessa: forma di un verbo, plurale/femminile, avverbio o superlativo di una parola nota.
func formaFlessa(w string) bool {
	return formaVerbo(w) || formaNome(w)
}

// formaVerbo: forma coniugata (regolare) di un verbo dell'elenco.
func formaVerbo(w string) bool {
	v := vocabolarioItaliano()
	in := func(s string) bool { return v[s] }
	for _, d := range desinenzeAre {
		if r, ok := strings.CutSuffix(w, d); ok && len([]rune(r)) >= 2 {
			if in(r+"are") || (strings.HasSuffix(r, "h") && in(r[:len(r)-1]+"are")) || in(r+"iare") {
				return true
			}
		}
	}
	for _, d := range desinenzeEre {
		if r, ok := strings.CutSuffix(w, d); ok && len([]rune(r)) >= 2 {
			// "traduc-" → tradurre, "pon-" → porre, "distra-" → distrarre
			if in(r+"ere") || in(r+"rre") || in(r[:len(r)-1]+"rre") {
				return true
			}
		}
	}
	for _, d := range desinenzeIre {
		if r, ok := strings.CutSuffix(w, d); ok && len([]rune(r)) >= 2 && in(r+"ire") {
			return true
		}
	}
	return false
}

// formaNome: plurale o femminile, avverbio in -mente o superlativo di una parola dell'elenco.
func formaNome(w string) bool {
	v := vocabolarioItaliano()
	in := func(s string) bool { return v[s] }
	// avverbi: "facilmente", "rapidamente", "velocemente"
	if r, ok := strings.CutSuffix(w, "mente"); ok && len([]rune(r)) >= 3 {
		if (strings.HasSuffix(r, "a") && (in(r[:len(r)-1]+"o") || in(r))) || in(r+"e") || in(r) {
			return true
		}
	}
	// superlativi: "bellissimo", "velocissime"
	for _, s := range []string{"issimo", "issima", "issimi", "issime"} {
		if r, ok := strings.CutSuffix(w, s); ok && len([]rune(r)) >= 2 {
			if in(r+"o") || in(r+"e") || (strings.HasSuffix(r, "h") && in(r[:len(r)-1]+"o")) {
				return true
			}
		}
	}
	// plurali e femminili. Prudente: "zatteri" no (c'è "zattera", quindi "zattere" è il suo
	// plurale e non un nome in -e)
	n := len(w)
	if n < 3 {
		return false
	}
	r, fine := w[:n-1], w[n-1]
	senzaH := strings.TrimSuffix(r, "h")
	switch fine {
	case 'i':
		if in(r+"o") || in(r+"io") || (r != senzaH && (in(senzaH+"o") || in(senzaH+"a"))) {
			return true // "nemici", "negozi", "lunghi"
		}
		if in(r+"e") && !in(r+"a") {
			return true // "fiori", "grandi"
		}
		// nomi maschili in -a: "problemi", "programmi", "terroristi", "piloti", "poeti"
		if in(r+"a") && (strings.HasSuffix(r, "m") || strings.HasSuffix(r, "ist") || strings.HasSuffix(r, "ot") || strings.HasSuffix(r, "et")) {
			return true
		}
	case 'e':
		// "armi", "amiche", "valigie", "rosse" (da "rosso"), "terroristiche"
		if in(r+"a") || in(r+"o") || (r != senzaH && (in(senzaH+"a") || in(senzaH+"o"))) || in(r+"ia") {
			return true
		}
		if in(r+"i") && !in(r+"o") {
			return true
		}
	case 'a':
		if in(r+"o") || in(r+"e") || in(r+"i") || in(r+"he") {
			return true
		}
	case 'o':
		if in(r+"a") || in(r+"i") || in(r+"hi") {
			return true
		}
	}
	return false
}

// paroleInventateIt: le parole della traduzione italiana it che in italiano non esistono (testo
// inglese en, per riconoscere i nomi e le parole lasciate uguali).
func paroleInventateIt(en, it string) []string {
	if len(vocabolarioItaliano()) == 0 {
		return nil
	}
	inglesi := map[string]bool{}
	var nomiEn []string // parole inglesi con la maiuscola (nomi: "Hadir" → "Hadiro")
	for _, w := range reParola.FindAllString(en, -1) {
		w = strings.TrimSuffix(strings.TrimSuffix(strings.Trim(w, "'"), "'s"), "'S")
		inglesi[strings.ToLower(w)] = true
		if r := []rune(w); len(r) >= 4 && unicode.IsUpper(r[0]) {
			nomiEn = append(nomiEn, strings.ToLower(w))
		}
	}
	tuttoMaiu := tuttoMaiuscolo(it)
	titolo := titoloMaiuscole(en)
	var out []string
	campi := reParolaIt.FindAllStringIndex(it, -1)
	for i, ix := range campi {
		tok := it[ix[0]:ix[1]]
		w := strings.Trim(tok, "'’")
		r := []rune(w)
		if len(r) < 4 || strings.ContainsFunc(w, unicode.IsDigit) {
			continue
		}
		low := strings.ToLower(w)
		if inglesi[low] || inglesi[strings.Split(low, "'")[0]] || inglesi[low[strings.LastIndex(low, "'")+1:]] {
			continue // nome o parola lasciata com'è
		}
		// un nome proprio a metà frase (non nei titoli tutti maiuscoli o con le iniziali maiuscole)
		if unicode.IsUpper(r[0]) && !tuttoMaiu && !titolo {
			prima := strings.TrimSpace(it[:ix[0]])
			if i > 0 && prima != "" && !strings.ContainsAny(prima[len(prima)-1:], ".!?:\"(«-") {
				continue
			}
		}
		if unicode.IsUpper(r[0]) && nomeAdattato(low, nomiEn) {
			continue // nome di persona o di luogo adattato dal traduttore
		}
		if maiuscoleInMezzo(w) {
			continue // sigle e nomi composti ("PlayStation", "XP")
		}
		if parolaItaliana(strings.ToLower(tok)) {
			continue
		}
		if esiste(low) && !strings.ContainsAny(low, "àèéìòù") {
			continue // parola inglese usata in italiano ("sprint", "backup", "boss")
		}
		out = append(out, w)
	}
	return out
}

var reParolaIt = reParola

// nomeAdattato: la parola è un nome inglese con una o due lettere in più o in meno in fondo
// ("Hadir" → "Hadiro") e quel nome non è una parola inglese comune.
func nomeAdattato(w string, nomi []string) bool {
	for _, n := range nomi {
		if esiste(n) {
			continue
		}
		corto, lungo := n, w
		if len(corto) > len(lungo) {
			corto, lungo = lungo, corto
		}
		if len(corto) >= 4 && strings.HasPrefix(lungo, corto) && len(lungo)-len(corto) <= 2 {
			return true
		}
	}
	return false
}

// maiuscoleInMezzo: una maiuscola dopo una minuscola ("PlayStation", "iPhone").
func maiuscoleInMezzo(w string) bool {
	r := []rune(w)
	for i := 1; i < len(r); i++ {
		if unicode.IsUpper(r[i]) && unicode.IsLower(r[i-1]) {
			return true
		}
	}
	return false
}

// titoloMaiuscole: scritta con tutte le parole lunghe con l'iniziale maiuscola ("Shop Owner",
// "Turning Points"): lì la maiuscola non dice che è un nome.
func titoloMaiuscole(s string) bool {
	n := 0
	for _, w := range strings.Fields(s) {
		r := []rune(strings.Trim(w, "\"'()[].,:;!?"))
		if len(r) < 4 {
			continue
		}
		if !unicode.IsUpper(r[0]) {
			return false
		}
		n++
	}
	return n >= 1
}

// traduzioneInventata: la traduzione italiana ha parole che non esistono e non va mostrata.
// Una parola sconosciuta "pesa" se: è quasi una parola vera (una lettera sbagliata, in più, in
// meno o scambiata: "Propriettario", "Indietto", "urlente", "zatteri"), finisce con una
// consonante o ha lettere che l'italiano non usa ("Impactvrh", "etürn"), oppure la scritta è
// corta (titolo, voce di menu: lì la parola inventata è metà del senso: "PUNCI DI
// SVOGLIAMENTO"). Una parola rara ma ben fatta in una frase lunga non basta (l'elenco delle parole
// non le ha tutte): servono due parole sconosciute. Restituisce le parole trovate.
func traduzioneInventata(en, it string) (bool, []string) {
	sospette := paroleInventateIt(en, it)
	if len(sospette) == 0 {
		return false, nil
	}
	if len(sospette) >= 2 {
		return true, sospette
	}
	corta := len(strings.Fields(it)) <= 4
	for _, w := range sospette {
		if corta || parolaQuasiVera(w) || formaNonItaliana(w) {
			return true, sospette
		}
	}
	return false, sospette
}

// formaNonItaliana: finisce con una consonante o ha lettere che le parole italiane non hanno.
func formaNonItaliana(w string) bool {
	r := []rune(strings.ToLower(w))
	if len(r) == 0 {
		return false
	}
	if !strings.ContainsRune("aeiouàèéìòù", r[len(r)-1]) {
		return true
	}
	return strings.ContainsAny(string(r), "kwxyjäëïöüñçøå")
}

// parolaQuasiVera: cambiando una lettera prima dell'ultima (o togliendola, aggiungendola,
// scambiando due vicine) viene una parola italiana.
func parolaQuasiVera(w string) bool {
	r := []rune(strings.ToLower(w))
	if len(r) < 5 {
		return false
	}
	lettere := []rune("abcdefghilmnopqrstuvzàèéìòù")
	prova := func(x []rune) bool { return len(x) >= 4 && parolaItaliana(string(x)) }
	// le lettere in fondo no: "interrogatori" è solo il plurale di "interrogatorio"
	for i := range r[:len(r)-1] {
		// tolta
		if prova(append(append([]rune{}, r[:i]...), r[i+1:]...)) {
			return true
		}
		// scambiate
		if i+1 < len(r) {
			x := append([]rune{}, r...)
			x[i], x[i+1] = x[i+1], x[i]
			if prova(x) {
				return true
			}
		}
		for _, c := range lettere {
			if c != r[i] { // cambiata
				x := append([]rune{}, r...)
				x[i] = c
				if prova(x) {
					return true
				}
			}
			// aggiunta
			if prova(append(append(append([]rune{}, r[:i]...), c), r[i:]...)) {
				return true
			}
		}
	}
	return false
}
