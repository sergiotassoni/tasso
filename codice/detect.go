package main

import "time"

// detectProblems è "Rileva i problemi": controlla il PC SOLO in lettura (non cambia niente)
// e restituisce i problemi trovati, ognuno collegato al pulsante Risolvi che lo sistema.
func detectProblems() {
	setMu.Lock()
	mode := settings.Mode
	setMu.Unlock()
	r := runPSArgs(ofHelpers+ofHelpers2+detectScript, map[string]string{"mode": mode}, 5*time.Minute, func(m string) { emit("onDetectProgress", m) })
	res := decodeJSON(r.JSON)
	if res == nil {
		res = map[string]interface{}{"error": "Il controllo non è riuscito: riprova tra poco."}
	}
	emit("onDetect", res)
}

const detectScript = `
# Solo lettura: qui niente viene cambiato, spostato o cancellato.
$found = New-Object System.Collections.ArrayList
function F([string]$id, [string]$sev, [string]$why) { if (-not @($found | Where-Object { $_.id -eq $id }).Count) { [void]$found.Add([pscustomobject]@{ id = $id; sev = $sev; why = $why }) } }
function C([string]$what) { Say $what }
$virt = 'PDF|XPS|OneNote|Fax|Anteprima|Send To|Invia a'

C 'Controllo le stampanti…'
try {
  $sp = Get-Service -Name Spooler -ErrorAction Stop
  if ($sp.Status -ne 'Running') { F 'fix_print' 'p' 'Il servizio di stampa di Windows è fermo: nessuna stampante può stampare.' }
  else {
    $pr = @(Get-Printer -ErrorAction SilentlyContinue)
    $off = @($pr | Where-Object { $_.Name -notmatch $virt -and ($_.WorkOffline -or [string]$_.PrinterStatus -match 'Offline') })
    if ($off.Count) { F 'of_print_offline' 'p' ('Stampante "offline": ' + (($off | Select-Object -First 3 | ForEach-Object { $_.Name }) -join ', ') + '.') }
    if ($pr.Count -and -not @($pr | Where-Object { $_.Name -match 'Microsoft Print to PDF' }).Count) { F 'of_print_pdf' 'c' 'Manca la stampante "Microsoft Print to PDF" (serve per salvare in PDF da qualsiasi programma).' }
    $stuck = @($pr | ForEach-Object { Get-PrintJob -PrinterName $_.Name -ErrorAction SilentlyContinue } | Where-Object { $_.SubmittedTime -lt (Get-Date).AddHours(-2) })
    if ($stuck.Count) { F 'of_print_offline' 'p' ('Ci sono ' + $stuck.Count + ' stampe bloccate in coda da più di 2 ore.') }
  }
} catch {}

C 'Controllo internet e rete…'
$net = $true
try {
  $net = Test-Internet
  if (-not $net) {
    if ((Test-Tcp '1.1.1.1' 443) -or (Test-Tcp '8.8.8.8' 53)) { F 'fix_net' 'p' 'Internet c''è, ma i nomi dei siti non si trovano (problema DNS).' }
    else { F 'fix_net' 'p' 'Il PC non riesce ad andare su internet.' }
  }
  $pe = Get-RegValue 'HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings' 'ProxyEnable'
  $ps = [string](Get-RegValue 'HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings' 'ProxyServer')
  if ($pe -eq 1 -and $ps) {
    $h = ($ps -split ';')[0] -replace '^\w+=', ''; $hp = $h -split ':'
    if ($hp.Count -ge 2 -and -not (Test-Tcp $hp[0] ([int]$hp[1]) 3000)) { F 'of_proxy' 'p' ('È impostato un proxy (' + $h + ') che non risponde: i siti non si aprono.') }
  }
  foreach ($a in @(Get-NetAdapter -Physical -ErrorAction SilentlyContinue | Where-Object { $_.Status -eq 'Up' })) {
    $ip = @(Get-NetIPAddress -InterfaceIndex $a.ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue)
    if (@($ip | Where-Object { $_.IPAddress -like '169.254.*' }).Count -and -not @($ip | Where-Object { $_.IPAddress -notlike '169.254.*' }).Count) {
      if ($a.MediaType -match '802\.3' -or $a.Name -match 'Ethernet') { F 'of_ethernet' 'p' ('Il cavo di rete (' + $a.Name + ') è collegato ma non riceve un indirizzo dal router.') }
      else { F 'of_wifi_conn' 'p' ('La rete ' + $a.Name + ' è collegata ma non riceve un indirizzo dal router.') }
    }
    $if = Get-NetIPInterface -InterfaceIndex $a.ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue
    if ($if -and [string]$if.Dhcp -eq 'Disabled' -and ($a.Name -match 'Wi-?Fi|WLAN' -or $a.MediaType -match 'Native 802\.11') -and -not $net) { F 'of_static_ip' 'p' 'Il Wi-Fi ha un indirizzo IP fisso: in un''altra rete non va su internet.' }
  }
} catch {}
try {
  $wi = (netsh.exe wlan show interfaces 2>$null) -join [char]10
  if ($wi -match '(?:Segnale|Signal)\s*:\s*(\d+)\s*%' -and [int]$Matches[1] -lt 40) { F 'of_net_slow' 'c' ('Il segnale del Wi-Fi è debole (' + $Matches[1] + '%): internet può essere lento o staccarsi.') }
  $since = (Get-Date).AddDays(-3)
  # Conto solo i distacchi "strani": non quelli vicini a sospensione, riavvio o spegnimento.
  $pw = @(Get-WinEvent -FilterHashtable @{ LogName = 'System'; Id = 42, 107, 1074, 6006, 6008; StartTime = $since } -MaxEvents 500 -ErrorAction SilentlyContinue | ForEach-Object { $_.TimeCreated })
  $drops = @(Get-WinEvent -FilterHashtable @{ LogName = 'Microsoft-Windows-WLAN-AutoConfig/Operational'; Id = 8003; StartTime = $since } -MaxEvents 200 -ErrorAction SilentlyContinue | Where-Object { $t = $_.TimeCreated; -not @($pw | Where-Object { [math]::Abs(($_ - $t).TotalMinutes) -lt 3 }).Count }).Count
  if ($drops -ge 15) { F 'of_wifi_drop' 'p' ('Il Wi-Fi si è staccato ' + $drops + ' volte negli ultimi 3 giorni.') }
} catch {}
try {
  [void][Windows.Networking.Connectivity.NetworkInformation, Windows.Networking.Connectivity, ContentType = WindowsRuntime]
  $cp = [Windows.Networking.Connectivity.NetworkInformation]::GetInternetConnectionProfile()
  if ($cp -and [string]$cp.GetConnectionCost().NetworkCostType -match 'Fixed|Variable') { F 'of_metered' 'p' ('La rete "' + $cp.ProfileName + '" è segnata "a consumo": aggiornamenti e OneDrive restano fermi.') }
} catch {}
try {
  foreach ($m in @(Get-ChildItem -LiteralPath 'HKCU:\Network' -ErrorAction SilentlyContinue)) {
    $rp = [string](Get-ItemProperty -LiteralPath $m.PSPath -Name RemotePath -ErrorAction SilentlyContinue).RemotePath
    if ($rp -match '^\\\\([^\\]+)\\' -and $net) { if (-not (Test-Tcp $Matches[1] 445 2500)) { F 'of_netdrive' 'p' ('L''unità di rete ' + $m.PSChildName.ToUpper() + ': (' + $rp + ') non risponde.') } }
  }
} catch {}
try {
  $hf = Join-Path $env:SystemRoot 'System32\drivers\etc\hosts'
  $imp = '(^|\.)(microsoft|windows|windowsupdate|office|office365|live|outlook|msn|bing|skype|sharepoint|onedrive|google|gmail|youtube|apple|icloud|amazon|paypal|zoom|webex|adobe|aruba|poste|intesasanpaolo|unicredit|bper|bancobpm|fineco|inps|agenziaentrate|kaspersky|eset|avast|avg|norton|mcafee|bitdefender|malwarebytes|virustotal)\.(com|it|net|org|eu)$|(^|\.)gov\.it$'
  $priv = '^(10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.)'
  $bad = @(Get-Content -LiteralPath $hf -ErrorAction SilentlyContinue | ForEach-Object { ($_ -split '#', 2)[0].Trim() } | Where-Object { $_ } | Where-Object { $p = $_ -split '\s+'; $blk = $p[0] -match '^(0\.0\.0\.0|127\.\d+\.\d+\.\d+|::1?)$'; ($p[0] -notmatch $priv) -and @($p | Select-Object -Skip 1 | Where-Object { $_ -match $imp -and (-not $blk -or ($_ -split '\.').Count -le 2 -or $_ -match '^(www|login|update|accounts|secure)\.') }).Count })
  if ($bad.Count) { F 'of_hosts' 'p' ('Il file "hosts" blocca o devia ' + $bad.Count + ' siti importanti (può essere un virus o un vecchio programma).') }
} catch {}

C 'Controllo l''ora del PC…'
try {
  if ($net) {
    $rq = [Net.WebRequest]::Create('http://www.msftconnecttest.com/connecttest.txt'); $rq.Method = 'HEAD'; $rq.Timeout = 6000
    $rs = $rq.GetResponse(); $d = [datetime]::ParseExact([string]$rs.Headers['Date'], 'r', [Globalization.CultureInfo]::InvariantCulture); $rs.Close()
    $diff = [math]::Abs(((Get-Date).ToUniversalTime() - $d).TotalMinutes)
    if ($diff -gt 3) { F 'of_time' 'p' ('L''orologio del PC è sbagliato di circa ' + [int]$diff + ' minuti: alcuni siti e Office danno errore.') }
  }
} catch {}

C 'Controllo Word, Excel e Outlook…'
try {
  $u = Get-RegValue 'HKLM\SOFTWARE\Microsoft\Office\ClickToRun\Configuration' 'UpdatesEnabled'
  if ([string]$u -eq 'False') { F 'of_office_update' 'c' 'Gli aggiornamenti automatici di Office sono spenti.' }
  $since = (Get-Date).AddDays(-7)
  $cr = @(Get-WinEvent -FilterHashtable @{ LogName = 'Application'; Id = 1000; StartTime = $since } -MaxEvents 300 -ErrorAction SilentlyContinue | Where-Object { $_.Message -match '(WINWORD|EXCEL|POWERPNT|OUTLOOK|ms-teams|Teams|AcroRd32|Acrobat)\.exe' })
  $byApp = @{}; foreach ($e in $cr) { if ($e.Message -match '(WINWORD|EXCEL|POWERPNT|OUTLOOK|ms-teams|Teams|AcroRd32|Acrobat)\.exe') { $byApp[$Matches[1].ToUpper()]++ } }
  foreach ($k in $byApp.Keys) {
    if ($byApp[$k] -lt 2) { continue }
    $n = @{ WINWORD = 'Word'; EXCEL = 'Excel'; POWERPNT = 'PowerPoint'; OUTLOOK = 'Outlook'; 'MS-TEAMS' = 'Teams'; TEAMS = 'Teams'; ACRORD32 = 'Adobe Reader'; ACROBAT = 'Adobe Acrobat' }[$k]
    $id = @{ WINWORD = 'of_office_crash'; EXCEL = 'of_office_crash'; POWERPNT = 'of_office_crash'; OUTLOOK = 'of_outlook_open'; 'MS-TEAMS' = 'of_teams'; TEAMS = 'of_teams'; ACRORD32 = 'of_adobe'; ACROBAT = 'of_adobe' }[$k]
    F $id 'p' ($n + ' si è chiuso da solo ' + $byApp[$k] + ' volte nell''ultima settimana.')
  }
  $big = @(Get-ChildItem -LiteralPath (Join-Path $env:LOCALAPPDATA 'Microsoft\Outlook') -Include '*.ost', '*.pst' -Recurse -File -ErrorAction SilentlyContinue | Where-Object { $_.Length -gt 40GB })
  if ($big.Count) { F 'of_outlook_size' 'p' ('La casella di Outlook occupa ' + (Fmt ($big | Measure-Object Length -Maximum).Maximum) + ': oltre i 40-50 GB Outlook diventa lento o si blocca.') }
  $us = @(Get-ChildItem -LiteralPath (Join-Path $env:LOCALAPPDATA 'Microsoft\Office\UnsavedFiles') -File -ErrorAction SilentlyContinue | Where-Object { $_.LastWriteTime -gt (Get-Date).AddDays(-4) })
  if ($us.Count) { F 'of_office_lost' 'c' ('Ci sono ' + $us.Count + ' documenti di Office non salvati che si possono ancora recuperare.') }
} catch {}

C 'Controllo microfono, webcam, audio e notifiche…'
try {
  $cm = 'HKCU\Software\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\'
  $cl = 'HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\'
  if ([string](Get-RegValue ($cm + 'microphone') 'Value') -eq 'Deny' -or [string](Get-RegValue ($cl + 'microphone') 'Value') -eq 'Deny') { F 'of_meet_mic' 'p' 'Il microfono è bloccato nelle impostazioni della privacy: in riunione non ti sentono.' }
  if ([string](Get-RegValue ($cm + 'webcam') 'Value') -eq 'Deny' -or [string](Get-RegValue ($cl + 'webcam') 'Value') -eq 'Deny') { F 'of_meet_cam' 'p' 'La webcam è bloccata nelle impostazioni della privacy.' }
  if (@(Get-Service -Name Audiosrv, AudioEndpointBuilder -ErrorAction SilentlyContinue | Where-Object { $_.Status -ne 'Running' }).Count) { F 'fix_audio' 'p' 'Il servizio audio di Windows è fermo: niente suono.' }
  # In modalità Gaming, FPS o Film le notifiche le mette in pausa Tasso apposta: non è un problema.
  if (@('gaming','fps','film') -notcontains [string]$A.mode -and (Get-RegValue 'HKCU\Software\Microsoft\Windows\CurrentVersion\PushNotifications' 'ToastEnabled') -eq 0) { F 'of_notif' 'p' 'Le notifiche di Windows sono spente: Teams e Outlook non ti avvisano.' }
} catch {}

C 'Controllo le periferiche…'
try {
  $map = @{ USB = 'of_usb'; HIDClass = 'of_usb'; Keyboard = 'of_usb'; Mouse = 'of_usb'; DiskDrive = 'of_usb'; Camera = 'of_meet_cam'; Image = 'of_meet_cam'; Bluetooth = 'of_meet_bt'; Printer = 'of_print_usb'; PrintQueue = 'of_print_usb'; Net = 'fix_net'; MEDIA = 'fix_audio'; AudioEndpoint = 'fix_audio'; Monitor = 'of_second_screen'; Display = 'of_second_screen' }
  foreach ($d in @(Get-CimInstance Win32_PnPEntity -ErrorAction SilentlyContinue | Where-Object { $_.ConfigManagerErrorCode -and $_.ConfigManagerErrorCode -notin 22, 24, 45 })) {
    $id = $map[[string]$d.PNPClass]
    if ($id) { F $id 'p' ('"' + $d.Name + '" ha un problema (codice ' + $d.ConfigManagerErrorCode + ').') }
  }
} catch {}

C 'Controllo tastiera, mouse e touchpad…'
try {
  if (([int](Get-RegValue 'HKCU\Control Panel\Accessibility\Keyboard Response' 'Flags') -band 1) -or ([int](Get-RegValue 'HKCU\Control Panel\Accessibility\StickyKeys' 'Flags') -band 1)) { F 'of_sticky' 'p' 'Sono accesi "Filtro tasti" o "Tasti permanenti": la tastiera fa bip o scrive lenta.' }
  $sm = Get-RegValue 'HKLM\SYSTEM\CurrentControlSet\Control\Keyboard Layout' 'Scancode Map'
  $nk = Get-RegValue 'HKCU\Software\Microsoft\Windows\CurrentVersion\Policies\Explorer' 'NoWinKeys'
  if ($nk -eq 1 -or ($sm -and ((($sm | ForEach-Object { '{0:x2}' -f $_ }) -join '') -match '00005be0|00005ce0'))) { F 'of_win_key' 'p' 'Il tasto Windows è stato spento da un programma.' }
  $ms = [int](Get-RegValue 'HKCU\Control Panel\Mouse' 'MouseSensitivity'); $dc = [int](Get-RegValue 'HKCU\Control Panel\Mouse' 'DoubleClickSpeed')
  if (($ms -and ($ms -lt 3 -or $ms -gt 18)) -or ($dc -and ($dc -lt 200 -or $dc -gt 900))) { F 'of_mouse' 'c' 'Velocità del mouse o del doppio clic molto diverse dal normale.' }
  if ((Get-RegValue 'HKCU\Software\Microsoft\Windows\CurrentVersion\PrecisionTouchPad\Status' 'Enabled') -eq 0) { F 'of_touchpad' 'p' 'Il touchpad è spento nelle impostazioni.' }
} catch {}

C 'Controllo Esplora file, desktop e file…'
try {
  $adv = 'HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced'
  if ((Get-RegValue $adv 'HideIcons') -eq 1) { F 'of_desktop_icons' 'p' 'Le icone del desktop sono nascoste.' }
  if ((Get-RegValue $adv 'IconsOnly') -eq 1) { F 'of_thumbs' 'c' 'Le anteprime di foto e documenti sono spente.' }
  if ((Get-RegValue $adv 'HideFileExt') -ne 0) { F 'of_ext' 'c' 'Non si vede la fine dei nomi dei file (.pdf, .exe): è più facile cadere nei falsi allegati.' }
  $dk = [Environment]::GetFolderPath('Desktop'); $sh = New-Object -ComObject WScript.Shell; $dead = @()
  foreach ($l in @(Get-ChildItem -LiteralPath $dk -Filter '*.lnk' -File -ErrorAction SilentlyContinue | Select-Object -First 80)) {
    try { $t = $sh.CreateShortcut($l.FullName).TargetPath; if ($t -match '^[A-Za-z]:\\' -and (Test-Path -LiteralPath $t.Substring(0, 3)) -and ([int](Get-CimInstance Win32_LogicalDisk -Filter ("DeviceID='" + $t.Substring(0, 2) + "'") -ErrorAction SilentlyContinue).DriveType -eq 3) -and -not (Test-Path -LiteralPath $t)) { $dead += $l.BaseName } } catch {}
  }
  if ($dead.Count) { F 'of_shortcuts' 'p' ('Icone del desktop che non aprono più niente: ' + (($dead | Select-Object -First 4) -join ', ') + '.') }
  $sys = Get-PSDrive -Name ($env:SystemDrive.TrimEnd(':')) -ErrorAction SilentlyContinue
  if ($sys -and ($sys.Used + $sys.Free) -gt 0) {
    $pct = [math]::Round(100 * $sys.Free / ($sys.Used + $sys.Free))
    if ($sys.Free -lt 10GB -or $pct -lt 8) { F 'of_disk_full' 'p' ('Il disco C: è quasi pieno: liberi ' + (Fmt $sys.Free) + ' (' + $pct + '%).') }
  }
  foreach ($x in @(@('.zip', 'of_zip', 'I file .zip'), @('.pdf', 'of_open_with', 'I PDF'), @('.docx', 'of_open_with', 'I documenti Word'), @('.xlsx', 'of_open_with', 'I file Excel'))) {
    $p = Get-UserChoice $x[0]
    if ($p -and -not (Test-ProgIdOk $p)) { F $x[1] 'p' ($x[2] + ' si aprono con un programma che non c''è più.') }
  }
  foreach ($x in @('http', 'https')) { $p = Get-UserChoice $x; if ($p -and -not (Test-ProgIdOk $p)) { F 'of_links' 'p' 'I link si aprono con un browser che non c''è più: cliccando non succede niente.' } }
} catch {}

C 'Controllo OneDrive…'
try {
  $roots = @(Get-OneDriveRoots)
  if ($roots.Count -and -not (Get-Process -Name OneDrive -ErrorAction SilentlyContinue)) { F 'of_onedrive' 'p' 'OneDrive è collegato ma non è acceso: i file non si sincronizzano.' }
  foreach ($r in $roots) {
    $rx = '-' + [regex]::Escape($env:COMPUTERNAME) + '(-\d+)?$'
    $dup = Get-ChildItem -LiteralPath $r -Recurse -Depth 6 -File -Filter ('*-' + $env:COMPUTERNAME + '*') -ErrorAction SilentlyContinue | Where-Object { $_.BaseName -match $rx } | Select-Object -First 1
    if ($dup) { F 'of_onedrive_conflict' 'p' ('In OneDrive ci sono file doppi con il nome del PC (es. ' + $dup.Name + ').'); break }
  }
} catch {}

C 'Controllo avvio, servizi e licenza di Windows…'
try {
  $ev = Get-WinEvent -FilterHashtable @{ LogName = 'Microsoft-Windows-Diagnostics-Performance/Operational'; Id = 100 } -MaxEvents 1 -ErrorAction SilentlyContinue
  if ($ev) { $bt = ([xml]$ev.ToXml()).Event.EventData.Data | Where-Object { $_.Name -eq 'BootTime' } | Select-Object -First 1; if ($bt -and [int]$bt.'#text' -gt 120000) { F 'of_slow_boot' 'p' ('L''ultima accensione è durata ' + [int]([int]$bt.'#text' / 1000) + ' secondi (normale: meno di 60).') } }
  foreach ($s in @(@('wuauserv', 'fix_wu', 'Windows Update'), @('WSearch', 'fix_search', 'la ricerca di Windows'))) {
    $sv = Get-Service -Name $s[0] -ErrorAction SilentlyContinue
    if ($sv -and [string]$sv.StartType -eq 'Disabled') { F $s[1] 'p' ('Il servizio di ' + $s[2] + ' è disattivato.') }
  }
  $lic = @(Get-CimInstance SoftwareLicensingProduct -Filter "ApplicationID='55c92734-d682-4d71-983e-d6ec3f16059f' AND PartialProductKey IS NOT NULL AND LicenseDependsOn IS NULL" -ErrorAction SilentlyContinue)
  if ($lic.Count -and -not @($lic | Where-Object { $_.LicenseStatus -eq 1 }).Count) { F 'of_activation' 'p' 'Windows non risulta attivato.' }
  if ($env:USERPROFILE -match '\\TEMP(\.[^\\]*)?$') { F 'of_temp_profile' 'p' 'Sei entrato con un profilo temporaneo: quello che salvi sparisce quando esci.' }
  $lu = Get-LocalUser -Name $env:USERNAME -ErrorAction SilentlyContinue
  if ($lu -and $lu.PasswordExpires -and $lu.PasswordExpires -lt (Get-Date).AddDays(14)) { F 'of_pwd_expire' 'p' ('La password di Windows scade il ' + $lu.PasswordExpires.ToString('dd/MM/yyyy') + '.') }
  $v = Get-PowerValue 'SUB_VIDEO' 'VIDEOIDLE'
  if ($v.Count -eq 2 -and $v[0] -gt 0 -and $v[0] -lt 120) { F 'of_screen_off' 'c' ('Con la corrente lo schermo si spegne dopo solo ' + $v[0] + ' secondi.') }
  $loc = [string](Get-RegValue 'HKCU\Control Panel\International' 'LocaleName')
  if ($loc -eq 'it-IT' -and [string](Get-RegValue 'HKCU\Control Panel\International' 'sDecimal') -ne ',') { F 'of_regional' 'p' 'I numeri usano il punto invece della virgola: Excel sbaglia decimali e CSV.' }
} catch {}

C 'Controllo programmi bloccati, memoria e batteria…'
try {
  # Le app di Windows sospese (Impostazioni, Store...) risultano 'non risponde' anche se vanno: le salto, e ricontrollo dopo 4 secondi.
  $hskip = '^(explorer|Tasso|dwm|csrss|winlogon|ShellExperienceHost|StartMenuExperienceHost|SearchHost|SearchApp|TextInputHost|ApplicationFrameHost|SystemSettings|LockApp|Widgets)$'
  $hung = @(Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.MainWindowHandle -ne 0 -and -not $_.Responding -and $_.ProcessName -notmatch $hskip -and "$($_.Path)" -notmatch '\\(WindowsApps|SystemApps)\\' })
  if ($hung.Count) { Start-Sleep -Seconds 4; $hung = @(Get-Process -Id ($hung | ForEach-Object { $_.Id }) -ErrorAction SilentlyContinue | Where-Object { -not $_.Responding } | Select-Object -First 3) }
  if ($hung.Count) { F 'of_hung' 'p' ('Non risponde: ' + (($hung | ForEach-Object { $_.ProcessName }) -join ', ') + '.') }
  $os = Get-CimInstance Win32_OperatingSystem
  $mem = [int](100 - 100 * $os.FreePhysicalMemory / $os.TotalVisibleMemorySize)
  $c1 = (Get-CimInstance Win32_Processor | Measure-Object LoadPercentage -Average).Average; Start-Sleep -Seconds 2; $c2 = (Get-CimInstance Win32_Processor | Measure-Object LoadPercentage -Average).Average
  if ([math]::Min($c1, $c2) -gt 90 -or $mem -gt 92) { F 'of_slow_now' 'p' ('Il PC è sotto sforzo adesso (processore ' + [int]$c2 + '%, memoria ' + $mem + '%).') }
  if (Get-CimInstance Win32_Battery -ErrorAction SilentlyContinue) {
    $fc = (Get-CimInstance -Namespace root\wmi -ClassName BatteryFullChargedCapacity -ErrorAction SilentlyContinue | Select-Object -First 1).FullChargedCapacity
    $dc = (Get-CimInstance -Namespace root\wmi -ClassName BatteryStaticData -ErrorAction SilentlyContinue | Select-Object -First 1).DesignedCapacity
    if ($fc -and $dc -and ($fc / $dc) -lt 0.6) { F 'of_battery' 'p' ('La batteria tiene solo il ' + [int](100 * $fc / $dc) + '% della carica che aveva da nuova.') }
  }
} catch {}

C 'Controllo le impostazioni per i giochi…'
try {
  if ((Get-RegValue 'HKCU\Control Panel\Desktop' 'ForegroundLockTimeout') -eq 0) { F 'fix_game_minimize' 'p' 'Un programma ha tolto la protezione che impedisce alle finestre di "rubare" il gioco: può ridursi a icona da solo.' }
  if ([string](Get-RegValue 'HKCU\Control Panel\Mouse' 'MouseSpeed') -eq '1') { F 'fix_game_mouse' 'c' 'L''accelerazione del mouse è accesa: la mira nei giochi è meno precisa.' }
  if ((Get-RegValue 'HKCU\Software\Microsoft\Windows\CurrentVersion\GameDVR' 'AppCaptureEnabled') -eq 1) { F 'fix_game_lag' 'c' 'La registrazione in background di Xbox è accesa: toglie qualche FPS.' }
  $st = [string](Get-RegValue 'HKCU\Software\Valve\Steam' 'SteamPath')
  if ($st) { $cfg = Join-Path ($st -replace '/', '\') 'config\config.vdf'; if ((Get-Content -LiteralPath $cfg -Raw -ErrorAction SilentlyContinue) -match '"DownloadThrottleKbps"\s+"([1-9]\d*)"') { F 'fix_game_download' 'c' 'Steam ha un limite alla velocità dei download.' } }
} catch {}

Write-Output ('JSON ' + (@{ found = @($found); checked = 58; games = 4 } | ConvertTo-Json -Depth 4 -Compress))
`
