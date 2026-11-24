package installa

import (
	"path/filepath"
	"strings"
)

// Nomi delle sottocartelle e dei file fissi.
const (
	CartellaZip      = "Tasso Zip"
	CartellaPDF      = "Tasso PDF"
	NomeDisinstalla  = "Disinstalla Tasso.exe"
	cartellaNuovi    = ".tasso-setup-nuovo"   // file nuovi in arrivo (poi rinominati al loro posto)
	cartellaVecchi   = ".tasso-setup-vecchio" // file sostituiti (tenuti fino alla fine per tornare indietro)
	CartellaTassoStd = "Tasso"
)

// CartellaPredefinita: C:\Program Files\Tasso (programFiles arriva da Windows).
func CartellaPredefinita(programFiles string) string {
	return filepath.Join(programFiles, CartellaTassoStd)
}

// CartellaDi dice dove va un programma: Tasso e il disinstallatore nella cartella
// scelta, Zip e PDF in una loro sottocartella.
func CartellaDi(base string, p Programma) string {
	switch p {
	case ProgZip:
		return filepath.Join(base, CartellaZip)
	case ProgPDF:
		return filepath.Join(base, CartellaPDF)
	}
	return base
}

// Codici degli errori sulla cartella: sono anche le chiavi dei testi tradotti.
const (
	ErrCartVuota      = "cart_vuota"
	ErrCartNonAssol   = "cart_non_assoluta"
	ErrCartCaratteri  = "cart_caratteri"
	ErrCartRadice     = "cart_radice"
	ErrCartWindows    = "cart_windows"
	ErrCartLunga      = "cart_lunga"
	ErrCartNomeRiserv = "cart_nome_riservato"
)

// ErroreCartella ha un codice da tradurre.
type ErroreCartella struct{ Codice string }

func (e *ErroreCartella) Error() string { return "cartella non valida: " + e.Codice }

var nomiRiservati = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true}

func init() {
	for _, n := range "123456789" {
		nomiRiservati["COM"+string(n)] = true
		nomiRiservati["LPT"+string(n)] = true
	}
}

// ControllaCartellaWin controlla una cartella scritta alla maniera di Windows
// (C:\Program Files\Tasso). È solo testo: si prova anche su Linux.
// windows è la cartella di Windows (es. C:\Windows): dentro non si installa.
func ControllaCartellaWin(p, windows string) error {
	p = strings.TrimSpace(p)
	if p == "" {
		return &ErroreCartella{ErrCartVuota}
	}
	if len(p) > 180 {
		return &ErroreCartella{ErrCartLunga}
	}
	if len(p) < 3 || !(p[0] >= 'A' && p[0] <= 'Z' || p[0] >= 'a' && p[0] <= 'z') || p[1] != ':' || p[2] != '\\' {
		return &ErroreCartella{ErrCartNonAssol} // anche \\server\condivisa: niente cartelle di rete
	}
	resto := strings.TrimRight(p[3:], `\`)
	if resto == "" {
		return &ErroreCartella{ErrCartRadice}
	}
	for _, parte := range strings.Split(resto, `\`) {
		if parte == "" || parte == "." || parte == ".." {
			return &ErroreCartella{ErrCartCaratteri}
		}
		if strings.ContainsAny(parte, `<>:"/|?*`) || strings.HasSuffix(parte, " ") || strings.HasSuffix(parte, ".") {
			return &ErroreCartella{ErrCartCaratteri}
		}
		for _, r := range parte {
			if r < 32 {
				return &ErroreCartella{ErrCartCaratteri}
			}
		}
		nome := strings.ToUpper(parte)
		if i := strings.IndexByte(nome, '.'); i >= 0 {
			nome = nome[:i]
		}
		if nomiRiservati[strings.TrimSpace(nome)] {
			return &ErroreCartella{ErrCartNomeRiserv}
		}
	}
	if w := strings.TrimRight(windows, `\`); w != "" {
		pl, wl := strings.ToLower(strings.TrimRight(p, `\`)), strings.ToLower(w)
		if pl == wl || strings.HasPrefix(pl, wl+`\`) {
			return &ErroreCartella{ErrCartWindows}
		}
	}
	return nil
}

// DentroProgrammi dice se la cartella è dentro Programmi (consigliato: lì solo gli
// amministratori possono cambiare i file, e Tasso usa sempre C:\Program Files\Tasso).
func DentroProgrammi(p, programFiles string) bool {
	pl := strings.ToLower(strings.TrimRight(p, `\/`))
	pf := strings.ToLower(strings.TrimRight(programFiles, `\/`))
	return pf != "" && strings.HasPrefix(pl, pf+string(filepath.Separator))
}

// nomeSicuro: un nome di file relativo, senza "..", senza percorso assoluto.
func nomeSicuro(n string) bool {
	if n == "" || strings.ContainsAny(n, ":\x00") || strings.HasPrefix(n, "/") || strings.HasPrefix(n, `\`) {
		return false
	}
	for _, parte := range strings.FieldsFunc(n, func(r rune) bool { return r == '/' || r == '\\' }) {
		if parte == ".." || parte == "." {
			return false
		}
	}
	return true
}

// percorsoFile unisce cartella e nome (con / nell'elenco) in modo sicuro.
func percorsoFile(dir, nome string) (string, bool) {
	if !nomeSicuro(nome) {
		return "", false
	}
	return filepath.Join(dir, filepath.FromSlash(strings.ReplaceAll(nome, `\`, "/"))), true
}
