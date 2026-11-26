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
	// "Press [icona] to sprint", "Hold to sprint": il motore lascia "sprint" in inglese
	{inglese: rx(`(?i)\bto sprint\b`), da: rx(`(?i)\bper (?:lo |fare (?:lo |uno )?)?sprint(?:are)?\b`), a: "per scattare"},
	{inglese: rx(`(?i)\b(press|hold|tap|click|toggle)\b[^.!?]*\bsprint\b`), da: rx(`(?i)\bsprint\b`), a: "scatto"},
	// comandi di combattimento
	{inglese: rx(`(?i)\btake cover\b`), da: rx(`(?i)\bprenditi copertura\b`), a: "mettiti al riparo"},
	{inglese: rx(`(?i)\bfast travel\b`), da: rx(`(?i)\bviaggiare velocemente\b`), a: "usare il viaggio rapido"},
	// guida (banco sui video, 25/11: "a sharper turn" -> "una svolta più nitina", "un giro più Sharp")
	{inglese: rx(`(?i)\bsharper turns?\b`), da: rx(`(?i)\b(una|la) (svolta|curva|virata) più (nitida|nitina|netta|affilata|acuta|sharp)\b`), a: "${1} curva più stretta"},
	{inglese: rx(`(?i)\bsharper turns?\b`), da: rx(`(?i)\b(un|il) giro più (nitido|nitino|netto|affilato|acuto|sharp)\b`), a: "una curva più stretta"},
	{inglese: rx(`(?i)\bwhile steering\b`), da: rx(`(?i)\bmentre si guida\b`), a: "mentre si sterza"},
	// prova finale (26/11, 6 giochi mai visti): errori visti
	{inglese: rx(`(?i)\bhealth segments?\b`), da: rx(`(?i)\b(del|il|un|al|dal|nel) segmento (sanitario|di salute|della salute)\b`), f: versoTacca},
	{inglese: rx(`(?i)\bhealth segments?\b`), da: rx(`(?i)\bsegment[oi] (sanitari[oi]|di salute|della salute)\b`), a: "tacca di salute"},
	{inglese: rx(`(?i)\btarget lock\b`), da: rx(`(?i)\b(la )?serratura (di destinazione|del bersaglio|bersaglio|di bersaglio)\b`), a: "il blocco del bersaglio"},
	{inglese: rx(`(?i)\bactivation charges?\b`), da: rx(`(?i)è necessario attivare le spese\b`), a: "servono le cariche di attivazione"},
	{inglese: rx(`(?i)\bactivation charges\b`), da: rx(`(?i)\b(spese|soste|cariche) (di|delle) attivazion[ei]\b`), a: "cariche di attivazione"},
	{inglese: rx(`(?i)\bfeed(ing|s)? you intel\b`), da: rx(`(?i)\bti (darà|dà|sta dando|starà dando) da mangiare\b`), a: "ti passerà le informazioni"},
	{inglese: rx(`(?i)\bprincipal\b`), evita: rx(`(?i)\bprincipal (component|amount|investigator|character|actor|engineer|cast)\b`), da: rx(`(?i)\bprincipale\b`), a: "preside"},
	{inglese: rx(`(?i)\bjournal\b`), evita: rx(`(?i)\b(newspaper|daily journal|wall street)\b`), da: rx(`(?i)\bgiornale\b`), a: "diario"},
	{inglese: rx(`(?i)\bdeploy\b`), da: rx(`(?i)\bdistribuisci\b`), a: "attiva"},
	{inglese: rx(`(?i)\bget in there\b`), da: rx(`(?i)\bvoglio che tu sia lì dentro\b`), a: "voglio che tu entri lì"},
	{inglese: rx(`(?i)\brigged\b`), evita: rx(`(?i)\b(game|election|match|vote|race|lottery)\b`), da: rx(`(?i)\btruccat(o|a|i|e)\b`), a: "minat$1"},
	{inglese: rx(`(?i)\bcatch on\b`), da: rx(`(?i)\bprenderai piede\b`), a: "ci arriverai"},
	{inglese: rx(`(?i)\bplaces to be\b`), da: rx(`(?i)\bposti dove stare\b`), a: "posti dove andare"},
	{inglese: rx(`(?i)\bstory, fair,? and challenging\b`), da: rx(`(?i)\bsu storie, fiere e difficoltà impegnative\b`), a: "nelle difficoltà Storia, Equa e Impegnativa"},
	{inglese: rx(`(?i)\bclassroom assignments?\b`), da: rx(`(?i)\b(gli )?incarichi in classe\b`), a: "le assegnazioni delle classi"},
	{inglese: rx(`(?i)\b(pretty|very|so|really) close\b`), da: rx(`(?i)\bdevi essere molto vicin[oa]\b`), a: "dovete essere molto legati"},
	{inglese: rx(`(?i)\bhow close are you\b`), da: rx(`(?i)\bquanto siete vicini\b`), a: "quanto siete legati"},
	{inglese: rx(`(?i)\bwhat's the deal between\b`), da: rx(`(?i)\bqual è l'accordo tra\b`), a: "cosa c'è tra"},
	{inglese: rx(`(?i)^hey, it's \pL`), da: rx(`(?i)^ehi, sono\b`), a: "Ehi, è"},
	{inglese: rx(`(?i)\bnew kid\b`), da: rx(`(?i)\bessere nel nuovo ragazzo\b`), a: "essere il nuovo arrivato"},
	{inglese: rx(`(?i)\bfingers and toes\b`), da: rx(`(?i)\bdita e dita dei piedi\b`), a: "dita delle mani e dei piedi"},
	{inglese: rx(`(?i)\bspecial issue\b`), da: rx(`(?i)\bun problema special[ei]?\b`), a: "una dotazione speciale"},
	{inglese: rx(`(?i)\bdodging the question\b`), da: rx(`(?i)\bscuotendo la domanda\b`), a: "evitando la domanda"},
	{inglese: rx(`(?i)\btaking in the view\b`), da: rx(`(?i)\bsto solo toccando la vista\b`), a: "mi godo solo il panorama"},
	{inglese: rx(`(?i)\bMs\. \pL`), da: rx(`(?i)\b(la )?signora\. (\p{Lu})`), a: "la signora $2"},
	{inglese: rx(`(?i)\byou still got it\b`), da: rx(`(?i)\bhai ancora capito\b`), a: "ce l'hai ancora"},
	{inglese: rx(`(?i)\bget up those stairs\b`), da: rx(`(?i)\balzate quelle scale\b`), a: "su per quelle scale"},
	{inglese: rx(`(?i)\biron\b`), evita: rx(`(?i)\biron(ing|ed)?\b.*\b(clothes|shirt)\b|\bironing\b`), da: rx(`(?i)\bferro da stiro\b`), a: "ferro"},
	{inglese: rx(`(?i)\b(this|per|the|your|next|night|work) shift\b`), evita: rx(`(?i)\b(press|hold|key)\b`), da: rx(`(?i)\b(questo|quel|il|lo|del|dello|nel|nello) (spostamento|cambiamento)\b`), f: versoTurno},
	// obiettivi delle missioni (prove 26/11 12:56 e 15:56, giochi diversi)
	// "Head to the faculty office" -> "Testa all'ufficio facoltà": "head" è il verbo andare
	{inglese: rx(`\b(?:Head|HEAD)(?:ing|ING)? (?i:(?:back |over |straight )?(?:to|towards?|into|inside))\b`), evita: rx(`(?i)\bhead (to|over) (toe|foot|heels|head)\b`), da: rx(`\b(?:Testa|TESTA) ((?i:a|al|allo|alla|ai|agli|alle|all'|verso|nel|nello|nella|nei|negli|nelle))\b`), f: versoVai},
	// "Walk your siblings to the river" -> "Cammina i tuoi fratelli": accompagnare qualcuno
	{inglese: rx(`(?i)\bwalk (your|the|my|his|her|their|our)\b[^.!?]*\bto\b`), evita: rx(`(?i)\bwalk (the|this|that) (path|line|road|plank|street|streets|halls?|earth|world|talk|way|walk)\b`), da: rx(`(?i)\bcammina (i|il|lo|la|le|gli|l'|tuo|tua|tuoi|tue|suo|sua|suoi|sue|mio|mia|miei|mie|loro|nostri|nostro|nostra|nostre)\b`), f: func(s string) string { _, r, _ := strings.Cut(s, " "); return "accompagna " + r }},
	// "ESCAPE THE CAMP" -> "FUGGISCI NEL CAMPO": scappare DA un posto (non "into": lì è giusto "nel")
	{inglese: rx(`(?i)^\W*escape (from )?(the|this|that|your)\b`), da: rx(`(?i)\b(fuggi|fuggisci|scappa) (nel|nello|nella|nei|negli|nelle|in|al|allo|alla|ai|agli|alle|il|lo|la|i|gli|le)\b`), f: versoDa},
	{inglese: rx(`(?i)\bescape\b`), da: rx(`(?i)\bfuggisci\b`), a: "fuggi"},
	// "CARGO HOLD" -> "TENUTA DI CARICO": la stiva della nave
	{inglese: rx(`(?i)\bcargo holds?\b`), da: rx(`(?i)\b((?:la|nella|della|alla|dalla|sulla) )?tenuta di carico\b`), a: "${1}stiva"},
	{inglese: rx(`(?i)\bcargo holds\b`), da: rx(`(?i)\btenute di carico\b`), a: "stive"},
	{inglese: rx(`(?i)^hold to\b`), da: rx(`(?i)^aspetta di\b`), a: "Tieni premuto per"},
	{inglese: rx(`(?i)^hold until\b`), da: rx(`(?i)^aspetta che( non)?\b`), a: "Tieni premuto finché"},
	{inglese: rx(`(?i)\bstagger(ed)?\b`), da: rx(`(?i)\bscaglionat(o|a|i|e)\b`), a: "stordit$1"},
	{inglese: rx(`(?i)\bherd animals?\b`), da: rx(`(?i)\banimali da pane\b`), a: "animali da branco"},
}

// versoVai: "Testa all'" -> "vai all'" (la maiuscola la rimette chi chiama).
func versoVai(s string) string {
	_, resto, _ := strings.Cut(s, " ")
	return "vai " + resto
}

// versoDa: "fuggisci nel" -> "fuggi dal", "scappa nella" -> "scappa dalla" ("Escape the camp").
func versoDa(s string) string {
	w := strings.Fields(s)
	verbo := w[0]
	if strings.EqualFold(verbo, "fuggisci") {
		verbo = "fuggi"
	}
	art := map[string]string{"nel": "dal", "nello": "dallo", "nella": "dalla", "nei": "dai", "negli": "dagli",
		"nelle": "dalle", "in": "da", "al": "dal", "allo": "dallo", "alla": "dalla", "ai": "dai", "agli": "dagli",
		"alle": "dalle", "il": "dal", "lo": "dallo", "la": "dalla", "i": "dai", "gli": "dagli", "le": "dalle"}
	return verbo + " " + art[strings.ToLower(w[1])]
}

// versoTurno: "questo spostamento" -> "questo turno" (il turno di lavoro, "this shift").
func versoTurno(s string) string {
	w := strings.Fields(s)
	art := map[string]string{"lo": "il", "dello": "del", "nello": "nel", "quel": "quel"}
	a := w[0]
	if x, ok := art[strings.ToLower(a)]; ok {
		a = x
	}
	return a + " turno"
}

// versoTacca: "del segmento sanitario" -> "della tacca di salute".
func versoTacca(s string) string {
	art := map[string]string{"del": "della", "il": "la", "un": "una", "al": "alla", "dal": "dalla", "nel": "nella"}
	w := strings.Fields(s)
	a := art[strings.ToLower(w[0])]
	if a == "" {
		a = w[0]
	}
	return a + " tacca di salute"
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
			if len([]rune(trovato)) > 1 && trovato == strings.ToUpper(trovato) && trovato != strings.ToLower(trovato) {
				return strings.ToUpper(nuovo) // "C) GIORNALE" -> "C) DIARIO"
			}
			if p := []rune(trovato); len(p) > 0 && unicode.IsUpper(p[0]) {
				q := []rune(nuovo)
				q[0] = unicode.ToUpper(q[0])
				nuovo = string(q)
			}
			return nuovo
		})
	}
	return senzaRipetizioni(en, strings.TrimSpace(ritoccaItaliano(it)))
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
