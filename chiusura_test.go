// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

package main

import (
	"testing"
	"time"
)

// F16-1: la X della finestra con documenti aperti chiede prima alla pagina.
func TestChiusuraX(t *testing.T) {
	azzera := func() {
		chiusura.mu.Lock()
		chiusura.aperti, chiusura.numero, chiusura.chiesta = false, 0, time.Time{}
		chiusura.mu.Unlock()
	}
	azzera()
	defer azzera()
	ora := time.Now()

	// Nessun documento aperto: la X chiude subito, come prima.
	if resta, chiedi := chiusuraX(ora); resta || chiedi {
		t.Fatalf("senza documenti: resta=%v chiedi=%v", resta, chiedi)
	}
	// Documenti aperti: la finestra resta e si chiede alla pagina.
	goDocumentiAperti(1, true)
	if resta, chiedi := chiusuraX(ora); !resta || !chiedi {
		t.Fatalf("con documenti: resta=%v chiedi=%v", resta, chiedi)
	}
	// Doppio clic sulla X subito dopo: resta, senza una seconda domanda.
	if resta, chiedi := chiusuraX(ora.Add(time.Second)); !resta || chiedi {
		t.Fatalf("doppio clic: resta=%v chiedi=%v", resta, chiedi)
	}
	// La pagina non risponde: dopo l'attesa la X chiude, così la finestra si chiude sempre.
	if resta, _ := chiusuraX(ora.Add(attesaRispostaChiusura + time.Second)); resta {
		t.Fatal("pagina bloccata: la finestra deve chiudersi")
	}
	// La pagina ha risposto ("Annulla"): la X dopo chiede di nuovo.
	chiusuraX(ora)
	goChiusuraVista()
	if resta, chiedi := chiusuraX(ora.Add(10 * time.Second)); !resta || !chiedi {
		t.Fatalf("dopo la risposta: resta=%v chiedi=%v", resta, chiedi)
	}
	goChiusuraVista()
	// Un aggiornamento vecchio arrivato in ritardo non conta.
	goDocumentiAperti(3, false)
	goDocumentiAperti(2, true)
	if resta, _ := chiusuraX(ora); resta {
		t.Fatal("l'aggiornamento vecchio (2) non deve vincere sul nuovo (3)")
	}
	// Le tre funzioni sono legate e veloci (non aspettano dietro un lavoro lungo).
	l := legami()
	for _, n := range []string{"goDocumentiAperti", "goChiusuraVista", "goChiudiFinestra"} {
		if _, ok := l[n]; !ok {
			t.Errorf("%s non è in legami()", n)
		}
		trovata := false
		for _, s := range legamiSubito {
			trovata = trovata || s == n
		}
		if !trovata {
			t.Errorf("%s non è in legamiSubito", n)
		}
	}
}
