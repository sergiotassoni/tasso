package main

// Aggiornamento dei programmi con winget (gratuito, già dentro Windows 10/11):
// Tasso mostra quali programmi hanno una versione nuova e li aggiorna uno per uno o tutti insieme.

import (
	"regexp"
	"strings"
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
	// La lista è già sullo schermo: le icone vere dei programmi arrivano dopo (icone_programmi.go).
	if m, ok := res.(map[string]interface{}); ok {
		iconeAggiornamenti(m)
	}
}

// updatesApply aggiorna solo i programmi scelti: id di winget, oppure "n:" e il nome esatto per quelli
// che winget mostra senza id. Mai "upgrade --all": aggiornerebbe anche quelli senza spunta.
func updatesApply(ids []string) {
	var ok, nomi []string
	for _, id := range ids {
		if n, isNome := strings.CutPrefix(id, "n:"); isNome {
			if nomeWingetValido(n) {
				nomi = append(nomi, n)
			}
			continue
		}
		if wingetIDRe.MatchString(id) {
			ok = append(ok, id)
		}
	}
	var msgs []string
	if len(ok) == 0 && len(nomi) == 0 {
		msgs = append(msgs, "Niente da aggiornare.")
	} else {
		args := map[string]interface{}{"ids": ok, "names": nomi}
		runPSArgs(updatesApplyScript, args, 90*time.Minute, func(m string) { msgs = append(msgs, m); emit("onUpdatesProgress", m) })
	}
	emit("onUpdatesDone", map[string]interface{}{"msgs": msgs})
	updatesList()
}

// nomeWingetValido: un nome di programma che si può passare a "winget upgrade --name ... --exact".
// No nomi accorciati da winget (…), virgolette, a capo o caratteri di controllo.
func nomeWingetValido(n string) bool {
	if n == "" || n != strings.TrimSpace(n) || len(n) > 200 || strings.ContainsAny(n, "\"\u2026`") {
		return false
	}
	for _, r := range n {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

const updatesListScript = `
$wg = Get-Winget
if (-not $wg) { [Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject ([ordered]@{ ok = $false; error = 'winget non c''è su questo PC. Si installa gratis dal Microsoft Store: cerca "Programma di installazione app".' }) -Compress)); return }
[Console]::OutputEncoding = [Text.Encoding]::UTF8
$OutputEncoding = [Text.Encoding]::UTF8
Say 'Chiedo a winget quali programmi hanno una versione nuova...'
# Gli ID completi: la tabella di winget a volte li accorcia con "…", l'esportazione no.
$full = @()
$tmp = Join-Path $env:TEMP ('tasso-winget-' + [guid]::NewGuid().ToString('N') + '.json')
try {
  & $wg export -o $tmp --accept-source-agreements --disable-interactivity 2>$null | Out-Null
  if (Test-Path -LiteralPath $tmp) {
    $j = Get-Content -LiteralPath $tmp -Raw -Encoding UTF8 | ConvertFrom-Json
    foreach ($s in @($j.Sources)) { foreach ($p in @($s.Packages)) { if ($p.PackageIdentifier) { $full += [string]$p.PackageIdentifier } } }
  }
} catch {} finally { Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue }
$lines = @(& $wg upgrade --accept-source-agreements --disable-interactivity 2>$null | ForEach-Object { ([string]$_) -replace '^.*\r', '' })
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
$wg = Get-Winget
if (-not $wg) { Say 'winget non c''è su questo PC.'; return }
$common = '--silent --accept-source-agreements --accept-package-agreements --disable-interactivity'
# Codici di winget: 0 fatto; 0x8A150109 fatto ma serve un riavvio; 0x8A150061 / 0x8A15010D il
# programma è già alla versione nuova; 0x8A15002B winget non ha un aggiornamento che può installare
# (di solito il programma si aggiorna da solo o va aggiornato dal suo sito). Solo 0 e 0x8A150109
# vogliono dire "aggiornato davvero".
function Esito-Winget([int]$c) {
  switch ($c) {
    0 { return 'ok' }
    -1978334967 { return 'riavvio' }
    -1978335135 { return 'gia' }
    -1978334963 { return 'gia' }
    -1978335189 { return 'nonapplicabile' }
    default { return 'no' }
  }
}
$ok = 0; $ko = 0; $na = 0
# Un programma alla volta: $sel è l'argomento per winget (--id o --name, sempre --exact), $nome quello da mostrare.
function Aggiorna-Uno([string]$sel, [string]$nome) {
  Say ('Aggiorno ' + $nome + '...')
  $code = Run-Hidden $wg ('upgrade ' + $sel + ' --exact ' + $common)
  switch (Esito-Winget $code) {
    'ok' { $script:ok++; Say ('Aggiornato: ' + $nome) }
    'riavvio' { $script:ok++; Say ('Aggiornato: ' + $nome + ' (per finire serve un riavvio)'); [Console]::Out.WriteLine('REBOOT') }
    'gia' { $script:ok++; Say ('Già alla versione nuova: ' + $nome) }
    'nonapplicabile' { $script:na++; Say ('Non aggiornato: ' + $nome + ' (winget non ha un aggiornamento che può installare: aggiornalo dal programma stesso o dal suo sito)') }
    default { $script:ko++; Say ('Non aggiornato: ' + $nome + ' (se è aperto, chiudilo e riprova)') }
  }
}
foreach ($id in @($A.ids)) {
  $id = [string]$id
  if ($id -notmatch '^[A-Za-z0-9][A-Za-z0-9._+\-]{1,127}$') { continue }
  Aggiorna-Uno ('--id "' + $id + '"') $id
}
# Programmi che winget mostra senza id: per nome esatto (se il nome vale per più programmi winget non fa niente).
foreach ($n in @($A.names)) {
  $n = [string]$n
  if (-not $n -or $n -match '["\r\n]' -or $n.Contains([string][char]0x2026)) { continue }
  Aggiorna-Uno ('--name "' + $n + '"') $n
}
$resto = @()
if ($na) { $resto += ($na.ToString() + ' da aggiornare dal programma stesso') }
if ($ko) { $resto += ($ko.ToString() + ' da riprovare') }
Say ('Aggiornati ' + $ok + ' programmi' + $(if ($resto.Count) { ', ' + ($resto -join ', ') + '.' } else { '.' }))
`
