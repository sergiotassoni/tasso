package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// F11-1: con la Modalità riunione accesa, cambiare modalità non deve salvare come "tuoi" le
// notifiche spente e il piano messi dalla riunione, e Silenzioso/Normale non devono riaccendere
// le notifiche in piena riunione.
func TestRiunioneECambioModalita(t *testing.T) {
	dir := t.TempDir()
	hp := "8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c"
	bal := "381b4222-f694-41f0-9685-ff5bb260df2e"

	// niente riunione: nessun valore della riunione
	if v := riunionePrima(filepath.Join(dir, "manca.json")); !strings.Contains(v, "$TassoInMeeting = $false") {
		t.Errorf("senza file la riunione deve risultare spenta: %q", v)
	}

	// riunione accesa da Normale: il file ha le notifiche e il piano di prima (con il BOM di PowerShell)
	f := filepath.Join(dir, "riunione.json")
	_ = os.WriteFile(f, []byte("\xef\xbb\xbf{\"noc\":\"\",\"toast\":\"1\",\"plan\":\""+bal+"\",\"wu\":true}"), 0o644)
	v := riunionePrima(f)
	for _, w := range []string{"$TassoInMeeting = $true", "$TassoMeetOk = $true", "$TassoMeetToast = '1'", "$TassoMeetPlan = '" + bal + "'", "$TassoMeetFile = '" + f + "'"} {
		if !strings.Contains(v, w) {
			t.Errorf("manca %q in %q", w, v)
		}
	}

	// toast mancante (Windows le teneva accese) e piano non valido: niente finisce nello script
	_ = os.WriteFile(f, []byte(`{"toast":"","plan":"x'; Remove-Item C:\\ #"}`), 0o644)
	v = riunionePrima(f)
	if !strings.Contains(v, "$TassoMeetToast = '1'") || !strings.Contains(v, "$TassoMeetPlan = ''") || strings.Contains(v, "Remove-Item") {
		t.Errorf("valori non controllati: %q", v)
	}
	_ = os.WriteFile(f, []byte(`{"toast":"0","plan":"`+hp+`"}`), 0o644)
	if v = riunionePrima(f); !strings.Contains(v, "$TassoMeetToast = '0'") {
		t.Errorf("toast 0 di prima della riunione perso: %q", v)
	}

	// file rovinato: riunione accesa ma valori sconosciuti (setMode allora non salva niente)
	_ = os.WriteFile(f, []byte("{rotto"), 0o644)
	if v = riunionePrima(f); !strings.Contains(v, "$TassoInMeeting = $true") || !strings.Contains(v, "$TassoMeetOk = $false") {
		t.Errorf("file rovinato: %q", v)
	}

	// Save-Prev usa i valori della riunione, non quelli di adesso
	if !strings.Contains(modeBase, "if ($TassoInMeeting -and $TassoMeetPlan) { $g = [string]$TassoMeetPlan }") ||
		!strings.Contains(modeBase, "if ($TassoInMeeting) { $tv = [int]$TassoMeetToast }") {
		t.Error("Save-Prev deve salvare notifiche e piano di prima della riunione")
	}
	// Restore-Prev non riaccende le notifiche e passa le tue alla riunione
	if !strings.Contains(modeBase, "if (-not $TassoInMeeting) { if ($null -ne $p.Toasts) { Set-Toasts ([int]$p.Toasts) } }") ||
		!strings.Contains(modeBase, "if (-not $TassoInMeeting) { Set-Toasts 1 }") ||
		!strings.Contains(modeBase, "$ms.toast = [string][int]$p.Toasts") {
		t.Error("Restore-Prev non deve riaccendere le notifiche durante la riunione")
	}
	// Silenzioso: Set-Toasts 1 solo senza riunione, e lo dice
	sil := strings.TrimPrefix(modeScripts["silenzioso"], modeBase)
	if strings.Count(sil, "Set-Toasts 1") != 1 || !strings.Contains(sil, "else { Set-Toasts 1 }") || !strings.Contains(sil, "restano spente finché è accesa la Modalità riunione") {
		t.Error("Silenzioso non deve riaccendere le notifiche durante la riunione")
	}
	// Normale con la riunione accesa non dice "notifiche" rimesse o attive
	nor := strings.TrimPrefix(modeScripts["normale"], modeBase)
	i := strings.Index(nor, "if ($TassoInMeeting) {")
	j := strings.Index(nor, "elseif ($rp)")
	if i < 0 || j < i || strings.Contains(nor[i:j], "notifiche attive") || strings.Contains(nor[i:j], "mouse, notifiche") {
		t.Error("Normale durante la riunione deve dire che le notifiche restano spente")
	}
}
