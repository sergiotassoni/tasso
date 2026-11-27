package main

// Funzioni 2.4: backup di foto e documenti con un clic, collegamento sul Desktop.

import (
	"os"
	"time"
)

// backup copia foto, documenti, desktop, video e musica in una cartella scelta dall'utente
// (una chiavetta o un disco esterno). Copia, non sposta: gli originali restano.
func backup() {
	var msgs []string
	r := runPS(backupScript, 3*time.Hour, func(m string) { msgs = append(msgs, m); emit("onBackupProgress", m) })
	emit("onBackupDone", map[string]interface{}{"msgs": msgs, "result": decodeJSON(r.JSON)})
}

// desktopShortcut crea un collegamento a Tasso sul Desktop.
func desktopShortcut() string {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return "no"
	}
	var last string
	r := runPSArgs(desktopShortcutScript, map[string]interface{}{"exe": exe}, time.Minute, func(m string) { last = m })
	if r.JSON == "ok" {
		// Il collegamento nuovo prende subito l'icona del tema in uso (non solo alla prossima apertura).
		go func() {
			defer guard("icone-collegamenti")
			aggiornaCollegamenti(idIconaInUso())
		}()
		return "ok"
	}
	if r.JSON == "gia" {
		return "gia"
	}
	if last == "" {
		last = "Non sono riuscito a creare il collegamento."
	}
	return last
}

const backupScript = `
$sh = New-Object -ComObject Shell.Application
$f = $sh.BrowseForFolder(0, 'Scegli dove salvare il backup (una chiavetta o un disco esterno)', 0x11)
if (-not $f) { Say 'Backup annullato.'; [Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject ([ordered]@{ ok = $false }) -Compress)); return }
$dest = ''
try { $dest = [string]$f.Self.Path } catch {}
if (-not $dest -or -not (Test-Path -LiteralPath $dest)) { Say 'Cartella non valida.'; [Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject ([ordered]@{ ok = $false }) -Compress)); return }
# La cartella del backup non può stare dentro quelle da copiare (per es. il Desktop): robocopy
# copierebbe il backup dentro sé stesso fino a riempire il disco.
$destFull = [IO.Path]::GetFullPath($dest).TrimEnd('\') + '\'
foreach ($s in @('MyPictures', 'MyDocuments', 'Desktop', 'MyVideos', 'MyMusic')) {
  $p = [Environment]::GetFolderPath($s)
  if ($p -and $destFull.StartsWith(([IO.Path]::GetFullPath($p).TrimEnd('\') + '\'), [StringComparison]::OrdinalIgnoreCase)) {
    Say 'Questa cartella è dentro quelle da copiare (Documenti, Desktop, Immagini, Video o Musica): scegli una chiavetta o un disco esterno.'
    [Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject ([ordered]@{ ok = $false }) -Compress)); return
  }
}
$stamp = Get-Date -Format 'yyyy-MM-dd'
$out = Join-Path $dest ('Backup_Tasso_' + $stamp)
New-Item -ItemType Directory -Path $out -Force | Out-Null
$n = 0; $fail = @()
foreach ($s in @('MyPictures', 'MyDocuments', 'Desktop', 'MyVideos', 'MyMusic')) {
  $p = [Environment]::GetFolderPath($s)
  if (-not $p -or -not (Test-Path -LiteralPath $p)) { continue }
  # Il nome che si vede in Esplora file (Immagini, Documenti...), non quello inglese della cartella vera.
  $name = ''
  try { $name = [string](New-Object -ComObject Shell.Application).Namespace($p).Self.Name } catch {}
  $name = ($name -replace '[\\/:*?"<>|]', '_').Trim()
  if (-not $name) { $name = Split-Path $p -Leaf }
  Say ('Copio ' + $name + '...')
  $d = Join-Path $out $name
  # /XA:SHO: niente file nascosti o di sistema, e niente file "solo online" di OneDrive
  # (altrimenti li scaricherebbe tutti e riempirebbe il disco).
  $code = 16
  try { & robocopy.exe $p $d /E /R:1 /W:1 /NFL /NDL /NJH /NJS /XA:SHO /XJ 2>$null | Out-Null; $code = $LASTEXITCODE } catch {}
  if ($code -lt 8) { $n++ } else { $fail += $name; Say ('Attenzione: alcuni file di ' + $name + ' non sono stati copiati (disco pieno, file troppo grandi per la chiavetta, o file in uso).') }
}
if ($fail.Count) { Say ('Backup finito, ma NON completo: controlla ' + ($fail -join ', ') + '. Le copie sono in: ' + $out) }
else { Say ('Backup finito. Copiato tutto (tranne i file nascosti e di sistema) in: ' + $out) }
Say 'I file di OneDrive che sono solo online (nuvoletta) non vengono copiati: sono già al sicuro su OneDrive.'
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject ([ordered]@{ ok = ($fail.Count -eq 0); path = $out; folders = $n }) -Compress))
`

const desktopShortcutScript = `
$exe = [string]$A.exe
if (-not $exe -or -not (Test-Path -LiteralPath $exe)) { Say 'Non trovo il programma.'; return }
try {
  $ws = New-Object -ComObject WScript.Shell
  # Se sul Desktop c'è già un collegamento a questo Tasso (anche quelli dell'installatore), non ne faccio un doppione.
  $full = [IO.Path]::GetFullPath($exe)
  foreach ($dk in @([Environment]::GetFolderPath('Desktop'), [Environment]::GetFolderPath('CommonDesktopDirectory'))) {
    if (-not $dk) { continue }
    foreach ($nm in @('Tasso Gaming.lnk', 'Tasso Ufficio.lnk', 'Tasso.lnk')) {
      $q = Join-Path $dk $nm
      if (-not (Test-Path -LiteralPath $q)) { continue }
      $t = ''; try { $t = [string]$ws.CreateShortcut($q).TargetPath } catch {}
      if ($t -and [string]::Equals([IO.Path]::GetFullPath($t), $full, [StringComparison]::OrdinalIgnoreCase)) { [Console]::Out.WriteLine('JSON gia'); return }
    }
  }
  $lnk = Join-Path ([Environment]::GetFolderPath('Desktop')) 'Tasso.lnk'
  $s = $ws.CreateShortcut($lnk)
  $s.TargetPath = $exe
  $s.IconLocation = $exe + ',0'
  $s.Description = 'Tasso - pulisce, ripara e velocizza il PC'
  $s.Save()
  if (-not (Test-Path -LiteralPath $lnk)) { throw 'no' }
  Say 'Collegamento a Tasso creato sul Desktop.'
  [Console]::Out.WriteLine('JSON ok')
} catch { Say 'Non sono riuscito a creare il collegamento.' }
`
