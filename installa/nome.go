package installa

// "Il tuo nome" (3.2): il passo "Come ti chiami?" dell'installatore. Il nome lo usano Tasso,
// Tasso Zip e Tasso PDF per salutarti. Sta solo sul PC, per ogni utente di Windows:
// HKCU\Software\Tasso, valore "Nome" (REG_SZ). Non va mai in rete.
//
// Come la lingua (scriviRegistro): lo scrivo in HKCU di chi fa girare l'installatore e, se è un
// altro account (UAC con la password di un altro amministratore), anche in quello dell'utente della
// sessione (RegistroUtente), che è quello che userà Tasso, Zip e PDF.
//
// Le regole sono le stesse di tasso/codice/nomeutente (copiate: l'installatore è un modulo a sé):
// da 2 a 20 caratteri, lettere (anche accentate), numeri, spazio, punto, trattino e trattino basso,
// almeno una lettera o un numero. Prima si puliscono gli spazi. Se cambi qui, cambia anche lì e
// nella pagina (ui/index.html, nomeControlla).

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	ValoreNome          = "Nome"
	valoreNomeBiglietto = "NomeBiglietto" // l'interruttore del biglietto di Tasso (lo scrive Tasso)
	NomeMin             = 2
	NomeMax             = 20
)

func nomeSpazio(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', '\u00a0', '\u1680', '\u2028', '\u2029', '\u202f', '\u205f', '\u3000', '\ufeff':
		return true
	}
	return r >= '\u2000' && r <= '\u200a'
}

// PulisciNome: spazi all'inizio e alla fine via, più spazi di fila diventano uno.
func PulisciNome(s string) string {
	if !utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	prima := false
	for _, r := range s {
		if nomeSpazio(r) {
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

// ControllaNome: "" se il nome (già pulito) va bene, altrimenti "corto", "lungo", "simbolo",
// "invisibile" o "senza" (come nomeutente.Controlla).
func ControllaNome(s string) string {
	if !utf8.ValidString(s) {
		return "invisibile"
	}
	n := utf8.RuneCountInString(s)
	if n < NomeMin {
		return "corto"
	}
	if n > NomeMax {
		return "lungo"
	}
	i, lettera := 0, false
	for _, r := range s {
		switch {
		case r == ' ' || r == '.' || r == '-' || r == '_':
		case unicode.IsLetter(r) || unicode.Is(unicode.Nd, r):
			lettera = true
		case unicode.Is(unicode.M, r) && i > 0:
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Z, r) || unicode.Is(unicode.Co, r) || unicode.Is(unicode.Cs, r) || !unicode.IsGraphic(r):
			return "invisibile"
		default:
			return "simbolo"
		}
		i++
	}
	if !lettera {
		return "senza"
	}
	return ""
}

// NomeValido: il nome così com'è si può salvare (già pulito e con le regole giuste).
func NomeValido(s string) bool { return s == PulisciNome(s) && ControllaNome(s) == "" }

// scriviNome salva il nome scritto nell'installatore. "" (aggiornamento automatico o silenzioso)
// non tocca niente: il nome che c'è resta. Un nome non valido non si scrive mai.
func (in *Installatore) scriviNome(scritto string) {
	if scritto == "" || in.Registro == nil {
		return
	}
	n := PulisciNome(scritto)
	if !NomeValido(n) {
		Diario("nome non valido: non lo salvo")
		return
	}
	if err := in.Registro.ScriviTesto(HKCU, ChiaveTasso, ValoreNome, n); err != nil {
		Diario("nome non scritto: %v", err)
	}
	if ru := in.RegistroUtente; ru != nil {
		if err := ru.ScriviTesto(HKCU, ChiaveTasso, ValoreNome, n); err != nil {
			Diario("nome non scritto per l'utente della sessione: %v", err)
		}
	}
}

// LeggiNome: il nome già salvato (prima quello dell'utente della sessione), "" se non c'è o non è
// valido. Serve a riempire il campo quando reinstalli o aggiorni dalla finestra.
func (in *Installatore) LeggiNome() string {
	for _, r := range []Registro{in.RegistroUtente, in.Registro} {
		if r == nil {
			continue
		}
		if v, ok := r.LeggiTesto(HKCU, ChiaveTasso, ValoreNome); ok {
			if NomeValido(v) {
				return v
			}
			return ""
		}
	}
	return ""
}
