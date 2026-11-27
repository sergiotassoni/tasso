package main

// Lingue di Tasso: italiano (quella "vera", scritta nel codice), inglese, spagnolo, francese e tedesco.
// Le traduzioni stanno in i18n/<lingua>.json (en, es, fr, de, tutti con le stesse chiavi):
// "ui" per i testi della finestra, "go" per i messaggi di Go e "ps" per i testi degli script
// PowerShell. In italiano non si traduce niente. Se in es/fr/de manca un testo, si usa l'inglese.

import (
	"embed"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tasso/traduce"
)

//go:embed i18n/en.json i18n/es.json i18n/fr.json i18n/de.json
var i18nFS embed.FS

type dizionario struct {
	UI    map[string]string `json:"ui"`
	Go    map[string]string `json:"go"`
	PS    map[string]string `json:"ps"`
	frags []string          // pezzi dei messaggi di Go, dal più lungo al più corto
}

var (
	i18nMu  sync.Mutex
	i18nDiz = map[string]*dizionario{}
	pUILang = kernel32.NewProc("GetUserDefaultUILanguage")
)

// leggiDiz: il file i18n/<l>.json così com'è (nil se non c'è).
func leggiDiz(l string) *dizionario {
	b, err := i18nFS.ReadFile("i18n/" + l + ".json")
	if err != nil {
		return nil
	}
	d := &dizionario{}
	if json.Unmarshal(b, d) != nil {
		return nil
	}
	return d
}

// dizLingua: il dizionario della lingua (inglese sotto, per i testi che mancano). nil in italiano.
func dizLingua(l string) *dizionario {
	if l == "it" || !linguaDisponibile(l) {
		return nil
	}
	i18nMu.Lock()
	defer i18nMu.Unlock()
	if d, ok := i18nDiz[l]; ok {
		return d
	}
	d := leggiDiz("en")
	if d == nil {
		d = &dizionario{}
	}
	if l != "en" {
		if x := leggiDiz(l); x != nil {
			unisci := func(dst *map[string]string, src map[string]string) {
				if *dst == nil {
					*dst = map[string]string{}
				}
				for k, v := range src {
					if v != "" {
						(*dst)[k] = v
					}
				}
			}
			unisci(&d.UI, x.UI)
			unisci(&d.Go, x.Go)
			unisci(&d.PS, x.PS)
		}
	}
	for k := range d.Go {
		if len(k) >= 6 {
			d.frags = append(d.frags, k)
		}
	}
	sort.Slice(d.frags, func(i, j int) bool { return len(d.frags[i]) > len(d.frags[j]) })
	i18nDiz[l] = d
	return d
}

// curDiz: il dizionario della lingua di adesso (nil in italiano).
func curDiz() *dizionario { return dizLingua(curLang()) }

// linguaDaLangID: la lingua di Tasso per una lingua di Windows (LANGID); le altre diventano inglese.
func linguaDaLangID(id uint16) string {
	switch id & 0x3ff {
	case 0x10:
		return "it"
	case 0x0a:
		return "es"
	case 0x0c:
		return "fr"
	case 0x07:
		return "de"
	}
	return "en"
}

// systemLang: la lingua di Windows, se Tasso ce l'ha (it, en, es, fr, de), altrimenti inglese.
func systemLang() string {
	r, _, _ := pUILang.Call()
	return linguaDaLangID(uint16(r))
}

// curLang: la lingua scelta nelle Impostazioni di Tasso; se non l'hai ancora scelta quella
// dell'installatore (HKCU\Software\Tasso\Lingua, ecosistema.go), altrimenti quella di Windows.
func curLang() string {
	setMu.Lock()
	l := settings.Lang
	setMu.Unlock()
	if linguaDisponibile(l) {
		return l
	}
	if l := linguaInstallatore(); l != "" {
		return l
	}
	return systemLang()
}

// isEN: Tasso è proprio in inglese (per numeri col punto, generi di Steam in inglese...).
func isEN() bool { return curLang() == "en" }

// traduciTasso: Tasso non è in italiano, quindi i testi passano dal dizionario.
func traduciTasso() bool { return curLang() != "it" }

// trGo traduce un messaggio di Go: prima tutto intero, poi pezzo per pezzo
// (per i messaggi con dentro numeri o nomi).
func trGo(s string) string {
	if s == "" {
		return s
	}
	d := curDiz()
	if d == nil {
		return s
	}
	if v, ok := d.Go[s]; ok {
		return v
	}
	if v, ok := d.PS[s]; ok {
		return v
	}
	out := s
	for _, k := range d.frags {
		if strings.Contains(out, k) {
			out = strings.ReplaceAll(out, k, d.Go[k])
		}
	}
	return out
}

// trPS traduce i testi tra apici singoli di uno script PowerShell ('...'), solo quelli
// che sono nel dizionario: percorsi, nomi e codici non ci sono e restano uguali.
func trPS(script string) string {
	d := curDiz()
	if d == nil || len(d.PS) == 0 {
		return script
	}
	var b strings.Builder
	s := script
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == '#':
			j := strings.IndexByte(s[i:], '\n')
			if j < 0 {
				j = len(s) - i
			}
			b.WriteString(s[i : i+j])
			i += j
		case c == '@' && i+1 < len(s) && (s[i+1] == '\'' || s[i+1] == '"'):
			end := strings.Index(s[i+2:], "\n"+string(s[i+1])+"@")
			if end < 0 {
				b.WriteString(s[i:])
				return b.String()
			}
			b.WriteString(s[i : i+2+end+3])
			i += 2 + end + 3
		case c == '"':
			j := strings.IndexByte(s[i+1:], '"')
			if j < 0 {
				b.WriteString(s[i:])
				return b.String()
			}
			b.WriteString(s[i : i+j+2])
			i += j + 2
		case c == '\'':
			j := i + 1
			for j < len(s) {
				if s[j] == '\'' {
					if j+1 < len(s) && s[j+1] == '\'' {
						j += 2
						continue
					}
					break
				}
				j++
			}
			if j >= len(s) {
				b.WriteString(s[i:])
				return b.String()
			}
			lit := strings.ReplaceAll(s[i+1:j], "''", "'")
			if v, ok := d.PS[lit]; ok {
				b.WriteByte('\'')
				b.WriteString(strings.ReplaceAll(v, "'", "''"))
				b.WriteByte('\'')
			} else {
				b.WriteString(s[i : j+1])
			}
			i = j + 1
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// uiDictJSON: tutto il dizionario per la finestra (testi della finestra + messaggi di Go e
// degli script, che la finestra traduce quando li riceve).
func uiDictJSON() string {
	d := curDiz()
	if d == nil {
		return "null"
	}
	g := map[string]string{}
	for k, v := range d.PS {
		g[k] = v
	}
	for k, v := range d.Go {
		g[k] = v
	}
	b, _ := json.Marshal(map[string]interface{}{"t": d.UI, "g": g, "frags": d.frags, "lang": curLang()})
	return string(b)
}

// Cambio di lingua (dalla 3.2): la finestra si ricarica subito nella lingua nuova, senza chiudere e
// riaprire Tasso. Prima Tasso si riapriva: per qualche secondo la finestra restava ferma (non
// rispondeva ai clic) e, appena riaperto, i controlli automatici dell'apertura (stato del PC,
// programmi all'avvio, aggiornamenti) facevano rispondere "Tasso sta lavorando" a chi voleva
// tornare subito alla lingua di prima.
var (
	linguaGen atomic.Int64 // quante volte la pagina si è ricaricata per la lingua
	// lavoriSoloLettura: controlli che leggono e basta (molti partono da soli all'apertura): non
	// fermano il cambio di lingua, il risultato arriva alla pagina nuova come prima.
	// Alcuni nomi valgono per leggere e per scrivere (startup, bloat, updates, drvcheck, perf): così le due
	// cose non girano mai insieme. La scrittura si segna a parte con scrittura(nome) e blocca sempre.
	lavoriSoloLettura = map[string]bool{"health": true, "startup": true, "library": true, "programs": true,
		"bloat": true, "updates": true, "drvcheck": true, "net": true, "perf": true, "gpudrvcheck": true, "detect": true}
	scrittureMu      sync.Mutex
	scritture        = map[string]int{} // nome -> scritture in corso
	soloControlloAgg atomic.Bool        // "update" sta solo controllando se c'è una versione nuova (main.go)
)

// maxRicariche: dopo tante ricariche Tasso si riapre come prima (ogni ricarica lascia nella
// WebView il suo dizionario per le pagine nuove: così la memoria non cresce senza fine).
const maxRicariche = 12

// scrittura: segna che il lavoro "nome" sta cambiando qualcosa (non solo leggendo).
// Si usa con defer scrittura("nome")() all'inizio del lavoro.
func scrittura(nome string) func() {
	scrittureMu.Lock()
	scritture[nome]++
	scrittureMu.Unlock()
	return func() {
		scrittureMu.Lock()
		if scritture[nome]--; scritture[nome] <= 0 {
			delete(scritture, nome)
		}
		scrittureMu.Unlock()
	}
}

func staScrivendo(nome string) bool {
	scrittureMu.Lock()
	defer scrittureMu.Unlock()
	return scritture[nome] > 0
}

// lavoroCheBlocca: c'è un lavoro vero in corso (pulizia, Boost, driver, aggiornamento...).
// Il solo controllo degli aggiornamenti non blocca; lo scaricamento e l'aggiornamento sì.
func lavoroCheBlocca() bool {
	if isRunning() || updating.Load() || traduce.StaInstallando() {
		return true
	}
	busyMu.Lock()
	attivi := []string{}
	for k, v := range busy {
		if v {
			attivi = append(attivi, k)
		}
	}
	busyMu.Unlock()
	for _, k := range attivi {
		if (!lavoriSoloLettura[k] || staScrivendo(k)) && !(k == "update" && soloControlloAgg.Load()) {
			return true
		}
	}
	return false
}

// datiPagina: quello che la pagina riceve all'apertura (window.__TASSO_BOOT__).
func datiPagina(autoLevel int, controllaAgg bool) map[string]interface{} {
	return map[string]interface{}{
		"version":      appVersion,
		"lang":         curLang(),
		"autoLevel":    autoLevel,
		"cassetta":     strings.HasPrefix(cassettaURL, "https://"),
		"settings":     getSettings(),
		"steps":        stepList(),
		"modoFisso":    modalitaFissa(), // ecosistema.go: "gioco"/"ufficio" = niente menu per cambiare modalità
		"nome":         nomeInfo(),      // nome_utente.go: "Il tuo nome" e il biglietto (anche dopo un cambio di lingua)
		"eco":          ecoInfo(),       // ecoupdate.go: Tasso, Zip e PDF installati con l'installatore (nil se no)
		"controllaAgg": controllaAgg,
	}
}

// scriptLingua: lo script che la pagina ricaricata legge prima di tutto. Ogni ricarica ne aggiunge
// uno (la WebView li tiene tutti): vince quello con il numero più alto, in qualsiasi ordine girino.
// Anche i numeri delle chiamate ripartono da lì, così una risposta in ritardo della pagina di prima
// non finisce a una chiamata della pagina nuova.
func scriptLingua(gen int64) string {
	boot := datiPagina(0, false)
	boot["gen"] = gen
	boot["dopoLingua"] = true // la pagina nuova riapre le Impostazioni (o la prima configurazione)
	b, _ := json.Marshal(boot)
	g := strconv.FormatInt(gen, 10)
	return "(function(){var g=" + g + ";var o=window.__TASSO_LINGUA__;" +
		"if(!o||o.gen<g){window.__TASSO_LINGUA__={gen:g,boot:" + string(b) + ",i18n:" + uiDictJSON() + "};}" +
		"var r=window._rpc=(window._rpc||{nextSeq:1});if(r.nextSeq<g*1e9)r.nextSeq=g*1e9;})();"
}

// setLang cambia la lingua: la finestra si ricarica subito nella lingua nuova.
// Risposte: "ok" (la pagina si ricarica), "ok-riapre" (Tasso si chiude e si riapre),
// "ok-manual" (salvata, vale alla prossima apertura), oppure il messaggio "Tasso sta lavorando".
func setLang(l string) string {
	if !linguaDisponibile(l) {
		return "no"
	}
	// Un cambio di modalità in corso deve finire prima (la pagina nuova deve vedere quella giusta).
	if !modeMu.TryLock() {
		return occupatoIn(l)
	}
	tieni := false // Tasso si riapre: modeMu resta preso fino alla chiusura
	defer func() {
		if !tieni {
			modeMu.Unlock()
		}
	}()
	riapri := wv != nil && linguaGen.Load() >= maxRicariche
	if lavoroCheBlocca() || (riapri && occupatoPerRiaprire()) {
		return occupatoIn(l)
	}
	updateSettings(func(s *Settings) { s.Lang = l })
	scriviLinguaEcosistema(l) // ecosistema.go: Tasso Zip e Tasso PDF seguono la lingua di Tasso
	if wv == nil {
		return "ok-manual"
	}
	if riapri {
		r := riapriInLingua()
		tieni = r == "ok-riapre"
		return r
	}
	gen := linguaGen.Add(1)
	js := scriptLingua(gen)
	scriviAvvio("cambio lingua: " + l)
	wv.Dispatch(func() {
		wv.Init(js)
		wv.SetHtml(indexHTML)
	})
	return "ok"
}

// occupatoPerRiaprire: per chiudere Tasso non deve girare niente, neanche una lettura
// (come prima della 3.2), né l'installazione della traduzione in un gioco.
func occupatoPerRiaprire() bool {
	return isRunning() || updating.Load() || otherBusy("") || traduce.StaInstallando()
}

// riapriLinguaScelta: la pagina nuova non è arrivata entro 4 secondi (goRiapriLingua):
// Tasso si riapre nella lingua già salvata.
func riapriLinguaScelta() string {
	if wv == nil {
		return "ok-manual"
	}
	if !modeMu.TryLock() {
		return occupatoIn(curLang())
	}
	if occupatoPerRiaprire() {
		modeMu.Unlock()
		return occupatoIn(curLang())
	}
	r := riapriInLingua()
	if r != "ok-riapre" {
		modeMu.Unlock()
	} // se no resta preso fino alla chiusura
	return r
}

// riapriInLingua: come prima della 3.2, Tasso si chiude e si riapre (dopo tante ricariche, o se la
// pagina nuova non arriva). Chi la chiama tiene modeMu fino alla chiusura.
func riapriInLingua() string {
	exe, err := os.Executable()
	if err != nil {
		return "ok-manual"
	}
	cmd := exec.Command(exe, "--restart")
	cmd.Dir = filepath.Dir(exe)
	if cmd.Start() != nil {
		return "ok-manual"
	}
	scriviAvvio("cambio lingua: Tasso si riapre")
	go func() {
		defer guard("i18n.go:riapri")
		time.Sleep(300 * time.Millisecond)
		wv.Dispatch(func() { wv.Terminate() })
	}()
	return "ok-riapre"
}

// occupatoIn: "Tasso sta lavorando" nella lingua appena scelta (quella di adesso può essere un'altra).
func occupatoIn(l string) string {
	const it = "Tasso sta lavorando: riprova quando ha finito."
	if d := dizLingua(l); d != nil {
		if v, ok := d.Go[it]; ok {
			return v
		}
	}
	return it
}

// trAnyGo traduce tutti i testi dentro mappe ed elenchi (per il report su Discord).
func trAnyGo(v interface{}) interface{} {
	switch x := v.(type) {
	case string:
		return trGo(x)
	case map[string]interface{}:
		for k, e := range x {
			if k == "url" || k == "icon_url" || k == "color" || k == "timestamp" {
				continue
			}
			x[k] = trAnyGo(e)
		}
		return x
	case []interface{}:
		for i, e := range x {
			x[i] = trAnyGo(e)
		}
		return x
	case []map[string]interface{}:
		for i, e := range x {
			x[i] = trAnyGo(e).(map[string]interface{})
		}
		return x
	}
	return v
}
