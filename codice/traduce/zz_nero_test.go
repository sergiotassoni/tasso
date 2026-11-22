//go:build collaudo && windows

package traduce

import (
	"testing"
	"time"
)

func TestCatturaNera(t *testing.T) {
	z := Zona{X: 0, Y: 0, W: 1200, H: 800}
	c, err := catturaClassica(z)
	t.Logf("classica: err %v nera %v", err, err == nil && catturaNera(c))
	if err == nil {
		salvaPNG(c, `C:\Users\Administrator\Desktop\TASSO TRADUTTORI\collaudo\prova-cattura-classica.png`)
	}
	for i := 0; i < 3; i++ {
		img, err := catturaDXGI(z)
		t.Logf("dxgi: err %v nera %v", err, err == nil && catturaNera(img))
		if err == nil && i == 2 {
			salvaPNG(img, `C:\Users\Administrator\Desktop\TASSO TRADUTTORI\collaudo\prova-cattura-dxgi.png`)
		}
		time.Sleep(300 * time.Millisecond)
	}
	for i, d := range duplic {
		t.Logf("monitor %d: %+v %dx%d pronta %v", i, d.schermo, d.w, d.h, d.pronta)
	}
}
