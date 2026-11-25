//go:build windows && collaudo

package traduce

// Prova della qualità per ogni lingua, con le frasi FLORES-200 (devtest, CC-BY-SA 4.0: testi
// generali di Wikipedia, NON di giochi): stesse frasi in tutte le lingue, traduzione umana
// italiana come riferimento. Misura chrF++ (come per le 1000 frasi) e il tempo a frase.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	provaLingueDir    string
	provaLingueUscita string
	provaLingueFrasi  = 200
)

var floresCodici = map[string]string{
	"en": "eng_Latn", "it": "ita_Latn", "fr": "fra_Latn", "de": "deu_Latn", "es": "spa_Latn", "pt": "por_Latn",
	"pl": "pol_Latn", "ru": "rus_Cyrl", "ja": "jpn_Jpan", "ko": "kor_Hang", "zh": "zho_Hans", "zt": "zho_Hant",
	"da": "dan_Latn", "fi": "fin_Latn", "nb": "nob_Latn", "sv": "swe_Latn", "tr": "tur_Latn",
}

func init() {
	for i, a := range os.Args {
		if i+1 >= len(os.Args) {
			break
		}
		switch a {
		case "--prova-lingue":
			provaLingueDir = os.Args[i+1]
		case "--uscita-lingue":
			provaLingueUscita = os.Args[i+1]
		case "--frasi-lingue":
			fmt.Sscan(os.Args[i+1], &provaLingueFrasi)
		}
	}
}

func provaLingueSeServe() bool {
	if provaLingueDir == "" {
		return false
	}
	go eseguiProvaLingue()
	return true
}

func leggiFlores(codice string, n int) []string {
	f, err := os.Open(filepath.Join(provaLingueDir, floresCodici[codice]+".devtest"))
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() && len(out) < n {
		out = append(out, strings.TrimSpace(sc.Text()))
	}
	return out
}

type esitoCoppia struct {
	Da, A      string
	ChrF       float64
	MsAFrase   float64
	Errori     int
	Esempi     [][3]string
	Scaricati  bool
	ErroreScar string
}

func eseguiProvaLingue() {
	defer guard("prova-lingue")
	defer func() { wv.Dispatch(func() { wv.Terminate() }) }()
	_ = os.MkdirAll(provaLingueUscita, 0o755)
	var log []string
	nota := func(s string) {
		log = append(log, time.Now().Format("15:04:05")+" "+s)
		_ = os.WriteFile(filepath.Join(provaLingueUscita, "registro.txt"), []byte(strings.Join(log, "\r\n")+"\r\n"), 0o644)
	}
	coppie := [][2]string{{"en", "it"}}
	for _, l := range lingueGioco {
		if l.Codice != "en" && l.Codice != "it" {
			coppie = append(coppie, [2]string{l.Codice, "it"})
		}
	}
	// qualche coppia verso altre lingue (l'esempio di Sergio: portoghese → francese)
	coppie = append(coppie, [2]string{"pt", "fr"}, [2]string{"en", "fr"}, [2]string{"en", "de"}, [2]string{"en", "es"}, [2]string{"de", "fr"}, [2]string{"ja", "en"}, [2]string{"ru", "en"}, [2]string{"fr", "en"}, [2]string{"it", "fr"})
	// 1) scarico tutto quello che serve
	for _, c := range coppie {
		if coppiaPronta(c[0], c[1]) {
			continue
		}
		t0 := time.Now()
		err := scaricaLingue(context.Background(), c[0], c[1], func(int64, int64) {})
		nota(fmt.Sprintf("scaricato %s→%s in %.0f s, errore: %v", c[0], c[1], time.Since(t0).Seconds(), err))
	}
	ricaricaMozilla()
	time.Sleep(8 * time.Second)
	var esiti []esitoCoppia
	for _, c := range coppie {
		src, ref := leggiFlores(c[0], provaLingueFrasi), leggiFlores(c[1], provaLingueFrasi)
		e := esitoCoppia{Da: c[0], A: c[1], Scaricati: coppiaPronta(c[0], c[1])}
		if len(src) == 0 || len(src) != len(ref) || !e.Scaricati {
			e.ErroreScar = "frasi o modelli mancanti"
			esiti = append(esiti, e)
			nota(fmt.Sprintf("%s→%s: saltata (%s)", c[0], c[1], e.ErroreScar))
			continue
		}
		// traduttori nuovi per ogni coppia: tengono in memoria solo i modelli che servono
		ricaricaMozilla()
		time.Sleep(6 * time.Second)
		hyp := make([]string, len(src))
		var errori int
		primoErrore := ""
		var mu sync.Mutex
		var wg sync.WaitGroup
		posti := make(chan struct{}, 8)
		t0 := time.Now()
		ctx := conLotto(context.Background())
		for i := range src {
			wg.Add(1)
			posti <- struct{}{}
			go func(i int) {
				defer func() { <-posti; wg.Done() }()
				out, err := moz.TraduciCoppia(ctx, src[i], c[0], c[1])
				if err != nil {
					mu.Lock()
					errori++
					if primoErrore == "" {
						primoErrore = err.Error()
					}
					mu.Unlock()
				}
				hyp[i] = out
			}(i)
		}
		wg.Wait()
		e.MsAFrase = float64(time.Since(t0).Milliseconds()) / float64(len(src))
		e.ChrF = chrFpp(hyp, ref)
		e.Errori = errori
		for i := 0; i < 3 && i < len(src); i++ {
			e.Esempi = append(e.Esempi, [3]string{src[i], hyp[i], ref[i]})
		}
		esiti = append(esiti, e)
		e.ErroreScar = primoErrore
		// tutte le frasi, per provare dopo il correttore IA sulle stesse uscite di Mozilla
		var righe []string
		for i := range src {
			r, _ := json.Marshal(map[string]any{"id": i, "da": c[0], "a": c[1], "src": src[i], "moz": hyp[i], "ref": ref[i]})
			righe = append(righe, string(r))
		}
		_ = os.WriteFile(filepath.Join(provaLingueUscita, "frasi-"+c[0]+"-"+c[1]+".jsonl"), []byte(strings.Join(righe, "\n")+"\n"), 0o644)
		nota(fmt.Sprintf("%s→%s: chrF++ %.1f, %.0f ms a frase (in gruppo), errori %d %s", c[0], c[1], e.ChrF, e.MsAFrase, errori, primoErrore))
	}
	b, _ := json.MarshalIndent(esiti, "", " ")
	_ = os.WriteFile(filepath.Join(provaLingueUscita, "esiti.json"), b, 0o644)
	// riassunto in ordine di qualità
	ord := append([]esitoCoppia(nil), esiti...)
	sort.Slice(ord, func(i, j int) bool { return ord[i].ChrF > ord[j].ChrF })
	var md strings.Builder
	md.WriteString(fmt.Sprintf("# Qualità per lingua (FLORES-200 devtest, prime %d frasi, chrF++)\n\n| Coppia | chrF++ | ms a frase |\n|---|---|---|\n", provaLingueFrasi))
	for _, e := range ord {
		md.WriteString(fmt.Sprintf("| %s → %s | %.1f | %.0f |\n", nomeLingua(e.Da), nomeLingua(e.A), e.ChrF, e.MsAFrase))
	}
	_ = os.WriteFile(filepath.Join(provaLingueUscita, "qualita-lingue.md"), []byte(md.String()), 0o644)
	nota("fine")
}
