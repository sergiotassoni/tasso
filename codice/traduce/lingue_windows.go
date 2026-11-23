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
	ocrLetteAlle   time.Time       // quando ho letto ocrLingue
	ocrRileggiFino time.Time       // dopo "Apri le Impostazioni di Windows": rileggo per qualche minuto
	ocrInLettura   atomic.Bool     // lettura in sottofondo in corso
	scaricandoLin  atomic.Bool
)

// coppiaAttiva: lingua del gioco e lingua di arrivo da usare adesso.
func coppiaAttiva() (string, string) {
	i := leggiImp()
	da, a := i.LinguaDa, linguaArrivo(i)
	if da == "" || da == "auto" {
		da = "en"
		if v, _ := linguaRilevata.Load().(string); v != "" {
			da = v
		}
	}
	return da, a
}

// linguaArrivo: la lingua in cui tradurre (quella scelta, o quella di Tasso se non l'hai mai scelta).
func linguaArrivo(i Impostazioni) string {
	if i.LinguaA != "" {
		return i.LinguaA
	}
	return arrivoDiBase()
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

// leggiLingueOCR: chiede a Windows quali lingue sa leggere (PowerShell, al massimo 20 secondi).
func leggiLingueOCR() map[string]bool {
	ps := `$null = [Windows.Media.Ocr.OcrEngine, Windows.Foundation, ContentType = WindowsRuntime]; [Windows.Media.Ocr.OcrEngine]::AvailableRecognizerLanguages | ForEach-Object { $_.LanguageTag }`
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, sysExe("powershell.exe"), "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.Output()
	m := map[string]bool{}
	if err == nil {
		for _, t := range strings.Fields(string(out)) {
			m[strings.ToLower(t)] = true
		}
	}
	return m
}

// ocrDaRileggere: col lucchetto preso. Niente lista, o lista vecchia mentre stai installando
// una lingua nelle Impostazioni di Windows.
func ocrDaRileggere() bool {
	return ocrLingue == nil || (time.Now().Before(ocrRileggiFino) && time.Since(ocrLetteAlle) > 15*time.Second)
}

// caricaLingueOCR: quali lingue sa leggere Windows su questo PC (aspetta la lettura, se serve).
func caricaLingueOCR() map[string]bool {
	muOCRLingue.Lock()
	defer muOCRLingue.Unlock()
	if ocrDaRileggere() {
		ocrLingue, ocrLetteAlle = leggiLingueOCR(), time.Now()
	}
	return ocrLingue
}

// lingueOCRSubito: per la finestra, senza aspettare PowerShell. Restituisce quello che so
// (nil = non ancora letto) e, se serve, rilegge in sottofondo e poi aggiorna la finestra.
func lingueOCRSubito() map[string]bool {
	muOCRLingue.Lock()
	m, rileggi := ocrLingue, ocrDaRileggere()
	muOCRLingue.Unlock()
	if rileggi {
		caricaLingueOCRInSottofondo()
	}
	return m
}

// caricaLingueOCRInSottofondo: legge le lingue di Windows senza fermare nessuno e manda alla
// finestra l'elenco aggiornato.
func caricaLingueOCRInSottofondo() {
	if !ocrInLettura.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer guard("lingue-ocr")
		defer ocrInLettura.Store(false)
		caricaLingueOCR()
		emetti("lingue", elencoLingue())
	}()
}

// ocrPerLingua: il tag OCR da usare per leggere una lingua, e se Windows lo ha.
// Le lingue con l'alfabeto latino si leggono bene anche con il lettore italiano o inglese.
func ocrPerLingua(codice string) (string, bool) {
	return ocrPerLinguaIn(codice, caricaLingueOCR())
}

// ocrPerLinguaIn: come ocrPerLingua con l'elenco dato (nil = non so ancora: dico di sì).
func ocrPerLinguaIn(codice string, inst map[string]bool) (string, bool) {
	l, ok := linguaDa(codice)
	if !ok || inst == nil {
		return "", true
	}
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

// installaOCRIn: codice della lingua che Windows sta aggiungendo adesso ("" = nessuna).
var installaOCRIn atomic.Value

// scriptAggiungiOCR: aggiunge a Windows il lettore di una lingua (pacchetto "Riconoscimento ottico dei
// caratteri", piccolo, da Windows Update). Tasso parte già come amministratore: nessuna altra
// richiesta all'utente. Cerca il nome esatto del pacchetto invece di indovinarne la versione.
const scriptAggiungiOCR = `$ErrorActionPreference = 'Stop'
try {
  $tutti = Get-WindowsCapability -Online | Where-Object { $_.Name -like 'Language.OCR~~~*' }
  $c = $tutti | Where-Object { $_.Name -like 'Language.OCR~~~%TAG%~*' } | Select-Object -First 1
  if (-not $c) { $c = $tutti | Where-Object { $_.Name -like 'Language.OCR~~~%PRE%-*' } | Select-Object -First 1 }
  if (-not $c) { [Console]::Out.WriteLine('NONE'); exit }
  if ($c.State -ne 'Installed') { $null = Add-WindowsCapability -Online -Name $c.Name }
  [Console]::Out.WriteLine('OK')
} catch { [Console]::Out.WriteLine('ERR') }
`

// pacchettoOCR: il tag della lingua come lo chiama il pacchetto di Windows.
func pacchettoOCR(tag string) string {
	switch tag {
	case "zh-Hans-CN":
		return "zh-CN"
	case "zh-Hant-TW":
		return "zh-TW"
	}
	return tag
}

// installaLetturaOCR: aggiunge da sola a Windows la lettura della lingua (in sottofondo, può
// durare qualche minuto). Se Windows non riesce, apre le Impostazioni sulla pagina delle lingue.
func installaLetturaOCR(codice string) string {
	l, ok := linguaDa(codice)
	if !ok || l.OCR == "" {
		return "lingua sconosciuta"
	}
	if v, _ := installaOCRIn.Load().(string); v != "" {
		return "Sto già aggiungendo un'altra lingua: aspetta che finisca."
	}
	installaOCRIn.Store(codice)
	emetti("lingue", elencoLingue())
	go func() {
		defer guard("installa-ocr")
		tag := pacchettoOCR(l.OCR)
		pre := strings.SplitN(tag, "-", 2)[0]
		out, err := psUnaVolta(strings.NewReplacer("%TAG%", tag, "%PRE%", pre).Replace(scriptAggiungiOCR), 20*time.Minute)
		installaOCRIn.Store("")
		muOCRLingue.Lock()
		ocrLingue = nil // rileggo subito cosa sa leggere Windows
		muOCRLingue.Unlock()
		letta := false
		for t := range caricaLingueOCR() {
			letta = letta || strings.HasPrefix(t, strings.ToLower(pre)+"-")
		}
		if err == nil && strings.Contains(out, "OK") && letta {
			emetti("info", map[string]string{"testo": "Fatto: ora Windows sa leggere questa lingua."})
		} else {
			apriImpostazioniLingue()
			emetti("info", map[string]string{"testo": "Windows non è riuscito ad aggiungerla da solo (serve internet). Ti ho aperto le Impostazioni: aggiungi la lingua da lì."})
		}
		emetti("lingue", elencoLingue())
	}()
	return ""
}

func apriImpostazioniLingue() {
	verbo, _ := windows.UTF16PtrFromString("open")
	dove, _ := windows.UTF16PtrFromString("ms-settings:regionlanguage")
	if windows.ShellExecute(0, verbo, dove, nil, nil, 1) == nil {
		muOCRLingue.Lock()
		ocrRileggiFino = time.Now().Add(5 * time.Minute)
		muOCRLingue.Unlock()
	}
}

// elencoLingue: per la finestra, ogni lingua con cosa c'è già sul PC.
func elencoLingue() []map[string]any {
	var out []map[string]any
	inst := lingueOCRSubito()
	inCorso, _ := installaOCRIn.Load().(string)
	for _, l := range lingueGioco {
		_, ocrOK := ocrPerLinguaIn(l.Codice, inst)
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
			"da_pronta": verso, "a_pronta": dall, "mb": int(mb + 0.5), "mb_da": int(mbDa + 0.5), "mb_a": int(mbA + 0.5), "ocr_ok": ocrOK, "ocr_in": l.Codice == inCorso,
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
		// l'ha scelta il giocatore: serve che i modelli ci siano
		if a := linguaArrivo(i); i.LinguaDa != a && !coppiaPronta(i.LinguaDa, a) {
			dillaUnaVolta("modelli"+i.LinguaDa+a, "Lingue scelte: "+nomeLingua(i.LinguaDa)+" → "+nomeLingua(a)+". Mancano i modelli: scaricali in Giochi → Impostazioni, accanto alle lingue.")
			return false
		}
		return true
	}
	prima, _ := linguaRilevata.Load().(string)
	if lingua == "han" { // cinese senza caratteri che distinguono: resta quello di prima
		lingua = "zh"
		if prima == "zt" {
			lingua = "zt"
		}
	}
	a := linguaArrivo(i)
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
