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

// vramStrati: memoria video con n strati sulla scheda (retta tra le due misure: 28 → 3,6 GB, 42 → 5,2 GB).
func vramStrati(n int) float64 {
	per := (vramIntero - vramMeta) / float64(stratiModello-stratiParziali)
	return vramMeta + float64(n-stratiParziali)*per
}

// stratiPer: quanti strati stanno in gb di memoria video (almeno stratiParziali, mai tutti).
func stratiPer(gb float64) int {
	per := (vramIntero - vramMeta) / float64(stratiModello-stratiParziali)
	n := stratiParziali + int((gb-vramMeta)/per+1e-9)
	return max(stratiParziali, min(n, stratiModello-1))
}

// limiteIA: regola di Sergio (23/11): l'IA usa al massimo il 44% della memoria video (5,2 GB su 12:
// l'IA intera, la più veloce)
// (4 GB su una scheda da 12), il resto è del gioco.
// Dati veri sui giochi pesanti: Sergio accetta 4,5-5 GB su 12; con 4,5 GB l'IA divisa correggeva in
// 0,7-1,8 s, intera in 0,3 s.
// (Subnautica 2 al massimo 7 GB, Throne and Liberty 6 GB; Call of Duty 10 GB → l'IA si fa da parte).
func limiteIA(memoriaGB float64) float64 { return memoriaGB * 0.44 }

// IA di scorta (richiesta di Sergio 23/11 19:00): quando l'IA grande non ci sta tutta sulla scheda,
// invece di dividerla (lenta) o spegnerla, Tasso usa un'IA piccola che ci sta tutta.
// conScorta è vero solo se il modello piccolo è scaricato e verificato.
var conScorta, conMedia bool

const (
	vramPiccola     = 2.0 // EuroLLM-1.7B: da misurare sul PC di Sergio
	vramMedia       = 4.8 // EuroLLM-9B Q3_K_M: da misurare sul PC di Sergio
	ramMinimaScorta = 7.5
)

func sceltaCorrettore(g InfoGPU, ramGB float64, core int) ProfiloCorrettore {
	p := sceltaGrande(g, ramGB, core)
	if p.Nome == "scheda" {
		return p
	}
	libera := g.MemoriaGB - g.UsataGB
	if conMedia && g.MemoriaGB > 0 && limiteIA(g.MemoriaGB) >= vramMedia && libera >= vramMedia+margineGiocoGB &&
		ramGB >= ramMinimaScorta && core >= coreMinimiIA {
		return ProfiloCorrettore{"media", -1, vramMedia, fmt.Sprintf("%.1f GB liberi sulla scheda video: uso l'IA grande compressa (circa %.1f GB), tutta sulla scheda.", libera, vramMedia)}
	}
	if !conScorta {
		return p
	}
	if g.MemoriaGB > 0 && limiteIA(g.MemoriaGB) >= vramPiccola && libera >= vramPiccola+margineGiocoGB &&
		ramGB >= ramMinimaScorta && core >= coreMinimiIA {
		return ProfiloCorrettore{"piccola", -1, vramPiccola, fmt.Sprintf("%.1f GB liberi sulla scheda video: l'IA grande non ci sta tutta, uso l'IA piccola (circa %.0f GB), veloce.", libera, vramPiccola)}
	}
	return p
}

// sceltaGrande: solo l'IA grande (intera, divisa o spenta).
func sceltaGrande(g InfoGPU, ramGB float64, core int) ProfiloCorrettore {
	libera := g.MemoriaGB - g.UsataGB
	if lim := limiteIA(g.MemoriaGB); g.MemoriaGB > 0 && lim < vramMeta && ramGB >= ramMinimaIA {
		// scheda piccola (8 GB): anche metà IA supererebbe il limite. Il gioco prima di tutto
		return ProfiloCorrettore{"", 0, 0, fmt.Sprintf("Con %.0f GB di scheda video l'IA toglierebbe troppa memoria al gioco: uso il traduttore veloce.", g.MemoriaGB)}
	}
	if lim := limiteIA(g.MemoriaGB); g.MemoriaGB > 0 && lim < vramIntero && lim >= vramMeta &&
		ramGB >= ramMinimaIA && core >= coreMinimiMeta && libera >= vramMeta+margineGiocoGB {
		// l'IA intera supererebbe il limite: ne metto sulla scheda solo la parte che ci sta
		// (circa 3,6 GB), il resto lavora sul processore
		n := stratiPer(min(lim, libera-margineGiocoGB))
		return ProfiloCorrettore{"meta", n, vramStrati(n), fmt.Sprintf("L'IA usa al massimo %.1f GB della scheda video (su %.0f): il resto lo lascio al gioco.", vramStrati(n), g.MemoriaGB)}
	}
	switch {
	case ramGB < ramMinimaIA || core < coreMinimiIA:
		return ProfiloCorrettore{"", 0, 0, fmt.Sprintf("Con questo PC (%.0f GB di memoria, %d core) l'IA rallenterebbe il gioco: uso solo il traduttore veloce.", ramGB, core)}
	case libera >= vramIntero+margineGiocoGB:
		return ProfiloCorrettore{"scheda", -1, vramIntero, fmt.Sprintf("%.1f GB liberi sulla scheda video: il correttore lavora tutto lì (circa %.0f GB).", libera, vramIntero)}
	case libera >= vramMeta+margineGiocoGB && core >= coreMinimiMeta:
		n := stratiPer(libera - margineGiocoGB)
		if lim := limiteIA(g.MemoriaGB); g.MemoriaGB > 0 {
			n = stratiPer(min(lim, libera-margineGiocoGB))
		}
		return ProfiloCorrettore{"meta", n, vramStrati(n), fmt.Sprintf("%.1f GB liberi: il correttore lavora in parte sulla scheda video (circa %.1f GB) e in parte sul processore, un po' più lento.", libera, vramStrati(n))}
	default:
		return ProfiloCorrettore{"", 0, 0, fmt.Sprintf("Liberi sulla scheda video solo %.1f GB: la lascio tutta al gioco e uso solo il traduttore veloce.", libera)}
	}
}

// sceltaLivello: il livello dell'IA scelto dall'utente nella procedura a passi ("auto", "grande",
// "media", "piccola"). Con un livello scelto Tasso non va mai sopra quello; se il gioco ha bisogno
// di memoria scende al livello sotto e poi torna su da solo.
func sceltaLivello(g InfoGPU, ramGB float64, core int, livello string) ProfiloCorrettore {
	p := sceltaCorrettore(g, ramGB, core)
	libera := g.MemoriaGB - g.UsataGB
	ci := func(gb float64) bool {
		return g.MemoriaGB > 0 && libera >= gb+margineGiocoGB && ramGB >= ramMinimaScorta && core >= coreMinimiIA
	}
	piccola := func() ProfiloCorrettore {
		if conScorta && ci(vramPiccola) {
			return ProfiloCorrettore{"piccola", -1, vramPiccola, fmt.Sprintf("Hai scelto l'IA piccola (circa %.0f GB della scheda video).", vramPiccola)}
		}
		return ProfiloCorrettore{"", 0, 0, "L'IA piccola non c'è o non ci sta adesso: uso il traduttore veloce."}
	}
	switch livello {
	case "piccola":
		return piccola()
	case "media":
		if p.Nome == "media" || p.Nome == "piccola" || p.Nome == "" || (p.Nome == "meta" && p.MemoriaGB <= vramMedia+0.01) {
			return p
		}
		if conMedia && ci(vramMedia) {
			return ProfiloCorrettore{"media", -1, vramMedia, fmt.Sprintf("Hai scelto l'IA media (circa %.1f GB della scheda video).", vramMedia)}
		}
		if !conMedia && ramGB >= ramMinimaIA && core >= coreMinimiMeta && ci(vramMeta) {
			// l'IA media non è scaricata: l'IA grande, con sulla scheda solo la parte che sta nei GB della media
			n := stratiPer(min(vramMedia, libera-margineGiocoGB))
			return ProfiloCorrettore{"meta", n, vramStrati(n), fmt.Sprintf("Hai scelto l'IA media: uso l'IA grande con circa %.1f GB sulla scheda video.", vramStrati(n))}
		}
		return piccola()
	}
	return p
}
