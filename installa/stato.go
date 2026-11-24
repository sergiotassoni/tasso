package installa

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FileStato: nella cartella di Tasso, dice cosa ha installato Tasso Setup.
// Il disinstallatore toglie solo quello che c'è scritto qui.
const FileStato = "tasso-setup.json"

// StatoProg: un programma installato.
type StatoProg struct {
	Versione string       `json:"versione"`
	Cartella string       `json:"cartella"`
	Avvio    string       `json:"avvio,omitempty"`
	File     []FileElenco `json:"file"`
}

// StatoCollegamento: un collegamento (.lnk) creato dall'installatore.
type StatoCollegamento struct {
	Percorso  string    `json:"percorso"`
	Programma Programma `json:"programma"`
}

// Stato: quello che è installato adesso.
type Stato struct {
	Versione     string                  `json:"versione"`
	Cartella     string                  `json:"cartella"`
	Lingua       string                  `json:"lingua"`
	Componenti   []Componente            `json:"componenti"`
	Programmi    map[Programma]StatoProg `json:"programmi"`
	Collegamenti []StatoCollegamento     `json:"collegamenti,omitempty"`
	Data         string                  `json:"data"` // AAAAMMGG
}

// LeggiStato legge tasso-setup.json dalla cartella. (nil, nil) se non c'è.
func LeggiStato(base string) (*Stato, error) {
	b, err := os.ReadFile(filepath.Join(base, FileStato))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s Stato
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("%s rovinato: %w", FileStato, err)
	}
	if s.Programmi == nil {
		s.Programmi = map[Programma]StatoProg{}
	}
	s.Componenti, _ = normalizza(s.Componenti)
	// le cartelle si ricavano sempre dalla cartella di Tasso: mai fidarsi di un percorso scritto nel file
	for id, p := range s.Programmi {
		p.Cartella = CartellaDi(base, id)
		s.Programmi[id] = p
	}
	s.Cartella = base
	return &s, nil
}

func (s *Stato) dati() ([]byte, error) { return json.MarshalIndent(s, "", "  ") }

// Dimensione dei file installati, in byte.
func (s *Stato) Dimensione() int64 {
	var n int64
	for _, p := range s.Programmi {
		for _, f := range p.File {
			n += f.Dim
		}
	}
	return n
}
