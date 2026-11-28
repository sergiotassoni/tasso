package traduce

import "testing"

// Righe vere delle foto di Sergio su Hytale e sulla libreria di Tasso (28/11).
func TestRigheOCRSporche(t *testing.T) {
	scartare := []string{
		"Voice Cha",        // parola tagliata al bordo della zona
		"OccWon & DiNr•ün", // maiuscole a caso e carattere strano
		"ı‾ —",             // solo simboli spuri
		"SUBNAUT'CAZ",      // parola tutta maiuscola inventata
		"Tonie",            // parola sconosciuta da sola: nome o lettura sbagliata, comunque no
		"rnedklt wqz",
		"ZC•e•.",
	}
	for _, s := range scartare {
		if _, ok := rigaDaTradurre(s, true); ok {
			t.Errorf("%q: doveva essere scartata", s)
		}
	}
	tenere := []string{
		"Master Volume", "Ul Volume", "SFX Volume", "Occlusion & Diffraction", "Diffraction Quality",
		"Toggle Voice Chat", "Output Device", "Reset to Defaults",
		"Talk to Sarah in the tavern before nightfall.",
		"ı‾ — SFX Volume",
	}
	for _, s := range tenere {
		if p, ok := rigaDaTradurre(s, true); !ok {
			t.Errorf("%q (pulita %q): doveva restare", s, p)
		}
	}
}

func TestTogliSimboliSpuri(t *testing.T) {
	for in, out := range map[string]string{
		"ı‾ — Volume effetti": "Volume effetti",
		"Master Volume —":     "Master Volume",
		"• Options •":         "Options",
		"Unbound — — x":       "Unbound — x",
		"Nome — Valore":       "Nome — Valore",
		"Steam · 31,4 GB":     "Steam · 31,4 GB",
		"RECENTE — -.10RL":    "RECENTE — -.10RL",
		"™ Press any key ®":   "Press any key",
	} {
		if g := togliSimboliSpuri(in); g != out {
			t.Errorf("%q -> %q, atteso %q", in, g, out)
		}
	}
}

func TestSoloNomiPropri(t *testing.T) {
	for s, v := range map[string]bool{
		"Sarah":           true,
		"Ethan":           true,
		"Dorothy Michael": true,
		"Steam":           true,
		"Hytale Launcher": true,
		"Volume":          false,
		"Master Volume":   false,
		"English":         false,
		"Talk to Sarah":   false,
		"Release date":    false,
	} {
		if g := soloNomiPropri(s); g != v {
			t.Errorf("%q: %v, atteso %v", s, g, v)
		}
	}
}

func TestTraduzioneDaMostrare(t *testing.T) {
	if traduzioneDaMostrare("UI Volume", "UI Volume") || traduzioneDaMostrare("Diffraction", "diffraction.") || traduzioneDaMostrare("Steam", "") {
		t.Error("le voci rimaste uguali non sono tradotte")
	}
	if !traduzioneDaMostrare("UI Volume", "Volume interfaccia") {
		t.Error("tradotta")
	}
}

func TestFrasiMenuHytale(t *testing.T) {
	for in, out := range map[string]string{
		"UI Volume":               "Volume interfaccia",
		"Ul Volume":               "Volume interfaccia",
		"Diffraction":             "Diffrazione",
		"Occlusion & Diffraction": "Occlusione e diffrazione",
		"Diffraction Quality":     "Qualità della diffrazione",
		"Ambience Volume":         "Volume ambiente",
		"Output Mode":             "Modalità di uscita",
		"Experimental":            "Sperimentale",
	} {
		if g, ok := fraseMenuConNumero(in); !ok || g != out {
			t.Errorf("%q -> %q, atteso %q", in, g, out)
		}
	}
}

func TestRigheAffidabiliZona(t *testing.T) {
	// la lista di nomi sopra YouTube (foto di Sergio 28/11): non si mostra niente
	if g := righeAffidabili("Sarah\nEthan\nLauren\nDorothy\nKaty\nMichael\nZC•e•.", true, false); g != "" {
		t.Errorf("lista di nomi: %q", g)
	}
	if g := righeAffidabili("Sarah\nEthan\nLauren", true, true); g != "" {
		t.Errorf("lista di nomi (menu): %q", g)
	}
	// dialogo che va a capo: la riga corta col nome resta
	if g := righeAffidabili("I told you to wait for me,\nCaz.", true, false); g != "I told you to wait for me,\nCaz." {
		t.Errorf("dialogo: %q", g)
	}
	// menu di Hytale letto male (foto 4): restano solo le voci buone
	g := righeAffidabili("Ambience Volume\nVoice Cha\nTonie\nOccWon & DiNr•ün\nDiffraction", true, true)
	if g != "Ambience Volume\nDiffraction" {
		t.Errorf("menu: %q", g)
	}
}
