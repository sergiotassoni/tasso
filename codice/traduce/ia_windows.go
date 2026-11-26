//go:build windows

package traduce

// Gestione del motore potente sul PC: controllo della scheda video, avvio di llama-server
// (llama.cpp, licenza MIT, versione Vulkan per NVIDIA e AMD) solo su 127.0.0.1.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const scriptGPU = `$ErrorActionPreference = 'SilentlyContinue'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$best = $null
$base = 'HKLM:\SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}'
Get-ChildItem $base | Where-Object { $_.PSChildName -match '^\d{4}$' } | ForEach-Object {
  $p = Get-ItemProperty $_.PSPath
  $mem = $p.'HardwareInformation.qwMemorySize'
  if (-not $mem) { $mem = $p.'HardwareInformation.MemorySize' }
  if ($mem -is [byte[]]) { $mem = [BitConverter]::ToUInt64(($mem + (New-Object byte[] 8))[0..7], 0) }
  if ($p.DriverDesc -and $mem -and ((-not $best) -or ([double]$mem -gt $best.mem))) { $best = @{ nome = [string]$p.DriverDesc; mem = [double]$mem } }
}
$usata = 0
try {
  $c = Get-Counter '\GPU Adapter Memory(*)\Dedicated Usage' -ErrorAction Stop
  $usata = ($c.CounterSamples | Measure-Object -Property CookedValue -Maximum).Maximum
} catch {}
if ($best) { [Console]::Out.WriteLine((ConvertTo-Json -Compress @{ nome = $best.nome; mem = $best.mem; usata = [double]$usata })) }
else { [Console]::Out.WriteLine('{}') }
`

const gb = 1024 * 1024 * 1024

func psUnaVolta(script string, attesa time.Duration) (string, error) {
	dir := cartellaScript()
	_ = os.MkdirAll(dir, 0o755)
	f := filepath.Join(dir, fmt.Sprintf("tt_%d_%d.ps1", os.Getpid(), time.Now().UnixNano()))
	if err := scriviNuovo(f, []byte("\uFEFF"+strings.ReplaceAll(script, "\n", "\r\n"))); err != nil {
		return "", err
	}
	defer togliScriptTT(f) // anche se Windows lo tiene aperto ancora per un attimo
	ctx, cancel := context.WithTimeout(context.Background(), attesa)
	defer cancel()
	cmd := exec.CommandContext(ctx, sysExe("powershell.exe"), "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", f)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000 | 0x00004000} // priorità sotto la normale: il gioco viene prima
	out, err := cmd.Output()
	return strings.TrimSpace(strings.TrimPrefix(string(out), "\uFEFF")), err
}

// controllaGPU: nome e memoria della scheda video (letti una volta sola con PowerShell: non
// cambiano) e memoria usata adesso. La memoria usata si legge con i contatori di Windows (PDH,
// come Gestione attività), senza aprire PowerShell ogni 15 secondi mentre si gioca; se i
// contatori non vanno, come prima con PowerShell.
func controllaGPU() InfoGPU {
	muGPU.Lock()
	defer muGPU.Unlock()
	if gpuFissa != nil {
		if usata, ok := memoriaUsataPDH(); ok {
			g := *gpuFissa
			g.UsataGB = usata / gb
			return valutaGPU(g)
		}
	}
	out, err := psUnaVolta(scriptGPU, 30*time.Second)
	var r struct {
		Nome  string  `json:"nome"`
		Mem   float64 `json:"mem"`
		Usata float64 `json:"usata"`
	}
	if err != nil || json.Unmarshal([]byte(out), &r) != nil || r.Nome == "" {
		return InfoGPU{Motivo: "Non ho trovato una scheda video dedicata."}
	}
	gpuFissa = &InfoGPU{Nome: r.Nome, MemoriaGB: r.Mem / gb}
	g := InfoGPU{Nome: r.Nome, MemoriaGB: r.Mem / gb, UsataGB: r.Usata / gb}
	return valutaGPU(g)
}

// ---- memoria video usata con i contatori di Windows (PDH)

var (
	muGPU    sync.Mutex
	gpuFissa *InfoGPU // nome e memoria totale della scheda, letti la prima volta

	dllPdh             = windows.NewLazySystemDLL("pdh.dll")
	pPdhApri           = dllPdh.NewProc("PdhOpenQueryW")
	pPdhAggiungi       = dllPdh.NewProc("PdhAddEnglishCounterW")
	pPdhRaccogli       = dllPdh.NewProc("PdhCollectQueryData")
	pPdhValori         = dllPdh.NewProc("PdhGetFormattedCounterArrayW")
	pdhMemQuery        uintptr
	pdhMemContatore    uintptr
	pdhMemStato        int // 0 = da preparare, 1 = pronto, -1 = non funziona su questo PC
	pdhMemFalliteDiSeg int
)

// voce di PDH_FMT_COUNTERVALUE_ITEM_W con PDH_FMT_DOUBLE (64 bit): nome, stato, valore
type pdhVoce struct {
	nome   *uint16
	stato  uint32
	_      uint32
	valore float64
}

// memoriaUsataPDH: byte di memoria video dedicata usati adesso (la scheda più piena), come il
// contatore "\GPU Adapter Memory(*)\Dedicated Usage" letto prima con Get-Counter.
// Da chiamare con muGPU preso.
func memoriaUsataPDH() (float64, bool) {
	if pdhMemStato < 0 {
		return 0, false
	}
	if pdhMemStato == 0 {
		pdhMemStato = -1
		if pPdhApri.Find() != nil {
			return 0, false
		}
		if r, _, _ := pPdhApri.Call(0, 0, uintptr(unsafe.Pointer(&pdhMemQuery))); r != 0 {
			return 0, false
		}
		percorso, _ := windows.UTF16PtrFromString(`\GPU Adapter Memory(*)\Dedicated Usage`)
		if r, _, _ := pPdhAggiungi.Call(pdhMemQuery, uintptr(unsafe.Pointer(percorso)), 0, uintptr(unsafe.Pointer(&pdhMemContatore))); r != 0 {
			return 0, false
		}
		pdhMemStato = 1
	}
	if r, _, _ := pPdhRaccogli.Call(pdhMemQuery); r != 0 {
		return 0, pdhSbagliato()
	}
	const pdhFmtDouble = 0x200
	var dim, quante uint32
	pPdhValori.Call(pdhMemContatore, pdhFmtDouble, uintptr(unsafe.Pointer(&dim)), uintptr(unsafe.Pointer(&quante)), 0)
	if dim == 0 || dim > 1<<20 {
		return 0, pdhSbagliato()
	}
	buf := make([]uint64, (dim+7)/8) // allineato a 8 byte per i numeri
	if r, _, _ := pPdhValori.Call(pdhMemContatore, pdhFmtDouble, uintptr(unsafe.Pointer(&dim)), uintptr(unsafe.Pointer(&quante)), uintptr(unsafe.Pointer(&buf[0]))); r != 0 || quante == 0 {
		return 0, pdhSbagliato()
	}
	voci := unsafe.Slice((*pdhVoce)(unsafe.Pointer(&buf[0])), quante)
	usata, valide := 0.0, 0
	for _, v := range voci {
		if v.stato == 0 || v.stato == 1 { // PDH_CSTATUS_VALID_DATA / NEW_DATA
			valide++
			usata = max(usata, v.valore)
		}
	}
	if valide == 0 {
		return 0, pdhSbagliato()
	}
	pdhMemFalliteDiSeg = 0
	return usata, true
}

// pdhSbagliato: una lettura non riuscita (si usa PowerShell); dopo 5 di fila i contatori non si
// usano più in questa sessione. Restituisce sempre false.
func pdhSbagliato() bool {
	if pdhMemFalliteDiSeg++; pdhMemFalliteDiSeg >= 5 {
		pdhMemStato = -1
	}
	return false
}

// ---------------------------------------------------------------- llama-server

// StratiNessuno: il correttore gira solo sul processore.
const StratiNessuno = -100

// impostaProfilo prepara il server secondo il profilo scelto da sceltaCorrettore.
func (s *ServerIA) impostaProfilo(p ProfiloCorrettore) {
	s.Contesto = 1024
	switch {
	case p.Strati == 0:
		s.Strati = StratiNessuno
	case p.Strati > 0:
		s.Strati = p.Strati
	default:
		s.Strati = 0
	}
}

type ServerIA struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	uscito chan struct{} // si chiude quando il processo di adesso è uscito davvero
	morti  atomic.Int64  // quante volte il motore si è chiuso da solo (non fermato da Tasso)
	Client *ClientIA
	// Strati: quanti strati del modello mettere sulla scheda video. 0 = tutti (valore di base);
	// usare StratiNessuno per farlo girare solo sul processore.
	Strati int
	// Contesto: memoria di lavoro del modello in token. Il correttore usa 1024: meno memoria video.
	Contesto int
}

func chiaveCasuale() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func portaLibera() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// opzioniVelociKO: in questa sessione il motore non è partito con le opzioni di velocità
// (opzioniVelociIA): da qui in poi si avvia senza, come prima.
var opzioniVelociKO atomic.Bool

// errChiusoAllAvvio: il processo è uscito da solo mentre caricava il modello.
var errChiusoAllAvvio = errors.New("il motore IA si è chiuso all'avvio (vedi logs\\motore-ia.txt)")

// Avvia llama-server con il modello. Solo indirizzo locale, niente interfaccia web.
func (s *ServerIA) Avvia(dirMotore, fileModello string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd != nil {
		return nil
	}
	exe := filepath.Join(dirMotore, "llama-server.exe")
	if _, err := os.Stat(exe); err != nil {
		return errors.New("motore IA non installato")
	}
	dirAvvio, notaCopia := dirMotore, ""
	if dirMotore == dirMotoreIA() {
		// la build nuova o quella di prima (riserva): tutti i file devono essere della stessa
		impronte := impronteLlamaDi(dirMotore)
		if err := verificaMotore(dirMotore, impronte, "llama-server.exe"); err != nil {
			return err
		}
		// la cartella dei modelli la può cambiare qualsiasi programma dell'utente: copio il motore
		// controllato dove scrivono solo gli amministratori e lo avvio da lì (ricontrollato)
		// (llama-server gira da amministratore: se la copia non riesce NON parte dalla cartella
		// dei modelli, che qualsiasi programma può cambiare tra il controllo e l'avvio: niente IA)
		d, err := copiaMotoreProtetto(dirMotore, impronte, "llama-server.exe")
		if err != nil {
			return errors.New("IA non disponibile: copia nella cartella protetta non riuscita (" + err.Error() + ")")
		}
		dirAvvio = d
	}
	exe = filepath.Join(dirAvvio, "llama-server.exe")
	nctx := s.Contesto
	if nctx <= 0 {
		nctx = 4096
	}
	ngl := "99" // tutto sulla scheda video
	switch {
	case s.Strati == StratiNessuno:
		ngl = "0"
	case s.Strati > 0:
		ngl = fmt.Sprint(s.Strati)
	}
	base := []string{
		"-m", fileModello,
		"-ngl", ngl,
		"-c", fmt.Sprint(nctx), "-np", "1",
		"--no-webui", "--no-slots",
	}
	var veloci []string
	if !opzioniVelociKO.Load() {
		veloci = opzioniVelociIA(os.Getenv)
	}
	err := s.avviaProcesso(exe, dirAvvio, append(base, veloci...), notaCopia, false)
	if errors.Is(err, errChiusoAllAvvio) && len(veloci) > 0 && s.cmd == nil {
		// forse la scheda (o il driver) non regge flash attention o la memoria compressa:
		// riprovo subito come prima, e in questa sessione non le uso più
		opzioniVelociKO.Store(true)
		nota := "\r\n---- chiuso all'avvio con " + strings.Join(veloci, " ") + ": riprovo senza.\r\n"
		err = s.avviaProcesso(exe, dirAvvio, base, nota, true)
	}
	return err
}

// avviaProcesso avvia llama-server con gli argomenti dati e aspetta che il modello sia caricato.
// Si chiama con s.mu preso e torna con s.mu preso (lo lascia mentre aspetta il caricamento).
// aggiungi: il registro continua quello di prima invece di ricominciare.
func (s *ServerIA) avviaProcesso(exe, dirAvvio string, args []string, nota string, aggiungi bool) error {
	porta, err := portaLibera()
	if err != nil {
		return err
	}
	tutti := append([]string{"--host", "127.0.0.1", "--port", fmt.Sprint(porta)}, args...)
	cmd := exec.Command(exe, tutti...)
	// chiave segreta per questa sessione (variabile d'ambiente, non visibile nella riga di comando):
	// così né le pagine web né altri programmi del PC possono usare il motore
	chiave := chiaveCasuale()
	cmd.Env = append(os.Environ(), "LLAMA_API_KEY="+chiave)
	cmd.Dir = dirAvvio
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000 | 0x00004000} // nessuna finestra, priorità sotto la normale
	modo := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if aggiungi {
		modo = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	}
	logf, _ := os.OpenFile(filepath.Join(dirApp, "logs", "motore-ia.txt"), modo, 0o644)
	if logf != nil && nota != "" {
		_, _ = logf.WriteString(nota)
	}
	if logf != nil {
		cmd.Stdout, cmd.Stderr = logf, logf
	}
	if err := cmd.Start(); err != nil {
		if logf != nil {
			logf.Close()
		}
		return err
	}
	morti0 := s.morti.Load()
	job := jobChiudiConMe(cmd.Process.Pid)
	uscito := make(chan struct{})
	s.cmd, s.uscito = cmd, uscito
	client := &ClientIA{URL: fmt.Sprintf("http://127.0.0.1:%d", porta), Chiave: chiave}
	go func() {
		defer guard("server-ia")
		_ = cmd.Wait()
		if logf != nil {
			logf.Close()
		}
		if job != 0 {
			windows.CloseHandle(job) // la protezione "chiudi con Tasso" non serve più: niente handle lasciati aperti
		}
		s.mu.Lock()
		if s.cmd == cmd { // nessuno l'ha fermato: si è chiuso da solo
			s.cmd, s.Client, s.uscito = nil, nil, nil
			s.morti.Add(1)
		}
		s.mu.Unlock()
		close(uscito)
	}()
	s.mu.Unlock() // non tengo il lucchetto mentre aspetto il caricamento
	defer s.mu.Lock()
	// aspetto che il modello sia caricato (con una scheda da gaming: pochi secondi)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	for {
		if client.Pronto(ctx) {
			s.mu.Lock()
			mio := s.cmd == cmd
			if mio {
				s.Client = client
			}
			s.mu.Unlock()
			if !mio {
				return errors.New("motore IA fermato mentre si avviava")
			}
			return nil
		}
		select {
		case <-uscito:
			if s.morti.Load() == morti0 { // fermato da Tasso (Ferma), non chiuso da solo
				return errors.New("motore IA fermato mentre si avviava")
			}
			return errChiusoAllAvvio
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return errors.New("il motore IA ci mette troppo ad avviarsi")
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// cartellaProtettaIA: <Programmi>\Tasso\lavoro\ia. Programmi è quella vera (KnownFolderPath),
// mai la variabile d'ambiente, che un programma senza privilegi può cambiare. Dentro Tasso la
// cartella di Tasso la dice Tasso (Ospite.CartellaTasso): un'altra cartella dentro Programmi se
// l'installatore l'ha messo lì. Restituisce la cartella di Tasso e quella del motore.
func cartellaProtettaIA() (tasso, dest string, err error) { return cartellaProtetta("ia") }

// cartellaProtetta: <Programmi>\Tasso\lavoro\<sotto> (sotto: "ia" per llama, "voci" per whisper).
func cartellaProtetta(sotto string) (tasso, dest string, err error) {
	pf, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil || pf == "" {
		return "", "", errors.New("cartella Programmi non trovata")
	}
	tasso = filepath.Join(pf, "Tasso")
	if Ospite.CartellaTasso != nil {
		if d := Ospite.CartellaTasso(); d != "" && filepath.IsAbs(d) {
			tasso = filepath.Clean(d)
		}
	}
	return tasso, filepath.Join(tasso, "lavoro", sotto), nil
}

// copiaMotoreProtetto: copia programma e librerie del motore (già controllati in dir) nella
// cartella protetta, dove solo gli amministratori possono scrivere, e ricontrolla la copia con
// le impronte. Così nessuno può cambiare i file tra il controllo e l'avvio. I file già giusti
// non si ricopiano. Restituisce la cartella da cui avviare.
func copiaMotoreProtetto(dir string, impronte map[string]string, exe string) (string, error) {
	return copiaMotoreProtettoIn("ia", dir, impronte, exe)
}

// copiaMotoreProtettoIn: come copiaMotoreProtetto, nella sottocartella sotto di lavoro.
func copiaMotoreProtettoIn(sotto, dir string, impronte map[string]string, exe string) (string, error) {
	tasso, dest, err := cartellaProtetta(sotto)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", err
	}
	// ogni pezzo del percorso dentro Programmi deve essere una cartella vera, non un collegamento
	for _, d := range []string{tasso, filepath.Join(tasso, "lavoro"), dest} {
		fi, err := os.Lstat(d)
		if err != nil || fi.Mode().Type() != fs.ModeDir {
			return "", fmt.Errorf("%s non è una cartella normale", d)
		}
	}
	if err := soloAmministratori(dest); err != nil {
		return "", err
	}
	// file che non c'entrano (vecchie versioni, copie a metà): via
	voci, err := os.ReadDir(dest)
	if err != nil {
		return "", err
	}
	for _, v := range voci {
		if _, ok := impronte[v.Name()]; !ok && !v.IsDir() {
			if err := os.Remove(filepath.Join(dest, v.Name())); err != nil {
				return "", err
			}
		}
	}
	trovatoExe := false
	for nome, h := range impronte {
		f := FileDaScaricare{Nome: nome, SHA256: h}
		if _, err := os.Stat(filepath.Join(dir, nome)); err != nil {
			// non c'è nella cartella dei modelli: non deve restare nemmeno nella copia
			if err := os.Remove(filepath.Join(dest, nome)); err != nil && !os.IsNotExist(err) {
				return "", err
			}
			continue
		}
		if !presente(dest, f) { // presente ricalcola sempre l'impronta di .exe e .dll
			if err := copiaFileIA(filepath.Join(dir, nome), filepath.Join(dest, nome)); err != nil {
				return "", err
			}
			if !presente(dest, f) { // cambiato durante la copia: non lo uso
				os.Remove(filepath.Join(dest, nome))
				return "", fmt.Errorf("la copia di %s non è uguale all'originale", nome)
			}
		}
		if nome == exe {
			trovatoExe = true
		}
	}
	if !trovatoExe {
		return "", fmt.Errorf("manca %s", exe)
	}
	return dest, nil
}

// soloAmministratori: la cartella (e quello che contiene) la possono cambiare solo SYSTEM e gli
// amministratori; gli utenti normali solo leggere. Senza ereditare i permessi da sopra.
func soloAmministratori(d string) error {
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;0x1200a9;;;BU)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(d, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

// copiaFileIA: copia in un file nuovo accanto e poi lo mette al posto giusto.
func copiaFileIA(da, a string) error {
	in, err := os.Open(da)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := a + ".copia"
	_ = os.Remove(tmp)
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, a)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// Ferma chiude il motore e aspetta (al massimo 5 secondi) che il processo sia uscito davvero:
// così il motore nuovo non parte mentre il vecchio tiene ancora la memoria video.
func (s *ServerIA) Ferma() {
	s.mu.Lock()
	cmd, uscito := s.cmd, s.uscito
	s.cmd, s.Client, s.uscito = nil, nil, nil
	s.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	if uscito != nil {
		select {
		case <-uscito:
		case <-time.After(5 * time.Second):
		}
	}
}

// tempoProcessore: tempo di processore usato finora da llama-server (unità da 100 ns, come
// GetSystemTimes). 0 se spento.
func (s *ServerIA) tempoProcessore() uint64 {
	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return 0
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(cmd.Process.Pid))
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(h)
	var c, e, k, u windows.Filetime
	if windows.GetProcessTimes(h, &c, &e, &k, &u) != nil {
		return 0
	}
	f := func(t windows.Filetime) uint64 { return uint64(t.HighDateTime)<<32 | uint64(t.LowDateTime) }
	return f(k) + f(u)
}

// Attivo: il motore è acceso e ha finito di caricare il modello.
func (s *ServerIA) Attivo() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cmd != nil && s.Client != nil
}

// jobChiudiConMe: se Tasso Traduce si chiude (anche per un errore), Windows chiude anche llama-server.
func jobChiudiConMe(pid int) windows.Handle {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, _ = windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	ph, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err == nil {
		_ = windows.AssignProcessToJobObject(h, ph)
		windows.CloseHandle(ph)
	}
	return h
}

// Motore IA come Motore per il Traduttore.
type motoreIA struct{ s *ServerIA }

func (m motoreIA) Nome() string { return "IA" }
func (m motoreIA) Traduci(ctx context.Context, r RichiestaTraduzione) (string, error) {
	m.s.mu.Lock()
	c := m.s.Client
	m.s.mu.Unlock()
	if c == nil {
		return "", errMotoreSpento
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return c.Traduci(ctx, r)
}
