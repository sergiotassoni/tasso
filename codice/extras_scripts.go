package main

// Script PowerShell delle funzioni nuove della 2.1. Restituiscono una riga "JSON ...".

const healthScript = `
$items = New-Object System.Collections.ArrayList
$good = New-Object System.Collections.ArrayList
$score = 100
function Bad([int]$pts, [string]$title, [string]$tip, [string]$action) { $script:score -= $pts; [void]$items.Add([pscustomobject]@{ pts = $pts; title = $title; tip = $tip; action = $action }) }
function Good([string]$title) { [void]$good.Add($title) }

# Spazio libero sul disco di sistema
try {
  $d = Get-CimInstance Win32_LogicalDisk -Filter ("DeviceID='" + $env:SystemDrive + "'")
  $pct = [math]::Round(100 * $d.FreeSpace / $d.Size)
  if ($pct -lt 10) { Bad 25 ('Disco quasi pieno: libero solo il ' + $pct + '%') 'Fai la pulizia Media e guarda la Mappa del disco per trovare cosa occupa spazio.' 'diskmap' }
  elseif ($pct -lt 20) { Bad 10 ('Spazio libero basso: ' + $pct + '%') 'Una pulizia Normale o Media libera spazio subito.' 'clean' }
  else { Good ('Spazio libero: ' + $pct + '%') }
} catch {}

# Programmi all'avvio
try {
  $n = @(Get-TassoStartup | Where-Object { $_.enabled }).Count
  if ($n -gt 10) { Bad 15 ($n.ToString() + ' programmi partono all''accensione') 'Spegni quelli che non usi: il PC si accende molto prima.' 'startup' }
  elseif ($n -gt 6) { Bad 7 ($n.ToString() + ' programmi partono all''accensione') 'Togline qualcuno per un avvio più rapido.' 'startup' }
  else { Good ('Programmi all''avvio: ' + $n) }
} catch {}

# File temporanei
$t = [long](Get-FolderSize $env:TEMP) + [long](Get-FolderSize (Join-Path $env:windir 'Temp'))
if ($t -gt 2GB) { Bad 10 ('File temporanei: ' + (Fmt $t)) 'La pulizia Normale li toglie in un paio di minuti.' 'clean' }
elseif ($t -gt 500MB) { Bad 5 ('File temporanei: ' + (Fmt $t)) 'Una pulizia Normale li toglie.' 'clean' }
else { Good 'File temporanei: pochi' }

# Cestino
try { $s = ((New-Object -ComObject Shell.Application).Namespace(0xA).Items() | Measure-Object -Property Size -Sum).Sum; if ($s -gt 1GB) { Bad 5 ('Cestino pieno: ' + (Fmt $s)) 'Svuotalo se non ti serve più niente lì dentro.' 'recycle' } } catch {}

# Antivirus
try {
  $mp = Get-MpComputerStatus -ErrorAction Stop
  $other = @()
  try { $other = @(Get-CimInstance -Namespace root/SecurityCenter2 -ClassName AntiVirusProduct -ErrorAction Stop | Where-Object { $_.displayName -notmatch 'Defender' }) } catch {}
  if ($other.Count -gt 0 -and ([string]$mp.AMRunningMode -ne 'Normal')) { Good ('Antivirus attivo: ' + $other[0].displayName) }
  elseif (-not $mp.RealTimeProtectionEnabled) { Bad 20 'Antivirus in tempo reale spento' 'Riaccendilo da Sicurezza di Windows, oppure controlla che il tuo antivirus funzioni.' 'defender' }
  elseif ($mp.AntivirusSignatureAge -gt 7) { Bad 8 ('Antivirus non aggiornato da ' + $mp.AntivirusSignatureAge + ' giorni') 'Fai gli aggiornamenti di Windows.' 'winupdate' }
  else { Good 'Antivirus attivo e aggiornato' }
} catch {}

# Aggiornamenti di Windows
try {
  $last = (New-Object -ComObject Microsoft.Update.AutoUpdate).Results.LastInstallationSuccessDate
  if ($last) {
    $days = [int]((Get-Date) - [datetime]$last).TotalDays
    if ($days -gt 45) { Bad 10 ('Windows non si aggiorna da ' + $days + ' giorni') 'Gli aggiornamenti chiudono falle di sicurezza: falli dagli Strumenti.' 'winupdate' }
    else { Good 'Windows aggiornato' }
  }
} catch {}

# Riavvio in sospeso
if ((Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending') -or (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired')) {
  Bad 5 'Un riavvio è in sospeso' 'Riavvia il PC per finire gli aggiornamenti.' 'reboot'
} else { Good 'Nessun riavvio in sospeso' }

# Acceso da troppi giorni
try {
  $up = (Get-Date) - (Get-CimInstance Win32_OperatingSystem).LastBootUpTime
  if ($up.TotalDays -gt 7) { Bad 5 ('Acceso senza riavvio da ' + [int]$up.TotalDays + ' giorni') 'Un riavvio ogni tanto libera la memoria e applica gli aggiornamenti.' 'reboot' }
  else { Good 'Riavviato di recente' }
} catch {}

# Firewall
try {
  $off = @(Get-NetFirewallProfile -ErrorAction Stop | Where-Object { -not $_.Enabled })
  if ($off.Count -gt 0) { Bad 10 ('Firewall spento (' + (($off | ForEach-Object { $_.Name }) -join ', ') + ')') 'Riaccendilo da Sicurezza di Windows > Firewall e protezione rete.' 'security' }
  else { Good 'Firewall acceso' }
} catch {}

# Periferiche
try {
  $bd = @(Get-CimInstance Win32_PnPEntity -ErrorAction Stop | Where-Object { $_.ConfigManagerErrorCode -ne 0 -and $_.ConfigManagerErrorCode -ne 22 -and $_.ConfigManagerErrorCode -ne 45 })
  if ($bd.Count -gt 0) { Bad 6 ($bd.Count.ToString() + ' periferiche con problemi') ('Ad esempio: ' + $bd[0].Name + '. Il controllo periferiche ti dice quali.') 'devices' }
  else { Good 'Periferiche funzionanti' }
} catch {}

# Memoria
try {
  $os = Get-CimInstance Win32_OperatingSystem
  $used = 100 - [math]::Round(100 * $os.FreePhysicalMemory / $os.TotalVisibleMemorySize)
  if ($used -gt 85) { Bad 5 ('Memoria quasi piena: ' + $used + '%') 'Chiudi i programmi che non usi o attiva il Turbo.' 'turbo' }
  else { Good ('Memoria usata: ' + $used + '%') }
} catch {}

# Salute dei dischi
try {
  $bad = @(Get-PhysicalDisk -ErrorAction Stop | Where-Object { [string]$_.HealthStatus -in @('Warning','Unhealthy') })
  if ($bad.Count -gt 0) { Bad 25 'Un disco ha problemi di salute' 'Fai subito un backup di foto e documenti.' 'disks' }
  else { Good 'Dischi in salute' }
} catch {}

if ($score -lt 0) { $score = 0 }
$sorted = @($items | Sort-Object -Property pts -Descending)
$o = [pscustomobject]@{ score = $score; issues = $sorted; good = @($good) }
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject $o -Compress -Depth 4))
`

const netUsageScript = `
$up = (Get-Date) - (Get-CimInstance Win32_OperatingSystem).LastBootUpTime
$list = New-Object System.Collections.ArrayList
foreach ($a in @(Get-NetAdapter -ErrorAction SilentlyContinue | Where-Object { $_.Status -eq 'Up' })) {
  $st = Get-NetAdapterStatistics -Name $a.Name -ErrorAction SilentlyContinue
  if (-not $st) { continue }
  # Niente schede virtuali (VirtualBox, VMware, Hyper-V, VPN) senza traffico.
  if ($st.ReceivedBytes -eq 0 -and $st.SentBytes -eq 0) { continue }
  if ($a.InterfaceDescription -match 'VirtualBox|VMware|Hyper-V|Virtual Ethernet' -and $st.ReceivedBytes -lt 1MB) { continue }
  $kind = 'Cavo'
  if ($a.PhysicalMediaType -match '802\.11|Wireless' -or $a.InterfaceDescription -match 'Wi-?Fi|Wireless') { $kind = 'Wi-Fi' }
  elseif (($a.Name + ' ' + $a.InterfaceDescription) -match 'VPN|WireGuard|Wintun|TAP-|OpenVPN|Tunnel|TUN\b') { $kind = 'VPN' }
  [void]$list.Add([pscustomobject]@{ name = $a.Name; kind = $kind; speed = [string]$a.LinkSpeed; down = [long]$st.ReceivedBytes; up = [long]$st.SentBytes })
}
$ssid = ''; $band = ''; $signal = $null; $locOff = $false
try {
  $iw = netsh wlan show interfaces
  # Windows 11 24H2: senza "Posizione" accesa netsh non dà più i dati del Wi-Fi.
  if (($iw -join ' ') -match '(?i)location|posizione') { $locOff = $true }
  $l = $iw | Select-String -Pattern '^\s*SSID\s*:\s*(.+)$' | Select-Object -First 1; if ($l) { $ssid = $l.Matches[0].Groups[1].Value.Trim() }
  $b = $iw | Select-String -Pattern '^\s*(Band|Banda)\s*:\s*(.+)$' | Select-Object -First 1; if ($b) { $band = $b.Matches[0].Groups[2].Value.Trim() }
  if (-not $band) { $r = $iw | Select-String -Pattern '^\s*(Radio type|Tipo radio)\s*:\s*(.+)$' | Select-Object -First 1; if ($r) { $band = $r.Matches[0].Groups[2].Value.Trim() } }
  if (-not $band -or $band -match '802') { $c = $iw | Select-String -Pattern '^\s*(Channel|Canale|Kanal|Canal)\s*:\s*(\d+)' | Select-Object -First 1; if ($c) { $band = $(if ([int]$c.Matches[0].Groups[2].Value -le 14) { '2.4 GHz' } else { '5 GHz' }) } }
  $g = $iw | Select-String -Pattern '^\s*(Signal|Segnale)\s*:\s*(\d+)\s*%' | Select-Object -First 1; if ($g) { $signal = [int]$g.Matches[0].Groups[2].Value }
} catch {}
if ($ssid) { $locOff = $false }
# Reti a 5 GHz visibili adesso (solo i nomi, per consigliare quella più veloce).
$five = @()
try {
  $cur = ''
  foreach ($ln in @(netsh wlan show networks mode=bssid 2>$null)) {
    if ($ln -match '^SSID\s+\d+\s*:\s*(.*)$') { $cur = $Matches[1].Trim() }
    elseif ($ln -match '^\s*(Band|Banda)\s*:\s*5') { if ($cur -and $five -notcontains $cur) { $five += $cur } }
    elseif ($ln -match '^\s*(Channel|Canale)\s*:\s*(\d+)') { if ([int]$Matches[2] -ge 32 -and $cur -and $five -notcontains $cur) { $five += $cur } }
  }
} catch {}
# Una presa di rete libera: con il cavo si va quasi sempre più veloce e stabili.
$cable = [bool](@(Get-NetAdapter -Physical -ErrorAction SilentlyContinue | Where-Object { $_.Status -ne 'Up' -and $_.Status -ne 'Disabled' -and $_.PhysicalMediaType -match '802\.3' }).Count)
$o = [pscustomobject]@{ hours = [math]::Round($up.TotalHours, 1); adapters = @($list); ssid = $ssid; band = $band; signal = $signal; five = @($five); cable = $cable; locOff = $locOff }
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject $o -Compress -Depth 4))
`

// Le reti Wi-Fi salvate con la loro password: si esportano in una cartella temporanea
// (formato XML, uguale in tutte le lingue di Windows), si leggono e si cancellano subito.
const wifiScript = `
# Se una volta precedente è stata interrotta, tolgo i file rimasti.
Get-ChildItem -LiteralPath $env:TEMP -Directory -Filter 'tasso_wifi_*' -Force -ErrorAction SilentlyContinue | ForEach-Object { Remove-Item -LiteralPath $_.FullName -Recurse -Force -ErrorAction SilentlyContinue }
$dir = Join-Path $env:TEMP ('tasso_wifi_' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $dir -Force | Out-Null
$list = New-Object System.Collections.ArrayList
try {
  & netsh.exe wlan export profile key=clear ('folder=' + $dir) | Out-Null
  foreach ($f in @(Get-ChildItem -LiteralPath $dir -Filter '*.xml' -ErrorAction SilentlyContinue)) {
    try {
      [xml]$x = Get-Content -Raw -Encoding UTF8 -LiteralPath $f.FullName
      $name = [string]$x.WLANProfile.SSIDConfig.SSID.name
      if (-not $name) { $name = [string]$x.WLANProfile.name }
      $key = [string]$x.WLANProfile.MSM.security.sharedKey.keyMaterial
      $auth = [string]$x.WLANProfile.MSM.security.authEncryption.authentication
      [void]$list.Add([pscustomobject]@{ name = $name; password = $key; open = ($auth -eq 'open') })
    } catch {}
  }
} finally { Remove-Item -LiteralPath $dir -Recurse -Force -ErrorAction SilentlyContinue }
$sorted = @($list | Sort-Object -Property name)
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject $sorted -Compress -Depth 3))
`

// Mappa del disco: dimensione delle cartelle principali con robocopy in modalità "solo elenco"
// (veloce, non copia niente, salta le cartelle protette).
const diskMapScript = `
function Measure-Dir([string]$p) {
  try {
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = 'robocopy.exe'
    $psi.Arguments = '"' + $p.TrimEnd('\') + '" "' + (Join-Path $env:TEMP 'tasso_nul') + '" /L /E /XJ /NFL /NDL /NJH /BYTES /R:0 /W:0 /NP'
    $psi.UseShellExecute = $false; $psi.CreateNoWindow = $true; $psi.RedirectStandardOutput = $true; $psi.RedirectStandardError = $true
    $pr = [System.Diagnostics.Process]::Start($psi)
    $err = $pr.StandardError.ReadToEndAsync()
    $out = $pr.StandardOutput.ReadToEnd(); $pr.WaitForExit()
    foreach ($line in ($out -split "\r?\n")) {
      $parts = @($line.Trim() -split '\s+')
      if ($parts.Count -ge 7 -and $line -match ':\s+\d+\s+\d+') {
        if ($line -match '^\s*(Bytes|Byte)\s*:') { return [long]$parts[2] }
      }
    }
    # Riga dei byte in altre lingue: è la terza riga di riepilogo con numeri.
    $nums = @($out -split "\r?\n" | Where-Object { $_ -match ':\s+\d+\s+\d+\s+\d+' })
    if ($nums.Count -ge 3) { return [long](@($nums[2].Split(':')[1].Trim() -split '\s+')[0]) }
  } catch {}
  return [long]0
}
$roots = New-Object System.Collections.ArrayList
$sys = $env:SystemDrive + '\'
$skip = @('$Recycle.Bin', 'System Volume Information', 'Recovery', 'Config.Msi', 'Documents and Settings')
$targets = New-Object System.Collections.ArrayList
foreach ($d in @(Get-ChildItem -LiteralPath $sys -Directory -Force -ErrorAction SilentlyContinue)) {
  if ($skip -contains $d.Name) { continue }
  if ($d.FullName -ieq (Split-Path $env:USERPROFILE -Parent)) { continue }
  [void]$targets.Add(@{ path = $d.FullName; group = 'Disco ' + $env:SystemDrive })
}
foreach ($d in @(Get-ChildItem -LiteralPath $env:USERPROFILE -Directory -Force -ErrorAction SilentlyContinue)) {
  if ($d.Attributes -band [IO.FileAttributes]::ReparsePoint) { continue }
  [void]$targets.Add(@{ path = $d.FullName; group = 'Cartella utente' })
}
$i = 0
foreach ($t in $targets) {
  $i++
  Say ('Misuro ' + $i + ' di ' + $targets.Count + ': ' + (Split-Path $t.path -Leaf))
  $size = Measure-Dir $t.path
  if ($size -gt 50MB) { [void]$roots.Add([pscustomobject]@{ path = $t.path; name = (Split-Path $t.path -Leaf); group = $t.group; size = $size }) }
}
$d = Get-CimInstance Win32_LogicalDisk -Filter ("DeviceID='" + $env:SystemDrive + "'")
$sorted = @($roots | Sort-Object -Property size -Descending)
$o = [pscustomobject]@{ total = [long]$d.Size; free = [long]$d.FreeSpace; items = $sorted }
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject $o -Compress -Depth 4))
`
