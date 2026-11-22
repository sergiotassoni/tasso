//go:build collaudo && windows

package traduce

import "testing"

func TestFotoSchermo(t *testing.T) {
	img, err := catturaClassica(Zona{X: 0, Y: 0, W: 1920, H: 1080})
	if err != nil {
		t.Fatal(err)
	}
	salvaPNG(img, `C:\Users\Administrator\Desktop\TASSO TRADUTTORI\collaudo\schermo-ora.png`)
}
