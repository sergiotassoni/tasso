//go:build windows

package traduce

import (
	"crypto/sha1"
	"encoding/hex"
	"path/filepath"
	"strings"
	"sync"
)

var (
	muAvvisate sync.Mutex
	avvisate   = map[string]bool{}
)

// avvisaVecchia: una volta per sessione, se in un gioco c'è ancora la traduzione della 3.1.1
// (non funziona più: il gioco si traduce sullo schermo). Non tocca niente: lo decidi tu.
func avvisaVecchia(base, chiave string) {
	m, ok := leggiManifestoVecchio(base, chiave)
	if !ok || !chiaveDellaCartella(chiave, m.Cartella) || m.Tipo == "unreal" {
		return
	}
	muAvvisate.Lock()
	gia := avvisate[chiave]
	avvisate[chiave] = true
	muAvvisate.Unlock()
	if gia {
		return
	}
	nome := m.Gioco
	if nome == "" {
		nome = filepath.Base(m.Cartella)
	}
	emetti("info", map[string]string{"testo": "In " + nome + " c'è la traduzione dentro il gioco della versione di prima di Tasso: ora lo traduco sullo schermo. Per rimetterla dentro il gioco, applica di nuovo la traduzione (Tasso toglie da solo quella vecchia)."})
}

// chiaveGiocoVecchia: la chiave della 3.1.1, senza cambiare "#", "%" e "?" nel nome.
func chiaveGiocoVecchia(cartella string) string {
	pulita := strings.ToLower(filepath.Clean(cartella))
	h := sha1.Sum([]byte(pulita))
	return filepath.Base(pulita) + "-" + hex.EncodeToString(h[:4])
}

// chiaveDellaCartella: la chiave è proprio quella della cartella scritta nell'elenco (nuova o 3.1.1).
func chiaveDellaCartella(chiave, cartella string) bool {
	return chiave == chiaveGioco(cartella) || chiave == chiaveGiocoVecchia(cartella)
}

// manifestoVecchioDi: l'elenco della 3.1.1 per questa cartella, se c'è (con la chiave nuova o vecchia),
// nella cartella protetta (copie spostate dalla 3.2) o in AppData. Restituisce anche dove l'ha trovato.
func manifestoVecchioDi(cartella string) (string, string, ManifestoMod, bool) {
	for _, base := range basiCopie() {
		for _, k := range []string{chiaveGioco(cartella), chiaveGiocoVecchia(cartella)} {
			if doppione(base, k) {
				continue // lo stesso gioco c'è anche nella cartella in uso: vale quello
			}
			if m, ok := leggiManifestoVecchio(base, k); ok && strings.EqualFold(filepath.Clean(m.Cartella), filepath.Clean(cartella)) {
				return base, k, m, true
			}
		}
	}
	return "", "", ManifestoMod{}, false
}

// togliVecchiaSeCe: se nel gioco c'è la traduzione messa dalla 3.1.1, la toglie (prima di rimetterla
// o quando la fermi). "" = niente da fare o tolta; altrimenti il motivo.
func togliVecchiaSeCe(cartella string) (trovata bool, motivo string) {
	base, k, m, ok := manifestoVecchioDi(cartella)
	if !ok {
		return false, ""
	}
	if m.Exe != "" && processoAperto(m.Exe) {
		return true, "Chiudi " + m.Gioco + " e riprova: con il gioco aperto non posso togliere i file."
	}
	if err := togliModVecchia(base, k, m); err != nil {
		return true, err.Error()
	}
	if k != chiaveGioco(cartella) {
		muImp.Lock()
		delete(imp.GiochiTradotti, k)
		muImp.Unlock()
		salvaImp()
	}
	return true, ""
}
