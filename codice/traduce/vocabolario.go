package traduce

// Vocabolario inglese (SCOWL, Kevin Atkinson, licenza permissiva: vedi dati/parole-inglesi-LICENZA.txt)
// usato per correggere le parole lette male dall'OCR: "hai led" -> "hailed", "Waitinq" -> "Waiting".
// Si corregge solo quando la parola letta NON esiste e una sola correzione esiste davvero.

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

//go:embed dati/parole-inglesi.txt.gz
var paroleInglesiGz []byte

var (
	vocOnce sync.Once
	vocLiv  map[string]int8 // parola minuscola -> livello di frequenza (10 = comunissima, 60 = rara)
)

func vocabolario() map[string]int8 {
	vocOnce.Do(func() {
		vocLiv = make(map[string]int8, 80000)
		zr, err := gzip.NewReader(bytes.NewReader(paroleInglesiGz))
		if err != nil {
			return
		}
		sc := bufio.NewScanner(zr)
		for sc.Scan() {
			p, l, ok := strings.Cut(sc.Text(), "\t")
			if !ok {
				continue
			}
			n, _ := strconv.Atoi(l)
			vocLiv[p] = int8(n)
		}
	})
	return vocLiv
}

// esiste: la parola (anche con 's, n't, 're... o al plurale/possessivo) è inglese?
func esiste(p string) bool {
	v := vocabolario()
	p = strings.ToLower(strings.Trim(p, "'"))
	if p == "" {
		return true
	}
	if _, ok := v[p]; ok {
		return true
	}
	for _, s := range []string{"'s", "'re", "'ve", "'ll", "'d", "'m", "n't", "s'"} {
		if strings.HasSuffix(p, s) {
			if _, ok := v[strings.TrimSuffix(p, s)]; ok {
				return true
			}
		}
	}
	return false
}

// confusioni tipiche dell'OCR (cosa ha letto -> cosa poteva essere)
var confusioni = [][2]string{
	{"q", "g"}, {"g", "q"}, {"rn", "m"}, {"m", "rn"}, {"cl", "d"}, {"vv", "w"}, {"ii", "u"},
	{"l", "i"}, {"i", "l"}, {"I", "l"}, {"1", "l"}, {"0", "o"}, {"5", "s"}, {"8", "B"}, {"c", "e"}, {"e", "c"},
	{"n", "h"}, {"h", "n"}, {"u", "n"}, {"C", "G"}, {"O", "G"}, {"G", "C"}, {"t", "f"}, {"f", "t"}, {"y", "v"}, {"v", "y"}, {"é", "e"}, {"è", "e"}, {"ë", "e"}, {"à", "a"}, {"ü", "u"}, {"ï", "i"},
}

var reParola = regexp.MustCompile(`[A-Za-z0-9À-ÿ][A-Za-z0-9À-ÿ']*`)

// candidati: tutte le parole vere ottenibili con UNA confusione tipica.
func candidati(p string) []string {
	visti := map[string]bool{}
	var out []string
	for _, c := range confusioni {
		for i := 0; ; {
			j := strings.Index(p[i:], c[0])
			if j < 0 {
				break
			}
			k := i + j
			n := p[:k] + c[1] + p[k+len(c[0]):]
			if !visti[n] && esiste(n) {
				visti[n] = true
				out = append(out, n)
			}
			i = k + 1
		}
	}
	return out
}

func soloLettere(p string) bool {
	for _, c := range p {
		if !unicode.IsLetter(c) && c != '\'' {
			return false
		}
	}
	return true
}

// correggiParole ripara le parole che non esistono in inglese. I nomi propri (maiuscola a metà
// frase) e le sigle restano come sono.
func correggiParole(r string) string {
	// la "J" di "Join" spesso si perde sopra uno sfondo disegnato: "in Multiplayer Game"
	if m := reJoinTagliato.FindStringSubmatchIndex(r); m != nil {
		r = "Join" + r[m[2]:]
	}
	if vocabolario() == nil || len(vocLiv) == 0 {
		return r
	}
	idx := reParola.FindAllStringIndex(r, -1)
	if len(idx) == 0 {
		return r
	}
	var b strings.Builder
	ultimo := 0
	for n := 0; n < len(idx); n++ {
		a, z := idx[n][0], idx[n][1]
		p := r[a:z]
		inizioFrase := n == 0 || strings.ContainsAny(strings.TrimSpace(r[idx[n-1][1]:a]), ".!?:")
		b.WriteString(r[ultimo:a])
		ultimo = z
		// 1) parola spezzata in due: "hai led" -> "hailed"
		if n+1 < len(idx) && r[z:idx[n+1][0]] == " " {
			q := r[idx[n+1][0]:idx[n+1][1]]
			unita := p + q
			if soloLettere(unita) && isLowerOrCap(p) && isLower(q) && esiste(unita) && (!esiste(p) || !esiste(q) || comuneSpezzata(p, q, unita)) {
				b.WriteString(unita)
				ultimo = idx[n+1][1]
				n++
				continue
			}
		}
		// 0a) accento spuntato dalla lettura italiana su una parola inglese: "Codë" -> "Code"
		if q := senzaAccenti(p); q != p && !esiste(p) && esiste(q) {
			b.WriteString(q)
			continue
		}
		// 0b) due parole lette attaccate: "MultiplayerGame" -> "Multiplayer Game"
		if q := staccaMaiuscola(p); q != "" {
			b.WriteString(q)
			continue
		}
		if esiste(p) || len(p) < 3 || maiuscole(p) || (!inizioFrase && unicode.IsUpper(rune(p[0])) && soloLettere(p)) {
			b.WriteString(p)
			continue
		}
		// 2) prima lettera sottile persa (J, I): "Oin Multiplayer" -> "Join Multiplayer"
		if q := lettereIniziali(p); q != "" {
			b.WriteString(q)
			continue
		}
		// 3) una lettera confusa: "Waitinq" -> "Waiting"
		if c := candidati(p); len(c) == 1 {
			b.WriteString(c[0])
			continue
		}
		// 4) lettere perse o sbagliate nelle scritte piccole: "remining" -> "remaining"
		if q := parolaVicina(p); q != "" {
			b.WriteString(q)
			continue
		}
		b.WriteString(p)
	}
	b.WriteString(r[ultimo:])
	return b.String()
}

// due parole vere che unite ne fanno una comune e molto più probabile ("hai led" no, "with out" no)
func comuneSpezzata(p, q, unita string) bool {
	v := vocabolario()
	lu, ok := v[strings.ToLower(unita)]
	if !ok || lu > 35 {
		return false
	}
	lp, lq := v[strings.ToLower(p)], v[strings.ToLower(q)]
	return lp > 40 || lq > 40 // almeno un pezzo è una parola rara (spesso un errore di lettura)
}

func isLower(s string) bool {
	for _, c := range s {
		if unicode.IsUpper(c) {
			return false
		}
	}
	return true
}

func isLowerOrCap(s string) bool { return len(s) > 0 && isLower(s[1:]) }

func maiuscole(s string) bool {
	n := 0
	for _, c := range s {
		if unicode.IsUpper(c) {
			n++
		}
	}
	return n >= 2
}

// lettereIniziali: la lettura di Windows a volte perde una J o una I all'inizio della parola
// (lettere strette, spesso decorate nei caratteri dei giochi). Solo se la parola letta non esiste
// e una sola delle due la rende una parola vera.
func lettereIniziali(p string) string {
	if !soloLettere(p) || !unicode.IsUpper(rune(p[0])) {
		return ""
	}
	resto := strings.ToLower(p)
	var trovate []string
	for _, l := range []string{"J", "I"} {
		if esiste(l + resto) {
			trovate = append(trovate, l+resto)
		}
	}
	if len(trovate) == 1 {
		return trovate[0]
	}
	return ""
}

// senzaAccenti: toglie gli accenti dalle lettere latine (solo per confrontare con l'inglese).
func senzaAccenti(p string) string {
	var b strings.Builder
	for _, c := range p {
		if c < 0xC0 || c > 0xFF {
			b.WriteRune(c)
			continue
		}
		switch {
		case strings.ContainsRune("àáâãäå", c):
			c = 'a'
		case strings.ContainsRune("ÀÁÂÃÄÅ", c):
			c = 'A'
		case strings.ContainsRune("èéêë", c):
			c = 'e'
		case strings.ContainsRune("ÈÉÊË", c):
			c = 'E'
		case strings.ContainsRune("ìíîï", c):
			c = 'i'
		case strings.ContainsRune("ÌÍÎÏ", c):
			c = 'I'
		case strings.ContainsRune("òóôõö", c):
			c = 'o'
		case strings.ContainsRune("ÒÓÔÕÖ", c):
			c = 'O'
		case strings.ContainsRune("ùúûü", c):
			c = 'u'
		case strings.ContainsRune("ÙÚÛÜ", c):
			c = 'U'
		}
		b.WriteRune(c)
	}
	return b.String()
}

// marchiAttaccati: nomi scritti apposta con due maiuscole, da non staccare.
var marchiAttaccati = map[string]bool{"playstation": true, "powerpoint": true,
	"xbox": true, "teamspeak": true, "battlenet": true, "gamepass": true, "steamworks": true,
	"onedrive": true, "whatsapp": true, "facetime": true, "blackjack": true, "deathmatch": true,
	"firestorm": true, "nightmare": true}

// staccaMaiuscola: "MultiplayerGame" non esiste, "Multiplayer" e "Game" sì: le stacco. Solo se
// le due parti sono lunghe (4+ lettere) e la parola intera non è un nome scritto così apposta.
func staccaMaiuscola(p string) string {
	if !soloLettere(p) || esiste(p) || marchiAttaccati[strings.ToLower(p)] {
		return ""
	}
	r := []rune(p)
	for i := 4; i <= len(r)-4; i++ {
		if unicode.IsLower(r[i-1]) && unicode.IsUpper(r[i]) {
			a, z := string(r[:i]), string(r[i:])
			if esiste(a) && esiste(z) && soloLettere(z) {
				if rest := staccaMaiuscola(z); rest != "" {
					z = rest
				}
				return a + " " + z
			}
		}
	}
	return ""
}

var reJoinTagliato = regexp.MustCompile(`^(?:[JjIl1|]?[Oo0]?in|Oin)( (?:Multiplayer|Game|Server|Online|Party|Session|Lobby|a Game)\b)`)

// parolaVicina: la parola vera più comune a una o due lettere di distanza ("remining" ->
// "remaining", "playes" -> "players"). Solo se ce n'è una chiaramente più comune delle altre.
func parolaVicina(p string) string {
	v := vocabolario()
	if len(v) == 0 || len(p) < 4 || !soloLettere(p) {
		return ""
	}
	low := strings.ToLower(p)
	scegli := func(cand map[string]bool) string {
		best, bl, pari := "", int8(127), false
		for c := range cand {
			l, ok := v[c]
			if !ok {
				continue
			}
			switch {
			case l < bl:
				best, bl, pari = c, l, false
			case l == bl:
				pari = true
			}
		}
		if best == "" || pari || bl > 45 {
			return ""
		}
		return best
	}
	d1 := modifiche(low)
	w := scegli(d1)
	if w == "" && len(low) >= 6 {
		d2 := map[string]bool{}
		for c := range d1 {
			for c2 := range modifiche(c) {
				d2[c2] = true
			}
		}
		w = scegli(d2)
	}
	if w == "" {
		return ""
	}
	if unicode.IsUpper(rune(p[0])) {
		w = strings.ToUpper(w[:1]) + w[1:]
	}
	return w
}

func modifiche(p string) map[string]bool {
	const lettere = "abcdefghijklmnopqrstuvwxyz"
	out := map[string]bool{}
	for i := 0; i <= len(p); i++ {
		a, b := p[:i], p[i:]
		if len(b) > 0 {
			out[a+b[1:]] = true // tolta
			for _, c := range lettere {
				out[a+string(c)+b[1:]] = true // cambiata
			}
		}
		if len(b) > 1 {
			out[a+string(b[1])+string(b[0])+b[2:]] = true // scambiate
		}
		for _, c := range lettere {
			out[a+string(c)+b] = true // aggiunta
		}
	}
	return out
}
