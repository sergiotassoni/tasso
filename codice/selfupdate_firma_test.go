package main

import "testing"

// Un Tasso non firmato (versione di prova) non si aggiorna; uno firmato vuole lo stesso nome e
// una firma valida.
func TestSigDecision(t *testing.T) {
	valido := sigInfo{Status: "Valid", Subject: "CN=Open Source Developer Sergio Tassoni"}
	casi := []struct {
		cur, nw sigInfo
		ok      bool
	}{
		{sigInfo{}, sigInfo{}, false},                                  // prova non firmata: mai
		{sigInfo{}, valido, false},                                     // anche se il nuovo è firmato
		{valido, valido, true},                                         // firmato -> firmato, stesso nome
		{valido, sigInfo{Status: "NotSigned"}, false},                  // nuovo senza firma
		{valido, sigInfo{Status: "Valid", Subject: "CN=Altro"}, false}, // firmato da un altro
		{sigInfo{Status: "UnknownError", Subject: valido.Subject}, valido, false},
	}
	for i, c := range casi {
		if got := sigDecision(c.cur, c.nw) == ""; got != c.ok {
			t.Errorf("caso %d: aggiornabile=%v, voglio %v", i, got, c.ok)
		}
	}
	if sigDecision(sigInfo{}, sigInfo{}) != msgNonFirmato {
		t.Fatal("messaggio sbagliato per la versione di prova")
	}
}
