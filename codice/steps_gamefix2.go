package main

// Altri problemi da giocatore risolvibili con un pulsante: ping, launcher, schermo, FPS,
// Xbox/Game Pass, anti-cheat e Discord.

import "time"

var gameFixSteps2 = []Step{
	{ID: "fix_game_ping", Name: "Ping alto o lag nei giochi online", Desc: "Svuota la memoria degli indirizzi internet, rimette la rete di Windows come deve essere e ferma la condivisione nascosta degli aggiornamenti.", Level: 0, Minutes: 0.4, Timeout: 5 * time.Minute, Script: `
Say 'Svuoto la memoria degli indirizzi internet (DNS)...'
ipconfig /flushdns | Out-Null
Say 'Rimetto la regolazione automatica della rete come la vuole Windows...'
netsh int tcp set global autotuninglevel=normal | Out-Null
# Ottimizzazione recapito: Windows può mandare pezzi di aggiornamenti ad altri PC su internet usando la tua connessione.
# La scelta fatta nelle Impostazioni sta qui (Windows 10 1709 e dopo); quella vecchia in Config.
$doSet = 'Registry::HKEY_USERS\S-1-5-20\Software\Microsoft\Windows\CurrentVersion\DeliveryOptimization\Settings'
$doOld = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\DeliveryOptimization\Config'
$done = $false
foreach ($kv in @(@($doSet, 'DownloadMode'), @($doOld, 'DODownloadMode'))) {
  $m = $null; try { $m = (Get-ItemProperty -LiteralPath $kv[0] -Name $kv[1] -ErrorAction Stop).($kv[1]) } catch {}
  if ($m -eq 3) { try { Set-ItemProperty -LiteralPath $kv[0] -Name $kv[1] -Value 1 -Type DWord -ErrorAction Stop; $done = $true } catch {} }
}
if ($done) { Say 'Windows mandava pezzi di aggiornamenti ad altri PC su internet: ora li condivide solo dentro casa tua.' }
$bits = @(Get-BitsTransfer -AllUsers -ErrorAction SilentlyContinue | Where-Object { [string]$_.JobState -match 'Transferring|Queued' })
if ($bits.Count) { Say ('Ci sono ' + $bits.Count + ' download di Windows in corso: rallentano il gioco online. Lasciali finire o mettili in pausa da Windows Update.') }
$up = @(Get-NetAdapter -ErrorAction SilentlyContinue | Where-Object { $_.Status -eq 'Up' })
$wifi = @($up | Where-Object { $_.PhysicalMediaType -match '802\.11|Wireless' -or $_.InterfaceDescription -match 'Wi-?Fi|Wireless' }).Count -gt 0
$wired = @($up | Where-Object { $_.PhysicalMediaType -match '802\.3' }).Count -gt 0
if ($wifi -and -not $wired) { Say 'Sei collegato in Wi-Fi: per giocare online il cavo di rete è molto meglio (ping più basso e senza salti). Se non puoi, avvicinati al router e usa la rete da 5 GHz.' }
Say 'Fatto. Per vedere quale server di gioco ti risponde più veloce usa Internet > Ping ai server dei giochi. Se il ping resta alto anche con il cavo, spegni il modem, aspetta 30 secondi e riaccendilo.'
`},
	{ID: "fix_game_launcher", Name: "Steam, Epic o altri launcher non si aprono", Desc: "Chiude i launcher bloccati (Steam, Epic, EA, Battle.net, Riot, Ubisoft, GOG) e svuota le loro pagine salvate, la causa più comune delle schermate bianche o nere. Giochi, account e salvataggi non vengono toccati.", Level: 0, Minutes: 0.4, Timeout: 5 * time.Minute, Script: `
$names = 'steam','steamwebhelper','EpicGamesLauncher','EpicWebHelper','EADesktop','EABackgroundService','Battle.net','RiotClientServices','RiotClientUx','UbisoftConnect','upc','GalaxyClient'
$open = @(Get-Process -Name $names -ErrorAction SilentlyContinue)
if ($open.Count) { Say 'Chiudo i launcher aperti o bloccati...'; $open | Stop-Process -Force -ErrorAction SilentlyContinue; Start-Sleep -Seconds 2 }
$steam = ''; try { $steam = [string](Get-ItemProperty -LiteralPath 'HKCU:\Software\Valve\Steam' -Name SteamPath -ErrorAction Stop).SteamPath } catch {}
$dirs = @()
if ($steam) { $steam = $steam -replace '/', '\'; $dirs += (Join-Path $steam 'appcache\httpcache') }
$dirs += (Join-Path $env:LOCALAPPDATA 'Steam\htmlcache')
$dirs += @(Get-ChildItem -LiteralPath (Join-Path $env:LOCALAPPDATA 'EpicGamesLauncher\Saved') -Directory -Filter 'webcache*' -ErrorAction SilentlyContinue | ForEach-Object { $_.FullName })
$dirs += (Join-Path $env:LOCALAPPDATA 'Electronic Arts\EA Desktop\CEF\BrowserCache')
$dirs += (Join-Path $env:LOCALAPPDATA 'Battle.net\BrowserCaches')
$dirs += (Join-Path $env:LOCALAPPDATA 'Riot Games\Riot Client\Cache')
$f = [long]0
foreach ($d in $dirs) { if ($d -and (Test-Path -LiteralPath $d)) { $f += Clear-Folder $d } }
Freed $f
Say ('Pagine salvate dei launcher svuotate: ' + (Fmt $f) + '. Giochi, account e salvataggi non sono stati toccati.')
Say 'Ora riapri il launcher: il primo avvio può essere un po'' più lento. Se Steam resta bianco, riavvia il PC.'
`},
	{ID: "fix_game_screen", Name: "Schermo nero, gioco bloccato o risoluzione sbagliata", Desc: "Prova a far ripartire la scheda video senza riavviare il PC (come Win + Ctrl + Maiusc + B: lo schermo lampeggia) e ti dice i tasti per rimettere a posto lo schermo.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: `
Say 'Faccio ripartire la scheda video: lo schermo diventa nero per un paio di secondi, è normale...'
# Uso la stessa combinazione di Windows (Win + Ctrl + Maiusc + B): riavvia solo il driver video,
# in modo sicuro, senza spegnere le altre schede video del PC.
$ok = $false
try {
  $sig = '[DllImport("user32.dll")] public static extern void keybd_event(byte k, byte s, uint f, System.UIntPtr e);'
  Add-Type -MemberDefinition $sig -Name KB -Namespace TassoKeys -ErrorAction SilentlyContinue | Out-Null
  foreach ($k in @(0x5B, 0x11, 0x10, 0x42)) { [TassoKeys.KB]::keybd_event([byte]$k, 0, 0, [System.UIntPtr]::Zero) }
  Start-Sleep -Milliseconds 80
  foreach ($k in @(0x42, 0x10, 0x11, 0x5B)) { [TassoKeys.KB]::keybd_event([byte]$k, 0, 2, [System.UIntPtr]::Zero) }
  $ok = $true
} catch {}
if ($ok) { Say 'Fatto. Se lo schermo non ha lampeggiato, premi tu insieme Win + Ctrl + Maiusc + B (fa la stessa cosa).' }
else { Say 'Da qui non sono riuscito a far ripartire la scheda video: premi insieme Win + Ctrl + Maiusc + B (fa la stessa cosa).' }
Say 'Consigli: se il gioco è piccolo o tagliato premi Alt + Invio (passa da finestra a schermo intero). Se lo schermo giusto resta nero premi Win + P e scegli "Solo schermo del PC" o "Duplica". Se la risoluzione è strana: tasto destro sul desktop > Impostazioni schermo > Risoluzione consigliata.'
`},
	{ID: "fix_game_fps", Name: "Pochi FPS (il gioco va lento)", Desc: "Mette il piano energetico al massimo, accende la Modalità Gioco e ti dice cosa sta rubando potenza.", Level: 0, Minutes: 0.4, Timeout: 5 * time.Minute, Script: modeBase + `
Use-HighPerf
Set-CpuMax 100
Say 'Piano energetico al massimo e Modalità Gioco accesa.'
$bat = Get-CimInstance Win32_Battery -ErrorAction SilentlyContinue | Select-Object -First 1
if ($bat -and $bat.BatteryStatus -eq 1) { Say 'ATTENZIONE: il portatile va a batteria. A batteria la scheda video va molto più piano: attaccalo alla corrente per giocare.' }
$os = Get-CimInstance Win32_OperatingSystem -ErrorAction SilentlyContinue
if ($os -and $os.TotalVisibleMemorySize) {
  $pct = [math]::Round(100 - ($os.FreePhysicalMemory / $os.TotalVisibleMemorySize * 100))
  if ($pct -ge 80) { Say ('La memoria (RAM) è piena al ' + $pct + ' per cento: chiudi i programmi che non usi (per esempio il browser con tante schede) o usa "Ottimizza prima di giocare" in Giochi.') }
}
$top = @(Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -notmatch '^(Idle|System|Tasso|dwm|csrss|explorer|Registry|MemCompression|svchost)$' } | Sort-Object WorkingSet64 -Descending | Select-Object -First 3)
if ($top.Count) { Say ('I programmi che usano più memoria adesso: ' + (($top | ForEach-Object { $_.ProcessName + ' (' + [math]::Round($_.WorkingSet64 / 1MB) + ' MB)' }) -join ', ') + '.') }
Say 'Fatto. Altri consigli: nel gioco abbassa "Ombre" e "Risoluzione di rendering" (sono le opzioni che pesano di più) e usa "Regola con Tasso" in Giochi. Per tenere tutto così anche dopo il riavvio accendi il Profilo gioco.'
`},
	{ID: "fix_game_xbox", Name: "App Xbox e Game Pass non funzionano", Desc: "Riavvia i servizi Xbox di Windows e ripara l'app Xbox e lo Store. Utile quando i giochi Game Pass non si installano, non partono o chiedono sempre l'accesso.", Level: 0, Minutes: 1, Timeout: 10 * time.Minute, Script: `
Say 'Chiudo l''app Xbox e lo Store...'
Get-Process -Name XboxPcApp,XboxApp,WinStore.App -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Say 'Riavvio i servizi Xbox di Windows (accesso, rete e Gaming Services)...'
foreach ($s in 'XblAuthManager','XblGameSave','XboxNetApiSvc','XboxGipSvc','GamingServices','GamingServicesNet','InstallService') {
  $svc = Get-Service -Name $s -ErrorAction SilentlyContinue
  if (-not $svc) { continue }
  if ([string]$svc.StartType -eq 'Disabled') {
    $want = 'Manual'; if ($s -like 'GamingServices*') { $want = 'Automatic' }
    Set-Service -Name $s -StartupType $want -ErrorAction SilentlyContinue
    $now = [string](Get-Service -Name $s -ErrorAction SilentlyContinue).StartType
    if ($now -ne 'Disabled') { Say ('Il servizio ' + $s + ' era spento del tutto: l''ho rimesso come lo vuole Windows.') }
    elseif ($s -like 'GamingServices*') { Say 'Gaming Services è spento e Windows non mi lascia riaccenderlo: reinstallalo dallo Store (ti apro la pagina).'; Start-Process 'ms-windows-store://pdp/?ProductId=9MWPM2CQNLHN' }
    else { Say ('Non sono riuscito a riaccendere il servizio ' + $s + '.') }
  }
  Restart-Service -Name $s -Force -ErrorAction SilentlyContinue
}
if (-not (Get-Service -Name GamingServices -ErrorAction SilentlyContinue)) {
  Say 'Manca "Gaming Services", il pezzo di Windows che installa i giochi Game Pass: ti apro la sua pagina ufficiale nello Store, premi Installa.'
  Start-Process 'ms-windows-store://pdp/?ProductId=9MWPM2CQNLHN'
}
Say 'Ripasso l''app Xbox e i suoi componenti...'
foreach ($n in 'Microsoft.GamingApp','Microsoft.XboxIdentityProvider','Microsoft.Xbox.TCUI','Microsoft.XboxGamingOverlay','Microsoft.WindowsStore') {
  Get-AppxPackage -Name $n -ErrorAction SilentlyContinue | ForEach-Object { Add-AppxPackage -DisableDevelopmentMode -Register (Join-Path $_.InstallLocation 'AppXManifest.xml') -ErrorAction SilentlyContinue }
}
try { Start-Process -FilePath 'wsreset.exe' -WindowStyle Hidden -ErrorAction Stop; Start-Sleep -Seconds 4; Get-Process -Name WinStore.App -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue } catch {}
Say ('Controlla anche data e ora del PC (ora segna ' + (Get-Date).ToString('dd/MM/yyyy HH:mm') + '): se sono sbagliate l''accesso a Xbox non funziona.')
Say 'Fatto. Riapri l''app Xbox e rifai l''accesso se te lo chiede. Se un gioco non si installa ancora, riavvia il PC.'
`},
	{ID: "fix_game_anticheat", Name: "Errore dell'anti-cheat (Vanguard, Easy Anti-Cheat, BattlEye)", Desc: "Controlla i programmi anti-trucchi dei giochi online (LoL, Valorant, Fortnite, Apex, Rainbow Six…) e riaccende i loro servizi se un altro programma li ha spenti. I loro file non vengono mai toccati.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: `
$found = 0
$list = @(
  @('vgc', 'Riot Vanguard (League of Legends, Valorant)'),
  @('EasyAntiCheat', 'Easy Anti-Cheat'),
  @('EasyAntiCheat_EOS', 'Easy Anti-Cheat (giochi Epic, es. Fortnite)'),
  @('BEService', 'BattlEye'),
  @('FACEIT', 'FACEIT')
)
foreach ($a in $list) {
  $s = Get-Service -Name $a[0] -ErrorAction SilentlyContinue
  if (-not $s) { continue }
  $found++
  # Riot dice che Vanguard (vgc) deve partire in automatico; gli altri partono quando il gioco li chiama.
  $want = 'Manual'; if ($a[0] -eq 'vgc') { $want = 'Automatic' }
  $st = [string]$s.StartType
  $bad = ($st -eq 'Disabled') -or ($a[0] -eq 'vgc' -and $st -ne 'Automatic')
  if ($bad) {
    Set-Service -Name $a[0] -StartupType $want -ErrorAction SilentlyContinue
    $now = [string](Get-Service -Name $a[0] -ErrorAction SilentlyContinue).StartType
    if ($now -eq $want) { Say ($a[1] + ': non partiva come deve (lo fanno alcuni programmi "ottimizzatori"). Sistemato: riavvia il PC.'); [Console]::Out.WriteLine('REBOOT') }
    else { Say ($a[1] + ': non riesco a sistemarlo da qui. Reinstallalo dal launcher del gioco.') }
  } else { Say ($a[1] + ': a posto.') }
}
if (Get-Service -Name vgc -ErrorAction SilentlyContinue) {
  $vk = 'HKLM:\SYSTEM\CurrentControlSet\Services\vgk'
  if (-not (Test-Path -LiteralPath $vk)) { Say 'Vanguard è installato a metà: apri il Riot Client, ti chiederà di reinstallarlo, poi riavvia il PC.' }
  else {
    $ks = $null; try { $ks = (Get-ItemProperty -LiteralPath $vk -Name Start -ErrorAction Stop).Start } catch {}
    if ($ks -eq 4) {
      [void](Run-Hidden 'sc.exe' 'config vgk start= system')
      Say 'Il pezzo di Vanguard che parte con Windows (vgk) era spento: l''ho riacceso. Riavvia il PC, altrimenti Valorant e LoL danno errore VAN.'; [Console]::Out.WriteLine('REBOOT')
    } else { Say 'Vanguard parte insieme a Windows: dopo un errore VAN riavvia sempre il PC.' }
  }
}
if ($found -eq 0) { Say 'Su questo PC non ho trovato anti-cheat installati: il gioco lo installa da solo al primo avvio.' }
Say 'Se l''errore resta: ripara il gioco dal suo launcher (Steam: Verifica integrità dei file; Epic: Verifica; Riot: reinstalla Vanguard). Tasso non tocca mai i file dell''anti-cheat, per non rischiare il ban.'
`},
	{ID: "fix_discord", Name: "Discord non si apre, non si aggiorna o non ti sentono", Desc: "Chiude Discord bloccato, svuota la sua cache (non i messaggi né l'accesso) e controlla che il microfono non sia bloccato da Windows.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: `
$p = @(Get-Process -Name Discord,DiscordPTB,DiscordCanary,Update -ErrorAction SilentlyContinue | Where-Object { $_.Path -match 'Discord' })
if ($p.Count) { Say 'Chiudo Discord bloccato...'; $p | Stop-Process -Force -ErrorAction SilentlyContinue; Start-Sleep -Seconds 2 }
$f = [long]0
foreach ($v in 'discord','discordptb','discordcanary') {
  $base = Join-Path $env:APPDATA $v
  foreach ($c in 'Cache','Code Cache','GPUCache','DawnCache','DawnWebGPUCache') { $d = Join-Path $base $c; if (Test-Path -LiteralPath $d) { $f += Clear-Folder $d } }
}
Freed $f
Say ('Cache di Discord svuotata: ' + (Fmt $f) + '. Messaggi, server e accesso restano.')
$mk = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\microphone'
$v = ''; try { $v = [string](Get-ItemProperty -LiteralPath $mk -Name Value -ErrorAction Stop).Value } catch {}
$nd = ''; try { $nd = [string](Get-ItemProperty -LiteralPath ($mk + '\NonPackaged') -Name Value -ErrorAction Stop).Value } catch {}
if ($v -eq 'Deny' -or $nd -eq 'Deny') { Say 'ATTENZIONE: Windows blocca il microfono ai programmi come Discord. Ti apro l''impostazione: accendi "Accesso al microfono" e "Consenti alle app desktop di accedere al microfono".'; Start-Process 'ms-settings:privacy-microphone' }
Say 'Ora riapri Discord dal menu Start: al primo avvio controlla gli aggiornamenti, aspetta qualche secondo.'
Say 'Se resta bloccato su "Aggiornamento": disinstallalo da Programmi e riscaricalo da discord.com (il sito ufficiale). Se non ti sentono: in Discord > Impostazioni > Voce e video scegli il microfono giusto.'
`},
}

func init() {
	insertStepAfter("fix_game_pads", gameFixSteps2...)
}
