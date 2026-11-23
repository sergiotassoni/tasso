//go:build windows

package traduce

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fmt"
)

func unityDi(cartella string) string {
	t, _ := TipoUnity(cartella)
	return t
}

var (
	muVerifiche      sync.Mutex
	verificheInCorso = map[string]bool{}
	muGiochiNoti     sync.Mutex
	giochiNoti       = map[string]GiocoInstallato{} // chiave → gioco, dall'ultimo elenco
)

// exeGioco: l'exe principale nella cartella (per Unity quello col suo _Data, se no il primo exe
// che non sia un programma di supporto).
func exeGioco(cartella string) string {
	if _, exe := TipoUnity(cartella); exe != "" {
		return exe
	}
	voci, _ := os.ReadDir(cartella)
	for _, v := range voci {
		n := strings.ToLower(v.Name())
		if v.IsDir() || filepath.Ext(n) != ".exe" || strings.Contains(n, "crash") || strings.Contains(n, "unins") || strings.Contains(n, "setup") || strings.Contains(n, "redist") || nonGiochiNegozi[n] {
			continue
		}
		return v.Name()
	}
	return ""
}

// avviaVerifica: un solo controllo alla volta per gioco.
func avviaVerifica(g GiocoInstallato) {
	muVerifiche.Lock()
	gia := verificheInCorso[chiaveGioco(g.Cartella)]
	verificheInCorso[chiaveGioco(g.Cartella)] = true
	muVerifiche.Unlock()
	if !gia {
		go func() {
			defer func() {
				muVerifiche.Lock()
				delete(verificheInCorso, chiaveGioco(g.Cartella))
				muVerifiche.Unlock()
			}()
			verificaTraduzione(g)
		}()
	}
}

// VerificaDopoAvvio (3.1): Tasso ha aperto il gioco con "Gioca" o "Apri il gioco"; se è tradotto
// controllo se la traduzione parte davvero e lo dico (evento "verifica").
func VerificaDopoAvvio(cartella string) string {
	if cartella == "" || !giocoTradotto(cartella) {
		return "no"
	}
	muGiochiNoti.Lock()
	g, ok := giochiNoti[chiaveGioco(cartella)]
	muGiochiNoti.Unlock()
	if !ok {
		return "no"
	}
	if !giocoInFile(cartella) && !acceso.Load() {
		return "spenta" // tradotto sullo schermo ma la traduzione è spenta: niente attese a vuoto
	}
	avviaVerifica(g)
	return "ok"
}

// verificaTraduzione: dopo "Apri il gioco" guardo se la traduzione parte DAVVERO, e lo dico.
func verificaTraduzione(g GiocoInstallato) {
	defer guard("verifica-traduzione")
	dici := func(ok bool, testo string) {
		emetti("verifica", map[string]any{"cartella": g.Cartella, "ok": ok, "testo": testo})
		emetti("info", map[string]string{"testo": testo})
	}
	chiave := chiaveGioco(g.Cartella)
	cartBasso := strings.ToLower(filepath.Clean(g.Cartella)) + `\`
	inGioco := func() bool {
		fg, _, _ := pGetForeground.Call()
		return fg != 0 && strings.HasPrefix(strings.ToLower(percorsoDiFinestra(fg)), cartBasso)
	}
	// aspetto che il gioco sia davanti (fino a 4 minuti: Steam e i giochi IL2CPP possono metterci)
	inizio := time.Now()
	for !inGioco() {
		if time.Since(inizio) > 4*time.Minute {
			dici(false, "Non ho visto partire il gioco ("+g.Nome+"): quando giochi, ti dico io se la traduzione funziona.")
			return
		}
		time.Sleep(time.Second)
	}
	richiestePrima := richiesteDelGioco(chiave)
	mostrataPrima := paginaMostrata.Load()
	for t := time.Now(); time.Since(t) < 5*time.Minute; time.Sleep(2 * time.Second) {
		if giocoInFile(g.Cartella) && ModNonPartita(g.Cartella) {
			segnaModNonPartita(chiave, g.Nome)
		}
		if n := richiesteDelGioco(chiave) - richiestePrima; n > 0 {
			dici(true, fmt.Sprintf("Funziona: %d scritte di %s tradotte dentro il gioco.", n, g.Nome))
			return
		}
		if l := linguaVista(chiave); l != "" {
			if _, a := coppiaAttiva(); l != a && coppiaPronta(l, a) {
				goto avanti
			}
			dici(false, messaggioLingua(g.Nome, l))
			return
		}
	avanti:
		if paginaMostrata.Load() != mostrataPrima && inGioco() {
			dici(true, "Funziona: ho tradotto una schermata di "+g.Nome+".")
			return
		}
	}
	dici(false, "In 5 minuti non ho visto scritte da tradurre in "+g.Nome+". Se ci sono scritte inglesi, premi "+nomeTasto("pagina")+" nel gioco.")
}

// InfoGioco (3.1, dentro Tasso): la libreria di Tasso conosce più negozi di Traduce (GOG, EA,
// Ubisoft, giochi aggiunti a mano…). Tasso registra qui il gioco con la sua cartella e il suo
// indirizzo di avvio, così "Applica" e "Apri il gioco" funzionano per tutti, e riceve le stesse
// informazioni dell'elenco di Traduce (anti-imbroglio, Unity, già tradotto…).
// in_file = la traduzione è GIÀ messa dentro il gioco; file_ok = si può mettere dentro il gioco.
func InfoGioco(nome, piattaforma, cartella, avvio string) map[string]any {
	if cartella == "" {
		return map[string]any{"nome": nome, "cartella": "", "errore": "Non so in che cartella è installato questo gioco."}
	}
	if st, err := os.Stat(cartella); err != nil || !st.IsDir() {
		return map[string]any{"nome": nome, "cartella": "", "errore": "La cartella del gioco non c'è più: aggiorna la libreria."}
	}
	g := GiocoInstallato{Nome: nome, Piattaforma: piattaforma, Cartella: cartella, Avvio: avvio}
	muGiochiNoti.Lock()
	if giochiNoti == nil {
		giochiNoti = map[string]GiocoInstallato{}
	}
	if vecchio, ok := giochiNoti[chiaveGioco(cartella)]; ok && g.Avvio == "" {
		g.Avvio = vecchio.Avvio
	}
	giochiNoti[chiaveGioco(cartella)] = g
	muGiochiNoti.Unlock()
	fileOK, fileMotivo := ModPossibile(cartella)
	tradotto := giocoTradotto(cartella)
	if tradotto {
		ricordaCartellaTradotta(cartella)
	}
	return map[string]any{
		"nome": nome, "piattaforma": piattaforma, "cartella": cartella,
		"tradotto":    tradotto,
		"unity":       unityDi(cartella),
		"in_file":     giocoInFile(cartella),
		"file_ok":     fileOK,
		"file_motivo": fileMotivo,
		"avviabile":   g.Avvio != "" || exeGioco(cartella) != "",
		"traduzioni":  ContaLibreria(fileLibreriaGioco(cartella, nome, nomeRiconosciuto(cartella))),
		"anticheat":   ConAntiCheat(GiocoRiconosciuto{Nome: nome, Piattaforma: piattaforma, Cartella: cartella}),
	}
}
