package main

// "Fai aiutare un amico" (in Risolvi un problema): apre Assistenza rapida (Quick Assist), il programma
// gratuito di Windows per farsi aiutare da lontano. Niente server di Tasso: la scelta di cosa aprire
// è in aiutoamico/ (con i test), qui c'è solo l'apertura.

import (
	"os"
	"path/filepath"
	"strings"

	"tasso/aiutoamico"

	webview2 "github.com/jchv/go-webview2"
)

// aiutoAmicoScelta: Assistenza rapida è installata? Se no, dove la si prende. I pacchetti vengono
// dal registro dell'utente (niente PowerShell); il manifest lo leggo solo per Assistenza rapida.
func aiutoAmicoScelta() aiutoamico.Scelta {
	var lista []aiutoamico.Installato
	for _, p := range leggiPacchettiAppx() {
		x := aiutoamico.Installato{NomeCompleto: p.nomeCompleto}
		// La cartella viene dal registro dell'utente: niente percorsi di rete (\\server\...) e niente
		// file enormi; la dimensione la guardo prima di leggere.
		if strings.EqualFold(p.nome, aiutoamico.Pacchetto) && p.cartella != "" && !strings.HasPrefix(p.cartella, `\\`) {
			f := filepath.Join(p.cartella, "AppxManifest.xml")
			if fi, err := os.Stat(f); err == nil && fi.Mode().IsRegular() && fi.Size() < 4<<20 {
				if b, err := os.ReadFile(f); err == nil {
					x.AppID, x.Protocolli = aiutoamico.LeggiManifest(b)
				}
			}
		}
		lista = append(lista, x)
	}
	return aiutoamico.Scegli(lista)
}

// aiutoAmicoApri: apre Assistenza rapida (shell:AppsFolder\<AUMID>, o la sua pagina nello Store)
// come utente normale, non da amministratore, passando da Esplora file. Risponde con il modo usato: "app", "store" o "web".
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
