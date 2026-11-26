//go:build !collaudo

package traduce

// forzaPerProva: nella versione pubblicata nessuna variabile d'ambiente cambia cosa fa la
// traduzione dentro il gioco (vedi forza_prova.go per le build di prova).
func forzaPerProva(string) bool { return false }
