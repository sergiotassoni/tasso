// Package ecoagg: le parti dell'aggiornamento dell'Ecosistema Tasso (Tasso, Tasso Zip, Tasso PDF)
// che non dipendono da Windows, così si provano anche su Linux con "go test".
//
// Se Tasso è installato con l'installatore dell'ecosistema (Tasso Setup: accanto a Tasso.exe c'è
// tasso-setup.json), l'aggiornamento non scarica più solo Tasso.zip: scarica TassoSetup.exe con la
// sua impronta TassoSetup.sha256 (stessa fonte di Tasso.zip, il progetto su GitHub), e lo avvia con
// --aggiorna-silenzioso. TassoSetup.sha256 (lo scrive l'installatore, strumenti/impronta):
//
//	<sha256 di TassoSetup.exe>  TassoSetup.exe  <versione dell'ecosistema>
//	tasso <versione>
//	zip <versione>
//	pdf <versione>
package ecoagg

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// Nomi dei file pubblicati (vedi version.go/ecoupdate.go per gli indirizzi).
const (
	NomeSetup     = "TassoSetup.exe"
	NomeManifesto = "TassoSetup.sha256"
)

// Manifesto: il contenuto di TassoSetup.sha256.
type Manifesto struct {
	SHA256   string            // impronta di TassoSetup.exe, minuscola
	Versione string            // versione dell'ecosistema
	Versioni map[string]string // "tasso", "zip", "pdf" -> versione dentro TassoSetup.exe
}

var ErrManifesto = errors.New("TassoSetup.sha256 non valido")

// VersioneValida: solo cifre separate da punti (una pagina di errore non diventa un numero).
func VersioneValida(s string) bool {
	if s == "" || len(s) > 20 {
		return false
	}
	for _, p := range strings.Split(s, ".") {
		if p == "" || strings.Trim(p, "0123456789") != "" {
			return false
		}
	}
	return true
}

// LeggiManifesto legge TassoSetup.sha256. Le righe che non capisce le salta (versioni future
// possono aggiungere cose), ma la prima deve essere giusta.
func LeggiManifesto(b []byte) (Manifesto, error) {
	m := Manifesto{Versioni: map[string]string{}}
	if len(b) == 0 || len(b) > 4096 {
		return m, ErrManifesto
	}
	righe := strings.Split(strings.ReplaceAll(string(b), "\r", ""), "\n")
	f := strings.Fields(righe[0])
	if len(f) < 3 || !strings.EqualFold(f[1], NomeSetup) || !VersioneValida(f[2]) {
		return m, ErrManifesto
	}
	sha := strings.ToLower(f[0])
	if h, err := hex.DecodeString(sha); err != nil || len(h) != 32 {
		return m, ErrManifesto
	}
	m.SHA256, m.Versione = sha, f[2]
	for _, r := range righe[1:] {
		f := strings.Fields(r)
		if len(f) != 2 || !VersioneValida(f[1]) {
			continue
		}
		switch id := strings.ToLower(f[0]); id {
		case "tasso", "zip", "pdf":
			m.Versioni[id] = f[1]
		}
	}
	if len(m.Versioni) == 0 {
		return m, ErrManifesto
	}
	return m, nil
}

// PiuNuova: a è più nuova di b ("3.10" > "3.9", "1.2" = "1.2.0"). Una versione non valida in a
// non è mai più nuova; una non valida in b (installata) è la più vecchia.
func PiuNuova(a, b string) bool {
	if !VersioneValida(a) {
		return false
	}
	if !VersioneValida(b) {
		return true
	}
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		x, y := 0, 0
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// Stato: la parte di tasso-setup.json che serve a Tasso.
type Stato struct {
	Versione   string   `json:"versione"`
	Lingua     string   `json:"lingua"`
	Componenti []string `json:"componenti"`
	Programmi  map[string]struct {
		Versione string `json:"versione"`
	} `json:"programmi"`
}

// LeggiStato legge tasso-setup.json.
func LeggiStato(b []byte) (*Stato, error) {
	if len(b) > 4<<20 {
		return nil, errors.New("tasso-setup.json troppo grande")
	}
	var s Stato
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Ha: quel programma ("tasso", "zip", "pdf") è installato.
func (s *Stato) Ha(id string) bool {
	if s == nil {
		return false
	}
	_, ok := s.Programmi[id]
	return ok
}

// Componente: una riga di Impostazioni → Aggiornamenti.
type Componente struct {
	ID          string `json:"id"`
	Nome        string `json:"nome"`
	Installata  string `json:"installata"`
	Disponibile string `json:"disponibile,omitempty"`
	Nuova       bool   `json:"nuova,omitempty"`
}

var nomi = []struct{ ID, Nome string }{{"tasso", "Tasso"}, {"zip", "Tasso Zip"}, {"pdf", "Tasso PDF"}}

// Componenti: i programmi installati, con la versione installata e (se m non è nil) quella
// disponibile. Per Tasso vale la più nuova tra tasso-setup.json e quella che gira (tassoVer).
func Componenti(s *Stato, m *Manifesto, tassoVer string) []Componente {
	var out []Componente
	for _, n := range nomi {
		if !s.Ha(n.ID) {
			continue
		}
		c := Componente{ID: n.ID, Nome: n.Nome, Installata: s.Programmi[n.ID].Versione}
		if n.ID == "tasso" && PiuNuova(tassoVer, c.Installata) {
			c.Installata = tassoVer
		}
		if m != nil {
			c.Disponibile = m.Versioni[n.ID]
			c.Nuova = PiuNuova(c.Disponibile, c.Installata)
		}
		out = append(out, c)
	}
	return out
}

// QualcosaDiNuovo: almeno un programma installato ha una versione più nuova.
func QualcosaDiNuovo(cc []Componente) bool {
	for _, c := range cc {
		if c.Nuova {
			return true
		}
	}
	return false
}
