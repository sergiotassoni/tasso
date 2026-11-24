package main

// Modalità Ufficio, novità 3.1 (approvate da Sergio il 22/11): Pronto per la videochiamata?,
// Modalità riunione, Chi ha usato webcam e microfono, Blocca il PC quando ti alzi.
// Stanno in Pulisci e ripara → Risolvi un problema, in cima, nel gruppo "Novità".
// Regole di sempre: niente documenti toccati, ogni impostazione cambiata si salva prima e si
// rimette com'era premendo di nuovo lo stesso pulsante.

import (
	"os"
	"path/filepath"
	"time"
)

// ofHelpers3: stato delle novità (cartella in AppData dell'utente) e "chi usa adesso".
const ofHelpers3 = `
$OfState = Join-Path $env:LOCALAPPDATA 'Tasso\ufficio'
New-Item -ItemType Directory -Path $OfState -Force | Out-Null
function Get-CapUse([string]$what) {
  # Programmi che hanno usato webcam o microfono (dato di Windows: Impostazioni > Privacy).
  $out = @()
  $b = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\' + $what
  $keys = @(Get-ChildItem -LiteralPath $b -ErrorAction SilentlyContinue | Where-Object { $_.PSChildName -ne 'NonPackaged' }) + @(Get-ChildItem -LiteralPath ($b + '\NonPackaged') -ErrorAction SilentlyContinue)
  foreach ($k in $keys) {
    $p = Get-ItemProperty -LiteralPath $k.PSPath -ErrorAction SilentlyContinue
    if (-not $p -or -not $p.LastUsedTimeStart) { continue }
    $start = [long]$p.LastUsedTimeStart; if ($start -le 0) { continue }
    $nm = [string]$k.PSChildName; $full = ''
    if ($nm -match '#') { $full = $nm -replace '#', '\'; $nm = Split-Path $full -Leaf } else { $nm = ($nm -split '_')[0] }
    $pretty = $nm
    if ($full -and (Test-Path -LiteralPath $full)) { try { $d = (Get-Item -LiteralPath $full).VersionInfo.FileDescription; if ($d) { $pretty = $d.Trim() } } catch {} }
    elseif ($nm -match '\.') { $pretty = ($nm -split '\.')[-1] }
    $now = ([long]$p.LastUsedTimeStop -eq 0)
    try { $when = [DateTime]::FromFileTime($start) } catch { continue }
    $out += [pscustomobject]@{ Name = $pretty; When = $when; Now = $now }
  }
  return $out
}
function Fmt-When([DateTime]$t) {
  $d = (Get-Date).Date
  if ($t.Date -eq $d) { return 'oggi alle ' + $t.ToString('HH:mm') }
  if ($t.Date -eq $d.AddDays(-1)) { return 'ieri alle ' + $t.ToString('HH:mm') }
  return $t.ToString('dd/MM') + ' alle ' + $t.ToString('HH:mm')
}
`

func ofs3(s string) string { return ofHelpers + ofHelpers3 + s }

// Fine della Modalità riunione: lo stesso file lo usano il pulsante (spegni) e la guardia che la
// spegne da sola a fine chiamata. Scritto su disco dalla prima accensione.
const ofRiunioneFine = `
$ErrorActionPreference = 'SilentlyContinue'
$OfState = Join-Path $env:LOCALAPPDATA 'Tasso\ufficio'
$f = Join-Path $OfState 'riunione.json'
if (-not (Test-Path -LiteralPath $f)) { return }
$s = Get-Content -LiteralPath $f -Raw | ConvertFrom-Json
Remove-Item -LiteralPath $f -Force
$n = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Notifications\Settings'
$t = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\PushNotifications'
if ($null -eq $s.noc -or $s.noc -eq '') { Remove-ItemProperty -LiteralPath $n -Name 'NOC_GLOBAL_SETTING_TOASTS_ENABLED' } else { Set-ItemProperty -LiteralPath $n -Name 'NOC_GLOBAL_SETTING_TOASTS_ENABLED' -Value ([int]$s.noc) -Type DWord }
if ($null -eq $s.toast -or $s.toast -eq '') { Remove-ItemProperty -LiteralPath $t -Name 'ToastEnabled' } else { Set-ItemProperty -LiteralPath $t -Name 'ToastEnabled' -Value ([int]$s.toast) -Type DWord }
if ($s.plan -and $s.plan -match '^[0-9a-fA-F-]{36}$') { & powercfg.exe /setactive $s.plan | Out-Null }
if ($s.wu) {
  $u = 'HKLM:\SOFTWARE\Microsoft\WindowsUpdate\UX\Settings'
  foreach ($v in 'PauseUpdatesStartTime','PauseUpdatesExpiryTime','PauseFeatureUpdatesStartTime','PauseFeatureUpdatesEndTime','PauseQualityUpdatesStartTime','PauseQualityUpdatesEndTime') { Remove-ItemProperty -LiteralPath $u -Name $v }
}
if ($s.od -and (Test-Path -LiteralPath $s.od)) { Start-Process -FilePath (Join-Path $env:SystemRoot 'explorer.exe') -ArgumentList ('"' + $s.od + '"') }
Add-Content -LiteralPath (Join-Path $OfState 'riunione.log') -Value ((Get-Date).ToString('yyyy-MM-dd HH:mm') + ' spenta (' + $args[0] + ')')
`

// Guardia: si accorge da sola della fine della chiamata (il microfono non è più usato da nessun
// programma per 3 minuti) e rimette tutto com'era. Al massimo dopo 4 ore si spegne comunque.
const ofRiunioneGuardia = `
$ErrorActionPreference = 'SilentlyContinue'
$OfState = Join-Path $env:LOCALAPPDATA 'Tasso\ufficio'
$f = Join-Path $OfState 'riunione.json'
$fine = Join-Path $OfState 'riunione-fine.ps1'
function MicInUse {
  $b = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\microphone'
  $keys = @(Get-ChildItem -LiteralPath $b -ErrorAction SilentlyContinue | Where-Object { $_.PSChildName -ne 'NonPackaged' }) + @(Get-ChildItem -LiteralPath ($b + '\NonPackaged') -ErrorAction SilentlyContinue)
  foreach ($k in $keys) { $p = Get-ItemProperty -LiteralPath $k.PSPath; if ($p -and [long]$p.LastUsedTimeStart -gt 0 -and [long]$p.LastUsedTimeStop -eq 0) { return $true } }
  return $false
}
$start = Get-Date; $seen = $false; $quiet = 0
while ($true) {
  Start-Sleep -Seconds 30
  if (-not (Test-Path -LiteralPath $f)) { return }
  if (MicInUse) { $seen = $true; $quiet = 0 } elseif ($seen) { $quiet++ }
  if ($quiet -ge 6) { & $fine 'fine chiamata'; return }
  if (((Get-Date) - $start).TotalHours -ge 4) { & $fine '4 ore'; return }
}
`

var officeSteps3 = []Step{
	{ID: "of_new_call", Name: "Pronto per la videochiamata?", Desc: "Prova in pochi secondi webcam, microfono, casse o cuffie e internet. Ti dice cosa va e cosa no e, se serve, ti propone la riparazione con un pulsante. Controlla e spiega.", Level: 0, Minutes: 0.2, Timeout: 90 * time.Second, Script: ofs3(`
Say 'Provo webcam, microfono, casse e internet...'
$fix = @(); $bad = 0
# Internet: Teams/Zoom/Meet usano la porta 443; il ping misura quanto è pronta la linea.
$tcp = $false
try { $c = New-Object System.Net.Sockets.TcpClient; $a = $c.BeginConnect('teams.microsoft.com', 443, $null, $null); if ($a.AsyncWaitHandle.WaitOne(4000) -and $c.Connected) { $tcp = $true }; $c.Close() } catch {}
$ms = @(); $lost = 0
$pg = New-Object System.Net.NetworkInformation.Ping
foreach ($i in 1..5) { try { $r = $pg.Send('1.1.1.1', 1500); if ($r.Status -eq 'Success') { $ms += [int]$r.RoundtripTime } else { $lost++ } } catch { $lost++ } }
if (-not $tcp -and -not $ms.Count) { Say 'NO  Internet: non raggiungo internet. Senza rete la chiamata non parte.'; $fix += 'of_wifi_conn'; $bad++ }
elseif ($ms.Count) {
  $avg = [int](($ms | Measure-Object -Average).Average)
  if ($avg -gt 150 -or $lost -ge 2) { Say ('ATTENZIONE  Internet: lento o a singhiozzo (' + $avg + ' ms, ' + $lost + ' risposte perse su 5). La chiamata può bloccarsi: avvicinati al Wi-Fi o usa il cavo.'); $bad++ }
  else { Say ('OK  Internet: va bene (' + $avg + ' ms).') }
} else { Say 'OK  Internet: raggiungo Teams (il ping è bloccato da questa rete, ma la chiamata funziona).' }
# Webcam
$cams = @(Get-PnpDevice -PresentOnly -ErrorAction SilentlyContinue | Where-Object { [string]$_.Class -match '^(Camera|Image)$' -and $_.FriendlyName -notmatch 'Scanner|IR' })
$pvc = Get-Privacy 'webcam'
if (-not $cams.Count) { Say 'NO  Webcam: Windows non la vede. Sul portatile cerca la tendina sopra lo schermo o il tasto con la telecamera barrata.'; $fix += 'of_meet_cam'; $bad++ }
elseif (@($cams | Where-Object { $_.Status -ne 'OK' }).Count -eq $cams.Count) { Say ('NO  Webcam: c''è (' + $cams[0].FriendlyName + ') ma è spenta o ha un problema.'); $fix += 'of_meet_cam'; $bad++ }
elseif ($pvc.pc -eq 'Deny' -or $pvc.all -eq 'Deny' -or $pvc.desk -eq 'Deny') { Say 'NO  Webcam: Windows la blocca ai programmi (privacy).'; $fix += 'of_meet_cam'; $bad++ }
else {
  $busy = @(Get-CapUse 'webcam' | Where-Object { $_.Now } | ForEach-Object { $_.Name } | Select-Object -Unique)
  if ($busy.Count) { Say ('ATTENZIONE  Webcam: la sta già usando ' + ($busy -join ', ') + '. Se non è il programma della chiamata chiudilo: una sola app alla volta può usarla.') }
  else { Say ('OK  Webcam: pronta (' + $cams[0].FriendlyName + ').') }
}
# Microfono e casse (quelli scelti per le chiamate)
if (Use-Audio) {
  $mic = [TassoAudio]::State(1, 2)
  if (-not $mic) { Say 'NO  Microfono: Windows non vede nessun microfono. Collega le cuffie.'; $fix += 'of_meet_mic'; $bad++ }
  else { $pa = $mic.Split('|'); if ($pa[0] -eq '1') { Say 'NO  Microfono: è IN MUTO in Windows.'; $fix += 'of_meet_mic'; $bad++ } elseif ([int]$pa[1] -lt 30) { Say ('NO  Microfono: volume troppo basso (' + $pa[1] + '%).'); $fix += 'of_meet_mic'; $bad++ } else { Say ('OK  Microfono: acceso, volume ' + $pa[1] + '%.') } }
  $spk = [TassoAudio]::State(0, 2)
  if (-not $spk) { Say 'NO  Casse/cuffie: Windows non vede niente per sentire.'; $fix += 'of_meet_speaker'; $bad++ }
  else { $pa = $spk.Split('|'); if ($pa[0] -eq '1') { Say 'NO  Casse/cuffie: sono IN MUTO.'; $fix += 'of_meet_speaker'; $bad++ } elseif ([int]$pa[1] -lt 15) { Say ('NO  Casse/cuffie: volume troppo basso (' + $pa[1] + '%).'); $fix += 'of_meet_speaker'; $bad++ } else { Say ('OK  Casse/cuffie: accese, volume ' + $pa[1] + '%.') } }
} else { Say 'ATTENZIONE  Audio: non riesco a leggere microfono e casse su questo PC.' }
$pvm = Get-Privacy 'microphone'
if ($pvm.pc -eq 'Deny' -or $pvm.all -eq 'Deny' -or $pvm.desk -eq 'Deny') { Say 'NO  Microfono: Windows lo blocca ai programmi (privacy).'; if ($fix -notcontains 'of_meet_mic') { $fix += 'of_meet_mic' }; $bad++ }
if (-not $bad) { Say 'Tutto pronto per la videochiamata.' }
elseif ($fix.Count) { Say ('Da sistemare: ' + $bad + '. Premi "Ripara adesso" qui sotto. [ripara:' + (($fix | Select-Object -Unique) -join ',') + ']') }
else { Say 'Niente da riparare in automatico: segui il consiglio qui sopra.' }
`)},
	{ID: "of_new_meeting", Name: "Modalità riunione", Desc: "Premi prima di una chiamata: niente notifiche, OneDrive e aggiornamenti di Windows in pausa, PC al massimo. A fine chiamata si spegne da sola e rimette tutto com'era. Premi di nuovo per spegnerla subito.", Level: 0, Minutes: 0.4, Timeout: 2 * time.Minute, Script: ofs3(`
$f = Join-Path $OfState 'riunione.json'
$fine = Join-Path $OfState 'riunione-fine.ps1'
$guardia = Join-Path $OfState 'riunione-guardia.ps1'
Set-Content -LiteralPath $fine -Value @'
` + ofRiunioneFine + `
'@ -Encoding UTF8
Set-Content -LiteralPath $guardia -Value @'
` + ofRiunioneGuardia + `
'@ -Encoding UTF8
if (Test-Path -LiteralPath $f) {
  & $fine 'pulsante'
  Say 'Modalità riunione SPENTA: notifiche, OneDrive, aggiornamenti e alimentazione sono tornati come prima.'
  return
}
$n = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Notifications\Settings'
$t = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\PushNotifications'
$s = @{ noc = ''; toast = ''; plan = ''; wu = $false; od = ''; at = (Get-Date).ToString('o') }
try { $s.noc = [string](Get-ItemProperty -LiteralPath $n -Name 'NOC_GLOBAL_SETTING_TOASTS_ENABLED' -ErrorAction Stop).NOC_GLOBAL_SETTING_TOASTS_ENABLED } catch {}
try { $s.toast = [string](Get-ItemProperty -LiteralPath $t -Name 'ToastEnabled' -ErrorAction Stop).ToastEnabled } catch {}
$m = [regex]::Match((& powercfg.exe /getactivescheme | Out-String), '[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}')
if ($m.Success) { $s.plan = $m.Value }
# Prima salvo com'era, poi cambio: se Tasso o il PC si spengono, premendo di nuovo torna tutto.
$s | ConvertTo-Json | Set-Content -LiteralPath $f -Encoding UTF8
New-Item -Path $n -Force -ErrorAction SilentlyContinue | Out-Null
Set-ItemProperty -LiteralPath $n -Name 'NOC_GLOBAL_SETTING_TOASTS_ENABLED' -Value 0 -Type DWord
Set-ItemProperty -LiteralPath $t -Name 'ToastEnabled' -Value 0 -Type DWord
Say 'Notifiche di Windows: spente (niente fumetti che coprono lo schermo condiviso).'
$hp = '8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c'
if ($s.plan -and $s.plan -ne $hp) {
  & powercfg.exe /setactive $hp 2>$null | Out-Null
  $now = [regex]::Match((& powercfg.exe /getactivescheme | Out-String), '[0-9a-fA-F-]{36}').Value
  if ($now -eq $hp) { Say 'Alimentazione: al massimo (prestazioni elevate).' } else { Say 'Alimentazione: questo PC non ha il piano "Prestazioni elevate", resta com''è.' }
} else { Say 'Alimentazione: già al massimo.' }
$u = 'HKLM:\SOFTWARE\Microsoft\WindowsUpdate\UX\Settings'
$already = $false; try { $already = [bool](Get-ItemProperty -LiteralPath $u -Name 'PauseUpdatesExpiryTime' -ErrorAction Stop).PauseUpdatesExpiryTime } catch {}
if ($already) { Say 'Aggiornamenti di Windows: erano già in pausa, li lascio come sono.' }
else {
  $a = (Get-Date).ToUniversalTime(); $b = $a.AddDays(1)
  $sa = $a.ToString('yyyy-MM-ddTHH:mm:ssZ'); $sb = $b.ToString('yyyy-MM-ddTHH:mm:ssZ')
  New-Item -Path $u -Force -ErrorAction SilentlyContinue | Out-Null
  foreach ($kv in @(@('PauseUpdatesStartTime', $sa), @('PauseUpdatesExpiryTime', $sb), @('PauseFeatureUpdatesStartTime', $sa), @('PauseFeatureUpdatesEndTime', $sb), @('PauseQualityUpdatesStartTime', $sa), @('PauseQualityUpdatesEndTime', $sb))) { Set-ItemProperty -LiteralPath $u -Name $kv[0] -Value $kv[1] -Type String }
  $s.wu = $true
  Say 'Aggiornamenti di Windows: in pausa (al massimo 1 giorno, poi ripartono da soli).'
}
$od = Get-Process -Name OneDrive -ErrorAction SilentlyContinue | Select-Object -First 1
if ($od -and $od.Path) {
  $s.od = $od.Path
  $s | ConvertTo-Json | Set-Content -LiteralPath $f -Encoding UTF8
  # OneDrive non parte da amministratore: il comando di chiusura lo lancio come utente normale.
  [void](Start-AsUser $od.Path '/shutdown')
  if (Get-Process -Name OneDrive -ErrorAction SilentlyContinue) { Stop-Process -Name OneDrive -ErrorAction SilentlyContinue }
  Say 'OneDrive: in pausa (riparte da solo a fine riunione; i file restano tutti).'
} else { Say 'OneDrive: non era acceso.' }
$s | ConvertTo-Json | Set-Content -LiteralPath $f -Encoding UTF8
Get-CimInstance Win32_Process -Filter "Name='powershell.exe'" -ErrorAction SilentlyContinue | Where-Object { [string]$_.CommandLine -like '*riunione-guardia.ps1*' } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
Start-Process -FilePath (Join-Path $PSHOME 'powershell.exe') -WindowStyle Hidden -ArgumentList ('-NoProfile -ExecutionPolicy Bypass -File "' + $guardia + '"')
Say 'Modalità riunione ACCESA. Si spegne da sola 3 minuti dopo la fine della chiamata (quando nessun programma usa più il microfono), al massimo dopo 4 ore. Per spegnerla prima premi di nuovo il pulsante.'
`)},
	{ID: "of_new_camlog", Name: "Chi ha usato webcam e microfono", Desc: "Ti fa vedere quali programmi hanno usato la webcam e il microfono negli ultimi 30 giorni e quando, e chi li sta usando adesso. Così scopri se qualcuno ti guarda o ascolta. Controlla e spiega.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs3(`
$lim = (Get-Date).AddDays(-30); $tot = 0
foreach ($w in @(@('webcam', 'Webcam'), @('microphone', 'Microfono'))) {
  $l = @(Get-CapUse $w[0] | Where-Object { $_.When -ge $lim } | Sort-Object When -Descending)
  if (-not $l.Count) { Say ($w[1] + ': nessun programma negli ultimi 30 giorni.'); continue }
  Say ($w[1] + ' (' + $l.Count + ' programmi):')
  foreach ($x in ($l | Select-Object -First 15)) { $tot++; Say ('  - ' + $x.Name + ': ' + (Fmt-When $x.When) + ($(if ($x.Now) { '  <- LA STA USANDO ADESSO' } else { '' }))) }
}
if ($tot) { Say 'Se vedi un programma che non conosci, puoi togliergli il permesso in Impostazioni > Privacy e sicurezza > Fotocamera (o Microfono), oppure premi "Controllo dei programmi sospetti" qui sotto.' }
`)},
	{ID: "of_new_lock", Name: "Blocca il PC quando ti alzi", Desc: "Accende il blocco automatico: dopo 5 minuti senza toccare mouse e tastiera lo schermo si blocca e per rientrare serve il PIN o la password. Premi di nuovo per rimettere com'era.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs3(`
if (-not ('TassoSpi' -as [type])) { Add-Type -TypeDefinition 'using System; using System.Runtime.InteropServices; public static class TassoSpi { [DllImport("user32.dll")] public static extern bool SystemParametersInfo(uint a, uint b, IntPtr c, uint d); }' }
$k = 'HKCU:\Control Panel\Desktop'
$f = Join-Path $OfState 'blocco.json'
function G([string]$nm) { try { return [string](Get-ItemProperty -LiteralPath $k -Name $nm -ErrorAction Stop).$nm } catch { return '' } }
function Apply([int]$to, [int]$act, [int]$sec) { [void][TassoSpi]::SystemParametersInfo(15, [uint32]$to, [IntPtr]::Zero, 3); [void][TassoSpi]::SystemParametersInfo(17, [uint32]$act, [IntPtr]::Zero, 3); [void][TassoSpi]::SystemParametersInfo(119, [uint32]$sec, [IntPtr]::Zero, 3) }
$pol = 'HKCU:\Software\Policies\Microsoft\Windows\Control Panel\Desktop'
if (Test-Path -LiteralPath $f) {
  $p = Get-Content -LiteralPath $f -Raw | ConvertFrom-Json
  Apply ([int]('0' + $p.to)) ([int]('0' + $p.act)) ([int]('0' + $p.sec))
  foreach ($kv in @(@('ScreenSaveTimeOut', $p.to), @('ScreenSaveActive', $p.act), @('ScreenSaverIsSecure', $p.sec), @('SCRNSAVE.EXE', $p.exe))) { if ([string]$kv[1] -eq '') { Remove-ItemProperty -LiteralPath $k -Name $kv[0] -ErrorAction SilentlyContinue } else { Set-ItemProperty -LiteralPath $k -Name $kv[0] -Value ([string]$kv[1]) } }
  Remove-Item -LiteralPath $f -Force
  Say 'Blocco automatico SPENTO: rimesso come prima.'
  return
}
if (Test-Path -LiteralPath $pol) { Say 'Su questo PC il blocco dello schermo lo decide l''ufficio (criteri di gruppo): Tasso non lo cambia. Puoi sempre bloccare al volo con Windows + L.'; return }
@{ to = (G 'ScreenSaveTimeOut'); act = (G 'ScreenSaveActive'); sec = (G 'ScreenSaverIsSecure'); exe = (G 'SCRNSAVE.EXE') } | ConvertTo-Json | Set-Content -LiteralPath $f -Encoding UTF8
$exe = G 'SCRNSAVE.EXE'
if (-not $exe -or -not (Test-Path -LiteralPath $exe)) { Set-ItemProperty -LiteralPath $k -Name 'SCRNSAVE.EXE' -Value (Join-Path $env:SystemRoot 'System32\scrnsave.scr') }
Apply 300 1 1
Set-ItemProperty -LiteralPath $k -Name 'ScreenSaveTimeOut' -Value '300'; Set-ItemProperty -LiteralPath $k -Name 'ScreenSaveActive' -Value '1'; Set-ItemProperty -LiteralPath $k -Name 'ScreenSaverIsSecure' -Value '1'
Say 'Blocco automatico ACCESO: dopo 5 minuti senza toccare niente lo schermo diventa nero e per rientrare serve il PIN o la password. Nessun documento si chiude.'
Say 'Quando ti alzi puoi bloccare subito con Windows + L. Se il telefono è abbinato via Bluetooth, in Impostazioni > Account > Opzioni di accesso c''è anche "Blocco dinamico": il PC si blocca quando ti allontani col telefono.'
`)},
}

func init() {
	insertStepAfter("of_bitlocker", officeSteps3...)
}

// officeStati: dice alla finestra se Modalità riunione e Blocco automatico sono accesi
// (così il pulsante scrive Accendi o Spegni).
func officeStati() map[string]bool {
	d := filepath.Join(os.Getenv("LOCALAPPDATA"), "Tasso", "ufficio")
	ex := func(n string) bool { _, err := os.Stat(filepath.Join(d, n)); return err == nil }
	return map[string]bool{"of_new_meeting": ex("riunione.json"), "of_new_lock": ex("blocco.json")}
}
