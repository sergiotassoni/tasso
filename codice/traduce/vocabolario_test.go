package traduce

import "testing"

func TestCorreggiParole(t *testing.T) {
	casi := map[string]string{
		"Captain, we're being hai led by an unknown vessel.": "Captain, we're being hailed by an unknown vessel.",
		"Waitinq for other players...":                       "Waiting for other players...",
		"The bridge to the northern valley collapsed.":       "The bridge to the northern valley collapsed.",
		"Talk to Vex'ahlia in Whitestone.":                   "Talk to Vex'ahlia in Whitestone.",
		"Your HP is low. Use a potion.":                      "Your HP is low. Use a potion.",
		"Press E to open the chest.":                         "Press E to open the chest.",
		"The rnines are dark.":                               "The mines are dark.",
		"Enemy sniper on the roof!":                          "Enemy sniper on the roof!",
		"I will come with out you.":                          "I will come with out you.",
	}
	for in, atteso := range casi {
		if got := correggiParole(in); got != atteso {
			t.Errorf("%q -> %q, atteso %q", in, got, atteso)
		}
	}
}

func TestVocabolario(t *testing.T) {
	if len(vocabolario()) < 70000 {
		t.Fatalf("vocabolario troppo piccolo: %d", len(vocabolario()))
	}
}

func TestCataloghi(t *testing.T) {
	caricaCataloghi()
	if len(termGiochi) < 300 || len(frasiMenu) < 300 {
		t.Fatalf("cataloghi piccoli: %d termini, %d frasi", len(termGiochi), len(frasiMenu))
	}
	for in, atteso := range map[string]string{
		"Loading...":                   "Caricamento...",
		"PRESS ANY KEY TO CONTINUE":    "Premi un tasto qualsiasi per continuare",
		"Your inventory is full!":      "Il tuo inventario è pieno!",
		"Waiting for other players...": "In attesa degli altri giocatori...",
		"Game saved.":                  "Partita salvata.",
	} {
		if got, ok := fraseMenu(in); !ok || got != atteso {
			t.Errorf("%q -> %q %v, atteso %q", in, got, ok, atteso)
		}
	}
	if _, ok := fraseMenu("Loading the cannons, captain."); ok {
		t.Fatal("una frase di dialogo non deve passare dal catalogo")
	}
	g := Glossario{Termini: map[string]string{"HP": "Vita"}}.conGiochi()
	if g.Termini["HP"] != "Vita" || g.Termini["Inventory"] != "Inventario" {
		t.Fatal("il glossario dell'utente deve vincere su quello da gioco")
	}
	v := g.trova("Your hull integrity is at 30 percent, use a medkit.")
	if len(v) < 2 {
		t.Fatalf("voci trovate: %+v", v)
	}
}

func TestSigleItaliane(t *testing.T) {
	v := glossarioBase().conGiochi().trova("Your HP is low and you gained 50 XP.")
	if got := sigleItaliane("I tuoi HP sono bassi e hai guadagnato 50 XP.", v); got != "I tuoi PV sono bassi e hai guadagnato 50 PE." {
		t.Fatal(got)
	}
}

func TestBloccaTermini(t *testing.T) {
	g := glossarioBase().conGiochi()
	testo := "- Bleed Resist Increased 10%"
	v := g.trova(testo)
	b, m := bloccaTermini(testo, v)
	if b != "- Qzxa Increased 10%" || m["Qzxa"] != "Resistenza al sanguinamento" {
		t.Fatalf("%q %v", b, m)
	}
	if s, ok := sbloccaTermini("- Qzxa Aumento del 10%", m); !ok || s != "- Resistenza al sanguinamento Aumento del 10%" {
		t.Fatal(s)
	}
	if _, ok := sbloccaTermini("- Aumento del 10%", m); ok {
		t.Fatal("nome finto perso: deve fallire")
	}
	lunga := "If you want to reach the tower you will need a potion and a sword."
	if _, m := bloccaTermini(lunga, g.trova(lunga)); m != nil {
		t.Fatal("le frasi lunghe non si bloccano")
	}
}

func TestConfusioneCG(t *testing.T) {
	if g := correggiParole("Crowth Rate Increased"); g != "Growth Rate Increased" {
		t.Errorf("%q", g)
	}
}

// Un nome letto a inizio frase ("Raffs") non diventa una parola rara ("Rafts" -> "zattere"):
// con la maiuscola si corregge solo verso le parole comuni.
func TestNomiNonCorretti(t *testing.T) {
	for in, atteso := range map[string]string{
		"Raffs":                        "Raffs",
		"Raffs, get over here!":        "Raffs, get over here!",
		"Hey. Raffs is waiting.":       "Hey. Raffs is waiting.",
		"Waitinq for other players...": "Waiting for other players...",
		"Crowth Rate Increased":        "Growth Rate Increased",
	} {
		if got := correggiParole(in); got != atteso {
			t.Errorf("%q -> %q, atteso %q", in, got, atteso)
		}
	}
}
