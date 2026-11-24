//go:build collaudo && windows

package traduce

import (
	"testing"
	"time"
)

func TestTempoCatturaDXGI(t *testing.T) {
	z := Zona{X: 0, Y: 0, W: 1581, H: 868}
	var tot time.Duration
	var errori int
	for i := 0; i < 30; i++ {
		t0 := time.Now()
		_, err := cattura(z)
		if err != nil {
			errori++
		}
		d := time.Since(t0)
		tot += d
		if i < 3 || i%10 == 0 {
			t.Logf("cattura %d: %v (DXGI pronta: %v)", i, d, dxgiPron && len(duplic) > 0 && duplic[0].pronta)
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Logf("media %v, errori %d", tot/30, errori)
}
