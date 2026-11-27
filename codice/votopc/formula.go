package votopc

import (
	"math"
	"regexp"
)

// Versione della formula del voto. Va aumentata ogni volta che cambiano i conti, così una futura
// classifica online confronta solo voti fatti con la stessa formula.
//
//	1 = Tasso fino alla 3.1: 60% velocità dei componenti (processore, RAM, disco) + 40% leggerezza.
//	2 = Tasso 3.2: entra la scheda video, la RAM conta anche quantità e configurazione, il
//	    raffreddamento è un voto a parte (fuori dal totale).
const Formula = 2

// Pesi della formula 2 (la parte "potenza del PC"). Sono uguali per tutti, Gaming o Ufficio:
// così il numero si può confrontare tra PC diversi.
const (
	PesoProcessore = 0.35
	PesoScheda     = 0.25
	PesoRAM        = 0.15
	PesoDisco      = 0.25
	QuotaPotenza   = 0.6 // il resto (0.4) è la leggerezza di Windows
)

// Punti100 porta i punti misurati (1000 = PC da gioco medio di oggi) a un voto 1-100:
// 1000 punti = 70, ogni volta che la velocità raddoppia +25.
func Punti100(p int) int {
	x := math.Round(70 + 25*math.Log2(math.Max(1, float64(p))/1000))
	return int(math.Max(1, math.Min(100, x)))
}

// PuntiProcessore unisce un core e tutti i core (media geometrica, metà e metà).
func PuntiProcessore(cpu1, cpuN int) int {
	if cpu1 <= 0 || cpuN <= 0 {
		return 0
	}
	return int(math.Round(math.Sqrt(float64(cpu1) * float64(cpuN))))
}

// Parametri: i voti 1-100 di ogni parte del PC. 0 = non misurato.
type Parametri struct {
	Processore  int `json:"processore"`
	SchedaVideo int `json:"schedaVideo"`
	RAM         int `json:"ram"`
	Disco       int `json:"disco"`
	Leggerezza  int `json:"leggerezza"`
	// Raffreddamento: solo se Tasso legge la temperatura del processore. Fuori dal totale.
	Raffreddamento int `json:"raffreddamento,omitempty"`
}

// Potenza: media pesata dei componenti misurati (quelli a 0 non contano e il loro peso si
// ridistribuisce sugli altri).
func Potenza(p Parametri) int {
	s, w := 0.0, 0.0
	for _, x := range []struct {
		v int
		w float64
	}{{p.Processore, PesoProcessore}, {p.SchedaVideo, PesoScheda}, {p.RAM, PesoRAM}, {p.Disco, PesoDisco}} {
		if x.v > 0 {
			s += float64(x.v) * x.w
			w += x.w
		}
	}
	if w == 0 {
		return 0
	}
	return int(math.Round(s / w))
}

// Totale: 60% potenza + 40% leggerezza, da 1 a 100.
func Totale(p Parametri) int {
	pot := Potenza(p)
	if pot == 0 {
		return 0
	}
	l := math.Max(1, float64(p.Leggerezza))
	t := math.Round(QuotaPotenza*float64(pot) + (1-QuotaPotenza)*l)
	return int(math.Max(1, math.Min(100, t)))
}

// Classifica: la fascia del voto totale (chiave fissa, il testo lo sceglie la finestra).
func Classifica(tot int) string {
	switch {
	case tot >= 95:
		return "top"
	case tot >= 80:
		return "veloce"
	case tot >= 60:
		return "buono"
	case tot >= 40:
		return "ufficio"
	}
	return "lento"
}

// Risultato: il voto in forma stabile, pronto per essere salvato o, un domani, mandato a una
// classifica. Niente dati personali: nessun nome utente, nome del PC o numero di serie.
type Risultato struct {
	Formula      int       `json:"formula"`
	Data         string    `json:"data"` // RFC 3339
	Totale       int       `json:"totale"`
	Classifica   string    `json:"classifica"`
	Potenza      int       `json:"potenza"`
	Parametri    Parametri `json:"parametri"`
	Processore   string    `json:"processore"`
	Core         int       `json:"core"`
	SchedaVideo  string    `json:"schedaVideo"`
	VRAMGB       float64   `json:"vramGB"`
	SchedaNota   bool      `json:"schedaNota"` // false = voto stimato dalla memoria video
	RAMGB        float64   `json:"ramGB"`
	RAMTipo      string    `json:"ramTipo,omitempty"` // "DDR4", "DDR5"...
	RAMMTs       int       `json:"ramMTs,omitempty"`  // velocità effettiva in MT/s
	RAMBanchi    int       `json:"ramBanchi,omitempty"`
	RAMSaldata   bool      `json:"ramSaldata,omitempty"`   // LPDDR saldata sulla scheda madre: niente banchi
	RAMCanali    int       `json:"ramCanali,omitempty"`    // stimati dal numero di banchi
	Disco        string    `json:"disco"`                  // "nvme", "ssd", "hdd" (stimato dalla velocità misurata), "" se saltato
	DiscoStimato bool      `json:"discoStimato,omitempty"` // tipo stimato dalla velocità (Windows non l'ha detto)
	DiscoMBs     int       `json:"discoMBs,omitempty"`     // lettura sequenziale misurata
	Portatile    bool      `json:"portatile,omitempty"`
	SchedaMadre  string    `json:"schedaMadre,omitempty"` // solo informazione, senza voto
	BIOS         string    `json:"bios,omitempty"`
	TempMax      float64   `json:"tempMax,omitempty"` // °C del processore sotto carico, se letta
}

// TipoDisco: dalla velocità misurata (punti, 1000 = SSD NVMe medio).
func TipoDisco(punti int) string {
	switch {
	case punti <= 0:
		return ""
	case punti >= 700:
		return "nvme"
	case punti >= 150:
		return "ssd"
	}
	return "hdd"
}

// ---------------- Memoria RAM ----------------

// TipoRAM: il nome dal codice SMBIOS (Win32_PhysicalMemory.SMBIOSMemoryType) e dalla velocità.
// LPDDR4X e LPDDR5X hanno lo stesso codice di LPDDR4 e LPDDR5: si riconoscono dalla velocità,
// oltre il massimo ufficiale delle versioni senza X (3200 e 6400 MT/s).
func TipoRAM(smbios, mts int) (tipo string, saldata bool) {
	switch smbios {
	case 24:
		return "DDR3", false
	case 26:
		return "DDR4", false
	case 34:
		return "DDR5", false
	case 27:
		return "LPDDR", true
	case 28:
		return "LPDDR2", true
	case 29:
		return "LPDDR3", true
	case 30:
		if mts > 3200 {
			return "LPDDR4X", true
		}
		return "LPDDR4", true
	case 35:
		if mts > 6400 {
			return "LPDDR5X", true
		}
		return "LPDDR5", true
	}
	return "", false
}

// Processori che usano LPDDR5X anche a 6400 MT/s (Core Ultra, Ryzen 7040/8040 e AI, Snapdragon X):
// con loro "6400" vuol dire LPDDR5X. I Ryzen 6000 usavano LPDDR5-6400, quindi senza processore
// noto 6400 resta LPDDR5.
var reCPULPDDR5X = regexp.MustCompile(`(?i)core\W*(\(tm\))?\s*ultra|ryzen\b.*\b(7\d4\d|8\d4\d)[a-z]*\b|ryzen\s+ai\b|snapdragon`)

// TipoRAMCPU: come TipoRAM, guardando anche il processore per LPDDR5X a 6400 MT/s.
func TipoRAMCPU(smbios, mts int, cpu string) (tipo string, saldata bool) {
	tipo, saldata = TipoRAM(smbios, mts)
	if tipo == "LPDDR5" && mts == 6400 && reCPULPDDR5X.MatchString(cpu) {
		tipo = "LPDDR5X"
	}
	return
}

// Canali: con 2 o più banchi la memoria lavora quasi sempre in doppio canale (stima: Windows non
// lo dice). 0 = non so.
func Canali(banchi int) int {
	switch {
	case banchi <= 0:
		return 0
	case banchi == 1:
		return 1
	}
	return 2
}

// VotoCapacita: quanta RAM c'è. 8 GB = 40, 16 GB = 65 (il minimo per giocare oggi), 32 GB = 85,
// 64 GB e più = 100.
func VotoCapacita(gb float64) int {
	if gb <= 0 {
		return 0
	}
	v := 40 + 25*math.Log2(gb/8)
	if gb > 16 {
		v = 65 + 20*math.Log2(gb/16)
	}
	return int(math.Max(1, math.Min(100, math.Round(v))))
}

// VotoConfigurazione: velocità effettiva (MT/s) per canali. DDR4-3200 in doppio canale = 70,
// ogni raddoppio +25 (DDR5-6000 in doppio canale ≈ 93, DDR4-2666 su un banco solo ≈ 38).
func VotoConfigurazione(mts, canali int) int {
	if mts <= 0 || canali <= 0 {
		return 0
	}
	v := 70 + 25*math.Log2(float64(mts*canali)/6400)
	return int(math.Max(1, math.Min(100, math.Round(v))))
}

// VotoRAM: 50% velocità misurata con la prova (copia di 128 MB: dentro c'è già l'effetto di
// MT/s e canali, misurato davvero), 30% quantità, 20% configurazione letta da Windows.
// Se Windows non dà quantità o velocità, conta solo la misura.
func VotoRAM(misurato, capacita, config int) int {
	s, w := 0.0, 0.0
	for _, x := range []struct {
		v int
		w float64
	}{{misurato, 0.5}, {capacita, 0.3}, {config, 0.2}} {
		if x.v > 0 {
			s += float64(x.v) * x.w
			w += x.w
		}
	}
	if w == 0 {
		return 0
	}
	return int(math.Round(s / w))
}

// ---------------- Raffreddamento ----------------

// VotoRaffreddamento: dalla temperatura più alta del processore sotto carico. Fino a 60 °C = 100,
// 70 = 88, 80 = 70, 90 = 45, 100 = 15. 0 = temperatura non letta (il voto non compare).
// Non entra nel voto totale: dipende dalla stanza, dal sensore e da quanto dura la prova.
func VotoRaffreddamento(tempC float64) int {
	if tempC < 20 || tempC > 120 {
		return 0
	}
	punti := [][2]float64{{60, 100}, {70, 88}, {80, 70}, {90, 45}, {100, 15}}
	if tempC <= punti[0][0] {
		return 100
	}
	for i := 1; i < len(punti); i++ {
		if tempC <= punti[i][0] {
			a, b := punti[i-1], punti[i]
			return int(math.Round(a[1] + (tempC-a[0])/(b[0]-a[0])*(b[1]-a[1])))
		}
	}
	return 10
}
