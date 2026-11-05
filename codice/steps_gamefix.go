package main

// Tre problemi da giocatore che si risolvono con un pulsante (Risolvi problemi).

import "time"

var gameFixSteps = []Step{
	{ID: "fix_game_lag", Name: "Il gioco va a scatti", Desc: "Accende la Modalità Gioco di Windows, controlla se il PC sta risparmiando energia e ti dice cosa fare. Le cache della grafica non le cancella: servono proprio a non scattare.", Level: 0, Minutes: 0.5, Timeout: 10 * time.Minute, Script: `
$gb = 'HKCU:\Software\Microsoft\GameBar'
if (-not (Test-Path $gb)) { New-Item -Path $gb -Force | Out-Null }
New-ItemProperty -Path $gb -Name 'AutoGameModeEnabled' -Value 1 -PropertyType DWord -Force | Out-Null
Say 'Modalità Gioco di Windows accesa: mentre giochi Windows non fa aggiornamenti pesanti.'
# "Limitazione dell'alimentazione": su alcuni PC Windows rallenta i programmi che pensa siano in secondo piano.
$pt = 'HKLM:\SYSTEM\CurrentControlSet\Control\Power\PowerThrottling'
$cur = $null; try { $cur = (Get-ItemProperty -LiteralPath $pt -Name PowerThrottlingOff -ErrorAction Stop).PowerThrottlingOff } catch {}
if ($cur -ne 1) { Say 'Su questo PC Windows può rallentare i programmi in secondo piano per risparmiare: se giochi con il portatile, attaccalo alla corrente e metti la modalità Gaming in Tasso.' }
$s = (powercfg /getactivescheme) -join ' '
$ov = ''; try { $ov = [string](Get-ItemProperty -LiteralPath 'HKLM:\SYSTEM\CurrentControlSet\Control\Power\User\PowerSchemes' -Name ActiveOverlayAcPowerScheme -ErrorAction Stop).ActiveOverlayAcPowerScheme } catch {}
if ($s -notmatch '8c5e7fda|Prestazioni elevate|High performance|Ultimate|Prestazioni ottimali' -and $ov -notmatch 'ded574b5') { Say 'Consiglio: in Tasso metti la modalità Gaming (o accendi il Profilo gioco): il piano energetico ora non è al massimo.' }
Say 'Le cache della grafica le lascio: servono proprio a non scattare. Solo se gli scatti o gli errori grafici sono iniziati dopo un aggiornamento del driver, usa "Cache della scheda video" in Tutti gli strumenti.'
Say 'Altre cause frequenti: il gioco installato su un disco meccanico (meglio un SSD), un programma che registra in sottofondo, o il portatile a batteria.'
`},
	{ID: "fix_game_start", Name: "Il gioco non parte o si chiude da solo", Desc: "Installa o ripara i componenti che servono ai giochi (Visual C++, DirectX, .NET) dal sito ufficiale Microsoft.", Level: 0, Minutes: 4, Timeout: 30 * time.Minute, Script: `
$wg = Get-Command winget.exe -ErrorAction SilentlyContinue
if (-not $wg) {
  Say 'winget non c''è su questo PC: ti apro la pagina ufficiale Microsoft dei componenti Visual C++.'
  Start-Process -FilePath "$env:SystemRoot\explorer.exe" -ArgumentList 'https://learn.microsoft.com/it-it/cpp/windows/latest-supported-vc-redist'
  return
}
$pk = @(
  @('Microsoft.VCRedist.2015+.x64', 'Visual C++ 2015-2022 (64 bit)'),
  @('Microsoft.VCRedist.2015+.x86', 'Visual C++ 2015-2022 (32 bit)'),
  @('Microsoft.DirectX', 'DirectX (librerie dei giochi più vecchi)'),
  @('Microsoft.DotNet.DesktopRuntime.8', '.NET Desktop Runtime 8')
)
$ok = 0
foreach ($p in $pk) {
  Say ('Controllo ' + $p[1] + '...')
  $code = Run-Hidden $wg.Source ('install --id "' + $p[0] + '" --exact --silent --force --accept-source-agreements --accept-package-agreements --disable-interactivity')
  if (@(0, -1978335189, -1978335135, -1978334963, -1978334967) -contains $code) { $ok++; Say ('A posto: ' + $p[1]) } else { Say ('Non sono riuscito a sistemare ' + $p[1] + ' (codice ' + $code + ').') }
}
Say ('Componenti dei giochi a posto: ' + $ok + ' su ' + $pk.Count + '.')
Say 'Se il gioco non parte ancora: su Steam fai tasto destro sul gioco > Proprietà > File installati > "Verifica integrità dei file"; su Epic: tre puntini > Gestisci > Verifica. Ricontrollano e riscaricano solo i file rotti.'
`},
	{ID: "fix_game_pads", Name: "Controller, cuffie o microfono non vanno", Desc: "Riavvia controller, cuffie, microfono e periferiche USB o Bluetooth che non rispondono, e riavvia l'audio di Windows.", Level: 0, Minutes: 0.5, Timeout: 5 * time.Minute, Script: `
Say 'Cerco controller, cuffie e periferiche USB con problemi...'
$cls = 'HIDClass|USB|MEDIA|AudioEndpoint|XnaComposite|XboxComposite|Bluetooth|Mouse|Keyboard'
$bad = @(Get-CimInstance Win32_PnPEntity -ErrorAction SilentlyContinue | Where-Object { $_.PNPClass -match $cls -and $_.ConfigManagerErrorCode -ne 0 -and $_.ConfigManagerErrorCode -ne 22 -and $_.ConfigManagerErrorCode -ne 45 })
foreach ($d in $bad) {
  Say ('Riavvio: ' + $d.Name)
  $id = [string]$d.PNPDeviceID
  if ($id -match '^[A-Za-z0-9\\&_.{}#\- ]{3,300}$') { [void](Run-Hidden 'pnputil.exe' ('/restart-device "' + $id + '"')) }
}
if ($bad.Count -eq 0) { Say 'Nessuna periferica risulta rotta per Windows.' }
Say 'Faccio ricercare a Windows le periferiche collegate...'
[void](Run-Hidden 'pnputil.exe' '/scan-devices')
Say 'Riavvio l''audio di Windows (l''audio si ferma per un secondo)...'
Restart-Service -Name AudioEndpointBuilder -Force -ErrorAction SilentlyContinue
Start-Service -Name Audiosrv -ErrorAction SilentlyContinue
Say 'Riavvio anche il Bluetooth: cuffie e controller Bluetooth si staccano per qualche secondo e poi si ricollegano da soli.'
Restart-Service -Name bthserv -Force -ErrorAction SilentlyContinue
# Microfono bloccato nelle impostazioni della privacy: è la causa più comune del "non mi sentono".
$mk = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\microphone'
$v = ''; try { $v = [string](Get-ItemProperty -LiteralPath $mk -Name Value -ErrorAction Stop).Value } catch {}
if ($v -eq 'Deny') { Say 'ATTENZIONE: il microfono è bloccato per tutte le app in Impostazioni > Privacy > Microfono. Accendilo lì per farti sentire su Discord e nei giochi.'; Start-Process 'ms-settings:privacy-microphone' }
$still = @(Get-CimInstance Win32_PnPEntity -ErrorAction SilentlyContinue | Where-Object { $_.PNPClass -match $cls -and $_.ConfigManagerErrorCode -ne 0 -and $_.ConfigManagerErrorCode -ne 22 -and $_.ConfigManagerErrorCode -ne 45 })
if ($still.Count) { Say ('Ancora con problemi: ' + (($still | ForEach-Object { $_.Name }) -join ', ') + '. Scollegala, aspetta 5 secondi e ricollegala (meglio in una porta USB dietro il PC). Poi guarda "Driver di tutto il PC" qui sotto.') }
else { Say 'Fatto. Se un controller o le cuffie non vanno ancora: scollegale e ricollegale, e controlla che siano scelte come dispositivo nel gioco o su Discord.' }
`},
}

func init() {
	insertStepAfter("fix_search", gameFixSteps...)
}
