package main

import (
	"sync"
	"testing"
	"time"
)

// Dopo un crash i programmi chiusi dal Boost si riaprono solo senza un gioco aperto, anche se il
// gioco non è riconosciuto ma è a schermo intero.
func TestRiapriAllAvvio(t *testing.T) {
	if !riapriAllAvvio(true, false, false) {
		t.Fatal("niente gioco: si riapre")
	}
	if riapriAllAvvio(true, true, false) || riapriAllAvvio(true, false, true) || riapriAllAvvio(false, false, false) {
		t.Fatal("con un gioco riconosciuto o a schermo intero (o niente da riaprire) non si riapre")
	}
}

// reopenMu tiene una riapertura alla volta: due chiamate insieme non lavorano mai in parallelo.
func TestReopenMuUnaAllaVolta(t *testing.T) {
	var dentro, max int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reopenMu.Lock()
			mu.Lock()
			dentro++
			if dentro > max {
				max = dentro
			}
			mu.Unlock()
			time.Sleep(5 * time.Millisecond)
			mu.Lock()
			dentro--
			mu.Unlock()
			reopenMu.Unlock()
		}()
	}
	wg.Wait()
	if max != 1 {
		t.Fatalf("%d riaperture insieme", max)
	}
}
