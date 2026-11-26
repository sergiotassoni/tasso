// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"tassozip/rianalisi"
	"time"

	"tassozip/archivio/settezip"

	"github.com/dsnet/compress/bzip2"
	"github.com/klauspost/compress/flate"
	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// Formato dell'archivio da creare.
type Formato string

const (
	// FormatoTarXz: compressione massima (.tar.xz, LZMA2: lo stesso metodo del 7z).
	FormatoTarXz Formato = "tarxz"
	// FormatoZip: compatibile con tutto (.zip, Deflate al livello massimo).
	FormatoZip Formato = "zip"
	// Formato7z: super compressione (.7z, LZMA2, solido, scritto da noi).
	Formato7z Formato = "7z"
	// FormatoTarZst: veloce (.tar.zst, Zstandard).
	FormatoTarZst Formato = "tarzst"
	// FormatoTar: tar senza compressione (.tar), per Linux e Mac.
	FormatoTar Formato = "tar"
	// FormatoTarGz: tar con gzip (.tar.gz), il classico di Linux e Mac.
	FormatoTarGz Formato = "targz"
	// FormatoTarBz2: tar con bzip2 (.tar.bz2).
	FormatoTarBz2 Formato = "tarbz2"
)

// FormatiCreabili: tutti i formati che Tasso Zip sa scrivere (in formato standard, con i test
// che li rileggono byte per byte), nell'ordine in cui li propone. Il RAR no: è un formato
// proprietario e la sua licenza non permette di crearlo.
var FormatiCreabili = []Formato{FormatoZip, Formato7z, FormatoTar, FormatoTarGz, FormatoTarXz, FormatoTarBz2, FormatoTarZst}

// FormatoValido dice se il formato si sa scrivere.
func FormatoValido(f string) bool {
	for _, x := range FormatiCreabili {
		if string(x) == f {
			return true
		}
	}
	return false
}

// Estensione del file creato.
func (f Formato) Estensione() string {
	switch f {
	case FormatoTarXz:
		return ".tar.xz"
	case Formato7z:
		return ".7z"
	case FormatoTarZst:
		return ".tar.zst"
	case FormatoTar:
		return ".tar"
	case FormatoTarGz:
		return ".tar.gz"
	case FormatoTarBz2:
		return ".tar.bz2"
	}
	return ".zip"
}

// HaLivello: il livello (Veloce...Ultra) cambia qualcosa per questo formato?
func (f Formato) HaLivello() bool {
	switch f {
	case Formato7z, FormatoTarGz, FormatoTarBz2, FormatoTarZst, FormatoTarXz:
		return true
	}
	return false
}

// Livello di compressione per il 7z: "veloce", "normale", "massima", "ultra".
type Livello string

const (
	LivelloVeloce  Livello = "veloce"
	LivelloNormale Livello = "normale"
	LivelloMassima Livello = "massima"
	LivelloUltra   Livello = "ultra"
)

func (l Livello) settezip() settezip.Livello {
	switch l {
	case LivelloVeloce:
		return settezip.Veloce
	case LivelloNormale:
		return settezip.Normale
	case LivelloUltra:
		return settezip.Ultra
	}
	return settezip.Massima
}

// OpzioniComprimi: come comprimere.
type OpzioniComprimi struct {
	Formato         Formato
	Livello         Livello // 7z, tar.gz, tar.bz2, tar.xz, tar.zst ("" = massima); vedi HaLivello
	Nome            string  // nome dell'archivio senza estensione ("" = dal primo elemento)
	NomeMultiplo    string  // nome se scegli più elementi (es. "Archivio"); "" = "Archivio"
	Cartella        string  // dove salvarlo ("" = accanto al primo elemento)
	CartellaRiserva string  // se lì non si può scrivere (es. Desktop)
	Avanzamento     FunzioneAvanzamento
}

// RisultatoComprimi: cosa è stato fatto.
type RisultatoComprimi struct {
	Archivio  string
	File      int
	Cartelle  int
	Originale int64 // byte dei file originali
	Compresso int64 // byte dell'archivio
	Saltati   []string
	Riserva   bool
}

type voce struct {
	percorso string
	nome     string // nome dentro l'archivio, con "/"
	cartella bool
	dim      int64
	quando   time.Time
}

// estensioniGiaCompresse: file che non si comprimono più (foto, video, archivi...):
// nello zip li metto così come sono, senza perdere tempo.
var estensioniGiaCompresse = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".heic": true, ".avif": true,
	".mp3": true, ".m4a": true, ".aac": true, ".ogg": true, ".opus": true, ".flac": true,
	".mp4": true, ".m4v": true, ".mkv": true, ".avi": true, ".mov": true, ".webm": true, ".wmv": true,
	".zip": true, ".7z": true, ".rar": true, ".gz": true, ".bz2": true, ".xz": true, ".zst": true, ".tgz": true,
	".docx": true, ".xlsx": true, ".pptx": true, ".odt": true, ".ods": true, ".epub": true, ".jar": true, ".apk": true,
}

// Comprimi crea un archivio con i file e le cartelle scelti. Scrive prima un file
// temporaneo accanto e lo rinomina solo alla fine (mai sovrascrivere: "Foto (2).zip").
func Comprimi(ctx context.Context, sorgenti []string, opz OpzioniComprimi) (ris RisultatoComprimi, err error) {
	if !FormatoValido(string(opz.Formato)) {
		opz.Formato = FormatoZip
	}
	var puliti []string
	visti := map[string]bool{}
	for _, s := range sorgenti {
		if strings.TrimSpace(s) == "" {
			continue
		}
		a, e := filepath.Abs(s)
		if e != nil {
			return ris, e
		}
		if visti[chiave(a)] {
			continue
		}
		visti[chiave(a)] = true
		puliti = append(puliti, a)
	}
	if len(puliti) == 0 {
		return ris, ErrNienteDaFare
	}

	// Dove salvare e con che nome.
	dove := opz.Cartella
	if dove == "" {
		dove = filepath.Dir(puliti[0])
	}
	nome := opz.Nome
	if nome == "" {
		if len(puliti) == 1 {
			fi, e := os.Lstat(puliti[0])
			if e != nil {
				return ris, e
			}
			nome, _ = DividiNome(filepath.Base(puliti[0]), fi.IsDir())
		} else {
			nome = opz.NomeMultiplo
		}
	}
	nome = pulisciPezzo(nome)
	if nome == "" {
		nome = "Archivio"
	}

	// File di lavoro.
	tmpNome := ""
	var out *os.File
	apri := func(dir string) error {
		for i := 0; i < 20; i++ {
			p := filepath.Join(dir, nomeTemporaneo(".tmp"))
			f, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if e == nil {
				tmpNome, out = p, f
				return nil
			}
			if !errors.Is(e, fs.ErrExist) {
				return e
			}
		}
		return errors.New("non riesco a creare il file di lavoro")
	}
	PulisciAvanzi(dove)
	if e := apri(dove); e != nil {
		if opz.CartellaRiserva == "" {
			return ris, fmt.Errorf("%w: %v", ErrNonScrivibile, e)
		}
		dove = opz.CartellaRiserva
		ris.Riserva = true
		if e := apri(dove); e != nil {
			return ris, fmt.Errorf("%w: %v", ErrNonScrivibile, e)
		}
	}
	finito := false
	defer func() {
		if out != nil {
			out.Close()
		}
		if !finito {
			_ = os.Remove(tmpNome) // annullato o errore: niente a metà
		}
	}()

	// L'elenco di tutto quello che va nell'archivio.
	voci, saltati, err := elencaVoci(ctx, puliti, tmpNome)
	if err != nil {
		return ris, err
	}
	ris.Saltati = saltati
	if len(voci) == 0 {
		return ris, ErrNienteDaFare
	}
	var totale int64
	for _, v := range voci {
		totale += v.dim
	}
	if lib := spazioLibero(dove); lib >= 0 && lib < 64<<20 {
		return ris, ErrSpazio
	}

	c := &copiatore{ctx: ctx, totale: totale, avanzamento: opz.Avanzamento, buf: make([]byte, 256<<10)}
	if opz.Formato == Formato7z {
		err = scrivi7z(ctx, c, out, dove, voci, opz.Livello, &ris)
	} else {
		bw := bufio.NewWriterSize(out, 1<<20)
		switch opz.Formato {
		case FormatoTarXz:
			err = scriviTarXz(c, bw, voci, opz.Livello, &ris)
		case FormatoTarZst:
			err = scriviTarZst(c, bw, voci, opz.Livello, &ris)
		case FormatoTar:
			err = scriviTar(c, bw, voci, &ris)
		case FormatoTarGz:
			err = scriviTarGz(c, bw, voci, opz.Livello, &ris)
		case FormatoTarBz2:
			err = scriviTarBz2(c, bw, voci, opz.Livello, &ris)
		default:
			err = scriviZip(c, bw, voci, &ris)
		}
		if err == nil {
			err = bw.Flush()
		}
	}
	if err == nil {
		err = out.Sync()
	}
	if e := out.Close(); err == nil {
		err = e
	}
	out = nil
	if err == nil {
		err = c.annullato()
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			err = ErrAnnullato
		}
		return ris, err
	}
	finale, err := spostaConNomeLibero(tmpNome, dove, nome+opz.Formato.Estensione(), false)
	if err != nil {
		return ris, err
	}
	finito = true
	ris.Archivio = finale
	ris.Originale = totale
	if fi, e := os.Stat(finale); e == nil {
		ris.Compresso = fi.Size()
	}
	return ris, nil
}

// normale: un file vero (anche se Windows lo tiene come punto di rianalisi non collegamento:
// OneDrive, file compressi da Windows). Va chiamata dopo rianalisi.Collegamento.
func normale(fi fs.FileInfo) bool {
	m := fi.Mode()
	return m.IsRegular() || (m&fs.ModeIrregular != 0 && !m.IsDir() && m&(fs.ModeNamedPipe|fs.ModeSocket|fs.ModeDevice|fs.ModeCharDevice) == 0)
}

// elencaVoci percorre file e cartelle. Non segue mai i collegamenti (symlink, giunzioni).
func elencaVoci(ctx context.Context, sorgenti []string, escludi string) ([]voce, []string, error) {
	var voci []voce
	var saltati []string
	salta := func(n, motivo string) {
		if len(saltati) < 1000 {
			saltati = append(saltati, n+" ("+motivo+")")
		}
	}
	radici := map[string]bool{}
	for _, s := range sorgenti {
		fi, err := os.Lstat(s)
		if err != nil {
			return nil, nil, err
		}
		base := pulisciPezzo(filepath.Base(s))
		if base == "" {
			base = "file"
		}
		// Due elementi con lo stesso nome (da cartelle diverse): il secondo diventa "nome (2)".
		nomeRadice := base
		for n := 2; radici[chiave(nomeRadice)]; n++ {
			nomeRadice = NomeNumero(base, fi.IsDir(), n)
		}
		radici[chiave(nomeRadice)] = true

		switch {
		case rianalisi.Collegamento(s, fi):
			salta(filepath.Base(s), "collegamento")
		case normale(fi):
			if s == escludi {
				continue
			}
			voci = append(voci, voce{percorso: s, nome: nomeRadice, dim: fi.Size(), quando: fi.ModTime()})
		case fi.IsDir():
			err := filepath.WalkDir(s, func(p string, d fs.DirEntry, err error) error {
				select {
				case <-ctx.Done():
					return ErrAnnullato
				default:
				}
				rel, _ := filepath.Rel(s, p)
				nomeArch := nomeRadice
				if rel != "." {
					nomeArch = nomeRadice + "/" + filepath.ToSlash(rel)
				}
				if err != nil {
					salta(nomeArch, "non_leggibile")
					if d != nil && d.IsDir() {
						return fs.SkipDir
					}
					return nil
				}
				if p == escludi {
					return nil
				}
				info, e := d.Info()
				if e != nil {
					salta(nomeArch, "non_leggibile")
					return nil
				}
				// collegamenti e giunzioni no; OneDrive e i file compressi da Windows sì
				if t := d.Type(); t&(fs.ModeSymlink|fs.ModeIrregular) != 0 && rianalisi.Collegamento(p, info) {
					salta(nomeArch, "collegamento")
					if d.IsDir() && p != s {
						return fs.SkipDir
					}
					return nil
				}
				if d.IsDir() {
					voci = append(voci, voce{percorso: p, nome: nomeArch, cartella: true, quando: info.ModTime()})
					return nil
				}
				if !normale(info) {
					salta(nomeArch, "file_speciale")
					return nil
				}
				voci = append(voci, voce{percorso: p, nome: nomeArch, dim: info.Size(), quando: info.ModTime()})
				return nil
			})
			if err != nil {
				return nil, nil, err
			}
		default:
			salta(filepath.Base(s), "file_speciale")
		}
	}
	return voci, saltati, nil
}

// copiatore copia i dati controllando l'annullamento e aggiornando l'avanzamento.
type copiatore struct {
	ctx         context.Context
	fatto       int64
	totale      int64
	avanzamento FunzioneAvanzamento
	buf         []byte
}

func (c *copiatore) annullato() error {
	select {
	case <-c.ctx.Done():
		return ErrAnnullato
	default:
		return nil
	}
}

func (c *copiatore) copia(w io.Writer, r io.Reader, nome string, max int64) (int64, error) {
	var tot int64
	for max < 0 || tot < max {
		if err := c.annullato(); err != nil {
			return tot, err
		}
		b := c.buf
		if max >= 0 && int64(len(b)) > max-tot {
			b = b[:max-tot]
		}
		n, errL := r.Read(b)
		if n > 0 {
			if _, err := w.Write(b[:n]); err != nil {
				return tot, err
			}
			tot += int64(n)
			c.fatto += int64(n)
			if c.avanzamento != nil {
				c.avanzamento(Avanzamento{Fatto: c.fatto, Totale: c.totale, Nome: nome})
			}
		}
		if errL == io.EOF {
			break
		}
		if errL != nil {
			return tot, errL
		}
	}
	return tot, nil
}

func scriviZip(c *copiatore, w io.Writer, voci []voce, ris *RisultatoComprimi) error {
	zw := zip.NewWriter(w)
	zw.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, flate.BestCompression)
	})
	for _, v := range voci {
		if err := c.annullato(); err != nil {
			return err
		}
		h := &zip.FileHeader{Name: v.nome, Modified: orarioZip(v.quando)}
		if v.cartella {
			h.Name += "/"
			h.SetMode(fs.ModeDir | 0o755)
			h.Method = zip.Store
			if _, err := zw.CreateHeader(h); err != nil {
				return err
			}
			ris.Cartelle++
			continue
		}
		h.SetMode(0o644)
		h.Method = zip.Deflate
		if estensioniGiaCompresse[strings.ToLower(filepath.Ext(v.nome))] {
			h.Method = zip.Store
		}
		f, err := os.Open(v.percorso)
		if err != nil {
			ris.Saltati = append(ris.Saltati, v.nome+" (non_leggibile)")
			continue
		}
		dst, err := zw.CreateHeader(h)
		if err != nil {
			f.Close()
			return err
		}
		_, err = c.copia(dst, f, v.nome, -1)
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", v.nome, err)
		}
		ris.File++
	}
	return zw.Close()
}

// orarioZip: la data che lo zip sa scrivere. Il formato (data MS-DOS) va solo dal 1980 al
// 2107: una data fuori (es. un file del 1970) diventerebbe un anno assurdo come 2103 in
// Esplora file. Fuori dai limiti metto il limite più vicino.
func orarioZip(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now()
	}
	min := time.Date(1980, 1, 1, 0, 0, 0, 0, t.Location())
	max := time.Date(2106, 12, 31, 23, 59, 58, 0, t.Location()) // anche la data Unix a 32 bit
	if t.Before(min) {
		return min
	}
	if t.After(max) {
		return max
	}
	return t
}

// threadPerSpazio: con più core ogni pezzo compresso va prima in un file di lavoro e poi
// nell'archivio, quindi sul disco serve posto per due copie compresse (al massimo grandi come i
// file). Se non c'è sicuramente, un core solo: scrive direttamente nell'archivio. 0 = decide
// settezip (secondo core e RAM).
func threadPerSpazio(libero, totale int64) int {
	if libero >= 0 && libero < 2*totale+(256<<20) {
		return 1
	}
	return 0
}

// scrivi7z: super compressione con il nostro writer 7z (più core insieme).
func scrivi7z(ctx context.Context, c *copiatore, out *os.File, dove string, voci []voce, liv Livello, ris *RisultatoComprimi) error {
	var mu sync.Mutex
	sv := make([]settezip.Voce, len(voci))
	for i, v := range voci {
		p := v.percorso
		sv[i] = settezip.Voce{Nome: v.nome, Cartella: v.cartella, Dim: v.dim, Quando: v.quando,
			GiaCompresso: estensioniGiaCompresse[strings.ToLower(filepath.Ext(v.nome))],
			Apri:         func() (io.ReadCloser, error) { return os.Open(p) }}
	}
	var totale int64
	for _, v := range voci {
		totale += v.dim
	}
	o := settezip.Opzioni{
		Livello:   liv.settezip(),
		RAMLibera: memoriaLibera(),
		DirTemp:   dove,
		Thread:    threadPerSpazio(spazioLibero(dove), totale),
		Progresso: func(n int64, nome string) {
			mu.Lock()
			c.fatto += n
			if c.fatto > c.totale {
				c.totale = c.fatto
			}
			a := Avanzamento{Fatto: c.fatto, Totale: c.totale, Nome: nome}
			mu.Unlock()
			if c.avanzamento != nil {
				c.avanzamento(a)
			}
		},
		Saltato: func(nome string, _ error) {
			mu.Lock()
			ris.Saltati = append(ris.Saltati, nome+" (non_leggibile)")
			mu.Unlock()
		},
	}
	r, err := settezip.Scrivi(ctx, out, sv, o)
	if errors.Is(err, settezip.ErrAnnullato) {
		return ErrAnnullato
	}
	ris.File, ris.Cartelle = r.File, r.Cartelle
	return err
}

func scriviTarZst(c *copiatore, w io.Writer, voci []voce, liv Livello, ris *RisultatoComprimi) error {
	lz := zstd.SpeedBetterCompression
	switch liv {
	case LivelloVeloce:
		lz = zstd.SpeedFastest
	case LivelloNormale:
		lz = zstd.SpeedDefault
	case LivelloUltra:
		lz = zstd.SpeedBestCompression
	}
	zw, err := zstd.NewWriter(w, zstd.WithEncoderLevel(lz))
	if err != nil {
		return err
	}
	if err := scriviTar(c, zw, voci, ris); err != nil {
		zw.Close()
		return err
	}
	return zw.Close()
}

func scriviTarXz(c *copiatore, w io.Writer, voci []voce, liv Livello, ris *RisultatoComprimi) error {
	dict := 32 << 20
	switch liv {
	case LivelloVeloce:
		dict = 4 << 20
	case LivelloNormale:
		dict = 16 << 20
	case LivelloUltra:
		dict = 64 << 20
	}
	if m := memoriaLibera(); m > 0 && int64(dict)*12 > m {
		dict = 8 << 20 // poca memoria libera: dizionario più piccolo
	}
	cfg := xz.WriterConfig{DictCap: dict, CheckSum: xz.CRC64}
	xw, err := cfg.NewWriter(w)
	if err != nil {
		return err
	}
	if err := scriviTar(c, xw, voci, ris); err != nil {
		return err
	}
	return xw.Close()
}

func scriviTarGz(c *copiatore, w io.Writer, voci []voce, liv Livello, ris *RisultatoComprimi) error {
	l := gzip.BestCompression
	switch liv {
	case LivelloVeloce:
		l = gzip.BestSpeed
	case LivelloNormale:
		l = gzip.DefaultCompression
	}
	gw, err := gzip.NewWriterLevel(w, l)
	if err != nil {
		return err
	}
	if err := scriviTar(c, gw, voci, ris); err != nil {
		gw.Close()
		return err
	}
	return gw.Close()
}

func scriviTarBz2(c *copiatore, w io.Writer, voci []voce, liv Livello, ris *RisultatoComprimi) error {
	l := bzip2.BestCompression
	switch liv {
	case LivelloVeloce:
		l = bzip2.BestSpeed
	case LivelloNormale:
		l = bzip2.DefaultCompression
	}
	bw, err := bzip2.NewWriter(w, &bzip2.WriterConfig{Level: l})
	if err != nil {
		return err
	}
	if err := scriviTar(c, bw, voci, ris); err != nil {
		bw.Close()
		return err
	}
	return bw.Close()
}

func scriviTar(c *copiatore, xw io.Writer, voci []voce, ris *RisultatoComprimi) error {
	tw := tar.NewWriter(xw)
	for _, v := range voci {
		if err := c.annullato(); err != nil {
			return err
		}
		if v.cartella {
			if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: v.nome + "/", Mode: 0o755, ModTime: v.quando}); err != nil {
				return err
			}
			ris.Cartelle++
			continue
		}
		f, err := os.Open(v.percorso)
		if err != nil {
			ris.Saltati = append(ris.Saltati, v.nome+" (non_leggibile)")
			continue
		}
		// La dimensione la rileggo adesso: nel tar deve essere esatta.
		fi, err := f.Stat()
		if err != nil {
			f.Close()
			ris.Saltati = append(ris.Saltati, v.nome+" (non_leggibile)")
			continue
		}
		c.totale += fi.Size() - v.dim
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: v.nome, Mode: 0o644, Size: fi.Size(), ModTime: v.quando}); err != nil {
			f.Close()
			return err
		}
		n, err := c.copia(tw, f, v.nome, fi.Size())
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", v.nome, err)
		}
		if n != fi.Size() {
			return fmt.Errorf("%s: il file è cambiato mentre lo comprimevo", v.nome)
		}
		ris.File++
	}
	return tw.Close()
}
