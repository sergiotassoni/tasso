//go:build windows

package traduce

import (
	"context"
	"os"
	"path/filepath"
)

// scaricaIA: solo dopo il sì dell'utente, solo file con impronta verificata.
func scaricaIA() {
	defer guard("scarica-ia")
	if !motoreIADisponibile() {
		emetti("erroreScarica", map[string]string{"cosa": "ia", "errore": "Il motore potente non è ancora disponibile in questa versione."})
		return
	}
	if !scaricando.CompareAndSwap(false, true) {
		return
	}
	defer scaricando.Store(false)
	muImp.Lock()
	imp.IASi = true
	muImp.Unlock()
	salvaImp()
	files := []FileDaScaricare{risorseIA.Motore, risorseIA.Modello}
	if risorseIA.Media.URL != "" && risorseIA.Media.SHA256 != "" {
		files = append(files, risorseIA.Media) // IA grande compressa: quando il gioco lascia un po' meno memoria
	}
	if risorseIA.Scorta.URL != "" && risorseIA.Scorta.SHA256 != "" {
		files = append(files, risorseIA.Scorta) // IA piccola per quando il gioco lascia poca memoria
	}
	err := scaricaTutti(context.Background(), dirIA(), files, func(f, t int64) {
		emetti("avanzamento", map[string]interface{}{"cosa": "ia", "fatti": f, "totale": t})
	})
	if err == nil {
		err = estraiMotore(filepath.Join(dirIA(), risorseIA.Motore.Nome), dirMotoreIA())
	}
	if err != nil {
		emetti("erroreScarica", map[string]string{"cosa": "ia", "errore": err.Error()})
		return
	}
	_ = os.Remove(filepath.Join(dirIA(), risorseIA.Motore.Nome)) // lo zip non serve più (l'impronta del motore resta nei file estratti)
	// i file nuovi sono verificati: tolgo quelli di prima (solo nella cartella dell'IA di Tasso)
	pulisciRiserveIA()
	emetti("scaricato", map[string]string{"cosa": "ia"})
	preparaMotori()
}
