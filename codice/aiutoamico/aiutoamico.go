// Package aiutoamico: la scelta di come aprire "Assistenza rapida" (Quick Assist), il programma
// gratuito di Windows con cui un amico vede o usa il tuo PC da lontano. Tasso non usa server suoi:
// apre soltanto il programma di Microsoft, oppure la sua pagina nello Store se manca.
// Logica pura: gira anche su Linux (go test ./aiutoamico/).
package aiutoamico

import (
	"encoding/xml"
	"regexp"
	"strings"
)

const (
	// Pacchetto dello Store di Assistenza rapida (Windows 10 e 11).
	Pacchetto = "MicrosoftCorporationII.QuickAssist"
	// Store: il pacchetto dello Store stesso, per sapere se si può aprire la sua pagina.
	Store = "Microsoft.WindowsStore"
	// IDStore: il codice di Assistenza rapida nel Microsoft Store.
	IDStore = "9P7BP5VNWKX5"
	// AppIDPredefinito: l'Id dell'applicazione nel pacchetto, se il manifest non si legge.
	AppIDPredefinito = "App"
	// EditorePredefinito: l'id dell'editore Microsoft nei nomi dei pacchetti.
	EditorePredefinito = "8wekyb3d8bbwe"
	// Protocollo: usato solo se il pacchetto lo dichiara e non si riesce a fare l'indirizzo dell'app.
	Protocollo = "ms-quick-assist:"
	// PaginaStore: la pagina di Assistenza rapida nell'app Microsoft Store.
	PaginaStore = "ms-windows-store://pdp/?ProductId=" + IDStore
	// PaginaWeb: la stessa pagina nel sito dello Store (se l'app Store non c'è).
	PaginaWeb = "https://apps.microsoft.com/detail/" + IDStore
	// prefissoApp: le app dello Store si aprono con shell:AppsFolder\<famiglia>!<Id app>.
	prefissoApp = `shell:AppsFolder\`
)

// Modi di apertura.
const (
	ModoApp   = "app"   // Assistenza rapida c'è: la apro
	ModoStore = "store" // non c'è: apro la sua pagina nell'app Microsoft Store
	ModoWeb   = "web"   // non c'è nemmeno lo Store: apro la pagina dello Store nel browser
)

// Installato: un pacchetto dell'utente, con quello che dice il suo manifest (se letto).
type Installato struct {
	NomeCompleto string   // "Nome_1.2.3.0_x64__8wekyb3d8bbwe"
	AppID        string   // Id dell'<Application> nel manifest ("" = non letto)
	Protocolli   []string // protocolli dichiarati nel manifest
}

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

var reParte = regexp.MustCompile(`^[A-Za-z0-9.\-]+$`)

// Famiglia: dal nome completo il nome di famiglia "Nome_IdEditore" (nome e id editore sono la
// prima e l'ultima parte). "" se il nome non ha la forma giusta.
func Famiglia(full string) string {
	parti := strings.Split(strings.TrimSpace(full), "_")
	if len(parti) != 5 || !reParte.MatchString(parti[0]) || !reParte.MatchString(parti[4]) {
		return ""
	}
	return parti[0] + "_" + parti[4]
}

// LeggiManifest: dal file AppxManifest.xml l'Id della prima applicazione e i protocolli.
func LeggiManifest(b []byte) (appID string, protocolli []string) {
	var m struct {
		Apps []struct {
			ID         string `xml:"Id,attr"`
			Estensioni []struct {
				Protocolli []struct {
					Nome string `xml:"Name,attr"`
				} `xml:"Protocol"`
			} `xml:"Extensions>Extension"`
		} `xml:"Applications>Application"`
	}
	if xml.Unmarshal(b, &m) != nil || len(m.Apps) == 0 {
		return "", nil
	}
	for _, a := range m.Apps {
		for _, e := range a.Estensioni {
			for _, p := range e.Protocolli {
				if p.Nome != "" {
					protocolli = append(protocolli, strings.ToLower(p.Nome))
				}
			}
		}
	}
	return m.Apps[0].ID, protocolli
}

// IndirizzoApp: "shell:AppsFolder\<famiglia>!<Id app>" (AUMID), il modo con cui Tasso apre già le
// app dello Store. Niente finestra "Ti serve una nuova app": il pacchetto è già verificato.
func IndirizzoApp(p Installato) string {
	f := Famiglia(p.NomeCompleto)
	if f == "" {
		return ""
	}
	id := p.AppID
	if id == "" {
		id = AppIDPredefinito
	}
	if !reParte.MatchString(id) {
		return ""
	}
	return prefissoApp + f + "!" + id
}

func trova(pacchetti []Installato, nome string) (Installato, bool) {
	for _, p := range pacchetti {
		if strings.EqualFold(nomeDi(p.NomeCompleto), nome) {
			return p, true
		}
	}
	return Installato{}, false
}

// Scegli: con i pacchetti dell'utente decide cosa aprire.
// Assistenza rapida installata -> l'app (AUMID; il protocollo solo se l'AUMID non si fa e il
// manifest lo dichiara; se no l'AUMID della famiglia Microsoft). Se manca: la pagina dello Store;
// se manca anche lo Store, la stessa pagina sul sito.
func Scegli(pacchetti []Installato) Scelta {
	if p, ok := trova(pacchetti, Pacchetto); ok {
		if u := IndirizzoApp(p); u != "" {
			return Scelta{ModoApp, u}
		}
		for _, pr := range p.Protocolli {
			if pr+":" == Protocollo {
				return Scelta{ModoApp, Protocollo}
			}
		}
		return Scelta{ModoApp, prefissoApp + Pacchetto + "_" + EditorePredefinito + "!" + AppIDPredefinito}
	}
	if _, ok := trova(pacchetti, Store); ok {
		return Scelta{ModoStore, PaginaStore}
	}
	return Scelta{ModoWeb, PaginaWeb}
}

var reApp = regexp.MustCompile(`^shell:AppsFolder\\` + regexp.QuoteMeta(Pacchetto) + `_[A-Za-z0-9]+![A-Za-z0-9.\-]+$`)

// IndirizzoAmmesso: si possono aprire solo Assistenza rapida e la sua pagina nello Store.
func IndirizzoAmmesso(u string) bool {
	return u == Protocollo || u == PaginaStore || u == PaginaWeb || reApp.MatchString(u)
}
