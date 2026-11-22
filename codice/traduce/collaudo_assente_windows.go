//go:build windows && !collaudo

package traduce

// Nell'exe per gli utenti le modalità di prova (--collaudo, --prova1000) non ci sono:
// si compilano solo con -tags collaudo. Così l'exe normale non scarica né avvia niente da solo.

func leggiArgomentiCollaudo([]string) {}
func avviaCollaudoSeServe()           {}
func modalitaProva([]string) bool     { return false }

var suVoceProva func(RigaVoce)
