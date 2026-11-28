//go:build windows

package traduce

import (
	"encoding/json"
	"testing"
)

// Prova sul PC vero di Tasso 3.2 (decisione di Sergio): "Traduci da solo appena apro un gioco" di
// base è spento, anche senza file delle impostazioni e con un file vecchio (prima della versione 2).
func TestTraduciDaSoloSpentoDiBase(t *testing.T) {
	base := Impostazioni{DimFont: 24, Motore: "auto", Versione: 2}
	if imp.TuttoAuto || imp.Auto || base.TuttoAuto {
		t.Error("di base Tasso non deve tradurre da solo")
	}
	var vecchio Impostazioni
	if err := json.Unmarshal([]byte(`{"tutto_auto":true,"auto":true}`), &vecchio); err != nil {
		t.Fatal(err)
	}
	if vecchio.Versione >= 2 {
		t.Fatal("un file senza versione deve contare come vecchio")
	}
}
