package main

// Lingue di Tasso: italiano (quella "vera", scritta nel codice) e inglese.
// Le traduzioni stanno in i18n/en.json: "ui" per i testi della finestra, "go" per i messaggi
// di Go e "ps" per i testi degli script PowerShell. In italiano non si traduce niente.

import (
	_ "embed"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed i18n/en.json
var enJSON []byte

var (
	i18nOnce sync.Once
	enDict   struct {
		UI map[string]string `json:"ui"`
		Go map[string]string `json:"go"`
		PS map[string]string `json:"ps"`
	}
	goFrags []string // pezzi dei messaggi di Go, dal più lungo al più corto
)

var pUILang = kernel32.NewProc("GetUserDefaultUILanguage")

func loadI18n() {
	i18nOnce.Do(func() {
		_ = json.Unmarshal(enJSON, &enDict)
		for k := range enDict.Go {
			if len(k) >= 6 {
				goFrags = append(goFrags, k)
			}
		}
		sort.Slice(goFrags, func(i, j int) bool { return len(goFrags[i]) > len(goFrags[j]) })
	})
}

// systemLang: la lingua di Windows (italiano se Windows è in italiano, altrimenti inglese).
func systemLang() string {
	r, _, _ := pUILang.Call()
	if r&0x3ff == 0x10 { // LANG_ITALIAN
		return "it"
	}
	return "en"
}

// curLang: la lingua scelta (o quella di Windows se non l'hai ancora scelta).
func curLang() string {
	setMu.Lock()
	l := settings.Lang
	setMu.Unlock()
	if l == "it" || l == "en" {
		return l
	}
	return systemLang()
}

func isEN() bool { return curLang() == "en" }

// trGo traduce un messaggio di Go: prima tutto intero, poi pezzo per pezzo
// (per i messaggi con dentro numeri o nomi).
func trGo(s string) string {
	if s == "" || !isEN() {
		return s
	}
	loadI18n()
	if v, ok := enDict.Go[s]; ok {
		return v
	}
	if v, ok := enDict.PS[s]; ok {
		return v
	}
	out := s
	for _, k := range goFrags {
		if strings.Contains(out, k) {
			out = strings.ReplaceAll(out, k, enDict.Go[k])
		}
	}
	return out
}

// trPS traduce i testi tra apici singoli di uno script PowerShell ('...'), solo quelli
// che sono nel dizionario: percorsi, nomi e codici non ci sono e restano uguali.
func trPS(script string) string {
	if !isEN() {
		return script
	}
	loadI18n()
	if len(enDict.PS) == 0 {
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
			if v, ok := enDict.PS[lit]; ok {
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
	if !isEN() {
		return "null"
	}
	loadI18n()
	g := map[string]string{}
	for k, v := range enDict.PS {
		g[k] = v
	}
	for k, v := range enDict.Go {
		g[k] = v
	}
	b, _ := json.Marshal(map[string]interface{}{"t": enDict.UI, "g": g, "frags": goFrags})
	return string(b)
}

// setLang cambia la lingua e riapre Tasso (i testi si caricano all'apertura).
func setLang(l string) string {
	if l != "it" && l != "en" {
		return "no"
	}
	if isRunning() || otherBusy("") {
		if l == "en" {
			return "Tasso is busy right now: try again when it has finished."
		}
		return "Tasso sta lavorando: riprova quando ha finito."
	}
	updateSettings(func(s *Settings) { s.Lang = l })
	exe, err := os.Executable()
	if err != nil {
		return "ok-manual"
	}
	cmd := exec.Command(exe, "--restart")
	cmd.Dir = filepath.Dir(exe)
	if cmd.Start() != nil {
		return "ok-manual"
	}
	go func() {
		defer guard("i18n.go:201")
		time.Sleep(300 * time.Millisecond)
		wv.Dispatch(func() { wv.Terminate() })
	}()
	return "ok"
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
