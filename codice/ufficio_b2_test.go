package main

import (
	"testing"
	"time"
)

// I file "per tornare indietro" delle riparazioni: nome leggibile e data dal nome del file.
func TestNomeEDataUndo(t *testing.T) {
	n, d := nomeEDataUndo("viste-cartelle_20261126_101500")
	if n != "viste cartelle" || d.IsZero() || d.Format("02/01/2006 15:04") != "26/11/2026 10:15" {
		t.Fatalf("viste-cartelle: %q %v", n, d)
	}
	n, d = nomeEDataUndo("usb-audio_20261126_090000")
	if n != "usb audio" || d.Hour() != 9 {
		t.Fatalf("usb-audio: %q %v", n, d)
	}
	// Senza data nel nome: il nome resta, la data la prende chi chiama (data del file).
	n, d = nomeEDataUndo("strano")
	if n != "strano" || !d.IsZero() {
		t.Fatalf("strano: %q %v", n, d)
	}
	_ = time.Now
}

// Il Wi-Fi: un errore non diventa mai "nessuna rete salvata".
func TestWifiRisultato(t *testing.T) {
	if m, ok := wifiRisultato(nil).(map[string]interface{}); !ok || m["error"] == nil {
		t.Fatal("lettura fallita senza errore")
	}
	if m, ok := wifiRisultato(map[string]interface{}{"error": "cartella"}).(map[string]interface{}); !ok || m["error"] == nil {
		t.Fatal("cartella privata: manca l'errore")
	}
	if l, ok := wifiRisultato([]interface{}{}).([]interface{}); !ok || len(l) != 0 {
		t.Fatal("elenco vuoto vero: deve restare un elenco")
	}
	if l, ok := wifiRisultato(map[string]interface{}{"name": "Casa"}).([]interface{}); !ok || len(l) != 1 {
		t.Fatal("una rete sola: deve diventare un elenco di una rete")
	}
}
