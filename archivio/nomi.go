// Tasso Zip - comprime ed estrae i tuoi file.
// Copyright (C) 2026 Sergio Tassoni. Licenza GPL-3.0 (vedi LICENSE).

package archivio

import (
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var suWindows = runtime.GOOS == "windows"

// estensioniDoppie: estensioni da tenere insieme quando si aggiunge " (2)" al nome.
var estensioniDoppie = []string{".tar.gz", ".tar.bz2", ".tar.xz", ".tar.zst"}

// DividiNome separa il nome dall'estensione ("Foto.tar.xz" -> "Foto", ".tar.xz").
// Le cartelle non hanno estensione.
func DividiNome(nome string, cartella bool) (base, ext string) {
	if cartella {
		return nome, ""
	}
	l := strings.ToLower(nome)
	for _, e := range estensioniDoppie {
		if strings.HasSuffix(l, e) && len(nome) > len(e) {
			return nome[:len(nome)-len(e)], nome[len(nome)-len(e):]
		}
	}
	ext = filepath.Ext(nome)
	if ext == nome || len(ext) > 12 || strings.ContainsAny(ext, " ") {
		return nome, "" // ".bashrc" o nomi strani: nessuna estensione
	}
	return nome[:len(nome)-len(ext)], ext
}

// NomeNumero restituisce il nome con il numero n: "Foto.zip" -> "Foto (2).zip".
func NomeNumero(nome string, cartella bool, n int) string {
	if n <= 1 {
		return nome
	}
	b, e := DividiNome(nome, cartella)
	return b + " (" + strconv.Itoa(n) + ")" + e
}

// nomiRiservati di Windows: un file con questi nomi (anche con estensione) non si può creare.
var nomiRiservati = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true, "conin$": true, "conout$": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true, "com6": true, "com7": true, "com8": true, "com9": true,
	"com¹": true, "com²": true, "com³": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
	"lpt¹": true, "lpt²": true, "lpt³": true,
}

// pulisciPezzo rende sicuro un pezzo di percorso (un nome di cartella o di file) per Windows.
// Restituisce "" se il pezzo va scartato ("", ".", "..").
func pulisciPezzo(p string) string {
	if !utf8.ValidString(p) {
		p = strings.ToValidUTF8(p, "_")
	}
	var b strings.Builder
	for _, r := range p {
		switch {
		case r < 32 || r == 127:
			b.WriteRune('_')
		case strings.ContainsRune(`<>:"|?*/\`, r):
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	p = b.String()
	// Windows toglie da solo punti e spazi alla fine: ".. " diventerebbe "..". Li tolgo prima.
	p = strings.TrimRightFunc(p, func(r rune) bool { return r == '.' || r == ' ' || unicode.IsSpace(r) })
	p = strings.TrimLeftFunc(p, unicode.IsSpace)
	if p == "" || p == "." || p == ".." {
		return ""
	}
	base := strings.ToLower(p)
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	base = strings.TrimRight(base, " ")
	if nomiRiservati[base] {
		p = "_" + p
	}
	if len(p) > 240 {
		// nomi lunghissimi: li accorcio senza spezzare le lettere
		taglio := 240
		for taglio > 0 && !utf8.RuneStart(p[taglio]) {
			taglio--
		}
		p = p[:taglio]
	}
	return p
}

var unitaDisco = regexp.MustCompile(`^[A-Za-z]:`)

// NomeSicuro trasforma il nome di una voce dell'archivio in un percorso relativo sicuro,
// fatto di pezzi separati. Rifiuta (ok=false) i nomi che proverebbero a uscire dalla
// cartella: percorsi assoluti, unità ("C:"), percorsi di rete ("\\server") e "..".
func NomeSicuro(nome string) (pezzi []string, ok bool) {
	n := strings.ReplaceAll(nome, `\`, "/")
	if strings.HasPrefix(n, "/") || unitaDisco.MatchString(n) {
		return nil, false
	}
	for _, p := range strings.Split(n, "/") {
		grezzo := strings.TrimRightFunc(p, func(r rune) bool { return r == '.' || r == ' ' })
		if p != "" && p != "." && grezzo == "" && strings.Contains(p, "..") {
			return nil, false // "..", "... ", ".. ": tentativo di risalire
		}
		if p == ".." {
			return nil, false
		}
		q := pulisciPezzo(p)
		if q == "" {
			continue
		}
		pezzi = append(pezzi, q)
	}
	if len(pezzi) == 0 {
		return nil, false
	}
	return pezzi, true
}

var (
	reParteRar    = regexp.MustCompile(`(?i)^(.*)\.part0*(\d+)\.rar$`)
	reVecchiaRar  = regexp.MustCompile(`(?i)^(.*)\.r(\d\d)$`)
	reParteNumero = regexp.MustCompile(`^(.*)\.(\d{3})$`)
)

// NomeBaseArchivio: il nome della cartella da creare per un archivio
// ("Foto.tar.gz" -> "Foto", "Lavoro.part1.rar" -> "Lavoro", "Film.7z.001" -> "Film").
func NomeBaseArchivio(percorso string) string {
	n := filepath.Base(percorso)
	if m := reParteNumero.FindStringSubmatch(n); m != nil {
		n = m[1]
	}
	if m := reParteRar.FindStringSubmatch(n); m != nil {
		n = m[1]
	} else if m := reVecchiaRar.FindStringSubmatch(n); m != nil {
		n = m[1]
	}
	l := strings.ToLower(n)
	for _, e := range []string{".tar.gz", ".tar.bz2", ".tar.xz", ".tar.zst", ".tgz", ".tbz2", ".tbz", ".txz", ".tzst", ".zip", ".7z", ".rar", ".tar", ".gz", ".bz2", ".xz", ".zst"} {
		if strings.HasSuffix(l, e) && len(n) > len(e) {
			n = n[:len(n)-len(e)]
			break
		}
	}
	n = pulisciPezzo(n)
	if n == "" {
		n = "Archivio"
	}
	return n
}
