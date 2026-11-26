package traduce

// Stabilità delle scritte già mostrate (collaudo finale 26/11, prova su 6 giochi mai visti).
//
//  1. Maiuscole: la traduzione fissata o presa dalla memoria segue le maiuscole della scritta di
//     ADESSO. La chiave delle traduzioni fisse è minuscola, quindi "Reach the camp" prendeva
//     "RAGGIUNGI IL CAMPO" fissata prima per "REACH THE CAMP" (007 First Light, 30 fotogrammi).
//  2. Decisione tenuta: una scritta già tradotta e mostrata in questa sessione (stesso testo,
//     normalizzato) non torna in inglese perché un fotogramma la giudica "scritta del mondo" o
//     "lettura incerta". "Early Morning" (Persona 3) cambiava una decina di volte tra italiano e
//     inglese, e "IMPACT IN 86" una quindicina. Le regole che evitano le righe mezze inglesi
//     (parola accanto non letta, pezzi della stessa frase, traduzioni sovrapposte) restano tutte:
//     tenere la decisione non deve mai lasciare mezza riga inglese accanto all'italiano.

import (
	"strings"
	"sync"
	"time"
	"unicode"
)

// maiuscoleDellaScritta: it (traduzione presa dalla memoria) con le maiuscole della scritta en
// di adesso. Scritta tutta maiuscola: traduzione tutta maiuscola. Scritta normale e traduzione
// tutta maiuscola (salvata da una scritta maiuscola): solo l'iniziale maiuscola, e restano com'erano
// le parole che nella scritta inglese sono uguali (nomi, sigle, tasti: "Bond", "HP", "E").
func maiuscoleDellaScritta(en, it string) string {
	en = strings.TrimSpace(en)
	if en == "" || strings.TrimSpace(it) == "" {
		return it
	}
	if tuttoMaiuscolo(en) {
		if strings.ContainsFunc(it, unicode.IsLower) {
			return strings.ToUpper(it)
		}
		return it
	}
	if !tuttoMaiuscolo(it) || !strings.ContainsFunc(en, unicode.IsLower) {
		return it
	}
	uguali := map[string]bool{} // le parole maiuscole della scritta inglese (sigle, tasti)
	for _, w := range strings.FieldsFunc(en, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if !strings.ContainsFunc(w, unicode.IsLower) {
			uguali[w] = true
		}
	}
	nomi := map[string]string{} // le parole con l'iniziale maiuscola: "BOND" → "Bond"
	for _, w := range strings.FieldsFunc(en, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if r := []rune(w); len(r) > 1 && unicode.IsUpper(r[0]) && strings.ContainsFunc(w, unicode.IsLower) {
			nomi[strings.ToUpper(w)] = w
		}
	}
	var b strings.Builder
	parola := []rune{}
	primaParola := true
	chiudi := func() {
		if len(parola) == 0 {
			return
		}
		w := string(parola)
		switch {
		case len(parola) <= 3 && uguali[w], len(parola) == 2 && !paroleCorteItaliane[strings.ToLower(w)]:
			// sigla o tasto rimasti uguali ("HP", "E", "LMB") o sigla italiana ("PV" per "HP")
		case nomi[w] != "":
			w = nomi[w]
		default:
			w = strings.ToLower(w)
			if primaParola {
				r := []rune(w)
				r[0] = unicode.ToUpper(r[0])
				w = string(r)
			}
		}
		primaParola = false
		b.WriteString(w)
		parola = parola[:0]
	}
	for _, r := range it {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			parola = append(parola, r)
			continue
		}
		chiudi()
		b.WriteRune(r)
	}
	chiudi()
	out := b.String()
	if r := []rune(en); unicode.IsLower(r[0]) { // "press E" → l'iniziale minuscola come il gioco
		if ro := []rune(out); len(ro) > 0 && unicode.IsUpper(ro[0]) && !uguali[strings.Fields(out)[0]] {
			ro[0] = unicode.ToLower(ro[0])
			out = string(ro)
		}
	}
	return out
}

// mostrataPrima: la scritta (testo esatto, normalizzato come le traduzioni fisse) è già stata
// tradotta e disegnata in questa sessione. Solo il testo esatto: una lettura con una lettera
// diversa non conta, potrebbe essere un'altra scritta.
func (f *traduzioniFisse) mostrataPrima(coppia, testo string) bool {
	if f == nil {
		return false
	}
	k := chiaveFissa(coppia, testo)
	if k == "" {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.m[k]
	return ok
}

// segnaGiaMostrate: segna le scritte la cui decisione di tradurle è tenuta (vedi
// Blocco.GiaMostrata): già tradotte e mostrate, e confermate di recente da sole (conferme).
func segnaGiaMostrate(f *traduzioniFisse, c *confermeDecisione, coppia string, bl []Blocco, adesso time.Time) {
	for i := range bl {
		bl[i].GiaMostrata = f.mostrataPrima(coppia, bl[i].Testo) && c.valida(coppia, bl[i].Testo, adesso)
	}
}

// La decisione tenuta scade (collaudo 26/11, giro 2): una scritta del mondo tradotta per sbaglio
// in un fotogramma non deve restare tradotta per tutta la sessione. Regola: la decisione si tiene
// solo se la scritta è stata tradotta e mostrata "da sola" (senza la decisione tenuta, Blocco.Tenuta)
// in almeno confermeMinime fotogrammi; da lì resta per tutta la partita (26/11 12:22, Sergio). I fotogrammi
// in cui resta tradotta solo grazie alla decisione tenuta non la rinnovano: passato il tempo, la
// scritta si giudica di nuovo da sola (e se è del mondo torna in inglese).
//
// La scadenza conta anche i giri mostrati (scadenzaGiri): con la schermata ferma non si rilegge e
// non si mostra niente di nuovo, quindi non passa nessun giro. Un menu fermo per un minuto non
// perde la decisione appena qualcos'altro cambia sullo schermo e fa rileggere tutto.
const (
	confermeMinime    = 2
	scadenzaDecisione = 20 * time.Second
	scadenzaGiri      = 40 // circa 10-20 s di giri quando lo schermo si muove davvero
)

type conferma struct {
	n      int
	ultima time.Time
	giro   int64 // giro (segna) dell'ultima conferma
}

type confermeDecisione struct {
	mu   sync.Mutex
	m    map[string]conferma
	giri int64 // quante volte è stato chiamato segna (giri mostrati)
}

// confermePagina: quelle della traduzione dello schermo.
var confermePagina = &confermeDecisione{}

// segna: le scritte mostrate (disegnate) che in questo fotogramma sono state decise da sole.
// freschi: le traduzioni di questa lettura (quelle tenute dal fotogramma prima non contano).
func (c *confermeDecisione) segna(coppia string, mostrate, freschi []Blocco, adesso time.Time) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]conferma{}
	}
	c.giri++
	for _, b := range mostrate {
		if b.Tenuta || !contieneBlocco(freschi, b) {
			continue
		}
		k := chiaveFissa(coppia, b.Testo)
		if k == "" {
			continue
		}
		x := c.m[k]
		x.n++
		x.ultima = adesso
		x.giro = c.giri
		c.m[k] = x
	}
	if len(c.m) > 5000 { // via le più vecchie
		for k, x := range c.m {
			if adesso.Sub(x.ultima) > scadenzaDecisione && c.giri-x.giro > scadenzaGiri {
				delete(c.m, k)
			}
		}
	}
}

// valida: la decisione di tradurre la scritta si tiene ancora.
func (c *confermeDecisione) valida(coppia, testo string, adesso time.Time) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	x, ok := c.m[chiaveFissa(coppia, testo)]
	if !ok || x.n < confermeMinime {
		return false
	}
	// Regola di Sergio (26/11 12:22): una scritta tradotta e confermata resta tradotta per tutta la
	// partita, uguale, anche quando ci ritorni: niente salti tra italiano e inglese. La decisione
	// non scade più col tempo; si azzera solo quando cambia il gioco (con le traduzioni fisse).
	_ = adesso
	return true
}

// letturaPiuGiusta: la lettura a è quella buona e b quella sbagliata della stessa scritta: a è già
// stata mostrata e confermata (decisione tenuta) e b no, oppure b ha una parola inglese che non
// esiste e a no ("Reach the camp" dopo "Reach the comp", "Early Morning" dopo "Early Meroing").
func letturaPiuGiusta(a, b Blocco) bool {
	if a.GiaMostrata != b.GiaMostrata {
		return a.GiaMostrata
	}
	return !letturaConParolaStrana(a.Testo) && letturaConParolaStrana(b.Testo)
}

// motivoTenibile: i motivi per lasciare l'inglese che una scritta già mostrata in italiano non
// segue (cambiano da un fotogramma all'altro sulla stessa scritta ferma). Gli altri restano
// sempre: firma o logo nell'angolo, righe incomplete, pezzi di frase, traduzioni sovrapposte.
func motivoTenibile(m string) bool {
	switch {
	case m == motivoFirmaAngolo:
		return false
	case strings.HasPrefix(m, "scritta del mondo"):
		return true
	case m == "lettura incerta":
		return true
	}
	return false
}

// paroleCorteItaliane: le parole italiane di due lettere (le altre di due lettere in una
// traduzione tutta maiuscola sono sigle: "PV", "PM", "PE", "IA").
var paroleCorteItaliane = map[string]bool{"il": true, "lo": true, "la": true, "le": true, "li": true,
	"un": true, "di": true, "da": true, "in": true, "su": true, "tu": true, "io": true, "mi": true, "ti": true,
	"ci": true, "vi": true, "si": true, "ne": true, "se": true, "ma": true, "ho": true, "ha": true, "fa": true,
	"va": true, "sa": true, "no": true, "sì": true, "è": true, "ai": true, "al": true, "re": true, "ok": true,
	"tè": true, "dì": true, "là": true, "lì": true, "po": true, "ad": true, "ed": true, "od": true, "gi": true}

// senzaScritteCambiate: tolte le traduzioni tenute dal fotogramma prima (isteresi, recuperate)
// sopra cui la lettura di adesso trova un'altra scritta, anche se quella lettura resta in inglese
// perché "non è testo da tradurre". Banco del mattino f.2368: la barra delle schede è scorsa,
// "PROSTHETICS" (letto con affidabilità 0) è arrivato dove c'era "HAIR COLOR" e restava
// "COLORE DEI CAPELLI" (due schede con lo stesso nome). freschi: le traduzioni di questa lettura;
// letti: tutte le scritte lette adesso. Una lettura della stessa scritta, anche a pezzi o con
// lettere sbagliate, tiene la traduzione.
func senzaScritteCambiate(pronti, freschi, letti []Blocco) []Blocco {
	out := pronti[:0:0]
	for _, p := range pronti {
		if contieneBlocco(freschi, p) || p.Tradotto == "" {
			out = append(out, p)
			continue
		}
		rp := rettBlocco(p)
		cambiata := false
		for _, l := range letti {
			rl := rettBlocco(l)
			in := rp.Intersect(rl)
			if in.Empty() || in.Dx()*in.Dy()*2 < min(rp.Dx()*rp.Dy(), rl.Dx()*rl.Dy()) {
				continue // tocca appena: è la scritta accanto
			}
			lp, ll := lettereConfuse(p.Testo), lettereConfuse(l.Testo)
			if len([]rune(ll)) < 3 || testoSimile(p.Testo, l.Testo) || strings.Contains(lp, ll) || strings.Contains(ll, lp) {
				continue // la stessa scritta (o un suo pezzo, o un segno)
			}
			cambiata = true
			break
		}
		if !cambiata {
			out = append(out, p)
		}
	}
	return out
}

// daFissare: la traduzione da fissare per tutta la sessione (fisse.go), ma non quella di una
// lettura con una parola inglese che non esiste ("Early Meroing", "Locate the cahp"): è una
// lettura sbagliata, e la sua traduzione ("La prima meroing") non deve diventare quella fissa
// della scritta né tenere la decisione di tradurla (collaudo finale 26/11, Persona 3). Le parole
// con la maiuscola dentro la frase sono nomi (restano buone anche se il vocabolario non le ha).
func daFissare(b Blocco, tradotto string) string {
	if letturaConParolaStrana(b.Testo) {
		return ""
	}
	return tradotto
}

// letturaConParolaStrana: nella scritta c'è una parola minuscola (almeno 4 lettere, senza
// cifre) che in inglese non esiste.
func letturaConParolaStrana(s string) bool {
	if len(vocabolario()) == 0 {
		return false
	}
	// nelle scritte con le iniziali maiuscole o tutte maiuscole la maiuscola non dice "nome"
	tutte := titoloMaiuscole(s) || tuttoMaiuscolo(s)
	for i, w := range strings.Fields(s) {
		w = strings.Trim(w, ".,;:!?\"'()[]<>«»*-—…")
		r := []rune(w)
		if len(r) < 4 || strings.ContainsFunc(w, unicode.IsDigit) || strings.ContainsAny(w, "'’-") {
			continue
		}
		if esiste(w) {
			continue
		}
		if unicode.IsUpper(r[0]) {
			if i > 0 && !tutte {
				continue // nome dentro la frase
			}
			// all'inizio o in un titolo: un nome ("Lunka") o una parola letta male ("Meroing"
			// per "Morning")? letta male se è vicinissima a una parola inglese vera
			if !vicinaAParolaVera(strings.ToLower(w)) {
				continue
			}
		}
		return true
	}
	return false
}

var (
	muVicine sync.Mutex
	vicine   = map[string]bool{}
)

// vicinaAParolaVera: w (minuscola, che il vocabolario non ha) è a una o due lettere da una
// parola inglese vera di lunghezza simile (due solo dalle 7 lettere in su): "meroing" → "morning",
// "cahp" → "camp". I nomi inventati dei giochi di solito no ("lunka", "toriumi"). Il risultato
// resta in memoria: si cerca una volta sola per parola.
func vicinaAParolaVera(w string) bool {
	muVicine.Lock()
	if v, ok := vicine[w]; ok {
		muVicine.Unlock()
		return v
	}
	muVicine.Unlock()
	n := len([]rune(w))
	soglia := 1
	if n >= 7 {
		soglia = 2
	}
	trovata := false
	for p := range vocabolario() {
		m := len([]rune(p))
		if m < 4 || abs(m-n) > soglia || p[0] != w[0] {
			continue
		}
		if distanzaModifica(w, p) <= soglia {
			trovata = true
			break
		}
	}
	muVicine.Lock()
	if len(vicine) > 5000 {
		vicine = map[string]bool{}
	}
	vicine[w] = trovata
	muVicine.Unlock()
	return trovata
}
