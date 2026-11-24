//go:build collaudo && windows

package traduce

import (
	"testing"
	"time"
)

func TestDXGIDebug(t *testing.T) {
	err := preparaDXGI()
	t.Logf("prepara: %v, duplicatori %d", err, len(duplic))
	for i, d := range duplic {
		for k := 0; k < 5; k++ {
			t0 := time.Now()
			nuovo, e := d.aggiorna()
			t.Logf("monitor %d %dx%d tentativo %d: nuovo=%v %v (%v)", i, d.w, d.h, k, nuovo, e, time.Since(t0))
		}
	}
}
