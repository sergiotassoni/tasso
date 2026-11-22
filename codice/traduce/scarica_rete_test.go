//go:build rete

package traduce

import (
	"context"
	"testing"
)

// go test -tags rete -run TestScaricaMozillaVero : scarica davvero il traduttore Mozilla e ne controlla le impronte.
func TestScaricaMozillaVero(t *testing.T) {
	dir := t.TempDir()
	if err := scaricaTutti(context.Background(), dir, fileMozillaENIT, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
}
