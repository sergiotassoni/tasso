package main

// Gli script PowerShell usati da features.go (programmi all'avvio, programmi installati).

const startupListScript = `
$out = Get-TassoStartup
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject @($out) -Compress -Depth 3))
`

const startupSetScript = `
# 3.2: il preludio ha ErrorActionPreference SilentlyContinue, quindi una scrittura che non riesce
# (chiave in sola lettura, criterio di gruppo) non fermerebbe lo script. Qui ogni scrittura ha
# -ErrorAction Stop dentro try/catch, e "JSON ok" esce solo se rileggendo il valore è quello giusto.
# La rilettura usa Get-Item -LiteralPath e GetValue (nome esatto: "App [x64]" non è un carattere jolly).
$map = @{
  hkcu   = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run'
  hklm   = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run'
  hklm32 = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run32'
  fu     = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\StartupFolder'
  fc     = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\StartupFolder'
}
if ([string]$A.scope -eq 'appx') {
  # App dello Store: 2 = attivata, 1 = disattivata da te (come l'interruttore di Gestione attività).
  $parts = ([string]$A.name) -split '\\', 2
  if ($parts.Count -ne 2) { Say 'Voce non valida.'; return }
  $k = 'HKCU:\Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\SystemAppData\' + $parts[0] + '\' + $parts[1]
  $v = 1; if ($A.enable) { $v = 2 }
  try {
    if (-not (Test-Path -LiteralPath $k)) { New-Item -Path $k -Force -ErrorAction Stop | Out-Null }
    New-ItemProperty -LiteralPath $k -Name 'State' -Value $v -PropertyType DWord -Force -ErrorAction Stop | Out-Null
    $letto = (Get-Item -LiteralPath $k -ErrorAction Stop).GetValue('State')
  } catch { Say 'Windows non mi lascia cambiare questa voce.'; return }
  if ($null -eq $letto -or [int]$letto -ne $v) { Say 'Windows non mi lascia cambiare questa voce.'; return }
  [Console]::Out.WriteLine('JSON ok')
  Say 'Fatto.'
  return
}
$key = $map[[string]$A.scope]
if (-not $key) { Say 'Voce non valida.'; return }
$n = [string]$A.name
$b = New-Object byte[] 12
if ($A.enable) { $b[0] = 2 } else {
  $b[0] = 3
  $ft = [BitConverter]::GetBytes([DateTime]::Now.ToFileTime())
  [Array]::Copy($ft, 0, $b, 4, 8)
}
try {
  if (-not (Test-Path -LiteralPath $key)) { New-Item -Path $key -Force -ErrorAction Stop | Out-Null }
  New-ItemProperty -LiteralPath $key -Name $n -Value $b -PropertyType Binary -Force -ErrorAction Stop | Out-Null
  $letto = [byte[]]((Get-Item -LiteralPath $key -ErrorAction Stop).GetValue($n))
} catch { Say 'Windows non mi lascia cambiare questa voce.'; return }
if (-not $letto -or $letto.Length -lt 1 -or $letto[0] -ne $b[0]) { Say 'Windows non mi lascia cambiare questa voce.'; return }
[Console]::Out.WriteLine('JSON ok')
Say 'Fatto.'
`

// arpHelper (3.1): leggo le voci di "Programmi installati" come le legge Windows.
// Get-RegText: il testo si ferma al primo carattere nullo (come in Programmi e funzionalità) e perde i
// caratteri invisibili: una voce rovinata da un disinstallatore (nome fatto solo di caratteri nulli o
// invisibili) si vedeva in Tasso come una riga bianca, senza nome e senza icona, che non si poteva togliere.
const arpHelper = `
function Get-RegText($v) {
  if ($null -eq $v) { return '' }
  $s = [string]$v
  $i = $s.IndexOf([char]0); if ($i -ge 0) { $s = $s.Substring(0, $i) }
  $s = [regex]::Replace($s, '\p{Cf}', '')
  $s = [regex]::Replace($s, '[\p{Cc}\s]+', ' ')
  return $s.Trim()
}
# Nome mostrabile: almeno una lettera o una cifra, altrimenti Windows non la mostra e Tasso neanche.
function Test-ShownName([string]$n) { return [bool]($n -and $n -match '[\p{L}\p{N}]') }
# Nomi del tipo "@C:\...\file.dll,-101": il testo vero sta nel file, come fa Windows.
$script:TassoInd = 0
function Resolve-RegText([string]$s) {
  if (-not $s.StartsWith('@')) { return $s }
  if (-not $script:TassoInd) {
    $script:TassoInd = 2
    try { if (-not ('TassoArp.Ind' -as [type])) { Add-Type -Namespace TassoArp -Name Ind -MemberDefinition '[DllImport("shlwapi.dll", CharSet = CharSet.Unicode)] public static extern int SHLoadIndirectString(string src, System.Text.StringBuilder buf, int cch, IntPtr r);' -ErrorAction Stop }; $script:TassoInd = 1 } catch {}
  }
  if ($script:TassoInd -eq 1) {
    try { $sb = New-Object System.Text.StringBuilder 1024; if ([TassoArp.Ind]::SHLoadIndirectString($s, $sb, 1024, [IntPtr]::Zero) -eq 0) { $r = Get-RegText $sb.ToString(); if ($r) { return $r } } } catch {}
  }
  return $s
}
# Codice "compresso" del prodotto MSI usato da Windows Installer nel registro.
function Get-PackedGuid([string]$g) {
  $m = [regex]::Match($g, '^\{([0-9A-Fa-f]{8})-([0-9A-Fa-f]{4})-([0-9A-Fa-f]{4})-([0-9A-Fa-f]{4})-([0-9A-Fa-f]{12})\}$')
  if (-not $m.Success) { return '' }
  $rev = { param($s) -join ($s.ToCharArray()[($s.Length - 1)..0]) }
  $pair = { param($s) -join (0..($s.Length / 2 - 1) | ForEach-Object { '' + $s[2 * $_ + 1] + $s[2 * $_] }) }
  return ((& $rev $m.Groups[1].Value) + (& $rev $m.Groups[2].Value) + (& $rev $m.Groups[3].Value) + (& $pair $m.Groups[4].Value) + (& $pair $m.Groups[5].Value)).ToUpper()
}
`

const programsListScript = arpHelper + `
$keys = @('HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall', 'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall', 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall')
$seen = @{}
$out = New-Object System.Collections.ArrayList
foreach ($k in $keys) {
  Get-ChildItem -LiteralPath $k -ErrorAction SilentlyContinue | ForEach-Object {
    # La PowerShell usa "/" come separatore: una voce con "/" nel nome non si potrebbe toccare.
    if ($_.PSChildName -match '[/\x00-\x1f]') { return }
    $p = Get-ItemProperty -LiteralPath $_.PSPath -ErrorAction SilentlyContinue
    if (-not $p) { return }
    # Nascoste come in Programmi e funzionalità: senza nome, componenti di sistema, aggiornamenti.
    $name = Resolve-RegText (Get-RegText $p.DisplayName)
    if (-not (Test-ShownName $name) -or [string]$p.SystemComponent -eq '1' -or (Get-RegText $p.ParentKeyName) -or ((Get-RegText $p.ReleaseType) -match 'Update|Hotfix')) { return }
    $uninst = Get-RegText $p.UninstallString
    if (-not $uninst) { $uninst = Get-RegText $p.QuietUninstallString }
    if (-not $uninst) { return }
    if ($seen.ContainsKey($name)) { return }
    $seen[$name] = $true
    $size = [long]0; try { if ($p.EstimatedSize) { $size = [long]$p.EstimatedSize * 1024 } } catch {}
    # Icona: DisplayIcon, poi l'icona del pacchetto MSI, poi la cartella del programma, poi il disinstallatore.
    $ico = Get-RegText $p.DisplayIcon
    if (-not $ico) {
      $pg = Get-PackedGuid $_.PSChildName
      if ($pg) { try { $ico = Get-RegText (Get-ItemProperty -LiteralPath ('HKLM:\SOFTWARE\Classes\Installer\Products\' + $pg) -Name ProductIcon -ErrorAction Stop).ProductIcon } catch {} }
    }
    [void]$out.Add([ordered]@{ key = $_.Name; name = $name; publisher = (Get-RegText $p.Publisher); version = (Get-RegText $p.DisplayVersion); size = $size; date = (Get-RegText $p.InstallDate); icon = $ico; loc = (Get-RegText $p.InstallLocation); uninst = $uninst })
  }
}
$sorted = @($out | Sort-Object { $_.name })
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject $sorted -Compress -Depth 3))
`

// orphanHelper (3.1): una voce è "orfana" se il programma non c'è più davvero: il suo programma di
// disinstallazione manca E la cartella del programma non c'è (o sono rimasti solo file che non sono
// programmi, tipo impostazioni e registri). Per i pacchetti MSI decide Windows Installer.
// Come il Pannello di controllo: "potrebbe essere già stato disinstallato, vuoi toglierlo dall'elenco?".
const orphanHelper = arpHelper + `
function Get-UninstallFile([string]$cmd) {
  $cmd = [Environment]::ExpandEnvironmentVariables((Get-RegText $cmd))
  if (-not $cmd) { return '' }
  if ($cmd.StartsWith('"')) { $i = $cmd.IndexOf('"', 1); if ($i -gt 1) { return $cmd.Substring(1, $i - 1) } else { return '' } }
  $m = [regex]::Match($cmd, '(?i)^([A-Z]:\\[^"]*?\.(exe|com|bat|cmd))(?=\s|$)')
  if ($m.Success) { return $m.Groups[1].Value }
  return ''
}
function Test-FileThere([string]$f, [bool]$wow) {
  if (-not $f) { return $true }
  $f = [Environment]::ExpandEnvironmentVariables($f)
  if ($f -notmatch '^[A-Za-z]:\\') { return $true }
  if (-not (Test-Path -LiteralPath ($f.Substring(0, 3)))) { return $true }
  $c = @($f)
  if ($wow) {
    $c += ($f -ireplace '\\System32\\', '\SysWOW64\')
    $pf = [Environment]::GetEnvironmentVariable('ProgramFiles'); $px = [Environment]::GetEnvironmentVariable('ProgramFiles(x86)')
    if ($pf -and $px -and $f.StartsWith($pf + '\', [StringComparison]::OrdinalIgnoreCase)) { $c += ($px + $f.Substring($pf.Length)) }
  }
  foreach ($x in $c) { if (Test-Path -LiteralPath $x -PathType Leaf) { return $true } }
  return $false
}
# La cartella del programma c'è ancora con dentro dei programmi? Nel dubbio (disco scollegato,
# cartelle generiche come C:\ o C:\Program Files) rispondo di sì.
function Test-ProgramDir([string]$loc) {
  $d = [Environment]::ExpandEnvironmentVariables((Get-RegText $loc)).Trim().Trim('"').TrimEnd('\')
  if (-not $d) { return $false }
  if ($d -notmatch '^[A-Za-z]:\\.') { return $true }
  if (-not (Test-Path -LiteralPath ($d.Substring(0, 3)))) { return $true }
  foreach ($g in @($env:ProgramFiles, ${env:ProgramFiles(x86)}, $env:ProgramW6432, $env:ProgramData, $env:windir, $env:LOCALAPPDATA, $env:APPDATA, $env:USERPROFILE, $env:PUBLIC, (Split-Path -Parent $env:USERPROFILE), $env:CommonProgramFiles, ${env:CommonProgramFiles(x86)})) {
    if ($g -and $d -ieq $g.TrimEnd('\')) { return $true }
  }
  if ($env:LOCALAPPDATA -and $d -ieq (Join-Path $env:LOCALAPPDATA 'Programs')) { return $true }
  if (-not (Test-Path -LiteralPath $d -PathType Container)) { return $false }
  # Cerco un file di programma (al massimo 5000 elementi, senza seguire i collegamenti di cartella).
  $todo = New-Object System.Collections.Queue; $todo.Enqueue($d); $n = 0
  while ($todo.Count -gt 0) {
    $cur = $todo.Dequeue()
    foreach ($e in @(Get-ChildItem -LiteralPath $cur -Force -ErrorAction SilentlyContinue)) {
      if (++$n -gt 5000) { return $true }
      if ($e.PSIsContainer) { if (-not ($e.Attributes -band [IO.FileAttributes]::ReparsePoint)) { $todo.Enqueue($e.FullName) } }
      elseif ($e.Extension -match '(?i)^\.(exe|dll|sys|msi|com|jar|bat|cmd|ps1)$') { return $true }
    }
  }
  return $false
}
# MSI: orfana solo se Windows Installer dice che il prodotto non esiste e non ne trovo traccia.
function Test-MsiGone([string]$cmd, [string]$key) {
  $guid = [regex]::Match($cmd, '\{[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\}').Value
  if (-not $guid) { $guid = [regex]::Match(($key -replace '^.*\\', ''), '^\{[0-9A-Fa-f-]{36}\}$').Value }
  $pg = Get-PackedGuid $guid
  if (-not $pg) { return $false }
  try { $wi = New-Object -ComObject WindowsInstaller.Installer; $st = [int]$wi.GetType().InvokeMember('ProductState', 'GetProperty', $null, $wi, @($guid)) } catch { return $false }
  if ($st -ne -1) { return $false }
  if (Test-Path -LiteralPath ('HKLM:\SOFTWARE\Classes\Installer\Products\' + $pg)) { return $false }
  if (Test-Path -LiteralPath ('HKCU:\Software\Microsoft\Installer\Products\' + $pg)) { return $false }
  foreach ($u in @(Get-ChildItem -LiteralPath 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Installer\UserData' -ErrorAction SilentlyContinue)) {
    if (Test-Path -LiteralPath ($u.PSPath + '\Products\' + $pg)) { return $false }
  }
  return $true
}
function Test-MsiCmd([string]$x) { return $x -match '(?i)(^|[\\"\s])msiexec(\.exe)?("|\s|$)' }
# Orfana SOLO se sono sicuro: nel dubbio (comandi tipo rundll32, cmd, dischi scollegati) no.
function Test-UninstallOrphan($p, [string]$key) {
  if ([string]$p.SystemComponent -eq '1' -or (Get-RegText $p.ParentKeyName)) { return $false }
  $wow = $key -match '(?i)\\WOW6432Node\\|^HKEY_CURRENT_USER\\'
  if (Test-ProgramDir $p.InstallLocation) { return $false }
  $cmd = Get-RegText $p.UninstallString; $q = Get-RegText $p.QuietUninstallString
  if (-not $cmd -and -not $q) {
    if ([string]$p.WindowsInstaller -eq '1') { return (Test-MsiGone '' $key) }
    return $true
  }
  foreach ($x in @($cmd, $q)) {
    if (-not $x) { continue }
    if (Test-MsiCmd $x) { if (Test-MsiGone $x $key) { continue } else { return $false } }
    $f = Get-UninstallFile $x
    if (-not $f) { return $false }
    if (Test-FileThere $f $wow) { return $false }
  }
  return $true
}
# La voce si vede ancora nell'elenco? (Una chiave rimasta senza nome Windows non la mostra più.)
function Test-UserWritable([string]$f) {
  # $true se il file (o una cartella che lo contiene) si può cambiare senza essere amministratore.
  # Fuori da Windows e da Programmi lo considero sempre scrivibile; dentro guardo i permessi.
  try { $full = [IO.Path]::GetFullPath($f) } catch { return $true }
  $trusted = @($TassoWin, $TassoPF, $TassoPF86) | Where-Object { $_ }
  if (-not @($trusted | Where-Object { In-Dir $full $_ }).Count) { return $true }
  $me = ''; try { $me = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value } catch {}
  $weak = @('S-1-1-0', 'S-1-5-11', 'S-1-5-32-545', 'S-1-5-4', $me) | Where-Object { $_ }
  # Solo i diritti che permettono di cambiare o sostituire il file (non quelli di lettura):
  # scrivere, aggiungere, cancellare (anche dalla cartella sopra), cambiare permessi o proprietario.
  $writeMask = 0x2 -bor 0x4 -bor 0x40 -bor 0x10000 -bor 0x40000 -bor 0x80000 -bor 0x10000000 -bor 0x40000000
  $cur = $full
  while ($cur -and $cur.Length -gt 3) {
    $top = @($trusted | Where-Object { $cur -ieq $_.TrimEnd('\') }).Count -gt 0
    try {
      $acl = Get-Acl -LiteralPath $cur -ErrorAction Stop
      foreach ($r in @($acl.Access)) {
        if ([string]$r.AccessControlType -ne 'Allow' -or ([int]$r.PropagationFlags -band 2)) { continue }  # 2 = vale solo per quello che c'è dentro
        $sid = ''; try { $sid = $r.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value } catch { continue }
        if ($weak -contains $sid -and (([int]$r.FileSystemRights) -band $writeMask)) { return $true }
      }
    } catch { return $true }
    if ($top) { break }
    $cur = [IO.Path]::GetDirectoryName($cur)
  }
  return $false
}
function Test-StillListed([string]$kp) {
  if (-not (Test-Path -LiteralPath $kp)) { return $false }
  $x = Get-ItemProperty -LiteralPath $kp -ErrorAction SilentlyContinue
  if (-not $x) { return $false }
  return (Test-ShownName (Get-RegText $x.DisplayName))
}
function Out-Orphan([string]$name, [string]$key) {
  [Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject ([pscustomobject]@{ orphan = $true; name = $name; key = $key }) -Compress))
}
`

// uninstallScript: ofHelpers serve per Start-AsUser (programmi installati solo per il tuo utente).
const uninstallScript = ofHelpers + orphanHelper + `
$kp = 'Registry::' + [string]$A.key
$p = Get-ItemProperty -LiteralPath $kp -ErrorAction SilentlyContinue
if (-not $p) { Say 'Programma non trovato: forse è già stato disinstallato.'; return }
$name = Resolve-RegText (Get-RegText $p.DisplayName)
if (-not (Test-ShownName $name)) { $name = [string]$A.key -replace '^.*\\', '' }
$cmd = Get-RegText $p.UninstallString
if (-not $cmd) { $cmd = Get-RegText $p.QuietUninstallString }
if (Test-UninstallOrphan $p ([string]$A.key)) { Out-Orphan $name ([string]$A.key); return }
if (-not $cmd) { Say ('Questo programma non ha un programma di disinstallazione. Prova da Impostazioni > App di Windows.'); return }
$file = ''; $a2 = ''; $isMsi = $false
if (Test-MsiCmd $cmd) {
  # Come Windows: per i pacchetti MSI sempre "disinstalla" (/x), anche se la voce dice /i (che apre "modifica").
  $isMsi = $true
  $guid = [regex]::Match($cmd, '\{[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\}').Value
  $file = Join-Path $TassoSys32 'msiexec.exe'
  if ($guid) { $a2 = '/x ' + $guid } else { $a2 = ($cmd -replace '(?i)^.*?msiexec(\.exe)?"?\s*', '') }
} elseif ($cmd.StartsWith('"')) {
  $i = $cmd.IndexOf('"', 1)
  if ($i -gt 1) { $file = $cmd.Substring(1, $i - 1); $a2 = $cmd.Substring($i + 1).Trim() } else { $file = $cmd.Trim('"') }
} else {
  # Percorso senza virgolette, anche con spazi: prendo il primo file .exe che esiste davvero.
  $ms = @([regex]::Matches($cmd, '(?i)\.(exe|com|bat|cmd)(?=\s|$)'))
  if ($ms.Count) {
    $file = $cmd.Substring(0, $ms[0].Index + $ms[0].Length); $a2 = $cmd.Substring($ms[0].Index + $ms[0].Length).Trim()
    foreach ($mm in $ms) {
      $cand = $cmd.Substring(0, $mm.Index + $mm.Length)
      if (Test-Path -LiteralPath ([Environment]::ExpandEnvironmentVariables($cand)) -PathType Leaf) { $file = $cand; $a2 = $cmd.Substring($mm.Index + $mm.Length).Trim(); break }
    }
  } else {
    # Senza estensione (es. "RunDll32 C:\...\file.dll,Funzione"): il programma è la prima parola.
    $sp = $cmd.IndexOf(' '); if ($sp -gt 0) { $file = $cmd.Substring(0, $sp); $a2 = $cmd.Substring($sp + 1).Trim() } else { $file = $cmd }
  }
}
# Le variabili come %ProgramFiles% le scrivo per esteso, e i comandi di Windows (rundll32...) li prendo da System32.
$file = [Environment]::ExpandEnvironmentVariables($file).Trim().Trim('"')
if ($file -and $file -notmatch '^[A-Za-z]:\\|^\\\\') { $sysF = Join-Path $TassoSys32 $file; if (-not $sysF.EndsWith('.exe', [StringComparison]::OrdinalIgnoreCase) -and (Test-Path -LiteralPath ($sysF + '.exe'))) { $sysF += '.exe' }; if (Test-Path -LiteralPath $sysF -PathType Leaf) { $file = $sysF } }
# Il programma di disinstallazione non c'è più e nemmeno il programma: come Windows, propongo di toglierlo dall'elenco.
if (-not $isMsi -and $file -match '^[A-Za-z]:\\' -and -not (Test-FileThere $file (([string]$A.key) -match '(?i)\\WOW6432Node\\|^HKEY_CURRENT_USER\\'))) {
  if (-not (Test-ProgramDir $p.InstallLocation)) { Out-Orphan $name ([string]$A.key); return }
  Say ('Il programma di disinstallazione non c''è più: ' + $file + '. La cartella del programma però c''è ancora: prova a reinstallarlo e poi a disinstallarlo.'); return
}
Say ('Avvio la disinstallazione di ' + $name + '. Segui le finestre che si aprono.')
$t0 = Get-Date
# Cartelle temporanee dove i disinstallatori fanno la copia di sé: quella di Tasso ($env:TEMP, per quelli
# avviati da amministratore) e quella vera dell'utente (per quelli avviati come utente normale): di
# solito AppData\Local\Temp, oppure quella dove l'utente ha spostato i temporanei ($TassoTempOriginale).
$tempDirs = @()
foreach ($td in @($env:TEMP, $TassoTempUtente, $TassoTempOriginale)) {
  if (-not $td) { continue }
  $tempDirs += $td.TrimEnd('\') + '\'
  try { $tempDirs += (Get-Item -LiteralPath $td -ErrorAction Stop).FullName.TrimEnd('\') + '\' } catch {}
}
# Programma installato solo per il tuo utente (HKCU): il suo disinstallatore lo può cambiare qualunque
# programma che gira col tuo account, quindi parte come utente normale, mai da amministratore.
$asUser = ([string]$A.key) -match '(?i)^HKEY_CURRENT_USER\\'
# Anche una voce HKLM può puntare a un disinstallatore in una cartella dove scrive un programma
# qualsiasi (C:\ProgramData\<app>, C:\<app>, il profilo...): chi lo sostituisce lo farebbe partire da
# amministratore senza nessuna domanda. In quel caso parte come utente normale, come farebbe Windows
# (se gli servono i permessi li chiede lui con la finestra di Windows).
if (-not $asUser -and -not $isMsi -and $file -match '^[A-Za-z]:\\' -and (Test-UserWritable ([Environment]::ExpandEnvironmentVariables($file).Trim('"')))) {
  $asUser = $true
  Say 'Il programma di disinstallazione sta in una cartella dove possono scrivere anche i programmi normali: lo faccio partire senza i permessi di amministratore (se gli servono, te li chiede Windows).'
}
try {
  if ($asUser) {
    $exe = [Environment]::ExpandEnvironmentVariables($file).Trim('"')
    if ($exe -notmatch '^[A-Za-z]:\\') { $exe = Join-Path $TassoSys32 $exe }
    if (-not (Start-AsUser $exe $a2)) { Say ('Non riesco ad avviare il programma di disinstallazione: ' + $exe); return }
    # Start-AsUser non restituisce il processo: aspetto finché quel programma resta aperto (al massimo un'ora).
    $end = $t0.AddHours(1); $seen = $false; $w = 0
    while ((Get-Date) -lt $end) {
      $run = @(Get-Process -ErrorAction SilentlyContinue | Where-Object { $st = $null; try { $st = $_.StartTime } catch {}; $st -and $st -ge $t0 -and $_.SessionId -ne 0 -and "$($_.Path)" -ieq $exe })
      if ($run.Count) { $seen = $true } elseif ($seen -or $w -ge 30) { break }
      Start-Sleep -Seconds 1; $w++
    }
  } else {
    # Aspetto solo il programma di disinstallazione, non le pagine web o i browser che apre lui.
    if ($a2) { $up = Start-Process -FilePath $file -ArgumentList $a2 -PassThru -ErrorAction Stop } else { $up = Start-Process -FilePath $file -PassThru -ErrorAction Stop }
    if ($up) {
      $null = $up.Handle  # senza questo, a volte la PowerShell non sa più il codice d'uscita
      [void]$up.WaitForExit(3600000)
      # 1605: Windows Installer dice che il prodotto non è installato. Se la voce è rimasta, è orfana.
      $ec = $null; try { $ec = $up.ExitCode } catch {}
      if ($isMsi -and $ec -eq 1605 -and (Test-Path -LiteralPath $kp)) {
        $p2 = Get-ItemProperty -LiteralPath $kp -ErrorAction SilentlyContinue
        if ($p2 -and (Test-UninstallOrphan $p2 ([string]$A.key))) { Out-Orphan $name ([string]$A.key); return }
      }
    }
  }
} catch {
  Say ('Non riesco ad avviare il programma di disinstallazione: ' + $_.Exception.Message)
  return
}
# Molti disinstallatori (NSIS, Inno) fanno una copia di sé nella cartella temporanea e chiudono subito
# il primo programma: finché quella copia è aperta (la finestra dove scegli) continuo ad aspettare.
$t = 0; $idle = 0
while ((Test-StillListed $kp) -and $t -lt 900 -and $idle -lt 45) {
  Start-Sleep -Seconds 2; $t++
  $helpers = @(Get-Process -ErrorAction SilentlyContinue | Where-Object {
    $st = $null; try { $st = $_.StartTime } catch {}
    $pp = "$($_.Path)"
    $st -and $st -ge $t0 -and (@($tempDirs | Where-Object { $pp.StartsWith($_, [StringComparison]::OrdinalIgnoreCase) }).Count -gt 0 -or $_.ProcessName -match '(?i)uninst|^au_$|^un_|^unins|^_iu')
  })
  if ($helpers.Count -gt 0) { $idle = 0 } else { $idle++ }
}
$removed = -not (Test-StillListed $kp)
# Alcuni disinstallatori tolgono la voce un attimo dopo aver chiuso la finestra: ricontrollo per qualche secondo.
for ($r = 0; -not $removed -and $r -lt 5; $r++) { Start-Sleep -Seconds 2; $removed = -not (Test-StillListed $kp) }
if ($removed) { Say ($name + ' è stato disinstallato.') } else { Say 'La disinstallazione non risulta completata (forse l''hai annullata).' }
Say 'Cerco i resti lasciati nel PC e nel registro...'
$pub = ([string]$p.Publisher).Trim()
$stop = @('microsoft','adobe','epic','riot','nvidia','intel','google','logitech','razer','corsair','asus','acer','lenovo','dell','apple','amazon','oracle','autodesk','ubisoft','electronic','arts','valve','steam','blizzard','activision','realtek','mozilla','office','visual','runtime','redistributable','driver','drivers','teams','classic','online','services','service','the','and','for','with','version','edition','x64','x86','win','windows','inc','ltd','llc','corporation','corp','company','software','app','application','tool','tools','free','pro','plus','setup','installer','launcher','games','game','data','cache','update','common','program','programs','files','user','users','system','default','client','studio','studios','media','player','video','audio','music','photo','photos','defender','security','edge','store','browser','manager','center','control','cloud','drive','sync','print','printer','network','helper','assistant','core','desktop','mail','internet','explorer','power','shell')
$tokens = New-Object System.Collections.ArrayList
# Solo dal nome del programma: le parole del produttore (es. "Epic Games") prenderebbero le cartelle di altri programmi.
foreach ($src in @($name)) {
  foreach ($w in ([regex]::Split([string]$src, '[^A-Za-z0-9]+'))) {
    if ($w.Length -ge 4 -and $stop -notcontains $w.ToLower() -and -not $tokens.Contains($w)) { [void]$tokens.Add($w) }
  }
}
$cands = New-Object System.Collections.ArrayList
if ($p.InstallLocation) { [void]$cands.Add(([string]$p.InstallLocation).Trim().Trim('"').TrimEnd('\')) }
$roots = @($env:ProgramFiles, ${env:ProgramFiles(x86)}, $env:LOCALAPPDATA, $env:APPDATA, $env:ProgramData, (Join-Path $env:LOCALAPPDATA 'Programs'))
foreach ($r in $roots) {
  if (-not $r) { continue }
  try { [void]$cands.Add((Join-Path $r $name)) } catch {}
  if (-not (Test-Path -LiteralPath $r)) { continue }
  Get-ChildItem -LiteralPath $r -Directory -ErrorAction SilentlyContinue | ForEach-Object {
    $dn = $_.Name
    foreach ($tk in $tokens) { if ($dn -like ('*' + $tk + '*')) { [void]$cands.Add($_.FullName); break } }
  }
}
# Cartelle degli altri programmi ancora installati: non le propongo mai (né quelle che le contengono).
$keepDirs = New-Object System.Collections.ArrayList
foreach ($ur in @('HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall', 'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall', 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall')) {
  Get-ChildItem -LiteralPath $ur -ErrorAction SilentlyContinue | ForEach-Object {
    if (('Registry::' + $_.Name) -eq $kp) { return }
    $q = Get-ItemProperty -LiteralPath $_.PSPath -ErrorAction SilentlyContinue
    if (-not $q) { return }
    foreach ($v in @([string]$q.InstallLocation, [string]$q.DisplayIcon)) {
      $v = ($v -replace ',\s*-?\d+$', '').Trim().Trim('"')
      if (-not $v) { continue }
      if ($v -match '\.(exe|ico|dll)$') { $v = Split-Path -Parent $v }
      $v = $v.TrimEnd('\')
      if ($v.Length -gt 3) { [void]$keepDirs.Add($v.ToLower()) }
    }
  }
}
$left = New-Object System.Collections.ArrayList
$done = @{}
foreach ($c in $cands) {
  if (-not $c -or $done.ContainsKey($c.ToLower())) { continue }
  $done[$c.ToLower()] = $true
  # Cartelle di Windows e cartelle comuni: mai, anche se il nome somiglia.
  if (@('windows media player','windows defender','windows defender advanced threat protection','windows nt','windows mail','windows photo viewer','windows sidebar','windows portable devices','windows security','windowspowershell','internet explorer','microsoft','common files','packages','temp','microsoft office','dotnet','modifiablewindowsapps','windowsapps','microsoft shared','windows kits','reference assemblies','msbuild','package cache') -contains (Split-Path -Leaf $c).ToLower()) { continue }
  $cl = $c.ToLower().TrimEnd('\')
  $other = $false
  foreach ($kd in $keepDirs) { if ($kd -eq $cl -or $kd.StartsWith($cl + '\')) { $other = $true; break } }
  if ($other) { continue }
  if (Test-Path -LiteralPath $c -PathType Container) { [void]$left.Add([ordered]@{ path = $c; size = (Get-FolderSize $c) }) }
}
$regBlock = @('microsoft','windows','classes','clients','policies','wow6432node','registeredapplications','intel','nvidia','amd','realtek','google','khronos','odbc','rpc','partner','mozilla','valve')
$regLeft = New-Object System.Collections.ArrayList
$regDone = @{}
if ($tokens.Count -gt 0) {
  foreach ($rr in @('HKCU:\Software', 'HKLM:\SOFTWARE', 'HKLM:\SOFTWARE\WOW6432Node')) {
    Get-ChildItem -LiteralPath $rr -ErrorAction SilentlyContinue | ForEach-Object {
      $kn = [string]$_.PSChildName
      if (-not $kn -or $regBlock -contains $kn.ToLower()) { return }
      foreach ($tk in $tokens) {
        if ($kn -like ('*' + $tk + '*')) {
          $full = [string]$_.Name
          if (-not $regDone.ContainsKey($full.ToLower())) { $regDone[$full.ToLower()] = $true; [void]$regLeft.Add([ordered]@{ key = $full }) }
          break
        }
      }
    }
  }
}
$o = [ordered]@{ removed = $removed; name = $name; leftovers = @($left); regleft = @($regLeft) }
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject $o -Compress -Depth 4))
`

const bloatListScript = `
$out = New-Object System.Collections.ArrayList
foreach ($k in @($A)) {
  $pk = Get-AppxPackage -Name ([string]$k.id) -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($pk) { [void]$out.Add([ordered]@{ id = [string]$k.id; name = [string]$k.name; junk = [bool]$k.junk }) }
}
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject @($out) -Compress -Depth 3))
`

const bloatRemoveScript = `
# App che tengono dati tuoi SOLO sul PC (le note di Sticky Notes senza account Microsoft, i contatti,
# le liste di To Do): togliere l'app cancella anche quelli. Prima ne faccio una copia nella cartella
# backup di Tasso; se la copia non riesce, l'app resta.
$withData = '(?i)^Microsoft\.(MicrosoftStickyNotes|People|Todos)$'
$bdir = [string]$A.dir
foreach ($id in @($A.ids)) {
  $pks = @(Get-AppxPackage -Name ([string]$id) -ErrorAction SilentlyContinue)
  # Nei messaggi il nome che vedi nell'elenco (Microsoft Notizie), non quello tecnico (Microsoft.BingNews).
  $nm = ''; try { $nm = [string]$A.names.PSObject.Properties[[string]$id].Value } catch {}
  foreach ($pk in $pks) {
    if ($nm) { $pn = $nm } else { $pn = $pk.Name }
    if ($pk.Name -match $withData) {
      $data = Join-Path $env:LOCALAPPDATA ('Packages\' + $pk.PackageFamilyName)
      $copied = $false; $dst = ''
      if (Test-Path -LiteralPath $data) {
        if (@(Get-Process -ErrorAction SilentlyContinue | Where-Object { [string]$_.Path -like ($pk.InstallLocation + '\*') }).Count) { Say ('Non rimossa: ' + $pn + ' è aperta. Chiudila e riprova (prima copio le tue note e i tuoi dati).'); continue }
        try {
          $dst = Join-Path $bdir ('app-' + $pk.Name + '-' + (Get-Date -Format 'yyyyMMdd_HHmmss'))
          foreach ($sub in @('LocalState', 'RoamingState', 'Settings')) {
            $from = Join-Path $data $sub
            if (Test-Path -LiteralPath $from) { [void](New-Item -ItemType Directory -Path $dst -Force -ErrorAction Stop); Copy-Item -LiteralPath $from -Destination $dst -Recurse -Force -ErrorAction Stop }
          }
          $copied = $true
        } catch { $copied = $false }
      } else { $copied = $true }
      if (-not $copied) { Say ('Non rimossa: ' + $pn + '. Non sono riuscito a fare la copia dei tuoi dati (le note di Sticky Notes, per esempio), quindi la lascio.'); continue }
      if ($dst -and (Test-Path -LiteralPath $dst)) { Say ('Copia dei dati di ' + $pn + ' salvata in: ' + $dst + ' (per le note di Sticky Notes è il file plum.sqlite).') }
    }
    try { Remove-AppxPackage -Package $pk.PackageFullName -ErrorAction Stop; Say ('Rimossa: ' + $pn) } catch { Say ('Non rimossa: ' + $pn) }
  }
}
`

const filesScanScript = `
$dl = Get-DownloadsPath
$roots = @([Environment]::GetFolderPath('Desktop'), [Environment]::GetFolderPath('MyDocuments'), [Environment]::GetFolderPath('MyPictures'), [Environment]::GetFolderPath('MyVideos'), [Environment]::GetFolderPath('MyMusic'), $dl)
$extra = [string]$A.extra
if ($extra -and (Test-Path -LiteralPath $extra -PathType Container)) { $roots += $extra }
$roots = @($roots | Where-Object { $_ -and (Test-Path -LiteralPath $_) } | Select-Object -Unique)
$min = [long]$A.min; if ($min -lt 50MB) { $min = 300MB }
# Mai dentro Windows, i programmi, i giochi e i dati delle app: lì i "doppioni" servono.
$noTouch = '(?i)\\(windows|program files( \(x86\))?|programdata|\$recycle\.bin|system volume information|windowsapps|steamapps|steamlibrary|epic games|riot games|xboxgames|battle\.net|ubisoft game launcher|ea games|gog galaxy|appdata|windows\.old|\$windows\.~bt|\$windows\.~ws|\$winreagent|recovery|perflogs)\\'
$skip = 0x1000 -bor 0x40000 -bor 0x400000
$files = New-Object System.Collections.Generic.List[object]
$seen = @{}
foreach ($r in $roots) {
  Say ('Guardo in ' + $r + '...')
  Get-ChildItem -LiteralPath $r -Recurse -File -ErrorAction SilentlyContinue | ForEach-Object {
    if (-not ([int]$_.Attributes -band $skip) -and -not $seen.ContainsKey($_.FullName) -and ($_.FullName -notmatch $noTouch)) { $seen[$_.FullName] = $true; $files.Add($_) }
  }
}
Say ('File controllati: ' + $files.Count)
$yearAgo = (Get-Date).AddYears(-1)
$large = @($files | Where-Object { $_.Length -ge $min } | Sort-Object Length -Descending | Select-Object -First 50 | ForEach-Object { [ordered]@{ path = $_.FullName; size = $_.Length; date = $_.LastWriteTime.ToString('dd/MM/yyyy'); old = ($_.LastWriteTime -lt $yearAgo) } })
Say 'Cerco i file doppi...'
$imgExt = '(?i)\.(jpe?g|png|heic|heif|gif|webp|bmp|tiff?|raw|cr2|nef|arw|dng)$'
$groups = New-Object System.Collections.ArrayList
foreach ($sg in ($files | Where-Object { $_.Length -ge 1MB -or ($_.Length -ge 50KB -and $_.Name -match $imgExt) } | Group-Object Length | Where-Object { $_.Count -gt 1 })) {
  if ($groups.Count -ge 300) { break }
  $hs = @(foreach ($f in $sg.Group) { $h = Get-FileHash -LiteralPath $f.FullName -Algorithm MD5 -ErrorAction SilentlyContinue; if ($h) { [pscustomobject]@{ F = $f; H = $h.Hash } } })
  foreach ($hg in ($hs | Group-Object H | Where-Object { $_.Count -gt 1 })) {
    $ord = @($hg.Group | ForEach-Object { $_.F } | Sort-Object @{ Expression = { $_.FullName.StartsWith($dl) } }, CreationTime)
    $items = @($ord | ForEach-Object { [ordered]@{ path = $_.FullName; size = $_.Length; date = $_.LastWriteTime.ToString('dd/MM/yyyy') } })
    [void]$groups.Add($items)
  }
}
Say 'Cerco le cartelle vuote...'
$empty = New-Object System.Collections.ArrayList
foreach ($r in $roots) {
  Get-ChildItem -LiteralPath $r -Recurse -Directory -ErrorAction SilentlyContinue | Where-Object { -not ($_.Attributes -band [IO.FileAttributes]::Hidden) -and -not ($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -and -not ($_.Attributes -band [IO.FileAttributes]::System) -and (($_.FullName + '\') -notmatch $noTouch) } | ForEach-Object {
    if ($empty.Count -lt 300) {
      try { $e = [IO.Directory]::EnumerateFileSystemEntries($_.FullName).GetEnumerator(); if (-not $e.MoveNext()) { [void]$empty.Add($_.FullName) } } catch {}
    }
  }
}
Say 'Controllo i collegamenti...'
$broken = New-Object System.Collections.ArrayList
$sh = New-Object -ComObject WScript.Shell
foreach ($r in @([Environment]::GetFolderPath('Desktop'), [Environment]::GetFolderPath('CommonDesktopDirectory'), [Environment]::GetFolderPath('StartMenu'), [Environment]::GetFolderPath('CommonStartMenu'))) {
  if (-not $r -or -not (Test-Path -LiteralPath $r)) { continue }
  Get-ChildItem -LiteralPath $r -Recurse -Filter '*.lnk' -File -ErrorAction SilentlyContinue | ForEach-Object {
    $t = ''
    try { $t = [string]$sh.CreateShortcut($_.FullName).TargetPath } catch {}
    if ($t -and ($t -match '^[A-Za-z]:\\') -and (Test-Path -LiteralPath $t.Substring(0, 3)) -and -not (Test-Path -LiteralPath $t)) { [void]$broken.Add([ordered]@{ path = $_.FullName; target = $t }) }
  }
}
$o = [ordered]@{ count = $files.Count; large = $large; dups = @($groups); empty = @($empty); broken = @($broken) }
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject $o -Compress -Depth 6))
`

const recycleScript = `
Add-Type -AssemblyName Microsoft.VisualBasic
$protect = @{}
foreach ($f in @('Desktop', 'MyDocuments', 'MyPictures', 'MyVideos', 'MyMusic', 'UserProfile', 'StartMenu', 'CommonStartMenu', 'CommonDesktopDirectory', 'Programs', 'CommonPrograms', 'Startup', 'CommonStartup')) {
  $x = [Environment]::GetFolderPath($f); if ($x) { $protect[$x.TrimEnd('\').ToLower()] = $true }
}
$protect[(Get-DownloadsPath).TrimEnd('\').ToLower()] = $true
# Quanto ci sta nel Cestino di ogni disco: se un elemento è più grande, Windows lo cancellerebbe
# per sempre senza chiedere. In quel caso non lo tocco e te lo dico.
$binCache = @{}
function Get-BinLimit([string]$path) {
  $dl = ''
  if ($path -match '^([A-Za-z]):\\') { $dl = $Matches[1].ToUpper() + ':' } else { return [long]0 }
  if ($binCache.ContainsKey($dl)) { return $binCache[$dl] }
  $lim = [long]0
  try {
    $v = Get-CimInstance Win32_Volume -Filter ("DriveLetter='" + $dl + "'") -ErrorAction Stop | Select-Object -First 1
    if ($v -and [int]$v.DriveType -eq 3) {
      $lim = [long]([double]$v.Capacity * 0.05)
      $g = [regex]::Match([string]$v.DeviceID, '\{[0-9A-Fa-f-]{36}\}').Value
      if ($g) {
        $k = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\BitBucket\Volume\' + $g
        $q = Get-ItemProperty -LiteralPath $k -ErrorAction SilentlyContinue
        $nuke = $null; $max = $null; if ($q) { $nuke = $q.NukeOnDelete; $max = $q.MaxCapacity }
        if ($nuke -eq 1) { $lim = [long]0 } elseif ($max) { $lim = [long]$max * 1MB }
      }
    }
  } catch {}
  $binCache[$dl] = $lim
  return $lim
}
$n = 0; $b = [long]0; $big = 0
$spostati = New-Object System.Collections.ArrayList  # solo quelli finiti davvero nel Cestino
foreach ($p in @($A)) {
  $p = [string]$p
  if ($protect.ContainsKey($p.TrimEnd('\').ToLower())) { Say ('Cartella di sistema, la salto: ' + $p); continue }
  try {
    if (Test-Path -LiteralPath $p -PathType Leaf) {
      $len = (Get-Item -LiteralPath $p -Force).Length
      if ($len -gt (Get-BinLimit $p)) { $big++; Say ('Troppo grande per il Cestino (verrebbe cancellato per sempre), non lo tocco: ' + $p); continue }
      [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($p, 'OnlyErrorDialogs', 'SendToRecycleBin')
      if (-not (Test-Path -LiteralPath $p)) { $n++; $b += $len; [void]$spostati.Add($p) } else { Say ('Non spostato: ' + $p) }
    } elseif (Test-Path -LiteralPath $p -PathType Container) {
      $len = Get-FolderSize $p
      if ($len -gt (Get-BinLimit $p)) { $big++; Say ('Troppo grande per il Cestino (verrebbe cancellato per sempre), non la tocco: ' + $p); continue }
      [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteDirectory($p, 'OnlyErrorDialogs', 'SendToRecycleBin')
      if (-not (Test-Path -LiteralPath $p)) { $n++; $b += $len; [void]$spostati.Add($p) } else { Say ('Non spostato: ' + $p) }
    }
  } catch { Say ('Non spostato: ' + $p) }
}
Say ('Spostati nel Cestino: ' + $n + ' elementi (' + (Fmt $b) + '). Puoi sempre recuperarli dal Cestino.')
if ($big) { Say ('Non toccati perché troppo grandi per il Cestino: ' + $big + '. Se vuoi davvero toglierli, cancellali tu da Esplora file sapendo che non si recuperano.') }
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject ([ordered]@{ n = $n; bytes = $b; moved = @($spostati) }) -Compress))
`

const regRemoveScript = `
$dir = [string]$A.dir
if ($dir -and -not (Test-Path -LiteralPath $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
$stamp = Get-Date -Format 'yyyyMMdd_HHmmss'
$n = 0
foreach ($k in @($A.keys)) {
  $k = [string]$k
  if (-not $k) { continue }
  $ps = $k -replace '^HKEY_CURRENT_USER\\', 'HKCU:\' -replace '^HKEY_LOCAL_MACHINE\\', 'HKLM:\'
  if (-not (Test-Path -LiteralPath $ps)) { Say ('Chiave già rimossa: ' + $k); continue }
  if ($dir) {
    $safe = ($k -replace '[^A-Za-z0-9]+', '_')
    if ($safe.Length -gt 80) { $safe = $safe.Substring(0, 80) }
    $bak = Join-Path $dir ($stamp + '_' + $safe + '.reg')
    try { & reg.exe export $k $bak /y 2>$null | Out-Null } catch {}
    if (-not (Test-Path -LiteralPath $bak)) { Say ('Non rimossa dal registro (non sono riuscito a farne il backup): ' + $k); continue }
  } else { Say ('Non rimossa dal registro (manca la cartella dei backup): ' + $k); continue }
  try { Remove-Item -LiteralPath $ps -Recurse -Force -ErrorAction Stop; $n++; Say ('Rimossa dal registro (backup salvato): ' + $k) } catch { Say ('Non rimossa dal registro: ' + $k) }
}
Say ('Chiavi di registro rimosse: ' + $n + '. I backup .reg sono nella cartella dei registri, cartella backup: basta farci doppio clic per rimetterle.')
`

const pickFolderScript = `
$sh = New-Object -ComObject Shell.Application
$f = $sh.BrowseForFolder([int]$A.hwnd, 'Scegli una cartella o un disco in più da controllare (per esempio D:\)', 0x11)
$p = ''
if ($f) { try { $p = [string]$f.Self.Path } catch {} }
if ($p -and -not (Test-Path -LiteralPath $p -PathType Container)) { $p = '' }
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject ([ordered]@{ path = $p }) -Compress))
`

// removeOrphanScript (3.1): toglie dall'elenco una voce orfana, solo se è ancora orfana,
// dopo averne salvato una copia .reg nella cartella dei backup (doppio clic per rimetterla).
const removeOrphanScript = orphanHelper + `
$k = [string]$A.key
$kp = 'Registry::' + $k
$i = $k.LastIndexOf('\'); $leaf = $k.Substring($i + 1)
$names = @(Get-ChildItem -LiteralPath ('Registry::' + $k.Substring(0, $i)) -ErrorAction SilentlyContinue | ForEach-Object { $_.PSChildName })
if ($names -cnotcontains $leaf) { Say 'Fatto: non era più nell''elenco.'; [Console]::Out.WriteLine('JSON {"ok":true}'); return }
$p = Get-ItemProperty -LiteralPath $kp -ErrorAction SilentlyContinue
if (-not $p) { Say 'Fatto: non era più nell''elenco.'; [Console]::Out.WriteLine('JSON {"ok":true}'); return }
if (-not (Test-UninstallOrphan $p $k)) { Say 'Questo programma sembra ancora installato: non lo tolgo dall''elenco. Prova a disinstallarlo.'; [Console]::Out.WriteLine('JSON {"ok":false}'); return }
$dir = [string]$A.dir
try { [void][IO.Directory]::CreateDirectory($dir) } catch {}
$safe = (((Get-RegText $p.DisplayName) + '_' + ($k -replace '^.*\\', '')) -replace '[^A-Za-z0-9]+', '_').Trim('_'); if ($safe.Length -gt 80) { $safe = $safe.Substring(0, 80) }
$bak = Join-Path $dir ('programma_tolto_' + (Get-Date -Format 'yyyyMMdd_HHmmss') + '_' + $safe + '.reg')
$rc = 1
try { & (Join-Path $TassoSys32 'reg.exe') export $k $bak /y 2>$null | Out-Null; $rc = $LASTEXITCODE } catch {}
if ($rc -ne 0 -or -not (Test-Path -LiteralPath $bak) -or (Get-Item -LiteralPath $bak).Length -lt 20) { Say 'Non l''ho tolto: non sono riuscito a farne una copia di sicurezza.'; [Console]::Out.WriteLine('JSON {"ok":false}'); return }
try { Remove-Item -LiteralPath $kp -Recurse -Force -ErrorAction Stop; Say ('Tolto dall''elenco: ' + (Get-RegText $p.DisplayName) + '. La copia di sicurezza è nella cartella backup di Tasso.'); [Console]::Out.WriteLine('JSON {"ok":true}') }
catch { Say 'Non sono riuscito a toglierlo dall''elenco.'; [Console]::Out.WriteLine('JSON {"ok":false}') }
`
