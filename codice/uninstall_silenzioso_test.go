package main

import "testing"

func TestUnaRiga(t *testing.T) {
	if g := unaRiga("Chiudi Gioco\r\ne riprova:\tniente  tolto. "); g != "Chiudi Gioco e riprova: niente tolto." {
		t.Fatalf("unaRiga = %q", g)
	}
}
