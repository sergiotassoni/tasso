package traduce

import "testing"

func TestSceltaCorrettore(t *testing.T) {
	casi := []struct {
		mem, usata, ram float64
		core            int
		vuole           string
	}{
		{12, 2, 48, 16, "scheda"}, // PC di Sergio: IA intera, 5,2 GB su 12 (regola 23/11)
		{12, 6.5, 48, 16, "meta"}, // PC di Sergio, gioco pesante
		{12, 9, 48, 16, ""},       // gioco pesantissimo: solo Mozilla
		{8, 1, 16, 8, ""},         // 8 GB: anche metà IA supera il limite (regola 23/11)
		{8, 4, 16, 4, ""},         // processore piccolo: niente lavoro sul processore
		{8, 1, 8, 8, ""},          // 8 GB di RAM: solo Mozilla
		{4, 0, 32, 12, ""},        // scheda da 4 GB: troppo piccola
	}
	for _, c := range casi {
		if p := sceltaCorrettore(InfoGPU{MemoriaGB: c.mem, UsataGB: c.usata}, c.ram, c.core); p.Nome != c.vuole {
			t.Errorf("%+v: %q, voleva %q", c, p.Nome, c.vuole)
		}
	}
}

func TestCorrezioneAccettabile(t *testing.T) {
	en := "Press E to open the door."
	bozza := "Premi E per aprire la porta."
	if !correzioneAccettabile(en, bozza, "Premi E per aprire la porta.") {
		t.Error("uguale deve andare")
	}
	if correzioneAccettabile(en, bozza, "Premi E per aprire la porta. Poi corri via veloce verso la nave del capitano che ti aspetta.") {
		t.Error("invenzione lunga va scartata")
	}
	if correzioneAccettabile(en, bozza, "") {
		t.Error("vuota va scartata")
	}
}

func TestPromptCorrettoreSenzaContesto(t *testing.T) {
	m := costruisciMessaggi(RichiestaTraduzione{Testo: "Hi", Bozza: "Ciao", Contesto: []string{"vecchia"}})
	if m[0].Content != promptCorrettore {
		t.Fatal("prompt sbagliato")
	}
	if contiene(m[1].Content, "vecchia") {
		t.Error("il correttore non deve vedere le battute precedenti")
	}
}

func contiene(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestConsiglioPC(t *testing.T) {
	sergio := consiglioPC(InfoGPU{Nome: "AMD Radeon RX 6700 XT", MemoriaGB: 12, UsataGB: 1}, 48, 16, true)
	if sergio.Righe[len(sergio.Righe)-1].Esito != "si" {
		t.Errorf("PC di Sergio: %+v", sergio)
	}
	vecchio := consiglioPC(InfoGPU{}, 4, 2, true)
	if vecchio.Righe[len(vecchio.Righe)-1].Esito != "no" {
		t.Errorf("PC vecchio: %+v", vecchio)
	}
	medio := consiglioPC(InfoGPU{Nome: "RTX 4060", MemoriaGB: 8, UsataGB: 1}, 16, 8, false)
	if medio.Righe[len(medio.Righe)-1].Esito != "no" { // regola 23/11: limite di memoria per l'IA
		t.Errorf("PC medio: %+v", medio)
	}
	debole := consiglioPC(InfoGPU{Nome: "RTX 3050", MemoriaGB: 8}, 8, 4, true)
	if debole.Righe[len(debole.Righe)-1].Esito != "no" {
		t.Errorf("PC medio: %+v", medio)
	}
}

func TestCopiaInglese(t *testing.T) {
	en := "Final whistle! We're through to the final!"
	if !copiaInglese(en, "Fischio finale! Siamo fino in finale!", "Final whistle! Siamo arrivati alla finale!") {
		t.Error("doveva scartare l'inglese ricopiato")
	}
	if copiaInglese("He's tired. Sub him.", "E' stanco. Sotto di lui.", "È stanco. Sostituiscilo.") {
		t.Error("correzione buona scartata")
	}
}

func TestRipetitivaScartata(t *testing.T) {
	en := "Press E to open the door."
	bozza := "Premi E per aprire la porta."
	if correzioneAccettabile(en, bozza, "Premi porta porta porta porta.") {
		t.Error("risposta incantata accettata")
	}
}

func TestStratiSchedaDa12(t *testing.T) {
	p := sceltaCorrettore(InfoGPU{MemoriaGB: 12, UsataGB: 2}, 48, 16)
	if p.Nome != "scheda" || p.MemoriaGB > 12*0.44 {
		t.Errorf("scheda da 12: %+v", p)
	}
	// gioco pesante (6,5 GB usati): ci stanno meno strati
	q := sceltaCorrettore(InfoGPU{MemoriaGB: 12, UsataGB: 6.5}, 48, 16)
	if q.Nome != "meta" || q.Strati != 33 {
		t.Errorf("gioco pesante: %+v", q)
	}
}

func TestScorta(t *testing.T) {
	conScorta = true
	defer func() { conScorta = false }()
	if p := sceltaCorrettore(InfoGPU{MemoriaGB: 12, UsataGB: 2}, 48, 16); p.Nome != "scheda" {
		t.Errorf("c'è posto: IA grande, non %+v", p)
	}
	if p := sceltaCorrettore(InfoGPU{MemoriaGB: 12, UsataGB: 7.5}, 48, 16); p.Nome != "piccola" || p.Strati != stratiPiccola {
		t.Errorf("gioco pesante: IA piccola, non %+v", p)
	}
	if p := sceltaCorrettore(InfoGPU{MemoriaGB: 12, UsataGB: 7.5}, 8, 4); p.Nome == "piccola" {
		t.Errorf("PC debole: la piccola lavora anche col processore, non va bene: %+v", p)
	}
	if p := sceltaCorrettore(InfoGPU{MemoriaGB: 8, UsataGB: 2}, 16, 8); p.Nome != "piccola" {
		t.Errorf("scheda da 8: IA piccola, non %+v", p)
	}
	if p := sceltaCorrettore(InfoGPU{MemoriaGB: 12, UsataGB: 10}, 48, 16); p.Nome != "" {
		t.Errorf("Call of Duty: solo Mozilla, non %+v", p)
	}
}

func TestMedia(t *testing.T) {
	conScorta, conMedia = true, true
	defer func() { conScorta, conMedia = false, false }()
	if p := sceltaCorrettore(InfoGPU{MemoriaGB: 12, UsataGB: 5.8}, 48, 16); p.Nome != "media" {
		t.Errorf("5,8 GB usati: IA compressa, non %+v", p)
	}
	if p := sceltaCorrettore(InfoGPU{MemoriaGB: 12, UsataGB: 7.5}, 48, 16); p.Nome != "piccola" {
		t.Errorf("7,5 GB usati: IA piccola, non %+v", p)
	}
}

func TestLivelloScelto(t *testing.T) {
	conScorta, conMedia = true, true
	defer func() { conScorta, conMedia = false, false }()
	g := InfoGPU{MemoriaGB: 12, UsataGB: 2}
	for liv, vuole := range map[string]string{"auto": "scheda", "grande": "scheda", "media": "media", "piccola": "piccola"} {
		if p := sceltaLivello(g, 48, 16, liv); p.Nome != vuole {
			t.Errorf("%s: %+v", liv, p)
		}
	}
	// media scelta ma il gioco pesa: scende alla piccola
	if p := sceltaLivello(InfoGPU{MemoriaGB: 12, UsataGB: 7}, 48, 16, "media"); p.Nome != "piccola" {
		t.Errorf("media con gioco pesante: %+v", p)
	}
}

func TestPiccolaSiStringe(t *testing.T) {
	conScorta, conMedia = true, true
	defer func() { conScorta, conMedia = false, false }()
	// Liar's Bar: 8,5 GB usati su 12 → 3,5 liberi
	p := sceltaLivello(InfoGPU{MemoriaGB: 12, UsataGB: 8.5}, 48, 16, "auto")
	if p.Nome != "piccola" || p.Strati >= stratiPiccola || p.Strati < stratiPiccolaMin || p.MemoriaGB+margineGiocoGB > 3.5+1e-9 {
		t.Fatalf("gioco pesante: %+v", p)
	}
	// Call of Duty: 10 GB usati → niente IA
	if p := sceltaLivello(InfoGPU{MemoriaGB: 12, UsataGB: 10}, 48, 16, "auto"); p.Nome != "" {
		t.Fatalf("troppo pesante: %+v", p)
	}
}

// Prima di scaricare l'IA, una scheda da 8 GB non deve sentirsi dire "non adatta" se dopo lo
// scaricamento userebbe davvero l'IA piccola.
func TestSceltaPrevista(t *testing.T) {
	vecchio := iaMancante
	defer func() { iaMancante = vecchio }()
	iaMancante = func() bool { return true }
	for _, c := range []struct{ mem, ram float64 }{{8, 16}, {8, 32}, {6, 16}} {
		if p := sceltaPrevista(InfoGPU{MemoriaGB: c.mem}, c.ram, 8); p.Nome != "piccola" {
			t.Errorf("%.0f GB scheda, %.0f GB RAM: prima di scaricare vuole piccola, ho %q (%s)", c.mem, c.ram, p.Nome, p.Motivo)
		}
	}
	iaMancante = func() bool { return false } // scaricata ma senza la Media: niente promesse
	if p := sceltaPrevista(InfoGPU{MemoriaGB: 8}, 16, 8); p.Nome != "" {
		t.Errorf("senza Media vuole \"\", ho %q", p.Nome)
	}
}
