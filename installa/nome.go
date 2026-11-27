package installa

// "Il tuo nome" (3.2): il passo "Come ti chiami?" dell'installatore. Il nome lo usano Tasso,
// Tasso Zip e Tasso PDF per salutarti. Sta solo sul PC, per ogni utente di Windows:
// HKCU\Software\Tasso, valore "Nome" (REG_SZ). Non va mai in rete.
//
// Lo scrivo nell'HKCU di chi usa il PC: se l'installatore gira come un altro account (UAC con la
// password di un altro amministratore) solo in quello dell'utente della sessione (RegistroUtente),
// che è quello che useranno Tasso, Zip e PDF; nel profilo dell'amministratore no.
//
// Le regole stanno in nome/ (copia identica di quelle di Tasso, Zip e PDF, controllata da un test).

import "tassosetup/nome"

const (
	ValoreNome          = "Nome"
	valoreNomeBiglietto = "NomeBiglietto" // l'interruttore del biglietto di Tasso (lo scrive Tasso)
)

// PulisciNome: spazi all'inizio e alla fine via, più spazi di fila diventano uno (nome.Pulisci).
func PulisciNome(s string) string { return nome.Pulisci(s) }

// ControllaNome: "" se il nome (già pulito) va bene, altrimenti il codice dell'errore ("corto",
// "lungo", "simbolo", "invisibile", "senza", "accenti") e il simbolo che non va.
func ControllaNome(s string) (codice, simbolo string) {
	e := nome.Controlla(s)
	return e.Codice, e.Simbolo
}

// NomeValido: il nome così com'è si può salvare (già pulito e con le regole giuste).
func NomeValido(s string) bool { return nome.Valido(s) }

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
	// UAC con un altro amministratore: il nome è di chi usa il PC, non dell'amministratore che ha
	// messo la password. Va solo nell'HKCU dell'utente della sessione (dove lo leggono Tasso, Zip e
	// PDF); nel profilo dell'amministratore non si scrive niente.
	r := in.Registro
	if in.RegistroUtente != nil {
		r = in.RegistroUtente
	}
	if err := r.ScriviTesto(HKCU, ChiaveTasso, ValoreNome, n); err != nil {
		Diario("nome non scritto: %v", err)
	}
}

// LeggiNome: il nome già salvato di chi usa il PC (l'utente della sessione, se è un altro account),
// "" se non c'è o non è valido. Serve a riempire il campo quando reinstalli o aggiorni dalla finestra.
func (in *Installatore) LeggiNome() string {
	r := in.Registro
	if in.RegistroUtente != nil {
		r = in.RegistroUtente
	}
	if r == nil {
		return ""
	}
	if v, ok := r.LeggiTesto(HKCU, ChiaveTasso, ValoreNome); ok && NomeValido(v) {
		return v
	}
	return ""
}
