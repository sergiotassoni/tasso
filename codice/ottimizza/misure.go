package ottimizza

import (
	"encoding/xml"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---------------- FPS della scena di prova ----------------

// MinFotogrammi: sotto questo numero la misura non vale (scena partita male o PC fermo).
const MinFotogrammi = 100

// StatFotogrammi: FPS medi (fotogrammi diviso tempo) e FPS più bassi (1%): il centesimo di
// fotogrammi più lenti, come media, trasformato in FPS (come il "1% low" dei programmi di prova).
func StatFotogrammi(d []time.Duration) (medi, bassi float64, ok bool) {
	if len(d) < MinFotogrammi {
		return 0, 0, false
	}
	var tot time.Duration
	l := make([]time.Duration, 0, len(d))
	for _, x := range d {
		if x <= 0 {
			continue
		}
		tot += x
		l = append(l, x)
	}
	if len(l) < MinFotogrammi || tot <= 0 {
		return 0, 0, false
	}
	medi = float64(len(l)) / tot.Seconds()
	sort.Slice(l, func(i, j int) bool { return l[i] > l[j] })
	n := len(l) / 100
	if n < 1 {
		n = 1
	}
	var lenti time.Duration
	for _, x := range l[:n] {
		lenti += x
	}
	bassi = float64(n) / lenti.Seconds()
	return medi, bassi, true
}

// ---------------- Prima e dopo ----------------

// Riga: una riga della tabella "prima → dopo".
type Riga struct {
	Chiave   string  `json:"chiave"` // fps, fps1, voto, accensione, apertura
	Prima    float64 `json:"prima"`
	Dopo     float64 `json:"dopo"`
	HaDopo   bool    `json:"haDopo"`
	Unita    string  `json:"unita"` // "FPS", "/100", "s"
	Decimali int     `json:"decimali"`
	Meglio   bool    `json:"meglio"` // il dopo è migliore in modo che si nota
}

// AperturaTotale: i secondi per aprire i programmi misurati sia prima sia dopo (un programma che
// non si è aperto una delle due volte non conta in nessuna delle due).
func AperturaTotale(prima, dopo map[string]int) (p, d float64, ok bool) {
	chiavi := []string{"esplora", "blocconote", "browser"}
	n := 0
	for _, k := range chiavi {
		a, b := prima[k], dopo[k]
		if dopo == nil {
			b = 1
		}
		if a > 0 && b > 0 {
			p += float64(a) / 1000
			if dopo != nil {
				d += float64(dopo[k]) / 1000
			}
			n++
		}
	}
	return p, d, n > 0
}

func arrot(x float64, dec int) float64 {
	m := math.Pow(10, float64(dec))
	return math.Round(x*m) / m
}

// Confronta: le righe della tabella e se il miglioramento è piccolo o nullo (allora la finestra dice
// onestamente "Il tuo PC era già messo bene"). Le soglie tengono conto che due misure dello stesso
// PC cambiano sempre un po'.
func Confronta(r Risultato) (righe []Riga, piccolo bool) {
	d := Misura{}
	ha := r.Dopo != nil
	if ha {
		d = *r.Dopo
	}
	p := r.Prima
	migliorato, confrontabili := false, 0
	aggiungi := func(rg Riga, sogliaPct float64, menoMeglio bool) {
		rg.Prima, rg.Dopo = arrot(rg.Prima, rg.Decimali), arrot(rg.Dopo, rg.Decimali)
		if rg.HaDopo && rg.Prima > 0 {
			confrontabili++
			diff := (rg.Dopo - rg.Prima) / rg.Prima * 100
			if menoMeglio {
				diff = -diff
			}
			rg.Meglio = diff >= sogliaPct
			if rg.Meglio {
				migliorato = true
			}
		}
		righe = append(righe, rg)
	}
	if r.Modo == ModoUfficio {
		if p.Accensione > 0 {
			aggiungi(Riga{Chiave: "accensione", Prima: float64(p.Accensione) / 1000, Dopo: float64(d.Accensione) / 1000, HaDopo: ha && d.Accensione > 0, Unita: "s"}, 5, true)
		}
		var dm map[string]int
		if ha {
			dm = d.Apertura
		}
		if a, b, ok := AperturaTotale(p.Apertura, dm); ok {
			_, _, okDopo := AperturaTotale(p.Apertura, dm)
			aggiungi(Riga{Chiave: "apertura", Prima: a, Dopo: b, HaDopo: ha && okDopo && b > 0, Unita: "s", Decimali: 1}, 10, true)
		}
	} else {
		if p.FPSOk {
			aggiungi(Riga{Chiave: "fps", Prima: p.FPSMedi, Dopo: d.FPSMedi, HaDopo: ha && d.FPSOk, Unita: "FPS"}, 3, false)
			aggiungi(Riga{Chiave: "fps1", Prima: p.FPSBassi, Dopo: d.FPSBassi, HaDopo: ha && d.FPSOk, Unita: "FPS"}, 5, false)
		}
	}
	if p.Voto > 0 {
		rg := Riga{Chiave: "voto", Prima: float64(p.Voto), Dopo: float64(d.Voto), HaDopo: ha && d.Voto > 0, Unita: "/100"}
		if rg.HaDopo {
			confrontabili++
			rg.Meglio = d.Voto-p.Voto >= 2
			if rg.Meglio {
				migliorato = true
			}
		}
		righe = append(righe, rg)
	}
	return righe, confrontabili > 0 && !migliorato
}

// ---------------- Tempo di accensione (evento 100 di Windows) ----------------

// Accensione: un'accensione del PC letta dal registro eventi di Windows
// (Microsoft-Windows-Diagnostics-Performance/Operational, evento 100).
type Accensione struct {
	Ms     int       // BootTime: millisecondi dall'accensione a Windows pronto
	Inizio time.Time // quando è cominciata (BootStartTime, o l'ora dell'evento)
}

type eventoXML struct {
	System struct {
		EventID     string `xml:"EventID"`
		TimeCreated struct {
			SystemTime string `xml:"SystemTime,attr"`
		} `xml:"TimeCreated"`
	} `xml:"System"`
	Data []struct {
		Nome   string `xml:"Name,attr"`
		Valore string `xml:",chardata"`
	} `xml:"EventData>Data"`
}

var ErrEvento = errors.New("evento non valido")

func leggiOra(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, f := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.0000000Z", "2006-01-02T15:04:05Z"} {
		if t, err := time.Parse(f, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// LeggiEvento100: un evento 100 come lo dà Windows (XML). Non legge nessun testo nella lingua di
// Windows: solo numeri e date con il loro nome fisso.
func LeggiEvento100(s string) (Accensione, error) {
	var e eventoXML
	if err := xml.Unmarshal([]byte(s), &e); err != nil || strings.TrimSpace(e.System.EventID) != "100" {
		return Accensione{}, ErrEvento
	}
	var a Accensione
	for _, d := range e.Data {
		switch d.Nome {
		case "BootTime":
			n, err := strconv.Atoi(strings.TrimSpace(d.Valore))
			if err != nil {
				return Accensione{}, ErrEvento
			}
			a.Ms = n
		case "BootStartTime":
			if t, ok := leggiOra(d.Valore); ok && t.Year() > 2000 {
				a.Inizio = t
			}
		}
	}
	if a.Inizio.IsZero() {
		if t, ok := leggiOra(e.System.TimeCreated.SystemTime); ok {
			a.Inizio = t
		}
	}
	// meno di un secondo o più di un'ora: non è una misura vera
	if a.Ms < 1000 || a.Ms > 3600000 || a.Inizio.IsZero() {
		return Accensione{}, ErrEvento
	}
	return a, nil
}

// UltimaAccensione: la più recente tra quelle lette.
func UltimaAccensione(l []Accensione) (Accensione, bool) {
	var best Accensione
	for _, a := range l {
		if a.Inizio.After(best.Inizio) {
			best = a
		}
	}
	return best, !best.Inizio.IsZero()
}

// AccensioneDopo: la prima accensione cominciata dopo Ottimizza tutto (quella che dice se il PC
// parte più in fretta).
func AccensioneDopo(l []Accensione, fatto time.Time) (Accensione, bool) {
	var best Accensione
	for _, a := range l {
		if a.Inizio.After(fatto) && (best.Inizio.IsZero() || a.Inizio.Before(best.Inizio)) {
			best = a
		}
	}
	return best, !best.Inizio.IsZero()
}

// ---------------- Portatile o fisso, quando Windows non risponde ----------------

// DecidiPortatile: letto = Windows ha detto tipo di PC e telaio; regola = votopc.Portatile su quei
// dati; batteria = GetSystemPowerStatus.BatteryFlag (128 = nessuna batteria, 255 = non si sa).
// Se la lettura non è riuscita, nel dubbio il PC è un portatile (niente piano energetico al
// massimo): dubbio=true lo fa dire. Senza lettura ma con "nessuna batteria" è un fisso.
func DecidiPortatile(letto, regola bool, batteria uint8) (portatile, dubbio bool) {
	if letto {
		return regola, false
	}
	if batteria == 128 {
		return false, false
	}
	return true, true
}

// ---------------- Notifiche in pausa durante il gioco ----------------

// NotificheDaRimettere: cosa fare con i fumetti (ToastEnabled) alla fine della pausa. prima = il
// valore salvato prima della pausa ("none" = non c'era, "1" acceso); adesso = il valore di adesso
// (ok=false se non c'è). Si tocca solo se sono ancora spenti come li ha messi Tasso; un file
// strano li riaccende (acceso è il valore di Windows): mai spenti per sempre.
// Risultato: "lascia", "cancella" (torna il valore di Windows) o "accendi".
func NotificheDaRimettere(prima string, adesso uint32, ok bool) string {
	if !ok || adesso != 0 {
		return "lascia"
	}
	if prima == "none" {
		return "cancella"
	}
	return "accendi"
}

// ---------------- Copia dei valori toccati (.reg) ----------------

func regNome(r Radice, chiave string) string {
	switch r {
	case HKCU:
		return `HKEY_CURRENT_USER\` + chiave
	case HKLM:
		return `HKEY_LOCAL_MACHINE\` + chiave
	case HKNS:
		return `HKEY_USERS\S-1-5-20\` + chiave
	}
	return ""
}

func regTesto(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

// TestoReg: un file .reg con solo questi valori com'erano (un valore che non c'era diventa
// "nome"=-, cioè si cancella). Le chiavi nell'ordine in cui compaiono.
func TestoReg(valori []Valore) string {
	var b strings.Builder
	b.WriteString("Windows Registry Editor Version 5.00\r\n")
	var ordine []string
	per := map[string][]Valore{}
	for _, v := range valori {
		k := regNome(v.Radice, v.Chiave)
		if k == "" {
			continue
		}
		if _, ok := per[k]; !ok {
			ordine = append(ordine, k)
		}
		per[k] = append(per[k], v)
	}
	for _, k := range ordine {
		b.WriteString("\r\n[" + k + "]\r\n")
		for _, v := range per[k] {
			n := regTesto(v.Nome)
			switch {
			case !v.Esiste:
				b.WriteString(n + "=-\r\n")
			case v.Tipo == TipoDword:
				b.WriteString(n + "=dword:" + strconv.FormatUint(uint64(v.Num)|1<<32, 16)[1:] + "\r\n")
			case v.Tipo == TipoSz:
				b.WriteString(n + "=" + regTesto(v.Testo) + "\r\n")
			}
		}
	}
	return b.String()
}

// ValoriUtente: i valori di prima (HKCU) delle modifiche ancora attive, per un .reg "per rimettere
// com'era" da aprire con l'account a cui appartengono (dopo Togli Tasso, quando Tasso non può
// raggiungere le impostazioni di un altro account). Ognuno una volta sola, il primo visto.
func ValoriUtente(s *Stato) []Valore {
	var out []Valore
	visti := map[string]bool{}
	var giro func(p Precedente)
	giro = func(p Precedente) {
		for _, v := range p.Valori {
			id := strings.ToLower(v.Chiave + "|" + v.Nome)
			if v.Radice != HKCU || visti[id] {
				continue
			}
			visti[id] = true
			out = append(out, v)
		}
		for _, x := range p.Parti {
			giro(x)
		}
	}
	for _, v := range s.Voci {
		if v.Attiva() {
			giro(v.Prima)
		}
	}
	return out
}
