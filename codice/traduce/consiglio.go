package traduce

// "Analizza il mio PC": guarda scheda video, memoria e processore e consiglia quale traduttore usare.
// Le soglie vengono dalle prove sul PC di Sergio del 7/11/2026 (vedi sceltaCorrettore in gpu.go).

import (
	"encoding/binary"
	"fmt"
)

type RigaAnalisi struct {
	Titolo string `json:"titolo"`
	Testo  string `json:"testo"`
	Esito  string `json:"esito"` // "si", "forse", "no"
}

type AnalisiPC struct {
	Righe     []RigaAnalisi `json:"righe"`
	Consiglio string        `json:"consiglio"`
	Verdetto  string        `json:"verdetto"` // una frase sola, per chi non vuole i dettagli
}

func consiglioPC(g InfoGPU, ramGB float64, core int, iaDisponibile bool) AnalisiPC {
	var a AnalisiPC
	if g.Nome != "" {
		a.Righe = append(a.Righe, RigaAnalisi{"Scheda video", fmt.Sprintf("%s, %.0f GB (adesso %.1f GB liberi)", g.Nome, g.MemoriaGB, g.MemoriaGB-g.UsataGB), "si"})
	} else {
		a.Righe = append(a.Righe, RigaAnalisi{"Scheda video", "Non ho trovato una scheda video dedicata.", "forse"})
	}
	esitoRam := "si"
	if ramGB < ramMinimaIA {
		esitoRam = "forse"
	}
	a.Righe = append(a.Righe, RigaAnalisi{"Memoria del PC", fmt.Sprintf("%.0f GB", ramGB), esitoRam})
	esitoCpu := "si"
	if core < coreMinimiMeta {
		esitoCpu = "forse"
	}
	a.Righe = append(a.Righe, RigaAnalisi{"Processore", fmt.Sprintf("%d core", core), esitoCpu})
	a.Righe = append(a.Righe, RigaAnalisi{"Traduttore veloce (Mozilla)", "Va bene su questo PC. Traduce subito, non usa la scheda video.", "si"})

	var ia RigaAnalisi
	p := sceltaCorrettore(g, ramGB, core)
	// per il consiglio conta la scheda a riposo, non quanto la usa adesso un gioco aperto
	if q := sceltaCorrettore(InfoGPU{Nome: g.Nome, MemoriaGB: g.MemoriaGB}, ramGB, core); q.Nome != "" {
		p = q
	}
	switch p.Nome {
	case "scheda":
		ia = RigaAnalisi{"Correttore IA", p.Motivo + " Mentre giochi si adatta da solo: se il gioco ha bisogno di memoria, si fa da parte.", "si"}
		a.Consiglio = "Usa il traduttore veloce con il correttore IA: traduzioni più precise, senza rallentare il gioco."
		a.Verdetto = "Il tuo PC va benissimo: traduzione veloce e precisa."
	case "media", "piccola":
		ia = RigaAnalisi{"Correttore IA", p.Motivo + " Se c'è più memoria libera, passa da solo all'IA grande.", "si"}
		a.Consiglio = "Usa il traduttore veloce con il correttore IA: traduzioni più precise, senza togliere memoria al gioco."
		a.Verdetto = "Il tuo PC va bene: traduzione veloce e corretta dall'IA."
	case "meta":
		if limiteIA(g.MemoriaGB) < vramIntero && g.MemoriaGB >= vramIntero+margineGiocoGB {
			// scheda grande: l'IA è divisa solo per lasciare al gioco due terzi della scheda
			ia = RigaAnalisi{"Correttore IA", p.Motivo + " Se il gioco ha bisogno di memoria, si fa da parte.", "si"}
			a.Consiglio = "Usa il traduttore veloce con il correttore IA: traduzioni più precise, senza togliere memoria al gioco."
			a.Verdetto = "Il tuo PC va benissimo: traduzione veloce e precisa."
			break
		}
		ia = RigaAnalisi{"Correttore IA", p.Motivo + " Si mette in pausa da solo se il gioco ha bisogno di processore o memoria.", "forse"}
		a.Consiglio = "Usa il traduttore veloce. Il correttore IA può aiutare, ma con i giochi pesanti meglio lasciarlo spento."
		a.Verdetto = "Il tuo PC va bene: traduzione veloce, e più precisa con i giochi leggeri."
	default:
		ia = RigaAnalisi{"Correttore IA", "Sconsigliato su questo PC: " + p.Motivo, "no"}
		a.Consiglio = "Usa solo il traduttore veloce (Mozilla). È quello giusto per questo PC e non pesa sul gioco."
		a.Verdetto = "Il tuo PC va bene per la traduzione veloce. Lascia spenta la traduzione più precisa."
	}
	if !iaDisponibile {
		ia.Testo += " (Il correttore arriverà con un aggiornamento: per ora c'è solo il traduttore veloce.)"
	}
	a.Righe = append(a.Righe, ia)
	return a
}

// contaCore: conta le voci RelationProcessorCore (0) restituite da GetLogicalProcessorInformationEx.
// Ogni voce comincia con Relationship (4 byte) e Size (4 byte).
func contaCore(buf []byte) int {
	c := 0
	for off := 0; off+8 <= len(buf); {
		rel := binary.LittleEndian.Uint32(buf[off:])
		size := int(binary.LittleEndian.Uint32(buf[off+4:]))
		if size < 8 {
			break
		}
		if rel == 0 {
			c++
		}
		off += size
	}
	return c
}
