package main

const startupListScript = `
$out = Get-TassoStartup
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject @($out) -Compress -Depth 3))
`

const startupSetScript = `
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
  if (-not (Test-Path -LiteralPath $k)) { New-Item -Path $k -Force | Out-Null }
  $v = 1; if ($A.enable) { $v = 2 }
  New-ItemProperty -LiteralPath $k -Name 'State' -Value $v -PropertyType DWord -Force | Out-Null
  Say 'Fatto.'
  return
}
$key = $map[[string]$A.scope]
if (-not $key) { Say 'Voce non valida.'; return }
if (-not (Test-Path -LiteralPath $key)) { New-Item -Path $key -Force | Out-Null }
$b = New-Object byte[] 12
if ($A.enable) { $b[0] = 2 } else {
  $b[0] = 3
  $ft = [BitConverter]::GetBytes([DateTime]::Now.ToFileTime())
  [Array]::Copy($ft, 0, $b, 4, 8)
}
New-ItemProperty -LiteralPath $key -Name ([string]$A.name) -Value $b -PropertyType Binary -Force | Out-Null
Say 'Fatto.'
`

const programsListScript = `
$keys = @('HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall', 'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall', 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall')
$seen = @{}
$out = New-Object System.Collections.ArrayList
foreach ($k in $keys) {
  Get-ChildItem -LiteralPath $k -ErrorAction SilentlyContinue | ForEach-Object {
    $p = Get-ItemProperty -LiteralPath $_.PSPath -ErrorAction SilentlyContinue
    if (-not $p) { return }
    $name = [string]$p.DisplayName
    if (-not $name -or $p.SystemComponent -eq 1 -or $p.ParentKeyName -or ([string]$p.ReleaseType -match 'Update|Hotfix') -or -not $p.UninstallString) { return }
    if ($seen.ContainsKey($name)) { return }
    $seen[$name] = $true
    $size = [long]0; if ($p.EstimatedSize) { $size = [long]$p.EstimatedSize * 1024 }
    # Icona: DisplayIcon, poi l'icona del pacchetto MSI, poi la cartella del programma, poi il disinstallatore.
    $ico = [string]$p.DisplayIcon
    if (-not $ico -and $_.PSChildName -match '^\{([0-9A-Fa-f]{8})-([0-9A-Fa-f]{4})-([0-9A-Fa-f]{4})-([0-9A-Fa-f]{4})-([0-9A-Fa-f]{12})\}$') {
      $g = $Matches
      $rev = { param($s) -join ($s.ToCharArray()[($s.Length - 1)..0]) }
      $pair = { param($s) -join (0..($s.Length / 2 - 1) | ForEach-Object { '' + $s[2 * $_ + 1] + $s[2 * $_] }) }
      $packed = (& $rev $g[1]) + (& $rev $g[2]) + (& $rev $g[3]) + (& $pair $g[4]) + (& $pair $g[5])
      try { $ico = [string](Get-ItemProperty -LiteralPath ('HKLM:\SOFTWARE\Classes\Installer\Products\' + $packed) -Name ProductIcon -ErrorAction Stop).ProductIcon } catch {}
    }
    [void]$out.Add([ordered]@{ key = $_.Name; name = $name; publisher = [string]$p.Publisher; version = [string]$p.DisplayVersion; size = $size; date = [string]$p.InstallDate; icon = $ico; loc = [string]$p.InstallLocation; uninst = [string]$p.UninstallString })
  }
}
$sorted = @($out | Sort-Object { $_.name })
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject $sorted -Compress -Depth 3))
`

const uninstallScript = `
$kp = 'Registry::' + [string]$A.key
$p = Get-ItemProperty -LiteralPath $kp -ErrorAction SilentlyContinue
if (-not $p) { Say 'Programma non trovato: forse è già stato disinstallato.'; return }
$name = [string]$p.DisplayName
$cmd = ([string]$p.UninstallString).Trim()
Say ('Avvio la disinstallazione di ' + $name + '. Segui le finestre che si aprono.')
$file = ''; $a2 = ''
if ($cmd -match '(?i)msiexec') {
  $guid = [regex]::Match($cmd, '\{[0-9A-Fa-f-]{36}\}').Value
  $file = 'msiexec.exe'
  if ($guid) { $a2 = '/x ' + $guid } else { $a2 = ($cmd -replace '(?i)^.*?msiexec(\.exe)?"?\s*', '') }
} elseif ($cmd.StartsWith('"')) {
  $i = $cmd.IndexOf('"', 1)
  if ($i -gt 1) { $file = $cmd.Substring(1, $i - 1); $a2 = $cmd.Substring($i + 1).Trim() } else { $file = $cmd.Trim('"') }
} else {
  $m = [regex]::Match($cmd, '(?i)^(.+?\.exe)(.*)$')
  if ($m.Success) { $file = $m.Groups[1].Value; $a2 = $m.Groups[2].Value.Trim() } else { $file = $cmd }
}
$t0 = Get-Date
$tempLong = $env:TEMP; try { $tempLong = (Get-Item -LiteralPath $env:TEMP -ErrorAction Stop).FullName } catch {}
try {
  # Aspetto solo il programma di disinstallazione, non le pagine web o i browser che apre lui.
  if ($a2) { $up = Start-Process -FilePath $file -ArgumentList $a2 -PassThru -ErrorAction Stop } else { $up = Start-Process -FilePath $file -PassThru -ErrorAction Stop }
  if ($up) { [void]$up.WaitForExit(3600000) }
} catch { Say ('Non riesco ad avviare il programma di disinstallazione: ' + $_.Exception.Message); return }
# Molti disinstallatori (NSIS, Inno) fanno una copia di sé nella cartella temporanea e chiudono subito
# il primo programma: finché quella copia è aperta (la finestra dove scegli) continuo ad aspettare.
$t = 0; $idle = 0
while ((Test-Path -LiteralPath $kp) -and $t -lt 900 -and $idle -lt 45) {
  Start-Sleep -Seconds 2; $t++
  $helpers = @(Get-Process -ErrorAction SilentlyContinue | Where-Object {
    $st = $null; try { $st = $_.StartTime } catch {}
    $st -and $st -ge $t0 -and ("$($_.Path)" -like ($env:TEMP + '*') -or "$($_.Path)" -like ($tempLong + '*') -or $_.ProcessName -match '(?i)uninst|^au_$|^un_|^unins|^_iu')
  })
  if ($helpers.Count -gt 0) { $idle = 0 } else { $idle++ }
}
$removed = -not (Test-Path -LiteralPath $kp)
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
foreach ($id in @($A)) {
  $pks = @(Get-AppxPackage -Name ([string]$id) -ErrorAction SilentlyContinue)
  foreach ($pk in $pks) {
    try { Remove-AppxPackage -Package $pk.PackageFullName -ErrorAction Stop; Say ('Rimossa: ' + $pk.Name) } catch { Say ('Non rimossa: ' + $pk.Name) }
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
foreach ($p in @($A)) {
  $p = [string]$p
  if ($protect.ContainsKey($p.TrimEnd('\').ToLower())) { Say ('Cartella di sistema, la salto: ' + $p); continue }
  try {
    if (Test-Path -LiteralPath $p -PathType Leaf) {
      $len = (Get-Item -LiteralPath $p -Force).Length
      if ($len -gt (Get-BinLimit $p)) { $big++; Say ('Troppo grande per il Cestino (verrebbe cancellato per sempre), non lo tocco: ' + $p); continue }
      [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($p, 'OnlyErrorDialogs', 'SendToRecycleBin')
      $n++; $b += $len
    } elseif (Test-Path -LiteralPath $p -PathType Container) {
      $len = Get-FolderSize $p
      if ($len -gt (Get-BinLimit $p)) { $big++; Say ('Troppo grande per il Cestino (verrebbe cancellato per sempre), non la tocco: ' + $p); continue }
      [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteDirectory($p, 'OnlyErrorDialogs', 'SendToRecycleBin')
      $n++; $b += $len
    }
  } catch { Say ('Non spostato: ' + $p) }
}
Say ('Spostati nel Cestino: ' + $n + ' elementi (' + (Fmt $b) + '). Puoi sempre recuperarli dal Cestino.')
if ($big) { Say ('Non toccati perché troppo grandi per il Cestino: ' + $big + '. Se vuoi davvero toglierli, cancellali tu da Esplora file sapendo che non si recuperano.') }
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject ([ordered]@{ n = $n; bytes = $b }) -Compress))
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
  if (-not (Test-Path -LiteralPath $ps)) { Say ('Chiave gia'' rimossa: ' + $k); continue }
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
