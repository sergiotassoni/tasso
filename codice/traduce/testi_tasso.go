package traduce

// Scritte di Tasso Traduce nella lingua di Tasso (italiano o inglese).

// InIngleseTasso: Tasso è in inglese (lo imposta Tasso all'avvio), per le scritte disegnate da qui.
var InIngleseTasso func() bool

func testoTasso(it, en string) string {
	if InIngleseTasso != nil && InIngleseTasso() {
		return en
	}
	return it
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
