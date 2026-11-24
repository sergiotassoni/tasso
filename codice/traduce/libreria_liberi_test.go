package traduce

import (
	"strings"
	"testing"
)

func TestLibreriaLiberi(t *testing.T) {
	caricaLiberi()
	if len(liberiEsatte) < 15000 {
		t.Fatalf("libreria troppo piccola: %d", len(liberiEsatte))
	}
	if _, ok := fraseLiberaEsatta("Got it"); ok {
		t.Fatal("le scritte corte non devono uscire dalla libreria")
	}
	if v, ok := fraseLiberaEsatta("Let them come. We will fight them with all we have!"); !ok || v != "Che vengano pure. Li combatteremo con tutte le nostre forze!" {
		t.Fatalf("frase: %q %v", v, ok)
	}
	if v, ok := fraseLiberaEsatta("ARE YOU SURE YOU WANT TO QUIT?"); !ok || v != "SEI SICURO DI VOLER USCIRE?" {
		t.Fatalf("maiuscole: %q %v", v, ok)
	}
	if _, ok := fraseLiberaEsatta("Zxqv the unknowable blorp"); ok {
		t.Fatal("trovata frase inventata")
	}
	es := esempiSimili("The trolls are coming through the cave entrance, hold the line!", 3)
	if len(es) == 0 || !strings.Contains(strings.ToLower(es[0].en+es[1].en+es[len(es)-1].en), "troll") {
		t.Fatalf("esempi: %+v", es)
	}
	t.Logf("%d frasi esatte, %d per gli esempi; es: %+v", len(liberiEsatte), len(liberiFrasi), es)
}
