package ottimizza

import (
	"errors"
	"strings"
)

// Registro, piano energetico e desktop finti, per provare tutto anche su Linux.

type regFinto struct {
	v        map[string]Valore
	bloccate map[Radice]bool // radici non accessibili (come senza amministratore)
	rotte    map[string]bool // valori che Windows non lascia scrivere
	ignora   map[string]bool // valori che "non restano" (Windows li rimette subito)
}

func chiaveFinta(r Radice, k, n string) string {
	return string(r) + `\` + strings.ToLower(k) + `|` + strings.ToLower(n)
}

func nuovoReg() *regFinto {
	return &regFinto{v: map[string]Valore{}, bloccate: map[Radice]bool{}, rotte: map[string]bool{}, ignora: map[string]bool{}}
}

func (f *regFinto) Leggi(r Radice, k, n string) (Valore, error) {
	if f.bloccate[r] {
		return Valore{}, ErrNonAccessibile
	}
	if v, ok := f.v[chiaveFinta(r, k, n)]; ok {
		return v, nil
	}
	return Valore{Radice: r, Chiave: k, Nome: n}, nil
}

func (f *regFinto) Scrivi(v Valore) error {
	c := chiaveFinta(v.Radice, v.Chiave, v.Nome)
	if f.bloccate[v.Radice] || f.rotte[c] {
		return errors.New("accesso negato")
	}
	if f.ignora[c] {
		return nil
	}
	f.v[c] = v
	return nil
}

func (f *regFinto) Cancella(r Radice, k, n string) error {
	if f.bloccate[r] {
		return errors.New("accesso negato")
	}
	delete(f.v, chiaveFinta(r, k, n))
	return nil
}

func (f *regFinto) Nomi(r Radice, k string) ([]string, error) { return nil, nil }

func (f *regFinto) metti(v Valore) { f.v[chiaveFinta(v.Radice, v.Chiave, v.Nome)] = v }

// copia: per confrontare prima e dopo.
func (f *regFinto) copia() map[string]Valore {
	out := map[string]Valore{}
	for k, v := range f.v {
		out[k] = v
	}
	return out
}

type enFinta struct {
	attivo, overlay string
	piani           map[string]bool
	val             map[string][2]uint32
}

func nuovaEn() *enFinta {
	return &enFinta{attivo: PianoBilanciato, overlay: OverlayNessuno, piani: map[string]bool{PianoBilanciato: true, PianoPrestazioni: true}, val: map[string][2]uint32{}}
}

func (e *enFinta) PianoAttivo() (string, error) { return e.attivo, nil }
func (e *enFinta) PianoEsiste(g string) bool    { return e.piani[g] }
func (e *enFinta) AttivaPiano(g string) error {
	if !e.piani[g] {
		return errors.New("piano che non c'è")
	}
	e.attivo = g
	return nil
}
func (e *enFinta) Overlay() (string, error)      { return e.overlay, nil }
func (e *enFinta) ImpostaOverlay(g string) error { e.overlay = g; return nil }
func (e *enFinta) Leggi(p, g, i string) (uint32, uint32, error) {
	if !e.piani[p] {
		return 0, 0, errors.New("piano che non c'è")
	}
	x := e.val[p+g+i]
	return x[0], x[1], nil
}
func (e *enFinta) ScriviAC(p, g, i string, v uint32) error {
	x := e.val[p+g+i]
	x[0] = v
	e.val[p+g+i] = x
	return nil
}
func (e *enFinta) ScriviDC(p, g, i string, v uint32) error {
	x := e.val[p+g+i]
	x[1] = v
	e.val[p+g+i] = x
	return nil
}

type sisFinto struct {
	spi    map[string]bool
	mouse  [3]int
	avvisi []string
}

func nuovoSis() *sisFinto {
	return &sisFinto{spi: map[string]bool{SPIAnimazioni: true, SPIFinestre: true, SPIOmbre: true}, mouse: [3]int{6, 10, 1}}
}

func (s *sisFinto) LeggiSPI(id string) (bool, error) { return s.spi[id], nil }
func (s *sisFinto) ScriviSPI(id string, on bool) error {
	s.spi[id] = on
	return nil
}
func (s *sisFinto) ImpostaMouse(a, b, c int) error { s.mouse = [3]int{a, b, c}; return nil }
func (s *sisFinto) Avvisa(area string)             { s.avvisi = append(s.avvisi, area) }

type prefFinte struct{ notif bool }

func (p *prefFinte) NotificheGioco() bool                { return p.notif }
func (p *prefFinte) ImpostaNotificheGioco(on bool) error { p.notif = on; return nil }

type pulFinto struct{ fatti []string }

func (p *pulFinto) Pulisci(id string) (int64, error) {
	p.fatti = append(p.fatti, id)
	return 1 << 20, nil
}

// ambienteFinto: un PC fisso da amministratore con Windows 11 e due schede video.
func ambienteFinto() (*Ambiente, *regFinto, *enFinta, *sisFinto) {
	r, e, s := nuovoReg(), nuovaEn(), nuovoSis()
	// valori di base di un Windows 11 appena installato
	r.metti(sz(HKCU, chiaveMouse, "MouseSpeed", "1"))
	r.metti(sz(HKCU, chiaveMouse, "MouseThreshold1", "6"))
	r.metti(sz(HKCU, chiaveMouse, "MouseThreshold2", "10"))
	r.metti(dw(HKNS, kRecapito, "DownloadMode", 1))
	r.metti(dw(HKLM, kAggUX, "ActiveHoursStart", 8))
	r.metti(dw(HKLM, kAggUX, "ActiveHoursEnd", 17))
	r.metti(dw(HKLM, kAggUX, "SmartActiveHoursState", 1))
	r.metti(dw(HKLM, kPriorita, "Win32PrioritySeparation", 0x18))
	e.val[PianoBilanciato+GruppoUSB+ImpUSBSospensione] = [2]uint32{1, 1}
	e.val[PianoPrestazioni+GruppoUSB+ImpUSBSospensione] = [2]uint32{1, 1}
	e.val[PianoBilanciato+GruppoRisparmio+ImpRisparmioSoglia] = [2]uint32{10, 10}
	e.val[PianoBilanciato+GruppoSchermo+ImpSchermoSpento] = [2]uint32{600, 900}
	a := &Ambiente{Reg: r, En: e, Sis: s, Pref: &prefFinte{}, Pul: &pulFinto{},
		PC: PC{Admin: true, Caricatore: true, Build: 22631, SchedeVere: 2, HAGS: true, Giochi: []string{`C:\Giochi\Gioco\gioco.exe`}}}
	return a, r, e, s
}

func salvaFinto(n *int) Salvatore {
	return func(*Stato) error { *n++; return nil }
}
