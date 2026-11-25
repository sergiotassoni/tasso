package traduce

// Correzione e controllo delle righe lette dall'OCR sullo schermo, per TUTTI i giochi.
//
// Prima di tradurre una scritta mi chiedo: l'ho letta bene? Le scritte dei giochi hanno caratteri
// strani (lettere strette, spaziate, con effetti di colore): l'OCR a volte legge "Curry thingthing
// to fill the progress bur" invece di "Carry that thing to fill the progress bar", "CO NTROL"
// invece di "CONTROL", "UBIIC GAME" invece di "PUBLIC GAME". Tradurre quella spazzatura è peggio
// che lasciare l'inglese. Qui:
//
//  1. unisco i pezzi di una parola spaziata ("CO NTROL" → "CONTROL", "P U B L I C" → "PUBLIC");
//  2. correggo le parole che non esistono con il vocabolario inglese generale (già in correggiParole);
//  3. aggancio la riga a una frase pronta del catalogo se è quasi uguale (pochi errori, e solo
//     se la lettura ha parole sbagliate o rare: "bur" → "bar");
//  4. do un voto di affidabilità: 2 = frase del catalogo, 1 = tutte parole vere, 0 = non tradurre.
//
// Niente regole per un gioco solo: vocabolario inglese, glossario da gioco e frasi pronte.

import (
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// livelli di affidabilità di una riga letta
const (
	letturaDubbia  = 0 // parole inventate o storpiate: meglio lasciare l'inglese del gioco
	letturaBuona   = 1 // tutte parole vere (o nomi): si traduce
	letturaCatalog = 2 // è (o è stata agganciata a) una frase pronta del catalogo
)

// nomiTastiFissi: nomi di tasti e del mouse che restano come sono, in qualsiasi riga.
var nomiTastiFissi = map[string]bool{"WASD": true, "LMB": true, "RMB": true, "MMB": true, "CTRL": true,
	"ESC": true, "LSHIFT": true, "RSHIFT": true, "LCTRL": true, "RCTRL": true, "LALT": true, "RALT": true}

// nomiTastiParole: nomi di tasti che sono anche parole inglesi: tasti solo in una riga non tutta
// maiuscola ("Press SPACE to jump") o dopo press/hold/tap/use/key.
var nomiTastiParole = map[string]bool{"CONTROL": true, "SHIFT": true, "ALT": true, "TAB": true, "SPACE": true,
	"SPACEBAR": true, "ENTER": true, "RETURN": true, "ESCAPE": true, "BACKSPACE": true, "DELETE": true,
	"INSERT": true, "HOME": true, "END": true, "CAPS": true}

// valida: la parola esiste in inglese o è una parola dei giochi/un nome di tasto.
func parolaValidaOCR(w string) bool {
	if w == "" {
		return true
	}
	if esiste(w) || paroleGioco[strings.ToLower(w)] || termGiochi[w] != "" || termGiochi[strings.ToLower(w)] != "" {
		return true
	}
	u := strings.ToUpper(w)
	if nomiTastiFissi[u] || nomiTastiParole[u] {
		return true
	}
	return false
}

// parolaRara: parola vera ma rarissima (livello 50 o più nel vocabolario: "bur", "tor", "nae").
// Nelle scritte dei giochi è spesso una lettura sbagliata di una parola comune.
func parolaRara(w string) bool {
	v := vocabolario()
	l, ok := v[strings.ToLower(w)]
	return ok && l >= 50 && len([]rune(w)) <= 5
}

// unisciFrammenti: le lettere molto spaziate dei titoli fanno spezzare le parole all'OCR:
// "CO NTROL" → "CONTROL", "P U B L I C" → "PUBLIC", "Inven tory" → "Inventory". Unisco due o più
// pezzi vicini solo se insieme fanno una parola vera e almeno un pezzo da solo non lo è
// (o sono lettere singole). Mai nomi di tasti singoli ("Press E to") né "A"/"I" da sole in frase.
func unisciFrammenti(s string) string {
	w := strings.Fields(s)
	if len(w) < 2 {
		return s
	}
	var out []string
	for i := 0; i < len(w); {
		fatto := false
		// prima i gruppi più lunghi (fino a 8 pezzi: "P U B L I C")
		for k := min(8, len(w)-i); k >= 2 && !fatto; k-- {
			pezzi := w[i : i+k]
			nudo := make([]string, k)
			ok := true
			for j, p := range pezzi {
				n := strings.TrimFunc(p, func(r rune) bool { return !unicode.IsLetter(r) })
				// punteggiatura solo prima del primo e dopo l'ultimo pezzo
				if n == "" || (j > 0 && strings.IndexFunc(p, unicode.IsLetter) != 0) || (j < k-1 && !strings.HasSuffix(p, n)) {
					ok = false
					break
				}
				nudo[j] = n
			}
			if !ok {
				continue
			}
			unita := strings.Join(nudo, "")
			if len([]rune(unita)) < 4 || !soloLettere(unita) || !parolaValidaOCR(unita) {
				continue
			}
			// stessa forma: tutto maiuscolo, oppure maiuscola solo all'inizio
			if !(strings.ToUpper(unita) == unita || isLowerOrCap(unita)) {
				continue
			}
			unoFalso, singole := false, 0
			for _, n := range nudo {
				if len([]rune(n)) == 1 {
					singole++
				}
				if !parolaValidaOCR(n) && len([]rune(n)) > 1 {
					unoFalso = true
				}
			}
			tutteSingole := singole == k
			if !unoFalso && !tutteSingole {
				continue // "to day" restano due parole vere: non le tocco
			}
			if tutteSingole && k < 3 {
				continue // "A I" no: troppo poco per dire che è una parola spaziata
			}
			pre := pezzi[0][:strings.Index(pezzi[0], nudo[0])]
			ultimo := pezzi[k-1]
			post := ultimo[strings.LastIndex(ultimo, nudo[k-1])+len(nudo[k-1]):]
			out = append(out, pre+unita+post)
			i += k
			fatto = true
		}
		if !fatto {
			out = append(out, w[i])
			i++
		}
	}
	return strings.Join(out, " ")
}

// paroleDoppie: "thing thing", "the the": parola ripetuta di fila (tipico dell'OCR che legge due
// volte la stessa zona). Alcune ripetizioni sono vere ("that that", "had had", "very very").
var doppieVere = map[string]bool{"that": true, "had": true, "very": true, "no": true, "go": true, "bye": true, "ha": true, "knock": true, "run": true, "now": true}

func paroleDoppie(s string) bool {
	// la ripetizione voluta ha la punteggiatura in mezzo ("I am sick, sick to the…", "No… no"):
	// quella non è l'OCR che legge due volte la stessa zona
	campi := strings.Fields(strings.ToLower(s))
	nuda := func(c string) string {
		return strings.TrimFunc(c, func(r rune) bool { return !unicode.IsLetter(r) && r != '\'' })
	}
	for i := 1; i < len(campi); i++ {
		a, b := nuda(campi[i-1]), nuda(campi[i])
		if a != b || len(b) < 3 || doppieVere[b] {
			continue
		}
		if fine := strings.TrimRightFunc(campi[i-1], func(r rune) bool { return !unicode.IsLetter(r) }); fine != campi[i-1] {
			continue // "sick, sick": c'è un segno dopo la prima
		}
		return true
	}
	return false
}

// ---------------------------------------------------------------- frasi pronte "quasi uguali"

type voceIndiceFrasi struct {
	inglese string // la frase del catalogo come è scritta
	norm    []rune // lettereConfuse della frase
	numeri  string
	it      string // la sua traduzione (due frasi con la stessa traduzione non sono in gara)
}

var (
	indiceFrasiOnce sync.Once
	indiceFrasi     map[int][]voceIndiceFrasi // lunghezza normalizzata -> frasi
	muAggancio      sync.Mutex
	cacheAggancio   = map[string]risAggancio{}
)

type risAggancio struct {
	frase string
	dist  int
	ok    bool
}

func preparaIndiceFrasi() {
	indiceFrasiOnce.Do(func() {
		caricaCataloghi()
		indiceFrasi = map[int][]voceIndiceFrasi{}
		for _, en := range frasiMenuInglesi() {
			n := []rune(lettereConfuse(en))
			if len(n) < 5 {
				continue
			}
			indiceFrasi[len(n)] = append(indiceFrasi[len(n)], voceIndiceFrasi{inglese: en, norm: n, numeri: numeriESegni(en), it: frasiMenu[chiaveFrase(en)]})
		}
	})
}

// frasiMenuInglesi: le frasi inglesi del catalogo come sono scritte nel file.
func frasiMenuInglesi() []string {
	var out []string
	for _, l := range strings.Split(frasiMenuTSV, "\n") {
		if strings.HasPrefix(l, "#") {
			continue
		}
		if en, _, ok := strings.Cut(l, "\t"); ok && strings.TrimSpace(en) != "" {
			out = append(out, strings.TrimSpace(en))
		}
	}
	return out
}

// agganciaCatalogo: la frase pronta più vicina alla lettura, se la differenza è piccola e chiara.
// conSospetti: la lettura ha parole inventate o rare (allora accetto più errori). Restituisce la
// frase del catalogo, quanti errori (lettere) e se va bene.
func agganciaCatalogo(s string, conSospetti bool) (string, int, bool) {
	if _, ok := fraseMenu(s); ok {
		return s, 0, true
	}
	n := []rune(lettereConfuse(s))
	if len(n) < 6 || len(n) > 120 {
		return "", 0, false
	}
	chiave := s
	if conSospetti {
		chiave += "\x00s"
	}
	muAggancio.Lock()
	if r, ok := cacheAggancio[chiave]; ok {
		muAggancio.Unlock()
		return r.frase, r.dist, r.ok
	}
	muAggancio.Unlock()
	preparaIndiceFrasi()
	// quanti errori accetto: pochi per le scritte corte, fino al 18% per le frasi lunghe con
	// parole sospette; senza parole sospette solo un errore ogni ~16 lettere ("bur" → "bar")
	var soglia int
	switch {
	case conSospetti && len(n) >= 25:
		soglia = len(n) * 18 / 100
	case conSospetti && len(n) >= 12:
		soglia = max(2, len(n)*14/100)
	case conSospetti:
		soglia = 1
	default:
		soglia = len(n) / 16
	}
	num := numeriESegni(s)
	migliore, itMig, dMig, dSec := "", "", soglia+1, soglia+4
	for l := len(n) - soglia; l <= len(n)+soglia; l++ {
		for _, v := range indiceFrasi[l] {
			if v.numeri != num {
				continue // "Level 5" non diventa mai "Level 6"
			}
			d := distanzaTesti(n, v.norm, dSec-1)
			switch {
			case d < dMig:
				if v.it != itMig {
					dSec = min(dSec, dMig)
				}
				migliore, itMig, dMig = v.inglese, v.it, d
			case d < dSec && v.it != itMig:
				dSec = d
			}
		}
	}
	// chiaro: la seconda frase più vicina (con un'altra traduzione) è più lontana: di almeno 2
	// lettere, o di 1 se la prima ha un errore solo (se no potrei scegliere "Move to fill..."
	// invece di "Sneak to fill...")
	ok := migliore != "" && dMig <= soglia && (dSec-dMig >= 2 || (dMig <= 1 && dSec > dMig))
	r := risAggancio{migliore, dMig, ok}
	muAggancio.Lock()
	if len(cacheAggancio) > 4000 {
		cacheAggancio = map[string]risAggancio{}
	}
	cacheAggancio[chiave] = r
	muAggancio.Unlock()
	return r.frase, r.dist, r.ok
}

// ---------------------------------------------------------------- voto della riga

var reSoloSimboli = regexp.MustCompile(`^[\pP\pS\s]+$`)

// valutaParole: quante parole (di 2+ lettere) e quante sospette: inventate, rare o mezze
// maiuscole a caso ("HoldtONTROL"). I nomi propri (maiuscola iniziale in una frase normale) vanno bene.
func valutaParole(s string) (tot int, false_ int, rare int) {
	tot, false_, rare, _ = valutaParoleN(s)
	return
}

// valutaParoleN: come valutaParole, e in più quante parole sconosciute ho preso per nomi propri
// (vanno bene per tradurre, ma rendono sospetta la lettura quando c'è una frase pronta vicina).
func valutaParoleN(s string) (tot int, false_ int, rare int, nomi int) {
	tot, false_, rare, nomi, _ = valutaParolePos(s)
	return
}

// falseVicine: due parole inventate con al massimo una parola in mezzo ("No, I lc nit. Je has
// gone missing…"): lì la riga è storpiata, anche se il resto della frase è lungo e giusto.
// Tradotta verrebbe una frase senza senso.
func falseVicine(s string) bool {
	_, _, _, _, pos := valutaParolePos(s)
	for i := 1; i < len(pos); i++ {
		if pos[i]-pos[i-1] <= 2 {
			return true
		}
	}
	return false
}

// valutaParolePos: come valutaParoleN, e in più la posizione (indice della parola) di ogni
// parola inventata.
func valutaParolePos(s string) (tot int, false_ int, rare int, nomi int, posFalse []int) {
	tuttoMaiusc := tuttoMaiuscolo(s)
	campi := campiFrase(s)
	etichetta := etichettaIniziale(campi)
	for i, c := range campi {
		w := strings.Trim(c.w, "'")
		r := []rune(w)
		if len(r) < 2 || i < etichetta {
			continue // l'etichetta iniziale ("MARCUS:", "OLD MAN:") è un nome: non si giudica
		}
		tot++
		if i+1 < len(campi) && len(r) <= 3 && balbettata(s, w, campi[i+1].w) {
			continue // "wh-what", "I-I": la parola balbettata
		}
		if parolaValidaOCR(w) {
			if parolaRara(w) {
				rare++
			}
			continue
		}
		if parolaComposta(w) {
			continue // "medkit", "headshot", "cooldown": due parole vere attaccate
		}
		// maiuscole e minuscole mescolate a caso dentro la parola: lettura sbagliata
		mista := false
		for _, c := range r[1:] {
			if unicode.IsUpper(c) {
				mista = true
				break
			}
		}
		switch {
		case tuttoMaiusc:
			// riga tutta maiuscola: le sigle corte (HP, FOV, NPC) vanno bene, le parole inventate no
			if len(r) <= 3 {
				continue
			}
		case !mista && unicode.IsUpper(r[0]) && (!c.inizio || len(campi) == 1) && len(r) >= 3:
			// (a inizio frase, dopo ". " o ": ", la maiuscola non dice che è un nome: "FIP: Crojucning")
			// nome proprio dentro una frase ("Palworld", "Kael"); da solo conta se è lungo
			if len(campi) > 1 || len(r) >= 4 {
				nomi++
				continue
			}
		case strings.ToUpper(w) == w && len(r) <= 4 && !tuttoMaiusc:
			continue // sigla in una frase normale ("XP", "DLSS")
		case !tuttoMaiusc && nomeChiamato(w, c.virgolaDopo):
			// "Caz, don't let me die…", "Koby's Capabilities", "Mr McCleary": anche a inizio
			// frase la parola è un nome (chiamato con la virgola, con il possessivo, "Mc…")
			nomi++
			continue
		}
		false_++
		if !(nomeProprio(w) && len(r) >= 4) { // "Farah. Barkov's soldiers…": nomi a inizio frase, non spazzatura
			posFalse = append(posFalse, i)
		}
	}
	return
}

// balbettata: la parola w è l'inizio della parola dopo ripetuto con il trattino ("wh-what").
func balbettata(s, w, dopo string) bool {
	lw, ld := strings.ToLower(w), strings.ToLower(dopo)
	return len(ld) > len(lw) && strings.HasPrefix(ld, lw) && strings.Contains(strings.ToLower(s), lw+"-"+ld)
}

// nomeChiamato: la parola sconosciuta con la maiuscola è un nome anche a inizio frase: seguita
// dalla virgola ("Caz,"), con il possessivo ("Koby's") o un cognome "Mc…"/"Mac…" ("McCleary").
func nomeChiamato(w string, virgola bool) bool {
	base := strings.TrimSuffix(strings.TrimSuffix(w, "'s"), "’s")
	if len([]rune(base)) < 3 {
		return false
	}
	if strings.HasPrefix(base, "Mc") && nomeProprio(base[2:]) {
		return true
	}
	if strings.HasPrefix(base, "Mac") && nomeProprio(base[3:]) {
		return true
	}
	return nomeProprio(base) && (virgola || base != w)
}

// campoFrase: una parola della riga e se inizia una frase (prima parola, o dopo . ! ? :).
type campoFrase struct {
	w           string
	inizio      bool
	dueDopo     bool // subito dopo la parola c'è ":" ("MARCUS:", "TIP:")
	virgolaDopo bool // subito dopo la parola c'è "," ("Caz, don't…": il nome di chi viene chiamato)
	maiuscolaIn bool
}

// campiFrase: le parole (lettere e apostrofi) della riga con la punteggiatura che le separa.
func campiFrase(s string) []campoFrase {
	var out []campoFrase
	var cur []rune
	punto := true
	chiudi := func() {
		if len(cur) > 0 {
			out = append(out, campoFrase{w: string(cur), inizio: punto, maiuscolaIn: unicode.IsUpper(cur[0])})
			cur = cur[:0]
			punto = false
		}
	}
	for _, r := range s {
		if unicode.IsLetter(r) || r == '\'' {
			cur = append(cur, r)
			continue
		}
		chiudi()
		switch r {
		case ':':
			if n := len(out); n > 0 && !punto {
				out[n-1].dueDopo = true
			}
			punto = true
		case '.', '!', '?':
			punto = true
		case ',':
			if n := len(out); n > 0 && !punto {
				out[n-1].virgolaDopo = true
			}
		}
	}
	chiudi()
	return out
}

// etichettaIniziale: quante parole all'inizio sono un'etichetta con i due punti, cioè il nome di
// chi parla o il tipo di scritta ("MARCUS:", "OLD MAN:", "Kael:"): una o due parole con la
// maiuscola seguite da ":" e poi altro testo. 0 se non c'è.
func etichettaIniziale(c []campoFrase) int {
	for k := 0; k < 2 && k < len(c)-1; k++ {
		if !c[k].maiuscolaIn {
			return 0
		}
		if c[k].dueDopo {
			return k + 1
		}
	}
	return 0
}

// parolaComposta: parola sconosciuta fatta di due parole inglesi vere e comuni, di almeno 3
// lettere ciascuna ("medkit" = med+kit, "headshot", "cooldown"): nei giochi è normale.
func parolaComposta(w string) bool {
	l := strings.ToLower(w)
	r := []rune(l)
	if len(r) < 6 || len(r) > 16 || !soloLettere(l) {
		return false
	}
	for k := 3; k <= len(r)-3; k++ {
		a, b := string(r[:k]), string(r[k:])
		if esiste(a) && esiste(b) && !parolaRara(a) && !parolaRara(b) {
			return true
		}
	}
	return false
}

// sconosciutaPlausibile: la sola parola sconosciuta della riga ha la forma di una parola vera
// (lettere minuscole, almeno 4, vocali e consonanti alternate in modo normale): un termine dei
// giochi che il vocabolario non ha, non lettere a caso dell'OCR ("rnedklt", "Hxq").
func sconosciutaPlausibile(s string) bool {
	trovata := ""
	for _, c := range campiFrase(s) {
		w := strings.Trim(c.w, "'")
		if len([]rune(w)) < 2 || parolaValidaOCR(w) || parolaComposta(w) {
			continue
		}
		if trovata != "" {
			return false
		}
		trovata = w
	}
	r := []rune(trovata)
	if len(r) < 4 || len(r) > 14 {
		return false
	}
	vocali, filaCons, maxCons := 0, 0, 0
	for i, c := range r {
		if !unicode.IsLetter(c) || (i > 0 && !unicode.IsLower(c)) {
			return false
		}
		if strings.ContainsRune("aeiouyAEIOUY", c) {
			vocali++
			filaCons = 0
		} else {
			filaCons++
			maxCons = max(maxCons, filaCons)
		}
		if i >= 2 && r[i] == r[i-1] && r[i] == r[i-2] {
			return false
		}
	}
	if vocali == 0 || vocali*100 < len(r)*20 || vocali*100 > len(r)*70 || maxCons > 3 {
		return false
	}
	// lettere che l'OCR confonde ("rn" per "m", "vv" per "w"): se la parola corretta esiste, era
	// una lettura sbagliata, non un termine nuovo
	for _, k := range candidati(strings.ToLower(trovata)) {
		if k != "" {
			return false
		}
	}
	return true
}

// reSegnoAttaccato: "health!find", "ready?Go": manca lo spazio dopo ! o ? tra due parole.
var reSegnoAttaccato = regexp.MustCompile(`(\p{Ll}{2})([!?]+)(\p{L})`)

// correggiRigaOCR: la riga letta corretta e quanto mi fido. Usata dalla traduzione di tutta la
// schermata: una riga con voto letturaDubbia NON si traduce (resta l'inglese del gioco).
func correggiRigaOCR(grezzo string) (string, int) {
	s := strings.Join(strings.Fields(maiuscolaConfusa(lettereOCRStrane(grezzo))), " ")
	s = reSegnoAttaccato.ReplaceAllString(s, "$1$2 $3")
	s = correggiContatori(correggiPercentuale(s))
	if s == "" || reSoloSimboli.MatchString(s) {
		return s, letturaDubbia
	}
	if _, ok := fraseMenuConNumero(s); ok {
		return s, letturaCatalog // già esatta (anche "progess", se il gioco la scrive così)
	}
	if letteraSolaIniziale(s) {
		return s, letturaDubbia // "s noise when…": la riga ripresa da metà parola, un pezzo
	}
	s1 := unisciFrammenti(s)
	s2 := verboFinale(correggiParole(separaAttaccate(s1)))
	if s2 != s1 {
		// la correzione delle parole ha unito pezzi veri o diviso parole: ricontrollo i frammenti
		s2 = unisciFrammenti(s2)
	}
	if _, ok := fraseMenuConNumero(s2); ok {
		return s2, letturaCatalog
	}
	tot, false_, rare, nomi := valutaParoleN(s2)
	_, f1, r1, n1 := valutaParoleN(s)
	doppie := paroleDoppie(s) || paroleDoppie(s2)
	sospetti := false_ > 0 || rare > 0 || doppie || f1 > 0 || r1 > 0 || nomi > 0 || n1 > 0
	// aggancio alla frase pronta: provo sia la lettura grezza sia quella corretta
	best, bestD := "", 1<<30
	for _, c := range []string{s, s1, s2} {
		if f, d, ok := agganciaCatalogo(c, sospetti); ok && d < bestD {
			best, bestD = f, d
		}
	}
	if best != "" {
		return conPunteggiatura(best, s2), letturaCatalog
	}
	if tot == 0 {
		return s2, letturaDubbia
	}
	// pezzo di frase (comincia minuscolo) con una parola che non esiste: è un seguito letto male,
	// meglio l'inglese del gioco che mezza frase storpiata
	if iniziaMinuscola(s) && (false_ > 0 || f1 > 0) {
		return s2, letturaDubbia
	}
	// parola spezzata in due dall'OCR in mezzo alla riga ("enemies ar Ire nearby."): un pezzo che
	// non esiste seguito da un pezzetto con la maiuscola che non è una parola comune
	if parolaSpezzata(s) {
		return s2, letturaDubbia
	}
	// due parole inventate nella lettura grezza e una rimasta anche dopo la correzione ("arepe
	// earby" -> "arepe ear by"): la riga è storpiata, anche se lunga
	if f1 >= 2 && false_ >= 1 && f1*100 > tot*15 {
		return s2, letturaDubbia
	}
	// riga corta (fino a 4 parole): ogni parola deve essere vera; al massimo UNA parola che non
	// conosco, se le altre (almeno due) sono vere e comuni e lei ha la forma di una parola
	// ("Low health! Find a medkit": "medkit" non è nel vocabolario, ma la lettura è buona)
	if tot <= 4 {
		if doppie || (rare > 0 && tot <= 2) {
			return s2, letturaDubbia
		}
		if false_ > 0 && !(false_ == 1 && tot >= 3 && rare == 0 && f1 <= 1 && sconosciutaPlausibile(s2)) {
			return s2, letturaDubbia
		}
		return s2, letturaBuona
	}
	// frase lunga: al massimo una parola sospetta ogni 8, niente parole doppie. Una sola parola
	// sconosciuta ma con la forma di una parola vera (termine del gioco, errore del gioco stesso)
	// pesa come una rara: non basta a scartare una frase per il resto giusta.
	peso := false_ * 2
	if false_ == 1 && sconosciutaPlausibile(s2) {
		peso = 1
	}
	if doppie || (peso+rare)*100 > tot*25 || falseVicine(s2) {
		return s2, letturaDubbia
	}
	// tante parole inventate nella lettura grezza (anche se il vocabolario ne ha "aggiustate"
	// alcune) e almeno una ancora sbagliata: è spazzatura dell'OCR ("FIP: Crojucning rnakes 123,
	// noise when enemies are earby"). Meglio l'inglese del gioco che una traduzione senza senso.
	if f1 >= 3 && f1*100 > tot*30 && false_+rare > 0 {
		return s2, letturaDubbia
	}
	return s2, letturaBuona
}

// maiuscolaConfusa: parola tutta maiuscola con l'ultima lettera letta minuscola perché le due
// forme si somigliano ("MAp", "BOx", "SAVe"): torna tutta maiuscola.
func maiuscolaConfusa(s string) string {
	campi := strings.Fields(s)
	cambiata := false
	for i, w := range campi {
		r := []rune(w)
		n := len(r)
		for n > 0 && !unicode.IsLetter(r[n-1]) {
			n--
		}
		if n < 3 || !strings.ContainsRune("cklopsuvwxz", r[n-1]) {
			continue
		}
		tutte := true
		for _, c := range r[:n-1] {
			if unicode.IsLetter(c) && !unicode.IsUpper(c) {
				tutte = false
				break
			}
		}
		if tutte && unicode.IsUpper(r[0]) && unicode.IsUpper(r[1]) {
			r[n-1] = unicode.ToUpper(r[n-1])
			campi[i] = string(r)
			cambiata = true
		}
	}
	if !cambiata {
		return s
	}
	return strings.Join(campi, " ")
}

// letteraSolaIniziale: la riga comincia con una lettera sola minuscola (non "a") seguita da una
// parola minuscola: "s noise when enemies…" è la fine di una parola tagliata, non una frase.
func letteraSolaIniziale(s string) bool {
	w := strings.Fields(s)
	if len(w) < 2 {
		return false
	}
	r := []rune(w[0])
	return len(r) == 1 && r[0] != 'a' && unicode.IsLower(r[0]) && iniziaMinuscola(w[1])
}

// conPunteggiatura: la frase del catalogo con la punteggiatura finale e le maiuscole della
// lettura ("PUBLIC GAME" resta tutta maiuscola, "!" in fondo resta).
func conPunteggiatura(catalogo, letto string) string {
	out := catalogo
	if tuttoMaiuscolo(letto) {
		out = strings.ToUpper(out)
	}
	fineL := letto[len(strings.TrimRightFunc(letto, func(r rune) bool { return unicode.IsPunct(r) && r != '+' })):]
	nuda := strings.TrimRightFunc(out, func(r rune) bool { return unicode.IsPunct(r) && r != '+' })
	if fineL != "" {
		return nuda + fineL
	}
	return out
}

// ---------------------------------------------------------------- righe mezze tradotte

// paroleUgualiIt: parole che in italiano si scrivono come in inglese (prestiti e parole comuni):
// se restano nella traduzione non vuol dire che la riga è mezza tradotta.
var paroleUgualiIt = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`mouse computer online offline menu tutorial file bar film sport video radio extra
		standard server chat team boss bonus multiplayer singleplayer quest round match skin loot stealth crafting
		hardware software internet email web download upload password login logout account premium gamepad joystick
		controller console pad zoom click hotel taxi bus camion gas golf tennis leader manager design fan stop ok okay
		hobby weekend pixel display monitor laptop tablet smartphone app wifi bluetooth reset partner killer spray
		sprint drone robot laser radar sonar jet box ring club pub rock jazz blues pop hip hop rap
		replay coop co-op speedrun streamer stream social slot item tank healer raid spawn respawn
		come dove fine sole solo male idea area camera pane cane mare era ora data base porta carta
		auto moto nord sud est ovest mini maxi super ultra media mega giga turbo pro ex
		deluxe premium gold silver platinum diamond hardcore casual arcade survival horror action fantasy
		sci-fi cyberpunk steampunk rpg mmo fps hud fov dlss fsr hdr vsync ui gui ai npc
		desktop lobby checkpoint volume default gameplay frame framerate ping lag bug patch mod combo perk
		buff nerf build crash texture shader bloom blur preset ranked clan party emote trailer badge medkit
		headshot cooldown v-sync anti-aliasing taa fxaa smaa msaa ssao xess gamma ray tracing widget
		jumpscare speedrunner leaderboard matchmaking hub dungeon raid tier
		signora signore signor signorina piazza villa opera studio piano ciao bravo brava maestro nonna nonno
		padre madre fresco pasta pizza mafia vendetta`) {
		paroleUgualiIt[w] = true
	}
}

// rigaMezzaTradotta: nella traduzione sono rimaste parole inglesi della frase (non nomi, non
// tasti, non prestiti come "mouse"): "Move to fill il progresso bur", "HoldtONTROL e use WASD".
// Una riga così NON si mostra: si traduce tutta o niente.
func rigaMezzaTradotta(en, it string) bool {
	en, it = strings.TrimSpace(en), strings.TrimSpace(it)
	if en == "" || it == "" || strings.EqualFold(en, it) {
		return false
	}
	divide := func(r rune) bool { return !unicode.IsLetter(r) && r != '\'' && r != '-' }
	inglesi := map[string]bool{}
	tuttoM := tuttoMaiuscolo(en)
	campiEn := strings.FieldsFunc(en, divide)
	nomi := map[string]bool{}
	tastiEn := map[string]bool{} // nomi di tasti scritti nella frase ("the Shift key"): restano uguali
	for i, w := range campiEn {
		w = strings.Trim(w, "'-")
		inglesi[strings.ToLower(w)] = true
		r := []rune(w)
		// nomi propri: maiuscola iniziale dentro la frase, parola non inglese ("Palbox", "Kael")
		if len(r) > 1 && unicode.IsUpper(r[0]) && !tuttoM && i > 0 && !esiste(w) {
			nomi[strings.ToLower(w)] = true
		}
		if len(r) > 1 && unicode.IsUpper(r[0]) && nomiTastiParole[strings.ToUpper(w)] {
			tastiEn[w] = true
		}
	}
	// nomi di oggetti e missioni dopo un'etichetta: "Item collected: Rusty Key", "Quest completed:
	// Old Friends" (tutte le parole con la maiuscola dopo i due punti, in una riga non tutta a
	// maiuscole né tutta con le iniziali maiuscole prima dei due punti)
	if prima, dopo, ok := strings.Cut(en, ":"); ok && !tuttoM && strings.ContainsFunc(prima, unicode.IsLower) {
		campi := strings.FieldsFunc(dopo, divide)
		titolo := len(campi) > 0
		for _, w := range campi {
			if r := []rune(w); len(r) > 3 && !unicode.IsUpper(r[0]) {
				titolo = false
			}
		}
		if titolo && !strings.ContainsAny(strings.TrimSpace(dopo), ".!?") {
			for _, w := range campi {
				nomi[strings.ToLower(strings.Trim(w, "'-"))] = true
			}
		}
	}
	for _, w := range nomeChiParla(en) {
		nomi[strings.ToLower(w)] = true // "MARCUS: ..." il nome di chi parla resta com'è
	}
	caricaCataloghi()
	titolo, nomeTitolo := titoloCorto(en), ""
	for _, w := range strings.FieldsFunc(it, divide) {
		w = strings.Trim(w, "'-")
		r := []rune(w)
		low := strings.ToLower(w)
		if len(r) < 3 || !inglesi[low] || paroleUgualiIt[low] || nomi[low] {
			continue
		}
		up := strings.ToUpper(w)
		if nomiTastiFissi[up] || (nomiTastiParole[up] && (w == up || tastiEn[w])) || segnapostoTasto(w) {
			continue // tasti: restano uguali apposta
		}
		if it2, ok := termGiochi[w]; ok && strings.EqualFold(it2, w) {
			continue // il glossario dice che resta così
		}
		if strings.ToUpper(w) == w && len(r) <= 5 && !tuttoM {
			continue // sigla ("HP", "XP", "DLSS")
		}
		if !esiste(w) {
			// parola inventata copiata così com'è: nome del gioco (maiuscola iniziale) va bene,
			// spazzatura dell'OCR ("thingthing", "HoldtONTROL") no
			mista := false
			for _, c := range r[1:] {
				if unicode.IsUpper(c) {
					mista = true
				}
			}
			if !mista && unicode.IsUpper(r[0]) && !tuttoM {
				continue
			}
			return true
		}
		if titolo && nomeTitolo == "" && parolaDaNome(w) {
			nomeTitolo = low // "EMBER KINGDOM" -> "REGNO DI EMBER": nome di fantasia del titolo
			continue
		}
		return true // parola inglese vera rimasta in inglese
	}
	if nomeTitolo != "" {
		// il nome resta solo se il resto del titolo è stato tradotto davvero
		for _, w := range strings.FieldsFunc(it, divide) {
			if l := strings.ToLower(strings.Trim(w, "'-")); len([]rune(l)) >= 3 && !inglesi[l] {
				return false
			}
		}
		return true
	}
	return false
}

// titoloCorto: un titolo (di gioco, di capitolo, di luogo): al massimo 4 parole, tutte con la
// maiuscola (o tutto maiuscolo), senza punteggiatura da frase.
func titoloCorto(en string) bool {
	if strings.ContainsAny(en, ".!?,;:") {
		return false
	}
	campi := strings.Fields(en)
	if len(campi) < 2 || len(campi) > 4 {
		return false
	}
	for _, w := range campi {
		r := []rune(w)
		if !unicode.IsUpper(r[0]) && !paroleSospese[strings.ToLower(w)] {
			return false
		}
	}
	return true
}

// parolaDaNome: parola inglese poco comune (livello 35 o più nel vocabolario, anche al singolare)
// scritta con la maiuscola: in un titolo è quasi sempre un nome di fantasia ("Ember", "Raven",
// "Thorn") che il traduttore lascia giustamente com'è. Le parole comuni ("Game", "Shadow",
// "Iron") restano errori: andavano tradotte.
func parolaDaNome(w string) bool {
	r := []rune(w)
	if len(r) < 3 || !unicode.IsUpper(r[0]) {
		return false
	}
	low := strings.ToLower(w)
	v := vocabolario()
	liv, ok := v[low]
	if !ok {
		return false
	}
	for _, suf := range []string{"es", "s"} {
		if b := strings.TrimSuffix(low, suf); b != low {
			if l2, ok2 := v[b]; ok2 && l2 < liv {
				liv = l2
			}
		}
	}
	return liv >= 35
}

// ---------------------------------------------------------------- tasti come segnaposti

// I nomi dei tasti (CONTROL, WASD, SHIFT, E, LMB, "left mouse") non si traducono: il motore
// veloce li storpia ("CONTROLLO", "sinistra mouse"). Prima di tradurre li metto da parte con dei
// "nomi finti" che il motore lascia come sono, poi li rimetto.

var segnapostiTasti = []string{"Qzva", "Qzve", "Qzvi", "Qzvo", "Qzvu", "Qzwa", "Qzwe", "Qzwi", "Qzwo", "Qzwu"}

func segnapostoTasto(w string) bool {
	for _, s := range segnapostiTasti {
		if w == s {
			return true
		}
	}
	return false
}

var (
	reMouseTasto = regexp.MustCompile(`(?i)\b(?:the\s+)?(left|right|middle)[\s-]+(?:mouse|click)(?:[\s-]+button)?\b`)
	reParolaTast = regexp.MustCompile(`\b[A-Za-z][A-Za-z0-9]*\b`)
	reDopoPress  = regexp.MustCompile(`(?i)\b(press|hold|tap|use|release|key|button|push|keep|holding|pressing)\s*$`)
)

// tastoMouseIt: "left mouse" in italiano.
var tastoMouseIt = map[string]string{"left": "il tasto sinistro del mouse", "right": "il tasto destro del mouse", "middle": "il tasto centrale del mouse"}

// proteggiTasti: il testo con i tasti sostituiti dai segnaposti, e cosa rimettere al loro posto.
func proteggiTasti(s string) (string, map[string]string) {
	m := map[string]string{}
	n := 0
	prossimo := func() string {
		if n >= len(segnapostiTasti) {
			return ""
		}
		p := segnapostiTasti[n]
		n++
		return p
	}
	// prima "left mouse (button)": diventa "il tasto sinistro del mouse"
	s = reMouseTasto.ReplaceAllStringFunc(s, func(t string) string {
		sm := reMouseTasto.FindStringSubmatch(t)
		p := prossimo()
		if p == "" {
			return t
		}
		m[p] = tastoMouseIt[strings.ToLower(sm[1])]
		return p
	})
	tuttoM := tuttoMaiuscolo(s)
	idx := reParolaTast.FindAllStringIndex(s, -1)
	var b strings.Builder
	ultimo := 0
	for _, ix := range idx {
		w := s[ix[0]:ix[1]]
		prima := s[:ix[0]]
		tasto := false
		switch {
		case segnapostoTasto(w):
		case nomiTastiFissi[w]:
			tasto = true
		case nomiTastiParole[w] && (!tuttoM || reDopoPress.MatchString(prima)):
			tasto = true
		case len(w) == 1 && w[0] >= 'A' && w[0] <= 'Z' && w != "A" && w != "I" && !tuttoM:
			tasto = true // "Press E to interact"
		case len(w) == 1 && w[0] >= 'A' && w[0] <= 'Z' && reDopoPress.MatchString(prima):
			tasto = true // "PRESS E", "Press A to jump"
		case len(w) >= 2 && len(w) <= 3 && w[0] == 'F' && w[1] >= '1' && w[1] <= '9' && strings.Trim(w[1:], "0123456789") == "":
			tasto = true // F1…F12
		}
		if !tasto {
			continue
		}
		p := prossimo()
		if p == "" {
			break
		}
		m[p] = w
		b.WriteString(s[ultimo:ix[0]])
		b.WriteString(p)
		ultimo = ix[1]
	}
	b.WriteString(s[ultimo:])
	if len(m) == 0 {
		return s, nil
	}
	return b.String(), m
}

var reArticoloDoppio = regexp.MustCompile(`(?i)\b(il|lo|la|l'|del|dello|della|al|allo|alla|nel|col|sul|dal|con il|con)\s+(il tasto)`)

// ripristinaTasti: rimette i tasti al posto dei segnaposti. false se il motore ne ha perso uno.
func ripristinaTasti(s string, m map[string]string) (string, bool) {
	// i segnaposti in ordine dal più lungo nome: nessuno è prefisso di un altro, ma vado sicuro
	chiavi := make([]string, 0, len(m))
	for k := range m {
		chiavi = append(chiavi, k)
	}
	sort.Strings(chiavi)
	for _, k := range chiavi {
		if !strings.Contains(s, k) {
			return "", false
		}
		s = strings.ReplaceAll(s, k, m[k])
	}
	// "del il tasto sinistro" → "del tasto sinistro", "il il tasto" → "il tasto"
	s = reArticoloDoppio.ReplaceAllStringFunc(s, func(t string) string {
		sm := reArticoloDoppio.FindStringSubmatch(t)
		art := strings.ToLower(sm[1])
		switch art {
		case "il", "lo", "la", "l'":
			return sm[2]
		case "con", "con il":
			return sm[1][:3] + " il tasto"
		}
		return sm[1] + " tasto"
	})
	return s, true
}

// tastiIntatti: dopo una traduzione senza segnaposti, i tasti dell'inglese che il motore ha
// storpiato ("CO NTROL", "CONTROLLO", "Control") tornano come nel gioco.
func tastiIntatti(en, it string) string {
	_, m := proteggiTasti(en)
	for _, tasto := range m {
		if strings.HasPrefix(tasto, "il tasto") || len(tasto) < 3 {
			continue
		}
		if regexp.MustCompile(`\b` + regexp.QuoteMeta(tasto) + `\b`).MatchString(it) {
			continue
		}
		// una parola o due vicine che somigliano al tasto: le sostituisco
		campi := strings.Fields(it)
		for i := range campi {
			for k := 1; k <= 2 && i+k <= len(campi); k++ {
				pezzo := strings.Join(campi[i:i+k], "")
				nudo := strings.TrimFunc(pezzo, func(r rune) bool { return !unicode.IsLetter(r) })
				if nudo == "" || unicode.ToUpper([]rune(nudo)[0]) != rune(tasto[0]) {
					continue
				}
				d := distanzaLettere(strings.ToLower(nudo), strings.ToLower(tasto))
				if d <= 2 && d*3 <= len(tasto) || strings.HasPrefix(strings.ToLower(nudo), strings.ToLower(tasto)) && len(nudo)-len(tasto) <= 2 {
					pre := pezzo[:strings.Index(pezzo, nudo)]
					post := pezzo[strings.Index(pezzo, nudo)+len(nudo):]
					nuovi := append(append(append([]string{}, campi[:i]...), pre+tasto+post), campi[i+k:]...)
					return tastiIntatti(en, strings.Join(nuovi, " "))
				}
			}
		}
	}
	return it
}

// nomeChiParla: le parole del nome di chi parla all'inizio di un sottotitolo o di un dialogo
// ("MARCUS: I don't trust him.", "OLD MAN: The bridge...", "Kael: Follow me."). Il nome resta
// com'è nella traduzione e non rende la riga "mezza in inglese". Il nome: da 1 a 3 parole tutte
// maiuscole, o una parola sola con la maiuscola che non è una parola inglese (un nome proprio);
// dopo i due punti una frase vera (con le minuscole, almeno due parole).
func nomeChiParla(en string) []string {
	if n, _ := separaParlante(strings.TrimSpace(en)); n != "" {
		return strings.Fields(n) // lo stesso nome che il traduttore lascia com'è
	}
	prima, dopo, ok := strings.Cut(strings.TrimSpace(en), ":")
	if !ok || !strings.ContainsFunc(dopo, unicode.IsLower) || len(strings.Fields(dopo)) < 2 {
		return nil
	}
	campi := strings.Fields(prima)
	if len(campi) == 0 || len(campi) > 3 {
		return nil
	}
	tutteMaiuscole := true
	for _, w := range campi {
		r := []rune(w)
		lettere := 0
		for _, c := range r {
			switch {
			case unicode.IsLetter(c):
				lettere++
				if unicode.IsLower(c) {
					tutteMaiuscole = false
				}
			case c == '\'' || c == '-' || c == '.':
			default:
				return nil
			}
		}
		if lettere == 0 {
			return nil
		}
	}
	if tutteMaiuscole {
		if len([]rune(strings.Join(campi, ""))) < 2 {
			return nil
		}
		return campi
	}
	if len(campi) == 1 {
		r := []rune(campi[0])
		if len(r) >= 2 && unicode.IsUpper(r[0]) && !esiste(campi[0]) {
			return campi
		}
	}
	return nil
}

// parolaSpezzata: nella riga c'è una parola letta in due pezzi ("ar Ire"): il primo è di sole
// minuscole e non esiste, il secondo è un pezzetto con la maiuscola (iniziaConFrammento).
func parolaSpezzata(s string) bool {
	w := strings.Fields(s)
	for i := 0; i+1 < len(w); i++ {
		if finisceConFrammento(w[i]) && iniziaConFrammento(w[i+1]) {
			return true
		}
	}
	return false
}

// verbiFinali: verbi comunissimi che chiudono una frase subito dopo il soggetto ("…that you
// have!", "Do you know?", "I can!").
var verbiFinali = map[string]bool{"have": true, "do": true, "did": true, "does": true, "are": true,
	"were": true, "am": true, "was": true, "can": true, "could": true, "will": true, "would": true,
	"should": true, "know": true, "see": true, "want": true, "need": true, "like": true, "mean": true,
	"think": true, "say": true, "said": true, "go": true, "went": true, "come": true, "came": true,
	"got": true, "get": true, "had": true, "may": true, "might": true, "must": true, "love": true,
	"hate": true, "made": true, "make": true, "take": true, "took": true, "saw": true, "told": true,
	"try": true, "care": true, "remember": true, "forget": true, "understand": true, "agree": true,
	"wish": true, "promise": true, "swear": true, "hope": true, "guess": true, "suppose": true}

var reVerboFinale = regexp.MustCompile(`\b(I|[Yy]ou|[Ww]e|[Tt]hey)\s+([A-Za-z]+)\s*([!?.]+["'”’)]*)(\s|$)`)

// verboFinale: dopo il soggetto, in fondo alla frase, l'OCR ha letto una parola vera ma fuori
// posto al posto di un verbo comunissimo: "that you hay !" (have!), "you nave?". La parola letta
// è meno comune (livello del vocabolario 20 o più) e c'è UN SOLO verbo comune a una lettera di
// distanza per una confusione tipica dell'OCR (y/v, c/e, rn/m… anche con la "e" finale persa:
// hay → hav → have): si usa quello. Se i verbi possibili
// sono due o più, o la parola è comune ("I bet!", "you too!"), resta com'è.
func verboFinale(s string) string {
	v := vocabolario()
	if len(v) == 0 || !strings.ContainsAny(s, "!?.") {
		return s
	}
	return reVerboFinale.ReplaceAllStringFunc(s, func(m string) string {
		p := reVerboFinale.FindStringSubmatch(m)
		w := strings.ToLower(p[2])
		if verbiFinali[w] || p[2] != w {
			return m // già un verbo, o maiuscole (nome, sigla)
		}
		if l, ok := v[w]; ok && l < 20 {
			return m // parola comune: la frase può essere giusta così
		}
		trovati := map[string]bool{}
		for _, c := range append(varianteConfuse(w), w) {
			for _, x := range []string{c, c + "e"} {
				if verbiFinali[x] && x != w {
					trovati[x] = true
				}
			}
		}
		if len(trovati) != 1 {
			return m
		}
		for x := range trovati {
			return p[1] + " " + x + p[3] + p[4]
		}
		return m
	})
}

// varianteConfuse: la parola con UNA confusione tipica dell'OCR (tabella confusioni), anche se
// la variante non esiste (la "e" finale si aggiunge dopo).
func varianteConfuse(w string) []string {
	var out []string
	for _, c := range confusioni {
		for i := 0; i+len(c[0]) <= len(w); i++ {
			if w[i:i+len(c[0])] == c[0] {
				out = append(out, w[:i]+c[1]+w[i+len(c[0]):])
			}
		}
	}
	return out
}
