//go:build windows

package traduce

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"golang.org/x/sys/windows"
)

// mai il controllo lungo (5 GB) sul filo della finestra: quello svelto usa il risultato già pronto
func init() { iaMancante = func() bool { return !iaPresenteSvelto() } }

// spazioDisco: byte liberi sul disco della cartella (per chi usa Tasso, quote comprese).
func spazioDisco(dir string) (uint64, bool) {
	for d := dir; ; d = filepath.Dir(d) {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			p, err := windows.UTF16PtrFromString(d)
			if err != nil {
				return 0, false
			}
			var libero, tot, totLibero uint64
			if windows.GetDiskFreeSpaceEx(p, &libero, &tot, &totLibero) != nil {
				return 0, false
			}
			return libero, true
		}
		if filepath.Dir(d) == d {
			return 0, false
		}
	}
}

// scaricandoIA: lo scaricamento in corso (scaricando, uno alla volta) è quello dell'IA. La pagina
// lo usa per non dire "Sto scaricando" sotto il traduttore veloce mentre arriva l'IA.
var scaricandoIA atomic.Bool

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
	scaricandoIA.Store(true) // la pagina dice che si aspetta l'IA, non il traduttore veloce
	defer scaricandoIA.Store(false)
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
	var serve int64
	for _, f := range files {
		if st, err := os.Stat(filepath.Join(dirIA(), f.Nome)); err != nil || st.Size() != f.Byte {
			serve += f.Byte
		}
	}
	if libero, ok := spazioDisco(dirIA()); ok && libero < uint64(serve)+2<<30 {
		emetti("erroreScarica", map[string]string{"cosa": "ia", "errore": fmt.Sprintf("Spazio sul disco non sufficiente per l'IA: liberi %.1f GB, ne servono circa %.1f (più un po' di margine). Libera spazio e riprova.", float64(libero)/(1<<30), float64(serve)/(1<<30))})
		return
	}
	err := scaricaTutti(context.Background(), dirIA(), files, func(f, t int64) {
		emetti("avanzamento", map[string]interface{}{"cosa": "ia", "fatti": f, "totale": t})
	})
	if err == nil {
		err = estraiMotore(filepath.Join(dirIA(), risorseIA.Motore.Nome), dirMotoreIA())
	}
	if err != nil {
		emetti("erroreScarica", map[string]string{"cosa": "ia", "errore": erroreScaricaPerUtente("ia", err)})
		return
	}
	_ = os.Remove(filepath.Join(dirIA(), risorseIA.Motore.Nome)) // lo zip non serve più (l'impronta del motore resta nei file estratti)
	// i file nuovi sono verificati: tolgo quelli di prima (solo nella cartella dell'IA di Tasso)
	pulisciRiserveIA()
	emetti("scaricato", map[string]string{"cosa": "ia"})
	preparaMotori()
}
