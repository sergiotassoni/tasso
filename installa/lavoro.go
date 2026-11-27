package installa

import (
	"strings"
	"time"
)

// Cartella di lavoro dell'installatore (diario e dati della WebView2): non più in %TEMP%
// dell'utente, dove qualunque programma dell'utente può scrivere, ma in %ProgramData%\TassoSetup
// con i permessi di Programmi (solo gli amministratori la possono cambiare).

// NomeCartellaLavoro: la cartella dentro %ProgramData%.
const NomeCartellaLavoro = "TassoSetup"

// PrefissoDatiWebView2: le cartelle dei dati della WebView2 dentro la cartella di lavoro.
const PrefissoDatiWebView2 = "WebView2-"

// AzioneLavoro: cosa fare con la cartella di lavoro che si trova sul disco.
type AzioneLavoro int

const (
	LavoroCrea      AzioneLavoro = iota // non c'è: la creo già protetta
	LavoroUsa                           // c'è ed è degli amministratori: rimetto i permessi e la uso
	LavoroSpostaVia                     // c'è ma non è sicura (di un utente, collegamento, file): la sposto via e ne creo una nuova
)

// DecidiCartellaLavoro: esiste = c'è qualcosa con quel nome; cartella = è una cartella vera;
// collegamento = collegamento simbolico o giunzione (reparse point); admin = il proprietario è
// Amministratori, SYSTEM o TrustedInstaller.
func DecidiCartellaLavoro(esiste, cartella, collegamento, admin bool) AzioneLavoro {
	switch {
	case !esiste:
		return LavoroCrea
	case cartella && !collegamento && admin:
		return LavoroUsa
	}
	return LavoroSpostaVia
}

// DatiWebView2Vecchi: una cartella di dati della WebView2 rimasta da un'installazione di prima
// (chiusa male). Le tolgo dopo un'ora: una copia dell'installatore aperta adesso non la tocco.
func DatiWebView2Vecchi(nome string, modifica, ora time.Time) bool {
	return strings.HasPrefix(nome, PrefissoDatiWebView2) && ora.Sub(modifica) > time.Hour
}
