package traduce

// Lingue dei giochi: si traduce da qualsiasi lingua dell'elenco a qualsiasi altra, con i
// modelli liberi di Mozilla (Firefox Translations). Dove non c'è il modello diretto si passa
// dall'inglese (es. portoghese → inglese → francese). Ogni lingua si scarica solo quando serve.
// Il percorso completo (glossario da gioco, libreria, correttore IA) c'è per inglese → italiano.

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// LinguaGioco: una lingua dell'elenco. Codice a 2 lettere (zt = cinese tradizionale: il
// traduttore usa le prime 2+2 lettere della coppia).
type LinguaGioco struct {
	Codice, Firefox, Nome, Nativo, OCR, Scrittura string
	VersoInglese, DallInglese                     []FileDaScaricare
}

func linguaDa(codice string) (LinguaGioco, bool) {
	for _, l := range lingueGioco {
		if l.Codice == codice {
			return l, true
		}
	}
	return LinguaGioco{}, false
}

// arrivoDiBase: la lingua in cui tradurre finché non ne scegli una: quella di Tasso.
// In spagnolo, francese o tedesco è quella lingua (i modelli si scaricano al primo uso, come l'italiano).
func arrivoDiBase() string {
	if l := linguaTasso(); l != "it" {
		if _, ok := linguaDa(l); ok {
			return l
		}
		return "en"
	}
	return "it"
}

// nomeLingua: il nome della lingua da scrivere nei messaggi, nella lingua di Tasso.
func nomeLingua(codice string) string {
	if inIngleseTasso() {
		if n, ok := nomiInglesi[codice]; ok { // stessi nomi del correttore IA (motore_ia.go)
			return n
		}
	}
	if l, ok := linguaDa(codice); ok {
		return strings.ToLower(l.Nome)
	}
	return codice
}

// dirCoppia: dove stanno i file di una coppia (es. "deen"). Inglese↔italiano restano dove erano.
func dirCoppia(dirApp, coppia string) string {
	return filepath.Join(dirApp, "modelli", "lingue", coppia)
}

// filesCoppia: i file del modello da → a (uno dei due è sempre l'inglese).
func filesCoppia(da, a string) []FileDaScaricare {
	if da == "en" {
		if l, ok := linguaDa(a); ok {
			return l.DallInglese
		}
	}
	if a == "en" {
		if l, ok := linguaDa(da); ok {
			return l.VersoInglese
		}
	}
	return nil
}

func coppiaPresente(dirApp, da, a string) bool {
	f := filesCoppia(da, a)
	if len(f) == 0 {
		return false
	}
	for _, x := range f {
		if st, err := os.Stat(filepath.Join(dirCoppia(dirApp, da+a), x.Nome)); err != nil || st.Size() != x.Byte {
			return false
		}
	}
	return true
}

// modelliServono: i modelli che mancano per tradurre da → a (passando dall'inglese).
func modelliServono(dirApp, da, a string, presente func(da, a string) bool) [][2]string {
	var serve [][2]string
	aggiungi := func(x, y string) {
		if !presente(x, y) {
			serve = append(serve, [2]string{x, y})
		}
	}
	if da == a {
		return nil
	}
	if da != "en" {
		aggiungi(da, "en")
	}
	if a != "en" {
		aggiungi("en", a)
	}
	return serve
}

func byteCoppie(c [][2]string) int64 {
	var n int64
	for _, x := range c {
		for _, f := range filesCoppia(x[0], x[1]) {
			n += f.Byte
		}
	}
	return n
}

// parteFile: che pezzo del modello è un file ("model", "lex", "vocab", "srcvocab", "trgvocab").
func parteFile(nome string) string {
	for _, p := range []string{"srcvocab", "trgvocab", "vocab", "lex"} {
		if strings.HasPrefix(nome, p) {
			return p
		}
	}
	return "model"
}

// ---- riconoscere la lingua delle scritte

var paroleLatine = map[string][]string{
	"en": {"the", "and", "you", "to", "of", "is", "your", "with", "for", "not", "this", "are", "press", "continue", "settings"},
	"it": {"il", "che", "non", "di", "è", "una", "per", "con", "gli", "della", "del", "gioco", "impostazioni", "premi"},
	"fr": {"le", "la", "les", "et", "vous", "des", "est", "pas", "une", "pour", "avec", "du", "dans", "jeu", "paramètres", "appuyez"},
	"de": {"der", "die", "und", "das", "nicht", "ist", "mit", "sie", "ein", "eine", "zu", "auf", "für", "ich", "du", "den", "dem", "spiel", "einstellungen", "beenden", "drücke"},
	"es": {"el", "los", "las", "y", "que", "es", "una", "por", "con", "para", "del", "juego", "ajustes", "pulsa", "configuración"},
	"pt": {"o", "os", "as", "e", "que", "não", "uma", "por", "com", "para", "do", "da", "jogo", "configurações", "pressione", "você"},
	"pl": {"i", "w", "nie", "się", "na", "jest", "z", "do", "to", "że", "gra", "ustawienia", "naciśnij", "jak"},
	// danese e norvegese hanno quasi tutte le parole uguali: qui solo quelle che li distinguono
	"da": {"af", "hvad", "spil", "indstillinger", "mig", "dig", "fortsæt", "tryk", "nej", "også", "efter", "hvor", "gemme"},
	"nb": {"av", "hva", "spill", "innstillinger", "meg", "deg", "fortsett", "trykk", "nei", "ikkje", "etter", "hvor", "lagre"},
	"sv": {"och", "är", "att", "inte", "till", "spel", "inställningar", "jag", "mig", "dig", "fortsätt", "tryck", "nej", "också"},
	"fi": {"ja", "on", "ei", "se", "että", "olet", "peli", "asetukset", "paina", "jatka", "kanssa"},
	"tr": {"ve", "bir", "bu", "için", "değil", "ile", "oyun", "ayarlar", "devam", "basın", "çok"},
}

// RilevaLingua: la lingua più probabile di un gruppo di scritte. Prima la scrittura
// (cirillico, kana, hangul, ideogrammi), poi le parole più comuni. "" se non si capisce.
func RilevaLingua(testi []string) string {
	var kana, hangul, han, cir, lat int
	for _, t := range testi {
		for _, r := range t {
			switch {
			case unicode.In(r, unicode.Hiragana, unicode.Katakana):
				kana++
			case unicode.Is(unicode.Hangul, r):
				hangul++
			case unicode.Is(unicode.Han, r):
				han++
			case unicode.Is(unicode.Cyrillic, r):
				cir++
			case unicode.IsLetter(r):
				lat++
			}
		}
	}
	tot := kana + hangul + han + cir + lat
	if tot < 6 {
		return ""
	}
	switch {
	case hangul*3 > tot:
		return "ko"
	case kana > 0 && (kana+han)*3 > tot:
		return "ja"
	case han*3 > tot:
		return cineseTradizionaleOSemplificato(testi)
	case cir*3 > tot:
		return "ru"
	}
	conta := map[string]int{}
	parole := 0
	for _, t := range testi {
		for _, w := range strings.FieldsFunc(strings.ToLower(t), func(r rune) bool { return !unicode.IsLetter(r) }) {
			parole++
			for l, lista := range paroleLatine {
				for _, p := range lista {
					if w == p {
						conta[l]++
					}
				}
			}
		}
	}
	migliore, n := "", 0
	for l, c := range conta {
		if c > n {
			migliore, n = l, c
		}
	}
	pari := 0
	for _, c := range conta {
		if c == n {
			pari++
		}
	}
	if n < 3 || parole < 8 || pari > 1 { // pochi indizi o pareggio: non lo so
		return ""
	}
	return migliore
}

// caratteri che esistono solo nel cinese semplificato o solo nel tradizionale (i più comuni)
const (
	soloSemplificato = "这们个来说时会过还对没为发经让东车问门见么后们开关战选设置继续返回进务级资"
	soloTradizionale = "這們個來說時會過還對沒為發經讓東車問門見麼後開關戰選設置繼續返回進務級資"
)

func cineseTradizionaleOSemplificato(testi []string) string {
	s, t := 0, 0
	for _, x := range testi {
		for _, r := range x {
			if strings.ContainsRune(soloSemplificato, r) && !strings.ContainsRune(soloTradizionale, r) {
				s++
			}
			if strings.ContainsRune(soloTradizionale, r) && !strings.ContainsRune(soloSemplificato, r) {
				t++
			}
		}
	}
	switch {
	case t > s:
		return "zt"
	case s > t:
		return "zh"
	}
	return "han" // non si capisce: chi chiama tiene la scelta di prima
}

// LinguaValida (3.1): per Tasso, che controlla le lingue ricordate per ogni gioco.
func LinguaValida(codice string) bool {
	_, ok := linguaDa(codice)
	return ok
}
