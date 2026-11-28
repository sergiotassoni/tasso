// Package segreti: i segreti di Tasso nelle impostazioni (l'indirizzo del webhook di Discord, che
// contiene il suo token) non stanno mai in chiaro su disco, nei registri o nei messaggi.
//
// Su disco va solo la versione protetta con DPAPI di Windows per l'utente (come il Portachiavi e il
// token del bot): la può leggere solo il tuo account su questo PC. Qui ci sono le regole pure
// (senza Windows), così si provano anche su Linux; la protezione vera la passa chi chiama.
package segreti

import (
	"regexp"
	"strings"
)

// Proteggi e Sblocca: le funzioni DPAPI di Tasso (protectString e unprotectString). Proteggi
// restituisce "" se Windows non riesce a proteggere.
type (
	Proteggi func(string) string
	Sblocca  func(string) string
)

// Migra: dalle impostazioni lette (vecchio = il webhook in chiaro delle versioni fino alla 3.1,
// enc = quello protetto) il webhook da usare in memoria e quello protetto da salvare.
// cambiato = il file va riscritto (c'era il testo in chiaro: dopo non c'è più).
// Se Windows non riesce a proteggere, il webhook resta solo in memoria per questa volta: in
// chiaro su disco non torna mai (andrà incollato di nuovo).
func Migra(vecchio, enc string, p Proteggi, s Sblocca) (inChiaro, protetto string, cambiato bool) {
	if vecchio != "" {
		return vecchio, p(vecchio), true
	}
	if enc == "" {
		return "", "", false
	}
	return s(enc), enc, false
}

// reIdWebhook: l'indirizzo di un webhook di Discord, con il numero (non segreto) e il token.
var reIdWebhook = regexp.MustCompile(`^(https://(?:(?:ptb|canary)\.)?discord(?:app)?\.com/api/webhooks/\d{5,25}/)[A-Za-z0-9_\-]{20,100}$`)

// Mascherato: il segno che sta al posto del token quando l'indirizzo si mostra.
const Mascherato = "••••••••"

// Maschera: l'indirizzo senza il token, da mostrare nella finestra ("" se vuoto). Un indirizzo
// strano non si mostra per niente (potrebbe contenere il token in un'altra forma).
func Maschera(url string) string {
	if url == "" {
		return ""
	}
	if m := reIdWebhook.FindStringSubmatch(url); m != nil {
		return m[1] + Mascherato
	}
	return Mascherato
}

// reWebhookInChiaro: il valore in chiaro "discordWebhook": "https://..." in un settings.json (anche
// rovinato o in una copia .prima/.bad).
var reWebhookInChiaro = regexp.MustCompile(`("discordWebhook"\s*:\s*)"(?:[^"\\]|\\.)*"`)

// TogliInChiaro: il testo con il webhook in chiaro svuotato. ok = c'era (va riscritto).
func TogliInChiaro(b []byte) (out []byte, ok bool) {
	if !reWebhookInChiaro.Match(b) {
		return b, false
	}
	out = reWebhookInChiaro.ReplaceAll(b, []byte(`${1}""`))
	return out, string(out) != string(b)
}

// Nasconde: s con l'indirizzo del webhook al posto del token (per un testo da scrivere in un
// registro o da mostrare, se per sbaglio lo contenesse).
func Nasconde(s, url string) string {
	if url == "" || !strings.Contains(s, url) {
		return s
	}
	return strings.ReplaceAll(s, url, Maschera(url))
}
