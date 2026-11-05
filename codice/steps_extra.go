package main

import "time"

// Passaggi nuovi della 2.5: più cose da pulire e da controllare, fatte
// in modo sicuro: solo cache e resti che si ricreano, mai documenti o impostazioni.

func init() {
	insertStepAfter("cleanmgr", extraSteps[0], extraSteps[1], extraSteps[2])
	insertStepAfter("dism_cleanup", extraSteps[3])
	insertStepAfter("browser_ext", extraSteps[4])
	insertStepAfter("chkdsk", extraSteps[5])
	insertStepAfter("privacy_undo", extraSteps[6])
	// 2.5: cinque livelli. I passaggi salgono di un livello e nasce "Classica" (1): la passata
	// velocissima con temporanei tuoi, Cestino e miniature.
	for i := range allSteps {
		if allSteps[i].Level > 0 {
			allSteps[i].Level++
		}
		switch allSteps[i].ID {
		case "temp_user", "recycle", "thumbs":
			allSteps[i].Level = 1
		}
	}
}

// insertStepAfter mette i passaggi subito dopo quello indicato (l'ordine conta: è l'ordine delle pulizie).
func insertStepAfter(id string, steps ...Step) {
	for i, s := range allSteps {
		if s.ID == id {
			rest := append([]Step{}, allSteps[i+1:]...)
			allSteps = append(append(allSteps[:i+1], steps...), rest...)
			return
		}
	}
	allSteps = append(allSteps, steps...)
}

var extraSteps = []Step{
	{ID: "driver_leftovers", Name: "Resti dei driver video", Desc: "Cartelle che AMD, NVIDIA e Intel lasciano dopo aver installato i driver (C:\\AMD, C:\\NVIDIA...). Spesso sono vari GB.", Level: 2, Minutes: 0.4, Timeout: 10 * time.Minute, Script: `
$drvRoots = @((Join-Path ($env:SystemDrive + '\') 'AMD\'), (Join-Path ($env:SystemDrive + '\') 'NVIDIA\'))
$busy = $false
foreach ($pr in @(Get-Process -ErrorAction SilentlyContinue)) {
  if ($pr.ProcessName -match 'AMDSoftwareInstaller|AMDInstallManager|AMDCleanupUtility|NVIDIA.*Install|Intel.*Install|igxpin') { $busy = $true; break }
  $pp = [string]$pr.Path
  foreach ($r in $drvRoots) { if ($pp -and $pp.StartsWith($r, [StringComparison]::OrdinalIgnoreCase)) { $busy = $true } }
  if ($busy) { break }
}
if ($busy) { Say 'Un programma di installazione dei driver è aperto: salto questo passaggio.'; return }
$sd = $env:SystemDrive + '\'
$f = [long]0
$list = @(
  @{ Label = 'AMD (C:\AMD)';               Path = (Join-Path $sd 'AMD') },
  @{ Label = 'NVIDIA (C:\NVIDIA)';         Path = (Join-Path $sd 'NVIDIA') },
  @{ Label = 'Intel (registri)';           Path = (Join-Path $sd 'Intel\Logs') },
  @{ Label = 'Intel (installazione)';      Path = (Join-Path $sd 'Intel\ExtremeGraphics') },
  @{ Label = 'NVIDIA (driver scaricati)';  Path = (Join-Path $env:ProgramData 'NVIDIA Corporation\Downloader') }
)
$any = $false
foreach ($it in $list) {
  if (-not (Test-Path -LiteralPath $it.Path)) { continue }
  $any = $true
  $x = Clear-Folder $it.Path
  $f += $x
  if ($x -gt 0) { Say ($it.Label + ': ' + (Fmt $x)) }
}
if (-not $any) { Say 'Nessun resto di installazione dei driver trovato.' }
Freed $f; Say ('Liberati ' + (Fmt $f))
`},
	{ID: "game_cache", Name: "Cache di launcher e app", Desc: "Battle.net, EA app, Ubisoft Connect, GOG, Riot, Steam (registri e dump) e Spotify: solo cache e registri vecchi. Giochi, salvataggi e accessi restano.", Level: 2, Minutes: 0.5, Timeout: 10 * time.Minute, Script: `
$steam = ''
try { $steam = [string](Get-ItemProperty -LiteralPath 'HKCU:\Software\Valve\Steam' -ErrorAction Stop).SteamPath; $steam = $steam -replace '/', '\' } catch {}
$pf86 = ${env:ProgramFiles(x86)}
$apps = @(
  @{ Label = 'Battle.net';      Proc = 'Battle.net';     Age = 0;   Paths = @((Join-Path $env:LOCALAPPDATA 'Battle.net\Cache'), (Join-Path $env:LOCALAPPDATA 'Battle.net\BrowserCaches')) },
  @{ Label = 'EA app';          Proc = 'EADesktop';      Age = 0;   Paths = @((Join-Path $env:LOCALAPPDATA 'Electronic Arts\EA Desktop\CEF\BrowserCache')) },
  @{ Label = 'Ubisoft Connect'; Proc = 'UbisoftConnect'; Age = 0;   Paths = @((Join-Path $pf86 'Ubisoft\Ubisoft Game Launcher\cache\http'), (Join-Path $pf86 'Ubisoft\Ubisoft Game Launcher\cache\assets')) },
  @{ Label = 'GOG Galaxy';      Proc = 'GalaxyClient';   Age = 0;   Paths = @((Join-Path $env:ProgramData 'GOG.com\Galaxy\webcache')) },
  @{ Label = 'Riot (registri)'; Proc = '';               Age = 48;  Paths = @((Join-Path $env:LOCALAPPDATA 'Riot Games\Riot Client\Logs')) },
  @{ Label = 'Spotify';         Proc = 'Spotify';        Age = 0;   Paths = @((Join-Path $env:LOCALAPPDATA 'Spotify\Data')) }
)
if ($steam) {
  $apps += @{ Label = 'Steam (registri e dump)'; Proc = ''; Age = 168; Paths = @((Join-Path $steam 'dumps'), (Join-Path $steam 'logs')) }
  $apps += @{ Label = 'Steam (immagini del negozio)'; Proc = 'steam'; Age = 0; Paths = @((Join-Path $steam 'appcache\httpcache')) }
}
$f = [long]0
$any = $false
foreach ($a in $apps) {
  $exist = @($a.Paths | Where-Object { $_ -and (Test-Path -LiteralPath $_) })
  if ($exist.Count -eq 0) { continue }
  $any = $true
  if ($a.Proc -and (Get-Process -Name $a.Proc -ErrorAction SilentlyContinue)) { Say ($a.Label + ' è aperto: lo salto.'); continue }
  $x = [long]0
  foreach ($p in $exist) { $x += Clear-Folder $p $a.Age }
  $f += $x
  Say ($a.Label + ': ' + (Fmt $x))
}
if (-not $any) { Say 'Nessuno di questi launcher trovato.' }
Freed $f; Say ('Liberati ' + (Fmt $f))
`},
	{ID: "extra_junk", Name: "Altri file inutili di Windows", Desc: "Rapporti di blocco del kernel, temporanei dei servizi di Windows, vecchia cache internet e registri vecchi degli aggiornamenti.", Level: 2, Minutes: 0.5, Timeout: 10 * time.Minute, Script: `
$f = [long]0
$list = @(
  @{ Label = 'Rapporti di blocco (LiveKernelReports)'; Path = (Join-Path $env:windir 'LiveKernelReports'); Age = 72 },
  @{ Label = 'Temporanei dei servizi (LocalService)';  Path = (Join-Path $env:windir 'ServiceProfiles\LocalService\AppData\Local\Temp'); Age = 24 },
  @{ Label = 'Temporanei dei servizi (NetworkService)'; Path = (Join-Path $env:windir 'ServiceProfiles\NetworkService\AppData\Local\Temp'); Age = 24 },
  # Solo IE e Low: in Content.Outlook, Content.MSO e Content.Word ci sono allegati e documenti di Office
  # aperti e modificati, che a volte sono l'unica copia. Quelli non li tocco mai.
  @{ Label = 'Vecchia cache internet (INetCache)';     Path = (Join-Path $env:LOCALAPPDATA 'Microsoft\Windows\INetCache\IE'); Age = 24 },
  @{ Label = 'Vecchia cache internet (INetCache Low)'; Path = (Join-Path $env:LOCALAPPDATA 'Microsoft\Windows\INetCache\Low'); Age = 24 }
)
foreach ($it in $list) {
  if (-not (Test-Path -LiteralPath $it.Path)) { continue }
  $x = Clear-Folder $it.Path $it.Age
  $f += $x
  if ($x -gt 0) { Say ($it.Label + ': ' + (Fmt $x)) }
}
# Installazione precedente di Windows: è protetta e serve per tornare indietro, quindi la segnalo e basta.
$old = Join-Path $env:SystemDrive 'Windows.old'
if (Test-Path -LiteralPath $old) {
  $s = Get-FolderSize $old
  Say ('C''è una copia della versione precedente di Windows (Windows.old, ' + (Fmt $s) + '). Se Windows funziona bene da più di 10 giorni puoi toglierla da Impostazioni > Sistema > Archiviazione > File temporanei > "Installazione precedente di Windows".')
}
Freed $f; Say ('Liberati ' + (Fmt $f))
`},
	{ID: "old_drivers", Name: "Driver video vecchi", Desc: "Toglie dall'archivio di Windows le versioni vecchie dei driver della scheda video che non sono più in uso (spesso 1-3 GB). Quello in uso e quello prima restano sempre.", Level: 3, Minutes: 1.5, Timeout: 20 * time.Minute, Script: `
if ((Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending') -or (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired')) { Say 'Windows aspetta un riavvio (forse per un driver nuovo): salto, così non tolgo niente che serve. Rifallo dopo il riavvio.'; return }
Say 'Leggo i driver salvati in Windows...'
try { $drv = @(Get-WindowsDriver -Online -ErrorAction Stop | Where-Object { [string]$_.ClassName -eq 'Display' }) } catch { Say 'Non riesco a leggere i driver: salto.'; return }
$inUse = @{}
foreach ($d in @(Get-CimInstance Win32_PnPSignedDriver -ErrorAction SilentlyContinue)) { if ($d.InfName) { $inUse[([string]$d.InfName).ToLower()] = $true } }
# Per ogni marca (AMD, NVIDIA, Intel) restano sempre le 2 versioni più nuove (per poter tornare indietro), più tutte quelle in uso.
$groups = $drv | Group-Object { ([string]$_.ProviderName).ToLower() }
$f = [long]0; $n = 0
foreach ($g in $groups) {
  if ($g.Count -lt 3) { continue }
  $sorted = @($g.Group | Sort-Object { try { [version]([string]$_.Version) } catch { [version]'0.0' } } -Descending)
  for ($i = 2; $i -lt $sorted.Count; $i++) {
    $d = $sorted[$i]
    $inf = ([string]$d.Driver).ToLower()
    if (-not $inf -or $inUse.ContainsKey($inf)) { continue }
    $dir = Split-Path -Parent ([string]$d.OriginalFileName)
    $size = Get-FolderSize $dir
    $code = Run-Hidden 'pnputil.exe' ('/delete-driver ' + $inf)
    if (($code -eq 0 -or $code -eq 3010) -and -not (Test-Path -LiteralPath $dir)) { $f += $size; $n++; Say ('Tolto: ' + $d.ProviderName + ' versione ' + $d.Version + ' (' + (Fmt $size) + ')') }
  }
}
if ($n -eq 0) { Say 'Nessun driver video vecchio da togliere.' }
Freed $f; Say ('Liberati ' + (Fmt $f))
`},
	{ID: "sec_check", Name: "Controllo sicurezza", Desc: "Firewall, antivirus, controllo account, SmartScreen, Avvio protetto, TPM, Desktop remoto, SMBv1 e account Guest. Tasso controlla e ti dice cosa sistemare, senza cambiare niente.", Level: 3, Minutes: 0.4, Timeout: 5 * time.Minute, Script: `
$ok = 0; $warn = 0
function Ok([string]$m) { $script:ok++; Say ('OK  ' + $m) }
function Warn([string]$m) { $script:warn++; Say ('DA SISTEMARE  ' + $m) }
$fw3 = @(); try { $fw3 = @(Get-CimInstance -Namespace root/SecurityCenter2 -ClassName FirewallProduct -ErrorAction Stop | Where-Object { $_.displayName -notmatch 'Windows' }) } catch {}
try { $off = @(Get-NetFirewallProfile -ErrorAction Stop | Where-Object { -not $_.Enabled }); if ($off.Count -eq 0) { Ok 'Firewall acceso su tutte le reti' } elseif ($fw3.Count -gt 0) { Ok ('Firewall: lo gestisce ' + (($fw3 | ForEach-Object { $_.displayName }) -join ', ')) } else { Warn ('Firewall spento: ' + (($off | ForEach-Object { $_.Name }) -join ', ') + '. Riaccendilo da Sicurezza di Windows.') } } catch {}
try {
  $mp = Get-MpComputerStatus -ErrorAction Stop
  $other = @(Get-CimInstance -Namespace root/SecurityCenter2 -ClassName AntiVirusProduct -ErrorAction SilentlyContinue | Where-Object { $_.displayName -notmatch 'Defender' -and (([int]$_.productState -band 0x1000) -ne 0) })
  if ($other.Count -or ($mp.AMRunningMode -and [string]$mp.AMRunningMode -ne 'Normal')) {
    # C'è un altro antivirus acceso: Defender è a riposo apposta, i suoi aggiornamenti non contano.
    Ok ('Antivirus attivo: ' + $(if ($other.Count) { [string]$other[0].displayName } else { 'un altro antivirus' }))
  } else {
    if ($mp.RealTimeProtectionEnabled) { Ok 'Antivirus in tempo reale acceso' } else { Warn 'Antivirus in tempo reale spento.' }
    if ($mp.AntivirusSignatureAge -le 7) { Ok 'Antivirus aggiornato' } else { Warn ('Antivirus non aggiornato da ' + $mp.AntivirusSignatureAge + ' giorni.') }
  }
} catch {}
try {
  $pol = Get-ItemProperty -LiteralPath 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System' -ErrorAction Stop
  if ($pol.EnableLUA -eq 0) { Warn 'Controllo account utente (UAC) spento: i programmi possono cambiare il PC senza chiedere.' }
  elseif ($pol.ConsentPromptBehaviorAdmin -eq 0) { Warn 'Il controllo account utente non chiede mai conferma: alzalo da "Modifica impostazioni di Controllo dell''account utente".' }
  else { Ok 'Controllo account utente attivo' }
} catch {}
try { $ss = [string](Get-ItemProperty -LiteralPath 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer' -ErrorAction Stop).SmartScreenEnabled; if ($ss -eq 'Off') { Warn 'SmartScreen spento: Windows non ti avvisa dei programmi pericolosi scaricati.' } else { Ok 'SmartScreen attivo' } } catch { Ok 'SmartScreen attivo' }
try { if (Confirm-SecureBootUEFI -ErrorAction Stop) { Ok 'Avvio protetto (Secure Boot) acceso' } else { Warn 'Avvio protetto (Secure Boot) spento: si accende dal BIOS. Alcuni giochi con anti-cheat (Valorant, Battlefield 6) lo richiedono.' } } catch { Say 'INFO  Avvio protetto: non disponibile su questo PC (BIOS vecchio stile).' }
try { $t = Get-Tpm -ErrorAction Stop; if ($t.TpmPresent -and $t.TpmReady) { Ok 'Chip di sicurezza TPM pronto' } elseif ($t.TpmPresent) { Warn 'Il chip TPM c''è ma non è pronto: attivalo dal BIOS (fTPM per AMD, PTT per Intel).' } else { Warn 'Chip TPM non trovato o spento nel BIOS.' } } catch {}
try { $rd = (Get-ItemProperty -LiteralPath 'HKLM:\SYSTEM\CurrentControlSet\Control\Terminal Server' -ErrorAction Stop).fDenyTSConnections; if ($rd -eq 0) { Warn 'Desktop remoto acceso: se non lo usi spegnilo da Impostazioni > Sistema > Desktop remoto.' } else { Ok 'Desktop remoto spento' } } catch {}
try { $smb = Get-WindowsOptionalFeature -Online -FeatureName SMB1Protocol -ErrorAction Stop; if ([string]$smb.State -eq 'Enabled') { Warn 'Il vecchio protocollo di rete SMBv1 è acceso: è insicuro (lo usava WannaCry). Spegnilo da "Attiva o disattiva funzionalità di Windows".' } else { Ok 'Protocollo insicuro SMBv1 spento' } } catch {}
try { $g = Get-LocalUser -ErrorAction Stop | Where-Object { [string]$_.SID -match '-501$' } | Select-Object -First 1; if ($g -and $g.Enabled) { Warn 'L''account Guest (ospite) è attivo: chiunque può entrare senza password.' } else { Ok 'Account Guest spento' } } catch {}
try { $hv = (Get-ItemProperty -LiteralPath 'HKLM:\SYSTEM\CurrentControlSet\Control\DeviceGuard\Scenarios\HypervisorEnforcedCodeIntegrity' -ErrorAction Stop).Enabled; if ($hv -eq 1) { Say 'INFO  Isolamento core (integrità della memoria) acceso: più sicuro, nei giochi costa pochissimi FPS.' } else { Say 'INFO  Isolamento core (integrità della memoria) spento.' } } catch {}
Say ('Risultato: ' + $ok + ' cose a posto, ' + $warn + ' da sistemare.')
`},
	{ID: "stability", Name: "Controllo stabilità", Desc: "Guarda gli ultimi 30 giorni: spegnimenti improvvisi, schermate blu, errori hardware (spesso RAM o overclock), errori del disco, driver video che si bloccano e programmi che si chiudono da soli.", Level: 4, Minutes: 0.5, Timeout: 5 * time.Minute, Script: `
$since = (Get-Date).AddDays(-30)
function Count-Ev([string]$log, [string]$prov, [int[]]$ids) {
  $h = @{ LogName = $log; ProviderName = $prov; StartTime = $since }
  if ($ids) { $h.Id = $ids }
  try { return @(Get-WinEvent -FilterHashtable $h -ErrorAction Stop) } catch { return @() }
}
$bad = 0
$e = Count-Ev 'System' 'Microsoft-Windows-Kernel-Power' @(41)
if ($e.Count -gt 0) { $bad++; Say ('DA GUARDARE  Spegnimenti improvvisi: ' + $e.Count + ' (ultimo ' + $e[0].TimeCreated.ToString('dd/MM HH:mm') + '). Se non hai tolto tu la corrente: alimentatore, temperature o overclock.') } else { Say 'OK  Nessuno spegnimento improvviso' }
$e = Count-Ev 'System' 'Microsoft-Windows-WER-SystemErrorReporting' @(1001)
if ($e.Count -gt 0) { $bad++; Say ('DA GUARDARE  Schermate blu: ' + $e.Count + ' (ultima ' + $e[0].TimeCreated.ToString('dd/MM HH:mm') + '). Aggiorna driver video e chipset; se hai attivato EXPO/XMP o un undervolt, prova a toglierli.') } else { Say 'OK  Nessuna schermata blu' }
# Solo gli errori veri: gli avvisi "corretti" (es. PCIe, evento 17) sono comuni e innocui.
$e = @(Count-Ev 'System' 'Microsoft-Windows-WHEA-Logger' $null | Where-Object { $_.Level -le 2 })
if ($e.Count -gt 0) { $bad++; Say ('DA GUARDARE  Errori hardware segnalati dal processore: ' + $e.Count + '. Di solito è RAM o overclock/undervolt troppo spinto: riporta le impostazioni del BIOS a un passo più tranquillo.') } else { Say 'OK  Nessun errore hardware' }
$e = @(Count-Ev 'System' 'disk' @(7, 51, 153)) + @(Count-Ev 'System' 'Ntfs' @(55))
if ($e.Count -gt 0) { $bad++; Say ('DA GUARDARE  Errori del disco: ' + $e.Count + '. Fai un backup e lancia "Controllo errori del disco".') } else { Say 'OK  Nessun errore del disco' }
$e = Count-Ev 'System' 'Display' @(4101)
if ($e.Count -gt 0) { $bad++; Say ('DA GUARDARE  Il driver video si è bloccato e ripreso ' + $e.Count + ' volte. Aggiorna il driver dal sito AMD/NVIDIA; se hai fatto overclock della scheda video, abbassalo.') } else { Say 'OK  Driver video stabile' }
$e = Count-Ev 'Application' 'Application Error' @(1000)
if ($e.Count -gt 0) {
  $top = $e | Group-Object { [string]$_.Properties[0].Value } | Sort-Object Count -Descending | Select-Object -First 3
  Say ('INFO  Programmi che si sono chiusi da soli: ' + (($top | ForEach-Object { $_.Name + ' (' + $_.Count + ')' }) -join ', '))
} else { Say 'OK  Nessun programma si è chiuso da solo' }
if ($bad -eq 0) { Say 'Il PC è stabile negli ultimi 30 giorni.' }
`},
	{ID: "privacy_traces", Name: "Tracce di attività", Desc: "Cancella l'elenco dei file aperti di recente, la cronologia di Esegui e della barra degli indirizzi di Esplora file e gli appunti. I file veri e i collegamenti fissati restano.", Level: 0, Minutes: 0.2, Timeout: 3 * time.Minute, Script: `
$rec = Join-Path $env:APPDATA 'Microsoft\Windows\Recent'
$n = 0
if (Test-Path -LiteralPath $rec) {
  Get-ChildItem -LiteralPath $rec -File -Filter '*.lnk' -Force -ErrorAction SilentlyContinue | ForEach-Object { Remove-Item -LiteralPath $_.FullName -Force -ErrorAction SilentlyContinue; $n++ }
}
Say ('File recenti: tolti ' + $n + ' collegamenti dall''elenco.')
foreach ($k in @('HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\RunMRU', 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\TypedPaths')) {
  $it = Get-Item -LiteralPath $k -ErrorAction SilentlyContinue
  if ($it) { foreach ($v in $it.GetValueNames()) { if ($v) { Remove-ItemProperty -LiteralPath $k -Name $v -ErrorAction SilentlyContinue } } }
}
Say 'Cronologia di Esegui e della barra di Esplora file: cancellata.'
try { Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.Clipboard]::Clear(); Say 'Appunti: svuotati.' } catch {}
try {
  $null = [Windows.ApplicationModel.DataTransfer.Clipboard, Windows.ApplicationModel.DataTransfer, ContentType = WindowsRuntime]
  if ([Windows.ApplicationModel.DataTransfer.Clipboard]::ClearHistory()) { Say 'Cronologia degli appunti (Win+V): cancellata.' }
} catch {}
`},
}
