package traduce

import (
	"testing"
	"time"
)

// Banco notturno del 26/11 (30 video, 3694 fotogrammi): le cause trovate e sistemate.

// 1. "PASSIVE SKILLS" -> "ABILITÀ PASSIVE" (frase pronta giusta) era contata come riga mezza
// tradotta DISEGNATA, e la stessa traduzione dal traduttore veloce veniva scartata 56 volte:
// "passive" è anche italiana, spostata dopo il nome come vuole l'italiano.
func TestNonoGiroParolaItalianaSpostata(t *testing.T) {
	buone := [][2]string{
		{"PASSIVE SKILLS", "ABILITÀ PASSIVE"},
		{"Passive Skills", "Abilità passive"},
		{"Complete the weapons test.", "Completa il test delle armi."},
		{"You can consume Stamina to move quickly.", "Puoi consumare Stamina per muoverti rapidamente."},
		{"That goblin up ahead has not seen you yet.", "Quel goblin laggiù non ti ha ancora visto."},
	}
	for _, c := range buone {
		if rigaMezzaTradotta(c[0], c[1]) {
			t.Errorf("traduzione buona presa per mezza inglese: %q -> %q", c[0], c[1])
		}
	}
	// mai una riga mezza tradotta: queste restano scartate
	mezze := [][2]string{
		{"Windows Update", "Windows Aggiornamento"},
		{"Stream Deck", "Deck di Streaming"},
		{"PASSIVE SKILLS", "PASSIVE SKILLS ABILITÀ"},
		{"Open the passive skills menu", "Apri il menu passive skills"},
		{"To move the lock pick from slot to slot", "Per spostare il lock pick da slot a slot"},
	}
	for _, c := range mezze {
		if !rigaMezzaTradotta(c[0], c[1]) {
			t.Errorf("riga mezza inglese non trovata: %q -> %q", c[0], c[1])
		}
	}
}

// 2. Le letture tagliate con un pezzo di parola in fondo ("Start s", "Survival is the r") davano
// "Inizia s", "La sopravvivenza è la r", con il pezzo inglese copiato: restano in inglese.
func TestNonoGiroPezzoDiParolaInFondo(t *testing.T) {
	for _, s := range []string{"Start s", "ALL j", "Lock n", "Our lives are he there like an op"} {
		if !pezzoDiParolaInFondo(s) {
			t.Errorf("%q: pezzo in fondo non visto", s)
		}
	}
	for en, it := range map[string]string{"Start s": "Inizia s", "Survival is the r": "La sopravvivenza è la r", "Gear v": "Equipaggiamento v", "Seriously, t": "Sul serio, t"} {
		if m := motivoScartaTraduzione(Blocco{Testo: en}, Risultato{Tradotto: it, Motore: "Mozilla"}); m != motivoPezzoInFondo {
			t.Errorf("%q -> %q: motivo %q", en, it, m)
		}
	}
	// il traduttore ha lasciato fuori il pezzo: la traduzione si vede bene
	for en, it := range map[string]string{"STATISTICS s": "STATISTICHE", "Move Piece w": "Muovi Pezzo", "He knows more than he's telling u": "Sa più di quanto ci stia dicendo"} {
		if m := motivoScartaTraduzione(Blocco{Testo: en}, Risultato{Tradotto: it, Motore: "Mozilla"}); m != "" {
			t.Errorf("%q -> %q: scartata (%q)", en, it, m)
		}
	}
	for _, s := range []string{"Press A", "Hold on to it", "Wait 5 s", "Run 10 m", "Where am I", "Let's go", "Volume", "Use E", "Give it to me"} {
		if pezzoDiParolaInFondo(s) {
			t.Errorf("%q: frase intera presa per tagliata", s)
		}
	}
}

// 3. Isteresi: la traduzione vecchia restava sopra una scritta cambiata davvero (stesso posto,
// forma delle lettere simile): "20 HEALTH PICKED UP" mostrato come "10 SALUTE RACCOLTE",
// "01 Evade" come "Evade dell'OIE", "Sack Cloth Pants" come "Arco di ferro arruggine".
func TestNonoGiroIsteresiScrittaCambiata(t *testing.T) {
	img1 := immagineMondo(600, 300, 0, 0, 1)
	f := lettereIrregolari(img1, 100, 100, 24)
	img2 := immagineMondo(600, 300, 0, 0, 1)
	lettereIrregolari(img2, 100, 100, 24)
	t0 := time.Unix(1000, 0)
	casi := [][4]string{
		{"10 HEALTH PICKED UP", "10 SALUTE RACCOLTA", "20 HEALTH PICKED UP", "20 SALUTE RACCOLTA"},
		{"OIE Evade", "Evade dell'OIE", "01 Evade", "01 Schiva"},
		{"Rusty Iron Bow", "Arco di ferro arrugginito", "Sack Cloth Pants", "Pantaloni di tela grezza"},
	}
	for _, c := range casi {
		ist := &isteresi{}
		a := bloccoMostrato(img1, c[0], c[1], 100, 100, f-100, 24)
		vis, _ := ist.applica([]Blocco{a}, nil, nil, img1, t0)
		b := bloccoMostrato(img2, c[2], c[3], 100, 100, f-100, 24)
		vis, _ = ist.applica([]Blocco{b}, vis, nil, img2, t0.Add(250*time.Millisecond))
		if len(vis) != 1 || vis[0].Tradotto != c[3] {
			t.Errorf("%q -> %q: mostrata ancora la traduzione vecchia %+v", c[0], c[2], vis)
		}
	}
}

// 4. Il banco conta come instabilità solo quello che cambia sullo schermo: inglese e inglese
// (con due motivi diversi) non è uno sfarfallio.
func TestNonoGiroInstabilitaSoloVisibile(t *testing.T) {
	a := esitoImmagine{Nome: "a", Righe: []infoRiga{{Grezzo: "Rusty Iron War Axe", X: 10, Y: 10, W: 100, H: 20, Scartata: "traduzioni sovrapposte"}}}
	b := esitoImmagine{Nome: "b", Righe: []infoRiga{{Grezzo: "Rusty Iron War Axe", X: 10, Y: 10, W: 100, H: 20, Scartata: "lettura incerta"}}}
	if ins := confrontaFotogrammi(a, b); len(ins) != 0 {
		t.Errorf("inglese/inglese contato come instabilità: %+v", ins)
	}
	b.Righe[0].Scartata, b.Righe[0].Tradotto = "", "Ascia da guerra di ferro arrugginita"
	if ins := confrontaFotogrammi(a, b); len(ins) != 1 {
		t.Errorf("inglese/italiano non contato: %+v", ins)
	}
}

// 5. Frasi pronte nuove dal banco (il traduttore veloce sbagliava: "COMPLETATO" al maschile,
// "Cuirass di ferro arruggine", "Casa di Hen").
func TestNonoGiroFrasiPronte(t *testing.T) {
	for en, it := range map[string]string{
		"Main Quest Completed": "Missione principale completata",
		"Rusty Iron Cuirass":   "Corazza di ferro arrugginita",
		"Hen House":            "Pollaio",
		"Previous Character":   "Personaggio precedente",
	} {
		if got, ok := fraseMenu(en); !ok || got != it {
			t.Errorf("%q -> %q (%v), voluto %q", en, got, ok, it)
		}
	}
}
