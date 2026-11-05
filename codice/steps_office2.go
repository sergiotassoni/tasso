package main

// Modalità Ufficio, seconda parte (2.6): altri 50 problemi dei PC da lavoro, così in Risolvi
// problemi diventano 100. Stesse regole della prima parte: niente documenti cancellati, ogni
// impostazione cambiata ha il suo file per tornare indietro, Office mai chiuso se ci stai
// lavorando, e dove Windows non lascia fare (o la scelta è tua) Tasso controlla e spiega.

import "time"

// ofHelpers2: funzioni in più per questi script (si aggiungono a ofHelpers).
const ofHelpers2 = `
function Get-FullKey([string]$key) {
  return ($key -replace '^HKCU\\', 'HKEY_CURRENT_USER\' -replace '^HKLM\\', 'HKEY_LOCAL_MACHINE\' -replace '^HKU\\', 'HKEY_USERS\')
}
function New-UndoFile([string[]]$body, [string]$label) {
  # File .reg per tornare come prima: basta un doppio clic.
  $d = Join-Path $env:LOCALAPPDATA 'Tasso\backup-registro'
  New-Item -ItemType Directory -Path $d -Force | Out-Null
  $f = Join-Path $d ($label + '_' + (Get-Date -Format 'yyyyMMdd_HHmmss') + '.reg')
  try { [IO.File]::WriteAllLines($f, [string[]](@('Windows Registry Editor Version 5.00', '') + $body + @('')), [Text.Encoding]::Unicode); return $f } catch { return '' }
}
function Save-Undo([string]$key, [string[]]$names, [string]$label) {
  # Salva i valori di prima (anche "non c'era": allora il file lo cancella).
  $full = Get-FullKey $key
  $it = Get-Item -LiteralPath ('Registry::' + $full) -ErrorAction SilentlyContinue
  $have = @(); if ($it) { $have = @($it.GetValueNames()) }
  $lines = @('[' + $full + ']')
  foreach ($n in $names) {
    if ($n) { $q = '"' + ($n -replace '\\', '\\' -replace '"', '\"') + '"' } else { $q = '@' }
    if ($have -notcontains $n) { $lines += ($q + '=-'); continue }
    $kind = [string]$it.GetValueKind($n); $val = $it.GetValue($n, $null, 'DoNotExpandEnvironmentNames')
    if ($kind -eq 'DWord') { $lines += ($q + '=dword:' + ('{0:x8}' -f [BitConverter]::ToUInt32([BitConverter]::GetBytes([int]$val), 0))) }
    elseif ($kind -eq 'String') { $lines += ($q + '="' + (([string]$val) -replace '\\', '\\' -replace '"', '\"') + '"') }
    elseif ($kind -eq 'Binary') { $lines += ($q + '=hex:' + ((@($val) | ForEach-Object { '{0:x2}' -f $_ }) -join ',')) }
    else { return '' }
  }
  return (New-UndoFile $lines $label)
}
function Set-RegValue([string]$key, [string]$name, $value, [string]$type) {
  $p = 'Registry::' + (Get-FullKey $key)
  if (-not (Test-Path -LiteralPath $p)) { New-Item -Path $p -Force | Out-Null }
  Set-ItemProperty -LiteralPath $p -Name $name -Value $value -Type $type
}
function Get-RegValue([string]$key, [string]$name) {
  try { return (Get-ItemProperty -LiteralPath ('Registry::' + (Get-FullKey $key)) -Name $name -ErrorAction Stop).$name } catch { return $null }
}
function Say-Undo([string]$f) {
  if ($f) { Say ('Per tornare come prima: doppio clic su ' + $f) }
}
function Get-PowerValue([string]$sub, [string]$set) {
  # Valori attuali (corrente, batteria) di un'impostazione del piano di alimentazione.
  $o = @(powercfg.exe /q SCHEME_CURRENT $sub $set 2>$null)
  $h = @($o | ForEach-Object { if ($_ -match ':\s*0x([0-9a-fA-F]+)\s*$') { [Convert]::ToInt64($Matches[1], 16) } })
  if ($h.Count -ge 2) { return @($h[$h.Count - 2], $h[$h.Count - 1]) }
  return @()
}
function Get-OneDriveRoots {
  $roots = @()
  foreach ($a in @(Get-ChildItem -LiteralPath 'HKCU:\Software\Microsoft\OneDrive\Accounts' -ErrorAction SilentlyContinue)) { try { $u = [string](Get-ItemProperty -LiteralPath $a.PSPath -Name UserFolder -ErrorAction Stop).UserFolder; if ($u -and (Test-Path -LiteralPath $u)) { $roots += $u } } catch {} }
  return @($roots | Select-Object -Unique)
}
function Get-LocalSize([string]$p) {
  # Solo lo spazio vero sul disco: i file di OneDrive "solo online" non contano.
  if (-not (Test-Path -LiteralPath $p)) { return [long]0 }
  $s = (Get-ChildItem -LiteralPath $p -Recurse -Force -File -ErrorAction SilentlyContinue | Where-Object { -not ([int]$_.Attributes -band 0x401000) } | Measure-Object -Property Length -Sum).Sum
  if ($s) { return [long]$s } else { return [long]0 }
}
function Get-AssocName([string]$progId) {
  $known = @{ 'AcroExch' = 'Adobe Acrobat'; 'Acrobat' = 'Adobe Acrobat'; 'MSEdge' = 'Microsoft Edge'; 'AppXd4nrz8ff68srnhf9t5a8sbjyar1cr723' = 'Microsoft Edge'; 'ChromeHTML' = 'Google Chrome'; 'ChromePDF' = 'Google Chrome'; 'Firefox' = 'Firefox'; 'BraveHTML' = 'Brave'; 'Opera' = 'Opera'; 'Word.' = 'Word'; 'Excel.' = 'Excel'; 'PowerPoint.' = 'PowerPoint'; 'Outlook.' = 'Outlook'; 'CompressedFolder' = 'Windows (cartelle compresse)'; 'WinRAR' = 'WinRAR'; '7-Zip' = '7-Zip'; 'Applications\' = 'un programma scelto a mano' }
  foreach ($k in $known.Keys) { if ($progId.StartsWith($k)) { return $known[$k] } }
  try { $n = [string](Get-ItemProperty -LiteralPath ('Registry::HKEY_CLASSES_ROOT\' + $progId) -ErrorAction Stop).'(default)'; if ($n) { return $n } } catch {}
  return $progId
}
function Get-UserChoice([string]$ext) {
  $k = 'HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\' + $ext
  if ($ext -notmatch '^\.') { $k = 'HKCU\Software\Microsoft\Windows\Shell\Associations\UrlAssociations\' + $ext }
  $p = [string](Get-RegValue ($k + '\UserChoiceLatest\ProgId') 'ProgId')
  if (-not $p) { $p = [string](Get-RegValue ($k + '\UserChoiceLatest') 'ProgId') }
  if (-not $p) { $p = [string](Get-RegValue ($k + '\UserChoice') 'ProgId') }
  return $p
}
function Test-ProgIdOk([string]$progId) {
  # Il programma a cui punta l'associazione esiste ancora?
  if (-not $progId) { return $false }
  $cmd = ''
  try { $cmd = [string](Get-ItemProperty -LiteralPath ('Registry::HKEY_CLASSES_ROOT\' + $progId + '\shell\open\command') -ErrorAction Stop).'(default)' } catch {}
  if (-not $cmd) { return (Test-Path -LiteralPath ('Registry::HKEY_CLASSES_ROOT\' + $progId)) }
  $exe = ''
  if ($cmd -match '^\s*"([^"]+)"') { $exe = $Matches[1] } elseif ($cmd -match '^\s*(\S+)') { $exe = $Matches[1] }
  $exe = [Environment]::ExpandEnvironmentVariables($exe)
  if ($exe -match '^[A-Za-z]:\\') { return (Test-Path -LiteralPath $exe) }
  return $true
}
function Use-Input {
  # Tasti permanenti, filtro tasti e mouse: si cambiano subito, senza uscire da Windows.
  if ('TassoInput' -as [type]) { return $true }
  try {
    Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class TassoInput {
  [StructLayout(LayoutKind.Sequential)] struct SK { public uint cb; public uint flags; }
  [StructLayout(LayoutKind.Sequential)] struct FK { public uint cb; public uint flags; public uint wait; public uint delay; public uint repeat; public uint bounce; }
  [DllImport("user32.dll", EntryPoint = "SystemParametersInfoW")] static extern bool SpiS(uint a, uint b, ref SK c, uint d);
  [DllImport("user32.dll", EntryPoint = "SystemParametersInfoW")] static extern bool SpiF(uint a, uint b, ref FK c, uint d);
  [DllImport("user32.dll", EntryPoint = "SystemParametersInfoW")] static extern bool SpiP(uint a, uint b, IntPtr c, uint d);
  [DllImport("user32.dll", EntryPoint = "SystemParametersInfoW")] static extern bool SpiA(uint a, uint b, int[] c, uint d);
  static string Off(uint get, uint set) {
    SK s = new SK(); s.cb = 8;
    if (!SpiS(get, 8, ref s, 0)) return "";
    string r = ((s.flags & 1u) != 0) ? "on" : "";
    uint nf = s.flags & ~5u;
    if (nf != s.flags) { s.flags = nf; SpiS(set, 8, ref s, 3); if (r == "") r = "hk"; }
    return r;
  }
  public static string Keys() {
    string r = "sticky=" + Off(0x3A, 0x3B) + ";toggle=" + Off(0x34, 0x35) + ";filter=";
    FK f = new FK(); f.cb = 24;
    if (SpiF(0x32, 24, ref f, 0)) {
      string x = ((f.flags & 1u) != 0) ? "on" : "";
      uint nf = f.flags & ~5u;
      if (nf != f.flags) { f.flags = nf; SpiF(0x33, 24, ref f, 3); if (x == "") x = "hk"; }
      r += x;
    }
    return r;
  }
  public static void Accel(bool on) { SpiA(0x04, 0, on ? new int[] { 6, 10, 1 } : new int[] { 0, 0, 0 }, 3); }
  public static void Mouse() {
    SpiP(0x71, 0, (IntPtr)10, 3);
    SpiP(0x20, 500, IntPtr.Zero, 3);
    SpiA(0x04, 0, new int[] { 6, 10, 1 }, 3);
    SpiP(0x69, 3, IntPtr.Zero, 3);
  }
}
'@ -ErrorAction Stop
    return $true
  } catch { return $false }
}
`

func ofs2(s string) string { return ofHelpers + ofHelpers2 + s }

var officeSteps2 = []Step{
	// ---------- Word, Excel e Office ----------
	{ID: "of_office_gpu", Name: "Word o Excel lenti, sfarfallano o restano bianchi", Desc: "Spegne l'accelerazione grafica di Office, la causa più comune di finestre bianche, sfarfallii e scatti mentre scorri. Puoi tornare indietro.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
if (-not (Test-Path -LiteralPath 'HKCU:\Software\Microsoft\Office\16.0')) { Say 'Non trovo Office 2016, 2019, 2021 o Microsoft 365 su questo PC.'; return }
if (Stop-IfOfficeOpen @('WINWORD', 'EXCEL', 'POWERPNT', 'OUTLOOK')) { return }
$k = 'HKCU\Software\Microsoft\Office\16.0\Common\Graphics'
if ((Get-RegValue $k 'DisableHardwareAcceleration') -eq 1) { Say 'L''accelerazione grafica di Office era già spenta.'; Say 'Altre cause frequenti: driver della scheda video vecchio (in Tasso: Aggiornamento driver) e componenti aggiuntivi (usa "Word o Excel si chiudono appena li apri").'; return }
$u = Save-Undo $k @('DisableHardwareAcceleration') 'office-grafica'
Set-RegValue $k 'DisableHardwareAcceleration' 1 'DWord'
Say 'Fatto: Office ora disegna le finestre senza la scheda video. Riapri Word ed Excel: finestre bianche e sfarfallii di solito spariscono.'
Say-Undo $u
`)},
	{ID: "of_office_cache", Name: "Word o Excel non salvano su OneDrive o SharePoint", Desc: "Mette da parte la memoria dei documenti di Office (si rovina e blocca i salvataggi online, \"Caricamento in sospeso\"). Office ne crea una nuova. Non cancella niente.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs2(`
if (Stop-IfOfficeOpen) { return }
$c = Join-Path $env:LOCALAPPDATA 'Microsoft\Office\16.0\OfficeFileCache'
if (-not (Test-Path -LiteralPath $c)) { Say 'La memoria dei documenti di Office è già vuota. Se Word non salva online: controlla internet e che OneDrive sia acceso (nuvoletta vicino all''orologio).'; return }
# Le modifiche "in attesa di caricamento" esistono solo qui: la prima volta avviso e mi fermo,
# la seconda volta (entro un'ora) procedo.
$mark = Join-Path $env:TEMP 'tasso_officecache_ok.txt'
$ok2 = (Test-Path -LiteralPath $mark) -and ((Get-Item -LiteralPath $mark).LastWriteTime -gt (Get-Date).AddHours(-1))
if (-not $ok2) {
  Set-Content -LiteralPath $mark -Value 'ok' -ErrorAction SilentlyContinue
  Say 'PRIMA DI CONTINUARE: se in Word o Excel qualche documento dice "Caricamento in sospeso" o "In attesa di caricamento", aprilo e fai File > Salva una copia sul PC (per esempio sul Desktop). Quelle modifiche non sono ancora su OneDrive e stanno solo in questa memoria.'
  Say 'Fatto questo, chiudi Word ed Excel e premi di nuovo Risolvi entro un''ora: allora metto da parte la memoria.'
  return
}
Remove-Item -LiteralPath $mark -Force -ErrorAction SilentlyContinue
Get-Process -Name MSOSYNC, MSOUC, CSISYNCCLIENT -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1
$old = $c + '_vecchia_' + (Get-Date -Format 'yyyyMMdd_HHmm')
try { Rename-Item -LiteralPath $c -NewName (Split-Path $old -Leaf) -ErrorAction Stop } catch { Say 'Un programma di Office la sta ancora usando: riavvia il PC e premi di nuovo Risolvi.'; return }
Say 'Fatto: la memoria dei documenti di Office è stata messa da parte e Office ne crea una nuova pulita.'
Say 'Riapri il file da OneDrive o SharePoint e salva di nuovo. Se un file era "in attesa di caricamento", riaprilo dal sito di OneDrive/SharePoint e guarda la "Cronologia versioni".'
Say ('La memoria vecchia è stata spostata in ' + $old + ': quando tutto funziona da qualche settimana si può cancellare.')
`)},
	{ID: "of_office_protected", Name: "Il file scaricato si apre in \"Visualizzazione protetta\"", Desc: "Ti spiega perché Word ed Excel aprono i file arrivati da internet in sola lettura e come sbloccare solo quelli di cui ti fidi. La protezione non la spegne: serve contro i virus nelle email. Controlla e spiega.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$dl = Get-DownloadsPath
$f = @(Get-ChildItem -LiteralPath $dl -File -ErrorAction SilentlyContinue | Where-Object { $_.Extension -match '^\.(docx?|xlsx?|xlsm|pptx?|rtf|csv)$' } | Where-Object { Get-Item -LiteralPath $_.FullName -Stream Zone.Identifier -ErrorAction SilentlyContinue })
Say 'Word ed Excel aprono in "Visualizzazione protetta" (sola lettura, barra gialla) i file che arrivano da internet o dalle email. È una protezione: un file con le macro può essere un virus.'
if ($f.Count) { Say ('Nella cartella Download ci sono ' + $f.Count + ' documenti di Office segnati "da internet".') }
Say 'Se il file è tuo o di un collega di cui ti fidi: premi "Abilita modifica" sulla barra gialla. Vale solo per quel file.'
Say 'Per non vederla più su un file: tasto destro sul file > Proprietà > in basso spunta "Sblocca" > OK.'
Say 'Se la barra è ROSSA o parla di macro bloccate: non sbloccare se non sei sicuro di chi l''ha mandato. Chiedi al mittente per telefono.'
Say 'Tasso non spegne questa protezione per tutti i file: è una delle difese più utili contro i virus nelle email.'
`)},
	{ID: "of_excel_locked", Name: "Il file è \"bloccato da un altro utente\"", Desc: "Chiude le copie di Word, Excel e PowerPoint rimaste aperte in sottofondo senza finestra (spesso l'\"altro utente\" sei tu) e ti spiega cosa fare se il file è aperto davvero da un collega.", Level: 0, Minutes: 0.2, Timeout: time.Minute, Script: ofs2(`
$ghost = @(Get-Process -Name WINWORD, EXCEL, POWERPNT -ErrorAction SilentlyContinue | Where-Object { $_.MainWindowHandle -eq 0 -and $_.StartTime -lt (Get-Date).AddMinutes(-2) })
$vis = @(Get-Process -Name WINWORD, EXCEL, POWERPNT -ErrorAction SilentlyContinue | Where-Object { $_.MainWindowHandle -ne 0 } | ForEach-Object { $_.ProcessName.ToUpper() })
$closed = @()
foreach ($p in $ghost) {
  $n = $p.ProcessName.ToUpper()
  if ($n -eq 'WINWORD' -and $vis -contains $n) { continue }
  try { $p | Stop-Process -Force -ErrorAction Stop; $closed += $OfficeNames[$n] } catch {}
}
if ($closed.Count) { Say ('Chiuse le copie di ' + (($closed | Select-Object -Unique) -join ', ') + ' rimaste aperte in sottofondo senza finestra: erano loro a tenere il file. Riprova ad aprirlo.') }
else { Say 'Sul tuo PC non c''è nessuna copia di Office nascosta che tiene il file.' }
Say 'Se il file è in una cartella di rete, su Teams o su SharePoint e lo ha aperto un collega: aprilo in "Sola lettura" o aspetta che lo chiuda. Su Teams e SharePoint potete anche lavorarci insieme, aprendolo dal browser.'
Say 'Se il collega l''ha già chiuso e il messaggio resta: nella stessa cartella c''è un file nascosto che inizia con ~$ e il nome del file. Lo può cancellare chi gestisce la cartella (o tu, se il file è solo tuo).'
`)},
	{ID: "of_regional", Name: "Date, decimali o file CSV sbagliati in Excel", Desc: "Rimette i formati italiani di Windows che usa Excel: date giorno/mese/anno, virgola per i decimali, punto e virgola nei file CSV (così non finisce tutto in una colonna). Puoi tornare indietro.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$k = 'HKCU\Control Panel\International'
$loc = [string](Get-RegValue $k 'LocaleName')
if ($TassoLang -eq 'en') {
  # In inglese non metto i formati italiani: mostro quelli di adesso e apro le impostazioni.
  $d = [string](Get-RegValue $k 'sDecimal'); $l = [string](Get-RegValue $k 'sList'); $sd = [string](Get-RegValue $k 'sShortDate')
  Say ('Your Windows formats: ' + $loc + ' (date ' + $sd + ', decimal separator "' + $d + '", list separator "' + $l + '").')
  Say 'Excel uses these formats. If dates or decimals look wrong, choose your country in the window I am opening (Settings > Time & language > Language & region > Regional format).'
  Say 'If a CSV file ends up all in one column: in Excel use Data > From Text/CSV and pick the separator. If a date stays reversed, the column is formatted as text: select it > Data > Text to Columns > Finish.'
  Open-User 'ms-settings:regionformatting'
  return
}
if ($loc -match '^it-' -and $loc -ne 'it-IT') { Say ('I formati sono quelli italiani di un altro paese (' + $loc + '): li lascio così.'); return }
if ($loc -and $loc -ne 'it-IT') {
  $b = Backup-Reg $k 'formati-regionali'
  if (-not $b) { Say 'Non sono riuscito a fare la copia di sicurezza, quindi non cambio niente. Ti apro le impostazioni dei formati.'; Open-User 'ms-settings:regionformatting'; return }
  try { Set-Culture -CultureInfo 'it-IT' -ErrorAction Stop } catch { Say 'Windows non mi ha lasciato cambiare i formati: te li apro, scegli "Italiano (Italia)".'; Open-User 'ms-settings:regionformatting'; return }
  Say ('I formati di Windows erano "' + $loc + '": ora sono italiani (date 25/10/2026, decimali con la virgola, CSV con il punto e virgola).')
  Say ('Per tornare come prima: doppio clic su ' + $b + ', poi esci e rientra in Windows.')
} else {
  $want = [ordered]@{ sShortDate = @('dd/MM/yyyy', 0x1F); sDecimal = @(',', 0x0E); sThousand = @('.', 0x0F); sList = @(';', 0x0C); sMonDecimalSep = @(',', 0x16); sMonThousandSep = @('.', 0x17) }
  $bad = @($want.Keys | Where-Object { [string](Get-RegValue $k $_) -ne $want[$_][0] })
  if (-not $bad.Count) { Say 'I formati sono già quelli italiani giusti.'; Say 'Se un CSV finisce tutto in una colonna: in Excel usa Dati > Da testo/CSV e scegli il separatore. Se una data resta al contrario, la colonna in Excel è formattata come testo: selezionala > Dati > Testo in colonne > Fine.'; return }
  $u = Save-Undo $k @($bad) 'formati-regionali'
  if (-not $u) { Say 'Non sono riuscito a fare la copia di sicurezza, quindi non cambio niente.'; return }
  try { Add-Type -Namespace TassoLoc -Name L -MemberDefinition '[DllImport("kernel32.dll", CharSet = CharSet.Unicode)] public static extern bool SetLocaleInfoW(uint lcid, uint type, string data); [DllImport("user32.dll", CharSet = CharSet.Unicode)] public static extern IntPtr SendMessageTimeoutW(IntPtr h, uint m, UIntPtr w, string l, uint f, uint t, out UIntPtr r);' -ErrorAction Stop } catch {}
  foreach ($n in $bad) {
    $ok = $false; try { $ok = [TassoLoc.L]::SetLocaleInfoW(0x400, [uint32]$want[$n][1], [string]$want[$n][0]) } catch {}
    if (-not $ok) { Set-RegValue $k $n $want[$n][0] 'String' }
  }
  try { $r = [UIntPtr]::Zero; [void][TassoLoc.L]::SendMessageTimeoutW([IntPtr]0xffff, 0x1A, [UIntPtr]::Zero, 'intl', 2, 3000, [ref]$r) } catch {}
  Say ('Sistemati: ' + (($bad | ForEach-Object { @{ sShortDate = 'formato della data'; sDecimal = 'virgola dei decimali'; sThousand = 'punto delle migliaia'; sList = 'separatore dei CSV'; sMonDecimalSep = 'decimali degli importi'; sMonThousandSep = 'migliaia degli importi' }[$_] }) -join ', ') + '.')
  Say ('Per tornare come prima: doppio clic su ' + $u + ', poi esci e rientra in Windows.')
}
Say 'Chiudi e riapri Excel per vedere i formati nuovi (se non cambiano, esci e rientra in Windows). I file già salvati non cambiano.'
`)},
	{ID: "of_adobe", Name: "Adobe Acrobat Reader non apre i PDF o si blocca", Desc: "Chiude Acrobat se è bloccato, mette da parte le sue impostazioni rovinate (riparte come nuovo) e avvia la riparazione ufficiale di Adobe. I tuoi PDF non si toccano.", Level: 0, Minutes: 2, Timeout: 15 * time.Minute, Script: ofs2(`
$exe = Get-AppPath 'Acrobat.exe'; if (-not $exe) { $exe = Get-AppPath 'AcroRd32.exe' }
$prod = @(Get-ChildItem -LiteralPath 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall', 'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall' -ErrorAction SilentlyContinue | ForEach-Object { Get-ItemProperty -LiteralPath $_.PSPath -ErrorAction SilentlyContinue } | Where-Object { [string]$_.DisplayName -match '^Adobe Acrobat' })
if (-not $exe -and -not $prod.Count) { Say 'Adobe Acrobat Reader non è installato. I PDF si possono aprire con Microsoft Edge (è già nel PC): tasto destro sul PDF > Apri con > Microsoft Edge. Se ti serve Acrobat, scaricalo solo da get.adobe.com/it/reader (attento alle caselle di programmi in più).'; return }
$run = @(Get-Process -Name Acrobat, AcroRd32, AcroCEF, RdrCEF -ErrorAction SilentlyContinue)
$hung = @($run | Where-Object { $_.MainWindowHandle -ne 0 -and -not $_.Responding })
if ($hung.Count) { Say 'Acrobat non risponde: lo chiudo.'; Get-Process -Name Acrobat, AcroRd32, AcroCEF, RdrCEF -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue; Start-Sleep -Seconds 2 }
elseif (@($run | Where-Object { $_.MainWindowHandle -ne 0 }).Count) { Say 'Acrobat è aperto: se stai compilando un modulo PDF salvalo, chiudi Acrobat e premi di nuovo Risolvi.'; return }
else { $run | Stop-Process -Force -ErrorAction SilentlyContinue }
$moved = 0
foreach ($d in @((Join-Path $env:APPDATA 'Adobe\Acrobat\DC'), (Join-Path $env:LOCALAPPDATA 'Adobe\Acrobat\DC'))) {
  if (Test-Path -LiteralPath $d) { try { Rename-Item -LiteralPath $d -NewName ('DC_vecchia_' + (Get-Date -Format 'yyyyMMdd_HHmm')) -ErrorAction Stop; $moved++ } catch {} }
}
if ($moved) { Say 'Messe da parte le impostazioni di Acrobat: riparte come appena installato (ti richiederà di accettare la licenza). Firme digitali, timbri e preferiti di Acrobat non si vedono finché non torni indietro. Per tornare indietro: rinomina di nuovo le cartelle "DC_vecchia_..." in "DC" (in %APPDATA%\Adobe\Acrobat e %LOCALAPPDATA%\Adobe\Acrobat).' }
$msi = @($prod | Where-Object { $_.WindowsInstaller -eq 1 -and [string]$_.PSChildName -match '^\{[0-9A-Fa-f-]{36}\}$' } | Select-Object -First 1)
if ($msi.Count) {
  Say 'Avvio la riparazione ufficiale di Acrobat (rimette a posto i file del programma, qualche minuto)...'
  $c = Run-Hidden 'msiexec.exe' ('/fa ' + $msi[0].PSChildName + ' /qn /norestart')
  if ($c -eq 0 -or $c -eq 3010) { Say 'Riparazione finita.' } else { Say 'La riparazione automatica non è partita. Apri Acrobat > Menu (o ?) > "Ripara installazione".' }
} else { Say 'Se ancora non va: apri Acrobat > Menu > Guida > "Ripara installazione", oppure disinstallalo e reinstallalo da get.adobe.com/it/reader.' }
$pc = Get-UserChoice '.pdf'
if ($pc -and $pc -notmatch '^Acro') { Say ('I PDF ora si aprono con: ' + (Get-AssocName $pc) + '. Se li vuoi in Acrobat: tasto destro su un PDF > Apri con > Scegli un''altra app > Adobe Acrobat > "Sempre".') }
`)},

	// ---------- File e cartelle ----------
	{ID: "of_open_with", Name: "I file si aprono con il programma sbagliato", Desc: "Ti dice con quale programma si aprono PDF, documenti di Word ed Excel, foto, file zip e link, e ti apre la pagina giusta per cambiarlo (Windows non lascia farlo ai programmi, per sicurezza). Controlla e spiega.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$list = [ordered]@{ '.pdf' = 'PDF'; '.docx' = 'Word (.docx)'; '.xlsx' = 'Excel (.xlsx)'; '.pptx' = 'PowerPoint (.pptx)'; '.jpg' = 'Foto (.jpg)'; '.zip' = 'File zip'; 'https' = 'Link dei siti'; 'mailto' = 'Indirizzi email' }
$broken = 0
foreach ($e in $list.Keys) {
  $p = Get-UserChoice $e
  if (-not $p) { Say ($list[$e] + ': programma normale di Windows.'); continue }
  $ok = Test-ProgIdOk $p
  if (-not $ok) { $broken++; Say ($list[$e] + ': ' + (Get-AssocName $p) + ' - ma quel programma non c''è più!') } else { Say ($list[$e] + ': ' + (Get-AssocName $p)) }
}
if ($broken) { Say 'Alcuni tipi di file puntano a un programma disinstallato: per questo non si aprono.' }
Say 'Per cambiare: nella finestra che ti apro scrivi il tipo di file in alto (per esempio .pdf) e scegli il programma. Oppure tasto destro sul file > Apri con > Scegli un''altra app > spunta "Usa sempre".'
Open-User 'ms-settings:defaultapps'
`)},
	{ID: "of_longpath", Name: "\"Il nome del file è troppo lungo\"", Desc: "Accende in Windows i percorsi lunghi (oltre 260 caratteri), utile con le cartelle di OneDrive, SharePoint e del server. Ti dice anche come accorciare, perché alcuni programmi restano al limite.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$k = 'HKLM\SYSTEM\CurrentControlSet\Control\FileSystem'
if ((Get-RegValue $k 'LongPathsEnabled') -eq 1) { Say 'I percorsi lunghi erano già accesi.' }
else { $u = Save-Undo $k @('LongPathsEnabled') 'percorsi-lunghi'; Set-RegValue $k 'LongPathsEnabled' 1 'DWord'; Say 'Accesi i percorsi lunghi: i programmi moderni ora possono usare nomi e cartelle più lunghi di 260 caratteri.'; Say-Undo $u }
Say 'Attenzione: Esplora file, Word ed Excel restano comunque al limite di circa 250 caratteri (nome di tutte le cartelle + nome del file). Se il messaggio torna:'
Say '1) accorcia i nomi delle cartelle (es. "Documenti condivisi del reparto amministrazione 2026" -> "Amministrazione 2026"); 2) sposta la cartella più in alto; 3) per le cartelle di Teams/SharePoint, sincronizza la sottocartella che ti serve invece di tutta la raccolta.'
`)},
	{ID: "of_file_locked", Name: "Non posso cancellare o rinominare un file (\"è aperto in un altro programma\")", Desc: "Chiude l'anteprima di Esplora file e riavvia Esplora file, che spesso tengono il file occupato. Le finestre delle cartelle si chiudono: se stai copiando file, aspetta che finisca.", Level: 0, Minutes: 0.2, Timeout: time.Minute, Script: ofs2(`
$o = Get-OpenOffice
if ($o.Count) { Say ('Sono aperti: ' + ($o -join ', ') + '. Se il file è un documento di Office, chiudilo lì: è quasi sempre lui a tenerlo.') }
Get-Process -Name prevhost -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Say 'Chiusa l''anteprima dei file e riavvio Esplora file...'
Restart-Explorer
Say 'Riprova adesso. Se ancora non va:'
Say '1) chiudi il programma con cui avevi aperto il file (anche dalla barra in basso o vicino all''orologio); 2) spegni il riquadro di anteprima in Esplora file (Alt + P); 3) riavvia il PC: libera sempre tutto.'
Say 'Se il file è in OneDrive e la nuvoletta sta sincronizzando, aspetta che finisca.'
`)},
	{ID: "of_deleted", Name: "Ho cancellato un file per sbaglio", Desc: "Ti dice dove può essere: Cestino, Cestino di OneDrive, versioni precedenti, Cronologia file. Apre il Cestino. Se non c'è, prova lo strumento \"Recupera file cancellati\" di Tasso. Controlla e spiega.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs2(`
$n = 0; try { $n = (New-Object -ComObject Shell.Application).Namespace(10).Items().Count } catch {}
Say ('1) Cestino: ci sono ' + $n + ' elementi. Te lo apro: cerca il file, tasto destro > Ripristina.')
$od = Get-OneDriveRoots
if ($od.Count) { Say '2) Il file era in OneDrive? Anche OneDrive ha il suo cestino (tiene i file 30 giorni, 93 per gli account di lavoro): tasto destro sulla nuvoletta vicino all''orologio > Visualizza online > Cestino.' }
$sc = @(Get-CimInstance Win32_ShadowCopy -ErrorAction SilentlyContinue)
if ($sc.Count) { Say ('3) Windows ha ' + $sc.Count + ' "versioni precedenti" salvate: tasto destro sulla cartella dove era il file > Proprietà > Versioni precedenti.') }
$fh = Get-Service -Name fhsvc -ErrorAction SilentlyContinue
if ($fh -and $fh.Status -eq 'Running') { Say '4) La Cronologia file è accesa: Pannello di controllo > Cronologia file > Ripristina file personali.' }
Say 'Se il file era in una cartella di rete o di Teams/SharePoint: chiedi all''assistenza, di solito hanno la copia della sera prima. Su Teams/SharePoint c''è anche il Cestino del sito.'
Say 'Se non è da nessuna parte: in Tasso c''è lo strumento "Recupera file cancellati". Più presto lo usi, più è probabile ritrovarlo: nel frattempo non salvare file grandi sullo stesso disco.'
Open-User 'shell:RecycleBinFolder'
`)},
	{ID: "of_hidden_files", Name: "Una cartella o un file è sparito (forse è nascosto)", Desc: "Fa vedere file e cartelle nascosti e, se una chiavetta ha le cartelle nascoste da un virus (succede spesso), le fa ricomparire. Puoi tornare indietro.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs2(`
$k = 'HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced'
if ((Get-RegValue $k 'Hidden') -ne 1) { $u = Save-Undo $k @('Hidden') 'file-nascosti'; Set-RegValue $k 'Hidden' 1 'DWord'; Say 'Ora Esplora file mostra anche i file e le cartelle nascosti (sono un po'' trasparenti). Premi F5 o riapri la cartella.'; Say-Undo $u }
else { Say 'I file nascosti si vedevano già.' }
$fixed = 0; $lnk = 0; $names = @()
foreach ($d in @(Get-CimInstance Win32_LogicalDisk -Filter 'DriveType=2' -ErrorAction SilentlyContinue)) {
  $root = $d.DeviceID + '\'
  foreach ($it in @(Get-ChildItem -LiteralPath $root -Directory -Force -ErrorAction SilentlyContinue | Where-Object { ($_.Attributes -band [IO.FileAttributes]::Hidden) -and $_.Name -notmatch '^(System Volume Information|\$RECYCLE\.BIN|\$Recycle\.Bin|\..*)$' })) {
    try { $it.Attributes = $it.Attributes -band (-bnot ([IO.FileAttributes]::Hidden -bor [IO.FileAttributes]::System)); $fixed++; $names += ($root + $it.Name) } catch {}
  }
  if ($fixed) { $lnk += @(Get-ChildItem -LiteralPath $root -Filter '*.lnk' -File -Force -ErrorAction SilentlyContinue).Count }
}
if ($fixed) { Say ('Sulla chiavetta erano nascoste ' + $fixed + ' cartelle: ora si vedono di nuovo (' + (($names | Select-Object -First 6) -join ', ') + '). I file dentro ci sono tutti. Se una la volevi nascosta: tasto destro > Proprietà > spunta "Nascosto".') }
if ($lnk) { Say 'ATTENZIONE: sulla chiavetta ci sono dei collegamenti (frecciette) al posto delle cartelle: è il segno di un virus da chiavetta. NON aprirli: apri le cartelle vere, copia i tuoi file sul PC e poi fai "Controllo dei programmi sospetti".' }
Say 'Se cerchi un file e non ricordi dove l''hai salvato: apri Esplora file, clicca su "Questo PC" e scrivi una parte del nome nella casella di ricerca in alto a destra.'
`)},
	{ID: "of_shortcuts", Name: "Icone del desktop che non aprono più niente", Desc: "Trova sul desktop i collegamenti che puntano a programmi o file che non ci sono più e li sposta in una cartella dentro Documenti (non li cancella).", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs2(`
$sh = New-Object -ComObject WScript.Shell
$dst = Join-Path ([Environment]::GetFolderPath('MyDocuments')) 'Tasso\Collegamenti che non funzionano'
$n = 0; $names = @()
foreach ($dir in @([Environment]::GetFolderPath('Desktop'), [Environment]::GetFolderPath('CommonDesktopDirectory'))) {
  foreach ($f in @(Get-ChildItem -LiteralPath $dir -Filter '*.lnk' -File -ErrorAction SilentlyContinue)) {
    $t = ''; try { $t = [string]$sh.CreateShortcut($f.FullName).TargetPath } catch { continue }
    if (-not $t -or $t -match '^\\\\' -or $t -notmatch '^[A-Za-z]:\\') { continue }
    $drv = $t.Substring(0, 3)
    if (-not (Test-Path -LiteralPath $drv)) { continue }
    if ((Get-CimInstance Win32_LogicalDisk -Filter ("DeviceID='" + $t.Substring(0, 2) + "'") -ErrorAction SilentlyContinue).DriveType -ne 3) { continue }
    if (Test-Path -LiteralPath $t) { continue }
    New-Item -ItemType Directory -Path $dst -Force | Out-Null
    $to = Join-Path $dst $f.Name; $i = 1
    while (Test-Path -LiteralPath $to) { $to = Join-Path $dst ($f.BaseName + ' (' + $i + ')' + $f.Extension); $i++ }
    try { Move-Item -LiteralPath $f.FullName -Destination $to -ErrorAction Stop; $n++; $names += $f.BaseName } catch {}
  }
}
if ($n) { Say ('Spostati ' + $n + ' collegamenti che non aprivano più niente: ' + (($names | Select-Object -First 8) -join ', ') + '.'); Say ('Sono in: ' + $dst + '. Se ti serve uno di quei programmi, reinstallalo; poi cancella pure la cartella.') }
else { Say 'Tutti i collegamenti del desktop funzionano (quelli verso cartelle di rete o chiavette non li controllo: dipendono da dove sei collegato).' }
`)},
	{ID: "of_folders_slow", Name: "Le cartelle si aprono lente o con la vista sbagliata", Desc: "Azzera le viste salvate delle cartelle (icone, colonne, ordine), che col tempo si rovinano e rallentano Esplora file. Con copia di sicurezza. Cartelle e file non si toccano.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs2(`
$base = 'HKCU\Software\Classes\Local Settings\Software\Microsoft\Windows\Shell'
$b1 = ''; $b2 = ''
if (Test-Path -LiteralPath ('Registry::' + (Get-FullKey ($base + '\Bags')))) { $b1 = Backup-Reg ($base + '\Bags') 'viste-cartelle' ; if (-not $b1) { Say 'Non sono riuscito a fare la copia di sicurezza, quindi non cambio niente.'; return } }
if (Test-Path -LiteralPath ('Registry::' + (Get-FullKey ($base + '\BagMRU')))) { $b2 = Backup-Reg ($base + '\BagMRU') 'viste-cartelle-elenco'; if (-not $b2) { Say 'Non sono riuscito a fare la copia di sicurezza, quindi non cambio niente.'; return } }
Remove-Item -LiteralPath ('Registry::' + (Get-FullKey ($base + '\Bags'))) -Recurse -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath ('Registry::' + (Get-FullKey ($base + '\BagMRU'))) -Recurse -Force -ErrorAction SilentlyContinue
Say 'Viste delle cartelle azzerate: riavvio Esplora file (le finestre delle cartelle si chiudono)...'
Restart-Explorer
Say 'Fatto. Ogni cartella si riapre con la vista normale; se ne vuoi una diversa (icone grandi, dettagli) la scegli da Visualizza e Windows la ricorda.'
if ($b1) { Say ('Per tornare come prima: doppio clic su ' + $b1 + ($(if ($b2) { ' e su ' + $b2 } else { '' }))) }
`)},
	{ID: "of_explorer_home", Name: "Esplora file si apre su \"Home\" invece di \"Questo PC\"", Desc: "Fa aprire Esplora file direttamente su Questo PC, con dischi, chiavette e unità di rete in vista. Puoi tornare indietro.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$k = 'HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced'
if ((Get-RegValue $k 'LaunchTo') -eq 1) { Say 'Esplora file si apriva già su Questo PC.'; return }
$u = Save-Undo $k @('LaunchTo') 'esplora-file-apertura'
Set-RegValue $k 'LaunchTo' 1 'DWord'
Say 'Fatto: da ora Esplora file (Win + E) si apre su Questo PC.'
Say-Undo $u
`)},
	{ID: "of_disk_full", Name: "Il disco C: è pieno", Desc: "Ti dice cosa occupa più spazio (Download, Desktop, Documenti, Cestino, OneDrive, vecchio Windows) e cosa puoi liberare in sicurezza. Non cancella niente da solo. Controlla e spiega.", Level: 0, Minutes: 1, Timeout: 8 * time.Minute, Script: ofs2(`
$c = Get-PSDrive -Name ($env:SystemDrive.Trim(':')) -ErrorAction SilentlyContinue
if ($c) { $tot = $c.Used + $c.Free; Say ('Disco C: liberi ' + (Fmt $c.Free) + ' su ' + (Fmt $tot) + ' (' + [int]($c.Free * 100 / [math]::Max(1, $tot)) + '%).') }
Say 'Misuro le cartelle più grandi (un minuto)...'
$rows = @()
foreach ($x in @(@('Download', (Get-DownloadsPath)), @('Desktop', [Environment]::GetFolderPath('Desktop')), @('Documenti', [Environment]::GetFolderPath('MyDocuments')), @('Immagini', [Environment]::GetFolderPath('MyPictures')), @('Video', [Environment]::GetFolderPath('MyVideos')))) { $rows += , @($x[0], (Get-LocalSize $x[1])) }
foreach ($r in Get-OneDriveRoots) { $rows += , @(('OneDrive (' + (Split-Path $r -Leaf) + ', solo i file scaricati)'), (Get-LocalSize $r)) }
$rb = Get-FolderSize (Join-Path $env:SystemDrive '$Recycle.Bin'); $rows += , @('Cestino', $rb)
$wo = Join-Path $env:SystemDrive 'Windows.old'; if (Test-Path -LiteralPath $wo) { $rows += , @('Windows.old (il Windows di prima)', (Get-FolderSize $wo)) }
$hib = Join-Path $env:SystemDrive 'hiberfil.sys'; if (Test-Path -LiteralPath $hib) { $rows += , @('Ibernazione (hiberfil.sys)', (Get-Item -LiteralPath $hib -Force).Length) }
foreach ($r in ($rows | Sort-Object { [long]$_[1] } -Descending)) { if ([long]$r[1] -gt 200MB) { Say ('- ' + $r[0] + ': ' + (Fmt $r[1])) } }
Say 'Cosa puoi fare in sicurezza:'
Say '1) In Tasso, Pulizia > livello Media o Profonda: toglie file temporanei, cache e (in Profonda) il vecchio Windows. Non tocca i tuoi documenti.'
Say '2) Download: è la cartella che cresce di più. In Tasso c''è "Riordino della cartella Download", oppure aprila e ordina per Dimensione.'
if ((Get-OneDriveRoots).Count) { Say '3) OneDrive: usa "Libera spazio con OneDrive" qui in Risolvi problemi: i file vecchi restano online e smettono di occupare il disco.' }
if ($rb -gt 1GB) { Say ('4) Il Cestino occupa ' + (Fmt $rb) + ': svuotalo se non ti serve niente.') }
Say 'Video e foto: spostali su un disco esterno o in OneDrive. Tasso non cancella mai i tuoi file da solo.'
`)},
	{ID: "of_zip", Name: "I file .zip non si aprono o danno errore", Desc: "Controlla con cosa si aprono i file zip, rimette le \"cartelle compresse\" di Windows se un programma disinstallato le ha rotte e ti dice cosa fare con zip protetti da password, file bloccati e nomi troppo lunghi.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$p = Get-UserChoice '.zip'
$sys = [string](Get-RegValue 'HKLM\SOFTWARE\Classes\.zip' '(default)')
if ($sys -and -not (Test-ProgIdOk $sys) -and (Test-Path -LiteralPath 'Registry::HKEY_CLASSES_ROOT\CompressedFolder')) {
  $u = Save-Undo 'HKLM\SOFTWARE\Classes\.zip' @('') 'zip-windows'
  Set-RegValue 'HKLM\SOFTWARE\Classes\.zip' '(default)' 'CompressedFolder' 'String'
  Say ('I file zip erano legati a un programma che non c''è più (' + (Get-AssocName $sys) + '): li ho rimessi alle cartelle compresse di Windows.'); Say-Undo $u
}
if ($p -and -not (Test-ProgIdOk $p)) { Say ('Hai scelto di aprire gli zip con ' + (Get-AssocName $p) + ', che però non c''è più. Tasto destro su uno zip > Apri con > Scegli un''altra app > "Esplora risorse" (o il programma che usi) > Sempre.'); Open-User 'ms-settings:defaultapps' }
elseif ($p) { Say ('Gli zip si aprono con: ' + (Get-AssocName $p) + '.') }
else { Say 'Gli zip si aprono con Windows (cartelle compresse).' }
$blk = @(Get-ChildItem -LiteralPath (Get-DownloadsPath) -Filter '*.zip' -File -ErrorAction SilentlyContinue | Where-Object { Get-Item -LiteralPath $_.FullName -Stream Zone.Identifier -ErrorAction SilentlyContinue })
if ($blk.Count) { Say ('Nei Download ci sono ' + $blk.Count + ' zip segnati "da internet": i file che estrai sono bloccati anche loro (Word li apre in sola lettura, i programmi non partono). Se ti fidi di chi l''ha mandato: tasto destro sullo zip > Proprietà > spunta "Sblocca" PRIMA di estrarre.') }
Say 'Consigli: 1) non aprire i file direttamente dentro lo zip, prima fai tasto destro > "Estrai tutto"; 2) se dice "percorso troppo lungo", estrailo in una cartella corta (es. C:\Estratti); 3) se lo zip è "non valido", spesso il download non è finito: scaricalo di nuovo.'
Say 'Zip con password moderne (AES), file .rar e .7z: Windows 11 aggiornato apre .rar e .7z, ma non gli zip con password AES. In quel caso serve 7-Zip (gratuito, da 7-zip.org).'
`)},
	{ID: "of_context_menu", Name: "Windows 11: il menu del tasto destro è corto", Desc: "Rimette il menu completo del tasto destro (come in Windows 10), senza dover premere \"Mostra altre opzioni\" ogni volta. Puoi tornare indietro.", Level: 0, Minutes: 0.2, Timeout: time.Minute, Script: ofs2(`
if ([Environment]::OSVersion.Version.Build -lt 22000) { Say 'Questo PC ha Windows 10: il menu del tasto destro è già quello completo.'; return }
$k = 'HKCU\Software\Classes\CLSID\{86ca1aa0-34aa-4e8b-a509-50c905bae2a2}'
if (Test-Path -LiteralPath ('Registry::' + (Get-FullKey ($k + '\InprocServer32')))) { Say 'Il menu completo era già acceso.'; return }
Set-RegValue ($k + '\InprocServer32') '(default)' '' 'String'
$u = New-UndoFile @('[-' + (Get-FullKey $k) + ']') 'menu-tasto-destro'
Say 'Fatto: riavvio Esplora file (le finestre delle cartelle si chiudono)...'
Restart-Explorer
Say 'Ora il tasto destro mostra subito tutte le voci (come in Windows 10).'
Say-Undo $u
`)},

	// ---------- Outlook e posta ----------
	{ID: "of_outlook_ost", Name: "Outlook non si aggiorna o mostra cartelle vuote", Desc: "Mette da parte il file locale di Outlook (.ost): Outlook lo riscarica dal server con le mail giuste. Solo per account Microsoft 365, Exchange, Outlook.com e IMAP, dove le mail sono online. Non cancella niente.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs2(`
if (Stop-IfOfficeOpen @('OUTLOOK')) { return }
$dir = Join-Path $env:LOCALAPPDATA 'Microsoft\Outlook'
$ost = @(Get-ChildItem -LiteralPath $dir -Filter '*.ost' -File -ErrorAction SilentlyContinue)
if (-not $ost.Count) { Say 'Non ci sono file .ost: il tuo account usa un file .pst (POP), dove le mail sono solo sul PC. In quel caso non tocco niente: usa "Outlook non si apre" o "Outlook lento: la casella è troppo grande".'; return }
$tot = ($ost | Measure-Object -Property Length -Sum).Sum
$c = Get-PSDrive -Name ($env:SystemDrive.Trim(':')) -ErrorAction SilentlyContinue
if ($c -and $c.Free -lt $tot) { Say ('Per riscaricare la posta servono circa ' + (Fmt $tot) + ' liberi e sul disco ce ne sono ' + (Fmt $c.Free) + ': prima libera spazio.'); return }
$stamp = Get-Date -Format 'yyyyMMdd_HHmm'
$n = 0
foreach ($f in $ost) { try { Rename-Item -LiteralPath $f.FullName -NewName ($f.Name + '.vecchio_' + $stamp) -ErrorAction Stop; $n++ } catch {} }
if (-not $n) { Say 'Il file di Outlook è ancora in uso: riavvia il PC e premi di nuovo Risolvi.'; return }
Say ('Messo da parte il file locale di Outlook (' + (Fmt $tot) + '). Apri Outlook: riscarica la posta dal server. Con tante mail può metterci da qualche minuto a qualche ora, ma puoi già usarlo.')
Say 'Attenzione: le mail rimaste nella Posta in uscita (non ancora inviate) erano solo nel file vecchio. Se ne avevi, riscrivile.'
Say ('Se qualcosa non va, per tornare indietro: chiudi Outlook, apri ' + $dir + ', cancella il .ost nuovo e togli ".vecchio_' + $stamp + '" dal nome di quello vecchio. Quando tutto va bene da una settimana, il vecchio si può cancellare.')
`)},
	{ID: "of_outlook_classic", Name: "Outlook è cambiato: rivoglio quello di prima", Desc: "Se Windows è passato da solo al \"nuovo Outlook\", rimette Outlook classico come predefinito (se è installato) e lo apre. Le mail sono le stesse: cambia solo il programma.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$ol = Get-AppPath 'OUTLOOK.EXE'
if (-not $ol) { Say 'Outlook classico non è installato: c''è solo il nuovo Outlook. Quello classico è compreso in Microsoft 365 e nelle versioni di Office con Outlook: chiedi a chi gestisce le licenze.'; return }
$k = 'HKCU\Software\Microsoft\Office\16.0\Outlook\Preferences'
if ((Get-RegValue $k 'UseNewOutlook') -ne 1) { Say 'Outlook classico è già quello predefinito: lo apro. Nel menu Start cercalo come "Outlook (classic)", non "Outlook (new)".'; Open-User $ol; return }
$u = Save-Undo $k @('UseNewOutlook') 'outlook-classico'
Set-RegValue $k 'UseNewOutlook' 0 'DWord'
Get-Process -Name olk -ErrorAction SilentlyContinue | Where-Object { $_.MainWindowHandle -eq 0 } | Stop-Process -Force -ErrorAction SilentlyContinue
Say 'Fatto: Outlook classico è di nuovo quello predefinito. Lo apro.'
Open-User $ol
Say 'Nel menu Start cercalo come "Outlook (classic)". Se in alto a destra vedi l''interruttore "Nuovo Outlook", lascialo spento.'
Say-Undo $u
`)},
	{ID: "of_links", Name: "I link nelle email non si aprono", Desc: "Controlla il browser predefinito, il problema più comune quando Outlook dice \"criteri dell'organizzazione\" o non fa niente. Ti apre la pagina giusta per sceglierlo di nuovo (Windows non lascia farlo ai programmi). Controlla e spiega.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$h = Get-UserChoice 'https'; $h2 = Get-UserChoice 'http'
if (-not $h) { Say 'Su questo PC non c''è un browser predefinito per i link: è la causa.' }
elseif (-not (Test-ProgIdOk $h)) { Say ('I link si aprono con ' + (Get-AssocName $h) + ', che però non c''è più (disinstallato): è la causa.') }
elseif ($h2 -and $h2 -ne $h) { Say ('I link "https" si aprono con ' + (Get-AssocName $h) + ' e quelli "http" con ' + (Get-AssocName $h2) + ': conviene sceglierne uno solo.') }
else { Say ('I link si aprono con ' + (Get-AssocName $h) + '.') }
Say 'Cosa fare: nella finestra che ti apro scegli di nuovo il browser che usi (anche se è già quello giusto) e premi "Imposta come predefinito". Questo sistema anche l''errore "criteri dell''organizzazione".'
Say 'Poi chiudi e riapri Outlook.'
Open-User 'ms-settings:defaultapps'
`)},
	{ID: "of_mailto", Name: "\"Nessun programma di posta\" quando invio un file per email", Desc: "Rimette Outlook come programma di posta di Windows, quello che usano \"Invia a > Destinatario\" e i pulsanti \"Invia per email\" degli altri programmi. Puoi tornare indietro.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$ol = Get-AppPath 'OUTLOOK.EXE'
if (-not $ol -or -not (Test-Path -LiteralPath 'HKLM:\SOFTWARE\Clients\Mail\Microsoft Outlook')) { Say 'Outlook classico non è installato. "Invia a > Destinatario" e "Invia per email" funzionano solo con un programma di posta installato (Outlook classico, Thunderbird): con il nuovo Outlook o la posta nel browser allega il file a mano.'; return }
$f = @()
$cur = [string](Get-RegValue 'HKLM\SOFTWARE\Clients\Mail' '(default)')
if ($cur -ne 'Microsoft Outlook') { $u = Save-Undo 'HKLM\SOFTWARE\Clients\Mail' @('') 'posta-predefinita-pc'; Set-RegValue 'HKLM\SOFTWARE\Clients\Mail' '(default)' 'Microsoft Outlook' 'String'; $f += $u }
$cu = Get-RegValue 'HKCU\Software\Clients\Mail' '(default)'
if ($cu -ne $null -and [string]$cu -ne 'Microsoft Outlook') { $u = Save-Undo 'HKCU\Software\Clients\Mail' @('') 'posta-predefinita-utente'; Set-RegValue 'HKCU\Software\Clients\Mail' '(default)' 'Microsoft Outlook' 'String'; $f += $u }
if ($f.Count) { Say 'Fatto: Outlook è di nuovo il programma di posta di Windows. Riprova "Invia a > Destinatario posta".' } else { Say 'Outlook era già il programma di posta di Windows.' }
$m = Get-UserChoice 'mailto'
if ($m -and $m -notmatch '^Outlook') { Say ('Gli indirizzi email nei siti (link "mailto") si aprono con ' + (Get-AssocName $m) + ': se li vuoi in Outlook, nella finestra che ti apro cerca "mailto" e scegli Outlook.'); Open-User 'ms-settings:defaultapps' }
foreach ($x in $f) { Say-Undo $x }
`)},

	// ---------- Teams e riunioni ----------
	{ID: "of_notif", Name: "Non arrivano le notifiche di Teams e Outlook", Desc: "Riaccende le notifiche di Windows e quelle di Teams e Outlook se erano spente, e ti dice come vedere se è acceso \"Non disturbare\". Puoi tornare indietro.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$ch = @()
$pk = 'HKCU\Software\Microsoft\Windows\CurrentVersion\PushNotifications'
if ((Get-RegValue $pk 'ToastEnabled') -eq 0) { $u = Save-Undo $pk @('ToastEnabled') 'notifiche-windows'; Set-RegValue $pk 'ToastEnabled' 1 'DWord'; $ch += 'le notifiche di Windows (erano spente per tutti i programmi)'; Say-Undo $u }
$base = 'HKCU\Software\Microsoft\Windows\CurrentVersion\Notifications\Settings'
foreach ($s in @(Get-ChildItem -LiteralPath ('Registry::' + (Get-FullKey $base)) -ErrorAction SilentlyContinue | Where-Object { $_.PSChildName -match 'Teams|OUTLOOK|Outlook|olk' })) {
  $k = $base + '\' + $s.PSChildName
  if ((Get-RegValue $k 'Enabled') -eq 0) { $u = Save-Undo $k @('Enabled') 'notifiche-app'; Set-RegValue $k 'Enabled' 1 'DWord'; $ch += ('le notifiche di ' + $(if ($s.PSChildName -match 'Teams') { 'Teams' } else { 'Outlook' })); Say-Undo $u }
}
foreach ($sv in @(Get-Service -Name 'WpnUserService_*', 'WpnService' -ErrorAction SilentlyContinue | Where-Object { $_.Status -ne 'Running' -and $_.StartType -ne 'Disabled' })) { Start-Service -Name $sv.Name -ErrorAction SilentlyContinue; if ((Get-Service -Name $sv.Name -ErrorAction SilentlyContinue).Status -eq 'Running') { $ch += 'il servizio delle notifiche (era fermo)' } }
if ($ch.Count) { Say ('Riacceso: ' + ($ch -join ', ') + '. Chiudi e riapri Teams e Outlook.') } else { Say 'Le notifiche di Windows, Teams e Outlook erano già accese.' }
Say '"Non disturbare": se vicino all''orologio vedi una campanella con le Z o una luna, è acceso e le notifiche restano nascoste. Cliccaci sopra per spegnerlo (in Windows 11 si accende anche da solo quando condividi lo schermo).'
Say 'In Teams controlla anche: i tre puntini in alto > Impostazioni > Notifiche e attività.'
Open-User 'ms-settings:notifications'
`)},
	{ID: "of_cam_busy", Name: "La webcam è \"usata da un altro programma\"", Desc: "Ti dice quale programma sta usando adesso la webcam o il microfono (Teams, Zoom, il browser...), così sai cosa chiudere. Controlla e spiega.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
function Get-InUse([string]$what) {
  $out = @()
  $b = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\' + $what
  $keys = @(Get-ChildItem -LiteralPath $b -ErrorAction SilentlyContinue | Where-Object { $_.PSChildName -ne 'NonPackaged' }) + @(Get-ChildItem -LiteralPath ($b + '\NonPackaged') -ErrorAction SilentlyContinue)
  foreach ($k in $keys) {
    $p = Get-ItemProperty -LiteralPath $k.PSPath -ErrorAction SilentlyContinue
    if ($p -and $p.LastUsedTimeStart -and [long]$p.LastUsedTimeStop -eq 0) {
      $n = $k.PSChildName; if ($n -match '#') { $n = ($n -split '#')[-1] } else { $n = ($n -split '_')[0] }
      $out += $n
    }
  }
  return @($out | Select-Object -Unique)
}
$cam = Get-InUse 'webcam'; $mic = Get-InUse 'microphone'
if ($cam.Count) { Say ('In questo momento la webcam è usata da: ' + ($cam -join ', ') + '. Chiudi quel programma (anche dall''icona vicino all''orologio) e riprova.') }
else { Say 'In questo momento nessun programma sta usando la webcam.' }
if ($mic.Count) { Say ('Il microfono è usato da: ' + ($mic -join ', ') + '.') }
Say 'Ricorda: la webcam la può usare un solo programma alla volta. Se hai Teams aperto anche nel browser, chiudi una delle due.'
if (-not $cam.Count) { Say 'Se la webcam dice ancora "in uso", prova "La webcam non va": riavvia la webcam e controlla i permessi.' }
`)},
	{ID: "of_sound_comm", Name: "Il volume si abbassa da solo durante le chiamate", Desc: "Dice a Windows di non abbassare più musica e altri suoni quando sei in chiamata su Teams, Zoom o Skype. Puoi tornare indietro.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$k = 'HKCU\Software\Microsoft\Multimedia\Audio'
if ((Get-RegValue $k 'UserDuckingPreference') -eq 3) { Say 'Windows già non abbassava il volume nelle chiamate. Se succede ancora, è il programma della riunione o delle cuffie: in Teams controlla Impostazioni > Dispositivi.'; return }
$u = Save-Undo $k @('UserDuckingPreference') 'volume-chiamate'
Set-RegValue $k 'UserDuckingPreference' 3 'DWord'
Say 'Fatto: durante le chiamate Windows non abbassa più gli altri suoni. Vale dalla prossima chiamata.'
Say 'Se le cuffie Bluetooth suonano "metalliche" in chiamata, è un''altra cosa: usa "Le cuffie Bluetooth non si collegano", te lo spiega.'
Say-Undo $u
`)},

	// ---------- Rete ----------
	{ID: "of_ethernet", Name: "Il cavo di rete è collegato ma non va", Desc: "Controlla la scheda di rete del cavo: se è spenta la riaccende, se va lenta ti dice di cambiare cavo, poi la riavvia e chiede di nuovo l'indirizzo al router.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs2(`
$eth = @(Get-NetAdapter -Physical -ErrorAction SilentlyContinue | Where-Object { -not ($_.PhysicalMediaType -match '802\.11|Wireless|BlueTooth' -or $_.InterfaceDescription -match 'Wi-?Fi|Wireless|WLAN|802\.11|Bluetooth') })
if (-not $eth.Count) { Say 'Windows non vede nessuna presa di rete per il cavo. Sui portatili sottili si usa un adattatore USB o la docking station: controlla che sia collegato bene.'; return }
foreach ($a in $eth) {
  $s = [string]$a.Status
  if ($s -eq 'Disabled') { Enable-NetAdapter -Name $a.Name -Confirm:$false -ErrorAction SilentlyContinue; Say ($a.Name + ': la scheda era spenta in Windows, l''ho riaccesa.'); continue }
  if ($s -match 'Disconnected|Not ?Present') { Say ($a.Name + ': il PC non sente il cavo. Controlla che sia inserito fino al "clic" da tutte e due le parti, prova un altro cavo o un''altra presa. Le lucine vicino alla presa del PC devono accendersi.'); continue }
  if ($s -eq 'Up') {
    $sp = [string]$a.LinkSpeed
    if ($sp -match '^(10|100) Mbps') { Say ($a.Name + ': collegato solo a ' + $sp + ' invece di 1 Gbps. Di solito è il cavo rovinato o vecchio: cambialo (va bene un cavo "Cat 6").') } else { Say ($a.Name + ': collegato a ' + $sp + '.') }
    Say 'Riavvio la scheda (internet si stacca per qualche secondo)...'
    Restart-NetAdapter -Name $a.Name -Confirm:$false -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 8
    $ip = @(Get-NetIPAddress -InterfaceIndex $a.ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue | Select-Object -ExpandProperty IPAddress)
    if (@($ip | Where-Object { $_ -like '169.254.*' }).Count -or -not $ip.Count) { [void](Run-Hidden 'ipconfig.exe' '/renew'); Start-Sleep -Seconds 3; $ip = @(Get-NetIPAddress -InterfaceIndex $a.ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue | Select-Object -ExpandProperty IPAddress) }
    if (@($ip | Where-Object { $_ -like '169.254.*' }).Count -or -not $ip.Count) { Say 'Il cavo funziona ma il router non dà l''indirizzo al PC: riavvia il router o lo switch, o prova un''altra presa. In ufficio può essere la presa non attivata: chiedi all''assistenza.' }
  }
}
if (Test-Internet) { Say 'Internet funziona.' } else { Say 'Internet ancora non va: prova anche "Risolvi problemi di internet".' }
`)},
	{ID: "of_metered", Name: "Aggiornamenti e OneDrive fermi: \"connessione a consumo\"", Desc: "Se la rete Wi-Fi che usi è segnata \"a consumo\", Windows e OneDrive non scaricano. Toglie il segno dalla rete di casa o dell'ufficio (non dall'hotspot del telefono). Puoi rimetterlo.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$p = $null
try { [void][Windows.Networking.Connectivity.NetworkInformation, Windows.Networking.Connectivity, ContentType = WindowsRuntime]; $p = [Windows.Networking.Connectivity.NetworkInformation]::GetInternetConnectionProfile() } catch {}
if (-not $p) { Say 'Non sei collegato a internet in questo momento.'; return }
$cost = [string]$p.GetConnectionCost().NetworkCostType
$name = [string]$p.ProfileName
if ($cost -eq 'Unrestricted' -or $cost -eq 'Unknown') { Say ('La rete "' + $name + '" non è a consumo: non è questo che blocca gli aggiornamenti.'); return }
if (-not $p.IsWlanConnectionProfile) { Say ('La rete via cavo "' + $name + '" è segnata a consumo: toglilo nella finestra che ti apro (Connessione a consumo: No).'); Open-User 'ms-settings:network-ethernet'; return }
if ($name -match '^(iPhone|Android|Galaxy|Redmi|Xiaomi|POCO|Huawei|Pixel|OnePlus|Oppo|Motorola|moto|Honor|Realme)|\b(Hotspot|Tethering|AndroidAP)\b') { Say ('La rete "' + $name + '" sembra l''hotspot di un telefono: lì il segno "a consumo" serve a non finire i giga. La lascio così.'); return }
if ($name -match '"') { Open-User 'ms-settings:network-wifi'; Say 'Toglilo a mano nella finestra che ti apro: nome della rete > Connessione a consumo: No.'; return }
$c = Run-Hidden 'netsh.exe' ('wlan set profileparameter name="' + $name + '" cost=Unrestricted')
if ($c -eq 0) { Say ('Fatto: la rete "' + $name + '" non è più a consumo. Aggiornamenti e OneDrive ripartono.'); Say ('Per rimetterlo: Impostazioni > Rete e Internet > Wi-Fi > "' + $name + '" > Connessione a consumo.') }
else { Say 'Windows non mi ha lasciato cambiarlo (forse lo decide l''ufficio). Prova a mano: nome della rete > Connessione a consumo: No.'; Open-User 'ms-settings:network-wifi' }
`)},
	{ID: "of_net_slow", Name: "Internet è lento solo su questo PC", Desc: "Controlla segnale Wi-Fi, velocità del cavo, VPN e i programmi che scaricano o caricano in sottofondo (OneDrive, aggiornamenti). Ti dice cosa rallenta. Controlla e spiega.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs2(`
$wi = (netsh.exe wlan show interfaces 2>$null) -join [char]10
if ($wi -match '(?:Segnale|Signal)\s*:\s*(\d+)%') { $sig = [int]$Matches[1]; Say ('Segnale Wi-Fi: ' + $sig + '%.'); if ($sig -lt 50) { Say 'È debole: avvicinati al router, o usa il cavo. Muri e armadi di metallo lo bloccano molto.' } }
if ($wi -match '(?:Banda|Band)\s*:\s*2[.,]4') { Say 'Sei collegato al Wi-Fi a 2,4 GHz (più lento e affollato): se il router ha la rete "5G" o "5GHz", collegati a quella.' }
elseif ($wi -match '(?:Tipo radio|Tipo di radio|Radio type)\s*:\s*802\.11(b|g)\b') { Say 'Sei collegato al Wi-Fi in un modo vecchio e lento (802.11' + $Matches[1] + '): se il router ha la rete "5G" o "5GHz", collegati a quella.' }
foreach ($a in @(Get-NetAdapter -Physical -ErrorAction SilentlyContinue | Where-Object { [string]$_.Status -eq 'Up' -and $_.InterfaceDescription -notmatch 'Wi-?Fi|Wireless|WLAN|802\.11' })) { if ([string]$a.LinkSpeed -match '^(10|100) Mbps') { Say ('Il cavo va solo a ' + $a.LinkSpeed + ': cambialo, dovrebbe andare a 1 Gbps.') } }
$vpn = @(Get-NetAdapter -ErrorAction SilentlyContinue | Where-Object { [string]$_.Status -eq 'Up' -and $_.InterfaceDescription -match 'VPN|TAP-|WireGuard|Fortinet|Cisco|AnyConnect|GlobalProtect|PANGP|Pulse|SonicWall|OpenVPN|Check Point' })
if ($vpn.Count) { Say 'La VPN è accesa: tutto passa dal server dell''ufficio, ed è normale che vada più piano. Spegnila quando non ti serve.' }
$busy = @()
$names = @{ OneDrive = 'OneDrive'; Dropbox = 'Dropbox'; GoogleDriveFS = 'Google Drive'; 'ms-teams' = 'Teams'; steam = 'Steam'; EpicGamesLauncher = 'Epic Games'; qbittorrent = 'qBittorrent'; uTorrent = 'uTorrent'; BitTorrent = 'BitTorrent' }
foreach ($n in $names.Keys) { if (Get-Process -Name $n -ErrorAction SilentlyContinue) { $busy += $names[$n] } }
if ($busy.Count) { Say ('Programmi aperti che possono usare internet in sottofondo: ' + ($busy -join ', ') + '. Se OneDrive sta caricando tanti file, la nuvoletta gira: aspetta o mettilo in pausa per 2 ore.') }
$wu = @(Get-Process -Name TiWorker, MoUsoCoreWorker -ErrorAction SilentlyContinue)
if ($wu.Count) { Say 'Windows Update sta lavorando: quando finisce, internet torna veloce.' }
Say 'Per misurare la velocità vera: in Tasso, pagina Internet > Test di velocità. Se anche il telefono sullo stesso Wi-Fi va piano, il problema è il router o la linea, non il PC.'
`)},
	{ID: "of_rdp", Name: "Desktop remoto: schermo nero, lento o non si collega", Desc: "Svuota la memoria delle immagini del Desktop remoto (causa schermi neri e pezzi di finestra sbagliati) e controlla se i computer a cui ti colleghi rispondono. Le connessioni salvate restano.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs2(`
if (Get-Process -Name mstsc -ErrorAction SilentlyContinue) { Say 'Il Desktop remoto è aperto: chiudi la connessione (salva il lavoro sul PC remoto) e premi di nuovo Risolvi.'; return }
$f = Clear-Folder (Join-Path $env:LOCALAPPDATA 'Microsoft\Terminal Server Client\Cache')
Freed $f
Say ('Memoria delle immagini del Desktop remoto svuotata (' + (Fmt $f) + ').')
$mru = @()
foreach ($i in 0..9) { $v = [string](Get-RegValue 'HKCU\Software\Microsoft\Terminal Server Client\Default' ('MRU' + $i)); if ($v) { $mru += $v } }
if (-not $mru.Count) { Say 'Non ci sono connessioni recenti salvate da controllare.' }
foreach ($m in ($mru | Select-Object -First 3)) {
  $h = $m; $port = 3389
  if ($m -match '^(.+):(\d+)$') { $h = $Matches[1]; $port = [int]$Matches[2] }
  if (Test-Tcp $h $port 3000) { Say ($m + ': risponde.') } else { Say ($m + ': NON risponde. È acceso? Serve la VPN dell''ufficio? Se lavori da casa, prima accendi la VPN.') }
}
Say 'Se resta lo schermo nero dopo il collegamento: premi Ctrl + Alt + Fine, poi Annulla: di solito ricompare. Se è lento: nelle opzioni del Desktop remoto > Esperienza, scegli una connessione più lenta.'
`)},
	{ID: "of_hosts", Name: "Un sito di lavoro è bloccato senza motivo", Desc: "Controlla il file \"hosts\" di Windows, dove virus o vecchi programmi possono bloccare o deviare siti come Microsoft, Google o la banca. Mette da parte solo le righe sospette, con copia di sicurezza.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$hp = Join-Path $env:SystemRoot 'System32\drivers\etc\hosts'
if (-not (Test-Path -LiteralPath $hp)) { Say 'Il file hosts non c''è: va bene così.'; return }
$lines = @(Get-Content -LiteralPath $hp -ErrorAction SilentlyContinue)
$act = @($lines | Where-Object { $_ -match '^\s*[0-9a-fA-F:.]+\s+\S' -and $_ -notmatch '^\s*(127\.0\.0\.1|::1)\s+localhost\s*$' })
if (-not $act.Count) { Say 'Il file hosts è pulito: nessun sito è bloccato o deviato da lì.'; Say 'Se un sito non si apre: prova un altro browser, "L''ora è sbagliata" o "Internet c''è ma i siti non si aprono".'; return }
$imp = '(^|\.)(microsoft|windows|windowsupdate|office|office365|live|outlook|msn|bing|skype|sharepoint|onedrive|google|gmail|youtube|apple|icloud|amazon|paypal|zoom|webex|adobe|aruba|poste|intesasanpaolo|unicredit|bper|bancobpm|fineco|inps|agenziaentrate|kaspersky|eset|avast|avg|norton|mcafee|bitdefender|malwarebytes|virustotal)\.(com|it|net|org|eu)$|(^|\.)(gov\.it|teams\.microsoft\.com)$'
$hostsOf = { param($l) @(($l.Trim() -split '#', 2)[0].Trim() -split '\s+' | Select-Object -Skip 1) }
$priv = '^(10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.)'
$bad = @($act | Where-Object { $ip = ($_.Trim() -split '\s+')[0]; ($ip -notmatch $priv) -and @(& $hostsOf $_ | Where-Object { $_ -match $imp }).Count })
Say ('Nel file hosts ci sono ' + $act.Count + ' righe attive (quelle dell''ufficio per i server interni sono normali).')
if (-not $bad.Count) { Say 'Nessuna blocca o devia siti importanti (Microsoft, Google, banche, antivirus, PA) verso indirizzi esterni. Non tocco niente.'; return }
$bk = $hp + '.tasso_' + (Get-Date -Format 'yyyyMMdd_HHmm') + '.bak'
try { Copy-Item -LiteralPath $hp -Destination $bk -ErrorAction Stop } catch { Say 'Non sono riuscito a fare la copia di sicurezza, quindi non cambio niente.'; return }
$new = $lines | ForEach-Object { if ($bad -contains $_) { '# disattivata da Tasso: ' + $_ } else { $_ } }
try { [IO.File]::WriteAllLines($hp, [string[]]$new) } catch { Say ('Windows (o l''antivirus) non mi ha lasciato modificarlo. Copia di sicurezza: ' + $bk); return }
[void](Run-Hidden 'ipconfig.exe' '/flushdns')
Say ('Disattivate ' + $bad.Count + ' righe che bloccavano o deviavano siti importanti: ' + ((($bad | ForEach-Object { (& $hostsOf $_) -join ' ' }) | Select-Object -First 6) -join ', ') + '.')
Say 'Righe così le mettono di solito virus o programmi "crackati": fai anche "Controllo dei programmi sospetti".'
Say ('Copia di sicurezza del file com''era: ' + $bk)
`)},
	{ID: "of_static_ip", Name: "Il portatile non va su internet in un'altra rete", Desc: "Se la scheda di rete ha un indirizzo o un DNS messi a mano (di un'altra rete) e qui non funzionano, li rimette automatici. Salva prima i valori vecchi in un file, per rimetterli in ufficio.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs2(`
$done = 0
foreach ($a in @(Get-NetAdapter -Physical -ErrorAction SilentlyContinue | Where-Object { [string]$_.Status -eq 'Up' })) {
  $if = Get-NetIPInterface -InterfaceIndex $a.ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue
  $cfg = Get-NetIPConfiguration -InterfaceIndex $a.ifIndex -ErrorAction SilentlyContinue
  $gw = @($cfg.IPv4DefaultGateway | ForEach-Object { $_.NextHop })
  $dns = @($cfg.DNSServer | Where-Object { $_.AddressFamily -eq 2 } | ForEach-Object { $_.ServerAddresses })
  $guid = [string]$a.InterfaceGuid
  $ns = [string](Get-RegValue ('HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\' + $guid) 'NameServer')
  $staticIp = ([string]$if.Dhcp -eq 'Disabled')
  if (-not $staticIp -and -not $ns) { continue }
  $works = (Test-Tcp '1.1.1.1' 443) -or (Test-Tcp '8.8.8.8' 53) -or (Test-Tcp '9.9.9.9' 443)
  $dnsOk = $true; try { [void](Resolve-DnsName 'www.microsoft.com' -DnsOnly -QuickTimeout -ErrorAction Stop) } catch { $dnsOk = $false }
  if ($works -and $dnsOk) { Say ($a.Name + ': ha impostazioni messe a mano, ma funzionano: non tocco niente (in ufficio possono servire).'); continue }
  $dir = Join-Path ([Environment]::GetFolderPath('MyDocuments')) 'Tasso'
  New-Item -ItemType Directory -Path $dir -Force | Out-Null
  $f = Join-Path $dir ('rete-' + ($a.Name -replace '[^\w-]', '_') + '-' + (Get-Date -Format 'yyyyMMdd_HHmm') + '.txt')
  $ips = @(Get-NetIPAddress -InterfaceIndex $a.ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue | ForEach-Object { $_.IPAddress + '/' + $_.PrefixLength })
  @('Impostazioni di rete messe a mano, salvate da Tasso il ' + (Get-Date -Format 'dd/MM/yyyy HH:mm'), ('Scheda: ' + $a.Name), ('Indirizzo IP: ' + ($ips -join ', ')), ('Gateway: ' + ($gw -join ', ')), ('DNS: ' + ($dns -join ', ')), '', 'Per rimetterle: Impostazioni > Rete e Internet > (Wi-Fi o Ethernet) > la tua rete > Assegnazione IP / DNS > Modifica > Manuale.') | Set-Content -LiteralPath $f -Encoding UTF8
  if ($staticIp -and -not $works) {
    Remove-NetRoute -InterfaceIndex $a.ifIndex -DestinationPrefix '0.0.0.0/0' -Confirm:$false -ErrorAction SilentlyContinue
    Remove-NetIPAddress -InterfaceIndex $a.ifIndex -AddressFamily IPv4 -PrefixOrigin Manual -Confirm:$false -ErrorAction SilentlyContinue
    Set-NetIPInterface -InterfaceIndex $a.ifIndex -Dhcp Enabled -ErrorAction SilentlyContinue
  }
  Set-DnsClientServerAddress -InterfaceIndex $a.ifIndex -ResetServerAddresses -ErrorAction SilentlyContinue
  [void](Run-Hidden 'ipconfig.exe' '/renew')
  Start-Sleep -Seconds 4
  $done++
  Say ($a.Name + ': aveva ' + $(if ($staticIp) { 'indirizzo e DNS' } else { 'il DNS' }) + ' messi a mano che qui non funzionavano: ora sono automatici. I valori vecchi sono salvati in ' + $f)
}
if (-not $done) { Say 'Le schede di rete sono già in automatico (o le impostazioni a mano funzionano): non era questo il problema.'; return }
if (Test-Internet) { Say 'Internet funziona.' } else { Say 'Internet ancora non va: prova "Il Wi-Fi non si collega" o "Risolvi problemi di internet".' }
Say 'Quando torni in ufficio, se lì serviva l''indirizzo fisso, rimettilo con i valori del file (o chiedi all''assistenza).'
`)},

	// ---------- Schermi e USB ----------
	{ID: "of_screen_off", Name: "Lo schermo si spegne o si blocca troppo presto", Desc: "Se lo schermo si spegne o chiede la password dopo 1-2 minuti, porta l'attesa a 10 minuti con la corrente e 5 a batteria. Ti dice anche se è l'ufficio a deciderlo.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$v = Get-PowerValue 'SUB_VIDEO' 'VIDEOIDLE'
$ch = @()
if ($v.Count -eq 2) {
  if ($v[0] -gt 0 -and $v[0] -lt 180) { [void](Run-Hidden 'powercfg.exe' '/setacvalueindex SCHEME_CURRENT SUB_VIDEO VIDEOIDLE 600'); $ch += ('con la corrente: da ' + $v[0] + ' secondi a 10 minuti') }
  if ($v[1] -gt 0 -and $v[1] -lt 120) { [void](Run-Hidden 'powercfg.exe' '/setdcvalueindex SCHEME_CURRENT SUB_VIDEO VIDEOIDLE 300'); $ch += ('a batteria: da ' + $v[1] + ' secondi a 5 minuti') }
  if ($ch.Count) { [void](Run-Hidden 'powercfg.exe' '/setactive SCHEME_CURRENT') }
}
$pol = Get-RegValue 'HKCU\Software\Policies\Microsoft\Windows\Control Panel\Desktop' 'ScreenSaveTimeOut'
$ina = Get-RegValue 'HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System' 'InactivityTimeoutSecs'
$k = 'HKCU\Control Panel\Desktop'
$ss = [int](Get-RegValue $k 'ScreenSaveTimeOut')
if (-not $pol -and (Get-RegValue $k 'ScreenSaveActive') -eq '1' -and $ss -gt 0 -and $ss -lt 120) { $u = Save-Undo $k @('ScreenSaveTimeOut') 'salvaschermo'; Set-RegValue $k 'ScreenSaveTimeOut' '600' 'String'; $ch += ('salvaschermo: da ' + $ss + ' secondi a 10 minuti'); Say-Undo $u }
if ($ch.Count) { Say ('Fatto: ' + ($ch -join '; ') + '.') } else { Say 'Lo schermo non si spegneva troppo presto per le impostazioni di Windows.' }
if ($pol -or $ina) { Say 'Il blocco dopo pochi minuti lo ha deciso l''ufficio (per sicurezza): non si può cambiare dal PC. Ricordati Win + L quando ti alzi.' }
Say 'Per cambiare i tempi a piacere: Impostazioni > Sistema > Alimentazione > Schermo e sospensione.'
`)},
	{ID: "of_brightness", Name: "La luminosità cambia da sola", Desc: "Spegne la luminosità automatica di Windows (quella che scurisce lo schermo con il sensore di luce) e ti dice cosa altro la cambia. Puoi riaccenderla dalle impostazioni.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$v = Get-PowerValue 'SUB_VIDEO' 'ADAPTBRIGHT'
if ($v.Count -eq 2 -and ($v[0] -ne 0 -or $v[1] -ne 0)) {
  [void](Run-Hidden 'powercfg.exe' '/setacvalueindex SCHEME_CURRENT SUB_VIDEO ADAPTBRIGHT 0')
  [void](Run-Hidden 'powercfg.exe' '/setdcvalueindex SCHEME_CURRENT SUB_VIDEO ADAPTBRIGHT 0')
  [void](Run-Hidden 'powercfg.exe' '/setactive SCHEME_CURRENT')
  Say 'Spenta la luminosità automatica: lo schermo non si scurisce più da solo con la luce della stanza.'
  Say 'Per riaccenderla: Impostazioni > Sistema > Schermo > Luminosità > "Modifica automaticamente la luminosità".'
} elseif ($v.Count -eq 2) { Say 'La luminosità automatica di Windows era già spenta.' }
else { Say 'Questo PC non ha la luminosità automatica (succede con i monitor da scrivania).' }
Say 'Altre cose che cambiano la luminosità: il "Risparmio energia" a batteria (la abbassa), e su alcuni portatili il programma della scheda video (Intel: "Risparmio energetico display"; AMD: "Vari-Bright"). Spegnili lì se succede ancora.'
Open-User 'ms-settings:display'
`)},
	{ID: "of_snip", Name: "Win + Maiusc + S (cattura dello schermo) non funziona", Desc: "Controlla che lo Strumento di cattura ci sia e funzioni, fa catturare lo schermo anche con il tasto Stamp e ti dice dove finiscono le catture. Puoi tornare indietro.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs2(`
if ((Get-RegValue 'HKLM\SOFTWARE\Policies\Microsoft\TabletPC' 'DisableSnippingTool') -eq 1) { Say 'Lo Strumento di cattura è stato spento dall''ufficio: non si può riaccendere dal PC. Chiedi all''assistenza.'; return }
$pk = @(Get-AppxPackage -Name 'Microsoft.ScreenSketch' -ErrorAction SilentlyContinue)
if (-not $pk.Count) { Say 'Lo Strumento di cattura non è installato: te lo apro nel Microsoft Store (è gratis, di Microsoft). Installalo e riprova.'; Open-User 'ms-windows-store://pdp/?productid=9MZ95KL8MR0L'; return }
Get-Process -Name ScreenSketch, SnippingTool, ScreenClippingHost -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
$man = Join-Path $pk[0].InstallLocation 'AppxManifest.xml'
if (Test-Path -LiteralPath $man) { try { Add-AppxPackage -DisableDevelopmentMode -Register $man -ErrorAction Stop; Say 'Strumento di cattura rimesso a posto.' } catch {} }
$k = 'HKCU\Control Panel\Keyboard'
if ((Get-RegValue $k 'PrintScreenKeyForSnippingEnabled') -ne 1) { $u = Save-Undo $k @('PrintScreenKeyForSnippingEnabled') 'tasto-stamp'; Set-RegValue $k 'PrintScreenKeyForSnippingEnabled' 1 'DWord'; Say 'Ora anche il tasto Stamp (PrtSc) apre la cattura dello schermo (vale dal prossimo accesso a Windows).'; Say-Undo $u }
Say 'Dove finiscono le catture: vengono copiate (incollale con Ctrl + V in Word, Outlook, Teams) e in Windows 11 si salvano anche in Immagini > Screenshot.'
Say 'Se premendo Win + Maiusc + S non succede niente: controlla che il tasto Windows funzioni (c''è "Il tasto Windows non funziona").'
`)},
	{ID: "of_usb_drop", Name: "Mouse, chiavette o periferiche USB si staccano da sole", Desc: "Spegne il risparmio energetico delle porte USB, la causa più comune di mouse, tastiere, chiavette e docking che si staccano. Consuma pochissimo in più. Puoi tornare indietro.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs2(`
$v = Get-PowerValue '2a737441-1930-4402-8d77-b2bebba308a3' '48e6b7a6-50f5-4782-a5d4-53bb8f07e226'
if ($v.Count -eq 2 -and ($v[0] -ne 0 -or $v[1] -ne 0)) {
  [void](Run-Hidden 'powercfg.exe' '/setacvalueindex SCHEME_CURRENT 2a737441-1930-4402-8d77-b2bebba308a3 48e6b7a6-50f5-4782-a5d4-53bb8f07e226 0')
  [void](Run-Hidden 'powercfg.exe' '/setdcvalueindex SCHEME_CURRENT 2a737441-1930-4402-8d77-b2bebba308a3 48e6b7a6-50f5-4782-a5d4-53bb8f07e226 0')
  [void](Run-Hidden 'powercfg.exe' '/setactive SCHEME_CURRENT')
  Say 'Spenta la "sospensione selettiva USB": Windows non spegne più le porte USB per risparmiare.'
} else { Say 'La sospensione selettiva USB era già spenta.' }
$n = 0
foreach ($d in @(Get-CimInstance -Namespace root\wmi -ClassName MSPower_DeviceEnable -ErrorAction SilentlyContinue | Where-Object { $_.InstanceName -match '^USB\\ROOT_HUB' -and $_.Enable })) { try { Set-CimInstance -InputObject $d -Property @{ Enable = $false } -ErrorAction Stop; $n++ } catch {} }
if ($n) { Say ('Anche ' + $n + ' porte USB principali non verranno più spente per risparmiare energia.') }
$r = Restart-BadDevices 'USB'
Say 'Se si staccano ancora: prova un''altra porta (meglio quelle dietro sul fisso), evita le prolunghe e gli hub senza alimentatore, e per i mouse senza filo cambia la pila.'
Say 'Per tornare come prima: Pannello di controllo > Opzioni risparmio energia > Modifica impostazioni avanzate > Impostazioni USB > Sospensione selettiva: Attivata. Per le porte: Gestione dispositivi > Controller USB > Hub radice USB > Risparmio energia > spunta "Consenti al computer di spegnere il dispositivo".'
`)},

	// ---------- Mouse, tastiera e portatile ----------
	{ID: "of_mouse", Name: "Il mouse va troppo piano, troppo veloce o il doppio clic non va", Desc: "Rimette velocità del puntatore, doppio clic e rotellina come quando Windows è nuovo (con copia di sicurezza) e ti dice se i tasti del mouse sono invertiti.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$b1 = Backup-Reg 'HKCU\Control Panel\Mouse' 'mouse'
$b2 = Backup-Reg 'HKCU\Control Panel\Desktop' 'mouse-rotellina'
if (-not $b1 -or -not $b2) { Say 'Non sono riuscito a fare la copia di sicurezza, quindi non cambio niente. Ti apro le impostazioni del mouse.'; Open-User 'ms-settings:mousetouchpad'; return }
if (-not (Use-Input)) { Say 'Windows non mi ha lasciato cambiare le impostazioni del mouse. Te le apro.'; Open-User 'ms-settings:mousetouchpad'; return }
[TassoInput]::Mouse()
Say 'Fatto: velocità del puntatore media, doppio clic normale, "precisione del puntatore" accesa e rotellina a 3 righe. Si vede subito. (Se giochi e avevi spento la precisione per la mira, rispegnila con "La mira con il mouse non è precisa".)'
if ((Get-RegValue 'HKCU\Control Panel\Mouse' 'SwapMouseButtons') -eq '1') { Say 'I tasti del mouse sono invertiti (impostazione per mancini): se non l''hai scelto tu, Impostazioni > Bluetooth e dispositivi > Mouse > "Pulsante principale del mouse": Sinistro.' }
Say 'Se il puntatore salta o si blocca: mouse senza filo con la pila scarica o ricevitore lontano (mettilo in una porta davanti), oppure il tappetino/la superficie (vetro e superfici lucide non vanno bene).'
Say ('Per tornare come prima: doppio clic su ' + $b1 + ' e su ' + $b2 + ', poi esci e rientra in Windows.')
`)},
	{ID: "of_touchpad", Name: "Il touchpad del portatile non funziona", Desc: "Riaccende il touchpad se era spento in Gestione dispositivi, riavvia il suo driver e ti dice dove si accende in Windows e quale tasto Fn lo spegne.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs2(`
$tp = @(Get-PnpDevice -PresentOnly -ErrorAction SilentlyContinue | Where-Object { $_.Class -match 'Mouse|HIDClass' -and $_.FriendlyName -match 'touch ?pad|TouchPad|Precision|I2C HID|Synaptics|ELAN|Alps' })
if (-not $tp.Count) { Say 'Windows non vede nessun touchpad. Se è un portatile: riavvia il PC; se non torna, manca il driver (in Tasso: Aggiornamento driver) o è un problema del touchpad.'; return }
foreach ($d in $tp) {
  if ([string]$d.Status -ne 'OK') {
    $id = [string]$d.InstanceId
    if ($id -match '^[A-Za-z0-9\\&_.{}#\- ]{3,300}$') {
      [void](Run-Hidden 'pnputil.exe' ('/enable-device "' + $id + '"'))
      [void](Run-Hidden 'pnputil.exe' ('/restart-device "' + $id + '"'))
      Say ('Riacceso e riavviato: ' + $d.FriendlyName)
    }
  }
}
Say 'Se ancora non va:'
Say '1) Molti portatili hanno un tasto che lo spegne: cerca sui tasti F1-F12 il disegno di un rettangolo barrato e premilo (a volte insieme a Fn).'
Say '2) In Windows deve essere acceso, e se usi anche un mouse controlla "Lascia attivo il touchpad quando è collegato un mouse". Te lo apro.'
Open-User 'ms-settings:devices-touchpad'
`)},
	{ID: "of_numlock", Name: "Il tastierino numerico è spento quando accendo il PC", Desc: "Fa partire Windows con il Bloc Num acceso, così il PIN e i numeri a destra della tastiera funzionano subito. Puoi tornare indietro.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$k1 = 'HKU\.DEFAULT\Control Panel\Keyboard'; $k2 = 'HKCU\Control Panel\Keyboard'
$v1 = [string](Get-RegValue $k1 'InitialKeyboardIndicators'); $v2 = [string](Get-RegValue $k2 'InitialKeyboardIndicators')
$on = { param($v) try { return (([long]$v) -band 2) -ne 0 } catch { return $false } }
if ((& $on $v1) -and (& $on $v2)) { Say 'Il Bloc Num era già impostato acceso all''avvio.'; Say 'Se si spegne lo stesso, lo decide il BIOS del PC (voce "NumLock" o "Boot Up NumLock State") o l''Avvio rapido: prova "Spegnendo il PC i problemi restano".'; return }
$u1 = Save-Undo $k1 @('InitialKeyboardIndicators') 'bloc-num-avvio'
$u2 = Save-Undo $k2 @('InitialKeyboardIndicators') 'bloc-num-utente'
Set-RegValue $k1 'InitialKeyboardIndicators' '2147483650' 'String'
Set-RegValue $k2 'InitialKeyboardIndicators' '2' 'String'
Say 'Fatto: alla prossima accensione il tastierino numerico parte acceso (anche nella schermata del PIN).'
Say 'Sui portatili senza tastierino a parte non serve: lì i numeri sono sopra le lettere.'
Say-Undo $u1; Say-Undo $u2
`)},
	{ID: "of_sticky", Name: "La tastiera fa bip, scrive lento o i tasti restano premuti", Desc: "Spegne Tasti permanenti, Filtro tasti e Segnali acustici (si accendono per sbaglio premendo Maiusc 5 volte o tenendolo premuto) e le loro scorciatoie. Con copia di sicurezza.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$b = Backup-Reg 'HKCU\Control Panel\Accessibility' 'tasti-accessibilita'
if (-not $b) { Say 'Non sono riuscito a fare la copia di sicurezza, quindi non cambio niente. Ti apro le impostazioni della tastiera.'; Open-User 'ms-settings:easeofaccess-keyboard'; return }
if (-not (Use-Input)) { Say 'Windows non mi ha lasciato cambiarle: te le apro.'; Open-User 'ms-settings:easeofaccess-keyboard'; return }
$r = [TassoInput]::Keys()
$m = @{}; foreach ($p in ($r -split ';')) { $kv = $p -split '=', 2; if ($kv.Count -eq 2) { $m[$kv[0]] = $kv[1] } }
$names = @{ sticky = 'Tasti permanenti (i tasti Maiusc, Ctrl e Alt restano premuti)'; filter = 'Filtro tasti (la tastiera scrive lenta o ignora i tasti)'; toggle = 'Segnali acustici (bip con Bloc Maiusc e Bloc Num)' }
$was = @(); $hk = @()
foreach ($x in 'sticky', 'filter', 'toggle') { if ($m[$x] -eq 'on') { $was += $names[$x] } elseif ($m[$x] -eq 'hk') { $hk += $x } }
if ($was.Count) { Say ('Erano accesi e li ho spenti: ' + ($was -join '; ') + '.') } else { Say 'Tasti permanenti, Filtro tasti e Segnali acustici erano spenti.' }
if ($was.Count -or $hk.Count) { Say 'Spente anche le scorciatoie che li accendono per sbaglio (Maiusc 5 volte, Maiusc tenuto premuto 8 secondi).' }
Say 'Se un tasto resta davvero incastrato o scrive da solo, è la tastiera: pulisci sotto il tasto o prova un''altra tastiera.'
Say ('Per tornare come prima: doppio clic su ' + $b + ', poi esci e rientra in Windows.')
`)},
	{ID: "of_win_key", Name: "Il tasto Windows non funziona", Desc: "Controlla se il tasto Windows è bloccato da un'impostazione e lo sblocca (se non l'ha deciso l'ufficio), con copia di sicurezza. Ti dice anche il blocco \"Gaming\" che hanno molte tastiere.", Level: 0, Minutes: 0.2, Timeout: time.Minute, Script: ofs2(`
$fixed = @(); $reboot = $false
$k = 'HKCU\Software\Microsoft\Windows\CurrentVersion\Policies\Explorer'
if ((Get-RegValue $k 'NoWinKeys') -eq 1) {
  $u = Save-Undo $k @('NoWinKeys') 'tasto-windows'
  Remove-ItemProperty -LiteralPath ('Registry::' + (Get-FullKey $k)) -Name NoWinKeys -ErrorAction SilentlyContinue
  $fixed += 'blocco delle scorciatoie con il tasto Windows'; Say-Undo $u
  if ([bool](Get-CimInstance Win32_ComputerSystem -ErrorAction SilentlyContinue).PartOfDomain) { Say 'Questo PC è dell''ufficio: se il blocco torna, lo rimette l''ufficio apposta.' }
}
$sk = 'HKLM\SYSTEM\CurrentControlSet\Control\Keyboard Layout'
$map = Get-RegValue $sk 'Scancode Map'
if ($map) {
  $hex = (@($map) | ForEach-Object { '{0:x2}' -f $_ }) -join ''
  if ($hex -match '00005be0|00005ce0') {
    $b = Backup-Reg $sk 'tasto-windows-tastiera'
    if ($b) { Remove-ItemProperty -LiteralPath ('Registry::' + (Get-FullKey $sk)) -Name 'Scancode Map' -ErrorAction SilentlyContinue; $fixed += 'tasto Windows spento da un programma (vale dopo il riavvio)'; $reboot = $true; Say 'Nota: se avevi cambiato anche altri tasti con un programma (per esempio Bloc Maiusc), quella modifica è stata tolta insieme.'; Say ('Per tornare come prima: doppio clic su ' + $b) }
  }
}
if ($fixed.Count) { Say ('Sbloccato: ' + ($fixed -join '; ') + '.') } else { Say 'In Windows il tasto non è bloccato.' }
Say 'Molte tastiere (soprattutto da gioco) hanno un tasto o una lucina "Win Lock" o "Game Mode", oppure si sblocca con Fn + tasto Windows: premili.'
Say 'Se il tasto funziona ma il menu Start non si apre, usa "Risolvi problemi di ricerca e menu Start".'
if ($reboot) { [Console]::Out.WriteLine('REBOOT') }
`)},
	{ID: "of_battery", Name: "La batteria del portatile dura poco", Desc: "Legge quanto si è consumata la batteria rispetto a nuova e i cicli di ricarica, e ti dice cosa la scarica e se è da cambiare. Controlla e spiega.", Level: 0, Minutes: 0.3, Timeout: 2 * time.Minute, Script: ofs2(`
if (-not @(Get-CimInstance Win32_Battery -ErrorAction SilentlyContinue).Count) { Say 'Questo PC non ha una batteria (è un fisso, o la batteria non viene vista).'; return }
$x = Join-Path $env:TEMP ('tasso-batteria-' + [guid]::NewGuid().ToString('N') + '.xml')
[void](Run-Hidden 'powercfg.exe' ('/batteryreport /xml /output "' + $x + '"'))
$r = $null; try { $r = [xml](Get-Content -LiteralPath $x -Raw -ErrorAction Stop) } catch {}
Remove-Item -LiteralPath $x -Force -ErrorAction SilentlyContinue
if ($r) {
  foreach ($b in @($r.BatteryReport.Batteries.Battery)) {
    $des = [double]$b.DesignCapacity; $full = [double]$b.FullChargeCapacity
    if ($des -gt 0 -and $full -gt 0) {
      $pct = [int][math]::Round($full * 100 / $des)
      Say ('Salute della batteria: ' + $pct + '% di quando era nuova' + $(if ($b.CycleCount -and [int]$b.CycleCount -gt 0) { ' (' + $b.CycleCount + ' cicli di ricarica)' } else { '' }) + '.')
      if ($pct -lt 50) { Say 'È molto consumata: va cambiata. Con il modello del portatile un tecnico la sostituisce, di solito tra 50 e 120 euro.' }
      elseif ($pct -lt 75) { Say 'È un po'' consumata: è normale dopo 2-3 anni. Dura meno, ma va ancora.' }
      else { Say 'La batteria è in buono stato: se dura poco è per come viene usato il PC.' }
    }
  }
} else { Say 'Windows non mi ha dato i dati della batteria.' }
Say 'Cosa la scarica di più: la luminosità alta, il Wi-Fi e il Bluetooth sempre accesi, Teams e le videochiamate, il browser con tante schede.'
Say 'Consigli: accendi "Risparmio energia" quando scende sotto il 30% (centro notifiche in basso a destra), abbassa la luminosità di un paio di tacche e chiudi le schede che non usi.'
Say 'Se il portatile sta sempre in carica sulla scrivania, molti hanno la funzione "carica fino all''80%" (nel programma del produttore: Lenovo Vantage, MyASUS, Dell Power Manager, HP): la fa durare più anni.'
Open-User 'ms-settings:batterysaver'
`)},
	{ID: "of_clipboard_history", Name: "Voglio ritrovare le cose copiate prima (Win + V)", Desc: "Accende la cronologia degli appunti: con Win + V ritrovi le ultime cose copiate, testi e immagini. Resta solo su questo PC. Puoi spegnerla quando vuoi.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
if ((Get-RegValue 'HKLM\SOFTWARE\Policies\Microsoft\Windows\System' 'AllowClipboardHistory') -eq 0) { Say 'La cronologia degli appunti è stata spenta dall''ufficio: non si può accendere dal PC.'; return }
$k = 'HKCU\Software\Microsoft\Clipboard'
if ((Get-RegValue $k 'EnableClipboardHistory') -eq 1) { Say 'La cronologia degli appunti era già accesa: premi Win + V per vederla.' }
else { $u = Save-Undo $k @('EnableClipboardHistory') 'cronologia-appunti'; Set-RegValue $k 'EnableClipboardHistory' 1 'DWord'; Say 'Fatto: da ora premi Win + V e ritrovi le ultime 25 cose copiate (da adesso in poi). Clicca su una per incollarla.'; Say-Undo $u }
Say 'Con la puntina fissi le cose che incolli spesso (indirizzo, IBAN, firma): restano anche dopo il riavvio.'
Say 'Attenzione: se copi una password finisce nella cronologia. Per toglierla: Win + V > tre puntini > Elimina (o "Cancella tutto").'
`)},

	// ---------- Windows ----------
	{ID: "of_desktop_icons", Name: "Le icone del desktop sono sparite", Desc: "Rifà vedere le icone del desktop se erano nascoste, rimette il Cestino (e Questo PC, se era stato tolto) e ti dice se il desktop è finito dentro OneDrive. Puoi tornare indietro.", Level: 0, Minutes: 0.2, Timeout: time.Minute, Script: ofs2(`
$ch = @()
$k = 'HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced'
if ((Get-RegValue $k 'HideIcons') -eq 1) { $u = Save-Undo $k @('HideIcons') 'icone-desktop'; Set-RegValue $k 'HideIcons' 0 'DWord'; $ch += 'le icone del desktop erano nascoste (Visualizza > Mostra icone del desktop)'; Say-Undo $u }
$hk = 'HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\HideDesktopIcons\NewStartPanel'
$ids = @{ '{20D04FE0-3AEA-1069-A2D8-08002B30309D}' = 'Questo PC'; '{645FF040-5081-101B-9F08-00AA002F954E}' = 'Cestino' }
$back = @()
foreach ($id in $ids.Keys) { if ((Get-RegValue $hk $id) -eq 1) { $back += $id } }
if ($back.Count) { $u = Save-Undo $hk $back 'icone-sistema-desktop'; foreach ($id in $back) { Set-RegValue $hk $id 0 'DWord' }; $ch += ('rimesse sul desktop: ' + (($back | ForEach-Object { $ids[$_] }) -join ' e ')); Say-Undo $u }
if ((Get-RegValue 'HKCU\Software\Microsoft\Windows\CurrentVersion\ImmersiveShell' 'TabletMode') -eq 1) { Say 'Il PC è in "modalità tablet", che nasconde il desktop: spegnila dal centro notifiche in basso a destra.' }
if ($ch.Count) { Say ('Sistemato: ' + ($ch -join '; ') + '. Riavvio Esplora file...'); Restart-Explorer } else { Say 'Le icone del desktop non erano nascoste.' }
$desk = [Environment]::GetFolderPath('Desktop')
$n = @(Get-ChildItem -LiteralPath $desk -Force -ErrorAction SilentlyContinue | Where-Object { $_.Name -ne 'desktop.ini' }).Count
if ($desk -match 'OneDrive') { Say ('Il tuo desktop è dentro OneDrive (' + $desk + '): ci sono ' + $n + ' elementi. Se ne mancano, guarda in OneDrive online o nel suo Cestino; se sei su un altro PC, i file si scaricano quando OneDrive ha finito di sincronizzare.') }
else { Say ('Sul desktop ci sono ' + $n + ' elementi (in ' + $desk + ').') }
`)},
	{ID: "of_taskbar_hidden", Name: "La barra in basso si nasconde da sola", Desc: "Spegne \"Nascondi automaticamente la barra delle applicazioni\", così la barra con Start e i programmi aperti resta sempre visibile. Puoi tornare indietro.", Level: 0, Minutes: 0.2, Timeout: time.Minute, Script: ofs2(`
$k = 'HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\StuckRects3'
$v = Get-RegValue $k 'Settings'
if (-not $v -or @($v).Count -lt 9) { Say 'Non trovo le impostazioni della barra: aprile da tasto destro sulla barra > Impostazioni della barra delle applicazioni.'; Open-User 'ms-settings:taskbar'; return }
if ((($v[8]) -band 1) -eq 0) { Say 'La barra non è impostata per nascondersi. Se sparisce lo stesso, usa "Barra in basso bloccata o icone sparite".'; return }
$u = Save-Undo $k @('Settings') 'barra-nascosta'
$v[8] = $v[8] -band 0xFE
Set-RegValue $k 'Settings' ([byte[]]$v) 'Binary'
Say 'Fatto: la barra non si nasconde più. Riavvio Esplora file...'
Restart-Explorer
Say-Undo $u
`)},
	{ID: "of_slow_now", Name: "Il PC è lento adesso", Desc: "Guarda cosa stanno usando processore, memoria e disco in questo momento e ti dice quale programma pesa di più e cosa fare. Non chiude niente. Controlla e spiega.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs2(`
$cores = [int](Get-CimInstance Win32_ComputerSystem -ErrorAction SilentlyContinue).NumberOfLogicalProcessors; if ($cores -lt 1) { $cores = 1 }
[void](Get-CimInstance Win32_PerfFormattedData_PerfProc_Process -ErrorAction SilentlyContinue); [void](Get-CimInstance Win32_PerfFormattedData_PerfOS_Processor -ErrorAction SilentlyContinue); [void](Get-CimInstance Win32_PerfFormattedData_PerfDisk_PhysicalDisk -ErrorAction SilentlyContinue)
Start-Sleep -Seconds 2
$cpu = [int](Get-CimInstance Win32_PerfFormattedData_PerfOS_Processor -Filter "Name='_Total'" -ErrorAction SilentlyContinue).PercentProcessorTime
$os = Get-CimInstance Win32_OperatingSystem -ErrorAction SilentlyContinue
$ram = 0; if ($os) { $ram = [int](100 - $os.FreePhysicalMemory * 100 / [math]::Max(1, $os.TotalVisibleMemorySize)) }
$dk = [int](Get-CimInstance Win32_PerfFormattedData_PerfDisk_PhysicalDisk -Filter "Name='_Total'" -ErrorAction SilentlyContinue).PercentIdleTime
Say ('Adesso: processore ' + $cpu + '%, memoria ' + $ram + '%, disco occupato ' + [math]::Max(0, 100 - $dk) + '%.')
$pp = @(Get-CimInstance Win32_PerfFormattedData_PerfProc_Process -ErrorAction SilentlyContinue | Where-Object { $_.Name -notmatch '^(_Total|Idle|System|Memory Compression)$' })
$top = @($pp | Sort-Object PercentProcessorTime -Descending | Select-Object -First 3 | Where-Object { $_.PercentProcessorTime / $cores -ge 10 })
$mem = @($pp | Group-Object { ($_.Name -split '#')[0] } | ForEach-Object { [pscustomobject]@{ n = $_.Name; m = ($_.Group | Measure-Object WorkingSetPrivate -Sum).Sum } } | Sort-Object m -Descending | Select-Object -First 3)
$why = @{ MsMpEng = 'l''antivirus sta controllando: aspetta qualche minuto'; SearchIndexer = 'Windows sta catalogando i file per la ricerca: finisce da solo'; TiWorker = 'Windows Update sta installando: lascialo finire'; TrustedInstaller = 'Windows Update sta installando: lascialo finire'; MoUsoCoreWorker = 'Windows Update sta lavorando'; OneDrive = 'OneDrive sta sincronizzando'; 'ms-teams' = 'Teams (in riunione è normale)'; chrome = 'Chrome: ogni scheda aperta pesa, chiudi quelle che non usi'; msedge = 'Edge: ogni scheda aperta pesa, chiudi quelle che non usi'; firefox = 'Firefox: chiudi le schede che non usi'; WINWORD = 'Word (un documento molto grande?)'; EXCEL = 'Excel sta calcolando (formule su tanti dati)'; OUTLOOK = 'Outlook sta sincronizzando la posta' }
foreach ($p in $top) { $n = ($p.Name -split '#')[0]; $t = $why[$n]; Say ('Usa il processore: ' + $n + ' (' + [int]($p.PercentProcessorTime / $cores) + '%)' + $(if ($t) { ': ' + $t } else { '' }) + '.') }
if ($ram -ge 85) { Say ('La memoria è quasi piena. I programmi che ne usano di più: ' + (($mem | ForEach-Object { $_.n + ' ' + (Fmt $_.m) }) -join ', ') + '. Chiudi quelli che non ti servono, soprattutto le schede del browser.') }
if ($dk -lt 15) { Say 'Il disco è al massimo: se è un disco "meccanico" (HDD) il PC va lento in questi momenti. Lascia finire o riavvia.' }
if ($cpu -lt 40 -and $ram -lt 80 -and $dk -ge 15) { Say 'In questo momento il PC non è sotto sforzo. Se ti sembra lento lo stesso: riavvialo (non solo "Arresta"), fai una pulizia con Tasso e guarda "Il PC ci mette tanto ad accendersi".' }
$up = (Get-Date) - $os.LastBootUpTime
if ($up.TotalDays -gt 5) { Say ('Il PC non si riavvia da ' + [int]$up.TotalDays + ' giorni: un riavvio libera la memoria e spesso basta.') }
`)},
	{ID: "of_fast_startup", Name: "Spegnendo il PC i problemi non spariscono", Desc: "Spegne l'\"Avvio rapido\": così \"Arresta il sistema\" spegne davvero Windows e ripulisce tutto come un riavvio. L'accensione può durare qualche secondo in più. Puoi tornare indietro.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$k = 'HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Power'
if ((Get-RegValue $k 'HiberbootEnabled') -eq 0) { Say 'L''Avvio rapido era già spento: "Arresta il sistema" spegne davvero il PC.'; return }
$u = Save-Undo $k @('HiberbootEnabled') 'avvio-rapido'
Set-RegValue $k 'HiberbootEnabled' 0 'DWord'
Say 'Fatto: spento l''Avvio rapido. Prima, con "Arresta il sistema", Windows non si spegneva del tutto (era come una pausa): per questo i problemi restavano.'
Say 'Ora spegnere e riaccendere è come riavviare. Risolve anche aggiornamenti che non finiscono, chiavette e stampanti non viste dopo l''accensione.'
Say-Undo $u
`)},

	// ---------- OneDrive, account e sicurezza ----------
	{ID: "of_onedrive_conflict", Name: "In OneDrive ci sono file doppi con il nome del PC", Desc: "Trova le copie che OneDrive crea quando lo stesso file viene cambiato in due posti (es. \"Offerta-PCUFFICIO.docx\") e te le elenca in un file, così scegli tu quale tenere. Non cancella niente. Controlla e spiega.", Level: 0, Minutes: 0.5, Timeout: 5 * time.Minute, Script: ofs2(`
$roots = Get-OneDriveRoots
if (-not $roots.Count) { Say 'OneDrive non è configurato su questo PC.'; return }
$pc = [regex]::Escape($env:COMPUTERNAME)
$found = @()
foreach ($r in $roots) { $found += @(Get-ChildItem -LiteralPath $r -Recurse -File -Force -ErrorAction SilentlyContinue | Where-Object { $_.BaseName -match ('-' + $pc + '(-\d+)?$') }) }
if (-not $found.Count) { Say 'Nessun file doppio con il nome del PC: OneDrive non ha trovato conflitti.'; return }
$dir = Join-Path ([Environment]::GetFolderPath('MyDocuments')) 'Tasso'
New-Item -ItemType Directory -Path $dir -Force | Out-Null
$f = Join-Path $dir 'OneDrive - file doppi.txt'
@('File doppi creati da OneDrive (trovati da Tasso il ' + (Get-Date -Format 'dd/MM/yyyy HH:mm') + ')', 'Accanto a ognuno c''è l''originale con lo stesso nome senza "-' + $env:COMPUTERNAME + '". Aprili tutti e due, tieni quello giusto e cancella l''altro.', '') + ($found | ForEach-Object { $_.FullName }) | Set-Content -LiteralPath $f -Encoding UTF8
Say ('Trovati ' + $found.Count + ' file doppi. Esempi: ' + (($found | Select-Object -First 5 | ForEach-Object { $_.Name }) -join ', ') + '.')
Say 'Succede quando lo stesso file viene modificato su due PC (o da te e da un collega) mentre uno dei due non era collegato. OneDrive non sa quale tenere e li tiene tutti e due.'
Say ('Ti apro l''elenco completo: ' + $f)
Say 'Per evitarlo: chiudi i file prima di spegnere il PC e aspetta che la nuvoletta di OneDrive abbia finito.'
Open-User $f
`)},
	{ID: "of_activation", Name: "Compare la scritta \"Attiva Windows\"", Desc: "Controlla se Windows è attivato e con che licenza, e ti spiega cosa fare. Tasso non usa e non userà mai programmi pirata per attivare. Controlla e spiega.", Level: 0, Minutes: 0.3, Timeout: 3 * time.Minute, Script: ofs2(`
$l = @(Get-CimInstance SoftwareLicensingProduct -Filter "ApplicationID='55c92734-d682-4d71-983e-d6ec3f16059f' AND PartialProductKey IS NOT NULL AND LicenseDependsOn IS NULL" -ErrorAction SilentlyContinue | Select-Object -First 1)
if (-not $l.Count) { Say 'Windows non ha nessun codice di licenza: non è attivato. Nella finestra che ti apro trovi come attivarlo.'; Open-User 'ms-settings:activation'; return }
$d = [string]$l[0].Description; $st = [int]$l[0].LicenseStatus
$kind = 'normale'; if ($d -match 'OEM') { $kind = 'del produttore del PC (OEM)' } elseif ($d -match 'KMS') { $kind = 'aziendale (KMS)' } elseif ($d -match 'MAK') { $kind = 'aziendale (MAK)' } elseif ($d -match 'RETAIL') { $kind = 'comprata a parte (Retail)' }
if ($st -eq 1) { Say ('Windows è attivato. Licenza ' + $kind + '.'); Say 'Se vedi lo stesso la scritta "Attiva Windows", riavvia il PC: sparisce.'; return }
Say ('Windows NON è attivato (licenza ' + $kind + ').')
if ($d -match 'KMS') { Say 'È una licenza aziendale: si attiva da sola quando il PC è collegato alla rete dell''ufficio (o con la VPN), almeno una volta ogni 6 mesi. Collegati e riavvia; se resta, chiama l''assistenza.' }
elseif ($d -match 'OEM') { Say 'La licenza è legata al PC. Succede dopo aver cambiato la scheda madre o reinstallato una versione diversa (Home/Pro). In Impostazioni > Attivazione premi "Risoluzione dei problemi".' }
else { Say 'In Impostazioni > Attivazione premi "Risoluzione dei problemi"; se hai cambiato pezzi del PC scegli "Ho cambiato l''hardware di recente". Se il codice non è valido, una licenza si compra nel Microsoft Store.' }
Say 'Attenzione ai programmi e ai siti che "attivano gratis": sono pirateria e spesso contengono virus.'
Open-User 'ms-settings:activation'
`)},
	{ID: "of_temp_profile", Name: "Sono entrato con un \"profilo temporaneo\" (desktop vuoto)", Desc: "Controlla se Windows ti ha fatto entrare con un profilo temporaneo e ti dice dov'è il tuo profilo vero: i tuoi file di solito sono ancora tutti lì. Controlla e spiega.", Level: 0, Minutes: 0.1, Timeout: time.Minute, Script: ofs2(`
$sid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$pl = 'HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList\'
$temp = ($env:USERPROFILE -match '\\TEMP(\.[^\\]*)?$')
$bak = [string](Get-RegValue ($pl + $sid + '.bak') 'ProfileImagePath')
if (-not $temp) {
  if ($bak) { Say ('(Nel registro è rimasta una vecchia copia del profilo, ' + $bak + ': non è un problema se tutto funziona.)') }
  Say ('Stai usando il tuo profilo normale (' + $env:USERPROFILE + ').')
  $desk = [Environment]::GetFolderPath('Desktop')
  if ($desk -match 'OneDrive') { Say ('Il tuo desktop è dentro OneDrive (' + $desk + '): se è vuoto, OneDrive sta ancora scaricando o i file sono online. Guarda la nuvoletta vicino all''orologio.') }
  else { Say 'Se il desktop sembra vuoto, prova "Le icone del desktop sono sparite".' }
  return
}
Say 'Sì: sei entrato con un PROFILO TEMPORANEO. Quello che salvi adesso sul desktop o in Documenti si PERDE quando esci.'
if ($bak) { Say ('Il tuo profilo vero è in: ' + $bak + '. I tuoi file sono lì: puoi aprirli da Esplora file e copiarli su una chiavetta per sicurezza.') }
Say 'Cosa fare: 1) riavvia il PC (non "Arresta"): spesso basta e rientri con il tuo profilo; 2) se resta temporaneo, serve un tecnico o l''assistenza: la riparazione tocca una parte delicata del registro e Tasso, per sicurezza, non la fa da solo.'
Say 'Di solito succede se il PC si è spento male mentre usciva, o se il disco ha errori: dopo, fai anche "Controllo errori del disco" in Tasso.'
`)},
	{ID: "of_pwd_expire", Name: "La password di Windows sta per scadere", Desc: "Ti dice quando scade la password del tuo account di Windows o dell'ufficio e come cambiarla prima (Ctrl + Alt + Canc > Cambia password). Tasso non vede né salva la password. Controlla e spiega.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs2(`
$cs = Get-CimInstance Win32_ComputerSystem -ErrorAction SilentlyContinue
$aad = ((dsregcmd.exe /status 2>$null) -join [char]10) -match 'AzureAdJoined\s*:\s*YES'
$who = [Security.Principal.WindowsIdentity]::GetCurrent().Name
$isDom = $cs.PartOfDomain -and ($who -split '\\')[0] -ne $env:COMPUTERNAME
if ($isDom) {
  $o = @(net.exe user $env:USERNAME /domain 2>$null)
  $line = @($o | Where-Object { $_ -match '^(Scadenza password|Password expires)' })
  if ($line.Count) { $v = ($line[0] -split '\s{2,}', 2)[-1].Trim(); if ($v -match '^(Mai|Never)$') { Say 'Account dell''ufficio: la password non scade.' } else { Say ('Account dell''ufficio: la password scade il ' + $v + '.') } } else { Say 'Account dell''ufficio: il server non risponde adesso (sei fuori ufficio senza VPN?).' }
  Say 'Per cambiarla: premi Ctrl + Alt + Canc > Cambia password, quando sei collegato alla rete dell''ufficio o alla VPN. Poi blocca e sblocca il PC con la nuova.'
} elseif ($aad) {
  Say 'Il PC usa l''account di lavoro Microsoft 365: la scadenza la decide l''azienda e Windows te lo ricorda da solo qualche giorno prima.'
  Say 'Per cambiarla: Ctrl + Alt + Canc > Cambia password, oppure dal sito mysignins.microsoft.com > Password.'
} else {
  $u = Get-LocalUser -Name $env:USERNAME -ErrorAction SilentlyContinue
  if ($u -and [string]$u.PrincipalSource -eq 'MicrosoftAccount') { Say 'Entri con un account Microsoft: la password non scade. Si cambia su account.microsoft.com > Sicurezza.'; Say 'Consiglio: usa il PIN per entrare in Windows (più comodo e sicuro): Impostazioni > Account > Opzioni di accesso.' }
  elseif ($u) {
    if ($u.PasswordExpires) { Say ('La password del tuo account scade il ' + $u.PasswordExpires.ToString('dd/MM/yyyy') + '. Per cambiarla: Ctrl + Alt + Canc > Cambia password.') } else { Say 'La password del tuo account di Windows non scade.' }
  } else { Say 'Non riesco a leggere i dati del tuo account.' }
}
Say 'Dopo averla cambiata, aggiorna la password salvata anche sul telefono (posta) e in Outlook se te la chiede: usa "Outlook chiede sempre la password".'
`)},
	{ID: "of_bitlocker", Name: "Il PC chiede la chiave di ripristino BitLocker", Desc: "Controlla se i dischi sono protetti con BitLocker e ti dice dove trovare la chiave di ripristino (account Microsoft o dell'ufficio) prima che serva. Tasso non mostra né salva la chiave. Controlla e spiega.", Level: 0, Minutes: 0.2, Timeout: 2 * time.Minute, Script: ofs2(`
if (-not (Get-Command Get-BitLockerVolume -ErrorAction SilentlyContinue)) { $o = (manage-bde.exe -status 2>$null) -join [char]10; if ($o -match '(?i)(Crittografia completata|Fully Encrypted|Crittografia in corso|Encryption in Progress)') { Say 'Almeno un disco è cifrato con BitLocker.' } else { Say 'Non riesco a controllare BitLocker su questa versione di Windows (Home senza BitLocker non cifra i dischi).' }; Say 'La chiave di ripristino è su aka.ms/myrecoverykey (account Microsoft) o la ha l''assistenza (PC dell''ufficio).'; return }
$v = @(Get-BitLockerVolume -ErrorAction SilentlyContinue | Where-Object { [string]$_.VolumeStatus -ne 'FullyDecrypted' })
if (-not $v.Count) { Say 'Nessun disco di questo PC è cifrato con BitLocker: non ti verrà chiesta nessuna chiave.'; return }
foreach ($x in $v) {
  $rp = @($x.KeyProtector | Where-Object { [string]$_.KeyProtectorType -eq 'RecoveryPassword' })
  $st = if ([string]$x.ProtectionStatus -eq 'On') { 'protetto' } else { 'cifrato ma con la protezione in pausa' }
  Say ('Disco ' + $x.MountPoint + ': ' + $st + '.' + $(if ($rp.Count) { ' Ha una chiave di ripristino (ID che inizia con ' + ([string]$rp[0].KeyProtectorId).Trim('{').Substring(0, 8) + ').' } else { ' NON ha una chiave di ripristino: chiedi all''assistenza di crearne una prima che serva.' }))
}
$aad = ((dsregcmd.exe /status 2>$null) -join [char]10) -match 'AzureAdJoined\s*:\s*YES'
$dom = [bool](Get-CimInstance Win32_ComputerSystem -ErrorAction SilentlyContinue).PartOfDomain
Say 'Dove trovare la chiave (48 numeri), se un giorno il PC te la chiede:'
if ($aad) { Say '- PC di lavoro con Microsoft 365: dal telefono vai su aka.ms/aadrecoverykey ed entra con l''account di lavoro, oppure chiedi all''assistenza.' }
elseif ($dom) { Say '- PC dell''ufficio: la chiave ce l''ha l''assistenza (è salvata nel server). Chiamali e dai l''ID che vedi sulla schermata.' }
else { Say '- Account Microsoft: dal telefono vai su aka.ms/myrecoverykey ed entra con lo stesso account del PC. Cerca l''ID che vedi sulla schermata blu.' }
Say 'La chiede di solito dopo un aggiornamento del BIOS, un cambio di pezzi o se cambi impostazioni del BIOS. Senza chiave i dati del disco non si recuperano: controlla ORA che sia dove ti ho detto.'
`)},
}

func init() {
	insertStepAfter("of_suspicious", officeSteps2...)
}
