package main

// I passaggi di pulizia e regolazione: l'elenco con nome, descrizione e script di ognuno.

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
$env:PSModulePath = (Join-Path ([Environment]::SystemDirectory) 'WindowsPowerShell\v1.0\Modules') + ';' + (Join-Path ([Environment]::GetFolderPath('ProgramFiles')) 'WindowsPowerShell\Modules')
$ErrorActionPreference = 'SilentlyContinue'
$ProgressPreference = 'SilentlyContinue'
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
function Say([string]$m) { [Console]::Out.WriteLine('MSG ' + $m); [Console]::Out.Flush() }
function Freed([long]$b) { [Console]::Out.WriteLine('FREED ' + $b); [Console]::Out.Flush() }
function Fmt([double]$b) { if ($b -ge 1GB) { return ('{0:N2} GB' -f ($b / 1GB)) } elseif ($b -ge 1MB) { return ('{0:N1} MB' -f ($b / 1MB)) } else { return ('{0:N0} KB' -f ($b / 1KB)) } }
$TassoInizio = [DateTime]::UtcNow
$TassoLento = $false
function Time-Left {
  # Secondi che restano a questo passaggio prima che Tasso lo fermi ($TassoSecondi lo scrive Tasso).
  if (-not $TassoSecondi) { return 86400 }
  return [math]::Max(0, [double]$TassoSecondi - ([DateTime]::UtcNow - $TassoInizio).TotalSeconds)
}
function Get-Limit([int]$maxSec) {
  # Il tempo massimo per un comando: quello chiesto, ma sempre un po' prima della fine del passaggio.
  $lim = [int](Time-Left) - 20
  if ($maxSec -gt 0 -and $maxSec -lt $lim) { $lim = $maxSec }
  if ($lim -lt 10) { $lim = 10 }
  return $lim
}
function Stop-Tree([int]$id) {
  # Chiude il programma $id e quelli che ha aperto lui (solo il suo albero, niente altro).
  $tk = ''
  if ($TassoSys32) { $tk = Join-Path $TassoSys32 'taskkill.exe' }
  if (-not $tk -or -not [IO.File]::Exists($tk)) { $tk = 'taskkill.exe' }
  try {
    $k = New-Object System.Diagnostics.ProcessStartInfo
    $k.FileName = $tk; $k.Arguments = '/T /F /PID ' + $id; $k.UseShellExecute = $false; $k.CreateNoWindow = $true
    $kp = [System.Diagnostics.Process]::Start($k); [void]$kp.WaitForExit(15000)
  } catch {}
  try { Stop-Process -Id $id -Force -ErrorAction SilentlyContinue } catch {}
}
function Run-Watch([string]$file, [string]$argline, [string]$label = '', [int]$maxSec = 0, [double]$progFrom = 0, [double]$progTo = 1) {
  # Come Run-Hidden, ma legge l'uscita mentre il programma lavora: la percentuale (DISM 37%, SFC,
  # defrag) va nella barra di Tasso e ogni tanto nel registro; se passa il tempo massimo chiude il
  # programma con tutti i suoi figli e restituisce -2 ($TassoLento = $true). -1 = non partito.
  $script:TassoLento = $false
  $lim = Get-Limit $maxSec
  if (-not [IO.Path]::IsPathRooted($file) -and $TassoSys32) { $c = Join-Path $TassoSys32 $file; if ([IO.File]::Exists($c)) { $file = $c } }
  try {
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = $file
    $psi.Arguments = $argline
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $psi.WindowStyle = [System.Diagnostics.ProcessWindowStyle]::Hidden
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.RedirectStandardInput = $true
    $p = [System.Diagnostics.Process]::Start($psi)
  } catch { return -1 }
  try { $p.StandardInput.Close() } catch {}  # niente domande: chi chiede una risposta la trova vuota
  $er = $p.StandardError.ReadToEndAsync()
  $sr = $p.StandardOutput
  $buf = New-Object char[] 4096
  $t0 = [DateTime]::UtcNow; $lastSay = $t0; $lastPc = -1; $tail = ''; $eof = $false; $exitAt = $null
  $rd = $sr.ReadAsync($buf, 0, $buf.Length)
  while ($true) {
    if (-not $eof) {
      $got = $false
      try { $got = $rd.Wait(1000) } catch { $eof = $true }
      if ($got) {
        $n = 0; try { $n = [int]$rd.Result } catch { $n = 0 }
        if ($n -le 0) { $eof = $true }
        else {
          # SFC scrive in UTF-16: tolgo gli zeri, restano le cifre.
          $tail = ($tail + [string]::new($buf, 0, $n)).Replace([string][char]0, '')
          if ($tail.Length -gt 600) { $tail = $tail.Substring($tail.Length - 600) }
          $ms = [regex]::Matches($tail, '(\d{1,3})(?:[.,]\d+)?\s?%')
          if ($ms.Count) {
            $pc = [int]$ms[$ms.Count - 1].Groups[1].Value
            if ($pc -ge 0 -and $pc -le 100 -and $pc -ne $lastPc) {
              $lastPc = $pc
              $fr = $progFrom + ($progTo - $progFrom) * $pc / 100
              [Console]::Out.WriteLine('PROG ' + $fr.ToString('0.000', [Globalization.CultureInfo]::InvariantCulture)); [Console]::Out.Flush()
              if ($label -and ([DateTime]::UtcNow - $lastSay).TotalSeconds -ge 60) { $lastSay = [DateTime]::UtcNow; Say ($label + ': ' + $pc + '%') }
            }
          }
          $rd = $sr.ReadAsync($buf, 0, $buf.Length)
        }
      }
    } else { if ($p.WaitForExit(1000)) { break } }
    if (-not $eof -and $p.HasExited) {
      # Finito, ma un programma aperto da lui tiene ancora aperta l'uscita: non lo aspetto.
      if (-not $exitAt) { $exitAt = [DateTime]::UtcNow } elseif (([DateTime]::UtcNow - $exitAt).TotalSeconds -ge 5) { break }
    }
    $el = ([DateTime]::UtcNow - $t0).TotalSeconds
    if ($el -ge $lim) {
      Stop-Tree $p.Id
      $script:TassoLento = $true
      if ($label) { Say ('{0}: troppo lento, l''ho fermato dopo {1} minuti e vado avanti.' -f $label, [int][math]::Round($el / 60)); [Console]::Out.WriteLine('SLOW') }
      return -2
    }
    if ($label -and ([DateTime]::UtcNow - $lastSay).TotalSeconds -ge 90) { $lastSay = [DateTime]::UtcNow; Say ('{0}: ancora al lavoro ({1} min, al massimo {2}).' -f $label, [int][math]::Floor($el / 60), [int][math]::Ceiling($lim / 60)) }
  }
  try { return $p.ExitCode } catch { return -1 }
}
function Invoke-Limited([scriptblock]$sb, [int]$maxSec, [string]$label = '', $arg = $null) {
  # Esegue $sb (comandi di Windows che a volte restano appesi: dischi, servizi, punti di ripristino)
  # a parte, con un tempo massimo. Dentro, il valore passato è $A. Restituisce quello che $sb
  # produce, oppure $null se ci metteva troppo ($TassoLento = $true: l'ho lasciato perdere).
  $script:TassoLento = $false
  $lim = Get-Limit $maxSec
  $ps = $null
  try {
    $rs = [runspacefactory]::CreateRunspace()
    $rs.Open()
    $rs.SessionStateProxy.SetVariable('A', $arg)
    $rs.SessionStateProxy.SetVariable('ErrorActionPreference', 'SilentlyContinue')
    $rs.SessionStateProxy.SetVariable('ProgressPreference', 'SilentlyContinue')
    $rs.SessionStateProxy.SetVariable('ConfirmPreference', 'None')
    $ps = [powershell]::Create()
    $ps.Runspace = $rs
    [void]$ps.AddScript($sb.ToString())
    $h = $ps.BeginInvoke()
  } catch { return $null }
  $t0 = [DateTime]::UtcNow; $last = $t0
  while (-not $h.AsyncWaitHandle.WaitOne(1000)) {
    $el = ([DateTime]::UtcNow - $t0).TotalSeconds
    if ($el -ge $lim) {
      $script:TassoLento = $true
      try { [void]$ps.BeginStop($null, $null) } catch {}
      if ($label) { Say ('{0}: troppo lento, l''ho fermato dopo {1} minuti e vado avanti.' -f $label, [int][math]::Round($el / 60)); [Console]::Out.WriteLine('SLOW') }
      return $null
    }
    if ($label -and ([DateTime]::UtcNow - $last).TotalSeconds -ge 90) { $last = [DateTime]::UtcNow; Say ('{0}: ancora al lavoro ({1} min, al massimo {2}).' -f $label, [int][math]::Floor($el / 60), [int][math]::Ceiling($lim / 60)) }
  }
  $r = $null
  try { $r = $ps.EndInvoke($h) } catch {}
  try { $ps.Dispose(); $rs.Dispose() } catch {}
  return ,@($r)
}
function Stop-ServiceLimited([string[]]$names, [int]$sec = 45) {
  # Ferma i servizi aspettando al massimo $sec secondi ciascuno (Stop-Service aspetterebbe per
  # sempre un servizio bloccato). $true = tutti fermi.
  $ok = $true
  foreach ($n in $names) {
    $s = Get-Service -Name $n -ErrorAction SilentlyContinue
    if (-not $s) { continue }
    try {
      if ([string]$s.Status -ne 'Stopped') {
        if ([string]$s.Status -ne 'StopPending') { $s.Stop() }
        $s.WaitForStatus('Stopped', [TimeSpan]::FromSeconds($sec))
      }
    } catch { $ok = $false }
  }
  return $ok
}
function Start-ServiceQuick([string[]]$names, [int]$sec = 20) {
  # Riaccende i servizi senza restare appeso: aspetta al massimo $sec secondi ciascuno.
  foreach ($n in $names) {
    $s = Get-Service -Name $n -ErrorAction SilentlyContinue
    if (-not $s) { continue }
    try { if ([string]$s.Status -eq 'Stopped') { $s.Start() }; $s.WaitForStatus('Running', [TimeSpan]::FromSeconds($sec)) } catch {}
  }
}
function Get-FolderSizeQuick([string]$p, [int]$maxSec = 30) {
  # Come Get-FolderSize ma senza seguire giunzioni e al massimo per $maxSec secondi: -1 = non ha finito.
  if (-not (Test-Path -LiteralPath $p)) { return [long]0 }
  $t0 = [DateTime]::UtcNow; $tot = [long]0
  $st = New-Object System.Collections.Stack
  try { $st.Push((New-Object IO.DirectoryInfo($p))) } catch { return [long]0 }
  while ($st.Count) {
    if (([DateTime]::UtcNow - $t0).TotalSeconds -ge $maxSec) { return [long]-1 }
    $d = $st.Pop()
    try { foreach ($f in $d.EnumerateFiles()) { $tot += $f.Length } } catch {}
    try { foreach ($s in $d.EnumerateDirectories()) { if (-not ($s.Attributes -band [IO.FileAttributes]::ReparsePoint)) { $st.Push($s) } } } catch {}
  }
  return $tot
}
function Wait-Servicing([int]$maxSec = 120) {
  # Se Windows sta installando aggiornamenti (TiWorker), DISM resterebbe ad aspettarlo: aspetto al
  # massimo $maxSec secondi. $true = libero, si può partire.
  $t0 = [DateTime]::UtcNow; $said = $false
  while (Get-Process -Name TiWorker -ErrorAction SilentlyContinue) {
    if (([DateTime]::UtcNow - $t0).TotalSeconds -ge $maxSec) { return $false }
    if (-not $said) { Say 'Windows sta installando aggiornamenti: aspetto che finisca (al massimo 2 minuti)...'; $said = $true }
    Start-Sleep -Seconds 5
  }
  return $true
}
function Get-FolderSize([string]$p) {
  if (-not (Test-Path -LiteralPath $p)) { return [long]0 }
  $s = (Get-ChildItem -LiteralPath $p -Recurse -Force -File -ErrorAction SilentlyContinue | Measure-Object -Property Length -Sum).Sum
  if ($s) { return [long]$s } else { return [long]0 }
}
function In-Dir([string]$p, [string]$base) {
  # $p è $base o sta dentro $base (senza badare a maiuscole e minuscole né alla \ finale).
  if (-not $p -or -not $base) { return $false }
  $p = $p.TrimEnd('\'); $base = $base.TrimEnd('\')
  if (-not $p -or -not $base) { return $false }
  return ($p -ieq $base -or $p.StartsWith($base + '\', [StringComparison]::OrdinalIgnoreCase))
}
function Get-SafeRoot([string]$p) {
  # Si svuotano solo cartelle DENTRO quelle che Tasso pulisce davvero (AppData, temporanei, cache di
  # Windows Update, Programmi e ProgramData per le cache dei launcher, C:\AMD, C:\NVIDIA...).
  # Le cartelle vere arrivano da Tasso ($TassoWin, $TassoLocal...), non dalle variabili d'ambiente.
  # Restituisce la "radice fidata" più vicina che contiene $p ('' = non si tocca): sotto la radice
  # niente giunzioni; la radice stessa la controlla Tasso (cancella_sicura.go).
  if ([string]::IsNullOrWhiteSpace($p) -or $p.StartsWith('\\')) { return '' }  # niente \\?\ né cartelle di rete
  try { $full = [IO.Path]::GetFullPath($p).TrimEnd('\') } catch { return '' }
  if ($full.Length -le 3) { return '' }  # mai una radice come C:\
  $deny = @($TassoWin, $TassoProfilo, $TassoPF, $TassoPF86, $TassoPD, $TassoRoaming, $TassoLocal) | Where-Object { $_ } | ForEach-Object { $_.TrimEnd('\') }
  foreach ($d in $deny) { if ($full -ieq $d) { return '' } }
  # "Foglie": si possono svuotare loro stesse (es. la cartella dei temporanei).
  $leaf = @($TassoTempUtente, $TassoTempOriginale)
  if ($TassoWin) { $leaf += @(($TassoWin + '\Temp'), ($TassoWin + '\SoftwareDistribution\Download'), ($TassoWin + '\Minidump'), ($TassoWin + '\LiveKernelReports'), ($TassoWin + '\ServiceProfiles\LocalService\AppData\Local\Temp'), ($TassoWin + '\ServiceProfiles\NetworkService\AppData\Local\Temp')) }
  if ($TassoSysDrive) { $leaf += @(($TassoSysDrive + '\AMD'), ($TassoSysDrive + '\NVIDIA')) }
  # "Contenitori": solo le cartelle che ci sono dentro, mai loro.
  $cont = @($TassoLocal, $TassoRoaming, $TassoPD, $TassoPF, $TassoPF86)
  if ($TassoSysDrive) { $cont += ($TassoSysDrive + '\Intel') }
  # Steam può stare su un altro disco (es. D:\Steam): la sua cartella vale come contenitore, mai dentro
  # Windows. Il percorso viene da HKLM (lo scrive l'installatore di Steam da amministratore), non da
  # HKCU che qualunque programma può cambiare, e lì dentro deve esserci steam.exe.
  $st = ''; try { $st = [string](Get-ItemProperty -LiteralPath 'HKLM:\SOFTWARE\WOW6432Node\Valve\Steam' -Name InstallPath -ErrorAction Stop).InstallPath } catch {}
  if ($st) { try { $st = [IO.Path]::GetFullPath(($st -replace '/', '\')).TrimEnd('\') } catch { $st = '' } }
  if ($st -and ($st.Length -le 3 -or -not (Test-Path -LiteralPath (Join-Path $st 'steam.exe') -PathType Leaf))) { $st = '' }
  if ($st -and $TassoWin -and (In-Dir $st $TassoWin)) { $st = '' }
  # Mai una cartella che contiene quelle dell'utente o di sistema (es. C:\Users), mai il profilo di un altro utente.
  foreach ($d in $deny) { if ($st -and (In-Dir $d $st)) { $st = '' } }
  if ($st -and $TassoProfilo) { $u = Split-Path $TassoProfilo -Parent; if ((In-Dir $st $u) -and -not (In-Dir $st $TassoProfilo)) { $st = '' } }
  if ($st) { $cont += $st }
  $best = ''
  foreach ($r in $leaf) {
    if (-not $r) { continue }
    $r = ([string]$r).TrimEnd('\'); if ($r.Length -le 3) { continue }
    if ((In-Dir $full $r) -and $r.Length -gt $best.Length) { $best = $r }
  }
  foreach ($r in $cont) {
    if (-not $r) { continue }
    $r = ([string]$r).TrimEnd('\'); if ($r.Length -le 3) { continue }
    if ($full.StartsWith($r + '\', [StringComparison]::OrdinalIgnoreCase) -and $r.Length -gt $best.Length) { $best = $r }
  }
  return $best
}
function Test-SafeTarget([string]$p) { return [bool](Get-SafeRoot $p) }
function Test-NoReparse([string]$p, [string]$root = '') {
  # Le cartelle fra la radice fidata $root (esclusa) e $p (compresa) non devono essere giunzioni o
  # collegamenti: C:\NVIDIA potrebbe puntare a System32, o una cartella in AppData a un'altra cartella
  # del PC. SOPRA la radice invece una giunzione va bene (C:\Users spostato su D:, Steam su un altro
  # disco): lì decide Tasso, che guarda dove porta davvero la radice. Senza $root guarda solo $p.
  # È solo un primo controllo veloce: quello che conta lo rifà Tasso un attimo prima di cancellare.
  try { $cur = [IO.Path]::GetFullPath($p).TrimEnd('\') } catch { return $false }
  $r = ''
  if ($root) { try { $r = [IO.Path]::GetFullPath($root).TrimEnd('\') } catch { return $false } }
  if ($r -and -not (In-Dir $cur $r)) { return $false }
  while ($cur -and -not ($r -and $cur -ieq $r)) {
    try { $a = [IO.File]::GetAttributes($cur) } catch { return $false }
    if ($a -band [IO.FileAttributes]::ReparsePoint) { return $false }
    if (-not $r) { break }
    $up = [IO.Path]::GetDirectoryName($cur)
    if (-not $up -or $up -eq $cur) { break }
    $cur = $up.TrimEnd('\')
  }
  return $true
}
$TassoRifiuto = ''
function Invoke-TassoCancella([string[]]$a) {
  # La cancellazione vera la fa Tasso (cancella_sicura.go): apre ogni cartella partendo da quella
  # sopra, già aperta, senza mai seguire giunzioni o collegamenti, e controlla dove si trova davvero.
  # Così un programma che sostituisce una cartella con una giunzione mentre pulisco non mi porta in
  # System32. Restituisce i byte liberati, o $null se Tasso ha rifiutato (non ha toccato niente).
  $script:TassoRifiuto = ''
  if (-not $TassoExe -or -not [IO.File]::Exists($TassoExe)) { $script:TassoRifiuto = 'non trovo Tasso.exe'; return $null }
  $out = ''
  try {
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = $TassoExe
    $psi.Arguments = '--cancella-sicura ' + ((@($a) | ForEach-Object { '"' + ([string]$_).Replace('"', '').TrimEnd('\') + '"' }) -join ' ')
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    # Tasso si ferma da solo un po' prima della fine del passaggio (cartelle con milioni di file) e
    # dice quanto ha liberato fin lì: così il passaggio finisce e lo spazio viene contato.
    $psi.EnvironmentVariables['TASSO_FINE'] = [string][long]([DateTimeOffset]::UtcNow.ToUnixTimeSeconds() + [math]::Max(5, (Time-Left) - 30))
    $pr = [System.Diagnostics.Process]::Start($psi)
    $er = $pr.StandardError.ReadToEndAsync()
    $out = $pr.StandardOutput.ReadToEnd()
    $pr.WaitForExit()
  } catch { $script:TassoRifiuto = $_.Exception.Message; return $null }
  foreach ($l in @($out -split "\r?\n")) {
    if ($l -match '^LIBERATI (\d+)\s*$') { return [long]$Matches[1] }
    if ($l -match '^RIFIUTO (.*)$') { $script:TassoRifiuto = $Matches[1].Trim(); return $null }
  }
  $script:TassoRifiuto = 'nessuna risposta'
  return $null
}
function Remove-Safe([string]$p) {
  # Toglie $p (un file, o una cartella con tutto dentro) senza mai seguire collegamenti o giunzioni:
  # un collegamento si toglie lui, mai i file a cui punta. Il lavoro lo fa Tasso (Invoke-TassoCancella).
  if ([string]::IsNullOrWhiteSpace($p)) { return }
  try { $full = [IO.Path]::GetFullPath($p) } catch { return }
  [void](Invoke-TassoCancella @('togli', $full, '0'))
}
function Get-Winget {
  # winget vero: quello del pacchetto Microsoft "Programma di installazione app", in
  # Programmi\WindowsApps (scrivibile solo da Windows). Mai quello trovato nel PATH: il suo alias
  # sta in AppData\Local\Microsoft\WindowsApps, dove può scrivere qualunque programma dell'utente,
  # e Tasso lo farebbe partire da amministratore. '' se non c'è.
  if (-not $TassoPF) { return '' }
  $base = (Join-Path $TassoPF 'WindowsApps').TrimEnd('\') + '\'
  $pk = @(Get-AppxPackage -Name 'Microsoft.DesktopAppInstaller' -ErrorAction SilentlyContinue)
  if (-not $pk.Count) { $pk = @(Get-AppxPackage -AllUsers -Name 'Microsoft.DesktopAppInstaller' -ErrorAction SilentlyContinue) }
  foreach ($x in @($pk | Where-Object { $_.PublisherId -eq '8wekyb3d8bbwe' -and $_.InstallLocation } | Sort-Object { try { [version]$_.Version } catch { [version]'0.0' } } -Descending)) {
    try { $exe = [IO.Path]::GetFullPath((Join-Path ([string]$x.InstallLocation) 'winget.exe')) } catch { continue }
    if (-not $exe.StartsWith($base, [StringComparison]::OrdinalIgnoreCase)) { continue }
    if ([IO.File]::Exists($exe)) { return $exe }
  }
  return ''
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
  # Come Remove-Safe, ma dentro le cartelle toglie solo i file vecchi (modificati e creati prima di
  # $limit): una cartella con file nuovi resta, una cartella vuota va via solo se creata da tempo.
  if ([string]::IsNullOrWhiteSpace($p)) { return }
  try { $full = [IO.Path]::GetFullPath($p) } catch { return }
  $h = [math]::Max(0.001, ((Get-Date) - $limit).TotalHours)
  [void](Invoke-TassoCancella @('togli', $full, ([string]$h)))
}
function Clear-Folder([string]$p, [double]$minAgeHours = 0) {
  # Svuota la cartella $p (lei resta). Restituisce i byte liberati. Se non la tocco, in $TassoRifiuto
  # c'è il perché (vuoto = tutto normale, anche se non c'era niente da togliere).
  $script:TassoRifiuto = ''
  $root = Get-SafeRoot $p
  if (-not $root) { $script:TassoRifiuto = 'non è fra le cartelle che Tasso pulisce'; return [long]0 }
  $full = [IO.Path]::GetFullPath($p).TrimEnd('\')
  if (-not (Test-Path -LiteralPath $full)) { return [long]0 }
  if (-not (Test-NoReparse $full $root)) { $script:TassoRifiuto = 'c''è una giunzione o un collegamento'; return [long]0 }  # finirei nella cartella a cui punta
  if ($minAgeHours -lt 0) { $minAgeHours = 0 }
  $r = Invoke-TassoCancella @('svuota', $root, $full, ([string]$minAgeHours))
  if ($null -eq $r) { return [long]0 }
  return [long]$r
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
    # Mai per sempre: al massimo fino a poco prima della fine del passaggio, poi chiudo lui e i suoi figli.
    $script:TassoLento = $false
    if (-not $p.WaitForExit((Get-Limit 0) * 1000)) { Stop-Tree $p.Id; $script:TassoLento = $true; return -2 }
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
function Ensure-RestorePoint([string]$desc) {
  # $true se c'è un punto di ripristino di meno di 24 ore fa (Windows non ne fa due nello stesso giorno),
  # altrimenti provo a crearlo (al massimo 5 minuti). Non accende "Protezione sistema" da solo.
  $recent = {
    $last = @(Get-ComputerRestorePoint -ErrorAction SilentlyContinue | Sort-Object SequenceNumber)
    if (-not $last.Count) { return $null }
    try { return [Management.ManagementDateTimeConverter]::ToDateTime([string]$last[-1].CreationTime) } catch { return $null }
  }
  $w = & $recent
  if ($w -and $w -gt (Get-Date).AddHours(-24)) { Say ('Punto di ripristino: uso quello del ' + $w.ToString('dd/MM HH:mm') + '.'); return $true }
  $cp = Invoke-Limited { try { Checkpoint-Computer -Description $A -RestorePointType MODIFY_SETTINGS -ErrorAction Stop; 'ok' } catch { 'ERR' } } 300 'Punto di ripristino' $desc
  $w = & $recent
  if ($w -and $w -gt (Get-Date).AddHours(-1)) { Say 'Punto di ripristino creato.'; return $true }
  return $false
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
# Windows Update a volte resta appeso per ore (ricerca che non finisce, download fermo). Lo faccio
# lavorare a parte e guardo da qui: ogni fase ha un tempo massimo; se lo supera la lascio perdere
# (Windows Update la può finire da solo più tardi) e la pulizia va avanti.
$wuSt = [hashtable]::Synchronized(@{ Fase = 'cerca'; Da = [DateTime]::UtcNow; Fine = $false })
$wuLim = @{ cerca = 900; scarica = 1500; installa = 2100 }
$wuNomi = @{ cerca = 'ricerca degli aggiornamenti'; scarica = 'download'; installa = 'installazione' }
$wuProg = @{ cerca = 0.02; scarica = 0.2; installa = 0.6 }
$wuRs = $null; $wuPs = $null; $wuH = $null
try {
  $wuRs = [runspacefactory]::CreateRunspace()
  $wuRs.ApartmentState = [Threading.ApartmentState]::STA
  $wuRs.Open()
  $wuRs.SessionStateProxy.SetVariable('A', @{ St = $wuSt; Criteria = $Criteria; NothingMsg = $NothingMsg; SkipDisplay = [bool]$SkipDisplay })
  $wuRs.SessionStateProxy.SetVariable('ErrorActionPreference', 'SilentlyContinue')
  $wuPs = [powershell]::Create(); $wuPs.Runspace = $wuRs
  [void]$wuPs.AddScript({
    function Say([string]$m) { [Console]::Out.WriteLine('MSG ' + $m); [Console]::Out.Flush() }
    $St = $A.St
    try {
      $session = New-Object -ComObject Microsoft.Update.Session
      $searcher = $session.CreateUpdateSearcher()
      Say 'Cerco su Windows Update...'
      $res = $searcher.Search($A.Criteria)
      $coll = New-Object -ComObject Microsoft.Update.UpdateColl
      foreach ($u in $res.Updates) {
        # I driver della scheda video non passano da qui (come in "Driver di tutto il PC"): Windows Update
        # metterebbe un driver diverso da quello di AMD Software/NVIDIA. Si aggiornano dal riquadro Schede video.
        if ($A.SkipDisplay -and [string]$u.DriverClass -eq 'Display') { Say ('Salto il driver della scheda video: ' + $u.Title + ' (si aggiorna dal riquadro Schede video).'); continue }
        if (-not $u.EulaAccepted) { $u.AcceptEula() }
        [void]$coll.Add($u)
        Say ('Trovato: ' + $u.Title)
      }
      if ($coll.Count -eq 0) { Say $A.NothingMsg; $St.Fine = $true; return }
      Say ('Scarico ' + $coll.Count + ' aggiornamenti...')
      $St.Fase = 'scarica'; $St.Da = [DateTime]::UtcNow
      $dl = $session.CreateUpdateDownloader(); $dl.Updates = $coll; [void]$dl.Download()
      Say 'Installo...'
      $St.Fase = 'installa'; $St.Da = [DateTime]::UtcNow
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
    $St.Fine = $true
  }.ToString())
  $wuH = $wuPs.BeginInvoke()
} catch { Say ('Windows Update non ha risposto: ' + $_.Exception.Message); return }
$wuFase = ''; $wuLast = [DateTime]::UtcNow
while (-not $wuH.AsyncWaitHandle.WaitOne(1000)) {
  $f = [string]$wuSt.Fase
  if ($f -ne $wuFase) { $wuFase = $f; [Console]::Out.WriteLine('PROG ' + ([double]$wuProg[$f]).ToString('0.000', [Globalization.CultureInfo]::InvariantCulture)); [Console]::Out.Flush() }
  $el = ([DateTime]::UtcNow - [datetime]$wuSt.Da).TotalSeconds
  $lim = [math]::Min([double]$wuLim[$f], (Time-Left) - 20)
  if ($el -ge $lim) {
    try { [void]$wuPs.BeginStop($null, $null) } catch {}
    Say ('Windows Update ({0}) ci mette troppo: lo lascio perdere dopo {1} minuti e vado avanti. Windows può finire da solo più tardi.' -f $wuNomi[$f], [int][math]::Round($el / 60))
    # Windows Update resta appeso dentro Windows: chiudo subito questo PowerShell, senza aspettarlo.
    [Console]::Out.WriteLine('SLOW'); [Console]::Out.Flush()
    [Environment]::Exit(0)
  }
  if (([DateTime]::UtcNow - $wuLast).TotalSeconds -ge 90) { $wuLast = [DateTime]::UtcNow; Say ('Windows Update ({0}): ancora al lavoro ({1} min, al massimo {2}).' -f $wuNomi[$f], [int][math]::Floor($el / 60), [int][math]::Ceiling($lim / 60)) }
}
try { [void]$wuPs.EndInvoke($wuH); $wuPs.Dispose(); $wuRs.Dispose() } catch {}
`

var allSteps = []Step{
	{ID: "backup", Name: "Backup (punto di ripristino)", Desc: "Crea un punto di ripristino: se qualcosa non va torni indietro.", Level: 2, Minutes: 1.5, Timeout: 8 * time.Minute, Script: `
Say 'Creo il backup di Windows (punto di ripristino)...'
$key = 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\SystemRestore'
# "Protezione sistema" spenta (RPSessionInterval = 0): per fare il backup la devo accendere, ma lo dico
# chiaramente e spiego come rispegnerla (occupa un po' di spazio del disco). Niente più accensione muta.
$srOff = $false
try { $ri = (Get-ItemProperty -LiteralPath $key -Name 'RPSessionInterval' -ErrorAction Stop).RPSessionInterval; if ([int]$ri -eq 0) { $srOff = $true } } catch {}
try { Enable-ComputerRestore -Drive ($TassoSysDrive + '\') -ErrorAction Stop } catch {}
if ($srOff) { Say 'Protezione sistema era spenta su questo PC: l''ho accesa per poter fare il backup (usa un po'' di spazio del disco). Se la vuoi spenta: Pannello di controllo > Sistema > Protezione sistema > Configura > Disattiva.' }
$oldFreq = (Get-ItemProperty -Path $key -Name 'SystemRestorePointCreationFrequency' -ErrorAction SilentlyContinue).SystemRestorePointCreationFrequency
# Mi segno il valore di prima: se il passaggio viene interrotto, Tasso lo rimette al prossimo avvio.
# Se il segno c'è già (un backup di prima interrotto e non ancora rimesso), il valore vero è quello
# segnato allora: adesso c'è lo 0 messo da Tasso. Non lo riscrivo e non ricreo mai la chiave
# (New-Item -Force su una chiave che c'è la svuota).
$mark = 'HKLM:\SOFTWARE\Tasso\PuntoRipristino'
try {
  $prev = ''
  if (Test-Path -LiteralPath $mark) { $prev = [string](Get-ItemProperty -LiteralPath $mark -Name 'Old' -ErrorAction SilentlyContinue).Old }
  if ($prev -eq 'none') { $oldFreq = $null }
  elseif ($prev -match '^\d{1,10}$') { $oldFreq = [int]$prev }
  else {
    if (-not (Test-Path -LiteralPath 'HKLM:\SOFTWARE\Tasso')) { New-Item -Path 'HKLM:\SOFTWARE\Tasso' -ErrorAction Stop | Out-Null }
    if (-not (Test-Path -LiteralPath $mark)) { New-Item -Path $mark -ErrorAction Stop | Out-Null }
    New-ItemProperty -LiteralPath $mark -Name 'Old' -Value $(if ($null -ne $oldFreq) { [string][int]$oldFreq } else { 'none' }) -PropertyType String -Force | Out-Null
  }
} catch {}
New-ItemProperty -Path $key -Name 'SystemRestorePointCreationFrequency' -Value 0 -PropertyType DWord -Force | Out-Null
$before = @(Get-ComputerRestorePoint).Count
# Windows a volte resta appeso a creare il punto (copie shadow): al massimo 5 minuti, poi vado avanti.
$cp = Invoke-Limited { try { Checkpoint-Computer -Description 'Tasso - prima della manutenzione' -RestorePointType MODIFY_SETTINGS -ErrorAction Stop; 'ok' } catch { 'ERR ' + $_.Exception.Message } } 300 'Backup'
if ($TassoLento) { Say 'Il punto di ripristino ci metteva troppo: non lo aspetto (Windows lo può finire da solo).' }
elseif ($cp -and ([string]@($cp)[0]).StartsWith('ERR ')) { Say ('Backup non creato: ' + ([string]@($cp)[0]).Substring(4)) }
$after = @(Get-ComputerRestorePoint).Count
if ($null -ne $oldFreq) { New-ItemProperty -Path $key -Name 'SystemRestorePointCreationFrequency' -Value ([int]$oldFreq) -PropertyType DWord -Force | Out-Null } else { Remove-ItemProperty -Path $key -Name 'SystemRestorePointCreationFrequency' -ErrorAction SilentlyContinue }
Remove-Item -Path $mark -Recurse -Force -ErrorAction SilentlyContinue
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
	{ID: "temp_user", Name: "File temporanei", Desc: "Svuota i file temporanei del tuo account più vecchi di un giorno (quelli in uso restano).", Level: 1, Minutes: 0.5, Timeout: 8 * time.Minute, Script: `
# La cartella temporanea vera dell'utente (AppData\Local\Temp, passata da Tasso): $env:TEMP qui
# è quella di Tasso in Programmi, e una variabile d'ambiente non decide mai cosa si svuota.
$f = Clear-Folder $TassoTempUtente 24
# Se avevi spostato i temporanei altrove (es. D:\Temp), Tasso passa anche quella ($TassoTempOriginale).
if ($TassoTempOriginale) { $f += Clear-Folder $TassoTempOriginale 24 }
Freed $f; Say ('Liberati ' + (Fmt $f))
`},
	{ID: "temp_win", Name: "File temporanei di Windows", Desc: "Svuota C:\\Windows\\Temp (solo i file più vecchi di un giorno).", Level: 1, Minutes: 0.5, Timeout: 8 * time.Minute, Script: `
$f = Clear-Folder ($TassoWin + '\Temp') 24
Freed $f; Say ('Liberati ' + (Fmt $f))
`},
	{ID: "recycle", Name: "Cestino", Desc: "Toglie dal Cestino solo quello che è lì da più di 7 giorni: le cose buttate di recente restano, nel caso ti servano.", Level: 1, Minutes: 0.3, Timeout: 5 * time.Minute, Script: `
# Anche a mano lascio le cose buttate di recente (come nella pulizia programmata): una foto buttata
# per sbaglio ieri, o i doppioni e i resti che Tasso ha appena spostato lì, si devono poter recuperare.
# Il Cestino intero si svuota solo con il passaggio a parte "Svuota tutto il Cestino" (recycle_all).
& {
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
}
`},
	{ID: "recycle_all", Name: "Svuota tutto il Cestino", Desc: "Svuota il Cestino di tutte le unità, comprese le cose buttate oggi: quello che c'è dentro non si potrà più recuperare. Non fa parte di nessuna pulizia: parte solo se lo scegli tu.", Level: 0, Minutes: 0.3, Timeout: 5 * time.Minute, Script: `
function Get-BinSize { $s = [long]0; try { $x = ((New-Object -ComObject Shell.Application).Namespace(0xA).Items() | Measure-Object -Property Size -Sum).Sum; if ($x) { $s = [long]$x } } catch {}; return $s }
$before = Get-BinSize
[void](Invoke-Limited { Clear-RecycleBin -Force -ErrorAction SilentlyContinue } 120 'Cestino')
$after = Get-BinSize
# Conto solo quello che è sparito davvero (se Windows non ha svuotato, non dico di aver liberato niente).
$f = [math]::Max([long]0, $before - $after)
Freed $f
if ($after -gt 0) { Say ('Cestino svuotato solo in parte: liberati ' + (Fmt $f) + ', restano ' + (Fmt $after) + ' (file in uso o su dischi non collegati).') }
else { Say ('Cestino svuotato: ' + (Fmt $f)) }
`},
	{ID: "thumbs", Name: "Cache delle anteprime", Desc: "Elimina le miniature di Esplora file (si ricreano da sole).", Level: 1, Minutes: 0.2, Timeout: 3 * time.Minute, Script: `
$dir = Join-Path $env:LOCALAPPDATA 'Microsoft\Windows\Explorer'
$f = [long]0
Get-ChildItem -LiteralPath $dir -Filter 'thumbcache_*.db' -Force -ErrorAction SilentlyContinue | ForEach-Object {
  $len = $_.Length
  Remove-Item -LiteralPath $_.FullName -Force -ErrorAction SilentlyContinue
  if (-not (Test-Path -LiteralPath $_.FullName)) { $f += $len }
}
Freed $f; Say ('Liberati ' + (Fmt $f))
`},
	{ID: "browser", Name: "Cache dei browser", Desc: "Chrome, Edge, Brave, Opera, Firefox: solo la cache. Password e cookie restano.", Level: 1, Minutes: 0.8, Timeout: 10 * time.Minute, Script: `
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
[void](Invoke-Limited { Clear-DnsClientCache } 60)
if ($TassoLento) { Say 'Il servizio DNS non risponde: salto, non è importante.' } else { Say 'Cache DNS svuotata.' }
`},
	{ID: "app_cache", Name: "Cache delle app", Desc: "Discord, Steam, Teams ed Epic Games: solo la cache, accessi e impostazioni restano.", Level: 2, Minutes: 0.5, Timeout: 8 * time.Minute, Script: `
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
	{ID: "wu_cache", Name: "Cache di Windows Update", Desc: "File degli aggiornamenti già scaricati. Se ne serve ancora uno, Windows lo riscarica da solo.", Level: 2, Minutes: 1, Timeout: 8 * time.Minute, Script: `
if ((Get-Process -Name TiWorker, MoUsoCoreWorker, TrustedInstaller -ErrorAction SilentlyContinue) -or (Test-Path -LiteralPath 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired')) { Say 'Windows sta installando degli aggiornamenti proprio adesso: salto, per non disturbarlo. Rifallo più tardi.'; return }
# Stop-Service aspetterebbe per sempre un servizio bloccato: al massimo 45 secondi ciascuno.
if (-not (Stop-ServiceLimited @('wuauserv', 'bits') 45)) { Start-ServiceQuick @('wuauserv', 'bits'); Say 'Windows Update non si ferma (forse sta lavorando): salto, rifallo più tardi.'; return }
$f = Clear-Folder (Join-Path $TassoWin 'SoftwareDistribution\Download')
Start-ServiceQuick @('wuauserv', 'bits')
Freed $f; Say ('Liberati ' + (Fmt $f))
`},
	{ID: "cleanmgr", Name: "Pulizia file di sistema", Desc: "Rapporti errori, file di dump, log vecchi e cache di Ottimizzazione recapito. Senza finestre.", Level: 2, Minutes: 1, Timeout: 10 * time.Minute, Script: `
$f = [long]0
$pd = $env:ProgramData
foreach ($d in @((Join-Path $pd 'Microsoft\Windows\WER\ReportArchive'), (Join-Path $pd 'Microsoft\Windows\WER\ReportQueue'), (Join-Path $pd 'Microsoft\Windows\WER\Temp'), (Join-Path $env:LOCALAPPDATA 'Microsoft\Windows\WER'), (Join-Path $env:LOCALAPPDATA 'CrashDumps'))) {
  if (Test-Path -LiteralPath $d) { $f += Clear-Folder $d }
}
# I rapporti delle schermate blu degli ultimi 14 giorni restano: servono per capire cosa non va.
$md = Join-Path $TassoWin 'Minidump'
if (Test-Path -LiteralPath $md) { $f += Clear-Folder $md 336 }
$dmp = Join-Path $TassoWin 'MEMORY.DMP'
if ((Test-Path -LiteralPath $dmp) -and (Get-Item -LiteralPath $dmp -Force).LastWriteTime -lt (Get-Date).AddDays(-14)) { $len = (Get-Item -LiteralPath $dmp -Force).Length; Remove-Item -LiteralPath $dmp -Force -ErrorAction SilentlyContinue; if (-not (Test-Path -LiteralPath $dmp)) { $f += $len } }
$old = (Get-Date).AddDays(-14)
foreach ($d in @((Join-Path $TassoWin 'Logs\CBS'), (Join-Path $TassoWin 'Logs\DISM'), (Join-Path $TassoWin 'Logs\WindowsUpdate'), (Join-Path $TassoWin 'Panther'))) {
  if (-not (Test-Path -LiteralPath $d)) { continue }
  Get-ChildItem -LiteralPath $d -File -Force -ErrorAction SilentlyContinue | Where-Object { $_.LastWriteTime -lt $old -and $_.Extension -in @('.log','.cab','.etl','.txt') } | ForEach-Object {
    $len = $_.Length; Remove-Item -LiteralPath $_.FullName -Force -ErrorAction SilentlyContinue; if (-not (Test-Path -LiteralPath $_.FullName)) { $f += $len }
  }
}
try {
  $drive = Get-PSDrive -Name ($TassoSysDrive.TrimEnd(':'))
  $before = [long]$drive.Free
  [void](Invoke-Limited { Delete-DeliveryOptimizationCache -Force -ErrorAction Stop | Out-Null } 120 'Ottimizzazione recapito')
  $after = [long](Get-PSDrive -Name ($TassoSysDrive.TrimEnd(':'))).Free
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
# Elenco di ogni spostamento (da -> a): con "Annulla il riordino dei Download" tutto torna dov'era.
$logDir = Join-Path $dl 'Tasso - riordini'
$log = Join-Path $logDir ('riordino-' + (Get-Date -Format 'yyyyMMdd-HHmmss') + '.tsv')
$righe = New-Object System.Collections.ArrayList
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
      $from = $_.FullName
      try { Move-Item -LiteralPath $from -Destination $target -ErrorAction Stop; $moved++; [void]$righe.Add($from + [string][char]9 + $target) } catch {}
      break
    }
  }
}
if ($righe.Count) {
  try { New-Item -ItemType Directory -Path $logDir -Force | Out-Null; [IO.File]::WriteAllLines($log, [string[]]$righe.ToArray(), (New-Object System.Text.UTF8Encoding($false))) } catch { Say 'Non sono riuscito a scrivere l''elenco degli spostamenti: il riordino non si potrà annullare da Tasso.' }
}
Say ('Riordinati ' + $moved + ' file nella cartella Download.')
if ($righe.Count) { Say 'Se vuoi rimettere tutto com''era: Pulisci › Un passaggio alla volta › "Annulla il riordino dei Download".' }
`},
	{ID: "downloads_undo", Name: "Annulla il riordino dei Download", Desc: "Rimette nella cartella Download i file spostati dall'ultimo riordino di Tasso. I file che hai già spostato o cancellato tu restano come sono.", Level: 0, Minutes: 0.3, Timeout: 10 * time.Minute, Script: `
$dl = Get-DownloadsPath
$logDir = Join-Path $dl 'Tasso - riordini'
$last = @(Get-ChildItem -LiteralPath $logDir -Filter 'riordino-*.tsv' -File -ErrorAction SilentlyContinue | Sort-Object Name -Descending | Select-Object -First 1)
if (-not $last.Count) { Say 'Non trovo nessun riordino dei Download fatto da Tasso da annullare.'; return }
$back = 0; $skip = 0
foreach ($r in @([IO.File]::ReadAllLines($last[0].FullName))) {
  $p = $r.Split([char]9)
  if ($p.Count -ne 2) { continue }
  $from = $p[0]; $to = $p[1]
  # Solo dentro la cartella Download, e solo se il file è ancora dove l'ho messo e il suo posto è libero.
  if (-not (In-Dir $from $dl) -or -not (In-Dir $to $dl)) { $skip++; continue }
  if (-not (Test-Path -LiteralPath $to -PathType Leaf) -or (Test-Path -LiteralPath $from)) { $skip++; continue }
  try { Move-Item -LiteralPath $to -Destination $from -ErrorAction Stop; $back++ } catch { $skip++ }
}
try { Rename-Item -LiteralPath $last[0].FullName -NewName ($last[0].BaseName + '-annullato.tsv') -ErrorAction Stop } catch {}
# Le cartelle create dal riordino che sono rimaste vuote le tolgo.
foreach ($c in @('Immagini', 'Documenti', 'Video', 'Musica', 'Programmi', 'Archivi')) {
  $d = Join-Path $dl $c
  if ((Test-Path -LiteralPath $d -PathType Container) -and -not (Get-ChildItem -LiteralPath $d -Force -ErrorAction SilentlyContinue | Select-Object -First 1)) { Remove-Item -LiteralPath $d -Force -ErrorAction SilentlyContinue }
}
Say ('Rimessi nella cartella Download: ' + $back + ' file.')
if ($skip) { Say ('Lasciati dove sono: ' + $skip + ' (li avevi già spostati, rinominati o cancellati, oppure al loro posto c''è un file nuovo con lo stesso nome).') }
`},
	{ID: "apps", Name: "Aggiornamento programmi", Desc: "Aggiorna i programmi installati con winget. Attenzione: alcune app (Discord, Claude, Proton...) dopo l'aggiornamento chiedono di rifare l'accesso.", Level: 0, Minutes: 6, Timeout: 60 * time.Minute, Script: `
$wg = Get-Winget
if (-not $wg) { Say 'winget non è disponibile su questo PC: salto.'; return }
Say 'Aggiorno i programmi (alcuni minuti)...'
$code = Run-Hidden $wg 'upgrade --all --silent --accept-source-agreements --accept-package-agreements --disable-interactivity'
if ($code -eq 0) { Say 'Programmi aggiornati.' } else { Say ('Fatto, con qualche programma da aggiornare a mano (codice ' + $code + ').') }
`},
	{ID: "drivers", Name: "Aggiornamento driver", Desc: "Driver certificati da Microsoft tramite Windows Update.", Level: 3, Minutes: 6, Timeout: 30 * time.Minute, Script: `
$Criteria = "IsInstalled=0 and Type='Driver' and IsHidden=0 and BrowseOnly=0"
$NothingMsg = 'I driver risultano già aggiornati.'
$SkipDisplay = $true
# Un driver di rete o audio sbagliato può lasciare il PC senza internet o senza suono: prima di
# installare voglio un punto di ripristino (quello della pulizia di poco fa va bene). Senza, salto.
if (-not (Ensure-RestorePoint 'Tasso - prima dei driver di Windows Update')) { Say 'Non sono riuscito a creare un punto di ripristino: per sicurezza non installo i driver adesso. Controlla che "Protezione sistema" sia attiva e riprova.'; return }
` + windowsUpdateScript},
	{ID: "winupdate", Name: "Aggiornamenti di Windows", Desc: "Installa gli aggiornamenti di sicurezza e di sistema.", Level: 3, Minutes: 12, Timeout: 60 * time.Minute, Script: `
$Criteria = "IsInstalled=0 and Type='Software' and IsHidden=0 and BrowseOnly=0"
$NothingMsg = 'Windows è già aggiornato.'
` + windowsUpdateScript},
	{ID: "startup", Name: "Programmi all'avvio", Desc: "Ti mostra quali programmi partono all'accensione e rallentano il PC.", Level: 3, Minutes: 0.2, Timeout: 3 * time.Minute, Script: `
$all = @(Get-TassoStartup)
$on = @($all | Where-Object { $_.enabled })
$off = @($all | Where-Object { -not $_.enabled })
Say ('Programmi che partono all''accensione: ' + $on.Count + ' (altri ' + $off.Count + ' sono già spenti)')
foreach ($s in $on) { Say (' - ' + $s.name) }
if ($on.Count -gt 5) { Say 'Consiglio: Tasso > Pulisci e ripara > Avvio del PC > spegni quelli che non usi.' }
`},
	{ID: "disk_health", Name: "Salute dei dischi", Desc: "Controlla stato, usura e temperatura di SSD e dischi.", Level: 3, Minutes: 0.3, Timeout: 3 * time.Minute, Script: `
# Un disco esterno o un lettore difettoso può far restare appesa la lettura: al massimo 90 secondi.
$disks = Invoke-Limited { Get-PhysicalDisk | ForEach-Object { [pscustomobject]@{ D = $_; R = ($_ | Get-StorageReliabilityCounter) } } } 90 'Salute dei dischi'
if ($TassoLento) { Say 'Windows non risponde sulla salute dei dischi: salto, riprova più tardi.'; return }
foreach ($x in @($disks)) {
  if (-not $x) { continue }
  $r = $x.R; $d = $x.D
  $t = ''; if ($r.Temperature) { $t = ', ' + $r.Temperature + ' gradi' }
  $w = ''; if ($r.Wear -ne $null) { $w = ', usura ' + $r.Wear + '%' }
  $h = [string]$d.HealthStatus
  if ($h -eq 'Healthy') { $h = 'in salute' } elseif ($h -eq 'Warning') { $h = 'ATTENZIONE, da tenere d''occhio' } elseif ($h -eq 'Unhealthy') { $h = 'A RISCHIO: fai subito un backup dei tuoi file' }
  Say ($d.FriendlyName + ' (' + $d.MediaType + '): ' + $h + $w + $t)
}
`},
	{ID: "dism_cleanup", Name: "Pulizia componenti di Windows", Desc: "Rimuove le versioni vecchie dei componenti di sistema.", Level: 3, Minutes: 10, Timeout: 40 * time.Minute, Script: `
if (Skip-AutoOnBattery) { return }
$drive = Get-PSDrive -Name ($TassoSysDrive.TrimEnd(':'))
$before = [long]$drive.Free
if (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending') { Say 'Windows sta aspettando un riavvio per finire un aggiornamento: adesso questo passaggio non funzionerebbe bene. Riavvia il PC e poi rifallo.'; [Console]::Out.WriteLine('REBOOT'); return }
if (-not (Wait-Servicing 120)) { Say 'Windows sta ancora installando aggiornamenti: salto la pulizia dei componenti, rifalla più tardi.'; return }
Say 'Pulizia componenti in corso (5-20 minuti)...'
$code = Run-Watch 'dism.exe' '/Online /English /Cleanup-Image /StartComponentCleanup' 'DISM' 2280
$after = [long](Get-PSDrive -Name ($TassoSysDrive.TrimEnd(':'))).Free
$f = [math]::Max(0, $after - $before)
Freed $f
if ($TassoLento) { Say ('Pulizia componenti fermata perché troppo lenta: liberati comunque ' + (Fmt $f) + '. Windows la rifà da solo quando il PC è inattivo.') }
else { Say ('Fatto (codice ' + $code + '), liberati ' + (Fmt $f)) }
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
	{ID: "programs", Name: "Controllo di ogni programma", Desc: "Ripara le app di Windows danneggiate e trova i programmi rotti.", Level: 4, Minutes: 3, Timeout: 15 * time.Minute, Script: `
# 3.1: solo le app di questo account e solo quelle rotte davvero (non quelle che si stanno aggiornando);
# nella pulizia programmata non le tocco, perché Windows potrebbe aggiornarle proprio in quel momento.
$badApps = @()
if (-not $TassoAuto) { $badApps = @(Get-AppxPackage -ErrorAction SilentlyContinue | Where-Object { [string]$_.Status -match 'NeedsRemediation|Modified|Tampered|LicenseIssue' -and $_.InstallLocation }) }
if ($TassoAuto) { Say 'App di Windows: nella pulizia programmata non le controllo (lo faccio quando la avvii tu).' }
elseif ($badApps.Count -eq 0) { Say 'App di Windows: tutte a posto.' }
foreach ($p in $badApps) {
  Say ('Riparo: ' + $p.Name)
  # Al massimo 2 minuti per app: a volte Windows resta ad aspettare un'app aperta o un aggiornamento.
  $ok = Invoke-Limited { try { Add-AppxPackage -DisableDevelopmentMode -Register $A -ErrorAction Stop; 'ok' } catch { 'no' } } 120 '' (Join-Path $p.InstallLocation 'AppxManifest.xml')
  if ($TassoLento) { Say '   ci metteva troppo: la salto.' } elseif ([string]@($ok)[0] -eq 'ok') { Say '   riparata.' } else { Say '   non riuscita, prova a reinstallarla dal Microsoft Store.' }
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
	{ID: "repair", Name: "Riparazione di Windows", Desc: "DISM RestoreHealth + SFC: ripara i file di sistema danneggiati.", Level: 4, Minutes: 25, Timeout: 60 * time.Minute, Script: `
if (Skip-AutoOnBattery) { return }
if (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending') { Say 'Windows sta aspettando un riavvio per finire un aggiornamento: adesso questo passaggio non funzionerebbe bene. Riavvia il PC e poi rifallo.'; [Console]::Out.WriteLine('REBOOT'); return }
Say 'Controllo e riparo l''immagine di Windows (DISM, 10-30 minuti)...'
if (-not (Wait-Servicing 120)) { Say 'Windows sta ancora installando aggiornamenti: salto la riparazione, rifalla più tardi (meglio dopo un riavvio).'; return }
# DISM al massimo 35 minuti, SFC al massimo 22: la barra segue la loro percentuale.
$c1 = Run-Watch 'dism.exe' '/Online /English /Cleanup-Image /RestoreHealth' 'DISM' 2100 0 0.55
$needReboot = $false
if ($TassoLento) { Say 'DISM era troppo lento (spesso è internet lenta): l''ho fermato. Passo al controllo dei file.' }
elseif ($c1 -eq 0) { Say 'Immagine di Windows a posto.' } elseif ($c1 -eq 3010) { Say 'Immagine di Windows riparata: serve un riavvio per finire.'; $needReboot = $true } else { Say ('DISM non è riuscito a finire (codice ' + $c1 + '). Spesso succede senza internet: riprova con la connessione attiva.') }
Say 'Controllo i file di sistema (SFC, 10-20 minuti)...'
$t0 = Get-Date
$c2 = Run-Watch 'sfc.exe' '/scannow' 'SFC' 1320 0.55 1
$sfcSlow = $TassoLento
if ($sfcSlow) { Say 'SFC era troppo lento: l''ho fermato. Rifai la riparazione da sola quando non ti serve il PC.' }
# SFC non dice il risultato con il codice: lo leggo dal suo registro (solo le righe scritte adesso).
$bad = 0; $fixed = 0; $seen = @{}
try {
  foreach ($l in @(Get-Content -LiteralPath (Join-Path $TassoWin 'Logs\CBS\CBS.log') -Tail 40000 -ErrorAction Stop)) {
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
elseif (-not $sfcSlow) { Say 'File di sistema: nessun problema trovato.' }
if ($needReboot -or $bad -gt 0 -or $fixed -gt 0) { [Console]::Out.WriteLine('REBOOT') }
`},
	{ID: "chkdsk", Name: "Controllo errori del disco", Desc: "Controlla il disco di sistema mentre usi il PC. Se trova errori gravi te lo dice; la riparazione al riavvio la prenota solo se avvii il controllo da solo o da Risolvi un problema, mai dentro una pulizia.", Level: 4, Minutes: 4, Timeout: 30 * time.Minute, Script: `
if (Skip-AutoOnBattery) { return }
Say ('Controllo il disco ' + $TassoSysDrive + '...')
$dl = $TassoSysDrive.TrimEnd(':')
$r = $null
$rr = Invoke-Limited { try { [string](Repair-Volume -DriveLetter $A -Scan -ErrorAction Stop) } catch { 'ERR' } } 1500 'Controllo del disco' $dl
if ($TassoLento) { Say 'Il controllo del disco ci metteva troppo: l''ho lasciato perdere. Riprova da solo quando non ti serve il PC.'; return }
$r = [string]@($rr)[0]
if (-not $r -or $r -eq 'ERR') { Say 'Il controllo del disco non è riuscito adesso (forse Windows sta già controllando il disco). Riprova più tardi.'; return }
switch ([string]$r) {
  'NoErrorsFound' { Say 'Nessun errore sul disco.' }
  'SpotFixNeeded' {
    Say 'Trovati piccoli errori: li sistemo subito (pochi secondi, senza riavviare)...'
    $f = [string]@(Invoke-Limited { try { [string](Repair-Volume -DriveLetter $A -SpotFix -ErrorAction Stop) } catch { 'ERR' } } 300 '' $dl)[0]
    if ($f -eq 'NoErrorsFound') { Say 'Errori del disco sistemati.' } else { Say 'Errori segnati: Windows li sistema al prossimo riavvio.'; [Console]::Out.WriteLine('REBOOT') }
  }
  'FullRepairNeeded' {
    Say 'ATTENZIONE: il disco ha errori che si riparano solo al riavvio. Prima di tutto fai una copia dei tuoi file importanti (chiavetta, disco esterno o OneDrive).'
    if ($TassoAuto -or -not $TassoSingle) { Say 'Non prenoto la riparazione dentro una pulizia: prima fai il backup, poi avvia da solo "Controllo errori del disco".'; return }
    $sched = $false; try { [void](Repair-Volume -DriveLetter $dl -OfflineScanAndFix -ErrorAction Stop); $sched = $true } catch {}
    if ($sched) { Say ('Ho prenotato la riparazione: Windows la fa al prossimo riavvio (può metterci qualche minuto, non spegnere il PC). Se vuoi annullarla per fare prima il backup, nel Prompt dei comandi come amministratore scrivi: chkntfs /x ' + $TassoSysDrive); [Console]::Out.WriteLine('REBOOT') }
    else { Say ('Non sono riuscito a prenotare la riparazione. Dopo il backup, apri il Prompt dei comandi come amministratore e scrivi: chkdsk ' + $dl + ': /f') }
  }
  default { Say ('Controllo finito (risultato: ' + $r + ').') }
}
`},
	{ID: "optimize", Name: "Deframmentazione / TRIM", Desc: "Deframmenta i dischi meccanici, ottimizza gli SSD.", Level: 3, Minutes: 5, Timeout: 30 * time.Minute, Script: `
if (Skip-AutoOnBattery) { return }
Say 'Ottimizzo i dischi (sui dischi meccanici può volerci un po'')...'
# Solo i dischi interni fissi (niente chiavette, dischi esterni o di rete).
# L'elenco dei dischi può restare appeso con dischi difettosi: al massimo 90 secondi.
$letters = Invoke-Limited {
  $vols = @(Get-Volume -ErrorAction SilentlyContinue | Where-Object { $_.DriveType -eq 'Fixed' -and $_.DriveLetter -and $_.FileSystem -match 'NTFS|ReFS' })
  $usb = @{}
  try { foreach ($d in @(Get-Disk -ErrorAction Stop | Where-Object { [string]$_.BusType -eq 'USB' })) { foreach ($pp in @(Get-Partition -DiskNumber $d.Number -ErrorAction SilentlyContinue)) { if ($pp.DriveLetter) { $usb[[string]$pp.DriveLetter] = $true } } } } catch {}
  foreach ($v in $vols) { if (-not $usb.ContainsKey([string]$v.DriveLetter)) { [string]$v.DriveLetter } }
} 90
if ($TassoLento) { Say 'Windows non risponde sull''elenco dei dischi: salto, riprova più tardi.'; return }
$letters = @($letters | Where-Object { $_ })
$n = 0; $i = 0
foreach ($l in $letters) {
  # Ogni disco al massimo 20 minuti (un disco meccanico molto pieno può metterci ore): /U per la percentuale.
  $code = Run-Watch 'defrag.exe' ($l + ': /O /U') ('Disco ' + $l) 1200 ($i / $letters.Count) (($i + 1) / $letters.Count)
  $i++; $n++
  if ($TassoLento) { Say ('Disco ' + $l + ': fermato perché troppo lento, lo finisce Windows da solo con la sua ottimizzazione automatica.') }
  else { Say ('Disco ' + $l + ': ottimizzato (codice ' + $code + ').') }
  if ((Time-Left) -lt 60) { break }
}
if ($n -eq 0) { Say 'Nessun disco interno da ottimizzare.' }
`},
	{ID: "defender", Name: "Scansione antivirus", Desc: "Scansione rapida con Microsoft Defender.", Level: 3, Minutes: 4, Timeout: 20 * time.Minute, Script: `
$mp = Join-Path $env:ProgramFiles 'Windows Defender\MpCmdRun.exe'
if (-not (Test-Path -LiteralPath $mp)) { Say 'Microsoft Defender non trovato: salto.'; return }
Say 'Scansione rapida in corso...'
$code = Run-Watch $mp '-Scan -ScanType 1' 'Antivirus' 1080
if ($TassoLento) { Say 'La scansione ci metteva troppo: l''ho fermata. Defender continua comunque a proteggerti in tempo reale.'; return }
if ($code -eq 0) { Say 'Nessuna minaccia trovata.' } elseif ($code -eq 2) { Say 'ATTENZIONE: trovate minacce. Apri Sicurezza di Windows per i dettagli.' } else { Say 'Scansione terminata. Se usi un altro antivirus, Defender potrebbe essere spento.' }
`},
	{ID: "browser_ext", Name: "Controllo estensioni dei browser", Desc: "Elenca le estensioni di Chrome, Edge e Brave: se non ne riconosci una, toglila.", Level: 3, Minutes: 0.3, Timeout: 3 * time.Minute, Script: `
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
	{ID: "turbo", Name: "Turbo: libera il PC per giocare", Desc: "Chiude con gentilezza i programmi in background che non servono mentre giochi (OneDrive, Teams, Skype...), solo se non hanno una finestra aperta: una chiamata o un lavoro in corso non si toccano. Si riaprono al prossimo avvio.", Level: 0, Minutes: 0.2, Timeout: 3 * time.Minute, Script: `
$names = @('OneDrive', 'Teams', 'ms-teams', 'Skype', 'PhoneExperienceHost', 'YourPhone', 'Widgets', 'GoogleDriveFS', 'Dropbox', 'CCXProcess', 'Creative Cloud', 'AdobeCollabSync', 'Cortana')
$freed = [long]0; $closed = New-Object System.Collections.ArrayList; $kept = New-Object System.Collections.ArrayList
$mySession = (Get-Process -Id $PID).SessionId
Add-Type -Namespace TassoTurbo -Name W -MemberDefinition '[DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc f, IntPtr l); public delegate bool EnumProc(IntPtr h, IntPtr l); [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h); [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint p);' -ErrorAction SilentlyContinue
# Processi con almeno una finestra visibile (anche ridotta a icona): una riunione di Teams, una
# chiamata di Skype o una finestra di OneDrive aperta non si chiudono mai.
$withWin = @{}
try { [void][TassoTurbo.W]::EnumWindows({ param($h, $l) if ([TassoTurbo.W]::IsWindowVisible($h)) { $wp = [uint32]0; [void][TassoTurbo.W]::GetWindowThreadProcessId($h, [ref]$wp); $withWin[[int]$wp] = $true }; return $true }, [IntPtr]::Zero) } catch {}
$tk = Join-Path $TassoSys32 'taskkill.exe'
foreach ($n in $names) {
  # Solo i programmi del tuo account (non quelli di altri utenti collegati allo stesso PC).
  $ps = @(Get-Process -Name $n -ErrorAction SilentlyContinue | Where-Object { $_.SessionId -eq $mySession })
  if ($ps.Count -eq 0) { continue }
  if (@($ps | Where-Object { $withWin.ContainsKey([int]$_.Id) -or $_.MainWindowHandle -ne [IntPtr]::Zero }).Count -gt 0) { [void]$kept.Add($n); continue }
  $mem = [long](($ps | Measure-Object -Property PrivateMemorySize64 -Sum).Sum)
  # Chiusura normale (senza /F), come la X: finiscono di salvare o di caricare.
  foreach ($p in $ps) { [void](Run-Hidden $tk ('/PID ' + $p.Id)) }
  for ($i = 0; $i -lt 16 -and @(Get-Process -Name $n -ErrorAction SilentlyContinue | Where-Object { $_.SessionId -eq $mySession }).Count -gt 0; $i++) { Start-Sleep -Milliseconds 500 }
  if (@(Get-Process -Name $n -ErrorAction SilentlyContinue | Where-Object { $_.SessionId -eq $mySession }).Count -eq 0) { $freed += $mem; [void]$closed.Add($n) } else { [void]$kept.Add($n) }
}
if ($closed.Count -eq 0) { Say 'Nessun programma inutile da chiudere in background: il PC è già libero.' }
else { Say ('Chiusi: ' + ($closed -join ', ')); Say ('Memoria liberata: ' + (Fmt $freed)) }
if ($kept.Count) { Say ('Lasciati aperti (hanno una finestra aperta o non si sono chiusi da soli): ' + ($kept -join ', ') + '.') }
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
$q = Join-Path $TassoWin 'System32\spool\PRINTERS'
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
$sd = Join-Path $TassoWin 'SoftwareDistribution'
$cr = Join-Path $TassoWin 'System32\catroot2'
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
try { $pn = (Get-Partition -DriveLetter ($TassoSysDrive.TrimEnd(':')) -ErrorAction Stop | Get-Disk -ErrorAction Stop).Number; $ssd = [string](Get-PhysicalDisk -ErrorAction Stop | Where-Object { [string]$_.DeviceId -eq [string]$pn } | Select-Object -First 1).MediaType -eq 'SSD' } catch {}
if ($ssd) { Say 'Nota onesta: il tuo disco è un SSD. Sugli SSD Windows cancella già i file per davvero da solo (TRIM), quindi questo passaggio serve poco e consuma un po'' il disco. Lo faccio lo stesso, visto che l''hai chiesto.' }
Say ('Sovrascrivo lo spazio libero di ' + $TassoSysDrive + ' (può durare molto, puoi usare il PC nel frattempo)...')
$code = Run-Hidden 'cipher.exe' ('/w:' + $TassoSysDrive + '\')
# Se cipher si ferma a metà lascia una cartella enorme: la tolgo sempre.
Remove-Safe (Join-Path ($TassoSysDrive + '\') 'EFSTMPWP')
if ($code -eq 0) { Say 'Fatto: i file cancellati in passato non sono più recuperabili.' } else { Say ('Terminato con codice ' + $code) }
`},
	{ID: "recover", Name: "Recupera file cancellati", Desc: "Apre il Cestino, installa lo strumento gratuito di Microsoft per recuperare i file (si usa scrivendo comandi) e apre la guida ufficiale passo passo.", Level: 0, Minutes: 1, Timeout: 20 * time.Minute, Script: `
Say 'Prima guarda nel Cestino: lo apro adesso.'
Start-Process 'explorer.exe' 'shell:RecycleBinFolder'
# Windows File Recovery è un'app dello Store: la cerco fra le app installate (il suo alias non è nel PATH di Tasso).
$w = @(Get-AppxPackage -Name 'Microsoft.WindowsFileRecovery' -ErrorAction SilentlyContinue).Count -gt 0
if (-not $w) {
  $wg = Get-Winget
  if ($wg) {
    Say 'Installo Windows File Recovery (strumento gratuito di Microsoft)...'
    [void](Run-Hidden $wg 'install --id 9N26S50LN705 --source msstore --accept-source-agreements --accept-package-agreements --silent')
  }
}
Start-Process -FilePath "$TassoWin\explorer.exe" -ArgumentList 'https://support.microsoft.com/it-it/windows/recuperare-file-persi-in-windows-10-61f5b28a-f5b8-3cc2-0f8e-a63cb4e1d4c4'
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
$junk += Get-FolderSize $TassoTempUtente
if ($TassoTempOriginale) { $junk += Get-FolderSize $TassoTempOriginale }
$junk += Get-FolderSize (Join-Path $TassoWin 'Temp')
$junk += Get-FolderSize (Join-Path $TassoWin 'SoftwareDistribution\Download')
# Le cache della scheda video (shader) non si contano: nessun livello di pulizia le toglie (solo il passaggio a mano gpu_cache).
foreach ($b in $BrowserRoots) {
  if (-not (Test-Path -LiteralPath $b.Root)) { continue }
  Get-ChildItem -LiteralPath $b.Root -Directory -Force -ErrorAction SilentlyContinue | ForEach-Object {
    foreach ($sub in @('Cache', 'Code Cache', 'GPUCache', 'cache2')) { $junk += Get-FolderSize (Join-Path $_.FullName $sub) }
  }
}
$rb = [long]0
try { $s = ((New-Object -ComObject Shell.Application).Namespace(0xA).Items() | Measure-Object -Property Size -Sum).Sum; if ($s) { $rb = [long]$s } } catch {}
$junk += $rb
foreach ($p in @((Join-Path ($TassoSysDrive + '\') 'AMD'), (Join-Path ($TassoSysDrive + '\') 'NVIDIA'), (Join-Path $env:ProgramData 'NVIDIA Corporation\Downloader'))) { $junk += Get-FolderSize $p }
# LiveKernelReports: la pulizia toglie solo i rapporti più vecchi di 3 giorni, quindi conto solo quelli.
try { $old = (Get-Date).AddHours(-72); $junk += [long](@(Get-ChildItem -LiteralPath (Join-Path $TassoWin 'LiveKernelReports') -Recurse -File -Force -ErrorAction SilentlyContinue | Where-Object { $_.LastWriteTime -lt $old }) | Measure-Object Length -Sum).Sum } catch {}
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
  pc = [ordered]@{ maker = $sys.Manufacturer; model = $sys.Model; family = [string]$sys.SystemFamily; laptop = ($bat.Count -gt 0) }
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
    # $TassoMousePrev: il mouse di prima del Boost (che lo cambia solo finché giochi). Altrimenti
    # leggo le impostazioni salvate di Windows: il Boost non le scrive mai, quindi anche se nel
    # frattempo ha tolto l'accelerazione qui c'è ancora quella vera.
    if ([string]$TassoMousePrev -match '^\d{1,2},\d{1,2},\d{1,2}$') { New-ItemProperty -Path $PrevKey -Name 'Mouse' -Value ([string]$TassoMousePrev) -PropertyType String -Force | Out-Null }
    else {
      $mk = Get-ItemProperty -Path 'HKCU:\Control Panel\Mouse' -ErrorAction SilentlyContinue
      $mv = (@([string]$mk.MouseThreshold1, [string]$mk.MouseThreshold2, [string]$mk.MouseSpeed) | ForEach-Object { $_.Trim() }) -join ','
      if ($mk -and $mv -match '^\d{1,2},\d{1,2},\d{1,2}$') { New-ItemProperty -Path $PrevKey -Name 'Mouse' -Value $mv -PropertyType String -Force | Out-Null }
    }
    $t = Get-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\PushNotifications' -Name 'ToastEnabled' -ErrorAction SilentlyContinue
    $tv = 1; if ($t -and $null -ne $t.ToastEnabled) { $tv = [int]$t.ToastEnabled }
    New-ItemProperty -Path $PrevKey -Name 'Toasts' -Value $tv -PropertyType DWord -Force | Out-Null
    New-ItemProperty -Path $PrevKey -Name 'Ok' -Value 1 -PropertyType DWord -Force | Out-Null
  } catch {}
}
function Undo-FpsMouse {
  # Uscendo da FPS competitivo verso Gaming/Film/Silenzioso: se il mouse è ancora senza
  # accelerazione (messo da FPS) rimetto quello di prima. Il Boost acceso lo gestisce da sé.
  if (-not $TassoFromFps -or [string]$TassoMousePrev) { return }
  $p = Get-ItemProperty -Path $PrevKey -ErrorAction SilentlyContinue
  if (-not $p -or -not $p.Ok -or -not $p.Mouse -or [string]$p.Mouse -eq '0,0,0') { return }
  $mk = Get-ItemProperty -Path 'HKCU:\Control Panel\Mouse' -ErrorAction SilentlyContinue
  if (-not $mk -or ([string]$mk.MouseSpeed).Trim() -ne '0') { return }
  $mm = [int[]]@(([string]$p.Mouse).Split(',') | ForEach-Object { [int]$_ })
  if ($mm.Count -eq 3) {
    $sig = '[DllImport("user32.dll")] public static extern bool SystemParametersInfo(int a, int b, int[] c, int d);'
    Add-Type -MemberDefinition $sig -Name SPI -Namespace TassoMouse -ErrorAction SilentlyContinue | Out-Null
    [void][TassoMouse.SPI]::SystemParametersInfo(4, 0, $mm, 3)
  }
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
Undo-FpsMouse
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
Undo-FpsMouse
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
Undo-FpsMouse
if (-not (Use-Plan $SAVER)) { [void](Use-Plan $BAL); powercfg /overlaysetactive 961cc777-2547-4f9d-8174-7d86181b8a7a 2>$null | Out-Null }
Set-CpuMax 70
Set-Toasts 1
Say 'Modalità Silenziosa: processore limitato al 70%, meno calore e ventole più tranquille.'
`,
}
