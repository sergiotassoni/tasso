//go:build windows

package traduce

import (
	"context"

	"golang.org/x/sys/windows"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	linguaRilevata atomic.Value // codice della lingua vista sullo schermo (se "auto")
	muOCRLingue    sync.Mutex
	ocrLingue      map[string]bool // tag OCR installati in Windows (es. "it-IT")
	scaricandoLin  atomic.Bool
)

// coppiaAttiva: lingua del gioco e lingua di arrivo da usare adesso.
func coppiaAttiva() (string, string) {
	i := leggiImp()
	da, a := i.LinguaDa, i.LinguaA
	if a == "" {
		a = "it"
	}
	if da == "" || da == "auto" {
		da = "en"
		if v, _ := linguaRilevata.Load().(string); v != "" {
			da = v
		}
	}
	return da, a
}

func coppiaPronta(da, a string) bool {
	return len(modelliServono(dirApp, da, a, presenteCoppia)) == 0
}

// presenteCoppia: anche inglese↔italiano, che stanno nelle cartelle di prima.
func presenteCoppia(da, a string) bool {
	switch {
	case da == "en" && a == "it":
		return mozillaPresente()
	case da == "it" && a == "en":
		return chatPresente()
	}
	return coppiaPresente(dirApp, da, a)
}

// scaricaLingue: scarica i modelli che mancano per da → a. avanz riceve la percentuale.
var muScaricaLingue sync.Mutex

func scaricaLingue(ctx context.Context, da, a string, avanz func(int)) error {
	muScaricaLingue.Lock() // due scaricamenti insieme scriverebbero lo stesso file
	defer muScaricaLingue.Unlock()
	serve := modelliServono(dirApp, da, a, presenteCoppia)
	tot := byteCoppie(serve)
	var fatti int64
	for _, c := range serve {
		files, dir := filesCoppia(c[0], c[1]), dirCoppia(dirApp, c[0]+c[1])
		switch c {
		case [2]string{"it", "en"}:
			files, dir = fileMozillaITEN, dirMozillaITEN()
		case [2]string{"en", "it"}:
			files, dir = fileMozillaENIT, dirMozilla()
		}
		base := fatti
		err := scaricaTutti(ctx, dir, files, func(f, t int64) {
			if tot > 0 {
				avanz(int(100 * (base + f) / tot))
			}
		})
		if err != nil {
			return err
		}
		for _, f := range files {
			fatti += f.Byte
		}
	}
	return nil
}

// ricaricaMozilla: la pagina rilegge l'elenco dei modelli (dopo uno scaricamento).
func ricaricaMozilla() {
	evalMotore("window.tt && window.tt.ricaricaModelli && window.tt.ricaricaModelli()")
}

// ---- lettura dello schermo nella lingua giusta

// caricaLingueOCR: quali lingue sa leggere Windows su questo PC (una volta, in sottofondo).
func caricaLingueOCR() map[string]bool {
	muOCRLingue.Lock()
	defer muOCRLingue.Unlock()
	if ocrLingue != nil {
		return ocrLingue
	}
	ocrLingue = map[string]bool{}
	ps := `$null = [Windows.Media.Ocr.OcrEngine, Windows.Foundation, ContentType = WindowsRuntime]; [Windows.Media.Ocr.OcrEngine]::AvailableRecognizerLanguages | ForEach-Object { $_.LanguageTag }`
	cmd := exec.Command(sysExe("powershell.exe"), "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.Output()
	if err == nil {
		for _, t := range strings.Fields(string(out)) {
			ocrLingue[strings.ToLower(t)] = true
		}
	}
	return ocrLingue
}

// ocrPerLingua: il tag OCR da usare per leggere una lingua, e se Windows lo ha.
// Le lingue con l'alfabeto latino si leggono bene anche con il lettore italiano o inglese.
func ocrPerLingua(codice string) (string, bool) {
	l, ok := linguaDa(codice)
	if !ok {
		return "", true
	}
	inst := caricaLingueOCR()
	if inst[strings.ToLower(l.OCR)] {
		return l.OCR, true
	}
	// stessa lingua, altra variante (es. pt-BR al posto di pt-PT)
	pre := strings.ToLower(strings.SplitN(l.OCR, "-", 2)[0]) + "-"
	for t := range inst {
		if strings.HasPrefix(t, pre) && (l.Codice != "zt" || strings.Contains(t, "hant")) && (l.Codice != "zh" || !strings.Contains(t, "hant")) {
			return t, true
		}
	}
	if l.Scrittura == "latina" {
		return "", true // il lettore che c'è (italiano o inglese) va bene
	}
	return "", false
}

// installaLetturaOCR: apre le Impostazioni di Windows sulla pagina delle lingue, dove l'utente
// aggiunge la lingua (con "Riconoscimento ottico dei caratteri"). Tasso non cambia niente da solo.
func installaLetturaOCR(codice string) string {
	if _, ok := linguaDa(codice); !ok {
		return "lingua sconosciuta"
	}
	verbo, _ := windows.UTF16PtrFromString("open")
	dove, _ := windows.UTF16PtrFromString("ms-settings:regionlanguage")
	if err := windows.ShellExecute(0, verbo, dove, nil, nil, 1); err != nil {
		return "Non riesco ad aprire le Impostazioni di Windows."
	}
	muOCRLingue.Lock()
	ocrLingue = nil // quando torni, rileggo cosa sa leggere Windows
	muOCRLingue.Unlock()
	return ""
}

// elencoLingue: per la finestra, ogni lingua con cosa c'è già sul PC.
func elencoLingue() []map[string]any {
	var out []map[string]any
	for _, l := range lingueGioco {
		_, ocrOK := ocrPerLingua(l.Codice)
		verso := l.Codice == "en" || presenteCoppia(l.Codice, "en")
		dall := l.Codice == "en" || presenteCoppia("en", l.Codice)
		mbDa, mbA := 0.0, 0.0
		if l.Codice != "en" && !verso {
			mbDa = float64(byteCoppie([][2]string{{l.Codice, "en"}})) / 1048576
		}
		if l.Codice != "en" && !dall {
			mbA = float64(byteCoppie([][2]string{{"en", l.Codice}})) / 1048576
		}
		mb := mbDa + mbA
		out = append(out, map[string]any{
			"codice": l.Codice, "nome": l.Nome, "nativo": l.Nativo,
			"da_pronta": verso, "a_pronta": dall, "mb": int(mb + 0.5), "mb_da": int(mbDa + 0.5), "mb_a": int(mbA + 0.5), "ocr_ok": ocrOK,
		})
	}
	return out
}

// segnaLinguaVista: la lingua delle scritte di una schermata. Se scelgo da solo la lingua del
// gioco, la uso; se mancano i modelli o la lettura, lo dico una volta.
func segnaLinguaVista(cartella, nome, lingua string) bool {
	if lingua == "" {
		return true
	}
	segnaLingua(cartella, nome, lingua)
	i := leggiImp()
	if i.LinguaDa != "" && i.LinguaDa != "auto" {
		return true // l'ha scelta il giocatore
	}
	prima, _ := linguaRilevata.Load().(string)
	if lingua == "han" { // cinese senza caratteri che distinguono: resta quello di prima
		lingua = "zh"
		if prima == "zt" {
			lingua = "zt"
		}
	}
	a := i.LinguaA
	if a == "" {
		a = "it"
	}
	if lingua == a {
		return false // il gioco è già nella lingua che vuoi (non lo ricordo come lingua del gioco)
	}
	if !coppiaPronta(lingua, a) {
		dillaUnaVolta("modelli"+lingua+a, "Lingua delle scritte: "+nomeLingua(lingua)+". Per tradurle in "+nomeLingua(a)+" scarica la lingua nella procedura a passi del gioco.")
		return false
	}
	linguaRilevata.Store(lingua) // solo una lingua che so davvero tradurre
	return true
}

var (
	muDette sync.Mutex
	dette   = map[string]time.Time{}
)

func dillaUnaVolta(chiave, testo string) {
	muDette.Lock()
	quando, gia := dette[chiave]
	if gia && time.Since(quando) < 10*time.Minute {
		muDette.Unlock()
		return
	}
	dette[chiave] = time.Now()
	muDette.Unlock()
	emetti("info", map[string]string{"testo": testo})
}
