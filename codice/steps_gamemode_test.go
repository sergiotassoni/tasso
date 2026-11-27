package main

import (
	"strings"
	"testing"
)

// La Modalità Gioco di Windows la accendono solo Gaming e FPS; "Normale" (e "Rimetti com'era")
// la rimette com'era prima, anche spenta o mancante.
func TestModalitaGiocoWindows(t *testing.T) {
	set := "'AutoGameModeEnabled' 1"
	for _, m := range []string{"gaming", "fps"} {
		if !strings.Contains(modeScripts[m], set) {
			t.Errorf("%s deve accendere la Modalità Gioco di Windows", m)
		}
	}
	for _, m := range []string{"normale", "film", "silenzioso"} {
		if strings.Contains(modeScripts[m], set) {
			t.Errorf("%s non deve forzare la Modalità Gioco di Windows", m)
		}
	}
	if !strings.Contains(modeBase, "-Name 'GameMode'") || !strings.Contains(modeBase, "[string]$p.GameMode -eq 'none'") {
		t.Error("Save-Prev/Restore-Prev devono salvare e rimettere la Modalità Gioco")
	}
}

// F12-4 e F12-5: Film tiene lo schermo acceso anche a batteria, ma prima salva i tempi di prima;
// Silenzioso e il Profilo prestazioni rimettono il piano e il limite del processore com'erano.
func TestModalitaRimettonoTutto(t *testing.T) {
	for _, s := range []string{"monitor-timeout-dc 0", "standby-timeout-dc 0"} {
		if !strings.Contains(modeScripts["film"], s) {
			t.Errorf("Film senza %q", s)
		}
	}
	for _, k := range []string{"MonitorDc", "StandbyDc", "BalMonitorDc", "BalStandbyDc", "SaverCpuAc", "SaverCpuDc", "BalCpuAc", "BalCpuDc"} {
		if !strings.Contains(modeBase, "$p."+k) {
			t.Errorf("Restore-Prev non rimette %s", k)
		}
	}
	if !strings.Contains(modeBase, "Save-Prev32 $g $false") || !strings.Contains(modeBase, "Save-Prev32 ([string]$old.Plan) $true") {
		t.Error("Save-Prev deve salvare anche i valori della 3.2")
	}
	if !strings.Contains(modeScripts["normale"], "Remove-HighAdded") {
		t.Error("Normale deve togliere il piano aggiunto da Tasso")
	}
	off := perfProfileScript(false)
	if !strings.Contains(off, "\nRestore-PerfPrima\n") || strings.Contains(off, "\n[void](Use-Plan $BAL)\npowercfg /overlaysetactive 00000000") {
		t.Error("spegnendo il profilo deve tornare il piano di prima")
	}
	if !strings.Contains(perfProfileScript(true), "\nSave-PerfPrima\nUse-HighPerf\n") {
		t.Error("accendendo il profilo deve salvare prima il piano")
	}
	for _, s := range []string{modeBase, perfPrimaScript} {
		if strings.Contains(s, "Prestazioni migliori") {
			t.Error(`il nome della modalità di Windows è "Prestazioni ottimali"`)
		}
	}
}
