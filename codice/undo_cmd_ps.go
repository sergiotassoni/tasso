package main

// undoCmdPS: aiuto per le riparazioni che cambiano servizi o impostazioni di rete (che non stanno
// nel registro dell'utente): prima di cambiare, scrivono in %LOCALAPPDATA%\Tasso\backup-registro un
// file .cmd con i comandi che rimettono com'era. "Torna indietro" lo mette nell'elenco dei file per
// tornare come prima (fileDaRimettere); con un doppio clic chiede da solo il permesso di
// amministratore (servizi e rete lo vogliono) e rimette i valori di prima.
const undoCmdPS = `
function New-UndoCmd([string[]]$cmds, [string]$label, [string]$done) {
  try {
    $d = Join-Path $env:LOCALAPPDATA 'Tasso\backup-registro'
    New-Item -ItemType Directory -Path $d -Force -ErrorAction Stop | Out-Null
    $f = Join-Path $d ($label + '_' + (Get-Date -Format 'yyyyMMdd_HHmmss') + '.cmd')
    $head = @('@echo off', 'chcp 65001 >nul', 'net session >nul 2>&1', 'if errorlevel 1 (', '  set "TASSO_UNDO=%~f0"', '  powershell.exe -NoProfile -Command "Start-Process -FilePath $env:TASSO_UNDO -Verb RunAs"', '  exit /b', ')')
    [IO.File]::WriteAllLines($f, [string[]]($head + $cmds + @(('echo ' + $done), 'pause')), (New-Object Text.UTF8Encoding $false))
    if (Test-Path -LiteralPath $f) { return $f }
  } catch {}
  return ''
}
function Get-ScStart([string]$t) {
  # Tipo di avvio di Get-Service -> valore di "sc config ... start=".
  switch ($t) { 'Disabled' { return 'disabled' } 'Manual' { return 'demand' } 'Automatic' { return 'auto' } 'Boot' { return 'boot' } 'System' { return 'system' } }
  return ''
}
function Say-UndoCmd([string]$f) {
  if ($f) { Say ('Per tornare come prima: doppio clic su ' + $f) }
}
`
