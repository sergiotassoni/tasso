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
