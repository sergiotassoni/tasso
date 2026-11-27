package main

// Ripristino: la foto di come era il PC prima di Tasso e il modo di rimettere tutto com'era.

import (
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows/registry"
)

var snapMu sync.Mutex

// ensureSnapshot salva le impostazioni originali di Windows la prima volta che Tasso sta per cambiarle.
// ensureSnapshot salva com'era Windows prima del primo cambio. Restituisce false se non ci
// riesce: in quel caso chi chiama non cambia niente (altrimenti "Rimetti com'era" non potrebbe funzionare).
func ensureSnapshot() bool {
	snapMu.Lock()
	defer snapMu.Unlock()
	setMu.Lock()
	has := len(settings.Snapshot) > 0
	setMu.Unlock()
	if has {
		return true
	}
	r := runPS(snapshotScript, 2*time.Minute, nil)
	if r.JSON == "" || !json.Valid([]byte(r.JSON)) {
		r = runPS(snapshotScript, 2*time.Minute, nil) // secondo tentativo
		if r.JSON == "" || !json.Valid([]byte(r.JSON)) {
			return false
		}
	}
	updateSettings(func(s *Settings) {
		if len(s.Snapshot) > 0 {
			return // c'è già quella vera (per esempio dal file ricaricato): non la sostituisco
		}
		s.Snapshot = json.RawMessage(r.JSON)
		s.SnapshotAt = time.Now().Format("2006-01-02 15:04")
	})
	return true
}

// rememberStartup ricorda com'era un programma all'avvio prima del primo cambio fatto da Tasso.
func rememberStartup(scope, name string, newState bool) {
	id := scope + "|" + name
	updateSettings(func(s *Settings) {
		if s.StartupOrig == nil {
			s.StartupOrig = map[string]bool{}
		}
		if _, ok := s.StartupOrig[id]; !ok {
			s.StartupOrig[id] = !newState
		}
	})
}

// restoreOriginal rimette le impostazioni di Windows com'erano prima di Tasso.
// ok = false se non ha potuto rimettere tutto (Tasso occupato o script non arrivato alla fine).
func restoreOriginal() (msgs []string, ok bool) {
	msgs, ok, avvioNo := restoreOriginalDett()
	return msgs, ok && len(avvioNo) == 0
}

// restoreOriginalDett: come restoreOriginal, ma i programmi all'avvio che non sono tornati com'erano
// non contano in ok: vengono restituiti a parte (avvioNo), così "Togli Tasso dal PC" può avvisare e
// andare avanti invece di bloccarsi per sempre (un criterio di gruppo può impedire di rimetterli).
func restoreOriginalDett() (msgs []string, ok bool, avvioNo []string) {
	if !claimRun() {
		return []string{"Sta girando una pulizia: aspetta che finisca, poi rimetti com'era."}, false, nil
	}
	defer releaseRun()
	profileForget()
	// Prima del modeMu (ordine dei blocchi: bOpMu poi modeMu): se il Boost ha cambiato qualcosa
	// (programmi chiusi, scheda video, mouse, modalità) lo rimetto anche qui.
	if boostDaRimettere() {
		msgs = append(msgs, boostOff(true)...)
	}
	modeMu.Lock()
	defer modeMu.Unlock()
	setMu.Lock()
	snap := append(json.RawMessage(nil), settings.Snapshot...)
	orig := map[string]bool{}
	for k, v := range settings.StartupOrig {
		orig[k] = v
	}
	setMu.Unlock()

	setMu.Lock()
	var tuned []string
	for id := range settings.GameTunes {
		tuned = append(tuned, id)
	}
	hasCmds := len(settings.GamingCmds) > 0
	setMu.Unlock()
	if hasCmds {
		msgs = append(msgs, gamingResetAll()...)
	}
	msgs = append(msgs, restoreUsbAudio()...)
	// Ottimizza tutto (3.2): anche le sue modifiche. Se qualcuna non torna lo dico, ma non blocca
	// (si riprova dalla sua pagina; "Togli Tasso" la mette con le copie di sicurezza)
	ottMsgs, _ := msgRimettiOtt()
	msgs = append(msgs, ottMsgs...)
	busy := 0
	for _, id := range tuned {
		if gameTuneBusy(id) {
			busy++
			continue
		}
		gameTuneUndoQuiet(id, false)
	}
	if len(tuned) > busy {
		msgs = append(msgs, "Regolazioni dei giochi tolte.")
	}
	if busy > 0 {
		msgs = append(msgs, "Un gioco regolato è aperto: chiudilo e premi di nuovo per togliere anche la sua regolazione.")
	}
	if len(snap) == 0 && len(orig) == 0 {
		if len(msgs) > 0 {
			return msgs, busy == 0, nil
		}
		return []string{"Tasso non ha ancora cambiato impostazioni di Windows: non c'è niente da rimettere."}, true, nil
	}
	snapOK := true
	if len(snap) > 0 {
		snapOK = runRestoreScript(snap, func(m string) { msgs = append(msgs, m) })
		if !snapOK {
			msgs = append(msgs, "Non sono riuscito a rimettere tutte le impostazioni di Windows: le tengo salvate, così puoi riprovare.")
		}
	}
	n := 0
	var falliti []string         // nomi da mostrare all'utente
	rimasti := map[string]bool{} // quelli da riprovare la prossima volta
	for id, enabled := range orig {
		parts := strings.SplitN(id, "|", 2)
		if len(parts) != 2 || !scopeAvvioNoto(parts[0]) {
			continue // voce rovinata: non la riprovo per sempre
		}
		r := runPSArgs(startupSetScript, map[string]interface{}{"scope": parts[0], "name": parts[1], "enable": enabled}, time.Minute, nil)
		if !startupRimessoOK(r) {
			falliti = append(falliti, nomeProgrammaAvvio(parts[1]))
			rimasti[id] = enabled
			continue
		}
		n++
	}
	if n > 0 && len(falliti) == 0 {
		msgs = append(msgs, "Programmi all'avvio rimessi com'erano.")
	}
	if len(falliti) > 0 {
		msgs = append(msgs, msgAvvioFalliti(falliti))
	}
	updateSettings(func(s *Settings) {
		if snapOK {
			s.Snapshot = nil
			s.SnapshotAt = ""
			s.Mode = "originale"
		}
		s.StartupOrig = nil
		if len(rimasti) > 0 {
			s.StartupOrig = rimasti // li tengo: premendo di nuovo "Rimetti com'era" riprovo
		}
	})
	return msgs, snapOK && busy == 0, falliti
}

// Risparmio energetico delle porte USB cambiato da "L'audio gracchia": il valore di prima sta in
// HKCU\Software\Tasso\AudioUSB (nome = GUID del piano energetico, valore = numero di prima).
const (
	usbAudioKey = `Software\Tasso\AudioUSB`
	usbSubGUID  = "2a737441-1930-4402-8d77-b2bebba308a3"
	usbSetGUID  = "48e6b7a6-50f5-4782-a5d4-53bb8f07e226"
)

// restoreUsbAudio rimette il risparmio energetico delle porte USB com'era prima di Tasso.
// Il registro dell'utente lo può scrivere chiunque: accetto solo GUID veri e i valori 0 o 1.
func restoreUsbAudio() []string {
	k, err := registry.OpenKey(registry.CURRENT_USER, usbAudioKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return nil
	}
	names, _ := k.ReadValueNames(50)
	done, failed := 0, 0
	for _, n := range names {
		v, _, err := k.GetIntegerValue(n)
		if err != nil || !snapGUID.MatchString(n) || v > 1 {
			_ = k.DeleteValue(n) // valore strano: non lo uso
			continue
		}
		if _, err := runHidden("powercfg.exe", "/query", n); err != nil {
			_ = k.DeleteValue(n) // il piano energetico non c'è più
			continue
		}
		if _, err := runHidden("powercfg.exe", "/setacvalueindex", n, usbSubGUID, usbSetGUID, strconv.FormatUint(v, 10)); err != nil {
			failed++ // lo tengo salvato per riprovare
			continue
		}
		_ = k.DeleteValue(n)
		done++
	}
	rest, _ := k.ReadValueNames(1)
	k.Close()
	if len(rest) == 0 {
		_ = registry.DeleteKey(registry.CURRENT_USER, usbAudioKey)
	}
	var msgs []string
	if done > 0 {
		_, _ = runHidden("powercfg.exe", "/setactive", "SCHEME_CURRENT")
		msgs = append(msgs, "Risparmio energetico delle porte USB: rimesso com'era.")
	}
	if failed > 0 {
		msgs = append(msgs, "Non sono riuscito a rimettere il risparmio energetico delle porte USB: riprova.")
	}
	return msgs
}

// runRestoreScript rimette la foto delle impostazioni e dice se lo script è arrivato davvero alla fine.
func runRestoreScript(snap json.RawMessage, onMsg func(string)) bool {
	args := sanitizeSnapshot(snap)
	if args == nil {
		return false
	}
	ok := false
	r := runPSArgs(restoreScript, args, 3*time.Minute, func(m string) {
		if m == "RESTORE_OK" {
			ok = true
			return
		}
		if onMsg != nil {
			onMsg(m)
		}
	})
	return ok && r.Err == nil
}

func restoreAndMaybePause(pause bool) string {
	return async("restore", func() {
		msgs, ok := restoreOriginal()
		// Se non ha rimesso tutto, Tasso resta acceso: spento non potrebbe più avvisarti.
		paused := pause && ok
		if paused {
			setPaused(true)
		}
		emit("onRestoreDone", map[string]interface{}{"msgs": msgs, "ok": ok, "paused": paused, "settings": getSettings()})
	})
}

const snapshotScript = `
function Q([string]$scheme, [string]$sub, [string]$set) {
  $o = powercfg /query $scheme $sub $set 2>$null
  if ($LASTEXITCODE -ne 0 -or -not $o) { return $null }
  $hex = @([regex]::Matches(($o -join ' '), '0x[0-9a-fA-F]{8}') | ForEach-Object { $_.Value })
  if ($hex.Count -ge 2) { return @([Convert]::ToInt64($hex[$hex.Count - 2], 16), [Convert]::ToInt64($hex[$hex.Count - 1], 16)) }
  return $null
}
function RV([string]$path, [string]$name) {
  try {
    $k = Get-Item -LiteralPath $path -ErrorAction Stop
    $v = $k.GetValue($name, $null)
    if ($v -eq $null) { return [ordered]@{ path = $path; name = $name; exists = $false } }
    return [ordered]@{ path = $path; name = $name; exists = $true; value = $v; kind = $k.GetValueKind($name).ToString() }
  } catch { return [ordered]@{ path = $path; name = $name; exists = $false } }
}
$guidRx = '[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}'
$active = [regex]::Match(((powercfg /getactivescheme) -join ' '), $guidRx).Value
$schemes = New-Object System.Collections.ArrayList
$seen = @{}
foreach ($g in @($active, '381b4222-f694-41f0-9685-ff5bb260df2e', '8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c', 'a1841308-3541-4fab-bc81-f71556f20b4a')) {
  if (-not $g -or $seen.ContainsKey($g.ToLower())) { continue }
  $seen[$g.ToLower()] = $true
  $cpu = Q $g 'SUB_PROCESSOR' 'PROCTHROTTLEMAX'
  if ($cpu -eq $null) { continue }
  [void]$schemes.Add([ordered]@{ guid = $g; cpu = $cpu; video = (Q $g 'SUB_VIDEO' 'VIDEOIDLE'); sleep = (Q $g 'SUB_SLEEP' 'STANDBYIDLE') })
}
$ps = 'HKLM:\SYSTEM\CurrentControlSet\Control\Power\User\PowerSchemes'
$ovAc = ''; $ovDc = ''
try { $pk = Get-Item -LiteralPath $ps -ErrorAction Stop; $ovAc = [string]$pk.GetValue('ActiveOverlayAcPowerScheme', ''); $ovDc = [string]$pk.GetValue('ActiveOverlayDcPowerScheme', '') } catch {}
$cdm = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\ContentDeliveryManager'
$reg = New-Object System.Collections.ArrayList
$list = @(
  @('HKCU:\Software\Microsoft\Windows\CurrentVersion\PushNotifications', 'ToastEnabled'),
  @('HKCU:\Software\Microsoft\GameBar', 'AutoGameModeEnabled'),
  @('HKCU:\Control Panel\Mouse', 'MouseSpeed'),
  @('HKCU:\Control Panel\Mouse', 'MouseThreshold1'),
  @('HKCU:\Control Panel\Mouse', 'MouseThreshold2'),
  @('HKCU:\Software\Microsoft\Windows\CurrentVersion\AdvertisingInfo', 'Enabled'),
  @('HKCU:\Software\Microsoft\Windows\CurrentVersion\Privacy', 'TailoredExperiencesWithDiagnosticDataEnabled'),
  @('HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced', 'ShowSyncProviderNotifications'),
  @('HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced', 'Start_IrisRecommendations'),
  @('HKCU:\Software\Policies\Microsoft\Windows\Explorer', 'DisableSearchBoxSuggestions'),
  @('HKLM:\SOFTWARE\Policies\Microsoft\Windows\DataCollection', 'AllowTelemetry'),
  @('HKCU:\Software\Microsoft\Input\TIPC', 'Enabled'),
  @('HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\SystemRestore', 'SystemRestorePointCreationFrequency')
)
foreach ($n in @('SilentInstalledAppsEnabled','SystemPaneSuggestionsEnabled','SoftLandingEnabled','SubscribedContent-338388Enabled','SubscribedContent-338389Enabled','SubscribedContent-338393Enabled','SubscribedContent-353694Enabled','SubscribedContent-353696Enabled','SubscribedContent-310093Enabled')) { $list += ,@($cdm, $n) }
foreach ($e in $list) { [void]$reg.Add((RV $e[0] $e[1])) }
$o = [ordered]@{ active = $active; overlayAc = $ovAc; overlayDc = $ovDc; schemes = @($schemes); reg = @($reg) }
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject $o -Compress -Depth 5))
`

const restoreScript = `
foreach ($s in @($A.schemes)) {
  $g = [string]$s.guid
  if (-not $g) { continue }
  if ($s.cpu)   { powercfg /setacvalueindex $g SUB_PROCESSOR PROCTHROTTLEMAX ([string]$s.cpu[0]) 2>$null | Out-Null; powercfg /setdcvalueindex $g SUB_PROCESSOR PROCTHROTTLEMAX ([string]$s.cpu[1]) 2>$null | Out-Null }
  if ($s.video) { powercfg /setacvalueindex $g SUB_VIDEO VIDEOIDLE ([string]$s.video[0]) 2>$null | Out-Null; powercfg /setdcvalueindex $g SUB_VIDEO VIDEOIDLE ([string]$s.video[1]) 2>$null | Out-Null }
  if ($s.sleep) { powercfg /setacvalueindex $g SUB_SLEEP STANDBYIDLE ([string]$s.sleep[0]) 2>$null | Out-Null; powercfg /setdcvalueindex $g SUB_SLEEP STANDBYIDLE ([string]$s.sleep[1]) 2>$null | Out-Null }
}
if ($A.active) { powercfg /setactive ([string]$A.active) 2>$null | Out-Null }
$ov = [string]$A.overlayAc
if (-not $ov) { $ov = '00000000-0000-0000-0000-000000000000' }
powercfg /overlaysetactive $ov 2>$null | Out-Null
Say 'Piano energetico, processore, schermo e sospensione: rimessi com''erano.'
foreach ($r in @($A.reg)) {
  $path = [string]$r.path; $name = [string]$r.name
  if (-not $path -or -not $name) { continue }
  if ($r.exists) {
    $kind = [string]$r.kind
    if ($kind -ne 'String' -and $kind -ne 'DWord' -and $kind -ne 'QWord' -and $kind -ne 'ExpandString') { continue }
    if (-not (Test-Path -LiteralPath $path)) { New-Item -Path $path -Force | Out-Null }
    $val = $r.value
    if ($kind -eq 'DWord') { $val = [int]$val } elseif ($kind -eq 'QWord') { $val = [long]$val } else { $val = [string]$val }
    New-ItemProperty -LiteralPath $path -Name $name -Value $val -PropertyType $kind -Force | Out-Null
  } else {
    if (Test-Path -LiteralPath $path) { Remove-ItemProperty -LiteralPath $path -Name $name -ErrorAction SilentlyContinue }
  }
}
Say 'Notifiche, Modalità Gioco e privacy: rimesse com''erano.'
try {
  $m = Get-Item -LiteralPath 'HKCU:\Control Panel\Mouse'
  $t1 = [int]$m.GetValue('MouseThreshold1', '6'); $t2 = [int]$m.GetValue('MouseThreshold2', '10'); $sp = [int]$m.GetValue('MouseSpeed', '1')
  $sig = '[DllImport("user32.dll")] public static extern bool SystemParametersInfo(int a, int b, int[] c, int d);'
  Add-Type -MemberDefinition $sig -Name SPI -Namespace TassoMouseRestore | Out-Null
  [void][TassoMouseRestore.SPI]::SystemParametersInfo(4, 0, [int[]]($t1, $t2, $sp), 3)
  Say 'Mouse: rimesso com''era.'
} catch {}
Remove-Item -Path 'HKCU:\Software\Tasso\PrimaDelleModalita' -Recurse -Force -ErrorAction SilentlyContinue
Say 'RESTORE_OK'
`

// ---------------- Controllo della foto prima di rimetterla ----------------

// La foto sta in settings.json, che un programma normale (senza permessi) può modificare.
// Prima di rimetterla da amministratore tengo solo quello che snapshotScript salva davvero:
// le chiavi del registro del suo elenco, GUID veri per i piani energetici e numeri per powercfg.
// Qualsiasi altra chiave o valore scritto nel file viene ignorato.

var snapGUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

var snapRegAllowed = func() map[string]bool {
	cdm := `HKCU:\Software\Microsoft\Windows\CurrentVersion\ContentDeliveryManager`
	list := [][2]string{
		{`HKCU:\Software\Microsoft\Windows\CurrentVersion\PushNotifications`, "ToastEnabled"},
		{`HKCU:\Software\Microsoft\GameBar`, "AutoGameModeEnabled"},
		{`HKCU:\Control Panel\Mouse`, "MouseSpeed"},
		{`HKCU:\Control Panel\Mouse`, "MouseThreshold1"},
		{`HKCU:\Control Panel\Mouse`, "MouseThreshold2"},
		{`HKCU:\Software\Microsoft\Windows\CurrentVersion\AdvertisingInfo`, "Enabled"},
		{`HKCU:\Software\Microsoft\Windows\CurrentVersion\Privacy`, "TailoredExperiencesWithDiagnosticDataEnabled"},
		{`HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced`, "ShowSyncProviderNotifications"},
		{`HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced`, "Start_IrisRecommendations"},
		{`HKCU:\Software\Policies\Microsoft\Windows\Explorer`, "DisableSearchBoxSuggestions"},
		{`HKLM:\SOFTWARE\Policies\Microsoft\Windows\DataCollection`, "AllowTelemetry"},
		{`HKCU:\Software\Microsoft\Input\TIPC`, "Enabled"},
		{`HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\SystemRestore`, "SystemRestorePointCreationFrequency"},
	}
	for _, n := range []string{"SilentInstalledAppsEnabled", "SystemPaneSuggestionsEnabled", "SoftLandingEnabled", "SubscribedContent-338388Enabled", "SubscribedContent-338389Enabled", "SubscribedContent-338393Enabled", "SubscribedContent-353694Enabled", "SubscribedContent-353696Enabled", "SubscribedContent-310093Enabled"} {
		list = append(list, [2]string{cdm, n})
	}
	m := map[string]bool{}
	for _, e := range list {
		m[strings.ToLower(e[0])+"|"+strings.ToLower(e[1])] = true
	}
	return m
}()

// snapPair: [ac, dc] di powercfg, due numeri interi tra 0 e 0xFFFFFFFF.
func snapPair(v interface{}) ([]interface{}, bool) {
	a, ok := v.([]interface{})
	if !ok || len(a) != 2 {
		return nil, false
	}
	for _, x := range a {
		f, ok := x.(float64)
		if !ok || f < 0 || f > math.MaxUint32 || f != math.Trunc(f) {
			return nil, false
		}
	}
	return a, true
}

// snapList: PowerShell a volte scrive un elenco di un solo elemento come oggetto singolo.
func snapList(v interface{}) ([]interface{}, bool) {
	if a, ok := v.([]interface{}); ok {
		return a, true
	}
	if m, ok := v.(map[string]interface{}); ok {
		return []interface{}{m}, true
	}
	return nil, false
}

func sanitizeSnapshot(snap json.RawMessage) map[string]interface{} {
	var in map[string]interface{}
	if json.Unmarshal(snap, &in) != nil || in == nil {
		return nil
	}
	out := map[string]interface{}{}
	for _, k := range []string{"active", "overlayAc", "overlayDc"} {
		if g, ok := in[k].(string); ok && snapGUID.MatchString(g) {
			out[k] = g
		}
	}
	schemes := []interface{}{}
	if arr, ok := snapList(in["schemes"]); ok {
		for _, x := range arr {
			sc, ok := x.(map[string]interface{})
			if !ok {
				continue
			}
			g, ok := sc["guid"].(string)
			if !ok || !snapGUID.MatchString(g) {
				continue
			}
			o := map[string]interface{}{"guid": g}
			for _, f := range []string{"cpu", "video", "sleep"} {
				if p, ok := snapPair(sc[f]); ok {
					o[f] = p
				}
			}
			schemes = append(schemes, o)
		}
	}
	out["schemes"] = schemes
	reg := []interface{}{}
	if arr, ok := snapList(in["reg"]); ok {
		for _, x := range arr {
			r, ok := x.(map[string]interface{})
			if !ok {
				continue
			}
			path, _ := r["path"].(string)
			name, _ := r["name"].(string)
			if !snapRegAllowed[strings.ToLower(path)+"|"+strings.ToLower(name)] {
				continue
			}
			exists, _ := r["exists"].(bool)
			o := map[string]interface{}{"path": path, "name": name, "exists": exists}
			if exists {
				kind, _ := r["kind"].(string)
				switch kind {
				case "DWord", "QWord":
					f, ok := r["value"].(float64)
					if !ok || f != math.Trunc(f) {
						continue
					}
				case "String", "ExpandString":
					sv, ok := r["value"].(string)
					if !ok || len(sv) > 260 {
						continue
					}
				default:
					continue
				}
				o["kind"], o["value"] = kind, r["value"]
			}
			reg = append(reg, o)
		}
	}
	out["reg"] = reg
	return out
}

// startupRimessoOK: lo script ha davvero cambiato la voce (ha scritto "JSON ok" alla fine).
func startupRimessoOK(r psResult) bool {
	return r.Err == nil && !r.TooSlow && r.JSON == "ok"
}

// nomeProgrammaAvvio: il nome da mostrare. Per le app dello Store la voce è "pacchetto\app":
// tengo solo la parte dopo la barra.
func nomeProgrammaAvvio(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.LastIndex(name, "\\"); i >= 0 && i < len(name)-1 {
		name = name[i+1:]
	}
	if name == "" {
		return "?"
	}
	return name
}

// msgAvvioFalliti: il messaggio semplice per i programmi all'avvio che non sono tornati com'erano.
// I due pezzi fissi sono nei dizionari (i18n/*.json, "go"), i nomi restano come sono.
func msgAvvioFalliti(nomi []string) string {
	sort.Strings(nomi)
	if len(nomi) > 8 {
		nomi = append(nomi[:8:8], "…")
	}
	return "Non sono riuscito a rimettere com'erano questi programmi all'avvio: " + strings.Join(nomi, ", ") +
		". Li tengo segnati: premi di nuovo \"Rimetti com'era\" tra poco. Se non basta, cambiali a mano in Gestione attività, scheda \"App di avvio\"."
}

// scopeAvvioNoto: i posti che startupSetScript sa cambiare.
func scopeAvvioNoto(scope string) bool {
	switch scope {
	case "hkcu", "hklm", "hklm32", "fu", "fc", "appx":
		return true
	}
	return false
}

// msgAvvioFallitiTogli: per "Togli Tasso dal PC", che va avanti lo stesso.
func msgAvvioFallitiTogli(nomi []string) string {
	sort.Strings(nomi)
	if len(nomi) > 8 {
		nomi = append(nomi[:8:8], "…")
	}
	return "Attenzione: non sono riuscito a rimettere com'erano questi programmi all'avvio: " + strings.Join(nomi, ", ") +
		". Tolgo Tasso lo stesso. Se vuoi, cambiali a mano in Gestione attività, scheda \"App di avvio\"."
}
