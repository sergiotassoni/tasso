package installa

import "testing"

func TestCollegamentoVecchioDaAggiornare(t *testing.T) {
	pf := `C:\Program Files`
	desc := "Tasso - pulisce, ripara e velocizza il PC" // quella della 3.0
	casi := []struct {
		dest, arg, desc string
		si              bool
	}{
		{`C:\Users\Sergio\Downloads\Tasso\Tasso.exe`, "", desc, true},
		{`D:\Portatili\TASSO.EXE`, "", "Tasso", true},
		{`C:\Program Files\Tasso\Tasso.exe`, "", desc, false},         // già quello installato
		{`C:\Users\Sergio\Downloads\TassoZip.exe`, "", desc, false},   // un altro programma
		{`C:\Users\Sergio\Downloads\Tasso.exe`, "--min", desc, false}, // con argomenti: fatto da qualcun altro
		{`C:\Users\Sergio\Downloads\Tasso.exe`, "", "Il mio gioco", false},
		{`C:\Users\Sergio\Downloads\Tasso.exe`, "", "", false},
		{`\\server\condivisa\Tasso.exe`, "", desc, false},
		{``, "", desc, false},
	}
	for _, c := range casi {
		if g := CollegamentoVecchioDaAggiornare(c.dest, c.arg, c.desc, pf); g != c.si {
			t.Errorf("%q %q %q: %v, voglio %v", c.dest, c.arg, c.desc, g, c.si)
		}
	}
	if CollegamentoVecchioDaAggiornare(`C:\Users\x\Tasso.exe`, "", desc, "") {
		t.Error("senza Programmi non si decide")
	}
}

func TestCollegamentoVecchioNostro(t *testing.T) {
	base := `C:\Program Files\Tasso`
	casi := []struct {
		dest, arg string
		si        bool
	}{
		{`C:\Program Files\Tasso\Tasso.exe`, "", true},
		{`c:\program files\tasso\TASSO.EXE`, "", true},
		{`C:\Program Files\Tasso\Tasso.exe`, "--pulisci", false},
		{`C:\Users\x\Downloads\Tasso.exe`, "", false}, // la 3.0 portatile mai sistemata: non è nostro
		{`C:\Program Files\Tasso\Tasso Zip\TassoZip.exe`, "", false},
		{`C:\Program Files\Tasso\Altro\Tasso.exe`, "", false},
		{``, "", false},
	}
	for _, c := range casi {
		if g := CollegamentoVecchioNostro(c.dest, c.arg, base); g != c.si {
			t.Errorf("%q %q: %v, voglio %v", c.dest, c.arg, g, c.si)
		}
	}
	if CollegamentoVecchioNostro(`\Tasso.exe`, "", "") {
		t.Error("senza cartella non si decide")
	}
}
