package main

import "testing"

// Il TEMP dell'utente si pulisce solo se è davvero una cartella di temporanei, mai dentro le cartelle personali.
func TestTempNomeAccettabile(t *testing.T) {
	pers := []string{`C:\Users\Mario`, `C:\Users\Mario\Documents`, `C:\Users\Mario\Desktop`, `C:\Users\Mario\Downloads`, `C:\Users\Mario\OneDrive`}
	casi := map[string]bool{
		`D:\Temp`:                            true,
		`D:\tmp`:                             true,
		`C:\Users\Mario\AppData\Local\Temp2`: true,
		`E:\Lavoro\MyTemp`:                   true,
		`C:\Users\Mario\Documents`:           false,
		`C:\Users\Mario\Documents\Temp`:      false,
		`C:\Users\Mario\Desktop`:             false,
		`C:\Users\Mario\Downloads\tmp`:       false,
		`C:\Users\Mario\OneDrive\Temp`:       false,
		`C:\Users\Mario`:                     false,
		`D:\Lavoro`:                          false,
		`D:\Documenti\temp`:                  false,
		`D:\OneDrive - Ditta\Temp`:           false,
		`D:\Dati`:                            false,
	}
	for p, want := range casi {
		if got := tempNomeAccettabile(p, pers); got != want {
			t.Errorf("tempNomeAccettabile(%q) = %v, voglio %v", p, got, want)
		}
	}
}
