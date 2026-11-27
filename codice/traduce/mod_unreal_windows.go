//go:build windows

package traduce

// Mette la traduzione dentro i giochi Unreal (file .pak in ~mods), passo dopo passo.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

var fermaUnreal atomic.Bool

// applicaModUnreal: traduce i file di lingua di un gioco Unreal e aggiunge il pak "_P".
// passo e fallito sono quelli di applicaModGioco (avanzamento nella finestra).
func applicaModUnreal(nome, cartella string, antiCheat bool, passo func(int, string), fallito func(string)) {
	paks := CartellaPaksUnreal(cartella)
	exe := exeGioco(cartella)
	aperto := processoAperto(exe)
	if sh, _ := filepath.Glob(filepath.Join(filepath.Dir(filepath.Dir(paks)), "Binaries", "Win64", "*-Shipping.exe")); len(sh) > 0 {
		aperto = aperto || processoAperto(filepath.Base(sh[0]))
	}
	if aperto {
		fallito("Chiudi " + nome + " e riprova: con il gioco aperto non posso aggiungere il file della traduzione.")
		return
	}
	chiave := chiaveGioco(cartella)
	if _, gia := leggiManifesto(baseCopie(), chiave); gia {
		// rifaccio da capo (es. dopo un aggiornamento del gioco o un cambio di lingua)
		if err := TogliModGioco(baseCopie(), chiave); err != nil {
			fallito("Non riesco a togliere la traduzione di prima: " + err.Error())
			return
		}
	}
	if e := aspettaMotore(); e != "" {
		fallito(e + ", poi riprova.")
		return
	}
	da, a := coppiaAttiva()
	if da == "auto" || da == "" {
		da = "en"
	}
	if da == a {
		fallito("Le scritte del gioco e la lingua che vuoi sono uguali (" + nomeLingua(a) + "): scegli la lingua del gioco al passo 1 e riprova.")
		return
	}
	if GiocoHaLingua(paks, a) && !forzaPerProva("TASSO_FORZA_FILE") {
		fallito(nome + " ha già la sua traduzione in " + strings.ToLower(nomeLingua(a)) + ", fatta dagli autori del gioco: sceglila nelle impostazioni del gioco (di solito Opzioni → Lingua). È la traduzione migliore e non serve aggiungere niente.")
		return
	}
	passo(5, "Leggo le scritte del gioco…")
	files, sorg, err := LeggiLocresUnreal(paks, da)
	if err != nil {
		fallito("Non riesco a leggere le scritte del gioco: " + err.Error() + ". Usa la traduzione dello schermo, funziona lo stesso.")
		return
	}
	traduttore.ApriLibreria(fileLibreriaGioco(cartella, nome, nomeRiconosciuto(cartella)))
	if fermaUnreal.Load() {
		fallito("Traduzione fermata: il gioco resta com'era.")
		return
	}
	traduci := func(s string) (string, bool) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// traduttore "a gruppi" (più fili insieme) e solo frasi esatte dalla libreria
		ctx = soloEsatta(conLotto(ctx))
		dialogo := len([]rune(s)) >= 40 || strings.Contains(s, ". ")
		r := traduttore.TraduciPer(ctx, s, dialogo, sorg, a)
		return r.Tradotto, r.Errore == "" && r.Tradotto != ""
	}
	inizio := time.Now()
	if sorg == a {
		// lingua voluta = quella da cui parto (es. gioco in coreano, voglio l'inglese): copio
		traduci = func(s string) (string, bool) { return s, true }
	}
	n, err := TraduciLocres(files, traduci, func(fatte, tot int) {
		resto := ""
		if fatte > 20 {
			sec := time.Since(inizio).Seconds() / float64(fatte) * float64(tot-fatte)
			resto = fmt.Sprintf(" (mancano circa %d min)", int(sec/60)+1)
		}
		passo(10+80*fatte/max(tot, 1), fmt.Sprintf("Traduco le scritte del gioco: %d/%d%s…", fatte, tot, resto))
	}, fermaUnreal.Load)
	traduttore.SalvaMemoria()
	var ep *ErrorePanico
	if errors.As(err, &ep) {
		logErrore("traduci-locres", ep.Valore, ep.Stack)
		passo(4, "Errore interno mentre traducevo le scritte del gioco: il gioco è rimasto com'era. Lo traduco sullo schermo.")
		applicaTraduzione(nome, cartella)
		return
	}
	if err != nil || fermaUnreal.Load() {
		fallito("Traduzione interrotta: il gioco è rimasto com'era.")
		return
	}
	if n == 0 {
		fallito("Non sono riuscito a tradurre le scritte: il gioco è rimasto com'era.")
		return
	}
	passo(92, "Aggiungo il file della traduzione (gli originali non si toccano)…")
	m := ManifestoMod{Gioco: nome, Cartella: cartella, Tipo: "unreal", Exe: exe, Quando: time.Now(), Lingue: [2]string{sorg, a}, AntiCheat: antiCheat}
	// il manifesto prima del file: se Tasso si chiude a metà, "Ferma" sa cosa togliere
	rel, _ := filepath.Rel(cartella, filepath.Join(paks, "~mods", nomeModUnreal))
	m.Aggiunti = []string{rel}
	m.CreataMods = !esisteVoce(filepath.Join(paks, "~mods"))
	if err := scriviManifesto(baseCopie(), chiave, m); err != nil {
		fallito("Non riesco a salvare l'elenco dei file: " + err.Error())
		return
	}
	if _, _, err := ScriviModUnreal(cartella, paks, files); err != nil {
		_ = TogliModGioco(baseCopie(), chiave)
		fallito("Non sono riuscito a scrivere il file: " + err.Error() + ". Il gioco è rimasto com'era.")
		return
	}
	impostaGiocoTradotto(cartella, true)
	esito := fmt.Sprintf("Fatto: %d scritte tradotte. Premi Apri il gioco: lo vedrai in %s, con le scritte del gioco. Se il gioco si aggiorna, rifai Applica.", n, nomeLingua(a))
	emetti("guidata", map[string]any{"nome": nome, "percento": 100, "fatto": true, "modo": "file", "testo": esito})
	emetti("aggiorna", stato())
}
