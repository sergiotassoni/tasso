package traduce

// Pulizia dell'uscita del traduttore (prova finale del 26/11/2026, 6 giochi mai visti): frasi
// tagliate in fondo, titoli dei video e dei giochi, parole ripetute inventate dal traduttore.

import (
	"math"
	"regexp"
	"strings"
)

// motivoFraseTroncata: la riga finisce con una parola che vuole il seguito ("...forward without"):
// la riga sotto c'è ma non è stata letta. Tradotta da sola si vedrebbe mezza frase italiana
// sopra una riga inglese ("Un rapido tocco di ti consente Shadowstep in avanti senza" / "aiming.").
const motivoFraseTroncata = "riga incompleta (la frase continua sotto)"

// fraseTroncata: "" se la riga è finita.
func fraseTroncata(b Blocco) string {
	if b.Righe > 1 || b.Affid == letturaCatalog {
		return ""
	}
	t := strings.TrimSpace(b.Testo)
	w := strings.Fields(t)
	if len(w) < 4 || strings.ContainsAny(t[len(t)-1:], ".!?:;…\"'") {
		return ""
	}
	u := strings.ToLower(strings.Trim(w[len(w)-1], ",\"'"))
	if paroleSospese[u] {
		return motivoFraseTroncata
	}
	return ""
}

// motivoTitoloVideo: il titolo del video nel lettore (YouTube, Twitch) o il nome del canale: non
// è testo del gioco, resta com'è (si vedeva "007 PRIMO gameplay di gioco LIGHT Walkfirough Parte 1").
const motivoTitoloVideo = "titolo del video o nome del canale (resta uguale)"

// paroleVideo: parole tipiche dei titoli dei video di gameplay.
var paroleVideo = map[string]bool{"gameplay": true, "walkthrough": true, "walktrough": true, "playthrough": true,
	"commentary": true, "longplay": true, "trailer": true, "4k": true, "60fps": true, "1080p": true, "1440p": true,
	"2160p": true, "120fps": true, "ps5": true, "ps4": true, "xbox": true, "rtx": true, "hdr": true, "letsplay": true,
	"episode": true, "ep": true, "ultra": true, "pro": true}

// titoloVideo: la scritta è il titolo di un video di gameplay ("ONE MORE DONUT - Full Gameplay
// Walkthrough Part 1 [FULL GAME] No Commentary"): almeno due segni tipici, contando anche le
// parole lette male ("Walkfirough", "Comnmtary").
func titoloVideo(t string) bool {
	low := strings.ToLower(t)
	n := 0
	for _, w := range strings.FieldsFunc(low, func(r rune) bool {
		return r == ' ' || r == '[' || r == ']' || r == '(' || r == ')' || r == '|' || r == '-'
	}) {
		w = strings.Trim(w, ".,:;!?'\"")
		if paroleVideo[w] || strings.HasPrefix(w, "walk") && len(w) >= 9 || strings.HasPrefix(w, "comm") && len(w) >= 8 ||
			strings.HasSuffix(w, "fps") && len(w) <= 6 || (len(w) >= 2 && len(w) <= 4 && (strings.HasSuffix(w, "k") && strings.Trim(w[:len(w)-1], "0123456789") == "")) {
			n++
		}
	}
	for _, f := range []string{"no commentary", "full game", "part 1", "part 2", "part 3", "parte 1", "let's play", "lets play", "no comm"} {
		if strings.Contains(low, f) {
			n++
		}
	}
	return n >= 2
}

// motivoNomeGioco: titolo del gioco o nome di fantasia ("The Blood of Dawnwalker"): resta com'è.
const motivoNomeGioco = "titolo del gioco o nome di fantasia (resta uguale)"

// titoloDelGioco: un titolo corto (2-6 parole, tutte con la maiuscola o tutto maiuscolo, senza
// punteggiatura da frase) con dentro una parola di fantasia fatta di due parole inglesi attaccate
// ("Dawnwalker", "Shadowfell", "Ironforge"): è il nome di un gioco o di un posto, non si traduce
// ("Il sangue di Dawnwalker" nell'elenco delle missioni). "Follow Lunka" no: Lunka è un nome
// semplice e la scritta è un comando.
func titoloDelGioco(t string) bool {
	t = strings.TrimSpace(t)
	if t == "" || strings.ContainsAny(t, ".!?,;:") {
		return false
	}
	w := strings.Fields(t)
	if len(w) < 2 || len(w) > 6 {
		return false
	}
	minuscoleOk := map[string]bool{"of": true, "the": true, "and": true, "in": true, "on": true, "a": true, "an": true, "for": true, "to": true}
	fantasia := false
	for _, x := range w {
		r := []rune(x)
		if minuscoleOk[strings.ToLower(x)] {
			continue
		}
		if !(r[0] >= 'A' && r[0] <= 'Z') && !(r[0] >= '0' && r[0] <= '9') {
			return false
		}
		if nomeComposto(strings.ToLower(x)) {
			fantasia = true
		}
	}
	return fantasia
}

// nomeComposto: parola che non esiste in inglese, di almeno 7 lettere, fatta di due parole inglesi.
func nomeComposto(w string) bool {
	return len(w) >= 7 && !esiste(w) && parolaComposta(w)
}

// ---------------------------------------------------------------- frasi ripetute

// frasiDi: le frasi di un testo, ognuna con la sua punteggiatura finale ("Presto! Seguimi!").
func frasiDi(s string) []string {
	var out []string
	inizio := 0
	r := []rune(s)
	for i := 0; i < len(r); i++ {
		if strings.ContainsRune(".!?…", r[i]) {
			j := i
			for j+1 < len(r) && strings.ContainsRune(".!?…\"»", r[j+1]) {
				j++
			}
			if j+1 == len(r) || r[j+1] == ' ' {
				if f := strings.TrimSpace(string(r[inizio : j+1])); f != "" {
					out = append(out, f)
				}
				inizio = j + 1
				i = j
			}
		}
	}
	if f := strings.TrimSpace(string(r[inizio:])); f != "" {
		out = append(out, f)
	}
	return out
}

func chiaveRipetuta(f string) string {
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(f), ".!?…\"» "))
}

// senzaRipetizioni: toglie le frasi ripetute che il traduttore veloce aggiunge da solo ("Quick!
// Follow me!" -> "Presto! Seguimi! Seguimi!", "Come on!" -> "Forza! Forza!", "What?" -> "Cosa?
// Cosa?", "What's going on?" -> "Che succede? Cosa?"). Solo se l'inglese ha meno frasi e non è
// ripetuto anche lui ("No, no!" resta).
func senzaRipetizioni(en, it string) string {
	fe, fi := frasiDi(strings.TrimSpace(en)), frasiDi(strings.TrimSpace(it))
	if len(fi) <= len(fe) || len(fi) < 2 {
		return it
	}
	ripetuteEn := false
	cosaEn := false
	for i, f := range fe {
		if i > 0 && chiaveRipetuta(f) == chiaveRipetuta(fe[i-1]) {
			ripetuteEn = true
		}
		if k := chiaveRipetuta(f); k == "what" || k == "huh" || k == "eh" {
			cosaEn = true
		}
	}
	parole := strings.FieldsFunc(strings.ToLower(en), func(r rune) bool { return !(r >= 'a' && r <= 'z') && r != '\'' })
	for i := 1; i < len(parole); i++ {
		if parole[i] == parole[i-1] {
			ripetuteEn = true // "Hurry, hurry!": la ripetizione c'è anche in inglese
		}
	}
	if ripetuteEn {
		return it
	}
	var out []string
	for i, f := range fi {
		if i > 0 && chiaveRipetuta(f) == chiaveRipetuta(fi[i-1]) && len(fi)-(i-len(out)) > len(fe) {
			continue // la stessa frase due volte di fila
		}
		out = append(out, f)
	}
	// "Cosa?" in più da solo (l'inglese non ha un "What?" da solo)
	if len(out) > len(fe) && !cosaEn {
		var tenute []string
		tolta := false
		for _, f := range out {
			if !tolta && chiaveRipetuta(f) == "cosa" && len(out)-1 >= len(fe) {
				tolta = true
				continue
			}
			tenute = append(tenute, f)
		}
		out = tenute
	}
	if len(out) == len(fi) {
		return it
	}
	return strings.Join(out, " ")
}

// frasiPronteTutte: una scritta di più frasi pronte una dopo l'altra (due scelte di dialogo lette
// come un paragrafo: "Nice to meet you! What's your first name?"): ogni frase dal catalogo.
// Solo se TUTTE le frasi sono pronte.
func frasiPronteTutte(s string) (string, bool) {
	f := frasiDi(strings.TrimSpace(s))
	if len(f) < 2 || len(f) > 4 {
		return "", false
	}
	var out []string
	for _, x := range f {
		v, ok := fraseMenu(x)
		if !ok {
			return "", false
		}
		out = append(out, v)
	}
	return strings.Join(out, " "), true
}

// reDataGioco: "4/7 Tue", "4/7 Tug" (il giorno letto male): data del calendario del gioco, resta.
var reDataGioco = regexp.MustCompile(`^\W{0,3}\d{1,2}\s?/\s?\d{1,2}\b`)

// ---------------------------------------------------------------- barra dei comandi

// tastoDiComando: la parola è il nome di un tasto ("TAB", "ENTER", "ESC", "MOUSE", "LMB", "F1").
func tastoDiComando(w string) bool {
	u := strings.ToUpper(strings.Trim(w, "[]()<>:"))
	return nomiTastiFissi[u] || nomiTastiParole[u] || u == "MOUSE" || u == "SPACE" || reTastoSicuro.MatchString(u) ||
		u == "←" || u == "→" || u == "↑" || u == "↓"
}

// voceDiBarra: una voce corta di una barra dei comandi in fondo ("TAB group", "ENTER buy",
// "select", "back"): al massimo 3 parole.
func voceDiBarra(b Blocco) bool {
	w := strings.Fields(strings.TrimSpace(b.Testo))
	return righeSchermo(b) == 1 && len(w) >= 1 && len(w) <= 3 && len(strings.TrimSpace(b.Testo)) <= 24
}

func conTasto(b Blocco) bool {
	for _, w := range strings.Fields(b.Testo) {
		if tastoDiComando(w) {
			return true
		}
	}
	return false
}

// barraConIngleseAccanto: p è una voce della barra dei comandi ("← → select · TAB group · ENTER
// buy · MOUSE rotate · ESC back") e un'altra voce della stessa barra resta in inglese: la barra è
// un gruppo, tutta tradotta o tutta in inglese (prova finale, One More Donut: "select" in inglese
// accanto a "gruppo TAB" e "Indietro").
func barraConIngleseAccanto(p Blocco, inglesi []Blocco) bool {
	if !voceDiBarra(p) {
		return false
	}
	for _, e := range inglesi {
		if e.X == p.X && e.Y == p.Y && e.W == p.W && e.H == p.H {
			continue
		}
		if !voceDiBarra(e) || !lineaSimile(p, e) || !(conTasto(p) || conTasto(e)) {
			continue
		}
		em := math.Max(math.Max(p.Em, e.Em), float64(max(p.H, e.H)))
		sx, dx := p, e
		if e.X < p.X {
			sx, dx = e, p
		}
		if float64(dx.X-(sx.X+sx.W)) <= 8*em {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- soldi e voci con prezzo

var (
	reMancano   = regexp.MustCompile(`(?i)^short by\s+([$€£]\s?[\d.,]+)$`)
	reVoceSoldi = regexp.MustCompile(`^(\pL[\pL ]{1,20}?)\s*([·•|:\-])\s*([$€£]\s?[\d.,]+)$`)
)

// soldiIt: "$1,621" -> "$1.621" (il separatore delle migliaia all'italiana).
func soldiIt(s string) string {
	if strings.Count(s, ",") >= 1 && !strings.Contains(s, ".") {
		return strings.ReplaceAll(s, ",", ".")
	}
	return s
}

// prontaConSoldi: "Short by $1,621" -> "Mancano $1.621", "BUY · $2,000" -> "COMPRA · $2.000".
func prontaConSoldi(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if m := reMancano.FindStringSubmatch(s); m != nil {
		return "Mancano " + soldiIt(m[1]), true
	}
	if m := reVoceSoldi.FindStringSubmatch(s); m != nil {
		v, ok := fraseMenu(strings.TrimSpace(m[1]))
		if !ok {
			return "", false
		}
		if tuttoMaiuscolo(m[1]) {
			v = strings.ToUpper(v)
		}
		return v + " " + m[2] + " " + soldiIt(m[3]), true
	}
	return "", false
}

// ---------------------------------------------------------------- icona letta come lettera

// reLetteraIcona: una lettera sola (con un apice) davanti a un numero o a un segno ("'t +5%
// damage": la freccia ↑ letta come "t"): è un'icona, non una parola.
var reLetteraIcona = regexp.MustCompile(`^['’‘]?\pL['’‘]?\s+([+\-−]?\d)`)

// togliLetteraIcona: toglie l'icona letta come lettera dall'inizio della scritta.
func togliLetteraIcona(bl []Blocco) []Blocco {
	for i := range bl {
		t := strings.TrimSpace(bl[i].Testo)
		m := reLetteraIcona.FindStringSubmatchIndex(t)
		if m == nil {
			continue
		}
		w := strings.Fields(t)[0]
		if strings.Trim(w, "'’‘") == "a" || strings.Trim(w, "'’‘") == "I" || strings.Trim(w, "'’‘") == "A" {
			continue // "a 5% bonus", "I 2": parole vere
		}
		resto := strings.TrimSpace(t[m[2]:])
		if bl[i].W > 0 && len([]rune(t)) > 0 {
			via := bl[i].W * (len([]rune(t)) - len([]rune(resto))) / len([]rune(t))
			bl[i].X += via
			bl[i].W -= via
		}
		bl[i].Testo = resto
		if bl[i].Grezzo == t {
			bl[i].Grezzo = resto
		}
	}
	return bl
}

// inBarraComandi: p è una voce di una barra dei comandi (sulla stessa riga ci sono almeno altre
// due voci corte con il nome di un tasto): "select" minuscolo lì non è un pezzo di frase senza
// testa, la sua icona (← →) non si legge.
func inBarraComandi(p Blocco, tutte []Blocco) bool {
	if !voceDiBarra(p) {
		return false
	}
	n := 0
	for _, q := range tutte {
		if q.X == p.X && q.Y == p.Y && q.W == p.W && q.H == p.H {
			continue
		}
		if voceDiBarra(q) && conTasto(q) && lineaSimile(p, q) {
			n++
		}
	}
	return n >= 2
}
