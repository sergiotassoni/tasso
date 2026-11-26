package main

import (
	"strings"
	"testing"
)

// Regole di sicurezza della pulizia scritte negli script: se qualcuno le toglie per sbaglio, la prova lo dice.

func passo(t *testing.T, id string) Step {
	t.Helper()
	for _, s := range allSteps {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("passaggio %q non trovato", id)
	return Step{}
}

func TestCestinoNonSiSvuotaNellePulizie(t *testing.T) {
	r := passo(t, "recycle")
	if r.Level == 0 || strings.Contains(r.Script, "Clear-RecycleBin") || !strings.Contains(r.Script, "AddDays(-7)") {
		t.Error("il passaggio Cestino delle pulizie deve togliere solo quello che è lì da più di 7 giorni")
	}
	all := passo(t, "recycle_all")
	if all.Level != 0 {
		t.Error("\"Svuota tutto il Cestino\" non deve far parte di nessun livello di pulizia")
	}
	if !strings.Contains(all.Script, "$before - $after") {
		t.Error("lo spazio del Cestino si conta solo se è sparito davvero")
	}
}

func TestDriverWindowsUpdate(t *testing.T) {
	d := passo(t, "drivers")
	if !strings.Contains(d.Script, "$SkipDisplay = $true") || !strings.Contains(windowsUpdateScript, "DriverClass -eq 'Display'") {
		t.Error("il passaggio dei driver deve saltare i driver della scheda video")
	}
	if !strings.Contains(d.Script, "Ensure-RestorePoint") {
		t.Error("prima dei driver di Windows Update serve un punto di ripristino")
	}
	if !strings.Contains(driversInstallScript, "DriverClass -eq 'Display'") {
		t.Error("anche il riquadro Driver deve saltare la scheda video")
	}
}

func TestTurboNonChiudeFinestre(t *testing.T) {
	s := passo(t, "turbo").Script
	if strings.Contains(s, "Stop-Process") || !strings.Contains(s, "IsWindowVisible") {
		t.Error("Turbo non deve chiudere di forza né chiudere programmi con una finestra aperta")
	}
}

func TestAllegatiOutlookSoloCartellaGiusta(t *testing.T) {
	s := passo(t, "of_outlook_attach").Script
	if !strings.Contains(s, "Content.Outlook") || !strings.Contains(s, "Test-NoReparse") {
		t.Error("la cartella degli allegati presa da HKCU va controllata prima di spostare file")
	}
}

func TestOstNonConIMAP(t *testing.T) {
	s := passo(t, "of_outlook_ost").Script
	i, j := strings.Index(s, "IMAP Server"), strings.Index(s, "Rename-Item")
	if i < 0 || j < 0 || i > j {
		t.Error("con un account IMAP il .ost non si deve mettere da parte (controllo prima della rinomina)")
	}
}

func TestAppConDatiCopiatePrima(t *testing.T) {
	if !strings.Contains(bloatRemoveScript, "MicrosoftStickyNotes") || strings.Index(bloatRemoveScript, "Copy-Item") > strings.Index(bloatRemoveScript, "Remove-AppxPackage") {
		t.Error("i dati di Sticky Notes vanno copiati prima di togliere l'app")
	}
}

func TestPassiConFoto(t *testing.T) {
	for _, id := range []string{"privacy", "fix_game_fps", "fix_game_lag"} {
		if !passoConFoto[id] {
			t.Errorf("%s cambia impostazioni di Windows: prima serve la foto di com'era", id)
		}
	}
}

func TestDisinstallatoreScrivibileNonDaAmministratore(t *testing.T) {
	if !strings.Contains(uninstallScript, "Test-UserWritable") || !strings.Contains(orphanHelper, "function Test-UserWritable") {
		t.Error("un disinstallatore in una cartella scrivibile non deve partire da amministratore")
	}
}

// K4 (fase 2): il Cestino intero si svuota solo da "Svuota tutto il Cestino" (con la conferma della
// finestra); nessun altro passaggio lo svuota.
func TestCestinoInteroSoloConConferma(t *testing.T) {
	for _, s := range allSteps {
		if s.ID == "recycle_all" {
			continue
		}
		if strings.Contains(s.Script, "Clear-RecycleBin") || strings.Contains(s.Script, "SHEmptyRecycleBin") {
			t.Errorf("%s: solo recycle_all può svuotare tutto il Cestino", s.ID)
		}
	}
}

// K4 (fase 2): i messaggi dicono il vero. "Protezione sistema ... l'ho accesa" solo se adesso è accesa;
// "tolti N collegamenti" conta solo quelli spariti; la cache di Office "svuotata" solo se Tasso non ha rifiutato.
func TestMessaggiVeriPulizia(t *testing.T) {
	ricontrollo := "if ([int]$ri -eq 0) { $srOff = $false }"
	if !strings.Contains(passo(t, "backup").Script, ricontrollo) || !strings.Contains(nvRestoreScript, ricontrollo) {
		t.Error("dopo Enable-ComputerRestore va ricontrollato che Protezione sistema sia accesa davvero")
	}
	if !strings.Contains(passo(t, "privacy_traces").Script, "if (-not (Test-Path -LiteralPath $_.FullName)) { $n++ }") {
		t.Error("i file recenti tolti si contano solo se sono spariti")
	}
	if !strings.Contains(passo(t, "of_office_addins").Script, "if ($TassoRifiuto)") {
		t.Error("la cache dei componenti di Office si dice svuotata solo se Tasso non ha rifiutato")
	}
}
