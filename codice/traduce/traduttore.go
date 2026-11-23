package traduce

// Sceglie il motore, ricorda il contesto del dialogo, tiene la cache delle frasi già tradotte.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
)

type Motore interface {
	Nome() string
	Traduci(ctx context.Context, r RichiestaTraduzione) (string, error)
}

type Traduttore struct {
	mu             sync.Mutex
	motori         []Motore // in ordine di preferenza: IA, poi Mozilla
	glossario      Glossario
	glossarioPieno Glossario // utente + glossario da gioco
	contesto       []string
	cache          map[string]string
	fileCache      string
	libreria       map[string]string // traduzioni del gioco aperto, tenute per sempre
	fileLibreria   string
	dubbi          map[string]Dubbio // traduzioni che sembrano strane, da guardare insieme
	corr           correttoreSfondo  // IA che ricontrolla le frasi in sottofondo
	// Coppia: lingua del gioco e lingua di arrivo scelte ("en","it" se nil). Per inglese →
	// italiano c'è tutto (glossario, libreria, correttore IA); per le altre coppie Mozilla,
	// passando dall'inglese se serve.
	Coppia      func() (string, string)
	altreCoppie map[string]string
}

// MotoreCoppie: un motore che sa tradurre fra coppie di lingue qualsiasi (Mozilla).
type MotoreCoppie interface {
	TraduciCoppia(ctx context.Context, testo, da, a string) (string, error)
}

const maxContesto = 4

func NuovoTraduttore(fileCache string, g Glossario, motori ...Motore) *Traduttore {
	t := &Traduttore{motori: motori, glossario: g, glossarioPieno: g.conGiochi(), cache: map[string]string{}, fileCache: fileCache}
	if fileCache != "" {
		if b, err := os.ReadFile(fileCache); err == nil {
			_ = json.Unmarshal(b, &t.cache)
		}
	}
	return t
}

func (t *Traduttore) ImpostaGlossario(g Glossario) {
	t.mu.Lock()
	t.glossario = g
	t.glossarioPieno = g.conGiochi()
	t.cache = map[string]string{} // le traduzioni vecchie potrebbero non rispettare il glossario nuovo
	t.mu.Unlock()
	t.salvaCache()
}

func (t *Traduttore) ImpostaMotori(m ...Motore) {
	t.mu.Lock()
	t.motori = m
	t.mu.Unlock()
}

// Risultato di una traduzione, pronto per la finestrella.
type Risultato struct {
	Originale string `json:"originale"`
	Parlante  string `json:"parlante"`
	Tradotto  string `json:"tradotto"`
	Motore    string `json:"motore"`
	Millisec  int64  `json:"ms"`
	Errore    string `json:"errore,omitempty"`
}

// Traduci prende il testo letto dallo schermo (grezzo) e lo traduce.
func (t *Traduttore) Traduci(ctx context.Context, grezzo string) Risultato {
	return t.traduci(ctx, grezzo, true)
}

// TraduciScritta: una scritta di un menu o di una schermata (modalità F6). Niente nome di chi
// parla, niente contesto del dialogo, memoria salvata su disco solo alla fine (SalvaMemoria).
func (t *Traduttore) TraduciScritta(ctx context.Context, grezzo string) Risultato {
	return t.traduci(ctx, grezzo, false)
}

func (t *Traduttore) SalvaMemoria() { t.salvaCache(); t.salvaLibreria() }

// TraduciMessaggio: un messaggio della chat degli altri giocatori (3.1). Niente contesto dei
// dialoghi, niente libreria del gioco, niente memoria su disco, niente correzioni IA: i messaggi
// dei giocatori non si salvano e non confondono le traduzioni dei dialoghi.
func (t *Traduttore) TraduciMessaggio(ctx context.Context, grezzo string) Risultato {
	inizio := time.Now()
	testo := pulisci(grezzo)
	if testo == "" {
		return Risultato{}
	}
	parlante, battuta := separaParlante(testo)
	battuta = strings.ReplaceAll(battuta, "\n", " ")
	ris := Risultato{Originale: testo, Parlante: parlante}
	if soloSigle(battuta) { // "gg wp", "afk": i giocatori italiani le scrivono uguali
		ris.Tradotto, ris.Motore = battuta, "catalogo"
		ris.Millisec = time.Since(inizio).Milliseconds()
		return ris
	}
	if v, ok := fraseMenuConNumero(battuta); ok { // frasi fatte ("Good game", "Nice shot")
		ris.Tradotto, ris.Motore = v, "catalogo"
		ris.Millisec = time.Since(inizio).Milliseconds()
		return ris
	}
	if t.Coppia != nil {
		if da, a := t.Coppia(); !(da == "en" && a == "it") {
			r := t.traduciCoppia(ctx, battuta, da, a, inizio)
			r.Originale, r.Parlante = testo, parlante
			return r
		}
	}
	t.mu.Lock()
	motori := append([]Motore(nil), t.motori...)
	battuta = espandiAbbreviazioni(battuta) // "u r late" → "you are late": il traduttore capisce meglio
	req := RichiestaTraduzione{Testo: battuta, Parlante: parlante, Glossario: t.glossarioPieno.trova(battuta)}
	t.mu.Unlock()
	var errori []string
	for _, m := range motori {
		out, err := m.Traduci(ctx, req)
		if err == nil && strings.TrimSpace(out) != "" {
			ris.Tradotto, ris.Motore = strings.TrimSpace(out), m.Nome()
			break
		}
		if err != nil {
			errori = append(errori, m.Nome()+": "+err.Error())
		}
	}
	ris.Millisec = time.Since(inizio).Milliseconds()
	if ris.Tradotto == "" {
		ris.Errore = strings.Join(errori, " · ")
	}
	return ris
}

func (t *Traduttore) traduci(ctx context.Context, grezzo string, dialogo bool) Risultato {
	da, a := "en", "it"
	if t.Coppia != nil {
		da, a = t.Coppia()
	}
	return t.traduciIn(ctx, grezzo, dialogo, da, a)
}

// TraduciPer: come Traduci (dialogo) o TraduciScritta, ma nella coppia di lingue data invece
// di quella scelta adesso (la traduzione dentro un gioco usa le lingue con cui l'hai installata).
func (t *Traduttore) TraduciPer(ctx context.Context, grezzo string, dialogo bool, da, a string) Risultato {
	return t.traduciIn(ctx, grezzo, dialogo, da, a)
}

func (t *Traduttore) traduciIn(ctx context.Context, grezzo string, dialogo bool, da, a string) Risultato {
	inizio := time.Now()
	if !(da == "en" && a == "it") {
		return t.traduciCoppia(ctx, grezzo, da, a, inizio)
	}
	testo := pulisci(grezzo)
	if testo == "" {
		return Risultato{}
	}
	parlante, battuta := "", strings.ReplaceAll(testo, "\n", " ")
	if dialogo {
		parlante, battuta = separaParlante(testo)
	} else if r, ok := t.traduciEtichetta(ctx, battuta, inizio); ok {
		return r
	}
	ricorda := func(b string) {
		if dialogo {
			t.ricorda(b)
		}
	}
	ris := Risultato{Originale: testo, Parlante: parlante}

	t.mu.Lock()
	chiave := parlante + "\x00" + battuta
	// prima le frasi curate (catalogo e traduzioni umane): un catalogo nuovo vince sulle
	// vecchie traduzioni automatiche rimaste in memoria. Solo se nessuna parola del glossario
	// dell'utente compare nella frase (lì decide lui).
	// Le frasi con parole fisse dell'utente non escono dalla libreria del gioco: le parole fisse
	// possono essere cambiate dopo, e decide la memoria (svuotata a ogni cambio delle parole fisse).
	conParoleFisse := len(t.glossario.trova(battuta)) > 0
	if !conParoleFisse {
		if v, ok := fraseMenuConNumero(battuta); ok {
			t.mu.Unlock()
			ris.Tradotto, ris.Motore = v, "catalogo"
			ris.Millisec = time.Since(inizio).Milliseconds()
			ricorda(battuta)
			return ris
		}
		if v, ok := fraseLiberaEsatta(battuta); ok {
			t.mu.Unlock()
			ris.Tradotto, ris.Motore = v, "libreria giochi liberi"
			ris.Millisec = time.Since(inizio).Milliseconds()
			ricorda(battuta)
			return ris
		}
	}
	if v, ok := t.libreria[chiave]; ok && !conParoleFisse {
		v = t.giaSalvataLocked(chiave, battuta, v)
		t.mu.Unlock()
		t.ricontrolla(ctx, chiave, battuta, v, dialogo)
		ris.Tradotto, ris.Motore = v, "libreria"
		ris.Millisec = time.Since(inizio).Milliseconds()
		ricorda(battuta)
		return ris
	}
	if v, ok := t.cache[chiave]; ok {
		v = t.giaSalvataLocked(chiave, battuta, v)
		if t.libreria != nil {
			t.libreria[chiave] = v
		}
		t.mu.Unlock()
		if !conParoleFisse {
			t.ricontrolla(ctx, chiave, battuta, v, dialogo)
		}
		ris.Tradotto, ris.Motore = v, "memoria"
		ris.Millisec = time.Since(inizio).Milliseconds()
		ricorda(battuta)
		return ris
	}
	if !conParoleFisse {
		if v, ok := t.cercaQuasiUguale(parlante, battuta); ok {
			t.mu.Unlock()
			ris.Tradotto, ris.Motore = nomiIntatti(battuta, v), "libreria"
			ris.Millisec = time.Since(inizio).Milliseconds()
			ricorda(battuta)
			return ris
		}
	}
	motori := append([]Motore(nil), t.motori...)
	req := RichiestaTraduzione{
		Testo:     battuta,
		Parlante:  parlante,
		Contesto:  contestoSe(dialogo, t.contesto),
		Schermata: schermataDa(ctx, dialogo),
		Glossario: t.glossarioPieno.trova(battuta),
	}
	t.mu.Unlock()

	var errori []string
	for _, m := range motori {
		var out string
		var err error
		if m.Nome() == "Mozilla" {
			if bloccato, mappa := bloccaTermini(req.Testo, req.Glossario); mappa != nil {
				r2 := req
				r2.Testo = bloccato
				if o, e := m.Traduci(ctx, r2); e == nil {
					if s, ok := sbloccaTermini(o, mappa); ok {
						out = s
					}
				}
			}
		}
		if out == "" {
			out, err = m.Traduci(ctx, req)
		}
		if err == nil && strings.TrimSpace(out) != "" {
			ris.Tradotto, ris.Motore = nomiIntatti(battuta, sigleItaliane(correggiTerminiFrase(battuta, strings.TrimSpace(out)), req.Glossario)), m.Nome()
			break
		}
		if err != nil {
			errori = append(errori, m.Nome()+": "+err.Error())
		}
	}
	ris.Millisec = time.Since(inizio).Milliseconds()
	if ris.Tradotto == "" {
		ris.Errore = strings.Join(errori, " · ")
		return ris
	}
	t.mu.Lock()
	t.cache[chiave] = ris.Tradotto
	if t.libreria != nil {
		t.libreria[chiave] = ris.Tradotto
		if perche := sembraStrana(battuta, ris.Tradotto); perche != "" {
			if t.dubbi == nil {
				t.dubbi = map[string]Dubbio{}
			}
			t.dubbi[chiave] = Dubbio{Inglese: battuta, Italiano: ris.Tradotto, Motore: ris.Motore, Perche: perche}
		}
	}
	t.mu.Unlock()
	if ris.Motore == "Mozilla" {
		t.accodaCorrezione(chiave, battuta, ris.Tradotto, req.Glossario, req.Schermata)
	}
	ricorda(battuta)
	if dialogo {
		t.salvaCache()
	}
	return ris
}

// traduciEtichetta: scritte dei menu fatte a pezzi ("- Bleed Resist", "Mating Status: Error",
// "HEALTH"). Ogni pezzo passa dal glossario, se c'è, altrimenti dal motore: così il motore veloce
// non si confonde con frasi che non sono frasi.
var reAumento = regexp.MustCompile(`^(\p{L}[\p{L} ]*?)\s+(?i:(increased|decreased|reduced|boosted))(?:\s+by)?\s+(\d+(?:[.,]\d+)?)\s?%$`)

var reQuantita = regexp.MustCompile(`[^\s](\s+(?:[x×]\s?\d+|\(\d+\)|\d+(?:/\d+)?))$`)

func (t *Traduttore) traduciEtichetta(ctx context.Context, s string, inizio time.Time) (Risultato, bool) {
	ris := Risultato{Originale: s}
	finito := func(it, motore string) (Risultato, bool) {
		ris.Tradotto, ris.Motore, ris.Millisec = it, motore, time.Since(inizio).Milliseconds()
		return ris, true
	}
	if it, ok := t.termineEsatto(s); ok {
		return finito(comeMaiuscola(s, it), "glossario")
	}
	if it, ok := t.oggettoDiMateriale(s); ok {
		return finito(it, "glossario")
	}
	if w := strings.Fields(s); len(w) == 1 && !esiste(strings.Trim(w[0], ".,:;!?")) && unicode.IsUpper([]rune(w[0])[0]) {
		return finito(s, "nome") // nome proprio (luogo, creatura, personaggio): resta com'è
	}
	if len(strings.Fields(s)) > 8 {
		return ris, false
	}
	// statistiche: "Health Regen Increased 6.7%" -> "Rigenerazione della salute: +6,7%"
	if m := reAumento.FindStringSubmatch(s); m != nil {
		r := t.traduci(ctx, m[1], false)
		if r.Tradotto == "" {
			return ris, false
		}
		segno := "+"
		if strings.EqualFold(m[2], "decreased") || strings.EqualFold(m[2], "reduced") {
			segno = "-"
		}
		return finito(strings.TrimRight(r.Tradotto, ".:")+": "+segno+strings.ReplaceAll(m[3], ".", ",")+"%", r.Motore)
	}
	// quantità in fondo: "Health Potion x3", "Arrows (20)", "Gold 150" -> traduco solo il nome
	if m := reQuantita.FindStringSubmatchIndex(s); m != nil && m[2] > 0 {
		nome, q := s[:m[2]], s[m[2]:]
		r := t.traduci(ctx, strings.TrimSpace(nome), false)
		if r.Tradotto == "" {
			return ris, false
		}
		return finito(r.Tradotto+q, r.Motore)
	}
	for _, pre := range []string{"- ", "• ", "+ ", "# "} {
		if strings.HasPrefix(s, pre) && len(s) > len(pre)+1 {
			r := t.traduci(ctx, s[len(pre):], false)
			if r.Tradotto == "" {
				return ris, false
			}
			return finito(pre+r.Tradotto, r.Motore)
		}
	}
	if k, v, ok := strings.Cut(s, ": "); ok && !strings.Contains(v, ": ") && strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
		a := t.traduciElenco(ctx, k) // "Boar, Located" -> "Cinghiale, Si trova"
		b := t.traduciElenco(ctx, v)
		ta, tb := a.Tradotto, b.Tradotto
		if ta == "" {
			ta = k
		}
		if tb == "" {
			tb = v
		}
		return finito(ta+": "+tb, a.Motore)
	}
	return ris, false
}

// traduciElenco: "Highland, Grassland, Riverdelta" -> ogni voce per conto suo.
func (t *Traduttore) traduciElenco(ctx context.Context, v string) Risultato {
	voci := strings.Split(v, ", ")
	if len(voci) < 2 {
		return t.traduci(ctx, v, false)
	}
	for _, x := range voci {
		if len(strings.Fields(x)) > 3 {
			return t.traduci(ctx, v, false)
		}
	}
	var out []string
	motore := ""
	for _, x := range voci {
		r := t.traduci(ctx, x, false)
		if r.Tradotto == "" {
			r.Tradotto = x
		}
		out = append(out, r.Tradotto)
		if motore == "" {
			motore = r.Motore
		}
	}
	return Risultato{Originale: v, Tradotto: strings.Join(out, ", "), Motore: motore}
}

// termineEsatto: la scritta intera è una voce del glossario ("Inventory", "HEALTH", "Diet:").
func (t *Traduttore) termineEsatto(s string) (string, bool) {
	nuda := strings.TrimRightFunc(strings.TrimSpace(s), func(r rune) bool { return r == ':' || r == '.' || r == '!' })
	fine := strings.TrimSpace(s)[len(nuda):]
	t.mu.Lock()
	termini := t.glossarioPieno.Termini
	t.mu.Unlock()
	for en, it := range termini {
		if strings.EqualFold(en, nuda) {
			if nuda == strings.ToUpper(nuda) && len(nuda) > 3 {
				it = strings.ToUpper(it)
			}
			return it + fine, true
		}
	}
	return "", false
}

type chiaveSchermata struct{}

// ConSchermata: le scritte della schermata intera, da dare all'IA come contesto quando traduce
// una scritta corta di un menu ("Mouse" vicino a "Shift" e "Ctrl" è il mouse, non il topo).
func ConSchermata(ctx context.Context, scritte []string) context.Context {
	var l []string
	tot := 0
	for _, s := range scritte {
		s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
		if s == "" || len(s) > 80 {
			continue
		}
		if tot += len(s); tot > 500 || len(l) >= 14 {
			break
		}
		l = append(l, s)
	}
	if len(l) < 2 {
		return ctx
	}
	return context.WithValue(ctx, chiaveSchermata{}, l)
}

func schermataDa(ctx context.Context, dialogo bool) []string {
	if dialogo || ctx == nil {
		return nil
	}
	l, _ := ctx.Value(chiaveSchermata{}).([]string)
	return l
}

// giaSalvataLocked: la frase salvata con i nomi fissi rimessi a posto; se cambia, la salva così
// (la correzione dell'IA confronta la bozza con quello che è salvato). Da chiamare con t.mu preso.
func (t *Traduttore) giaSalvataLocked(chiave, battuta, v string) string {
	n := nomiIntatti(battuta, v)
	if n != v {
		if _, ok := t.cache[chiave]; ok {
			t.cache[chiave] = n
		}
		if t.libreria != nil {
			if _, ok := t.libreria[chiave]; ok {
				t.libreria[chiave] = n
			}
		}
	}
	return n
}

// ricontrolla: le frasi già salvate (anche da sessioni vecchie, magari solo con Mozilla) vengono
// ricontrollate dall'IA una volta per sessione, con le altre scritte della schermata come aiuto.
// Se l'IA trova di meglio, la scritta sullo schermo cambia subito (QuandoCorretta).
func (t *Traduttore) ricontrolla(ctx context.Context, chiave, battuta, v string, dialogo bool) {
	t.mu.Lock()
	g := t.glossarioPieno.trova(battuta)
	t.mu.Unlock()
	t.accodaCorrezione(chiave, battuta, v, g, schermataDa(ctx, dialogo))
}

// comeMaiuscola: una scritta da sola (titolo, voce di menu) che nel gioco inizia con la maiuscola
// la tiene anche in italiano ("Mouse" → "Mouse", non "mouse" come dentro una frase).
func comeMaiuscola(en, it string) string {
	re, ri := []rune(strings.TrimSpace(en)), []rune(it)
	if len(re) == 0 || len(ri) == 0 || !unicode.IsUpper(re[0]) || !unicode.IsLower(ri[0]) {
		return it
	}
	ri[0] = unicode.ToUpper(ri[0])
	return string(ri)
}

func contestoSe(si bool, c []string) []string {
	if !si {
		return nil
	}
	return append([]string(nil), c...)
}

func (t *Traduttore) ricorda(battuta string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if n := len(t.contesto); n > 0 && t.contesto[n-1] == battuta {
		return
	}
	t.contesto = append(t.contesto, battuta)
	if len(t.contesto) > maxContesto {
		t.contesto = t.contesto[len(t.contesto)-maxContesto:]
	}
}

var muFileCache sync.Mutex

// ApriLibreria: da qui in poi le traduzioni vanno anche nella libreria di questo gioco
// (un file per gioco). Quando lo riapri, le scritte già viste escono subito, sempre uguali.
func (t *Traduttore) ApriLibreria(file string) {
	t.mu.Lock()
	if file == t.fileLibreria {
		t.mu.Unlock()
		return
	}
	t.mu.Unlock()
	t.salvaLibreria()
	nuova := map[string]string{}
	if b, err := os.ReadFile(file); err == nil && len(b) < 64<<20 {
		_ = json.Unmarshal(b, &nuova)
	}
	dubbi := map[string]Dubbio{}
	if b, err := os.ReadFile(strings.TrimSuffix(file, ".json") + "-da-controllare.json"); err == nil && len(b) < 16<<20 {
		_ = json.Unmarshal(b, &dubbi)
	}
	t.mu.Lock()
	t.libreria, t.fileLibreria, t.dubbi = nuova, file, dubbi
	t.mu.Unlock()
}

func (t *Traduttore) salvaLibreria() {
	t.mu.Lock()
	file := t.fileLibreria
	if file == "" || len(t.libreria) == 0 {
		t.mu.Unlock()
		return
	}
	b, err := json.Marshal(t.libreria)
	var bd []byte
	if len(t.dubbi) > 0 {
		bd, _ = json.MarshalIndent(t.dubbi, "", " ")
	}
	t.mu.Unlock()
	if err != nil {
		return
	}
	if bd != nil {
		_ = os.MkdirAll(filepath.Dir(file), 0o755)
		dc := strings.TrimSuffix(file, ".json") + "-da-controllare.json"
		if os.WriteFile(dc+".tmp", bd, 0o644) == nil {
			_ = os.Rename(dc+".tmp", dc)
		}
	}
	muFileCache.Lock()
	defer muFileCache.Unlock()
	_ = os.MkdirAll(filepath.Dir(file), 0o755)
	tmp := file + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, file)
	}
}

// Dubbio: una traduzione da controllare a mano.
type Dubbio struct {
	Inglese  string `json:"inglese"`
	Italiano string `json:"italiano"`
	Motore   string `json:"motore"`
	Perche   string `json:"perche"`
}

// sembraStrana: perché una traduzione merita un'occhiata ("" se sembra a posto).
func sembraStrana(en, it string) string {
	pe, pi := strings.Fields(en), strings.Fields(it)
	if len(pe) == 0 || len(pi) == 0 {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(en), strings.TrimSpace(it)) && len(pe) > 1 {
		return "rimasta in inglese"
	}
	if len(pe) >= 4 {
		r := float64(len(pi)) / float64(len(pe))
		if r > 2.2 {
			return "molto più lunga dell'originale"
		}
		if r < 0.45 {
			return "molto più corta dell'originale"
		}
	}
	inglesi := 0
	for _, w := range pi {
		l := strings.ToLower(strings.Trim(w, ".,!?;:\"'()"))
		if paroleSoloInglesi[l] {
			inglesi++
		}
	}
	if inglesi >= 2 || (inglesi == 1 && len(pi) <= 3) {
		return "parole inglesi nella traduzione"
	}
	if strings.Contains(it, "Qzx") {
		return "segnaposto rimasto"
	}
	for _, n := range reNumero.FindAllString(en, -1) {
		if !strings.Contains(it, n) && !strings.Contains(it, strings.ReplaceAll(n, ".", ",")) {
			return "numero cambiato (" + n + ")"
		}
	}
	for _, k := range reTastoGioco.FindAllString(en, -1) {
		if !strings.Contains(it, k) {
			return "tasto perso (" + k + ")"
		}
	}
	return ""
}

var (
	reNumero     = regexp.MustCompile(`\d+(?:[.,]\d+)?`)
	reTastoGioco = regexp.MustCompile(`\[[^\]]{1,8}\]`)
)

// parole inglesi comuni che in una frase italiana non ci dovrebbero essere
var paroleSoloInglesi = map[string]bool{
	"the": true, "and": true, "you": true, "your": true, "with": true, "this": true, "that": true,
	"have": true, "will": true, "are": true, "is": true, "of": true, "to": true, "for": true, "from": true,
	"what": true, "where": true, "when": true, "they": true, "we": true, "my": true, "be": true, "it": true,
}

// ContaLibreria: quante traduzioni ci sono nel file di libreria di un gioco.
func ContaLibreria(file string) int {
	b, err := os.ReadFile(file)
	if err != nil {
		return 0
	}
	m := map[string]string{}
	if json.Unmarshal(b, &m) != nil {
		return 0
	}
	return len(m)
}

// fileLibreriaPer: nome del file della libreria di un gioco ("Path of Titans" -> path-of-titans.json).
func fileLibreriaPer(dir, gioco string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(gioco) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	nome := strings.Trim(b.String(), "-")
	if nome == "" {
		nome = "senza-nome"
	}
	return filepath.Join(dir, nome+".json")
}

func (t *Traduttore) salvaCache() {
	if t.fileCache == "" {
		return
	}
	t.mu.Lock()
	if len(t.cache) > 20000 { // non la lascio crescere all'infinito
		t.cache = map[string]string{}
	}
	b, err := json.Marshal(t.cache)
	t.mu.Unlock()
	if err != nil {
		return
	}
	muFileCache.Lock()
	defer muFileCache.Unlock()
	tmp := t.fileCache + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, t.fileCache)
	}
}

// sigleItaliane: se il motore ha lasciato una sigla inglese del glossario (HP, XP, NPC...),
// la sostituisce con quella italiana (PV, PE, PNG...). Serve soprattutto al motore veloce,
// che il glossario non lo legge.
func sigleItaliane(s string, voci []voceTrovata) string {
	for _, v := range voci {
		if len(v.Inglese) > 5 || strings.ToUpper(v.Inglese) != v.Inglese || v.Inglese == v.Italiano {
			continue
		}
		re, err := regexp.Compile(`\b` + regexp.QuoteMeta(v.Inglese) + `\b`)
		if err == nil {
			s = re.ReplaceAllString(s, v.Italiano)
		}
	}
	return s
}

// Il motore veloce non conosce il glossario: nelle scritte brevi (menu, statistiche) metto al
// posto dei termini del glossario dei "nomi finti" che il motore lascia come sono, e dopo la
// traduzione li sostituisco con l'italiano del glossario. Se un nome finto si perde, niente blocco.
var nomiFinti = []string{"Qzxa", "Qzxe", "Qzxi", "Qzxo", "Qzxu", "Qzya", "Qzye", "Qzyi"}

func bloccaTermini(testo string, voci []voceTrovata) (string, map[string]string) {
	if len(strings.Fields(testo)) > 8 || len(voci) == 0 {
		return testo, nil
	}
	m := map[string]string{}
	out := testo
	n := 0
	for _, v := range voci { // le più lunghe prima (trova le ordina così)
		if n >= len(nomiFinti) || v.Inglese == v.Italiano {
			continue
		}
		// solo all'inizio della scritta (etichetta): in mezzo alle frasi il motore si confonde
		re, err := regexp.Compile(`(?i)^\W*` + regexp.QuoteMeta(v.Inglese) + `\b`)
		if err != nil || !re.MatchString(out) {
			continue
		}
		loc := re.FindStringIndex(out)
		inizio := strings.IndexFunc(out, unicode.IsLetter)
		out = out[:inizio] + nomiFinti[n] + out[loc[1]:]
		m[nomiFinti[n]] = v.Italiano
		n++
	}
	if n == 0 {
		return testo, nil
	}
	return out, m
}

func sbloccaTermini(s string, m map[string]string) (string, bool) {
	for finto, it := range m {
		if !strings.Contains(s, finto) {
			return "", false
		}
		s = strings.ReplaceAll(s, finto, it)
	}
	return s, true
}

// materiali: "Iron Helmet" -> "Elmo di ferro", sempre uguale in tutto il gioco.
var materiali = map[string]string{
	"iron": "di ferro", "steel": "d'acciaio", "leather": "di cuoio", "silver": "d'argento",
	"gold": "d'oro", "golden": "d'oro", "wooden": "di legno", "wood": "di legno", "bronze": "di bronzo",
	"copper": "di rame", "stone": "di pietra", "bone": "d'osso", "crystal": "di cristallo",
	"diamond": "di diamante", "glass": "di vetro", "silk": "di seta", "cloth": "di stoffa",
	"linen": "di lino", "wool": "di lana", "fur": "di pelliccia", "obsidian": "d'ossidiana",
	"ebony": "d'ebano", "ivory": "d'avorio", "ruby": "di rubino", "emerald": "di smeraldo",
	"sapphire": "di zaffiro", "mithril": "di mithril", "titanium": "di titanio", "chain": "di maglia",
	"dragonbone": "d'osso di drago", "dragonscale": "di scaglie di drago", "hide": "di pelle",
	"scale": "a scaglie", "plate": "a piastre", "brass": "d'ottone", "tin": "di stagno",
}

func (t *Traduttore) oggettoDiMateriale(s string) (string, bool) {
	w := strings.Fields(s)
	if len(w) < 2 || len(w) > 3 {
		return "", false
	}
	mat, ok := materiali[strings.ToLower(w[0])]
	if !ok {
		return "", false
	}
	resto := strings.Join(w[1:], " ")
	ogg, ok := t.termineEsatto(resto)
	if !ok {
		return "", false
	}
	if resto == strings.ToUpper(resto) && len(resto) > 3 {
		mat = strings.ToUpper(mat)
	}
	return ogg + " " + mat, true
}

// traduciCoppia: lingue diverse da inglese → italiano. Mozilla (con l'inglese in mezzo se non
// c'è il modello diretto) e una memoria delle frasi già tradotte.
func (t *Traduttore) traduciCoppia(ctx context.Context, grezzo, da, a string, inizio time.Time) Risultato {
	testo := strings.TrimSpace(strings.Join(strings.Fields(strings.ReplaceAll(grezzo, "\n", " ")), " "))
	ris := Risultato{Originale: testo}
	if testo == "" || da == a {
		ris.Tradotto = testo
		return ris
	}
	chiave := da + a + "\x00" + testo
	t.mu.Lock()
	if v, ok := t.altreCoppie[chiave]; ok {
		t.mu.Unlock()
		ris.Tradotto, ris.Motore = v, "memoria"
		ris.Millisec = time.Since(inizio).Milliseconds()
		return ris
	}
	motori := append([]Motore(nil), t.motori...)
	t.mu.Unlock()
	if da == "en" {
		if v, ok := fraseLiberaLingua(testo, a); ok {
			ris.Tradotto, ris.Motore = v, "libreria giochi liberi"
			ris.Millisec = time.Since(inizio).Milliseconds()
			return ris
		}
	}
	for _, m := range motori {
		mc, ok := m.(MotoreCoppie)
		if !ok {
			continue
		}
		out, err := mc.TraduciCoppia(ctx, testo, da, a)
		if err != nil {
			ris.Errore = err.Error()
			continue
		}
		if strings.TrimSpace(out) == "" {
			ris.Errore = "traduzione vuota"
			continue
		}
		ris.Tradotto, ris.Motore, ris.Errore = strings.TrimSpace(out), m.Nome(), ""
		t.mu.Lock()
		if t.altreCoppie == nil || len(t.altreCoppie) > 20000 {
			t.altreCoppie = map[string]string{}
		}
		t.altreCoppie[chiave] = ris.Tradotto
		t.mu.Unlock()
		t.accodaCorrezioneCoppia(chiave, testo, ris.Tradotto, da, a)
		break
	}
	if ris.Tradotto == "" && ris.Errore == "" {
		ris.Errore = "nessun traduttore per " + da + " → " + a
	}
	ris.Millisec = time.Since(inizio).Milliseconds()
	return ris
}

// nomiIntatti: i nomi inventati del gioco ("Palbox", "Eldenring") non sono parole inglesi e non
// vanno tradotti; a volte il motore li storpia ("Palibox"). Se nella traduzione c'è una parola
// quasi uguale al nome (1-2 lettere diverse, stessa iniziale) la rimetto com'era.
func nomiIntatti(en, it string) string {
	it = nomiFissiIntatti(en, it)
	for _, w := range strings.FieldsFunc(en, func(r rune) bool { return !unicode.IsLetter(r) }) {
		r := []rune(w)
		if len(r) < 4 || !unicode.IsUpper(r[0]) || strings.ToUpper(w) == w || esiste(w) || strings.Contains(it, w) {
			continue
		}
		if _, ok := termGiochi[w]; ok {
			continue
		}
		parole := strings.FieldsFunc(it, func(r rune) bool { return !unicode.IsLetter(r) })
		for _, v := range parole {
			rv := []rune(v)
			if len(rv) < 3 || unicode.ToLower(rv[0]) != unicode.ToLower(r[0]) || abs(len(rv)-len(r)) > 2 {
				continue
			}
			if d := distanzaLettere(strings.ToLower(v), strings.ToLower(w)); d >= 1 && d <= 2 && d*4 <= len(r) {
				it = sostituisciParola(it, v, w)
				break
			}
		}
	}
	return it
}

// nomiFissiIntatti: i nomi del catalogo da non tradurre ("Hello Games", "No Man's Sky") che il
// motore ha tradotto lo stesso dentro una frase: "Ciao Games" -> "Hello Games",
// "Sky Forsaken di No Man" -> "No Man's Sky Forsaken".
// nomeGiocoAttivo: il nome del gioco che si sta traducendo ("Liar's Bar"). Conta come un nome
// fisso: "Liar's Bar DECK" non deve diventare "Il Ponte Bar di Liar" (visto il 23/11).
var nomeGiocoAttivo atomic.Value

func impostaNomeGiocoAttivo(n string) { nomeGiocoAttivo.Store(n) }

func nomiFissiIntatti(en, it string) string {
	caricaCataloghi()
	nomi := nomiFissi
	if g, _ := nomeGiocoAttivo.Load().(string); len(strings.Fields(g)) >= 2 {
		nomi = append([]string{g}, nomiFissi...)
	}
	for _, n := range nomi {
		if !strings.Contains(strings.ToLower(en), strings.ToLower(n)) || strings.Contains(it, n) {
			continue
		}
		// "X's Y" diventa spesso "Y di X"
		if x, y, ok := strings.Cut(n, "'s "); ok {
			re, err := regexp.Compile(`(?i:\b(?:il|lo|la|l')\s*)?\b(?i:` + regexp.QuoteMeta(y) + `)((?:\s+\p{Lu}[\p{L}\p{N}']*){0,3})\s+(?i:di|del|dello|della|dei|degli)\s+` + regexp.QuoteMeta(x) + `\b`)
			if err == nil && re.MatchString(it) {
				it = re.ReplaceAllString(it, strings.ReplaceAll(n, "$", "$$")+"$1")
				continue
			}
		}
		// le ultime parole sono rimaste: cambio le prime ("Ciao Games" -> "Hello Games")
		w := strings.Fields(n)
		coda := strings.Join(w[1:], " ")
		if len([]rune(coda)) < 4 {
			continue
		}
		re, err := regexp.Compile(`(?:^|\b)[\p{L}']+\s+` + regexp.QuoteMeta(coda) + `\b`)
		if err != nil {
			continue
		}
		if loc := re.FindStringIndex(it); loc != nil {
			it = it[:loc[0]] + n + it[loc[1]:]
		}
	}
	return it
}

func sostituisciParola(s, vecchia, nuova string) string {
	re, err := regexp.Compile(`\b` + regexp.QuoteMeta(vecchia) + `\b`)
	if err != nil {
		return s
	}
	return re.ReplaceAllLiteralString(s, nuova)
}

// distanzaLettere: quante lettere aggiungere, togliere o cambiare per passare da a a b.
func distanzaLettere(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prec := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prec {
		prec[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			c := 1
			if ra[i-1] == rb[j-1] {
				c = 0
			}
			cur[j] = min(prec[j]+1, cur[j-1]+1, prec[j-1]+c)
		}
		prec, cur = cur, prec
	}
	return prec[len(rb)]
}

// fraseMenuConNumero: come fraseMenu, e in più le voci con un numero davanti o dietro, anche tra
// parentesi: "32 (Default)" → "32 (Predefinito)", "Quality: 2" → "Qualità: 2". La libreria del
// gioco aveva imparato "32 (Defeal)" (PC Cafe Simulator, 23/11): il catalogo ora vince.
func fraseMenuConNumero(s string) (string, bool) {
	if v, ok := fraseMenu(s); ok {
		return v, true
	}
	if v, ok := suggerimentoTasto(s); ok {
		return v, true
	}
	m := reNumeroVoce.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	pre, voce, post := m[1], m[2], m[3]
	if strings.TrimSpace(pre) == "" && strings.TrimSpace(post) == "" {
		return "", false
	}
	apri, chiudi := "", ""
	if strings.HasPrefix(voce, "(") && strings.HasSuffix(voce, ")") {
		apri, chiudi, voce = "(", ")", voce[1:len(voce)-1]
	}
	v, ok := fraseMenu(voce)
	if !ok {
		return "", false
	}
	return pre + apri + v + chiudi + post, true
}

var reNumeroVoce = regexp.MustCompile(`^([0-9][0-9.,%x×]*\s+)?(\(?[^0-9()]+\)?)(:?\s+[0-9][0-9.,%x×]*)?$`)

// suggerimentoTasto: i suggerimenti dei tasti in basso nei giochi. "Hold T skip", "HoldT skip",
// "Hold T to skip", "Press E to interact", "Q Hold" → "Tieni premuto T: salta", "E: interagisci",
// "Tieni premuto Q" (PC Cafe Simulator, 23/11: restavano in inglese).
func suggerimentoTasto(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if m := reTastoDopo.FindStringSubmatch(s); m != nil { // "Q Hold"
		return "Tieni premuto " + m[1], true
	}
	m := reTastoPrima.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	tieni, tasto, azione := strings.EqualFold(m[1], "hold"), m[2], strings.TrimSpace(m[3])
	pre := tasto
	if tieni {
		pre = "Tieni premuto " + tasto
	}
	if azione == "" {
		return pre, true
	}
	it, ok := fraseMenu(azione)
	if !ok {
		it, ok = fraseMenu(strings.ToUpper(azione[:1]) + azione[1:])
	}
	if !ok {
		return "", false // azione sconosciuta: ci pensa il traduttore
	}
	return pre + ": " + strings.ToLower(it[:1]) + it[1:], true
}

var (
	nomiTastiGioco = `[A-Z0-9]|F[0-9]{1,2}|Space|Spacebar|Enter|Esc|Tab|Shift|Ctrl|Alt|LMB|RMB|Mouse[12]`
	reTastoPrima   = regexp.MustCompile(`^(?i:(hold|press))\s*(` + nomiTastiGioco + `)\b\s*(?i:to\s+)?(.*)$`)
	reTastoDopo    = regexp.MustCompile(`^(` + nomiTastiGioco + `)\s+(?i:hold)$`)
)
