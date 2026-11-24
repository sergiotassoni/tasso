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
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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
	defer os.Remove(f)
	ctx, cancel := context.WithTimeout(context.Background(), attesa)
	defer cancel()
	cmd := exec.CommandContext(ctx, sysExe("powershell.exe"), "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", f)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000 | 0x00004000} // priorità sotto la normale: il gioco viene prima
	out, err := cmd.Output()
	return strings.TrimSpace(strings.TrimPrefix(string(out), "\uFEFF")), err
}

func controllaGPU() InfoGPU {
	out, err := psUnaVolta(scriptGPU, 30*time.Second)
	var r struct {
		Nome  string  `json:"nome"`
		Mem   float64 `json:"mem"`
		Usata float64 `json:"usata"`
	}
	if err != nil || json.Unmarshal([]byte(out), &r) != nil || r.Nome == "" {
		return InfoGPU{Motivo: "Non ho trovato una scheda video dedicata."}
	}
	g := InfoGPU{Nome: r.Nome, MemoriaGB: r.Mem / gb, UsataGB: r.Usata / gb}
	return valutaGPU(g)
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
	job    windows.Handle
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
	if dirMotore == dirMotoreIA() {
		if err := verificaMotore(dirMotore, improntaLlama, "llama-server.exe"); err != nil {
			return err
		}
	}
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
	porta, err := portaLibera()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe,
		"-m", fileModello,
		"--host", "127.0.0.1", "--port", fmt.Sprint(porta),
		"-ngl", ngl,
		"-c", fmt.Sprint(nctx), "-np", "1",
		"--no-webui", "--no-slots",
	)
	// chiave segreta per questa sessione (variabile d'ambiente, non visibile nella riga di comando):
	// così né le pagine web né altri programmi del PC possono usare il motore
	chiave := chiaveCasuale()
	cmd.Env = append(os.Environ(), "LLAMA_API_KEY="+chiave)
	cmd.Dir = dirMotore
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000 | 0x00004000} // nessuna finestra, priorità sotto la normale
	logf, _ := os.Create(filepath.Join(dirApp, "logs", "motore-ia.txt"))
	cmd.Stdout, cmd.Stderr = logf, logf
	if err := cmd.Start(); err != nil {
		return err
	}
	s.job = jobChiudiConMe(cmd.Process.Pid)
	s.cmd = cmd
	client := &ClientIA{URL: fmt.Sprintf("http://127.0.0.1:%d", porta), Chiave: chiave}
	uscito := make(chan struct{})
	go func() {
		defer guard("server-ia")
		_ = cmd.Wait()
		if logf != nil {
			logf.Close()
		}
		close(uscito)
		s.mu.Lock()
		if s.cmd == cmd {
			s.cmd, s.Client = nil, nil
		}
		s.mu.Unlock()
	}()
	s.mu.Unlock() // non tengo il lucchetto mentre aspetto il caricamento
	defer s.mu.Lock()
	// aspetto che il modello sia caricato (con una scheda da gaming: pochi secondi)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	for {
		if client.Pronto(ctx) {
			s.mu.Lock()
			if s.cmd == cmd {
				s.Client = client
			}
			s.mu.Unlock()
			return nil
		}
		select {
		case <-uscito:
			return errors.New("il motore IA si è chiuso all'avvio (vedi logs\\motore-ia.txt)")
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return errors.New("il motore IA ci mette troppo ad avviarsi")
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (s *ServerIA) Ferma() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	s.cmd = nil
	s.Client = nil
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
		return "", errors.New("motore IA spento")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return c.Traduci(ctx, r)
}
