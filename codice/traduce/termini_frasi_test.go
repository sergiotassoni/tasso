package traduce

import "testing"

func TestCorreggiTerminiFrase(t *testing.T) {
	casi := [][3]string{
		{"You found an Iron Helmet.", "Hai trovato un casco di ferro.", "Hai trovato un elmo di ferro."},
		{"Equip the Iron Helmet to increase your defense.", "Equipaggia il casco di ferro per aumentare la tua difesa.", "Equipaggia l'elmo di ferro per aumentare la tua difesa."},
		{"Sell the iron helmets.", "Vendi i caschi di ferro.", "Vendi gli elmi di ferro."},
		{"IRON HELMET FOUND", "TROVATO IL CASCO DI FERRO", "TROVATO L'ELMO DI FERRO"},
		{"Put it in the cargo bay.", "Mettilo nella baia di carico.", "Mettilo nella stiva."},
		{"You found a new magazine for your rifle.", "Hai trovato una nuova rivista per il tuo fucile.", "Hai trovato un nuovo caricatore per il tuo fucile."},
		{"Keep your helmet on.", "Tieni il casco.", "Tieni il casco."},
		{"Last magazine. Make it count.", "L'ultima rivista. Fai in modo che conti.", "L'ultimo caricatore. Fai in modo che conti."},
		{"I read that magazine.", "Ho letto quella rivista.", "Ho letto quella rivista."},
		{"Stay away from the airlock.", "State lontani dalla camera d'aria.", "State lontani dalla camera stagna."},
		{"Captain, we're being hailed by an unknown vessel.", "Capitano, siamo salutati da un vascello sconosciuto.", "Capitano, riceviamo una chiamata da un vascello sconosciuto."},
		{"Captain, we're being hailed by an unknown vessel, should I put them on screen?", "Capitano, siamo stati salutati da una nave sconosciuta, dovrei metterli sullo schermo?", "Capitano, riceviamo una chiamata da una nave sconosciuta, dovrei metterli sullo schermo?"},
		{"Grenade! Take cover!", "Granata! Prenditi copertura!", "Granata! Mettiti al riparo!"},
		{"Who put a cheese wheel in the treasure chest?", "Chi ha messo una ruota del formaggio nel petto del tesoro?", "Chi ha messo una ruota del formaggio nel forziere?"},
		{"Our scouts haven't returned.", "I nostri scout non sono tornati.", "I nostri esploratori non sono tornati."},
	}
	for _, c := range casi {
		if g := correggiTerminiFrase(c[0], c[1]); g != c[2] {
			t.Errorf("%s\n  %q\n  voleva %q", c[0], g, c[2])
		}
	}
}

func TestPuoEssereNome(t *testing.T) {
	for _, s := range []string{"Captain Price", "Geralt", "Ser Davos of Seaworth"} {
		if !puoEssereNome(s) {
			t.Errorf("%q è un nome", s)
		}
	}
	for _, s := range []string{"Skill points available", "Time trial", "Cooldown"} {
		if puoEssereNome(s) {
			t.Errorf("%q non è un nome", s)
		}
	}
}

func TestRitoccaItaliano(t *testing.T) {
	casi := [][2]string{
		{"E' stato troppo vicino. Bel colpo, pero'.", "È stato troppo vicino. Bel colpo, però."},
		{"C'e' una tempesta in arrivo.", "C'è una tempesta in arrivo."},
		{"Mettiti giu'!", "Mettiti giù!"},
		{"puoi affittarla per un po '.", "puoi affittarla per un po'."},
		{"- Ricarica in corso, coprimi!", "Ricarica in corso, coprimi!"},
		{"Qualunque cosa sia successa qui, e' successo in fretta.", "Qualunque cosa sia successa qui, è successo in fretta."},
		{"L'arma e la spada.", "L'arma e la spada."},
		{"Mi servira' una pistola piu' grande. Portatelo fuori di li'!", "Mi servirà una pistola più grande. Portatelo fuori di lì!"},
		{"Aspetta un po' qui. Di' qualcosa. Va' via.", "Aspetta un po' qui. Di' qualcosa. Va' via."},
		{"Dell'oro e dell'argento.", "Dell'oro e dell'argento."},
		{"Seleziona 'Gioco' per iniziare.", "Seleziona 'Gioco' per iniziare."},
		{"Ha detto 'ciao' a tutti, poi servira' aiuto.", "Ha detto 'ciao' a tutti, poi servirà aiuto."},
	}
	for _, c := range casi {
		if g := ritoccaItaliano(c[0]); g != c[1] {
			t.Errorf("%q -> %q, voleva %q", c[0], g, c[1])
		}
	}
}

func TestTerminiMenu(t *testing.T) {
	casi := [][3]string{
		{"Autosave in progress. Do not turn off your console.", "Autosvegliere in corso. Non spegnere la console.", "Salvataggio automatico in corso. Non spegnere la console."},
		{"Hold Shift to sprint.", "Tieni premuto il cambio per sprint.", "Tieni premuto Maiusc per scattare."},
		{"Press to sprint.", "Premi per sprint.", "Premi per scattare."},
		{"Hold to sprint", "Tieni premuto per fare sprint", "Tieni premuto per scattare"},
		{"Press t to sprint.", "Premi t per sprint.", "Premi t per scattare."},
		{"Tap Shift to sprint while running", "Tocca Maiusc per sprintare mentre corri", "Tocca Maiusc per scattare mentre corri"},
		{"Hold Shift: Sprint", "Tieni premuto Maiusc: Sprint", "Tieni premuto Maiusc: Scatto"},
		{"Sprint Mode", "Modalità Sprint", "Modalità Sprint"}, // non è un comando: resta com'è
		{"The night shift starts at ten.", "Il cambio notturno inizia alle dieci.", "Il cambio notturno inizia alle dieci."},
	}
	for _, c := range casi {
		if g := correggiTerminiFrase(c[0], c[1]); g != c[2] {
			t.Errorf("%s\n  %q\n  voleva %q", c[0], g, c[2])
		}
	}
}
