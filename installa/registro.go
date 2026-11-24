package installa

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

// Radice del registro di Windows.
type Radice int

const (
	HKLM Radice = iota // HKEY_LOCAL_MACHINE (tutto il PC)
	HKCU               // HKEY_CURRENT_USER (l'utente)
)

// Chiavi usate dall'installatore.
const (
	// "Programmi e funzionalità" / "App installate".
	ChiaveDisinstalla = `Software\Microsoft\Windows\CurrentVersion\Uninstall\Tasso`
	// Impostazioni dell'ecosistema: HKLM per l'installazione, HKCU per la lingua.
	ChiaveTasso = `Software\Tasso`
)

// Registro: quello vero su Windows (sistema_windows.go) o RegistroMemoria nei test.
type Registro interface {
	ScriviTesto(r Radice, chiave, nome, valore string) error
	ScriviNumero(r Radice, chiave, nome string, valore uint32) error
	LeggiTesto(r Radice, chiave, nome string) (string, bool)
	TogliValore(r Radice, chiave, nome string) error
	// TogliChiave toglie la chiave con i suoi valori (senza sottochiavi). Se non c'è va bene.
	TogliChiave(r Radice, chiave string) error
	// Vuota dice se la chiave non ha valori né sottochiavi (o non esiste).
	Vuota(r Radice, chiave string) bool
}

// ErrNonTrovato: valore o chiave che non c'è.
var ErrNonTrovato = errors.New("non trovato nel registro")

// RegistroMemoria simula il registro (per i test e per le prove su Linux).
type RegistroMemoria struct {
	mu      sync.Mutex
	chiavi  map[string]map[string]any
	Sbaglia func(op, chiave, nome string) error // per provare gli errori
}

func NuovoRegistroMemoria() *RegistroMemoria {
	return &RegistroMemoria{chiavi: map[string]map[string]any{}}
}

func nomeChiave(r Radice, k string) string {
	p := "HKLM"
	if r == HKCU {
		p = "HKCU"
	}
	return p + `\` + strings.ToLower(strings.Trim(k, `\`))
}

func (m *RegistroMemoria) scrivi(r Radice, chiave, nome string, v any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Sbaglia != nil {
		if err := m.Sbaglia("scrivi", chiave, nome); err != nil {
			return err
		}
	}
	k := nomeChiave(r, chiave)
	if m.chiavi[k] == nil {
		m.chiavi[k] = map[string]any{}
	}
	m.chiavi[k][strings.ToLower(nome)] = v
	return nil
}

func (m *RegistroMemoria) ScriviTesto(r Radice, chiave, nome, valore string) error {
	return m.scrivi(r, chiave, nome, valore)
}

func (m *RegistroMemoria) ScriviNumero(r Radice, chiave, nome string, valore uint32) error {
	return m.scrivi(r, chiave, nome, valore)
}

func (m *RegistroMemoria) valore(r Radice, chiave, nome string) (any, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.chiavi[nomeChiave(r, chiave)][strings.ToLower(nome)]
	return v, ok
}

func (m *RegistroMemoria) LeggiTesto(r Radice, chiave, nome string) (string, bool) {
	v, ok := m.valore(r, chiave, nome)
	s, ok2 := v.(string)
	return s, ok && ok2
}

// LeggiNumero (solo per i test).
func (m *RegistroMemoria) LeggiNumero(r Radice, chiave, nome string) (uint32, bool) {
	v, ok := m.valore(r, chiave, nome)
	n, ok2 := v.(uint32)
	return n, ok && ok2
}

func (m *RegistroMemoria) TogliValore(r Radice, chiave, nome string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Sbaglia != nil {
		if err := m.Sbaglia("togli", chiave, nome); err != nil {
			return err
		}
	}
	k := nomeChiave(r, chiave)
	delete(m.chiavi[k], strings.ToLower(nome))
	if len(m.chiavi[k]) == 0 {
		delete(m.chiavi, k)
	}
	return nil
}

func (m *RegistroMemoria) TogliChiave(r Radice, chiave string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Sbaglia != nil {
		if err := m.Sbaglia("togli", chiave, ""); err != nil {
			return err
		}
	}
	delete(m.chiavi, nomeChiave(r, chiave))
	return nil
}

func (m *RegistroMemoria) Vuota(r Radice, chiave string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := nomeChiave(r, chiave)
	if len(m.chiavi[k]) > 0 {
		return false
	}
	for altra := range m.chiavi {
		if strings.HasPrefix(altra, k+`\`) {
			return false
		}
	}
	return true
}

// Chiavi elenca le chiavi presenti (per i test).
func (m *RegistroMemoria) Chiavi() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for k := range m.chiavi {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
