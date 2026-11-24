// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
)

// volumi: un archivio diviso in parti "nome.001", "nome.002"... letto come un file solo.
// Conta anche i byte letti, per la barra di avanzamento.
type volumi struct {
	file   []*os.File
	inizio []int64 // posizione di inizio di ogni parte
	dim    int64
	letti  atomic.Int64
	nomi   []string
}

func apriVolumi(nomi []string) (*volumi, error) {
	v := &volumi{nomi: nomi}
	for _, n := range nomi {
		f, err := os.Open(n)
		if err != nil {
			v.Close()
			return nil, err
		}
		fi, err := f.Stat()
		if err != nil {
			f.Close()
			v.Close()
			return nil, err
		}
		if !fi.Mode().IsRegular() {
			f.Close()
			v.Close()
			return nil, fmt.Errorf("%s: non è un file", n)
		}
		v.file = append(v.file, f)
		v.inizio = append(v.inizio, v.dim)
		v.dim += fi.Size()
	}
	return v, nil
}

func (v *volumi) Size() int64 { return v.dim }

func (v *volumi) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("posizione negativa")
	}
	if off >= v.dim {
		return 0, io.EOF
	}
	letti := 0
	for len(p) > 0 && off < v.dim {
		i := sort.Search(len(v.inizio), func(i int) bool { return v.inizio[i] > off }) - 1
		fine := v.dim
		if i+1 < len(v.inizio) {
			fine = v.inizio[i+1]
		}
		m := int64(len(p))
		if m > fine-off {
			m = fine - off
		}
		n, err := v.file[i].ReadAt(p[:m], off-v.inizio[i])
		letti += n
		off += int64(n)
		p = p[n:]
		if err != nil && !(err == io.EOF && int64(n) == m) {
			v.letti.Add(int64(n))
			return letti, err
		}
	}
	v.letti.Add(int64(letti))
	if len(p) > 0 {
		return letti, io.EOF
	}
	return letti, nil
}

func (v *volumi) Close() error {
	for _, f := range v.file {
		f.Close()
	}
	v.file = nil
	return nil
}

// lettoreSequenziale: legge i volumi dall'inizio alla fine (per tar, gz, bz2, xz).
func (v *volumi) lettoreSequenziale() io.Reader {
	return io.NewSectionReader(v, 0, v.dim)
}

var reNumero3 = regexp.MustCompile(`^(.*)\.(\d{3})$`)

// partiNumerate: se il file è "nome.001" (o .002...), restituisce tutte le parti in ordine
// partendo da .001. Se manca una parte in mezzo dà ErrParteMancante.
func partiNumerate(percorso string) ([]string, bool, error) {
	m := reNumero3.FindStringSubmatch(percorso)
	if m == nil {
		return []string{percorso}, false, nil
	}
	larghezza := len(m[2])
	base := m[1]
	primo := fmt.Sprintf("%s.%0*d", base, larghezza, 1)
	if _, err := os.Stat(primo); err != nil {
		return nil, true, fmt.Errorf("%w: %s", ErrParteMancante, filepath.Base(primo))
	}
	var parti []string
	for i := 1; i < 100000; i++ {
		p := fmt.Sprintf("%s.%0*d", base, larghezza, i)
		if _, err := os.Stat(p); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				break
			}
			return nil, true, err
		}
		parti = append(parti, p)
	}
	// Una parte con numero più alto dopo un buco: ne manca una.
	dopo := fmt.Sprintf("%s.%0*d", base, larghezza, len(parti)+2)
	if _, err := os.Stat(dopo); err == nil {
		return nil, true, fmt.Errorf("%w: %s", ErrParteMancante, filepath.Base(fmt.Sprintf("%s.%0*d", base, larghezza, len(parti)+1)))
	}
	return parti, true, nil
}

var reParteRarN = regexp.MustCompile(`(?i)^(.*\.part)(\d+)(\.rar)$`)

// primaParteRar: da "Film.part3.rar" a "Film.part1.rar" (o part01, part001), da "Film.r05" a "Film.rar".
func primaParteRar(percorso string) (string, error) {
	dir, nome := filepath.Split(percorso)
	if m := reParteRarN.FindStringSubmatch(nome); m != nil {
		n, _ := strconv.Atoi(m[2])
		if n <= 1 {
			return percorso, nil
		}
		primo := filepath.Join(dir, m[1]+fmt.Sprintf("%0*d", len(m[2]), 1)+m[3])
		if _, err := os.Stat(primo); err != nil {
			return "", fmt.Errorf("%w: %s", ErrParteMancante, filepath.Base(primo))
		}
		return primo, nil
	}
	if m := reVecchiaRar.FindStringSubmatch(nome); m != nil {
		for _, e := range []string{".rar", ".RAR"} {
			primo := filepath.Join(dir, m[1]+e)
			if _, err := os.Stat(primo); err == nil {
				return primo, nil
			}
		}
		return "", fmt.Errorf("%w: %s.rar", ErrParteMancante, m[1])
	}
	return percorso, nil
}

// minuscolo: estensione in minuscolo.
func minuscolo(p string) string { return strings.ToLower(filepath.Base(p)) }
