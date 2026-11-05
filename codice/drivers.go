package main

// Driver di tutto il PC: Tasso elenca ogni periferica (audio, mouse, tastiera, scheda video AMD,
// NVIDIA o Intel, rete, Bluetooth, USB, dischi, webcam, monitor...) con la versione del driver,
// cerca gli aggiornamenti su Windows Update (driver certificati da Microsoft) e li installa.
// Per le cose che Windows Update non ha (driver video più nuovi, programmi di mouse e cuffie da
// gaming) apre la pagina ufficiale del produttore. Mai siti di "driver gratis" o pacchetti strani.

import (
	"regexp"
	"time"

	"golang.org/x/sys/windows/registry"
)

var (
	wuIDRe    = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	devInstRe = regexp.MustCompile(`^[A-Za-z0-9\\&_.{}#\- ]{3,300}$`)
)

// Pagine ufficiali dei produttori (solo queste: niente link presi da fuori).
var vendorPages = map[string][2]string{
	"nvidia":      {"NVIDIA App (driver GeForce)", "https://www.nvidia.com/it-it/software/nvidia-app/"},
	"nvidia_drv":  {"Driver NVIDIA (ricerca manuale)", "https://www.nvidia.com/it-it/drivers/"},
	"amd":         {"AMD Software: Adrenalin (driver Radeon e chipset Ryzen)", "https://www.amd.com/it/support/download/drivers.html"},
	"intel":       {"Intel Driver & Support Assistant", "https://www.intel.it/content/www/it/it/support/detect.html"},
	"logitech":    {"Logitech G HUB", "https://www.logitechg.com/it-it/innovation/g-hub.html"},
	"logi_opt":    {"Logi Options+", "https://www.logitech.com/it-it/software/logi-options-plus.html"},
	"razer":       {"Razer Synapse", "https://www.razer.com/synapse-4"},
	"corsair":     {"Corsair iCUE", "https://www.corsair.com/it/it/s/downloads"},
	"steelseries": {"SteelSeries GG", "https://steelseries.com/gg"},
	"hyperx":      {"HyperX NGENUITY", "https://hyperx.com/pages/ngenuity"},
	"roccat":      {"Turtle Beach / ROCCAT Swarm", "https://www.turtlebeach.com/pages/swarm-ii"},
	"asus":        {"Assistenza ASUS (driver della scheda madre e audio)", "https://www.asus.com/it/support/download-center/"},
	"msi":         {"Assistenza MSI (driver della scheda madre e audio)", "https://it.msi.com/support/download"},
	"gigabyte":    {"Assistenza GIGABYTE (driver della scheda madre e audio)", "https://www.gigabyte.com/it/Support"},
	"asrock":      {"Assistenza ASRock (driver della scheda madre e audio)", "https://www.asrock.com/support/index.it.asp"},
	"lenovo":      {"Lenovo Vantage / assistenza Lenovo", "https://pcsupport.lenovo.com/it/it"},
	"hp":          {"HP Support Assistant", "https://support.hp.com/it-it/drivers"},
	"dell":        {"Dell SupportAssist", "https://www.dell.com/support/home/it-it"},
	"acer":        {"Assistenza Acer (driver)", "https://www.acer.com/it-it/support/drivers-and-manuals"},
	"realtek":     {"Realtek (meglio dal sito della scheda madre o del PC)", "https://www.realtek.com/Download/List?cate_id=593"},
	"elgato":      {"Elgato (Stream Deck, Wave, Capture)", "https://www.elgato.com/it/it/s/downloads"},
	"focusrite":   {"Focusrite (schede audio)", "https://downloads.focusrite.com/"},
	"xbox":        {"Accessori Xbox (app ufficiale)", "https://apps.microsoft.com/detail/9nblggh30xj3"},
	"sony":        {"Controller PlayStation su PC (DualSense)", "https://www.playstation.com/it-it/support/hardware/pair-dualsense-controller-bluetooth/"},
}

func driversScan() {
	r := runPS(driversScanScript, 4*time.Minute, func(m string) { emit("onDriversProgress", m) })
	res, _ := decodeJSON(r.JSON).(map[string]interface{})
	if res == nil {
		res = map[string]interface{}{"ok": false, "error": "Non sono riuscito a leggere le periferiche."}
	}
	res["auto"] = driversAutoState()
	res["pages"] = vendorPages
	emit("onDrivers", res)
}

func driversCheck() {
	r := runPS(driversCheckScript, 10*time.Minute, func(m string) { emit("onDriversProgress", m) })
	res, _ := decodeJSON(r.JSON).(map[string]interface{})
	if res == nil {
		res = map[string]interface{}{"ok": false, "error": "Windows Update non ha risposto. Riprova tra poco (o usa Risolvi problemi → Windows Update bloccato)."}
	}
	emit("onDriversUpdates", res)
}

func driversInstall(ids []string) {
	var ok []string
	all := false
	for _, id := range ids {
		if id == "*" {
			all = true
		} else if wuIDRe.MatchString(id) {
			ok = append(ok, id)
		}
	}
	var msgs []string
	reboot := false
	if all || len(ok) > 0 {
		r := runPSArgs(driversInstallScript, map[string]interface{}{"all": all, "ids": ok}, 120*time.Minute, func(m string) {
			msgs = append(msgs, m)
			emit("onDriversProgress", m)
		})
		reboot = r.Reboot
	} else {
		msgs = append(msgs, "Niente da installare.")
	}
	emit("onDriversDone", map[string]interface{}{"msgs": msgs, "reboot": reboot})
}

// driverFix: riavvia una periferica che dà problemi e fa ricercare i driver a Windows.
func driverFix(id string) {
	if id != "" && !devInstRe.MatchString(id) {
		emit("onDriversDone", map[string]interface{}{"msgs": []string{"Periferica non valida."}})
		return
	}
	var msgs []string
	runPSArgs(driverFixScript, map[string]interface{}{"id": id}, 5*time.Minute, func(m string) { msgs = append(msgs, m) })
	emit("onDriversDone", map[string]interface{}{"msgs": msgs, "rescan": true})
}

func openVendorPage(id string) string {
	if p, ok := vendorPages[id]; ok {
		openURL(p[1])
		return "ok"
	}
	return "no"
}

const (
	drvSearchKey = `SOFTWARE\Microsoft\Windows\CurrentVersion\DriverSearching`
	drvPolicyKey = `SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate`
	drvMetaKey   = `SOFTWARE\Microsoft\Windows\CurrentVersion\Device Metadata`
)

// driversAutoState: Windows scarica da solo driver e icone delle periferiche?
func driversAutoState() bool {
	so, ok := regDword(registry.LOCAL_MACHINE, drvSearchKey, "SearchOrderConfig")
	if ok && so == 0 {
		return false
	}
	if ex, ok := regDword(registry.LOCAL_MACHINE, drvPolicyKey, "ExcludeWUDriversInQualityUpdate"); ok && ex == 1 {
		return false
	}
	return true
}

// setDriversAuto accende o spegne il download automatico dei driver di Windows (è un'impostazione
// di Windows, resta anche dopo il riavvio).
func setDriversAuto(on bool) string {
	v := uint32(0)
	if on {
		v = 1
	}
	if err := setDword(registry.LOCAL_MACHINE, drvSearchKey, "SearchOrderConfig", v); err != nil {
		return "Windows non mi ha lasciato cambiare questa impostazione."
	}
	_ = setDword(registry.LOCAL_MACHINE, drvMetaKey, "PreventDeviceMetadataFromNetwork", 1-v)
	if on {
		if k, err := registry.OpenKey(registry.LOCAL_MACHINE, drvPolicyKey, registry.SET_VALUE); err == nil {
			_ = k.DeleteValue("ExcludeWUDriversInQualityUpdate")
			k.Close()
		}
	} else {
		// Senza questa regola Windows Update continuerebbe a mandare driver insieme agli aggiornamenti.
		_ = setDword(registry.LOCAL_MACHINE, drvPolicyKey, "ExcludeWUDriversInQualityUpdate", 1)
	}
	return "ok"
}

const driversScanScript = `
Say 'Leggo tutte le periferiche e i loro driver...'
$ent = @{}
foreach ($e in @(Get-CimInstance Win32_PnPEntity -ErrorAction SilentlyContinue)) { if ($e.PNPDeviceID) { $ent[$e.PNPDeviceID] = $e } }
$catOf = @{ MEDIA = 'Audio'; AUDIOENDPOINT = 'Audio'; MOUSE = 'Mouse'; KEYBOARD = 'Tastiera'; DISPLAY = 'Scheda video'; NET = 'Rete e Wi-Fi';
  BLUETOOTH = 'Bluetooth'; USB = 'USB'; HIDCLASS = 'Controller e periferiche USB'; DISKDRIVE = 'Dischi'; SCSIADAPTER = 'Dischi'; HDC = 'Dischi';
  MONITOR = 'Monitor'; CAMERA = 'Webcam'; IMAGE = 'Webcam'; PRINTER = 'Stampanti'; XNACOMPOSITE = 'Controller e periferiche USB'; BIOMETRIC = 'Altro'; SENSOR = 'Altro'; SYSTEM = 'Scheda madre e chipset' }
$list = New-Object System.Collections.ArrayList
$seen = @{}
foreach ($d in @(Get-CimInstance Win32_PnPSignedDriver -ErrorAction SilentlyContinue)) {
  if (-not $d.DeviceName -or -not $d.DeviceID) { continue }
  $cls = ([string]$d.DeviceClass).ToUpper()
  $cat = $catOf[$cls]
  $e = $ent[$d.DeviceID]
  $err = 0; if ($e) { $err = [int]$e.ConfigManagerErrorCode }
  if (-not $cat) { if ($err -ne 0 -and $err -ne 22 -and $err -ne 45) { $cat = 'Altro' } else { continue } }
  # Le voci di sistema sono centinaia: tengo solo il chipset vero (AMD, Intel) e quelle con problemi.
  if ($cls -eq 'SYSTEM' -and $err -eq 0 -and ([string]$d.Manufacturer + ' ' + $d.DeviceName) -notmatch 'AMD|Intel|NVIDIA|chipset|SMBus|PSP|Management Engine|GPIO|Serial IO|Thermal') { continue }
  if ($cls -eq 'USB' -and $err -eq 0 -and $d.DeviceName -match 'Hub|Root|Generic USB|Composite') { continue }
  if ($cls -eq 'HIDCLASS' -and $err -eq 0 -and $d.DeviceName -match 'HID-compliant|conforme HID|Dispositivo di input USB|USB Input Device|consumer control|vendor-defined|definito dal fornitore') { continue }
  if ($cls -eq 'AUDIOENDPOINT') { continue }
  if ($cls -eq 'DISKDRIVE' -and $d.DeviceName -match 'Virtual') { continue }
  $key = [string]$d.DeviceName + '|' + [string]$d.DriverVersion
  if ($seen[$key] -and $err -eq 0) { continue }
  $seen[$key] = 1
  $date = ''; try { if ($d.DriverDate) { $date = ([datetime]$d.DriverDate).ToString('yyyy-MM-dd') } } catch {}
  $vid = ''; if ($d.DeviceID -match 'VID_([0-9A-Fa-f]{4})') { $vid = $Matches[1].ToUpper() } elseif ($d.DeviceID -match 'VEN_([0-9A-Fa-f]{4})') { $vid = $Matches[1].ToUpper() }
  [void]$list.Add([ordered]@{ name = [string]$d.DeviceName; cat = $cat; maker = [string]$d.Manufacturer; provider = [string]$d.DriverProviderName; ver = [string]$d.DriverVersion; date = $date; id = [string]$d.DeviceID; vid = $vid; err = $err })
}
# Periferiche senza nessun driver (codice 28) non compaiono tra i driver firmati.
foreach ($e in $ent.Values) {
  $c = [int]$e.ConfigManagerErrorCode
  if ($c -eq 0 -or $c -eq 22 -or $c -eq 45) { continue }
  if (@($list | Where-Object { $_.id -eq $e.PNPDeviceID }).Count) { continue }
  $vid = ''; if ($e.PNPDeviceID -match 'VID_([0-9A-Fa-f]{4})') { $vid = $Matches[1].ToUpper() } elseif ($e.PNPDeviceID -match 'VEN_([0-9A-Fa-f]{4})') { $vid = $Matches[1].ToUpper() }
  $n = [string]$e.Name; if (-not $n) { $n = 'Periferica sconosciuta' }
  [void]$list.Add([ordered]@{ name = $n; cat = 'Con problemi'; maker = [string]$e.Manufacturer; provider = ''; ver = ''; date = ''; id = [string]$e.PNPDeviceID; vid = $vid; err = $c })
}
$gpus = @(Get-CimInstance Win32_VideoController -ErrorAction SilentlyContinue | ForEach-Object {
  $dd = ''; try { if ($_.DriverDate) { $dd = ([datetime]$_.DriverDate).ToString('yyyy-MM-dd') } } catch {}
  $v = 'altro'; $n = [string]$_.Name
  if ($n -match 'NVIDIA|GeForce|RTX|GTX|Quadro') { $v = 'nvidia' } elseif ($n -match 'AMD|Radeon|ATI') { $v = 'amd' } elseif ($n -match 'Intel|Arc|Iris|UHD') { $v = 'intel' }
  [ordered]@{ name = $n; vendor = $v; ver = [string]$_.DriverVersion; date = $dd }
} | Where-Object { $_.name -notmatch 'Basic Display|Remote|Virtual|Parsec|Meta|Citrix' })
$cpu = [string](Get-CimInstance Win32_Processor -ErrorAction SilentlyContinue | Select-Object -First 1).Name
$bb = Get-CimInstance Win32_BaseBoard -ErrorAction SilentlyContinue | Select-Object -First 1
$cs = Get-CimInstance Win32_ComputerSystem -ErrorAction SilentlyContinue
Say ('Periferiche trovate: ' + $list.Count)
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject ([ordered]@{ ok = $true; devices = @($list); gpus = @($gpus); cpu = $cpu; board = [string]$bb.Manufacturer; boardModel = [string]$bb.Product; pcMaker = [string]$cs.Manufacturer; pcModel = [string]$cs.Model }) -Compress -Depth 4))
`

const driversCheckScript = `
try {
  Say 'Chiedo a Windows Update se ci sono driver nuovi (1-3 minuti)...'
  $s = New-Object -ComObject Microsoft.Update.Session
  $se = $s.CreateUpdateSearcher()
  try { $se.ServerSelection = 2 } catch {}
  $res = $se.Search("IsInstalled=0 and Type='Driver' and IsHidden=0")
  $out = New-Object System.Collections.ArrayList
  foreach ($u in $res.Updates) {
    $d = ''; try { if ($u.DriverVerDate) { $d = ([datetime]$u.DriverVerDate).ToString('yyyy-MM-dd') } } catch {}
    $mb = 0; try { $mb = [math]::Round([double]$u.MaxDownloadSize / 1MB, 1) } catch {}
    [void]$out.Add([ordered]@{ id = [string]$u.Identity.UpdateID; title = [string]$u.Title; cls = [string]$u.DriverClass; model = [string]$u.DriverModel; maker = [string]$u.DriverManufacturer; date = $d; mb = $mb; optional = [bool]$u.BrowseOnly })
  }
  Say ('Driver nuovi trovati: ' + $out.Count)
  [Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject ([ordered]@{ ok = $true; items = @($out) }) -Compress -Depth 4))
} catch {
  [Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject ([ordered]@{ ok = $false; error = ('Windows Update non ha risposto: ' + $_.Exception.Message) }) -Compress))
}
`

const driversInstallScript = `
try {
  $s = New-Object -ComObject Microsoft.Update.Session
  $se = $s.CreateUpdateSearcher()
  try { $se.ServerSelection = 2 } catch {}
  Say 'Preparo i driver da installare...'
  $res = $se.Search("IsInstalled=0 and Type='Driver' and IsHidden=0")
  $want = @{}; foreach ($i in @($A.ids)) { $want[[string]$i] = 1 }
  $coll = New-Object -ComObject Microsoft.Update.UpdateColl
  foreach ($u in $res.Updates) {
    if (($A.all -and -not $u.BrowseOnly) -or $want[[string]$u.Identity.UpdateID]) {
      if (-not $u.EulaAccepted) { $u.AcceptEula() }
      [void]$coll.Add($u)
    }
  }
  if ($coll.Count -eq 0) { Say 'Niente da installare: i driver sono già aggiornati.'; return }
  Say ('Scarico ' + $coll.Count + ' driver da Windows Update...')
  $dl = $s.CreateUpdateDownloader(); $dl.Updates = $coll; [void]$dl.Download()
  Say 'Installo (lo schermo o l''audio possono spegnersi un attimo: è normale)...'
  $inst = $s.CreateUpdateInstaller(); $inst.Updates = $coll
  $r = $inst.Install()
  $ok = 0
  for ($i = 0; $i -lt $coll.Count; $i++) {
    $c = [int]$r.GetUpdateResult($i).ResultCode
    if ($c -eq 2 -or $c -eq 3) { $ok++; Say ('Installato: ' + $coll.Item($i).Title) } else { Say ('Non installato: ' + $coll.Item($i).Title) }
  }
  Say ('Driver installati: ' + $ok + ' su ' + $coll.Count + '.')
  if ($r.RebootRequired) { Say 'Riavvia il PC per completare.'; [Console]::Out.WriteLine('REBOOT') }
} catch { Say ('Windows Update non ha risposto: ' + $_.Exception.Message) }
`

const driverFixScript = `
$id = [string]$A.id
if ($id) {
  Say 'Riavvio la periferica...'
  $code = Run-Hidden 'pnputil.exe' ('/restart-device "' + $id + '"')
  if ($code -eq 3010) { Say 'Windows chiede di riavviare il PC per far ripartire questa periferica.'; [Console]::Out.WriteLine('REBOOT') }
  elseif ($code -ne 0) {
    # Windows vecchio senza /restart-device: spengo e riaccendo, ma mai dischi, controller, sistema,
    # tastiere e mouse (se non si riaccendessero resteresti senza).
    $dev = Get-PnpDevice -InstanceId $id -ErrorAction SilentlyContinue
    if ($dev -and [string]$dev.Class -notmatch '^(DiskDrive|SCSIAdapter|HDC|System|Keyboard|Mouse|Volume|Processor|Computer)$') {
      try { Disable-PnpDevice -InstanceId $id -Confirm:$false -ErrorAction Stop; Start-Sleep -Seconds 2 } catch {}
      finally { try { Enable-PnpDevice -InstanceId $id -Confirm:$false -ErrorAction Stop } catch { Say 'ATTENZIONE: non sono riuscito a riaccendere la periferica: riavvia il PC e tornerà a posto.'; [Console]::Out.WriteLine('REBOOT') } }
    }
  }
}
Say 'Faccio cercare a Windows i driver mancanti...'
[void](Run-Hidden 'pnputil.exe' '/scan-devices')
Start-Sleep -Seconds 3
if ($id) {
  $e = Get-CimInstance Win32_PnPEntity -ErrorAction SilentlyContinue | Where-Object { $_.PNPDeviceID -eq $id } | Select-Object -First 1
  if ($e -and [int]$e.ConfigManagerErrorCode -eq 0) { Say 'Fatto: ora la periferica funziona.' }
  elseif ($e) { Say ('La periferica ha ancora un problema (codice ' + $e.ConfigManagerErrorCode + '). Prova "Cerca driver nuovi" qui sopra o il sito del produttore.') }
  else { Say 'Fatto. Se la periferica è scollegata, ricollegala.' }
} else { Say 'Fatto.' }
`
