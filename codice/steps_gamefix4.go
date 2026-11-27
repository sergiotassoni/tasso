package main

// 3.1: altri 14 problemi da gioco, i più cercati da chi gioca. Ogni pulsante controlla davvero
// il PC e cambia solo cose sicure (con il file per tornare indietro quando tocca il registro).
// Non tocca mai giochi, salvataggi, file dell'anti-cheat o le protezioni di Windows.

import "time"

// gpuTempPS: temperatura della scheda video NVIDIA (se c'è nvidia-smi), in $gt.
const gpuTempPS = `
$gt = $null
foreach ($smi in @((Join-Path $env:SystemRoot 'System32\nvidia-smi.exe'), (Join-Path $env:ProgramFiles 'NVIDIA Corporation\NVSMI\nvidia-smi.exe'))) {
  if (-not $gt -and (Test-Path -LiteralPath $smi)) {
    try { $o = (& $smi --query-gpu=temperature.gpu --format=csv,noheader,nounits 2>$null | Select-Object -First 1); if ([string]$o -match '^\s*(\d{2,3})') { $gt = [int]$Matches[1] } } catch {}
  }
}
`

// cpuTempPS: temperatura del processore (stesso metodo del pannellino), in $ct.
const cpuTempPS = `
$ct = $null; $ctAcpi = $false
try { $s = Get-CimInstance -Namespace root/LibreHardwareMonitor -ClassName Sensor -ErrorAction Stop | Where-Object { $_.SensorType -eq 'Temperature' -and ($_.Name -match 'CPU Package|Core \(Tctl|Tctl|Tdie|CPU Total') } | Select-Object -First 1; if ($s) { $ct = [double]$s.Value } } catch {}
if (-not $ct) { try { $z = Get-CimInstance -Namespace root/wmi -ClassName MSAcpi_ThermalZoneTemperature -ErrorAction Stop | Sort-Object -Property CurrentTemperature -Descending | Select-Object -First 1; if ($z) { $c = ([double]$z.CurrentTemperature / 10) - 273.15; if ($c -gt 28 -or $c -lt 27) { $ct = $c; $ctAcpi = $true } } } catch {} }
if ($ct -and ($ct -lt 15 -or $ct -gt 115)) { $ct = $null }
`

// steamLibsPS: cartelle delle librerie di Steam, in $libs.
const steamLibsPS = `
$libs = @()
$steam = ''; try { $steam = [string](Get-ItemProperty -LiteralPath 'HKCU:\Software\Valve\Steam' -Name SteamPath -ErrorAction Stop).SteamPath } catch {}
if ($steam) {
  $steam = $steam -replace '/', '\'
  $vdf = Join-Path $steam 'steamapps\libraryfolders.vdf'
  if (Test-Path -LiteralPath $vdf) { foreach ($m in [regex]::Matches((Get-Content -LiteralPath $vdf -Raw -ErrorAction SilentlyContinue), '"path"\s+"([^"]+)"')) { $p = $m.Groups[1].Value -replace '\\\\', '\'; if (Test-Path -LiteralPath $p) { $libs += $p } } }
  if (-not $libs.Count -and (Test-Path -LiteralPath $steam)) { $libs += $steam }
}
`

// diskKindPS: funzione che dice se una lettera di disco è SSD o disco meccanico (HDD).
const diskKindPS = `
function Get-DiskKind([string]$letter) {
  try {
    $part = Get-Partition -DriveLetter $letter.Trim(':') -ErrorAction Stop | Select-Object -First 1
    $pd = Get-PhysicalDisk -ErrorAction Stop | Where-Object { [string]$_.DeviceId -eq [string]$part.DiskNumber } | Select-Object -First 1
    if ($pd) { return [string]$pd.MediaType }
  } catch {}
  return ''
}
`

var gameFixSteps4 = []Step{
	{ID: "fix_game_save", Name: "Il gioco non salva i progressi", Desc: "Controlla le due cause più comuni: la protezione di Windows \"Accesso alle cartelle controllato\" che blocca il gioco, e OneDrive che sposta o tiene solo online la cartella Documenti dove i giochi salvano. Non cambia le protezioni: ti dice dove consentire il gioco.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs2(`
$found = $false
$cfa = $null; try { $cfa = (Get-MpPreference -ErrorAction Stop).EnableControlledFolderAccess } catch {}
if ($cfa -eq 1) {
  $found = $true
  Say 'Trovato: è accesa la protezione di Windows "Accesso alle cartelle controllato". Blocca i programmi che non conosce quando scrivono in Documenti, ed è proprio lì che molti giochi salvano.'
  Say 'Non la spengo: è una protezione contro i virus che rubano i file. Ti apro la pagina giusta: premi "Consenti un''app tramite Accesso alle cartelle controllato" > "Aggiungi un''app consentita" e scegli il gioco.'
  try { $ev = @(Get-WinEvent -FilterHashtable @{ LogName = 'Microsoft-Windows-Windows Defender/Operational'; Id = 1123 } -MaxEvents 5 -ErrorAction Stop); foreach ($e in $ev) { if ([string]$e.Message -match '([A-Za-z]:\\[^\r\n]+?\.exe)') { Say ('Bloccato di recente: ' + $Matches[1]) } } } catch {}
  Open-User 'windowsdefender://ransomwareprotection'
} elseif ($cfa -eq 2 -or $cfa -eq 3 -or $cfa -eq 4) { Say 'La protezione "Accesso alle cartelle controllato" non è in modo "blocca": non blocca i salvataggi.' }
else { Say 'La protezione "Accesso alle cartelle controllato" è spenta: non è lei a bloccare i salvataggi.' }
$docs = [Environment]::GetFolderPath('MyDocuments')
$od = @(Get-OneDriveRoots)
$inOd = $false; foreach ($r in $od) { if ($docs -and $docs.StartsWith($r, [StringComparison]::OrdinalIgnoreCase)) { $inOd = $true } }
if ($inOd) {
  $found = $true
  Say ('La cartella Documenti è dentro OneDrive (' + $docs + '). Molti giochi salvano lì: se OneDrive è fermo, pieno o tiene i file "solo online", il gioco non riesce a salvare o perde i progressi.')
  $mg = Join-Path $docs 'My Games'
  if (Test-Path -LiteralPath $mg) {
    $cloud = @(Get-ChildItem -LiteralPath $mg -Recurse -File -Force -ErrorAction SilentlyContinue | Where-Object { ([int]$_.Attributes -band 0x400000) -ne 0 -or ([int]$_.Attributes -band 0x1000) -ne 0 } | Select-Object -First 50)
    if ($cloud.Count) { Say ('In Documenti\My Games ci sono file tenuti solo online (' + $cloud.Count + ' o più). Tasto destro sulla cartella My Games > "Mantieni sempre su questo dispositivo".') }
  }
  if (-not (Get-Process -Name OneDrive -ErrorAction SilentlyContinue)) { Say 'OneDrive non è aperto: i file "solo online" non si possono aprire. Aprilo dal menu Start.' }
  Say 'Se non ti serve, puoi togliere Documenti da OneDrive: icona di OneDrive > Impostazioni > Sincronizzazione e backup > Gestisci backup > spegni Documenti.'
}
$c = Get-PSDrive -Name ($env:SystemDrive.Trim(':')) -ErrorAction SilentlyContinue
if ($c -and $c.Free -lt 2GB) { $found = $true; Say ('Sul disco C: ci sono solo ' + (Fmt $c.Free) + ' liberi: con il disco pieno i giochi non riescono a scrivere il salvataggio. Libera spazio da Pulisci e ripara.') }
if (-not $found) { Say 'Non vedo blocchi da parte di Windows. Altre cause: il gioco aperto "come amministratore" una volta e normale un''altra (salva in due posti diversi), o i salvataggi nel cloud di Steam in conflitto: in Steam scegli la versione più recente quando te lo chiede.' }
`)},
	{ID: "fix_game_gpucrash", Name: "Il gioco si chiude con un errore della scheda video", Desc: "Per errori come DEVICE_REMOVED, DXGI_ERROR o \"il driver video ha smesso di rispondere\": cerca nel registro di Windows quante volte il driver si è bloccato, controlla temperatura e driver, e ti dice cosa fare. Non cambia il driver da solo.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs2(gpuTempPS + `
$since = (Get-Date).AddDays(-14)
$tdr = @(); try { $tdr = @(Get-WinEvent -FilterHashtable @{ LogName = 'System'; Id = 4101; StartTime = $since } -ErrorAction Stop) } catch {}
$lkd = @(); try { $lkd = @(Get-ChildItem -LiteralPath (Join-Path $env:SystemRoot 'LiveKernelReports\WATCHDOG') -Filter *.dmp -ErrorAction Stop | Where-Object { $_.LastWriteTime -gt $since }) } catch {}
if ($tdr.Count -or $lkd.Count) { Say ('Negli ultimi 14 giorni il driver della scheda video si è bloccato e ripreso ' + [math]::Max($tdr.Count, $lkd.Count) + ' volte. È la causa dell''errore del gioco.') }
else { Say 'Negli ultimi 14 giorni Windows non ha registrato blocchi del driver video: l''errore viene probabilmente dal gioco stesso (prova "Verifica integrità dei file" in Steam o "Ripara" in Epic).' }
$v = @(Get-CimInstance Win32_VideoController -ErrorAction SilentlyContinue | Where-Object { $_.Name -notmatch 'Basic Display|Microsoft Remote|Virtual|Parsec|Meta' })
foreach ($g in $v) {
  $age = ''
  if ($g.DriverDate) { $days = [int]((Get-Date) - $g.DriverDate).TotalDays; if ($days -gt 180) { $age = ' (driver vecchio di ' + [int]($days / 30) + ' mesi)' } }
  Say ($g.Name + ': driver ' + $g.DriverVersion + $age)
}
if ($gt) { if ($gt -ge 85) { Say ('La scheda video è a ' + $gt + '°C anche adesso: è molto calda. Il caldo è una causa frequente di questo errore: vedi "Il PC si scalda troppo".') } else { Say ('Temperatura della scheda video adesso: ' + $gt + '°C.') } }
$oc = @(Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -match '^(MSIAfterburner|EVGAPrecision.*|GPUTweak.*|TriXX|ZOTAC.*FireStorm)$' } | ForEach-Object { $_.ProcessName } | Select-Object -Unique)
if ($oc.Count) { Say ('È aperto un programma per spingere la scheda video (overclock): ' + ($oc -join ', ') + '. Rimetti i valori di fabbrica ("Reset"): è la causa più comune di DEVICE_REMOVED.') }
Say 'Cosa fare, in ordine: 1) aggiorna il driver video da Risolvi un problema > "Driver di tutto il PC" o dal sito di NVIDIA/AMD/Intel; 2) togli overclock e "profili OC" dai programmi della scheda video; 3) nel gioco abbassa un po'' la grafica o metti un limite agli FPS; 4) se succede in tutti i giochi, controlla che i cavi di alimentazione della scheda video siano infilati bene.'
`)},
	{ID: "fix_game_heat", Name: "Il PC si scalda troppo quando gioco", Desc: "Legge le temperature di processore e scheda video, controlla se il PC sta già rallentando per il caldo e ti dice cosa fare. Non tocca le ventole.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs2(cpuTempPS + gpuTempPS + `
$any = $false
if ($ct -and $ctAcpi) { $any = $true; Say ('Sensore di temperatura della scheda madre: ' + [math]::Round($ct) + '°C. Su molti PC non è il processore: per la temperatura vera del processore apri LibreHardwareMonitor e rifai il controllo.') }
elseif ($ct) { $any = $true; $r = [math]::Round($ct); if ($r -ge 90) { Say ('Processore: ' + $r + '°C adesso. Troppo caldo: oltre 90°C rallenta da solo per proteggersi.') } elseif ($r -ge 75) { if ($TassoUfficio) { Say ('Processore: ' + $r + '°C adesso. Caldo: succede con programmi pesanti o tante schede aperte; a PC fermo dovrebbe stare sotto i 60°C.') } else { Say ('Processore: ' + $r + '°C adesso. Caldo, ma è normale se stai giocando; a PC fermo dovrebbe stare sotto i 60°C.') } } else { Say ('Processore: ' + $r + '°C adesso: va bene.') } }
if ($gt) { $any = $true; if ($gt -ge 87) { Say ('Scheda video: ' + $gt + '°C adesso. Troppo calda: oltre 85-90°C rallenta.') } elseif ($gt -ge 75) { if ($TassoUfficio) { Say ('Scheda video: ' + $gt + '°C adesso. Calda: per il lavoro d''ufficio è tanto, guarda se un programma la sta usando molto.') } else { Say ('Scheda video: ' + $gt + '°C adesso. Calda, normale mentre giochi.') } } else { Say ('Scheda video: ' + $gt + '°C adesso: va bene.') } }
if (-not $any) { if ($TassoUfficio) { Say 'Su questo PC Windows non mi dà le temperature. Per vederle, prova la pagina Hardware di Tasso mentre il PC lavora.' } else { Say 'Su questo PC Windows non mi dà le temperature. Per vederle, prova la pagina Hardware di Tasso mentre giochi.' } }
try { $thr = @(Get-WinEvent -FilterHashtable @{ LogName = 'System'; ProviderName = 'Microsoft-Windows-Kernel-Processor-Power'; Id = 37; StartTime = (Get-Date).AddDays(-7) } -ErrorAction Stop); if ($thr.Count) { Say ('Negli ultimi 7 giorni il processore è stato rallentato dal sistema ' + $thr.Count + ' volte (di solito per il caldo o per il caricatore).') } } catch {}
$lap = $false; try { $lap = [bool](Get-CimInstance Win32_Battery -ErrorAction Stop) } catch {}
if ($lap) { Say 'È un portatile: tienilo su un piano rigido (non sul letto o sul cuscino), con le griglie di sotto libere. Una base con ventole aiuta molto.' }
if ($TassoUfficio) { Say 'Cosa fare: 1) pulisci la polvere dalle griglie e dalle ventole (aria compressa, a PC spento); 2) chiudi i programmi e le schede del browser che non usi; 3) se il PC ha più di 4-5 anni, la pasta termica sul processore va cambiata (da un tecnico).' }
else { Say 'Cosa fare: 1) pulisci la polvere dalle griglie e dalle ventole (aria compressa, a PC spento); 2) metti un limite agli FPS uguale ai Hz dello schermo: la scheda video lavora meno e scalda meno; 3) se il PC ha più di 4-5 anni, la pasta termica sul processore va cambiata (da un tecnico).' }
`)},
	{ID: "fix_game_hz", Name: "Lo schermo va a 60 Hz invece di 144", Desc: "Controlla a quanti Hz va ogni schermo e, se può andare più veloce, ti apre le impostazioni giuste e ti dice il numero da scegliere. Se il massimo è 60 ti dice perché (cavo, porta sbagliata, driver).", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
try {
  if (-not ('TassoHz' -as [type])) { Add-Type -ErrorAction Stop -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class TassoHz {
  [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
  public struct DEVMODE {
    [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 32)] public string dmDeviceName;
    public short dmSpecVersion; public short dmDriverVersion; public short dmSize; public short dmDriverExtra; public int dmFields;
    public int dmPositionX; public int dmPositionY; public int dmDisplayOrientation; public int dmDisplayFixedOutput;
    public short dmColor; public short dmDuplex; public short dmYResolution; public short dmTTOption; public short dmCollate;
    [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 32)] public string dmFormName;
    public short dmLogPixels; public int dmBitsPerPel; public int dmPelsWidth; public int dmPelsHeight; public int dmDisplayFlags; public int dmDisplayFrequency;
    public int dmICMMethod; public int dmICMIntent; public int dmMediaType; public int dmDitherType; public int dmReserved1; public int dmReserved2; public int dmPanningWidth; public int dmPanningHeight;
  }
  [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
  public struct DISPLAY_DEVICE {
    public int cb;
    [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 32)] public string DeviceName;
    [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 128)] public string DeviceString;
    public int StateFlags;
    [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 128)] public string DeviceID;
    [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 128)] public string DeviceKey;
  }
  [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern bool EnumDisplayDevices(string dev, uint i, ref DISPLAY_DEVICE dd, uint flags);
  [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern bool EnumDisplaySettings(string dev, int mode, ref DEVMODE dm);
  [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int ChangeDisplaySettingsEx(string dev, ref DEVMODE dm, IntPtr hwnd, uint flags, IntPtr lParam);
  // Una riga per schermo: "numero|larghezza x altezza|Hz adesso|Hz massimi|esito" (esito: ok, alzato, errore).
  public static string Fix() {
    string outp = ""; int n = 0;
    for (uint i = 0; i < 16; i++) {
      DISPLAY_DEVICE d = new DISPLAY_DEVICE(); d.cb = Marshal.SizeOf(d);
      if (!EnumDisplayDevices(null, i, ref d, 0)) break;
      if ((d.StateFlags & 1) == 0) continue;
      n++;
      DEVMODE cur = new DEVMODE(); cur.dmSize = (short)Marshal.SizeOf(cur);
      if (!EnumDisplaySettings(d.DeviceName, -1, ref cur)) continue;
      int best = cur.dmDisplayFrequency;
      for (int m = 0; m < 2000; m++) {
        DEVMODE x = new DEVMODE(); x.dmSize = (short)Marshal.SizeOf(x);
        if (!EnumDisplaySettings(d.DeviceName, m, ref x)) break;
        if (x.dmPelsWidth == cur.dmPelsWidth && x.dmPelsHeight == cur.dmPelsHeight && x.dmBitsPerPel == cur.dmBitsPerPel && (x.dmDisplayFlags & 2) == 0 && x.dmDisplayFrequency > best && x.dmDisplayFrequency <= 1000) best = x.dmDisplayFrequency;
      }
      string res = "ok";
      if (best > cur.dmDisplayFrequency) res = "puo";
      outp += n + "|" + cur.dmPelsWidth + " x " + cur.dmPelsHeight + "|" + cur.dmDisplayFrequency + "|" + best + "|" + res + "\n";
    }
    return outp;
  }
}
'@ }
} catch { Say 'Non riesco a leggere gli schermi: ti apro le impostazioni. Schermo > Schermo avanzato > "Scegli una frequenza di aggiornamento" e metti il numero più alto.'; Open-User 'ms-settings:display-advanced'; return }
$rows = @(([TassoHz]::Fix()) -split "\n" | Where-Object { $_ })
if (-not $rows.Count) { Say 'Non trovo schermi accesi da controllare.'; return }
$sixty = $false; $up = $false
foreach ($r in $rows) {
  $p = $r -split '\|'
  if ($p[4] -eq 'puo') { $up = $true; Say ('Schermo ' + $p[0] + ' (' + $p[1] + '): va a ' + $p[2] + ' Hz ma può andare a ' + $p[3] + ' Hz. Ti apro le impostazioni: in "Scegli una frequenza di aggiornamento" metti ' + $p[3] + ' Hz. Se lo schermo diventa nero non toccare niente: Windows torna com''era da solo dopo 15 secondi.') }
  else { Say ('Schermo ' + $p[0] + ' (' + $p[1] + '): va già a ' + $p[2] + ' Hz, il massimo che Windows vede.'); if ([int]$p[3] -le 60) { $sixty = $true } }
}
if ($up) { Open-User 'ms-settings:display-advanced' }
if ($sixty) { Say 'Se il tuo schermo è da 120, 144 Hz o più e qui vedi solo 60: 1) usa il cavo DisplayPort, o un HDMI 2.0 o più recente (quelli vecchi arrivano a 60 Hz); 2) collegalo alla scheda video, non alla presa della scheda madre; 3) aggiorna il driver video; 4) nel menu dello schermo controlla che sia attiva la modalità ad alta frequenza.' }
Say 'Nei giochi controlla anche che nelle opzioni video la frequenza sia la più alta (alcuni giochi la tengono a 60).'
`)},
	{ID: "fix_game_gpuswitch", Name: "Il portatile usa la scheda video sbagliata", Desc: "Sui portatili con due schede video (Intel o AMD integrata più NVIDIA o AMD dedicata) controlla quale usano i giochi, se lo schermo è collegato a quella giusta e se il PC risparmia energia. Ti dice come fissare la scheda potente per ogni gioco.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$v = @(Get-CimInstance Win32_VideoController -ErrorAction SilentlyContinue | Where-Object { $_.Name -notmatch 'Basic Display|Microsoft Remote|Virtual|Parsec|Meta' })
$dedicated = @($v | Where-Object { $_.Name -match 'NVIDIA|GeForce|RTX|GTX|Radeon RX|Radeon Pro|Arc(\(TM\))? [AB]\d' })
$integrated = @($v | Where-Object { $_.Name -match 'Intel\(R\) (UHD|HD|Iris)|Intel.*Graphics|Radeon\(TM\) Graphics|Radeon Graphics|Radeon Vega' -and $_.Name -notmatch 'Arc(\(TM\))? [AB]\d' })
foreach ($g in $v) { Say ('Scheda video trovata: ' + $g.Name + $(if ($g.Status -ne 'OK') { ' (stato: ' + $g.Status + ')' } else { '' })) }
if (-not $dedicated.Count) { Say 'Non trovo una scheda video dedicata (NVIDIA o AMD). Se il portatile ne ha una, forse è spenta o senza driver: aggiorna i driver da Risolvi un problema > "Driver di tutto il PC" o dal sito del produttore.'; return }
if (-not $integrated.Count) { Say 'C''è solo la scheda video potente: i giochi usano già quella.'; return }
$bad = @($dedicated | Where-Object { $_.ConfigManagerErrorCode -and $_.ConfigManagerErrorCode -ne 0 })
if ($bad.Count) { Say ('La scheda ' + $bad[0].Name + ' ha un problema di driver (codice ' + $bad[0].ConfigManagerErrorCode + '): finché non si sistema i giochi usano quella integrata. Reinstalla il driver dal sito del produttore.') }
$pref = 'HKCU\Software\Microsoft\DirectX\UserGpuPreferences'
$k = Get-Item -LiteralPath ('Registry::' + (Get-FullKey $pref)) -ErrorAction SilentlyContinue
$saving = @(); if ($k) { foreach ($n in $k.GetValueNames()) { if ($n -ne 'DirectXUserGlobalSettings' -and [string]$k.GetValue($n) -match 'GpuPreference=1;') { $saving += [IO.Path]::GetFileName($n) } } }
if ($saving.Count) { Say ('Questi programmi sono impostati sulla scheda "a risparmio energetico": ' + (($saving | Select-Object -First 8) -join ', ') + '. Se sono giochi, cambiali in Impostazioni > Schermo > Grafica.') }
if (Get-CimInstance Win32_Battery -ErrorAction SilentlyContinue | Where-Object { $_.BatteryStatus -eq 1 }) { Say 'Stai andando a batteria: molti portatili spengono la scheda potente senza il caricatore. Gioca con il caricatore attaccato.' }
Say 'Come fissare la scheda potente: apri Giochi, tocca il gioco > "Prepara a passi" > impostazioni consigliate e scegli la scheda video ad alte prestazioni. Oppure Impostazioni > Schermo > Grafica > scegli il gioco > "Prestazioni elevate".'
Say 'Se il portatile ha la modalità "MUX" o "solo scheda dedicata" (nel programma del produttore: Armoury Crate, Legion Vantage, Omen, Alienware, MSI Center…), attivala per avere più FPS.'
Open-User 'ms-settings:display-advancedgraphics'
`)},
	{ID: "fix_game_overlays", Name: "Programmi sopra al gioco che lo rallentano", Desc: "Trova i programmi che si disegnano sopra al gioco o lo registrano di nascosto (registrazione in sottofondo di Xbox, overlay di NVIDIA, Overwolf, Medal e simili) e spegne la registrazione in sottofondo di Windows. Puoi tornare indietro.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$k1 = 'HKCU\System\GameConfigStore'; $k2 = 'HKCU\Software\Microsoft\Windows\CurrentVersion\GameDVR'
$rec = ((Get-RegValue $k2 'HistoricalCaptureEnabled') -eq 1)
if ($rec) {
  $u = Save-Undo $k2 @('HistoricalCaptureEnabled') 'registrazione-in-sottofondo'
  Set-RegValue $k2 'HistoricalCaptureEnabled' 0 'DWord'
  Say 'Fatto: spenta la "registrazione in sottofondo" di Xbox Game Bar. Registrava sempre gli ultimi minuti di gioco e costava qualche FPS. Puoi ancora registrare a mano con Win + Alt + R.'
  Say-Undo $u
} else { Say 'La registrazione in sottofondo di Windows è già spenta.' }
$known = [ordered]@{ 'Overlay di NVIDIA (GeForce Experience / App NVIDIA)' = 'NVIDIA Overlay|NVIDIA Share'; 'Overwolf' = 'Overwolf|OverwolfBrowser'; 'Medal' = 'Medal'; 'Outplayed' = 'Outplayed'; 'RivaTuner (con MSI Afterburner)' = 'RTSS|RTSSHooksLoader64'; 'Razer Cortex' = 'RazerCortex'; 'AMD Adrenalin (registrazione e overlay)' = 'AMDRSServ|RadeonSoftware'; 'Steelseries Moments' = 'SteelSeriesMoments'; 'Nahimic' = 'Nahimic3|NahimicSvc64'; 'Wallpaper Engine' = 'wallpaper32|wallpaper64'; 'Blitz / Porofessor / Mobalytics' = 'Blitz|Porofessor|Mobalytics' }
$names = @(Get-Process -ErrorAction SilentlyContinue | ForEach-Object { $_.ProcessName })
$hit = @(); foreach ($n in $known.Keys) { if (@($names | Where-Object { $_ -match ('^(' + $known[$n] + ')$') }).Count) { $hit += $n } }
if ($hit.Count) { Say ('Aperti adesso, si disegnano sopra al gioco o lo registrano: ' + ($hit -join ', ') + '. Se il gioco scatta, spegni il loro overlay o la registrazione automatica dalle loro impostazioni, oppure chiudili prima di giocare.') }
else { Say 'Non vedo programmi che si disegnano sopra al gioco tra quelli più comuni.' }
Say 'L''overlay di Steam (Maiusc + Tab) di solito non pesa; se un gioco ha problemi lo spegni in Steam: tasto destro sul gioco > Proprietà > Generali.'
`)},
	{ID: "fix_game_space", Name: "Non c'è spazio per installare il gioco", Desc: "Ti dice quanto spazio libero c'è su ogni disco, quali giochi installati occupano di più e cosa liberare. Svuota i resti di download di Steam rimasti a metà. Giochi e salvataggi non si toccano.", Level: 0, Minutes: 0.4, Timeout: 5 * time.Minute, Script: ofs2(steamLibsPS + `
foreach ($d in @(Get-PSDrive -PSProvider FileSystem -ErrorAction SilentlyContinue | Where-Object { $_.Used -ne $null -and ($_.Free + $_.Used) -gt 5GB })) { Say ('Disco ' + $d.Name + ': liberi ' + (Fmt $d.Free) + ' su ' + (Fmt ($d.Free + $d.Used)) + '.') }
$big = @()
foreach ($l in $libs) {
  $common = Join-Path $l 'steamapps\common'
  foreach ($m in @(Get-ChildItem -LiteralPath (Join-Path $l 'steamapps') -Filter 'appmanifest_*.acf' -File -ErrorAction SilentlyContinue)) {
    $txt = Get-Content -LiteralPath $m.FullName -Raw -ErrorAction SilentlyContinue
    if ($txt -match '"name"\s+"([^"]+)"') { $nm = $Matches[1]; $sz = 0; if ($txt -match '"SizeOnDisk"\s+"(\d+)"') { $sz = [double]$Matches[1] }; if ($sz -gt 0) { $big += [pscustomobject]@{ N = $nm; S = $sz } } }
  }
  $dl = Join-Path $l 'steamapps\downloading'
  # mai attraverso collegamenti: se downloading (o steamapps) porta altrove non tocco niente
  $link = $false
  foreach ($q in @($dl, (Join-Path $l 'steamapps'), $l)) { try { if ((Get-Item -LiteralPath $q -Force -ErrorAction Stop).Attributes -band [IO.FileAttributes]::ReparsePoint) { $link = $true } } catch {} }
  if (-not $link -and (Test-Path -LiteralPath $dl) -and -not (Get-Process -Name steam -ErrorAction SilentlyContinue)) {
    # solo download interi fermi da più di 3 giorni (tutta la cartella del gioco), mai un pezzo
    $f = 0
    foreach ($g in @(Get-ChildItem -LiteralPath $dl -Directory -ErrorAction SilentlyContinue)) {
      $recent = @(Get-ChildItem -LiteralPath $g.FullName -Recurse -File -Force -ErrorAction SilentlyContinue | Where-Object { $_.LastWriteTime -gt (Get-Date).AddDays(-3) } | Select-Object -First 1)
      if (-not $recent.Count) { $sz = Get-FolderSize $g.FullName; Remove-Safe $g.FullName; if (-not (Test-Path -LiteralPath $g.FullName)) { $f += $sz } }
    }
    if ($f -gt 0) { Freed $f; Say ('Svuotati i download di Steam rimasti a metà e fermi da più di 3 giorni (' + (Fmt $f) + '). Steam li riscarica se li riprendi.') }
  }
}
$big = @($big | Sort-Object S -Descending | Select-Object -First 5)
if ($big.Count) { Say ('Giochi di Steam che occupano di più: ' + (($big | ForEach-Object { $_.N + ' ' + (Fmt $_.S) }) -join ', ') + '. Quelli a cui non giochi più li togli da Giochi > tocca il gioco > ⋯ > Disinstalla.') }
Say 'Per liberare spazio senza perdere niente: Pulisci e ripara (pulizia normale), poi Mappa del disco per vedere cosa occupa di più.'
Say 'Se hai un secondo disco con spazio, installa lì il gioco: in Steam scegli la cartella nel momento in cui installi, oppure sposta un gioco già installato da Proprietà > File installati > Sposta cartella.'
`)},
	{ID: "fix_game_nat", Name: "Chat vocale o partite online con altri giocatori non funzionano (NAT)", Desc: "Per \"NAT rigido o moderato\", party chat di Xbox che non si collega o altri giocatori che non riescono a entrare nella tua partita: rimette a posto i servizi di rete che servono ai giochi, controlla la rete e il microfono, e ti dice cosa fare sul router.", Level: 0, Minutes: 0.4, Timeout: 3 * time.Minute, Script: ofs2(undoCmdPS + `
$ns = @('iphlpsvc', 'SSDPSRV', 'upnphost', 'XblAuthManager', 'XboxNetApiSvc')
$td = ''; try { $td = [string](Get-NetTeredoConfiguration -ErrorAction Stop).Type } catch { $td = '' }
# Prima di riaccendere servizi disattivati e Teredo salvo com'erano: con un doppio clic tornano così.
$spenti = @($ns | Where-Object { $v = Get-Service -Name $_ -ErrorAction SilentlyContinue; $v -and [string]$v.StartType -eq 'Disabled' })
$cmds = @($spenti | ForEach-Object { 'sc.exe config ' + $_ + ' start= disabled' })
if ($td -eq 'Disabled') { $cmds += 'netsh.exe interface teredo set state disabled' }
$u = ''; $salvo = $true
if ($cmds.Count) {
  $u = New-UndoCmd $cmds 'rete-chat-giochi' 'Servizi di rete e Teredo rimessi come prima.'
  if (-not $u) { $salvo = $false; Say 'Alcuni servizi di rete per i giochi sono disattivati, ma non sono riuscito a salvare com''erano, quindi non li cambio.' }
}
foreach ($s in $ns) {
  $sv = Get-Service -Name $s -ErrorAction SilentlyContinue
  if ($sv -and $sv.StartType -eq 'Automatic' -and $sv.Status -ne 'Running') { try { Start-Service -Name $s -ErrorAction Stop; Say ('Riacceso il servizio ' + $sv.DisplayName + ', che doveva essere acceso.') } catch {} }
  elseif ($sv -and $sv.StartType -eq 'Disabled' -and $salvo) { try { Set-Service -Name $s -StartupType Manual -ErrorAction Stop; Start-Service -Name $s -ErrorAction SilentlyContinue; Say ('Il servizio ' + $sv.DisplayName + ' era disattivato da un programma: ora parte quando serve.') } catch {} }
}
if ($td -eq 'Disabled' -and $salvo) {
  & netsh.exe interface teredo set state type=default 2>$null | Out-Null
  if ($LASTEXITCODE -eq 0) { Say 'Riattivato "Teredo", il pezzo di Windows che serve alla party chat e al multigiocatore Xbox (era disattivato).' } else { Say 'Teredo, il pezzo di Windows per la party chat Xbox, è disattivato e Windows non mi lascia riattivarlo (forse lo blocca un criterio del PC).' }
}
Say-UndoCmd $u
$pub = @(Get-NetConnectionProfile -ErrorAction SilentlyContinue | Where-Object { $_.NetworkCategory -eq 'Public' -and $_.IPv4Connectivity -eq 'Internet' })
foreach ($p in $pub) { Say ('La rete "' + $p.Name + '" è segnata come Pubblica: per la rete di casa scegli "Privata" in Impostazioni > Rete e Internet > ' + $p.Name + '. Aiuta giochi e chat a collegarsi con gli altri giocatori.') }
$m = Get-Privacy 'microphone'
if ($m.all -eq 'Deny' -or $m.pc -eq 'Deny') { Say 'Windows blocca il microfono a tutte le app: per questo nella chat non ti sentono. Accendilo in Impostazioni > Privacy > Microfono.'; Open-User 'ms-settings:privacy-microphone' }
elseif ($m.desk -eq 'Deny') { Say 'Windows blocca il microfono ai programmi normali (giochi, Steam): accendi "Consenti alle app desktop di accedere al microfono" in Impostazioni > Privacy > Microfono.'; Open-User 'ms-settings:privacy-microphone' }
else { Say 'Il microfono non è bloccato da Windows.' }
Say 'Se il gioco dice ancora "NAT rigido" o "moderato": nel router accendi UPnP (pagina del router, di solito 192.168.1.1). Se hai due router uno dietro l''altro (quello dell''operatore più il tuo), chiedi all''operatore la modalità "bridge". Per Xbox: Impostazioni > Giochi > Rete Xbox ti dice il tipo di NAT.'
`)},
	{ID: "fix_game_crackle", Name: "L'audio gracchia o va a scatti mentre giochi", Desc: "Riavvia l'audio di Windows, spegne il risparmio energetico delle porte USB (causa comune con cuffie e DAC USB) e controlla le cuffie Bluetooth in modalità \"chiamata\". Puoi tornare indietro.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs2(`
Say 'Riavvio l''audio di Windows (si sente un attimo di silenzio)...'
Restart-Service -Name Audiosrv -Force -ErrorAction SilentlyContinue
$usb = Get-PowerValue '2a737441-1930-4402-8d77-b2bebba308a3' '48e6b7a6-50f5-4782-a5d4-53bb8f07e226'
if ($usb.Count -eq 2 -and $usb[0] -ne 0) {
  # Prima di cambiarlo salvo il valore di prima in un file: con un doppio clic torna com'era.
  $plan = [regex]::Match((@(powercfg.exe /getactivescheme 2>$null) -join ' '), '[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}').Value
  $u = ''
  if ($plan) {
    try {
      $d = Join-Path $env:LOCALAPPDATA 'Tasso\backup-registro'
      New-Item -ItemType Directory -Path $d -Force -ErrorAction Stop | Out-Null
      $f = Join-Path $d ('usb-audio_' + (Get-Date -Format 'yyyyMMdd_HHmmss') + '.cmd')
      $lines = @('@echo off', ('"%SystemRoot%\System32\powercfg.exe" /setacvalueindex ' + $plan + ' 2a737441-1930-4402-8d77-b2bebba308a3 48e6b7a6-50f5-4782-a5d4-53bb8f07e226 ' + [int64]$usb[0]), '"%SystemRoot%\System32\powercfg.exe" /setactive SCHEME_CURRENT', 'echo Risparmio energetico delle porte USB rimesso come prima.', 'pause')
      [IO.File]::WriteAllLines($f, [string[]]$lines, [Text.Encoding]::ASCII)
      if (Test-Path -LiteralPath $f) { $u = $f }
    } catch { $u = '' }
  }
  # Salvo il valore di prima anche per "Rimetti com'era" di Tasso: un valore per ogni piano
  # energetico, solo la prima volta (così resta quello vero di prima di Tasso).
  if ($u) {
    try {
      $tk = 'HKCU:\Software\Tasso\AudioUSB'
      if (-not (Test-Path -LiteralPath $tk)) { New-Item -Path $tk -Force -ErrorAction Stop | Out-Null }
      if ((Get-Item -LiteralPath $tk -ErrorAction Stop).GetValue($plan.ToLower(), $null) -eq $null) {
        New-ItemProperty -LiteralPath $tk -Name $plan.ToLower() -Value ([int]$usb[0]) -PropertyType DWord -Force -ErrorAction Stop | Out-Null
      }
    } catch { Remove-Item -LiteralPath $u -Force -ErrorAction SilentlyContinue; $u = '' }
  }
  if (-not $u) { Say 'Non sono riuscito a salvare il valore di prima del risparmio energetico delle porte USB, quindi non lo cambio. Se vuoi spegnerlo tu: Pannello di controllo > Opzioni risparmio energia > Modifica impostazioni avanzate > Impostazioni USB.' } else {
  [void](Run-Hidden 'powercfg.exe' ('/setacvalueindex ' + $plan + ' 2a737441-1930-4402-8d77-b2bebba308a3 48e6b7a6-50f5-4782-a5d4-53bb8f07e226 0'))
  [void](Run-Hidden 'powercfg.exe' '/setactive SCHEME_CURRENT')
  $usb2 = Get-PowerValue '2a737441-1930-4402-8d77-b2bebba308a3' '48e6b7a6-50f5-4782-a5d4-53bb8f07e226'
  if ($usb2.Count -eq 2 -and $usb2[0] -ne 0) { Remove-Item -LiteralPath $u -Force -ErrorAction SilentlyContinue; Say 'Non sono riuscito a spegnere il risparmio energetico delle porte USB: fallo da Pannello di controllo > Opzioni risparmio energia > Modifica impostazioni avanzate > Impostazioni USB.' } else {
  Say 'Fatto: con il caricatore attaccato Windows non spegne più le porte USB per risparmiare. Era la causa più comune di scatti nell''audio di cuffie USB.'; Say-Undo $u }
  }
} else { Say 'Il risparmio energetico delle porte USB è già spento (o non c''è su questo PC).' }
$bt = @(Get-CimInstance Win32_PnPEntity -ErrorAction SilentlyContinue | Where-Object { [string]$_.PNPDeviceID -match '^BTHHFENUM\\|^BTHENUM\\\{0000111E' -and $_.Status -eq 'OK' })
if ($bt.Count) { if ($TassoUfficio) { Say 'Hai cuffie Bluetooth: quando usi il loro microfono passano in modalità "chiamata" e l''audio diventa più basso e meno pulito. È normale con il Bluetooth: per chiamate migliori usa cuffie con il loro ricevitore USB, o un microfono a parte e le cuffie in "Stereo".' } else { Say 'Hai cuffie Bluetooth: quando usi il loro microfono passano in modalità "chiamata" e l''audio diventa basso e gracchiante. Nel gioco o in Windows scegli come uscita le cuffie "Stereo" e usa un altro microfono, oppure usa cuffie con il loro ricevitore USB.' } }
$heavy = @(Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -match '^(Nahimic3|NahimicSvc64|SonicStudio3|RtkAudUService64|DolbyAccess|SteelSeriesGG)$' } | ForEach-Object { $_.ProcessName } | Select-Object -Unique)
if ($heavy.Count) { Say ('Sono aperti programmi che "migliorano" l''audio: ' + ($heavy -join ', ') + $(if ($TassoUfficio) { '. Se gracchia ancora, spegni i loro effetti.' } else { '. Se gracchia ancora, spegni i loro effetti (in particolare Nahimic è noto per dare scatti nei giochi).' })) }
Say 'Altri consigli: collega le cuffie USB a una porta dietro il PC (non a un hub), e in Impostazioni > Sistema > Audio > proprietà del dispositivo prova il formato 24 bit, 48000 Hz.'
`)},
	{ID: "fix_game_inputlag", Name: "C'è ritardo tra il mouse e il gioco (input lag)", Desc: "Controlla le impostazioni di Windows che aggiungono ritardo (Modalità Gioco, frequenza dello schermo, accelerazione del mouse, batteria) e accende quelle che lo riducono. Ti dice cosa cambiare nel gioco.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$gm = 'HKCU\Software\Microsoft\GameBar'
if ((Get-RegValue $gm 'AutoGameModeEnabled') -eq 0) { $u = Save-Undo $gm @('AutoGameModeEnabled') 'modalita-gioco'; Set-RegValue $gm 'AutoGameModeEnabled' 1 'DWord'; Say 'Accesa la Modalità Gioco di Windows: mentre giochi Windows non fa partire aggiornamenti e lavori in sottofondo.'; Say-Undo $u }
else { Say 'La Modalità Gioco di Windows è accesa.' }
$hz = @(Get-CimInstance Win32_VideoController -ErrorAction SilentlyContinue | Where-Object { $_.CurrentRefreshRate -gt 0 } | ForEach-Object { [int]$_.CurrentRefreshRate })
if ($hz.Count -and ($hz | Measure-Object -Maximum).Maximum -le 60) { Say 'Lo schermo va a 60 Hz: se il tuo schermo può andare più veloce usa "Lo schermo va a 60 Hz invece di 144": più Hz = meno ritardo.' }
$hags = Get-RegValue 'HKLM\SYSTEM\CurrentControlSet\Control\GraphicsDrivers' 'HwSchMode'
if ($hags -eq 2) { Say 'La "pianificazione GPU con accelerazione hardware" è accesa: va bene con le schede recenti e serve a DLSS 3 / Frame Generation.' }
if ([string](Get-RegValue 'HKCU\Control Panel\Mouse' 'MouseSpeed') -ne '0') { Say 'L''accelerazione del mouse di Windows è accesa: la mira cambia con la velocità. Spegnila con "La mira con il mouse non è precisa".' }
if (Get-CimInstance Win32_Battery -ErrorAction SilentlyContinue | Where-Object { $_.BatteryStatus -eq 1 }) { Say 'Stai andando a batteria: il PC rallenta per risparmiare e il ritardo aumenta. Gioca con il caricatore.' }
Say 'Nel gioco, le cose che tolgono più ritardo: 1) "NVIDIA Reflex" su "Attivo + Boost" o "AMD Anti-Lag" se c''è; 2) V-Sync spento, con il limite FPS uguale ai Hz dello schermo; 3) schermo intero invece di "finestra"; 4) mouse e tastiera con il cavo o con il ricevitore vicino.'
`)},
	{ID: "fix_game_loading", Name: "I giochi caricano lentamente", Desc: "Controlla su quale disco sono installati i giochi: se sono su un disco meccanico (HDD) te lo dice e ti spiega come spostarli sull'SSD. Controlla anche se il disco è troppo pieno o se Windows lo sta usando al massimo.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs2(steamLibsPS + diskKindPS + `
$letters = @()
foreach ($l in $libs) { if ($l -match '^([A-Za-z]):') { $letters += $Matches[1].ToUpper() } }
foreach ($b in @('Epic Games', 'XboxGames', 'Riot Games', 'EA Games', 'Ubisoft')) { foreach ($d in @(Get-PSDrive -PSProvider FileSystem -ErrorAction SilentlyContinue)) { if (Test-Path -LiteralPath ($d.Root + $b)) { $letters += $d.Name.ToUpper() } } }
$letters = @($letters | Select-Object -Unique)
if (-not $letters.Count) { $letters = @($env:SystemDrive.Trim(':')) }
$ssd = @(); $hdd = @()
foreach ($x in $letters) { $k = Get-DiskKind $x; if ($k -eq 'HDD') { $hdd += $x } elseif ($k -eq 'SSD') { $ssd += $x } }
foreach ($x in $ssd) { Say ('Disco ' + $x + ': (con i giochi) è un SSD, veloce.') }
foreach ($x in $hdd) { Say ('Disco ' + $x + ': (con i giochi) è un disco meccanico (HDD): carica 5-10 volte più piano di un SSD. È la causa più comune dei caricamenti lunghi e delle texture che compaiono tardi.') }
$allSsd = @(); foreach ($d in @(Get-PSDrive -PSProvider FileSystem -ErrorAction SilentlyContinue)) { if ((Get-DiskKind $d.Name) -eq 'SSD' -and $d.Free -gt 30GB) { $allSsd += ($d.Name + ': (' + (Fmt $d.Free) + ' liberi)') } }
if ($hdd.Count -and $allSsd.Count) { Say ('Hai un SSD con spazio: ' + ($allSsd -join ', ') + '. Sposta lì i giochi che usi di più: in Steam tasto destro sul gioco > Proprietà > File installati > Sposta cartella. Non serve riscaricarli.') }
elseif ($hdd.Count) { Say 'Un SSD costa poco ed è il miglioramento più grande per i caricamenti.' }
foreach ($x in $letters) { $d = Get-PSDrive -Name $x -ErrorAction SilentlyContinue; if ($d -and ($d.Free + $d.Used) -gt 0 -and ($d.Free / ($d.Free + $d.Used)) -lt 0.1) { Say ('Il disco ' + $x + ': è pieno oltre il 90%: anche un SSD rallenta quando è quasi pieno. Libera spazio da Pulisci e ripara.') } }
try { $busy = (Get-CimInstance Win32_PerfFormattedData_PerfDisk_PhysicalDisk -ErrorAction Stop | Where-Object { $_.Name -eq '_Total' }).PercentDiskTime; if ($busy -ge 90) { Say 'In questo momento il disco è usato al massimo da qualcosa: guarda in Gestione attività (Ctrl + Maiusc + Esc) > colonna Disco.' } } catch {}
Say 'La prima volta che apri un gioco dopo un aggiornamento è normale che carichi di più: prepara gli "shader" della grafica (lo fa una volta sola).'
`)},
	{ID: "fix_game_fullscreen", Name: "Il gioco non va a schermo intero", Desc: "Per giochi che restano in finestra, con bordi neri, tagliati o che escono dallo schermo intero: controlla il ridimensionamento di Windows e le vecchie impostazioni di compatibilità, e ti spiega i tasti per sistemarlo.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
try { $dpi = [int](Get-RegValue 'HKCU\Control Panel\Desktop\WindowMetrics' 'AppliedDPI'); if (-not $dpi) { Add-Type -Namespace TassoDpi -Name D -MemberDefinition '[DllImport("user32.dll")] public static extern bool SetProcessDPIAware(); [DllImport("user32.dll")] public static extern int GetDpiForSystem();' -ErrorAction Stop; [void][TassoDpi.D]::SetProcessDPIAware(); $dpi = [TassoDpi.D]::GetDpiForSystem() }; if ($dpi -gt 96) { Say ('Il ridimensionamento di Windows è al ' + [int]($dpi * 100 / 96) + '%. Alcuni giochi vecchi per questo escono tagliati o piccoli: tasto destro sul file del gioco > Proprietà > Compatibilità > Modifica impostazioni DPI elevati > spunta "Sostituisci comportamento ridimensionamento DPI" e scegli "Applicazione".') } else { Say 'Il ridimensionamento di Windows è al 100%: va bene per i giochi.' } } catch {}
$lay = 'HKCU\Software\Microsoft\Windows NT\CurrentVersion\AppCompatFlags\Layers'
$k = Get-Item -LiteralPath ('Registry::' + (Get-FullKey $lay)) -ErrorAction SilentlyContinue
$old = @(); if ($k) { foreach ($n in $k.GetValueNames()) { if ([string]$k.GetValue($n) -match '640X480|256COLOR|16BITCOLOR|WIN95|WIN98|WINXP') { $old += [IO.Path]::GetFileName($n) } } }
if ($old.Count) { Say ('Questi programmi hanno vecchie impostazioni di compatibilità (risoluzione bassa o colori ridotti) che possono rovinare lo schermo intero: ' + (($old | Select-Object -First 6) -join ', ') + '. Toglile da tasto destro > Proprietà > Compatibilità.') }
Say 'Tasti utili: Alt + Invio passa da finestra a schermo intero in quasi tutti i giochi. Win + Freccia su ingrandisce la finestra.'
Say 'Nel gioco, nelle opzioni video: scegli "Schermo intero" (o "Esclusivo") e la stessa risoluzione dello schermo. "Finestra senza bordi" è comoda ma a volte ha più ritardo.'
Say 'Se il gioco esce dallo schermo intero da solo: usa "Il gioco si riduce a icona da solo". Se resta con bande nere: nel pannello della scheda video (NVIDIA o AMD) metti il ridimensionamento su "Schermo intero" e "Esegui su GPU".'
`)},
	{ID: "fix_game_controller", Name: "Il controller non viene riconosciuto nel gioco", Desc: "Per controller Xbox, PlayStation e altri che Windows vede ma il gioco no (o che non si collegano): riavvia i controller bloccati e il Bluetooth, controlla i programmi che li \"rubano\" e ti dice come attivare Steam Input.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs2(`
$n = Restart-BadDevices 'HIDClass|XnaComposite|XboxComposite|Bluetooth|USB'
if ($n -gt 0) { Say ('Riavviati ' + $n + ' dispositivi bloccati. Stacca e riattacca il controller.') }
$pads = @(Get-CimInstance Win32_PnPEntity -ErrorAction SilentlyContinue | Where-Object { $_.Name -match 'Xbox|Controller|Gamepad|DualSense|DualShock|Wireless Controller|8BitDo|Joy-Con|Pro Controller' -and $_.Name -notmatch 'Host|SATA|AHCI|Audio|NVM|Storage|PCI|SMBus|USB|Bluetooth|RAID|Ethernet|Network|Wi-?Fi|Thunderbolt' -and $_.Status -eq 'OK' } | ForEach-Object { $_.Name } | Select-Object -Unique)
if ($pads.Count) { Say ('Windows vede: ' + (($pads | Select-Object -First 4) -join ', ') + '.') } else { Say 'Windows non vede nessun controller acceso. Prova un altro cavo (molti cavi servono solo a caricare e non portano i dati) o un''altra porta USB.' }
$bt = Get-Service -Name bthserv -ErrorAction SilentlyContinue
if ($bt -and $bt.Status -ne 'Running' -and $bt.StartType -ne 'Disabled') { Start-Service -Name bthserv -ErrorAction SilentlyContinue; Say 'Riacceso il servizio Bluetooth di Windows.' }
$ds = @(Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -match '^(DS4Windows|DSX|InputMapper|x360ce|reWASD.*|JoyToKey)$' } | ForEach-Object { $_.ProcessName } | Select-Object -Unique)
if ($ds.Count) { Say ('È aperto ' + ($ds -join ', ') + ': trasforma il controller in un altro. Insieme a Steam Input il gioco può vedere due controller o nessuno: chiudilo o spegni Steam Input per quel gioco.') }
$isPs = @($pads | Where-Object { $_ -match 'DualSense|DualShock|Wireless Controller' }).Count -gt 0
if ($isPs) { Say 'Controller PlayStation: molti giochi su PC capiscono solo quelli Xbox. Aprili da Steam e attiva Steam Input: Steam > Impostazioni > Controller > "Supporto per la configurazione PlayStation".' }
Say 'Per i giochi di Steam: tasto destro sul gioco > Proprietà > Controller > prova "Attiva Steam Input" (o disattivalo se il controller fa cose strane). Molti giochi riconoscono il controller solo se è acceso PRIMA di aprire il gioco.'
`)},
	{ID: "fix_game_freeze", Name: "Il gioco si blocca durante il caricamento", Desc: "Controlla la memoria (RAM) e il file di paging, che quando è spento o troppo piccolo fa bloccare i giochi grandi, e se il disco è pieno o lento. Ti dice cosa sistemare.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs2(`
$cs = Get-CimInstance Win32_ComputerSystem -ErrorAction SilentlyContinue
$os = Get-CimInstance Win32_OperatingSystem -ErrorAction SilentlyContinue
$ram = 0; if ($cs) { $ram = [double]$cs.TotalPhysicalMemory }
if ($ram -gt 0) { Say ('RAM: ' + (Fmt $ram) + '.'); if ($ram -lt 15GB) { if ($TassoUfficio) { Say 'Con meno di 16 GB di RAM i file molto grandi possono far bloccare il PC: chiudi i programmi e le schede del browser che non usi.' } else { Say 'Con meno di 16 GB di RAM i giochi recenti possono bloccarsi nei caricamenti: chiudi browser e altri programmi prima di giocare (il Boost di Tasso lo fa per te).' } } }
if ($os) { $free = [double]$os.FreePhysicalMemory * 1KB; if ($free -lt 2GB) { Say ('Adesso sono liberi solo ' + (Fmt $free) + ' di RAM: qualcosa sta usando molta memoria.') } }
$pf = @(Get-CimInstance Win32_PageFileUsage -ErrorAction SilentlyContinue)
if ($cs -and -not $cs.AutomaticManagedPagefile -and -not $pf.Count) { if ($TassoUfficio) { Say 'Trovato: il file di paging è SPENTO. È la causa più comune del PC che si blocca con i file grandi. Rimettilo: Impostazioni > Sistema > Informazioni > Impostazioni di sistema avanzate > Prestazioni > Impostazioni > Avanzate > Memoria virtuale > Cambia > spunta "Gestisci automaticamente".' } else { Say 'Trovato: il file di paging è SPENTO. È la causa più comune dei giochi che si bloccano o si chiudono durante il caricamento. Rimettilo: Impostazioni > Sistema > Informazioni > Impostazioni di sistema avanzate > Prestazioni > Impostazioni > Avanzate > Memoria virtuale > Cambia > spunta "Gestisci automaticamente".' }; Start-Process -FilePath 'SystemPropertiesPerformance.exe' -ErrorAction SilentlyContinue }
elseif ($cs -and -not $cs.AutomaticManagedPagefile -and $pf.Count) { $tot = 0; foreach ($p in $pf) { $tot += [double]$p.AllocatedBaseSize * 1MB }; if ($tot -lt 8GB) { Say ('Il file di paging è impostato a mano a ' + (Fmt $tot) + $(if ($TassoUfficio) { ': è poco. Meglio "Gestisci automaticamente" (Prestazioni > Avanzate > Memoria virtuale).' } else { ': per i giochi è poco. Meglio "Gestisci automaticamente" (Prestazioni > Avanzate > Memoria virtuale).' })) } else { Say ('File di paging impostato a mano a ' + (Fmt $tot) + ': va bene.') } }
else { Say 'Il file di paging è gestito da Windows: va bene.' }
$c = Get-PSDrive -Name ($env:SystemDrive.Trim(':')) -ErrorAction SilentlyContinue
if ($c -and $c.Free -lt 10GB) { Say ('Sul disco C: ci sono solo ' + (Fmt $c.Free) + $(if ($TassoUfficio) { ' liberi: il file di paging non riesce a crescere e il PC si blocca. Libera spazio da Pulisci e ripara.' } else { ' liberi: il file di paging non riesce a crescere e il gioco si blocca. Libera spazio da Pulisci e ripara.' })) }
if ($TassoUfficio) { Say 'Se si blocca sempre con lo stesso file: fanne una copia sul disco del PC (non in rete o su OneDrive) e apri quella. Se il disco è meccanico (HDD), con i file grandi è normale che sia lento.' }
else { Say 'Se si blocca sempre allo stesso punto: verifica i file del gioco (Steam: Proprietà > File installati > Verifica integrità; Epic: ⋯ > Gestisci > Verifica). Se è la prima apertura dopo un aggiornamento, aspetta qualche minuto: sta preparando la grafica.' }
`)},
}

func init() {
	insertStepAfter("fix_game_download", gameFixSteps4...)
}
