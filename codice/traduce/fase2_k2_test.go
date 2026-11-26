package traduce

// Prove delle correzioni di qualità dopo i banchi del 26/11 (12:56 e 15:56): parole inventate
// del traduttore veloce, obiettivi tradotti male, correzioni dell'OCR che aggiungevano parole.

import "testing"

func TestK2ParoleInventateViste(t *testing.T) {
	inventate := [][2]string{
		{"Depth 13 / 13 m", "Profodita 13 / 13 m"},
		{"Sounds like it's coming from Ms. Toriumi's class.", "Sembra che provena dalla classe della signora Toriumi."},
		{"ESCAPE THE CAMP", "FUGGISCI NEL CAMPO"},
	}
	for _, c := range inventate {
		if si, w := traduzioneInventata(c[0], c[1]); !si {
			t.Errorf("%q: doveva essere scartata (parole %v)", c[1], w)
		}
	}
	buone := [][2]string{
		{"METEORIC IRON", "FERRO METEORICO"},
		{"Garden gnome", "Nano da giardino"},
		{"Garden gnome", "Gnomo da giardino"},
		{"PLASMA CUTTER", "TAGLIERINA AL PLASMA"},
		{"COMPACTOR", "COMPATTATORE"},
		{"Get the detonator", "Prendi il detonatore"},
		{"Depth 13 / 13 m", "Profondità 13 / 13 m"},
		{"Where does it come from?", "Da dove proviene?"},
		{"Run!", "Fuggi!"},
		{"They flee", "Fuggono"},
		{"I come with you", "Vengo con te"},
		{"It must come from the river", "Deve provenire dal fiume"},
		{"He finishes", "Lui finisce"},
	}
	for _, c := range buone {
		if si, w := traduzioneInventata(c[0], c[1]); si {
			t.Errorf("%q: è italiano giusto, scartata per %v", c[1], w)
		}
	}
	for _, w := range []string{"fuggisci", "fuggiscono", "provena", "partisce", "dormisci"} {
		if parolaItaliana(w) {
			t.Errorf("%q non è italiano", w)
		}
	}
	for _, w := range []string{"fuggi", "fuggono", "provenga", "vengo", "finisci", "capisce", "pulisce", "parte", "dormi", "vena"} {
		if !parolaItaliana(w) {
			t.Errorf("%q è italiano", w)
		}
	}
}

func TestK2TerminiObiettivi(t *testing.T) {
	casi := []struct{ en, it, atteso string }{
		{"Head to the faculty office", "Testa all'ufficio facoltà", "Vai all'ufficio facoltà"},
		{"EO [f Head to the faculty office J", "EO [f Testa all'ufficio facoltà J", "EO [f Vai all'ufficio facoltà J"},
		{"Head back to the camp", "Testa al campo", "Vai al campo"},
		{"HEAD TO THE SHIP", "TESTA ALLA NAVE", "VAI ALLA NAVE"},
		{"Walk your siblings to the river •", "Cammina i tuoi fratelli fino al fiume •", "Accompagna i tuoi fratelli fino al fiume •"},
		{"Walk the dog to the park", "Cammina il cane fino al parco", "Accompagna il cane fino al parco"},
		{"ESCAPE THE CAMP", "FUGGISCI NEL CAMPO", "FUGGI DAL CAMPO"},
		{"Escape the prison", "Scappa nella prigione", "Scappa dalla prigione"},
		{"CARGO HOLD", "TENUTA DI CARICO", "STIVA"},
		{"Go to the cargo hold", "Vai alla tenuta di carico", "Vai alla stiva"},
		{"—IBS m' this shift", "—IBS m' questo cambiamento", "—IBS m' questo turno"},
		// qui non si tocca niente
		{"He was shot in the head to make sure", "Gli hanno sparato alla testa per essere sicuri", "Gli hanno sparato alla testa per essere sicuri"},
		{"From head to toe", "Dalla testa ai piedi", "Dalla testa ai piedi"},
		{"Walk to the river", "Cammina fino al fiume", "Cammina fino al fiume"},
		{"Escape into the forest", "Fuggi nella foresta", "Fuggi nella foresta"},
		{"Things change", "Le cose cambiano: questo cambiamento è grande", "Le cose cambiano: questo cambiamento è grande"},
	}
	for _, c := range casi {
		if got := correggiTerminiFrase(c.en, c.it); got != c.atteso {
			t.Errorf("%q: %q, volevo %q", c.en, got, c.atteso)
		}
	}
}

func TestK2VociConMisura(t *testing.T) {
	for in, atteso := range map[string]string{
		"Depth 13 / 13 m": "Profondità 13 / 13 m",
		"Depth 5/ 13 m":   "Profondità 5/ 13 m",
		"Depth 250 m":     "Profondità 250 m",
	} {
		if got, ok := fraseMenuConNumero(in); !ok || got != atteso {
			t.Errorf("%q: %q %v, volevo %q", in, got, ok, atteso)
		}
	}
	for _, s := range []string{"Save the captives", "Escape the camp", "Get the detonator", "Defeat the soldiers"} {
		if _, ok := fraseMenuConNumero(s); !ok {
			t.Errorf("%q: manca la frase pronta", s)
		}
	}
}

func TestK2CorrezioniOCR(t *testing.T) {
	// la parola spezzata con le lettere sottili perse diventa la parola vera, non "cap yes"
	if got := correggiParole("Save the cap ves"); got != "Save the captives" {
		t.Errorf("cap ves: %q", got)
	}
	// le contrazioni e il parlato non si dividono
	for _, s := range []string{"It's real addicting, ain't it?", "It's pretty tough bein' the new kid", "What? I was just bein' friendly."} {
		if got := separaAttaccate(s); got != s {
			t.Errorf("%q diviso in %q", s, got)
		}
	}
	// la rilettura che aggiunge solo paroline o parole a un prezzo non vale
	if letturaPiuCompleta("Oh, by", "Oh, by on this") {
		t.Error(`"Oh, by" -> "Oh, by on this" accettata`)
	}
	if letturaPiuCompleta("$600", "$600 one") {
		t.Error(`"$600" -> "$600 one" accettata`)
	}
	if !letturaPiuCompleta("There's a crowd forming in", "There's a crowd forming in front of the bulletin board...") {
		t.Error("il seguito vero della frase deve valere")
	}
	// Esc senza la E
	if got, _ := correggiRigaOCR("SC back"); got != "ESC back" {
		t.Errorf("SC back: %q", got)
	}
	// riga storpiata: una lettera sola in mezzo e una parola inventata
	if _, v := correggiRigaOCR("M16 Handler: How an by whom s what needy rhelp figuring out."); v != letturaDubbia {
		t.Errorf("riga storpiata tradotta (voto %d)", v)
	}
	if _, v := correggiRigaOCR("I think it's a good idea to go there first."); v == letturaDubbia {
		t.Error("riga buona scartata")
	}
}
