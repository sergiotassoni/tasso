package traduce

// Righe lette male dall'OCR: non si mostrano. La lettura del testo di Windows
// (Windows.Media.Ocr) non dà un numero di "confidenza" per le parole: la fiducia si calcola
// qui da quello che si vede nel testo letto: parole che non esistono nel vocabolario, caratteri
// strani dentro le parole ("DiNr•ün"), maiuscole a caso ("OccWon"), pezzi di parola tagliati
// ("Voce Cha"), simboli spuri da soli ("ı‾ —"). Prima si tolgono i simboli spuri; se la riga
// resta dubbia non si traduce e non si mostra: meglio l'inglese del gioco che una parola inventata.
// Si scartano anche le righe fatte solo di nomi propri (una lista di nomi: "Sarah", "Ethan") e
// le traduzioni uguali all'originale (non sono "tradotte").

import (
	"strings"
	"unicode"
)

// segni ammessi dentro una scritta (punteggiatura normale dei menu e dei dialoghi)
const segniAmmessi = ".,!?:;'\"()%+-–—/&#·…’‘“”[]*=<>@$€£°"

// separatori che possono stare da soli tra due voci ("Nome — Valore")
var separatoriSoli = map[string]bool{"—": true, "–": true, "-": true, "·": true, "&": true, "/": true, "+": true, "|": true, ":": true}

// caratteri che l'OCR mette al posto di sporco, bordi e icone: in una scritta di un gioco
// (alfabeto latino) non servono mai
const caratteriSpuri = "ı‾¯•¬¦§¤™®©¨´`^~˜¸ˇ˘˙˚˛˝†‡¶¡¿«»‹›⁄∙◦▪▫■□●○◆◇►◄▲▼"

// togliSimboliSpuri: via i pezzi fatti solo di simboli ("ı‾", "•"), i simboli spuri ai bordi
// delle parole e i separatori rimasti doppi o in fondo.
func togliSimboliSpuri(s string) string {
	var out []string
	for _, w := range strings.Fields(s) {
		pulita := strings.Trim(w, caratteriSpuri)
		if pulita == "" {
			continue
		}
		if !haLettereONumeri(pulita) {
			if separatoriSoli[pulita] {
				out = append(out, pulita)
			}
			continue
		}
		// un pezzetto di una o due lettere con caratteri spuri dentro ("ı‾"): sporco
		if len([]rune(pulita)) <= 2 && strings.ContainsAny(pulita, caratteriSpuri) {
			continue
		}
		out = append(out, pulita)
	}
	// separatori doppi ("— —") e separatori all'inizio o in fondo
	var fine []string
	for _, w := range out {
		if separatoriSoli[w] && (len(fine) == 0 || separatoriSoli[fine[len(fine)-1]]) {
			continue
		}
		fine = append(fine, w)
	}
	for len(fine) > 0 && separatoriSoli[fine[len(fine)-1]] {
		fine = fine[:len(fine)-1]
	}
	return strings.Join(fine, " ")
}

func haLettereONumeri(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0
}

// carattereStrano: dentro una parola, un carattere che non è lettera, numero o segno ammesso.
func carattereStrano(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
		return strings.ContainsRune(caratteriSpuri, r)
	}
	return !strings.ContainsRune(segniAmmessi, r)
}

// maiuscoleACaso: una maiuscola dopo una minuscola dentro una parola che non esiste
// ("OccWon", "DiNr"). Le parole vere scritte così ("PlayStation", "YouTube") sono nei nomi.
func maiuscoleACaso(w string) bool {
	r := []rune(w)
	for i := 1; i < len(r); i++ {
		if unicode.IsUpper(r[i]) && unicode.IsLower(r[i-1]) {
			return true
		}
	}
	return false
}

// nomeProtettoIn: la parola fa parte di un nome da non tradurre (negozi, giochi della libreria).
func nomeProtettoIn(w string) bool {
	lw := strings.ToLower(w)
	for _, n := range nomiProtetti() {
		for _, p := range strings.Fields(strings.ToLower(n)) {
			if strings.Trim(p, "'.:") == lw {
				return true
			}
		}
	}
	return false
}

// giudizioParola: com'è una parola letta dall'OCR in una scritta inglese.
const (
	parolaBuona  = iota // esiste, è un termine dei giochi, una sigla o un nome protetto
	parolaNome          // sconosciuta ma con la forma di un nome proprio ("Sarah", "Hytale")
	parolaDubbia        // probabilmente letta male
)

func giudicaParola(w string) int {
	w = strings.Trim(w, "'’")
	for _, suf := range []string{"'s", "’s", "'S", "’S"} { // possessivo: "Koby's" è come "Koby"
		if strings.HasSuffix(w, suf) && len(w) > len(suf)+1 {
			w = strings.TrimSuffix(w, suf)
			break
		}
	}
	r := []rune(w)
	if len(r) == 0 {
		return parolaBuona
	}
	if nomeProtettoIn(w) || sigleOCR(w) != w { // "Ul" è la sigla "UI" letta male
		return parolaBuona
	}
	if parolaValidaOCR(w) || parolaComposta(w) {
		if maiuscoleACaso(w) && !esiste(strings.ToLower(w)) {
			return parolaDubbia
		}
		return parolaBuona
	}
	if strings.ContainsAny(w, "'’") {
		// apostrofo in mezzo a una parola sconosciuta: "SUBNAUT'CAZ" (le parole vere con
		// l'apostrofo, "don't", "Liar's", le conosce già esiste)
		return parolaDubbia
	}
	if maiuscoleACaso(w) {
		return parolaDubbia
	}
	tutteMaiuscole := strings.ToUpper(w) == w
	if tutteMaiuscole && len(r) <= 4 {
		return parolaBuona // sigla: "UI", "SFX", "HDR", "FOV"
	}
	if tutteMaiuscole {
		return parolaDubbia // "SUBNAUTCAZ": parola lunga tutta maiuscola che non esiste
	}
	if unicode.IsUpper(r[0]) && len(r) >= 4 && nomeProprio(w) {
		return parolaNome
	}
	return parolaDubbia // minuscola sconosciuta, o "Cha": pezzo di parola tagliato
}

// rigaOCRAffidabile: la riga letta (già senza simboli spuri) si può tradurre e mostrare.
// inglese=false (gioco in un'altra lingua): solo il controllo dei caratteri strani.
func rigaOCRAffidabile(s string, inglese bool) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, w := range strings.Fields(s) {
		if !haLettereONumeri(w) {
			continue
		}
		core := strings.TrimFunc(w, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
		if strings.IndexFunc(core, carattereStrano) >= 0 {
			return false // "DiNr•ün": un carattere strano dentro una parola
		}
	}
	if !inglese {
		return true
	}
	tot, dubbie := 0, 0
	for _, w := range reParolaLettere.FindAllString(s, -1) {
		if len([]rune(strings.Trim(w, "'"))) < 2 {
			continue
		}
		tot++
		if giudicaParola(w) == parolaDubbia {
			dubbie++
		}
	}
	if tot == 0 {
		return true
	}
	if tot <= 3 {
		return dubbie == 0
	}
	return dubbie*3 <= tot
}

// soloNomiPropri: la riga è fatta solo di nomi (persone, giochi, negozi): "Sarah", "Ethan
// Lauren", "Steam", "Hytale Launcher". Non si traduce e non si mostra.
func soloNomiPropri(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if _, pronta := fraseMenuConNumero(s); pronta {
		return false // "English", "Volume": voci con la loro traduzione pronta
	}
	pezzi := dividiProtetti(s, nomiProtetti())
	parole := 0
	for _, p := range pezzi {
		if p.protetto {
			if strings.IndexFunc(p.testo, unicode.IsLetter) >= 0 {
				parole++
			}
			continue
		}
		for _, w := range reParolaLettere.FindAllString(p.testo, -1) {
			w = strings.Trim(w, "'")
			if len([]rune(w)) < 2 {
				continue
			}
			parole++
			if giudicaParola(w) != parolaNome {
				return false
			}
		}
	}
	return parole > 0 && parole <= 8
}

// traduzioneDaMostrare: la traduzione è diversa dall'originale (senza contare maiuscole, spazi
// e punteggiatura finale). Una voce rimasta uguale non si mostra come "tradotta".
func traduzioneDaMostrare(en, it string) bool {
	if strings.TrimSpace(it) == "" {
		return false
	}
	return chiaveFrase(en) != chiaveFrase(it)
}

// rigaDaTradurre: la riga letta dall'OCR, pulita, e se va tradotta e mostrata.
func rigaDaTradurre(s string, inglese bool) (string, bool) {
	p := togliSimboliSpuri(s)
	if p == "" || !rigaOCRAffidabile(p, inglese) {
		return p, false
	}
	if inglese && soloNomiPropri(p) {
		return p, false
	}
	return p, true
}

// righeAffidabili: il testo letto in una zona, senza simboli spuri e senza le righe da non
// mostrare. menu=true (voci una per riga): ogni riga si giudica da sola. Dialoghi: una frase
// va spesso a capo, quindi si giudica tutto il testo insieme (una riga corta come "Caz." non è
// una lettura sbagliata), e una lista di soli nomi o con caratteri strani sparisce tutta.
func righeAffidabili(s string, inglese, menu bool) string {
	var out []string
	for _, r := range strings.Split(s, "\n") {
		if strings.TrimSpace(r) == "" {
			continue
		}
		if !menu {
			if p := togliSimboliSpuri(r); p != "" {
				out = append(out, p)
			}
			continue
		}
		if p, ok := rigaDaTradurre(r, inglese); ok {
			out = append(out, p)
		}
	}
	if !menu {
		tutto := strings.Join(out, " ")
		if tutto == "" || !rigaOCRAffidabile(tutto, inglese) || (inglese && soloNomiPropri(tutto)) {
			return ""
		}
	}
	return strings.Join(out, "\n")
}
