package traduce

// CTRL-FIN-20 F20-7 (S4 di O3-sicgen): llama-server e whisper-server girano da amministratore e
// leggono il modello (.gguf, .bin) dalla cartella dei modelli, che qualsiasi programma dell'utente
// può cambiare. I programmi si copiano nella cartella protetta, i modelli (gigabyte) no. Quindi:
//   - prima di avviare il server apro il modello in sola lettura SENZA lasciare a nessun altro
//     scriverlo, rinominarlo o cancellarlo (condivisione solo lettura): da lì nessuno lo può
//     cambiare o sostituire;
//   - mentre è bloccato lo controllo con l'impronta SHA-256. Per non rileggere ogni volta gigabyte,
//     un controllo riuscito si ricorda insieme al numero del file e al numero di modifica di NTFS
//     (USN, che un programma senza permessi non può rimettere indietro); se il disco non lo sa dire,
//     si rilegge tutto;
//   - il blocco resta finché il server non ha caricato il modello (poi lo legge lui).

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/windows"
)

const fsctlReadFileUsnData = 0x000900eb

var (
	muBloccati       sync.Mutex
	bloccatiVisti    = map[string]string{} // percorso -> segno del controllo riuscito
	caricaBloccatiDo sync.Once
)

// modelliConosciuti: i modelli che Tasso scarica, con la loro impronta.
func modelliConosciuti() []struct {
	dir string
	f   FileDaScaricare
} {
	var out []struct {
		dir string
		f   FileDaScaricare
	}
	for _, f := range []FileDaScaricare{risorseIA.Modello, risorseIA.Media, risorseIA.Scorta, risorseIA.ModelloRiserva, risorseIA.MediaRiserva} {
		out = append(out, struct {
			dir string
			f   FileDaScaricare
		}{dirIA(), f})
	}
	for _, f := range []FileDaScaricare{risorseVoci.Modello, risorseVoci.VAD} {
		out = append(out, struct {
			dir string
			f   FileDaScaricare
		}{dirVoci(), f})
	}
	return out
}

// modelloAtteso: l'impronta attesa per il modello in p. ok=false se p non è un modello di Tasso.
func modelloAtteso(p string) (FileDaScaricare, bool) {
	p = filepath.Clean(p)
	for _, m := range modelliConosciuti() {
		if m.f.Nome != "" && m.f.SHA256 != "" && strings.EqualFold(p, filepath.Clean(filepath.Join(m.dir, m.f.Nome))) {
			return m.f, true
		}
	}
	return FileDaScaricare{}, false
}

// fileBloccati: i controlli riusciti, solo nella cartella protetta (come fileVerificati).
func fileBloccati() string {
	if f := fileVerificati(); f != "" {
		return filepath.Join(filepath.Dir(f), "modelli-bloccati.json")
	}
	return ""
}

func caricaBloccati() {
	caricaBloccatiDo.Do(func() {
		f := fileBloccati()
		if f == "" {
			return
		}
		b, err := os.ReadFile(f)
		if err != nil || len(b) > 1<<16 {
			return
		}
		m := map[string]string{}
		if json.Unmarshal(b, &m) != nil {
			return
		}
		muBloccati.Lock()
		for k, v := range m {
			bloccatiVisti[k] = v
		}
		muBloccati.Unlock()
	})
}

func ricordaBloccato(p, segno string) {
	muBloccati.Lock()
	bloccatiVisti[strings.ToLower(p)] = segno
	b, err := json.Marshal(bloccatiVisti)
	muBloccati.Unlock()
	f := fileBloccati()
	if err != nil || f == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(f), 0o755)
	if os.WriteFile(f+".tmp", b, 0o644) == nil {
		_ = os.Rename(f+".tmp", f)
	}
}

// segnoNTFS: numero del volume e del file, dimensione, data e numero di modifica (USN) del file
// aperto. "" se il disco non tiene il registro delle modifiche (allora si rilegge tutto).
func segnoNTFS(h windows.Handle, info *windows.ByHandleFileInformation) string {
	buf := make([]byte, 1024)
	var n uint32
	if windows.DeviceIoControl(h, fsctlReadFileUsnData, nil, 0, &buf[0], uint32(len(buf)), &n, nil) != nil || n < 8 {
		return ""
	}
	var usn int64
	switch binary.LittleEndian.Uint16(buf[4:6]) {
	case 2: // USN_RECORD_V2: numeri dei file da 64 bit
		if n < 32 {
			return ""
		}
		usn = int64(binary.LittleEndian.Uint64(buf[24:32]))
	case 3: // USN_RECORD_V3 (ReFS): numeri dei file da 128 bit
		if n < 48 {
			return ""
		}
		usn = int64(binary.LittleEndian.Uint64(buf[40:48]))
	default:
		return ""
	}
	if usn <= 0 {
		return ""
	}
	return fmt.Sprintf("%08x-%08x%08x-%d-%d-%d", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow,
		int64(info.FileSizeHigh)<<32|int64(info.FileSizeLow), int64(info.LastWriteTime.HighDateTime)<<32|int64(info.LastWriteTime.LowDateTime), usn)
}

// bloccaModello apre il modello p bloccato (nessuno lo può cambiare, rinominare o cancellare) e lo
// controlla. sblocca va chiamata quando il server ha caricato il modello (o non è partito).
// I modelli che non sono di Tasso (solo nelle prove di Tasso-collaudo, da una cartella scelta da
// chi prova) si bloccano e basta; dentro la cartella dei dati di Tasso un modello sconosciuto non si usa.
func bloccaModello(p string) (sblocca func(), err error) {
	atteso, conosciuto := modelloAtteso(p)
	if !conosciuto && dirApp != "" && dentroCartella(p, dirApp) {
		return nil, errors.New("modello sconosciuto: per sicurezza non lo uso")
	}
	// R3-2: il percorso vero di TassoTraduce, preso prima di aprire il modello (vedi sotto)
	radice := ""
	if dirApp != "" && dentroCartella(p, dirApp) {
		r, err := radiceVera(dirApp)
		if err != nil {
			return nil, errors.New("la cartella dei modelli porta a un'altra cartella (collegamento o giunzione) o non si può controllare: per sicurezza non uso il modello")
		}
		radice = r
	}
	u, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return nil, err
	}
	// FILE_FLAG_OPEN_REPARSE_POINT: un collegamento non si segue (si apre lui, e non è un file normale)
	h, err := windows.CreateFile(u, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_SEQUENTIAL_SCAN, 0)
	if err != nil {
		return nil, fmt.Errorf("non riesco ad aprire il modello solo per me (forse lo usa un altro programma): %w", err)
	}
	f := os.NewFile(uintptr(h), p)
	tenuto := false
	defer func() {
		if !tenuto {
			f.Close()
		}
	}()
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return nil, errors.New("il modello non è un file normale: per sicurezza non lo uso")
	}
	// R3-2: il server riapre il modello per nome. Con il modello aperto qui (senza condivisione per
	// rinominare o cancellare), Windows non lascia rinominare né togliere le cartelle vere che lo
	// contengono; una giunzione invece si toglie e si rifà verso un'altra cartella. Quindi, adesso
	// che il file è aperto: da TassoTraduce in giù nessun pezzo del percorso è un collegamento o una
	// giunzione, e il percorso vero del file aperto è proprio quello che passo al server.
	if radice != "" {
		if percorsoDatiSicuro(p) != nil {
			return nil, errors.New("la cartella dei modelli porta a un'altra cartella (collegamento o giunzione) o non si può controllare: per sicurezza non uso il modello")
		}
		rel, e := filepath.Rel(filepath.Clean(dirApp), filepath.Clean(p))
		if v := finaleDiHandle(h); e != nil || v == "" || !strings.EqualFold(filepath.Clean(v), filepath.Join(radice, rel)) {
			return nil, errors.New("la cartella dei modelli porta a un'altra cartella (collegamento o giunzione) o non si può controllare: per sicurezza non uso il modello")
		}
	}
	if conosciuto {
		dim := int64(info.FileSizeHigh)<<32 | int64(info.FileSizeLow)
		if atteso.Byte > 0 && dim != atteso.Byte {
			return nil, errors.New("il modello non è quello giusto: riscaricalo")
		}
		segno := segnoNTFS(h, &info)
		caricaBloccati()
		muBloccati.Lock()
		visto := segno != "" && bloccatiVisti[strings.ToLower(p)] == segno+"|"+strings.ToLower(atteso.SHA256)
		muBloccati.Unlock()
		if !visto {
			s := sha256.New()
			if _, err := io.Copy(s, f); err != nil {
				return nil, fmt.Errorf("non riesco a leggere il modello: %w", err)
			}
			if !strings.EqualFold(hex.EncodeToString(s.Sum(nil)), atteso.SHA256) {
				return nil, errors.New("il modello è stato cambiato: per sicurezza non lo uso (riscaricalo)")
			}
			if segno != "" {
				ricordaBloccato(p, segno+"|"+strings.ToLower(atteso.SHA256))
			}
		}
	}
	tenuto = true
	var una sync.Once
	return func() { una.Do(func() { f.Close() }) }, nil
}

// dentroCartella: p sta dentro d (senza badare a maiuscole).
func dentroCartella(p, d string) bool {
	rel, err := filepath.Rel(strings.ToLower(filepath.Clean(d)), strings.ToLower(filepath.Clean(p)))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
