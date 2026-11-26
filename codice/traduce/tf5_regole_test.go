package traduce

// Collaudo finale TF5 (26/11): le regole di Sergio sul traduttore, una prova per regola.

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Regola 1: una scritta tradotta e confermata resta tradotta (sempre con la stessa traduzione)
// per tutta la partita; si azzera solo quando cambia il gioco.
func TestTF5TraduzioneFermaPerTuttaLaPartita(t *testing.T) {
	f := &traduzioniFisse{}
	c := &confermeDecisione{}
	coppia := "en>it"
	f.fissa(coppia, "Options", "Opzioni")
	f.fissa(coppia, "Options", "Impostazioni") // una traduzione diversa arrivata dopo: vince la prima
	for _, letta := range []string{"Options", "OPTIONS", "Options.", "  Options "} {
		r := traduciTutteFisse(context.Background(), nil, []Blocco{{Testo: letta}}, true, f)
		if got := strings.ToLower(r[0].Tradotto); got != "opzioni" {
			t.Errorf("%q: %q, attesa sempre la prima traduzione (Opzioni)", letta, r[0].Tradotto)
		}
	}
	// due parole vere diverse non prendono la traduzione dell'altra
	f.fissa(coppia, "Fail", "Fallito")
	if v, ok := f.cerca(coppia, "Fall"); ok {
		t.Errorf("Fall ha preso la traduzione di Fail: %q", v)
	}
	// confermata da sola in due fotogrammi: la decisione resta anche dopo ore
	b := Blocco{Testo: "Options", Tradotto: "Opzioni", X: 10, Y: 10, W: 80, H: 20}
	t0 := time.Now()
	c.segna(coppia, []Blocco{b}, []Blocco{b}, t0)
	c.segna(coppia, []Blocco{b}, []Blocco{b}, t0.Add(time.Second))
	bl := []Blocco{{Testo: "Options"}}
	segnaGiaMostrate(f, c, coppia, bl, t0.Add(3*time.Hour))
	if !bl[0].GiaMostrata {
		t.Errorf("la decisione confermata è scaduta col tempo: deve restare per tutta la partita")
	}
	// gioco cambiato: via tutto (traduzioni fisse e conferme)
	f.svuota()
	c.svuota()
	if _, ok := f.cerca(coppia, "Options"); ok {
		t.Errorf("traduzione fissa rimasta dopo il cambio di gioco")
	}
	if c.valida(coppia, "Options", t0) {
		t.Errorf("conferma rimasta dopo il cambio di gioco")
	}
	// nel gioco nuovo una sola volta mostrata non basta per tenere la decisione
	f.fissa(coppia, "Options", "Opzioni")
	c.segna(coppia, []Blocco{b}, []Blocco{b}, t0)
	bl = []Blocco{{Testo: "Options"}}
	segnaGiaMostrate(f, c, coppia, bl, t0)
	if bl[0].GiaMostrata {
		t.Errorf("decisione tenuta dopo un solo fotogramma nel gioco nuovo")
	}
}

// Regola 1 (codice Windows): al cambio di gioco si svuotano insieme traduzioni fisse e conferme.
func TestTF5CambioGiocoSvuotaFisseEConferme(t *testing.T) {
	src, err := os.ReadFile("pagina_main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	i := strings.Index(s, "fissePagina.svuota()")
	if i < 0 {
		t.Fatal("fissePagina.svuota() non trovato in avviaPaginaSu")
	}
	blocco := s[max(0, i-300):min(len(s), i+200)]
	if !strings.Contains(blocco, "giocoDelleFisse") || !strings.Contains(blocco, "confermePagina.svuota()") {
		t.Errorf("al cambio di gioco vanno svuotate sia le traduzioni fisse sia le conferme")
	}
	if strings.Count(s, "fissePagina.svuota()") != 1 {
		t.Errorf("le traduzioni fisse si svuotano solo al cambio di gioco")
	}
}

// Regola 2: mai righe mezze inglesi e mezze italiane.
func TestTF5MaiMezzeRighe(t *testing.T) {
	riga := Blocco{Testo: "Fine. I'll ask him. Wait here and don't touch anything.", X: 40, Y: 40, W: 330, H: 18, Righe: 1, Em: 18}
	pezzo := Blocco{Testo: "Wait here", Tradotto: "Aspetta qui", X: 160, Y: 40, W: 60, H: 18, Righe: 1, Em: 18}
	if tenute, tolte := senzaMezzeRighe([]Blocco{pezzo}, []Blocco{riga}); len(tenute) != 0 || len(tolte) != 1 {
		t.Errorf("pezzo tradotto sopra una riga rimasta in inglese")
	}
	// i motivi per cui una scritta resta in inglese con parole di una frase contano come inglese
	for _, m := range []string{"lettura incerta", motivoMezzaRiga, motivoNonCiSta, motivoFraseTroncata} {
		if !ingleseVisibile(m) {
			t.Errorf("%q: la scritta resta in inglese e deve togliere le traduzioni accanto", m)
		}
	}
	// le traduzioni mezze inglesi non si mostrano
	b := Blocco{Testo: "Open the door to continue", X: 10, Y: 10, W: 200, H: 18, Righe: 1}
	if m := motivoScartaTraduzione(b, Risultato{Tradotto: "Open the door to continue"}); m == "" {
		t.Errorf("traduzione uguale all'inglese mostrata")
	}
}

// Regola 3: testi del mondo (titoli dei video, nomi di fantasia, nomi con codice) in inglese, e
// non tolgono le traduzioni accanto (non sono pezzi di una frase).
func TestTF5TestiDelMondoInOriginale(t *testing.T) {
	casi := map[string]string{
		"ONE MORE DONUT - Full Gameplay Walkthrough Part 1 [FULL GAME] No Commentary": motivoTitoloVideo,
		"The Blood of Dawnwalker": motivoNomeGioco,
		"1S4V1 POINT":             motivoNomeCodice,
	}
	for s, atteso := range casi {
		m := motivoNonTradurre(Blocco{Testo: s, X: 10, Y: 10, W: 20 * len(s) / 2, H: 20, Righe: 1, Affid: letturaBuona})
		if m != atteso {
			t.Errorf("%q: motivo %q, atteso %q (resta in originale)", s, m, atteso)
		}
		if ingleseVisibile(m) {
			t.Errorf("%q: un nome non deve togliere le traduzioni accanto", s)
		}
	}
	// la firma o il logo nell'angolo non si tengono mai tradotti, nemmeno se già mostrati
	if motivoTenibile(motivoFirmaAngolo) {
		t.Errorf("firma nell'angolo tenuta tradotta")
	}
}

// Regola 4: i comandi col tasto davanti si traducono e il tasto resta davanti.
func TestTF5TastoDavanti(t *testing.T) {
	casi := map[string]string{
		"X Transfer":   "X Trasferisci",
		"[X] Transfer": "[X] Trasferisci",
		"[E] Use":      "[E] Usa",
		"E Use":        "E Usa",
	}
	for en, atteso := range casi {
		if v, ok := fraseMenuConNumero(en); !ok || v != atteso {
			t.Errorf("%q: %q %v (atteso %q)", en, v, ok, atteso)
		}
	}
}

// Regola 5: le traduzioni dei comandi non escono dal riquadro del pulsante.
func TestTF5DentroIlRiquadro(t *testing.T) {
	for _, c := range []struct{ en, it string }{
		{"Hold", "Tieni premuto"}, {"Tap", "Premere brevemente"}, {"Press", "Premi"},
		{"Hold Shift", "Tieni premuto Shift"}, {"Skip", "Salta la scena introduttiva adesso"},
	} {
		b := etichetta(c.en, 18)
		b.Tradotto = c.it
		if m := adattaAlRiquadro(&b); m == "" && !ciStaNelRiquadro(b, b.Tradotto) {
			t.Errorf("%q → %q: esce dal riquadro", c.en, b.Tradotto)
		}
	}
}

// Regola 6: mai file aggiunti ai giochi con l'anti-imbroglio.
func TestTF5MaiFileNeiGiochiConAntiCheat(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "EasyAntiCheat"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !ConAntiCheat(GiocoRiconosciuto{Nome: "Gioco Qualsiasi", Cartella: dir}) {
		t.Errorf("cartella con EasyAntiCheat non riconosciuta")
	}
	for _, g := range []GiocoRiconosciuto{{Nome: "VALORANT", Piattaforma: "Riot"}, {Nome: "Fortnite"}, {Nome: "Rust"}} {
		if !ConAntiCheat(g) {
			t.Errorf("%s: anti-imbroglio non riconosciuto", g.Nome)
		}
	}
	if ConAntiCheat(GiocoRiconosciuto{Nome: "Trust Me"}) {
		t.Errorf("Trust Me preso per Rust")
	}
	// nel codice: l'unico punto che aggiunge file (applicaModGioco) controlla l'anti-imbroglio
	// prima di tutto, e la mod Unreal non si mette mai con l'anti-imbroglio
	src, err := os.ReadFile("mod_gioco_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	i := strings.Index(s, "func applicaModGioco(")
	if i < 0 {
		t.Fatal("applicaModGioco non trovata")
	}
	corpo := s[i:]
	ac := strings.Index(corpo, "haAntiCheat(cartella)")
	for _, passo := range []string{"applicaModUnreal(", "zipBepInEx", "ModGiaPresenti("} {
		if j := strings.Index(corpo, passo); j < 0 || ac < 0 || ac > j {
			t.Errorf("%s prima del controllo dell'anti-imbroglio", passo)
		}
	}
	if regexp.MustCompile(`applicaModUnreal\([^)]*true`).MatchString(s) {
		t.Errorf("mod Unreal messa con l'anti-imbroglio")
	}
}

// Regola 7: nessun dato esce dal PC. I server di Tasso ascoltano solo su 127.0.0.1; le sole
// richieste in uscita sono GET dei file da scaricare (motori e modelli), senza testi.
func TestTF5NessunDatoEsceDalPC(t *testing.T) {
	file, _ := filepath.Glob("*.go")
	reListen := regexp.MustCompile(`net\.Listen\([^)]*\)`)
	reReq := regexp.MustCompile(`http\.NewRequest(WithContext)?\([^,]*,\s*"([A-Z]+)",\s*([^,]+),`)
	for _, f := range file {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		for _, l := range reListen.FindAllString(s, -1) {
			if !strings.Contains(l, "127.0.0.1") {
				t.Errorf("%s: %s non ascolta solo sul PC", f, l)
			}
		}
		for _, m := range reReq.FindAllStringSubmatch(s, -1) {
			metodo, url := m[2], strings.TrimSpace(m[3])
			if metodo == "GET" {
				continue
			}
			// POST solo ai programmi locali: motore IA (ClientIA.URL = http://127.0.0.1:porta) e
			// riconoscimento della voce (s.url = http://127.0.0.1:porta/chiave)
			locale := (f == "motore_ia.go" && strings.HasPrefix(url, "c.URL")) ||
				(f == "voci_windows.go" && strings.HasPrefix(url, "url+") && strings.Contains(s, `s.url = fmt.Sprintf("http://127.0.0.1:`))
			if !locale {
				t.Errorf("%s: richiesta %s a %s", f, metodo, url)
			}
		}
		if strings.Contains(s, "http.Post(") || strings.Contains(s, "http.PostForm(") {
			t.Errorf("%s: http.Post", f)
		}
	}
}
