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
	"strings"
	"sync"
	"time"
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
	i18nMu   sync.Mutex
	i18nDiz  = map[string]*dizionario{}
	pUILang  = kernel32.NewProc("GetUserDefaultUILanguage")
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

// setLang cambia la lingua e riapre Tasso (i testi si caricano all'apertura).
func setLang(l string) string {
	if !linguaDisponibile(l) {
		return "no"
	}
	// Un cambio di modalità in corso deve finire prima del riavvio (se no Tasso perde quale modalità
	// ha messo): tengo modeMu fino alla chiusura, come l'aggiornamento.
	if !modeMu.TryLock() {
		return occupatoIn(l)
	}
	locked := true
	defer func() {
		if locked {
			modeMu.Unlock()
		}
	}()
	if isRunning() || otherBusy("") {
		return occupatoIn(l)
	}
	updateSettings(func(s *Settings) { s.Lang = l })
	scriviLinguaEcosistema(l) // ecosistema.go: Tasso Zip e Tasso PDF seguono la lingua di Tasso
	exe, err := os.Executable()
	if err != nil {
		return "ok-manual"
	}
	cmd := exec.Command(exe, "--restart")
	cmd.Dir = filepath.Dir(exe)
	if cmd.Start() != nil {
		return "ok-manual"
	}
	locked = false // resta preso fino alla chiusura
	go func() {
		defer guard("i18n.go:201")
		time.Sleep(300 * time.Millisecond)
		wv.Dispatch(func() { wv.Terminate() })
	}()
	return "ok"
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
