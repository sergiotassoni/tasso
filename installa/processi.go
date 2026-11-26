package installa

// Quali processi chiudere prima di sostituire i file (la parte che si può provare anche su Linux;
// la fotografia dei processi e la chiusura vera sono in sistema_windows.go).

import "strings"

// Processo: una riga della fotografia dei processi di Windows.
type Processo struct {
	PID, Padre uint32
	Nome       string // tasso.exe (minuscolo o no, non importa)
	Percorso   string // percorso completo dell'exe ("" se non si legge)
	Nato       int64  // ora di avvio (per non confondere un PID riusato da un altro programma)
}

// AiutantiTasso: i programmi che Tasso avvia per la traduzione e il gaming. (Tasso Traduce da
// solo, tassotraduce.exe, non è un figlio di Tasso e non tiene file dell'installazione: resta.)
var AiutantiTasso = map[string]bool{
	"llama-server.exe": true, "whisper-server.exe": true, "presentmon.exe": true,
}

// SottocartelleTasso: le cartelle di lavoro che Tasso crea dentro la sua cartella. Sono solo sue:
// ogni processo partito da qui è di Tasso (llama-server, PresentMon, driver...).
var SottocartelleTasso = []string{"lavoro", "strumenti", "driver"}

// CriteriChiusura: cosa si può chiudere. Mai "tutto quello che parte dalla cartella": la cartella
// scelta potrebbe contenere altri programmi (installazioni vecchie in D:\Giochi o C:\Program Files).
type CriteriChiusura struct {
	Radici []uint32 // i programmi di Tasso che si stanno chiudendo (Tasso, Zip, PDF): i loro figli
	Io     uint32   // questo installatore: mai lui, i suoi antenati e i suoi figli
	// File: gli exe (e gli altri file) dei programmi da sostituire, percorsi completi: elenco
	// installato (tasso-setup.json) ed elenco nuovo. Un processo partito da uno di questi si chiude.
	File []string
	// Cartelle: le cartelle che sono solo di Tasso (SottocartelleTasso dentro la cartella di Tasso).
	// Ogni processo partito da qui si chiude.
	Cartelle []string
	// Escluse: sottocartelle da non toccare (es. Tasso Zip se aggiorno solo Tasso).
	Escluse []string
	// CartelleAiutanti: %LOCALAPPDATA%\TassoTraduce. Da qui si chiudono solo gli AiutantiTasso
	// e i figli dei programmi che si chiudono.
	CartelleAiutanti []string
}

// normPercorso: minuscolo, con \ e senza \ in fondo (i percorsi di Windows non distinguono maiuscole).
func normPercorso(p string) string {
	p = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(p), "/", `\`))
	return strings.TrimRight(p, `\`)
}

// Dentro: il file p è dentro la cartella dir (o è dir).
func Dentro(p, dir string) bool {
	p, dir = normPercorso(p), normPercorso(dir)
	if p == "" || dir == "" {
		return false
	}
	return p == dir || strings.HasPrefix(p, dir+`\`)
}

func dentroUna(p string, dd []string) bool {
	for _, d := range dd {
		if Dentro(p, d) {
			return true
		}
	}
	return false
}

// DaChiudere sceglie dalla fotografia i processi da chiudere:
//   - i discendenti dei programmi che si chiudono (Radici) partiti dalle cartelle di Tasso o da
//     quella degli aiutanti, e i loro msedgewebview2.exe (la finestra di Tasso);
//   - i processi partiti da un file dei programmi da sostituire o dalle cartelle solo di Tasso
//     (lavoro, strumenti, driver: es. un llama-server rimasto aperto);
//   - gli aiutanti di Tasso (llama-server, whisper-server, PresentMon) partiti dalla cartella degli aiutanti.
//
// Mai le Radici stesse (si chiudono a parte), mai questo installatore, i suoi antenati o i suoi
// figli, mai un msedgewebview2.exe che non è figlio di Tasso.
func DaChiudere(lista []Processo, c CriteriChiusura) []Processo {
	per := map[uint32]Processo{}
	figli := map[uint32][]uint32{}
	for _, p := range lista {
		per[p.PID] = p
	}
	for _, p := range lista {
		if p.Padre != p.PID {
			figli[p.Padre] = append(figli[p.Padre], p.PID)
		}
	}
	// figlio vero: nato dopo il padre (altrimenti il PID del padre è stato riusato)
	discendenti := func(radici []uint32) map[uint32]bool {
		visti := map[uint32]bool{}
		coda := append([]uint32(nil), radici...)
		for len(coda) > 0 {
			pid := coda[0]
			coda = coda[1:]
			padre, ok := per[pid]
			for _, f := range figli[pid] {
				if visti[f] {
					continue
				}
				if ok && padre.Nato != 0 && per[f].Nato != 0 && per[f].Nato < padre.Nato {
					continue
				}
				visti[f] = true
				coda = append(coda, f)
			}
		}
		return visti
	}
	protetti := map[uint32]bool{0: true, 4: true}
	if c.Io != 0 {
		protetti[c.Io] = true
		for pid := range discendenti([]uint32{c.Io}) {
			protetti[pid] = true
		}
		// gli antenati (Tasso che mi ha avviato si chiude da solo, come radice)
		for p, ok := per[c.Io]; ok && !protetti[p.Padre]; p, ok = per[p.Padre] {
			protetti[p.Padre] = true
		}
	}
	for _, r := range c.Radici {
		protetti[r] = true
	}
	daRadici := discendenti(c.Radici)

	file := map[string]bool{}
	for _, f := range c.File {
		if n := normPercorso(f); n != "" {
			file[n] = true
		}
	}
	var out []Processo
	for _, p := range lista {
		if protetti[p.PID] {
			continue
		}
		nome := strings.ToLower(p.Nome)
		escluso := dentroUna(p.Percorso, c.Escluse)
		nelleCartelle := !escluso && (file[normPercorso(p.Percorso)] || dentroUna(p.Percorso, c.Cartelle))
		negliAiutanti := dentroUna(p.Percorso, c.CartelleAiutanti)
		var si bool
		switch {
		case nelleCartelle:
			si = true
		case negliAiutanti && (daRadici[p.PID] || AiutantiTasso[nome]):
			si = true
		case daRadici[p.PID] && nome == "msedgewebview2.exe":
			si = true
		}
		if si {
			out = append(out, p)
		}
	}
	return out
}

// CriteriPer prepara i CriteriChiusura per sostituire (o togliere) i componenti cc nella
// cartella base: i file dell'elenco installato (st, può essere nil) e di quello nuovo (e, nil
// nel disinstallatore), le cartelle solo di Tasso e quella degli aiutanti (aiutanti, "" = nessuna).
func CriteriPer(base string, cc []Componente, st *Stato, e *Elenco, radici []uint32, io uint32, aiutanti string) CriteriChiusura {
	c := CriteriChiusura{Radici: radici, Io: io}
	scelti := map[Programma]bool{}
	progs := ProgrammiPer(cc)
	if len(progs) > 0 {
		progs = append(progs, ProgSetup) // il disinstallatore si sostituisce (o si toglie) con loro
	}
	visti := map[string]bool{}
	aggiungi := func(dir string, ff []FileElenco) {
		for _, f := range ff {
			if p, ok := percorsoFile(dir, f.Nome); ok && !visti[strings.ToLower(p)] {
				visti[strings.ToLower(p)] = true
				c.File = append(c.File, p)
			}
		}
	}
	for _, p := range progs {
		scelti[p] = true
		dir := CartellaDi(base, p)
		if st != nil {
			aggiungi(dir, st.Programmi[p].File)
		}
		if e != nil {
			if ep := e.Prog(p); ep != nil {
				aggiungi(dir, ep.File)
			}
		}
	}
	if scelti[ProgTasso] {
		for _, d := range SottocartelleTasso {
			c.Cartelle = append(c.Cartelle, base+`\`+d)
		}
		for _, p := range []Programma{ProgZip, ProgPDF} {
			if !scelti[p] {
				c.Escluse = append(c.Escluse, CartellaDi(base, p))
			}
		}
		if aiutanti != "" {
			c.CartelleAiutanti = append(c.CartelleAiutanti, aiutanti)
		}
	}
	return c
}
