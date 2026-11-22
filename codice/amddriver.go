package main

// Driver AMD Radeon dentro il Tasso (3.1).
// AMD non ha un servizio pubblico per chiedere "qual è il driver nuovo", quindi qui uso la via
// sicura: Windows Update, che per le Radeon offre il driver AMD certificato da Microsoft (firmato,
// scelto da Windows per quella scheda). Il Tasso installa SOLO quel driver, dopo il punto di
// ripristino. Per l'ultimissima versione (che esce prima sul sito AMD) apre AMD.
// Onestà: la versione di Windows Update a volte è un po' più vecchia di quella sul sito AMD.

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"
)

type amdGpu struct {
	Name   string `json:"name"`
	Ver    string `json:"ver"`  // versione Windows del driver, es. 32.0.21013.1000
	Date   string `json:"date"` // aaaa-mm-gg
	reboot bool
	wu     bool
}

type amdOffer struct {
	ID    string  `json:"id"`
	Title string  `json:"title"`
	Ver   string  `json:"ver"`
	Date  string  `json:"date"`
	MB    float64 `json:"mb"`
}

var (
	amdMu     sync.Mutex
	amdOffer1 *amdOffer // l'offerta mostrata: si installa solo quella
	amdVerRe  = regexp.MustCompile(`\b(\d{2}\.\d{1,3}\.\d{1,5}\.\d{1,5})\b`)
	amdMfrRe  = regexp.MustCompile(`(?i)advanced micro devices|\bamd\b|\bati\b`)
	errNoAMD  = errors.New("Non ho trovato una scheda video AMD Radeon.")
)

const amdGpuScript = `
$g = @(Get-CimInstance Win32_VideoController -ErrorAction SilentlyContinue | Where-Object { [string]$_.PNPDeviceID -match 'VEN_1002' } | Select-Object -First 1)
$rb = (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending') -or (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired')
$wu = [bool](Get-Process -Name TiWorker, TrustedInstaller -ErrorAction SilentlyContinue)
if ($g.Count) {
  $d = ''; try { if ($g[0].DriverDate) { $d = ([datetime]$g[0].DriverDate).ToString('yyyy-MM-dd') } } catch {}
  [Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -Compress -InputObject ([ordered]@{ name = [string]$g[0].Name; ver = [string]$g[0].DriverVersion; date = $d; reboot = $rb; wu = $wu })))
} else { [Console]::Out.WriteLine('JSON {}') }
`

func amdDetect() (*amdGpu, error) {
	r := runPS(amdGpuScript, 90*time.Second, nil)
	var raw struct {
		Name   string `json:"name"`
		Ver    string `json:"ver"`
		Date   string `json:"date"`
		Reboot bool   `json:"reboot"`
		WU     bool   `json:"wu"`
	}
	if r.JSON == "" || json.Unmarshal([]byte(r.JSON), &raw) != nil {
		return nil, errors.New("Non riesco a leggere la scheda video.")
	}
	if raw.Name == "" {
		return nil, errNoAMD
	}
	return &amdGpu{Name: strings.TrimSpace(raw.Name), Ver: strings.TrimSpace(raw.Ver), Date: raw.Date, reboot: raw.Reboot, wu: raw.WU}, nil
}

// Cerco su Windows Update solo i driver video di AMD.
const amdWUSearchScript = `
try {
  $s = New-Object -ComObject Microsoft.Update.Session
  $se = $s.CreateUpdateSearcher()
  try { $se.ServerSelection = 2 } catch {}
  $res = $se.Search("IsInstalled=0 and Type='Driver' and IsHidden=0")
  $out = New-Object System.Collections.ArrayList
  foreach ($u in $res.Updates) {
    $d = ''; try { if ($u.DriverVerDate) { $d = ([datetime]$u.DriverVerDate).ToString('yyyy-MM-dd') } } catch {}
    $mb = 0; try { $mb = [math]::Round([double]$u.MaxDownloadSize / 1MB, 1) } catch {}
    [void]$out.Add([ordered]@{ id = [string]$u.Identity.UpdateID; title = [string]$u.Title; cls = [string]$u.DriverClass; maker = [string]$u.DriverManufacturer; date = $d; mb = $mb })
  }
  [Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject ([ordered]@{ ok = $true; items = @($out) }) -Compress -Depth 4))
} catch { [Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -Compress -InputObject ([ordered]@{ ok = $false }))) }
`

type wuDrv struct {
	ID    string  `json:"id"`
	Title string  `json:"title"`
	Cls   string  `json:"cls"`
	Maker string  `json:"maker"`
	Date  string  `json:"date"`
	MB    float64 `json:"mb"`
}

// amdPick sceglie, tra i driver offerti da Windows Update, quello video di AMD più recente (per le prove).
func amdPick(items []wuDrv) *amdOffer {
	var best *amdOffer
	for _, it := range items {
		if !strings.EqualFold(strings.TrimSpace(it.Cls), "Display") || !amdMfrRe.MatchString(it.Maker+" "+it.Title) || !wuIDRe.MatchString(it.ID) {
			continue
		}
		o := &amdOffer{ID: it.ID, Title: it.Title, Date: it.Date, MB: it.MB}
		if m := amdVerRe.FindStringSubmatch(it.Title); m != nil {
			o.Ver = m[1]
		}
		if best == nil || o.Date > best.Date {
			best = o
		}
	}
	return best
}

func amdCheck(g *amdGpu) {
	r := runPS(amdWUSearchScript, 10*time.Minute, nil)
	var res struct {
		OK    bool    `json:"ok"`
		Items []wuDrv `json:"items"`
	}
	out := map[string]interface{}{"vendor": "amd", "gpu": map[string]string{"name": g.Name, "ver": g.Ver, "date": g.Date}}
	amdMu.Lock()
	amdOffer1 = nil
	amdMu.Unlock()
	if r.JSON == "" || json.Unmarshal([]byte(r.JSON), &res) != nil || !res.OK {
		out["error"] = "Windows Update non ha risposto. Riprova tra poco, oppure prendi il driver dal sito AMD."
		emit("onGpuDrvInfo", out)
		return
	}
	o := amdPick(res.Items)
	if o == nil {
		out["newer"] = false
		emit("onGpuDrvInfo", out)
		return
	}
	amdMu.Lock()
	amdOffer1 = o
	amdMu.Unlock()
	out["offer"] = map[string]interface{}{"ver": o.Ver, "date": o.Date, "mb": o.MB, "title": o.Title}
	out["newer"] = true
	emit("onGpuDrvInfo", out)
}

// Installa SOLO l'aggiornamento scelto, e solo se è ancora un driver video AMD.
const amdWUInstallScript = `
try {
  $s = New-Object -ComObject Microsoft.Update.Session
  $se = $s.CreateUpdateSearcher()
  try { $se.ServerSelection = 2 } catch {}
  $res = $se.Search("IsInstalled=0 and Type='Driver' and IsHidden=0")
  $coll = New-Object -ComObject Microsoft.Update.UpdateColl
  foreach ($u in $res.Updates) {
    if ([string]$u.Identity.UpdateID -eq [string]$A.id -and [string]$u.DriverClass -eq 'Display' -and ([string]$u.DriverManufacturer + ' ' + [string]$u.Title) -match '(?i)advanced micro devices|\bamd\b|\bati\b') {
      if (-not $u.EulaAccepted) { $u.AcceptEula() }
      [void]$coll.Add($u)
    }
  }
  if ($coll.Count -eq 0) { [Console]::Out.WriteLine('JSON {"res":"gone"}'); return }
  $dl = $s.CreateUpdateDownloader(); $dl.Updates = $coll; [void]$dl.Download()
  $inst = $s.CreateUpdateInstaller(); $inst.Updates = $coll
  $r = $inst.Install()
  $c = [int]$r.GetUpdateResult(0).ResultCode
  [Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -Compress -InputObject ([ordered]@{ res = 'done'; code = $c; reboot = [bool]$r.RebootRequired })))
} catch { [Console]::Out.WriteLine('JSON {"res":"error"}') }
`

func amdInstall(noBackup bool) {
	fail := func(msg string) { gdEmit("error", msg, nil) }
	if !claimRun() {
		fail("Sta girando una pulizia o un aggiornamento del Tasso: aspetta che finisca.")
		return
	}
	defer releaseRun()
	g, err := amdDetect()
	if err != nil {
		fail(err.Error())
		return
	}
	if b := gdBlocker(onBattery(), safeName(runningGame()), g.reboot, g.wu); b != "" {
		fail(b)
		return
	}
	amdMu.Lock()
	o := amdOffer1
	amdMu.Unlock()
	if o == nil {
		fail("Windows Update non ha un driver AMD più nuovo per la tua scheda.")
		return
	}
	backupMsg := ""
	if !noBackup {
		gdEmit("backup", "Creo il punto di ripristino…", map[string]interface{}{"from": g.Ver, "to": o.Ver})
		switch gdRestorePoint(o.Ver) {
		case "new":
		case "recent":
			backupMsg = " Uso il punto di ripristino creato poco fa."
		default:
			gdEmit("nobackup", "Non sono riuscito a creare il punto di ripristino. Vuoi installare lo stesso?", map[string]interface{}{"undo": false, "vendor": "amd"})
			return
		}
	}
	if b := gdBlocker(onBattery(), safeName(runningGame()), false, false); b != "" {
		fail(b)
		return
	}
	gdEmit("install", "Scarico e installo il driver AMD da Windows Update."+backupMsg+" Lo schermo può diventare nero per qualche secondo: è normale. Non spegnere e non riavviare il PC.", map[string]interface{}{"from": g.Ver, "to": o.Ver})
	r := runPSArgs(amdWUInstallScript, map[string]string{"id": o.ID}, 90*time.Minute, nil)
	var res struct {
		Res    string `json:"res"`
		Code   int    `json:"code"`
		Reboot bool   `json:"reboot"`
	}
	if r.JSON == "" || json.Unmarshal([]byte(r.JSON), &res) != nil || res.Res == "error" {
		fail("Windows Update non è riuscito a installare il driver AMD. Il driver di prima è rimasto.")
		return
	}
	if res.Res == "gone" {
		fail("Windows Update non offre più quel driver: premi di nuovo \"Aggiorna con Tasso\" per ricontrollare.")
		return
	}
	amdMu.Lock()
	amdOffer1 = nil
	amdMu.Unlock()
	// ResultCode di Windows Update: 2 = riuscito, 3 = riuscito con errori.
	if res.Code != 2 && res.Code != 3 {
		fail("Windows Update non è riuscito a installare il driver AMD. Il driver di prima è rimasto. Prova dal sito AMD.")
		return
	}
	now := ""
	if g2, e := amdDetect(); e == nil {
		now = g2.Ver
	}
	if res.Reboot || now == g.Ver {
		gdEmit("done", "Driver AMD installato: riavvia il PC per finire.", map[string]interface{}{"reboot": true, "vendor": "amd"})
		return
	}
	gdEmit("done", "Fatto: driver AMD installato ("+now+"). Se ti dà problemi usa il punto di ripristino, oppure in Gestione dispositivi → scheda video → Ripristina driver.", map[string]interface{}{"vendor": "amd"})
}
