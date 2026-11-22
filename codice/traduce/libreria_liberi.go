package traduce

// Libreria grande: ~27.000 frasi di giochi liberi (Wesnoth, SuperTuxKart, 0 A.D., Luanti,
// Freeciv) tradotte a mano in italiano dai loro traduttori volontari (vedi
// dati/libreria-giochi-liberi-LICENZA.txt). Due usi:
//  1. se la scritta del gioco è identica, esce subito la traduzione umana (0 ms);
//  2. le frasi più simili diventano esempi per il correttore IA (stile e parole da gioco).

import (
	"bytes"
	"compress/gzip"
	"embed"
	"io"
	"math"
	"sort"
	"strings"
	"sync"
	"unicode"
)

//go:embed dati/libreria-giochi-liberi.tsv.gz
var libreriaLiberiGZ []byte

type fraseLibera struct{ en, it string }

var (
	liberiOnce   sync.Once
	liberiEsatte map[string]string // chiaveFrase → italiano
	liberiFrasi  []fraseLibera     // solo frasi "da dialogo", per gli esempi
	liberiIndice map[string][]int32
	liberiIDF    map[string]float64
)

var apostrofiTipografici = strings.NewReplacer("’", "'", "‘", "'", "“", "\"", "”", "\"")

func chiaveLibera(s string) string {
	return chiaveFrase(apostrofiTipografici.Replace(s))
}

func caricaLiberi() {
	liberiOnce.Do(func() {
		liberiEsatte = map[string]string{}
		liberiIndice = map[string][]int32{}
		zr, err := gzip.NewReader(bytes.NewReader(libreriaLiberiGZ))
		if err != nil {
			return
		}
		b, err := io.ReadAll(zr)
		if err != nil {
			return
		}
		for _, l := range strings.Split(string(b), "\n") {
			if l == "" || strings.HasPrefix(l, "#") {
				continue
			}
			en, it, ok := strings.Cut(l, "\t")
			if !ok {
				continue
			}
			it = apostrofiTipografici.Replace(it)
			// le scritte corte ("Got it", "Shield") cambiano senso da gioco a gioco: per quelle
			// meglio il catalogo e il traduttore. Dalla libreria prendo solo frasi di 4+ parole.
			if len(strings.Fields(en)) >= 4 {
				liberiEsatte[chiaveLibera(en)] = it
			}
			if n := len(en); n >= 25 && n <= 220 {
				id := int32(len(liberiFrasi))
				liberiFrasi = append(liberiFrasi, fraseLibera{apostrofiTipografici.Replace(en), it})
				for p := range paroleUniche(en) {
					liberiIndice[p] = append(liberiIndice[p], id)
				}
			}
		}
		liberiIDF = make(map[string]float64, len(liberiIndice))
		n := float64(len(liberiFrasi))
		for p, ids := range liberiIndice {
			liberiIDF[p] = math.Log(n / float64(len(ids)))
		}
	})
}

func paroleUniche(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(apostrofiTipografici.Replace(s)), func(r rune) bool {
		return !unicode.IsLetter(r) && r != '\''
	}) {
		w = strings.Trim(w, "'")
		if len([]rune(w)) >= 3 {
			out[w] = true
		}
	}
	return out
}

// fraseLiberaEsatta: traduzione umana se la scritta è identica (maiuscole, spazi e punteggiatura
// finale a parte). Rimette la punteggiatura finale del gioco.
func fraseLiberaEsatta(battuta string) (string, bool) {
	b := strings.TrimSpace(battuta)
	if len(b) < 2 || len(b) > 300 {
		return "", false
	}
	caricaLiberi()
	it, ok := liberiEsatte[chiaveLibera(b)]
	if !ok {
		return "", false
	}
	fine := b[len(strings.TrimRightFunc(b, func(r rune) bool { return unicode.IsPunct(r) && r != '+' })):]
	itSenza := strings.TrimRightFunc(it, func(r rune) bool { return unicode.IsPunct(r) && r != '+' })
	// stessa maiuscola iniziale del gioco ("CONTINUE" resta tutto maiuscolo)
	if strings.ToUpper(b) == b && strings.ToLower(b) != b {
		itSenza = strings.ToUpper(itSenza)
	}
	return itSenza + fine, true
}

// esempiSimili: le n frasi della libreria che condividono più parole rare con la battuta.
func esempiSimili(battuta string, n int) []fraseLibera {
	caricaLiberi()
	parole := paroleUniche(battuta)
	if len(parole) == 0 {
		return nil
	}
	punti := map[int32]float64{}
	for p := range parole {
		ids := liberiIndice[p]
		if len(ids) == 0 || len(ids) > 2500 { // parole troppo comuni non dicono niente
			continue
		}
		w := liberiIDF[p]
		for _, id := range ids {
			punti[id] += w
		}
	}
	type cand struct {
		id int32
		p  float64
	}
	var c []cand
	for id, p := range punti {
		lung := float64(len(paroleUniche(liberiFrasi[id].en)))
		c = append(c, cand{id, p / math.Sqrt(lung+1)})
	}
	sort.Slice(c, func(i, j int) bool {
		if c[i].p != c[j].p {
			return c[i].p > c[j].p
		}
		return c[i].id < c[j].id
	})
	var out []fraseLibera
	for _, x := range c {
		if len(out) >= n || x.p < 1.2 {
			break
		}
		out = append(out, liberiFrasi[x.id])
	}
	return out
}

func testoEsempi(es []fraseLibera) string {
	if len(es) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Esempi di frasi di altri giochi tradotte bene a mano (solo come guida per stile e parole, NON tradurle):\n")
	for _, e := range es {
		b.WriteString("- " + e.en + " = " + e.it + "\n")
	}
	return b.String() + "\n"
}

// ---- librerie per le altre lingue di arrivo (stesse fonti libere, traduzioni dall'inglese)

//go:embed dati/librerie/*.tsv.gz
var librerieAltreFS embed.FS

var (
	muLibAltre sync.Mutex
	libAltre   = map[string]map[string]string{} // lingua → chiaveFrase → traduzione
)

func libreriaLingua(a string) map[string]string {
	muLibAltre.Lock()
	defer muLibAltre.Unlock()
	if m, ok := libAltre[a]; ok {
		return m
	}
	m := map[string]string{}
	libAltre[a] = m
	b, err := librerieAltreFS.ReadFile("dati/librerie/libreria-" + a + ".tsv.gz")
	if err != nil {
		return m
	}
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return m
	}
	tutto, _ := io.ReadAll(zr)
	for _, l := range strings.Split(string(tutto), "\n") {
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		en, tr, ok := strings.Cut(l, "\t")
		if ok && len(strings.Fields(en)) >= 4 {
			m[chiaveLibera(en)] = apostrofiTipografici.Replace(tr)
		}
	}
	return m
}

// fraseLiberaLingua: traduzione umana inglese → lingua a, se la frase è identica (4+ parole).
func fraseLiberaLingua(battuta, a string) (string, bool) {
	b := strings.TrimSpace(battuta)
	if len(b) < 2 || len(b) > 300 {
		return "", false
	}
	if a == "it" {
		return fraseLiberaEsatta(b)
	}
	tr, ok := libreriaLingua(a)[chiaveLibera(b)]
	if !ok {
		return "", false
	}
	fine := b[len(strings.TrimRightFunc(b, func(r rune) bool { return unicode.IsPunct(r) && r != '+' })):]
	trSenza := strings.TrimRight(strings.TrimRightFunc(tr, func(r rune) bool { return unicode.IsPunct(r) && r != '+' }), " \u00a0")
	if a == "fr" && (fine == "?" || fine == "!" || fine == ":") {
		fine = "\u00a0" + fine // in francese c'è uno spazio prima di ? ! :
	}
	if strings.ToUpper(b) == b && strings.ToLower(b) != b {
		trSenza = strings.ToUpper(trSenza)
	}
	return trSenza + fine, true
}
