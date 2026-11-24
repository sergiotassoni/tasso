//go:build windows

package traduce

// Lettura del testo con l'OCR integrato in Windows 10/11 (Windows.Media.Ocr): gratis,
// senza internet, senza programmi di altri. Un solo PowerShell resta aperto e legge
// un'immagine per volta, così non perdo tempo a riaprirlo a ogni dialogo.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const scriptOCR = `$ErrorActionPreference = 'Stop'
try { [Console]::OutputEncoding = [System.Text.Encoding]::UTF8 } catch { }
try { [Console]::InputEncoding = New-Object System.Text.UTF8Encoding($false) } catch { }
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$null = [Windows.Foundation.IAsyncOperation` + "`" + `1, Windows.Foundation, ContentType = WindowsRuntime]
$null = [Windows.Media.Ocr.OcrEngine, Windows.Foundation, ContentType = WindowsRuntime]
$null = [Windows.Media.Ocr.OcrResult, Windows.Foundation, ContentType = WindowsRuntime]
$null = [Windows.Globalization.Language, Windows.Foundation, ContentType = WindowsRuntime]
$null = [Windows.Graphics.Imaging.BitmapDecoder, Windows.Foundation, ContentType = WindowsRuntime]
$null = [Windows.Graphics.Imaging.SoftwareBitmap, Windows.Foundation, ContentType = WindowsRuntime]
$null = [Windows.Storage.StorageFile, Windows.Storage, ContentType = WindowsRuntime]
$null = [Windows.Storage.FileAccessMode, Windows.Storage, ContentType = WindowsRuntime]
$null = [Windows.Storage.Streams.IRandomAccessStream, Windows.Storage.Streams, ContentType = WindowsRuntime]
$asTask = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
  $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation` + "`" + `1' })[0]
function Await($op, [Type]$t) { $task = $asTask.MakeGenericMethod($t).Invoke($null, @($op)); $null = $task.Wait(-1); $task.Result }
$lingua = $null
$voluta = $env:TASSO_OCR_LINGUA
$prove = @('en-US','en-GB','en')
if ($voluta) { $prove = @($voluta) + $prove }
foreach ($l in $prove) {
  $x = [Windows.Globalization.Language]::new($l)
  if ([Windows.Media.Ocr.OcrEngine]::IsLanguageSupported($x)) { $lingua = $x; break }
}
if ($lingua) { $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage($lingua) }
else { $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages() }
if (-not $engine) { [Console]::Out.WriteLine('{"ok":false,"errore":"nessuna lingua OCR disponibile in Windows"}'); exit 1 }
$tag = if ($lingua) { $lingua.LanguageTag } else { $engine.RecognizerLanguage.LanguageTag }
[Console]::Out.WriteLine((ConvertTo-Json -Compress @{ ok = $true; pronto = $true; lingua = $tag }))
while ($null -ne ($p = [Console]::In.ReadLine())) {
  try {
    $f = Await ([Windows.Storage.StorageFile]::GetFileFromPathAsync($p)) ([Windows.Storage.StorageFile])
    $s = Await ($f.OpenAsync([Windows.Storage.FileAccessMode]::Read)) ([Windows.Storage.Streams.IRandomAccessStream])
    $d = Await ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($s)) ([Windows.Graphics.Imaging.BitmapDecoder])
    $b = Await ($d.GetSoftwareBitmapAsync()) ([Windows.Graphics.Imaging.SoftwareBitmap])
    $r = Await ($engine.RecognizeAsync($b)) ([Windows.Media.Ocr.OcrResult])
    $righe = @($r.Lines | ForEach-Object { $_.Text })
    $pos = @($r.Lines | ForEach-Object {
      @{ p = @($_.Words | ForEach-Object { $q = $_.BoundingRect; @{ t = $_.Text; x = [math]::Round($q.X,1); y = [math]::Round($q.Y,1); w = [math]::Round($q.Width,1); h = [math]::Round($q.Height,1) } }) }
    })
    $b.Dispose(); $s.Dispose(); $d = $null; $f = $null
    [Console]::Out.WriteLine((ConvertTo-Json -Compress -Depth 5 @{ ok = $true; righe = $righe; pos = $pos }))
  } catch {
    [Console]::Out.WriteLine((ConvertTo-Json -Compress @{ ok = $false; errore = $_.Exception.Message }))
  }
}
`

type LettoreOCR struct {
	lingua string // tag OCR voluto ("" = inglese o quello di Windows)
	mu     sync.Mutex
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    *bufio.Scanner
	Lingua string
	dir    string
	script string // file .ps1 di questo avvio (tolto alla chiusura)
	n      int
}

func pulisciCatture(dir, tieni string) {
	vecchie, _ := filepath.Glob(filepath.Join(dir, "cattura*.png"))
	for _, f := range vecchie {
		if f != tieni {
			_ = os.Remove(f) // se è ancora aperta ci riprovo la volta dopo
		}
	}
}

type rispostaOCR struct {
	OK     bool      `json:"ok"`
	Pronto bool      `json:"pronto"`
	Lingua string    `json:"lingua"`
	Righe  []string  `json:"righe"`
	Pos    []RigaOCR `json:"pos"`
	Errore string    `json:"errore"`
}

func sysExe(nome string) string {
	dir, err := windows.GetSystemDirectory()
	if err != nil || dir == "" {
		dir = filepath.Join(os.Getenv("SystemRoot"), "System32")
	}
	return filepath.Join(dir, "WindowsPowerShell", "v1.0", nome)
}

func avviaOCR(dir string) (*LettoreOCR, error) {
	l := &LettoreOCR{dir: dir}
	if da, _ := coppiaAttiva(); da != "" {
		l.lingua, _ = ocrPerLingua(da)
	}
	return l, l.avvia()
}

func (l *LettoreOCR) avvia() error {
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return err
	}
	// lo script va nella cartella protetta (Tasso gira come amministratore): un programma normale
	// non può cambiarlo prima che parta. Nome nuovo a ogni avvio, file creato da zero.
	_ = os.MkdirAll(cartellaScript(), 0o755)
	script := filepath.Join(cartellaScript(), fmt.Sprintf("tt_ocr_%d_%d.ps1", os.Getpid(), time.Now().UnixNano()))
	// UTF-8 con BOM: PowerShell 5.1 legge bene le lettere accentate
	if err := scriviNuovo(script, []byte("\uFEFF"+strings.ReplaceAll(scriptOCR, "\n", "\r\n"))); err != nil {
		return err
	}
	l.script = script
	cmd := exec.Command(sysExe("powershell.exe"), "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	cmd.Env = append(os.Environ(), "TASSO_OCR_LINGUA="+l.lingua)
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	logf, err := os.Create(filepath.Join(l.dir, "ocr-errori.txt"))
	if err == nil {
		cmd.Stderr = logf // i messaggi di errore di PowerShell, per capire cosa non va
	}
	err = cmd.Start()
	if logf != nil {
		// PowerShell ha la sua copia del file: la mia si chiude subito (prima restava aperta
		// a ogni riavvio della lettura)
		logf.Close()
	}
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	l.cmd, l.in, l.out = cmd, in, sc
	r, err := l.leggiRisposta(40 * time.Second)
	if err != nil {
		l.chiudi()
		return err
	}
	if !r.OK {
		l.chiudi()
		return errors.New(r.Errore)
	}
	l.Lingua = r.Lingua
	return nil
}

func (l *LettoreOCR) leggiRisposta(attesa time.Duration) (rispostaOCR, error) {
	ch := make(chan rispostaOCR, 1)
	errc := make(chan error, 1)
	go func() {
		defer guard("ocr-lettura")
		sc := l.out // se il lettore viene riavviato, questa lettura resta sul vecchio
		if sc == nil {
			errc <- errors.New("lettore chiuso")
			return
		}
		for sc.Scan() {
			riga := strings.TrimPrefix(strings.TrimSpace(sc.Text()), "\uFEFF")
			if !strings.HasPrefix(riga, "{") {
				continue
			}
			var r rispostaOCR
			if json.Unmarshal([]byte(riga), &r) == nil {
				ch <- r
				return
			}
		}
		dettaglio := ""
		if b, err := os.ReadFile(filepath.Join(l.dir, "ocr-errori.txt")); err == nil {
			dettaglio = strings.TrimSpace(string(b))
			if len(dettaglio) > 300 {
				dettaglio = dettaglio[:300]
			}
		}
		if dettaglio != "" {
			errc <- errors.New("la lettura del testo di Windows si è chiusa: " + dettaglio)
			return
		}
		errc <- errors.New("la lettura del testo di Windows si è chiusa")
	}()
	select {
	case r := <-ch:
		return r, nil
	case err := <-errc:
		return rispostaOCR{}, err
	case <-time.After(attesa):
		return rispostaOCR{}, errors.New("la lettura del testo non risponde")
	}
}

// Leggi salva l'immagine e restituisce il testo trovato (righe separate da a capo).
func (l *LettoreOCR) Leggi(png []byte) (string, error) {
	r, err := l.leggiTutto(png)
	if err != nil {
		return "", err
	}
	return strings.Join(r.Righe, "\n"), nil
}

// LeggiPosizioni: ogni riga con le sue parole e i rettangoli (pixel dell'immagine passata).
func (l *LettoreOCR) LeggiPosizioni(png []byte) ([]RigaOCR, error) {
	r, err := l.leggiTutto(png)
	if err != nil {
		return nil, err
	}
	return r.Pos, nil
}

func (l *LettoreOCR) leggiTutto(png []byte) (rispostaOCR, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cmd == nil {
		if err := l.avvia(); err != nil {
			return rispostaOCR{}, err
		}
	}
	// Windows tiene "mappato" il file letto per un po': ogni cattura ha un nome nuovo e le
	// vecchie si cancellano quando Windows le lascia andare.
	l.n++
	p := filepath.Join(l.dir, fmt.Sprintf("cattura-%d.png", l.n))
	if err := os.WriteFile(p, png, 0o644); err != nil {
		return rispostaOCR{}, err
	}
	defer pulisciCatture(l.dir, p)
	if _, err := fmt.Fprintln(l.in, p); err != nil {
		l.chiudi()
		return rispostaOCR{}, err
	}
	r, err := l.leggiRisposta(15 * time.Second)
	if err != nil {
		l.chiudi() // al prossimo giro lo riapro
		return rispostaOCR{}, err
	}
	if !r.OK {
		return rispostaOCR{}, errors.New(r.Errore)
	}
	return r, nil
}

func (l *LettoreOCR) chiudi() {
	if l.in != nil {
		l.in.Close()
	}
	if l.cmd != nil && l.cmd.Process != nil {
		_ = l.cmd.Process.Kill()
		_, _ = l.cmd.Process.Wait()
	}
	l.cmd, l.in, l.out = nil, nil, nil
	if l.script != "" {
		_ = os.Remove(l.script)
		l.script = ""
	}
}

func (l *LettoreOCR) Chiudi() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.chiudi()
}
