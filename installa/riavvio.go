package installa

import (
	"path/filepath"
	"strings"
)

// Cancellazioni al riavvio rimaste da una disinstallazione (PendingFileRenameOperations di Windows:
// coppie "origine, destinazione"; destinazione vuota = cancella al riavvio). Reinstallando Tasso
// nella stessa cartella prima di riavviare, Windows al riavvio cancellerebbe i file NUOVI con lo
// stesso nome (Tasso.exe, la cartella lavoro...). Dopo un'installazione riuscita tolgo queste
// cancellazioni, ma solo quelle dentro la cartella di Tasso e solo per file che adesso servono:
// restano quelle delle copie da buttare (.tasso-setup-*, *.vecchio-N, Tasso.exe.old...).
// E solo per i programmi che si stanno installando: reinstallando solo Tasso, le cancellazioni
// dei file di Tasso Zip e Tasso PDF (tolti prima) restano, e viceversa.

// TogliCancellazioniPendenti: voci è il valore di PendingFileRenameOperations (a coppie). Restituisce
// le voci senza le cancellazioni dentro base da annullare, e quante ne ha tolte. Le rinomine vere
// (destinazione non vuota), quelle fuori da base e quelle dei resti da buttare restano, in ordine.
// progs: i programmi appena installati (ProgrammiPer delle opzioni). Se le voci non sono a coppie
// non tocca niente.
func TogliCancellazioniPendenti(voci []string, base string, progs []Programma) ([]string, int) {
	if len(voci)%2 != 0 || base == "" {
		return voci, 0
	}
	out := make([]string, 0, len(voci))
	tolte := 0
	for i := 0; i < len(voci); i += 2 {
		da, a := voci[i], voci[i+1]
		if a == "" && cancellazioneDaAnnullare(percorsoPendente(da), base, progs) {
			tolte++
			continue
		}
		out = append(out, da, a)
	}
	return out, tolte
}

// percorsoPendente: "\??\C:\..." -> "C:\...".
func percorsoPendente(p string) string {
	p = strings.TrimPrefix(p, `\??\`)
	return strings.TrimPrefix(p, `\\?\`)
}

func cancellazioneDaAnnullare(p, base string, progs []Programma) bool {
	if !Dentro(p, base) {
		return false
	}
	np, nb := normPercorso(p), normPercorso(base)
	if np == nb {
		return true // la cartella stessa: adesso c'è di nuovo Tasso
	}
	rel := strings.TrimPrefix(np, nb+`\`)
	prima := strings.SplitN(rel, `\`, 2)[0]
	if prima == cartellaNuovi || prima == cartellaVecchi || prima == cartellaScarto {
		return false
	}
	nome := strings.ToLower(filepath.Base(strings.ReplaceAll(rel, `\`, "/")))
	if strings.Contains(nome, ".vecchio-") {
		return false
	}
	for _, r := range ResiduiAggiornamentoTasso {
		if nome == strings.ToLower(r) {
			return false
		}
	}
	// di quale programma è: Tasso Zip e Tasso PDF hanno la loro cartella, il resto è di Tasso,
	// tranne il disinstallatore e tasso-setup.json (si riscrivono con qualunque programma)
	di := ProgTasso
	switch {
	case strings.EqualFold(prima, CartellaZip):
		di = ProgZip
	case strings.EqualFold(prima, CartellaPDF):
		di = ProgPDF
	case !strings.Contains(rel, `\`) && (nome == strings.ToLower(NomeDisinstalla) || nome == FileStato):
		return len(progs) > 0
	}
	for _, x := range progs {
		if x == di {
			return true
		}
	}
	return false
}
