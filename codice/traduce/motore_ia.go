package traduce

// Motore potente: un modello di intelligenza artificiale aperto che gira SUL PC con
// llama.cpp (licenza MIT), usando la scheda video con Vulkan (NVIDIA e AMD).
// llama-server ascolta solo su 127.0.0.1: niente esce dal computer.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// istruzioni fisse per il modello: traduttore di videogiochi, solo la traduzione in uscita
const promptSistema = `Sei un traduttore professionista di videogiochi dall'inglese all'italiano.
Regole:
- Traduci in italiano naturale e scorrevole, come in un gioco localizzato bene.
- Mantieni il tono del personaggio: slang, battute, minacce, emozioni, modo di parlare (pirati, soldati, ecc.).
- Testi dei menu e dei comandi: brevi e chiari, come nei giochi italiani ("Premi", "Tieni premuto", "Salvataggio automatico").
- Non tradurre i nomi propri di persone e luoghi inventati, i tasti (E, Spazio si traduce, Shift no) e i numeri.
- Usa sempre le traduzioni del glossario quando sono indicate.
- Rispondi SOLO con la traduzione, senza spiegazioni, virgolette o note.`

type RichiestaTraduzione struct {
	Testo     string        // testo inglese già pulito
	Parlante  string        // nome di chi parla, se riconosciuto
	Contesto  []string      // ultime battute inglesi (dalla più vecchia), per capire il discorso
	Glossario []voceTrovata // voci del glossario presenti nel testo
	Bozza     string        // modo correttore: traduzione già fatta da Mozilla, da correggere solo se sbagliata
	Da, A     string        // coppia di lingue se non è inglese → italiano
}

// istruzioni del modo correttore: il modello NON traduce da zero, controlla la traduzione di Mozilla.
// Niente battute precedenti: con il contesto i modelli piccoli inventano frasi (visto con Phi-4 mini).
const promptCorrettore = `Sei un localizzatore professionista di videogiochi dall'inglese all'italiano.
Ricevi una frase inglese di un gioco e una traduzione automatica fatta da un programma.
Il tuo compito: restituire la traduzione italiana MIGLIORE, come in un gioco localizzato bene.
- Se la traduzione automatica è già corretta e naturale, riscrivila identica.
- Se sbaglia il senso, usa parole sbagliate per il contesto (es. "rivista" per "magazine" di un'arma), ha errori di grammatica, di genere o di accordo, o suona innaturale, correggila.
- Espressioni idiomatiche e modi di dire: rendili con il modo di dire italiano equivalente, non parola per parola.
- Mantieni tono e registro del personaggio (slang, battute, minacce). Dai del tu al giocatore.
- Comandi e menu: brevi, imperativo ("Premi", "Tieni premuto").
- Non aggiungere né togliere informazioni. Numeri, nomi propri e tasti restano uguali.
- Usa le traduzioni del glossario quando indicate.
Rispondi SOLO con la traduzione italiana finale.`

type messaggio struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func costruisciMessaggi(r RichiestaTraduzione) []messaggio {
	if r.Bozza != "" && r.Da != "" && !(r.Da == "en" && r.A == "it") {
		return costruisciCorrezioneCoppia(r.Testo, r.Bozza, r.Da, r.A)
	}
	if r.Bozza != "" {
		return costruisciCorrezione(r)
	}
	var b strings.Builder
	if len(r.Contesto) > 0 {
		b.WriteString("Battute precedenti (solo per capire il contesto, NON tradurle):\n")
		for _, c := range r.Contesto {
			b.WriteString("- " + c + "\n")
		}
		b.WriteString("\n")
	}
	if len(r.Glossario) > 0 {
		b.WriteString("Glossario da rispettare:\n")
		for _, v := range r.Glossario {
			b.WriteString("- " + v.Inglese + " = " + v.Italiano + "\n")
		}
		b.WriteString("\n")
	}
	if r.Parlante != "" {
		b.WriteString("Chi parla: " + r.Parlante + "\n\n")
	}
	b.WriteString("Testo da tradurre:\n" + r.Testo)
	return []messaggio{{"system", promptSistema}, {"user", b.String()}}
}

func costruisciCorrezione(r RichiestaTraduzione) []messaggio {
	var b strings.Builder
	if len(r.Glossario) > 0 {
		b.WriteString("Glossario da rispettare:\n")
		for _, v := range r.Glossario {
			b.WriteString("- " + v.Inglese + " = " + v.Italiano + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("Inglese:\n" + r.Testo + "\n\nTraduzione automatica:\n" + r.Bozza)
	return []messaggio{{"system", promptCorrettore}, {"user", b.String()}}
}

// correzioneAccettabile: tiene la correzione dell'IA solo se non sembra un'invenzione.
// Se l'IA allunga o accorcia troppo, cambia i numeri o lascia l'inglese, resta la traduzione di Mozilla.
func correzioneAccettabile(en, bozza, corretta string) bool {
	if strings.TrimSpace(corretta) == "" {
		return false
	}
	if sembraStrana(en, corretta) != "" && sembraStrana(en, bozza) == "" {
		return false
	}
	if copiaInglese(en, bozza, corretta) {
		return false
	}
	if ripetitiva(corretta) && !ripetitiva(bozza) { // l'IA si è "incantata" (idea da Game Overlay Translator)
		return false
	}
	lb, lc := len([]rune(bozza)), len([]rune(corretta))
	if lb > 0 && (float64(lc) > float64(lb)*1.6+10 || float64(lc) < float64(lb)*0.55-5) {
		return false
	}
	return true
}

// ripetitiva: la stessa parola (di almeno 3 lettere) 4 volte o più, o lo stesso pezzo ripetuto.
func ripetitiva(s string) bool {
	conta := map[string]int{}
	for _, w := range strings.Fields(strings.ToLower(s)) {
		w = strings.Trim(w, ".,!?;:\"'()«»…-")
		if len([]rune(w)) >= 3 {
			conta[w]++
			if conta[w] >= 4 {
				return true
			}
		}
	}
	return false
}

// copiaInglese: la correzione ha ricopiato parole dell'inglese che Mozilla aveva tradotto
// (es. "Final whistle! Siamo in finale!"). Nomi propri e numeri non contano.
func copiaInglese(en, bozza, corretta string) bool {
	parole := func(s string) map[string]bool {
		m := map[string]bool{}
		for _, w := range strings.Fields(s) {
			w = strings.ToLower(strings.Trim(w, ".,!?;:\"'()«»…-"))
			if len([]rune(w)) >= 3 {
				m[w] = true
			}
		}
		return m
	}
	pe, pb := parole(en), parole(bozza)
	for w := range parole(corretta) {
		if pe[w] && !pb[w] {
			return true
		}
	}
	return false
}

// pulisciRisposta toglie quello che a volte i modelli aggiungono (virgolette, "Traduzione:", ragionamenti).
func pulisciRisposta(s string) string {
	// alcuni modelli scrivono un ragionamento tra <think> e </think>
	if i := strings.Index(s, "</think>"); i >= 0 {
		s = s[i+len("</think>"):]
	}
	s = strings.TrimSpace(s)
	for _, pre := range []string{"Traduzione:", "Traduzione italiana:", "Italiano:"} {
		if strings.HasPrefix(strings.ToLower(s), strings.ToLower(pre)) {
			s = strings.TrimSpace(s[len(pre):])
		}
	}
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"') {
		s = s[1 : len(s)-1]
	}
	if strings.HasPrefix(s, "«") && strings.HasSuffix(s, "»") {
		s = strings.TrimSuffix(strings.TrimPrefix(s, "«"), "»")
	}
	return strings.TrimSpace(s)
}

// temperatura: 0 nel modo correttore (sempre la stessa correzione per la stessa frase, niente
// fantasia), un po' di più quando traduce da zero.
func temperatura(r RichiestaTraduzione) float64 {
	if r.Bozza != "" {
		return 0
	}
	return 0.2
}

// ClientIA parla con llama-server (interfaccia compatibile OpenAI) su 127.0.0.1.
type ClientIA struct {
	URL    string // es. http://127.0.0.1:41234
	HTTP   *http.Client
	Chiave string // chiave segreta della sessione (Authorization: Bearer)
}

func (c *ClientIA) Traduci(ctx context.Context, r RichiestaTraduzione) (string, error) {
	return c.Chat(ctx, costruisciMessaggi(r), temperatura(r))
}

// Chat: messaggi già pronti -> risposta pulita.
func (c *ClientIA) Chat(ctx context.Context, msgs []messaggio, temp float64) (string, error) {
	corpo, _ := json.Marshal(map[string]interface{}{
		"messages":    msgs,
		"temperature": temp,
		"top_p":       0.9,
		"max_tokens":  400,
		"stream":      false,
		// niente ragionamenti lunghi: serve velocità
		"chat_template_kwargs": map[string]interface{}{"enable_thinking": false},
	})
	req, err := http.NewRequestWithContext(ctx, "POST", c.URL+"/v1/chat/completions", bytes.NewReader(corpo))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Chiave != "" {
		req.Header.Set("Authorization", "Bearer "+c.Chiave)
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	dati, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("motore IA: risposta %d", resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message messaggio `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(dati, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", errors.New("motore IA: risposta vuota")
	}
	t := pulisciRisposta(out.Choices[0].Message.Content)
	if t == "" {
		return "", errors.New("motore IA: traduzione vuota")
	}
	return t, nil
}

// Pronto controlla se llama-server ha finito di caricare il modello.
func (c *ClientIA) Pronto(ctx context.Context) bool {
	req, _ := http.NewRequestWithContext(ctx, "GET", c.URL+"/health", nil)
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

// ---- correttore per le altre lingue (EuroLLM conosce 35 lingue: tutte quelle dei giochi)

var nomiInglesi = map[string]string{
	"en": "English", "it": "Italian", "fr": "French", "de": "German", "es": "Spanish", "pt": "Portuguese",
	"pl": "Polish", "ru": "Russian", "ja": "Japanese", "ko": "Korean", "zh": "Simplified Chinese",
	"zt": "Traditional Chinese", "da": "Danish", "fi": "Finnish", "nb": "Norwegian Bokmål", "sv": "Swedish", "tr": "Turkish",
}

// promptCorrettoreCoppia: le stesse regole del correttore italiano, per una coppia qualsiasi.
func promptCorrettoreCoppia(da, a string) string {
	src, dst := nomiInglesi[da], nomiInglesi[a]
	return "You are a professional video game localizer translating from " + src + " into " + dst + ".\n" +
		"You receive a game text in " + src + " and a machine translation into " + dst + " made by a program.\n" +
		"Your task: return the BEST " + dst + " translation, as in a well localized game.\n" +
		"- If the machine translation is already correct and natural, repeat it unchanged.\n" +
		"- If it gets the meaning wrong, uses the wrong word for the context, has grammar or agreement errors, or sounds unnatural, fix it.\n" +
		"- Render idioms with the equivalent " + dst + " idiom, not word for word.\n" +
		"- Keep the tone and register of the character. Menus and commands: short.\n" +
		"- Do not add or remove information. Numbers, proper names and keys stay the same.\n" +
		"Reply ONLY with the final " + dst + " translation."
}

func costruisciCorrezioneCoppia(testo, bozza, da, a string) []messaggio {
	return []messaggio{{"system", promptCorrettoreCoppia(da, a)}, {"user", nomiInglesi[da] + ":\n" + testo + "\n\nMachine translation (" + nomiInglesi[a] + "):\n" + bozza}}
}

// correzioneAccettabileCoppia: come per l'italiano, senza i controlli fatti apposta per l'italiano.
func correzioneAccettabileCoppia(src, bozza, corretta string) bool {
	if strings.TrimSpace(corretta) == "" || strings.Contains(corretta, "\n\n") {
		return false
	}
	if copiaInglese(src, bozza, corretta) { // parole della lingua di partenza ricopiate
		return false
	}
	if ripetitiva(corretta) && !ripetitiva(bozza) {
		return false
	}
	if numeri(corretta) != numeri(bozza) && numeri(corretta) != numeri(src) {
		return false
	}
	lb, lc := len([]rune(bozza)), len([]rune(corretta))
	if lb > 0 && (float64(lc) > float64(lb)*1.6+10 || float64(lc) < float64(lb)*0.55-5) {
		return false
	}
	return true
}

func numeri(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
