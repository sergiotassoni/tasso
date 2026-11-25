package traduce

// Ottavo giro sui video veri (r5: gli stessi 9 giochi). Regole universali:
//   - frasi con l'icona di un tasto in mezzo ("Tap [Q] to quickly throw the item. Hold [Q] to
//     aim.", "…to aim. Hold [E] while aiming / for a powerful shot."): Windows divide la riga
//     attorno all'icona e il pezzo dopo l'ultima icona si traduceva da solo, accanto
//     all'inglese. Un pezzo che finisce con un verbo del tasto ("…Hold", "or hold") vuole il
//     seguito: i pezzi si uniscono e la frase si traduce intera (le lettere dei tasti restano
//     nel testo); se non si uniscono, il controllo finale li vede come pezzi della stessa frase
//     (anche dentro un paragrafo di più righe) e restano tutti in inglese;
//   - il tasto disegnato come freccia e letto come una lettera minuscola ("Press t to sprint."):
//     è l'icona, non una parola: si toglie e la scritta si divide attorno all'icona;
//   - "/ Back" (il segno dell'icona staccato dalla voce): il segno resta del gioco;
//   - nomi di posti con un codice ("1S4V1 POINT", "SECTOR 7G"): restano in inglese;
//   - parole impossibili lette al posto di altre ("rm son'" era "I'm sorry"): lettura incerta.

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// verbiTastoFrase: i verbi che nelle istruzioni vogliono dopo di sé l'icona o il nome del tasto
// ("use" no: "Ready to use" finisce davvero lì).
var verbiTastoFrase = map[string]bool{"press": true, "hold": true, "tap": true, "click": true, "push": true}

// comandoAperto: il pezzo finisce con un verbo del tasto che apre un comando ("Press", "…the
// item. Hold", "…or hold", "…to aim Hold"): dopo viene l'icona del tasto e il seguito della frase.
// Non conta un'etichetta con tutte le iniziali maiuscole ("Long Press").
func comandoAperto(t string) bool {
	w := strings.Fields(strings.TrimSpace(t))
	if len(w) == 0 {
		return false
	}
	u := w[len(w)-1]
	if !verbiTastoFrase[strings.ToLower(u)] {
		return false
	}
	if len(w) == 1 {
		return true
	}
	prima := w[len(w)-2]
	if strings.ContainsAny(prima[len(prima)-1:], ".,;:!?") {
		return true
	}
	switch strings.ToLower(strings.Trim(prima, "…\"'")) {
	case "or", "and", "then", "to", "and/or":
		return true
	}
	if !unicode.IsUpper([]rune(u)[0]) {
		return false
	}
	// "…to aim Hold": il verbo con la maiuscola in mezzo a una frase scritta minuscola apre la
	// frase dopo (il punto l'OCR l'ha perso); in un'etichetta "Long Press" no.
	for _, p := range w[:len(w)-1] {
		if r := []rune(p); unicode.IsLetter(r[0]) && unicode.IsLower(r[0]) {
			return true
		}
	}
	return false
}

// nomiTastiCorti: nomi di tasti scritti nelle icone ("ESC", "LMB").
var nomiTastiCorti = map[string]bool{"ESC": true, "CTRL": true, "LMB": true, "RMB": true, "MMB": true,
	"LB": true, "RB": true, "LT": true, "RT": true, "SHIFT": true, "SPACE": true, "TAB": true, "ALT": true,
	"ENTER": true, "F": true}

// inizioTasto: il pezzo comincia con il tasto letto dall'icona: una lettera maiuscola o una cifra
// da sola ("Q to aim.", "E while aiming"), o il nome di un tasto ("Shift to sprint").
func inizioTasto(t string) bool {
	w := strings.Fields(strings.TrimSpace(t))
	if len(w) < 2 {
		return false
	}
	k := strings.Trim(w[0], "[]()<>")
	r := []rune(k)
	if len(r) == 1 && (unicode.IsUpper(r[0]) || unicode.IsDigit(r[0])) {
		return true
	}
	u := strings.ToUpper(k)
	if nomiTastiCorti[u] || nomiTastiParole[u] {
		return true
	}
	return len(r) >= 2 && len(r) <= 3 && r[0] == 'F' && strings.Trim(string(r[1:]), "0123456789") == ""
}

// seguitoDelComando: c (a destra di a sulla stessa linea) è il seguito del comando aperto in a:
// "…Hold" | "Q to aim." / "E while aiming" / "to aim.".
func seguitoDelComando(a, c string) bool {
	return comandoAperto(a) && (inizioTasto(c) || iniziaMinuscola(c))
}

// righeDelBlocco: le righe di una scritta di più righe (paragrafo), ognuna con il suo rettangolo e
// il suo testo, dalle posizioni delle parole. Una scritta di una riga torna com'è; senza le
// posizioni, le righe divise in altezza con il testo intero.
func righeDelBlocco(b Blocco) []Blocco {
	if righeSchermo(b) == 1 {
		return []Blocco{b}
	}
	p := append([]ParolaOCR(nil), b.parole()...)
	if len(p) == 0 {
		var out []Blocco
		h := b.H / righeSchermo(b)
		for i := 0; i < righeSchermo(b); i++ {
			r := b
			r.Righe, r.Y, r.H, r.Pos = 1, b.Y+i*h, h, nil
			out = append(out, r)
		}
		return out
	}
	sort.SliceStable(p, func(i, j int) bool { return p[i].Y+p[i].H/2 < p[j].Y+p[j].H/2 })
	var gruppi [][]ParolaOCR
	for _, w := range p {
		c := w.Y + w.H/2
		if n := len(gruppi); n > 0 {
			u := gruppi[n-1]
			hm, cm := 0.0, 0.0
			for _, x := range u {
				hm += x.H
				cm += x.Y + x.H/2
			}
			hm, cm = hm/float64(len(u)), cm/float64(len(u))
			if math.Abs(c-cm) < 0.5*math.Max(hm, w.H) {
				gruppi[n-1] = append(u, w)
				continue
			}
		}
		gruppi = append(gruppi, []ParolaOCR{w})
	}
	var out []Blocco
	for _, g := range gruppi {
		sort.SliceStable(g, func(i, j int) bool { return g[i].X < g[j].X })
		r := bloccoDaPosizioni(g, b)
		if b.Em > 0 && r.Em <= 0 {
			r.Em = b.Em
		}
		out = append(out, r)
	}
	return out
}

// pezziStessaLinea: sx e dx (dx a destra), due scritte di una riga sulla stessa linea, sono pezzi
// della stessa frase: vicini (fino a 4 em di buco: lì ci sta l'icona di un tasto) e il primo vuole
// il seguito, o il secondo comincia minuscolo, o il secondo è il seguito di un comando ("Hold" |
// "Q to aim."). Vale anche quando le altezze dei riquadri sono molto diverse ("lost..." senza
// gambe accanto a "…the only one we").
func pezziStessaLinea(sx, dx Blocco) bool {
	if !lineaSimile(sx, dx) {
		return false
	}
	em := math.Max(sx.Em, dx.Em)
	if em <= 0 {
		em = float64(max(sx.H, dx.H)) * 1.1
	}
	buco := float64(dx.X - (sx.X + sx.W))
	if buco < -float64(max(sx.H, dx.H)) || buco > 4*em {
		return false
	}
	if seguitoDelComando(sx.Testo, dx.Testo) {
		return true
	}
	if buco > 3*em {
		return false
	}
	return chiedeSeguito(sx.Testo) || iniziaMinuscola(dx.Testo) || pezzoTroncato(sx.Testo, dx.Testo)
}

// ---------------------------------------------------------------- icona del tasto letta come lettera

// lettereNonParola: lettere minuscole da sole che in inglese sono parole ("a", "i" letta male).
var lettereNonParola = map[string]bool{"a": true, "i": true}

// iconaInMezzo: la posizione della parola che è l'icona di un tasto letta come una lettera
// minuscola o un segno tra il verbo e il seguito ("Press t to sprint.", "Hold • to aim"); -1 se
// non c'è.
func iconaInMezzo(campi []string) int {
	for i := 1; i+1 < len(campi); i++ {
		if !verbiTastoFrase[strings.ToLower(strings.Trim(campi[i-1], ".,:;"))] {
			continue
		}
		k := campi[i]
		r := []rune(k)
		icona := segnoIcona(k) || (len(r) == 1 && unicode.IsLetter(r[0]) && unicode.IsLower(r[0]) && !lettereNonParola[k])
		if icona && iniziaMinuscola(strings.Join(campi[i+1:], " ")) {
			return i
		}
	}
	return -1
}

// spezzaIconeInMezzo: "Press t to sprint." → "Press" e "to sprint.": la lettera è la freccia
// dell'icona del tasto, che resta del gioco (non si traduce, non si copre). Con le posizioni
// delle parole la scritta si divide attorno all'icona; senza, la lettera si toglie dal testo.
func spezzaIconeInMezzo(b []Blocco) []Blocco {
	var out []Blocco
	for _, bl := range b {
		if righeSchermo(bl) != 1 {
			out = append(out, bl)
			continue
		}
		campi := strings.Fields(bl.Testo)
		i := iconaInMezzo(campi)
		if i < 0 {
			out = append(out, bl)
			continue
		}
		p := bl.parole()
		if len(p) == len(campi) && strings.TrimSpace(p[i].T) == campi[i] {
			sx, dx := bloccoDaPosizioni(p[:i], bl), bloccoDaPosizioni(p[i+1:], bl)
			sx.Testo, dx.Testo = strings.Join(campi[:i], " "), strings.Join(campi[i+1:], " ")
			sx.Affid, dx.Affid = bl.Affid, bl.Affid
			sx.Mossa, dx.Mossa = bl.Mossa, bl.Mossa
			sx.IconaDx = int(p[i].X)
			dx.IconaSx = int(p[i].X + p[i].W)
			out = append(out, sx, dx)
			continue
		}
		bl.Testo = strings.Join(append(append([]string(nil), campi[:i]...), campi[i+1:]...), " ")
		out = append(out, bl)
	}
	return out
}

// ---------------------------------------------------------------- nomi con un codice

// nomeConCodice: etichetta corta tutta maiuscola (2-3 parole) con un codice di lettere e cifre
// mescolate ("1S4V1 POINT", "SECTOR 7G", "B2 HANGAR"): è il nome di un posto, resta com'è.
func nomeConCodice(t string) bool {
	w := strings.Fields(strings.TrimSpace(t))
	if len(w) < 2 || len(w) > 3 || !tuttoMaiuscolo(t) {
		return false
	}
	codice := false
	for _, p := range w {
		lettere, cifre := 0, 0
		for _, c := range p {
			switch {
			case unicode.IsLetter(c):
				lettere++
			case unicode.IsDigit(c):
				cifre++
			case strings.ContainsRune("-/.'", c):
			default:
				return false
			}
		}
		if lettere > 0 && cifre > 0 {
			codice = true
		}
	}
	return codice
}

// ---------------------------------------------------------------- parole impossibili

// interiezioniSenzaVocali: parole vere senza vocali.
var interiezioniSenzaVocali = map[string]bool{"hm": true, "hmm": true, "mm": true, "mmm": true, "sh": true,
	"shh": true, "psst": true, "pst": true, "brr": true, "grr": true, "zz": true, "zzz": true, "tsk": true,
	"nth": true, "st": true, "nd": true, "rd": true, "th": true, "pfft": true, "hrm": true, "cwm": true,
	// abbreviazioni scritte minuscole nei giochi e nei dialoghi
	"mr": true, "mrs": true, "dr": true, "hp": true, "xp": true, "sp": true, "mp": true, "km": true, "kg": true,
	"cm": true, "ml": true, "mph": true, "kph": true, "fps": true, "lvl": true, "pts": true, "dmg": true,
	"ft": true, "lb": true, "lbs": true, "hr": true, "hrs": true, "ms": true, "px": true, "dps": true,
	"rpm": true, "gg": true, "wp": true, "np": true, "pls": true, "plz": true, "thx": true, "brb": true,
	"bc": true, "rn": true, "tbh": true, "smh": true, "nvm": true, "ty": true, "lmk": true, "pvp": true, "pve": true}

// parolaSenzaVocali: nella scritta c'è una parola staccata di 2-4 lettere minuscole senza vocali
// che non è un'interiezione ("rm" letto al posto di "I'm sorry"): la lettura è sbagliata.
func parolaSenzaVocali(s string) bool {
	for _, w := range strings.Fields(s) {
		p := strings.Trim(w, ".,;:!?\"'()…-")
		r := []rune(p)
		if len(r) < 2 || len(r) > 4 || interiezioniSenzaVocali[p] {
			continue
		}
		tutte := true
		for _, c := range r {
			if !unicode.IsLower(c) || strings.ContainsRune("aeiouyàèéìòù", c) {
				tutte = false
				break
			}
		}
		if tutte {
			return true
		}
	}
	return false
}
