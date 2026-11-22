//go:build windows && collaudo

package traduce

// Modalità prova: TassoTraduce.exe --prova100 "C:\cartella"
// Nella cartella: llama-server.exe (con le sue DLL) e uno o più modelli .gguf da confrontare.
// Per ogni modello avvia il motore, traduce le 100 frasi e scrive prova-100.md e prova-100.json.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func modalitaProva(args []string) bool {
	for i, a := range args {
		if (a == "--prova100" || a == "--prova1000") && i+1 < len(args) {
			correttore, strati := false, 0
			for j, b := range args {
				if b == "--correttore" {
					correttore = true
				}
				if b == "--ngl" && j+1 < len(args) {
					fmt.Sscan(args[j+1], &strati)
				}
			}
			eseguiProvaModelli(args[i+1], correttore, strati)
			return true
		}
	}
	return false
}

// correttore = true: invece di tradurre da zero, l'IA corregge le traduzioni di Mozilla
// (una volta tutte le frasi, una volta solo quelle dubbie) e misura quanta memoria video usa.
// strati > 0: solo quegli strati sulla scheda video (--ngl N), il resto sul processore.
func eseguiProvaModelli(cartella string, correttore bool, strati int) {
	defer guard("prova100")
	frasi, err := caricaFrasiProva()
	if err != nil {
		finestraErrore("Frasi di prova non leggibili: " + err.Error())
		return
	}
	var esiti []EsitoProva
	if m, err := esitoMozillaRegistrato(); err == nil {
		esiti = append(esiti, m)
	}
	modelli, _ := filepath.Glob(filepath.Join(cartella, "*.gguf"))
	sort.Strings(modelli)
	if len(modelli) == 0 {
		finestraErrore("Nella cartella non ci sono modelli .gguf.")
		return
	}
	for _, mod := range modelli {
		s := &ServerIA{}
		var prima InfoGPU
		if strati > 0 {
			s.Strati = strati
		}
		if correttore {
			s.Contesto = 1024
			prima = controllaGPU()
		}
		nome := strings.TrimSuffix(filepath.Base(mod), ".gguf")
		if err := s.Avvia(cartella, mod); err != nil {
			esiti = append(esiti, EsitoProva{Motore: nome, Note: "non avviato: " + err.Error()})
			continue
		}
		// una frase di riscaldamento, non conteggiata
		_, _ = motoreIA{s}.Traduci(context.Background(), RichiestaTraduzione{Testo: "Hello."})
		if correttore && len(esiti) > 0 && esiti[0].Motore == "Mozilla (veloce)" {
			dopo := controllaGPU()
			vram := fmt.Sprintf("memoria video del correttore: circa %.1f GB", dopo.UsataGB-prima.UsataGB)
			for _, solo := range []bool{false, true} {
				etichetta := nome + " corregge tutte"
				if solo {
					etichetta = nome + " corregge solo le dubbie"
				}
				es := eseguiCorrezione(context.Background(), motoreConNome{motoreIA{s}, etichetta}, esiti[0], solo)
				es.Note += "; " + vram
				esiti = append(esiti, es)
			}
			s.Ferma()
			continue
		}
		es := eseguiProva(context.Background(), motoreConNome{motoreIA{s}, nome}, frasi)
		esiti = append(esiti, es)
		s.Ferma()
	}
	_ = os.WriteFile(filepath.Join(cartella, "prova-1000.md"), []byte(rapportoProva(esiti)), 0o644)
	b, _ := json.MarshalIndent(esiti, "", " ")
	_ = os.WriteFile(filepath.Join(cartella, "prova-1000.json"), b, 0o644)
	finestraInfo("Prova finita. Risultati in:\n" + filepath.Join(cartella, "prova-1000.md"))
}

type motoreConNome struct {
	Motore
	nome string
}

func (m motoreConNome) Nome() string { return m.nome }
