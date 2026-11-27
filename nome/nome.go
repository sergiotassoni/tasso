// Package nome: "Il tuo nome" dell'Ecosistema Tasso. Il nome (o nickname) lo scrivi
// nell'installatore o nelle Impostazioni e lo usano Tasso, Tasso Zip e Tasso PDF per salutarti.
// Sta solo sul PC, per ogni utente di Windows: HKCU\Software\Tasso, valore "Nome" (REG_SZ).
// Non va mai in rete.
//
// Le regole: da 2 a 20 caratteri; lettere (anche accentate, di molte scritture), numeri, spazio,
// punto, trattino e trattino basso; almeno una lettera o un numero; al massimo 3 accenti
// "staccati" sopra una lettera; niente caratteri che non si vedono. Prima si puliscono gli spazi.
package nome

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// ==== REGOLE DEL NOME: INIZIO ====
// Uguali, carattere per carattere, in Tasso (nomeutente), installatore (nome), Tasso Zip (nome) e
// Tasso PDF (nome); nelle pagine c'è la stessa cosa in JavaScript (nomeControlla). Un test controlla
// l'impronta di questo pezzo: se lo cambi qui, cambialo in tutte e 8 le copie.
//
// Per avere lo stesso esito in Go e nella pagina (che usano versioni diverse di Unicode) le lettere
// ammesse stanno in un elenco esplicito di blocchi di Unicode con scritture stabili da molti anni.
// 3.2: tutte le scritture delle lingue vive che Unicode 15 e 17 vedono allo stesso modo (prima
// erano rifiutate, per esempio, tamil, bengali, etiope e 々).

// Lunghezza ammessa, in caratteri; accenti "staccati" di fila sulla stessa lettera.
const (
	Min        = 2
	Max        = 20
	MaxAccenti = 3
)

// Codici dell'esito di Controlla (la pagina li trasforma in frasi gentili).
const (
	OK         = ""
	Corto      = "corto"      // meno di 2 caratteri
	Lungo      = "lungo"      // più di 20 caratteri
	Simbolo    = "simbolo"    // un carattere che non si può usare (Esito.Simbolo dice quale)
	Invisibile = "invisibile" // un carattere che non si vede
	Senza      = "senza"      // solo punti, trattini o spazi: niente lettere né numeri
	Accenti    = "accenti"    // un accento staccato non sopra una lettera, o più di 3 di fila
)

// Esito di Controlla.
type Esito struct {
	Codice  string
	Simbolo string // per Codice == Simbolo: il primo carattere non ammesso
}

// blocchiAmmessi: le scritture delle lingue vive, in blocchi di Unicode che non cambiano tra
// Unicode 15 (Go) e Unicode 17 (WebView2): latino (con IPA e lettere modificatrici), greco,
// cirillico, armeno, ebraico, arabo, siriaco, thaana, n'ko, le scritture dell'India e dello Sri
// Lanka (devanagari, bengali, gurmukhi, gujarati, oriya, tamil, telugu, kannada, malayalam,
// singalese), thai, lao, tibetano, birmano, georgiano, etiope, cherokee, sillabico canadese,
// filippine, khmer, mongolo, limbu, tai le, tai lue, tai tham, buginese, balinese, sundanese,
// batak, lepcha, ol chiki, tifinagh, hiragana, katakana, bopomofo, il segno 々, ideogrammi cinesi
// (base, estensioni A-G e compatibilità), yi, lisu, vai, bamum, giavanese, cham, tai viet,
// meetei mayek, sillabe coreane, osage, chakma, hmong, miao, wancho e adlam. Le lettere aggiunte
// dopo Unicode 15 (U+0C5C, U+0CDC, U+2B73A-F, U+2CEA2-D) restano fuori. Dentro i blocchi
// valgono solo lettere, cifre e accenti (le categorie L, Nd e M di Unicode).
var blocchiAmmessi = [][2]rune{
	{0x0030, 0x0039}, {0x0041, 0x005A}, {0x0061, 0x007A}, {0x00C0, 0x036F}, {0x0370, 0x03FF},
	{0x0400, 0x052F}, {0x0531, 0x058F}, {0x0591, 0x05FF}, {0x0600, 0x07FF}, {0x08A0, 0x08FF},
	{0x0900, 0x0C5B}, {0x0C5D, 0x0CDB}, {0x0CDD, 0x0DFF}, {0x0E00, 0x10FF}, {0x1200, 0x167F},
	{0x1700, 0x19DF}, {0x1A00, 0x1AAF}, {0x1B00, 0x1C7F}, {0x1C90, 0x1CBF}, {0x1E00, 0x1FFF},
	{0x2D00, 0x2DDF}, {0x3005, 0x3005}, {0x3040, 0x30FF}, {0x3100, 0x312F}, {0x31A0, 0x31BF},
	{0x31F0, 0x31FF}, {0x3400, 0x4DBF}, {0x4E00, 0x9FFF}, {0xA000, 0xA48F}, {0xA4D0, 0xA63F},
	{0xA6A0, 0xA6FF}, {0xA880, 0xAB2F}, {0xAB70, 0xABFF}, {0xAC00, 0xD7A3}, {0xF900, 0xFAFF},
	{0x104B0, 0x104FF}, {0x11100, 0x1114F}, {0x16B00, 0x16B8F}, {0x16F00, 0x16F9F}, {0x1E100, 0x1E14F},
	{0x1E2C0, 0x1E2FF}, {0x1E900, 0x1E95F}, {0x20000, 0x2B739}, {0x2B740, 0x2CEA1}, {0x2CEB0, 0x2EBEF},
	{0x2F800, 0x2FA1F}, {0x30000, 0x3134F},
}

// invisibili: caratteri che non si vedono (controllo, larghezza zero, direzione del testo,
// riempitivi coreani, selettori di variante, segni di formato siriaci e arabi, etichette...).
var invisibili = [][2]rune{
	{0x0000, 0x001F}, {0x007F, 0x009F}, {0x00AD, 0x00AD}, {0x034F, 0x034F}, {0x0600, 0x0605},
	{0x061C, 0x061C}, {0x06DD, 0x06DD}, {0x070F, 0x070F}, {0x08E2, 0x08E2}, {0x115F, 0x1160},
	{0x17B4, 0x17B5}, {0x180B, 0x180F}, {0x2000, 0x200F}, {0x2028, 0x202F}, {0x205F, 0x206F},
	{0x3000, 0x3000}, {0x3164, 0x3164}, {0xFE00, 0xFE0F}, {0xFEFF, 0xFEFF}, {0xFFA0, 0xFFA0},
	{0xFFF0, 0xFFFB}, {0xE0000, 0xE0FFF},
}

func dentro(r rune, t [][2]rune) bool {
	for _, x := range t {
		if r >= x[0] && r <= x[1] {
			return true
		}
	}
	return false
}

// spazio: gli spazi che Pulisci unisce (gli stessi della pagina).
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
	lettera, dopoLettera, accenti := false, false, 0
	for _, r := range s {
		switch {
		case r == ' ' || r == '.' || r == '-' || r == '_' || r == '\'' || r == '’':
			dopoLettera, accenti = false, 0
		case dentro(r, invisibili):
			return Esito{Codice: Invisibile}
		case !dentro(r, blocchiAmmessi):
			return Esito{Codice: Simbolo, Simbolo: string(r)}
		case unicode.IsLetter(r) || unicode.Is(unicode.Nd, r):
			lettera, dopoLettera, accenti = true, true, 0
		case unicode.Is(unicode.M, r):
			// tutti i segni vanno dopo una lettera; contano come "accenti" (al massimo 3 di fila)
			// solo quelli che si appoggiano sopra o sotto (Mn, Me): le vocali che occupano posto
			// (Mc, per esempio in birmano, tamil o bengali) non sono zalgo
			if !dopoLettera {
				return Esito{Codice: Accenti}
			}
			if !unicode.Is(unicode.Mc, r) {
				if accenti++; accenti > MaxAccenti {
					return Esito{Codice: Accenti}
				}
			}
		default:
			return Esito{Codice: Simbolo, Simbolo: string(r)}
		}
	}
	if !lettera {
		return Esito{Codice: Senza}
	}
	return Esito{}
}

// ==== REGOLE DEL NOME: FINE ====

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
