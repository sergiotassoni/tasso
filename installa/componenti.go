// Package installa contiene la logica di Tasso Setup che non dipende da Windows:
// scelta dei componenti, cartelle, elenco dei file incorporati, installazione atomica,
// registro (attraverso un'interfaccia) e disinstallazione selettiva.
// Così si prova tutto anche su Linux con "go test".
package installa

import (
	"fmt"
	"strings"
)

// Componente è una casella della pagina "Componenti".
type Componente string

const (
	Gaming  Componente = "gaming"  // Tasso in modalità Gaming
	Ufficio Componente = "ufficio" // Tasso in modalità Ufficio
	Zip     Componente = "zip"     // Tasso Zip
	PDF     Componente = "pdf"     // Tasso PDF
)

// TuttiComponenti nell'ordine in cui compaiono (tutti spuntati di base).
var TuttiComponenti = []Componente{Gaming, Ufficio, Zip, PDF}

// Programma è un gruppo di file che si installa in una cartella.
// Gaming e Ufficio sono due modalità dello stesso Tasso.exe: un solo programma.
type Programma string

const (
	ProgTasso Programma = "tasso" // Tasso.exe, in <cartella>
	ProgZip   Programma = "zip"   // TassoZip.exe, in <cartella>\Tasso Zip
	ProgPDF   Programma = "pdf"   // TassoPDF.exe, in <cartella>\Tasso PDF
	ProgSetup Programma = "setup" // il disinstallatore, in <cartella>
)

// ProgrammaDi dice quale programma serve per un componente.
func ProgrammaDi(c Componente) Programma {
	switch c {
	case Gaming, Ufficio:
		return ProgTasso
	case Zip:
		return ProgZip
	case PDF:
		return ProgPDF
	}
	return ""
}

// Normalizza toglie i doppioni, mette i componenti nell'ordine standard e rifiuta
// quelli sconosciuti. Una scelta vuota è un errore: non c'è niente da installare.
func Normalizza(cc []Componente) ([]Componente, error) {
	c, err := normalizza(cc)
	if err == nil && len(c) == 0 {
		err = fmt.Errorf("nessun componente scelto")
	}
	return c, err
}

func normalizza(cc []Componente) ([]Componente, error) {
	scelti := map[Componente]bool{}
	for _, c := range cc {
		c = Componente(strings.ToLower(strings.TrimSpace(string(c))))
		if ProgrammaDi(c) == "" {
			return nil, fmt.Errorf("componente sconosciuto %q", c)
		}
		scelti[c] = true
	}
	var out []Componente
	for _, c := range TuttiComponenti {
		if scelti[c] {
			out = append(out, c)
		}
	}
	return out, nil
}

// Unione dei componenti di due scelte (per l'aggiornamento: quello che c'era resta).
func Unione(a, b []Componente) []Componente {
	out, _ := normalizza(append(append([]Componente{}, a...), b...))
	return out
}

// Meno restituisce i componenti di a che non sono in b.
func Meno(a, b []Componente) []Componente {
	via := map[Componente]bool{}
	for _, c := range b {
		via[c] = true
	}
	var out []Componente
	for _, c := range a {
		if !via[c] {
			out = append(out, c)
		}
	}
	out, _ = normalizza(out)
	return out
}

// ProgrammiPer restituisce i programmi da installare per i componenti scelti
// (senza il disinstallatore, che l'installatore aggiunge sempre).
func ProgrammiPer(cc []Componente) []Programma {
	var out []Programma
	visti := map[Programma]bool{}
	for _, c := range TuttiComponenti {
		for _, x := range cc {
			if x == c && !visti[ProgrammaDi(c)] {
				visti[ProgrammaDi(c)] = true
				out = append(out, ProgrammaDi(c))
			}
		}
	}
	return out
}

// Valori di HKLM\Software\Tasso\Modalita, con gli stessi nomi del profilo di Tasso
// (settings.json "profile"): "gioco", "ufficio", "entrambi".
const (
	ModalitaGioco    = "gioco"
	ModalitaUfficio  = "ufficio"
	ModalitaEntrambi = "entrambi"
)

// ModalitaTasso dice in che modalità deve partire Tasso. Con una sola modalità
// scelta Tasso deve anche nascondere l'altra (vedi LEGGIMI: serve una modifica in Tasso).
// "" = Tasso non installato.
func ModalitaTasso(cc []Componente) string {
	g, u := false, false
	for _, c := range cc {
		g = g || c == Gaming
		u = u || c == Ufficio
	}
	switch {
	case g && u:
		return ModalitaEntrambi
	case g:
		return ModalitaGioco
	case u:
		return ModalitaUfficio
	}
	return ""
}

// TestoComponenti: "gaming,ufficio,zip,pdf" (valore Componenti nel registro).
func TestoComponenti(cc []Componente) string {
	s := make([]string, len(cc))
	for i, c := range cc {
		s[i] = string(c)
	}
	return strings.Join(s, ",")
}

// LeggiComponenti fa il contrario di TestoComponenti (ignora i nomi sconosciuti).
func LeggiComponenti(s string) []Componente {
	var out []Componente
	for _, p := range strings.Split(s, ",") {
		c := Componente(strings.TrimSpace(p))
		if ProgrammaDi(c) != "" {
			out = append(out, c)
		}
	}
	out, _ = normalizza(out)
	return out
}
