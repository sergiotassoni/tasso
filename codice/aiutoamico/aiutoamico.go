// Package aiutoamico: la scelta di come aprire "Assistenza rapida" (Quick Assist), il programma
// gratuito di Windows con cui un amico vede o usa il tuo PC da lontano. Tasso non usa server suoi:
// apre soltanto il programma di Microsoft, oppure la sua pagina nello Store se manca.
// Logica pura: gira anche su Linux (go test ./aiutoamico/).
package aiutoamico

import "strings"

const (
	// Pacchetto dello Store di Assistenza rapida (Windows 10 e 11).
	Pacchetto = "MicrosoftCorporationII.QuickAssist"
	// Store: il pacchetto dello Store stesso, per sapere se si può aprire la sua pagina.
	Store = "Microsoft.WindowsStore"
	// IDStore: il codice di Assistenza rapida nel Microsoft Store.
	IDStore = "9P7BP5VNWKX5"
	// Protocollo con cui Windows apre Assistenza rapida.
	Protocollo = "ms-quick-assist:"
	// PaginaStore: la pagina di Assistenza rapida nell'app Microsoft Store.
	PaginaStore = "ms-windows-store://pdp/?ProductId=" + IDStore
	// PaginaWeb: la stessa pagina nel sito dello Store (se l'app Store non c'è).
	PaginaWeb = "https://apps.microsoft.com/detail/" + IDStore
)

// Modi di apertura.
const (
	ModoApp   = "app"   // Assistenza rapida c'è: la apro
	ModoStore = "store" // non c'è: apro la sua pagina nell'app Microsoft Store
	ModoWeb   = "web"   // non c'è nemmeno lo Store: apro la pagina dello Store nel browser
)

// Scelta: cosa aprire e come dirlo alla finestra.
type Scelta struct {
	Modo      string
	Indirizzo string
}

// nomeDi: dal nome completo di un pacchetto ("Nome_1.2.3.0_x64__8wekyb3d8bbwe") il nome ("Nome").
func nomeDi(p string) string {
	p = strings.TrimSpace(p)
	if i := strings.IndexByte(p, '_'); i >= 0 {
		p = p[:i]
	}
	return p
}

// HaPacchetto: tra i pacchetti installati (nomi o nomi completi) c'è quello cercato?
func HaPacchetto(pacchetti []string, nome string) bool {
	for _, p := range pacchetti {
		if strings.EqualFold(nomeDi(p), nome) {
			return true
		}
	}
	return false
}

// Scegli: con l'elenco dei pacchetti dell'utente decide cosa aprire.
// Assistenza rapida installata -> il programma; se no la pagina dello Store; se manca anche lo
// Store, la stessa pagina sul sito (si scarica da lì).
func Scegli(pacchetti []string) Scelta {
	switch {
	case HaPacchetto(pacchetti, Pacchetto):
		return Scelta{ModoApp, Protocollo}
	case HaPacchetto(pacchetti, Store):
		return Scelta{ModoStore, PaginaStore}
	default:
		return Scelta{ModoWeb, PaginaWeb}
	}
}

// IndirizzoAmmesso: la finestra può chiedere di aprire solo questi tre indirizzi, niente altro.
func IndirizzoAmmesso(u string) bool {
	return u == Protocollo || u == PaginaStore || u == PaginaWeb
}
