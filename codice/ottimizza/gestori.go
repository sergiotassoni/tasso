package ottimizza

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// Precedente: com'era prima (Valori) e come l'ha messo Tasso (Messi), più i dati che non sono
// valori del registro (Extra: piano energetico, impostazioni del desktop). Parti: per le modifiche
// fatte di più pezzi (per esempio registro + impostazioni del desktop).
type Precedente struct {
	Valori []Valore          `json:"v,omitempty"`
	Messi  []Valore          `json:"m,omitempty"`
	Extra  map[string]string `json:"x,omitempty"`
	Parti  []Precedente      `json:"p,omitempty"`
	Vuoto  bool              `json:"z,omitempty"` // questa parte era già a posto: niente da fare
}

// Rimesso: risultato di "rimetti com'era" di una modifica.
type Rimesso int

const (
	RimessoTutto  Rimesso = iota // tutto rimesso com'era
	RimessoLascia                // qualcosa era già cambiato da te (o da un'altra parte di Tasso): lasciato com'è
)

// Gestore: le quattro parti di una modifica.
type Gestore interface {
	// Disponibile: false se su questo PC la modifica non c'è o non si può fare (con il motivo).
	Disponibile(a *Ambiente) (bool, string)
	// APosto: Windows è già come serve.
	APosto(a *Ambiente) (bool, error)
	// Prepara: legge com'è adesso e decide cosa scrivere, senza scrivere niente.
	Prepara(a *Ambiente) (Precedente, error)
	// Applica: scrive quello che Prepara ha deciso.
	Applica(a *Ambiente, p Precedente) error
	// Verifica: il cambio c'è davvero (rilegge).
	Verifica(a *Ambiente, p Precedente) bool
	// Rimetti: rimette com'era, solo dove c'è ancora quello che ha messo Tasso.
	Rimetti(a *Ambiente, p Precedente) (Rimesso, error)
	// Valida: tiene di p (letto da un file) solo quello che questa modifica può davvero aver scritto.
	// admin=false: niente valori di tutto il PC (HKLM, Servizio di rete). ok=false: p non si usa.
	Valida(p Precedente, admin bool) (Precedente, bool)
}

// ErrNonDisponibile: usato dalle prove quando una modifica non c'è.
var ErrNonDisponibile = errors.New("non disponibile")

// ---------------- Registro ----------------

// bersaglio: un valore del registro, quello che serve (vuole) e quando va già bene (ok).
type bersaglio struct {
	vuole Valore
	ok    func(v Valore) bool // nil: va bene solo se è proprio vuole
}

func (b bersaglio) vaBene(v Valore) bool {
	if b.ok != nil {
		return b.ok(v)
	}
	return v.Uguale(b.vuole)
}

func dw(r Radice, k, n string, val uint32) Valore {
	return Valore{Radice: r, Chiave: k, Nome: n, Tipo: TipoDword, Esiste: true, Num: val}
}

func sz(r Radice, k, n, val string) Valore {
	return Valore{Radice: r, Chiave: k, Nome: n, Tipo: TipoSz, Esiste: true, Testo: val}
}

// dwordE: il valore è un numero uguale a uno di questi.
func dwordE(v Valore, vals ...uint32) bool {
	if !v.Esiste || v.Tipo != TipoDword {
		return false
	}
	for _, x := range vals {
		if v.Num == x {
			return true
		}
	}
	return false
}

// assenteO: il valore non c'è (Windows usa il suo valore di base, che va bene) oppure è uno di questi.
func assenteO(vals ...uint32) func(Valore) bool {
	return func(v Valore) bool { return !v.Esiste || dwordE(v, vals...) }
}

func soloQuesti(vals ...uint32) func(Valore) bool {
	return func(v Valore) bool { return dwordE(v, vals...) }
}

// regGest: una modifica fatta di valori del registro.
type regGest struct {
	bers []bersaglio
	cond func(a *Ambiente) (bool, string) // nil = sempre disponibile
	// aPosto: se c'è, decide lui "già a posto" (per esempio la pianificazione GPU già accesa dal driver)
	aPosto func(a *Ambiente) (bool, error)
	dopo   func(a *Ambiente) // dopo aver scritto o rimesso (avvisi a Windows)
}

const msgAdmin = "Per questa serve Tasso aperto come amministratore: la salto."

func serveAdmin(r Radice) bool { return r == HKLM || r == HKNS }

func (g *regGest) Disponibile(a *Ambiente) (bool, string) {
	for _, b := range g.bers {
		if serveAdmin(b.vuole.Radice) && !a.PC.Admin {
			return false, msgAdmin
		}
	}
	if g.cond != nil {
		return g.cond(a)
	}
	return true, ""
}

func (g *regGest) APosto(a *Ambiente) (bool, error) {
	if g.aPosto != nil {
		if ok, err := g.aPosto(a); err != nil || ok {
			return ok, err
		}
	}
	for _, b := range g.bers {
		v, err := a.Reg.Leggi(b.vuole.Radice, b.vuole.Chiave, b.vuole.Nome)
		if err != nil {
			return false, err
		}
		if !b.vaBene(v) {
			return false, nil
		}
	}
	return true, nil
}

func (g *regGest) Prepara(a *Ambiente) (Precedente, error) {
	var p Precedente
	for _, b := range g.bers {
		v, err := a.Reg.Leggi(b.vuole.Radice, b.vuole.Chiave, b.vuole.Nome)
		if err != nil {
			return Precedente{}, err
		}
		if b.vaBene(v) {
			continue // questo pezzo va già bene: non lo tocco
		}
		v.Radice, v.Chiave, v.Nome = b.vuole.Radice, b.vuole.Chiave, b.vuole.Nome
		p.Valori = append(p.Valori, v)
		p.Messi = append(p.Messi, b.vuole)
	}
	if len(p.Messi) == 0 {
		p.Vuoto = true
	}
	return p, nil
}

func scriviValore(r Registro, v Valore) error {
	if v.Esiste {
		return r.Scrivi(v)
	}
	return r.Cancella(v.Radice, v.Chiave, v.Nome)
}

func (g *regGest) Applica(a *Ambiente, p Precedente) error {
	for _, m := range p.Messi {
		if err := scriviValore(a.Reg, m); err != nil {
			return err
		}
	}
	if g.dopo != nil && len(p.Messi) > 0 {
		g.dopo(a)
	}
	return nil
}

func (g *regGest) Verifica(a *Ambiente, p Precedente) bool {
	for _, m := range p.Messi {
		v, err := a.Reg.Leggi(m.Radice, m.Chiave, m.Nome)
		if err != nil || !v.Uguale(m) {
			return false
		}
	}
	return true
}

// rimettiValori: rimette Valori[i] solo dove adesso c'è ancora Messi[i].
func rimettiValori(r Registro, p Precedente) (Rimesso, bool, error) {
	esito, scritto := RimessoTutto, false
	var primoErr error
	for i, m := range p.Messi {
		if i >= len(p.Valori) {
			break
		}
		cur, err := r.Leggi(m.Radice, m.Chiave, m.Nome)
		if err != nil {
			if primoErr == nil {
				primoErr = err
			}
			continue
		}
		if !cur.Uguale(m) {
			esito = RimessoLascia // l'hai cambiato tu (o un'altra parte di Tasso): resta com'è
			continue
		}
		if err := scriviValore(r, p.Valori[i]); err != nil {
			if primoErr == nil {
				primoErr = err
			}
			continue
		}
		scritto = true
	}
	return esito, scritto, primoErr
}

func (g *regGest) Rimetti(a *Ambiente, p Precedente) (Rimesso, error) {
	esito, scritto, err := rimettiValori(a.Reg, p)
	if scritto && g.dopo != nil {
		g.dopo(a)
	}
	return esito, err
}

// testoSicuro: un testo che si può rimettere nel registro (niente caratteri strani, non lungo).
func testoSicuro(s string, max int) bool {
	return len(s) <= max && !strings.ContainsAny(s, "\x00\r\n")
}

// valoreSicuro: tipo noto e contenuto ragionevole.
func valoreSicuro(v Valore) bool {
	if !v.Esiste {
		return true
	}
	switch v.Tipo {
	case TipoDword:
		return v.Testo == ""
	case TipoSz:
		return testoSicuro(v.Testo, 260)
	}
	return false
}

func (g *regGest) Valida(p Precedente, admin bool) (Precedente, bool) {
	if len(p.Valori) != len(p.Messi) || len(p.Parti) > 0 || len(p.Extra) > 0 {
		return Precedente{}, false
	}
	var out Precedente
	for i, m := range p.Messi {
		trovato := false
		for _, b := range g.bers {
			if m.Uguale(b.vuole) {
				trovato = true
				break
			}
		}
		v := p.Valori[i]
		if !trovato || !v.Stesso(m) || !valoreSicuro(v) {
			return Precedente{}, false
		}
		if serveAdmin(m.Radice) && !admin {
			continue // valore di tutto il PC da un file che può cambiare chiunque: non lo uso
		}
		out.Valori = append(out.Valori, v)
		out.Messi = append(out.Messi, m)
	}
	out.Vuoto = len(out.Messi) == 0
	return out, true
}

// ---------------- Più pezzi insieme ----------------

// insieme: una modifica fatta di più pezzi (registro e impostazioni del desktop).
type insieme struct {
	parti []Gestore
	cond  func(a *Ambiente) (bool, string)
}

func (g *insieme) Disponibile(a *Ambiente) (bool, string) {
	for _, p := range g.parti {
		if ok, m := p.Disponibile(a); !ok {
			return false, m
		}
	}
	if g.cond != nil {
		return g.cond(a)
	}
	return true, ""
}

func (g *insieme) APosto(a *Ambiente) (bool, error) {
	for _, p := range g.parti {
		ok, err := p.APosto(a)
		if err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}

func (g *insieme) Prepara(a *Ambiente) (Precedente, error) {
	out := Precedente{Vuoto: true}
	for _, p := range g.parti {
		ok, err := p.APosto(a)
		if err != nil {
			return Precedente{}, err
		}
		if ok {
			out.Parti = append(out.Parti, Precedente{Vuoto: true})
			continue
		}
		x, err := p.Prepara(a)
		if err != nil {
			return Precedente{}, err
		}
		out.Parti = append(out.Parti, x)
		if !x.Vuoto {
			out.Vuoto = false
		}
	}
	return out, nil
}

func (g *insieme) Applica(a *Ambiente, p Precedente) error {
	if len(p.Parti) != len(g.parti) {
		return errors.New("pezzi diversi")
	}
	for i, x := range p.Parti {
		if x.Vuoto {
			continue
		}
		if err := g.parti[i].Applica(a, x); err != nil {
			return err
		}
	}
	return nil
}

func (g *insieme) Verifica(a *Ambiente, p Precedente) bool {
	if len(p.Parti) != len(g.parti) {
		return false
	}
	for i, x := range p.Parti {
		if !x.Vuoto && !g.parti[i].Verifica(a, x) {
			return false
		}
	}
	return true
}

func (g *insieme) Rimetti(a *Ambiente, p Precedente) (Rimesso, error) {
	if len(p.Parti) != len(g.parti) {
		return RimessoTutto, errors.New("pezzi diversi")
	}
	esito := RimessoTutto
	var primoErr error
	for i := len(p.Parti) - 1; i >= 0; i-- {
		x := p.Parti[i]
		if x.Vuoto {
			continue
		}
		e, err := g.parti[i].Rimetti(a, x)
		if err != nil && primoErr == nil {
			primoErr = err
		}
		if e == RimessoLascia {
			esito = RimessoLascia
		}
	}
	return esito, primoErr
}

func (g *insieme) Valida(p Precedente, admin bool) (Precedente, bool) {
	if len(p.Parti) != len(g.parti) || len(p.Valori) > 0 || len(p.Messi) > 0 || len(p.Extra) > 0 {
		return Precedente{}, false
	}
	out := Precedente{Vuoto: true}
	for i, x := range p.Parti {
		if x.Vuoto {
			out.Parti = append(out.Parti, Precedente{Vuoto: true})
			continue
		}
		y, ok := g.parti[i].Valida(x, admin)
		if !ok {
			return Precedente{}, false
		}
		out.Parti = append(out.Parti, y)
		if !y.Vuoto {
			out.Vuoto = false
		}
	}
	return out, true
}

// ---------------- Impostazioni del desktop (SystemParametersInfo) ----------------

type spiGest struct {
	id    string
	vuole bool
}

func b01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func (g *spiGest) Disponibile(a *Ambiente) (bool, string) { return true, "" }

func (g *spiGest) APosto(a *Ambiente) (bool, error) {
	v, err := a.Sis.LeggiSPI(g.id)
	return err == nil && v == g.vuole, err
}

func (g *spiGest) Prepara(a *Ambiente) (Precedente, error) {
	v, err := a.Sis.LeggiSPI(g.id)
	if err != nil {
		return Precedente{}, err
	}
	if v == g.vuole {
		return Precedente{Vuoto: true}, nil
	}
	return Precedente{Extra: map[string]string{"spi": g.id, "prima": b01(v), "messo": b01(g.vuole)}}, nil
}

func (g *spiGest) Applica(a *Ambiente, p Precedente) error {
	if p.Vuoto {
		return nil
	}
	return a.Sis.ScriviSPI(g.id, p.Extra["messo"] == "1")
}

func (g *spiGest) Verifica(a *Ambiente, p Precedente) bool {
	if p.Vuoto {
		return true
	}
	v, err := a.Sis.LeggiSPI(g.id)
	return err == nil && b01(v) == p.Extra["messo"]
}

func (g *spiGest) Rimetti(a *Ambiente, p Precedente) (Rimesso, error) {
	if p.Vuoto {
		return RimessoTutto, nil
	}
	v, err := a.Sis.LeggiSPI(g.id)
	if err != nil {
		return RimessoTutto, err
	}
	if b01(v) != p.Extra["messo"] {
		return RimessoLascia, nil
	}
	return RimessoTutto, a.Sis.ScriviSPI(g.id, p.Extra["prima"] == "1")
}

func (g *spiGest) Valida(p Precedente, admin bool) (Precedente, bool) {
	if p.Vuoto {
		return Precedente{Vuoto: true}, len(p.Extra) == 0
	}
	x := p.Extra
	if len(p.Valori) > 0 || len(p.Messi) > 0 || len(p.Parti) > 0 || len(x) != 3 || x["spi"] != g.id || !SPINoto(x["spi"]) ||
		(x["prima"] != "0" && x["prima"] != "1") || x["messo"] != b01(g.vuole) {
		return Precedente{}, false
	}
	return Precedente{Extra: map[string]string{"spi": x["spi"], "prima": x["prima"], "messo": x["messo"]}}, true
}

// ---------------- Mouse ----------------

// mouseGest: accelerazione del mouse ("Precisione puntatore avanzata") spenta. Nel registro sono
// tre testi (MouseSpeed, MouseThreshold1, MouseThreshold2); poi Windows la applica subito.
type mouseGest struct{ reg regGest }

const chiaveMouse = `Control Panel\Mouse`

func nuovoMouse() *mouseGest {
	return &mouseGest{reg: regGest{bers: []bersaglio{
		{vuole: sz(HKCU, chiaveMouse, "MouseSpeed", "0")},
		{vuole: sz(HKCU, chiaveMouse, "MouseThreshold1", "0")},
		{vuole: sz(HKCU, chiaveMouse, "MouseThreshold2", "0")},
	}}}
}

func (g *mouseGest) Disponibile(a *Ambiente) (bool, string) { return true, "" }

func (g *mouseGest) APosto(a *Ambiente) (bool, error) {
	v, err := a.Reg.Leggi(HKCU, chiaveMouse, "MouseSpeed")
	if err != nil {
		return false, err
	}
	return v.Esiste && strings.TrimSpace(v.Testo) == "0", nil
}

func (g *mouseGest) Prepara(a *Ambiente) (Precedente, error) { return g.reg.Prepara(a) }

// numeroMouse: i tre testi del mouse sono numeri piccoli; un valore strano vale come quello di Windows.
func numeroMouse(v Valore, base int) int {
	if !v.Esiste {
		return base
	}
	n, err := strconv.Atoi(strings.TrimSpace(v.Testo))
	if err != nil || n < 0 || n > 99 {
		return base
	}
	return n
}

func (g *mouseGest) applicaSubito(a *Ambiente) {
	r := func(n string, base int) int {
		v, _ := a.Reg.Leggi(HKCU, chiaveMouse, n)
		return numeroMouse(v, base)
	}
	_ = a.Sis.ImpostaMouse(r("MouseThreshold1", 6), r("MouseThreshold2", 10), r("MouseSpeed", 1))
}

func (g *mouseGest) Applica(a *Ambiente, p Precedente) error {
	if err := g.reg.Applica(a, p); err != nil {
		return err
	}
	g.applicaSubito(a)
	return nil
}

func (g *mouseGest) Verifica(a *Ambiente, p Precedente) bool { return g.reg.Verifica(a, p) }

func (g *mouseGest) Rimetti(a *Ambiente, p Precedente) (Rimesso, error) {
	e, scritto, err := rimettiValori(a.Reg, p)
	if scritto {
		g.applicaSubito(a)
	}
	return e, err
}

func (g *mouseGest) Valida(p Precedente, admin bool) (Precedente, bool) {
	out, ok := g.reg.Valida(p, admin)
	if !ok {
		return out, false
	}
	for _, v := range out.Valori {
		if v.Esiste && (v.Tipo != TipoSz || len(v.Testo) > 4) {
			return Precedente{}, false
		}
	}
	return out, true
}

// ---------------- Piano energetico ----------------

// GUID dei piani e delle impostazioni di Windows usati qui (uguali in tutte le lingue).
const (
	PianoBilanciato    = "381b4222-f694-41f0-9685-ff5bb260df2e"
	PianoPrestazioni   = "8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c"
	PianoMassime       = "e9a42b02-d5df-448d-aa00-03f14749eb61" // Prestazioni ottimali (Ultimate)
	PianoRyzenAlte     = "9897998c-92de-4669-853f-b7cd3ecb2790" // AMD Ryzen High Performance
	OverlayMigliori    = "ded574b5-45a0-4f42-8737-46345c09c238" // modalità "Prestazioni migliori"
	OverlayNessuno     = "00000000-0000-0000-0000-000000000000"
	GruppoUSB          = "2a737441-1930-4402-8d77-b2bebba308a3"
	ImpUSBSospensione  = "48e6b7a6-50f5-4782-a5d4-53bb8f07e226"
	GruppoRisparmio    = "de830923-a562-41af-a086-e3a2c6bad2da"
	ImpRisparmioSoglia = "e69653ca-cf7f-4f05-aa73-cb833fa90ad4"
	GruppoSchermo      = "7516b95f-f776-4464-8c53-06167f40cc99"
	ImpSchermoSpento   = "3c0bc021-c8a8-4e07-a973-6b14cbcb2b7e"
)

var reGUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// GUIDValido: un GUID scritto come lo scrive Tasso (minuscolo).
func GUIDValido(s string) bool { return reGUID.MatchString(s) }

func pianoGiaAlto(g string) bool {
	return g == PianoPrestazioni || g == PianoMassime || g == PianoRyzenAlte
}

// pianoGest: piano "Prestazioni elevate" (solo sui PC fissi). Se il PC non ce l'ha (molti PC nuovi
// hanno solo Bilanciato), la modalità "Prestazioni migliori" di Windows.
type pianoGest struct{}

const msgPianoPortatile = "Sui portatili Tasso non cambia il piano energetico: a batteria si scaricherebbe in fretta. Quando giochi tieni attaccato il caricatore."

func (g *pianoGest) Disponibile(a *Ambiente) (bool, string) {
	if a.PC.Portatile {
		return false, msgPianoPortatile
	}
	return true, ""
}

func (g *pianoGest) APosto(a *Ambiente) (bool, error) {
	att, err := a.En.PianoAttivo()
	if err != nil {
		return false, err
	}
	if pianoGiaAlto(att) {
		return true, nil
	}
	if !a.En.PianoEsiste(PianoPrestazioni) && att == PianoBilanciato {
		ov, err := a.En.Overlay()
		return err == nil && ov == OverlayMigliori, err
	}
	return false, nil
}

func (g *pianoGest) Prepara(a *Ambiente) (Precedente, error) {
	att, err := a.En.PianoAttivo()
	if err != nil {
		return Precedente{}, err
	}
	if a.En.PianoEsiste(PianoPrestazioni) {
		return Precedente{Extra: map[string]string{"modo": "piano", "prima": att, "messo": PianoPrestazioni}}, nil
	}
	if att != PianoBilanciato {
		// piano personale senza "Prestazioni elevate": la modalità vale solo con Bilanciato, non lo cambio
		return Precedente{}, ErrNonDisponibile
	}
	ov, err := a.En.Overlay()
	if err != nil {
		return Precedente{}, err
	}
	if ov == "" {
		ov = OverlayNessuno
	}
	return Precedente{Extra: map[string]string{"modo": "overlay", "prima": ov, "messo": OverlayMigliori}}, nil
}

func (g *pianoGest) Applica(a *Ambiente, p Precedente) error {
	if p.Extra["modo"] == "overlay" {
		return a.En.ImpostaOverlay(p.Extra["messo"])
	}
	return a.En.AttivaPiano(p.Extra["messo"])
}

func (g *pianoGest) adesso(a *Ambiente, modo string) (string, error) {
	if modo == "overlay" {
		v, err := a.En.Overlay()
		if v == "" {
			v = OverlayNessuno
		}
		return v, err
	}
	return a.En.PianoAttivo()
}

func (g *pianoGest) Verifica(a *Ambiente, p Precedente) bool {
	v, err := g.adesso(a, p.Extra["modo"])
	return err == nil && v == p.Extra["messo"]
}

func (g *pianoGest) Rimetti(a *Ambiente, p Precedente) (Rimesso, error) {
	v, err := g.adesso(a, p.Extra["modo"])
	if err != nil {
		return RimessoTutto, err
	}
	if v != p.Extra["messo"] {
		return RimessoLascia, nil
	}
	if p.Extra["modo"] == "overlay" {
		return RimessoTutto, a.En.ImpostaOverlay(p.Extra["prima"])
	}
	if !a.En.PianoEsiste(p.Extra["prima"]) {
		// il piano di prima non c'è più (tolto nel frattempo): torno a Bilanciato
		return RimessoTutto, a.En.AttivaPiano(PianoBilanciato)
	}
	return RimessoTutto, a.En.AttivaPiano(p.Extra["prima"])
}

func (g *pianoGest) Valida(p Precedente, admin bool) (Precedente, bool) {
	x := p.Extra
	if len(p.Valori) > 0 || len(p.Messi) > 0 || len(p.Parti) > 0 || p.Vuoto || len(x) != 3 || !GUIDValido(x["prima"]) {
		return Precedente{}, false
	}
	switch {
	case x["modo"] == "piano" && x["messo"] == PianoPrestazioni:
	case x["modo"] == "overlay" && x["messo"] == OverlayMigliori:
	default:
		return Precedente{}, false
	}
	return Precedente{Extra: map[string]string{"modo": x["modo"], "prima": x["prima"], "messo": x["messo"]}}, true
}

// ---------------- Un'impostazione di un piano energetico ----------------

// enGest: un'impostazione del piano attivo, a corrente (dc=false) o a batteria (dc=true).
type enGest struct {
	gruppo, imp string
	dc          bool
	ok          func(v uint32) bool
	vuole       uint32
	cond        func(a *Ambiente) (bool, string)
}

func (g *enGest) Disponibile(a *Ambiente) (bool, string) {
	if g.cond != nil {
		return g.cond(a)
	}
	return true, ""
}

func (g *enGest) leggi(a *Ambiente, piano string) (uint32, error) {
	ac, dc, err := a.En.Leggi(piano, g.gruppo, g.imp)
	if g.dc {
		return dc, err
	}
	return ac, err
}

func (g *enGest) scrivi(a *Ambiente, piano string, v uint32) error {
	if g.dc {
		return a.En.ScriviDC(piano, g.gruppo, g.imp, v)
	}
	return a.En.ScriviAC(piano, g.gruppo, g.imp, v)
}

func (g *enGest) APosto(a *Ambiente) (bool, error) {
	att, err := a.En.PianoAttivo()
	if err != nil {
		return false, err
	}
	v, err := g.leggi(a, att)
	return err == nil && g.ok(v), err
}

func (g *enGest) Prepara(a *Ambiente) (Precedente, error) {
	att, err := a.En.PianoAttivo()
	if err != nil {
		return Precedente{}, err
	}
	v, err := g.leggi(a, att)
	if err != nil {
		return Precedente{}, err
	}
	return Precedente{Extra: map[string]string{"piano": att, "prima": strconv.FormatUint(uint64(v), 10), "messo": strconv.FormatUint(uint64(g.vuole), 10)}}, nil
}

func numExtra(p Precedente, k string) (uint32, bool) {
	n, err := strconv.ParseUint(p.Extra[k], 10, 32)
	return uint32(n), err == nil
}

func (g *enGest) Applica(a *Ambiente, p Precedente) error {
	m, ok := numExtra(p, "messo")
	if !ok {
		return errors.New("valore non valido")
	}
	return g.scrivi(a, p.Extra["piano"], m)
}

func (g *enGest) Verifica(a *Ambiente, p Precedente) bool {
	m, ok := numExtra(p, "messo")
	v, err := g.leggi(a, p.Extra["piano"])
	return ok && err == nil && v == m
}

func (g *enGest) Rimetti(a *Ambiente, p Precedente) (Rimesso, error) {
	m, ok1 := numExtra(p, "messo")
	pr, ok2 := numExtra(p, "prima")
	if !ok1 || !ok2 {
		return RimessoTutto, errors.New("valore non valido")
	}
	if !a.En.PianoEsiste(p.Extra["piano"]) {
		return RimessoLascia, nil // il piano non c'è più: niente da rimettere
	}
	v, err := g.leggi(a, p.Extra["piano"])
	if err != nil {
		return RimessoTutto, err
	}
	if v != m {
		return RimessoLascia, nil
	}
	return RimessoTutto, g.scrivi(a, p.Extra["piano"], pr)
}

func (g *enGest) Valida(p Precedente, admin bool) (Precedente, bool) {
	x := p.Extra
	if len(p.Valori) > 0 || len(p.Messi) > 0 || len(p.Parti) > 0 || p.Vuoto || len(x) != 3 || !GUIDValido(x["piano"]) {
		return Precedente{}, false
	}
	m, ok1 := numExtra(p, "messo")
	_, ok2 := numExtra(p, "prima")
	if !ok1 || !ok2 || m != g.vuole {
		return Precedente{}, false
	}
	return Precedente{Extra: map[string]string{"piano": x["piano"], "prima": x["prima"], "messo": x["messo"]}}, true
}

// ---------------- Scheda video per i giochi ----------------

// ChiaveGPU: le scelte di Windows "Impostazioni grafiche" (una stringa per programma).
const ChiaveGPU = `Software\Microsoft\DirectX\UserGpuPreferences`

// GPUScelta: nella stringa c'è già una scelta (1 risparmio energetico, 2 prestazioni elevate).
// Se l'hai messa tu, Tasso non la cambia.
func GPUScelta(v string) bool {
	for _, p := range strings.Split(v, ";") {
		if k, val, ok := strings.Cut(strings.TrimSpace(p), "="); ok && strings.TrimSpace(k) == "GpuPreference" {
			val = strings.TrimSpace(val)
			return val == "1" || val == "2"
		}
	}
	return false
}

// GPUConPotente: la stringa con GpuPreference=2 al posto della scelta di prima; le altre scelte di
// Windows per quel gioco (HDR automatico, frequenza variabile...) restano.
func GPUConPotente(old string) string {
	parts := []string{"GpuPreference=2"}
	for _, p := range strings.Split(old, ";") {
		p = strings.TrimSpace(p)
		k, _, ok := strings.Cut(p, "=")
		if p == "" || !ok || strings.TrimSpace(k) == "GpuPreference" {
			continue
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, ";") + ";"
}

// exeGioco: un percorso completo di un programma .exe (C:\...\gioco.exe o \\server\...).
func exeGioco(p string) bool {
	l := strings.ToLower(p)
	return testoSicuro(p, 520) && strings.HasSuffix(l, ".exe") && (len(p) > 3 && p[1] == ':' && p[2] == '\\' || strings.HasPrefix(p, `\\`)) && !strings.Contains(p, `\..\`)
}

type gpuGest struct{}

func (g *gpuGest) Disponibile(a *Ambiente) (bool, string) {
	if a.PC.SchedeVere < 2 {
		return false, "Su questo PC c'è una sola scheda video: i giochi usano già quella."
	}
	if len(a.PC.Giochi) == 0 {
		return false, "Tasso non ha trovato giochi su questo PC: quando ne installi uno, rifai Ottimizza tutto."
	}
	return true, ""
}

// daCambiare: i giochi senza nessuna scelta della scheda video.
func (g *gpuGest) daCambiare(a *Ambiente) ([]Valore, error) {
	var out []Valore
	visti := map[string]bool{}
	for _, exe := range a.PC.Giochi {
		if !exeGioco(exe) || visti[strings.ToLower(exe)] {
			continue
		}
		visti[strings.ToLower(exe)] = true
		v, err := a.Reg.Leggi(HKCU, ChiaveGPU, exe)
		if err != nil {
			return nil, err
		}
		if v.Esiste && v.Tipo == TipoSz && GPUScelta(v.Testo) {
			continue // scelta già fatta (da te o da Tasso): non la tocco
		}
		v.Radice, v.Chiave, v.Nome = HKCU, ChiaveGPU, exe
		out = append(out, v)
	}
	return out, nil
}

func (g *gpuGest) APosto(a *Ambiente) (bool, error) {
	l, err := g.daCambiare(a)
	return err == nil && len(l) == 0, err
}

func (g *gpuGest) Prepara(a *Ambiente) (Precedente, error) {
	l, err := g.daCambiare(a)
	if err != nil {
		return Precedente{}, err
	}
	var p Precedente
	for _, v := range l {
		old := ""
		if v.Esiste && v.Tipo == TipoSz {
			old = v.Testo
		}
		p.Valori = append(p.Valori, v)
		p.Messi = append(p.Messi, sz(HKCU, ChiaveGPU, v.Nome, GPUConPotente(old)))
	}
	p.Vuoto = len(p.Messi) == 0
	return p, nil
}

func (g *gpuGest) Applica(a *Ambiente, p Precedente) error {
	for _, m := range p.Messi {
		if err := a.Reg.Scrivi(m); err != nil {
			return err
		}
	}
	return nil
}

func (g *gpuGest) Verifica(a *Ambiente, p Precedente) bool { return (&regGest{}).Verifica(a, p) }

func (g *gpuGest) Rimetti(a *Ambiente, p Precedente) (Rimesso, error) {
	e, _, err := rimettiValori(a.Reg, p)
	return e, err
}

func (g *gpuGest) Valida(p Precedente, admin bool) (Precedente, bool) {
	if len(p.Valori) != len(p.Messi) || len(p.Parti) > 0 || len(p.Extra) > 0 || len(p.Messi) > 500 {
		return Precedente{}, false
	}
	for i, m := range p.Messi {
		v := p.Valori[i]
		if m.Radice != HKCU || m.Chiave != ChiaveGPU || !exeGioco(m.Nome) || m.Tipo != TipoSz || !m.Esiste ||
			!strings.HasPrefix(m.Testo, "GpuPreference=2;") || !testoSicuro(m.Testo, 260) || !v.Stesso(m) || !valoreSicuro(v) {
			return Precedente{}, false
		}
	}
	return Precedente{Valori: p.Valori, Messi: p.Messi, Vuoto: len(p.Messi) == 0}, true
}

// ---------------- Preferenza di Tasso: notifiche in pausa durante il gioco ----------------

type notifGest struct{}

func (g *notifGest) Disponibile(a *Ambiente) (bool, string) { return a.Pref != nil, "" }
func (g *notifGest) APosto(a *Ambiente) (bool, error)       { return a.Pref.NotificheGioco(), nil }
func (g *notifGest) Prepara(a *Ambiente) (Precedente, error) {
	return Precedente{Extra: map[string]string{"pref": "notifiche-gioco"}}, nil
}
func (g *notifGest) Applica(a *Ambiente, p Precedente) error {
	return a.Pref.ImpostaNotificheGioco(true)
}
func (g *notifGest) Verifica(a *Ambiente, p Precedente) bool { return a.Pref.NotificheGioco() }
func (g *notifGest) Rimetti(a *Ambiente, p Precedente) (Rimesso, error) {
	if !a.Pref.NotificheGioco() {
		return RimessoLascia, nil
	}
	return RimessoTutto, a.Pref.ImpostaNotificheGioco(false)
}
func (g *notifGest) Valida(p Precedente, admin bool) (Precedente, bool) {
	if len(p.Extra) != 1 || p.Extra["pref"] != "notifiche-gioco" || len(p.Valori) > 0 || len(p.Messi) > 0 || len(p.Parti) > 0 {
		return Precedente{}, false
	}
	return Precedente{Extra: map[string]string{"pref": "notifiche-gioco"}}, true
}

// ---------------- Pulizia dei file inutili (non si rimette) ----------------

type puliziaGest struct{ passi []string }

func (g *puliziaGest) Disponibile(a *Ambiente) (bool, string) {
	if a.Pul == nil {
		return false, "La pulizia non è disponibile adesso."
	}
	return true, ""
}
func (g *puliziaGest) APosto(a *Ambiente) (bool, error) { return false, nil }
func (g *puliziaGest) Prepara(a *Ambiente) (Precedente, error) {
	return Precedente{Extra: map[string]string{"pulizia": "1"}}, nil
}
func (g *puliziaGest) Applica(a *Ambiente, p Precedente) error {
	var primoErr error
	for _, id := range g.passi {
		if _, err := a.Pul.Pulisci(id); err != nil && primoErr == nil {
			primoErr = err
		}
	}
	return primoErr
}
func (g *puliziaGest) Verifica(a *Ambiente, p Precedente) bool { return true }
func (g *puliziaGest) Rimetti(a *Ambiente, p Precedente) (Rimesso, error) {
	return RimessoLascia, nil // i file tolti non tornano: lo dice il riepilogo
}
func (g *puliziaGest) Valida(p Precedente, admin bool) (Precedente, bool) {
	return Precedente{Extra: map[string]string{"pulizia": "1"}}, true
}
