//go:build collaudo && windows

package traduce

// Prove di qualità del correttore IA, su METÀ delle 1000 frasi (id dispari = "sviluppo").
// L'altra metà (id pari = "verifica") si usa una volta sola alla fine, per il numero vero.
//   BASE=collaudo.json (uscite di Mozilla dal vivo)  MODELLO=file.gguf  MOTORE=cartella llama
//   VARIANTI=v0,v1,...  META=sviluppo|verifica  NGL=99
//   go test -tags collaudo -run TestQualita -timeout 3h -v .

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type varianteIA struct {
	nome     string
	sistema  string
	utente   func(en, bozza string, g []voceTrovata, prec []string) string
	contesto bool
}

func glossarioTesto(g []voceTrovata) string {
	if len(g) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Glossario da rispettare:\n")
	for _, v := range g {
		b.WriteString("- " + v.Inglese + " = " + v.Italiano + "\n")
	}
	return b.String() + "\n"
}

const sistemaV1 = `Sei un localizzatore professionista di videogiochi dall'inglese all'italiano.
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


// 24/11: nuove prove per "più intelligenza" (voce 6 della lista).
const sistemaV7 = `You are a professional video game localizer from English to Italian.
You get an English line from a game and a machine translation into Italian.
Return the BEST Italian translation, as in a well localized game.
- If the machine translation is already correct and natural, copy it unchanged.
- Fix wrong meaning, wrong word for the game context, grammar, gender or agreement errors, unnatural phrasing.
- Idioms: use the equivalent Italian idiom, not word for word.
- Keep the character's tone and register. Address the player with "tu".
- Menus and commands: short, imperative ("Premi", "Tieni premuto").
- Do not add or remove information. Numbers, proper names and keys stay the same.
- Use the glossary translations when given.
Answer ONLY with the final Italian translation.`

const esempiFissi = `Esempi di correzioni giuste:
Inglese: Reload your magazine.
Traduzione automatica: Ricarica la tua rivista.
Finale: Ricarica il caricatore.
Inglese: Press E to loot the body.
Traduzione automatica: Premere E per saccheggiare il corpo.
Finale: Premi E per perquisire il corpo.
Inglese: Low health!
Traduzione automatica: Salute bassa!
Finale: Salute bassa!
Inglese: You've got some nerve coming back here.
Traduzione automatica: Hai un po' di nervi a tornare qui.
Finale: Hai un bel coraggio a tornare qui.

`

const sistemaV9 = promptCorrettore + `
Cambia il meno possibile: correggi solo le parole sbagliate e lascia uguale il resto della frase (stesse parole, stesso ordine, stessa punteggiatura).`

var variantiIA = map[string]varianteIA{
	"v7": {nome: "istruzioni in inglese", sistema: sistemaV7, utente: func(en, bozza string, g []voceTrovata, _ []string) string {
		return glossarioTesto(g) + "Inglese:\n" + en + "\n\nTraduzione automatica:\n" + bozza
	}},
	"v8": {nome: "prompt di oggi + 4 esempi fissi di errori tipici", sistema: promptCorrettore, utente: func(en, bozza string, g []voceTrovata, _ []string) string {
		return esempiFissi + glossarioTesto(g) + "Inglese:\n" + en + "\n\nTraduzione automatica:\n" + bozza
	}},
	"v9": {nome: "prompt di oggi + cambia il meno possibile", sistema: sistemaV9, utente: func(en, bozza string, g []voceTrovata, _ []string) string {
		return glossarioTesto(g) + "Inglese:\n" + en + "\n\nTraduzione automatica:\n" + bozza
	}},

	"v0": {nome: "prompt di oggi", sistema: promptCorrettore, utente: func(en, bozza string, g []voceTrovata, _ []string) string {
		return glossarioTesto(g) + "Inglese:\n" + en + "\n\nTraduzione automatica:\n" + bozza
	}},
	"v1": {nome: "prompt da localizzatore", sistema: sistemaV1, utente: func(en, bozza string, g []voceTrovata, _ []string) string {
		return glossarioTesto(g) + "Inglese:\n" + en + "\n\nTraduzione automatica:\n" + bozza
	}},
	"v3": {nome: "traduci tu, bozza solo come aiuto", sistema: promptSistema, utente: func(en, bozza string, g []voceTrovata, _ []string) string {
		return glossarioTesto(g) + "Testo da tradurre:\n" + en + "\n\n(Una traduzione automatica, che può contenere errori: " + bozza + ")"
	}},
	"v5": {nome: "prompt di oggi + 3 esempi simili dalla libreria", sistema: promptCorrettore, utente: func(en, bozza string, g []voceTrovata, _ []string) string {
		return testoEsempi(esempiSimili(en, 3)) + glossarioTesto(g) + "Inglese:\n" + en + "\n\nTraduzione automatica:\n" + bozza
	}},
	"v6": {nome: "prompt di oggi + 2 esempi simili dalla libreria", sistema: promptCorrettore, utente: func(en, bozza string, g []voceTrovata, _ []string) string {
		return testoEsempi(esempiSimili(en, 2)) + glossarioTesto(g) + "Inglese:\n" + en + "\n\nTraduzione automatica:\n" + bozza
	}},
	"v2": {nome: "localizzatore + battute precedenti", sistema: sistemaV1, contesto: true, utente: func(en, bozza string, g []voceTrovata, prec []string) string {
		var b strings.Builder
		if len(prec) > 0 {
			b.WriteString("Battute precedenti dello stesso gioco (solo per capire il contesto, NON tradurle):\n")
			for _, p := range prec {
				b.WriteString("- " + p + "\n")
			}
			b.WriteString("\n")
		}
		return b.String() + glossarioTesto(g) + "Inglese:\n" + en + "\n\nTraduzione automatica:\n" + bozza
	}},
}

func TestQualita(t *testing.T) {
	base, modello, motore := os.Getenv("BASE"), os.Getenv("MODELLO"), os.Getenv("MOTORE")
	if base == "" || modello == "" {
		t.Skip()
	}
	dirApp = t.TempDir()
	_ = os.MkdirAll(filepath.Join(dirApp, "logs"), 0o755)
	b, err := os.ReadFile(base)
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Mozilla EsitoProva `json:"mozilla"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	pari := os.Getenv("META") == "verifica"
	var frasi []EsitoFrase
	for _, f := range c.Mozilla.Frasi {
		if (f.ID%2 == 0) == pari {
			frasi = append(frasi, f)
		}
	}
	s := &ServerIA{Contesto: 2048}
	if os.Getenv("NGL") != "" {
		fmt.Sscan(os.Getenv("NGL"), &s.Strati)
	}
	if err := s.Avvia(motore, modello); err != nil {
		t.Fatal(err)
	}
	defer s.Ferma()
	g := glossarioBase().conGiochi()
	for _, nomeV := range strings.Split(os.Getenv("VARIANTI"), ",") {
		v, ok := variantiIA[nomeV]
		if !ok {
			continue
		}
		var hypM, hypC, ref []string
		var tempi time.Duration
		cambiate, meglio, peggio := 0, 0, 0
		prec := map[string][]string{}
		for _, f := range frasi {
			parlante, battuta := separaParlante(pulisci(f.EN))
			bozza := strings.TrimSpace(strings.TrimPrefix(f.Uscita, parlante+":"))
			vg := g.trova(battuta)
			msgs := []messaggio{{"system", v.sistema}, {"user", v.utente(battuta, bozza, vg, prec[f.Genere])}}
			t0 := time.Now()
			out, err := s.chat(context.Background(), msgs)
			tempi += time.Since(t0)
			finale := bozza
			if err == nil {
				out = sigleItaliane(correggiTerminiFrase(battuta, out), vg)
				if correzioneAccettabile(battuta, bozza, out) {
					finale = out
				}
			}
			if v.contesto {
				p := append(prec[f.Genere], battuta)
				if len(p) > 3 {
					p = p[len(p)-3:]
				}
				prec[f.Genere] = p
			}
			if parlante != "" {
				finale = parlante + ": " + finale
			}
			hypM, hypC, ref = append(hypM, f.Uscita), append(hypC, finale), append(ref, f.IT)
			if finale != f.Uscita {
				cambiate++
				a, b := chrFpp([]string{f.Uscita}, []string{f.IT}), chrFpp([]string{finale}, []string{f.IT})
				if b > a {
					meglio++
				} else if b < a {
					peggio++
				}
			}
		}
		t.Logf("%s [%s] %s: Mozilla %.2f -> %.2f · cambiate %d (meglio %d, peggio %d) · %.0f ms a frase",
			filepath.Base(modello), nomeV, v.nome, chrFpp(hypM, ref), chrFpp(hypC, ref), cambiate, meglio, peggio,
			float64(tempi.Milliseconds())/float64(len(frasi)))
	}
}

func (s *ServerIA) chat(ctx context.Context, msgs []messaggio) (string, error) {
	s.mu.Lock()
	c := s.Client
	s.mu.Unlock()
	if c == nil {
		return "", fmt.Errorf("spento")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return c.Chat(ctx, msgs, 0)
}
