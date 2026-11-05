package main

// Sezione "Prestazioni+" (overclock sicuro): legge i componenti del PC e dà consigli
// su misura, SENZA toccare da solo frequenze o voltaggi. Apre gli strumenti ufficiali
// e spiega come fare restando sotto i limiti del produttore.

import "time"

func perfScan() {
	r := runPS(perfScript, 2*time.Minute, nil)
	emit("onPerf", decodeJSON(r.JSON))
}

// setPerfProfile attiva il piano energetico "Prestazioni elevate" (on) o "Bilanciato" (off).
// È reversibile e non modifica l'hardware: cambia solo come Windows gestisce la CPU.
func setPerfProfile(on bool) string {
	modeMu.Lock()
	defer modeMu.Unlock()
	if !ensureSnapshot() {
		return "Non riesco a salvare com'era Windows, quindi per sicurezza non cambio niente. Riprova tra poco."
	}
	runPS(perfProfileScript(on), time.Minute, nil)
	return "ok"
}

// openPerfTool apre lo strumento ufficiale se installato, altrimenti la sua pagina di download.
func openPerfTool(id string) string {
	paths := map[string]string{}
	urls := map[string]string{
		"afterburner": "https://www.msi.com/Landing/afterburner/graphics-cards",
		"ryzen":       "https://www.amd.com/en/products/software/ryzen-master.html",
		"xtu":         "https://www.intel.com/content/www/us/en/download/17881/intel-extreme-tuning-utility-intel-xtu.html",
	}
	_ = paths
	if u, ok := urls[id]; ok {
		openURL(u)
		return "ok"
	}
	return "no"
}

func perfProfileScript(on bool) string {
	// Molti PC (soprattutto portatili nuovi) hanno solo il piano "Bilanciato": Use-HighPerf lo crea o,
	// se non si può, mette la modalità "Prestazioni migliori".
	if on {
		return modeBase + "\nUse-HighPerf\n"
	}
	return modeBase + "\n[void](Use-Plan $BAL)\npowercfg /overlaysetactive 00000000-0000-0000-0000-000000000000 2>$null | Out-Null\n"
}

const perfScript = `
$cpu = Get-CimInstance Win32_Processor -ErrorAction SilentlyContinue | Select-Object -First 1
# Portatile: lo dice il tipo di PC (2 = mobile) o il telaio, non la batteria (anche un UPS USB è una "batteria").
$isLaptop = $false
try { if ([int](Get-CimInstance Win32_ComputerSystem -ErrorAction Stop).PCSystemType -eq 2) { $isLaptop = $true } } catch {}
try { foreach ($c in @((Get-CimInstance Win32_SystemEnclosure -ErrorAction Stop).ChassisTypes)) { if (@(8,9,10,11,12,14,30,31,32) -contains [int]$c) { $isLaptop = $true } } } catch {}
# Prima la scheda video vera (NVIDIA, Radeon, Arc), poi quella dentro il processore.
$gpu = @(Get-CimInstance Win32_VideoController -ErrorAction SilentlyContinue | Where-Object { $_.Name -match 'NVIDIA|AMD|Radeon|GeForce|RTX|GTX|Arc|Intel' } | Sort-Object { if ($_.Name -match 'NVIDIA|GeForce|RTX|GTX|Radeon RX|Radeon Pro|Arc\(TM\) [AB]|Arc [AB]') { 0 } else { 1 } }) | Select-Object -First 1
$bb = Get-CimInstance Win32_BaseBoard -ErrorAction SilentlyContinue | Select-Object -First 1
$mem = @(Get-CimInstance Win32_PhysicalMemory -ErrorAction SilentlyContinue)
$cur = 0; $rated = 0; $tot = [long]0
$ddr = 0; $pnRated = 0
foreach ($m in $mem) {
  $t = [int]$m.SMBIOSMemoryType; if ($t -eq 26) { $ddr = 4 } elseif ($t -eq 34) { $ddr = 5 }
  # La velocità vera del kit è spesso scritta nel codice del modulo (es. F4-3600C16, CMK16GX4M2B3200C16, KF436C17).
  $pn = ([string]$m.PartNumber).Trim()
  $mm = [regex]::Match($pn, '(?i)(?:F[45]-|X[45]M\d[A-Z]?|-)(\d{4})[CJ]\d{2}')
  if ($mm.Success) { $v = [int]$mm.Groups[1].Value; if ($v -gt $pnRated -and $v -lt 9000) { $pnRated = $v } }
  else { $mk = [regex]::Match($pn, '(?i)^KF[45](\d{2})C'); if ($mk.Success) { $v = [int]$mk.Groups[1].Value * 100; if ($v -gt $pnRated) { $pnRated = $v } } }
  if ($m.ConfiguredClockSpeed -and [int]$m.ConfiguredClockSpeed -gt $cur) { $cur = [int]$m.ConfiguredClockSpeed }
  if ($m.Speed -and [int]$m.Speed -gt $rated) { $rated = [int]$m.Speed }
  if ($m.Capacity) { $tot += [long]$m.Capacity }
}
$cpuName = ([string]$cpu.Name).Trim()
$cpuVendor = 'altro'; if ($cpuName -match 'AMD|Ryzen|Threadripper') { $cpuVendor = 'amd' } elseif ($cpuName -match 'Intel|Core|Pentium|Celeron') { $cpuVendor = 'intel' }
$gpuName = ([string]$gpu.Name).Trim()
$gpuVendor = 'altro'; if ($gpuName -match 'NVIDIA|GeForce|RTX|GTX') { $gpuVendor = 'nvidia' } elseif ($gpuName -match 'AMD|Radeon') { $gpuVendor = 'amd' } elseif ($gpuName -match 'Intel|Arc') { $gpuVendor = 'intel' }
function Find-Tool($paths) { foreach ($p in $paths) { if ($p -and (Test-Path -LiteralPath $p)) { return $true } }; return $false }
$pf = $env:ProgramFiles; $pf86 = ${env:ProgramFiles(x86)}
$afterburner = Find-Tool @((Join-Path $pf86 'MSI Afterburner\MSIAfterburner.exe'), (Join-Path $pf 'MSI Afterburner\MSIAfterburner.exe'))
$ryzen = Find-Tool @((Join-Path $pf 'AMD\RyzenMaster\bin\AMDRyzenMasterx64.exe'), (Join-Path $pf 'AMD\RyzenMasterMonitoringSDK\bin\AMDRyzenMasterx64.exe'))
$xtu = Find-Tool @((Join-Path $pf 'Intel\Intel(R) Extreme Tuning Utility\Client\PerfTune.exe'), (Join-Path $pf86 'Intel\Intel(R) Extreme Tuning Utility\Client\PerfTune.exe'))
$scheme = ''
try { $scheme = (powercfg /getactivescheme) | Out-String } catch {}
$highOn = ($scheme -match '8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c') -or ($scheme -match 'Prestazioni elevate') -or ($scheme -match 'High performance') -or ($scheme -match 'Ultimate')
$o = [ordered]@{
  cpu = $cpuName; cpuVendor = $cpuVendor; gpu = $gpuName; gpuVendor = $gpuVendor;
  board = ((([string]$bb.Manufacturer).Trim() + ' ' + ([string]$bb.Product).Trim()).Trim());
  ramCur = $cur; ramRated = $rated; ramDdr = $ddr; ramKit = $pnRated; laptop = $isLaptop; ramCount = $mem.Count; ramGB = [math]::Round($tot / 1GB, 0);
  afterburner = $afterburner; ryzen = $ryzen; xtu = $xtu; highPerf = [bool]$highOn
}
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject $o -Compress -Depth 3))
`
