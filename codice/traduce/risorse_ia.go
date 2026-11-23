package traduce

// Cosa serve per il motore potente: llama.cpp (MIT, versione Vulkan per Windows x64)
// e un modello aperto con licenza libera (Apache-2.0 o MIT), compresso sotto i 4 GB.
//
// IMPORTANTE: le impronte SHA-256 vanno scritte qui SOLO dopo aver scaricato e controllato
// i file a mano con Sergio (prova sulla sua RX 6700 XT). Finché un'impronta è vuota il
// programma NON scarica quel file e il motore potente non viene offerto: niente file non verificati.

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

type RisorseIA struct {
	Motore  FileDaScaricare // zip ufficiale di llama.cpp (Vulkan, Windows x64)
	Modello FileDaScaricare // file .gguf
	Scorta  FileDaScaricare // IA piccola di scorta (vuota finché non è verificata)
	Nome    string          // nome del modello mostrato all'utente
	Licenza string
}

// Verificati sul PC di Sergio il 7/11/2026 (file scaricati e impronte controllate a mano).
var risorseIA = RisorseIA{
	Motore: FileDaScaricare{
		URL:    "https://github.com/ggml-org/llama.cpp/releases/download/b12189/llama-b12189-bin-win-vulkan-x64.zip",
		Nome:   "llama-vulkan-win-x64.zip",
		SHA256: "9d56405579492423833cdde94cf46cae15d38333f585f3957cafebfc6fb0e182",
		Byte:   37706485,
	},
	Modello: FileDaScaricare{
		// EuroLLM-9B-Instruct (progetto europeo UTTER: Unbabel, università di Lisbona, Edimburgo,
		// Parigi-Saclay e altri), fatto apposta per tradurre le lingue europee. Prova del 22/11/2026
		// sulla metà delle 1000 frasi mai usata per scegliere: Mozilla 70,18 → EuroLLM 72,98
		// (Qwen3-8B 71,64), e sbaglia molto meno (52 frasi peggiorate contro 92).
		URL:    "https://huggingface.co/bartowski/EuroLLM-9B-Instruct-GGUF/resolve/main/EuroLLM-9B-Instruct-Q4_K_M.gguf",
		Nome:   "EuroLLM-9B-Instruct-Q4_K_M.gguf",
		SHA256: "785a3b2883532381704ef74f866f822f179a931801d1ed1cf12e6deeb838806b",
		Byte:   5582838496,
	},
	Nome:    "EuroLLM-9B (progetto europeo UTTER)",
	Licenza: "Apache-2.0",
}

// motoreIADisponibile: il motore potente si può offrire solo se tutto è fissato e verificato.
func motoreIADisponibile() bool {
	r := risorseIA
	licenzaLibera := strings.HasPrefix(r.Licenza, "Apache") || strings.HasPrefix(r.Licenza, "MIT")
	return licenzaLibera && r.Motore.URL != "" && r.Motore.SHA256 != "" && r.Modello.URL != "" && r.Modello.SHA256 != ""
}

func dirMotoreIA() string   { return filepath.Join(dirIA(), "llama") }
func fileScortaIA() string  { return filepath.Join(dirIA(), risorseIA.Scorta.Nome) }

// scortaPresente: l'IA piccola di scorta è scaricata e verificata.
func scortaPresente() bool {
	r := risorseIA.Scorta
	return r.URL != "" && r.SHA256 != "" && presente(dirIA(), r)
}
func fileModelloIA() string { return filepath.Join(dirIA(), risorseIA.Modello.Nome) }

func iaPresente() bool {
	if !motoreIADisponibile() {
		return false
	}
	_, e1 := os.Stat(filepath.Join(dirMotoreIA(), "llama-server.exe"))
	return e1 == nil && presente(dirIA(), risorseIA.Modello)
}

// estraiMotore apre lo zip (già verificato con SHA-256) e copia solo i file necessari.
func estraiMotore(zipPath, dest string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	trovato := false
	for _, f := range zr.File {
		nome := filepath.Base(strings.ReplaceAll(f.Name, "\\", "/"))
		basso := strings.ToLower(nome)
		utile := basso == "llama-server.exe" || strings.HasSuffix(basso, ".dll") || basso == "license" || strings.HasPrefix(basso, "license")
		if f.FileInfo().IsDir() || !utile || nome == "." || nome == ".." {
			continue
		}
		if basso == "llama-server.exe" {
			trovato = true
		}
		if err := copiaDaZip(f, filepath.Join(dest, nome)); err != nil {
			return err
		}
	}
	if !trovato {
		return fmt.Errorf("nello zip non c'è llama-server.exe")
	}
	return nil
}

func copiaDaZip(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, io.LimitReader(rc, 1<<30))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// iaPresenteSvelto: per l'interfaccia, che non deve mai aspettare. Restituisce l'ultimo risultato
// noto e intanto ricontrolla in sottofondo (il primo controllo legge 5 GB, qualche secondo).
var (
	iaUltimo    atomic.Bool
	iaControllo atomic.Bool
)

var quandoCambiaIA func()

func iaPresenteSvelto() bool {
	if iaControllo.CompareAndSwap(false, true) {
		go func() {
			defer guard("controllo-ia")
			defer iaControllo.Store(false)
			ora := iaPresente()
			if iaUltimo.Swap(ora) != ora && quandoCambiaIA != nil {
				quandoCambiaIA() // la finestra si aggiorna appena finisce il controllo (5 GB, qualche secondo)
			}
		}()
	}
	return iaUltimo.Load()
}
