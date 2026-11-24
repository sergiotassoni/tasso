//go:build windows

package traduce

// Voci dei personaggi → sottotitoli italiani dal vivo, tutto sul PC.
//   audio del PC (WASAPI loopback) → battute tagliate nelle pause → whisper.cpp (MIT, modello
//   OpenAI Whisper base inglese, MIT) con rilevatore di voce Silero (MIT) → Mozilla in italiano
//   (e il correttore IA, se c'è) → sottotitolo in basso sulla finestra in primo piano.
// Si accende con Ctrl+Alt+V o dalla finestra. I file si scaricano solo dopo il sì dell'utente.

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// Impronte controllate sul PC di Sergio il 22/11/2026 (release ufficiale whisper.cpp b5454 e
// modelli su Hugging Face, licenza MIT).
var risorseVoci = struct {
	Motore, Modello, VAD FileDaScaricare
}{
	Motore: FileDaScaricare{URL: "https://github.com/ggml-org/whisper.cpp/releases/download/b5454/whisper-bin-x64.zip",
		Nome: "whisper-bin-x64.zip", SHA256: "6ba69e3482d7826214f90a6a9c84ca07782aec1e1d0c6a7c30c994fd5d816ccb", Byte: 8928640},
	Modello: FileDaScaricare{URL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin",
		Nome: "ggml-base.en.bin", SHA256: "a03779c86df3323075f5e796cb2ce5029f00ec8869eee3fdfb897afe36c6d002", Byte: 147964211},
	VAD: FileDaScaricare{URL: "https://huggingface.co/ggml-org/whisper-vad/resolve/main/ggml-silero-v5.1.2.bin",
		Nome: "ggml-silero-v5.1.2.bin", SHA256: "29940d98d42b91fbd05ce489f3ecf7c72f0a42f027e4875919a28fb4c04ea2cf", Byte: 885098},
}

func dirVoci() string       { return filepath.Join(dirApp, "modelli", "voci") }
func dirMotoreVoci() string { return filepath.Join(dirVoci(), "whisper") }

func vociPresenti() bool {
	_, err := os.Stat(filepath.Join(dirMotoreVoci(), "whisper-server.exe"))
	if err != nil {
		return false
	}
	for _, f := range []FileDaScaricare{risorseVoci.Modello, risorseVoci.VAD} {
		if st, err := os.Stat(filepath.Join(dirVoci(), f.Nome)); err != nil || st.Size() != f.Byte {
			return false
		}
	}
	return true
}

var scaricandoVoci atomic.Bool

func scaricaVoci() {
	defer guard("scarica-voci")
	if !scaricandoVoci.CompareAndSwap(false, true) {
		return
	}
	defer scaricandoVoci.Store(false)
	files := []FileDaScaricare{risorseVoci.Motore, risorseVoci.VAD, risorseVoci.Modello}
	err := scaricaTutti(context.Background(), dirVoci(), files, func(f, t int64) {
		emetti("avanzamento", map[string]interface{}{"cosa": "voci", "fatti": f, "totale": t})
	})
	if err == nil {
		err = estraiWhisper(filepath.Join(dirVoci(), risorseVoci.Motore.Nome), dirMotoreVoci())
	}
	if err != nil {
		emetti("erroreScarica", map[string]string{"cosa": "voci", "errore": err.Error()})
		return
	}
	_ = os.Remove(filepath.Join(dirVoci(), risorseVoci.Motore.Nome))
	emetti("scaricato", map[string]string{"cosa": "voci"})
	emetti("aggiorna", stato())
}

// estraiWhisper: dallo zip (già verificato) solo il server e le sue librerie.
func estraiWhisper(zipPath, dest string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	trovato := false
	for _, f := range zr.File {
		nome := filepath.Base(strings.ReplaceAll(f.Name, "\\", "/"))
		b := strings.ToLower(nome)
		utile := b == "whisper-server.exe" || b == "whisper.dll" || (strings.HasPrefix(b, "ggml") && strings.HasSuffix(b, ".dll"))
		if f.FileInfo().IsDir() || !utile {
			continue
		}
		if b == "whisper-server.exe" {
			trovato = true
		}
		if err := copiaDaZip(f, filepath.Join(dest, nome)); err != nil {
			return err
		}
	}
	if !trovato {
		return errors.New("nello zip manca il riconoscimento della voce di Tasso")
	}
	return os.WriteFile(filepath.Join(dest, "LICENZE.txt"), []byte(licenzeVoci), 0o644)
}

const licenzeVoci = `Riconoscimento delle voci di Tasso Traduce: componenti di terze parti, tutti con licenza MIT.

whisper.cpp (programma whisper-server e librerie ggml) - https://github.com/ggml-org/whisper.cpp
  Copyright (c) 2023-2026 The ggml authors
Modello Whisper "base.en" - https://github.com/openai/whisper
  Copyright (c) 2022 OpenAI
Silero VAD (rilevatore di voce) - https://github.com/snakers4/silero-vad
  Copyright (c) 2020-present Silero Team

MIT License: Permission is hereby granted, free of charge, to any person obtaining a copy of this
software and associated documentation files (the "Software"), to deal in the Software without
restriction, including without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the
Software is furnished to do so, subject to the following conditions: The above copyright notice and
this permission notice shall be included in all copies or substantial portions of the Software.
THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT
LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN
NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
`

// ---------------------------------------------------------------- server whisper

type serverVoci struct {
	mu  sync.Mutex
	cmd *exec.Cmd
	url string
}

var sv = &serverVoci{}

func (s *serverVoci) avvia() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd != nil {
		return nil
	}
	if err := verificaMotore(dirMotoreVoci(), improntaWhisper, "whisper-server.exe"); err != nil {
		return err
	}
	porta, err := portaLibera()
	if err != nil {
		return err
	}
	// tutti i core per pochi decimi di secondo, a priorità più bassa del gioco: il gioco passa
	// sempre davanti, ma quando il processore è libero la voce si capisce in un terzo di secondo
	fili := max(2, min(8, coreFisici()))
	// percorso segreto per questa sessione: whisper-server gira da amministratore e senza chiave
	// qualunque programma o pagina web potrebbe chiamare /load o /inference
	chiave := chiaveCasuale()
	cmd := exec.Command(filepath.Join(dirMotoreVoci(), "whisper-server.exe"),
		"-m", filepath.Join(dirVoci(), risorseVoci.Modello.Nome),
		"--host", "127.0.0.1", "--port", fmt.Sprint(porta),
		"--request-path", "/"+chiave,
		"-t", fmt.Sprint(fili), "-l", "en", "-nt", "-sns",
		"-ac", "384", "-nf", // contesto di 7,7 s (le battute sono al massimo 7 s): 1,1 s -> 0,33 s a battuta
		"-bo", "1",
		"--vad", "-vm", filepath.Join(dirVoci(), risorseVoci.VAD.Nome),
	)
	cmd.Dir = dirMotoreVoci()
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000 | 0x00004000}
	logf, _ := os.Create(filepath.Join(dirApp, "logs", "voci.txt"))
	cmd.Stdout, cmd.Stderr = logf, logf
	if err := cmd.Start(); err != nil {
		return err
	}
	jobChiudiConMe(cmd.Process.Pid)
	s.cmd = cmd
	s.url = fmt.Sprintf("http://127.0.0.1:%d/%s", porta, chiave) // tutte le chiamate partono da qui
	uscito := make(chan struct{})
	go func() {
		defer guard("server-voci")
		_ = cmd.Wait()
		if logf != nil {
			logf.Close()
		}
		close(uscito)
		s.mu.Lock()
		if s.cmd == cmd {
			s.cmd = nil
		}
		s.mu.Unlock()
	}()
	for i := 0; i < 120; i++ {
		select {
		case <-uscito:
			return errors.New("il riconoscimento delle voci si è chiuso all'avvio (vedi logs\\voci.txt)")
		case <-time.After(250 * time.Millisecond):
		}
		if r, err := (&http.Client{Timeout: time.Second}).Get(s.url + "/"); err == nil {
			r.Body.Close()
			return nil
		}
	}
	_ = cmd.Process.Kill()
	s.cmd = nil
	return errors.New("il riconoscimento delle voci ci mette troppo ad avviarsi")
}

func (s *serverVoci) attivo() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cmd != nil
}

func (s *serverVoci) ferma() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	s.cmd = nil
}

// trascrivi: audio di una battuta → testo inglese.
func (s *serverVoci) trascrivi(ctx context.Context, audio []float32) (string, error) {
	s.mu.Lock()
	url := s.url
	attivo := s.cmd != nil
	s.mu.Unlock()
	if !attivo {
		return "", errors.New("riconoscimento spento")
	}
	var corpo bytes.Buffer
	mw := multipart.NewWriter(&corpo)
	fw, _ := mw.CreateFormFile("file", "battuta.wav")
	_, _ = fw.Write(wav16(audio))
	_ = mw.WriteField("response_format", "json")
	_ = mw.WriteField("temperature", "0.0")
	_ = mw.Close()
	req, err := http.NewRequestWithContext(ctx, "POST", url+"/inference", &corpo)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	dati, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("riconoscimento: risposta %d", resp.StatusCode)
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(dati, &out); err != nil {
		return "", err
	}
	return out.Text, nil
}

// ---------------------------------------------------------------- ascolto e sottotitoli

var (
	muVoci      sync.Mutex
	fermaVoci   chan struct{}
	vociAccese  atomic.Bool
	ultimaVoce  atomic.Value // RigaVoce
	battuteVoci atomic.Int64
)

type RigaVoce struct {
	Inglese  string `json:"inglese"`
	Italiano string `json:"italiano"`
	Ms       int64  `json:"ms"` // dalla fine della battuta al sottotitolo
}

// accendiVoci accende o spegne l'ascolto delle voci.
func accendiVoci(on bool) error {
	muVoci.Lock()
	defer muVoci.Unlock()
	return accendiVociLocked(on)
}

// commutaVoci: accende se spente, spegne se accese; la scelta si fa sotto lo stesso lucchetto,
// così due pressioni veloci del tasto non accendono due volte.
func commutaVoci() (bool, error) {
	muVoci.Lock()
	defer muVoci.Unlock()
	on := fermaVoci == nil
	return on, accendiVociLocked(on)
}

// spegniSessione: spegne solo se è ancora accesa la stessa sessione (non una riaccesa dopo).
func spegniSessione(ferma chan struct{}) {
	muVoci.Lock()
	defer muVoci.Unlock()
	if fermaVoci == ferma {
		_ = accendiVociLocked(false)
	}
}

func accendiVociLocked(on bool) error {
	if !on {
		if fermaVoci != nil {
			close(fermaVoci)
			fermaVoci = nil
		}
		sv.ferma()
		vociAccese.Store(false)
		ov.NascondiVoce()
		return nil
	}
	if fermaVoci != nil {
		return nil
	}
	if !vociPresenti() {
		return errors.New("prima scarica il riconoscimento delle voci (150 MB) in Giochi → Impostazioni")
	}
	if err := sv.avvia(); err != nil {
		return err
	}
	riavviiVoci.Store(0)
	ferma := make(chan struct{})
	fermaVoci = ferma
	vociAccese.Store(true)
	battute := make(chan battutaVoce, 3)
	anticipi := make(chan battutaVoce, 1) // battuta mandata appena inizia la pausa
	parziali := make(chan []float32, 1)   // pezzo della battuta in corso, per il sottotitolo provvisorio
	go func() {
		defer guard("ascolto-voci")
		tg := NuovoTagliatore()
		ultimoParziale := time.Time{}
		consegna := func(x []float32) {
			finite := tg.Aggiungi(x)
			info := tg.Info()
			// 0,15 s di pausa: comincio subito a capire la frase (la chiusura vera arriva a 0,45 s)
			if a, id, ok := tg.Anticipo(frequenzaVoce * 15 / 100); ok {
				select {
				case anticipi <- battutaVoce{a, id, false, time.Now().Add(-150 * time.Millisecond)}:
				default:
				}
			}
			// mentre il personaggio parla ancora: ogni 1,2 s mando quello che ha detto finora,
			// così il sottotitolo compare quasi subito anche nelle frasi lunghe
			if len(finite) == 0 && tg.InCorso() >= 1800*time.Millisecond && time.Since(ultimoParziale) >= 1200*time.Millisecond && len(battute) == 0 {
				select {
				case parziali <- tg.CopiaInCorso():
					ultimoParziale = time.Now()
				default:
				}
			}
			if len(finite) > 0 {
				ultimoParziale = time.Time{}
			}
			for i, a := range finite {
				b := battutaVoce{a, info[i].ID, info[i].Anticipata, time.Now()}
				select {
				case battute <- b:
				default: // sono indietro: salto la battuta più vecchia
					select {
					case <-battute:
					default:
					}
					battute <- b
				}
			}
		}
		// se cambia l'uscita audio (cuffie staccate, altre casse) l'ascolto si interrompe:
		// riapro l'uscita predefinita; dopo 5 errori di fila mi fermo e lo dico
		errori := 0
		for {
			err := ascoltaPC(ferma, consegna)
			select {
			case <-ferma:
				return
			default:
			}
			if err == nil {
				errori = 0
				continue
			}
			errori++
			if errori >= 5 {
				emetti("info", map[string]string{"testo": "Voci: " + err.Error() + ". Le spengo."})
				go spegniSessione(ferma)
				return
			}
			select {
			case <-ferma:
				return
			case <-time.After(1500 * time.Millisecond):
			}
		}
	}()
	go func() {
		defer guard("voci-sottotitoli")
		for {
			select {
			case <-ferma:
				return
			case a := <-anticipi:
				if gestisciBattuta(a.audio, ferma, a.inizioPausa) {
					anticipataMostrata.Store(a.id)
				}
			case b := <-battute:
				if b.anticipata && anticipataMostrata.Load() == b.id {
					continue // già mostrata con l'anticipo: è la stessa frase
				}
				gestisciBattuta(b.audio, ferma, b.inizioPausa.Add(-time.Duration(NuovoTagliatore().PausaFine)*time.Second/frequenzaVoce))
			case p := <-parziali:
				if len(battute) == 0 { // la battuta intera ha sempre la precedenza
					gestisciParziale(p, ferma)
				}
			}
		}
	}()
	return nil
}

var (
	riavviiVoci        atomic.Int64
	anticipataMostrata atomic.Int64
)

type battutaVoce struct {
	audio       []float32
	id          int64
	anticipata  bool
	inizioPausa time.Time // quando il personaggio ha smesso di parlare (circa)
}

// traduttore a parte per i pezzi provvisori: non finiscono in memoria, libreria o correttore
var (
	muProvvisorio sync.Mutex
	provvisorio   *Traduttore
)

func gestisciParziale(audio []float32, ferma chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	en, err := sv.trascrivi(ctx, audio)
	if err != nil {
		return
	}
	if en = testoVoce(en); en == "" {
		return
	}
	muProvvisorio.Lock()
	if provvisorio == nil || len(provvisorio.cache) > 300 {
		provvisorio = NuovoTraduttore("", leggiImp().Glossario, moz)
	}
	t := provvisorio
	muProvvisorio.Unlock()
	r := t.TraduciScritta(ctx, en)
	select {
	case <-ferma:
		return
	default:
	}
	if r.Tradotto != "" {
		mostraVoce(r.Tradotto+" …", en)
	}
}

func gestisciBattuta(audio []float32, ferma chan struct{}, fineVoce time.Time) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	select {
	case <-ferma: // già spente: non trascrivo e soprattutto non riavvio il riconoscimento
		return false
	default:
	}
	en, err := sv.trascrivi(ctx, audio)
	if err != nil {
		select {
		case <-ferma: // spente mentre trascrivevo: il riconoscimento chiuso è voluto
			return false
		default:
		}
		// il riconoscimento si è chiuso da solo: lo riavvio (al massimo 3 volte)
		if !sv.attivo() && riavviiVoci.Add(1) <= 3 {
			if sv.avvia() == nil {
				en, err = sv.trascrivi(ctx, audio)
			}
		}
		if err != nil {
			if riavviiVoci.Load() > 3 {
				emetti("info", map[string]string{"testo": "Voci: il riconoscimento continua a chiudersi, le spengo (vedi logs\\voci.txt)."})
				go spegniSessione(ferma)
			}
			return false
		}
	}
	select {
	case <-ferma: // spento mentre lavoravo: niente sottotitolo
		return false
	default:
	}
	en = testoVoce(en)
	if en == "" {
		return false
	}
	r := traduttore.Traduci(ctx, en)
	if r.Tradotto == "" {
		return false
	}
	select {
	case <-ferma:
		return false
	default:
	}
	// tempo dalla fine vera della voce (l'inizio della pausa)
	riga := RigaVoce{Inglese: en, Italiano: r.Tradotto, Ms: time.Since(fineVoce).Milliseconds()}
	ultimaVoce.Store(riga)
	battuteVoci.Add(1)
	mostraVoce(r.Tradotto, en)
	emetti("voce", riga)
	if suVoceProva != nil {
		suVoceProva(riga)
	}
	r.Millisec = riga.Ms
	aggiungiCronologia(r, false)
	return true
}

var giroVoce atomic.Int64

// mostraVoce: sottotitolo in basso, sulla finestra in primo piano, per il tempo di lettura.
var primoSottotitolo atomic.Int64 // per il collaudo: quando è comparso il primo sottotitolo

func mostraVoce(it, en string) {
	primoSottotitolo.CompareAndSwap(0, time.Now().UnixNano())
	fg, _, _ := pGetForeground.Call()
	area, ok := areaFinestra(fg)
	if !ok {
		return
	}
	z := Zona{X: area.X + area.W/10, Y: area.Y + area.H*78/100, W: area.W * 8 / 10, H: area.H / 12}
	ov.MostraVoce(z, it, en)
	g := giroVoce.Add(1)
	go func() {
		defer guard("voce-timer")
		time.Sleep(time.Duration(durataSottotitolo(it) * float64(time.Second)))
		if giroVoce.Load() == g {
			ov.NascondiVoce()
		}
	}()
}

func statoVoci() map[string]interface{} {
	u, _ := ultimaVoce.Load().(RigaVoce)
	return map[string]interface{}{"file": vociPresenti(), "accese": vociAccese.Load(), "ultima": u, "battute": battuteVoci.Load(), "scaricando": scaricandoVoci.Load()}
}

var _ = windows.Handle(0)
