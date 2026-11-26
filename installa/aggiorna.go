package installa

// Aggiornamento silenzioso ("TassoSetup.exe --aggiorna-silenzioso", lo avvia Tasso dalle sue
// Impostazioni → Aggiornamenti): si aggiornano SOLO i programmi già installati e solo se questo
// installatore ne ha una versione più nuova. Stessa cartella, stesse caselle, stessa lingua,
// collegamenti sul Desktop solo quelli che c'erano già. Mai tornare a una versione più vecchia.

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrNienteDaAggiornare: niente di installato, o questo installatore non ha niente di più nuovo.
var ErrNienteDaAggiornare = errors.New("niente da aggiornare")

// PiuNuova dice se la versione a è più nuova di b ("3.10" > "3.9", "1.2" = "1.2.0").
// Una versione vuota o scritta male conta come la più vecchia possibile, così un numero strano
// in tasso-setup.json fa aggiornare, mentre uno strano dentro l'installatore non fa mai niente.
func PiuNuova(a, b string) bool {
	va, oka := numeriVersione(a)
	vb, okb := numeriVersione(b)
	if !oka {
		return false
	}
	if !okb {
		return true
	}
	for i := 0; i < len(va) || i < len(vb); i++ {
		x, y := 0, 0
		if i < len(va) {
			x = va[i]
		}
		if i < len(vb) {
			y = vb[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

func numeriVersione(s string) ([]int, bool) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 32 {
		return nil, false
	}
	var out []int
	for _, p := range strings.Split(s, ".") {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p == "" || strings.ContainsAny(p, "+-") {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// Maggiore: la più nuova delle due versioni.
func Maggiore(a, b string) string {
	if PiuNuova(b, a) {
		return b
	}
	return a
}

// VersioneDaAggiornare: per ogni programma installato, la versione installata e quella di
// questo installatore.
type VersioneDaAggiornare struct {
	Programma  Programma
	Installata string
	Nuova      string
}

// PianoAggiornamento: cosa fa l'aggiornamento silenzioso.
type PianoAggiornamento struct {
	Opzioni   Opzioni                // da passare a Installa
	Programmi []VersioneDaAggiornare // solo quelli che diventano più nuovi
}

// PreparaAggiornamento legge cosa c'è installato (st, da tasso-setup.json) e decide cosa
// aggiornare. versioneTasso è il file VERSIONE accanto a Tasso.exe (Tasso, aggiornandosi da solo,
// lo riscrive: vale la più nuova tra quella e tasso-setup.json). menu e desktop sono le cartelle
// dei collegamenti (menu Start di tutti, Desktop pubblico).
// Restituisce ErrNienteDaAggiornare se non c'è niente di installato o niente di più nuovo.
func PreparaAggiornamento(st *Stato, e *Elenco, versioneTasso, menu, desktop string) (*PianoAggiornamento, error) {
	if st == nil || e == nil || len(st.Componenti) == 0 {
		return nil, ErrNienteDaAggiornare
	}
	piano := &PianoAggiornamento{}
	var scelti []Componente
	visti := map[Programma]bool{}
	for _, c := range st.Componenti {
		id := ProgrammaDi(c)
		p := e.Prog(id)
		if p == nil {
			continue // non è dentro questo installatore: resta com'è
		}
		inst := st.Programmi[id].Versione
		if id == ProgTasso {
			inst = Maggiore(inst, versioneTasso)
		}
		if !PiuNuova(p.Versione, inst) {
			continue // uguale o più vecchia: mai indietro
		}
		scelti = append(scelti, c) // Gaming e Ufficio insieme: stesso Tasso.exe, stessi collegamenti
		if !visti[id] {
			visti[id] = true
			piano.Programmi = append(piano.Programmi, VersioneDaAggiornare{Programma: id, Installata: inst, Nuova: p.Versione})
		}
	}
	if len(scelti) == 0 {
		return nil, ErrNienteDaAggiornare
	}
	scelti, _ = normalizza(scelti)
	piano.Opzioni = Opzioni{
		Base:            st.Cartella,
		Componenti:      scelti,
		Desktop:         false, // il Desktop resta com'è: solo i collegamenti che ci sono già
		Lingua:          st.Lingua,
		CartellaMenu:    menu,
		CartellaDesktop: desktop,
		// il disinstallatore di una versione dell'ecosistema più nuova non si sostituisce con uno più vecchio
		TieniDisinstallatore: PiuNuova(st.Versione, e.Versione),
	}
	return piano, nil
}

// DesktopUsato: c'è ancora almeno un collegamento sul Desktop di un programma installato (per
// la casella "Metti i collegamenti anche sul Desktop" quando si aggiorna: parte spuntata).
func DesktopUsato(st *Stato, desktop string) bool {
	return len(DesktopEsistenti(st, desktop)) > 0
}

// DesktopEsistenti: i programmi che hanno ancora un collegamento sul Desktop (se l'hai tolto tu,
// non conta). Vale sia quello scritto in tasso-setup.json sia un .lnk col nome dei nostri.
func DesktopEsistenti(st *Stato, desktop string) map[Programma]bool {
	out := map[Programma]bool{}
	if st == nil || desktop == "" {
		return out
	}
	d := filepath.Clean(desktop)
	file := func(p string) bool {
		fi, err := os.Lstat(p)
		return err == nil && fi.Mode().IsRegular()
	}
	for _, c := range st.Collegamenti {
		if c.Programma == "" || !strings.EqualFold(filepath.Dir(filepath.Clean(c.Percorso)), d) {
			continue
		}
		if file(c.Percorso) {
			out[c.Programma] = true
		}
	}
	for id := range st.Programmi {
		nomi := []string{NomiCollegamento[id]}
		if id == ProgTasso {
			nomi = append(nomi, "Tasso Ufficio.lnk")
		}
		for _, n := range nomi {
			if n != "" && file(filepath.Join(d, n)) {
				out[id] = true
			}
		}
	}
	return out
}

// Codici d'uscita di "TassoSetup.exe --aggiorna-silenzioso".
const (
	UscitaFatto        = 0 // aggiornato (o, con --solo-controlla: c'è qualcosa da aggiornare)
	UscitaErrore       = 1 // non riuscito: sul PC non è cambiato niente
	UscitaNienteDaFare = 2 // niente di installato, o versioni uguali o più vecchie
)

// SenzaPiuNuovi toglie dai componenti scelti quelli il cui programma è già installato in una
// versione più nuova di quella di questo installatore (mai tornare indietro, anche con
// l'installatore normale). versioneTasso: il file VERSIONE accanto a Tasso.exe.
func SenzaPiuNuovi(st *Stato, e *Elenco, scelti []Componente, versioneTasso string) []Componente {
	if st == nil || e == nil {
		return scelti
	}
	var out []Componente
	for _, c := range scelti {
		id := ProgrammaDi(c)
		inst := st.Programmi[id].Versione
		if id == ProgTasso {
			inst = Maggiore(inst, versioneTasso)
		}
		if p := e.Prog(id); p != nil && inst != "" && PiuNuova(inst, p.Versione) {
			Diario("%s: installata %s, più nuova di %s: resta com'è", id, inst, p.Versione)
			continue
		}
		out = append(out, c)
	}
	return out
}
