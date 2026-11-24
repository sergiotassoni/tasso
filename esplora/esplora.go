// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

// Package esplora legge le cartelle per la vista "Esplora i file" di Tasso Zip: elenco con
// limiti (niente blocchi su cartelle enormi), ordinamento, ricerca per nome con limiti,
// misura di file e cartelle, tipo di file per le icone. Gira anche su Linux (per i test).
package esplora

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Voce: un file o una cartella.
type Voce struct {
	Nome      string    `json:"nome"`
	Percorso  string    `json:"percorso"`
	Cartella  bool      `json:"cartella"`
	Dim       int64     `json:"dim"`
	Quando    time.Time `json:"quando"`
	Categoria string    `json:"cat"` // vedi Categoria
	Ext       string    `json:"ext"` // estensione senza il punto, minuscola ("tar.gz" per i doppi)
	Nascosto  bool      `json:"nascosto"`
	Archivio  bool      `json:"archivio"`
	Dove      string    `json:"dove,omitempty"` // solo nella ricerca: la cartella, relativa a quella cercata
}

// Elenco: il contenuto di una cartella.
type Elenco struct {
	Percorso string `json:"percorso"`
	Padre    string `json:"padre"` // "" = sono già alla radice del disco
	Voci     []Voce `json:"voci"`
	Nascosti int    `json:"nascosti"` // quanti non mostrati perché nascosti o di sistema
	Troppi   bool   `json:"troppi"`   // più di Max voci: mostro solo le prime
	Lento    bool   `json:"lento"`    // fermato per il tempo (cartella di rete lenta...)
}

// Opzioni di Elenca.
type Opzioni struct {
	Nascosti bool          // mostra anche i file nascosti e di sistema
	Max      int           // al massimo (0 = MaxVoci)
	Tempo    time.Duration // tempo massimo (0 = TempoMax)
}

const (
	MaxVoci  = 20000
	TempoMax = 8 * time.Second
)

// Categorie di file, per le icone.
const (
	CatCartella   = "cartella"
	CatArchivio   = "archivio"
	CatImmagine   = "immagine"
	CatVideo      = "video"
	CatAudio      = "audio"
	CatDocumento  = "documento"
	CatEseguibile = "eseguibile"
	CatGenerico   = "file"
)

var categorie = map[string]string{}

func init() {
	agg := func(cat string, est ...string) {
		for _, e := range est {
			categorie[e] = cat
		}
	}
	agg(CatArchivio, "zip", "7z", "rar", "tar", "gz", "tgz", "bz2", "tbz2", "tbz", "xz", "txz", "zst", "tzst",
		"tar.gz", "tar.bz2", "tar.xz", "tar.zst", "001", "r00", "cab", "iso")
	agg(CatImmagine, "jpg", "jpeg", "png", "gif", "bmp", "webp", "heic", "heif", "avif", "tif", "tiff", "ico", "svg", "raw", "cr2", "nef", "psd")
	agg(CatVideo, "mp4", "m4v", "mkv", "avi", "mov", "webm", "wmv", "flv", "mpg", "mpeg", "3gp", "ts")
	agg(CatAudio, "mp3", "m4a", "aac", "ogg", "opus", "flac", "wav", "wma", "mid", "midi")
	agg(CatDocumento, "pdf", "doc", "docx", "odt", "rtf", "txt", "md", "xls", "xlsx", "ods", "csv", "ppt", "pptx", "odp",
		"epub", "html", "htm", "xml", "json", "log", "ini")
	agg(CatEseguibile, "exe", "msi", "bat", "cmd", "ps1", "vbs", "js", "com", "scr", "lnk", "msix", "appx", "jar", "reg", "dll", "sys")
}

// Estensione: "Foto.TAR.GZ" -> "tar.gz", "nota.txt" -> "txt", "README" -> "".
func Estensione(nome string) string {
	n := strings.ToLower(nome)
	for _, d := range []string{".tar.gz", ".tar.bz2", ".tar.xz", ".tar.zst"} {
		if strings.HasSuffix(n, d) && len(n) > len(d) {
			return d[1:]
		}
	}
	e := filepath.Ext(n)
	if e == n || len(e) < 2 {
		return ""
	}
	return e[1:]
}

// Categoria del file dal nome (per le cartelle: CatCartella).
func Categoria(nome string, cartella bool) string {
	if cartella {
		return CatCartella
	}
	if c, ok := categorie[Estensione(nome)]; ok {
		return c
	}
	if c, ok := categorie[strings.TrimPrefix(strings.ToLower(filepath.Ext(nome)), ".")]; ok {
		return c
	}
	return CatGenerico
}

// ErrNonCartella: il percorso non è una cartella.
var ErrNonCartella = errors.New("non è una cartella")

// Padre: la cartella sopra ("" se sono alla radice di un disco o di una condivisione).
func Padre(p string) string {
	p = filepath.Clean(p)
	d := filepath.Dir(p)
	if d == p || d == "." {
		return ""
	}
	return d
}

// voceDa costruisce la Voce da un DirEntry (segue i collegamenti solo per sapere se sono
// cartelle; non li percorre mai nella ricerca).
func voceDa(dir string, de fs.DirEntry) (Voce, bool) {
	nome := de.Name()
	p := filepath.Join(dir, nome)
	fi, err := de.Info()
	if err != nil {
		return Voce{}, false // sparito nel frattempo
	}
	v := Voce{Nome: nome, Percorso: p, Quando: fi.ModTime(), Nascosto: nascosto(nome, fi)}
	isDir := fi.IsDir()
	if fi.Mode()&fs.ModeSymlink != 0 {
		if s, err := os.Stat(p); err == nil {
			isDir = s.IsDir()
			if !isDir {
				fi = s
			}
		}
	}
	v.Cartella = isDir
	if !isDir {
		v.Dim = fi.Size()
		v.Ext = Estensione(nome)
	}
	v.Categoria = Categoria(nome, isDir)
	v.Archivio = v.Categoria == CatArchivio && v.Ext != "iso" && v.Ext != "cab"
	return v, true
}

// Elenca legge la cartella. Si ferma dopo Max voci o dopo Tempo (le voci lette fin lì restano).
func Elenca(ctx context.Context, dir string, opz Opzioni) (Elenco, error) {
	if opz.Max <= 0 {
		opz.Max = MaxVoci
	}
	if opz.Tempo <= 0 {
		opz.Tempo = TempoMax
	}
	dir = filepath.Clean(dir)
	el := Elenco{Percorso: dir, Padre: Padre(dir), Voci: []Voce{}}
	fi, err := os.Stat(dir)
	if err != nil {
		return el, err
	}
	if !fi.IsDir() {
		return el, ErrNonCartella
	}
	f, err := os.Open(dir)
	if err != nil {
		return el, err
	}
	defer f.Close()
	scadenza := time.Now().Add(opz.Tempo)
	for {
		if ctx.Err() != nil {
			return el, ctx.Err()
		}
		blocco, err := f.ReadDir(256)
		for _, de := range blocco {
			v, ok := voceDa(dir, de)
			if !ok {
				continue
			}
			if v.Nascosto && !opz.Nascosti {
				el.Nascosti++
				continue
			}
			if len(el.Voci) >= opz.Max {
				el.Troppi = true
				break
			}
			el.Voci = append(el.Voci, v)
		}
		if el.Troppi {
			break
		}
		if err == io.EOF || (err == nil && len(blocco) == 0) {
			break
		}
		if err != nil {
			if len(el.Voci) == 0 {
				return el, err
			}
			break // errore a metà: mostro quello che ho
		}
		if time.Now().After(scadenza) {
			el.Lento = true
			break
		}
	}
	Ordina(el.Voci, "nome", false)
	return el, nil
}

// ---------------- ordinamento ----------------

// ConfrontaNomi: ordine "naturale" senza maiuscole/minuscole ("foto 2" prima di "foto 10").
func ConfrontaNomi(a, b string) int {
	for a != "" && b != "" {
		ra, la := utf8.DecodeRuneInString(a)
		rb, lb := utf8.DecodeRuneInString(b)
		if unicode.IsDigit(ra) && unicode.IsDigit(rb) {
			na, ca := cifre(a)
			nb, cb := cifre(b)
			// confronto i numeri senza gli zeri davanti, prima per lunghezza
			ta, tb := strings.TrimLeft(na, "0"), strings.TrimLeft(nb, "0")
			if len(ta) != len(tb) {
				if len(ta) < len(tb) {
					return -1
				}
				return 1
			}
			if ta != tb {
				if ta < tb {
					return -1
				}
				return 1
			}
			a, b = a[ca:], b[cb:]
			continue
		}
		fa, fb := piega(ra), piega(rb)
		if fa != fb {
			if fa < fb {
				return -1
			}
			return 1
		}
		a, b = a[la:], b[lb:]
	}
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return -1
	}
	return 1
}

func cifre(s string) (string, int) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i], i
}

// piega: minuscola e senza accento per le lettere più comuni (è = e).
func piega(r rune) rune {
	r = unicode.ToLower(r)
	switch r {
	case 'à', 'á', 'â', 'ä', 'ã', 'å', 'ą':
		return 'a'
	case 'è', 'é', 'ê', 'ë', 'ę':
		return 'e'
	case 'ì', 'í', 'î', 'ï':
		return 'i'
	case 'ò', 'ó', 'ô', 'ö', 'õ':
		return 'o'
	case 'ù', 'ú', 'û', 'ü':
		return 'u'
	case 'ç', 'ć':
		return 'c'
	case 'ñ', 'ń':
		return 'n'
	case 'ś':
		return 's'
	case 'ź', 'ż':
		return 'z'
	case 'ł':
		return 'l'
	}
	return r
}

// Ordina le voci: le cartelle sempre prima, poi per campo ("nome", "dim", "tipo", "quando").
// A parità, per nome.
func Ordina(v []Voce, campo string, discendente bool) {
	sort.SliceStable(v, func(i, j int) bool {
		a, b := v[i], v[j]
		if a.Cartella != b.Cartella {
			return a.Cartella
		}
		c := 0
		switch campo {
		case "dim":
			c = cmp64(a.Dim, b.Dim)
		case "quando":
			c = cmp64(a.Quando.UnixNano(), b.Quando.UnixNano())
		case "tipo":
			c = strings.Compare(a.Categoria, b.Categoria)
			if c == 0 {
				c = strings.Compare(a.Ext, b.Ext)
			}
		}
		if c == 0 {
			c = ConfrontaNomi(a.Nome, b.Nome)
		}
		if discendente {
			return c > 0
		}
		return c < 0
	})
}

func cmp64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// ---------------- ricerca ----------------

// OpzioniTrova: limiti della ricerca.
type OpzioniTrova struct {
	MaxRisultati int           // 0 = 500
	MaxCartelle  int           // cartelle visitate al massimo (0 = 20000)
	Tempo        time.Duration // 0 = 15 secondi
	Nascosti     bool
}

// RisultatoTrova: cosa è stato trovato.
type RisultatoTrova struct {
	Radice   string `json:"radice"`
	Cerca    string `json:"cerca"`
	Voci     []Voce `json:"voci"`
	Cartelle int    `json:"cartelle"` // cartelle guardate
	Troppi   bool   `json:"troppi"`   // fermata al numero massimo di risultati
	Fermata  bool   `json:"fermata"`  // fermata per tempo o per numero di cartelle
}

// Corrisponde: il nome contiene il testo cercato (senza maiuscole e accenti); con * e ?
// funziona come in Windows ("*.pdf", "foto??.jpg").
func Corrisponde(nome, cerca string) bool {
	n, c := pulisci(nome), pulisci(strings.TrimSpace(cerca))
	if c == "" {
		return false
	}
	if strings.ContainsAny(c, "*?") {
		ok, err := filepath.Match(c, n)
		return err == nil && ok
	}
	return strings.Contains(n, c)
}

func pulisci(s string) string {
	var b strings.Builder
	for _, r := range s {
		b.WriteRune(piega(r))
	}
	return b.String()
}

// Trova cerca per nome nella cartella e nelle sottocartelle (prima quelle vicine). Non segue
// mai i collegamenti e le giunzioni (niente giri infiniti).
func Trova(ctx context.Context, radice, cerca string, opz OpzioniTrova) (RisultatoTrova, error) {
	if opz.MaxRisultati <= 0 {
		opz.MaxRisultati = 500
	}
	if opz.MaxCartelle <= 0 {
		opz.MaxCartelle = 20000
	}
	if opz.Tempo <= 0 {
		opz.Tempo = 15 * time.Second
	}
	radice = filepath.Clean(radice)
	ris := RisultatoTrova{Radice: radice, Cerca: cerca, Voci: []Voce{}}
	if strings.TrimSpace(cerca) == "" {
		return ris, nil
	}
	if fi, err := os.Stat(radice); err != nil {
		return ris, err
	} else if !fi.IsDir() {
		return ris, ErrNonCartella
	}
	scadenza := time.Now().Add(opz.Tempo)
	coda := []string{radice}
	for len(coda) > 0 {
		if ctx.Err() != nil {
			return ris, ctx.Err()
		}
		if ris.Cartelle >= opz.MaxCartelle || time.Now().After(scadenza) {
			ris.Fermata = true
			break
		}
		dir := coda[0]
		coda = coda[1:]
		ris.Cartelle++
		voci, _ := leggiPoche(dir, 50000)
		for _, de := range voci {
			v, ok := voceDa(dir, de)
			if !ok || (v.Nascosto && !opz.Nascosti) {
				continue
			}
			if Corrisponde(v.Nome, cerca) {
				if rel, err := filepath.Rel(radice, dir); err == nil && rel != "." {
					v.Dove = rel
				}
				ris.Voci = append(ris.Voci, v)
				if len(ris.Voci) >= opz.MaxRisultati {
					ris.Troppi = true
					return ris, nil
				}
			}
			// Solo cartelle vere: niente collegamenti o giunzioni (punti di rianalisi).
			if de.IsDir() && de.Type()&(fs.ModeSymlink|fs.ModeIrregular) == 0 && !giunzione(filepath.Join(dir, de.Name())) {
				coda = append(coda, filepath.Join(dir, de.Name()))
			}
		}
	}
	return ris, nil
}

func leggiPoche(dir string, max int) ([]fs.DirEntry, error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	v, err := f.ReadDir(max)
	sort.Slice(v, func(i, j int) bool { return ConfrontaNomi(v[i].Name(), v[j].Name()) < 0 })
	return v, err
}

// ---------------- misura ----------------

// Misura: quanti file e quanto pesano (per "Info").
type Misura struct {
	File     int   `json:"file"`
	Cartelle int   `json:"cartelle"`
	Byte     int64 `json:"byte"`
	Fermata  bool  `json:"fermata"` // troppi file o troppo tempo: i numeri sono "almeno"
}

// MisuraPercorsi conta file, cartelle e byte (senza seguire collegamenti), con limiti.
func MisuraPercorsi(ctx context.Context, percorsi []string, maxFile int, tempo time.Duration) Misura {
	if maxFile <= 0 {
		maxFile = 500000
	}
	if tempo <= 0 {
		tempo = 10 * time.Second
	}
	var m Misura
	scadenza := time.Now().Add(tempo)
	for _, p := range percorsi {
		_ = filepath.WalkDir(p, func(q string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if ctx.Err() != nil || m.File+m.Cartelle >= maxFile || time.Now().After(scadenza) {
				m.Fermata = true
				return filepath.SkipAll
			}
			if d.Type()&fs.ModeSymlink != 0 {
				return nil
			}
			if d.IsDir() {
				if q != p {
					m.Cartelle++
				}
				if q != p && giunzione(q) {
					return filepath.SkipDir
				}
				return nil
			}
			if fi, err := d.Info(); err == nil {
				m.File++
				m.Byte += fi.Size()
			}
			return nil
		})
	}
	return m
}
