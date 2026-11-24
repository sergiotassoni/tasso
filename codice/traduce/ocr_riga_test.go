package traduce

import (
	"strings"
	"testing"
)

// Righe lette male dall'OCR in giochi diversi: correzione con vocabolario, glossario e frasi
// pronte. Se la lettura resta incerta la riga NON si traduce.
func TestCorreggiRigaOCR(t *testing.T) {
	casi := []struct {
		letto, atteso string
		voto          int // voto minimo (o esatto se dubbia)
	}{
		// frasi pronte agganciate nonostante gli errori
		{"Curry thingthing to fill the progress bur", "Carry that thing to fill the progress bar", letturaCatalog},
		{"Move to fill the progress bur", "Move to fill the progress bar", letturaCatalog},
		{"Sneak to fill the progress bur", "Sneak to fill the progress bar", letturaCatalog},
		{"Press CO NTROL to sneak. Monsters are less likely to see you.", "Press CONTROL to sneak. Monsters are less likely to see you.", letturaCatalog},
		{"HoldtONTROL and use WASD until the progess is filled!", "Hold CONTROL and use WASD until the progess is filled!", letturaCatalog},
		{"UBIIC GAME", "PUBLIC GAME", letturaCatalog},
		{"PUBIIC GAME", "PUBLIC GAME", letturaCatalog},
		// altri giochi: menu e suggerimenti comuni
		{"Prcss any kev to continue", "Press any key to continue", letturaCatalog},
		{"Rcsume Game", "Resume game", letturaCatalog},
		{"SETTlNGS", "SETTINGS", letturaCatalog},
		{"Quit to Desktcp", "Quit to Desktop", letturaCatalog},
		{"S E T T I N G S", "SETTINGS", letturaCatalog},
		// parole vere: si traduce
		{"The bridge collapsed behind you", "The bridge collapsed behind you", letturaBuona},
		{"Return the stolen relic to Kael", "Return the stolen relic to Kael", letturaBuona},
		// spazzatura: non si traduce
		{"FELS", "", letturaDubbia},
		{"GIOCO I.JBIIC", "", letturaDubbia},
		{"trithutthing", "", letturaDubbia},
		{"Xqwv zzkt prrn", "", letturaDubbia},
		{"|||  ~~", "", letturaDubbia},
	}
	for _, c := range casi {
		got, voto := correggiRigaOCR(c.letto)
		if c.voto == letturaDubbia {
			if voto != letturaDubbia {
				t.Errorf("%q: doveva restare dubbia, invece %q voto %d", c.letto, got, voto)
			}
			continue
		}
		if voto < c.voto || !strings.EqualFold(got, c.atteso) {
			t.Errorf("%q -> %q voto %d, atteso %q voto >= %d", c.letto, got, voto, c.atteso, c.voto)
		}
	}
}

// Il numero non si cambia mai per agganciare una frase pronta.
func TestAgganciaNonCambiaNumeri(t *testing.T) {
	if f, _, ok := agganciaCatalogo("Render Distance: 13", true); ok && strings.Contains(f, "12") {
		t.Fatalf("numero cambiato: %q", f)
	}
}

func TestUnisciFrammenti(t *testing.T) {
	for in, atteso := range map[string]string{
		"CO NTROL":            "CONTROL",
		"P U B L I C GAME":    "PUBLIC GAME",
		"Inven tory full":     "Inventory full",
		"Press E to interact": "Press E to interact",
		"to day":              "to day",
		"A I":                 "A I",
		"Hold CO NTROL!":      "Hold CONTROL!",
	} {
		if got := unisciFrammenti(in); got != atteso {
			t.Errorf("%q -> %q, atteso %q", in, got, atteso)
		}
	}
}

// Righe mezze tradotte: se resta inglese vero (o spazzatura dell'OCR) la riga si scarta.
func TestRigaMezzaTradotta(t *testing.T) {
	mezze := [][2]string{
		{"Move to fill the progress bur", "Move to fill il progresso bur"},
		{"Sneak to fill the progress bar", "Sneak to fill il progresso"},
		{"HoldtONTROL and use WASD until the progess is filled!", "HoldtONTROL e use WASD Fino a quando il progresso è pieno!"},
		{"QUIT GAME", "QUIT GIOCO"},
		{"Curry thingthing to fill the progress bur", "Curry thingthing per riempire il bur di progresso"},
		{"Open the inventory", "Open l'inventario"},
	}
	for _, c := range mezze {
		if !rigaMezzaTradotta(c[0], c[1]) {
			t.Errorf("%q -> %q: doveva essere mezza tradotta", c[0], c[1])
		}
	}
	intere := [][2]string{
		{"Use WASD to move.", "Usa WASD per muoverti."},
		{"Grab objects using left mouse.", "Afferra gli oggetti con il tasto sinistro del mouse."},
		{"Press CONTROL to sneak.", "Premi CONTROL per muoverti furtivamente."},
		{"Talk to Kael in Riverwood", "Parla con Kael a Riverwood"},
		{"Open the online menu", "Apri il menu online"},
		{"Press E to interact", "Premi E per interagire"},
		{"Upgrade your HUD", "Migliora il tuo HUD"},
		{"TUTORIAL", "TUTORIAL"},
	}
	for _, c := range intere {
		if rigaMezzaTradotta(c[0], c[1]) {
			t.Errorf("%q -> %q: è tradotta tutta", c[0], c[1])
		}
	}
}

// Tasti come segnaposti intoccabili.
func TestSegnapostiTasti(t *testing.T) {
	casi := []struct {
		en, motore, atteso string // motore: cosa restituisce il motore finto (segnaposti già tradotti attorno)
	}{
		{"Hold CONTROL and use WASD to sneak", "Tieni premuto Qzva e usa Qzve per muoverti di soppiatto", "Tieni premuto CONTROL e usa WASD per muoverti di soppiatto"},
		{"Grab objects using left mouse.", "Afferra gli oggetti usando Qzva.", "Afferra gli oggetti usando il tasto sinistro del mouse."},
		{"Press the right mouse button to aim", "Premi il Qzva per mirare", "Premi il tasto destro del mouse per mirare"},
		{"Press E to interact, SHIFT to run", "Premi Qzva per interagire, Qzve per correre", "Premi E per interagire, SHIFT per correre"},
		{"Press F5 to quicksave", "Premi Qzva per salvare", "Premi F5 per salvare"},
		{"Use LMB to attack", "Usa Qzva per attaccare", "Usa LMB per attaccare"},
	}
	for _, c := range casi {
		p, m := proteggiTasti(c.en)
		if m == nil {
			t.Errorf("%q: nessun tasto protetto", c.en)
			continue
		}
		if strings.Contains(p, "CONTROL") || strings.Contains(p, "WASD") || strings.Contains(strings.ToLower(p), "mouse") {
			t.Errorf("%q: tasto non protetto: %q", c.en, p)
		}
		got, ok := ripristinaTasti(c.motore, m)
		if !ok || got != c.atteso {
			t.Errorf("%q -> %q %v, atteso %q (protetto %q %v)", c.en, got, ok, c.atteso, p, m)
		}
	}
	// righe tutte maiuscole: "OUTER SPACE" non è un tasto, "A" e "I" nemmeno
	for _, s := range []string{"OUTER SPACE", "QUIT GAME", "I am here", "A new day"} {
		if _, m := proteggiTasti(s); m != nil {
			t.Errorf("%q: niente tasti, invece %v", s, m)
		}
	}
	// segnaposto perso dal motore: si rifà senza, poi i tasti storpiati tornano giusti
	_, m := proteggiTasti("Press CONTROL to crouch")
	if _, ok := ripristinaTasti("Premi per accovacciarti", m); ok {
		t.Error("segnaposto perso: doveva dire false")
	}
	for in, atteso := range map[string]string{
		"Premi CO NTROL per accovacciarti":  "Premi CONTROL per accovacciarti",
		"Premi CONTROLLO per accovacciarti": "Premi CONTROL per accovacciarti",
		"Premi CONTROL per accovacciarti":   "Premi CONTROL per accovacciarti",
	} {
		if got := tastiIntatti("Press CONTROL to crouch", in); got != atteso {
			t.Errorf("tastiIntatti %q -> %q, atteso %q", in, got, atteso)
		}
	}
}
