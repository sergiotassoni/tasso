package main

// Numeri anonimi (3.0): solo se la persona ha detto sì (Settings.Stats).
// Tasso manda alla cassetta pochi numeri senza nome: versione, Windows 10 o 11, marca del processore
// e della scheda video, quanta RAM, quanto spazio libera, quali pulsanti usa e quanti errori ha fermato.
// Mai nomi, file, percorsi, indirizzi o numeri di serie. Il paese lo ricava la cassetta, senza salvare l'IP.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const statsMaxRuns = 20 // pulizie/soluzioni contate al giorno da questo PC

func statsOn() bool {
	setMu.Lock()
	defer setMu.Unlock()
	return strings.HasPrefix(cassettaURL, "https://") && settings.Stats != nil && *settings.Stats
}

func setStats(on bool) string {
	updateSettings(func(s *Settings) { v := on; s.Stats = &v })
	if on {
		go func() { defer guard("numeri"); statsPending() }()
	}
	return "ok"
}

// Marche e fasce: solo poche parole fisse, così non può uscire niente di personale.
func statsCPU(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "intel"):
		return "Intel"
	case strings.Contains(n, "amd") || strings.Contains(n, "ryzen"):
		return "AMD"
	case n == "":
		return ""
	}
	return "Altro"
}

func statsGPU(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "nvidia") || strings.Contains(n, "geforce") || strings.Contains(n, "rtx") || strings.Contains(n, "gtx"):
		return "NVIDIA"
	case strings.Contains(n, "amd") || strings.Contains(n, "radeon"):
		return "AMD"
	case strings.Contains(n, "intel"):
		return "Intel"
	case n == "":
		return ""
	}
	return "Altro"
}

func statsRAM(bytes float64) string {
	gb := bytes / (1 << 30)
	switch {
	case gb <= 0:
		return ""
	case gb < 6:
		return "4 GB"
	case gb < 12:
		return "8 GB"
	case gb < 24:
		return "16 GB"
	case gb < 48:
		return "32 GB"
	}
	return "64 GB o più"
}

func statsWin(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "windows 11"):
		return "Windows 11"
	case strings.Contains(n, "windows 10"):
		return "Windows 10"
	case n == "":
		return ""
	}
	return "Altro"
}

// statsStep: nome del passaggio, solo lettere, numeri e poca punteggiatura (sono nomi scritti da noi).
var statsStepRe = regexp.MustCompile(`^[\p{L}\p{N} '’().,+/&:-]{1,60}$`)

func statsStep(n string) string {
	n = strings.TrimSpace(n)
	if !statsStepRe.MatchString(n) {
		return ""
	}
	return n
}

// statsErrors: quanti errori fermati (o chiusure) ci sono in crash.txt.
func statsErrors() int {
	b, err := os.ReadFile(filepath.Join(appDir, "logs", "crash.txt"))
	if err != nil {
		return 0
	}
	n := 0
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "--- Errore fermato") || strings.HasPrefix(l, "panic:") || strings.HasPrefix(l, "fatal error:") {
			n++
		}
	}
	return n
}

var statsHW = map[string]string{}

// statsHardware arriva dall'interfaccia quando ha letto il PC: tengo solo marca e fascia.
func statsHardware(osName, cpu, gpu string, ram float64) string {
	setMu.Lock()
	statsHW = map[string]string{"win": statsWin(osName), "cpu": statsCPU(cpu), "gpu": statsGPU(gpu), "ram": statsRAM(ram)}
	setMu.Unlock()
	go func() { defer guard("numeri"); statsPending() }()
	return "ok"
}

func statsPost(ev map[string]interface{}) bool {
	b, _ := json.Marshal(ev)
	req, err := http.NewRequest("POST", cassettaURL+"/v1/uso", bytes.NewReader(b))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Tasso/"+appVersion)
	c := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<14))
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func statsBase(ev string) map[string]interface{} {
	setMu.Lock()
	prof := "gaming"
	if settings.Profile == "ufficio" {
		prof = "ufficio"
	}
	setMu.Unlock()
	return map[string]interface{}{"ev": ev, "ver": appVersion, "lang": curLang(), "prof": prof}
}

// statsPending: una volta al giorno "Tasso è stato aperto" (con il PC, se l'ho già letto).
func statsPending() {
	if !statsOn() {
		return
	}
	today := time.Now().Format("2006-01-02")
	month := today[:7]
	setMu.Lock()
	hw := statsHW
	done := settings.StatsHello == today
	newMonth := settings.StatsMonth != month
	seen := settings.StatsErr
	setMu.Unlock()
	if done || hw["win"] == "" && hw["cpu"] == "" {
		return
	}
	errs := statsErrors()
	delta := errs - seen
	if delta < 0 { // crash.txt è stato rinnovato
		delta = errs
	}
	if delta > 50 {
		delta = 50
	}
	ev := statsBase("avvio")
	for k, v := range hw {
		ev[k] = v
	}
	ev["mese"] = newMonth
	ev["errori"] = delta
	if statsPost(ev) {
		updateSettings(func(s *Settings) { s.StatsHello, s.StatsMonth, s.StatsErr = today, month, errs })
	}
}

// statsRun: dopo una pulizia o una soluzione. names = passaggi fatti.
func statsRun(level int, names []string, freed int64, secs int, cancelled bool) {
	if !statsOn() {
		return
	}
	today := time.Now().Format("2006-01-02")
	ok := false
	updateSettings(func(s *Settings) {
		if s.StatsRunDay != today {
			s.StatsRunDay, s.StatsRuns = today, 0
		}
		if s.StatsRuns < statsMaxRuns {
			s.StatsRuns++
			ok = true
		}
	})
	if !ok {
		return
	}
	steps := []string{}
	for _, n := range names {
		if v := statsStep(n); v != "" && len(steps) < 30 {
			steps = append(steps, v)
		}
	}
	if freed < 0 {
		freed = 0
	}
	ev := statsBase("pulizia")
	ev["livello"] = level
	ev["passi"] = steps
	ev["liberati"] = freed
	ev["secondi"] = secs
	ev["annullata"] = cancelled
	statsPost(ev)
	statsPending()
}
