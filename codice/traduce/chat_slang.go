package traduce

import (
	"regexp"
	"strings"
)

// Chat dei giocatori: abbreviazioni e gergo. Due liste scritte a mano:
//   - sigleDaGiocatori: parole che i giocatori italiani usano uguali (gg, afk, lol): un messaggio
//     fatto solo di queste resta com'è, senza passare dal traduttore (che le storpiava).
//   - abbreviazioniChat: abbreviazioni inglesi da scrivere per intero PRIMA di tradurre, così il
//     traduttore capisce la frase ("u r late" → "you are late" → "sei in ritardo").
var sigleDaGiocatori = map[string]bool{
	"gg": true, "ggwp": true, "wp": true, "gl": true, "hf": true, "glhf": true, "afk": true,
	"lol": true, "lmao": true, "xd": true, "ez": true, "noob": true, "nub": true, "op": true,
	"nerf": true, "buff": true, "gank": true, "dps": true, "tank": true, "healer": true,
	"pvp": true, "pve": true, "mvp": true, "rip": true, "ok": true, "okay": true, "kk": true,
	"k": true, "o7": true, "gj": true, "nt": true, "ff": true, "ggs": true, "rekt": true,
	"pog": true, "poggers": true, "brb": false, // brb si traduce (vedi sotto)
}

var abbreviazioniChat = map[string]string{
	"u": "you", "ur": "your", "r": "are", "pls": "please", "plz": "please", "thx": "thanks",
	"ty": "thank you", "tyvm": "thank you very much", "np": "no problem", "idk": "I don't know",
	"imo": "in my opinion", "btw": "by the way", "omw": "on my way", "brb": "be right back",
	"nvm": "never mind", "ppl": "people", "rn": "right now", "tbh": "to be honest",
	"cuz": "because", "bc": "because", "b4": "before", "gonna": "going to", "wanna": "want to",
	"gotta": "got to", "dunno": "don't know", "ya": "you", "yea": "yes", "yeah": "yes",
	"yep": "yes", "nope": "no", "msg": "message", "dm": "message", "pm": "message",
	"asap": "as soon as possible", "irl": "in real life", "w8": "wait", "wtb": "want to buy",
	"wts": "want to sell", "wtt": "want to trade", "lf": "looking for", "lfg": "looking for group",
	"lfm": "looking for more players", "sry": "sorry", "srry": "sorry", "tho": "though",
	"m8": "mate", "ofc": "of course", "smth": "something", "sth": "something", "abt": "about",
	"wym": "what do you mean", "wdym": "what do you mean", "hbu": "how about you",
	"wyd": "what are you doing", "hmu": "contact me", "ily": "I love you", "jk": "just kidding",
}

var reParolaChat = regexp.MustCompile(`[A-Za-z0-9']+`)

// soloSigle: il messaggio è fatto solo di sigle che i giocatori usano uguali (es. "gg wp", "GLHF!").
func soloSigle(s string) bool {
	parole := reParolaChat.FindAllString(s, -1)
	if len(parole) == 0 {
		return false
	}
	for _, p := range parole {
		if !sigleDaGiocatori[strings.ToLower(p)] {
			return false
		}
	}
	return true
}

// espandiAbbreviazioni scrive per intero le abbreviazioni della chat, lasciando il resto uguale.
func espandiAbbreviazioni(s string) string {
	return reParolaChat.ReplaceAllStringFunc(s, func(p string) string {
		if v, ok := abbreviazioniChat[strings.ToLower(p)]; ok {
			return v
		}
		return p
	})
}

// uniscriRigheChat: un messaggio lungo va a capo su più righe. Se la chat ha i nomi dei
// giocatori ("Nome: testo"), una riga senza nome è il seguito della riga sopra.
func uniscriRigheChat(righe []string) []string {
	conNome := 0
	for _, r := range righe {
		if n, _ := separaParlante(r); n != "" {
			conNome++
		}
	}
	if conNome == 0 {
		return righe
	}
	var out []string
	for _, r := range righe {
		if n, _ := separaParlante(r); n == "" && len(out) > 0 {
			out[len(out)-1] += " " + r
			continue
		}
		out = append(out, r)
	}
	return out
}
