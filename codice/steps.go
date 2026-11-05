package main

import "time"

type Step struct {
	ID      string
	Name    string
	Desc    string
	Level   int // nei sorgenti: 1 Normale, 2 Media, 3 Profonda, 4 Eccezionale (0 = solo strumento); all'avvio diventano 2-5 e nasce 1 Classica (steps_extra.go)
	Minutes float64
	Timeout time.Duration
	Script  string
}

func stepList() []map[string]interface{} {
	out := []map[string]interface{}{}
	for _, s := range allSteps {
		out = append(out, map[string]interface{}{"id": s.ID, "name": s.Name, "desc": s.Desc, "level": s.Level, "minutes": s.Minutes})
	}
	return out
}

const psPrelude = `
$ErrorActionPreference = 'SilentlyContinue'
$ProgressPreference = 'SilentlyContinue'
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
function Say([string]$m) { [Console]::Out.WriteLine('MSG ' + $m); [Console]::Out.Flush() }
function Freed([long]$b) { [Console]::Out.WriteLine('FREED ' + $b); [Console]::Out.Flush() }
function Fmt([double]$b) { if ($b -ge 1GB) { return ('{0:N2} GB' -f ($b / 1GB)) } elseif ($b -ge 1MB) { return ('{0:N1} MB' -f ($b / 1MB)) } else { return ('{0:N0} KB' -f ($b / 1KB)) } }
function Get-FolderSize([string]$p) {
  if (-not (Test-Path -LiteralPath $p)) { return [long]0 }
  $s = (Get-ChildItem -LiteralPath $p -Recurse -Force -File -ErrorAction SilentlyContinue | Measure-Object -Property Length -Sum).Sum
  if ($s) { return [long]$s } else { return [long]0 }
}
function Test-SafeTarget([string]$p) {
  if ([string]::IsNullOrWhiteSpace($p)) { return $false }
  try { $full = [IO.Path]::GetFullPath($p).TrimEnd('\') } catch { return $false }
  if ($full.Length -le 3) { return $false }  # mai una radice come C:\
  $deny = @($env:windir, $env:USERPROFILE, $env:ProgramFiles, ${env:ProgramFiles(x86)}, $env:ProgramData, $env:APPDATA, $env:LOCALAPPDATA) | Where-Object { $_ } | ForEach-Object { $_.TrimEnd('\') }
  foreach ($d in $deny) { if ($full -ieq $d) { return $false } }
  return $true
}
function Remove-Safe([string]$p) {
  # Non segue mai collegamenti/giunzioni: cancella solo il collegamento, mai i file a cui punta.
  $it = Get-Item -LiteralPath $p -Force -ErrorAction SilentlyContinue
  if (-not $it) { return }
  if ($it.Attributes -band [IO.FileAttributes]::ReparsePoint) {
    try { if ($it.PSIsContainer) { [IO.Directory]::Delete($it.FullName, $false) } else { [IO.File]::Delete($it.FullName) } } catch {}
    return
  }
  if ($it.PSIsContainer) {
    Get-ChildItem -LiteralPath $it.FullName -Force -ErrorAction SilentlyContinue | ForEach-Object { Remove-Safe $_.FullName }
    Remove-Item -LiteralPath $it.FullName -Force -ErrorAction SilentlyContinue
  } else {
    Remove-Item -LiteralPath $it.FullName -Force -ErrorAction SilentlyContinue
  }
}
function On-Battery {
  # Chiedo a Windows se il caricatore è attaccato (Offline = a batteria). Un PC fisso risponde Online.
  try { Add-Type -AssemblyName System.Windows.Forms -ErrorAction Stop; return ([string][System.Windows.Forms.SystemInformation]::PowerStatus.PowerLineStatus -eq 'Offline') } catch { return $false }
}
function Skip-AutoOnBattery {
  # Nella pulizia programmata i lavori lunghi non partono a batteria: meglio non scaricare il portatile.
  if ($TassoAuto -and (On-Battery)) { Say 'Il portatile va a batteria: questo lavoro lungo lo salto, lo faccio la prossima volta con il caricatore attaccato.'; return $true }
  return $false
}
function Remove-Old([string]$p, [datetime]$limit) {
  # Come Remove-Safe, ma dentro le cartelle toglie solo i file vecchi: una cartella con file nuovi resta.
  $it = Get-Item -LiteralPath $p -Force -ErrorAction SilentlyContinue
  if (-not $it) { return }
  if ($it.Attributes -band [IO.FileAttributes]::ReparsePoint) {
    if ($it.LastWriteTime -lt $limit) { try { if ($it.PSIsContainer) { [IO.Directory]::Delete($it.FullName, $false) } else { [IO.File]::Delete($it.FullName) } } catch {} }
    return
  }
  if ($it.PSIsContainer) {
    Get-ChildItem -LiteralPath $it.FullName -Force -ErrorAction SilentlyContinue | ForEach-Object { Remove-Old $_.FullName $limit }
    if (-not (Get-ChildItem -LiteralPath $it.FullName -Force -ErrorAction SilentlyContinue | Select-Object -First 1)) { Remove-Item -LiteralPath $it.FullName -Force -ErrorAction SilentlyContinue }
  } elseif ($it.LastWriteTime -lt $limit -and $it.CreationTime -lt $limit) {
    # Anche la data di creazione: un file appena estratto da uno zip può avere una data di modifica vecchia.
    Remove-Item -LiteralPath $it.FullName -Force -ErrorAction SilentlyContinue
  }
}
function Clear-Folder([string]$p, [double]$minAgeHours = 0) {
  if (-not (Test-SafeTarget $p)) { return [long]0 }
  if (-not (Test-Path -LiteralPath $p)) { return [long]0 }
  $b = Get-FolderSize $p
  $limit = (Get-Date).AddHours(-$minAgeHours)
  if ($minAgeHours -le 0) {
    Get-ChildItem -LiteralPath $p -Force -ErrorAction SilentlyContinue | ForEach-Object { Remove-Safe $_.FullName }
  } else {
    Get-ChildItem -LiteralPath $p -Force -ErrorAction SilentlyContinue | ForEach-Object { Remove-Old $_.FullName $limit }
  }
  $a = Get-FolderSize $p
  return [long][math]::Max(0, $b - $a)
}
function Run-Hidden([string]$file, [string]$argline) {
  # Nessuna finestra: processo senza console, output letto in background per non bloccarsi.
  try {
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = $file
    $psi.Arguments = $argline
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $psi.WindowStyle = [System.Diagnostics.ProcessWindowStyle]::Hidden
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $p = [System.Diagnostics.Process]::Start($psi)
    $o = $p.StandardOutput.ReadToEndAsync()
    $e = $p.StandardError.ReadToEndAsync()
    $p.WaitForExit()
    return $p.ExitCode
  } catch { return -1 }
}
function Get-DownloadsPath {
  try { $p = (New-Object -ComObject Shell.Application).Namespace('shell:Downloads').Self.Path; if ($p -and (Test-Path -LiteralPath $p)) { return $p } } catch {}
  return (Join-Path $env:USERPROFILE 'Downloads')
}
function Test-StartupOn([string]$key, [string]$name) {
  # StartupApproved: primo byte dispari = disattivato (come in Gestione attività).
  try {
    $v = (Get-ItemProperty -LiteralPath $key -Name $name -ErrorAction Stop).$name
    if ($v -and $v.Length -gt 0 -and (([int]$v[0]) -band 1) -eq 1) { return $false }
  } catch {}
  return $true
}
function Get-TassoStartup {
  # Le stesse fonti di Gestione attività > App di avvio: chiavi Run, cartelle Esecuzione automatica
  # e app dello Store con avvio automatico (ChatGPT, Teams, Xbox, Collegamento al telefono...).
  $out = New-Object System.Collections.ArrayList
  $src = @(
    @{ scope = 'hkcu';   run = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run';             ap = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run' },
    @{ scope = 'hklm';   run = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Run';             ap = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run' },
    @{ scope = 'hklm32'; run = 'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Run'; ap = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run32' }
  )
  foreach ($s in $src) {
    $k = Get-Item -LiteralPath $s.run -ErrorAction SilentlyContinue
    if (-not $k) { continue }
    foreach ($n in $k.GetValueNames()) {
      if (-not $n) { continue }
      $cmd = [string]$k.GetValue($n)
      if (-not $cmd.Trim()) { continue }
      [void]$out.Add([ordered]@{ scope = $s.scope; name = $n; key = $n; cmd = $cmd; enabled = (Test-StartupOn $s.ap $n) })
    }
  }
  $folders = @(
    @{ scope = 'fu'; p = [Environment]::GetFolderPath('Startup');       ap = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\StartupFolder' },
    @{ scope = 'fc'; p = [Environment]::GetFolderPath('CommonStartup'); ap = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\StartupFolder' }
  )
  foreach ($f in $folders) {
    if (-not $f.p -or -not (Test-Path -LiteralPath $f.p)) { continue }
    Get-ChildItem -LiteralPath $f.p -File -ErrorAction SilentlyContinue | Where-Object { $_.Name -ne 'desktop.ini' } | ForEach-Object {
      [void]$out.Add([ordered]@{ scope = $f.scope; name = $_.BaseName; key = $_.Name; cmd = $_.FullName; enabled = (Test-StartupOn $f.ap $_.Name) })
    }
  }
  # App dello Store: l'avvio automatico è dichiarato nel manifesto (StartupTask), lo stato è nel registro.
  $sad = 'HKCU:\Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\SystemAppData'
  $names = @{}
  try { foreach ($a in @(Get-StartApps)) { $id = [string]$a.AppID; if ($id -match '^([^!]+)!(.+)$' -and -not $names.ContainsKey($id)) { $names[$id] = [string]$a.Name } } } catch {}
  foreach ($p in @(Get-AppxPackage -ErrorAction SilentlyContinue)) {
    if ($p.IsFramework -or -not $p.InstallLocation) { continue }
    $man = Join-Path $p.InstallLocation 'AppxManifest.xml'
    try { $raw = [IO.File]::ReadAllText($man) } catch { continue }
    if ($raw.IndexOf('StartupTask', [StringComparison]::OrdinalIgnoreCase) -lt 0) { continue }
    try { $x = [xml]$raw } catch { continue }
    foreach ($t in @($x.SelectNodes("//*[local-name()='StartupTask']"))) {
      $tid = [string]$t.GetAttribute('TaskId')
      if (-not $tid) { continue }
      $app = $t; while ($app -and $app.LocalName -ne 'Application') { $app = $app.ParentNode }
      $appId = ''; if ($app) { $appId = [string]$app.GetAttribute('Id') }
      $disp = $names[$p.PackageFamilyName + '!' + $appId]
      if (-not $disp) { $dn = [string]$t.GetAttribute('DisplayName'); if ($dn -and $dn -notmatch '^ms-resource:') { $disp = $dn } }
      if (-not $disp) { $disp = ($p.Name -split '\.')[-1] }
      $on = ([string]$t.GetAttribute('Enabled') -eq 'true')
      try { $st = (Get-ItemProperty -LiteralPath (Join-Path (Join-Path $sad $p.PackageFamilyName) $tid) -Name State -ErrorAction Stop).State; $on = ($st -eq 2 -or $st -eq 4) } catch {}
      # Logo dell'app: il file vero ha un suffisso di grandezza (es. .targetsize-48.png o .scale-200.png).
      $logo = ''
      try {
        $ve = @($app.SelectNodes("*[local-name()='VisualElements']"))[0]
        $rel = [string]$ve.GetAttribute('Square44x44Logo'); if (-not $rel) { $rel = [string]$ve.GetAttribute('Square150x150Logo') }
        if ($rel) {
          $full = Join-Path $p.InstallLocation $rel
          if (Test-Path -LiteralPath $full) { $logo = $full } else {
            $dir = Split-Path $full -Parent; $bn = [IO.Path]::GetFileNameWithoutExtension($full)
            $c = @(Get-ChildItem -LiteralPath $dir -Filter ($bn + '*.png') -ErrorAction SilentlyContinue | Sort-Object { if ($_.Name -match 'targetsize-48(_|\.)') { 0 } elseif ($_.Name -match 'scale-200') { 1 } elseif ($_.Name -match 'targetsize-32') { 2 } else { 3 } }, Length)
            if ($c.Count) { $logo = $c[0].FullName }
          }
        }
      } catch {}
      [void]$out.Add([ordered]@{ scope = 'appx'; name = $disp; key = ($p.PackageFamilyName + '\' + $tid); cmd = ('App di Windows: ' + $p.Name); enabled = $on; icon = $logo })
    }
  }
  return $out.ToArray()
}
$BrowserRoots = @(
  @{ Label = 'Chrome';  Proc = 'chrome'; Root = (Join-Path $env:LOCALAPPDATA 'Google\Chrome\User Data') },
  @{ Label = 'Edge';    Proc = 'msedge';  Root = (Join-Path $env:LOCALAPPDATA 'Microsoft\Edge\User Data') },
  @{ Label = 'Brave';   Proc = 'brave';   Root = (Join-Path $env:LOCALAPPDATA 'BraveSoftware\Brave-Browser\User Data') },
  @{ Label = 'Opera';   Proc = 'opera';   Root = (Join-Path $env:APPDATA 'Opera Software') },
  @{ Label = 'Opera';   Proc = 'opera';   Root = (Join-Path $env:LOCALAPPDATA 'Opera Software') },
  @{ Label = 'Firefox'; Proc = 'firefox'; Root = (Join-Path $env:LOCALAPPDATA 'Mozilla\Firefox\Profiles') }
)
$GpuCaches = @(
  (Join-Path $env:LOCALAPPDATA 'D3DSCache'),
  (Join-Path $env:LOCALAPPDATA 'NVIDIA\DXCache'),
  (Join-Path $env:LOCALAPPDATA 'NVIDIA\GLCache'),
  (Join-Path $env:ProgramData 'NVIDIA Corporation\NV_Cache'),
  (Join-Path $env:LOCALAPPDATA 'AMD\DxCache'),
  (Join-Path $env:LOCALAPPDATA 'AMD\DxcCache'),
  (Join-Path $env:LOCALAPPDATA 'AMD\GLCache'),
  (Join-Path $env:LOCALAPPDATA 'AMD\VkCache'),
  (Join-Path $env:LOCALAPPDATA 'Intel\ShaderCache')
)
`

const windowsUpdateScript = `
try {
  $session = New-Object -ComObject Microsoft.Update.Session
  $searcher = $session.CreateUpdateSearcher()
  Say 'Cerco su Windows Update...'
  $res = $searcher.Search($Criteria)
  $coll = New-Object -ComObject Microsoft.Update.UpdateColl
  foreach ($u in $res.Updates) {
    if (-not $u.EulaAccepted) { $u.AcceptEula() }
    [void]$coll.Add($u)
    Say ('Trovato: ' + $u.Title)
  }
  if ($coll.Count -eq 0) { Say $NothingMsg; return }
  Say ('Scarico ' + $coll.Count + ' aggiornamenti...')
  $dl = $session.CreateUpdateDownloader(); $dl.Updates = $coll; [void]$dl.Download()
  Say 'Installo...'
  $inst = $session.CreateUpdateInstaller(); $inst.Updates = $coll
  $r = $inst.Install()
  $names = @{ 2 = 'installato'; 3 = 'installato con avvisi'; 4 = 'NON riuscito'; 5 = 'annullato' }
  for ($i = 0; $i -lt $coll.Count; $i++) {
    $c = [int]$r.GetUpdateResult($i).ResultCode
    $t = $names[$c]; if (-not $t) { $t = 'esito ' + $c }
    Say ($coll.Item($i).Title + ' -> ' + $t)
  }
  if ($r.RebootRequired) { Say 'Serve un riavvio per completare.'; [Console]::Out.WriteLine('REBOOT') }
} catch { Say ('Windows Update non ha risposto: ' + $_.Exception.Message) }
`

var allSteps = []Step{
	{ID: "backup", Name: "Backup (punto di ripristino)", Desc: "Crea un punto di ripristino: se qualcosa non va torni indietro.", Level: 2, Minutes: 1.5, Timeout: 10 * time.Minute, Script: `
Say 'Creo il backup di Windows (punto di ripristino)...'
try { Enable-ComputerRestore -Drive ($env:SystemDrive + '\') -ErrorAction Stop } catch {}
$key = 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\SystemRestore'
$oldFreq = (Get-ItemProperty -Path $key -Name 'SystemRestorePointCreationFrequency' -ErrorAction SilentlyContinue).SystemRestorePointCreationFrequency
New-ItemProperty -Path $key -Name 'SystemRestorePointCreationFrequency' -Value 0 -PropertyType DWord -Force | Out-Null
$before = @(Get-ComputerRestorePoint).Count
try { Checkpoint-Computer -Description 'Tasso - prima della manutenzione' -RestorePointType MODIFY_SETTINGS -ErrorAction Stop } catch { Say ('Backup non creato: ' + $_.Exception.Message) }
$after = @(Get-ComputerRestorePoint).Count
if ($null -ne $oldFreq) { New-ItemProperty -Path $key -Name 'SystemRestorePointCreationFrequency' -Value ([int]$oldFreq) -PropertyType DWord -Force | Out-Null } else { Remove-ItemProperty -Path $key -Name 'SystemRestorePointCreationFrequency' -ErrorAction SilentlyContinue }
if ($after -gt $before) { Say 'Backup creato. Se qualcosa non ti piace puoi tornare indietro da Ripristino configurazione di sistema.' }
elseif ($after -gt 0) {
  $last = @(Get-ComputerRestorePoint | Sort-Object SequenceNumber)[-1]
  $when = $null; try { $when = [Management.ManagementDateTimeConverter]::ToDateTime([string]$last.CreationTime) } catch {}
  if ($when -and $when -gt (Get-Date).AddDays(-1)) { Say ('Uso il backup di poco fa (' + $when.ToString('dd/MM HH:mm') + ').') }
  elseif ($when) { Say ('ATTENZIONE: non sono riuscito a fare un backup nuovo. L''ultimo è del ' + $when.ToString('dd/MM/yyyy') + ': se qualcosa va storto si torna a quel giorno.') }
  else { Say 'ATTENZIONE: non sono riuscito a fare un backup nuovo; c''è solo uno vecchio.' }
}
else { Say 'ATTENZIONE: il Ripristino configurazione di sistema sembra disattivato su questo PC.' }
`},
	{ID: "temp_user", Name: "File temporanei", Desc: "Svuota i file temporanei del tuo account più vecchi di un giorno (quelli in uso restano).", Level: 1, Minutes: 0.5, Timeout: 10 * time.Minute, Script: `
$f = Clear-Folder $env:TEMP 24
Freed $f; Say ('Liberati ' + (Fmt $f))
`},
	{ID: "temp_win", Name: "File temporanei di Windows", Desc: "Svuota C:\\Windows\\Temp (solo i file più vecchi di un giorno).", Level: 1, Minutes: 0.5, Timeout: 10 * time.Minute, Script: `
$f = Clear-Folder (Join-Path $env:windir 'Temp') 24
Freed $f; Say ('Liberati ' + (Fmt $f))
`},
	{ID: "recycle", Name: "Cestino", Desc: "Svuota il Cestino di tutte le unità. Nella pulizia programmata toglie solo quello che è nel Cestino da più di 7 giorni.", Level: 1, Minutes: 0.3, Timeout: 5 * time.Minute, Script: `
if ($TassoAuto) {
  # Pulizia programmata: nessuno sta guardando, quindi lascio le cose buttate di recente.
  $limit = (Get-Date).AddDays(-7); $f = [long]0; $n = 0; $kept = 0
  try {
    foreach ($it in @((New-Object -ComObject Shell.Application).Namespace(0xA).Items())) {
      $d = $null; try { $d = [datetime]$it.ExtendedProperty('System.Recycle.DateDeleted') } catch {}
      if (-not $d -or $d -gt $limit) { $kept++; continue }
      $p = [string]$it.Path; $len = [long]$it.Size
      if (-not $p -or -not (Test-Path -LiteralPath $p)) { continue }
      Remove-Safe $p
      if (-not (Test-Path -LiteralPath $p)) {
        $f += $len; $n++
        $info = Join-Path (Split-Path $p -Parent) ((Split-Path $p -Leaf) -replace '^\$R', '$I')
        Remove-Item -LiteralPath $info -Force -ErrorAction SilentlyContinue
      }
    }
  } catch {}
  Freed $f
  Say ('Cestino: tolti ' + $n + ' elementi buttati più di 7 giorni fa (' + (Fmt $f) + '). Quelli più recenti (' + $kept + ') restano, nel caso ti servano.')
  return
}
$size = [long]0
try { $s = ((New-Object -ComObject Shell.Application).Namespace(0xA).Items() | Measure-Object -Property Size -Sum).Sum; if ($s) { $size = [long]$s } } catch {}
Clear-RecycleBin -Force -ErrorAction SilentlyContinue
Freed $size; Say ('Cestino svuotato: ' + (Fmt $size))
`},
	{ID: "thumbs", Name: "Cache delle anteprime", Desc: "Elimina le miniature di Esplora file (si ricreano da sole).", Level: 1, Minutes: 0.2, Timeout: 5 * time.Minute, Script: `
$dir = Join-Path $env:LOCALAPPDATA 'Microsoft\Windows\Explorer'
$f = [long]0
Get-ChildItem -LiteralPath $dir -Filter 'thumbcache_*.db' -Force -ErrorAction SilentlyContinue | ForEach-Object {
  $len = $_.Length
  Remove-Item -LiteralPath $_.FullName -Force -ErrorAction SilentlyContinue
  if (-not (Test-Path -LiteralPath $_.FullName)) { $f += $len }
}
Freed $f; Say ('Liberati ' + (Fmt $f))
`},
	{ID: "browser", Name: "Cache dei browser", Desc: "Chrome, Edge, Brave, Opera, Firefox: solo la cache. Password e cookie restano.", Level: 1, Minutes: 0.8, Timeout: 15 * time.Minute, Script: `
foreach ($b in $BrowserRoots) {
  if (-not (Test-Path -LiteralPath $b.Root)) { continue }
  # Conta solo se ha una finestra aperta: Edge e Chrome spesso restano accesi in background senza finestre.
  if (@(Get-Process -Name $b.Proc -ErrorAction SilentlyContinue | Where-Object { $_.MainWindowHandle -ne 0 }).Count -gt 0) { Say ($b.Label + ' è aperto: lo salto. Chiudilo e ripeti per pulirlo.'); continue }
  $f = [long]0
  Get-ChildItem -LiteralPath $b.Root -Directory -Force -ErrorAction SilentlyContinue | ForEach-Object {
    foreach ($sub in @('Cache', 'Code Cache', 'GPUCache', 'cache2')) {
      $p = Join-Path $_.FullName $sub
      if (Test-Path -LiteralPath $p) { $f += Clear-Folder $p }
    }
  }
  Freed $f; Say ('Cache ' + $b.Label + ': ' + (Fmt $f))
}
`},
	{ID: "dns", Name: "Cache DNS", Desc: "Utile se qualche sito non si apre bene.", Level: 1, Minutes: 0.1, Timeout: 2 * time.Minute, Script: `
Clear-DnsClientCache
Say 'Cache DNS svuotata.'
`},
	{ID: "app_cache", Name: "Cache delle app", Desc: "Discord, Steam, Teams ed Epic Games: solo la cache, accessi e impostazioni restano.", Level: 2, Minutes: 0.5, Timeout: 10 * time.Minute, Script: `
$apps = @(
  @{ Label = 'Discord';    Proc = 'Discord';           Paths = @((Join-Path $env:APPDATA 'discord\Cache'), (Join-Path $env:APPDATA 'discord\Code Cache'), (Join-Path $env:APPDATA 'discord\GPUCache')) },
  @{ Label = 'Steam';      Proc = 'steamwebhelper';    Paths = @((Join-Path $env:LOCALAPPDATA 'Steam\htmlcache\Cache'), (Join-Path $env:LOCALAPPDATA 'Steam\htmlcache\Code Cache'), (Join-Path $env:LOCALAPPDATA 'Steam\htmlcache\GPUCache')) },
  @{ Label = 'Teams';      Proc = 'Teams';             Paths = @((Join-Path $env:APPDATA 'Microsoft\Teams\Cache'), (Join-Path $env:APPDATA 'Microsoft\Teams\Code Cache'), (Join-Path $env:APPDATA 'Microsoft\Teams\GPUCache')) },
  @{ Label = 'Epic Games'; Proc = 'EpicGamesLauncher'; Paths = @((Join-Path $env:LOCALAPPDATA 'EpicGamesLauncher\Saved\webcache'), (Join-Path $env:LOCALAPPDATA 'EpicGamesLauncher\Saved\webcache_4147'), (Join-Path $env:LOCALAPPDATA 'EpicGamesLauncher\Saved\webcache_4430')) }
)
$any = $false
foreach ($a in $apps) {
  $exist = @($a.Paths | Where-Object { Test-Path -LiteralPath $_ })
  if ($exist.Count -eq 0) { continue }
  $any = $true
  if (Get-Process -Name $a.Proc -ErrorAction SilentlyContinue) { Say ($a.Label + ' è aperto: lo salto.'); continue }
  $f = [long]0
  foreach ($p in $exist) { $f += Clear-Folder $p }
  Freed $f; Say ('Cache ' + $a.Label + ': ' + (Fmt $f))
}
if (-not $any) { Say 'Nessuna di queste app trovata.' }
`},
	{ID: "gpu_cache", Name: "Cache della scheda video", Desc: "Shader di DirectX, NVIDIA, AMD e Intel. Usala solo se i giochi danno errori grafici dopo un aggiornamento del driver: dopo, le prime partite scattano un po' finché le cache si rifanno.", Level: 0, Minutes: 0.5, Timeout: 10 * time.Minute, Script: `
$f = [long]0
foreach ($p in $GpuCaches) { if (Test-Path -LiteralPath $p) { $f += Clear-Folder $p } }
Freed $f; Say ('Liberati ' + (Fmt $f))
`},
	{ID: "wu_cache", Name: "Cache di Windows Update", Desc: "File degli aggiornamenti già scaricati. Se ne serve ancora uno, Windows lo riscarica da solo.", Level: 2, Minutes: 1, Timeout: 15 * time.Minute, Script: `
if (Get-Process -Name TiWorker -ErrorAction SilentlyContinue) { Say 'Windows sta installando degli aggiornamenti proprio adesso: salto, per non disturbarlo. Rifallo più tardi.'; return }
Stop-Service -Name wuauserv, bits -Force -ErrorAction SilentlyContinue
$f = Clear-Folder (Join-Path $env:windir 'SoftwareDistribution\Download')
Start-Service -Name wuauserv, bits -ErrorAction SilentlyContinue
Freed $f; Say ('Liberati ' + (Fmt $f))
`},
	{ID: "cleanmgr", Name: "Pulizia file di sistema", Desc: "Rapporti errori, file di dump, log vecchi e cache di Ottimizzazione recapito. Senza finestre.", Level: 2, Minutes: 1, Timeout: 20 * time.Minute, Script: `
$f = [long]0
$pd = $env:ProgramData
foreach ($d in @((Join-Path $pd 'Microsoft\Windows\WER\ReportArchive'), (Join-Path $pd 'Microsoft\Windows\WER\ReportQueue'), (Join-Path $pd 'Microsoft\Windows\WER\Temp'), (Join-Path $env:LOCALAPPDATA 'Microsoft\Windows\WER'), (Join-Path $env:LOCALAPPDATA 'CrashDumps'))) {
  if (Test-Path -LiteralPath $d) { $f += Clear-Folder $d }
}
# I rapporti delle schermate blu degli ultimi 14 giorni restano: servono per capire cosa non va.
$md = Join-Path $env:windir 'Minidump'
if (Test-Path -LiteralPath $md) { $f += Clear-Folder $md 336 }
$dmp = Join-Path $env:windir 'MEMORY.DMP'
if ((Test-Path -LiteralPath $dmp) -and (Get-Item -LiteralPath $dmp -Force).LastWriteTime -lt (Get-Date).AddDays(-14)) { $len = (Get-Item -LiteralPath $dmp -Force).Length; Remove-Item -LiteralPath $dmp -Force -ErrorAction SilentlyContinue; if (-not (Test-Path -LiteralPath $dmp)) { $f += $len } }
$old = (Get-Date).AddDays(-14)
foreach ($d in @((Join-Path $env:windir 'Logs\CBS'), (Join-Path $env:windir 'Logs\DISM'), (Join-Path $env:windir 'Logs\WindowsUpdate'), (Join-Path $env:windir 'Panther'))) {
  if (-not (Test-Path -LiteralPath $d)) { continue }
  Get-ChildItem -LiteralPath $d -File -Force -ErrorAction SilentlyContinue | Where-Object { $_.LastWriteTime -lt $old -and $_.Extension -in @('.log','.cab','.etl','.txt') } | ForEach-Object {
    $len = $_.Length; Remove-Item -LiteralPath $_.FullName -Force -ErrorAction SilentlyContinue; if (-not (Test-Path -LiteralPath $_.FullName)) { $f += $len }
  }
}
try {
  $drive = Get-PSDrive -Name ($env:SystemDrive.TrimEnd(':'))
  $before = [long]$drive.Free
  Delete-DeliveryOptimizationCache -Force -ErrorAction Stop | Out-Null
  $after = [long](Get-PSDrive -Name ($env:SystemDrive.TrimEnd(':'))).Free
  $f += [math]::Max(0, $after - $before)
} catch {}
Freed $f; Say ('Liberati ' + (Fmt $f))
`},
	{ID: "downloads", Name: "Riordino della cartella Download", Desc: "Mette i file sciolti in Immagini, Documenti, Video, Musica, Programmi, Archivi.", Level: 0, Minutes: 0.5, Timeout: 10 * time.Minute, Script: `
$dl = Get-DownloadsPath
$cats = [ordered]@{
  'Immagini'  = @('.jpg','.jpeg','.png','.gif','.bmp','.webp','.heic','.tif','.tiff','.svg')
  'Documenti' = @('.pdf','.doc','.docx','.odt','.txt','.rtf','.xls','.xlsx','.ods','.csv','.ppt','.pptx','.odp')
  'Video'     = @('.mp4','.mkv','.avi','.mov','.wmv','.webm','.m4v')
  'Musica'    = @('.mp3','.wav','.flac','.aac','.m4a','.ogg','.wma')
  'Programmi' = @('.exe','.msi','.msix','.appx','.msixbundle')
  'Archivi'   = @('.zip','.rar','.7z','.tar','.gz','.iso')
}
$limit = (Get-Date).AddMinutes(-15)
$moved = 0
Get-ChildItem -LiteralPath $dl -File -ErrorAction SilentlyContinue | ForEach-Object {
  if ($_.LastWriteTime -gt $limit) { return }
  if ($_.Name -like 'Tasso*.exe') { return }
  $ext = $_.Extension.ToLower()
  foreach ($c in $cats.Keys) {
    if ($cats[$c] -contains $ext) {
      $destDir = Join-Path $dl $c
      New-Item -ItemType Directory -Path $destDir -Force | Out-Null
      $target = Join-Path $destDir $_.Name
      $n = 2
      while (Test-Path -LiteralPath $target) { $target = Join-Path $destDir ($_.BaseName + ' (' + $n + ')' + $_.Extension); $n++ }
      try { Move-Item -LiteralPath $_.FullName -Destination $target -ErrorAction Stop; $moved++ } catch {}
      break
    }
  }
}
Say ('Riordinati ' + $moved + ' file nella cartella Download.')
`},
	{ID: "apps", Name: "Aggiornamento programmi", Desc: "Aggiorna i programmi installati con winget. Attenzione: alcune app (Discord, Claude, Proton...) dopo l'aggiornamento chiedono di rifare l'accesso.", Level: 0, Minutes: 6, Timeout: 60 * time.Minute, Script: `
$wg = Get-Command winget.exe -ErrorAction SilentlyContinue
if (-not $wg) { Say 'winget non è disponibile su questo PC: salto.'; return }
Say 'Aggiorno i programmi (alcuni minuti)...'
$code = Run-Hidden $wg.Source 'upgrade --all --silent --accept-source-agreements --accept-package-agreements --disable-interactivity'
if ($code -eq 0) { Say 'Programmi aggiornati.' } else { Say ('Fatto, con qualche programma da aggiornare a mano (codice ' + $code + ').') }
`},
	{ID: "drivers", Name: "Aggiornamento driver", Desc: "Driver certificati da Microsoft tramite Windows Update.", Level: 3, Minutes: 6, Timeout: 90 * time.Minute, Script: `
$Criteria = "IsInstalled=0 and Type='Driver' and IsHidden=0 and BrowseOnly=0"
$NothingMsg = 'I driver risultano già aggiornati.'
` + windowsUpdateScript},
	{ID: "winupdate", Name: "Aggiornamenti di Windows", Desc: "Installa gli aggiornamenti di sicurezza e di sistema.", Level: 3, Minutes: 12, Timeout: 120 * time.Minute, Script: `
$Criteria = "IsInstalled=0 and Type='Software' and IsHidden=0 and BrowseOnly=0"
$NothingMsg = 'Windows è già aggiornato.'
` + windowsUpdateScript},
	{ID: "startup", Name: "Programmi all'avvio", Desc: "Ti mostra quali programmi partono all'accensione e rallentano il PC.", Level: 3, Minutes: 0.2, Timeout: 3 * time.Minute, Script: `
$all = @(Get-TassoStartup)
$on = @($all | Where-Object { $_.enabled })
$off = @($all | Where-Object { -not $_.enabled })
Say ('Programmi che partono all''accensione: ' + $on.Count + ' (altri ' + $off.Count + ' sono già spenti)')
foreach ($s in $on) { Say (' - ' + $s.name) }
if ($on.Count -gt 5) { Say 'Consiglio: Tasso > Velocità > Programmi all''avvio > spegni quelli che non usi.' }
`},
	{ID: "disk_health", Name: "Salute dei dischi", Desc: "Controlla stato, usura e temperatura di SSD e dischi.", Level: 3, Minutes: 0.3, Timeout: 5 * time.Minute, Script: `
Get-PhysicalDisk | ForEach-Object {
  $r = $_ | Get-StorageReliabilityCounter
  $t = ''; if ($r.Temperature) { $t = ', ' + $r.Temperature + ' gradi' }
  $w = ''; if ($r.Wear -ne $null) { $w = ', usura ' + $r.Wear + '%' }
  $h = [string]$_.HealthStatus
  if ($h -eq 'Healthy') { $h = 'in salute' } elseif ($h -eq 'Warning') { $h = 'ATTENZIONE, da tenere d''occhio' } elseif ($h -eq 'Unhealthy') { $h = 'A RISCHIO: fai subito un backup dei tuoi file' }
  Say ($_.FriendlyName + ' (' + $_.MediaType + '): ' + $h + $w + $t)
}
`},
	{ID: "dism_cleanup", Name: "Pulizia componenti di Windows", Desc: "Rimuove le versioni vecchie dei componenti di sistema.", Level: 3, Minutes: 10, Timeout: 90 * time.Minute, Script: `
if (Skip-AutoOnBattery) { return }
$drive = Get-PSDrive -Name ($env:SystemDrive.TrimEnd(':'))
$before = [long]$drive.Free
if (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending') { Say 'Windows sta aspettando un riavvio per finire un aggiornamento: adesso questo passaggio non funzionerebbe bene. Riavvia il PC e poi rifallo.'; [Console]::Out.WriteLine('REBOOT'); return }
Say 'Pulizia componenti in corso (5-20 minuti)...'
$code = Run-Hidden 'dism.exe' '/Online /Cleanup-Image /StartComponentCleanup'
$after = [long](Get-PSDrive -Name ($env:SystemDrive.TrimEnd(':'))).Free
$f = [math]::Max(0, $after - $before)
Freed $f; Say ('Fatto (codice ' + $code + '), liberati ' + (Fmt $f))
`},
	{ID: "devices", Name: "Controllo periferiche", Desc: "Verifica che audio, rete, video e USB funzionino.", Level: 4, Minutes: 0.2, Timeout: 3 * time.Minute, Script: `
$bad = @(Get-CimInstance Win32_PnPEntity -ErrorAction SilentlyContinue | Where-Object { $_.ConfigManagerErrorCode -ne 0 -and $_.ConfigManagerErrorCode -ne 22 -and $_.ConfigManagerErrorCode -ne 45 })
if ($bad.Count -eq 0) { Say 'Tutte le periferiche funzionano correttamente.' }
else {
  Say ('ATTENZIONE: ' + $bad.Count + ' periferiche con problemi:')
  foreach ($d in $bad) { Say (' - ' + $d.Name + ' (codice ' + $d.ConfigManagerErrorCode + ')') }
  Say 'Per queste conviene scaricare il driver dal sito del produttore.'
}
`},
	{ID: "programs", Name: "Controllo di ogni programma", Desc: "Ripara le app di Windows danneggiate e trova i programmi rotti.", Level: 4, Minutes: 3, Timeout: 30 * time.Minute, Script: `
$badApps = @(Get-AppxPackage -AllUsers -ErrorAction SilentlyContinue | Where-Object { [string]$_.Status -ne 'Ok' })
if ($badApps.Count -eq 0) { Say 'App di Windows: tutte a posto.' }
foreach ($p in $badApps) {
  Say ('Riparo: ' + $p.Name)
  try { Add-AppxPackage -DisableDevelopmentMode -Register (Join-Path $p.InstallLocation 'AppxManifest.xml') -ErrorAction Stop; Say '   riparata.' } catch { Say '   non riuscita, prova a reinstallarla dal Microsoft Store.' }
}
$keys = @('HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*', 'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*', 'HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*')
$progs = @(Get-ItemProperty $keys -ErrorAction SilentlyContinue | Where-Object { $_.DisplayName -and -not $_.SystemComponent })
$broken = @()
foreach ($p in $progs) {
  if ($p.InstallLocation -and -not (Test-Path -LiteralPath $p.InstallLocation)) { $broken += $p.DisplayName }
}
Say ('Programmi installati: ' + $progs.Count)
if ($broken.Count -gt 0) {
  Say ('Programmi che risultano installati ma la loro cartella non esiste più (' + $broken.Count + '):')
  foreach ($b in $broken) { Say (' - ' + $b) }
  Say 'Puoi toglierli da Impostazioni > App > App installate.'
} else { Say 'Nessun programma rotto trovato.' }
`},
	{ID: "repair", Name: "Riparazione di Windows", Desc: "DISM RestoreHealth + SFC: ripara i file di sistema danneggiati.", Level: 4, Minutes: 25, Timeout: 120 * time.Minute, Script: `
if (Skip-AutoOnBattery) { return }
if (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending') { Say 'Windows sta aspettando un riavvio per finire un aggiornamento: adesso questo passaggio non funzionerebbe bene. Riavvia il PC e poi rifallo.'; [Console]::Out.WriteLine('REBOOT'); return }
Say 'Controllo e riparo l''immagine di Windows (DISM, 10-30 minuti)...'
$c1 = Run-Hidden 'dism.exe' '/Online /Cleanup-Image /RestoreHealth'
$needReboot = $false
if ($c1 -eq 0) { Say 'Immagine di Windows a posto.' } elseif ($c1 -eq 3010) { Say 'Immagine di Windows riparata: serve un riavvio per finire.'; $needReboot = $true } else { Say ('DISM non è riuscito a finire (codice ' + $c1 + '). Spesso succede senza internet: riprova con la connessione attiva.') }
Say 'Controllo i file di sistema (SFC, 10-20 minuti)...'
$t0 = Get-Date
$c2 = Run-Hidden 'sfc.exe' '/scannow'
# SFC non dice il risultato con il codice: lo leggo dal suo registro (solo le righe scritte adesso).
$bad = 0; $fixed = 0; $seen = @{}
try {
  foreach ($l in @(Get-Content -LiteralPath (Join-Path $env:windir 'Logs\CBS\CBS.log') -Tail 40000 -ErrorAction Stop)) {
    if ($l -notmatch '\[SR\]') { continue }
    if ($l -match '^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})') { try { if ([datetime]::ParseExact($Matches[1], 'yyyy-MM-dd HH:mm:ss', [Globalization.CultureInfo]::InvariantCulture) -lt $t0.AddMinutes(-1)) { continue } } catch {} }
    # Lo stesso file compare in più righe: lo conto una volta sola.
    $key = ($l -replace '^.*?\[SR\]\s*', '') -replace '\s+', ' '
    if ($l -match 'Cannot repair') { $m2 = [regex]::Match($key, '\[l:\d+\]"?[^"\]]+').Value; if (-not $m2) { $m2 = $key }; $k2 = 'b|' + $m2; if (-not $seen.ContainsKey($k2)) { $seen[$k2] = 1; $bad++ } }
    elseif ($l -match 'Repairing corrupted file|Repaired file') { $m2 = [regex]::Match($key, '\[l:\d+\]"?[^"\]]+').Value; if (-not $m2) { $m2 = $key }; $k2 = 'f|' + $m2; if (-not $seen.ContainsKey($k2)) { $seen[$k2] = 1; $fixed++ } }
  }
} catch {}
if ($bad -gt 0) { Say ('SFC ha trovato file rovinati che non è riuscito a riparare (' + $bad + '). Riavvia e rifai la riparazione; se resta così, conviene Ripristina Windows mantenendo i file.') }
elseif ($fixed -gt 0) { Say ('SFC ha trovato e riparato ' + $fixed + ' file di sistema. Riavvia il PC per finire.') }
else { Say 'File di sistema: nessun problema trovato.' }
if ($needReboot -or $bad -gt 0 -or $fixed -gt 0) { [Console]::Out.WriteLine('REBOOT') }
`},
	{ID: "chkdsk", Name: "Controllo errori del disco", Desc: "Controlla il disco di sistema mentre usi il PC. Se trova errori gravi te lo dice; la riparazione al riavvio la prenota solo se avvii questo controllo da solo.", Level: 4, Minutes: 4, Timeout: 60 * time.Minute, Script: `
if (Skip-AutoOnBattery) { return }
Say ('Controllo il disco ' + $env:SystemDrive + '...')
$dl = $env:SystemDrive.TrimEnd(':')
$r = $null
try { $r = Repair-Volume -DriveLetter $dl -Scan -ErrorAction Stop } catch { Say 'Il controllo del disco non è riuscito adesso (forse Windows sta già controllando il disco). Riprova più tardi.'; return }
switch ([string]$r) {
  'NoErrorsFound' { Say 'Nessun errore sul disco.' }
  'SpotFixNeeded' {
    Say 'Trovati piccoli errori: li sistemo subito (pochi secondi, senza riavviare)...'
    try { $f = Repair-Volume -DriveLetter $dl -SpotFix -ErrorAction Stop; if ([string]$f -eq 'NoErrorsFound') { Say 'Errori del disco sistemati.' } else { Say 'Errori segnati: Windows li sistema al prossimo riavvio.'; [Console]::Out.WriteLine('REBOOT') } } catch { Say 'Errori segnati: Windows li sistema al prossimo riavvio.'; [Console]::Out.WriteLine('REBOOT') }
  }
  'FullRepairNeeded' {
    Say 'ATTENZIONE: il disco ha errori che si riparano solo al riavvio. Prima di tutto fai una copia dei tuoi file importanti (chiavetta, disco esterno o OneDrive).'
    if ($TassoAuto -or -not $TassoSingle) { Say 'Non prenoto la riparazione dentro una pulizia: prima fai il backup, poi avvia da solo "Controllo errori del disco".'; return }
    $sched = $false; try { [void](Repair-Volume -DriveLetter $dl -OfflineScanAndFix -ErrorAction Stop); $sched = $true } catch {}
    if ($sched) { Say ('Ho prenotato la riparazione: Windows la fa al prossimo riavvio (può metterci qualche minuto, non spegnere il PC). Se vuoi annullarla per fare prima il backup, nel Prompt dei comandi come amministratore scrivi: chkntfs /x ' + $env:SystemDrive); [Console]::Out.WriteLine('REBOOT') }
    else { Say ('Non sono riuscito a prenotare la riparazione. Dopo il backup, apri il Prompt dei comandi come amministratore e scrivi: chkdsk ' + $dl + ': /f') }
  }
  default { Say ('Controllo finito (risultato: ' + $r + ').') }
}
`},
	{ID: "optimize", Name: "Deframmentazione / TRIM", Desc: "Deframmenta i dischi meccanici, ottimizza gli SSD.", Level: 3, Minutes: 5, Timeout: 120 * time.Minute, Script: `
if (Skip-AutoOnBattery) { return }
Say 'Ottimizzo i dischi (sui dischi meccanici può volerci un po'')...'
# Solo i dischi interni fissi (niente chiavette, dischi esterni o di rete).
$vols = @(Get-Volume -ErrorAction SilentlyContinue | Where-Object { $_.DriveType -eq 'Fixed' -and $_.DriveLetter -and $_.FileSystem -match 'NTFS|ReFS' })
$usb = @{}
try { foreach ($d in @(Get-Disk -ErrorAction Stop | Where-Object { [string]$_.BusType -eq 'USB' })) { foreach ($pp in @(Get-Partition -DiskNumber $d.Number -ErrorAction SilentlyContinue)) { if ($pp.DriveLetter) { $usb[[string]$pp.DriveLetter] = $true } } } } catch {}
$n = 0
foreach ($v in $vols) {
  if ($usb.ContainsKey([string]$v.DriveLetter)) { continue }
  $code = Run-Hidden 'defrag.exe' ([string]$v.DriveLetter + ': /O')
  $n++
  Say ('Disco ' + $v.DriveLetter + ': ottimizzato (codice ' + $code + ').')
}
if ($n -eq 0) { Say 'Nessun disco interno da ottimizzare.' }
`},
	{ID: "defender", Name: "Scansione antivirus", Desc: "Scansione rapida con Microsoft Defender.", Level: 3, Minutes: 4, Timeout: 60 * time.Minute, Script: `
$mp = Join-Path $env:ProgramFiles 'Windows Defender\MpCmdRun.exe'
if (-not (Test-Path -LiteralPath $mp)) { Say 'Microsoft Defender non trovato: salto.'; return }
Say 'Scansione rapida in corso...'
$code = Run-Hidden $mp '-Scan -ScanType 1'
if ($code -eq 0) { Say 'Nessuna minaccia trovata.' } elseif ($code -eq 2) { Say 'ATTENZIONE: trovate minacce. Apri Sicurezza di Windows per i dettagli.' } else { Say 'Scansione terminata. Se usi un altro antivirus, Defender potrebbe essere spento.' }
`},
	{ID: "browser_ext", Name: "Controllo estensioni dei browser", Desc: "Elenca le estensioni di Chrome, Edge e Brave: se non ne riconosci una, toglila.", Level: 3, Minutes: 0.3, Timeout: 5 * time.Minute, Script: `
function Get-ExtName([string]$dir) {
  $man = Join-Path $dir 'manifest.json'
  if (-not (Test-Path -LiteralPath $man)) { return $null }
  try { $j = Get-Content -Raw -Encoding UTF8 -LiteralPath $man | ConvertFrom-Json } catch { return $null }
  $n = [string]$j.name
  if ($n -match '^__MSG_(.+)__$') {
    $key = $Matches[1]
    foreach ($loc in @('it', 'en', 'en_US', [string]$j.default_locale)) {
      if (-not $loc) { continue }
      $mf = Join-Path $dir ('_locales\' + $loc + '\messages.json')
      if (Test-Path -LiteralPath $mf) {
        try { $m = Get-Content -Raw -Encoding UTF8 -LiteralPath $mf | ConvertFrom-Json; foreach ($pr in $m.PSObject.Properties) { if ($pr.Name -ieq $key) { return [string]$pr.Value.message } } } catch {}
      }
    }
  }
  return $n
}
$total = 0
foreach ($b in $BrowserRoots) {
  if ($b.Label -eq 'Firefox' -or $b.Label -eq 'Opera') { continue }
  if (-not (Test-Path -LiteralPath $b.Root)) { continue }
  $names = New-Object System.Collections.ArrayList
  Get-ChildItem -LiteralPath $b.Root -Directory -Force -ErrorAction SilentlyContinue | ForEach-Object {
    $ext = Join-Path $_.FullName 'Extensions'
    if (-not (Test-Path -LiteralPath $ext)) { return }
    Get-ChildItem -LiteralPath $ext -Directory -ErrorAction SilentlyContinue | ForEach-Object {
      $ver = Get-ChildItem -LiteralPath $_.FullName -Directory -ErrorAction SilentlyContinue | Sort-Object Name -Descending | Select-Object -First 1
      if ($ver) { $nm = Get-ExtName $ver.FullName; if ($nm -and $nm -notmatch '^(Chrome|Microsoft Edge|Edge) ' -and -not $names.Contains($nm)) { [void]$names.Add($nm) } }
    }
  }
  if ($names.Count -gt 0) {
    Say ($b.Label + ': ' + $names.Count + ' estensioni')
    foreach ($n in $names) { Say (' - ' + $n) }
    $total += $names.Count
  }
}
if ($total -eq 0) { Say 'Nessuna estensione trovata.' } else { Say 'Se una di queste non l''hai messa tu, toglila dal menu Estensioni del browser.' }
`},
	{ID: "turbo", Name: "Turbo: libera il PC per giocare", Desc: "Chiude i programmi in background che non servono mentre giochi (OneDrive, Teams, Skype...). Si riaprono al prossimo avvio.", Level: 0, Minutes: 0.2, Timeout: 3 * time.Minute, Script: `
$names = @('OneDrive', 'Teams', 'ms-teams', 'Skype', 'PhoneExperienceHost', 'YourPhone', 'Widgets', 'GoogleDriveFS', 'Dropbox', 'CCXProcess', 'Creative Cloud', 'AdobeCollabSync', 'Cortana')
$freed = [long]0; $closed = New-Object System.Collections.ArrayList
$mySession = (Get-Process -Id $PID).SessionId
foreach ($n in $names) {
  # Solo i programmi del tuo account (non quelli di altri utenti collegati allo stesso PC).
  $ps = @(Get-Process -Name $n -ErrorAction SilentlyContinue | Where-Object { $_.SessionId -eq $mySession })
  if ($ps.Count -eq 0) { continue }
  $mem = ($ps | Measure-Object -Property PrivateMemorySize64 -Sum).Sum
  $ps | Stop-Process -Force -ErrorAction SilentlyContinue
  $freed += [long]$mem
  [void]$closed.Add($n)
}
if ($closed.Count -eq 0) { Say 'Nessun programma inutile in background: il PC è già libero.' }
else { Say ('Chiusi: ' + ($closed -join ', ')); Say ('Memoria liberata: ' + (Fmt $freed)) }
`},
	{ID: "fix_audio", Name: "Risolvi problemi audio", Desc: "Riavvia i servizi audio di Windows e controlla le schede audio.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: `
Say 'Riavvio i servizi audio...'
Restart-Service -Name AudioEndpointBuilder -Force -ErrorAction SilentlyContinue
Restart-Service -Name Audiosrv -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 2
$s = Get-Service -Name Audiosrv -ErrorAction SilentlyContinue
if ($s -and $s.Status -eq 'Running') { Say 'Servizio audio di Windows: attivo.' } else { Say 'ATTENZIONE: il servizio audio non è partito. Prova a riavviare il PC.' }
$dev = @(Get-CimInstance Win32_SoundDevice -ErrorAction SilentlyContinue)
foreach ($d in $dev) { $st = 'ok'; if ($d.ConfigManagerErrorCode -ne 0) { $st = 'PROBLEMA (codice ' + $d.ConfigManagerErrorCode + '): reinstalla il driver audio' }; Say ($d.Name + ': ' + $st) }
if ($dev.Count -eq 0) { Say 'Nessuna scheda audio trovata: controlla che casse o cuffie siano collegate.' }
Say 'Se ancora non senti nulla: clic sull''icona del volume e controlla quale uscita è scelta.'
`},
	{ID: "fix_net", Name: "Risolvi problemi di internet", Desc: "Svuota la cache, rinnova la connessione e ripristina le impostazioni di rete. Internet si stacca per qualche secondo.", Level: 0, Minutes: 0.5, Timeout: 5 * time.Minute, Script: `
Say 'Svuoto la cache DNS...'
Clear-DnsClientCache
Say 'Rinnovo l''indirizzo di rete (internet si stacca per qualche secondo)...'
[void](Run-Hidden 'ipconfig.exe' '/release')
[void](Run-Hidden 'ipconfig.exe' '/renew')
Say 'Ripristino le impostazioni di rete di Windows...'
[void](Run-Hidden 'netsh.exe' 'winsock reset')
# "int ip reset" rimette tutte le schede in automatico: cancellerebbe un indirizzo IP fisso o dei DNS
# scelti a mano (tipico nei PC d'ufficio). In quel caso lo salto.
$manual = $false
try {
  foreach ($ad in @(Get-NetAdapter -Physical -ErrorAction Stop)) {
    $ip = Get-NetIPInterface -InterfaceIndex $ad.ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue
    if ($ip -and $ip.Dhcp -eq 'Disabled') { $manual = $true }
    $dns = Get-ItemProperty -LiteralPath ('HKLM:\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\' + $ad.InterfaceGuid) -ErrorAction SilentlyContinue
    if ($dns -and [string]$dns.NameServer) { $manual = $true }
  }
} catch { $manual = $true }
if ($manual) { Say 'Il PC ha un indirizzo IP o dei DNS scelti a mano: li lascio come sono (non faccio il ripristino completo della rete).' }
else { [void](Run-Hidden 'netsh.exe' 'int ip reset') }
Start-Sleep -Seconds 3
$ok = Test-Connection -ComputerName 1.1.1.1 -Count 1 -Quiet -ErrorAction SilentlyContinue
if ($ok) { Say 'Connessione a internet: funziona.' } else { Say 'Internet non risponde ancora: riavvia il PC e, se serve, anche il modem.' }
Say 'Per completare il ripristino serve un riavvio.'
[Console]::Out.WriteLine('REBOOT')
`},
	{ID: "fix_print", Name: "Risolvi problemi di stampa", Desc: "Riavvia il servizio di stampa (spooler) e svuota la coda bloccata. Utile quando la stampante non stampa.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: `
Say 'Fermo il servizio di stampa...'
Stop-Service -Name Spooler -Force -ErrorAction SilentlyContinue
$q = Join-Path $env:windir 'System32\spool\PRINTERS'
if (Test-Path -LiteralPath $q) { Get-ChildItem -LiteralPath $q -Force -ErrorAction SilentlyContinue | ForEach-Object { Remove-Item -LiteralPath $_.FullName -Force -ErrorAction SilentlyContinue }; Say 'Coda di stampa svuotata.' }
Start-Service -Name Spooler -ErrorAction SilentlyContinue
Say 'Servizio di stampa riavviato. Riprova a stampare.'
`},
	{ID: "fix_store", Name: "Risolvi problemi del Microsoft Store e delle app", Desc: "Svuota la cache del Microsoft Store e ri-registra le app di Windows quando non si aprono o non si aggiornano.", Level: 0, Minutes: 1, Timeout: 10 * time.Minute, Script: `
Say 'Svuoto la cache del Microsoft Store...'
try { Start-Process -FilePath 'wsreset.exe' -WindowStyle Hidden -ErrorAction Stop } catch {}
Start-Sleep -Seconds 3
Get-Process -Name WinStore.App -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Say 'Ricontrollo le app di sistema (può volerci un minuto)...'
Get-AppxPackage -AllUsers Microsoft.WindowsStore | ForEach-Object { Add-AppxPackage -DisableDevelopmentMode -Register ($_.InstallLocation + '\AppXManifest.xml') -ErrorAction SilentlyContinue }
Say 'Fatto. Se un app non si apriva ancora, prova a riavviare il PC.'
`},
	{ID: "fix_search", Name: "Risolvi problemi di ricerca e menu Start", Desc: "Riavvia la ricerca di Windows e l'interfaccia del menu Start quando non rispondono o non trovano niente.", Level: 0, Minutes: 0.4, Timeout: 5 * time.Minute, Script: `
Say 'Riavvio la ricerca di Windows...'
Restart-Service -Name WSearch -Force -ErrorAction SilentlyContinue
Say 'Riavvio il menu Start e la barra...'
Get-Process -Name StartMenuExperienceHost -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Get-Process -Name SearchHost,SearchApp -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Say 'Fatto: ricerca e menu Start riavviati (si ricaricano da soli in qualche secondo).'
`},
	{ID: "fix_wu", Name: "Risolvi problemi di Windows Update", Desc: "Ripristina i componenti di Windows Update quando gli aggiornamenti si bloccano.", Level: 0, Minutes: 1, Timeout: 10 * time.Minute, Script: `
$svcs = @('wuauserv', 'bits', 'cryptsvc', 'msiserver')
Say 'Fermo i servizi di Windows Update...'
foreach ($s in $svcs) { Stop-Service -Name $s -Force -ErrorAction SilentlyContinue }
$sd = Join-Path $env:windir 'SoftwareDistribution'
$cr = Join-Path $env:windir 'System32\catroot2'
$wuFail = $false
foreach ($p in @($sd, $cr)) {
  $old = $p + '.old'
  if (Test-Path -LiteralPath $old) { Remove-Item -LiteralPath $old -Recurse -Force -ErrorAction SilentlyContinue }
  # Windows a volte rifà partire i servizi da solo: riprovo qualche volta.
  for ($i = 0; $i -lt 5 -and (Test-Path -LiteralPath $p); $i++) {
    foreach ($s in $svcs) { Stop-Service -Name $s -Force -ErrorAction SilentlyContinue }
    Rename-Item -LiteralPath $p -NewName ((Split-Path $p -Leaf) + '.old') -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $p) { Start-Sleep -Seconds 2 }
  }
  if (Test-Path -LiteralPath $p) { $wuFail = $true }
}
Say 'Riavvio i servizi...'
foreach ($s in $svcs) { Start-Service -Name $s -ErrorAction SilentlyContinue }
if ($wuFail) { Say 'Non sono riuscito a ripristinare tutto: Windows teneva occupati i file. Riavvia il PC e premi di nuovo Risolvi.' }
else { Say 'Windows Update ripristinato: riprova ad aggiornare da Impostazioni > Windows Update.' }
`},
	{ID: "wipe_free", Name: "Cancella lo spazio libero", Desc: "Sovrascrive lo spazio libero del disco così i file già cancellati non si recuperano più. Può durare ore.", Level: 0, Minutes: 90, Timeout: 8 * time.Hour, Script: `
$ssd = $false
try { $pn = (Get-Partition -DriveLetter ($env:SystemDrive.TrimEnd(':')) -ErrorAction Stop | Get-Disk -ErrorAction Stop).Number; $ssd = [string](Get-PhysicalDisk -ErrorAction Stop | Where-Object { [string]$_.DeviceId -eq [string]$pn } | Select-Object -First 1).MediaType -eq 'SSD' } catch {}
if ($ssd) { Say 'Nota onesta: il tuo disco è un SSD. Sugli SSD Windows cancella già i file per davvero da solo (TRIM), quindi questo passaggio serve poco e consuma un po'' il disco. Lo faccio lo stesso, visto che l''hai chiesto.' }
Say ('Sovrascrivo lo spazio libero di ' + $env:SystemDrive + ' (può durare molto, puoi usare il PC nel frattempo)...')
$code = Run-Hidden 'cipher.exe' ('/w:' + $env:SystemDrive + '\')
# Se cipher si ferma a metà lascia una cartella enorme: la tolgo sempre.
Remove-Safe (Join-Path ($env:SystemDrive + '\') 'EFSTMPWP')
if ($code -eq 0) { Say 'Fatto: i file cancellati in passato non sono più recuperabili.' } else { Say ('Terminato con codice ' + $code) }
`},
	{ID: "recover", Name: "Recupera file cancellati", Desc: "Apre il Cestino e lo strumento gratuito di Microsoft per recuperare foto e documenti cancellati.", Level: 0, Minutes: 1, Timeout: 20 * time.Minute, Script: `
Say 'Prima guarda nel Cestino: lo apro adesso.'
Start-Process 'explorer.exe' 'shell:RecycleBinFolder'
$w = Get-Command winfr.exe -ErrorAction SilentlyContinue
if (-not $w) {
  $wg = Get-Command winget.exe -ErrorAction SilentlyContinue
  if ($wg) {
    Say 'Installo Windows File Recovery (strumento gratuito di Microsoft)...'
    [void](Run-Hidden $wg.Source 'install --id 9N26S50LN705 --source msstore --accept-source-agreements --accept-package-agreements --silent')
  }
}
Start-Process -FilePath "$env:SystemRoot\explorer.exe" -ArgumentList 'https://support.microsoft.com/it-it/windows/recuperare-file-persi-in-windows-10-61f5b28a-f5b8-3cc2-0f8e-a63cb4e1d4c4'
Say 'Ho aperto la guida ufficiale Microsoft: spiega passo passo come recuperare i file.'
Say 'Consiglio: non salvare nuovi file sul disco finché non hai recuperato quelli che ti servono.'
`},
	{ID: "privacy", Name: "Privacy: stop pubblicità e tracciamento", Desc: "Spegne pubblicità, suggerimenti e raccolta dati di Windows.", Level: 0, Minutes: 0.2, Timeout: 3 * time.Minute, Script: `
function SetDw([string]$path, [string]$name, [int]$val) { if (-not (Test-Path $path)) { New-Item -Path $path -Force | Out-Null }; New-ItemProperty -Path $path -Name $name -Value $val -PropertyType DWord -Force | Out-Null }
SetDw 'HKCU:\Software\Microsoft\Windows\CurrentVersion\AdvertisingInfo' 'Enabled' 0
$cdm = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\ContentDeliveryManager'
foreach ($n in @('SilentInstalledAppsEnabled','SystemPaneSuggestionsEnabled','SoftLandingEnabled','SubscribedContent-338388Enabled','SubscribedContent-338389Enabled','SubscribedContent-338393Enabled','SubscribedContent-353694Enabled','SubscribedContent-353696Enabled','SubscribedContent-310093Enabled')) { SetDw $cdm $n 0 }
SetDw 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Privacy' 'TailoredExperiencesWithDiagnosticDataEnabled' 0
SetDw 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced' 'ShowSyncProviderNotifications' 0
SetDw 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced' 'Start_IrisRecommendations' 0
SetDw 'HKCU:\Software\Policies\Microsoft\Windows\Explorer' 'DisableSearchBoxSuggestions' 1
SetDw 'HKLM:\SOFTWARE\Policies\Microsoft\Windows\DataCollection' 'AllowTelemetry' 1
SetDw 'HKCU:\Software\Microsoft\Input\TIPC' 'Enabled' 0
Say 'Pubblicità nel menu Start, suggerimenti, ID pubblicitario e raccolta dati extra: spenti.'
Say 'Alcune modifiche si vedono dopo il riavvio. Puoi annullarle con "Privacy: ripristina".'
`},
	{ID: "privacy_undo", Name: "Privacy: ripristina", Desc: "Rimette le impostazioni di privacy com'erano di fabbrica.", Level: 0, Minutes: 0.2, Timeout: 3 * time.Minute, Script: `
function SetDw([string]$path, [string]$name, [int]$val) { if (-not (Test-Path $path)) { New-Item -Path $path -Force | Out-Null }; New-ItemProperty -Path $path -Name $name -Value $val -PropertyType DWord -Force | Out-Null }
SetDw 'HKCU:\Software\Microsoft\Windows\CurrentVersion\AdvertisingInfo' 'Enabled' 1
$cdm = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\ContentDeliveryManager'
foreach ($n in @('SilentInstalledAppsEnabled','SystemPaneSuggestionsEnabled','SoftLandingEnabled','SubscribedContent-338388Enabled','SubscribedContent-338389Enabled','SubscribedContent-338393Enabled','SubscribedContent-353694Enabled','SubscribedContent-353696Enabled','SubscribedContent-310093Enabled')) { SetDw $cdm $n 1 }
SetDw 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Privacy' 'TailoredExperiencesWithDiagnosticDataEnabled' 1
Remove-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced' -Name 'ShowSyncProviderNotifications' -ErrorAction SilentlyContinue
Remove-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced' -Name 'Start_IrisRecommendations' -ErrorAction SilentlyContinue
Remove-ItemProperty -Path 'HKCU:\Software\Policies\Microsoft\Windows\Explorer' -Name 'DisableSearchBoxSuggestions' -ErrorAction SilentlyContinue
Remove-ItemProperty -Path 'HKLM:\SOFTWARE\Policies\Microsoft\Windows\DataCollection' -Name 'AllowTelemetry' -ErrorAction SilentlyContinue
Remove-ItemProperty -Path 'HKCU:\Software\Microsoft\Input\TIPC' -Name 'Enabled' -ErrorAction SilentlyContinue
Say 'Impostazioni di privacy rimesse come di fabbrica.'
`},
}

const analyzeScript = `
$junk = [long]0
$junk += Get-FolderSize $env:TEMP
$junk += Get-FolderSize (Join-Path $env:windir 'Temp')
$junk += Get-FolderSize (Join-Path $env:windir 'SoftwareDistribution\Download')
foreach ($p in $GpuCaches) { $junk += Get-FolderSize $p }
foreach ($b in $BrowserRoots) {
  if (-not (Test-Path -LiteralPath $b.Root)) { continue }
  Get-ChildItem -LiteralPath $b.Root -Directory -Force -ErrorAction SilentlyContinue | ForEach-Object {
    foreach ($sub in @('Cache', 'Code Cache', 'GPUCache', 'cache2')) { $junk += Get-FolderSize (Join-Path $_.FullName $sub) }
  }
}
$rb = [long]0
try { $s = ((New-Object -ComObject Shell.Application).Namespace(0xA).Items() | Measure-Object -Property Size -Sum).Sum; if ($s) { $rb = [long]$s } } catch {}
$junk += $rb
foreach ($p in @((Join-Path ($env:SystemDrive + '\') 'AMD'), (Join-Path ($env:SystemDrive + '\') 'NVIDIA'), (Join-Path $env:ProgramData 'NVIDIA Corporation\Downloader'))) { $junk += Get-FolderSize $p }
# LiveKernelReports: la pulizia toglie solo i rapporti più vecchi di 3 giorni, quindi conto solo quelli.
try { $old = (Get-Date).AddHours(-72); $junk += [long](@(Get-ChildItem -LiteralPath (Join-Path $env:windir 'LiveKernelReports') -Recurse -File -Force -ErrorAction SilentlyContinue | Where-Object { $_.LastWriteTime -lt $old }) | Measure-Object Length -Sum).Sum } catch {}
$startup = @(Get-TassoStartup | Where-Object { $_.enabled }).Count
$badDev = @(Get-CimInstance Win32_PnPEntity -ErrorAction SilentlyContinue | Where-Object { $_.ConfigManagerErrorCode -ne 0 -and $_.ConfigManagerErrorCode -ne 22 -and $_.ConfigManagerErrorCode -ne 45 }).Count
$diskWarn = @(Get-PhysicalDisk -ErrorAction SilentlyContinue | Where-Object { [string]$_.HealthStatus -ne 'Healthy' }).Count
$o = [ordered]@{ junk = $junk; recycle = $rb; startup = $startup; badDevices = $badDev; diskWarnings = $diskWarn }
[Console]::Out.WriteLine('JSON ' + ($o | ConvertTo-Json -Compress))
`

const hardwareScript = `
$cpu = Get-CimInstance Win32_Processor | Select-Object -First 1
$gpus = @(Get-CimInstance Win32_VideoController | Where-Object { $_.Name -notmatch 'Basic|Remote|Virtual|Parsec|Meta' })
$board = Get-CimInstance Win32_BaseBoard
$bios = Get-CimInstance Win32_BIOS
$sys = Get-CimInstance Win32_ComputerSystem
$os = Get-CimInstance Win32_OperatingSystem
$mem = @(Get-CimInstance Win32_PhysicalMemory)
$speed = ($mem | Measure-Object -Property ConfiguredClockSpeed -Maximum).Maximum
if (-not $speed) { $speed = ($mem | Measure-Object -Property Speed -Maximum).Maximum }
$bat = @(Get-CimInstance Win32_Battery -ErrorAction SilentlyContinue)
$disks = @()
foreach ($d in @(Get-PhysicalDisk -ErrorAction SilentlyContinue)) {
  $r = $d | Get-StorageReliabilityCounter -ErrorAction SilentlyContinue
  $disks += [ordered]@{ name = $d.FriendlyName; type = [string]$d.MediaType; bus = [string]$d.BusType; size = [long]$d.Size; health = [string]$d.HealthStatus; temp = $r.Temperature; wear = $r.Wear }
}
$gl = @()
foreach ($g in $gpus) {
  $gl += [ordered]@{ name = $g.Name; driver = $g.DriverVersion; driverDate = $(if ($g.DriverDate) { $g.DriverDate.ToString('dd/MM/yyyy') } else { '' }); ram = [long]$g.AdapterRAM; res = $(if ($g.CurrentHorizontalResolution) { [string]$g.CurrentHorizontalResolution + 'x' + [string]$g.CurrentVerticalResolution } else { '' }); hz = $g.CurrentRefreshRate }
}
$o = [ordered]@{
  cpu = [ordered]@{ name = ($cpu.Name -replace '\s+', ' ').Trim(); cores = $cpu.NumberOfCores; threads = $cpu.NumberOfLogicalProcessors; mhz = $cpu.MaxClockSpeed }
  gpus = $gl
  board = [ordered]@{ maker = $board.Manufacturer; model = $board.Product; bios = $bios.SMBIOSBIOSVersion; biosDate = $(if ($bios.ReleaseDate) { $bios.ReleaseDate.ToString('dd/MM/yyyy') } else { '' }) }
  pc = [ordered]@{ maker = $sys.Manufacturer; model = $sys.Model; laptop = ($bat.Count -gt 0) }
  ram = [ordered]@{ total = [long]$sys.TotalPhysicalMemory; modules = $mem.Count; speed = $speed }
  os = [ordered]@{ name = $os.Caption; version = $os.Version; build = $os.BuildNumber; arch = $os.OSArchitecture }
  disks = $disks
}
[Console]::Out.WriteLine('JSON ' + ($o | ConvertTo-Json -Compress -Depth 5))
`

const modeBase = `
$BAL = '381b4222-f694-41f0-9685-ff5bb260df2e'
$HIGH = '8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c'
$SAVER = 'a1841308-3541-4fab-bc81-f71556f20b4a'
function SetDw([string]$path, [string]$name, [int]$val) { if (-not (Test-Path $path)) { New-Item -Path $path -Force | Out-Null }; New-ItemProperty -Path $path -Name $name -Value $val -PropertyType DWord -Force | Out-Null }
function Use-Plan([string]$guid) { powercfg /setactive $guid 2>$null | Out-Null; return ($LASTEXITCODE -eq 0) }
function Use-HighPerf {
  # Ryzen X3D con due gruppi di core (7900X3D, 7950X3D, 9900X3D, 9950X3D): AMD vuole il piano Bilanciato
  # per mettere i giochi sui core con la cache 3D. "Prestazioni elevate" qui farebbe perdere FPS.
  $cpuN = ''; try { $cpuN = [string](Get-CimInstance Win32_Processor -ErrorAction Stop | Select-Object -First 1).Name } catch {}
  if ($cpuN -match '(?i)\b(7900|7950|9900|9950)X3D|\b(7945|9955|9945)HX3D') {
    [void](Use-Plan $BAL)
    powercfg /overlaysetactive ded574b5-45a0-4f42-8737-46345c09c238 2>$null | Out-Null
    Say 'Piano energetico: Bilanciato con "Prestazioni migliori". Sul tuo Ryzen X3D è il più veloce per i giochi (lo consiglia AMD).'
    return
  }
  if (Use-Plan $HIGH) { Say 'Piano energetico: Prestazioni elevate.'; return }
  # Prima cerco una copia già fatta (da Tasso o da Windows), così non ne creo una nuova a ogni gioco.
  $re = '[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}'
  foreach ($l in @(powercfg /list 2>$null)) {
    if ($l -match 'Prestazioni elevate|High performance|Tasso prestazioni') {
      $g = [regex]::Match($l, $re).Value
      if ($g -and (Use-Plan $g)) { Say 'Piano energetico: Prestazioni elevate.'; return }
    }
  }
  # Copia con lo stesso codice di Windows: la prossima volta basta attivarla.
  powercfg -duplicatescheme $HIGH $HIGH 2>$null | Out-Null
  if (Use-Plan $HIGH) { Say 'Piano energetico: Prestazioni elevate (l''ho aggiunto io, mancava).'; return }
  [void](Use-Plan $BAL)
  powercfg /overlaysetactive ded574b5-45a0-4f42-8737-46345c09c238 2>$null | Out-Null
  if ($LASTEXITCODE -eq 0) { Say 'Il PC ha un solo piano energetico: ho messo la modalità "Prestazioni migliori".' }
  else { Say 'Questo PC permette solo il piano "Bilanciato": ho tenuto quello. Se è un portatile, attaccalo alla corrente quando giochi.' }
}
function Set-CpuMax([int]$pct) {
  powercfg /setacvalueindex SCHEME_CURRENT SUB_PROCESSOR PROCTHROTTLEMAX $pct 2>$null | Out-Null
  powercfg /setdcvalueindex SCHEME_CURRENT SUB_PROCESSOR PROCTHROTTLEMAX $pct 2>$null | Out-Null
  powercfg /setactive SCHEME_CURRENT 2>$null | Out-Null
}
function Set-Toasts([int]$on) { SetDw 'HKCU:\Software\Microsoft\Windows\CurrentVersion\PushNotifications' 'ToastEnabled' $on }
function Set-MouseAccel([bool]$on) {
  $sig = '[DllImport("user32.dll")] public static extern bool SystemParametersInfo(int a, int b, int[] c, int d);'
  Add-Type -MemberDefinition $sig -Name SPI -Namespace TassoMouse | Out-Null
  if ($on) { [void][TassoMouse.SPI]::SystemParametersInfo(4, 0, [int[]](6, 10, 1), 3) } else { [void][TassoMouse.SPI]::SystemParametersInfo(4, 0, [int[]](0, 0, 0), 3) }
}
function Get-PowerIdx([string]$sub, [string]$set, [string]$scheme = 'SCHEME_CURRENT') {
  # Le ultime due righe "0x..." di powercfg /q sono il valore attuale con la corrente e a batteria.
  $m = @([regex]::Matches((@(powercfg /q $scheme $sub $set 2>$null) -join "\n"), '0x[0-9a-fA-F]{8}'))
  if ($m.Count -lt 2) { return $null }
  return @([Convert]::ToInt32($m[$m.Count-2].Value, 16), [Convert]::ToInt32($m[$m.Count-1].Value, 16))
}
$PrevKey = 'HKCU:\Software\Tasso\PrimaDelleModalita'
function Save-Prev {
  # Salvo com'era il PC solo la prima volta: così "Normale" rimette le TUE impostazioni, non quelle di fabbrica.
  if (-not $TassoCanSave) { return }
  $old = Get-ItemProperty -Path $PrevKey -ErrorAction SilentlyContinue
  if ($old -and $old.Ok) { return }
  Remove-Item -Path $PrevKey -Recurse -Force -ErrorAction SilentlyContinue
  try {
    New-Item -Path $PrevKey -Force | Out-Null
    $g = [regex]::Match((@(powercfg /getactivescheme 2>$null) -join ' '), '[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}').Value
    if ($g) { New-ItemProperty -Path $PrevKey -Name 'Plan' -Value $g -PropertyType String -Force | Out-Null }
    $v = Get-PowerIdx 'SUB_VIDEO' 'VIDEOIDLE'; if ($v) { New-ItemProperty -Path $PrevKey -Name 'Monitor' -Value $v[0] -PropertyType DWord -Force | Out-Null }
    $v = Get-PowerIdx 'SUB_SLEEP' 'STANDBYIDLE'; if ($v) { New-ItemProperty -Path $PrevKey -Name 'Standby' -Value $v[0] -PropertyType DWord -Force | Out-Null }
    $v = Get-PowerIdx 'SUB_VIDEO' 'VIDEOIDLE' $BAL; if ($v) { New-ItemProperty -Path $PrevKey -Name 'BalMonitor' -Value $v[0] -PropertyType DWord -Force | Out-Null }
    $v = Get-PowerIdx 'SUB_SLEEP' 'STANDBYIDLE' $BAL; if ($v) { New-ItemProperty -Path $PrevKey -Name 'BalStandby' -Value $v[0] -PropertyType DWord -Force | Out-Null }
    $ov = (Get-ItemProperty 'HKLM:\SYSTEM\CurrentControlSet\Control\Power\User\PowerSchemes' -Name 'ActiveOverlayAcPowerScheme' -ErrorAction SilentlyContinue).ActiveOverlayAcPowerScheme
    if ($ov) { New-ItemProperty -Path $PrevKey -Name 'Overlay' -Value ([string]$ov) -PropertyType String -Force | Out-Null }
    $v = Get-PowerIdx 'SUB_PROCESSOR' 'PROCTHROTTLEMAX'; if ($v) { New-ItemProperty -Path $PrevKey -Name 'CpuAc' -Value $v[0] -PropertyType DWord -Force | Out-Null; New-ItemProperty -Path $PrevKey -Name 'CpuDc' -Value $v[1] -PropertyType DWord -Force | Out-Null }
    $sig = '[DllImport("user32.dll")] public static extern bool SystemParametersInfo(int a, int b, int[] c, int d);'
    Add-Type -MemberDefinition $sig -Name SPI -Namespace TassoMouse -ErrorAction SilentlyContinue | Out-Null
    $mm = [int[]](0, 0, 0); if ([TassoMouse.SPI]::SystemParametersInfo(3, 0, $mm, 0)) { New-ItemProperty -Path $PrevKey -Name 'Mouse' -Value ($mm -join ',') -PropertyType String -Force | Out-Null }
    $t = Get-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\PushNotifications' -Name 'ToastEnabled' -ErrorAction SilentlyContinue
    $tv = 1; if ($t -and $null -ne $t.ToastEnabled) { $tv = [int]$t.ToastEnabled }
    New-ItemProperty -Path $PrevKey -Name 'Toasts' -Value $tv -PropertyType DWord -Force | Out-Null
    New-ItemProperty -Path $PrevKey -Name 'Ok' -Value 1 -PropertyType DWord -Force | Out-Null
  } catch {}
}
function Restore-Prev {
  $p = Get-ItemProperty -Path $PrevKey -ErrorAction SilentlyContinue
  if (-not $p -or -not $p.Ok) {
    # Niente di salvato: rimetto solo il minimo, senza toccare il mouse e i tempi che hai scelto tu.
    [void](Use-Plan $BAL)
    Set-CpuMax 100
    Set-Toasts 1
    $v = Get-PowerIdx 'SUB_VIDEO' 'VIDEOIDLE'; if ($v -and $v[0] -eq 0) { powercfg /change monitor-timeout-ac 10 | Out-Null }
    $v = Get-PowerIdx 'SUB_SLEEP' 'STANDBYIDLE'; if ($v -and $v[0] -eq 0) { powercfg /change standby-timeout-ac 30 | Out-Null }
    Remove-Item -Path $PrevKey -Recurse -Force -ErrorAction SilentlyContinue
    return $false
  }
  # Film e Silenzioso scrivono nel piano Bilanciato: rimetto anche i suoi tempi.
  if ($null -ne $p.BalMonitor) { powercfg /setacvalueindex $BAL SUB_VIDEO VIDEOIDLE $p.BalMonitor 2>$null | Out-Null }
  if ($null -ne $p.BalStandby) { powercfg /setacvalueindex $BAL SUB_SLEEP STANDBYIDLE $p.BalStandby 2>$null | Out-Null }
  if (-not ($p.Plan -and (Use-Plan $p.Plan))) { [void](Use-Plan $BAL) }
  if ($null -ne $p.CpuAc) { powercfg /setacvalueindex SCHEME_CURRENT SUB_PROCESSOR PROCTHROTTLEMAX $p.CpuAc 2>$null | Out-Null }
  if ($null -ne $p.CpuDc) { powercfg /setdcvalueindex SCHEME_CURRENT SUB_PROCESSOR PROCTHROTTLEMAX $p.CpuDc 2>$null | Out-Null }
  if ($null -ne $p.Monitor) { powercfg /setacvalueindex SCHEME_CURRENT SUB_VIDEO VIDEOIDLE $p.Monitor 2>$null | Out-Null }
  if ($null -ne $p.Standby) { powercfg /setacvalueindex SCHEME_CURRENT SUB_SLEEP STANDBYIDLE $p.Standby 2>$null | Out-Null }
  powercfg /setactive SCHEME_CURRENT 2>$null | Out-Null
  if ($p.Overlay -and $p.Overlay -match '^[0-9a-fA-F-]{36}$') { powercfg /overlaysetactive $p.Overlay 2>$null | Out-Null }
  if ($null -ne $p.Toasts) { Set-Toasts ([int]$p.Toasts) }
  if ($p.Mouse) {
    $mm = [int[]]@(([string]$p.Mouse).Split(',') | ForEach-Object { [int]$_ })
    if ($mm.Count -eq 3) {
      $sig = '[DllImport("user32.dll")] public static extern bool SystemParametersInfo(int a, int b, int[] c, int d);'
      Add-Type -MemberDefinition $sig -Name SPI -Namespace TassoMouse -ErrorAction SilentlyContinue | Out-Null
      [void][TassoMouse.SPI]::SystemParametersInfo(4, 0, $mm, 3)
    }
  }
  Remove-Item -Path $PrevKey -Recurse -Force -ErrorAction SilentlyContinue
  return $true
}
SetDw 'HKCU:\Software\Microsoft\GameBar' 'AutoGameModeEnabled' 1
`

var modeScripts = map[string]string{
	"normale": modeBase + `
powercfg /overlaysetactive 00000000-0000-0000-0000-000000000000 2>$null | Out-Null
if (Restore-Prev) { Say 'Modalità Normale: ho rimesso piano energetico, mouse, notifiche e tempi dello schermo come li avevi tu.' }
else { Say 'Modalità Normale: equilibrio tra velocità e consumi, notifiche attive.' }
`,
	"gaming": modeBase + `
Save-Prev
Use-HighPerf
Set-CpuMax 100
Set-Toasts 0
Say 'Modalità Gioco di Windows attiva, notifiche in pausa.'
Say 'Modalità Gaming pronta: tutta la potenza al gioco.'
`,
	"fps": modeBase + `
Save-Prev
Use-HighPerf
Set-CpuMax 100
Set-Toasts 0
Set-MouseAccel $false
Say 'Accelerazione del mouse spenta: mira più precisa.'
Say 'Modalità FPS competitivo pronta. Consiglio: nel gioco usa la frequenza massima del monitor.'
`,
	"film": modeBase + `
Save-Prev
[void](Use-Plan $BAL)
powercfg /overlaysetactive 00000000-0000-0000-0000-000000000000 2>$null | Out-Null
Set-CpuMax 100
Set-Toasts 0
powercfg /change monitor-timeout-ac 0 | Out-Null
powercfg /change standby-timeout-ac 0 | Out-Null
Say 'Modalità Film: niente notifiche, lo schermo non si spegne durante il film.'
`,
	"silenzioso": modeBase + `
Save-Prev
if (-not (Use-Plan $SAVER)) { [void](Use-Plan $BAL); powercfg /overlaysetactive 961cc777-2547-4f9d-8174-7d86181b8a7a 2>$null | Out-Null }
Set-CpuMax 70
Set-Toasts 1
Say 'Modalità Silenziosa: processore limitato al 70%, meno calore e ventole più tranquille.'
`,
}
