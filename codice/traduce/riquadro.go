package traduce

// Suggerimenti dei tasti dentro un riquadro piccolo (26/11, PC Cafe Simulator, tutorial):
// "Hold" sopra il tasto T diventava "Tieni premuto", usciva dal riquadro del pulsante e si
// vedeva scritto male. Regola per le etichette dei comandi:
//   1. la traduzione deve stare nel riquadro dell'inglese (o fino al bordo vero del pulsante, se
//      l'ho trovato), mai sopra l'icona del tasto;
//   2. prima si rimpicciolisce il carattere, fino alla grandezza più piccola che si legge ancora
//      (la stessa di impagina: 62%, mai sotto 9 pixel);
//   3. se non basta, una frase pronta più corta: "Tieni premuto" → "Tieni", "Premere" → "Premi",
//      "Comprimi" → "Riduci"; i nomi dei tasti restano come sul tasto (Shift, Ctrl, T);
//   4. se nemmeno così ci sta, resta l'inglese del gioco (meglio dell'italiano che esce fuori).
//
// Qui la larghezza si stima dalle lettere: l'inglese occupa b.W pixel con le sue lettere, la
// traduzione con lo stesso carattere occupa in proporzione (ogni lettera ha la sua larghezza
// tipica: "i" stretta, "m" larga). Su Windows impagina misura poi davvero, con lo stesso limite.

import (
	"regexp"
	"strings"
	"sync"
	"unicode"
)

// motivoNonCiSta: la traduzione dell'etichetta non sta nel suo riquadro nemmeno corta.
const motivoNonCiSta = "non ci sta nel riquadro del comando (resta l'inglese)"

// reParolaComando: le parole dei suggerimenti dei tasti.
var reParolaComando = regexp.MustCompile(`(?i)\b(hold|holding|press|tap|release|skip|collapse|expand|toggle)\b`)

// reNomeTasto: un nome di tasto da solo ("T", "Shift", "Ctrl", "LMB").
// (non "A" e "I" da sole, che sono anche parole: "A New Game")
var reNomeTasto = regexp.MustCompile(`^([B-HJ-Z]|F[0-9]{1,2}|Space|Spacebar|Enter|Esc|Tab|Shift|Ctrl|Control|Alt|LMB|RMB|Backspace)$`)

// etichettaComando: una scritta corta su una riga che è il suggerimento di un tasto: ha
// l'icona del tasto accanto, o una parola dei comandi ("Hold", "Press", "Skip"), o il nome di
// un tasto ("Hold T Skip", "Shift Sprint").
func etichettaComando(b Blocco) bool {
	if max(1, b.Righe) != 1 {
		return false
	}
	parole := strings.Fields(b.Testo)
	if len(parole) == 0 || len(parole) > 4 {
		return false
	}
	if reParolaComando.MatchString(b.Testo) {
		return true
	}
	// l'icona accanto basta per le etichette corte ("Sprint", "Open Map", "Pick up item");
	// quattro parole con un'icona sono un obiettivo o una voce ("FIGHT YOUR WAY OUT" col segno
	// della missione), non il nome di un tasto: restavano in inglese a fotogrammi alterni
	if (b.IconaDx > b.X || b.IconaSx > 0) && len(parole) <= 3 {
		return true
	}
	if len(parole) >= 2 {
		for _, p := range parole {
			if reNomeTasto.MatchString(strings.Trim(p, "[]()<>:")) {
				return true
			}
		}
	}
	return false
}

// limiteComando: quanto può essere larga al massimo la traduzione di un'etichetta dei comandi.
// Con il bordo vero del pulsante trovato, fino a lì; se no il riquadro dell'inglese e poco più
// (un decimo): non si va né sopra il bordo che non vedo né sopra l'icona del tasto.
func limiteComando(b Blocco) int {
	if b.Bordo {
		return largConsentita(b)
	}
	return min(largConsentita(b), b.W+max(3, b.W/10))
}

// scalaMinimaComando: la grandezza più piccola che si legge ancora, rispetto a quella del gioco.
func scalaMinimaComando(b Blocco) float64 {
	px := b.Em
	if px <= 0 {
		px = float64(b.H) * 1.15
	}
	if px <= 0 {
		return 1
	}
	return min(1, max(9, px*0.62)/px)
}

// larghezzaLettere: larghezza tipica del testo in "em" (carattere senza grazie dei giochi).
func larghezzaLettere(s string) float64 {
	tot := 0.0
	for _, r := range s {
		switch {
		case r == ' ':
			tot += 0.28
		case strings.ContainsRune("iljI.,:;'!|ìí", r):
			tot += 0.26
		case strings.ContainsRune("frt()[]-/", r):
			tot += 0.36
		case strings.ContainsRune("mwMW", r):
			tot += 0.84
		case unicode.IsUpper(r):
			tot += 0.66
		case unicode.IsDigit(r):
			tot += 0.55
		default:
			tot += 0.53
		}
	}
	return tot
}

// ciStaNelRiquadro: la traduzione t, alla grandezza più piccola ammessa, non supera il limite.
func ciStaNelRiquadro(b Blocco, t string) bool {
	ue := larghezzaLettere(b.Testo)
	if ue <= 0 || b.W <= 0 {
		return true
	}
	perEm := float64(b.W) / ue
	return larghezzaLettere(t)*perEm*scalaMinimaComando(b) <= float64(limiteComando(b))
}

// versioniCorte: le forme corte dei comandi, dalla più lunga. I nomi dei tasti non si toccano.
var versioniCorte = []struct{ lunga, corta string }{
	{"Tieni premuto", "Tieni"},
	{"Tenere premuto", "Tieni"},
	{"tenendo premuto", "tenendo"},
	{"Premere", "Premi"},
	{"Comprimi", "Riduci"},
	{"Attiva/disattiva", "Alterna"},
	{"Interagisci", "Usa"},
	{"Fai clic", "Clic"},
	{"Accovacciati", "Abbassati"},
}

// versioneCorta: la traduzione con i comandi nella forma corta (stesse maiuscole).
func versioneCorta(it string) (string, bool) {
	out := it
	maiuscolo := tuttoMaiuscolo(it)
	for _, v := range versioniCorte {
		l, c := v.lunga, v.corta
		if maiuscolo {
			l, c = strings.ToUpper(l), strings.ToUpper(c)
		}
		out = sostituisciComando(out, l, c)
		if !maiuscolo { // anche con la minuscola iniziale ("premere per…")
			out = sostituisciComando(out, strings.ToLower(l[:1])+l[1:], strings.ToLower(c[:1])+c[1:])
		}
	}
	return out, out != it
}

// sostituisciParola: sostituisce l con c solo come parole intere.
func sostituisciComando(s, l, c string) string {
	re := regexpComando(l)
	return re.ReplaceAllString(s, "${1}"+strings.ReplaceAll(c, "$", "$$")+"${2}")
}

// adattaAlRiquadro: per le etichette dei comandi sceglie la traduzione che sta nel riquadro
// (quella intera, se rimpicciolita ci sta; se no quella corta). Se nessuna ci sta restituisce
// il motivo: la scritta resta in inglese.
func adattaAlRiquadro(b *Blocco) string {
	if !etichettaComando(*b) || strings.TrimSpace(b.Tradotto) == "" {
		return ""
	}
	if ciStaNelRiquadro(*b, b.Tradotto) {
		return ""
	}
	if c, ok := versioneCorta(b.Tradotto); ok && ciStaNelRiquadro(*b, c) {
		b.Tradotto = c
		return ""
	}
	return motivoNonCiSta
}

// regexpComando: l'espressione per la parola l, preparata una volta sola (prima si ricompilava a
// ogni scritta di ogni fotogramma).
var cacheRegexpComando sync.Map // string -> *regexp.Regexp

func regexpComando(l string) *regexp.Regexp {
	if v, ok := cacheRegexpComando.Load(l); ok {
		return v.(*regexp.Regexp)
	}
	re := regexp.MustCompile(`(^|[^\p{L}])` + regexp.QuoteMeta(l) + `($|[^\p{L}])`)
	cacheRegexpComando.Store(l, re)
	return re
}
