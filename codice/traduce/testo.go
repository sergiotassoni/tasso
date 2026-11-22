package traduce

// Pulizia del testo inglese letto dallo schermo, prima di tradurlo.
// Non dipende da Windows: si prova con "go test" anche su altri sistemi.

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var (
	reBarraI     = regexp.MustCompile(`(^|[^A-Za-z])\|([^A-Za-z]|$)`) // "| am" -> "I am"
	reSpazioPunt = regexp.MustCompile(`\s+([,.!?;:])`)                // "ciao ," -> "ciao,"
	reSpazi      = regexp.MustCompile(`\s{2,}`)
	reParlante   = regexp.MustCompile(`^\s*([A-Z][A-Za-z0-9' .\-]{1,24}):\s*(.+)$`)
	// cifre in mezzo a una parola minuscola: errori tipici dell'OCR (g0od, he1lo, ca5e)
	reCifraInParola = regexp.MustCompile(`[a-z][015][a-z]`)
)

var cifraLettera = map[byte]byte{'0': 'o', '1': 'l', '5': 's'}

// pulisci corregge gli errori tipici della lettura e riunisce le righe della stessa frase.
func pulisci(testo string) string {
	var righe []string
	for _, r := range strings.Split(strings.ReplaceAll(testo, "\r", ""), "\n") {
		r = strings.TrimSpace(r)
		if r != "" && sembraTesto(r) {
			righe = append(righe, r)
		}
	}
	if len(righe) == 0 {
		return ""
	}
	unite := []string{righe[0]}
	for _, r := range righe[1:] {
		prec := unite[len(unite)-1]
		primaMinuscola := unicode.IsLower(rune(r[0]))
		switch {
		case strings.HasSuffix(prec, "-") && primaMinuscola:
			unite[len(unite)-1] = prec[:len(prec)-1] + r // parola andata a capo
		case !finisceFrase(prec) || primaMinuscola:
			unite[len(unite)-1] = prec + " " + r // stessa frase su due righe
		default:
			unite = append(unite, r)
		}
	}
	for i, r := range unite {
		r = strings.NewReplacer("‘", "'", "’", "'", "`", "'", "´", "'", "“", `"`, "”", `"`).Replace(r)
		r = reBarraI.ReplaceAllString(r, "${1}I${2}")
		r = reCifraInParola.ReplaceAllStringFunc(r, func(m string) string {
			return string([]byte{m[0], cifraLettera[m[1]], m[2]})
		})
		r = reSpazioPunt.ReplaceAllString(r, "$1")
		r = correggiOCRInglese(r)
		r = correggiNumeriOCR(r)
		r = correggiParole(r)
		r = reSpazi.ReplaceAllString(r, " ")
		unite[i] = strings.TrimSpace(r)
	}
	return strings.Join(unite, "\n")
}

func finisceFrase(s string) bool {
	return strings.ContainsAny(s[len(s)-1:], `.!?:"')]…»`) || strings.HasSuffix(s, "…")
}

// sembraTesto scarta il rumore: righe senza lettere o fatte quasi solo di simboli.
func sembraTesto(r string) bool {
	lettere, tot := 0, 0
	for _, c := range r {
		tot++
		if unicode.IsLetter(c) {
			lettere++
		}
	}
	return lettere >= 2 && float64(lettere)/float64(tot) >= 0.4
}

// etichette dei menu che sembrano nomi ma vanno tradotte ("Warning: ...", "Quest updated: ...")
var etichetteUI = map[string]bool{
	"warning": true, "quest": true, "achievement": true, "objective": true, "note": true, "tip": true,
	"hint": true, "error": true, "level": true, "day": true, "new": true, "research": true, "mission": true,
	"tutorial": true, "status": true, "reward": true, "rewards": true, "chapter": true, "goal": true,
}

// puoEssereNome: "Captain Price" sì; "Skill points available", "Time trial", "Cooldown" no
// (scritte di menu: parole minuscole dopo la prima, oppure termini del glossario da gioco).
func puoEssereNome(s string) bool {
	w := strings.Fields(strings.TrimSpace(s))
	if len(w) == 0 {
		return false
	}
	for _, x := range w[1:] {
		r := []rune(x)
		if unicode.IsLower(r[0]) && !paroleNome[strings.ToLower(x)] {
			return false
		}
	}
	caricaCataloghi()
	nome := strings.Join(w, " ")
	for en := range termGiochi {
		if strings.EqualFold(en, nome) {
			return false
		}
	}
	return true
}

// parole minuscole che possono stare dentro un nome ("Ser Davos of Seaworth", "Ludwig van Beethoven")
var paroleNome = map[string]bool{"of": true, "the": true, "van": true, "von": true, "de": true, "da": true, "del": true, "di": true, "la": true, "le": true, "al": true, "bin": true}

// separaParlante: "Captain Price: Move out!" -> ("Captain Price", "Move out!").
// Il nome di chi parla non si traduce.
func separaParlante(testo string) (string, string) {
	if strings.Contains(testo, "\n") {
		prima, resto, _ := strings.Cut(testo, "\n")
		if n, b := separaParlante(prima); n != "" {
			return n, strings.TrimSpace(b + "\n" + resto)
		}
		return "", testo
	}
	m := reParlante.FindStringSubmatch(testo)
	if m != nil && len(strings.Fields(m[1])) <= 3 && !etichetteUI[strings.ToLower(strings.Fields(m[1])[0])] && puoEssereNome(m[1]) {
		return strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
	}
	return "", testo
}

// ---------------------------------------------------------------- glossario

// Glossario: termini con traduzione imposta e nomi da lasciare così come sono.
type Glossario struct {
	NonTradurre []string          `json:"non_tradurre"`
	Termini     map[string]string `json:"termini"`
}

func glossarioBase() Glossario {
	return Glossario{
		NonTradurre: []string{},
		Termini: map[string]string{
			"HP": "PV", "MP": "PM", "XP": "PE",
			"Save Game": "Salva partita", "Load Game": "Carica partita",
			"Quest Log": "Diario delle missioni", "Side Quest": "Missione secondaria",
			"Main Quest": "Missione principale", "Inventory": "Inventario",
			"Skill Tree": "Albero delle abilità", "Level Up": "Aumento di livello",
			"Autosave": "Salvataggio automatico", "Checkpoint": "Checkpoint",
		},
	}
}

// voce di glossario trovata in un testo
type voceTrovata struct{ Inglese, Italiano string }

// trova restituisce le voci del glossario presenti nel testo (le più lunghe prima).
func (g Glossario) trova(testo string) []voceTrovata {
	voci := map[string]string{}
	for _, n := range g.NonTradurre {
		voci[n] = n
	}
	for k, v := range g.Termini {
		voci[k] = v
	}
	chiavi := make([]string, 0, len(voci))
	for k := range voci {
		chiavi = append(chiavi, k)
	}
	sort.Slice(chiavi, func(i, j int) bool { return len(chiavi[i]) > len(chiavi[j]) })
	var out []voceTrovata
	basso := strings.ToLower(testo)
	preso := make([]bool, len(basso))
	for _, k := range chiavi {
		kb := strings.ToLower(k)
		for i := 0; ; {
			j := strings.Index(basso[i:], kb)
			if j < 0 {
				break
			}
			a, b := i+j, i+j+len(kb)
			if confine(basso, a-1) && confine(basso, b) && !preso[a] {
				for x := a; x < b; x++ {
					preso[x] = true
				}
				out = append(out, voceTrovata{k, voci[k]})
				break
			}
			i = a + 1
		}
	}
	return out
}

func confine(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return true
	}
	c := rune(s[i])
	return !unicode.IsLetter(c) && !unicode.IsDigit(c)
}
