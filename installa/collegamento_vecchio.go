package installa

import (
	"path/filepath"
	"strings"
)

// NomeCollegamentoVecchio: il collegamento che Tasso 3.0 (portatile) metteva sul Desktop
// dell'utente con "Collegamento sul Desktop": Desktop\Tasso.lnk -> il Tasso.exe scaricato.
const NomeCollegamentoVecchio = "Tasso.lnk"

// CollegamentoVecchioDaAggiornare: il Desktop\Tasso.lnk letto (destinazione, argomenti,
// descrizione) è quello della 3.0 portatile e si può far puntare al Tasso installato. Solo se:
// porta a un file che si chiama Tasso.exe, fuori da Programmi (non un'installazione), senza
// argomenti, e la descrizione è di Tasso. In ogni altro caso non si tocca.
func CollegamentoVecchioDaAggiornare(dest, argomenti, descrizione, programFiles string) bool {
	dest = strings.TrimSpace(dest)
	if dest == "" || len(dest) < 3 || dest[1] != ':' || strings.TrimSpace(argomenti) != "" {
		return false
	}
	if !strings.EqualFold(filepathBaseWin(dest), "Tasso.exe") {
		return false
	}
	if programFiles == "" || Dentro(dest, programFiles) {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(descrizione), "Tasso")
}

// percorsoCollegamentoVecchio: Desktop\Tasso.lnk ("" senza Desktop).
func PercorsoCollegamentoVecchio(desktop string) string {
	if desktop == "" {
		return ""
	}
	return filepath.Join(desktop, NomeCollegamentoVecchio)
}

// CollegamentoVecchioNostro: il Desktop\Tasso.lnk (destinazione dest, argomenti) è quello della 3.0
// che aggiornaCollegamentoVecchio ha fatto puntare al Tasso installato in base: porta proprio a
// base\Tasso.exe, senza argomenti. Togliendo Tasso va via anche lui (altrimenti resterebbe sul
// Desktop a puntare a un Tasso.exe che non c'è più). In ogni altro caso non si tocca.
func CollegamentoVecchioNostro(dest, argomenti, base string) bool {
	if strings.TrimSpace(argomenti) != "" || normPercorso(base) == "" {
		return false
	}
	d := normPercorso(dest)
	return d != "" && d == normPercorso(base)+`\tasso.exe`
}
