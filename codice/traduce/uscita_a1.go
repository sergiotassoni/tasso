package traduce

// Pulizia dell'uscita del traduttore (prova finale del 26/11/2026, 6 giochi mai visti): frasi
// tagliate in fondo, titoli dei video e dei giochi, parole ripetute inventate dal traduttore.

import (
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
	for _, w := range strings.FieldsFunc(low, func(r rune) bool { return r == ' ' || r == '[' || r == ']' || r == '(' || r == ')' || r == '|' || r == '-' }) {
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

// nomeGioco: un titolo corto (2-6 parole, tutte con la maiuscola o tutto maiuscolo, senza
// punteggiatura da frase) con dentro una parola di fantasia fatta di due parole inglesi attaccate
// ("Dawnwalker", "Shadowfell", "Ironforge"): è il nome di un gioco o di un posto, non si traduce
// ("Il sangue di Dawnwalker" nell'elenco delle missioni). "Follow Lunka" no: Lunka è un nome
// semplice e la scritta è un comando.
func nomeGioco(t string) bool {
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
