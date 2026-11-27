package main

// "Fai aiutare un amico" (in Risolvi un problema): apre Assistenza rapida (Quick Assist), il programma
// gratuito di Windows per farsi aiutare da lontano. Niente server di Tasso: la scelta di cosa aprire
// è in aiutoamico/ (con i test), qui c'è solo l'apertura.

import (
	"tasso/aiutoamico"

	webview2 "github.com/jchv/go-webview2"
)

// aiutoAmicoScelta: Assistenza rapida è installata? Se no, dove la si prende.
func aiutoAmicoScelta() aiutoamico.Scelta {
	var nomi []string
	for _, p := range leggiPacchettiAppx() {
		nomi = append(nomi, p.nomeCompleto)
	}
	return aiutoamico.Scegli(nomi)
}

// aiutoAmicoApri: apre Assistenza rapida (o la sua pagina nello Store) come utente normale,
// non da amministratore. Risponde con il modo usato: "app", "store" o "web".
func aiutoAmicoApri() string {
	s := aiutoAmicoScelta()
	if !aiutoamico.IndirizzoAmmesso(s.Indirizzo) {
		return "no"
	}
	if s.Modo == aiutoamico.ModoWeb {
		openURL(s.Indirizzo)
		return s.Modo
	}
	if !openPath(explorerExe(), "\""+s.Indirizzo+"\"") {
		openURL(s.Indirizzo)
	}
	return s.Modo
}

func bindAiutoAmico(w webview2.WebView) {
	_ = w.Bind("goAiutoAmicoStato", func() string { return aiutoAmicoScelta().Modo })
	_ = w.Bind("goAiutoAmicoApri", func() string { return aiutoAmicoApri() })
}
