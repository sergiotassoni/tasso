package main

// Modalità Ufficio (2.6): i 50 problemi più comuni dei PC da lavoro, ognuno con un pulsante in
// Risolvi problemi. Solo azioni sicure: niente documenti cancellati (al massimo spostati in una
// cartella di recupero), impostazioni del registro salvate prima di cambiarle, programmi di
// Office mai chiusi se sono aperti (potresti perdere il lavoro). Dove una cosa non si può fare
// in automatico Tasso controlla, lo dice chiaro e spiega cosa fare.

import "time"

// ofHelpers: funzioni in comune agli script dell'ufficio (si aggiungono a psPrelude).
const ofHelpers = `
$OfficeProcs = 'WINWORD','EXCEL','POWERPNT','OUTLOOK','MSACCESS','ONENOTE','MSPUB','VISIO','WINPROJ'
$OfficeNames = @{ WINWORD = 'Word'; EXCEL = 'Excel'; POWERPNT = 'PowerPoint'; OUTLOOK = 'Outlook'; MSACCESS = 'Access'; ONENOTE = 'OneNote'; MSPUB = 'Publisher'; VISIO = 'Visio'; WINPROJ = 'Project' }
function Get-OpenOffice([string[]]$names = $OfficeProcs) {
  return @(Get-Process -Name $names -ErrorAction SilentlyContinue | ForEach-Object { $n = $OfficeNames[$_.ProcessName.ToUpper()]; if ($n) { $n } else { $_.ProcessName } } | Select-Object -Unique)
}
function Stop-IfOfficeOpen([string[]]$names = $OfficeProcs) {
  # Mai chiudere Office al posto tuo: potresti perdere un documento non salvato.
  $o = Get-OpenOffice $names
  if ($o.Count) { Say ('Prima salva il lavoro e chiudi: ' + ($o -join ', ') + '. Poi premi di nuovo Risolvi. (Tasso non lo chiude da solo per non farti perdere niente.)'); return $true }
  return $false
}
function Open-User([string]$target) {
  # Esplora file gira senza permessi di amministratore: quello che apre parte come utente normale.
  Start-Process -FilePath (Join-Path $env:SystemRoot 'explorer.exe') -ArgumentList ('"' + $target + '"')
}
function Start-AsUser([string]$exe, [string]$arg) {
  # Programma con argomenti come utente normale: passo da un collegamento temporaneo aperto da Esplora file.
  if (-not $exe -or -not (Test-Path -LiteralPath $exe)) { return $false }
  if (-not $arg) { Open-User $exe; return $true }
  $d = Join-Path $env:LOCALAPPDATA 'Tasso\tmp'
  New-Item -ItemType Directory -Path $d -Force | Out-Null
  Get-ChildItem -LiteralPath $d -Filter 'apri-*.lnk' -ErrorAction SilentlyContinue | Where-Object { $_.LastWriteTime -lt (Get-Date).AddMinutes(-2) } | Remove-Item -Force -ErrorAction SilentlyContinue
  $lnk = Join-Path $d ('apri-' + [guid]::NewGuid().ToString('N') + '.lnk')
  try { $sh = New-Object -ComObject WScript.Shell; $l = $sh.CreateShortcut($lnk); $l.TargetPath = $exe; $l.Arguments = $arg; $l.WorkingDirectory = (Split-Path $exe -Parent); $l.Save() } catch { return $false }
  Open-User $lnk
  Start-Sleep -Seconds 15
  Remove-Item -LiteralPath $lnk -Force -ErrorAction SilentlyContinue
  return $true
}
function Get-AppPath([string]$exe) {
  foreach ($r in @('HKCU:\Software\Microsoft\Windows\CurrentVersion\App Paths\', 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\', 'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\App Paths\')) {
    try { $v = ([string](Get-ItemProperty -LiteralPath ($r + $exe) -ErrorAction Stop).'(default)').Trim('"'); if ($v -and (Test-Path -LiteralPath $v)) { return $v } } catch {}
  }
  return ''
}
function Get-C2R {
  foreach ($p in @((Join-Path $env:CommonProgramFiles 'microsoft shared\ClickToRun'), (Join-Path ${env:CommonProgramFiles(x86)} 'microsoft shared\ClickToRun'))) {
    if ($p -and (Test-Path -LiteralPath (Join-Path $p 'OfficeClickToRun.exe'))) { return $p }
  }
  return ''
}
function Get-C2RConf([string]$name) {
  try { return [string](Get-ItemProperty -LiteralPath 'HKLM:\SOFTWARE\Microsoft\Office\ClickToRun\Configuration' -Name $name -ErrorAction Stop).$name } catch { return '' }
}
function Wait-Cmdline([string]$like, [int]$maxMin) {
  # Aspetta che finisca il programma che ha quel pezzo nella riga di comando (Office lavora in sottofondo).
  $end = (Get-Date).AddMinutes($maxMin); $seen = $false; $last = Get-Date
  while ((Get-Date) -lt $end) {
    $p = @(Get-CimInstance Win32_Process -Filter "Name='OfficeClickToRun.exe' OR Name='OfficeC2RClient.exe'" -ErrorAction SilentlyContinue | Where-Object { [string]$_.CommandLine -like $like })
    if ($p.Count) { $seen = $true } elseif ($seen -or ((Get-Date) - $last).TotalSeconds -gt 40) { break }
    if (((Get-Date) - $last).TotalMinutes -ge 2) { Say 'Office ci sta ancora lavorando, aspetta...'; $last = Get-Date }
    Start-Sleep -Seconds 5
  }
  return $seen
}
function Backup-Reg([string]$key, [string]$name) {
  # Copia di sicurezza di una chiave del registro: con doppio clic sul file torna com'era.
  $d = Join-Path $env:LOCALAPPDATA 'Tasso\backup-registro'
  New-Item -ItemType Directory -Path $d -Force | Out-Null
  $f = Join-Path $d ($name + '_' + (Get-Date -Format 'yyyyMMdd_HHmmss') + '.reg')
  $c = Run-Hidden 'reg.exe' ('export "' + $key + '" "' + $f + '" /y')
  if ($c -eq 0 -and (Test-Path -LiteralPath $f)) { return $f }
  return ''
}
function Restart-BadDevices([string]$cls) {
  $n = 0
  foreach ($d in @(Get-CimInstance Win32_PnPEntity -ErrorAction SilentlyContinue | Where-Object { $_.PNPClass -match $cls -and $_.ConfigManagerErrorCode -ne 0 -and $_.ConfigManagerErrorCode -ne 22 -and $_.ConfigManagerErrorCode -ne 45 })) {
    $id = [string]$d.PNPDeviceID
    if ($id -match '^[A-Za-z0-9\\&_.{}#\- ]{3,300}$') { Say ('Riavvio: ' + $d.Name); [void](Run-Hidden 'pnputil.exe' ('/restart-device "' + $id + '"')); $n++ }
  }
  return $n
}
function Test-Tcp([string]$h, [int]$port, [int]$ms = 2000) {
  try { $c = New-Object System.Net.Sockets.TcpClient; $a = $c.BeginConnect($h, $port, $null, $null); $ok = $a.AsyncWaitHandle.WaitOne($ms) -and $c.Connected; $c.Close(); return $ok } catch { return $false }
}
function Test-Internet {
  # Prima diretto; poi con il proxy di Windows (nelle reti d'ufficio spesso si esce solo da lì).
  if (Test-Tcp 'www.microsoft.com' 443) { return $true }
  try { $r = [Net.WebRequest]::Create('http://www.msftconnecttest.com/connecttest.txt'); $r.Proxy = [Net.WebRequest]::GetSystemWebProxy(); $r.Proxy.Credentials = [Net.CredentialCache]::DefaultCredentials; $r.Timeout = 6000; $r.GetResponse().Close(); return $true } catch { return $false }
}
function Restart-Explorer {
  # Windows fa ripartire da solo Esplora file (barra, desktop) come utente normale.
  Get-Process -Name explorer -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
  Start-Sleep -Seconds 5
  if (-not (Get-Process -Name explorer -ErrorAction SilentlyContinue)) { Start-Process -FilePath (Join-Path $env:SystemRoot 'explorer.exe') }
}
function Get-Privacy([string]$what) {
  $b = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\' + $what
  $m = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\' + $what
  $r = @{ all = ''; desk = ''; pc = '' }
  try { $r.all = [string](Get-ItemProperty -LiteralPath $b -Name Value -ErrorAction Stop).Value } catch {}
  try { $r.desk = [string](Get-ItemProperty -LiteralPath ($b + '\NonPackaged') -Name Value -ErrorAction Stop).Value } catch {}
  try { $r.pc = [string](Get-ItemProperty -LiteralPath $m -Name Value -ErrorAction Stop).Value } catch {}
  return $r
}
function Use-Audio {
  # Volume e "muto" di casse e microfono predefiniti (API audio di Windows).
  if ('TassoAudio' -as [type]) { return $true }
  try {
    Add-Type -ErrorAction Stop -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
[Guid("5CDF2C82-841E-4546-9722-0CF74078229A"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
interface IAudioEndpointVolume {
  int f(); int g(); int h(); int i();
  int SetMasterVolumeLevelScalar(float fLevel, Guid pguidEventContext);
  int j();
  int GetMasterVolumeLevelScalar(out float pfLevel);
  int k(); int l(); int m(); int n();
  int SetMute([MarshalAs(UnmanagedType.Bool)] bool bMute, Guid pguidEventContext);
  int GetMute([MarshalAs(UnmanagedType.Bool)] out bool pbMute);
}
[Guid("D666063F-1587-4E43-81F1-B948E807363F"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
interface IMMDevice { int Activate(ref Guid id, int clsCtx, int activationParams, out IAudioEndpointVolume aev); }
[Guid("A95664D2-9614-4F35-A746-DE8DB63617E6"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
interface IMMDeviceEnumerator { int f(); int GetDefaultAudioEndpoint(int dataFlow, int role, out IMMDevice endpoint); }
[ComImport, Guid("BCDE0395-E52F-467C-8E3D-C4579291692E")] class MMDeviceEnumeratorComObject { }
public static class TassoAudio {
  static IAudioEndpointVolume Vol(int flow, int role) {
    IMMDeviceEnumerator e = new MMDeviceEnumeratorComObject() as IMMDeviceEnumerator;
    IMMDevice dev = null;
    Marshal.ThrowExceptionForHR(e.GetDefaultAudioEndpoint(flow, role, out dev));
    IAudioEndpointVolume v = null;
    Guid id = typeof(IAudioEndpointVolume).GUID;
    Marshal.ThrowExceptionForHR(dev.Activate(ref id, 23, 0, out v));
    return v;
  }
  // flow: 0 casse, 1 microfono. role: 0 normale, 2 chiamate. Risposta: "muto|volume%" oppure "" se non c'è.
  public static string State(int flow, int role) {
    try { IAudioEndpointVolume v = Vol(flow, role); float l; bool m; v.GetMasterVolumeLevelScalar(out l); v.GetMute(out m); return (m ? "1" : "0") + "|" + ((int)Math.Round(l * 100)).ToString(); } catch { return ""; }
  }
  public static bool Fix(int flow, int role, float min, float set) {
    try { IAudioEndpointVolume v = Vol(flow, role); v.SetMute(false, Guid.Empty); float l; v.GetMasterVolumeLevelScalar(out l); if (l < min) v.SetMasterVolumeLevelScalar(set, Guid.Empty); return true; } catch { return false; }
  }
}
'@
    return $true
  } catch { return $false }
}
function Use-Display {
  if ('TassoDisplay' -as [type]) { return $true }
  try {
    Add-Type -ErrorAction Stop -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class TassoDisplay {
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
  [DllImport("user32.dll")] static extern int SetDisplayConfig(uint np, IntPtr pa, uint nm, IntPtr ma, uint flags);
  // 0x80 = applica; 0x2 = duplica, 0x4 = estendi. Come Win+P.
  public static int Topology(uint t) { return SetDisplayConfig(0, IntPtr.Zero, 0, IntPtr.Zero, 0x80 | t); }
  // Rimette dritti gli schermi girati. Risposta: nomi degli schermi sistemati, separati da ";".
  public static string Straighten() {
    string done = "";
    for (uint i = 0; i < 16; i++) {
      DISPLAY_DEVICE d = new DISPLAY_DEVICE(); d.cb = Marshal.SizeOf(d);
      if (!EnumDisplayDevices(null, i, ref d, 0)) break;
      if ((d.StateFlags & 1) == 0) continue;
      DEVMODE m = new DEVMODE(); m.dmSize = (short)Marshal.SizeOf(m);
      if (!EnumDisplaySettings(d.DeviceName, -1, ref m)) continue;
      if (m.dmDisplayOrientation == 0) continue;
      if (m.dmDisplayOrientation == 1 || m.dmDisplayOrientation == 3) { int w = m.dmPelsWidth; m.dmPelsWidth = m.dmPelsHeight; m.dmPelsHeight = w; }
      m.dmDisplayOrientation = 0;
      m.dmFields = 0x80 | 0x80000 | 0x100000;
      if (ChangeDisplaySettingsEx(d.DeviceName, ref m, IntPtr.Zero, 1, IntPtr.Zero) == 0) done += d.DeviceName + ";";
    }
    return done;
  }
}
'@
    return $true
  } catch { return $false }
}
`

func ofs(s string) string { return ofHelpers + s }

var officeSteps = []Step{
	// ---------- Stampa e scanner ----------
	{ID: "of_print_offline", Name: "La stampante dice \"offline\"", Desc: "Riavvia il servizio di stampa, toglie la spunta \"Usa stampante offline\" e controlla se le stampanti di rete rispondono.", Level: 0, Minutes: 0.4, Timeout: 4 * time.Minute, Script: ofs(`
Say 'Riavvio il servizio di stampa di Windows...'
Restart-Service -Name Spooler -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 2
$pr = @(Get-CimInstance Win32_Printer -ErrorAction SilentlyContinue | Where-Object { $_.Name -notmatch 'PDF|XPS|OneNote|Fax' })
if (-not $pr.Count) { Say 'Non trovo nessuna stampante installata: ti apro le impostazioni per aggiungerla (Aggiungi dispositivo).'; Open-User 'ms-settings:printers'; return }
foreach ($p in $pr) {
  if ($p.WorkOffline) {
    try { Set-CimInstance -InputObject $p -Property @{ WorkOffline = $false } -ErrorAction Stop; Say ($p.Name + ': tolta la spunta "Usa stampante offline".') }
    catch { Say ($p.Name + ': è messa offline a mano. Togli la spunta da Impostazioni > Stampanti > ' + $p.Name + ' > Apri coda di stampa > menu Stampante.') }
  }
}
foreach ($p in @(Get-Printer -ErrorAction SilentlyContinue | Where-Object { $_.Name -notmatch 'PDF|XPS|OneNote|Fax' })) {
  $port = Get-PrinterPort -Name $p.PortName -ErrorAction SilentlyContinue
  $h = [string]$port.PrinterHostAddress
  if (-not $h -and [string]$p.PortName -match '^(IP_)?(\d{1,3}(\.\d{1,3}){3})$') { $h = $Matches[2] }
  if ($h) {
    $ok = (Test-Tcp $h 9100) -or (Test-Tcp $h 631) -or (Test-Tcp $h 80)
    if ($ok) { Say ($p.Name + ' (' + $h + '): risponde in rete.') }
    else { Say ($p.Name + ' (' + $h + '): NON risponde. Controlla che sia accesa e collegata alla stessa rete (stesso Wi-Fi o cavo dell''ufficio). Spegnila 30 secondi e riaccendila.') }
  } elseif ([string]$p.PortName -match '^WSD') { Say ($p.Name + ': collegata in rete in modo automatico (WSD). Se resta offline spesso, reinstallala con il suo indirizzo IP (lo trovi nel menu della stampante): Aggiungi dispositivo > "La stampante desiderata non è nell''elenco".')
  } elseif ([string]$p.PortName -match '^USB') { Say ($p.Name + ': collegata con il cavo USB. Controlla che il cavo sia attaccato e la stampante accesa.') }
}
Say 'Fatto. Riprova a stampare. Se resta offline, spegni e riaccendi la stampante e poi premi di nuovo Risolvi.'
`)},
	{ID: "of_print_default", Name: "Stampa sempre sulla stampante sbagliata", Desc: "Spegne \"Lascia che Windows gestisca la stampante predefinita\" (che la cambia da solo) e ti fa scegliere quella giusta una volta per tutte.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs(`
$k = 'HKCU:\Software\Microsoft\Windows NT\CurrentVersion\Windows'
$v = $null; try { $v = (Get-ItemProperty -LiteralPath $k -Name LegacyDefaultPrinterMode -ErrorAction Stop).LegacyDefaultPrinterMode } catch {}
if ($v -ne 1) { Set-ItemProperty -LiteralPath $k -Name LegacyDefaultPrinterMode -Value 1 -Type DWord -ErrorAction SilentlyContinue; Say 'Spento "Lascia che Windows gestisca la stampante predefinita": da ora Windows non la cambia più da solo.' }
else { Say 'Windows già non cambiava da solo la stampante predefinita.' }
$all = @(Get-CimInstance Win32_Printer -ErrorAction SilentlyContinue)
$real = @($all | Where-Object { $_.Name -notmatch 'PDF|XPS|OneNote|Fax' })
$def = @($all | Where-Object { $_.Default })[0]
if ($def) { Say ('Ora la stampante predefinita è: ' + $def.Name) }
if ($real.Count -eq 1 -and (-not $def -or $def.Name -ne $real[0].Name)) {
  $r = Invoke-CimMethod -InputObject $real[0] -MethodName SetDefaultPrinter -ErrorAction SilentlyContinue
  if ($r -and $r.ReturnValue -eq 0) { Say ('Hai una sola stampante vera: l''ho messa come predefinita (' + $real[0].Name + ').') }
} elseif ($real.Count -gt 1) {
  Say ('Stampanti trovate: ' + (($real | ForEach-Object { $_.Name }) -join ', ') + '.')
  Say 'Ti apro le impostazioni: clicca la stampante giusta e poi "Imposta come predefinita". Resterà quella.'
  Open-User 'ms-settings:printers'
}
Say 'Per tornare come prima: Impostazioni > Stampanti e scanner > riaccendi "Lascia che Windows gestisca la stampante predefinita".'
`)},
	{ID: "of_print_pdf", Name: "Manca \"Stampa su PDF\"", Desc: "Rimette la stampante \"Microsoft Print to PDF\", quella che salva qualsiasi documento come PDF.", Level: 0, Minutes: 1, Timeout: 10 * time.Minute, Script: ofs(`
$fn = 'Printing-PrintToPDFServices-Features'
$has = { @(Get-Printer -ErrorAction SilentlyContinue | Where-Object { $_.Name -eq 'Microsoft Print to PDF' -or $_.DriverName -eq 'Microsoft Print To PDF' }).Count -gt 0 }
if (& $has) { Say '"Microsoft Print to PDF" c''è già: quando stampi sceglila nell''elenco delle stampanti.'; return }
Say 'Rimetto la funzione di Windows "Stampa su PDF"...'
$f = Get-WindowsOptionalFeature -Online -FeatureName $fn -ErrorAction SilentlyContinue
if ($f -and [string]$f.State -eq 'Enabled') { Disable-WindowsOptionalFeature -Online -FeatureName $fn -NoRestart -ErrorAction SilentlyContinue | Out-Null }
Enable-WindowsOptionalFeature -Online -FeatureName $fn -NoRestart -All -ErrorAction SilentlyContinue | Out-Null
Start-Sleep -Seconds 2
if (-not (& $has)) { try { Add-Printer -Name 'Microsoft Print to PDF' -DriverName 'Microsoft Print To PDF' -PortName 'PORTPROMPT:' -ErrorAction Stop } catch {} }
if (& $has) { Say 'Fatto: "Microsoft Print to PDF" è tornata. Quando stampi scegli lei e salva il file.' }
else { Say 'Windows non me l''ha lasciata rimettere ora: riavvia il PC e premi di nuovo Risolvi.'; [Console]::Out.WriteLine('REBOOT') }
`)},
	{ID: "of_print_find", Name: "Non trovo la stampante (o i PC) in rete", Desc: "Accende i servizi di Windows che cercano stampanti e PC nella rete dell'ufficio. Se la rete è \"Pubblica\" ti spiega perché sono nascosti.", Level: 0, Minutes: 0.4, Timeout: 4 * time.Minute, Script: ofs(`
Say 'Accendo i servizi che cercano stampanti e computer in rete...'
foreach ($s in @('Dnscache', 'FDResPub', 'fdPHost', 'SSDPSRV', 'upnphost', 'LanmanWorkstation')) {
  $sv = Get-Service -Name $s -ErrorAction SilentlyContinue
  if ($sv -and $sv.StartType -ne 'Disabled' -and $sv.Status -ne 'Running') { Start-Service -Name $s -ErrorAction SilentlyContinue }
  if ($sv -and $sv.StartType -eq 'Disabled') { Say ('Il servizio ' + $sv.DisplayName + ' è disattivato: se il PC è dell''azienda l''ha deciso l''assistenza (IT), chiedi a loro.') }
}
$np = @(Get-NetConnectionProfile -ErrorAction SilentlyContinue)
foreach ($n in $np) {
  $cat = [string]$n.NetworkCategory
  if ($cat -eq 'DomainAuthenticated') { Say ('Rete "' + $n.Name + '": è la rete dell''azienda, la gestisce l''assistenza (IT). Se non vedi la stampante chiedi a loro il nome da aggiungere.') }
  elseif ($cat -eq 'Public') { Say ('Rete "' + $n.Name + '": è segnata come PUBBLICA, così Windows nasconde stampanti e PC per sicurezza. Se è la rete del tuo ufficio o di casa, mettila su "Privata": Impostazioni > Rete e Internet > Proprietà della rete.'); $pub = $true }
  else {
    Get-NetFirewallRule -Group '@FirewallAPI.dll,-32752' -ErrorAction SilentlyContinue | Where-Object { [string]$_.Profile -match 'Private' -and [string]$_.Enabled -ne 'True' } | Enable-NetFirewallRule -ErrorAction SilentlyContinue
    Say ('Rete "' + $n.Name + '" (privata): accesa la "Individuazione rete", così il PC vede stampanti e computer vicini (se l''ufficio non la blocca).')
  }
}
Say 'Ora vai su Impostazioni > Stampanti e scanner > "Aggiungi dispositivo" (te lo apro). Se non compare, chiedi l''indirizzo IP della stampante e scegli "La stampante desiderata non è nell''elenco".'
Open-User 'ms-settings:printers'
`)},
	{ID: "of_print_usb", Name: "La stampante USB non si vede", Desc: "Fa ricercare a Windows le stampanti collegate col cavo, riavvia quelle bloccate e il servizio di stampa.", Level: 0, Minutes: 0.4, Timeout: 4 * time.Minute, Script: ofs(`
Say 'Cerco stampanti e porte USB bloccate...'
$n = Restart-BadDevices 'Printer|PrintQueue|USB'
if ($n -eq 0) { Say 'Nessuna stampante o porta USB risulta rotta per Windows.' }
Say 'Faccio ricercare a Windows i dispositivi collegati...'
[void](Run-Hidden 'pnputil.exe' '/scan-devices')
Restart-Service -Name Spooler -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 3
$usb = @(Get-Printer -ErrorAction SilentlyContinue | Where-Object { [string]$_.PortName -match '^USB' })
if ($usb.Count) { Say ('Stampanti col cavo trovate: ' + (($usb | ForEach-Object { $_.Name }) -join ', ') + '.') }
else { Say 'Non vedo stampanti col cavo USB. Prova così: stampante accesa, stacca il cavo, aspetta 10 secondi, attaccalo in un''altra porta (meglio dietro il PC, non in un hub). Windows la installa da solo in un minuto.' }
`)},
	{ID: "of_scan", Name: "Lo scanner non funziona", Desc: "Riavvia il servizio di Windows per scanner e fotocamere, sblocca gli scanner bloccati e ti apre il programma per scansionare.", Level: 0, Minutes: 0.4, Timeout: 4 * time.Minute, Script: ofs(`
Say 'Riavvio il servizio di Windows per scanner (Acquisizione immagini)...'
$sv = Get-Service -Name stisvc -ErrorAction SilentlyContinue
if ($sv -and $sv.StartType -eq 'Disabled') { Set-Service -Name stisvc -StartupType Manual -ErrorAction SilentlyContinue; Say 'Il servizio degli scanner era spento del tutto: l''ho rimesso come lo vuole Windows (manuale).' }
Restart-Service -Name stisvc -Force -ErrorAction SilentlyContinue
Start-Service -Name stisvc -ErrorAction SilentlyContinue
Restart-Service -Name ShellHWDetection -Force -ErrorAction SilentlyContinue
$n = Restart-BadDevices 'Image|USB'
[void](Run-Hidden 'pnputil.exe' '/scan-devices')
$sc = @(Get-PnpDevice -Class Image -PresentOnly -ErrorAction SilentlyContinue | Where-Object { $_.FriendlyName -notmatch 'Camera|Webcam|IR' })
if ($sc.Count) { Say ('Scanner trovati: ' + (($sc | ForEach-Object { $_.FriendlyName }) -join ', ') + '.') } else { Say 'Non vedo nessuno scanner: se è una multifunzione in rete, deve essere accesa e sulla stessa rete. Molte multifunzioni scansionano solo con il programma del produttore.' }
$wfs = Join-Path $env:SystemRoot 'System32\WFS.exe'
if (Test-Path -LiteralPath $wfs) { Say 'Ti apro "Fax e scanner di Windows": Nuova analisi > scegli lo scanner.'; Open-User $wfs }
else { Say 'Per scansionare puoi usare l''app gratuita "Scanner di Windows" dal Microsoft Store.' }
`)},

	// ---------- Word, Excel e Office ----------
	{ID: "of_office_repair", Name: "Office si blocca o dà errori", Desc: "Fa la \"riparazione rapida\" ufficiale di Microsoft Office, senza internet. Documenti e impostazioni restano. Office deve essere chiuso.", Level: 0, Minutes: 8, Timeout: 40 * time.Minute, Script: ofs(`
if (Stop-IfOfficeOpen) { return }
$c = Get-C2R
if (-not $c) { Say 'Non trovo Office "a clic" (Microsoft 365, 2016-2024) su questo PC: forse è una versione vecchia o dello Store. Ti apro le app installate: cerca Microsoft Office > Modifica > Ripristina.'; Open-User 'ms-settings:appsfeatures'; return }
$plat = Get-C2RConf 'Platform'; if (-not $plat) { $plat = 'x64' }
$cul = Get-C2RConf 'ClientCulture'; if (-not $cul) { $cul = 'it-it' }
Say 'Faccio la riparazione rapida di Office (5-10 minuti, non serve internet). Non aprire Word, Excel o Outlook finché non ho finito.'
[void](Run-Hidden (Join-Path $c 'OfficeClickToRun.exe') ('scenario=Repair platform=' + $plat + ' culture=' + $cul + ' RepairType=QuickRepair forceappshutdown=False DisplayLevel=False'))
[void](Wait-Cmdline '*scenario=Repair*' 35)
Say 'Fatto: riparazione rapida finita. Riapri il programma che dava problemi.'
Say 'Se si blocca ancora: Impostazioni > App installate > Microsoft 365 > Modifica > "Ripristino online" (serve internet, mezz''ora).'
`)},
	{ID: "of_office_update", Name: "Aggiorna Office", Desc: "Scarica e installa gli aggiornamenti ufficiali di Word, Excel, Outlook e PowerPoint. Molti errori spariscono così.", Level: 0, Minutes: 6, Timeout: 40 * time.Minute, Script: ofs(`
$c = Get-C2R
if (-not $c -or -not (Test-Path -LiteralPath (Join-Path $c 'OfficeC2RClient.exe'))) { Say 'Su questo PC Office non è la versione "a clic": si aggiorna con Windows Update (Opzioni avanzate > "Ricevi aggiornamenti per altri prodotti Microsoft"). Te lo apro.'; Open-User 'ms-settings:windowsupdate-options'; return }
$before = Get-C2RConf 'VersionToReport'
Say ('Versione di Office ora: ' + $before + '. Cerco aggiornamenti...')
$open = Get-OpenOffice
if ($open.Count) { Say ('Sono aperti ' + ($open -join ', ') + ': scarico l''aggiornamento, ma si installa quando li chiudi (non li chiudo io per non farti perdere niente).') }
$t0 = Get-Date
[void](Run-Hidden (Join-Path $c 'OfficeC2RClient.exe') '/update user displaylevel=false forceappshutdown=false')
Say 'Office controlla e scarica in sottofondo: aspetto fino a 30 minuti che finisca...'
$end = (Get-Date).AddMinutes(30); $after = $before; $last = Get-Date
do {
  Start-Sleep -Seconds 15
  $after = Get-C2RConf 'VersionToReport'
  $busy = @(Get-CimInstance Win32_Process -Filter "Name='OfficeClickToRun.exe' OR Name='OfficeC2RClient.exe'" -ErrorAction SilentlyContinue | Where-Object { [string]$_.CommandLine -match 'scenario=|/update' })
  if (((Get-Date) - $last).TotalMinutes -ge 2) { Say 'Office ci sta ancora lavorando, aspetta...'; $last = Get-Date }
} while ($after -eq $before -and (Get-Date) -lt $end -and ($busy.Count -or ((Get-Date) - $t0).TotalSeconds -lt 60))
if ($after -and $after -ne $before) { Say ('Office aggiornato: ' + $before + ' -> ' + $after + '.') }
elseif ($open.Count) { Say 'Chiudi i programmi di Office: se c''è un aggiornamento, finisce da solo quando li chiudi.' }
else { Say 'Adesso non risulta installato nessun aggiornamento: o Office era già aggiornato, o lo sta finendo in sottofondo. Per vederlo: Word > File > Account > Opzioni di aggiornamento > Aggiorna.' }
`)},
	{ID: "of_office_lost", Name: "Ho perso un documento non salvato", Desc: "Cerca le copie automatiche di Word, Excel e PowerPoint degli ultimi 14 giorni e te le mette in una cartella in Documenti. Non cancella niente.", Level: 0, Minutes: 0.5, Timeout: 5 * time.Minute, Script: ofs(`
$src = @(
  (Join-Path $env:LOCALAPPDATA 'Microsoft\Office\UnsavedFiles'),
  (Join-Path $env:APPDATA 'Microsoft\Word'),
  (Join-Path $env:APPDATA 'Microsoft\Excel'),
  (Join-Path $env:APPDATA 'Microsoft\PowerPoint'),
  (Join-Path $env:LOCALAPPDATA 'Microsoft\Office\16.0\OfficeFileCache')
)
$ext = '\.(asd|wbk|xar|xlsb|xlsx|xls|docx|doc|pptx|ppt|tmp)$'
$lim = (Get-Date).AddDays(-14)
$found = @()
foreach ($s in $src) { if (Test-Path -LiteralPath $s) { $found += @(Get-ChildItem -LiteralPath $s -Recurse -File -Force -ErrorAction SilentlyContinue | Where-Object { $_.LastWriteTime -gt $lim -and $_.Length -gt 0 -and $_.Name -match $ext -and $_.Name -notmatch '^~' -and $_.DirectoryName -notmatch 'STARTUP|XLSTART|Templates' }) } }
if ($found.Count -eq 0) {
  Say 'Non ho trovato copie automatiche degli ultimi 14 giorni.'
  Say 'Altre strade: in Word o Excel vai su File > Informazioni > Gestisci documento > "Recupera documenti non salvati". Se il file era su OneDrive o SharePoint, guarda "Cronologia versioni" con il tasto destro sul file.'
  return
}
$dst = Join-Path ([Environment]::GetFolderPath('MyDocuments')) ('Tasso - documenti recuperati\' + (Get-Date -Format 'yyyy-MM-dd HH.mm'))
New-Item -ItemType Directory -Path $dst -Force | Out-Null
$n = 0
foreach ($f in ($found | Sort-Object LastWriteTime -Descending | Select-Object -First 200)) {
  $name = $f.LastWriteTime.ToString('dd-MM HH.mm') + ' - ' + $f.Name
  try { Copy-Item -LiteralPath $f.FullName -Destination (Join-Path $dst $name) -ErrorAction Stop; $n++ } catch {}
}
Say ('Trovate e copiate ' + $n + ' copie automatiche in Documenti > "Tasso - documenti recuperati". Gli originali restano dove sono.')
Say 'Aprile con Word, Excel o PowerPoint (File > Apri > scegli il file; per i file .asd scegli "Tutti i file"). Il nome inizia con giorno e ora dell''ultima modifica.'
Open-User $dst
`)},
	{ID: "of_office_crash", Name: "Word o Excel si chiudono appena li apri", Desc: "Mette da parte il modello di Word rovinato (Normal.dotm, Word ne crea uno nuovo) e ti dice quali componenti aggiuntivi partono con Office.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs(`
if (Stop-IfOfficeOpen @('WINWORD', 'EXCEL', 'POWERPNT')) { return }
$tpl = Join-Path $env:APPDATA 'Microsoft\Templates\Normal.dotm'
if (Test-Path -LiteralPath $tpl) {
  $new = 'Normal_vecchio_' + (Get-Date -Format 'yyyyMMdd_HHmm') + '.dotm'
  try { Rename-Item -LiteralPath $tpl -NewName $new -ErrorAction Stop; Say ('Il modello di Word (Normal.dotm) è stato messo da parte come ' + $new + ': Word ne crea uno nuovo pulito. Per tornare indietro: chiudi Word, cancella il Normal.dotm nuovo e rinomina ' + $new + ' in Normal.dotm (cartella %APPDATA%\Microsoft\Templates).') } catch { Say 'Non riesco a spostare il modello di Word: forse Word è ancora aperto in sottofondo. Riavvia il PC e riprova.' }
}
$ins = @()
foreach ($app in @('Word', 'Excel', 'PowerPoint')) {
  foreach ($root in @('HKCU:\Software\Microsoft\Office\' + $app + '\Addins', 'HKLM:\SOFTWARE\Microsoft\Office\' + $app + '\Addins', 'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Office\' + $app + '\Addins')) {
    foreach ($k in @(Get-ChildItem -LiteralPath $root -ErrorAction SilentlyContinue)) {
      $lb = 0; try { $lb = [int](Get-ItemProperty -LiteralPath $k.PSPath -Name LoadBehavior -ErrorAction Stop).LoadBehavior } catch {}
      if ($lb -eq 3) { $fn = ''; try { $fn = [string](Get-ItemProperty -LiteralPath $k.PSPath -Name FriendlyName -ErrorAction Stop).FriendlyName } catch {}; if (-not $fn) { $fn = $k.PSChildName }; $ins += ($app + ': ' + $fn) }
    }
  }
}
if ($ins.Count) { Say ('Componenti aggiuntivi che partono con Office: ' + (($ins | Select-Object -Unique) -join '; ') + '.') }
$xs = Join-Path $env:APPDATA 'Microsoft\Excel\XLSTART'
$xf = @(Get-ChildItem -LiteralPath $xs -File -ErrorAction SilentlyContinue)
if ($xf.Count) { Say ('Excel apre da solo all''avvio questi file: ' + (($xf | ForEach-Object { $_.Name }) -join ', ') + ' (cartella XLSTART). Se uno è rovinato Excel si chiude.') }
Say 'Prova a riaprire. Se si chiude ancora: tieni premuto Ctrl mentre lo apri (modalità provvisoria). Se così funziona, il colpevole è un componente aggiuntivo: File > Opzioni > Componenti aggiuntivi > Gestisci "Componenti aggiuntivi COM" > Vai > togli la spunta uno alla volta.'
`)},
	{ID: "of_office_login", Name: "Office chiede sempre di accedere o dice \"senza licenza\"", Desc: "Svuota l'accesso e le password salvate di Office (il registro con copia di sicurezza): poi rientri una volta con il tuo account e torna tutto a posto.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs(`
if (Stop-IfOfficeOpen) { return }
$done = 0
foreach ($ver in @('16.0', '15.0')) {
  $k = 'HKCU\Software\Microsoft\Office\' + $ver + '\Common\Identity'
  if (Test-Path -LiteralPath ('Registry::HKEY_CURRENT_USER\Software\Microsoft\Office\' + $ver + '\Common\Identity')) {
    $b = Backup-Reg $k ('office-accesso-' + $ver)
    if ($b) { Remove-Item -LiteralPath ('Registry::HKEY_CURRENT_USER\Software\Microsoft\Office\' + $ver + '\Common\Identity') -Recurse -Force -ErrorAction SilentlyContinue; $done++; Say ('Accesso salvato di Office svuotato. Copia di sicurezza: ' + $b) }
    else { Say 'Non sono riuscito a fare la copia di sicurezza, quindi per sicurezza non tocco niente.' }
  }
}
$lic = Join-Path $env:LOCALAPPDATA 'Microsoft\Office\Licenses'
if (Test-Path -LiteralPath $lic) { $nn = 'Licenses_vecchie_' + (Get-Date -Format 'yyyyMMdd_HHmm'); try { Rename-Item -LiteralPath $lic -NewName $nn -ErrorAction Stop; $done++; Say 'Licenze salvate di Office messe da parte: Office le riscarica quando accedi.' } catch {} }
$n = 0
$list = (cmdkey /list) -join "§"
foreach ($m in [regex]::Matches($list, '(?:Destinazione|Target):\s*(?:LegacyGeneric:target=)?(MicrosoftOffice1[56]_Data:[^§]+)')) { $t = $m.Groups[1].Value.Trim(); [void](Run-Hidden 'cmdkey.exe' ('/delete:"' + $t + '"')); $n++ }
if ($n) { Say ('Tolte ' + $n + ' password vecchie di Office salvate in Windows.') }
if ($done -eq 0 -and $n -eq 0) { Say 'Non c''era niente di salvato da svuotare.' }
Say 'Ora apri Word: ti chiede di accedere. Usa l''account con cui hai Office (quello di lavoro o il tuo Microsoft). Se dice ancora "senza licenza", l''abbonamento potrebbe essere scaduto: controlla su account.microsoft.com o chiedi all''ufficio.'
`)},
	{ID: "of_office_addins", Name: "Manca il pulsante di Teams in Outlook", Desc: "Riaccende il componente \"Riunione di Teams\" di Outlook quando sparisce e svuota la cache dei componenti aggiuntivi di Office.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs(`
if (Stop-IfOfficeOpen) { return }
$f = [long]0
$wef = Join-Path $env:LOCALAPPDATA 'Microsoft\Office\16.0\Wef'
if (Test-Path -LiteralPath $wef) { $f += Clear-Folder $wef; Say 'Cache dei componenti aggiuntivi di Office svuotata (si riscaricano da soli).' }
Freed $f
$found = $false
foreach ($root in @('HKCU:\Software\Microsoft\Office\Outlook\Addins', 'HKLM:\SOFTWARE\Microsoft\Office\Outlook\Addins', 'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Office\Outlook\Addins')) {
  if (Test-Path -LiteralPath ($root + '\TeamsAddin.FastConnect')) { $found = $true }
}
if ($found) {
  foreach ($rk in @('HKCU\Software\Microsoft\Office\Outlook\Addins\TeamsAddin.FastConnect', 'HKCU\Software\Microsoft\Office\16.0\Outlook\Resiliency\DoNotDisableAddinList')) { if (Test-Path -LiteralPath ('Registry::HKEY_CURRENT_USER' + $rk.Substring(4))) { [void](Backup-Reg $rk ('outlook-' + (Split-Path $rk -Leaf))) } }
  $k = 'HKCU:\Software\Microsoft\Office\Outlook\Addins\TeamsAddin.FastConnect'
  if (-not (Test-Path -LiteralPath $k)) { New-Item -Path $k -Force | Out-Null }
  New-ItemProperty -LiteralPath $k -Name LoadBehavior -Value 3 -PropertyType DWord -Force | Out-Null
  $dn = 'HKCU:\Software\Microsoft\Office\16.0\Outlook\Resiliency\DoNotDisableAddinList'
  if (-not (Test-Path -LiteralPath $dn)) { New-Item -Path $dn -Force | Out-Null }
  New-ItemProperty -LiteralPath $dn -Name 'TeamsAddin.FastConnect' -Value 1 -PropertyType DWord -Force | Out-Null
  Say 'Riacceso il componente "Riunione di Teams" di Outlook e detto a Outlook di non spegnerlo più da solo.'
} else { Say 'Il componente di Teams per Outlook non è installato: si installa da solo aprendo Teams (quello nuovo) e accedendo con l''account di lavoro. Poi riapri Outlook.' }
$dis = 'Registry::HKEY_CURRENT_USER\Software\Microsoft\Office\16.0\Outlook\Resiliency\DisabledItems'
if (Test-Path -LiteralPath $dis) {
  $b = Backup-Reg 'HKCU\Software\Microsoft\Office\16.0\Outlook\Resiliency\DisabledItems' 'outlook-componenti-spenti'
  if ($b) { Remove-Item -LiteralPath $dis -Recurse -Force -ErrorAction SilentlyContinue; Say 'Tolti i componenti che Outlook aveva "spento per lentezza" (copia di sicurezza salvata).' }
}
Say 'Fatto: riapri Outlook. Il pulsante "Nuova riunione di Teams" è nel Calendario.'
`)},

	// ---------- Outlook e posta ----------
	{ID: "of_outlook_open", Name: "Outlook non si apre", Desc: "Se Outlook è bloccato da più di 2 minuti senza finestra lo chiude, poi lo riapre sistemando il riquadro delle cartelle (la causa più comune). Posta e cartelle restano.", Level: 0, Minutes: 0.4, Timeout: 3 * time.Minute, Script: ofs(`
$ol = @(Get-Process -Name OUTLOOK -ErrorAction SilentlyContinue)
$stuck = @($ol | Where-Object { (-not $_.Responding) -or ($_.MainWindowHandle -eq 0 -and $_.StartTime -lt (Get-Date).AddMinutes(-2)) })
if ($ol.Count -and $stuck.Count -lt $ol.Count) { Say 'Outlook è già aperto (o si sta ancora aprendo): aspetta un minuto e guarda nella barra in basso o vicino all''orologio. Se è su un altro schermo: clic sull''icona, poi Win + Maiusc + freccia.'; return }
if ($stuck.Count) { Say 'Outlook è rimasto bloccato senza finestra da più di 2 minuti: lo chiudo. Un messaggio che stavi scrivendo e non era ancora salvato potrebbe perdersi.'; $stuck | Stop-Process -Force -ErrorAction SilentlyContinue; Start-Sleep -Seconds 3 }
$exe = Get-AppPath 'OUTLOOK.EXE'
if (-not $exe) { Say 'Non trovo Outlook classico su questo PC. Se usi il "nuovo Outlook", si ripara da Impostazioni > App installate > Outlook > Opzioni avanzate > Ripristina.'; return }
Say 'Riapro Outlook sistemando il riquadro delle cartelle (/resetnavpane): posta e cartelle restano tutte.'
[void](Start-AsUser $exe '/resetnavpane')
Say 'Se non si apre ancora: tieni premuto Ctrl e clicca Outlook (modalità provvisoria). Se così va, prova "Manca il pulsante di Teams in Outlook" qui vicino per sistemare i componenti aggiuntivi.'
`)},
	{ID: "of_outlook_search", Name: "La ricerca di Outlook non trova le email", Desc: "Fa ricostruire a Windows l'indice di ricerca da zero. Ci vogliono alcune ore col PC acceso, poi la ricerca torna completa.", Level: 0, Minutes: 0.5, Timeout: 5 * time.Minute, Script: ofs(`
$sv = Get-Service -Name WSearch -ErrorAction SilentlyContinue
if (-not $sv) { Say 'La ricerca di Windows non c''è su questo PC.'; return }
if ($sv.StartType -eq 'Disabled') { [void](Run-Hidden 'sc.exe' 'config WSearch start= delayed-auto'); Say 'La ricerca di Windows era spenta del tutto (Outlook la usa per cercare): l''ho riaccesa.' }
Say 'Fermo la ricerca di Windows e preparo la ricostruzione dell''indice...'
Stop-Service -Name WSearch -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 2
$bk = Backup-Reg 'HKLM\SOFTWARE\Microsoft\Windows Search' 'ricerca-windows'
Set-ItemProperty -LiteralPath 'HKLM:\SOFTWARE\Microsoft\Windows Search' -Name SetupCompletedSuccessfully -Value 0 -Type DWord -ErrorAction SilentlyContinue
Start-Service -Name WSearch -ErrorAction SilentlyContinue
Start-Sleep -Seconds 2
if ((Get-Service -Name WSearch).Status -eq 'Running') { Say 'Fatto: Windows sta rifacendo l''indice da zero. Lascia il PC acceso: in qualche ora la ricerca di Outlook (e del menu Start) torna completa. Nel frattempo i risultati possono mancare.' }
else { Say 'La ricerca di Windows non è ripartita: riavvia il PC.'; [Console]::Out.WriteLine('REBOOT') }
Say 'Se usi il "nuovo Outlook" la ricerca è fatta sul server: lì basta chiuderlo e riaprirlo.'
`)},
	{ID: "of_outlook_mail", Name: "Le email non arrivano o non partono", Desc: "Controlla internet, i server di posta Microsoft e lo spazio su disco e ti dice dov'è il problema. Le impostazioni dell'account le cambi tu.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs(`
Say 'Controllo la connessione...'
$net = Test-Internet
if (-not $net) { Say 'Internet NON funziona: per questo la posta non va. Usa "Internet non va" qui in Risolvi problemi.'; return }
Say 'Internet: funziona.'
$m365 = Test-Tcp 'outlook.office365.com' 443
if ($m365) { Say 'Server di posta Microsoft (Outlook, Microsoft 365, Hotmail): raggiungibili.' } else { Say 'Il server di posta Microsoft non risponde: può essere un blocco della rete dell''ufficio o un problema di Microsoft (guarda status.cloud.microsoft).' }
$imap = @(); foreach ($h in @('imap.gmail.com', 'imaps.aruba.it', 'imapmail.libero.it')) { if (Test-Tcp $h 993 1500) { $imap += $h } }
if ($imap.Count) { Say 'Le porte della posta IMAP (Gmail, Aruba, Libero...) sono aperte: la rete non blocca la posta.' } else { Say 'ATTENZIONE: la rete blocca le porte della posta IMAP (993). Se usi Gmail, Aruba o Libero in Outlook, chiedi a chi gestisce la rete o prova con un''altra connessione.' }
$c = Get-PSDrive -Name ($env:SystemDrive.Trim(':')) -ErrorAction SilentlyContinue
if ($c -and $c.Free -lt 2GB) { Say ('Il disco C: è quasi pieno (' + (Fmt $c.Free) + ' liberi): Outlook non riesce a scaricare la posta. Fai una pulizia in Pulizia.') }
$ost = @(Get-ChildItem -LiteralPath (Join-Path $env:LOCALAPPDATA 'Microsoft\Outlook') -Include *.ost,*.pst -Recurse -File -ErrorAction SilentlyContinue | Where-Object { $_.Length -gt 45GB })
if ($ost.Count) { Say 'Un file della posta è quasi al limite di 50 GB: usa "Outlook lento: la casella è troppo grande".' }
Say 'Cose da controllare in Outlook: in alto scheda "Invia/Ricevi" > "Lavora offline" deve essere SPENTO. In basso a destra deve esserci "Connesso". Se una email resta nella Posta in uscita, aprila e rimandala (gli allegati sopra 20 MB spesso vengono bloccati).'
`)},
	{ID: "of_outlook_size", Name: "Outlook lento: la casella è troppo grande", Desc: "Misura i file della posta di Outlook e ti dice se sono troppo grandi e cosa fare per alleggerirli. Non cancella email.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs(`
$dirs = @((Join-Path $env:LOCALAPPDATA 'Microsoft\Outlook'), (Join-Path ([Environment]::GetFolderPath('MyDocuments')) 'File di Outlook'), (Join-Path ([Environment]::GetFolderPath('MyDocuments')) 'Outlook Files'))
$fs = @(); foreach ($d in $dirs) { if (Test-Path -LiteralPath $d) { $fs += @(Get-ChildItem -LiteralPath $d -Recurse -File -Include *.ost,*.pst -ErrorAction SilentlyContinue) } }
if (-not $fs.Count) { Say 'Non trovo file della posta di Outlook classico su questo PC (con il "nuovo Outlook" la posta resta sul server).'; return }
$big = $false
foreach ($f in ($fs | Sort-Object Length -Descending)) {
  $t = $f.Name + ': ' + (Fmt $f.Length)
  if ($f.Length -gt 40GB) { $t += ' - TROPPO GRANDE (il limite è 50 GB)'; $big = $true } elseif ($f.Length -gt 10GB) { $t += ' - grande, può rallentare'; $big = $true }
  Say $t
}
if ($big) {
  Say 'Come alleggerire: svuota "Posta eliminata" e "Posta indesiderata"; cerca le email con allegati grandi (in alto: Cerca > Dimensioni > Enorme) e cancella quelle che non servono.'
  Say 'Con un account di lavoro: File > Impostazioni account > doppio clic sull''account > "Scarica la posta degli ultimi" 1 anno. Le email più vecchie restano sul server e le trovi comunque cercando.'
} else { Say 'I file della posta hanno una grandezza normale: la lentezza non dipende da questo. Prova "Office si blocca o dà errori" o "Manca il pulsante di Teams in Outlook" (componenti aggiuntivi).' }
`)},
	{ID: "of_outlook_pwd", Name: "Outlook chiede sempre la password", Desc: "Toglie le password vecchie di Outlook e Office salvate in Windows: le reinserisci una volta e restano.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs(`
if (Stop-IfOfficeOpen @('OUTLOOK')) { return }
$list = (cmdkey /list) -join "§"
$n = 0
foreach ($m in [regex]::Matches($list, '(?:Destinazione|Target):\s*(?:LegacyGeneric:target=)?((?:MicrosoftOffice1[56]_Data:|Outlook|msoutlook|MS\.Outlook)[^§]*)')) { $t = $m.Groups[1].Value.Trim(); [void](Run-Hidden 'cmdkey.exe' ('/delete:"' + $t + '"')); $n++ }
if ($n) { Say ('Tolte ' + $n + ' password vecchie di Outlook/Office salvate in Windows.') } else { Say 'In Windows non c''erano password vecchie di Outlook.' }
Say 'Ora apri Outlook e inserisci la password una volta, con la spunta "Ricorda le credenziali". Se l''hai cambiata da poco, usa quella nuova.'
Say 'Se continua a chiederla con Gmail, Aruba o Libero: serve la "password per le app" del tuo provider (Gmail: myaccount.google.com > Sicurezza > Password per le app).'
`)},
	{ID: "of_outlook_attach", Name: "Gli allegati di Outlook non si aprono", Desc: "Svuota la cartella nascosta dove Outlook apre gli allegati, quando è piena e dà errore. I file vengono spostati in Documenti, non cancellati.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs(`
if (Stop-IfOfficeOpen @('OUTLOOK')) { return }
$dirs = @()
foreach ($ver in @('16.0', '15.0', '14.0')) { try { $p = [string](Get-ItemProperty -LiteralPath ('HKCU:\Software\Microsoft\Office\' + $ver + '\Outlook\Security') -Name OutlookSecureTempFolder -ErrorAction Stop).OutlookSecureTempFolder; if ($p -and (Test-Path -LiteralPath $p)) { $dirs += $p } } catch {} }
if (-not $dirs.Count) { Say 'Non trovo la cartella degli allegati di Outlook: forse non hai Outlook classico o non hai mai aperto allegati.'; return }
$dst = Join-Path ([Environment]::GetFolderPath('MyDocuments')) ('Tasso - allegati di Outlook\' + (Get-Date -Format 'yyyy-MM-dd HH.mm'))
$n = 0
foreach ($d in ($dirs | Select-Object -Unique)) {
  foreach ($f in @(Get-ChildItem -LiteralPath $d -File -Force -ErrorAction SilentlyContinue)) {
    if (-not (Test-Path -LiteralPath $dst)) { New-Item -ItemType Directory -Path $dst -Force | Out-Null }
    $to = Join-Path $dst $f.Name; $i = 1
    while (Test-Path -LiteralPath $to) { $to = Join-Path $dst ($f.BaseName + ' (' + $i + ')' + $f.Extension); $i++ }
    try { Move-Item -LiteralPath $f.FullName -Destination $to -ErrorAction Stop; $n++ } catch {}
  }
}
if ($n) { Say ('Spostati ' + $n + ' allegati vecchi in Documenti > "Tasso - allegati di Outlook". Se avevi modificato un allegato aprendolo da Outlook, la tua modifica è lì dentro.') } else { Say 'La cartella degli allegati era già vuota.' }
Say 'Fatto: riapri Outlook e riprova l''allegato. Consiglio: per modificare un allegato, prima salvalo (tasto destro > Salva con nome).'
`)},
	{ID: "of_outlook_reminders", Name: "I promemoria di Outlook impazziscono", Desc: "Ricrea l'elenco dei promemoria quando ne compaiono di vecchi o doppi, o non compaiono per niente. Appuntamenti e attività restano.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs(`
if (Stop-IfOfficeOpen @('OUTLOOK')) { return }
$exe = Get-AppPath 'OUTLOOK.EXE'
if (-not $exe) { Say 'Non trovo Outlook classico su questo PC.'; return }
Say 'Riapro Outlook ricreando i promemoria (/cleanreminders): il calendario non viene toccato.'
[void](Start-AsUser $exe '/cleanreminders')
Say 'Fatto. I promemoria di appuntamenti futuri ricompaiono all''ora giusta.'
`)},

	// ---------- Teams e riunioni ----------
	{ID: "of_teams", Name: "Teams lento, bianco o non si apre", Desc: "Chiude Teams e svuota la sua cache, la soluzione ufficiale di Microsoft. Chat, file e riunioni restano (sono online). Se sei in riunione, prima esci: Teams si chiude.", Level: 0, Minutes: 0.4, Timeout: 4 * time.Minute, Script: ofs(`
$p = @(Get-Process -Name 'ms-teams', 'Teams', 'msteams', 'MSTeams' -ErrorAction SilentlyContinue)
if ($p.Count) { Say 'Chiudo Teams...'; $p | Stop-Process -Force -ErrorAction SilentlyContinue; Start-Sleep -Seconds 3 }
$f = [long]0; $any = $false
$new = Join-Path $env:LOCALAPPDATA 'Packages\MSTeams_8wekyb3d8bbwe\LocalCache\Microsoft\MSTeams'
if (Test-Path -LiteralPath $new) { $f += Clear-Folder $new; $any = $true; Say 'Cache del nuovo Teams svuotata.' }
$old = Join-Path $env:APPDATA 'Microsoft\Teams'
if (Test-Path -LiteralPath $old) {
  foreach ($s in @('application cache\cache', 'blob_storage', 'Cache', 'Code Cache', 'databases', 'GPUCache', 'IndexedDB', 'Local Storage', 'tmp')) { $d = Join-Path $old $s; if (Test-Path -LiteralPath $d) { $f += Clear-Folder $d; $any = $true } }
  Say 'Cache di Teams classico svuotata.'
}
Freed $f
if (-not $any) { Say 'Non trovo Teams su questo PC.'; return }
Say ('Liberati ' + (Fmt $f) + '. Riapro Teams: il primo avvio è un po'' più lento e potrebbe chiederti di accedere di nuovo.')
if (Test-Path -LiteralPath (Join-Path $env:LOCALAPPDATA 'Packages\MSTeams_8wekyb3d8bbwe')) { Open-User 'shell:AppsFolder\MSTeams_8wekyb3d8bbwe!MSTeams' }
else { $up = Join-Path $env:LOCALAPPDATA 'Microsoft\Teams\Update.exe'; if (Test-Path -LiteralPath $up) { [void](Start-AsUser $up '--processStart "Teams.exe"') } }
`)},
	{ID: "of_meet_mic", Name: "In riunione non mi sentono", Desc: "Toglie il muto al microfono e alza il volume se è a zero, controlla che Windows non blocchi il microfono a Teams, Zoom e Meet e, solo se non lo vede, riavvia l'audio.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs(`
$n = Restart-BadDevices 'AudioEndpoint|MEDIA'
if (Use-Audio) {
  $st = [TassoAudio]::State(1, 2)
  if (-not $st) {
    Say 'Windows non vede il microfono: riavvio l''audio di Windows (si ferma per un secondo, anche in riunione)...'
    Restart-Service -Name AudioEndpointBuilder -Force -ErrorAction SilentlyContinue
    Start-Service -Name Audiosrv -ErrorAction SilentlyContinue
    for ($i = 0; $i -lt 4 -and -not $st; $i++) { Start-Sleep -Seconds 1; $st = [TassoAudio]::State(1, 2) }
  }
  if (-not $st) { Say 'Windows non vede nessun microfono: collega le cuffie o il microfono (se è USB prova un''altra porta).' }
  else {
    $pa = $st.Split('|'); if ($pa[0] -eq '1') { Say 'Il microfono era IN MUTO in Windows: l''ho riacceso.' }
    if ([int]$pa[1] -lt 30) { Say ('Il volume del microfono era al ' + $pa[1] + '%: l''ho alzato all''80%.') }
    [void][TassoAudio]::Fix(1, 2, 0.3, 0.8); [void][TassoAudio]::Fix(1, 0, 0.3, 0.8)
  }
}
$pv = Get-Privacy 'microphone'
if ($pv.pc -eq 'Deny' -or $pv.all -eq 'Deny' -or $pv.desk -eq 'Deny') { Say 'ATTENZIONE: Windows blocca il microfono ai programmi. Ti apro l''impostazione: accendi "Accesso al microfono" e "Consenti alle app desktop di accedere al microfono".'; Open-User 'ms-settings:privacy-microphone' }
else { Say 'Windows lascia usare il microfono ai programmi: ok.' }
Say 'Ultimo controllo da fare tu: nel programma della riunione (Teams: Impostazioni > Dispositivi; Zoom: Impostazioni > Audio) scegli il microfono giusto e controlla che il pulsante del microfono non sia barrato. Molte cuffie hanno anche un tasto muto sul filo.'
`)},
	{ID: "of_meet_cam", Name: "La webcam non va", Desc: "Ti dice se la webcam è già usata da un altro programma, la riaccende se era spenta, riavvia il servizio della fotocamera e controlla la privacy.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs(`
$base = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\webcam'
$busy = @()
foreach ($k in @(Get-ChildItem -LiteralPath $base -Recurse -ErrorAction SilentlyContinue)) {
  try { $p = Get-ItemProperty -LiteralPath $k.PSPath -ErrorAction Stop; if ($p.LastUsedTimeStart -and $p.LastUsedTimeStop -eq 0) { $nm = $k.PSChildName -replace '#', '\'; $busy += (Split-Path $nm -Leaf) } } catch {}
}
if ($busy.Count) { Say ('La webcam la sta usando: ' + (($busy | Select-Object -Unique) -join ', ') + '. Chiudi quel programma (una sola app alla volta può usare la webcam), poi riprova.') }
$off = @(Get-CimInstance Win32_PnPEntity -ErrorAction SilentlyContinue | Where-Object { $_.PNPClass -match '^(Camera|Image)$' -and $_.ConfigManagerErrorCode -eq 22 })
foreach ($d in $off) { $id = [string]$d.PNPDeviceID; if ($id -match '^[A-Za-z0-9\\&_.{}#\- ]{3,300}$') { [void](Run-Hidden 'pnputil.exe' ('/enable-device "' + $id + '"')); Say ('La webcam era disattivata in Windows: riaccesa (' + $d.Name + '). Per spegnerla di nuovo: Gestione dispositivi > Fotocamere > tasto destro > Disabilita.') } }
Say 'Riavvio il servizio della fotocamera di Windows...'
Restart-Service -Name FrameServer -Force -ErrorAction SilentlyContinue
$n = Restart-BadDevices '^(Camera|Image)$'
[void](Run-Hidden 'pnputil.exe' '/scan-devices')
$cams = @(Get-PnpDevice -PresentOnly -ErrorAction SilentlyContinue | Where-Object { [string]$_.Class -match '^(Camera|Image)$' -and $_.FriendlyName -notmatch 'Scanner|IR' })
if ($cams.Count) { Say ('Webcam trovate: ' + (($cams | ForEach-Object { $_.FriendlyName }) -join ', ') + '.') } else { Say 'Windows non vede nessuna webcam. Sul portatile cerca un tasto con la telecamera barrata (spesso F8 o F10) o una tendina scorrevole sopra lo schermo.' }
$pv = Get-Privacy 'webcam'
if ($pv.pc -eq 'Deny' -or $pv.all -eq 'Deny' -or $pv.desk -eq 'Deny') { Say 'ATTENZIONE: Windows blocca la webcam ai programmi. Ti apro l''impostazione: accendi "Accesso alla fotocamera" e "Consenti alle app desktop".'; Open-User 'ms-settings:privacy-webcam' }
Say 'Nel programma della riunione scegli la webcam giusta (Teams: Impostazioni > Dispositivi > Fotocamera).'
`)},
	{ID: "of_meet_bt", Name: "Le cuffie Bluetooth non si collegano", Desc: "Riavvia il Bluetooth e i suoi servizi audio. Ti spiega anche perché in chiamata le cuffie Bluetooth si sentono peggio.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs(`
$radio = @(Get-PnpDevice -Class Bluetooth -PresentOnly -ErrorAction SilentlyContinue)
if (-not $radio.Count) { Say 'Questo PC non ha il Bluetooth acceso o non ce l''ha. Sul portatile controlla che la Modalità aereo sia spenta (centro notifiche in basso a destra).'; Open-User 'ms-settings:bluetooth'; return }
Say 'Riavvio il Bluetooth: mouse, tastiere e cuffie Bluetooth si staccano per qualche secondo e poi si ricollegano da sole.'
foreach ($s in @('bthserv', 'BTAGService', 'BthAvctpSvc')) { $sv = Get-Service -Name $s -ErrorAction SilentlyContinue; if ($sv -and $sv.StartType -ne 'Disabled') { Restart-Service -Name $s -Force -ErrorAction SilentlyContinue } }
$n = Restart-BadDevices 'Bluetooth|AudioEndpoint|MEDIA'
[void](Run-Hidden 'pnputil.exe' '/scan-devices')
Say 'Fatto. Se le cuffie non si collegano: tienile vicine al PC, spegnile e riaccendile. Se ancora niente, in Impostazioni > Bluetooth rimuovile e riabbinale (tasto di abbinamento premuto qualche secondo).'
Say 'Da sapere: in chiamata le cuffie Bluetooth passano in modalità "vivavoce" per usare il loro microfono e la musica si sente peggio. È normale per il Bluetooth: per un audio migliore usa le cuffie con la chiavetta USB o con il filo.'
Open-User 'ms-settings:bluetooth'
`)},
	{ID: "of_meet_speaker", Name: "Non sento gli altri in riunione", Desc: "Toglie il muto alle casse e alza il volume se è a zero, riavvia l'audio solo se Windows non le vede e ti dice dove scegliere le cuffie giuste.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs(`
$n = Restart-BadDevices 'AudioEndpoint|MEDIA'
if (Use-Audio) {
  $st = [TassoAudio]::State(0, 0)
  if (-not $st) {
    Say 'Windows non vede casse o cuffie: riavvio l''audio di Windows (si ferma per un secondo)...'
    Restart-Service -Name AudioEndpointBuilder -Force -ErrorAction SilentlyContinue
    Start-Service -Name Audiosrv -ErrorAction SilentlyContinue
    for ($i = 0; $i -lt 4 -and -not $st; $i++) { Start-Sleep -Seconds 1; $st = [TassoAudio]::State(0, 0) }
  }
  if (-not $st) { Say 'Windows non vede casse o cuffie: collegale (se sono USB prova un''altra porta).' }
  else {
    $pa = $st.Split('|'); if ($pa[0] -eq '1') { Say 'Le casse erano IN MUTO: le ho riaccese.' }
    if ([int]$pa[1] -lt 15) { Say ('Il volume era al ' + $pa[1] + '%: l''ho alzato al 50%.') }
    [void][TassoAudio]::Fix(0, 0, 0.15, 0.5); [void][TassoAudio]::Fix(0, 2, 0.15, 0.5)
  }
}
$outs = @(Get-PnpDevice -Class AudioEndpoint -PresentOnly -Status OK -ErrorAction SilentlyContinue | Where-Object { $_.FriendlyName -notmatch '\b(Microfono|Microfoni|Microphone|Mic)\b|Line In|Ingresso' })
if ($outs.Count -gt 1) { Say ('Uscite audio collegate: ' + (($outs | ForEach-Object { $_.FriendlyName }) -join ', ') + '. Se il suono va in quella sbagliata, scegli quella giusta nella finestra che ti apro (Uscita).') }
Open-User 'ms-settings:sound'
Say 'Nel programma della riunione controlla anche l''altoparlante scelto (Teams: Impostazioni > Dispositivi > Altoparlante).'
`)},

	// ---------- Rete, Wi-Fi e VPN ----------
	{ID: "of_wifi_drop", Name: "Il Wi-Fi si stacca spesso", Desc: "Impedisce a Windows di spegnere il Wi-Fi per risparmiare energia (la causa più comune) e ti dice quanto è forte il segnale.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs(`
$wl = @(Get-NetAdapter -Physical -ErrorAction SilentlyContinue | Where-Object { $_.PhysicalMediaType -match '802\.11|Wireless' -or $_.InterfaceDescription -match 'Wi-?Fi|Wireless|WLAN|802\.11' })
if (-not $wl.Count) { Say 'Non trovo una scheda Wi-Fi su questo PC.'; return }
foreach ($a in $wl) {
  $pm = Get-NetAdapterPowerManagement -Name $a.Name -ErrorAction SilentlyContinue
  if ($pm -and [string]$pm.AllowComputerToTurnOffDevice -eq 'Enabled') {
    try { Set-NetAdapterPowerManagement -Name $a.Name -AllowComputerToTurnOffDevice Disabled -ErrorAction Stop; Say ($a.Name + ': Windows non può più spegnerla per risparmiare energia. (Per tornare come prima: Gestione dispositivi > scheda Wi-Fi > Risparmio energia.)') } catch { Say ($a.Name + ': il driver non mi lascia cambiare il risparmio energetico.') }
  } else { Say ($a.Name + ': il risparmio energetico non la spegne, ok.') }
}
$wi = (netsh wlan show interfaces) -join "§"
$sig = ''; if ($wi -match '(?:Segnale|Signal)\s*:\s*(\d+)\s*%') { $sig = $Matches[1] }
$ssid = ''; if ($wi -match '§\s*SSID\s*:\s*([^§]+)') { $ssid = $Matches[1].Trim() }
if ($sig) {
  Say ('Collegato a "' + $ssid + '" con segnale al ' + $sig + '%.')
  if ([int]$sig -lt 50) { Say 'Il segnale è debole: è questo che fa staccare il Wi-Fi. Avvicinati al router o al ripetitore; se puoi, usa il cavo di rete.' }
}
$drv = @($wl | Where-Object { $_.DriverDate -and ([datetime]$_.DriverDate) -lt (Get-Date).AddYears(-3) })
if ($drv.Count) { Say 'Il driver del Wi-Fi ha più di 3 anni: aggiornalo da "Driver di tutto il PC" qui sotto.' }
Say 'Fatto. Se si stacca ancora: spegni il router 30 secondi e riaccendilo.'
`)},
	{ID: "of_wifi_conn", Name: "Il Wi-Fi non si collega", Desc: "Riaccende la scheda Wi-Fi se è spenta, riavvia il servizio del Wi-Fi e la scheda. Le reti e le password salvate restano.", Level: 0, Minutes: 0.4, Timeout: 4 * time.Minute, Script: ofs(`
$wl = @(Get-NetAdapter -Physical -ErrorAction SilentlyContinue | Where-Object { $_.PhysicalMediaType -match '802\.11|Wireless' -or $_.InterfaceDescription -match 'Wi-?Fi|Wireless|WLAN|802\.11' })
if (-not $wl.Count) { Say 'Non trovo una scheda Wi-Fi: forse è disattivata in Gestione dispositivi o sul portatile c''è un tasto Wi-Fi spento.'; [void](Run-Hidden 'pnputil.exe' '/scan-devices'); return }
foreach ($a in $wl) { if ([string]$a.Status -eq 'Disabled') { Enable-NetAdapter -Name $a.Name -Confirm:$false -ErrorAction SilentlyContinue; Say ($a.Name + ' era spenta: riaccesa.') } }
Say 'Riavvio il servizio del Wi-Fi e la scheda (qualche secondo senza internet)...'
Restart-Service -Name WlanSvc -Force -ErrorAction SilentlyContinue
foreach ($a in $wl) { Restart-NetAdapter -Name $a.Name -Confirm:$false -ErrorAction SilentlyContinue }
Start-Sleep -Seconds 8
$up = @(Get-NetAdapter -Physical -ErrorAction SilentlyContinue | Where-Object { ($_.PhysicalMediaType -match '802\.11|Wireless' -or $_.InterfaceDescription -match 'Wi-?Fi|Wireless|WLAN|802\.11') -and [string]$_.Status -eq 'Up' })
if ($up.Count) { Say 'Il Wi-Fi è collegato.'; if (Test-Internet) { Say 'E internet funziona.' } else { Say 'Il Wi-Fi è collegato ma internet non va: usa "Internet non va" o riavvia il router.' } }
else {
  $rs = $null; try { $rs = (Get-ItemProperty -LiteralPath 'HKLM:\SYSTEM\CurrentControlSet\Control\RadioManagement\SystemRadioState' -ErrorAction Stop).'(default)' } catch {}
  if ($rs -eq 1) { Say 'La MODALITÀ AEREO è accesa: spegnila dal centro notifiche in basso a destra.' }
  Say 'Non è ancora collegato. Ti apro le reti Wi-Fi: scegli la tua e inserisci la password. Se la password è cambiata: tasto destro sulla rete > "Non memorizzare" e ricollegati.'
  Open-User 'ms-settings:network-wifi'
}
`)},
	{ID: "of_vpn", Name: "La VPN non si collega", Desc: "Riavvia i servizi di Windows per le VPN e quelli del programma VPN dell'ufficio (FortiClient, GlobalProtect, Cisco...), e rimette l'ora giusta.", Level: 0, Minutes: 0.5, Timeout: 4 * time.Minute, Script: ofs(`
if (-not (Test-Internet)) { Say 'Internet non funziona: senza internet la VPN non può collegarsi. Usa prima "Internet non va".'; return }
Say 'Riavvio i servizi di Windows per le VPN...'
foreach ($s in @('RasMan', 'IKEEXT', 'PolicyAgent', 'SstpSvc')) { $sv = Get-Service -Name $s -ErrorAction SilentlyContinue; if ($sv -and $sv.StartType -ne 'Disabled') { Restart-Service -Name $s -Force -ErrorAction SilentlyContinue; Start-Service -Name $s -ErrorAction SilentlyContinue } }
$vend = @{ 'FortiClient' = 'FA_Scheduler|FortiClient|fortishield'; 'GlobalProtect' = 'PanGPS'; 'Cisco AnyConnect / Secure Client' = 'vpnagent|csc_vpnagent'; 'OpenVPN' = 'OpenVPNService|ovpnconnect|agent_ovpnconnect'; 'WireGuard' = 'WireGuardManager'; 'Check Point' = 'TracSrvWrapper'; 'SonicWall' = 'SWGVpnClient|NEGui'; 'Pulse / Ivanti' = 'JuniperAccessService|PulseSecureService' }
foreach ($v in $vend.Keys) {
  foreach ($sv in @(Get-Service -ErrorAction SilentlyContinue | Where-Object { $_.Name -match ('^(' + $vend[$v] + ')$') })) {
    if ($sv.StartType -eq 'Disabled') { continue }
    if ($sv.Status -ne 'Running') { Start-Service -Name $sv.Name -ErrorAction SilentlyContinue; Say ($v + ': il suo servizio era fermo, l''ho fatto ripartire.') } else { Say ($v + ': il servizio è acceso.') }
  }
}
Say 'Rimetto l''ora giusta (con l''ora sbagliata la VPN rifiuta il collegamento)...'
Start-Service -Name W32Time -ErrorAction SilentlyContinue
[void](Run-Hidden 'w32tm.exe' '/resync /force')
Clear-DnsClientCache
$vp = @(Get-VpnConnection -ErrorAction SilentlyContinue) + @(Get-VpnConnection -AllUserConnection -ErrorAction SilentlyContinue)
foreach ($c in $vp) { if ($c) { Say ('VPN di Windows "' + $c.Name + '": ' + $c.ConnectionStatus) } }
Say 'Ora riprova a collegarti. Se la VPN dice "password o certificato non validi", la password potrebbe essere scaduta: chiedi all''assistenza (IT) dell''ufficio.'
`)},
	{ID: "of_vpn_dns", Name: "Con la VPN i siti dell'ufficio non si aprono", Desc: "Svuota la memoria degli indirizzi (DNS), registra di nuovo il PC nella rete e ti mostra quali server DNS sta usando.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs(`
Say 'Svuoto la memoria degli indirizzi (DNS) e registro di nuovo il PC nella rete...'
Clear-DnsClientCache
[void](Run-Hidden 'ipconfig.exe' '/flushdns')
[void](Run-Hidden 'ipconfig.exe' '/registerdns')
foreach ($d in @(Get-DnsClientServerAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue | Where-Object { $_.ServerAddresses.Count -gt 0 })) { Say ($d.InterfaceAlias + ': DNS ' + ($d.ServerAddresses -join ', ')) }
$hosts = Join-Path $env:SystemRoot 'System32\drivers\etc\hosts'
$lines = @(Get-Content -LiteralPath $hosts -ErrorAction SilentlyContinue | Where-Object { $_ -match '^\s*\d' })
if ($lines.Count) { Say ('Nel file "hosts" ci sono ' + $lines.Count + ' indirizzi scritti a mano: possono mandare i siti nel posto sbagliato. Se non li ha messi l''ufficio, falli controllare.') }
Say 'Fatto. Con la VPN accesa riprova il sito dell''ufficio usando il suo nome completo (es. intranet.azienda.it). Se ancora non va, scollega e ricollega la VPN.'
`)},
	{ID: "of_netdrive", Name: "Le unità di rete hanno la X rossa", Desc: "Controlla le unità di rete (Z:, S:...) una per una, ti dice quali server rispondono e apre quelle raggiungibili per ricollegarle.", Level: 0, Minutes: 0.4, Timeout: 4 * time.Minute, Script: ofs(`
foreach ($s in @('LanmanWorkstation', 'Dnscache')) { $sv = Get-Service -Name $s -ErrorAction SilentlyContinue; if ($sv -and $sv.Status -ne 'Running' -and $sv.StartType -ne 'Disabled') { Start-Service -Name $s -ErrorAction SilentlyContinue } }
$maps = @(Get-ChildItem -LiteralPath 'HKCU:\Network' -ErrorAction SilentlyContinue)
if (-not $maps.Count) { Say 'Su questo PC non ci sono unità di rete salvate (lettere tipo Z: collegate a una cartella dell''ufficio).'; return }
$bad = 0
foreach ($m in $maps) {
  $rp = ''; try { $rp = [string](Get-ItemProperty -LiteralPath $m.PSPath -Name RemotePath -ErrorAction Stop).RemotePath } catch {}
  if (-not $rp) { continue }
  $L = $m.PSChildName.ToUpper() + ':'
  $srv = ''; if ($rp -match '^\\\\([^\\]+)') { $srv = $Matches[1] }
  if ($srv -and (Test-Tcp $srv 445 2500)) {
    Say ($L + ' (' + $rp + '): il server risponde. La apro per ricollegarla.')
    Open-User ($L + '\')
    if (-not (Test-Path -LiteralPath $rp)) { Say ('Se ' + $L + ' chiede la password o dice "accesso negato", potrebbe essere cambiata la password: usa "La cartella di rete chiede sempre la password".') }
  } else { Say ($L + ' (' + $rp + '): il server NON risponde. Sei collegato alla rete dell''ufficio (o alla VPN se sei fuori)?'); $bad++ }
}
if ($bad -eq 0) { Say 'Tutte le unità sono raggiungibili.' } else { Say 'Quando sei di nuovo collegato alla rete dell''ufficio, apri "Questo PC" e fai doppio clic sulle unità per ricollegarle.' }
`)},
	{ID: "of_netpwd", Name: "La cartella di rete chiede sempre la password", Desc: "Ti mostra le password salvate per i server dell'ufficio e ti apre il posto dove sistemarle. La password nuova la scrivi tu.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs(`
$list = (cmdkey /list) -join "§"
$tg = @(); foreach ($m in [regex]::Matches($list, '(?:Destinazione|Target):\s*Domain:target=([^§]+)')) { $tg += $m.Groups[1].Value.Trim() }
if ($tg.Count) { Say ('Password di Windows salvate per questi server: ' + ($tg -join ', ') + '. Se hai cambiato la password del lavoro, quella salvata è vecchia: va aggiornata.') }
else { Say 'In Windows non c''è nessuna password salvata per i server dell''ufficio.' }
Say 'Ti apro "Gestione credenziali" > Credenziali di Windows. Per sistemare: apri il server giusto > Modifica, scrivi la password nuova e salva. Se non c''è: "Aggiungi credenziale di Windows", indirizzo del server (es. \\server), utente nella forma DOMINIO\nome.'
[void](Start-AsUser (Join-Path $env:SystemRoot 'System32\control.exe') '/name Microsoft.CredentialManager')
`)},
	{ID: "of_proxy", Name: "Internet c'è ma i siti non si aprono", Desc: "Controlla il \"proxy\" (un passaggio obbligato per internet): se è rimasto quello di un programma tolto e non funziona, lo spegne (con copia di sicurezza).", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs(`
$k = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings'
$ip = Get-ItemProperty -LiteralPath $k -ErrorAction SilentlyContinue
$on = [int]$ip.ProxyEnable; $srv = [string]$ip.ProxyServer; $pac = [string]$ip.AutoConfigURL
if (-not $on -and -not $pac) { Say 'Nessun proxy impostato: internet va diretto, ok.' }
if ($on -and $srv) {
  $first = ($srv -split ';')[0] -replace '^[a-z]+=', ''
  $h = $first; $port = 80; if ($first -match '^(.+):(\d+)$') { $h = $Matches[1]; $port = [int]$Matches[2] }
  $ok = Test-Tcp $h $port
  if ($ok) { Say ('Proxy acceso (' + $srv + ') e funziona: probabilmente lo usa l''ufficio, lo lascio.') }
  elseif ($h -match '^(127\.0\.0\.1|localhost|::1)$') {
    $b = Backup-Reg 'HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings' 'proxy'
    if ($b) {
      Set-ItemProperty -LiteralPath $k -Name ProxyEnable -Value 0 -Type DWord
      $ck = $k + '\Connections'
      $bl = $null; try { $bl = (Get-ItemProperty -LiteralPath $ck -Name DefaultConnectionSettings -ErrorAction Stop).DefaultConnectionSettings } catch {}
      if ($bl -and $bl.Length -gt 8) { $bl[8] = $bl[8] -band 0xFD; Set-ItemProperty -LiteralPath $ck -Name DefaultConnectionSettings -Value $bl -Type Binary -ErrorAction SilentlyContinue }
      try { Add-Type -Namespace TassoNet -Name WI -MemberDefinition '[DllImport("wininet.dll")] public static extern bool InternetSetOption(IntPtr h, int o, IntPtr b, int l);' -ErrorAction Stop; [void][TassoNet.WI]::InternetSetOption([IntPtr]::Zero, 39, [IntPtr]::Zero, 0); [void][TassoNet.WI]::InternetSetOption([IntPtr]::Zero, 37, [IntPtr]::Zero, 0) } catch {}
      Say ('C''era un proxy rimasto da un programma (' + $srv + ') che non funziona più: spento. Copia di sicurezza: ' + $b)
    }
  } else { Say ('Proxy acceso (' + $srv + ') ma NON risponde. Se sei fuori ufficio è normale (funziona solo lì): chiedi all''assistenza (IT) se puoi spegnerlo in Impostazioni > Rete > Proxy.'); Open-User 'ms-settings:network-proxy' }
}
if ($pac) { Say ('Configurazione automatica del proxy: ' + $pac + '. La imposta di solito l''ufficio.') }
$wh = (netsh winhttp show proxy) -join ' '
if ($wh -match '(?:Server proxy|Proxy Server)(?:\(s\))?\s*:\s*(\S+)') { Say ('Proxy di sistema (per Windows Update e app): ' + $Matches[1]) }
Clear-DnsClientCache
if (Test-Internet) { Say 'Il PC raggiunge internet. Se un solo sito non si apre, il problema è di quel sito: prova da un''altra rete o più tardi.' } else { Say 'Il PC non raggiunge internet: usa "Internet non va".' }
`)},

	// ---------- Schermo, tastiera e periferiche ----------
	{ID: "of_second_screen", Name: "Il secondo schermo è nero", Desc: "Fa ricercare gli schermi collegati e mette \"Estendi\" (come Win+P), così il secondo monitor si accende accanto al primo.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs(`
[void](Run-Hidden 'pnputil.exe' '/scan-devices')
Start-Sleep -Seconds 2
$mon = @(Get-CimInstance -Namespace root\wmi -ClassName WmiMonitorBasicDisplayParams -ErrorAction SilentlyContinue | Where-Object { $_.Active })
Say ('Schermi che Windows vede: ' + $mon.Count + '.')
if ($mon.Count -lt 2) { Say 'Windows vede un solo schermo: controlla che il secondo sia acceso, che il cavo sia ben attaccato alle due parti e che sul monitor sia scelto l''ingresso giusto (tasto "Input" o "Source": HDMI, DisplayPort...).' }
if (Use-Display) {
  $r = [TassoDisplay]::Topology(4)
  if ($r -eq 0) { Say 'Messo "Estendi": il desktop continua sul secondo schermo.' } else { Say 'Windows non ha accettato "Estendi": premi Win + P e scegli Estendi.' }
}
Say 'Se gli schermi sono "al contrario" (il mouse esce dal lato sbagliato): Impostazioni > Schermo, trascina i riquadri 1 e 2 come sono sulla scrivania. Te lo apro.'
Open-User 'ms-settings:display'
`)},
	{ID: "of_projector", Name: "Il proiettore o la TV non mostrano niente", Desc: "Mette \"Duplica\" (come Win+P): il proiettore o la TV mostrano lo stesso schermo del portatile.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs(`
[void](Run-Hidden 'pnputil.exe' '/scan-devices')
Start-Sleep -Seconds 2
$mon = @(Get-CimInstance -Namespace root\wmi -ClassName WmiMonitorBasicDisplayParams -ErrorAction SilentlyContinue | Where-Object { $_.Active })
if ($mon.Count -lt 2) { Say 'Windows non vede il proiettore: controlla il cavo (HDMI) e che sul proiettore sia scelto l''ingresso giusto. Con gli adattatori USB-C prova a staccarlo e riattaccarlo.' }
if (Use-Display) {
  $r = [TassoDisplay]::Topology(2)
  if ($r -eq 0) { Say 'Messo "Duplica": il proiettore mostra lo stesso schermo del PC.' } else { Say 'Windows non ha accettato "Duplica": premi Win + P e scegli Duplica.' }
}
Say 'Se l''immagine è tagliata o piccola, Windows usa la risoluzione più adatta a tutti e due: per le presentazioni va bene così. Per tornare a uno schermo solo: Win + P > "Solo schermo PC".'
`)},
	{ID: "of_rotated", Name: "Lo schermo è girato", Desc: "Rimette dritti gli schermi girati di lato o sottosopra (succede premendo per sbaglio Ctrl+Alt+frecce).", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs(`
if (-not (Use-Display)) { Say 'Non riesco a parlare con lo schermo: usa Impostazioni > Schermo > Orientamento > Orizzontale.'; Open-User 'ms-settings:display'; return }
$r = [TassoDisplay]::Straighten()
if ($r) { Say ('Schermo rimesso dritto (' + ($r.TrimEnd(';') -replace '\\\\\.\\', '' -replace ';', ', ') + '). Se avevi un monitor messo in verticale apposta, rimettilo da Impostazioni > Schermo > Orientamento.') }
else { Say 'Per Windows gli schermi sono già dritti. Se vedi ancora l''immagine girata, prova Ctrl + Alt + freccia su, oppure Impostazioni > Schermo > Orientamento.'; Open-User 'ms-settings:display' }
`)},
	{ID: "of_blurry", Name: "Scritte sfocate o troppo piccole", Desc: "Accende la correzione di Windows per i programmi sfocati e la scrittura più nitida (ClearType). Poi ti apre la grandezza del testo.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs(`
$d = 'HKCU:\Control Panel\Desktop'
$b = Backup-Reg 'HKCU\Control Panel\Desktop' 'schermo-nitido'
if (-not $b) { Say 'Non sono riuscito a fare la copia di sicurezza, quindi per sicurezza non cambio niente. Ti apro le impostazioni dello schermo.'; Open-User 'ms-settings:display'; return }
Set-ItemProperty -LiteralPath $d -Name EnablePerProcessSystemDPI -Value 1 -Type DWord -ErrorAction SilentlyContinue
Say 'Accesa "Consenti a Windows di correggere le app sfocate".'
Set-ItemProperty -LiteralPath $d -Name FontSmoothing -Value '2' -ErrorAction SilentlyContinue
Set-ItemProperty -LiteralPath $d -Name FontSmoothingType -Value 2 -Type DWord -ErrorAction SilentlyContinue
Say ('Accesa la scrittura nitida (ClearType). Copia di sicurezza delle impostazioni di prima: ' + $b)
$mon = @(Get-CimInstance Win32_VideoController -ErrorAction SilentlyContinue | Where-Object { $_.CurrentHorizontalResolution })
foreach ($m in $mon) { Say ('Risoluzione attuale: ' + $m.CurrentHorizontalResolution + ' x ' + $m.CurrentVerticalResolution + '.') }
Say 'Ti apro Impostazioni > Schermo: in "Risoluzione" scegli quella con scritto (consigliato); in "Scala" alza a 125% o 150% se le scritte sono piccole. Per alcuni programmi serve uscire e rientrare in Windows.'
Open-User 'ms-settings:display'
`)},
	{ID: "of_keyboard", Name: "La tastiera scrive i simboli sbagliati", Desc: "Rimette la tastiera italiana come predefinita e spegne Alt+Maiusc, la scorciatoia che la cambia per sbaglio in inglese (con copia di sicurezza).", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs(`
$langs = @(Get-WinUserLanguageList -ErrorAction SilentlyContinue)
$tips = @(); foreach ($l in $langs) { foreach ($t in $l.InputMethodTips) { $tips += [string]$t } }
Say ('Tastiere installate: ' + ((($langs | ForEach-Object { $_.LanguageTag }) -join ', ')) + '.')
if ($TassoLang -eq 'en') {
  # In inglese: la tastiera "giusta" è la prima della tua lingua principale di Windows.
  $first = ''; if ($langs.Count -gt 0 -and @($langs[0].InputMethodTips).Count -gt 0) { $first = [string]@($langs[0].InputMethodTips)[0] }
  if ($first) { Set-WinDefaultInputMethodOverride -InputTip $first -ErrorAction SilentlyContinue; Say ('Default keyboard set to the first one of ' + $langs[0].LanguageTag + '.') }
  else { Say 'I could not read your keyboards: add or pick one in Settings > Time & language > Language & region (I am opening it).'; Open-User 'ms-settings:regionlanguage' }
}
elseif ($tips -contains '0410:00000410') { Set-WinDefaultInputMethodOverride -InputTip '0410:00000410' -ErrorAction SilentlyContinue; Say 'Tastiera italiana messa come predefinita.' }
else { Say 'La tastiera italiana non è installata: aggiungila da Impostazioni > Ora e lingua > Lingua (te lo apro).'; Open-User 'ms-settings:regionlanguage' }
$k = 'HKCU:\Keyboard Layout\Toggle'
$b = Backup-Reg 'HKCU\Keyboard Layout\Toggle' 'tastiera-scorciatoie'
if ((Test-Path -LiteralPath $k) -and -not $b) { Say 'Non sono riuscito a fare la copia di sicurezza delle scorciatoie, quindi non le cambio. Per cambiare tastiera usa Win + Spazio.'; return }
if (-not (Test-Path -LiteralPath $k)) { New-Item -Path $k -Force | Out-Null }
foreach ($n in @('Hotkey', 'Language Hotkey', 'Layout Hotkey')) { Set-ItemProperty -LiteralPath $k -Name $n -Value '3' -ErrorAction SilentlyContinue }
Say 'Spente le scorciatoie Alt+Maiusc e Ctrl+Maiusc che cambiano tastiera per sbaglio. Se ti serve cambiarla apposta usa Win + Spazio.'
if ($b) { Say ('Copia di sicurezza delle scorciatoie: ' + $b) }
Say 'Altri casi: se escono numeri al posto delle lettere sul portatile premi "Bloc Num"; se scrive tutto MAIUSCOLO premi "Bloc Maiusc". Serve uscire e rientrare in Windows perché valga ovunque.'
`)},
	{ID: "of_usb", Name: "Chiavetta, disco USB o docking non vanno", Desc: "Riavvia le porte USB bloccate, fa ricercare i dispositivi e dà una lettera (E:, F:...) alle chiavette che non ce l'hanno. Non formatta mai niente.", Level: 0, Minutes: 0.4, Timeout: 4 * time.Minute, Script: ofs(`
Say 'Cerco porte USB e dispositivi bloccati...'
$n = Restart-BadDevices 'USB|DiskDrive|WPD|Volume|HIDClass|Net'
[void](Run-Hidden 'pnputil.exe' '/scan-devices')
Start-Sleep -Seconds 3
$usb = @(Get-Disk -ErrorAction SilentlyContinue | Where-Object { [string]$_.BusType -match 'USB|SD|MMC' })
$net = @(Get-ChildItem -LiteralPath 'HKCU:\Network' -ErrorAction SilentlyContinue | ForEach-Object { $_.PSChildName.ToUpper() })
foreach ($d in $usb) {
  if ($d.IsOffline) {
    if ([string]$d.OfflineReason -match 'Signature') { Say ($d.FriendlyName + ': Windows lo tiene "non in linea" perché è la copia identica di un altro disco (succede con i dischi clonati). Non lo tocco: rimettilo in linea da Gestione disco se sai che è giusto.') }
    else { Set-Disk -Number $d.Number -IsOffline $false -ErrorAction SilentlyContinue; Say ($d.FriendlyName + ': era "non in linea", l''ho rimesso in linea.') }
  }
  foreach ($p in @(Get-Partition -DiskNumber $d.Number -ErrorAction SilentlyContinue | Where-Object { -not $_.DriveLetter -and -not $_.NoDefaultDriveLetter -and -not $_.IsHidden -and [string]$_.Type -notmatch 'Reserved|System|Recovery|Unknown' -and $_.Size -gt 50MB })) {
    $used = $net + @([IO.DriveInfo]::GetDrives() | ForEach-Object { $_.Name.Substring(0, 1).ToUpper() })
    $free = @(69..90 | ForEach-Object { [string][char]$_ } | Where-Object { $used -notcontains $_ })[0]
    if ($free) { try { Set-Partition -DiskNumber $d.Number -PartitionNumber $p.PartitionNumber -NewDriveLetter $free -ErrorAction Stop; Say ($d.FriendlyName + ': mancava la lettera, ora è ' + $free + ':') } catch {} }
  }
  foreach ($v in @(Get-Partition -DiskNumber $d.Number -ErrorAction SilentlyContinue | Where-Object { $_.DriveLetter } | Get-Volume -ErrorAction SilentlyContinue)) {
    $dl = [string]$v.DriveLetter
    if ($dl -and $net -contains $dl.ToUpper()) { Say ($d.FriendlyName + ': ha la lettera ' + $dl + ': che è già usata da un''unità di rete, per questo non la vedi. Cambiala in Gestione disco (tasto destro su Start > Gestione disco > tasto destro sulla chiavetta > Cambia lettera).'); continue }
    if ([string]$v.FileSystem -eq 'RAW' -or -not $v.FileSystem) { Say ($d.FriendlyName + ' (' + $v.DriveLetter + ':): Windows non la legge e potrebbe chiederti di formattarla. NON formattarla se dentro ci sono file importanti: falla vedere a un tecnico.') }
    else { Say ($d.FriendlyName + ': pronta come ' + $v.DriveLetter + ':') }
  }
}
if (-not $usb.Count) { Say 'Non vedo chiavette o dischi USB collegati. Prova un''altra porta (meglio dietro il PC) e un altro cavo. Con la docking: stacca l''alimentatore della docking 10 secondi e riattaccalo.' }
Say 'Fatto. Per staccare una chiavetta senza rovinare i file usa sempre "Rimozione sicura" (icona della chiavetta in basso a destra).'
`)},
	{ID: "of_clipboard", Name: "Copia e incolla non funziona", Desc: "Riavvia il servizio degli appunti di Windows (e quello del Desktop remoto, se lo usi).", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs(`
$n = 0
foreach ($s in @(Get-Service -Name 'cbdhsvc*' -ErrorAction SilentlyContinue)) { Restart-Service -Name $s.Name -Force -ErrorAction SilentlyContinue; $n++ }
if ($n) { Say 'Servizio degli appunti di Windows riavviato.' }
$rd = @(Get-Process -Name rdpclip -ErrorAction SilentlyContinue)
if ($rd.Count) { $rd | Stop-Process -Force -ErrorAction SilentlyContinue; Start-Sleep -Seconds 1; [void](Start-AsUser (Join-Path $env:SystemRoot 'System32\rdpclip.exe') ''); Say 'Riavviato anche il copia e incolla del Desktop remoto.' }
Say 'Riprova a copiare (Ctrl + C) e incollare (Ctrl + V). Trucco: Win + V apre la cronologia degli appunti con le ultime cose copiate.'
Say 'Se non va solo in un programma, chiudi e riapri quel programma (spesso è lui bloccato, per esempio Excel durante un calcolo lungo).'
`)},

	// ---------- Windows lento o bloccato ----------
	{ID: "of_explorer", Name: "Esplora file si blocca o è lento", Desc: "Riavvia Esplora file e controlla se un'unità di rete che non risponde lo sta rallentando. Le finestre delle cartelle aperte si chiudono: se stai copiando file, aspetta che finisca.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs(`
foreach ($m in @(Get-ChildItem -LiteralPath 'HKCU:\Network' -ErrorAction SilentlyContinue)) {
  $rp = ''; try { $rp = [string](Get-ItemProperty -LiteralPath $m.PSPath -Name RemotePath -ErrorAction Stop).RemotePath } catch {}
  if ($rp -match '^\\\\([^\\]+)' -and -not (Test-Tcp $Matches[1] 445 1500)) { Say ('L''unità ' + $m.PSChildName.ToUpper() + ': (' + $rp + ') non risponde: Esplora file aspetta lei e si blocca. Collegati alla rete dell''ufficio o scollegala (tasto destro > Disconnetti).') }
}
Say 'Riavvio Esplora file (barra e desktop spariscono per un attimo)...'
Restart-Explorer
Say 'Fatto. Se una cartella con tante foto o video è lenta: tasto destro dentro la cartella > Visualizza > Elenco (senza anteprime) va molto più veloce.'
`)},
	{ID: "of_taskbar", Name: "Barra in basso bloccata o icone sparite", Desc: "Ricrea le icone di Windows e riavvia barra delle applicazioni, menu Start e area notifiche. Se stai copiando file, aspetta che finisca.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs(`
Say 'Riavvio barra, menu Start e icone (spariscono per qualche secondo)...'
Get-Process -Name explorer -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
$f = [long]0
foreach ($p in @((Join-Path $env:LOCALAPPDATA 'IconCache.db')) + @(Get-ChildItem -LiteralPath (Join-Path $env:LOCALAPPDATA 'Microsoft\Windows\Explorer') -Filter 'iconcache_*.db' -Force -ErrorAction SilentlyContinue | ForEach-Object { $_.FullName })) {
  if (Test-Path -LiteralPath $p) { $len = (Get-Item -LiteralPath $p -Force).Length; Remove-Item -LiteralPath $p -Force -ErrorAction SilentlyContinue; if (-not (Test-Path -LiteralPath $p)) { $f += $len } }
}
Get-Process -Name StartMenuExperienceHost, ShellExperienceHost, SearchHost -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 5
if (-not (Get-Process -Name explorer -ErrorAction SilentlyContinue)) { Start-Process -FilePath (Join-Path $env:SystemRoot 'explorer.exe') }
[void](Run-Hidden 'ie4uinit.exe' '-show')
Freed $f
if ($f -gt 0) { Say 'Fatto: icone ricreate. Se qualche icona è ancora bianca, sparisce al prossimo riavvio del PC.' }
else { Say 'Barra e menu Start riavviati. Le icone erano in uso da Windows: si ricreano al prossimo riavvio del PC.'; [Console]::Out.WriteLine('REBOOT') }
`)},
	{ID: "of_hung", Name: "Un programma non risponde", Desc: "Trova i programmi bloccati (\"Non risponde\") da più di 20 secondi e li chiude. Word, Excel e Outlook non li chiude mai (spesso stanno solo salvando). Il lavoro non salvato nel programma chiuso si perde.", Level: 0, Minutes: 0.3, Timeout: 2 * time.Minute, Script: ofs(`
$skip = 'explorer|Tasso|dwm|csrss|winlogon|ShellExperienceHost|StartMenuExperienceHost|SearchHost|SearchApp|TextInputHost|ApplicationFrameHost|SystemSettings|LockApp|Widgets'
# Le app di Windows e dello Store sospese risultano "non risponde" anche se vanno: non le chiudo mai.
$a = @(Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.MainWindowHandle -ne 0 -and -not $_.Responding -and $_.ProcessName -notmatch ('^(' + $skip + ')$') -and "$($_.Path)" -notmatch '\\(WindowsApps|SystemApps)\\' } | ForEach-Object { $_.Id })
if (-not $a.Count) { Say 'In questo momento nessun programma risulta bloccato. Se uno era lento, forse stava solo lavorando: aspetta un minuto.'; return }
Say 'Ricontrollo per 20 secondi, per essere sicuro che siano bloccati davvero...'
foreach ($i in 1..3) { Start-Sleep -Seconds 7; $a = @(Get-Process -Id $a -ErrorAction SilentlyContinue | Where-Object { -not $_.Responding } | ForEach-Object { $_.Id }); if (-not $a.Count) { break } }
$closed = @(); $office = @()
foreach ($p in @(if ($a.Count) { Get-Process -Id $a -ErrorAction SilentlyContinue | Where-Object { -not $_.Responding } })) {
  $on = $OfficeNames[$p.ProcessName.ToUpper()]
  if ($on) { $office += $on; continue }
  $n = $p.MainWindowTitle; if (-not $n) { $n = $p.ProcessName }
  try { $p | Stop-Process -Force -ErrorAction Stop; $closed += $n } catch {}
}
if ($office.Count) { Say ((($office | Select-Object -Unique) -join ', ') + ' non risponde: di solito sta salvando, calcolando o sincronizzando. Non lo chiudo per non farti perdere il lavoro: aspetta qualche minuto. Se resta bloccato, chiudilo tu da Gestione attività (Ctrl + Maiusc + Esc).') }
if ($closed.Count) { Say ('Chiusi perché bloccati: ' + ($closed -join ', ') + '. Riaprili: il lavoro non salvato in quei programmi potrebbe essersi perso.') }
elseif (-not $office.Count) { Say 'Si sono sbloccati da soli: non ho chiuso niente.' }
`)},
	{ID: "of_fonts", Name: "Scritte strane o caratteri che mancano", Desc: "Ricostruisce la memoria dei caratteri di Windows quando le scritte si vedono a quadratini o con il carattere sbagliato. Serve un riavvio.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs(`
Say 'Fermo il servizio dei caratteri...'
foreach ($s in @('FontCache', 'FontCache3.0.0.0')) { Stop-Service -Name $s -Force -ErrorAction SilentlyContinue }
Start-Sleep -Seconds 2
$f = [long]0
$dir = Join-Path $env:SystemRoot 'ServiceProfiles\LocalService\AppData\Local\FontCache'
foreach ($x in @(Get-ChildItem -LiteralPath $dir -Filter '*FontCache*' -File -Force -ErrorAction SilentlyContinue) + @(Get-Item -LiteralPath (Join-Path $env:SystemRoot 'System32\FNTCACHE.DAT') -Force -ErrorAction SilentlyContinue)) {
  if ($x) { $len = $x.Length; Remove-Item -LiteralPath $x.FullName -Force -ErrorAction SilentlyContinue; if (-not (Test-Path -LiteralPath $x.FullName)) { $f += $len } }
}
foreach ($s in @('FontCache', 'FontCache3.0.0.0')) { Start-Service -Name $s -ErrorAction SilentlyContinue }
Freed $f
if ($f -gt 0) { Say 'Memoria dei caratteri svuotata: Windows la rifà da sola. Riavvia il PC per vedere le scritte a posto.' }
else { Say 'Windows non mi ha lasciato svuotare tutta la memoria dei caratteri: il riavvio del PC la rinfresca comunque. Riavvialo.' }
Say 'Se manca un carattere preciso in un documento (es. ricevuto da un collega), va installato: doppio clic sul file del carattere > Installa.'
[Console]::Out.WriteLine('REBOOT')
`)},
	{ID: "of_thumbs", Name: "Non vedo le anteprime di foto e documenti", Desc: "Riaccende le anteprime in Esplora file se erano spente e ricrea le miniature rovinate. Se stai copiando file, aspetta che finisca.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs(`
$k = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced'
$io = 0; try { $io = [int](Get-ItemProperty -LiteralPath $k -Name IconsOnly -ErrorAction Stop).IconsOnly } catch {}
if ($io -eq 1) { Set-ItemProperty -LiteralPath $k -Name IconsOnly -Value 0 -Type DWord; Say 'Era accesa "Mostra sempre icone, mai anteprime": spenta.' }
Say 'Ricreo le miniature (Esplora file si riavvia per un attimo)...'
Get-Process -Name explorer -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 800
$f = [long]0
foreach ($x in @(Get-ChildItem -LiteralPath (Join-Path $env:LOCALAPPDATA 'Microsoft\Windows\Explorer') -Filter 'thumbcache_*.db' -Force -ErrorAction SilentlyContinue)) { $len = $x.Length; Remove-Item -LiteralPath $x.FullName -Force -ErrorAction SilentlyContinue; if (-not (Test-Path -LiteralPath $x.FullName)) { $f += $len } }
Start-Sleep -Seconds 4
if (-not (Get-Process -Name explorer -ErrorAction SilentlyContinue)) { Start-Process -FilePath (Join-Path $env:SystemRoot 'explorer.exe') }
Freed $f
if ($f -eq 0) { Say 'Le miniature erano in uso da Windows: si ricreano al prossimo riavvio del PC.'; [Console]::Out.WriteLine('REBOOT') }
Say 'Le anteprime si rifanno aprendo le cartelle. Per i PDF serve un lettore che le crea (Adobe Acrobat Reader le fa; anche il "Riquadro di anteprima" con Alt + P).'
`)},
	{ID: "of_ext", Name: "Non vedo .pdf .docx alla fine dei file", Desc: "Fa vedere sempre l'estensione dei file (.pdf, .docx, .exe). Così capisci cos'è un file e riconosci gli allegati truffa come fattura.pdf.exe.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs(`
$k = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced'
$v = 1; try { $v = [int](Get-ItemProperty -LiteralPath $k -Name HideFileExt -ErrorAction Stop).HideFileExt } catch {}
if ($v -ne 0) { Set-ItemProperty -LiteralPath $k -Name HideFileExt -Value 0 -Type DWord; Say 'Fatto: ora Esplora file mostra la fine dei nomi dei file (.pdf, .docx, .xlsx, .exe). Premi F5 o riapri la cartella per vederla.' } else { Say 'Le estensioni si vedevano già.' }
Say 'Perché serve: un file "fattura.pdf.exe" è un programma, non un PDF. Se arriva per email e finisce con .exe, .js, .vbs, .scr o .bat NON aprirlo.'
Say 'Per tornare come prima: Esplora file > Visualizza > Mostra > togli la spunta a "Estensioni nomi file".'
`)},
	{ID: "of_slow_boot", Name: "Il PC ci mette tanto ad accendersi", Desc: "Misura quanto ci mette Windows a partire, trova i programmi che lo rallentano e ti dice cosa spegnere. Controlla e spiega.", Level: 0, Minutes: 0.4, Timeout: 3 * time.Minute, Script: ofs(`
$ev = @(Get-WinEvent -FilterHashtable @{ LogName = 'Microsoft-Windows-Diagnostics-Performance/Operational'; Id = 100 } -MaxEvents 5 -ErrorAction SilentlyContinue)
$t = @(); foreach ($e in $ev) { try { $x = [xml]$e.ToXml(); $b = ($x.Event.EventData.Data | Where-Object { $_.Name -eq 'BootTime' }).'#text'; if ($b) { $t += [double]$b / 1000 } } catch {} }
if ($t.Count) { $avg = [math]::Round(($t | Measure-Object -Average).Average); Say ('Le ultime accensioni sono durate in media ' + $avg + ' secondi (fino a quando puoi usare il PC).'); if ($avg -gt 90) { Say 'È lento: un PC da ufficio con disco SSD dovrebbe starci sotto il minuto.' } }
else { Say 'Windows non ha ancora i tempi di accensione (si misurano dopo qualche riavvio).' }
$slow = @(Get-WinEvent -FilterHashtable @{ LogName = 'Microsoft-Windows-Diagnostics-Performance/Operational'; Id = 101 } -MaxEvents 30 -ErrorAction SilentlyContinue)
$names = @{}; foreach ($e in $slow) { try { $x = [xml]$e.ToXml(); $fn = ($x.Event.EventData.Data | Where-Object { $_.Name -eq 'FriendlyName' }).'#text'; if ($fn) { $names[$fn] = 1 } } catch {} }
if ($names.Count) { Say ('Programmi che Windows ha visto rallentare l''accensione: ' + (($names.Keys | Select-Object -First 6) -join ', ') + '.') }
$st = @(Get-TassoStartup | Where-Object { $_.enabled })
Say ('Programmi che partono da soli: ' + $st.Count + '.')
if ($st.Count -gt 8) { Say 'Sono tanti: in Velocità > Programmi all''avvio spegni quelli che non ti servono subito (Spotify, Steam, Skype, aggiornatori...). Antivirus, OneDrive e il programma della VPN lasciali accesi.' }
$sys = Get-PSDrive -Name ($env:SystemDrive.Trim(':')) -ErrorAction SilentlyContinue
if ($sys -and $sys.Free -lt 10GB) { Say ('Il disco C: ha solo ' + (Fmt $sys.Free) + ' liberi: anche questo rallenta. Fai una pulizia.') }
$sd = Get-Partition -DriveLetter ($env:SystemDrive.Trim(':')) -ErrorAction SilentlyContinue | Get-Disk -ErrorAction SilentlyContinue
$hdd = @(Get-PhysicalDisk -ErrorAction SilentlyContinue | Where-Object { $sd -and [string]$_.DeviceId -eq [string]$sd.Number -and [string]$_.MediaType -eq 'HDD' })
if ($hdd.Count) { Say 'Questo PC ha un disco "meccanico" (HDD): è la causa principale della lentezza. Passare a un SSD (circa 40-60 euro) lo rende 3-4 volte più veloce ad accendersi.' }
$up = (Get-Date) - (Get-CimInstance Win32_OperatingSystem).LastBootUpTime
if ($up.TotalDays -gt 7) { Say ('Il PC non si riavvia da ' + [int]$up.TotalDays + ' giorni (con "Arresta" Windows non si spegne del tutto): ogni tanto fai Riavvia, aiuta molto.') }
`)},
	{ID: "of_sleep", Name: "Il PC non va in sospensione o si riaccende da solo", Desc: "Ti dice quale programma o dispositivo tiene sveglio il PC o lo ha riacceso. Controlla e spiega cosa fare.", Level: 0, Minutes: 0.3, Timeout: 2 * time.Minute, Script: ofs(`
$req = (powercfg /requests) 2>$null
$cur = ''; $hits = @()
foreach ($l in $req) { $s = [string]$l; if ($s -match '^[A-Z]+:\s*$') { $cur = $s.TrimEnd(':').Trim() } elseif ($s.Trim() -and $s -notmatch '^\s*(Nessuno|None)\.?\s*$' -and $cur -match 'DISPLAY|SYSTEM|AWAYMODE|EXECUTION') { $hits += ($cur + ': ' + $s.Trim()) } }
if ($hits.Count) { Say ('Tengono sveglio il PC adesso: ' + (($hits | Select-Object -First 6) -join ' | ')) ; Say 'Di solito è un video o una riunione aperta nel browser, una stampa in coda o un download: chiudili e il PC va in sospensione.' }
else { Say 'Adesso nessun programma impedisce la sospensione.' }
$lw = ((powercfg /lastwake) 2>$null) -join ' '
if ($lw -match '(?:Nome descrittivo|Friendly Name)\s*:\s*(.+?)(?:\s{2,}|$)') { Say ('L''ultima volta il PC l''ha svegliato: ' + $Matches[1].Trim() + '.') }
$wt = ((powercfg /waketimers) 2>$null) -join ' '
if ($wt -match '\[[A-Z]+\]') { Say 'Ci sono attività programmate che possono riaccendere il PC (spesso aggiornamenti di Windows). Si possono spegnere in Pannello di controllo > Opzioni risparmio energia > Modifica impostazioni avanzate > Sospensione > "Consenti timer di riattivazione".' }
$arm = @((powercfg /devicequery wake_armed) 2>$null | Where-Object { $_ -and $_ -notmatch '^(NONE|NESSUNO)$' })
if ($arm.Count) { Say ('Possono svegliare il PC: ' + ($arm -join ', ') + '. Se si riaccende da solo di notte, in Gestione dispositivi apri quel dispositivo > Risparmio energia > togli "Consenti al dispositivo di riattivare il computer" (di solito il mouse o la scheda di rete).') }
`)},

	// ---------- Ora, OneDrive e sicurezza ----------
	{ID: "of_time", Name: "L'ora è sbagliata (e i siti danno errore)", Desc: "Rimette l'ora giusta da internet. Con l'ora sbagliata i siti dicono \"connessione non privata\" e VPN e Office non si collegano.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs(`
$sv = Get-Service -Name W32Time -ErrorAction SilentlyContinue
if ($sv -and $sv.StartType -eq 'Disabled') { Set-Service -Name W32Time -StartupType Manual -ErrorAction SilentlyContinue; Say 'Il servizio dell''ora di Windows era spento del tutto: riacceso.' }
Start-Service -Name W32Time -ErrorAction SilentlyContinue
$dom = [bool](Get-CimInstance Win32_ComputerSystem -ErrorAction SilentlyContinue).PartOfDomain
Say 'Sincronizzo l''ora...'
$c = Run-Hidden 'w32tm.exe' '/resync /force'
if ($c -ne 0 -and -not $dom) {
  $bk = Backup-Reg 'HKLM\SYSTEM\CurrentControlSet\Services\W32Time\Parameters' 'ora-server'
  if ($bk) { Say ('Uso il server dell''ora di Microsoft (time.windows.com). Copia di sicurezza delle impostazioni di prima: ' + $bk) }
  [void](Run-Hidden 'w32tm.exe' '/config /manualpeerlist:"time.windows.com,0x9" /syncfromflags:manual /update')
  Restart-Service -Name W32Time -Force -ErrorAction SilentlyContinue
  Start-Sleep -Seconds 2
  $c = Run-Hidden 'w32tm.exe' '/resync /force'
}
if ($c -eq 0) { Say ('Ora sincronizzata: ' + (Get-Date -Format 'dd/MM/yyyy HH:mm') + '.') } elseif ($dom) { Say 'Questo PC prende l''ora dal server dell''ufficio, che ora non risponde: si sistema da solo quando sei collegato alla rete dell''ufficio.' } else { Say 'Non sono riuscito a sincronizzare. Se l''ora è sbagliata di molte ore o di giorni (succede quando si scarica la pila del PC), Windows non la corregge da solo: mettila a mano una volta nella finestra che ti apro (Imposta ora > Modifica). Se invece internet non va, sistemalo prima.'; Open-User 'ms-settings:dateandtime' }
$tz = Get-TimeZone -ErrorAction SilentlyContinue
if ($tz -and $tz.Id -ne 'W. Europe Standard Time' -and (Get-Culture).Name -eq 'it-IT') { Say ('Il fuso orario è "' + $tz.DisplayName + '". In Italia deve essere (UTC+01:00) Amsterdam, Berlino, Roma... Se non sei all''estero, cambialo nella finestra che ti apro.'); Open-User 'ms-settings:dateandtime' }
`)},
	{ID: "of_onedrive", Name: "OneDrive non sincronizza", Desc: "Fa il \"reset\" ufficiale di OneDrive: si riavvia e ricontrolla tutti i file. I file non vengono cancellati, né sul PC né online.", Level: 0, Minutes: 0.5, Timeout: 4 * time.Minute, Script: ofs(`
$exe = ''
foreach ($p in @((Join-Path $env:LOCALAPPDATA 'Microsoft\OneDrive\OneDrive.exe'), (Join-Path $env:ProgramFiles 'Microsoft OneDrive\OneDrive.exe'), (Join-Path ${env:ProgramFiles(x86)} 'Microsoft OneDrive\OneDrive.exe'))) { if ($p -and (Test-Path -LiteralPath $p)) { $exe = $p; break } }
if (-not $exe) { Say 'OneDrive non è installato su questo PC.'; return }
$c = Get-PSDrive -Name ($env:SystemDrive.Trim(':')) -ErrorAction SilentlyContinue
if ($c -and $c.Free -lt 2GB) { Say ('Il disco C: è quasi pieno (' + (Fmt $c.Free) + '): OneDrive si ferma per questo. Libera spazio con una pulizia o con "Libera spazio con OneDrive".') }
Say 'Faccio il reset di OneDrive (l''icona sparisce per un minuto)...'
[void](Start-AsUser $exe '/reset')
Start-Sleep -Seconds 25
if (-not (Get-Process -Name OneDrive -ErrorAction SilentlyContinue)) { Say 'Riapro OneDrive...'; [void](Start-AsUser $exe '') }
Say 'Fatto: OneDrive ricontrolla tutti i file, la prima volta ci mette un po''. Se compare una X rossa su un file, di solito il nome ha caratteri strani o è troppo lungo: rinominalo.'
Say 'Se ti chiede di nuovo quali cartelle sincronizzare (succede con le cartelle di SharePoint del lavoro), rimettile come prima.'
`)},
	{ID: "of_onedrive_space", Name: "Libera spazio con OneDrive", Desc: "I file di OneDrive che non modifichi da 60 giorni restano online e smettono di occupare il disco. Li vedi sempre, si riscaricano quando li apri (serve internet).", Level: 0, Minutes: 1, Timeout: 15 * time.Minute, Script: ofs(`
$roots = @()
foreach ($a in @(Get-ChildItem -LiteralPath 'HKCU:\Software\Microsoft\OneDrive\Accounts' -ErrorAction SilentlyContinue)) { try { $u = [string](Get-ItemProperty -LiteralPath $a.PSPath -Name UserFolder -ErrorAction Stop).UserFolder; if ($u -and (Test-Path -LiteralPath $u)) { $roots += $u } } catch {} }
if (-not $roots.Count) { Say 'OneDrive non è configurato su questo PC: non c''è niente da liberare così.'; return }
$pol = $null; try { $pol = (Get-ItemProperty -LiteralPath 'HKLM:\SOFTWARE\Policies\Microsoft\OneDrive' -Name FilesOnDemandEnabled -ErrorAction Stop).FilesOnDemandEnabled } catch {}
if ($pol -eq 0) { Say 'Su questo PC l''ufficio ha spento "File su richiesta" di OneDrive: i file non possono restare solo online. Chiedi all''assistenza (IT).'; return }
$lim = (Get-Date).AddDays(-60)
$sum = [long]0; $n = 0
foreach ($r in ($roots | Select-Object -Unique)) {
  Say ('Controllo ' + $r + '...')
  foreach ($f in @(Get-ChildItem -LiteralPath $r -Recurse -File -Force -ErrorAction SilentlyContinue)) {
    if ($n -ge 5000) { break }
    $at = [int]$f.Attributes
    if ($at -band 0x400000 -or $at -band 0x1000 -or $at -band 0x80000) { continue }  # già solo online o "sempre su questo dispositivo"
    if ($f.Length -lt 1MB -or $f.LastWriteTime -gt $lim) { continue }
    if ((Run-Hidden 'attrib.exe' ('+U -P "' + $f.FullName + '"')) -eq 0) { $sum += $f.Length; $n++ }
  }
}
if ($n) { Say ('Chiesto a OneDrive di tenere solo online ' + $n + ' file vecchi: se sono già tutti caricati, si libera circa ' + (Fmt $sum) + ' nei prossimi minuti (OneDrive lo fa in sottofondo).'); Say 'I file restano nelle cartelle con l''icona della nuvola: doppio clic e si riaprono. Per tenerne uno sempre sul PC: tasto destro > "Mantieni sempre su questo dispositivo".' }
else { Say 'Non ci sono file vecchi e grandi da mettere solo online: OneDrive occupa già poco.' }
`)},
	{ID: "of_suspicious", Name: "Controllo dei programmi sospetti", Desc: "Fa la scansione veloce dell'antivirus di Windows e cerca i programmi senza firma che partono da soli da cartelle insolite. Ti dice cosa guardare, non cancella niente da solo.", Level: 0, Minutes: 6, Timeout: 30 * time.Minute, Script: ofs(`
$odd = '\\AppData\\|\\Temp\\|\\Users\\Public\\|\\ProgramData\\[^\\]+\.exe|\\Downloads\\'
$found = @()
foreach ($s in @(Get-TassoStartup)) {
  $cmd = [string]$s.cmd; $exe = ''
  if ($cmd -match '^"([^"]+\.exe)"') { $exe = $Matches[1] } elseif ($cmd -match '^(\S+\.exe)') { $exe = $Matches[1] }
  $exe = [Environment]::ExpandEnvironmentVariables($exe)
  if ($exe -and $exe -match $odd -and (Test-Path -LiteralPath $exe)) { $sg = Get-AuthenticodeSignature -LiteralPath $exe -ErrorAction SilentlyContinue; if (-not $sg -or [string]$sg.Status -ne 'Valid') { $found += ('All''avvio "' + $s.name + '": ' + $exe) } }
}
foreach ($t in @(Get-ScheduledTask -ErrorAction SilentlyContinue | Where-Object { $_.TaskPath -notmatch '^\\Microsoft\\' -and [string]$_.State -ne 'Disabled' })) {
  foreach ($a in @($t.Actions)) {
    $exe = [Environment]::ExpandEnvironmentVariables(([string]$a.Execute).Trim('"'))
    if ($exe -and $exe -match $odd -and (Test-Path -LiteralPath $exe)) { $sg = Get-AuthenticodeSignature -LiteralPath $exe -ErrorAction SilentlyContinue; if (-not $sg -or [string]$sg.Status -ne 'Valid') { $found += ('Attività pianificata "' + $t.TaskName + '": ' + $exe) } }
  }
}
if ($found.Count) { Say ('Da controllare (programmi senza firma che partono da soli da cartelle insolite): ' + (($found | Select-Object -Unique -First 10) -join ' | ')); Say 'Non vuol dire per forza virus (anche Teams, Zoom o programmi piccoli stanno lì), ma se un nome non lo conosci cercalo su internet e, se serve, toglilo da Programmi > Disinstalla.' }
else { Say 'Nessun programma sospetto che parte da solo da cartelle insolite.' }
$mp = Get-MpComputerStatus -ErrorAction SilentlyContinue
if (-not $mp -or -not $mp.AntivirusEnabled) { Say 'L''antivirus di Windows non è attivo: probabilmente c''è un altro antivirus. Fai una scansione con quello.'; return }
if (-not $mp.RealTimeProtectionEnabled) { Say 'ATTENZIONE: la protezione in tempo reale di Windows è SPENTA. Riaccendila in Sicurezza di Windows > Protezione da virus e minacce.'; Open-User 'windowsdefender://threat' }
Say 'Aggiorno l''antivirus di Windows e faccio la scansione veloce (5-10 minuti, puoi usare il PC)...'
Update-MpSignature -ErrorAction SilentlyContinue
$t0 = Get-Date
Start-MpScan -ScanType QuickScan -ErrorAction SilentlyContinue
$th = @(Get-MpThreatDetection -ErrorAction SilentlyContinue | Where-Object { $_.InitialDetectionTime -gt $t0.AddDays(-7) })
if ($th.Count) {
  $ko = @($th | Where-Object { -not $_.ActionSuccess })
  if ($ko.Count) { Say ('ATTENZIONE: minacce trovate negli ultimi 7 giorni: ' + $th.Count + ', di cui ' + $ko.Count + ' NON ancora tolte. Apri Sicurezza di Windows > Cronologia protezione e premi "Avvia azioni".'); Open-User 'windowsdefender://history' }
  else { Say ('Minacce trovate negli ultimi 7 giorni: ' + $th.Count + '. L''antivirus le ha già bloccate: i dettagli sono in Sicurezza di Windows > Cronologia protezione.') }
} else { Say 'Scansione finita: nessuna minaccia.' }
Say 'Le pubblicità che compaiono in basso a destra di solito sono "notifiche" di un sito accettate per sbaglio: nel browser vai su Impostazioni > Privacy > Notifiche e blocca i siti che non conosci.'
`)},
}

func init() {
	insertStepAfter("fix_discord", officeSteps...)
}
