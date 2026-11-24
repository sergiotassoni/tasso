//go:build windows && collaudo

package traduce

// Tasso-collaudo.exe --prova-traduzione=<cartella>              schermate di una cartella
// Tasso-collaudo.exe --prova-traduzione-schermo=<cartella>[,secondi]  lo schermo, ogni 2 s (120 s di base)
// Usano il traduttore vero di Tasso (motore veloce e, se acceso, il correttore IA). Alla fine
// scrivono banco-prova.html e banco-prova.txt nella cartella e chiudono il programma.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func init() {
	for _, a := range os.Args[1:] {
		switch {
		case strings.HasPrefix(a, "--prova-traduzione="):
			dir := strings.Trim(strings.TrimPrefix(a, "--prova-traduzione="), `"`)
			go bancoCollaudo(func() error { return EseguiBancoCartella(dir) }, dir)
		case strings.HasPrefix(a, "--prova-traduzione-schermo="):
			v := strings.Trim(strings.TrimPrefix(a, "--prova-traduzione-schermo="), `"`)
			dir, sec := v, 120
			if i := strings.LastIndex(v, ","); i > 0 {
				if n, err := strconv.Atoi(strings.TrimSpace(v[i+1:])); err == nil && n > 0 {
					dir, sec = v[:i], n
				}
			}
			go bancoCollaudo(func() error {
				return EseguiBancoSchermo(dir, time.Duration(sec)*time.Second, 2*time.Second, 1600, nil, nil)
			}, dir)
		}
	}
}

// bancoCollaudo: aspetta che Tasso Traduce sia partito, accende il motore (e il correttore IA come
// impostato), fa il banco e chiude il programma.
func bancoCollaudo(fai func() error, dir string) {
	defer guard("banco-collaudo")
	for !avviato.Load() {
		time.Sleep(500 * time.Millisecond)
	}
	_ = os.MkdirAll(dir, 0o755)
	acceso.Store(true) // il correttore IA parte come scelto nelle impostazioni (senza tasti né scritte)
	sveglia()
	assicuraMotore()
	for i := 0; i < 300 && !mozOK.Load(); i++ {
		time.Sleep(200 * time.Millisecond)
	}
	esito := "fatto"
	if err := fai(); err != nil {
		esito = "errore: " + err.Error()
	}
	_ = os.WriteFile(filepath.Join(dir, "banco-esito.txt"), []byte(esito+"\r\n"+statoMotoriBanco()+"\r\n"), 0o644)
	if wv != nil {
		wv.Dispatch(func() { wv.Terminate() })
	}
}
