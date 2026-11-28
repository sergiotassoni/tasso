package traduce

// Nomi da non tradurre: negozi e launcher ("Steam" non è "Vapore"), marchi, i giochi della
// libreria di Tasso con i loro launcher ("Hytale Launcher" non è "Lanciatore Hytale"), nomi
// utente con numeri ("Lando94", "1LMess1a") e numeri con unità ("15,3 GB", "144 Hz").
//
// Come: la scritta si divide nei pezzi protetti e nei pezzi da tradurre; si traducono solo
// questi ultimi e i nomi tornano al loro posto così come erano. Se dopo i nomi non resta niente
// da tradurre, la scritta resta uguale (e chi la mostra non la fa vedere come "tradotta").
// Nelle frasi lunghe (dialoghi) dividere guasterebbe la frase: lì si traduce tutto e poi si
// rimette il nome se il traduttore l'ha cambiato.

import (
	"context"
	_ "embed"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

//go:embed dati/non-tradurre.txt
var nonTradurreTXT string

// VoceLibreria: un gioco della libreria di Tasso (nome mostrato e programma da avviare).
type VoceLibreria struct {
	Nome string
	Exe  string
}

var nonTradurreFissi = sync.OnceValue(func() []string {
	var out []string
	for _, l := range strings.Split(nonTradurreTXT, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		out = append(out, l)
	}
	return out
})

var (
	muNomiProt    sync.Mutex
	nomiProtCache []string
	nomiProtDa    time.Time
)

// nomiProtetti: tutti i nomi da non tradurre, dal più lungo al più corto ("Hytale Launcher"
// prima di "Hytale"). Quelli della libreria si rileggono al massimo ogni 30 secondi.
func nomiProtetti() []string {
	muNomiProt.Lock()
	defer muNomiProt.Unlock()
	if nomiProtCache != nil && time.Since(nomiProtDa) < 30*time.Second {
		return nomiProtCache
	}
	visti := map[string]bool{}
	var out []string
	add := func(n string) {
		n = strings.Join(strings.Fields(n), " ")
		k := strings.ToLower(n)
		if len([]rune(n)) < 2 || visti[k] {
			return
		}
		visti[k] = true
		out = append(out, n)
	}
	for _, n := range nonTradurreFissi() {
		add(n)
	}
	if Ospite.VociLibreria != nil {
		for _, v := range Ospite.VociLibreria() {
			add(v.Nome)
			add(senzaLauncher(v.Nome))
		}
	}
	if g, _ := nomeGiocoAttivo.Load().(string); g != "" {
		add(g)
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	nomiProtCache, nomiProtDa = out, time.Now()
	return out
}

// dimenticaNomiProtetti: la libreria è cambiata, i nomi si rileggono alla prossima scritta.
func dimenticaNomiProtetti() {
	muNomiProt.Lock()
	nomiProtCache = nil
	muNomiProt.Unlock()
}

// senzaLauncher: "Hytale Launcher" → "Hytale"; "Launcher di Hytale" → "Hytale". Resta uguale se
// non c'è la parola o se senza non resta niente.
func senzaLauncher(n string) string {
	w := strings.Fields(n)
	var out []string
	for _, p := range w {
		switch strings.ToLower(strings.Trim(p, "-–—:()")) {
		case "launcher", "launch", "client", "di", "the":
			if len(w) > 1 {
				continue
			}
		}
		out = append(out, p)
	}
	r := strings.Join(out, " ")
	if len([]rune(r)) < 2 {
		return n
	}
	return r
}

var (
	// numeri con unità di misura: "15,3 GB", "9.2 GB", "144 Hz", "60 FPS", "16 ms", "3,5 GHz"
	reNumeroUnita = regexp.MustCompile(`(?i)\b\d+(?:[.,]\d+)?\s?(?:kb|mb|gb|tb|kib|mib|gib|tib|hz|khz|mhz|ghz|fps|ms|mbps|kbps|gbps|°c|°f|w|v|mah|px|dpi)\b`)
	// nomi utente e sigle con lettere e numeri insieme: "Lando94", "1LMess1a", "xX_Pro_Xx"
	reNomeUtente = regexp.MustCompile(`\b(?:[A-Za-z][A-Za-z_]*\d[A-Za-z0-9_]*|\d+[A-Za-z][A-Za-z0-9_]*[A-Za-z][A-Za-z0-9_]*)\b`)
)

// pezzo di una scritta: protetto (resta com'è) o da tradurre.
type pezzoScritta struct {
	testo    string
	protetto bool
}

// dividiProtetti: la scritta divisa in pezzi protetti e pezzi da tradurre. nomi: i nomi da
// non tradurre (dal più lungo). Senza nessun pezzo protetto restituisce un solo pezzo.
func dividiProtetti(s string, nomi []string) []pezzoScritta {
	segnati := make([]bool, len(s))
	segna := func(a, z int) {
		for i := a; i < z; i++ {
			segnati[i] = true
		}
	}
	basso := strings.ToLower(s)
	for _, n := range nomi {
		nb := strings.ToLower(n)
		for i := 0; ; {
			j := strings.Index(basso[i:], nb)
			if j < 0 {
				break
			}
			a, z := i+j, i+j+len(nb)
			if confineParola(s, a, z) && !segnati[a] && formaGiusta(s[a:z], n) {
				segna(a, z)
			}
			i = z
		}
	}
	for _, m := range reNumeroUnita.FindAllStringIndex(s, -1) {
		segna(m[0], m[1])
	}
	for _, m := range reNomeUtente.FindAllStringIndex(s, -1) {
		if sigleBreviConNumero(s[m[0]:m[1]]) {
			continue // "x3", "F7", "C4", "4K", "Lv10": restano già uguali, non servono come nomi
		}
		segna(m[0], m[1])
	}
	var out []pezzoScritta
	for i := 0; i < len(s); {
		j := i
		for j < len(s) && segnati[j] == segnati[i] {
			j++
		}
		out = append(out, pezzoScritta{testo: s[i:j], protetto: segnati[i]})
		i = j
	}
	return out
}

// formaGiusta: un nome fatto solo di parole inglesi ("Steam", "CRACK", "The Forest") vale solo
// scritto come nel nome o tutto maiuscolo: "steam" o "the forest" in una frase restano da
// tradurre. Un nome con una parola inventata ("Hytale", "Subnautica 2") vale comunque scritto.
func formaGiusta(trovato, nome string) bool {
	if trovato == nome || trovato == strings.ToUpper(nome) {
		return true
	}
	for _, w := range strings.FieldsFunc(nome, func(r rune) bool { return !unicode.IsLetter(r) && r != '\'' }) {
		if !esiste(w) {
			return true
		}
	}
	return false
}

// sigleBreviConNumero: lettere e numeri insieme ma corti (al massimo 4 caratteri): tasti,
// quantità, livelli. Non sono nomi utente.
func sigleBreviConNumero(w string) bool { return len(w) <= 4 }

// confineParola: s[a:z] è una parola intera (niente lettere o numeri subito prima o dopo).
func confineParola(s string, a, z int) bool {
	parola := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
	if a > 0 {
		r := []rune(s[:a])
		if parola(r[len(r)-1]) {
			return false
		}
	}
	if z < len(s) {
		r := []rune(s[z:])
		if parola(r[0]) {
			return false
		}
	}
	return true
}

// soloProtetti: nei pezzi non resta niente da tradurre (solo nomi, numeri e segni).
func soloProtetti(pezzi []pezzoScritta) bool {
	for _, p := range pezzi {
		if !p.protetto && strings.IndexFunc(p.testo, unicode.IsLetter) >= 0 {
			return false
		}
	}
	return true
}

// paroleDaTradurre: quante parole ci sono nei pezzi da tradurre.
func paroleDaTradurre(pezzi []pezzoScritta) int {
	n := 0
	for _, p := range pezzi {
		if !p.protetto {
			n += len(strings.FieldsFunc(p.testo, func(r rune) bool { return !unicode.IsLetter(r) && r != '\'' }))
		}
	}
	return n
}

// separatoreEtichette: i pezzi da tradurre sono etichette staccate dai nomi da segni
// (" · ", " — ", " | ", ":") o corte: si possono tradurre da sole senza guastare una frase.
func pezziStaccati(pezzi []pezzoScritta) bool {
	for _, p := range pezzi {
		if p.protetto {
			continue
		}
		t := strings.TrimSpace(p.testo)
		if t == "" || strings.IndexFunc(t, unicode.IsLetter) < 0 {
			continue
		}
		if len(strings.Fields(t)) > 4 {
			return false
		}
	}
	return true
}

// traduciConNomiProtetti: se la scritta contiene nomi da non tradurre, traduce solo il resto.
// ok=false: niente nomi protetti (si traduce come sempre).
func (t *Traduttore) traduciConNomiProtetti(ctx context.Context, s string, dialogo bool, da, a string, inizio time.Time) (Risultato, bool) {
	if ctx.Value(chiaveProtetti{}) != nil {
		return Risultato{}, false // già diviso: i pezzi non contengono nomi
	}
	pezzi := dividiProtetti(s, nomiProtetti())
	if len(pezzi) == 1 && !pezzi[0].protetto {
		return Risultato{}, false
	}
	ris := Risultato{Originale: s}
	if soloProtetti(pezzi) {
		ris.Tradotto, ris.Motore, ris.Millisec = s, "nome", time.Since(inizio).Milliseconds()
		return ris, true
	}
	if !pezziStaccati(pezzi) {
		return Risultato{}, false // frase lunga con un nome dentro: la ripara rimettiNomiProtetti
	}
	ctx = context.WithValue(ctx, chiaveProtetti{}, true)
	var b strings.Builder
	for _, p := range pezzi {
		if p.protetto || strings.IndexFunc(p.testo, unicode.IsLetter) < 0 {
			b.WriteString(p.testo)
			continue
		}
		testa := p.testo[:len(p.testo)-len(strings.TrimLeft(p.testo, " ·—–-|:,/()"))]
		coda := p.testo[len(strings.TrimRight(p.testo, " ·—–-|:,/()")):]
		dentro := strings.TrimSpace(p.testo[len(testa) : len(p.testo)-len(coda)])
		if dentro == "" {
			b.WriteString(p.testo)
			continue
		}
		r := t.traduciIn(ctx, dentro, dialogo, da, a)
		if r.Errore != "" && r.Tradotto == "" {
			ris.Errore = r.Errore
			ris.Millisec = time.Since(inizio).Milliseconds()
			return ris, true
		}
		if ris.Motore == "" {
			ris.Motore = r.Motore
		}
		tr := strings.TrimSpace(r.Tradotto)
		if tr == "" {
			tr = dentro
		}
		b.WriteString(testa + tr + coda)
	}
	ris.Tradotto = b.String()
	if ris.Motore == "" {
		ris.Motore = "nome"
	}
	ris.Millisec = time.Since(inizio).Milliseconds()
	return ris, true
}

type chiaveProtetti struct{}

// rimettiNomiProtetti: dopo la traduzione di una frase intera, un nome protetto che il
// traduttore ha cambiato torna com'era. tradotti: come il traduttore rende il nome da solo
// ("Steam" → "Vapore"); se quella resa compare nella traduzione, al suo posto torna il nome.
func rimettiNomiProtetti(en, it string, nomi []string, tradotti func(string) string) string {
	for _, p := range dividiProtetti(en, nomi) {
		if !p.protetto {
			continue
		}
		n := strings.TrimSpace(p.testo)
		if n == "" || strings.Contains(strings.ToLower(it), strings.ToLower(n)) {
			continue
		}
		if tradotti == nil {
			continue
		}
		v := strings.TrimSpace(strings.TrimRight(tradotti(n), "."))
		if v == "" || strings.EqualFold(v, n) {
			continue
		}
		if i := strings.Index(strings.ToLower(it), strings.ToLower(v)); i >= 0 && confineParola(it, i, i+len(v)) {
			it = it[:i] + n + it[i+len(v):]
		}
	}
	return it
}
