//go:build windows

package traduce

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"

	"fmt"
	"golang.org/x/sys/windows/registry"
)

// elencoGiochi: i giochi installati con Steam ed Epic (solo lettura dei file dei launcher).
func elencoGiochi() []map[string]any {
	var tutti []GiocoInstallato
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam`, registry.QUERY_VALUE); err == nil {
		if p, _, err := k.GetStringValue("SteamPath"); err == nil && p != "" {
			tutti = append(tutti, giochiSteam(filepath.FromSlash(p))...)
		}
		k.Close()
	}
	tutti = append(tutti, giochiEpic(filepath.Join(os.Getenv("ProgramData"), "Epic", "EpicGamesLauncher", "Data", "Manifests"))...)
	sort.Slice(tutti, func(i, j int) bool { return tutti[i].Nome < tutti[j].Nome })
	out := make([]map[string]any, 0, len(tutti))
	muGiochiNoti.Lock()
	giochiNoti = map[string]GiocoInstallato{}
	for _, g := range tutti {
		giochiNoti[chiaveGioco(g.Cartella)] = g
	}
	muGiochiNoti.Unlock()
	for _, g := range tutti {
		fileOK, fileMotivo := ModPossibile(g.Cartella)
		cop := ""
		if g.Copertina != "" {
			if b, err := os.ReadFile(g.Copertina); err == nil && len(b) < 4<<20 {
				tipo := "image/jpeg"
				if strings.HasSuffix(strings.ToLower(g.Copertina), ".png") {
					tipo = "image/png"
				}
				cop = "data:" + tipo + ";base64," + base64.StdEncoding.EncodeToString(b)
			}
		}
		out = append(out, map[string]any{
			"nome": g.Nome, "piattaforma": g.Piattaforma, "copertina": cop, "cartella": g.Cartella,
			"tradotto":    giocoTradotto(g.Cartella),
			"unity":       unityDi(g.Cartella),
			"in_file":     giocoInFile(g.Cartella),
			"file_ok":     fileOK,
			"file_motivo": fileMotivo,
			"avviabile":   g.Avvio != "" || exeGioco(g.Cartella) != "",
			"traduzioni":  ContaLibreria(fileLibreriaPer(filepath.Join(dirApp, "librerie"), g.Nome)),
			"anticheat":   conAntiCheat(GiocoRiconosciuto{Nome: g.Nome, Piattaforma: g.Piattaforma, Cartella: g.Cartella}),
		})
	}
	return out
}

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

// apriGioco avvia il gioco dal suo negozio (così Steam/Epic restano contenti) o, se non si può,
// direttamente dall'exe. Poi controlla davvero se la traduzione parte.
func apriGioco(cartella string) string {
	muGiochiNoti.Lock()
	g, ok := giochiNoti[chiaveGioco(cartella)]
	muGiochiNoti.Unlock()
	if !ok {
		return "Non trovo il gioco: aprilo tu dal suo negozio."
	}
	// dentro Tasso (amministratore) il gioco parte come utente normale, come con "Gioca"
	if g.Avvio != "" && Ospite.ApriComeUtente != nil {
		if !Ospite.ApriComeUtente(g.Avvio, "") {
			return "Non riesco ad aprirlo da " + g.Piattaforma + ": aprilo tu."
		}
	} else if exe := exeGioco(g.Cartella); exe != "" && Ospite.ApriComeUtente != nil {
		if !Ospite.ApriComeUtente(filepath.Join(g.Cartella, exe), "") {
			return "Non riesco ad aprirlo: aprilo tu."
		}
	} else if g.Avvio != "" {
		verbo, _ := windows.UTF16PtrFromString("open")
		dove, _ := windows.UTF16PtrFromString(g.Avvio)
		if err := windows.ShellExecute(0, verbo, dove, nil, nil, 1); err != nil {
			return "Non riesco ad aprirlo da " + g.Piattaforma + ": aprilo tu."
		}
	} else if exe := exeGioco(g.Cartella); exe != "" {
		cmd := exec.Command(filepath.Join(g.Cartella, exe))
		cmd.Dir = g.Cartella
		if err := cmd.Start(); err != nil {
			return "Non riesco ad aprirlo: aprilo tu."
		}
	} else {
		return "Non trovo come aprirlo: aprilo tu."
	}
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
	return ""
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
			dici(false, "Non ho visto partire "+g.Nome+": quando giochi, ti dico io se la traduzione funziona.")
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
	return map[string]any{
		"nome": nome, "piattaforma": piattaforma, "cartella": cartella,
		"tradotto":    giocoTradotto(cartella),
		"unity":       unityDi(cartella),
		"in_file":     giocoInFile(cartella),
		"file_ok":     fileOK,
		"file_motivo": fileMotivo,
		"avviabile":   g.Avvio != "" || exeGioco(cartella) != "",
		"traduzioni":  ContaLibreria(fileLibreriaPer(filepath.Join(dirApp, "librerie"), nome)),
		"anticheat":   conAntiCheat(GiocoRiconosciuto{Nome: nome, Piattaforma: piattaforma, Cartella: cartella}),
	}
}
