package main

// Aggiornamento del driver NVIDIA dentro il Tasso (3.1).
// Regole di sicurezza:
//   - la versione nuova la chiedo al server NVIDIA, il file lo scarico SOLO da download.nvidia.com in https;
//   - prima di installarlo il file resta bloccato (nessuno può cambiarlo) e controllo la firma digitale:
//     deve essere valida e di NVIDIA Corporation, altrimenti lo cancello;
//   - prima di installare: punto di ripristino, niente gioco aperto, niente batteria;
//   - "Torna al driver di prima" riscarica da NVIDIA la versione vecchia, con gli stessi controlli.
// AMD e Intel: i loro programmi ufficiali aggiornano già da soli, il Tasso li apre e basta.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

const gpuDrvMax = 3 << 30 // un pacchetto NVIDIA pesa ~700 MB-1 GB: oltre 3 GB qualcosa non va

type nvGpu struct {
	Name   string `json:"name"`
	DevID  string `json:"devId"`  // es. "2504"
	WinVer string `json:"winVer"` // versione Windows, es. 32.0.15.6124
	Ver    string `json:"ver"`    // versione NVIDIA, es. 561.24
	Laptop bool   `json:"laptop"`
	reboot bool   // Windows aspetta un riavvio
	wu     bool   // Windows Update sta installando qualcosa
}

type nvOffer struct {
	Ver  string `json:"ver"`
	URL  string `json:"url"`
	Date string `json:"date"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// gpuDrvState: salvato in Programmi\Tasso\driver (solo l'amministratore può scriverci).
type gpuDrvState struct {
	PrevVer string `json:"prevVer"`
	PrevURL string `json:"prevURL"`
	NewVer  string `json:"newVer"`
	When    string `json:"when"`
	Reboot  bool   `json:"reboot"`         // installato ma Windows aspetta un riavvio
	Tick    uint64 `json:"tick,omitempty"` // millisecondi da quando è acceso Windows, quando è stato segnato Reboot
	Mark    int64  `json:"mark,omitempty"` // quando è stato segnato Reboot (secondi Unix)
}

// upTick: millisecondi da quando si è acceso Windows. Riparte da zero a ogni riavvio e non cambia
// se si corregge l'orologio.
func upTick() uint64 {
	ms, _, _ := pTick64.Call()
	return uint64(ms)
}

var (
	gdMu     sync.Mutex
	gdCancel context.CancelFunc
	gdOffer  *nvOffer // ultima offerta mostrata: si installa solo quella
	gdVendor string   // "nvidia" o "amd": cosa ha trovato l'ultimo controllo
)

var errNoNvidia = errors.New("Non ho trovato una scheda video NVIDIA.")

var (
	nvDevRe = regexp.MustCompile(`(?i)VEN_10DE&DEV_([0-9A-F]{4})`)
	nvVerRe = regexp.MustCompile(`^\d{3}\.\d{2}$`)
)

// nvVersion trasforma la versione di Windows (32.0.15.6124) in quella NVIDIA (561.24).
func nvVersion(win string) string {
	p := strings.Split(strings.TrimSpace(win), ".")
	if len(p) != 4 {
		return ""
	}
	// Il driver "Scheda video di base Microsoft" ha versioni tipo 10.0.19041.1: non è NVIDIA.
	if maj, err := strconv.Atoi(p[0]); err != nil || maj < 20 {
		return ""
	}
	d := p[2] + p[3]
	if len(d) < 5 {
		return ""
	}
	d = d[len(d)-5:]
	for _, c := range d {
		if c < '0' || c > '9' {
			return ""
		}
	}
	return d[:3] + "." + d[3:]
}

// nvNewer: a è più nuova di b? (formato 561.24)
func nvNewer(a, b string) bool {
	pa, pb := strings.SplitN(a, ".", 2), strings.SplitN(b, ".", 2)
	if len(pa) != 2 || len(pb) != 2 {
		return false
	}
	a1, _ := strconv.Atoi(pa[0])
	a2, _ := strconv.Atoi(pa[1])
	b1, _ := strconv.Atoi(pb[0])
	b2, _ := strconv.Atoi(pb[1])
	return a1 > b1 || (a1 == b1 && a2 > b2)
}

// nvURLOk: solo https e solo i server di download NVIDIA, file .exe.
func nvURLOk(s string) bool {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	if h != "us.download.nvidia.com" && h != "international.download.nvidia.com" && h != "download.nvidia.com" {
		return false
	}
	return strings.HasSuffix(strings.ToLower(u.Path), ".exe") && !strings.Contains(u.Path, "..")
}

// nvPrevURL: l'indirizzo della versione vecchia, uguale a quello nuovo con il numero cambiato
// (NVIDIA usa sempre lo stesso schema: /Windows/561.24/561.24-desktop-...exe).
func nvPrevURL(newURL, newVer, oldVer string) string {
	if !nvVerRe.MatchString(newVer) || !nvVerRe.MatchString(oldVer) || strings.Count(newURL, newVer) != 2 {
		return ""
	}
	u := strings.ReplaceAll(newURL, newVer, oldVer)
	if !nvURLOk(u) {
		return ""
	}
	return u
}

// nvParseOffer legge la risposta del server NVIDIA cercando l'oggetto con Version e DownloadURL.
func nvParseOffer(b []byte) *nvOffer {
	var v interface{}
	if json.Unmarshal(b, &v) != nil {
		return nil
	}
	var found *nvOffer
	var walk func(x interface{})
	walk = func(x interface{}) {
		if found != nil {
			return
		}
		switch t := x.(type) {
		case map[string]interface{}:
			ver, _ := t["Version"].(string)
			u, _ := t["DownloadURL"].(string)
			if ver != "" && u != "" {
				if ok, has := t["Success"]; has && fmt.Sprint(ok) != "1" {
					return
				}
				o := &nvOffer{Ver: strings.TrimSpace(ver), URL: strings.TrimSpace(u)}
				o.Date, _ = t["ReleaseDateTime"].(string)
				o.Name, _ = t["Name"].(string)
				if s, ok := t["DownloadURLFileSize"].(string); ok {
					o.Size, _ = strconv.ParseInt(strings.TrimSpace(s), 10, 64)
				}
				if dn, err := url.QueryUnescape(o.Name); err == nil {
					o.Name = dn
				}
				if nvVerRe.MatchString(o.Ver) && nvURLOk(o.URL) && strings.Contains(o.URL, "/"+o.Ver+"/"+o.Ver+"-") {
					found = o
				}
				return
			}
			for _, y := range t {
				walk(y)
			}
		case []interface{}:
			for _, y := range t {
				walk(y)
			}
		}
	}
	walk(v)
	return found
}

const nvGpuScript = `
$g = @(Get-CimInstance Win32_VideoController -ErrorAction SilentlyContinue | Where-Object { [string]$_.PNPDeviceID -match 'VEN_10DE' } | Select-Object -First 1)
$lap = $false
try { $lap = [bool](@(Get-CimInstance Win32_SystemEnclosure -ErrorAction Stop).ChassisTypes | Where-Object { $_ -in 8,9,10,11,12,14,18,21,30,31,32 }) } catch {}
$rb = (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending') -or (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired')
$wu = [bool](Get-Process -Name TiWorker, TrustedInstaller -ErrorAction SilentlyContinue)
if ($g.Count) { [Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -Compress -InputObject ([ordered]@{ name = [string]$g[0].Name; pnp = [string]$g[0].PNPDeviceID; ver = [string]$g[0].DriverVersion; laptop = $lap; reboot = $rb; wu = $wu }))) }
else { [Console]::Out.WriteLine('JSON {}') }
`

func nvDetect() (*nvGpu, error) {
	r := runPS(nvGpuScript, 90*time.Second, nil)
	var raw struct {
		Name   string `json:"name"`
		PNP    string `json:"pnp"`
		Ver    string `json:"ver"`
		Laptop bool   `json:"laptop"`
		Reboot bool   `json:"reboot"`
		WU     bool   `json:"wu"`
	}
	if r.JSON == "" || json.Unmarshal([]byte(r.JSON), &raw) != nil {
		return nil, errors.New("Non riesco a leggere la scheda video.")
	}
	m := nvDevRe.FindStringSubmatch(raw.PNP)
	if m == nil {
		return nil, errNoNvidia
	}
	g := &nvGpu{Name: strings.TrimSpace(raw.Name), DevID: strings.ToUpper(m[1]), WinVer: raw.Ver, Ver: nvVersion(raw.Ver), Laptop: raw.Laptop, reboot: raw.Reboot, wu: raw.WU}
	if g.Ver == "" {
		return nil, errors.New("Non riesco a leggere la versione del driver NVIDIA (forse il driver NVIDIA non è installato: usa NVIDIA App o il sito NVIDIA).")
	}
	return g, nil
}

func nvHTTP(timeout time.Duration) *http.Client {
	// Niente proxy preso dalle variabili d'ambiente dell'utente: le può cambiare chiunque.
	return &http.Client{Timeout: timeout, Transport: &http.Transport{Proxy: nil, ResponseHeaderTimeout: 30 * time.Second, TLSHandshakeTimeout: 20 * time.Second},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 3 || req.URL.Scheme != "https" || !strings.HasSuffix(strings.ToLower(req.URL.Hostname()), ".nvidia.com") {
				return errors.New("redirect non sicuro")
			}
			return nil
		}}
}

// nvLookup chiede al server NVIDIA il driver più nuovo per questa scheda (stesso servizio che usa NVIDIA).
func nvLookup(g *nvGpu) (*nvOffer, error) {
	v := windows.RtlGetVersion()
	lp := "0"
	if g.Laptop {
		lp = "1"
	}
	q := map[string]interface{}{"dIDa": []string{g.DevID + "_10DE"}, "osC": fmt.Sprintf("%d.%d", v.MajorVersion, v.MinorVersion),
		"osB": strconv.Itoa(int(v.BuildNumber)), "is6": "1", "lg": "1033", "iLp": lp, "prvMd": "0", "gcV": "",
		"gIsB": "1", "dch": "1", "upCRD": "0", "isCRD": "0"}
	b, _ := json.Marshal(q)
	u := "https://gfwsl.geforce.com/nvidia_web_services/controller.gfeclientcontent.NG.php/com.nvidia.services.GFEClientContent_NG.getDispDrvrByDevid/" + url.PathEscape(string(b))
	resp, err := nvHTTP(30 * time.Second).Get(u)
	if err != nil {
		return nil, errors.New("Il server NVIDIA non risponde: controlla internet e riprova.")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Il server NVIDIA ha risposto %d: riprova più tardi.", resp.StatusCode)
	}
	o := nvParseOffer(body)
	if o == nil {
		return nil, errors.New("Non riesco a leggere la risposta di NVIDIA. Usa NVIDIA App per aggiornare.")
	}
	return o, nil
}

func gpuDrvDir() (string, error) {
	pf, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil || pf == "" {
		return "", errors.New("cartella Programmi non trovata")
	}
	return filepath.Join(cartellaTasso(), "driver"), nil // ecosistema.go: di base Programmi\Tasso
}

func loadGpuDrvState() gpuDrvState {
	var s gpuDrvState
	d, err := gpuDrvDir()
	if err != nil {
		return s
	}
	b, err := os.ReadFile(filepath.Join(d, "stato.json"))
	if err != nil || len(b) > 4096 || json.Unmarshal(b, &s) != nil {
		return gpuDrvState{}
	}
	if !nvVerRe.MatchString(s.PrevVer) || !nvVerRe.MatchString(s.NewVer) || (s.PrevURL != "" && !nvURLOk(s.PrevURL)) {
		return gpuDrvState{}
	}
	return s
}

func saveGpuDrvState(s gpuDrvState) {
	d, err := gpuDrvDir()
	if err != nil {
		return
	}
	b, _ := json.Marshal(s)
	_ = os.WriteFile(filepath.Join(d, "stato.json"), b, 0o644)
}

func gdEmit(stage, msg string, extra map[string]interface{}) {
	p := map[string]interface{}{"stage": stage, "msg": msg}
	for k, v := range extra {
		p[k] = v
	}
	emit("onGpuDrv", p)
}

// gpuDrvCheck: c'è un driver nuovo? Emette onGpuDrvInfo.
func gpuDrvCheck() {
	gdMu.Lock()
	gdVendor = ""
	gdMu.Unlock()
	g, err := nvDetect()
	if errors.Is(err, errNoNvidia) {
		ga, aerr := amdDetect()
		if aerr == nil {
			gdMu.Lock()
			gdVendor = "amd"
			gdMu.Unlock()
			amdCheck(ga)
			return
		}
		if errors.Is(aerr, errNoAMD) {
			err = errors.New("Non ho trovato una scheda video NVIDIA o AMD Radeon.")
		} else {
			err = aerr
		}
	}
	if err != nil {
		emit("onGpuDrvInfo", map[string]interface{}{"error": err.Error()})
		return
	}
	o, err := nvLookup(g)
	st := loadGpuDrvState()
	if st.Reboot && st.NewVer == g.Ver {
		st.Reboot = false
		saveGpuDrvState(st)
	} else if st.Reboot && st.Tick == 0 {
		// Stato di una versione di prima senza il tempo di accensione: lo segno ora, così l'avviso si
		// toglie dopo il prossimo riavvio.
		st.Tick, st.Mark = upTick(), time.Now().Unix()
		saveGpuDrvState(st)
	} else if st.Reboot && (upTick() < st.Tick || (st.Mark > 0 && time.Since(time.Unix(st.Mark, 0)) > time.Duration(upTick())*time.Millisecond+time.Minute)) {
		// Riavviato: il tempo di accensione è ripartito da zero, oppure da quando ho segnato l'avviso
		// è passato più tempo di quanto Windows è acceso adesso.
		// Il PC è stato riavviato ma il driver è ancora quello di prima: l'installazione non è
		// andata davvero. Tolgo l'avviso "serve un riavvio", così si può riprovare.
		st.Reboot, st.Tick, st.Mark = false, 0, 0
		saveGpuDrvState(st)
	}
	gdMu.Lock()
	gdVendor = "nvidia"
	gdMu.Unlock()
	out := map[string]interface{}{"vendor": "nvidia", "gpu": g, "canUndo": st.PrevVer != "" && st.NewVer == g.Ver, "prevVer": st.PrevVer,
		"undoURL": st.PrevURL != "", "rebootFor": ""}
	if st.Reboot && st.NewVer != g.Ver {
		out["rebootFor"] = st.NewVer // installato, manca il riavvio: non lo riproporre
	}
	if err != nil {
		out["error"] = err.Error()
	} else {
		out["offer"] = o
		out["newer"] = nvNewer(o.Ver, g.Ver)
	}
	gdMu.Lock()
	gdOffer = nil
	if err == nil {
		gdOffer = o
	}
	gdMu.Unlock()
	emit("onGpuDrvInfo", out)
}

// freeBytes: spazio libero sul disco della cartella.
func freeBytes(dir string) uint64 {
	var free uint64
	p, _ := windows.UTF16PtrFromString(dir)
	if windows.GetDiskFreeSpaceEx(p, &free, nil, nil) != nil {
		return 0
	}
	return free
}

// gdDownload scarica in Programmi\Tasso\driver con la barra di avanzamento; Annulla ferma tutto.
func gdDownload(ctx context.Context, u, ver string) (string, error) {
	if !nvURLOk(u) {
		return "", errors.New("Indirizzo del driver non sicuro: non scarico.")
	}
	dir, err := gpuDrvDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, "nvidia-"+ver+".exe")
	if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() && fi.Size() > 1<<20 {
		return p, nil // già scaricato (es. dopo "installa senza backup"): la firma la controllo comunque
	}
	// Tolgo i pacchetti vecchi scaricati da me (solo i miei nomi).
	if old, _ := filepath.Glob(filepath.Join(dir, "nvidia-*.exe*")); old != nil {
		for _, f := range old {
			_ = os.Remove(f)
		}
	}
	// Se per 60 secondi non arriva niente, il download si ferma (invece di restare appeso).
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	idle := time.AfterFunc(60*time.Second, stop)
	defer idle.Stop()
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, err := nvHTTP(0).Do(req)
	if err != nil {
		return "", errors.New("Download non riuscito: controlla internet e riprova.")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("NVIDIA ha risposto %d: questa versione non si può scaricare.", resp.StatusCode)
	}
	if !nvURLOk(resp.Request.URL.String()) {
		return "", errors.New("Il download è finito su un server diverso da NVIDIA: non scarico.")
	}
	total := resp.ContentLength
	if total > gpuDrvMax {
		return "", errors.New("Il file è troppo grande: non scarico.")
	}
	need := uint64(4 << 30)
	if total > 0 {
		need = uint64(total)*3 + 2<<30 // file + estrazione + installazione
	}
	if fr := freeBytes(dir); fr > 0 && fr < need {
		return "", fmt.Errorf("Spazio libero insufficiente: servono circa %d GB liberi sul disco C:.", need>>30+1)
	}
	tmp := filepath.Join(dir, "nvidia-"+ver+".exe.part")
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", errors.New("Non riesco a salvare il file (forse l'antivirus lo blocca).")
	}
	buf := make([]byte, 256*1024)
	var done int64
	last := time.Time{}
	for {
		n, rerr := resp.Body.Read(buf)
		idle.Reset(60 * time.Second)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				os.Remove(tmp)
				return "", errors.New("Non riesco a salvare il file: forse il disco è pieno.")
			}
			done += int64(n)
			if done > gpuDrvMax {
				f.Close()
				os.Remove(tmp)
				return "", errors.New("Il file è troppo grande: non scarico.")
			}
			if time.Since(last) > 400*time.Millisecond {
				last = time.Now()
				pct := -1
				if total > 0 {
					pct = int(done * 100 / total)
				}
				gdEmit("download", "", map[string]interface{}{"pct": pct, "done": done, "total": total})
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			os.Remove(tmp)
			return "", errors.New("Download interrotto: controlla internet e riprova.")
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return "", errors.New("Non riesco a salvare il file: forse il disco è pieno.")
	}
	if total > 0 && done != total {
		os.Remove(tmp)
		return "", errors.New("Download incompleto: riprova.")
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return "", errors.New("Non riesco a salvare il file (forse l'antivirus lo blocca).")
	}
	return p, nil
}

const nvSigScript = `
$s = Get-AuthenticodeSignature -LiteralPath ([string]$A.p) -ErrorAction SilentlyContinue
$o = [ordered]@{ status = 'Error'; rdn = @(); machineRoot = $false }
if ($s) {
  $o.status = [string]$s.Status
  $c = $s.SignerCertificate
  if ($c) {
    # Un pezzo del nome per riga (le virgole dentro le virgolette non contano): niente trucchi col nome.
    $o.rdn = @(($c.SubjectName.Decode([Security.Cryptography.X509Certificates.X500DistinguishedNameFlags]::UseNewLines)) -split "\r?\n" | Where-Object { $_ })
    # La radice della catena deve essere tra quelle del computer, non una aggiunta dall'utente.
    $ch = New-Object Security.Cryptography.X509Certificates.X509Chain
    $ch.ChainPolicy.RevocationMode = 'NoCheck'
    [void]$ch.Build($c)
    if ($ch.ChainElements.Count -gt 0) {
      $root = $ch.ChainElements[$ch.ChainElements.Count - 1].Certificate
      $o.machineRoot = Test-Path -LiteralPath ('Cert:\LocalMachine\Root\' + $root.Thumbprint)
    }
  }
}
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject $o -Compress))
`

type nvSig struct {
	Status      string   `json:"status"`
	RDN         []string `json:"rdn"`
	MachineRoot bool     `json:"machineRoot"`
}

// nvSigOk: firma valida, nome esatto "NVIDIA Corporation" (CN e O), radice del computer.
func nvSigOk(s nvSig) bool {
	if s.Status != "Valid" || !s.MachineRoot {
		return false
	}
	cn, o := false, false
	for _, r := range s.RDN {
		switch strings.TrimSpace(r) {
		case "CN=NVIDIA Corporation", `CN="NVIDIA Corporation"`:
			cn = true
		case "O=NVIDIA Corporation", `O="NVIDIA Corporation"`:
			o = true
		default:
			k := strings.SplitN(strings.TrimSpace(r), "=", 2)[0]
			if k == "CN" || k == "O" {
				return false // un secondo CN/O diverso: non mi fido
			}
		}
	}
	return cn && o
}

// gdLockVerify blocca il file (solo lettura per gli altri) e controlla la firma.
// La maniglia resta aperta finché l'installazione non è partita.
func gdLockVerify(p string) (windows.Handle, error) {
	u, _ := windows.UTF16PtrFromString(p)
	h, err := windows.CreateFile(u, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return 0, errors.New("Non riesco ad aprire il file scaricato.")
	}
	r := runPSArgs(nvSigScript, map[string]string{"p": p}, 3*time.Minute, nil)
	var s nvSig
	if r.JSON == "" || json.Unmarshal([]byte(r.JSON), &s) != nil || !nvSigOk(s) {
		windows.CloseHandle(h)
		_ = os.Remove(p)
		return 0, errors.New("La firma digitale del file non è di NVIDIA o non è valida: l'ho cancellato e non installo niente.")
	}
	return h, nil
}

const nvRestoreScript = `
$key = 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\SystemRestore'
# "Protezione sistema" spenta (RPSessionInterval = 0): la accendo per poter fare il punto di ripristino,
# ma lo dico (srOff nel risultato, Tasso lo scrive nella finestra).
$srOff = $false
try { $ri = (Get-ItemProperty -LiteralPath $key -Name 'RPSessionInterval' -ErrorAction Stop).RPSessionInterval; if ([int]$ri -eq 0) { $srOff = $true } } catch {}
try { Enable-ComputerRestore -Drive ($TassoSysDrive + '\') -ErrorAction Stop } catch {}
# "L'ho accesa" solo se adesso è accesa davvero (un criterio dell'azienda può impedirlo).
if ($srOff) { try { $ri = (Get-ItemProperty -LiteralPath $key -Name 'RPSessionInterval' -ErrorAction Stop).RPSessionInterval; if ([int]$ri -eq 0) { $srOff = $false } } catch { $srOff = $false } }
$oldFreq = (Get-ItemProperty -Path $key -Name 'SystemRestorePointCreationFrequency' -ErrorAction SilentlyContinue).SystemRestorePointCreationFrequency
New-ItemProperty -Path $key -Name 'SystemRestorePointCreationFrequency' -Value 0 -PropertyType DWord -Force | Out-Null
$before = @(Get-ComputerRestorePoint).Count
try { Checkpoint-Computer -Description ('Tasso - prima del driver ' + [string]$A.v) -RestorePointType DEVICE_DRIVER_INSTALL -ErrorAction Stop } catch {}
$after = @(Get-ComputerRestorePoint).Count
if ($null -ne $oldFreq) { New-ItemProperty -Path $key -Name 'SystemRestorePointCreationFrequency' -Value ([int]$oldFreq) -PropertyType DWord -Force | Out-Null } else { Remove-ItemProperty -Path $key -Name 'SystemRestorePointCreationFrequency' -ErrorAction SilentlyContinue }
$res = 'none'
if ($after -gt $before) { $res = 'new' }
elseif ($after -gt 0) {
  $last = @(Get-ComputerRestorePoint | Sort-Object SequenceNumber)[-1]
  try { $when = [Management.ManagementDateTimeConverter]::ToDateTime([string]$last.CreationTime); if ($when -gt (Get-Date).AddHours(-2)) { $res = 'recent' } } catch {}
}
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -Compress -InputObject ([ordered]@{ res = $res; srOff = [bool]$srOff })))
`

// notaProtezioneAccesa: da aggiungere al messaggio quando Tasso ha dovuto accendere "Protezione sistema"
// per fare il punto di ripristino (prima la accendeva senza dirlo). Comincia con lo spazio: si attacca
// in fondo a un'altra frase e si traduce come pezzo.
const notaProtezioneAccesa = " Protezione sistema era spenta: l'ho accesa per poter fare il punto di ripristino (usa un po' di spazio del disco). Si rispegne da Pannello di controllo > Sistema > Protezione sistema > Configura."

// gdRestorePointNota: fa il punto di ripristino. Torna "new" (fatto adesso), "recent" (ce n'è uno
// di meno di 2 ore fa) o "none", e la nota da mostrare se ha dovuto accendere "Protezione sistema"
// ("" se era già accesa).
func gdRestorePointNota(label string) (string, string) {
	r := runPSArgs(nvRestoreScript, map[string]string{"v": label}, 10*time.Minute, nil)
	var o struct {
		Res   string `json:"res"`
		SrOff bool   `json:"srOff"`
	}
	if r.JSON == "" || json.Unmarshal([]byte(r.JSON), &o) != nil {
		return "none", ""
	}
	nota := ""
	if o.SrOff {
		nota = notaProtezioneAccesa
	}
	return o.Res, nota
}

// gdBlocker: motivi per non installare adesso (anche per le prove).
func gdBlocker(battery bool, game string, reboot, wu bool) string {
	if reboot {
		return "Windows aspetta un riavvio per finire un aggiornamento: riavvia il PC e poi aggiorna il driver."
	}
	if wu {
		return "Windows sta installando degli aggiornamenti proprio adesso: riprova tra un po'."
	}
	if battery {
		return "Sei a batteria: collega il caricatore, l'installazione non si deve interrompere."
	}
	if game != "" {
		return "Chiudi " + game + " prima di aggiornare il driver."
	}
	return ""
}

// gpuDrvInstall: scarica, controlla, fa il backup e installa. undo=true torna alla versione di prima.
// noBackup=true solo se l'utente ha scelto di continuare senza punto di ripristino.
func gpuDrvInstall(undo, noBackup bool) {
	fail := func(msg string) { gdEmit("error", msg, nil) }
	gdMu.Lock()
	v := gdVendor
	gdMu.Unlock()
	if v == "amd" {
		if undo {
			fail("Per AMD si torna indietro con il punto di ripristino, oppure in Gestione dispositivi → scheda video → Ripristina driver.")
			return
		}
		amdInstall(noBackup)
		return
	}
	// Occupo il posto delle pulizie: nessuna pulizia (che tocca C:\NVIDIA e i driver vecchi)
	// può partire mentre installo, e viceversa.
	if !claimRun() {
		fail("Sta girando una pulizia o un aggiornamento di Tasso: aspetta che finisca.")
		return
	}
	defer releaseRun()
	g, err := nvDetect()
	if err != nil {
		fail(err.Error())
		return
	}
	if b := gdBlocker(onBattery(), safeName(runningGame()), g.reboot, g.wu); b != "" {
		fail(b)
		return
	}
	var ver, u string
	st := loadGpuDrvState()
	if undo {
		if st.PrevVer == "" || st.NewVer != g.Ver {
			fail("Non c'è un driver di prima a cui tornare.")
			return
		}
		if st.PrevURL == "" {
			fail(trGo("Il driver di prima (" + st.PrevVer + ") non si può riscaricare da NVIDIA. Per tornare indietro usa il punto di ripristino del " + st.When + ".")) // data con /: la traduco qui
			return
		}
		ver, u = st.PrevVer, st.PrevURL
	} else {
		gdMu.Lock()
		o := gdOffer
		gdMu.Unlock()
		if o == nil || !nvNewer(o.Ver, g.Ver) {
			fail("Hai già il driver più nuovo.")
			return
		}
		if st.Reboot && st.NewVer == o.Ver {
			fail("Il driver " + o.Ver + " è già installato: riavvia il PC per finire.")
			return
		}
		ver, u = o.Ver, o.URL
	}
	ctx, cancel := context.WithCancel(context.Background())
	gdMu.Lock()
	gdCancel = cancel
	gdMu.Unlock()
	defer func() {
		gdMu.Lock()
		gdCancel = nil
		gdMu.Unlock()
		cancel()
	}()
	gdEmit("download", "", map[string]interface{}{"pct": 0, "from": g.Ver, "to": ver})
	p, err := gdDownload(ctx, u, ver)
	if ctx.Err() != nil {
		gdEmit("cancel", "Download annullato: il driver non è cambiato.", nil)
		return
	}
	if err != nil {
		fail(err.Error())
		return
	}
	gdEmit("verify", "Controllo la firma digitale di NVIDIA…", nil)
	h, err := gdLockVerify(p)
	if err != nil {
		fail(err.Error())
		return
	}
	defer func() {
		if h != 0 {
			windows.CloseHandle(h)
		}
	}()
	if ctx.Err() != nil {
		gdEmit("cancel", "Annullato: il driver non è cambiato.", nil)
		return
	}
	backupMsg := ""
	if !noBackup {
		gdEmit("backup", "Creo il punto di ripristino…", nil)
		rp, nota := gdRestorePointNota("NVIDIA " + ver)
		backupMsg = nota // Protezione sistema accesa da Tasso: lo dico nel messaggio dell'installazione
		switch rp {
		case "new":
		case "recent":
			backupMsg = " Uso il punto di ripristino creato poco fa." + nota
		default:
			if ctx.Err() == nil {
				gdEmit("nobackup", "Non sono riuscito a creare il punto di ripristino. Vuoi installare lo stesso?", map[string]interface{}{"undo": undo, "undoURL": !undo && nvPrevURL(u, ver, g.Ver) != ""})
				return
			}
		}
	}
	if ctx.Err() != nil {
		gdEmit("cancel", "Annullato: il driver non è cambiato.", nil)
		return
	}
	// Controllo di nuovo poco prima di installare: nel frattempo può essere partito un gioco.
	if b := gdBlocker(onBattery(), safeName(runningGame()), false, false); b != "" {
		fail(b)
		return
	}
	prevURL := ""
	if !undo {
		prevURL = nvPrevURL(u, ver, g.Ver)
		if prevURL != "" && !nvExists(prevURL) {
			prevURL = "" // la versione di prima non è più sul sito NVIDIA: resta il punto di ripristino
		}
	}
	gdEmit("install", "Installo il driver "+ver+"."+backupMsg+" Lo schermo può diventare nero per qualche secondo: è normale. Non spegnere e non riavviare il PC.", nil)
	// Salvo lo stato PRIMA di installare: se va via la corrente o chiudi il Tasso, al prossimo
	// controllo so comunque com'era prima.
	when := time.Now().Format("02/01/2006 15:04")
	if undo {
		saveGpuDrvState(gpuDrvState{PrevVer: st.NewVer, NewVer: ver, When: when})
	} else {
		saveGpuDrvState(gpuDrvState{PrevVer: g.Ver, PrevURL: prevURL, NewVer: ver, When: when})
	}
	// -s = senza finestre, -noreboot = non riavvia da solo, -noeula = niente schermata licenza.
	// Niente installazione pulita (-clean): di solito le impostazioni del pannello NVIDIA restano.
	dir := filepath.Dir(p)
	cmd := exec.Command(p, "-s", "-noreboot", "-noeula")
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		saveGpuDrvState(st)
		fail("Non riesco ad avviare l'installazione NVIDIA.")
		return
	}
	windows.CloseHandle(h)
	h = 0
	// Non lo fermo mai a metà: un driver interrotto è peggio di uno lento.
	werr := cmd.Wait()
	code := 0
	if ee, ok := werr.(*exec.ExitError); ok {
		code = ee.ExitCode()
	}
	// La prova vera: che versione c'è adesso? Con codice 0 il pacchetto a volte finisce prima
	// dell'installazione vera, quindi aspetto fino a 15 minuti che Windows mostri la versione nuova.
	// Con 1 o 3010 serve il riavvio: lo dico subito. Con un altro codice è andata male: al massimo
	// un minuto (per sicurezza) e poi lo dico, senza far aspettare un quarto d'ora per niente.
	wait := time.Minute
	switch code {
	case 0:
		wait = 15 * time.Minute
	case 1, 3010:
		wait = 0
	}
	var now string
	deadline := time.Now().Add(wait)
	for {
		if g2, e := nvDetect(); e == nil {
			now = g2.Ver
			if now == ver {
				break
			}
		}
		if !time.Now().Before(deadline) {
			break
		}
		time.Sleep(10 * time.Second)
	}
	_ = os.Remove(p)
	if now == ver {
		if undo {
			saveGpuDrvState(gpuDrvState{})
		}
		gdEmit("done", "Fatto: driver NVIDIA "+ver+" installato.", map[string]interface{}{"undo": undo})
		return
	}
	if code == 0 || code == 1 || code == 3010 {
		// Il programma NVIDIA dice che è andata, ma Windows mostra ancora la versione vecchia:
		// di solito manca solo il riavvio.
		cur := loadGpuDrvState()
		cur.Reboot, cur.Tick, cur.Mark = true, upTick(), time.Now().Unix()
		saveGpuDrvState(cur)
		gdEmit("done", "Installazione finita, ma Windows mostra ancora il driver di prima. Riavvia il PC: poi controllo di nuovo.", map[string]interface{}{"reboot": true, "undo": undo})
		return
	}
	saveGpuDrvState(st) // non è cambiato niente: rimetto lo stato di prima
	msg := fmt.Sprintf("L'installazione NVIDIA non è andata (codice %d).", code)
	if now == g.Ver {
		msg += " Hai ancora il driver " + now + "."
	}
	fail(msg + " Se qualcosa non va, usa il punto di ripristino.")
}

// nvExists: il file c'è ancora sul sito NVIDIA? (solo intestazioni, niente download)
func nvExists(u string) bool {
	if !nvURLOk(u) {
		return false
	}
	req, _ := http.NewRequest("HEAD", u, nil)
	resp, err := nvHTTP(20 * time.Second).Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200 && nvURLOk(resp.Request.URL.String())
}

func gpuDrvCancel() {
	gdMu.Lock()
	if gdCancel != nil {
		gdCancel()
	}
	gdMu.Unlock()
}

func bindGpuDrv(w webview2.WebView) {
	_ = w.Bind("goGpuDrvCheck", func() string { return async("gpudrvcheck", gpuDrvCheck) })
	_ = w.Bind("goGpuDrvInstall", func(undo, noBackup bool) string {
		return async("gpudrv", func() { gpuDrvInstall(undo, noBackup) })
	})
	_ = w.Bind("goGpuDrvCancel", func() string { gpuDrvCancel(); return "ok" })
}
