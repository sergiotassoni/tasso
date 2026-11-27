// Tasso PDF - Copyright (C) 2026 Sergio Tassoni - GPL-3.0-or-later (vedi LICENSE).

// Package nome: "Il tuo nome" dell'Ecosistema Tasso. Il nome (o nickname) lo scrivi
// nell'installatore o nelle Impostazioni e lo usano Tasso, Tasso Zip e Tasso PDF per salutarti.
// Sta solo sul PC, per ogni utente di Windows: HKCU\Software\Tasso, valore "Nome" (REG_SZ).
// Non va mai in rete.
//
// Qui ci sono le regole, uguali in tutti i programmi (e nella pagina, in JavaScript):
//   - da 2 a 20 caratteri (contati come caratteri veri, non byte);
//   - lettere (anche accentate, di qualsiasi alfabeto), numeri, spazio, punto, trattino e
//     trattino basso; i segni che si appoggiano a una lettera (accenti "staccati") vanno bene
//     dopo una lettera;
//   - almeno una lettera o un numero.
//
// Prima di controllare si pulisce: spazi all'inizio e alla fine via, più spazi di fila
// diventano uno solo (anche tabulazioni, a capo e spazi "speciali" incollati da altre parti).
//
// Copia di tasso/codice/nomeutente (le regole sono le stesse anche in: installatore,
// Tasso, Tasso PDF e nelle pagine, funzione nomeControlla). Se cambi qui, cambia anche lì.
package nome

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Lunghezza ammessa, in caratteri.
const (
	Min = 2
	Max = 20
)

// Codici dell'esito di Controlla (la pagina li trasforma in frasi gentili).
const (
	OK         = ""
	Corto      = "corto"      // meno di 2 caratteri
	Lungo      = "lungo"      // più di 20 caratteri
	Simbolo    = "simbolo"    // un simbolo che non si può usare (Esito.Simbolo dice quale)
	Invisibile = "invisibile" // un carattere che non si vede (di controllo, a larghezza zero...)
	Senza      = "senza"      // solo punti, trattini o spazi: niente lettere né numeri
)

// Esito di Controlla.
type Esito struct {
	Codice  string
	Simbolo string // per Codice == Simbolo: il primo carattere non ammesso
}

// spazio: gli spazi che la pagina toglie con /\s/ (JavaScript), così Go e la pagina puliscono
// allo stesso modo.
func spazio(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', '\u00a0', '\u1680', '\u2028', '\u2029', '\u202f', '\u205f', '\u3000', '\ufeff':
		return true
	}
	return r >= '\u2000' && r <= '\u200a'
}

// Pulisci toglie gli spazi all'inizio e alla fine e unisce quelli di fila in uno spazio solo.
// Un testo che non è UTF-8 valido resta com'è (Controlla lo rifiuta).
func Pulisci(s string) string {
	if !utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	prima := false // l'ultimo carattere scritto era uno spazio
	for _, r := range s {
		if spazio(r) {
			if b.Len() > 0 && !prima {
				b.WriteByte(' ')
				prima = true
			}
			continue
		}
		b.WriteRune(r)
		prima = false
	}
	return strings.TrimRight(b.String(), " ")
}

// ammesso: il carattere r, in posizione i, va bene.
func ammesso(r rune, i int) bool {
	switch {
	case r == ' ' || r == '.' || r == '-' || r == '_':
		return true
	case unicode.IsLetter(r) || unicode.Is(unicode.Nd, r):
		return true
	case unicode.Is(unicode.M, r):
		return i > 0 // un accento "staccato" va bene solo dopo un altro carattere
	}
	return false
}

// invisibile: caratteri che non si vedono (di controllo, formato, spazi strani, non assegnati).
func invisibile(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Z, r) ||
		unicode.Is(unicode.Co, r) || unicode.Is(unicode.Cs, r) || !unicode.Is(unicode.L, r) && !unicode.IsGraphic(r)
}

// Controlla dice se il nome (già pulito con Pulisci) va bene e, se no, perché.
func Controlla(s string) Esito {
	if !utf8.ValidString(s) {
		return Esito{Codice: Invisibile}
	}
	n := utf8.RuneCountInString(s)
	if n < Min {
		return Esito{Codice: Corto}
	}
	if n > Max {
		return Esito{Codice: Lungo}
	}
	i := 0
	for _, r := range s {
		if !ammesso(r, i) {
			if invisibile(r) {
				return Esito{Codice: Invisibile}
			}
			return Esito{Codice: Simbolo, Simbolo: string(r)}
		}
		i++
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.Is(unicode.Nd, r) {
			return Esito{}
		}
	}
	return Esito{Codice: Senza}
}

// Valido: il nome, così com'è, si può salvare e mostrare (già pulito e con le regole giuste).
func Valido(s string) bool {
	return s == Pulisci(s) && Controlla(s).Codice == OK
}

// DaSalvare pulisce il nome scritto e lo controlla: restituisce il nome da salvare e l'esito.
func DaSalvare(scritto string) (string, Esito) {
	p := Pulisci(scritto)
	return p, Controlla(p)
}

// Letto: il nome letto dal registro, da usare solo se è valido; "" se manca o è strano (scritto a
// mano, troppo lungo, con simboli): allora i saluti restano senza nome.
func Letto(v string) string {
	if Valido(v) {
		return v
	}
	return ""
}

// Archivio: dove sta il nome (in Windows il registro, HKCU\Software\Tasso; nei test una mappa).
type Archivio interface {
	Leggi() (string, bool) // il valore salvato e se c'è
	Scrivi(string) error
	Togli() error
}

// Leggi: il nome salvato nell'archivio, "" se manca o non è valido.
func Leggi(a Archivio) string {
	v, ok := a.Leggi()
	if !ok {
		return ""
	}
	return Letto(v)
}

// CodiceRegistro: Windows non ha lasciato scrivere il nome.
const CodiceRegistro = "registro"

// Salva pulisce e controlla il nome scritto e, solo se va bene, lo scrive nell'archivio.
// Restituisce il nome salvato ("" se non salvato) e l'esito.
func Salva(a Archivio, scritto string) (string, Esito) {
	n, e := DaSalvare(scritto)
	if e.Codice != OK {
		return "", e
	}
	if err := a.Scrivi(n); err != nil {
		return "", Esito{Codice: CodiceRegistro}
	}
	return n, Esito{}
}
