package main

// Stato del PC su Discord: Tasso scrive un riepilogo in un canale privato
// tramite un webhook. Solo lettura: da Discord non si può comandare il PC.
// Facoltativo: un bot (tuo) che legge il canale, così scrivendo "!stato" arriva il report subito.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

var webhookRe = regexp.MustCompile(`^https://(?:(?:ptb|canary)\.)?discord(?:app)?\.com/api/webhooks/(\d{5,25})/[A-Za-z0-9_\-]{20,100}$`)

var discordMu sync.Mutex // una sola spedizione alla volta

var errWebhookGone = errors.New("il webhook non esiste più: creane uno nuovo su Discord e incollalo qui")

// Ogni quante ore arriva il report (scelta dell'utente).
var discordEveryOK = map[int]bool{3: true, 6: true, 12: true, 24: true}

func discordEvery() int {
	setMu.Lock()
	h := settings.DiscordEvery
	setMu.Unlock()
	if !discordEveryOK[h] {
		return 6
	}
	return h
}

// setDiscord salva l'indirizzo del webhook e se l'invio automatico è acceso.
func setDiscord(url string, on bool) string {
	url = strings.TrimSpace(url)
	if url != "" && !webhookRe.MatchString(url) {
		return "Questo non sembra un indirizzo webhook di Discord. Deve iniziare con https://discord.com/api/webhooks/"
	}
	if url == "" {
		on = false
	}
	updateSettings(func(s *Settings) {
		if s.DiscordWebhook != url {
			s.DiscordChannel = "" // canale nuovo: lo rileggo dal webhook
		}
		s.DiscordWebhook = url
		s.DiscordOn = on
	})
	return "ok"
}

func setDiscordEvery(h int) string {
	if !discordEveryOK[h] {
		return "Scegli 3, 6, 12 o 24 ore."
	}
	updateSettings(func(s *Settings) { s.DiscordEvery = h })
	return "ok"
}

// discordSendNow manda subito lo stato (tasto "Invia una prova").
func discordSendNow() {
	err := sendDiscordStatus("Prova")
	res := map[string]interface{}{"ok": err == nil}
	if err != nil {
		res["error"] = err.Error()
	}
	emit("onDiscord", res)
}

// startDiscordReporter: invio automatico ogni 3/6/12/24 ore (solo con Tasso acceso e l'invio attivo),
// più il bot facoltativo che risponde a "!stato".
func startDiscordReporter() {
	go func() {
		defer guard("discord.go:81")
		time.Sleep(3 * time.Minute)
		for {
			setMu.Lock()
			on := settings.DiscordOn && settings.DiscordWebhook != "" && !settings.Paused
			last := settings.DiscordLast
			setMu.Unlock()
			if on {
				every := time.Duration(discordEvery()) * time.Hour
				t, err := time.Parse(time.RFC3339, last)
				if err != nil || time.Since(t) >= every || t.After(time.Now()) {
					if e := sendDiscordStatus(""); e == errWebhookGone {
						// Il webhook è stato cancellato su Discord: spengo l'invio automatico.
						updateSettings(func(s *Settings) { s.DiscordOn = false })
					}
				}
			}
			time.Sleep(10 * time.Minute)
		}
	}()
	go discordBotLoop()
}

// discordAfterClean: dopo una pulizia completa manda il riepilogo, se attivo.
func discordAfterClean(freed int64) {
	setMu.Lock()
	on := settings.DiscordOn && settings.DiscordWebhook != "" && !settings.Paused
	setMu.Unlock()
	if on {
		go func() {
			defer guard("discord.go:110")
			_ = sendDiscordStatus("Pulizia finita: liberati " + fmtBytes(freed))
		}()
	}
}

// ---------------- Raccolta dei dati per il report ----------------

type healthData struct {
	Score  int `json:"score"`
	Issues []struct {
		Title string `json:"title"`
		Tip   string `json:"tip"`
	} `json:"issues"`
	Good []string `json:"good"`
}

// Nomi dei componenti: non cambiano mentre Tasso è aperto, quindi li leggo una volta sola.
var (
	hwOnce  sync.Once
	hwCache struct {
		CPU struct {
			Name string `json:"name"`
		} `json:"cpu"`
		GPUs []struct {
			Name string `json:"name"`
			Hz   int    `json:"hz"`
		} `json:"gpus"`
		Board struct {
			Maker string `json:"maker"`
			Model string `json:"model"`
		} `json:"board"`
		RAM struct {
			Total int64 `json:"total"`
			Speed int   `json:"speed"`
		} `json:"ram"`
		Disks []struct {
			Name   string   `json:"name"`
			Type   string   `json:"type"`
			Health string   `json:"health"`
			Temp   *float64 `json:"temp"`
			Wear   *float64 `json:"wear"`
		} `json:"disks"`
	}
)

func loadHwCache() {
	if r := runPS(hardwareScript, 2*time.Minute, nil); r.JSON != "" {
		_ = json.Unmarshal([]byte(r.JSON), &hwCache)
	}
}

// Temperature da LibreHardwareMonitor (se è aperto) o dal sensore di Windows.
const reportSensorsScript = `
$o = [ordered]@{ cpu = $null; board = $null; gpu = $null; lhm = $false }
try {
  $s = @(Get-CimInstance -Namespace root/LibreHardwareMonitor -ClassName Sensor -ErrorAction Stop | Where-Object { $_.SensorType -eq 'Temperature' })
  if ($s.Count -gt 0) {
    $o.lhm = $true
    $c = $s | Where-Object { $_.Name -match 'CPU Package|Core \(Tctl|Tctl|Tdie|CPU Total' } | Select-Object -First 1
    if ($c) { $o.cpu = [math]::Round([double]$c.Value, 0) }
    $hw = @(Get-CimInstance -Namespace root/LibreHardwareMonitor -ClassName Hardware -ErrorAction SilentlyContinue)
    $mb = @($hw | Where-Object { [string]$_.HardwareType -match 'Motherboard|SuperIO' } | ForEach-Object { $_.Identifier })
    $b = $s | Where-Object { $mb -contains $_.Parent -or $_.Parent -like '/lpc/*' } | Where-Object { $_.Name -notmatch 'CPU' } | Select-Object -First 1
    if ($b) { $o.board = [math]::Round([double]$b.Value, 0) }
    $g = $s | Where-Object { $_.Name -match 'GPU Core|GPU Hot Spot' -or $_.Parent -like '/gpu*' } | Select-Object -First 1
    if ($g) { $o.gpu = [math]::Round([double]$g.Value, 0) }
  }
} catch {}
if ($o.cpu -eq $null) {
  try {
    $z = Get-CimInstance -Namespace root/wmi -ClassName MSAcpi_ThermalZoneTemperature -ErrorAction Stop | Sort-Object -Property CurrentTemperature -Descending | Select-Object -First 1
    if ($z) { $c = ([double]$z.CurrentTemperature / 10) - 273.15; if ($c -gt 28 -or $c -lt 27) { $o.cpu = [math]::Round($c, 0) } }
  } catch {}
}
[Console]::Out.WriteLine('JSON ' + (ConvertTo-Json -InputObject $o -Compress))
`

type sensorData struct {
	CPU   *float64 `json:"cpu"`
	Board *float64 `json:"board"`
	GPU   *float64 `json:"gpu"`
	LHM   bool     `json:"lhm"`
}

func pingInternet() (int, bool) {
	start := time.Now()
	c, err := net.DialTimeout("tcp", "1.1.1.1:443", 4*time.Second)
	if err != nil {
		c, err = net.DialTimeout("tcp", "8.8.8.8:443", 4*time.Second)
		if err != nil {
			return 0, false
		}
	}
	c.Close()
	return int(time.Since(start).Milliseconds()), true
}

func tempIcon(t float64, warn, hot float64) string {
	switch {
	case t >= hot:
		return "🔥"
	case t >= warn:
		return "⚠️"
	}
	return "✅"
}

func shortName(s string) string {
	s = strings.TrimSpace(s)
	for _, cut := range []string{"(R)", "(TM)", "(tm)", "(r)", " Processor", " CPU", "NVIDIA ", "AMD ", "Intel ", " Graphics", "Corporation"} {
		s = strings.ReplaceAll(s, cut, "")
	}
	s = strings.Join(strings.Fields(s), " ")
	if i := strings.Index(s, " with "); i > 0 {
		s = s[:i]
	}
	if i := strings.Index(s, "-Core"); i > 0 {
		if j := strings.LastIndex(s[:i], " "); j > 0 {
			s = s[:j]
		}
	}
	return s
}

func num(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case uint64:
		return float64(x), true
	}
	return 0, false
}

func gb(b float64) string {
	return strings.Replace(fmt.Sprintf("%.1f", b/(1<<30)), ".", ",", 1)
}

// buildReport prepara il messaggio: voto, elenco con spunte verdi (o avvisi) e una scheda per ogni componente.
func buildReport(note string) map[string]interface{} {
	hwOnce.Do(loadHwCache)
	var h healthData
	h.Score = -1
	if r := runPS(healthScript, 3*time.Minute, nil); r.JSON != "" {
		if json.Unmarshal([]byte(r.JSON), &h) != nil {
			h.Score = -1
		}
	}
	var sens sensorData
	if r := runPS(reportSensorsScript, time.Minute, nil); r.JSON != "" {
		_ = json.Unmarshal([]byte(r.JSON), &sens)
	}
	ms, online := pingInternet()

	setMu.Lock()
	lc := settings.LastClean
	mode := settings.Mode
	botOn := settings.DiscordBotOn && settings.DiscordBot != ""
	setMu.Unlock()

	sampleGPUNow()
	ls := liveStats()
	diskFree, _ := ls["diskFree"].(uint64)
	diskTotal, _ := ls["diskTotal"].(uint64)
	ramUsed, _ := ls["ramUsed"].(uint64)
	ramTotal, _ := ls["ramTotal"].(uint64)
	cpuUse, _ := ls["cpu"].(float64)
	gpu, _ := ls["gpu"].(map[string]interface{})

	pct := func(a, b uint64) int {
		if b == 0 {
			return 0
		}
		return int(a * 100 / b)
	}

	color := 0x2FF3A6
	scoreTxt := "non disponibile"
	if h.Score >= 0 {
		bars := h.Score / 20
		scoreTxt = fmt.Sprintf("**%d/100**\n%s%s", h.Score, strings.Repeat("🟩", bars), strings.Repeat("⬜", 5-bars))
		switch {
		case h.Score < 60:
			color = 0xF25F5C
		case h.Score < 80:
			color = 0xF5B841
		}
	}

	// Processore
	cpuName := shortName(hwCache.CPU.Name)
	if cpuName == "" {
		cpuName = "Processore"
	}
	cpuLine := fmt.Sprintf("Uso %d%%", int(cpuUse+0.5))
	cpuIcon := "✅"
	if sens.CPU != nil {
		cpuLine += fmt.Sprintf(" · %.0f °C", *sens.CPU)
		cpuIcon = tempIcon(*sens.CPU, 80, 90)
	}

	// Scheda video
	gpuName := "Scheda video"
	if len(hwCache.GPUs) > 0 {
		gpuName = shortName(hwCache.GPUs[0].Name)
	}
	gpuIcon := "✅"
	var gl []string
	if u, ok := num(gpu["util"]); ok {
		gl = append(gl, fmt.Sprintf("Uso %d%%", int(u+0.5)))
	}
	gt, okT := num(gpu["temp"])
	if !okT && sens.GPU != nil {
		gt, okT = *sens.GPU, true
	}
	if okT {
		gl = append(gl, fmt.Sprintf("%.0f °C", gt))
		gpuIcon = tempIcon(gt, 80, 90)
	}
	if mu, ok := num(gpu["memUsed"]); ok {
		if mt, ok2 := num(gpu["memTot"]); ok2 && mt > 0 {
			gl = append(gl, fmt.Sprintf("VRAM %s/%s GB", gb(mu*(1<<20)), gb(mt*(1<<20))))
		}
	}
	if len(gl) == 0 {
		gl = append(gl, "funziona")
	}

	// Scheda madre
	board := strings.TrimSpace(hwCache.Board.Model)
	if board == "" {
		board = "Scheda madre"
	}
	boardLine := "funziona"
	boardIcon := "✅"
	if sens.Board != nil {
		boardLine = fmt.Sprintf("%.0f °C", *sens.Board)
		boardIcon = tempIcon(*sens.Board, 65, 80)
	} else if !sens.LHM {
		boardLine = "funziona (temperatura: apri LibreHardwareMonitor)"
	}

	// RAM
	ramP := pct(ramUsed, ramTotal)
	ramIcon := "✅"
	if ramP >= 90 {
		ramIcon = "⚠️"
	}
	ramLine := fmt.Sprintf("%s / %s GB (%d%%)", gb(float64(ramUsed)), gb(float64(ramTotal)), ramP)
	if hwCache.RAM.Speed > 0 {
		ramLine += fmt.Sprintf("\n%d MHz", hwCache.RAM.Speed)
	}

	// Disco di sistema e salute dei dischi
	dfP := pct(diskFree, diskTotal)
	diskIcon := "✅"
	if dfP < 10 {
		diskIcon = "🔥"
	} else if dfP < 20 {
		diskIcon = "⚠️"
	}
	diskLine := fmt.Sprintf("%s liberi (%d%%)", fmtBytes(int64(diskFree)), dfP)
	var dl []string
	for i, d := range hwCache.Disks {
		if i >= 3 {
			break
		}
		st := "in salute"
		ic := "✅"
		switch d.Health {
		case "Warning":
			st, ic = "da tenere d'occhio", "⚠️"
		case "Unhealthy":
			st, ic = "A RISCHIO", "🔥"
		}
		line := ic + " " + shortName(d.Name) + ": " + st
		if d.Temp != nil && *d.Temp > 0 {
			line += fmt.Sprintf(" · %.0f °C", *d.Temp)
		}
		if d.Wear != nil {
			line += fmt.Sprintf(" · usura %.0f%%", *d.Wear)
		}
		dl = append(dl, line)
	}

	netLine := "❌ Non connesso"
	if online {
		netLine = fmt.Sprintf("✅ Connesso · %d ms", ms)
	}

	lastTxt := "mai"
	if lc != nil && lc.Date != "" {
		lastTxt = lc.Date + " · " + fmtBytes(lc.Freed)
	}

	field := func(name, value string) map[string]interface{} {
		return map[string]interface{}{"name": name, "value": value, "inline": true}
	}
	fields := []map[string]interface{}{
		field("💯 Salute", scoreTxt),
		field(cpuIcon+" Processore", cpuName+"\n"+cpuLine),
		field(gpuIcon+" Scheda video", gpuName+"\n"+strings.Join(gl, " · ")),
		field(boardIcon+" Scheda madre", board+"\n"+boardLine),
		field(ramIcon+" RAM", ramLine),
		field(diskIcon+" Disco "+strings.TrimSuffix(envOr("SystemDrive", "C:"), "\\"), diskLine),
		field("🌐 Internet", netLine),
		field("⚙️ Modalità", modeName(mode)),
		field("⏱️ Acceso da", uptimeText()),
		field("🧹 Ultima pulizia", lastTxt),
		field("🔁 Prossimo report", fmt.Sprintf("tra %d ore", discordEvery())),
	}
	if len(dl) > 0 {
		fields = append(fields, map[string]interface{}{"name": "💾 Dischi", "value": strings.Join(dl, "\n"), "inline": false})
	}

	// Elenco: prima le cose da sistemare, poi le spunte verdi.
	var b strings.Builder
	if note != "" {
		b.WriteString("**" + note + "**\n")
	}
	if h.Score >= 0 {
		if len(h.Issues) == 0 {
			b.WriteString("Il PC è a posto 👍\n")
		} else {
			fmt.Fprintf(&b, "Da sistemare (%d):\n", len(h.Issues))
			for i, it := range h.Issues {
				if i >= 6 {
					break
				}
				b.WriteString("⚠️ " + it.Title + "\n")
			}
		}
		if len(h.Good) > 0 {
			b.WriteString("\nGià a posto:\n")
			for i, g := range h.Good {
				if i >= 12 {
					break
				}
				b.WriteString("✅ " + g + "\n")
			}
		}
	}
	desc := b.String()
	if len(desc) > 3900 {
		desc = desc[:3900]
	}
	footer := "Tasso " + appVersion + " · solo lettura"
	if botOn {
		footer += " · scrivi !stato per un report"
	}
	return map[string]interface{}{
		"username":         "Tasso",
		"allowed_mentions": map[string]interface{}{"parse": []string{}},
		"embeds": []map[string]interface{}{{
			"title":       "🦡 Stato del PC",
			"description": desc,
			"color":       color,
			"fields":      fields,
			"footer":      map[string]string{"text": footer},
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
		}},
	}
}

func sendDiscordStatus(note string) error {
	discordMu.Lock()
	defer discordMu.Unlock()

	setMu.Lock()
	url := settings.DiscordWebhook
	setMu.Unlock()
	if url == "" {
		return fmt.Errorf("prima incolla l'indirizzo del webhook")
	}
	if !webhookRe.MatchString(url) {
		return fmt.Errorf("l'indirizzo salvato non è un webhook di Discord: incollalo di nuovo")
	}
	b, _ := json.Marshal(trAnyGo(buildReport(note)))
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("Discord non risponde: controlla la connessione")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 || resp.StatusCode == 401 {
		return errWebhookGone
	}
	if resp.StatusCode == 429 {
		return fmt.Errorf("Discord chiede di aspettare un po': riprova tra qualche minuto")
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("Discord ha rifiutato il messaggio (codice %d)", resp.StatusCode)
	}
	updateSettings(func(s *Settings) { s.DiscordLast = time.Now().Format(time.RFC3339) })
	return nil
}

// ---------------- Bot facoltativo: report su richiesta ----------------
//
// Un webhook può solo scrivere. Per far arrivare il report quando lo chiedi serve un bot che
// legga il canale: lo crei tu (gratis) sul sito sviluppatori di Discord e incolli qui il suo token.
// Tasso controlla il canale ogni 15 secondi, solo mentre è aperto, e risponde a "!stato".
// Il bot non può fare nient'altro: Tasso non esegue mai comandi ricevuti da Discord.

var botTokenRe = regexp.MustCompile(`^[A-Za-z0-9_\-]{20,40}\.[A-Za-z0-9_\-]{4,10}\.[A-Za-z0-9_\-]{20,60}$`)
var botAsk = regexp.MustCompile(`(?i)^\s*(!stato|!report|!tasso|/stato)\b`)

var (
	botMu    sync.Mutex
	botState = map[string]interface{}{}
)

func discordAPI(method, path, token string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, "https://discord.com/api/v10"+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bot "+token)
	req.Header.Set("User-Agent", "DiscordBot (https://github.com/sergiotassoni/tasso, "+appVersion+")")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return client.Do(req)
}

// webhookChannel legge dal webhook in quale canale scrive (non serve nessun permesso).
func webhookChannel(url string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("Discord non risponde: controlla la connessione")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", errWebhookGone
	}
	var w struct {
		ChannelID string `json:"channel_id"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&w) != nil || w.ChannelID == "" {
		return "", fmt.Errorf("non riesco a leggere il canale del webhook")
	}
	return w.ChannelID, nil
}

func setBotState(m map[string]interface{}) {
	botMu.Lock()
	botState = m
	botMu.Unlock()
	emit("onDiscordBot", m)
}

func getBotState() map[string]interface{} {
	botMu.Lock()
	defer botMu.Unlock()
	out := map[string]interface{}{}
	for k, v := range botState {
		out[k] = v
	}
	return out
}

// setDiscordBot salva (protetto con Windows) il token del bot e controlla che funzioni.
// token vuoto + on=false: spegne il bot e cancella il token.
func setDiscordBot(token string, on bool) {
	token = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(token), "Bot "))
	if !on && token == "" {
		updateSettings(func(s *Settings) { s.DiscordBot = ""; s.DiscordBotOn = false })
		setBotState(map[string]interface{}{"ok": true, "off": true, "msg": "Bot spento e token cancellato."})
		return
	}
	if token == "" {
		setMu.Lock()
		token = unprotectString(settings.DiscordBot)
		setMu.Unlock()
	}
	if !botTokenRe.MatchString(token) {
		setBotState(map[string]interface{}{"ok": false, "msg": "Questo non sembra il token di un bot. Su Discord: Developer Portal > la tua app > Bot > Reset Token > Copia."})
		return
	}
	setMu.Lock()
	hook := settings.DiscordWebhook
	setMu.Unlock()
	if hook == "" {
		setBotState(map[string]interface{}{"ok": false, "msg": "Prima collega il webhook (passi 1-3), poi il bot."})
		return
	}
	resp, err := discordAPI("GET", "/users/@me", token, nil)
	if err != nil {
		setBotState(map[string]interface{}{"ok": false, "msg": "Discord non risponde: controlla la connessione."})
		return
	}
	var me struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&me)
	resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 401 {
		setBotState(map[string]interface{}{"ok": false, "msg": fmt.Sprintf("Discord ora non risponde bene (errore %d): riprova tra un minuto.", resp.StatusCode)})
		return
	}
	if resp.StatusCode == 401 || me.ID == "" {
		setBotState(map[string]interface{}{"ok": false, "msg": "Token non valido: rigeneralo (Reset Token) e incollalo di nuovo."})
		return
	}
	ch, err := webhookChannel(hook)
	if err != nil {
		setBotState(map[string]interface{}{"ok": false, "msg": err.Error()})
		return
	}
	invite := "https://discord.com/oauth2/authorize?client_id=" + me.ID + "&scope=bot&permissions=68672"
	resp, err = discordAPI("GET", "/channels/"+ch+"/messages?limit=1", token, nil)
	if err != nil {
		setBotState(map[string]interface{}{"ok": false, "msg": "Discord non risponde: controlla la connessione."})
		return
	}
	resp.Body.Close()
	enc := protectString(token)
	if enc == "" {
		setBotState(map[string]interface{}{"ok": false, "msg": "Windows non mi lascia proteggere il token: non lo salvo."})
		return
	}
	updateSettings(func(s *Settings) {
		s.DiscordBot = enc
		s.DiscordBotOn = on
		s.DiscordBotID = me.ID
		s.DiscordChannel = ch
		s.DiscordLastMsg = ""
	})
	if resp.StatusCode == 403 || resp.StatusCode == 404 {
		setBotState(map[string]interface{}{"ok": false, "name": me.Username, "invite": invite, "msg": "Token giusto (bot " + me.Username + "), ma il bot non vede il canale. Premi \"Invita il bot\", scegli il tuo server e, se il canale è privato, aggiungi il bot ai membri del canale."})
		return
	}
	setBotState(map[string]interface{}{"ok": true, "on": on, "name": me.Username, "invite": invite, "msg": "Bot " + me.Username + " collegato. Scrivi !stato nel canale: il report arriva entro un minuto."})
}

func discordBotLoop() {
	time.Sleep(20 * time.Second)
	lastAsk := time.Time{}
	fails := 0
	for {
		wait := 15 * time.Second
		setMu.Lock()
		on := settings.DiscordBotOn && settings.DiscordOn && settings.DiscordBot != "" && settings.DiscordWebhook != "" && !settings.Paused
		enc, ch, after, botID, hook := settings.DiscordBot, settings.DiscordChannel, settings.DiscordLastMsg, settings.DiscordBotID, settings.DiscordWebhook
		setMu.Unlock()
		if on && ch == "" {
			// Webhook cambiato: rileggo il canale, così il bot continua a rispondere nel canale nuovo.
			if c, err := webhookChannel(hook); err == nil && c != "" {
				ch = c
				updateSettings(func(s *Settings) { s.DiscordChannel = c; s.DiscordLastMsg = "" })
				after = ""
			} else {
				on = false
				wait = 2 * time.Minute
			}
		}
		if on {
			wait = botPoll(unprotectString(enc), ch, after, botID, &lastAsk, &fails)
		}
		time.Sleep(wait)
	}
}

func botPoll(token, ch, after, botID string, lastAsk *time.Time, fails *int) time.Duration {
	if token == "" {
		return time.Minute
	}
	if after == "" {
		// Punto di partenza: adesso. Rispondo solo ai messaggi scritti da qui in poi.
		start := fmt.Sprint((time.Now().UnixMilli() - 1420070400000) << 22)
		updateSettings(func(s *Settings) { s.DiscordLastMsg = start })
		return 15 * time.Second
	}
	resp, err := discordAPI("GET", "/channels/"+ch+"/messages?limit=20&after="+after, token, nil)
	if err != nil {
		*fails++
		return time.Duration(min(*fails, 8)) * 30 * time.Second
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == 401:
		updateSettings(func(s *Settings) { s.DiscordBotOn = false })
		setBotState(map[string]interface{}{"ok": false, "msg": "Il token del bot non vale più: rigeneralo e incollalo di nuovo."})
		return time.Minute
	case resp.StatusCode == 403 || resp.StatusCode == 404:
		setBotState(map[string]interface{}{"ok": false, "msg": "Il bot non vede più il canale: invitalo di nuovo o aggiungilo ai membri del canale."})
		return 5 * time.Minute
	case resp.StatusCode == 429:
		return time.Minute
	case resp.StatusCode != 200:
		return time.Minute
	}
	*fails = 0
	var msgs []struct {
		ID        string `json:"id"`
		Content   string `json:"content"`
		WebhookID string `json:"webhook_id"`
		Author    struct {
			ID  string `json:"id"`
			Bot bool   `json:"bot"`
		} `json:"author"`
		Mentions []struct {
			ID string `json:"id"`
		} `json:"mentions"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&msgs) != nil || len(msgs) == 0 {
		return 15 * time.Second
	}
	// Il messaggio più nuovo diventa il nuovo punto di partenza (gli ID di Discord crescono nel tempo).
	newest := after
	ask := ""
	for _, m := range msgs {
		if idNewer(m.ID, newest) {
			newest = m.ID
		}
		if m.Author.Bot || m.WebhookID != "" {
			continue
		}
		mentioned := false
		for _, u := range m.Mentions {
			if u.ID == botID {
				mentioned = true
			}
		}
		if botAsk.MatchString(m.Content) || mentioned {
			ask = m.ID
		}
	}
	updateSettings(func(s *Settings) { s.DiscordLastMsg = newest })
	if ask != "" && time.Since(*lastAsk) > 45*time.Second {
		*lastAsk = time.Now()
		if r, err := discordAPI("PUT", "/channels/"+ch+"/messages/"+ask+"/reactions/%F0%9F%91%80/@me", token, nil); err == nil {
			r.Body.Close()
		}
		go func() { defer guard("discord.go:750"); _ = sendDiscordStatus("Report richiesto da Discord") }()
	}
	return 15 * time.Second
}

// idNewer confronta due ID di Discord (numeri lunghi scritti come testo).
func idNewer(a, b string) bool {
	if len(a) != len(b) {
		return len(a) > len(b)
	}
	return a > b
}

// ---------------- Testi ----------------

func modeName(m string) string {
	switch m {
	case "gaming":
		return "Gaming"
	case "fps":
		return "FPS"
	case "film":
		return "Film"
	case "silenzioso":
		return "Silenzioso"
	case "", "normale":
		return "Normale"
	case "originale":
		return "Com'era prima di Tasso"
	}
	return m
}

var pTick64 = kernel32.NewProc("GetTickCount64")

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func uptimeText() string {
	r, _, _ := pTick64.Call()
	d := time.Duration(r) * time.Millisecond
	h := int(d.Hours())
	if h >= 24 {
		return fmt.Sprintf("%d g %d h", h/24, h%24)
	}
	return fmt.Sprintf("%d h %d min", h, int(d.Minutes())%60)
}
