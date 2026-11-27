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

// fileCaricatoreVecchio: i soli file che tolgo dalla cartella del gioco (BepInEx a parte).
var fileCaricatoreVecchio = []string{"winhttp.dll", "doorstop_config.ini", ".doorstop_version"}

// togliModVecchia: toglie i file della traduzione della 3.1.1 dal gioco e mette da parte il suo
// elenco. La chiave deve essere quella della cartella scritta nell'elenco (chiaveOK).
func togliModVecchia(dirApp, chiave string, m ManifestoMod) error {
	copie := cartellaCopie(dirApp, chiave)
	if !copieSenzaLink(dirApp, chiave, "") {
		return errors.New("la cartella delle copie di sicurezza porta altrove (link o giunzione): per sicurezza non tocco niente")
	}
	if st, err := os.Lstat(m.Cartella); err != nil || !st.IsDir() || st.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return errors.New("la cartella del gioco non c'è più o è un collegamento: non tolgo niente")
	}
	adesso := time.Now().Format("20060102-150405")
	if m.Tipo == "unreal" {
		// toglie solo il file con il nome della traduzione di Tasso, e solo se è un file normale
		if err := togliModUnreal(m); err != nil {
			return err
		}
	} else {
		if !cartellaModValida(m.Cartella) {
			return errors.New("la cartella nell'elenco non è un gioco Unity: per sicurezza non tocco niente")
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
		bep := filepath.Join(m.Cartella, "BepInEx")
		if st, err := os.Lstat(bep); err == nil {
			if !st.IsDir() || st.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
				errori = append(errori, "BepInEx")
			} else if err := os.Rename(bep, filepath.Join(m.Cartella, "BepInEx-tolto-da-Tasso-"+adesso)); err != nil {
				errori = append(errori, "BepInEx")
			}
		}
		if len(errori) > 0 {
			return fmt.Errorf("non sono riuscito a togliere: %s (il gioco è aperto?)", strings.Join(errori, ", "))
		}
	}
	// l'elenco resta da parte (non si usa più): così non blocca la prossima traduzione
	return os.Rename(filepath.Join(copie, "manifesto.json"), filepath.Join(copie, "manifesto-3.1.1-"+adesso+".json"))
}
