//go:build !windows

package traduce

// Solo per le prove automatiche fuori da Windows (go test su Linux): le parti di Windows mancano,
// queste due servono alla logica comune.

import (
	"fmt"
	"os"
	"runtime/debug"
)

const sogliaCambiato = 4.0

func guard(dove string) {
	if r := recover(); r != nil {
		fmt.Fprintf(os.Stderr, "errore fermato in %s: %v\n%s\n", dove, r, debug.Stack())
	}
}
