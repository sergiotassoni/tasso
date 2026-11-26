//go:build collaudo

package apertura

import "os"

// Solo nella build di prova (-tags collaudo). Le modalità di prova (--passo=, --guardia=,
// --collaudo, --foto ...) leggono os.Args nei loro init, prima di main. Se questa copia è partita
// dall'Utilità di pianificazione (primo argomento ArgDaAttivita) tengo solo il segno e gli
// argomenti permessi, così nessun init di prova vede quello che ha passato chi ha avviato
// l'attività. Questo pacchetto non usa altri pacchetti di Tasso: il suo init gira prima di quelli
// di traduce (che lo importa apposta) e di main.
func init() {
	os.Args = SoloDaAttivita(os.Args)
}

// BuildCollaudo: questa è la build di prova.
const BuildCollaudo = true
