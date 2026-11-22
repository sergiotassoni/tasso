package traduce

import "fmt"

// InfoGPU: la scheda video più potente trovata.
type InfoGPU struct {
	Nome      string  `json:"nome"`
	MemoriaGB float64 `json:"memoria_gb"` // memoria dedicata totale
	UsataGB   float64 `json:"usata_gb"`   // usata adesso (giochi aperti compresi)
	Adatta    bool    `json:"adatta"`     // abbastanza potente per il motore IA
	Motivo    string  `json:"motivo"`
}

// requisiti del motore IA: RTX 3060 / RX 6700 XT in su, cioè almeno ~8 GB di memoria video,
// e memoria libera sufficiente per il modello (circa 4 GB) più un margine.
const (
	memoriaMinimaGB = 7.5
	liberaMinimaGB  = 4.8
)

func valutaGPU(g InfoGPU) InfoGPU {
	switch {
	case g.MemoriaGB < memoriaMinimaGB:
		g.Motivo = fmt.Sprintf("%s ha %.0f GB di memoria video: per il motore potente ne servono almeno 8 (RTX 3060 o RX 6700 XT in su). Uso il traduttore veloce.", g.Nome, g.MemoriaGB)
	case g.MemoriaGB-g.UsataGB < liberaMinimaGB:
		g.Motivo = fmt.Sprintf("Adesso la scheda video ha solo %.1f GB liberi (il gioco ne usa tanta): uso il traduttore veloce per non far scattare il gioco.", g.MemoriaGB-g.UsataGB)
	default:
		g.Adatta = true
		g.Motivo = fmt.Sprintf("%s con %.0f GB: va bene per il motore potente.", g.Nome, g.MemoriaGB)
	}
	return g
}

// ---------------------------------------------------------------- correttore che si adatta al PC

// ProfiloCorrettore: come far girare l'IA correttore (Qwen3-8B) su questo PC, scelto da solo.
// Prova delle 1000 frasi sul PC di Sergio (RX 6700 XT, 7/11/2026):
//
//	Mozilla da solo                 68,1
//	Qwen3-4B corregge (con filtri)  68,4  -> non vale la memoria, scartato
//	Qwen3-8B corregge (con filtro)  70,2  0,36 s/frase, circa 5 GB di scheda video
//
// Regola di Sergio: sui PC deboli (8 GB di RAM, processore scarso) l'IA non deve pesare sul gioco,
// quindi lì si usa solo Mozilla. Il lavoro sul processore solo su PC forti.
type ProfiloCorrettore struct {
	Nome      string  // "scheda", "meta" o "" (solo Mozilla)
	Strati    int     // strati del modello sulla scheda video (-1 = tutti)
	MemoriaGB float64 // memoria video che occupa più o meno
	Motivo    string
}

const (
	stratiModello  = 42   // EuroLLM-9B
	stratiParziali = 28   // due terzi sulla scheda, come i 24 su 36 misurati con Qwen3-8B
	vramIntero     = 5.2  // EuroLLM-9B misurato: 5,2 GB (regola di Sergio: tutto sulla scheda da 6,5 GB liberi)
	vramMeta       = 3.6  // misurato con 28 strati su 42 (regola: diviso da 4,9 GB liberi)
	margineGiocoGB = 1.3  // memoria che lasciamo sempre libera per il gioco
	ramMinimaIA    = 15.5 // "almeno 16 GB": con un po' di tolleranza per la memoria riservata
	ramLiberaMeta  = 4.0  // con il modello diviso servono circa 2 GB di RAM, più margine per il gioco
	coreMinimiMeta = 8    // per far lavorare anche il processore
	coreMinimiIA   = 4
)

func sceltaCorrettore(g InfoGPU, ramGB float64, core int) ProfiloCorrettore {
	libera := g.MemoriaGB - g.UsataGB
	switch {
	case ramGB < ramMinimaIA || core < coreMinimiIA:
		return ProfiloCorrettore{"", 0, 0, fmt.Sprintf("Con %.0f GB di memoria e %d core l'IA rallenterebbe il gioco: uso solo il traduttore veloce.", ramGB, core)}
	case libera >= vramIntero+margineGiocoGB:
		return ProfiloCorrettore{"scheda", -1, vramIntero, fmt.Sprintf("%.1f GB liberi sulla scheda video: il correttore lavora tutto lì (circa %.0f GB).", libera, vramIntero)}
	case libera >= vramMeta+margineGiocoGB && core >= coreMinimiMeta:
		return ProfiloCorrettore{"meta", stratiParziali, vramMeta, fmt.Sprintf("%.1f GB liberi: il correttore lavora in parte sulla scheda video (circa %.1f GB) e in parte sul processore, un po' più lento.", libera, vramMeta)}
	default:
		return ProfiloCorrettore{"", 0, 0, fmt.Sprintf("Solo %.1f GB liberi sulla scheda video: la lascio tutta al gioco e uso solo il traduttore veloce.", libera)}
	}
}
