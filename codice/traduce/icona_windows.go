//go:build windows

package traduce

// Comandi per l'icona di Tasso vicino all'orologio (codice/tray.go): accende e spegne la
// traduzione dei giochi come l'interruttore della pagina Giochi (ttAccendi) e aggiorna la pagina.

// TastiTraduzione: i tasti della traduzione scelti adesso (funzione -> codice, vedi tasti.go).
func TastiTraduzione() map[string]uint32 { return tastiCompleti(leggiImp().Tasti) }

// NomeTasto: il tasto scelto adesso per una funzione della traduzione ("F7").
func NomeTasto(funzione string) string { return nomeTasto(funzione) }

// Accesa: la traduzione dei giochi è accesa.
func Accesa() bool { return acceso.Load() }

// Accendi accende o spegne la traduzione dei giochi. Se non si può accendere dice perché
// (testo in italiano: lo traduce chi lo mostra).
func Accendi(on bool) (bool, string) {
	if on {
		switch {
		case InUfficio.Load():
			return false, "In modalità Ufficio la traduzione dei giochi è spenta."
		case !avviato.Load():
			return false, "Non riesco ad accendere la traduzione adesso: chiudi Tasso e riaprilo, poi riprova."
		case !mozOK.Load() && !mozillaPresente():
			return false, "Prima scarica il traduttore veloce in Giochi → Impostazioni."
		}
	}
	r := accendi(on)
	emetti("aggiorna", stato()) // la pagina Giochi mostra subito l'interruttore giusto
	if on && !r {
		return false, "Non riesco ad accendere la traduzione adesso: chiudi Tasso e riaprilo, poi riprova."
	}
	return r, ""
}
