package main

// Tre problemi da gioco in più (2.6): mira del mouse imprecisa, gioco che si riduce a icona da
// solo, download dei giochi lenti. Usano gli aiuti della modalità Ufficio (copia di sicurezza
// con file .reg per tornare indietro).

import "time"

var gameFixSteps3 = []Step{
	{ID: "fix_game_mouse", Name: "La mira con il mouse non è precisa", Desc: "Spegne la \"precisione del puntatore\" di Windows (è un'accelerazione: lo stesso movimento sposta la mira di più o di meno a seconda della velocità). Così la mira è sempre uguale. Puoi tornare indietro.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$k = 'HKCU\Control Panel\Mouse'
if ([string](Get-RegValue $k 'MouseSpeed') -eq '0') { Say 'La precisione del puntatore era già spenta: la mira non ha accelerazione da parte di Windows.' }
else {
  $u = Save-Undo $k @('MouseSpeed', 'MouseThreshold1', 'MouseThreshold2') 'mouse-accelerazione'
  if (-not $u -or -not (Use-Input)) { Say 'Windows non mi ha lasciato cambiarla: Impostazioni > Mouse > Impostazioni aggiuntive > Opzioni puntatore > togli "Aumenta precisione puntatore".'; Open-User 'ms-settings:mousetouchpad'; return }
  [TassoInput]::Accel($false)
  Say 'Fatto: spenta la "precisione del puntatore". Ora muovere il mouse di 5 cm sposta la mira sempre della stessa quantità, piano o veloce.'
  Say 'All''inizio può sembrarti diverso: dopo qualche partita la mira diventa più costante. Regola la sensibilità nel gioco o nel programma del mouse (DPI).'
  Say-Undo $u
  Say '(Dopo il doppio clic sul file, esci e rientra in Windows.)'
}
Say 'Altri consigli: nel gioco attiva "Raw input" o "Input diretto" se c''è; usa il mouse con il cavo o un ricevitore vicino; tappetino pulito.'
`)},
	{ID: "fix_game_minimize", Name: "Il gioco si riduce a icona da solo", Desc: "Impedisce agli altri programmi di \"rubare\" la finestra mentre giochi (impostazione di Windows rovinata da qualche programma) e ti dice quali programmi aperti lo fanno di solito. Puoi tornare indietro.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$k = 'HKCU\Control Panel\Desktop'
$v = Get-RegValue $k 'ForegroundLockTimeout'
if ($v -ne $null -and [long]$v -lt 200000) {
  $u = Save-Undo $k @('ForegroundLockTimeout') 'finestra-in-primo-piano'
  Set-RegValue $k 'ForegroundLockTimeout' 200000 'DWord'
  try { Add-Type -Namespace TassoFg -Name F -MemberDefinition '[DllImport("user32.dll")] public static extern bool SystemParametersInfoW(uint a, uint b, IntPtr c, uint d);' -ErrorAction Stop; [void][TassoFg.F]::SystemParametersInfoW(0x2001, 0, [IntPtr]200000, 3) } catch {}
  Say 'Trovata la causa: un programma aveva tolto a Windows la protezione che impedisce di "rubare" la finestra in primo piano. L''ho rimessa come di fabbrica, vale da subito.'
  Say-Undo $u
} else { Say 'La protezione di Windows contro chi "ruba" la finestra è a posto.' }
$known = [ordered]@{ 'Razer Synapse' = 'Razer Synapse.*|RazerCentral|RazerAppEngine'; 'Armoury Crate' = 'ArmouryCrate|ArmourySocketServer'; 'iCUE' = 'iCUE'; 'Logitech G HUB' = 'lghub'; 'MSI Center / Dragon Center' = 'MSI.CentralServer|DCv2|MSI_Center'; 'NVIDIA overlay' = 'NVIDIA Overlay'; 'Overwolf' = 'Overwolf'; 'Wallpaper Engine' = 'wallpaper32|wallpaper64'; 'Programma della stampante HP' = 'HPPrintScanDoctor|HPSmart|HP\.Smart'; 'Aggiornamenti Java / Adobe' = 'jusched|AdobeARM' }
$names = @(Get-Process -ErrorAction SilentlyContinue | ForEach-Object { $_.ProcessName })
$hit = @(); foreach ($n in $known.Keys) { if (@($names | Where-Object { $_ -match ('^(' + $known[$n] + ')$') }).Count) { $hit += $n } }
if ($hit.Count) { Say ('Programmi aperti che spesso fanno uscire dal gioco (con finestre o avvisi): ' + ($hit -join ', ') + '. Se succede ancora, chiudili prima di giocare o spegnili da Velocità > Programmi all''avvio.') }
Say 'Altre cause: notifiche (Windows le blocca da sola mentre giochi a schermo intero), il tasto Xbox del controller che apre la Game Bar, e i giochi in "finestra senza bordi": prova "schermo intero" nelle opzioni video.'
`)},
	{ID: "fix_game_download", Name: "I download dei giochi sono lenti", Desc: "Controlla cosa rallenta i download di Steam, Epic e Xbox (rete \"a consumo\", limiti di velocità, Windows Update, disco quasi pieno) e svuota la cache dei download di Steam. Giochi e salvataggi non si toccano.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs2(`
$p = $null
try { [void][Windows.Networking.Connectivity.NetworkInformation, Windows.Networking.Connectivity, ContentType = WindowsRuntime]; $p = [Windows.Networking.Connectivity.NetworkInformation]::GetInternetConnectionProfile() } catch {}
if ($p -and [string]$p.GetConnectionCost().NetworkCostType -notmatch 'Unrestricted|Unknown') { Say 'La rete è segnata "a consumo": l''app Xbox e Windows scaricano piano o si fermano. Usa "Aggiornamenti e OneDrive fermi: connessione a consumo" (in Risolvi problemi, sezione Ufficio).' }
$steam = ''; try { $steam = [string](Get-ItemProperty -LiteralPath 'HKCU:\Software\Valve\Steam' -Name SteamPath -ErrorAction Stop).SteamPath } catch {}
if ($steam -and (Test-Path -LiteralPath $steam)) {
  $cfg = Join-Path $steam 'config\config.vdf'
  if ((Test-Path -LiteralPath $cfg) -and ((Get-Content -LiteralPath $cfg -Raw -ErrorAction SilentlyContinue) -match '"DownloadThrottleKbps"\s+"([1-9]\d*)"')) { Say 'Steam ha un limite di velocità dei download impostato: toglilo in Steam > Impostazioni > Download > "Limita la larghezza di banda".' }
  if (Get-Process -Name steam -ErrorAction SilentlyContinue) { Say 'Steam è aperto: per svuotare la cache dei download chiudilo (Steam > Esci) e premi di nuovo Risolvi. Oppure fallo da Steam > Impostazioni > Download > "Svuota la cache dei download".' }
  else { $f = Clear-Folder (Join-Path $steam 'appcache\httpcache'); Freed $f; Say ('Svuotata la memoria web di Steam (' + (Fmt $f) + '): giochi installati e salvataggi restano.'); Say 'Se il download resta lento, svuota anche la "cache dei download" da Steam > Impostazioni > Download (Steam ti chiederà di rientrare).' }
  Say 'Su Steam aiuta anche cambiare "Area di download" (Impostazioni > Download) con una città vicina, per esempio Milano.'
}
if (Get-Process -Name TiWorker, MoUsoCoreWorker -ErrorAction SilentlyContinue) { Say 'Windows Update sta scaricando o installando: in questo momento i giochi scaricano più piano.' }
$c = Get-PSDrive -Name ($env:SystemDrive.Trim(':')) -ErrorAction SilentlyContinue
if ($c -and $c.Free -lt 30GB) { Say ('Sul disco C: ci sono solo ' + (Fmt $c.Free) + ' liberi: con poco spazio download e installazioni rallentano o si fermano.') }
Say 'Se il download resta lento: fai il test di velocità nella pagina Internet. Se lì va bene e solo il gioco è lento, spesso è il server del gioco (il giorno di uscita di un gioco o di un aggiornamento grosso è normale).'
`)},
}

func init() {
	insertStepAfter("fix_game_anticheat", gameFixSteps3...)
}
