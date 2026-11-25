package traduce

// Termini da gioco dentro le frasi. Il motore veloce non conosce il glossario e in mezzo alle
// frasi sbaglia alcune parole sempre allo stesso modo ("magazine" -> "rivista" invece di
// "caricatore"). Qui ci sono solo errori VISTI nel collaudo dal vivo delle 1000 frasi
// (22/11/2026, Mozilla in WebView2 sul PC di Sergio), corretti solo quando la frase inglese
// conferma il senso da gioco. Niente regole "a indovinare": se il senso non è sicuro, resta com'è.

import (
	"regexp"
	"strings"
	"unicode"
)

type regolaTermine struct {
	inglese *regexp.Regexp      // la frase inglese deve corrispondere
	evita   *regexp.Regexp      // se la frase inglese corrisponde a questo, niente correzione
	da      *regexp.Regexp      // cosa cercare nell'italiano
	a       string              // con cosa sostituirlo (rispetta la maiuscola iniziale)
	f       func(string) string // in alternativa ad a: sostituzione calcolata (articoli al maschile)
}

// articoli e aggettivi femminili -> maschili, per "la rivista" -> "il caricatore"
var alMaschileArt = map[string]string{
	"la": "il", "una": "un", "l'ultima": "l'ultimo", "un'altra": "un altro", "nuova": "nuovo", "ultima": "ultimo", "altra": "altro",
	"questa": "questo", "quella": "quel", "della": "del", "nella": "nel", "sulla": "sul", "alla": "al", "dalla": "dal",
	"le": "i", "delle": "dei", "nelle": "nei", "sulle": "sui", "alle": "ai", "dalle": "dai", "altre": "altri", "ultime": "ultimi", "nuove": "nuovi",
}

func alMaschile(nome string) func(string) string {
	return func(s string) string {
		inizio := s[:len(s)-len(strings.TrimLeft(s, " "))]
		w := strings.Fields(s)
		var out []string
		for _, x := range w[:len(w)-1] { // articolo e aggettivo prima del nome
			if m, ok := alMaschileArt[strings.ToLower(x)]; ok {
				x = m
			}
			out = append(out, x)
		}
		// "un altro" si scrive staccato, "l'ultimo" attaccato: già così nella tabella
		return inizio + strings.Join(append(out, nome), " ")
	}
}

// versoElmo: "il casco di ferro" -> "l'elmo di ferro", "del casco" -> "dell'elmo".
var articoliElmo = map[string]string{"il": "l'", "un": "un ", "del": "dell'", "al": "all'", "dal": "dall'", "nel": "nell'", "sul": "sull'", "col": "con l'"}

func versoElmo(s string) string {
	w := strings.Fields(s)
	art := ""
	if a, ok := articoliElmo[strings.ToLower(w[0])]; ok && len(w) > 2 {
		art, w = a, w[1:]
	}
	nuovo := art + "elmo " + strings.Join(w[1:], " ") // w[0] è "casco"
	if s == strings.ToUpper(s) {
		nuovo = strings.ToUpper(nuovo)
	}
	return nuovo
}

var articoliElmi = map[string]string{"i": "gli", "dei": "degli", "ai": "agli", "dai": "dagli", "nei": "negli", "sui": "sugli", "coi": "con gli"}

func versoElmi(s string) string {
	w := strings.Fields(s)
	art := ""
	if a, ok := articoliElmi[strings.ToLower(w[0])]; ok && len(w) > 2 {
		art, w = a+" ", w[1:]
	}
	nuovo := art + "elmi " + strings.Join(w[1:], " ")
	if s == strings.ToUpper(s) {
		nuovo = strings.ToUpper(nuovo)
	}
	return nuovo
}

func rx(s string) *regexp.Regexp { return regexp.MustCompile(s) }

var regoleTermini = []regolaTermine{
	// elmi delle armature: "casco di ferro" -> "elmo di ferro" (nei giochi di fantascienza "casco" va bene)
	{inglese: rx(`(?i)\bhelmets?\b`), da: rx(`(?i)\b((?:il|un|del|al|dal|nel|sul|col) )?casco (di ferro|d'acciaio|di cuoio|d'argento|d'oro|di bronzo|di rame|d'osso|di cristallo|di diamante|di mithril|di maglia|di drago|del cavaliere|da cavaliere|vichingo)`), f: versoElmo},
	{inglese: rx(`(?i)\bhelmets?\b`), da: rx(`(?i)\b((?:i|dei|ai|dai|nei|sui|coi) )?caschi (di ferro|d'acciaio|di cuoio|d'argento|d'oro|di bronzo|di rame|d'osso|di cristallo|di diamante|di mithril|di maglia|di drago)`), f: versoElmi},
	// armi: caricatore
	{inglese: rx(`(?i)\bmag(azine)?s?\b`), evita: rx(`(?i)\b(read|reading|article|issue|cover|page|subscribe|newspaper)\b`), da: rx(`(?i)\b((?:(?:le|delle|nelle|sulle|alle|dalle) )?(?:(?:altre|ultime|nuove) )?)riviste\b`), f: alMaschile("caricatori")},
	{inglese: rx(`(?i)\bmag(azine)?s?\b`), evita: rx(`(?i)\b(read|reading|article|issue|cover|page|subscribe|newspaper)\b`), da: rx(`(?i)(?:^|\b|\s)((?:(?:la|una|questa|quella|della|nella|sulla|alla|dalla) )?(?:(?:l'ultima|un'altra|nuova|ultima|altra) )?)rivista\b`), f: alMaschile("caricatore")},
	// fantascienza
	{inglese: rx(`(?i)\bairlocks?\b`), da: rx(`(?i)\bcamera d'aria\b`), a: "camera stagna"},
	{inglese: rx(`(?i)\bairlocks?\b`), da: rx(`(?i)\bcamere d'aria\b`), a: "camere stagne"},
	{inglese: rx(`(?i)\bcargo bay\b`), da: rx(`(?i)\b((?:la|nella|della|alla|dalla|sulla) )?baia di carico\b`), a: "${1}stiva"},
	{inglese: rx(`(?i)\b(being hailed|hailed by)\b`), da: rx(`(?i)\bsiamo (?:stati )?salutati da\b`), a: "riceviamo una chiamata da"},
	{inglese: rx(`(?i)\bis hailing us\b`), da: rx(`(?i)\bci sta salutando\b`), a: "ci sta chiamando"},
	// fantasy / sopravvivenza
	{inglese: rx(`(?i)\btreasure chests?\b`), da: rx(`(?i)\b(il |nel |dal |al )?petto del tesoro\b`), a: "${1}forziere"},
	{inglese: rx(`(?i)\binnkeepers?\b`), da: rx(`(?i)\bil locande\b`), a: "il locandiere"},
	{inglese: rx(`(?i)\bfresh ?water\b`), da: rx(`(?i)\bacqua fresca\b`), a: "acqua dolce"},
	{inglese: rx(`(?i)\bscouts\b`), evita: rx(`(?i)\bboy scouts?\b`), da: rx(`(?i)\bscout\b`), a: "esploratori"},
	// menu e comandi
	{inglese: rx(`(?i)\bauto-?sav(e|ing)\b`), da: rx(`(?i)\b(auto-?sv\pL*|autosalva\pL*|autosave)\b`), a: "salvataggio automatico"},
	{inglese: rx(`(?i)\b(press|hold|tap|release|use)\b[^.!?]*\bshift\b`), da: rx(`(?i)\b(il )?(tasto )?cambio\b`), a: "Maiusc"},
	{inglese: rx(`(?i)\bshift\b[^.!?]*\bto (sprint|run)\b`), da: rx(`(?i)\bper sprint\b`), a: "per scattare"},
	// comandi di combattimento
	{inglese: rx(`(?i)\btake cover\b`), da: rx(`(?i)\bprenditi copertura\b`), a: "mettiti al riparo"},
	{inglese: rx(`(?i)\bfast travel\b`), da: rx(`(?i)\bviaggiare velocemente\b`), a: "usare il viaggio rapido"},
	// guida (banco sui video, 25/11: "a sharper turn" -> "una svolta più nitina", "un giro più Sharp")
	{inglese: rx(`(?i)\bsharper turns?\b`), da: rx(`(?i)\b(una|la) (svolta|curva|virata) più (nitida|nitina|netta|affilata|acuta|sharp)\b`), a: "${1} curva più stretta"},
	{inglese: rx(`(?i)\bsharper turns?\b`), da: rx(`(?i)\b(un|il) giro più (nitido|nitino|netto|affilato|acuto|sharp)\b`), a: "una curva più stretta"},
	{inglese: rx(`(?i)\bwhile steering\b`), da: rx(`(?i)\bmentre si guida\b`), a: "mentre si sterza"},
}

// correggiTerminiFrase applica le regole a una traduzione di una frase inglese.
func correggiTerminiFrase(en, it string) string {
	for _, r := range regoleTermini {
		if !r.inglese.MatchString(en) || (r.evita != nil && r.evita.MatchString(en)) {
			continue
		}
		it = r.da.ReplaceAllStringFunc(it, func(trovato string) string {
			var nuovo string
			if r.f != nil {
				nuovo = r.f(trovato)
			} else {
				nuovo = r.da.ReplaceAllString(trovato, r.a)
			}
			if p := []rune(trovato); len(p) > 0 && unicode.IsUpper(p[0]) {
				q := []rune(nuovo)
				q[0] = unicode.ToUpper(q[0])
				nuovo = string(q)
			}
			return nuovo
		})
	}
	return strings.TrimSpace(ritoccaItaliano(it))
}

// ritoccaItaliano: errori di scrittura tipici del motore veloce, sempre uguali e sicuri da
// correggere subito (senza aspettare l'IA): accenti scritti con l'apostrofo ("E'", "pero'",
// "C'e'"), "po '" staccato, trattino di dialogo all'inizio. Visti nel collaudo delle 1000 frasi.
var (
	reAccentoApostrofo = regexp.MustCompile(`(?i)\b(c'e|perche|poiche|affinche|benche|giacche|pero|piu|puo|gia|cosi|giu|meta|citta|verita|virtu|liberta|eta|qualita|novita|tribu|caffe|lunedi|martedi|mercoledi|giovedi|venerdi|e)'(\W|$)`)
	accentati          = map[string]string{
		"c'e": "c'è", "perche": "perché", "poiche": "poiché", "affinche": "affinché", "benche": "benché", "giacche": "giacché",
		"pero": "però", "piu": "più", "puo": "può", "gia": "già", "cosi": "così", "giu": "giù", "meta": "metà", "citta": "città",
		"verita": "verità", "virtu": "virtù", "liberta": "libertà", "eta": "età", "qualita": "qualità", "novita": "novità",
		"tribu": "tribù", "caffe": "caffè", "lunedi": "lunedì", "martedi": "martedì", "mercoledi": "mercoledì",
		"giovedi": "giovedì", "venerdi": "venerdì", "e": "è",
	}
	rePoStaccato = regexp.MustCompile(`\bpo '`)
	// "servira'" -> "servirà", "li'" -> "lì": vocale finale con apostrofo seguita da spazio o punteggiatura
	reVocaleApostrofo = regexp.MustCompile(`\b(\pL*[aiouAIOU])'(\s|[.,;:!?…)"»]|$)`)
	troncamenti       = map[string]bool{"po": true, "mo": true, "di": true, "fa": true, "va": true, "sta": true, "da": true, "to": true}
	vocaleAccentata   = map[rune]rune{'a': 'à', 'i': 'ì', 'o': 'ò', 'u': 'ù', 'A': 'À', 'I': 'Ì', 'O': 'Ò', 'U': 'Ù'}
)

// dentroApici: prima di questo punto c'è un apice di apertura ('Gioco) non ancora chiuso,
// quindi l'apice che segue è una chiusura di citazione e non un accento.
func dentroApici(prima string) bool {
	if i := strings.LastIndexAny(prima, ".!?"); i >= 0 {
		prima = prima[i+1:]
	}
	aperti := 0
	r := []rune(prima)
	for i, c := range r {
		if c != '\'' {
			continue
		}
		precSpazio := i == 0 || unicode.IsSpace(r[i-1]) || strings.ContainsRune("(\"«[", r[i-1])
		segueLettera := i+1 < len(r) && unicode.IsLetter(r[i+1])
		if precSpazio && segueLettera {
			aperti++
		} else if aperti > 0 && !segueLettera {
			aperti--
		}
	}
	return aperti > 0
}

func ritoccaItaliano(s string) string {
	s = reAccentoApostrofo.ReplaceAllStringFunc(s, func(m string) string {
		sub := reAccentoApostrofo.FindStringSubmatch(m)
		parola, dopo := sub[1], sub[2]
		nuova, ok := accentati[strings.ToLower(parola)]
		if !ok {
			return m
		}
		if strings.ToLower(parola) == "e" && parola == "e" && dopo == "" {
			return m
		}
		r := []rune(parola)
		if unicode.IsUpper(r[0]) {
			q := []rune(nuova)
			q[0] = unicode.ToUpper(q[0])
			if string(q[0]) == "È" || parola == strings.ToUpper(parola) && len(r) > 1 {
				nuova = strings.ToUpper(nuova)
			} else {
				nuova = string(q)
			}
		}
		return nuova + dopo
	})
	s = rePoStaccato.ReplaceAllString(s, "po'")
	var out strings.Builder
	ultimo := 0
	for _, ix := range reVocaleApostrofo.FindAllStringSubmatchIndex(s, -1) {
		parola := []rune(s[ix[2]:ix[3]])
		basso := strings.ToLower(string(parola))
		salta := troncamenti[basso] || (len(parola) < 3 && basso != "li" && basso != "la") || dentroApici(s[:ix[3]])
		out.WriteString(s[ultimo:ix[0]])
		if salta {
			out.WriteString(s[ix[0]:ix[1]])
		} else {
			parola[len(parola)-1] = vocaleAccentata[parola[len(parola)-1]]
			out.WriteString(string(parola) + s[ix[4]:ix[5]])
		}
		ultimo = ix[1]
	}
	out.WriteString(s[ultimo:])
	s = out.String()
	if strings.HasPrefix(s, "- ") && !strings.Contains(s[2:], " - ") {
		s = s[2:]
	}
	return s
}
