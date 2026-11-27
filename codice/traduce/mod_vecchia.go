package traduce

// Traduzioni dentro i giochi messe dalla 3.1.1: il suo elenco dei file aggiunti (manifesto.json)
// non ha la firma di Tasso, quindi la 3.1.2 non lo usa per rimettere gli originali. Lo uso solo
// per togliere i file di Tasso, con regole strette: niente si scrive nel gioco, si cancellano solo
// i file del caricatore (winhttp.dll e doorstop) e la cartella BepInEx si rinomina, non si cancella
// (dentro ci possono essere plugin dell'utente). Poi la traduzione si rimette da Tasso.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// leggiManifestoVecchio: l'elenco senza firma lasciato dalla 3.1.1. Un elenco con una firma che
// non torna non è "vecchio": è rovinato o cambiato, e non lo tocco.
func leggiManifestoVecchio(dirApp, chiave string) (ManifestoMod, bool) {
	var m ManifestoMod
	if !copieSenzaLink(dirApp, chiave, "manifesto.json") {
		return m, false
	}
	b, err := os.ReadFile(filepath.Join(cartellaCopie(dirApp, chiave), "manifesto.json"))
	if err != nil || json.Unmarshal(b, &m) != nil || m.Firma != "" || m.Cartella == "" || !filepath.IsAbs(m.Cartella) {
		return m, false
	}
	return m, true
}

// fileCaricatoreVecchio: i soli file che tolgo dalla cartella del gioco (BepInEx e dotnet a parte).
var fileCaricatoreVecchio = []string{"winhttp.dll", "doorstop_config.ini", ".doorstop_version"}

// percorsoSenzaLink: nessun pezzo del percorso completo c (dalla radice del disco) è un link o una
// giunzione: un altro programma non può spostare la cartella del gioco altrove.
func percorsoSenzaLink(c string) bool {
	c = filepath.Clean(c)
	vol := filepath.VolumeName(c)
	resto := strings.TrimPrefix(strings.TrimPrefix(c, vol), string(filepath.Separator))
	if resto == "" {
		return false // la radice del disco non è mai la cartella di un gioco
	}
	return senzaCollegamenti(vol+string(filepath.Separator), resto)
}

// traduzioneTassoVecchia: nel gioco c'è proprio la traduzione che metteva Tasso (XUnity con il
// traduttore di Tasso), non un BepInEx messo dall'utente.
func traduzioneTassoVecchia(cartella string) bool {
	x, _ := filepath.Glob(filepath.Join(cartella, "BepInEx", "plugins", "XUnity.AutoTranslator*"))
	cfg, err := os.ReadFile(filepath.Join(cartella, "BepInEx", "config", "AutoTranslatorConfig.ini"))
	return len(x) > 0 && err == nil && strings.Contains(string(cfg), "127.0.0.1:47321")
}

// togliModVecchia: toglie i file della traduzione della 3.1.1 dal gioco e mette da parte il suo
// elenco. Solo quando lo chiedi tu (Ferma, Applica di nuovo, Togli Tasso), mai da solo.
func togliModVecchia(dirApp, chiave string, m ManifestoMod) error {
	copie := cartellaCopie(dirApp, chiave)
	if !copieSenzaLink(dirApp, chiave, "") {
		return errors.New("la cartella delle copie di sicurezza porta altrove (link o giunzione): per sicurezza non tocco niente")
	}
	adesso := time.Now().Format("20060102-150405")
	metti := func() error { // l'elenco resta da parte (non si usa più): così non blocca la prossima traduzione
		return os.Rename(filepath.Join(copie, "manifesto.json"), filepath.Join(copie, "manifesto-3.1.1-"+adesso+".json"))
	}
	if m.Tipo == "unreal" {
		// la 3.1.1 non metteva traduzioni nei giochi Unreal (era spenta): un elenco così non lo
		// seguo, metto da parte solo l'elenco
		return metti()
	}
	if st, err := os.Lstat(m.Cartella); err != nil || !st.IsDir() || !percorsoSenzaLink(m.Cartella) {
		return errors.New("la cartella del gioco non c'è più o passa da un collegamento: non tolgo niente")
	}
	if !cartellaModValida(m.Cartella) {
		return errors.New("la cartella nell'elenco non è un gioco Unity: per sicurezza non tocco niente")
	}
	if !esisteVoce(filepath.Join(m.Cartella, "BepInEx")) && !esisteVoce(filepath.Join(m.Cartella, "winhttp.dll")) {
		return metti() // già tolta (a mano o verificando i file dal launcher)
	}
	if !traduzioneTassoVecchia(m.Cartella) {
		return errors.New("nel gioco c'è un BepInEx che non è quello di Tasso: per sicurezza non lo tocco")
	}
	var errori []string
	for _, n := range fileCaricatoreVecchio {
		p := filepath.Join(m.Cartella, n)
		st, err := os.Lstat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !st.Mode().IsRegular() {
			errori = append(errori, n) // cartella, link o giunzione con quel nome: non è mio
			continue
		}
		if err := os.Remove(p); err != nil {
			errori = append(errori, n)
		}
	}
	// BepInEx (con i plugin dell'utente dentro) e dotnet (BepInEx 6, se l'aveva aggiunto Tasso):
	// rinominate, non cancellate
	cartelle := []string{"BepInEx"}
	for _, a := range m.Aggiunti {
		if strings.EqualFold(a, "dotnet") {
			cartelle = append(cartelle, "dotnet")
		}
	}
	for _, n := range cartelle {
		p := filepath.Join(m.Cartella, n)
		st, err := os.Lstat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !st.IsDir() || st.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			errori = append(errori, n)
		} else if err := os.Rename(p, filepath.Join(m.Cartella, n+"-tolto-da-Tasso-"+adesso)); err != nil {
			errori = append(errori, n)
		}
	}
	if len(errori) > 0 {
		return fmt.Errorf("non sono riuscito a togliere: %s (il gioco è aperto?)", strings.Join(errori, ", "))
	}
	return metti()
}
