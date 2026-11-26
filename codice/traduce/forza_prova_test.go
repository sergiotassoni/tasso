//go:build !collaudo

package traduce

import "testing"

// Nella versione pubblicata le variabili d'ambiente non accendono i rami sperimentali
// (e l'anti-cheat non si forza in nessun modo).
func TestForzaPerProvaSpentaFuoriDalCollaudo(t *testing.T) {
	t.Setenv("TASSO_FILE_UNREAL", "1")
	t.Setenv("TASSO_FORZA_FILE", "1")
	if forzaPerProva("TASSO_FILE_UNREAL") || forzaPerProva("TASSO_FORZA_FILE") {
		t.Fatal("fuori dal collaudo forzaPerProva deve essere sempre falsa")
	}
}
