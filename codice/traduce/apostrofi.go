package traduce

// Correzioni degli errori tipici dell'OCR di Windows sull'inglese, misurati sul PC di Sergio
// (7/11/2026, 100 immagini): l'apostrofo letto come lettera o puntino ("itis", "welre", "lim",
// "it•s"), la l minuscola letta come I maiuscola ("Iost") e maiuscole a metà frase ("Older").

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	rePuntinoApostrofo = regexp.MustCompile(`([A-Za-z])[•·](s|t|re|ve|ll|d|m)\b`)
	reContrazioneL     = regexp.MustCompile(`\b(?i:(we|you|they|who|what|there|that|it|he|she))[l|1](re|ve|ll|d|s)\b`)
	reNegazioneL       = regexp.MustCompile(`\b(?i:(don|didn|doesn|isn|wasn|aren|weren|can|won|couldn|wouldn|shouldn|haven|hasn|hadn|mustn|needn))[l|1I]t\b`)
	reIoSbagliato      = regexp.MustCompile(`(^|[^A-Za-z'])(?:[Il|1]{1,2}[l'|]?|l)(m|ve|ll|d)\b`)
	reIoLim            = regexp.MustCompile(`(^|[^A-Za-z'])(?:lim|Iim|lIm|llm|IIm|1m)\b`)
	reIdue             = regexp.MustCompile(`(^|[^A-Za-z'])[Il]'(m|ve|ll|d)\b`)
	reIoL              = regexp.MustCompile(`\bI(ost|ook|ooks|ooked|ike|ikes|iked|ife|ast|eft|et|ets|ove|oved|ight|ittle|ong|ater|ead|eave|ine|ist|ive|ived|ot|ow|ower|ady|and|ate|ate|ie|ies|ied|ying|ock|ocked|oot|ord|uck)\b`)
	reParolaMaiuscola  = regexp.MustCompile(`([a-z,]) ([OoYyVvWwSsCcUuXxZzKkPp][a-z]+)\b`)
)

// parole comuni con iniziale "uguale" in maiuscolo e minuscolo (o/O, y/Y, v/V...): a metà frase
// dopo una parola minuscola sono quasi sempre minuscole.
var parolaComune = map[string]bool{
	"older": true, "other": true, "others": true, "our": true, "out": true, "over": true, "own": true, "only": true,
	"once": true, "open": true, "one": true, "of": true, "off": true, "on": true, "or": true, "order": true,
	"your": true, "you": true, "yours": true, "years": true, "year": true, "yet": true, "yes": true,
	"very": true, "valley": true, "village": true, "voice": true, "vessel": true, "view": true,
	"way": true, "was": true, "we": true, "well": true, "with": true, "will": true, "would": true, "were": true,
	"what": true, "when": true, "where": true, "which": true, "while": true, "who": true, "why": true, "world": true, "water": true, "wall": true, "war": true, "work": true,
	"so": true, "some": true, "see": true, "say": true, "she": true, "should": true, "still": true, "stop": true, "step": true, "sure": true, "side": true, "ship": true, "sword": true,
	"can": true, "come": true, "could": true, "castle": true, "city": true, "computer": true, "confidence": true, "car": true, "cave": true,
	"us": true, "up": true, "upstairs": true, "under": true, "until": true, "use": true,
	"keep": true, "know": true, "kill": true,
	"people": true, "place": true, "players": true, "please": true, "point": true, "power": true, "percent": true, "progress": true,
}

func correggiOCRInglese(r string) string {
	r = rePuntinoApostrofo.ReplaceAllString(r, "$1'$2")
	r = reContrazioneL.ReplaceAllString(r, "$1'$2")
	r = reNegazioneL.ReplaceAllString(r, "$1't") // "donlt" → "don't" (sottotitoli, 24/11)
	r = strings.ReplaceAll(r, "itis ", "it's ")
	r = reIoLim.ReplaceAllString(r, "${1}I'm")
	r = reIdue.ReplaceAllString(r, "${1}I'$2")
	r = reIoSbagliato.ReplaceAllStringFunc(r, func(m string) string {
		sub := reIoSbagliato.FindStringSubmatch(m)
		corpo := m[len(sub[1]) : len(m)-len(sub[2])]
		if corpo == "I" { // "Im", "Id": lascio stare, possono essere parole vere
			return m
		}
		return sub[1] + "I'" + sub[2]
	})
	r = reIoL.ReplaceAllString(r, "l$1")
	for _, f := range fraseFisse {
		r = f.re.ReplaceAllString(r, f.con)
	}
	for giro := 0; giro < 2; giro++ { // due giri: le corrispondenze si sovrappongono ("off Your")
		r = minuscoleAMetaFrase(r)
	}
	return r
}

var fraseFisse = []struct {
	re  *regexp.Regexp
	con string
}{
	{regexp.MustCompile(`\bDa not\b`), "Do not"},
	{regexp.MustCompile(`\bda not\b`), "do not"},
	{regexp.MustCompile(`\btao (late|much|many|far|soon|long|bad|easy|hard|close|dangerous)\b`), "too $1"},
	{regexp.MustCompile(`\bfar (me|you|him|her|us|them|it|now)\b`), "for $1"},
	{regexp.MustCompile(`\bout far\b`), "out for"},
	// provate sul PC di Sergio su 80 frasi nuove (7/11/2026)
	{regexp.MustCompile(`(^|[^A-Za-z'])[Il1]'[Il1]l\b`), "${1}I'll"},
	{regexp.MustCompile(`\b([Ww])eill\b`), "${1}e'll"},
	{regexp.MustCompile(`(^|[.!?"]\s*)[Ll]ive (seen|been|got|done|never|always|heard|had|made|told|found|lost|tried|waited)\b`), "${1}I've $2"},
	{regexp.MustCompile(`(^|[.!?"]\s*)[li]t('s|\b)`), "${1}It$2"},
	{regexp.MustCompile(`(^|[\s"])l(\s)`), "${1}I$2"},
}

func minuscoleAMetaFrase(r string) string {
	return reParolaMaiuscola.ReplaceAllStringFunc(r, func(m string) string {
		sub := reParolaMaiuscola.FindStringSubmatch(m)
		p := sub[2]
		if unicode.IsUpper(rune(p[0])) && parolaComune[strings.ToLower(p)] {
			return sub[1] + " " + strings.ToLower(p)
		}
		return m
	})
}
