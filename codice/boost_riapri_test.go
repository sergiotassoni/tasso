package main

import (
	"os"
	"path/filepath"
	"strings"
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

// reopenBackground vera, chiamata 4 volte insieme: le riaperture non lavorano mai in parallelo e
// ogni programma chiuso dal Boost si riapre una volta sola (non due finestre dello stesso programma).
// Quello che non si apre resta nell'elenco per il prossimo "Rimetti com'era".
func TestReopenMuUnaAllaVolta(t *testing.T) {
	dir := t.TempDir()
	vecchiDir, vecchiGia, vecchiApri := appDir, riapriGiaAperto, riapriApri
	bMu.Lock()
	vecchioState, vecchiClosed := bState, bClosed
	bMu.Unlock()
	t.Cleanup(func() {
		appDir, riapriGiaAperto, riapriApri = vecchiDir, vecchiGia, vecchiApri
		bMu.Lock()
		bState, bClosed = vecchioState, vecchiClosed
		bMu.Unlock()
	})
	appDir = dir
	var progs []string
	for _, n := range []string{"discord.exe", "spotify.exe", "steamwebhelper.exe"} {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		progs = append(progs, p)
	}
	bMu.Lock()
	bState = &boostState{History: map[string][]*fpsResult{}}
	bClosed = append([]string(nil), progs...)
	bMu.Unlock()
	var mu sync.Mutex
	var dentro, max int
	aperti := map[string]int{}
	riapriGiaAperto = func(string) bool { return false }
	riapriApri = func(_ string, args string) bool {
		mu.Lock()
		dentro++
		if dentro > max {
			max = dentro
		}
		aperti[args]++
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		mu.Lock()
		dentro--
		mu.Unlock()
		return !strings.Contains(args, "spotify") // spotify non si apre: deve restare da riaprire
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reopenBackground()
		}()
	}
	wg.Wait()
	if max != 1 {
		t.Fatalf("%d riaperture insieme", max)
	}
	for _, p := range progs {
		n := aperti["\""+p+"\""]
		if strings.Contains(p, "spotify") {
			if n < 1 {
				t.Errorf("%s: mai provato", p)
			}
			continue
		}
		if n != 1 {
			t.Errorf("%s aperto %d volte, volevo 1", p, n)
		}
	}
	bMu.Lock()
	resta := append([]string(nil), bClosed...)
	bMu.Unlock()
	if len(resta) != 1 || !strings.Contains(resta[0], "spotify") {
		t.Errorf("da riaprire dopo: %v, volevo solo spotify", resta)
	}
}
