package installa

// Diario: un file di testo semplice (%TEMP%\TassoSetup.log) con ogni passo e l'errore esatto.
// Si aggiunge in fondo; se supera 2 MB il vecchio diventa TassoSetup.log.1.

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

const NomeDiario = "TassoSetup.log"

var (
	diarioMu       sync.Mutex
	diarioPercorso string
)

// ImpostaDiario sceglie il file del diario ("" = nessun diario, come nei test).
func ImpostaDiario(p string) {
	diarioMu.Lock()
	diarioPercorso = p
	diarioMu.Unlock()
}

// PercorsoDiario: il file del diario ("" se non c'è).
func PercorsoDiario() string {
	diarioMu.Lock()
	defer diarioMu.Unlock()
	return diarioPercorso
}

// Diario scrive una riga con data e ora. Non fallisce mai (il diario non deve fermare niente).
func Diario(formato string, a ...any) {
	diarioMu.Lock()
	defer diarioMu.Unlock()
	if diarioPercorso == "" {
		return
	}
	if fi, err := os.Stat(diarioPercorso); err == nil && fi.Size() > 2<<20 {
		_ = os.Rename(diarioPercorso, diarioPercorso+".1")
	}
	f, err := os.OpenFile(diarioPercorso, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	riga := strings.ReplaceAll(fmt.Sprintf(formato, a...), "\n", " ")
	_, _ = fmt.Fprintf(f, "%s [%d] %s\r\n", time.Now().Format("2006-01-02 15:04:05.000"), os.Getpid(), riga)
}
