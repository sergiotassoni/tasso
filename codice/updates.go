package main

// Aggiornamento dei programmi con winget (gratuito, già dentro Windows 10/11):
// Tasso mostra quali programmi hanno una versione nuova e li aggiorna uno per uno o tutti insieme.

import (
	"regexp"
	"time"
)

var wingetIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+\-]{1,127}$`)

func updatesList() {
	r := runPS(updatesListScript, 5*time.Minute, func(m string) { emit("onUpdatesProgress", m) })
	res := decodeJSON(r.JSON)
	if res == nil {
		res = map[string]interface{}{"ok": false, "error": "Non sono riuscito a leggere gli aggiornamenti."}
	}
	emit("onUpdates", res)
}

// updatesApply aggiorna i programmi scelti (id winget) oppure tutti ("*").
func updatesApply(ids []string) {
	var ok []string
	all := false
	for _, id := range ids {
		if id == "*" {
			all = true
			break
		}
		if wingetIDRe.MatchString(id) {
			ok = append(ok, id)
		}
	}
	var msgs []string
	if !all && len(ok) == 0 {
		msgs = append(msgs, "Niente da aggiornare.")
	} else {
		args := map[string]interface{}{"all": all, "ids": ok}
		runPSArgs(updatesApplyScript, args, 90*time.Minute, func(m string) { msgs = append(msgs, m); emit("onUpdatesProgress", m) })
	}
	emit("onUpdatesDone", map[string]interface{}{"msgs": msgs})
	updatesList()
}

const updatesListScript = `
$wg = Get-Command winget.exe -ErrorAction SilentlyContinue
if (-not $wg) { [Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject ([ordered]@{ ok = $false; error = 'winget non c''è su questo PC. Si installa gratis dal Microsoft Store: cerca "Programma di installazione app".' }) -Compress)); return }
[Console]::OutputEncoding = [Text.Encoding]::UTF8
$OutputEncoding = [Text.Encoding]::UTF8
Say 'Chiedo a winget quali programmi hanno una versione nuova...'
# Gli ID completi: la tabella di winget a volte li accorcia con "…", l'esportazione no.
$full = @()
$tmp = Join-Path $env:TEMP ('tasso-winget-' + [guid]::NewGuid().ToString('N') + '.json')
try {
  & $wg.Source export -o $tmp --accept-source-agreements --disable-interactivity 2>$null | Out-Null
  if (Test-Path -LiteralPath $tmp) {
    $j = Get-Content -LiteralPath $tmp -Raw -Encoding UTF8 | ConvertFrom-Json
    foreach ($s in @($j.Sources)) { foreach ($p in @($s.Packages)) { if ($p.PackageIdentifier) { $full += [string]$p.PackageIdentifier } } }
  }
} catch {} finally { Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue }
$lines = @(& $wg.Source upgrade --accept-source-agreements --disable-interactivity 2>$null | ForEach-Object { ([string]$_) -replace '^.*\r', '' })
$hi = -1
for ($i = 1; $i -lt $lines.Count; $i++) { if ($lines[$i] -match '^-{10,}\s*$') { $hi = $i - 1; break } }
$items = New-Object System.Collections.ArrayList
if ($hi -ge 0) {
  $h = $lines[$hi]
  $cols = @([regex]::Matches($h, '\S+') | ForEach-Object { $_.Index })
  if ($cols.Count -ge 4) {
    for ($i = $hi + 2; $i -lt $lines.Count; $i++) {
      $l = $lines[$i]
      if (-not $l.Trim()) { break }
      if ($l.Length -lt $cols[3]) { continue }
      $cut = { param($a, $b) if ($a -ge $l.Length) { return '' }; if ($b -lt 0 -or $b -gt $l.Length) { return $l.Substring($a).Trim() }; return $l.Substring($a, $b - $a).Trim() }
      $name = & $cut $cols[0] $cols[1]
      $id = & $cut $cols[1] $cols[2]
      $ver = & $cut $cols[2] $cols[3]
      $av = if ($cols.Count -ge 5) { & $cut $cols[3] $cols[4] } else { & $cut $cols[3] -1 }
      if (-not $id -or -not $av) { continue }
      if ($id.EndsWith([string][char]0x2026)) {
        $pre = $id.TrimEnd([char]0x2026)
        $m = @($full | Where-Object { $_.StartsWith($pre, [StringComparison]::OrdinalIgnoreCase) })
        if ($m.Count -eq 1) { $id = $m[0] } else { $id = '' }
      }
      [void]$items.Add([ordered]@{ name = $name; id = $id; ver = $ver; avail = $av })
    }
  }
}
Say ('Programmi da aggiornare: ' + $items.Count)
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject ([ordered]@{ ok = $true; items = @($items) }) -Compress -Depth 4))
`

const updatesApplyScript = `
$wg = Get-Command winget.exe -ErrorAction SilentlyContinue
if (-not $wg) { Say 'winget non c''è su questo PC.'; return }
$common = '--silent --accept-source-agreements --accept-package-agreements --disable-interactivity'
if ($A.all) {
  Say 'Aggiorno tutti i programmi (può volerci qualche minuto)...'
  $code = Run-Hidden $wg.Source ('upgrade --all ' + $common)
  if (@(0, -1978335189, -1978335135, -1978334963, -1978334967) -contains $code) { Say 'Tutti i programmi sono aggiornati.' } else { Say 'Fatto. Qualche programma va aggiornato a mano (di solito perché è aperto: chiudilo e riprova).' }
  return
}
$ok = 0; $ko = 0
foreach ($id in @($A.ids)) {
  $id = [string]$id
  if ($id -notmatch '^[A-Za-z0-9][A-Za-z0-9._+\-]{1,127}$') { continue }
  Say ('Aggiorno ' + $id + '...')
  $code = Run-Hidden $wg.Source ('upgrade --id "' + $id + '" --exact ' + $common)
  if (@(0, -1978335189, -1978335135, -1978334963, -1978334967) -contains $code) { $ok++; Say ('Aggiornato: ' + $id) } else { $ko++; Say ('Non aggiornato: ' + $id + ' (se è aperto, chiudilo e riprova)') }
}
Say ('Aggiornati ' + $ok + ' programmi' + $(if ($ko) { ', ' + $ko + ' da riprovare.' } else { '.' }))
`
