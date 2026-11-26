package traduce

// Cosa serve per il motore potente: llama.cpp (MIT, versione Vulkan per Windows x64)
// e un modello aperto con licenza libera (Apache-2.0 o MIT), compresso sotto i 4 GB.
//
// IMPORTANTE: le impronte SHA-256 vanno scritte qui SOLO se prese da fonti ufficiali e controllate
// (e prima di pubblicare, provate sul PC di Sergio con la sua RX 6700 XT). Finché un'impronta è vuota il
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
	Media   FileDaScaricare // la stessa IA grande compressa (~1 GB in meno), tutta sulla scheda
	Scorta  FileDaScaricare // IA piccola di scorta (vuota finché non è verificata)
	// Riserve: i file della versione di prima, che non si scaricano più ma si usano se l'utente li
	// ha già e non ha ancora quelli nuovi (così l'aggiornamento non rompe niente).
	ModelloRiserva FileDaScaricare
	MediaRiserva   FileDaScaricare
	Nome           string // nome del modello mostrato all'utente
	Licenza        string
}

// Motore: llama.cpp b12380 (14/11/2026), zip ufficiale della release su GitHub: impronta uguale
// al digest "sha256:" della pagina della release e a quella calcolata scaricando lo zip (24/11/2026).
// Modelli: EuroLLM-9B-Instruct-2512 (Apache-2.0), quantizzati da mradermacher; impronte e
// dimensioni dall'API di Hugging Face (lfs.oid / lfs.size) e dalla pagina di ogni file.
// DA PROVARE sul PC di Sergio: prova delle 1000 frasi e tempi con la versione 2512 prima di
// pubblicare (con la versione di prima: Grande 71,6, Media 70,7, Mozilla 68,1).
var risorseIA = RisorseIA{
	Motore: FileDaScaricare{
		URL:    "https://github.com/ggml-org/llama.cpp/releases/download/b12380/llama-b12380-bin-win-vulkan-x64.zip",
		Nome:   "llama-vulkan-win-x64.zip",
		SHA256: "66a0fd3d7e7fda666336d249b164747d8cfbf0c13aa4555decbc557aa9b911d3",
		Byte:   38117943,
	},
	// Motore di prima, verificato sul PC di Sergio il 7/11/2026 (per tornare indietro basta rimetterlo
	// qui; le impronte dei suoi file restano in impronte_motori.go):
	//	URL:    "https://github.com/ggml-org/llama.cpp/releases/download/b12189/llama-b12189-bin-win-vulkan-x64.zip",
	//	SHA256: "9d56405579492423833cdde94cf46cae15d38333f585f3957cafebfc6fb0e182",
	//	Byte:   37706485,
	Modello: FileDaScaricare{
		// EuroLLM-9B-Instruct-2512 (progetto europeo UTTER: Unbabel, università di Lisbona, Edimburgo,
		// Parigi-Saclay e altri), fatto apposta per le lingue europee. Versione nuova della stessa IA:
		// stessa struttura (stessi strati, dimensione quasi uguale), addestramento finale rifatto come
		// EuroLLM-22B. Formato della chat: ChatML (<|im_start|> … <|im_end|>), nessuna istruzione di
		// sistema aggiunta da sola: va bene con i messaggi system + user di motore_ia.go.
		URL:    "https://huggingface.co/mradermacher/EuroLLM-9B-Instruct-2512-GGUF/resolve/main/EuroLLM-9B-Instruct-2512.Q4_K_M.gguf",
		Nome:   "EuroLLM-9B-Instruct-2512.Q4_K_M.gguf",
		SHA256: "fe9b50d4ba67eb0131f1ceeaab2e56ea8fb6b2bbc37391d3219c791c3b140a28",
		Byte:   5582838912,
	},
	// Media: la stessa IA compressa di più (~1 GB in meno), tutta sulla scheda.
	Media: FileDaScaricare{
		URL:    "https://huggingface.co/mradermacher/EuroLLM-9B-Instruct-2512-GGUF/resolve/main/EuroLLM-9B-Instruct-2512.Q3_K_M.gguf",
		Nome:   "EuroLLM-9B-Instruct-2512.Q3_K_M.gguf",
		SHA256: "1e1b5b8c605bc98c9eb5c95c6671514a7a37bfdc9a31f7127b0736604ba7b756",
		Byte:   4553137280,
	},
	// EuroLLM-9B-Instruct di prima (bartowski), verificata sul PC di Sergio. Prova del 22/11/2026
	// sulla metà delle 1000 frasi mai usata per scegliere: Mozilla 70,18 → EuroLLM 72,98
	// (Qwen3-8B 71,64), e sbaglia molto meno (52 frasi peggiorate contro 92).
	ModelloRiserva: FileDaScaricare{
		URL:    "https://huggingface.co/bartowski/EuroLLM-9B-Instruct-GGUF/resolve/main/EuroLLM-9B-Instruct-Q4_K_M.gguf",
		Nome:   "EuroLLM-9B-Instruct-Q4_K_M.gguf",
		SHA256: "785a3b2883532381704ef74f866f822f179a931801d1ed1cf12e6deeb838806b",
		Byte:   5582838496,
	},
	// Media di prima. Prova 1000 frasi 23/11: 70,7 (Grande 71,6, Mozilla 68,1). Impronta
	// controllata sul PC di Sergio il 23/11 (uguale a quella di bartowski su Hugging Face).
	MediaRiserva: FileDaScaricare{
		URL:    "https://huggingface.co/bartowski/EuroLLM-9B-Instruct-GGUF/resolve/main/EuroLLM-9B-Instruct-Q3_K_M.gguf",
		Nome:   "EuroLLM-9B-Instruct-Q3_K_M.gguf",
		SHA256: "c4734729b75ec7eb157b9da0b5b7e946bd6ebb2b9c864b5ee1d37f399a89f8f7",
		Byte:   4553136864,
	},
	Nome:    "IA di Tasso",
	Licenza: "Apache-2.0",
}

// motoreIADisponibile: il motore potente si può offrire solo se tutto è fissato e verificato.
func motoreIADisponibile() bool {
	r := risorseIA
	licenzaLibera := strings.HasPrefix(r.Licenza, "Apache") || strings.HasPrefix(r.Licenza, "MIT")
	return licenzaLibera && r.Motore.URL != "" && r.Motore.SHA256 != "" && r.Modello.URL != "" && r.Modello.SHA256 != ""
}

func dirMotoreIA() string { return filepath.Join(dirIA(), "llama") }

// fileDaUsare: il file nuovo; se non c'è (ancora) ma c'è quello di riserva verificato, quello.
func fileDaUsare(nuovo, riserva FileDaScaricare) FileDaScaricare {
	if riserva.Nome != "" && riserva.SHA256 != "" && !presente(dirIA(), nuovo) && presente(dirIA(), riserva) {
		return riserva
	}
	return nuovo
}

// presenteORiserva: il file nuovo o quello di riserva è scaricato e verificato.
func presenteORiserva(nuovo, riserva FileDaScaricare) bool {
	if nuovo.SHA256 != "" && presente(dirIA(), nuovo) {
		return true
	}
	return riserva.Nome != "" && riserva.SHA256 != "" && presente(dirIA(), riserva)
}

func fileMediaIA() string {
	return filepath.Join(dirIA(), fileDaUsare(risorseIA.Media, risorseIA.MediaRiserva).Nome)
}

// mediaPresente: l'IA grande compressa è scaricata e verificata (nuova o quella di prima).
func mediaPresente() bool {
	r := risorseIA.Media
	return r.URL != "" && r.SHA256 != "" && presenteORiserva(r, risorseIA.MediaRiserva)
}

func fileModelloIA() string {
	return filepath.Join(dirIA(), fileDaUsare(risorseIA.Modello, risorseIA.ModelloRiserva).Nome)
}

func iaPresente() bool {
	if !motoreIADisponibile() {
		return false
	}
	_, e1 := os.Stat(filepath.Join(dirMotoreIA(), "llama-server.exe"))
	return e1 == nil && presenteORiserva(risorseIA.Modello, risorseIA.ModelloRiserva)
}

// iaDaAggiornare: l'IA funziona ma con file della versione di prima (modello o motore): si può
// proporre all'utente di riscaricarla. Da collegare all'interfaccia.
func iaDaAggiornare() bool {
	if !iaPresente() {
		return false
	}
	vecchioMotore := impronteLlamaDi(dirMotoreIA())["llama-server.exe"] != improntaLlama["llama-server.exe"]
	return vecchioMotore || !presente(dirIA(), risorseIA.Modello)
}

// pulisciRiserveIA: dopo aver scaricato e verificato i file nuovi, cancella quelli di prima
// (circa 10 GB). Da chiamare alla fine di scaricaIA. Cancella un file vecchio solo se c'è il nuovo.
func pulisciRiserveIA() {
	r := risorseIA
	for _, c := range [][2]FileDaScaricare{{r.Modello, r.ModelloRiserva}, {r.Media, r.MediaRiserva}} {
		nuovo, vecchio := c[0], c[1]
		if vecchio.Nome == "" || vecchio.Nome == nuovo.Nome || !presente(dirIA(), nuovo) {
			continue
		}
		_ = os.Remove(filepath.Join(dirIA(), vecchio.Nome))
	}
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
		return fmt.Errorf("nello zip manca il motore dell'IA di Tasso")
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
