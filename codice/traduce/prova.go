package traduce

// Prova delle 100 frasi: misura qualità (chrF++) e velocità di un motore con le stesse
// istruzioni e la stessa pulizia usate dal programma vero. Le frasi sono originali,
// scritte apposta, con traduzione di riferimento; non vengono mai usate per addestrare.

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"
)

//go:embed prova/test_1000.jsonl
var frasiProva string

// Altre 1000 frasi (id 1001-2000), stesso formato: si usano solo con TASSO_FRASI_PROVA=2000.
//
//go:embed prova/test_2000_extra.jsonl
var frasiProvaExtra string

//go:embed prova/mozilla_1000.json
var risultatiMozilla string

type FraseProva struct {
	ID     int    `json:"id"`
	Genere string `json:"genere"`
	EN     string `json:"en"`
	IT     string `json:"it"`
}

type EsitoFrase struct {
	FraseProva
	Uscita string  `json:"uscita"`
	Ms     float64 `json:"ms"`
}

type EsitoProva struct {
	Motore  string       `json:"motore"`
	ChrF    float64      `json:"chrf"`
	Mediana float64      `json:"mediana_ms"`
	P90     float64      `json:"p90_ms"`
	Massimo float64      `json:"max_ms"`
	Frasi   []EsitoFrase `json:"frasi"`
	Data    string       `json:"data"`
	Note    string       `json:"note"`
}

// caricaFrasiProva: le 1000 frasi di sempre; con la variabile d'ambiente TASSO_FRASI_PROVA=2000
// anche le 1000 in più di prova/test_2000_extra.jsonl (in coda, id 1001-2000). La metà "mai vista"
// resta quella con id pari, come per le prime 1000.
func caricaFrasiProva() ([]FraseProva, error) {
	testo := frasiProva
	if os.Getenv("TASSO_FRASI_PROVA") == "2000" {
		testo += "\n" + frasiProvaExtra
	}
	var out []FraseProva
	sc := bufio.NewScanner(strings.NewReader(testo))
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var f FraseProva
		if err := json.Unmarshal([]byte(sc.Text()), &f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, sc.Err()
}

// eseguiProva traduce le frasi una per una, con il contesto delle precedenti dello stesso genere
// (come in un dialogo vero).
func eseguiProva(ctx context.Context, m Motore, frasi []FraseProva) EsitoProva {
	es := EsitoProva{Motore: m.Nome(), Data: time.Now().Format("02/01/2006 15:04")}
	var ctxPrec []string
	genere := ""
	g := glossarioBase()
	for _, f := range frasi {
		if f.Genere != genere {
			ctxPrec, genere = nil, f.Genere
		}
		parlante, battuta := separaParlante(pulisci(f.EN))
		req := RichiestaTraduzione{Testo: battuta, Parlante: parlante, Contesto: ctxPrec, Glossario: g.trova(battuta)}
		t0 := time.Now()
		out, err := m.Traduci(ctx, req)
		ms := float64(time.Since(t0).Microseconds()) / 1000
		if err != nil {
			out = "[errore: " + err.Error() + "]"
		}
		if parlante != "" {
			out = parlante + ": " + out
		}
		es.Frasi = append(es.Frasi, EsitoFrase{FraseProva: f, Uscita: out, Ms: ms})
		ctxPrec = append(ctxPrec, battuta)
		if len(ctxPrec) > maxContesto {
			ctxPrec = ctxPrec[1:]
		}
	}
	calcolaPunteggi(&es)
	return es
}

func calcolaPunteggi(es *EsitoProva) {
	var hyp, ref []string
	var tempi []float64
	for _, f := range es.Frasi {
		hyp = append(hyp, f.Uscita)
		ref = append(ref, f.IT)
		tempi = append(tempi, f.Ms)
	}
	es.ChrF = chrFpp(hyp, ref)
	sort.Float64s(tempi)
	if n := len(tempi); n > 0 {
		es.Mediana, es.P90, es.Massimo = tempi[n/2], tempi[n*9/10], tempi[n-1]
	}
}

// chrFpp: chrF++ a livello di corpus (n-grammi di caratteri 1-6 e di parole 1-2, beta=2),
// come sacrebleu (word_order=2): 0-100, più alto è meglio.
func chrFpp(hyp, ref []string) float64 {
	const charN, wordN, beta = 6, 2, 2.0
	n := charN + wordN
	match := make([]float64, n)
	totH := make([]float64, n)
	totR := make([]float64, n)
	for i := range hyp {
		hc, rc := senzaSpazi(hyp[i]), senzaSpazi(ref[i])
		for k := 1; k <= charN; k++ {
			m, th, tr := confronta(ngrammi(hc, k), ngrammi(rc, k))
			match[k-1] += m
			totH[k-1] += th
			totR[k-1] += tr
		}
		hw, rw := paroleSacre(hyp[i]), paroleSacre(ref[i])
		for k := 1; k <= wordN; k++ {
			m, th, tr := confronta(ngrammiParole(hw, k), ngrammiParole(rw, k))
			match[charN+k-1] += m
			totH[charN+k-1] += th
			totR[charN+k-1] += tr
		}
	}
	var p, r float64
	cnt := 0
	for k := 0; k < n; k++ {
		if totH[k] > 0 && totR[k] > 0 {
			p += match[k] / totH[k]
			r += match[k] / totR[k]
		}
		cnt++
	}
	p /= float64(cnt)
	r /= float64(cnt)
	if p+r == 0 {
		return 0
	}
	b2 := beta * beta
	return 100 * (1 + b2) * p * r / (b2*p + r)
}

func senzaSpazi(s string) []rune {
	var out []rune
	for _, c := range s {
		if !unicode.IsSpace(c) {
			out = append(out, c)
		}
	}
	return out
}

func ngrammi(r []rune, k int) map[string]float64 {
	m := map[string]float64{}
	for i := 0; i+k <= len(r); i++ {
		m[string(r[i:i+k])]++
	}
	return m
}

// paroleSacre: come sacrebleu, la punteggiatura a inizio/fine parola diventa una parola a sé
func paroleSacre(s string) []string {
	var out []string
	for _, w := range strings.Fields(s) {
		rs := []rune(w)
		var pre, post []string
		for len(rs) > 1 && unicode.IsPunct(rs[0]) {
			pre = append(pre, string(rs[0]))
			rs = rs[1:]
		}
		for len(rs) > 1 && unicode.IsPunct(rs[len(rs)-1]) {
			post = append([]string{string(rs[len(rs)-1])}, post...)
			rs = rs[:len(rs)-1]
		}
		out = append(out, pre...)
		out = append(out, string(rs))
		out = append(out, post...)
	}
	return out
}

func ngrammiParole(w []string, k int) map[string]float64 {
	m := map[string]float64{}
	for i := 0; i+k <= len(w); i++ {
		m[strings.Join(w[i:i+k], " ")]++
	}
	return m
}

func confronta(h, r map[string]float64) (m, th, tr float64) {
	for g, c := range h {
		th += c
		if rc, ok := r[g]; ok {
			m += math.Min(c, rc)
		}
	}
	for _, c := range r {
		tr += c
	}
	return
}

// rapportoProva: tabella in Markdown con tutti i motori affiancati.
func rapportoProva(esiti []EsitoProva) string {
	var b strings.Builder
	rif := esiti[0] // l'esito con più frasi (con 2000 frasi Mozilla registrato ne ha solo 1000)
	for _, e := range esiti {
		if len(e.Frasi) > len(rif.Frasi) {
			rif = e
		}
	}
	b.WriteString(fmt.Sprintf("# Prova delle %d frasi\n\n", len(rif.Frasi)))
	b.WriteString("Qualità chrF++ (0-100, più alto è meglio) e tempo di traduzione per frase.\n\n")
	b.WriteString("| Motore | Qualità chrF++ | Mediana | 90% sotto | Massimo |\n|---|---|---|---|---|\n")
	for _, e := range esiti {
		fmt.Fprintf(&b, "| %s | %.1f | %.0f ms | %.0f ms | %.0f ms |\n", e.Motore, e.ChrF, e.Mediana, e.P90, e.Massimo)
	}
	b.WriteString("\n## Frase per frase\n\n| # | Inglese | Riferimento |")
	for _, e := range esiti {
		b.WriteString(" " + e.Motore + " |")
	}
	b.WriteString("\n|---|---|---|")
	for range esiti {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	if len(esiti) == 0 {
		return b.String()
	}
	for i, f := range rif.Frasi {
		fmt.Fprintf(&b, "| %d | %s | %s |", f.ID, f.EN, f.IT)
		for _, e := range esiti {
			if i < len(e.Frasi) {
				b.WriteString(" " + strings.ReplaceAll(e.Frasi[i].Uscita, "\n", " ") + " |")
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// esitoMozillaRegistrato: la prova del motore Mozilla fatta il 7/11/2026 (in Chromium).
func esitoMozillaRegistrato() (EsitoProva, error) {
	var r struct {
		Out []struct {
			ID     int     `json:"id"`
			Genere string  `json:"genere"`
			EN     string  `json:"en"`
			Ref    string  `json:"ref"`
			Hyp    string  `json:"hyp"`
			Ms     float64 `json:"ms"`
		} `json:"out"`
	}
	if err := json.Unmarshal([]byte(risultatiMozilla), &r); err != nil {
		return EsitoProva{}, err
	}
	es := EsitoProva{Motore: "Mozilla (veloce)", Data: "07/11/2026", Note: "misurato in Chromium su server Linux a 4 core"}
	for _, o := range r.Out {
		es.Frasi = append(es.Frasi, EsitoFrase{FraseProva: FraseProva{o.ID, o.Genere, o.EN, o.Ref}, Uscita: o.Hyp, Ms: o.Ms})
	}
	calcolaPunteggi(&es)
	return es, nil
}

// eseguiCorrezione: modo correttore. Parte dalle traduzioni di Mozilla e chiede all'IA di correggerle.
// soloDubbie = true: l'IA guarda solo le frasi che sembraStrana segnala (come farà nel programma).
func eseguiCorrezione(ctx context.Context, m Motore, base EsitoProva, soloDubbie bool) EsitoProva {
	es := EsitoProva{Motore: m.Nome(), Data: time.Now().Format("02/01/2006 15:04")}
	g := glossarioBase()
	guardate, cambiate, scartate := 0, 0, 0
	for _, f := range base.Frasi {
		parlante, battuta := separaParlante(pulisci(f.EN))
		bozza := strings.TrimSpace(f.Uscita)
		if parlante != "" {
			bozza = strings.TrimSpace(strings.TrimPrefix(bozza, parlante+":"))
		}
		out, ms := bozza, 0.0
		if !soloDubbie || sembraStrana(battuta, bozza) != "" {
			guardate++
			t0 := time.Now()
			corr, err := m.Traduci(ctx, RichiestaTraduzione{Testo: battuta, Glossario: g.trova(battuta), Bozza: bozza})
			ms = float64(time.Since(t0).Microseconds()) / 1000
			switch {
			case err != nil || !correzioneAccettabile(battuta, bozza, corr):
				scartate++
			case corr != bozza:
				cambiate++
				out = corr
			}
		}
		if parlante != "" {
			out = parlante + ": " + out
		}
		es.Frasi = append(es.Frasi, EsitoFrase{FraseProva: f.FraseProva, Uscita: out, Ms: ms})
	}
	calcolaPunteggi(&es)
	es.Note = fmt.Sprintf("guardate %d, cambiate %d, correzioni scartate %d", guardate, cambiate, scartate)
	return es
}
