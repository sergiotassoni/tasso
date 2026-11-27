// Package ottimizza: la logica (senza Windows) di "Ottimizza tutto" (Tasso 3.2).
//
// Ottimizza tutto misura il PC, cambia una serie di impostazioni sicure di Windows (14 in Gaming,
// 10 in Ufficio), misura di nuovo e mostra la differenza. Ogni modifica ha quattro parti:
//   - "già a posto": se Windows è già come serve, la salta e lo dice;
//   - applica: prima salva com'era (su disco, nella cartella protetta di Tasso), poi cambia;
//   - verifica: rilegge e controlla che il cambio ci sia davvero;
//   - rimetti com'era: rimette il valore di prima, ma solo se adesso c'è ancora quello messo da
//     Tasso (se l'hai cambiato tu, o un'altra parte di Tasso, resta com'è).
//
// Qui non c'è niente di Windows: il registro, il piano energetico e le impostazioni del desktop
// arrivano da interfacce (Registro, Energia, Sistema). Tasso passa quelle vere (../ottimizza_win.go),
// i test passano quelle finte, la build di prova quelle del "registro di prova".
package ottimizza

import (
	"errors"
	"strings"
	"time"
)

// Radice: la parte del registro.
type Radice string

const (
	HKCU Radice = "HKCU" // l'utente che usa Tasso
	HKLM Radice = "HKLM" // tutto il PC (serve l'amministratore)
	HKNS Radice = "HKNS" // HKEY_USERS\S-1-5-20 (Servizio di rete: Ottimizzazione recapito)
)

// Tipi dei valori del registro che Ottimizza tutto scrive.
const (
	TipoDword = "dword"
	TipoSz    = "sz"
)

// Valore: un valore del registro com'è (o come va scritto). Esiste=false: il valore non c'è.
type Valore struct {
	Radice Radice `json:"r"`
	Chiave string `json:"k"`
	Nome   string `json:"n"`
	Tipo   string `json:"t,omitempty"`
	Esiste bool   `json:"e"`
	Num    uint32 `json:"d,omitempty"`
	Testo  string `json:"s,omitempty"`
}

// Stesso: stesso posto nel registro (radice, chiave e nome, senza badare a maiuscole).
func (v Valore) Stesso(o Valore) bool {
	return v.Radice == o.Radice && strings.EqualFold(v.Chiave, o.Chiave) && strings.EqualFold(v.Nome, o.Nome)
}

// Uguale: stesso posto e stesso contenuto (due valori assenti sono uguali).
func (v Valore) Uguale(o Valore) bool {
	if !v.Stesso(o) || v.Esiste != o.Esiste {
		return false
	}
	if !v.Esiste {
		return true
	}
	if v.Tipo != o.Tipo {
		// "0" scritto come testo e 0 come numero non sono la stessa cosa per Windows
		return false
	}
	if v.Tipo == TipoDword {
		return v.Num == o.Num
	}
	return v.Testo == o.Testo
}

// ErrNonAccessibile: la chiave c'è ma Tasso non la può leggere o scrivere (permessi, parte del
// registro non caricata). Chi la riceve salta la modifica con un messaggio.
var ErrNonAccessibile = errors.New("parte del registro non accessibile")

// ErrTipoDiverso: il valore c'è ma è di un tipo che Tasso non scrive mai (per esempio l'hai
// cambiato tu in binario): per "rimetti com'era" vale come "l'hai cambiato tu".
var ErrTipoDiverso = errors.New("valore di un tipo diverso")

// Registro: leggere e scrivere valori. Un valore o una chiave che non c'è NON è un errore: torna
// Esiste=false.
type Registro interface {
	Leggi(r Radice, chiave, nome string) (Valore, error)
	Scrivi(v Valore) error
	Cancella(r Radice, chiave, nome string) error
	// Nomi: i nomi dei valori di una chiave (per le preferenze della scheda video dei giochi).
	Nomi(r Radice, chiave string) ([]string, error)
}

// Energia: il piano energetico di Windows (powrprof). I GUID sono scritti come testo minuscolo.
type Energia interface {
	PianoAttivo() (string, error)
	PianoEsiste(guid string) bool
	AttivaPiano(guid string) error
	// Overlay: la "modalità di risparmio energia" di Windows 10/11 (Bilanciato, Prestazioni migliori...).
	Overlay() (string, error)
	ImpostaOverlay(guid string) error
	// Valore di un'impostazione di un piano: a corrente (AC) e a batteria (DC).
	Leggi(piano, gruppo, impostazione string) (ac, dc uint32, err error)
	ScriviAC(piano, gruppo, impostazione string, v uint32) error
	ScriviDC(piano, gruppo, impostazione string, v uint32) error
}

// Sistema: impostazioni del desktop che Windows tiene da sé (SystemParametersInfo). Gli id sono
// quelli di SPI* qui sotto. Scrivi le salva anche per i prossimi accessi.
type Sistema interface {
	LeggiSPI(id string) (bool, error)
	ScriviSPI(id string, on bool) error
	// ImpostaMouse: soglie e velocità dell'accelerazione del mouse, subito e per i prossimi accessi.
	ImpostaMouse(soglia1, soglia2, velocita int) error
	// Avvisa: dice a Windows e ai programmi aperti che un'impostazione è cambiata (WM_SETTINGCHANGE).
	Avvisa(area string)
}

// Impostazioni di SystemParametersInfo usate da Ottimizza tutto.
const (
	SPIAnimazioni = "animazioni" // SPI_GETCLIENTAREAANIMATION: animazioni di Windows
	SPIFinestre   = "finestre"   // SPI_GETANIMATION: animazione di riduzione/ingrandimento
	SPIOmbre      = "ombre"      // SPI_GETDROPSHADOW: ombre sotto le finestre
)

// SPINoto: gli id che Ottimizza tutto conosce (un file rovinato non ne fa usare altri).
func SPINoto(id string) bool {
	return id == SPIAnimazioni || id == SPIFinestre || id == SPIOmbre
}

// Preferenze: le scelte di Tasso stesso (non di Windows) che Ottimizza tutto può accendere.
type Preferenze interface {
	NotificheGioco() bool
	ImpostaNotificheGioco(on bool) error
}

// PC: quello che serve sapere del PC per decidere cosa fare.
type PC struct {
	Admin     bool // Tasso gira da amministratore
	Portatile bool // regola di votopc.Portatile (un gruppo di continuità non conta); nel dubbio true
	// PortatileDubbio: Windows non ha saputo dire se è un portatile (lettura non riuscita): si tratta
	// come portatile (niente piano energetico) e lo si dice.
	PortatileDubbio bool
	// AltroUtente: Tasso gira con un altro account amministratore (UAC con la password di un altro):
	// le impostazioni dell'utente (HKCU, desktop, pulizia dei suoi file) non si toccano.
	AltroUtente bool
	// ModoTasso: una modalità di Tasso (Gaming, FPS, Film, Silenziosa) è accesa e gestisce già
	// piano energetico, mouse e Modalità Gioco: quelle voci si lasciano a lei ("" = nessuna).
	ModoTasso  string
	Caricatore bool     // a corrente (sui fissi sempre true)
	Build      int      // numero di build di Windows (19045 = Windows 10 22H2, 22000+ = Windows 11)
	SchedeVere int      // schede video vere (DirectX, senza quelle software)
	HAGS       bool     // la scheda video e il driver permettono la pianificazione GPU hardware
	HAGSAcceso bool     // la pianificazione GPU hardware è già accesa (anche di base, dal driver)
	Giochi     []string // programmi dei giochi trovati da Tasso (percorsi completi .exe)
}

// Pulitore: la pulizia di Tasso (passaggi di "Pulisci"), per i file inutili dell'Ufficio. Non si
// rimette com'era: i file tolti non servono più. Prima si fa il backup come nella pulizia Media.
type Pulitore interface {
	Pulisci(id string) (liberati int64, err error)
}

// Ambiente: tutto quello che una modifica può usare.
type Ambiente struct {
	Reg  Registro
	En   Energia
	Sis  Sistema
	Pref Preferenze
	Pul  Pulitore
	PC   PC
	// Attendi: aspetta un po' prima di ricontrollare un cambio che Windows potrebbe rimettere
	// (orari degli aggiornamenti). nil = non aspetta (prove).
	Attendi func(d time.Duration)
	// RegSenzaCopia: lo stesso registro senza la copia di sicurezza (.reg). Per rimettere com'era:
	// la copia salverebbe come "com'era" il valore di Tasso. nil = si usa Reg.
	RegSenzaCopia Registro
}

// perRimettere: l'ambiente per rimettere com'era, con il registro senza copia.
func perRimettere(a *Ambiente) *Ambiente {
	if a.RegSenzaCopia == nil {
		return a
	}
	b := *a
	b.Reg = a.RegSenzaCopia
	return &b
}
