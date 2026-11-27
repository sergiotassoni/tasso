//go:build windows

package traduce

import (
	"crypto/sha1"
	"encoding/hex"
	"path/filepath"
	"strings"
)

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

// manifestoVecchioDi: l'elenco della 3.1.1 per questa cartella, se c'è (con la chiave nuova o vecchia).
func manifestoVecchioDi(cartella string) (string, ManifestoMod, bool) {
	for _, k := range []string{chiaveGioco(cartella), chiaveGiocoVecchia(cartella)} {
		if m, ok := leggiManifestoVecchio(dirApp, k); ok && strings.EqualFold(filepath.Clean(m.Cartella), filepath.Clean(cartella)) {
			return k, m, true
		}
	}
	return "", ManifestoMod{}, false
}

// togliVecchiaSeCe: se nel gioco c'è la traduzione messa dalla 3.1.1, la toglie (prima di rimetterla
// o quando la fermi). "" = niente da fare o tolta; altrimenti il motivo.
func togliVecchiaSeCe(cartella string) (trovata bool, motivo string) {
	k, m, ok := manifestoVecchioDi(cartella)
	if !ok {
		return false, ""
	}
	if m.Exe != "" && processoAperto(m.Exe) {
		return true, "Chiudi " + m.Gioco + " e riprova: con il gioco aperto non posso togliere i file."
	}
	if err := togliModVecchia(dirApp, k, m); err != nil {
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
