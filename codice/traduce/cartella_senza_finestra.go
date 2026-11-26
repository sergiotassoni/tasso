package traduce

import (
	"os"
	"path/filepath"
)

// PreparaCartella imposta solo la cartella dei dati di Tasso Traduce (%LOCALAPPDATA%\TassoTraduce),
// come fa Prepara, ma senza finestra né collegamenti: serve a Tasso.exe --togli-silenzioso (lo usa
// il disinstallatore) per trovare le copie dei giochi tradotti e togliere le traduzioni.
// Se Prepara è già stata chiamata non cambia niente.
func PreparaCartella() {
	if dirApp != "" {
		return
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	dirApp = filepath.Join(base, "TassoTraduce")
}
