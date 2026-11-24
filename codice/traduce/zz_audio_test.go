//go:build audio && windows

package traduce

import (
	"os"
	"testing"
	"time"
)

func TestAscoltaPC(t *testing.T) {
	ferma := make(chan struct{})
	var tot int
	var picco float64
	fatto := make(chan error, 1)
	go func() {
		fatto <- ascoltaPC(ferma, func(x []float32) {
			tot += len(x)
			if r := rms(x); r > picco {
				picco = r
			}
		})
	}()
	time.Sleep(500 * time.Millisecond)
	if f := os.Getenv("WAV"); f != "" {
		suona(f)
	}
	time.Sleep(500 * time.Millisecond)
	close(ferma)
	select {
	case err := <-fatto:
		t.Logf("errore: %v", err)
	case <-time.After(2 * time.Second):
		t.Log("non si è fermato")
	}
	t.Logf("campioni 16k: %d (%.1f s), picco %.4f", tot, float64(tot)/16000, picco)
}
