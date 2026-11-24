package main

// Driver AMD Radeon dentro il Tasso (3.1).
// AMD non ha un servizio pubblico per chiedere "qual è il driver nuovo", quindi qui uso la via
// sicura scelta da Sergio: Windows Update, che per le Radeon offre il driver AMD certificato da
// Microsoft. Regole (dopo la revisione 4 + scettico):
//   - offro un driver SOLO se è per una scheda AMD di questo PC (stesso codice hardware) ed è
//     davvero più nuovo di quello installato: mai tornare indietro di versione;
//   - se c'è AMD Software (Adrenalin, preso dal sito AMD) non installo da Windows Update: driver e
//     programma AMD devono restare della stessa versione, quindi apro AMD Software;
//   - prima di installare: punto di ripristino.
// Onestà: la versione di Windows Update a volte è più vecchia di quella sul sito AMD, e spesso
// Windows l'ha già installata da solo, quindi il risultato più comune sarà "niente di nuovo".

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

type amdGpu struct {
	Name string `json:"name"`
	Ver  string `json:"ver"`  // versione Windows del driver, es. 32.0.21013.1000
	Date string `json:"date"` // aaaa-mm-gg
	PNP  string `json:"pnp"`
}

type amdOffer struct {
	ID    string  `json:"id"`
	Title string  `json:"title"`
	Ver   string  `json:"ver"`
	Date  string  `json:"date"`
	MB    float64 `json:"mb"`
	gpu   amdGpu  // la scheda a cui è destinato
}

var (
	amdMu     sync.Mutex
	amdOffer1 *amdOffer // l'offerta mostrata: si installa solo quella
	amdVerRe  = regexp.MustCompile(`\b(\d{2}\.\d{1,3}\.\d{1,5}\.\d{1,5})\b`)
	amdMfrRe  = regexp.MustCompile(`(?i)advanced micro devices|\bamd\b|\bati\b`)
	errNoAMD  = errors.New("Non ho trovato una scheda video AMD Radeon.")
)

const amdGpuScript = `
$gs = @(Get-CimInstance Win32_VideoController -ErrorAction SilentlyContinue | Where-Object { [string]$_.PNPDeviceID -match 'VEN_1002' })
$out = New-Object System.Collections.ArrayList
foreach ($g in $gs) {
  $d = ''; try { if ($g.DriverDate) { $d = ([datetime]$g.DriverDate).ToString('yyyy-MM-dd') } } catch {}
  [void]$out.Add([ordered]@{ name = [string]$g.Name; ver = [string]$g.DriverVersion; date = $d; pnp = [string]$g.PNPDeviceID })
}
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -Compress -Depth 3 -InputObject ([ordered]@{ gpus = @($out) })))
`

func amdDetect() ([]amdGpu, error) {
	r := runPS(amdGpuScript, 90*time.Second, nil)
	var raw struct {
		Gpus []amdGpu `json:"gpus"`
	}
	if r.JSON == "" || json.Unmarshal([]byte(r.JSON), &raw) != nil {
		return nil, errors.New("Non riesco a leggere la scheda video.")
	}
	var out []amdGpu
	for _, g := range raw.Gpus {
		if strings.TrimSpace(g.Name) != "" {
			g.Name, g.Ver = strings.TrimSpace(g.Name), strings.TrimSpace(g.Ver)
			out = append(out, g)
		}
	}
	if len(out) == 0 {
		return nil, errNoAMD
	}
	return out, nil
}

// amdAdrenalin: è installato AMD Software (Adrenalin)? Il percorso lo chiedo a Windows.
func amdAdrenalin() string {
	pf, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil || pf == "" {
		return ""
	}
	p := filepath.Join(pf, "AMD", "CNext", "CNext", "RadeonSoftware.exe")
	if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
		return p
	}
	return ""
}

// Cerco su Windows Update i driver offerti, con il codice hardware a cui sono destinati.
const amdWUSearchScript = `
$pol = $false
try { $pol = ((Get-ItemProperty -Path 'HKLM:\SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate' -Name ExcludeWUDriversInQualityUpdate -ErrorAction Stop).ExcludeWUDriversInQualityUpdate -eq 1) } catch {}
try {
  $s = New-Object -ComObject Microsoft.Update.Session
  $se = $s.CreateUpdateSearcher()
  try { $se.ServerSelection = 2 } catch {}
  $res = $se.Search("IsInstalled=0 and Type='Driver' and IsHidden=0")
  $out = New-Object System.Collections.ArrayList
  foreach ($u in $res.Updates) {
    $d = ''; try { if ($u.DriverVerDate) { $d = ([datetime]$u.DriverVerDate).ToString('yyyy-MM-dd') } } catch {}
    $mb = 0; try { $mb = [math]::Round([double]$u.MaxDownloadSize / 1MB, 1) } catch {}
    [void]$out.Add([ordered]@{ id = [string]$u.Identity.UpdateID; title = [string]$u.Title; cls = [string]$u.DriverClass; maker = [string]$u.DriverManufacturer; hwid = [string]$u.DriverHardwareID; date = $d; mb = $mb })
  }
  [Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject ([ordered]@{ ok = $true; policy = $pol; items = @($out) }) -Compress -Depth 4))
} catch {
  [Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -Compress -InputObject ([ordered]@{ ok = $false; policy = $pol; hr = [string]('{0:X8}' -f $_.Exception.HResult) })))
}
`

type wuDrv struct {
	ID    string  `json:"id"`
	Title string  `json:"title"`
	Cls   string  `json:"cls"`
	Maker string  `json:"maker"`
	HWID  string  `json:"hwid"`
	Date  string  `json:"date"`
	MB    float64 `json:"mb"`
}

// winVerNewer: a è più nuova di b? (versioni Windows a 4 numeri, es. 32.0.21013.1000)
func winVerNewer(a, b string) (newer, ok bool) {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	if len(pa) != 4 || len(pb) != 4 {
		return false, false
	}
	for i := 0; i < 4; i++ {
		x, e1 := strconv.Atoi(pa[i])
		y, e2 := strconv.Atoi(pb[i])
		if e1 != nil || e2 != nil {
			return false, false
		}
		if x != y {
			return x > y, true
		}
	}
	return false, true
}

// amdPick sceglie il driver video AMD di Windows Update per una scheda di QUESTO PC, solo se è
// davvero più nuovo di quello installato su quella scheda (separato per le prove).
func amdPick(items []wuDrv, gpus []amdGpu) *amdOffer {
	var best *amdOffer
	for _, it := range items {
		if !strings.EqualFold(strings.TrimSpace(it.Cls), "Display") || !amdMfrRe.MatchString(it.Maker+" "+it.Title) || !wuIDRe.MatchString(it.ID) {
			continue
		}
		hw := strings.ToUpper(strings.TrimSpace(it.HWID))
		if !strings.Contains(hw, "VEN_1002") {
			continue
		}
		o := &amdOffer{ID: it.ID, Title: it.Title, Date: it.Date, MB: it.MB}
		if m := amdVerRe.FindStringSubmatch(it.Title); m != nil {
			o.Ver = m[1]
		}
		for _, g := range gpus {
			if !strings.HasPrefix(strings.ToUpper(g.PNP), hw) {
				continue
			}
			newer, ok := winVerNewer(o.Ver, g.Ver)
			if !ok { // versione non leggibile: decido con la data, solo se è chiaramente più nuova
				newer = o.Date != "" && g.Date != "" && o.Date > g.Date
			}
			if newer && (best == nil || o.Date > best.Date) {
				o.gpu = g
				c := *o
				best = &c
			}
		}
	}
	return best
}

func amdCheck(gpus []amdGpu) {
	g := gpus[0]
	out := map[string]interface{}{"vendor": "amd", "gpu": map[string]string{"name": g.Name, "ver": g.Ver, "date": g.Date}}
	amdMu.Lock()
	amdOffer1 = nil
	amdMu.Unlock()
	if p := amdAdrenalin(); p != "" {
		// Con AMD Software il driver lo aggiorna lui: così driver e programma restano uguali.
		out["adrenalin"] = true
		out["newer"] = false
		emit("onGpuDrvInfo", out)
		return
	}
	r := runPS(amdWUSearchScript, 10*time.Minute, nil)
	var res struct {
		OK     bool    `json:"ok"`
		Policy bool    `json:"policy"`
		HR     string  `json:"hr"`
		Items  []wuDrv `json:"items"`
	}
	if r.JSON == "" || json.Unmarshal([]byte(r.JSON), &res) != nil || !res.OK {
		switch {
		case res.HR == "80070422":
			out["error"] = "Windows Update è spento su questo PC, quindi non posso cercare il driver. Prendilo dal sito AMD."
		case res.Policy:
			out["error"] = "Su questo PC Windows Update non cerca i driver (impostazione dell'azienda o della scuola). Prendilo dal sito AMD."
		default:
			out["error"] = "Windows Update non ha risposto: controlla internet e riprova, oppure prendi il driver dal sito AMD."
		}
		emit("onGpuDrvInfo", out)
		return
	}
	if res.Policy {
		out["error"] = "Su questo PC Windows Update non cerca i driver (impostazione dell'azienda o della scuola). Prendilo dal sito AMD."
		emit("onGpuDrvInfo", out)
		return
	}
	o := amdPick(res.Items, gpus)
	if o == nil {
		out["newer"] = false
		emit("onGpuDrvInfo", out)
		return
	}
	amdMu.Lock()
	amdOffer1 = o
	amdMu.Unlock()
	out["gpu"] = map[string]string{"name": o.gpu.Name, "ver": o.gpu.Ver, "date": o.gpu.Date}
	out["offer"] = map[string]interface{}{"ver": o.Ver, "date": o.Date, "mb": o.MB}
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

// amdVerOf: la versione attuale della scheda con questo codice (dopo l'installazione).
func amdVerOf(pnp string) string {
	gs, err := amdDetect()
	if err != nil {
		return ""
	}
	for _, g := range gs {
		if strings.EqualFold(g.PNP, pnp) {
			return g.Ver
		}
	}
	return ""
}

func amdInstall(noBackup bool) {
	fail := func(msg string) { gdEmit("error", msg, map[string]interface{}{"vendor": "amd"}) }
	if !claimRun() {
		fail("Sta girando una pulizia o un aggiornamento di Tasso: aspetta che finisca.")
		return
	}
	defer releaseRun()
	// Windows Update mette in coda da solo le sue installazioni: qui blocco solo batteria e gioco.
	if b := gdBlocker(onBattery(), safeName(runningGame()), false, false); b != "" {
		fail(b)
		return
	}
	if amdAdrenalin() != "" {
		fail("Hai AMD Software: aggiorna il driver da lì (Driver e software → Controlla aggiornamenti), così driver e programma restano uguali.")
		return
	}
	amdMu.Lock()
	o := amdOffer1
	amdMu.Unlock()
	if o == nil {
		fail("Windows Update non ha un driver AMD più nuovo per la tua scheda.")
		return
	}
	before := amdVerOf(o.gpu.PNP)
	if before == "" {
		fail("Non riesco a leggere la scheda video: riprova.")
		return
	}
	if newer, ok := winVerNewer(o.Ver, before); ok && !newer {
		fail("Hai già un driver AMD uguale o più nuovo: non lo cambio.")
		return
	}
	backupMsg := ""
	if !noBackup {
		gdEmit("backup", "Creo il punto di ripristino…", map[string]interface{}{"from": before, "to": o.Ver})
		switch gdRestorePoint("AMD " + o.Ver) {
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
	gdEmit("install", "Scarico e installo il driver AMD da Windows Update. Può volerci qualche minuto."+backupMsg+" Lo schermo può diventare nero per qualche secondo: è normale. Non spegnere e non riavviare il PC.", map[string]interface{}{"from": before, "to": o.Ver})
	r := runPSArgs(amdWUInstallScript, map[string]string{"id": o.ID}, 90*time.Minute, nil)
	var res struct {
		Res    string `json:"res"`
		Code   int    `json:"code"`
		Reboot bool   `json:"reboot"`
	}
	parsed := r.JSON != "" && json.Unmarshal([]byte(r.JSON), &res) == nil
	amdMu.Lock()
	amdOffer1 = nil
	amdMu.Unlock()
	now := amdVerOf(o.gpu.PNP)
	switch {
	case parsed && res.Res == "gone":
		fail("Windows Update non offre più quel driver (forse l'ha già messo da solo): premi di nuovo \"Aggiorna con Tasso\" per ricontrollare.")
	case now != "" && now != before:
		gdEmit("done", "Fatto: driver AMD installato ("+now+"). Se ti dà problemi usa il punto di ripristino.", map[string]interface{}{"vendor": "amd", "reboot": res.Reboot})
	case parsed && (res.Code == 2 || res.Code == 3) && res.Reboot:
		gdEmit("done", "Driver AMD installato: riavvia il PC per finire.", map[string]interface{}{"reboot": true, "vendor": "amd"})
	case parsed && res.Res == "done" && res.Code != 2 && res.Code != 3 && now == before:
		fail("Windows Update non è riuscito a installare il driver AMD. Il driver di prima è rimasto. Prova dal sito AMD.")
	default:
		// Non so com'è andata (Windows Update occupato, tempo scaduto, risposta strana).
		fail("Non sono sicuro di com'è andata. Riavvia il PC e poi premi di nuovo \"Aggiorna con Tasso\" per controllare.")
	}
}
