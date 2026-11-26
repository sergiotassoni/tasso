//go:build collaudo

package traduce

import "os"

// forzaPerProva: solo nelle build di prova (-tags collaudo) le variabili TASSO_FILE_UNREAL e
// TASSO_FORZA_FILE accendono i rami sperimentali della traduzione dentro il gioco.
// Nella versione pubblicata non esistono (forza_prova_no.go). L'anti-cheat non si forza mai.
func forzaPerProva(nome string) bool { return os.Getenv(nome) != "" }
