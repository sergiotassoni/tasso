package main

import (
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"
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
func restoreOriginal() []string {
	if isRunning() {
		return []string{"Sta girando una pulizia: aspetta che finisca, poi rimetti com'era."}
	}
	profileForget()
	modeMu.Lock()
	defer modeMu.Unlock()
	setMu.Lock()
	snap := append(json.RawMessage(nil), settings.Snapshot...)
	orig := map[string]bool{}
	for k, v := range settings.StartupOrig {
		orig[k] = v
	}
	setMu.Unlock()

	var msgs []string
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
	for _, id := range tuned {
		gameTuneUndoQuiet(id)
	}
	if len(tuned) > 0 {
		msgs = append(msgs, "Regolazioni dei giochi tolte.")
	}
	if len(snap) == 0 && len(orig) == 0 {
		if len(msgs) > 0 {
			return msgs
		}
		return []string{"Tasso non ha ancora cambiato impostazioni di Windows: non c'è niente da rimettere."}
	}
	snapOK := true
	if len(snap) > 0 {
		snapOK = runRestoreScript(snap, func(m string) { msgs = append(msgs, m) })
		if !snapOK {
			msgs = append(msgs, "Non sono riuscito a rimettere tutte le impostazioni di Windows: le tengo salvate, così puoi riprovare.")
		}
	}
	n := 0
	for id, enabled := range orig {
		parts := strings.SplitN(id, "|", 2)
		if len(parts) != 2 {
			continue
		}
		runPSArgs(startupSetScript, map[string]interface{}{"scope": parts[0], "name": parts[1], "enable": enabled}, time.Minute, nil)
		n++
	}
	if n > 0 {
		msgs = append(msgs, "Programmi all'avvio rimessi com'erano.")
	}
	updateSettings(func(s *Settings) {
		if snapOK {
			s.Snapshot = nil
			s.SnapshotAt = ""
			s.Mode = "originale"
		}
		s.StartupOrig = nil
	})
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
		msgs := restoreOriginal()
		if pause {
			setPaused(true)
		}
		emit("onRestoreDone", map[string]interface{}{"msgs": msgs, "paused": pause, "settings": getSettings()})
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
