package traduce

import (
	"testing"
	"time"
)

func TestTempoLibreria(t *testing.T) {
	t0 := time.Now()
	caricaLiberi()
	carica := time.Since(t0)
	t1 := time.Now()
	n := 0
	for i := 0; i < 10000; i++ {
		if _, ok := fraseLiberaEsatta("Are you sure you want to quit?"); ok {
			n++
		}
	}
	t.Logf("caricamento %v, ricerca %v a frase (%d)", carica, time.Since(t1)/10000, n)
}
