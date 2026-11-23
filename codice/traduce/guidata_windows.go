//go:build windows

package traduce

// "Traduci un gioco" a passi (idea presa dalla Traduzione gioco di LagoFast): scegli il gioco,
// conferma la lingua, scegli la lingua di arrivo, "Applica traduzione". La differenza: LagoFast
// cambia i file del gioco, Tasso NO (regola anti-cheat). "Applicare" qui vuol dire: preparare i
// traduttori, la libreria delle traduzioni di quel gioco e ricordare che quel gioco va tradotto
// da solo appena lo apri (sovrimpressione sullo schermo, come sempre).

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"
)

// chiaveGioco: il gioco si ricorda dalla sua cartella (Steam ed Epic a volte chiamano lo stesso
// gioco in modo diverso, la cartella invece è quella).
func chiaveGioco(cartella string) string {
	pulita := strings.ToLower(filepath.Clean(cartella))
	h := sha1.Sum([]byte(pulita))
	return filepath.Base(pulita) + "-" + hex.EncodeToString(h[:4])
}

// giocoTradotto: il gioco ha la traduzione "applicata" (parte da sola quando lo apri).
func giocoTradotto(cartella string) bool {
	if cartella == "" {
		return false
	}
	muImp.Lock()
	defer muImp.Unlock()
	return imp.GiochiTradotti[chiaveGioco(cartella)]
}

func impostaGiocoTradotto(cartella string, si bool) {
	nome := chiaveGioco(cartella)
	muImp.Lock()
	if imp.GiochiTradotti == nil {
		imp.GiochiTradotti = map[string]bool{}
	}
	pulita := strings.ToLower(filepath.Clean(cartella))
	var resto []string
	for _, c := range imp.CartelleTradotte {
		if c != pulita {
			resto = append(resto, c)
		}
	}
	if si {
		imp.GiochiTradotti[nome] = true
		if len(resto) >= 500 {
			resto = resto[len(resto)-499:] // pieno: tolgo le cartelle più vecchie
		}
		resto = append(resto, pulita)
	} else {
		delete(imp.GiochiTradotti, nome)
	}
	imp.CartelleTradotte = resto
	muImp.Unlock()
	salvaImp()
}

// ricordaCartellaTradotta: un gioco tradotto con una versione di prima (senza la sua cartella
// nell'elenco) la aggiunge adesso, così viene riconosciuto anche fuori dai negozi.
func ricordaCartellaTradotta(cartella string) {
	pulita := strings.ToLower(filepath.Clean(cartella))
	muImp.Lock()
	for _, c := range imp.CartelleTradotte {
		if c == pulita {
			muImp.Unlock()
			return
		}
	}
	muImp.Unlock()
	impostaGiocoTradotto(cartella, true)
}

// cartellaTradottaDi: se il programma del gioco sta dentro la cartella di un gioco tradotto,
// restituisce quella cartella (anche per i giochi che riconosciGioco non conosce).
func cartellaTradottaDi(percorso string) string {
	p := strings.ToLower(filepath.Clean(percorso))
	if p == "" || p == "." {
		return ""
	}
	muImp.Lock()
	defer muImp.Unlock()
	best := ""
	for _, c := range imp.CartelleTradotte {
		if strings.HasPrefix(p, c+`\`) && len(c) > len(best) {
			best = c
		}
	}
	return best
}

// aspettaMotore: il traduttore veloce è pronto? Se è scaricato ma si sta ancora aprendo (pochi
// secondi dopo l'avvio di Tasso o dopo lo scaricamento) aspetto fino a 20 secondi. "" = pronto,
// altrimenti cosa dire: "non scaricato" e "scaricato ma non partito" sono due cose diverse.
func aspettaMotore() string {
	if !mozillaPresente() {
		return "Il traduttore veloce non è ancora scaricato: scaricalo in Giochi → Impostazioni"
	}
	if mozOK.Load() {
		return ""
	}
	assicuraMotore()
	for t := time.Now(); !mozOK.Load() && time.Since(t) < 20*time.Second; time.Sleep(250 * time.Millisecond) {
		if e, _ := mozErr.Load().(string); e != "" {
			break
		}
	}
	if mozOK.Load() {
		return ""
	}
	return "Il traduttore veloce è scaricato ma non è partito: in Giochi → Impostazioni premi Riprova"
}

// aspettaCoppia: dopo aver scaricato dei modelli il motore li rilegge; aspetto che traduca
// davvero da → a (al massimo 20 secondi) invece di un'attesa fissa.
func aspettaCoppia(da, a string) bool {
	for t := time.Now(); time.Since(t) < 20*time.Second; time.Sleep(500 * time.Millisecond) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := moz.traduciCoppia(ctx, "Continue", da, a)
		cancel()
		if err == nil {
			return true
		}
	}
	return false
}

// applicaTraduzione: i passi veri, con l'avanzamento mandato alla finestra.
// haAntiCheat: il gioco in questa cartella ha un sistema anti-imbroglio.
func haAntiCheat(cartella string) bool {
	g, ok := riconosciGioco(filepath.Join(cartella, "x.exe"))
	if !ok {
		g = GiocoRiconosciuto{Cartella: cartella} // anche fuori dalle cartelle dei negozi
	}
	return ConAntiCheat(g)
}

// messaggioAntiCheat: con l'anti-imbroglio non tocco mai i file, traduco sullo schermo.
func messaggioAntiCheat(nome string) string {
	return "Per non rischiare con l'anti-imbroglio, " + nome + " viene tradotto sullo schermo."
}

func applicaTraduzione(nome, cartella string) {
	antiCheat := haAntiCheat(cartella)
	defer func() {
		if r := recover(); r != nil {
			logErrore("applica-traduzione", r, debug.Stack())
			emetti("guidata", map[string]any{"nome": nome, "errore": "Errore inatteso: riprova (dettagli in logs\\crash.txt)."})
		}
	}()
	passo := func(p int, testo string) {
		emetti("guidata", map[string]any{"nome": nome, "percento": p, "testo": testo})
	}
	fallito := func(testo string) {
		emetti("guidata", map[string]any{"nome": nome, "errore": testo})
	}
	passo(5, "Controllo il traduttore veloce…")
	if e := aspettaMotore(); e != "" {
		fallito(e + " (o con il pulsante qui sopra), poi rifai Applica.")
		return
	}
	// lingue scelte: scarico i modelli che mancano (il giocatore ha visto la dimensione al passo 3)
	i := leggiImp()
	da, a := i.LinguaDa, linguaArrivo(i)
	riconosci := da == "" || da == "auto"
	if riconosci {
		da = "en"
	}
	// prima la lettura: inutile scaricare decine di MB se Windows non sa leggere questa lingua
	if _, ok := ocrPerLingua(da); !ok {
		fallito("Windows su questo PC non sa ancora leggere il " + nomeLingua(da) + ": aggiungi la lingua nelle Impostazioni di Windows (pulsante al passo 1), poi riprova. Senza, la traduzione dello schermo non può funzionare.")
		return
	}
	if !coppiaPronta(da, a) {
		mb := byteCoppie(modelliServono(dirApp, da, a, presenteCoppia)) / 1048576
		passo(8, fmt.Sprintf("Scarico i modelli liberi per %s → %s (%d MB)…", nomeLingua(da), nomeLingua(a), mb))
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		err := scaricaLingue(ctx, da, a, func(p int) {
			passo(8+p*15/100, fmt.Sprintf("Scarico i modelli liberi per %s → %s… %d%%", nomeLingua(da), nomeLingua(a), p))
		})
		cancel()
		if err != nil {
			fallito("Scaricamento non riuscito: " + err.Error())
			return
		}
		ricaricaMozilla()
		aspettaCoppia(da, a) // il motore rilegge i modelli: aspetto che risponda (al massimo 20 s)
	}
	passo(25, "Preparo la libreria delle traduzioni di "+nome+"…")
	dir := filepath.Join(dirApp, "librerie")
	_ = os.MkdirAll(dir, 0o755)
	file := fileLibreriaGioco(cartella, nome, nomeRiconosciuto(cartella))
	prima := ContaLibreria(file)
	passo(45, "Provo il traduttore sulle scritte più comuni dei giochi…")
	// prova vera: qualche scritta tipica, tradotta con lo stesso traduttore del gioco
	t0 := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	prove := []string{"Continue", "Settings", "Quit to desktop", "You need a key to open this door."}
	ok := 0
	for _, s := range prove {
		if r := traduttore.TraduciScritta(ctx, s); r.Tradotto != "" && r.Errore == "" {
			ok++
		}
	}
	if ok == 0 {
		fallito("Il traduttore non risponde: chiudi e riapri Tasso, poi riprova.")
		return
	}
	ms := time.Since(t0).Milliseconds() / int64(len(prove))
	passo(70, "Controllo il correttore IA…")
	ia := "non installato: tradurrò con il traduttore veloce"
	if iaPresenteSvelto() {
		switch {
		case leggiImp().Motore == "mozilla":
			ia = "spento per questo gioco: traduco con il traduttore veloce"
		case !iaAdattaPC():
			ia = "installato, ma su questo PC rallenterebbe il gioco: traduco con il traduttore veloce"
		default:
			ia = "installato: migliorerà le traduzioni mentre giochi, se la scheda video ha spazio"
		}
	}
	passo(85, "Ricordo che "+nome+" va tradotto…")
	impostaGiocoTradotto(cartella, true)
	time.Sleep(300 * time.Millisecond)
	var esito string
	if riconosci {
		esito = fmt.Sprintf(testoTasso("Pronto. Ora premi Apri il gioco: quando vedo scritte da tradurre le traduco al loro posto e ti dico se funziona (%d ms a scritta).",
			"Ready. Now press Open the game: when I see text to translate I translate it in place and tell you if it works (%d ms per text)."), ms)
	} else {
		esito = fmt.Sprintf(testoTasso("Pronto. Ora premi Apri il gioco: quando vedo scritte in %s le traduco al loro posto e ti dico se funziona (%d ms a scritta).",
			"Ready. Now press Open the game: when I see %s text I translate it in place and tell you if it works (%d ms per text)."), nomeLingua(da), ms)
	}
	esito += testoTasso(" Correttore IA ", " AI corrector ") + trTasso(ia) + "."
	if prima > 0 {
		esito += fmt.Sprintf(" Ho già %d traduzioni salvate di questo gioco.", prima)
	}
	if antiCheat {
		esito += " " + messaggioAntiCheat(nome)
	}
	emetti("guidata", map[string]any{"nome": nome, "percento": 100, "fatto": true, "modo": "schermo", "testo": esito})
	emetti("aggiorna", stato())
}
