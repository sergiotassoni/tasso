package installa

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
)

// NomeElenco è il file, dentro i file incorporati, che descrive i programmi.
// Lo scrive strumenti/prepara al momento della compilazione.
const NomeElenco = "elenco.json"

// FileElenco: un file da installare, con la sua impronta SHA-256.
type FileElenco struct {
	Nome   string `json:"nome"` // relativo alla cartella del programma, con /
	SHA256 string `json:"sha256"`
	Dim    int64  `json:"dim"`
}

// ProgElenco: un programma incorporato. I suoi file sono in <ID>/<Nome>.
type ProgElenco struct {
	ID       Programma    `json:"id"`
	Nome     string       `json:"nome"`
	Versione string       `json:"versione"`
	Avvio    string       `json:"avvio,omitempty"` // exe da aprire (collegamenti, icona)
	File     []FileElenco `json:"file"`
}

// Elenco: tutti i programmi dentro TassoSetup.exe.
type Elenco struct {
	Versione  string       `json:"versione"` // versione dell'ecosistema (DisplayVersion)
	Programmi []ProgElenco `json:"programmi"`
}

// LeggiElenco legge e controlla elenco.json.
func LeggiElenco(sorgente fs.FS) (*Elenco, error) {
	b, err := fs.ReadFile(sorgente, NomeElenco)
	if err != nil {
		return nil, fmt.Errorf("nessun programma dentro l'installatore (%w)", err)
	}
	var e Elenco
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, fmt.Errorf("elenco dei programmi rovinato: %w", err)
	}
	visti := map[Programma]bool{}
	for _, p := range e.Programmi {
		switch p.ID {
		case ProgTasso, ProgZip, ProgPDF, ProgSetup:
		default:
			return nil, fmt.Errorf("programma sconosciuto %q nell'elenco", p.ID)
		}
		if visti[p.ID] {
			return nil, fmt.Errorf("programma %q due volte nell'elenco", p.ID)
		}
		visti[p.ID] = true
		if len(p.File) == 0 {
			return nil, fmt.Errorf("il programma %q non ha file", p.ID)
		}
		for _, f := range p.File {
			if !nomeSicuro(f.Nome) || f.Nome == FileStato {
				return nil, fmt.Errorf("nome di file non valido %q", f.Nome)
			}
			if h, err := hex.DecodeString(f.SHA256); err != nil || len(h) != 32 {
				return nil, fmt.Errorf("impronta SHA-256 non valida per %q", f.Nome)
			}
		}
	}
	return &e, nil
}

// Prog restituisce il programma con quell'ID (nil se non è incorporato).
func (e *Elenco) Prog(id Programma) *ProgElenco {
	for i := range e.Programmi {
		if e.Programmi[i].ID == id {
			return &e.Programmi[i]
		}
	}
	return nil
}

// Dimensione: byte da scrivere per questi programmi.
func (e *Elenco) Dimensione(pp []Programma) int64 {
	var n int64
	for _, id := range pp {
		if p := e.Prog(id); p != nil {
			for _, f := range p.File {
				n += f.Dim
			}
		}
	}
	return n
}
