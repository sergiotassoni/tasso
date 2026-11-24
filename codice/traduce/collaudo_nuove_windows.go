//go:build windows && collaudo

package traduce

// Collaudo delle funzioni nuove: TassoTraduce.exe --collaudo "C:\cartella" --solo-nuove --voci-prova "C:\audio"
//  - chat al contrario: scarica (con impronte) il modello it→en, traduce frasi da chat, prova gli
//    appunti e il tasto Ctrl+Alt+T come lo userebbe un giocatore;
//  - voci: scarica (con impronte) whisper e i modelli, accende l'ascolto dell'audio del PC,
//    fa suonare dalle casse i file voce*.wav della cartella e controlla i sottotitoli che escono.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var windowsLazyWinmm = windows.NewLazySystemDLL("winmm.dll")

var (
	soloNuove          bool
	cartellaAudioProva string
	muRigheVoci        sync.Mutex
	righeVociProva     []RigaVoce
)

func leggiArgomentiNuove(args []string) {
	for i, a := range args {
		if a == "--solo-nuove" {
			soloNuove = true
		}
		if a == "--voci-prova" && i+1 < len(args) {
			cartellaAudioProva = args[i+1]
		}
	}
}

type EsitoChat struct {
	Italiano string  `json:"italiano"`
	Inglese  string  `json:"inglese"`
	Ms       float64 `json:"ms"`
}

type EsitoVoce struct {
	File      string `json:"file"`
	Detto     string `json:"detto"`
	Capito    string `json:"capito"`
	Italiano  string `json:"italiano"`
	Ms        int64  `json:"ms"`
	PrimoMs   int64  `json:"primo_ms"` // dall'inizio della voce al primo sottotitolo (anche provvisorio)
	DurataMs  int64  `json:"durata_ms"`
	ParoleOK  int    `json:"parole_ok"`
	ParoleTot int    `json:"parole_tot"`
}

type CollaudoNuove struct {
	Data          string      `json:"data"`
	ChatScaricata string      `json:"chat_scaricata"`
	Chat          []EsitoChat `json:"chat"`
	Appunti       string      `json:"appunti"`
	TastoChat     string      `json:"tasto_chat"`
	VociScaricate string      `json:"voci_scaricate"`
	LivelloMax    float64     `json:"livello_audio_max"`
	Voci          []EsitoVoce `json:"voci"`
	Altre         []RigaVoce  `json:"altre_righe"`
	Errori        []string    `json:"errori"`
}

func collaudoNuove(cartella string) {
	var c CollaudoNuove
	c.Data = time.Now().Format("02/01/2006 15:04")
	scrivi := func() {
		_ = os.WriteFile(filepath.Join(cartella, "collaudo-nuove.md"), []byte(rapportoNuove(c)), 0o644)
	}
	defer scrivi()

	// ---- chat
	t0 := time.Now()
	if !chatPresente() {
		scaricaChat()
	}
	c.ChatScaricata = fmt.Sprintf("presente=%v (%.1f s)", chatPresente(), time.Since(t0).Seconds())
	frasi := []string{
		"Ciao a tutti, qualcuno mi aiuta con questa missione?",
		"Aspettatemi all'ingresso del castello, arrivo tra due minuti.",
		"Ho bisogno di munizioni, qualcuno ne ha?",
		"Bella partita ragazzi, ci vediamo domani sera.",
		"Attenzione, il boss sta caricando l'attacco speciale!",
		"Scusate, ho perso la connessione.",
		"Chi vuole fare squadra per la prossima incursione?",
		"Non ho capito, puoi ripetere?",
	}
	for _, it := range frasi {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		s := time.Now()
		en, err := traduciChat(ctx, it)
		cancel()
		if err != nil {
			en = "[errore: " + err.Error() + "]"
		}
		c.Chat = append(c.Chat, EsitoChat{it, en, float64(time.Since(s).Microseconds()) / 1000})
	}
	if vecchi, err := leggiAppunti(); err == nil {
		defer scriviAppunti(vecchi) // rimetto gli appunti com'erano
	}
	prova := "Prova degli appunti: àèìòù €"
	if err := scriviAppunti(prova); err != nil {
		c.Appunti = "scrittura non riuscita: " + err.Error()
	} else if letto, err := leggiAppunti(); err != nil || letto != prova {
		c.Appunti = fmt.Sprintf("letto %q (%v)", letto, err)
	} else {
		c.Appunti = "ok (scritto e riletto uguale, accenti compresi)"
	}
	_ = scriviAppunti("Andiamo a destra, il tesoro è dietro la cascata.")
	suTastoChat()
	if en, err := leggiAppunti(); err == nil {
		c.TastoChat = "appunti dopo il tasto: " + en
	} else {
		c.TastoChat = "errore: " + err.Error()
	}
	scrivi()

	// ---- voci
	t0 = time.Now()
	if !vociPresenti() {
		scaricaVoci()
	}
	c.VociScaricate = fmt.Sprintf("presenti=%v (%.1f s)", vociPresenti(), time.Since(t0).Seconds())
	if cartellaAudioProva == "" || !vociPresenti() {
		c.Errori = append(c.Errori, "voci non provate (manca la cartella audio o il modello)")
		return
	}
	suVoceProva = func(r RigaVoce) {
		muRigheVoci.Lock()
		righeVociProva = append(righeVociProva, r)
		tempiVociProva = append(tempiVociProva, time.Now())
		muRigheVoci.Unlock()
	}
	if err := accendiVoci(true); err != nil {
		c.Errori = append(c.Errori, "accendi voci: "+err.Error())
		return
	}
	defer accendiVoci(false)
	time.Sleep(2 * time.Second)
	file, _ := filepath.Glob(filepath.Join(cartellaAudioProva, "voce*.wav"))
	sort.Strings(file)
	detti := map[string]string{}
	if b, err := os.ReadFile(filepath.Join(cartellaAudioProva, "voci.txt")); err == nil {
		righe := strings.Split(strings.ReplaceAll(strings.TrimPrefix(string(b), "\uFEFF"), "\r", ""), "\n")
		for i, f := range file {
			if i < len(righe) {
				detti[f] = righe[i]
			}
		}
	}
	var picco float64
	fermaPicco := make(chan struct{})
	go func() {
		for {
			select {
			case <-fermaPicco:
				return
			case <-time.After(50 * time.Millisecond):
				if v := float64(livelloAudio.Load()) / 1000; v > picco {
					picco = v
				}
			}
		}
	}()
	for _, f := range file {
		muRigheVoci.Lock()
		prima := len(righeVociProva)
		muRigheVoci.Unlock()
		primoSottotitolo.Store(0)
		inizioVoce := time.Now()
		suona(f)
		durata := time.Since(inizioVoce)
		// fine VERA della voce: fine del file meno la musica che c'è dopo la voce (coda.txt)
		var r RigaVoce
		for i := 0; i < 80; i++ { // fino a 8 s per il sottotitolo
			time.Sleep(100 * time.Millisecond)
			muRigheVoci.Lock()
			if len(righeVociProva) > prima {
				r = righeVociProva[prima]
				for _, x := range righeVociProva[prima+1:] {
					r.Inglese += " " + x.Inglese
					r.Italiano += " " + x.Italiano
				}
				// tempo: dalla fine della voce come la sente Tasso (inizio della pausa) all'ultimo
				// sottotitolo. La fine del file non serve: Windows finisce di suonarlo in ritardo.
				r.Ms = righeVociProva[len(righeVociProva)-1].Ms
			}
			muRigheVoci.Unlock()
			if r.Inglese != "" && i > 15 {
				break
			}
		}
		ok, tot := paroleUguali(detti[f], r.Inglese)
		primo := int64(-1)
		if p := primoSottotitolo.Load(); p > 0 {
			primo = (p - inizioVoce.UnixNano()) / 1e6
		}
		c.Voci = append(c.Voci, EsitoVoce{filepath.Base(f), detti[f], r.Inglese, r.Italiano, r.Ms, primo, durata.Milliseconds(), ok, tot})
		scrivi()
		time.Sleep(1500 * time.Millisecond)
	}
	close(fermaPicco)
	c.LivelloMax = picco
}

var (
	suVoceProva    func(RigaVoce)
	tempiVociProva []time.Time
)

var pPlaySound = windowsLazyWinmm.NewProc("PlaySoundW")

// suona fa uscire un file WAV dalle casse (come farebbe il gioco) e aspetta che finisca.
// (PlaySoundW di Windows: niente PowerShell, niente comandi costruiti con il nome del file)
func suona(file string) {
	p, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return
	}
	pPlaySound.Call(uintptr(unsafe.Pointer(p)), 0, 0x00020000 /*SND_FILENAME*/ |0x0000 /*SND_SYNC*/ |0x0002 /*SND_NODEFAULT*/)
}

// paroleUguali: quante parole della frase detta si ritrovano, nell'ordine, in quella capita.
func paroleUguali(detto, capito string) (int, int) {
	norm := func(s string) []string {
		var out []string
		for _, w := range strings.Fields(strings.ToLower(s)) {
			w = strings.Trim(w, ".,!?;:\"'")
			if w != "" {
				out = append(out, w)
			}
		}
		return out
	}
	a, b := norm(detto), norm(capito)
	// sottosequenza comune più lunga
	m := make([][]int, len(a)+1)
	for i := range m {
		m[i] = make([]int, len(b)+1)
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				m[i][j] = m[i-1][j-1] + 1
			} else {
				m[i][j] = max(m[i-1][j], m[i][j-1])
			}
		}
	}
	return m[len(a)][len(b)], len(a)
}

func rapportoNuove(c CollaudoNuove) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Collaudo funzioni nuove (%s)\n\n## Chat al contrario (italiano → inglese)\n\nModello: %s\n\n| Italiano | Inglese | ms |\n|---|---|---|\n", c.Data, c.ChatScaricata)
	for _, x := range c.Chat {
		fmt.Fprintf(&b, "| %s | %s | %.0f |\n", x.Italiano, x.Inglese, x.Ms)
	}
	fmt.Fprintf(&b, "\n- Appunti: %s\n- Tasto Ctrl+Alt+T: %s\n\n## Voci dei personaggi → sottotitoli\n\nModelli: %s · livello audio massimo sentito: %.3f\n\n", c.Appunti, c.TastoChat, c.VociScaricate, c.LivelloMax)
	if len(c.Voci) > 0 {
		b.WriteString("| File | Detto | Capito | Sottotitolo italiano | Parole giuste | voce dura s | primo sottotitolo s dall'inizio | sottotitolo finale s dopo la fine della voce |\n|---|---|---|---|---|---|---|---|\n")
		for _, v := range c.Voci {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %d/%d | %.1f | %.1f | %.1f |\n", v.File, v.Detto, v.Capito, v.Italiano, v.ParoleOK, v.ParoleTot, float64(v.DurataMs)/1000, float64(v.PrimoMs)/1000, float64(v.Ms)/1000)
		}
	}
	if len(c.Errori) > 0 {
		b.WriteString("\n## Errori\n\n")
		for _, e := range c.Errori {
			b.WriteString("- " + e + "\n")
		}
	}
	return b.String()
}
