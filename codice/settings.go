package main

// Impostazioni di Tasso: lettura, salvataggio e le funzioni che le cambiano dalla pagina.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"tasso/icotema"
	"tasso/segreti"
)

type LastClean struct {
	Date  string `json:"date"`
	Level int    `json:"level"`
	Freed int64  `json:"freed"`
}

type Settings struct {
	Onboarded     bool          `json:"onboarded"`
	Profile       string        `json:"profile"`
	Mode          string        `json:"mode"`
	Theme         string        `json:"theme"`
	Lang          string        `json:"lang,omitempty"`        // "it", "en", "es", "fr" o "de": lingua di Tasso
	ThemeOffice   string        `json:"themeOffice,omitempty"` // tema scelto nella modalità Ufficio (2.6)
	Autostart     bool          `json:"autostart"`
	Weekly        bool          `json:"weekly"`
	PuliziaDaDire bool          `json:"puliziaDaDire,omitempty"` // pulizia programmata accesa dall'installatore: la pagina lo deve ancora dire
	WeeklyDay     int           `json:"weeklyDay"`
	WeeklyHour    int           `json:"weeklyHour,omitempty"`
	WeeklyLevel   int           `json:"weeklyLevel,omitempty"`
	WeeklyEvery   int           `json:"weeklyEvery,omitempty"`
	Bench         []benchResult `json:"bench,omitempty"`
	BenchAround   bool          `json:"benchAround,omitempty"`
	LevelsV       int           `json:"levelsV,omitempty"` // 5 = livelli della 2.5 (Classica ... Eccezionale)
	SmartAlerts   *bool         `json:"smartAlerts"`
	LastClean     *LastClean    `json:"lastClean"`
	KnownStartup  []string      `json:"knownStartup"`
	Paused        bool          `json:"paused"`
	PrevAuto      bool          `json:"prevAutostart"`
	PrevWeekly    bool          `json:"prevWeekly"`
	AutoGame      bool          `json:"autoGame"`
	AutoGameTune  bool          `json:"autoGameTune,omitempty"` // accesa da "Regola con Tasso", non da te
	// 3.2: sfondo del desktop abbinato al tema (sfondo_desktop.go). SfondoMio*: lo sfondo di
	// Windows com'era prima che Tasso mettesse il suo (per "Rimetti il mio"); SfondoMioOk dice
	// che è stato salvato (SfondoMio "" = nessuna immagine, solo il colore).
	SfondoMio        string   `json:"sfondoMio,omitempty"`
	SfondoMioStile   string   `json:"sfondoMioStile,omitempty"`
	SfondoMioTile    string   `json:"sfondoMioTile,omitempty"`
	SfondoMioStileC  bool     `json:"sfondoMioStileC,omitempty"` // WallpaperStyle c'era (se no, si cancella)
	SfondoMioTileC   bool     `json:"sfondoMioTileC,omitempty"`  // TileWallpaper c'era (se no, si cancella)
	SfondoMioOk      bool     `json:"sfondoMioOk,omitempty"`
	SfondoTasso      string   `json:"sfondoTasso,omitempty"`      // sfondo di Tasso messo adesso ("" = quello dell'utente)
	SfondoNoChiedere bool     `json:"sfondoNoChiedere,omitempty"` // "Non chiedermelo più"
	SfondoChiesti    []string `json:"sfondoChiesti,omitempty"`    // sfondi già proposti (una volta sola ciascuno)
	// Stato del PC su Discord (webhook di un canale privato, solo lettura). L'indirizzo contiene il
	// token del webhook: in chiaro solo in memoria (DiscordWebhook, mai su disco), su disco solo
	// protetto con Windows (DPAPI, DiscordWebhookEnc) come il token del bot. DiscordWebhookVecchio:
	// il testo in chiaro delle versioni fino alla 3.1, letto solo per proteggerlo e toglierlo
	// (segretiDopoLettura).
	DiscordWebhook        string `json:"-"`
	DiscordWebhookEnc     string `json:"discordWebhookEnc,omitempty"`
	DiscordWebhookVecchio string `json:"discordWebhook,omitempty"`
	DiscordOn             bool   `json:"discordOn"`
	DiscordLast           string `json:"discordLast,omitempty"`
	DiscordEvery          int    `json:"discordEvery,omitempty"` // ore tra un report e l'altro: 3, 6, 12 o 24
	// Bot facoltativo per il report su richiesta: token protetto con Windows (DPAPI), mai in chiaro.
	DiscordBot     string `json:"discordBot,omitempty"`
	DiscordBotOn   bool   `json:"discordBotOn"`
	DiscordBotID   string `json:"discordBotId,omitempty"`
	DiscordChannel string `json:"discordChannel,omitempty"`
	DiscordLastMsg string `json:"discordLastMsg,omitempty"`
	// Modalità da rimettere quando si chiude il gioco (modalità gioco automatica).
	ModeBeforeGame string `json:"modeBeforeGame,omitempty"`
	// Modalità messa da Tasso all'apertura del gioco (3.1: salvata, così vale anche dopo uno spegnimento in partita).
	GameTarget string `json:"gameTarget,omitempty"`
	// Modalità da rimettere a fine partita dopo "Ottimizza prima di giocare".
	ModeBeforeBoost string `json:"modeBeforeBoost,omitempty"`
	// Pannellino in gioco: si mostra da solo quando parte un gioco; posizione "dx" o "sx".
	OverlayOn bool `json:"overlayOn"`
	// Tasto dell'autoclick (codice tasto di Windows e Ctrl/Alt/Maiusc); 0 = F6.
	AcVK   uint32 `json:"acVK,omitempty"`
	AcMods uint32 `json:"acMods,omitempty"`
	// Tasto del pannellino in gioco (codice come in tasti_rapidi.go); 0 = Ctrl+Maiusc+F12.
	OvKey      uint32 `json:"ovKey,omitempty"`
	OverlayPos string `json:"overlayPos,omitempty"`
	// 3.1: cosa mostra il pannellino (vuoto = tutto). Valori: cpu, cputemp, gpu, gputemp, ram, ora.
	OverlayItems []string `json:"overlayItems,omitempty"`
	// Snapshot: le impostazioni di Windows com'erano prima che Tasso le cambiasse la prima volta.
	Snapshot    json.RawMessage `json:"snapshot,omitempty"`
	SnapshotAt  string          `json:"snapshotAt,omitempty"`
	StartupOrig map[string]bool `json:"startupOrig,omitempty"`
	// "Regola con Tasso": regolazioni per gioco e modalità del PC da mettere quando parte quel gioco.
	GameTunes map[string]*GameTune `json:"gameTunes,omitempty"`
	GameModes map[string]string    `json:"gameModes,omitempty"`
	// 3.1: giochi aggiunti a mano e giochi nascosti dalla libreria.
	CustomGames []CustomGame `json:"customGames,omitempty"`
	HiddenGames []string     `json:"hiddenGames,omitempty"`
	ShowAll     bool         `json:"showAll,omitempty"` // 3.1: in Gaming mostra anche le funzioni nascoste
	// Copertine dei giochi non di Steam cercate su Steam per nome: il nome del gioco va a Valve,
	// quindi solo se l'utente lo accende (spento di base).
	CoverInternet bool `json:"coverInternet,omitempty"`
	// 3.1: "Aggiorna Tasso da solo" è acceso di base; true = l'utente l'ha spento.
	NoAutoUpdate bool `json:"noAutoUpdate,omitempty"`
	// Ecosistema (ecoupdate.go): "<sha256 di TassoSetup.exe>|<ora unix>" dell'ultimo aggiornamento
	// avviato, per non riprovarlo da solo per un giorno se non è andato.
	EcoTentata string `json:"ecoTentata,omitempty"`
	// 3.1: la X nasconde Tasso nell'icona vicino all'orologio (tray.go), acceso di base; true = l'utente l'ha spento.
	NoCloseToTray bool `json:"noCloseToTray,omitempty"`
	// 3.1: l'avviso "Tasso resta acceso qui" dall'icona è già stato mostrato (solo la prima volta).
	TrayTipShown bool `json:"trayTipShown,omitempty"`
	// Comandi gaming accesi da Tasso (per poterli rimettere com'erano) e riavvio richiesto per la GPU.
	GamingCmds  map[string]bool      `json:"gamingCmds,omitempty"`
	PingRegion  string               `json:"pingRegion,omitempty"`
	StartupV    int                  `json:"startupV,omitempty"` // 2 = elenco nuovo (2.5) con le app dello Store
	GamingPrev  map[string][]regPrev `json:"gamingPrev,omitempty"`
	HagsReboot  string               `json:"hagsReboot,omitempty"`
	GameProfile *gameProfile         `json:"gameProfile,omitempty"`
	GamePower   map[string]gamePower `json:"gamePower,omitempty"`
	// 3.1 "Tasso si ricorda": cosa hai scelto l'ultima volta nella procedura a passi di ogni gioco.
	GameMemo map[string]*GameMemo `json:"gameMemo,omitempty"`
	// 3.1 "Non chiudere mai anche": programmi che il Boost lascia sempre aperti (nomi .exe in minuscolo).
	BoostKeep []string `json:"boostKeep,omitempty"`
	// 3.1: quando hai aperto ogni gioco da Tasso (secondi), per "Giocati di recente".
	GamePlayed map[string]int64 `json:"gamePlayed,omitempty"`
	// Avvisi: cosa l'utente vuole che Tasso gli segnali (temperature, disco pieno, avvio, memoria, salute dischi).
	Alerts map[string]bool `json:"alerts,omitempty"`
	// Scrivici (3.0): la domanda dopo la prima pulizia si fa una volta sola; massimo 5 messaggi al giorno.
	FeedbackAsked bool   `json:"feedbackAsked,omitempty"`
	FeedbackDay   string `json:"feedbackDay,omitempty"`
	FeedbackCount int    `json:"feedbackCount,omitempty"`
	FeedbackSent  int    `json:"feedbackSent,omitempty"`
	// Portachiavi > Compila (vault_compila.go): finestrella sui campi di login (0 accesa di base,
	// 2 spenta), scorciatoia (modificatori<<16|tasto, 0 = spenta), avviso già letto, ci sono account.
	VtSuggest  int    `json:"vtSuggest,omitempty"`
	VtKey      uint32 `json:"vtKey,omitempty"`
	VtFirstOK  bool   `json:"vtFirstOK,omitempty"`
	VtHasItems bool   `json:"vtHasItems,omitempty"`
	// Numeri anonimi (3.0): nil = non ancora chiesto. Partono solo se la persona ha detto sì.
	Stats       *bool  `json:"stats"`
	StatsDay    string `json:"statsDay,omitempty"`
	StatsHello  string `json:"statsHello,omitempty"`
	StatsMonth  string `json:"statsMonth,omitempty"`
	StatsRuns   int    `json:"statsRuns,omitempty"`
	StatsRunDay string `json:"statsRunDay,omitempty"`
	StatsErr    int    `json:"statsErr,omitempty"`
	// Rileva i problemi (3.1): problemi che la persona ha scelto di ignorare (id del passaggio Risolvi).
	DetIgnored []string `json:"detIgnored,omitempty"`
	// Ottimizza tutto (3.2): notifiche in pausa durante il gioco, con Tasso acceso (ottimizza_win.go).
	OttNotifGioco bool `json:"ottNotifGioco,omitempty"`
}

// setPaused spegne o riaccende Tasso: da spento non parte con Windows, non fa pulizie
// automatiche e non mostra avvisi. Riaccendendolo torna tutto com'era.
func setPaused(on bool) string {
	setMu.Lock()
	paused, a, w, pa, pw := settings.Paused, settings.Autostart, settings.Weekly, settings.PrevAuto, settings.PrevWeekly
	setMu.Unlock()
	if on == paused {
		return "ok"
	}
	if on {
		autoClickStop()
		acDisarm()
		ovPost() // in pausa niente Ctrl+Maiusc+F12
		tolte := true
		for _, tn := range []string{taskAvvio, taskWeek, taskProfilo} {
			if !togliAttivita(tn) {
				tolte = false
			}
		}
		updateSettings(func(s *Settings) {
			s.PrevAuto, s.PrevWeekly = a, w
			s.Autostart, s.Weekly = false, false
			s.Paused = true
		})
		if !tolte {
			// Tasso è spento lo stesso, ma la pagina non deve dire che non parte più con Windows.
			return "ok:" + "Tasso è spento, ma alcune attività pianificate non si sono tolte: puoi cancellarle a mano in Utilità di pianificazione, cartella Tasso."
		}
		return "ok"
	}
	updateSettings(func(s *Settings) { s.Paused = false })
	ovPost()
	var errs []string
	if pa {
		if r := setAutostart(true); r != "ok" {
			errs = append(errs, r)
		}
	}
	if pw {
		if r := setWeekly(true); r != "ok" {
			errs = append(errs, r)
		}
	}
	profileTask(profileIsOn())
	if len(errs) > 0 {
		// Tasso è acceso, ma quello che non è tornato si dice (e si riaccende da Impostazioni o da Pulisci).
		return "ok:" + "Tasso è di nuovo acceso, ma qualcosa non è tornato:" + " " + strings.Join(errs, " ")
	}
	return "ok"
}

// togliAttivita cancella un'attività pianificata di Tasso. true se dopo non c'è più (anche se non
// c'era già prima); false se è ancora lì.
func togliAttivita(tn string) bool {
	if _, err := runHidden("schtasks.exe", "/Delete", "/TN", tn, "/F"); err == nil {
		return true
	}
	_, err := runHidden("schtasks.exe", "/Query", "/TN", tn)
	return err != nil
}

var validThemes = map[string]bool{"tasso": true, "aurora": true, "lava": true, "neon": true, "neonrgb": true, "oro": true, "argento": true, "carbonio": true, "belva": true, "ghiaccio": true, "sabbia": true, "ufficio": true, "carta": true, "notte": true, "salvia": true, "lavanda": true, "ardesia": true, "perla": true,
	"grafite": true, "bosco": true, "prugna": true, "petrolio": true, "tortora": true} // 3.2: salvia e lavanda restano valide (come notte) ma non si scelgono più

// Avvisi: cosa l'utente vuole che Tasso gli segnali. Se non impostato, sono tutti attivi.
var alertKeys = []string{"temp", "disk", "startup", "ram", "health"}

func defaultAlerts() map[string]bool {
	m := map[string]bool{}
	for _, k := range alertKeys {
		m[k] = true
	}
	return m
}

func setAlert(key string, on bool) string {
	valid := false
	for _, k := range alertKeys {
		if k == key {
			valid = true
		}
	}
	if !valid {
		return "avviso sconosciuto"
	}
	updateSettings(func(s *Settings) {
		if s.Alerts == nil {
			s.Alerts = defaultAlerts()
		}
		s.Alerts[key] = on
	})
	return "ok"
}

// setDetIgnored mette o toglie un problema dalla lista "Problemi ignorati" di Rileva i problemi.
func setDetIgnored(id string, on bool) string {
	known := false
	for _, st := range allSteps {
		if st.ID == id {
			known = true
			break
		}
	}
	if !known {
		return "problema sconosciuto"
	}
	updateSettings(func(s *Settings) {
		out := make([]string, 0, len(s.DetIgnored)+1)
		for _, x := range s.DetIgnored {
			if x != id {
				out = append(out, x)
			}
		}
		if on {
			out = append(out, id)
		}
		s.DetIgnored = out
	})
	return "ok"
}

func setTheme(id, look string) string {
	if !validThemes[id] {
		return "tema sconosciuto"
	}
	// Gaming e Ufficio ricordano ognuna il suo tema (look arriva dall'interfaccia: "ufficio" o "gioco").
	updateSettings(func(s *Settings) {
		if look == "ufficio" || (look == "" && s.Profile == "ufficio") {
			s.ThemeOffice = id
		} else {
			s.Theme = id
		}
	})
	return "ok"
}

// setLook cambia solo la modalità di Tasso (Gaming o Ufficio): schermate, tema e strumenti
// mostrati. Non tocca il PC (piano energetico, pulizia programmata): quello lo fa setProfile
// alla prima apertura.
func setLook(p string) string {
	if p != "gioco" && p != "ufficio" {
		return "modalità sconosciuta"
	}
	if !modalitaPermessa(p) {
		return trGo(msgModalitaFissa) // ecosistema.go: l'installatore ha messo solo l'altra modalità
	}
	updateSettings(func(s *Settings) {
		s.Onboarded = true
		s.Profile = p
	})
	return "ok"
}

// officeMode: Tasso è in modalità Ufficio (niente giochi, Boost, pannellino né autoclick).
func officeMode() bool {
	setMu.Lock()
	defer setMu.Unlock()
	return settings.Profile == "ufficio"
}

func setSmartAlerts(on bool) string {
	updateSettings(func(s *Settings) { v := on; s.SmartAlerts = &v })
	return "ok"
}

var (
	setMu    sync.Mutex
	settings = Settings{Mode: "normale"}
	// settingsReadOnly: il file c'era ma non si è potuto leggere; per non perderlo non lo riscrivo.
	settingsReadOnly bool
)

func settingsPath() string { return filepath.Join(appDir, "settings.json") }

func loadSettings() {
	b, err := leggiFileApp(settingsPath())
	if errors.Is(err, errPercorsoLink) {
		if fi, e := os.Lstat(settingsPath()); e == nil && èLink(fi) {
			// settings.json stesso è un collegamento o una giunzione (messo da un altro programma):
			// non lo seguo. Prima lo metto da parte rinominandolo (si sposta il collegamento, non il
			// file a cui porta: così, se fosse un caso voluto, non si perde niente), poi riparto
			// dalle impostazioni di base. Dopo il rinomino non c'è più: l'avviso compare una volta sola.
			via := settingsPath() + ".collegamento-" + time.Now().Format("20060102-150405")
			if rinominaApp(settingsPath(), via) == nil {
				scriviAvvio("attenzione: settings.json era un collegamento o una giunzione: messo da parte come " + filepath.Base(via) + ", riparto dalle impostazioni di base")
				go avvisaSettingsLink()
				err = os.ErrNotExist
			} else {
				scriviAvvio("attenzione: settings.json è un collegamento e non si sposta: lo lascio com'è e non salvo")
			}
		} else {
			// il collegamento è più in alto (la cartella dati o logs) o il file porta fuori: come
			// prima, non leggo e non scrivo sopra (sola lettura qui sotto)
			scriviAvvio("attenzione: settings.json passa da un collegamento o una giunzione: non lo leggo e non lo sovrascrivo")
		}
	}
	// errNonVerificato (Windows non sa dire dove porta il file) e file occupato: riprovo qui sotto e,
	// se non basta, sola lettura: le impostazioni non si perdono mai.
	for i := 0; i < 5 && err != nil && !os.IsNotExist(err) && !errors.Is(err, errPercorsoLink); i++ {
		// File occupato un attimo (antivirus, backup): riprovo invece di ripartire da zero.
		time.Sleep(400 * time.Millisecond)
		b, err = leggiFileApp(settingsPath())
	}
	if err != nil && !os.IsNotExist(err) {
		settingsReadOnly = true // non lo sovrascrivo: dentro ci sono le impostazioni originali di Windows
	}
	if err == nil {
		if json.Unmarshal(b, &settings) != nil {
			// File rovinato: lo metto da parte invece di sovrascriverlo (dentro c'è il backup delle
			// impostazioni), senza il webhook in chiaro (segreti.TogliInChiaro).
			pulito, _ := segreti.TogliInChiaro(b)
			_ = scriviFileApp(settingsPath()+".bad", pulito, 0o644)
			settings = Settings{Mode: "normale"}
		} else if segretiDopoLettura(&settings) && !settingsReadOnly {
			// 3.2: il webhook di Discord era in chiaro (fino alla 3.1): adesso è protetto e il
			// testo in chiaro sparisce subito dal file
			saveSettings()
			scriviAvvio("indirizzo del webhook di Discord protetto con Windows e tolto in chiaro da settings.json")
		}
	}
	togliWebhookCopie()
	if settings.Mode == "" {
		settings.Mode = "normale"
	}
	if settings.LevelsV < 5 {
		// Dalla 2.4: i livelli sono saliti di uno (è nata la Classica).
		if settings.LastClean != nil && settings.LastClean.Level > 0 {
			settings.LastClean.Level++
		}
		settings.LevelsV = 5
		if err == nil {
			saveSettings() // subito, così al prossimo avvio non li alzo di nuovo
		}
	}
	if fixSettingsDefaults() && err == nil && !settingsReadOnly {
		saveSettings() // 3.2: tema dell'Ufficio ritirato già sostituito, non lo rifaccio a ogni avvio
	}
}

// fixSettingsDefaults mette i valori di partenza dove mancano (temi, avvisi). True se ha portato
// un tema dell'Ufficio ritirato al suo sostituto (da salvare).
func fixSettingsDefaults() (cambiato bool) {
	if !validThemes[settings.Theme] {
		settings.Theme = "tasso"
	}
	if settings.ThemeOffice != "" && !validThemes[settings.ThemeOffice] {
		settings.ThemeOffice = ""
	}
	// 3.2: Salvia e Lavanda non si scelgono più: chi le aveva passa al tema chiaro più simile
	// (Carta e Perla). Chi chiama salva (loadSettings), così succede una volta sola.
	if nuovo, ok := icotema.TemaUfficioSostituto(settings.ThemeOffice); ok {
		settings.ThemeOffice = nuovo
		cambiato = true
	}
	if settings.SmartAlerts == nil {
		v := true
		settings.SmartAlerts = &v
	}
	if settings.Alerts == nil {
		settings.Alerts = defaultAlerts()
	} else {
		for _, k := range alertKeys {
			if _, ok := settings.Alerts[k]; !ok {
				settings.Alerts[k] = true
			}
		}
	}
	return cambiato
}

// saveSettings scrive le impostazioni. Restituisce true se invece ha ricaricato il file vero
// (era illeggibile all'avvio): chi ha appena fatto un cambio lo deve rifare su quello.
func saveSettings() bool {
	if settingsReadOnly {
		// All'avvio il file non si leggeva: prima di scriverci sopra ne salvo una copia.
		old, err := leggiFileApp(settingsPath())
		if err != nil && !os.IsNotExist(err) {
			return false
		}
		if err == nil {
			pulito, _ := segreti.TogliInChiaro(old) // la copia non tiene il webhook in chiaro
			_ = scriviFileApp(settingsPath()+".prima", pulito, 0o644)
			// Il file adesso si legge: quello è quello vero. Lo tengo così com'è invece di
			// sovrascriverlo con le impostazioni di partenza di questa sessione (si perde al massimo
			// l'ultimo cambio fatto adesso, non tutto il resto).
			var o Settings
			if json.Unmarshal(old, &o) == nil {
				segretiDopoLettura(&o) // il file vero, riletto: webhook protetto anche qui
				if o.Mode == "" {
					o.Mode = "normale"
				}
				settings = o
				settingsReadOnly = false
				fixSettingsDefaults()
				return true
			}
		}
		settingsReadOnly = false
	}
	b, _ := json.MarshalIndent(settings, "", "  ")
	// Scrittura sicura: prima un file temporaneo, poi lo scambio (niente file a metà se il PC si spegne).
	tmp := settingsPath() + ".tmp"
	// sicuro_scrittura.go: niente scritture attraverso collegamenti o giunzioni nella cartella dati.
	// Un .tmp che è un collegamento lo tolgo (si toglie il collegamento, non il file a cui punta);
	// lo scambio finale sostituisce un settings.json collegamento invece di seguirlo.
	if fi, err := os.Lstat(tmp); err == nil && èLink(fi) {
		_ = os.Remove(tmp)
	}
	if scriviFileApp(tmp, b, 0o644) == nil {
		if rinominaApp(tmp, settingsPath()) != nil {
			_ = scriviFileApp(settingsPath(), b, 0o644)
			_ = os.Remove(tmp)
		}
	}
	return false
}

func getSettings() map[string]interface{} {
	setMu.Lock()
	defer setMu.Unlock()
	b, _ := json.Marshal(settings)
	m := map[string]interface{}{}
	_ = json.Unmarshal(b, &m)
	// Il token del bot (anche se protetto) non va mai all'interfaccia: basta sapere se c'è.
	m["discordBotSaved"] = settings.DiscordBot != ""
	// Il webhook neanche: la finestra vede l'indirizzo senza il token (setDiscord lo riconosce).
	m["discordWebhook"] = segreti.Maschera(settings.DiscordWebhook)
	delete(m, "discordWebhookEnc")
	m["autoGameAll"] = autoGameForAll(&settings) // i giochi senza una loro potenza vanno in Gaming
	delete(m, "discordBot")
	delete(m, "vault")
	return m
}

func updateSettings(f func(s *Settings)) {
	setMu.Lock()
	f(&settings)
	if saveSettings() {
		f(&settings) // rifaccio il cambio sulle impostazioni vere appena ricaricate
		saveSettings()
	}
	setMu.Unlock()
}

// setProfile applica il profilo scelto alla prima apertura (gioco, ufficio, entrambi).
func setProfile(p string) string {
	switch p {
	case "gioco", "ufficio", "entrambi":
	default:
		return "profilo sconosciuto"
	}
	if !modalitaPermessa(p) {
		return trGo(msgModalitaFissa) // ecosistema.go: l'installatore ha messo solo l'altra modalità
	}
	updateSettings(func(s *Settings) {
		s.Onboarded = true
		s.Profile = p
	})
	lookChanged(p == "ufficio")
	if r := profiloPC(p); r != "ok" {
		return "ok:" + r // la scelta è salvata, ma la pulizia programmata non si è accesa: la pagina lo dice
	}
	return "ok"
}

// profiloPC: la parte di setProfile che regola il PC (modalità e pulizia settimanale). La usa
// anche la prima apertura dopo l'installatore, che la scelta l'ha già fatta (ecosistema.go).
// La modalità di Tasso cambia solo cosa vedi: con Gaming il PC non si tocca per niente (piano
// energetico, risparmio energia di Windows 11 e notifiche restano come sono). Gaming e FPS li mette
// il Boost, solo mentre giochi. La pulizia programmata non si tocca mai, in nessuna modalità:
// di base è spenta (decisione di Sergio, prova sul PC vero di Tasso 3.2) e chi l'aveva già accesa
// in una versione di prima la tiene com'è. Restituisce sempre "ok".
func profiloPC(p string) string {
	setMu.Lock()
	prima := settings.Mode
	setMu.Unlock()
	if m := modoPrimoAvvio(p, prima); m != "" {
		go setMode(m)
	}
	return "ok"
}

// modoPrimoAvvio: la modalità del PC da mettere alla prima scelta del profilo ("" = non toccare).
// Gaming: mai niente (nemmeno lo script Normale, che rimetterebbe la modalità risparmio energia
// di Windows 11 su Bilanciato). Ufficio: Normale, come prima.
func modoPrimoAvvio(profilo, modoAttuale string) string {
	switch profilo {
	case "ufficio", "entrambi":
		return "normale"
	}
	return ""
}

// sysExe: i programmi di Windows (schtasks, powercfg, taskkill, shutdown, powershell...) li avvio
// sempre dalla cartella di sistema, mai da un'altra cartella del PATH dove qualcuno potrebbe
// averne messo uno finto (Tasso gira come amministratore).
func sysExe(name string) string {
	if strings.ContainsAny(name, `\/:`) {
		return name
	}
	sys, err := windows.GetSystemDirectory()
	if err != nil || sys == "" {
		return name
	}
	p := filepath.Join(sys, name)
	if strings.EqualFold(name, "powershell.exe") {
		p = filepath.Join(sys, "WindowsPowerShell", "v1.0", "powershell.exe")
	}
	if fileExists(p) {
		return p
	}
	return name
}

// runHiddenTimeout è come runHidden ma chiude il programma se ci mette troppo (es. nvidia-smi bloccato).
func runHiddenTimeout(d time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	cmd := exec.CommandContext(ctx, sysExe(name), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runHidden(name string, args ...string) (string, error) {
	cmd := exec.Command(sysExe(name), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

const (
	taskAvvio = "Tasso\\Avvio con Windows"
	taskWeek  = "Tasso\\Pulizia settimanale"
)

// taskExeOK: in questa apertura di Tasso la copia in Programmi è già a posto (non la rileggo:
// sono due file da ~40 MB ogni volta).
var (
	taskExeMu sync.Mutex
	taskExeOK string
)

// taskExe: le attività pianificate partono con i permessi da amministratore, quindi non
// devono puntare a una cartella dove chiunque può scrivere (es. Download). Copio Tasso.exe
// in Programmi\Tasso, che solo un amministratore può modificare, e uso quella copia.
// Se la copia non si può fare restituisce "": meglio nessuna attività che una che fa partire
// come amministratore un file che chiunque può sostituire.
// Accanto alla copia c'è VERSIONE: non rimetto mai una versione vecchia sopra una più nuova
// (es. dopo un aggiornamento fatto dalla copia in Programmi).
func taskExe() string {
	taskExeMu.Lock()
	defer taskExeMu.Unlock()
	if taskExeOK != "" && fileExists(taskExeOK) {
		return taskExeOK
	}
	r := taskExeCheck()
	if r != "" {
		taskExeOK = r
	}
	debug.FreeOSMemory() // restituisco subito a Windows la memoria usata per confrontare i file
	return r
}

func taskExeCheck() string {
	if programFilesDir() == "" || exePath == "" {
		return ""
	}
	dir := cartellaTasso() // C:\Program Files\Tasso, o la cartella dell'installatore dentro Programmi (ecosistema.go)
	dst := filepath.Join(dir, "Tasso.exe")
	if strings.EqualFold(filepath.Clean(exePath), dst) {
		return dst
	}
	verFile := filepath.Join(dir, "VERSIONE")
	if fileExists(dst) {
		if v, err := os.ReadFile(verFile); err == nil && verNewer(strings.TrimSpace(string(v)), appVersion) {
			return dst
		}
	}
	src, err := readSelfExe()
	if err != nil {
		return existingOr(dst)
	}
	if old, err := os.ReadFile(dst); err == nil && bytes.Equal(old, src) {
		_ = os.WriteFile(verFile, []byte(appVersion), 0o644)
		return dst
	}
	if os.MkdirAll(dir, 0o755) != nil {
		return ""
	}
	// La copia in Programmi la avviano le attività di Windows da amministratore: ci metto solo un
	// Tasso con la firma digitale di Sergio Tassoni (Certum). Tasso.exe nella cartella dove l'hai
	// estratto lo potrebbe scambiare un altro programma mentre parte.
	if !copiaFirmata(src) && !fileExists(filepath.Join(dir, "prova-non-firmata")) {
		return existingOr(dst)
	}
	if installProgramCopy(dir, src, appVersion) != nil {
		return existingOr(dst)
	}
	return dst
}

func existingOr(dst string) string {
	if fileExists(dst) {
		return dst
	}
	return ""
}

var progCopyMu sync.Mutex

// installProgramCopy mette exe in Programmi\Tasso\Tasso.exe (prima in un file a parte, poi lo scambia)
// e scrive la versione in VERSIONE.
func installProgramCopy(dir string, exe []byte, ver string) error {
	progCopyMu.Lock()
	defer progCopyMu.Unlock()
	dst := filepath.Join(dir, "Tasso.exe")
	tmp := dst + ".new"
	if err := os.WriteFile(tmp, exe, 0o755); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	old := dst + ".old"
	_ = os.Remove(old)
	if fileExists(dst) {
		// Se la copia in Programmi è aperta (Tasso partito da lì) la rinomino: Windows lo permette.
		if err := os.Rename(dst, old); err != nil {
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Rename(old, dst)
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Remove(old) // se è in uso resta lì e si toglie la volta dopo
	return os.WriteFile(filepath.Join(dir, "VERSIONE"), []byte(ver), 0o644)
}

// logonTaskScript crea un'attività che parte quando entri in Windows: anche a batteria,
// senza essere chiusa se stacchi il caricatore e senza tempo massimo (schtasks da solo
// metterebbe "solo con la corrente" e "chiudi dopo 3 giorni").
const logonTaskScript = `
try {
  $who = [Security.Principal.WindowsIdentity]::GetCurrent().Name
  $act = New-ScheduledTaskAction -Execute ([string]$A.exe) -Argument ([string]$A.arg)
  $trg = New-ScheduledTaskTrigger -AtLogOn -User $who
  $set = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit ([TimeSpan]::Zero) -MultipleInstances IgnoreNew
  $prn = New-ScheduledTaskPrincipal -UserId $who -LogonType Interactive -RunLevel Highest
  Register-ScheduledTask -TaskPath '\Tasso\' -TaskName ([string]$A.name) -Action $act -Trigger $trg -Settings $set -Principal $prn -Force -ErrorAction Stop | Out-Null
  Say 'TASK_OK'
} catch { Say ('Errore: ' + $_.Exception.Message) }
`

// createLogonTask: name è il nome dentro la cartella Tasso (es. "Avvio con Windows").
func createLogonTask(name, arg string) bool {
	exe := taskExe()
	if exe == "" {
		return false
	}
	ok := false
	runPSArgs(logonTaskScript, map[string]string{"name": name, "exe": exe, "arg": arg}, 2*time.Minute, func(m string) {
		if m == "TASK_OK" {
			ok = true
		}
	})
	return ok
}

func setAutostart(on bool) string {
	if on {
		if !createLogonTask("Avvio con Windows", "--min") {
			if taskExe() == "" {
				_, _ = runHidden("schtasks.exe", "/Delete", "/TN", taskAvvio, "/F")
			}
			return "Non sono riuscito a impostare l'avvio con Windows."
		}
	} else {
		_, _ = runHidden("schtasks.exe", "/Delete", "/TN", taskAvvio, "/F")
	}
	updateSettings(func(s *Settings) { s.Autostart = on })
	profileTask(profileIsOn())
	return "ok"
}

// refreshScheduledTasks aggiorna il percorso di Tasso.exe nelle attività pianificate,
// nel caso il file sia stato spostato.
func refreshScheduledTasks() {
	time.Sleep(3 * time.Second)
	// \Tasso\Apri Tasso: apre Tasso da amministratore senza chiedere (apri_admin.go). Anche per chi
	// passa dalla 3.1 alla 3.1.1 con l'aggiornamento di Tasso, senza l'installatore.
	func() {
		defer guard("apri-admin")
		if m := preparaAttivitaApri(); m != "" {
			scriviAvvio("attività Apri Tasso: " + m) // logs\avvio.txt, per capire perché chiede i permessi
		}
	}()
	setMu.Lock()
	a, w, p := settings.Autostart, settings.Weekly, settings.Paused
	setMu.Unlock()
	if p {
		return
	}
	if a {
		setAutostart(true)
	}
	if w {
		setWeekly(true)
	}
	if profileIsOn() {
		profileTask(true)
	}
}

// avvisaSettingsLink: una volta sola (il salvataggio sostituisce il collegamento con un file vero),
// appena c'è l'icona vicino all'orologio.
func avvisaSettingsLink() {
	defer guard("avvisaSettingsLink")
	for i := 0; i < 120 && trayHwnd.Load() == 0; i++ {
		time.Sleep(500 * time.Millisecond)
	}
	trayAvviso("Tasso", "Il file delle impostazioni di Tasso era stato cambiato da un altro programma: per sicurezza riparto dalle impostazioni di base. Controlla le tue scelte in Tasso.", false)
}

// segretiDopoLettura: dopo aver letto settings.json. Il webhook di Discord in chiaro (fino alla
// 3.1) si protegge con Windows (DPAPI) e si toglie; quello protetto si sblocca in memoria.
// true = il file va riscritto (c'era il testo in chiaro).
func segretiDopoLettura(s *Settings) bool {
	chiaro, enc, cambiato := segreti.Migra(s.DiscordWebhookVecchio, s.DiscordWebhookEnc, protectString, unprotectString)
	if cambiato && enc == "" {
		// Windows non ha protetto: vale solo per adesso, in chiaro su disco non torna
		scriviAvvio("attenzione: Windows non ha protetto l'indirizzo del webhook di Discord: lo tengo solo finché Tasso è aperto (va incollato di nuovo)")
	}
	s.DiscordWebhook, s.DiscordWebhookEnc, s.DiscordWebhookVecchio = chiaro, enc, ""
	return cambiato
}

// togliWebhookCopie: le copie di settings.json fatte da Tasso (.prima, .bad, .tmp) non devono
// tenere il webhook in chiaro delle versioni di prima.
func togliWebhookCopie() {
	for _, suf := range []string{".prima", ".bad", ".tmp"} {
		p := settingsPath() + suf
		b, err := leggiFileApp(p)
		if err != nil {
			continue
		}
		if pulito, ok := segreti.TogliInChiaro(b); ok {
			_ = scriviFileApp(p, pulito, 0o644)
		}
	}
}
