package traduce

// Scritte di Tasso Traduce nella lingua di Tasso (italiano, inglese, spagnolo, francese, tedesco).

// InIngleseTasso: Tasso non è in italiano (lo imposta Tasso all'avvio), per le scritte disegnate
// da qui: in spagnolo, francese e tedesco quelle che non sono nel dizionario di Tasso sono in inglese.
var InIngleseTasso func() bool

// LinguaTasso: la lingua di Tasso ("it", "en", "es", "fr", "de"), impostata da Tasso.
var LinguaTasso func() string

func inIngleseTasso() bool { return InIngleseTasso != nil && InIngleseTasso() }

func linguaTasso() string {
	if LinguaTasso != nil {
		return LinguaTasso()
	}
	if inIngleseTasso() {
		return "en"
	}
	return "it"
}

func testoTasso(it, en string) string {
	if !inIngleseTasso() {
		return it
	}
	if l := linguaTasso(); l != "en" && l != "it" { // es/fr/de: dal dizionario di Tasso, se c'è
		if t := trTasso(it); t != it {
			return t
		}
	}
	return en
}

// TraduciTasso: il traduttore dei messaggi di Tasso (dizionario i18n), impostato da Tasso.
// Serve per i messaggi d'errore italiani scritti sopra il gioco quando Tasso è in inglese.
var TraduciTasso func(string) string

func trTasso(s string) string {
	if TraduciTasso != nil {
		return TraduciTasso(s)
	}
	return s
}

// nomeMotore: chi ha tradotto, da scrivere sopra il gioco nella lingua di Tasso. Risultato.Motore
// resta in italiano perché il codice lo confronta ("memoria", "libreria"...).
func nomeMotore(m string) string {
	if m == "Mozilla" { // nome interno del traduttore veloce: all'utente si mostra quello di Tasso
		return testoTasso("Traduzione veloce", "Fast translation")
	}
	if !inIngleseTasso() {
		return m
	}
	switch m {
	case "memoria":
		return "memory"
	case "libreria":
		return "library"
	case "libreria (IA)":
		return "library (AI)"
	case "catalogo":
		return "catalogue"
	case "libreria giochi liberi":
		return "free games library"
	case "IA":
		return "AI"
	}
	return m
}
