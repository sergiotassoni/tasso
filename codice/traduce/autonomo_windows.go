//go:build windows

package traduce

// Tasso Traduce senza la finestra di Tasso, per TassoProva.exe (cmd/tassoprova): niente
// amministratore, niente tasti, niente scritte sopra i giochi. Usa i modelli già scaricati del
// traduttore (in %LOCALAPPDATA%\TassoTraduce) SOLO in lettura: la memoria delle traduzioni è una
// copia nella cartella dei risultati, la cartella dei dati di WebView2 è nuova.

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AvviaAutonomo prepara traduttore, frasi pronte e motore veloce. uscita: cartella scrivibile
// (risultati). Aspetta il motore veloce al massimo attesa; senza motore si va avanti con le sole
// frasi pronte (e lo dice il report).
func AvviaAutonomo(uscita string, attesa time.Duration) error {
	base, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	dirApp = filepath.Join(base, "TassoTraduce")
	if err := os.MkdirAll(uscita, 0o755); err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Join(dirApp, "logs"), 0o755)
	_ = os.MkdirAll(filepath.Join(dirApp, "lavoro"), 0o755) // la lettura del testo scrive qui le sue catture
	caricaImp()
	// copia della memoria: le traduzioni già fatte aiutano, ma l'originale non si tocca
	mem := filepath.Join(uscita, "memoria-copia.json")
	_ = copiaFile(filepath.Join(dirApp, "memoria-traduzioni-2.json"), mem)
	traduttore = NuovoTraduttore(mem, leggiImp().Glossario, moz)
	traduttore.Coppia = coppiaAttiva
	caricaCataloghi()
	caricaLiberi()
	srv, indirizzo, err := avviaServerLocale()
	if err != nil {
		return err
	}
	srvLocale = srv
	motoreMu.Lock()
	motoreURL = indirizzo
	motoreMu.Unlock()
	motoreDati = filepath.Join(uscita, "webview2")
	avviato.Store(true)
	if !mozillaPresente() {
		return fmt.Errorf("traduttore veloce non scaricato: solo frasi pronte")
	}
	assicuraMotore()
	fine := time.Now().Add(attesa)
	for !mozOK.Load() && time.Now().Before(fine) {
		time.Sleep(200 * time.Millisecond)
	}
	if !mozOK.Load() {
		return fmt.Errorf("traduttore veloce non pronto dopo %v: solo frasi pronte", attesa)
	}
	return nil
}

// StatoMotori: per il report e la finestrella.
func StatoMotori() string { return statoMotoriBanco() }
