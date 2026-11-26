package traduce

// Chi parla nelle scritte dello schermo (collaudo finale 26/11): la traduzione dello schermo non
// separava il nome di chi parla, e i nomi che sono anche parole inglesi si traducevano: "Cope"
// (il pilota di Ace Combat) → "Affrontare", "SHADE:" → "OMA:". Il nome resta com'è e si traduce
// solo la battuta, anche nel formato radio "Cope << One more time. >>".

import (
	"regexp"
	"strings"
)

// battutaConNome: la scritta è "Nome: battuta" (o "Nome << battuta >>") con una battuta vera
// (una frase, un'azione tra asterischi o parentesi), non un'etichetta con il suo valore
// ("Difficulty: Easy", "Lockpicks Left: 3"). sep è quello che va tra il nome e la traduzione.
func (t *Traduttore) battutaConNome(s string) (nome, sep, battuta string, ok bool) {
	s = strings.TrimSpace(s)
	nome, battuta = separaParlante(s)
	if nome == "" || battuta == "" || battuta == s {
		return "", "", "", false
	}
	resto := strings.TrimSpace(strings.TrimPrefix(s, nome))
	sep = " "
	if strings.HasPrefix(resto, ":") {
		sep = ": "
	} else if !strings.HasPrefix(resto, "<") {
		return "", "", "", false
	}
	// un'etichetta dell'interfaccia o un termine del gioco ("Difficulty", "Objective") si traduce
	if _, pronta := fraseMenu(nome); pronta {
		return "", "", "", false
	}
	if _, termine := t.termineEsatto(nome); termine {
		return "", "", "", false
	}
	b := strings.TrimSpace(battuta)
	frase := len(strings.Fields(b)) >= 4 ||
		strings.ContainsAny(b[:1], "*(<«\"'") ||
		strings.ContainsAny(b[len(b)-1:], ".!?…-—)>\"'*")
	if !frase {
		return "", "", "", false
	}
	return nome, sep, battuta, true
}

// reTitoloPunto: "Ms. Toriumi", "Dr. Kim", "Sgt. Rivers": il punto del titolo davanti a un nome.
var reTitoloPunto = regexp.MustCompile(`\b(Mr|Mrs|Ms|Mx|Dr|St|Mt|Jr|Sr|Lt|Sgt|Capt|Cpt|Col|Gen|Prof|Rev|Fr)\.(\s+\p{Lu})`)

// senzaPuntoTitoli: il traduttore veloce divide le frasi a ogni punto, anche dopo "Ms." ("I'm Ms.
// Toriumi. I teach Composition." → "Sono la signora. Toriumi. Insegno composizione.", collaudo
// 26/11). Senza il punto ("Ms Toriumi") il titolo resta attaccato al nome.
func senzaPuntoTitoli(s string) string {
	return reTitoloPunto.ReplaceAllString(s, "$1$2")
}
